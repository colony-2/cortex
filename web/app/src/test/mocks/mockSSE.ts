/**
 * Mock EventSource for testing SSE connections
 * Based on the OpenAPI InputSSEEvent specification
 */

type SSEEventType = 'connected' | 'input_pending' | 'input_cancelled' | 'heartbeat' | 'error';

interface MockEventSourceConfig {
  url: string;
  onOpen?: () => void;
  events?: Array<{
    type: SSEEventType;
    data: any;
    delay?: number;
  }>;
  autoConnect?: boolean;
}

export class MockEventSource implements EventSource {
  readonly CONNECTING = 0 as const;
  readonly OPEN = 1 as const;
  readonly CLOSED = 2 as const;

  url: string;
  readyState: number = 0;
  withCredentials = false;

  onopen: ((this: EventSource, ev: Event) => any) | null = null;
  onmessage: ((this: EventSource, ev: MessageEvent) => any) | null = null;
  onerror: ((this: EventSource, ev: Event) => any) | null = null;

  private listeners = new Map<string, Set<EventListener>>();
  private eventQueue: MockEventSourceConfig['events'] = [];
  private timeouts: NodeJS.Timeout[] = [];

  constructor(url: string, config?: EventSourceInit) {
    this.url = url;
    this.withCredentials = config?.withCredentials || false;

    // Auto-connect after a tick
    setTimeout(() => {
      this.readyState = this.OPEN;
      if (this.onopen) {
        this.onopen(new Event('open'));
      }
    }, 0);
  }

  addEventListener(type: string, listener: EventListener): void {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, new Set());
    }
    this.listeners.get(type)!.add(listener);
  }

  removeEventListener(type: string, listener: EventListener): void {
    this.listeners.get(type)?.delete(listener);
  }

  dispatchEvent(event: Event): boolean {
    const listeners = this.listeners.get(event.type);
    if (listeners) {
      listeners.forEach((listener) => listener(event));
    }

    // Also call onmessage for message events
    if (event.type === 'message' && this.onmessage) {
      this.onmessage(event as MessageEvent);
    }

    return true;
  }

  close(): void {
    this.readyState = this.CLOSED;
    this.timeouts.forEach(clearTimeout);
    this.timeouts = [];
  }

  // Test utilities
  emitConnected(clientId = 'test-client-123') {
    const event = new MessageEvent('connected', {
      data: JSON.stringify({ client_id: clientId }),
    });
    this.dispatchEvent(event);
  }

  emitInputPending(jobId: string, delay = 0) {
    const emitFn = () => {
      const event = new MessageEvent('input_pending', {
        data: JSON.stringify({ id: jobId }),
      });
      this.dispatchEvent(event);
    };

    if (delay > 0) {
      const timeout = setTimeout(emitFn, delay);
      this.timeouts.push(timeout);
    } else {
      emitFn();
    }
  }

  emitInputCancelled(jobId: string, reason?: string, delay = 0) {
    const emitFn = () => {
      const event = new MessageEvent('input_cancelled', {
        data: JSON.stringify({ jobId, reason }),
      });
      this.dispatchEvent(event);
    };

    if (delay > 0) {
      const timeout = setTimeout(emitFn, delay);
      this.timeouts.push(timeout);
    } else {
      emitFn();
    }
  }

  emitHeartbeat(delay = 0) {
    const emitFn = () => {
      const event = new MessageEvent('heartbeat', {
        data: JSON.stringify({ timestamp: new Date().toISOString() }),
      });
      this.dispatchEvent(event);
    };

    if (delay > 0) {
      const timeout = setTimeout(emitFn, delay);
      this.timeouts.push(timeout);
    } else {
      emitFn();
    }
  }

  emitError(errorMessage: string, delay = 0) {
    const emitFn = () => {
      const event = new MessageEvent('error', {
        data: JSON.stringify({ error: errorMessage }),
      });
      this.dispatchEvent(event);
    };

    if (delay > 0) {
      const timeout = setTimeout(emitFn, delay);
      this.timeouts.push(timeout);
    } else {
      emitFn();
    }
  }
}

// Global mock for EventSource
let mockEventSourceInstance: MockEventSource | null = null;

export function mockEventSource() {
  const OriginalEventSource = global.EventSource;

  // @ts-ignore
  global.EventSource = class extends MockEventSource {
    constructor(url: string, config?: EventSourceInit) {
      super(url, config);
      mockEventSourceInstance = this;
    }
  };

  return {
    restore: () => {
      global.EventSource = OriginalEventSource;
      mockEventSourceInstance = null;
    },
    getInstance: () => mockEventSourceInstance,
  };
}

// Helper to wait for SSE connection
export async function waitForSSEConnection(timeout = 1000): Promise<MockEventSource> {
  const start = Date.now();
  while (!mockEventSourceInstance || mockEventSourceInstance.readyState !== 1) {
    if (Date.now() - start > timeout) {
      throw new Error('SSE connection timeout');
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  return mockEventSourceInstance;
}
