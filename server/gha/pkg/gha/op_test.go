package gha

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/stretchr/testify/require"
)

type fakeBackend struct {
	result  backendResult
	err     error
	request backendRequest
}

func (f *fakeBackend) Run(_ context.Context, req backendRequest) (backendResult, error) {
	f.request = req
	return f.result, f.err
}

type workflowBackendFunc func(context.Context, backendRequest) (backendResult, error)

func (f workflowBackendFunc) Run(ctx context.Context, req backendRequest) (backendResult, error) {
	return f(ctx, req)
}

func TestResolveWorkflowSelectorRepoAndCell(t *testing.T) {
	worktree := t.TempDir()
	repoWorkflow := filepath.Join(worktree, ".github", "workflows", "ci.yml")
	cellWorkflow := filepath.Join(worktree, "cells", "alpha", "workflows", "cell.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(repoWorkflow), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(cellWorkflow), 0o755))
	require.NoError(t, os.WriteFile(repoWorkflow, []byte("name: repo\n"), 0o644))
	require.NoError(t, os.WriteFile(cellWorkflow, []byte("name: cell\n"), 0o644))

	gitCtx := coreops.GitExecutionContext{
		WorktreePath:     worktree,
		CellPath:         filepath.ToSlash(filepath.Join("cells", "alpha")),
		ResolvedBaseHash: "deadbeef",
	}

	resolvedRepo, err := resolveWorkflowSelector("repo://.github/workflows/ci.yml", gitCtx)
	require.NoError(t, err)
	require.Equal(t, repoWorkflow, resolvedRepo.Path)
	require.Equal(t, "repo://.github/workflows/ci.yml", resolvedRepo.Selector)
	require.NotEmpty(t, resolvedRepo.ContentHash)

	resolvedCell, err := resolveWorkflowSelector("cell://workflows/cell.yml", gitCtx)
	require.NoError(t, err)
	require.Equal(t, cellWorkflow, resolvedCell.Path)

	_, err = resolveWorkflowSelector("repo://../escape.yml", gitCtx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "escape")
}

func TestResolveWorkflowSelectorGitFile(t *testing.T) {
	repo := initGitRepoWithFiles(t, map[string]string{
		".github/workflows/ci.yml": "name: external\n",
	})
	selector := "git+file://" + filepath.ToSlash(repo) + "//.github/workflows/ci.yml@HEAD"

	resolved, err := resolveWorkflowSelector(selector, coreops.GitExecutionContext{})
	require.NoError(t, err)
	require.Equal(t, selector, resolved.Selector)
	require.NotEmpty(t, resolved.ResolvedCommit)
	require.NotEmpty(t, resolved.ContentHash)
	data, err := os.ReadFile(resolved.Path)
	require.NoError(t, err)
	require.Equal(t, "name: external\n", string(data))
}

func TestReadFileWorkflowFromRemoteRepo(t *testing.T) {
	repo := initGitRepoWithFiles(t, map[string]string{
		".github/workflows/ci.yml": "name: remote\n",
	})

	data, resolvedCommit, err := readFileWorkflowFromRemoteRepo(parsedGitWorkflowSelector{
		Scheme:       "https",
		RepoURL:      repo,
		WorkflowPath: ".github/workflows/ci.yml",
		Ref:          "HEAD",
	})
	require.NoError(t, err)
	require.NotEmpty(t, resolvedCommit)
	require.Equal(t, "name: remote\n", string(data))
}

func TestRunRegistersExternalArtifactsAndPassesResolvedWorkflowToBackend(t *testing.T) {
	worktree := t.TempDir()
	workflowPath := filepath.Join(worktree, ".github", "workflows", "ci.yml")
	logPath := filepath.Join(worktree, "gha.log")
	require.NoError(t, os.MkdirAll(filepath.Dir(workflowPath), 0o755))
	require.NoError(t, os.WriteFile(workflowPath, []byte("name: ci\n"), 0o644))
	require.NoError(t, os.WriteFile(logPath, []byte("hello"), 0o644))

	fake := &fakeBackend{
		result: backendResult{
			Output: RunOutput{
				Status: statusSuccess,
				Jobs: map[string]WorkflowJobOutput{
					"test": {Status: statusSuccess},
				},
			},
			ArtifactRefs: map[string]externalFileRef{
				"gha-logs": {Path: logPath},
			},
		},
	}
	origFactory := backendFactory
	backendFactory = func(name string) (workflowBackend, error) {
		require.Equal(t, backendAct, name)
		return fake, nil
	}
	t.Cleanup(func() {
		backendFactory = origFactory
	})

	deps := coreops.NewOpDependenciesBuilder().
		WithGitContext(coreops.GitExecutionContext{
			BaseRepo:         "acme/widgets",
			BaseRef:          "main",
			ResolvedBaseHash: "deadbeef",
			CellPath:         filepath.ToSlash(filepath.Join("cells", "alpha")),
			WorktreePath:     worktree,
		}).
		Build()

	output, err := run(deps, context.Background(), RunInput{
		Workflow: "repo://.github/workflows/ci.yml",
		Backend:  backendAct,
	})
	require.NoError(t, err)
	require.Equal(t, statusSuccess, output.Status)
	require.Equal(t, "repo://.github/workflows/ci.yml", output.Workflow.ResolvedSelector)
	require.Equal(t, "deadbeef", output.Workflow.ResolvedCommit)
	require.NotEmpty(t, output.Workflow.ContentHash)
	require.Equal(t, workflowPath, fake.request.Workflow.Path)
	require.Equal(t, "acme/widgets", fake.request.GitContext.BaseRepo)

	refs := deps.GetExternalArtifacts()
	require.Contains(t, refs, "gha-logs")
	expectedURL, err := fileURL(logPath)
	require.NoError(t, err)
	require.Equal(t, expectedURL, refs["gha-logs"].External.URL)
}

func TestRunFailureAnnotatesOutputWhenContinueOnErrorIsFalse(t *testing.T) {
	worktree := t.TempDir()
	workflowPath := filepath.Join(worktree, ".github", "workflows", "ci.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(workflowPath), 0o755))
	require.NoError(t, os.WriteFile(workflowPath, []byte("name: ci\n"), 0o644))

	fake := &fakeBackend{
		result: backendResult{
			Output: RunOutput{
				Status: statusFailure,
				Jobs: map[string]WorkflowJobOutput{
					"test": {Status: statusFailure},
				},
			},
		},
	}
	origFactory := backendFactory
	backendFactory = func(string) (workflowBackend, error) {
		return fake, nil
	}
	t.Cleanup(func() {
		backendFactory = origFactory
	})

	deps := coreops.NewOpDependenciesBuilder().
		WithGitContext(coreops.GitExecutionContext{
			BaseRef:          "main",
			ResolvedBaseHash: "deadbeef",
			WorktreePath:     worktree,
		}).
		Build()

	output, err := run(deps, context.Background(), RunInput{
		Workflow: "repo://.github/workflows/ci.yml",
		Backend:  backendAct,
	})
	require.Error(t, err)
	require.Equal(t, statusFailure, output.Status)
	require.Contains(t, output.ErrorMessage, "workflow concluded with status")

	output, err = run(deps, context.Background(), RunInput{
		Workflow:        "repo://.github/workflows/ci.yml",
		Backend:         backendAct,
		ContinueOnError: true,
	})
	require.NoError(t, err)
	require.Equal(t, statusFailure, output.Status)
	require.Empty(t, output.ErrorMessage)
}

func TestRunsFailureReturnsStructuredOutputWhenContinueOnErrorIsFalse(t *testing.T) {
	worktree := initGitRepoWithFiles(t, map[string]string{
		".github/workflows/ci.yml": "name: ci\n",
	})

	origFactory := backendFactory
	backendFactory = func(string) (workflowBackend, error) {
		return &fakeBackend{
			result: backendResult{
				Output: RunOutput{
					Status:       statusFailure,
					ExitCode:     1,
					ErrorMessage: "lint failed",
				},
			},
		}, nil
	}
	t.Cleanup(func() {
		backendFactory = origFactory
	})

	deps := coreops.NewOpDependenciesBuilder().
		WithGitContext(coreops.GitExecutionContext{
			BaseRef:          "main",
			ResolvedBaseHash: "deadbeef",
			WorktreePath:     worktree,
		}).
		Build()

	output, err := runs(deps, context.Background(), RunsInput{
		Workflows: []RunsWorkflowInput{
			{ID: "ci", RunInput: RunInput{Workflow: "repo://.github/workflows/ci.yml", Backend: backendAct}},
		},
	})
	require.Error(t, err)
	require.Equal(t, statusFailure, output.Status)
	require.False(t, output.AllPassed)
	require.Equal(t, statusFailure, output.Results["ci"].Status)
	require.Equal(t, "lint failed", output.Results["ci"].ErrorMessage)
}

func TestRunSelectsGitHubBackend(t *testing.T) {
	worktree := t.TempDir()
	workflowPath := filepath.Join(worktree, ".github", "workflows", "ci.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(workflowPath), 0o755))
	require.NoError(t, os.WriteFile(workflowPath, []byte("name: ci\n"), 0o644))

	fake := &fakeBackend{
		result: backendResult{
			Output: RunOutput{
				Status: statusSuccess,
			},
		},
	}
	origFactory := backendFactory
	backendFactory = func(name string) (workflowBackend, error) {
		require.Equal(t, backendGitHub, name)
		return fake, nil
	}
	t.Cleanup(func() {
		backendFactory = origFactory
	})

	deps := coreops.NewOpDependenciesBuilder().
		WithGitContext(coreops.GitExecutionContext{
			BaseRef:          "main",
			ResolvedBaseHash: "deadbeef",
			WorktreePath:     worktree,
		}).
		Build()

	output, err := run(deps, context.Background(), RunInput{
		Workflow: "repo://.github/workflows/ci.yml",
		Backend:  backendGitHub,
	})
	require.NoError(t, err)
	require.Equal(t, statusSuccess, output.Status)
}

func TestRunJobFlattensSelectedJob(t *testing.T) {
	worktree := t.TempDir()
	workflowPath := filepath.Join(worktree, ".github", "workflows", "ci.yml")
	require.NoError(t, os.MkdirAll(filepath.Dir(workflowPath), 0o755))
	require.NoError(t, os.WriteFile(workflowPath, []byte("name: ci\n"), 0o644))

	origFactory := backendFactory
	backendFactory = func(string) (workflowBackend, error) {
		return &fakeBackend{
			result: backendResult{
				Output: RunOutput{
					Status:   statusSuccess,
					ExitCode: 0,
					Workflow: WorkflowOutput{ResolvedSelector: "repo://.github/workflows/ci.yml"},
					Jobs: map[string]WorkflowJobOutput{
						"test": {
							Status:          statusSuccess,
							Conclusion:      statusSuccess,
							DurationSeconds: 12,
							Steps: []WorkflowStepOutput{
								{Name: "Echo", Status: statusSuccess, Conclusion: statusSuccess, DurationSeconds: 12},
							},
						},
					},
				},
			},
		}, nil
	}
	t.Cleanup(func() {
		backendFactory = origFactory
	})

	deps := coreops.NewOpDependenciesBuilder().
		WithGitContext(coreops.GitExecutionContext{
			BaseRef:          "main",
			ResolvedBaseHash: "deadbeef",
			WorktreePath:     worktree,
		}).
		Build()

	output, err := runJob(deps, context.Background(), RunInput{
		Workflow: "repo://.github/workflows/ci.yml",
		Backend:  backendAct,
		Job:      "test",
	})
	require.NoError(t, err)
	require.Equal(t, statusSuccess, output.Status)
	require.Equal(t, statusSuccess, output.Conclusion)
	require.Len(t, output.Steps, 1)
}

func TestRunsAggregatesResultsAndPrefixesArtifacts(t *testing.T) {
	worktree := initGitRepoWithFiles(t, map[string]string{
		".github/workflows/ci.yml": "name: ci\n",
	})
	logPath := filepath.Join(t.TempDir(), "combined.log")
	require.NoError(t, os.WriteFile(logPath, []byte("hello"), 0o644))

	origFactory := backendFactory
	backendFactory = func(string) (workflowBackend, error) {
		return &fakeBackend{
			result: backendResult{
				Output: RunOutput{
					Status: statusSuccess,
					Jobs: map[string]WorkflowJobOutput{
						"test": {Status: statusSuccess},
					},
				},
				ArtifactRefs: map[string]externalFileRef{
					"gha-logs": {Path: logPath},
				},
			},
		}, nil
	}
	t.Cleanup(func() {
		backendFactory = origFactory
	})

	deps := coreops.NewOpDependenciesBuilder().
		WithGitContext(coreops.GitExecutionContext{
			BaseRef:          "main",
			ResolvedBaseHash: "deadbeef",
			WorktreePath:     worktree,
		}).
		Build()

	output, err := runs(deps, context.Background(), RunsInput{
		Workflows: []RunsWorkflowInput{
			{ID: "ci", RunInput: RunInput{Workflow: "repo://.github/workflows/ci.yml", Backend: backendAct}},
			{ID: "lint", RunInput: RunInput{Workflow: "repo://.github/workflows/ci.yml", Backend: backendAct}},
		},
		ContinueOnError: true,
	})
	require.NoError(t, err)
	require.Equal(t, statusSuccess, output.Status)
	require.True(t, output.AllPassed)
	require.Contains(t, output.Results, "ci")
	require.Contains(t, output.Results, "lint")

	refs := deps.GetExternalArtifacts()
	require.Contains(t, refs, "ci/gha-logs")
	require.Contains(t, refs, "lint/gha-logs")
}

func TestRunsContinueOnErrorCapturesPerWorkflowErrors(t *testing.T) {
	worktree := initGitRepoWithFiles(t, map[string]string{
		".github/workflows/ci.yml": "name: ci\n",
	})

	origFactory := backendFactory
	backendFactory = func(string) (workflowBackend, error) {
		return &fakeBackend{
			result: backendResult{
				Output: RunOutput{
					Status: statusSuccess,
				},
			},
		}, nil
	}
	t.Cleanup(func() {
		backendFactory = origFactory
	})

	deps := coreops.NewOpDependenciesBuilder().
		WithGitContext(coreops.GitExecutionContext{
			BaseRef:          "main",
			ResolvedBaseHash: "deadbeef",
			WorktreePath:     worktree,
		}).
		Build()

	output, err := runs(deps, context.Background(), RunsInput{
		Workflows: []RunsWorkflowInput{
			{ID: "good", RunInput: RunInput{Workflow: "repo://.github/workflows/ci.yml", Backend: backendAct}},
			{ID: "bad", RunInput: RunInput{Workflow: "repo://.github/workflows/missing.yml", Backend: backendAct}},
		},
		ContinueOnError: true,
	})
	require.NoError(t, err)
	require.Equal(t, statusFailure, output.Status)
	require.False(t, output.AllPassed)
	require.Equal(t, statusSuccess, output.Results["good"].Status)
	require.Equal(t, statusFailure, output.Results["bad"].Status)
	require.NotEmpty(t, output.Results["bad"].ErrorMessage)
}

func TestRunsRejectsMutatingWorkflows(t *testing.T) {
	worktree := initGitRepoWithFiles(t, map[string]string{
		".github/workflows/ci.yml": "name: ci\n",
	})

	origFactory := backendFactory
	backendFactory = func(string) (workflowBackend, error) {
		return workflowBackendFunc(func(_ context.Context, req backendRequest) (backendResult, error) {
			target := filepath.Join(req.GitContext.WorktreePath, "mutated.txt")
			require.NoError(t, os.WriteFile(target, []byte("changed"), 0o644))
			return backendResult{Output: RunOutput{Status: statusSuccess}}, nil
		}), nil
	}
	t.Cleanup(func() {
		backendFactory = origFactory
	})

	deps := coreops.NewOpDependenciesBuilder().
		WithGitContext(coreops.GitExecutionContext{
			BaseRef:          "main",
			ResolvedBaseHash: "deadbeef",
			WorktreePath:     worktree,
		}).
		Build()

	output, err := runs(deps, context.Background(), RunsInput{
		Workflows: []RunsWorkflowInput{
			{ID: "mutating", RunInput: RunInput{Workflow: "repo://.github/workflows/ci.yml", Backend: backendAct}},
		},
		ContinueOnError: true,
	})
	require.NoError(t, err)
	require.Equal(t, statusFailure, output.Status)
	require.Contains(t, output.Results["mutating"].ErrorMessage, "does not support workflows that mutate the worktree")
}

func TestFlattenRunJobOutputFailsWhenJobIsMissing(t *testing.T) {
	_, _, err := flattenRunJobOutput("missing", RunOutput{
		Jobs: map[string]WorkflowJobOutput{
			"other": {Status: statusSuccess},
			"lint":  {Status: statusSuccess},
		},
	})
	require.Error(t, err)
}

func TestBatchWorkflowKeyFallsBackWhenIDMissing(t *testing.T) {
	key := batchWorkflowKey(RunsWorkflowInput{
		RunInput: RunInput{Workflow: "repo://.github/workflows/ci.yml"},
	}, 0)
	require.Equal(t, "ci", key)
}

func initGitRepoWithFiles(t *testing.T, files map[string]string) string {
	t.Helper()

	repo := t.TempDir()
	runGitTest(t, repo, "init")
	runGitTest(t, repo, "config", "user.email", "test@example.com")
	runGitTest(t, repo, "config", "user.name", "Test User")

	for rel, body := range files {
		full := filepath.Join(repo, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}

	runGitTest(t, repo, "add", ".")
	runGitTest(t, repo, "commit", "-m", "init")
	return repo
}

func runGitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(output))
	return string(output)
}
