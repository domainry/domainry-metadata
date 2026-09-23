// Package moduleassembly composes the in-process Metadata module over the
// database, SQL dialect and migration registrar supplied by its host.
package moduleassembly

import (
	"context"
	"fmt"

	"github.com/domainry/domainry-foundation/modulehttp"
	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	"github.com/domainry/domainry-metadata-sdk/modulehost"
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

// OpenDefinitionStore installs Metadata's canonical tables in the database
// selected by the caller and constructs a store local to that module. Module
// hosts share the physical tables by supplying the same database; standalone
// services get an independent copy by supplying their own database. The Store
// itself never crosses a module Host boundary.
func OpenDefinitionStore(ctx context.Context, application metadatasdk.ApplicationRef, host modulehost.Host) (metadatasdk.DefinitionStore, error) {
	return openDefinitionStore(ctx, application, host)
}

func openDefinitionStore(ctx context.Context, application metadatasdk.ApplicationRef, host modulehost.Host) (metadatastore.DefinitionStore, error) {
	if err := application.Validate(); err != nil {
		return metadatastore.DefinitionStore{}, err
	}
	if host == nil || host.Database() == nil || host.Dialect() == nil || host.Migrations() == nil {
		return metadatastore.DefinitionStore{}, fmt.Errorf("Metadata Module persistence host is incomplete")
	}
	migrations, err := metadatastore.SchemaMigrationsForDialect(host.Dialect(), host.Migrations().Driver())
	if err != nil {
		return metadatastore.DefinitionStore{}, err
	}
	if err := host.Migrations().ApplyOwnedMigrations(ctx, "metadata", migrations); err != nil {
		return metadatastore.DefinitionStore{}, fmt.Errorf("apply Metadata Module migrations: %w", err)
	}
	store := metadatastore.NewDefinitionStore(host.Database(), host.Dialect(), application.InstallationID)
	return store, nil
}

func (*Factory) OpenModule(ctx context.Context, application metadatasdk.ApplicationRef, host modulehost.Host) (metadatasdk.Binding, error) {
	store, err := openDefinitionStore(ctx, application, host)
	if err != nil {
		return nil, err
	}
	definitions := metadataapplication.NewDefinitionApplicationService(store)
	localization := metadataapplication.NewLocalizationApplicationService(store, definitions)
	dictionaries := metadatadomain.NewDictionaryService(definitions, localization)
	binding, err := metadatasdkadapter.NewBinding(definitions, localization, dictionaries, definitions)
	if err != nil {
		return nil, err
	}
	adapter, err := modulehttptransport.NewAdapter(binding)
	if err != nil {
		return nil, err
	}
	binding.SetHTTPAdapters([]modulehttp.Adapter{adapter})
	if err := binding.Descriptor().Validate(); err != nil {
		return nil, err
	}
	return binding, nil
}
