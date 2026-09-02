package modulehttptransport

import (
	"strings"

	"github.com/domainry/domainry-foundation/modulecapability"
	metadatasdk "github.com/domainry/domainry-metadata-sdk"
)

func metadataOpenAPIOperationsByAction() map[string]map[string]any {
	security := []any{map[string]any{"BearerAuth": []any{}}}
	jsonResponse := func(description string, schema map[string]any) map[string]any {
		return map[string]any{"200": map[string]any{"description": description, "content": map[string]any{"application/json": map[string]any{"schema": schema}}}, "401": map[string]any{"description": "Authentication required"}, "403": map[string]any{"description": "Metadata access denied"}, "404": map[string]any{"description": "Metadata resource not found"}}
	}
	path := func(name string) map[string]any {
		return map[string]any{"name": name, "in": "path", "required": true, "schema": map[string]any{"type": "string", "minLength": 1}}
	}
	query := func(name string) map[string]any {
		return map[string]any{"name": name, "in": "query", "required": false, "schema": map[string]any{"type": "string"}}
	}
	definition := modulecapability.JSONSchemaForGoValue(metadatasdk.Definition{})
	localized := modulecapability.JSONSchemaForGoValue(metadatasdk.LocalizedText{})
	return map[string]map[string]any{
		metadatasdk.ActionMetadataDefinitionsList:          {"operationId": "listMetadataDefinitions", "tags": []string{"Metadata Definitions"}, "summary": "List projected definitions by source-owned resource type", "security": security, "parameters": []any{path("resourceType"), query("workspace_id")}, "responses": jsonResponse("Projected metadata definitions", map[string]any{"type": "object", "required": []string{"definitions", "resource_type"}, "properties": map[string]any{"definitions": map[string]any{"type": "array", "items": definition}, "resource_type": map[string]any{"type": "string"}}})},
		metadatasdk.ActionMetadataDefinitionsGet:           {"operationId": "getMetadataDefinition", "tags": []string{"Metadata Definitions"}, "summary": "Get one projected source-owned definition", "security": security, "parameters": []any{path("resourceType"), path("resourceKey")}, "responses": jsonResponse("Projected metadata definition", definition)},
		metadatasdk.ActionMetadataLocalizedTextsList:       {"operationId": "listMetadataLocalizedTexts", "tags": []string{"Metadata Localization"}, "summary": "List projected localized text", "security": security, "parameters": metadataLocalizationQueryParameters(query), "responses": jsonResponse("Localized text", map[string]any{"type": "object", "required": []string{"localized_texts"}, "properties": map[string]any{"localized_texts": map[string]any{"type": "array", "items": localized}}})},
		metadatasdk.ActionMetadataLocalizedTextsCoverage:   {"operationId": "getMetadataLocalizationCoverage", "tags": []string{"Metadata Localization"}, "summary": "Measure localization coverage with explicit fallback", "security": security, "parameters": []any{query("locale"), query("fallback_locale")}, "responses": jsonResponse("Localization coverage", modulecapability.JSONSchemaForGoValue(metadatasdk.LocalizedTextCoverage{}))},
		metadatasdk.ActionMetadataLocalizedTextsExportCSV:  metadataExportOperation("exportMetadataLocalizedTextsCSV", "text/csv", metadataLocalizationQueryParameters(query), security),
		metadatasdk.ActionMetadataLocalizedTextsExportXLSX: metadataExportOperation("exportMetadataLocalizedTextsXLSX", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", metadataLocalizationQueryParameters(query), security),
		metadatasdk.ActionMetadataDictionaryItemsList:      {"operationId": "listMetadataDictionaryItems", "tags": []string{"Metadata Dictionaries"}, "summary": "Resolve localized items from one projected dictionary", "security": security, "parameters": []any{path("dictionaryKey"), query("locale")}, "responses": jsonResponse("Localized dictionary items", modulecapability.JSONSchemaForGoValue(metadatasdk.DictionaryItems{}))},
	}
}

func metadataLocalizationQueryParameters(query func(string) map[string]any) []any {
	result := []any{}
	for _, name := range strings.Fields("workspace_id entity_type entity_key property locale") {
		result = append(result, query(name))
	}
	return result
}

func metadataExportOperation(operationID, contentType string, parameters []any, security []any) map[string]any {
	return map[string]any{"operationId": operationID, "tags": []string{"Metadata Localization"}, "summary": "Export projected localized text", "security": security, "parameters": parameters, "responses": map[string]any{"200": map[string]any{"description": "Localized text export", "content": map[string]any{contentType: map[string]any{"schema": map[string]any{"type": "string", "format": "binary"}}}}, "401": map[string]any{"description": "Authentication required"}, "403": map[string]any{"description": "Metadata access denied"}}}
}
