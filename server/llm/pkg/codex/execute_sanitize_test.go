package codex

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeCodexHomeOutputRemovesSensitiveFilesOnly(t *testing.T) {
	workdir := t.TempDir()
	codexHome := filepath.Join(workdir, codexHomeDirName)
	require.NoError(t, os.MkdirAll(codexHome, 0o755))

	write := func(path string, value string) {
		t.Helper()
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(value), 0o644))
	}

	write(filepath.Join(codexHome, "auth.json"), "host-auth")
	write(filepath.Join(codexHome, "config.toml"), "host-config")
	write(filepath.Join(codexHome, "unchanged.txt"), "same")
	write(filepath.Join(codexHome, "nested", "same.txt"), "same-nested")
	write(filepath.Join(codexHome, "changed.txt"), "after")
	write(filepath.Join(codexHome, "new.txt"), "new")

	opts := Options{
		Prompt:       "test",
		WorkDirRoot:  workdir,
		WorktreeRoot: filepath.Join(workdir, "worktree"),
		CodexHome:    codexHome,
	}

	require.NoError(t, sanitizeCodexHomeOutput(opts))

	require.NoFileExists(t, filepath.Join(codexHome, "auth.json"))
	require.NoFileExists(t, filepath.Join(codexHome, "config.toml"))
	require.FileExists(t, filepath.Join(codexHome, "unchanged.txt"))
	require.FileExists(t, filepath.Join(codexHome, "nested", "same.txt"))
	require.FileExists(t, filepath.Join(codexHome, "changed.txt"))
	require.FileExists(t, filepath.Join(codexHome, "new.txt"))
}

func TestSanitizeCodexHomeOutputWithoutInboxKeepsNonSensitiveFiles(t *testing.T) {
	workdir := t.TempDir()
	codexHome := filepath.Join(workdir, codexHomeDirName)
	require.NoError(t, os.MkdirAll(codexHome, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte("auth"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte("cfg"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(codexHome, "session.json"), []byte("session"), 0o644))

	opts := Options{
		Prompt:        "test",
		WorkDirRoot:   workdir,
		WorktreeRoot:  filepath.Join(workdir, "worktree"),
		ArtifactInbox: filepath.Join(workdir, "inbox"),
		CodexHome:     codexHome,
	}

	require.NoError(t, sanitizeCodexHomeOutput(opts))

	require.NoFileExists(t, filepath.Join(codexHome, "auth.json"))
	require.NoFileExists(t, filepath.Join(codexHome, "config.toml"))
	require.FileExists(t, filepath.Join(codexHome, "session.json"))
}

func TestPrepareDirectCodexHomeLinksSessionsIntoOutbox(t *testing.T) {
	workdir := t.TempDir()
	inbox := filepath.Join(workdir, "inbox")
	outbox := filepath.Join(workdir, "outbox")
	codexHome := filepath.Join(workdir, codexHomeDirName)
	stagedSkills := filepath.Join(workdir, ".codex-skill-inputs", "skills")
	require.NoError(t, os.MkdirAll(filepath.Join(inbox, codexSessionsArtifactDirName, "nested"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(outbox, codexSessionsArtifactDirName), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(stagedSkills, "platform-skill"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(inbox, codexSessionsArtifactDirName, "nested", "session.json"), []byte("session"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(stagedSkills, "platform-skill", "SKILL.md"), []byte("skill"), 0o644))
	require.NoError(t, os.MkdirAll(codexHome, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(codexHome, "stale.txt"), []byte("stale"), 0o644))

	err := prepareDirectCodexHome(Options{
		Prompt:              "test",
		WorkDirRoot:         workdir,
		WorktreeRoot:        filepath.Join(workdir, "worktree"),
		ArtifactInbox:       inbox,
		ArtifactOutbox:      outbox,
		CodexHome:           codexHome,
		ConfiguredSkillDirs: []string{stagedSkills},
	})
	require.NoError(t, err)

	require.NoFileExists(t, filepath.Join(codexHome, "stale.txt"))
	require.DirExists(t, filepath.Join(codexHome, ".agents", "skills", "platform-skill"))
	require.FileExists(t, filepath.Join(codexHome, ".agents", "skills", "platform-skill", "SKILL.md"))
	require.FileExists(t, filepath.Join(outbox, codexSessionsArtifactDirName, "nested", "session.json"))

	linkTarget, err := os.Readlink(filepath.Join(codexHome, "sessions"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(outbox, codexSessionsArtifactDirName), linkTarget)
}
