package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/recipe-core/pkg/starter"
	"github.com/colony-2/colony2/server/workflow/internal/model"
	"github.com/colony-2/strata-go/pkg/client/story"
	"github.com/colony-2/swf-go/pkg/swf"
)

func TestMapWorkflowStatus(t *testing.T) {
	if got := mapWorkflowStatus(swf.JobStatusActive); got != "running" {
		t.Fatalf("expected running, got %q", got)
	}
	if got := mapWorkflowStatus(swf.JobStatusCompleted); got != "completed" {
		t.Fatalf("expected completed, got %q", got)
	}
	if got := mapWorkflowStatus(swf.JobStatusExpired); got != "timed_out" {
		t.Fatalf("expected timed_out, got %q", got)
	}
	if got := mapWorkflowStatus(swf.JobStatusCrashConcern); got != "failed" {
		t.Fatalf("expected failed, got %q", got)
	}
	if got := mapWorkflowStatus(swf.JobStatus("UNKNOWN")); got != "unknown" {
		t.Fatalf("expected unknown, got %q", got)
	}
}

func TestChapterToDetailInputOutputAndError(t *testing.T) {
	startedAt := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	makeChapter := func(ordinal int64, payloadKind string, payload map[string]interface{}, taskType string) story.Chapter {
		env := chapterEnvelope{
			Meta: chapterMeta{
				Ordinal:   ordinal,
				TaskType:  taskType,
				CreatedAt: startedAt,
			},
			PayloadKind: payloadKind,
		}
		if payload != nil {
			raw, _ := json.Marshal(payload)
			env.Payload = raw
		}
		rawEnv, _ := json.Marshal(env)
		return story.NewChapter().WithBytes(rawEnv).WithOrdinal(ordinal)
	}

	chapter0 := makeChapter(0, "App", map[string]interface{}{"input": "value"}, "op")
	detail0, err := chapterToDetail(chapter0)
	if err != nil {
		t.Fatalf("chapterToDetail input: %v", err)
	}
	if detail0.Input["input"] != "value" {
		t.Fatalf("expected input payload, got %#v", detail0.Input)
	}
	if detail0.Output != nil {
		t.Fatalf("expected output nil for ordinal 0, got %#v", detail0.Output)
	}

	chapter1 := makeChapter(1, "App", map[string]interface{}{"output": "value"}, "task")
	detail1, err := chapterToDetail(chapter1)
	if err != nil {
		t.Fatalf("chapterToDetail output: %v", err)
	}
	if detail1.Output == nil || (*detail1.Output)["output"] != "value" {
		t.Fatalf("expected output payload, got %#v", detail1.Output)
	}
	if detail1.Input == nil || len(detail1.Input) != 0 {
		t.Fatalf("expected empty input for ordinal > 0, got %#v", detail1.Input)
	}

	chapterErr := makeChapter(2, "AppError", map[string]interface{}{"message": "boom"}, "")
	detailErr, err := chapterToDetail(chapterErr)
	if err != nil {
		t.Fatalf("chapterToDetail error: %v", err)
	}
	if detailErr.Error == nil || *detailErr.Error != "boom" {
		t.Fatalf("expected error message boom, got %#v", detailErr.Error)
	}
	if detailErr.ChapterType != "workflow" {
		t.Fatalf("expected default chapter type workflow, got %q", detailErr.ChapterType)
	}
}

func TestWorkflowStatusesToJobStatuses_NilReturnsNil(t *testing.T) {
	// Test with nil statuses - should return nil to indicate "all statuses"
	// The SWF engine will interpret nil as "no filter"
	result := workflowStatusesToJobStatuses(nil)

	if result != nil {
		t.Fatalf("expected nil when input is nil, got %v (len=%d)", result, len(result))
	}
}

func TestWorkflowStatusesToJobStatuses_EmptyReturnsNil(t *testing.T) {
	// Test with empty slice - should return nil to indicate "all statuses"
	// The SWF engine will interpret nil as "no filter"
	result := workflowStatusesToJobStatuses([]model.WorkflowStatus{})

	if result != nil {
		t.Fatalf("expected nil when input is empty, got %v (len=%d)", result, len(result))
	}
}

func TestWorkflowStatusesToJobStatuses_SpecificStatus(t *testing.T) {
	// Test with specific status - should only return those job statuses
	result := workflowStatusesToJobStatuses([]model.WorkflowStatus{model.WorkflowStatusCompleted})

	if len(result) != 1 {
		t.Fatalf("expected 1 job status for 'completed', got %d", len(result))
	}

	if result[0] != swf.JobStatusCompleted {
		t.Errorf("expected JobStatusCompleted, got %q", result[0])
	}
}

func TestBuildSummary_UsesJobMetadata(t *testing.T) {
	created := time.Date(2026, 2, 5, 12, 1, 0, 0, time.UTC)

	meta := starter.JobMetadata{
		Version:    starter.JobMetadataVersion,
		RecipeName: "recipe-1",
		TicketID:   "T-1",
		CellID:     "cell-1",
		CellName:   "alpha",
		ActorEmail: "user@example.com",
		GitRef:     "main",
	}
	metaRaw, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}

	svc := &Service{logger: slog.Default()}
	job := swf.JobSummary{
		JobKey:    swf.JobKey{TenantId: "proj-1", JobId: "job-1"},
		Status:    swf.JobStatusActive,
		CreatedAt: created,
		Metadata:  metaRaw,
	}

	summary, ok, err := svc.buildSummary(context.Background(), "proj-1", job)
	if err != nil {
		t.Fatalf("buildSummary error: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if summary.RecipeName != "recipe-1" {
		t.Fatalf("expected recipe_name recipe-1, got %q", summary.RecipeName)
	}
	if summary.TicketID == nil || *summary.TicketID != "T-1" {
		t.Fatalf("expected ticket_id T-1, got %#v", summary.TicketID)
	}
	if summary.CellID == nil || *summary.CellID != "cell-1" {
		t.Fatalf("expected cell_id cell-1, got %#v", summary.CellID)
	}
	if summary.CellName == nil || *summary.CellName != "alpha" {
		t.Fatalf("expected cell_name alpha, got %#v", summary.CellName)
	}
	if summary.SubmittedAt == nil || !summary.SubmittedAt.Equal(created) {
		t.Fatalf("expected submitted_at %v, got %#v", created, summary.SubmittedAt)
	}
	if summary.Actor.User == nil || summary.Actor.User.Email != "user@example.com" {
		t.Fatalf("expected actor email user@example.com, got %#v", summary.Actor)
	}
}

func TestJobRunStory_TerminalTimeout_OverridesStatusAndSuppressesMismatch(t *testing.T) {
	run := swf.GetJobRunResponse{
		Job: swf.JobRunSummary{Status: swf.JobStatusCompleted},
		Attempts: []swf.JobAttempt{
			{
				Attempt: 1,
				Outcome: swf.TaskOutcome{
					Status:      swf.TaskOutcomeStatusFailed,
					PayloadKind: "Timeout",
					Error: &swf.TaskError{
						Kind:    "TIMEOUT",
						Code:    "timeout_total",
						Message: "job total timed out after 30m0s",
					},
				},
			},
		},
	}

	status, ok := terminalStoryStatusOverrideFromRun(run)
	if !ok {
		t.Fatalf("expected override ok=true")
	}
	if status != model.WorkflowStatusTimedOut {
		t.Fatalf("expected timed_out, got %q", status)
	}
	if !shouldSuppressReplayMismatchForRun(run) {
		t.Fatalf("expected mismatch suppression for terminal timeout")
	}

	msg, code, ok := latestAttemptTerminalError(run)
	if !ok {
		t.Fatalf("expected terminal error present")
	}
	if msg != "job total timed out after 30m0s" {
		t.Fatalf("unexpected msg %q", msg)
	}
	if code != "timeout_total" {
		t.Fatalf("unexpected code %q", code)
	}
}

func TestJobRunStory_NonTimeoutTerminal_DoesNotSuppressMismatch(t *testing.T) {
	run := swf.GetJobRunResponse{
		Job: swf.JobRunSummary{Status: swf.JobStatusCompleted},
		Attempts: []swf.JobAttempt{
			{
				Attempt: 1,
				Outcome: swf.TaskOutcome{
					Status:      swf.TaskOutcomeStatusFailed,
					PayloadKind: "AppError",
					Error: &swf.TaskError{
						Kind:    "APP",
						Code:    "boom",
						Message: "boom",
					},
				},
			},
		},
	}

	if _, ok := terminalStoryStatusOverrideFromRun(run); ok {
		t.Fatalf("expected no override")
	}
	if shouldSuppressReplayMismatchForRun(run) {
		t.Fatalf("expected no mismatch suppression")
	}
}

func TestJobRunStory_AttemptOutput_IsShownOnRecipeAttemptNodes(t *testing.T) {
	run := swf.GetJobRunResponse{
		Attempts: []swf.JobAttempt{
			{
				Attempt: 1,
				Output:  &swf.TaskIO{Data: json.RawMessage(`{"message":"attempt1"}`)},
				Outcome: swf.TaskOutcome{Error: &swf.TaskError{Message: "attempt1 err", Code: "e1"}},
			},
			{
				Attempt: 2,
				Output:  &swf.TaskIO{Data: json.RawMessage(`{"message":"attempt2"}`)},
				Outcome: swf.TaskOutcome{Error: &swf.TaskError{Message: "attempt2 err", Code: "e2"}},
			},
		},
	}

	st := &model.JobRunStory{
		Root: &model.JobRunStoryNode{
			JobAttempt: 2,
			Output:     map[string]any{"old": true},
			PastAttempts: []*model.JobRunStoryNode{
				{JobAttempt: 1, Output: map[string]any{"old": true}},
			},
		},
	}

	applyJobAttemptOutcomeAndOutputToStory(st, run)

	rootOut, ok := st.Root.Output.(map[string]any)
	if !ok || rootOut["message"] != "attempt2" {
		t.Fatalf("expected root output attempt2, got %#v", st.Root.Output)
	}
	if st.Root.Error == nil || st.Root.Error.Message != "attempt2 err" || st.Root.Error.Code != "e2" {
		t.Fatalf("expected root error from attempt2, got %#v", st.Root.Error)
	}

	pa := st.Root.PastAttempts[0]
	paOut, ok := pa.Output.(map[string]any)
	if !ok || paOut["message"] != "attempt1" {
		t.Fatalf("expected past attempt output attempt1, got %#v", pa.Output)
	}
	if pa.Error == nil || pa.Error.Message != "attempt1 err" || pa.Error.Code != "e1" {
		t.Fatalf("expected past attempt error from attempt1, got %#v", pa.Error)
	}
}
