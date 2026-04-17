package ticketop

import (
	"context"
	"errors"
	"testing"
	"time"

	recipeartifacts "github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/ops"
	"github.com/colony-2/c2j/pkg/workflow"
	"github.com/colony-2/c2j/pkg/workflowctl"
	"github.com/colony-2/colony2/server/ticket/internal/model"
	"github.com/colony-2/colony2/server/ticket/pkg/ticket"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/plugin/optimisticlock"
)

type stubService struct {
	createFunc         func(context.Context, ticket.CreateInput) (*ticket.Ticket, string, error)
	updateFunc         func(context.Context, ticket.ID, ticket.UpdateInput) (*ticket.Ticket, error)
	appendTicketFunc   func(context.Context, ticket.ID, ticket.TicketEventInput) (*ticket.TicketEvent, error)
	appendMarkdownFunc func(context.Context, ticket.ID, ticket.MarkdownEventInput) (*ticket.TicketEvent, error)
	appendWorkflowFunc func(context.Context, ticket.ID, ticket.WorkflowEventInput) (*ticket.TicketEvent, error)
	resetFunc          func(context.Context, ticket.ID, ticket.TicketResetInput) (*ticket.TicketReset, error)
	getTicketAtFunc    func(context.Context, ticket.ID, time.Time) (*ticket.Ticket, error)
}

func (s *stubService) CreateTicket(ctx context.Context, in ticket.CreateInput) (*ticket.Ticket, string, error) {
	if s.createFunc != nil {
		return s.createFunc(ctx, in)
	}
	return nil, "", errors.New("unexpected CreateTicket call")
}

func (s *stubService) UpdateTicket(ctx context.Context, id ticket.ID, in ticket.UpdateInput) (*ticket.Ticket, error) {
	if s.updateFunc != nil {
		return s.updateFunc(ctx, id, in)
	}
	return nil, errors.New("unexpected UpdateTicket call")
}

func (s *stubService) ApplyActions(ctx context.Context, actions []model.Action, fallback ticket.Actor) ([]model.ActionResult, error) {
	results := make([]model.ActionResult, 0, len(actions))
	for _, action := range actions {
		actor, err := resolveActor(action.ActorPayload(), fallback)
		if err != nil {
			return results, err
		}
		result, err := executeAction(ctx, s, action, actor)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *stubService) SearchTickets(context.Context, ticket.SearchFilter) (ticket.Iterator[*ticket.Ticket], error) {
	return nil, errors.New("not implemented")
}

func (s *stubService) SearchStages(context.Context, ticket.SearchFilter) (ticket.Iterator[ticket.Stage], error) {
	return nil, errors.New("not implemented")
}

func (s *stubService) GetStates(context.Context) ([]ticket.State, error) {
	return nil, errors.New("not implemented")
}

func (s *stubService) GetTicketAt(ctx context.Context, id ticket.ID, at time.Time) (*ticket.Ticket, error) {
	if s.getTicketAtFunc != nil {
		return s.getTicketAtFunc(ctx, id, at)
	}
	return nil, errors.New("unexpected GetTicketAt call")
}

func (s *stubService) AppendWorkflowEvent(ctx context.Context, id ticket.ID, in ticket.WorkflowEventInput) (*ticket.TicketEvent, error) {
	if s.appendWorkflowFunc != nil {
		return s.appendWorkflowFunc(ctx, id, in)
	}
	return nil, errors.New("unexpected AppendWorkflowEvent call")
}

func (s *stubService) AppendMarkdownEvent(ctx context.Context, id ticket.ID, in ticket.MarkdownEventInput) (*ticket.TicketEvent, error) {
	if s.appendMarkdownFunc != nil {
		return s.appendMarkdownFunc(ctx, id, in)
	}
	return nil, errors.New("unexpected AppendMarkdownEvent call")
}

func (s *stubService) AppendChangeSetEvent(context.Context, ticket.ID, ticket.ChangeSetEventInput) (*ticket.TicketEvent, error) {
	return nil, errors.New("unexpected AppendChangeSetEvent call")
}

func (s *stubService) AppendTicketEvent(ctx context.Context, id ticket.ID, in ticket.TicketEventInput) (*ticket.TicketEvent, error) {
	if s.appendTicketFunc != nil {
		return s.appendTicketFunc(ctx, id, in)
	}
	return nil, errors.New("unexpected AppendTicketEvent call")
}

func (s *stubService) ListEvents(context.Context, ticket.ID, ticket.TicketEventFilter) (ticket.Iterator[*ticket.TicketEvent], error) {
	return nil, errors.New("not implemented")
}

func (s *stubService) ResetTicket(ctx context.Context, id ticket.ID, in ticket.TicketResetInput) (*ticket.TicketReset, error) {
	if s.resetFunc != nil {
		return s.resetFunc(ctx, id, in)
	}
	return nil, errors.New("unexpected ResetTicket call")
}

var _ ops.OpDependencies = (*stubDeps)(nil)

type stubDeps struct {
	db                *gorm.DB
	inputArtifacts    []swf.Artifact
	outputs           []swf.Artifact
	externalArtifacts map[string]recipeartifacts.Ref
	workflow          workflowctl.WorkflowControl
	worktreePath      string
	gitContext        ops.GitExecutionContext
	jobTool           ops.JobTool
}

func (d *stubDeps) SetNextTaskType(taskType string) {
}

func (d *stubDeps) Database() *gorm.DB { return d.db }
func (d *stubDeps) AddOutputArtifact(a swf.Artifact) error {
	if a == nil {
		return errors.New("nil artifact")
	}
	d.outputs = append(d.outputs, a)
	return nil
}
func (d *stubDeps) AddExternalArtifact(name string, url string, expand bool) error {
	artifactRef := recipeartifacts.NewExternalRef(name, url, expand)
	if err := artifactRef.Validate(); err != nil {
		return err
	}
	if d.externalArtifacts == nil {
		d.externalArtifacts = make(map[string]recipeartifacts.Ref)
	}
	d.externalArtifacts[name] = artifactRef
	return nil
}
func (d *stubDeps) GetInputArtifacts() []swf.Artifact  { return d.inputArtifacts }
func (d *stubDeps) GetOutputArtifacts() []swf.Artifact { return d.outputs }
func (d *stubDeps) GetExternalArtifacts() map[string]recipeartifacts.Ref {
	if len(d.externalArtifacts) == 0 {
		return nil
	}
	out := make(map[string]recipeartifacts.Ref, len(d.externalArtifacts))
	for name, artifactRef := range d.externalArtifacts {
		out[name] = artifactRef
	}
	return out
}
func (d *stubDeps) WorkflowControl() workflowctl.WorkflowControl {
	return d.workflow
}
func (d *stubDeps) WorktreePath() string { return d.worktreePath }
func (d *stubDeps) GitContext() ops.GitExecutionContext {
	return d.gitContext
}
func (d *stubDeps) JobTool() ops.JobTool { return d.jobTool }
func (d *stubDeps) FindArtifact(key swf.ArtifactKey) (swf.Artifact, error) {
	var found swf.Artifact
	for _, artifact := range d.inputArtifacts {
		if artifact.Name() == key.Name {
			if found != nil {
				return nil, errors.New("duplicate artifact found")
			}
			found = artifact
		}
	}
	if found == nil {
		return nil, errors.New("artifact not found")
	}
	return found, nil
}

func newOpDeps() ops.OpDependencies { return newOpDepsWithDB(&gorm.DB{}) }
func newOpDepsWithDB(db *gorm.DB) ops.OpDependencies {
	return &stubDeps{db: db}
}

func TestExecute_CreateAndUpdate(t *testing.T) {
	svc := &stubService{}
	restore := TestingStub{Service: svc}.Install()
	defer restore()

	created := &ticket.Ticket{
		ID:        ticket.ID("TCK-001"),
		ProjectID: ticket.ProjectID("proj-1"),
		Stage:     ticket.Stage("open"),
		State:     ticket.StateWorking,
		UpdatedAt: time.Date(2024, 9, 20, 10, 0, 0, 0, time.UTC),
		Version:   optimisticlock.Version{Int64: 1, Valid: true},
	}
	svc.createFunc = func(ctx context.Context, input ticket.CreateInput) (*ticket.Ticket, string, error) {
		require.Equal(t, ticket.Stage("open"), input.Stage)
		require.Equal(t, ticket.StateWorking, input.State)
		require.Equal(t, ticket.ActorTypeAgent, input.Actor.Type)
		require.Equal(t, "cell-x", input.Actor.Agent.CellName)
		require.Equal(t, "recipe-alpha", input.Actor.Agent.WorkflowName)
		require.Equal(t, ticket.ProjectID("proj-1"), input.ProjectID)
		return created, "job-1", nil
	}

	updated := &ticket.Ticket{
		ID:        ticket.ID("TCK-001"),
		ProjectID: ticket.ProjectID("proj-1"),
		Stage:     ticket.Stage("cancelled"),
		State:     ticket.StateWorking,
		UpdatedAt: time.Date(2024, 9, 20, 11, 0, 0, 0, time.UTC),
		Version:   optimisticlock.Version{Int64: 2, Valid: true},
	}
	svc.updateFunc = func(ctx context.Context, id ticket.ID, in ticket.UpdateInput) (*ticket.Ticket, error) {
		require.Equal(t, ticket.ID("TCK-001"), id)
		require.NotNil(t, in.Stage)
		require.Equal(t, ticket.Stage("cancelled"), *in.Stage)
		require.Nil(t, in.Actor)
		return updated, nil
	}

	deps := newOpDeps()
	input := Input{
		Actions: ActionList{
			&model.CreateTicketAction{
				BaseAction: model.BaseAction{
					Actor: &model.ActorPayload{
						Type: "agent",
						Agent: &model.ActorAgentPayload{
							CellName:       "cell-x",
							WorkflowName:   "recipe-alpha",
							ExecutionID:    "exec-1",
							InvocationHash: "inv-1",
						},
					},
				},
				Cell:      "cell-x",
				ProjectID: "proj-1",
				Title:     "Bootstrap",
				Stage:     "Open",
				State:     string(ticket.StateWorking),
			},
			&model.UpdateTicketAction{
				ExistingTicketOp: model.ExistingTicketOp{
					TicketID: ticket.ID("TCK-001"),
				},
				ExpectedVersion: int64Ptr(1),
				Stage:           strPtr("Cancelled"),
			},
		},
	}

	output, err := execute(deps, context.Background(), input)
	require.NoError(t, err)
	require.Len(t, output.Results, 2)
	createResult, ok := output.Results[0].(*model.CreateResult)
	require.True(t, ok)
	require.Equal(t, ticket.ID("TCK-001"), createResult.ID)
	updateResult, ok := output.Results[1].(*model.UpdateResult)
	require.True(t, ok)
	require.NotNil(t, updateResult.Ticket)
	require.Equal(t, ticket.Stage("cancelled"), updateResult.Ticket.Stage)
}

func TestExecute_AppendTicketNote(t *testing.T) {
	svc := &stubService{}
	restore := TestingStub{Service: svc}.Install()
	defer restore()

	event := &ticket.TicketEvent{
		ID:        ticket.TicketEventID("EVT-1"),
		TicketID:  ticket.ID("TCK-42"),
		Kind:      ticket.TicketEventKindTicket,
		EventTime: time.Date(2024, 9, 20, 12, 0, 0, 0, time.UTC),
	}
	event.SetPayload(ticket.TicketEventKindTicket, ticket.TicketEventBody{Ticket: &ticket.TicketEventPayload{Notes: "note"}})

	svc.appendTicketFunc = func(ctx context.Context, id ticket.ID, in ticket.TicketEventInput) (*ticket.TicketEvent, error) {
		require.Equal(t, ticket.ID("TCK-42"), id)
		require.Equal(t, ticket.ActorTypeAgent, in.Actor.Type)
		require.Equal(t, "note", in.Notes)
		return event, nil
	}

	deps := newOpDeps()
	input := Input{
		Actions: ActionList{
			&model.AppendTicketNoteAction{
				ExistingTicketOp: model.ExistingTicketOp{
					TicketID: ticket.ID("TCK-42"),
				},
				Note:      " note ",
				EventTime: timePtr(time.Date(2024, 9, 20, 12, 0, 0, 0, time.UTC)),
			},
		},
	}

	output, err := execute(deps, context.Background(), input)
	require.NoError(t, err)
	require.Len(t, output.Results, 1)
	appendResult, ok := output.Results[0].(*model.AppendTicketNoteResult)
	require.True(t, ok)
	require.NotNil(t, appendResult.Event)
	require.Equal(t, ticket.TicketEventID("EVT-1"), appendResult.Event.ID)
}

func TestExecute_UpdateWithoutExpectedVersion(t *testing.T) {
	svc := &stubService{}
	restore := TestingStub{Service: svc}.Install()
	defer restore()

	updated := &ticket.Ticket{
		ID:        ticket.ID("T-2"),
		ProjectID: ticket.ProjectID("proj-1"),
		Stage:     ticket.Stage("cancelled"),
		State:     ticket.StateWorking,
		UpdatedAt: time.Date(2024, 9, 21, 9, 0, 0, 0, time.UTC),
		Version:   optimisticlock.Version{Int64: 2, Valid: true},
	}
	svc.updateFunc = func(ctx context.Context, id ticket.ID, in ticket.UpdateInput) (*ticket.Ticket, error) {
		require.Equal(t, ticket.ID("T-2"), id)
		require.False(t, in.ExpectedVersion.Valid)
		require.NotNil(t, in.Stage)
		require.Equal(t, ticket.Stage("cancelled"), *in.Stage)
		return updated, nil
	}

	deps := newOpDeps()
	input := Input{
		Actions: ActionList{
			&model.UpdateTicketAction{
				ExistingTicketOp: model.ExistingTicketOp{
					TicketID: ticket.ID("T-2"),
				},
				Stage: strPtr("Cancelled"),
			},
		},
	}

	output, err := execute(deps, context.Background(), input)
	require.NoError(t, err)
	require.Len(t, output.Results, 1)
	updateResult, ok := output.Results[0].(*model.UpdateResult)
	require.True(t, ok)
	require.NotNil(t, updateResult.Ticket)
	require.Equal(t, ticket.Stage("cancelled"), updateResult.Ticket.Stage)
}

func TestExecute_UpdateRequiresField(t *testing.T) {
	svc := &stubService{}
	restore := TestingStub{Service: svc}.Install()
	defer restore()

	deps := newOpDeps()
	input := Input{
		Actions: ActionList{
			&model.UpdateTicketAction{
				ExistingTicketOp: model.ExistingTicketOp{
					TicketID: ticket.ID("T-1"),
				},
				ExpectedVersion: int64Ptr(2),
			},
		},
	}

	_, err := execute(deps, context.Background(), input)
	require.Error(t, err)
	appErr := new(workflow.NonRetryableError)
	require.True(t, errors.As(err, &appErr))
	require.True(t, appErr.NonRetryable())
}

func TestExecute_ResetFetchesLatestTicket(t *testing.T) {
	svc := &stubService{}
	restore := TestingStub{Service: svc}.Install()
	defer restore()

	svc.resetFunc = func(ctx context.Context, id ticket.ID, in ticket.TicketResetInput) (*ticket.TicketReset, error) {
		require.Equal(t, ticket.ID("T-55"), id)
		return &ticket.TicketReset{ID: ticket.TicketResetID("RST-1"), TicketID: id, Reason: "cleanup", CreatedAt: time.Date(2024, 9, 20, 18, 0, 0, 0, time.UTC)}, nil
	}
	svc.getTicketAtFunc = func(ctx context.Context, id ticket.ID, at time.Time) (*ticket.Ticket, error) {
		require.Equal(t, ticket.ID("T-55"), id)
		return &ticket.Ticket{
			ID:        id,
			Stage:     ticket.Stage("open"),
			State:     ticket.StateWorking,
			UpdatedAt: at,
			Version:   optimisticlock.Version{Int64: 5, Valid: true},
		}, nil
	}

	deps := newOpDeps()
	input := Input{
		Actions: ActionList{
			&model.ResetTicketAction{
				ExistingTicketOp: model.ExistingTicketOp{
					TicketID: ticket.ID("T-55"),
				},
				Reason: "cleanup",
			},
		},
	}

	output, err := execute(deps, context.Background(), input)
	require.NoError(t, err)
	require.Len(t, output.Results, 1)
	resetResult, ok := output.Results[0].(*model.ResetResult)
	require.True(t, ok)
	require.NotNil(t, resetResult.Ticket)
	require.Equal(t, ticket.Stage("open"), resetResult.Ticket.Stage)
}

func TestExecute_ErrorMappingVersionConflict(t *testing.T) {
	svc := &stubService{}
	restore := TestingStub{Service: svc}.Install()
	defer restore()

	svc.updateFunc = func(ctx context.Context, id ticket.ID, in ticket.UpdateInput) (*ticket.Ticket, error) {
		return nil, ticket.ErrVersionConflict
	}

	deps := newOpDeps()
	input := Input{
		Actions: ActionList{
			&model.UpdateTicketAction{
				ExistingTicketOp: model.ExistingTicketOp{
					TicketID: ticket.ID("T-1"),
				},
				ExpectedVersion: int64Ptr(2),
				Stage:           strPtr("Cancelled"),
			},
		},
	}

	_, err := execute(deps, context.Background(), input)
	require.Error(t, err)
	appErr := new(workflow.NonRetryableError)
	require.True(t, errors.As(err, &appErr))
	require.True(t, appErr.NonRetryable())
}

func strPtr(v string) *string {
	return &v
}

func int64Ptr(v int64) *int64 {
	return &v
}

func timePtr(t time.Time) *time.Time {
	return &t
}
