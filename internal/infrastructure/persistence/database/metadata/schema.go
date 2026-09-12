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

func SchemaMigrationsForDialect(renderer modulehost.Dialect, driver string) ([]modulehost.SchemaMigration, error) {
	catalog, _, err := ormschema.NewTable(renderer, definitionTableName).IfNotExists().Columns(
		required("id", ormschema.TextKey(255)), required("resource_type", ormschema.TextKey(255)),
		required("resource_key", ormschema.TextKey(255)), required("object_key", ormschema.TextKey(255)),
		required("name", ormschema.Text()), required("payload_json", ormschema.LongText()),
		required("schema_version", ormschema.TextKey(255)), required("schema_hash", ormschema.TextKey(255)),
		required("source_kind", ormschema.TextKey(255)), required("source_id", ormschema.TextKey(255)),
		optional("disabled_at", ormschema.TextKey(255)), required("created_at", ormschema.TextKey(255)),
		required("updated_at", ormschema.TextKey(255)),
	).PrimaryKey("id").Unique("resource_type", "resource_key").Build()
	if err != nil {
		return nil, fmt.Errorf("build %s: %w", definitionTableName, err)
	}
	versions, _, err := ormschema.NewTable(renderer, "_metadata_definition_versions").IfNotExists().Columns(
		required("id", ormschema.TextKey(255)), required("resource_type", ormschema.TextKey(255)),
		required("resource_key", ormschema.TextKey(255)), required("schema_version", ormschema.TextKey(255)),
		required("schema_hash", ormschema.TextKey(255)), required("payload_json", ormschema.LongText()),
		required("created_at", ormschema.TextKey(255)),
	).PrimaryKey("id").Build()
	if err != nil {
		return nil, fmt.Errorf("build _metadata_definition_versions: %w", err)
	}
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
	projection, _, err := ormschema.NewTable(renderer, "_metadata_projection").IfNotExists().Columns(
		required("id", ormschema.TextKey(255)), required("schema_version", ormschema.TextKey(255)),
		required("source_kind", ormschema.TextKey(255)), required("source_id", ormschema.TextKey(255)),
		required("name", ormschema.Text()), required("default_locale", ormschema.TextKey(255)),
		required("updated_at", ormschema.TextKey(255)),
	).PrimaryKey("id").Build()
	if err != nil {
		return nil, fmt.Errorf("build _metadata_projection: %w", err)
	}
	return []modulehost.SchemaMigration{{
		Version:    1,
		Name:       "metadata_catalog",
		Statements: []string{catalog, versions, localized, projection},
	}}, nil
}

func required(name string, kind ormschema.ColumnType) ormschema.ColumnDefinition {
	return ormschema.Column(name, kind).NotNull()
}

func optional(name string, kind ormschema.ColumnType) ormschema.ColumnDefinition {
	return ormschema.Column(name, kind)
}

func OwnedTables() []string {
	return []string{definitionTableName, "_metadata_definition_versions", localizedTextTableName, "_metadata_projection"}
}
