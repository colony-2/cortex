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

type OpDependenciesBuilder struct {
	db              *gorm.DB
	artifacts       []swf.Artifact
	workflowControl workflowctl.WorkflowControl
}

// NewOpDependenciesBuilder creates a new, empty builder instance.
func NewOpDependenciesBuilder() *OpDependenciesBuilder {
	return &OpDependenciesBuilder{
		artifacts: make([]swf.Artifact, 0), // Initialize inputArtifacts slice
	}
}

func (b *OpDependenciesBuilder) WithDatabase(db *gorm.DB) *OpDependenciesBuilder {
	b.db = db
	return b
}

func (b *OpDependenciesBuilder) WithArtifacts(initialArtifacts []swf.Artifact) *OpDependenciesBuilder {
	return b
}

func (b *OpDependenciesBuilder) WithWorkflowControl(wc workflowctl.WorkflowControl) *OpDependenciesBuilder {
	b.workflowControl = wc
	return b
}

func (b *OpDependenciesBuilder) Build() OpDependencies {
	deps := &opDepImpl{
		db:              b.db,
		inputArtifacts:  b.artifacts,
		workflowControl: b.workflowControl,
		outputArtifacts: make([]swf.Artifact, 0),
	}

	return deps, nil
}
