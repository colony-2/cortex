package main

import (
	"fmt"
	"os"

	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/p2"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/validate"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/commandop"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/sleepop"
)

func main() {
	p2.PrintSchema(commandop.GetOp(), sleepop.GetOp())

	err := validate.Validate(recipe, commandop.GetOp(), sleepop.GetOp())
	if err != nil {
		fmt.Printf("failure to validate:\n\t %v", err)
		os.Exit(-1)
	}
}

var recipe = `
id: simple-test-recipe
version: "1.0.0"
desc: A simple test recipe for integration testing

# Root is a sequence of operations
sequence:
  - id: step1
    op: command_execution
    inputs:
      run: "curl -s https://httpbin.org/get"
    
  - id: step2
    op: command_execution
    inputs:
      run: "echo 'Hello from test'"
`
