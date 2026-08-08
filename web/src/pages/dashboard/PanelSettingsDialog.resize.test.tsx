// PanelSettingsDialog — 2경계 리사이즈/비율 영속(T2/T3) + draft/committed 누수 방지(T9).
//
// @spec SPEC-PANEL-SETTINGS-001 (AC-02/AC-03/AC-14)
//
// store 폴링/StoreSourceSection 네트워크 훅을 정적 값으로 대체해 QueryClient 없이 렌더한다
// (heatmap.test.tsx 와 동일 패턴).

import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { PanelConfig } from '@/stores/uiStore';

const storeMock = vi.hoisted(() => ({
  panel: { id: 'p1', type: 'heatmap', title: 'orig', config: {} } as PanelConfig,
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
  storeMock.panel = { id: 'p1', type: 'heatmap', title: 'orig', config: {} };
  window.localStorage.clear();
});

describe('T2 — 2경계 스플리터', () => {
  it('좌우 + 상하 스플리터가 모두 렌더된다(미리보기+데이터소스 공존 시)', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.getByTestId('panel-settings-splitter')).toBeInTheDocument();
    expect(screen.getByTestId('panel-settings-preview-splitter')).toBeInTheDocument();
  });
});

describe('T3 — 비율 영속/복원 (AC-03)', () => {
  it('저장된 비율로 옵션 컬럼 폭을 복원한다', () => {
    window.localStorage.setItem(
      'panel-settings-ratio:p1',
      JSON.stringify({ optionsWidth: 500, previewRatio: 0.3 }),
    );
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    expect(screen.getByTestId('panel-settings-options').style.width).toBe('500px');
  });

  it('손상된 비율은 기본값으로 폴백하고 예외를 던지지 않는다(edge)', () => {
    window.localStorage.setItem('panel-settings-ratio:p1', '{corrupt');
    expect(() =>
      render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />),
    ).not.toThrow();
    // 기본 옵션 폭 360px.
    expect(screen.getByTestId('panel-settings-options').style.width).toBe('360px');
  });

  it('세로 스플리터 드래그가 비율을 패널별 키에 영속한다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    fireEvent.mouseDown(screen.getByTestId('panel-settings-splitter'));
    // jsdom getBoundingClientRect=0 → optionsWidth = right(0) - clientX(600) = -600 → 클램프(240).
    fireEvent.mouseMove(window, { clientX: 600 });
    fireEvent.mouseUp(window);
    const raw = window.localStorage.getItem('panel-settings-ratio:p1');
    expect(raw).not.toBeNull();
    expect(JSON.parse(raw!).optionsWidth).toBe(240);
  });
});

describe('T9 — draft/committed 누수 방지 (AC-14)', () => {
  it('편집 후 닫기(취소)는 committed 를 변경하지 않는다', () => {
    const onClose = vi.fn();
    render(<PanelSettingsDialog panelId="p1" onClose={onClose} />);
    const titleInput = screen.getByDisplayValue('orig');
    fireEvent.change(titleInput, { target: { value: 'edited' } });
    fireEvent.blur(titleInput);
    // 취소(Cancel) 버튼 = onClose.
    fireEvent.click(screen.getByText('common.cancel'));
    expect(onClose).toHaveBeenCalledTimes(1);
    // committed 미변경(누수 없음).
    expect(storeMock.updatePanelTitle).not.toHaveBeenCalled();
    expect(storeMock.updatePanelConfig).not.toHaveBeenCalled();
  });

  it('적용 시 draft 가 committed 로 승격된다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    const titleInput = screen.getByDisplayValue('orig');
    fireEvent.change(titleInput, { target: { value: 'edited' } });
    fireEvent.blur(titleInput);
    fireEvent.click(screen.getByText('dashboard.settings.apply'));
    expect(storeMock.updatePanelTitle).toHaveBeenCalledWith('p1', 'edited');
    expect(storeMock.updatePanelConfig).toHaveBeenCalledTimes(1);
  });
});
