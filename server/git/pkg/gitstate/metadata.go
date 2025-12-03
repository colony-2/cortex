package gitstate

import (
	"fmt"
)

// buildCommitMessage renders the structured commit metadata block used for git persistence.
func buildCommitMessage(ctx *GitTaskContext, persistHash, thinPackPath string) string {
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
ticket:
  id: %s
  cell: %s
invocation:
  path: %s
  seq: %d

---
`,
		ctx.BaseHash,
		ctx.PreviousHash,
		persistHash,
		ctx.BlobStoreURI,
		thinPackPath,
		ctx.TicketID,
		ctx.CellName,
		ctx.NodePath,
		ctx.InvokeSeq,
	)
}
