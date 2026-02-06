package openapi_test

import (
	"testing"
	"time"

	"github.com/colony-2/colony2/server/openapi/pkg/openapi"
)

func TestWorkflowSummaryTypesGenerated(t *testing.T) {
	now := time.Now()
	summary := openapi.WorkflowSummary{
		WorkflowId: "prj_123_abc",
		RunId:      "prj_123_abc",
		Status:     openapi.WorkflowStatusRunning,
		RecipeName: "test_recipe",
		CreatedAt:  now,
	}

	if summary.WorkflowId != "prj_123_abc" {
		t.Fatalf("expected workflow id, got %q", summary.WorkflowId)
	}
	if summary.Status != openapi.WorkflowStatusRunning {
		t.Fatalf("expected status running, got %q", summary.Status)
	}
}

func TestWorkflowDetailTypesGenerated(t *testing.T) {
	now := time.Now()
	detail := openapi.WorkflowDetail{
		WorkflowId: "prj_123_abc",
		RunId:      "prj_123_abc",
		Status:     openapi.WorkflowStatusCompleted,
		RecipeName: "test_recipe",
		Chapters:   []openapi.ChapterDetail{},
		CreatedAt:  now,
	}

	if detail.WorkflowId != "prj_123_abc" {
		t.Fatalf("expected workflow id, got %q", detail.WorkflowId)
	}
	if detail.Status != openapi.WorkflowStatusCompleted {
		t.Fatalf("expected status completed, got %q", detail.Status)
	}
	if detail.Chapters == nil {
		t.Fatal("expected chapters slice to be non-nil")
	}
}

func TestChapterDetailTypesGenerated(t *testing.T) {
	input := map[string]interface{}{"key": "value"}
	artifacts := []openapi.ArtifactReference{}
	chapter := openapi.ChapterDetail{
		ChapterNumber: 1,
		ChapterType:   "op",
		Status:        openapi.ChapterStatusCompleted,
		Input:         &input,
		Artifacts:     &artifacts,
	}

	if chapter.ChapterNumber != 1 {
		t.Fatalf("expected chapter number 1, got %d", chapter.ChapterNumber)
	}
	if chapter.ChapterType != "op" {
		t.Fatalf("expected chapter type op, got %q", chapter.ChapterType)
	}
	if chapter.Status != openapi.ChapterStatusCompleted {
		t.Fatalf("expected status completed, got %q", chapter.Status)
	}
}

func TestWorkflowStatusConstants(t *testing.T) {
	statuses := []openapi.WorkflowStatus{
		openapi.WorkflowStatusRunning,
		openapi.WorkflowStatusCompleted,
		openapi.WorkflowStatusFailed,
		openapi.WorkflowStatusCanceled,
		openapi.WorkflowStatusTerminated,
		openapi.WorkflowStatusTimedOut,
		openapi.WorkflowStatusUnknown,
	}

	if len(statuses) != 7 {
		t.Fatalf("expected 7 workflow statuses, got %d", len(statuses))
	}
}

func TestChapterStatusConstants(t *testing.T) {
	statuses := []openapi.ChapterStatus{
		openapi.ChapterStatusPending,
		openapi.ChapterStatusRunning,
		openapi.ChapterStatusCompleted,
		openapi.ChapterStatusFailed,
		openapi.ChapterStatusSkipped,
	}

	if len(statuses) != 5 {
		t.Fatalf("expected 5 chapter statuses, got %d", len(statuses))
	}
}
