package cortex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

// Pulse diagnostics describe the controller's fleet. Only these read endpoints
// are exposed. The upstream address is chosen by the Cortex operator.
func (s *Server) registerPulseRoutes(api *mux.Router) {
	for _, path := range []string{
		"/status", "/config", "/instances", "/providers", "/providers/health",
		"/providers/{provider}/capabilities", "/providers/{provider}/instances",
		"/providers/{provider}/launches/{launch}/logs",
	} {
		api.HandleFunc("/pulse"+path, s.pulseRead(path)).Methods(http.MethodGet)
	}
	api.HandleFunc("/pulse/providers/{provider}/launches/{launch}/download", s.downloadPulseLog).Methods(http.MethodGet)
	api.HandleFunc("/projects/{projectId}/runner-launches", s.handleRunnerHistory).Methods(http.MethodGet)
}

func (s *Server) pulseRead(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		resp, cancel, err := s.pulseRequest(r, path, r.URL.RawQuery)
		if err != nil {
			writePulseError(w, err)
			return
		}
		defer cancel()
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(resp.StatusCode) // A 502 instance list can contain usable rows.
		_, _ = io.Copy(w, resp.Body)
	}
}

// Each request has its own timeout. A long download can continue one page at
// a time, and closing the browser request cancels the active upstream read.
func (s *Server) pulseRequest(r *http.Request, path string, query string) (*http.Response, func(), error) {
	target := path
	for name, value := range mux.Vars(r) {
		if value == "." || value == ".." || strings.ContainsAny(value, "/\\") {
			return nil, nil, fmt.Errorf("invalid Pulse path parameter")
		}
		target = strings.Replace(target, "{"+name+"}", url.PathEscape(value), 1)
	}
	addr := strings.TrimRight(s.cfg.PulseURL, "/") + target
	if query != "" {
		addr += "?" + query
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	up, err := http.NewRequestWithContext(ctx, http.MethodGet, addr, nil)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	resp, err := http.DefaultClient.Do(up)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return resp, cancel, nil
}

func writePulseError(w http.ResponseWriter, err error) {
	writeHTTPError(w, http.StatusBadGateway, fmt.Errorf("Pulse request failed: %w", err))
}

func (s *Server) downloadPulseLog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	const path = "/providers/{provider}/launches/{launch}/logs"
	query := url.Values{"page_size": {"100"}}
	seen := map[string]bool{}
	started := false
	for {
		resp, cancel, err := s.pulseRequest(r, path, query.Encode())
		if err != nil {
			if started {
				panic(http.ErrAbortHandler)
			}
			writePulseError(w, err)
			return
		}
		var page struct {
			Records []struct{ Data []byte } `json:"records"`
			Next    string                  `json:"next_page_token"`
			More    bool                    `json:"has_more"`
		}
		if resp.StatusCode != http.StatusOK {
			defer resp.Body.Close()
			defer cancel()
			if started {
				panic(http.ErrAbortHandler)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(resp.StatusCode)
			_, _ = io.Copy(w, resp.Body)
			return
		}
		err = json.NewDecoder(resp.Body).Decode(&page)
		resp.Body.Close()
		cancel()
		if err != nil || page.Records == nil || (page.More && (page.Next == "" || seen[page.Next])) {
			if started {
				panic(http.ErrAbortHandler)
			}
			writeHTTPError(w, http.StatusBadGateway, fmt.Errorf("invalid Pulse log page"))
			return
		}
		if !started {
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": mux.Vars(r)["launch"] + ".log"}))
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.WriteHeader(http.StatusOK)
			started = true
		}
		for _, record := range page.Records {
			if _, err := w.Write(record.Data); err != nil {
				return
			}
		}
		if !page.More {
			return
		}
		seen[page.Next] = true
		query.Set("page_token", page.Next)
	}
}

// Acquisitions identify launches even after their providers stop listing them.
// An acquired lease alone supplies neither a terminal state nor an end time.
func (s *Server) handleRunnerHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.leaseEvents == nil {
		writeError(w, jobdb.ErrLeaseEventsUnsupported)
		return
	}
	query := r.URL.Query()
	req := jobdb.ListLeaseEventsRequest{TenantID: mux.Vars(r)["projectId"], EventTypes: []jobdb.LeaseEventType{jobdb.LeaseEventAcquired}, IncludeHidden: true, PageSize: 100, PageToken: query.Get("pageToken")}
	if id := query.Get("jobId"); id != "" {
		req.JobIDs = []string{id}
	}
	if id := query.Get("launchId"); id != "" {
		req.WorkerIDs = []string{id}
	}
	page, err := s.leaseEvents.ListLeaseEvents(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}
