// SPEC-DASHBOARD-002 (REQ-01, AC-01-2/AC-01-3): AddPanelDialog agent-status 스텝.
//
// - AC-01-2: data 카테고리에 agent-status 옵션이 라벨/설명과 함께 노출된다.
// - AC-01-3: 옵션 선택 시 전체 타입(필터 없음) 에이전트 선택 스텝으로 진입하고,
//   하나를 선택해 완료하면 config.agentId 를 저장한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

// 서로 다른 타입 3종(modbus-gateway, xsfm, mqtt-client) — 타입 필터가 없어야 전부 노출된다.
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({
    data: {
      data: [
        { id: 'gw-1', name: '게이트웨이', type: 'modbus-gateway', status: 'running' },
        { id: 'xs-1', name: '설비', type: 'xsfm', status: 'running' },
        { id: 'mq-1', name: 'MQTT', type: 'mqtt-client', status: 'running' },
      ],
    },
  }),
}));

// 무관 스텝 훅 최소 mock.
vi.mock('@/pages/dashboard/panels/modbus/useModbusData', () => ({
  useModbusListDevices: () => ({ devices: [], isLoading: false, isError: false }),
  formatUnitLabel: (u: number) => `U${u}`,
}));
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

function openData() {
  render(<AddPanelDialog open={true} onClose={() => {}} />);
  fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.data' }));
}

describe('AddPanelDialog — 에이전트 상태 패널 (SPEC-DASHBOARD-002)', () => {
  beforeEach(() => {
    storeState.addPanelCalls = [];
    storeState.addPanelWithConfigCalls = [];
  });

  it('AC-01-2: data 카테고리에 agent-status 옵션이 노출된다', () => {
    openData();
    expect(screen.getByText('dashboard.panelTypes.agentStatus')).toBeInTheDocument();
  });

  it('AC-01-3: 옵션 선택 시 즉시 추가되지 않고 에이전트 선택 스텝으로 진입한다', () => {
    openData();
    fireEvent.click(screen.getByText('dashboard.panelTypes.agentStatus'));

    expect(screen.getByTestId('agent-status-select')).toBeInTheDocument();
    expect(screen.getByText('dashboard.addPanel.selectAgentStatus')).toBeInTheDocument();
    expect(storeState.addPanelCalls).toEqual([]);
    expect(storeState.addPanelWithConfigCalls).toEqual([]);
  });

  it('AC-01-3: 에이전트 셀렉트는 타입 필터 없이 전체(3종)를 노출한다', () => {
    openData();
    fireEvent.click(screen.getByText('dashboard.panelTypes.agentStatus'));

    const sel = screen.getByTestId('agent-status-select') as HTMLSelectElement;
    // placeholder + 3종 = 4
    expect(sel.options.length).toBe(4);
    expect(screen.getByRole('option', { name: '게이트웨이 (modbus-gateway)' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: '설비 (xsfm)' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'MQTT (mqtt-client)' })).toBeInTheDocument();
  });

  it('AC-01-3: 에이전트 선택 후 저장 시 { agentId } config + 이름 타이틀로 추가한다', () => {
    const onClose = vi.fn();
    render(<AddPanelDialog open={true} onClose={onClose} />);
    fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.data' }));
    fireEvent.click(screen.getByText('dashboard.panelTypes.agentStatus'));

    const save = screen.getByTestId('agent-status-save') as HTMLButtonElement;
    // 미선택 시 저장 비활성.
    expect(save.disabled).toBe(true);

    fireEvent.change(screen.getByTestId('agent-status-select'), { target: { value: 'xs-1' } });
    expect(save.disabled).toBe(false);
    fireEvent.click(save);

    expect(storeState.addPanelWithConfigCalls).toEqual([
      { type: 'agent-status', config: { agentId: 'xs-1' }, title: '설비' },
    ]);
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
