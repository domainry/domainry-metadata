package metadata

import (
	"fmt"
	ormschema "github.com/domainry/domainry-orm/schema"

	"github.com/domainry/domainry-metadata-sdk/modulehost"
	ormdialect "github.com/domainry/domainry-orm/dialect"
)

var definitionTables = []string{
	"_metadata_object_definitions",
	"_metadata_field_definitions",
	"_metadata_validation_definitions",
	"_metadata_action_definitions",
	"_metadata_dictionary_definitions",
	"_metadata_role_definitions",
}

func SchemaMigrations(driver, schema string) ([]modulehost.SchemaMigration, error) {
	parsed, err := ormdialect.Parse(driver)
	if err != nil {
		return nil, fmt.Errorf("Metadata database driver %q is unsupported: %w", driver, err)
	}
	dialect, err := ormdialect.New(parsed.Name())
	if err != nil {
		return nil, err
	}
	return SchemaMigrationsForDialect(dialect.WithSchema(schema))
}

func SchemaMigrationsForDialect(renderer modulehost.Dialect) ([]modulehost.SchemaMigration, error) {
	statements := make([]string, 0, len(definitionTables))
	for _, table := range definitionTables {
		statement, _, buildErr := definitionTable(renderer, table).Build()
		if buildErr != nil {
			return nil, fmt.Errorf("build %s: %w", table, buildErr)
		}
		statements = append(statements, statement)
	}
	versionTable, _, err := ormschema.NewTable(renderer, "_metadata_definition_versions").IfNotExists().Columns(
		required("id", ormschema.TextKey(255)), required("resource_type", ormschema.TextKey(255)),
		required("resource_key", ormschema.TextKey(255)), required("schema_version", ormschema.TextKey(255)),
		required("schema_hash", ormschema.TextKey(255)), required("payload_json", ormschema.LongText()),
		required("created_at", ormschema.TextKey(255)),
	).PrimaryKey("id").Build()
	if err != nil {
		return nil, fmt.Errorf("build _metadata_definition_versions: %w", err)
	}
	return []modulehost.SchemaMigration{
		{Version: 1, Name: "metadata_definition_catalog", Statements: statements},
		{Version: 2, Name: "metadata_definition_versions", Statements: []string{versionTable}},
	}, nil
}

func definitionTable(renderer modulehost.Dialect, name string) *ormschema.TableBuilder {
	return ormschema.NewTable(renderer, name).IfNotExists().Columns(
		required("id", ormschema.TextKey(255)), required("resource_key", ormschema.TextKey(255)),
		required("object_key", ormschema.TextKey(255)), required("name", ormschema.Text()),
		required("payload_json", ormschema.LongText()), required("schema_version", ormschema.TextKey(255)),
		required("schema_hash", ormschema.TextKey(255)), required("source_kind", ormschema.TextKey(255)),
		required("source_id", ormschema.TextKey(255)), optional("disabled_at", ormschema.TextKey(255)),
		required("created_at", ormschema.TextKey(255)), required("updated_at", ormschema.TextKey(255)),
	).PrimaryKey("id").Unique("resource_key")
}

func required(name string, kind ormschema.ColumnType) ormschema.ColumnDefinition {
	return ormschema.Column(name, kind).NotNull()
}

func optional(name string, kind ormschema.ColumnType) ormschema.ColumnDefinition {
	return ormschema.Column(name, kind)
}

func DefinitionTables() []string { return append([]string(nil), definitionTables...) }
