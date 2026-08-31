package metadata

import (
	"context"

	metadatamodel "github.com/domainry/domainry-metadata/internal/domain/metadata/model"
	metadataservice "github.com/domainry/domainry-metadata/internal/domain/metadata/service"
)

type DefinitionApplicationService struct {
	domain *metadataservice.DefinitionService
}

func NewDefinitionApplicationService(domain *metadataservice.DefinitionService) *DefinitionApplicationService {
	return &DefinitionApplicationService{domain: domain}
}

func (s *DefinitionApplicationService) SyncDefinitions(ctx context.Context, snapshot metadatamodel.Snapshot) error {
	return s.domain.Sync(ctx, snapshot)
}

func (s *DefinitionApplicationService) DefinitionSnapshot(ctx context.Context) (metadatamodel.Snapshot, error) {
	return s.domain.Snapshot(ctx)
}
