package gha

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
)

func resolveWorkflowSelector(selector string, gitCtx coreops.GitExecutionContext) (resolvedWorkflow, error) {
	raw := strings.TrimSpace(selector)
	if raw == "" {
		return resolvedWorkflow{}, fmt.Errorf("workflow is required")
	}

	switch {
	case strings.HasPrefix(raw, "repo://"):
		return resolveLocalWorkflow(raw, "repo", strings.TrimPrefix(raw, "repo://"), gitCtx.WorktreePath)
	case strings.HasPrefix(raw, "cell://"):
		cellPath := strings.TrimSpace(gitCtx.CellPath)
		if cellPath == "" {
			return resolvedWorkflow{}, fmt.Errorf("cell:// selectors require cell_path in git context")
		}
		return resolveLocalWorkflow(raw, "cell", strings.TrimPrefix(raw, "cell://"), filepath.Join(gitCtx.WorktreePath, filepath.FromSlash(cellPath)))
	case strings.HasPrefix(raw, "git+"):
		return resolveExternalWorkflow(raw)
	default:
		return resolvedWorkflow{}, fmt.Errorf("unsupported workflow selector %q", raw)
	}
}

type parsedGitWorkflowSelector struct {
	Selector     string
	Scheme       string
	RepoURL      string
	RepoPath     string
	WorkflowPath string
	Ref          string
}

func resolveLocalWorkflow(selector string, scheme string, rawPath string, baseDir string) (resolvedWorkflow, error) {
	if strings.TrimSpace(baseDir) == "" {
		return resolvedWorkflow{}, fmt.Errorf("%s:// selectors require a worktree path", scheme)
	}

	normalized, err := normalizeSelectorPath(rawPath)
	if err != nil {
		return resolvedWorkflow{}, fmt.Errorf("invalid %s:// selector: %w", scheme, err)
	}

	targetPath, err := resolveUnderBase(baseDir, normalized)
	if err != nil {
		return resolvedWorkflow{}, fmt.Errorf("resolve workflow path: %w", err)
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		return resolvedWorkflow{}, err
	}
	if info.IsDir() {
		return resolvedWorkflow{}, fmt.Errorf("workflow path %q is a directory", targetPath)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		return resolvedWorkflow{}, err
	}

	return resolvedWorkflow{
		Selector:       selector,
		Path:           targetPath,
		ContentHash:    contentHash(data),
		ResolvedCommit: "",
	}, nil
}

func resolveExternalWorkflow(selector string) (resolvedWorkflow, error) {
	parsed, err := parseGitWorkflowSelector(selector)
	if err != nil {
		return resolvedWorkflow{}, err
	}

	var (
		data           []byte
		resolvedCommit string
	)
	switch parsed.Scheme {
	case "file":
		data, resolvedCommit, err = readFileWorkflowFromLocalRepo(parsed)
	default:
		data, resolvedCommit, err = readFileWorkflowFromRemoteRepo(parsed)
	}
	if err != nil {
		return resolvedWorkflow{}, err
	}

	materializedPath, err := materializeExternalWorkflow(selector, parsed.WorkflowPath, data)
	if err != nil {
		return resolvedWorkflow{}, err
	}

	return resolvedWorkflow{
		Selector:       selector,
		Path:           materializedPath,
		ContentHash:    contentHash(data),
		ResolvedCommit: resolvedCommit,
	}, nil
}

func parseGitWorkflowSelector(selector string) (parsedGitWorkflowSelector, error) {
	raw := strings.TrimSpace(selector)
	if !strings.HasPrefix(raw, "git+") {
		return parsedGitWorkflowSelector{}, fmt.Errorf("unsupported workflow selector %q", selector)
	}

	at := strings.LastIndex(raw, "@")
	if at == -1 || at == len(raw)-1 {
		return parsedGitWorkflowSelector{}, fmt.Errorf("git+ selector %q must include a non-empty ref", selector)
	}
	repoAndPath := raw[:at]
	ref := strings.TrimSpace(raw[at+1:])

	schemeEnd := strings.Index(repoAndPath, "://")
	if schemeEnd == -1 {
		return parsedGitWorkflowSelector{}, fmt.Errorf("git+ selector %q must include a supported scheme", selector)
	}
	scheme := strings.TrimPrefix(repoAndPath[:schemeEnd], "git+")
	switch scheme {
	case "file", "ssh", "http", "https":
	default:
		return parsedGitWorkflowSelector{}, fmt.Errorf("git+ selector %q uses unsupported scheme %q", selector, scheme)
	}

	splitAt := strings.LastIndex(repoAndPath, "//")
	if splitAt == -1 || splitAt <= schemeEnd+2 || splitAt == len(repoAndPath)-2 {
		return parsedGitWorkflowSelector{}, fmt.Errorf("git+ selector %q must include //<repo-relative-workflow-path>", selector)
	}

	repoURL := strings.TrimSpace(repoAndPath[:splitAt])
	workflowPath, err := normalizeSelectorPath(repoAndPath[splitAt+2:])
	if err != nil {
		return parsedGitWorkflowSelector{}, fmt.Errorf("invalid git+ selector path: %w", err)
	}

	parsed := parsedGitWorkflowSelector{
		Selector:     selector,
		Scheme:       scheme,
		RepoURL:      repoURL,
		WorkflowPath: workflowPath,
		Ref:          ref,
	}
	if scheme == "file" {
		u, err := url.Parse(repoURL)
		if err != nil {
			return parsedGitWorkflowSelector{}, fmt.Errorf("invalid file repository url %q: %w", repoURL, err)
		}
		if u.Host != "" && u.Host != "localhost" {
			return parsedGitWorkflowSelector{}, fmt.Errorf("file repository url %q must not include a remote host", repoURL)
		}
		if u.Path == "" {
			return parsedGitWorkflowSelector{}, fmt.Errorf("file repository url %q must include a repository path", repoURL)
		}
		parsed.RepoPath = u.Path
	}
	return parsed, nil
}

func readFileWorkflowFromLocalRepo(parsed parsedGitWorkflowSelector) ([]byte, string, error) {
	repoPath, err := filepath.Abs(parsed.RepoPath)
	if err != nil {
		return nil, "", err
	}
	resolvedRef, err := resolveGitRef(repoPath, parsed.Ref)
	if err != nil {
		return nil, "", err
	}
	data, err := gitShowFile(repoPath, resolvedRef, parsed.WorkflowPath)
	if err != nil {
		return nil, "", err
	}
	return data, resolvedRef, nil
}

func readFileWorkflowFromRemoteRepo(parsed parsedGitWorkflowSelector) ([]byte, string, error) {
	repoDir, err := os.MkdirTemp("", "c2-gha-selector-*")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(repoDir)

	if _, err := runGit(context.Background(), "", "init", repoDir); err != nil {
		return nil, "", err
	}
	if _, err := runGit(context.Background(), repoDir, "remote", "add", "origin", parsed.RepoURL); err != nil {
		return nil, "", err
	}
	if _, err := runGit(context.Background(), repoDir, "fetch", "--depth", "1", "origin", parsed.Ref); err != nil {
		return nil, "", err
	}
	resolvedRef, err := resolveGitRef(repoDir, "FETCH_HEAD")
	if err != nil {
		return nil, "", err
	}
	data, err := gitShowFile(repoDir, resolvedRef, parsed.WorkflowPath)
	if err != nil {
		return nil, "", err
	}
	return data, resolvedRef, nil
}

func resolveGitRef(repoPath string, ref string) (string, error) {
	output, err := runGit(context.Background(), repoPath, "rev-parse", strings.TrimSpace(ref))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(output), nil
}

func gitShowFile(repoPath string, ref string, relPath string) ([]byte, error) {
	output, err := runGit(context.Background(), repoPath, "show", fmt.Sprintf("%s:%s", strings.TrimSpace(ref), relPath))
	if err != nil {
		return nil, err
	}
	return []byte(output), nil
}

func materializeExternalWorkflow(selector string, relPath string, data []byte) (string, error) {
	root, err := os.MkdirTemp("", "c2-gha-workflow-*")
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return "", err
	}
	return target, nil
}

func normalizeSelectorPath(rawPath string) (string, error) {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return "", fmt.Errorf("path cannot be empty")
	}
	if strings.HasPrefix(trimmed, "/") {
		return "", fmt.Errorf("path must be relative")
	}
	parts := strings.Split(trimmed, "/")
	for _, segment := range parts {
		if segment == "" {
			return "", fmt.Errorf("path must not contain empty segments")
		}
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." {
		return "", fmt.Errorf("path must target a file")
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("path must not escape the repository")
	}
	return cleaned, nil
}

func resolveUnderBase(baseDir string, relPath string) (string, error) {
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	target := filepath.Clean(filepath.Join(baseAbs, filepath.FromSlash(relPath)))
	rel, err := filepath.Rel(baseAbs, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes repository root")
	}
	return target, nil
}

func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
