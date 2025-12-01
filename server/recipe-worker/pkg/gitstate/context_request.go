package gitstate

import (
	"fmt"

	coreops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
)

// ContextFromPayload reconstructs a git execution context from the invocation and typed payload.
func ContextFromPayload(inv coreops.Invocation, payload WorkspacePayload) (*Context, error) {
	gitCtx := payload.Context

	// Fill in base defaults from invocation when missing.
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
	if gitCtx.TicketID == "" {
		gitCtx.TicketID = payload.TicketID
	}
	if gitCtx.CellName == "" {
		gitCtx.CellName = payload.CellName
	}
	if gitCtx.PersistHash == "" && payload.GitPersistHash != "" {
		gitCtx.PersistHash = payload.GitPersistHash
	}
	if gitCtx.PreviousHash == "" {
		gitCtx.PreviousHash = gitCtx.PersistHash
	}
	if gitCtx.GitAuthor == "" && gitCtx.CellName != "" {
		gitCtx.GitAuthor = fmt.Sprintf("%s <%s@vibethis>", gitCtx.CellName, gitCtx.CellName)
	}
	return &gitCtx, nil
}

// ContextFromRequest is a compatibility helper for legacy map inputs (registry boundary).
func ContextFromRequest(inv coreops.Invocation, input map[string]interface{}) (*Context, error) {
	payload, err := LegacyPayloadFromInput(inv, input)
	if err != nil {
		return nil, err
	}
	return ContextFromPayload(inv, payload)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
