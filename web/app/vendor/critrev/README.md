Critrev is installed from the immutable local tarball identified in
[package-identity.json](package-identity.json), retaining its [MIT license](LICENSE).
Source: https://github.com/colony-2/critrev.

Version `0.1.5-link.20261121.t2145.xwtz5b.r3` adds the semantic
`applyMarkdownFormat({ action: 'link', url, placeholder })` public command.
Cortex uses the mode-free `ReviewEditorMarkdownFormatInput` type. Selected rich
text becomes an anchor; a collapsed selection inserts a selected placeholder.
Existing source formatting, empty initialization and image-history behavior remain.

Delivery: completed Critrev job `Xwtz5bTf3qyqxOskRZVyicukGkb`.
The native merge record identifies `9888037b4d4dffc5763d2db7d6059d59eb3c949f` on
main. The immutable tarball records clean build revision
`9009afc8db2baddda47f389112fb7aba091d7d05`; both revisions have source tree
`a02cabaf93f1e9f2cb750dee0a929ceb0731327f`. The delivered archive is not repacked.
SHA-256: `218d5a3d49f6e2f9949e1383de5d598d7d3412ea591c9f584ff1ffb17453e9fd`.
The pnpm lockfile pins recomputed npm integrity. Native publication was skipped
because no publish hook is configured; no registry publication is claimed.

This supersedes the tarball from completed editor-empty replacement
`e489021a-d40c-5f0c-88df-906f8e2a5a9c`, retaining that correction without duplicate
owner work. Import only public `critrev` and `critrev/react` entry points. Cortex
owns image bytes, previews, retained undo quotas, submission and authorized readers.
