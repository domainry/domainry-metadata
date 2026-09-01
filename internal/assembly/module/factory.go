// Package moduleassembly composes the in-process Metadata module over the
// database, SQL dialect and migration registrar supplied by its host.
package moduleassembly

import (
	"context"
	"fmt"

	"github.com/domainry/domainry-foundation/modulehttp"
	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	metadatacapability "github.com/domainry/domainry-metadata/capability"
	metadatasdkadapter "github.com/domainry/domainry-metadata/internal/adapter/metadatasdk"
	metadataapplication "github.com/domainry/domainry-metadata/internal/application/metadata"
	metadatadomain "github.com/domainry/domainry-metadata/internal/domain/metadata/service"
	metadatastore "github.com/domainry/domainry-metadata/internal/infrastructure/persistence/database/metadata"
	modulehttptransport "github.com/domainry/domainry-metadata/internal/transport/http/module"
)

type Factory struct{}

func NewFactory() *Factory { return &Factory{} }

func OwnedTables() []string {
	return metadatastore.OwnedTables()
}

func (*Factory) OpenModule(ctx context.Context, application metadatasdk.ApplicationRef, host modulehost.Host) (metadatasdk.Binding, error) {
	if err := application.Validate(); err != nil {
		return nil, err
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
	definitions := metadataapplication.NewDefinitionApplicationService(store)
	localization := metadataapplication.NewLocalizationApplicationService(store, definitions)
	dictionaries := metadatadomain.NewDictionaryService(definitions, localization)
	capability, err := metadatacapability.Open(metadatacapability.Inputs{})
	if err != nil {
		return nil, fmt.Errorf("build Metadata capability disclosure: %w", err)
	}
	binding, err := metadatasdkadapter.NewBinding(definitions, localization, dictionaries, definitions, capability)
	if err != nil {
		return nil, err
	}
	surface, err := modulehttptransport.NewSurface(binding)
	if err != nil {
		return nil, err
	}
	binding.SetHTTPSurfaces([]modulehttp.Surface{surface})
	if err := binding.Descriptor().Validate(); err != nil {
		return nil, err
	}
	return binding, nil
}
