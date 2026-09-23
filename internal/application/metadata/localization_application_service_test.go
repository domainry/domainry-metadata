package metadata

import (
	"testing"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
)

func TestLatestApplicationDefinitionNameUsesLatestProjectionDefinition(t *testing.T) {
	definitions := []metadatasdk.Definition{
		{ResourceType: "application", ResourceKey: "projection:a", Name: "Older", UpdatedAt: "2026-09-22T01:00:00Z"},
		{ResourceType: "object", ResourceKey: "customer", Name: "Customer", UpdatedAt: "2026-09-22T03:00:00Z"},
		{ResourceType: "application", ResourceKey: "projection:b", Name: "Current", UpdatedAt: "2026-09-22T02:00:00Z"},
	}
	if got := latestApplicationDefinitionName(definitions); got != "Current" {
		t.Fatalf("application name=%q", got)
	}
}
