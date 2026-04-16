// chartChannelTypes 테스트.
// getByPath 는 ChartEntry 에서 점 경로(dot path)로 값을 추출한다.

import { describe, it, expect } from 'vitest';

import { getByPath, type ChartEntry } from './chartChannelTypes';

describe('getByPath', () => {
  const entry: ChartEntry = {
    timestamp: 1713312000000,
    value: 42.5,
    labels: { room: 'A', zone: 'north' },
    meta: { source: 'sensor', nested: { level: 3 } },
  };

  const tableCases: Array<{ name: string; path: string; expected: unknown }> = [
    { name: '최상위 value', path: 'value', expected: 42.5 },
    { name: '최상위 timestamp', path: 'timestamp', expected: 1713312000000 },
    { name: 'labels.room', path: 'labels.room', expected: 'A' },
    { name: 'labels.zone', path: 'labels.zone', expected: 'north' },
    { name: 'meta.source', path: 'meta.source', expected: 'sensor' },
    { name: 'meta.nested.level (2단 깊이)', path: 'meta.nested.level', expected: 3 },
    { name: '존재하지 않는 최상위', path: 'missing', expected: undefined },
    { name: '존재하지 않는 중첩', path: 'labels.missing', expected: undefined },
    { name: '빈 경로', path: '', expected: undefined },
  ];

  for (const tc of tableCases) {
    it(`${tc.name} -> ${String(tc.expected)}`, () => {
      expect(getByPath(entry, tc.path)).toEqual(tc.expected);
    });
  }

  it('value 가 객체일 때 value.inner 접근 가능', () => {
    const e: ChartEntry = {
      timestamp: 1,
      value: { inner: 10, deep: { x: 99 } },
    };
    expect(getByPath(e, 'value.inner')).toBe(10);
    expect(getByPath(e, 'value.deep.x')).toBe(99);
  });

  it('중간 경로가 null/undefined 면 undefined', () => {
    const e: ChartEntry = { timestamp: 1, value: null };
    expect(getByPath(e, 'value.anything')).toBeUndefined();
  });

  it('labels 가 없을 때 labels.x 는 undefined', () => {
    const e: ChartEntry = { timestamp: 1, value: 5 };
    expect(getByPath(e, 'labels.room')).toBeUndefined();
  });
});
