# Recipe Core

## Overview

Recipe Core is a Go library for defining, parsing, validating, and transforming “recipes” that describe executable workflows in YAML. The model is intentionally small and composable:

- A recipe is one of: a single op, a sequence, or a state machine.
- Nodes support shared references, conditional execution via CEL, and simple metadata.
- Ops are pluggable via a registry, including input/output typing and whether they run as Temporal activities or inline.

Module path: `github.com/divisive-ai/vibethis/server/recipe-core`.

## Layout

- `pkg/recipe`: Core types, YAML parsing, JSON Schema, validation, visitors, hashing.
- `pkg/ops`: Op interface + registry, metadata, management service hooks.
- `pkg/ops/service_deps2.go`: Backward-compatible `ServiceDependencies2` extension and wrapper.
- `pkg/cel`: Small wrapper and helpers around `cel-go` for conditional expressions.
- `pkg/workflowctl`: Minimal, SDK-agnostic workflow control interface (Describe/Signal/Cancel) and dependency helper.

## Core Types

- `recipe.Recipe`: Root wrapper around one of `RecipeOp`, `RecipeSequence`, `RecipeState`.
- `recipe.Node`: Inner nodes used by containers. One of `NodeOp`, `NodeSequence`, `NodeState`, `NodeShared`.
- `recipe.NodeMetadata`: Common fields on every node
  - `id`, `desc`, `timeout`, `retry`, `inputs`, `when` (CEL expression).
- `recipe.RecipeMetadata`: Top-level recipe fields
  - `version`, inline `NodeMetadata`, `defs` (shared node map), `input_schema` (optional input contract).
- `recipe.StateMap`/`recipe.State`: State machine data with `transitions[]` each with `to` and optional `when` (CEL).
- `recipe.Job`, `recipe.ActivityExecution`, `recipe.WorkerStatus`, `recipe.JobStatus`: Execution tracking structs used by runtimes.

## YAML Shape

Top-level is metadata + exactly one root node kind (op | sequence | state):

```yaml
id: example
version: 1.0.0
desc: Demo

# Optional shared node defs (referenced via `shared: <name>`)
defs:
  say_shared:
    op: echo
    inputs:
      message: "from shared"

# Optional input schema for external inputs
input_schema:
  message:
    type: string
    description: Greeting to use
    required: true

# Root node (choose one)
sequence:
  - id: greet
    op: echo
    inputs:
      message: "hello"
  - shared: say_shared
```

State machine example:

```yaml
id: approval
version: 1.0.0
state:
  initial: start
  states:
    start:
      op: evaluate
      transitions:
        - to: approved
          when: "inputs.score >= 80"
        - to: rejected
          when: "inputs.score < 80"
    approved: {}
    rejected: {}
```

Notes:
- Sequence runs nodes in order. Parallelism is not modeled in core.
- `when` uses CEL; empty or `true` means no gating.
- `shared` nodes resolve via `defs` using the visitor described below.

## Parsing and Validation

- YAML → Go: `recipe.LoadRecipeFromString([]byte)` or `LoadRecipeFromReader(io.Reader)`.
- Input type checking: on unmarshal, ops are looked up in `pkg/ops` and `inputs` are decoded into the op’s concrete input struct; type errors surface with line/column.
- JSON Schema: `recipe.GenerateSchemaString()` reflects the model plus all registered ops’ input shapes into a single schema.
- Validate: `recipe.Validate(yamlText)` compiles the generated schema and validates the provided YAML.

## Visitors and Shared Nodes

- `recipe.NodeVisitor` and `recipe.NodeWalker` implement a transform-friendly traversal over the recipe tree.
- `recipe.SharedNodeResolver` resolves `NodeShared` references using the top-level `defs` map, with circular reference detection.
- Visitors can control traversal of children via `ShouldTraverseSequenceChildren` and `ShouldTraverseStateChildren`.

## Ops Registry (pkg/ops)

- `ops.RegisterableOp`: contract for pluggable operations
  - `Execute(ctx)`, `ExecuteInline(workflowCtx)`, `GetInputStruct()`, `GetInputType()`, `GetOutputType()`, `ExecuteAsActivity()`.
  - `GetMetadata()` returns `OpMetadata{Name, Type, Description, Version, DefaultTimeout}`.
- Constructors:
  - `ops.NewActivityMappedOp[In,Out](metadata, func(context.Context, In) (Out, error))`.
  - `ops.NewInlineOp[In,Out](metadata, func(workflow.Context, In) (Out, error))`.
- Registry API: `ops.Register(op...)`, `ops.Get(name)`, `ops.List()`, `ops.Clear()`, `ops.Size()`.
- Optional `ManagementService` for HTTP routes used by surrounding systems.
- Backward-compatible dependencies extension:
  - `ops.ServiceDependencies2` extends `ServiceDependencies` with `WorkflowControl() (workflowctl.WorkflowControl, bool)`.
  - `ops.WithWorkflowControl(base, ctl)` produces a wrapper that implements both interfaces so callers can pass it where `ServiceDependencies` is expected and callees can type-assert to `ServiceDependencies2`.

Example op registration:

```go
package myops

import (
    "context"
    "github.com/divisive-ai/vibethis/server/recipe-core/pkg/ops"
)

type EchoIn struct { Message string `json:"message"` }
type EchoOut struct { Echoed string `json:"echoed"` }

var Echo = ops.NewActivityMappedOp[EchoIn, EchoOut](
    ops.OpMetadata{Type: "echo", Name: "echo", Description: "Echo a message", Version: "1.0.0"},
    func(ctx context.Context, in EchoIn) (EchoOut, error) { return EchoOut{Echoed: in.Message}, nil },
)

func init() { ops.Register(Echo) }
```

## Workflow Control (pkg/workflowctl)

- `workflowctl.WorkflowControl`: Normalized control-plane API for runtimes (Describe, Signal, Cancel).
- `workflowctl.ExecutionRef`, `workflowctl.WorkflowStatus`, `workflowctl.WorkflowSummary`: Portable types with no SDK coupling.
- `workflowctl.DependencyName`: Well-known key for dependency containers.
- `workflowctl.From(deps)`: Helper to retrieve a `WorkflowControl` from `ops.ServiceDependencies`-style containers.

Migration plan for typed workflow control
- Phase 1: Introduce `ServiceDependencies2` and wrappers (done here); do not change Initialize signatures.
- Phase 2: Callers start passing a value implementing `ServiceDependencies2` (e.g., using `ops.WithWorkflowControl`).
- Phase 3: Callees type-assert to `ServiceDependencies2` when they want the typed accessor.
- Phase 4: Optionally update Initialize signatures to accept `ServiceDependencies2` once adoption is complete.

## CEL Integration (pkg/cel)

- `cel.CELExpr` wraps compiled CEL programs and marshals as a YAML string.
- `NodeMetadata.when` and `Transition.when` use `CELExpr`.
- Access pattern for conditions is flexible; a `DynamicMapValue` adapter enables dot access into maps.

## Hashing

- `recipe.HashComputer` computes a deterministic SHA-256 over the normalized recipe structure (fallback to id+version on error).

## Minimal Usage Examples

Parse and validate:

```go
import (
  "fmt"
  "github.com/divisive-ai/vibethis/server/recipe-core/pkg/recipe"
)

data := `id: demo\nversion: 1.0.0\nop: echo\ninputs: { message: "hi" }\n`
r, err := recipe.LoadRecipeFromString([]byte(data))
if err != nil { panic(err) }

if err := recipe.Validate(data); err != nil {
  panic(err)
}

fmt.Println(r.GetMetdata().ID, r.GetMetdata().Version)
```

Resolve shared nodes via visitor:

```go
defs := map[string]recipe.Node{
  "say": { NodeImpl: &recipe.NodeOp{ OpData: recipe.OpData{ Op: "echo" } } },
}
resolver := recipe.NewSharedNodeResolver(defs)
walker := recipe.NewNodeWalker(resolver)
out, err := walker.Walk(*r)
_ = out; _ = err
```

## Notes and Constraints

- Parallelism is out of scope for core; compose sequences/states to model flows.
- The core focuses on structure, typing, schema and transformation; execution is provided by external runtimes that consume `ops.RegisterableOp` and the recipe tree.
