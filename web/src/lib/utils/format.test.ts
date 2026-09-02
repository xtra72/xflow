// epoch milliseconds 포맷터 테스트.
// formatEpochMs: 절대 시각(이력 테이블). formatRelativeEpochMs: 상대 시각(측정치 신선도).

import { describe, expect, it } from 'vitest';

import { formatEpochMs, formatModulation, formatRelativeEpochMs } from './format';

describe('formatEpochMs', () => {
  it('유효한 epoch ms 를 로컬 시간 문자열로 변환한다', () => {
    const out = formatEpochMs(Date.UTC(2026, 7, 13, 5, 32, 1));
    // 로케일 포맷 세부(구분자/공백)는 런타임에 따라 다르므로 구성요소로 검증한다.
    expect(out).not.toBe('-');
    expect(out).toContain('2026');
    expect(out).toMatch(/\d{2}:\d{2}:\d{2}/);
  });

  it('유효하지 않은 값은 "-" 를 반환한다', () => {
    expect(formatEpochMs(0)).toBe('-');
    expect(formatEpochMs(-1)).toBe('-');
    expect(formatEpochMs(NaN)).toBe('-');
  });
});

describe('formatRelativeEpochMs', () => {
  const now = Date.UTC(2026, 7, 13, 12, 0, 0);

  it('경과 시간을 한국어 상대 표현으로 반환한다', () => {
    expect(formatRelativeEpochMs(now - 3 * 60_000, now)).toBe('3분 전');
    expect(formatRelativeEpochMs(now - 2 * 3_600_000, now)).toBe('2시간 전');
    expect(formatRelativeEpochMs(now - 5 * 86_400_000, now)).toBe('5일 전');
  });

  it('1초 미만은 "방금" 으로 표시한다', () => {
    expect(formatRelativeEpochMs(now, now)).toBe('방금');
    expect(formatRelativeEpochMs(now - 300, now)).toBe('방금');
  });

  it('미래 시각도 처리한다(노드 간 시계 오차 대비)', () => {
    expect(formatRelativeEpochMs(now + 5 * 60_000, now)).toBe('5분 후');
  });

  it('유효하지 않은 값은 "-" 를 반환한다', () => {
    expect(formatRelativeEpochMs(0, now)).toBe('-');
    expect(formatRelativeEpochMs(-1, now)).toBe('-');
    expect(formatRelativeEpochMs(NaN, now)).toBe('-');
  });
});

describe('formatModulation', () => {
  it('SF 와 대역폭을 함께 표기한다', () => {
    expect(formatModulation(7, 125_000)).toBe('SF7 / 125 kHz');
    expect(formatModulation(12, 250_000)).toBe('SF12 / 250 kHz');
  });

  it('대역폭이 없으면(txInfo 누락) SF 만 표기한다', () => {
    expect(formatModulation(7, 0)).toBe('SF7');
  });

  it('SF 가 없으면 대역폭만 표기한다', () => {
    expect(formatModulation(0, 125_000)).toBe('125 kHz');
  });

  it('둘 다 없으면 "-" 를 반환한다', () => {
    expect(formatModulation(0, 0)).toBe('-');
    expect(formatModulation(NaN, NaN)).toBe('-');
  });
});
