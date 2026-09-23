package service

import (
	"context"
	"encoding/json"
	"testing"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
)

type dictionaryDefinitionsStub struct {
	definition metadatasdk.Definition
	found      bool
}

func (s dictionaryDefinitionsStub) List(context.Context, metadatasdk.DefinitionQuery) ([]metadatasdk.Definition, error) {
	return nil, nil
}
func (s dictionaryDefinitionsStub) Get(context.Context, string, string, string) (metadatasdk.Definition, bool, error) {
	return s.definition, s.found, nil
}
func (s dictionaryDefinitionsStub) Snapshot(context.Context, metadatasdk.DefinitionQuery) (metadatasdk.DefinitionSnapshot, error) {
	return metadatasdk.DefinitionSnapshot{}, nil
}

type dictionaryLocalizationStub struct{ calls int }

func (s *dictionaryLocalizationStub) List(_ context.Context, query metadatasdk.LocalizedTextQuery) ([]metadatasdk.LocalizedText, error) {
	s.calls++
	return []metadatasdk.LocalizedText{{
		WorkspaceID: query.WorkspaceID, EntityType: "dictionary_item", EntityKey: "status.active",
		Property: "label", Locale: query.Locale, Text: query.WorkspaceID + " active",
	}}, nil
}
func (*dictionaryLocalizationStub) Coverage(context.Context, metadatasdk.LocalizedTextCoverageQuery) (metadatasdk.LocalizedTextCoverage, error) {
	return metadatasdk.LocalizedTextCoverage{}, nil
}

func TestDictionaryItemsResolveLocaleAndKeepWorkspaceCachesIsolated(t *testing.T) {
	payload, err := json.Marshal(metadatasdk.Dictionary{Key: "status", Items: []metadatasdk.DictionaryItem{
		{Key: "active", Label: "Active", SortOrder: 2},
		{Key: "inactive", Label: "Inactive", SortOrder: 1, Status: "disabled"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	localization := &dictionaryLocalizationStub{}
	service := NewDictionaryService(dictionaryDefinitionsStub{definition: metadatasdk.Definition{ResourceType: "dictionary", ResourceKey: "status", Payload: payload, SchemaHash: "schema"}, found: true}, localization)
	first, err := service.Items(t.Context(), metadatasdk.DictionaryItemsQuery{WorkspaceID: "workspace-a", DictionaryKey: "status", Locale: "en-US"})
	if err != nil || len(first.Items) != 1 || first.Items[0].Label != "workspace-a active" || first.Cached {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	cached, err := service.Items(t.Context(), metadatasdk.DictionaryItemsQuery{WorkspaceID: "workspace-a", DictionaryKey: "status", Locale: "en-US"})
	if err != nil || !cached.Cached || localization.calls != 1 {
		t.Fatalf("cached=%#v calls=%d err=%v", cached, localization.calls, err)
	}
	secondWorkspace, err := service.Items(t.Context(), metadatasdk.DictionaryItemsQuery{WorkspaceID: "workspace-b", DictionaryKey: "status", Locale: "en-US"})
	if err != nil || secondWorkspace.Cached || secondWorkspace.Items[0].Label != "workspace-b active" || localization.calls != 2 {
		t.Fatalf("workspace-b=%#v calls=%d err=%v", secondWorkspace, localization.calls, err)
	}
}

func TestDictionaryItemsRejectInvalidOrUnknownKeys(t *testing.T) {
	service := NewDictionaryService(dictionaryDefinitionsStub{}, &dictionaryLocalizationStub{})
	for name, query := range map[string]metadatasdk.DictionaryItemsQuery{
		"workspace":  {DictionaryKey: "status"},
		"dictionary": {WorkspaceID: "workspace"},
		"not_found":  {WorkspaceID: "workspace", DictionaryKey: "status"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := service.Items(t.Context(), query); err == nil {
				t.Fatal("invalid dictionary query was accepted")
			}
		})
	}
}

func TestMergeDictionaryItemsPreservesEstablishedLocaleOverrideSemantics(t *testing.T) {
	items := mergeDictionaryItems(
		[]metadatasdk.DictionaryItem{{Key: "active", Label: "Active", Description: "Default", Value: "1", SortOrder: 3, Config: map[string]any{"owner": "default"}}},
		[]metadatasdk.DictionaryItem{{Key: "active", Label: "Aktiv"}},
	)
	if len(items) != 1 {
		t.Fatalf("items=%#v", items)
	}
	item := items[0]
	if item.Label != "Aktiv" || item.Description != "Default" || item.Value != "1" || item.SortOrder != 3 {
		t.Fatalf("merged item=%#v", item)
	}
	if item.Config != nil {
		t.Fatalf("localized Config must not inherit from the default item: %#v", item.Config)
	}
}
