package service

import (
	"encoding/json"
	"testing"
	"time"

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
