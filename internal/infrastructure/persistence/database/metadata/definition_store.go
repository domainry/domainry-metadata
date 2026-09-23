package metadata

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	"github.com/domainry/domainry-orm/query"
)

const (
	definitionTableName        = "_definitions"
	definitionVersionTableName = "_definition_versions"
)

type DefinitionStore struct {
	database       modulehost.Database
	dialect        modulehost.Dialect
	installationID string
	writeMu        *sync.Mutex
}

func NewDefinitionStore(database modulehost.Database, dialect modulehost.Dialect, installationID string) DefinitionStore {
	return DefinitionStore{database: database, dialect: dialect, installationID: strings.TrimSpace(installationID), writeMu: &sync.Mutex{}}
}

var registeredDefinitionKinds = map[string]map[string]bool{
	metadatasdk.DefinitionOwnerMetadata: {
		"application": true, "object": true, "field": true, "validation": true, "action": true, "dictionary": true,
	},
	metadatasdk.DefinitionOwnerIdentity:     {"identity_profile_binding": true, "role": true},
	metadatasdk.DefinitionOwnerWorkflow:     {"workflow": true},
	metadatasdk.DefinitionOwnerAutomation:   {"automation_rule": true},
	metadatasdk.DefinitionOwnerIntegration:  {"integration_connector": true, "integration_event_mapping": true},
	metadatasdk.DefinitionOwnerReport:       {"report": true},
	metadatasdk.DefinitionOwnerAgent:        {"skill": true, "agent": true},
	metadatasdk.DefinitionOwnerScheduler:    {"scheduler": true},
	metadatasdk.DefinitionOwnerNotification: {"delivery_policy": true, "notification_template": true, "notification_template_version": true},
	metadatasdk.DefinitionOwnerLifecycle:    {"retention_policy": true},
}

func normalizeDefinitionOwnerKind(owner, kind string) (string, string, error) {
	owner, kind = strings.TrimSpace(owner), strings.TrimSpace(kind)
	if owner == "" {
		return "", "", &metadatasdk.Error{StatusCode: 400, Code: "metadata.definition_owner_required"}
	}
	registered, found := registeredDefinitionKinds[owner]
	if !found || !registered[kind] {
		return "", "", &metadatasdk.Error{StatusCode: 400, Code: "metadata.definition_kind_unregistered"}
	}
	return owner, kind, nil
}

func registeredDefinitionKind(kind string) bool {
	kind = strings.TrimSpace(kind)
	for _, registered := range registeredDefinitionKinds {
		if registered[kind] {
			return true
		}
	}
	return false
}

func (s DefinitionStore) SyncProjection(ctx context.Context, snapshot metadatasdk.ProjectionSnapshot) error {
	if err := s.validate(); err != nil {
		return err
	}
	if err := validateProjectionIdentity(snapshot.Owner, snapshot.SchemaVersion, snapshot.SourceKind, snapshot.SourceID); err != nil {
		return err
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
	sync := func(executor modulehost.DBTX) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := s.syncDefinitionRows(ctx, executor, snapshot.Owner, snapshot.SchemaVersion, snapshot.SourceKind, snapshot.SourceID, snapshot.Definitions, now); err != nil {
			return err
		}
		if snapshot.Owner == metadatasdk.DefinitionOwnerMetadata {
			return s.syncLocalizedTextRows(ctx, executor, snapshot.SourceKind, snapshot.SourceID, snapshot.LocalizedText, now)
		}
		return nil
	}
	if executor := modulehost.ExecutorFromContext(ctx, nil); executor != nil {
		return sync(executor)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
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

func validateProjectionIdentity(owner, schemaVersion, sourceKind, sourceID string) error {
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(schemaVersion) == "" || strings.TrimSpace(sourceKind) == "" || strings.TrimSpace(sourceID) == "" {
		return &metadatasdk.Error{StatusCode: 400, Code: "metadata.projection_identity_required"}
	}
	if _, found := registeredDefinitionKinds[strings.TrimSpace(owner)]; !found {
		return &metadatasdk.Error{StatusCode: 400, Code: "metadata.definition_owner_unregistered"}
	}
	return nil
}

func (s DefinitionStore) validate() error {
	if s.database == nil || s.dialect == nil || s.installationID == "" || s.writeMu == nil {
		return &metadatasdk.Error{StatusCode: 503, Code: "metadata.store_unavailable"}
	}
	return nil
}

func (s DefinitionStore) syncDefinitionRows(ctx context.Context, executor modulehost.DBTX, owner, schemaVersion, sourceKind, sourceID string, definitions []metadatasdk.Definition, now string) error {
	disable, args, err := query.NewUpdateBuilder(s.dialect, definitionTableName).
		Set("status", "disabled").Set("disabled_at", now).Set("updated_at", now).
		Where(query.And(
			query.Equal("installation_id", s.installationID), query.Equal("owner", owner),
			query.Equal("source_kind", sourceKind), query.Equal("source_id", sourceID), query.Equal("status", "active"),
		)).Build()
	if err != nil {
		return err
	}
	if _, err := executor.ExecContext(ctx, disable, args...); err != nil {
		return err
	}
	for _, definition := range definitions {
		if definition.Owner != "" && strings.TrimSpace(definition.Owner) != owner {
			return &metadatasdk.Error{StatusCode: 400, Code: "metadata.definition_owner_mismatch"}
		}
		_, kind, err := normalizeDefinitionOwnerKind(owner, definition.ResourceType)
		if err != nil {
			return err
		}
		key := strings.TrimSpace(definition.ResourceKey)
		if key == "" || len(definition.Payload) == 0 || !json.Valid(definition.Payload) {
			return &metadatasdk.Error{StatusCode: 400, Code: "metadata.definition_invalid"}
		}
		hash, err := definitionPayloadHash(definition.Payload, definition.SchemaHash)
		if err != nil {
			return err
		}
		current, found, err := s.getDefinition(ctx, executor, owner, kind, key, false)
		if err != nil {
			return err
		}
		if found && (current.SourceKind != sourceKind || current.SourceID != sourceID) {
			continue
		}
		if found && current.SchemaVersion == schemaVersion && current.SchemaHash != hash {
			return definitionVersionConflict()
		}
		definitionID := s.definitionID(owner, kind, key)
		versionID, err := s.ensureDefinitionVersion(ctx, executor, definitionVersion{
			DefinitionID: definitionID, InstallationID: s.installationID, Owner: owner,
			ResourceType: kind, ResourceKey: key, SchemaVersion: schemaVersion,
			SchemaHash: hash, Payload: append(json.RawMessage(nil), definition.Payload...), CreatedAt: now,
		})
		if err != nil {
			return err
		}
		publishedBy := sourceKind + ":" + sourceID
		if found {
			statement, values, buildErr := query.NewUpdateBuilder(s.dialect, definitionTableName).
				Set("current_version_id", versionID).Set("status", "active").
				Set("object_key", strings.TrimSpace(definition.ObjectKey)).Set("name", strings.TrimSpace(definition.Name)).
				Set("payload_json", definition.Payload).Set("schema_version", schemaVersion).Set("schema_hash", hash).
				Set("source_kind", sourceKind).Set("source_id", sourceID).
				Set("published_at", now).Set("published_by", publishedBy).Set("disabled_at", nil).Set("disabled_by", nil).Set("updated_at", now).
				Where(query.Equal("id", definitionID)).Build()
			if buildErr != nil {
				return buildErr
			}
			if _, err := executor.ExecContext(ctx, statement, values...); err != nil {
				return err
			}
			continue
		}
		statement, values, buildErr := query.NewInsertBuilder(s.dialect, definitionTableName).Columns(
			"id", "installation_id", "owner", "kind", "definition_key", "current_version_id", "status", "object_key", "name",
			"payload_json", "schema_version", "schema_hash", "source_kind", "source_id", "published_at", "published_by", "disabled_at", "disabled_by", "created_at", "updated_at",
		).Values(
			definitionID, s.installationID, owner, kind, key, versionID, "active", strings.TrimSpace(definition.ObjectKey), strings.TrimSpace(definition.Name),
			definition.Payload, schemaVersion, hash, sourceKind, sourceID, now, publishedBy, nil, nil, now, now,
		).Build()
		if buildErr != nil {
			return buildErr
		}
		if _, err := executor.ExecContext(ctx, statement, values...); err != nil {
			return err
		}
	}
	purge, purgeArgs, err := query.NewDeleteBuilder(s.dialect, definitionTableName).Where(query.And(
		query.Equal("installation_id", s.installationID), query.Equal("owner", owner),
		query.Equal("source_kind", sourceKind), query.Equal("source_id", sourceID), query.Equal("status", "disabled"),
	)).Build()
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, purge, purgeArgs...)
	return err
}

func (s DefinitionStore) List(ctx context.Context, queryValue metadatasdk.DefinitionQuery) ([]metadatasdk.Definition, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	owner := strings.TrimSpace(queryValue.Owner)
	if owner == "" && !queryValue.CrossOwner {
		return nil, &metadatasdk.Error{StatusCode: 400, Code: "metadata.definition_owner_required"}
	}
	if owner != "" {
		if _, found := registeredDefinitionKinds[owner]; !found {
			return nil, &metadatasdk.Error{StatusCode: 400, Code: "metadata.definition_owner_unregistered"}
		}
	}
	predicates := []query.Predicate{query.Equal("installation_id", s.installationID), query.Equal("status", "active")}
	if owner != "" {
		predicates = append(predicates, query.Equal("owner", owner))
	}
	if kind := strings.TrimSpace(queryValue.ResourceType); kind != "" {
		if owner != "" {
			if _, _, err := normalizeDefinitionOwnerKind(owner, kind); err != nil {
				return nil, err
			}
		} else if !registeredDefinitionKind(kind) {
			return nil, &metadatasdk.Error{StatusCode: 400, Code: "metadata.definition_kind_unregistered"}
		}
		predicates = append(predicates, query.Equal("kind", kind))
	}
	if sourceID := strings.TrimSpace(queryValue.SourceID); sourceID != "" {
		predicates = append(predicates, query.Equal("source_id", sourceID))
	}
	statement, args, err := query.NewSelectBuilder(s.dialect, definitionTableName).Columns(
		definitionColumns()...,
	).Where(query.And(predicates...)).OrderBy(query.Ascending("owner"), query.Ascending("kind"), query.Ascending("definition_key")).Build()
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

func (s DefinitionStore) Get(ctx context.Context, owner, resourceType, key string) (metadatasdk.Definition, bool, error) {
	if err := s.validate(); err != nil {
		return metadatasdk.Definition{}, false, err
	}
	owner, resourceType, err := normalizeDefinitionOwnerKind(owner, resourceType)
	if err != nil {
		return metadatasdk.Definition{}, false, err
	}
	return s.getDefinition(ctx, modulehost.ExecutorFromContext(ctx, s.database), owner, resourceType, strings.TrimSpace(key), true)
}

func (s DefinitionStore) Snapshot(ctx context.Context, queryValue metadatasdk.DefinitionQuery) (metadatasdk.DefinitionSnapshot, error) {
	values, err := s.List(ctx, queryValue)
	return metadatasdk.DefinitionSnapshot{Definitions: values}, err
}

func (s DefinitionStore) getDefinition(ctx context.Context, executor modulehost.DBTX, owner, resourceType, key string, activeOnly bool) (metadatasdk.Definition, bool, error) {
	predicates := []query.Predicate{
		query.Equal("installation_id", s.installationID), query.Equal("owner", owner),
		query.Equal("kind", resourceType), query.Equal("definition_key", key),
	}
	if activeOnly {
		predicates = append(predicates, query.Equal("status", "active"))
	}
	statement, args, err := query.NewSelectBuilder(s.dialect, definitionTableName).Columns(
		definitionColumns()...,
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

func (s DefinitionStore) definitionID(owner, resourceType, key string) string {
	sum := sha256.Sum256([]byte(s.installationID + "\x00" + owner + "\x00" + resourceType + "\x00" + key))
	return "definition:" + hex.EncodeToString(sum[:16])
}

type rowScanner interface{ Scan(...any) error }

func scanDefinition(row rowScanner) (metadatasdk.Definition, error) {
	var value metadatasdk.Definition
	var payload string
	var disabled sql.NullString
	var disabledBy sql.NullString
	err := row.Scan(
		&value.Owner, &value.ResourceType, &value.ResourceKey, &value.CurrentVersionID, &value.Status,
		&value.ObjectKey, &value.Name, &payload, &value.SchemaVersion, &value.SchemaHash,
		&value.SourceKind, &value.SourceID, &value.PublishedAt, &value.PublishedBy,
		&disabled, &disabledBy, &value.CreatedAt, &value.UpdatedAt,
	)
	value.Payload = json.RawMessage(payload)
	if disabled.Valid {
		value.DisabledAt = disabled.String
	}
	if disabledBy.Valid {
		value.DisabledBy = disabledBy.String
	}
	return value, err
}

func definitionColumns() []string {
	return []string{
		"owner", "kind", "definition_key", "current_version_id", "status",
		"object_key", "name", "payload_json", "schema_version", "schema_hash",
		"source_kind", "source_id", "published_at", "published_by",
		"disabled_at", "disabled_by", "created_at", "updated_at",
	}
}

func definitionPayloadHash(payload json.RawMessage, declared string) (string, error) {
	if len(payload) == 0 || !json.Valid(payload) {
		return "", &metadatasdk.Error{StatusCode: 400, Code: "metadata.definition_invalid"}
	}
	sum := sha256.Sum256(payload)
	actual := hex.EncodeToString(sum[:])
	if declared = strings.TrimSpace(declared); declared != "" && declared != actual {
		return "", &metadatasdk.Error{StatusCode: 400, Code: "metadata.definition_hash_mismatch"}
	}
	return actual, nil
}

var _ metadatasdk.Definitions = DefinitionStore{}
var _ metadatasdk.DefinitionStore = DefinitionStore{}
