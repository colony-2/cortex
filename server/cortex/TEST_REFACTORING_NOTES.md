Test Refactoring Notes for server/cortex

Overview
- We updated cortex tests to align with the recipe-core and recipe-worker refactors described in server/recipe-worker/test-refactoring-spec.md.
- Main goals: stop simulating execution and run real recipes via recipe-worker; align with new interface-based Recipe/Node shapes and state metadata.

What Changed
- Replaced simulated execution with real execution using recipe-worker StandaloneExecutor and ActivityRegistry:
  - server/cortex/internal/shared/scope_test.go: builds a RecipeSequence (two command_execution steps) and asserts outputs from actual execution.
  - server/cortex/internal/shared/state_test.go: builds RecipeState and executes via worker (e.g., start → finish with output templates) to assert resolved outputs.
- Removed redundant and legacy tests:
  - server/cortex/cmd/cortex/workflow_template_test.go removed (template behavior is comprehensively tested in recipe-worker).
- Cleanups:
  - Eliminated temporary schema fallbacks; schema now uses recipe.GenerateSchemaString directly.
  - Removed ad-hoc op sanity test once the core bug was fixed (see below).

Key Learnings / Alignment With Spec
- Follow the new interface-based models:
  - Recipe is one of: RecipeOp, RecipeSequence, or RecipeState (via RecipeImpl).
  - Node is one of: NodeOp, NodeSequence, NodeState, or NodeShared.
  - State embeds SingleStateMetadata (Transitions moved off the node).
- Prefer real execution via recipe-worker over bespoke simulations. This validates:
  - Activity registration + input schema checks
  - Template/CEL resolution (inputs, sequence, states)
  - Sequence/state output mapping and final outputs
- Reference: server/recipe-worker/test-refactoring-spec.md

Bug Discovered (Fixed in recipe-core)
- Inline ops reported the wrong input type (GetInputType used the wrong parameter index), causing schema generation to reflect on non-struct types and panic.
- Fix: For inline ops, GetInputType should return the 4th param (index 3) of the inline handler (In), not the duration.
- After the fix in recipe-core, recipe.GenerateSchemaString is stable again, so we removed the prior guard/fallback.

Current Status
- Cortex scope/state tests now execute real recipes through the worker and pass locally.
- Schema/validate paths depend on the stable recipe-core generator and are green post fix.

Notes for Future
- If you need to add more execution-based tests, prefer using StandaloneExecutor with minimal recipes that assert key behaviors (transitions on CEL conditions, sequence outputs, etc.).
- Keep cortex focused on integration/UX-facing responsibilities; deep template/state mechanics are already covered in server/recipe-worker tests.

