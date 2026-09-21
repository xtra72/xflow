// 유휴 판정 순수 층의 계약 (@SPEC:SPEC-AUTH-IDLE-001).

import { describe, expect, it } from 'vitest';

import {
  DEFAULT_IDLE_SETTING,
  MAX_TIMEOUT_MINUTES,
  MIN_TIMEOUT_MINUTES,
  clampTimeoutMinutes,
  evaluateIdle,
  normalizeIdleSetting,
  warnLeadMs,
} from './idlePolicy';

describe('normalizeIdleSetting — 불투명 저장소에서 온 값을 좁힌다', () => {
  it('저장된 적 없는 값(undefined/null)은 기본 정책이 된다', () => {
    expect(normalizeIdleSetting(undefined)).toEqual(DEFAULT_IDLE_SETTING);
    expect(normalizeIdleSetting(null)).toEqual(DEFAULT_IDLE_SETTING);
    expect(normalizeIdleSetting('10분')).toEqual(DEFAULT_IDLE_SETTING);
  });

  it('기본값은 10분·켜짐이다 — 사용자가 말한 그대로', () => {
    expect(DEFAULT_IDLE_SETTING).toEqual({ enabled: true, timeoutMinutes: 10 });
  });

  it('한 필드가 깨져도 다른 필드는 살린다', () => {
    // enabled 는 멀쩡하고 시간만 쓰레기 — 시간만 기본값으로 돌아간다.
    expect(normalizeIdleSetting({ enabled: false, timeoutMinutes: 'abc' })).toEqual({
      enabled: false,
      timeoutMinutes: 10,
    });
    // 반대 방향.
    expect(normalizeIdleSetting({ enabled: 'yes', timeoutMinutes: 30 })).toEqual({
      enabled: true,
      timeoutMinutes: 30,
    });
  });

  it('범위를 벗어난 시간은 잘라서 받는다', () => {
    expect(normalizeIdleSetting({ enabled: true, timeoutMinutes: 0 }).timeoutMinutes).toBe(
      MIN_TIMEOUT_MINUTES,
    );
    expect(normalizeIdleSetting({ enabled: true, timeoutMinutes: 99999 }).timeoutMinutes).toBe(
      MAX_TIMEOUT_MINUTES,
    );
    expect(normalizeIdleSetting({ enabled: true, timeoutMinutes: 10.9 }).timeoutMinutes).toBe(10);
  });

  it('수가 아닌 값은 하한이 아니라 기본 정책으로 돌아간다', () => {
    // NaN 은 범위 비교가 둘 다 거짓이라 그냥 통과해 타이머를 조용히 무력화한다.
    // 하한(1분)으로 떨어뜨리면 관리자가 정한 적 없는 가장 공격적인 정책이 된다.
    expect(clampTimeoutMinutes(Number.NaN)).toBe(DEFAULT_IDLE_SETTING.timeoutMinutes);
    expect(clampTimeoutMinutes(Number.POSITIVE_INFINITY)).toBe(
      DEFAULT_IDLE_SETTING.timeoutMinutes,
    );
  });
});

describe('warnLeadMs — 경고가 한도를 집어삼키지 않는다', () => {
  it('넉넉한 한도에서는 1분 전에 경고한다', () => {
    expect(warnLeadMs(10)).toBe(60_000);
    expect(warnLeadMs(3)).toBe(60_000);
  });

  it('한도가 2분이면 1분 경고가 절반이라 그대로 두고, 1분이면 30초로 줄인다', () => {
    expect(warnLeadMs(2)).toBe(60_000);
    expect(warnLeadMs(1)).toBe(30_000);
  });
});

describe('evaluateIdle — 국면 판정', () => {
  const base = { timeoutMinutes: 10 };

  it('방금 활동했으면 active 이고 남은 시간은 한도 전부다', () => {
    const r = evaluateIdle({ ...base, now: 1_000_000, lastActivityAt: 1_000_000 });
    expect(r.phase).toBe('active');
    expect(r.remainingMs).toBe(600_000);
  });

  it('경고 구간(남은 1분 이하)에 들어가면 warning 이다', () => {
    const now = 1_000_000;
    expect(evaluateIdle({ ...base, now, lastActivityAt: now - 539_000 }).phase).toBe('active');
    expect(evaluateIdle({ ...base, now, lastActivityAt: now - 540_000 }).phase).toBe('warning');
    expect(evaluateIdle({ ...base, now, lastActivityAt: now - 599_000 }).phase).toBe('warning');
  });

  it('한도를 넘기면 expired 이고 남은 시간은 0이다', () => {
    const now = 1_000_000;
    const r = evaluateIdle({ ...base, now, lastActivityAt: now - 600_000 });
    expect(r.phase).toBe('expired');
    expect(r.remainingMs).toBe(0);
  });

  it('절전에서 몇 시간 만에 깨어나면 곧바로 expired 다 — 자리를 비운 것이 맞다', () => {
    const now = 1_000_000_000;
    expect(evaluateIdle({ ...base, now, lastActivityAt: now - 3 * 3600_000 }).phase).toBe(
      'expired',
    );
  });

  it('마지막 활동이 미래로 적혀 있어도(시계 역행) 즉시 로그아웃하지 않는다', () => {
    const now = 1_000_000;
    const r = evaluateIdle({ ...base, now, lastActivityAt: now + 120_000 });
    expect(r.phase).toBe('active');
    expect(r.remainingMs).toBeGreaterThan(600_000);
  });
});
