package cortex

import (
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
)

const defaultTenantID = "1"

type Config struct {
	Addr            string
	JobDBURL        string
	WorkingDir      string
	DefaultTenantID string
	CORSOrigins     []string
	StaticFS        fs.FS
	HTTPClient      *http.Client
	Logger          *slog.Logger
}

type Server struct {
	cfg     Config
	engine  jobworkflow.Engine
	router  http.Handler
	story   storyService
	cells   *cellCatalog
	logger  *slog.Logger
	started time.Time
}

func normalizeConfig(cfg Config) Config {
	if strings.TrimSpace(cfg.Addr) == "" {
		cfg.Addr = ":8080"
	}
	if strings.TrimSpace(cfg.WorkingDir) == "" {
		cfg.WorkingDir = "."
	}
	abs, err := filepath.Abs(cfg.WorkingDir)
	if err == nil {
		cfg.WorkingDir = abs
	}
	if strings.TrimSpace(cfg.DefaultTenantID) == "" {
		cfg.DefaultTenantID = defaultTenantID
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	return cfg
}

func envOrDefault(name string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func SplitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
