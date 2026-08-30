package metadata

import (
	"context"
	"strings"

	metadatarepository "github.com/domainry/domainry-metadata-sdk/repository"
	ormbuilder "github.com/domainry/domainry-orm/query"
)

func (s DefinitionStore) CountDefinitionVersionsWithExecutor(ctx context.Context, executor metadatarepository.QueryExecutor, resourceType, resourceKey string) (int, error) {
	query, args, err := ormbuilder.NewSelectBuilder(s.dialect, "metadata_definition_versions").Projections(ormbuilder.Project(ormbuilder.CountAll())).Where(ormbuilder.And(ormbuilder.Equal("resource_type", strings.TrimSpace(resourceType)), ormbuilder.Equal("resource_key", strings.TrimSpace(resourceKey)))).Build()
	if err != nil {
		return 0, err
	}
	rows, err := executor.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, rows.Err()
	}
	var count int
	return count, rows.Scan(&count)
}

func (s DefinitionStore) InsertDefinitionVersionWithExecutor(ctx context.Context, executor metadatarepository.ExecutionExecutor, value metadatarepository.DefinitionVersion) error {
	id := strings.TrimSpace(value.ResourceType) + ":version:" + strings.TrimSpace(value.ResourceKey) + ":" + strings.TrimSpace(value.SchemaVersion) + ":" + shortHash(value.SchemaHash)
	query, args, err := ormbuilder.NewInsertBuilder(s.dialect, "metadata_definition_versions").Columns("id", "resource_type", "resource_key", "schema_version", "schema_hash", "payload_json", "created_at").Values(id, value.ResourceType, value.ResourceKey, value.SchemaVersion, value.SchemaHash, value.Payload, value.CreatedAt).Build()
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, query, args...)
	return err
}

func (s DefinitionStore) ListDefinitionVersionsWithExecutor(ctx context.Context, executor metadatarepository.QueryExecutor, resourceType, resourceKey string) ([]metadatarepository.DefinitionVersion, error) {
	query, args, err := ormbuilder.NewSelectBuilder(s.dialect, "metadata_definition_versions").Columns("schema_version", "schema_hash", "payload_json", "created_at").Where(ormbuilder.And(ormbuilder.Equal("resource_type", resourceType), ormbuilder.Equal("resource_key", resourceKey))).OrderBy(ormbuilder.Descending("created_at")).Build()
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []metadatarepository.DefinitionVersion{}
	for rows.Next() {
		value := metadatarepository.DefinitionVersion{ResourceType: resourceType, ResourceKey: resourceKey}
		var payload string
		if err := rows.Scan(&value.SchemaVersion, &value.SchemaHash, &payload, &value.CreatedAt); err != nil {
			return nil, err
		}
		value.Payload = []byte(payload)
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s DefinitionStore) GetDefinitionVersionWithExecutor(ctx context.Context, executor metadatarepository.QueryExecutor, resourceType, resourceKey, schemaVersion string) (metadatarepository.DefinitionVersion, bool, error) {
	query, args, err := ormbuilder.NewSelectBuilder(s.dialect, "metadata_definition_versions").Columns("schema_hash", "payload_json", "created_at").Where(ormbuilder.And(ormbuilder.Equal("resource_type", resourceType), ormbuilder.Equal("resource_key", resourceKey), ormbuilder.Equal("schema_version", schemaVersion))).Build()
	if err != nil {
		return metadatarepository.DefinitionVersion{}, false, err
	}
	rows, err := executor.QueryContext(ctx, query, args...)
	if err != nil {
		return metadatarepository.DefinitionVersion{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return metadatarepository.DefinitionVersion{}, false, rows.Err()
	}
	value := metadatarepository.DefinitionVersion{ResourceType: resourceType, ResourceKey: resourceKey, SchemaVersion: schemaVersion}
	var payload string
	if err := rows.Scan(&value.SchemaHash, &payload, &value.CreatedAt); err != nil {
		return metadatarepository.DefinitionVersion{}, false, err
	}
	value.Payload = []byte(payload)
	return value, true, nil
}

func shortHash(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 12 {
		return value[:12]
	}
	return value
}
