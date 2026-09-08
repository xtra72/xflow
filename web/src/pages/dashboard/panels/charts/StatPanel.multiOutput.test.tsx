// StatPanel 다중 출력(구간 대표값) 동작 테스트 — SPEC-CHART-002 M3.2.
//
// 레거시 경로는 StatPanel.test.tsx / StatPanelStoreSource.test.tsx 의 특성화가 지킨다.
// 이 파일은 `series_reduce` 가 **지정된** 경우의 신규 동작만 다룬다.
//
// 실제 `useStoreChartData` 를 쓰되 `queryMatrixFn` / `nowFn` 만 주입한다 — 대표값 변경이
// 재조회를 유발하지 않는다는 계약(AC-18)은 훅을 통째로 모킹하면 검증할 수 없다.
//
// @spec SPEC-CHART-002 AC-10 / AC-15 / AC-17 / AC-18

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, act } from '@testing-library/react';

import type { SeriesMatrix, SeriesMatrixQuery } from '@/services/api/seriesDataSource';

/** nowFn 고정 — 쿼리 윈도우를 결정적으로 만든다. */
const NOW = 5_000;

const query = vi.hoisted(() => ({
  fn: undefined as unknown as ReturnType<typeof vi.fn>,
  keysFn: undefined as unknown as ReturnType<typeof vi.fn>,
  calls: [] as SeriesMatrixQuery[],
}));

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

// 에이전트 목록이 비면 저장된 agent_name 이 그대로 쓰인다(SPEC-WEB-006 폴백).
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [] } }),
}));

// 채널 경로(AC-25)는 이 훅이 유일한 데이터 공급원이다. Store 모드 테스트에서는 훅이
// 비활성(enabled=false)이라 빈 결과가 실제 동작과 같으므로 기존 테스트에 영향이 없다.
const channelMock = vi.hoisted(() => ({
  current: {
    entries: [] as Array<{ timestamp: number; value: unknown }>,
    status: 'connected' as string,
    closedReason: undefined as string | undefined,
    errorReason: undefined as string | undefined,
  },
}));
vi.mock('./useChartChannel', () => ({
  useChartChannel: () => channelMock.current,
}));

// 훅은 실제 구현을 쓰고 테스트 주입점만 채운다.
vi.mock('./useStoreChartData', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./useStoreChartData')>();
  return {
    ...actual,
    useStoreChartData: (config: Parameters<typeof actual.useStoreChartData>[0], enabled: boolean) =>
      actual.useStoreChartData(config, enabled, {
        queryMatrixFn: (agentName, q, signal) => query.fn(agentName, q, signal),
        resolveKeysFn: (agentName, filters, signal) => query.keysFn(agentName, filters, signal),
        nowFn: () => NOW,
      }),
  };
});

import type { StoreSourceConfig } from './chartChannelTypes';
import { storeSeriesLabel } from './chartChannelTypes';
import StatPanel from './StatPanel';

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

function f1Config(overrides: Partial<StoreSourceConfig> = {}): StoreSourceConfig {
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

function panelConfig(extra: Record<string, unknown> = {}, store?: StoreSourceConfig) {
  return {
    channel_name: 'c1',
    decimal_places: 0,
    data_source: 'store',
    store_source: store ?? f1Config(),
    ...extra,
  };
}

async function renderPanel(config: Record<string, unknown>) {
  let view!: ReturnType<typeof render>;
  await act(async () => {
    view = render(<StatPanel panelId="p1" config={config} />);
  });
  return view;
}

function tiles(): Array<{ label: string; value: string }> {
  return screen.getAllByTestId('series-tile').map((tile) => ({
    label: tile.querySelector('[data-testid="stat-tile-label"]')?.textContent ?? '',
    value: tile.querySelector('[data-testid="stat-tile-value"]')?.textContent ?? '',
  }));
}

describe('StatPanel 다중 출력 (SPEC-CHART-002 M3)', () => {
  beforeEach(() => {
    query.calls = [];
    query.fn = vi.fn(async (_agent: string, q: SeriesMatrixQuery) => {
      query.calls.push(q);
      return F1_MATRIX;
    });
    query.keysFn = vi.fn(async () => ['k.room1', 'k.room2', 'k.room3']);
    channelMock.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });

  it('tile_rows 미지정이면 1행 — 시리즈 수만큼 열이 된다', async () => {
    await renderPanel(panelConfig({ series_reduce: 'max' }));
    const grid = screen.getByTestId('series-tile-grid');
    expect(grid.getAttribute('data-rows')).toBe('1');
    expect(grid.getAttribute('data-columns')).toBe('3');
  });

  it('tile_rows 를 지정하면 그 행 수를 목표로 열이 줄어든다', async () => {
    await renderPanel(panelConfig({ series_reduce: 'max', tile_rows: 2 }));
    const grid = screen.getByTestId('series-tile-grid');
    expect(grid.getAttribute('data-rows')).toBe('2');
    // 시리즈 3개 / 2행 → 2열(마지막 행이 덜 찬다).
    expect(grid.getAttribute('data-columns')).toBe('2');
  });

  it('series_reduce 지정 시 시리즈당 타일 1개를 렌더한다', async () => {
    await renderPanel(panelConfig({ series_reduce: 'max' }));

    const out = tiles();
    expect(out).toHaveLength(3);
    expect(out[0]).toMatchObject({ label: 'temp.room1', value: '26' });
    expect(out[1]).toMatchObject({ label: 'temp.room2', value: '23' });
    // 레거시 단일 값 슬롯은 렌더되지 않는다(경로가 갈렸다).
    expect(screen.queryByTestId('stat-value')).toBeNull();
    // 보조 delta 줄도 없다(OQ5 — 대표값 1개만).
    expect(screen.queryByTestId('stat-delta')).toBeNull();
  });

  // --- SPEC-CHART-003: 타일 경로의 보조 줄 ---
  // OQ5("stat 타일 보조 지표 미표시")를 **기본값이 아니라 선택지로** 뒤집는다.
  // 미지정일 때는 종전대로 아무것도 나오지 않아야 한다(spec.md §5 D2).

  it('AC-08: delta_display 미지정이면 타일에 변화량이 없다(종전 동작 유지)', async () => {
    await renderPanel(panelConfig({ series_reduce: 'last' }));

    expect(tiles()).toHaveLength(3);
    expect(screen.queryByTestId('stat-tile-delta')).toBeNull();
  });

  it('AC-17: window_stats 미지정이면 타일에 구간 통계가 없다', async () => {
    await renderPanel(panelConfig({ series_reduce: 'last' }));

    expect(screen.queryByTestId('stat-tile-window-stats')).toBeNull();
  });

  it('AC-09 / AC-10: enabled=true 면 타일마다 **그 시리즈의** 변화량을 그린다', async () => {
    await renderPanel(
      panelConfig({ series_reduce: 'last', delta_display: { enabled: true } }),
    );

    const deltas = screen
      .getAllByTestId('series-tile')
      .map((t) => t.querySelector('[data-testid="stat-tile-delta"]')?.textContent ?? null);

    // room1: 표본 20 22 26 24 21 → 21 − 24 = -3
    expect(deltas[0]).toContain('↓');
    expect(deltas[0]).toContain('-3');
    // room2: 표본 18 19 19 23 (null 은 표본이 아니다) → 23 − 19 = +4.
    // 시리즈를 가로질러 비교했다면 room1 의 값이 섞여 이 수가 나오지 않는다.
    expect(deltas[1]).toContain('↑');
    expect(deltas[1]).toContain('+4');
    // room3: 전 버킷 null → 표본 0개 → 변화량 줄 없음.
    expect(deltas[2]).toBeNull();
  });

  it('AC-21: 타일 구간 통계는 시리즈별 값을 그리고, 표본 없는 시리즈는 — 로 자리를 지킨다', async () => {
    await renderPanel(
      panelConfig({ series_reduce: 'last', window_stats: { avg: true, max: true, min: true } }),
    );

    const lines = screen
      .getAllByTestId('series-tile')
      .map((t) => t.querySelector('[data-testid="stat-tile-window-stats"]'));

    // room1 — 평 22.6(소수 0자리 반올림 23) · 최대 26 · 최소 20
    expect(lines[0]?.textContent).toContain('26');
    expect(lines[0]?.textContent).toContain('20');
    // room2 — 최대 23 · 최소 18
    expect(lines[1]?.textContent).toContain('23');
    expect(lines[1]?.textContent).toContain('18');
    // room3 — 표본 0개. 세 항목이 자리를 지키고 모두 —.
    expect(lines[2]?.querySelectorAll('[data-stat-kind]')).toHaveLength(3);
    expect((lines[2]?.textContent ?? '').match(/—/g)).toHaveLength(3);
  });

  // SPEC-CHART-004 M1-1.4 — 특성화(DDD PRESERVE).
  // 타일 경로에는 요소 직접 편집이 없다. M6 이후에도 이 서술은 유지되어야 한다
  // (spec.md §5 D2 — 그리드가 자리를 정하므로 요소 오프셋과 싸운다).
  it('M1-1.4: 타일 경로에는 편집 입구가 없고 저장된 layout 도 읽지 않는다', async () => {
    const { container } = await renderPanel(
      panelConfig({
        series_reduce: 'last',
        value_layout: { offset_x: 20, font_size: 90 },
        delta_layout: { offset_y: -10 },
        stats_layout: { font_size: 30 },
      }),
    );

    expect(tiles()).toHaveLength(3);
    expect(container.querySelectorAll('[data-panel-drag]')).toHaveLength(0);
    expect(container.querySelectorAll('[data-panel-resize]')).toHaveLength(0);
    expect(screen.queryByTestId('stat-edit-toggle')).toBeNull();

    // 저장된 layout 은 무시된다 — 타일 값은 기본 크기(24)를 유지하고 transform 도 없다.
    const value = container.querySelector('[data-testid="stat-tile-value"]') as HTMLElement;
    expect(value.style.fontSize).toBe('24px');
    expect(value.style.transform).toBe('');
  });

  it('AC-22: 타일 구간 통계 라벨은 타일이 1개여도 축약형이다', async () => {
    // 축약 여부는 타일 **개수**가 아니라 경로가 정한다 — 시리즈를 하나로 줄여도
    // 라벨이 갑자기 길어지면 안 된다.
    query.fn = vi.fn(async () => ({
      columns: ['k.room1'],
      rows: F1_MATRIX.rows.map((r) => ({ bucketStartMs: r.bucketStartMs, values: [r.values[0]!] })),
    }));
    query.keysFn = vi.fn(async () => ['k.room1']);
    await renderPanel(
      panelConfig(
        { series_reduce: 'last', window_stats: { avg: true } },
        f1Config({ series: [{ key: 'k.room1', alias: 'temp.room1' }] }),
      ),
    );

    expect(tiles()).toHaveLength(1);
    const line = screen.getByTestId('stat-tile-window-stats');
    // 축약 라벨은 aria-hidden, 완결 낱말은 sr-only 로 함께 존재한다.
    expect(line.querySelector('[aria-hidden="true"]')).not.toBeNull();
    expect(line.querySelectorAll('.sr-only')).toHaveLength(1);
  });

  it('대표값이 undefined 인 시리즈도 슬롯을 유지하고 — 를 표시한다', async () => {
    await renderPanel(panelConfig({ series_reduce: 'avg' }));

    const out = tiles();
    expect(out).toHaveLength(3);
    // room3 은 전 버킷 null → 표본 0개 → undefined. 슬롯은 남고 값만 —.
    expect(out[2]!.label).toBe('temp.room3');
    expect(out[2]!.value).toBe('—');
  });

  it('타일 순서는 store_source.series 배열 순서를 따른다', async () => {
    // 값 크기 순(26 > 23)이 아니라 config 순서를 유지해야 한다(UB1-11).
    await renderPanel(panelConfig({ series_reduce: 'max' }));
    expect(tiles().map((t) => t.label)).toEqual([
      'temp.room1',
      'temp.room2',
      'temp.room3',
    ]);
  });

  it('빈 윈도우에서 count 만 0 을 표시하고 나머지는 — 를 표시한다', async () => {
    await renderPanel(panelConfig({ series_reduce: 'count' }));
    expect(tiles().map((t) => t.value)).toEqual(['5', '4', '0']);
  });

  // @spec SPEC-CHART-002 AC-26
  it('빈 윈도우 sum 은 0 이 아니라 — 로 표시된다', async () => {
    // 수학적으로 공집합의 합은 0 이지만, 여기서 0 을 표시하면 "합이 0 인 시리즈" 와
    // "표본이 없는 시리즈" 가 화면에서 구분되지 않는다. count 만 0 을 낸다(§2.2).
    await renderPanel(panelConfig({ series_reduce: 'sum' }));

    const out = tiles();
    expect(out).toHaveLength(3);
    // room3 은 전 버킷 null → 표본 0개.
    expect(out[2]!.label).toBe('temp.room3');
    expect(out[2]!.value).toBe('—');
    expect(out[2]!.value).not.toBe('0');
    // 표본이 있는 시리즈는 정상적으로 합계를 낸다(대비군).
    expect(out[0]!.value).toBe('113');
  });

  // @spec SPEC-CHART-002 AC-31
  it('tag 매칭 0개는 빈 상태로 렌더되고 오류가 아니다', async () => {
    query.keysFn = vi.fn(async () => []);

    await renderPanel(
      panelConfig(
        { series_reduce: 'max' },
        f1Config({ selection_mode: 'tag', series: [], tag_filters: { room: 'zzz' } }),
      ),
    );

    // 크래시 없이 빈 상태 — 타일은 하나도 없다.
    expect(screen.queryAllByTestId('series-tile')).toHaveLength(0);
    // 오류 오버레이가 아니다(status !== 'error'). 0개 매칭은 정상적인 빈 결과다(UB2-2).
    expect(screen.queryByTestId('stat-overlay')).toBeNull();
    // 해석된 키가 없으므로 매트릭스 조회 자체가 일어나지 않는다.
    expect(query.fn).not.toHaveBeenCalled();
  });

  it('출력 라벨은 storeSeriesLabel 규칙(alias > series_name_format > 서술 표기)을 따른다', async () => {
    const store = f1Config({
      series_name_format: '{$.measurement}-fmt',
      series: [
        { key: 'k.room1', alias: 'aliasA' }, // 1) alias 우선
        { key: 'k.room2' }, // 2) 패널 이름 형식
        { key: 'k.room3', field: 'gauge', tags: { room: '3' } }, // 3) 서술 표기
      ],
    });
    await renderPanel(panelConfig({ series_reduce: 'last' }, store));

    const expected = store.series.map((ref) =>
      storeSeriesLabel(ref, store.series_name_format),
    );
    expect(tiles().map((t) => t.label)).toEqual(expected);
    // 자체 명명 규칙을 만들지 않았음을 못박는다.
    expect(expected[0]).toBe('aliasA');
    expect(expected[1]).toBe('k.room2-fmt');
  });

  it('alias 변경이 색상/순서에 영향을 주지 않는다 (동일성은 인덱스 기준)', async () => {
    const before = f1Config({
      series: [
        { key: 'k.room1', alias: 'A', color: '#ff0000' },
        { key: 'k.room2', alias: 'B', color: '#0000ff' },
        { key: 'k.room3', alias: 'C' },
      ],
    });
    const view = await renderPanel(panelConfig({ series_reduce: 'max' }, before));
    const labelColor = (): string[] =>
      screen.getAllByTestId('stat-tile-label').map((el) => el.style.color);
    expect(labelColor()).toEqual(['rgb(255, 0, 0)', 'rgb(0, 0, 255)', '']);

    const after = f1Config({
      series: [
        { key: 'k.room1', alias: 'Z-renamed', color: '#ff0000' },
        { key: 'k.room2', alias: 'B', color: '#0000ff' },
        { key: 'k.room3', alias: 'C' },
      ],
    });
    await act(async () => {
      view.rerender(<StatPanel panelId="p1" config={panelConfig({ series_reduce: 'max' }, after)} />);
    });

    // 이름만 바뀌고 순서/색은 인덱스 기준 그대로다.
    expect(tiles().map((t) => t.label)).toEqual(['Z-renamed', 'B', 'C']);
    expect(labelColor()).toEqual(['rgb(255, 0, 0)', 'rgb(0, 0, 255)', '']);
  });

  it('stat: 값 색은 threshold 가, 라벨 색은 시리즈 color 가 결정한다', async () => {
    const store = f1Config({
      series: [{ key: 'k.room1', alias: 'temp.room1', color: '#ff0000' }],
    });
    // 시리즈 1개 → 응답도 1컬럼이어야 컬럼↔시리즈 정렬 매핑이 성립한다.
    query.fn = vi.fn(async () => ({
      columns: ['k.room1'],
      rows: F1_MATRIX.rows.map((r) => ({ bucketStartMs: r.bucketStartMs, values: [r.values[0]!] })),
    }));
    await renderPanel(
      panelConfig(
        {
          series_reduce: 'max',
          threshold_color_rules: [{ min: 0, color: '#00ff00' }],
        },
        store,
      ),
    );

    // 시리즈 색은 라벨에만, 임계 색은 값에만 — 두 축은 서로를 덮어쓰지 않는다(§2.6).
    expect(screen.getByTestId('stat-tile-label').style.color).toBe('rgb(255, 0, 0)');
    expect(screen.getByTestId('stat-tile-value').style.color).toBe('rgb(0, 255, 0)');
  });

  it('series_reduce 변경은 queryMatrixFn 을 다시 호출하지 않는다', async () => {
    const store = f1Config();
    const view = await renderPanel(panelConfig({ series_reduce: 'max' }, store));
    expect(query.fn).toHaveBeenCalledTimes(1);
    expect(tiles()[0]!.value).toBe('26');

    await act(async () => {
      view.rerender(
        <StatPanel panelId="p1" config={panelConfig({ series_reduce: 'min' }, store)} />,
      );
    });

    // 값은 즉시 바뀌지만 재조회는 없다(E1 — pollKey 불변).
    expect(tiles()[0]!.value).toBe('20');
    expect(query.fn).toHaveBeenCalledTimes(1);
  });

  it('store_source.aggregation 변경은 queryMatrixFn 을 다시 호출한다', async () => {
    const view = await renderPanel(panelConfig({ series_reduce: 'max' }, f1Config()));
    expect(query.fn).toHaveBeenCalledTimes(1);

    await act(async () => {
      view.rerender(
        <StatPanel
          panelId="p1"
          config={panelConfig({ series_reduce: 'max' }, f1Config({ aggregation: 'max' }))}
        />,
      );
    });

    // 대조군: 조회 축(aggregation)은 재조회를 유발한다.
    expect(query.fn).toHaveBeenCalledTimes(2);
    expect(query.calls[1]!.aggregation).toBe('max');
  });

  it('시리즈가 상한을 넘으면 12개만 그리고 +K 표기를 노출한다', async () => {
    const columns = Array.from({ length: 20 }, (_, i) => `k.s${i}`);
    query.fn = vi.fn(async () => ({
      columns,
      rows: [{ bucketStartMs: 1000, values: columns.map((_, i) => i + 1) }],
    }));
    const store = f1Config({
      series: columns.map((key, i) => ({ key, alias: `s${i}` })),
    });
    await renderPanel(panelConfig({ series_reduce: 'last' }, store));

    expect(screen.getAllByTestId('series-tile')).toHaveLength(12);
    expect(screen.getByTestId('series-tile-truncation').textContent).toContain('+8');
  });

  it('시리즈 선택이 0개면 신규 경로의 빈 상태(—)를 보여준다', async () => {
    // 선택이 비면 store 경로 자체가 비활성이므로 채널 경로 빈 상태와 같은 — 가 남는다.
    await renderPanel(panelConfig({ series_reduce: 'max' }, f1Config({ series: [] })));
    expect(screen.queryByTestId('series-tile')).toBeNull();
    expect(screen.getByTestId('stat-value').textContent).toContain('—');
  });
});

// ---------------------------------------------------------------------------
// AC-25 — 채널 모드에서 series_reduce 는 읽히지 않고 삭제되지도 않는다 (§2.10 [S2]).
//
// 설정 UI 쪽 절반(선택기 미노출)은 `ChartPanelSections.test.tsx` 가 잠근다. 여기서는
// **렌더 절반** — 저장된 series_reduce 가 있어도 채널 경로 결과가 그대로 나오는지 —
// 를 잠근다.
// ---------------------------------------------------------------------------

describe('채널 모드에서의 series_reduce (AC-25 / §2.10 [S2])', () => {
  beforeEach(() => {
    query.calls = [];
    query.fn = vi.fn(async (_agent: string, q: SeriesMatrixQuery) => {
      query.calls.push(q);
      return F1_MATRIX;
    });
    channelMock.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });

  // 채널이 패널 소스에서 빠지면서 이 규칙이 뒤집혔다. 폐지된 `'channel'` 값은 store 로
  // 접히므로, 그 config 에 갖춰진 store_source 와 대표값이 **그대로 살아난다** — 구 패널을
  // 열면 고른 적 있는 시리즈가 다시 보이는 것이 이 변화의 목적이다.
  it("폐지된 'channel' 값도 store 경로로 대표값을 낸다", async () => {
    await renderPanel({
      channel_name: 'c1',
      decimal_places: 0,
      data_source: 'channel',
      store_source: f1Config(),
      series_reduce: 'max',
    });

    expect(tiles().map((t) => t.value)).toEqual(['26', '23', '—']);
  });

  it('채널 모드 전환이 series_reduce 를 config 에서 제거하지 않는다', async () => {
    // 패널은 config 를 읽기만 한다 — 렌더가 저장값을 지우면 Store 로 되돌렸을 때 설정이
    // 사라진다(§2.10 마지막 문단).
    const config: Record<string, unknown> = {
      channel_name: 'c1',
      decimal_places: 0,
      data_source: 'channel',
      store_source: f1Config(),
      series_reduce: 'max',
    };
    await renderPanel(config);
    expect(config.series_reduce).toBe('max');
    expect(config.store_source).toBeDefined();
  });

  it('data_source 를 store 로 고쳐도 같은 값을 낸다 — 이관은 값을 바꾸지 않는다', async () => {
    const view = await renderPanel({
      channel_name: 'c1',
      decimal_places: 0,
      data_source: 'channel',
      store_source: f1Config(),
      series_reduce: 'max',
    });
    const before = tiles().map((t) => t.value);
    view.unmount();

    await renderPanel(panelConfig({ series_reduce: 'max' }));
    expect(tiles().map((t) => t.value)).toEqual(before);
  });
});

// ---------------------------------------------------------------------------
// AC-27 — 폴링으로 값이 바뀌어도 출력 순서는 config 순서를 유지한다 (UB1-11).
//
// "타일 순서는 config 순서" 는 이미 잠겨 있지만, 그것만으로는 **정렬 로직이 없다**는 것을
// 증명하지 못한다(첫 렌더의 값 순서와 config 순서가 우연히 같을 수 있다). 폴링으로 값의
// 대소를 뒤집어도 자리가 그대로인지가 실제 계약이다 — 매 폴링마다 타일이 자리를 바꾸면
// 읽을 수 없다.
// ---------------------------------------------------------------------------

describe('폴링 간 출력 순서 안정성 (AC-27 / UB1-11)', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    channelMock.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('폴링으로 값이 바뀌어도 출력 순서는 config 순서를 유지한다', async () => {
    // 1회차 [A=10, B=90, C=50] → 2회차 [A=90, B=10, C=50] 로 대소를 뒤집는다.
    const matrixOf = (values: number[]) => ({
      columns: ['k.a', 'k.b', 'k.c'],
      rows: [{ bucketStartMs: 1000, values }],
    });
    let call = 0;
    query.fn = vi.fn(async () => matrixOf(call++ === 0 ? [10, 90, 50] : [90, 10, 50]));

    const store = f1Config({
      series: [
        { key: 'k.a', alias: 'A' },
        { key: 'k.b', alias: 'B' },
        { key: 'k.c', alias: 'C' },
      ],
      refresh_interval_ms: 5000,
    });

    let view!: ReturnType<typeof render>;
    await act(async () => {
      view = render(<StatPanel panelId="p1" config={panelConfig({ series_reduce: 'last' }, store)} />);
    });
    await act(async () => {
      await Promise.resolve();
    });

    expect(tiles()).toEqual([
      { label: 'A', value: '10' },
      { label: 'B', value: '90' },
      { label: 'C', value: '50' },
    ]);

    await act(async () => {
      vi.advanceTimersByTime(5_000);
      await Promise.resolve();
    });

    // 값은 바뀌었지만 자리는 그대로다.
    expect(tiles()).toEqual([
      { label: 'A', value: '90' },
      { label: 'B', value: '10' },
      { label: 'C', value: '50' },
    ]);
    view.unmount();
  });
});

// ---------------------------------------------------------------------------
// AC-32 — "값 없음" 과 "폴링 실패" 가 화면에서 구분된다 (§2.13 [UB2] 4).
//
// 둘 다 "숫자가 안 보인다" 로 끝나면 사용자는 데이터가 없는 것인지 시스템이 고장 난
// 것인지 알 수 없다. 전자는 — 만, 후자는 오버레이 + 직전 렌더 보존이다.
// ---------------------------------------------------------------------------

describe('값 없음과 폴링 실패의 구분 (AC-32 / UB2-4)', () => {
  beforeEach(() => {
    query.calls = [];
    channelMock.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });

  it('대표값 undefined 는 오류 오버레이를 띄우지 않는다', async () => {
    query.fn = vi.fn(async () => F1_MATRIX);
    await renderPanel(panelConfig({ series_reduce: 'max' }));

    // room3 은 전 버킷 null → undefined → — 만 표시된다.
    expect(tiles()[2]).toMatchObject({ label: 'temp.room3', value: '—' });
    expect(screen.queryByTestId('stat-overlay')).toBeNull();
  });

  it('폴링 실패는 오류 오버레이를 띄우고 직전 렌더를 보존한다', async () => {
    vi.useFakeTimers();
    try {
      let call = 0;
      query.fn = vi.fn(async () => {
        if (call++ === 0) return F1_MATRIX;
        throw new Error('boom');
      });

      const store = f1Config({ refresh_interval_ms: 5000 });
      let view!: ReturnType<typeof render>;
      await act(async () => {
        view = render(
          <StatPanel panelId="p1" config={panelConfig({ series_reduce: 'max' }, store)} />,
        );
      });
      await act(async () => {
        await Promise.resolve();
      });
      expect(screen.queryByTestId('stat-overlay')).toBeNull();
      const before = tiles();
      expect(before[0]).toMatchObject({ value: '26' });

      await act(async () => {
        vi.advanceTimersByTime(5_000);
        await Promise.resolve();
        await Promise.resolve();
      });

      // 오버레이가 뜨고(구분 가능) 마지막 성공 렌더는 파괴되지 않는다(§2.13 [UB2] 3).
      expect(screen.getByTestId('stat-overlay')).toBeInTheDocument();
      expect(tiles()).toEqual(before);
      view.unmount();
    } finally {
      vi.useRealTimers();
    }
  });
});
