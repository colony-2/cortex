package recipeset

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/colony-2/colony2/server/recipe-child/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/runmetadata"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflow"
)

// Input defines the payload for the recipe_set op.
type Input struct {
	Recipes []map[string]interface{} `json:"recipes"`
	Raw     map[string]interface{}   `json:"-" mapstructure:",remain"`
}

// Output is empty because the op returns no payload on success.
type Output struct{}

type childPlan struct {
	index  int
	name   string
	inputs map[string]interface{}
	extras map[string]interface{}
}

type scheduledChild struct {
	plan   childPlan
	future workflow.ChildWorkflowFuture
	ctx    workflow.Context
	exec   workflow.Execution
}

// GetOp registers the recipe_set inline op.
func GetOp() ops.RegisterableOp {
	return ops.NewActivityMappedOpV2[Input, Output](ops.OpMetadata{
		Type:           "recipe_set",
		Description:    "Executes multiple recipes in parallel with discrete async children and waits for completion",
		Version:        "1.0.0",
		DefaultTimeout: 35 * time.Minute,
	}, execute)
}

func execute(deps ops.OpDependencies, ctx context.Context, in Input) (map[string]interface{}, error) {
	if len(input.Recipes) == 0 {
		return nil, workflow.NewNonRetryableApplicationError("inputs.recipes must contain at least one entry", "INVALID_RECIPE_SET", nil)
	}

	base := cloneMap(input.Raw)
	delete(base, "recipes")

	plans := make([]childPlan, 0, len(input.Recipes))
	for idx, raw := range input.Recipes {
		plan, err := buildChildPlan(idx, raw)
		if err != nil {
			return Output{}, err
		}
		plans = append(plans, plan)
	}

	logger := workflow.GetLogger(ctx)
	scheduled := make([]scheduledChild, 0, len(plans))
	statuses := make([]map[string]interface{}, len(plans))
	anyFailure := false

	for _, plan := range plans {
		if segment, remaining := resumeSegmentForPlan(inv, plan.index); segment != nil {
			_, err := resetRecipesetChild(inv, ctx, timeout, retry, plan, segment, remaining)
			if err != nil {
				anyFailure = true
				statuses[plan.index] = failureStatus(plan.index, plan.name, err)
			} else {
				statuses[plan.index] = successStatus(plan.index, plan.name)
			}
			continue
		}

		callBase := cloneMap(base)
		for k, v := range plan.extras {
			callBase[k] = v
		}

		launch, err := recipe.LaunchDiscreteAsyncChild(inv, ctx, timeout, retry, callBase, plan.name, plan.inputs)
		if err != nil {
			return Output{}, wrapPlanError(plan, err)
		}

		var exec workflow.Execution
		if err := launch.Future.GetChildWorkflowExecution().Get(launch.Ctx, &exec); err != nil {
			story.RecordChildRecipeResult(launch.Ctx, inv, story.ChildRecipeResultPayload{
				Status:       "start_failed",
				CompletedAt:  workflow.Now(launch.Ctx),
				ErrorMessage: err.Error(),
				RecipeName:   plan.name,
				Metadata: map[string]interface{}{
					"index": plan.index,
				},
			})
			return Output{}, wrapPlanError(plan, err)
		}

		if err := recipe.SignalChildRunMetadata(ctx, inv, exec, nil, &plan.index); err != nil {
			story.RecordChildRecipeResult(launch.Ctx, inv, story.ChildRecipeResultPayload{
				ChildWorkflowID: exec.ID,
				ChildRunID:      exec.RunID,
				Status:          "start_failed",
				CompletedAt:     workflow.Now(launch.Ctx),
				ErrorMessage:    err.Error(),
				RecipeName:      plan.name,
				Metadata: map[string]interface{}{
					"index": plan.index,
				},
			})
			return Output{}, wrapPlanError(plan, err)
		}

		story.RecordChildRecipeTrigger(launch.Ctx, inv, story.ChildRecipeTriggerPayload{
			ChildWorkflowID: exec.ID,
			ChildRunID:      exec.RunID,
			TriggeredAt:     workflow.Now(launch.Ctx),
			RecipeName:      plan.name,
			Inputs:          cloneMap(launch.Inputs),
			Metadata: map[string]interface{}{
				"index": plan.index,
			},
		})

		logger.Info("recipe_set child scheduled", "index", plan.index, "recipe", plan.name, "workflow_id", exec.ID, "run_id", exec.RunID)

		scheduled = append(scheduled, scheduledChild{
			plan:   plan,
			future: launch.Future,
			ctx:    launch.Ctx,
			exec:   exec,
		})
	}

	if len(scheduled) > 0 {
		// Propagate cancellation to child workflows.
		workflow.Go(ctx, func(cancelCtx workflow.Context) {
			if done := cancelCtx.Done(); done != nil {
				done.Receive(cancelCtx, nil)
				for _, child := range scheduled {
					if child.exec.ID == "" {
						continue
					}
					workflow.RequestCancelExternalWorkflow(cancelCtx, child.exec.ID, child.exec.RunID)
				}
			}
		})
	}

	for _, child := range scheduled {
		var childResult map[string]interface{}
		err := child.future.Get(child.ctx, &childResult)
		if err != nil {
			anyFailure = true
			statuses[child.plan.index] = failureStatus(child.plan.index, child.plan.name, err)
			story.RecordChildRecipeResult(child.ctx, inv, story.ChildRecipeResultPayload{
				ChildWorkflowID: child.exec.ID,
				ChildRunID:      child.exec.RunID,
				Status:          "failed",
				CompletedAt:     workflow.Now(child.ctx),
				ErrorMessage:    err.Error(),
				RecipeName:      child.plan.name,
				Metadata: map[string]interface{}{
					"index": child.plan.index,
				},
			})
		} else {
			statuses[child.plan.index] = successStatus(child.plan.index, child.plan.name)
			story.RecordChildRecipeResult(child.ctx, inv, story.ChildRecipeResultPayload{
				ChildWorkflowID: child.exec.ID,
				ChildRunID:      child.exec.RunID,
				Status:          "completed",
				CompletedAt:     workflow.Now(child.ctx),
				Outputs:         cloneMap(childResult),
				RecipeName:      child.plan.name,
				Metadata: map[string]interface{}{
					"index": child.plan.index,
				},
			})
		}
	}

	if anyFailure {
		return Output{}, temporal.NewNonRetryableApplicationError("one or more child recipes failed", "RECIPE_SET_FAILURE", nil, statuses)
	}

	return Output{}, nil
}

func buildChildPlan(index int, raw map[string]interface{}) (childPlan, error) {
	if raw == nil {
		return childPlan{}, temporal.NewNonRetryableApplicationError(fmt.Sprintf("recipes[%d] must be an object", index), "INVALID_RECIPE_SET", nil, map[string]int{"index": index})
	}

	if _, exists := raw["git_state"]; exists {
		return childPlan{}, temporal.NewNonRetryableApplicationError(fmt.Sprintf("recipes[%d] cannot set git_state", index), "INVALID_RECIPE_SET", nil, map[string]int{"index": index})
	}
	if _, exists := raw["run_mode"]; exists {
		return childPlan{}, temporal.NewNonRetryableApplicationError(fmt.Sprintf("recipes[%d] cannot set run_mode", index), "INVALID_RECIPE_SET", nil, map[string]int{"index": index})
	}

	name := stringValue(raw["recipe"])
	if name == "" {
		name = stringValue(raw["name"])
	}
	if name == "" {
		return childPlan{}, temporal.NewNonRetryableApplicationError(fmt.Sprintf("recipes[%d] must define recipe", index), "INVALID_RECIPE_SET", nil, map[string]int{"index": index})
	}

	inputs, err := extractInputs(index, raw)
	if err != nil {
		return childPlan{}, err
	}

	extras := make(map[string]interface{})
	for k, v := range raw {
		switch k {
		case "recipe", "name", "inputs", "input":
			continue
		default:
			extras[k] = v
		}
	}

	return childPlan{
		index:  index,
		name:   name,
		inputs: inputs,
		extras: extras,
	}, nil
}

func resetRecipesetChild(inv ops.OpDependencies, ctx workflow.Context, timeout time.Duration, retry *temporal.RetryPolicy, plan childPlan, segment *runmetadata.Segment, remaining *runmetadata.Resume) (map[string]interface{}, error) {
	logger := workflow.GetLogger(ctx)
	if segment.WorkflowID == "" {
		return nil, temporal.NewNonRetryableApplicationError("resume segment missing workflow_id", "RESET_INVALID_SEGMENT", nil, map[string]interface{}{"index": plan.index})
	}

	reason := ""
	if inv.Deps != nil {
		if meta, ok := inv.Deps.RunMetadata(); ok && meta != nil {
			reason = meta.Reason
		}
	}

	activityTimeout := timeout
	if activityTimeout <= 0 {
		activityTimeout = 30 * time.Minute
	}
	activityOpts := workflow.ActivityOptions{
		StartToCloseTimeout: activityTimeout,
		RetryPolicy:         retry,
	}
	actx := workflow.WithActivityOptions(ctx, activityOpts)

	var resetResult recipe.ResetChildWorkflowActivityResult
	input := recipe.ResetChildWorkflowActivityInput{
		WorkflowID:        segment.WorkflowID,
		RunID:             segment.RunID,
		EventID:           segment.EventID,
		Reason:            reason,
		WaitForCompletion: true,
	}
	if err := workflow.ExecuteActivity(actx, recipe.ResetChildWorkflowActivityName, input).Get(actx, &resetResult); err != nil {
		story.RecordChildRecipeResult(ctx, inv, story.ChildRecipeResultPayload{
			ChildWorkflowID: segment.WorkflowID,
			ChildRunID:      segment.RunID,
			Status:          "start_failed",
			CompletedAt:     workflow.Now(ctx),
			ErrorMessage:    err.Error(),
			RecipeName:      plan.name,
			Metadata: map[string]interface{}{
				"index": plan.index,
				"reset": true,
			},
		})
		return nil, err
	}

	childExec := workflow.Execution{ID: segment.WorkflowID, RunID: resetResult.RunID}
	if err := recipe.SignalChildRunMetadata(ctx, inv, childExec, remaining, &plan.index); err != nil {
		story.RecordChildRecipeResult(ctx, inv, story.ChildRecipeResultPayload{
			ChildWorkflowID: childExec.ID,
			ChildRunID:      childExec.RunID,
			Status:          "start_failed",
			CompletedAt:     workflow.Now(ctx),
			ErrorMessage:    err.Error(),
			RecipeName:      plan.name,
			Metadata: map[string]interface{}{
				"index": plan.index,
				"reset": true,
			},
		})
		return nil, err
	}

	story.RecordChildRecipeTrigger(ctx, inv, story.ChildRecipeTriggerPayload{
		ChildWorkflowID: childExec.ID,
		ChildRunID:      childExec.RunID,
		TriggeredAt:     workflow.Now(ctx),
		RecipeName:      plan.name,
		Inputs:          cloneMap(plan.inputs),
		Metadata: map[string]interface{}{
			"index": plan.index,
			"reset": true,
		},
	})

	outputs := resetResult.Result
	if outputs == nil {
		outputs = make(map[string]interface{})
	}
	logger.Info("recipe_set child reset completed", "index", plan.index, "recipe", plan.name, "workflow_id", childExec.ID, "run_id", childExec.RunID, "output_keys", len(outputs))
	story.RecordChildRecipeResult(ctx, inv, story.ChildRecipeResultPayload{
		ChildWorkflowID: childExec.ID,
		ChildRunID:      childExec.RunID,
		Status:          "completed",
		CompletedAt:     workflow.Now(ctx),
		Outputs:         cloneMap(outputs),
		RecipeName:      plan.name,
		Metadata: map[string]interface{}{
			"index": plan.index,
			"reset": true,
		},
	})

	return outputs, nil
}

func resumeSegmentForPlan(inv ops.OpDependencies, index int) (*runmetadata.Segment, *runmetadata.Resume) {
	if inv.Deps == nil {
		return nil, nil
	}
	resume, ok := inv.Deps.ResumeMetadata()
	if !ok || resume == nil || len(resume.ExecutionPath) == 0 {
		return nil, nil
	}
	segment := resume.ExecutionPath[0]
	if segment.InvocationHash != invocationHash(inv) {
		return nil, nil
	}
	if segment.RecipeSetIndex == nil || *segment.RecipeSetIndex != index {
		return nil, nil
	}
	segCopy := segment
	remaining := cloneResumeWithoutHead(resume)
	return &segCopy, remaining
}

func cloneResumeWithoutHead(resume *runmetadata.Resume) *runmetadata.Resume {
	if resume == nil || len(resume.ExecutionPath) <= 1 {
		return nil
	}
	cloned := make([]runmetadata.Segment, len(resume.ExecutionPath)-1)
	copy(cloned, resume.ExecutionPath[1:])
	return &runmetadata.Resume{ExecutionPath: cloned}
}

func invocationHash(inv ops.OpDependencies) string {
	if inv.ID != "" {
		return inv.ID
	}
	return inv.Hash()
}

func extractInputs(index int, raw map[string]interface{}) (map[string]interface{}, error) {
	if raw == nil {
		return make(map[string]interface{}), nil
	}

	if v, ok := raw["inputs"]; ok {
		inputs, err := ensureMap(v)
		if err != nil {
			return nil, temporal.NewNonRetryableApplicationError(fmt.Sprintf("recipes[%d].inputs must be an object", index), "INVALID_RECIPE_SET", nil, map[string]int{"index": index})
		}
		return cloneMap(inputs), nil
	}

	if v, ok := raw["input"]; ok {
		inputs, err := ensureMap(v)
		if err != nil {
			return nil, temporal.NewNonRetryableApplicationError(fmt.Sprintf("recipes[%d].input must be an object", index), "INVALID_RECIPE_SET", nil, map[string]int{"index": index})
		}
		return cloneMap(inputs), nil
	}

	return make(map[string]interface{}), nil
}

func ensureMap(value interface{}) (map[string]interface{}, error) {
	if value == nil {
		return make(map[string]interface{}), nil
	}
	m, ok := value.(map[string]interface{})
	if !ok {
		return nil, errors.New("not a map")
	}
	return m, nil
}

func wrapPlanError(plan childPlan, err error) error {
	detail := map[string]interface{}{
		"index":  plan.index,
		"recipe": plan.name,
	}
	return temporal.NewNonRetryableApplicationError(err.Error(), "RECIPE_SET_CHILD_ERROR", err, detail)
}

func successStatus(index int, name string) map[string]interface{} {
	return map[string]interface{}{
		"index":  index,
		"recipe": name,
		"status": "succeeded",
	}
}

func failureStatus(index int, name string, err error) map[string]interface{} {
	status := map[string]interface{}{
		"index":  index,
		"recipe": name,
		"status": "failed",
	}
	if err != nil {
		status["error"] = serializeError(err)
	}
	return status
}

func serializeError(err error) map[string]interface{} {
	if err == nil {
		return nil
	}
	detail := map[string]interface{}{
		"message": err.Error(),
	}

	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) {
		if appErr.Type() != "" {
			detail["type"] = appErr.Type()
		}
		detail["non_retryable"] = appErr.NonRetryable()
		var data []interface{}
		if derr := appErr.Details(&data); derr == nil && len(data) > 0 {
			detail["details"] = data
		}
	}

	var timeoutErr *temporal.TimeoutError
	if errors.As(err, &timeoutErr) {
		if _, hasType := detail["type"]; !hasType {
			detail["type"] = "Timeout"
		}
		detail["timeout_type"] = timeoutErr.TimeoutType().String()
	}

	var canceledErr *temporal.CanceledError
	if errors.As(err, &canceledErr) {
		detail["type"] = "Canceled"
		if canceledErr.HasDetails() {
			var data []interface{}
			if derr := canceledErr.Details(&data); derr == nil && len(data) > 0 {
				detail["details"] = data
			}
		}
	}

	return detail
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return make(map[string]interface{})
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func stringValue(val interface{}) string {
	if val == nil {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return fmt.Sprint(val)
}
