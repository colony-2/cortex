package ticketop

import (
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
)

type ActionResult interface {
	isResultType()
}

type isResult struct {
}

func (isResult) isResultType() {}

type CreateResult struct {
	isResult
	ticket.Ticket `json:"ticket"`
}

type UpdateResult struct {
	isResult
	Ticket *ticket.Ticket `json:"ticket"`
}

type AppendTicketNoteResult struct {
	isResult
	Event *ticket.TicketEvent `json:"event"`
}

type MarkdownResult struct {
	isResult
	Event *ticket.TicketEvent `json:"event"`
}

type WorkflowResult struct {
	isResult
	Event *ticket.TicketEvent `json:"event"`
}

type ResetResult struct {
	isResult
	Reset  *ticket.TicketReset `json:"reset"`
	Ticket *ticket.Ticket      `json:"ticket"`
}
