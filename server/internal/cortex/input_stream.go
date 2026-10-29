package cortex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

// Poll compact summaries, including payload revisions, so changed requests and
// expiry reach browsers without reading task handles or forms. Send one snapshot
// per change rather than triggering a new list request for every pending job.
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

	var previous []byte
	poll := time.NewTicker(time.Second)
	defer poll.Stop()
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()
	for {
		inputs, err := s.pendingInputSummaries(r.Context(), projectID)
		if err != nil {
			if r.Context().Err() != nil {
				return
			}
			s.logger.Warn("load pending inputs for stream", "tenant", projectID, "error", err)
			if err := send("error", map[string]string{"error": "failed to load pending inputs"}); err != nil {
				return
			}
		} else {
			current, err := json.Marshal(inputs)
			if err != nil {
				return
			}
			if !bytes.Equal(previous, current) {
				if err := send("inputs_changed", map[string]any{"inputs": inputs}); err != nil {
					return
				}
				previous = current
			}
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
