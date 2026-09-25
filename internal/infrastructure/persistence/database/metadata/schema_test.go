package metadata

import (
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	shareddefinition "github.com/domainry/domainry-foundation/definition"
	"github.com/domainry/domainry-foundation/schemaownership"
	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	ormdialect "github.com/domainry/domainry-orm/dialect"
	ormmigration "github.com/domainry/domainry-orm/migration"
	_ "modernc.org/sqlite"
)

func TestSchemaOwnershipMatchesEveryFreshMetadataTableAndPrimaryKey(t *testing.T) {
	tables := SchemaOwnership()
	if err := schemaownership.ValidateAll(tables); err != nil {
		t.Fatal(err)
	}
	migrations, err := SchemaMigrations("sqlite", "")
	if err != nil {
		t.Fatal(err)
	}
	created := map[string]string{}
	for _, migration := range migrations {
		for _, statement := range migration.Statements {
			const prefix = `CREATE TABLE IF NOT EXISTS "`
			if !strings.HasPrefix(statement, prefix) {
				continue
			}
			name, _, found := strings.Cut(strings.TrimPrefix(statement, prefix), `"`)
			if !found || name == "" {
				t.Fatalf("invalid CREATE TABLE statement: %s", statement)
			}
			created[name] = statement
		}
	}
	if len(created) != len(tables) || !slices.Equal(OwnedTables(), schemaownership.Names(tables)) {
		t.Fatalf("fresh Metadata tables=%v ownership=%+v", created, tables)
	}
	for _, table := range tables {
		statement, found := created[table.Name]
		if !found {
			t.Fatalf("Metadata table %s has ownership but no canonical DDL", table.Name)
		}
		quoted := make([]string, len(table.PrimaryKey))
		for index, column := range table.PrimaryKey {
			quoted[index] = `"` + column + `"`
		}
		if primaryKey := "PRIMARY KEY (" + strings.Join(quoted, ", ") + ")"; !strings.Contains(statement, primaryKey) {
			t.Fatalf("Metadata table %s ownership primary key %v does not match DDL: %s", table.Name, table.PrimaryKey, statement)
		}
	}
}

func TestMetadataMigrationOwnsOnlyLocalization(t *testing.T) {
	migrations, err := SchemaMigrations("sqlite", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 1 || migrations[0].Version != 1 || migrations[0].Name != "metadata_localization" {
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
	for _, sharedTable := range shareddefinition.OwnedTables() {
		var count int
		if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, sharedTable).Scan(&count); err != nil || count != 0 {
			t.Fatalf("shared table must not be owned by Metadata migration: table=%s count=%d err=%v", sharedTable, count, err)
		}
	}
	for _, retired := range []string{"_metadata_projection", "_metadata_definitions", "_metadata_definition_versions"} {
		var count int
		if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, retired).Scan(&count); err != nil || count != 0 {
			t.Fatalf("retired Metadata table=%s count=%d err=%v", retired, count, err)
		}
	}
	var metadataTableCount int
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name LIKE '_metadata_%'`).Scan(&metadataTableCount); err != nil || metadataTableCount != 1 {
		t.Fatalf("metadata table count=%d err=%v", metadataTableCount, err)
	}
	for _, migration := range migrations {
		if checksum := ormmigration.Checksum(migration); checksum == "" {
			t.Fatalf("empty migration checksum for %s", migration.Name)
		}
	}
}

func TestMetadataLocalizationSchemaRendersForEverySupportedORMDialect(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres", "mysql"} {
		t.Run(driver, func(t *testing.T) {
			migrations, err := SchemaMigrations(driver, "metadata_scope")
			if err != nil {
				t.Fatal(err)
			}
			if len(migrations) != 1 || len(migrations[0].Statements) != 1 {
				t.Fatalf("migrations=%#v", migrations)
			}
			localizationDDL := migrations[0].Statements[0]
			for _, requiredFragment := range []string{"_metadata_localized_texts", "workspace_id", "entity_type", "entity_key", "property", "locale"} {
				if !strings.Contains(localizationDDL, requiredFragment) {
					t.Fatalf("%s localization DDL missing %q: %s", driver, requiredFragment, localizationDDL)
				}
			}
		})
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
	if err := store.syncLocalizedTextRows(t.Context(), store.database, "generated", "manifest", first, time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	second := []metadatasdk.LocalizedText{{WorkspaceID: "workspace-a", EntityType: "object", EntityKey: "a2", Property: "name", Locale: "en-US", Text: "A2"}}
	if err := store.syncLocalizedTextRows(t.Context(), store.database, "generated", "manifest", second, time.Date(2026, 9, 3, 0, 0, 1, 0, time.UTC).UnixMilli()); err != nil {
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
		Owner: metadatasdk.DefinitionOwnerMetadata, SchemaVersion: "1", SourceKind: "user", SourceID: "user-a",
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

func TestLocalizedTextResourceProjectionReplacesOnlyItsResource(t *testing.T) {
	_, store := localizedTextTestStore(t)
	for _, snapshot := range []metadatasdk.LocalizedTextResourceSnapshot{
		{WorkspaceID: "workspace-a", EntityType: "role", EntityKey: "admin", SourceKind: "metadata_definition", SourceID: "role-admin", Values: []metadatasdk.LocalizedText{{Property: "name", Locale: "en-US", Text: "Admin"}}},
		{WorkspaceID: "workspace-a", EntityType: "role", EntityKey: "viewer", SourceKind: "metadata_definition", SourceID: "role-viewer", Values: []metadatasdk.LocalizedText{{Property: "name", Locale: "en-US", Text: "Viewer"}}},
	} {
		if err := store.ReplaceResource(t.Context(), snapshot); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.ReplaceResource(t.Context(), metadatasdk.LocalizedTextResourceSnapshot{
		WorkspaceID: "workspace-a", EntityType: "role", EntityKey: "admin", SourceKind: "metadata_definition", SourceID: "role-admin-v2",
		Values: []metadatasdk.LocalizedText{{Property: "name", Locale: "zh-CN", Text: "管理员"}},
	}); err != nil {
		t.Fatal(err)
	}
	values, err := store.ListLocalizedTexts(t.Context(), metadatasdk.LocalizedTextQuery{WorkspaceID: "workspace-a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0].EntityKey != "admin" || values[0].Locale != "zh-CN" || values[0].Text != "管理员" || values[1].EntityKey != "viewer" || values[1].Text != "Viewer" {
		t.Fatalf("localized values=%#v", values)
	}
}

func localizedTextTestStore(t *testing.T) (*sql.DB, DefinitionStore) {
	t.Helper()
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	applyTestSchema(t, database)
	dialect, err := ormdialect.New(ormdialect.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	return database, NewDefinitionStore(database, dialect.WithSchema(""), "test-installation")
}

func TestDefinitionStoreSynchronizesAndReadsOwnedSnapshot(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	applyTestSchema(t, database)
	dialect, _ := ormdialect.New(ormdialect.SQLite)
	store := NewDefinitionStore(database, dialect.WithSchema(""), "test-installation")
	first := metadatasdk.ProjectionSnapshot{Owner: metadatasdk.DefinitionOwnerMetadata, SchemaVersion: "1", SourceKind: "manifest", SourceID: "app", Definitions: []metadatasdk.Definition{
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
	snapshot, err := store.Snapshot(t.Context(), metadatasdk.DefinitionQuery{Owner: metadatasdk.DefinitionOwnerMetadata})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Definitions) != 2 {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	customer, found, err := store.Get(t.Context(), metadatasdk.DefinitionOwnerMetadata, "object", "customer")
	if err != nil || !found || customer.SchemaVersion != "2" {
		t.Fatalf("customer=%#v found=%t err=%v", customer, found, err)
	}
	var fields int
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM _definitions WHERE kind = 'field'`).Scan(&fields); err != nil || fields != 0 {
		t.Fatalf("fields=%d err=%v", fields, err)
	}
	var versions int
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM _definition_versions`).Scan(&versions); err != nil || versions != 5 {
		t.Fatalf("versions=%d err=%v", versions, err)
	}
	for kind, want := range map[string]int{"application": 2, "object": 2, "field": 1} {
		var count int
		if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM _definition_versions WHERE kind = ?`, kind).Scan(&count); err != nil || count != want {
			t.Fatalf("%s versions=%d want=%d err=%v", kind, count, want, err)
		}
	}
}

func TestDefinitionStoreRequiresExplicitOwnerAndIsolatesOwnersAndInstallations(t *testing.T) {
	database, store := localizedTextTestStore(t)
	metadataProjection := metadatasdk.ProjectionSnapshot{
		Owner: metadatasdk.DefinitionOwnerMetadata, SchemaVersion: "1", SourceKind: "project_model", SourceID: "project",
		Definitions: []metadatasdk.Definition{{ResourceType: "object", ResourceKey: "customer", Payload: json.RawMessage(`{"key":"customer"}`)}},
	}
	identityProjection := metadatasdk.ProjectionSnapshot{
		Owner: metadatasdk.DefinitionOwnerIdentity, SchemaVersion: "1", SourceKind: "project_model", SourceID: "project",
		Definitions: []metadatasdk.Definition{{ResourceType: "identity_profile_binding", ResourceKey: "customer", Payload: json.RawMessage(`{"object_key":"customer"}`)}},
	}
	if err := store.SyncProjection(t.Context(), metadataProjection); err != nil {
		t.Fatal(err)
	}
	if err := store.SyncProjection(t.Context(), identityProjection); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(t.Context(), metadatasdk.DefinitionQuery{}); errorCode(err) != "metadata.definition_owner_required" {
		t.Fatalf("ownerless list error=%v", err)
	}
	metadataValues, err := store.List(t.Context(), metadatasdk.DefinitionQuery{Owner: metadatasdk.DefinitionOwnerMetadata})
	if err != nil || len(metadataValues) != 2 {
		t.Fatalf("metadata definitions=%#v err=%v", metadataValues, err)
	}
	identityValues, err := store.List(t.Context(), metadatasdk.DefinitionQuery{Owner: metadatasdk.DefinitionOwnerIdentity})
	if err != nil || len(identityValues) != 1 || identityValues[0].ResourceType != "identity_profile_binding" {
		t.Fatalf("identity definitions=%#v err=%v", identityValues, err)
	}
	allValues, err := store.List(t.Context(), metadatasdk.DefinitionQuery{CrossOwner: true})
	if err != nil || len(allValues) != 3 {
		t.Fatalf("cross-owner definitions=%#v err=%v", allValues, err)
	}
	otherInstallation := NewDefinitionStore(database, store.dialect, "other-installation")
	if err := otherInstallation.SyncProjection(t.Context(), metadataProjection); err != nil {
		t.Fatal(err)
	}
	otherValues, err := otherInstallation.List(t.Context(), metadatasdk.DefinitionQuery{Owner: metadatasdk.DefinitionOwnerMetadata})
	if err != nil || len(otherValues) != 2 {
		t.Fatalf("other installation definitions=%#v err=%v", otherValues, err)
	}
	var currentRows int
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM _definitions`).Scan(&currentRows); err != nil || currentRows != 5 {
		t.Fatalf("definition rows=%d err=%v", currentRows, err)
	}
}

func TestDefinitionStoreRejectsUnregisteredOwnerKindAndCrossOwnerLocalizedText(t *testing.T) {
	_, store := localizedTextTestStore(t)
	for name, projection := range map[string]metadatasdk.ProjectionSnapshot{
		"owner": {Owner: "unknown", SchemaVersion: "1", SourceKind: "test", SourceID: "test"},
		"kind": {
			Owner: metadatasdk.DefinitionOwnerMetadata, SchemaVersion: "1", SourceKind: "test", SourceID: "test",
			Definitions: []metadatasdk.Definition{{ResourceType: "unknown", ResourceKey: "unknown", Payload: json.RawMessage(`{}`)}},
		},
		"retired_report_auxiliary_kind": {
			Owner: metadatasdk.DefinitionOwnerReport, SchemaVersion: "1", SourceKind: "test", SourceID: "test",
			Definitions: []metadatasdk.Definition{{ResourceType: "report_sensitive_field_policy", ResourceKey: "summary:secret", Payload: json.RawMessage(`{}`)}},
		},
		"localized_text_owner": {
			Owner: metadatasdk.DefinitionOwnerIdentity, SchemaVersion: "1", SourceKind: "test", SourceID: "test",
			LocalizedText: []metadatasdk.LocalizedText{{WorkspaceID: "workspace", EntityType: "role", EntityKey: "admin", Property: "name", Locale: "en-US", Text: "Admin"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := store.SyncProjection(t.Context(), projection); err == nil {
				t.Fatal("invalid projection was accepted")
			}
		})
	}
}

func errorCode(err error) string {
	var metadataError *metadatasdk.Error
	if errors.As(err, &metadataError) {
		return metadataError.Code
	}
	return ""
}

func TestProjectionParticipatesInHostTransaction(t *testing.T) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	applyTestSchema(t, database)
	dialect, _ := ormdialect.New(ormdialect.SQLite)
	store := NewDefinitionStore(database, dialect.WithSchema(""), "test-installation")
	tx, err := database.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := modulehost.WithExecutor(t.Context(), tx)
	projection := metadatasdk.ProjectionSnapshot{
		Owner: metadatasdk.DefinitionOwnerMetadata, SchemaVersion: "1", SourceKind: "generated", SourceID: "app", DefaultLocale: "en-US",
		Definitions:   []metadatasdk.Definition{{ResourceType: "object", ResourceKey: "customer", ObjectKey: "customer", Payload: json.RawMessage(`{"key":"customer"}`)}},
		LocalizedText: []metadatasdk.LocalizedText{{WorkspaceID: "workspace", EntityType: "object", EntityKey: "customer", Property: "name", Locale: "en-US", Text: "Customer"}},
	}
	if err := store.SyncProjection(ctx, projection); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	snapshot, err := store.Snapshot(ctx, metadatasdk.DefinitionQuery{Owner: metadatasdk.DefinitionOwnerMetadata})
	if err != nil || len(snapshot.Definitions) != 2 {
		_ = tx.Rollback()
		t.Fatalf("transaction snapshot=%#v err=%v", snapshot, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	snapshot, err = store.Snapshot(t.Context(), metadatasdk.DefinitionQuery{Owner: metadatasdk.DefinitionOwnerMetadata})
	if err != nil || len(snapshot.Definitions) != 0 {
		t.Fatalf("rolled-back snapshot=%#v err=%v", snapshot, err)
	}
	var localized int
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM _metadata_localized_texts`).Scan(&localized); err != nil || localized != 0 {
		t.Fatalf("rolled-back localized rows=%d err=%v", localized, err)
	}
}

func applyTestSchema(t *testing.T, database *sql.DB) {
	t.Helper()
	shared, err := shareddefinition.SchemaMigrations("sqlite", "")
	if err != nil {
		t.Fatal(err)
	}
	local, err := SchemaMigrations("sqlite", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range append(shared, local...) {
		for _, statement := range migration.Statements {
			if _, err := database.ExecContext(t.Context(), statement); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestDefinitionPublicationAndDisableParticipateInHostTransaction(t *testing.T) {
	database, store := localizedTextTestStore(t)
	tx, err := database.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	transactionContext := modulehost.WithExecutor(t.Context(), tx)
	created, err := store.Publish(transactionContext, metadatasdk.DefinitionPublishCommand{
		Owner: metadatasdk.DefinitionOwnerWorkflow, ResourceType: "workflow", ResourceKey: "approval",
		ExpectedCurrentVersionID: metadatasdk.DefinitionNoCurrentVersion, SchemaVersion: "1",
		Payload: json.RawMessage(`{"key":"approval"}`), SourceKind: "workflow_registry", SourceID: "workflows", PublishedBy: "system:test",
	})
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if value, found, err := store.Get(transactionContext, metadatasdk.DefinitionOwnerWorkflow, "workflow", "approval"); err != nil || !found || value.CurrentVersionID != created.CurrentVersionID {
		_ = tx.Rollback()
		t.Fatalf("transaction definition=%#v found=%t err=%v", value, found, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if value, found, err := store.Get(t.Context(), metadatasdk.DefinitionOwnerWorkflow, "workflow", "approval"); err != nil || found {
		t.Fatalf("rolled-back definition=%#v found=%t err=%v", value, found, err)
	}
	created, err = store.Publish(t.Context(), metadatasdk.DefinitionPublishCommand{
		Owner: metadatasdk.DefinitionOwnerWorkflow, ResourceType: "workflow", ResourceKey: "approval",
		ExpectedCurrentVersionID: metadatasdk.DefinitionNoCurrentVersion, SchemaVersion: "1",
		Payload: json.RawMessage(`{"key":"approval"}`), SourceKind: "workflow_registry", SourceID: "workflows", PublishedBy: "system:test",
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, err = database.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	transactionContext = modulehost.WithExecutor(t.Context(), tx)
	if err := store.Disable(transactionContext, metadatasdk.DefinitionDisableCommand{
		Owner: metadatasdk.DefinitionOwnerWorkflow, ResourceType: "workflow", ResourceKey: "approval",
		ExpectedCurrentVersionID: created.CurrentVersionID, DisabledBy: "system:test",
	}); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if _, found, err := store.Get(transactionContext, metadatasdk.DefinitionOwnerWorkflow, "workflow", "approval"); err != nil || found {
		_ = tx.Rollback()
		t.Fatalf("disabled transaction found=%t err=%v", found, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if value, found, err := store.Get(t.Context(), metadatasdk.DefinitionOwnerWorkflow, "workflow", "approval"); err != nil || !found || value.CurrentVersionID != created.CurrentVersionID {
		t.Fatalf("rolled-back disable definition=%#v found=%t err=%v", value, found, err)
	}
}

func TestNotificationDeliveryPolicyDefinitionIsRegisteredAndCASVersioned(t *testing.T) {
	_, store := localizedTextTestStore(t)
	created, err := store.Publish(t.Context(), metadatasdk.DefinitionPublishCommand{
		Owner: metadatasdk.DefinitionOwnerNotification, ResourceType: "delivery_policy", ResourceKey: "workspace:workspace-1",
		ExpectedCurrentVersionID: metadatasdk.DefinitionNoCurrentVersion,
		SchemaVersion:            "domainry-notification-delivery-policy-v1:enabled", Payload: json.RawMessage(`{"enabled":true}`),
		SourceKind: "notification_policy", SourceID: "workspace-1", PublishedBy: "user-1",
	})
	if err != nil || created.CurrentVersionID == "" {
		t.Fatalf("create Notification policy=%#v err=%v", created, err)
	}
	updated, err := store.Publish(t.Context(), metadatasdk.DefinitionPublishCommand{
		Owner: metadatasdk.DefinitionOwnerNotification, ResourceType: "delivery_policy", ResourceKey: "workspace:workspace-1",
		ExpectedCurrentVersionID: created.CurrentVersionID,
		SchemaVersion:            "domainry-notification-delivery-policy-v1:disabled", Payload: json.RawMessage(`{"enabled":false}`),
		SourceKind: "notification_policy", SourceID: "workspace-1", PublishedBy: "user-2",
	})
	if err != nil || updated.CurrentVersionID == "" || updated.CurrentVersionID == created.CurrentVersionID {
		t.Fatalf("update Notification policy=%#v err=%v", updated, err)
	}
	if _, err := store.Publish(t.Context(), metadatasdk.DefinitionPublishCommand{
		Owner: metadatasdk.DefinitionOwnerNotification, ResourceType: "delivery_policy", ResourceKey: "workspace:workspace-1",
		ExpectedCurrentVersionID: created.CurrentVersionID,
		SchemaVersion:            "domainry-notification-delivery-policy-v1:stale", Payload: json.RawMessage(`{"enabled":true}`),
		SourceKind: "notification_policy", SourceID: "workspace-1", PublishedBy: "stale-user",
	}); err == nil {
		t.Fatal("stale Notification policy revision was accepted")
	}
}

func TestNotificationTemplateDefinitionKindsAreRegistered(t *testing.T) {
	_, store := localizedTextTestStore(t)
	for _, resourceType := range []string{"notification_template", "notification_template_version"} {
		t.Run(resourceType, func(t *testing.T) {
			created, err := store.Publish(t.Context(), metadatasdk.DefinitionPublishCommand{
				Owner: metadatasdk.DefinitionOwnerNotification, ResourceType: resourceType, ResourceKey: "workspace:workspace-1:template:welcome",
				ExpectedCurrentVersionID: metadatasdk.DefinitionNoCurrentVersion,
				SchemaVersion:            "domainry-notification-" + resourceType + "-v1:initial", Payload: json.RawMessage(`{"key":"welcome"}`),
				SourceKind: "notification_template", SourceID: "workspace-1", PublishedBy: "user-1",
			})
			if err != nil || created.CurrentVersionID == "" {
				t.Fatalf("create %s=%#v err=%v", resourceType, created, err)
			}
		})
	}
}

func TestLifecycleRetentionPolicyDefinitionIsRegisteredAndVersioned(t *testing.T) {
	database, store := localizedTextTestStore(t)
	created, err := store.Publish(t.Context(), metadatasdk.DefinitionPublishCommand{
		Owner: metadatasdk.DefinitionOwnerLifecycle, ResourceType: "retention_policy", ResourceKey: "workspace:abc:retention-policy:def",
		ExpectedCurrentVersionID: metadatasdk.DefinitionNoCurrentVersion,
		SchemaVersion:            "domainry-lifecycle-retention-policy-v1:revision:1", Payload: json.RawMessage(`{"revision":1}`),
		SourceKind: "lifecycle_retention_policy", SourceID: "workspace-1", PublishedBy: "user-1",
	})
	if err != nil || created.CurrentVersionID == "" {
		t.Fatalf("create Lifecycle retention policy=%#v err=%v", created, err)
	}
	updated, err := store.Publish(t.Context(), metadatasdk.DefinitionPublishCommand{
		Owner: metadatasdk.DefinitionOwnerLifecycle, ResourceType: "retention_policy", ResourceKey: "workspace:abc:retention-policy:def",
		ExpectedCurrentVersionID: created.CurrentVersionID,
		SchemaVersion:            "domainry-lifecycle-retention-policy-v1:revision:2", Payload: json.RawMessage(`{"revision":2}`),
		SourceKind: "lifecycle_retention_policy", SourceID: "workspace-1", PublishedBy: "user-2",
	})
	if err != nil || updated.CurrentVersionID == "" || updated.CurrentVersionID == created.CurrentVersionID {
		t.Fatalf("update Lifecycle retention policy=%#v err=%v", updated, err)
	}
	var history int
	if err := database.QueryRow(`SELECT COUNT(*) FROM _definition_versions WHERE owner = ? AND kind = ? AND definition_key = ?`, metadatasdk.DefinitionOwnerLifecycle, "retention_policy", "workspace:abc:retention-policy:def").Scan(&history); err != nil || history != 2 {
		t.Fatalf("Lifecycle retention policy history=%d err=%v", history, err)
	}
	if _, err := store.Publish(t.Context(), metadatasdk.DefinitionPublishCommand{
		Owner: metadatasdk.DefinitionOwnerLifecycle, ResourceType: "retention_policy", ResourceKey: "workspace:abc:retention-policy:def",
		ExpectedCurrentVersionID: created.CurrentVersionID,
		SchemaVersion:            "domainry-lifecycle-retention-policy-v1:revision:3", Payload: json.RawMessage(`{"revision":3}`),
		SourceKind: "lifecycle_retention_policy", SourceID: "workspace-1", PublishedBy: "stale-user",
	}); err == nil {
		t.Fatal("stale Lifecycle retention policy revision was accepted")
	}
}
