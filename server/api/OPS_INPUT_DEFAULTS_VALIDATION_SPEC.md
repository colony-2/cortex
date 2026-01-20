# Ops Input Defaults + Validation Spec

## Scope

This document enumerates production (non-test) ops defined via:
- `ops.NewActivityMappedOpV2`
- `ops.NewActivityMappedOpWithProviderV2`
- `ops.NewOp()`

It also proposes:
- contextual default expansions using `context.*` (from `contextual.JobContext`)
- go-playground validator tags for required fields and tighter constraints
- where to run validation before publishing recipes in `server/recipes`

## Production Ops Inventory

The following ops are defined outside of test code:

- `input` (`Input`) in `server/recipe-input/pkg/input/activity.go`
- `recipe_set` (`Input`) in `server/.recipe-child/pkg/recipeset/op.go`
- `ticket.manage` (`Input`) in `server/ticket/pkg/op/op.go`
- `codex.exec` (`ExecOpInput`) in `server/ops/pkg/codex/op.go`
- `llm_inference` (`LLMInput`) in `server/ops/pkg/llm/wrapper.go`
- `llm_inference2` (`LLMInferenceInput`) in `server/ops/pkg/llm/llm_inference_enhanced.go`
- extension ops (`extInputsWrapper`) in `server/ops/pkg/extensions/extension_ops.go`
- `command_execution` (`CommandExecutionInput`) in `server/recipe-worker/pkg/commandop/command_execution.go`
- `sleep` (`SleepInput`) in `server/recipe-worker/pkg/sleepop/op.go`
- `git_file_collector` (`GitFileCollectorInput`) in `server/git/pkg/gitcollector/git_file_collector.go`
- `git_shallow_clone` (`GitShallowInput`) in `server/git/pkg/gitshallow/wrapper.go`
- `git_persist_commit` (`PersistCommitInput`) in `server/git/pkg/gitcommit/wrapper.go`
- `git_restore_commit` (`RestoreCommitInput`) in `server/git/pkg/gitcommit/wrapper.go`
- `thinpackrebase` (`ThinpackRebaseInput`) in `server/git/pkg/thinpackrebase/wrapper.go`
- `squashrebasemerge` (`SquashRebaseMergeInput`) in `server/git/pkg/squashrebasemerge/wrapper.go`

## Contextual Defaults and Validation Tags

Notes:
- `context.*` refers to `contextual.JobContext` (and its children) in template resolution.
- Tags are written as if placed on the Go struct field: `validate:"..."`
- When a constraint is not supported by stock validator tags, it is called out explicitly.

### input (Input)

Defaults:
- None from `JobContext` today.

Required + constraints:
- `Input.Form` -> `validate:"required"`
- `Config.Question` -> `validate:"required_without=Fields"`
- `Config.Fields` -> `validate:"required_without=Question,min=1,dive"`
- `Config.Type` -> `validate:"required_with=Question,oneof=short_answer paragraph_text multiple_choice checkboxes dropdown linear_scale date time"`
- `Config.Options` -> `validate:"required_if=Type multiple_choice required_if=Type checkboxes required_if=Type dropdown,min=1,dive"`
- `Config.Scale` -> `validate:"required_if=Type linear_scale"`
- `Config.Timeout` -> `validate:"omitempty,gte=1,lte=3600"`
- `Option.Value` -> `validate:"required"`
- `LinearScale.Min` -> `validate:"required,gte=0,lte=10"`
- `LinearScale.Max` -> `validate:"required,gte=1,lte=10"`
- `LinearScale` additionally needs a custom check: `Max >= Min`
- `FormField.ID` -> `validate:"required"`
- `FormField.Type` -> `validate:"required,oneof=short_answer paragraph_text multiple_choice checkboxes dropdown linear_scale multiple_choice_grid checkbox_grid date time file_upload"`
- `FormField.Question` -> `validate:"required"`
- `FormField.Options` -> `validate:"required_if=Type multiple_choice required_if=Type checkboxes required_if=Type dropdown,min=1,dive"`
- `FormField.Scale` -> `validate:"required_if=Type linear_scale"`
- `Artifact.Path` -> `validate:"required"`
- `GlobPattern.Pattern` -> `validate:"required"`

### recipe_set (Input)

Defaults:
- None from `JobContext` today.

Required + constraints:
- `Input.Recipes` -> `validate:"required,min=1"`
- Each recipe entry must include `recipe` or `name` and must not include `git_state` or `run_mode` (custom validation; not expressible via tags).

### ticket.manage (Input)

Defaults:
- `Input.TicketID` -> `default:"{{ context.ticket.id }}"`

Required + constraints:
- `Input.Actions` -> `validate:"required,min=1,dive"`
- `Action.Type` -> `validate:"required,oneof=create_ticket update_ticket append_ticket_note link_markdown_doc override_markdown_doc remove_markdown_doc append_workflow_event reset_ticket"`

Action payload requirements (validated after JSON unmarshal; tag suggestions apply to the payload structs):
- `create_ticket`: `cell`, `project_id`, `title`, `stage`, `state` required.
  - `createTicketAction.Cell` -> `validate:"required"`
  - `createTicketAction.ProjectID` -> `validate:"required"`
  - `createTicketAction.Title` -> `validate:"required"`
  - `createTicketAction.Stage` -> `validate:"required"`
  - `createTicketAction.State` -> `validate:"required"`
- `update_ticket`: at least one of `stage`, `state`, `description` required (custom validation).
  - `updateTicketAction.ExpectedVersion` -> `validate:"omitempty,gt=0"`
- `append_ticket_note`: `note` required.
  - `appendTicketNoteAction.Note` -> `validate:"required"`
- `link_markdown_doc` / `override_markdown_doc` / `remove_markdown_doc`: `name` and `path` required.
  - `markdownAction.Name` -> `validate:"required"`
  - `markdownAction.Path` -> `validate:"required"`
- `append_workflow_event`: `workflow_id`, `run_id`, `status` required.
  - `appendWorkflowAction.WorkflowID` -> `validate:"required"`
  - `appendWorkflowAction.RunID` -> `validate:"required"`
  - `appendWorkflowAction.Status` -> `validate:"required"`
- `reset_ticket`: `reason` required.
  - `resetTicketAction.Reason` -> `validate:"required"`

### codex.exec (ExecOpInput)

Defaults:
- `WorktreePath` -> `default:"{{ context.environment.worktree_path }}"`
- `CellRelativePath` -> `default:"{{ context.workflow.cell_path }}"`

Required + constraints:
- `Prompt` -> `validate:"required"`
- `WorktreePath` -> `validate:"required"`
- `CellRelativePath` -> `validate:"required"`

### llm_inference (LLMInput)

Defaults:
- None from `JobContext` today.

Required + constraints:
- `Provider` -> `validate:"required,oneof=openai anthropic gemini"`
- `Model` -> `validate:"required"`
- `Prompt` -> `validate:"required"`
- `Temperature` -> `validate:"omitempty,gte=0,lte=2"`
- `MaxTokens` -> `validate:"omitempty,gte=1"`
- `TopP` -> `validate:"omitempty,gte=0,lte=1"`
- `StopSequences` -> `validate:"omitempty,dive,required"`

### llm_inference2 (LLMInferenceInput)

Defaults:
- `ToolWorkingDir` -> `default:"{{ context.environment.worktree_path }}"` (preferred over `"."` for repo-aware tools)
- `DefaultWorkingDir` -> `default:"{{ context.environment.worktree_path }}"`

Required + constraints:
- `Provider` -> `validate:"required,oneof=openai anthropic gemini"`
- `Model` -> `validate:"required"`
- `Prompt` -> `validate:"required_without=Files"`
- `Files` -> `validate:"required_without=Prompt"`
- `Temperature` -> `validate:"omitempty,gte=0,lte=2"`
- `MaxTokens` -> `validate:"omitempty,gte=1"`
- `TopP` -> `validate:"omitempty,gte=0,lte=1"`
- `FileHandling` -> `validate:"omitempty,oneof=native text_fallback hybrid"`
- `MaxFileContextSize` -> `validate:"omitempty,gte=0"`
- `Tools` -> `validate:"required_if=ExecuteTools true,min=1,dive"`
- `ToolTimeout` -> requires custom duration validation (string parsed by `time.ParseDuration`)
- `MaxToolRounds` -> `validate:"omitempty,gte=0"`

Tool definition (already tagged in code):
- `ToolDefinition.Name` -> `validate:"required"`
- `ToolDefinition.Description` -> `validate:"required"`
- `ToolDefinition.Parameters` -> `validate:"required"` (also must be valid JSON schema; custom validation already in `validateToolDefinition`)

Additional required-if logic (custom validation):
- If `ExecuteTools` is true, then `EnableToolExecution` must be true.

### extension ops (extInputsWrapper)

Defaults:
- None from `JobContext` at the wrapper level. Individual extension ops can embed defaults in their `op.yaml` schemas.

Required + constraints:
- `extInputsWrapper` is a `map[string]interface{}`; required fields are dictated by the op's `input_schema`.
- Use JSON Schema required fields in `op.yaml` instead of go-playground tags.

### command_execution (CommandExecutionInput)

Defaults:
- `WorkingDirectory` -> `default:"{{ context.environment.worktree_path }}"`

Required + constraints:
- `Run` -> `validate:"required"`
- `Timeout` -> requires custom duration validation (`time.ParseDuration`)
- `WorkingDirectory` -> `validate:"omitempty,dir"` (if supplied)
- `Shell` -> `validate:"omitempty,oneof=bash sh powershell cmd"` (optional; if allowing arbitrary shells, keep `omitempty`)

### sleep (SleepInput)

Defaults:
- None from `JobContext` today.

Required + constraints:
- `Duration` -> `validate:"required"` plus custom duration validation (`time.ParseDuration`)

### git_file_collector (GitFileCollectorInput)

Defaults:
- `ContextDir` -> `default:"{{ context.environment.worktree_path }}"`

Required + constraints:
- `ContextDir` -> `validate:"required,dir"`
- `MaxFileSize` -> `validate:"omitempty,gte=0"`
- `MaxTotalSize` -> `validate:"omitempty,gte=0"`
- `FilePatterns` -> `validate:"omitempty,dive,required"` (pattern validity still custom)
- `ExcludePatterns` -> `validate:"omitempty,dive,required"` (pattern validity still custom)

### git_shallow_clone (GitShallowInput)

Defaults:
- `SourceDir` -> `default:"{{ context.environment.worktree_path }}"`

Required + constraints:
- `SourceDir` -> `validate:"required,dir"`
- `TargetDir` -> `validate:"required"`
- `CommitHash` -> `validate:"required,hexadecimal,min=7,max=40"`

### git_persist_commit (PersistCommitInput)

Defaults:
- `RepoPath` -> `default:"{{ context.environment.worktree_path }}"`
- `RootHash` -> `default:"{{ context.git.resolved_hash }}"`
- `Author` -> `default:"{{ context.git.author }}"`

Required + constraints:
- `RepoPath` -> `validate:"required,dir"`
- `StorageLocation` -> `validate:"required"`
- `RootHash` -> `validate:"required,hexadecimal,min=7,max=40"`
- `Timeout` -> optional (uses `time.Duration`; no additional tag needed)

### git_restore_commit (RestoreCommitInput)

Defaults:
- `RepoPath` -> `default:"{{ context.environment.worktree_path }}"`
- `RootHash` -> `default:"{{ context.git.resolved_hash }}"`

Required + constraints:
- `RepoPath` -> `validate:"required,dir"`
- `TargetCommit` -> `validate:"required,hexadecimal,min=7,max=40"`
- `RootHash` -> `validate:"required,hexadecimal,min=7,max=40"`
- `StorageLocation` -> `validate:"required"`
- `Timeout` -> optional (uses `time.Duration`; no additional tag needed)

### thinpackrebase (ThinpackRebaseInput)

Defaults:
- `RepoPath` -> `default:"{{ context.environment.worktree_path }}"`
- `Context` -> `default:"{{ context }}"` (so `context.git.*` and `context.workflow.*` are present when input omits it)

Required + constraints:
- `TargetBaseHash` -> `validate:"required,hexadecimal,min=7,max=40"`
- `UpdateRefs` -> `validate:"omitempty"` (requires custom validation if restricting to ref formats)

### squashrebasemerge (SquashRebaseMergeInput)

Defaults:
- `RepoPath` -> `default:"{{ context.environment.worktree_path }}"`
- `TargetBranch` -> `default:"refs/heads/main"` (matches current runtime default)
- `Context` -> `default:"{{ context }}"`

Required + constraints:
- `TargetBranch` -> `validate:"omitempty"` (custom validation if restricting ref formats)

## Where to Run Validation in server/recipes

Proposed hook:
- Add op input validation into `server/recipes/internal/service/validation.go` inside `validateRecipe`.
- Run it after:
  1) recipe schema load + ID match
  2) existing CEL/contextual validation (`s.celValidator.ValidateCEL`)
- This ensures the new validation runs for:
  - explicit `ValidateRecipe` calls
  - `CreateRecipe`/`UpdateRecipe` with `AutoPublish`
  - `PublishRecipe` (since it calls `validateRecipe`)

Implementation sketch:
- Introduce a `RecipeOpInputValidator` (or extend the existing `CELValidator`) that runs:
  - default injection (already in compiler)
  - template resolution in validation mode
  - go-playground validation on decoded op inputs using the tags specified above
- Wire the validator call from `validateRecipe`, alongside the current `celValidator` usage, to keep publish-time behavior consistent.
