package module

import (
	"context"
	"fmt"
	"strings"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	metadatarepository "github.com/domainry/domainry-metadata-sdk/repository"
	metadatapersistence "github.com/domainry/domainry-metadata/internal/infrastructure/persistence/database/metadata"
)

type Factory struct{}

func OwnedTables() []string {
	return append(metadatapersistence.DefinitionTables(), "_metadata_definition_versions")
}

func SchemaMigrationsForDialect(dialect modulehost.Dialect) ([]modulehost.SchemaMigration, error) {
	return metadatapersistence.SchemaMigrationsForDialect(dialect)
}

func NewDefinitionRepository(database modulehost.Database, dialect modulehost.Dialect) metadatarepository.DefinitionRepository {
	return metadatapersistence.NewDefinitionStore(database, dialect)
}

func NewFactory() *Factory { return &Factory{} }

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
	return binding{definitions: metadatapersistence.NewDefinitionStore(host.Database(), host.Dialect())}, nil
}

type binding struct {
	definitions metadatarepository.DefinitionRepository
}

func (binding) Descriptor() metadatasdk.Descriptor {
	return metadatasdk.Descriptor{ProtocolVersion: metadatasdk.ProtocolVersionV1, Mode: "module"}
}
func (binding) Close(context.Context) error                                     { return nil }
func (b binding) DefinitionRepository() metadatarepository.DefinitionRepository { return b.definitions }

var _ metadatarepository.Binding = binding{}
