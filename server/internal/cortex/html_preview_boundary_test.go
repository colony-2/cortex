package cortex

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// This diagnostic records why an opaque preview origin is not an API boundary.
// Remove/revise this baseline when API browser-principal enforcement is added;
// it does not assert that allowing untrusted cancellation is desirable.
func TestHTMLPreviewBoundaryBaseline(t *testing.T) {
	srv, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t)})
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(srv)
	defer api.Close()
	postJSON[map[string]interface{}](t, api.URL+"/api/projects/fixture/cells", map[string]string{"name": "self", "repo": "github.com/acme/boo-self"})
	for _, id := range []string{"control", "opaque"} {
		postJSON[map[string]interface{}](t, api.URL+"/api/projects/fixture/jobs", map[string]interface{}{"job_id": id, "recipe": "smoke", "cell": "self", "repo": "github.com/acme/boo-self"})
	}
	for _, id := range []string{"control", "opaque"} {
		req, err := http.NewRequest(http.MethodPost, api.URL+"/api/projects/fixture/jobs/"+id+"/cancel", nil)
		if err != nil {
			t.Fatal(err)
		}
		if id == "opaque" {
			req.Header.Set("Origin", "null")
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			req.Header.Set("Sec-Fetch-Mode", "no-cors")
		}
		resp, err := api.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("%s status = %d", id, resp.StatusCode)
		}
		state := getJSON[map[string]interface{}](t, api.URL+"/api/projects/fixture/jobs/"+id)
		if state["status"] != "CANCELLED" {
			t.Fatalf("%s state = %#v", id, state)
		}
		t.Logf("%s: cookie=%q authorization=%q origin=%q response=%d finalStatus=%s allowOrigin=%q", id, req.Header.Get("Cookie"), req.Header.Get("Authorization"), req.Header.Get("Origin"), resp.StatusCode, state["status"], resp.Header.Get("Access-Control-Allow-Origin"))
	}
}
