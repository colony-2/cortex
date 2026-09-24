package cortex

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
	"github.com/gorilla/mux"
)

// Track the waiting task, not just the job: a recipe can ask for input more than
// once. Read JobDB so transitions made by other processes (including expiry)
// reach every browser, even when no local input API broadcast was sent.
func (s *Server) handleInputStream(w http.ResponseWriter, r *http.Request) {
	projectID := mux.Vars(r)["projectId"]
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	send := func(event string, data any) error {
		payload, err := json.Marshal(data)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload); err != nil {
			return err
		}
		return controller.Flush()
	}
	if err := send("connected", map[string]string{"client_id": fmt.Sprintf("cortex-%d", time.Now().UnixNano())}); err != nil {
		return
	}

	previous := map[string]int64{}
	poll := time.NewTicker(time.Second)
	defer poll.Stop()
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	for {
		tasks, err := s.engine.FindTasksWaiting(r.Context(), jobworkflow.FindTasksWaitingRequest{
			JobType: "recipe", TaskType: "input:collect_user_input", TenantIds: []string{projectID},
		})
		if err != nil {
			if r.Context().Err() != nil {
				return
			}
			s.logger.Warn("load pending inputs for stream", "tenant", projectID, "error", err)
			if err := send("error", map[string]string{"error": "failed to load pending inputs"}); err != nil {
				return
			}
		} else {
			current := make(map[string]int64, len(tasks))
			for _, task := range tasks {
				current[task.JobKey().JobId] = task.TaskOrdinalToComplete()
			}
			for id := range previous {
				if _, pending := current[id]; !pending {
					// The request has ended, whether answered, cancelled, or expired.
					if err := send("input_completed", map[string]string{"jobId": id}); err != nil {
						return
					}
				}
			}
			for id, ordinal := range current {
				if old, seen := previous[id]; !seen || old != ordinal {
					if err := send("input_pending", map[string]any{"id": id, "task_ordinal": ordinal}); err != nil {
						return
					}
				}
			}
			previous = current
		}
		select {
		case <-r.Context().Done():
			return
		case <-poll.C:
		case <-heartbeat.C:
			if err := send("heartbeat", map[string]any{}); err != nil {
				return
			}
		}
	}
}
