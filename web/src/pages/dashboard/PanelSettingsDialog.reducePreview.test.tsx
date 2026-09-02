// 설정 화면의 stat / gauge 라이브 미리보기 — SPEC-CHART-002 M6.3 / §2.11 [O1].
//
// 라인 차트가 이미 갖고 있는 선례(`isStoreLinePreview`)를 stat 과 gauge 로 넓힌다:
// **신규 경로가 실제로 값을 낼 수 있을 때만** 합성 샘플 대신 실제 패널을 draft config 로
// 렌더한다. 그래야 사용자가 대표값·다중 출력의 결과를 저장 전에 확인할 수 있다.
//
// 이 파일이 잠그는 계약은 두 가지다.
//   1. 노출 조건이 각 패널의 **자체 활성 조건과 같다** — 미리보기와 실제 렌더가 서로 다른
//      조건으로 갈리면 "설정 화면에서는 보이는데 대시보드에서는 안 보인다" 가 된다.
//   2. 레거시 경로에서는 기존 미리보기가 그대로 남는다(순수 추가, 회귀 없음).

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, render, screen, within } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';
import type { SeriesMatrix, SeriesMatrixQuery } from '@/services/api/seriesDataSource';

const NOW = 5_000;

const query = vi.hoisted(() => ({
  fn: undefined as unknown as ReturnType<typeof vi.fn>,
}));

const storeMock = vi.hoisted(() => ({
  panel: { id: 'p1', type: 'stat', title: '패널', config: {} } as PanelConfig,
  updatePanelConfig: vi.fn(),
  updatePanelTitle: vi.fn(),
}));

vi.mock('@/stores/uiStore', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/stores/uiStore')>();
  const state = () => ({
    activeDashboardId: 'd',
    dashboardPages: [
      { id: 'd', name: 'x', isDefault: true, panels: [storeMock.panel], layout: [] },
    ],
    updatePanelConfig: storeMock.updatePanelConfig,
    updatePanelTitle: storeMock.updatePanelTitle,
    dashboardRefreshInterval: 5,
    dashboardEditMode: false,
  });
  return { ...actual, useUIStore: (selector: (s: unknown) => unknown) => selector(state()) };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

// 훅은 실제 구현을 쓰고 조회 주입점만 채운다 — 미리보기가 **실제 데이터 경로**를 타는지가
// 이 파일의 요점이므로 훅을 통째로 모킹하면 검증이 성립하지 않는다.
vi.mock('./panels/charts/useStoreChartData', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./panels/charts/useStoreChartData')>();
  return {
    ...actual,
    useStoreChartData: (
      config: Parameters<typeof actual.useStoreChartData>[0],
      enabled: boolean,
    ) =>
      actual.useStoreChartData(config, enabled, {
        queryMatrixFn: (agentName: string, q: SeriesMatrixQuery, signal?: AbortSignal) =>
          query.fn(agentName, q, signal),
        nowFn: () => NOW,
      }),
  };
});

vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [{ id: 'a1', name: 'store-1', type: 'store' }] } }),
  useAgent: () => ({ data: undefined }),
}));

vi.mock('@/hooks/useFlow', () => ({ useFlows: () => ({ data: { data: [] } }) }));

vi.mock('@/services/api/store', () => ({
  useStoreKeysWithTags: () => ({ data: { keyObjects: [] }, isLoading: false, isError: false }),
}));

// 게이지 레거시 store 폴링(useStoreLatestValue)의 POST 를 가로챈다.
vi.mock('@/services/api/client', () => ({
  post: async () => ({ entries: [] as Array<{ value: unknown; timestamp: number }> }),
}));

// sysmetrics 소스는 에이전트 스냅샷을 폴링한다. 조회 컨텍스트 없이 렌더하려고 갈아 끼운다.
vi.mock('./panels/sysmetrics/useSysMetricsSnapshot', () => ({
  useSysMetricsSnapshot: () => ({
    snapshot: {
      status: 'running',
      collectedAt: 1_000,
      intervalSeconds: 5,
      cpu: { usage_percent: 42 },
      network: { en0: { bytes_recv: 10 }, en1: { bytes_recv: 20 } },
      targets: { mountpoints: [], devices: [], interfaces: ['en0', 'en1'] },
    },
    previous: null,
    state: 'ready' as const,
  }),
}));

vi.mock('./panels/charts/useChartChannel', () => ({
  useChartChannel: () => ({
    entries: [],
    status: 'idle' as const,
    closedReason: undefined,
    errorReason: undefined,
  }),
}));

import PanelSettingsDialog from './PanelSettingsDialog';

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

function f1Store(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    agent_id: 'a1',
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

async function renderDialog(type: PanelConfig['type'], config: Record<string, unknown>) {
  storeMock.panel = { id: 'p1', type, title: '패널', config };
  await act(async () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
  });
  // 미리보기는 디바운스된 draft(previewPanel)로 렌더되고 store 조회는 비동기다.
  await act(async () => {
    await Promise.resolve();
  });
}

beforeEach(() => {
  storeMock.updatePanelConfig.mockReset();
  storeMock.updatePanelTitle.mockReset();
  query.fn = vi.fn(async () => F1_MATRIX);
  window.localStorage.clear();
});

describe('stat 라이브 미리보기 (M6.3)', () => {
  it('Store + 대표값 지정이면 실제 StatPanel 이 draft config 로 렌더된다', async () => {
    await renderDialog('stat', {
      channel_name: 'c1',
      decimal_places: 0,
      data_source: 'store',
      store_source: f1Store(),
      series_reduce: 'max',
    });

    const wrapper = screen.getByTestId('stat-preview-wrapper');
    const preview = within(wrapper);
    // 합성 샘플이 아니라 실제 조회 결과다 — F1 의 max 는 26 / 23 / 값 없음.
    expect(preview.getByText('26')).toBeInTheDocument();
    expect(preview.getByText('23')).toBeInTheDocument();
    expect(preview.getAllByTestId('series-tile')).toHaveLength(3);
    expect(query.fn).toHaveBeenCalled();
  });

  it('대표값을 지정하지 않아도(레거시 경로) 미리보기를 렌더한다', async () => {
    // stat 에는 합성 미니 프리뷰가 없다 — 렌더하지 않으면 미리보기 영역이 빈 화면이 된다.
    // StatPanel 은 대표값 없이도 시리즈 소스 데이터를 그리므로 미리보기도 같은 조건이다.
    await renderDialog('stat', {
      channel_name: 'c1',
      data_source: 'store',
      store_source: f1Store(),
    });
    expect(screen.getByTestId('stat-preview-wrapper')).toBeInTheDocument();
  });

  it('시리즈 미선택이면 실패널의 빈 상태를 렌더한다(빈 화면이 아니다)', async () => {
    // 신규 통계 패널은 store 기본 소스 + 시리즈 0개로 태어난다. 이때 미리보기를 끄면
    // 사용자는 소스를 고르기도 전에 빈 화면을 본다. 소스가 비활성이면 조회는 idle 이므로
    // (`usePanelSeriesData`) 빈 시리즈로 요청이 나가지도 않는다.
    await renderDialog('stat', {
      channel_name: 'c1',
      data_source: 'store',
      store_source: f1Store({ series: [] }),
      series_reduce: 'max',
    });
    expect(screen.getByTestId('stat-preview-wrapper')).toBeInTheDocument();
    expect(query.fn).not.toHaveBeenCalled();
  });
});

describe('gauge 라이브 미리보기 (M6.3)', () => {
  /** 게이지 미리보기 래퍼 안의 합성 샘플 안내 여부로 두 경로를 구분한다. */
  function isMiniPreview(): boolean {
    const wrapper = screen.getByTestId('gauge-preview-wrapper');
    return (
      within(wrapper).queryByText(
        'dashboard.settings.gaugeSection.previewSample',
      ) !== null
    );
  }

  it('store-source 경로가 이기면 합성 샘플 대신 실제 GaugePanel 이 렌더된다', async () => {
    await renderDialog('gauge', {
      gaugeType: 'simple',
      min: 0,
      max: 100,
      unit: '%',
      value: 7,
      data_source: 'store',
      store_source: f1Store(),
      series_reduce: 'last',
    });

    expect(isMiniPreview()).toBe(false);
    const preview = within(screen.getByTestId('gauge-preview-wrapper'));
    // F1 의 last 는 21 / 23 / 값 없음. static config.value(7)는 쓰이지 않는다.
    // 게이지 숫자는 값 표기 자릿수(기본 2)를 따른다.
    expect(preview.getByText('21.00')).toBeInTheDocument();
    expect(preview.getByText('23.00')).toBeInTheDocument();
    expect(preview.getByText('--')).toBeInTheDocument();
    expect(preview.queryByText('7.00')).toBeNull();
  });

  it('레거시 경로가 이기는 동안에는 기존 합성 샘플 미리보기가 유지된다', async () => {
    // 대표값 미지정 → §2.9 게이지 추가 조건에 따라 레거시가 이긴다.
    await renderDialog('gauge', {
      gaugeType: 'simple',
      min: 0,
      max: 100,
      unit: '%',
      value: 7,
      data_source: 'store',
      store_source: f1Store(),
    });
    expect(isMiniPreview()).toBe(true);
  });

  it('레거시 바인딩만 있는 기존 게이지도 합성 샘플 미리보기를 그대로 쓴다', async () => {
    await renderDialog('gauge', {
      gaugeType: 'simple',
      min: 0,
      max: 100,
      unit: '%',
      value: 7,
      dataSources: [{ sourceType: 'chart-emitter', channelName: 'ch1' }],
    });
    expect(isMiniPreview()).toBe(true);
  });

  it('시리즈가 0개면 신규 경로가 값을 낼 수 없으므로 합성 샘플로 남는다', async () => {
    await renderDialog('gauge', {
      gaugeType: 'simple',
      min: 0,
      max: 100,
      unit: '%',
      value: 7,
      data_source: 'store',
      store_source: f1Store({ series: [] }),
      series_reduce: 'last',
    });
    expect(isMiniPreview()).toBe(true);
  });
});


describe('sysmetrics 소스 라이브 미리보기', () => {
  /** 보고된 상태: 라인 차트 + sysmetrics 소스 + 인터페이스 둘. */
  function sysmetricsSource(over: Record<string, unknown> = {}) {
    return {
      agent_id: 'a1',
      agent_name: 'host-1',
      series: [{ key: 'network.bytes_recv' }],
      interfaces: ['en0', 'en1'],
      ...over,
    };
  }

  it('라인 차트가 합성 미니 프리뷰가 아니라 실제 패널을 그린다', async () => {
    // 종전에는 게이트가 소스 종류를 `store | tsdb` 로 **열거**해 sysmetrics 가 빠졌고,
    // 미리보기 영역이 합성 미니 프리뷰에 머물러 "출력 안됨" 으로 보였다.
    await renderDialog('graph-chart', {
      data_source: 'sysmetrics',
      sysmetrics_source: sysmetricsSource(),
    });

    const wrapper = within(screen.getByTestId('line-chart-preview-wrapper'));
    expect(wrapper.getByTestId('line-chart-container')).toBeInTheDocument();
  });

  it('소스가 비활성이면(값 미선택) 종전대로 합성 미니 프리뷰가 남는다', async () => {
    await renderDialog('graph-chart', {
      data_source: 'sysmetrics',
      sysmetrics_source: sysmetricsSource({ series: [] }),
    });

    const wrapper = within(screen.getByTestId('line-chart-preview-wrapper'));
    expect(wrapper.queryByTestId('line-chart-container')).toBeNull();
  });

  it('실제 데이터 적용 토글이 노출된다', async () => {
    // 게이트가 종류를 열거하던 때는 sysmetrics 에서 이 토글도 함께 사라졌다.
    await renderDialog('graph-chart', {
      data_source: 'sysmetrics',
      sysmetrics_source: sysmetricsSource(),
    });

    expect(screen.getByTestId('preview-real-data-toggle')).toBeInTheDocument();
  });
});
