package modulehttptransport

import (
	"net/http/httptest"
	"testing"

	metadatasdk "github.com/domainry/domainry-metadata-sdk"
)

type classificationCapture struct {
	*httptest.ResponseRecorder
	kind, code string
}

func (capture *classificationCapture) RecordError(kind, code string) {
	capture.kind, capture.code = kind, code
}

func TestMetadataErrorClassificationMatrix(t *testing.T) {
	tests := []struct {
		status int
		kind   string
		code   string
	}{
		{503, "unavailable", "backend.metadata.database_unavailable"},
		{409, "conflict", "backend.metadata.publication_fence_rejected"},
		{400, "bad_request", "backend.metadata.definition_invalid"},
		{403, "forbidden", "backend.metadata.permission_denied"},
	}
	for _, test := range tests {
		capture := &classificationCapture{ResponseRecorder: httptest.NewRecorder()}
		writeMetadataError(capture, &metadatasdk.Error{StatusCode: test.status, Code: test.code})
		if capture.kind != test.kind || capture.code != test.code {
			t.Fatalf("status=%d kind=%q code=%q", test.status, capture.kind, capture.code)
		}
	}
}
