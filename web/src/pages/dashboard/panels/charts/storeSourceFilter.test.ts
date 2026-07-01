// storeSourceFilter 순수 필터 로직 테스트.

import { describe, it, expect } from 'vitest';

import type { StoreKeyObject } from '@/services/api/store';
import {
  distinctDataTypes,
  distinctMetricTypes,
  filterStoreKeyObjects,
  makeTagFilterId,
  matchesTagFilters,
  sortStoreKeyObjects,
} from './storeSourceFilter';

function obj(o: Partial<StoreKeyObject> & { key: string }): StoreKeyObject {
  return {
    registration: 'manual',
    data_type: 'float',
    metric_type: 'gauge',
    tags: {},
    ...o,
  };
}

const objects: StoreKeyObject[] = [
  obj({ key: 'room:1:temp', metric_type: 'gauge', data_type: 'float', tags: { room: '1', type: 'temperature' } }),
  obj({ key: 'room:2:temp', metric_type: 'gauge', data_type: 'float', tags: { room: '2', type: 'temperature' } }),
  obj({ key: 'room:1:humidity', metric_type: 'counter', data_type: 'int', tags: { room: '1', type: 'humidity' } }),
  obj({ key: 'system:status', metric_type: 'state', data_type: 'string', tags: {} }),
];

describe('filterStoreKeyObjects', () => {
  it('빈 필터면 전체를 원본 순서로 반환한다', () => {
    expect(filterStoreKeyObjects(objects, {})).toEqual(objects);
  });

  it('key 이름 부분 검색(대소문자 무시)', () => {
    const r = filterStoreKeyObjects(objects, { search: 'TEMP' });
    expect(r.map((o) => o.key)).toEqual(['room:1:temp', 'room:2:temp']);
  });

  it('metric_type 정확 일치 필터', () => {
    const r = filterStoreKeyObjects(objects, { metricType: 'counter' });
    expect(r.map((o) => o.key)).toEqual(['room:1:humidity']);
  });

  it('data_type 정확 일치 필터', () => {
    const r = filterStoreKeyObjects(objects, { dataType: 'string' });
    expect(r.map((o) => o.key)).toEqual(['system:status']);
  });

  it('단일 tag 필터(AND 단일 페어)', () => {
    const r = filterStoreKeyObjects(objects, {
      tagFilters: new Set([makeTagFilterId('room', '1')]),
    });
    expect(r.map((o) => o.key)).toEqual(['room:1:temp', 'room:1:humidity']);
  });

  it('같은 tagKey 의 여러 값은 OR, 다른 tagKey 끼리는 AND', () => {
    const r = filterStoreKeyObjects(objects, {
      tagFilters: new Set([
        makeTagFilterId('room', '1'),
        makeTagFilterId('room', '2'),
        makeTagFilterId('type', 'temperature'),
      ]),
    });
    // (room∈{1,2}) AND (type=temperature)
    expect(r.map((o) => o.key)).toEqual(['room:1:temp', 'room:2:temp']);
  });

  it('모든 필터를 AND 로 결합한다', () => {
    const r = filterStoreKeyObjects(objects, {
      search: 'room',
      metricType: 'gauge',
      dataType: 'float',
      tagFilters: new Set([makeTagFilterId('room', '1')]),
    });
    expect(r.map((o) => o.key)).toEqual(['room:1:temp']);
  });
});

describe('matchesTagFilters', () => {
  it('빈 필터셋이면 항상 true', () => {
    expect(matchesTagFilters(objects[0]!, new Set())).toBe(true);
  });
  it('태그가 없는 객체는 태그 필터가 있으면 false', () => {
    expect(
      matchesTagFilters(objects[3]!, new Set([makeTagFilterId('room', '1')])),
    ).toBe(false);
  });
});

describe('distinctMetricTypes / distinctDataTypes', () => {
  it('distinct + 정렬된 metric_type 목록', () => {
    expect(distinctMetricTypes(objects)).toEqual(['counter', 'gauge', 'state']);
  });
  it('distinct + 정렬된 data_type 목록', () => {
    expect(distinctDataTypes(objects)).toEqual(['float', 'int', 'string']);
  });
});

describe('sortStoreKeyObjects', () => {
  it('sort 가 null 이면 원본 순서를 보존한다', () => {
    expect(sortStoreKeyObjects(objects, null)).toEqual(objects);
  });

  it('key asc/desc 정렬', () => {
    const asc = sortStoreKeyObjects(objects, { field: 'key', order: 'asc' });
    expect(asc.map((o) => o.key)).toEqual([
      'room:1:humidity',
      'room:1:temp',
      'room:2:temp',
      'system:status',
    ]);
    const desc = sortStoreKeyObjects(objects, { field: 'key', order: 'desc' });
    expect(desc.map((o) => o.key)).toEqual([
      'system:status',
      'room:2:temp',
      'room:1:temp',
      'room:1:humidity',
    ]);
  });

  it('metric_type 기준 정렬', () => {
    const asc = sortStoreKeyObjects(objects, { field: 'metric_type', order: 'asc' });
    expect(asc.map((o) => o.metric_type)).toEqual([
      'counter',
      'gauge',
      'gauge',
      'state',
    ]);
  });

  it('data_type 기준 정렬', () => {
    const asc = sortStoreKeyObjects(objects, { field: 'data_type', order: 'asc' });
    expect(asc.map((o) => o.data_type)).toEqual(['float', 'float', 'int', 'string']);
  });

  it('tags 기준 정렬(키 사전순 직렬화)', () => {
    const asc = sortStoreKeyObjects(objects, { field: 'tags', order: 'asc' });
    // 직렬화: "room=1,type=humidity" < "room=1,type=temperature" < "room=2,type=temperature" < ""(system).
    // 빈 태그("")가 가장 앞으로 정렬된다.
    expect(asc[0]!.key).toBe('system:status');
  });

  it('원본 배열을 변형하지 않는다(불변)', () => {
    const before = objects.map((o) => o.key);
    sortStoreKeyObjects(objects, { field: 'key', order: 'desc' });
    expect(objects.map((o) => o.key)).toEqual(before);
  });
});
