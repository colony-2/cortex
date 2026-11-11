package cortex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/workflowctl"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

// Legacy attachment fields remain unchanged. Document locators are independent of names.
type inputDocument struct {
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
		rows = append(rows, inputDocument{ID: fmt.Sprintf("attachment:%d:%s", a.TaskOrdinal, a.Name), Name: a.Name, Sources: []string{"Attachment"}, Kind: "stored", JobID: a.JobID, TaskOrdinal: &a.TaskOrdinal, SizeBytes: a.SizeBytes, StoredKey: &key})
	}
	var data []byte
	for _, ch := range chapters {
		switch b := ch.Body.(type) {
		case jobdb.JobStartChapter:
			data = b.Input.Data
		case *jobdb.JobStartChapter:
			if b != nil {
				data = b.Input.Data
			}
		}
	}
	if len(data) == 0 {
		return rows, nil
	}
	var submission workflowctl.StartJob
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&submission); err != nil {
		return append(rows, inputDocument{ID: "documents:error", Name: "Submitted documents", Sources: []string{"Document"}, Kind: "invalid", Reason: "Unable to decode submitted inputs: " + err.Error(), SizeBytes: -1}), nil
	}
	raw, exists := submission.Inputs["documents"]
	if !exists || raw == nil {
		return rows, nil
	}
	bindings, ok := raw.(map[string]interface{})
	if !ok {
		return append(rows, inputDocument{ID: "documents:error", Name: "Submitted documents", Sources: []string{"Document"}, Kind: "invalid", Reason: "inputs.documents must be an object", SizeBytes: -1}), nil
	}
	names := make([]string, 0, len(bindings))
	for name := range bindings {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, binding := range names {
		row := inputDocument{ID: "document:" + binding, Binding: binding, Name: binding, Sources: []string{"Document"}, Kind: "invalid", SizeBytes: -1}
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
	vars := mux.Vars(r)
	if s.chapters == nil {
		writeError(w, fmt.Errorf("chapter reading unavailable"))
		return
	}
	chapters, err := s.chapters.ListChapters(r.Context(), jobdb.ListChaptersRequest{JobKey: jobdb.JobKey{TenantId: s.tenantID(r), JobId: vars["jobId"]}})
	if err != nil {
		writeError(w, err)
		return
	}
	rows, err := documentInventory(vars["jobId"], chapters)
	if err != nil {
		writeError(w, err)
		return
	}
	for _, row := range rows {
		if row.Binding != r.URL.Query().Get("binding") || row.ContentBinding == nil {
			continue
		}
		var reader io.Reader
		if row.Kind == "value" {
			var data []byte
			data, err = row.ref.ValueBytes()
			reader = bytes.NewReader(data)
		} else {
			artifact, e := s.engine.GetArtifact(s.tenantID(r), *row.StoredKey)
			err = e
			if err == nil {
				var opened io.ReadCloser
				opened, err = artifact.Open()
				if err == nil {
					defer opened.Close()
					reader = opened
				}
			}
		}
		if err != nil {
			writeHTTPError(w, http.StatusNotFound, fmt.Errorf("document %q unavailable: %w", row.Binding, err))
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(row.Name))
		_, _ = io.Copy(w, reader)
		return
	}
	writeHTTPError(w, http.StatusNotFound, fmt.Errorf("document unavailable"))
}
