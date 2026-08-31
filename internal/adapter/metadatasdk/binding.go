package metadatasdkadapter

import (
	"context"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	metadatapersistence "github.com/domainry/domainry-metadata-sdk/persistence"
)

type Binding struct {
	definitions metadatapersistence.DefinitionRepository
}

func NewBinding(definitions metadatapersistence.DefinitionRepository) *Binding {
	return &Binding{definitions: definitions}
}

func (*Binding) Descriptor() metadatasdk.Descriptor {
	return metadatasdk.Descriptor{ProtocolVersion: metadatasdk.ProtocolVersionV1, Mode: "module"}
}

func (*Binding) Close(context.Context) error { return nil }

func (b *Binding) DefinitionRepository() metadatapersistence.DefinitionRepository {
	return b.definitions
}

var _ metadatasdk.Binding = (*Binding)(nil)
var _ metadatapersistence.Binding = (*Binding)(nil)
