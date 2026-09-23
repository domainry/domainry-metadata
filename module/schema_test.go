package module

import (
	"strings"
	"testing"
)

func TestModulePublishesCanonicalMetadataMigration(t *testing.T) {
	migrations, err := SchemaMigrations("sqlite", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 1 || len(migrations[0].Statements) != 1 {
		t.Fatalf("Metadata migrations=%#v", migrations)
	}
	for _, table := range OwnedTables() {
		if !strings.Contains(migrations[0].Statements[0], `CREATE TABLE IF NOT EXISTS "`+table+`"`) {
			t.Fatalf("canonical Metadata migration omits %s", table)
		}
	}
}
