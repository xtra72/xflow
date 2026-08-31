// 테이블 패널 순수 열 계산 테스트 — 폭 비율 · 열 필터.

import { describe, expect, it } from 'vitest';

import type { ChartEntry, TableColumn } from './chartChannelTypes';
import {
  MIN_COLUMN_PX,
  applyColumnWidths,
  applyTableColumnFilters,
  columnWidthPercents,
  epochToLocalInput,
  formatCell,
  isRangeFilterColumn,
  localInputToEpoch,
  numericCellValue,
  resizeColumnWidths,
  resolveCellValue,
  uniqueTableColumnValues,
} from './tableColumns';

/** 텍스트 부분일치 필터 하나. */
function textFilter(text: string) {
  return { text, values: new Set<string>() };
}

/** 값 선택 필터 하나(나열된 값 중 고르기). */
function valueFilter(...values: string[]) {
  return { text: '', values: new Set(values) };
}

/** 범위 필터 하나. 경계는 생략 가능(한쪽만 지정). */
function rangeFilter(min?: number, max?: number) {
  return { text: '', values: new Set<string>(), min, max };
}

/** 태그를 실은 엔트리. */
function tagged(timestamp: number, value: number, tags: Record<string, string>): ChartEntry {
  return { timestamp, value, labels: tags } as unknown as ChartEntry;
}

const COLS: TableColumn[] = [
  { field: 'timestamp', header: '시간', format: 'datetime' },
  { field: 'value', header: '값', format: 'number' },
  { field: 'name', header: '이름', format: 'string' },
];

function entry(timestamp: number, value: number, name: string): ChartEntry {
  return { timestamp, value, name } as unknown as ChartEntry;
}

describe('columnWidthPercents — 폭 비율', () => {
  it('폭을 지정한 열이 없으면 undefined (자동 폭 유지)', () => {
    expect(columnWidthPercents(COLS)).toBeUndefined();
  });

  it('전 열 지정 시 비율대로 백분율을 나눈다', () => {
    const cols: TableColumn[] = [
      { field: 'a', header: 'A', width: 1 },
      { field: 'b', header: 'B', width: 3 },
    ];
    expect(columnWidthPercents(cols)).toEqual(['25.0000%', '75.0000%']);
  });

  it('일부만 지정하면 미지정 열은 undefined 로 남아 남은 폭을 나눠 갖는다', () => {
    const cols: TableColumn[] = [
      { field: 'a', header: 'A', width: 2 },
      { field: 'b', header: 'B' },
      { field: 'c', header: 'C', width: 2 },
    ];
    const out = columnWidthPercents(cols)!;
    expect(out[0]).toBe('50.0000%');
    expect(out[1]).toBeUndefined();
    expect(out[2]).toBe('50.0000%');
  });

  it('0 이하 폭은 미지정으로 취급한다 (열이 사라지지 않게)', () => {
    const cols: TableColumn[] = [
      { field: 'a', header: 'A', width: 0 },
      { field: 'b', header: 'B', width: 4 },
    ];
    const out = columnWidthPercents(cols)!;
    expect(out[0]).toBeUndefined();
    expect(out[1]).toBe('100.0000%');
  });
});

describe('uniqueTableColumnValues — 필터 드롭다운 값 목록', () => {
  const rows = [
    entry(1, 10, 'alpha'),
    entry(2, 25, 'bravo'),
    entry(3, 30, 'alpha'),
  ];

  it('중복을 제거하고 정렬해 반환한다', () => {
    expect(uniqueTableColumnValues(rows, COLS[2]!)).toEqual(['alpha', 'bravo']);
  });

  it('표시 문자열 기준이다 (number 열은 기본 2자리로 포맷된 값)', () => {
    expect(uniqueTableColumnValues(rows, COLS[1]!)).toEqual(['10.00', '25.00', '30.00']);
  });

  it('자릿수를 지정하면 목록도 그 자릿수로 만들어진다 (보이는 값 = 고르는 값)', () => {
    expect(uniqueTableColumnValues(rows, COLS[1]!, 0)).toEqual(['10', '25', '30']);
  });

  it('엔트리가 없으면 빈 목록', () => {
    expect(uniqueTableColumnValues([], COLS[2]!)).toEqual([]);
  });
});

describe('applyTableColumnFilters — 열 필터', () => {
  const rows = [
    entry(1_700_000_000_000, 10, 'alpha'),
    entry(1_700_000_060_000, 25, 'bravo'),
    entry(1_700_000_120_000, 30, 'Alpha-2'),
  ];
  const names = (out: ChartEntry[]) =>
    out.map((r) => (r as unknown as { name: string }).name);

  it('활성 필터가 없으면 원본을 그대로 반환한다', () => {
    expect(applyTableColumnFilters(rows, COLS, {})).toBe(rows);
    expect(applyTableColumnFilters(rows, COLS, { name: textFilter('   ') })).toBe(rows);
    // 값 선택이 비어 있는 것도 "전체" 이므로 비활성이다.
    expect(applyTableColumnFilters(rows, COLS, { name: valueFilter() })).toBe(rows);
  });

  it('나열된 값 중 고른 것만 남긴다 (주 사용 경로)', () => {
    const out = applyTableColumnFilters(rows, COLS, { name: valueFilter('alpha', 'Alpha-2') });
    expect(names(out)).toEqual(['alpha', 'Alpha-2']);
  });

  it('값 선택은 표시 문자열 완전 일치다 (부분 일치가 아니다)', () => {
    expect(applyTableColumnFilters(rows, COLS, { name: valueFilter('alpha') })).toHaveLength(1);
  });

  it('텍스트 부분 일치는 대소문자를 구분하지 않는다', () => {
    expect(names(applyTableColumnFilters(rows, COLS, { name: textFilter('ALPHA') })))
      .toEqual(['alpha', 'Alpha-2']);
  });

  it('같은 열의 텍스트와 값 선택은 AND 로 결합된다', () => {
    const out = applyTableColumnFilters(rows, COLS, {
      name: { text: 'alpha', values: new Set(['Alpha-2']) },
    });
    expect(names(out)).toEqual(['Alpha-2']);
  });

  it('여러 열 필터도 AND 로 결합된다', () => {
    // 자릿수를 0 으로 두어 값 문자열을 정수로 고정한다 — 이 테스트의 관심사는 결합
    // 규칙이지 포맷이 아니다. 표시·목록·판정이 같은 자릿수를 쓰는지는 아래 테스트가 본다.
    const out = applyTableColumnFilters(
      rows,
      COLS,
      {
        name: valueFilter('alpha', 'Alpha-2'),
        value: valueFilter('30'),
      },
      0,
    );
    expect(names(out)).toEqual(['Alpha-2']);
  });

  it('값 선택은 표시 자릿수와 같은 문자열로 판정된다', () => {
    // 기본 자릿수(2)에서는 셀이 '30.00' 이므로 '30' 은 걸리지 않고 '30.00' 이 걸린다.
    expect(names(applyTableColumnFilters(rows, COLS, { value: valueFilter('30') }))).toEqual([]);
    expect(names(applyTableColumnFilters(rows, COLS, { value: valueFilter('30.00') }))).toEqual([
      'Alpha-2',
    ]);
  });

  it('표시 문자열 기준으로 거른다 (datetime 열을 포맷된 값으로 검색)', () => {
    const shown = formatCell(rows[0]!.timestamp, 'datetime');
    const out = applyTableColumnFilters(rows, COLS, { timestamp: textFilter(shown.slice(0, 7)) });
    expect(out.length).toBeGreaterThan(0);
    // 값 선택으로도 같은 행을 집을 수 있다.
    expect(applyTableColumnFilters(rows, COLS, { timestamp: valueFilter(shown) })).toHaveLength(1);
  });

  it('일치가 없으면 빈 배열', () => {
    expect(applyTableColumnFilters(rows, COLS, { name: valueFilter('zzz') })).toEqual([]);
  });
});

describe('resolveCellValue — 태그 컬럼', () => {
  const e = tagged(1, 10, { location: 'roomA', floor: '3' });

  it('$.tags.<키> 로 시리즈 태그를 읽는다', () => {
    expect(resolveCellValue(e, '$.tags.location')).toBe('roomA');
    expect(resolveCellValue(e, '$.tags.floor')).toBe('3');
  });

  it('없는 태그는 undefined', () => {
    expect(resolveCellValue(e, '$.tags.missing')).toBeUndefined();
  });

  it('$.tags. 뒤가 비면 undefined (경로 오타를 값으로 만들지 않는다)', () => {
    expect(resolveCellValue(e, '$.tags.')).toBeUndefined();
  });

  it('기존 점 경로는 그대로 동작한다 (하위 호환)', () => {
    expect(resolveCellValue(e, 'value')).toBe(10);
    expect(resolveCellValue(e, 'timestamp')).toBe(1);
    expect(resolveCellValue(e, 'labels.location')).toBe('roomA');
  });
});

describe('태그 컬럼의 값 선택 필터', () => {
  const rows = [
    tagged(1, 10, { location: 'roomA' }),
    tagged(2, 20, { location: 'roomB' }),
    tagged(3, 30, { location: 'roomA' }),
  ];
  const tagCol: TableColumn = { field: '$.tags.location', header: '위치', format: 'string' };

  it('드롭다운 값 목록을 태그 값에서 뽑는다', () => {
    expect(uniqueTableColumnValues(rows, tagCol)).toEqual(['roomA', 'roomB']);
  });

  it('고른 태그 값의 행만 남긴다', () => {
    const out = applyTableColumnFilters(rows, [tagCol], {
      '$.tags.location': valueFilter('roomA'),
    });
    expect(out).toHaveLength(2);
  });
});

describe('isRangeFilterColumn — 범위 UI 대상', () => {
  it('number / datetime 열은 범위로 거른다', () => {
    expect(isRangeFilterColumn({ field: 'value', header: 'V', format: 'number' })).toBe(true);
    expect(isRangeFilterColumn({ field: 'timestamp', header: 'T', format: 'datetime' })).toBe(true);
  });

  it('string 열과 형식 미지정은 값 선택으로 거른다', () => {
    expect(isRangeFilterColumn({ field: 'n', header: 'N', format: 'string' })).toBe(false);
    expect(isRangeFilterColumn({ field: 'n', header: 'N' })).toBe(false);
  });
});

describe('numericCellValue — 범위 비교값', () => {
  it('숫자는 그대로, 시각은 epoch ms 그대로', () => {
    const e = entry(1_700_000_000_000, 42, 'x');
    expect(numericCellValue(e, { field: 'value', header: 'V', format: 'number' })).toBe(42);
    expect(numericCellValue(e, { field: 'timestamp', header: 'T', format: 'datetime' }))
      .toBe(1_700_000_000_000);
  });

  it('수치 문자열은 변환하고, 비수치는 undefined', () => {
    const e = tagged(1, 0, { n: '12.5', s: 'abc' });
    expect(numericCellValue(e, { field: '$.tags.n', header: 'N', format: 'number' })).toBe(12.5);
    expect(numericCellValue(e, { field: '$.tags.s', header: 'S', format: 'number' })).toBeUndefined();
  });
});

describe('범위 필터', () => {
  const rows = [entry(100, 10, 'a'), entry(200, 25, 'b'), entry(300, 30, 'c')];
  const valueCol: TableColumn = { field: 'value', header: 'V', format: 'number' };
  const timeCol: TableColumn = { field: 'timestamp', header: 'T', format: 'datetime' };
  const values = (out: ChartEntry[]) => out.map((r) => (r as unknown as { value: number }).value);

  it('하한만 지정하면 그 이상만 남긴다 (경계 포함)', () => {
    expect(values(applyTableColumnFilters(rows, [valueCol], { value: rangeFilter(25) })))
      .toEqual([25, 30]);
  });

  it('상한만 지정하면 그 이하만 남긴다 (경계 포함)', () => {
    expect(values(applyTableColumnFilters(rows, [valueCol], { value: rangeFilter(undefined, 25) })))
      .toEqual([10, 25]);
  });

  it('하한·상한을 함께 지정하면 닫힌 구간이다', () => {
    expect(values(applyTableColumnFilters(rows, [valueCol], { value: rangeFilter(20, 29) })))
      .toEqual([25]);
  });

  it('timestamp 도 같은 방식으로 epoch ms 구간이다', () => {
    expect(values(applyTableColumnFilters(rows, [timeCol], { timestamp: rangeFilter(150, 250) })))
      .toEqual([25]);
  });

  it('경계가 둘 다 없으면 비활성이다 (원본 그대로)', () => {
    expect(applyTableColumnFilters(rows, [valueCol], { value: rangeFilter() })).toBe(rows);
  });

  it('수치로 볼 수 없는 행은 범위 필터에서 탈락한다', () => {
    const mixed = [
      tagged(1, 0, { n: '5' }),
      tagged(2, 0, { n: 'abc' }),
    ];
    const col: TableColumn = { field: '$.tags.n', header: 'N', format: 'number' };
    const out = applyTableColumnFilters(mixed, [col], { '$.tags.n': rangeFilter(0, 10) });
    expect(out).toHaveLength(1);
  });

  it('범위와 값 선택은 AND 로 결합된다', () => {
    const out = applyTableColumnFilters(
      rows,
      [valueCol],
      { value: { text: '', values: new Set(['30']), min: 20 } },
      0,
    );
    expect(values(out)).toEqual([30]);
  });
});

describe('범위 입력 변환 (datetime-local ↔ epoch ms)', () => {
  it('왕복 변환이 분 단위까지 보존된다', () => {
    const ms = new Date(2026, 3, 16, 14, 30, 0, 0).getTime();
    expect(localInputToEpoch(epochToLocalInput(ms))).toBe(ms);
  });

  it('빈 값/undefined 는 경계 없음으로 다룬다', () => {
    expect(epochToLocalInput(undefined)).toBe('');
    expect(localInputToEpoch('')).toBeUndefined();
    expect(localInputToEpoch('not-a-date')).toBeUndefined();
  });
});

describe('resizeColumnWidths — 경계 드래그', () => {
  const start = [100, 200, 300]; // 합 600

  it('인접 두 열만 주고받고 전체 폭은 변하지 않는다', () => {
    const out = resizeColumnWidths(start, 0, 50);
    expect(out).toEqual([150, 150, 300]);
    expect(out.reduce((a, b) => a + b, 0)).toBe(600);
  });

  it('음수 방향(왼쪽으로 끌기)도 같은 규칙이다', () => {
    expect(resizeColumnWidths(start, 1, -80)).toEqual([100, 120, 380]);
  });

  it('왼쪽 열이 최소 폭 아래로 내려가지 않는다', () => {
    const out = resizeColumnWidths(start, 0, -999);
    expect(out[0]).toBe(MIN_COLUMN_PX);
    expect(out[1]).toBe(600 - MIN_COLUMN_PX - 300);
  });

  it('오른쪽 열이 최소 폭 아래로 내려가지 않는다', () => {
    const out = resizeColumnWidths(start, 0, 999);
    expect(out[1]).toBe(MIN_COLUMN_PX);
    expect(out[0]).toBe(300 - MIN_COLUMN_PX);
  });

  it('마지막 경계 밖(상대가 없는 index)은 원본을 그대로 돌려준다', () => {
    expect(resizeColumnWidths(start, 2, 50)).toEqual(start);
  });

  it('dx 가 0 이면 변화가 없다', () => {
    expect(resizeColumnWidths(start, 1, 0)).toEqual(start);
  });
});

describe('applyColumnWidths — 드래그 결과 반영', () => {
  const cols: TableColumn[] = [
    { field: 'a', header: 'A' },
    { field: 'b', header: 'B', width: 5 },
    { field: 'c', header: 'C' },
  ];

  it('모든 열에 width 를 적는다 (일부만 적으면 자동 폭 열이 튄다)', () => {
    const out = applyColumnWidths(cols, [150, 150, 300]);
    expect(out.map((c) => c.width)).toEqual([150, 150, 300]);
  });

  it('소수는 반올림한다', () => {
    expect(applyColumnWidths(cols, [100.4, 100.6, 100]).map((c) => c.width))
      .toEqual([100, 101, 100]);
  });

  it('폭이 없거나 0 이하인 자리는 기존 열을 그대로 둔다', () => {
    const out = applyColumnWidths(cols, [120, 0]);
    expect(out[0]!.width).toBe(120);
    expect(out[1]!.width).toBe(5); // 원본 유지
    expect(out[2]!.width).toBeUndefined();
  });

  it('field/header 등 다른 속성은 보존한다', () => {
    const out = applyColumnWidths(cols, [10, 20, 30]);
    expect(out[1]).toMatchObject({ field: 'b', header: 'B' });
  });
});

describe('드래그 결과가 화면 비율로 이어진다', () => {
  it('픽셀 폭을 가중치로 그대로 써도 백분율이 맞는다', () => {
    const cols: TableColumn[] = [
      { field: 'a', header: 'A' },
      { field: 'b', header: 'B' },
    ];
    const resized = resizeColumnWidths([100, 300], 0, 100); // [200, 200]
    const out = columnWidthPercents(applyColumnWidths(cols, resized))!;
    expect(out).toEqual(['50.0000%', '50.0000%']);
  });
});
