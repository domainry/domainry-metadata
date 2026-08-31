package service

import (
	"encoding/json"
	"sort"
	"strings"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
)

type localizedTextExpectedItem struct {
	entityType  string
	entityKey   string
	property    string
	defaultText string
}

// LocalizedTextCoverage projects the source-owned definition catalog and
// persisted translations into the Metadata coverage read model.
func LocalizedTextCoverage(query metadatasdk.LocalizedTextCoverageQuery, applicationName string, definitions []metadatasdk.Definition, values []metadatasdk.LocalizedText) metadatasdk.LocalizedTextCoverage {
	byLocale := make(map[string]metadatasdk.LocalizedText, len(values))
	for _, value := range values {
		byLocale[localizedTextLookupKey(value.Locale, value.EntityType, value.EntityKey, value.Property)] = value
	}
	expected := localizedTextExpectedItems(applicationName, definitions)
	items := make([]metadatasdk.LocalizedTextCoverageItem, 0, len(expected))
	missing := 0
	for _, expectedValue := range expected {
		requested := byLocale[localizedTextLookupKey(query.Locale, expectedValue.entityType, expectedValue.entityKey, expectedValue.property)]
		fallback := metadatasdk.LocalizedText{}
		if query.FallbackLocale != "" {
			fallback = byLocale[localizedTextLookupKey(query.FallbackLocale, expectedValue.entityType, expectedValue.entityKey, expectedValue.property)]
		}
		item := metadatasdk.LocalizedTextCoverageItem{
			WorkspaceID: query.WorkspaceID, EntityType: expectedValue.entityType, EntityKey: expectedValue.entityKey,
			Property: expectedValue.property, Locale: query.Locale, RequestedText: strings.TrimSpace(requested.Text),
			FallbackLocale: query.FallbackLocale, FallbackText: strings.TrimSpace(fallback.Text),
			DefaultText: expectedValue.defaultText, SourceKind: requested.SourceKind, SourceID: requested.SourceID,
		}
		switch {
		case item.RequestedText != "":
			item.ResolvedText, item.ResolvedSource = item.RequestedText, "requested_locale"
		case item.FallbackText != "":
			item.ResolvedText, item.ResolvedSource, item.Missing = item.FallbackText, "fallback_locale", true
		case item.DefaultText != "":
			item.ResolvedText, item.ResolvedSource, item.Missing = item.DefaultText, "default_field", true
		default:
			item.ResolvedText, item.ResolvedSource, item.Missing = humanizeMetadataKey(item.EntityKey), "humanized_key", true
		}
		if item.Missing {
			missing++
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return localizedTextSortKey(items[i]) < localizedTextSortKey(items[j]) })
	return metadatasdk.LocalizedTextCoverage{
		WorkspaceID: query.WorkspaceID, Locale: query.Locale, FallbackLocale: query.FallbackLocale,
		Items: items, MissingCount: missing, TotalCount: len(items),
	}
}

func localizedTextExpectedItems(applicationName string, definitions []metadatasdk.Definition) []localizedTextExpectedItem {
	result := []localizedTextExpectedItem{}
	add := func(entityType, entityKey, property, defaultText string) {
		entityKey, property = strings.TrimSpace(entityKey), strings.TrimSpace(property)
		if entityKey == "" {
			return
		}
		result = append(result, localizedTextExpectedItem{entityType: entityType, entityKey: entityKey, property: property, defaultText: strings.TrimSpace(defaultText)})
	}
	add("app", "app", "name", applicationName)
	for _, definition := range definitions {
		payload := map[string]any{}
		_ = json.Unmarshal(definition.Payload, &payload)
		name := firstNonEmpty(definition.Name, metadataString(payload, "name"))
		switch definition.ResourceType {
		case "object":
			add("object", definition.ResourceKey, "name", name)
			add("object", definition.ResourceKey, "description", metadataString(payload, "description"))
		case "field":
			add("field", definition.ResourceKey, "name", name)
			addValueOptionExpectedItems(&result, "field_option", definition.ResourceKey, payload["options"])
		case "validation":
			add("validation", definition.ResourceKey, "message", firstNonEmpty(definition.Name, metadataString(payload, "message")))
		case "action":
			add("action", definition.ResourceKey, "label", firstNonEmpty(definition.Name, metadataString(payload, "label")))
			for _, field := range metadataMaps(payload["payload_fields"]) {
				add("action_payload_field", strings.Trim(definition.ResourceKey+"."+metadataString(field, "key"), "."), "name", metadataString(field, "name"))
			}
		case "workflow":
			add("workflow", definition.ResourceKey, "name", name)
		case "dictionary":
			var dictionary metadatasdk.Dictionary
			if json.Unmarshal(definition.Payload, &dictionary) != nil {
				continue
			}
			add("dictionary", definition.ResourceKey, "name", firstNonEmpty(dictionary.Name, definition.Name))
			add("dictionary", definition.ResourceKey, "description", dictionary.Description)
			for _, item := range dictionary.Items {
				key := strings.Trim(definition.ResourceKey+"."+firstNonEmpty(item.Key, item.Value), ".")
				add("dictionary_item", key, "label", item.Label)
				add("dictionary_item", key, "description", item.Description)
			}
		case "report":
			add("report", definition.ResourceKey, "name", name)
		case "skill":
			add("skill", definition.ResourceKey, "name", name)
			add("skill", definition.ResourceKey, "description", metadataString(payload, "description"))
		case "agent":
			add("agent", definition.ResourceKey, "name", name)
			add("agent", definition.ResourceKey, "description", metadataString(payload, "description"))
		}
	}
	return result
}

func addValueOptionExpectedItems(result *[]localizedTextExpectedItem, entityType, parentKey string, value any) {
	for _, option := range metadataMaps(value) {
		key := firstNonEmpty(metadataString(option, "value"), metadataString(option, "key"))
		if key == "" {
			continue
		}
		entityKey := strings.Trim(parentKey+"."+key, ".")
		if label := metadataString(option, "label"); label != "" {
			*result = append(*result, localizedTextExpectedItem{entityType: entityType, entityKey: entityKey, property: "label", defaultText: label})
		}
		if description := metadataString(option, "description"); description != "" {
			*result = append(*result, localizedTextExpectedItem{entityType: entityType, entityKey: entityKey, property: "description", defaultText: description})
		}
	}
}

func metadataMaps(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		result := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				result = append(result, mapped)
			}
		}
		return result
	default:
		return nil
	}
}

func metadataString(value map[string]any, key string) string {
	if text, ok := value[key].(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func localizedTextLookupKey(locale, entityType, entityKey, property string) string {
	return strings.TrimSpace(locale) + "\x00" + strings.TrimSpace(entityType) + "\x00" + strings.TrimSpace(entityKey) + "\x00" + strings.TrimSpace(property)
}

func localizedTextSortKey(item metadatasdk.LocalizedTextCoverageItem) string {
	prefix := "1"
	if item.Missing {
		prefix = "0"
	}
	return prefix + "\x00" + item.EntityType + "\x00" + item.EntityKey + "\x00" + item.Property
}

func humanizeMetadataKey(value string) string {
	value = strings.NewReplacer("_", " ", ".", " ", "-", " ").Replace(strings.TrimSpace(value))
	return strings.Join(strings.Fields(value), " ")
}
