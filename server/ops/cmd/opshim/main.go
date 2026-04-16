package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	ghaops "github.com/colony-2/colony2/server/gha/pkg/gha"
	codexops "github.com/colony-2/colony2/server/llm/pkg/codex"
	llmops "github.com/colony-2/colony2/server/llm/pkg/llm"
	coreops "github.com/colony-2/colony2/server/recipe-core/pkg/ops"
	"github.com/colony-2/swf-go/pkg/swf"
)

func main() {
	if len(os.Args) != 2 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: opshim <op-name>")
		os.Exit(2)
	}

	input := map[string]interface{}{}
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil && err.Error() != "EOF" {
		_, _ = fmt.Fprintf(os.Stderr, "decode input: %v\n", err)
		os.Exit(1)
	}

	op, err := resolveOp(os.Args[1])
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	deps := coreops.NewOpDependenciesBuilder().
		WithWorktreePath(strings.TrimSpace(os.Getenv("VIBETHIS_WORKTREE_PATH"))).
		WithGitContext(loadGitContext()).
		Build()

	output, err := op.TaskChain()[0].Invoke(deps, context.Background(), input)
	writeCapturedArtifacts(deps.GetOutputArtifacts(), outboxPath(input))

	envelope := map[string]interface{}{
		"output": output,
	}
	if refs := deps.GetExternalArtifacts(); len(refs) > 0 {
		envelope["artifact_refs"] = refs
	}
	if err == nil {
		if encodeErr := json.NewEncoder(os.Stdout).Encode(envelope); encodeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "encode output: %v\n", encodeErr)
			os.Exit(1)
		}
		return
	}
	if len(output) > 0 || len(deps.GetExternalArtifacts()) > 0 {
		_ = json.NewEncoder(os.Stdout).Encode(envelope)
	}
	_, _ = fmt.Fprintln(os.Stderr, err.Error())
	os.Exit(1)
}

func resolveOp(name string) (coreops.RegisterableOp, error) {
	switch strings.TrimSpace(name) {
	case "codex.exec":
		return codexops.GetOp(), nil
	case "llm_inference":
		return llmops.GetOp(), nil
	case "llm_inference2":
		return llmops.GetEnhancedOp(), nil
	case "gha.run":
		return ghaops.GetOp(), nil
	case "gha.runs":
		return ghaops.GetRunsOp(), nil
	default:
		return nil, fmt.Errorf("unsupported shim op %q", name)
	}
}

func loadGitContext() coreops.GitExecutionContext {
	ctx := coreops.GitExecutionContext{
		BaseRepo:         strings.TrimSpace(os.Getenv("VIBETHIS_GIT_BASE_REPO")),
		BaseRef:          strings.TrimSpace(os.Getenv("VIBETHIS_GIT_BASE_REF")),
		ResolvedBaseHash: strings.TrimSpace(os.Getenv("VIBETHIS_GIT_RESOLVED_BASE_HASH")),
		RecipeSourceRepo: strings.TrimSpace(os.Getenv("VIBETHIS_GIT_RECIPE_SOURCE_REPO")),
		RecipeSourceRef:  strings.TrimSpace(os.Getenv("VIBETHIS_GIT_RECIPE_SOURCE_REF")),
		PersistHash:      strings.TrimSpace(os.Getenv("VIBETHIS_GIT_PERSIST_HASH")),
		ParentHash:       strings.TrimSpace(os.Getenv("VIBETHIS_GIT_PARENT_HASH")),
		CellName:         strings.TrimSpace(os.Getenv("VIBETHIS_GIT_CELL_NAME")),
		CellPath:         strings.TrimSpace(os.Getenv("VIBETHIS_GIT_CELL_PATH")),
		NodePath:         strings.TrimSpace(os.Getenv("VIBETHIS_GIT_NODE_PATH")),
		InvokeHash:       strings.TrimSpace(os.Getenv("VIBETHIS_GIT_INVOKE_HASH")),
		WorktreePath:     strings.TrimSpace(os.Getenv("VIBETHIS_WORKTREE_PATH")),
	}
	if invokeSeq := strings.TrimSpace(os.Getenv("VIBETHIS_GIT_INVOKE_SEQ")); invokeSeq != "" {
		if parsed, err := strconv.ParseInt(invokeSeq, 10, 64); err == nil {
			ctx.InvokeSeq = parsed
		}
	}
	return ctx
}

func outboxPath(input map[string]interface{}) string {
	if outbox, ok := input["artifact_outbox_path"].(string); ok && strings.TrimSpace(outbox) != "" {
		return strings.TrimSpace(outbox)
	}
	return ""
}

func writeCapturedArtifacts(artifacts []swf.Artifact, outbox string) {
	if len(artifacts) == 0 || strings.TrimSpace(outbox) == "" {
		return
	}
	for _, artifact := range artifacts {
		dest := filepath.Join(outbox, artifact.Name())
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			continue
		}
		_ = artifact.SaveToFile(context.Background(), dest)
	}
}
