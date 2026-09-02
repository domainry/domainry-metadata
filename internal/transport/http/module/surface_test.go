package modulehttptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	actioncontract "github.com/domainry/domainry-foundation/action"
	"github.com/domainry/domainry-foundation/modulecapability"
	"github.com/domainry/domainry-foundation/modulehttp"
	identitysdk "github.com/domainry/domainry-identity-sdk"
	metadatasdk "github.com/domainry/domainry-metadata-sdk"
)

func TestSurfaceDeclaresMetadataOwnedReadRoutes(t *testing.T) {
	surface, err := NewSurface(testBinding{})
	if err != nil {
		t.Fatal(err)
	}
	if err := modulehttp.ValidateSurface(surface); err != nil {
		t.Fatalf("validate surface: %v", err)
	}
	patterns := make([]string, 0, len(surface.Routes()))
	for _, route := range surface.Routes() {
		patterns = append(patterns, route.Pattern())
		if route.Action.EffectClass != actioncontract.EffectRead || route.Action.IdempotencyDecision != "not_applicable" {
			t.Fatalf("route %q action=%#v", route.Pattern(), route.Action)
		}
	}
	want := []string{
		"GET /tenant-admin/metadata/definitions/{resourceType}",
		"GET /tenant-admin/metadata/definitions/{resourceType}/{resourceKey}",
		"GET /tenant-admin/metadata/localized-texts",
		"GET /tenant-admin/metadata/localized-texts/coverage",
		"GET /tenant-admin/metadata/localized-texts/export",
		"GET /tenant-admin/metadata/localized-texts/export.xlsx",
		"GET /dictionaries/{dictionaryKey}/items",
	}
	if !reflect.DeepEqual(patterns, want) {
		t.Fatalf("patterns=%#v", patterns)
	}
}

func TestSurfaceUsesAuthenticatedWorkspaceForDefinitionsAndDictionaries(t *testing.T) {
	definitions := &testDefinitions{values: []metadatasdk.Definition{{ResourceType: "object", ResourceKey: "account"}}}
	dictionaries := &testDictionaries{}
	surface, err := NewSurface(testBinding{definitions: definitions, dictionaries: dictionaries})
	if err != nil {
		t.Fatal(err)
	}
	principal := metadataTestPrincipal(metadatasdk.ActionMetadataDefinitionsList)

	request := httptest.NewRequest(http.MethodGet, "/tenant-admin/metadata/definitions/object?workspace_id=workspace-a", nil)
	request = request.WithContext(identitysdk.WithRequestIdentity(request.Context(), identitysdk.RequestIdentity{Principal: principal}))
	response := httptest.NewRecorder()
	surface.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || definitions.query.ResourceType != "object" {
		t.Fatalf("definition response=%d query=%#v body=%s", response.Code, definitions.query, response.Body.String())
	}
	if response.Header().Get("ETag") == "" {
		t.Fatal("definition response has no ETag")
	}

	request = httptest.NewRequest(http.MethodGet, "/dictionaries/status/items?locale=zh-CN", nil)
	request = request.WithContext(identitysdk.WithRequestIdentity(request.Context(), identitysdk.RequestIdentity{Principal: principal}))
	response = httptest.NewRecorder()
	surface.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || dictionaries.query.WorkspaceID != "workspace-a" || dictionaries.query.DictionaryKey != "status" || dictionaries.query.Locale != "zh-CN" {
		t.Fatalf("dictionary response=%d query=%#v body=%s", response.Code, dictionaries.query, response.Body.String())
	}
}

func TestSurfaceRejectsCrossWorkspaceDefinitionRead(t *testing.T) {
	definitions := &testDefinitions{}
	surface, err := NewSurface(testBinding{definitions: definitions})
	if err != nil {
		t.Fatal(err)
	}
	principal := metadataTestPrincipal(metadatasdk.ActionMetadataDefinitionsList)
	request := httptest.NewRequest(http.MethodGet, "/tenant-admin/metadata/definitions/object?workspace_id=workspace-b", nil)
	request = request.WithContext(identitysdk.WithRequestIdentity(request.Context(), identitysdk.RequestIdentity{Principal: principal}))
	response := httptest.NewRecorder()
	surface.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if definitions.listCalls != 0 {
		t.Fatalf("definition port called %d times", definitions.listCalls)
	}
}

func TestSurfaceHandlersRejectMissingPrincipalBeforeCallingBusinessPorts(t *testing.T) {
	definitions := &testDefinitions{}
	dictionaries := &testDictionaries{}
	surface, err := NewSurface(testBinding{definitions: definitions, dictionaries: dictionaries})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/tenant-admin/metadata/definitions/object/account", "/dictionaries/status/items"} {
		response := httptest.NewRecorder()
		surface.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusForbidden {
			t.Fatalf("path=%s status=%d body=%s", path, response.Code, response.Body.String())
		}
		for _, field := range []string{`"error":"auth.permission_denied"`, `"message_key":"auth.permission_denied"`, `"params":{}`} {
			if !strings.Contains(response.Body.String(), field) {
				t.Fatalf("path=%s error contract missing %s: %s", path, field, response.Body.String())
			}
		}
	}
	if definitions.getCalls != 0 {
		t.Fatalf("definition port called %d times", definitions.getCalls)
	}
	if dictionaries.calls != 0 {
		t.Fatalf("dictionary port called %d times", dictionaries.calls)
	}
}

func TestSurfaceDoesNotAuthorizeListWithAnotherExactMetadataPermission(t *testing.T) {
	definitions := &testDefinitions{}
	surface, err := NewSurface(testBinding{definitions: definitions})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/tenant-admin/metadata/definitions/object", nil)
	request = request.WithContext(identitysdk.WithRequestIdentity(request.Context(), identitysdk.RequestIdentity{
		Principal: metadataTestPrincipal(metadatasdk.ActionMetadataDefinitionsGet),
	}))
	response := httptest.NewRecorder()
	surface.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || definitions.listCalls != 0 {
		t.Fatalf("status=%d calls=%d body=%s", response.Code, definitions.listCalls, response.Body.String())
	}
}

func metadataTestPrincipal(actionKey string) identitysdk.Principal {
	separator := strings.LastIndex(actionKey, ".")
	return identitysdk.Principal{
		ContractVersion: identitysdk.PrincipalContextContractVersion,
		Known:           true, WorkspaceID: "workspace-a", UserID: "user-a",
		AccessBundle: &identitysdk.AccessBundle{FunctionGrants: []identitysdk.FunctionGrant{{
			Resource: identitysdk.ResourceType(actionKey[:separator]), Action: identitysdk.Action(actionKey[separator+1:]), Effect: identitysdk.EffectAllow,
		}}},
	}
}

type testBinding struct {
	modulecapability.Binding
	definitions  metadatasdk.Definitions
	localization metadatasdk.Localization
	dictionaries metadatasdk.Dictionaries
}

func (binding testBinding) Descriptor() metadatasdk.Descriptor {
	return metadatasdk.Descriptor{ProtocolVersion: metadatasdk.ProtocolVersionV1, Mode: metadatasdk.DeploymentModeModule, Capabilities: []string{"definitions", "localization", "dictionaries", "projection"}}
}

func (binding testBinding) Definitions() metadatasdk.Definitions {
	if binding.definitions != nil {
		return binding.definitions
	}
	return &testDefinitions{}
}

func (binding testBinding) Localization() metadatasdk.Localization {
	if binding.localization != nil {
		return binding.localization
	}
	return testLocalization{}
}

func (binding testBinding) Dictionaries() metadatasdk.Dictionaries {
	if binding.dictionaries != nil {
		return binding.dictionaries
	}
	return &testDictionaries{}
}

func (testBinding) Projection() metadatasdk.Projection { return testProjection{} }
func (testBinding) Close(context.Context) error        { return nil }

type testDefinitions struct {
	values    []metadatasdk.Definition
	query     metadatasdk.DefinitionQuery
	listCalls int
	getCalls  int
}

func (definitions *testDefinitions) List(_ context.Context, query metadatasdk.DefinitionQuery) ([]metadatasdk.Definition, error) {
	definitions.query = query
	definitions.listCalls++
	return definitions.values, nil
}

func (definitions *testDefinitions) Get(context.Context, string, string) (metadatasdk.Definition, bool, error) {
	definitions.getCalls++
	return metadatasdk.Definition{}, false, nil
}

func (*testDefinitions) Snapshot(context.Context) (metadatasdk.DefinitionSnapshot, error) {
	return metadatasdk.DefinitionSnapshot{}, nil
}

type testLocalization struct{}

func (testLocalization) List(context.Context, metadatasdk.LocalizedTextQuery) ([]metadatasdk.LocalizedText, error) {
	return nil, nil
}

func (testLocalization) Coverage(context.Context, metadatasdk.LocalizedTextCoverageQuery) (metadatasdk.LocalizedTextCoverage, error) {
	return metadatasdk.LocalizedTextCoverage{}, nil
}

type testDictionaries struct {
	query metadatasdk.DictionaryItemsQuery
	calls int
}

func (dictionaries *testDictionaries) Items(_ context.Context, query metadatasdk.DictionaryItemsQuery) (metadatasdk.DictionaryItems, error) {
	dictionaries.query = query
	dictionaries.calls++
	return metadatasdk.DictionaryItems{DictionaryKey: query.DictionaryKey, Locale: query.Locale}, nil
}

type testProjection struct{}

func (testProjection) Sync(context.Context, metadatasdk.ProjectionSnapshot) error { return nil }
