package ticketop

import (
	"context"
	"testing"

	"github.com/colony-2/colony2/server/cell/pkg/cell"
	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/ticket/internal/model"
	"github.com/colony-2/colony2/server/pgembed/pkg/pgembed"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	"github.com/stretchr/testify/require"
)

func TestExecuteIntegration_BatchLifecycle(t *testing.T) {
	pg := pgembed.StartEmbeddedPostgres(t)
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

	ticketSvc, err := ticket.NewServiceFromDB(pg.DB)
	require.NoError(t, err)
	created, _, err := ticketSvc.CreateTicket(ctx, ticket.CreateInput{
		Cell:        "cell-integration",
		ProjectID:   ticket.ProjectID(proj.ID),
		Title:       "Integration Test",
		Stage:       ticket.Stage("open"),
		State:       ticket.StateWorking,
		Description: "Seed ticket for manage op",
		Actor:       ticket.NewAgentActor("cell-integration", "integration", "exec-1", "inv-1"),
	})
	require.NoError(t, err)

	input := Input{
		Actions: ActionList{
			&model.UpdateTicketAction{
				ExistingTicketOp: model.ExistingTicketOp{
					TicketID: created.ID,
				},
				ExpectedVersion: int64Ptr(created.Version.Int64),
				Stage:           strPtr("Cancelled"),
				Description:     strPtr("Shift to cancelled"),
			},
			&model.MarkdownLinkAction{
				BaseMarkdownAction: model.BaseMarkdownAction{
					ExistingTicketOp: model.ExistingTicketOp{
						TicketID: created.ID,
					},
					Name:   "Design Doc",
					Path:   "docs/design.md",
					Reason: "Initial link",
				},
			},
			&model.AppendWorkflowAction{
				ExistingTicketOp: model.ExistingTicketOp{
					TicketID: created.ID,
				},
				WorkflowID: "wf-123",
				RunID:      "run-abc",
				Status:     ticket.WorkflowEventRunning,
			},
			&model.ResetTicketAction{
				ExistingTicketOp: model.ExistingTicketOp{
					TicketID: created.ID,
				},
				Reason: "Rewind after workflow",
			},
		},
	}

	output, err := execute(inv, context.Background(), input)
	require.NoError(t, err)
	require.Len(t, output.Results, 4)
	updateResult, ok := output.Results[0].(*model.UpdateResult)
	require.True(t, ok)
	require.NotNil(t, updateResult.Ticket)
	require.Equal(t, ticket.Stage("cancelled"), updateResult.Ticket.Stage)
	_, ok = output.Results[1].(*model.MarkdownResult)
	require.True(t, ok)
	_, ok = output.Results[2].(*model.WorkflowResult)
	require.True(t, ok)
	resetResult, ok := output.Results[3].(*model.ResetResult)
	require.True(t, ok)
	require.NotNil(t, resetResult.Ticket)
}
