package cortex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"testing"
)

type goalPart struct {
	field, name, data string
	binary            bool
}

func TestGoalAttachmentDecode(t *testing.T) {
	valid := goalPart{field: "submission", data: `{"cell":"platform","inputs":{"prompt":"hello","type":"build"}}`}
	file := func(name, data string) goalPart {
		return goalPart{field: "files", name: name, data: data, binary: true}
	}
	tests := []struct {
		name   string
		parts  []goalPart
		status int
	}{
		{"ATT-023 binary Unicode empty", []goalPart{valid, file("雪.bin", "\x00\xff"), file("empty", ""), file("A", "a"), file("a", "b")}, 0},
		{"ATT-021 browser escaped names", []goalPart{valid, {field: "filenames", data: `["a\"b.txt","a%22b.txt"]`}, file("a%22b.txt", "quoted"), file("a%22b.txt", "percent")}, 0},
		{"ATT-039 malformed names", []goalPart{valid, {field: "filenames", data: `[`}, file("0", "x")}, 400},
		{"ATT-039 non-string names", []goalPart{valid, {field: "filenames", data: `[1]`}, file("0", "x")}, 400},
		{"ATT-042 literal legacy names", []goalPart{valid, file(`a"b.txt`, "quoted"), file("a%22b.txt", "percent")}, 0},
		{"ATT-044 ordinary spaces", []goalPart{valid, file("my brief 雪.txt", "space")}, 0},
		{"ATT-041 recovered control", []goalPart{valid, {field: "filenames", data: `["a\u0001b"]`}, file("0", "x")}, 400},
		{"ATT-024 invalid names JSON", []goalPart{valid, {field: "filenames", data: `["x"] {}`}, file("0", "x")}, 400},
		{"ATT-024 null names", []goalPart{valid, {field: "filenames", data: `null`}, file("0", "x")}, 400},
		{"ATT-024 mismatched names", []goalPart{valid, {field: "filenames", data: `[]`}, file("0", "x")}, 400},
		{"ATT-024 repeated names", []goalPart{valid, {field: "filenames", data: `["x"]`}, {field: "filenames", data: `["x"]`}, file("0", "x")}, 400},
		{"ATT-026 unsafe MIME with names", []goalPart{valid, {field: "filenames", data: `["safe"]`}, file("../x", "x")}, 400},
		{"ATT-026 unsafe recovered name", []goalPart{valid, {field: "filenames", data: `["../x"]`}, file("0", "x")}, 400},
		{"ATT-027 duplicate recovered names", []goalPart{valid, {field: "filenames", data: `["x","x"]`}, file("0", "a"), file("1", "b")}, 400},
		{"ATT-024 missing", []goalPart{file("x", "x")}, 400},
		{"ATT-024 repeated", []goalPart{valid, valid}, 400},
		{"ATT-024 unknown metadata", []goalPart{{field: "submission", data: `{"unknown":true}`}}, 400},
		{"ATT-024 trailing JSON", []goalPart{{field: "submission", data: `{} {}`}}, 400},
		{"ATT-024 oversized metadata", []goalPart{{field: "submission", data: strings.Repeat(" ", 1<<20) + "{}"}}, 413},
		{"ATT-025 unknown text", []goalPart{valid, {field: "other", data: "x"}}, 400},
		{"ATT-025 text files", []goalPart{valid, {field: "files", data: "x"}}, 400},
		{"ATT-025 binary submission", []goalPart{valid, {field: "submission", name: "x", binary: true}}, 400},
		{"ATT-025 unknown file", []goalPart{valid, {field: "other", name: "x", binary: true}}, 400},
		{"ATT-027 duplicates", []goalPart{valid, file("x", "1"), file("x", "2")}, 400},
		{"ATT-008 exact single", []goalPart{valid, file("big", strings.Repeat("a", goalFileBytes))}, 0},
		{"ATT-029 file overflow", []goalPart{valid, file("big", strings.Repeat("a", goalFileBytes+1))}, 413},
		{"ATT-029 body overflow", []goalPart{valid, file("big", strings.Repeat("a", 33<<20))}, 413},
	}
	for _, name := range []string{"../x", "/x", "a/b", `a\b`, "C:x", "=x", ".", "..", " ", ""} {
		tests = append(tests, struct {
			name   string
			parts  []goalPart
			status int
		}{"ATT-026 " + name, []goalPart{valid, file(name, "x")}, 400})
	}
	ten := []goalPart{valid}
	for i := 0; i < 10; i++ {
		data := ""
		if i == 0 {
			data = strings.Repeat("a", goalFileBytes)
		}
		ten = append(ten, file(fmt.Sprint(i), data))
	}
	tests = append(tests, struct {
		name   string
		parts  []goalPart
		status int
	}{"ATT-007 exact ten", ten, 0}, struct {
		name   string
		parts  []goalPart
		status int
	}{"ATT-007 eleven", append(append([]goalPart{}, ten...), file("eleven", "")), 400})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("TMPDIR", dir)
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			for _, part := range tt.parts {
				if !part.binary {
					if err := writer.WriteField(part.field, part.data); err != nil {
						t.Fatal(err)
					}
					continue
				}
				header := textproto.MIMEHeader{}
				header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, part.field, part.name))
				header.Set("Content-Type", "application/octet-stream")
				out, err := writer.CreatePart(header)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = io.WriteString(out, part.data)
			}
			_ = writer.Close()
			req := httptest.NewRequest("POST", "/", &body)
			req.ContentLength = -1
			req.Header.Set("Content-Type", writer.FormDataContentType())
			var input submitJobRequest
			artifacts, status, err := decodeGoalSubmission(httptest.NewRecorder(), req, &input)
			if status != tt.status || (err != nil) != (tt.status != 0) {
				t.Fatalf("status %d error %v, want %d", status, err, tt.status)
			}
			if err == nil {
				var names []string
				for _, p := range tt.parts {
					if p.binary {
						names = append(names, p.name)
					}
				}
				for _, p := range tt.parts {
					if p.field == "filenames" {
						if err := json.Unmarshal([]byte(p.data), &names); err != nil {
							t.Fatal(err)
						}
					}
				}
				if len(artifacts) != len(names) {
					t.Fatal("lost files")
				}
				for i, a := range artifacts {
					if a.Name() != names[i] {
						t.Fatal("changed name")
					}
					reader, e := a.Open()
					if e != nil {
						t.Fatal(e)
					}
					data, e := io.ReadAll(reader)
					reader.Close()
					if e != nil {
						t.Fatal(e)
					}
					fileIndex := 0
					for _, p := range tt.parts {
						if p.binary {
							if fileIndex == i && string(data) != p.data {
								t.Fatal("changed bytes")
							}
							fileIndex++
						}
					}
				}
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 0 {
				t.Fatal("temporary files leaked")
			}
		})
	}
}

func TestGoalAttachmentMediaAndMalformed(t *testing.T) {
	for _, tt := range []struct {
		media, body string
		want        int
	}{{"text/plain", "{}", 415}, {"multipart/form-data; boundary=x", "broken", 400}, {"application/json", `{"cell":"x"}`, 400}, {"application/json", `{"unknown":true}`, 400}} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(tt.body))
		r.Header.Set("Content-Type", tt.media)
		var req submitJobRequest
		_, status, err := decodeGoalSubmission(httptest.NewRecorder(), r, &req)
		if status != tt.want {
			t.Fatalf("%s %d %v", tt.media, status, err)
		}
		if tt.body == `{"cell":"x"}` && err != nil {
			t.Fatal(err)
		}
	}
}

func TestGoalAttachmentPartialParseCleanup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	_ = w.WriteField("submission", `{"cell":"platform"}`)
	part, _ := w.CreateFormFile("files", "large")
	_, _ = part.Write(bytes.Repeat([]byte("x"), 7<<20))
	_ = w.Close()
	// A transport failure after spill must clean up partially parsed disk files.
	raw := body.Bytes()
	r := httptest.NewRequest("POST", "/", bytes.NewReader(raw[:len(raw)-100]))
	r.Header.Set("Content-Type", w.FormDataContentType())
	var req submitJobRequest
	_, status, err := decodeGoalSubmission(httptest.NewRecorder(), r, &req)
	if status != 400 || err == nil {
		t.Fatalf("%d %v", status, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("partial parse leaked files")
	}
}

func TestGoalAttachmentLegacyJSON(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		invalid    bool
	}{
		{"valid", `{"cell":"platform","inputs":{"prompt":"hello","type":"build"}}`, false},
		{"unknown", `{"unknown":true}`, true},
		{"trailing", `{} {}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
			var req submitJobRequest
			artifacts, _, err := decodeGoalSubmission(httptest.NewRecorder(), r, &req)
			if (err != nil) != tc.invalid || len(artifacts) != 0 {
				t.Fatalf("artifacts %v error %v", artifacts, err)
			}
			if !tc.invalid && (req.Cell != "platform" || req.Inputs["prompt"] != "hello") {
				t.Fatalf("metadata changed: %+v", req)
			}
		})
	}
}
