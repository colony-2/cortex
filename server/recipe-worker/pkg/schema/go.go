package main

import (
	"fmt"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
	"github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/commandop"
	"github.com/divisive-ai/vibethis/server/recipe-worker/pkg/sleepop"
	"gopkg.in/yaml.v3"
)

func main() {
	ops.Register(commandop.GetOp(), sleepop.GetOp())
	rec := r1
	err := recipe.Validate(rec)
	if err != nil {
		fmt.Printf("failure to validate:\n\t %v", err)
		os.Exit(-1)
	}

	parsedRecipe, err := recipe.LoadRecipeFromString([]byte(rec))
	if err != nil {
		fmt.Printf("failure to parse:\n\t %v", err)
		os.Exit(-1)
	}

	out, err := yaml.Marshal(parsedRecipe)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(out))
	spew.Dump(parsedRecipe)

}

var r2 = `
id: simple-test-recipe
version: "1.0.0"
desc: A simple test recipe for integration testing
op: Command Execution
inputs:
  run: "curl -s https://httpbin.org/get"

`

var r1 = `
id: simple-test-recipe
version: "1.0.0"
desc: A simple test recipe for integration testing

# Root is a sequence of operations
sequence:
  - id: step1
    op: Command Execution
    inputs:
      run: "curl -s https://httpbin.org/get"

  - id: step2
    op: Command Execution
    inputs:
      run: "echo 'Hello from test'"
`
