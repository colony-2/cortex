package cmd

import (
	"bytes"
	"context"
	"net/http"
	"testing"
)

func TestWorkflowStory(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/projects/proj/jobs/w1/story" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"job_id":"w1",
			"invocation_sequence":1,
			"recipe":{
				"id":"r1",
				"name":"r1",
				"version":"v1",
				"source":{"kind":"jobStartArtifact","artifact_name":"job-start.json"}
			},
			"root":{
				"id":"root",
				"kind":"recipe",
				"status":"succeeded",
				"title":"root",
				"attempt":1,
				"invoke_seq":1,
				"path":[],
				"children":[],
				"prior_attempts":[],
				"artifact_keys":[],
				"input":{},
				"output":{}
			},
			"started_at":"2025-01-01T00:00:00Z",
			"finished_at":"2025-01-01T00:00:01Z",
			"status":"completed"
		}`))
	})
	app, buf, cleanup := setupWorkflowApp(t, handler)
	defer cleanup()

	cmd := newWorkflowStoryCmd()
	cmd.SetArgs([]string{"w1"})
	cmd.SetContext(storeApp(context.Background(), app))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := buf.String(); !bytes.Contains([]byte(got), []byte(`"job_id": "w1"`)) {
		t.Fatalf("unexpected output: %s", got)
	}
}
