package metadatasdkadapter

import (
	"context"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	metadatarepository "github.com/domainry/domainry-metadata-sdk/repository"
	metadataapplication "github.com/domainry/domainry-metadata/internal/application/metadata"
)

type Binding struct {
	definitions metadatarepository.DefinitionRepository
}

func NewBinding(definitions *metadataapplication.DefinitionApplicationService) *Binding {
	return &Binding{definitions: definitions}
}

func (*Binding) Descriptor() metadatasdk.Descriptor {
	return metadatasdk.Descriptor{ProtocolVersion: metadatasdk.ProtocolVersionV1, Mode: "module"}
}

func (*Binding) Close(context.Context) error { return nil }

func (b *Binding) DefinitionRepository() metadatarepository.DefinitionRepository {
	return b.definitions
}

var _ metadatasdk.Binding = (*Binding)(nil)
var _ metadatarepository.Binding = (*Binding)(nil)
