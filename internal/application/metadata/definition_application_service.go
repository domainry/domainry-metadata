package metadata

import (
	"context"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
)

type definitionRepository interface {
	metadatasdk.Definitions
	SyncProjection(context.Context, metadatasdk.ProjectionSnapshot) error
}

type DefinitionApplicationService struct{ repository definitionRepository }

func NewDefinitionApplicationService(repository definitionRepository) *DefinitionApplicationService {
	return &DefinitionApplicationService{repository: repository}
}

func (s *DefinitionApplicationService) List(ctx context.Context, query metadatasdk.DefinitionQuery) ([]metadatasdk.Definition, error) {
	return s.repository.List(ctx, query)
}

func (s *DefinitionApplicationService) Get(ctx context.Context, resourceType, resourceKey string) (metadatasdk.Definition, bool, error) {
	return s.repository.Get(ctx, resourceType, resourceKey)
}

func (s *DefinitionApplicationService) Snapshot(ctx context.Context) (metadatasdk.DefinitionSnapshot, error) {
	return s.repository.Snapshot(ctx)
}

func (s *DefinitionApplicationService) Sync(ctx context.Context, snapshot metadatasdk.ProjectionSnapshot) error {
	return s.repository.SyncProjection(ctx, snapshot)
}

var _ metadatasdk.Definitions = (*DefinitionApplicationService)(nil)
var _ metadatasdk.Projection = (*DefinitionApplicationService)(nil)
