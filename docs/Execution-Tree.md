# Job execution tree

Open the **DAG** tab in a job story or follow a DAG link from selected job details in Goals. Generic jobs use `/project/<tenant>/jobs/<entry-job>/tree`. The opened job stays highlighted when you select another job; **Find opened job** returns to it.

The desktop diagram connects recorded runtime parents to children across cells and job types. Card selection and branch controls work by keyboard and on narrow screens. Select a job for its full identity, status, goal and discovery coverage. Recipe jobs can open their story and return to the original tree.

The selected-job pane has small icons at its top right for **Cancel job**, **Goal overview** and **Open execution**, when each action is available. Hover or focus an icon to read its tooltip. The icons have accessible names and larger touch targets on narrow screens.

Search matches **loaded jobs** and reveals their ancestry. Collapse affects presentation only. Zoom and Fit adjust the diagram; its viewport scrolls. Refresh starts discovery again. There is no automatic tree polling.

Discovery follows authorized retained records, including hidden and archived jobs. Recipe attribution, shared goals, waits and copied history do not create ancestry edges. Direct restarts are parentless; lease restarts follow their submitting job. Missing ancestors and malformed relationships remain boundaries.

Each view retains at most 1,000 nodes, 64 ancestor hops, 64 descendant levels and 200 attempted child pages. Each initial/resume/retry operation allows 300 requests and 30 seconds. Calls currently run serially, within the maximum concurrency of four. Failed pages retain their cursor for explicit retry. Request/time limits allow bounded resume; persistent limits require a fresh view. Opening a branch as the entry preserves that job preferentially but still ascends recorded parents and remains subject to the same limits.

Coverage reports loading, exhausted listings, failures and limits, with observation times. “No children found in this traversal” requires a successfully exhausted listing. Finished discovery describes the authorized reachable records returned during those observations; it cannot establish complete historical coverage or an atomic snapshot.

## Cancel a selected job

The selected-job pane offers **Cancel job** for every unfinished recipe or generic job, including jobs waiting, scheduled, expired or repeatedly failing. **Job keeps failing** is the display label for `CRASH_CONCERN`. Confirmation identifies the tenant and job and accepts an optional cancellation message. Submitted child jobs remain independent, running work may take time to stop, and performed actions are retained.

After submission, Cortex checks that exact job again and updates its card and pane without resetting diagram controls. **Cancellation requested** remains pending until a fresh read confirms termination. Use **Refresh status** when the request is pending or its outcome is unavailable. A completion race preserves the recorded outcome. Reading a hidden job in the tree does not grant cancellation access.
