# Web Update Plan (new API alignment)

## Goals
- Adapt web apps to the new API under `/api/projects/{projectId}/…` while keeping flowchart and kanban experiences.
- Identify what to keep vs. remove in `web/*`, then clean and rewire to the new project-scoped endpoints and data shapes (cells/boxes → cells; tickets; graphs).

## What to keep (and adapt)
- **App shell & routing** (`web/app/src/App.tsx`, `MainView.tsx`): keep BrowserRouter + AntD Splitter layout; add project-aware routes and project selector; expose flowchart + kanban views.
- **Flowchart** (`web/flowchart/src/*`, used in `MainView`): keep ReactFlow UX and pending-input badges; switch data to `GET /api/projects/{projectId}/graph`; update types for cells/edges; replace/remove position persistence (no `/positions` in new spec—choose new store or drop save).
- **Input workflows** (`web/app/src/components/InputFormsTab.tsx`, `InputFormRenderer.tsx`, shared `inputActivityService` + context): keep UI and SSE wiring; align schemas and URLs to new `UserInputs` endpoints; ensure IDs include project + cell as required by spec.
- **Shared library** (`web/shared/src/*`): keep; regenerate API client via `web/openapi` from `api/openapi/colony2-api.yaml`; replace custom fetchers with generated client; add project-aware URL state helpers; refresh graph/cell/ticket types.
- **UI stack/tooling** (Ant Design, Vite configs, XYFlow, Monaco where still used): keep.

## What to remove (no matching endpoints in new spec)
- **File browser** (`web/files/src/FileBrowser.tsx`, SidePanel “Files” tab): remove unless a new project-scoped file API appears.
- **Git changes** (`web/changes/src/GitChanges.tsx`, SidePanel “Changes” tab): remove (git endpoints absent).
- **Devcontainer/container editor** (`web/config/src/EnvEditor.tsx` and Config tab content): remove (container endpoints absent).
- **SidePanel placeholders** (Claude/notes/todo stubs in `SidePanel.tsx`): drop or replace with real project/cell metadata editors once fields are known.
- **Position save helpers** in `web/shared/src/api.ts`: remove or replace with new position storage approach.
- **Legacy generated client** under `web/openapi/src/generated`: replace with fresh generation from the new spec.

## New/expanded items to add
- **Project CRUD always available**: add a persistent projects view (list/create/update/delete via `/api/projects`); ensure a project picker is shown first and required before enabling flowchart/kanban. Keep selection in app state + URL so children get `projectId`.
- **Kanban for tickets**: build board using `/api/projects/{projectId}/tickets` + stages/states metadata; surface alongside flowchart once a project is selected.
