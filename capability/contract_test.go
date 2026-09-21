package capability

import (
	"testing"

	"github.com/domainry/domainry-foundation/modulecapability"
	"github.com/domainry/domainry-foundation/modulecapability/contracttest"
	metadatahttp "github.com/domainry/domainry-metadata/internal/transport/http/module"
)

func TestMetadataCapabilityTracksReadOnlyProjectionContract(t *testing.T) {
	binding, err := Open(Inputs{})
	if err != nil {
		t.Fatal(err)
	}
	contracttest.VerifyBinding(t, binding)
	summary, err := binding.CapabilitySummary(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	operations := 0
	for _, category := range summary.Categories {
		operations += category.OperationCount
	}
	routes, err := metadatahttp.CapabilityRoutes()
	if err != nil {
		t.Fatal(err)
	}
	if operations != len(routes) || len(summary.Identity.SupportedDeploymentModes) != 1 || summary.Identity.SupportedDeploymentModes[0] != modulecapability.DeploymentModeModule {
		t.Fatalf("Metadata operations=%d topology=%v", operations, summary.Identity.SupportedDeploymentModes)
	}
	if len(summary.Composition.ValidationScopes) != 0 {
		t.Fatalf("Metadata read-only projection exposes authoring scopes: %v", summary.Composition.ValidationScopes)
	}
}
