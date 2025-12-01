package gitstate

import (
	"fmt"

	"github.com/colony-2/swf-go/pkg/swf"
	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
)

// ContextFromRequest reconstructs a git execution context from the invocation and input payload.
// TODO: accept fully typed payloads once upstream callers are migrated off maps.
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
		if gitCtx.NodePath == "" {
			if val, ok := stringFromMap(recipeMeta, "node_path"); ok {
				gitCtx.NodePath = val
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
		if gitCtx.JobID == "" {
			if val, ok := stringFromMap(recipeMeta, "job_id"); ok {
				gitCtx.JobID = swf.JobId(val)
			}
		}
	}

	if val, ok := stringFromMap(input, "git_author"); ok {
		gitCtx.GitAuthor = val
	}

	gitCtx.BoxID = firstNonEmpty(gitCtx.BoxID, inv.BoxID)
	gitCtx.ActivityID = firstNonEmpty(gitCtx.ActivityID, inv.ActivityID)
	gitCtx.RecipeID = firstNonEmpty(gitCtx.RecipeID, inv.RecipeID)
	gitCtx.NodePath = firstNonEmpty(gitCtx.NodePath, inv.NodePath)
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
