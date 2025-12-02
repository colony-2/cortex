package gitstate

import (
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/contextual"
)

// Context captures git execution metadata that persists across activity invocations.
// It composes shared contextual structs to avoid duplicated field definitions.
type Context struct {
	contextual.ActorContext
	contextual.EnvironmentContext
	contextual.GitSnapshotContext
	contextual.WorkflowEnvelope
}

func (c *Context) GetBaseRepo() string     { return c.GitSnapshotContext.BaseRepo }
func (c *Context) GetBaseHash() string     { return c.GitSnapshotContext.BaseHash }
func (c *Context) GetPersistHash() string  { return c.GitSnapshotContext.PersistHash }
func (c *Context) GetPreviousHash() string { return c.GitSnapshotContext.PreviousHash }
func (c *Context) GetWorktreePath() string { return c.EnvironmentContext.WorktreePath }
func (c *Context) GetBlobStoreURI() string { return c.EnvironmentContext.BlobStoreURI }
func (c *Context) GetThinPackPath() string { return c.EnvironmentContext.ThinPackPath }
func (c *Context) GetTicketID() string     { return c.ActorContext.TicketID }
func (c *Context) GetCellName() string     { return c.WorkflowEnvelope.CellName }
func (c *Context) GetGitAuthor() string    { return c.GitSnapshotContext.GitAuthor }
