package mysql

import (
	"strings"
	"testing"

	"github.com/domainry/domainry-orm/dialect"
	"github.com/domainry/domainry-orm/schema"
)

func TestLocalizedGeneratedKeyPrecedesAuditFields(t *testing.T) {
	renderer, err := dialect.ParseRenderer("mysql", "", "")
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{"workspace_id", "entity_type", "entity_key", "property", "locale"}
	columns := []schema.ColumnDefinition{schema.Column("id", schema.BigInt()).NotNull().AutoIncrement()}
	for _, key := range keys {
		columns = append(columns, schema.Column(key, schema.TextKey(255)).NotNull())
	}
	columns = append(columns, schema.Column("created_at", schema.BigInt()).NotNull(), schema.Column("updated_at", schema.BigInt()).NotNull())
	statement, _, err := schema.NewTable(renderer, "localized").Columns(columns...).PrimaryKey("id").Unique(keys...).Build()
	if err != nil {
		t.Fatal(err)
	}
	statement = LocalizedTable(renderer, statement)
	generated, audit := strings.Index(statement, "`natural_key_sha256` BINARY(32)"), strings.Index(statement, "`created_at` BIGINT")
	if generated < 0 || generated >= audit || !strings.Contains(statement, "UNIQUE (`natural_key_sha256`)") {
		t.Fatalf("invalid generated key order: %s", statement)
	}
}
