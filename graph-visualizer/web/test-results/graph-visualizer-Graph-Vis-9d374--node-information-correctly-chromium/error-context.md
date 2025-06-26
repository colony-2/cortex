# Page snapshot

```yaml
- application:
  - img
  - button "Zoom In":
    - img
  - button "Zoom Out":
    - img
  - button "Fit View":
    - img
  - button "Toggle Interactivity":
    - img
  - img "Mini Map"
  - link "React Flow attribution":
    - /url: https://reactflow.dev
    - text: React Flow
- separator
- tablist:
  - tab "setting Configuration" [selected]:
    - img "setting"
    - text: Configuration
- tabpanel "setting Configuration":
  - tablist:
    - tab "container Container" [selected]:
      - img "container"
      - text: Container
    - tab "edit Notes":
      - img "edit"
      - text: Notes
    - tab "check-square Todo List":
      - img "check-square"
      - text: Todo List
  - tabpanel "container Container":
    - heading "Container Configuration" [level=5]
    - img "No data"
    - text: Container settings coming soon
```