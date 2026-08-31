package metadata

import (
	"database/sql"
	"encoding/json"
	"testing"

	metadatapersistence "github.com/domainry/domainry-metadata-sdk/persistence"
	ormdialect "github.com/domainry/domainry-orm/dialect"
	ormmigration "github.com/domainry/domainry-orm/migration"
	_ "modernc.org/sqlite"
)

func TestMetadataMigrationOwnsDefinitionCatalog(t *testing.T) {
	migrations, err := SchemaMigrations("sqlite", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 || migrations[0].Version != 1 || migrations[1].Version != 2 {
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
	var versionTableCount int
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='_metadata_definition_versions'`).Scan(&versionTableCount); err != nil || versionTableCount != 1 {
		t.Fatalf("version table count=%d err=%v", versionTableCount, err)
	}
	for _, table := range DefinitionTables() {
		var count int
		if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("table=%s count=%d err=%v", table, count, err)
		}
	}
	if checksum := ormmigration.Checksum(migrations[0]); checksum == "" {
		t.Fatal("empty migration checksum")
	}
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
	first := metadatapersistence.Snapshot{SchemaVersion: "1", SourceKind: "manifest", SourceID: "app", Definitions: []metadatapersistence.Definition{
		{ResourceType: "object", Key: "customer", ObjectKey: "customer", Name: "Customer", Payload: json.RawMessage(`{"key":"customer"}`)},
		{ResourceType: "field", Key: "customer.name", ObjectKey: "customer", Name: "Name", Payload: json.RawMessage(`{"key":"name"}`)},
	}}
	if err := store.SyncDefinitions(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.SchemaVersion = "2"
	second.Definitions = second.Definitions[:1]
	if err := store.SyncDefinitions(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.DefinitionSnapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Definitions) != 1 || snapshot.Definitions[0].Key != "customer" || snapshot.SchemaVersion != "2" {
		t.Fatalf("snapshot=%#v", snapshot)
	}
	var disabled int
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM _metadata_field_definitions WHERE disabled_at IS NOT NULL`).Scan(&disabled); err != nil || disabled != 1 {
		t.Fatalf("disabled=%d err=%v", disabled, err)
	}
}

func TestDefinitionStoreOwnsVersionHistory(t *testing.T) {
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
	version := metadatapersistence.DefinitionVersion{ResourceType: "object", ResourceKey: "customer", SchemaVersion: "1", SchemaHash: "abcdef1234567890", Payload: json.RawMessage(`{"key":"customer"}`), CreatedAt: "now"}
	if err := store.InsertDefinitionVersionWithExecutor(t.Context(), database, version); err != nil {
		t.Fatal(err)
	}
	count, err := store.CountDefinitionVersionsWithExecutor(t.Context(), database, "object", "customer")
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	loaded, found, err := store.GetDefinitionVersionWithExecutor(t.Context(), database, "object", "customer", "1")
	if err != nil || !found || loaded.SchemaHash != version.SchemaHash {
		t.Fatalf("loaded=%#v found=%t err=%v", loaded, found, err)
	}
	versions, err := store.ListDefinitionVersionsWithExecutor(t.Context(), database, "object", "customer")
	if err != nil || len(versions) != 1 {
		t.Fatalf("versions=%#v err=%v", versions, err)
	}
}
