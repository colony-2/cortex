package gitstate

import "github.com/colony-2/colony2/server/recipe-core/pkg/contextual"

type GitTaskContext struct {
	BaseRepo     string
	BaseHash     string
	PersistHash  string
	PreviousHash string
	WorktreePath string
	BlobStoreURI string
	ThinPackPath string
	TicketID     string
	CellName     string
	GitAuthor    string
	NodePath     string
	InvokeSeq    int64
	InvokeHash   string
}

func NewGitTaskContext(tec contextual.TaskExecutionContext) *GitTaskContext {
	return &GitTaskContext{
		BaseRepo:     tec.GitBase.BaseRepo,
		BaseHash:     tec.GitBase.BaseHash,
		GitAuthor:    tec.GitBase.GitAuthor,
		PersistHash:  tec.GitCommit.PersistHash,
		PreviousHash: tec.GitCommit.ParentHash,
		WorktreePath: tec.Environment.WorktreePath,
		BlobStoreURI: tec.Environment.BlobStoreURI,
		ThinPackPath: tec.Environment.ThinPackPath,
		TicketID:     tec.Actor.TicketID,
		CellName:     tec.Workflow.CellName,
		NodePath:     tec.Invocation.NodePath,
		InvokeSeq:    tec.Invocation.InvokeSeq,
		InvokeHash:   tec.Invocation.Hash(),
	}
}

func (c *GitTaskContext) GetBaseRepo() string     { return c.BaseRepo }
func (c *GitTaskContext) GetBaseHash() string     { return c.BaseHash }
func (c *GitTaskContext) GetPersistHash() string  { return c.PersistHash }
func (c *GitTaskContext) GetPreviousHash() string { return c.PreviousHash }
func (c *GitTaskContext) GetWorktreePath() string { return c.WorktreePath }
func (c *GitTaskContext) GetBlobStoreURI() string { return c.BlobStoreURI }
func (c *GitTaskContext) GetThinPackPath() string { return c.ThinPackPath }
func (c *GitTaskContext) GetTicketID() string     { return c.TicketID }
func (c *GitTaskContext) GetCellName() string     { return c.CellName }
func (c *GitTaskContext) GetGitAuthor() string    { return c.GitAuthor }
func (c *GitTaskContext) GetNodePath() string     { return c.NodePath }
func (c *GitTaskContext) GetInvokeSeq() int64     { return c.InvokeSeq }
func (c *GitTaskContext) GetInvokeHash() string   { return c.InvokeHash }
