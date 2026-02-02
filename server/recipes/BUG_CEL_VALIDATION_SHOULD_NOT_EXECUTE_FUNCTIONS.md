# Bug: CEL validation executes functions on synthetic data

## Summary
During `recipe validate`, CEL expressions are *executed* against fabricated outputs (built from op output structs). This causes runtime-only functions such as `json_parse` to run on placeholder data, producing false errors (e.g., `json_parse: expected string` when the synthetic output isn’t the real string). Validation should check CEL compilation/type correctness, not parse or transform fake data.

## Impact
- Recipes using `json_parse(sequence.<step>.outputs.response)` for `llm_inference2` fail validation even though they work at runtime.
- Any CEL function with runtime dependencies can misbehave on placeholders, leading to false negatives and noisy validation.

## Root Cause
- Validation currently evaluates CEL with synthetic values instead of stopping after type-checking.
- Placeholder outputs don’t always match runtime values, so function evaluation hits type/value mismatches.

## Proposed Fix
1) Change validation to *type-check only*:
   - Compile CEL ASTs and ensure types resolve against variable declarations and op output shapes.
   - Do not execute functions during validation; treat functions as opaque nodes unless they are pure type-level constructs.
2) Ensure placeholder values remain simple typed stubs (e.g., empty strings for string fields) but are never fed into function execution.
3) Add regression tests:
   - Validation should pass for recipes that use `json_parse(...)` on string-typed outputs.
   - Introduce a test that would previously fail because a function was executed on a placeholder.

## Status
- Temporary hacks (widening `json_parse`, seeding `response` with `"{}"`) have been removed.
- Awaiting redesign of validation to avoid function execution.
