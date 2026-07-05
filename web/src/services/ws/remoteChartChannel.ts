// 원격 차트 채널 SSE 클라이언트 (SPEC-REMOTE-001 M10, 그룹 L, REQ-L07).
//
// 로컬 차트는 `/ws/chart/{channel}` WebSocket(ChartChannelClient)으로 구독한다.
// 원격 노드의 차트는 별도 WS 경로를 신설하지 않고 M8 스트림 프록시에 추가된
// `chart` stream-action 을 SSE(EventSource)로 소비한다(OQ-L2 RESOLVED). 노드는
// 자신의 `/ws/chart/{channel}` 소스를 구독해 chart.backfill/chart.append 프레임을
// `stream_data` 로 중계하고, 서버는 이를 SSE `data:` 프레임으로 fan-out 한다.
//
// 프레임 형태는 로컬 WS 와 동일한 ChartServerMessage 이므로, useChartChannel/
// useChartChannels 의 onMessage 핸들러가 그대로 동작한다(REQ-L07 — 동일 패널 코드).
//
// 토큰: EventSource 는 헤더를 설정할 수 없으므로 JWT 를 `?token=` 쿼리로 운반한다
// (useRemoteStream 과 동일 패턴). teardown: disconnect 시 EventSource 를 close 한다
// (REQ-J08b — 누수 없음).

import { getAuthState } from '@/stores';

import type {
  ChartChannelClientOptions,
  ChartConnectionStatus,
  ChartServerMessage,
} from './chartChannel';
import type { ChartChannelClientLike } from '@/pages/dashboard/panels/charts/useChartChannel';

/** 테스트 가능성을 위한 최소 EventSource 인터페이스(useRemoteStream 과 동일). */
export interface EventSourceLike {
  addEventListener(type: string, listener: (ev: MessageEvent) => void): void;
  close(): void;
  onerror: ((ev: Event) => void) | null;
  onopen: ((ev: Event) => void) | null;
}

/** access_token 을 authStore 에서 읽는다(없으면 undefined). */
function currentToken(): string | undefined {
  return getAuthState().tokens?.access_token ?? undefined;
}

/**
 * 원격 차트 SSE 클라이언트. ChartChannelClient 와 동일한 ChartChannelClientLike
 * 계약을 구현하되, WebSocket 대신 SSE 로 동일한 chart.* 프레임을 중계한다.
 *
 * 로컬 ChartChannelClient 와 달리:
 *   - SSE base URL 은 생성 시점에 주입된다(remoteChartStreamUrl). opts.url(WS 경로)은
 *     무시한다(원격은 SSE 경로 사용 — useChartChannel 이 createClient 로 본 클라이언트를
 *     주입한다).
 *   - chart.closed 수신 시 종단 상태로 전이해 재연결을 금지한다(로컬과 동일).
 *   - 전송 계층 오류(onerror)는 'disconnected' 로 알리되, 폴링 폴백은 호출 측이
 *     별도 처리하지 않는다(차트 패널은 closed/error 오버레이로 표시 — REQ-L07 폴백은
 *     스냅샷 query-action 이나 v1.5 차트는 라이브 전용으로 둔다).
 */
export class RemoteChartChannelClient implements ChartChannelClientLike {
  private readonly baseUrl: string;
  private readonly onMessage: (msg: ChartServerMessage) => void;
  private readonly onStatus: (status: ChartConnectionStatus, detail?: string) => void;
  private readonly factory: (url: string) => EventSourceLike;

  private source: EventSourceLike | null = null;
  private disposed = false;
  /** chart.closed 수신 후 true; 재연결 영구 금지. */
  private terminal = false;

  constructor(
    opts: ChartChannelClientOptions,
    baseUrl: string,
    factory?: (url: string) => EventSourceLike,
  ) {
    this.onMessage = opts.onMessage;
    this.onStatus = opts.onStatus;
    this.baseUrl = baseUrl;
    this.factory =
      factory ?? ((u: string) => new EventSource(u) as unknown as EventSourceLike);
  }

  connect(): void {
    if (this.disposed || this.terminal) return;
    if (this.source) return;

    this.onStatus('connecting');

    // 매 연결마다 최신 토큰을 부착한다(refresh 반영).
    const token = currentToken();
    const url = token
      ? `${this.baseUrl}${this.baseUrl.includes('?') ? '&' : '?'}token=${encodeURIComponent(token)}`
      : this.baseUrl;

    const source = this.factory(url);
    this.source = source;

    source.onopen = () => {
      if (this.disposed) return;
      this.onStatus('connected');
    };

    // data: <json> — chart.backfill / chart.append 프레임(로컬과 동일 형태).
    source.addEventListener('message', (ev: MessageEvent) => {
      if (this.disposed) return;
      this.handleFrame(ev.data);
    });

    // event: end — 노드 스트림 종료(오프라인/터미널). closed 로 전이.
    source.addEventListener('end', () => {
      if (this.disposed) return;
      this.terminal = true;
      this.teardown();
      this.onStatus('closed', 'stream ended');
    });

    // event: error — 구독 실패 통지. error 로 전이.
    source.addEventListener('error', () => {
      if (this.disposed) return;
      this.terminal = true;
      this.teardown();
      this.onStatus('error', 'stream error');
    });

    // 전송 계층 오류(연결 끊김). disconnected 로 알린다.
    source.onerror = () => {
      if (this.disposed || this.terminal) return;
      this.onStatus('disconnected');
    };
  }

  disconnect(): void {
    this.disposed = true;
    this.teardown();
  }

  isConnected(): boolean {
    return this.source !== null;
  }

  // --- private ---

  private handleFrame(raw: unknown): void {
    if (typeof raw !== 'string' || raw.length === 0) return;
    let parsed: ChartServerMessage;
    try {
      parsed = JSON.parse(raw) as ChartServerMessage;
    } catch {
      return;
    }
    if (!parsed || typeof parsed !== 'object' || !('type' in parsed)) return;

    if (parsed.type === 'chart.closed') {
      this.terminal = true;
      this.teardown();
      this.onStatus('closed', parsed.reason);
      this.onMessage(parsed);
      return;
    }
    if (parsed.type === 'chart.error') {
      this.onStatus('error', parsed.reason);
      this.onMessage(parsed);
      return;
    }
    this.onMessage(parsed);
  }

  private teardown(): void {
    if (this.source) {
      this.source.close();
      this.source = null;
    }
  }
}
