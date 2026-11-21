package cortex

import (
	"errors"
	"net/http"

	"github.com/colony-2/c2j/pkg/input"
	"github.com/gorilla/mux"
)

// Discovery advertises pending requests; this read retrieves their content.
// A request can finish after discovery without making the read an error.
func (s *Server) handleInputDetails(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	s.requireArchivedJob(s.serveInputDetails)(w, r)
}

func (s *Server) serveInputDetails(w http.ResponseWriter, r *http.Request) {
	details, err := s.inputs.GetDetails(r.Context(), s.tenantID(r), mux.Vars(r)["jobId"])
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
