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
		class metadatasdk.ErrorKind
		kind  string
		code  string
	}{
		{metadatasdk.ErrorKindUnavailable, "unavailable", "backend.metadata.database_unavailable"},
		{metadatasdk.ErrorKindConflict, "conflict", "backend.metadata.publication_fence_rejected"},
		{metadatasdk.ErrorKindBadRequest, "bad_request", "backend.metadata.definition_invalid"},
		{metadatasdk.ErrorKindForbidden, "forbidden", "backend.metadata.permission_denied"},
	}
	for _, test := range tests {
		capture := &classificationCapture{ResponseRecorder: httptest.NewRecorder()}
		writeMetadataError(capture, &metadatasdk.Error{Kind: test.class, Code: test.code})
		if capture.kind != test.kind || capture.code != test.code {
			t.Fatalf("class=%q kind=%q code=%q", test.class, capture.kind, capture.code)
		}
	}
}
