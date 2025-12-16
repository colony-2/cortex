package ops

import (
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/swf-go/pkg/swf"
	"gorm.io/gorm"
)

// testDeps is a minimal OpDependencies test double.
type testDeps struct {
	artifacts []swf.Artifact
}

func (d *testDeps) AddOutputArtifact(artifact swf.Artifact) error {
	return nil
}

func (d *testDeps) GetOutputArtifacts() []swf.Artifact {
	return nil
}

func (d *testDeps) Database() *gorm.DB { return nil }

func (d *testDeps) AddArtifact(a swf.Artifact) error {
	d.artifacts = append(d.artifacts, a)
	return nil
}

func (d *testDeps) GetInputArtifacts() []swf.Artifact { return d.artifacts }

func (d *testDeps) WorkflowControl() workflowctl.WorkflowControl { return nil }

var _ OpDependencies = &testDeps{}
