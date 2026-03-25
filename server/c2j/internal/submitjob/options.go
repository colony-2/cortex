package submitjob

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/colony-2/colony2/server/c2j/internal/defaults"
)

type Options struct {
	TenantID   string
	SWFURL     string
	Recipe     string
	RecipeFile string
	RecipesDir string

	InputsJSON string
	InputsFile string

	RepoPath   string
	GitRef     string
	CellPath   string
	CellName   string
	ActorEmail string
	TicketID   string

	JSONOutput bool
	Stdout     io.Writer
	Stderr     io.Writer
}

func (o *Options) Complete() {
	if o.SWFURL == "" {
		o.SWFURL = strings.TrimSpace(os.Getenv(defaults.SWFEnv))
	}
	if o.SWFURL == "" {
		o.SWFURL = defaults.SWFURL
	}
	if o.TenantID == "" {
		o.TenantID = strings.TrimSpace(os.Getenv(defaults.TenantEnv))
	}
	if o.TenantID == "" {
		o.TenantID = defaults.TenantID
	}
	if strings.TrimSpace(o.Recipe) != "" && strings.TrimSpace(o.RecipesDir) == "" {
		o.RecipesDir = strings.TrimSpace(os.Getenv(defaults.RecipesDirEnv))
		if o.RecipesDir == "" {
			o.RecipesDir = "."
		}
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	if strings.TrimSpace(o.RepoPath) == "" {
		if cwd, err := os.Getwd(); err == nil {
			o.RepoPath = cwd
		}
	}
	if strings.TrimSpace(o.RepoPath) != "" {
		if absPath, err := filepath.Abs(o.RepoPath); err == nil {
			o.RepoPath = absPath
		}
	}
	if strings.TrimSpace(o.CellPath) == "" {
		o.CellPath = "."
	}
	if strings.TrimSpace(o.CellName) == "" {
		clean := filepath.Clean(o.CellPath)
		if clean == "." || clean == string(filepath.Separator) {
			o.CellName = "."
		} else {
			o.CellName = filepath.Base(clean)
		}
	}
	if strings.TrimSpace(o.GitRef) == "" {
		if gitRef, err := resolveGitRef(o.RepoPath); err == nil {
			o.GitRef = gitRef
		}
	}
}

func (o Options) Validate() error {
	if strings.TrimSpace(o.TenantID) == "" {
		return fmt.Errorf("--tenant-id is required (or %s)", defaults.TenantEnv)
	}
	if strings.TrimSpace(o.SWFURL) == "" {
		return fmt.Errorf("--swf-url is required (or %s)", defaults.SWFEnv)
	}
	if strings.TrimSpace(o.Recipe) == "" && strings.TrimSpace(o.RecipeFile) == "" {
		return fmt.Errorf("either --recipe or --recipe-file is required")
	}
	if strings.TrimSpace(o.Recipe) != "" && strings.TrimSpace(o.RecipeFile) != "" {
		return fmt.Errorf("--recipe and --recipe-file are mutually exclusive")
	}
	if strings.TrimSpace(o.InputsJSON) != "" && strings.TrimSpace(o.InputsFile) != "" {
		return fmt.Errorf("--inputs-json and --inputs-file are mutually exclusive")
	}
	return nil
}

func resolveGitRef(repoPath string) (string, error) {
	repoPath = strings.TrimSpace(repoPath)
	if repoPath == "" {
		return "", fmt.Errorf("repo path is required")
	}

	cmd := exec.Command("git", "-C", repoPath, "rev-parse", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("resolve git ref in %s: %w", repoPath, err)
	}
	return strings.TrimSpace(string(out)), nil
}
