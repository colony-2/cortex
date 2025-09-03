package shai

import (
    "os"
    "path/filepath"
    "testing"
)

func TestToSnakeUpper(t *testing.T) {
    cases := map[string]string{
        "installZsh":                 "INSTALL_ZSH",
        "configureZshAsDefaultShell": "CONFIGURE_ZSH_AS_DEFAULT_SHELL",
        "username":                   "USERNAME",
        "uid":                        "UID",
        "gid":                        "GID",
        "dash-case":                  "DASH_CASE",
    }

    for in, expected := range cases {
        if got := toSnakeUpper(in); got != expected {
            t.Fatalf("toSnakeUpper(%q)=%q want %q", in, got, expected)
        }
    }
}

func TestOrderResolvedFeatures(t *testing.T) {
    a := ResolvedFeature{ID: "A"}
    b := ResolvedFeature{ID: "B", InstallsAfter: []string{"A"}}
    c := ResolvedFeature{ID: "C", InstallsAfter: []string{"B"}}
    out, err := orderResolvedFeatures([]ResolvedFeature{c, a, b})
    if err != nil { t.Fatalf("order error: %v", err) }
    if !(out[0].ID=="A" && out[1].ID=="B" && out[2].ID=="C") {
        t.Fatalf("unexpected order: %v,%v,%v", out[0].ID, out[1].ID, out[2].ID)
    }
}

func TestValidateFeaturesUnsupported(t *testing.T) {
    feats := []ResolvedFeature{{ID: "X", Unsupported: []string{"mounts"}}}
    if err := validateFeatures(feats); err == nil {
        t.Fatal("expected error for unsupported fields")
    }
}

func TestParseFeatureSpecUnknownKeys(t *testing.T) {
    dir := t.TempDir()
    // Write a minimal devcontainer-feature.json with an unknown key
    spec := `{
        "id": "example",
        "version": "1.0.0",
        "unknownKey": true
    }`
    if err := os.WriteFile(filepath.Join(dir, "devcontainer-feature.json"), []byte(spec), 0644); err != nil {
        t.Fatal(err)
    }
    f := &ResolvedFeature{Dir: dir}
    err := parseFeatureSpec(f)
    if err == nil {
        t.Fatal("expected error due to unknown keys")
    }
}

func TestParseFeatureSpecAllowsCustomizations(t *testing.T) {
    dir := t.TempDir()
    spec := `{
        "id": "example",
        "version": "1.0.0",
        "customizations": {"vscode": {"extensions": ["foo.bar"]}}
    }`
    if err := os.WriteFile(filepath.Join(dir, "devcontainer-feature.json"), []byte(spec), 0644); err != nil {
        t.Fatal(err)
    }
    f := &ResolvedFeature{Dir: dir}
    if err := parseFeatureSpec(f); err != nil {
        t.Fatalf("customizations should be allowed, got error: %v", err)
    }
}

func TestParseFeatureSpecOptionDefaults(t *testing.T) {
    dir := t.TempDir()
    spec := `{
        "id": "git",
        "version": "1.0.0",
        "options": {
            "version": {"type": "string", "default": "os-provided"}
        }
    }`
    if err := os.WriteFile(filepath.Join(dir, "devcontainer-feature.json"), []byte(spec), 0644); err != nil {
        t.Fatal(err)
    }
    f := &ResolvedFeature{Dir: dir}
    if err := parseFeatureSpec(f); err != nil {
        t.Fatalf("parse defaults err: %v", err)
    }
    if f.Defaults["VERSION"] != "os-provided" {
        t.Fatalf("expected VERSION default 'os-provided', got %q", f.Defaults["VERSION"])
    }
}
