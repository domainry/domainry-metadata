package metadata

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	"github.com/domainry/domainry-orm/query"
)

func (s DefinitionStore) ReplaceSourceSnapshot(ctx context.Context, snapshot metadatasdk.ProjectionSnapshot) error {
	return s.SyncProjection(ctx, snapshot)
}

func (s DefinitionStore) Publish(ctx context.Context, command metadatasdk.DefinitionPublishCommand) (metadatasdk.DefinitionPublishResult, error) {
	if err := s.validate(); err != nil {
		return metadatasdk.DefinitionPublishResult{}, err
	}
	owner, kind, err := normalizeDefinitionOwnerKind(command.Owner, command.ResourceType)
	if err != nil {
		return metadatasdk.DefinitionPublishResult{}, err
	}
	command.Owner, command.ResourceType = owner, kind
	command.ResourceKey = strings.TrimSpace(command.ResourceKey)
	command.ExpectedCurrentVersionID = strings.TrimSpace(command.ExpectedCurrentVersionID)
	command.SchemaVersion = strings.TrimSpace(command.SchemaVersion)
	command.SourceKind = strings.TrimSpace(command.SourceKind)
	command.SourceID = strings.TrimSpace(command.SourceID)
	command.PublishedBy = strings.TrimSpace(command.PublishedBy)
	if command.ResourceKey == "" || command.ExpectedCurrentVersionID == "" || command.SchemaVersion == "" || command.SourceKind == "" || command.SourceID == "" || command.PublishedBy == "" {
		return metadatasdk.DefinitionPublishResult{}, &metadatasdk.Error{StatusCode: 400, Code: "metadata.definition_publication_invalid"}
	}
	hash, err := definitionPayloadHash(command.Payload, command.SchemaHash)
	if err != nil {
		return metadatasdk.DefinitionPublishResult{}, err
	}
	command.SchemaHash = hash
	publish := func(executor modulehost.DBTX) (metadatasdk.DefinitionPublishResult, error) {
		return s.publishDefinition(ctx, executor, command)
	}
	if executor := modulehost.ExecutorFromContext(ctx, nil); executor != nil {
		return publish(executor)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return metadatasdk.DefinitionPublishResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := publish(tx)
	if err != nil {
		return metadatasdk.DefinitionPublishResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return metadatasdk.DefinitionPublishResult{}, err
	}
	return result, nil
}

func (s DefinitionStore) publishDefinition(ctx context.Context, executor modulehost.DBTX, command metadatasdk.DefinitionPublishCommand) (metadatasdk.DefinitionPublishResult, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	definitionID := s.definitionID(command.Owner, command.ResourceType, command.ResourceKey)
	versionID, err := s.ensureDefinitionVersion(ctx, executor, definitionVersion{
		DefinitionID: definitionID, InstallationID: s.installationID, Owner: command.Owner,
		ResourceType: command.ResourceType, ResourceKey: command.ResourceKey,
		SchemaVersion: command.SchemaVersion, SchemaHash: command.SchemaHash,
		Payload: append(json.RawMessage(nil), command.Payload...), CreatedAt: now,
	})
	if err != nil {
		return metadatasdk.DefinitionPublishResult{}, err
	}
	if command.ExpectedCurrentVersionID != metadatasdk.DefinitionNoCurrentVersion && versionID == command.ExpectedCurrentVersionID {
		current, found, readErr := s.getDefinition(ctx, executor, command.Owner, command.ResourceType, command.ResourceKey, true)
		if readErr != nil {
			return metadatasdk.DefinitionPublishResult{}, readErr
		}
		if found && current.CurrentVersionID == versionID && current.SchemaVersion == command.SchemaVersion && current.SchemaHash == command.SchemaHash &&
			current.ObjectKey == strings.TrimSpace(command.ObjectKey) && current.Name == strings.TrimSpace(command.Name) &&
			current.SourceKind == command.SourceKind && current.SourceID == command.SourceID && current.PublishedBy == command.PublishedBy && bytes.Equal(current.Payload, command.Payload) {
			return metadatasdk.DefinitionPublishResult{Definition: current, CurrentVersionID: versionID}, nil
		}
		return metadatasdk.DefinitionPublishResult{}, definitionRevisionConflict()
	}
	var affected int64
	if command.ExpectedCurrentVersionID == metadatasdk.DefinitionNoCurrentVersion {
		statement, args, buildErr := query.NewInsertBuilder(s.dialect, definitionTableName).Columns(
			"id", "installation_id", "owner", "kind", "definition_key", "current_version_id", "status", "object_key", "name",
			"payload_json", "schema_version", "schema_hash", "source_kind", "source_id", "published_at", "published_by", "disabled_at", "disabled_by", "created_at", "updated_at",
		).Values(
			definitionID, s.installationID, command.Owner, command.ResourceType, command.ResourceKey, versionID, "active",
			strings.TrimSpace(command.ObjectKey), strings.TrimSpace(command.Name), command.Payload, command.SchemaVersion, command.SchemaHash,
			command.SourceKind, command.SourceID, now, command.PublishedBy, nil, nil, now, now,
		).OnConflictDoNothing("id").Build()
		if buildErr != nil {
			return metadatasdk.DefinitionPublishResult{}, buildErr
		}
		result, execErr := executor.ExecContext(ctx, statement, args...)
		if execErr != nil {
			return metadatasdk.DefinitionPublishResult{}, execErr
		}
		affected, err = result.RowsAffected()
	} else {
		statement, args, buildErr := query.NewUpdateBuilder(s.dialect, definitionTableName).
			Set("current_version_id", versionID).Set("status", "active").
			Set("object_key", strings.TrimSpace(command.ObjectKey)).Set("name", strings.TrimSpace(command.Name)).
			Set("payload_json", command.Payload).Set("schema_version", command.SchemaVersion).Set("schema_hash", command.SchemaHash).
			Set("source_kind", command.SourceKind).Set("source_id", command.SourceID).
			Set("published_at", now).Set("published_by", command.PublishedBy).Set("disabled_at", nil).Set("disabled_by", nil).Set("updated_at", now).
			Where(query.And(
				query.Equal("id", definitionID), query.Equal("installation_id", s.installationID),
				query.Equal("owner", command.Owner), query.Equal("kind", command.ResourceType),
				query.Equal("definition_key", command.ResourceKey), query.Equal("current_version_id", command.ExpectedCurrentVersionID), query.Equal("status", "active"),
			)).Build()
		if buildErr != nil {
			return metadatasdk.DefinitionPublishResult{}, buildErr
		}
		result, execErr := executor.ExecContext(ctx, statement, args...)
		if execErr != nil {
			return metadatasdk.DefinitionPublishResult{}, execErr
		}
		affected, err = result.RowsAffected()
	}
	if err != nil {
		return metadatasdk.DefinitionPublishResult{}, err
	}
	if affected != 1 {
		return metadatasdk.DefinitionPublishResult{}, definitionRevisionConflict()
	}
	current, found, err := s.getDefinition(ctx, executor, command.Owner, command.ResourceType, command.ResourceKey, true)
	if err != nil {
		return metadatasdk.DefinitionPublishResult{}, err
	}
	if !found || current.CurrentVersionID != versionID {
		return metadatasdk.DefinitionPublishResult{}, definitionRevisionConflict()
	}
	return metadatasdk.DefinitionPublishResult{Definition: current, CurrentVersionID: versionID}, nil
}

func (s DefinitionStore) Disable(ctx context.Context, command metadatasdk.DefinitionDisableCommand) error {
	if err := s.validate(); err != nil {
		return err
	}
	owner, kind, err := normalizeDefinitionOwnerKind(command.Owner, command.ResourceType)
	if err != nil {
		return err
	}
	key := strings.TrimSpace(command.ResourceKey)
	expected := strings.TrimSpace(command.ExpectedCurrentVersionID)
	disabledBy := strings.TrimSpace(command.DisabledBy)
	if key == "" || expected == "" || expected == metadatasdk.DefinitionNoCurrentVersion || disabledBy == "" {
		return &metadatasdk.Error{StatusCode: 400, Code: "metadata.definition_disable_invalid"}
	}
	disable := func(executor modulehost.DBTX) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		statement, args, buildErr := query.NewUpdateBuilder(s.dialect, definitionTableName).
			Set("status", "disabled").Set("disabled_at", now).Set("disabled_by", disabledBy).Set("updated_at", now).
			Where(query.And(
				query.Equal("installation_id", s.installationID), query.Equal("owner", owner), query.Equal("kind", kind),
				query.Equal("definition_key", key), query.Equal("current_version_id", expected), query.Equal("status", "active"),
			)).Build()
		if buildErr != nil {
			return buildErr
		}
		result, execErr := executor.ExecContext(ctx, statement, args...)
		if execErr != nil {
			return execErr
		}
		affected, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return rowsErr
		}
		if affected != 1 {
			return definitionRevisionConflict()
		}
		return nil
	}
	if executor := modulehost.ExecutorFromContext(ctx, nil); executor != nil {
		return disable(executor)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := disable(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s DefinitionStore) GetVersion(ctx context.Context, value metadatasdk.DefinitionVersionQuery) (metadatasdk.DefinitionVersion, bool, error) {
	if err := s.validate(); err != nil {
		return metadatasdk.DefinitionVersion{}, false, err
	}
	owner, kind, err := normalizeDefinitionOwnerKind(value.Owner, value.ResourceType)
	if err != nil {
		return metadatasdk.DefinitionVersion{}, false, err
	}
	key := strings.TrimSpace(value.ResourceKey)
	versionID := strings.TrimSpace(value.VersionID)
	schemaVersion := strings.TrimSpace(value.SchemaVersion)
	if key == "" || (versionID == "") == (schemaVersion == "") {
		return metadatasdk.DefinitionVersion{}, false, &metadatasdk.Error{StatusCode: 400, Code: "metadata.definition_version_query_invalid"}
	}
	predicates := []query.Predicate{
		query.Equal("installation_id", s.installationID), query.Equal("owner", owner), query.Equal("kind", kind), query.Equal("definition_key", key),
	}
	if versionID != "" {
		predicates = append(predicates, query.Equal("id", versionID))
	} else {
		predicates = append(predicates, query.Equal("schema_version", schemaVersion))
	}
	statement, args, err := query.NewSelectBuilder(s.dialect, definitionVersionTableName).Columns(
		"id", "owner", "kind", "definition_key", "schema_version", "schema_hash", "payload_json", "created_at",
	).Where(query.And(predicates...)).Build()
	if err != nil {
		return metadatasdk.DefinitionVersion{}, false, err
	}
	var result metadatasdk.DefinitionVersion
	var payload string
	err = modulehost.ExecutorFromContext(ctx, s.database).QueryRowContext(ctx, statement, args...).Scan(
		&result.ID, &result.Owner, &result.ResourceType, &result.ResourceKey, &result.SchemaVersion, &result.SchemaHash, &payload, &result.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return metadatasdk.DefinitionVersion{}, false, nil
	}
	if err != nil {
		return metadatasdk.DefinitionVersion{}, false, err
	}
	result.Payload = json.RawMessage(payload)
	return result, true, nil
}

func definitionRevisionConflict() error {
	return &metadatasdk.Error{StatusCode: 409, Code: "metadata.definition_revision_conflict"}
}
