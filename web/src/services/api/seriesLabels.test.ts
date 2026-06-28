// seriesLabels.ts 단위 테스트 — labels 파싱/서명/표시 (SPEC-STORE-004 M5).
//
// @spec SPEC-STORE-004

import { describe, expect, it } from 'vitest';

import {
  METRIC_LABEL_KEY,
  SERIES_ID_SEPARATOR,
  formatSeriesLabel,
  makeSeriesId,
  parseSeriesLabels,
  seriesDisplayName,
  seriesSignature,
} from './seriesLabels';

describe('parseSeriesLabels', () => {
  it('__metric__ 을 metric 으로 추출하고 나머지를 tags 로 분류', () => {
    const result = parseSeriesLabels({
      [METRIC_LABEL_KEY]: 'temperature',
      room: '1',
      type: 'sensor',
    });
    expect(result.metric).toBe('temperature');
    expect(result.tags).toEqual({ room: '1', type: 'sensor' });
  });

  it('undefined 입력은 빈 metric + 빈 tags', () => {
    expect(parseSeriesLabels(undefined)).toEqual({ metric: '', tags: {} });
  });

  it('null 입력은 빈 metric + 빈 tags', () => {
    expect(parseSeriesLabels(null)).toEqual({ metric: '', tags: {} });
  });

  it('metric 없이 tags 만 있는 경우', () => {
    expect(parseSeriesLabels({ room: '1' })).toEqual({
      metric: '',
      tags: { room: '1' },
    });
  });
});

describe('seriesSignature', () => {
  it('tag key 정렬로 결정적 서명을 만든다 (입력 순서 무관)', () => {
    const a = seriesSignature({
      [METRIC_LABEL_KEY]: 'temp',
      b: '2',
      a: '1',
    });
    const b = seriesSignature({
      a: '1',
      [METRIC_LABEL_KEY]: 'temp',
      b: '2',
    });
    expect(a).toBe(b);
    expect(a).toBe('temp|a=1,b=2');
  });

  it('라벨이 없으면 빈 서명 ("")', () => {
    expect(seriesSignature(undefined)).toBe('');
    expect(seriesSignature({})).toBe('');
  });

  it('metric 만 있는 시리즈와 tags 만 있는 시리즈는 다른 서명', () => {
    expect(seriesSignature({ [METRIC_LABEL_KEY]: 'temp' })).toBe('temp|');
    expect(seriesSignature({ room: '1' })).toBe('|room=1');
  });

  it('metric/tags 가 다른 두 시리즈는 다른 서명을 가진다', () => {
    const s1 = seriesSignature({ [METRIC_LABEL_KEY]: 'temp', room: '1' });
    const s2 = seriesSignature({ [METRIC_LABEL_KEY]: 'temp', room: '2' });
    const s3 = seriesSignature({ [METRIC_LABEL_KEY]: 'humid', room: '1' });
    expect(new Set([s1, s2, s3]).size).toBe(3);
  });
});

describe('makeSeriesId', () => {
  it('key + metric + tags 를 NUL 구분 결정적 ID 로 만든다', () => {
    const id = makeSeriesId('sensor', 'temp', { room: '1' });
    expect(id).toBe(`sensor${SERIES_ID_SEPARATOR}temp|room=1`);
  });

  it('tag 입력 순서가 달라도 같은 ID (서명 정규화)', () => {
    const a = makeSeriesId('s', 'temp', { b: '2', a: '1' });
    const b = makeSeriesId('s', 'temp', { a: '1', b: '2' });
    expect(a).toBe(b);
  });

  it('metric/tags 가 없으면 빈 서명 → `key<NUL>`', () => {
    expect(makeSeriesId('lonely', '', undefined)).toBe(
      `lonely${SERIES_ID_SEPARATOR}`,
    );
    expect(makeSeriesId('lonely', undefined, {})).toBe(
      `lonely${SERIES_ID_SEPARATOR}`,
    );
  });

  it('같은 key 의 서로 다른 시리즈는 다른 ID 를 가진다 (#2 시리즈별 분류)', () => {
    const a = makeSeriesId('sensor', 'temp', { room: '1' });
    const b = makeSeriesId('sensor', 'temp', { room: '2' });
    const c = makeSeriesId('sensor', 'humid', { room: '1' });
    expect(new Set([a, b, c]).size).toBe(3);
  });

  it('selectorSignature(store) 와 동일 규칙 — ID 의 서명부가 seriesSignature 와 일치', () => {
    const labels = { [METRIC_LABEL_KEY]: 'temp', room: '1' };
    const id = makeSeriesId('sensor', 'temp', { room: '1' });
    const sigPart = id.slice(`sensor${SERIES_ID_SEPARATOR}`.length);
    expect(sigPart).toBe(seriesSignature(labels));
  });
});

describe('formatSeriesLabel', () => {
  it('metric + tags 를 metric{k=v} 형태로 표기 (tag 정렬)', () => {
    expect(
      formatSeriesLabel({ metric: 'temp', tags: { b: '2', a: '1' } }),
    ).toBe('temp{a=1, b=2}');
  });

  it('metric 만 있으면 metric 그대로', () => {
    expect(formatSeriesLabel({ metric: 'temp', tags: {} })).toBe('temp');
  });

  it('tags 만 있으면 {k=v} 형태', () => {
    expect(formatSeriesLabel({ metric: '', tags: { room: '1' } })).toBe(
      '{room=1}',
    );
  });

  it('둘 다 없으면 빈 문자열', () => {
    expect(formatSeriesLabel({ metric: '', tags: {} })).toBe('');
  });
});

describe('seriesDisplayName', () => {
  it('withLabel=false 이면 store key 만 반환 (기존 호환)', () => {
    expect(
      seriesDisplayName('indoor:1:room_temp', { [METRIC_LABEL_KEY]: 'temp' }, false),
    ).toBe('indoor:1:room_temp');
  });

  it('withLabel=true + 라벨 있으면 key · 라벨 형태', () => {
    expect(
      seriesDisplayName(
        'sensor',
        { [METRIC_LABEL_KEY]: 'temp', room: '1' },
        true,
      ),
    ).toBe('sensor · temp{room=1}');
  });

  it('withLabel=true 이지만 라벨이 비어있으면 key 만', () => {
    expect(seriesDisplayName('sensor', undefined, true)).toBe('sensor');
    expect(seriesDisplayName('sensor', {}, true)).toBe('sensor');
  });
});
