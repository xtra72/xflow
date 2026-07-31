// SPEC-SCHEDULE-VIEW-001 — ScheduleManagementTab (단일 플랫 표).
// 교차-플로우 스케줄을 하나의 플랫 표에 노드별 <tbody> 로 렌더 + 에이전트 이름 컬럼(AC-7) /
// 미지정 행("미지정", AC-8) / 교차-플로우 집계 ≥2 플로우(AC-18) / 부분 실패 통지(AC-18) /
// "규칙 추가"가 작성 카드 없이 설정 팝업을 직접 열고 팝업 안 대상 노드+에이전트 선택으로
// agent_id 를 각인 / CRUD 경로가 dual-write(configureNode+updateFlow)를 호출.
//
// useScheduleAggregation 훅만 스텁하고 resolveScheduleAgent 는 실제를 사용해 에이전트
// 해석 로직을 함께 검증한다. FacilityRuleModal 은 노드/에이전트 셀렉터 + 고정 draft 저장
// 스텁으로 대체하고, dual-write 하위 서비스는 목으로 관측한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';

import type { ScheduleNodeEntry, UseScheduleAggregation } from './scheduleAggregation';

const agg = vi.hoisted(() => ({
  value: {
    nodeEntries: [] as ScheduleNodeEntry[],
    isLoading: false,
    failedCount: 0,
    totalFlows: 0,
    hasError: false,
    refetch: vi.fn(),
  } as UseScheduleAggregation,
}));

const state = vi.hoisted(() => ({
  configCalls: [] as Array<{ config: Record<string, unknown> }>,
  notifications: [] as Array<{ type: string; message: string }>,
}));
const configureNodeMock = vi.hoisted(() => vi.fn());
const getFlowMock = vi.hoisted(() => vi.fn());
const updateFlowMock = vi.hoisted(() => vi.fn());

// scheduleAggregation: 훅만 스텁, 순수 함수(groupEntriesByAgent 등)는 실제 사용.
vi.mock('./scheduleAggregation', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./scheduleAggregation')>();
  return { ...actual, useScheduleAggregation: () => agg.value };
});

// FacilityRuleModal 스텁: 대상 노드/에이전트 셀렉터 노출 + 저장 시 고정 유효 draft 로 onSave.
// 실제 모달의 노드→에이전트 프리필은 여기서 재현하지 않으므로(실제 모달 테스트가 담당),
// 탭 배선(선택 노드 + 실효 에이전트 → dual-write)을 검증하려면 두 셀렉터를 명시 선택한다.
const modalDraft = vi.hoisted(() => ({
  value: {
    name: '신규 규칙',
    validFrom: '',
    validTo: '',
    priority: 5,
    enabled: true,
    schedule: { type: 'interval', value: '10s' },
    target: { kind: 'all', value: '2' },
    action: { power: true, fanSpeed: null },
  } as unknown,
}));
vi.mock('@/pages/dashboard/panels/facilitySchedule/FacilityRuleModal', () => ({
  default: ({
    onSave,
    onCancel,
    agents,
    selectedAgentId,
    onAgentChange,
    nodes,
    selectedNodeKey,
    onNodeChange,
  }: {
    onSave: (d: unknown) => void;
    onCancel: () => void;
    agents?: { id: string; name: string }[];
    selectedAgentId?: string;
    onAgentChange?: (id: string) => void;
    nodes?: { flowId: string; nodeId: string; label: string; derivedAgentIds: string[] }[];
    selectedNodeKey?: string;
    onNodeChange?: (key: string) => void;
  }) => (
    <div data-testid="modal-stub">
      {nodes && (
        <select
          data-testid="modal-stub-node"
          value={selectedNodeKey ?? ''}
          onChange={(e) => onNodeChange?.(e.target.value)}
        >
          <option value="">(노드 선택)</option>
          {nodes.map((n) => (
            <option key={`${n.flowId}:${n.nodeId}`} value={`${n.flowId}:${n.nodeId}`}>
              {n.label}
            </option>
          ))}
        </select>
      )}
      {agents && (
        <select
          data-testid="modal-stub-agent"
          value={selectedAgentId ?? ''}
          onChange={(e) => onAgentChange?.(e.target.value)}
        >
          <option value="">(선택)</option>
          {agents.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
        </select>
      )}
      <button type="button" data-testid="modal-stub-save" onClick={() => onSave(modalDraft.value)}>
        save
      </button>
      <button type="button" data-testid="modal-stub-cancel" onClick={onCancel}>
        cancel
      </button>
    </div>
  ),
}));

const agentsState = vi.hoisted(() => ({ list: [] as Array<{ id: string; name: string; type: string }> }));
vi.mock('@/hooks/useAgent', () => ({ useAgents: () => ({ data: { data: agentsState.list } }) }));
vi.mock('@/services/api/nodeService', () => ({
  configureNode: (flowId: string, nodeId: string, config: Record<string, unknown>) => {
    state.configCalls.push({ config });
    return configureNodeMock(flowId, nodeId, config);
  },
}));
vi.mock('@/services/api/flowService', () => ({
  getFlow: (id: string) => getFlowMock(id),
  updateFlow: (id: string, req: unknown) => updateFlowMock(id, req),
}));
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector: (s: unknown) => unknown) =>
    selector({ addNotification: (n: { type: string; message: string }) => state.notifications.push(n) }),
}));

import ScheduleManagementTab from './ScheduleManagementTab';

function entry(
  flowId: string,
  nodeId: string,
  schedules: Record<string, unknown>[],
  nodeConfig: Record<string, unknown> = { nodeType: 'trigger' },
  derivedAgentIds: string[] = [],
): ScheduleNodeEntry {
  return {
    flowId,
    flowName: `${flowId} 이름`,
    nodeId,
    nodeName: `${nodeId} 노드`,
    nodeConfig,
    schedules,
    derivedAgentIds,
  };
}

beforeEach(() => {
  state.configCalls = [];
  state.notifications = [];
  configureNodeMock.mockReset().mockResolvedValue(undefined);
  getFlowMock.mockReset().mockResolvedValue({ config: { nodes: [] } });
  updateFlowMock.mockReset().mockResolvedValue(undefined);
  agentsState.list = [];
  agg.value = {
    nodeEntries: [],
    isLoading: false,
    failedCount: 0,
    totalFlows: 0,
    hasError: false,
    refetch: vi.fn(),
  };
});

describe('플랫 표 렌더 — 노드별 tbody + 에이전트 이름 컬럼(AC-7/AC-8, 교차 플로우 AC-18)', () => {
  it('2개 플로우의 스케줄을 하나의 플랫 표에 노드별 tbody 로 렌더하고 각 행에 에이전트 이름을 표시한다', () => {
    agentsState.list = [{ id: 'ag-x', name: '설비X', type: 'xsfm' }];
    agg.value = {
      ...agg.value,
      totalFlows: 2,
      nodeEntries: [
        entry('flow-1', 't1', [{ agent_id: 'ag-x', type: 'interval', value: '1s', payload: { command: 'set_power', line: '2', params: { power: true } } }]),
        entry('flow-2', 't2', [{ type: 'interval', value: '1s', payload: { command: 'set_power', line: '2', params: { power: true } } }]),
      ],
    };
    render(<ScheduleManagementTab />);

    // 그룹 섹션 헤더는 없다 — 단일 플랫 표 하나만 존재(교차-플로우, 드롭 없음).
    expect(screen.getByTestId('schedule-table')).toBeInTheDocument();
    expect(screen.queryByTestId('schedule-group-ag-x')).toBeNull();
    expect(screen.queryByTestId('schedule-group-unassigned')).toBeNull();
    // 각 노드가 자신의 tbody 를 소유한다.
    expect(screen.getByTestId('schedule-node-flow-1-t1')).toBeInTheDocument();
    expect(screen.getByTestId('schedule-node-flow-2-t2')).toBeInTheDocument();
    // 에이전트 컬럼: id → 이름 매핑(있으면 이름), 미해석은 "미지정".
    expect(screen.getByTestId('schedule-agent-flow-1-t1-0')).toHaveTextContent('설비X');
    expect(screen.getByTestId('schedule-agent-flow-2-t2-0')).toHaveTextContent('미지정');
  });

  it('스케줄이 하나도 없으면 빈 상태', () => {
    render(<ScheduleManagementTab />);
    expect(screen.getByTestId('schedule-empty')).toBeInTheDocument();
    expect(screen.queryByTestId('schedule-table')).toBeNull();
  });
});

describe('부분 실패 통지(AC-18/REQ-07-05)', () => {
  it('일부 플로우 조회 실패 시 성공 그룹 유지 + 통지 배너', () => {
    agg.value = {
      ...agg.value,
      totalFlows: 3,
      failedCount: 1,
      nodeEntries: [entry('flow-1', 't1', [{ agent_id: 'ag-x', type: 'interval', value: '1s' }])],
    };
    render(<ScheduleManagementTab />);
    expect(screen.getByTestId('schedule-partial-failure')).toBeInTheDocument();
    expect(screen.getByTestId('schedule-node-flow-1-t1')).toBeInTheDocument(); // 성공 행 유지
  });
});

describe('규칙 추가 → 설정 팝업 직접 오픈(작성 카드 제거)', () => {
  it('규칙 추가 클릭 시 작성 카드 없이 팝업을 직접 열고, 팝업 안에 대상 노드 셀렉터가 있다', () => {
    agentsState.list = [{ id: 'ag-x', name: '설비X', type: 'xsfm' }];
    agg.value = {
      ...agg.value,
      totalFlows: 1,
      nodeEntries: [entry('flow-1', 't1', [{ agent_id: 'ag-x', type: 'interval', value: '1s' }])],
    };
    render(<ScheduleManagementTab />);

    // 옛 작성 카드/노드 셀렉터 진입점은 존재하지 않는다.
    expect(screen.queryByTestId('schedule-creator')).toBeNull();
    expect(screen.queryByTestId('schedule-creator-node')).toBeNull();

    fireEvent.click(screen.getByTestId('schedule-add-rule'));

    // 팝업 직접 오픈 + 대상 노드 셀렉터가 팝업 안에 있다.
    expect(screen.getByTestId('modal-stub')).toBeInTheDocument();
    expect(screen.getByTestId('modal-stub-node')).toBeInTheDocument();
  });

  it('취소 시 팝업이 닫힌다', () => {
    agg.value = {
      ...agg.value,
      totalFlows: 1,
      nodeEntries: [entry('flow-1', 't1', [{ agent_id: 'ag-x', type: 'interval', value: '1s' }])],
    };
    render(<ScheduleManagementTab />);
    fireEvent.click(screen.getByTestId('schedule-add-rule'));
    expect(screen.getByTestId('modal-stub')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('modal-stub-cancel'));
    expect(screen.queryByTestId('modal-stub')).toBeNull();
  });
});

describe('CRUD → dual-write(AC-7/AC-9, REQ-06-04)', () => {
  it('팝업에서 대상 노드+에이전트 선택·저장 시 소유 노드에 append + configureNode/updateFlow + agent_id 각인', async () => {
    agentsState.list = [{ id: 'ag-x', name: '설비X', type: 'xsfm' }];
    getFlowMock.mockResolvedValue({ config: { nodes: [{ id: 't1', data: {} }] } });
    agg.value = {
      ...agg.value,
      totalFlows: 1,
      nodeEntries: [entry('flow-1', 't1', [{ agent_id: 'ag-x', type: 'interval', value: '1s', payload: { command: 'set_power', line: '2', params: { power: true } } }])],
    };
    render(<ScheduleManagementTab />);

    // 규칙 추가 → 팝업 직접 오픈. 대상 노드 + 에이전트를 팝업 안에서 선택.
    fireEvent.click(screen.getByTestId('schedule-add-rule'));
    fireEvent.change(screen.getByTestId('modal-stub-node'), { target: { value: 'flow-1:t1' } });
    fireEvent.change(screen.getByTestId('modal-stub-agent'), { target: { value: 'ag-x' } });
    fireEvent.click(screen.getByTestId('modal-stub-save'));

    await waitFor(() => expect(updateFlowMock).toHaveBeenCalled());
    expect(configureNodeMock).toHaveBeenCalledWith('flow-1', 't1', expect.any(Object));
    const sent = state.configCalls[0]!.config.schedules as Array<Record<string, unknown>>;
    expect(sent).toHaveLength(2); // 기존 1 + 신규 1(append, origin 보존)
    const created = sent[1]!;
    expect(created.name).toBe('신규 규칙');
    expect(created.agent_id).toBe('ag-x'); // REQ-01-05 각인(선택 에이전트)
    expect(created.payload).toEqual({ command: 'set_power', line: '2', params: { power: true } });
    expect(agg.value.refetch).toHaveBeenCalled(); // 재조회 트리거
  });

  it('대상 노드 미선택 상태로 저장 시 dual-write 하지 않는다(노드 필수 가드)', () => {
    agentsState.list = [{ id: 'ag-x', name: '설비X', type: 'xsfm' }];
    agg.value = {
      ...agg.value,
      totalFlows: 1,
      nodeEntries: [entry('flow-1', 't1', [{ agent_id: 'ag-x', type: 'interval', value: '1s' }])],
    };
    render(<ScheduleManagementTab />);

    fireEvent.click(screen.getByTestId('schedule-add-rule'));
    // 노드 미선택 상태로 저장(스텁은 강제 저장) → 탭 가드가 대상 노드 미해석으로 무시.
    fireEvent.click(screen.getByTestId('modal-stub-save'));

    expect(configureNodeMock).not.toHaveBeenCalled();
    expect(updateFlowMock).not.toHaveBeenCalled();
  });

  it('규칙 삭제 시 소유 노드 스케줄에서 제거되어 빈 배열로 지속화된다(Bug 2b, 기본 스케줄 재주입 없음)', async () => {
    getFlowMock.mockResolvedValue({ config: { nodes: [{ id: 't1', data: {} }] } });
    agg.value = {
      ...agg.value,
      totalFlows: 1,
      nodeEntries: [entry('flow-1', 't1', [{ agent_id: 'ag-x', type: 'interval', value: '1s', payload: { command: 'set_power', line: '2', params: { power: true } } }])],
    };
    render(<ScheduleManagementTab />);

    fireEvent.click(screen.getByTestId('schedule-delete-flow-1-t1-0'));

    await waitFor(() => expect(updateFlowMock).toHaveBeenCalled());
    expect(configureNodeMock).toHaveBeenCalledWith('flow-1', 't1', expect.any(Object));
    const sent = state.configCalls[0]!.config.schedules as unknown[];
    expect(sent).toEqual([]); // 마지막 규칙 삭제 → 명시적 빈 배열 지속화
  });

  it('두 규칙 중 하나 삭제 시 나머지만 남는다(origin 인덱스 보존, Bug 2b)', async () => {
    getFlowMock.mockResolvedValue({ config: { nodes: [{ id: 't1', data: {} }] } });
    agg.value = {
      ...agg.value,
      totalFlows: 1,
      nodeEntries: [
        entry('flow-1', 't1', [
          { agent_id: 'ag-x', type: 'interval', value: '1s', name: 'A', payload: { command: 'set_power', line: '2', params: { power: true } } },
          { agent_id: 'ag-x', type: 'interval', value: '2s', name: 'B', payload: { command: 'set_power', line: '2', params: { power: true } } },
        ]),
      ],
    };
    render(<ScheduleManagementTab />);

    fireEvent.click(screen.getByTestId('schedule-delete-flow-1-t1-0')); // 인덱스 0(A) 삭제

    await waitFor(() => expect(updateFlowMock).toHaveBeenCalled());
    const sent = state.configCalls[0]!.config.schedules as Array<Record<string, unknown>>;
    expect(sent).toHaveLength(1);
    expect(sent[0]!.name).toBe('B');
  });

  it('STATE 토글 시 소유 노드 스케줄 enabled 반전 + dual-write', async () => {
    getFlowMock.mockResolvedValue({ config: { nodes: [{ id: 't1', data: {} }] } });
    agg.value = {
      ...agg.value,
      totalFlows: 1,
      nodeEntries: [entry('flow-1', 't1', [{ agent_id: 'ag-x', type: 'interval', value: '1s', enabled: true, payload: { command: 'set_power', line: '2', params: { power: true } } }])],
    };
    render(<ScheduleManagementTab />);

    fireEvent.click(screen.getByTestId('schedule-state-flow-1-t1-0'));

    await waitFor(() => expect(updateFlowMock).toHaveBeenCalled());
    const sent = state.configCalls[0]!.config.schedules as Array<Record<string, unknown>>;
    expect(sent[0]!.enabled).toBe(false);
  });
});

describe('빈(스케줄 0) 노드 숨김 + 규칙 추가 팝업 노드 인벤토리(Bug 2a)', () => {
  it('스케줄 0개 노드는 기본 목록에서 숨기되, 팝업 노드 셀렉터에는 노출한다', () => {
    agg.value = {
      ...agg.value,
      totalFlows: 1,
      nodeEntries: [entry('flow-1', 'empty', [], { nodeType: 'trigger', agentId: 'ag-z' })],
    };
    render(<ScheduleManagementTab />);

    // 기본 목록에서는 숨김(빈 상태) — 노드 섹션 미표시.
    expect(screen.queryByTestId('schedule-node-flow-1-empty')).toBeNull();
    expect(screen.getByTestId('schedule-empty')).toBeInTheDocument();

    // 규칙 추가 팝업 → 노드 인벤토리에 빈 노드가 존재.
    fireEvent.click(screen.getByTestId('schedule-add-rule'));
    const nodeSel = screen.getByTestId('modal-stub-node') as HTMLSelectElement;
    expect(Array.from(nodeSel.options).some((o) => o.textContent?.includes('empty 노드'))).toBe(true);
  });

  it('팝업에서 빈 노드 선택 후 에이전트 선택·저장 시 첫 규칙이 agent_id 각인 + dual-write', async () => {
    agentsState.list = [{ id: 'ag-z', name: '설비Z', type: 'xsfm' }];
    getFlowMock.mockResolvedValue({ config: { nodes: [{ id: 'empty', data: {} }] } });
    agg.value = {
      ...agg.value,
      totalFlows: 1,
      nodeEntries: [entry('flow-1', 'empty', [], { nodeType: 'trigger' })],
    };
    render(<ScheduleManagementTab />);

    fireEvent.click(screen.getByTestId('schedule-add-rule'));
    fireEvent.change(screen.getByTestId('modal-stub-node'), { target: { value: 'flow-1:empty' } });
    fireEvent.change(screen.getByTestId('modal-stub-agent'), { target: { value: 'ag-z' } });
    fireEvent.click(screen.getByTestId('modal-stub-save'));

    await waitFor(() => expect(updateFlowMock).toHaveBeenCalled());
    const sent = state.configCalls[0]!.config.schedules as Array<Record<string, unknown>>;
    expect(sent).toHaveLength(1);
    expect(sent[0]!.name).toBe('신규 규칙');
    expect(sent[0]!.agent_id).toBe('ag-z'); // 선택 에이전트 각인(REQ-01-05)
  });
});
