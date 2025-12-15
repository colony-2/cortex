//go:build futureworkspace

package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflow"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
)

func TestWithDetachedWorkspaceLifecycle(t *testing.T) {
	t.Parallel()

	baseRepo, baseHash, cleanup := setupGitRepo(t)
	defer cleanup()

	workspaceRoot := t.TempDir()
	blobStore := t.TempDir()
	parentWorktree := filepath.Join(workspaceRoot, "parent", "work")

	inputs := map[string]interface{}{
		"ticket_id": "TICKET-DETACHED",
		"cell_name": "cells/beta",
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
			"ticketid":  "TICKET-DETACHED",
			"cellname":  "cells/beta",
		},
		"git_persist_hash": baseHash,
	}

	inv := coreops.Invocation{RecipeID: "recipe.parent", NodePath: "parent.sequence", InvokeSeq: 1, ID: "inv-1"}

	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	defer env.AssertExpectations(t)
	primeRecipeMetadataSignal(env)

	env.RegisterActivity(inlineWriteFileActivity)

	env.ExecuteWorkflow(func(ctx workflow.Context) (*DetachedWorkspaceResult, error) {
		payload, err := LegacyPayloadFromInput(inv, inputs)
		require.NoError(t, err)
		res, err := WithDetachedWorkspace(ctx, inv, payload, DetachedWorkspaceOptions{}, func(inner workflow.Context, childPayload WorkspacePayload) (json.RawMessage, error) {
			worktree := childPayload.Context.WorktreePath
			laOpts := workflow.LocalActivityOptions{StartToCloseTimeout: time.Minute}
			inner = workflow.WithLocalActivityOptions(inner, laOpts)
			var ignore interface{}
			err := workflow.ExecuteLocalActivity(inner, inlineWriteFileActivity, inlineWriteFileArgs{
				Dir:     worktree,
				Name:    filepath.Join("cells", "beta", "child.txt"),
				Content: "detached",
			}).Get(inner, &ignore)
			if err != nil {
				return nil, err
			}
			return json.RawMessage(`{"status":"ok"}`), nil
		})
		if err != nil {
			return nil, err
		}
		return res, nil
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result *DetachedWorkspaceResult
	require.NoError(t, env.GetWorkflowResult(&result))
	require.NotNil(t, result)
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(result.Result, &out))
	require.Equal(t, "ok", out["status"])
	require.NotEqual(t, baseHash, result.Context.PersistHash)

	worktreePath := result.Context.WorktreePath

	stat, err := os.Stat(filepath.Join(worktreePath, "cells", "beta", "child.txt"))
	require.NoError(t, err)
	require.False(t, stat.IsDir())

	_, err = os.Stat(filepath.Join(parentWorktree, "cells", "beta", "child.txt"))
	require.Error(t, err)
}
