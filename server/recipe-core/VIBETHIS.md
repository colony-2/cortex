# VIBETHIS

## Project: `recipe-core`

**Description:**

This project, `recipe-core`, is a Go library that provides the foundational data structures and parsing logic for "recipes". A recipe is a structured definition of a recipe, composed of ops and agents, all defined in YAML files. This library is the core component for understanding and interpreting these recipe definitions.

**Key Components:**

*   **`pkg/recipe`**: This package contains the primary data structures for representing recipes and their execution state.
    *   `Recipe`: The central struct representing a fully parsed and validated recipe. It includes the recipe's name, version, description, file paths, and the parsed content of its recipe, ops, and agents. It also includes metadata like a content hash and last modified time.
    *   `Job`: Represents a single execution of a recipe, tracking its status, start/end times, inputs, outputs, and any errors.
    *   `Parser`: The high-level parser that can take a file path (to either a single-file recipe or a directory) and produce a `Recipe` struct. It handles the logic of finding and parsing the manifest (`recipe.yaml`) and the associated workflow, activities, and agent files.
    *   `HashComputer`: Computes a canonical hash of a recipe's content, useful for versioning and change detection.

*   **`pkg/yaml`**: This package provides the low-level parsing capabilities and defines the Go structs that map directly to the YAML file schemas.
    *   `RecipeDefinition`: Defines the structure of a recipe, including its inputs, outputs, steps, and retry policies.
    *   `OpDefinition`: Defines a single op, including its inputs, outputs, timeout, retry policy, and implementation details (e.g., HTTP request, script).
    *   `AgentDefinition`: Defines an agent or role, including its capabilities, goals, and constraints.
    *   `Parser`: A low-level parser responsible for unmarshalling the YAML content of individual recipe files into the corresponding Go structs.

**Functionality:**

The primary function of this library is to:

1.  **Define the Recipe Schema:** Through the structs in `pkg/yaml`, it establishes the canonical structure for `recipe.yaml`, `recipe.yaml`, `ops.yaml`, and `agents.yaml` files.
2.  **Parse Recipes:** It can parse recipes from two primary formats:
    *   **Multi-file:** A directory containing a `recipe.yaml` manifest that points to other files for the recipe, ops, and agents.
    *   **Single-file:** A single YAML file that contains all the definitions for the recipe.
3.  **Represent Recipes in Go:** It provides the `recipe.Recipe` struct as a convenient, in-memory representation of a recipe, abstracting away the file-based details.
4.  **Provide Core Data Types:** It defines the core data types (`Job`, `WorkerStatus`, `JobStatus`, etc.) that are used by other components of the system to manage and track the execution of recipes.

**How to Use:**

To use this library, you would typically create an instance of the `recipe.Parser` and call its `ParseRecipe()` method with the path to a recipe file or directory. The result is a `*recipe.Recipe` object that can then be used to instantiate a workflow, schedule a job, or inspect the recipe's details.

**Example Usage:**

```go
package main

import (
    "fmt"
    "log"

    "go.uber.org/zap"
    "github.com/vibethis/server/recipe-core/pkg/recipe"
)

func main() {
    logger, _ := zap.NewProduction()
    defer logger.Sync()

    parser := recipe.NewParser(logger)
    recipe, err := parser.ParseRecipe("/path/to/your/recipe")
    if err != nil {
        log.Fatalf("Failed to parse recipe: %v", err)
    }

    fmt.Printf("Successfully parsed recipe: %s (v%s)\n", recipe.Name, recipe.Version)
    fmt.Printf("Description: %s\n", recipe.Description)
    fmt.Printf("Recipe has %d steps\n", len(recipe.Recipe.Recipe.Steps))
    fmt.Printf("Found %d ops\n", len(recipe.Ops))
}
```
