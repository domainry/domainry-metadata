// Package moduleassembly composes the in-process Metadata module over the
// database, SQL dialect and migration registrar supplied by its host.
package moduleassembly

import (
	"context"
	"fmt"
	"strings"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	metadatarepository "github.com/domainry/domainry-metadata-sdk/repository"
	metadatasdkadapter "github.com/domainry/domainry-metadata/internal/adapter/metadatasdk"
	metadataapplication "github.com/domainry/domainry-metadata/internal/application/metadata"
	metadatadomain "github.com/domainry/domainry-metadata/internal/domain/metadata/service"
	metadatapersistence "github.com/domainry/domainry-metadata/internal/infrastructure/persistence/database/metadata"
)

type Database = modulehost.Database
type Dialect = modulehost.Dialect

type Factory struct{}

func NewFactory() *Factory { return &Factory{} }

func OwnedTables() []string {
	return append(metadatapersistence.DefinitionTables(), "_metadata_definition_versions")
}

func SchemaMigrationsForDialect(dialect Dialect) ([]modulehost.SchemaMigration, error) {
	return metadatapersistence.SchemaMigrationsForDialect(dialect)
}

func NewDefinitionRepository(database Database, dialect Dialect) metadatarepository.DefinitionRepository {
	return metadatapersistence.NewDefinitionStore(database, dialect)
}

func (*Factory) OpenModule(ctx context.Context, application metadatasdk.ApplicationRef, host modulehost.Host) (metadatasdk.Binding, error) {
	if strings.TrimSpace(application.InstallationID) == "" {
		return nil, fmt.Errorf("Metadata installation identity is required")
	}
	if host == nil || host.Database() == nil || host.Dialect() == nil || host.Migrations() == nil {
		return nil, fmt.Errorf("Metadata Module persistence host is incomplete")
	}
	migrations, err := metadatapersistence.SchemaMigrationsForDialect(host.Dialect())
	if err != nil {
		return nil, err
	}
	if err := host.Migrations().ApplyOwnedMigrations(ctx, "metadata", migrations); err != nil {
		return nil, fmt.Errorf("apply Metadata Module migrations: %w", err)
	}
	store := metadatapersistence.NewDefinitionStore(host.Database(), host.Dialect())
	domain := metadatadomain.NewDefinitionService(store)
	applicationService := metadataapplication.NewDefinitionApplicationService(domain)
	return metadatasdkadapter.NewBinding(applicationService), nil
}
