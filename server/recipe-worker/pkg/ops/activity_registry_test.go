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

	recipeops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/gitstate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	func(inv recipeops.Invocation, ctx context.Context, input TestInput) (TestOutput, error) {
		return runTestActivity(inv, ctx, input)
	},
)

func runTestActivity(_ recipeops.Invocation, ctx context.Context, input TestInput) (TestOutput, error) {
	return TestOutput{
		Result:  input.Data + " processed",
		Success: true,
	}, nil
}

type stubDeps struct{}

func (s *stubDeps) WorkflowControl() (workflowctl.WorkflowControl, bool) {
	return nil, false
}

func (s *stubDeps) SSEManager() (recipeops.SSEManager, bool) {
	return nil, false
}

func (s *stubDeps) TemporalNamespace() (string, bool) {
	return "", false
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
		func(inv recipeops.Invocation, ctx context.Context, input struct {
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
	wrapped := withGitWorkspace(registration, controller, nil)

	inv := recipeops.Invocation{RecipeID: "recipe.test", NodePath: "sequence.node", InvokeSeq: 1}
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
			"cellname":  "beta",
		},
	}

	outputs, err := wrapped(context.Background(), ActivityInvocationRequest{Invocation: inv, Input: input})
	require.NoError(t, err)
	require.NotNil(t, outputs)
	require.NotContains(t, outputs, "git_context_patch")

	ctxMap := outputs["context"].(map[string]interface{})
	gitMap := ctxMap["git"].(map[string]interface{})
	require.Equal(t, newBase, gitMap["base_hash"])
	require.Equal(t, outputs["git_persist_hash"], gitMap["persist_hash"])
	// ensure new thin pack recorded relative to blobstore
	thinPack, ok := gitMap["thin_pack_path"].(string)
	require.True(t, ok)
	require.NotEmpty(t, thinPack)
	stat, err := os.Stat(filepath.Join(blobStore, filepath.FromSlash(thinPack)))
	require.NoError(t, err)
	require.True(t, stat.Mode().IsRegular())
}

func TestWithGitWorkspaceInjectsDependencies(t *testing.T) {
	t.Parallel()

	deps := &stubDeps{}
	repoDir, baseHash, persistHash := initTwoCommitRepo(t)
	blobStore := t.TempDir()
	worktree := filepath.Join(t.TempDir(), "worktree")

	activity := recipeops.NewActivityMappedOpV2[TestInput, TestOutput](
		recipeops.OpMetadata{Type: "deps_check", Description: "ensure deps present", Version: "1.0.0"},
		func(inv recipeops.Invocation, ctx context.Context, input TestInput) (TestOutput, error) {
			require.NotNil(t, inv.Deps)
			return runTestActivity(inv, ctx, input)
		},
	)
	registration := ActivityRegistration{Activity: activity, Metadata: activity.GetMetadata()}

	wrapped := withGitWorkspace(registration, nil, deps)
	input := map[string]interface{}{
		"context": map[string]interface{}{
			"git": map[string]interface{}{
				"base_repo":      repoDir,
				"base_hash":      baseHash,
				"persist_hash":   persistHash,
				"worktree_path":  worktree,
				"blob_store_uri": "file://" + filepath.ToSlash(blobStore),
			},
		},
	}
	_, err := wrapped(context.Background(), ActivityInvocationRequest{Invocation: recipeops.Invocation{}, Input: input})
	require.NoError(t, err)
}

func initTwoCommitRepo(t *testing.T) (string, string, string) {
	t.Helper()

	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	runGitCmd(t, root, "git", "init", repoDir)
	runGitCmd(t, repoDir, "git", "config", "user.name", "Tester")
	runGitCmd(t, repoDir, "git", "config", "user.email", "tester@example.com")
	writeFile(t, repoDir, "README.md", "first\n")
	runGitCmd(t, repoDir, "git", "add", "README.md")
	runGitCmd(t, repoDir, "git", "commit", "-m", "first")
	baseHash := strings.TrimSpace(runGitCmd(t, repoDir, "git", "rev-parse", "HEAD"))
	runGitCmd(t, repoDir, "git", "checkout", "-B", "main")
	writeFile(t, repoDir, "README.md", "second\n")
	runGitCmd(t, repoDir, "git", "add", "README.md")
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
