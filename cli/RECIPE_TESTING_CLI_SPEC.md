# Recipe Testing CLI Requirements & Spec

## Purpose

Define the CLI contract for recipe testing with a functional server API:

1. API operates one test case at a time.
2. Server persists no test-run state.
3. CLI performs suite orchestration, parallelism, and live reporting.

## Scope

This spec defines:

1. commands and flags
2. local suite compilation
3. per-case API invocation model
4. live progress reporting and local result artifacts

## Core requirements

1. CLI must support target recipe modes:
   - `server_ref` (`name` + `version` or `ref`)
   - `inline_recipe` (local recipe file content)
2. CLI must support suite source formats:
   - compact YAML
   - scenario markdown
   - canonical JSON/YAML
3. CLI must compile suite -> canonical IR locally.
4. CLI must expand suite into independent case execution requests.
5. CLI must run cases in parallel and print outcomes as each case completes.
6. CLI must produce local summary/report files because server stores no run object.
7. CLI must support case evaluations beyond assertions, including:
   - deterministic text pattern checks
   - LLM-as-judge evaluations

## Target recipe arguments

Exactly one mode:

1. server ref mode:
   - `--recipe <name>`
   - exactly one of `--version <vN>` or `--ref <ref>`
2. inline mode:
   - `--recipe-file <path>`

Validation:

1. `--recipe` and `--recipe-file` are mutually exclusive.
2. `--version`/`--ref` invalid with `--recipe-file`.
3. ambiguous target recipe args fail before network calls.

## Suite input arguments

One input source required:

1. `--file <path>`
2. `--stdin`

Format resolution:

1. use `--format` if provided
2. else infer from file extension/content

Supported formats:

1. `compact_yaml`
2. `scenario_md`
3. `canonical_yaml`
4. `canonical_json`

Canonical case model requirement:

1. case may define `assertions` and `evaluations`.
2. evaluation entries are passed to server unchanged after compile.

Evaluation examples:

```yaml
cases:
  - id: codex-trace-safety
    type: recipe_case
    evaluations:
      - id: no-disallowed-phrases
        type: text_pattern
        mode: enforce
        source:
          kind: artifact
          path: codex/internal-trace.txt
          allow_sensitive: true
        config:
          forbid_regex:
            - "(?i)api[_-]?key\\s*[:=]"
            - "(?i)password\\s*[:=]"
      - id: judge-style-compliance
        type: llm_judge
        mode: report_only
        source:
          kind: artifact_glob
          glob: implementation/*.md
        config:
          provider: openai
          model: gpt-4.1
          system_prompt: "Evaluate policy compliance."
          prompt_template: "Review the provided content and report violations."
          response_schema:
            type: object
            properties:
              verdict: { type: string }
              score: { type: number }
              findings:
                type: array
                items: { type: string }
          pass_when: "verdict == 'pass' && score >= 0.8"
          temperature: 0
```

## Commands

## `c2 recipe test compile`

Compile suite to canonical IR locally.

Required:

1. target recipe args
2. suite input source
3. `--out <path>`

Flags:

1. `--format <format>`
2. `--case <case-id>` (repeatable)
3. `--strict`

Output:

1. canonical IR file
2. compile diagnostics

## `c2 recipe test validate`

Validate suite by calling server per case.

Flags:

1. all compile flags except `--out`
2. `--parallelism <n>` (default `4`)
3. `--fail-fast` (stop scheduling after first invalid case)

Behavior:

1. compile and materialize canonical cases locally
2. call `POST /v1/recipe-tests/cases/validate` for each selected case
3. run requests in parallel with worker pool
4. print each case validation result immediately on completion
5. print final validation summary

## `c2 recipe test run`

Execute suite by calling server per case.

Flags:

1. all `validate` flags
2. `--stop-on-failure` (stop scheduling new cases after first failed case)
3. `--case-timeout <duration>`
4. `--artifact-mode none|inline`
5. `--artifact-max-bytes <n>`
6. `--out-dir <dir>` (default `.c2/test-results/<timestamp>/`)
7. `--jsonl-events <path>` optional
8. `--evaluation-mode enforce|report-only` (default `enforce`)
9. `--judge-timeout <duration>` override for llm judges
10. `--judge-max-tokens <n>` override for llm judges

Behavior:

1. compile suite and resolve cases locally
2. execute each case via `POST /v1/recipe-tests/cases/execute`
3. process responses as they complete (unordered by case declaration)
4. print per-case status lines immediately
5. write local outputs:
   - `summary.json`
   - `summary.md`
   - `cases/<case-id>/result.json`
   - `cases/<case-id>/artifacts/*` (when returned inline)
   - `cases/<case-id>/evaluations.json`

## `c2 recipe test case validate`

Validate one case ID from suite.

## `c2 recipe test case run`

Execute one case ID from suite.

## Live reporting requirements

During `run` and `validate`, CLI must stream completion events:

1. case ID
2. status
3. duration
4. short failure reason (if failed)
5. evaluation summary (`passed/failed/error` counts)

Optional machine-readable stream:

1. `--jsonl-events` writes one event per line:
   - `case_started`
   - `case_completed`
   - `case_evaluation`
   - `summary`

## API mapping

1. `validate`/`case validate` -> `POST /v1/recipe-tests/cases/validate`
2. `run`/`case run` -> `POST /v1/recipe-tests/cases/execute`

No CLI commands should depend on persisted run IDs.

## Request construction

For each case request, CLI sends:

1. `target_recipe` (`server_ref` or `inline_recipe`)
2. one resolved canonical case object
3. execution options (execute endpoint only)
4. optional metadata:
   - source file path
   - git ref
   - CLI invocation ID

Important:

1. compact YAML / markdown must be compiled locally first
2. server receives one case per request
3. evaluation definitions travel with the case payload.

## Local output requirements

CLI must store local run directory with:

1. request envelopes (optional debug mode)
2. response payloads
3. final merged summary
4. evaluation findings and judge outputs (subject to redaction)

This local directory is the durable test record since server is stateless for runs.

## Exit codes

1. `0` all selected cases valid/passed
2. `1` CLI usage error
3. `2` local compile/parse error
4. `3` network/API request error
5. `4` one or more cases invalid/failed

Evaluation exit behavior:

1. in `enforce` mode, failed evaluations make the case fail.
2. in `report-only` mode, evaluation failures are reported but do not flip exit code unless assertions/case execution fail.

## Minimum CLI acceptance tests

1. run suite with parallelism 4 and print results as each case completes.
2. stop scheduling on first case failure when `--stop-on-failure` enabled.
3. validate and run both `server_ref` and `inline_recipe` modes.
4. generate local summary/report files without any run-ID endpoint usage.
5. single-case commands behave identically to suite mode for same case.
6. forbidden text pattern evaluation can fail a case in enforce mode.
7. llm-judge evaluation results are surfaced in live output and final summaries.
