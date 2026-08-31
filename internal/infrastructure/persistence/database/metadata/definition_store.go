package metadata

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
	metadatapersistence "github.com/domainry/domainry-metadata-sdk/persistence"
	"github.com/domainry/domainry-orm/query"
)

type DefinitionStore struct {
	database modulehost.Database
	dialect  modulehost.Dialect
}

func NewDefinitionStore(database modulehost.Database, dialect modulehost.Dialect) DefinitionStore {
	return DefinitionStore{database: database, dialect: dialect}
}

var definitionTableByResourceType = map[string]string{
	"object": "_metadata_object_definitions", "field": "_metadata_field_definitions", "validation": "_metadata_validation_definitions",
	"action": "_metadata_action_definitions", "dictionary": "_metadata_dictionary_definitions", "role": "_metadata_role_definitions",
}

func (s DefinitionStore) SyncDefinitions(ctx context.Context, snapshot metadatapersistence.Snapshot) error {
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
		statement, args, buildErr := query.NewUpdateBuilder(s.dialect, table).Set("disabled_at", now).Set("updated_at", now).Where(query.And(
			query.Equal("source_kind", snapshot.SourceKind), query.Equal("source_id", snapshot.SourceID), query.IsNull("disabled_at"),
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
		lookup, lookupArgs, buildErr := query.NewSelectBuilder(s.dialect, table).Columns("source_kind", "source_id").Where(query.Equal("resource_key", key)).Build()
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
		update, args, buildErr := query.NewUpdateBuilder(s.dialect, table).
			Set("object_key", strings.TrimSpace(definition.ObjectKey)).Set("name", strings.TrimSpace(definition.Name)).
			Set("payload_json", definition.Payload).Set("schema_version", snapshot.SchemaVersion).Set("schema_hash", hash).
			Set("source_kind", snapshot.SourceKind).Set("source_id", snapshot.SourceID).Set("disabled_at", nil).Set("updated_at", now).
			Where(query.Equal("resource_key", key)).Build()
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
		statement, args, buildErr := query.NewInsertBuilder(s.dialect, table).Columns(
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

func (s DefinitionStore) DefinitionSnapshot(ctx context.Context) (metadatapersistence.Snapshot, error) {
	return s.DefinitionSnapshotWithExecutor(ctx, s.database)
}

func (s DefinitionStore) DefinitionSnapshotWithExecutor(ctx context.Context, executor metadatapersistence.QueryExecutor) (metadatapersistence.Snapshot, error) {
	result := metadatapersistence.Snapshot{Definitions: []metadatapersistence.Definition{}}
	for _, resourceType := range []string{"object", "field", "validation", "action", "dictionary", "role"} {
		table := definitionTableByResourceType[resourceType]
		statement, args, buildErr := query.NewSelectBuilder(s.dialect, table).Columns(
			"resource_key", "object_key", "name", "payload_json", "schema_version", "schema_hash", "source_kind", "source_id",
		).Where(query.IsNull("disabled_at")).OrderBy(query.Ascending("resource_key")).Build()
		if buildErr != nil {
			return metadatapersistence.Snapshot{}, buildErr
		}
		rows, err := executor.QueryContext(ctx, statement, args...)
		if err != nil {
			return metadatapersistence.Snapshot{}, err
		}
		for rows.Next() {
			value := metadatapersistence.Definition{ResourceType: resourceType}
			var version, sourceKind, sourceID, payload string
			if err := rows.Scan(&value.Key, &value.ObjectKey, &value.Name, &payload, &version, &value.SchemaHash, &sourceKind, &sourceID); err != nil {
				rows.Close()
				return metadatapersistence.Snapshot{}, err
			}
			value.Payload = json.RawMessage(payload)
			result.Definitions = append(result.Definitions, value)
			if result.SchemaVersion == "" {
				result.SchemaVersion, result.SourceKind, result.SourceID = version, sourceKind, sourceID
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return metadatapersistence.Snapshot{}, err
		}
		rows.Close()
	}
	return result, nil
}

var _ metadatapersistence.DefinitionRepository = DefinitionStore{}
var _ metadatapersistence.ExecutorSnapshotRepository = DefinitionStore{}
