// SPEC-WEB-007 v0.1.0 (M9) — formatUptime 유틸 테스트.
//
// uptime_seconds(초) → "3d 4h 12m" 가독 형식 변환 사양 테스트 (AC-6).
//
// @spec SPEC-WEB-007 v0.1.0 (M9, AC-6)

import { describe, expect, it } from 'vitest';

import { formatUptime } from './formatUptime';

describe('formatUptime', () => {
  it('일/시/분 모두 있는 경우 "3d 4h 12m" 형식으로 표시한다', () => {
    // 3d 4h 12m = 3*86400 + 4*3600 + 12*60 = 259200 + 14400 + 720 = 274320
    expect(formatUptime(274320)).toBe('3d 4h 12m');
  });

  it('시/분만 있는 경우 일(d)을 생략한다', () => {
    // 4h 12m = 14400 + 720 = 15120
    expect(formatUptime(15120)).toBe('4h 12m');
  });

  it('분만 있는 경우 시/일을 생략한다', () => {
    // 12m = 720
    expect(formatUptime(720)).toBe('12m');
  });

  it('1분 미만(초 단위)은 "<1m" 로 표시한다', () => {
    expect(formatUptime(45)).toBe('<1m');
    expect(formatUptime(0)).toBe('<1m');
  });

  it('정확히 1일이면 "1d 0h 0m" 로 표시한다', () => {
    expect(formatUptime(86400)).toBe('1d 0h 0m');
  });

  it('일이 있으면 시/분이 0이어도 모두 표기한다 (자릿수 안정성)', () => {
    // 2d 0h 5m = 172800 + 300
    expect(formatUptime(173100)).toBe('2d 0h 5m');
  });

  it('시가 있고 일이 없으면 분이 0이어도 표기한다', () => {
    // 5h 0m = 18000
    expect(formatUptime(18000)).toBe('5h 0m');
  });

  it('음수/NaN 등 비정상 입력은 "-" 로 안전 처리한다', () => {
    expect(formatUptime(-1)).toBe('-');
    expect(formatUptime(Number.NaN)).toBe('-');
  });

  it('소수점 초는 내림하여 처리한다', () => {
    // 119.9s → 1m 59s → 분 단위로 1m
    expect(formatUptime(119.9)).toBe('1m');
  });
});
