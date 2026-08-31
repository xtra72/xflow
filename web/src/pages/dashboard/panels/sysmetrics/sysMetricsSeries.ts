// sysmetrics 에이전트 상태 → 화면 시리즈 변환.
//
// 에이전트는 `State()` 로 "그 시점의 스냅샷" 하나만 준다. 화면이 보는 것은 두 갈래다:
//   - 상태값 계열 (cpu·memory·storage): 스냅샷 값을 그대로 쓴다.
//   - 누적 카운터 계열 (network·disk_io): 연속한 두 스냅샷의 차이를 단위시간으로 환산한다.
//
// 이 파일에는 순수 함수만 둔다. 차트나 React 를 섞으면 되감김·경과 0·대상 결측 같은
// 경계를 테스트하기 위해 컴포넌트를 띄워야 하는데, 그 경계들이야말로 실제로 깨지는
// 곳이라 값싸게 검증할 수 있어야 한다.

import { UNIT_TIME_SECONDS, type UnitTime } from '@/pages/monitoring/networkSeries';

// --- 스냅샷 타입 ---

/** 인스턴스 축이 없는 지표 그룹 (필드 → 값) */
export type MetricGroup = Record<string, number>;

/** 인스턴스 축이 있는 지표 그룹 (인스턴스 → 필드 → 값) */
export type InstanceGroup = Record<string, MetricGroup>;

/** 에이전트 상태 문자열 (백엔드 sysmetrics_state.go 와 같은 어휘) */
export type SysMetricsStatus = 'running' | 'stopped' | 'no_sample';

/** 관측 대상 이름 목록 */
export interface SysMetricsTargets {
  mountpoints: string[];
  devices: string[];
  interfaces: string[];
}

/**
 * 에이전트 `state` 를 타입 있는 형태로 정리한 것.
 *
 * 지표 그룹이 **부재**하면 그 지표는 수집이 꺼져 있다는 뜻이다. 빈 오브젝트(`{}`)와
 * 구분해야 한다 — 빈 오브젝트는 "수집은 켰으나 대상이 없다"이다.
 */
export interface SysMetricsSnapshot {
  status: SysMetricsStatus;
  /** 표본 시각 (epoch ms). 표본 이전이면 null. */
  collectedAt: number | null;
  /** 표본 주기 (초) */
  intervalSeconds: number;
  cpu?: MetricGroup;
  /** 논리 코어 수 (정적 정보) */
  cpuCores?: number;
  memory?: MetricGroup;
  storage?: InstanceGroup;
  diskIo?: InstanceGroup;
  network?: InstanceGroup;
  targets: SysMetricsTargets;
}

/** 수집 여부를 물을 수 있는 지표 이름 */
export type SysMetricKind = 'cpu' | 'memory' | 'storage' | 'diskIo' | 'network';

// --- 파싱 ---

/** 값이 유한한 수인지 */
function num(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

/** 문자열 배열만 남긴다. */
function stringList(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.filter((v): v is string => typeof v === 'string' && v !== '');
}

/** 필드 → 수 맵으로 정리한다. 수가 아닌 값은 버린다. */
function toMetricGroup(value: unknown): MetricGroup | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  const out: MetricGroup = {};
  for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
    const n = num(v);
    if (n !== undefined) out[k] = n;
  }
  return out;
}

/** 인스턴스 → 필드 → 수 맵으로 정리한다. */
function toInstanceGroup(value: unknown): InstanceGroup | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  const out: InstanceGroup = {};
  for (const [name, group] of Object.entries(value as Record<string, unknown>)) {
    const parsed = toMetricGroup(group);
    if (parsed) out[name] = parsed;
  }
  return out;
}

/** 알려진 상태 문자열로 정규화한다. 모르는 값은 표본 없음으로 본다. */
function toStatus(value: unknown): SysMetricsStatus {
  return value === 'running' || value === 'stopped' ? value : 'no_sample';
}

/**
 * 에이전트 응답의 `state` 를 스냅샷으로 정리한다.
 *
 * `state` 자체가 없으면(에이전트가 삭제됐거나 다른 타입이면) null 을 돌려준다 —
 * 호출부가 "값이 없다"와 "형식이 다르다"를 같은 분기로 다룰 수 있게 한다.
 */
export function toSnapshot(state: unknown): SysMetricsSnapshot | null {
  if (!state || typeof state !== 'object' || Array.isArray(state)) return null;
  const raw = state as Record<string, unknown>;

  const targetsRaw = (raw.targets ?? {}) as Record<string, unknown>;

  return {
    status: toStatus(raw.status),
    collectedAt: num(raw.collected_at) ?? null,
    intervalSeconds: num(raw.interval_seconds) ?? 0,
    cpu: toMetricGroup(raw.cpu),
    cpuCores: num(raw.cpu_cores),
    memory: toMetricGroup(raw.memory),
    storage: toInstanceGroup(raw.storage),
    diskIo: toInstanceGroup(raw.disk_io),
    network: toInstanceGroup(raw.network),
    targets: {
      mountpoints: stringList(targetsRaw.mountpoints),
      devices: stringList(targetsRaw.devices),
      interfaces: stringList(targetsRaw.interfaces),
    },
  };
}

// --- 수집 여부 ---

/**
 * 그 지표를 수집하고 있는지.
 *
 * 그룹이 **부재**하면 수집이 꺼진 것이고, 빈 오브젝트이면 수집은 켰으나 대상이 없는
 * 것이다. 둘을 같게 다루면 화면에서 "디스크가 비어 있다"와 "관측하지 않는다"가
 * 구별되지 않는다.
 */
export function isCollected(snapshot: SysMetricsSnapshot, kind: SysMetricKind): boolean {
  return snapshot[kind] !== undefined;
}

// --- 합산 ---

/**
 * 인스턴스 그룹의 필드를 전부 더한다.
 *
 * 어떤 인스턴스에 그 필드가 없으면 0 으로 보고 건너뛴다 — OS 마다 채워 주는 필드가
 * 달라, 하나가 빠졌다고 합계를 통째로 버리면 화면이 비게 된다.
 */
export function sumInstances(group: InstanceGroup | undefined): MetricGroup {
  const out: MetricGroup = {};
  if (!group) return out;
  for (const metrics of Object.values(group)) {
    for (const [field, value] of Object.entries(metrics)) {
      out[field] = (out[field] ?? 0) + value;
    }
  }
  return out;
}

/** 디스크 I/O 종합 — 모든 장치의 누적 카운터 합. */
export function sumDiskIO(snapshot: SysMetricsSnapshot): MetricGroup {
  return sumInstances(snapshot.diskIo);
}

/** 네트워크 종합 — 모든 인터페이스의 누적 카운터 합. */
export function sumNetwork(snapshot: SysMetricsSnapshot): MetricGroup {
  return sumInstances(snapshot.network);
}

/** 스토리지 합계 */
export interface StorageTotals {
  usedBytes: number;
  freeBytes: number;
  totalBytes: number;
  /** 합계 기준 사용률 (%) */
  usagePercent: number;
}

/**
 * 스토리지 종합 — 모든 마운트의 용량 합과 **합계 기준** 사용률.
 *
 * 각 마운트 사용률의 평균이 아니다. 1TB 디스크 90% 와 1GB 램디스크 10% 의 평균 50% 는
 * 어떤 현실도 나타내지 않는다.
 *
 * 같은 볼륨의 여러 마운트(macOS 의 `/` 와 `/System/Volumes/Data`)는 합치지 않고 그대로
 * 더한다. 어떤 마운트가 같은 볼륨인지는 OS 마다 다르고 에이전트도 알지 못하므로,
 * 임의로 합치면 사용자가 고른 대상이 화면에서 사라진다. 정확한 종합이 필요하면
 * 에이전트 설정에서 마운트를 골라 두는 것이 옳은 해법이다.
 */
export function sumStorage(snapshot: SysMetricsSnapshot): StorageTotals {
  const summed = sumInstances(snapshot.storage);
  const usedBytes = summed.used_bytes ?? 0;
  const freeBytes = summed.free_bytes ?? 0;
  const totalBytes = summed.total_bytes ?? 0;

  return {
    usedBytes,
    freeBytes,
    totalBytes,
    usagePercent: totalBytes > 0 ? (usedBytes / totalBytes) * 100 : 0,
  };
}

// --- rate ---

/** rate 계산의 기준점 — 누적값과 그 값을 읽은 시각. */
export interface CounterPoint {
  value: number;
  /** 표본 시각 (epoch ms) */
  at: number;
}

/**
 * 누적 카운터 두 점에서 단위시간당 증가량을 만든다. 만들 수 없으면 null.
 *
 * null 을 돌려주는 세 경우:
 *   - 기준점이 없다 (첫 표본)
 *   - 카운터가 뒤로 갔다 (재부팅·인터페이스 재설정). 그대로 두면 거대한 음수
 *     스파이크가 차트를 망가뜨린다.
 *   - 경과 시간이 0 이하다 (같은 표본을 두 번 읽었거나 시계가 뒤로 갔다). 0 으로
 *     나누면 Infinity 가 나온다.
 *
 * null 은 "그리지 않는다"는 뜻이지 0 이 아니다. 0 으로 대체하면 재부팅 직후 트래픽이
 * 끊긴 것처럼 보인다.
 */
export function deltaRate(
  prev: CounterPoint | null,
  next: CounterPoint,
  unit: UnitTime,
): number | null {
  if (!prev) return null;
  if (next.value < prev.value) return null;

  const elapsedSec = (next.at - prev.at) / 1_000;
  if (elapsedSec <= 0) return null;

  return ((next.value - prev.value) / elapsedSec) * UNIT_TIME_SECONDS[unit];
}

/**
 * 두 스냅샷이 같은 표본인지.
 *
 * 패널 폴링 주기가 에이전트 표본 주기보다 짧으면 같은 표본을 여러 번 받는다. 그때
 * 새 점을 추가하면 경과 0 짜리 구간이 생겨 rate 가 계단처럼 끊긴다.
 */
export function isSameSample(
  prev: SysMetricsSnapshot | null,
  next: SysMetricsSnapshot,
): boolean {
  if (!prev || prev.collectedAt === null || next.collectedAt === null) return false;
  return prev.collectedAt === next.collectedAt;
}

/**
 * 인스턴스 그룹에서 표시 대상 이름을 고른다.
 *
 * 선택이 비어 있으면 "종합"을 뜻하므로 빈 배열을 돌려준다(호출부가 합산을 그린다).
 * 선택한 이름 중 스냅샷에 없는 것은 조용히 건너뛴다 — 인터페이스가 사라져도 나머지
 * 시리즈는 계속 그려야 한다.
 */
export function resolveTargets(
  group: InstanceGroup | undefined,
  selected: string[],
): string[] {
  if (!group || selected.length === 0) return [];
  return selected.filter((name) => group[name] !== undefined);
}
