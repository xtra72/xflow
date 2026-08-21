// PanelSettingsDialog — store 라인 차트 미리보기가 실제 데이터 패널을 쓰는지 검증.
//
// 보고된 요구: 시리즈를 선택하면 합성 사인파가 아니라 실제 조회 데이터를 미리보기에 그린다.
// 선택 전(또는 채널 모드)에는 스타일 확인용 합성 미니 프리뷰를 유지한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';

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
