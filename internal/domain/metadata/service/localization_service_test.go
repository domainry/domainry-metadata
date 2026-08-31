package service

import (
	"encoding/json"
	"testing"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
)

func TestLocalizedTextCoverageUsesDefinitionsForCompletelyMissingTranslations(t *testing.T) {
	definitions := []metadatasdk.Definition{
		{ResourceType: "object", ResourceKey: "customer", Name: "客户", Payload: json.RawMessage(`{"key":"customer","name":"客户","description":"客户主数据"}`)},
		{ResourceType: "field", ResourceKey: "customer.status", Name: "状态", Payload: json.RawMessage(`{"key":"status","name":"状态","options":[{"value":"active","label":"活跃"}]}`)},
		{ResourceType: "action", ResourceKey: "customer.activate", Name: "启用客户", Payload: json.RawMessage(`{"key":"customer.activate","label":"启用客户","payload_fields":[{"key":"reason","name":"原因"}]}`)},
		{ResourceType: "dictionary", ResourceKey: "customer_status", Name: "客户状态", Payload: json.RawMessage(`{"key":"customer_status","name":"客户状态","items":[{"key":"active","label":"活跃"}]}`)},
	}
	values := []metadatasdk.LocalizedText{
		{WorkspaceID: "workspace", Locale: "en-US", EntityType: "app", EntityKey: "app", Property: "name", Text: "Customer System", SourceKind: "generated", SourceID: "app"},
		{WorkspaceID: "workspace", Locale: "en-US", EntityType: "object", EntityKey: "customer", Property: "name", Text: "Customer"},
	}
	coverage := LocalizedTextCoverage(metadatasdk.LocalizedTextCoverageQuery{WorkspaceID: "workspace", Locale: "fr-FR", FallbackLocale: "en-US"}, "客户系统", definitions, values)
	if coverage.TotalCount != 11 || coverage.MissingCount != 11 {
		t.Fatalf("coverage counts=%d/%d items=%#v", coverage.MissingCount, coverage.TotalCount, coverage.Items)
	}
	assertCoverageItem(t, coverage.Items, "app", "app", "name", "Customer System", "fallback_locale")
	assertCoverageItem(t, coverage.Items, "object", "customer", "description", "客户主数据", "default_field")
	assertCoverageItem(t, coverage.Items, "field_option", "customer.status.active", "label", "活跃", "default_field")
	assertCoverageItem(t, coverage.Items, "action_payload_field", "customer.activate.reason", "name", "原因", "default_field")
}

func TestLocalizedTextCoverageUsesRequestedTranslationWithoutMarkingItMissing(t *testing.T) {
	definitions := []metadatasdk.Definition{{ResourceType: "object", ResourceKey: "customer", Name: "客户", Payload: json.RawMessage(`{"key":"customer","name":"客户"}`)}}
	values := []metadatasdk.LocalizedText{{WorkspaceID: "workspace", Locale: "fr-FR", EntityType: "object", EntityKey: "customer", Property: "name", Text: "Client", SourceKind: "user", SourceID: "translation"}}
	coverage := LocalizedTextCoverage(metadatasdk.LocalizedTextCoverageQuery{WorkspaceID: "workspace", Locale: "fr-FR"}, "", definitions, values)
	assertCoverageItem(t, coverage.Items, "object", "customer", "name", "Client", "requested_locale")
	for _, item := range coverage.Items {
		if item.EntityType == "object" && item.EntityKey == "customer" && item.Property == "name" && item.Missing {
			t.Fatalf("requested translation marked missing: %#v", item)
		}
	}
}

func assertCoverageItem(t *testing.T, items []metadatasdk.LocalizedTextCoverageItem, entityType, entityKey, property, text, source string) {
	t.Helper()
	for _, item := range items {
		if item.EntityType == entityType && item.EntityKey == entityKey && item.Property == property {
			if item.ResolvedText != text || item.ResolvedSource != source {
				t.Fatalf("coverage item=%#v want text=%q source=%q", item, text, source)
			}
			return
		}
	}
	t.Fatalf("coverage item %s/%s/%s is missing", entityType, entityKey, property)
}
