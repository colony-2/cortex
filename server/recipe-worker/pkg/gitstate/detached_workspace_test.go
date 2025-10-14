package gitstate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
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
		res, err := WithDetachedWorkspace(ctx, inv, inputs, DetachedWorkspaceOptions{}, func(inner workflow.Context, childInputs map[string]interface{}) (map[string]interface{}, error) {
			gitMap := childInputs["context"].(map[string]interface{})["git"].(map[string]interface{})
			worktree := gitMap["worktree_path"].(string)
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
			return map[string]interface{}{"status": "ok"}, nil
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
	require.Equal(t, "ok", result.Result["status"])
	require.NotEqual(t, baseHash, result.GitContext.PersistHash)

	gitMap := result.ContextMap["git"].(map[string]interface{})
	worktreePath := gitMap["worktree_path"].(string)

	stat, err := os.Stat(filepath.Join(worktreePath, "cells", "beta", "child.txt"))
	require.NoError(t, err)
	require.False(t, stat.IsDir())

	_, err = os.Stat(filepath.Join(parentWorktree, "cells", "beta", "child.txt"))
	require.Error(t, err)
}
