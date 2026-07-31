// SPEC-TRIGGER-SCHED-001 M2 / AC-13 — AddPanelDialog 설비 제어 예약 옵션 + 대상 선택 스텝.
// 옵션 노출(공존) / 노드 피커 진입 / 에이전트(선택) + 노드 선택 시 config 저장.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

const nodeInstances = vi.hoisted(() => ({
  instances: [
    { flowId: 'flow-a', flowName: '플로우 A', nodeId: 'trig-1', nodeName: '트리거1', processed: 0, errors: 0 },
  ],
  isLoading: false,
}));
vi.mock('@/hooks/useNodeTypeInstances', () => ({ useNodeTypeInstances: () => nodeInstances }));

vi.mock('@/hooks/useDevice', () => ({ useDevices: () => ({ data: { data: [] }, isLoading: false }) }));
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [{ id: 'ag-1', name: 'xsfm-1', type: 'xsfm' }, { id: 'ag-2', name: 'other', type: 'mqtt' }] } }),
}));
vi.mock('@/hooks/useStation', () => ({
  useStations: () => ({ data: [], isLoading: false }),
  useXsfmDevices: () => ({ data: [], isLoading: false }),
}));
vi.mock('@/hooks/useGroups', () => ({ useGroups: () => ({ data: [], isLoading: false }) }));
vi.mock('@/services/api/charts', () => ({ listChartChannels: vi.fn().mockResolvedValue([]) }));

const storeState = vi.hoisted(() => ({
  addPanelWithConfigCalls: [] as Array<{ type: string; config: Record<string, unknown>; title?: string }>,
}));
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector: (s: unknown) => unknown) =>
    selector({
      addPanel: () => {},
      addPanelWithConfig: (type: string, config: Record<string, unknown>, title?: string) =>
        storeState.addPanelWithConfigCalls.push({ type, config, title }),
    }),
}));
vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import AddPanelDialog from './AddPanelDialog';

beforeEach(() => {
  storeState.addPanelWithConfigCalls = [];
});

describe('AddPanelDialog — 설비 제어 예약 (SPEC-TRIGGER-SCHED-001)', () => {
  function openStep() {
    render(<AddPanelDialog open={true} onClose={() => {}} />);
    // 제어 카테고리에 설비 제어 예약 옵션이 위치한다.
    fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.control' }));
    fireEvent.click(screen.getByText('dashboard.panelTypes.facilitySchedule'));
  }

  it('제어 카테고리에 설비 제어 예약 옵션이 노출된다(공존)', () => {
    render(<AddPanelDialog open={true} onClose={() => {}} />);
    fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.control' }));
    expect(screen.getByText('dashboard.panelTypes.facilitySchedule')).toBeInTheDocument();
  });

  it('옵션 선택 시 노드 피커 스텝으로 진입한다(즉시 추가 없음)', () => {
    openStep();
    expect(screen.getByText('dashboard.addPanel.selectFacilitySchedule')).toBeInTheDocument();
    expect(screen.getByText('트리거1')).toBeInTheDocument();
    expect(storeState.addPanelWithConfigCalls).toEqual([]);
  });

  it('에이전트 선택 + 노드 선택 시 { flowId, nodeId, agentId } config 로 추가한다', () => {
    const onClose = vi.fn();
    render(<AddPanelDialog open={true} onClose={onClose} />);
    fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.control' }));
    fireEvent.click(screen.getByText('dashboard.panelTypes.facilitySchedule'));

    // xsfm 에이전트만 옵션에 노출(mqtt 제외).
    const agentSel = screen.getByTestId('facility-schedule-agent-select') as HTMLSelectElement;
    const agentValues = Array.from(agentSel.options).map((o) => o.value);
    expect(agentValues).toContain('ag-1');
    expect(agentValues).not.toContain('ag-2');

    fireEvent.change(agentSel, { target: { value: 'ag-1' } });
    fireEvent.click(screen.getByTestId('facility-schedule-node-flow-a-trig-1'));

    expect(storeState.addPanelWithConfigCalls).toEqual([
      { type: 'facility-schedule', config: { flowId: 'flow-a', nodeId: 'trig-1', agentId: 'ag-1' }, title: '트리거1' },
    ]);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('에이전트 미지정으로도 노드 선택 시 agentId="" 로 추가한다(자유 입력 폴백)', () => {
    openStep();
    fireEvent.click(screen.getByTestId('facility-schedule-node-flow-a-trig-1'));
    expect(storeState.addPanelWithConfigCalls[0]!.config).toEqual({
      flowId: 'flow-a',
      nodeId: 'trig-1',
      agentId: '',
    });
  });
});
