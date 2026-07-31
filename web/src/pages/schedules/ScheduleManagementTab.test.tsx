// SPEC-SCHEDULE-VIEW-001 M5 — ScheduleManagementTab.
// agent 그룹 렌더 + 미지정 버킷(AC-8) / 교차-플로우 집계 ≥2 플로우(AC-7/AC-18) /
// 부분 실패 통지(AC-18) / CRUD 경로가 dual-write(configureNode+updateFlow)를 호출.
//
// useScheduleAggregation 훅만 스텁하고 groupEntriesByAgent/resolveScheduleAgent 는
// 실제를 사용해(부분 목) 그룹핑 로직을 함께 검증한다. FacilityRuleModal 은 고정 draft
// 저장 스텁으로 대체하고, dual-write 하위 서비스는 목으로 관측한다.

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

// FacilityRuleModal 스텁: 저장 시 고정 유효 draft 로 onSave 호출(신규 규칙 append 경로).
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
  default: ({ onSave, onCancel }: { onSave: (d: unknown) => void; onCancel: () => void }) => (
    <div data-testid="modal-stub">
      <button type="button" data-testid="modal-stub-save" onClick={() => onSave(modalDraft.value)}>
        save
      </button>
      <button type="button" data-testid="modal-stub-cancel" onClick={onCancel}>
        cancel
      </button>
    </div>
  ),
}));

vi.mock('@/hooks/useAgent', () => ({ useAgents: () => ({ data: { data: [] } }) }));
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
): ScheduleNodeEntry {
  return { flowId, flowName: `${flowId} 이름`, nodeId, nodeName: `${nodeId} 노드`, nodeConfig, schedules };
}

beforeEach(() => {
  state.configCalls = [];
  state.notifications = [];
  configureNodeMock.mockReset().mockResolvedValue(undefined);
  getFlowMock.mockReset().mockResolvedValue({ config: { nodes: [] } });
  updateFlowMock.mockReset().mockResolvedValue(undefined);
  agg.value = {
    nodeEntries: [],
    isLoading: false,
    failedCount: 0,
    totalFlows: 0,
    hasError: false,
    refetch: vi.fn(),
  };
});

describe('그룹 렌더 — agent 그룹 + 미지정 버킷(AC-7/AC-8, 교차 플로우 AC-18)', () => {
  it('2개 플로우의 스케줄을 agent 그룹과 미지정 버킷으로 나눠 렌더한다', () => {
    agg.value = {
      ...agg.value,
      totalFlows: 2,
      nodeEntries: [
        entry('flow-1', 't1', [{ agent_id: 'ag-x', type: 'interval', value: '1s', payload: { command: 'set_power', line: '2', params: { power: true } } }]),
        entry('flow-2', 't2', [{ type: 'interval', value: '1s', payload: { command: 'set_power', line: '2', params: { power: true } } }]),
      ],
    };
    render(<ScheduleManagementTab />);

    // agent 그룹 + 미지정 버킷 동시 존재(교차-플로우, 드롭 없음).
    expect(screen.getByTestId('schedule-group-ag-x')).toBeInTheDocument();
    expect(screen.getByTestId('schedule-group-unassigned')).toBeInTheDocument();
    // 각 그룹에 소유 노드 섹션.
    expect(screen.getByTestId('schedule-node-flow-1-t1')).toBeInTheDocument();
    expect(screen.getByTestId('schedule-node-flow-2-t2')).toBeInTheDocument();
  });

  it('스케줄이 하나도 없으면 빈 상태', () => {
    render(<ScheduleManagementTab />);
    expect(screen.getByTestId('schedule-empty')).toBeInTheDocument();
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
    expect(screen.getByTestId('schedule-group-ag-x')).toBeInTheDocument(); // 성공 그룹 유지
  });
});

describe('CRUD → dual-write(AC-7/AC-9, REQ-06-04)', () => {
  it('규칙 추가 → 모달 저장 시 소유 노드에 append + configureNode/updateFlow + agent_id 각인', async () => {
    getFlowMock.mockResolvedValue({ config: { nodes: [{ id: 't1', data: {} }] } });
    agg.value = {
      ...agg.value,
      totalFlows: 1,
      nodeEntries: [entry('flow-1', 't1', [{ agent_id: 'ag-x', type: 'interval', value: '1s', payload: { command: 'set_power', line: '2', params: { power: true } } }])],
    };
    render(<ScheduleManagementTab />);

    fireEvent.click(screen.getByTestId('schedule-add-flow-1-t1'));
    expect(screen.getByTestId('modal-stub')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('modal-stub-save'));

    await waitFor(() => expect(updateFlowMock).toHaveBeenCalled());
    expect(configureNodeMock).toHaveBeenCalledWith('flow-1', 't1', expect.any(Object));
    const sent = state.configCalls[0]!.config.schedules as Array<Record<string, unknown>>;
    expect(sent).toHaveLength(2); // 기존 1 + 신규 1(append, origin 보존)
    const created = sent[1]!;
    expect(created.name).toBe('신규 규칙');
    expect(created.agent_id).toBe('ag-x'); // REQ-01-05 각인
    expect(created.payload).toEqual({ command: 'set_power', line: '2', params: { power: true } });
    expect(agg.value.refetch).toHaveBeenCalled(); // 재조회 트리거
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
