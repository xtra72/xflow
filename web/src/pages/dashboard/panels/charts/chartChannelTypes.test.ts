// chartChannelTypes 테스트.
// getByPath 는 ChartEntry 에서 점 경로(dot path)로 값을 추출한다.

import { describe, it, expect } from 'vitest';

import {
  buildEnumLabelMap,
  formatEnumValue,
  getByPath,
  pickSeriesColor,
  resolveAxisFont,
  SERIES_PALETTE,
  DEFAULT_AXIS_FONT,
  type ChartEntry,
} from './chartChannelTypes';

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

describe('buildEnumLabelMap', () => {
  it('유효한 매핑만 값→라벨 Map 으로 만든다', () => {
    const map = buildEnumLabelMap([
      { value: 0, label: '정지' },
      { value: 1, label: '운전' },
      { value: 2, label: '자동' },
    ]);
    expect(map.size).toBe(3);
    expect(map.get(0)).toBe('정지');
    expect(map.get(2)).toBe('자동');
  });

  it('undefined 입력은 빈 Map', () => {
    expect(buildEnumLabelMap(undefined).size).toBe(0);
  });

  it('빈 라벨/공백 라벨/비유한 값은 제외한다', () => {
    const map = buildEnumLabelMap([
      { value: 0, label: '' },
      { value: 1, label: '   ' },
      { value: Number.NaN, label: '무효' },
      { value: 3, label: 'ON' },
    ]);
    expect(map.size).toBe(1);
    expect(map.get(3)).toBe('ON');
  });

  it('같은 value 중복 시 마지막 정의가 우선한다', () => {
    const map = buildEnumLabelMap([
      { value: 1, label: '첫번째' },
      { value: 1, label: '두번째' },
    ]);
    expect(map.get(1)).toBe('두번째');
  });

  it('라벨 앞뒤 공백은 트림한다', () => {
    const map = buildEnumLabelMap([{ value: 1, label: '  운전  ' }]);
    expect(map.get(1)).toBe('운전');
  });
});

describe('formatEnumValue', () => {
  const map = buildEnumLabelMap([
    { value: 0, label: 'OFF' },
    { value: 1, label: 'ON' },
  ]);

  it('매핑된 값은 라벨로 변환한다', () => {
    expect(formatEnumValue(0, map)).toBe('OFF');
    expect(formatEnumValue(1, map)).toBe('ON');
  });

  it('매핑에 없는 값은 숫자 문자열로 폴백한다', () => {
    expect(formatEnumValue(2, map)).toBe('2');
  });

  it('숫자가 아니거나 비유한 값은 빈 문자열', () => {
    expect(formatEnumValue(Number.NaN, map)).toBe('');
    expect(formatEnumValue(Number.POSITIVE_INFINITY, map)).toBe('');
  });
});

describe('pickSeriesColor', () => {
  it('인덱스별로 팔레트 색을 반환한다', () => {
    expect(pickSeriesColor(0)).toBe(SERIES_PALETTE[0]);
    expect(pickSeriesColor(1)).toBe(SERIES_PALETTE[1]);
  });

  it('팔레트 길이를 넘으면 순환한다', () => {
    const n = SERIES_PALETTE.length;
    expect(pickSeriesColor(n)).toBe(SERIES_PALETTE[0]);
    expect(pickSeriesColor(n + 2)).toBe(SERIES_PALETTE[2]);
  });

  it('음수 인덱스도 안전하게 순환한다', () => {
    expect(pickSeriesColor(-1)).toBe(SERIES_PALETTE[SERIES_PALETTE.length - 1]);
  });

  it('연속 인덱스는 서로 다른 색을 준다(팔레트 범위 내)', () => {
    const colors = [0, 1, 2, 3].map(pickSeriesColor);
    expect(new Set(colors).size).toBe(4);
  });
});

describe('resolveAxisFont', () => {
  it('undefined 는 전부 기본값', () => {
    expect(resolveAxisFont(undefined)).toEqual({
      fontSize: DEFAULT_AXIS_FONT.size,
      fill: DEFAULT_AXIS_FONT.color,
      fontWeight: DEFAULT_AXIS_FONT.weight,
    });
  });

  it('지정 필드는 반영하고 미지정 필드는 기본값으로 폴백한다', () => {
    expect(resolveAxisFont({ size: 14 })).toEqual({
      fontSize: 14,
      fill: DEFAULT_AXIS_FONT.color,
      fontWeight: 'normal',
    });
    expect(resolveAxisFont({ color: '#ff0000', weight: 'bold' })).toEqual({
      fontSize: DEFAULT_AXIS_FONT.size,
      fill: '#ff0000',
      fontWeight: 'bold',
    });
  });
});
