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
	"reflect"
	"strings"
	"sync"
	"testing"

	shareddefinition "github.com/domainry/domainry-foundation/definition"
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
	database *sql.DB
	renderer modulehost.Dialect
	mu       sync.Mutex
	owners   []string
}

func (*integrationMigrationRegistrar) Driver() string { return "sqlite" }
func (*integrationMigrationRegistrar) Schema() string { return "" }
func (r *integrationMigrationRegistrar) ApplyOwnedMigrations(ctx context.Context, owner string, migrations []modulehost.SchemaMigration) error {
	if owner != "metadata" && owner != shareddefinition.MigrationOwner {
		return fmt.Errorf("unexpected migration owner %q", owner)
	}
	r.mu.Lock()
	r.owners = append(r.owners, owner)
	r.mu.Unlock()
	ledger := "_schema_migrations_" + strings.ReplaceAll(owner, "/", "_")
	runner, err := ormmigration.NewRunner(r.database, r.renderer, ormmigration.Options{LedgerTable: ledger})
	if err != nil {
		return err
	}
	return runner.Apply(ctx, migrations)
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
	registrar := &integrationMigrationRegistrar{database: database, renderer: renderer}
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
		Owner:         metadatasdk.DefinitionOwnerMetadata,
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
	if owners := registrar.appliedOwners(); !reflect.DeepEqual(owners, []string{shareddefinition.MigrationOwner, "metadata"}) {
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
	if value, found, err := binding.Definitions().Get(transactionContext, metadatasdk.DefinitionOwnerMetadata, "object", "customer"); err != nil || !found || value.SchemaVersion != "rollback-version" {
		_ = transaction.Rollback()
		t.Fatalf("transaction definition=%#v found=%v err=%v", value, found, err)
	}
	if err := transaction.Rollback(); err != nil {
		t.Fatal(err)
	}
	if value, found, err := binding.Definitions().Get(t.Context(), metadatasdk.DefinitionOwnerMetadata, "object", "customer"); err != nil || found {
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
	definition, found, err := binding.Definitions().Get(t.Context(), metadatasdk.DefinitionOwnerMetadata, "object", "customer")
	if err != nil || !found || definition.SchemaVersion != "2" || definition.Name != "Customer account" || !strings.Contains(string(definition.Payload), "Customer account") {
		t.Fatalf("published definition=%#v found=%v err=%v", definition, found, err)
	}
	definitions, err := binding.Definitions().List(t.Context(), metadatasdk.DefinitionQuery{Owner: metadatasdk.DefinitionOwnerMetadata, ResourceType: "object"})
	if err != nil || len(definitions) != 1 || definitions[0].ResourceKey != "customer" {
		t.Fatalf("definitions=%#v err=%v", definitions, err)
	}
	items, err := binding.Dictionaries().Items(t.Context(), metadatasdk.DictionaryItemsQuery{WorkspaceID: "workspace-a", DictionaryKey: "status", Locale: "zh-CN"})
	if err != nil || len(items.Items) != 1 || items.Items[0].Label != "启用" {
		t.Fatalf("dictionary items=%#v err=%v", items, err)
	}
	var objectVersions int
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM _definition_versions WHERE kind = ? AND definition_key = ?`, "object", "customer").Scan(&objectVersions); err != nil || objectVersions != 2 {
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
	if owners := reopenedRegistrar.appliedOwners(); !reflect.DeepEqual(owners, []string{shareddefinition.MigrationOwner, "metadata"}) {
		t.Fatalf("reopened migration owners=%v", owners)
	}
	definition, found, err = reopened.Definitions().Get(t.Context(), metadatasdk.DefinitionOwnerMetadata, "object", "customer")
	if err != nil || !found || definition.SchemaVersion != "2" || definition.Name != "Customer account" {
		t.Fatalf("reopened definition=%#v found=%v err=%v", definition, found, err)
	}
	for _, ledger := range []string{"_schema_migrations_shared_definitions", "_schema_migrations_metadata"} {
		var ledgerRows int
		if err := reopenedDatabase.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM `+ledger).Scan(&ledgerRows); err != nil || ledgerRows != 1 {
			t.Fatalf("host migration ledger=%s rows=%d err=%v", ledger, ledgerRows, err)
		}
	}
	var ledgers int
	if err := reopenedDatabase.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name LIKE '%schema_migrations%'`).Scan(&ledgers); err != nil || ledgers != 2 {
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
	definition, found, err := binding.Definitions().Get(t.Context(), metadatasdk.DefinitionOwnerMetadata, "object", "customer")
	if err != nil || !found || definition.SchemaVersion != "2" {
		t.Fatalf("winning definition=%#v found=%v err=%v", definition, found, err)
	}
	var versions int
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM _definition_versions WHERE kind = ? AND definition_key = ?`, "object", "customer").Scan(&versions); err != nil || versions != 2 {
		t.Fatalf("definition versions=%d err=%v", versions, err)
	}
}

func TestPublicDefinitionStorePublishDisableAndImmutableVersionRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "definition-store.db")
	database, binding, _ := openIntegrationModule(t, path)
	defer database.Close()
	defer binding.Close(t.Context())
	store := binding.DefinitionStore()
	first, err := store.Publish(t.Context(), metadatasdk.DefinitionPublishCommand{
		Owner: metadatasdk.DefinitionOwnerReport, ResourceType: "report", ResourceKey: "daily-sales",
		ExpectedCurrentVersionID: metadatasdk.DefinitionNoCurrentVersion, SchemaVersion: "1",
		Payload:    json.RawMessage(`{"key":"daily-sales","name":"Daily sales"}`),
		SourceKind: "report_registry", SourceID: "reports", PublishedBy: "user:publisher-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.CurrentVersionID == "" || first.Definition.CurrentVersionID != first.CurrentVersionID || first.Definition.Status != "active" || first.Definition.PublishedBy != "user:publisher-a" {
		t.Fatalf("first publication=%#v", first)
	}
	firstVersion, found, err := store.GetVersion(t.Context(), metadatasdk.DefinitionVersionQuery{
		Owner: metadatasdk.DefinitionOwnerReport, ResourceType: "report", ResourceKey: "daily-sales", VersionID: first.CurrentVersionID,
	})
	if err != nil || !found || firstVersion.SchemaVersion != "1" || !strings.Contains(string(firstVersion.Payload), "Daily sales") {
		t.Fatalf("first version=%#v found=%t err=%v", firstVersion, found, err)
	}
	if _, err := store.Publish(t.Context(), metadatasdk.DefinitionPublishCommand{
		Owner: metadatasdk.DefinitionOwnerReport, ResourceType: "report", ResourceKey: "daily-sales",
		ExpectedCurrentVersionID: metadatasdk.DefinitionNoCurrentVersion, SchemaVersion: "2",
		Payload:    json.RawMessage(`{"key":"daily-sales","name":"Stale"}`),
		SourceKind: "report_registry", SourceID: "reports", PublishedBy: "user:publisher-b",
	}); metadataErrorCode(err) != "metadata.definition_revision_conflict" {
		t.Fatalf("stale create error=%v", err)
	}
	second, err := store.Publish(t.Context(), metadatasdk.DefinitionPublishCommand{
		Owner: metadatasdk.DefinitionOwnerReport, ResourceType: "report", ResourceKey: "daily-sales",
		ExpectedCurrentVersionID: first.CurrentVersionID, SchemaVersion: "2",
		Payload:    json.RawMessage(`{"key":"daily-sales","name":"Daily sales v2"}`),
		SourceKind: "report_registry", SourceID: "reports", PublishedBy: "user:publisher-b",
	})
	if err != nil || second.CurrentVersionID == first.CurrentVersionID {
		t.Fatalf("second publication=%#v err=%v", second, err)
	}
	replayed, err := store.Publish(t.Context(), metadatasdk.DefinitionPublishCommand{
		Owner: metadatasdk.DefinitionOwnerReport, ResourceType: "report", ResourceKey: "daily-sales",
		ExpectedCurrentVersionID: second.CurrentVersionID, SchemaVersion: "2",
		Payload:    json.RawMessage(`{"key":"daily-sales","name":"Daily sales v2"}`),
		SourceKind: "report_registry", SourceID: "reports", PublishedBy: "user:publisher-b",
	})
	if err != nil || replayed.CurrentVersionID != second.CurrentVersionID {
		t.Fatalf("idempotent replay=%#v err=%v", replayed, err)
	}
	if _, err := store.Publish(t.Context(), metadatasdk.DefinitionPublishCommand{
		Owner: metadatasdk.DefinitionOwnerReport, ResourceType: "report", ResourceKey: "daily-sales",
		ExpectedCurrentVersionID: second.CurrentVersionID, SchemaVersion: "3",
		Payload:    json.RawMessage(`{"key":"daily-sales","name":"Daily sales v2"}`),
		SourceKind: "report_registry", SourceID: "reports", PublishedBy: "user:publisher-b",
	}); metadataErrorCode(err) != "metadata.definition_revision_conflict" {
		t.Fatalf("token-stable mutation error=%v", err)
	}
	firstVersion, found, err = store.GetVersion(t.Context(), metadatasdk.DefinitionVersionQuery{
		Owner: metadatasdk.DefinitionOwnerReport, ResourceType: "report", ResourceKey: "daily-sales", SchemaVersion: "1",
	})
	if err != nil || !found || !strings.Contains(string(firstVersion.Payload), "Daily sales\"") || strings.Contains(string(firstVersion.Payload), "v2") {
		t.Fatalf("immutable first version=%#v found=%t err=%v", firstVersion, found, err)
	}
	if err := store.Disable(t.Context(), metadatasdk.DefinitionDisableCommand{
		Owner: metadatasdk.DefinitionOwnerReport, ResourceType: "report", ResourceKey: "daily-sales",
		ExpectedCurrentVersionID: first.CurrentVersionID, DisabledBy: "user:publisher-a",
	}); metadataErrorCode(err) != "metadata.definition_revision_conflict" {
		t.Fatalf("stale disable error=%v", err)
	}
	if err := store.Disable(t.Context(), metadatasdk.DefinitionDisableCommand{
		Owner: metadatasdk.DefinitionOwnerReport, ResourceType: "report", ResourceKey: "daily-sales",
		ExpectedCurrentVersionID: second.CurrentVersionID, DisabledBy: "user:publisher-b",
	}); err != nil {
		t.Fatal(err)
	}
	if value, found, err := store.Get(t.Context(), metadatasdk.DefinitionOwnerReport, "report", "daily-sales"); err != nil || found {
		t.Fatalf("disabled definition=%#v found=%t err=%v", value, found, err)
	}
	secondVersion, found, err := store.GetVersion(t.Context(), metadatasdk.DefinitionVersionQuery{
		Owner: metadatasdk.DefinitionOwnerReport, ResourceType: "report", ResourceKey: "daily-sales", VersionID: second.CurrentVersionID,
	})
	if err != nil || !found || secondVersion.SchemaVersion != "2" {
		t.Fatalf("disabled current version=%#v found=%t err=%v", secondVersion, found, err)
	}
}

func TestPublicDefinitionStoreConcurrentPublicationUsesSingleCASRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "definition-store-cas.db")
	database, binding, _ := openIntegrationModule(t, path)
	defer database.Close()
	defer binding.Close(t.Context())
	store := binding.DefinitionStore()
	base, err := store.Publish(t.Context(), metadatasdk.DefinitionPublishCommand{
		Owner: metadatasdk.DefinitionOwnerScheduler, ResourceType: "scheduler", ResourceKey: "nightly",
		ExpectedCurrentVersionID: metadatasdk.DefinitionNoCurrentVersion, SchemaVersion: "1", Payload: json.RawMessage(`{"key":"nightly","cron":"0 0 * * *"}`),
		SourceKind: "scheduler_registry", SourceID: "schedules", PublishedBy: "system:startup",
	})
	if err != nil {
		t.Fatal(err)
	}
	commands := []metadatasdk.DefinitionPublishCommand{
		{Owner: metadatasdk.DefinitionOwnerScheduler, ResourceType: "scheduler", ResourceKey: "nightly", ExpectedCurrentVersionID: base.CurrentVersionID, SchemaVersion: "2-a", Payload: json.RawMessage(`{"key":"nightly","cron":"0 1 * * *"}`), SourceKind: "scheduler_registry", SourceID: "schedules", PublishedBy: "user:a"},
		{Owner: metadatasdk.DefinitionOwnerScheduler, ResourceType: "scheduler", ResourceKey: "nightly", ExpectedCurrentVersionID: base.CurrentVersionID, SchemaVersion: "2-b", Payload: json.RawMessage(`{"key":"nightly","cron":"0 2 * * *"}`), SourceKind: "scheduler_registry", SourceID: "schedules", PublishedBy: "user:b"},
	}
	start := make(chan struct{})
	results := make([]metadatasdk.DefinitionPublishResult, len(commands))
	errorsByCandidate := make([]error, len(commands))
	var wait sync.WaitGroup
	wait.Add(len(commands))
	for index := range commands {
		go func(index int) {
			defer wait.Done()
			<-start
			results[index], errorsByCandidate[index] = store.Publish(t.Context(), commands[index])
		}(index)
	}
	close(start)
	wait.Wait()
	successes, conflicts := 0, 0
	winner := ""
	for index, err := range errorsByCandidate {
		if err == nil {
			successes++
			winner = results[index].CurrentVersionID
			continue
		}
		if metadataErrorCode(err) == "metadata.definition_revision_conflict" {
			conflicts++
			continue
		}
		t.Fatalf("unexpected concurrent CAS error=%v", err)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("CAS outcomes successes=%d conflicts=%d errors=%v", successes, conflicts, errorsByCandidate)
	}
	current, found, err := store.Get(t.Context(), metadatasdk.DefinitionOwnerScheduler, "scheduler", "nightly")
	if err != nil || !found || current.CurrentVersionID != winner {
		t.Fatalf("current definition=%#v winner=%q found=%t err=%v", current, winner, found, err)
	}
	var versions int
	if err := database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM _definition_versions WHERE owner = ? AND kind = ? AND definition_key = ?`, metadatasdk.DefinitionOwnerScheduler, "scheduler", "nightly").Scan(&versions); err != nil || versions != 2 {
		t.Fatalf("CAS versions=%d err=%v", versions, err)
	}
}

func metadataErrorCode(err error) string {
	var metadataError *metadatasdk.Error
	if errors.As(err, &metadataError) {
		return metadataError.Code
	}
	return ""
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
