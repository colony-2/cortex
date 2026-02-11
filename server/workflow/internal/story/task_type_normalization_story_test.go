package story

import (
	"errors"
	"testing"
	"time"

	"github.com/colony-2/swf-go/pkg/swf"
)

func TestStoryBuildingContext_NormalizesJobTypePrefixedTaskTypes(t *testing.T) {
	tasks := []swf.TaskRun{
		{
			TaskRunID: "recipe:input:collect_user_input:6",
			TaskType:  "recipe:input:collect_user_input",
			Attempts: []swf.TaskAttempt{
				{
					Ordinal:   6,
					Attempt:   1,
					WorkerID:  "w1",
					CreatedAt: time.Date(2026, 2, 8, 0, 0, 0, 0, time.UTC),
					State:     swf.TaskAttemptStateReady,
					Outcome:   swf.TaskOutcome{},
				},
			},
		},
	}

	attempts := []swf.JobAttempt{{Attempt: 1, Tasks: tasks}}
	jobCtx := NewStoryBuildingContext(nil, "tenant", swf.JobKey{TenantId: "tenant", JobId: "job"}, "recipe", swf.JobStatusActive, attempts, nil)

	if ty, ok := jobCtx.NextTaskTypeForPrefix("input:"); !ok || ty != "input:collect_user_input" {
		t.Fatalf("expected NextTaskTypeForPrefix to return normalized task type, got ty=%q ok=%v", ty, ok)
	}

	_, err := jobCtx.DoTask(swf.RunPolicy{}, "input:collect_user_input", nil)
	if err == nil || !errors.Is(err, ErrReplayInProgress) {
		t.Fatalf("expected DoTask to return ErrReplayInProgress, got %v", err)
	}
}
