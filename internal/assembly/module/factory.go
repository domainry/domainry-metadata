// Package moduleassembly composes the in-process Metadata module over the
// database, SQL dialect and migration registrar supplied by its host.
package moduleassembly

import (
	"context"
	"fmt"
	"strings"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	metadatapersistence "github.com/domainry/domainry-metadata-sdk/persistence"
	metadatasdkadapter "github.com/domainry/domainry-metadata/internal/adapter/metadatasdk"
	metadatastore "github.com/domainry/domainry-metadata/internal/infrastructure/persistence/database/metadata"
)

type Database = modulehost.Database
type Dialect = modulehost.Dialect

type Factory struct{}

func NewFactory() *Factory { return &Factory{} }

func OwnedTables() []string {
	return append(metadatastore.DefinitionTables(), "_metadata_definition_versions")
}

func SchemaMigrationsForDialect(dialect Dialect) ([]modulehost.SchemaMigration, error) {
	return metadatastore.SchemaMigrationsForDialect(dialect)
}

func NewDefinitionRepository(database Database, dialect Dialect) metadatapersistence.DefinitionRepository {
	return metadatastore.NewDefinitionStore(database, dialect)
}

func (*Factory) OpenModule(ctx context.Context, application metadatasdk.ApplicationRef, host modulehost.Host) (metadatasdk.Binding, error) {
	if strings.TrimSpace(application.InstallationID) == "" {
		return nil, fmt.Errorf("Metadata installation identity is required")
	}
	if host == nil || host.Database() == nil || host.Dialect() == nil || host.Migrations() == nil {
		return nil, fmt.Errorf("Metadata Module persistence host is incomplete")
	}
	migrations, err := metadatastore.SchemaMigrationsForDialect(host.Dialect())
	if err != nil {
		return nil, err
	}
	if err := host.Migrations().ApplyOwnedMigrations(ctx, "metadata", migrations); err != nil {
		return nil, fmt.Errorf("apply Metadata Module migrations: %w", err)
	}
	store := metadatastore.NewDefinitionStore(host.Database(), host.Dialect())
	return metadatasdkadapter.NewBinding(store), nil
}
