package cortex

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/colony-2/jobdb/pkg/jobdb"
)

func TestChapterExportVariants(t *testing.T) {
	binary := []byte{0, 255, 128, 10}
	outcomes := []struct {
		name    string
		outcome jobdb.ChapterOutcome
		want    string
	}{
		{"output", jobdb.ApplicationOutputOutcome{Output: jobdb.ApplicationOutputBytes{Data: binary}}, `"data": "AP+ACg=="`},
		{"app_error", jobdb.AppErrorOutcome{Error: jobdb.AppErrorPayload{Message: "錯誤", Attrs: map[string]any{"integer": int64(9007199254740993)}}}, `9007199254740993`},
		{"system_error", jobdb.SystemErrorOutcome{Error: jobdb.SystemErrorPayload{Message: "broken", Code: "io", Retryable: true}}, `"code": "io"`},
		{"timeout", jobdb.TimeoutOutcome{Timeout: jobdb.TimeoutPayload{Scope: "total", Retryable: true, Message: "late"}}, `"scope": "total"`},
	}
	for _, tc := range outcomes {
		for _, task := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{true: "/task", false: "/job"}[task], func(t *testing.T) {
				var body jobdb.ChapterBody = jobdb.JobAttemptOutcomeChapter{Outcome: tc.outcome}
				kind := "job_attempt_outcome"
				if task {
					body = jobdb.TaskAttemptOutcomeChapter{Outcome: tc.outcome}
					kind = "task_attempt_outcome"
				}
				data, err := encodeChapterExport("tenant", "job", []jobdb.Chapter{{Ordinal: 3, Body: body}}, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Contains(data, []byte(tc.want)) || !bytes.Contains(data, []byte(`"kind": "`+kind+`"`)) {
					t.Fatalf("unexpected export: %s", data)
				}
				var envelope chapterExport
				if err := json.Unmarshal(data, &envelope); err != nil {
					t.Fatal(err)
				}
				outcome := envelope.Chapters[0].Body.(map[string]any)["outcome"].(map[string]any)
				expectedKind := tc.name
				if tc.name == "output" {
					expectedKind = "application_output"
					output := outcome["output"].(map[string]any)
					decoded, err := base64.StdEncoding.DecodeString(output["data"].(string))
					if err != nil || output["encoding"] != "base64" || !bytes.Equal(decoded, binary) {
						t.Fatalf("payload bytes: %#v, %v", output, err)
					}
				}
				if outcome["kind"] != expectedKind {
					t.Fatalf("outcome kind: %#v", outcome)
				}
			})
		}
	}
	for _, tc := range []struct {
		body jobdb.ChapterBody
		want string
	}{
		{jobdb.JobStartChapter{Input: jobdb.ApplicationInputBytes{Data: binary}}, `"kind": "job_start"`},
		{jobdb.RestartExtraChapter{Output: jobdb.ApplicationOutputBytes{Data: nil}}, `"data": ""`},
		{jobdb.ExecutionChapter{Record: json.RawMessage(`{"kind":"restart","descriptor":[{"kind":"scope"}],"big":9007199254740993}`)}, `9007199254740993`},
	} {
		data, err := encodeChapterExport("tenant", "job", []jobdb.Chapter{{Body: tc.body}}, time.Now())
		if err != nil || !bytes.Contains(data, []byte(tc.want)) {
			t.Fatalf("export %T: %s, %v", tc.body, data, err)
		}
	}
}

func TestChapterExportEnvelopeAndMetadata(t *testing.T) {
	now := time.Date(2026, 10, 28, 2, 0, 0, 0, time.FixedZone("offset", 3600))
	fields := map[string]jobdb.ChapterMetadataValue{
		"null":   {Kind: jobdb.ChapterMetadataNull},
		"bool":   {Kind: jobdb.ChapterMetadataBool, Bool: true},
		"int":    {Kind: jobdb.ChapterMetadataInt, Int: 9007199254740993},
		"double": {Kind: jobdb.ChapterMetadataDouble, Double: 1.25},
		"string": {Kind: jobdb.ChapterMetadataString, String: "雪"},
		"list":   {Kind: jobdb.ChapterMetadataList, List: []jobdb.ChapterMetadataValue{{Kind: jobdb.ChapterMetadataInt, Int: 7}}},
		"map":    {Kind: jobdb.ChapterMetadataMap, Map: map[string]jobdb.ChapterMetadataValue{"key": {Kind: jobdb.ChapterMetadataBool, Bool: false}}},
	}
	chapters := []jobdb.Chapter{
		{Ordinal: 9, TaskType: "task", CreatedAt: now, InputHash: "hash", Body: jobdb.JobStartChapter{}, Metadata: jobdb.ChapterMetadata{Fields: fields}, Artifacts: []jobdb.StoredArtifact{{Name: "雪.txt", Digest: "sha256:abc", Size: 9007199254740993}}},
		{Ordinal: 1, Body: jobdb.RestartExtraChapter{}},
	}
	data, err := encodeChapterExport("tenant", "job", chapters, now)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		FormatVersion int    `json:"format_version"`
		ProjectID     string `json:"project_id"`
		JobID         string `json:"job_id"`
		ExportedAt    string `json:"exported_at"`
		Chapters      []struct {
			Ordinal   int64     `json:"ordinal"`
			TaskType  string    `json:"task_type"`
			CreatedAt time.Time `json:"created_at"`
			InputHash string    `json:"input_hash"`
			Metadata  map[string]struct {
				Kind  string          `json:"kind"`
				Value json.RawMessage `json:"value"`
			} `json:"metadata"`
			Artifacts []exportedArtifact `json:"artifacts"`
		} `json:"chapters"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.FormatVersion != 1 || result.ProjectID != "tenant" || result.JobID != "job" || result.ExportedAt != "2026-10-28T01:00:00Z" {
		t.Fatalf("envelope: %s", data)
	}
	if result.Chapters[0].Ordinal != 1 || result.Chapters[1].Ordinal != 9 || chapters[0].Ordinal != 9 {
		t.Fatalf("ordering: %s", data)
	}
	c := result.Chapters[1]
	if c.TaskType != "task" || !c.CreatedAt.Equal(now) || c.InputHash != "hash" || c.Metadata["int"].Kind != "int" || string(c.Metadata["int"].Value) != "9007199254740993" || len(c.Metadata) != 7 {
		t.Fatalf("metadata: %s", data)
	}
	if c.Artifacts[0].Name != "雪.txt" || c.Artifacts[0].Size != 9007199254740993 || c.Artifacts[0].Digest != "sha256:abc" {
		t.Fatalf("artifacts: %s", data)
	}
	for _, want := range []string{`"kind": "list"`, `"kind": "map"`, `"value": "雪"`, `"value": 1.25`, `"value": null`, `"value": true`, `"value": false`, `"value": 7`} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("missing %s: %s", want, data)
		}
	}
	empty, err := encodeChapterExport("tenant", "job", nil, now)
	if err != nil || !bytes.Contains(empty, []byte(`"chapters": []`)) {
		t.Fatalf("empty: %s, %v", empty, err)
	}
}

func TestChapterExportRejectsInvalidRecords(t *testing.T) {
	for _, c := range []jobdb.Chapter{
		{Body: nil},
		{Body: jobdb.TaskAttemptOutcomeChapter{}},
		{Body: jobdb.ExecutionChapter{Record: json.RawMessage(`{"bad":`)}},
		{Body: jobdb.JobStartChapter{}, Metadata: jobdb.ChapterMetadata{Fields: map[string]jobdb.ChapterMetadataValue{"bad": {Kind: "future"}}}},
		{Body: jobdb.JobStartChapter{}, Metadata: jobdb.ChapterMetadata{Fields: map[string]jobdb.ChapterMetadataValue{"bad": {Kind: jobdb.ChapterMetadataDouble, Double: math.NaN()}}}},
	} {
		data, err := encodeChapterExport("tenant", "job", []jobdb.Chapter{c}, time.Now())
		if err == nil || data != nil {
			t.Fatalf("invalid record produced %s: %v", data, err)
		}
	}
}

func TestChapterDownloadFilename(t *testing.T) {
	for _, job := range []string{"normal-job", "雪", "../a\\b\r\n\x00", "", "../", strings.Repeat("雪", 300)} {
		name := chapterDownloadFilename(job)
		if strings.ContainsAny(name, "/\\\r\n\x00") || !strings.HasPrefix(name, "job-") || !strings.HasSuffix(name, "-chapters.json") || len(name) > 180 {
			t.Fatalf("unsafe filename %q", name)
		}
	}
	if chapterDownloadFilename("") != "job-export-chapters.json" {
		t.Fatal("missing safe fallback")
	}
}

// E01: representation must not change the version-one export contract.
func TestChapterExportPointerVariants(t *testing.T) {
	start := jobdb.JobStartChapter{Input: jobdb.ApplicationInputBytes{Data: []byte{0, 255}}}
	extra := jobdb.RestartExtraChapter{Output: jobdb.ApplicationOutputBytes{Data: []byte("雪")}}
	execution := jobdb.ExecutionChapter{Record: json.RawMessage(`{"big":9007199254740993}`)}
	for _, tc := range []struct {
		name           string
		value, pointer jobdb.ChapterBody
	}{
		{"job_start", start, &start}, {"restart_extra", extra, &extra}, {"execution", execution, &execution},
	} {
		t.Run(tc.name, func(t *testing.T) { assertChapterPointerExport(t, tc.value, tc.pointer) })
	}
	output := jobdb.ApplicationOutputOutcome{Output: jobdb.ApplicationOutputBytes{Data: []byte{0, 255}}}
	appError := jobdb.AppErrorOutcome{Error: jobdb.AppErrorPayload{Message: "錯誤", Attrs: map[string]any{"integer": int64(9007199254740993)}}}
	systemError := jobdb.SystemErrorOutcome{Error: jobdb.SystemErrorPayload{Message: "broken", Code: "io", Retryable: true}}
	timeout := jobdb.TimeoutOutcome{Timeout: jobdb.TimeoutPayload{Scope: "total", Retryable: true, Message: "late"}}
	for _, tc := range []struct {
		name           string
		value, pointer jobdb.ChapterOutcome
	}{
		{"output", output, &output}, {"app_error", appError, &appError},
		{"system_error", systemError, &systemError}, {"timeout", timeout, &timeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, outcome := range []jobdb.ChapterOutcome{tc.value, tc.pointer} {
				job := jobdb.JobAttemptOutcomeChapter{Outcome: outcome}
				task := jobdb.TaskAttemptOutcomeChapter{Outcome: outcome}
				// Check both parent representations and nested outcome representations.
				assertChapterPointerExport(t, jobdb.JobAttemptOutcomeChapter{Outcome: tc.value}, job)
				assertChapterPointerExport(t, jobdb.JobAttemptOutcomeChapter{Outcome: tc.value}, &job)
				assertChapterPointerExport(t, jobdb.TaskAttemptOutcomeChapter{Outcome: tc.value}, task)
				assertChapterPointerExport(t, jobdb.TaskAttemptOutcomeChapter{Outcome: tc.value}, &task)
			}
		})
	}
}

func assertChapterPointerExport(t *testing.T, value, pointer jobdb.ChapterBody) {
	t.Helper()
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	baseline, err := encodeChapterExport("tenant", "job", []jobdb.Chapter{{Ordinal: 1, Body: value}}, now)
	if err != nil {
		t.Fatalf("value export: %v", err)
	}
	actual, err := encodeChapterExport("tenant", "job", []jobdb.Chapter{{Ordinal: 1, Body: pointer}}, now)
	if err != nil {
		t.Fatalf("%T export: %v", pointer, err)
	}
	if !bytes.Equal(actual, baseline) {
		t.Fatalf("%T representation changed export:\n%s\nwant:\n%s", pointer, actual, baseline)
	}
}

func TestChapterExportRejectsTypedNil(t *testing.T) {
	bodies := []jobdb.ChapterBody{
		(*jobdb.JobStartChapter)(nil), (*jobdb.RestartExtraChapter)(nil), (*jobdb.ExecutionChapter)(nil),
		(*jobdb.JobAttemptOutcomeChapter)(nil), (*jobdb.TaskAttemptOutcomeChapter)(nil),
	}
	for _, outcome := range []jobdb.ChapterOutcome{
		(*jobdb.ApplicationOutputOutcome)(nil), (*jobdb.AppErrorOutcome)(nil),
		(*jobdb.SystemErrorOutcome)(nil), (*jobdb.TimeoutOutcome)(nil),
	} {
		job := jobdb.JobAttemptOutcomeChapter{Outcome: outcome}
		task := jobdb.TaskAttemptOutcomeChapter{Outcome: outcome}
		bodies = append(bodies, job, &job, task, &task)
	}
	for _, body := range bodies {
		data, err := encodeChapterExport("tenant", "job", []jobdb.Chapter{{Body: body}}, time.Now())
		if err == nil || data != nil {
			t.Fatalf("typed nil in %T produced %s: %v", body, data, err)
		}
	}
}
