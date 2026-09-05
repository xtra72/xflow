// PanelSettingsDialog — stat 패널 스타일 섹션.
//
// @spec SPEC-CHART-003 AC-01 / AC-02 / AC-03 / AC-04
//
// M1-1.4 에서 "stat 이 악센트 그룹 고르기를 받고 있다" 를 특성화로 잠근 뒤,
// M5 축소로 아래 서술이 그 자리를 대신했다. 죽은 그룹 3개(header/badges/table)와
// 고르기-편집 2단계는 사라지고, 살아 있는 panelColor 만 한 줄로 남는다(spec.md §5 D1).

import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { PanelConfig } from '@/stores/uiStore';

const storeMock = vi.hoisted(() => ({
  panel: { id: 'p1', type: 'stat', title: 's', config: {} } as PanelConfig,
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
  useAgent: () => ({ data: undefined }),
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
  storeMock.panel = { id: 'p1', type: 'stat', title: 's', config: {} };
});

describe('SPEC-CHART-003 AC-01 — stat 패널은 악센트 그룹 고르기를 내지 않는다', () => {
  it('accent-group-picker 가 존재하지 않는다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.queryByTestId('accent-group-picker')).toBeNull();
  });

  it('죽은 그룹(header · badges · table)의 편집 입구가 사라졌다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    for (const group of ['header', 'badges', 'table', '_base']) {
      expect(screen.queryByTestId(`accent-group-${group}`)).toBeNull();
    }
  });
});

describe('SPEC-CHART-003 AC-02 / AC-28 — 패널 색상은 한 줄로 남아 편집된다', () => {
  it('스와치를 누르면 panelColor 가 그 색으로 저장된다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.getByTestId('panel-color-row')).toBeInTheDocument();

    // 고르기 단계 없이 곧바로 색을 바꾼다(U1-3).
    fireEvent.click(screen.getByTestId('panel-color-#8b5cf6'));
    fireEvent.click(screen.getByText('dashboard.settings.apply'));

    const [, cfg] = storeMock.updatePanelConfig.mock.calls[0]!;
    expect((cfg as Record<string, unknown>).panelColor).toBe('#8b5cf6');
  });

  it('저장된 색이 있으면 초기화로 지울 수 있다', () => {
    storeMock.panel = { id: 'p1', type: 'stat', title: 's', config: { panelColor: '#ef4444' } };
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    fireEvent.click(screen.getByTestId('panel-color-reset'));
    fireEvent.click(screen.getByText('dashboard.settings.apply'));

    const [, cfg] = storeMock.updatePanelConfig.mock.calls[0]!;
    expect((cfg as Record<string, unknown>).panelColor).toBeUndefined();
  });

  it('색이 지정되지 않았으면 초기화 버튼을 내지 않는다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.queryByTestId('panel-color-reset')).toBeNull();
  });
});

describe('SPEC-CHART-003 AC-03 — 저장된 accentElements 는 손대지 않는다', () => {
  it('패널 색상을 바꿔도 accentElements 값이 유지된다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'stat',
      title: 's',
      config: { accentElements: { header: '#ff0000' } },
    };
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    fireEvent.click(screen.getByTestId('panel-color-#06b6d4'));
    fireEvent.click(screen.getByText('dashboard.settings.apply'));

    const [, cfg] = storeMock.updatePanelConfig.mock.calls[0]!;
    // 편집 입구만 사라졌을 뿐 값은 그대로다 — 다른 패널 타입으로 바꾸면 되살아나야 한다.
    expect((cfg as Record<string, unknown>).accentElements).toEqual({ header: '#ff0000' });
  });
});

describe('SPEC-CHART-003 AC-04 — stat 외 패널의 스타일 섹션은 유지된다', () => {
  it('graph-chart 패널은 악센트 그룹 고르기를 계속 받는다', () => {
    storeMock.panel = { id: 'p1', type: 'graph-chart', title: 'g', config: {} };
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.getByTestId('accent-group-picker')).toBeInTheDocument();
    expect(screen.queryByTestId('panel-color-row')).toBeNull();
  });

  it('bar-chart 패널도 그대로다', () => {
    storeMock.panel = { id: 'p1', type: 'bar-chart', title: 'b', config: {} };
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.getByTestId('accent-group-picker')).toBeInTheDocument();
  });
});
