package mysql

import (
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	"strings"
)

// LocalizedTable keeps all five 255-character natural-key columns. Their
// utf8mb4 index would require 5100 bytes (MySQL allows 3072). ORM has no stored
// generated-column builder, so this adapter indexes a deterministic SHA-256 of
// the full, length-delimited key instead. No prefix uniqueness or key truncation
// is used. The generated value also protects writes made outside this service.
func LocalizedTable(d modulehost.Dialect, statement string) string {
	keys := []string{"workspace_id", "entity_type", "entity_key", "property", "locale"}
	quoted := make([]string, len(keys))
	parts := make([]string, len(keys))
	for i, key := range keys {
		quoted[i] = d.Identifier(key)
		parts[i] = "OCTET_LENGTH(" + quoted[i] + "), ':', " + quoted[i]
	}
	old := "UNIQUE (" + strings.Join(quoted, ", ") + ")"
	hash := d.Identifier("natural_key_sha256")
	generated := hash + " BINARY(32) GENERATED ALWAYS AS (UNHEX(SHA2(CONCAT(" + strings.Join(parts, ", ") + "), 256))) STORED, UNIQUE (" + hash + ")"
	return strings.Replace(statement, old, generated, 1)
}
