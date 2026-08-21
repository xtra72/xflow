// GaugePanel 신규 Store 데이터 소스 경로 동작 테스트 — SPEC-CHART-002 M4.7.
//
// 레거시 `config.dataSources[]` 경로는 GaugePanel.test.tsx / GaugePanel.storeLegacy.test.tsx
// 의 특성화(CH-11~CH-18)가 지킨다. 이 파일은 `data_source:'store'` + 활성 `store_source`
// + `series_reduce` 세 조건이 모두 성립할 때의 **신규 동작만** 다룬다.
//
// StatPanel.multiOutput.test.tsx 와 같은 방식으로 실제 `useStoreChartData` 를 쓰되
// `queryMatrixFn` / `resolveKeysFn` / `nowFn` 만 주입한다 — 훅을 통째로 모킹하면
// "대표값 변경이 재조회를 유발하지 않는다"(AC-18)를 검증할 수 없다.
//
// @spec SPEC-CHART-002 AC-11 / AC-17 / AC-14 / AC-18

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, act } from '@testing-library/react';

import type { SeriesMatrix, SeriesMatrixQuery } from '@/services/api/seriesDataSource';

/** nowFn 고정 — 쿼리 윈도우를 결정적으로 만든다. */
const NOW = 5_000;

const query = vi.hoisted(() => ({
  fn: undefined as unknown as ReturnType<typeof vi.fn>,
  keysFn: undefined as unknown as ReturnType<typeof vi.fn>,
}));

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

// 에이전트 목록이 비면 저장된 agent_name 이 그대로 쓰인다(SPEC-WEB-006 폴백).
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [] } }),
}));

// 레거시 store 폴링(useStoreLatestValue)의 POST 를 가로챈다. 신규 경로 테스트에서는
// 레거시 바인딩이 없으므로 호출되지 않아야 하며, 그 사실 자체를 단언한다.
const mockPost = vi.hoisted(() => ({
  fn: vi.fn(async (_url: string, _body: unknown) => ({
    entries: [] as Array<{ value: unknown; timestamp: number }>,
  })),
}));
vi.mock('@/services/api/client', () => ({
  post: (url: string, body: unknown) => mockPost.fn(url, body),
}));

// 채널 훅은 idle 로 고정한다(신규 경로에는 chart-emitter 바인딩이 없다).
vi.mock('./charts/useChartChannel', () => ({
  useChartChannel: () => ({
    entries: [],
    status: 'idle' as const,
    closedReason: undefined,
    errorReason: undefined,
  }),
}));

// 훅은 실제 구현을 쓰고 테스트 주입점만 채운다.
vi.mock('./charts/useStoreChartData', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./charts/useStoreChartData')>();
  return {
    ...actual,
    useStoreChartData: (
      config: Parameters<typeof actual.useStoreChartData>[0],
      enabled: boolean,
    ) =>
      actual.useStoreChartData(config, enabled, {
        queryMatrixFn: (agentName, q, signal) => query.fn(agentName, q, signal),
        resolveKeysFn: (agentName, filters, signal) => query.keysFn(agentName, filters, signal),
        nowFn: () => NOW,
      }),
  };
});

import type { StoreSourceConfig } from './charts/chartChannelTypes';
import GaugePanel from './GaugePanel';

/** F1 — 3시리즈 × 5버킷(acceptance.md 공통 픽스처). room3 은 전 버킷 null. */
const F1_MATRIX: SeriesMatrix = {
  columns: ['k.room1', 'k.room2', 'k.room3'],
  rows: [
    { bucketStartMs: 1000, values: [20, 18, null] },
    { bucketStartMs: 2000, values: [22, null, null] },
    { bucketStartMs: 3000, values: [26, 19, null] },
    { bucketStartMs: 4000, values: [24, 19, null] },
    { bucketStartMs: 5000, values: [21, 23, null] },
  ],
};

function f1Store(overrides: Partial<StoreSourceConfig> = {}): StoreSourceConfig {
  return {
    agent_name: 'store-1',
    namespace: 'default',
    selection_mode: 'keys',
    series: [
      { key: 'k.room1', alias: 'temp.room1' },
      { key: 'k.room2', alias: 'temp.room2' },
      { key: 'k.room3', alias: 'temp.room3' },
    ],
    time_window_ms: 60_000,
    interval_ms: 1_000,
    aggregation: 'average',
    ...overrides,
  };
}

/** 게이지 패널 config — 패널 공통 축(min/max/unit/thresholds/gaugeType)을 함께 싣는다. */
function panelConfig(extra: Record<string, unknown> = {}, store?: StoreSourceConfig) {
  return {
    gaugeType: 'simple',
    value: 77, // 신규 경로에서는 쓰이지 않아야 하는 정적 값(레거시 폴백 감지용)
    min: 0,
    max: 100,
    unit: '°C',
    data_source: 'store',
    store_source: store ?? f1Store(),
    ...extra,
  };
}

async function renderPanel(
  config: Record<string, unknown>,
  props: Partial<{
    onConfigChange: (c: Record<string, unknown>) => void;
    onTitleChange: (t: string) => void;
  }> = {},
) {
  let view!: ReturnType<typeof render>;
  await act(async () => {
    view = render(
      <GaugePanel panelId="p1" title="테스트 게이지" config={config} {...props} />,
    );
  });
  return view;
}

/** 렌더된 게이지 타일의 캡션 + 값 텍스트. */
function gaugeTiles(): Array<{ caption: string; value: string }> {
  return screen.getAllByTestId('series-tile').map((tile) => ({
    caption: tile.querySelector('[data-testid="gauge-tile-caption"]')?.textContent ?? '',
    // SVG 중앙 텍스트의 첫 tspan 이 값(단위는 두 번째 tspan).
    value: tile.querySelector('svg text tspan')?.textContent ?? '',
  }));
}

describe('GaugePanel Store 데이터 소스 (SPEC-CHART-002 M4)', () => {
  beforeEach(() => {
    mockPost.fn.mockClear();
    query.fn = vi.fn(async (_agent: string, _q: SeriesMatrixQuery) => F1_MATRIX);
    query.keysFn = vi.fn(async () => ['k.room1', 'k.room2']);
  });

  it('store_source + series_reduce 지정 시 시리즈당 게이지 1개를 렌더한다', async () => {
    await renderPanel(panelConfig({ series_reduce: 'last' }));

    const tiles = gaugeTiles();
    expect(tiles).toHaveLength(3);
    expect(tiles[0]).toMatchObject({ caption: 'temp.room1', value: '21' });
    expect(tiles[1]).toMatchObject({ caption: 'temp.room2', value: '23' });
    // 정적 config.value(77)로 폴백하지 않는다 — 레거시 경로가 밀려났다는 증거.
    expect(screen.queryByText('77')).toBeNull();
    // 레거시 store 폴링은 시작되지 않는다.
    expect(mockPost.fn).not.toHaveBeenCalled();
  });

  it('각 게이지는 패널 공통 min/max/unit/gaugeType 을 공유한다', async () => {
    await renderPanel(panelConfig({ series_reduce: 'max' }));

    const tiles = screen.getAllByTestId('series-tile');
    expect(tiles).toHaveLength(3);
    for (const tile of tiles) {
      // gaugeType:'simple' → 200x200 viewBox. 세 게이지가 같은 렌더러를 쓴다.
      expect(tile.querySelector('svg')?.getAttribute('viewBox')).toBe('0 0 200 200');
    }
    // 값이 있는 두 게이지는 공통 단위를 함께 표시한다.
    expect(screen.getAllByText('°C')).toHaveLength(2);
    // max 대표값 — room1=26, room2=23.
    expect(gaugeTiles().map((t) => t.value)).toEqual(['26', '23', '--']);
  });

  it('값 없는 시리즈 게이지는 -- 를 표시하고 슬롯을 유지한다', async () => {
    await renderPanel(panelConfig({ series_reduce: 'avg' }));

    const tiles = gaugeTiles();
    // room3 은 전 버킷 null → 표본 0개 → undefined. 슬롯은 남고 값만 --(§2.4).
    expect(tiles).toHaveLength(3);
    expect(tiles[2]).toMatchObject({ caption: 'temp.room3', value: '--' });
  });

  it('게이지 순서는 store_source.series 배열 순서를 따른다', async () => {
    // 값 크기 순(26 > 23)이 아니라 config 순서를 유지해야 한다(UB1-11).
    await renderPanel(panelConfig({ series_reduce: 'max' }));
    expect(gaugeTiles().map((t) => t.caption)).toEqual([
      'temp.room1',
      'temp.room2',
      'temp.room3',
    ]);
  });

  it('gauge: 호 색은 thresholds 가, 캡션 색은 시리즈 color 가 결정한다', async () => {
    // §2.6 — 값을 의미하는 색과 시리즈를 식별하는 색은 서로를 덮어쓰지 않는다.
    const store = f1Store({
      series: [{ key: 'k.room1', alias: 'temp.room1', color: '#ff0000' }],
    });
    query.fn = vi.fn(async () => ({
      columns: ['k.room1'],
      rows: F1_MATRIX.rows.map((r) => ({
        bucketStartMs: r.bucketStartMs,
        values: [r.values[0]!],
      })),
    }));
    await renderPanel(
      panelConfig(
        {
          series_reduce: 'last',
          thresholds: [{ name: '', color: '#00ff00', from: 0, to: 100 }],
          // 임계값 sector 를 끄면 남는 path 는 값 호(arc) 하나뿐이다.
          showThresholdZones: false,
        },
        store,
      ),
    );

    const tile = screen.getByTestId('series-tile');
    const paths = tile.querySelectorAll('path');
    expect(paths).toHaveLength(1);
    expect(paths[0]!.getAttribute('fill')).toBe('#00ff00');
    expect(screen.getByTestId('gauge-tile-caption').style.color).toBe('rgb(255, 0, 0)');
  });

  it('시리즈 색이 없으면 캡션에 팔레트 폴백을 적용하지 않는다', async () => {
    // stat/gauge 는 색 미지정 시 기본 라벨색이다(§2.6 — 팔레트는 bar/pie 채움색 규칙).
    await renderPanel(panelConfig({ series_reduce: 'last' }));
    for (const caption of screen.getAllByTestId('gauge-tile-caption')) {
      expect(caption.style.color).toBe('');
    }
  });

  it('연결 상태 아이콘을 store 모드에서 노출한다', async () => {
    // M4.6 — 레거시는 chart-emitter 일 때만 아이콘을 냈다(CH-17). 신규 경로는 Store
    // 조회 상태를 그대로 노출한다.
    await renderPanel(panelConfig({ series_reduce: 'last' }));
    expect(screen.getByTestId('chart-status-icon').getAttribute('data-status')).toBe(
      'connected',
    );
  });

  it('Store 조회가 실패하면 아이콘이 error 상태를 표시한다', async () => {
    query.fn = vi.fn(async () => {
      throw new Error('boom');
    });
    await renderPanel(panelConfig({ series_reduce: 'last' }));
    expect(screen.getByTestId('chart-status-icon').getAttribute('data-status')).toBe('error');
  });

  it('시리즈가 상한을 넘으면 12개만 그리고 +K 표기를 노출한다', async () => {
    const columns = Array.from({ length: 20 }, (_, i) => `k.s${i}`);
    query.fn = vi.fn(async () => ({
      columns,
      rows: [{ bucketStartMs: 1000, values: columns.map((_, i) => i + 1) }],
    }));
    await renderPanel(
      panelConfig(
        { series_reduce: 'last' },
        f1Store({ series: columns.map((key, i) => ({ key, alias: `s${i}` })) }),
      ),
    );

    expect(screen.getAllByTestId('series-tile')).toHaveLength(12);
    expect(screen.getByTestId('series-tile-truncation').textContent).toContain('+8');
  });

  it('multi_output_limit 으로 상한을 재정의할 수 있다', async () => {
    await renderPanel(panelConfig({ series_reduce: 'last', multi_output_limit: 2 }));
    expect(screen.getAllByTestId('series-tile')).toHaveLength(2);
    expect(screen.getByTestId('series-tile-truncation').textContent).toContain('+1');
  });

  it('series_reduce 변경은 queryMatrixFn 을 다시 호출하지 않는다', async () => {
    const store = f1Store();
    const view = await renderPanel(panelConfig({ series_reduce: 'max' }, store));
    expect(query.fn).toHaveBeenCalledTimes(1);
    expect(gaugeTiles()[0]!.value).toBe('26');

    await act(async () => {
      view.rerender(
        <GaugePanel
          panelId="p1"
          title="테스트 게이지"
          config={panelConfig({ series_reduce: 'min' }, store)}
        />,
      );
    });

    // 값은 즉시 바뀌지만 재조회는 없다(E1 — pollKey 불변).
    expect(gaugeTiles()[0]!.value).toBe('20');
    expect(query.fn).toHaveBeenCalledTimes(1);
  });

  it('tag 모드에서도 해석된 키만큼 게이지를 렌더한다', async () => {
    query.fn = vi.fn(async () => ({
      columns: ['k.room1', 'k.room2'],
      rows: [{ bucketStartMs: 1000, values: [11, 22] }],
    }));
    await renderPanel(
      panelConfig(
        { series_reduce: 'last' },
        f1Store({ selection_mode: 'tag', series: [], tag_filters: { room: 'a' } }),
      ),
    );

    expect(query.keysFn).toHaveBeenCalled();
    expect(gaugeTiles().map((t) => t.value)).toEqual(['11', '22']);
  });

  it('renderDashboardPanel 이 넘기는 onConfigChange/onTitleChange 계약을 바꾸지 않는다', async () => {
    // GaugePanel 은 두 콜백을 받는 유일한 차트 계열 패널이다(renderDashboardPanel.tsx:255).
    // 신규 경로가 이를 호출하거나 요구해서는 안 된다.
    const onConfigChange = vi.fn();
    const onTitleChange = vi.fn();
    await renderPanel(panelConfig({ series_reduce: 'last' }), {
      onConfigChange,
      onTitleChange,
    });

    expect(screen.getAllByTestId('series-tile')).toHaveLength(3);
    expect(onConfigChange).not.toHaveBeenCalled();
    expect(onTitleChange).not.toHaveBeenCalled();
  });
});

describe('GaugePanel 신규 경로 진입 조건 (SPEC-CHART-002 §2.9 [S1])', () => {
  beforeEach(() => {
    // mockClear 는 구현을 되돌리지 않는다 — AC-31 테스트가 심은 구현이 다음 테스트로
    // 새지 않도록 기본 구현(빈 entries)까지 명시적으로 복원한다.
    mockPost.fn.mockReset();
    mockPost.fn.mockImplementation(async () => ({ entries: [] }));
    query.fn = vi.fn(async () => F1_MATRIX);
    query.keysFn = vi.fn(async () => []);
  });

  // @spec SPEC-CHART-002 AC-31
  it('gauge: store_source 비활성 시 레거시 값으로 폴백한다', async () => {
    // 진리표 3행 — data_source:'store' + series_reduce 있음 + store_source 비활성
    // (keys 모드인데 series 0개) → legacy. 다른 3종 패널은 이 상황에서 빈 상태를 내지만
    // 게이지만 레거시 바인딩이 값을 낼 수 있으면 그 값을 낸다(의도된 비대칭).
    mockPost.fn.mockImplementation(async () => ({
      entries: [{ value: 42, timestamp: 1000 }],
    }));

    await renderPanel(
      panelConfig(
        {
          series_reduce: 'max',
          dataSources: [
            {
              sourceType: 'store',
              storeAgent: 'store-1',
              storeKey: 'k.legacy',
              storeNamespace: 'default',
            },
          ],
        },
        f1Store({ selection_mode: 'keys', series: [] }),
      ),
    );

    // 신규 경로로 진입하지 않는다.
    expect(screen.queryByTestId('gauge-tiles')).toBeNull();
    expect(screen.queryByTestId('series-tile')).toBeNull();
    // 레거시 폴링이 살아 있고 그 값이 표시된다 — 정적 config.value(77)가 아니다.
    expect(mockPost.fn).toHaveBeenCalled();
    expect(screen.getByText('42')).toBeInTheDocument();
    expect(screen.queryByText('77')).toBeNull();
    // 매트릭스 조회는 시작되지 않는다(store_source 가 비활성이므로).
    expect(query.fn).not.toHaveBeenCalled();
  });

  it('series_reduce 가 없으면 store_source 가 있어도 레거시 경로다', async () => {
    // 유무 스위치(§4.2) — 부재는 "기본값 last" 가 아니라 레거시다.
    await renderPanel(panelConfig({}));

    expect(screen.queryByTestId('gauge-tiles')).toBeNull();
    expect(screen.queryByTestId('series-tile')).toBeNull();
    // 바인딩이 없으므로 정적 config.value 가 그대로 표시된다(기존 동작).
    expect(screen.getByText('77')).toBeInTheDocument();
    // Store 조회 자체가 시작되지 않는다.
    expect(query.fn).not.toHaveBeenCalled();
  });

  it("data_source 가 'store' 가 아니면 series_reduce 가 있어도 레거시 경로다", async () => {
    // §2.10 [S2] — 채널 모드에서는 series_reduce 를 읽지 않는다.
    await renderPanel(panelConfig({ series_reduce: 'max', data_source: 'channel' }));

    expect(screen.queryByTestId('series-tile')).toBeNull();
    expect(screen.getByText('77')).toBeInTheDocument();
    expect(query.fn).not.toHaveBeenCalled();
  });

  it('store_source 가 비활성(시리즈 0개)이면 레거시 경로로 폴백한다', async () => {
    // §2.9 게이지 추가 조건 — 신규 경로가 실제로 값을 낼 수 있을 때만 레거시를 밀어낸다.
    await renderPanel(panelConfig({ series_reduce: 'max' }, f1Store({ series: [] })));

    expect(screen.queryByTestId('series-tile')).toBeNull();
    expect(screen.getByText('77')).toBeInTheDocument();
    expect(query.fn).not.toHaveBeenCalled();
  });

  it('tag 모드에서 해석된 키가 0개면 신규 경로의 빈 상태(--)를 보여준다', async () => {
    // 진입 조건(태그 ≥ 1)은 성립하므로 신규 경로다. 결과가 0개인 것은 정상적인
    // 빈 상태이며(UB2-2) 레거시 정적 값으로 몰래 되돌아가지 않는다.
    await renderPanel(
      panelConfig(
        { series_reduce: 'max' },
        f1Store({ selection_mode: 'tag', series: [], tag_filters: { room: 'none' } }),
      ),
    );

    expect(screen.queryByTestId('series-tile')).toBeNull();
    expect(screen.getByText('--')).toBeInTheDocument();
    expect(screen.queryByText('77')).toBeNull();
  });
});
