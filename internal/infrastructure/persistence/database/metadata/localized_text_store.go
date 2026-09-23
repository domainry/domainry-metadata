package metadata

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	"github.com/domainry/domainry-orm/query"
)

func (s DefinitionStore) ReplaceResource(ctx context.Context, snapshot metadatasdk.LocalizedTextResourceSnapshot) error {
	if err := s.validate(); err != nil {
		return err
	}
	snapshot.WorkspaceID = strings.TrimSpace(snapshot.WorkspaceID)
	snapshot.EntityType = strings.TrimSpace(snapshot.EntityType)
	snapshot.EntityKey = strings.TrimSpace(snapshot.EntityKey)
	snapshot.SourceKind = strings.TrimSpace(snapshot.SourceKind)
	snapshot.SourceID = strings.TrimSpace(snapshot.SourceID)
	if snapshot.WorkspaceID == "" || snapshot.EntityType == "" || snapshot.EntityKey == "" || snapshot.SourceKind == "" || snapshot.SourceID == "" {
		return &metadatasdk.Error{StatusCode: 400, Code: "metadata.localized_text_projection_identity_required"}
	}
	values := make([]metadatasdk.LocalizedText, 0, len(snapshot.Values))
	for _, value := range snapshot.Values {
		value = normalizeLocalizedText(value)
		value.WorkspaceID = snapshot.WorkspaceID
		value.EntityType = snapshot.EntityType
		value.EntityKey = snapshot.EntityKey
		value.SourceKind = snapshot.SourceKind
		value.SourceID = snapshot.SourceID
		if value.Property == "" || value.Locale == "" || value.Text == "" {
			return &metadatasdk.Error{StatusCode: 400, Code: "metadata.localized_text_invalid"}
		}
		values = append(values, value)
	}
	replace := func(executor modulehost.DBTX) error {
		return s.replaceLocalizedTextResource(ctx, executor, snapshot, values, time.Now().UTC().Format(time.RFC3339Nano))
	}
	if executor := modulehost.ExecutorFromContext(ctx, nil); executor != nil {
		return replace(executor)
	}
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := replace(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s DefinitionStore) replaceLocalizedTextResource(ctx context.Context, executor modulehost.DBTX, snapshot metadatasdk.LocalizedTextResourceSnapshot, values []metadatasdk.LocalizedText, now string) error {
	remove, args, err := query.NewWorkspaceDeleteBuilder(s.dialect, LocalizedTextTableName, snapshot.WorkspaceID).Where(query.And(
		query.Equal("entity_type", snapshot.EntityType), query.Equal("entity_key", snapshot.EntityKey), query.Equal("source_kind", snapshot.SourceKind),
	)).Build()
	if err != nil {
		return err
	}
	if _, err := executor.ExecContext(ctx, remove, args...); err != nil {
		return err
	}
	for _, value := range values {
		identity := localizedTextIdentity(value)
		lookup, lookupArgs, err := query.NewWorkspaceSelectBuilder(s.dialect, LocalizedTextTableName, snapshot.WorkspaceID).Columns("id").Where(identity).Build()
		if err != nil {
			return err
		}
		var existingID string
		lookupErr := executor.QueryRowContext(ctx, lookup, lookupArgs...).Scan(&existingID)
		if lookupErr != nil && lookupErr != sql.ErrNoRows {
			return lookupErr
		}
		if lookupErr == nil {
			update, updateArgs, err := query.NewWorkspaceUpdateBuilder(s.dialect, LocalizedTextTableName, snapshot.WorkspaceID).
				Set("text", value.Text).Set("source_kind", snapshot.SourceKind).Set("source_id", snapshot.SourceID).Set("updated_at", now).
				Where(identity).Build()
			if err != nil {
				return err
			}
			if _, err := executor.ExecContext(ctx, update, updateArgs...); err != nil {
				return err
			}
			continue
		}
		insert, insertArgs, err := query.NewWorkspaceInsertBuilder(s.dialect, LocalizedTextTableName, snapshot.WorkspaceID).Columns(
			"id", "entity_type", "entity_key", "property", "locale", "text", "source_kind", "source_id", "created_at", "updated_at",
		).Values(localizedTextID(value), value.EntityType, value.EntityKey, value.Property, value.Locale, value.Text, snapshot.SourceKind, snapshot.SourceID, now, now).Build()
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(ctx, insert, insertArgs...); err != nil {
			return err
		}
	}
	return nil
}

func (s DefinitionStore) syncLocalizedTextRows(ctx context.Context, executor modulehost.DBTX, sourceKind, sourceID string, values []metadatasdk.LocalizedText, now string) error {
	sourceKind, sourceID = strings.TrimSpace(sourceKind), strings.TrimSpace(sourceID)
	byWorkspace := map[string][]metadatasdk.LocalizedText{}
	for _, value := range values {
		value = normalizeLocalizedText(value)
		if value.WorkspaceID == "" || value.EntityType == "" || value.EntityKey == "" || value.Property == "" || value.Locale == "" || value.Text == "" {
			return &metadatasdk.Error{StatusCode: 400, Code: "metadata.localized_text_invalid"}
		}
		value.SourceKind, value.SourceID = sourceKind, sourceID
		byWorkspace[value.WorkspaceID] = append(byWorkspace[value.WorkspaceID], value)
	}
	workspaces := make([]string, 0, len(byWorkspace))
	for workspaceID := range byWorkspace {
		workspaces = append(workspaces, workspaceID)
	}
	sort.Strings(workspaces)
	for _, workspaceID := range workspaces {
		remove, args, err := query.NewWorkspaceDeleteBuilder(s.dialect, LocalizedTextTableName, workspaceID).Where(query.And(
			query.Equal("source_kind", sourceKind), query.Equal("source_id", sourceID),
		)).Build()
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(ctx, remove, args...); err != nil {
			return err
		}
		for _, value := range byWorkspace[workspaceID] {
			predicates := localizedTextIdentity(value)
			lookup, lookupArgs, err := query.NewWorkspaceSelectBuilder(s.dialect, LocalizedTextTableName, workspaceID).Columns("source_kind", "source_id").Where(predicates).Build()
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
				update, updateArgs, err := query.NewWorkspaceUpdateBuilder(s.dialect, LocalizedTextTableName, workspaceID).
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
			insert, insertArgs, err := query.NewWorkspaceInsertBuilder(s.dialect, LocalizedTextTableName, workspaceID).Columns(
				"id", "entity_type", "entity_key", "property", "locale", "text", "source_kind", "source_id", "created_at", "updated_at",
			).Values(localizedTextID(value), value.EntityType, value.EntityKey, value.Property, value.Locale, value.Text, sourceKind, sourceID, now, now).Build()
			if err != nil {
				return err
			}
			if _, err := executor.ExecContext(ctx, insert, insertArgs...); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s DefinitionStore) ListLocalizedTexts(ctx context.Context, queryValue metadatasdk.LocalizedTextQuery) ([]metadatasdk.LocalizedText, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	statement, args, err := s.localizedTextListStatement(queryValue)
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

func (s DefinitionStore) localizedTextListStatement(queryValue metadatasdk.LocalizedTextQuery) (string, []any, error) {
	queryValue.WorkspaceID = strings.TrimSpace(queryValue.WorkspaceID)
	if queryValue.WorkspaceID == "" {
		return "", nil, &metadatasdk.Error{StatusCode: 400, Code: "metadata.workspace_required"}
	}
	predicates := []query.Predicate{}
	add := func(column, value string) {
		if value = strings.TrimSpace(value); value != "" {
			predicates = append(predicates, query.Equal(column, value))
		}
	}
	add("entity_type", queryValue.EntityType)
	add("entity_key", queryValue.EntityKey)
	add("property", queryValue.Property)
	add("locale", queryValue.Locale)
	builder := query.NewWorkspaceSelectBuilder(s.dialect, LocalizedTextTableName, queryValue.WorkspaceID).Columns(
		"workspace_id", "entity_type", "entity_key", "property", "locale", "text", "source_kind", "source_id", "created_at", "updated_at",
	)
	if len(predicates) > 0 {
		builder.Where(query.And(predicates...))
	}
	return builder.OrderBy(query.Ascending("entity_type"), query.Ascending("entity_key"), query.Ascending("property"), query.Ascending("locale")).Build()
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
	return query.And(query.Equal("entity_type", value.EntityType), query.Equal("entity_key", value.EntityKey), query.Equal("property", value.Property), query.Equal("locale", value.Locale))
}

func localizedTextID(value metadatasdk.LocalizedText) string {
	sum := sha256.Sum256([]byte(value.WorkspaceID + "\x00" + value.EntityType + "\x00" + value.EntityKey + "\x00" + value.Property + "\x00" + value.Locale))
	return "localized:" + hex.EncodeToString(sum[:16])
}
