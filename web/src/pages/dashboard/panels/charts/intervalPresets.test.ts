// 인터벌 프리셋/표기 — Store · TSDB 공용 눈금.

import { describe, expect, it } from 'vitest';

import { INTERVAL_PRESETS_MS, formatIntervalMs, isIntervalPreset } from './intervalPresets';

describe('formatIntervalMs — 사람이 읽는 눈금', () => {
  it('초/분/시/일 단위로 가장 큰 단위를 고른다', () => {
    expect(formatIntervalMs(10_000)).toBe('10s');
    expect(formatIntervalMs(60_000)).toBe('1m');
    expect(formatIntervalMs(300_000)).toBe('5m');
    expect(formatIntervalMs(3_600_000)).toBe('1h');
    expect(formatIntervalMs(21_600_000)).toBe('6h');
    expect(formatIntervalMs(86_400_000)).toBe('1d');
  });

  it('단위로 나누어떨어지지 않으면 초로 반올림한다', () => {
    expect(formatIntervalMs(45_000)).toBe('45s');
    expect(formatIntervalMs(90_000)).toBe('90s');
  });
});

describe('isIntervalPreset — 직접 입력 경로 판정', () => {
  it('프리셋 목록의 값은 true', () => {
    for (const ms of INTERVAL_PRESETS_MS) {
      expect(isIntervalPreset(ms)).toBe(true);
    }
  });

  it('목록에 없는 값은 false (직접 입력칸이 열린다)', () => {
    expect(isIntervalPreset(45_000)).toBe(false);
    expect(isIntervalPreset(0)).toBe(false);
  });
});

describe('프리셋 목록 자체', () => {
  it('오름차순이고 중복이 없다', () => {
    const arr = [...INTERVAL_PRESETS_MS];
    expect(arr).toEqual([...arr].sort((a, b) => a - b));
    expect(new Set(arr).size).toBe(arr.length);
  });

  it('Store 기본값(1분 버킷)이 목록에 있다 — 기본 패널이 "직접 입력" 으로 열리지 않는다', () => {
    expect(isIntervalPreset(60_000)).toBe(true);
  });
});
