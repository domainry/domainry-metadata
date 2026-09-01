package modulehttptransport

import (
	"encoding/json"
	"testing"

	"github.com/domainry/domainry-foundation/modulecapability"
	"github.com/domainry/domainry-foundation/modulecapability/contracttest"
)

func TestMetadataCapabilityTracksProjectionContractAndOwnerValidation(t *testing.T) {
	binding, err := NewCapabilityBinding(ValidateCapabilityCandidate)
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
	if operations != len(metadataRoutes()) || len(summary.Identity.SupportedDeploymentModes) != 1 || summary.Identity.SupportedDeploymentModes[0] != modulecapability.DeploymentModeModule {
		t.Fatalf("Metadata operations=%d topology=%v", operations, summary.Identity.SupportedDeploymentModes)
	}
	request := modulecapability.ValidationRequest{ContractVersion: modulecapability.ValidationContractVersion, ModuleKey: "metadata", CategoryKey: "metadata.dictionaries", ContractSHA256: summary.Identity.ContractSHA256, Kind: "metadata.dictionary", Candidate: modulecapability.AuthoringFragment{Collection: "dictionaries", Key: "status", Value: json.RawMessage(`{"key":"status","items":[{"key":"active"},{"key":"active"}]}`)}}
	result, err := binding.ValidateCapabilityCandidate(t.Context(), request)
	if err != nil || len(result.Diagnostics) != 1 || result.Diagnostics[0].RuleKey != "metadata.dictionary.item_key_invalid" {
		t.Fatalf("Metadata diagnostics=%+v err=%v", result.Diagnostics, err)
	}
}
