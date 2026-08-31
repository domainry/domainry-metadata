package metadata

import (
	"context"
	"encoding/json"
	"strings"

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
	id := strings.TrimSpace(value.ResourceType) + ":version:" + strings.TrimSpace(value.ResourceKey) + ":" + strings.TrimSpace(value.SchemaVersion) + ":" + hashPrefix(value.SchemaHash)
	statement, args, err := query.NewInsertBuilder(s.dialect, "_metadata_definition_versions").Columns(
		"id", "resource_type", "resource_key", "schema_version", "schema_hash", "payload_json", "created_at",
	).Values(id, value.ResourceType, value.ResourceKey, value.SchemaVersion, value.SchemaHash, value.Payload, value.CreatedAt).OnConflictDoNothing("id").Build()
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, statement, args...)
	return err
}

func hashPrefix(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
