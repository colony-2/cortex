package cortex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

type timingEngine struct {
	jobworkflow.Engine
	mu       sync.Mutex
	revision int64
	removed  bool
	calls    int
	failCall int
}

func (e *timingEngine) ListJobs(_ context.Context, req jobdb.ListJobsRequest) (jobdb.ListJobsResponse, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	if e.calls == e.failCall {
		return jobdb.ListJobsResponse{}, errors.New("verification unavailable")
	}
	if e.removed {
		return jobdb.ListJobsResponse{}, nil
	}
	rows := compactJobs(1)
	rows[0].JobKey.TenantId = req.TenantIds[0]
	rows[0].ClientPayloadRevision = e.revision
	return jobdb.ListJobsResponse{Jobs: rows}, nil
}
func (e *timingEngine) change(revision int64, removed bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.revision, e.removed = revision, removed
}

type timingRuntime struct {
	reviewRuntimeStub
	mu         sync.Mutex
	calls      int
	tenants    []string
	fail       bool
	hook       func()
	emptyPage  bool
	occurrence input.PendingInputOccurrence
}

func (r *timingRuntime) ListPendingInputsPage(ctx context.Context, tenant string, options input.PendingInputOptions) (input.PendingInputPage, error) {
	r.mu.Lock()
	r.calls++
	r.tenants = append(r.tenants, tenant)
	r.mu.Unlock()
	if r.hook != nil {
		r.hook()
	}
	if ctx.Err() != nil {
		return input.PendingInputPage{}, ctx.Err()
	}
	if r.fail {
		return input.PendingInputPage{}, errors.New("timing unavailable")
	}
	if r.emptyPage && options.PageToken == "" {
		return input.PendingInputPage{NextPageToken: "next"}, nil
	}
	if r.occurrence.JobID != "" {
		return input.PendingInputPage{Inputs: []input.PendingInputOccurrence{r.occurrence}}, nil
	}
	return input.PendingInputPage{Inputs: []input.PendingInputOccurrence{{JobID: "0", TaskOrdinal: 12, RequestID: tenant + "-request", RequestedAt: "2026-11-01T01:02:03Z"}}}, nil
}
func timingContext() context.Context { return testjobdb.Context(context.Background()) }
func TestPendingTimingCachePagingRetryReplacementAndEviction(t *testing.T) {
	e := &timingEngine{revision: 1}
	r := &timingRuntime{emptyPage: true}
	s := &Server{engine: e, inputs: r}
	rows, err := s.timedPendingInputs(timingContext(), "a")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "2026-11-01T01:02:03Z", rows[0].RequestedAt)
	require.Equal(t, "a-request", rows[0].RequestID)
	require.EqualValues(t, 12, *rows[0].TaskOrdinal)
	require.Equal(t, 2, r.calls)
	for range 5 {
		_, err = s.timedPendingInputs(timingContext(), "a")
		require.NoError(t, err)
	}
	require.Equal(t, 2, r.calls, "unchanged polls must not inspect forms")
	_, err = s.timedPendingInputs(timingContext(), "b")
	require.NoError(t, err)
	require.Equal(t, 4, r.calls)
	require.Equal(t, "a-request", s.timing.tenants["a"].entries["0"].occurrence.RequestID)
	e.change(2, false)
	r.occurrence = input.PendingInputOccurrence{JobID: "0", TaskOrdinal: 13, RequestID: "successor", RequestedAt: "2026-11-09T01:02:03Z"}
	r.fail = true
	rows, err = s.timedPendingInputs(timingContext(), "a")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Empty(t, rows[0].RequestedAt)
	require.Empty(t, rows[0].RequestID)
	require.Equal(t, 5, r.calls)
	for range 5 {
		_, err = s.timedPendingInputs(timingContext(), "a")
		require.NoError(t, err)
	}
	require.Equal(t, 5, r.calls, "failures have 30-second retry delay")
	entry := s.timing.tenants["a"].entries["0"]
	entry.retry = time.Now().Add(-time.Second)
	s.timing.tenants["a"].entries["0"] = entry
	r.fail = false
	rows, err = s.timedPendingInputs(timingContext(), "a")
	require.NoError(t, err)
	require.Equal(t, "2026-11-09T01:02:03Z", rows[0].RequestedAt)
	require.Equal(t, "successor", rows[0].RequestID)
	require.EqualValues(t, 13, *rows[0].TaskOrdinal)
	e.change(3, true)
	rows, err = s.timedPendingInputs(timingContext(), "a")
	require.NoError(t, err)
	require.Empty(t, rows)
	require.Empty(t, s.timing.tenants["a"].entries)
	for i := 0; i < 70; i++ {
		_, err = s.timedPendingInputs(timingContext(), string(rune(100+i)))
		require.NoError(t, err)
	}
	require.Len(t, s.timing.tenants, 64)
}
func TestPendingTimingRejectsDiscoveryRaceAndDeduplicates(t *testing.T) {
	for _, removed := range []bool{false, true} {
		t.Run(map[bool]string{false: "successor", true: "removed"}[removed], func(t *testing.T) {
			e := &timingEngine{revision: 1}
			r := &timingRuntime{}
			entered, proceed := make(chan struct{}), make(chan struct{})
			r.hook = func() { close(entered); <-proceed }
			s := &Server{engine: e, inputs: r}
			result := make(chan []pendingInputSummary, 1)
			go func() { rows, _ := s.timedPendingInputs(timingContext(), "a"); result <- rows }()
			<-entered
			e.change(2, removed)
			close(proceed)
			rows := <-result
			if removed {
				require.Empty(t, rows)
			} else {
				require.Len(t, rows, 1)
				require.Empty(t, rows[0].RequestedAt)
				require.EqualValues(t, 2, rows[0].ClientPayloadRevision)
			}
			require.Empty(t, s.timing.tenants["a"].entries)
		})
	}
	e := &timingEngine{revision: 1}
	r := &timingRuntime{}
	entered, proceed := make(chan struct{}), make(chan struct{})
	r.hook = func() { close(entered); <-proceed }
	s := &Server{engine: e, inputs: r}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = s.timedPendingInputs(timingContext(), "a") }()
	}
	<-entered
	close(proceed)
	wg.Wait()
	require.Equal(t, 1, r.calls, "concurrent consumers share discovery")
}
func TestPendingTimingRESTStreamParity(t *testing.T) {
	e := &timingEngine{revision: 1}
	r := &timingRuntime{}
	s := &Server{engine: e, inputs: r}
	w := httptest.NewRecorder()
	request := mux.SetURLVars(httptest.NewRequest("GET", "/", nil).WithContext(timingContext()), map[string]string{"projectId": "a"})
	s.handlePendingInputs(w, request)
	require.Equal(t, 200, w.Code)
	var rest []pendingInputSummary
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rest))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.handleInputStream(w, mux.SetURLVars(r, map[string]string{"projectId": "a"}))
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(timingContext())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	response, err := server.Client().Do(req)
	require.NoError(t, err)
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") && strings.Contains(line, "inputs") {
			var snapshot struct {
				Inputs []pendingInputSummary `json:"inputs"`
			}
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &snapshot))
			require.Equal(t, rest, snapshot.Inputs)
			require.Equal(t, 1, r.calls)
			return
		}
	}
	t.Fatal("stream ended without snapshot")
}

func TestPendingTimingVerificationFailureBacksOff(t *testing.T) {
	e := &timingEngine{revision: 1, failCall: 2}
	r := &timingRuntime{}
	s := &Server{engine: e, inputs: r}
	rows, err := s.timedPendingInputs(timingContext(), "a")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Empty(t, rows[0].RequestedAt)
	for range 5 {
		rows, err = s.timedPendingInputs(timingContext(), "a")
		require.NoError(t, err)
		require.Empty(t, rows[0].RequestedAt)
	}
	require.Equal(t, 1, r.calls)
}
func TestPendingTimingLegacyAndInvalidTimesNeverBorrowJobTime(t *testing.T) {
	for _, timestamp := range []string{"", "not-a-date", "0001-01-01T00:00:00Z"} {
		e := &timingEngine{revision: 1}
		r := &timingRuntime{occurrence: input.PendingInputOccurrence{JobID: "0", TaskOrdinal: 7, RequestedAt: timestamp}}
		s := &Server{engine: e, inputs: r}
		rows, err := s.timedPendingInputs(timingContext(), "a")
		require.NoError(t, err)
		require.Len(t, rows, 1)
		require.Empty(t, rows[0].RequestedAt)
		require.EqualValues(t, 7, *rows[0].TaskOrdinal)
		_, err = s.timedPendingInputs(timingContext(), "a")
		require.NoError(t, err)
		require.Equal(t, 1, r.calls)
	}
}

func TestPendingTimingCancellationAndIdleEviction(t *testing.T) {
	e := &timingEngine{revision: 1}
	r := &timingRuntime{}
	s := &Server{engine: e, inputs: r}
	ctx, cancel := context.WithCancel(timingContext())
	r.hook = cancel
	rows, err := s.timedPendingInputs(ctx, "a")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Empty(t, rows[0].RequestedAt)
	// The canceled call released its gate, so another request is never stranded.
	r.hook = nil
	s.timing.tenants["a"].used = time.Now().Add(-6 * time.Minute)
	_, err = s.timedPendingInputs(timingContext(), "b")
	require.NoError(t, err)
	require.NotContains(t, s.timing.tenants, "a")
	_, err = s.timedPendingInputs(timingContext(), "a")
	require.NoError(t, err)
	require.Equal(t, 3, r.calls)
}
