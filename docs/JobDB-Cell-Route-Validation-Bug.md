# JobDB remote validation matches tenant names as cell routes

**Status: resolved in Cortex with JobDB `v0.0.70`.**

Verified on 2026-11-04 at JobDB commit
[`e199a54304e7`](https://github.com/colony-2/jobdb/commit/e199a54304e7922aa5e311a1d1c6b0dfe3a485dd),
Go version `v0.0.70-0.20261104212407-e199a54304e7`.
`TestCellValidationRouteScope` and `TestCellNamesDoNotInterceptJobRPCs` both pass.
They cover tenant/job identifiers beginning with `cells`, escaped path separators,
job listing, pending-input queries, lease acquisition, and cell resolution, while
retaining strict validation on actual cell routes.

Cortex adopted the fix with JobDB `v0.0.70` and c2j
`v0.0.118-0.20261104214226-dc36b1bdf6c9`. The UI and submission flow also adopt
ordinary `root` names and explicit cell registration.
`TestCellRouteValidationDoesNotMatchTenantOrJobNames` exercises Cortex against
remote SQLite with tenants `cells`, `cells-example`, and `registry-example`,
including job ID `cells-job`, empty job/input lists, submission, and cancellation.
The historical failure below no longer applies to the current dependency pair.

Originally observed with JobDB `v0.0.69` during the Cortex registry migration.

The remote server's `prepareCellRequest` in
`pkg/jobdb/runtime/remote/cells.go` selects routes with
`strings.Contains(r.URL.Path, "/cells")`. This also matches a tenant such as
`cells-example` on unrelated routes. Job-list RPC bodies contain optional null
fields, so the cell-specific envelope validator rejects them with HTTP 400
`{"error":"invalid_request","code":"invalid_request"}`.

Reproduction against Cortex connected to that remote server:

1. Create an ordinary cell with `POST /api/projects/cells-example/cells`, body
   `{"name":"example","repo":"github.com/acme/example"}`. Creation succeeds.
2. Fetch `GET /api/projects/cells-example/jobs?status=all`. It returns 400.
3. Repeat under tenant `registry-example`: the job list returns 200 with no jobs.

Pending-input polling is affected too. Cortex forwards the backend error.

The upstream fix now matches resource positions using the escaped URL path,
rather than arbitrary URL substrings. No Cortex workaround is needed after
adopting that revision. This report is local; no upstream issue was posted.
