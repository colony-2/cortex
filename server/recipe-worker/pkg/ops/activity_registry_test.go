package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/git/pkg/gitstate"
	recipeops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/activity"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Test types with proper JSON tags
type TestConfig struct {
	URL     string `json:"url"`
	Timeout int    `json:"timeout"`
}

type TestInput struct {
	Data string `json:"data"`
	Size int    `json:"size"`
}

type TestOutput struct {
	Result  string `json:"result"`
	Success bool   `json:"success"`
}

// Test types missing JSON tags (should fail registration)
type BadConfig struct {
	URL     string // Missing json tag
	Timeout int    `json:"timeout"`
}

type BadInput struct {
	Data string `json:"data"`
	Size int    // Missing json tag
}

// testActivity is a mock RegisterableOp for testing
var testActivity = recipeops.NewActivityMappedOpV2[TestInput, TestOutput](
	recipeops.OpMetadata{
		Type:           "test_registry_activity",
		Description:    "A test activity for unit testing",
		Version:        "1.0.0",
		DefaultTimeout: 30 * time.Second,
	},
	func(inv recipeops.OpDependencies, ctx context.Context, input TestInput) (TestOutput, error) {
		return runTestActivity(inv, ctx, input)
	},
)

func runTestActivity(_ recipeops.OpDependencies, ctx context.Context, input TestInput) (TestOutput, error) {
	return TestOutput{
		Result:  input.Data + " processed",
		Success: true,
	}, nil
}

func TestNewNoTaskStepIsDisallowedAndNotRegisteredAsWorker(t *testing.T) {
	orig := recipeops.List()
	recipeops.Clear()
	t.Cleanup(func() {
		recipeops.Clear()
		if len(orig) > 0 {
			recipeops.Register(orig...)
		}
	})

	type stepIn struct {
		Name string `json:"name"`
	}
	type stepOut struct {
		Confirmed bool `json:"confirmed"`
	}

	op, err := recipeops.NewOp().
		WithType("no-task-op").
		AddStep("collect", recipeops.NewNoTaskStep[stepIn, stepOut]()).
		Build()
	require.NoError(t, err)
	recipeops.Register(op.(recipeops.RegisterableOp))

	registry, err := NewActivityRegistry()
	require.NoError(t, err)

	all := registry.GetAll()
	require.Contains(t, all, "no-task-op:collect")
	entry := all["no-task-op:collect"]
	require.True(t, entry.Step.DisallowAsTask, "NoTaskStep should be marked disallowed")

	workers := registry.GetTaskWorkers(recipeops.NewServiceDepsBuilder().Build())
	require.Empty(t, workers, "disallowed steps should not be exposed as task workers")
}

func TestWithGitWorkspaceAppliesContextPatch(t *testing.T) {
	t.Parallel()

	repoDir, baseHash, nextHash := initTwoCommitRepo(t)
	blobStore := t.TempDir()

	controller := gitstate.NewController(nil)
	deps := recipeops.NewServiceDepsBuilder().Build()

	newBase := nextHash
	patchActivity := recipeops.NewActivityMappedOpV2[struct {
		Context map[string]interface{} `json:"context,omitempty"`
	}, struct {
		GitContextPatch map[string]interface{} `json:"git_context_patch,omitempty"`
	}](
		recipeops.OpMetadata{
			Type:        "test_git_context_patch",
			Description: "emits git context patch",
			Version:     "1.0.0",
		},
		func(inv recipeops.OpDependencies, ctx context.Context, input struct {
			Context map[string]interface{} `json:"context,omitempty"`
		}) (struct {
			GitContextPatch map[string]interface{} `json:"git_context_patch,omitempty"`
		}, error) {
			return struct {
				GitContextPatch map[string]interface{} `json:"git_context_patch,omitempty"`
			}{
				GitContextPatch: map[string]interface{}{"base_hash": newBase},
			}, nil
		},
	)

	step := patchActivity.TaskChain()[0]
	registration := ActivityRegistration{
		Activity:  patchActivity,
		Step:      step,
		StepIndex: 0,
		TaskType:  fmt.Sprintf("%s:%s", patchActivity.GetMetadata().Type, step.Name),
		Metadata:  patchActivity.GetMetadata(),
	}
	wrapped := withGitWorkspace(deps, registration, controller)

	worktreePath := filepath.Join(t.TempDir(), "worktree")
	output, artifacts, err := wrapped(context.Background(), ActivityInvocationRequest{
		Input: map[string]interface{}{
			"context": map[string]interface{}{
				"git": map[string]interface{}{
					"base_hash":    baseHash,
					"persist_hash": baseHash,
				},
			},
		},
		GitTaskContext: gitstate.GitTaskContext{
			BaseRepo:     repoDir,
			BaseHash:     baseHash,
			PersistHash:  baseHash,
			WorktreePath: worktreePath,
			BlobStoreURI: "file://" + filepath.ToSlash(blobStore),
			TicketID:     "T-1",
			CellName:     "cells/beta",
		},
	}, nil)
	require.NoError(t, err)
	require.Empty(t, artifacts)

	patch, ok := output.OpOutput["git_context_patch"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, newBase, patch["base_hash"])
	assert.NotEmpty(t, output.GitResult.PersistHash)
}

func TestEnableActivitiesInWorkerInjectsDependencies(t *testing.T) {
	t.Parallel()

	db := &gorm.DB{}
	wc := &stubWorkflowControl{}
	deps := recipeops.NewServiceDepsBuilder().
		WithDatabase(db).
		WithWorkflowControl(wc).
		Build()
	registry, err := NewActivityRegistry()
	require.NoError(t, err)

	type depInput struct {
		Message string `json:"message"`
	}
	type depOutput struct {
		Acknowledged bool `json:"acknowledged"`
	}

	const activityType = "deps_injection_check"
	seenDeps := false
	activity := recipeops.NewActivityMappedOpV2[depInput, depOutput](
		recipeops.OpMetadata{Type: activityType, Description: "ensure deps present", Version: "1.0.0"},
		func(inv recipeops.OpDependencies, ctx context.Context, input depInput) (depOutput, error) {
			require.Same(t, db, inv.Database())
			require.Same(t, wc, inv.WorkflowControl())
			seenDeps = true
			return depOutput{Acknowledged: true}, nil
		},
	)
	require.NoError(t, Register(registry, activity))

	worker := newCapturingWorker(t)
	registry.EnableActivitiesInWorker(deps, worker)

	taskType := fmt.Sprintf("%s:%s", activityType, activityType)
	handler, ok := worker.handlers[taskType]
	require.True(t, ok)

	repoPath, baseHash, _ := initTwoCommitRepo(t)
	worktreeDir := filepath.Join(t.TempDir(), "work")
	blobDir := t.TempDir()
	input := map[string]interface{}{"message": "hi"}
	_, _, err = handler(context.Background(), ActivityInvocationRequest{
		Input: input,
		GitTaskContext: gitstate.GitTaskContext{
			BaseRepo:     repoPath,
			BaseHash:     baseHash,
			PersistHash:  baseHash,
			WorktreePath: worktreeDir,
			BlobStoreURI: "file://" + filepath.ToSlash(blobDir),
			TicketID:     "TEST-1",
			CellName:     "cells/cell-a",
		},
	}, nil)
	require.NoError(t, err)
	require.True(t, seenDeps)
}

type capturingWorker struct {
	t        *testing.T
	handlers map[string]func(context.Context, ActivityInvocationRequest, []swf.Artifact) (ActivityInvocationOutput, []swf.Artifact, error)
}

func newCapturingWorker(t *testing.T) *capturingWorker {
	return &capturingWorker{t: t, handlers: make(map[string]func(context.Context, ActivityInvocationRequest, []swf.Artifact) (ActivityInvocationOutput, []swf.Artifact, error))}
}

func (c *capturingWorker) RegisterActivityWithOptions(a interface{}, options activity.RegisterOptions) {
	handler, ok := a.(func(context.Context, ActivityInvocationRequest, []swf.Artifact) (ActivityInvocationOutput, []swf.Artifact, error))
	if !ok {
		return
	}
	c.handlers[options.Name] = handler
}

type stubWorkflowControl struct{}

func (s *stubWorkflowControl) CompleteTask(ctx context.Context, jobId swf.JobId, taskOrdinal int64, hash string, data any) error {
	return nil
}

var _ workflowctl.WorkflowControl = &stubWorkflowControl{}

func (s *stubWorkflowControl) StartJob(ctx context.Context, req workflowctl.StartJob) (swf.JobId, error) {
	_ = ctx
	_ = req
	return swf.JobId(""), nil
}

func (s *stubWorkflowControl) Cancel(ctx context.Context, jobId swf.JobId) error {
	_ = ctx
	_ = jobId
	return nil
}

func (s *stubWorkflowControl) ListJobs(ctx context.Context, request swf.ListJobsRequest) ([]workflowctl.JobItem, string, error) {
	_ = ctx
	_ = request
	return nil, "", nil
}

func initTwoCommitRepo(t *testing.T) (string, string, string) {
	t.Helper()

	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	runGitCmd(t, root, "git", "init", repoDir)
	runGitCmd(t, repoDir, "git", "config", "user.name", "Tester")
	runGitCmd(t, repoDir, "git", "config", "user.email", "tester@example.com")
	writeFile(t, repoDir, "README.md", "first\n")
	for _, cell := range []string{"beta", "cell-a", "test-cell"} {
		writeFile(t, repoDir, filepath.Join("cells", cell, "README.md"), cell+"\n")
	}
	runGitCmd(t, repoDir, "git", "add", ".")
	runGitCmd(t, repoDir, "git", "commit", "-m", "first")
	baseHash := strings.TrimSpace(runGitCmd(t, repoDir, "git", "rev-parse", "HEAD"))
	runGitCmd(t, repoDir, "git", "checkout", "-B", "main")
	writeFile(t, repoDir, "README.md", "second\n")
	runGitCmd(t, repoDir, "git", "add", ".")
	runGitCmd(t, repoDir, "git", "commit", "-m", "second")
	nextHash := strings.TrimSpace(runGitCmd(t, repoDir, "git", "rev-parse", "HEAD"))
	return repoDir, baseHash, nextHash
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
}

func runGitCmd(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, output)
	}
	return string(output)
}

func TestActivityRegistration(t *testing.T) {
	registry, err := NewActivityRegistry()
	require.NoError(t, err)

	t.Run("successful registration", func(t *testing.T) {
		err := Register(registry, testActivity)
		assert.NoError(t, err)

		// Verify activity was registered
		taskType := "test_registry_activity:test_registry_activity"
		registration, exists := registry.Get(taskType)
		assert.True(t, exists)
		assert.NotNil(t, registration.Activity)
		assert.NotNil(t, registration.InputSchema)
		assert.NotNil(t, registration.OutputSchema)
		assert.Equal(t, "test_registry_activity", registration.Metadata.Type)
		assert.Equal(t, "test_registry_activity", registration.Step.Name)
	})

	t.Run("duplicate registration fails", func(t *testing.T) {
		err := Register(registry, testActivity)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("list registered activities", func(t *testing.T) {
		types := registry.List()
		assert.Contains(t, types, "test_registry_activity:test_registry_activity")
	})
}

// BadActivity for testing validation failures
type BadActivity struct{}

func (a *BadActivity) GetMetadata() recipeops.OpMetadata {
	return recipeops.OpMetadata{
		Type:           "bad_activity",
		Description:    "Test activity with bad types",
		Version:        "1.0.0",
		DefaultTimeout: 30 * time.Second,
	}
}

func (a *BadActivity) Execute(ctx context.Context, input BadInput) (TestOutput, error) {
	return TestOutput{}, nil
}

func TestJSONTagValidation(t *testing.T) {
	// Test the schema generator directly for validation
	generator := NewDefaultSchemaGenerator()

	t.Run("missing config json tag fails", func(t *testing.T) {
		err := generator.ValidateStructTags(reflect.TypeOf(BadConfig{}))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing required json tag")
		assert.Contains(t, err.Error(), "URL")
	})

	t.Run("missing input json tag fails", func(t *testing.T) {
		err := generator.ValidateStructTags(reflect.TypeOf(BadInput{}))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing required json tag")
		assert.Contains(t, err.Error(), "Size")
	})

	t.Run("nested struct validation", func(t *testing.T) {
		type NestedBad struct {
			Field string // Missing json tag
		}

		type ConfigWithNested struct {
			Name   string    `json:"name"`
			Nested NestedBad `json:"nested"`
		}

		err := generator.ValidateStructTags(reflect.TypeOf(ConfigWithNested{}))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing required json tag")
		assert.Contains(t, err.Error(), "Field")
	})

	t.Run("ignored fields are allowed", func(t *testing.T) {
		type ConfigWithIgnored struct {
			Public  string `json:"public"`
			Private string `json:"-"` // Explicitly ignored
		}

		err := generator.ValidateStructTags(reflect.TypeOf(ConfigWithIgnored{}))
		assert.NoError(t, err)
	})

	t.Run("all fields with tags pass", func(t *testing.T) {
		err := generator.ValidateStructTags(reflect.TypeOf(TestConfig{}))
		assert.NoError(t, err)

		err = generator.ValidateStructTags(reflect.TypeOf(TestInput{}))
		assert.NoError(t, err)

		err = generator.ValidateStructTags(reflect.TypeOf(TestOutput{}))
		assert.NoError(t, err)
	})
}

func TestSchemaGeneration(t *testing.T) {
	registry, err := NewActivityRegistry()
	require.NoError(t, err)

	err = Register(registry, testActivity)
	require.NoError(t, err)

	registration, exists := registry.Get("test_registry_activity:test_registry_activity")
	require.True(t, exists)

	// Form schema test removed - no longer part of ActivityRegistration

	t.Run("input schema", func(t *testing.T) {
		schema := registration.InputSchema
		assert.NotNil(t, schema)

		schemaJSON, err := json.Marshal(schema)
		require.NoError(t, err)

		var schemaMap map[string]interface{}
		err = json.Unmarshal(schemaJSON, &schemaMap)
		require.NoError(t, err)

		properties, ok := schemaMap["properties"].(map[string]interface{})
		assert.True(t, ok)
		assert.Contains(t, properties, "data")
		assert.Contains(t, properties, "size")
	})
}

// TestActivityProvider tests are commented out to avoid circular import
// These tests should be moved to a separate test package or integration tests
// func TestActivityProvider(t *testing.T) {
// 	registry := NewActivityRegistry()
//
// 	err := Register[TestInput, TestOutput](registry, testActivity)
// 	require.NoError(t, err)
//
// 	registration, _ := registry.Get("test_activity")
// 	provider := worker.NewActivityProvider(testActivity, registration)
//
// 	t.Run("provider metadata", func(t *testing.T) {
// 		assert.Equal(t, "test_activity", provider.GetType())
// 		assert.Equal(t, "A test activity for unit testing", provider.GetDescription())
// 	})
//
// 	t.Run("provider execution", func(t *testing.T) {
// 		ctx := context.Background()
//
// 		config := map[string]interface{}{
// 			"url":     "https://example.com",
// 			"timeout": 30,
// 		}
//
// 		input := map[string]interface{}{
// 			"data": "test data",
// 			"size": 100,
// 		}
//
// 		result, err := provider.Run(ctx, config, input)
// 		assert.NoError(t, err)
//
// 		output, ok := result.(map[string]interface{})
// 		assert.True(t, ok)
// 		assert.Equal(t, "test data processed", output["result"])
// 		assert.Equal(t, true, output["success"])
// 	})
//
// 	t.Run("provider schemas", func(t *testing.T) {
// 		configSchema, inputSchema, outputSchema := provider.GetSchemas()
// 		assert.NotNil(t, configSchema)
// 		assert.NotNil(t, inputSchema)
// 		assert.NotNil(t, outputSchema)
// 	})
// }
