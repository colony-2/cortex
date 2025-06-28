# Page snapshot

```yaml
- text: "[plugin:vite:import-analysis] Failed to resolve import \"@graph-visualizer/flowchart\" from \"src/components/MainView.tsx\". Does the file exist? /Users/jnadeau/src/graph/graph-visualizer/web/projects/app/src/components/MainView.tsx:4:26 19 | import { useParams, useNavigate } from \"react-router-dom\"; 20 | import { Splitter } from \"antd\"; 21 | import { GraphFlow } from \"@graph-visualizer/flowchart\"; | ^ 22 | import SidePanel from \"./SidePanel\"; 23 | import { navigateToPath } from \"@graph-visualizer/shared\"; at TransformPluginContext._formatLog (file:///Users/jnadeau/src/graph/graph-visualizer/web/node_modules/vite/dist/node/chunks/dep-DBxKXgDP.js:42499:41) at TransformPluginContext.error (file:///Users/jnadeau/src/graph/graph-visualizer/web/node_modules/vite/dist/node/chunks/dep-DBxKXgDP.js:42496:16) at normalizeUrl (file:///Users/jnadeau/src/graph/graph-visualizer/web/node_modules/vite/dist/node/chunks/dep-DBxKXgDP.js:40475:23) at process.processTicksAndRejections (node:internal/process/task_queues:105:5) at async file:///Users/jnadeau/src/graph/graph-visualizer/web/node_modules/vite/dist/node/chunks/dep-DBxKXgDP.js:40594:37 at async Promise.all (index 6) at async TransformPluginContext.transform (file:///Users/jnadeau/src/graph/graph-visualizer/web/node_modules/vite/dist/node/chunks/dep-DBxKXgDP.js:40521:7) at async EnvironmentPluginContainer.transform (file:///Users/jnadeau/src/graph/graph-visualizer/web/node_modules/vite/dist/node/chunks/dep-DBxKXgDP.js:42294:18) at async loadAndTransform (file:///Users/jnadeau/src/graph/graph-visualizer/web/node_modules/vite/dist/node/chunks/dep-DBxKXgDP.js:35735:27 Click outside, press Esc key, or fix the code to dismiss. You can also disable this overlay by setting"
- code: server.hmr.overlay
- text: to
- code: "false"
- text: in
- code: vite.config.ts
- text: .
```