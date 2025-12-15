package testfixtures

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	recipeops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
)

type testWriteFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type testWriteFileOutput struct {
	Path string `json:"path"`
}

type testReadFileInput struct {
	Path string `json:"path"`
}

type testReadFileOutput struct {
	Content string `json:"content"`
}

type testGitCommitInput struct {
	RepoPath string `json:"repo_path"`
	Message  string `json:"message"`
}

type testGitCommitOutput struct {
	CommitHash string `json:"commit_hash"`
}

func init() {
	recipeops.Register(
		recipeops.NewActivityMappedOpV2[testWriteFileInput, testWriteFileOutput](
			recipeops.OpMetadata{
				Type:        "test_write_file",
				Description: "writes file contents for git workspace tests",
				Version:     "1.0.0",
			},
			func(_ recipeops.OpDependencies, ctx context.Context, input testWriteFileInput) (testWriteFileOutput, error) {
				if input.Path == "" {
					return testWriteFileOutput{}, fmt.Errorf("path is required")
				}
				if err := os.MkdirAll(filepath.Dir(input.Path), 0o755); err != nil {
					return testWriteFileOutput{}, err
				}
				if err := os.WriteFile(input.Path, []byte(input.Content), 0o644); err != nil {
					return testWriteFileOutput{}, err
				}
				return testWriteFileOutput{Path: input.Path}, nil
			},
		),
		recipeops.NewActivityMappedOpV2[testReadFileInput, testReadFileOutput](
			recipeops.OpMetadata{
				Type:        "test_read_file",
				Description: "reads file contents from git workspace for tests",
				Version:     "1.0.0",
			},
			func(_ recipeops.OpDependencies, ctx context.Context, input testReadFileInput) (testReadFileOutput, error) {
				if input.Path == "" {
					return testReadFileOutput{}, fmt.Errorf("path is required")
				}
				data, err := os.ReadFile(input.Path)
				if err != nil {
					return testReadFileOutput{}, err
				}
				return testReadFileOutput{Content: string(data)}, nil
			},
		),
		recipeops.NewActivityMappedOpV2[testGitCommitInput, testGitCommitOutput](
			recipeops.OpMetadata{
				Type:        "test_git_commit_all",
				Description: "stages all changes and creates a git commit for tests",
				Version:     "1.0.0",
			},
			func(_ recipeops.OpDependencies, ctx context.Context, input testGitCommitInput) (testGitCommitOutput, error) {
				if input.RepoPath == "" {
					return testGitCommitOutput{}, fmt.Errorf("repo_path is required")
				}
				if input.Message == "" {
					return testGitCommitOutput{}, fmt.Errorf("message is required")
				}
				if err := runGitCmd(ctx, input.RepoPath, "add", "-A"); err != nil {
					return testGitCommitOutput{}, err
				}
				if err := runGitCmd(ctx, input.RepoPath, "commit", "-m", input.Message); err != nil {
					return testGitCommitOutput{}, err
				}
				hash, err := gitOutput(ctx, input.RepoPath, "rev-parse", "HEAD")
				if err != nil {
					return testGitCommitOutput{}, err
				}
				return testGitCommitOutput{CommitHash: hash}, nil
			},
		),
	)
}

func runGitCmd(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v failed: %w (%s)", args, err, out)
	}
	return nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %v failed: %w (%s)", args, err, out)
	}
	return strings.TrimSpace(string(out)), nil
}
