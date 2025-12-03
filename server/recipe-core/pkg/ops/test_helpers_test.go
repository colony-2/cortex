package ops

import (
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflowctl"
	"gorm.io/gorm"
)

// testDeps is a minimal OpDependencies test double.
type testDeps struct {
	artifacts []swf.Artifact
}

func (d *testDeps) Database() *gorm.DB { return nil }

func (d *testDeps) AddArtifact(a swf.Artifact) error {
	d.artifacts = append(d.artifacts, a)
	return nil
}

func (d *testDeps) GetArtifacts() []swf.Artifact { return d.artifacts }

func (d *testDeps) WorkflowControl() workflowctl.WorkflowControl { return nil }
