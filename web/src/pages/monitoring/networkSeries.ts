// 네트워크 통계 → 시계열 변환.
//
// 백엔드는 부팅 이후 누적 카운터만 준다(상태를 들고 있지 않기 위해서다). 화면은
// 두 가지로 본다:
//   - rate 계열: 연속한 두 표본의 차이를 단위시간으로 환산한 증가량
//   - 누적 계열: 카운터 원값 추이
//
// 인터페이스별로 각각 계열을 유지해 한 차트에 여러 선을 겹쳐 그린다.

import type { MetricDataPoint } from './MetricsChart';
import type { NetworkInterfaceStat, NetworkStats } from '@/services/api/monitorService';

/** 네트워크 채널 키 (monitoringLayout.NetworkItemKey 와 같은 어휘) */
export type NetworkChannel =
  | 'rxBytes'
  | 'txBytes'
  | 'rxPackets'
  | 'txPackets'
  | 'rxBytesTotal'
  | 'txBytesTotal'
  | 'rxPacketsTotal'
  | 'txPacketsTotal';

/** 전체 채널 목록 (렌더 순서) */
export const NETWORK_CHANNELS: NetworkChannel[] = [
  'rxBytes',
  'txBytes',
  'rxPackets',
  'txPackets',
  'rxBytesTotal',
  'txBytesTotal',
  'rxPacketsTotal',
  'txPacketsTotal',
];

/** 채널 → 누적 카운터 필드 */
const CHANNEL_FIELD: Record<NetworkChannel, keyof NetworkInterfaceStat> = {
  rxBytes: 'bytes_recv',
  txBytes: 'bytes_sent',
  rxPackets: 'packets_recv',
  txPackets: 'packets_sent',
  rxBytesTotal: 'bytes_recv',
  txBytesTotal: 'bytes_sent',
  rxPacketsTotal: 'packets_recv',
  txPacketsTotal: 'packets_sent',
};

/** 누적 계열인지 (아니면 단위시간당 증가량) */
export function isTotalChannel(channel: NetworkChannel): boolean {
  return channel.endsWith('Total');
}

/** 바이트 계열인지 (아니면 패킷) */
export function isByteChannel(channel: NetworkChannel): boolean {
  return channel.startsWith('rxBytes') || channel.startsWith('txBytes');
}

/** 메트릭 항목 키가 네트워크 채널인지 판별한다. */
export function isNetworkChannel(key: string): key is NetworkChannel {
  return (NETWORK_CHANNELS as string[]).includes(key);
}

// --- 단위시간 ---

/** rate 계열의 단위시간 */
export type UnitTime = 'sec' | 'min' | 'hour';

/** 단위시간 → 초 */
export const UNIT_TIME_SECONDS: Record<UnitTime, number> = {
  sec: 1,
  min: 60,
  hour: 3_600,
};

/** 단위시간 접미사 (`B/s`, `B/min`, `B/h`) */
export const UNIT_TIME_SUFFIX: Record<UnitTime, string> = {
  sec: '/s',
  min: '/min',
  hour: '/h',
};

/** 저장값을 유효한 단위시간으로 정규화한다. */
export function normalizeUnitTime(raw: unknown): UnitTime {
  return raw === 'min' || raw === 'hour' ? raw : 'sec';
}

// --- 계열 ---

/** 인터페이스 이름 → 채널 → 시계열 */
export type NetworkSeries = Record<string, Partial<Record<NetworkChannel, MetricDataPoint[]>>>;

/** 합산 항목의 이름 (백엔드와 동일) */
export const TOTAL_INTERFACE = 'total';

/** 직전 표본 — rate 계산의 기준점 */
export interface NetworkSample {
  stat: NetworkInterfaceStat;
  /** 표본을 받은 시각 (epoch ms) */
  at: number;
}

/**
 * 응답에서 인터페이스 이름 → 카운터 맵을 만든다 (합산 포함).
 *
 * 합산은 `total` 이라는 이름으로 함께 담아, 선택 목록에서 다른 인터페이스와
 * 똑같이 다룰 수 있게 한다.
 */
export function indexInterfaces(stats: NetworkStats): Record<string, NetworkInterfaceStat> {
  const map: Record<string, NetworkInterfaceStat> = { [TOTAL_INTERFACE]: stats.total };
  for (const iface of stats.interfaces) map[iface.name] = iface;
  return map;
}

/**
 * 두 표본에서 채널 값을 만든다. 만들 수 없으면 null.
 *
 * 누적 계열은 직전 표본이 필요 없다 — 카운터 원값을 그대로 쓴다.
 * rate 계열은 차이를 경과 시간으로 나눈 뒤 단위시간을 곱한다. 카운터가 뒤로 가면
 * (재부팅·인터페이스 재설정) null 을 돌려준다 — 그대로 두면 거대한 음수 스파이크가
 * 차트를 망가뜨린다.
 */
export function toChannelValue(
  prev: NetworkSample | null,
  next: NetworkSample,
  channel: NetworkChannel,
  unit: UnitTime,
): number | null {
  const field = CHANNEL_FIELD[channel];
  const after = next.stat[field];
  if (typeof after !== 'number') return null;

  if (isTotalChannel(channel)) return after;

  if (!prev) return null;
  const before = prev.stat[field];
  if (typeof before !== 'number') return null;
  if (after < before) return null;

  const elapsedSec = (next.at - prev.at) / 1_000;
  if (elapsedSec <= 0) return null;

  return ((after - before) / elapsedSec) * UNIT_TIME_SECONDS[unit];
}

/** 시계열에 포인트를 덧붙이고 상한을 적용한다. */
export function appendPoint(
  prev: MetricDataPoint[],
  point: MetricDataPoint,
  maxPoints: number,
): MetricDataPoint[] {
  const next = [...prev, point];
  return next.length > maxPoints ? next.slice(next.length - maxPoints) : next;
}

// --- 표기 ---

/** 바이트를 단위시간 접미사와 함께 접어 표기한다. */
export function formatBytes(value: number, unit?: UnitTime): string {
  if (!Number.isFinite(value) || value < 0) return '-';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let v = value;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i += 1;
  }
  const suffix = unit ? UNIT_TIME_SUFFIX[unit] : '';
  // 단위를 접은 뒤에는 소수 1자리면 충분하고, B 는 정수가 자연스럽다.
  return `${i === 0 ? Math.round(v) : v.toFixed(1)} ${units[i]}${suffix}`;
}

/** 패킷 수를 단위시간 접미사와 함께 접어 표기한다. */
export function formatPackets(value: number, unit?: UnitTime): string {
  if (!Number.isFinite(value) || value < 0) return '-';
  const suffix = unit ? UNIT_TIME_SUFFIX[unit] : '';
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M p${suffix}`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}k p${suffix}`;
  return `${Math.round(value)} p${suffix}`;
}

/** 채널 값 표기 함수를 고른다. */
export function channelFormatter(
  channel: NetworkChannel,
  unit: UnitTime,
): (value: number) => string {
  // 누적 계열에는 단위시간 접미사를 붙이지 않는다 — 총량이지 비율이 아니다.
  const applied = isTotalChannel(channel) ? undefined : unit;
  return isByteChannel(channel)
    ? (v: number) => formatBytes(v, applied)
    : (v: number) => formatPackets(v, applied);
}

/** 인터페이스별 선 색 — 목록 순서에 따라 순환한다. */
const LINE_COLORS = [
  '#0ea5e9',
  '#f97316',
  '#10b981',
  '#8b5cf6',
  '#ef4444',
  '#f59e0b',
  '#06b6d4',
  '#ec4899',
];

/** 인터페이스 이름 목록에서 이름 → 색 맵을 만든다. */
export function interfaceColors(names: string[]): Record<string, string> {
  const map: Record<string, string> = {};
  names.forEach((name, i) => {
    map[name] = LINE_COLORS[i % LINE_COLORS.length]!;
  });
  return map;
}
