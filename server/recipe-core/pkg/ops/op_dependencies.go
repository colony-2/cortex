package ops

import (
	"errors"

	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/swf-go/pkg/swf"
	"gorm.io/gorm"
)

type OpDependencies interface {
	Database() *gorm.DB
	WorkflowControl() workflowctl.WorkflowControl
	GetInputArtifacts() []swf.Artifact
	AddOutputArtifact(swf.Artifact) error
	GetOutputArtifacts() []swf.Artifact
	WorktreePath() string
	JobTool() JobTool
	FindArtifact(key swf.ArtifactKey) (swf.Artifact, error)
}

// JobTool provides a way to influence the current jobs operation. It is separate from workflowcontrol, which is about running jobs independent of this jobs context.
type JobTool interface {
	GetJobKey() swf.JobKey
	AwaitJobs(jobIds ...string) error
}

type TaskBasedJobTool struct {
	TaskContext swf.TaskContext
}

func (j *TaskBasedJobTool) GetJobKey() swf.JobKey {
	return j.TaskContext.JobKey
}

func (j *TaskBasedJobTool) AwaitJobs(jobIds ...string) error {
	return j.TaskContext.AwaitJobs(jobIds...)
}

// opDepImpl holds the actual dependencies.
type opDepImpl struct {
	db              *gorm.DB
	inputArtifacts  []swf.Artifact
	outputArtifacts []swf.Artifact
	workflowControl workflowctl.WorkflowControl
	worktreePath    string
	jobTool         JobTool
}

func (c *opDepImpl) FindArtifact(key swf.ArtifactKey) (swf.Artifact, error) {
	var found swf.Artifact
	for _, artifact := range c.GetInputArtifacts() {
		if artifact.Name() == key.Name {
			if found != nil {
				return nil, errors.New("duplicate artifact found")
			}
			found = artifact
		}
	}

	if found == nil {
		return nil, errors.New("artifact not found")
	}
	return found, nil
}

func (c *opDepImpl) GetOutputArtifacts() []swf.Artifact {
	out := make([]swf.Artifact, len(c.outputArtifacts))
	copy(out, c.outputArtifacts)
	return out
}

func (c *opDepImpl) JobTool() JobTool {
	return c.jobTool
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

// WorktreePath implements the OpDependencies interface.
func (c *opDepImpl) WorktreePath() string {
	return c.worktreePath
}

type OpDependenciesBuilder struct {
	db              *gorm.DB
	artifacts       []swf.Artifact
	workflowControl workflowctl.WorkflowControl
	worktreePath    string
	jobTool         JobTool
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

func (b *OpDependenciesBuilder) WithTaskContext(tc swf.TaskContext) *OpDependenciesBuilder {
	b.jobTool = &TaskBasedJobTool{tc}
	return b
}

func (b *OpDependenciesBuilder) WithJobTool(jt JobTool) *OpDependenciesBuilder {
	b.jobTool = jt
	return b
}

func (b *OpDependenciesBuilder) WithArtifacts(initialArtifacts []swf.Artifact) *OpDependenciesBuilder {
	if initialArtifacts == nil {
		b.artifacts = make([]swf.Artifact, 0)
		return b
	}
	out := make([]swf.Artifact, len(initialArtifacts))
	copy(out, initialArtifacts)
	b.artifacts = out
	return b
}

func (b *OpDependenciesBuilder) WithWorkflowControl(wc workflowctl.WorkflowControl) *OpDependenciesBuilder {
	b.workflowControl = wc
	return b
}

func (b *OpDependenciesBuilder) WithWorktreePath(path string) *OpDependenciesBuilder {
	b.worktreePath = path
	return b
}

func (b *OpDependenciesBuilder) Build() OpDependencies {
	deps := &opDepImpl{
		db:              b.db,
		inputArtifacts:  b.artifacts,
		workflowControl: b.workflowControl,
		worktreePath:    b.worktreePath,
		outputArtifacts: make([]swf.Artifact, 0),
		jobTool:         b.jobTool,
	}

	return deps
}
