# shai-alias Diagnostics

When `shai-alias` fails it can be difficult to tell whether the problem is
missing env, blocked egress, or the host-side supervisor. To make the failure
mode obvious we now ship an optional verbose mode that prints the key signals
before invoking the alias.

## Verbose Mode

```
shai-alias --verbose <alias> [args...]
shai-alias --verbose --list
```

When `--verbose` (or `-v`) is present the helper prints, to stderr:

1. Timestamp, mode, alias name, and args that will be forwarded.
2. All `SHAI_ALIAS_*`, `ALLOW_DOCKER_HOST_PORT`, and `DEV_UID` env vars.
3. Current `id` output so we know which user is running.
4. A `nc -vz host.docker.internal:<port>` probe so we can tell if the host port
   is reachable.

After the diagnostics finish the script executes the exact same SSH invocation
as the non-verbose path, so behaviour is unchanged aside from the extra logs.

## Usage Tips

- `shai-alias --verbose --list` is the quickest sanity check; it exercises the
  host connection without running a backing command.
- The integration tests call `shai-alias --verbose ...` automatically so CI
  logs always capture the diagnostics.
- Combine with `SHAI_ALIAS_DEBUG=1` on the host to surface supervisor logs when
  running `shai` locally.

If the `nc` probe fails, focus on the container bootstrap path (`ALLOW_DOCKER_HOST_PORT` or
`dev-egress-setup`). If diagnostics succeed but the command still fails, the
issue is on the host side (e.g., `.shai-cmds` validation or the supervisor).*** End Patch
