package cortex

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/colony-2/c2j/pkg/artifacts"
	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/google/uuid"
)

type reviewV2Direction struct {
	ID               string `json:"id"`
	Label            string `json:"label"`
	FeedbackRequired *bool  `json:"feedback_required"`
	FeedbackMode     string `json:"feedback_mode"`
	AllowAttachments *bool  `json:"allow_attachments"`
	ContinuesReview  *bool  `json:"continues_review"`
}
type reviewV2Request struct {
	ReviewID   string                   `json:"review_id"`
	Title      string                   `json:"title"`
	Documents  map[string]artifacts.Ref `json:"documents"`
	Directions []reviewV2Direction      `json:"directions"`
	Feedback   struct {
		Label            string `json:"label"`
		AllowAttachments *bool  `json:"allow_attachments"`
	} `json:"feedback"`
}

func reviewV2Form(form input.InputForm) (reviewV2Request, error) {
	var request reviewV2Request
	if form.Presentation["type"] != "review/v2" || form.Kind != "" || form.Request == nil || form.ResponseSchema == nil || form.RequestSchema == nil {
		return request, fmt.Errorf("incompatible review/v2 request")
	}
	data, err := json.Marshal(form.Request)
	if err != nil {
		return request, err
	}
	if err = json.Unmarshal(data, &request); err != nil {
		return request, err
	}
	if !reviewV2Nonblank(request.ReviewID) || !reviewV2Nonblank(request.Title) || request.Documents == nil || len(request.Directions) == 0 || !reviewV2Nonblank(request.Feedback.Label) {
		return request, fmt.Errorf("incomplete review/v2 declaration")
	}
	// Presence matters: null and mixed encodings cannot acquire fallback rules.
	var raw struct {
		Directions []map[string]json.RawMessage `json:"directions"`
		Feedback   map[string]json.RawMessage   `json:"feedback"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return request, err
	}
	_, legacy := raw.Feedback["allow_attachments"]
	if legacy && request.Feedback.AllowAttachments == nil {
		return request, fmt.Errorf("invalid frozen feedback policy")
	}
	seen := map[string]bool{}
	for i := range request.Directions {
		d := &request.Directions[i]
		if !reviewV2Nonblank(d.ID) || !reviewV2Nonblank(d.Label) || seen[d.ID] || d.ContinuesReview == nil {
			return request, fmt.Errorf("invalid review/v2 direction declaration")
		}
		_, oldFlag := raw.Directions[i]["feedback_required"]
		_, mode := raw.Directions[i]["feedback_mode"]
		_, localUploads := raw.Directions[i]["allow_attachments"]
		if legacy {
			if d.FeedbackRequired == nil || mode || localUploads {
				return request, fmt.Errorf("mixed or incomplete frozen direction rules")
			}
			d.FeedbackMode = "optional"
			if *d.FeedbackRequired {
				d.FeedbackMode = "required"
			}
			d.AllowAttachments = request.Feedback.AllowAttachments
		} else if oldFlag || !mode || d.AllowAttachments == nil || (d.FeedbackMode != "forbidden" && d.FeedbackMode != "optional" && d.FeedbackMode != "required") || (d.FeedbackMode == "forbidden" && *d.AllowAttachments) {
			return request, fmt.Errorf("mixed, incomplete or contradictory direction rules")
		}
		seen[d.ID] = true
	}
	for id, ref := range request.Documents {
		if id == "" || ref.Kind != artifacts.RefKindStored || ref.Stored == nil || ref.Validate() != nil {
			return request, fmt.Errorf("invalid review/v2 document %q", id)
		}
	}
	return request, nil
}
func reviewV2Nonblank(text string) bool {
	for _, r := range text {
		if !unicode.Is(unicode.White_Space, r) && r != '\ufeff' {
			return true
		}
	}
	return false
}

// Native validation remains authoritative. Only this adapter creates upload refs;
// clients cannot substitute existing artifact keys, receipts or conversation.
func (s *Server) submitReviewV2(w http.ResponseWriter, r *http.Request, form input.InputForm, actor input.Actor, job string) {
	request, err := reviewV2Form(form)
	if err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	var envelope struct {
		RequestID    string `json:"request_id"`
		SubmissionID string `json:"submission_id"`
		Response     struct {
			Direction   string            `json:"direction"`
			Feedback    string            `json:"feedback"`
			Attachments []json.RawMessage `json:"attachments"`
			Settings    map[string]any    `json:"settings"`
		} `json:"response"`
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("review/v2 requires multipart submission"))
		return
	}
	// ParseMultipartForm may leave temporary files even on an error.
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	if len(r.MultipartForm.Value) != 1 || len(r.MultipartForm.Value["submission"]) != 1 {
		writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("exactly one submission part is required"))
		return
	}
	for name := range r.MultipartForm.File {
		if name != "attachments" {
			writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("unexpected upload part %q", name))
			return
		}
	}
	decoder := json.NewDecoder(strings.NewReader(r.MultipartForm.Value["submission"][0]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("unexpected data after submission"))
		return
	}
	var rawEnvelope map[string]json.RawMessage
	var rawResponse map[string]json.RawMessage
	if err := json.Unmarshal([]byte(r.MultipartForm.Value["submission"][0]), &rawEnvelope); err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	if err := json.Unmarshal(rawEnvelope["response"], &rawResponse); err != nil {
		writeHTTPError(w, http.StatusBadRequest, err)
		return
	}
	if len(rawResponse) != 4 || rawResponse["direction"] == nil || rawResponse["feedback"] == nil || string(rawResponse["direction"]) == "null" || string(rawResponse["feedback"]) == "null" {
		writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("all four contribution fields are required"))
		return
	}
	if envelope.RequestID != form.RequestID {
		writeHTTPError(w, http.StatusConflict, input.ErrStaleRequest)
		return
	}
	if envelope.SubmissionID == "" || envelope.Response.Attachments == nil || len(envelope.Response.Attachments) != 0 || envelope.Response.Settings == nil {
		writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("a submission ID, empty attachments array and settings object are required"))
		return
	}
	var direction *reviewV2Direction
	for i := range request.Directions {
		if request.Directions[i].ID == envelope.Response.Direction {
			direction = &request.Directions[i]
		}
	}
	files := r.MultipartForm.File["attachments"]
	if direction == nil || (len(files) > 0 && !*direction.AllowAttachments) {
		writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("unknown direction or disallowed attachments"))
		return
	}
	if direction.FeedbackMode == "forbidden" && envelope.Response.Feedback != "" {
		writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("this direction does not allow feedback"))
		return
	}
	if direction.FeedbackMode == "required" && !reviewV2Nonblank(envelope.Response.Feedback) && len(files) == 0 {
		writeHTTPError(w, http.StatusBadRequest, fmt.Errorf("this direction requires feedback or a permitted attachment"))
		return
	}
	uploads := []any{}
	for _, header := range files {
		file, err := header.Open()
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, err)
			return
		}
		data, readErr := io.ReadAll(file)
		closeErr := file.Close()
		if readErr != nil {
			writeHTTPError(w, http.StatusBadRequest, readErr)
			return
		}
		if closeErr != nil {
			writeHTTPError(w, http.StatusBadRequest, closeErr)
			return
		}
		name := filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
		if name == "." || name == "" {
			name = "attachment"
		}
		// A distinct path preserves readable filenames while preventing duplicate-name collisions.
		uploads = append(uploads, jobdb.NewArtifactFromBytes("review-notes/"+uuid.NewString()+"/"+name, data))
	}
	response := map[string]any{"direction": envelope.Response.Direction, "feedback": envelope.Response.Feedback, "attachments": uploads, "settings": envelope.Response.Settings}
	result, err := s.inputs.SubmitStructuredResponse(r.Context(), s.tenantID(r), job, input.StructuredSubmission{RequestID: envelope.RequestID, SubmissionID: envelope.SubmissionID, Response: response}, actor)
	writeReviewResult(w, result, err)
}
