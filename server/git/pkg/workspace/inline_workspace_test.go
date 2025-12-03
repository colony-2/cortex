//go:build futureworkspace

package workspace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflow"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
)

type inlineWriteFileArgs struct {
	Dir     string
	Name    string
	Content string
}

func inlineWriteFileActivity(ctx context.Context, args inlineWriteFileArgs) error {
	fullPath := filepath.Join(args.Dir, args.Name)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(fullPath, []byte(args.Content), 0o644)
}

func TestWithInlineWorkspaceLifecycle(t *testing.T) {
	t.Parallel()

	baseRepo, baseHash, cleanup := setupGitRepo(t)
	defer cleanup()

	workspaceRoot := t.TempDir()
	blobStore := t.TempDir()

	parentWorktree := filepath.Join(workspaceRoot, "run-parent", "work")
	inputs := map[string]interface{}{
		"ticket_id": "TICKET-123",
		"cell_name": "cells/alpha",
		"context": map[string]interface{}{
			"git": map[string]interface{}{
				"base_repo":      baseRepo,
				"base_hash":      baseHash,
				"persist_hash":   baseHash,
				"previous_hash":  baseHash,
				"blob_store_uri": "file://" + filepath.ToSlash(blobStore),
				"worktree_path":  parentWorktree,
			},
			"worktree":  parentWorktree,
			"blobstore": "file://" + filepath.ToSlash(blobStore),
			"ticketid":  "TICKET-123",
			"cellname":  "cells/alpha",
			"recipe": map[string]interface{}{
				"id":        "recipe.parent",
				"node_path": "root",
			},
		},
		"git_persist_hash": baseHash,
	}

	inv := coreops.Invocation{
		RecipeID:  "recipe.parent",
		NodePath:  "root.sequence.child",
		InvokeSeq: 1,
		ID:        "child-inv",
	}

	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)
	primeRecipeMetadataSignal(env)

	env.RegisterActivity(inlineWriteFileActivity)

	env.ExecuteWorkflow(func(ctx workflow.Context) (InlineWorkspaceResult, error) {
		payload, err := LegacyPayloadFromInput(inv, inputs)
		require.NoError(t, err)
		res, err := WithInlineWorkspace(ctx, inv, payload, InlineWorkspaceOptions{}, func(inner workflow.Context, childPayload WorkspacePayload) (json.RawMessage, error) {
			worktreePath := childPayload.Context.WorktreePath
			laOpts := workflow.LocalActivityOptions{
				StartToCloseTimeout: time.Minute,
			}
			inner = workflow.WithLocalActivityOptions(inner, laOpts)
			var ignore interface{}
			future := workflow.ExecuteLocalActivity(inner, inlineWriteFileActivity, inlineWriteFileArgs{
				Dir:     worktreePath,
				Name:    filepath.Join("cells", "alpha", "child.txt"),
				Content: "nested",
			})
			if err := future.Get(inner, &ignore); err != nil {
				return nil, err
			}
			return json.RawMessage(`{"status":"ok"}`), nil
		})
		if err != nil {
			return InlineWorkspaceResult{}, err
		}
		return *res, nil
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result InlineWorkspaceResult
	require.NoError(t, env.GetWorkflowResult(&result))

	var outMap map[string]interface{}
	require.NoError(t, json.Unmarshal(result.Result, &outMap))
	require.Equal(t, "ok", outMap["status"])
	require.NotEmpty(t, result.Context.PersistHash)
	require.NotEqual(t, baseHash, result.Context.PersistHash)
	require.Equal(t, result.Context.PersistHash, result.Context.PersistHash)
	require.Equal(t, baseHash, result.Context.PreviousHash)
	thinPack := result.Context.ThinPackPath
	require.NotEmpty(t, thinPack)
	packAbs := filepath.Join(blobStore, filepath.FromSlash(thinPack))
	stat, err := os.Stat(packAbs)
	require.NoError(t, err)
	require.Greater(t, stat.Size(), int64(0))

	childWorktree := result.Context.WorktreePath
	require.True(t, strings.HasPrefix(childWorktree, filepath.Dir(parentWorktree)))
	require.Equal(t, "work", filepath.Base(childWorktree))
	require.DirExists(t, filepath.Dir(childWorktree))
	_, err = os.Stat(filepath.Join(childWorktree, "cells", "alpha", "child.txt"))
	require.NoError(t, err)
	require.True(t, result.Context.WorkspacePrepared)
	require.Equal(t, childWorktree, result.Context.WorktreePath)
}

func TestWithInlineWorkspaceSkipFinalize(t *testing.T) {
	t.Parallel()

	baseRepo, baseHash, cleanup := setupGitRepo(t)
	defer cleanup()

	workspaceRoot := t.TempDir()
	blobStore := t.TempDir()
	parentWorktree := filepath.Join(workspaceRoot, "run-parent", "work")

	inputs := map[string]interface{}{
		"context": map[string]interface{}{
			"git": map[string]interface{}{
				"base_repo":      baseRepo,
				"base_hash":      baseHash,
				"persist_hash":   baseHash,
				"previous_hash":  baseHash,
				"blob_store_uri": "file://" + filepath.ToSlash(blobStore),
				"worktree_path":  parentWorktree,
			},
			"worktree":  parentWorktree,
			"blobstore": "file://" + filepath.ToSlash(blobStore),
		},
		"git_persist_hash": baseHash,
	}

	inv := coreops.Invocation{
		RecipeID:  "recipe.parent",
		NodePath:  "root.sequence.child",
		InvokeSeq: 1,
		ID:        "child-inv",
	}

	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)
	primeRecipeMetadataSignal(env)

	env.ExecuteWorkflow(func(ctx workflow.Context) (InlineWorkspaceResult, error) {
		payload, err := LegacyPayloadFromInput(inv, inputs)
		require.NoError(t, err)
		res, err := WithInlineWorkspace(ctx, inv, payload, InlineWorkspaceOptions{SkipFinalize: true}, func(inner workflow.Context, childPayload WorkspacePayload) (json.RawMessage, error) {
			return json.Marshal(map[string]interface{}{
				"status":       "skipped",
				"context_copy": childPayload.Context.ToMap(),
			})
		})
		if err != nil {
			return InlineWorkspaceResult{}, err
		}
		return *res, nil
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result InlineWorkspaceResult
	require.NoError(t, env.GetWorkflowResult(&result))

	require.Equal(t, baseHash, result.Context.PersistHash)
	// When skip finalize is enabled, result payload may be empty; primary assertion is context persistence.
}
