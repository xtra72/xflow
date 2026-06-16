// storeEntrySort — store key 테이블 정렬·검색 순수 로직 단위 테스트.

import { describe, expect, it } from 'vitest';

import {
  filterEntries,
  nextSortState,
  sortEntries,
  type SortState,
  type StoreEntry,
} from './storeEntrySort';

// 결정성 검증을 위한 고정 엔트리 셋.
const ENTRIES: StoreEntry[] = [
  {
    key: 'outdoor:humidity',
    value: 55,
    namespace: 'default',
    metric_type: 'humidity',
    tags: { room: '2' },
    updated_at: '2026-06-10T10:00:00.000Z',
  },
  {
    key: 'indoor:temp',
    value: 21.5,
    namespace: 'zone-a',
    metric_type: 'temperature',
    tags: { room: '1' },
    updated_at: '2026-06-12T10:00:00.000Z',
  },
  {
    // 같은 key 의 다른 metric 시리즈 (보조 정렬 결정성 검증용).
    key: 'indoor:temp',
    value: 99,
    namespace: 'zone-b',
    metric_type: 'apparent_temperature',
    tags: {},
    updated_at: '2026-06-11T10:00:00.000Z',
  },
  {
    key: 'dynamic:count',
    value: 7,
    namespace: '',
    metric_type: 'unknown',
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

  it('metric_type 으로 필터한다 (key 에 없는 텍스트)', () => {
    const out = filterEntries(ENTRIES, 'humidity');
    expect(out.map((e) => e.key)).toEqual(['outdoor:humidity']);
  });

  it('metric 누락 엔트리는 "unknown" 으로 검색된다', () => {
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
    // 같은 key 는 metric 보조 정렬로 결정적: apparent_temperature < temperature.
    expect(out.map((e) => e.metric_type)).toEqual([
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

  it('metric 오름차순 정렬', () => {
    const out = sortEntries(
      ENTRIES,
      { column: 'metric', direction: 'asc' },
      ctx(),
    );
    expect(out.map((e) => e.metric_type)).toEqual([
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
    // 정적 키(indoor:temp) 2건이 앞, 나머지 동적이 뒤. 동률은 key→metric 보조 정렬.
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
    const cur: SortState = { column: 'metric', direction: 'desc' };
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
