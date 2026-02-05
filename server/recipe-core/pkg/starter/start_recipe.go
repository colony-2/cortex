package starter

import (
	"context"
	"encoding/json"

	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/swf-go/pkg/swf"
	"gopkg.in/yaml.v3"
)

const (
	RecipeJobType        = "recipe"
	RecipeArtifactSuffix = ".recipe.yaml"
)

const (
	JobMetadataVersion = 1

	MetaFieldVersion    swf.FieldName = "v"
	MetaFieldRecipe     swf.FieldName = "recipe"
	MetaFieldTicketID   swf.FieldName = "ticket_id"
	MetaFieldCellID     swf.FieldName = "cell_id"
	MetaFieldCellName   swf.FieldName = "cell_name"
	MetaFieldActorEmail swf.FieldName = "actor_email"
	MetaFieldGitRef     swf.FieldName = "git_ref"
)

type JobMetadata struct {
	Version    int    `json:"v"`
	RecipeName string `json:"recipe,omitempty"`
	TicketID   string `json:"ticket_id,omitempty"`
	CellID     string `json:"cell_id,omitempty"`
	CellName   string `json:"cell_name,omitempty"`
	ActorEmail string `json:"actor_email,omitempty"`
	GitRef     string `json:"git_ref,omitempty"`
}

func JobMetadataFromStartJob(startJob workflowctl.StartJob) JobMetadata {
	return JobMetadata{
		Version:    JobMetadataVersion,
		RecipeName: startJob.RecipeName,
		TicketID:   startJob.JobContext.Actor.TicketID,
		CellID:     startJob.JobContext.Workflow.CellID,
		CellName:   startJob.JobContext.Workflow.CellName,
		ActorEmail: startJob.JobContext.Actor.ActorEmail,
		GitRef:     startJob.GitRef,
	}
}

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

	meta := JobMetadataFromStartJob(startJob)
	metaRaw, err := json.Marshal(meta)
	if err != nil {
		return swf.JobKey{}, err
	}

	job := swf.StartJob{
		TenantId:     startJob.TenantId,
		JobType:      RecipeJobType,
		SingletonKey: startJob.SingletonKey,
		Data:         inputData,
		RunPolicy:    swf.DefaultRunPolicy(),
		Metadata:     metaRaw,
	}
	return engine.StartJob(ctx, job)
}
