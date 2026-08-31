package metadatasdkadapter

import (
	"context"

	"github.com/domainry/domainry-foundation/modulehttp"
	metadatasdk "github.com/domainry/domainry-metadata-sdk"
)

type Binding struct {
	definitions  metadatasdk.Definitions
	localization metadatasdk.Localization
	dictionaries metadatasdk.Dictionaries
	projection   metadatasdk.Projection
	surfaces     []modulehttp.Surface
}

func NewBinding(definitions metadatasdk.Definitions, localization metadatasdk.Localization, dictionaries metadatasdk.Dictionaries, projection metadatasdk.Projection) *Binding {
	return &Binding{definitions: definitions, localization: localization, dictionaries: dictionaries, projection: projection}
}

func (b *Binding) SetHTTPSurfaces(surfaces []modulehttp.Surface) {
	b.surfaces = append([]modulehttp.Surface(nil), surfaces...)
}

func (*Binding) Descriptor() metadatasdk.Descriptor {
	return metadatasdk.Descriptor{ProtocolVersion: metadatasdk.ProtocolVersionV1, Mode: metadatasdk.DeploymentModeModule, Capabilities: []string{"definitions", "localization", "dictionaries", "projection"}}
}

func (*Binding) Close(context.Context) error { return nil }

func (b *Binding) Definitions() metadatasdk.Definitions   { return b.definitions }
func (b *Binding) Localization() metadatasdk.Localization { return b.localization }
func (b *Binding) Dictionaries() metadatasdk.Dictionaries { return b.dictionaries }
func (b *Binding) Projection() metadatasdk.Projection     { return b.projection }
func (b *Binding) HTTPSurfaces() []modulehttp.Surface {
	return append([]modulehttp.Surface(nil), b.surfaces...)
}

var _ metadatasdk.Binding = (*Binding)(nil)
var _ modulehttp.Provider = (*Binding)(nil)
