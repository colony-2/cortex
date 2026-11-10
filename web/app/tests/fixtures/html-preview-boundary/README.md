# Native HTML frame boundary experiment

`probe.mjs` creates three disposable local HTTPS servers, a signed-in host control,
protected read/write endpoints and a cancellation-shaped endpoint. It records
received credentials and final mutation counts. No Cortex, JobDB or third-party
service is contacted. TLS keys are generated in the scratch evidence directory.

Run after installing the repository's Playwright development dependencies:

```sh
CHROMIUM_PATH="$(command -v chromium)" \
EVIDENCE_DIR=/tmp/html-preview-boundary \
node web/app/tests/fixtures/html-preview-boundary/probe.mjs
```

With a provisioned Playwright package instead, set `PLAYWRIGHT_MODULE` to its
absolute module directory. Chromium uses the normal fontconfig environment;
this worker needs `FONTCONFIG_FILE` pointing to the existing DejaVu fonts in
`/nix/store`. See the retained validation report for the exact executed command.
Node, Chromium and OpenSSL are required. Only synthetic cookie values are logged.

Seven cases distinguish sandbox isolation, credentialless storage, document CSP,
self-navigation and iframe-required CSP. The assertions intentionally establish
that weaker policies allow writes; passing this diagnostic does not establish
product acceptance. The probe reads the implemented CSP from htmlPreparation.ts. The product
security suite supplies the remaining request-channel and lifecycle assertions;
this experiment alone is not the complete HP-014/015 suite. The screenshot shows the synthetic experiment,
not the Cortex viewer.

`server/internal/cortex/html_preview_boundary_test.go` independently exercises
Cortex's actual local handler: a cookie-free opaque-origin POST can cancel a
job. This diagnostic documents the baseline to protect against when enabling
scripts. Revise/remove its insecure-baseline expectation if API enforcement is
added in a later assignment.
