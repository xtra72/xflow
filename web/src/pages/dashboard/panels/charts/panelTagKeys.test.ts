// 표의 태그 열 후보 추출 — 설정에 적힌 태그를 빠짐없이, 중복 없이 모으는가.

import { describe, expect, it } from 'vitest';

import {
  panelTagKeys,
  SYSMETRICS_TAG_KEYS,
  tagFieldPath,
  tagKeyOfField,
} from './panelTagKeys';

describe('태그 열 필드 표기', () => {
  it('태그 키를 필드 경로로 바꾼다', () => {
    expect(tagFieldPath('location')).toBe('$.tags.location');
  });

  it('필드 경로에서 태그 키를 되읽는다', () => {
    expect(tagKeyOfField('$.tags.location')).toBe('location');
  });

  it('태그 열이 아니면 undefined', () => {
    expect(tagKeyOfField('value')).toBeUndefined();
    expect(tagKeyOfField('labels.name')).toBeUndefined();
    // 접두만 있고 키가 없는 것은 태그 열이 아니다.
    expect(tagKeyOfField('$.tags.')).toBeUndefined();
  });
});

describe('panelTagKeys', () => {
  it('설정이 없으면 빈 목록', () => {
    expect(panelTagKeys(undefined)).toEqual([]);
    expect(panelTagKeys({})).toEqual([]);
  });

  it('store 시리즈의 태그 필터 키를 모은다', () => {
    const keys = panelTagKeys({
      store_source: {
        agent_name: 'a',
        series: [{ key: 'LAI', tags: { room: '1', floor: '3' } }],
      },
    });
    expect(keys).toEqual(['floor', 'room']);
  });

  it('group by 축도 태그다', () => {
    const keys = panelTagKeys({
      store_source: { agent_name: 'a', series: [{ key: 'LAI', group_by: ['host'] }] },
    });
    expect(keys).toEqual(['host']);
  });

  it('태그 선택 모드의 tag_filters 키를 모은다', () => {
    const keys = panelTagKeys({
      store_source: { agent_name: 'a', selection_mode: 'tag', tag_filters: { room: '1' } },
    });
    expect(keys).toEqual(['room']);
  });

  it('여러 시리즈·여러 소스의 태그를 합치고 중복을 없앤다', () => {
    const keys = panelTagKeys({
      store_source: {
        agent_name: 'a',
        series: [{ key: 'k1', tags: { room: '1' } }, { key: 'k2', tags: { room: '2', floor: '3' } }],
      },
      tsdb_source: {
        agent_name: 'b',
        series: [{ key: 'cpu', field: 'value', tags: { host: 'h1' }, group_by: ['room'] }],
      },
    });
    // 정렬되어 있고 room 은 한 번만 나온다.
    expect(keys).toEqual(['floor', 'host', 'room']);
  });

  it('시스템 지표 소스는 고정 태그 축을 제시한다', () => {
    const keys = panelTagKeys({ sysmetrics_source: { agent_name: 'a', series: [] } });
    expect(keys).toEqual([...SYSMETRICS_TAG_KEYS]);
  });

  it('형태가 깨진 설정에도 던지지 않는다', () => {
    expect(panelTagKeys({ store_source: { series: 'not-an-array', tag_filters: 7 } })).toEqual([]);
    expect(panelTagKeys({ store_source: { series: [null, 3, { tags: null }] } })).toEqual([]);
  });
});
