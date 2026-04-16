// 차트 채널 WebSocket 클라이언트.
// SPEC-CHART-001 §4.1.2 WebSocket 프레임 규격을 구현한다.
// - chart.backfill / chart.append / chart.closed / chart.error 메시지 파싱
// - 예기치 않은 close 에 대해 exponential backoff 재연결 (REQ-M4-10)
// - chart.closed 수신 시 재연결 중단 (REQ-M4-11)

// --- 타입 ---

import type { ChartEntry } from '@/pages/dashboard/panels/charts/chartChannelTypes';

export interface ChartBackfillMessage {
  type: 'chart.backfill';
  channel: string;
  entries: ChartEntry[];
  backfilled_count: number;
}

export interface ChartAppendMessage {
  type: 'chart.append';
  channel: string;
  entry: ChartEntry;
}

export interface ChartClosedMessage {
  type: 'chart.closed';
  channel: string;
  reason: string;
}

export interface ChartErrorMessage {
  type: 'chart.error';
  channel: string;
  reason: string;
}

export type ChartServerMessage =
  | ChartBackfillMessage
  | ChartAppendMessage
  | ChartClosedMessage
  | ChartErrorMessage;

export type ChartConnectionStatus =
  | 'idle'
  | 'connecting'
  | 'connected'
  | 'disconnected'
  | 'closed'
  | 'error';

export interface ChartChannelClientOptions {
  /** 전체 WebSocket URL (예: ws://host/ws/chart/{channel}) */
  url: string;
  /** 메시지 콜백 */
  onMessage: (msg: ChartServerMessage) => void;
  /** 상태 전이 콜백 */
  onStatus: (status: ChartConnectionStatus, detail?: string) => void;
  /** exponential backoff ms (default: [1000, 2000, 4000, 8000, 16000]) */
  backoff?: number[];
  /** 최대 재연결 시도 수 (default: Infinity) */
  maxReconnectAttempts?: number;
  /** 테스트용 소켓 팩토리 (default: new WebSocket(url)) */
  createSocket?: (url: string) => WebSocket;
}

const DEFAULT_BACKOFF = [1000, 2000, 4000, 8000, 16000] as const;

/**
 * 단일 차트 채널용 WebSocket 클라이언트.
 *
 * 기존 WSClient 와 달리 본 클라이언트는:
 * - 채널별 독립 연결 (메인 /ws 와 분리)
 * - chart.closed 수신 시 재연결 금지
 * - 서버측 envelope 에 type 을 이미 포함하므로 별도 라우팅 없음
 */
export class ChartChannelClient {
  private readonly opts: ChartChannelClientOptions;
  private readonly backoff: number[];
  private readonly maxReconnect: number;
  private readonly createSocket: (url: string) => WebSocket;

  private ws: WebSocket | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private retryCount = 0;
  private lastStatus: ChartConnectionStatus = 'idle';
  private disposed = false;
  /** chart.closed 수신 후 true; 재연결 영구 금지 */
  private terminal = false;

  constructor(opts: ChartChannelClientOptions) {
    this.opts = opts;
    this.backoff = opts.backoff ?? [...DEFAULT_BACKOFF];
    this.maxReconnect = opts.maxReconnectAttempts ?? Number.POSITIVE_INFINITY;
    this.createSocket = opts.createSocket ?? ((u) => new WebSocket(u));
  }

  connect(): void {
    if (this.disposed || this.terminal) return;
    if (this.ws && this.ws.readyState === WebSocket.OPEN) return;

    this.setStatus('connecting');

    const socket = this.createSocket(this.opts.url);
    this.ws = socket;

    socket.onopen = () => {
      this.retryCount = 0;
      this.setStatus('connected');
    };

    socket.onmessage = (ev: MessageEvent) => {
      this.handleMessage(ev);
    };

    socket.onclose = () => {
      this.ws = null;
      if (this.disposed || this.terminal) {
        this.setStatus('disconnected');
        return;
      }
      this.setStatus('disconnected');
      this.scheduleReconnect();
    };

    socket.onerror = () => {
      this.setStatus('error');
    };
  }

  disconnect(): void {
    this.disposed = true;
    this.clearReconnectTimer();

    const socket = this.ws;
    if (socket) {
      // 콜백 제거하여 close 이벤트에 의한 재연결 방지
      socket.onopen = null;
      socket.onmessage = null;
      socket.onclose = null;
      socket.onerror = null;
      try {
        if (
          socket.readyState === WebSocket.OPEN ||
          socket.readyState === WebSocket.CONNECTING
        ) {
          socket.close(1000, 'client disconnect');
        }
      } catch {
        // 닫기 실패는 무시
      }
      this.ws = null;
    }
  }

  isConnected(): boolean {
    return this.ws !== null && this.ws.readyState === WebSocket.OPEN;
  }

  // --- private ---

  private handleMessage(ev: MessageEvent): void {
    let parsed: ChartServerMessage;
    try {
      parsed = JSON.parse(ev.data as string) as ChartServerMessage;
    } catch {
      return;
    }
    if (!parsed || typeof parsed !== 'object' || !('type' in parsed)) return;

    // chart.closed 는 종단 상태 — 재연결 금지 (REQ-M4-11)
    if (parsed.type === 'chart.closed') {
      this.terminal = true;
      this.clearReconnectTimer();
      this.setStatus('closed', parsed.reason);
      this.opts.onMessage(parsed);
      return;
    }

    if (parsed.type === 'chart.error') {
      this.setStatus('error', parsed.reason);
      this.opts.onMessage(parsed);
      return;
    }

    this.opts.onMessage(parsed);
  }

  private scheduleReconnect(): void {
    if (this.disposed || this.terminal) return;
    if (this.retryCount >= this.maxReconnect) return;

    const idx = Math.min(this.retryCount, this.backoff.length - 1);
    const delay = this.backoff[idx] ?? DEFAULT_BACKOFF[DEFAULT_BACKOFF.length - 1];
    this.retryCount += 1;

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

  private setStatus(status: ChartConnectionStatus, detail?: string): void {
    if (this.lastStatus === status && detail === undefined) return;
    this.lastStatus = status;
    this.opts.onStatus(status, detail);
  }
}
