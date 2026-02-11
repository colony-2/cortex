package story

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/colony-2/colony2/server/git/pkg/gitstate"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	coretasks "github.com/colony-2/colony2/server/recipe-core/pkg/task"
	"github.com/colony-2/colony2/server/workflow/internal/model"
	"github.com/colony-2/swf-go/pkg/swf"
)

type activityInvocationOutput struct {
	GitResult contextual.GitCommitContext `json:"git,omitempty"`
	NextTask  string                      `json:"nextTaskType,omitempty"`
	OpOutput  any                         `json:"output"`
}

type activityInvocationRequest struct {
	Input        any                           `json:"input"`
	GitTaskCtx   gitstate.GlobalGitTaskContext `json:"context"`
	ArtifactKeys []swf.ArtifactKey             `json:"artifact_keys,omitempty"`
	Artifacts    map[string]swf.ArtifactKey    `json:"artifacts,omitempty"`
}

func splitInvocationNodePath(nodePath string) []string {
	nodePath = strings.TrimSpace(nodePath)
	if nodePath == "" {
		return make([]string, 0)
	}
	parts := strings.Split(nodePath, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func setStoryNodePath(n *model.JobRunStoryNode, invocationNodePath string, extra ...string) {
	if n == nil {
		return
	}
	path := splitInvocationNodePath(invocationNodePath)
	for _, seg := range extra {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		path = append(path, seg)
	}
	n.Path = path
}

func fillAttemptOnNode(n *model.JobRunStoryNode, taskType string, att swf.TaskAttempt, jobID string) {
	n.Attempt = att.Attempt
	n.Status = statusFromAttempt(att)
	t := att.CreatedAt
	n.StartedAt = &t
	n.TaskOrdinal = &att.Ordinal

	// Try to decode the activity invocation request so input and invoke_seq are recipe-centric.
	var req activityInvocationRequest
	if att.Input != nil && len(att.Input.Data) > 0 && json.Unmarshal(att.Input.Data, &req) == nil {
		n.Input = req.Input
		n.InvokeSeq = req.GitTaskCtx.InvokeSeq
		if strings.TrimSpace(req.GitTaskCtx.NodePath) != "" {
			if n.Kind == model.JobRunStoryNodeKindOpStep && strings.TrimSpace(n.StepID) != "" {
				setStoryNodePath(n, req.GitTaskCtx.NodePath, "step:"+strings.TrimSpace(n.StepID))
			} else {
				setStoryNodePath(n, req.GitTaskCtx.NodePath)
			}
		}
	}
	if n.Input == nil && att.Input != nil && len(att.Input.Data) > 0 {
		// Some non-activity chapters store a task envelope in the "input" field. For API consumers,
		// unwrap the envelope and return only the payload.
		if payload, ok := unwrapTaskEnvelopePayload(att.Input.Data); ok {
			n.Input = payload
		} else {
			// Best-effort: preserve raw input for unknown/non-envelope chapters.
			var raw any
			if json.Unmarshal(att.Input.Data, &raw) == nil {
				n.Input = raw
			}
		}
	}

	// Decode the task output envelope so output is recipe-centric.
	if att.Output != nil && len(att.Output.Data) > 0 {
		var outEnv coretasks.OutputEnvelope
		if json.Unmarshal(att.Output.Data, &outEnv) == nil && outEnv.Version == coretasks.OutputEnvelopeVersion {
			switch outEnv.Kind {
			case coretasks.OutputKindActivityInvocationOutput:
				var env activityInvocationOutput
				if outEnv.DecodePayload(&env) == nil {
					n.Output = env.OpOutput
				}
			case coretasks.OutputKindContextPatch:
				var patch coretasks.ContextPatch
				if outEnv.DecodePayload(&patch) == nil {
					n.Output = patch
				}
			default:
				n.Output = map[string]any{"kind": string(outEnv.Kind)}
			}
		}
	}

	// Artifacts from output IO.
	keys := make([]swf.ArtifactKey, 0)
	if att.Output != nil {
		for _, info := range att.Output.Artifacts {
			if info.Key != nil {
				keys = append(keys, *info.Key)
				continue
			}
			keys = append(keys, swf.ArtifactKey{
				JobId:       jobID,
				TaskOrdinal: att.Ordinal,
				Name:        info.Name,
				SizeBytes:   info.SizeBytes,
			})
		}
	}
	n.ArtifactKeys = keys

	// Error object.
	if att.Outcome.Error != nil && strings.TrimSpace(att.Outcome.Error.Message) != "" {
		n.Error = &model.JobRunStoryError{
			Message: att.Outcome.Error.Message,
			Code:    strings.TrimSpace(att.Outcome.Error.Code),
		}
	}

	_ = taskType
}

func statusFromAttempt(att swf.TaskAttempt) model.JobRunStoryNodeStatus {
	switch att.State {
	case swf.TaskAttemptStateReady, swf.TaskAttemptStateLeased, swf.TaskAttemptStateWaiting, swf.TaskAttemptStateRunning:
		return model.JobRunStoryNodeStatusRunning
	case swf.TaskAttemptStateSucceeded:
		return model.JobRunStoryNodeStatusSucceeded
	case swf.TaskAttemptStateFailed:
		return model.JobRunStoryNodeStatusFailed
	default:
	}
	if att.Outcome.Status == swf.TaskOutcomeStatusSucceeded {
		return model.JobRunStoryNodeStatusSucceeded
	}
	if att.Outcome.Status == swf.TaskOutcomeStatusFailed {
		return model.JobRunStoryNodeStatusFailed
	}
	return model.JobRunStoryNodeStatusUnknown
}

func statusFromErr(err error, fallback model.JobRunStoryNodeStatus) model.JobRunStoryNodeStatus {
	if err == nil {
		return fallback
	}
	if errors.Is(err, ErrReplayInProgress) {
		return model.JobRunStoryNodeStatusRunning
	}
	return model.JobRunStoryNodeStatusFailed
}

func deriveContainerStatus(children []*model.JobRunStoryNode) model.JobRunStoryNodeStatus {
	if len(children) == 0 {
		return model.JobRunStoryNodeStatusUnknown
	}
	last := children[len(children)-1]
	if last != nil && last.Status == model.JobRunStoryNodeStatusRunning {
		return model.JobRunStoryNodeStatusRunning
	}
	for _, ch := range children {
		if ch != nil && ch.Status == model.JobRunStoryNodeStatusFailed {
			return model.JobRunStoryNodeStatusFailed
		}
	}
	return model.JobRunStoryNodeStatusSucceeded
}

func stepIDFromTaskType(opID, taskType string) string {
	// Expected: "<opId>:<stepId>"
	taskType = strings.TrimSpace(taskType)
	if taskType == "" {
		return ""
	}
	parts := strings.SplitN(taskType, ":", 2)
	if len(parts) != 2 {
		return taskType
	}
	if strings.TrimSpace(parts[0]) != strings.TrimSpace(opID) {
		return parts[1]
	}
	return parts[1]
}

func unwrapTaskEnvelopePayload(raw []byte) (any, bool) {
	var env coretasks.OutputEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, false
	}
	if env.Version != coretasks.OutputEnvelopeVersion || strings.TrimSpace(string(env.Kind)) == "" {
		return nil, false
	}
	if len(env.Payload) == 0 {
		return nil, true
	}
	var payload any
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return nil, false
	}
	return payload, true
}

func opIDFromTaskType(taskType string) string {
	taskType = strings.TrimSpace(taskType)
	if taskType == "" {
		return ""
	}
	parts := strings.SplitN(taskType, ":", 2)
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func rewriteActivityNextTaskIfMissing(opID, taskType string, raw []byte, nextFn func(opID, taskType string) (string, bool)) ([]byte, bool) {
	var env coretasks.OutputEnvelope
	if json.Unmarshal(raw, &env) != nil {
		return raw, false
	}
	if env.Version != coretasks.OutputEnvelopeVersion || env.Kind != coretasks.OutputKindActivityInvocationOutput {
		return raw, false
	}
	var payload activityInvocationOutput
	if env.DecodePayload(&payload) != nil {
		return raw, false
	}
	if strings.TrimSpace(payload.NextTask) != "" {
		return raw, false
	}
	next, ok := nextFn(opID, taskType)
	if !ok || strings.TrimSpace(next) == "" {
		return raw, false
	}
	payload.NextTask = next
	rebuilt, err := coretasks.NewOutputEnvelope(env.Kind, payload)
	if err != nil {
		return raw, false
	}
	out, err := json.Marshal(rebuilt)
	if err != nil {
		return raw, false
	}
	return out, true
}

func outputKindFromRaw(raw []byte) (coretasks.OutputKind, bool) {
	var env coretasks.OutputEnvelope
	if json.Unmarshal(raw, &env) != nil {
		return "", false
	}
	if env.Version != coretasks.OutputEnvelopeVersion {
		return "", false
	}
	return env.Kind, true
}

func findFirstChildStep(children []*model.JobRunStoryNode) *model.JobRunStoryNode {
	for _, ch := range children {
		if ch != nil && ch.Kind == model.JobRunStoryNodeKindOpStep {
			return ch
		}
	}
	return nil
}

func findLastChildStep(children []*model.JobRunStoryNode) *model.JobRunStoryNode {
	for i := len(children) - 1; i >= 0; i-- {
		ch := children[i]
		if ch != nil && ch.Kind == model.JobRunStoryNodeKindOpStep {
			return ch
		}
	}
	return nil
}
