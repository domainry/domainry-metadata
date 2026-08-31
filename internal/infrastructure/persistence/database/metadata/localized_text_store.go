package metadata

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	"github.com/domainry/domainry-orm/query"
)

const localizedTextTableName = "_metadata_localized_texts"

func (s DefinitionStore) syncLocalizedTextRows(ctx context.Context, executor modulehost.DBTX, sourceKind, sourceID string, values []metadatasdk.LocalizedText, now string) error {
	remove, args, err := query.NewDeleteBuilder(s.dialect, localizedTextTableName).Where(query.And(
		query.Equal("source_kind", sourceKind), query.Equal("source_id", sourceID),
	)).Build()
	if err != nil {
		return err
	}
	if _, err := executor.ExecContext(ctx, remove, args...); err != nil {
		return err
	}
	for _, value := range values {
		value = normalizeLocalizedText(value)
		if value.WorkspaceID == "" || value.EntityType == "" || value.EntityKey == "" || value.Property == "" || value.Locale == "" || value.Text == "" {
			return &metadatasdk.Error{StatusCode: 400, Code: "metadata.localized_text_invalid"}
		}
		value.SourceKind, value.SourceID = sourceKind, sourceID
		predicates := localizedTextIdentity(value)
		lookup, lookupArgs, err := query.NewSelectBuilder(s.dialect, localizedTextTableName).Columns("source_kind", "source_id").Where(predicates).Build()
		if err != nil {
			return err
		}
		var existingKind, existingID string
		lookupErr := executor.QueryRowContext(ctx, lookup, lookupArgs...).Scan(&existingKind, &existingID)
		if lookupErr != nil && lookupErr != sql.ErrNoRows {
			return lookupErr
		}
		if lookupErr == nil && (existingKind != sourceKind || existingID != sourceID) {
			continue
		}
		if lookupErr == nil {
			update, updateArgs, err := query.NewUpdateBuilder(s.dialect, localizedTextTableName).
				Set("text", value.Text).Set("source_kind", sourceKind).Set("source_id", sourceID).Set("updated_at", now).
				Where(predicates).Build()
			if err != nil {
				return err
			}
			if _, err := executor.ExecContext(ctx, update, updateArgs...); err != nil {
				return err
			}
			continue
		}
		insert, insertArgs, err := query.NewInsertBuilder(s.dialect, localizedTextTableName).Columns(
			"id", "workspace_id", "entity_type", "entity_key", "property", "locale", "text", "source_kind", "source_id", "created_at", "updated_at",
		).Values(localizedTextID(value), value.WorkspaceID, value.EntityType, value.EntityKey, value.Property, value.Locale, value.Text, sourceKind, sourceID, now, now).Build()
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(ctx, insert, insertArgs...); err != nil {
			return err
		}
	}
	return nil
}

func (s DefinitionStore) replaceProjectionIdentity(ctx context.Context, executor modulehost.DBTX, snapshot metadatasdk.ProjectionSnapshot, now string) error {
	remove, args, err := query.NewDeleteBuilder(s.dialect, "_metadata_projection").Where(query.Equal("id", "current")).Build()
	if err != nil {
		return err
	}
	if _, err := executor.ExecContext(ctx, remove, args...); err != nil {
		return err
	}
	insert, args, err := query.NewInsertBuilder(s.dialect, "_metadata_projection").Columns(
		"id", "schema_version", "source_kind", "source_id", "name", "default_locale", "updated_at",
	).Values("current", strings.TrimSpace(snapshot.SchemaVersion), strings.TrimSpace(snapshot.SourceKind), strings.TrimSpace(snapshot.SourceID), strings.TrimSpace(snapshot.Name), valueOrDefault(snapshot.DefaultLocale, "en-US"), now).Build()
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, insert, args...)
	return err
}

func (s DefinitionStore) ListLocalizedTexts(ctx context.Context, queryValue metadatasdk.LocalizedTextQuery) ([]metadatasdk.LocalizedText, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	queryValue.WorkspaceID = strings.TrimSpace(queryValue.WorkspaceID)
	if queryValue.WorkspaceID == "" {
		return nil, &metadatasdk.Error{StatusCode: 400, Code: "metadata.workspace_required"}
	}
	predicates := []query.Predicate{query.Equal("workspace_id", queryValue.WorkspaceID)}
	add := func(column, value string) {
		if value = strings.TrimSpace(value); value != "" {
			predicates = append(predicates, query.Equal(column, value))
		}
	}
	add("entity_type", queryValue.EntityType)
	add("entity_key", queryValue.EntityKey)
	add("property", queryValue.Property)
	add("locale", queryValue.Locale)
	statement, args, err := query.NewSelectBuilder(s.dialect, localizedTextTableName).Columns(
		"workspace_id", "entity_type", "entity_key", "property", "locale", "text", "source_kind", "source_id", "created_at", "updated_at",
	).Where(query.And(predicates...)).OrderBy(query.Ascending("entity_type"), query.Ascending("entity_key"), query.Ascending("property"), query.Ascending("locale")).Build()
	if err != nil {
		return nil, err
	}
	rows, err := modulehost.ExecutorFromContext(ctx, s.database).QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []metadatasdk.LocalizedText{}
	for rows.Next() {
		var value metadatasdk.LocalizedText
		if err := rows.Scan(&value.WorkspaceID, &value.EntityType, &value.EntityKey, &value.Property, &value.Locale, &value.Text, &value.SourceKind, &value.SourceID, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s DefinitionStore) ProjectionName(ctx context.Context) (string, error) {
	statement, args, err := query.NewSelectBuilder(s.dialect, "_metadata_projection").Columns("name").Where(query.Equal("id", "current")).Build()
	if err != nil {
		return "", err
	}
	var name string
	if err := modulehost.ExecutorFromContext(ctx, s.database).QueryRowContext(ctx, statement, args...).Scan(&name); err != nil {
		return "", fmt.Errorf("load Metadata projection name: %w", err)
	}
	return strings.TrimSpace(name), nil
}

func normalizeLocalizedText(value metadatasdk.LocalizedText) metadatasdk.LocalizedText {
	value.WorkspaceID = strings.TrimSpace(value.WorkspaceID)
	value.EntityType = strings.TrimSpace(value.EntityType)
	value.EntityKey = strings.TrimSpace(value.EntityKey)
	value.Property = strings.TrimSpace(value.Property)
	value.Locale = strings.TrimSpace(value.Locale)
	value.Text = strings.TrimSpace(value.Text)
	value.SourceKind = strings.TrimSpace(value.SourceKind)
	value.SourceID = strings.TrimSpace(value.SourceID)
	return value
}

func localizedTextIdentity(value metadatasdk.LocalizedText) query.Predicate {
	return query.And(query.Equal("workspace_id", value.WorkspaceID), query.Equal("entity_type", value.EntityType), query.Equal("entity_key", value.EntityKey), query.Equal("property", value.Property), query.Equal("locale", value.Locale))
}

func localizedTextID(value metadatasdk.LocalizedText) string {
	sum := sha256.Sum256([]byte(value.WorkspaceID + "\x00" + value.EntityType + "\x00" + value.EntityKey + "\x00" + value.Property + "\x00" + value.Locale))
	return "localized:" + hex.EncodeToString(sum[:16])
}

func valueOrDefault(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}
