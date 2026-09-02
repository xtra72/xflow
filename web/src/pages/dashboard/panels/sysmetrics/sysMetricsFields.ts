// 시스템 지표의 값 카탈로그.
//
// 항목의 단위는 **값 하나**다. 예전에는 그룹 하나(cpu / memory / disk_io / network)가
// 항목이어서 한 타일에 여러 값이 묶여 나왔다 — 메모리 사용량만 크게 보거나 네트워크
// 송신만 보는 것이 불가능했고, 값마다 스타일·정렬·색을 정할 자리도 없었다.
//
// 키는 스냅샷의 경로를 그대로 쓴다(`memory.used_bytes`). 저장 경로(storage-write)가
// 쓰는 필드 이름과 같으므로, 패널에서 보던 값을 나중에 시계열로 옮길 때 이름을
// 다시 찾지 않는다.

import type { SysMetricValueKind } from './sysMetricsItemOptions';
import type { SysMetricKind } from './sysMetricsSeries';

/** 값의 표기 방식 */
export type SysMetricFormat = 'percent' | 'bytes' | 'count';

/** 값 하나의 정의 */
export interface SysMetricField {
  /** 항목 키 — 스냅샷 경로 (`memory.used_bytes`) */
  key: string;
  /** 스냅샷의 그룹 */
  group: SysMetricKind;
  /** 그룹 안의 필드 이름 */
  field: string;
  /** 표시 이름 i18n 키 (설정 UI 표시용). */
  labelKey: string;
  /** 값 성격 — 고를 수 있는 스타일을 가른다 */
  kind: SysMetricValueKind;
  format: SysMetricFormat;
  /**
   * 누적 카운터인가.
   *
   * true 면 두 표본의 차이를 단위시간당 증가량으로 환산해 보여준다. 부팅 이후
   * 누적값을 그대로 보여 주면 언제나 커지기만 하는 수라 읽을 것이 없다.
   */
  rate: boolean;
}

/** 시스템 패널이 보여줄 수 있는 값 전체 (설정 UI 표시 순서) */
export const SYSTEM_FIELDS: readonly SysMetricField[] = [
  { key: 'cpu.usage_percent', group: 'cpu', field: 'usage_percent', labelKey: 'sysmetrics.fields.cpuUsage', kind: 'ratio', format: 'percent', rate: false },

  { key: 'memory.usage_percent', group: 'memory', field: 'usage_percent', labelKey: 'sysmetrics.fields.memoryUsage', kind: 'ratio', format: 'percent', rate: false },
  { key: 'memory.used_bytes', group: 'memory', field: 'used_bytes', labelKey: 'sysmetrics.fields.memoryUsed', kind: 'counter', format: 'bytes', rate: false },
  { key: 'memory.available_bytes', group: 'memory', field: 'available_bytes', labelKey: 'sysmetrics.fields.memoryAvailable', kind: 'counter', format: 'bytes', rate: false },
  { key: 'memory.total_bytes', group: 'memory', field: 'total_bytes', labelKey: 'sysmetrics.fields.memoryTotal', kind: 'counter', format: 'bytes', rate: false },

  { key: 'disk_io.read_bytes', group: 'diskIo', field: 'read_bytes', labelKey: 'sysmetrics.fields.diskRead', kind: 'counter', format: 'bytes', rate: true },
  { key: 'disk_io.write_bytes', group: 'diskIo', field: 'write_bytes', labelKey: 'sysmetrics.fields.diskWrite', kind: 'counter', format: 'bytes', rate: true },
  { key: 'disk_io.read_count', group: 'diskIo', field: 'read_count', labelKey: 'sysmetrics.fields.diskReadCount', kind: 'counter', format: 'count', rate: true },
  { key: 'disk_io.write_count', group: 'diskIo', field: 'write_count', labelKey: 'sysmetrics.fields.diskWriteCount', kind: 'counter', format: 'count', rate: true },

  { key: 'network.bytes_recv', group: 'network', field: 'bytes_recv', labelKey: 'sysmetrics.fields.netRecv', kind: 'counter', format: 'bytes', rate: true },
  { key: 'network.bytes_sent', group: 'network', field: 'bytes_sent', labelKey: 'sysmetrics.fields.netSent', kind: 'counter', format: 'bytes', rate: true },
  { key: 'network.packets_recv', group: 'network', field: 'packets_recv', labelKey: 'sysmetrics.fields.netPacketsRecv', kind: 'counter', format: 'count', rate: true },
  { key: 'network.packets_sent', group: 'network', field: 'packets_sent', labelKey: 'sysmetrics.fields.netPacketsSent', kind: 'counter', format: 'count', rate: true },
  { key: 'network.err_in', group: 'network', field: 'err_in', labelKey: 'sysmetrics.fields.netErrIn', kind: 'counter', format: 'count', rate: true },
  { key: 'network.err_out', group: 'network', field: 'err_out', labelKey: 'sysmetrics.fields.netErrOut', kind: 'counter', format: 'count', rate: true },
  { key: 'network.drop_in', group: 'network', field: 'drop_in', labelKey: 'sysmetrics.fields.netDropIn', kind: 'counter', format: 'count', rate: true },
  { key: 'network.drop_out', group: 'network', field: 'drop_out', labelKey: 'sysmetrics.fields.netDropOut', kind: 'counter', format: 'count', rate: true },
];

/**
 * 스토리지 값 — 마운트별 인스턴스 축을 갖는다.
 *
 * `SYSTEM_FIELDS` 와 **분리해 둔다.** 시스템 패널의 항목 목록은 그 상수를 정본으로
 * 쓰므로, 여기 값을 그쪽에 섞으면 시스템 패널 설정에 마운트 값이 나타난다. 차트
 * 패널의 sysmetrics 소스만 두 목록을 합쳐 쓴다(`SYSMETRIC_CHART_FIELDS`).
 */
export const STORAGE_FIELDS: readonly SysMetricField[] = [
  { key: 'storage.usage_percent', group: 'storage', field: 'usage_percent', labelKey: 'sysmetrics.fields.storageUsage', kind: 'ratio', format: 'percent', rate: false },
  { key: 'storage.used_bytes', group: 'storage', field: 'used_bytes', labelKey: 'sysmetrics.fields.storageUsed', kind: 'counter', format: 'bytes', rate: false },
  { key: 'storage.free_bytes', group: 'storage', field: 'free_bytes', labelKey: 'sysmetrics.fields.storageFree', kind: 'counter', format: 'bytes', rate: false },
  { key: 'storage.total_bytes', group: 'storage', field: 'total_bytes', labelKey: 'sysmetrics.fields.storageTotal', kind: 'counter', format: 'bytes', rate: false },
];

/**
 * 차트 패널의 sysmetrics 소스가 그릴 수 있는 값 전체.
 *
 * 시스템 값 + 스토리지 값이다. 차트는 타일 격자가 아니라 시계열이므로 마운트별
 * 용량도 선으로 그릴 수 있고, 그래서 시스템 패널보다 카탈로그가 넓다.
 */
export const SYSMETRIC_CHART_FIELDS: readonly SysMetricField[] = [
  ...SYSTEM_FIELDS,
  ...STORAGE_FIELDS,
];

/**
 * 차트 카탈로그(시스템 + 스토리지)에서 값 정의를 찾는다.
 *
 * `findField` 와 나누어 두는 이유는 `normalizeSystemItems` 때문이다 — 그 함수가
 * 넓은 카탈로그를 보면 시스템 패널 config 에 저장된 스토리지 키가 살아남아 그
 * 패널이 그리지 못하는 항목을 갖게 된다.
 */
export function findChartField(key: string): SysMetricField | undefined {
  return SYSMETRIC_CHART_FIELDS.find((f) => f.key === key);
}

/** 키로 값 정의를 찾는다. */
export function findField(key: string): SysMetricField | undefined {
  return SYSTEM_FIELDS.find((f) => f.key === key);
}

/**
 * 옛 그룹 키 → 그 그룹의 기본 값들.
 *
 * 저장된 대시보드는 `['cpu','memory','diskIo','network']` 를 담고 있다. 값 단위로
 * 바뀌었다고 그 패널을 빈 화면으로 만들 수는 없으므로, 읽는 자리에서 옮겨 준다 —
 * 저장을 강제로 다시 쓰지 않는다(`normalizePanelType` 과 같은 방식).
 */
const LEGACY_GROUP_FIELDS: Record<string, string[]> = {
  cpu: ['cpu.usage_percent'],
  memory: ['memory.usage_percent'],
  diskIo: ['disk_io.read_bytes', 'disk_io.write_bytes'],
  network: ['network.bytes_recv', 'network.bytes_sent'],
};

/** 시스템 패널의 기본 항목 — 옛 네 타일이 보여 주던 대표 값들. */
export const DEFAULT_SYSTEM_FIELDS: string[] = [
  'cpu.usage_percent',
  'memory.usage_percent',
  'disk_io.read_bytes',
  'network.bytes_recv',
];

/**
 * 저장된 항목 목록을 값 키로 옮긴다.
 *
 * 이미 값 키인 것은 그대로 두고, 옛 그룹 키만 그 그룹의 기본 값들로 편다. 모르는
 * 키는 버린다 — 패널 유형을 바꾸며 남은 다른 어휘가 섞일 수 있다.
 */
export function normalizeSystemItems(raw: unknown): string[] | undefined {
  if (!Array.isArray(raw)) return undefined;

  const out: string[] = [];
  for (const item of raw) {
    if (typeof item !== 'string') continue;
    if (findField(item)) {
      out.push(item);
      continue;
    }
    for (const key of LEGACY_GROUP_FIELDS[item] ?? []) {
      // 그룹이 겹쳐 같은 값이 두 번 들어오지 않게 한다.
      if (!out.includes(key)) out.push(key);
    }
  }
  return out;
}
