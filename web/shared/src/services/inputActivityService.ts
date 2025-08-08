const API_BASE = import.meta.env.DEV ? 'http://localhost:8080/api' : '/api';

// Simple EventEmitter implementation for browser
class EventEmitter {
  private events: Map<string, Set<Function>> = new Map();

  on(event: string, listener: Function): void {
    if (!this.events.has(event)) {
      this.events.set(event, new Set());
    }
    this.events.get(event)!.add(listener);
  }

  off(event: string, listener: Function): void {
    this.events.get(event)?.delete(listener);
  }

  emit(event: string, ...args: any[]): void {
    this.events.get(event)?.forEach(listener => {
      listener(...args);
    });
  }
}

export interface InputForm {
  id: string;
  title: string;
  description?: string;
  fields: InputField[];
}

export interface InputField {
  id: string;
  label: string;
  type: 'short_answer' | 'paragraph_text' | 'multiple_choice' | 'checkboxes' | 'dropdown' | 'linear_scale' | 'date' | 'time' | 'file_upload';
  required: boolean;
  options?: string[];
  min?: number;
  max?: number;
  placeholder?: string;
  validation?: {
    pattern?: string;
    message?: string;
  };
}

export interface FormContext {
  workflowName?: string;
  activityName?: string;
  metadata?: Record<string, any>;
}

export interface FormResponse {
  fields: Record<string, any>;
  metadata?: {
    submittedAt: string;
    [key: string]: any;
  };
}

export interface InputEvent {
  type: 'input_requested' | 'input_completed' | 'input_expired';
  workflowId: string;
  cellId: string;
  timestamp: string;
  data?: any;
}

export interface PendingInput {
  workflowId: string;
  cellId: string;
  formTitle: string;
  createdAt: string;
  expiresAt: string;
  status: 'pending' | 'completed' | 'expired';
}

export interface InputFormDetails extends PendingInput {
  form: InputForm;
  context: FormContext;
}

class InputActivityService extends EventEmitter {
  private eventSource: EventSource | null = null;
  private reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 3;
  private reconnectDelay = 1000;
  private pendingInputsCache = new Map<string, PendingInput[]>();
  private formDetailsCache = new Map<string, InputFormDetails>();

  connect(): void {
    if (this.eventSource?.readyState === EventSource.OPEN) {
      return;
    }

    this.disconnect();

    try {
      this.eventSource = new EventSource(`${API_BASE}/user-inputs/stream`);

      this.eventSource.onopen = () => {
        console.log('SSE connection established');
        this.reconnectAttempts = 0;
        this.emit('connected');
      };

      this.eventSource.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data) as InputEvent;
          this.handleInputEvent(data);
        } catch (error) {
          console.error('Failed to parse SSE message:', error);
        }
      };

      this.eventSource.onerror = (error) => {
        console.error('SSE connection error:', error);
        this.handleConnectionError();
      };
    } catch (error) {
      console.error('Failed to create SSE connection:', error);
      this.handleConnectionError();
    }
  }

  disconnect(): void {
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }
    this.emit('disconnected');
  }

  private handleInputEvent(event: InputEvent): void {
    this.emit('input_event', event);
    this.emit(`input_event:${event.cellId}`, event);

    // Update cache based on event type
    if (event.type === 'input_requested') {
      this.invalidatePendingInputsCache(event.cellId);
    } else if (event.type === 'input_completed' || event.type === 'input_expired') {
      this.invalidatePendingInputsCache(event.cellId);
      this.formDetailsCache.delete(event.workflowId);
    }
  }

  private handleConnectionError(): void {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      console.error('Max reconnection attempts reached');
      this.emit('connection_failed');
      return;
    }

    this.reconnectAttempts++;
    const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1);
    
    console.log(`Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})`);
    
    this.reconnectTimeout = setTimeout(() => {
      this.connect();
    }, delay);
  }

  private invalidatePendingInputsCache(cellId: string): void {
    this.pendingInputsCache.delete(cellId);
    // Also clear the "all" cache
    this.pendingInputsCache.delete('all');
  }

  subscribe(cellId: string, callback: (event: InputEvent) => void): () => void {
    const eventName = `input_event:${cellId}`;
    this.on(eventName, callback);
    
    // Return unsubscribe function
    return () => {
      this.off(eventName, callback);
    };
  }

  async getPendingInputs(cellId?: string): Promise<PendingInput[]> {
    const cacheKey = cellId || 'all';
    
    // Check cache first
    if (this.pendingInputsCache.has(cacheKey)) {
      return this.pendingInputsCache.get(cacheKey)!;
    }

    try {
      const url = cellId 
        ? `${API_BASE}/user-inputs/pending?cell_id=${cellId}`
        : `${API_BASE}/user-inputs/pending`;
      
      const response = await fetch(url);
      if (!response.ok) {
        throw new Error(`Failed to fetch pending inputs: ${response.statusText}`);
      }
      
      const inputs = await response.json();
      this.pendingInputsCache.set(cacheKey, inputs);
      return inputs;
    } catch (error) {
      console.error('Failed to fetch pending inputs:', error);
      throw error;
    }
  }

  async getInputDetails(workflowId: string): Promise<InputFormDetails> {
    // Check cache first
    if (this.formDetailsCache.has(workflowId)) {
      return this.formDetailsCache.get(workflowId)!;
    }

    try {
      const response = await fetch(`${API_BASE}/user-inputs/${workflowId}`);
      if (!response.ok) {
        throw new Error(`Failed to fetch input details: ${response.statusText}`);
      }
      
      const details = await response.json();
      this.formDetailsCache.set(workflowId, details);
      return details;
    } catch (error) {
      console.error('Failed to fetch input details:', error);
      throw error;
    }
  }

  async submitResponse(workflowId: string, response: FormResponse): Promise<void> {
    try {
      const apiResponse = await fetch(`${API_BASE}/user-inputs/${workflowId}/respond`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          ...response,
          metadata: {
            ...response.metadata,
            submittedAt: new Date().toISOString(),
          },
        }),
      });

      if (!apiResponse.ok) {
        throw new Error(`Failed to submit response: ${apiResponse.statusText}`);
      }

      // Clear caches for this workflow
      this.formDetailsCache.delete(workflowId);
      // Clear all pending inputs caches as the list has changed
      this.pendingInputsCache.clear();
    } catch (error) {
      console.error('Failed to submit response:', error);
      throw error;
    }
  }

  async cancelInput(workflowId: string): Promise<void> {
    try {
      const response = await fetch(`${API_BASE}/user-inputs/${workflowId}/cancel`, {
        method: 'POST',
      });

      if (!response.ok) {
        throw new Error(`Failed to cancel input: ${response.statusText}`);
      }

      // Clear caches for this workflow
      this.formDetailsCache.delete(workflowId);
      // Clear all pending inputs caches as the list has changed
      this.pendingInputsCache.clear();
    } catch (error) {
      console.error('Failed to cancel input:', error);
      throw error;
    }
  }

  getConnectionState(): 'connecting' | 'open' | 'closed' {
    if (!this.eventSource) return 'closed';
    switch (this.eventSource.readyState) {
      case EventSource.CONNECTING:
        return 'connecting';
      case EventSource.OPEN:
        return 'open';
      default:
        return 'closed';
    }
  }
}

// Create singleton instance
export const inputActivityService = new InputActivityService();