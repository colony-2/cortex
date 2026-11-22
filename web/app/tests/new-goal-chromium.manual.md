# New-goal composer acceptance in Chromium

Accepted scope: “Just use chromium in different sizes. No physical phone tests
required,” requested November 21, 2026 under
`4f9a5ea8-ef44-4cdd-854a-7261b6b4f4f2`, and accepted in revised direction
`a0a7c073-08db-446e-96a6-9608dc59fdea`. This replaces the former phone protocol.
Physical phones, Safari, native screen readers, device services and tester
recruitment are outside this acceptance. Simulated paste does not establish OS
clipboard behavior; viewport simulation does not establish physical keyboards.

Use the pinned installed Chromium and frozen workspace dependencies. Record the
upstream revision, candidate diff identity, Chromium version, command, viewport or
native zoom, actual result and selected captures. Do not download another browser
or skip a required failure. The recipe owns complete verification before merge.

## Automated browser checks

From the repository root, in the verify hook's supplied tool environment:

```sh
pnpm install --frozen-lockfile
bash scripts/verify-browser.sh node scripts/verify-command.mjs pnpm --filter @colony2/app test:e2e --config playwright.composer.config.ts
bash scripts/verify-browser.sh node scripts/verify-command.mjs pnpm --filter @colony2/app test:e2e --config playwright.composer-runtime.config.ts
```

The first configuration uses deterministic HTTP fixtures and synthetic clipboard
events. The second uses the local Go test API and SQLite state to persist actual
multipart requests and compare stored image bytes. Report their results separately.
The composer configuration includes a test-only Chromium extension for native
200% zoom; changing device scale factor or CSS zoom alone does not satisfy it.

## Observable acceptance

1. At 1440×900, expect a 1120×836 composer with visible header and actions. Enter
   text, select a cell, attach a file and paste a raster. Resize through widths
   320, 390, 767, 768 and 1440 without losing draft state. Below 768px, expect
   edge-to-edge takeover, square corners and no horizontal overflow; at 768px,
   expect desktop margins. Confirm New goal exposes existing cells only.
2. At 390×400, fill a long draft. Scroll the body and focus Attach files. Header,
   focused controls and Create goal must remain reachable. Simulate visualViewport
   height 400 and offsetTop 40. Confirm retained text and reachable actions.
   Rotate the dimensions to 844×390 and confirm state and horizontal containment.
3. In Chromium, emulate safe-area insets using CDP `Emulation.setSafeAreaInsetsOverride`: top 24px, left/right 20px and
   bottom 30px. Confirm the corresponding header/body/footer padding, 16px editor
   text and at least 44px primary controls. Submit an empty prompt: validation
   must be associated with the labeled Prompt textbox. Tab through the dialog;
   focus must stay contained and visible. Escape must return focus to the opener.
4. Use the native-zoom test to set and read Chromium tab zoom 2. With a fixed
   1280×900 browser viewport, confirm innerWidth 640, innerHeight 450 and DPR 2.
   Enter long content; focus controls and the final caret. Confirm the caret lies
   inside the scrolling body above the footer, controls remain reachable, focus
   stays contained through 30 Tabs, and no horizontal overflow occurs. Restore
   zoom 1 and confirm the draft remains. Inspect the compositor capture.
5. Exercise all Markdown toolbar formats and literal review delimiters. Open
   More formats by keyboard at 390px, apply a heading to selected text, and resize.
   Confirm exact source retention on incompatible conversion. From untouched and
   populated drafts, enter `---` and `## ` in source: preserve exact characters
   and trailing space, show required validation, submit nothing and raise no error.
6. Paste valid PNG, JPEG, GIF and WebP files; confirm previews and original bytes
   in submitted multipart bodies. A mixed image/plain-text/HTML paste inserts one
   image and explains omitted document content. HTML-only remote images must not
   trigger image requests. Ordinary text paste must remain editable. Batch paste
   replaces the selection in one undo step; delayed previews must preserve later
   deletion and caret position. Verify shared quotas, undo/redo and retry identity.
7. With the native API fixture, use keyboard Attach files, remove and reattach a
   file, and submit. Confirm exact stored bytes. Paste a PNG and submit at 1440px
   and 390px; navigate to goal and story readers and confirm the owning job's
   persisted image and original download. Discard/reopen must release the draft.

## Result record

For each run, record results in the selected review material. Required failed
assertions remain failed acceptance; unavailable checks remain unverified.
Retained evidence must identify its original source and must not be called a new
run. A screenshot establishes only what appears inside its frame. Automated
assertions establish off-screen validation, exact bytes and request behavior.
