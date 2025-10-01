package gitstate

import (
	"fmt"

	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
)

// ContextFromRequest reconstructs a git execution context from the invocation and input payload.
func ContextFromRequest(inv coreops.Invocation, input map[string]interface{}) (*Context, error) {
	ctxMap, err := mapFromAny(input, "context")
	if err != nil {
		return nil, err
	}

	gitMap, err := mapFromAny(ctxMap, "git")
	if err != nil {
		return nil, err
	}

	gitCtx := &Context{}
	if err := gitCtx.UpdateFromMap(gitMap); err != nil {
		return nil, err
	}

	if gitCtx.WorktreePath == "" {
		if val, ok := stringFromMap(ctxMap, "worktree"); ok {
			gitCtx.WorktreePath = val
		}
	}

	if gitCtx.BlobStoreURI == "" {
		if val, ok := stringFromMap(ctxMap, "blobstore"); ok {
			gitCtx.BlobStoreURI = val
		}
	}

	if gitCtx.TicketID == "" {
		if val, ok := stringFromMap(ctxMap, "ticketid"); ok {
			gitCtx.TicketID = val
		} else if val, ok := stringFromMap(input, "ticket_id"); ok {
			gitCtx.TicketID = val
		}
	}

	if gitCtx.CellName == "" {
		if val, ok := stringFromMap(ctxMap, "cellname"); ok {
			gitCtx.CellName = val
		} else if val, ok := stringFromMap(input, "cell_name"); ok {
			gitCtx.CellName = val
		}
	}

	if recipeMeta, ok := mapFromAnyOptional(ctxMap, "recipe"); ok {
		if gitCtx.RecipeID == "" {
			if val, ok := stringFromMap(recipeMeta, "id"); ok {
				gitCtx.RecipeID = val
			}
		}
		if gitCtx.RecipeNode == "" {
			if val, ok := stringFromMap(recipeMeta, "node_path"); ok {
				gitCtx.RecipeNode = val
			}
		}
		if gitCtx.WorkflowID == "" {
			if val, ok := stringFromMap(recipeMeta, "workflow_id"); ok {
				gitCtx.WorkflowID = val
			}
		}
		if gitCtx.WorkflowRunID == "" {
			if val, ok := stringFromMap(recipeMeta, "workflow_run_id"); ok {
				gitCtx.WorkflowRunID = val
			}
		}
		if gitCtx.InvocationHash == "" {
			if val, ok := stringFromMap(recipeMeta, "invocation_hash"); ok {
				gitCtx.InvocationHash = val
			}
		}
		if gitCtx.InvocationID == "" {
			if val, ok := stringFromMap(recipeMeta, "invocation_id"); ok {
				gitCtx.InvocationID = val
			}
		}
		if gitCtx.InvocationAttempt == 0 {
			if val, ok := intFromMap(recipeMeta, "invocation_attempt"); ok {
				gitCtx.InvocationAttempt = val
			}
		}
	}

	if val, ok := stringFromMap(input, "git_author"); ok {
		gitCtx.GitAuthor = val
	}

	gitCtx.BoxID = firstNonEmpty(gitCtx.BoxID, inv.BoxID)
	gitCtx.ActivityID = firstNonEmpty(gitCtx.ActivityID, inv.ActivityID)
	gitCtx.RecipeID = firstNonEmpty(gitCtx.RecipeID, inv.RecipeID)
	gitCtx.RecipeNode = firstNonEmpty(gitCtx.RecipeNode, inv.NodePath)
	gitCtx.InvocationID = firstNonEmpty(gitCtx.InvocationID, inv.ID)
	if gitCtx.InvocationHash == "" {
		gitCtx.InvocationHash = inv.Hash()
	}
	if gitCtx.InvocationAttempt == 0 {
		gitCtx.InvocationAttempt = inv.InvokeSeq
	}

	if gitCtx.PreviousHash == "" {
		gitCtx.PreviousHash = gitCtx.PersistHash
	}

	if gitCtx.GitAuthor == "" && gitCtx.CellName != "" {
		gitCtx.GitAuthor = fmt.Sprintf("%s <%s@vibethis>", gitCtx.CellName, gitCtx.CellName)
	}

	return gitCtx, nil
}

func mapFromAny(input map[string]interface{}, key string) (map[string]interface{}, error) {
	val, ok := input[key]
	if !ok {
		return nil, fmt.Errorf("missing %s in input", key)
	}
	m, ok := val.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%s must be a map", key)
	}
	return m, nil
}

func mapFromAnyOptional(input map[string]interface{}, key string) (map[string]interface{}, bool) {
	val, ok := input[key]
	if !ok {
		return nil, false
	}
	m, ok := val.(map[string]interface{})
	if !ok {
		return nil, false
	}
	return m, true
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
