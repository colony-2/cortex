package cortex

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode"

	"github.com/colony-2/c2j/pkg/story"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

// handleInboxValue accepts only durable recorded selectors, never content to generate.
func (s *Server) handleInboxValue(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	q := r.URL.Query()
	allowed := map[string]bool{"job_attempt": true, "invocation_id": true, "task_ordinal": true, "task_attempt": true, "binding": true}
	for key, values := range q {
		if !allowed[key] || len(values) != 1 {
			writeHTTPError(w, 400, fmt.Errorf("invalid inbox selectors"))
			return
		}
	}
	positive := func(key string) int64 {
		n, err := strconv.ParseInt(q.Get(key), 10, 64)
		if err != nil || n <= 0 {
			return 0
		}
		return n
	}
	ja, ordinal, ta := positive("job_attempt"), positive("task_ordinal"), positive("task_attempt")
	if ja == 0 || ordinal == 0 || ta == 0 || strings.TrimSpace(q.Get("binding")) == "" || strings.TrimSpace(q.Get("invocation_id")) == "" {
		writeHTTPError(w, 400, fmt.Errorf("explicit inbox attempt and binding selectors required"))
		return
	}
	ctx := r.Context()
	if reader, ok := s.chapters.(chapterLookup); ok {
		key := jobdb.JobKey{TenantId: s.tenantID(r), JobId: mux.Vars(r)["jobId"]}
		ch, err := reader.GetChapter(ctx, jobdb.ChapterRef{JobKey: key, Ordinal: ordinal})
		if err != nil {
			writeError(w, err)
			return
		}
		// Native execution results contain the complete prepared input and
		// attempt identity. c2j still validates the selectors and binding.
		if record, err := chapterRecord(ch); err == nil && record.Kind == "result" {
			run := jobdb.GetJobRunResponse{Job: jobdb.JobRunSummary{JobKey: key}, Execution: []jobdb.ExecutionHistoryEntry{{Ordinal: ch.Ordinal, Record: record}}}
			ctx = context.WithValue(ctx, storyRunKey{}, &run)
		}
	}
	value, err := s.story.GetInboxValue(ctx, story.GetInboxValueRequest{ProjectID: s.tenantID(r), JobID: mux.Vars(r)["jobId"], JobAttempt: int(ja), InvocationID: q.Get("invocation_id"), TaskOrdinal: ordinal, TaskAttempt: int(ta), Binding: q.Get("binding")})
	if err != nil {
		status, reason := http.StatusInternalServerError, "history_unreadable"
		var failure *story.InboxError
		if errors.As(err, &failure) {
			reason = failure.Reason
			switch {
			case reason == "invalid_selector":
				status = 400
			case strings.HasSuffix(reason, "denied"):
				status = 403
			case reason == "binding_unknown" || strings.HasSuffix(reason, "missing"):
				status = 404
			case strings.Contains(reason, "unsupported") || reason == "stored_reference":
				status = 422
			}
		}
		writeJSON(w, status, map[string]string{"error": "Recorded inbox content unavailable", "code": reason})
		return
	}
	filename := path.Base(strings.ReplaceAll(value.Filename, "\\", "/"))
	filename = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, filename)
	if filename == "" || filename == "." || filename == ".." || filename == "/" {
		filename = "inbox.txt"
	}
	contentType := "text/plain; charset=utf-8"
	if value.Format == "json" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	w.Write(value.Data)
}
