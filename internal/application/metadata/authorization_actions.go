package metadata

import (
	"fmt"
	"strings"

	actioncontract "github.com/domainry/domainry-foundation/action"
	metadatasdk "github.com/domainry/domainry-metadata-sdk"
)

const AuthorizationOwner = "module:metadata"

// AuthorizationActions is Metadata's complete source-owned executable
// manifest. Transport routes, typed client contracts and Permission
// reconciliation are projections of this set.
func AuthorizationActions() ([]actioncontract.ActionDefinition, error) {
	definitions := []actioncontract.ActionDefinition{
		metadataRoleAction(metadatasdk.ActionMetadataDefinitionsList, metadatasdk.CapabilityMetadataDefinitions, "Metadata definitions", "GET /metadata/definitions/{resourceType}", "List metadata definitions"),
		metadataRoleAction(metadatasdk.ActionMetadataDefinitionsGet, metadatasdk.CapabilityMetadataDefinitions, "Metadata definitions", "GET /metadata/definitions/{resourceType}/{resourceKey}", "Get metadata definition"),
		metadataRoleAction(metadatasdk.ActionMetadataLocalizedTextsList, metadatasdk.CapabilityMetadataLocalization, "Metadata localization", "GET /metadata/localized-texts", "List localized texts"),
		metadataRoleAction(metadatasdk.ActionMetadataLocalizedTextsCoverage, metadatasdk.CapabilityMetadataLocalization, "Metadata localization", "GET /metadata/localized-texts/coverage", "Read localization coverage"),
		metadataRoleAction(metadatasdk.ActionMetadataLocalizedTextsExportCSV, metadatasdk.CapabilityMetadataLocalization, "Metadata localization", "GET /metadata/localized-texts/export", "Export localized texts as CSV"),
		metadataRoleAction(metadatasdk.ActionMetadataLocalizedTextsExportXLSX, metadatasdk.CapabilityMetadataLocalization, "Metadata localization", "GET /metadata/localized-texts/export.xlsx", "Export localized texts as XLSX"),
		metadataAuthenticatedAction(metadatasdk.ActionMetadataDictionaryItemsList, metadatasdk.CapabilityMetadataDictionaries, "Metadata dictionaries", "GET /metadata/dictionaries/{dictionaryKey}/items", "List dictionary items"),
	}
	result := make([]actioncontract.ActionDefinition, 0, len(definitions))
	for _, definition := range definitions {
		normalized, err := actioncontract.NormalizeDefinition(definition)
		if err != nil {
			return nil, fmt.Errorf("normalize Metadata Action %q: %w", definition.Key, err)
		}
		result = append(result, normalized)
	}
	return result, nil
}

func metadataRoleAction(key, capabilityKey, capabilityLabel, pattern, label string) actioncontract.ActionDefinition {
	definition := metadataAction(key, capabilityKey, capabilityLabel, pattern, label)
	separator := strings.LastIndex(key, ".")
	definition.Authorization = actioncontract.Authorization{Strategy: actioncontract.AuthorizationAuthenticated}
	definition.Permission = &actioncontract.PermissionDefinition{
		Key: key, Owner: AuthorizationOwner, ResourceKey: key[:separator], OperationKey: key[separator+1:],
		Label: label, Category: capabilityLabel, LifecycleStatus: actioncontract.LifecycleActive,
	}
	return definition
}

func metadataAuthenticatedAction(key, capabilityKey, capabilityLabel, pattern, label string) actioncontract.ActionDefinition {
	definition := metadataAction(key, capabilityKey, capabilityLabel, pattern, label)
	definition.Exposures = []actioncontract.Exposure{actioncontract.ExposurePublic, actioncontract.ExposureManagement}
	definition.Authorization = actioncontract.Authorization{Strategy: actioncontract.AuthorizationAuthenticated}
	return definition
}

func metadataAction(key, capabilityKey, capabilityLabel, pattern, label string) actioncontract.ActionDefinition {
	method, path, _ := strings.Cut(strings.TrimSpace(pattern), " ")
	separator := strings.LastIndex(key, ".")
	return actioncontract.ActionDefinition{
		Key: key, Owner: AuthorizationOwner, SourceKind: "module_http",
		CapabilityKey: capabilityKey, CapabilityLabel: capabilityLabel,
		OperationKey: key[separator+1:], OperationLabel: label, Label: label,
		Exposures:   []actioncontract.Exposure{actioncontract.ExposureManagement},
		HTTP:        &actioncontract.HTTPBinding{Method: method, RouteTemplate: path},
		EffectClass: actioncontract.EffectRead, RiskLevel: actioncontract.RiskLow,
		IdempotencyDecision: "not_applicable", AuditClass: "owner_read_audit_policy",
		LifecycleStatus: actioncontract.LifecycleActive,
	}
}
