package module_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/domainry/domainry-foundation/modulecapability"
	"github.com/domainry/domainry-foundation/modulehttp"
	identitysdk "github.com/domainry/domainry-identity-sdk"
	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	"github.com/domainry/domainry-metadata-sdk/modulehost"
	metadatamodule "github.com/domainry/domainry-metadata/module"
	ormdialect "github.com/domainry/domainry-orm/dialect"
	ormmigration "github.com/domainry/domainry-orm/migration"
	_ "modernc.org/sqlite"
)

type integrationHost struct {
	database  *sql.DB
	dialect   modulehost.Dialect
	registrar *integrationMigrationRegistrar
}

func (h integrationHost) Database() modulehost.Database             { return h.database }
func (h integrationHost) Dialect() modulehost.Dialect               { return h.dialect }
func (h integrationHost) Migrations() modulehost.MigrationRegistrar { return h.registrar }

type integrationMigrationRegistrar struct {
	runner *ormmigration.Runner
	mu     sync.Mutex
	owners []string
}

func (*integrationMigrationRegistrar) Driver() string { return "sqlite" }
func (*integrationMigrationRegistrar) Schema() string { return "" }
func (r *integrationMigrationRegistrar) ApplyOwnedMigrations(ctx context.Context, owner string, migrations []modulehost.SchemaMigration) error {
	if owner != "metadata" {
		return fmt.Errorf("unexpected migration owner %q", owner)
	}
	r.mu.Lock()
	r.owners = append(r.owners, owner)
	r.mu.Unlock()
	return r.runner.Apply(ctx, migrations)
}

func (r *integrationMigrationRegistrar) appliedOwners() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.owners...)
}

func openIntegrationModule(t *testing.T, path string) (*sql.DB, metadatasdk.Binding, *integrationMigrationRegistrar) {
	t.Helper()
	dsn := path + "?_pragma=busy_timeout%285000%29&_pragma=foreign_keys%281%29"
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(8)
	if err := database.PingContext(t.Context()); err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	dialect, err := ormdialect.New(ormdialect.SQLite)
	if err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	renderer := dialect.WithSchema("")
	runner, err := ormmigration.NewRunner(database, renderer, ormmigration.Options{})
	if err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	registrar := &integrationMigrationRegistrar{runner: runner}
	binding, err := metadatamodule.NewFactory().OpenModule(t.Context(), metadatasdk.ApplicationRef{InstallationID: "integration-installation"}, integrationHost{
		database: database, dialect: renderer, registrar: registrar,
	})
	if err != nil {
		_ = database.Close()
		t.Fatal(err)
	}
	return database, binding, registrar
}

func projectionSnapshot(version, objectName string) metadatasdk.ProjectionSnapshot {
	return metadatasdk.ProjectionSnapshot{
		SchemaVersion: version,
		SourceKind:    "generated",
		SourceID:      "application-manifest",
		Name:          "Integration application",
		DefaultLocale: "en-US",
		Definitions: []metadatasdk.Definition{
			{ResourceType: "object", ResourceKey: "customer", ObjectKey: "customer", Name: objectName, Payload: json.RawMessage(`{"key":"customer","name":"` + objectName + `"}`)},
			{ResourceType: "dictionary", ResourceKey: "status", Name: "Status", Payload: json.RawMessage(`{"key":"status","items":[{"key":"active","label":"Active","status":"active"}]}`)},
		},
		LocalizedText: []metadatasdk.LocalizedText{
			{WorkspaceID: "workspace-a", EntityType: "object", EntityKey: "customer", Property: "name", Locale: "zh-CN", Text: "客户"},
			{WorkspaceID: "workspace-a", EntityType: "dictionary_item", EntityKey: "status.active", Property: "label", Locale: "zh-CN", Text: "启用"},
			{WorkspaceID: "workspace-b", EntityType: "object", EntityKey: "customer", Property: "name", Locale: "zh-CN", Text: "客户 B"},
		},
	}
}

func TestPublicModuleProjectionRoundTripTransactionReplayAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata.db")
	database, binding, registrar := openIntegrationModule(t, path)
	if owners := registrar.appliedOwners(); len(owners) != 1 || owners[0] != "metadata" {
		t.Fatalf("migration owners=%v", owners)
	}

	transaction, err := database.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	transactionContext := modulehost.WithExecutor(t.Context(), transaction)
	rolledBack := projectionSnapshot("rollback-version", "Rolled back customer")
	if err := binding.Projection().Sync(transactionContext, rolledBack); err != nil {
		_ = transaction.Rollback()
		t.Fatal(err)
	}
	if value, found, err := binding.Definitions().Get(transactionContext, "object", "customer"); err != nil || !found || value.SchemaVersion != "rollback-version" {
		_ = transaction.Rollback()
		t.Fatalf("transaction definition=%#v found=%v err=%v", value, found, err)
	}
	if err := transaction.Rollback(); err != nil {
		t.Fatal(err)
	}
	if value, found, err := binding.Definitions().Get(t.Context(), "object", "customer"); err != nil || found {
		t.Fatalf("rolled-back definition=%#v found=%v err=%v", value, found, err)
	}

	versionOne := projectionSnapshot("1", "Customer")
	if err := binding.Projection().Sync(t.Context(), versionOne); err != nil {
		t.Fatal(err)
	}
	if err := binding.Projection().Sync(t.Context(), versionOne); err != nil {
		t.Fatalf("replay projection: %v", err)
	}
	versionTwo := projectionSnapshot("2", "Customer account")
	if err := binding.Projection().Sync(t.Context(), versionTwo); err != nil {
		t.Fatal(err)
	}
	err = binding.Projection().Sync(t.Context(), projectionSnapshot("1", "Rewritten customer"))
	var versionConflict *metadatasdk.Error
	if !errors.As(err, &versionConflict) || versionConflict.StatusCode != http.StatusConflict || versionConflict.Code != "backend.metadata.definition_version_conflict" {
		t.Fatalf("historical version rewrite error=%v", err)
	}
	definition, found, err := binding.Definitions().Get(t.Context(), "object", "customer")
	if err != nil || !found || definition.SchemaVersion != "2" || definition.Name != "Customer account" || !strings.Contains(string(definition.Payload), "Customer account") {
		t.Fatalf("published definition=%#v found=%v err=%v", definition, found, err)
	}
	definitions, err := binding.Definitions().List(t.Context(), metadatasdk.DefinitionQuery{ResourceType: "object"})
	if err != nil || len(definitions) != 1 || definitions[0].ResourceKey != "customer" {
		t.Fatalf("definitions=%#v err=%v", definitions, err)
	}
	items, err := binding.Dictionaries().Items(t.Context(), metadatasdk.DictionaryItemsQuery{WorkspaceID: "workspace-a", DictionaryKey: "status", Locale: "zh-CN"})
	if err != nil || len(items.Items) != 1 || items.Items[0].Label != "启用" {
		t.Fatalf("dictionary items=%#v err=%v", items, err)
	}
	var objectVersions int
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM _metadata_definition_versions WHERE resource_type = ? AND resource_key = ?`, "object", "customer").Scan(&objectVersions); err != nil || objectVersions != 2 {
		t.Fatalf("object versions=%d err=%v", objectVersions, err)
	}
	if err := binding.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	reopenedDatabase, reopened, reopenedRegistrar := openIntegrationModule(t, path)
	defer reopenedDatabase.Close()
	defer reopened.Close(t.Context())
	if owners := reopenedRegistrar.appliedOwners(); len(owners) != 1 || owners[0] != "metadata" {
		t.Fatalf("reopened migration owners=%v", owners)
	}
	definition, found, err = reopened.Definitions().Get(t.Context(), "object", "customer")
	if err != nil || !found || definition.SchemaVersion != "2" || definition.Name != "Customer account" {
		t.Fatalf("reopened definition=%#v found=%v err=%v", definition, found, err)
	}
	var ledgerRows int
	if err := reopenedDatabase.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM _schema_migrations`).Scan(&ledgerRows); err != nil || ledgerRows != 1 {
		t.Fatalf("host migration ledger rows=%d err=%v", ledgerRows, err)
	}
	var ledgers int
	if err := reopenedDatabase.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name LIKE '%schema_migrations%'`).Scan(&ledgers); err != nil || ledgers != 1 {
		t.Fatalf("migration ledger tables=%d err=%v", ledgers, err)
	}
	for _, table := range metadatamodule.OwnedTables() {
		var count int
		if err := reopenedDatabase.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("owned table=%s count=%d err=%v", table, count, err)
		}
	}
}

func TestPublicModuleRejectsConcurrentDefinitionVersionConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata-concurrency.db")
	database, binding, _ := openIntegrationModule(t, path)
	defer database.Close()
	defer binding.Close(t.Context())
	if err := binding.Projection().Sync(t.Context(), projectionSnapshot("1", "Customer")); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errorsByCandidate := make([]error, 2)
	candidates := []metadatasdk.ProjectionSnapshot{
		projectionSnapshot("2", "Customer from publisher A"),
		projectionSnapshot("2", "Customer from publisher B"),
	}
	var wait sync.WaitGroup
	wait.Add(len(candidates))
	for index := range candidates {
		go func(index int) {
			defer wait.Done()
			<-start
			errorsByCandidate[index] = binding.Projection().Sync(t.Context(), candidates[index])
		}(index)
	}
	close(start)
	wait.Wait()

	successes, conflicts := 0, 0
	for _, err := range errorsByCandidate {
		if err == nil {
			successes++
			continue
		}
		var metadataError *metadatasdk.Error
		if errors.As(err, &metadataError) && metadataError.StatusCode == http.StatusConflict && metadataError.Code == "backend.metadata.definition_version_conflict" {
			conflicts++
			continue
		}
		t.Fatalf("unexpected concurrent publication error: %v", err)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("publication outcomes successes=%d conflicts=%d errors=%v", successes, conflicts, errorsByCandidate)
	}
	definition, found, err := binding.Definitions().Get(t.Context(), "object", "customer")
	if err != nil || !found || definition.SchemaVersion != "2" {
		t.Fatalf("winning definition=%#v found=%v err=%v", definition, found, err)
	}
	var versions int
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM _metadata_definition_versions WHERE resource_type = ? AND resource_key = ?`, "object", "customer").Scan(&versions); err != nil || versions != 2 {
		t.Fatalf("definition versions=%d err=%v", versions, err)
	}
}

func TestPublicModuleCapabilityRejectsInvalidCandidate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata-capability.db")
	database, binding, _ := openIntegrationModule(t, path)
	defer database.Close()
	defer binding.Close(t.Context())
	summary, err := binding.CapabilitySummary(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	result, err := binding.ValidateCapabilityCandidate(t.Context(), modulecapability.ValidationRequest{
		ContractVersion: modulecapability.ValidationContractVersion,
		ModuleKey:       "metadata",
		CategoryKey:     metadatasdk.CapabilityMetadataDictionaries,
		ContractSHA256:  summary.Identity.ContractSHA256,
		Kind:            "metadata.dictionary",
		Candidate: modulecapability.AuthoringFragment{
			Collection: "dictionaries",
			Key:        "status",
			Value:      json.RawMessage(`{"key":"status","items":[{"key":"active"},{"key":"active"}]}`),
		},
	})
	if err != nil || len(result.Diagnostics) != 1 || result.Diagnostics[0].RuleKey != "metadata.dictionary.item_key_invalid" || result.Diagnostics[0].Severity != modulecapability.SeverityError {
		t.Fatalf("diagnostics=%#v err=%v", result.Diagnostics, err)
	}
}

func TestPublicHTTPAdapterEnforcesWorkspaceAndExactAction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata-authorization.db")
	database, binding, _ := openIntegrationModule(t, path)
	defer database.Close()
	defer binding.Close(t.Context())
	if err := binding.Projection().Sync(t.Context(), projectionSnapshot("1", "Customer")); err != nil {
		t.Fatal(err)
	}
	provider, ok := binding.(modulehttp.Provider)
	if !ok {
		t.Fatalf("HTTP provider unavailable on %T", binding)
	}
	if len(provider.HTTPAdapters()) != 1 {
		t.Fatalf("HTTP adapters=%d", len(provider.HTTPAdapters()))
	}
	handler := provider.HTTPAdapters()[0].Handler()

	response := serveAuthorized(handler, "/metadata/definitions/object?workspace_id=workspace-a", exactActionPrincipal("workspace-a", "user-a", metadatasdk.ActionMetadataDefinitionsList))
	if response.Code != http.StatusOK {
		t.Fatalf("authorized definition status=%d body=%s", response.Code, response.Body.String())
	}
	response = serveAuthorized(handler, "/metadata/definitions/object?workspace_id=workspace-a", exactActionPrincipal("workspace-a", "user-a", metadatasdk.ActionMetadataDefinitionsGet))
	if response.Code != http.StatusForbidden {
		t.Fatalf("wrong Action status=%d body=%s", response.Code, response.Body.String())
	}
	response = serveAuthorized(handler, "/metadata/definitions/object?workspace_id=workspace-b", exactActionPrincipal("workspace-a", "user-a", metadatasdk.ActionMetadataDefinitionsList))
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-workspace definition status=%d body=%s", response.Code, response.Body.String())
	}

	response = serveAuthorized(handler, "/metadata/localized-texts?workspace_id=workspace-a&locale=zh-CN", exactActionPrincipal("workspace-a", "user-a", metadatasdk.ActionMetadataLocalizedTextsList))
	if response.Code != http.StatusOK {
		t.Fatalf("localized-text status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		LocalizedTexts []metadatasdk.LocalizedText `json:"localized_texts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.LocalizedTexts) != 2 {
		t.Fatalf("workspace-a localized texts=%#v", payload.LocalizedTexts)
	}
	for _, value := range payload.LocalizedTexts {
		if value.WorkspaceID != "workspace-a" {
			t.Fatalf("cross-workspace localized text leaked: %#v", value)
		}
	}
}

func serveAuthorized(handler http.Handler, path string, principal identitysdk.Principal) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request = request.WithContext(identitysdk.WithRequestIdentity(request.Context(), identitysdk.RequestIdentity{Principal: principal}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func exactActionPrincipal(workspaceID, userID, permission string) identitysdk.Principal {
	separator := strings.LastIndexByte(permission, '.')
	resource, action := identitysdk.ResourceType(permission[:separator]), identitysdk.Action(permission[separator+1:])
	bundle := &identitysdk.AccessBundle{
		Subject:        identitysdk.Subject{WorkspaceID: identitysdk.WorkspaceID(workspaceID), SubjectID: identitysdk.SubjectID(userID)},
		FunctionGrants: []identitysdk.FunctionGrant{{Resource: resource, Action: action, Effect: identitysdk.EffectAllow}},
		DataPolicies: []identitysdk.DataPolicy{{
			Key: permission, Resource: resource, Action: action, Effect: identitysdk.EffectAllow, DataScopes: []identitysdk.DataScope{identitysdk.DataScopeAll},
		}},
	}
	return identitysdk.Principal{
		ContractVersion: identitysdk.PrincipalContextContractVersion,
		Known:           true,
		WorkspaceID:     workspaceID,
		UserID:          userID,
		AccessBundle:    bundle,
	}
}
