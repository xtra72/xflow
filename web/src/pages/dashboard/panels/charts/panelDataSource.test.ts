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
  isPanelSeriesActive,
  isStoreSourceActive,
  isTsdbSourceActive,
  PANEL_SOURCE_CAPABILITIES,
  panelSourceCapabilities,
  resolvePanelSourceBinding,
  readPanelSourceKinds,
  resolvePanelSourceBindings,
  SOURCE_LABELS,
  MAX_SOURCES_PER_KIND,
  readPanelSources,
  sourceEntryPatch,
  resolvePanelSourceEntries,
  sourceEntryLabel,
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
  // 채널이 패널 소스에서 빠진 뒤로 폴백은 store 다. 활성 판정도 함께 옮겼다 — 채널
  // 폴백은 언제나 활성이었지만(채널 훅이 빈 상태를 스스로 처리했다) store 는 시리즈를
  // 골라야 활성이다. 참으로 두면 고른 것이 없는 패널이 "조회 중" 으로 보인다.
  it('data_source 부재 → store/inactive', () => {
    const b = resolvePanelSourceBinding(CFG_NONE);
    expect(b.kind).toBe('store');
    expect(b.active).toBe(false);
    // 부재는 "인식 불가"가 아니다 — 경고 표시 대상이 아니다(§2.4).
    expect(b.unknownKind).toBeUndefined();
  });

  it("없어진 'channel' 값도 store 로 접되 경고를 붙이지 않는다 — 오타가 아니라 폐지다", () => {
    expect(resolvePanelSourceBinding(CFG_CHANNEL)).toEqual({
      kind: 'store',
      active: false,
    });
  });

  it('부재 폴백도 store_source 가 갖춰지면 활성이다', () => {
    const b = resolvePanelSourceBinding({
      store_source: { agent_name: 'a', series: [{ key: 'k', field: 'value' }] },
    });
    expect(b).toEqual({ kind: 'store', active: true });
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

  it('인식 불가 문자열 → store/inactive + unknownKind', () => {
    const b = resolvePanelSourceBinding({ data_source: 'influxdb' });
    expect(b.kind).toBe('store');
    expect(b.active).toBe(false);
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

  it('인식 불가 문자열 → store/inactive + unknownKind 플래그', () => {
    // §2.17-2: 사용자가 명시할 수 없는 상태이므로 폴백이 안전하다.
    expect(resolvePanelSourceBinding({ data_source: 'memtsdb' })).toEqual({
      kind: 'store',
      active: false,
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
  it('능력 표가 소스 종류 전부를 갖는다', () => {
    expect(Object.keys(PANEL_SOURCE_CAPABILITIES).sort()).toEqual([
      'store',
      'sysmetrics',
      'tsdb',
    ]);
    expect(panelSourceCapabilities('store')).toBe(PANEL_SOURCE_CAPABILITIES.store);
    expect(panelSourceCapabilities('tsdb')).toBe(PANEL_SOURCE_CAPABILITIES.tsdb);
  });

  it('store 도 fill:null/zero/previous 를 지원한다 (서버가 빈 버킷을 채운다)', () => {
    expect(panelSourceCapabilities('store').fillStrategies).toBe(true);
    expect(panelSourceCapabilities('tsdb').fillStrategies).toBe(true);
  });

  it('시스템 지표는 여전히 fill 을 지원하지 않는다', () => {
    expect(panelSourceCapabilities('sysmetrics').fillStrategies).toBe(false);
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

describe('sysmetrics 소스 — 판정 계약', () => {
  const active = {
    data_source: 'sysmetrics',
    sysmetrics_source: {
      agent_id: 'a1',
      agent_name: 'host',
      series: [{ key: 'cpu.usage_percent' }],
    },
  };

  it('agent_id 와 값이 둘 다 있으면 활성이다', () => {
    expect(resolvePanelSourceBinding(active)).toEqual({ kind: 'sysmetrics', active: true });
  });

  it('agent_id 가 없으면 비활성이다 (agent_name 만으로는 활성이 아니다)', () => {
    expect(
      resolvePanelSourceBinding({
        data_source: 'sysmetrics',
        sysmetrics_source: { agent_name: 'host', series: [{ key: 'cpu.usage_percent' }] },
      }),
    ).toEqual({ kind: 'sysmetrics', active: false });
  });

  it('값이 하나도 없으면 비활성이다', () => {
    expect(
      resolvePanelSourceBinding({
        data_source: 'sysmetrics',
        sysmetrics_source: { agent_id: 'a1', agent_name: 'host', series: [] },
      }),
    ).toEqual({ kind: 'sysmetrics', active: false });
  });

  it('대상 없는 시리즈(종합)도 활성이다', () => {
    expect(
      resolvePanelSourceBinding({
        ...active,
        sysmetrics_source: {
          ...active.sysmetrics_source,
          series: [{ key: 'network.bytes_recv' }],
        },
      }).active,
    ).toBe(true);
  });

  it('소스 블록이 없어도 channel 로 조용히 폴백하지 않는다', () => {
    expect(resolvePanelSourceBinding({ data_source: 'sysmetrics' })).toEqual({
      kind: 'sysmetrics',
      active: false,
    });
  });

  it('능력 표는 집계·fill 없이 에이전트 필수로 선언된다', () => {
    const caps = panelSourceCapabilities('sysmetrics');
    // 폴링 주기가 곧 표본 간격이라 버킷 집계 축이 없다.
    expect(caps.aggregationBasic).toBe(false);
    expect(caps.aggregationFirstLast).toBe(false);
    // 표본이 없는 구간에는 점 자체가 없으므로 채울 대상이 없다.
    expect(caps.fillStrategies).toBe(false);
    // 어느 호스트의 지표인지 알아야 한다.
    expect(caps.agentSelection).toBe(true);
    expect(caps.agentRequired).toBe(true);
    // 구간 대표값은 소스와 직교한다 — 통계·게이지가 쓴다.
    expect(caps.seriesReduce).toBe(true);
  });
});

describe('buildPreviewSeries — sysmetrics 등록 값으로 구성', () => {
  const base = {
    dataSource: 'sysmetrics',
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

  it('고른 시리즈가 그대로 미리보기 줄이 된다 (설정과 대시보드가 갈리지 않는다)', () => {
    const r = buildPreviewSeries({
      ...base,
      sysmetricsSource: {
        agent_id: 'a1',
        agent_name: 'host',
        time_window_ms: 60 * 60_000,
        interval_ms: 60_000,
        aggregation: 'average' as const,
        series: [
          { key: 'network.bytes_recv', target: 'en0' },
          { key: 'network.bytes_recv', target: 'en1' },
        ],
      },
    });
    expect(r.map((x) => x.key)).toEqual([
      'bytes_recv · {category=network, interface=en0}',
      'bytes_recv · {category=network, interface=en1}',
    ]);
    expect(r.map((x) => x.color)).toEqual(['#p0', '#p1']);
  });

  it('항목 색·선 모양을 그대로 반영한다', () => {
    const r = buildPreviewSeries({
      ...base,
      sysmetricsSource: {
        agent_id: 'a1',
        agent_name: 'host',
        time_window_ms: 60 * 60_000,
        interval_ms: 60_000,
        aggregation: 'average' as const,
        series: [
          { key: 'cpu.usage_percent', color: '#abc', stroke_style: 'dashed', stroke_width: 3 },
        ],
      },
    });
    expect(r).toEqual([
      {
        key: 'usage_percent · {category=cpu}',
        color: '#abc',
        smooth: false,
        strokeWidth: 3,
        strokeDasharray: '6 4',
        graphStyle: 'line',
      },
    ]);
  });

  it('고른 값이 없으면 sample 한 줄로 폴백한다 (빈 차트는 고장처럼 보인다)', () => {
    const r = buildPreviewSeries({
      ...base,
      sysmetricsSource: {
        agent_id: 'a1',
        agent_name: 'host',
        series: [],
        time_window_ms: 60 * 60_000,
        interval_ms: 60_000,
        aggregation: 'average' as const,
      },
    });
    expect(r).toHaveLength(1);
    expect(r[0]!.key).toBe('sample');
  });
});

// ---------------------------------------------------------------------------
// 다중 소스 — 한 패널이 여러 종류를 동시에 쓴다.
//
// 저장된 패널은 전부 단일 축(`data_source`)이므로, 목록이 없을 때 결과가 종전 판정과
// 같아야 그림이 변하지 않는다. 그것이 이 절의 첫 검사다.
// ---------------------------------------------------------------------------

describe('readPanelSourceKinds', () => {
  it('필드가 없으면 빈 목록이다 — 호출부가 단일 축으로 되돌아간다', () => {
    expect(readPanelSourceKinds({})).toEqual([]);
    expect(readPanelSourceKinds({ data_sources: 'store' })).toEqual([]);
  });

  it('적은 순서를 지킨다 — 시리즈 순서와 색 배정이 그 순서를 따른다', () => {
    expect(readPanelSourceKinds({ data_sources: ['tsdb', 'store'] })).toEqual(['tsdb', 'store']);
  });

  it('인식 불가 값은 버리고 중복은 첫 자리만 남긴다', () => {
    expect(
      readPanelSourceKinds({ data_sources: ['store', 'wat', 'store', 'tsdb', 'channel'] }),
    ).toEqual(['store', 'tsdb']);
  });

  it('쓸 값이 하나도 없으면 빈 목록이다', () => {
    expect(readPanelSourceKinds({ data_sources: ['channel', 'wat'] })).toEqual([]);
  });
});

describe('resolvePanelSourceBindings', () => {
  const activeStore = { agent_name: 'a', series: [{ key: 'k', field: 'value' }] };

  it('목록이 없으면 단일 판정 한 개다 — 저장된 패널의 동작이 그대로다', () => {
    const one = resolvePanelSourceBindings({ data_source: 'store', store_source: activeStore });
    expect(one).toEqual([resolvePanelSourceBinding({ data_source: 'store', store_source: activeStore })]);
  });

  it('목록이 있으면 종류마다 하나씩 판정한다', () => {
    const list = resolvePanelSourceBindings({
      data_sources: ['store', 'tsdb'],
      store_source: activeStore,
    });
    expect(list.map((b) => b.kind)).toEqual(['store', 'tsdb']);
    // 활성 판정은 종류별로 따로 선다 — store 만 갖춰졌으므로 tsdb 는 비활성이다.
    expect(list.map((b) => b.active)).toEqual([true, false]);
  });

  it('빈 목록은 단일 축으로 되돌아간다 — "골랐는데 아무것도 없다" 를 막는다', () => {
    const list = resolvePanelSourceBindings({
      data_sources: [],
      data_source: 'store',
      store_source: activeStore,
    });
    expect(list.map((b) => b.kind)).toEqual(['store']);
  });
});

describe('SOURCE_LABELS', () => {
  it('종류마다 사람이 읽을 이름이 있다 — 시리즈 이름이 겹칠 때 꼬리표가 된다', () => {
    expect(Object.keys(SOURCE_LABELS).sort()).toEqual(['store', 'sysmetrics', 'tsdb']);
    for (const v of Object.values(SOURCE_LABELS)) expect(v.length).toBeGreaterThan(0);
  });
});

// ---------------------------------------------------------------------------
// 소스 인스턴스 — 같은 종류를 둘 이상.
//
// 종류 목록으로는 Store 에이전트 A 와 B 를 한 차트에 겹쳐 볼 수 없었다. 저장 단위를
// 인스턴스로 옮긴 뒤에도 **저장된 세 세대의 형상이 모두 읽혀야** 한다.
// ---------------------------------------------------------------------------

describe('readPanelSources', () => {
  const storeA = { agent_name: 'A', series: [{ key: 'k', field: 'value' }] };
  const storeB = { agent_name: 'B', series: [{ key: 'k', field: 'value' }] };

  it('같은 종류를 둘 이상 담는다 — 종류 목록으로는 못 하던 것', () => {
    const out = readPanelSources({
      sources: [
        { kind: 'store', store_source: storeA },
        { kind: 'store', store_source: storeB },
      ],
    });
    expect(out).toHaveLength(2);
    expect(out.map((e) => e.store_source?.agent_name)).toEqual(['A', 'B']);
  });

  it('단일 축(구 형상)은 인스턴스 하나로 읽힌다 — 저장된 패널의 경로', () => {
    const out = readPanelSources({ data_source: 'store', store_source: storeA });
    expect(out).toEqual([
      expect.objectContaining({ kind: 'store', store_source: storeA }),
    ]);
  });

  it('종류 목록(중간 형상)도 인스턴스로 옮겨 읽는다', () => {
    const out = readPanelSources({ data_sources: ['store', 'tsdb'], store_source: storeA });
    expect(out.map((e) => e.kind)).toEqual(['store', 'tsdb']);
  });

  it('인식 불가 종류는 버린다', () => {
    const out = readPanelSources({
      sources: [{ kind: 'wat' }, { kind: 'store', store_source: storeA }],
    });
    expect(out.map((e) => e.kind)).toEqual(['store']);
  });

  it('종류당 상한을 넘으면 잘라 낸다 — 훅 슬롯이 고정이라 조용히 넘길 수 없다', () => {
    const many = Array.from({ length: MAX_SOURCES_PER_KIND + 3 }, () => ({
      kind: 'store' as const,
      store_source: storeA,
    }));
    expect(readPanelSources({ sources: many })).toHaveLength(MAX_SOURCES_PER_KIND);
  });

  it('빈 목록은 구 형상으로 되돌아간다 — "골랐는데 아무것도 없다" 를 막는다', () => {
    const out = readPanelSources({ sources: [], data_source: 'store', store_source: storeA });
    expect(out.map((e) => e.kind)).toEqual(['store']);
  });
});

describe('resolvePanelSourceEntries', () => {
  const active = { agent_name: 'A', series: [{ key: 'k', field: 'value' }] };

  it('인스턴스마다 따로 판정한다 — 하나가 비어도 나머지는 산다', () => {
    const out = resolvePanelSourceEntries({
      sources: [
        { kind: 'store', store_source: active },
        { kind: 'store', store_source: { agent_name: '', series: [] } },
      ],
    });
    expect(out.map((r) => r.binding.active)).toEqual([true, false]);
  });

  it('인식 불가 단일 축은 패널 자신의 판정을 지킨다 — 옮겨 담다 사연을 잃으면 안 된다', () => {
    const out = resolvePanelSourceEntries({ data_source: 'wat', store_source: active });
    expect(out).toHaveLength(1);
    // 갖춰진 store 블록이 있어도 조회하지 않는다(고른 적 없는 값이므로).
    expect(out[0]!.binding.active).toBe(false);
    expect(out[0]!.binding.unknownKind).toBe(true);
  });
});

describe('resolvePanelSourceBindings — 종류 축 요약', () => {
  const active = { agent_name: 'A', series: [{ key: 'k', field: 'value' }] };

  it('같은 종류가 여럿이면 하나로 접는다 — 토글은 종류마다 하나다', () => {
    const out = resolvePanelSourceBindings({
      sources: [
        { kind: 'store', store_source: active },
        { kind: 'store', store_source: active },
      ],
    });
    expect(out).toHaveLength(1);
  });

  it('하나라도 활성이면 그 종류는 활성이다', () => {
    const out = resolvePanelSourceBindings({
      sources: [
        { kind: 'store', store_source: { agent_name: '', series: [] } },
        { kind: 'store', store_source: active },
      ],
    });
    expect(out[0]!.active).toBe(true);
  });
});

describe('sourceEntryLabel', () => {
  it('종류가 하나뿐이면 종류 이름만 쓴다 — 범례가 공연히 길어지지 않는다', () => {
    const entries = [{ kind: 'store' as const }, { kind: 'tsdb' as const }];
    expect(sourceEntryLabel(entries, 0)).toBe('Store');
    expect(sourceEntryLabel(entries, 1)).toBe('TSDB');
  });

  it('같은 종류가 여럿이면 순번을 붙인다 — 꼬리표까지 같으면 구분이 안 된다', () => {
    const entries = [
      { kind: 'store' as const },
      { kind: 'tsdb' as const },
      { kind: 'store' as const },
    ];
    expect(sourceEntryLabel(entries, 0)).toBe('Store 1');
    expect(sourceEntryLabel(entries, 2)).toBe('Store 2');
    // 하나뿐인 종류는 순번 없이 그대로다.
    expect(sourceEntryLabel(entries, 1)).toBe('TSDB');
  });
});

describe('sourceEntryPatch — 편집기와 표가 같은 규칙을 쓴다', () => {
  const a = { agent_name: 'A', series: [{ key: 'k', field: 'value' }] };
  const b = { agent_name: 'B', series: [] };

  it('지정한 인스턴스의 블록만 갈아 끼운다', () => {
    const config = {
      sources: [
        { kind: 'store', store_source: a },
        { kind: 'store', store_source: b },
      ],
    };
    const patch = sourceEntryPatch(config, 1, { store_source: { ...b, agent_name: 'C' } });
    const list = patch.sources as Array<{ store_source: { agent_name: string } }>;
    expect(list[0]!.store_source.agent_name).toBe('A');
    expect(list[1]!.store_source.agent_name).toBe('C');
  });

  it('구 형상(인스턴스 하나)에서는 최상위 블록을 그대로 고친다 — 그 둘이 같은 자리다', () => {
    const patch = sourceEntryPatch({ data_source: 'store', store_source: a }, 0, {
      store_source: b,
    });
    expect(patch).toEqual({ store_source: b });
    expect(patch.sources).toBeUndefined();
  });

  it('다른 인스턴스의 설정을 지우지 않는다', () => {
    const config = {
      sources: [
        { kind: 'store', store_source: a },
        { kind: 'tsdb', tsdb_source: { backend: 'influxdb' } },
      ],
    };
    const patch = sourceEntryPatch(config, 0, { store_source: b });
    const list = patch.sources as Array<Record<string, unknown>>;
    expect(list[1]!.tsdb_source).toEqual({ backend: 'influxdb' });
  });
});

// ---------------------------------------------------------------------------
// 활성 판정은 소스 목록 전체를 본다
//
// 시리즈를 고르면 그 선택은 `sources[i].store_source.series` 에 쓰인다. 활성 판정이
// 최상위 `store_source` 만 보면 고른 것이 목록 쪽에만 있는 패널은 영원히 비활성이고,
// 설정 화면 미리보기가 합성 샘플에 머문다("시리즈를 골랐는데 미리보기가 그대로다").
// ---------------------------------------------------------------------------
describe('isPanelSeriesActive — 목록 전체 기준 활성 판정', () => {
  const seriesRef = { key: 'temp', field: 'value' };

  it('목록의 인스턴스만 시리즈를 가져도 활성이다 — 최상위는 비어 있다', () => {
    const config = {
      data_source: 'store',
      store_source: { agent_name: 'a', series: [] },
      sources: [{ kind: 'store', store_source: { agent_name: 'a', series: [seriesRef] } }],
    };
    expect(isPanelSeriesActive(config)).toBe(true);
    // 단일 해석기는 최상위만 보므로 거짓이다 — 이 차이가 결함의 원인이었다.
    expect(resolvePanelSourceBinding(config).active).toBe(false);
  });

  it('둘째 인스턴스만 활성이어도 활성이다', () => {
    const config = {
      data_source: 'store',
      sources: [
        { kind: 'store', store_source: { agent_name: 'a', series: [] } },
        { kind: 'store', store_source: { agent_name: 'b', series: [seriesRef] } },
      ],
    };
    expect(isPanelSeriesActive(config)).toBe(true);
  });

  it('아무 인스턴스도 시리즈를 갖지 않으면 비활성이다', () => {
    const config = {
      data_source: 'store',
      sources: [{ kind: 'store', store_source: { agent_name: 'a', series: [] } }],
    };
    expect(isPanelSeriesActive(config)).toBe(false);
  });

  it('목록이 없는 구 패널은 최상위 블록으로 판정한다 — 하위 호환', () => {
    expect(
      isPanelSeriesActive({
        data_source: 'store',
        store_source: { agent_name: 'a', series: [seriesRef] },
      }),
    ).toBe(true);
    expect(
      isPanelSeriesActive({ data_source: 'store', store_source: { agent_name: 'a', series: [] } }),
    ).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// 인스턴스 패치는 **소스 블록만** 인스턴스로 보낸다
//
// 시리즈 표는 한 번의 패치에 소스 블록과 패널 단위 값(히트맵 센서 좌표)을 함께 담는다.
// 전부 인스턴스 안으로 밀어 넣으면 좌표가 `sources[i]` 밑에 묻혀 히트맵이 읽지 못한다.
// ---------------------------------------------------------------------------
describe('sourceEntryPatch — 소스 외 키는 최상위에 남는다', () => {
  const config = {
    data_source: 'store',
    sources: [{ kind: 'store', store_source: { agent_name: 'a', series: [] } }],
  };

  it('sensor_positions 는 최상위로 나간다', () => {
    const patch = sourceEntryPatch(config, 0, {
      store_source: { agent_name: 'a', series: [{ key: 'k' }] },
      sensor_positions: { 'k||': { x: 0.5, y: 0.5 } },
    });
    expect(patch.sensor_positions).toEqual({ 'k||': { x: 0.5, y: 0.5 } });
    const list = patch.sources as Array<Record<string, unknown>>;
    expect(list[0]!.sensor_positions).toBeUndefined();
    expect(list[0]!.store_source).toEqual({ agent_name: 'a', series: [{ key: 'k' }] });
  });

  it('소스 블록만 담긴 패치는 종전과 같다 — 최상위에 새 키가 생기지 않는다', () => {
    const patch = sourceEntryPatch(config, 0, { store_source: { agent_name: 'b' } });
    expect(Object.keys(patch)).toEqual(['sources']);
  });
});
