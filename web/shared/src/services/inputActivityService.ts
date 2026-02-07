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

// Types matching OpenAPI spec
export type FieldType =
  | 'short_answer'
  | 'paragraph_text'
  | 'multiple_choice'
  | 'checkboxes'
  | 'dropdown'
  | 'linear_scale'
  | 'multiple_choice_grid'
  | 'checkbox_grid'
  | 'date'
  | 'time'
  | 'file_upload';

export interface Option {
  value: string;
  label?: string;
}

export interface LinearScale {
  min: number;
  max: number;
  min_label?: string;
  max_label?: string;
}

export interface FieldValidation {
  min_length?: number;
  max_length?: number;
  pattern?: string;
  min?: number;
  max?: number;
}

export interface FormField {
  id: string;
  type: FieldType;
  question: string;
  required?: boolean;
  placeholder?: string;
  options?: Option[];
  scale?: LinearScale;
  validation?: FieldValidation;
}

export interface Artifact {
  path: string;
}

export interface GlobPattern {
  pattern: string;
}

export interface FormContext {
  artifacts?: Artifact[];
  artifacts_from_output?: string;
  artifacts_glob?: GlobPattern[];
}

export interface InputFormConfig {
  // Single question format
  question?: string;
  type?: FieldType;
  options?: Option[];
  scale?: LinearScale;

  // Multi-field format
  title?: string;
  fields?: FormField[];
  context?: FormContext;
  timeout?: number;
  default_on_timeout?: any;
}

// Component-friendly form structure (for InputFormRenderer)
export interface InputForm {
  id: string;
  title: string;
  description?: string;
  fields: InputField[];
}

export interface InputField {
  id: string;
  label: string;
  type: FieldType;
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

export interface FormResponse {
  fields: Record<string, any>;
  activity_id?: string;
  user_id?: string;
  response?: any;
  submitted_at?: string;
  time_to_complete?: number;
  hash?: string;
}

// API response types
export interface PendingInput {
  id: string;  // jobId
}

export interface UserInputDetails {
  jobId: string;
  status: string;
  startTime: string;
  form: InputFormConfig;
  hash?: string;
  task_ordinal?: number;
  taskOrdinal?: number;
}

// SSE event types based on OpenAPI InputSSEEvent
export type InputSSEEventType =
  | 'connected'
  | 'input_pending'
  | 'input_cancelled'
  | 'heartbeat'
  | 'error';

export interface InputSSEEvent {
  type: InputSSEEventType;
  data: any;
}

class InputActivityService extends EventEmitter {
  private eventSource: EventSource | null = null;
  private reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 3;
  private reconnectDelay = 1000;
  private currentProjectId: string | null = null;
  private pendingInputsCache = new Map<string, PendingInput[]>();
  private formDetailsCache = new Map<string, UserInputDetails>();

  connect(projectId: string): void {
    // If already connected to this project, do nothing
    if (this.eventSource?.readyState === 1 && this.currentProjectId === projectId) {
      return;
    }

    // Disconnect from previous project if any
    this.disconnect();

    this.currentProjectId = projectId;

    // Check if EventSource is available (browser environment)
    if (typeof EventSource === 'undefined') {
      console.warn('EventSource not available, SSE connection disabled');
      return;
    }

    // Attempt SSE connection
    this.createEventSource();
  }

  private createEventSource(): void {
    if (!this.currentProjectId) {
      console.warn('No project ID set, cannot create SSE connection');
      return;
    }

    try {
      this.eventSource = new EventSource(
        `${API_BASE}/projects/${this.currentProjectId}/user-inputs/stream`
      );

      this.eventSource.onopen = () => {
        console.log('SSE connection established for project:', this.currentProjectId);
        this.reconnectAttempts = 0;
        this.emit('connected');
      };

      this.eventSource.addEventListener('connected', (event) => {
        try {
          const data = JSON.parse((event as MessageEvent).data);
          console.log('SSE connected, client_id:', data.client_id);
        } catch (error) {
          console.error('Failed to parse connected event:', error);
        }
      });

      this.eventSource.addEventListener('input_pending', (event) => {
        try {
          const data = JSON.parse((event as MessageEvent).data);
          this.handleInputPendingEvent(data);
        } catch (error) {
          console.error('Failed to parse input_pending event:', error);
        }
      });

      this.eventSource.addEventListener('input_cancelled', (event) => {
        try {
          const data = JSON.parse((event as MessageEvent).data);
          this.handleInputCancelledEvent(data);
        } catch (error) {
          console.error('Failed to parse input_cancelled event:', error);
        }
      });

      this.eventSource.addEventListener('heartbeat', (event) => {
        // Heartbeat - can be used for connection monitoring
      });

      this.eventSource.addEventListener('error', (event) => {
        try {
          const data = JSON.parse((event as MessageEvent).data);
          console.error('SSE error event:', data.error);
        } catch (error) {
          // Ignore parsing errors for error events
        }
      });

      this.eventSource.onerror = () => {
        // Check if the connection was immediately closed (likely 404)
        if (this.eventSource?.readyState === 2 && this.reconnectAttempts === 0) {
          // Connection closed immediately, likely endpoint doesn't exist
          // Silently disable without logging error
          this.disconnect();
          return;
        }

        // For subsequent errors, handle reconnection
        if (this.eventSource?.readyState === 2) {
          this.handleConnectionError();
        }
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
    this.currentProjectId = null;
    this.emit('disconnected');
  }

  private handleInputPendingEvent(data: { id: string }): void {
    // Invalidate cache BEFORE emitting event so listeners get fresh data
    if (this.currentProjectId) {
      this.invalidatePendingInputsCache();
    }
    this.emit('input_pending', data);
  }

  private handleInputCancelledEvent(data: { jobId: string; reason?: string }): void {
    // Invalidate cache BEFORE emitting event so listeners get fresh data
    if (this.currentProjectId) {
      this.invalidatePendingInputsCache();
      this.formDetailsCache.delete(data.jobId);
    }
    this.emit('input_cancelled', data);
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
      this.createEventSource();
    }, delay);
  }

  private invalidatePendingInputsCache(): void {
    if (this.currentProjectId) {
      this.pendingInputsCache.delete(this.currentProjectId);
    }
  }

  async getPendingInputs(projectId: string): Promise<PendingInput[]> {
    // Check cache first
    if (this.pendingInputsCache.has(projectId)) {
      return this.pendingInputsCache.get(projectId)!;
    }

    try {
      const url = `${API_BASE}/projects/${projectId}/user-inputs/pending`;

      const response = await fetch(url);
      if (!response.ok) {
        throw new Error(`Failed to fetch pending inputs: ${response.statusText}`);
      }

      const inputs = await response.json();
      this.pendingInputsCache.set(projectId, inputs);
      return inputs;
    } catch (error) {
      console.error('Failed to fetch pending inputs:', error);
      throw error;
    }
  }

  async getInputDetails(projectId: string, jobId: string): Promise<UserInputDetails> {
    const cacheKey = `${projectId}:${jobId}`;

    // Check cache first
    if (this.formDetailsCache.has(cacheKey)) {
      return this.formDetailsCache.get(cacheKey)!;
    }

    try {
      const response = await fetch(`${API_BASE}/projects/${projectId}/user-inputs/${jobId}`);
      if (!response.ok) {
        throw new Error(`Failed to fetch input details: ${response.statusText}`);
      }

      const details = await response.json();
      this.formDetailsCache.set(cacheKey, details);
      return details;
    } catch (error) {
      console.error('Failed to fetch input details:', error);
      throw error;
    }
  }

  async submitResponse(projectId: string, jobId: string, response: FormResponse): Promise<void> {
    try {
      const apiResponse = await fetch(`${API_BASE}/projects/${projectId}/user-inputs/${jobId}/respond`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(response),
      });

      if (!apiResponse.ok) {
        throw new Error(`Failed to submit response: ${apiResponse.statusText}`);
      }

      // Clear caches
      const cacheKey = `${projectId}:${jobId}`;
      this.formDetailsCache.delete(cacheKey);
      this.pendingInputsCache.delete(projectId);
    } catch (error) {
      console.error('Failed to submit response:', error);
      throw error;
    }
  }

  async cancelInput(projectId: string, jobId: string, reason?: string): Promise<void> {
    try {
      const response = await fetch(`${API_BASE}/projects/${projectId}/user-inputs/${jobId}/cancel`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ reason: reason || 'Cancelled by user' }),
      });

      if (!response.ok) {
        throw new Error(`Failed to cancel input: ${response.statusText}`);
      }

      // Clear caches
      const cacheKey = `${projectId}:${jobId}`;
      this.formDetailsCache.delete(cacheKey);
      this.pendingInputsCache.delete(projectId);
    } catch (error) {
      console.error('Failed to cancel input:', error);
      throw error;
    }
  }

  getConnectionState(): 'connecting' | 'open' | 'closed' {
    if (!this.eventSource) return 'closed';
    switch (this.eventSource.readyState) {
      case 0: // CONNECTING
        return 'connecting';
      case 1: // OPEN
        return 'open';
      default:
        return 'closed';
    }
  }

  getCurrentProjectId(): string | null {
    return this.currentProjectId;
  }

  // Test utility: clear all caches
  clearCaches(): void {
    this.pendingInputsCache.clear();
    this.formDetailsCache.clear();
  }
}

// Create singleton instance
export const inputActivityService = new InputActivityService();
