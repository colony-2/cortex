# Secret administration APIs for Cortex

Baseline: c2j `0252e9b3c1da`, JobDB `v0.0.71`.

Cortex now has an Admin page for tenant hook policies, configuration, Git
profiles, and secrets. Cell pages expose the existing repository-bound hook
policy overrides and read-only views of tenant settings. This request does not
propose additional cell-scoped settings or authentication work.

## Existing capabilities

- `jobdb.ConfigurationAdmin` provides coherent reads and revision-checked writes.
- c2j's public `hookpolicy` package validates, selects and edits tenant/cell hook
  policies; `gitprofile` validates profiles containing secret references.
- JobDB's `openapi/jobdb-dev-secrets.yaml` exposes write-only set and remove
  endpoints under `/dev/v1/tenants/{tenantId}/secrets/{set,remove}`. These operate
  on the configured writable secret store, not arbitrary external providers.
- The public remote Go runtime has no corresponding secret-administration client;
  c2j currently implements one inside `cmd/c2j/internal/swfruntime/administration.go`.
- Runtime secret resolution requires a job lease and returns values. It is not a
  suitable API for an administration inventory, and Cortex does not call it.

## Requested APIs

1. **Metadata-only inventory.** Add a paginated tenant secret listing containing
   name, provider/store identifier, and supported operations. Include version and
   update timestamps when available. Never return secret values, previews,
   prefixes, hashes, or lengths. Distinguish provider inventory unsupported from
   an empty store. A list of references in configuration is not an inventory.
2. **Write capability discovery.** Expose which stores/providers support set,
   replace, and remove, with an explicit unavailable response when none does.
   Existing dev-store writes can remain supported; provider-backed credentials
   should not appear editable unless their provider supports administration.
3. **Public Go client.** Export typed secret-administration methods usable by
   Cortex and c2j without copying internal CLI transport code. Preserve no-store
   responses, bounded requests, redirect refusal and sanitized errors. Keep
   values write-only, including in errors and diagnostics.
4. **Conflict-safe replacement/removal.** Where the store supports versions,
   accept expected versions to avoid overwriting or deleting a concurrent update.
   Report unsupported conditional writes explicitly for other providers.

## Current Cortex behavior and acceptance criteria

Cortex shows names referenced by effective hook policies and Git profiles,
explicitly labeled as references rather than stored-secret inventory. It accepts
new/replacement values and removes known names using the existing writable-store
API. Password inputs have no reveal control, values are never fetched, drafts
are discarded after submission, and provider error bodies are never forwarded.

With the requested APIs, Cortex should be able to enumerate secrets without a job
lease or retrieving any credential, show each entry's supported actions, update
or remove it safely, and distinguish missing metadata support from an empty list.
