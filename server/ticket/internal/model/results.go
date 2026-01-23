package model

type ActionResult interface {
	isResultType()
}

type isResult struct {
}

func (isResult) isResultType() {}

type CreateResult struct {
	isResult
	Ticket `json:"ticket"`
	JobID  string `json:"job_id,omitempty"`
}

type UpdateResult struct {
	isResult
	Ticket *Ticket `json:"ticket"`
}

type AppendTicketNoteResult struct {
	isResult
	Event *TicketEvent `json:"event"`
}

type MarkdownResult struct {
	isResult
	Event *TicketEvent `json:"event"`
}

type WorkflowResult struct {
	isResult
	Event *TicketEvent `json:"event"`
}

type ResetResult struct {
	isResult
	Reset  *TicketReset `json:"reset"`
	Ticket *Ticket      `json:"ticket"`
}
