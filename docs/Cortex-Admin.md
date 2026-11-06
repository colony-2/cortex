# Cortex administration

Open **Admin** in the navigation for the selected tenant. Tabs are addressable at
`/project/{tenant}/admin?tab=hooks|configs|git|secrets` and work with browser history.
Cell detail pages have the same tabs alongside Overview.

- **Hook permissions:** list effective per-hook entries, add or edit permissions using
  secret-access rows, job-submission and cell-creation controls, and per-grant
  trust conditions, clear grants, or remove entries. Cell overrides replace the tenant
  entry for that hook. Clear permissions writes an explicit empty policy; Use
  tenant default removes the cell override. Policies are bound to the cell's
  registry repository, matching c2j. Source trust is validated by c2j.
- **Configs:** tenant-wide named JSON values. Reserved c2j policy/profile documents
  have dedicated editors and cannot be overwritten here. `git-profile-ro` and
  `git-profile-rw` select Git defaults; a JSON null disables the corresponding
  fallback. These are non-secret configuration values.
- **Git:** tenant-wide profiles with forms for repository destinations, URL rewrites,
  usernames, and credential secret names. Lists show routing and credential
  references without exposing the storage format. Credential values stay in
  the secret store; local credential files are not accepted. Use Secrets to provision values. Conventional profile
  names `read` and `write` act as defaults unless Configs overrides them.
- **Secrets:** referenced names only, since JobDB does not expose inventory yet.
  Set/replace a value or remove a known name from JobDB's writable secret store.
  External providers are managed separately. Existing values are never read,
  and supplied values use masked, write-only fields without a reveal control.

Configs, Git profiles and secret-store administration are tenant-wide; their cell
views are read-only and link to Admin. No additional cell-scoped semantics are
introduced. Config and policy writes use the revision loaded by the editor and
reject stale updates. On conflict, close the editor, refresh, and review the latest
value before editing again. Secret writes have no revision support upstream yet.

The Cortex adapter exposes `GET /api/projects/{tenant}/admin` and
`POST /api/projects/{tenant}/admin/{hooks|configs|git|secrets}`. A `cellId` query
parameter resolves the cell through the tenant registry; only hook policies can
be mutated in that scope. A mutation carries `name`, `operation`, optional
`value`, and (except for secrets) the required `revision`. Reads never resolve
secret references. See [the API feature request](C2J-Secret-Administration-API-Feature-Request.md)
for remaining secret-management gaps.
