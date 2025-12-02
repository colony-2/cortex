package gitstate

import (
	"fmt"
)

// ContextFromPayload reconstructs a git execution context from the invocation and typed payload.
func ContextFromPayload(payload WorkspacePayload) (*Context, error) {
	gitCtx := payload.Context

	// Fill in base defaults from invocation when missing.
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
