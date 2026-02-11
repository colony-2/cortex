package story

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/colony-2/swf-go/pkg/swf"
)

// StoryBuildingContext replays a job run using recorded TaskRun/Attempt outcomes from swf.GetJobRun.
// It implements swf.JobContext so higher-level recipe logic can be executed deterministically without
// re-running any real tasks.
type StoryBuildingContext struct {
	jobKey    swf.JobKey
	logger    *slog.Logger
	engine    swf.SWFEngine
	tenantID  string
	jobType   string
	jobStatus swf.JobStatus

	mu       sync.Mutex
	runsByTy map[string][]swf.TaskRun
	cursor   map[string]int

	onConsume func(taskType string, run swf.TaskRun, final swf.TaskAttempt)
}

func (c *StoryBuildingContext) isJobTypeTaskType(taskType string) bool {
	if c == nil {
		return false
	}
	jt := strings.TrimSpace(c.jobType)
	if jt == "" {
		return false
	}
	return strings.TrimSpace(taskType) == jt
}

func (c *StoryBuildingContext) shouldIgnoreJobTypeRun(taskType string, run swf.TaskRun) bool {
	// Job-level runs for the job type (e.g. TaskType=="recipe" for jobType=="recipe") can appear
	// in the task list. For terminal attempts, these are boundary chapters and should not block
	// story replay. For in-progress attempts, they should block replay so the story surfaces as
	// "running" rather than incorrectly skipping ahead.
	if !c.isJobTypeTaskType(taskType) {
		return false
	}
	return !isTaskRunInProgress(run)
}

func NewStoryBuildingContext(engine swf.SWFEngine, tenantID string, jobKey swf.JobKey, jobType string, jobStatus swf.JobStatus, attempts []swf.JobAttempt, logger *slog.Logger) *StoryBuildingContext {
	runsByTy := make(map[string][]swf.TaskRun, 64)
	for i := range attempts {
		for _, tr := range attempts[i].Tasks {
			ty := normalizeTaskType(jobType, tr.TaskType)
			runsByTy[ty] = append(runsByTy[ty], tr)
		}
	}
	const unknownOrdinal int64 = 1<<63 - 1
	for ty := range runsByTy {
		runs := runsByTy[ty]
		sort.SliceStable(runs, func(i, j int) bool {
			oi, ok := taskRunFirstOrdinal(runs[i])
			if !ok {
				oi = unknownOrdinal
			}
			oj, ok := taskRunFirstOrdinal(runs[j])
			if !ok {
				oj = unknownOrdinal
			}
			return oi < oj
		})
		runsByTy[ty] = runs
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &StoryBuildingContext{
		jobKey:    jobKey,
		logger:    logger,
		engine:    engine,
		tenantID:  tenantID,
		jobType:   strings.TrimSpace(jobType),
		jobStatus: jobStatus,
		runsByTy:  runsByTy,
		cursor:    make(map[string]int, len(runsByTy)),
	}
}

func normalizeTaskType(jobType, taskType string) string {
	jobType = strings.TrimSpace(jobType)
	taskType = strings.TrimSpace(taskType)
	if jobType == "" || taskType == "" {
		return taskType
	}
	prefix := jobType + ":"
	if !strings.HasPrefix(taskType, prefix) {
		return taskType
	}
	rest := strings.TrimSpace(strings.TrimPrefix(taskType, prefix))
	if rest == "" {
		return taskType
	}
	return rest
}

func (c *StoryBuildingContext) SetOnConsume(fn func(taskType string, run swf.TaskRun, final swf.TaskAttempt)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onConsume = fn
}

func (c *StoryBuildingContext) GetJobKey() swf.JobKey { return c.jobKey }
func (c *StoryBuildingContext) Logger() *slog.Logger  { return c.logger }

func (c *StoryBuildingContext) AwaitDuration(_ swf.Duration) error { return nil }
func (c *StoryBuildingContext) AwaitJobs(_ ...string) error        { return nil }

// NextTaskTypeForPrefix finds the next unconsumed TaskRun whose TaskType matches the provided prefix
// (e.g. "<opName>:"). It returns the TaskType to use for the next DoTask call.
func (c *StoryBuildingContext) NextTaskTypeForPrefix(prefix string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", false
	}

	bestTy := ""
	var bestOrd int64 = -1
	for ty, runs := range c.runsByTy {
		if !strings.HasPrefix(ty, prefix) {
			continue
		}
		idx := c.cursor[ty]
		if idx >= len(runs) {
			continue
		}
		run := runs[idx]
		ord, ok := taskRunFirstOrdinal(run)
		if !ok {
			continue
		}
		if bestOrd < 0 || ord < bestOrd {
			bestOrd = ord
			bestTy = ty
		}
	}
	return bestTy, bestTy != ""
}

func (c *StoryBuildingContext) peekNextRun(taskType string) (swf.TaskRun, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	runs := c.runsByTy[taskType]
	idx := c.cursor[taskType]
	if idx >= len(runs) {
		return swf.TaskRun{}, false
	}
	return runs[idx], true
}

// peekNextRunAny returns the next unconsumed TaskRun across all task types by earliest ordinal.
// It is used to support restart-injected context patch chapters, which may not match the next
// expected task type during replay.
func (c *StoryBuildingContext) peekNextRunAny() (string, swf.TaskRun, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.peekNextRunAnyLocked()
}

func (c *StoryBuildingContext) peekNextRunAnyLocked() (string, swf.TaskRun, bool) {
	bestTy := ""
	var bestOrd int64 = -1
	var bestRun swf.TaskRun

	for ty, runs := range c.runsByTy {
		idx := c.cursor[ty]
		if idx >= len(runs) {
			continue
		}
		run := runs[idx]
		if c.shouldIgnoreJobTypeRun(ty, run) {
			continue
		}
		ord, ok := taskRunFirstOrdinal(run)
		if !ok {
			continue
		}
		if bestOrd < 0 || ord < bestOrd {
			bestOrd = ord
			bestTy = ty
			bestRun = run
		}
	}
	return bestTy, bestRun, bestTy != ""
}

func (c *StoryBuildingContext) peekNextTimeAny() (time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var best time.Time
	for ty, runs := range c.runsByTy {
		idx := c.cursor[ty]
		if idx >= len(runs) {
			continue
		}
		run := runs[idx]
		if c.shouldIgnoreJobTypeRun(ty, run) {
			continue
		}
		if len(run.Attempts) == 0 {
			continue
		}
		t := run.Attempts[0].CreatedAt
		if best.IsZero() || (!t.IsZero() && t.Before(best)) {
			best = t
		}
	}
	if best.IsZero() {
		return time.Time{}, false
	}
	return best, true
}

func (c *StoryBuildingContext) DoTask(_ swf.RunPolicy, taskType string, _ swf.TaskData) (swf.TaskData, error) {
	// Enforce global ordinal ordering during replay. This prevents skipping over restart-injected
	// chapters (e.g. context patches) whose task type does not match the expected recipe task.
	if nextTy, nextRun, ok := c.peekNextRunAny(); ok {
		if nextTy != taskType {
			ord := int64(-1)
			if o, ok := taskRunFirstOrdinal(nextRun); ok {
				ord = o
			}
			// When a job is still running, SWF can surface "runtime" task runs/attempts in non-terminal
			// states that do not necessarily align to the next replayable recipe task type. In that
			// scenario, prefer surfacing "in progress" rather than a deterministic mismatch.
			if isJobStatusRunning(c.jobStatus) && isTaskRunInProgress(nextRun) {
				return nil, fmt.Errorf("%w: next task is %q (ordinal=%d) but requested %q", ErrReplayInProgress, nextTy, ord, taskType)
			}
			return nil, fmt.Errorf("%w: next task is %q (ordinal=%d) but requested %q", ErrReplayMismatch, nextTy, ord, taskType)
		}
	}

	c.mu.Lock()
	runs := c.runsByTy[taskType]
	idx := c.cursor[taskType]
	if idx >= len(runs) {
		c.mu.Unlock()
		// When a job is still running, "unheld"/capability-based tasks can be waiting even though
		// the engine has not yet recorded a task attempt/run for the next step. Treat that as
		// in-progress rather than a deterministic mismatch.
		if isJobStatusRunning(c.jobStatus) {
			return nil, fmt.Errorf("%w: taskType=%q not started (no task runs)", ErrReplayInProgress, taskType)
		}
		return nil, fmt.Errorf("%w: no remaining task runs for taskType=%q", ErrReplayMismatch, taskType)
	}
	run := runs[idx]
	c.cursor[taskType] = idx + 1
	fn := c.onConsume
	c.mu.Unlock()

	if len(run.Attempts) == 0 {
		// A TaskRun can exist before its first attempt is created. Treat it as "in progress"
		// so the story surfaces the node as pending/running rather than failed.
		return nil, fmt.Errorf("%w: taskType=%q not started (no attempts)", ErrReplayInProgress, taskType)
	}
	final := run.Attempts[len(run.Attempts)-1]
	if fn != nil {
		fn(taskType, run, final)
	}

	// Non-terminal runtime attempts indicate the job is still in progress.
	switch final.State {
	case swf.TaskAttemptStateReady, swf.TaskAttemptStateLeased, swf.TaskAttemptStateWaiting, swf.TaskAttemptStateRunning:
		return nil, fmt.Errorf("%w: taskType=%q not complete (state=%s)", ErrReplayInProgress, taskType, final.State)
	}

	if final.Outcome.Status == swf.TaskOutcomeStatusFailed || final.State == swf.TaskAttemptStateFailed {
		msg := "task failed"
		if final.Outcome.Error != nil && strings.TrimSpace(final.Outcome.Error.Message) != "" {
			msg = final.Outcome.Error.Message
		}
		return nil, fmt.Errorf("story replay: taskType=%q failed: %s", taskType, msg)
	}

	var outData []byte
	if final.Output != nil && len(final.Output.Data) > 0 {
		outData = final.Output.Data
	} else {
		outData = []byte("null")
	}

	artifacts := make([]swf.Artifact, 0)
	if final.Output != nil {
		artifacts = make([]swf.Artifact, 0, len(final.Output.Artifacts))
		for _, a := range final.Output.Artifacts {
			key := swf.ArtifactKey{
				JobId:       c.jobKey.JobId,
				TaskOrdinal: final.Ordinal,
				Name:        a.Name,
				SizeBytes:   a.SizeBytes,
			}
			artifacts = append(artifacts, key.ToLazyArtifact(c.engine, c.tenantID))
		}
	}

	// Preserve raw bytes since the downstream decoder expects the original task output envelope.
	return &swf.SimpleTaskData{Data: json.RawMessage(outData), Artifacts: artifacts}, nil
}

// ConsumeNextAny consumes the next unconsumed TaskRun across all task types by earliest ordinal.
// This is used by the story executor to "consume" restart-injected context patch chapters before
// continuing with normal op execution.
func (c *StoryBuildingContext) ConsumeNextAny() (string, swf.TaskData, error) {
	c.mu.Lock()
	taskType, run, ok := c.peekNextRunAnyLocked()
	if !ok {
		c.mu.Unlock()
		if isJobStatusRunning(c.jobStatus) {
			return "", nil, fmt.Errorf("%w: no remaining task runs", ErrReplayInProgress)
		}
		return "", nil, fmt.Errorf("%w: no remaining task runs", ErrReplayMismatch)
	}
	idx := c.cursor[taskType]
	c.cursor[taskType] = idx + 1
	fn := c.onConsume
	c.mu.Unlock()

	if len(run.Attempts) == 0 {
		return "", nil, fmt.Errorf("%w: taskType=%q not started (no attempts)", ErrReplayInProgress, taskType)
	}
	final := run.Attempts[len(run.Attempts)-1]
	if fn != nil {
		fn(taskType, run, final)
	}

	// Non-terminal runtime attempts indicate the job is still in progress.
	switch final.State {
	case swf.TaskAttemptStateReady, swf.TaskAttemptStateLeased, swf.TaskAttemptStateWaiting, swf.TaskAttemptStateRunning:
		return "", nil, fmt.Errorf("%w: taskType=%q not complete (state=%s)", ErrReplayInProgress, taskType, final.State)
	}

	if final.Outcome.Status == swf.TaskOutcomeStatusFailed || final.State == swf.TaskAttemptStateFailed {
		msg := "task failed"
		if final.Outcome.Error != nil && strings.TrimSpace(final.Outcome.Error.Message) != "" {
			msg = final.Outcome.Error.Message
		}
		return "", nil, fmt.Errorf("story replay: taskType=%q failed: %s", taskType, msg)
	}

	var outData []byte
	if final.Output != nil && len(final.Output.Data) > 0 {
		outData = final.Output.Data
	} else {
		outData = []byte("null")
	}

	artifacts := make([]swf.Artifact, 0)
	if final.Output != nil {
		artifacts = make([]swf.Artifact, 0, len(final.Output.Artifacts))
		for _, a := range final.Output.Artifacts {
			key := swf.ArtifactKey{
				JobId:       c.jobKey.JobId,
				TaskOrdinal: final.Ordinal,
				Name:        a.Name,
				SizeBytes:   a.SizeBytes,
			}
			artifacts = append(artifacts, key.ToLazyArtifact(c.engine, c.tenantID))
		}
	}

	return taskType, &swf.SimpleTaskData{Data: json.RawMessage(outData), Artifacts: artifacts}, nil
}

func isJobStatusRunning(st swf.JobStatus) bool {
	switch st {
	case swf.JobStatusActive, swf.JobStatusPendingJobs, swf.JobStatusAwaitingFuture, swf.JobStatusReady:
		return true
	default:
		return false
	}
}

func isTaskAttemptInProgress(att swf.TaskAttempt) bool {
	switch att.State {
	case swf.TaskAttemptStateReady, swf.TaskAttemptStateLeased, swf.TaskAttemptStateWaiting, swf.TaskAttemptStateRunning:
		return true
	default:
		return false
	}
}

func isTaskRunInProgress(run swf.TaskRun) bool {
	if len(run.Attempts) == 0 {
		return true
	}
	final := run.Attempts[len(run.Attempts)-1]
	return isTaskAttemptInProgress(final)
}

func taskRunFirstOrdinal(run swf.TaskRun) (int64, bool) {
	if len(run.Attempts) > 0 {
		return run.Attempts[0].Ordinal, true
	}
	taskRunID := strings.TrimSpace(run.TaskRunID)
	if taskRunID == "" {
		return 0, false
	}
	taskType := strings.TrimSpace(run.TaskType)
	if taskType != "" {
		prefix := taskType + ":"
		if strings.HasPrefix(taskRunID, prefix) {
			ord, err := strconv.ParseInt(strings.TrimPrefix(taskRunID, prefix), 10, 64)
			return ord, err == nil
		}
	}
	last := taskRunID
	if idx := strings.LastIndex(taskRunID, ":"); idx >= 0 && idx < len(taskRunID)-1 {
		last = taskRunID[idx+1:]
	}
	ord, err := strconv.ParseInt(last, 10, 64)
	return ord, err == nil
}

var _ swf.JobContext = (*StoryBuildingContext)(nil)
