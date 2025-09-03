package shai

import (
    "archive/tar"
    "bytes"
    "compress/gzip"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "os"
    "path/filepath"
    "regexp"
    "strings"

    zstd "github.com/klauspost/compress/zstd"
)

// ResolvedFeature represents a feature pulled to a local directory
type ResolvedFeature struct {
    ID       string
    SafeName string
    Dir      string
    Options  map[string]interface{}
    InstallsAfter []string
    Unsupported   []string
    Defaults      map[string]string // option defaults from devcontainer-feature.json
}

// OptionsEnv returns env var names/values derived from Options (camelCase -> SNAKE_UPPER)
func (f ResolvedFeature) OptionsEnv() map[string]string {
    env := map[string]string{}
    for k, v := range f.Options {
        name := toSnakeUpper(k)
        switch val := v.(type) {
        case string:
            env[name] = val
        case float64:
            env[name] = fmt.Sprintf("%v", val)
        case bool:
            if val {
                env[name] = "true"
            } else {
                env[name] = "false"
            }
        default:
            b, _ := json.Marshal(val)
            env[name] = string(b)
        }
    }
    return env
}

var camelToSnake = regexp.MustCompile(`([a-z0-9])([A-Z])`)

func toSnakeUpper(s string) string {
    withUnderscore := camelToSnake.ReplaceAllString(s, `${1}_${2}`)
    withUnderscore = strings.ReplaceAll(withUnderscore, "-", "_")
    return strings.ToUpper(withUnderscore)
}

// ResolveOCIFeatures downloads GHCR DevContainer features to temp dirs.
// Supports IDs like: ghcr.io/devcontainers/features/common-utils:2
func ResolveOCIFeatures(features map[string]interface{}) ([]ResolvedFeature, error) {
    var result []ResolvedFeature
    for id, cfg := range features {
        rf, err := resolveSingleFeature(id, cfg)
        if err != nil {
            return nil, err
        }
        result = append(result, rf)
    }
    return result, nil
}

func resolveSingleFeature(id string, cfg interface{}) (ResolvedFeature, error) {
    rf := ResolvedFeature{ID: id, SafeName: safeFeatureName(id), Options: map[string]interface{}{} }
    if m, ok := cfg.(map[string]interface{}); ok {
        rf.Options = m
    }

    // Only GHCR is supported for now
    if !strings.HasPrefix(id, "ghcr.io/") {
        return rf, fmt.Errorf("unsupported feature registry: %s", id)
    }

    hostRepo, version, found := strings.Cut(id, ":")
    if !found || version == "" {
        // Default tag when omitted
        version = "latest"
        hostRepo = id
    }
    // Repository path under ghcr.io
    repo := strings.TrimPrefix(hostRepo, "ghcr.io/")

    // Fetch manifest or manifest list with bearer token auth
    manifestURL := fmt.Sprintf("https://ghcr.io/v2/%s/manifests/%s", repo, version)
    resp, _, err := fetchWithBearer(manifestURL, repo, "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json")
    if err != nil {
        return rf, fmt.Errorf("fetch manifest: %w", err)
    }
    defer resp.Body.Close()

    contentType := resp.Header.Get("Content-Type")

    // Handle manifest list (index)
    if strings.Contains(contentType, "application/vnd.oci.image.index.v1+json") || strings.Contains(contentType, "application/vnd.docker.distribution.manifest.list.v2+json") {
        var idx struct {
            Manifests []struct {
                MediaType string `json:"mediaType"`
                Digest    string `json:"digest"`
                Platform  struct {
                    OS           string `json:"os"`
                    Architecture string `json:"architecture"`
                } `json:"platform"`
            } `json:"manifests"`
        }
        if err := json.NewDecoder(resp.Body).Decode(&idx); err != nil {
            return rf, fmt.Errorf("parse index: %w", err)
        }

        // Pick linux/amd64 if available, else linux/arm64, else first linux, else first
        pick := func(osn, arch string) (string, bool) {
            for _, m := range idx.Manifests {
                if (osn == "" || m.Platform.OS == osn) && (arch == "" || m.Platform.Architecture == arch) {
                    return m.Digest, true
                }
            }
            return "", false
        }
        var digest string
        if d, ok := pick("linux", "amd64"); ok { digest = d } else
        if d, ok := pick("linux", "arm64"); ok { digest = d } else
        if d, ok := pick("linux", ""); ok { digest = d } else if len(idx.Manifests) > 0 { digest = idx.Manifests[0].Digest }
        if digest == "" {
            return rf, errors.New("no suitable manifest in index")
        }
        // Fetch the chosen manifest
        mURL := fmt.Sprintf("https://ghcr.io/v2/%s/manifests/%s", repo, digest)
        resp.Body.Close()
        resp2, _, err := fetchWithBearer(mURL, repo, "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json")
        if err != nil { return rf, fmt.Errorf("fetch selected manifest: %w", err) }
        defer resp2.Body.Close()
        resp = resp2
    }

    var mani struct {
        Layers []struct {
            MediaType string `json:"mediaType"`
            Digest    string `json:"digest"`
        } `json:"layers"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&mani); err != nil {
        return rf, fmt.Errorf("parse manifest: %w", err)
    }

    var digest string
    // Prefer any layer whose mediaType includes both "layer" and "tar"
    for _, l := range mani.Layers {
        mt := strings.ToLower(l.MediaType)
        if strings.Contains(mt, "layer") && strings.Contains(mt, "tar") {
            digest = l.Digest
            break
        }
    }
    // Fallback: first layer if none matched
    if digest == "" && len(mani.Layers) > 0 {
        digest = mani.Layers[0].Digest
    }
    if digest == "" {
        return rf, fmt.Errorf("feature layer not found in manifest")
    }

    // Download blob
    blobURL := fmt.Sprintf("https://ghcr.io/v2/%s/blobs/%s", repo, digest)
    br, _, err := fetchWithBearer(blobURL, repo, "application/octet-stream")
    if err != nil { return rf, fmt.Errorf("fetch blob: %w", err) }
    defer br.Body.Close()

    // Extract into temp dir
    tmpDir, err := os.MkdirTemp("", "devcontainer-feature-*")
    if err != nil {
        return rf, err
    }

    if err := untarAutodetect(br.Body, tmpDir); err != nil {
        return rf, fmt.Errorf("extract feature: %w", err)
    }

    rf.Dir = tmpDir

    // Parse devcontainer-feature.json for metadata
    if err := parseFeatureSpec(&rf); err != nil {
        return rf, err
    }
    return rf, nil
}

func safeFeatureName(id string) string {
    s := strings.ReplaceAll(id, ":", "_")
    s = strings.ReplaceAll(s, "/", "_")
    return s
}

func untarAutodetect(r io.Reader, dest string) error {
    data, err := io.ReadAll(r)
    if err != nil { return err }

    // Sniff magic bytes
    if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
        // gzip
        gr, err := gzip.NewReader(bytes.NewReader(data))
        if err != nil { return err }
        defer gr.Close()
        return untar(gr, dest)
    }
    if len(data) >= 4 && data[0] == 0x28 && data[1] == 0xB5 && data[2] == 0x2F && data[3] == 0xFD {
        // zstd
        zr, err := zstd.NewReader(bytes.NewReader(data))
        if err != nil { return err }
        defer zr.Close()
        if err := untar(zr, dest); err != nil {
            // If zstd untar fails, try plain tar as last resort
            return untar(bytes.NewReader(data), dest)
        }
        return nil
    }
    // Fallback to plain tar
    return untar(bytes.NewReader(data), dest)
}

func untar(r io.Reader, dest string) error {
    tr := tar.NewReader(r)
    for {
        hdr, err := tr.Next()
        if err == io.EOF {
            break
        }
        if err != nil {
            return err
        }
        target := filepath.Join(dest, hdr.Name)
        switch hdr.Typeflag {
        case tar.TypeDir:
            if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
                return err
            }
        case tar.TypeReg:
            if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
                return err
            }
            f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
            if err != nil {
                return err
            }
            if _, err := io.Copy(f, tr); err != nil {
                f.Close()
                return err
            }
            f.Close()
        default:
            // ignore others
        }
    }
    return nil
}

// parseFeatureSpec loads devcontainer-feature.json from the resolved feature dir
// and extracts ordering and flags. Unsupported flags will be recorded for validation.
func parseFeatureSpec(f *ResolvedFeature) error {
    specPath := filepath.Join(f.Dir, "devcontainer-feature.json")
    data, err := os.ReadFile(specPath)
    if err != nil {
        return err
    }
    var raw map[string]interface{}
    if err := json.Unmarshal(data, &raw); err != nil {
        return err
    }
    if v, ok := raw["installsAfter"].([]interface{}); ok {
        for _, it := range v {
            if s, ok := it.(string); ok { f.InstallsAfter = append(f.InstallsAfter, s) }
        }
    }
    // Extract option defaults
    if opts, ok := raw["options"].(map[string]interface{}); ok {
        f.Defaults = map[string]string{}
        for name, val := range opts {
            // Option can be object with default field, or a simple type
            switch ov := val.(type) {
            case map[string]interface{}:
                if def, ok := ov["default"]; ok {
                    f.Defaults[strings.ToUpper(strings.ReplaceAll(name, "-", "_"))] = stringify(def)
                }
            default:
                // No explicit default field; skip
            }
        }
    }
    // Validate keys against spec: allow known keys, but mark operational keys we do not implement as unsupported
    knownKeys := map[string]bool{
        "id": true, "name": true, "version": true,
        "description": true, "documentationURL": true, "licenseURL": true,
        "keywords": true,
        "options": true, "containerEnv": true,
        "privileged": true, "init": true, "capAdd": true, "securityOpt": true, "entrypoint": true,
        "customizations": true,
        "dependsOn": true, "installsAfter": true,
        "legacyIds": true, "deprecated": true,
        "mounts": true,
    }
    var unknown []string
    for k := range raw {
        if !knownKeys[k] {
            unknown = append(unknown, k)
        }
    }
    if len(unknown) > 0 {
        f.Unsupported = append(f.Unsupported, unknown...)
        return fmt.Errorf("unknown feature keys: %v", unknown)
    }
    // Mark certain known but unimplemented operational keys as unsupported
    for _, k := range []string{"privileged", "mounts", "init", "capAdd", "securityOpt", "entrypoint", "containerEnv", "dependsOn"} {
        if _, ok := raw[k]; ok {
            f.Unsupported = append(f.Unsupported, k)
        }
    }
    return nil
}

func stringify(v interface{}) string {
    switch t := v.(type) {
    case string:
        return t
    case bool:
        if t { return "true" } 
        return "false"
    case float64:
        return fmt.Sprintf("%v", t)
    default:
        b, _ := json.Marshal(t)
        return string(b)
    }
}

// fetchWithBearer performs an HTTP GET and if 401 with WWW-Authenticate is returned,
// it obtains a bearer token from the realm and retries.
func fetchWithBearer(urlStr, repo, accept string) (*http.Response, string, error) {
    req, _ := http.NewRequest("GET", urlStr, nil)
    if accept != "" {
        req.Header.Set("Accept", accept)
    }
    if tok := os.Getenv("GHCR_TOKEN"); tok != "" {
        req.Header.Set("Authorization", "Bearer "+tok)
    } else if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
        req.Header.Set("Authorization", "Bearer "+tok)
    }
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, "", err
    }
    if resp.StatusCode != http.StatusUnauthorized {
        if resp.StatusCode < 200 || resp.StatusCode >= 300 {
            defer resp.Body.Close()
            return nil, "", fmt.Errorf(resp.Status)
        }
        return resp, "", nil
    }
    // Parse WWW-Authenticate header
    www := resp.Header.Get("Www-Authenticate")
    resp.Body.Close()
    realm, service, scope := parseWWWAuthenticate(www, repo)
    if realm == "" {
        return nil, "", fmt.Errorf("unauthorized and no realm in WWW-Authenticate")
    }
    // Request token
    q := make(url.Values)
    if service != "" { q.Set("service", service) }
    if scope != "" { q.Set("scope", scope) }
    tokenURL := realm
    if strings.Contains(realm, "?") {
        tokenURL += "&" + q.Encode()
    } else if len(q) > 0 {
        tokenURL += "?" + q.Encode()
    }
    tr, err := http.Get(tokenURL)
    if err != nil { return nil, "", fmt.Errorf("get token: %w", err) }
    defer tr.Body.Close()
    if tr.StatusCode < 200 || tr.StatusCode >= 300 { return nil, "", fmt.Errorf("get token: %s", tr.Status) }
    var tok struct{ Token string `json:"token"`; AccessToken string `json:"access_token"` }
    if err := json.NewDecoder(tr.Body).Decode(&tok); err != nil { return nil, "", fmt.Errorf("parse token: %w", err) }
    token := tok.Token
    if token == "" { token = tok.AccessToken }
    if token == "" { return nil, "", fmt.Errorf("empty token") }

    // Retry request with Authorization header
    req2, _ := http.NewRequest("GET", urlStr, nil)
    if accept != "" { req2.Header.Set("Accept", accept) }
    req2.Header.Set("Authorization", "Bearer "+token)
    resp2, err := http.DefaultClient.Do(req2)
    if err != nil { return nil, "", err }
    if resp2.StatusCode < 200 || resp2.StatusCode >= 300 {
        defer resp2.Body.Close()
        return nil, "", fmt.Errorf(resp2.Status)
    }
    return resp2, tok.Token, nil
}

func parseWWWAuthenticate(h, repo string) (realm, service, scope string) {
    // Example: Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:devcontainers/features/go:pull"
    parts := strings.Split(h, ",")
    for _, p := range parts {
        p = strings.TrimSpace(p)
        if strings.HasPrefix(strings.ToLower(p), "bearer ") {
            p = strings.TrimSpace(p[len("bearer "):])
        }
        kv := strings.SplitN(p, "=", 2)
        if len(kv) != 2 { continue }
        key := strings.ToLower(strings.TrimSpace(kv[0]))
        val := strings.Trim(kv[1], "\"")
        switch key {
        case "realm":
            realm = val
        case "service":
            service = val
        case "scope":
            scope = val
        }
    }
    if scope == "" && repo != "" {
        scope = "repository:" + repo + ":pull"
    }
    return
}
