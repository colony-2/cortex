package cortex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

type chapterReader interface {
	ListChapters(context.Context, jobdb.ListChaptersRequest) ([]jobdb.Chapter, error)
}

type chapterExport struct {
	FormatVersion int               `json:"format_version"`
	ProjectID     string            `json:"project_id"`
	JobID         string            `json:"job_id"`
	ExportedAt    time.Time         `json:"exported_at"`
	Chapters      []exportedChapter `json:"chapters"`
}

type exportedChapter struct {
	Ordinal   int64              `json:"ordinal"`
	TaskType  string             `json:"task_type"`
	CreatedAt time.Time          `json:"created_at"`
	InputHash string             `json:"input_hash"`
	Metadata  map[string]any     `json:"metadata"`
	Artifacts []exportedArtifact `json:"artifacts"`
	Body      any                `json:"body"`
}

type exportedArtifact struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

func exportBytes(data []byte) any {
	return map[string]any{"encoding": "base64", "data": base64.StdEncoding.EncodeToString(data)}
}

func exportOutcome(outcome jobdb.ChapterOutcome) (any, error) {
	switch v := outcome.(type) {
	case *jobdb.ApplicationOutputOutcome:
		if v == nil {
			return nil, fmt.Errorf("nil chapter outcome %T", outcome)
		}
		return exportOutcome(*v)
	case jobdb.ApplicationOutputOutcome:
		return map[string]any{"kind": "application_output", "output": exportBytes(v.Output.Data)}, nil
	case *jobdb.AppErrorOutcome:
		if v == nil {
			return nil, fmt.Errorf("nil chapter outcome %T", outcome)
		}
		return exportOutcome(*v)
	case jobdb.AppErrorOutcome:
		return map[string]any{"kind": "app_error", "error": v.Error}, nil
	case *jobdb.SystemErrorOutcome:
		if v == nil {
			return nil, fmt.Errorf("nil chapter outcome %T", outcome)
		}
		return exportOutcome(*v)
	case jobdb.SystemErrorOutcome:
		return map[string]any{"kind": "system_error", "error": v.Error}, nil
	case *jobdb.TimeoutOutcome:
		if v == nil {
			return nil, fmt.Errorf("nil chapter outcome %T", outcome)
		}
		return exportOutcome(*v)
	case jobdb.TimeoutOutcome:
		return map[string]any{"kind": "timeout", "timeout": v.Timeout}, nil
	default:
		return nil, fmt.Errorf("unsupported chapter outcome %T", outcome)
	}
}

func exportBody(body jobdb.ChapterBody) (any, error) {
	switch v := body.(type) {
	case *jobdb.JobStartChapter:
		if v == nil {
			return nil, fmt.Errorf("nil chapter body %T", body)
		}
		return exportBody(*v)
	case jobdb.JobStartChapter:
		return map[string]any{"kind": "job_start", "input": exportBytes(v.Input.Data)}, nil
	case *jobdb.RestartExtraChapter:
		if v == nil {
			return nil, fmt.Errorf("nil chapter body %T", body)
		}
		return exportBody(*v)
	case jobdb.RestartExtraChapter:
		return map[string]any{"kind": "restart_extra", "output": exportBytes(v.Output.Data)}, nil
	case *jobdb.ExecutionChapter:
		if v == nil {
			return nil, fmt.Errorf("nil chapter body %T", body)
		}
		return exportBody(*v)
	case jobdb.ExecutionChapter:
		if !json.Valid(v.Record) {
			return nil, fmt.Errorf("invalid execution chapter JSON")
		}
		return map[string]any{"kind": "execution", "record": v.Record}, nil
	case *jobdb.JobAttemptOutcomeChapter:
		if v == nil {
			return nil, fmt.Errorf("nil chapter body %T", body)
		}
		return exportBody(*v)
	case jobdb.JobAttemptOutcomeChapter:
		outcome, err := exportOutcome(v.Outcome)
		return map[string]any{"kind": "job_attempt_outcome", "outcome": outcome}, err
	case *jobdb.TaskAttemptOutcomeChapter:
		if v == nil {
			return nil, fmt.Errorf("nil chapter body %T", body)
		}
		return exportBody(*v)
	case jobdb.TaskAttemptOutcomeChapter:
		outcome, err := exportOutcome(v.Outcome)
		return map[string]any{"kind": "task_attempt_outcome", "outcome": outcome}, err
	default:
		return nil, fmt.Errorf("unsupported chapter body %T", body)
	}
}

func exportMetadata(fields map[string]jobdb.ChapterMetadataValue) (map[string]any, error) {
	result := make(map[string]any, len(fields))
	for key, v := range fields {
		value, err := exportMetadataValue(v)
		if err != nil {
			return nil, err
		}
		result[key] = value
	}
	return result, nil
}

func exportMetadataValue(v jobdb.ChapterMetadataValue) (any, error) {
	var value any
	switch v.Kind {
	case jobdb.ChapterMetadataNull:
	case jobdb.ChapterMetadataBool:
		value = v.Bool
	case jobdb.ChapterMetadataInt:
		value = v.Int
	case jobdb.ChapterMetadataDouble:
		value = v.Double
	case jobdb.ChapterMetadataString:
		value = v.String
	case jobdb.ChapterMetadataMap:
		m, err := exportMetadata(v.Map)
		if err != nil {
			return nil, err
		}
		value = m
	case jobdb.ChapterMetadataList:
		list := make([]any, 0, len(v.List))
		for _, item := range v.List {
			entry, err := exportMetadataValue(item)
			if err != nil {
				return nil, err
			}
			list = append(list, entry)
		}
		value = list
	default:
		return nil, fmt.Errorf("unsupported chapter metadata kind %q", v.Kind)
	}
	return map[string]any{"kind": v.Kind, "value": value}, nil
}

func encodeChapterExport(tenant, job string, chapters []jobdb.Chapter, now time.Time) ([]byte, error) {
	result := chapterExport{FormatVersion: 1, ProjectID: tenant, JobID: job, ExportedAt: now.UTC(), Chapters: make([]exportedChapter, 0, len(chapters))}
	for _, c := range chapters {
		body, err := exportBody(c.Body)
		if err != nil {
			return nil, fmt.Errorf("export chapter %d: %w", c.Ordinal, err)
		}
		metadata, err := exportMetadata(c.Metadata.Fields)
		if err != nil {
			return nil, fmt.Errorf("export chapter %d: %w", c.Ordinal, err)
		}
		artifacts := make([]exportedArtifact, 0, len(c.Artifacts))
		for _, a := range c.Artifacts {
			artifacts = append(artifacts, exportedArtifact{Name: a.Name, Digest: a.Digest, Size: a.Size})
		}
		result.Chapters = append(result.Chapters, exportedChapter{Ordinal: c.Ordinal, TaskType: c.TaskType, CreatedAt: c.CreatedAt, InputHash: c.InputHash, Metadata: metadata, Artifacts: artifacts, Body: body})
	}
	sort.Slice(result.Chapters, func(i, j int) bool { return result.Chapters[i].Ordinal < result.Chapters[j].Ordinal })
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func chapterDownloadFilename(job string) string {
	safe := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, job)
	safe = strings.Trim(safe, "_")
	// Bound header and filesystem filename length even for multibyte identifiers.
	if runes := []rune(safe); len(runes) > 40 {
		safe = string(runes[:40])
	}
	if safe == "" {
		safe = "export"
	}
	return "job-" + safe + "-chapters.json"
}

func (s *Server) handleChapterDownload(w http.ResponseWriter, r *http.Request) {
	if s.chapters == nil {
		writeError(w, fmt.Errorf("chapter reading unavailable"))
		return
	}
	tenant, job := s.tenantID(r), mux.Vars(r)["jobId"]
	chapters, err := s.chapters.ListChapters(r.Context(), jobdb.ListChaptersRequest{JobKey: jobdb.JobKey{TenantId: tenant, JobId: job}})
	if err != nil {
		writeError(w, err)
		return
	}
	data, err := encodeChapterExport(tenant, job, chapters, time.Now())
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": chapterDownloadFilename(job)}))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
