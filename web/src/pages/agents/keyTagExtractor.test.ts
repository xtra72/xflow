// keyTagExtractor 단위 테스트 — 키 패턴별 태그 자동 추출, 정적 태그 우선순위,
// 페어 집계 동작을 검증한다.
//
// @spec SPEC-WEB-005

import { describe, expect, it } from 'vitest';

import {
  buildExtractedTagPairs,
  buildExtractedTagsByKey,
  extractTagsFromKey,
} from './keyTagExtractor';

describe('extractTagsFromKey', () => {
  it('InfluxDB 스타일: measurement + 태그 페어 추출', () => {
    expect(extractTagsFromKey('temp,room=1,sensor=A')).toEqual({
      measurement: 'temp',
      room: '1',
      sensor: 'A',
    });
  });

  it('InfluxDB 스타일: measurement 만 있고 태그가 없으면 measurement 만 반환', () => {
    // 콤마는 있지만 `=` 가 없는 경우 InfluxDB 패턴으로 인식하지 않는다.
    // → 다음 패턴 (colon/slash) 도 일치하지 않으므로 빈 객체.
    expect(extractTagsFromKey('temp,extra')).toEqual({});
  });

  it('InfluxDB 스타일: 빈 키/값은 무시', () => {
    // `=` 만 있는 토큰 (`k=`) 또는 시작이 `=` 인 경우 무시.
    expect(extractTagsFromKey('temp,room=,=v,room=2')).toEqual({
      measurement: 'temp',
      room: '2',
    });
  });

  it('Colon-separated: 세그먼트를 seg0..seg{n} 으로 매핑', () => {
    expect(extractTagsFromKey('indoor:1:room_temp')).toEqual({
      seg0: 'indoor',
      seg1: '1',
      seg2: 'room_temp',
    });
  });

  it('사용자 지정 구분자 `/` 로 세그먼트 분리', () => {
    expect(extractTagsFromKey('indoor/1/room_temp', '/')).toEqual({
      seg0: 'indoor',
      seg1: '1',
      seg2: 'room_temp',
    });
  });

  it('기본 구분자(`:`)에서 키에 콜론이 없으면 빈 객체 (폴백 없음)', () => {
    // 폴백을 두지 않으므로 `/` 가 있어도 자동 매칭하지 않는다.
    expect(extractTagsFromKey('indoor/1/room_temp')).toEqual({});
  });

  it('구조 없음: 빈 객체 반환', () => {
    expect(extractTagsFromKey('plain_key')).toEqual({});
  });

  it('빈 문자열: 빈 객체 반환', () => {
    expect(extractTagsFromKey('')).toEqual({});
  });

  it('우선순위: InfluxDB 가 사용자 구분자보다 우선', () => {
    // 콤마 + `=` + `:` + `/` 가 모두 포함된 경우 InfluxDB 로 우선 인식.
    expect(extractTagsFromKey('m,k=v:rest/path')).toEqual({
      measurement: 'm',
      k: 'v:rest/path',
    });
  });

  it('사용자 구분자가 키에 없으면 빈 객체 (폴백 없음)', () => {
    expect(extractTagsFromKey('indoor:1:roomtemp', '_')).toEqual({});
    expect(extractTagsFromKey('indoor:1:roomtemp', '.')).toEqual({});
  });
});

describe('buildExtractedTagPairs', () => {
  it('여러 키를 가로질러 동일 태그 키의 값을 집계한다', () => {
    const keys = ['temp,room=1,sensor=A', 'temp,room=2,sensor=B', 'humid,room=1'];
    const pairs = buildExtractedTagPairs(keys, {});
    // 알파벳 순 정렬: measurement, room, sensor
    expect(pairs).toEqual([
      { key: 'measurement', values: ['humid', 'temp'] },
      { key: 'room', values: ['1', '2'] },
      { key: 'sensor', values: ['A', 'B'] },
    ]);
  });

  it('정적 태그가 있는 키도 자동 추출과 병합된다 (정적 태그 우선)', () => {
    const keys = ['device:1:temp'];
    const staticTags = {
      'device:1:temp': { type: 'temperature', floor: '3' },
    };
    const pairs = buildExtractedTagPairs(keys, staticTags);
    // 자동 추출의 seg0/seg1/seg2 와 정적 태그(floor, type) 가 모두 노출된다.
    expect(pairs).toEqual([
      { key: 'floor', values: ['3'] },
      { key: 'seg0', values: ['device'] },
      { key: 'seg1', values: ['1'] },
      { key: 'seg2', values: ['temp'] },
      { key: 'type', values: ['temperature'] },
    ]);
  });

  it('Mixed: 일부 키는 정적 태그+자동 추출, 나머지는 자동 추출만', () => {
    const keys = ['device:1:temp', 'temp,room=2'];
    const staticTags = {
      'device:1:temp': { type: 'temperature' },
    };
    const pairs = buildExtractedTagPairs(keys, staticTags);
    expect(pairs).toEqual([
      { key: 'measurement', values: ['temp'] },
      { key: 'room', values: ['2'] },
      { key: 'seg0', values: ['device'] },
      { key: 'seg1', values: ['1'] },
      { key: 'seg2', values: ['temp'] },
      { key: 'type', values: ['temperature'] },
    ]);
  });

  it('사용자 구분자 변경: 정적 태그가 있는 키도 새 구분자가 적용된다', () => {
    const keys = ['indoor/1/room_temp'];
    const staticTags = {
      'indoor/1/room_temp': { room: '우리집' },
    };
    const pairs = buildExtractedTagPairs(keys, staticTags, '/');
    // `/` 구분자 자동 추출 (seg0..seg2) + 정적 태그 (room) 병합.
    expect(pairs).toEqual([
      { key: 'room', values: ['우리집'] },
      { key: 'seg0', values: ['indoor'] },
      { key: 'seg1', values: ['1'] },
      { key: 'seg2', values: ['room_temp'] },
    ]);
  });

  it('빈 키 목록은 빈 페어 배열을 반환', () => {
    expect(buildExtractedTagPairs([], {})).toEqual([]);
  });

  it('구조 없는 키만 있으면 빈 페어 배열을 반환', () => {
    expect(buildExtractedTagPairs(['foo', 'bar'], {})).toEqual([]);
  });
});

describe('buildExtractedTagsByKey', () => {
  it('각 키에 대해 자동 추출 + 정적 태그 병합 (정적 태그 우선)', () => {
    const keys = ['temp,room=1', 'device:2:meta'];
    const staticTags: Record<string, Record<string, string>> = {
      'temp,room=1': { customTag: 'override' },
    };
    const result = buildExtractedTagsByKey(keys, staticTags);
    expect(result).toEqual({
      // InfluxDB 자동 추출(measurement, room) + 정적 태그(customTag) 병합.
      'temp,room=1': {
        measurement: 'temp',
        room: '1',
        customTag: 'override',
      },
      'device:2:meta': { seg0: 'device', seg1: '2', seg2: 'meta' },
    });
  });

  it('정적 태그와 자동 추출 키가 충돌하면 정적 태그가 우선', () => {
    const keys = ['indoor:1:room_temp'];
    const staticTags = {
      // seg0 키가 충돌하는 경우, 정적 태그 값 ('outdoor') 이 우선.
      'indoor:1:room_temp': { seg0: 'outdoor' },
    };
    const result = buildExtractedTagsByKey(keys, staticTags);
    expect(result['indoor:1:room_temp']).toEqual({
      seg0: 'outdoor', // 정적 태그가 자동 추출 ('indoor') 을 덮어씀.
      seg1: '1',
      seg2: 'room_temp',
    });
  });

  it('구조 없는 키도 항목으로 포함되며 빈 객체 매핑된다', () => {
    const result = buildExtractedTagsByKey(['plain'], {});
    expect(result).toEqual({ plain: {} });
  });
});
