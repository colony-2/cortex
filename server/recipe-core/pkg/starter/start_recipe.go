package starter

import (
	"context"

	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/swf-go/pkg/swf"
	"gopkg.in/yaml.v3"
)

const (
	RecipeJobType        = "recipe"
	RecipeArtifactSuffix = ".recipe.yaml"
)

func StartRecipeJob(ctx context.Context, startJob workflowctl.StartJob, engine swf.SWFEngine, recipes ...recipe.Recipe) (swf.JobKey, error) {
	recipeCount := len(recipes)
	artifacts := make([]swf.Artifact, recipeCount+len(startJob.Artifacts))
	for i, r := range recipes {
		recipeYaml, err := yaml.Marshal(&r)
		if err != nil {
			return swf.JobKey{}, err
		}
		name := r.GetMetadata().ID + RecipeArtifactSuffix
		artifacts[i] = swf.NewArtifactFromBytes(name, recipeYaml)
	}

	for i, a := range startJob.Artifacts {
		artifacts[recipeCount+i] = a
	}

	inputData, err := swf.NewTaskData(startJob, artifacts...)

	if err != nil {
		return swf.JobKey{}, err
	}

	job := swf.StartJob{
		TenantId:     startJob.TenantId,
		JobType:      RecipeJobType,
		SingletonKey: startJob.SingletonKey,
		Data:         inputData,
		RunPolicy:    swf.DefaultRunPolicy(),
	}
	return engine.StartJob(ctx, job)
}
