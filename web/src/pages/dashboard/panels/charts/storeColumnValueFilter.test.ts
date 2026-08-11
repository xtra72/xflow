// storeColumnValueFilter — 같은 컬럼 OR + 컬럼 간 AND 매처 순수 로직 테스트.
//
// @spec SPEC-PANEL-SETTINGS-001 (REQ-15 / AC-17)

import { describe, expect, it } from 'vitest';

import {
  hasActiveColumnValueFilters,
  matchesColumnValueFilters,
  splitTagPairsToDimensions,
  tagFiltersToColumnValues,
  type ColumnValueFilters,
} from './storeColumnValueFilter';

/** AC-17 예시 행: metric 컬럼 + tag `floor` 컬럼. */
interface Row {
  metric: string;
  floor: string;
}

const R1: Row = { metric: 'temp', floor: '1F' };
const R2: Row = { metric: 'humidity', floor: '1F' };
const R3: Row = { metric: 'co2', floor: '1F' };
const R4: Row = { metric: 'temp', floor: '2F' };
const ROWS: Row[] = [R1, R2, R3, R4];

/** 행 → (columnId → 셀 값) 추출기. floor 는 태그 키를 컬럼으로 취급한다. */
const getter = (row: Row) => (col: string): string | undefined => {
  if (col === 'metric') return row.metric;
  if (col === 'floor') return row.floor;
  return undefined;
};

function filterRows(rows: Row[], filters: ColumnValueFilters): Row[] {
  return rows.filter((r) => matchesColumnValueFilters(getter(r), filters));
}

describe('matchesColumnValueFilters — AC-17', () => {
  it('metric∈{temp,humidity} AND floor=1F → R1, R2 만 통과', () => {
    const filters: ColumnValueFilters = {
      metric: new Set(['temp', 'humidity']),
      floor: new Set(['1F']),
    };
    const result = filterRows(ROWS, filters);
    expect(result).toEqual([R1, R2]);
    // R3(metric=co2)은 metric OR 집합 불충족, R4(floor=2F)는 floor AND 조건 불충족.
    expect(result).not.toContain(R3);
    expect(result).not.toContain(R4);
  });

  it('(Edge) metric=temp 만 설정하고 floor 필터를 비우면 R1, R4 (floor 는 AND 제외)', () => {
    const filters: ColumnValueFilters = {
      metric: new Set(['temp']),
      floor: new Set<string>(), // 빈 집합 → AND 결합에서 제외(항상 통과).
    };
    expect(filterRows(ROWS, filters)).toEqual([R1, R4]);
  });

  it('(Edge) metric=temp 만 설정하고 floor 키 자체가 없어도 R1, R4', () => {
    const filters: ColumnValueFilters = { metric: new Set(['temp']) };
    expect(filterRows(ROWS, filters)).toEqual([R1, R4]);
  });

  it('(하위호환) 필터를 하나도 설정하지 않으면 전체 행이 통과한다', () => {
    expect(filterRows(ROWS, {})).toEqual(ROWS);
    expect(filterRows(ROWS, { metric: new Set(), floor: new Set() })).toEqual(ROWS);
  });

  it('같은 컬럼 다중값은 OR — floor∈{1F,2F} 는 metric=temp 와 결합해 R1, R4', () => {
    const filters: ColumnValueFilters = {
      metric: new Set(['temp']),
      floor: new Set(['1F', '2F']),
    };
    expect(filterRows(ROWS, filters)).toEqual([R1, R4]);
  });

  it('셀 값이 없는(undefined) 컬럼에 필터가 걸리면 탈락한다', () => {
    const filters: ColumnValueFilters = { missing: new Set(['x']) };
    expect(filterRows(ROWS, filters)).toEqual([]);
  });
});

describe('splitTagPairsToDimensions + 태그 키별 차원 매처 — AC-17b', () => {
  // AC-17b 예시 행: 태그 키 device_id, type.
  interface TagRow {
    device_id: string;
    type: string;
  }
  const S1: TagRow = { device_id: 'A', type: 'report' };
  const S2: TagRow = { device_id: 'B', type: 'report' };
  const S3: TagRow = { device_id: 'C', type: 'report' };
  const S4: TagRow = { device_id: 'A', type: 'command' };
  const ROWS: TagRow[] = [S1, S2, S3, S4];
  const tagGetter = (r: TagRow) => (dim: string): string | undefined =>
    dim === 'device_id' ? r.device_id : dim === 'type' ? r.type : undefined;

  it('device_id∈{A,B} AND type=report → S1, S2 만 통과(태그 키별 OR/AND)', () => {
    // 태그 팝오버가 쓰는 "k=v" 값 집합에서 태그 키별 차원으로 분해한다.
    const pairs = new Set(['device_id=A', 'device_id=B', 'type=report']);
    const dims = splitTagPairsToDimensions(pairs);
    expect(dims).toEqual({ device_id: new Set(['A', 'B']), type: new Set(['report']) });

    const result = ROWS.filter((r) => matchesColumnValueFilters(tagGetter(r), dims));
    expect(result).toEqual([S1, S2]);
    // S3(device_id=C)은 device_id OR 집합 불충족, S4(type=command)는 type AND 조건 불충족.
    expect(result).not.toContain(S3);
    expect(result).not.toContain(S4);
  });

  it('"태그" 전체를 한 차원으로 OR 하지 않는다 — device_id=A 만 걸면 type 무관 S1, S4', () => {
    const dims = splitTagPairsToDimensions(new Set(['device_id=A']));
    expect(ROWS.filter((r) => matchesColumnValueFilters(tagGetter(r), dims))).toEqual([S1, S4]);
  });

  it('= 가 없는 값은 무시되고, 빈 집합은 전체 통과', () => {
    expect(splitTagPairsToDimensions(new Set(['bare', 'k=v']))).toEqual({ k: new Set(['v']) });
    expect(splitTagPairsToDimensions(new Set())).toEqual({});
  });
});

describe('hasActiveColumnValueFilters', () => {
  it('활성 값 집합이 있으면 true', () => {
    expect(hasActiveColumnValueFilters({ metric: new Set(['temp']) })).toBe(true);
  });
  it('모두 비어있으면 false', () => {
    expect(hasActiveColumnValueFilters({})).toBe(false);
    expect(hasActiveColumnValueFilters({ metric: new Set(), floor: new Set() })).toBe(false);
  });
});

describe('tagFiltersToColumnValues — 하위호환(단일값→1원소 집합)', () => {
  it('키당 단일값을 원소 1개의 집합으로 승격하며 AND 매칭이 보존된다', () => {
    const cols = tagFiltersToColumnValues({ floor: '1F', room: '3' });
    expect(cols.floor).toEqual(new Set(['1F']));
    expect(cols.room).toEqual(new Set(['3']));

    // floor=1F AND room=3 → 해당 태그를 모두 가진 행만 통과.
    const rowTags = (tags: Record<string, string>) => (col: string) => tags[col];
    expect(
      matchesColumnValueFilters(rowTags({ floor: '1F', room: '3' }), cols),
    ).toBe(true);
    expect(
      matchesColumnValueFilters(rowTags({ floor: '1F', room: '4' }), cols),
    ).toBe(false);
    expect(matchesColumnValueFilters(rowTags({ floor: '1F' }), cols)).toBe(false);
  });

  it('빈 tag_filters 는 빈 필터가 되어 전체 통과', () => {
    const cols = tagFiltersToColumnValues({});
    expect(hasActiveColumnValueFilters(cols)).toBe(false);
  });
});
