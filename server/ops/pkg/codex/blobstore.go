package codex

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type fileBlobStore struct{}

func (fileBlobStore) Put(ctx context.Context, baseURI, relativePath, sourcePath string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	basePath, err := fileURIToPath(baseURI)
	if err != nil {
		return "", err
	}
	destination := filepath.Join(basePath, relativePath)
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return "", fmt.Errorf("create blobstore path: %w", err)
	}
	if filepath.Clean(sourcePath) == filepath.Clean(destination) {
		return relativePath, nil
	}
	if err := copyFile(sourcePath, destination); err != nil {
		return "", err
	}
	return relativePath, nil
}

func fileURIToPath(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("blobstore URI is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse blobstore URI: %w", err)
	}
	if u.Scheme == "" || u.Scheme == "file" {
		path := u.Path
		if path == "" {
			path = u.Host
		}
		if path == "" {
			return "", fmt.Errorf("file URI must include a path")
		}
		return filepath.Clean(path), nil
	}
	return "", fmt.Errorf("unsupported storage scheme: %s", u.Scheme)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer func() {
		_ = out.Close()
	}()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy blob: %w", err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("flush blob: %w", err)
	}
	return nil
}
