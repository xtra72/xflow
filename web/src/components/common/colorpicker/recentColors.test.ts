// @spec SPEC-COLOR-001 §결정 9 (M4) — AC-09 · 함정 E7
//
// 함정 E7: `localStorage` 는 시험 사이에 살아남는다. 각 시험 앞에서 비운다.
// 저장 실패 폴백은 `setItem` 이 **실제로 던지게** 만들어 재는 것이지 "없다고 치고"
// 재는 것이 아니다.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  pushRecentColor,
  readRecentColors,
  RECENT_COLORS_LIMIT,
  RECENT_COLORS_STORAGE_KEY,
  resetRecentColorsForTest,
  visibleRecentColors,
} from './recentColors';

const BLUE = '#3b82f6';
const RED = '#ef4444';
const BLUE_A50 = '#3b82f680';

beforeEach(() => {
  localStorage.clear();
  resetRecentColorsForTest();
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('MRU 동작', () => {
  it('빈 저장소에서 빈 목록', () => {
    expect(readRecentColors()).toEqual([]);
  });

  it('중복은 추가가 아니라 맨 앞으로 승격이다', () => {
    pushRecentColor(BLUE);
    pushRecentColor(RED);
    expect(pushRecentColor(BLUE)).toEqual([BLUE, RED]);
  });

  it(`${RECENT_COLORS_LIMIT}칸을 넘기면 가장 오래된 것이 밀려난다`, () => {
    const colors = Array.from(
      { length: RECENT_COLORS_LIMIT + 1 },
      (_, i) => `#${i.toString(16).padStart(6, '0')}`,
    );
    let list: string[] = [];
    for (const c of colors) list = pushRecentColor(c);
    expect(list).toHaveLength(RECENT_COLORS_LIMIT);
    expect(list[0]).toBe(colors[RECENT_COLORS_LIMIT]);
    expect(list).not.toContain(colors[0]);
  });

  it('비우기(`undefined`)는 목록을 바꾸지 않는다', () => {
    pushRecentColor(BLUE);
    expect(pushRecentColor(undefined)).toEqual([BLUE]);
  });

  it('정규화를 통과하지 못하는 값은 들어가지 않는다', () => {
    pushRecentColor(BLUE);
    expect(pushRecentColor('var(--x)')).toEqual([BLUE]);
  });

  it('저장할 때 정규화한다 — 대문자·약식이 같은 칸을 두 번 차지하지 않는다', () => {
    pushRecentColor('#3B82F6');
    expect(pushRecentColor('3b82f6')).toEqual([BLUE]);
  });

  it('세션을 넘겨도 남는다 — 모듈 상태를 비우고 다시 읽는다', () => {
    pushRecentColor(BLUE);
    resetRecentColorsForTest();
    expect(readRecentColors()).toEqual([BLUE]);
  });
});

describe('저장소 내용을 관용적으로 읽는다', () => {
  it('배열이 아니면 빈 목록', () => {
    localStorage.setItem(RECENT_COLORS_STORAGE_KEY, JSON.stringify({ a: 1 }));
    expect(readRecentColors()).toEqual([]);
  });

  it('JSON 이 깨졌으면 빈 목록', () => {
    localStorage.setItem(RECENT_COLORS_STORAGE_KEY, '{{{');
    expect(readRecentColors()).toEqual([]);
  });

  it('문자열이 아닌 항목과 못 읽는 색과 중복을 버린다', () => {
    localStorage.setItem(
      RECENT_COLORS_STORAGE_KEY,
      JSON.stringify([BLUE, 42, 'nope', '#3B82F6', RED]),
    );
    expect(readRecentColors()).toEqual([BLUE, RED]);
  });

  it(`저장된 길이가 넘쳐도 ${RECENT_COLORS_LIMIT}칸에서 자른다`, () => {
    const many = Array.from({ length: 20 }, (_, i) => `#${i.toString(16).padStart(6, '0')}`);
    localStorage.setItem(RECENT_COLORS_STORAGE_KEY, JSON.stringify(many));
    expect(readRecentColors()).toHaveLength(RECENT_COLORS_LIMIT);
  });
});

describe('저장소가 막힌 환경', () => {
  it('`setItem` 이 던져도 예외가 밖으로 나오지 않고 세션 안에서는 계속 동작한다', () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('QuotaExceededError');
    });
    expect(() => pushRecentColor(BLUE)).not.toThrow();
    expect(pushRecentColor(RED)).toEqual([RED, BLUE]);
    expect(readRecentColors()).toEqual([RED, BLUE]);
  });

  it('`getItem` 이 던져도 빈 목록으로 떨어진다', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('SecurityError');
    });
    expect(readRecentColors()).toEqual([]);
  });
});

describe('`alpha` 가 꺼진 자리에서는 8자리를 거른다', () => {
  it('8자리 항목이 격자에 없다', () => {
    pushRecentColor(BLUE_A50);
    pushRecentColor(BLUE);
    expect(visibleRecentColors({ alpha: false })).toEqual([BLUE]);
  });

  it('`alpha` 가 켜지면 있다', () => {
    pushRecentColor(BLUE_A50);
    pushRecentColor(BLUE);
    expect(visibleRecentColors({ alpha: true })).toEqual([BLUE, BLUE_A50]);
  });
});
