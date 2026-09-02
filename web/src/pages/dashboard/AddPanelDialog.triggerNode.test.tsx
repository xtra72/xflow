// SPEC-TRIGGER-PANEL-001 §B — AddPanelDialog trigger 노드 피커.
// B-1: running trigger 노드 목록이 피커에 렌더된다.
// B-2: 노드 선택 시 { flowId, nodeId } 가 패널 config 로 저장된다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

// trigger 노드 인스턴스 열거 훅 mock (running flow 2개).
const nodeInstances = vi.hoisted(() => ({
  instances: [
    { flowId: 'flow-a', flowName: '플로우 A', nodeId: 'trig-1', nodeName: '트리거1', processed: 0, errors: 0 },
    { flowId: 'flow-b', flowName: '플로우 B', nodeId: 'trig-2', nodeName: '트리거2', processed: 0, errors: 0 },
  ],
  isLoading: false,
}));

vi.mock('@/hooks/useNodeTypeInstances', () => ({
  useNodeTypeInstances: () => nodeInstances,
}));

// 다른 스텝 훅은 이 테스트에서 렌더되지 않으므로 최소 mock 만 둔다.
vi.mock('@/hooks/useDevice', () => ({ useDevices: () => ({ data: { data: [] }, isLoading: false }) }));
vi.mock('@/hooks/useAgent', () => ({ useAgents: () => ({ data: { data: [] } }) }));
vi.mock('@/hooks/useStation', () => ({
  useStations: () => ({ data: [], isLoading: false }),
  useXsfmDevices: () => ({ data: [], isLoading: false }),
}));
vi.mock('@/hooks/useGroups', () => ({ useGroups: () => ({ data: [], isLoading: false }) }));
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

describe('AddPanelDialog — trigger 노드 피커 (SPEC-TRIGGER-PANEL-001)', () => {
  beforeEach(() => {
    storeState.addPanelCalls = [];
    storeState.addPanelWithConfigCalls = [];
  });

  function openTriggerStep() {
    render(<AddPanelDialog open={true} onClose={() => {}} />);
    // 데이터 카테고리에 trigger 옵션이 위치한다.
    fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.etc' }));
    fireEvent.click(screen.getByText('dashboard.panelTypes.triggerConfig'));
  }

  it('B-1: 기타 카테고리에 trigger 설정 옵션이 노출된다', () => {
    render(<AddPanelDialog open={true} onClose={() => {}} />);
    fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.etc' }));
    expect(screen.getByText('dashboard.panelTypes.triggerConfig')).toBeInTheDocument();
  });

  it('B-1: 옵션 선택 시 즉시 추가되지 않고 노드 피커로 진입하며 running 노드 목록을 렌더한다', () => {
    openTriggerStep();

    // 노드 피커 헤더 + 두 노드/플로우 쌍
    expect(screen.getByText('dashboard.addPanel.selectTriggerNode')).toBeInTheDocument();
    expect(screen.getByText('트리거1')).toBeInTheDocument();
    expect(screen.getByText('플로우 A')).toBeInTheDocument();
    expect(screen.getByText('트리거2')).toBeInTheDocument();
    expect(screen.getByText('플로우 B')).toBeInTheDocument();
    // 즉시 추가 없음
    expect(storeState.addPanelCalls).toEqual([]);
    expect(storeState.addPanelWithConfigCalls).toEqual([]);
  });

  it('B-2: 노드 선택 시 addPanelWithConfig({flowId,nodeId}) + 기본 타이틀(노드명) + onClose', () => {
    const onClose = vi.fn();
    render(<AddPanelDialog open={true} onClose={onClose} />);
    fireEvent.click(screen.getByRole('button', { name: 'dashboard.panelCategories.etc' }));
    fireEvent.click(screen.getByText('dashboard.panelTypes.triggerConfig'));

    fireEvent.click(screen.getByTestId('trigger-node-option-flow-b-trig-2'));

    expect(storeState.addPanelWithConfigCalls).toEqual([
      { type: 'trigger-config', config: { flowId: 'flow-b', nodeId: 'trig-2' }, title: '트리거2' },
    ]);
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
