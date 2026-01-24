package recipe

import (
	"context"
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestStartJobsEmpty(t *testing.T) {
	ctl := &fakeWorkflowControl{}
	_, err := startJobs(context.Background(), nil, "tenant", ctl, nil, "")
	if err == nil {
		t.Fatal("expected error for empty recipes")
	}
}

func TestStartJobsSingle(t *testing.T) {
	ctl := &fakeWorkflowControl{}
	recipes := []SingleRecipe{{Name: "child", Inputs: map[string]interface{}{"value": "ok"}}}
	keys, err := startJobs(context.Background(), nil, "tenant", ctl, recipes, "git-ref")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if len(ctl.startRequests) != 1 {
		t.Fatalf("expected 1 StartJob call, got %d", len(ctl.startRequests))
	}
	if ctl.startSawTx[0] {
		t.Fatal("did not expect transaction for single job")
	}
}

func TestStartJobsMultipleUsesTransaction(t *testing.T) {
	ctl := &fakeWorkflowControl{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	recipes := []SingleRecipe{{Name: "child-a"}, {Name: "child-b"}}
	keys, err := startJobs(context.Background(), db, "tenant", ctl, recipes, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	for i, sawTx := range ctl.startSawTx {
		if !sawTx {
			t.Fatalf("expected transaction context for call %d", i)
		}
	}
}

func TestStartJobsMultipleErrorReturnsNoKeys(t *testing.T) {
	ctl := &fakeWorkflowControl{
		startErrs: []error{nil, errors.New("boom")},
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	recipes := []SingleRecipe{{Name: "child-a"}, {Name: "child-b"}}
	keys, err := startJobs(context.Background(), db, "tenant", ctl, recipes, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if keys != nil {
		t.Fatalf("expected nil keys on error, got %v", keys)
	}
}
