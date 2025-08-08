import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { inputActivityService } from './inputActivityService';

// Mock EventSource
(globalThis as any).EventSource = vi.fn(() => ({
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
})) as any;

// Mock fetch
(globalThis as any).fetch = vi.fn();

describe('InputActivityService', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Reset the service state
    inputActivityService.disconnect();
    // Reset the cache
    (inputActivityService as any).pendingInputsCache.clear();
    (inputActivityService as any).formDetailsCache.clear();
  });

  afterEach(() => {
    inputActivityService.disconnect();
  });

  describe('SSE Connection', () => {
    it('should create an EventSource connection when connect is called', () => {
      inputActivityService.connect();
      expect((globalThis as any).EventSource).toHaveBeenCalledWith(expect.stringContaining('/user-inputs/stream'));
    });

    it('should not create duplicate connections', () => {
      inputActivityService.connect();
      inputActivityService.connect();
      expect((globalThis as any).EventSource).toHaveBeenCalledTimes(1);
    });

    it('should close connection when disconnect is called', () => {
      const mockClose = vi.fn();
      (globalThis as any).EventSource = vi.fn(() => ({
        close: mockClose,
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
      })) as any;

      inputActivityService.connect();
      inputActivityService.disconnect();
      
      expect(mockClose).toHaveBeenCalled();
    });
  });

  describe('getPendingInputs', () => {
    it('should fetch pending inputs for a specific cell', async () => {
      const mockInputs = [
        {
          workflowId: 'wf1',
          cellId: 'cell1',
          formTitle: 'Test Form',
          createdAt: '2024-01-01T00:00:00Z',
          expiresAt: '2024-01-01T01:00:00Z',
          status: 'pending'
        }
      ];

      ((globalThis as any).fetch as any).mockResolvedValueOnce({
        ok: true,
        json: async () => mockInputs
      });

      const result = await inputActivityService.getPendingInputs('cell1');
      
      expect((globalThis as any).fetch).toHaveBeenCalledWith(expect.stringContaining('/user-inputs/pending?cell_id=cell1'));
      expect(result).toEqual(mockInputs);
    });

    it('should fetch all pending inputs when no cellId is provided', async () => {
      const mockInputs = [
        {
          workflowId: 'wf1',
          cellId: 'cell1',
          formTitle: 'Test Form 1',
          createdAt: '2024-01-01T00:00:00Z',
          expiresAt: '2024-01-01T01:00:00Z',
          status: 'pending'
        },
        {
          workflowId: 'wf2',
          cellId: 'cell2',
          formTitle: 'Test Form 2',
          createdAt: '2024-01-01T00:00:00Z',
          expiresAt: '2024-01-01T01:00:00Z',
          status: 'pending'
        }
      ];

      ((globalThis as any).fetch as any).mockResolvedValueOnce({
        ok: true,
        json: async () => mockInputs
      });

      const result = await inputActivityService.getPendingInputs();
      
      expect((globalThis as any).fetch).toHaveBeenCalledWith(expect.stringContaining('/user-inputs/pending'));
      expect(result).toEqual(mockInputs);
    });

    it('should cache results', async () => {
      const mockInputs = [
        {
          workflowId: 'wf1',
          cellId: 'cell1',
          formTitle: 'Test Form',
          createdAt: '2024-01-01T00:00:00Z',
          expiresAt: '2024-01-01T01:00:00Z',
          status: 'pending'
        }
      ];

      ((globalThis as any).fetch as any).mockResolvedValue({
        ok: true,
        json: async () => mockInputs
      });

      const result1 = await inputActivityService.getPendingInputs('cell1');
      const result2 = await inputActivityService.getPendingInputs('cell1');
      
      // Should be called only once due to caching
      expect((globalThis as any).fetch).toHaveBeenCalledTimes(1);
      expect(result1).toEqual(result2);
    });
  });

  describe('getInputDetails', () => {
    it('should fetch input details for a workflow', async () => {
      const mockDetails = {
        workflowId: 'wf1',
        cellId: 'cell1',
        formTitle: 'Test Form',
        createdAt: '2024-01-01T00:00:00Z',
        expiresAt: '2024-01-01T01:00:00Z',
        status: 'pending',
        form: {
          id: 'form1',
          title: 'Test Form',
          fields: []
        },
        context: {
          workflowName: 'Test Workflow'
        }
      };

      ((globalThis as any).fetch as any).mockImplementation((url: string) => {
        if (url.includes('/user-inputs/wf1')) {
          return Promise.resolve({
            ok: true,
            json: async () => mockDetails
          });
        }
        return Promise.reject(new Error('Unexpected URL'));
      });

      const result = await inputActivityService.getInputDetails('wf1');
      
      expect((globalThis as any).fetch).toHaveBeenCalledWith(expect.stringContaining('/user-inputs/wf1'));
      expect(result).toEqual(mockDetails);
    });
  });

  describe('submitResponse', () => {
    it('should submit a response for a workflow', async () => {
      const mockResponse = {
        fields: { field1: 'value1' },
        metadata: { test: 'data', submittedAt: '2024-01-01T00:00:00Z' }
      };

      ((globalThis as any).fetch as any).mockResolvedValueOnce({
        ok: true
      });

      await inputActivityService.submitResponse('wf1', mockResponse);
      
      expect((globalThis as any).fetch).toHaveBeenCalledWith(
        expect.stringContaining('/user-inputs/wf1/respond'),
        expect.objectContaining({
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: expect.stringContaining('value1')
        })
      );
    });

    it('should add submittedAt timestamp to metadata', async () => {
      const mockResponse = {
        fields: { field1: 'value1' },
        metadata: { submittedAt: '2024-01-01T00:00:00Z' }
      };

      ((globalThis as any).fetch as any).mockResolvedValueOnce({
        ok: true
      });

      await inputActivityService.submitResponse('wf1', mockResponse);
      
      const callArgs = ((globalThis as any).fetch as any).mock.calls[0];
      const body = JSON.parse(callArgs[1].body);
      
      expect(body.metadata.submittedAt).toBeDefined();
    });
  });

  describe('cancelInput', () => {
    it('should cancel an input for a workflow', async () => {
      ((globalThis as any).fetch as any).mockResolvedValueOnce({
        ok: true
      });

      await inputActivityService.cancelInput('wf1');
      
      expect((globalThis as any).fetch).toHaveBeenCalledWith(
        expect.stringContaining('/user-inputs/wf1/cancel'),
        expect.objectContaining({
          method: 'POST'
        })
      );
    });
  });

  describe('subscribe', () => {
    it('should return an unsubscribe function', () => {
      const callback = vi.fn();
      const unsubscribe = inputActivityService.subscribe('cell1', callback);
      
      expect(typeof unsubscribe).toBe('function');
    });
  });

  describe('getConnectionState', () => {
    it('should return closed when no connection exists', () => {
      const state = inputActivityService.getConnectionState();
      expect(state).toBe('closed');
    });

    it('should return correct state based on EventSource readyState', () => {
      (globalThis as any).EventSource = vi.fn(() => ({
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
      })) as any;

      inputActivityService.connect();
      
      // Mock the internal eventSource to have OPEN state
      const eventSource = (inputActivityService as any).eventSource;
      if (eventSource) {
        eventSource.readyState = 1;
      }
      
      const state = inputActivityService.getConnectionState();
      expect(['connecting', 'open']).toContain(state);
    });
  });
});