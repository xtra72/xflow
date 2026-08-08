// PanelSettingsDialog — 3분할 셸(3-split shell) 구성 테스트.
//
// @spec SPEC-PANEL-SETTINGS-001 (T1 / REQ-01 / AC-01)
//
// 검증: Store 바인딩 패널(heatmap / line-chart) 설정 셸이 미리보기·옵션·데이터소스
// 3영역을 모두 렌더하고, 기존 편집 슬롯(패널 옵션/데이터소스)이 각 영역에 손실 없이
// 배치된다(셸 골격 교체 + 슬롯 이관, 편집 로직 보존). 배치 기본값:
//   좌측 컬럼 상단 = 미리보기 / 좌측 컬럼 하단 = 데이터소스 / 우측 = 옵션.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, within } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';

const storeMock = vi.hoisted(() => ({
  panel: { id: 'p1', type: 'heatmap', title: '패널', config: {} } as PanelConfig,
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
  window.localStorage.clear();
});

describe('PanelSettingsDialog 3분할 셸 (AC-01)', () => {
  it('heatmap 패널: 미리보기·옵션·데이터소스 3영역이 모두 존재한다', () => {
    storeMock.panel = { id: 'p1', type: 'heatmap', title: '히트맵', config: {} };
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    expect(screen.getByTestId('panel-settings-preview')).toBeInTheDocument();
    expect(screen.getByTestId('panel-settings-options')).toBeInTheDocument();
    expect(screen.getByTestId('panel-settings-data-source')).toBeInTheDocument();
  });

  it('line-chart 패널: 3영역이 모두 존재하고 편집 슬롯이 각 영역에 배치된다', () => {
    storeMock.panel = { id: 'p1', type: 'line-chart', title: '라인', config: {} };
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    const preview = screen.getByTestId('panel-settings-preview');
    const options = screen.getByTestId('panel-settings-options');
    const dataSource = screen.getByTestId('panel-settings-data-source');
    expect(preview).toBeInTheDocument();
    expect(options).toBeInTheDocument();
    expect(dataSource).toBeInTheDocument();

    // 옵션 영역에 기존 "패널 옵션" 편집 슬롯이 이관되어 있다(편집 기능 손실 없음).
    expect(within(options).getByText('dashboard.settings.panelOptions')).toBeInTheDocument();
    // 데이터소스 영역은 좌측 컬럼(미리보기 영역) 안에 위치한다(좌측 상단 미리보기 / 하단 데이터소스).
    expect(preview).toContainElement(dataSource);
  });

  it('기본 배치: 미리보기·데이터소스는 좌측 컬럼(order-1), 옵션은 우측(order-3)', () => {
    storeMock.panel = { id: 'p1', type: 'line-chart', title: '라인', config: {} };
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    // 좌측 컬럼(미리보기+데이터소스)은 order-1, 우측 옵션 컬럼은 order-3 클래스를 갖는다.
    expect(screen.getByTestId('panel-settings-preview').className).toContain('order-1');
    expect(screen.getByTestId('panel-settings-options').className).toContain('order-3');
  });
});
