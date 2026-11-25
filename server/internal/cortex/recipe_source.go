package cortex

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/colony-2/c2j/pkg/worker/compiler"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

type recipeGitSource struct {
	Path   string `json:"path,omitempty"`
	Commit string `json:"commit,omitempty"`
}

func gitSource(selector, commit string) recipeGitSource {
	if !compiler.IsGitRecipeSelector(selector) || compiler.ValidateRecipeSelector(selector) != nil {
		return recipeGitSource{}
	}
	selector = strings.TrimSpace(selector)
	at := strings.LastIndexByte(selector, '@')
	ref := selector[at+1:]
	if commit == "" && (len(ref) == 40 || len(ref) == 64) {
		if _, err := hex.DecodeString(ref); err == nil {
			commit = ref
		}
	}
	return recipeGitSource{Path: strings.TrimPrefix(selector[:at], "git+"), Commit: commit}
}

// Read only the root resolver's result. Included recipes and ordinary task
// outputs can carry their own selectors and commits.
func recordedRecipeSource(run jobdb.GetJobRunResponse) recipeGitSource {
	var metadata struct {
		Recipe string `json:"recipe"`
	}
	_ = json.Unmarshal(run.Job.Metadata, &metadata)
	source := gitSource(metadata.Recipe, "")
	apply := func(raw json.RawMessage) {
		resolved, err := compiler.ParseResolvedRecipeSourceJSON(raw)
		if err == nil {
			source = gitSource(resolved.EffectiveSelector(), resolved.ResolvedCommit)
		}
	}

	for _, attempt := range run.Attempts {
		for _, task := range attempt.Tasks {
			if task.TaskType != compiler.RootSourceResolutionTaskType {
				continue
			}
			for _, a := range task.Attempts {
				if a.Output != nil && a.Outcome.Status == jobdb.TaskOutcomeStatusSucceeded {
					apply(a.Output.Data)
				}
			}
		}
	}
	var root string
	for _, entry := range run.Execution {
		r := entry.Record
		switch r.Kind {
		case "task":
			var descriptor struct {
				Call struct {
					Type string `json:"task_type"`
				} `json:"call"`
			}
			if json.Unmarshal(r.Descriptor, &descriptor) == nil && descriptor.Call.Type == compiler.RootSourceResolutionTaskType {
				root = r.ID
			}
		case "result":
			i := strings.LastIndex(r.ID, "/attempt:")
			if i >= 0 && root != "" && r.ID[:i] == root && r.ErrorKind == "" && r.Output != nil {
				apply(r.Output.Data)
			}
		}
	}
	return source
}

func (s *Server) handleRecipeSource(w http.ResponseWriter, r *http.Request) {
	run, err := s.engine.GetJobRun(r.Context(), jobdb.GetJobRunRequest{
		JobKey:         jobdb.JobKey{TenantId: s.tenantID(r), JobId: mux.Vars(r)["jobId"]},
		IncludeOutputs: true,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, recordedRecipeSource(run))
}
