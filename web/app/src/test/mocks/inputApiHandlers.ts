import { http, HttpResponse } from 'msw';
import type { PendingInput, UserInputDetails, InputFormConfig } from '@colony2/shared';

const API_BASE = 'http://localhost:8080/api';

// Mock data storage
export const mockPendingInputsStore = new Map<string, PendingInput[]>();
export const mockInputDetailsStore = new Map<string, UserInputDetails>();

// Helper to create mock data
export function createMockPendingInput(jobId: string): PendingInput {
  return { id: jobId };
}

export function createMockSingleQuestionDetails(jobId: string): UserInputDetails {
  return {
    jobId,
    status: 'pending',
    startTime: new Date().toISOString(),
    task_ordinal: 12,
    form: {
      question: 'How old are you?',
      type: 'short_answer',
    },
  };
}

export function createMockMultiFieldDetails(jobId: string): UserInputDetails {
  return {
    jobId,
    status: 'pending',
    startTime: new Date().toISOString(),
    task_ordinal: 12,
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
            { value: 'production', label: 'Production' },
          ],
        },
        {
          id: 'approver',
          type: 'short_answer',
          question: 'Approver name',
          required: true,
        },
        {
          id: 'notes',
          type: 'paragraph_text',
          question: 'Additional notes',
          required: false,
        },
      ],
    },
  };
}

// Reset mock data
export function resetMockInputStore() {
  mockPendingInputsStore.clear();
  mockInputDetailsStore.clear();
}

// Setup initial mock data for a project
export function setupMockInputs(projectId: string, inputs: PendingInput[], details: UserInputDetails[]) {
  mockPendingInputsStore.set(projectId, inputs);
  details.forEach((detail) => {
    mockInputDetailsStore.set(`${projectId}:${detail.jobId}`, detail);
  });
}

// MSW handlers
export const inputApiHandlers = [
  // GET /api/projects/{projectId}/user-inputs/pending
  http.get(`${API_BASE}/projects/:projectId/user-inputs/pending`, ({ params }) => {
    const { projectId } = params;
    const inputs = mockPendingInputsStore.get(projectId as string) || [];
    return HttpResponse.json(inputs);
  }),

  // GET /api/projects/{projectId}/user-inputs/{jobId}
  http.get(`${API_BASE}/projects/:projectId/user-inputs/:jobId`, ({ params }) => {
    const { projectId, jobId } = params;
    const key = `${projectId}:${jobId}`;
    const details = mockInputDetailsStore.get(key);

    if (!details) {
      return new HttpResponse(null, { status: 404 });
    }

    return HttpResponse.json(details);
  }),

  // POST /api/projects/{projectId}/user-inputs/{jobId}/respond
  http.post(`${API_BASE}/projects/:projectId/user-inputs/:jobId/respond`, async ({ params, request }) => {
    const { projectId, jobId } = params;
    const body = await request.json();

    // Remove from pending inputs
    const inputs = mockPendingInputsStore.get(projectId as string) || [];
    const filtered = inputs.filter((input) => input.id !== jobId);
    mockPendingInputsStore.set(projectId as string, filtered);

    // Remove from details
    const key = `${projectId}:${jobId}`;
    mockInputDetailsStore.delete(key);

    return HttpResponse.json({ ok: true });
  }),

  // POST /api/projects/{projectId}/user-inputs/{jobId}/cancel
  http.post(`${API_BASE}/projects/:projectId/user-inputs/:jobId/cancel`, async ({ params }) => {
    const { projectId, jobId } = params;

    // Remove from pending inputs
    const inputs = mockPendingInputsStore.get(projectId as string) || [];
    const filtered = inputs.filter((input) => input.id !== jobId);
    mockPendingInputsStore.set(projectId as string, filtered);

    // Remove from details
    const key = `${projectId}:${jobId}`;
    mockInputDetailsStore.delete(key);

    return HttpResponse.json({ ok: true });
  }),
];
