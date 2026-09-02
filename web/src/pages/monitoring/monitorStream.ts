// 모니터링 실시간 스트림 — 프로세스 전역 단일 구독.
//
// 모니터링 페이지와 대시보드 모니터링 패널이 같은 WebSocket 스트림(flow.metrics /
// log.entry / system.event)을 본다. 소비자마다 핸들러를 붙이면 대시보드에 패널을
// 3개 올렸을 때 로그 버퍼가 3벌 생기고 메모리도 3배가 된다. 그래서 버퍼는 모듈
// 하나가 소유하고, 소비자는 refcount 로 붙었다 떨어진다.
//
// 스냅샷은 불변 객체를 통째로 갈아 끼우므로 useSyncExternalStore 의 참조 비교가
// 그대로 동작한다.

import { useEffect, useSyncExternalStore } from 'react';

import { useWebSocket } from '@/hooks';
import type { WSClient } from '@/services/ws/wsClient';
import { WS_MESSAGE_TYPES } from '@/services/ws/wsHandlers';

import { appendLog } from './logBuffer';
import type { LogEntry, LogLevel } from './LogViewer';
import type { MetricDataPoint, MetricsData } from './MetricsChart';
import type { SystemEvent } from './EventTimeline';

/** 메트릭 보관 기간 (5분, 1초 간격 가정) */
const MAX_METRIC_POINTS = 300;
/** 이벤트 보관 상한 — 표시는 100건이지만 유형별 필터를 위해 넉넉히 둔다. */
const MAX_EVENT_BUFFER = 1_000;

/** 구독자가 보는 스트림 스냅샷 */
export interface MonitorStreamState {
  metrics: MetricsData;
  logs: LogEntry[];
  events: SystemEvent[];
  /** 구독 시작 후 수신한 로그 누적 건수 (버퍼 상한과 무관) */
  logsReceived: number;
  /** 구독 시작 후 수신한 이벤트 누적 건수 (버퍼 상한과 무관) */
  eventsReceived: number;
}

const EMPTY_STATE: MonitorStreamState = {
  metrics: { cpu: [], memory: [], throughput: [], errorRate: [] },
  logs: [],
  events: [],
  logsReceived: 0,
  eventsReceived: 0,
};

let state: MonitorStreamState = EMPTY_STATE;
const listeners = new Set<() => void>();

/** 새 스냅샷으로 교체하고 구독자에게 알린다. */
function commit(next: MonitorStreamState): void {
  state = next;
  for (const listener of listeners) listener();
}

/** 타임스탬프를 HH:MM:SS 형태로 포맷 */
function formatTime(ts: string | Date): string {
  const d = ts instanceof Date ? ts : new Date(ts);
  const h = String(d.getHours()).padStart(2, '0');
  const m = String(d.getMinutes()).padStart(2, '0');
  const s = String(d.getSeconds()).padStart(2, '0');
  return `${h}:${m}:${s}`;
}

/** 메트릭 데이터 포인트 추가 (최대 개수 제한) */
function appendMetric(prev: MetricDataPoint[], point: MetricDataPoint): MetricDataPoint[] {
  const next = [...prev, point];
  return next.length > MAX_METRIC_POINTS ? next.slice(next.length - MAX_METRIC_POINTS) : next;
}

/** 고유 ID 생성 (로그/이벤트용) */
let idCounter = 0;
function nextId(): string {
  idCounter += 1;
  return `m-${idCounter}`;
}

// --- WS 핸들러 (모듈 수준 고정 참조 — off() 가 정확히 같은 함수를 받아야 한다) ---

function handleMetrics(data: unknown): void {
  const d = data as Record<string, number>;
  // 차트 X축이 표시 구간에 고정되려면 시각이 수치여야 한다(라벨 문자열이 아니라).
  const ts = Date.now();
  const prev = state.metrics;

  commit({
    ...state,
    metrics: {
      cpu: appendMetric(prev.cpu, { ts, value: d.cpu ?? 0 }),
      memory: appendMetric(prev.memory, { ts, value: d.memory ?? 0 }),
      throughput: appendMetric(prev.throughput, { ts, value: d.throughput ?? 0 }),
      errorRate: appendMetric(prev.errorRate, { ts, value: d.error_rate ?? 0 }),
    },
  });
}

function handleLog(data: unknown): void {
  const d = data as {
    level?: string;
    message?: string;
    timestamp?: string;
    component?: string;
    source?: string;
    componentKind?: string;
    componentName?: string;
  };
  // 표시용 문자열은 날짜를 버리므로, 정렬·구간 필터가 쓸 epoch 값을 따로 남긴다.
  // 백엔드는 slog 의 RFC3339 시각을 그대로 보낸다(internal/api/ws/log_writer.go).
  const parsed = d.timestamp ? Date.parse(d.timestamp) : Number.NaN;
  const ts = Number.isNaN(parsed) ? Date.now() : parsed;

  const entry: LogEntry = {
    id: nextId(),
    timestamp: d.timestamp ? formatTime(d.timestamp) : formatTime(new Date()),
    ts,
    level: (d.level?.toUpperCase() as LogLevel) ?? 'INFO',
    message: d.message ?? '',
    component: d.component ?? '',
    source: d.source ?? 'system',
    componentKind: d.componentKind ?? '',
    componentName: d.componentName ?? '',
  };

  commit({
    ...state,
    logs: appendLog(state.logs, entry),
    logsReceived: state.logsReceived + 1,
  });
}

function handleEvent(data: unknown): void {
  const d = data as {
    type?: string;
    message?: string;
    timestamp?: string;
    details?: string;
  };
  // 메시지 폴백은 렌더 시점(EventTimeline)에서 로케일에 맞춰 채운다 — 이 모듈은
  // i18n 컨텍스트 밖이라 번역 함수를 쓸 수 없다.
  const event: SystemEvent = {
    id: nextId(),
    type: (d.type as SystemEvent['type']) ?? 'system',
    message: d.message ?? '',
    timestamp: d.timestamp ?? new Date().toISOString(),
    details: d.details,
  };

  const nextEvents = [...state.events, event];
  commit({
    ...state,
    events:
      nextEvents.length > MAX_EVENT_BUFFER
        ? nextEvents.slice(nextEvents.length - MAX_EVENT_BUFFER)
        : nextEvents,
    eventsReceived: state.eventsReceived + 1,
  });
}

// --- refcount 구독 ---

let refCount = 0;
let attachedClient: WSClient | null = null;

/**
 * WS 클라이언트에 스트림을 연결한다. 반환된 해제 함수를 호출하면 refcount 가 줄고,
 * 마지막 소비자가 떠날 때 실제로 핸들러를 뗀다.
 *
 * 클라이언트 인스턴스가 바뀌면(재연결 등) 이전 클라이언트에서 떼고 새로 붙인다.
 */
export function attachMonitorStream(client: WSClient): () => void {
  if (attachedClient && attachedClient !== client) {
    detachHandlers(attachedClient);
    attachedClient = null;
  }
  if (!attachedClient) {
    attachedClient = client;
    attachHandlers(client);
  }
  refCount += 1;

  let released = false;
  return () => {
    // 같은 해제 함수가 두 번 불려도 refcount 가 음수로 내려가지 않게 막는다.
    if (released) return;
    released = true;
    refCount -= 1;
    if (refCount <= 0) {
      refCount = 0;
      if (attachedClient) detachHandlers(attachedClient);
      attachedClient = null;
    }
  };
}

function attachHandlers(client: WSClient): void {
  client.on(WS_MESSAGE_TYPES.FLOW_METRICS, handleMetrics);
  client.on(WS_MESSAGE_TYPES.LOG_ENTRY, handleLog);
  client.on(WS_MESSAGE_TYPES.SYSTEM_EVENT, handleEvent);
}

function detachHandlers(client: WSClient): void {
  client.off(WS_MESSAGE_TYPES.FLOW_METRICS, handleMetrics);
  client.off(WS_MESSAGE_TYPES.LOG_ENTRY, handleLog);
  client.off(WS_MESSAGE_TYPES.SYSTEM_EVENT, handleEvent);
}

/** 구독 등록 (useSyncExternalStore 용) */
function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** 현재 스냅샷 (useSyncExternalStore 용 — 참조가 안정적이어야 한다) */
function getSnapshot(): MonitorStreamState {
  return state;
}

/**
 * 버퍼와 구독을 초기 상태로 되돌린다. 테스트 격리용.
 *
 * 구독자 목록은 비우지 않는다 — 살아 있는 컴포넌트의 구독을 끊으면 이후 갱신이
 * 전달되지 않는다. 구독 해제는 언마운트가 담당한다.
 */
export function resetMonitorStream(): void {
  if (attachedClient) detachHandlers(attachedClient);
  attachedClient = null;
  refCount = 0;
  idCounter = 0;
  commit(EMPTY_STATE);
}

/**
 * 모니터링 스트림 구독 훅.
 *
 * 소비자가 몇 개든 WS 핸들러는 한 벌만 걸린다. 마지막 소비자가 언마운트되면
 * 핸들러가 정리되므로 화면을 떠난 뒤 버퍼가 계속 자라지 않는다.
 */
export function useMonitorStream(): MonitorStreamState {
  const { client } = useWebSocket();

  useEffect(() => {
    if (!client) return;
    return attachMonitorStream(client);
  }, [client]);

  return useSyncExternalStore(subscribe, getSnapshot);
}
