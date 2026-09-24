import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { inputActivityService } from './inputActivityService';

// Create mock EventSource instance
const createMockEventSource = () => ({
  close: vi.fn(),
  addEventListener: vi.fn(),
  removeEventListener: vi.fn(),
  dispatchEvent: vi.fn(),
  onopen: null,
  onmessage: null,
  onerror: null,
  readyState: 0,
  url: '',
  withCredentials: false,
  CONNECTING: 0,
  OPEN: 1,
  CLOSED: 2,
});

// Mock EventSource constructor - need to define it as a class-like function
const EventSourceMock = vi.fn(createMockEventSource);
EventSourceMock.CONNECTING = 0;
EventSourceMock.OPEN = 1;
EventSourceMock.CLOSED = 2;
(globalThis as any).EventSource = EventSourceMock;

// Mock fetch
(globalThis as any).fetch = vi.fn();

describe('InputActivityService', () => {
  const projectId = 'proj-1';

  beforeEach(() => {
    vi.clearAllMocks();
    // Reset EventSource mock
    EventSourceMock.mockClear();
    EventSourceMock.mockImplementation(createMockEventSource);
    // Reset the service state
    inputActivityService.disconnect();
    // Reset the cache
    (inputActivityService as any).pendingInputsCache.clear();
    (inputActivityService as any).formDetailsCache.clear();
    // Reset reconnect attempts
    (inputActivityService as any).reconnectAttempts = 0;
  });

  afterEach(() => {
    inputActivityService.disconnect();
  });

  describe('SSE Connection', () => {
    it('refreshes cached requests and details when another tab completes an input', async () => {
      inputActivityService.connect(projectId);
      const completed = vi.fn();
      inputActivityService.on('input_completed', completed);
      try {
        vi.mocked(fetch).mockResolvedValue({ ok: true, json: async () => [{ id: 'job-1' }] } as Response);
        await inputActivityService.getPendingInputs(projectId);
        await inputActivityService.getInputDetails(projectId, 'job-1');

        const source = EventSourceMock.mock.results[0].value;
        const listener = source.addEventListener.mock.calls.find(([name]: [string]) => name === 'input_completed')[1];
        listener({ data: JSON.stringify({ jobId: 'job-1' }) });
        expect(completed).toHaveBeenCalledWith({ jobId: 'job-1' });

        vi.mocked(fetch).mockResolvedValue({ ok: true, json: async () => [] } as Response);
        expect(await inputActivityService.getPendingInputs(projectId)).toEqual([]);
        await inputActivityService.getInputDetails(projectId, 'job-1');
        expect(fetch).toHaveBeenCalledTimes(4);
      } finally {
        inputActivityService.off('input_completed', completed);
      }
    });

    it('fetches a fresh form when the same job requests another input', async () => {
      inputActivityService.connect(projectId);
      vi.mocked(fetch).mockResolvedValue({ ok: true, json: async () => ({ form: { question: 'First?' } }) } as Response);
      await inputActivityService.getInputDetails(projectId, 'job-1');
      const source = EventSourceMock.mock.results[0].value;
      const listener = source.addEventListener.mock.calls.find(([name]: [string]) => name === 'input_pending')[1];
      listener({ data: JSON.stringify({ id: 'job-1', task_ordinal: 9 }) });
      vi.mocked(fetch).mockResolvedValue({ ok: true, json: async () => ({ form: { question: 'Second?' } }) } as Response);
      expect(await inputActivityService.getInputDetails(projectId, 'job-1')).toMatchObject({ form: { question: 'Second?' } });
      expect(fetch).toHaveBeenCalledTimes(2);
    });

    it('should create an EventSource connection when connect is called', () => {
      inputActivityService.connect(projectId);
      expect(EventSourceMock).toHaveBeenCalledWith(
        expect.stringContaining(`/projects/${projectId}/user-inputs/stream`)
      );
    });

    it('should not create duplicate connections', () => {
      // Create a mock that simulates an open connection
      const mockEventSource = {
        close: vi.fn(),
        readyState: 1, // OPEN - this prevents creating a duplicate
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
        onopen: null,
        onmessage: null,
        onerror: null,
        url: '',
        withCredentials: false,
        CONNECTING: 0,
        OPEN: 1,
        CLOSED: 2,
      };
      
      // First call creates the connection
      EventSourceMock.mockImplementationOnce(() => mockEventSource);
      
      inputActivityService.connect(projectId);
      expect(EventSourceMock).toHaveBeenCalledTimes(1);
      
      // Second call should not create a new connection because readyState is OPEN
      inputActivityService.connect(projectId);
      expect(EventSourceMock).toHaveBeenCalledTimes(1);
    });

    it('should close connection when disconnect is called', () => {
      // Create a mock EventSource with a close method we can spy on
      const mockEventSource = {
        close: vi.fn(),
        readyState: 1, // OPEN
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
        onopen: null,
        onmessage: null,
        onerror: null,
        url: '',
        withCredentials: false,
        CONNECTING: 0,
        OPEN: 1,
        CLOSED: 2,
      };
      
      EventSourceMock.mockImplementation(() => mockEventSource);

      inputActivityService.connect(projectId);
      
      // Verify connection was created
      expect(EventSourceMock).toHaveBeenCalled();
      
      inputActivityService.disconnect();
      
      // Verify close was called on the event source
      expect(mockEventSource.close).toHaveBeenCalled();
    });
  });

  describe('getPendingInputs', () => {
    it('should fetch pending inputs for a project', async () => {
      const mockInputs = [
        { id: 'job-1' }
      ];

      ((globalThis as any).fetch as any).mockResolvedValueOnce({
        ok: true,
        json: async () => mockInputs
      });

      const result = await inputActivityService.getPendingInputs(projectId);
      
      expect((globalThis as any).fetch).toHaveBeenCalledWith(
        `http://localhost:8080/api/projects/${projectId}/user-inputs/pending`
      );
      expect(result).toEqual(mockInputs);
    });

    it('should cache results', async () => {
      const mockInputs = [{ id: 'job-1' }];

      ((globalThis as any).fetch as any).mockResolvedValue({
        ok: true,
        json: async () => mockInputs
      });

      const result1 = await inputActivityService.getPendingInputs(projectId);
      const result2 = await inputActivityService.getPendingInputs(projectId);
      
      // Should be called only once due to caching
      expect((globalThis as any).fetch).toHaveBeenCalledTimes(1);
      expect(result1).toEqual(result2);
    });
  });

  describe('getInputDetails', () => {
    it('should fetch input details for a job', async () => {
      const mockDetails = {
        jobId: 'job-1',
        status: 'pending',
        form: {
          title: 'Test Form',
          fields: []
        },
        startTime: '2024-01-01T00:00:00Z',
      };

      ((globalThis as any).fetch as any).mockImplementation((url: string) => {
        if (url.includes(`/projects/${projectId}/user-inputs/job-1`)) {
          return Promise.resolve({
            ok: true,
            json: async () => mockDetails
          });
        }
        return Promise.reject(new Error('Unexpected URL'));
      });

      const result = await inputActivityService.getInputDetails(projectId, 'job-1');
      
      expect((globalThis as any).fetch).toHaveBeenCalledWith(
        `http://localhost:8080/api/projects/${projectId}/user-inputs/job-1`
      );
      expect(result).toEqual(mockDetails);
    });
  });

  describe('submitResponse', () => {
    it('should submit a response for a job', async () => {
      const mockResponse = {
        fields: { field1: 'value1' },
        submitted_at: '2024-01-01T00:00:00Z',
      };

      ((globalThis as any).fetch as any).mockResolvedValueOnce({
        ok: true
      });

      await inputActivityService.submitResponse(projectId, 'job-1', mockResponse);
      
      expect((globalThis as any).fetch).toHaveBeenCalledWith(
        `http://localhost:8080/api/projects/${projectId}/user-inputs/job-1/respond`,
        expect.objectContaining({
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: expect.stringContaining('value1')
        })
      );
    });

    it('should send submitted_at timestamp', async () => {
      const mockResponse = {
        fields: { field1: 'value1' },
        submitted_at: '2024-01-01T00:00:00Z',
      };

      ((globalThis as any).fetch as any).mockResolvedValueOnce({
        ok: true
      });

      await inputActivityService.submitResponse(projectId, 'job-1', mockResponse);
      
      const callArgs = ((globalThis as any).fetch as any).mock.calls[0];
      const body = JSON.parse(callArgs[1].body);
      
      expect(body.submitted_at).toBeDefined();
    });
  });

  describe('cancelInput', () => {
    it('should cancel an input for a job', async () => {
      ((globalThis as any).fetch as any).mockResolvedValueOnce({
        ok: true
      });

      await inputActivityService.cancelInput(projectId, 'job-1');
      
      expect((globalThis as any).fetch).toHaveBeenCalledWith(
        `http://localhost:8080/api/projects/${projectId}/user-inputs/job-1/cancel`,
        expect.objectContaining({
          method: 'POST'
        })
      );
    });
  });

  describe('event emitter', () => {
    it('should register and remove listeners', () => {
      const callback = vi.fn();

      inputActivityService.on('custom-event', callback);
      (inputActivityService as any).emit('custom-event', { id: 'job-1' });
      expect(callback).toHaveBeenCalledWith({ id: 'job-1' });

      inputActivityService.off('custom-event', callback);
      (inputActivityService as any).emit('custom-event', { id: 'job-2' });
      expect(callback).toHaveBeenCalledTimes(1);
    });
  });

  describe('getConnectionState', () => {
    it('should return closed when no connection exists', () => {
      const state = inputActivityService.getConnectionState();
      expect(state).toBe('closed');
    });

    it('should return correct state based on EventSource readyState', () => {
      // Test CONNECTING state
      const mockEventSourceConnecting = {
        close: vi.fn(),
        readyState: 0, // CONNECTING
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
        onopen: null,
        onmessage: null,
        onerror: null,
        url: '',
        withCredentials: false,
        CONNECTING: 0,
        OPEN: 1,
        CLOSED: 2,
      };
      
      EventSourceMock.mockImplementation(() => mockEventSourceConnecting);
      inputActivityService.connect(projectId);
      
      let state = inputActivityService.getConnectionState();
      expect(state).toBe('connecting');
      
      // Clean up for next test
      inputActivityService.disconnect();
      
      // Test OPEN state
      const mockEventSourceOpen = {
        close: vi.fn(),
        readyState: 1, // OPEN
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
        onopen: null,
        onmessage: null,
        onerror: null,
        url: '',
        withCredentials: false,
        CONNECTING: 0,
        OPEN: 1,
        CLOSED: 2,
      };
      
      EventSourceMock.mockImplementation(() => mockEventSourceOpen);
      inputActivityService.connect(projectId);
      
      state = inputActivityService.getConnectionState();
      expect(state).toBe('open');
    });
  });
});
