package metadata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	shareddefinition "github.com/domainry/domainry-foundation/definition"
	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	"github.com/domainry/domainry-metadata-sdk/modulehost"
)

// DefinitionStore composes Foundation's shared Definition kernel with
// Metadata's private localization projection. The shared SQL implementation
// and its two tables are not owned by this package.
type DefinitionStore struct {
	shared   shareddefinition.Store
	database modulehost.Database
	dialect  modulehost.Dialect
	writeMu  *sync.Mutex
}

func NewDefinitionStore(database modulehost.Database, dialect modulehost.Dialect, installationID string) DefinitionStore {
	return NewDefinitionStoreWithShared(database, dialect, shareddefinition.NewStore(database, dialect, installationID))
}

func NewDefinitionStoreWithShared(database modulehost.Database, dialect modulehost.Dialect, shared shareddefinition.Store) DefinitionStore {
	return DefinitionStore{shared: shared, database: database, dialect: dialect, writeMu: &sync.Mutex{}}
}

func (s DefinitionStore) SyncProjection(ctx context.Context, snapshot metadatasdk.ProjectionSnapshot) error {
	if err := s.validate(); err != nil {
		return err
	}
	if strings.TrimSpace(snapshot.Owner) == "" || strings.TrimSpace(snapshot.SchemaVersion) == "" || strings.TrimSpace(snapshot.SourceKind) == "" || strings.TrimSpace(snapshot.SourceID) == "" {
		return &metadatasdk.Error{StatusCode: 400, Code: "metadata.projection_identity_required"}
	}
	snapshot.Owner = strings.TrimSpace(snapshot.Owner)
	snapshot.SchemaVersion = strings.TrimSpace(snapshot.SchemaVersion)
	snapshot.SourceKind = strings.TrimSpace(snapshot.SourceKind)
	snapshot.SourceID = strings.TrimSpace(snapshot.SourceID)
	if snapshot.Owner != metadatasdk.DefinitionOwnerMetadata && len(snapshot.LocalizedText) > 0 {
		return &metadatasdk.Error{StatusCode: 400, Code: "metadata.localized_text_owner_invalid"}
	}
	if snapshot.Owner == metadatasdk.DefinitionOwnerMetadata {
		snapshot.Definitions = append(snapshot.Definitions, projectionApplicationDefinition(snapshot))
	}
	apply := func(executor modulehost.DBTX) error {
		sharedContext := modulehost.WithExecutor(ctx, executor)
		if err := s.shared.ReplaceSourceSnapshot(sharedContext, shareddefinition.SourceSnapshot{
			Owner: snapshot.Owner, SchemaVersion: snapshot.SchemaVersion,
			SourceKind: snapshot.SourceKind, SourceID: snapshot.SourceID,
			Definitions: snapshot.Definitions,
		}); err != nil {
			return err
		}
		if snapshot.Owner == metadatasdk.DefinitionOwnerMetadata {
			return s.syncLocalizedTextRows(ctx, executor, snapshot.SourceKind, snapshot.SourceID, snapshot.LocalizedText, time.Now().UTC().Format(time.RFC3339Nano))
		}
		return nil
	}
	if executor := modulehost.ExecutorFromContext(ctx, nil); executor != nil {
		return apply(executor)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := apply(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func projectionApplicationDefinition(snapshot metadatasdk.ProjectionSnapshot) metadatasdk.Definition {
	defaultLocale := strings.TrimSpace(snapshot.DefaultLocale)
	if defaultLocale == "" {
		defaultLocale = "en-US"
	}
	payload, _ := json.Marshal(struct {
		Name          string `json:"name"`
		DefaultLocale string `json:"default_locale"`
	}{Name: strings.TrimSpace(snapshot.Name), DefaultLocale: defaultLocale})
	sum := sha256.Sum256([]byte(strings.TrimSpace(snapshot.SourceKind) + "\x00" + strings.TrimSpace(snapshot.SourceID)))
	return metadatasdk.Definition{
		Owner: snapshot.Owner, ResourceType: "application", ResourceKey: "projection:" + hex.EncodeToString(sum[:16]),
		Name: strings.TrimSpace(snapshot.Name), Payload: payload,
	}
}

func (s DefinitionStore) validate() error {
	if s.database == nil || s.dialect == nil || s.writeMu == nil {
		return &metadatasdk.Error{StatusCode: 503, Code: "metadata.store_unavailable"}
	}
	return nil
}

func (s DefinitionStore) List(ctx context.Context, query metadatasdk.DefinitionQuery) ([]metadatasdk.Definition, error) {
	return s.shared.List(ctx, query)
}

func (s DefinitionStore) Get(ctx context.Context, owner, resourceType, key string) (metadatasdk.Definition, bool, error) {
	return s.shared.Get(ctx, owner, resourceType, key)
}

func (s DefinitionStore) Snapshot(ctx context.Context, query metadatasdk.DefinitionQuery) (metadatasdk.DefinitionSnapshot, error) {
	return s.shared.Snapshot(ctx, query)
}

func (s DefinitionStore) ReplaceSourceSnapshot(ctx context.Context, snapshot metadatasdk.ProjectionSnapshot) error {
	return s.SyncProjection(ctx, snapshot)
}

func (s DefinitionStore) Publish(ctx context.Context, command metadatasdk.DefinitionPublishCommand) (metadatasdk.DefinitionPublishResult, error) {
	return s.shared.Publish(ctx, command)
}

func (s DefinitionStore) Disable(ctx context.Context, command metadatasdk.DefinitionDisableCommand) error {
	return s.shared.Disable(ctx, command)
}

func (s DefinitionStore) GetVersion(ctx context.Context, query metadatasdk.DefinitionVersionQuery) (metadatasdk.DefinitionVersion, bool, error) {
	return s.shared.GetVersion(ctx, query)
}

var _ metadatasdk.Definitions = DefinitionStore{}
var _ metadatasdk.DefinitionStore = DefinitionStore{}
