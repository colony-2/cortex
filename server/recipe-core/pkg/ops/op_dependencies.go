package ops

import (
	"errors"

	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
	"gorm.io/gorm"
)

type OpDependencies interface {
	Database() *gorm.DB
	WorkflowControl() workflowctl.WorkflowControl
	GetInputArtifacts() []swf.Artifact
	AddOutputArtifact(swf.Artifact) error
	GetOutputArtifacts() []swf.Artifact
}

// opDepImpl holds the actual dependencies.
type opDepImpl struct {
	db              *gorm.DB
	inputArtifacts  []swf.Artifact
	outputArtifacts []swf.Artifact
	workflowControl workflowctl.WorkflowControl
}

func (c *opDepImpl) GetOutputArtifacts() []swf.Artifact {
	out := make([]swf.Artifact, len(c.outputArtifacts))
	copy(out, c.outputArtifacts)
	return out
}

// Database implements the OpDependencies interface.
func (c *opDepImpl) Database() *gorm.DB {
	return c.db
}

// AddArtifact implements the OpDependencies interface.
func (c *opDepImpl) AddOutputArtifact(a swf.Artifact) error {
	if a == nil {
		return errors.New("cannot add nil artifact")
	}
	c.outputArtifacts = append(c.outputArtifacts, a)
	// Log/side effect removed for simplicity
	return nil
}

// GetInputArtifacts implements the OpDependencies interface.
func (c *opDepImpl) GetInputArtifacts() []swf.Artifact {
	return c.inputArtifacts
}

// WorkflowControl implements the OpDependencies interface.
func (c *opDepImpl) WorkflowControl() workflowctl.WorkflowControl {
	return c.workflowControl
}

// --- THE BUILDER STRUCT AND METHODS ---

// OpDependenciesBuilder is the struct that collects the configuration steps.
type OpDependenciesBuilder struct {
	db              *gorm.DB
	artifacts       []swf.Artifact
	workflowControl workflowctl.WorkflowControl
	err             error // To track any configuration errors
}

// NewOpDependenciesBuilder creates a new, empty builder instance.
func NewOpDependenciesBuilder() *OpDependenciesBuilder {
	return &OpDependenciesBuilder{
		artifacts: make([]swf.Artifact, 0), // Initialize inputArtifacts slice
	}
}

// WithDatabase sets the GORM database connection.
// It uses a pointer receiver to mutate the builder and returns the builder for chaining.
func (b *OpDependenciesBuilder) WithDatabase(db *gorm.DB) *OpDependenciesBuilder {
	if db == nil {
		b.err = errors.Join(b.err, errors.New("database connection cannot be nil"))
		return b
	}
	b.db = db
	return b
}

// WithArtifacts sets an initial list of inputArtifacts.
// This is useful for pre-loading state.
func (b *OpDependenciesBuilder) WithArtifacts(initialArtifacts []swf.Artifact) *OpDependenciesBuilder {
	if initialArtifacts != nil {
		b.artifacts = append(b.artifacts, initialArtifacts...)
	}
	return b
}

// WithWorkflowControl sets the workflow control dependency.
func (b *OpDependenciesBuilder) WithWorkflowControl(wc workflowctl.WorkflowControl) *OpDependenciesBuilder {
	if wc == nil {
		b.err = errors.Join(b.err, errors.New("workflow control cannot be nil"))
		return b
	}
	b.workflowControl = wc
	return b
}

// Build finalizes the construction and returns the OpDependencies interface.
// It performs validation and returns the concrete object or an error.
func (b *OpDependenciesBuilder) Build() (OpDependencies, error) {
	// 1. Check for configuration errors accumulated during chaining
	if b.err != nil {
		return nil, b.err
	}

	// 3. Construct and return the concrete implementation
	deps := &opDepImpl{
		db:              b.db,
		inputArtifacts:  b.artifacts,
		workflowControl: b.workflowControl,
		outputArtifacts: make([]swf.Artifact, 0),
	}

	return deps, nil
}
