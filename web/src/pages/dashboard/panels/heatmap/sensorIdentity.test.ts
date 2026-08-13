// 센서 동일성 키 + sensor_positions 마이그레이션 테스트.
//
// 결함: 좌표가 store key 하나로 키잉되어, 한 key 를 공유하는 여러 시리즈가 좌표를 공유하고
// (같이 움직임), 하나를 해제하면 형제의 좌표까지 삭제되어 히트맵이 비었다. 여기서는 재키잉의
// 순수 로직(동일성/라벨/이관 규칙/멱등성)을 커버한다.

import { describe, expect, it } from 'vitest';

import type { StoreSeriesRef } from '../charts/chartChannelTypes';
import { heatmapSensorId, migrateSensorPositions, sensorSeriesLabel } from './sensorIdentity';

/** 테스트 가독성용 시리즈 생성기. */
function ref(
  key: string,
  metric?: string,
  tags?: Record<string, string>,
  alias?: string,
): StoreSeriesRef {
  return { key, metric_type: metric, tags, alias } as StoreSeriesRef;
}

describe('heatmapSensorId', () => {
  it('같은 key 라도 metric/tags 가 다르면 서로 다른 동일성 키를 만든다', () => {
    const a = heatmapSensorId(ref('temp', 'temperature', { room: '1' }));
    const b = heatmapSensorId(ref('temp', 'temperature', { room: '2' }));
    const c = heatmapSensorId(ref('temp', 'humidity', { room: '1' }));
    expect(a).not.toBe(b);
    expect(a).not.toBe(c);
    expect(b).not.toBe(c);
  });

  it('태그 삽입 순서가 달라도 같은 태그 집합이면 같은 키다(정렬 직렬화)', () => {
    const a = heatmapSensorId(ref('t', 'm', { b: '2', a: '1' }));
    const b = heatmapSensorId(ref('t', 'm', { a: '1', b: '2' }));
    expect(a).toBe(b);
  });

  it('metric/tags 미지정은 빈 값으로 정규화되어 재현 가능하다(리로드 안정성)', () => {
    expect(heatmapSensorId(ref('t'))).toBe(heatmapSensorId(ref('t', undefined, {})));
    expect(heatmapSensorId(ref('t', '', {}))).toBe(heatmapSensorId(ref('t')));
  });

  it('alias(사용자 편집 이름)는 동일성에 영향을 주지 않는다', () => {
    expect(heatmapSensorId(ref('t', 'm', { a: '1' }, '거실'))).toBe(
      heatmapSensorId(ref('t', 'm', { a: '1' }, '안방')),
    );
  });

  it('JSON 객체 키로 왕복해도 보존된다(불투명 JSON config 로 영속)', () => {
    const id = heatmapSensorId(ref('t', 'm', { a: '1', b: 'x y' }));
    const round = JSON.parse(JSON.stringify({ [id]: { x: 0.1, y: 0.2 } })) as Record<
      string,
      unknown
    >;
    expect(Object.keys(round)).toEqual([id]);
  });
});

describe('sensorSeriesLabel', () => {
  it('alias 가 있으면 alias, 없거나 공백이면 key 를 쓴다', () => {
    expect(sensorSeriesLabel({ key: 'k', alias: '거실' })).toBe('거실');
    expect(sensorSeriesLabel({ key: 'k', alias: '   ' })).toBe('k');
    expect(sensorSeriesLabel({ key: 'k' })).toBe('k');
  });
});

describe('migrateSensorPositions', () => {
  it('명확(unambiguous): key 를 가진 체크 시리즈가 1개면 동일성 키로 옮긴다', () => {
    const series = [ref('temp', 'temperature', { room: '1' })];
    const id = heatmapSensorId(series[0]!);
    const r = migrateSensorPositions({ temp: { x: 0.2, y: 0.3 } }, series);
    expect(r.changed).toBe(true);
    expect(r.positions).toEqual({ [id]: { x: 0.2, y: 0.3 } });
    expect(r.positions['temp']).toBeUndefined();
    expect(r.ambiguousKeys).toEqual([]);
  });

  it('모호(ambiguous): key 를 공유하는 시리즈가 여럿이면 config 순서상 첫 시리즈로 결정적으로 귀속한다', () => {
    // 옛 데이터에는 이 좌표가 어느 형제의 것인지 정보가 없다. 무작위 선택 대신 첫 시리즈로
    // 귀속시켜 "옛 렌더(한 점)"를 보존하고, 나머지 형제는 미배치로 남겨 UI 가 안내하게 한다.
    const first = ref('temp', 'temperature', { room: '1' });
    const second = ref('temp', 'temperature', { room: '2' });
    const r = migrateSensorPositions({ temp: { x: 0.4, y: 0.6 } }, [first, second]);
    expect(r.positions).toEqual({ [heatmapSensorId(first)]: { x: 0.4, y: 0.6 } });
    expect(r.positions[heatmapSensorId(second)]).toBeUndefined();
    expect(r.ambiguousKeys).toEqual(['temp']);
    // 결정적: series 순서가 같으면 몇 번을 돌려도 같은 결과.
    const again = migrateSensorPositions({ temp: { x: 0.4, y: 0.6 } }, [first, second]);
    expect(again.positions).toEqual(r.positions);
  });

  it('고아(orphan): 체크된 시리즈와 맞지 않는 좌표는 보존한다(무해 + 재선택 시 복구 가능)', () => {
    const series = [ref('temp', 'temperature', { room: '1' })];
    const id = heatmapSensorId(series[0]!);
    const r = migrateSensorPositions(
      { temp: { x: 0.2, y: 0.3 }, gone: { x: 0.9, y: 0.9 } },
      series,
    );
    expect(r.positions).toEqual({
      [id]: { x: 0.2, y: 0.3 },
      gone: { x: 0.9, y: 0.9 },
    });
  });

  it('고아 좌표는 나중에 그 key 를 다시 체크하면 다시 이관된다', () => {
    const orphan = { gone: { x: 0.9, y: 0.9 } };
    // 1) 체크된 시리즈 없음 → 그대로 보존.
    expect(migrateSensorPositions(orphan, []).positions).toEqual(orphan);
    // 2) 같은 key 를 다시 체크 → 동일성 키로 이관되어 좌표가 되살아난다.
    const back = [ref('gone', 'temperature', {})];
    const r = migrateSensorPositions(orphan, back);
    expect(r.positions).toEqual({ [heatmapSensorId(back[0]!)]: { x: 0.9, y: 0.9 } });
  });

  it('멱등: 두 번 실행해도 결과가 같고 두 번째는 변경 없음으로 보고한다', () => {
    const series = [ref('temp', 'temperature', { room: '1' }), ref('hum', 'humidity', {})];
    const once = migrateSensorPositions(
      { temp: { x: 0.2, y: 0.3 }, hum: { x: 0.7, y: 0.1 }, orphan: { x: 0, y: 0 } },
      series,
    );
    const twice = migrateSensorPositions(once.positions, series);
    expect(twice.positions).toEqual(once.positions);
    expect(twice.changed).toBe(false);
    // 3회차도 동일(수렴).
    expect(migrateSensorPositions(twice.positions, series).positions).toEqual(once.positions);
  });

  it('이미 동일성 키로 저장된 값이 raw key 값보다 우선한다(입력 키 순서와 무관)', () => {
    const series = [ref('temp', 'temperature', { room: '1' })];
    const id = heatmapSensorId(series[0]!);
    const rawFirst = migrateSensorPositions(
      { temp: { x: 0.1, y: 0.1 }, [id]: { x: 0.9, y: 0.9 } },
      series,
    );
    const idFirst = migrateSensorPositions(
      { [id]: { x: 0.9, y: 0.9 }, temp: { x: 0.1, y: 0.1 } },
      series,
    );
    expect(rawFirst.positions).toEqual({ [id]: { x: 0.9, y: 0.9 } });
    expect(idFirst.positions).toEqual(rawFirst.positions);
  });

  it('변경이 없으면 입력 객체 참조를 그대로 돌려준다(불필요한 재렌더 방지)', () => {
    const series = [ref('temp', 'temperature', { room: '1' })];
    const positions = { [heatmapSensorId(series[0]!)]: { x: 0.2, y: 0.3 } };
    expect(migrateSensorPositions(positions, series).positions).toBe(positions);
  });

  it('series 가 없거나 배열이 아니어도 예외 없이 입력을 보존한다(손상 config 방어)', () => {
    const positions = { temp: { x: 0.2, y: 0.3 } };
    expect(migrateSensorPositions(positions, undefined).positions).toEqual(positions);
    expect(
      migrateSensorPositions(positions, 'nope' as unknown as StoreSeriesRef[]).positions,
    ).toEqual(positions);
  });
});
