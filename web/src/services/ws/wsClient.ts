// WebSocket client with auto-reconnect, heartbeat, and typed event handling.
// Uses pure browser WebSocket API with no external dependencies.

// Connection lifecycle states
export type ConnectionState = 'connecting' | 'connected' | 'disconnected' | 'reconnecting';

// Reconnection configuration options
export interface WSClientOptions {
  reconnect: boolean;
  initialDelay: number;
  maxDelay: number;
  maxRetries: number;
  backoffMultiplier: number;
  pingInterval: number;
  pongTimeout: number;
}

const DEFAULT_OPTIONS: WSClientOptions = {
  reconnect: true,
  initialDelay: 1000,
  maxDelay: 30000,
  maxRetries: 10,
  backoffMultiplier: 2,
  pingInterval: 30000,
  pongTimeout: 10000,
};

type MessageHandler = (data: unknown) => void;
type StateChangeHandler = (state: ConnectionState) => void;

export class WSClient {
  private readonly url: string;
  private readonly options: WSClientOptions;
  private ws: WebSocket | null = null;
  private state: ConnectionState = 'disconnected';
  private retryCount = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private pingTimer: ReturnType<typeof setInterval> | null = null;
  private pongTimer: ReturnType<typeof setTimeout> | null = null;
  private handlers = new Map<string, Set<MessageHandler>>();
  private stateHandlers = new Set<StateChangeHandler>();
  private disposed = false;

  constructor(url: string, options: Partial<WSClientOptions> = {}) {
    this.url = url;
    this.options = { ...DEFAULT_OPTIONS, ...options };
  }

  // Establish WebSocket connection
  connect(): void {
    if (this.disposed) return;
    if (this.ws?.readyState === WebSocket.OPEN) return;

    this.setState('connecting');
    this.ws = new WebSocket(this.url);

    this.ws.onopen = () => {
      this.retryCount = 0;
      this.setState('connected');
      this.startHeartbeat();
    };

    this.ws.onmessage = (event: MessageEvent) => {
      this.handleMessage(event);
    };

    this.ws.onclose = () => {
      this.stopHeartbeat();
      if (!this.disposed && this.options.reconnect) {
        this.scheduleReconnect();
      } else {
        this.setState('disconnected');
      }
    };

    this.ws.onerror = () => {
      // Error details are intentionally limited by the browser for security.
      // The onclose handler will fire next and manage reconnection.
    };
  }

  // Cleanly close the connection
  disconnect(): void {
    this.disposed = true;
    this.clearReconnectTimer();
    this.stopHeartbeat();

    if (this.ws) {
      this.ws.onopen = null;
      this.ws.onmessage = null;
      this.ws.onclose = null;
      this.ws.onerror = null;

      if (this.ws.readyState === WebSocket.OPEN || this.ws.readyState === WebSocket.CONNECTING) {
        this.ws.close(1000, 'client disconnect');
      }
      this.ws = null;
    }

    this.setState('disconnected');
  }

  // Send a typed JSON message
  send(type: string, payload: unknown): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      throw new Error(`Cannot send message: WebSocket is ${this.state}`);
    }

    const message = JSON.stringify({ type, payload, timestamp: new Date().toISOString() });
    this.ws.send(message);
  }

  // Register a handler for a specific message type
  on(type: string, handler: MessageHandler): void {
    if (!this.handlers.has(type)) {
      this.handlers.set(type, new Set());
    }
    this.handlers.get(type)!.add(handler);
  }

  // Unregister a handler. If no handler specified, removes all for that type.
  off(type: string, handler?: MessageHandler): void {
    if (!handler) {
      this.handlers.delete(type);
      return;
    }

    const set = this.handlers.get(type);
    if (set) {
      set.delete(handler);
      if (set.size === 0) {
        this.handlers.delete(type);
      }
    }
  }

  // Subscribe to connection state changes
  onStateChange(handler: StateChangeHandler): void {
    this.stateHandlers.add(handler);
  }

  // Get the current connection state
  getState(): ConnectionState {
    return this.state;
  }

  // -- Private methods --

  private setState(next: ConnectionState): void {
    if (this.state === next) return;
    this.state = next;
    for (const handler of this.stateHandlers) {
      handler(next);
    }
  }

  private handleMessage(event: MessageEvent): void {
    // Respond to server pong frames
    if (event.data === 'pong') {
      this.clearPongTimer();
      return;
    }

    let parsed: { type?: string; payload?: unknown };
    try {
      parsed = JSON.parse(event.data as string);
    } catch {
      return; // Ignore malformed messages
    }

    const { type, payload } = parsed;
    if (!type) return;

    const set = this.handlers.get(type);
    if (set) {
      for (const handler of set) {
        handler(payload);
      }
    }
  }

  private scheduleReconnect(): void {
    if (this.retryCount >= this.options.maxRetries) {
      this.setState('disconnected');
      return;
    }

    this.setState('reconnecting');
    const delay = Math.min(
      this.options.initialDelay * Math.pow(this.options.backoffMultiplier, this.retryCount),
      this.options.maxDelay,
    );
    this.retryCount++;

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, delay);
  }

  private clearReconnectTimer(): void {
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }

  private startHeartbeat(): void {
    this.stopHeartbeat();
    this.pingTimer = setInterval(() => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        this.ws.send('ping');
        this.pongTimer = setTimeout(() => {
          // Pong not received within timeout -- force reconnect
          this.ws?.close(4000, 'pong timeout');
        }, this.options.pongTimeout);
      }
    }, this.options.pingInterval);
  }

  private stopHeartbeat(): void {
    if (this.pingTimer !== null) {
      clearInterval(this.pingTimer);
      this.pingTimer = null;
    }
    this.clearPongTimer();
  }

  private clearPongTimer(): void {
    if (this.pongTimer !== null) {
      clearTimeout(this.pongTimer);
      this.pongTimer = null;
    }
  }
}

// Factory function that creates a pre-configured WSClient instance.
// Default URL is derived from the current page location.
export function createWSClient(url?: string): WSClient {
  const resolvedUrl =
    url ?? `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`;
  return new WSClient(resolvedUrl);
}
