# Embedded Temporal Story Builder Test Spec

## Objective
Validate `storybuilder` against real Temporal history by executing a recipe via an embedded Temporal server, then rebuilding the execution story through `Client.BuildStory`.

## Status
1. Embedded Temporal server bootstrapping (temp DB, free port selection, manual namespace registration, Temporal client lifecycle) — **done**.
2. Worker wiring with `ActivityRegistry`, custom inline/activity ops, and parent/child recipe workflows — **done**.
3. Recipe fixtures covering inline, activity, child recipe, and input ops registered and loading helper implemented — **done**.
4. Workflow execution & signal coordination — **done**: verified the inline input invocation hash (`e828a08912993e7c`), registered the missing search attributes so the inline op can upsert metadata, and signalled the workflow successfully so the run completes.
   1. Review existing input workflow tests (`server/ops/pkg/input/*`) to document how they compute/emit the signal channel and hash — **done** (tests signal `user-response:<InputWorkflowParams.ID>` directly; invocation hash is deterministic per invocation tracker rather than random).
   2. Trace invocation generation in `compiler.ExecuteRecipe`/`newInvocationTracker` for the input node used in this fixture — **done** (tracking shows segments duplicate the recipe ID, yielding node path `story-parent/story-parent/<node>`; instrumentation confirms the input node should hash to `e828a08912993e7c`).
   3. Capture the inline input invocation ID at runtime (via story markers or search attributes) and log it — **done** (temporary wrapper around the input op reports the handler receiving `inv.Hash()` = `e828a08912993e7c`, matching the deterministic computation).
   4. Align signal dispatch with the captured ID and re-run the integration test until the workflow completes — **done** (registered required search attributes on the embedded namespace and re-sent the captured hash; signals now land, workflow proceeds past the input step, and the run completes).
5. Output verification and story reconstruction assertions — **done**: with the workflow completing, the test now asserts metadata, node runs, input responses, child workflow results, and timeline ordering successfully.

## Pre-reqs
- Use `server/embeddedtemporal/pkg/temporal.Server` to host Temporal with SQLite persistence.
- Reuse the main recipe-worker runtime (`compiler.ExecuteRecipe`, `workerops.ActivityRegistry`).
- Sample recipe fixture that covers inline ops, activity ops, child recipe invocation, and input signals.

## Test Setup
1. **Start Embedded Temporal**
   - Create temp dir for database: `dbPath := filepath.Join(t.TempDir(), "temporal.db")`.
   - Pick free frontend port (e.g. via `temporal.IsPortAvailable`).
   - Construct `temporal.Options`:
     ```go
     opts := temporal.Options{
       FrontendIP:    "127.0.0.1",
       FrontendPort:  port,
       UIPort:        0,
       DatabaseFile:  dbPath,
       Namespaces:    []string{"default"},
       DisableScanners:          true,
       DisableParentClosePolicy: true,
       DisableNexus:            true,
       EnableInternalWorker:    false,
     }
     ```
   - Call `temporal.NewServer(opts)` then `server.Start()` inside `t.Cleanup` to ensure `server.Stop()`.
   - The readiness helper waits for TCP availability; namespace creation happens asynchronously.

2. **Temporal Client**
   - Build `client.Options{HostPort: server.GetFrontendAddress(), Namespace: "default"}` and call `client.NewClient`.
   - Register cleanup: `client.Close()`.

3. **Worker / Registry Wiring**
   - Instantiate registry: `registry := workerops.NewActivityRegistry()`.
   - Set dependencies: `registry.SetDependencies(ops.NewServiceDepsBuilder().Build())`.
   - Create Temporal SDK worker with `workflowworker := worker.New(client, taskQueue, worker.Options{})`.
   - Register activities: iterate `registry.GetAll()`, wrap inline `ExecuteV2` as in existing tests.
   - Register workflow wrapper that executes recipe: `workflowworker.RegisterWorkflow(func(ctx workflow.Context, input map[string]interface{}) (map[string]interface{}, error) { return compiler.ExecuteRecipe(ctx, registry, recipe.Recipe, input) })`.
   - Start worker in goroutine; stop via `defer workflowworker.Stop()`.

4. **Recipe Fixture**
   - Load YAML from `server/recipe-worker/test-fixtures/recipes` using existing helper:
     ```go
     data, _ := os.ReadFile(path)
     recipeFile, err := recipe.LoadRecipeFromReader(bytes.NewReader(data))
     ```
   - Wrap into `recipe.RecipeFile{ID: "story-test", Version: "1.0.0", Recipe: *recipeFile}` for worker manager compatibility (if needed).
   - Ensure the recipe uses:
     - An inline `command_execution` op (to yield inline markers).
     - An activity-mapped op (also `command_execution` but `ExecuteAsActivity() == true` variant).
     - A `recipe` op referencing a small child recipe (e.g. simple echo) so child markers emit.
     - An `input` op waiting on user signal.
   - If fixtures missing these, create `story-builder.yaml` specifically for the test.

5. **Invocation Hash For Input Signal**
   - Input op waits for `user-response:<invocation-hash>`.
   - Derive hash using `coreops.Invocation{RecipeID: recipeFile.GetMetdata().ID, NodePath: "<path>", InvokeSeq: 0}.Hash()`; `NodePath` matches tracker path (`segmentForMetadata` logic), typically `sequenceID/nodeID`.

## Execution Flow
1. Provide base inputs (git repo, ticket ID) mirroring other tests.
2. Start workflow via client `we := client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{TaskQueue: taskQueue, ID: "story-test"}, workflowName, inputs)`.
3. Wait for input op to block (optional short sleep or poll `DescribeWorkflowExecution` for `PendingActivities`).
4. Send user signal: `client.SignalWorkflow(ctx, workflowID, we.GetRunID(), "user-response:"+invHash, payload)`.
5. Await completion: `var out map[string]interface{}; require.NoError(t, we.Get(ctx, &out))`.

## Story Reconstruction
1. Use same Temporal client to fetch metadata: `desc, _ := client.DescribeWorkflowExecution(ctx, workflowID, we.GetRunID())`.
2. Stream history: `itr := client.GetWorkflowHistory(ctx, workflowID, we.GetRunID(), false, enumspb.HISTORY_EVENT_FILTER_TYPE_ALL_EVENT)`; iterate `itr.Next()`.
3. Initialize builder: `b := storybuilder.New("story-test", recipeFile)` and `b.SetExecutionInfo(desc)`.
4. Feed events via `b.Process(event)`.
5. Call `story := b.Build()`.

## Assertions
- `story.Metadata` should reflect the workflow IDs, status `completed`, and non-zero duration.
- Inline node: run status transitions from start marker to completion, inputs/outputs present.
- Activity node: run populated from schedule/start/complete events.
- Child recipe node: trigger/result events recorded with child workflow IDs and metadata; run includes outputs.
- Input node: event list contains `input-response` with user payload, and timeline entry referenced.
- Timeline ordered chronologically with event IDs matching history.

## Cleanup
- Stop worker (`workflowworker.Stop()`), cancel context used for worker start.
- Close Temporal client.
- Call `server.Stop()` (covered by cleanup).
- Remove temp directories automatically via `t.TempDir()`.

## Notes
- Embedded Temporal server defaults to namespace creation asynchronously; integration test should retry workflow start if namespace not yet ready (e.g., wrap start in `require.Eventually`).
- Keep test under build tag (e.g., `//go:build temporal_integration`) if runtime cost is significant.
- Use `context.WithTimeout` (~60s) for client operations to avoid hangs.
