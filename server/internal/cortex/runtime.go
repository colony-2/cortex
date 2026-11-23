package cortex

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/colony-2/c2j/pkg/jobdbschema"
	"github.com/colony-2/colony2/server/internal/testjobdb"
	"github.com/colony-2/jobdb/pkg/jobdb"
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
	clientRuntime, err := remote.New(cfg.JobDBURL, client)
	if err != nil {
		return nil, err
	}
	runtime := artifactNameRuntime{clientRuntime}
	engine, err := jobworkflow.NewEngineBuilder().WithRuntime(runtime).WithWorkerTenantId(cfg.DefaultTenantID).BuildEngine()
	if err != nil {
		return nil, err
	}
	server, err := NewServer(cfg, jobdbschema.WorkflowEngine{Engine: engine, Registry: runtime})
	if err == nil {
		server.documentClient = client
		server.jobDeletion = runtime
		server.chapters = runtime
		server.leaseEvents = runtime
		server.crashConcern = runtime
		server.configureChanges(runtime)
		server.configuration = runtime
		server.secretAdmin = &remoteSecretAdmin{base: strings.TrimRight(cfg.JobDBURL, "/"), client: client}
	}
	return server, err
}

// The pinned remote server binds artifact path parameters after net/http has
// already decoded canonical paths. Protect literal percent signs from that
// second decode. Noncanonical escapes (including encoded slashes) retain
// RawPath, so those names are decoded only once and pass through unchanged.
// Keep this compatibility shim
// covered against the native remote server when updating the JobDB pin.
type artifactNameRuntime struct{ *remote.Runtime }

func (r artifactNameRuntime) OpenArtifact(ctx context.Context, ref jobdb.ArtifactRef) (jobdb.ArtifactReader, error) {
	name := ref.Name
	if url.PathEscape(name) == (&url.URL{Path: name}).EscapedPath() {
		ref.Name = strings.ReplaceAll(name, "%", "%25")
	}
	reader, err := r.Runtime.OpenArtifact(ctx, ref)
	if err != nil {
		return nil, err
	}
	return namedArtifactReader{ArtifactReader: reader, name: name}, nil
}

type namedArtifactReader struct {
	jobdb.ArtifactReader
	name string
}

func (r namedArtifactReader) Name() string { return r.name }

func NewUITestServer(cfg Config) (*Server, error) {
	cfg = normalizeConfig(cfg)
	runtime := toy.New()
	engine, err := jobworkflow.NewEngineBuilder().WithRuntime(runtime).WithWorkerTenantId(cfg.DefaultTenantID).BuildEngine()
	if err != nil {
		return nil, err
	}
	server, err := NewServer(cfg, jobdbschema.WorkflowEngine{Engine: engine, Registry: runtime})
	if err == nil {
		server.router = testjobdb.Handler(server.router)
		server.jobDeletion = runtime
		server.chapters = runtime
		server.leaseEvents = runtime
		server.crashConcern = runtime
		server.configureChanges(runtime)
		server.configuration = runtime
	}
	return server, err
}
