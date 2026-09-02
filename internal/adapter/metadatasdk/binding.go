package metadatasdkadapter

import (
	"context"
	"fmt"

	actioncontract "github.com/domainry/domainry-foundation/action"
	"github.com/domainry/domainry-foundation/modulecapability"
	"github.com/domainry/domainry-foundation/modulehttp"
	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	metadataapplication "github.com/domainry/domainry-metadata/internal/application/metadata"
)

type Binding struct {
	definitions  metadatasdk.Definitions
	localization metadatasdk.Localization
	dictionaries metadatasdk.Dictionaries
	projection   metadatasdk.Projection
	surfaces     []modulehttp.Surface
	capability   modulecapability.Binding
}

func NewBinding(definitions metadatasdk.Definitions, localization metadatasdk.Localization, dictionaries metadatasdk.Dictionaries, projection metadatasdk.Projection, capability modulecapability.Binding) (*Binding, error) {
	if capability == nil {
		return nil, fmt.Errorf("Metadata capability binding is required")
	}
	return &Binding{definitions: definitions, localization: localization, dictionaries: dictionaries, projection: projection, capability: capability}, nil
}

func (b *Binding) CapabilitySummary(ctx context.Context) (modulecapability.ModuleSummary, error) {
	return b.capability.CapabilitySummary(ctx)
}
func (b *Binding) CapabilityCategory(ctx context.Context, key string) (modulecapability.CategoryDocument, error) {
	return b.capability.CapabilityCategory(ctx, key)
}
func (b *Binding) ValidateCapabilityCandidate(ctx context.Context, request modulecapability.ValidationRequest) (modulecapability.ValidationResult, error) {
	return b.capability.ValidateCapabilityCandidate(ctx, request)
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

func (*Binding) AuthorizationActions() ([]actioncontract.ActionDefinition, error) {
	return metadataapplication.AuthorizationActions()
}

var _ metadatasdk.Binding = (*Binding)(nil)
var _ modulehttp.Provider = (*Binding)(nil)
var _ actioncontract.Provider = (*Binding)(nil)
