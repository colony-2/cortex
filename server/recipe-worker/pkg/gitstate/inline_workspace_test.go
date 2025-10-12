package gitstate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
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
		"cell_name": "alpha",
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
			"cellname":  "alpha",
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
		res, err := WithInlineWorkspace(ctx, inv, inputs, InlineWorkspaceOptions{}, func(inner workflow.Context, childInputs map[string]interface{}) (map[string]interface{}, error) {
			gitMap := childInputs["context"].(map[string]interface{})["git"].(map[string]interface{})
			worktreePath := gitMap["worktree_path"].(string)
			laOpts := workflow.LocalActivityOptions{
				StartToCloseTimeout: time.Minute,
			}
			inner = workflow.WithLocalActivityOptions(inner, laOpts)
			var ignore interface{}
			future := workflow.ExecuteLocalActivity(inner, inlineWriteFileActivity, inlineWriteFileArgs{
				Dir:     worktreePath,
				Name:    "child.txt",
				Content: "nested",
			})
			if err := future.Get(inner, &ignore); err != nil {
				return nil, err
			}
			return map[string]interface{}{"status": "ok"}, nil
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

	require.Equal(t, "ok", result.Result["status"])
	require.NotEmpty(t, result.GitContext.PersistHash)
	require.NotEqual(t, baseHash, result.GitContext.PersistHash)
	require.Equal(t, result.GitContext.PersistHash, result.ContextMap["git"].(map[string]interface{})["persist_hash"])
	require.Equal(t, baseHash, result.ContextMap["git"].(map[string]interface{})["previous_hash"])
	thinPack := result.GitContext.ThinPackPath
	require.NotEmpty(t, thinPack)
	packAbs := filepath.Join(blobStore, filepath.FromSlash(thinPack))
	stat, err := os.Stat(packAbs)
	require.NoError(t, err)
	require.Greater(t, stat.Size(), int64(0))

	childWorktree := result.GitContext.WorktreePath
	require.True(t, strings.HasPrefix(childWorktree, filepath.Dir(parentWorktree)))
	require.Equal(t, "work", filepath.Base(childWorktree))
	require.DirExists(t, filepath.Dir(childWorktree))
	_, err = os.Stat(filepath.Join(childWorktree, "child.txt"))
	require.NoError(t, err)
	gitMap := result.ContextMap["git"].(map[string]interface{})
	if prepared, ok := gitMap["workspace_prepared"].(bool); ok {
		require.True(t, prepared)
	}
	require.Equal(t, childWorktree, result.ContextMap["worktree"])
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
		res, err := WithInlineWorkspace(ctx, inv, inputs, InlineWorkspaceOptions{SkipFinalize: true}, func(inner workflow.Context, childInputs map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{
				"status":       "skipped",
				"context_copy": childInputs["context"],
			}, nil
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

	require.Equal(t, "skipped", result.Result["status"])
	require.Nil(t, result.ContextMap)
	require.Equal(t, baseHash, result.GitContext.PersistHash)
	ctxCopy, ok := result.Result["context_copy"].(map[string]interface{})
	require.True(t, ok)
	gitCopy, ok := ctxCopy["git"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, baseHash, gitCopy["persist_hash"])
}
