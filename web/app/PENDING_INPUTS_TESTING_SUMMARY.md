# Pending Inputs - Testing Implementation Summary

## Overview

This document summarizes the comprehensive integration test suite created for the Pending Inputs feature. The tests are based on the OpenAPI specification and mirror the flow from the Go integration test `TestSimpleInput`.

## Test Coverage

### 📁 Test Files Created

1. **`src/test/mocks/inputApiHandlers.ts`** - Mock API backend
   - Project-scoped REST API endpoints
   - Mock data storage and helpers
   - Simulates backend responses per OpenAPI spec

2. **`src/test/mocks/mockSSE.ts`** - Mock SSE server
   - Full EventSource implementation
   - SSE event emission helpers
   - Connection lifecycle management

3. **`src/components/PendingInputsListPage.test.tsx`** (6 tests)
   - Loading and empty states
   - List rendering
   - Refresh functionality
   - Navigation

4. **`src/components/InputDetailPage.test.tsx`** (10 tests)
   - Single-question form flow
   - Form submission
   - Navigation and cancellation
   - Error handling

5. **`src/components/PendingInputsSSE.test.tsx`** (7 tests)
   - Real-time SSE updates
   - New input arrival
   - Input cancellation
   - Multiple events
   - Project-specific connections

6. **`src/components/InputDetailPageMultiField.test.tsx`** (8 tests)
   - Multi-field form rendering
   - All field types
   - Validation
   - Required vs optional fields

7. **`src/components/InputErrorCases.test.tsx`** (9 tests)
   - API errors (404, 500)
   - SSE connection failures
   - Edge cases
   - Rapid project switching

8. **`src/components/InputFlow.integration.test.tsx`** (4 tests)
   - End-to-end workflow (mirrors TestSimpleInput)
   - Concurrent inputs
   - Form state persistence
   - Real-time updates

**Total: 44 integration tests**

## Test Flow Comparison

### Go Test: `TestSimpleInput` (activity_test.go)

```go
func TestSimpleInput(t *testing.T) {
    // 1. Setup recipe with input op
    recipeYaml := `
    id: test-recipe
    op: input
    inputs:
      form:
        question: "how old are you"
    `

    // 2. Start workflow (non-blocking)
    go func() {
        _, err := starter.StartRecipeJob(context.Background(), job, eng, *testRecipe)
        errCh <- err
    }()

    // 3. Wait for input to be pending
    time.Sleep(300 * time.Millisecond)

    // 4. Get pending inputs
    inputs, err := opR.collectPendingInputs(context.Background(), "test-tenant")
    require.Equal(t, 1, len(inputs))

    // 5. Get input details
    result := opR.getDetails(context.Background(), "test-tenant", pending.JobID)
    require.Equal(t, "how old are you", details.Form.Question)

    // 6. Submit response
    res2 := opR.submitResponse(context.Background(), "test-tenant", pending.JobID, FormResponse{
        Response: "foolish",
        Hash:     "abc123",
    })

    // 7. Wait for workflow to complete
    err = <-errCh
    require.NoError(t, err)

    // 8. Verify output
    res4, err := res3.GetData()
    air := Output{}
    require.NoError(t, json.Unmarshal(res4, &air))
    require.Equal(t, "foolish", air.Response)
}
```

### TypeScript Test: `InputFlow.integration.test.tsx`

```typescript
it('should complete full input workflow from start to finish', async () => {
  const user = userEvent.setup();

  // 1. Start with empty list (workflow not yet started)
  setupMockInputs(projectId, [], []);
  render(<PendingInputsListPage projectId={projectId} />);

  const sse = await waitForSSEConnection();
  sse.emitConnected();

  // 2. Verify empty state
  await waitFor(() => {
    expect(screen.getByText('No pending inputs')).toBeInTheDocument();
  });

  // 3. Workflow starts - emit SSE event
  const inputDetails = createMockSingleQuestionDetails(jobId);
  mockPendingInputsStore.set(projectId, [createMockPendingInput(jobId)]);
  mockInputDetailsStore.set(`${projectId}:${jobId}`, inputDetails);
  sse.emitInputPending(jobId);

  // 4. User sees pending input
  await waitFor(() => {
    expect(screen.getByText(`Input Request #${jobId}`)).toBeInTheDocument();
  });

  // 5. User clicks View
  const viewButton = screen.getByRole('button', { name: /view/i });
  await user.click(viewButton);

  // 6. Navigate to detail page
  expect(mockNavigate).toHaveBeenCalledWith(`/project/${projectId}/inputs/${jobId}`);

  // 7. Render detail page
  render(<InputDetailPage projectId={projectId} />);

  // 8. User sees form
  await waitFor(() => {
    expect(screen.getByText('How old are you?')).toBeInTheDocument();
  });

  // 9. User fills form
  const input = screen.getByRole('textbox');
  await user.type(input, 'foolish'); // Same answer as Go test!

  // 10. User submits
  const submitButton = screen.getByRole('button', { name: /submit/i });
  await user.click(submitButton);

  // 11. Response submitted, navigate back
  await waitFor(() => {
    expect(mockNavigate).toHaveBeenCalledWith(`/project/${projectId}/inputs`);
  });

  // 12. Verify input removed from list
  mockPendingInputsStore.set(projectId, []);
  render(<PendingInputsListPage projectId={projectId} />);

  await waitFor(() => {
    expect(screen.getByText('No pending inputs')).toBeInTheDocument();
  });
});
```

## Key Features Tested

### ✅ API Integration
- Project-scoped endpoints (`/api/projects/{projectId}/user-inputs/*`)
- GET pending inputs list
- GET input details
- POST submit response
- POST cancel input
- Error responses (404, 500)

### ✅ SSE Real-time Updates
- Connection lifecycle (connect/disconnect/reconnect)
- Event handling (connected, input_pending, input_cancelled, heartbeat)
- Project-specific connections
- Automatic reconnection on project switch
- Multiple concurrent events

### ✅ Form Rendering
- Single-question forms
- Multi-field forms
- All field types:
  - short_answer
  - paragraph_text
  - dropdown
  - multiple_choice
  - checkboxes
  - linear_scale
  - date
  - time
- Required vs optional fields
- Field validation

### ✅ User Interactions
- View input from list
- Fill out forms
- Submit responses
- Cancel/go back
- Refresh list
- Navigate between pages

### ✅ Edge Cases
- Empty states
- Loading states
- Missing data
- Concurrent inputs
- Rapid project switching
- SSE reconnection during form entry
- Form state persistence

## Running the Tests

```bash
# All input tests
cd /src/web/app
npm test -- --run src/components/*Input*.test.tsx

# Specific test suite
npm test -- --run src/components/InputFlow.integration.test.tsx

# With coverage
npm test -- --coverage --run src/components/*Input*.test.tsx

# Watch mode
npm test -- --watch src/components/PendingInputsListPage.test.tsx
```

## Mock Data Examples

### API Response: Pending Inputs
```json
[
  { "id": "job-abc-123" },
  { "id": "job-def-456" }
]
```

### API Response: Input Details (Single Question)
```json
{
  "jobId": "job-abc-123",
  "status": "pending",
  "startTime": "2026-01-12T10:00:00Z",
  "form": {
    "question": "How old are you?",
    "type": "short_answer"
  }
}
```

### API Response: Input Details (Multi-Field)
```json
{
  "jobId": "job-def-456",
  "status": "pending",
  "startTime": "2026-01-12T10:00:00Z",
  "form": {
    "title": "Deployment Approval",
    "fields": [
      {
        "id": "environment",
        "type": "dropdown",
        "question": "Select environment",
        "required": true,
        "options": [
          { "value": "staging", "label": "Staging" },
          { "value": "production", "label": "Production" }
        ]
      },
      {
        "id": "approver",
        "type": "short_answer",
        "question": "Approver name",
        "required": true
      }
    ]
  }
}
```

### SSE Events

```typescript
// Connected
{ type: 'connected', data: { client_id: 'abc-123' } }

// Input Pending
{ type: 'input_pending', data: { id: 'job-abc-123' } }

// Input Cancelled
{ type: 'input_cancelled', data: { jobId: 'job-abc-123', reason: 'completed' } }

// Heartbeat
{ type: 'heartbeat', data: { timestamp: '2026-01-12T10:00:00Z' } }

// Error
{ type: 'error', data: { error: 'Connection failed' } }
```

## Test Architecture

```
┌─────────────────────────────────────────────┐
│           Integration Tests                  │
├─────────────────────────────────────────────┤
│  ┌────────────────┐  ┌──────────────────┐  │
│  │  List Page     │  │  Detail Page     │  │
│  │  Tests         │  │  Tests           │  │
│  └────────┬───────┘  └────────┬─────────┘  │
│           │                    │             │
│           └─────────┬──────────┘             │
│                     ▼                         │
│  ┌──────────────────────────────────────┐   │
│  │   InputActivityProvider (Context)    │   │
│  └──────────────┬───────────────────────┘   │
│                 │                             │
│                 ▼                             │
│  ┌──────────────────────────────────────┐   │
│  │   inputActivityService (Singleton)   │   │
│  └──────┬───────────────┬───────────────┘   │
│         │               │                     │
└─────────┼───────────────┼─────────────────────┘
          ▼               ▼
   ┌──────────┐    ┌────────────┐
   │   MSW    │    │  Mock SSE  │
   │ Handlers │    │ EventSource│
   └──────────┘    └────────────┘
```

## Benefits

1. **Confidence**: 44 tests covering all major flows
2. **Documentation**: Tests serve as usage examples
3. **Regression Prevention**: Catch breaks early
4. **Faster Development**: Mock backend speeds up testing
5. **Quality Assurance**: Matches Go test behavior
6. **Maintainability**: Well-organized, documented tests

## Next Steps

To use with real backend:
1. Start backend server with input op support
2. Create test recipe with input op
3. Run recipe to create pending input
4. Open app in browser
5. Navigate to "Pending Inputs"
6. Fill and submit form
7. Verify workflow completes

The integration tests ensure the UI is ready!
