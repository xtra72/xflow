// 센서-좌표 결합 순수 로직 단위 테스트 (SPEC-HEATMAP-PANEL-001 T6).
// AC-E2(좌표 미지정 센서 제외 + 노출), 최신값 추출, autoBounds 커버.

import { describe, it, expect } from 'vitest';

import type { ChartEntry, StoreSeriesRef } from '../charts/chartChannelTypes';
import { latestFiniteValue, joinSensorPoints, resolveSensorSeries } from './heatmapJoin';
import { heatmapSensorId } from './sensorIdentity';

/** 타임라인 생성 헬퍼(값 배열 → ChartEntry[]). */
function series(values: unknown[]): ChartEntry[] {
  return values.map((value, i) => ({ timestamp: i, value }));
}

describe('resolveSensorSeries — 조회 이름 공간 → 센서 동일성 키 공간', () => {
  const A = { key: 'dup', metric_type: 'temperature', tags: { room: 'A' } } as StoreSeriesRef;
  const B = { key: 'dup', metric_type: 'temperature', tags: { room: 'B' } } as StoreSeriesRef;

  it('컬럼과 config.series 가 1:1 이면 인덱스로 짝지어 동일성 키를 붙인다', () => {
    // 훅이 어떤 표시 이름을 쓰든(여기서는 둘 다 기본 alias 'dup') 인덱스로 결합한다.
    const r = resolveSensorSeries(
      ['dup', 'dup#2'],
      new Map([
        ['dup', series([20])],
        ['dup#2', series([26])],
      ]),
      [A, B],
    );
    expect(r.aligned).toBe(true);
    expect(r.ids).toEqual([heatmapSensorId(A), heatmapSensorId(B)]);
    // 형제의 타임라인이 서로 섞이지 않는다.
    expect(latestFiniteValue(r.entriesById.get(heatmapSensorId(A)))).toBe(20);
    expect(latestFiniteValue(r.entriesById.get(heatmapSensorId(B)))).toBe(26);
  });

  it('타임라인이 없는 시리즈도 동일성 키는 부여된다(미배치/미판독 구분은 join 이 담당)', () => {
    const r = resolveSensorSeries(['dup'], new Map(), [A]);
    expect(r.ids).toEqual([heatmapSensorId(A)]);
    expect(r.entriesById.size).toBe(0);
  });

  it('정렬 불가(한 key 가 다중 컬럼으로 확장)면 조회 이름 공간을 그대로 통과시킨다', () => {
    // 컬럼 2개 vs 요청 시리즈 1개 → 인덱스 짝짓기가 불가능하므로 이름을 그대로 쓴다.
    const r = resolveSensorSeries(
      ['dup · temperature{room=A}', 'dup · temperature{room=B}'],
      new Map([['dup · temperature{room=A}', series([20])]]),
      [A],
    );
    expect(r.aligned).toBe(false);
    expect(r.ids).toEqual(['dup · temperature{room=A}', 'dup · temperature{room=B}']);
    // 동일성 키와 맞지 않으므로 좌표 매칭은 실패한다(전부 미배치로 graceful degrade).
    const join = joinSensorPoints(r.ids, r.entriesById, {
      [heatmapSensorId(A)]: { x: 0.5, y: 0.5 },
    });
    expect(join.points).toEqual([]);
    expect(join.unplacedNames).toEqual(['dup · temperature{room=A}']);
  });
});

describe('latestFiniteValue', () => {
  it('마지막 유한 숫자값을 반환한다', () => {
    expect(latestFiniteValue(series([20, 21, 22]))).toBe(22);
  });

  it('후행 null/비숫자는 건너뛰고 마지막 유한값을 찾는다', () => {
    expect(latestFiniteValue(series([20, 21, null, 'x', Infinity]))).toBe(21);
  });

  it('유한값이 없으면 null 을 반환한다', () => {
    expect(latestFiniteValue(series([null, NaN, 'a']))).toBeNull();
    expect(latestFiniteValue(undefined)).toBeNull();
    expect(latestFiniteValue([])).toBeNull();
  });
});

describe('joinSensorPoints', () => {
  it('좌표가 배치된 센서만 보간 입력점이 되고, 미배치 센서는 unplacedNames 로 분리된다(AC-E2)', () => {
    const seriesNames = ['s1', 's2', 's3', 's4'];
    const entries = new Map<string, ChartEntry[]>([
      ['s1', series([20])],
      ['s2', series([24])],
      ['s3', series([22])],
      ['s4', series([26])], // 좌표 없음 → unplaced.
    ]);
    const positions = {
      s1: { x: 0.1, y: 0.1 },
      s2: { x: 0.9, y: 0.1 },
      s3: { x: 0.5, y: 0.9 },
    };
    const r = joinSensorPoints(seriesNames, entries, positions);

    expect(r.points).toEqual([
      { x: 0.1, y: 0.1, value: 20 },
      { x: 0.9, y: 0.1, value: 24 },
      { x: 0.5, y: 0.9, value: 22 },
    ]);
    expect(r.placedNames).toEqual(['s1', 's2', 's3']);
    expect(r.unplacedNames).toEqual(['s4']);
  });

  it('최신 판독값이 없는 시리즈는 placed/unplaced 어디에도 넣지 않는다', () => {
    const entries = new Map<string, ChartEntry[]>([
      ['s1', series([null, NaN])], // 판독값 없음.
      ['s2', series([25])],
    ]);
    const r = joinSensorPoints(['s1', 's2'], entries, { s1: { x: 0.2, y: 0.2 } });
    // s1 은 좌표가 있어도 판독값이 없어 제외, s2 는 좌표가 없어 unplaced.
    expect(r.points).toEqual([]);
    expect(r.placedNames).toEqual([]);
    expect(r.unplacedNames).toEqual(['s2']);
  });

  it('autoBounds 는 배치 센서값의 [min, max] 다', () => {
    const entries = new Map<string, ChartEntry[]>([
      ['a', series([18])],
      ['b', series([30])],
      ['c', series([24])],
    ]);
    const positions = {
      a: { x: 0, y: 0 },
      b: { x: 1, y: 1 },
      c: { x: 0.5, y: 0.5 },
    };
    const r = joinSensorPoints(['a', 'b', 'c'], entries, positions);
    expect(r.autoBounds).toEqual({ min: 18, max: 30 });
  });

  it('배치 센서가 0개면 autoBounds 는 null, points 는 빈 배열이다(AC-E1 입력)', () => {
    const entries = new Map<string, ChartEntry[]>([['a', series([20])]]);
    const r = joinSensorPoints(['a'], entries, {}); // 좌표 전무.
    expect(r.points).toEqual([]);
    expect(r.autoBounds).toBeNull();
    expect(r.unplacedNames).toEqual(['a']);
  });

  it('빈 시리즈 목록은 전부 빈 결과다', () => {
    const r = joinSensorPoints([], new Map(), {});
    expect(r.points).toEqual([]);
    expect(r.placedNames).toEqual([]);
    expect(r.unplacedNames).toEqual([]);
    expect(r.autoBounds).toBeNull();
  });
});
