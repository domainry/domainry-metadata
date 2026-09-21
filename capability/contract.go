package capability

import (
	"encoding/json"
	"fmt"

	"github.com/domainry/domainry-foundation/modulecapability"
	"github.com/domainry/domainry-foundation/modulehttp"
	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	metadatahttp "github.com/domainry/domainry-metadata/internal/transport/http/module"
)

func openContract(_ Inputs) (*modulecapability.StaticBinding, error) {
	routes, err := metadatahttp.CapabilityRoutes()
	if err != nil {
		return nil, err
	}
	groups := map[string][]modulehttp.Route{}
	for _, route := range routes {
		groups[route.Action.CapabilityKey] = append(groups[route.Action.CapabilityKey], route)
	}
	byAction := metadatahttp.CapabilityOpenAPIOperationsByAction()
	operations := make(map[string]map[string]any, len(routes))
	for _, route := range routes {
		operation, found := byAction[route.Action.Key]
		if !found {
			return nil, fmt.Errorf("Metadata Action %q has no OpenAPI operation", route.Action.Key)
		}
		operations[route.Pattern()] = operation
		delete(byAction, route.Action.Key)
	}
	if len(byAction) != 0 {
		return nil, fmt.Errorf("Metadata OpenAPI operations have no Action manifest entries")
	}
	keys := []string{metadatasdk.CapabilityMetadataDefinitions, metadatasdk.CapabilityMetadataDictionaries, metadatasdk.CapabilityMetadataLocalization}
	documents := make([]modulecapability.CategoryDocument, 0, len(keys))
	for _, key := range keys {
		name, description := metadataCategoryMetadata(key)
		document, err := modulecapability.CategoryFromHTTPRoutes(modulecapability.HTTPRouteCategory{
			Owner: "metadata", Category: modulecapability.CategorySummary{Key: key, Name: name, Description: description, AssemblyChains: []string{"source_owner_projection_to_metadata_catalog"}},
			Routes: groups[key], Operations: operations,
			Components: map[string]map[string]json.RawMessage{"securitySchemes": {"BearerAuth": json.RawMessage(`{"type":"http","scheme":"bearer","bearerFormat":"JWT"}`)}},
		})
		if err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
	summary := modulecapability.ModuleSummary{
		Identity: modulecapability.ModuleIdentity{Key: "metadata", SourceOwner: "metadata", ModuleVersion: metadatasdk.ProtocolVersionV1, ValidationRevision: "metadata-projection-validation-v1", SupportedDeploymentModes: []modulecapability.DeploymentMode{modulecapability.DeploymentModeModule}},
		Name:     "Metadata", Description: "Read-only projection and localization catalog for source-owned definitions, dictionaries, and display text.",
		Composition: modulecapability.ModuleComposition{
			ProvidedCapabilities: []string{"metadata.definition_projection", "metadata.localization", "metadata.dictionary"}, RequiredModules: []string{"identity"}, OptionalModules: []string{}, ConflictingModules: []string{},
			AssemblyChains: []string{"source_owner_projection_to_metadata_catalog"}, ValidationScopes: []string{},
		},
	}
	return modulecapability.NewStaticBinding(summary, documents, nil)
}

func metadataCategoryMetadata(key string) (string, string) {
	switch key {
	case metadatasdk.CapabilityMetadataDictionaries:
		return "Metadata dictionaries", "Resolve localized items from one Runtime-authored, source-owned dictionary; Metadata is not a second dictionary authoring owner."
	case metadatasdk.CapabilityMetadataLocalization:
		return "Metadata localization", "List, measure coverage, and export projected localized text."
	default:
		return "Metadata definitions", "List and inspect source-owned Object, Field, Validation, Action, Workflow, Business Calendar, Automation, Dictionary, Integration mapping, Report, Profile binding, Skill, Agent, and Scheduler definitions with projection provenance."
	}
}
