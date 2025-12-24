package starter

import (
	"context"

	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/strata-go/pkg/client/artifact"
	"github.com/colony-2/swf-go/pkg/swf"
	"gopkg.in/yaml.v3"
)

const (
	RecipeJobType        = "recipe"
	RecipeArtifactSuffix = ".recipe.yaml"
)

func StartRecipeJob(ctx context.Context, startJob workflowctl.StartJob, engine swf.SWFEngine, recipes ...recipe.Recipe) (swf.JobKey, error) {
	artifacts := make([]artifact.Artifact, len(recipes))
	for i, r := range recipes {
		recipeYaml, err := yaml.Marshal(&r)
		if err != nil {
			return swf.JobKey{}, err
		}
		name := r.GetMetadata().ID + RecipeArtifactSuffix
		artifacts[i] = artifact.FromBytes(name, "", recipeYaml)
	}

	inputData, err := swf.NewTaskData(startJob, artifacts...)

	if err != nil {
		return swf.JobKey{}, err
	}

	job := swf.StartJob{
		TenantId:  startJob.TenantId,
		JobType:   RecipeJobType,
		Data:      inputData,
		RunPolicy: swf.DefaultRunPolicy(),
	}
	return engine.StartJob(ctx, job)
}
