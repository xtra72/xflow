// PanelSettingsDialog — store 라인 차트 미리보기가 실제 데이터 패널을 쓰는지 검증.
//
// 보고된 요구: 시리즈를 선택하면 합성 사인파가 아니라 실제 조회 데이터를 미리보기에 그린다.
// 선택 전(또는 채널 모드)에는 스타일 확인용 합성 미니 프리뷰를 유지한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';

const storeMock = vi.hoisted(() => ({
  panel: { id: 'p1', type: 'line-chart', title: '라인', config: {} } as PanelConfig,
  updatePanelConfig: vi.fn(),
  updatePanelTitle: vi.fn(),
}));

vi.mock('@/stores/uiStore', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/stores/uiStore')>();
  const state = () => ({
    activeDashboardId: 'd',
    dashboardPages: [{ id: 'd', name: 'x', isDefault: true, panels: [storeMock.panel], layout: [] }],
    updatePanelConfig: storeMock.updatePanelConfig,
    updatePanelTitle: storeMock.updatePanelTitle,
    dashboardRefreshInterval: 5,
    dashboardEditMode: false,
  });
  return { ...actual, useUIStore: (selector: (s: unknown) => unknown) => selector(state()) };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

// 실제 패널이 쓰는 store 조회 훅 — 빈 결과로 고정한다(렌더 경로만 검증).
vi.mock('./panels/charts/useStoreChartData', () => ({
  useStoreChartData: () => ({
    entries: [],
    seriesEntries: new Map(),
    seriesStyles: new Map(),
    seriesNames: [],
    booleanSeries: new Set(),
    status: 'idle',
  }),
}));

vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [{ id: 'store-uuid-1', name: 'store-1', type: 'store' }] } }),
  useAgent: () => ({ data: undefined }),
}));
vi.mock('@/services/api/store', () => ({
  useStoreKeysWithTags: () => ({ data: { keyObjects: [] }, isLoading: false, isError: false }),
}));

import PanelSettingsDialog from './PanelSettingsDialog';

/** store 라인 차트 패널. series 를 비우면 "미선택" 상태다. */
function storeLinePanel(series: unknown[]): PanelConfig {
  return {
    id: 'p1',
    type: 'line-chart',
    title: '라인',
    config: {
      data_source: 'store',
      store_source: { agent_id: 'store-uuid-1', agent_name: 'store-1', series },
    },
  } as unknown as PanelConfig;
}

beforeEach(() => {
  storeMock.updatePanelConfig.mockReset();
  storeMock.updatePanelTitle.mockReset();
  window.localStorage.clear();
});

describe('store 라인 차트 미리보기 — 실제 데이터 패널 사용', () => {
  it('시리즈를 선택하면 실제 라인 차트 패널을 렌더한다(합성 미니 프리뷰 아님)', () => {
    storeMock.panel = storeLinePanel([{ key: 'LAI', field: 'value' }]);
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    const wrapper = screen.getByTestId('line-chart-preview-wrapper');
    // 미니 프리뷰의 표식(합성 데이터 라벨)이 없어야 한다.
    expect(wrapper.textContent).not.toContain('dashboard.settings.preview.label');
  });

  it('시리즈 미선택이면 합성 미니 프리뷰를 유지한다(스타일 확인용)', () => {
    storeMock.panel = storeLinePanel([]);
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    const wrapper = screen.getByTestId('line-chart-preview-wrapper');
    expect(wrapper.textContent).toContain('dashboard.settings.preview.label');
  });

  it('채널 모드는 시리즈 유무와 무관하게 합성 미니 프리뷰를 쓴다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'line-chart',
      title: '라인',
      config: { data_source: 'channel', channel_name: 'ch-a' },
    } as unknown as PanelConfig;
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    const wrapper = screen.getByTestId('line-chart-preview-wrapper');
    expect(wrapper.textContent).toContain('dashboard.settings.preview.label');
  });

  it('태그 자동 바인딩(시리즈 배열 비어있음)도 실제 패널을 쓴다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'line-chart',
      title: '라인',
      config: {
        data_source: 'store',
        store_source: {
          agent_id: 'store-uuid-1',
          agent_name: 'store-1',
          series: [],
          selection_mode: 'tag',
          tag_filters: { room: '1' },
        },
      },
    } as unknown as PanelConfig;
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    const wrapper = screen.getByTestId('line-chart-preview-wrapper');
    expect(wrapper.textContent).not.toContain('dashboard.settings.preview.label');
  });
});

// ---------------------------------------------------------------------------
// SPEC-TSDB-002 M2 — 특성화 테스트 (DDD PRESERVE).
//
// `PanelSettingsDialog.tsx:714`(라인 미리보기 게이트)와 `:728`(stat 미리보기 게이트)는
// 서로 다른 config 참조를 쓴다 — 전자는 `previewRenderPanel.config?.` 로 옵셔널
// 체이닝하고, 후자는 `previewChartConfig = previewRenderPanel.config ?? {}` 를 거친다.
// 또한 판정에 쓰는 부가 조건도 다르다(라인은 시리즈/태그, stat 은 시리즈 + series_reduce).
//
// 두 게이트를 하나로 뭉개면 "설정 화면에서는 보이는데 대시보드에서는 안 보인다" 가
// 되므로, **독립 판정**임을 잠근다.
//
// @spec SPEC-TSDB-002 §2.3 (U3) · §2.4 (U4) — plan.md §3.4 CT-13 ~ CT-15
// ---------------------------------------------------------------------------
describe('미리보기 게이트 2곳의 독립 판정 특성화 (SPEC-TSDB-002 M2, CT-13~CT-15)', () => {
  /** 시리즈 1개를 가진 store 블록(두 게이트가 공유하는 동일 config 조각). */
  function activeStore(): Record<string, unknown> {
    return {
      agent_id: 'store-uuid-1',
      agent_name: 'store-1',
      selection_mode: 'keys',
      series: [{ key: 'LAI', field: 'value' }],
    };
  }

  function renderPanel(type: string, config: Record<string, unknown>) {
    storeMock.panel = { id: 'p1', type, title: '패널', config } as unknown as PanelConfig;
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
  }

  /** 라인 게이트(`:714`)가 참인가 = 합성 미니 프리뷰가 아니라 실제 패널을 그렸는가. */
  function lineGateActive(): boolean {
    const wrapper = screen.queryByTestId('line-chart-preview-wrapper');
    if (!wrapper) return false;
    return !wrapper.textContent?.includes('dashboard.settings.preview.label');
  }

  /** stat 게이트(`:728`)가 참인가 = stat 미리보기 래퍼가 렌더됐는가. */
  function statGateActive(): boolean {
    return screen.queryByTestId('stat-preview-wrapper') !== null;
  }

  it("CT-13: line-chart + previewRenderPanel.config.data_source === 'store' + 시리즈 N → `:714` 게이트 참", () => {
    renderPanel('line-chart', { data_source: 'store', store_source: activeStore() });
    expect(lineGateActive()).toBe(true);
    // 다른 게이트는 패널 타입이 달라 거짓이다.
    expect(statGateActive()).toBe(false);
  });

  it("CT-14: stat + previewChartConfig.data_source === 'store' + 시리즈 N → stat 게이트 참", () => {
    renderPanel('stat', {
      data_source: 'store',
      store_source: activeStore(),
      series_reduce: 'last',
    });
    expect(statGateActive()).toBe(true);
    // 라인 래퍼는 `panel.type === 'line-chart'` 일 때만 렌더된다.
    expect(screen.queryByTestId('line-chart-preview-wrapper')).toBeNull();
  });

  it('CT-15: series_reduce 는 더 이상 stat 게이트의 축이 아니다', () => {
    // 종전에는 `series_reduce` 부재가 stat 게이트를 거짓으로 만들었다. 그러면 대표값을
    // 지정하지 않은 store 통계 패널이 **미리보기 영역 자체가 빈 화면**이 되는데, stat 에는
    // 대신 보여줄 합성 미니 프리뷰가 없어 사용자에게는 "미리보기가 안 나온다" 로 보인다.
    // 실제 StatPanel 은 대표값 없이도 시리즈 소스 데이터를 그리므로, 미리보기도 같은
    // 조건으로 판정한다(미리보기와 실제 렌더가 갈리지 않게).
    renderPanel('stat', { data_source: 'store', store_source: activeStore() });
    expect(statGateActive()).toBe(true);
  });

  it('CT-15: 채널 모드에서는 stat 게이트가 거짓이다(부가 조건은 소스 종류 축 하나)', () => {
    renderPanel('stat', { data_source: 'channel', store_source: activeStore() });
    expect(statGateActive()).toBe(false);
  });

  it('CT-15: 같은 config 조각이 line-chart 에서는 `:714` 참을 낸다(부가 조건이 다르다)', () => {
    // 위 (a) 와 완전히 같은 data_source/store_source/series_reduce 조합이지만
    // 라인 게이트는 `series_reduce` 를 보지 않으므로 참이다.
    renderPanel('line-chart', { data_source: 'store', store_source: activeStore() });
    expect(lineGateActive()).toBe(true);
  });

  it('CT-15: config 자체가 없으면 두 게이트 모두 거짓이며 예외를 던지지 않는다', () => {
    // `:714` 는 `config?.` 옵셔널 체이닝, `:728` 은 `config ?? {}` 로 서로 다른 방식으로
    // 방어한다. 결과는 같아야 한다.
    expect(() => renderPanel('line-chart', undefined as never)).not.toThrow();
    expect(lineGateActive()).toBe(false);
    expect(statGateActive()).toBe(false);
  });
});

// ===== 바/파이 미리보기 =====

describe('PanelSettingsDialog — 바/파이 라이브 미리보기', () => {
  function activeStore() {
    return {
      agent_name: 'store-1',
      series: [{ key: 'room:temp' }],
      time_window_ms: 60_000,
      interval_ms: 10_000,
      aggregation: 'average',
    };
  }
  function renderPanel(type: string, config: Record<string, unknown>) {
    storeMock.panel = { id: 'p1', type, title: '패널', config } as unknown as PanelConfig;
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
  }

  for (const [type, testid] of [
    ['bar-chart', 'bar-chart-preview-wrapper'],
    ['pie-chart', 'pie-chart-preview-wrapper'],
  ] as const) {
    it(`${type}: store 소스를 고르면 실패널을 렌더한다`, () => {
      renderPanel(type, { data_source: 'store', store_source: activeStore() });
      expect(screen.getByTestId(testid)).toBeInTheDocument();
    });

    it(`${type}: 시리즈 미선택(신규 패널 기본값)도 실패널의 빈 상태를 렌더한다`, () => {
      // 이 두 패널에는 합성 미니 프리뷰가 없다 — 렌더하지 않으면 빈 화면이 된다.
      renderPanel(type, {
        data_source: 'store',
        store_source: { ...activeStore(), series: [] },
      });
      expect(screen.getByTestId(testid)).toBeInTheDocument();
    });

    it(`${type}: tsdb 소스도 같은 자격이다`, () => {
      renderPanel(type, {
        data_source: 'tsdb',
        tsdb_source: {
          backend: 'influxdb',
          agent_name: 'influx-1',
          bucket: 'metrics',
          series: [{ key: 'room1', field: 'temp' }],
          time_window_ms: 60_000,
          interval_ms: 10_000,
          aggregation: 'average',
        },
      });
      expect(screen.getByTestId(testid)).toBeInTheDocument();
    });

    it(`${type}: 채널 모드에서는 렌더하지 않는다(종전 동작)`, () => {
      renderPanel(type, { data_source: 'channel', channel_name: 'c1' });
      expect(screen.queryByTestId(testid)).toBeNull();
    });

    it(`${type}: data_source 미지정(구 패널)도 렌더하지 않는다`, () => {
      renderPanel(type, { channel_name: 'c1' });
      expect(screen.queryByTestId(testid)).toBeNull();
    });
  }
});

// ===== 실제 데이터 적용 옵션 (SPEC-TSDB-004) =====

describe('PanelSettingsDialog — 실제 데이터 적용 옵션', () => {
  function activeStore() {
    return {
      agent_name: 'store-1',
      series: [{ key: 'room:temp' }],
      time_window_ms: 60_000,
      interval_ms: 10_000,
      aggregation: 'average',
    };
  }
  function renderPanel(type: string, config: Record<string, unknown>) {
    storeMock.panel = { id: 'p1', type, title: '패널', config } as unknown as PanelConfig;
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
  }

  /** 실제 패널을 그렸는가(= 합성 미니 프리뷰가 아닌가). 위 describe 의 판정과 같다. */
  function realRenderActive(): boolean {
    const wrapper = screen.queryByTestId('line-chart-preview-wrapper');
    if (!wrapper) return false;
    return !wrapper.textContent?.includes('dashboard.settings.preview.label');
  }

  it('토글이 **데이터 소스 설정** 안에 있다 (미리보기 영역이 아니다)', () => {
    renderPanel('line-chart', { data_source: 'store', store_source: activeStore() });
    const toggle = screen.getByTestId('preview-real-data-toggle');
    const dataSource = screen.getByTestId('panel-settings-data-source');
    // 조회를 낼지 말지를 정하는 옵션이므로 소스 설정에 속한다.
    expect(dataSource.contains(toggle)).toBe(true);

    const previewWrapper = screen.queryByTestId('line-chart-preview-wrapper');
    expect(previewWrapper?.contains(toggle) ?? false).toBe(false);
  });

  it('기본은 켜짐이며 끄면 실제 렌더 대신 합성 미리보기로 내려간다', () => {
    renderPanel('line-chart', { data_source: 'store', store_source: activeStore() });
    const toggle = screen.getByTestId('preview-real-data-toggle')
      .querySelector('input') as HTMLInputElement;
    // 기본 켜짐 — Store 의 종전 동작을 그대로 둔다.
    expect(toggle.checked).toBe(true);
    expect(realRenderActive()).toBe(true);

    fireEvent.click(toggle);
    expect(realRenderActive()).toBe(false);
  });

  it('소스가 비활성이면 토글을 노출하지 않는다', () => {
    // 채널 모드에는 조회 옵션이 성립하지 않는다.
    renderPanel('line-chart', { channel_name: 'ch1' });
    expect(screen.queryByTestId('preview-real-data-toggle')).toBeNull();
  });
});
