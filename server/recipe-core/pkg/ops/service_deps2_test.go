package ops

import (
	"testing"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/runmetadata"
)

func TestServiceDepsCloneMetadata(t *testing.T) {
	builder := NewServiceDepsBuilder()
	base := builder.Build()
	if base == nil {
		t.Fatalf("builder returned nil dependencies")
	}

	if _, ok := base.RunMetadata(); ok {
		t.Fatalf("expected no run metadata on fresh dependencies")
	}
	if _, ok := base.ResumeMetadata(); ok {
		t.Fatalf("expected no resume metadata on fresh dependencies")
	}

	runMarker := &runmetadata.Signal{Reason: "initial"}
	withRun := base.CloneWithRunMetadata(runMarker)
	if withRun == nil {
		t.Fatalf("CloneWithRunMetadata returned nil")
	}

	runValue, ok := withRun.RunMetadata()
	if !ok {
		t.Fatalf("expected run metadata after clone")
	}
	if runValue != runMarker {
		t.Fatalf("run metadata pointer mismatch: %p vs %p", runValue, runMarker)
	}

	if _, ok := base.RunMetadata(); ok {
		t.Fatalf("base dependencies should remain unmodified")
	}

	resumeMarker := &runmetadata.Resume{ExecutionPath: []runmetadata.Segment{{InvocationHash: "child"}}}
	withResume := withRun.CloneWithResumeMetadata(resumeMarker)
	if withResume == nil {
		t.Fatalf("CloneWithResumeMetadata returned nil")
	}

	resumeValue, ok := withResume.ResumeMetadata()
	if !ok {
		t.Fatalf("expected resume metadata after clone")
	}
	if resumeValue != resumeMarker {
		t.Fatalf("resume metadata pointer mismatch: %p vs %p", resumeValue, resumeMarker)
	}

	// Ensure the intermediate clone retained run metadata.
	retainedRun, ok := withResume.RunMetadata()
	if !ok {
		t.Fatalf("expected run metadata to persist after resume clone")
	}
	if retainedRun != runMarker {
		t.Fatalf("run metadata pointer mismatch on resume clone")
	}
}
