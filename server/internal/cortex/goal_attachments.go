package cortex

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode"

	"github.com/colony-2/jobdb/pkg/jobdb"
)

const goalFileBytes = 25 << 20

// Decode the entire submission before resolving targets or creating a native job.
func decodeGoalSubmission(w http.ResponseWriter, r *http.Request, req *submitJobRequest) ([]jobdb.Artifact, int, error) {
	// Preserve legacy JSON callers that omitted Content-Type.
	if strings.TrimSpace(r.Header.Get("Content-Type")) == "" {
		return nil, 400, decodeRequest(w, r, req)
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return nil, 415, fmt.Errorf("unsupported Content-Type")
	}
	if media == "application/json" {
		return nil, 400, decodeRequest(w, r, req)
	}
	if media != "multipart/form-data" {
		return nil, 415, fmt.Errorf("unsupported Content-Type")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	// Install cleanup before parsing, including partially parsed requests.
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		return nil, goalUploadStatus(err), err
	}
	form := r.MultipartForm
	for field, values := range form.Value {
		if (field != "submission" && field != "filenames") || len(values) != 1 {
			return nil, 400, fmt.Errorf("unknown or repeated text field %q", field)
		}
	}
	if len(form.Value["submission"]) != 1 {
		return nil, 400, fmt.Errorf("exactly one submission text field is required")
	}
	raw := form.Value["submission"][0]
	if len(raw) > 1<<20 {
		return nil, 413, fmt.Errorf("submission metadata exceeds 1 MiB")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(req); err != nil {
		return nil, 400, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, 400, fmt.Errorf("expected one submission JSON object")
	}
	for field := range form.File {
		if field != "files" {
			return nil, 400, fmt.Errorf("unknown file field %q", field)
		}
	}
	files := form.File["files"]
	if len(files) > 10 {
		return nil, 400, fmt.Errorf("at most 10 files are supported")
	}
	// Optional ordered names preserve browser filenames without percent decoding.
	// Older multipart clients retain their literal MIME filename semantics.
	var names []string
	if values, ok := form.Value["filenames"]; ok {
		if len(values[0]) > 1<<20 {
			return nil, 413, fmt.Errorf("filename metadata exceeds 1 MiB")
		}
		if err := json.Unmarshal([]byte(values[0]), &names); err != nil || names == nil || len(names) != len(files) {
			return nil, 400, fmt.Errorf("filenames must contain one name per file")
		}
	}
	artifacts := make([]jobdb.Artifact, 0, len(files))
	seen := map[string]bool{}
	remaining := int64(goalFileBytes)
	for index, header := range files {
		_, params, err := mime.ParseMediaType(header.Header.Get("Content-Disposition"))
		if err != nil {
			return nil, 400, err
		}
		name := params["filename"]
		if err := validateGoalFilename(name); err != nil {
			return nil, 400, err
		}
		if names != nil {
			name = names[index]
		}
		if err := validateGoalFilename(name); err != nil {
			return nil, 400, err
		}
		if seen[name] {
			return nil, 400, fmt.Errorf("duplicate filename %q", name)
		}
		seen[name] = true
		if header.Size > remaining {
			return nil, 413, fmt.Errorf("files exceed 25 MiB total")
		}
		file, err := header.Open()
		if err != nil {
			return nil, 400, err
		}
		data, err := io.ReadAll(io.LimitReader(file, remaining+1))
		closeErr := file.Close()
		if err != nil {
			return nil, goalUploadStatus(err), err
		}
		if closeErr != nil {
			return nil, 400, closeErr
		}
		if int64(len(data)) > remaining {
			return nil, 413, fmt.Errorf("files exceed 25 MiB total")
		}
		remaining -= int64(len(data))
		artifacts = append(artifacts, jobdb.NewArtifactFromBytes(name, data))
	}
	return artifacts, 0, nil
}

func goalUploadStatus(err error) int {
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}

func validateGoalFilename(name string) error {
	if strings.TrimSpace(name) == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\=") || (len(name) >= 2 && name[1] == ':') || strings.ContainsFunc(name, unicode.IsControl) {
		return fmt.Errorf("invalid filename %q: choose a flat filename without paths or control characters", name)
	}
	return nil
}
