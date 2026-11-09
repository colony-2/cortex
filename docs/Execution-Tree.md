# Job execution tree

Open **Execution tree** from a job story or selected job details in Goals. You can also open `/project/<tenant>/jobs/<entry-job>/tree` directly. The entry stays highlighted when you select other jobs; **Find entry job** returns to it.

The desktop diagram connects recorded runtime parents to children across cells and job types. The outline offers the same selection and branch controls by keyboard and on narrow screens. Select a job for its full identity, status, goal and discovery coverage. Recipe jobs can open their story and return to the original tree.

Search matches **loaded jobs** and reveals their ancestry. Collapse affects presentation only. Zoom and Fit adjust the diagram; its viewport scrolls. Refresh starts discovery again. There is no automatic tree polling.

Discovery follows authorized retained records, including hidden and archived jobs. Recipe attribution, shared goals, waits and copied history do not create ancestry edges. Direct restarts are parentless; lease restarts follow their submitting job. Missing ancestors and malformed relationships remain boundaries.

Each view retains at most 1,000 nodes, 64 ancestor hops, 64 descendant levels and 200 attempted child pages. Each initial/resume/retry operation allows 300 requests and 30 seconds. Calls currently run serially, within the maximum concurrency of four. Failed pages retain their cursor for explicit retry. Request/time limits allow bounded resume; persistent limits require a fresh view. Opening a branch as the entry preserves that job preferentially but still ascends recorded parents and remains subject to the same limits.

Coverage reports loading, exhausted listings, failures and limits, with observation times. “No children found in this traversal” requires a successfully exhausted listing. Finished discovery describes the authorized reachable records returned during those observations; it cannot establish complete historical coverage or an atomic snapshot.
