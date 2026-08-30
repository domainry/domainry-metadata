package persistence

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/domainry/domainry-metadata-sdk/modulehost"
	metadatarepository "github.com/domainry/domainry-metadata-sdk/repository"
	ormbuilder "github.com/domainry/domainry-orm/builder"
)

type DefinitionStore struct {
	database modulehost.Database
	dialect  modulehost.Dialect
}

func NewDefinitionStore(database modulehost.Database, dialect modulehost.Dialect) DefinitionStore {
	return DefinitionStore{database: database, dialect: dialect}
}

var definitionTableByResourceType = map[string]string{
	"object": "object_definitions", "field": "field_definitions", "validation": "validation_definitions",
	"action": "action_definitions", "dictionary": "dictionary_definitions", "role": "role_definitions",
	"identity_profile_binding": "identity_profile_binding_definitions",
}

func (s DefinitionStore) SyncDefinitions(ctx context.Context, snapshot metadatarepository.Snapshot) error {
	if s.database == nil || s.dialect == nil {
		return fmt.Errorf("Metadata definition store is unavailable")
	}
	if strings.TrimSpace(snapshot.SchemaVersion) == "" || strings.TrimSpace(snapshot.SourceKind) == "" || strings.TrimSpace(snapshot.SourceID) == "" {
		return fmt.Errorf("Metadata definition snapshot identity is required")
	}
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, table := range definitionTables {
		statement, args, buildErr := ormbuilder.NewUpdateBuilder(s.dialect, table).Set("disabled_at", now).Set("updated_at", now).Where(ormbuilder.And(
			ormbuilder.Equal("source_kind", snapshot.SourceKind), ormbuilder.Equal("source_id", snapshot.SourceID), ormbuilder.IsNull("disabled_at"),
		)).Build()
		if buildErr != nil {
			return buildErr
		}
		if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
			return err
		}
	}
	for _, definition := range snapshot.Definitions {
		table := definitionTableByResourceType[strings.TrimSpace(definition.ResourceType)]
		key := strings.TrimSpace(definition.Key)
		if table == "" || key == "" || len(definition.Payload) == 0 {
			return fmt.Errorf("Metadata definition identity is invalid")
		}
		sum := sha256.Sum256(definition.Payload)
		hash := hex.EncodeToString(sum[:])
		lookup, lookupArgs, buildErr := ormbuilder.NewSelectBuilder(s.dialect, table).Columns("source_kind", "source_id").Where(ormbuilder.Equal("resource_key", key)).Build()
		if buildErr != nil {
			return buildErr
		}
		var existingKind, existingID string
		lookupErr := tx.QueryRowContext(ctx, lookup, lookupArgs...).Scan(&existingKind, &existingID)
		if lookupErr != nil && lookupErr != sql.ErrNoRows {
			return lookupErr
		}
		if lookupErr == nil && (existingKind != snapshot.SourceKind || existingID != snapshot.SourceID) {
			continue
		}
		update, args, buildErr := ormbuilder.NewUpdateBuilder(s.dialect, table).
			Set("object_key", strings.TrimSpace(definition.ObjectKey)).Set("name", strings.TrimSpace(definition.Name)).
			Set("payload_json", definition.Payload).Set("schema_version", snapshot.SchemaVersion).Set("schema_hash", hash).
			Set("source_kind", snapshot.SourceKind).Set("source_id", snapshot.SourceID).Set("disabled_at", nil).Set("updated_at", now).
			Where(ormbuilder.Equal("resource_key", key)).Build()
		if buildErr != nil {
			return buildErr
		}
		result, err := tx.ExecContext(ctx, update, args...)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected > 0 {
			continue
		}
		statement, args, buildErr := ormbuilder.NewInsertBuilder(s.dialect, table).Columns(
			"id", "resource_key", "object_key", "name", "payload_json", "schema_version", "schema_hash", "source_kind", "source_id", "disabled_at", "created_at", "updated_at",
		).Values(definition.ResourceType+":"+key, key, strings.TrimSpace(definition.ObjectKey), strings.TrimSpace(definition.Name), definition.Payload, snapshot.SchemaVersion, hash, snapshot.SourceKind, snapshot.SourceID, nil, now, now).Build()
		if buildErr != nil {
			return buildErr
		}
		if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s DefinitionStore) DefinitionSnapshot(ctx context.Context) (metadatarepository.Snapshot, error) {
	return s.DefinitionSnapshotWithExecutor(ctx, s.database)
}

func (s DefinitionStore) DefinitionSnapshotWithExecutor(ctx context.Context, executor metadatarepository.QueryExecutor) (metadatarepository.Snapshot, error) {
	result := metadatarepository.Snapshot{Definitions: []metadatarepository.Definition{}}
	for _, resourceType := range []string{"object", "field", "validation", "action", "dictionary", "role", "identity_profile_binding"} {
		table := definitionTableByResourceType[resourceType]
		statement, args, buildErr := ormbuilder.NewSelectBuilder(s.dialect, table).Columns(
			"resource_key", "object_key", "name", "payload_json", "schema_version", "schema_hash", "source_kind", "source_id",
		).Where(ormbuilder.IsNull("disabled_at")).OrderBy(ormbuilder.Ascending("resource_key")).Build()
		if buildErr != nil {
			return metadatarepository.Snapshot{}, buildErr
		}
		rows, err := executor.QueryContext(ctx, statement, args...)
		if err != nil {
			return metadatarepository.Snapshot{}, err
		}
		for rows.Next() {
			value := metadatarepository.Definition{ResourceType: resourceType}
			var version, sourceKind, sourceID, payload string
			if err := rows.Scan(&value.Key, &value.ObjectKey, &value.Name, &payload, &version, &value.SchemaHash, &sourceKind, &sourceID); err != nil {
				rows.Close()
				return metadatarepository.Snapshot{}, err
			}
			value.Payload = json.RawMessage(payload)
			result.Definitions = append(result.Definitions, value)
			if result.SchemaVersion == "" {
				result.SchemaVersion, result.SourceKind, result.SourceID = version, sourceKind, sourceID
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return metadatarepository.Snapshot{}, err
		}
		rows.Close()
	}
	return result, nil
}

var _ metadatarepository.DefinitionRepository = DefinitionStore{}
var _ metadatarepository.ExecutorSnapshotRepository = DefinitionStore{}
