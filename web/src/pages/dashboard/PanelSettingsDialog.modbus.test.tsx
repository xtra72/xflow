// PanelSettingsDialog MODBUS Gateway 설정 테스트 (SPEC-MODBUS-012, BUG A).
//
// modbus 패널 편집 시 (1) 게이트웨이 에이전트 재선택기(modbus-gateway 만 필터)가 노출되고,
// (2) 에이전트를 바꾸면 draft config.agentId 가 갱신되어 적용 시 저장되며,
// (3) modbus-device-registers 는 대상 unit 선택기가 추가로 노출되는지 검증한다.
// 프리뷰는 실제 패널 컴포넌트를 렌더하므로 useModbusData 의 폴링 훅을 정적 값으로 대체한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';

const storeMock = vi.hoisted(() => ({
  panel: { id: 'p1', type: 'modbus-real-devices', title: 'MODBUS', config: {} } as PanelConfig,
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
  });
  return { ...actual, useUIStore: (selector: (s: unknown) => unknown) => selector(state()) };
});

vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({
    data: {
      data: [
        { id: 'gw-1', name: '게이트웨이 A', type: 'modbus-gateway', status: 'running' },
        { id: 'gw-2', name: '게이트웨이 B', type: 'modbus-gateway', status: 'running' },
        { id: 'cli-1', name: '클라이언트', type: 'modbus-client', status: 'running' },
      ],
    },
  }),
}));

// 폴링 훅(react-query 의존)을 정적 값으로 대체 — QueryClientProvider 없이 프리뷰 렌더.
// useModbusGate / formatUnitLabel / 헬퍼는 실제 구현을 유지한다.
vi.mock('./panels/modbus/useModbusData', async () => {
  const actual = await vi.importActual<typeof import('./panels/modbus/useModbusData')>(
    './panels/modbus/useModbusData',
  );
  const dev = (unit_id: number, name: string) => ({
    unit_id,
    name,
    register_counts: { coils: 0, discrete_inputs: 0, holding_registers: 0, input_registers: 0 },
    status: 'active',
    backed: false,
    mode: '',
    stats: { read_count: 0, write_count: 0, error_count: 0 },
  });
  return {
    ...actual,
    useModbusListDevices: () => ({
      devices: [dev(1, '유닛-1'), dev(2, '유닛-2')],
      isLoading: false,
      isError: false,
    }),
    useModbusDeviceStatus: () => ({ status: undefined, isLoading: false, isError: false }),
    useModbusRegisterMap: () => ({ registerMap: undefined, isLoading: false, isError: false }),
    useModbusStatus: () => ({ status: undefined, isLoading: false, isError: false }),
    useModbusListClients: () => ({ clients: [], isLoading: false, isError: false }),
  };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import PanelSettingsDialog from './PanelSettingsDialog';

beforeEach(() => {
  storeMock.updatePanelConfig.mockReset();
  storeMock.updatePanelTitle.mockReset();
  storeMock.panel = { id: 'p1', type: 'modbus-real-devices', title: 'MODBUS', config: { agentId: 'gw-1' } };
});

describe('PanelSettingsDialog MODBUS 설정 (BUG A)', () => {
  it('modbus 패널 편집 시 게이트웨이 에이전트 선택기(modbus-gateway 만)를 노출한다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    const agentSel = screen.getByTestId('modbus-settings-agent-select') as HTMLSelectElement;
    expect(agentSel).toBeInTheDocument();
    // placeholder + modbus-gateway 2개 = 3 (modbus-client 제외).
    expect(agentSel.options.length).toBe(3);
    expect(agentSel.value).toBe('gw-1');
    // real-devices 는 unit 선택기가 없다.
    expect(screen.queryByTestId('modbus-settings-unit-select')).toBeNull();
  });

  it('에이전트를 변경하면 draft config.agentId 가 갱신되어 적용 시 저장된다', () => {
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    const agentSel = screen.getByTestId('modbus-settings-agent-select') as HTMLSelectElement;
    fireEvent.change(agentSel, { target: { value: 'gw-2' } });
    // 컨트롤드 셀렉트가 새 agentId 로 재렌더된다(draft 반영).
    expect((screen.getByTestId('modbus-settings-agent-select') as HTMLSelectElement).value).toBe('gw-2');

    fireEvent.click(screen.getByRole('button', { name: 'dashboard.settings.apply' }));
    expect(storeMock.updatePanelConfig).toHaveBeenCalledTimes(1);
    const savedConfig = storeMock.updatePanelConfig.mock.calls[0]![1] as Record<string, unknown>;
    expect(savedConfig.agentId).toBe('gw-2');
  });

  it('modbus-device-registers 는 대상 unit 선택기를 추가로 노출한다', () => {
    storeMock.panel = {
      id: 'p1',
      type: 'modbus-device-registers',
      title: 'MODBUS',
      config: { agentId: 'gw-1', unitId: 2 },
    };
    render(<PanelSettingsDialog panelId="p1" onClose={() => {}} />);

    const unitSel = screen.getByTestId('modbus-settings-unit-select') as HTMLSelectElement;
    expect(unitSel).toBeInTheDocument();
    expect(unitSel.value).toBe('2');
  });
});
