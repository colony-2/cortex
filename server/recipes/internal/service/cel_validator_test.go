package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/colony-2/colony2/server/project/pkg/project"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-template/pkg/funcregistry"
	"github.com/stretchr/testify/require"
)

func TestRecipeWorkerCELValidator_PassesProjectIDToTemplateHelpers(t *testing.T) {
	originalOps := coreops.List()
	coreops.Clear()
	coreops.Register(coreops.NewActivityMappedOpV2[map[string]interface{}, map[string]interface{}](
		coreops.OpMetadata{Type: "echo"},
		func(_ coreops.OpDependencies, _ context.Context, in map[string]interface{}) (map[string]interface{}, error) {
			return in, nil
		},
	))
	t.Cleanup(func() {
		coreops.Clear()
		if len(originalOps) > 0 {
			coreops.Register(originalOps...)
		}
	})

	const expectedProjectID = "proj-validate-cells"

	builder := funcregistry.NewBuilder().WithDefaults()
	funcregistry.AddZeroFuncWithContext(builder, "cells", func(_ context.Context, taskCtx contextual.TaskExecutionContext) ([]funcregistry.CELCell, error) {
		if taskCtx.Workflow.ProjectId != expectedProjectID {
			return nil, fmt.Errorf("cells: expected project_id %q, got %q", expectedProjectID, taskCtx.Workflow.ProjectId)
		}
		return []funcregistry.CELCell{
			{Name: "alpha", ID: "1", Path: "/cells/alpha"},
		}, nil
	})

	rec, err := recipe.LoadRecipeFromString([]byte(`version: "1.0"
id: "r1"
op: echo
inputs:
  payload: "{{ cells | to_json }}"
`))
	require.NoError(t, err)

	validator := NewRecipeWorkerCELValidatorWithProvider(coreops.NewServiceDepsBuilder().Build(), builder)
	errs, err := validator.ValidateCEL(context.Background(), project.ID(expectedProjectID), *rec)
	require.NoError(t, err)
	require.Len(t, errs, 0, "validation should not fail when project_id is propagated to template helpers")
}
