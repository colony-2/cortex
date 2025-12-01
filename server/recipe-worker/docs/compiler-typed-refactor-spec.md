# Step 3: Typed Refactor for pkg/compiler

## Goals and Boundaries
- Prerequisite order: completes after gitstate and ops refactors so compiler can consume typed git/context/output structs.
- Remove `map[string]interface{}` usage from workflow execution paths; use typed containers for inputs, context, step outputs, and git data.
- Perform CEL/template resolution using CEL dynamic types over typed structs; no templating maps or field-level accessor shims.
- Build next-op inputs by resolving CEL in JSON and decoding directly into each op’s `GetInputType()` in coordination with the activity registry.
- No support for legacy map payloads.

## Target Design
- **Workflow state structs:** Define typed holders (e.g., `WorkflowInputs`, `WorkflowContext`, `StepOutputs`, `NodeResult`) that aggregate op outputs and git context structs.
- **CEL resolution:** Expose top-level namespaces (`context`, `inputs`, `steps`, `sequence`, etc.) to CEL via dynamic types rooted in the typed structs. Use a resolver that walks the JSON representation of the target input struct, replaces CEL expressions, then decodes into the op’s input type.
- **Op execution path:** `executeOp` receives typed workflow inputs, resolves templates into the target input struct, and passes the decoded struct to the registry (which performs the final decode/marshal boundary).
- **Git propagation:** Use typed gitstate helpers to propagate context/persist data; no manual map patching.

## Implementation Steps
1) **State modeling**
   - Introduce typed workflow state structs to replace map-based `WorkflowState`, `StepResult`, and git propagation caches.
2) **CEL/template resolver**
   - Implement a resolver that:
     - Serializes the target input struct shape to JSON,
     - Walks the JSON tree replacing CEL expressions using CEL dyn types over the current typed workflow state,
     - Decodes the resolved JSON into the op’s `GetInputType()`.
   - Ensure access to top-level namespaces without creating intermediate maps.
3) **Execution rewrites**
   - Update `executeOp`, `executeSequence`, and state-machine helpers to traffic typed structs end-to-end (inputs, outputs, context).
   - Remove map merges and cloning; rely on typed aggregation of node outputs.
4) **Git propagation**
   - Replace `propagateGitOutputs` map logic with typed gitstate output structs and merge helpers; maintain invocation tracking with typed data.
5) **Tests**
   - Update compiler tests to build typed inputs/outputs and validate CEL resolution on typed structs.
   - Add cases for: invalid CEL, decode failures into target input types, git propagation correctness, and sequence/state-machine aggregation without maps.

## Notes
- Compiler never emits or mutates generic maps; any external interfaces that require JSON are fed via encoded typed structs.
- Collaboration point: the template resolver produces JSON ready for the registry to decode into the op’s input type, keeping the registry as the single marshal/unmarshal boundary.
