package recipe

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	recipeartifacts "github.com/colony-2/colony2/server/recipe-core/pkg/artifacts"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/swf-go/pkg/swf"
	"github.com/segmentio/ksuid"
)

const childJobIDNamespace = "colony2.recipe-child/v1"

func deterministicChildJobID(parentJobID string, invocation contextual.Invocation, recipeIndex int) string {
	hasher := sha256.New()
	hasher.Write([]byte(childJobIDNamespace))
	hasher.Write([]byte{0})
	hasher.Write([]byte(parentJobID))
	hasher.Write([]byte{0})
	hasher.Write([]byte(invocation.NodePath))
	hasher.Write([]byte{0})

	var intBuf [8]byte
	binary.BigEndian.PutUint64(intBuf[:], uint64(invocation.InvokeSeq))
	hasher.Write(intBuf[:])
	binary.BigEndian.PutUint64(intBuf[:], uint64(recipeIndex))
	hasher.Write(intBuf[:])

	sum := hasher.Sum(nil)
	var jobID ksuid.KSUID
	copy(jobID[:], sum[:len(jobID)])
	return jobID.String()
}

func recipeToStart(ctx context.Context, tenantId string, ctl workflowctl.WorkflowControl, recipe SingleRecipe, gitRef string, jobID string) workflowctl.StartJob {
	artifacts := make([]swf.Artifact, 0, len(recipe.Artifacts))
	for _, artifactRef := range recipe.Artifacts {
		key, ok := artifactRef.StoredKey()
		if !ok {
			continue
		}
		artifacts = append(artifacts, ctl.GetArtifactLazy(ctx, tenantId, key))
	}

	return workflowctl.StartJob{
		TenantId:     tenantId,
		JobID:        jobID,
		RecipeName:   recipe.Name,
		Inputs:       recipe.Inputs,
		Artifacts:    artifacts,
		ArtifactRefs: append([]recipeartifacts.Ref(nil), recipe.Artifacts...),
		JobContext: contextual.JobContext{
			Workflow: contextual.WorkflowContext{
				CellName: recipe.CellName,
				CellPath: recipe.CellPath,
			},
			GitBase: contextual.GitBaseContext{
				BaseRepo:         recipe.Git.BaseRepo,
				BaseRef:          recipe.Git.BaseRef,
				ResolvedBaseHash: recipe.Git.BaseHash,
				GitAuthor:        recipe.Git.Author,
			},
		},
		GitRef: gitRef,
	}
}

// func(deps OpDependencies, ctx context.Context, in In)
// Execute runs the activity with provided configuration and inputs
func startJobs(ctx context.Context, parentJobKey swf.JobKey, invocation contextual.Invocation, ctl workflowctl.WorkflowControl, recipes []SingleRecipe, gitRef string) ([]swf.JobKey, error) {
	if len(recipes) == 0 {
		return nil, fmt.Errorf("no jobs to start")
	}

	jobs := make([]workflowctl.StartJob, len(recipes))
	for i, recipe := range recipes {
		jobs[i] = recipeToStart(ctx, parentJobKey.TenantId, ctl, recipe, gitRef, deterministicChildJobID(parentJobKey.JobId, invocation, i))
	}

	if len(jobs) == 1 {
		key, err := ctl.StartJob(ctx, jobs[0])
		if err != nil {
			return nil, err
		}
		return []swf.JobKey{key}, nil
	}

	keys := make([]swf.JobKey, len(jobs))
	for i, job := range jobs {
		key, err := ctl.StartJob(ctx, job)
		if err != nil {
			return nil, err
		}
		keys[i] = key
	}
	return keys, nil
}
