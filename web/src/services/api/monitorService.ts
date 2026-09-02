import { useQuery, type UseQueryResult } from '@tanstack/react-query';

import { del, get, put } from './client';

/**
 * `GET /api/v1/monitor/metrics` 응답 (envelope 풀린 후).
 *
 * 백엔드 `MetricsResponse`(internal/api/handler/monitor.go) 의 JSON 직렬화 태그와
 * 1:1 로 매핑된다. CPU 사용률은 샘플링이 필요해 v1 에서 0 으로 고정되며, 나머지는
 * Go runtime(MemStats, NumGoroutine) 기반 실측값이다.
 *
 * @spec SPEC-WEB-007
 */
export interface SystemMetrics {
  /** 프로세스 CPU 사용률(%). v1 백엔드는 0 고정(샘플링 미지원). */
  cpu_usage_percent: number;
  /** 메모리 사용률(%). MemStats.Alloc / MemStats.Sys * 100. */
  memory_usage_percent: number;
  /** 활성 goroutine 수. */
  go_routines: number;
  /** 힙 할당량(MB). MemStats.Alloc. */
  go_mem_alloc_mb: number;
  /** OS 로부터 예약한 메모리(MB). MemStats.Sys. */
  go_mem_sys_mb: number;
  /** 프로세스 uptime(초). */
  uptime_seconds: number;
  // 인덱스 시그니처: 6개 명명 필드는 타입 안전하게 보장하되, 기존 소비자
  // (ResourceWidget/renderDashboardPanel 등 Record<string, unknown> 기대)와의
  // 구조적 호환 및 향후 백엔드/WS 추가 필드(throughput, error_rate 등 별칭)를
  // 깨지 않고 수용하기 위함이다. any 가 아닌 unknown 으로 타입 안전을 유지한다.
  [key: string]: unknown;
}

/**
 * 런타임 시스템 메트릭을 조회한다 (envelope 인터셉터가 풀어준 payload).
 *
 * SPEC-WEB-007 에서 느슨한 `Record<string, unknown>` 대신 백엔드 직렬화 태그와
 * 일치하는 `SystemMetrics` 로 강화했다. ResourceWidget 등 기존 소비자는
 * 이 payload 를 그대로 사용하므로 동작은 보존된다.
 */
export async function getMetrics(): Promise<SystemMetrics> {
  return get<SystemMetrics>('/monitor/metrics');
}

/**
 * Change the server-side log level at runtime.
 * Valid levels typically include: debug, info, warn, error.
 */
export async function setLogLevel(level: string): Promise<void> {
  await put<void>('/monitor/loglevel', { level });
}

// --- 컴포넌트별 로그 레벨 ---

/** 서버에서 반환하는 로그 레벨 정보 */
export interface LogLevelInfo {
  default_level: string;
  components: Record<string, string>;
}

/**
 * 전체 로그 레벨 정보를 조회한다.
 * 기본 레벨과 컴포넌트별 오버라이드를 포함한다.
 */
export async function getLogLevels(): Promise<LogLevelInfo> {
  return get<LogLevelInfo>('/monitor/loglevel');
}

/**
 * 특정 컴포넌트의 로그 레벨을 설정한다.
 */
export async function setComponentLogLevel(component: string, level: string): Promise<void> {
  await put<void>(`/monitor/loglevel/${component}`, { level });
}

/**
 * 특정 컴포넌트의 로그 레벨 오버라이드를 제거하여 기본값으로 되돌린다.
 */
export async function resetComponentLogLevel(component: string): Promise<void> {
  await del(`/monitor/loglevel/${component}`);
}

// --- 로그 출력 식별자 표시 방식 ---

/**
 * 로그 출력에서 agent/device/node 식별자를 표시하는 방식.
 *
 * - `name`: 사람이 읽기 좋은 이름만 표시(가독성 우선).
 * - `id`: UUID·composite ID만 표시(로그 파싱·자동화 우선).
 * - `both`: 이름과 ID를 함께 표시(디버깅 시 상관관계 추적).
 */
export type LogStyle = 'name' | 'id' | 'both';

/**
 * 백엔드 `GET/PUT /api/v1/monitor/logstyle` 응답 payload
 * (envelope 인터셉터가 `data` 를 풀어준 뒤의 형태).
 */
export interface LogStyleInfo {
  style: LogStyle;
}

/**
 * 현재 로그 출력 식별자 표시 방식을 조회한다
 * (envelope 인터셉터가 풀어준 payload).
 */
export async function getLogStyle(): Promise<LogStyleInfo> {
  return get<LogStyleInfo>('/monitor/logstyle');
}

/**
 * 로그 출력 식별자 표시 방식을 변경한다.
 * 잘못된 값은 백엔드가 400(INVALID_LOG_STYLE)으로 거부한다.
 */
export async function setLogStyle(style: LogStyle): Promise<void> {
  await put<void>('/monitor/logstyle', { style });
}

// ─────────────────────────────────────────────────────────────────────
// SPEC-WEB-007 — 시스템 메트릭 폴링 훅
// ─────────────────────────────────────────────────────────────────────

/**
 * 시스템 메트릭 폴링 훅 (5초 간격).
 *
 * SPEC-WEB-007:
 *   - `queryKey: ['monitor', 'metrics']` — 동일 엔드포인트/queryFn 을 쓰는 기존
 *     `useMetricsTarget`(로컬 분기) 과 캐시를 공유한다. 별도 키를 쓰면 같은
 *     리소스에 대해 중복 폴링이 발생하므로 의도적으로 통일했다.
 *   - `refetchInterval: 5_000` — 시스템 정보 패널의 갱신 주기.
 *   - `refetchIntervalInBackground: false` — 다른 탭으로 전환 시 폴링 일시 중지
 *     (노드 부하/네트워크 절감).
 */
export function useSystemMetrics(
  refetchIntervalMs: number = 5_000,
): UseQueryResult<SystemMetrics, Error> {
  return useQuery<SystemMetrics, Error>({
    queryKey: ['monitor', 'metrics'],
    queryFn: getMetrics,
    // 인자를 받도록 넓혔지만 기본값은 기존 5초 그대로다 — 주기를 넘기지 않는
    // 기존 호출부(SystemInfoCard 등)의 동작은 바뀌지 않는다. 캐시 키는 그대로라
    // 여러 소비자가 같은 쿼리를 공유하며, 각자 자기 주기로 재요청한다.
    refetchInterval: refetchIntervalMs,
    refetchIntervalInBackground: false,
  });
}

// ─────────────────────────────────────────────────────────────────────
// 네트워크 인터페이스 통계
// ─────────────────────────────────────────────────────────────────────

/**
 * 인터페이스 하나의 누적 카운터.
 *
 * 백엔드 `NetworkInterfaceStat`(internal/api/handler/monitor_network.go) 와 1:1 이다.
 * 값은 모두 부팅 이후 누적치이며, 초당 전송량 같은 비율은 두 시점의 차이로 계산한다.
 */
export interface NetworkInterfaceStat {
  /** 인터페이스 이름. 합산 항목은 "total". */
  name: string;
  bytes_sent: number;
  bytes_recv: number;
  packets_sent: number;
  packets_recv: number;
  err_in: number;
  err_out: number;
  drop_in: number;
  drop_out: number;
}

/** `GET /api/v1/monitor/network` 응답 (envelope 풀린 후) */
export interface NetworkStats {
  /** 모든 인터페이스의 합산 */
  total: NetworkInterfaceStat;
  /** 인터페이스별 통계 (이름 사전순) */
  interfaces: NetworkInterfaceStat[];
}

/** 네트워크 인터페이스 통계를 조회한다. */
export async function getNetworkStats(): Promise<NetworkStats> {
  return get<NetworkStats>('/monitor/network');
}

/**
 * 네트워크 인터페이스 통계 폴링 훅.
 *
 * `useSystemMetrics` 와 같은 이유로 queryKey 를 고정해 여러 소비자가 캐시를 공유한다.
 */
export function useNetworkStats(
  refetchIntervalMs: number = 5_000,
): UseQueryResult<NetworkStats, Error> {
  return useQuery<NetworkStats, Error>({
    queryKey: ['monitor', 'network'],
    queryFn: getNetworkStats,
    refetchInterval: refetchIntervalMs,
    refetchIntervalInBackground: false,
  });
}

/**
 * `GET /api/v1/monitor/sysresources` 응답 (envelope 풀린 후).
 *
 * 값이 아니라 "선택 가능한 이름 목록"이다. sysmetrics 에이전트 설정에서 마운트·
 * 장치·인터페이스를 손으로 적는 대신 골라 쓰게 하기 위한 조회다.
 */
export interface SysResources {
  /** 물리 파티션의 마운트 지점 (사전순) */
  mountpoints: string[];
  /** I/O 통계를 낼 수 있는 디스크 장치 이름 (사전순) */
  devices: string[];
  /** 네트워크 인터페이스 이름 (사전순) */
  interfaces: string[];
}

/** 선택 가능한 관측 대상 목록을 조회한다. */
export async function getSysResources(): Promise<SysResources> {
  return get<SysResources>('/monitor/sysresources');
}

/**
 * 관측 대상 목록 조회 훅.
 *
 * 목록은 거의 변하지 않으므로(마운트·NIC 추가는 드물다) 폴링하지 않고 캐시한다.
 * 설정 화면을 여는 동안만 필요한 값이라 재요청 비용을 들일 이유가 없다.
 */
export function useSysResources(): UseQueryResult<SysResources, Error> {
  return useQuery<SysResources, Error>({
    queryKey: ['monitor', 'sysresources'],
    queryFn: getSysResources,
    staleTime: 60_000,
  });
}
