package recipes

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/colony-2/colony2/server/api/internal/opssetup"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflow"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/stretchr/testify/require"
)

type noopJobContext struct{}

func (n *noopJobContext) GetJobKey() swf.JobKey            { return swf.JobKey{TenantId: "tenant", JobId: "job"} }
func (n *noopJobContext) Logger() *slog.Logger             { return slog.Default() }
func (n *noopJobContext) AwaitDuration(swf.Duration) error { return nil }
func (n *noopJobContext) SpawnAsync(string, swf.TaskData) (*swf.Future, error) {
	return nil, fmt.Errorf("not supported")
}
func (n *noopJobContext) DoTask(swf.RunPolicy, string, swf.TaskData) (swf.TaskData, error) {
	return nil, fmt.Errorf("unexpected task invocation")
}

func TestValidateRecipeR1(t *testing.T) {
	originalOps := coreops.List()
	coreops.Clear()
	opssetup.RegisterOps()
	t.Cleanup(func() {
		coreops.Clear()
		if len(originalOps) > 0 {
			coreops.Register(originalOps...)
		}
	})

	jobCtx := contextual.JobContext{
		Actor:       contextual.ActorContext{TicketID: "TICKET-123"},
		Environment: contextual.EnvironmentContext{},
		Workflow:    contextual.WorkflowContext{CellPath: "cells/test-cell"},
	}
	gitCtx := contextual.GitCommitContext{ParentRef: "HEAD"}

	ctx := workflow.Context{
		JobContext:           &noopJobContext{},
		ServiceDependencies2: coreops.NewServiceDepsBuilder().Build(),
	}

	t.Run("invalid recipe", func(t *testing.T) {
		rec, err := recipe.LoadRecipeFromString([]byte(`id: r1
version: "0.0.1"
desc: Default new ticket auto-complete recipe
sequence:
  - op: input
    id: q1
    inputs:
      form:
        question: "provide a user prompt"
  - op: codex.exec
    inputs:
      prompt: "{{ sequence.q1.outputs.fields.response }}"
  - op: ticket.manage
    inputs:
      ticket_id: "{{ context.actor.ticket_id }}"
      actions:
        - type: update_ticket
          expected_version: 1
          stage: "__completed__"
          state: "waiting_user"
  - op: command_execution
    inputs:
      run: "echo {{ context.environment.worktree_path }}/{{ context.workflow.cell_path }}"
  - op: command_execution
    inputs:
      run: "ls {{ context.environment.worktree_path }}/{{ context.workflow.cell_path }}"
  - op: command_execution
    inputs:
      run: "cat {{ context.environment.worktree_path }}/{{ context.workflow.cell_path }}/README.md"
  - op: codex.exec
    inputs:
      prompt: update the readme to be more concise. avoid any reference to temporal
`))
		require.NoError(t, err)

		_, err = compiler.ExecuteRecipe(ctx, *rec, nil, jobCtx, gitCtx, compiler.ExecutionOptions{Mode: compiler.ExecutionModeValidate})
		require.Error(t, err)
	})

	t.Run("valid recipe", func(t *testing.T) {
		rec, err := recipe.LoadRecipeFromString([]byte(`id: r1
version: "0.0.1"
desc: Default new ticket auto-complete recipe
sequence:
  - op: input
    id: q1
    inputs:
      form:
        question: "provide a user prompt"
  - op: codex.exec
    inputs:
      prompt: "{{ sequence.q1.outputs.response }}"
  - op: ticket.manage
    inputs:
      ticket_id: "{{ context.actor.ticket_id }}"
      actions:
        - type: update_ticket
          expected_version: 1
          stage: "__completed__"
          state: "waiting_user"
  - op: command_execution
    inputs:
      run: "echo {{ context.environment.worktree_path }}/{{ context.workflow.cell_path }}"
  - op: command_execution
    inputs:
      run: "ls {{ context.environment.worktree_path }}/{{ context.workflow.cell_path }}"
  - op: command_execution
    inputs:
      run: "cat {{ context.environment.worktree_path }}/{{ context.workflow.cell_path }}/README.md"
  - op: codex.exec
    inputs:
      prompt: update the readme to be more concise. avoid any reference to temporal
`))
		require.NoError(t, err)

		result, err := compiler.ExecuteRecipe(ctx, *rec, nil, jobCtx, gitCtx, compiler.ExecutionOptions{Mode: compiler.ExecutionModeValidate})
		require.NoError(t, err)
		require.NotNil(t, result)
	})
}
