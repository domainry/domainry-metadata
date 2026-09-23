package metadata

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	"github.com/domainry/domainry-orm/query"
)

type definitionVersion struct {
	DefinitionID   string
	InstallationID string
	Owner          string
	ResourceType   string
	ResourceKey    string
	SchemaVersion  string
	SchemaHash     string
	Payload        json.RawMessage
	CreatedAt      string
}

func (s DefinitionStore) ensureDefinitionVersion(ctx context.Context, executor modulehost.DBTX, value definitionVersion) (string, error) {
	if id, hash, found, err := s.definitionVersionByVersion(ctx, executor, value); err != nil {
		return "", err
	} else if found {
		if hash != strings.TrimSpace(value.SchemaHash) {
			return "", definitionVersionConflict()
		}
		return id, nil
	}
	if id, found, err := s.definitionVersionByHash(ctx, executor, value); err != nil {
		return "", err
	} else if found {
		return id, nil
	}
	id := definitionVersionID(value.DefinitionID, value.SchemaHash)
	statement, args, err := query.NewInsertBuilder(s.dialect, definitionVersionTableName).Columns(
		"id", "definition_id", "installation_id", "owner", "kind", "definition_key", "schema_version", "schema_hash", "payload_json", "created_at",
	).Values(
		id, value.DefinitionID, value.InstallationID, value.Owner, value.ResourceType, value.ResourceKey,
		value.SchemaVersion, value.SchemaHash, value.Payload, value.CreatedAt,
	).OnConflictDoNothing("id").Build()
	if err != nil {
		return "", err
	}
	if _, err := executor.ExecContext(ctx, statement, args...); err != nil {
		if existingID, existingHash, found, readErr := s.definitionVersionByVersion(ctx, executor, value); readErr == nil && found {
			if existingHash == strings.TrimSpace(value.SchemaHash) {
				return existingID, nil
			}
			return "", definitionVersionConflict()
		}
		return "", err
	}
	if existingID, existingHash, found, err := s.definitionVersionByVersion(ctx, executor, value); err != nil {
		return "", err
	} else if !found || existingHash != strings.TrimSpace(value.SchemaHash) {
		return "", definitionVersionConflict()
	} else {
		return existingID, nil
	}
}

func (s DefinitionStore) definitionVersionByVersion(ctx context.Context, executor modulehost.DBTX, value definitionVersion) (string, string, bool, error) {
	statement, args, err := query.NewSelectBuilder(s.dialect, definitionVersionTableName).Columns("id", "schema_hash").Where(query.And(
		query.Equal("definition_id", strings.TrimSpace(value.DefinitionID)),
		query.Equal("schema_version", strings.TrimSpace(value.SchemaVersion)),
	)).Build()
	if err != nil {
		return "", "", false, err
	}
	var id, hash string
	err = executor.QueryRowContext(ctx, statement, args...).Scan(&id, &hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", false, nil
		}
		return "", "", false, err
	}
	return strings.TrimSpace(id), strings.TrimSpace(hash), true, nil
}

func (s DefinitionStore) definitionVersionByHash(ctx context.Context, executor modulehost.DBTX, value definitionVersion) (string, bool, error) {
	statement, args, err := query.NewSelectBuilder(s.dialect, definitionVersionTableName).Columns("id").Where(query.And(
		query.Equal("definition_id", strings.TrimSpace(value.DefinitionID)),
		query.Equal("schema_hash", strings.TrimSpace(value.SchemaHash)),
	)).Build()
	if err != nil {
		return "", false, err
	}
	var id string
	err = executor.QueryRowContext(ctx, statement, args...).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}
	return strings.TrimSpace(id), true, nil
}

func definitionVersionID(definitionID, schemaHash string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(definitionID) + "\x00" + strings.TrimSpace(schemaHash)))
	return "definition-version:" + hex.EncodeToString(sum[:16])
}

func definitionVersionConflict() error {
	return &metadatasdk.Error{StatusCode: 409, Code: "backend.metadata.definition_version_conflict"}
}
