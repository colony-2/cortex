package cortex

import (
	"encoding/json"
	"net/http/httptest"
	"runtime/debug"
	"strings"
	"testing"
)

func TestLinkedBuildInformation(t *testing.T) {
	for _, tc := range []struct {
		name, version, commit string
		replacement           *debug.Module
		effective             string
	}{
		{name: "pseudo", version: "v0.0.0-20260101120000-abcdef123456", commit: "abcdef123456"},
		{name: "tag", version: "v1.2.3"},
		{name: "versioned replacement", version: "v1.0.0", replacement: &debug.Module{Path: "example.com/fork", Version: "v1.2.4-0.20260101120000-123456abcdef"}, effective: "example.com/fork", commit: "123456abcdef"},
		{name: "local replacement", version: "v1.0.0", replacement: &debug.Module{Path: "/private/source"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := linkedBuildInformation(&debug.BuildInfo{Deps: []*debug.Module{{Path: "github.com/colony-2/c2j", Version: tc.version, Sum: "h1:not-a-commit", Replace: tc.replacement}}})
			got := result.Libraries["c2j"]
			wantVersion := tc.version
			if tc.replacement != nil {
				wantVersion = tc.replacement.Version
				if wantVersion == "" {
					wantVersion = "local/development"
				}
			}
			if got.Version == nil || *got.Version != wantVersion {
				t.Fatalf("version: %+v", got)
			}
			if tc.commit == "" {
				if got.Commit != nil {
					t.Fatal("invented revision")
				}
			} else if got.Commit == nil || *got.Commit != tc.commit {
				t.Fatal("missing revision")
			}
			if tc.effective != "" && got.Module != tc.effective {
				t.Fatal("wrong effective identity")
			}
			bytes, _ := json.Marshal(result)
			if strings.Contains(string(bytes), "/private") || strings.Contains(string(bytes), "h1:") {
				t.Fatal("leaked path or checksum")
			}
		})
	}
	for _, info := range []*debug.BuildInfo{nil, {}} {
		for _, lib := range linkedBuildInformation(info).Libraries {
			if lib.Version != nil || lib.Commit != nil {
				t.Fatal("missing metadata must stay unknown")
			}
		}
	}
}
func TestBuildInfoEndpoint(t *testing.T) {
	srv, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t), DefaultTenantID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/api/build-info", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	srv.ServeHTTP(response, request)
	if response.Code != 200 || response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("response: %v", response)
	}
	var value buildInformation
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil || len(value.Libraries) != 2 {
		t.Fatalf("body: %s", response.Body)
	}
	response = httptest.NewRecorder()
	srv.ServeHTTP(response, httptest.NewRequest("POST", "/api/build-info", nil))
	if response.Code == 200 {
		t.Fatal("endpoint is read-only")
	}
}
