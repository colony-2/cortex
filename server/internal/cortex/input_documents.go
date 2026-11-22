package cortex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/workflowctl"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

// Legacy attachment fields remain unchanged. Document locators are independent of names.
type inputDocument struct {
	InputOrdinal   int64              `json:"inputOrdinal"`
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Binding        string             `json:"binding,omitempty"`
	Sources        []string           `json:"sources"`
	Kind           string             `json:"kind"`
	Reason         string             `json:"reason,omitempty"`
	JobID          string             `json:"jobId,omitempty"`
	TaskOrdinal    *int64             `json:"taskOrdinal,omitempty"`
	SizeBytes      int64              `json:"sizeBytes"`
	ContentBinding *string            `json:"contentBinding,omitempty"`
	StoredKey      *jobdb.ArtifactKey `json:"storedKey,omitempty"`
	ref            artifacts.Ref
}

func documentInventory(job string, chapters []jobdb.Chapter) ([]inputDocument, error) {
	direct, err := inputArtifacts(job, chapters)
	if err != nil {
		return nil, err
	}
	rows := make([]inputDocument, 0, len(direct))
	for _, a := range direct {
		key := jobdb.ArtifactKey{JobId: a.JobID, TaskOrdinal: a.TaskOrdinal, Name: a.Name, SizeBytes: a.SizeBytes}
		id := fmt.Sprintf("attachment:%d:%s", a.TaskOrdinal, a.Name)
		if a.contribution != nil {
			id = fmt.Sprintf("contribution:%d:%s:%d:%s", *a.contribution, a.JobID, a.TaskOrdinal, a.Name)
		}
		rows = append(rows, inputDocument{InputOrdinal: a.inputOrdinal, ID: id, Name: a.Name, Sources: []string{"Attachment"}, Kind: "stored", JobID: a.JobID, TaskOrdinal: &a.TaskOrdinal, SizeBytes: a.SizeBytes, StoredKey: &key})
	}
	var startOrdinal int64
	var data []byte
	for _, ch := range chapters {
		switch b := ch.Body.(type) {
		case jobdb.JobStartChapter:
			data = b.Input.Data
			startOrdinal = ch.Ordinal
		case *jobdb.JobStartChapter:
			if b != nil {
				data = b.Input.Data
				startOrdinal = ch.Ordinal
			}
		}
	}
	if len(data) == 0 {
		return rows, nil
	}
	// JobStart.Input is the ordinary creator payload even for grouped jobs.
	// AdditionalInput chapters are contributions, not replacement submissions.
	var submission workflowctl.StartJob
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&submission); err != nil {
		return append(rows, inputDocument{InputOrdinal: startOrdinal, ID: "documents:error", Name: "Submitted documents", Sources: []string{"Document"}, Kind: "invalid", Reason: "Unable to decode submitted inputs: " + err.Error(), SizeBytes: -1}), nil
	}
	raw, exists := submission.Inputs["documents"]
	if !exists || raw == nil {
		return rows, nil
	}
	bindings, ok := raw.(map[string]interface{})
	if !ok {
		return append(rows, inputDocument{InputOrdinal: startOrdinal, ID: "documents:error", Name: "Submitted documents", Sources: []string{"Document"}, Kind: "invalid", Reason: "inputs.documents must be an object", SizeBytes: -1}), nil
	}
	names := make([]string, 0, len(bindings))
	for name := range bindings {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, binding := range names {
		row := inputDocument{InputOrdinal: startOrdinal, ID: "document:" + binding, Binding: binding, Name: binding, Sources: []string{"Document"}, Kind: "invalid", SizeBytes: -1}
		var ref artifacts.Ref
		err := ref.DecodeFromMap(bindings[binding])
		if err == nil {
			err = ref.Validate()
		}
		if ref.NameValue() != "" {
			row.Name = ref.NameValue()
		}
		if err != nil {
			row.Reason = "Invalid document: " + err.Error()
			rows = append(rows, row)
			continue
		}
		row.Kind = string(ref.Kind)
		row.ref = ref
		switch ref.Kind {
		case artifacts.RefKindStored:
			key, _ := ref.StoredKey()
			row.StoredKey = &key
			row.SizeBytes = ref.SizeBytes()
			row.ContentBinding = &row.Binding
			merged := false
			for i := range rows {
				a := &rows[i]
				if a.Binding == "" && a.Name == binding && a.Name == row.Name && a.StoredKey != nil && a.StoredKey.JobId == key.JobId && a.StoredKey.TaskOrdinal == key.TaskOrdinal && a.StoredKey.Name == key.Name {
					a.Binding = binding
					a.Sources = append(a.Sources, "Document")
					merged = true
					break
				}
			}
			if merged {
				continue
			}
		case artifacts.RefKindValue:
			bytes, _ := ref.ValueBytes()
			row.SizeBytes = int64(len(bytes))
			row.ContentBinding = &row.Binding
		case artifacts.RefKindExternal:
			row.Reason = "External reference — content not loaded"
		case artifacts.RefKindAttached:
			row.Reason = "Attached document has no stored producer reference"
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (s *Server) handleInputDocument(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	rows, err := s.readInputInventory(r)
	if err != nil {
		writeError(w, err)
		return
	}
	for _, row := range rows {
		if row.Binding != r.URL.Query().Get("binding") || row.ContentBinding == nil {
			continue
		}
		if row.StoredKey != nil {
			key := row.StoredKey
			ref := jobdb.ArtifactRef{JobKey: jobdb.JobKey{TenantId: s.tenantID(r), JobId: key.JobId}, Ordinal: key.TaskOrdinal, Name: key.Name}
			s.serveStoredDocument(w, r, ref, row.Name)
			return
		}
		data, err := row.ref.ValueBytes()
		if err != nil {
			writeHTTPError(w, http.StatusNotFound, fmt.Errorf("document %q unavailable: %w", row.Binding, err))
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(row.Name))
		_, _ = w.Write(data)
		return
	}
	writeHTTPError(w, http.StatusNotFound, fmt.Errorf("document unavailable"))
}

// handleStoredDocument uses the producer coordinates already carried by a review
// document. The artifact read is scoped to the caller and tenant by JobDB.
func (s *Server) handleStoredDocument(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	query := r.URL.Query()
	ordinal, err := strconv.ParseInt(query.Get("ordinal"), 10, 64)
	if err != nil || ordinal < 0 || len(query["ordinal"]) != 1 || len(query["name"]) != 1 || query.Get("name") == "" {
		writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("a non-negative ordinal and artifact name are required"))
		return
	}
	ref := jobdb.ArtifactRef{JobKey: jobdb.JobKey{TenantId: s.tenantID(r), JobId: mux.Vars(r)["jobId"]}, Ordinal: ordinal, Name: query.Get("name")}
	s.serveStoredDocument(w, r, ref, ref.Name)
}

func (s *Server) serveStoredDocument(w http.ResponseWriter, r *http.Request, ref jobdb.ArtifactRef, filename string) {
	var stream io.ReadCloser
	if s.documentClient != nil {
		// Stream the JobDB response and preserve its status. The
		// generic remote OpenArtifact client buffers the whole body first.
		path := fmt.Sprintf("%s/v1/tenants/%s/jobs/%s/chapters/%d/artifacts/%s", strings.TrimRight(s.cfg.JobDBURL, "/"), url.PathEscape(ref.JobKey.TenantId), url.PathEscape(ref.JobKey.JobId), ref.Ordinal, url.PathEscape(ref.Name))
		request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, path, nil)
		if err != nil {
			writeError(w, err)
			return
		}
		if auth := r.Header.Get("Authorization"); auth != "" {
			request.Header.Set("Authorization", auth)
		}
		upstream, err := s.documentClient.Do(request)
		if err != nil {
			writeError(w, err)
			return
		}
		if upstream.StatusCode != http.StatusOK {
			defer upstream.Body.Close()
			message, _ := io.ReadAll(io.LimitReader(upstream.Body, 4096))
			writeHTTPError(w, upstream.StatusCode, fmt.Errorf("JobDB artifact read: %s %s", upstream.Status, strings.TrimSpace(string(message))))
			return
		}
		if upstream.ContentLength >= 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(upstream.ContentLength, 10))
		}
		stream = upstream.Body
	} else {
		reader, ok := s.chapters.(artifactLookup)
		if !ok {
			writeHTTPError(w, http.StatusServiceUnavailable, fmt.Errorf("artifact reading unavailable"))
			return
		}
		// Embedded runtimes apply cell visibility through their scoped job list.
		if _, scoped := jobdb.CellAuthorityFromContext(r.Context()); scoped {
			if err := s.checkProducerVisibility(r.Context(), ref.JobKey); err != nil {
				writeError(w, fmt.Errorf("document unavailable: %w", err))
				return
			}
		}
		artifact, err := reader.OpenArtifact(r.Context(), ref)
		if err != nil {
			// The embedded JobDB runtimes use a plain error for a missing artifact.
			if err.Error() == fmt.Sprintf("artifact %s not found for job %s ordinal %d", ref.Name, ref.JobKey.JobId, ref.Ordinal) {
				writeHTTPError(w, http.StatusNotFound, fmt.Errorf("document unavailable: %w", err))
				return
			}
			writeError(w, fmt.Errorf("document unavailable: %w", err))
			return
		}
		stream, err = artifact.Open()
		if err != nil {
			writeError(w, err)
			return
		}
	}
	defer stream.Close()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(filename))
	if _, err := io.Copy(w, stream); err != nil {
		// Prevent clients from treating a truncated upstream read as complete.
		panic(http.ErrAbortHandler)
	}
}
