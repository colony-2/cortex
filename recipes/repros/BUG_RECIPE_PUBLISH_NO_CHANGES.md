# Bug: `c2 recipe update --publish` fails when content is unchanged (fixed)

## Summary

Publishing recipes via `c2 recipe update ... --publish` should be idempotent (“ensure this recipe is published”), even when the provided recipe content matches the latest stored content (no diff).

This issue was causing `c2 recipe update ... --publish` (and sometimes `c2 recipe update` without `--publish`) to fail when there were no changes to commit. The bug has been addressed: `update --publish` should always succeed for a valid recipe, regardless of whether the content is changing.

## Status

- Fixed (behavioral contract: `update --publish` is always safe/idempotent for valid recipes).

## Expected behavior

Running:

```bash
c2 recipe update <recipe-name> --content-file <path> --publish
```

Should **not** fail when the YAML content is identical to the latest revision. Instead it should always return success for a valid recipe, and:

- If the latest revision is already published: return success (no-op).
- If the latest revision is not published: publish the latest revision and return success.

Similarly, `c2 recipe update ... --content-file ...` without `--publish` should return success (no-op) when content is identical (not an HTTP 500).

## Actual behavior

When the content is identical to the latest revision, `c2 recipe update` fails with an HTTP 500:

```text
unexpected status 500 PUT http://host.docker.internal:8080/api/projects/<project>/recipes/<name>: {"error":"no changes to commit"}
```

In practice, this also prevents `--publish` from being usable as an “ensure published” operation.

## Reproduction steps

1. Ensure a recipe exists (any recipe name works).
2. Run an update with content identical to the current latest content:
   ```bash
   c2 recipe update <recipe-name> --content-file <path-to-identical-yaml>
   ```
3. Observe failure with `{"error":"no changes to commit"}` and HTTP 500.
4. (Publishing case) Run:
   ```bash
   c2 recipe update <recipe-name> --content-file <path-to-identical-yaml> --publish
   ```
5. Observe the same behavior (or inability to publish due to the update error).

## Impact

- Makes publishing non-idempotent and brittle for CI/CD.
- Forces “fake edits” (e.g., bumping `version:`) just to publish, creating unnecessary churn/noise in recipe history.
- Prevents using `--publish` as a declarative “ensure published” operation.

## Workaround

Make an artificial change (e.g., bump `version:` or add whitespace) before re-running `c2 recipe update ... --publish`.

## Suggested fix

Backend/API behavior:

- Treat “no diff” as a successful update (return 200/204 + stable metadata).
- When `--publish` is requested:
  - If there is no diff, still publish the latest revision if it is not already published.
  - If already published, return success (no-op).

CLI behavior:

- If the server indicates “no diff”, exit 0 and print a message like “no changes; already up to date”.
  - If `--publish` is set, print “no changes; published latest revision” (or “already published”).

## Notes

This was observed on 2026-02-03 (local API at `host.docker.internal:8080`).

## Verification

With a recipe that already exists:

```bash
c2 recipe update <recipe-name> --content-file <path-to-identical-yaml> --publish
```

Should exit 0 and leave the recipe published (no forced edits needed).
