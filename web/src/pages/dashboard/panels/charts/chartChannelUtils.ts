// 차트 패널 공용 유틸 (순수 함수).
// - 타임스탬프 포맷팅
// - 값 변환 / 집계 헬퍼
// 리액트 컴포넌트는 ConnectionStatusIcon.tsx 에 분리되어 있다.

import { getByPath, type AggFunc, type ChartEntry } from './chartChannelTypes';

/**
 * epoch ms -> "YYYY-MM-DD HH:mm:ss.SSS" (local time)
 * REQ-M4-08 의 table datetime 포맷과 동일.
 */
export function formatTimestamp(ms: number): string {
  const d = new Date(ms);
  const pad = (n: number, w = 2) => String(n).padStart(w, '0');
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ` +
    `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}.` +
    `${pad(d.getMilliseconds(), 3)}`
  );
}

/** x축 tick 에 쓸 짧은 HH:mm:ss 포맷 */
export function formatTimeShort(ms: number): string {
  const d = new Date(ms);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

/** 숫자로 강제 변환 (실패 시 NaN) */
export function toNumber(value: unknown): number {
  if (typeof value === 'number') return value;
  if (typeof value === 'string') {
    const n = Number(value);
    return Number.isFinite(n) ? n : NaN;
  }
  return NaN;
}

/** 소수 자리수 포맷, undefined 면 원본 숫자 */
export function formatNumber(n: number, decimals?: number): string {
  if (!Number.isFinite(n)) return '—';
  return decimals === undefined ? String(n) : n.toFixed(decimals);
}

/** label_field 별로 entries 를 그룹화하고 agg_func 로 집계 */
export function aggregateByLabel(
  entries: ChartEntry[],
  labelField: string,
  valueField: string,
  agg: AggFunc,
): Array<{ label: string; value: number; count: number }> {
  const groups = new Map<string, { sum: number; count: number }>();
  for (const e of entries) {
    const lblRaw = getByPath(e, labelField);
    const label = lblRaw == null ? 'unknown' : String(lblRaw);
    const valRaw = getByPath(e, valueField);
    const val = toNumber(valRaw);
    if (!groups.has(label)) groups.set(label, { sum: 0, count: 0 });
    const g = groups.get(label)!;
    if (Number.isFinite(val)) g.sum += val;
    g.count += 1;
  }

  const result: Array<{ label: string; value: number; count: number }> = [];
  for (const [label, g] of groups) {
    let value = 0;
    if (agg === 'count') value = g.count;
    else if (agg === 'sum') value = g.sum;
    else if (agg === 'avg') value = g.count > 0 ? g.sum / g.count : 0;
    result.push({ label, value, count: g.count });
  }
  return result;
}

/** 시간 bin 별 집계 (bar-chart time_bin 모드용) */
export function aggregateByTimeBin(
  entries: ChartEntry[],
  valueField: string,
  binSec: number,
  agg: AggFunc,
): Array<{ binStart: number; value: number; count: number }> {
  if (binSec <= 0) return [];
  const binMs = binSec * 1000;
  const buckets = new Map<number, { sum: number; count: number }>();
  for (const e of entries) {
    const bin = Math.floor(e.timestamp / binMs) * binMs;
    const val = toNumber(getByPath(e, valueField));
    if (!buckets.has(bin)) buckets.set(bin, { sum: 0, count: 0 });
    const b = buckets.get(bin)!;
    if (Number.isFinite(val)) b.sum += val;
    b.count += 1;
  }
  const out: Array<{ binStart: number; value: number; count: number }> = [];
  for (const [binStart, b] of buckets) {
    let value = 0;
    if (agg === 'count') value = b.count;
    else if (agg === 'sum') value = b.sum;
    else if (agg === 'avg') value = b.count > 0 ? b.sum / b.count : 0;
    out.push({ binStart, value, count: b.count });
  }
  out.sort((a, b) => a.binStart - b.binStart);
  return out;
}

/** stat 패널: threshold rule 중 value 이하인 최대 min 의 color 반환 */
export function pickThresholdColor(
  value: number,
  rules: Array<{ min: number; color: string }> | undefined,
): string | undefined {
  if (!rules || rules.length === 0) return undefined;
  const sorted = [...rules].sort((a, b) => a.min - b.min);
  let picked: { min: number; color: string } | undefined;
  for (const r of sorted) {
    if (value >= r.min) picked = r;
  }
  return picked?.color;
}
