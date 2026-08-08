// PanelSettingsDialog — heatmap 색상 프리셋(T8) 테스트.
//
// @spec SPEC-PANEL-SETTINGS-001 (AC-11/AC-12)
//   - AC-11: heatmap 옵션에 프리셋 선택 + 커스텀 편집 제공, 선택 시 draft color_table 반영.
//   - AC-12: 차트 5종(bar-chart)에서는 프리셋/커스텀 UI 미노출.

import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { PanelConfig } from '@/stores/uiStore';
import { HEATMAP_COLOR_PRESETS } from './panels/heatmap/heatmapColorPresets';

const storeMock = vi.hoisted(() => ({
  panel: { id: 'p1', type: 'heatmap', title: 'h', config: {} } as PanelConfig,
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
  useAgents: () => ({ data: { data: [] } }),
  useExecAgent: () => ({ isPending: false, mutate: vi.fn() }),
}));
vi.mock('@/services/api/store', () => ({
  useStoreKeysWithTags: () => ({ data: { keyObjects: [] }, isLoading: false, isError: false }),
  useStoreTagPairs: () => ({ data: [], isLoading: false, isError: false }),
}));

import PanelSettingsDialog from './PanelSettingsDialog';

beforeEach(() => {
  storeMock.updatePanelConfig.mockReset();
  storeMock.updatePanelTitle.mockReset();
  window.localStorage.clear();
});

describe('T8/AC-11 — heatmap 색상 프리셋', () => {
  beforeEach(() => {
    storeMock.panel = { id: 'p1', type: 'heatmap', title: 'h', config: {} };
  });

  it('프리셋 선택 UI + 커스텀 ColorStop 편집이 제공된다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.getByTestId('heatmap-color-presets')).toBeInTheDocument();
    // 5종 프리셋 버튼.
    for (const p of HEATMAP_COLOR_PRESETS) {
      expect(screen.getByTestId(`heatmap-preset-${p.id}`)).toBeInTheDocument();
    }
    // 커스텀 편집(정지점 추가) 버튼도 존재.
    expect(screen.getByTestId('heatmap-add-colorstop')).toBeInTheDocument();
  });

  it('프리셋(Viridis) 선택 → 저장 시 config.color_table 이 프리셋 stops 로 반영된다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    fireEvent.click(screen.getByTestId('heatmap-preset-viridis'));
    fireEvent.click(screen.getByText('dashboard.settings.apply'));

    const viridis = HEATMAP_COLOR_PRESETS.find((p) => p.id === 'viridis')!;
    expect(storeMock.updatePanelConfig).toHaveBeenCalledTimes(1);
    const [, cfg] = storeMock.updatePanelConfig.mock.calls[0]!;
    expect((cfg as Record<string, unknown>).color_table).toEqual(viridis.stops);
  });
});

describe('T8/AC-12 — heatmap 외 미노출', () => {
  it('bar-chart 패널에서는 프리셋/커스텀 UI 가 노출되지 않는다', () => {
    storeMock.panel = { id: 'p1', type: 'bar-chart', title: 'b', config: {} };
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.queryByTestId('heatmap-color-presets')).not.toBeInTheDocument();
    expect(screen.queryByTestId('heatmap-add-colorstop')).not.toBeInTheDocument();
  });
});
