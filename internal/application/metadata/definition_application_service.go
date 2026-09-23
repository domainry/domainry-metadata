package metadata

import (
	"context"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
)

type definitionRepository interface {
	metadatasdk.DefinitionStore
	SyncProjection(context.Context, metadatasdk.ProjectionSnapshot) error
	ReplaceResource(context.Context, metadatasdk.LocalizedTextResourceSnapshot) error
}

type DefinitionApplicationService struct{ repository definitionRepository }

func NewDefinitionApplicationService(repository definitionRepository) *DefinitionApplicationService {
	return &DefinitionApplicationService{repository: repository}
}

func (s *DefinitionApplicationService) List(ctx context.Context, query metadatasdk.DefinitionQuery) ([]metadatasdk.Definition, error) {
	return s.repository.List(ctx, query)
}

func (s *DefinitionApplicationService) Get(ctx context.Context, owner, resourceType, resourceKey string) (metadatasdk.Definition, bool, error) {
	return s.repository.Get(ctx, owner, resourceType, resourceKey)
}

func (s *DefinitionApplicationService) Snapshot(ctx context.Context, query metadatasdk.DefinitionQuery) (metadatasdk.DefinitionSnapshot, error) {
	return s.repository.Snapshot(ctx, query)
}

func (s *DefinitionApplicationService) Sync(ctx context.Context, snapshot metadatasdk.ProjectionSnapshot) error {
	return s.repository.SyncProjection(ctx, snapshot)
}

func (s *DefinitionApplicationService) ReplaceSourceSnapshot(ctx context.Context, snapshot metadatasdk.ProjectionSnapshot) error {
	return s.repository.ReplaceSourceSnapshot(ctx, snapshot)
}

func (s *DefinitionApplicationService) Publish(ctx context.Context, command metadatasdk.DefinitionPublishCommand) (metadatasdk.DefinitionPublishResult, error) {
	return s.repository.Publish(ctx, command)
}

func (s *DefinitionApplicationService) Disable(ctx context.Context, command metadatasdk.DefinitionDisableCommand) error {
	return s.repository.Disable(ctx, command)
}

func (s *DefinitionApplicationService) GetVersion(ctx context.Context, query metadatasdk.DefinitionVersionQuery) (metadatasdk.DefinitionVersion, bool, error) {
	return s.repository.GetVersion(ctx, query)
}

func (s *DefinitionApplicationService) ReplaceResource(ctx context.Context, snapshot metadatasdk.LocalizedTextResourceSnapshot) error {
	return s.repository.ReplaceResource(ctx, snapshot)
}

var _ metadatasdk.Definitions = (*DefinitionApplicationService)(nil)
var _ metadatasdk.DefinitionStore = (*DefinitionApplicationService)(nil)
var _ metadatasdk.Projection = (*DefinitionApplicationService)(nil)
var _ metadatasdk.LocalizationProjection = (*DefinitionApplicationService)(nil)
