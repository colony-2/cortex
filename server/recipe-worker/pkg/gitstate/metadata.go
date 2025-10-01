package gitstate

import (
	"fmt"
)

// buildCommitMessage renders the structured commit metadata block used for git persistence.
func buildCommitMessage(ctx Context, persistHash, thinPackPath string) string {
	if persistHash == "" {
		persistHash = ctx.PersistHash
	}
	if thinPackPath == "" {
		thinPackPath = ctx.ThinPackPath
	}
	return fmt.Sprintf(`Recipe %s node %s

---
git:
  base_hash: %s
  previous_hash: %s
  persist_hash: %s
  blob_store_uri: %s
  thin_pack_path: %s
invocation:
  hash: %s
  id: %s
  attempt: %d
  box_id: %s
  activity_id: %s
workflow:
  id: %s
  run_id: %s
ticket:
  id: %s
  cell: %s
recipe:
  id: %s
  node_id: %s
---
`,
		ctx.RecipeID,
		ctx.RecipeNode,
		ctx.BaseHash,
		ctx.PreviousHash,
		persistHash,
		ctx.BlobStoreURI,
		thinPackPath,
		ctx.InvocationHash,
		ctx.InvocationID,
		ctx.InvocationAttempt,
		ctx.BoxID,
		ctx.ActivityID,
		ctx.WorkflowID,
		ctx.WorkflowRunID,
		ctx.TicketID,
		ctx.CellName,
		ctx.RecipeID,
		ctx.RecipeNode,
	)
}
