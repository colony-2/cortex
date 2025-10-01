package gitstate

import "fmt"

// Context captures git execution metadata that persists across activity invocations.
type Context struct {
	BaseRepo          string
	BaseHash          string
	PersistHash       string
	PreviousHash      string
	WorktreePath      string
	BlobStoreURI      string
	ThinPackPath      string
	TicketID          string
	CellName          string
	RecipeID          string
	RecipeNode        string
	InvocationID      string
	InvocationHash    string
	InvocationAttempt int
	BoxID             string
	ActivityID        string
	WorkflowID        string
	WorkflowRunID     string
	WorkspacePrepared bool
	GitAuthor         string
}

func (c *Context) GetBaseRepo() string         { return c.BaseRepo }
func (c *Context) GetBaseHash() string         { return c.BaseHash }
func (c *Context) GetPersistHash() string      { return c.PersistHash }
func (c *Context) GetPreviousHash() string     { return c.PreviousHash }
func (c *Context) GetWorktreePath() string     { return c.WorktreePath }
func (c *Context) GetBlobStoreURI() string     { return c.BlobStoreURI }
func (c *Context) GetThinPackPath() string     { return c.ThinPackPath }
func (c *Context) GetTicketID() string         { return c.TicketID }
func (c *Context) GetCellName() string         { return c.CellName }
func (c *Context) GetRecipeID() string         { return c.RecipeID }
func (c *Context) GetRecipeNode() string       { return c.RecipeNode }
func (c *Context) GetInvocationID() string     { return c.InvocationID }
func (c *Context) GetInvocationHash() string   { return c.InvocationHash }
func (c *Context) GetInvocationAttempt() int   { return c.InvocationAttempt }
func (c *Context) GetBoxID() string            { return c.BoxID }
func (c *Context) GetActivityID() string       { return c.ActivityID }
func (c *Context) GetWorkflowID() string       { return c.WorkflowID }
func (c *Context) GetWorkflowRunID() string    { return c.WorkflowRunID }
func (c *Context) GetGitAuthor() string        { return c.GitAuthor }
func (c *Context) IsWorkspacePrepared() bool   { return c.WorkspacePrepared }
func (c *Context) SetWorkspacePrepared(v bool) { c.WorkspacePrepared = v }

// ToMap converts the Context into a map suitable for embedding inside outputs["context"]["git"].
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
		"recipe_node":        c.RecipeNode,
		"invocation_id":      c.InvocationID,
		"invocation_hash":    c.InvocationHash,
		"invocation_attempt": c.InvocationAttempt,
		"box_id":             c.BoxID,
		"activity_id":        c.ActivityID,
		"workflow_id":        c.WorkflowID,
		"workflow_run_id":    c.WorkflowRunID,
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
		c.RecipeNode = str
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

	if str, ok := stringFromMap(input, "workflow_id"); ok {
		c.WorkflowID = str
	}

	if str, ok := stringFromMap(input, "workflow_run_id"); ok {
		c.WorkflowRunID = str
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

func stringFromMap(m map[string]interface{}, key string) (string, bool) {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok && str != "" {
			return str, true
		}
	}
	return "", false
}

func boolFromMap(m map[string]interface{}, key string) (bool, bool) {
	if val, ok := m[key]; ok {
		if b, ok := val.(bool); ok {
			return b, true
		}
	}
	return false, false
}

func intFromMap(m map[string]interface{}, key string) (int, bool) {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case int:
			return v, true
		case int32:
			return int(v), true
		case int64:
			return int(v), true
		case float64:
			return int(v), true
		}
	}
	return 0, false
}
