package cortex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

// Keep review validation, request identity, and persistence in c2j.
type inputRuntime interface {
	ListPendingInputs(context.Context, string) ([]input.PendingInput, error)
	GetForm(context.Context, string, string) (input.InputForm, error)
	SubmitFormResponse(context.Context, string, string, input.FormSubmission, input.Actor) (input.Output, error)
}

type pendingInputSummary struct {
	ID            string `json:"id"`
	Kind          string `json:"kind,omitempty"`
	Title         string `json:"title,omitempty"`
	RequestID     string `json:"request_id,omitempty"`
	DocumentCount int    `json:"document_count,omitempty"`
}

func (s *Server) handlePendingInputs(w http.ResponseWriter, r *http.Request) {
	tenant := s.tenantID(r)
	pending, err := s.inputs.ListPendingInputs(r.Context(), tenant)
	if err != nil {
		writeError(w, err)
		return
	}
	summaries := make([]pendingInputSummary, 0, len(pending))
	for _, item := range pending {
		form, err := s.inputs.GetForm(r.Context(), tenant, item.Id)
		if errors.Is(err, input.ErrInputNotPending) {
			continue
		}
		if err != nil {
			writeError(w, err)
			return
		}
		title := form.Title
		if title == "" {
			title = form.Question
		}
		summaries = append(summaries, pendingInputSummary{ID: item.Id, Kind: form.Kind, Title: title, RequestID: form.RequestID, DocumentCount: len(form.Documents)})
	}
	writeJSON(w, http.StatusOK, summaries)
}

func (s *Server) handleReviewDocument(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	form, err := s.inputs.GetForm(r.Context(), s.tenantID(r), vars["jobId"])
	if err != nil {
		writeHTTPError(w, http.StatusNotFound, err)
		return
	}
	if form.Kind != "review" {
		writeHTTPError(w, http.StatusNotFound, fmt.Errorf("review not found"))
		return
	}
	if r.URL.Query().Get("request_id") != form.RequestID {
		writeHTTPError(w, http.StatusConflict, fmt.Errorf("review has changed; reload it before opening documents"))
		return
	}
	ref, ok := form.Documents[r.URL.Query().Get("document_id")]
	if !ok || ref.Stored == nil {
		writeHTTPError(w, http.StatusNotFound, fmt.Errorf("document not found"))
		return
	}
	// Resolve the original producer, including child jobs, within the current tenant.
	artifact, err := s.engine.GetArtifact(s.tenantID(r), ref.Stored.Key)
	if err != nil {
		writeError(w, err)
		return
	}
	reader, err := artifact.Open()
	if err != nil {
		writeError(w, err)
		return
	}
	defer reader.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(artifact.Name()))
	_, _ = io.Copy(w, reader)
}

func (s *Server) handleReviewResponse(w http.ResponseWriter, r *http.Request) {
	// Cortex currently identifies users through its email login cookie.
	// The existing browser cookie contains colons, which net/http's cookie
	// parser rejects as a name. Read that exact legacy name from the header.
	var encodedEmail string
	for _, header := range r.Header.Values("Cookie") {
		for _, part := range strings.Split(header, ";") {
			name, value, ok := strings.Cut(strings.TrimSpace(part), "=")
			if ok && name == "colony2:user:email" {
				encodedEmail = value
			}
		}
	}
	if encodedEmail == "" {
		writeHTTPError(w, http.StatusUnauthorized, fmt.Errorf("sign in before submitting a review"))
		return
	}
	email, err := url.PathUnescape(encodedEmail)
	if err != nil || strings.TrimSpace(email) == "" {
		writeHTTPError(w, http.StatusUnauthorized, fmt.Errorf("invalid user identity"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	var submission input.FormSubmission
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			writeHTTPError(w, http.StatusBadRequest, err)
			return
		}
		defer r.MultipartForm.RemoveAll()
		if err := json.Unmarshal([]byte(r.FormValue("submission")), &submission); err != nil {
			writeHTTPError(w, http.StatusBadRequest, err)
			return
		}
		if submission.Fields == nil {
			submission.Fields = map[string]any{}
		}
		for field, files := range r.MultipartForm.File {
			if len(files) != 1 {
				writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("one attachment per question is supported"))
				return
			}
			file, err := files[0].Open()
			if err != nil {
				writeHTTPError(w, http.StatusBadRequest, err)
				return
			}
			data, err := io.ReadAll(file)
			file.Close()
			if err != nil {
				writeHTTPError(w, http.StatusBadRequest, err)
				return
			}
			submission.Fields[field] = jobdb.NewArtifactFromBytes(files[0].Filename, data)
		}
	} else if err := json.NewDecoder(r.Body).Decode(&submission); err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	result, err := s.inputs.SubmitFormResponse(r.Context(), s.tenantID(r), mux.Vars(r)["jobId"], submission, input.Actor{ID: email, Kind: "human"})
	if errors.Is(err, input.ErrInputNotPending) {
		writeHTTPError(w, http.StatusConflict, err)
		return
	}
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
