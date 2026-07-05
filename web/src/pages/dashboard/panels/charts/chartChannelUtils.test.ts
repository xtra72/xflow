// chartChannelUtils 테스트 (순수 함수).

import { describe, it, expect } from 'vitest';

import {
  aggregateByLabel,
  aggregateByTimeBin,
  computeNiceTimeTicks,
  formatNumber,
  formatTimeShort,
  formatTimestamp,
  pickThresholdColor,
  toNumber,
  toLineValue,
} from './chartChannelUtils';
import type { ChartEntry } from './chartChannelTypes';

describe('formatTimestamp', () => {
  it('epoch ms 를 YYYY-MM-DD HH:mm:ss.SSS 로 포맷', () => {
    const d = new Date(2026, 3, 16, 14, 30, 0, 123);
    const ms = d.getTime();
    const formatted = formatTimestamp(ms);
    expect(formatted).toBe('2026-04-16 14:30:00.123');
  });
});

describe('formatTimeShort', () => {
  it('HH:mm:ss', () => {
    const d = new Date(2026, 0, 1, 9, 5, 3);
    expect(formatTimeShort(d.getTime())).toBe('09:05:03');
  });
});

describe('toNumber', () => {
  it('숫자 그대로', () => {
    expect(toNumber(42)).toBe(42);
  });
  it('숫자 문자열 파싱', () => {
    expect(toNumber('3.14')).toBeCloseTo(3.14);
  });
  it('숫자 아닌 문자열은 NaN', () => {
    expect(Number.isNaN(toNumber('abc'))).toBe(true);
  });
  it('객체는 NaN', () => {
    expect(Number.isNaN(toNumber({ a: 1 }))).toBe(true);
  });
});

describe('toLineValue', () => {
  it('int/float 은 그대로 (혼합 표시)', () => {
    expect(toLineValue(42)).toBe(42);
    expect(toLineValue(3.14)).toBeCloseTo(3.14);
    expect(toLineValue(-0.5)).toBe(-0.5);
  });
  it('boolean 은 1/0 으로', () => {
    expect(toLineValue(true)).toBe(1);
    expect(toLineValue(false)).toBe(0);
  });
  it('문자열은 숫자 모양이어도 제외(NaN)', () => {
    expect(Number.isNaN(toLineValue('3.14'))).toBe(true);
    expect(Number.isNaN(toLineValue('cool'))).toBe(true);
    expect(Number.isNaN(toLineValue(''))).toBe(true);
  });
  it('null/undefined/객체/비유한 숫자는 NaN', () => {
    expect(Number.isNaN(toLineValue(null))).toBe(true);
    expect(Number.isNaN(toLineValue(undefined))).toBe(true);
    expect(Number.isNaN(toLineValue({ a: 1 }))).toBe(true);
    expect(Number.isNaN(toLineValue(Number.POSITIVE_INFINITY))).toBe(true);
    expect(Number.isNaN(toLineValue(NaN))).toBe(true);
  });
});

describe('formatNumber', () => {
  it('decimals 지정', () => {
    expect(formatNumber(3.14159, 2)).toBe('3.14');
  });
  it('decimals 미지정 시 원본', () => {
    expect(formatNumber(3.14)).toBe('3.14');
  });
  it('NaN/Infinity 는 —', () => {
    expect(formatNumber(NaN)).toBe('—');
    expect(formatNumber(Infinity)).toBe('—');
  });
});

describe('aggregateByLabel', () => {
  const entries: ChartEntry[] = [
    { timestamp: 1, value: 10, labels: { name: 'A' } },
    { timestamp: 2, value: 20, labels: { name: 'A' } },
    { timestamp: 3, value: 5, labels: { name: 'B' } },
  ];

  it('sum 집계', () => {
    const out = aggregateByLabel(entries, 'labels.name', 'value', 'sum');
    expect(out).toHaveLength(2);
    const a = out.find((r) => r.label === 'A');
    const b = out.find((r) => r.label === 'B');
    expect(a?.value).toBe(30);
    expect(b?.value).toBe(5);
  });

  it('count 집계', () => {
    const out = aggregateByLabel(entries, 'labels.name', 'value', 'count');
    expect(out.find((r) => r.label === 'A')?.value).toBe(2);
    expect(out.find((r) => r.label === 'B')?.value).toBe(1);
  });

  it('avg 집계', () => {
    const out = aggregateByLabel(entries, 'labels.name', 'value', 'avg');
    expect(out.find((r) => r.label === 'A')?.value).toBe(15);
    expect(out.find((r) => r.label === 'B')?.value).toBe(5);
  });

  it('label 이 없으면 "unknown"', () => {
    const e: ChartEntry[] = [{ timestamp: 1, value: 1 }];
    const out = aggregateByLabel(e, 'labels.name', 'value', 'sum');
    expect(out[0]!.label).toBe('unknown');
  });
});

describe('aggregateByTimeBin', () => {
  it('60초 bin 에서 같은 bin 의 값 합계', () => {
    const entries: ChartEntry[] = [
      { timestamp: 1000, value: 10 },
      { timestamp: 30_000, value: 20 },
      { timestamp: 61_000, value: 5 },
    ];
    const out = aggregateByTimeBin(entries, 'value', 60, 'sum');
    expect(out).toHaveLength(2);
    expect(out[0]!.value).toBe(30);
    expect(out[1]!.value).toBe(5);
  });

  it('binSec <= 0 면 빈 배열', () => {
    expect(aggregateByTimeBin([{ timestamp: 1, value: 1 }], 'value', 0, 'sum')).toEqual([]);
  });

  it('count 모드', () => {
    const entries: ChartEntry[] = [
      { timestamp: 0, value: 1 },
      { timestamp: 100, value: 2 },
      { timestamp: 60_000, value: 3 },
    ];
    const out = aggregateByTimeBin(entries, 'value', 60, 'count');
    expect(out[0]!.value).toBe(2);
    expect(out[1]!.value).toBe(1);
  });
});

describe('pickThresholdColor', () => {
  const rules = [
    { min: 0, color: 'green' },
    { min: 50, color: 'yellow' },
    { min: 80, color: 'red' },
  ];

  it('value 가 가장 높은 매칭 rule 색상', () => {
    expect(pickThresholdColor(85, rules)).toBe('red');
    expect(pickThresholdColor(60, rules)).toBe('yellow');
    expect(pickThresholdColor(10, rules)).toBe('green');
  });

  it('모든 rule 보다 낮으면 undefined', () => {
    const r = [{ min: 50, color: 'red' }];
    expect(pickThresholdColor(10, r)).toBeUndefined();
  });

  it('rules 없거나 빈배열이면 undefined', () => {
    expect(pickThresholdColor(10, undefined)).toBeUndefined();
    expect(pickThresholdColor(10, [])).toBeUndefined();
  });
});

describe('computeNiceTimeTicks', () => {
  it('30초 범위 → 5~10개 틱', () => {
    const ticks = computeNiceTimeTicks(0, 30_000);
    expect(ticks.length).toBeGreaterThanOrEqual(5);
    expect(ticks.length).toBeLessThanOrEqual(10);
  });

  it('3분 범위 → 5~10개 틱, 균등 간격', () => {
    const ticks = computeNiceTimeTicks(0, 3 * 60_000);
    expect(ticks.length).toBeGreaterThanOrEqual(5);
    expect(ticks.length).toBeLessThanOrEqual(10);
    const interval = ticks[1]! - ticks[0]!;
    for (let i = 2; i < ticks.length; i++) {
      expect(ticks[i]! - ticks[i - 1]!).toBe(interval);
    }
  });

  it('10분 범위 → 1분 또는 2분 간격', () => {
    const ticks = computeNiceTimeTicks(0, 10 * 60_000);
    const interval = ticks[1]! - ticks[0]!;
    expect(interval).toBeGreaterThanOrEqual(60_000);
    expect(ticks.length).toBeGreaterThanOrEqual(5);
  });

  it('1시간 범위 → 5~10분 간격', () => {
    const ticks = computeNiceTimeTicks(0, 3600_000);
    expect(ticks.length).toBeGreaterThanOrEqual(5);
    expect(ticks.length).toBeLessThanOrEqual(10);
  });

  it('틱이 간격의 배수 위치에 정렬', () => {
    const ticks = computeNiceTimeTicks(7_500, 37_500);
    const interval = ticks[1]! - ticks[0]!;
    for (const t of ticks) {
      expect(t % interval).toBe(0);
    }
    expect(ticks[0]).toBeGreaterThanOrEqual(7_500);
    expect(ticks[ticks.length - 1]).toBeLessThanOrEqual(37_500);
  });

  it('범위 0 이하 → 빈 배열', () => {
    expect(computeNiceTimeTicks(100, 100)).toEqual([]);
    expect(computeNiceTimeTicks(200, 100)).toEqual([]);
  });

  it('커스텀 min/max ticks 지정', () => {
    const ticks = computeNiceTimeTicks(0, 60_000, 3, 8);
    expect(ticks.length).toBeGreaterThanOrEqual(3);
    expect(ticks.length).toBeLessThanOrEqual(8);
  });
});
