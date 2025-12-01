package gitstate

import (
	"encoding/json"
	"fmt"

	"github.com/colony-2/swf-go/pkg/swf"
	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
)

// LegacyPayloadFromInput converts legacy map inputs into a typed workspace payload.
// This should be used only at the registry boundary; the rest of gitstate remains typed.
func LegacyPayloadFromInput(inv coreops.Invocation, input map[string]interface{}) (WorkspacePayload, error) {
	var payload WorkspacePayload

	ctxMap, err := mapFromAny(input, "context")
	if err != nil {
		return payload, err
	}

	gitMap, err := mapFromAny(ctxMap, "git")
	if err != nil {
		return payload, err
	}

	if err := payload.Context.UpdateFromMap(gitMap); err != nil {
		return payload, err
	}

	if payload.Context.WorktreePath == "" {
		if val, ok := stringFromMap(ctxMap, "worktree"); ok {
			payload.Context.WorktreePath = val
		}
	}

	if payload.Context.BlobStoreURI == "" {
		if val, ok := stringFromMap(ctxMap, "blobstore"); ok {
			payload.Context.BlobStoreURI = val
		}
	}

	if payload.TicketID == "" {
		if val, ok := stringFromMap(ctxMap, "ticketid"); ok {
			payload.TicketID = val
		} else if val, ok := stringFromMap(input, "ticket_id"); ok {
			payload.TicketID = val
		}
	}

	if payload.CellName == "" {
		if val, ok := stringFromMap(ctxMap, "cellname"); ok {
			payload.CellName = val
		} else if val, ok := stringFromMap(input, "cell_name"); ok {
			payload.CellName = val
		}
	}

	if recipeMeta, ok := mapFromAnyOptional(ctxMap, "recipe"); ok {
		if payload.Context.RecipeID == "" {
			if val, ok := stringFromMap(recipeMeta, "id"); ok {
				payload.Context.RecipeID = val
			}
		}
		if payload.Context.NodePath == "" {
			if val, ok := stringFromMap(recipeMeta, "node_path"); ok {
				payload.Context.NodePath = val
			}
		}
		if payload.Context.InvocationHash == "" {
			if val, ok := stringFromMap(recipeMeta, "invocation_hash"); ok {
				payload.Context.InvocationHash = val
			}
		}
		if payload.Context.InvocationID == "" {
			if val, ok := stringFromMap(recipeMeta, "invocation_id"); ok {
				payload.Context.InvocationID = val
			}
		}
		if payload.Context.InvocationAttempt == 0 {
			if val, ok := intFromMap(recipeMeta, "invocation_attempt"); ok {
				payload.Context.InvocationAttempt = val
			}
		}
		if payload.Context.JobID == "" {
			if val, ok := stringFromMap(recipeMeta, "job_id"); ok {
				payload.Context.JobID = swf.JobId(val)
			}
		}
	}

	if val, ok := stringFromMap(input, "git_author"); ok {
		payload.Context.GitAuthor = val
	}

	raw, err := json.Marshal(input)
	if err != nil {
		return payload, fmt.Errorf("marshal legacy input: %w", err)
	}
	payload.RawInput = raw

	ctx, err := ContextFromPayload(inv, payload)
	if err != nil {
		return payload, err
	}
	payload.Context = *ctx
	// For legacy callers, disable cell scoping to avoid path mismatches until inputs are fully typed.
	payload.CellName = ""
	payload.Context.CellName = ""
	if payload.GitPersistHash == "" && payload.Context.PersistHash != "" {
		payload.GitPersistHash = payload.Context.PersistHash
	}
	return payload, nil
}
