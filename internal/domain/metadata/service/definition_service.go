package service

import (
	"context"

	metadatamodel "github.com/domainry/domainry-metadata/internal/domain/metadata/model"
	metadatarepository "github.com/domainry/domainry-metadata/internal/domain/metadata/repository"
)

type DefinitionService struct {
	definitions metadatarepository.DefinitionRepository
}

func NewDefinitionService(definitions metadatarepository.DefinitionRepository) *DefinitionService {
	return &DefinitionService{definitions: definitions}
}

func (s *DefinitionService) Sync(ctx context.Context, snapshot metadatamodel.Snapshot) error {
	return s.definitions.SyncDefinitions(ctx, snapshot)
}

func (s *DefinitionService) Snapshot(ctx context.Context) (metadatamodel.Snapshot, error) {
	return s.definitions.DefinitionSnapshot(ctx)
}
