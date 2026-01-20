package ticketop

import (
	"context"
	"testing"

	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/ticket/internal/testutil"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	"github.com/stretchr/testify/require"
)

func TestExecuteIntegration_BatchLifecycle(t *testing.T) {
	pg := testutil.StartEmbeddedPostgres(t)
	t.Cleanup(func() { pg.Close(t) })

	inv := newOpDepsWithDB(pg.DB)
	ctx := context.Background()

	projStore, err := project.NewStore(pg.DB)
	require.NoError(t, err)
	projSvc, err := project.NewService(project.ServiceConfig{Store: projStore})
	require.NoError(t, err)
	proj, err := projSvc.CreateProject(ctx, project.CreateInput{
		Name:        "op-integration",
		GitRepoPath: "git@example.com/op-integration.git",
	})
	require.NoError(t, err)

	cellStore, err := cell.NewStore(pg.DB)
	require.NoError(t, err)
	cellSvc, err := cell.NewService(cell.ServiceConfig{Store: cellStore, Projects: projSvc})
	require.NoError(t, err)
	_, err = cellSvc.CreateCell(ctx, cell.CreateInput{
		ProjectID:   proj.ID,
		Name:        "cell-integration",
		WorkingPath: "/repo/cell-integration",
	})
	require.NoError(t, err)

	input := Input{
		Actions: []Action{
			{
				Type: ActionCreateTicket,
				Raw: mustMarshal(t, createTicketAction{
					Cell:      "cell-integration",
					ProjectID: string(proj.ID),
					Title:     "Integration Test",
					Stage:     "Open",
					State:     string(ticket.StateWorking),
				}),
			},
			{
				Type: ActionUpdateTicket,
				Raw: mustMarshal(t, updateTicketAction{
					ExpectedVersion: int64Ptr(1),
					Stage:           strPtr("Cancelled"),
					Description:     strPtr("Shift to cancelled"),
				}),
			},
			{
				Type: ActionLinkMarkdown,
				Raw: mustMarshal(t, markdownAction{
					Name:   "Design Doc",
					Path:   "docs/design.md",
					Reason: "Initial link",
				}),
			},
			{
				Type: ActionAppendWorkflow,
				Raw: mustMarshal(t, appendWorkflowAction{
					WorkflowID: "wf-123",
					RunID:      "run-abc",
					Status:     ticket.WorkflowEventRunning,
				}),
			},
			{
				Type: ActionResetTicket,
				Raw: mustMarshal(t, resetTicketAction{
					Reason: "Rewind after workflow",
				}),
			},
		},
	}

	output, err := execute(inv, context.Background(), input)
	require.NoError(t, err)
	require.Len(t, output.Results, 5)
	require.NotNil(t, output.Ticket)
	require.Equal(t, ticket.Stage("cancelled"), output.Ticket.Stage)
	require.Equal(t, "wf-123", output.ContextPatch["ticket.workflow_id"])
	require.Contains(t, output.ContextPatch, "ticket.last_event_id")
	require.Contains(t, output.ContextPatch, "ticket.last_reset_id")

	createResult := output.Results[0]
	require.NotNil(t, createResult.Ticket)
	ticketID := string(createResult.Ticket.ID)
	require.NotEmpty(t, ticketID)
	require.Equal(t, ticketID, output.ContextPatch["ticket.id"])
}
