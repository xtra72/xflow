// 패널 데이터소스 판정 순수 로직 단위 테스트.
//
// spec.md §2.3 [U3] 의 활성 조건 표가 **유일한 정본**이며, 이 파일은 그 표를
// 3종 × (블록 부재 / 시리즈 0 / 시리즈 N / 에이전트 미선택 / tag 모드 /
// 인식 불가 문자열)로 전수 커버한다.
//
// @spec SPEC-TSDB-002 §2.3 (U3) · §2.4 (U4) · §2.13 (S1) · §2.17 (UB2)
// 인수조건: AC-04 · AC-05 · AC-37 · AC-47

import { describe, expect, it } from 'vitest';

import {
  DEFAULT_STORE_SOURCE_WINDOW,
  defaultTsdbSource,
  type StoreSourceConfig,
  type TsdbSourceConfig,
} from './chartChannelTypes';
import {
  isStoreSourceActive,
  isTsdbSourceActive,
  PANEL_SOURCE_CAPABILITIES,
  panelSourceCapabilities,
  resolvePanelSourceBinding,
} from './panelDataSource';

// ---- 픽스처 (acceptance.md §공통 픽스처 F2) ----

const CFG_NONE: Record<string, unknown> = {};
const CFG_CHANNEL: Record<string, unknown> = { data_source: 'channel' };
const CFG_STORE: Record<string, unknown> = {
  data_source: 'store',
  store_source: { agent_name: 'st', series: [{ key: 'k1' }] },
};
const CFG_TSDB: Record<string, unknown> = {
  data_source: 'tsdb',
  tsdb_source: {
    backend: 'influxdb',
    agent_id: 'a-1',
    agent_name: 'ix',
    series: [{ key: 'cpu', field: 'usage', tags: { host: 'a' } }],
    time_window_ms: 3_600_000,
    interval_ms: 60_000,
    aggregation: 'average',
  },
};

/** `CFG_TSDB` 의 tsdb_source 를 부분 치환한 config 를 만든다. */
function tsdbCfg(patch: Partial<TsdbSourceConfig>): Record<string, unknown> {
  return {
    data_source: 'tsdb',
    tsdb_source: { ...(CFG_TSDB.tsdb_source as TsdbSourceConfig), ...patch },
  };
}

/** 조회 창까지 채운 완전한 `StoreSourceConfig` 를 만든다. */
function storeSource(patch: Partial<StoreSourceConfig> = {}): StoreSourceConfig {
  return {
    agent_name: 'st',
    series: [{ key: 'k1' }],
    ...DEFAULT_STORE_SOURCE_WINDOW,
    ...patch,
  };
}

/** `CFG_STORE` 의 store_source 를 부분 치환한 config 를 만든다. */
function storeCfg(patch: Partial<StoreSourceConfig>): Record<string, unknown> {
  return {
    data_source: 'store',
    store_source: { ...(CFG_STORE.store_source as StoreSourceConfig), ...patch },
  };
}

// ---- A. 소스 종류 판정 + 활성 조건 (AC-05) ----

describe('resolvePanelSourceBinding — 종류 판정', () => {
  it('data_source 부재 → channel/active', () => {
    const b = resolvePanelSourceBinding(CFG_NONE);
    expect(b.kind).toBe('channel');
    expect(b.active).toBe(true);
    // 부재는 "인식 불가"가 아니다 — 경고 표시 대상이 아니다(§2.4).
    expect(b.unknownKind).toBeUndefined();
  });

  it('data_source:channel → channel/active', () => {
    expect(resolvePanelSourceBinding(CFG_CHANNEL)).toEqual({
      kind: 'channel',
      active: true,
    });
  });

  it('store + store_source 부재 → store/inactive', () => {
    const b = resolvePanelSourceBinding({ data_source: 'store' });
    expect(b.kind).toBe('store');
    expect(b.active).toBe(false);
  });

  it('store + series 0 → store/inactive', () => {
    const b = resolvePanelSourceBinding(storeCfg({ series: [] }));
    expect(b).toEqual({ kind: 'store', active: false });
  });

  it('store + series N → store/active', () => {
    expect(resolvePanelSourceBinding(CFG_STORE)).toEqual({
      kind: 'store',
      active: true,
    });
  });

  it('store + selection_mode:tag + tag_filters N → store/active', () => {
    const b = resolvePanelSourceBinding(
      storeCfg({ series: [], selection_mode: 'tag', tag_filters: { room: '1' } }),
    );
    expect(b).toEqual({ kind: 'store', active: true });
  });

  it('store + selection_mode:tag + tag_filters 0 → store/inactive', () => {
    const b = resolvePanelSourceBinding(
      storeCfg({ series: [], selection_mode: 'tag', tag_filters: {} }),
    );
    expect(b.active).toBe(false);
  });

  it('tsdb + tsdb_source 부재 → tsdb/inactive', () => {
    const b = resolvePanelSourceBinding({ data_source: 'tsdb' });
    expect(b).toEqual({ kind: 'tsdb', active: false });
  });

  it('tsdb + agent_name 빈 문자열 → tsdb/inactive', () => {
    const b = resolvePanelSourceBinding(tsdbCfg({ agent_name: '' }));
    expect(b).toEqual({ kind: 'tsdb', active: false });
  });

  it('tsdb + agent_name + series 0 → tsdb/inactive', () => {
    const b = resolvePanelSourceBinding(tsdbCfg({ series: [] }));
    expect(b).toEqual({ kind: 'tsdb', active: false });
  });

  it('tsdb + agent_name + series N → tsdb/active', () => {
    expect(resolvePanelSourceBinding(CFG_TSDB)).toEqual({
      kind: 'tsdb',
      active: true,
    });
  });

  it('인식 불가 문자열 → channel/active + unknownKind', () => {
    const b = resolvePanelSourceBinding({ data_source: 'influxdb' });
    expect(b.kind).toBe('channel');
    expect(b.active).toBe(true);
    expect(b.unknownKind).toBe(true);
  });
});

// ---- B. 상태 정합 — 부재와 인식 불가의 구분 (AC-47) ----

describe('resolvePanelSourceBinding — 부재 vs 인식 불가 (§2.17)', () => {
  it('tsdb + tsdb_source 부재 → tsdb/inactive (channel 폴백 아님)', () => {
    // §2.17-1: 사용자가 명시한 선택을 뒤집지 않는다. 빈 선택 상태로 남는다.
    const b = resolvePanelSourceBinding({ data_source: 'tsdb' });
    expect(b.kind).toBe('tsdb');
    expect(b.kind).not.toBe('channel');
    expect(b.unknownKind).toBeUndefined();
  });

  it('인식 불가 문자열 → channel/active + unknownKind 플래그', () => {
    // §2.17-2: 사용자가 명시할 수 없는 상태이므로 폴백이 안전하다.
    expect(resolvePanelSourceBinding({ data_source: 'memtsdb' })).toEqual({
      kind: 'channel',
      active: true,
      unknownKind: true,
    });
  });

  it('data_source 가 문자열이 아니면 인식 불가로 처리한다', () => {
    expect(resolvePanelSourceBinding({ data_source: 42 }).unknownKind).toBe(true);
  });
});

// ---- C. 히트맵 파생 소스 주입 (plan.md 3.3 — 옵션 b) ----

describe('resolvePanelSourceBinding — storeSourceOverride', () => {
  it('override 가 주어지면 config.store_source 대신 그것으로 활성을 판정한다', () => {
    const b = resolvePanelSourceBinding(CFG_STORE, {
      storeSourceOverride: {
        ...(CFG_STORE.store_source as StoreSourceConfig),
        series: [],
      },
    });
    expect(b).toEqual({ kind: 'store', active: false });
  });

  it('override 가 tag 모드를 지우면 tag 활성도 사라진다 (히트맵 동작 보존)', () => {
    const tagged = storeCfg({
      series: [],
      selection_mode: 'tag',
      tag_filters: { room: '1' },
    });
    // 히트맵은 파생 소스에서 selection_mode 를 'keys' 로 강제하고 tag_filters 를 지운다.
    const override = {
      ...(tagged.store_source as StoreSourceConfig),
      selection_mode: 'keys' as const,
      tag_filters: undefined,
    };
    expect(resolvePanelSourceBinding(tagged).active).toBe(true);
    expect(resolvePanelSourceBinding(tagged, { storeSourceOverride: override }).active).toBe(
      false,
    );
  });

  it('override 는 store 종류가 아닐 때 무시된다', () => {
    const b = resolvePanelSourceBinding(CFG_TSDB, {
      storeSourceOverride: storeSource({ agent_name: 'x', series: [] }),
    });
    expect(b).toEqual({ kind: 'tsdb', active: true });
  });
});

// ---- D. 활성 조건 술어 단독 (M3 이관 대비) ----

describe('활성 조건 술어', () => {
  it('isStoreSourceActive — undefined 는 비활성', () => {
    expect(isStoreSourceActive(undefined)).toBe(false);
  });

  it('isStoreSourceActive — series N 또는 tag 필터 N 이면 활성', () => {
    expect(isStoreSourceActive(storeSource({ series: [{ key: 'k' }] }))).toBe(true);
    expect(
      isStoreSourceActive(
        storeSource({ series: [], selection_mode: 'tag', tag_filters: { r: '1' } }),
      ),
    ).toBe(true);
    expect(isStoreSourceActive(storeSource({ series: [] }))).toBe(false);
  });

  it('isTsdbSourceActive — 에이전트와 시리즈가 모두 있어야 활성', () => {
    expect(isTsdbSourceActive(undefined)).toBe(false);
    const base = CFG_TSDB.tsdb_source as TsdbSourceConfig;
    expect(isTsdbSourceActive(base)).toBe(true);
    expect(isTsdbSourceActive({ ...base, agent_name: '   ' })).toBe(false);
    expect(isTsdbSourceActive({ ...base, series: [] })).toBe(false);
  });
});

// ---- E. 능력 표 (AC-37) ----

describe('panelSourceCapabilities — 능력 표 (§2.13)', () => {
  it('능력 표가 3종 전부를 갖는다', () => {
    expect(Object.keys(PANEL_SOURCE_CAPABILITIES).sort()).toEqual([
      'channel',
      'store',
      'tsdb',
    ]);
    expect(panelSourceCapabilities('channel')).toBe(PANEL_SOURCE_CAPABILITIES.channel);
    expect(panelSourceCapabilities('store')).toBe(PANEL_SOURCE_CAPABILITIES.store);
    expect(panelSourceCapabilities('tsdb')).toBe(PANEL_SOURCE_CAPABILITIES.tsdb);
  });

  it('store 는 fill:null/zero/previous 를 지원하지 않는다', () => {
    expect(panelSourceCapabilities('store').fillStrategies).toBe(false);
    expect(panelSourceCapabilities('tsdb').fillStrategies).toBe(true);
  });

  it('tsdb 는 fill:avg 를 지원하지 않는다', () => {
    expect(panelSourceCapabilities('tsdb').fillAvg).toBe(false);
    expect(panelSourceCapabilities('store').fillAvg).toBe(false);
  });

  it('tsdb 는 에이전트 선택이 필수다', () => {
    const tsdb = panelSourceCapabilities('tsdb');
    expect(tsdb.agentSelection).toBe(true);
    expect(tsdb.agentRequired).toBe(true);
    const store = panelSourceCapabilities('store');
    expect(store.agentSelection).toBe(true);
    expect(store.agentRequired).toBe(false);
  });

  it('집계 first/last 는 store 가 클라이언트 폴백, tsdb 가 백엔드 직접이다', () => {
    expect(panelSourceCapabilities('store').aggregationFirstLast).toBe(true);
    expect(panelSourceCapabilities('store').aggregationFirstLastBackend).toBe(false);
    expect(panelSourceCapabilities('tsdb').aggregationFirstLast).toBe(true);
    expect(panelSourceCapabilities('tsdb').aggregationFirstLastBackend).toBe(true);
  });

  it('태그 동적 바인딩은 store 만, bucket 선택은 tsdb 만 지원한다 (OQ6 · OQ10)', () => {
    expect(panelSourceCapabilities('store').tagBinding).toBe(true);
    expect(panelSourceCapabilities('tsdb').tagBinding).toBe(false);
    expect(panelSourceCapabilities('store').bucketSelection).toBe(false);
    expect(panelSourceCapabilities('tsdb').bucketSelection).toBe(true);
  });

  it('channel 은 조회 파라미터 능력을 전부 갖지 않는다', () => {
    const ch = panelSourceCapabilities('channel');
    expect(Object.values(ch).every((v) => v === false)).toBe(true);
  });

  it('series_reduce 는 store/tsdb 양쪽에서 소스와 직교하게 지원된다 (§2.4)', () => {
    expect(panelSourceCapabilities('store').seriesReduce).toBe(true);
    expect(panelSourceCapabilities('tsdb').seriesReduce).toBe(true);
  });
});

// ---- F. defaultTsdbSource (§2.2 · §2.12) ----

describe('defaultTsdbSource', () => {
  it('조회 창 기본값을 DEFAULT_STORE_SOURCE_WINDOW 에서 전개한다', () => {
    // 값을 복제하면 한쪽만 바뀔 때 소스를 갈아탄 사용자가 조용히 다른 창을 본다(§2.2).
    expect(defaultTsdbSource()).toMatchObject(DEFAULT_STORE_SOURCE_WINDOW);
  });

  it('backend 는 influxdb, 에이전트 미선택, 시리즈 0 으로 시작한다', () => {
    const d = defaultTsdbSource();
    expect(d.backend).toBe('influxdb');
    expect(d.agent_name).toBe('');
    expect(d.series).toEqual([]);
  });

  it('기본값 블록은 비활성이다 — 에이전트를 고르기 전에는 조회하지 않는다', () => {
    expect(
      resolvePanelSourceBinding({ data_source: 'tsdb', tsdb_source: defaultTsdbSource() }),
    ).toEqual({ kind: 'tsdb', active: false });
  });

  it('호출마다 새 객체를 반환한다 (공유 참조 변이 방지)', () => {
    const a = defaultTsdbSource();
    const b = defaultTsdbSource();
    expect(a).not.toBe(b);
    expect(a.series).not.toBe(b.series);
  });
});

// ===== TSDB 미리보기 구성 (SPEC-TSDB-004) =====

import { buildPreviewSeries } from './previewSeries';

describe('buildPreviewSeries — TSDB 등록 시리즈로 구성', () => {
  const base = {
    dataSource: 'tsdb',
    storeSource: undefined,
    channels: [],
    channelName: '',
    globalSmooth: false,
    strokeDasharray: { solid: '', dashed: '6 4', dotted: '2 3' } as Record<
      'solid' | 'dashed' | 'dotted',
      string
    >,
    palette: ['#p0', '#p1', '#p2'],
    sampleName: 'sample',
    channelFallbackName: (i: number) => `ch${i}`,
  };

  it('정확 일치 항목은 한 줄이며 항목 색을 쓴다', () => {
    const r = buildPreviewSeries({
      ...base,
      tsdbSource: {
        backend: 'influxdb',
        agent_name: 'ix',
        series: [{ key: 'cpu', field: 'usage', alias: 'CPU', color: '#abc' }],
        time_window_ms: 60_000,
        interval_ms: 10_000,
        aggregation: 'average',
      },
    });
    expect(r).toHaveLength(1);
    expect(r[0]!.key).toBe('CPU');
    expect(r[0]!.color).toBe('#abc');
  });

  it('group by 항목은 고른 조합 수만큼 펼쳐진다', () => {
    // config 상 1항목이지만 런타임에는 여러 줄이 된다. 펼치지 않으면 "설정에서는
    // 한 줄인데 대시보드에는 여러 줄" 이 되어 미리보기가 제 일을 못 한다.
    const r = buildPreviewSeries({
      ...base,
      tsdbSource: {
        backend: 'influxdb',
        agent_name: 'ix',
        series: [
          {
            key: 'cpu',
            field: 'usage',
            color: '#abc',
            group_by: ['host'],
            group_filter: [{ host: 'A' }, { host: 'B' }],
          },
        ],
        time_window_ms: 60_000,
        interval_ms: 10_000,
        aggregation: 'average',
      },
    });
    expect(r).toHaveLength(2);
    // 이름이 서로 달라야 한다 — 같으면 범례에서 구분되지 않는다.
    expect(new Set(r.map((x) => x.key)).size).toBe(2);
    // 자동 팔레트를 쓴다(OQ1) — 실제 렌더와 같아야 미리보기가 신뢰된다.
    expect(r.map((x) => x.color)).toEqual(['#p0', '#p1']);
  });

  it('등록이 없으면 sample 한 줄로 폴백한다 (빈 차트는 고장처럼 보인다)', () => {
    const r = buildPreviewSeries({
      ...base,
      tsdbSource: {
        backend: 'influxdb',
        agent_name: 'ix',
        series: [],
        time_window_ms: 60_000,
        interval_ms: 10_000,
        aggregation: 'average',
      },
    });
    expect(r).toHaveLength(1);
    expect(r[0]!.key).toBe('sample');
  });
});
