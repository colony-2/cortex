package ops

import (
	"sync"

	"gorm.io/gorm"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
)

// serviceDependenciesSeal is an unexported type used to seal ServiceDependencies2 implementations
// so that only recipe-core can provide compliant values.
type serviceDependenciesSeal struct{}

// ServiceDependencies2 defines the typed dependencies exposed to management services
// and ops that require access to shared runtime collaborators.
type ServiceDependencies2 interface {
	// WorkflowControl returns a typed workflow controller if available.
	WorkflowControl() (workflowctl.WorkflowControl, bool)

	// SSEManager returns the server-sent-event manager when supported.
	SSEManager() (SSEManager, bool)

	// Database returns the shared GORM handle for the current runtime when available.
	Database() (*gorm.DB, bool)

	// serviceDependenciesMarker seals the interface to recipe-core implementations.
	serviceDependenciesMarker() serviceDependenciesSeal
}

// ServiceDepsBuilder constructs ServiceDependencies2 instances via a fluent API.
type ServiceDepsBuilder struct {
	workflowCtl workflowctl.WorkflowControl
	sseManager  SSEManager
	database    *gorm.DB

	once   sync.Once
	result ServiceDependencies2
}

// NewServiceDepsBuilder returns a new builder for ServiceDependencies2 implementations.
func NewServiceDepsBuilder() *ServiceDepsBuilder { return &ServiceDepsBuilder{} }

// WithWorkflowControl configures the workflow controller dependency.
func (b *ServiceDepsBuilder) WithWorkflowControl(ctl workflowctl.WorkflowControl) *ServiceDepsBuilder {
	b.workflowCtl = ctl
	return b
}

// WithSSEManager configures the SSE manager dependency.
func (b *ServiceDepsBuilder) WithSSEManager(mgr SSEManager) *ServiceDepsBuilder {
	b.sseManager = mgr
	return b
}

// WithDatabase configures the shared database handle. Passing nil leaves the dependency unset.
func (b *ServiceDepsBuilder) WithDatabase(db *gorm.DB) *ServiceDepsBuilder {
	b.database = db
	return b
}

// Build materializes the immutable ServiceDependencies2 instance.
func (b *ServiceDepsBuilder) Build() ServiceDependencies2 {
	b.once.Do(func() {
		b.result = &serviceDependencies{
			workflowCtl: b.workflowCtl,
			sseManager:  b.sseManager,
			database:    b.database,
		}
	})
	if b.result == nil {
		panic("ServiceDepsBuilder.Build called multiple times concurrently")
	}
	return b.result
}

type serviceDependencies struct {
	workflowCtl workflowctl.WorkflowControl
	sseManager  SSEManager
	database    *gorm.DB
}

func (d *serviceDependencies) WorkflowControl() (workflowctl.WorkflowControl, bool) {
	if d == nil || d.workflowCtl == nil {
		return nil, false
	}
	return d.workflowCtl, true
}

func (d *serviceDependencies) SSEManager() (SSEManager, bool) {
	if d == nil || d.sseManager == nil {
		return nil, false
	}
	return d.sseManager, true
}

func (d *serviceDependencies) Database() (*gorm.DB, bool) {
	if d == nil || d.database == nil {
		return nil, false
	}
	return d.database, true
}

func (d *serviceDependencies) serviceDependenciesMarker() serviceDependenciesSeal {
	return serviceDependenciesSeal{}
}
