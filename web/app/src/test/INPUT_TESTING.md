# Pending Inputs - Integration Testing Guide

This document describes the integration test suite for the Pending Inputs feature.

## Overview

The test suite provides comprehensive coverage of the Pending Inputs feature, including:
- List page functionality
- Detail page and form submission
- Real-time SSE updates
- Multi-field forms
- Error handling
- End-to-end job

## Test Files

### 1. `mocks/inputApiHandlers.ts`
Mock API handlers for MSW (Mock Service Worker) that simulate the backend API.

**Key Features:**
- Mock data storage for pending inputs and details
- Helper functions to create mock data
- REST API handlers matching OpenAPI spec
- Project-scoped endpoints

**Usage:**
```typescript
import { setupMockInputs, createMockPendingInput, createMockSingleQuestionDetails } from './mocks/inputApiHandlers';

// Setup test data
setupMockInputs(
  projectId,
  [createMockPendingInput('job-123')],
  [createMockSingleQuestionDetails('job-123')]
);
```

### 2. `mocks/mockSSE.ts`
Mock EventSource for testing Server-Sent Events (SSE) connections.

**Key Features:**
- Full EventSource API implementation
- Helper methods to emit SSE events
- Support for all event types (connected, input_pending, input_cancelled, heartbeat, error)
- Delay support for simulating timing

**Usage:**
```typescript
import { mockEventSource, waitForSSEConnection } from './mocks/mockSSE';

// Setup mock
const eventSourceMock = mockEventSource();

// Get instance and emit events
const sse = await waitForSSEConnection();
sse.emitConnected();
sse.emitInputPending('job-123');
sse.emitInputCancelled('job-123', 'completed');
```

### 3. `PendingInputsListPage.test.tsx`
Tests for the list page component.

**Coverage:**
- ✅ Loading states
- ✅ Empty state display
- ✅ List rendering with multiple inputs
- ✅ Refresh button functionality
- ✅ View button navigation

### 4. `InputDetailPage.test.tsx`
Tests for the detail page and single-question form submission.

**Coverage:**
- ✅ Loading states
- ✅ Input details display
- ✅ Single-question form rendering
- ✅ Form submission flow
- ✅ Navigation (back/cancel buttons)
- ✅ Error handling (404)

### 5. `PendingInputsSSE.test.tsx`
Tests for real-time SSE functionality.

**Coverage:**
- ✅ New input arrival via SSE
- ✅ Input cancellation via SSE
- ✅ Multiple sequential events
- ✅ Heartbeat handling
- ✅ Project-specific SSE connections
- ✅ Reconnection on project switch

### 6. `InputDetailPageMultiField.test.tsx`
Tests for multi-field forms.

**Coverage:**
- ✅ Multi-field form title
- ✅ All field types rendered
- ✅ Dropdown with options
- ✅ Text input fields
- ✅ Textarea for paragraph text
- ✅ Form submission with all values
- ✅ Required field validation
- ✅ Optional field handling

### 7. `InputErrorCases.test.tsx`
Tests for error scenarios.

**Coverage:**
- ✅ 404 errors when fetching details
- ✅ Network errors fetching pending inputs
- ✅ Submit failure handling
- ✅ SSE connection failures
- ✅ SSE error events
- ✅ Missing parameters
- ✅ Empty form configuration
- ✅ Rapid project switching

### 8. `InputFlow.integration.test.tsx`
End-to-end integration tests simulating complete jobs.

**Coverage:**
- ✅ Complete job from start to finish (mirrors TestSimpleInput)
- ✅ Concurrent inputs from multiple jobs
- ✅ Form state persistence during SSE reconnection
- ✅ Real-time count badge updates

## Running Tests

### Run all input tests
```bash
cd /src/web/app
npm test -- --run src/components/*Input*.test.tsx
```

### Run specific test file
```bash
npm test -- --run src/components/PendingInputsListPage.test.tsx
```

### Run with coverage
```bash
npm test -- --coverage --run src/components/*Input*.test.tsx
```

### Run in watch mode
```bash
npm test -- --watch src/components/InputDetailPage.test.tsx
```

## Test Flow Comparison

### Go Test (TestSimpleInput)
```
1. Start job with input op
2. Wait 300ms for activity to start
3. collectPendingInputs() -> verify 1 input
4. getDetails() -> verify form.Question
5. submitResponse() with answer
6. Wait for job completion
7. Verify job output contains answer
```

### TypeScript Integration Test (InputFlow.integration.test)
```
1. Start with empty list
2. Emit SSE event for new input (simulates job start)
3. Verify input appears in list
4. Click View button
5. Navigate to detail page
6. Verify form renders with question
7. Fill out form
8. Submit response
9. Verify navigation back to list
10. Verify input removed from list
```

## Mock Data Examples

### Single Question Input
```typescript
const details = {
  jobId: 'job-123',
  status: 'pending',
  startTime: '2026-01-12T10:00:00Z',
  form: {
    question: 'How old are you?',
    type: 'short_answer',
  }
};
```

### Multi-Field Input
```typescript
const details = {
  jobId: 'job-456',
  status: 'pending',
  startTime: '2026-01-12T10:00:00Z',
  form: {
    title: 'Deployment Approval',
    fields: [
      {
        id: 'environment',
        type: 'dropdown',
        question: 'Select environment',
        required: true,
        options: [
          { value: 'staging', label: 'Staging' },
          { value: 'production', label: 'Production' }
        ]
      },
      {
        id: 'approver',
        type: 'short_answer',
        question: 'Approver name',
        required: true
      }
    ]
  }
};
```

## SSE Event Examples

### Connected Event
```typescript
sse.emitConnected('client-123');
// Emits: { type: 'connected', data: { client_id: 'client-123' } }
```

### Input Pending Event
```typescript
sse.emitInputPending('job-123');
// Emits: { type: 'input_pending', data: { id: 'job-123' } }
```

### Input Cancelled Event
```typescript
sse.emitInputCancelled('job-123', 'User cancelled');
// Emits: { type: 'input_cancelled', data: { jobId: 'job-123', reason: 'User cancelled' } }
```

### Heartbeat Event
```typescript
sse.emitHeartbeat();
// Emits: { type: 'heartbeat', data: { timestamp: '2026-01-12T10:00:00Z' } }
```

## Testing Best Practices

### 1. Always Clean Up
```typescript
beforeEach(() => {
  server.listen({ onUnhandledRequest: 'error' });
  resetMockInputStore();
  eventSourceMock = mockEventSource();
});

afterEach(() => {
  server.close();
  eventSourceMock.restore();
  vi.clearAllMocks();
});
```

### 2. Wait for SSE Connection
```typescript
const sse = await waitForSSEConnection();
sse.emitConnected();

await waitFor(() => {
  expect(screen.getByText('No pending inputs')).toBeInTheDocument();
});
```

### 3. Use User Events
```typescript
const user = userEvent.setup();
await user.type(input, 'answer');
await user.click(submitButton);
```

### 4. Test Real-Time Updates
```typescript
// Update mock store
mockPendingInputsStore.set(projectId, [newInput]);

// Emit SSE event
sse.emitInputPending(newJobId);

// Verify UI updates
await waitFor(() => {
  expect(screen.getByText(`Input Request #${newJobId}`)).toBeInTheDocument();
});
```

## Troubleshooting

### SSE Connection Timeout
If you see "SSE connection timeout" errors:
- Ensure `mockEventSource()` is called before rendering
- Check that the component is wrapped in `InputActivityProvider`
- Verify `waitForSSEConnection()` is awaited

### Mock Data Not Found
If tests fail with 404 errors:
- Ensure `setupMockInputs()` is called with correct project ID
- Verify mock stores are reset in `beforeEach`
- Check that job IDs match between pending inputs and details

### Navigation Not Working
If navigation assertions fail:
- Ensure `vi.mock('react-router-dom')` is set up
- Check that `mockNavigate.mockClear()` is called in `beforeEach`
- Verify the component is wrapped in `<BrowserRouter>`

## Future Improvements

- [ ] Add visual regression tests
- [ ] Add accessibility (a11y) tests
- [ ] Add performance tests for large input lists
- [ ] Add tests for file upload fields
- [ ] Add tests for form validation edge cases
- [ ] Add tests for network retry logic
- [ ] Add tests for SSE reconnection with exponential backoff
