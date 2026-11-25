package cortex

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/c2j/pkg/jobinspect"
	"github.com/colony-2/jobdb/pkg/jobdb"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
)

const defaultTenantID = "1"
const DefaultPulseURL = "http://localhost:8080"

type Config struct {
	Addr            string
	JobDBURL        string
	PulseURL        string
	WorkingDir      string
	DefaultTenantID string
	CORSOrigins     []string
	StaticFS        fs.FS
	HTTPClient      *http.Client
	Logger          *slog.Logger
	// Deprecated: retained for Go configuration compatibility; saved diff reads do not reconstruct Git history.
	Changes jobinspect.ChangesConfig
}

type crashConcernRuntime interface {
	ClearCrashConcern(context.Context, jobdb.ClearCrashConcernRequest) error
}

type Server struct {
	threadCursors  threadCursorCache
	crashConcern   crashConcernRuntime
	timing         pendingTiming
	cfg            Config
	engine         jobworkflow.Engine
	jobDeletion    jobdb.JobDeletionRuntime
	chapters       chapterReader
	documentClient *http.Client
	leaseEvents    jobdb.LeaseEventReader
	workspaceDiff  func(string) (jobinspect.WorkspaceDiffAPI, error)
	router         http.Handler
	story          storyService
	inputs         inputRuntime
	history        input.HistoryService
	cells          jobdb.CellRuntime
	configuration  jobdb.ConfigurationAdmin
	secretAdmin    secretAdmin
	logger         *slog.Logger
	started        time.Time
}

func normalizeConfig(cfg Config) Config {
	if strings.TrimSpace(cfg.PulseURL) == "" {
		cfg.PulseURL = DefaultPulseURL
	}
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
