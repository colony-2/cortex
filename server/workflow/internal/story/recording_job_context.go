package story

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	coretasks "github.com/colony-2/colony2/server/recipe-core/pkg/task"
	"github.com/colony-2/colony2/server/workflow/internal/model"
	"github.com/colony-2/swf-go/pkg/swf"
)

type recordingJobContext struct {
	*StoryBuildingContext

	jobID string
	tree  *treeBuilder

	mu            sync.Mutex
	currentOpNode *model.JobRunStoryNode
	consumeTarget *model.JobRunStoryNode
}

func newRecordingJobContext(inner *StoryBuildingContext, jobID string, tree *treeBuilder) *recordingJobContext {
	r := &recordingJobContext{
		StoryBuildingContext: inner,
		jobID:                strings.TrimSpace(jobID),
		tree:                 tree,
	}
	if inner != nil {
		inner.SetOnConsume(r.onConsume)
	}
	return r
}

func (r *recordingJobContext) SetCurrentOpNode(op *model.JobRunStoryNode) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.currentOpNode = op
}

func (r *recordingJobContext) DoTask(policy swf.RunPolicy, taskType string, input swf.TaskData) (swf.TaskData, error) {
	r.mu.Lock()
	opNode := r.currentOpNode
	r.mu.Unlock()

	// If we aren't inside an op, fall back to replay semantics without attempting to record.
	if opNode == nil || r.tree == nil || r.StoryBuildingContext == nil {
		return r.StoryBuildingContext.DoTask(policy, taskType, input)
	}

	stepID := stepIDFromTaskType(opNode.OpID, taskType)
	stepNode := r.tree.newNode(model.JobRunStoryNodeKindOpStep, "step "+strings.TrimSpace(stepID))
	stepNode.StepID = strings.TrimSpace(stepID)
	stepNode.StepType = "other"
	stepNode.Status = model.JobRunStoryNodeStatusRunning
	stepNode.InvokeSeq = opNode.InvokeSeq
	stepNode.Path = append([]string{}, opNode.Path...)
	if stepNode.StepID != "" {
		stepNode.Path = append(stepNode.Path, "step:"+stepNode.StepID)
	}
	opNode.Children = append(opNode.Children, stepNode)

	r.mu.Lock()
	r.consumeTarget = stepNode
	r.mu.Unlock()

	td, err := r.StoryBuildingContext.DoTask(policy, taskType, input)

	r.mu.Lock()
	r.consumeTarget = nil
	r.mu.Unlock()

	// Determine the effective task data for decoding/rewrites.
	effective := td
	if mismatch, ok := swf.UnexpectedChapter(err); ok {
		effective = mismatch.CachedTaskData()
		if effective != nil {
			rewritten, ok := r.rewriteTaskData(taskType, effective)
			if ok {
				mismatch.CachedOutput = rewritten
				err = mismatch
			}
		}

		// The executor expects nil TaskData when returning a determinism mismatch.
		r.maybeConvertStepNodeKind(stepNode, mismatch.CachedTaskData())
		return nil, err
	}

	if effective != nil {
		if rewritten, ok := r.rewriteTaskData(taskType, effective); ok {
			td = rewritten
			effective = rewritten
		}
	}
	r.maybeConvertStepNodeKind(stepNode, effective)
	return td, err
}

func (r *recordingJobContext) rewriteTaskData(taskType string, td swf.TaskData) (swf.TaskData, bool) {
	if td == nil {
		return td, false
	}
	raw, err := td.GetData()
	if err != nil || len(raw) == 0 {
		return td, false
	}

	opID := opIDFromTaskType(taskType)
	newRaw, changed := rewriteActivityNextTaskIfMissing(opID, taskType, raw, nextTaskFromRegistry)
	if !changed {
		return td, false
	}
	arts, err := td.GetArtifacts()
	if err != nil {
		return td, false
	}
	return &swf.SimpleTaskData{Data: json.RawMessage(newRaw), Artifacts: arts}, true
}

func (r *recordingJobContext) maybeConvertStepNodeKind(stepNode *model.JobRunStoryNode, td swf.TaskData) {
	if stepNode == nil || td == nil {
		return
	}
	raw, err := td.GetData()
	if err != nil || len(raw) == 0 {
		return
	}
	kind, ok := outputKindFromRaw(raw)
	if !ok {
		return
	}
	if kind != coretasks.OutputKindContextPatch {
		return
	}

	stepNode.Kind = model.JobRunStoryNodeKindContextPatch
	stepNode.Title = "context patch"
	stepNode.StepID = ""
	stepNode.StepType = ""

	r.mu.Lock()
	opNode := r.currentOpNode
	r.mu.Unlock()
	if opNode != nil {
		ord := int64(-1)
		if stepNode.TaskOrdinal != nil {
			ord = *stepNode.TaskOrdinal
		}
		stepNode.Path = append([]string{}, opNode.Path...)
		if ord >= 0 {
			stepNode.Path = append(stepNode.Path, fmt.Sprintf("contextPatch:%d", ord))
		} else {
			stepNode.Path = append(stepNode.Path, "contextPatch")
		}
	}

	for _, pa := range stepNode.PriorAttempts {
		if pa == nil {
			continue
		}
		pa.Kind = stepNode.Kind
		pa.Title = stepNode.Title
		pa.StepID = ""
		pa.StepType = ""
		pa.Path = append([]string{}, stepNode.Path...)
	}
}

func (r *recordingJobContext) onConsume(taskType string, run swf.TaskRun, final swf.TaskAttempt) {
	r.mu.Lock()
	n := r.consumeTarget
	r.mu.Unlock()
	if n == nil {
		return
	}
	if len(run.Attempts) == 0 {
		return
	}

	fillAttemptOnNode(n, taskType, run.Attempts[len(run.Attempts)-1], r.jobID)

	// Build prior attempts as nodes of the same kind.
	prior := make([]*model.JobRunStoryNode, 0, len(run.Attempts)-1)
	for i := 0; i < len(run.Attempts)-1; i++ {
		a := run.Attempts[i]
		pa := *n
		pa.ID = fmt.Sprintf("%s_a%d", n.ID, a.Attempt)
		pa.Attempt = a.Attempt
		pa.PriorAttempts = make([]*model.JobRunStoryNode, 0)
		pa.Children = make([]*model.JobRunStoryNode, 0)
		pa.ArtifactKeys = make([]swf.ArtifactKey, 0)
		pa.TaskOrdinal = nil
		pa.RestartFromOrdinal = nil
		fillAttemptOnNode(&pa, taskType, a, r.jobID)
		prior = append(prior, &pa)
	}
	n.PriorAttempts = prior
	n.Attempt = final.Attempt

	// Expose a safe restart ordinal for the logical node: attempt 1 (or earliest ordinal fallback).
	var restartOrd *int64
	for i := range run.Attempts {
		if run.Attempts[i].Attempt == 1 {
			v := run.Attempts[i].Ordinal
			restartOrd = &v
			break
		}
	}
	if restartOrd == nil && len(run.Attempts) > 0 {
		min := run.Attempts[0].Ordinal
		for i := 1; i < len(run.Attempts); i++ {
			if run.Attempts[i].Ordinal < min {
				min = run.Attempts[i].Ordinal
			}
		}
		restartOrd = &min
	}
	n.RestartFromOrdinal = restartOrd

	// started_at is always attempt created time.
	t := run.Attempts[0].CreatedAt
	n.StartedAt = &t

	// Status from final attempt state/outcome.
	n.Status = statusFromAttempt(final)
}
