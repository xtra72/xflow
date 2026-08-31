// 활성 소스의 X축 고정 창 판정 테스트.
//
// 라인 차트는 X축을 "지금 − 창 길이 ~ 지금" 으로 고정한다. 데이터가 창보다 짧아도 축을
// 유지해야 스케일이 흔들리지 않기 때문이다.
//
// 종전에는 그 창을 `config.store_source.time_window_ms` 에서 **하드코딩**해 읽었다.
// store 가 아닌 소스에서는 언제나 `undefined` 라 축이 데이터 범위(dataMin~dataMax)로
// 떨어졌고, TSDB 는 첫 응답에 창 전체가 들어와 티가 나지 않았지만 라이브로 쌓는
// sysmetrics 는 점이 0~1개인 동안 축이 한 점으로 접혀 **선이 아예 보이지 않았다**.

import { describe, expect, it } from 'vitest';

import { panelSourceWindowMs } from './panelDataSource';

const WINDOW = 5 * 60_000;

describe('panelSourceWindowMs — 활성 소스의 창을 읽는다', () => {
  it('store 소스는 store_source 에서 읽는다', () => {
    expect(
      panelSourceWindowMs({
        data_source: 'store',
        store_source: { agent_name: 'a', series: [{ key: 'k' }], time_window_ms: WINDOW },
      }),
    ).toBe(WINDOW);
  });

  it('tsdb 소스는 tsdb_source 에서 읽는다 (종전에는 store_source 만 봐서 undefined 였다)', () => {
    expect(
      panelSourceWindowMs({
        data_source: 'tsdb',
        tsdb_source: {
          backend: 'influxdb',
          agent_name: 'ix',
          series: [{ key: 'm', field: 'f' }],
          time_window_ms: WINDOW,
        },
      }),
    ).toBe(WINDOW);
  });

  it('sysmetrics 소스는 sysmetrics_source 에서 읽는다', () => {
    expect(
      panelSourceWindowMs({
        data_source: 'sysmetrics',
        sysmetrics_source: {
          agent_id: 'a1',
          agent_name: 'host',
          series: [{ key: 'cpu.usage_percent' }],
          time_window_ms: WINDOW,
        },
      }),
    ).toBe(WINDOW);
  });

  it('다른 소스의 블록이 남아 있어도 활성 소스의 것만 읽는다', () => {
    // 소스를 오가면 블록이 함께 남는다. 종류가 정본이므로 남은 블록에 끌려가면 안 된다.
    expect(
      panelSourceWindowMs({
        data_source: 'sysmetrics',
        store_source: { agent_name: 'a', series: [{ key: 'k' }], time_window_ms: 999 },
        sysmetrics_source: {
          agent_id: 'a1',
          agent_name: 'host',
          series: [{ key: 'cpu.usage_percent' }],
          time_window_ms: WINDOW,
        },
      }),
    ).toBe(WINDOW);
  });

  it('채널 소스에는 창이 없다', () => {
    expect(panelSourceWindowMs({ data_source: 'channel' })).toBeUndefined();
    expect(panelSourceWindowMs({})).toBeUndefined();
  });

  it('창이 없거나 0 이하면 undefined 다 (축을 데이터 범위로 둔다)', () => {
    const base = {
      data_source: 'sysmetrics',
      sysmetrics_source: {
        agent_id: 'a1',
        agent_name: 'host',
        series: [{ key: 'cpu.usage_percent' }],
      } as Record<string, unknown>,
    };
    expect(panelSourceWindowMs(base)).toBeUndefined();
    expect(
      panelSourceWindowMs({
        ...base,
        sysmetrics_source: { ...base.sysmetrics_source, time_window_ms: 0 },
      }),
    ).toBeUndefined();
    expect(
      panelSourceWindowMs({
        ...base,
        sysmetrics_source: { ...base.sysmetrics_source, time_window_ms: -1 },
      }),
    ).toBeUndefined();
  });

  it('소스 블록이 아예 없으면 undefined 다', () => {
    expect(panelSourceWindowMs({ data_source: 'sysmetrics' })).toBeUndefined();
    expect(panelSourceWindowMs({ data_source: 'tsdb' })).toBeUndefined();
  });
});
