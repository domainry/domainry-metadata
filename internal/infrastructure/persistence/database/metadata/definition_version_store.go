package metadata

import (
	"context"
	"encoding/json"
	"strings"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	"github.com/domainry/domainry-orm/query"
)

type definitionVersion struct {
	ResourceType  string
	ResourceKey   string
	SchemaVersion string
	SchemaHash    string
	Payload       json.RawMessage
	CreatedAt     string
}

func (s DefinitionStore) insertDefinitionVersionIfMissing(ctx context.Context, executor modulehost.DBTX, value definitionVersion) error {
	hashes, err := s.definitionVersionHashes(ctx, executor, value)
	if err != nil {
		return err
	}
	if len(hashes) != 0 {
		return validateDefinitionVersionHashes(hashes, value.SchemaHash)
	}
	id := strings.TrimSpace(value.ResourceType) + ":version:" + strings.TrimSpace(value.ResourceKey) + ":" + strings.TrimSpace(value.SchemaVersion)
	statement, args, err := query.NewInsertBuilder(s.dialect, "_metadata_definition_versions").Columns(
		"id", "resource_type", "resource_key", "schema_version", "schema_hash", "payload_json", "created_at",
	).Values(id, value.ResourceType, value.ResourceKey, value.SchemaVersion, value.SchemaHash, value.Payload, value.CreatedAt).OnConflictDoNothing("id").Build()
	if err != nil {
		return err
	}
	result, err := executor.ExecContext(ctx, statement, args...)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted == 1 {
		return nil
	}
	hashes, err = s.definitionVersionHashes(ctx, executor, value)
	if err != nil {
		return err
	}
	if len(hashes) == 0 {
		return definitionVersionConflict()
	}
	return validateDefinitionVersionHashes(hashes, value.SchemaHash)
}

func (s DefinitionStore) definitionVersionHashes(ctx context.Context, executor modulehost.DBTX, value definitionVersion) ([]string, error) {
	statement, args, err := query.NewSelectBuilder(s.dialect, "_metadata_definition_versions").Columns("schema_hash").Where(query.And(
		query.Equal("resource_type", strings.TrimSpace(value.ResourceType)),
		query.Equal("resource_key", strings.TrimSpace(value.ResourceKey)),
		query.Equal("schema_version", strings.TrimSpace(value.SchemaVersion)),
	)).Build()
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hashes := []string{}
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, err
		}
		hashes = append(hashes, strings.TrimSpace(hash))
	}
	return hashes, rows.Err()
}

func validateDefinitionVersionHashes(hashes []string, expected string) error {
	expected = strings.TrimSpace(expected)
	for _, hash := range hashes {
		if strings.TrimSpace(hash) != expected {
			return definitionVersionConflict()
		}
	}
	return nil
}

func definitionVersionConflict() error {
	return &metadatasdk.Error{StatusCode: 409, Code: "backend.metadata.definition_version_conflict"}
}
