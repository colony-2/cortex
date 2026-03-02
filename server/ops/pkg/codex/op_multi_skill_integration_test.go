package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/starter"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/commandop"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	workerops "github.com/colony-2/colony2/server/recipe-worker/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/workflow"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/colony-2/swf-go/pkg/swf/toy"
	"github.com/stretchr/testify/require"
)

type multiSkillLoopOutput struct {
	RunCount             int    `json:"run_count"`
	Run1Status           string `json:"run1_status"`
	Run2Status           string `json:"run2_status"`
	Run3Status           string `json:"run3_status"`
	Run1Skill            string `json:"run1_skill"`
	Run2Skill            string `json:"run2_skill"`
	Run3Skill            string `json:"run3_skill"`
	Run1Subskill         string `json:"run1_subskill"`
	Run2Subskill         string `json:"run2_subskill"`
	Run3Subskill         string `json:"run3_subskill"`
	Run1CheckpointStatus string `json:"run1_checkpoint_status"`
	Run2CheckpointStatus string `json:"run2_checkpoint_status"`
	Run3CheckpointStatus string `json:"run3_checkpoint_status"`
	Run1NextAction       string `json:"run1_next_action"`
	Run2NextAction       string `json:"run2_next_action"`
	Run3NextAction       string `json:"run3_next_action"`
	Run1ReturnTriggered  bool   `json:"run1_return_triggered"`
	Run2ReturnTriggered  bool   `json:"run2_return_triggered"`
	Run3ReturnTriggered  bool   `json:"run3_return_triggered"`
	Run2Scope            string `json:"run2_scope"`
	Run2BlockingSkill    string `json:"run2_blocking_skill"`
	Run1SelectionMode    string `json:"run1_selection_mode"`
	Run2SelectionMode    string `json:"run2_selection_mode"`
	Run3SelectionMode    string `json:"run3_selection_mode"`
}

func TestCodexOpMultiSkillStateMachineLoop(t *testing.T) {
	ensureCodexRequired(t)
	t.Setenv("VIBETHIS_CODEX_USE_DIRECT", "1")
	t.Setenv("RUST_LOG", "codex_core::rollout::list=off")

	originalOps := coreops.List()
	t.Cleanup(func() {
		coreops.Clear()
		if len(originalOps) > 0 {
			coreops.Register(originalOps...)
		}
	})
	coreops.Clear()
	coreops.Register(commandop.GetOp())
	coreops.Register(GetOp())

	cellRel := filepath.Join("cells", "alpha")
	token := "amber-lion-42"

	recipeYaml := fmt.Sprintf(`
---
id: codex-op-multi-skill-loop
state:
  initial: seed
  states:
    seed:
      op: command_execution
      inputs:
        working_directory: "{{ context.environment.outbox }}"
        run: |
          set -euo pipefail
          mkdir -p codex-home/skills/software-dev-orchestrator
          mkdir -p codex-home/skills/software-dev-plan
          mkdir -p codex-home/skills/software-dev-execute
          mkdir -p codex-home/skills/software-dev-test
          mkdir -p implementation
          cat > implementation/latest-status.json <<'JSON'
          {"checkpoint":{"status":"start","scope":"top_level","stack":[{"skill":"software-dev-orchestrator","scope":"top_level"}]},"summary":{"human":"seed","reason":"init"}}
          JSON

          cat > codex-home/skills/software-dev-orchestrator/SKILL.md <<'EOF_SKILL'
          ---
          name: software-dev-orchestrator
          description: Top-level software development workflow coordinator that routes between planning, execution, and testing subskills across continue turns.
          ---
          # Software Dev Orchestrator

          ## Objective
          Route a single workflow segment per invocation. Do not implement phase logic here; delegate to subskills.

          ## Subskills
          - software-dev-plan ($CODEX_HOME/skills/software-dev-plan/SKILL.md)
          - software-dev-execute ($CODEX_HOME/skills/software-dev-execute/SKILL.md)
          - software-dev-test ($CODEX_HOME/skills/software-dev-test/SKILL.md)

          ## Routing Rules
          1. Read ../inbox/implementation/latest-status.json.
          2. If checkpoint.status == start, execute software-dev-plan.
          3. If checkpoint.status == checkpoint_ready, execute software-dev-execute.
          4. If checkpoint.status == needs_user_input, execute software-dev-test.
          5. If checkpoint.status == completed, do not restart planning.

          ## Execution Requirements
          1. Before running a subskill, read that subskill file from disk and follow it exactly.
          2. Keep machine-routing state in ../outbox/implementation/latest-status.json.
          3. Respond only with schema JSON.
          EOF_SKILL

          cat > codex-home/skills/software-dev-plan/SKILL.md <<'EOF_SKILL'
          ---
          name: software-dev-plan
          description: Planning segment for software-dev-orchestrator. Creates the first checkpoint artifact and schedules execute phase.
          ---
          # Planning Skill

          ## Steps
          1. Run this command exactly:
          mkdir -p ../outbox/implementation
          cat > ../outbox/implementation/latest-status.json <<'JSON'
          {"checkpoint":{"status":"checkpoint_ready","scope":"top_level","stack":[{"skill":"software-dev-orchestrator","scope":"top_level"},{"skill":"software-dev-plan","scope":"nested"}]},"next_skill_candidates":["software-dev-execute"]}
          JSON
          2. Respond with schema JSON and assistantSummary exactly planning complete.
          EOF_SKILL

          cat > codex-home/skills/software-dev-execute/SKILL.md <<'EOF_SKILL'
          ---
          name: software-dev-execute
          description: Execution segment for software-dev-orchestrator. Creates nested checkpoint for test phase and validates session memory.
          ---
          # Execution Skill

          ## Steps
          1. Run this command exactly:
          mkdir -p ../outbox/implementation
          cat > ../outbox/implementation/latest-status.json <<'JSON'
          {"checkpoint":{"status":"needs_user_input","scope":"nested","blocking_skill":"software-dev-test","stack":[{"skill":"software-dev-orchestrator","scope":"top_level"},{"skill":"software-dev-execute","scope":"nested"}]},"next_skill_candidates":["software-dev-test"]}
          JSON
          2. Respond with schema JSON and assistantSummary exactly token: <token> where <token> is the token remembered from the initial user prompt in this same session.
          EOF_SKILL

          cat > codex-home/skills/software-dev-test/SKILL.md <<'EOF_SKILL'
          ---
          name: software-dev-test
          description: Testing segment for software-dev-orchestrator. Marks workflow complete.
          ---
          # Testing Skill

          ## Steps
          1. Run this command exactly:
          mkdir -p ../outbox/implementation
          cat > ../outbox/implementation/latest-status.json <<'JSON'
          {"checkpoint":{"status":"completed","scope":"top_level","stack":[{"skill":"software-dev-orchestrator","scope":"top_level"},{"skill":"software-dev-test","scope":"nested"}]}}
          JSON
          2. Respond with schema JSON and assistantSummary exactly validation complete.
          EOF_SKILL
      transitions:
        - to: codex_loop
          when: 'true'

    codex_loop:
      op: codex.exec
      artifacts:
        codex-home/skills/software-dev-orchestrator/SKILL.md: '${{ states.seed.artifacts["codex-home/skills/software-dev-orchestrator/SKILL.md"] }}'
        codex-home/skills/software-dev-plan/SKILL.md: '${{ states.seed.artifacts["codex-home/skills/software-dev-plan/SKILL.md"] }}'
        codex-home/skills/software-dev-execute/SKILL.md: '${{ states.seed.artifacts["codex-home/skills/software-dev-execute/SKILL.md"] }}'
        codex-home/skills/software-dev-test/SKILL.md: '${{ states.seed.artifacts["codex-home/skills/software-dev-test/SKILL.md"] }}'
        implementation/latest-status.json: '${{ "codex_loop" in states && "implementation/latest-status.json" in states["codex_loop"].artifacts ? states["codex_loop"].artifacts["implementation/latest-status.json"] : states.seed.artifacts["implementation/latest-status.json"] }}'
      inputs:
        sessionId: '${{ "codex_loop" in states ? states["codex_loop"].outputs.sessionId : "" }}'
        skill: "software-dev-orchestrator"
        skill_mode: "enforce"
        status_contract:
          path: "implementation/latest-status.json"
        cell_relative_path: %q
        prompt: '${{ !("codex_loop" in states) ? "Implement a small change using your software-dev workflow. Remember this token for follow-up turns: %s" : "continue" }}'
      transitions:
        - to: codex_loop
          when: 'outputs.status == "incomplete"'

outputs:
  run_count: '${{ size(states["codex_loop"].runs) + 1 }}'
  run1_status: '${{ states["codex_loop"].runs[0].outputs.status }}'
  run2_status: '${{ states["codex_loop"].runs[1].outputs.status }}'
  run3_status: '${{ states["codex_loop"].outputs.status }}'
  run1_skill: '${{ states["codex_loop"].runs[0].outputs.outcome.skill.executed }}'
  run2_skill: '${{ states["codex_loop"].runs[1].outputs.outcome.skill.executed }}'
  run3_skill: '${{ states["codex_loop"].outputs.outcome.skill.executed }}'
  run1_subskill: '${{ states["codex_loop"].runs[0].outputs.outcome.checkpoint.stack[1].skill }}'
  run2_subskill: '${{ states["codex_loop"].runs[1].outputs.outcome.checkpoint.stack[1].skill }}'
  run3_subskill: '${{ states["codex_loop"].outputs.outcome.checkpoint.stack[1].skill }}'
  run1_checkpoint_status: '${{ states["codex_loop"].runs[0].outputs.outcome.checkpoint.status }}'
  run2_checkpoint_status: '${{ states["codex_loop"].runs[1].outputs.outcome.checkpoint.status }}'
  run3_checkpoint_status: '${{ states["codex_loop"].outputs.outcome.checkpoint.status }}'
  run1_next_action: '${{ states["codex_loop"].runs[0].outputs.outcome.routing.nextAction }}'
  run2_next_action: '${{ states["codex_loop"].runs[1].outputs.outcome.routing.nextAction }}'
  run3_next_action: '${{ states["codex_loop"].outputs.outcome.routing.nextAction }}'
  run1_return_triggered: '${{ states["codex_loop"].runs[0].outputs.outcome.checkpoint.returnTriggered }}'
  run2_return_triggered: '${{ states["codex_loop"].runs[1].outputs.outcome.checkpoint.returnTriggered }}'
  run3_return_triggered: '${{ states["codex_loop"].outputs.outcome.checkpoint.returnTriggered }}'
  run2_scope: '${{ states["codex_loop"].runs[1].outputs.outcome.checkpoint.scope }}'
  run2_blocking_skill: '${{ states["codex_loop"].runs[1].outputs.outcome.checkpoint.blockingSkill }}'
  run1_selection_mode: '${{ states["codex_loop"].runs[0].outputs.outcome.skill.selectionMode }}'
  run2_selection_mode: '${{ states["codex_loop"].runs[1].outputs.outcome.skill.selectionMode }}'
  run3_selection_mode: '${{ states["codex_loop"].outputs.outcome.skill.selectionMode }}'
`, cellRel, token)

	testRecipe, err := recipe.LoadRecipeFromString([]byte(recipeYaml))
	require.NoError(t, err)

	registry, err := workerops.NewActivityRegistry()
	require.NoError(t, err)
	g := jobIDGen{max: 1}
	engine := toy.NewToyEngine([]swf.WorkSet{}, toy.WithJobIDGenerator(g.Generate))

	wf := workflow.SWFWorkflowControl{Engine: engine}
	deps := coreops.NewServiceDepsBuilder().WithWorkflowControl(&wf).Build()
	workSet, err := compiler.NewRecipeWorker(deps, registry, nil)
	require.NoError(t, err)
	workSet.JobWorker = &artifactValidatingJobWorker{inner: workSet.JobWorker, t: t}
	require.NoError(t, engine.RegisterWorkers(workSet))

	jobCtx, gitCtx := compiler.GenerateTestContext()
	jobCtx.Workflow.CellName = cellRel
	jobCtx.Workflow.CellPath = cellRel
	start := workflowctl.StartJob{
		TenantId:   "test-tenant",
		RecipeName: testRecipe.GetMetadata().ID,
		Inputs:     map[string]interface{}{},
		JobContext: jobCtx,
		GitRef:     gitCtx.ParentRef,
	}

	key, err := starter.StartRecipeJob(context.Background(), start, engine, *testRecipe)
	require.NoError(t, err)
	require.NoError(t, swf.WaitForJobToComplete(context.Background(), 4*time.Minute, key, engine))

	res, err := engine.GetJobResult(context.Background(), key)
	require.NoError(t, err)

	raw, err := res.GetData()
	require.NoError(t, err)

	var output multiSkillLoopOutput
	require.NoError(t, json.Unmarshal(raw, &output), "raw=%s", string(raw))

	require.Equal(t, 3, output.RunCount)

	require.Equal(t, string(StatusIncomplete), output.Run1Status)
	require.Equal(t, string(StatusIncomplete), output.Run2Status)
	require.Equal(t, string(StatusCompleted), output.Run3Status)

	require.Equal(t, "software-dev-orchestrator", output.Run1Skill)
	require.Equal(t, "software-dev-orchestrator", output.Run2Skill)
	require.Equal(t, "software-dev-orchestrator", output.Run3Skill)

	require.Equal(t, "software-dev-plan", output.Run1Subskill)
	require.Equal(t, "software-dev-execute", output.Run2Subskill)
	require.Equal(t, "software-dev-test", output.Run3Subskill)

	require.Equal(t, "checkpoint_ready", output.Run1CheckpointStatus)
	require.Equal(t, "needs_user_input", output.Run2CheckpointStatus)
	require.Equal(t, "completed", output.Run3CheckpointStatus)

	require.Equal(t, routingReturnToCheckpoint, output.Run1NextAction)
	require.Equal(t, routingReturnToCheckpoint, output.Run2NextAction)
	require.Equal(t, routingCompleteSkillSegment, output.Run3NextAction)

	require.True(t, output.Run1ReturnTriggered)
	require.True(t, output.Run2ReturnTriggered)
	require.False(t, output.Run3ReturnTriggered)

	require.Equal(t, checkpointScopeNested, output.Run2Scope)
	require.Equal(t, "software-dev-test", output.Run2BlockingSkill)

	require.Equal(t, skillSelectionModeAdaptive, output.Run1SelectionMode)
	require.Equal(t, skillSelectionModeAdaptive, output.Run2SelectionMode)
	require.Equal(t, skillSelectionModeAdaptive, output.Run3SelectionMode)
}
