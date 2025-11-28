# SWF Migration - server/recipe-history

## Objectives
- Retire the Temporal-based recipe-history module now; plan a future rebuild on SWF/Strata when needed.

## Scope & Plan (Temporal usage to remove)
- Entire module `server/recipe-history` is Temporal-focused (history parsing, rewind specs, embeddedtemporal tests); delete code, tests, and docs.
- Capture requirements for a future SWF-native history/story reader (chapters, rewind via RestartJob) when reinstated.
- Ensure downstream references are guarded or stubbed until the rebuild.

## SWF Gaps Affecting This Project
- Limited status APIs: progress inferred from chapters rather than workflow execution status.
- No signal/query events: rely solely on compiler-emitted envelopes.
