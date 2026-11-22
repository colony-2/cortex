package cortex

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/colony-2/jobdb/pkg/jobdb/celltest"
	"github.com/stretchr/testify/require"
)

func TestClearCrashConcernRecovery(t *testing.T) {
	for _, mode := range []string{"toy", "sqlite", "remote", "sqlite-remote"} {
		t.Run(mode, func(t *testing.T) {
			runtime := retirementRuntime(t, mode)
			ctx, cancel := context.WithTimeout(testjobdb.Context(context.Background()), 30*time.Second)
			defer cancel()
			key := jobdb.JobKey{TenantId: "tenant", JobId: "flapping"}
			for _, tenant := range []string{"tenant", "other"} {
				_, err := celltest.Submit(runtime, ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: tenant, JobID: key.JobId, Cell: "platform", CellRepo: "github.com/acme/platform", JobType: "recipe", Data: jobdb.NewTaskDataOrPanic(1)}})
				require.NoError(t, err)
			}
			srv := retirementServer(t, runtime, Config{WorkingDir: writeC2JConfig(t)})
			srv.crashConcern = runtime
			api := httptest.NewServer(testjobdb.Handler(srv))
			defer api.Close()
			clear := func(tenant string) int {
				t.Helper()
				req, _ := http.NewRequest("POST", api.URL+"/api/projects/"+tenant+"/jobs/"+key.JobId+"/clear-crash-concern", nil)
				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				resp.Body.Close()
				return resp.StatusCode
			}
			get := func() jobdb.ExecutionLease {
				t.Helper()
				lease, err := runtime.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: key, WorkerID: "crash-test", Routes: []jobdb.Route{{JobType: "recipe"}}, LeaseDuration: 200 * time.Millisecond})
				require.NoError(t, err)
				return lease
			}
			var previous string
			for n := 0; n < 30; n++ {
				info, err := runtime.GetJob(ctx, key)
				require.NoError(t, err)
				if info.Status == jobdb.JobStatusCrashConcern {
					break
				}
				lease := get()
				require.NotNil(t, lease)
				lease.StopKeepAlive()
				previous = lease.LeaseID()
				if n == 0 {
					require.Equal(t, 409, clear("tenant"))
					token := ""
					if l, ok := lease.(interface{ LeaseToken() string }); ok {
						token = l.LeaseToken()
					}
					require.NoError(t, runtime.PutChapter(ctx, jobdb.PutChapterRequest{LeaseID: lease.LeaseID(), LeaseToken: token, Ref: jobdb.ChapterRef{JobKey: key, Ordinal: 1}, Chapter: jobdb.Chapter{Ordinal: 1, TaskType: "__execution__", CreatedAt: time.Now().UTC(), Body: jobdb.ExecutionChapter{Record: []byte(fmt.Sprintf(`{"version":%d,"kind":"header","versions":{"execution_pattern_version":%d,"jobdb_version":"test"}}`, jobdb.ExecutionPatternVersion, jobdb.ExecutionPatternVersion))}}}))
				}
				rows, err := runtime.ListJobs(ctx, jobdb.ListJobsRequest{TenantIds: []string{key.TenantId}, JobKeys: []jobdb.JobKey{key}})
				require.NoError(t, err)
				require.Len(t, rows.Jobs, 1)
				var expiry time.Time
				if renewable, ok := lease.(jobdb.RenewableExecutionLease); ok {
					expiry = renewable.LeaseExpiry()
				}
				if stored := rows.Jobs[0].LeaseExpiresAt; stored != nil && stored.After(expiry) {
					expiry = *stored
				}
				// Remote capabilities expire before the stored lease. Toy's list
				// omits that timestamp, so use the retained acquisition event.
				recorded, err := runtime.ListLeaseEvents(ctx, jobdb.ListLeaseEventsRequest{TenantID: key.TenantId, JobIDs: []string{key.JobId}, PageSize: 100})
				require.NoError(t, err)
				for _, event := range recorded.Events {
					if event.LeaseID == lease.LeaseID() && event.LeaseExpiresAt.After(expiry) {
						expiry = event.LeaseExpiresAt
					}
				}
				require.False(t, expiry.IsZero())
				delay := time.Until(expiry) + 15*time.Millisecond
				select {
				case <-time.After(delay):
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			info, err := runtime.GetJob(ctx, key)
			require.NoError(t, err)
			require.Equal(t, jobdb.JobStatusCrashConcern, info.Status)
			require.Nil(t, get())
			leases, err := runtime.PollWork(ctx, jobdb.PollWorkRequest{TenantId: key.TenantId, WorkerID: "poller", Routes: []jobdb.Route{{JobType: "recipe"}}, LeaseDuration: time.Second})
			require.NoError(t, err)
			require.Empty(t, leases)
			chapters, err := runtime.ListChapters(ctx, jobdb.ListChaptersRequest{JobKey: key})
			require.NoError(t, err)
			require.NotEmpty(t, chapters)
			events, err := runtime.ListLeaseEvents(ctx, jobdb.ListLeaseEventsRequest{TenantID: key.TenantId, JobIDs: []string{key.JobId}})
			require.NoError(t, err)
			require.NotEmpty(t, events.Events)
			require.Equal(t, 204, clear("other"))
			info, err = runtime.GetJob(ctx, key)
			require.NoError(t, err)
			require.Equal(t, jobdb.JobStatusCrashConcern, info.Status)
			require.Equal(t, 204, clear("tenant"))
			require.Equal(t, 204, clear("tenant"))
			after, err := runtime.ListChapters(ctx, jobdb.ListChaptersRequest{JobKey: key})
			require.NoError(t, err)
			require.True(t, reflect.DeepEqual(chapters, after), "history changed")
			afterEvents, err := runtime.ListLeaseEvents(ctx, jobdb.ListLeaseEventsRequest{TenantID: key.TenantId, JobIDs: []string{key.JobId}})
			require.NoError(t, err)
			require.Equal(t, events, afterEvents)
			recovered := get()
			require.NotNil(t, recovered)
			defer recovered.StopKeepAlive()
			require.Equal(t, key, recovered.Job().JobKey)
			require.NotEqual(t, previous, recovered.LeaseID())
			require.Equal(t, 409, clear("tenant"))
			require.NoError(t, runtime.CancelJob(ctx, jobdb.CancelJobRequest{JobKey: jobdb.JobKey{TenantId: "other", JobId: key.JobId}}))
			require.Equal(t, 409, clear("other"))
		})
	}
}

func TestClearCrashConcernScheduling(t *testing.T) {
	for _, mode := range []string{"toy", "sqlite", "remote", "sqlite-remote"} {
		t.Run(mode, func(t *testing.T) {
			runtime := retirementRuntime(t, mode)
			ctx := testjobdb.Context(context.Background())
			srv := retirementServer(t, runtime, Config{WorkingDir: writeC2JConfig(t)})
			srv.crashConcern = runtime
			api := httptest.NewServer(testjobdb.Handler(srv))
			defer api.Close()
			post := func(id string) int {
				t.Helper()
				resp, err := http.Post(api.URL+"/api/projects/tenant/jobs/"+id+"/clear-crash-concern", "application/json", nil)
				require.NoError(t, err)
				resp.Body.Close()
				return resp.StatusCode
			}
			submit := func(id string, at *time.Time) jobdb.JobKey {
				t.Helper()
				h, err := celltest.Submit(runtime, ctx, jobdb.SubmitJobRequest{Job: jobdb.SubmitJob{TenantId: "tenant", JobID: id, Cell: "platform", CellRepo: "github.com/acme/platform", JobType: "recipe", Data: jobdb.NewTaskDataOrPanic(1), AvailableAt: at}})
				require.NoError(t, err)
				return h.JobKey
			}
			acquire := func(key jobdb.JobKey) jobdb.ExecutionLease {
				t.Helper()
				lease, err := runtime.GetJobLease(ctx, jobdb.GetJobLeaseRequest{JobKey: key, WorkerID: "scheduler-test", Routes: []jobdb.Route{{JobType: "recipe"}}, LeaseDuration: time.Minute})
				require.NoError(t, err)
				return lease
			}
			later := time.Now().UTC().Add(time.Hour)
			future := submit("future", &later)
			wait := submit("waiting", nil)
			dependency := submit("dependency", nil)
			lease := acquire(wait)
			require.NotNil(t, lease)
			defer lease.StopKeepAlive()
			require.NoError(t, lease.Reschedule(ctx, jobdb.RescheduleExecutionRequest{NextRoute: jobdb.Route{JobType: "recipe"}, WaitForJobIDs: []string{dependency.JobId}, WaitUntil: &later}))
			for _, key := range []jobdb.JobKey{future, wait} {
				before, err := runtime.GetJob(ctx, key)
				require.NoError(t, err)
				rows, err := runtime.ListJobs(ctx, jobdb.ListJobsRequest{TenantIds: []string{"tenant"}, JobKeys: []jobdb.JobKey{key}})
				require.NoError(t, err)
				require.Equal(t, 204, post(key.JobId))
				require.Equal(t, 204, post(key.JobId))
				after, err := runtime.GetJob(ctx, key)
				require.NoError(t, err)
				require.Equal(t, before, after)
				afterRows, err := runtime.ListJobs(ctx, jobdb.ListJobsRequest{TenantIds: []string{"tenant"}, JobKeys: []jobdb.JobKey{key}})
				require.NoError(t, err)
				require.Equal(t, rows, afterRows)
				require.Nil(t, acquire(key))
			}
			completed := submit("completed", nil)
			completedLease := acquire(completed)
			require.NotNil(t, completedLease)
			defer completedLease.StopKeepAlive()
			require.NoError(t, completedLease.Complete(ctx, jobdb.CompleteExecutionRequest{Status: "success", Chapter: &jobdb.Chapter{Ordinal: 1, TaskType: "recipe", CreatedAt: time.Now().UTC(), Body: jobdb.JobAttemptOutcomeChapter{Outcome: jobdb.ApplicationOutputOutcome{Output: jobdb.ApplicationOutputBytes{Data: []byte(`1`)}}}}}))
			require.Equal(t, 409, post(completed.JobId))
			hidden := submit("hidden", nil)
			_, err := runtime.(jobdb.JobDeletionRuntime).DeleteJobs(ctx, jobdb.DeleteJobsRequest{TenantID: "tenant", JobIDs: []string{hidden.JobId}})
			require.NoError(t, err)
			require.Equal(t, 404, post(hidden.JobId))
		})
	}
}
