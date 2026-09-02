// PanelSettingsDialog 설비 그룹 설정 테스트 (SPEC-XSFM-GROUP-001 M7).
//
// facility-group 패널의 설정에서 (1) 대상 그룹 선택기(역사·라인·커스텀 나열, groupId 로 저장),
// (2) 표시 옵션(showStats / offlineAsOff / deviceLabelMode)이 역사 패널과 동형으로 노출되고,
// 적용 시 config 에 반영되는지 검증한다. 프리뷰/차트 경로는 facility 패널에서 렌더되지 않는다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';

// 설정 다이얼로그가 참조하는 스토어 상태(활성 페이지 + 대상 패널 + 콜백).
const storeMock = vi.hoisted(() => ({
  panel: { id: 'p1', type: 'facility-group', title: '설비 그룹', config: {} } as PanelConfig,
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
  });
  return { ...actual, useUIStore: (selector: (s: unknown) => unknown) => selector(state()) };
});

vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [{ id: 'air-1', name: 'xsfm 에이전트', type: 'xsfm', status: 'running' }] } }),
}));

vi.mock('@/hooks/useStation', () => ({
  useStations: () => ({ data: [{ station: 's1', line: 'L1', display_name: '1역', order: 0, places: [] }], isLoading: false }),
  useXsfmDevices: () => ({ data: [{ device_id: 'd1', name: '기기-1', station: 's1', place: 'p1', index: 0 }], isLoading: false }),
}));

vi.mock('@/hooks/useGroups', () => ({
  useGroups: () => ({
    data: [
      { id: 'station:s1', name: '1역', type: 'station', member_count: 3, members: ['d1', 'd2', 'd3'] },
      { id: 'custom:g1', name: '커스텀그룹', type: 'custom', member_count: 2, members: ['d1', 'd2'] },
    ],
    isLoading: false,
  }),
}));

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import PanelSettingsDialog from './PanelSettingsDialog';

beforeEach(() => {
  storeMock.updatePanelConfig.mockReset();
  storeMock.updatePanelTitle.mockReset();
  storeMock.panel = {
    id: 'p1',
    type: 'facility-group',
    title: '설비 그룹',
    config: { agentId: 'air-1', groupId: 'custom:g1', showStats: true, deviceLabelMode: 'placeIndex', offlineAsOff: false },
  };
});

describe('PanelSettingsDialog facility-group 설정', () => {
  it('대상 그룹 선택기(역사·커스텀 나열)와 표시 옵션(통계/오프라인/라벨)을 노출한다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    // 대상 셀렉트: placeholder + station + custom = 3.
    const target = screen.getByTestId('facility-target-select') as HTMLSelectElement;
    expect(target.options.length).toBe(3);
    // 현재 config.groupId 가 선택되어 있다.
    expect(target.value).toBe('custom:g1');
    // 그룹 전용 표시 옵션(역사 패널 동형, testid 접두사 facility-group-*).
    expect(screen.getByTestId('facility-group-show-stats')).toBeInTheDocument();
    expect(screen.getByTestId('facility-group-offline-as-off')).toBeInTheDocument();
    expect(screen.getByTestId('facility-group-device-label-mode')).toBeInTheDocument();
  });

  it('그룹을 변경하면 draft config.groupId 가 갱신되고, 적용 시 updatePanelConfig 로 저장된다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    const target = screen.getByTestId('facility-target-select') as HTMLSelectElement;
    fireEvent.change(target, { target: { value: 'station:s1' } });
    // 컨트롤드 셀렉트가 새 groupId 로 재렌더된다(draft 반영).
    expect((screen.getByTestId('facility-target-select') as HTMLSelectElement).value).toBe('station:s1');

    // 적용 → updatePanelConfig 가 groupId=station:s1 을 포함한 config 로 호출된다.
    fireEvent.click(screen.getByRole('button', { name: 'dashboard.settings.apply' }));
    expect(storeMock.updatePanelConfig).toHaveBeenCalledTimes(1);
    const savedConfig = storeMock.updatePanelConfig.mock.calls[0]![1] as Record<string, unknown>;
    expect(savedConfig.groupId).toBe('station:s1');
  });

  it('표시 옵션(showStats) 토글이 draft 에 반영되어 적용 시 저장된다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);
    fireEvent.click(screen.getByTestId('facility-group-show-stats')); // true → false
    fireEvent.click(screen.getByRole('button', { name: 'dashboard.settings.apply' }));
    const savedConfig = storeMock.updatePanelConfig.mock.calls[0]![1] as Record<string, unknown>;
    expect(savedConfig.showStats).toBe(false);
  });
});
