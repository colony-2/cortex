package input_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    recipepkg "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
    recipeops "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
    workerops "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
    executor "github.com/divisive-ai/vibethis/server/recipe-worker/pkg/executor"
    exportops "github.com/divisive-ai/vibethis/server/ops/pkg/export"
    "go.uber.org/zap"
)

// TestFullInputActivityLifecycle tests the input op using the StandaloneExecutor
func TestFullInputActivityLifecycle(t *testing.T) {
    // Register ops into recipe-core ops registry
    recipeops.Clear()
    recipeops.Register(exportops.GetAll()...)

    // Build a simple recipe that invokes the input op directly at root
    r := recipepkg.Recipe{
        RecipeImpl: &recipepkg.RecipeOp{
            RecipeMetadata: recipepkg.RecipeMetadata{
                Version: "1.0.0",
                NodeMetadata: recipepkg.NodeMetadata{ID: "root"},
            },
            OpData: recipepkg.OpData{Op: "input"},
        },
    }

    // Create the registry and standalone executor
    reg, err := workerops.NewActivityRegistry()
    require.NoError(t, err)
    exec, err := executor.NewStandaloneExecutor(reg, zap.NewNop())
    require.NoError(t, err)

    // Execute with inputs mapping to the input op struct fields
    ctx := context.Background()
    outputs, err := exec.Execute(ctx, r, map[string]interface{}{
        "box_id":      "test-cell",
        "activity_id": "approval-activity",
        "config": map[string]interface{}{
            "question": "Do you approve this deployment?",
            "type":     "multiple_choice",
            "options": []map[string]interface{}{
                {"value": "approve"},
                {"value": "reject"},
            },
            "timeout": 60,
        },
    })
    require.NoError(t, err)

    // Verify the result from the input op mock
    assert.Equal(t, "test-user", outputs["user_id"])
    // Response should be the first option (approve) per mock
    assert.Equal(t, "approve", outputs["response"])
}
