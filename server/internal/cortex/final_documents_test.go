package cortex

import (
	"context"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/colony-2/c2j/pkg/story"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

type finalDocumentsStub struct {
	storyService
	manifest *story.FinalDocuments
	err      error
}

func (s *finalDocumentsStub) GetFinalDocuments(context.Context, story.GetWorkflowOutcomeRequest) (*story.FinalDocuments, error) {
	return s.manifest, s.err
}
func (s *finalDocumentsStub) GetFinalDocument(context.Context, story.GetFinalDocumentRequest) (*story.ArtifactData, error) {
	panic("must use exact validated member coordinates")
}

func TestFinalDocumentsAdapterStates(t *testing.T) {
	stub := &finalDocumentsStub{}
	srv := &Server{story: stub}
	call := func(path string, document bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r = mux.SetURLVars(r, map[string]string{"projectId": "tenant", "jobId": "job"})
		w := httptest.NewRecorder()
		if document {
			srv.handleFinalDocument(w, r)
		} else {
			srv.handleFinalDocuments(w, r)
		}
		return w
	}
	for _, state := range []story.FinalManifestState{story.FinalManifestPresent, story.FinalManifestEmpty, story.FinalManifestAbsent, story.FinalManifestNull, story.FinalManifestMalformed, story.FinalManifestUnsupported} {
		stub.manifest = &story.FinalDocuments{JobID: "job", State: state, Documents: []story.FinalDocument{}}
		w := call("/", false)
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), `"state":"`+string(state)+`"`)
		require.Contains(t, w.Body.String(), `"manifest_id":"`+finalManifestID(stub.manifest)+`"`)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	}
	stub.err = story.ErrOutcomePending
	require.Equal(t, 202, call("/", false).Code)
	stub.err = nil
	stub.manifest = &story.FinalDocuments{JobID: "job", State: story.FinalManifestPresent, Documents: []story.FinalDocument{{Alias: "external.md", Resolution: "external"}, {Alias: "value.json", Resolution: "value"}, {Alias: "missing.md", Resolution: "unresolved"}}}
	require.Equal(t, 409, call("/?alias=external.md&manifest_id=forged", true).Code)
	identity := url.QueryEscape(finalManifestID(stub.manifest))
	for _, alias := range []string{"external.md", "value.json", "missing.md"} {
		w := call("/?manifest_id="+identity+"&alias="+alias, true)
		require.Equal(t, 410, w.Code)
		require.True(t, strings.Contains(w.Body.String(), "final_document_unavailable"))
	}
	require.Equal(t, 404, call("/?manifest_id="+identity+"&alias=forged.md", true).Code)
	attempt := int64(2)
	stub.manifest.AttemptOrdinal = &attempt
	require.Equal(t, 409, call("/?manifest_id="+identity+"&alias=external.md", true).Code)
}
