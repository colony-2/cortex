## Migration: Typed Workflow Control (ServiceDependencies2)

  This document defines an idempotent, stepwise migration to introduce a typed workflow control accessor alongside existing ServiceDependencies, roll it out safely, and then remove legacy patterns.

  Run each detect command in a project’s root. A step is considered complete for a component when its detect command returns no matches. Only proceed to the next step once the current step shows zero matches across all components participating in that step.

  ### Ordered Steps

  1. Provide typed controller at call sites
      - To-be-applied change: When constructing or passing ServiceDependencies, wrap with ops.WithWorkflowControl(base, ctl) or ensure the "workflowctl" key is registered so a typed workflowctl.WorkflowControl is available.
      - Can apply when: The component can produce a workflowctl.WorkflowControl adapter or already registers one under key "workflowctl".
      - Detect command:

        sh -lc '
        FILES=$(grep -RIl -E "func\s*\(.*\)\s*Get\s*\(name\s+string\)\s*\(interface\{\},\s*error\)" . --exclude-dir node_modules --exclude-dir vendor || true);
        for f in $FILES; do
          if ! grep -qE "workflowctl|ops\.WithWorkflowControl\(" "$f"; then echo "$f"; fi;
        done
        '
  2. Consumers use typed accessor optionally
      - To-be-applied change: In Initialize(deps ops.ServiceDependencies) implementations, add an optional type assertion to ops.ServiceDependencies2 and prefer v2.WorkflowControl() when present; keep existing deps.Get(...) as fallback.
      - Can apply when: Step 1 shows zero remaining files in this component or adding a non-breaking optional assertion is acceptable.
      - Detect command:

        sh -lc '
        CANDS=$(grep -RIl --exclude-dir node_modules --exclude-dir vendor -E "Initialize\s*\([^)]*ops\.ServiceDependencies[^)]*\)" . || true);
        if [ -n "$CANDS" ]; then
          echo "$CANDS" | xargs -r grep -L -E "(ServiceDependencies2|WorkflowControl\s*\()" || true;
        fi
        '
  3. Dependency containers implement ServiceDependencies2
      - To-be-applied change: For types implementing ops.ServiceDependencies, add WorkflowControl() (workflowctl.WorkflowControl, bool) so they satisfy ops.ServiceDependencies2. Return (nil,false) until a controller is available.
      - Can apply when: The component owns a ServiceDependencies implementation.
      - Detect command:

        sh -lc '
        FILES=$(grep -RIl -E "func\s*\(.*\)\s*Get\s*\(name\s+string\)\s*\(interface\{\},\s*error\)" . --exclude-dir node_modules --exclude-dir vendor || true);
        for f in $FILES; do
          if ! grep -qE "\bWorkflowControl\s*\(\)\s*\(.*workflowctl\.WorkflowControl.*,\\s*bool\)" "$f"; then echo "$f"; fi;
        done
        '
  4. Remove legacy string-based retrieval
      - To-be-applied change: Replace usage of workflowctl.From(deps) and deps.Get("workflowctl") with the typed accessor ServiceDependencies2.WorkflowControl(). Keep a temporary fallback only if needed for interop.
      - Can apply when: All Initialize implementations in this component support the ServiceDependencies2 type assertion or you maintain a temporary fallback.
      - Detect command:

        sh -lc 'grep -RIn --exclude-dir node_modules --exclude-dir vendor -E "workflowctl\.From\(|Get\(\"workflowctl\"\)" . || true'
  5. Upgrade Initialize signature
      - To-be-applied change: Change Initialize(deps ops.ServiceDependencies) to Initialize(deps ops.ServiceDependencies2) and update implementations/callers accordingly.
      - Can apply when: Steps 1–4 report zero matches organization-wide (global barrier).
      - Detect command:

        sh -lc 'grep -RIn --exclude-dir node_modules --exclude-dir vendor -E "Initialize\s*\([^)]*ops\.ServiceDependencies[^)]*\)" . || true'
  6. Eliminate deprecated adapters
      - To-be-applied change: Remove transitional helpers and deprecated patterns in this component: ops.WithWorkflowControl wrapper usage, workflowctl.From, and any remaining references to ServiceDependencies2 after signatures are upgraded.
      - Can apply when: Step 5 shows zero results in this component.
      - Detect command:

        sh -lc 'grep -RIn --exclude-dir node_modules --exclude-dir vendor -E "ops\.WithWorkflowControl\(|workflowctl\.From\(|\bServiceDependencies2\b" . || true'

  ### Projects Affected (current scan)

  Only projects with at least one pending step are listed.

  | Project            | Step 1 | Step 2 | Step 3 | Step 4 | Step 5 | Step 6 |
  |--------------------|:------:|:------:|:------:|:------:|:------:|:------:|
  | server/recipe-core |        |        |        |   ✔    |        |   ✔    |
  | server/ops         |   ✔    |   ✔    |   ✔    |        |   ✔    |        |
  | server/api         |   ✔    |        |   ✔    |        |        |        |
  | server/cortex      |   ✔    |        |   ✔    |        |        |        |

  Notes:

  - The table reflects the latest automated scan at authoring time. Rerun the detect commands in each project directory to refresh status.
  - Steps are idempotent; re-running detection after applying changes should eventually yield no matches for that step.
