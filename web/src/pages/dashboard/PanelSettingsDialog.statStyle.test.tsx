// PanelSettingsDialog — stat 패널 스타일 섹션.
//
// @spec SPEC-CHART-003 AC-01 / AC-02 / AC-03 / AC-04
//
// M1-1.4 에서 "stat 이 악센트 그룹 고르기를 받고 있다" 를 특성화로 잠근 뒤,
// M5 축소로 아래 서술이 그 자리를 대신했다. 죽은 그룹 3개(header/badges/table)와
// 고르기-편집 2단계는 사라지고, 살아 있던 panelColor 는 **패널 옵션**으로 옮겨
// 모든 패널이 공유한다(spec.md §5 D1). 그 결과 stat 의 스타일 섹션은 비어 사라졌다.

import { QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { inertQueryClient } from '@/hooks/inertQueryClient';
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
// 조회 훅만 비활성으로 덮고 나머지 export 는 원본을 유지한다 — 패널 타입별로
// 어떤 훅을 타는지가 달라서, 목록을 손으로 나열하면 타입을 하나 더할 때마다 깨진다.
vi.mock('@/hooks/useAgent', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/hooks/useAgent')>()),
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

describe('AC-02 보강 — 패널 색상은 패널 옵션에 있고 모든 타입이 공유한다', () => {
  // 일부 패널 타입은 React Query 훅을 탄다 — 조회 없이 렌더만 되면 되므로
  // 비활성 클라이언트를 씌운다.
  const renderTyped = (type: PanelConfig['type']) => {
    storeMock.panel = { id: 'p1', type, title: 't', config: {} };
    return render(
      <QueryClientProvider client={inertQueryClient()}>
        <PanelSettingsDialog panelId="p1" onClose={() => {}} />
      </QueryClientProvider>,
    );
  };

  it('_base 그룹은 어느 패널 타입에서도 더 이상 나오지 않는다', () => {
    for (const type of ['logs', 'gauge', 'resource', 'graph-chart'] as const) {
      const view = renderTyped(type);
      expect(screen.queryByTestId('accent-group-_base')).toBeNull();
      // 대신 패널 옵션의 패널 색상이 모든 타입에 있다.
      expect(screen.getByTestId('panel-color-row')).toBeInTheDocument();
      view.unmount();
    }
  });

  it('스타일 섹션이 없던 목록형 패널도 이제 패널 색상을 지정할 수 있다', () => {
    // flows · agents · devices · properties-grid · agent-status 는 종전에 편집 입구가
    // 아예 없어 저장된 색을 바꿀 방법이 없었다.
    for (const type of ['flows', 'agents', 'devices', 'agent-status'] as const) {
      const view = renderTyped(type);
      expect(screen.getByTestId('panel-color-row')).toBeInTheDocument();
      view.unmount();
    }
  });
});

describe('SPEC-CHART-003 AC-02 / AC-28 — 패널 색상은 한 줄로 편집된다', () => {
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
  });

  it('bar-chart 패널도 그대로다', () => {
    storeMock.panel = { id: 'p1', type: 'bar-chart', title: 'b', config: {} };
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.getByTestId('accent-group-picker')).toBeInTheDocument();
  });
});
