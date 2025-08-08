# Input Activity UI Integration Specification

## Overview
This specification defines how the input activity system will be integrated into the vibethis UI, allowing users to see and respond to pending input requests from workflow activities. The system uses a consistent tab-based interface that opens directly to the input form when a user clicks on a notification badge.

## Design Philosophy
- **Simple and Consistent**: Single interface pattern for all input scenarios
- **Direct Access**: Clicking badge navigates directly to the input tab with the oldest pending input open
- **Familiar Navigation**: Uses existing left-nav pattern for multiple inputs
- **No Over-optimization**: Straightforward implementation without complex pre-fetching or modal variations

## Architecture Overview

### Key Components
1. **Input Activity Monitor** - SSE connection to receive real-time input requests
2. **Notification Badge** - Visual indicator on nodes showing pending input count
3. **Input Forms Tab** - New tab in box details panel for input forms
4. **Input Form Renderer** - Dynamic form component based on field types
5. **Response Manager** - Handles form submission and validation

## Component Specifications

### 1. Input Activity Monitor Service
Location: `web/shared/src/services/inputActivityService.ts`

```typescript
interface InputActivityService {
  // SSE connection management
  connect(): void
  disconnect(): void
  
  // Subscription management
  subscribe(boxId: string, callback: (event: InputEvent) => void): () => void
  
  // Get current state
  getPendingInputs(boxId?: string): Promise<PendingInput[]>
  getInputDetails(workflowId: string): Promise<InputFormDetails>
  
  // Submit responses
  submitResponse(workflowId: string, response: FormResponse): Promise<void>
  cancelInput(workflowId: string): Promise<void>
}

interface InputEvent {
  type: 'input_requested' | 'input_completed' | 'input_expired'
  workflowId: string
  boxId: string
  timestamp: string
  data?: any
}

interface PendingInput {
  workflowId: string
  boxId: string
  formTitle: string
  createdAt: string
  expiresAt: string
  status: 'pending' | 'completed' | 'expired'
}

interface InputFormDetails extends PendingInput {
  form: InputForm
  context: FormContext
}
```

### 2. Notification Badge Component
Location: `web/flowchart/src/components/InputBadge.tsx`

```typescript
interface InputBadgeProps {
  count: number
  status?: 'pending' | 'urgent' | 'overdue'
  onClick?: () => void
  size?: 'small' | 'default'
  nodeId: string
}
```

Visual Design:
- Position: Top-right corner of the node
- Color scheme:
  - Normal (> 5 min to expiry): Blue (#1890ff)
  - Urgent (< 5 min to expiry): Orange (#fa8c16)
  - Overdue (expired): Red (#f5222d)
- Animation: Subtle pulse for urgent/overdue items
- Display: Show count when > 0, hide when 0
- **Click Behavior**: Navigate to box's Input tab, open oldest pending input

Integration with ProFlowNode:
```tsx
// In ProFlowNode.tsx
<div className="graph-node" style={{ position: 'relative' }}>
  {data.pendingInputCount > 0 && (
    <InputBadge 
      count={data.pendingInputCount}
      status={data.inputUrgency}
      nodeId={id}
      onClick={() => navigateToInputTab(id)}
      size="small"
    />
  )}
  {/* Existing node content */}
</div>
```

### 3. Input Forms Tab
Location: `web/app/src/components/InputFormsTab.tsx`

The Input Forms Tab provides a consistent interface for responding to input requests, using a left navigation pattern for multiple inputs.

```typescript
interface InputFormsTabProps {
  node: DependencyNode
  boxId: string
  initialInputId?: string  // Which input to show initially (defaults to oldest)
}
```

Features:
- Left navigation list showing all pending inputs (sorted by age, oldest first)
- Main content area shows selected input form
- Auto-selects oldest pending input when tab opens
- Real-time updates via SSE
- Completed inputs are removed from the list

UI Layout (using existing left-nav pattern):
```
┌─────────────────────┬────────────────────────────────────┐
│ Pending Inputs      │ Deployment Approval                │
├─────────────────────┤                                    │
│ • Deployment        │ Do you approve this deployment?    │
│   Approval          │                                    │
│   (2 min ago)       │ ○ Approve                         │
│   URGENT            │ ○ Reject                          │
│                     │                                    │
│ • Config Review     │ Additional Notes:                  │
│   (5 min ago)       │ ┌──────────────────────────────┐  │
│                     │ │                              │  │
│ • Resource Request  │ └──────────────────────────────┘  │
│   (12 min ago)      │                                    │
│                     │ Expires in: 3 minutes             │
│                     │                                    │
│ ─────────────────   │ [Cancel]              [Submit]     │
│                     │                                    │
│ Completed (2)       │                                    │
│ • Previous Input    │                                    │
│ • Another Input     │                                    │
└─────────────────────┴────────────────────────────────────┘
```

### 4. Dynamic Form Renderer
Location: `web/shared/src/components/InputFormRenderer.tsx`

```typescript
interface InputFormRendererProps {
  form: InputForm
  context?: FormContext
  onSubmit: (response: FormResponse) => void
  onCancel?: () => void
  loading?: boolean
}
```

Field Type Mappings to Ant Design Components:
- `short_answer` → Input
- `paragraph_text` → TextArea
- `multiple_choice` → Radio.Group
- `checkboxes` → Checkbox.Group
- `dropdown` → Select
- `linear_scale` → Slider
- `date` → DatePicker
- `time` → TimePicker
- `file_upload` → Upload

Validation:
- Required field validation
- Field-specific validation rules
- Real-time validation feedback
- Submission prevention for invalid forms

### 5. Integration with SidePanel

Update SidePanel to include Input Forms tab:
```tsx
// In SidePanel.tsx
const items = (showNodeTabs || selectedNode) ? [
  {
    key: 'files',
    label: (
      <span>
        <FileOutlined />
        Files
      </span>
    ),
    children: <FileBrowser node={selectedNode} boxId={boxId} />,
  },
  {
    key: 'inputs',
    label: (
      <span>
        <FormOutlined />
        Inputs
        {pendingInputCount > 0 && (
          <Badge count={pendingInputCount} style={{ marginLeft: 8 }} />
        )}
      </span>
    ),
    children: <InputFormsTab 
      node={selectedNode} 
      boxId={boxId}
      initialInputId={navigatedFromBadge ? oldestInputId : undefined}
    />,
  },
  // ... existing tabs
]
```

When badge is clicked:
```typescript
const navigateToInputTab = (nodeId: string) => {
  // Navigate to the inputs tab for this node
  const path = navigateToPath({ 
    boxId: nodeId, 
    tab: 'inputs'
  });
  navigate(path);
  // The InputFormsTab will automatically open the oldest pending input
}
```

## User Experience Flow

### Standard Flow (All Cases)
1. **Input Arrives**: Badge appears on node with count
2. **User Clicks Badge**: Navigates to Inputs tab with oldest input open
3. **User Responds**: Fills form and clicks Submit
4. **Completion**: Form clears, next oldest input loads (or tab shows empty state)

### Multiple Inputs
- Left navigation shows all pending inputs sorted by age (oldest first)
- Clicking an input in the left nav loads it in the main area
- Badge count updates as inputs are completed
- Completed inputs move to "Completed" section in left nav

## Data Flow

### 1. Initial Connection
```
App Start → InputActivityService.connect() → SSE /api/user-inputs/stream
```

### 2. Input Request Flow
```
SSE Event → Update State → Update Node Badge → Update Tab Badge
```

### 3. User Response Flow
```
Badge Click → Navigate to Tab → Load Form → User Submit → API Call → Update List → Load Next
```

## State Management

### Redux/Context Structure
```typescript
interface InputActivityState {
  connected: boolean
  pendingInputs: Map<string, PendingInput[]> // Keyed by boxId
  activeWorkflows: Set<string>
  formCache: Map<string, InputFormDetails>
}
```

### Actions
- `CONNECT_SSE`
- `DISCONNECT_SSE`
- `INPUT_REQUESTED`
- `INPUT_COMPLETED`
- `INPUT_EXPIRED`
- `SUBMIT_RESPONSE_START`
- `SUBMIT_RESPONSE_SUCCESS`
- `SUBMIT_RESPONSE_FAILURE`

## API Integration

### Endpoints Used
```typescript
// SSE Stream
GET /api/user-inputs/stream

// List pending inputs
GET /api/user-inputs/pending?box_id={boxId}

// Get input details
GET /api/user-inputs/{workflowId}

// Submit response
POST /api/user-inputs/{workflowId}/respond
Body: {
  fields: { [fieldId]: value },
  metadata: { submittedAt: timestamp }
}

// Cancel input
POST /api/user-inputs/{workflowId}/cancel
```

## Visual Indicators

### Badge States
1. **Normal**: Blue badge with white text
2. **Urgent** (< 5 min): Orange badge with pulse animation
3. **Overdue**: Red badge with stronger pulse

### Form States
1. **Pending**: Normal form display
2. **Submitting**: Loading spinner, disabled inputs
3. **Submitted**: Success message, form hidden
4. **Expired**: Disabled form with expiry message

## Error Handling

### Connection Errors
- Automatic reconnection with exponential backoff
- User notification after 3 failed attempts
- Fallback to polling mode if SSE fails

### Submission Errors
- Display error message inline
- Preserve form data
- Allow retry
- Log errors for debugging

## Performance Considerations

1. **SSE Connection**: Single connection shared across all boxes
2. **Simple Loading**: Load form data when tab opens or input is selected
3. **Caching**: Cache completed form responses for history view
4. **Debouncing**: Debounce rapid state updates

## Accessibility

1. **ARIA Labels**: All form fields properly labeled
2. **Keyboard Navigation**: Full keyboard support
3. **Screen Reader**: Announce urgent inputs
4. **Focus Management**: Proper focus on form open/close
5. **Color Contrast**: WCAG AA compliant

## Testing Requirements

### Unit Tests
- Form validation logic
- State management reducers
- API service methods
- Component rendering

### Integration Tests
- SSE connection handling
- Form submission flow
- Error recovery
- Multi-box scenarios

### E2E Tests (Playwright)
- Complete input flow from notification to submission
- Timeout handling
- Multiple simultaneous inputs
- Connection recovery

## Dependencies

### New NPM Packages
- None required (using existing Ant Design components)

### Existing Dependencies
- @ant-design/icons: For form icons
- antd: Form components
- react-router-dom: Navigation
- @xyflow/react: Node integration
