package cortex

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/colony-2/c2j/pkg/jobdbschema"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/remote"
	"github.com/colony-2/jobdb/pkg/jobdb/runtime/toy"
	jobworkflow "github.com/colony-2/jobdb/pkg/workflow"
)

func NewRemoteServer(cfg Config) (*Server, error) {
	cfg = normalizeConfig(cfg)
	if strings.TrimSpace(cfg.JobDBURL) == "" {
		return nil, fmt.Errorf("jobdb URL is required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	runtime, err := remote.New(cfg.JobDBURL, client)
	if err != nil {
		return nil, err
	}
	engine, err := jobworkflow.NewEngineBuilder().WithRuntime(runtime).WithWorkerTenantId(cfg.DefaultTenantID).BuildEngine()
	if err != nil {
		return nil, err
	}
	return NewServer(cfg, jobdbschema.WorkflowEngine{Engine: engine, Registry: runtime})
}

func NewUITestServer(cfg Config) (*Server, error) {
	cfg = normalizeConfig(cfg)
	runtime := toy.New()
	engine, err := jobworkflow.NewEngineBuilder().WithRuntime(runtime).WithWorkerTenantId(cfg.DefaultTenantID).BuildEngine()
	if err != nil {
		return nil, err
	}
	return NewServer(cfg, jobdbschema.WorkflowEngine{Engine: engine, Registry: runtime})
}
