package cortex

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/colony-2/jobdb/pkg/jobdb"
)

func adminRequest(t *testing.T, server *httptest.Server, path string, body any, want int) string {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(server.URL+path, "application/json", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	if response.StatusCode != want {
		t.Fatalf("status %d, want %d: %s", response.StatusCode, want, data)
	}
	return string(data)
}
func TestAdminConfigurationAndCellPolicies(t *testing.T) {
	srv, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t)})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	base := "/api/projects/tenant/admin"
	get := func() adminSnapshot { return getJSON[adminSnapshot](t, ts.URL+base) }
	snap := get()
	adminRequest(t, ts, base+"/configs", map[string]any{"name": "answer", "operation": "set", "value": 42, "revision": snap.Revision}, 200)
	adminRequest(t, ts, base+"/configs", map[string]any{"name": "stale", "operation": "set", "value": true, "revision": snap.Revision}, 409)
	snap = get()
	if len(snap.Configs) != 1 || snap.Configs[0].Name != "answer" {
		t.Fatalf("configs = %#v", snap.Configs)
	}
	other := getJSON[adminSnapshot](t, ts.URL+"/api/projects/other/admin")
	if len(other.Configs) != 0 {
		t.Fatal("tenant configuration leaked")
	}
	adminRequest(t, ts, base+"/configs", map[string]any{"name": "_c2j_hidden", "operation": "set", "value": true, "revision": snap.Revision}, 400)
	adminRequest(t, ts, base+"/configs", map[string]any{"name": "missing-revision", "operation": "set", "value": true}, 400)
	cell := postJSON[jobdb.Cell](t, ts.URL+"/api/projects/tenant/cells", map[string]string{"name": "app", "repo": "github.com/acme/app"})
	entry := map[string]any{"perms": map[string]any{"jobdb": map[string]any{"secrets": map[string]any{"resolve": []string{"token"}}}}, "source_trust": map[string]any{"jobdb.secrets.resolve": map[string]any{"token": map[string]string{"mode": "candidate"}}}}
	adminRequest(t, ts, base+"/hooks", map[string]any{"name": ".c2/hooks/build.yaml", "operation": "set", "value": entry, "revision": snap.Revision}, 200)
	scoped := base + "?cellId=" + cell.ID
	cs := getJSON[adminSnapshot](t, ts.URL+scoped)
	if len(cs.Hooks) != 1 || cs.Hooks[0].Source != "tenant" || len(cs.SecretReferences) != 1 {
		t.Fatalf("inherited hooks = %#v", cs)
	}
	adminRequest(t, ts, base+"/configs?cellId="+cell.ID, map[string]any{"name": "answer", "operation": "set", "value": 3, "revision": cs.Revision}, 400)
	adminRequest(t, ts, base+"/hooks?cellId="+cell.ID, map[string]any{"name": ".c2/hooks/build.yaml", "operation": "clear", "revision": cs.Revision}, 200)
	cs = getJSON[adminSnapshot](t, ts.URL+scoped)
	if cs.Hooks[0].Source != "cell" || len(cs.SecretReferences) != 0 {
		t.Fatalf("clear did not deny inherited grants: %#v", cs)
	}
	adminRequest(t, ts, base+"/hooks?cellId="+cell.ID, map[string]any{"name": ".c2/hooks/build.yaml", "operation": "remove", "revision": cs.Revision}, 200)
	cs = getJSON[adminSnapshot](t, ts.URL+scoped)
	if cs.Hooks[0].Source != "tenant" || len(cs.SecretReferences) != 1 {
		t.Fatal("inheritance not restored")
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/projects/other/admin?cellId="+cell.ID, nil)
	doJSON[errorResponse](t, req, 404)
	adminRequest(t, ts, "/api/projects/other/admin/hooks?cellId="+cell.ID, map[string]any{"name": ".c2/hooks/build.yaml", "operation": "clear", "revision": cs.Revision}, 404)
	adminRequest(t, ts, base+"/hooks", map[string]any{"name": ".c2/hooks/bad.yaml", "operation": "set", "value": map[string]any{"perms": entry["perms"]}, "revision": cs.Revision}, 400)
	// Recipe callsites may inherit permissions; administrative hook policies
	// must remain complete grants, separate from cell-to-tenant inheritance.
	adminRequest(t, ts, base+"/hooks", map[string]any{"name": ".c2/hooks/inherit.yaml", "operation": "set", "value": map[string]any{"perms": map[string]any{"inherit": true}}, "revision": cs.Revision}, 400)
	profile := map[string]any{"version": 1, "entries": []any{map[string]string{"url": "https://github.com/acme/", "username": "git", "secret_ref": "git_token"}}}
	adminRequest(t, ts, base+"/git", map[string]any{"name": "read", "operation": "set", "value": profile, "revision": cs.Revision}, 200)
	snap = get()
	if len(snap.Git) != 1 || len(snap.SecretReferences) != 2 || len(snap.Configs) != 1 {
		t.Fatalf("profile categorization = %#v", snap)
	}
	adminRequest(t, ts, base+"/git", map[string]any{"name": "unsafe", "operation": "set", "value": map[string]any{"version": 1, "entries": []any{map[string]string{"url": "https://github.com", "username": "git", "secret_file": "/etc/secret"}}}, "revision": snap.Revision}, 400)
	adminRequest(t, ts, base+"/git", map[string]any{"name": "read", "operation": "remove", "revision": snap.Revision}, 200)
	snap = get()
	adminRequest(t, ts, base+"/configs", map[string]any{"name": "answer", "operation": "remove", "revision": snap.Revision}, 200)
	if len(get().Configs) != 0 {
		t.Fatal("configuration removal failed")
	}
}

type recordingSecrets struct {
	tenant, name, value string
	err                 error
}

func (s *recordingSecrets) SetDevSecret(_ context.Context, tenant, name, value string) error {
	s.tenant, s.name, s.value = tenant, name, value
	return s.err
}
func (s *recordingSecrets) RemoveDevSecret(_ context.Context, tenant, name string) error {
	s.tenant, s.name, s.value = tenant, name, ""
	return s.err
}
func TestAdminSecretsNeverReturned(t *testing.T) {
	srv, err := NewUITestServer(Config{WorkingDir: writeC2JConfig(t)})
	if err != nil {
		t.Fatal(err)
	}
	store := &recordingSecrets{}
	srv.secretAdmin = store
	ts := httptest.NewServer(srv)
	defer ts.Close()
	base := "/api/projects/tenant/admin"
	body := map[string]any{"name": "token", "operation": "set", "value": "private-value"}
	response := adminRequest(t, ts, base+"/secrets", body, 204)
	if response != "" || store.tenant != "tenant" || store.value != "private-value" {
		t.Fatal("secret write failed")
	}
	snap := getJSON[adminSnapshot](t, ts.URL+base)
	raw, _ := json.Marshal(snap)
	if strings.Contains(string(raw), "private-value") || len(snap.SecretReferences) != 0 {
		t.Fatal("secret value or invented inventory leaked")
	}
	store.err = errors.New("provider echoed private-value")
	if strings.Contains(adminRequest(t, ts, base+"/secrets", body, 502), "private-value") {
		t.Fatal("secret error leaked")
	}
	store.err = nil
	adminRequest(t, ts, base+"/secrets", map[string]any{"name": "token", "operation": "remove"}, 204)
	if store.value != "" {
		t.Fatal("secret remove failed")
	}
	adminRequest(t, ts, base+"/secrets", map[string]any{"name": "token", "operation": "set", "value": nil}, 400)
}
func TestRemoteSecretAdminDoesNotFollowRedirect(t *testing.T) {
	reached := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached = true }))
	defer target.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dev/v1/tenants/tenant/secrets/set" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		http.Redirect(w, r, target.URL, 307)
	}))
	defer upstream.Close()
	admin := remoteSecretAdmin{base: upstream.URL, client: upstream.Client()}
	if admin.SetDevSecret(context.Background(), "tenant", "token", "private-value") == nil || reached {
		t.Fatal("secret followed redirect")
	}
}

func TestRemoteServerSecretAdministration(t *testing.T) {
	var stored map[string]string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dev/v1/tenants/tenant/secrets/set" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&stored); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	srv, err := NewRemoteServer(Config{JobDBURL: upstream.URL, WorkingDir: writeC2JConfig(t), HTTPClient: upstream.Client()})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	adminRequest(t, ts, "/api/projects/tenant/admin/secrets", map[string]any{"name": "token", "operation": "set", "value": "secret"}, 204)
	if stored["name"] != "token" || stored["value"] != "secret" {
		t.Fatal("remote write did not reach tenant store")
	}
}
