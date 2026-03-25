# Start Recipe Reference-By-Name Plan

## Purpose

This plan covers the real start-path work for recipe execution centered on:

1. `recipe-core/pkg/starter/start_recipe.go`
2. `recipe-worker/pkg/compiler/job_worker.go`
3. `recipe-worker/pkg/workflow/control.go`
4. `workflow/internal/service/service.go`
5. `ticket/internal/service/service.go`
6. `recipe-child/pkg/recipe/launcher.go`
7. `c2j/internal/submitjob`
8. `c2j/internal/runjob`
9. replay/story surfaces under `workflow/internal/story`

The goal is still to stop making embedded root recipe YAML the normal production start path.

The contract decision is:

1. do not add a new `recipe_ref` field
2. reuse the existing start-job `recipe` / `name` field
3. widen that field from "server recipe name" to "root recipe source selector"

This is not only cosmetic. The real implementation work is to standardize source resolution, perform that resolution when the job actually begins executing, record the authoritative resolved source in an internal early job chapter, and make worker bootstrap/replay use that recorded result.

## Confirmation

Yes, reusing `RecipeName` still keeps the contract surface smaller.

It avoids new fields in:

1. `workflowctl.StartJob`
2. `starter.JobMetadata`
3. workflow API/OpenAPI request models
4. child-start inputs
5. workflow summary/detail models

But the real work is still substantial:

1. define one canonical recipe-source model
2. standardize git-backed selectors
3. pin mutable selectors to stable identities in an early internal chapter
4. replace artifact-only worker bootstrap with source resolution
5. carry the same behavior into replay/story and `c2j`

So the plan below is centered on those implementation steps, not just the field decision.

## Core Source Model

The existing `StartJob.RecipeName` field should become the only root recipe selector.

Steady-state rule:

1. `RecipeName` is authoritative
2. `GitRef` remains workflow execution git context and must not be overloaded for recipe source selection
3. every new job should record a root-source resolution chapter before normal recipe execution
4. root recipe resolution uses one shared source model everywhere:
   - top-level workflow starts
   - ticket autostart
   - child recipe starts
   - worker bootstrap
   - replay/story
   - `c2j submit`
   - `c2j exec`

## Root Source Resolution Chapter

Recommended behavior:

1. chapter 0 remains the SWF job start payload
2. chapter 1 becomes an internal job-level task such as `recipe.root_source.resolve`
3. that task records the authoritative root-source resolution result used by the rest of the job
4. normal recipe op/state chapters begin after that

This internal chapter should exist for all new jobs if that keeps the implementation simpler.

Why this should be the authoritative pinning mechanism:

1. submitted refs may be symbolic and mutable
2. replay needs a stable recorded resolution result
3. child jobs need the same determinism guarantee
4. the event log should show what was actually resolved, not only what was requested

## Execution-Time Resolution Requirement

Resolution must happen during recipe execution, not during recipe submission.

Why this must be explicit:

1. a job may sit queued for some time after submission
2. a mutable ref can change during that delay
3. we want the recorded resolution to reflect what the runner actually executed, at the time execution began

Required behavior:

1. submitters pass through the requested selector in `StartJob.RecipeName`
2. submitters do not pin branches/tags/published aliases at submission time
3. the runner resolves and records the selector in chapter 1 when the job starts executing
4. everything after chapter 1 uses that recorded result

Allowed submission-time checks:

1. basic syntax validation
2. shape validation for obviously invalid selectors

Not allowed as the authoritative mechanism:

1. submission-time branch/tag resolution
2. submission-time published-recipe pinning
3. any approach that would cause execution to depend on a pin computed before the job actually started running

Recommended output payload for the internal chapter:

1. submitted selector
2. source kind
3. resolved stable selector/ref
4. resolved commit hash when applicable
5. whether the submitted selector was already pinned

Example conceptual output:

```json
{
  "source_kind": "git",
  "submitted_selector": "git+https://github.com/acme/platform-recipes.git//.colony2/recipes/new-ticket.recipe.yaml@main",
  "resolved_selector": "git+https://github.com/acme/platform-recipes.git//.colony2/recipes/new-ticket.recipe.yaml@9c71eb0d4379a4aa8f4ab94e545e1f53ec94b0b4",
  "resolved_commit": "9c71eb0d4379a4aa8f4ab94e545e1f53ec94b0b4",
  "was_already_pinned": false
}
```

For artifact-backed starts, the same chapter can simply record:

1. `source_kind = "artifact"`
2. submitted selector / recipe name
3. matching artifact name
4. no resolved commit

## Root Recipe Resolution Order

The worker and replay code should resolve the root recipe in this order:

1. read job start payload and artifacts
2. run the internal root-source resolution task as the first job task
3. use the recorded resolution result to decide how to load the root recipe
4. load from matching `<RecipeName>.recipe.yaml` start artifact when the resolved source kind is `artifact`
5. otherwise load via the shared resolver using the resolved stable selector returned by chapter 1

This keeps the existing artifact path available for explicit local-file submission flows without making embedded YAML the standard production mechanism.

Important implication:

1. "point to an arbitrary local file at runtime" is not a supported selector pattern
2. if a caller wants to run a local non-git recipe file, it should upload that file as a start artifact and set `RecipeName` to the recipe id/name used for the artifact key

That is the intended replacement for direct local-file selectors.

## Supported Selector Families

`RecipeName` should support these selector families:

1. server-managed recipe references
   - `name`
   - `name@ref`
2. git-backed recipe references
   - `git+file://...`
   - `git+ssh://...`
   - `git+http://...`
   - `git+https://...`
3. explicit start-artifact recipe upload
   - not a new selector syntax
   - uses the existing `<RecipeName>.recipe.yaml` artifact convention

Direct `file:` selectors should be removed from the plan.

## Standard Git Selector Format

To support local git repos, SSH git, and HTTP(S) git with one rule set, use one canonical git selector format:

`git+<scheme>://<repo-location>//<repo-relative-recipe-path>@<git-ref>`

Supported schemes:

1. `file`
2. `ssh`
3. `http`
4. `https`

Examples:

1. `git+file:///src/server//ops/test-recipes/parent-child-basic.yaml@HEAD`
2. `git+ssh://git@github.com/acme/platform-recipes.git//.colony2/recipes/new-ticket.recipe.yaml@main`
3. `git+https://github.com/acme/platform-recipes.git//.colony2/recipes/new-ticket.recipe.yaml@refs/heads/main`

Notes:

1. the repo URL is everything before the `//<repo-relative-recipe-path>` suffix
2. the recipe path is repository-relative
3. the git ref is everything after the last `@`
4. SCP-like syntax such as `git@github.com:org/repo.git` should not be the canonical selector format
5. callers should convert to the URL-based form above

Using one URL-based form avoids separate `gitlocal:` grammar and keeps all git transports under one parser.

## Git Selector Validation Rules

For git-backed selectors:

1. split on the last `@`
2. require a non-empty git ref
3. require a `git+` scheme of `file`, `ssh`, `http`, or `https`
4. require a non-empty repository-relative recipe path after the `//` delimiter
5. reject empty path segments
6. reject absolute recipe paths
7. reject `.` and path traversal outside repository root
8. require the target to be a file, not a directory
9. normalize the recipe path before resolution

## Pinning Requirement

This is the most important real-work addition.

If a selector resolves through any mutable indirection, the job must record the stable resolved identity in chapter 1 before normal recipe execution.

This applies to:

1. git branches
2. git tags
3. any non-hash git ref
4. bare server recipe names such as "current published"
5. any other server recipe ref that resolves to a later-stable saved version

Why this is required:

1. replay must use the same root recipe content as original execution
2. child jobs must not drift if refs move between submit and execution
3. job stories should show both requested and resolved recipe source identities
4. the event log needs an auditable answer to "what did this job actually run?"

Recommended behavior:

1. callers submit the requested selector in `StartJob.RecipeName`
2. the runner executes the internal root-source resolution task at ordinal 1
3. that task resolves the requested selector to a stable identity
4. subsequent root recipe loading uses the recorded stable identity, not the original mutable selector

For git selectors, the stable identity is a commit-pinned selector.

For server recipe refs, the stable identity should be the recipe service's stable saved-version ref, such as the returned `CommitHash` / saved ordinal form.

## Shared Resolver Contract

The existing simple provider shape is no longer rich enough by itself for pinning + story metadata:

`func(projectId string, recipeRef string) (*recipe.Recipe, error)`

We should introduce a richer internal resolver result, for example:

1. parsed source kind
2. loaded `recipe.Recipe`
3. resolved stable selector string
4. source metadata needed for story/replay and the internal resolution chapter

Suggested shape:

```go
type ResolvedRecipeSource struct {
    Recipe            *recipe.Recipe
    ResolvedSelector  string
    SourceKind        string // artifact|serverRef|git
    ResolvedCommit    string
    SubmittedSelector string
}
```

The old provider function can still be kept as a thin compatibility layer where only `Recipe` is needed, but the start path itself should use the richer result.

## Standard Git Resolution Implementation

Git-backed resolution should use the existing `server/git` package rather than ad hoc `exec.Command("git", ...)` logic scattered across features.

That package already exposes the needed primitives:

1. `Clone`
2. `Fetch`
3. `Checkout`
4. `GetCurrentCommit`
5. `GetFileAtCommit`

Recommended approach for the execution-time resolution task:

1. parse the git selector into repo URL, recipe path, and git ref
2. materialize or reuse a local temporary repository workspace
3. use `server/git` to resolve the submitted ref to a concrete commit
4. return a commit-pinned selector for the internal resolution chapter output
5. read the recipe file at that commit
6. parse the recipe bytes
7. return both the parsed recipe and the resolved selector metadata

This must work uniformly for:

1. `git+file://`
2. `git+ssh://`
3. `git+http://`
4. `git+https://`

## Start Submission Changes

### Workflow service

`workflow/internal/service/service.go`

Real work here:

1. stop resolving the root recipe only for YAML packaging
2. submit the requested selector in `StartJob.RecipeName`
3. stop embedding root YAML for normal ref-based starts
4. if desired, keep only lightweight syntax validation at submission time
5. do not perform authoritative source pinning at submission time
6. treat the runner's resolution chapter as the authoritative execution-time pinning mechanism

### Ticket autostart

`ticket/internal/service/service.go`

Real work here:

1. use the same selector semantics as workflow start
2. submit the requested selector in `RecipeName`
3. stop relying on embedded root YAML for normal starts
4. do not perform authoritative source pinning at submission time
5. rely on the runner's resolution chapter for authoritative pinning

### Child recipe starts

`recipe-worker/pkg/workflow/control.go`
`recipe-child/pkg/recipe/launcher.go`

Real work here:

1. child-start requests still carry `recipe.Name`
2. workflow control submits that selector without embedding root YAML for normal ref-based starts
3. workflow control does not pin the selector before submission
4. child jobs pin the selector through their own internal resolution chapter
5. child jobs therefore inherit the same deterministic selector behavior as top-level jobs

This is critical. Fixing only top-level starts would leave nested jobs inconsistent.

## Worker Bootstrap Changes

`recipe-worker/pkg/compiler/job_worker.go`
`recipe-worker/pkg/compiler/recipe_retriever.go`

Current bootstrap is artifact-only.

Required real changes:

1. add a root recipe resolver to `RecipeJobWorkerOptions`
2. add an internal task worker for root-source resolution
3. make the recipe job worker call that task as its first real task
4. teach the worker to load from matching start artifact when the recorded source kind is `artifact`
5. otherwise resolve/load using the recorded stable selector returned by chapter 1
6. remove the assumption that job start artifacts are the only authoritative root source
7. fail clearly when neither artifact nor resolver can supply the root recipe

`recipe_retriever.go` should become either:

1. a more general root-source loader, or
2. a small artifact-only helper used by a higher-level root resolver

It should no longer be the complete root bootstrap mechanism.

## Replay And Story Changes

`workflow/internal/story/builder.go`
`workflow/internal/story/replay_recorder.go`
`workflow/internal/model/job_run_story.go`

Replay must use the same root recipe source rules as execution.

Required real changes:

1. `BuildJobRunStory(...)` needs access to the shared root resolver and the internal resolution task behavior
2. replay should replay the root-source resolution chapter at ordinal 1 and consume its cached output
3. if the chapter says `artifact`, replay should load the matching start artifact
4. otherwise replay should load using the stable selector recorded in chapter 1
5. story source metadata should distinguish:
   - `jobStartArtifact`
   - `jobStartRef`
6. story output for ref-based starts should surface both:
   - the submitted selector
   - the resolved stable selector / commit

Recommended job-story treatment:

1. extend `JobRunStoryRecipeSource` with fields such as:
   - `submitted_ref`
   - `resolved_ref`
   - `resolved_commit`
   - `resolution_task_ordinal`
2. add a synthetic non-restartable root child node kind such as `recipeSourceResolution`
3. map the internal chapter-1 task onto that node so the story timeline matches the underlying SWF history
4. for artifact-backed starts, the node still exists if we choose the always-record model, but it reports `source_kind=artifact`

Without this, replay will drift or fail for ref-based jobs.

## c2j Changes

`c2j` now needs first-class coverage in the plan because it has both submission and execution paths.

### `c2j submit`

Files:

1. `c2j/internal/submitjob/service.go`
2. `c2j/internal/submitjob/options.go`
3. `c2j/internal/cmd/job_submit.go`

Required real changes:

1. `--recipe` should accept the widened selector syntax
2. `--recipe` should submit the requested selector without embedding root YAML
3. `c2j submit` should not perform authoritative source pinning before submission
4. `--recipe-file` should remain supported, but its meaning changes:
   - load the file locally
   - parse it only enough to obtain the recipe id/name
   - upload it as `<recipe-id>.recipe.yaml`
   - set `StartJob.RecipeName` to `<recipe-id>`
   - do not invent a direct local-file selector format
5. CLI help text should explain the new git selector format, the execution-time runner-side resolution chapter, and the artifact-upload behavior for `--recipe-file`

### `c2j exec`

Files:

1. `c2j/internal/jobutil/recipes.go`
2. `c2j/internal/runjob/service.go`
3. `c2j/internal/runjob/options.go`
4. `c2j/internal/cmd/exec.go`

Required real changes:

1. `c2j` worker construction must pass the shared root resolver into both live execution and replay workers
2. `c2j` workers must register the internal root-source resolution task worker
3. child recipe starts launched from `c2j exec` must use the same runner-side pinning behavior as server-side starts
4. `BuildRecipeProvider(...)` must grow beyond local-registry-only lookup
5. `BuildRecipeProvider(...)` should support:
   - local registry / embedded recipes for bare names
   - shared git selector resolution for `git+file://`, `git+ssh://`, `git+http://`, and `git+https://`
6. `--recipes-dir` should remain a way to resolve bare names locally, not a general direct-file selector mechanism

## Starter Changes

`recipe-core/pkg/starter/start_recipe.go`

Required real changes:

1. normal starts must not require a materialized root recipe object
2. starter should continue to carry `RecipeName` and any uploaded artifacts
3. inline recipe packaging should become an explicit helper path for local submission cases such as `c2j --recipe-file`, not the default production path
4. new jobs should reserve chapter 1 for root-source resolution before normal recipe execution begins

This file should get simpler for ref-based starts and remain useful for explicit artifact-backed starts.

## Metadata And Display Consequences

`starter.JobMetadata.recipe` can stay as the single persisted recipe source field.

Recommended meaning:

1. store the submitted selector string in job metadata
2. store the recipe id/name for artifact-backed starts

Consequence:

1. workflow summary/detail surfaces will show the submitted selector, not necessarily the resolved stable selector
2. the resolved stable selector belongs in chapter 1 and in job-story source metadata

That is acceptable for the first implementation, though a later display-only field could be added if needed.

## Files Most Likely To Change

Core start path:

1. `recipe-core/pkg/starter/start_recipe.go`
2. `recipe-core/pkg/starter/start_recipe_test.go`
3. `recipe-core/pkg/workflowctl/types.go` only if helper comments or docs need updates; no new field expected

Shared source resolution:

1. a new shared recipe-source resolver package
2. `recipes/pkg/recipe/provider.go`
3. `api/pkg/serverdeps/recipes.go`
4. possibly shared git-selector parsing helpers backed by `server/git`

Worker/bootstrap:

1. `recipe-worker/pkg/compiler/job_worker.go`
2. `recipe-worker/pkg/compiler/recipe_retriever.go`
3. `recipe-worker/pkg/workflow/control.go`
4. `recipe-worker/pkg/compiler/*tests`

Workflow/ticket callers:

1. `workflow/internal/service/service.go`
2. `ticket/internal/service/service.go`
3. `recipe-child/pkg/recipe/launcher.go`
4. caller tests that assert embedded-artifact behavior

Replay/story:

1. `workflow/internal/story/builder.go`
2. `workflow/internal/story/replay_recorder.go`
3. `workflow/internal/model/job_run_story.go`

`c2j`:

1. `c2j/internal/jobutil/recipes.go`
2. `c2j/internal/submitjob/service.go`
3. `c2j/internal/submitjob/options.go`
4. `c2j/internal/runjob/service.go`
5. `c2j/internal/runjob/options.go`
6. `c2j/internal/cmd/job_submit.go`
7. `c2j/internal/cmd/exec.go`
8. `c2j` integration tests

## Migration Strategy

Recommended order:

1. define the shared selector grammar and resolver result type
2. implement git-selector parsing and stable-resolution on top of `server/git`
3. add the internal root-source resolution task + task worker
4. update worker bootstrap to use chapter-1 resolution output plus artifact-first loading
5. update top-level and child start callers to stop embedding root YAML for normal ref-based starts
6. update replay/story to use the same chapter-1 rules
7. update `c2j submit` and `c2j exec`
8. remove any remaining assumptions that the root recipe always comes from embedded YAML

## Testing Plan

Implementation should add or update tests for:

1. selector parsing:
   - bare server refs
   - `name@ref`
   - `git+file://`
   - `git+ssh://`
   - `git+http://`
   - `git+https://`
2. selector validation:
   - missing ref
   - missing recipe path
   - traversal attempts
   - unsupported git scheme
3. root-source resolution chapter:
   - chapter 1 is always emitted for new jobs if we choose the always-record model
   - symbolic git ref resolves to commit-pinned selector
   - bare server ref resolves to stable saved-version ref
   - already-pinned ref echoes the same stable identity
4. worker bootstrap:
   - matching start artifact wins when chapter 1 reports `artifact`
   - missing artifact falls back to resolved stable selector
   - missing both fails clearly
5. workflow/ticket/child start submission:
   - submitted `RecipeName` remains the requested selector
   - root recipe YAML is not embedded for normal ref-based starts
   - no authoritative source pinning happens at submission time
6. replay/story:
   - replay consumes the cached chapter-1 resolution output deterministically
   - story source kind is `jobStartArtifact` or `jobStartRef` as appropriate
   - story includes the internal root-source resolution node/metadata
7. `c2j submit`:
   - `--recipe` submits git/server selectors directly
   - no authoritative source pinning happens at submit time
   - `--recipe-file` uploads artifact and sets `RecipeName` correctly
8. `c2j exec`:
   - root bootstrap uses the chapter-1 resolution path
   - child starts use the same runner-side pinning behavior

## Risks

1. if chapter 1 is not treated as the authoritative pinning record, replay will become nondeterministic
2. git transport/auth handling for `ssh` and `http(s)` needs one consistent configuration path
3. `c2j` can diverge from server behavior if it uses a separate resolver implementation instead of the shared one
4. artifact-first bootstrap must stay explicit so local file submission remains supported without reintroducing arbitrary runtime file selectors
5. workflow list/detail surfaces may remain less human-friendly because job metadata stores submitted selectors while stories expose resolved selectors

## Non-Goals

This plan does not cover:

1. recipe testing API changes
2. recipe CRUD storage redesign
3. generic artifact system redesign
4. adding a separate `recipe_ref` field
5. adding direct runtime `file:` selectors

## Recommended Implementation Bias

1. keep `RecipeName` as the only root recipe selector field
2. standardize git-backed selectors under one `git+<scheme>://...//path@ref` format
3. record authoritative root-source resolution in chapter 1 before normal recipe execution
4. support local non-git files through explicit artifact upload, not runtime file selectors
5. make worker bootstrap and replay chapter-1-driven, with artifact-first loading when the resolution result says `artifact`
6. use one shared resolver implementation across server and `c2j`
