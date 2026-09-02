package modulehttptransport

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/domainry/domainry-foundation/modulehttp"
	identitysdk "github.com/domainry/domainry-identity-sdk"
	metadatasdk "github.com/domainry/domainry-metadata-sdk"
	metadataapplication "github.com/domainry/domainry-metadata/internal/application/metadata"
)

type metadataSurface struct {
	handler    http.Handler
	routes     []modulehttp.Route
	operations map[string]map[string]any
}

func (*metadataSurface) ContractVersion() string { return modulehttp.ContractVersion }
func (*metadataSurface) Owner() string           { return "metadata" }
func (*metadataSurface) Name() string            { return "metadata_catalog" }
func (s *metadataSurface) Handler() http.Handler { return s.handler }
func (s *metadataSurface) Routes() []modulehttp.Route {
	return append([]modulehttp.Route(nil), s.routes...)
}
func (s *metadataSurface) OpenAPIOperations() map[string]map[string]any {
	return s.operations
}

func NewSurface(binding metadatasdk.Binding) (modulehttp.Surface, error) {
	if binding == nil || binding.Definitions() == nil || binding.Localization() == nil || binding.Dictionaries() == nil {
		return nil, errors.New("Metadata HTTP dependencies are incomplete")
	}
	handler := &metadataHandler{definitions: binding.Definitions(), localization: binding.Localization(), dictionaries: binding.Dictionaries(), mux: http.NewServeMux()}
	routes, err := metadataRoutes()
	if err != nil {
		return nil, err
	}
	handlers := handler.handlers()
	byAction := metadataOpenAPIOperationsByAction()
	operations := make(map[string]map[string]any, len(routes))
	for _, route := range routes {
		key := strings.TrimSpace(route.Action.Key)
		implementation, found := handlers[key]
		if !found {
			return nil, fmt.Errorf("Metadata Action %q has no HTTP handler", key)
		}
		operation, found := byAction[key]
		if !found {
			return nil, fmt.Errorf("Metadata Action %q has no OpenAPI operation", key)
		}
		handler.mux.HandleFunc(route.Pattern(), implementation)
		operations[route.Pattern()] = operation
		delete(handlers, key)
		delete(byAction, key)
	}
	if len(handlers) != 0 || len(byAction) != 0 {
		keys := make([]string, 0, len(handlers)+len(byAction))
		for key := range handlers {
			keys = append(keys, "handler:"+key)
		}
		for key := range byAction {
			keys = append(keys, "openapi:"+key)
		}
		sort.Strings(keys)
		return nil, fmt.Errorf("Metadata implementations have no Action manifest entries: %v", keys)
	}
	return &metadataSurface{handler: handler.mux, routes: routes, operations: operations}, nil
}

func metadataRoutes() ([]modulehttp.Route, error) {
	definitions, err := metadataapplication.AuthorizationActions()
	if err != nil {
		return nil, err
	}
	routes := make([]modulehttp.Route, 0, len(definitions))
	for _, definition := range definitions {
		if definition.HTTP == nil {
			continue
		}
		route, err := modulehttp.RouteFromAction(definition)
		if err != nil {
			return nil, fmt.Errorf("project Metadata Action %q: %w", definition.Key, err)
		}
		routes = append(routes, route)
	}
	return routes, nil
}

type metadataHandler struct {
	definitions  metadatasdk.Definitions
	localization metadatasdk.Localization
	dictionaries metadatasdk.Dictionaries
	mux          *http.ServeMux
}

func (h *metadataHandler) handlers() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		metadatasdk.ActionMetadataDefinitionsList:          h.listDefinitions,
		metadatasdk.ActionMetadataDefinitionsGet:           h.getDefinition,
		metadatasdk.ActionMetadataLocalizedTextsList:       h.listLocalizedTexts,
		metadatasdk.ActionMetadataLocalizedTextsCoverage:   h.localizedTextCoverage,
		metadatasdk.ActionMetadataLocalizedTextsExportCSV:  h.exportLocalizedTextsCSV,
		metadatasdk.ActionMetadataLocalizedTextsExportXLSX: h.exportLocalizedTextsXLSX,
		metadatasdk.ActionMetadataDictionaryItemsList:      h.dictionaryItems,
	}
}

func (h *metadataHandler) listDefinitions(writer http.ResponseWriter, request *http.Request) {
	principal, ok := metadataPrincipal(request, metadatasdk.ActionMetadataDefinitionsList)
	if !ok || !sameWorkspace(request.URL.Query().Get("workspace_id"), principal.WorkspaceID) {
		writeMetadataError(writer, &metadatasdk.Error{StatusCode: 403, Code: "auth.permission_denied"})
		return
	}
	resourceType := strings.TrimSpace(request.PathValue("resourceType"))
	values, err := h.definitions.List(request.Context(), metadatasdk.DefinitionQuery{ResourceType: resourceType})
	if err != nil {
		writeMetadataError(writer, err)
		return
	}
	writeMetadataCachedJSON(writer, request, map[string]any{"definitions": values, "resource_type": resourceType})
}

func (h *metadataHandler) getDefinition(writer http.ResponseWriter, request *http.Request) {
	if _, ok := metadataPrincipal(request, metadatasdk.ActionMetadataDefinitionsGet); !ok {
		writeMetadataError(writer, &metadatasdk.Error{StatusCode: 403, Code: "auth.permission_denied"})
		return
	}
	value, found, err := h.definitions.Get(request.Context(), strings.TrimSpace(request.PathValue("resourceType")), strings.TrimSpace(request.PathValue("resourceKey")))
	if err != nil {
		writeMetadataError(writer, err)
		return
	}
	if !found {
		writeMetadataError(writer, &metadatasdk.Error{StatusCode: 404, Code: "backend.metadata.definition_not_found"})
		return
	}
	writeMetadataCachedJSON(writer, request, value)
}

func (h *metadataHandler) listLocalizedTexts(writer http.ResponseWriter, request *http.Request) {
	query, ok := localizedTextQuery(request, metadatasdk.ActionMetadataLocalizedTextsList)
	if !ok {
		writeMetadataError(writer, &metadatasdk.Error{StatusCode: 403, Code: "auth.permission_denied"})
		return
	}
	values, err := h.localization.List(request.Context(), query)
	if err != nil {
		writeMetadataError(writer, err)
		return
	}
	writeMetadataCachedJSON(writer, request, map[string]any{"localized_texts": values})
}

func (h *metadataHandler) localizedTextCoverage(writer http.ResponseWriter, request *http.Request) {
	principal, ok := metadataPrincipal(request, metadatasdk.ActionMetadataLocalizedTextsCoverage)
	if !ok {
		writeMetadataError(writer, &metadatasdk.Error{StatusCode: 403, Code: "auth.permission_denied"})
		return
	}
	result, err := h.localization.Coverage(request.Context(), metadatasdk.LocalizedTextCoverageQuery{
		WorkspaceID: principal.WorkspaceID, Locale: strings.TrimSpace(request.URL.Query().Get("locale")), FallbackLocale: strings.TrimSpace(request.URL.Query().Get("fallback_locale")),
	})
	if err != nil {
		writeMetadataError(writer, err)
		return
	}
	writeMetadataCachedJSON(writer, request, result)
}

func (h *metadataHandler) exportLocalizedTextsCSV(writer http.ResponseWriter, request *http.Request) {
	values, ok := h.exportValues(writer, request, metadatasdk.ActionMetadataLocalizedTextsExportCSV)
	if !ok {
		return
	}
	writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
	writer.Header().Set("Content-Disposition", `attachment; filename="localized-texts.csv"`)
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(localizedTextCSV(values))
}

func (h *metadataHandler) exportLocalizedTextsXLSX(writer http.ResponseWriter, request *http.Request) {
	values, ok := h.exportValues(writer, request, metadatasdk.ActionMetadataLocalizedTextsExportXLSX)
	if !ok {
		return
	}
	writer.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	writer.Header().Set("Content-Disposition", `attachment; filename="localized-texts.xlsx"`)
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(localizedTextXLSX(values))
}

func (h *metadataHandler) exportValues(writer http.ResponseWriter, request *http.Request, actionKey string) ([]metadatasdk.LocalizedText, bool) {
	query, ok := localizedTextQuery(request, actionKey)
	if !ok {
		writeMetadataError(writer, &metadatasdk.Error{StatusCode: 403, Code: "auth.permission_denied"})
		return nil, false
	}
	values, err := h.localization.List(request.Context(), query)
	if err != nil {
		writeMetadataError(writer, err)
		return nil, false
	}
	return values, true
}

func (h *metadataHandler) dictionaryItems(writer http.ResponseWriter, request *http.Request) {
	principal, ok := metadataPrincipal(request, "")
	if !ok {
		writeMetadataError(writer, &metadatasdk.Error{StatusCode: 403, Code: "auth.permission_denied"})
		return
	}
	result, err := h.dictionaries.Items(request.Context(), metadatasdk.DictionaryItemsQuery{WorkspaceID: principal.WorkspaceID, DictionaryKey: strings.TrimSpace(request.PathValue("dictionaryKey")), Locale: strings.TrimSpace(request.URL.Query().Get("locale"))})
	if err != nil {
		writeMetadataError(writer, err)
		return
	}
	writer.Header().Set("Cache-Control", "private, max-age=5, must-revalidate")
	writeMetadataJSON(writer, http.StatusOK, result)
}

func localizedTextQuery(request *http.Request, actionKey string) (metadatasdk.LocalizedTextQuery, bool) {
	principal, ok := metadataPrincipal(request, actionKey)
	if !ok {
		return metadatasdk.LocalizedTextQuery{}, false
	}
	workspaceID := strings.TrimSpace(request.URL.Query().Get("workspace_id"))
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(principal.WorkspaceID)
	}
	if !sameWorkspace(workspaceID, principal.WorkspaceID) {
		return metadatasdk.LocalizedTextQuery{}, false
	}
	return metadatasdk.LocalizedTextQuery{WorkspaceID: workspaceID, EntityType: strings.TrimSpace(request.URL.Query().Get("entity_type")), EntityKey: strings.TrimSpace(request.URL.Query().Get("entity_key")), Property: strings.TrimSpace(request.URL.Query().Get("property")), Locale: strings.TrimSpace(request.URL.Query().Get("locale"))}, true
}

func metadataPrincipal(request *http.Request, actionKey string) (identitysdk.Principal, bool) {
	principal, ok := identitysdk.PrincipalFromContext(request.Context())
	authorized := ok && principal.Known && strings.TrimSpace(principal.WorkspaceID) != ""
	if authorized && strings.TrimSpace(actionKey) != "" {
		authorized = principal.HasPermission(actionKey)
	}
	return principal, authorized
}

func sameWorkspace(requested, principal string) bool {
	requested, principal = strings.TrimSpace(requested), strings.TrimSpace(principal)
	return requested == "" || requested == principal
}

func writeMetadataError(writer http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "backend.metadata.internal"
	var sdkError *metadatasdk.Error
	if errors.As(err, &sdkError) {
		if sdkError.StatusCode > 0 {
			status = sdkError.StatusCode
		}
		if strings.TrimSpace(sdkError.Code) != "" {
			code = sdkError.Code
		}
	}
	writer.Header().Set("Cache-Control", "no-store")
	writeMetadataJSON(writer, status, map[string]any{
		"error": code, "code": code, "message": code, "message_key": code,
		"params": map[string]string{},
	})
}

func writeMetadataCachedJSON(writer http.ResponseWriter, request *http.Request, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		writeMetadataError(writer, err)
		return
	}
	sum := sha256.Sum256(payload)
	etag := `"` + hex.EncodeToString(sum[:])[:24] + `"`
	writer.Header().Set("ETag", etag)
	writer.Header().Set("Cache-Control", "private, max-age=5, must-revalidate")
	if strings.TrimSpace(request.Header.Get("If-None-Match")) == etag {
		writer.WriteHeader(http.StatusNotModified)
		return
	}
	writeMetadataJSON(writer, http.StatusOK, value)
}

func writeMetadataJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

var localizedTextColumns = []string{"workspace_id", "entity_type", "entity_key", "property", "locale", "text"}

func localizedTextCSV(values []metadatasdk.LocalizedText) []byte {
	var buffer bytes.Buffer
	csvWriter := csv.NewWriter(&buffer)
	_ = csvWriter.Write(localizedTextColumns)
	for _, value := range values {
		_ = csvWriter.Write([]string{value.WorkspaceID, value.EntityType, value.EntityKey, value.Property, value.Locale, value.Text})
	}
	csvWriter.Flush()
	return buffer.Bytes()
}

func localizedTextXLSX(values []metadatasdk.LocalizedText) []byte {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	files := map[string]string{
		"[Content_Types].xml":        `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`,
		"_rels/.rels":                `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="localized_texts" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   localizedTextWorksheetXML(values),
	}
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "xl/workbook.xml", "xl/_rels/workbook.xml.rels", "xl/worksheets/sheet1.xml"} {
		entry, _ := archive.Create(name)
		_, _ = entry.Write([]byte(files[name]))
	}
	_ = archive.Close()
	return buffer.Bytes()
}

func localizedTextWorksheetXML(values []metadatasdk.LocalizedText) string {
	rows := make([][]string, 0, len(values)+1)
	rows = append(rows, localizedTextColumns)
	for _, value := range values {
		rows = append(rows, []string{value.WorkspaceID, value.EntityType, value.EntityKey, value.Property, value.Locale, value.Text})
	}
	var buffer bytes.Buffer
	buffer.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for rowIndex, row := range rows {
		buffer.WriteString(fmt.Sprintf(`<row r="%d">`, rowIndex+1))
		for columnIndex, value := range row {
			buffer.WriteString(`<c r="` + spreadsheetColumn(columnIndex+1) + fmt.Sprint(rowIndex+1) + `" t="inlineStr"><is><t>`)
			_ = xml.EscapeText(&buffer, []byte(value))
			buffer.WriteString(`</t></is></c>`)
		}
		buffer.WriteString(`</row>`)
	}
	buffer.WriteString(`</sheetData></worksheet>`)
	return buffer.String()
}

func spreadsheetColumn(index int) string {
	if index <= 0 {
		return "A"
	}
	name := ""
	for index > 0 {
		index--
		name = string(rune('A'+index%26)) + name
		index /= 26
	}
	return name
}

var _ modulehttp.Surface = (*metadataSurface)(nil)
var _ modulehttp.OpenAPIProvider = (*metadataSurface)(nil)
