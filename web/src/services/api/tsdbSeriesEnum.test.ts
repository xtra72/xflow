// @spec SPEC-TSDB-004 §2.7.2 — 그룹 후보 도출과 페이지 슬라이스.

import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  deriveGroupCombos,
  groupComboSignature,
  groupPageCount,
  sliceGroupPage,
  enumerateTsdbSeries,
  type TsdbEnumeratedSeries,
} from './tsdbSeriesEnum';
import * as client from './client';

const S = (tags: Record<string, string>): TsdbEnumeratedSeries => ({ tags, fields: ['usage'] });

describe('deriveGroupCombos', () => {
  it('그룹 키로 투영하고 중복을 제거한다', () => {
    // 같은 host 가 rack 만 다른 두 시리즈 -> host 축에서는 후보 1개.
    const combos = deriveGroupCombos(
      [S({ host: 'a', rack: 'r1' }), S({ host: 'a', rack: 'r2' }), S({ host: 'b', rack: 'r1' })],
      ['host'],
    );
    expect(combos).toEqual([{ host: 'a' }, { host: 'b' }]);
  });

  it('다중 키는 조합 단위로 후보를 만든다', () => {
    const combos = deriveGroupCombos(
      [S({ host: 'a', rack: 'r1' }), S({ host: 'a', rack: 'r2' })],
      ['host', 'rack'],
    );
    expect(combos).toEqual([
      { host: 'a', rack: 'r1' },
      { host: 'a', rack: 'r2' },
    ]);
  });

  it('서명 사전순으로 안정 정렬한다', () => {
    // 입력 순서를 뒤집어도 결과가 같아야 페이지 경계가 흔들리지 않는다.
    const asc = deriveGroupCombos([S({ host: 'a' }), S({ host: 'b' }), S({ host: 'c' })], ['host']);
    const desc = deriveGroupCombos([S({ host: 'c' }), S({ host: 'b' }), S({ host: 'a' })], ['host']);
    expect(desc).toEqual(asc);
    expect(asc.map((c) => c.host)).toEqual(['a', 'b', 'c']);
  });

  it('그룹 키가 없는 시리즈는 빈 값 후보가 된다 (조용히 버리지 않는다)', () => {
    const combos = deriveGroupCombos([S({ host: 'a', rack: 'r1' }), S({ host: 'd' })], ['rack']);
    expect(combos).toEqual([{ rack: '' }, { rack: 'r1' }]);
  });

  it('그룹 키가 비면 후보가 없다', () => {
    expect(deriveGroupCombos([S({ host: 'a' })], [])).toEqual([]);
  });

  it('키 입력 순서가 결과에 영향을 주지 않는다', () => {
    const a = deriveGroupCombos([S({ host: 'a', rack: 'r1' })], ['rack', 'host']);
    const b = deriveGroupCombos([S({ host: 'a', rack: 'r1' })], ['host', 'rack']);
    expect(a).toEqual(b);
  });
});

describe('groupComboSignature', () => {
  it('값에 구분자가 있어도 충돌하지 않는다', () => {
    // NUL 은 태그 값에 등장할 수 없으므로 'a|b' 와 'a'+'|b' 가 구분된다.
    const x = groupComboSignature({ k1: 'a|b', k2: '' }, ['k1', 'k2']);
    const y = groupComboSignature({ k1: 'a', k2: '|b' }, ['k1', 'k2']);
    expect(x).not.toBe(y);
  });
});

describe('sliceGroupPage / groupPageCount', () => {
  const combos = ['a', 'b', 'c', 'd', 'e'].map((h) => ({ host: h }));

  it('페이지를 자른다', () => {
    expect(sliceGroupPage(combos, 0, 2)).toEqual([{ host: 'a' }, { host: 'b' }]);
    expect(sliceGroupPage(combos, 1, 2)).toEqual([{ host: 'c' }, { host: 'd' }]);
    expect(sliceGroupPage(combos, 2, 2)).toEqual([{ host: 'e' }]);
  });

  it('범위를 넘는 페이지는 빈 배열이다', () => {
    expect(sliceGroupPage(combos, 9, 2)).toEqual([]);
  });

  it('pageSize 가 0 이하이면 전량을 돌려준다 (페이지네이션 비활성)', () => {
    expect(sliceGroupPage(combos, 3, 0)).toEqual(combos);
    expect(groupPageCount(5, 0)).toBe(1);
  });

  it('페이지 수를 올림으로 구한다', () => {
    expect(groupPageCount(5, 2)).toBe(3);
    expect(groupPageCount(4, 2)).toBe(2);
    expect(groupPageCount(0, 2)).toBe(1);
  });
});

describe('enumerateTsdbSeries', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('질의 파라미터를 조립하고 태그를 k=v 로 인코딩한다', async () => {
    const getSpy = vi.spyOn(client, 'get').mockResolvedValue({
      series: [{ tags: { host: 'a' }, fields: ['usage'] }],
      field_exact: true,
      count: 1,
      truncated: false,
      window: { start_ms: 1, end_ms: 2 },
    });

    const r = await enumerateTsdbSeries('agent 1', {
      measurement: 'cpu',
      bucket: 'metrics',
      tags: { region: 'kr', az: 'a' },
      startMs: 100,
      endMs: 200,
      limit: 50,
    });

    const url = String(getSpy.mock.calls[0]![0]);
    expect(url).toContain('/influxdb/agent%201/series?');
    expect(url).toContain('measurement=cpu');
    expect(url).toContain('bucket=metrics');
    // 태그는 키 오름차순으로 고정된다.
    expect(decodeURIComponent(url)).toContain('tags=az=a,region=kr');
    expect(url).toContain('start_ms=100');
    expect(url).toContain('limit=50');
    expect(r.count).toBe(1);
  });

  it('measurement 가 없으면 요청하지 않고 거부한다', async () => {
    const getSpy = vi.spyOn(client, 'get');
    await expect(enumerateTsdbSeries('a', { measurement: '' })).rejects.toThrow();
    expect(getSpy).not.toHaveBeenCalled();
  });

  it('응답 누락 필드를 안전한 기본값으로 채운다', async () => {
    vi.spyOn(client, 'get').mockResolvedValue(undefined);
    const r = await enumerateTsdbSeries('a', { measurement: 'cpu' });
    expect(r.series).toEqual([]);
    expect(r.truncated).toBe(false);
    expect(r.count).toBe(0);
  });
});
