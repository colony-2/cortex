package ops

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/divisive-ai/vibethis/server/git/pkg/gitstate"
	recipeops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
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

func TestWithGitWorkspaceAppliesContextPatch(t *testing.T) {
	t.Parallel()

	repoDir, baseHash, nextHash := initTwoCommitRepo(t)
	blobStore := t.TempDir()

	controller := gitstate.NewController(nil)

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

	registration := ActivityRegistration{Activity: patchActivity, Metadata: patchActivity.GetMetadata()}
	wrapped := withGitWorkspace(registration, controller)

	inv := recipeops.NewOpDependenciesBuilder().Build()
	input := map[string]interface{}{
		"context": map[string]interface{}{
			"git": map[string]interface{}{
				"base_repo":      repoDir,
				"base_hash":      baseHash,
				"persist_hash":   nextHash,
				"worktree_path":  repoDir,
				"blob_store_uri": "file://" + filepath.ToSlash(blobStore),
			},
			"worktree":  repoDir,
			"blobstore": "file://" + filepath.ToSlash(blobStore),
			"ticketid":  "T-1",
			"cellname":  "cells/beta",
		},
	}
	raw, err := json.Marshal(input)
	require.NoError(t, err)
	workspace, err := gitstate.LegacyPayloadFromInput(inv, input)
	require.NoError(t, err)

	envelope, err := wrapped(context.Background(), ActivityInvocationRequest{Invocation: inv, Workspace: workspace})
	require.NoError(t, err)
	require.NotNil(t, envelope.Workspace.Context)
	require.NotEmpty(t, envelope.Workspace.Context.PersistHash)
}

func TestEnableActivitiesInWorkerInjectsDependencies(t *testing.T) {
	t.Parallel()

	deps := recipeops.NewServiceDepsBuilder().Build()
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
			require.Same(t, deps, inv.Deps)
			seenDeps = true
			return depOutput{Acknowledged: true}, nil
		},
	)
	require.NoError(t, Register(registry, activity))
	registry.SetDependencies(deps)

	worker := newCapturingWorker(t)
	registry.EnableActivitiesInWorker(worker)

	handler, ok := worker.handlers[activityType]
	require.True(t, ok)

	repoPath, baseHash, _ := initTwoCommitRepo(t)
	worktreeDir := filepath.Join(t.TempDir(), "work")
	blobDir := t.TempDir()
	input := map[string]interface{}{
		"message": "hi",
		"context": map[string]interface{}{
			"git": map[string]interface{}{
				"base_repo":    repoPath,
				"base_hash":    baseHash,
				"persist_hash": baseHash,
			},
			"worktree":  worktreeDir,
			"blobstore": "file://" + filepath.ToSlash(blobDir),
			"ticketid":  "TEST-1",
			"cellname":  "cells/cell-a",
		},
		"git_persist_hash": baseHash,
	}
	rawInput, err := json.Marshal(input)
	require.NoError(t, err)
	wp, err := gitstate.LegacyPayloadFromInput(recipeops.OpDependencies{}, input)
	require.NoError(t, err)
	_, err = handler(context.Background(), ActivityInvocationRequest{Invocation: recipeops.OpDependencies{}, OpInput: rawInput, Workspace: wp})
	require.NoError(t, err)
	require.True(t, seenDeps)
}

type capturingWorker struct {
	t        *testing.T
	handlers map[string]func(context.Context, ActivityInvocationRequest) (ActivityInvocationOutput, error)
}

func newCapturingWorker(t *testing.T) *capturingWorker {
	return &capturingWorker{t: t, handlers: make(map[string]func(context.Context, ActivityInvocationRequest) (ActivityInvocationOutput, error))}
}

func (c *capturingWorker) RegisterActivityWithOptions(a interface{}, options activity.RegisterOptions) {
	handler, ok := a.(func(context.Context, ActivityInvocationRequest) (ActivityInvocationOutput, error))
	if !ok {
		return
	}
	c.handlers[options.Name] = handler
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
		registration, exists := registry.Get("test_registry_activity")
		assert.True(t, exists)
		assert.NotNil(t, registration.Activity)
		assert.NotNil(t, registration.InputSchema)
		assert.NotNil(t, registration.OutputSchema)
		assert.Equal(t, "test_registry_activity", registration.Metadata.Type)
	})

	t.Run("duplicate registration fails", func(t *testing.T) {
		err := Register(registry, testActivity)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("list registered activities", func(t *testing.T) {
		types := registry.List()
		assert.Contains(t, types, "test_registry_activity")
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

	registration, exists := registry.Get("test_registry_activity")
	require.True(t, exists)

	// Config schema test removed - no longer part of ActivityRegistration

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
