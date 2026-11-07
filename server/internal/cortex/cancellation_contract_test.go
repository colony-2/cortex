package cortex

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/celltest"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/sqlite"
)

// Verify the pinned runtime contract using persisted reads, including reopening SQLite.
func TestCancellationArchiveContract(t *testing.T) {
	for _, leased := range []bool{false, true} {
		t.Run(map[bool]string{false: "queued", true: "leased"}[leased], func(t *testing.T) {
			ctx := celltest.Context(context.Background())
			cfg := sqlite.Config{DBPath: filepath.Join(t.TempDir(), "jobs.db")}
			rt, err := sqlite.NewFromConfig(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			key := jobdb.JobKey{TenantId: "tenant", JobId: "job"}
			_, err = celltest.Submit(rt, ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: key.TenantId, JobID: key.JobId, Cell: "test", CellRepo: "example.test/test", JobType: "job", Data: jobdb.NewTaskDataOrPanic(nil)}})
			if err != nil {
				t.Fatal(err)
			}
			if leased {
				lease, err := rt.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: key, Routes: []jobdb.Route{{JobType: "job"}}, WorkerID: "worker", LeaseDuration: time.Hour})
				if err != nil || lease == nil {
					t.Fatalf("lease: %v, %v", lease, err)
				}
			}
			const reason = "Operator 🧪\nretained reason"
			if err := rt.CancelJob(ctx, jobdb.CancelJobRequest{JobKey: key, Reason: reason}); err != nil {
				t.Fatal(err)
			}
			if err := rt.CancelJob(ctx, jobdb.CancelJobRequest{JobKey: key, Reason: "replacement"}); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if _, err := rt.DeleteJobs(ctx, jobdb.DeleteJobsRequest{TenantID: key.TenantId, JobIDs: []string{key.JobId}}); err != nil {
					t.Fatal(err)
				}
			}
			if err := rt.Close(ctx); err != nil {
				t.Fatal(err)
			}
			rt, err = sqlite.NewFromConfig(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer rt.Close(ctx)
			run, err := rt.ListJobs(ctx, jobdb.ListJobsRequest{TenantIds: []string{key.TenantId}, JobKeys: []jobdb.JobKey{key}, IncludeHidden: true})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := rt.GetChapter(ctx, jobdb.ChapterRef{JobKey: key, Ordinal: 0}); err != nil {
				t.Fatalf("archived history lost: %v", err)
			}
			if run.Jobs[0].CompletionDetail != reason {
				t.Fatalf("persisted reason = %q, want %q", run.Jobs[0].CompletionDetail, reason)
			}
		})
	}
}

// A completion winning the confirmation race must retain its terminal outcome.
func TestCancellationAfterCompletionContract(t *testing.T) {
	ctx := celltest.Context(context.Background())
	rt, err := sqlite.NewFromConfig(ctx, sqlite.Config{DBPath: filepath.Join(t.TempDir(), "jobs.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)
	key := jobdb.JobKey{TenantId: "tenant", JobId: "finished"}
	_, err = celltest.Submit(rt, ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: key.TenantId, JobID: key.JobId, Cell: "test", CellRepo: "example.test/test", JobType: "job", Data: jobdb.NewTaskDataOrPanic(nil)}})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := rt.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: key, Routes: []jobdb.Route{{JobType: "job"}}, WorkerID: "worker", LeaseDuration: time.Hour})
	if err != nil || lease == nil {
		t.Fatalf("lease: %v", err)
	}
	if err := lease.Complete(ctx, jobdb.CompleteExecutionRequest{Status: "success", Detail: "original outcome", Chapter: &jobdb.Chapter{Ordinal: 1, TaskType: "job", CreatedAt: time.Now(), Body: jobdb.JobAttemptOutcomeChapter{Outcome: jobdb.ApplicationOutputOutcome{Output: jobdb.ApplicationOutputBytes{Data: []byte(`{}`)}}}}}); err != nil {
		t.Fatal(err)
	}
	if err := rt.CancelJob(ctx, jobdb.CancelJobRequest{JobKey: key, Reason: "late cancellation"}); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.DeleteJobs(ctx, jobdb.DeleteJobsRequest{TenantID: key.TenantId, JobIDs: []string{key.JobId}}); err != nil {
		t.Fatal(err)
	}
	listed, err := rt.ListJobs(ctx, jobdb.ListJobsRequest{TenantIds: []string{key.TenantId}, JobKeys: []jobdb.JobKey{key}, IncludeHidden: true})
	if err != nil {
		t.Fatal(err)
	}
	if listed.Jobs[0].CompletionStatus != "success" || listed.Jobs[0].CompletionDetail != "original outcome" {
		t.Fatalf("terminal outcome changed: %#v", listed.Jobs)
	}
}
