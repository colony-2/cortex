# Bug + Feature Request: `GetJobRun` Result Semantics and Output Helper

This document captures:

1. A bug report: the **SWF toy engine** can return `GetJobRunResponse.Result == nil` even when attempts exist.
2. A feature request: add a convenience helper (e.g. `GetOutput`) on `GetJobRunResponse` to standardize output/error handling.

## Context

We want to rely on `SWFEngine.GetJobRun()` to:

- detect whether a job failed (via the returned outcome), and
- fetch job outputs + artifacts without separately calling `GetJobResult()`.

In practice, this is currently awkward in the toy engine because `GetJobRunResponse.Result` may be missing, and there is no built-in helper that converts the `TaskIO` into a `TaskData`/`JobData` while enforcing consistent failure semantics.

---

## 1) Bug Report: Toy Engine `GetJobRun` Response Missing `Result`

### Summary

The SWF **toy engine** can return `GetJobRunResponse{ Result: nil }` even for completed jobs that clearly have attempts in the response.

### Expected

For completed jobs (or jobs that have job attempts):

- `GetJobRunResponse.Result` should be set to the **final job attempt**, i.e. the authoritative completion attempt.
- At minimum, if `JobAttempts` is non-empty, `Result` should not be nil.

If a job truly has no attempts, then `Result` may be nil.

### Observed

In fixture runs using the toy engine, `GetJobRunResponse.Result` may be `nil` even when:

- `JobAttempts` contains attempt entries, and/or
- `Tasks` contains attempt information.

This forces client code to add workarounds such as:

- `if Result == nil { pick max-ordinal attempt from JobAttempts }`

### Repro (High Level)

- Run a recipe that launches a child job (e.g. via `recipe.run_and_get_result`).
- Make the child job fail (e.g. `command_execution` with an invalid command).
- Call `Engine.GetJobRun(...IncludeOutputs=true...)` for the child job key.
- Observe `Result == nil` even though attempts exist.

### Impact

Any consumer switching to `GetJobRun` as the source of truth for job outcome and output must either:

- fall back to `GetJobResult` (extra engine call), or
- re-implement attempt selection logic (fragile, inconsistent).

### Suggested Fix

In toy engine `GetJobRun` implementation:

- always set `Result` to the final/latest attempt when attempts exist:
  - choose the attempt with max `Ordinal` (and/or max `Attempt` if needed).
- ensure `Result.Outcome` is present and reflects success/failure.
- if outputs are requested, populate `Result.Output` consistently.

---

## 2) Feature Request: `GetJobRunResponse.GetOutput(...)` Helper

### Motivation

Most clients want the same behavior:

- "Give me the job output payload and output artifacts"
- "If the job failed, return an error (with the failure message/kind)"

Today each consumer re-implements this:

- validate `Result` exists
- interpret outcome (FAILED / Error != nil)
- convert `TaskIO` into `TaskData` (`JobData`)
- materialize artifacts (often as lazy artifacts through the engine)
- preserve payload-kind metadata (for envelope decoding)

This leads to inconsistent behavior and duplicated code.

### Proposed API

Add a helper on `GetJobRunResponse`:

```go
// Example signature; exact placement and parameterization can vary.
func (r GetJobRunResponse) GetOutput(engine swf.SWFEngine, tenantId string) (swf.JobData, error)
```

Or a split API if you want to keep it engine-free by default:

```go
func (r GetJobRunResponse) GetOutputData() (swf.Data, error)
func (r GetJobRunResponse) GetOutputArtifacts(engine swf.SWFEngine, tenantId string) ([]swf.Artifact, error)
```

But the single `GetOutput(...)` is the most ergonomic for callers.

### Desired Semantics

- **Final attempt selection**
  - If `Result == nil`, fall back to the best attempt in `JobAttempts` (max ordinal / attempt).
  - If there are no attempts, return an error (`no result`).

- **Failure detection**
  - If `Outcome.Status == FAILED` or `Outcome.Error != nil`, return `error`.
  - Error message should include `Outcome.Error.Message` when present (and optionally kind/level).

- **Output payload**
  - Build a `TaskData` (or `JobData`) from `Result.Output.Data`.

- **Artifacts**
  - Convert `Result.Output.Artifacts` (the summaries) into lazy artifacts that can be materialized via `engine.GetArtifact`.
  - If `ArtifactInfo` does not include the full `ArtifactKey`, reconstruct deterministically using:
    - job id
    - producing ordinal (e.g. `Result.Ordinal`)
    - name + size

- **Payload kind**
  - If `Outcome.PayloadKind` is set, return an `EnvelopedTaskData` (or equivalent) so downstream envelope handling preserves payload-kind metadata.

### Payoff

Client code becomes:

```go
run, err := engine.GetJobRun(ctx, swf.GetJobRunRequest{...})
if err != nil { ... }
return run.GetOutput(engine, tenantId)
```

This standardizes behavior and removes duplicated logic around outcome detection and output reconstruction.

