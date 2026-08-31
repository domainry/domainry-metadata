package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetadataUsesInternalLayeredLayout(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	for _, required := range []string{
		"cmd/metadata-server",
		"internal/application/metadata",
		"internal/domain/metadata/service",
		"internal/adapter/metadatasdk",
		"internal/assembly/module",
		"internal/assembly/saas",
		"internal/transport/http/module",
		"internal/transport/http/saas",
		"internal/infrastructure/persistence/database/metadata",
		"internal/infrastructure/persistence/database/migration",
		"internal/infrastructure/persistence/database/schema",
		"internal/infrastructure/persistence/sqlite",
		"internal/infrastructure/persistence/mysql",
		"internal/infrastructure/persistence/postgres",
		"module",
	} {
		if info, err := os.Stat(filepath.Join(root, required)); err != nil || !info.IsDir() {
			t.Errorf("required Metadata boundary %q is missing", required)
		}
	}
	for _, forbidden := range []string{"application", "domain", "infrastructure", "server"} {
		if _, err := os.Stat(filepath.Join(root, forbidden)); !os.IsNotExist(err) {
			t.Errorf("Metadata implementation package %q must remain internal", forbidden)
		}
	}
}

func TestPublicModuleIsThinFacade(t *testing.T) {
	entries, err := os.ReadDir("../../module")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || entry.Name() == "module.go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		t.Errorf("public module package contains implementation file %q", entry.Name())
	}
}

func TestModuleUsesTaggedDependencies(t *testing.T) {
	content, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "replace ") || strings.Contains(string(content), "../domainry-") {
		t.Fatal("Metadata must consume released module tags, not local directory replacements")
	}
}
