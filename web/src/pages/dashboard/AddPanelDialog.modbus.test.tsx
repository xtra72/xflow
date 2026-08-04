// SPEC-MODBUS-012 M1 (REQ-01, AC-01/02): AddPanelDialog MODBUS Gateway 스텝.
//
// - AC-01: data 카테고리에 6종 옵션이 라벨/설명과 함께 노출된다.
// - AC-02: 옵션 선택 시 에이전트 선택 스텝(modbus-gateway 필터)으로 진입하고,
//   완료 시 config.agentId 를 저장한다. 가상 디바이스 레지스터 맵은 unit 2차 선택 후 unitId 도 저장한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

// modbus-gateway 2개, modbus-client 1개(필터 검증용) + 무관 에이전트.
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

// 가상 디바이스 레지스터 맵의 unit 2차 선택 목록(list_devices 결과).
vi.mock('@/pages/dashboard/panels/modbus/useModbusData', () => ({
  useModbusListDevices: (agentId: string, enabled: boolean) => ({
    devices:
      enabled && agentId
        ? [
            { unit_id: 1, name: '펌프', register_counts: {}, status: 'active', backed: false, mode: '', stats: {} },
            { unit_id: 2, name: '밸브', register_counts: {}, status: 'active', backed: false, mode: '', stats: {} },
          ]
        : [],
    isLoading: false,
    isError: false,
  }),
  formatUnitLabel: (u: number) => `U${String(u).padStart(2, '0')}`,
}));

// 무관 스텝 훅 최소 mock.
vi.mock('@/hooks/useDevice', () => ({ useDevices: () => ({ data: { data: [] }, isLoading: false }) }));
vi.mock('@/hooks/useStation', () => ({
  useStations: () => ({ data: [], isLoading: false }),
  useXsfmDevices: () => ({ data: [], isLoading: false }),
}));
vi.mock('@/hooks/useGroups', () => ({ useGroups: () => ({ data: [], isLoading: false }) }));
vi.mock('@/hooks/useNodeTypeInstances', () => ({ useNodeTypeInstances: () => ({ instances: [], isLoading: false }) }));
vi.mock('@/services/api/charts', () => ({ listChartChannels: vi.fn().mockResolvedValue([]) }));

const storeState = vi.hoisted(() => ({
  addPanelCalls: [] as Array<{ type: string }>,
  addPanelWithConfigCalls: [] as Array<{ type: string; config: Record<string, unknown>; title?: string }>,
}));

vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector: (s: unknown) => unknown) =>
    selector({
      addPanel: (type: string) => storeState.addPanelCalls.push({ type }),
      addPanelWithConfig: (type: string, config: Record<string, unknown>, title?: string) =>
        storeState.addPanelWithConfigCalls.push({ type, config, title }),
    }),
}));

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import AddPanelDialog from './AddPanelDialog';

const MODBUS_TYPES = [
  'modbusRealDevices',
  'modbusVirtualDevices',
  'modbusSharedRegisters',
  'modbusDeviceRegisters',
  'modbusBusStats',
  'modbusSummaryStats',
] as const;

function openData() {
  render(<AddPanelDialog open={true} onClose={() => {}} />);
  fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.data' }));
}

describe('AddPanelDialog — MODBUS Gateway 패널 (SPEC-MODBUS-012)', () => {
  beforeEach(() => {
    storeState.addPanelCalls = [];
    storeState.addPanelWithConfigCalls = [];
  });

  it('AC-01: data 카테고리에 6종 MODBUS Gateway 옵션이 노출된다', () => {
    openData();
    for (const key of MODBUS_TYPES) {
      expect(screen.getByText(`dashboard.panelTypes.${key}`)).toBeInTheDocument();
    }
  });

  it('AC-02: 옵션 선택 시 즉시 추가되지 않고 에이전트 스텝으로 진입한다', () => {
    openData();
    fireEvent.click(screen.getByText('dashboard.panelTypes.modbusVirtualDevices'));

    expect(screen.getByTestId('modbus-agent-select')).toBeInTheDocument();
    expect(screen.getByText('dashboard.addPanel.selectModbusGateway')).toBeInTheDocument();
    expect(storeState.addPanelCalls).toEqual([]);
    expect(storeState.addPanelWithConfigCalls).toEqual([]);
  });

  it('AC-02: 에이전트 셀렉트는 modbus-gateway 만 노출한다(modbus-client 제외)', () => {
    openData();
    fireEvent.click(screen.getByText('dashboard.panelTypes.modbusRealDevices'));

    const sel = screen.getByTestId('modbus-agent-select') as HTMLSelectElement;
    // placeholder + gateway 2개 = 3
    expect(sel.options.length).toBe(3);
    expect(screen.getByRole('option', { name: '게이트웨이 A' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: '게이트웨이 B' })).toBeInTheDocument();
    expect(screen.queryByRole('option', { name: '클라이언트' })).toBeNull();
  });

  it('AC-02: 비-레지스터 패널은 unit 스텝 없이 {agentId} 저장 + onClose', () => {
    const onClose = vi.fn();
    render(<AddPanelDialog open={true} onClose={onClose} />);
    fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.data' }));
    fireEvent.click(screen.getByText('dashboard.panelTypes.modbusBusStats'));

    // unit 셀렉트는 없어야 한다.
    expect(screen.queryByTestId('modbus-unit-select')).toBeNull();

    fireEvent.change(screen.getByTestId('modbus-agent-select'), { target: { value: 'gw-2' } });
    const save = screen.getByTestId('modbus-save') as HTMLButtonElement;
    expect(save.disabled).toBe(false);
    fireEvent.click(save);

    expect(storeState.addPanelWithConfigCalls).toEqual([
      { type: 'modbus-bus-stats', config: { agentId: 'gw-2' }, title: '게이트웨이 B' },
    ]);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('AC-02: 가상 레지스터 맵은 에이전트→unit 2차 선택 후 {agentId, unitId} 저장', () => {
    openData();
    fireEvent.click(screen.getByText('dashboard.panelTypes.modbusDeviceRegisters'));

    // 에이전트 미선택 상태에서는 저장 비활성 + unit 셀렉트 비활성.
    const save = screen.getByTestId('modbus-save') as HTMLButtonElement;
    expect(save.disabled).toBe(true);

    fireEvent.change(screen.getByTestId('modbus-agent-select'), { target: { value: 'gw-1' } });
    const unitSel = screen.getByTestId('modbus-unit-select') as HTMLSelectElement;
    // placeholder + 디바이스 2개
    expect(unitSel.options.length).toBe(3);
    // 에이전트만 선택된 상태에서는 아직 저장 비활성(unit 필수).
    expect(save.disabled).toBe(true);

    fireEvent.change(unitSel, { target: { value: '2' } });
    expect(save.disabled).toBe(false);
    fireEvent.click(save);

    expect(storeState.addPanelWithConfigCalls).toEqual([
      { type: 'modbus-device-registers', config: { agentId: 'gw-1', unitId: 2 }, title: '게이트웨이 A' },
    ]);
  });
});
