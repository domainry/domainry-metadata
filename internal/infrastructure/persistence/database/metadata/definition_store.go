package metadata

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	"github.com/domainry/domainry-orm/query"
)

const definitionTableName = "_metadata_definitions"

type DefinitionStore struct {
	database modulehost.Database
	dialect  modulehost.Dialect
}

func NewDefinitionStore(database modulehost.Database, dialect modulehost.Dialect) DefinitionStore {
	return DefinitionStore{database: database, dialect: dialect}
}

var supportedDefinitionTypes = map[string]bool{
	"object": true, "field": true, "validation": true, "action": true, "dictionary": true,
	"workflow": true, "automation_rule": true, "integration_event_mapping": true,
	"report": true, "operation_state_example": true, "sensitive_field_policy": true,
	"report_export_control": true, "identity_profile_binding": true, "skill": true,
	"agent": true, "scheduler": true,
}

func normalizeDefinitionType(resourceType string) (string, error) {
	resourceType = strings.TrimSpace(resourceType)
	if !supportedDefinitionTypes[resourceType] {
		return "", &metadatasdk.Error{StatusCode: 400, Code: "metadata.definition_type_unsupported"}
	}
	return resourceType, nil
}

func (s DefinitionStore) SyncProjection(ctx context.Context, snapshot metadatasdk.ProjectionSnapshot) error {
	if err := s.validate(); err != nil {
		return err
	}
	if err := validateProjectionIdentity(snapshot.SchemaVersion, snapshot.SourceKind, snapshot.SourceID); err != nil {
		return err
	}
	sync := func(executor modulehost.DBTX) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := s.syncDefinitionRows(ctx, executor, snapshot.SchemaVersion, snapshot.SourceKind, snapshot.SourceID, snapshot.Definitions, now); err != nil {
			return err
		}
		if err := s.syncLocalizedTextRows(ctx, executor, snapshot.SourceKind, snapshot.SourceID, snapshot.LocalizedText, now); err != nil {
			return err
		}
		return s.replaceProjectionIdentity(ctx, executor, snapshot, now)
	}
	if executor := modulehost.ExecutorFromContext(ctx, nil); executor != nil {
		return sync(executor)
	}
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := sync(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func validateProjectionIdentity(schemaVersion, sourceKind, sourceID string) error {
	if strings.TrimSpace(schemaVersion) == "" || strings.TrimSpace(sourceKind) == "" || strings.TrimSpace(sourceID) == "" {
		return &metadatasdk.Error{StatusCode: 400, Code: "metadata.projection_identity_required"}
	}
	return nil
}

func (s DefinitionStore) validate() error {
	if s.database == nil || s.dialect == nil {
		return &metadatasdk.Error{StatusCode: 503, Code: "metadata.store_unavailable"}
	}
	return nil
}

func (s DefinitionStore) syncDefinitionRows(ctx context.Context, tx modulehost.DBTX, schemaVersion, sourceKind, sourceID string, definitions []metadatasdk.Definition, now string) error {
	disable, args, err := query.NewUpdateBuilder(s.dialect, definitionTableName).
		Set("disabled_at", now).Set("updated_at", now).
		Where(query.And(query.Equal("source_kind", sourceKind), query.Equal("source_id", sourceID), query.IsNull("disabled_at"))).Build()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, disable, args...); err != nil {
		return err
	}
	for _, definition := range definitions {
		resourceType, err := normalizeDefinitionType(definition.ResourceType)
		if err != nil {
			return err
		}
		key := strings.TrimSpace(definition.ResourceKey)
		if key == "" || len(definition.Payload) == 0 || !json.Valid(definition.Payload) {
			return &metadatasdk.Error{StatusCode: 400, Code: "metadata.definition_invalid"}
		}
		hash := strings.TrimSpace(definition.SchemaHash)
		if hash == "" {
			sum := sha256.Sum256(definition.Payload)
			hash = hex.EncodeToString(sum[:])
		}
		current, found, err := s.getDefinition(ctx, tx, resourceType, key, false)
		if err != nil {
			return err
		}
		if found && (current.SourceKind != sourceKind || current.SourceID != sourceID) {
			continue
		}
		if !found || current.SchemaHash != hash {
			if err := s.insertDefinitionVersionIfMissing(ctx, tx, definitionVersion{
				ResourceType: resourceType, ResourceKey: key, SchemaVersion: schemaVersion,
				SchemaHash: hash, Payload: append(json.RawMessage(nil), definition.Payload...), CreatedAt: now,
			}); err != nil {
				return err
			}
		}
		if found {
			statement, values, buildErr := query.NewUpdateBuilder(s.dialect, definitionTableName).
				Set("object_key", strings.TrimSpace(definition.ObjectKey)).Set("name", strings.TrimSpace(definition.Name)).
				Set("payload_json", definition.Payload).Set("schema_version", schemaVersion).Set("schema_hash", hash).
				Set("source_kind", sourceKind).Set("source_id", sourceID).Set("disabled_at", nil).Set("updated_at", now).
				Where(query.And(query.Equal("resource_type", resourceType), query.Equal("resource_key", key))).Build()
			if buildErr != nil {
				return buildErr
			}
			if _, err := tx.ExecContext(ctx, statement, values...); err != nil {
				return err
			}
			continue
		}
		statement, values, buildErr := query.NewInsertBuilder(s.dialect, definitionTableName).Columns(
			"id", "resource_type", "resource_key", "object_key", "name", "payload_json", "schema_version", "schema_hash", "source_kind", "source_id", "disabled_at", "created_at", "updated_at",
		).Values(resourceType+":"+key, resourceType, key, strings.TrimSpace(definition.ObjectKey), strings.TrimSpace(definition.Name), definition.Payload, schemaVersion, hash, sourceKind, sourceID, nil, now, now).Build()
		if buildErr != nil {
			return buildErr
		}
		if _, err := tx.ExecContext(ctx, statement, values...); err != nil {
			return err
		}
	}
	purge, purgeArgs, err := query.NewDeleteBuilder(s.dialect, definitionTableName).Where(query.And(
		query.Equal("source_kind", sourceKind), query.Equal("source_id", sourceID), query.IsNotNull("disabled_at"),
	)).Build()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, purge, purgeArgs...)
	return err
}

func (s DefinitionStore) List(ctx context.Context, queryValue metadatasdk.DefinitionQuery) ([]metadatasdk.Definition, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	predicates := []query.Predicate{query.IsNull("disabled_at")}
	if resourceType := strings.TrimSpace(queryValue.ResourceType); resourceType != "" {
		var err error
		resourceType, err = normalizeDefinitionType(resourceType)
		if err != nil {
			return nil, err
		}
		predicates = append(predicates, query.Equal("resource_type", resourceType))
	}
	if sourceID := strings.TrimSpace(queryValue.SourceID); sourceID != "" {
		predicates = append(predicates, query.Equal("source_id", sourceID))
	}
	statement, args, err := query.NewSelectBuilder(s.dialect, definitionTableName).Columns(
		"resource_type", "resource_key", "object_key", "name", "payload_json", "schema_version", "schema_hash", "source_kind", "source_id", "disabled_at", "created_at", "updated_at",
	).Where(query.And(predicates...)).OrderBy(query.Ascending("resource_type"), query.Ascending("resource_key")).Build()
	if err != nil {
		return nil, err
	}
	rows, err := modulehost.ExecutorFromContext(ctx, s.database).QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []metadatasdk.Definition{}
	for rows.Next() {
		value, err := scanDefinition(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s DefinitionStore) Get(ctx context.Context, resourceType, key string) (metadatasdk.Definition, bool, error) {
	if err := s.validate(); err != nil {
		return metadatasdk.Definition{}, false, err
	}
	resourceType, err := normalizeDefinitionType(resourceType)
	if err != nil {
		return metadatasdk.Definition{}, false, err
	}
	return s.getDefinition(ctx, modulehost.ExecutorFromContext(ctx, s.database), resourceType, strings.TrimSpace(key), true)
}

func (s DefinitionStore) Snapshot(ctx context.Context) (metadatasdk.DefinitionSnapshot, error) {
	values, err := s.List(ctx, metadatasdk.DefinitionQuery{})
	return metadatasdk.DefinitionSnapshot{Definitions: values}, err
}

func (s DefinitionStore) getDefinition(ctx context.Context, executor modulehost.DBTX, resourceType, key string, activeOnly bool) (metadatasdk.Definition, bool, error) {
	predicates := []query.Predicate{query.Equal("resource_type", resourceType), query.Equal("resource_key", key)}
	if activeOnly {
		predicates = append(predicates, query.IsNull("disabled_at"))
	}
	statement, args, err := query.NewSelectBuilder(s.dialect, definitionTableName).Columns(
		"resource_type", "resource_key", "object_key", "name", "payload_json", "schema_version", "schema_hash", "source_kind", "source_id", "disabled_at", "created_at", "updated_at",
	).Where(query.And(predicates...)).Build()
	if err != nil {
		return metadatasdk.Definition{}, false, err
	}
	rows, err := executor.QueryContext(ctx, statement, args...)
	if err != nil {
		return metadatasdk.Definition{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return metadatasdk.Definition{}, false, rows.Err()
	}
	value, err := scanDefinition(rows)
	return value, err == nil, err
}

type rowScanner interface{ Scan(...any) error }

func scanDefinition(row rowScanner) (metadatasdk.Definition, error) {
	var value metadatasdk.Definition
	var payload string
	var disabled sql.NullString
	err := row.Scan(&value.ResourceType, &value.ResourceKey, &value.ObjectKey, &value.Name, &payload, &value.SchemaVersion, &value.SchemaHash, &value.SourceKind, &value.SourceID, &disabled, &value.CreatedAt, &value.UpdatedAt)
	value.Payload = json.RawMessage(payload)
	if disabled.Valid {
		value.DisabledAt = disabled.String
	}
	return value, err
}

var _ metadatasdk.Definitions = DefinitionStore{}
