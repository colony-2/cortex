package main

import (
	"context"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/executor"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/ops"
	"go.uber.org/zap/zaptest"
	"gopkg.in/yaml.v3"
	"testing"
)

func TestDebugExecutor(t *testing.T) {
	yamlStr := `id: hello_world
desc: A minimal recipe that demonstrates basic structure  
version: "1.0"
op: echo_activity
inputs:
  message: "Hello, World!"`

	var r recipe.Recipe
	err := yaml.Unmarshal([]byte(yamlStr), &r)
	if err != nil {
		t.Fatalf("Parse Error: %v", err)
	}

	t.Logf("Recipe ID: %s", r.GetMetdata().ID)

	// Create executor
	logger := zaptest.NewLogger(t)
	a, err := ops.NewActivityRegistry()
	if err != nil {
		t.Fatalf("Registry Error: %v", err)
	}

	exec, err := executor.NewStandaloneExecutor(a, logger)
	if err != nil {
		t.Fatalf("Executor Error: %v", err)
	}

	// Execute recipe
	result, err := exec.Execute(
		context.Background(),
		r,
		map[string]interface{}{
			"name":        "Test",
			"basegitrepo": "/tmp/vibethis/test-repo",
			"basegithash": "0000000000000000000000000000000000000000",
			"ticketid":    "TEST-TICKET",
			"cellname":    "test-cell",
		},
		executor.ExecutionOptions{
			SuppressLogs: false, // Show logs for debugging
		},
	)

	if err != nil {
		t.Fatalf("Execution Error: %v", err)
	}

	t.Logf("Result: %v", result)
}
