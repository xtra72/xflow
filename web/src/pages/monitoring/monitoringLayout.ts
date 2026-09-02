// 모니터링 항목 어휘.
//
// 대시보드 모니터링 패널 5종(통계/메트릭/네트워크/로그/이벤트)이 "무엇을 보여줄지"를
// 이 어휘로 정한다. 패널 설정의 `config.items` 가 이 키들을 담고, DEFAULT_LAYOUT 이
// 패널 생성 시 기본 항목이 된다.
//
// 항목은 모두 "고유 키 집합" 모델이다. 같은 항목을 두 번 담을 수 없으므로 검증이
// 배열 하나로 끝나고, 설정 화면은 이미 담긴 항목을 자연스럽게 표시할 수 있다.

/** 섹션 식별자 */
export type MonitorSectionKey = 'stats' | 'metrics' | 'network' | 'logs' | 'events';

/** 섹션 렌더 순서 (통계 → 메트릭 → 로그 → 이벤트) */
export const MONITOR_SECTION_KEYS: readonly MonitorSectionKey[] = [
  'stats',
  'metrics',
  'network',
  'logs',
  'events',
] as const;

/**
 * 통계 항목 키.
 *
 * - `cpuUsage`~`uptime`: `GET /api/v1/monitor/metrics` 실측 런타임 통계.
 * - `totalFlows`/`runningFlows`: `useFlows()` 기반 플로우 통계.
 * - `logsReceived`/`eventsReceived`: WebSocket 수신 누적 카운트.
 * - `wsState`: WebSocket 연결 상태.
 */
export type StatItemKey =
  | 'cpuUsage'
  | 'memoryUsage'
  | 'goRoutines'
  | 'heapAlloc'
  | 'memSys'
  | 'uptime'
  | 'totalFlows'
  | 'runningFlows'
  | 'logsReceived'
  | 'eventsReceived'
  | 'wsState';

/** 메트릭 차트 항목 키 (WebSocket `flow.metrics` 채널) */
export type MetricItemKey = 'cpu' | 'memory' | 'throughput' | 'errorRate';

/**
 * 네트워크 항목 키.
 *
 * `GET /monitor/network` 의 누적 카운터를 두 가지로 그린다:
 *   - rate 계열(`rx*`/`tx*`): 단위시간당 증가량. 단위시간은 설정으로 초/분/시.
 *   - 누적 계열(`*Total`): 카운터 원값. 부팅 이후 총량 추이를 본다.
 */
export type NetworkItemKey =
  | 'rxBytes'
  | 'txBytes'
  | 'rxPackets'
  | 'txPackets'
  | 'rxBytesTotal'
  | 'txBytesTotal'
  | 'rxPacketsTotal'
  | 'txPacketsTotal';

/** 로그 뷰어 항목 키 — `all` 은 전체, 나머지는 해당 레벨 이상 고정 필터 */
export type LogItemKey = 'all' | 'error' | 'warn' | 'info' | 'debug';

/** 이벤트 타임라인 항목 키 — `all` 은 전체, 나머지는 해당 유형만 */
export type EventItemKey = 'all' | 'status_change' | 'deployment' | 'error' | 'system';

/** 항목 목록을 갖는 섹션 (테스트·순회용) */
export const MONITOR_SECTIONS_WITH_ITEMS = [
  'stats',
  'metrics',
  'network',
  'logs',
  'events',
] as const satisfies readonly MonitorSectionKey[];

/** 섹션별 기본 항목 묶음 */
export interface MonitoringLayout {
  stats: StatItemKey[];
  metrics: MetricItemKey[];
  network: NetworkItemKey[];
  logs: LogItemKey[];
  events: EventItemKey[];
}

/** 섹션별로 추가 가능한 전체 항목 키 (다이얼로그 목록 및 정규화 검증에 사용) */
export const VALID_ITEM_KEYS = {
  stats: [
    'cpuUsage',
    'memoryUsage',
    'goRoutines',
    'heapAlloc',
    'memSys',
    'uptime',
    'totalFlows',
    'runningFlows',
    'logsReceived',
    'eventsReceived',
    'wsState',
  ] as StatItemKey[],
  metrics: ['cpu', 'memory', 'throughput', 'errorRate'] as MetricItemKey[],
  network: [
    'rxBytes',
    'txBytes',
    'rxPackets',
    'txPackets',
    'rxBytesTotal',
    'txBytesTotal',
    'rxPacketsTotal',
    'txPacketsTotal',
  ] as NetworkItemKey[],
  logs: ['all', 'error', 'warn', 'info', 'debug'] as LogItemKey[],
  events: ['all', 'status_change', 'deployment', 'error', 'system'] as EventItemKey[],
} as const;

/**
 * 섹션별 기본 항목.
 *
 * 패널을 새로 추가했을 때 채워지는 항목이며(`uiStore.createDefaultPanel`),
 * `config.items` 가 없는 패널의 폴백이기도 하다(`readPanelItems`).
 */
export const DEFAULT_LAYOUT: MonitoringLayout = {
  stats: ['totalFlows', 'runningFlows', 'logsReceived', 'eventsReceived'],
  metrics: ['cpu', 'memory', 'throughput', 'errorRate'],
  network: ['rxBytes', 'txBytes'],
  logs: ['all'],
  events: ['all'],
};

/** 알려진 키만 남기고 중복을 제거한다 (순서는 입력 순서 유지) */
function sanitizeSection<K extends string>(raw: unknown, valid: readonly K[]): K[] | null {
  if (!Array.isArray(raw)) return null;
  const allowed = new Set<string>(valid);
  const seen = new Set<string>();
  const out: K[] = [];
  for (const entry of raw) {
    if (typeof entry !== 'string') continue;
    if (!allowed.has(entry) || seen.has(entry)) continue;
    seen.add(entry);
    out.push(entry as K);
  }
  return out;
}

/**
 * 섹션 하나의 항목 배열을 정규화한다. 배열이 아니면 null.
 *
 * 모니터링 페이지의 localStorage 레이아웃과 대시보드 패널의 `config.items` 가 같은
 * 항목 어휘를 쓰므로, 검증도 한곳에서 한다.
 */
export function sanitizeSectionItems(section: MonitorSectionKey, raw: unknown): string[] | null {
  return sanitizeSection(raw, VALID_ITEM_KEYS[section] as readonly string[]);
}
