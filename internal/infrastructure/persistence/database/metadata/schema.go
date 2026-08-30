package metadata

import (
	"fmt"

	"github.com/domainry/domainry-metadata-sdk/modulehost"
	ormbuilder "github.com/domainry/domainry-orm/builder"
	ormdialect "github.com/domainry/domainry-orm/dialect"
)

var definitionTables = []string{
	"object_definitions",
	"field_definitions",
	"validation_definitions",
	"action_definitions",
	"dictionary_definitions",
	"role_definitions",
	"identity_profile_binding_definitions",
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
	versionTable, _, err := ormbuilder.NewCreateTableBuilder(renderer, "metadata_definition_versions").WithoutSystemColumns().IfNotExists().Columns(
		required("id", ormbuilder.TextKeyType(255)), required("resource_type", ormbuilder.TextKeyType(255)),
		required("resource_key", ormbuilder.TextKeyType(255)), required("schema_version", ormbuilder.TextKeyType(255)),
		required("schema_hash", ormbuilder.TextKeyType(255)), required("payload_json", ormbuilder.LongTextType()),
		required("created_at", ormbuilder.TextKeyType(255)),
	).PrimaryKey("id").Build()
	if err != nil {
		return nil, fmt.Errorf("build metadata_definition_versions: %w", err)
	}
	return []modulehost.SchemaMigration{
		{Version: 1, Name: "metadata_definition_catalog", Statements: statements},
		{Version: 2, Name: "metadata_definition_versions", Statements: []string{versionTable}},
	}, nil
}

func definitionTable(renderer modulehost.Dialect, name string) *ormbuilder.CreateTableBuilder {
	return ormbuilder.NewCreateTableBuilder(renderer, name).WithoutSystemColumns().IfNotExists().Columns(
		required("id", ormbuilder.TextKeyType(255)), required("resource_key", ormbuilder.TextKeyType(255)),
		required("object_key", ormbuilder.TextKeyType(255)), required("name", ormbuilder.TextType()),
		required("payload_json", ormbuilder.LongTextType()), required("schema_version", ormbuilder.TextKeyType(255)),
		required("schema_hash", ormbuilder.TextKeyType(255)), required("source_kind", ormbuilder.TextKeyType(255)),
		required("source_id", ormbuilder.TextKeyType(255)), optional("disabled_at", ormbuilder.TextKeyType(255)),
		required("created_at", ormbuilder.TextKeyType(255)), required("updated_at", ormbuilder.TextKeyType(255)),
	).PrimaryKey("id").Unique("resource_key")
}

func required(name string, kind ormbuilder.ColumnType) ormbuilder.SchemaColumn {
	return ormbuilder.DefineColumn(name, kind).NotNull()
}

func optional(name string, kind ormbuilder.ColumnType) ormbuilder.SchemaColumn {
	return ormbuilder.DefineColumn(name, kind)
}

func DefinitionTables() []string { return append([]string(nil), definitionTables...) }
