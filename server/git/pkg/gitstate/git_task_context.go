package gitstate

import "github.com/colony-2/colony2/server/recipe-core/pkg/contextual"

type GitTaskContext struct {
	BaseRepo         string
	BaseRef          string
	ResolvedBaseHash string
	PersistHash      string
	ParentHash       string
	WorktreePath     string
	ThinPackPath     string
	TicketID         string
	CellName         string
	CellPath         string // Cell relative path from repo root
	GitAuthor        string
	NodePath         string
	InvokeSeq        int64
	InvokeHash       string
}

func NewGitTaskContext(tec contextual.TaskExecutionContext) *GitTaskContext {
	return &GitTaskContext{
		BaseRepo:         tec.GitBase.BaseRepo,
		BaseRef:          tec.GitBase.BaseRef,
		ResolvedBaseHash: tec.GitBase.ResolvedBaseHash,
		GitAuthor:        tec.GitBase.GitAuthor,
		PersistHash:      tec.GitCommit.PersistHash,
		ParentHash:       tec.GitCommit.ParentHash,
		WorktreePath:     tec.Environment.WorktreePath,
		ThinPackPath:     tec.Environment.ThinPackPath,
		TicketID:         tec.Actor.TicketID,
		CellName:         tec.Workflow.CellName,
		CellPath:         tec.Workflow.CellPath,
		NodePath:         tec.Invocation.NodePath,
		InvokeSeq:        tec.Invocation.InvokeSeq,
		InvokeHash:       tec.Invocation.Hash(),
	}
}

func (c *GitTaskContext) GetBaseRepo() string         { return c.BaseRepo }
func (c *GitTaskContext) GetBaseRef() string          { return c.BaseRef }
func (c *GitTaskContext) GetResolvedBaseHash() string { return c.ResolvedBaseHash }
func (c *GitTaskContext) GetPersistHash() string      { return c.PersistHash }
func (c *GitTaskContext) GetParentHash() string       { return c.ParentHash }
func (c *GitTaskContext) GetWorktreePath() string     { return c.WorktreePath }
func (c *GitTaskContext) GetThinPackPath() string     { return c.ThinPackPath }
func (c *GitTaskContext) GetTicketID() string         { return c.TicketID }
func (c *GitTaskContext) GetCellName() string         { return c.CellName }
func (c *GitTaskContext) GetCellPath() string         { return c.CellPath }
func (c *GitTaskContext) GetGitAuthor() string        { return c.GitAuthor }
func (c *GitTaskContext) GetNodePath() string         { return c.NodePath }
func (c *GitTaskContext) GetInvokeSeq() int64         { return c.InvokeSeq }
func (c *GitTaskContext) GetInvokeHash() string       { return c.InvokeHash }
