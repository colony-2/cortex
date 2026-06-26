package cortex

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/colony-2/c2j/pkg/input"
	"github.com/colony-2/c2j/pkg/ops"
	"github.com/colony-2/c2j/pkg/story"
	workerworkflow "github.com/colony-2/c2j/pkg/worker/workflow"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
	"github.com/gorilla/mux"
)

type storyService = story.Service

func NewServer(cfg Config, engine jobworkflow.Engine) (*Server, error) {
	cfg = normalizeConfig(cfg)
	if engine == nil {
		return nil, fmt.Errorf("workflow engine is required")
	}

	cells := &cellCatalog{
		workingDir:       cfg.WorkingDir,
		defaultTenantID:  cfg.DefaultTenantID,
		defaultProjectID: cfg.DefaultTenantID,
	}
	projects := &tenantProjectService{cells: cells, defaultTenantID: cfg.DefaultTenantID}
	storySvc, err := story.New(story.ServiceConfig{
		Engine:   engine,
		Cells:    cells,
		Projects: projects,
	})
	if err != nil {
		return nil, err
	}

	ctl := &workerworkflow.SWFWorkflowControl{
		Engine:                        engine,
		PreferRuntimeRecipeResolution: true,
	}
	sse := input.NewSimpleSSEManager()
	inputService := input.GetOp().GetManagementService()
	if err := inputService.Initialize(ops.NewServiceDepsBuilder().
		WithWorkflowControl(ctl).
		WithSSEManager(sse).
		Build()); err != nil {
		return nil, err
	}

	s := &Server{
		cfg:     cfg,
		engine:  engine,
		story:   storySvc,
		cells:   cells,
		logger:  cfg.Logger,
		started: time.Now().UTC(),
	}
	s.router = s.buildRouter(inputService)
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) buildRouter(inputService ops.ManagementService) http.Handler {
	r := mux.NewRouter()
	r.Use(s.corsMiddleware)

	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/health", s.handleHealth).Methods(http.MethodGet, http.MethodOptions)
	api.HandleFunc("/projects", s.handleListProjects).Methods(http.MethodGet, http.MethodOptions)
	api.HandleFunc("/projects/{projectId}", s.handleGetProject).Methods(http.MethodGet, http.MethodOptions)
	api.HandleFunc("/projects/{projectId}/cells", s.handleListCells).Methods(http.MethodGet, http.MethodOptions)
	api.HandleFunc("/projects/{projectId}/cells/self", s.handleSelfCell).Methods(http.MethodGet, http.MethodOptions)
	api.HandleFunc("/projects/{projectId}/jobs", s.handleListJobs).Methods(http.MethodGet, http.MethodOptions)
	api.HandleFunc("/projects/{projectId}/jobs", s.handleSubmitJob).Methods(http.MethodPost, http.MethodOptions)
	api.HandleFunc("/projects/{projectId}/jobs/{jobId}", s.handleGetJob).Methods(http.MethodGet, http.MethodOptions)
	api.HandleFunc("/projects/{projectId}/jobs/{jobId}/restart", s.handleRestartJob).Methods(http.MethodPost, http.MethodOptions)
	api.HandleFunc("/projects/{projectId}/jobs/{jobId}/story", s.handleJobStory).Methods(http.MethodGet, http.MethodOptions)
	api.HandleFunc("/projects/{projectId}/jobs/{jobId}/outcome", s.handleJobOutcome).Methods(http.MethodGet, http.MethodOptions)
	api.HandleFunc("/projects/{projectId}/jobs/{jobId}/tasks/{taskOrdinal}/artifacts/{artifactName:.+}", s.handleArtifactByOrdinal).Methods(http.MethodGet, http.MethodOptions)

	for _, route := range inputService.GetRoutes() {
		r.HandleFunc(route.Path, route.Handler).Methods(route.Method, http.MethodOptions)
	}

	if s.cfg.StaticFS != nil {
		r.PathPrefix("/").Handler(spaHandler(s.cfg.StaticFS))
	}
	return r
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := allowedOrigin(origin, s.cfg.CORSOrigins)
		if allowed != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowed)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func allowedOrigin(origin string, configured []string) string {
	if len(configured) == 0 {
		configured = []string{"http://localhost:3000", "http://127.0.0.1:3000"}
	}
	for _, candidate := range configured {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" {
			return "*"
		}
		if origin != "" && strings.EqualFold(candidate, origin) {
			return origin
		}
	}
	return ""
}
