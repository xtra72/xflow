// storeEntrySort — store key 테이블 정렬·검색 순수 로직 단위 테스트.

import { describe, expect, it } from 'vitest';

import {
  applyColumnFilters,
  columnCellValues,
  entryPassesColumnFilter,
  filterEntries,
  isColumnFilterActive,
  nextSortState,
  sortEntries,
  uniqueColumnValues,
  type ColumnFilter,
  type FilterContext,
  type SortState,
  type StoreEntry,
} from './storeEntrySort';

// 결정성 검증을 위한 고정 엔트리 셋.
const ENTRIES: StoreEntry[] = [
  {
    key: 'outdoor:humidity',
    value: 55,
    namespace: 'default',
    field: 'humidity',
    tags: { room: '2' },
    updated_at: '2026-06-10T10:00:00.000Z',
  },
  {
    key: 'indoor:temp',
    value: 21.5,
    namespace: 'zone-a',
    field: 'temperature',
    tags: { room: '1' },
    updated_at: '2026-06-12T10:00:00.000Z',
  },
  {
    // 같은 key 의 다른 field 시리즈 (보조 정렬 결정성 검증용).
    key: 'indoor:temp',
    value: 99,
    namespace: 'zone-b',
    field: 'apparent_temperature',
    tags: {},
    updated_at: '2026-06-11T10:00:00.000Z',
  },
  {
    key: 'dynamic:count',
    value: 7,
    namespace: '',
    field: 'unknown',
    tags: {},
    updated_at: '2026-06-09T10:00:00.000Z',
  },
];

const STATIC_KEYS = new Set<string>(['indoor:temp']);

function ctx() {
  return { staticKeyNames: STATIC_KEYS };
}

describe('filterEntries', () => {
  it('빈 검색어는 원본을 (복사본으로) 그대로 반환한다', () => {
    const out = filterEntries(ENTRIES, '');
    expect(out).toHaveLength(ENTRIES.length);
    expect(out).not.toBe(ENTRIES);
  });

  it('공백만 입력하면 필터하지 않는다', () => {
    expect(filterEntries(ENTRIES, '   ')).toHaveLength(ENTRIES.length);
  });

  it('key 부분일치(대소문자 무시)로 필터한다', () => {
    const out = filterEntries(ENTRIES, 'INDOOR');
    expect(out).toHaveLength(2);
    expect(out.every((e) => (e.key as string).startsWith('indoor'))).toBe(true);
  });

  it('field 으로 필터한다 (key 에 없는 텍스트)', () => {
    const out = filterEntries(ENTRIES, 'humidity');
    expect(out.map((e) => e.key)).toEqual(['outdoor:humidity']);
  });

  it('field 누락 엔트리는 "unknown" 으로 검색된다', () => {
    const out = filterEntries(ENTRIES, 'unknown');
    expect(out.map((e) => e.key)).toEqual(['dynamic:count']);
  });

  it('tags 의 키/값/"k=v" 결합으로 필터한다', () => {
    expect(filterEntries(ENTRIES, 'room')).toHaveLength(2);
    expect(filterEntries(ENTRIES, 'room=1')).toHaveLength(1);
    expect(filterEntries(ENTRIES, '=2')).toHaveLength(1);
  });

  it('어디에도 매칭되지 않으면 빈 배열', () => {
    expect(filterEntries(ENTRIES, 'zzz-nomatch')).toHaveLength(0);
  });
});

describe('sortEntries', () => {
  it('column=null 이면 원본 순서(복사본)를 유지한다', () => {
    const sort: SortState = { column: null, direction: 'asc' };
    const out = sortEntries(ENTRIES, sort, ctx());
    expect(out.map((e) => e.key)).toEqual(ENTRIES.map((e) => e.key));
    expect(out).not.toBe(ENTRIES);
  });

  it('key 오름차순 정렬', () => {
    const out = sortEntries(ENTRIES, { column: 'key', direction: 'asc' }, ctx());
    expect(out.map((e) => e.key)).toEqual([
      'dynamic:count',
      'indoor:temp',
      'indoor:temp',
      'outdoor:humidity',
    ]);
    // 같은 key 는 field 보조 정렬로 결정적: apparent_temperature < temperature.
    expect(out.map((e) => e.field)).toEqual([
      'unknown',
      'apparent_temperature',
      'temperature',
      'humidity',
    ]);
  });

  it('key 내림차순 정렬 (보조 정렬은 항상 오름차순 유지)', () => {
    const out = sortEntries(ENTRIES, { column: 'key', direction: 'desc' }, ctx());
    expect(out.map((e) => e.key)).toEqual([
      'outdoor:humidity',
      'indoor:temp',
      'indoor:temp',
      'dynamic:count',
    ]);
  });

  it('field 오름차순 정렬', () => {
    const out = sortEntries(
      ENTRIES,
      { column: 'field', direction: 'asc' },
      ctx(),
    );
    expect(out.map((e) => e.field)).toEqual([
      'apparent_temperature',
      'humidity',
      'temperature',
      'unknown',
    ]);
  });

  it('binding 정렬: 정적(0) 먼저, 동적(1) 나중 (asc)', () => {
    const out = sortEntries(
      ENTRIES,
      { column: 'binding', direction: 'asc' },
      ctx(),
    );
    // 정적 키(indoor:temp) 2건이 앞, 나머지 동적이 뒤. 동률은 key→field 보조 정렬.
    const isStaticSeq = out.map((e) => STATIC_KEYS.has(e.key as string));
    expect(isStaticSeq).toEqual([true, true, false, false]);
  });

  it('namespace 오름차순 정렬 (빈 네임스페이스 먼저)', () => {
    const out = sortEntries(
      ENTRIES,
      { column: 'namespace', direction: 'asc' },
      ctx(),
    );
    expect(out.map((e) => e.namespace)).toEqual([
      '',
      'default',
      'zone-a',
      'zone-b',
    ]);
  });

  it('updated 오름차순 정렬 (오래된 것 먼저)', () => {
    const out = sortEntries(
      ENTRIES,
      { column: 'updated', direction: 'asc' },
      ctx(),
    );
    expect(out.map((e) => e.key)).toEqual([
      'dynamic:count', // 06-09
      'outdoor:humidity', // 06-10
      'indoor:temp', // 06-11
      'indoor:temp', // 06-12
    ]);
  });

  it('value 숫자 인지 정렬 (numeric collation)', () => {
    const out = sortEntries(
      ENTRIES,
      { column: 'value', direction: 'asc' },
      ctx(),
    );
    expect(out.map((e) => e.value)).toEqual([7, 21.5, 55, 99]);
  });

  it('정렬은 원본 배열을 변경하지 않는다', () => {
    const before = ENTRIES.map((e) => e.key);
    sortEntries(ENTRIES, { column: 'key', direction: 'desc' }, ctx());
    expect(ENTRIES.map((e) => e.key)).toEqual(before);
  });
});

describe('nextSortState', () => {
  it('다른 컬럼 클릭 시 해당 컬럼 오름차순으로 전환', () => {
    const cur: SortState = { column: 'field', direction: 'desc' };
    expect(nextSortState(cur, 'key')).toEqual({
      column: 'key',
      direction: 'asc',
    });
  });

  it('같은 컬럼 클릭 시 asc → desc 토글', () => {
    expect(nextSortState({ column: 'key', direction: 'asc' }, 'key')).toEqual({
      column: 'key',
      direction: 'desc',
    });
  });

  it('같은 컬럼 클릭 시 desc → asc 토글', () => {
    expect(nextSortState({ column: 'key', direction: 'desc' }, 'key')).toEqual({
      column: 'key',
      direction: 'asc',
    });
  });
});

// --- Excel 유사 컬럼 필터 순수 함수 ---

const FILTER_CTX: FilterContext = {
  staticKeyNames: new Set<string>(['indoor:temp']),
  bindingLabels: { static: '정적', dynamic: '동적' },
};

const FILTER_ENTRIES: StoreEntry[] = [
  {
    key: 'indoor:temp',
    value: 21.5,
    namespace: 'zone-a',
    field: 'temperature',
    tags: { room: '1', floor: '2' },
    ttl: '10m',
  },
  {
    key: 'outdoor:humidity',
    value: 55,
    namespace: 'default',
    field: 'humidity',
    tags: { room: '2' },
  },
  {
    key: 'dynamic:count',
    value: 7,
    namespace: '',
    field: 'unknown',
    tags: {},
  },
];

function filter(partial: Partial<ColumnFilter>): ColumnFilter {
  return { text: '', values: new Set<string>(), ...partial };
}

// noUncheckedIndexedAccess 대응: 인덱스 접근을 non-null 로 좁힌다.
function at(i: number): StoreEntry {
  const e = FILTER_ENTRIES[i];
  if (!e) throw new Error(`no entry at ${i}`);
  return e;
}

describe('columnCellValues', () => {
  it('key/field/value/namespace 단일 값을 반환한다', () => {
    const e = at(0);
    expect(columnCellValues(e, 'key', FILTER_CTX)).toEqual(['indoor:temp']);
    expect(columnCellValues(e, 'field', FILTER_CTX)).toEqual(['temperature']);
    expect(columnCellValues(e, 'value', FILTER_CTX)).toEqual(['21.5']);
    expect(columnCellValues(e, 'namespace', FILTER_CTX)).toEqual(['zone-a']);
  });

  it('field 누락은 unknown 으로 정규화된다', () => {
    expect(columnCellValues(at(2), 'field', FILTER_CTX)).toEqual([
      'unknown',
    ]);
  });

  it('binding 은 정적/동적 라벨을 셀 값으로 반환한다', () => {
    expect(columnCellValues(at(0), 'binding', FILTER_CTX)).toEqual([
      '정적',
    ]);
    expect(columnCellValues(at(1), 'binding', FILTER_CTX)).toEqual([
      '동적',
    ]);
  });

  it('tags 는 정렬된 "k=v" 배열을 반환한다', () => {
    expect(columnCellValues(at(0), 'tags', FILTER_CTX)).toEqual([
      'floor=2',
      'room=1',
    ]);
    expect(columnCellValues(at(2), 'tags', FILTER_CTX)).toEqual([]);
  });

  it('ttl 은 값이 없으면 무한대 기호를 반환한다', () => {
    expect(columnCellValues(at(0), 'ttl', FILTER_CTX)).toEqual([
      '10m',
    ]);
    expect(columnCellValues(at(1), 'ttl', FILTER_CTX)).toEqual([
      '∞',
    ]);
  });
});

describe('uniqueColumnValues', () => {
  it('field 고유 값을 정렬해 반환한다', () => {
    expect(uniqueColumnValues(FILTER_ENTRIES, 'field', FILTER_CTX)).toEqual([
      'humidity',
      'temperature',
      'unknown',
    ]);
  });

  it('binding 고유 값(정적/동적)을 반환한다', () => {
    const vals = uniqueColumnValues(FILTER_ENTRIES, 'binding', FILTER_CTX);
    expect(new Set(vals)).toEqual(new Set(['정적', '동적']));
  });

  it('tags 고유 값은 모든 k=v 를 합쳐 반환한다', () => {
    expect(uniqueColumnValues(FILTER_ENTRIES, 'tags', FILTER_CTX)).toEqual([
      'floor=2',
      'room=1',
      'room=2',
    ]);
  });
});

describe('entryPassesColumnFilter', () => {
  it('텍스트 부분일치(대소문자 무시)', () => {
    const f = filter({ text: 'TEMP' });
    expect(entryPassesColumnFilter(at(0), 'field', f, FILTER_CTX)).toBe(true);
    expect(entryPassesColumnFilter(at(1), 'field', f, FILTER_CTX)).toBe(false);
  });

  it('값 체크박스: 선택 없으면 전체 통과', () => {
    const f = filter({});
    expect(entryPassesColumnFilter(at(0), 'field', f, FILTER_CTX)).toBe(true);
  });

  it('값 체크박스: 선택된 값 집합에 포함될 때만 통과', () => {
    const f = filter({ values: new Set(['humidity']) });
    expect(entryPassesColumnFilter(at(0), 'field', f, FILTER_CTX)).toBe(false);
    expect(entryPassesColumnFilter(at(1), 'field', f, FILTER_CTX)).toBe(true);
  });

  it('텍스트 AND 값: 둘 다 만족해야 통과', () => {
    // text 는 room 매칭이지만 값 집합이 room=1 만 허용 → outdoor(room=2) 는 탈락.
    const f = filter({ text: 'room', values: new Set(['room=1']) });
    expect(entryPassesColumnFilter(at(0), 'tags', f, FILTER_CTX)).toBe(true);
    expect(entryPassesColumnFilter(at(1), 'tags', f, FILTER_CTX)).toBe(false);
  });

  it('tags: 다중 값 중 하나라도 체크 집합에 있으면 통과', () => {
    const f = filter({ values: new Set(['floor=2']) });
    expect(entryPassesColumnFilter(at(0), 'tags', f, FILTER_CTX)).toBe(true);
    expect(entryPassesColumnFilter(at(1), 'tags', f, FILTER_CTX)).toBe(false);
  });
});

describe('isColumnFilterActive', () => {
  it('undefined/빈 필터는 비활성', () => {
    expect(isColumnFilterActive(undefined)).toBe(false);
    expect(isColumnFilterActive(filter({}))).toBe(false);
    expect(isColumnFilterActive(filter({ text: '   ' }))).toBe(false);
  });

  it('텍스트 또는 값 선택이 있으면 활성', () => {
    expect(isColumnFilterActive(filter({ text: 'x' }))).toBe(true);
    expect(isColumnFilterActive(filter({ values: new Set(['a']) }))).toBe(true);
  });
});

describe('applyColumnFilters', () => {
  it('활성 필터가 없으면 원본 복사본을 반환한다', () => {
    const out = applyColumnFilters(FILTER_ENTRIES, {}, FILTER_CTX);
    expect(out).toHaveLength(FILTER_ENTRIES.length);
    expect(out).not.toBe(FILTER_ENTRIES);
  });

  it('여러 컬럼 필터를 AND 로 결합한다', () => {
    const out = applyColumnFilters(
      FILTER_ENTRIES,
      {
        binding: filter({ values: new Set(['정적']) }),
        field: filter({ text: 'temp' }),
      },
      FILTER_CTX,
    );
    expect(out.map((e) => e.key)).toEqual(['indoor:temp']);
  });

  it('빈(비활성) 필터는 무시된다', () => {
    const out = applyColumnFilters(
      FILTER_ENTRIES,
      { field: filter({}), namespace: filter({ text: 'zone' }) },
      FILTER_CTX,
    );
    expect(out.map((e) => e.key)).toEqual(['indoor:temp']);
  });
});
