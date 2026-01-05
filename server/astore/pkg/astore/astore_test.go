package astore

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/stretchr/testify/require"
)

func TestCreateExpandsInbound(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	inArt := swf.NewArtifactFromBytes("logs/out.txt", []byte("hello"))

	workspace, inboundDir, outboundDir, cleanup, err := Create(ctx, "abc", []swf.Artifact{inArt}, Config{Root: root, Limits: DefaultLimits()})
	require.NoError(t, err)
	defer cleanup()

	require.DirExists(t, workspace)
	require.DirExists(t, inboundDir)
	require.DirExists(t, outboundDir)

	data, err := os.ReadFile(filepath.Join(inboundDir, "logs", "out.txt"))
	require.NoError(t, err)
	require.Equal(t, []byte("hello"), data)
}

func TestCreateRejectsInvalidInboundPath(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	inArt := swf.NewArtifactFromBytes("../escape.txt", []byte("x"))

	_, _, _, cleanup, err := Create(ctx, "abc", []swf.Artifact{inArt}, Config{Root: root, Limits: DefaultLimits()})
	if cleanup != nil {
		defer cleanup()
	}
	require.Error(t, err)
}

func TestPersistCollectsArtifacts(t *testing.T) {
	ctx := context.Background()
	outbound := filepath.Join(t.TempDir(), "outbound")
	require.NoError(t, os.MkdirAll(filepath.Join(outbound, "nested"), 0o700))

	content := []byte("capture me")
	target := filepath.Join(outbound, "nested", "file.txt")
	require.NoError(t, os.WriteFile(target, content, 0o600))

	arts, err := Persist(ctx, outbound, Limits{})
	require.NoError(t, err)
	require.Len(t, arts, 1)

	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	got := arts[0]
	require.Equal(t, "nested/file.txt", got.Name())
	require.Equal(t, int64(len(content)), got.Size())

	gotDigest, err := got.Sha256(ctx)
	require.NoError(t, err)
	require.Equal(t, digest, gotDigest)
}

func TestPersistSortingStable(t *testing.T) {
	ctx := context.Background()
	outbound := filepath.Join(t.TempDir(), "outbound")
	require.NoError(t, os.MkdirAll(outbound, 0o700))

	files := []string{"b.txt", "a.txt", "c/inner.txt"}
	for _, f := range files {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(outbound, f)), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(outbound, f), []byte(f), 0o600))
	}

	arts, err := Persist(ctx, outbound, Limits{})
	require.NoError(t, err)
	require.Equal(t, []string{"a.txt", "b.txt", "c/inner.txt"}, []string{arts[0].Name(), arts[1].Name(), arts[2].Name()})
}

func TestPersistExcludes(t *testing.T) {
	ctx := context.Background()
	outbound := filepath.Join(t.TempDir(), "outbound")
	require.NoError(t, os.MkdirAll(filepath.Join(outbound, ".git"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(outbound, ".git", "config"), []byte("noop"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(outbound, ".DS_Store"), []byte("ignore"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(outbound, "keep.txt"), []byte("keep"), 0o600))

	arts, err := Persist(ctx, outbound, Limits{})
	require.NoError(t, err)
	require.Len(t, arts, 1)
	require.Equal(t, "keep.txt", arts[0].Name())
}

func TestPersistRejectsSymlink(t *testing.T) {
	ctx := context.Background()
	outbound := filepath.Join(t.TempDir(), "outbound")
	require.NoError(t, os.MkdirAll(outbound, 0o700))

	target := filepath.Join(outbound, "real.txt")
	require.NoError(t, os.WriteFile(target, []byte("real"), 0o600))

	link := filepath.Join(outbound, "link.txt")
	require.NoError(t, os.Symlink("real.txt", link))

	_, err := Persist(ctx, outbound, Limits{})
	require.Error(t, err)
}

func TestPersistLimits(t *testing.T) {
	ctx := context.Background()
	outbound := filepath.Join(t.TempDir(), "outbound")
	require.NoError(t, os.MkdirAll(outbound, 0o700))

	require.NoError(t, os.WriteFile(filepath.Join(outbound, "a.txt"), []byte("a"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(outbound, "b.txt"), []byte("b"), 0o600))

	_, err := Persist(ctx, outbound, Limits{MaxFiles: 1})
	require.Error(t, err)

	_, err = Persist(ctx, outbound, Limits{MaxTotalBytes: 1})
	require.Error(t, err)
}

func TestPersistEmptyOutbound(t *testing.T) {
	ctx := context.Background()
	outbound := filepath.Join(t.TempDir(), "outbound")
	require.NoError(t, os.MkdirAll(outbound, 0o700))

	arts, err := Persist(ctx, outbound, Limits{})
	require.NoError(t, err)
	require.Empty(t, arts)
}

func TestSanitize(t *testing.T) {
	require.Equal(t, "op", sanitize(""))
	require.Equal(t, "abc-123", sanitize("abc!123"))
}
