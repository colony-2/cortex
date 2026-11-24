package cortex

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"
	"unicode"

	"github.com/colony-2/jobdb/pkg/jobdb"
)

// Keep uploads on disk until the synchronous job submission finishes.
type goalUploads struct{ dir string }

func (u *goalUploads) cleanup() {
	if u.dir != "" {
		_ = os.RemoveAll(u.dir)
	}
}

// Decode the entire submission before resolving targets or creating a native job.
func decodeGoalSubmission(r *http.Request, req *submitJobRequest, uploads *goalUploads) ([]jobdb.Artifact, int, error) {
	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	if contentType == "" {
		return nil, 400, decodeJSON(r.Body, req)
	}
	media, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, 415, fmt.Errorf("unsupported Content-Type")
	}
	if media == "application/json" {
		return nil, 400, decodeJSON(r.Body, req)
	}
	if media != "multipart/form-data" {
		return nil, 415, fmt.Errorf("unsupported Content-Type")
	}
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, 400, err
	}
	type upload struct{ name, path string }
	var files []upload
	var names []string
	var hasSubmission, hasNames bool
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 400, err
		}
		disposition, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		if err != nil || disposition != "form-data" {
			return nil, 400, fmt.Errorf("invalid form-data part")
		}
		field := params["name"]
		name, isFile := params["filename"]
		switch {
		case field == "submission" && !isFile && !hasSubmission:
			if err := decodeJSON(part, req); err != nil {
				return nil, 400, err
			}
			hasSubmission = true
		case field == "filenames" && !isFile && !hasNames:
			if err := decodeJSON(part, &names); err != nil || names == nil {
				return nil, 400, fmt.Errorf("filenames must contain one name per file")
			}
			hasNames = true
		case field == "files" && isFile:
			if err := validateGoalFilename(name); err != nil {
				return nil, 400, err
			}
			if uploads.dir == "" {
				uploads.dir, err = os.MkdirTemp("", "cortex-job-attachments-")
				if err != nil {
					return nil, 500, fmt.Errorf("could not create attachment storage")
				}
			}
			file, err := os.CreateTemp(uploads.dir, "attachment-")
			if err != nil {
				return nil, 500, fmt.Errorf("could not store attachment %q", name)
			}
			_, err = io.Copy(file, part)
			closeErr := file.Close()
			if err != nil {
				return nil, 400, fmt.Errorf("could not read attachment %q", name)
			}
			if closeErr != nil {
				return nil, 500, fmt.Errorf("could not save attachment %q", name)
			}
			files = append(files, upload{name: name, path: file.Name()})
		default:
			return nil, 400, fmt.Errorf("unknown or repeated field %q", field)
		}
	}
	if !hasSubmission {
		return nil, 400, fmt.Errorf("exactly one submission text field is required")
	}
	if hasNames && len(names) != len(files) {
		return nil, 400, fmt.Errorf("filenames must contain one name per file")
	}
	artifacts := make([]jobdb.Artifact, 0, len(files))
	seen := map[string]bool{}
	for i, file := range files {
		name := file.name
		// Browser clients send the original ordered names separately so MIME
		// quoting does not change filenames. Older callers use literal MIME names.
		if hasNames {
			name = names[i]
			if err := validateGoalFilename(name); err != nil {
				return nil, 400, err
			}
		}
		if seen[name] {
			return nil, 400, fmt.Errorf("duplicate filename %q", name)
		}
		seen[name] = true
		artifact, err := jobdb.NewArtifactFromFileNoCleanup(name, file.path)
		if err != nil {
			return nil, 500, fmt.Errorf("could not read stored attachment %q", name)
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, 0, nil
}

func validateGoalFilename(name string) error {
	if strings.TrimSpace(name) == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\=") || (len(name) >= 2 && name[1] == ':') || strings.ContainsFunc(name, unicode.IsControl) {
		return fmt.Errorf("invalid filename %q: choose a flat filename without paths or control characters", name)
	}
	return nil
}
