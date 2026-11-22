package cortex

import (
	"context"
	"errors"
	"net/http"

	"github.com/colony-2/c2j/pkg/input"
	workerworkflow "github.com/colony-2/c2j/pkg/worker/workflow"
	"github.com/colony-2/c2j/pkg/workflowctl"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

// GetDetails asks for history only to recover the alternate deadline of its
// waiting task. Native jobs record that wait immediately before the output.
type pendingTaskControl struct {
	*workerworkflow.SWFWorkflowControl
	chapters chapterLookup
}

type pendingTaskKey struct{}

func (c pendingTaskControl) GetWaitingTask(ctx context.Context, key jobdb.JobKey) (workflowctl.TaskHandle, error) {
	task, err := c.SWFWorkflowControl.GetWaitingTask(ctx, key)
	if ref, ok := ctx.Value(pendingTaskKey{}).(*jobdb.ChapterRef); ok && err == nil && task != nil {
		*ref = jobdb.ChapterRef{JobKey: key, Ordinal: task.TaskOrdinalToComplete() - 1}
	}
	return task, err
}

func (c pendingTaskControl) GetJobRun(ctx context.Context, req jobdb.GetJobRunRequest) (jobdb.GetJobRunResponse, error) {
	if ref, ok := ctx.Value(pendingTaskKey{}).(*jobdb.ChapterRef); ok && ref.JobKey == req.JobKey && ref.Ordinal >= 0 {
		ch, err := c.chapters.GetChapter(ctx, *ref)
		if err != nil && !errors.Is(err, jobdb.ErrChapterNotFound) {
			return jobdb.GetJobRunResponse{}, err
		}
		if err == nil {
			if record, err := chapterRecord(ch); err == nil && record.Kind == "wait" {
				return jobdb.GetJobRunResponse{Execution: []jobdb.ExecutionHistoryEntry{{Ordinal: ch.Ordinal, Record: record}}}, nil
			}
		}
	}
	return c.SWFWorkflowControl.GetJobRun(ctx, req)
}

// Discovery advertises pending requests; this read retrieves their content.
// A request can finish after discovery without making the read an error.
func (s *Server) handleInputDetails(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	s.requireArchivedJob(s.serveInputDetails)(w, r)
}

func (s *Server) serveInputDetails(w http.ResponseWriter, r *http.Request) {
	ctx := context.WithValue(r.Context(), pendingTaskKey{}, &jobdb.ChapterRef{})
	details, err := s.inputs.GetDetails(ctx, s.tenantID(r), mux.Vars(r)["jobId"])
	if errors.Is(err, input.ErrInputNotPending) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		s.logger.Error("get input details", "tenant", s.tenantID(r), "job_id", mux.Vars(r)["jobId"], "error", err)
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, details)
}
