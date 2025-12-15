package astore

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/colony-2/strata-go/pkg/client/artifact"
	"github.com/colony-2/swf-go/pkg/swf"
)

// Create provisions an astore workspace rooted at cfg.Root with inbound/outbound dirs.
// Inbound artifacts are materialized into the inbound directory before returning.
func Create(ctx context.Context, opID string, inbound []swf.Artifact, cfg Config) (root string, inboundDir string, outboundDir string, cleanup func(), err error) {
	cfg = cfg.withEnv()
	if cfg.Root == "" {
		cfg.Root = os.TempDir()
	}
	if err := os.MkdirAll(cfg.Root, 0o700); err != nil {
		return "", "", "", nil, fmt.Errorf("astore: ensure root: %w", err)
	}

	prefix := fmt.Sprintf("astore-%s-", sanitize(opID))
	rootDir, err := os.MkdirTemp(cfg.Root, prefix)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("astore: create temp dir: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(rootDir) }

	inboundDir = filepath.Join(rootDir, "inbound")
	outboundDir = filepath.Join(rootDir, "outbound")
	for _, dir := range []string{inboundDir, outboundDir} {
		if mkErr := os.MkdirAll(dir, 0o700); mkErr != nil {
			cleanup()
			return "", "", "", nil, fmt.Errorf("astore: create subdir %s: %w", dir, mkErr)
		}
	}

	if err := expandInbound(ctx, inboundDir, inbound); err != nil {
		cleanup()
		return "", "", "", nil, err
	}

	return rootDir, inboundDir, outboundDir, cleanup, nil
}

// Persist walks outboundDir and returns artifacts representing all files.
// Zero files is not an error.
func Persist(ctx context.Context, outboundDir string, limits Limits) ([]swf.Artifact, error) {
	limits = limits.withDefaults()

	var (
		artifacts []swf.Artifact
		total     int64
		count     int
	)

	err := filepath.WalkDir(outboundDir, func(current string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(outboundDir, current)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if isExcluded(rel, limits.Excludes) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("astore: symlink not supported: %s", rel)
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		size := info.Size()
		count++
		if limits.MaxFiles > 0 && count > limits.MaxFiles {
			return fmt.Errorf("astore: max files exceeded (%d)", limits.MaxFiles)
		}
		if limits.MaxTotalBytes > 0 && total+size > limits.MaxTotalBytes {
			return fmt.Errorf("astore: max bytes exceeded (%d)", limits.MaxTotalBytes)
		}
		digest, bytesRead, err := hashFile(current)
		if err != nil {
			return fmt.Errorf("astore: hash %s: %w", rel, err)
		}
		if bytesRead != size {
			return fmt.Errorf("astore: size changed while hashing %s: expected %d read %d", rel, size, bytesRead)
		}
		artifactEntry, err := artifactFromFile(rel, current, size, digest)
		if err != nil {
			return err
		}
		artifacts = append(artifacts, artifactEntry)
		total += size
		return nil
	})
	if err != nil {
		return nil, err
	}

	slices.SortFunc(artifacts, func(a, b swf.Artifact) int {
		return strings.Compare(a.Name(), b.Name())
	})
	return artifacts, nil
}

func expandInbound(ctx context.Context, inboundDir string, inbound []swf.Artifact) error {
	for _, art := range inbound {
		name := strings.TrimSpace(art.Name())
		if name == "" {
			return errors.New("astore: inbound artifact missing name")
		}
		clean := filepath.Clean(filepath.FromSlash(name))
		if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return fmt.Errorf("astore: invalid inbound artifact path %q", name)
		}
		target := filepath.Join(inboundDir, clean)
		if !strings.HasPrefix(target, inboundDir+string(os.PathSeparator)) && target != inboundDir {
			return fmt.Errorf("astore: inbound path escapes root: %q", name)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return fmt.Errorf("astore: create inbound dir for %s: %w", name, err)
		}
		if err := art.SaveToFile(ctx, target); err != nil {
			return fmt.Errorf("astore: write inbound artifact %s: %w", name, err)
		}
	}
	return nil
}

func hashFile(path string) (digest string, bytesRead int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	hasher := sha256.New()
	n, err := io.Copy(hasher, f)
	if err != nil {
		return "", n, err
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), n, nil
}

func artifactFromFile(relPath, fullPath string, size int64, digest string) (swf.Artifact, error) {
	rel := filepath.ToSlash(relPath)
	if rel == "" || rel == "." {
		return nil, fmt.Errorf("astore: invalid relative path %q", relPath)
	}
	opener := func(context.Context) (io.ReadCloser, error) {
		return os.Open(fullPath)
	}
	return artifact.FromReader(rel, "", size, opener, artifact.WithSha256(digest)), nil
}

func isExcluded(rel string, excludes []string) bool {
	if len(excludes) == 0 {
		return false
	}
	segments := strings.Split(filepath.ToSlash(rel), "/")
	for _, seg := range segments {
		for _, ex := range excludes {
			if seg == ex {
				return true
			}
		}
	}
	return false
}

func sanitize(opID string) string {
	if strings.TrimSpace(opID) == "" {
		return "op"
	}
	var b strings.Builder
	for _, r := range opID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	clean := strings.Trim(b.String(), "-_")
	if clean == "" {
		return "op"
	}
	return clean
}
