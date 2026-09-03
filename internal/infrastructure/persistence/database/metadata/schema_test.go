package metadata

import (
	"database/sql"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	ormdialect "github.com/domainry/domainry-orm/dialect"
	ormmigration "github.com/domainry/domainry-orm/migration"
	_ "modernc.org/sqlite"
)

func TestMetadataMigrationOwnsDefinitionCatalog(t *testing.T) {
	migrations, err := SchemaMigrations("sqlite", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 1 || migrations[0].Version != 1 || migrations[0].Name != "metadata_catalog" {
		t.Fatalf("migrations=%#v", migrations)
	}
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, migration := range migrations {
		for _, statement := range migration.Statements {
			if _, err := database.ExecContext(t.Context(), statement); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, table := range OwnedTables() {
		var count int
		if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("table=%s count=%d err=%v", table, count, err)
		}
	}
	var metadataTableCount int
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name LIKE '_metadata_%'`).Scan(&metadataTableCount); err != nil || metadataTableCount != len(OwnedTables()) {
		t.Fatalf("metadata table count=%d err=%v", metadataTableCount, err)
	}
	for _, migration := range migrations {
		if checksum := ormmigration.Checksum(migration); checksum == "" {
			t.Fatalf("empty migration checksum for %s", migration.Name)
		}
	}
}

func TestLocalizedTextReadPushesWorkspaceAndNoInventedOwnerRangeIntoSQL(t *testing.T) {
	_, store := localizedTextTestStore(t)
	statement, args, err := store.localizedTextListStatement(metadatasdk.LocalizedTextQuery{WorkspaceID: "workspace-a", Locale: "en-US"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(statement, "workspace_id") || strings.Contains(statement, "owner_user_id") || strings.Contains(statement, "owner_org_id") || !reflect.DeepEqual(args, []any{"workspace-a", "en-US"}) {
		t.Fatalf("statement=%s args=%#v", statement, args)
	}
}

func TestLocalizedTextSourceReplacementCannotDeleteAnotherWorkspace(t *testing.T) {
	_, store := localizedTextTestStore(t)
	first := []metadatasdk.LocalizedText{
		{WorkspaceID: "workspace-a", EntityType: "object", EntityKey: "a", Property: "name", Locale: "en-US", Text: "A"},
		{WorkspaceID: "workspace-b", EntityType: "object", EntityKey: "b", Property: "name", Locale: "en-US", Text: "B"},
	}
	if err := store.syncLocalizedTextRows(t.Context(), store.database, "generated", "manifest", first, "2026-09-03T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	second := []metadatasdk.LocalizedText{{WorkspaceID: "workspace-a", EntityType: "object", EntityKey: "a2", Property: "name", Locale: "en-US", Text: "A2"}}
	if err := store.syncLocalizedTextRows(t.Context(), store.database, "generated", "manifest", second, "2026-09-03T00:00:01Z"); err != nil {
		t.Fatal(err)
	}
	workspaceA, err := store.ListLocalizedTexts(t.Context(), metadatasdk.LocalizedTextQuery{WorkspaceID: "workspace-a"})
	if err != nil || len(workspaceA) != 1 || workspaceA[0].EntityKey != "a2" {
		t.Fatalf("workspace-a values=%#v err=%v", workspaceA, err)
	}
	workspaceB, err := store.ListLocalizedTexts(t.Context(), metadatasdk.LocalizedTextQuery{WorkspaceID: "workspace-b"})
	if err != nil || len(workspaceB) != 1 || workspaceB[0].EntityKey != "b" {
		t.Fatalf("workspace-b values=%#v err=%v", workspaceB, err)
	}
}

func TestProjectionLocalizedTextBatchIsAtomic(t *testing.T) {
	database, store := localizedTextTestStore(t)
	err := store.SyncProjection(t.Context(), metadatasdk.ProjectionSnapshot{
		SchemaVersion: "1", SourceKind: "user", SourceID: "user-a",
		LocalizedText: []metadatasdk.LocalizedText{
			{WorkspaceID: "workspace-a", EntityType: "object", EntityKey: "valid", Property: "name", Locale: "en-US", Text: "Valid"},
			{WorkspaceID: "workspace-a", EntityType: "object", EntityKey: "invalid", Property: "name", Locale: "en-US"},
		},
	})
	if err == nil {
		t.Fatal("invalid localized-text batch was accepted")
	}
	var count int
	if queryErr := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM _metadata_localized_texts`).Scan(&count); queryErr != nil || count != 0 {
		t.Fatalf("localized rows=%d err=%v", count, queryErr)
	}
}

func localizedTextTestStore(t *testing.T) (*sql.DB, DefinitionStore) {
	t.Helper()
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	migrations, err := SchemaMigrations("sqlite", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations {
		for _, statement := range migration.Statements {
			if _, err := database.ExecContext(t.Context(), statement); err != nil {
				t.Fatal(err)
			}
		}
	}
	dialect, err := ormdialect.New(ormdialect.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	return database, NewDefinitionStore(database, dialect.WithSchema(""))
}

func TestDefinitionStoreSynchronizesAndReadsOwnedSnapshot(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	migrations, err := SchemaMigrations("sqlite", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations {
		for _, statement := range migration.Statements {
			if _, err := database.ExecContext(t.Context(), statement); err != nil {
				t.Fatal(err)
			}
		}
	}
	dialect, _ := ormdialect.New(ormdialect.SQLite)
	store := NewDefinitionStore(database, dialect.WithSchema(""))
	first := metadatasdk.ProjectionSnapshot{SchemaVersion: "1", SourceKind: "manifest", SourceID: "app", Definitions: []metadatasdk.Definition{
		{ResourceType: "object", ResourceKey: "customer", ObjectKey: "customer", Name: "Customer", Payload: json.RawMessage(`{"key":"customer"}`)},
		{ResourceType: "field", ResourceKey: "customer.name", ObjectKey: "customer", Name: "Name", Payload: json.RawMessage(`{"key":"name"}`)},
	}}
	if err := store.SyncProjection(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.SchemaVersion = "2"
	second.Definitions = second.Definitions[:1]
	if err := store.SyncProjection(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Definitions) != 1 || snapshot.Definitions[0].ResourceKey != "customer" || snapshot.Definitions[0].SchemaVersion != "2" {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	var fields int
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM _metadata_definitions WHERE resource_type = 'field'`).Scan(&fields); err != nil || fields != 0 {
		t.Fatalf("fields=%d err=%v", fields, err)
	}
	var versions int
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM _metadata_definition_versions`).Scan(&versions); err != nil || versions != 2 {
		t.Fatalf("versions=%d err=%v", versions, err)
	}
}

func TestProjectionParticipatesInHostTransaction(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	migrations, err := SchemaMigrations("sqlite", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations {
		for _, statement := range migration.Statements {
			if _, err := database.ExecContext(t.Context(), statement); err != nil {
				t.Fatal(err)
			}
		}
	}
	dialect, _ := ormdialect.New(ormdialect.SQLite)
	store := NewDefinitionStore(database, dialect.WithSchema(""))
	tx, err := database.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := modulehost.WithExecutor(t.Context(), tx)
	projection := metadatasdk.ProjectionSnapshot{
		SchemaVersion: "1", SourceKind: "generated", SourceID: "app", DefaultLocale: "en-US",
		Definitions:   []metadatasdk.Definition{{ResourceType: "object", ResourceKey: "customer", ObjectKey: "customer", Payload: json.RawMessage(`{"key":"customer"}`)}},
		LocalizedText: []metadatasdk.LocalizedText{{WorkspaceID: "workspace", EntityType: "object", EntityKey: "customer", Property: "name", Locale: "en-US", Text: "Customer"}},
	}
	if err := store.SyncProjection(ctx, projection); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(ctx)
	if err != nil || len(snapshot.Definitions) != 1 {
		_ = tx.Rollback()
		t.Fatalf("transaction snapshot=%#v err=%v", snapshot, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Snapshot(t.Context())
	if err != nil || len(snapshot.Definitions) != 0 {
		t.Fatalf("rolled-back snapshot=%#v err=%v", snapshot, err)
	}
	var localized int
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM _metadata_localized_texts`).Scan(&localized); err != nil || localized != 0 {
		t.Fatalf("rolled-back localized rows=%d err=%v", localized, err)
	}
}
