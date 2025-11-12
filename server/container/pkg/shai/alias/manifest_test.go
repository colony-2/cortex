package alias

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".shai-cmds")
	content := `
# comment
deploy  ^(--env=(dev|prod))$   ./scripts/deploy.sh
sync    -   git pull --rebase
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}
	if len(manifest.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(manifest.Entries))
	}
	entry := manifest.Entries["deploy"]
	if entry == nil || entry.Command != "./scripts/deploy.sh" || entry.ArgsRegex == "" {
		t.Fatalf("unexpected entry: %#v", entry)
	}
	if err := entry.ValidateArgs("--env=dev"); err != nil {
		t.Fatalf("validate args failed: %v", err)
	}
	if err := entry.ValidateArgs("--env=qa"); err == nil {
		t.Fatalf("expected validation failure")
	}
	if err := manifest.Entries["sync"].ValidateArgs(" --anything"); err == nil {
		t.Fatalf("expected sync to reject args")
	}
}

func TestLoadManifestErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".shai-cmds")
	content := `bad alias`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(path); err == nil {
		t.Fatalf("expected error for invalid manifest")
	}
}
