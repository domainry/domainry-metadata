package service

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
)

const DictionaryItemsCacheTTL = 5 * time.Second

type dictionaryCacheEntry struct {
	result    metadatasdk.DictionaryItems
	expiresAt time.Time
}

type DictionaryService struct {
	definitions  metadatasdk.Definitions
	localization metadatasdk.Localization
	mu           sync.Mutex
	cache        map[string]dictionaryCacheEntry
}

func NewDictionaryService(definitions metadatasdk.Definitions, localization metadatasdk.Localization) *DictionaryService {
	return &DictionaryService{definitions: definitions, localization: localization, cache: map[string]dictionaryCacheEntry{}}
}

func (s *DictionaryService) Items(ctx context.Context, query metadatasdk.DictionaryItemsQuery) (metadatasdk.DictionaryItems, error) {
	query.WorkspaceID = strings.TrimSpace(query.WorkspaceID)
	query.DictionaryKey = strings.TrimSpace(query.DictionaryKey)
	query.Locale = strings.TrimSpace(query.Locale)
	if query.WorkspaceID == "" {
		return metadatasdk.DictionaryItems{}, &metadatasdk.Error{StatusCode: 400, Code: "metadata.workspace_required"}
	}
	if query.DictionaryKey == "" {
		return metadatasdk.DictionaryItems{}, &metadatasdk.Error{StatusCode: 400, Code: "backend.dictionary.missing_key"}
	}
	definition, found, err := s.definitions.Get(ctx, "dictionary", query.DictionaryKey)
	if err != nil {
		return metadatasdk.DictionaryItems{}, err
	}
	if !found {
		return metadatasdk.DictionaryItems{}, &metadatasdk.Error{StatusCode: 404, Code: "backend.dictionary.not_found"}
	}
	var dictionary metadatasdk.Dictionary
	if err := json.Unmarshal(definition.Payload, &dictionary); err != nil {
		return metadatasdk.DictionaryItems{}, &metadatasdk.Error{StatusCode: 500, Code: "metadata.dictionary_definition_invalid", Cause: err}
	}
	version := dictionaryVersion(definition.SchemaHash, dictionary)
	cacheKey := query.WorkspaceID + "\x00" + query.DictionaryKey + "\x00" + query.Locale
	now := time.Now()
	s.mu.Lock()
	if cached, ok := s.cache[cacheKey]; ok && now.Before(cached.expiresAt) && cached.result.Version == version {
		result := cloneDictionaryItems(cached.result)
		result.Cached = true
		s.mu.Unlock()
		return result, nil
	}
	s.mu.Unlock()
	localized := map[string]metadatasdk.LocalizedText{}
	if query.Locale != "" {
		values, err := s.localization.List(ctx, metadatasdk.LocalizedTextQuery{WorkspaceID: query.WorkspaceID, EntityType: "dictionary_item", Locale: query.Locale})
		if err != nil {
			return metadatasdk.DictionaryItems{}, err
		}
		for _, value := range values {
			if strings.HasPrefix(value.EntityKey, query.DictionaryKey+".") {
				localized[value.EntityKey+"\x00"+value.Property] = value
			}
		}
	}
	items := localizeDictionaryItems(query.DictionaryKey, dictionaryItemsForLocale(dictionary.Items, query.Locale), localized)
	result := metadatasdk.DictionaryItems{DictionaryKey: query.DictionaryKey, Locale: query.Locale, Version: version, Items: items, CacheTTLMS: DictionaryItemsCacheTTL.Milliseconds()}
	s.mu.Lock()
	s.cache[cacheKey] = dictionaryCacheEntry{result: cloneDictionaryItems(result), expiresAt: now.Add(DictionaryItemsCacheTTL)}
	s.mu.Unlock()
	return result, nil
}

func dictionaryVersion(hash string, dictionary metadatasdk.Dictionary) int {
	value := strings.TrimSpace(hash)
	if value == "" {
		payload, _ := json.Marshal(dictionary)
		value = string(payload)
	}
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(value))
	return int(hasher.Sum32() & 0x7fffffff)
}

func dictionaryItemsForLocale(items []metadatasdk.DictionaryItem, locale string) []metadatasdk.DictionaryItem {
	defaults, localized := []metadatasdk.DictionaryItem{}, []metadatasdk.DictionaryItem{}
	for _, item := range items {
		if strings.TrimSpace(item.Locale) == "" {
			defaults = append(defaults, item)
		} else if locale != "" && strings.EqualFold(item.Locale, locale) {
			localized = append(localized, item)
		}
	}
	if len(defaults) == 0 && len(localized) == 0 {
		defaults = append(defaults, items...)
	}
	return mergeDictionaryItems(defaults, localized)
}

func mergeDictionaryItems(defaults, localized []metadatasdk.DictionaryItem) []metadatasdk.DictionaryItem {
	merged := map[string]metadatasdk.DictionaryItem{}
	for _, item := range defaults {
		item = normalizeDictionaryItem(item)
		if item.Key != "" {
			merged[item.Key] = item
		}
	}
	for _, item := range localized {
		item = cloneDictionaryItem(item)
		item.Key = strings.TrimSpace(item.Key)
		base, found := merged[item.Key]
		if found {
			if item.Description == "" {
				item.Description = base.Description
			}
			if item.Value == "" {
				item.Value = base.Value
			}
			if item.SortOrder == 0 {
				item.SortOrder = base.SortOrder
			}
			if item.Status == "" {
				item.Status = base.Status
			}
			if item.ParentKey == "" {
				item.ParentKey = base.ParentKey
			}
			if item.Color == "" {
				item.Color = base.Color
			}
			if item.Icon == "" {
				item.Icon = base.Icon
			}
			if len(item.Tags) == 0 {
				item.Tags = append([]string(nil), base.Tags...)
			}
			if len(item.UI) == 0 {
				item.UI = cloneMap(base.UI)
			}
			if len(item.Metadata) == 0 {
				item.Metadata = cloneMap(base.Metadata)
			}
		}
		item = normalizeDictionaryItem(item)
		if item.Key != "" {
			merged[item.Key] = item
		}
	}
	result := make([]metadatasdk.DictionaryItem, 0, len(merged))
	for _, item := range merged {
		if item.Status == "active" {
			result = append(result, item)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SortOrder == result[j].SortOrder {
			return result[i].Key < result[j].Key
		}
		return result[i].SortOrder < result[j].SortOrder
	})
	return result
}

func normalizeDictionaryItem(item metadatasdk.DictionaryItem) metadatasdk.DictionaryItem {
	item = cloneDictionaryItem(item)
	item.Key = strings.TrimSpace(item.Key)
	if strings.TrimSpace(item.Value) == "" {
		item.Value = item.Key
	}
	if strings.TrimSpace(item.Label) == "" {
		item.Label = item.Value
	}
	if strings.TrimSpace(item.Status) == "" {
		item.Status = "active"
	}
	if len(item.UI) == 0 {
		item.UI = cloneMap(item.Config)
	}
	if item.Color == "" {
		item.Color = mapString(item.UI, "color")
	}
	if item.Color == "" {
		item.Color = mapString(item.Config, "color")
	}
	if item.Icon == "" {
		item.Icon = mapString(item.UI, "icon")
	}
	if item.Icon == "" {
		item.Icon = mapString(item.Config, "icon")
	}
	return item
}

func localizeDictionaryItems(dictionaryKey string, items []metadatasdk.DictionaryItem, localized map[string]metadatasdk.LocalizedText) []metadatasdk.DictionaryItem {
	result := append([]metadatasdk.DictionaryItem(nil), items...)
	for index := range result {
		itemKey := strings.Trim(dictionaryKey+"."+firstNonEmpty(result[index].Key, result[index].Value), ".")
		if value := localized[itemKey+"\x00label"]; value.Text != "" {
			result[index].Label = value.Text
		}
		if value := localized[itemKey+"\x00description"]; value.Text != "" {
			result[index].Description = value.Text
		}
	}
	return result
}

func cloneDictionaryItems(result metadatasdk.DictionaryItems) metadatasdk.DictionaryItems {
	result.Items = append([]metadatasdk.DictionaryItem(nil), result.Items...)
	for index := range result.Items {
		result.Items[index] = cloneDictionaryItem(result.Items[index])
	}
	return result
}

func cloneDictionaryItem(item metadatasdk.DictionaryItem) metadatasdk.DictionaryItem {
	item.Tags = append([]string(nil), item.Tags...)
	item.Config, item.UI, item.Metadata = cloneMap(item.Config), cloneMap(item.UI), cloneMap(item.Metadata)
	return item
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func mapString(value map[string]any, key string) string {
	text, _ := value[key].(string)
	return strings.TrimSpace(text)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

var _ metadatasdk.Dictionaries = (*DictionaryService)(nil)
