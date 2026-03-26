package submitjob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/colony-2/colony2/server/api/pkg/serverdeps/opregistry"
	"github.com/colony-2/colony2/server/recipe-core/pkg/contextual"
	"github.com/colony-2/colony2/server/recipe-core/pkg/recipe"
	"github.com/colony-2/colony2/server/recipe-core/pkg/starter"
	"github.com/colony-2/colony2/server/recipe-core/pkg/workflowctl"
	"github.com/colony-2/colony2/server/recipe-worker/pkg/compiler"
	"github.com/colony-2/swf-go/pkg/swf"
	remoteruntime "github.com/colony-2/swf-go/pkg/swf/runtime/remote"
	"gopkg.in/yaml.v3"
)

type submitResult struct {
	TenantID string `json:"tenant_id"`
	JobID    string `json:"job_id"`
	Recipe   string `json:"recipe"`
}

func Run(ctx context.Context, opts Options) error {
	opts.Complete()
	if err := opts.Validate(); err != nil {
		return err
	}

	opregistry.Register(opregistry.ProfileC2J)

	recipeName, embeddedRecipe, cleanup, err := loadRecipeStart(opts)
	if err != nil {
		return err
	}
	defer cleanup()

	inputs, err := loadInputs(opts)
	if err != nil {
		return err
	}

	runtime, err := remoteruntime.New(opts.SWFURL, &http.Client{Timeout: 30 * time.Second})
	if err != nil {
		return fmt.Errorf("create remote runtime: %w", err)
	}

	engine, err := swf.NewEngineBuilder().WithRuntime(runtime).BuildEngine()
	if err != nil {
		return fmt.Errorf("build engine: %w", err)
	}

	submittedAt := time.Now().UTC()
	start := workflowctl.StartJob{
		TenantId:   opts.TenantID,
		RecipeName: recipeName,
		Inputs:     inputs,
		JobContext: contextual.JobContext{
			Actor: contextual.ActorContext{
				TicketID:   strings.TrimSpace(opts.TicketID),
				ActorEmail: strings.TrimSpace(opts.ActorEmail),
			},
			Workflow: contextual.WorkflowContext{
				CellName:  strings.TrimSpace(opts.CellName),
				CellPath:  strings.TrimSpace(opts.CellPath),
				ProjectId: opts.TenantID,
			},
			GitBase: contextual.GitBaseContext{
				BaseRepo: strings.TrimSpace(opts.RepoPath),
				BaseRef:  strings.TrimSpace(opts.GitRef),
			},
		},
		GitRef:      strings.TrimSpace(opts.GitRef),
		SubmittedAt: &submittedAt,
		InputHash:   hashInputs(inputs),
	}

	var jobKey swf.JobKey
	if embeddedRecipe != nil {
		jobKey, err = starter.StartRecipeJob(ctx, start, engine, *embeddedRecipe)
	} else {
		jobKey, err = starter.StartRecipeJob(ctx, start, engine)
	}
	if err != nil {
		return fmt.Errorf("submit job: %w", err)
	}

	result := submitResult{
		TenantID: opts.TenantID,
		JobID:    jobKey.JobId,
		Recipe:   recipeName,
	}
	if opts.JSONOutput {
		return json.NewEncoder(opts.Stdout).Encode(result)
	}
	_, err = fmt.Fprintf(opts.Stdout, "submitted job tenant=%s job_id=%s recipe=%s\n", result.TenantID, result.JobID, result.Recipe)
	return err
}

func loadRecipeStart(opts Options) (string, *recipe.Recipe, func(), error) {
	if recipeFile := strings.TrimSpace(opts.RecipeFile); recipeFile != "" {
		absPath, err := filepath.Abs(recipeFile)
		if err != nil {
			return "", nil, nil, fmt.Errorf("resolve recipe file: %w", err)
		}
		f, err := os.Open(absPath)
		if err != nil {
			return "", nil, nil, fmt.Errorf("open recipe file: %w", err)
		}
		defer func() {
			_ = f.Close()
		}()
		rec, err := recipe.LoadRecipeFromReader(f)
		if err != nil {
			return "", nil, nil, fmt.Errorf("load recipe file: %w", err)
		}
		return rec.GetMetdata().ID, rec, func() {}, nil
	}

	selector := strings.TrimSpace(opts.Recipe)
	if err := compiler.ValidateRecipeSelector(selector); err != nil {
		return "", nil, nil, err
	}
	return selector, nil, func() {}, nil
}

func loadInputs(opts Options) (map[string]interface{}, error) {
	if strings.TrimSpace(opts.InputsJSON) == "" && strings.TrimSpace(opts.InputsFile) == "" {
		return map[string]interface{}{}, nil
	}

	var raw []byte
	switch {
	case strings.TrimSpace(opts.InputsJSON) != "":
		raw = []byte(opts.InputsJSON)
	default:
		absPath, err := filepath.Abs(opts.InputsFile)
		if err != nil {
			return nil, fmt.Errorf("resolve inputs file: %w", err)
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("read inputs file: %w", err)
		}
		raw = data
	}

	inputs := map[string]interface{}{}
	if err := json.Unmarshal(raw, &inputs); err == nil {
		return inputs, nil
	}
	if err := yaml.Unmarshal(raw, &inputs); err == nil {
		return inputs, nil
	}
	return nil, fmt.Errorf("decode inputs: expected a JSON or YAML object")
}

func hashInputs(inputs map[string]interface{}) string {
	if len(inputs) == 0 {
		return ""
	}
	raw, err := json.Marshal(inputs)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
