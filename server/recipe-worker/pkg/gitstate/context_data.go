package gitstate

import (
	"fmt"

	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/contextual"
)

// Context captures git execution metadata that persists across activity invocations.
// It composes shared contextual structs to avoid duplicated field definitions.
type Context struct {
	contextual.InvocationContext
	contextual.ActorContext
	contextual.EnvironmentContext
	contextual.GitSnapshotContext
	Workflow contextual.WorkflowEnvelope
}

func (c *Context) GetBaseRepo() string         { return c.GitSnapshotContext.BaseRepo }
func (c *Context) GetBaseHash() string         { return c.GitSnapshotContext.BaseHash }
func (c *Context) GetPersistHash() string      { return c.GitSnapshotContext.PersistHash }
func (c *Context) GetPreviousHash() string     { return c.GitSnapshotContext.PreviousHash }
func (c *Context) GetWorktreePath() string     { return c.EnvironmentContext.WorktreePath }
func (c *Context) GetBlobStoreURI() string     { return c.EnvironmentContext.BlobStoreURI }
func (c *Context) GetThinPackPath() string     { return c.EnvironmentContext.ThinPackPath }
func (c *Context) GetTicketID() string         { return c.ActorContext.TicketID }
func (c *Context) GetCellName() string         { return c.ActorContext.CellName }
func (c *Context) GetRecipeID() string         { return c.InvocationContext.RecipeID }
func (c *Context) GetRecipeNode() string       { return c.InvocationContext.NodePath }
func (c *Context) GetInvocationID() string     { return c.InvocationContext.InvocationID }
func (c *Context) GetInvocationHash() string   { return c.InvocationContext.InvocationHash }
func (c *Context) GetInvocationAttempt() int   { return c.InvocationContext.InvocationAttempt }
func (c *Context) GetBoxID() string            { return c.InvocationContext.BoxID }
func (c *Context) GetActivityID() string       { return c.InvocationContext.ActivityID }
func (c *Context) GetJobID() swf.JobId         { return c.InvocationContext.JobID }
func (c *Context) GetGitAuthor() string        { return c.GitSnapshotContext.GitAuthor }
func (c *Context) IsWorkspacePrepared() bool   { return c.GitSnapshotContext.WorkspacePrepared }
func (c *Context) SetWorkspacePrepared(v bool) { c.GitSnapshotContext.WorkspacePrepared = v }

// ToMap converts the Context into a map suitable for embedding inside outputs["context"]["git"].
// TODO: remove once callers are fully typed.
func (c Context) ToMap() map[string]interface{} {
	result := map[string]interface{}{
		"base_repo":          c.BaseRepo,
		"base_hash":          c.BaseHash,
		"persist_hash":       c.PersistHash,
		"previous_hash":      c.PreviousHash,
		"blob_store_uri":     c.BlobStoreURI,
		"thin_pack_path":     c.ThinPackPath,
		"ticket_id":          c.TicketID,
		"cell_name":          c.CellName,
		"recipe_id":          c.RecipeID,
		"recipe_node":        c.NodePath,
		"invocation_id":      c.InvocationID,
		"invocation_hash":    c.InvocationHash,
		"invocation_attempt": c.InvocationAttempt,
		"box_id":             c.BoxID,
		"activity_id":        c.ActivityID,
		"job_id":             c.JobID,
		"workspace_prepared": c.WorkspacePrepared,
	}
	if c.WorktreePath != "" {
		result["worktree_path"] = c.WorktreePath
	}
	if c.GitAuthor != "" {
		result["git_author"] = c.GitAuthor
	}
	return result
}

// UpdateFromMap merges values from map input into the context. Missing required fields trigger an error.
// TODO: delete when all callers supply typed payloads.
func (c *Context) UpdateFromMap(input map[string]interface{}) error {
	if input == nil {
		return fmt.Errorf("git context map is missing")
	}

	if str, ok := stringFromMap(input, "base_repo"); ok {
		c.BaseRepo = str
	} else if c.BaseRepo == "" {
		return fmt.Errorf("git context is missing base_repo")
	}

	if str, ok := stringFromMap(input, "base_hash"); ok {
		c.BaseHash = str
	} else if c.BaseHash == "" {
		return fmt.Errorf("git context is missing base_hash")
	}

	if str, ok := stringFromMap(input, "persist_hash"); ok {
		c.PersistHash = str
	} else if c.PersistHash == "" {
		c.PersistHash = c.BaseHash
	}

	if str, ok := stringFromMap(input, "previous_hash"); ok {
		c.PreviousHash = str
	}

	if str, ok := stringFromMap(input, "blob_store_uri"); ok {
		c.BlobStoreURI = str
	}

	if str, ok := stringFromMap(input, "thin_pack_path"); ok {
		c.ThinPackPath = str
	}

	if str, ok := stringFromMap(input, "ticket_id"); ok {
		c.TicketID = str
	}

	if str, ok := stringFromMap(input, "cell_name"); ok {
		c.CellName = str
	}

	if str, ok := stringFromMap(input, "recipe_id"); ok {
		c.RecipeID = str
	}

	if str, ok := stringFromMap(input, "recipe_node"); ok {
		c.NodePath = str
	}

	if str, ok := stringFromMap(input, "invocation_id"); ok {
		c.InvocationID = str
	}

	if str, ok := stringFromMap(input, "invocation_hash"); ok {
		c.InvocationHash = str
	}

	if str, ok := stringFromMap(input, "box_id"); ok {
		c.BoxID = str
	}

	if str, ok := stringFromMap(input, "activity_id"); ok {
		c.ActivityID = str
	}

	if val, ok := stringFromMap(input, "job_id"); ok {
		c.JobID = swf.JobId(val)
	}

	if val, ok := boolFromMap(input, "workspace_prepared"); ok {
		c.WorkspacePrepared = val
	}

	if str, ok := stringFromMap(input, "git_author"); ok {
		c.GitAuthor = str
	}

	if str, ok := stringFromMap(input, "worktree_path"); ok {
		c.WorktreePath = str
	}

	if val, ok := intFromMap(input, "invocation_attempt"); ok {
		c.InvocationAttempt = val
	}

	return nil
}
