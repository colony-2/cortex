# Job ID copy verification

Use the existing Playwright Chromium suite to accept the job ID copy repair.
The November 21 direction replaces the earlier native Safari/Chrome requirement:
“do the test in playwright chromium. desktop browser validation is not something
that should be required.” JC-25–27 are retired, not reported as passed. Native
desktop sessions and external candidate hosting are not prerequisites.

## Run the checks

From the repository root, use the toolchain declared by the Linux build verify
hook, including its pinned Chromium, FFmpeg and fonts. The `job-id-copy-tests`
step runs:

```sh
export CI=1
pnpm install --frozen-lockfile
pnpm --filter @colony2/app test src/components/CopyJobId.test.tsx -t CT-20
pnpm --filter @colony2/app test src/components/JobActionDialog.test.tsx
pnpm --filter @colony2/app test src/utils/copyText.test.ts src/components/CopyJobId.test.tsx src/components/JobsListPage.test.tsx src/components/JobActionDialog.test.tsx src/components/JobStoryPage.test.tsx
bash scripts/verify-browser.sh pnpm --filter @colony2/app test:e2e --config playwright.copy.config.ts
bash scripts/verify-browser.sh pnpm --filter @colony2/app test:e2e --config playwright.fixtures.config.ts --grep 'copies the title job'
```

Run type checks and production builds too:

```sh
pnpm --filter @colony2/shared typecheck
pnpm --filter @colony2/app typecheck
pnpm build:web
```

The managed recipe runs the configured broader suite before merge. Dedicated
copy cases run serially to avoid clipboard contention. The copy specification
uses its own Vite hostname allowance and Playwright configuration; it is excluded
from the default project. No additional browser platform is required.

## Clipboard evidence and failure simulation

Checks exercise the real React UI with intercepted API fixtures. On
`http://cortex-copy.test:15173`, Chromium maps the hostname to loopback but the
application remains an insecure context. Tests assert that `isSecureContext`
is false and `navigator.clipboard` is absent before first-click compatibility
copying. This origin receives no clipboard grants or secure-origin override.

A separate trustworthy page at `http://localhost:15173` seeds a sentinel and
independently reads the exact copied value. Clipboard read/write grants belong
to that verifier. Modern-success cases separately grant write permission to
the loopback application origin. The application itself never reads the
clipboard or queries clipboard permissions.

Modern rejection and compatibility-command failure are injected and labeled.
Fresh-click retry uses the real compatibility command; manual recovery uses
real keyboard copying. A success message or mocked successful command alone
is insufficient. Record exact clipboard assertions, command exit statuses,
test counts, candidate revision and Chromium/Playwright versions. An unrun
check remains unverified and a failed required assertion fails acceptance.

Successful copies use archive's Ant Design `message.success` defaults after the
write succeeds, including explicit compatibility retries. There is one global
success notice and no visible row-local success text. Because the installed Ant
Design static message has no live-region semantics, copy retains a single polite,
atomic local status announcement using the app's screen-reader-only style. A fresh
span on each success also updates repeated synchronous compatibility copies.
CT-20 renders the real message DOM and requires exactly one live success region;
Chromium checks its accessibility snapshot and clipped styling alongside the toast.
These are DOM/accessibility-tree checks, not native screen-reader listening tests.
Failures retain their visible polite status with retry and manual recovery. Obsolete
completions emit no toast or success announcement.
Browser checks use the real success notice, wait for earlier notices to disappear
between actions, and verify default placement, lifetime and unchanged row geometry.

The suite captures success, recovery and long IDs at desktop and phone widths. Inspect
selected actual captures before including them in review material. Linux
Chromium evidence does not claim native Safari or macOS Chrome validation,
and the original Mac deployment's cause remains unconfirmed.

## Active expectations

JC-01–24 remain active. The goal-specific overview has no copy control; its
separate JC-12 navigation check preserves exact-job navigation without adding
UI scope. All statements below are acceptance-critical (P0).

U: Vitest/jsdom with clipboard and command spies reset between cases.
R: U plus existing control/list fixtures, distinct IDs, two rows and controllable
write promises. B: Playwright Chromium, real UI, API fixtures and independent
clipboard verification. H: B plus non-local HTTP without application clipboard
grants or a secure-origin override.

| ID | Priority | Level | Filename | Depends | Expectation |
| --- | --- | --- | --- | --- | --- |
| JC-01 | P0 | unit | `web/app/src/utils/copyText.test.ts` | U | Expect the exact modern write immediately, with success only after fulfillment and no compatibility operation. |
| JC-02 | P0 | unit | `web/app/src/utils/copyText.test.ts` | U | Expect missing clipboard or non-callable writeText to invoke compatibility copying synchronously on the original click. |
| JC-03 | P0 | unit | `web/app/src/utils/copyText.test.ts` | U | Expect compatibility copying to select the exact complete value without trimming, decoration, or appended characters. |
| JC-04 | P0 | unit | `web/app/src/utils/copyText.test.ts` | U | Expect false, missing command, selection failure, and exceptions to produce contained failure without announcing success. |
| JC-05 | P0 | unit | `web/app/src/utils/copyText.test.ts` | U | Expect every compatibility outcome to remove temporary controls and restore surviving focus, text/document selection, and scroll. |
| JC-06 | P0 | unit | `web/app/src/utils/copyText.test.ts` | U | Expect detached prior focus targets to leave no temporary control and preserve a successful copy result. |
| JC-07 | P0 | unit | `web/app/src/components/CopyJobId.test.tsx` | R | Expect modern throw or rejection to reveal retry and manual recovery, without automatic compatibility copying or stealing focus. |
| JC-08 | P0 | unit | `web/app/src/components/CopyJobId.test.tsx` | R | Expect Copy another way to perform synchronous compatibility copying from its click, bypass modern writing, and announce confirmed success. |
| JC-09 | P0 | unit | `web/app/src/components/CopyJobId.test.tsx` | R | Expect compatibility failure to retain exact read-only manual recovery, remove alternative retry, and never announce success. |
| JC-10 | P0 | unit | `web/app/src/components/CopyJobId.test.tsx` | R | Expect older completions, unmounted controls, and replaced job IDs not to overwrite current feedback. |
| JC-11 | P0 | unit | `web/app/src/components/JobsListPage.test.tsx` | R | Expect grouped lists to copy the title attempt’s exact job ID, never its goal ID or another attempt. |
| JC-12 | P0 | unit | `web/app/src/components/JobsListPage.test.tsx` | R | Expect copying on jobs/goals list aliases to preserve URL, selection, expansion, and API request counts. |
| JC-12 navigation | P0 | integration | `web/app/tests/job-id-copy.spec.ts` | B | Expect the goal overview to retain exact-job navigation without gaining a copy control. |
| JC-13 | P0 | unit | `web/app/src/components/CopyJobId.test.tsx` | R | Expect another original copy click to clear earlier feedback and permit successful recovery after failure. |
| JC-14 | P0 | unit | `web/app/src/components/CopyJobId.test.tsx` | R | Expect macOS hints showing ⌘C, other hints Ctrl+C, labeled read-only fields, and polite local errors with recovery controls outside the status region. |
| JC-15 | P0 | integration | `web/app/tests/job-id-copy.spec.ts` | B/H | Expect mouse, Enter, Space, and compact touch activation to copy exact IDs while preserving another row’s feedback. |
| JC-16 | P0 | integration | `web/app/tests/job-id-copy.spec.ts` | H | Expect insecure-context and missing-API assertions before testing first-click HTTP compatibility copying without application clipboard grants. |
| JC-17 | P0 | integration | `web/app/tests/job-id-copy.spec.ts` | H | Expect an independent clipboard comparison to replace a seeded sentinel with the exact copied ID after non-local HTTP activation. |
| JC-18 | P0 | integration | `web/app/tests/job-id-copy.spec.ts` | B | Expect injected modern rejection followed by fresh-click real compatibility copying to produce the exact independently verified ID. |
| JC-19 | P0 | integration | `web/app/tests/job-id-copy.spec.ts` | B | Expect both automatic methods failing to retain manual selection supporting real keyboard copying and exact independent clipboard verification. |
| JC-20 | P0 | integration | `web/app/tests/job-id-copy.spec.ts` | B | Expect recovery and long IDs at 1280, 390, and 320 pixels to remain visible without changing values or overflowing horizontally. |
| JC-21 | P0 | integration | `web/app/tests/job-id-copy.spec.ts` | B | Expect phone retry targets at least 44 pixels high, with full-card recovery placed below summary controls. |
| JC-22 | P0 | integration | `web/app/tests/job-id-copy.spec.ts` | B | Expect focused retry removal to focus the original copy button on success or the selected manual field on failure. |
| JC-23 | P0 | integration | `web/app/tests/job-id-copy.spec.ts` | H | Expect compatibility copying to restore surviving input focus, backward selection, and page scroll, leaving no temporary textarea. |
| JC-24 | P0 | integration | `web/app/tests/job-id-copy.spec.ts` | B/H | Expect contained failures without uncaught page errors, application clipboard reads, or application permission queries. |


Restore document ranges before text-control selection: reversing that order
previously cleared Chromium's input selection. Keep this regression covered.
macOS shortcut hints are unit-tested values and create no desktop prerequisite.
