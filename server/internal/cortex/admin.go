package cortex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/colony-2/c2j/pkg/gitprofile"
	"github.com/colony-2/c2j/pkg/hookpolicy"
	"github.com/colony-2/jobdb/pkg/jobdb"
	"github.com/gorilla/mux"
)

type adminEntry struct {
	Name   string `json:"name"`
	Value  any    `json:"value,omitempty"`
	Source string `json:"source,omitempty"`
}
type adminSnapshot struct {
	Revision         string       `json:"revision"`
	Hooks            []adminEntry `json:"hooks"`
	Configs          []adminEntry `json:"configs"`
	Git              []adminEntry `json:"git"`
	SecretReferences []string     `json:"secretReferences"`
	SecretWrites     bool         `json:"secretWrites"`
	Repository       string       `json:"repository,omitempty"`
}

// Scope comes exclusively from the URL tenant and registry cell ID, never a
// caller-supplied repository or internal configuration key.
func (s *Server) adminScope(r *http.Request) (string, error) {
	if id := r.URL.Query().Get("cellId"); id != "" {
		cell, err := s.cells.GetCell(r.Context(), jobdb.CellKey{TenantID: s.tenantID(r), CellID: id})
		if err != nil {
			return "", err
		}
		return cell.Repo, nil
	}
	return "", nil
}
func (s *Server) handleAdminRead(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	repo, err := s.adminScope(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if s.configuration == nil {
		writeHTTPError(w, 501, errors.New("configuration administration is unavailable"))
		return
	}
	snapshot, err := s.configuration.GetTenantConfiguration(r.Context(), s.tenantID(r))
	if err != nil {
		writeAdminError(w, err)
		return
	}
	policy, err := hookpolicy.Capture(snapshot, repo)
	if err != nil {
		writeHTTPError(w, 422, errors.New("stored hook policy is invalid"))
		return
	}
	out := adminSnapshot{Revision: snapshot.Revision, Hooks: []adminEntry{}, Configs: []adminEntry{}, Git: []adminEntry{}, SecretReferences: []string{}, SecretWrites: s.secretAdmin != nil, Repository: repo}
	paths, refs := map[string]bool{}, map[string]bool{}
	for _, doc := range []*hookpolicy.Document{policy.Tenant, policy.Cell} {
		if doc != nil {
			for name := range doc.Hooks {
				paths[name] = true
			}
		}
	}
	for name := range paths {
		entry, source := policy.Select(name)
		out.Hooks = append(out.Hooks, adminEntry{Name: name, Value: entry, Source: source})
		if entry.Perms.JobDB != nil {
			for _, name := range entry.Perms.JobDB.Secrets.Resolve {
				refs[name] = true
			}
		}
	}
	for name, raw := range snapshot.Values {
		switch {
		case strings.HasPrefix(name, gitprofile.ProfilePrefix):
			profile, err := gitprofile.Decode(raw, false)
			if err != nil {
				writeHTTPError(w, 422, errors.New("stored Git profile is invalid"))
				return
			}
			out.Git = append(out.Git, adminEntry{Name: strings.TrimPrefix(name, gitprofile.ProfilePrefix), Value: profile, Source: "tenant"})
			for _, entry := range profile.Entries {
				if entry.SecretRef != "" {
					refs[entry.SecretRef] = true
				}
			}
		case strings.HasPrefix(name, "_c2j_"):
			// Internal policy/profile documents have dedicated validated views. Never
			// expose arbitrary reserved values through the general configuration editor.
		default:
			out.Configs = append(out.Configs, adminEntry{Name: name, Value: raw, Source: "tenant"})
		}
	}
	for name := range refs {
		out.SecretReferences = append(out.SecretReferences, name)
	}
	sort.Strings(out.SecretReferences)
	for _, entries := range [][]adminEntry{out.Hooks, out.Configs, out.Git} {
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	}
	writeJSON(w, 200, out)
}

type adminMutation struct {
	Name      string          `json:"name"`
	Operation string          `json:"operation"`
	Value     json.RawMessage `json:"value,omitempty"`
	Revision  *string         `json:"revision,omitempty"`
}

func (s *Server) handleAdminWrite(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	repo, err := s.adminScope(r)
	if err != nil {
		writeError(w, err)
		return
	}
	kind := mux.Vars(r)["kind"]
	var req adminMutation
	if err := decodeRequest(w, r, &req); err != nil {
		writeHTTPError(w, 400, errors.New("invalid administration request"))
		return
	}
	if req.Name == "" {
		writeHTTPError(w, 400, errors.New("name is required"))
		return
	}
	if repo != "" && kind != "hooks" {
		writeHTTPError(w, 400, errors.New("this setting is tenant-wide; edit it in Admin"))
		return
	}
	if kind == "secrets" {
		s.writeSecret(w, r, req)
		return
	}
	if s.configuration == nil {
		writeHTTPError(w, 501, errors.New("configuration administration is unavailable"))
		return
	}
	if req.Revision == nil {
		writeHTTPError(w, 400, errors.New("configuration revision is required; refresh before editing"))
		return
	}
	snapshot, err := s.configuration.GetTenantConfiguration(r.Context(), s.tenantID(r))
	if err != nil {
		writeAdminError(w, err)
		return
	}
	if snapshot.Revision != *req.Revision {
		writeAdminError(w, jobdb.ErrConfigurationRevisionMismatch)
		return
	}
	key, value, remove := req.Name, req.Value, req.Operation == "remove"
	switch kind {
	case "configs":
		if strings.HasPrefix(key, "_c2j_") {
			writeHTTPError(w, 400, errors.New("reserved configuration names cannot be edited here"))
			return
		}
		if req.Operation != "set" && !remove {
			writeHTTPError(w, 400, errors.New("unsupported configuration operation"))
			return
		}
		if !remove && (key == gitprofile.ReadOnlyKey || key == gitprofile.ReadWriteKey) {
			var name *string
			if json.Unmarshal(value, &name) != nil {
				writeHTTPError(w, 400, errors.New("Git profile default must be a profile name or null"))
				return
			}
			if name != nil {
				if _, err := gitprofile.Key(*name); err != nil {
					writeHTTPError(w, 400, err)
					return
				}
			}
		}
	case "git":
		key, err = gitprofile.Key(req.Name)
		if err == nil && req.Operation != "set" && !remove {
			err = errors.New("unsupported Git profile operation")
		}
		if err == nil && !remove {
			var profile gitprofile.Profile
			profile, err = gitprofile.Decode(value, false)
			if err == nil {
				value, err = json.Marshal(profile)
			}
		}
	case "hooks":
		var policy hookpolicy.Snapshot
		policy, err = hookpolicy.Capture(snapshot, repo)
		if err != nil {
			writeHTTPError(w, 422, errors.New("stored hook policy is invalid"))
			return
		}
		key = hookpolicy.TenantKey
		if repo != "" {
			key = hookpolicy.CellKey(repo)
		}
		if remove {
			// Removing a cell override restores inheritance. Clearing is a separate,
			// explicit deny-all override; these must never be conflated.
			if repo != "" {
				req.Operation = "inherit"
			} else {
				doc := hookpolicy.Document{Version: 1, Hooks: map[string]hookpolicy.Entry{}}
				if policy.Tenant != nil {
					doc = *policy.Tenant
				}
				delete(doc.Hooks, req.Name)
				value, err = json.Marshal(doc)
				remove = len(doc.Hooks) == 0
				break
			}
		}
		if req.Operation != "set" && req.Operation != "clear" && req.Operation != "inherit" {
			err = errors.New("unsupported hook operation")
			break
		}
		var entry hookpolicy.Entry
		if req.Operation == "set" {
			err = hookpolicy.Decode(value, &entry)
			if err == nil {
				err = entry.Validate()
			}
		}
		if err == nil {
			var doc hookpolicy.Document
			doc, err = hookpolicy.Edit(policy, repo != "", req.Name, req.Operation, nil, false, hookpolicy.Trust{}, false, &entry)
			if err == nil {
				value, err = json.Marshal(doc)
				remove = req.Operation == "inherit" && len(doc.Hooks) == 0
			}
		}
	default:
		err = errors.New("unknown administration category")
	}
	if err != nil {
		writeHTTPError(w, 400, err)
		return
	}
	var revision string
	if remove {
		revision, err = s.configuration.RemoveTenantConfigurationValue(r.Context(), jobdb.RemoveTenantConfigurationValueRequest{TenantID: s.tenantID(r), Name: key, ExpectedRevision: req.Revision})
	} else {
		revision, err = s.configuration.SetTenantConfigurationValue(r.Context(), jobdb.SetTenantConfigurationValueRequest{TenantID: s.tenantID(r), Name: key, Value: value, ExpectedRevision: req.Revision})
	}
	if err != nil {
		writeAdminError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"revision": revision})
}
func writeAdminError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, jobdb.ErrConfigurationRevisionMismatch):
		writeHTTPError(w, 409, errors.New("settings changed; refresh and review before saving again"))
	case errors.Is(err, jobdb.ErrInvalidConfigurationRequest), errors.Is(err, jobdb.ErrConfigurationTooLarge):
		writeHTTPError(w, 400, errors.New("invalid configuration name, value, or size"))
	default:
		writeHTTPError(w, 502, errors.New("configuration administration failed"))
	}
}

type secretAdmin interface {
	SetDevSecret(context.Context, string, string, string) error
	RemoveDevSecret(context.Context, string, string) error
}

var adminSecretName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,255}$`)

func (s *Server) writeSecret(w http.ResponseWriter, r *http.Request, req adminMutation) {
	if s.secretAdmin == nil {
		writeHTTPError(w, 501, errors.New("secret administration is unavailable"))
		return
	}
	if !adminSecretName.MatchString(req.Name) {
		writeHTTPError(w, 400, errors.New("invalid secret name"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	var err error
	switch req.Operation {
	case "set":
		var value *string
		if json.Unmarshal(req.Value, &value) != nil || value == nil {
			writeHTTPError(w, 400, errors.New("secret value must be a string"))
			return
		}
		err = s.secretAdmin.SetDevSecret(ctx, s.tenantID(r), req.Name, *value)
	case "remove":
		err = s.secretAdmin.RemoveDevSecret(ctx, s.tenantID(r), req.Name)
	default:
		writeHTTPError(w, 400, errors.New("unsupported secret operation"))
		return
	}
	// Never forward provider error text: it can contain supplied credentials.
	if err != nil {
		writeHTTPError(w, 502, errors.New("secret update failed; verify that JobDB has a writable secret store configured"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// JobDB publishes this write-only API, but its public remote Go runtime does
// not yet provide a client for it. Do not follow redirects or read error bodies.
type remoteSecretAdmin struct {
	base   string
	client *http.Client
}

func (a *remoteSecretAdmin) SetDevSecret(ctx context.Context, tenant, name, value string) error {
	return a.write(ctx, tenant, "set", map[string]string{"name": name, "value": value})
}
func (a *remoteSecretAdmin) RemoveDevSecret(ctx context.Context, tenant, name string) error {
	return a.write(ctx, tenant, "remove", map[string]string{"name": name})
}
func (a *remoteSecretAdmin) write(ctx context.Context, tenant, op string, body map[string]string) error {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.base+"/dev/v1/tenants/"+url.PathEscape(tenant)+"/secrets/"+op, bytes.NewReader(raw))
	if err != nil {
		return errors.New("invalid secret request")
	}
	req.Header.Set("Content-Type", "application/json")
	client := *a.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		return errors.New("secret administration unavailable")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("secret administration HTTP %d", response.StatusCode)
	}
	return nil
}
