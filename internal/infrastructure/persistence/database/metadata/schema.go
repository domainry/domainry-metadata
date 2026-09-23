package metadata

import (
	"fmt"

	"github.com/domainry/domainry-metadata/internal/infrastructure/persistence"

	"github.com/domainry/domainry-metadata-sdk/modulehost"
	ormdialect "github.com/domainry/domainry-orm/dialect"
	ormschema "github.com/domainry/domainry-orm/schema"
)

func SchemaMigrations(driver, schema string) ([]modulehost.SchemaMigration, error) {
	parsed, err := ormdialect.Parse(driver)
	if err != nil {
		return nil, fmt.Errorf("Metadata database driver %q is unsupported: %w", driver, err)
	}
	dialect, err := ormdialect.New(parsed.Name())
	if err != nil {
		return nil, err
	}
	return SchemaMigrationsForDialect(dialect.WithSchema(schema), driver)
}

// SchemaMigrationsForDialect owns only Metadata localization. The shared
// Definition tables are installed by foundation/definition under their own
// migration owner.
func SchemaMigrationsForDialect(renderer modulehost.Dialect, driver string) ([]modulehost.SchemaMigration, error) {
	localized, _, err := ormschema.NewTable(renderer, localizedTextTableName).IfNotExists().Columns(
		required("id", ormschema.TextKey(255)), required("workspace_id", ormschema.TextKey(255)),
		required("entity_type", ormschema.TextKey(255)), required("entity_key", ormschema.TextKey(255)),
		required("property", ormschema.TextKey(255)), required("locale", ormschema.TextKey(255)),
		required("text", ormschema.LongText()), required("source_kind", ormschema.TextKey(255)),
		required("source_id", ormschema.TextKey(255)), required("created_at", ormschema.TextKey(255)),
		required("updated_at", ormschema.TextKey(255)),
	).PrimaryKey("workspace_id", "id").Unique("workspace_id", "entity_type", "entity_key", "property", "locale").Build()
	if err != nil {
		return nil, fmt.Errorf("build %s: %w", localizedTextTableName, err)
	}
	localized, err = persistence.LocalizedTable(driver, renderer, localized)
	if err != nil {
		return nil, err
	}
	return []modulehost.SchemaMigration{{Version: 1, Name: "metadata_localization", Statements: []string{localized}}}, nil
}

func required(name string, kind ormschema.ColumnType) ormschema.ColumnDefinition {
	return ormschema.Column(name, kind).NotNull()
}

func OwnedTables() []string { return []string{localizedTextTableName} }
