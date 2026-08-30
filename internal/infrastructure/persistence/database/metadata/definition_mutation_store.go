package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	metadatarepository "github.com/domainry/domainry-metadata-sdk/repository"
	ormbuilder "github.com/domainry/domainry-orm/builder"
)

func definitionTableForResourceType(resourceType string) (string, error) {
	table := definitionTableByResourceType[strings.TrimSpace(resourceType)]
	if table == "" {
		return "", fmt.Errorf("unsupported Metadata resource type %q", resourceType)
	}
	return table, nil
}

func (s DefinitionStore) GetDefinitionWithExecutor(ctx context.Context, executor metadatarepository.QueryExecutor, resourceType, key string) (metadatarepository.StoredDefinition, bool, error) {
	table, err := definitionTableForResourceType(resourceType)
	if err != nil {
		return metadatarepository.StoredDefinition{}, false, err
	}
	query, args, err := ormbuilder.NewSelectBuilder(s.dialect, table).Columns("resource_key", "object_key", "name", "payload_json", "schema_version", "schema_hash", "source_kind", "source_id", "disabled_at", "created_at", "updated_at").Where(ormbuilder.Equal("resource_key", strings.TrimSpace(key))).Build()
	if err != nil {
		return metadatarepository.StoredDefinition{}, false, err
	}
	rows, err := executor.QueryContext(ctx, query, args...)
	if err != nil {
		return metadatarepository.StoredDefinition{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return metadatarepository.StoredDefinition{}, false, rows.Err()
	}
	value, err := scanStoredDefinition(rows, resourceType)
	return value, err == nil, err
}

func (s DefinitionStore) ListDefinitionsWithExecutor(ctx context.Context, executor metadatarepository.QueryExecutor, resourceType, sourceID string) ([]metadatarepository.StoredDefinition, error) {
	table, err := definitionTableForResourceType(resourceType)
	if err != nil {
		return nil, err
	}
	predicates := []ormbuilder.Predicate{ormbuilder.IsNull("disabled_at")}
	if sourceID = strings.TrimSpace(sourceID); sourceID != "" {
		predicates = append(predicates, ormbuilder.Equal("source_id", sourceID))
	}
	query, args, err := ormbuilder.NewSelectBuilder(s.dialect, table).Columns("resource_key", "object_key", "name", "payload_json", "schema_version", "schema_hash", "source_kind", "source_id", "disabled_at", "created_at", "updated_at").Where(ormbuilder.And(predicates...)).OrderBy(ormbuilder.Ascending("resource_key")).Build()
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []metadatarepository.StoredDefinition{}
	for rows.Next() {
		value, scanErr := scanStoredDefinition(rows, resourceType)
		if scanErr != nil {
			return nil, scanErr
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

type rowScanner interface{ Scan(...any) error }

func scanStoredDefinition(row rowScanner, resourceType string) (metadatarepository.StoredDefinition, error) {
	value := metadatarepository.StoredDefinition{Definition: metadatarepository.Definition{ResourceType: strings.TrimSpace(resourceType)}}
	var payload string
	var disabled sql.NullString
	err := row.Scan(&value.Key, &value.ObjectKey, &value.Name, &payload, &value.SchemaVersion, &value.SchemaHash, &value.SourceKind, &value.SourceID, &disabled, &value.CreatedAt, &value.UpdatedAt)
	value.Payload = []byte(payload)
	if disabled.Valid {
		value.DisabledAt = disabled.String
	}
	return value, err
}

func (s DefinitionStore) ReplaceDefinitionWithExecutor(ctx context.Context, executor metadatarepository.ExecutionExecutor, value metadatarepository.StoredDefinition, expectedHash *string) (metadatarepository.ReplaceResult, error) {
	table, err := definitionTableForResourceType(value.ResourceType)
	if err != nil {
		return metadatarepository.ReplaceResult{}, err
	}
	key := strings.TrimSpace(value.Key)
	if expectedHash != nil && strings.TrimSpace(*expectedHash) == "" {
		current, found, lookupErr := s.GetDefinitionWithExecutor(ctx, executor, value.ResourceType, key)
		if lookupErr != nil {
			return metadatarepository.ReplaceResult{}, lookupErr
		}
		if found {
			return metadatarepository.ReplaceResult{CurrentHash: current.SchemaHash}, nil
		}
		expectedHash = nil
	}
	predicate := ormbuilder.Equal("resource_key", key)
	if expectedHash != nil {
		predicate = ormbuilder.And(predicate, ormbuilder.Equal("schema_hash", strings.TrimSpace(*expectedHash)))
	}
	deleteQuery, deleteArgs, err := ormbuilder.NewDeleteBuilder(s.dialect, table).Where(predicate).Build()
	if err != nil {
		return metadatarepository.ReplaceResult{}, err
	}
	result, err := executor.ExecContext(ctx, deleteQuery, deleteArgs...)
	if err != nil {
		return metadatarepository.ReplaceResult{}, err
	}
	if expectedHash != nil {
		affected, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return metadatarepository.ReplaceResult{}, rowsErr
		}
		if affected != 1 {
			current, found, lookupErr := s.GetDefinitionWithExecutor(ctx, executor, value.ResourceType, key)
			if lookupErr != nil {
				return metadatarepository.ReplaceResult{}, lookupErr
			}
			currentHash := ""
			if found {
				currentHash = current.SchemaHash
			}
			return metadatarepository.ReplaceResult{CurrentHash: currentHash}, nil
		}
	}
	insert, args, err := ormbuilder.NewInsertBuilder(s.dialect, table).Columns("id", "resource_key", "object_key", "name", "payload_json", "schema_version", "schema_hash", "source_kind", "source_id", "disabled_at", "created_at", "updated_at").Values(value.ResourceType+":"+key, key, value.ObjectKey, value.Name, value.Payload, value.SchemaVersion, value.SchemaHash, value.SourceKind, value.SourceID, nil, value.CreatedAt, value.UpdatedAt).Build()
	if err != nil {
		return metadatarepository.ReplaceResult{}, err
	}
	if _, err := executor.ExecContext(ctx, insert, args...); err != nil {
		return metadatarepository.ReplaceResult{}, err
	}
	return metadatarepository.ReplaceResult{Replaced: true}, nil
}

func (s DefinitionStore) DisableDefinitionWithExecutor(ctx context.Context, executor metadatarepository.ExecutionExecutor, resourceType, key, disabledAt string, expectedHash *string) (bool, error) {
	table, err := definitionTableForResourceType(resourceType)
	if err != nil {
		return false, err
	}
	predicates := []ormbuilder.Predicate{ormbuilder.Equal("resource_key", strings.TrimSpace(key)), ormbuilder.IsNull("disabled_at")}
	if expectedHash != nil {
		predicates = append(predicates, ormbuilder.Equal("schema_hash", strings.TrimSpace(*expectedHash)))
	}
	query, args, err := ormbuilder.NewUpdateBuilder(s.dialect, table).Set("disabled_at", disabledAt).Set("updated_at", disabledAt).Where(ormbuilder.And(predicates...)).Build()
	if err != nil {
		return false, err
	}
	result, err := executor.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

var _ metadatarepository.ExecutorDefinitionRepository = DefinitionStore{}
