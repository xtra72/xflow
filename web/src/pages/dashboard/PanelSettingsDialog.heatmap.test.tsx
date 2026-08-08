// PanelSettingsDialog 히트맵 설정 화면 테스트.
//
// 두 가지 회귀 방지를 검증한다:
//   (a) 히트맵 패널 설정 시 미리보기 영역에 실제 HeatmapPanel 이 렌더된다(데이터 없으면 빈상태 안내).
//   (b) 데이터 소스 섹션(panel-settings-data-source)이 차트 패널과 동일하게 프리뷰 아래에 노출된다.
//
// store 폴링(useStoreChartData)과 StoreSourceSection 의 네트워크 훅을 정적 값으로 대체해
// QueryClientProvider 없이 렌더한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';

const storeMock = vi.hoisted(() => ({
  panel: { id: 'p1', type: 'heatmap', title: '히트맵', config: {} } as PanelConfig,
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
    // HeatmapPanel 이 배치편집 토글 게이팅에 참조한다(false = 뷰어).
    dashboardEditMode: false,
  });
  return { ...actual, useUIStore: (selector: (s: unknown) => unknown) => selector(state()) };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

// store 폴링 훅(react-query 의존) — HeatmapPanel + HeatmapSettingsSection 공용. idle 정적 값으로 대체.
vi.mock('./panels/charts/useStoreChartData', () => ({
  useStoreChartData: () => ({
    entries: [],
    seriesEntries: new Map(),
    seriesStyles: [],
    seriesNames: [],
    booleanSeries: [],
    status: 'idle',
  }),
}));

// StoreSourceSection 의 네트워크 훅(에이전트/키 목록) — 정적 값으로 대체.
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [{ id: 'store-uuid-1', name: 'store-1', type: 'store' }] } }),
}));
vi.mock('@/services/api/store', () => ({
  useStoreKeysWithTags: () => ({ data: { keyObjects: [] }, isLoading: false, isError: false }),
}));

import PanelSettingsDialog from './PanelSettingsDialog';

beforeEach(() => {
  storeMock.updatePanelConfig.mockReset();
  storeMock.updatePanelTitle.mockReset();
  storeMock.panel = { id: 'p1', type: 'heatmap', title: '히트맵', config: {} };
});

describe('PanelSettingsDialog 히트맵 설정 화면', () => {
  it('미리보기 영역에 HeatmapPanel 을 렌더한다(데이터 없으면 빈상태 안내)', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    // HeatmapPanel 의 빈상태 안내가 노출되면 실제 패널이 프리뷰에 렌더된 것이다.
    expect(screen.getByText('dashboard.heatmap.emptyState')).toBeInTheDocument();
  });

  it('데이터 소스 섹션을 프리뷰 아래에 노출한다(차트 패널과 동일)', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    expect(screen.getByTestId('panel-settings-data-source')).toBeInTheDocument();
  });
});
