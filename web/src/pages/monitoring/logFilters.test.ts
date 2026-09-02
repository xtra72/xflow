// 로그 정렬·필터 순수 로직 테스트.

import { describe, it, expect } from 'vitest';

import {
  collectOptions,
  emptyFilterState,
  entryTimeOfDay,
  filterLogs,
  hasAnyFilter,
  parseTimeOfDay,
  sortLogs,
} from './logFilters';
import type { LogEntry } from './LogViewer';

/** 테스트용 로그 항목 생성 */
function entry(over: Partial<LogEntry> & { id: string }): LogEntry {
  return {
    timestamp: '00:00:00',
    level: 'INFO',
    message: '',
    ...over,
  };
}

/** 로컬 시간 기준 epoch ms */
function at(h: number, m: number, s = 0): number {
  const d = new Date(2026, 0, 15, h, m, s);
  return d.getTime();
}

describe('parseTimeOfDay', () => {
  it('HH:MM 과 HH:MM:SS 를 모두 받는다', () => {
    expect(parseTimeOfDay('01:30')).toBe(5_400);
    expect(parseTimeOfDay('01:30:15')).toBe(5_415);
  });

  it('형식이나 범위가 어긋나면 null', () => {
    expect(parseTimeOfDay('')).toBeNull();
    expect(parseTimeOfDay('25:00')).toBeNull();
    expect(parseTimeOfDay('12:70')).toBeNull();
    expect(parseTimeOfDay('abc')).toBeNull();
  });
});

describe('entryTimeOfDay', () => {
  it('ts 가 있으면 epoch 에서 읽는다', () => {
    expect(entryTimeOfDay(entry({ id: '1', ts: at(9, 5, 30) }))).toBe(9 * 3600 + 5 * 60 + 30);
  });

  it('ts 가 없으면 표시 문자열에서 읽는다', () => {
    expect(entryTimeOfDay(entry({ id: '1', timestamp: '13:45:00' }))).toBe(13 * 3600 + 45 * 60);
  });

  it('둘 다 쓸 수 없으면 null', () => {
    expect(entryTimeOfDay(entry({ id: '1', timestamp: '-' }))).toBeNull();
  });
});

describe('sortLogs — 시간 기준', () => {
  const entries = [
    entry({ id: 'b', ts: at(10, 0) }),
    entry({ id: 'a', ts: at(9, 0) }),
    entry({ id: 'c', ts: at(11, 0) }),
  ];

  it('오름차순은 이른 시각이 먼저', () => {
    expect(sortLogs(entries, 'asc').map((e) => e.id)).toEqual(['a', 'b', 'c']);
  });

  it('내림차순은 늦은 시각이 먼저', () => {
    expect(sortLogs(entries, 'desc').map((e) => e.id)).toEqual(['c', 'b', 'a']);
  });

  it('시각이 같으면 수신 순서를 유지한다 (안정 정렬)', () => {
    const same = [
      entry({ id: '1', ts: at(9, 0) }),
      entry({ id: '2', ts: at(9, 0) }),
      entry({ id: '3', ts: at(9, 0) }),
    ];
    expect(sortLogs(same, 'asc').map((e) => e.id)).toEqual(['1', '2', '3']);
    expect(sortLogs(same, 'desc').map((e) => e.id)).toEqual(['3', '2', '1']);
  });

  it('ts 가 없으면 수신 순서를 시간 순서로 본다', () => {
    const noTs = [entry({ id: '1' }), entry({ id: '2' }), entry({ id: '3' })];
    expect(sortLogs(noTs, 'asc').map((e) => e.id)).toEqual(['1', '2', '3']);
    expect(sortLogs(noTs, 'desc').map((e) => e.id)).toEqual(['3', '2', '1']);
  });

  it('원본 배열을 바꾸지 않는다', () => {
    const original = entries.map((e) => e.id);
    sortLogs(entries, 'desc');
    expect(entries.map((e) => e.id)).toEqual(original);
  });
});

describe('filterLogs — 선택 필터', () => {
  const entries = [
    entry({ id: '1', level: 'ERROR', source: 'agent', componentKind: 'modbus', componentName: 'a1' }),
    entry({ id: '2', level: 'INFO', source: 'flow', componentKind: 'http', componentName: 'f1' }),
    entry({ id: '3', level: 'WARN', source: 'agent', componentKind: 'modbus', componentName: 'a2' }),
  ];

  it('빈 선택은 전체 통과', () => {
    expect(filterLogs(entries, emptyFilterState())).toHaveLength(3);
  });

  it('레벨 다중 선택은 합집합', () => {
    const state = { ...emptyFilterState(), levels: new Set(['ERROR', 'WARN']) };
    expect(filterLogs(entries, state).map((e) => e.id)).toEqual(['1', '3']);
  });

  it('소스 선택', () => {
    const state = { ...emptyFilterState(), sources: new Set(['flow']) };
    expect(filterLogs(entries, state).map((e) => e.id)).toEqual(['2']);
  });

  it('타입 선택', () => {
    const state = { ...emptyFilterState(), kinds: new Set(['modbus']) };
    expect(filterLogs(entries, state).map((e) => e.id)).toEqual(['1', '3']);
  });

  it('이름 선택', () => {
    const state = { ...emptyFilterState(), names: new Set(['a2']) };
    expect(filterLogs(entries, state).map((e) => e.id)).toEqual(['3']);
  });

  it('여러 컬럼은 AND 로 묶인다', () => {
    const state = {
      ...emptyFilterState(),
      sources: new Set(['agent']),
      levels: new Set(['WARN']),
    };
    expect(filterLogs(entries, state).map((e) => e.id)).toEqual(['3']);
  });

  it('값이 비어 있는 항목은 해당 컬럼 선택 시 제외된다', () => {
    const withEmpty = [...entries, entry({ id: '4', componentKind: '' })];
    const state = { ...emptyFilterState(), kinds: new Set(['modbus']) };
    expect(filterLogs(withEmpty, state).map((e) => e.id)).toEqual(['1', '3']);
  });
});

describe('filterLogs — 시간 구간', () => {
  const entries = [
    entry({ id: 'morning', ts: at(9, 0) }),
    entry({ id: 'noon', ts: at(12, 0) }),
    entry({ id: 'evening', ts: at(20, 0) }),
    entry({ id: 'lateNight', ts: at(23, 30) }),
  ];

  it('시작만 주면 그 이후', () => {
    const state = { ...emptyFilterState(), timeFrom: '12:00' };
    expect(filterLogs(entries, state).map((e) => e.id)).toEqual(['noon', 'evening', 'lateNight']);
  });

  it('종료만 주면 그 이전', () => {
    const state = { ...emptyFilterState(), timeTo: '12:00' };
    expect(filterLogs(entries, state).map((e) => e.id)).toEqual(['morning', 'noon']);
  });

  it('양끝을 주면 그 사이 (경계 포함)', () => {
    const state = { ...emptyFilterState(), timeFrom: '09:00', timeTo: '20:00' };
    expect(filterLogs(entries, state).map((e) => e.id)).toEqual(['morning', 'noon', 'evening']);
  });

  it('시작이 종료보다 늦으면 자정을 넘는 구간으로 본다', () => {
    // 22:00~10:00 → 23:30 과 09:00 이 들어오고 낮 시간은 빠진다.
    const state = { ...emptyFilterState(), timeFrom: '22:00', timeTo: '10:00' };
    expect(filterLogs(entries, state).map((e) => e.id)).toEqual(['morning', 'lateNight']);
  });

  it('시각을 읽을 수 없는 항목은 구간 조회에서 빠진다', () => {
    const withBad = [...entries, entry({ id: 'bad', timestamp: '-' })];
    const state = { ...emptyFilterState(), timeFrom: '00:00', timeTo: '23:59' };
    expect(filterLogs(withBad, state).map((e) => e.id)).not.toContain('bad');
  });
});

describe('collectOptions', () => {
  it('중복을 제거하고 사전순으로 정렬한다', () => {
    const entries = [
      entry({ id: '1', componentKind: 'modbus' }),
      entry({ id: '2', componentKind: 'http' }),
      entry({ id: '3', componentKind: 'modbus' }),
    ];
    expect(collectOptions(entries, (e) => e.componentKind)).toEqual(['http', 'modbus']);
  });

  it('빈 값은 옵션에 넣지 않는다', () => {
    const entries = [entry({ id: '1', componentKind: '' }), entry({ id: '2' })];
    expect(collectOptions(entries, (e) => e.componentKind)).toEqual([]);
  });
});

describe('hasAnyFilter', () => {
  it('초기 상태는 false', () => {
    expect(hasAnyFilter(emptyFilterState())).toBe(false);
  });

  it('선택이나 구간이 하나라도 있으면 true', () => {
    expect(hasAnyFilter({ ...emptyFilterState(), levels: new Set(['INFO']) })).toBe(true);
    expect(hasAnyFilter({ ...emptyFilterState(), timeFrom: '09:00' })).toBe(true);
  });
});
