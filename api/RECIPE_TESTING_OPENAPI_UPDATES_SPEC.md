# Proposed OpenAPI Updates for Recipe Testing (Functional / Per-Case)

## Purpose

Define a functional API contract for recipe testing:

1. one test case per request
2. no persisted run entity
3. full result returned in response
4. support for outcome evaluation (pattern + LLM judge)

## Explicit constraints

1. No server-side storage of reusable test suites.
2. No run lifecycle endpoints (`status`, `report`, `artifacts list/get` by run ID).
3. CLI is responsible for orchestrating multiple cases and storing local reports.

## Endpoints

1. `POST /v1/recipe-tests/cases/validate`
2. `POST /v1/recipe-tests/cases/execute`

## Component schemas

## `RecipeTestTargetRecipe`

`oneOf`:

1. `RecipeTestTargetRecipeServerRef`
2. `RecipeTestTargetRecipeInline`

### `RecipeTestTargetRecipeServerRef`

```json
{
  "type": "object",
  "required": ["mode", "name"],
  "properties": {
    "mode": { "type": "string", "enum": ["server_ref"] },
    "name": { "type": "string", "minLength": 1 },
    "version": { "type": "string", "minLength": 1 },
    "ref": { "type": "string", "minLength": 1 }
  },
  "additionalProperties": false
}
```

Rule:

1. Exactly one of `version` or `ref` required.

### `RecipeTestTargetRecipeInline`

```json
{
  "type": "object",
  "required": ["mode", "format", "content"],
  "properties": {
    "mode": { "type": "string", "enum": ["inline_recipe"] },
    "format": { "type": "string", "enum": ["yaml", "json"] },
    "content": { "type": "string", "minLength": 1 }
  },
  "additionalProperties": false
}
```

## `RecipeTestCase`

```json
{
  "type": "object",
  "required": ["id", "type"],
  "properties": {
    "id": { "type": "string", "minLength": 1 },
    "type": { "type": "string", "enum": ["op_case", "recipe_case", "integration_case"] },
    "target": { "type": "object" },
    "inputs": { "type": "object" },
    "mocks": { "$ref": "#/components/schemas/RecipeTestMocks" },
    "assertions": {
      "type": "array",
      "items": { "$ref": "#/components/schemas/RecipeTestAssertion" }
    },
    "evaluations": {
      "type": "array",
      "items": { "$ref": "#/components/schemas/RecipeTestEvaluation" }
    },
    "options": { "type": "object" }
  },
  "additionalProperties": false
}
```

`op_case` rule:

1. `target.node_path` required.

## `RecipeTestMocks`

```json
{
  "type": "object",
  "properties": {
    "ops": {
      "type": "array",
      "items": { "$ref": "#/components/schemas/RecipeTestOpMock" }
    },
    "child_recipes": {
      "type": "array",
      "items": { "$ref": "#/components/schemas/RecipeTestChildRecipeMock" }
    },
    "functions": {
      "type": "object",
      "additionalProperties": { "type": "object" }
    },
    "context_overrides": { "type": "object" },
    "seed_artifacts": {
      "type": "array",
      "items": { "$ref": "#/components/schemas/RecipeTestSeedArtifact" }
    }
  },
  "additionalProperties": false
}
```

## `RecipeTestOpMock`

```json
{
  "type": "object",
  "required": ["match", "behavior"],
  "properties": {
    "match": {
      "type": "object",
      "properties": {
        "node_path": { "type": "string" },
        "op": { "type": "string" }
      },
      "additionalProperties": false
    },
    "behavior": { "$ref": "#/components/schemas/RecipeTestMockBehavior" }
  },
  "additionalProperties": false
}
```

## `RecipeTestChildRecipeMock`

```json
{
  "type": "object",
  "required": ["match", "behavior"],
  "properties": {
    "match": {
      "type": "object",
      "required": ["name"],
      "properties": {
        "name": { "type": "string" },
        "version": { "type": "string" },
        "ref": { "type": "string" }
      },
      "additionalProperties": false
    },
    "behavior": { "$ref": "#/components/schemas/RecipeTestMockBehavior" }
  },
  "additionalProperties": false
}
```

## `RecipeTestMockBehavior`

```json
{
  "type": "object",
  "required": ["mode"],
  "properties": {
    "mode": {
      "type": "string",
      "enum": ["return", "fail", "passthrough", "record_passthrough", "replay"]
    },
    "outputs": { "type": "object" },
    "artifacts": {
      "type": "object",
      "additionalProperties": { "type": "string" }
    },
    "error": {
      "type": "object",
      "properties": {
        "code": { "type": "string" },
        "message": { "type": "string" }
      },
      "additionalProperties": false
    },
    "cassette_key": { "type": "string" }
  },
  "additionalProperties": false
}
```

## `RecipeTestSeedArtifact`

```json
{
  "type": "object",
  "required": ["node_path", "files"],
  "properties": {
    "node_path": { "type": "string" },
    "files": {
      "type": "object",
      "additionalProperties": { "type": "string" }
    }
  },
  "additionalProperties": false
}
```

## `RecipeTestAssertion`

```json
{
  "type": "object",
  "required": ["type"],
  "properties": {
    "type": {
      "type": "string",
      "enum": [
        "output_equals",
        "output_matches",
        "artifact_exists",
        "artifact_json_equals",
        "node_executed",
        "node_not_executed",
        "status_is",
        "cel_true"
      ]
    },
    "path": { "type": "string" },
    "value": {},
    "regex": { "type": "string" },
    "json_path": { "type": "string" },
    "node_path": { "type": "string" },
    "status": { "type": "string" },
    "expr": { "type": "string" }
  },
  "additionalProperties": false
}
```

## `RecipeTestEvaluation`

```json
{
  "type": "object",
  "required": ["id", "type", "source", "mode"],
  "properties": {
    "id": { "type": "string", "minLength": 1 },
    "type": { "type": "string", "enum": ["text_pattern", "llm_judge"] },
    "mode": { "type": "string", "enum": ["enforce", "report_only"], "default": "enforce" },
    "source": { "$ref": "#/components/schemas/RecipeTestEvaluationSource" },
    "config": { "$ref": "#/components/schemas/RecipeTestEvaluationConfig" }
  },
  "additionalProperties": false
}
```

## `RecipeTestEvaluationSource`

```json
{
  "type": "object",
  "required": ["kind"],
  "properties": {
    "kind": {
      "type": "string",
      "enum": ["artifact", "artifact_glob", "output_path", "trace"]
    },
    "path": { "type": "string" },
    "glob": { "type": "string" },
    "output_path": { "type": "string" },
    "trace_name": { "type": "string" },
    "allow_sensitive": { "type": "boolean", "default": false }
  },
  "additionalProperties": false
}
```

## `RecipeTestEvaluationConfig`

`oneOf`:

1. `RecipeTestTextPatternConfig`
2. `RecipeTestLLMJudgeConfig`

### `RecipeTestTextPatternConfig`

```json
{
  "type": "object",
  "properties": {
    "forbid_regex": { "type": "array", "items": { "type": "string" } },
    "require_regex": { "type": "array", "items": { "type": "string" } },
    "max_matches": {
      "type": "object",
      "additionalProperties": { "type": "integer", "minimum": 0 }
    },
    "case_insensitive": { "type": "boolean", "default": false },
    "normalize_whitespace": { "type": "boolean", "default": true }
  },
  "additionalProperties": false
}
```

### `RecipeTestLLMJudgeConfig`

```json
{
  "type": "object",
  "required": ["provider", "model", "system_prompt", "prompt_template", "pass_when"],
  "properties": {
    "provider": { "type": "string" },
    "model": { "type": "string" },
    "system_prompt": { "type": "string" },
    "prompt_template": { "type": "string" },
    "response_schema": { "type": "object" },
    "pass_when": { "type": "string" },
    "temperature": { "type": "number", "default": 0, "minimum": 0, "maximum": 2 },
    "max_tokens": { "type": "integer", "minimum": 1 },
    "timeout": { "type": "string" }
  },
  "additionalProperties": false
}
```

## `RecipeTestEvaluationResult`

```json
{
  "type": "object",
  "required": ["id", "type", "mode", "passed"],
  "properties": {
    "id": { "type": "string" },
    "type": { "type": "string", "enum": ["text_pattern", "llm_judge"] },
    "mode": { "type": "string", "enum": ["enforce", "report_only"] },
    "passed": { "type": "boolean" },
    "score": { "type": "number" },
    "findings": { "type": "array", "items": { "type": "string" } },
    "error": {
      "type": "object",
      "properties": {
        "code": { "type": "string" },
        "message": { "type": "string" }
      },
      "additionalProperties": false
    },
    "raw_output": {}
  },
  "additionalProperties": false
}
```

## Validate case endpoint

## `POST /v1/recipe-tests/cases/validate`

Request: `RecipeTestCaseValidateRequest`

```json
{
  "type": "object",
  "required": ["target_recipe", "case"],
  "properties": {
    "target_recipe": { "$ref": "#/components/schemas/RecipeTestTargetRecipe" },
    "case": { "$ref": "#/components/schemas/RecipeTestCase" },
    "options": {
      "type": "object",
      "properties": {
        "strict": { "type": "boolean", "default": false }
      },
      "additionalProperties": false
    },
    "metadata": { "type": "object" }
  },
  "additionalProperties": false
}
```

Response `200`:

```json
{
  "valid": true,
  "case_id": "happy",
  "case_hash": "sha256:...",
  "resolved_recipe": {
    "source_mode": "server_ref",
    "name": "new-ticket",
    "version": "v10",
    "ref": "abc123"
  },
  "errors": [],
  "warnings": []
}
```

Errors:

1. `400` malformed request
2. `404` recipe reference not found
3. `422` semantic validation failure
4. `500` internal error

## Execute case endpoint

## `POST /v1/recipe-tests/cases/execute`

Request: `RecipeTestCaseExecuteRequest`

```json
{
  "type": "object",
  "required": ["target_recipe", "case"],
  "properties": {
    "target_recipe": { "$ref": "#/components/schemas/RecipeTestTargetRecipe" },
    "case": { "$ref": "#/components/schemas/RecipeTestCase" },
    "execution": {
      "type": "object",
      "properties": {
        "timeout": { "type": "string" },
        "artifact_mode": { "type": "string", "enum": ["none", "inline"], "default": "none" },
        "artifact_max_bytes": { "type": "integer", "minimum": 0, "default": 1048576 },
        "redact_secrets": { "type": "boolean", "default": true },
        "evaluation_mode": { "type": "string", "enum": ["enforce", "report_only"], "default": "enforce" },
        "judge_timeout": { "type": "string" },
        "judge_max_tokens": { "type": "integer", "minimum": 1 }
      },
      "additionalProperties": false
    },
    "metadata": { "type": "object" }
  },
  "additionalProperties": false
}
```

Response `200`:

```json
{
  "case_id": "happy",
  "status": "passed",
  "duration_ms": 8120,
  "resolved_recipe": {
    "source_mode": "inline_recipe",
    "name": "inline",
    "version": "",
    "ref": ""
  },
  "assertions": [
    {
      "index": 0,
      "type": "output_equals",
      "passed": true,
      "expected": true,
      "actual": true,
      "message": ""
    }
  ],
  "evaluations": [
    {
      "id": "no-disallowed-patterns",
      "type": "text_pattern",
      "mode": "enforce",
      "passed": true,
      "score": 1.0,
      "findings": []
    },
    {
      "id": "judge-format-compliance",
      "type": "llm_judge",
      "mode": "report_only",
      "passed": false,
      "score": 0.42,
      "findings": ["Detected disallowed language pattern X"],
      "raw_output": {}
    }
  ],
  "evaluation_summary": {
    "passed": 1,
    "failed": 1,
    "errors": 0
  },
  "outputs": {},
  "artifacts": [
    {
      "name": "report.md",
      "content_type": "text/markdown",
      "encoding": "base64",
      "content": "IyByZXBvcnQK",
      "truncated": false
    }
  ],
  "failures": []
}
```

`status` enum:

1. `passed`
2. `failed`
3. `timed_out`
4. `canceled`

Evaluation rule:

1. In `enforce` mode, any failed evaluation sets case `status=failed`.
2. In `report_only` mode, evaluation failures are returned but do not by themselves set `status=failed`.

Failure category guidance (`failures[]` entries):

1. `validation_error`
2. `runtime_error`
3. `assertion_failure`
4. `evaluation_failure`
5. `evaluator_error`
6. `policy_blocked`
7. `timeout`

Errors:

1. `400` malformed request
2. `404` recipe reference not found
3. `422` case invalid for execution
4. `500` internal error

## Non-additions (intentional)

Do not add:

1. persisted run creation/list/get endpoints
2. report/artifact retrieval by run ID
3. stored suite create/update/list APIs

All results must be returned directly by per-case calls.
