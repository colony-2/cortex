package cortex

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/cellref"
	"github.com/colony-2/c2j/pkg/recipejob"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
)

// Fixture selection is a trusted host grant, never a public submission field.
func primaryFixtureContext(ctx context.Context) context.Context {
	return jobdb.WithPrimaryGoalSelectionAuthorizer(testjobdb.Context(ctx), func(context.Context, jobdb.GoalKey, jobdb.ResolvedCell) error { return nil })
}

func submitPrimaryFixture(t *testing.T, s *Server, tenant, id, cell, goal string, inputs map[string]interface{}) recipejob.RecipeJob {
	t.Helper()
	ctx := primaryFixtureContext(context.Background())
	target, err := recipejob.ResolveRegisteredTarget(ctx, recipejob.ResolvedTarget{CellName: cell, TenantID: tenant, DefaultRef: cellref.DefaultRef}, cellref.Registry(s.cells, tenant))
	if err != nil {
		t.Fatalf("fixture %s/%s: %v", tenant, id, err)
	}
	key, err := recipejob.SubmitRecipeJob(ctx, recipejob.BuildStartJobRequest{TenantID: tenant, JobID: id, GoalID: goal, Target: target, Recipe: "smoke", Inputs: inputs}, nativeSubmitter{s.engine})
	if err != nil {
		t.Fatalf("fixture %s/%s: %v", tenant, id, err)
	}
	job, err := recipejob.GetRecipeJob(ctx, s.engine, recipejob.GetRecipeJobRequest{TenantID: tenant, JobID: key.JobID})
	if err != nil {
		t.Fatalf("fixture %s/%s: %v", tenant, id, err)
	}
	return job
}

func completePrimaryFixture(t *testing.T, runtime interface {
	GetJobLease(context.Context, jobdb.GetJobLeaseRequest) (jobdb.ExecutionLease, error)
}, tenant, id, outcome string) {
	t.Helper()
	ctx := primaryFixtureContext(context.Background())
	lease, err := runtime.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: jobdb.JobKey{TenantId: tenant, JobId: id}, WorkerID: "primary-fixture", Routes: []jobdb.Route{{JobType: "recipe"}}, LeaseDuration: time.Minute})
	if err != nil || lease == nil {
		t.Fatalf("fixture lease: %v %v", lease, err)
	}
	defer lease.StopKeepAlive()
	if err := lease.Complete(ctx, jobdb.CompleteExecutionRequest{Status: outcome, Chapter: &jobdb.Chapter{Ordinal: 1, TaskType: "recipe", CreatedAt: time.Now().UTC(), Body: jobdb.JobAttemptOutcomeChapter{Outcome: jobdb.ApplicationOutputOutcome{Output: jobdb.ApplicationOutputBytes{Data: []byte(`{"ok":true}`)}}}}}); err != nil {
		t.Fatalf("fixture %s/%s: %v", tenant, id, err)
	}
}

func primaryFixtureHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(primaryFixtureContext(r.Context())))
	})
}
