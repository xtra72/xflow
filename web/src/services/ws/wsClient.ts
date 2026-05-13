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
  pongTimeout: 30000,
};

type MessageHandler = (data: unknown) => void;
type StateChangeHandler = (state: ConnectionState) => void;

// SPEC-AUTH-003: connect 시점에 매번 호출되는 토큰 조회 콜백.
// undefined/'' 반환 시 query 파라미터를 추가하지 않는다.
export type TokenGetter = () => string | undefined;

export class WSClient {
  // SPEC-AUTH-003 U1: base URL 은 connect 시점에 평가되므로 readonly 가 아닌 일반 필드.
  private readonly baseUrl: string;
  private readonly tokenGetter: TokenGetter | undefined;
  private readonly options: WSClientOptions;
  private ws: WebSocket | null = null;
  private state: ConnectionState = 'disconnected';
  private retryCount = 0;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private pingTimer: ReturnType<typeof setInterval> | null = null;
  private pongTimer: ReturnType<typeof setTimeout> | null = null;
  private handlers = new Map<string, Set<MessageHandler>>();
  private stateHandlers = new Set<StateChangeHandler>();
  // SPEC-AUTH-003: disconnect() 호출 시 일시적 종료를 표시. reconnect 가 재활성화 시 false 로 복귀.
  private disposed = false;
  private visibilityHandler: (() => void) | null = null;

  // SPEC-AUTH-003 UB1: 인증 실패 휴리스틱 판정에 사용하는 내부 상태.
  // 직전 connect 시도가 토큰을 동반했는지 — `?token=` 가 URL 에 포함되었는지로 판정.
  private lastConnectAttemptHadToken = false;
  // 직전 connect 시도 동안 onopen 이 한 번이라도 발생했는지 — 즉시 1006 종료인지 식별.
  private connectionEverEstablished = false;
  // 직전 종료가 인증 실패로 판정되었는지 — 외부에서 getLastFailureWasAuth() 로 조회 가능.
  private lastFailureWasAuth = false;

  constructor(baseUrl: string, tokenGetter?: TokenGetter, options: Partial<WSClientOptions> = {}) {
    this.baseUrl = baseUrl;
    this.tokenGetter = tokenGetter;
    this.options = { ...DEFAULT_OPTIONS, ...options };
  }

  // SPEC-AUTH-003 U1: connect 시점에 base URL + 최신 토큰으로 final URL 을 합성한다.
  // 기존 query 가 있으면 보존하며 ?token= 또는 &token= 형태로 토큰을 추가한다.
  private buildConnectUrl(): { url: string; hasToken: boolean } {
    const token = this.tokenGetter?.();
    if (!token) {
      return { url: this.baseUrl, hasToken: false };
    }
    const separator = this.baseUrl.includes('?') ? '&' : '?';
    return { url: `${this.baseUrl}${separator}token=${encodeURIComponent(token)}`, hasToken: true };
  }

  // Establish WebSocket connection
  connect(): void {
    // SPEC-AUTH-003: 외부 명령(WebSocketProvider gate)으로 connect 가 재호출될 수 있으므로
    // disposed 플래그를 false 로 복귀시켜 이후 onclose 흐름에서 reconnect 가 정상 동작하도록 한다.
    this.disposed = false;
    if (this.ws?.readyState === WebSocket.OPEN) return;

    // SPEC-AUTH-003 U1/UB1: 매 connect 시도마다 URL 재평가 및 토큰 동반 여부 기록.
    const { url, hasToken } = this.buildConnectUrl();
    this.lastConnectAttemptHadToken = hasToken;
    this.connectionEverEstablished = false;

    this.setState('connecting');
    this.ws = new WebSocket(url);

    this.ws.onopen = () => {
      this.retryCount = 0;
      // SPEC-AUTH-003 UB1: onopen 발생 시점에 인증 통과로 간주.
      this.connectionEverEstablished = true;
      this.lastFailureWasAuth = false;
      this.setState('connected');
      this.startHeartbeat();
    };

    this.ws.onmessage = (event: MessageEvent) => {
      this.handleMessage(event);
    };

    this.ws.onclose = (event: { code?: number; reason?: string; wasClean?: boolean }) => {
      this.stopHeartbeat();
      // SPEC-AUTH-003 UB1: 인증 실패로 추정되는 close 식별.
      // 조건: code === 1006, wasClean === false, onopen 미발생 (connectionEverEstablished === false).
      // - lastConnectAttemptHadToken === true  → 토큰 보냈는데 거절 (위조/만료/blacklist)
      // - lastConnectAttemptHadToken === false → basic_auth: true 서버에 토큰 없이 시도
      // 두 경우 모두 동일 자격증명으로 재시도해도 동일 결과이므로 reconnect 차단.
      const isAuthFailure =
        event.code === 1006 && event.wasClean === false && !this.connectionEverEstablished;

      if (isAuthFailure) {
        this.lastFailureWasAuth = true;
        this.setState('disconnected');
        return;
      }

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

  // SPEC-AUTH-003 UB1: 직전 종료가 인증 실패였는지 외부에서 조회.
  // WebSocketProvider 가 토큰 회전 시 자동 reset 여부 결정에 사용한다.
  getLastFailureWasAuth(): boolean {
    return this.lastFailureWasAuth;
  }

  // SPEC-AUTH-003 UB1: 테스트/디버깅용 — 직전 connect 시도가 토큰을 동반했는지.
  getLastConnectAttemptHadToken(): boolean {
    return this.lastConnectAttemptHadToken;
  }

  // SPEC-AUTH-003 UB1: 테스트/디버깅용 — 직전 connect 동안 onopen 이 발생했는지.
  getConnectionEverEstablished(): boolean {
    return this.connectionEverEstablished;
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
    let parsed: { type?: string; payload?: unknown };
    try {
      parsed = JSON.parse(event.data as string);
    } catch {
      return; // Ignore malformed messages
    }

    const { type, payload } = parsed;
    if (!type) return;

    // Server pong response clears the pending pong timer
    if (type === 'pong') {
      this.clearPongTimer();
      return;
    }

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
        // Send JSON ping matching the server's expected message format
        try {
          this.send('ping', null);
        } catch {
          return; // Connection not open
        }
        this.pongTimer = setTimeout(() => {
          // Pong not received within timeout -- force reconnect
          this.ws?.close(4000, 'pong timeout');
        }, this.options.pongTimeout);
      }
    }, this.options.pingInterval);

    // Pause heartbeat when browser tab is hidden to prevent false timeouts.
    // Browsers throttle timers in background tabs, causing pong timeouts.
    this.visibilityHandler = () => {
      if (document.hidden) {
        this.stopHeartbeatTimers();
      } else if (this.ws?.readyState === WebSocket.OPEN) {
        this.startHeartbeat();
      }
    };
    document.addEventListener('visibilitychange', this.visibilityHandler);
  }

  private stopHeartbeat(): void {
    this.stopHeartbeatTimers();
    if (this.visibilityHandler) {
      document.removeEventListener('visibilitychange', this.visibilityHandler);
      this.visibilityHandler = null;
    }
  }

  private stopHeartbeatTimers(): void {
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
// SPEC-AUTH-003 U1: tokenGetter 옵션은 connect 시점마다 호출되어 최신 토큰을 query 로 동반한다.
// 기존 호출자(`createWSClient()` 인자 없음)와의 backward compatibility 를 유지한다.
export interface CreateWSClientOptions {
  /** WebSocket base URL. 미제공 시 window.location 기반으로 결정. */
  url?: string;
  /** connect 시점에 매번 호출되는 토큰 조회 콜백. truthy 반환 시 ?token=<jwt> 추가. */
  tokenGetter?: TokenGetter;
}

export function createWSClient(options: CreateWSClientOptions = {}): WSClient {
  const resolvedUrl =
    options.url ??
    `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`;
  return new WSClient(resolvedUrl, options.tokenGetter);
}
