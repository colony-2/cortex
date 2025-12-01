package compiler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/colony-2/strata/strata-go/pkg/client/artifact"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/workflow"
	workerops "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"gopkg.in/yaml.v3"
)

type recipeWorkerImpl struct {
	activityRegistry *workerops.ActivityRegistry
	recipes          *recipeRetriever
}

const (
	RecipeJobType        = "recipe"
	RecipeArtifactSuffix = ".recipe.yaml"
)

func NewRecipeWorker(activityRegistry *workerops.ActivityRegistry) (*swf.WorkSet, error) {
	job := &recipeWorkerImpl{
		activityRegistry: activityRegistry,
	}
	return swf.AsWorkSet(job, activityRegistry.GetTaskWorkers()...)
}

func (j recipeWorkerImpl) Name() string {
	return RecipeJobType
}

func (j recipeWorkerImpl) Run(ctx swf.JobContext, jobData swf.JobData) (swf.JobData, error) {
	artifacts, err := jobData.GetArtifacts()
	if err != nil {
		return nil, err
	}
	j.recipes = newRetriever(artifacts)

	data, err := jobData.GetData()
	if err != nil {
		return nil, err
	}
	input := make(map[string]interface{})
	err = json.Unmarshal(data, &input)
	if err != nil {
		return nil, err
	}
	recipeName, ok := input["recipe"]
	if !ok {
		if len(artifacts) == 1 {
			recipeName = strings.TrimSuffix(artifacts[0].Name(), RecipeArtifactSuffix)
			input["recipe"] = recipeName
		} else {
			return nil, fmt.Errorf("missing recipe name")
		}
	}

	name, _ := recipeName.(string)
	if name == "" {
		return nil, fmt.Errorf("missing recipe name")
	}

	r, err := j.recipes.GetRecipe(name)
	if err != nil {
		return nil, err
	}

	wCtx := workflow.Context{JobContext: ctx}
	out, err := ExecuteRecipe(wCtx, j.activityRegistry, r, input)
	if err != nil {
		return nil, err
	}
	taskData, err := swf.NewTaskData(out)
	if err != nil {
		return nil, err
	}
	return swf.JobData(taskData), nil

}

var _ swf.JobWorker = &recipeWorkerImpl{}

func StartRecipeJob(ctx context.Context, input map[string]interface{}, engine swf.SWFEngine, recipes ...recipe.Recipe) (swf.JobId, error) {
	artifacts := make([]artifact.Artifact, len(recipes))
	for i, r := range recipes {
		recipeYaml, err := yaml.Marshal(&r)
		if err != nil {
			return "", err
		}
		name := r.GetMetadata().ID + RecipeArtifactSuffix
		artifacts[i] = artifact.FromBytes(name, "", recipeYaml)
	}

	inputData, err := swf.NewTaskData(input, artifacts...)

	if err != nil {
		return "", err
	}

	job := swf.StartJob{
		JobType:   RecipeJobType,
		Data:      inputData,
		RunPolicy: swf.DefaultRunPolicy(),
	}
	return engine.StartJob(ctx, job)
}
