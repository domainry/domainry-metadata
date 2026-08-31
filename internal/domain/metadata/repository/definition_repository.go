package repository

import (
	"context"

	metadatamodel "github.com/domainry/domainry-metadata/internal/domain/metadata/model"
)

// DefinitionRepository is the domain-facing persistence port.
type DefinitionRepository interface {
	SyncDefinitions(context.Context, metadatamodel.Snapshot) error
	DefinitionSnapshot(context.Context) (metadatamodel.Snapshot, error)
}
