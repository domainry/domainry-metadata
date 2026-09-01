package modulehttptransport

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/domainry/domainry-foundation/modulecapability"
	"github.com/domainry/domainry-foundation/modulehttp"
	metadatasdk "github.com/domainry/domainry-metadata-sdk"
)

func NewCapabilityBinding(validator modulecapability.Validator) (*modulecapability.StaticBinding, error) {
	routes, operations := metadataRoutes(), metadataOpenAPIOperations()
	groups := map[string][]modulehttp.Route{}
	for _, route := range routes {
		key := metadataCapabilityCategory(route.Pattern())
		groups[key] = append(groups[key], route)
	}
	keys := []string{"metadata.definitions", "metadata.dictionaries", "metadata.localization"}
	documents := make([]modulecapability.CategoryDocument, 0, len(keys))
	for _, key := range keys {
		name, description, scopes := metadataCategoryMetadata(key)
		document, err := modulecapability.CategoryFromHTTPRoutes(modulecapability.HTTPRouteCategory{
			Owner: "metadata", Category: modulecapability.CategorySummary{Key: key, Name: name, Description: description, AssemblyChains: []string{"source_owner_projection_to_metadata_catalog"}, ValidationScopes: scopes},
			Routes: groups[key], Operations: operations,
			Components: map[string]map[string]json.RawMessage{"securitySchemes": {"BearerAuth": json.RawMessage(`{"type":"http","scheme":"bearer","bearerFormat":"JWT"}`)}},
		})
		if err != nil {
			return nil, err
		}
		if key == "metadata.dictionaries" {
			document.ValidationContracts = []modulecapability.ValidationScopeContract{{Kind: "metadata.dictionary", Description: "Validate one source-owned metadata dictionary.", Coverage: modulecapability.ValidationCoverageAllCandidates, CandidateCollections: []string{"dictionaries"}}}
		}
		documents = append(documents, document)
	}
	summary := modulecapability.ModuleSummary{
		Identity: modulecapability.ModuleIdentity{Key: "metadata", SourceOwner: "metadata", ModuleVersion: metadatasdk.ProtocolVersionV1, ValidationRevision: "metadata-projection-validation-v1", SupportedDeploymentModes: []modulecapability.DeploymentMode{modulecapability.DeploymentModeModule}},
		Name:     "Metadata", Description: "Read-only projection and localization catalog for source-owned definitions, dictionaries, and display text.",
		Scenarios: modulecapability.AdaptationScenarios{
			UseWhen:              []string{"A PRD needs centrally discoverable source-owned definitions, localized display text, localization coverage, or reusable dictionary items"},
			DoNotUseWhen:         []string{"The requirement authors business rules or domain definitions owned by another module; Metadata projects those definitions but does not replace their owner"},
			RequirementSignals:   []string{"localized labels", "translation coverage", "dictionary items", "definition discovery", "metadata projection"},
			ProvidedCapabilities: []string{"metadata.definition_projection", "metadata.localization", "metadata.dictionary"}, RequiredModules: []string{"identity"}, OptionalModules: []string{}, ConflictingModules: []string{},
			AssemblyChains: []string{"source_owner_projection_to_metadata_catalog"}, ValidationScopes: []string{"metadata.dictionary"},
			SelectionExamples: []modulecapability.ScenarioExample{{Requirement: "Expose localized field labels and report missing French translations", Reason: "Metadata owns projected localized text and coverage"}},
			RejectionExamples: []modulecapability.ScenarioExample{{Requirement: "Define a scheduler retry policy", Reason: "Scheduler owns that domain definition; Metadata may only project it later"}},
		},
	}
	return modulecapability.NewStaticBinding(summary, documents, validator)
}

func metadataCapabilityCategory(pattern string) string {
	_, path, _ := strings.Cut(pattern, " ")
	if strings.HasPrefix(path, "/dictionaries/") {
		return "metadata.dictionaries"
	}
	if strings.Contains(path, "/localized-texts") {
		return "metadata.localization"
	}
	return "metadata.definitions"
}

func metadataCategoryMetadata(key string) (string, string, []string) {
	switch key {
	case "metadata.dictionaries":
		return "Metadata dictionaries", "Resolve localized items from one projected source-owned dictionary.", []string{"metadata.dictionary"}
	case "metadata.localization":
		return "Metadata localization", "List, measure coverage, and export projected localized text.", []string{}
	default:
		return "Metadata definitions", "List and inspect definitions projected from their semantic source owners.", []string{}
	}
}

func ValidateCapabilityCandidate(_ context.Context, request modulecapability.ValidationRequest) (modulecapability.ValidationResult, error) {
	result := modulecapability.ValidationResult{Diagnostics: []modulecapability.Diagnostic{}}
	invalid := func(rule, path, message string) (modulecapability.ValidationResult, error) {
		result.Diagnostics = append(result.Diagnostics, modulecapability.Diagnostic{Owner: "metadata", RuleKey: rule, Severity: modulecapability.SeverityError, FieldPath: path, Message: message})
		return result, nil
	}
	switch request.Kind {
	case "metadata.dictionary":
		var value metadatasdk.Dictionary
		if err := modulecapability.DecodeKeyedAuthoringValue(request.Candidate, "key", &value); err != nil {
			return invalid("metadata.dictionary.invalid_json", "$.candidate.value", err.Error())
		}
		if strings.TrimSpace(value.Key) == "" || strings.TrimSpace(value.Key) != request.Candidate.Key {
			return invalid("metadata.dictionary.key_required", "$.candidate.value.key", "dictionary value key must equal the source fragment key")
		}
		seen := map[string]bool{}
		for index, item := range value.Items {
			key := strings.TrimSpace(item.Key)
			if key == "" || seen[key] {
				return invalid("metadata.dictionary.item_key_invalid", fmt.Sprintf("$.candidate.value.items[%d].key", index), "dictionary item key is required and must be unique")
			}
			seen[key] = true
		}
	default:
		return modulecapability.ValidationResult{}, &modulecapability.Error{StatusCode: 400, Code: "module_capability.validation_scope_invalid"}
	}
	return result, nil
}
