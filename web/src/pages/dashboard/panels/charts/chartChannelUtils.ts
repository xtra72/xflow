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

/** x축 tick 에 쓸 짧은 포맷 — 범위에 따라 초/분/시 자동 결정 */
export function formatTimeShort(ms: number): string {
  const d = new Date(ms);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

/**
 * 시간 범위(ms) 에 맞는 "깔끔한" 틱 간격(ms)을 선택한다.
 * 구간 크기에 따라 사람이 읽기 좋은 1/5/10/15/30초·분·시 단위를 반환.
 */
const NICE_INTERVALS_MS: number[] = [
  1_000,        // 1초
  5_000,        // 5초
  10_000,       // 10초
  30_000,       // 30초
  60_000,       // 1분
  5 * 60_000,   // 5분
  10 * 60_000,  // 10분
  15 * 60_000,  // 15분
  30 * 60_000,  // 30분
  3600_000,     // 1시간
  6 * 3600_000, // 6시간
  12 * 3600_000,// 12시간
  86400_000,    // 1일
];

/**
 * rangeMs 에 대해 틱 개수가 [minTicks, maxTicks] 안에 들어오는
 * 가장 큰 (= 가장 성긴) nice 간격을 선택한다.
 */
function pickNiceInterval(
  rangeMs: number,
  minTicks: number,
  maxTicks: number,
): number {
  let best = NICE_INTERVALS_MS[NICE_INTERVALS_MS.length - 1]!;
  for (const iv of NICE_INTERVALS_MS) {
    const count = rangeMs / iv;
    if (count >= minTicks - 1 && count <= maxTicks + 1) {
      best = iv;
    }
  }
  return best;
}

/**
 * [startMs, endMs] 구간에 대해 깔끔한 틱 값 배열을 생성한다.
 * 각 틱은 해당 간격의 배수 위치에 정렬된다 (예: 5초 간격이면 10:00:00, 10:00:05, ...).
 * @param minTicks 최소 틱 개수 (기본 5)
 * @param maxTicks 최대 틱 개수 (기본 10)
 */
export function computeNiceTimeTicks(
  startMs: number,
  endMs: number,
  minTicks = 5,
  maxTicks = 10,
): number[] {
  const range = endMs - startMs;
  if (range <= 0 || !Number.isFinite(range)) return [];
  const interval = pickNiceInterval(range, minTicks, maxTicks);
  const first = Math.ceil(startMs / interval) * interval;
  const ticks: number[] = [];
  for (let t = first; t <= endMs; t += interval) {
    ticks.push(t);
  }
  return ticks;
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

/**
 * 라인 차트 전용 값 변환.
 *
 * 라인 차트 데이터 소스 규칙:
 *   - number (int/float 혼합) → 그대로 표시
 *   - boolean → 1(true)/0(false) 로 그린다 (표시는 true/false — 렌더 측에서 처리)
 *   - string 을 포함한 그 외 타입 → NaN (제외; connectNulls 로 이어지거나 라인에서 빠진다)
 *
 * toNumber 와 달리 숫자 모양 문자열("3.14")도 제외한다(스트링 타입 제외). bar/pie/stat
 * 등 다른 차트의 집계 의미를 바꾸지 않도록 라인 차트에서만 사용한다.
 */
export function toLineValue(value: unknown): number {
  if (typeof value === 'number') return Number.isFinite(value) ? value : NaN;
  if (typeof value === 'boolean') return value ? 1 : 0;
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
