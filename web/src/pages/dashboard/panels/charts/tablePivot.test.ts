// 시각 기준 행(넓은 형식) 변환 테스트.
//
// 축을 돌리는 변환은 조용히 틀리기 쉽다 — 시각이 어긋나면 행이 갈라지고, 같은 시각에
// 값이 두 번 오면 어느 쪽이 남는지가 규약이다. 그 규약을 여기서 잠근다.

import { describe, expect, it } from 'vitest';

import type { ChartEntry, TableColumn } from './chartChannelTypes';
import {
  entrySeriesName,
  pivotByTimestamp,
  pivotColumns,
  resolveSeriesValue,
  seriesFieldPath,
  seriesNameOfField,
} from './tablePivot';

/** store 경로가 싣는 형상(meta.seriesName + labels.name). */
function entry(ts: number, name: string, value: unknown): ChartEntry {
  return { timestamp: ts, value, labels: { name }, meta: { seriesName: name } };
}

describe('시리즈 열 필드 표기', () => {
  it('시리즈 이름을 필드 경로로 바꾸고 되읽는다', () => {
    expect(seriesFieldPath('온도')).toBe('$.series.온도');
    expect(seriesNameOfField('$.series.온도')).toBe('온도');
  });

  it('점·공백이 든 이름도 그대로 왕복한다 — 점 경로로는 읽을 수 없는 이름이다', () => {
    const name = 'room 1.temp';
    expect(seriesNameOfField(seriesFieldPath(name))).toBe(name);
  });

  it('시리즈 열이 아니면 undefined', () => {
    expect(seriesNameOfField('value')).toBeUndefined();
    expect(seriesNameOfField('$.tags.room')).toBeUndefined();
    expect(seriesNameOfField('$.series.')).toBeUndefined();
  });
});

describe('시리즈 이름 판정', () => {
  it('meta.seriesName 이 정본이다 — 범례·라인 이름과 같은 값', () => {
    expect(entrySeriesName(entry(1, '온도', 1), 'fb')).toBe('온도');
  });

  it('채널 경로처럼 meta 가 없으면 labels.name 을 쓴다', () => {
    expect(entrySeriesName({ timestamp: 1, value: 1, labels: { name: '습도' } }, 'fb')).toBe('습도');
  });

  it('둘 다 없으면 폴백 이름 한 줄로 본다', () => {
    expect(entrySeriesName({ timestamp: 1, value: 1 }, '값')).toBe('값');
  });
});

describe('pivotByTimestamp', () => {
  it('같은 시각의 여러 시리즈를 한 행으로 접는다', () => {
    const { rows, seriesNames } = pivotByTimestamp(
      [entry(1_000, '온도', 21.5), entry(1_000, '습도', 40), entry(2_000, '온도', 22)],
      '값',
    );
    expect(seriesNames).toEqual(['온도', '습도']);
    expect(rows).toHaveLength(2);
    expect(rows[0]).toMatchObject({ timestamp: 1_000, series: { 온도: 21.5, 습도: 40 } });
    // 2초 행에는 습도가 없다 — 셀은 빈칸이 된다.
    expect(rows[1]?.series).toEqual({ 온도: 22 });
  });

  it('행은 시각 오름차순이다 (입력 순서와 무관)', () => {
    const { rows } = pivotByTimestamp(
      [entry(3_000, 'a', 3), entry(1_000, 'a', 1), entry(2_000, 'a', 2)],
      '값',
    );
    expect(rows.map((r) => r.timestamp)).toEqual([1_000, 2_000, 3_000]);
  });

  it('시리즈 열 순서는 등장 순서를 지킨다', () => {
    const { seriesNames } = pivotByTimestamp(
      [entry(1, 'c', 1), entry(1, 'a', 1), entry(2, 'b', 1), entry(2, 'a', 2)],
      '값',
    );
    expect(seriesNames).toEqual(['c', 'a', 'b']);
  });

  it('같은 시각·같은 시리즈에 값이 둘이면 마지막이 남는다', () => {
    const { rows } = pivotByTimestamp([entry(1, 'a', 1), entry(1, 'a', 9)], '값');
    expect(rows[0]?.series).toEqual({ a: 9 });
  });

  it('시각이 어긋나면 행이 갈라진다 — 임의 허용 오차로 묶지 않는다', () => {
    const { rows } = pivotByTimestamp([entry(1_000, 'a', 1), entry(1_001, 'b', 2)], '값');
    expect(rows).toHaveLength(2);
  });

  it('수로 볼 수 없는 값은 null 이다 (셀은 빈칸)', () => {
    const { rows } = pivotByTimestamp(
      [entry(1, 'a', 'abc'), entry(1, 'b', null), entry(1, 'c', true), entry(1, 'd', '3.5')],
      '값',
    );
    expect(rows[0]?.series).toEqual({ a: null, b: null, c: 1, d: 3.5 });
  });

  it('시각이 수가 아닌 엔트리는 건너뛴다', () => {
    const bad = { timestamp: Number.NaN, value: 1, meta: { seriesName: 'a' } } as ChartEntry;
    const { rows } = pivotByTimestamp([bad, entry(1, 'a', 1)], '값');
    expect(rows).toHaveLength(1);
  });

  it('행에는 단일 value 가 남지 않는다 — 어느 시리즈인지 모르는 수가 앉으면 안 된다', () => {
    const { rows } = pivotByTimestamp([entry(1, 'a', 1), entry(1, 'b', 2)], '값');
    expect(rows[0]?.value).toBeUndefined();
  });

  it('피벗 행에서 시리즈 값을 읽는다', () => {
    const { rows } = pivotByTimestamp([entry(1, 'a', 7)], '값');
    expect(resolveSeriesValue(rows[0]!, 'a')).toBe(7);
    expect(resolveSeriesValue(rows[0]!, 'none')).toBeUndefined();
    // 피벗 행이 아닌 엔트리에는 시리즈 칸이 없다.
    expect(resolveSeriesValue(entry(1, 'a', 7), 'a')).toBeUndefined();
  });
});

describe('pivotColumns', () => {
  it('시각 열 + 시리즈 열을 만든다', () => {
    const cols = pivotColumns(['온도', '습도'], [], '시간');
    expect(cols).toEqual([
      { field: 'timestamp', header: '시간', format: 'datetime' },
      { field: '$.series.온도', header: '온도', format: 'number', unit: undefined },
      { field: '$.series.습도', header: '습도', format: 'number', unit: undefined },
    ]);
  });

  it('공통 단위를 시리즈 열에만 붙인다', () => {
    const cols = pivotColumns(['온도'], [], '시간', '°C');
    expect(cols[0]?.unit).toBeUndefined();
    expect(cols[1]?.unit).toBe('°C');
  });

  it('저장된 폭·이름을 물려받는다 — 헤더 드래그 폭이 유지된다', () => {
    const saved: TableColumn[] = [
      { field: 'timestamp', header: '옛 이름', width: 120 },
      { field: '$.series.온도', header: '실내온도', width: 80 },
    ];
    const cols = pivotColumns(['온도'], saved, '시간');
    // 시각 열 이름은 설정값이 이긴다(전용 입력칸이 있다). 폭은 물려받는다.
    expect(cols[0]).toMatchObject({ header: '시간', width: 120 });
    // 시리즈 열 이름은 사용자가 고쳤으면 그 이름이 이긴다.
    expect(cols[1]).toMatchObject({ header: '실내온도', width: 80 });
  });

  it('사라진 시리즈의 열은 빠진다', () => {
    const saved: TableColumn[] = [{ field: '$.series.옛시리즈', header: '옛', width: 50 }];
    const cols = pivotColumns(['새시리즈'], saved, '시간');
    expect(cols.map((c) => c.field)).toEqual(['timestamp', '$.series.새시리즈']);
  });

  it('시각 열 형식은 고정이다 — 수로 찍으면 epoch ms 원값이 나온다', () => {
    const saved: TableColumn[] = [{ field: 'timestamp', header: 't', format: 'number' }];
    expect(pivotColumns([], saved, '시간')[0]?.format).toBe('datetime');
  });
});
