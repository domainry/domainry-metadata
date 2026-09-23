package metadatasdkadapter

import (
	"context"

	actioncontract "github.com/domainry/domainry-foundation/action"
	"github.com/domainry/domainry-foundation/modulehttp"
	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	metadataapplication "github.com/domainry/domainry-metadata/internal/application/metadata"
)

type Binding struct {
	definitionStore metadatasdk.DefinitionStore
	localization    metadatasdk.Localization
	dictionaries    metadatasdk.Dictionaries
	projection      metadatasdk.Projection
	adapters        []modulehttp.Adapter
}

func NewBinding(definitionStore metadatasdk.DefinitionStore, localization metadatasdk.Localization, dictionaries metadatasdk.Dictionaries, projection metadatasdk.Projection) (*Binding, error) {
	return &Binding{definitionStore: definitionStore, localization: localization, dictionaries: dictionaries, projection: projection}, nil
}

func (b *Binding) SetHTTPAdapters(adapters []modulehttp.Adapter) {
	b.adapters = append([]modulehttp.Adapter(nil), adapters...)
}

func (*Binding) Descriptor() metadatasdk.Descriptor {
	return metadatasdk.Descriptor{ProtocolVersion: metadatasdk.ProtocolVersionV1, Mode: metadatasdk.DeploymentModeModule, Capabilities: []string{"definitions", "definition_store", "localization", "dictionaries", "projection"}}
}

func (*Binding) Close(context.Context) error { return nil }

func (b *Binding) Definitions() metadatasdk.Definitions         { return b.definitionStore }
func (b *Binding) DefinitionStore() metadatasdk.DefinitionStore { return b.definitionStore }
func (b *Binding) Localization() metadatasdk.Localization       { return b.localization }
func (b *Binding) Dictionaries() metadatasdk.Dictionaries       { return b.dictionaries }
func (b *Binding) Projection() metadatasdk.Projection           { return b.projection }
func (b *Binding) LocalizationProjection() metadatasdk.LocalizationProjection {
	projection, _ := b.projection.(metadatasdk.LocalizationProjection)
	return projection
}
func (b *Binding) HTTPAdapters() []modulehttp.Adapter {
	return append([]modulehttp.Adapter(nil), b.adapters...)
}

func (*Binding) AuthorizationActions() ([]actioncontract.ActionDefinition, error) {
	return metadataapplication.AuthorizationActions()
}

var _ metadatasdk.Binding = (*Binding)(nil)
var _ metadatasdk.LocalizationProjectionBinding = (*Binding)(nil)
var _ modulehttp.Provider = (*Binding)(nil)
var _ actioncontract.Provider = (*Binding)(nil)
