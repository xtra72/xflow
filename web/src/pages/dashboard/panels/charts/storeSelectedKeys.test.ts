// storeSelectedKeys 순수 헬퍼 테스트.
// @spec SPEC-PANEL-SETTINGS-001 (T6)

import { describe, expect, it } from 'vitest';

import type { StoreSourceConfig } from './chartChannelTypes';
import {
  isKeySelected,
  isSelectionAtLimit,
  readSelectedKeys,
  SELECTED_KEYS_LIMIT,
  toggleSelectedKey,
} from './storeSelectedKeys';

function store(selected?: unknown): StoreSourceConfig {
  return {
    agent_name: 'a',
    series: [],
    time_window_ms: 1000,
    interval_ms: 100,
    aggregation: 'last',
    selected_keys: selected as string[] | undefined,
  };
}

describe('readSelectedKeys', () => {
  it('미설정/비배열이면 빈 배열(하위호환)', () => {
    expect(readSelectedKeys(undefined)).toEqual([]);
    expect(readSelectedKeys(store(undefined))).toEqual([]);
    expect(readSelectedKeys(store('not-array'))).toEqual([]);
    expect(readSelectedKeys(store(123))).toEqual([]);
  });

  it('문자열만 남기고 빈 문자열/중복을 제거하며 순서를 보존한다', () => {
    expect(readSelectedKeys(store(['b', 'a', 'b', '', 3, 'c']))).toEqual(['b', 'a', 'c']);
  });
});

describe('isKeySelected', () => {
  it('선택 여부를 판정한다', () => {
    expect(isKeySelected(store(['a', 'b']), 'a')).toBe(true);
    expect(isKeySelected(store(['a', 'b']), 'z')).toBe(false);
    expect(isKeySelected(undefined, 'a')).toBe(false);
  });
});

describe('toggleSelectedKey', () => {
  it('없으면 추가, 있으면 제거하며 순서를 보존한다(불변)', () => {
    const base = ['a', 'b'];
    expect(toggleSelectedKey(base, 'c')).toEqual(['a', 'b', 'c']);
    expect(toggleSelectedKey(base, 'a')).toEqual(['b']);
    // 원본 불변.
    expect(base).toEqual(['a', 'b']);
  });
});

describe('isSelectionAtLimit (AC-15)', () => {
  it('상한 미만은 false, 상한 이상은 true', () => {
    expect(isSelectionAtLimit([])).toBe(false);
    expect(isSelectionAtLimit(Array.from({ length: SELECTED_KEYS_LIMIT - 1 }, (_, i) => `k${i}`))).toBe(false);
    expect(isSelectionAtLimit(Array.from({ length: SELECTED_KEYS_LIMIT }, (_, i) => `k${i}`))).toBe(true);
  });
});
