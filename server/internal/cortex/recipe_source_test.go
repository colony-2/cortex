package cortex

import (
	"encoding/json"
	"testing"

	"github.com/colony-2/jobdb/pkg/jobdb"
)

func TestRecordedRecipeSource(t *testing.T) {
	const path = "https://example.org/recipes.git//.c2/recipes/release.yaml"
	const sha = "7fe41a14a5f604f51b206e5de123a6f68d6eb602"
	run := jobdb.GetJobRunResponse{Job: jobdb.JobRunSummary{Metadata: json.RawMessage(`{"recipe":"git+` + path + `@main","git_ref":"workspace-branch"}`)}}
	if got := recordedRecipeSource(run); got != (recipeGitSource{Path: path}) {
		t.Fatalf("unresolved branch: %+v", got)
	}
	const root = "/scope:job/attempt:1/task:root-source"
	out := &jobdb.ExecutionSnapshot{Data: json.RawMessage(`{"v":2,"kind":"internal_result","payload":{"source_kind":"git","resolved_selector":"git+` + path + `@` + sha + `","resolved_commit":"` + sha + `"}}`)}
	run.Execution = []jobdb.ExecutionHistoryEntry{
		{Record: jobdb.ExecutionRecord{Kind: "task", ID: root, Descriptor: json.RawMessage(`{"call":{"task_type":"recipe_root_source_resolve","site":"root-source"}}`)}},
		{Record: jobdb.ExecutionRecord{Kind: "result", ID: root + "/attempt:2", Output: out}},
		{Record: jobdb.ExecutionRecord{Kind: "result", ID: "other/attempt:1", Output: &jobdb.ExecutionSnapshot{Data: json.RawMessage(`{"source_kind":"git","resolved_selector":"git+https://example.org/other//wrong.yml@other"}`)}}},
	}
	want := recipeGitSource{Path: path, Commit: sha}
	if got := recordedRecipeSource(run); got != want {
		t.Fatalf("recorded source: %+v", got)
	}
	run.Execution = nil
	run.Attempts = []jobdb.JobAttempt{{Tasks: []jobdb.TaskRun{{TaskType: "recipe_root_source_resolve", Attempts: []jobdb.TaskAttempt{{Outcome: jobdb.TaskOutcome{Status: jobdb.TaskOutcomeStatusSucceeded}, Output: &jobdb.TaskIO{Data: out.Data}}}}}}}
	if got := recordedRecipeSource(run); got != want {
		t.Fatalf("legacy source: %+v", got)
	}
}
