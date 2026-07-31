// SPEC-TRIGGER-SCHED-001 M2/M6 — FacilitySchedulePanel.
// 6컬럼 테이블 렌더(AC-4) / PRIO 정렬 / STATE 토글 dual-write(AC-7) /
// 모달 저장 dual-write(AC-5) / 404 persist-only(AC-11) / 빈 상태(REQ-02-08) / 미타겟.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';

import { APIError } from '@/types/api';

const state = vi.hoisted(() => ({
  flowNodes: [] as unknown[],
  instances: [] as Array<{ flowId: string; nodeId: string; flowName: string; nodeName: string; processed: number; errors: number }>,
  notifications: [] as Array<{ type: string; message: string }>,
  configCalls: [] as Array<{ config: Record<string, unknown> }>,
}));

const configureNodeMock = vi.hoisted(() => vi.fn());
const getFlowMock = vi.hoisted(() => vi.fn());
const updateFlowMock = vi.hoisted(() => vi.fn());

// 모달 스텁: 열리면 저장 버튼이 고정 draft 로 onSave 를 호출한다(신규 규칙 추가 경로 검증).
const modalDraft = vi.hoisted(() => ({
  value: {
    name: '신규',
    validFrom: '',
    validTo: '',
    priority: 5,
    enabled: true,
    schedule: { type: 'interval', value: '10s' },
    target: { kind: 'all', value: '2' },
    action: { power: true, fanSpeed: null },
  } as unknown,
}));
vi.mock('./FacilityRuleModal', () => ({
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

vi.mock('@/hooks/useFlow', () => ({
  useFlowNodes: () => ({ data: state.flowNodes, isLoading: false }),
}));
vi.mock('@/hooks/useNodeTypeInstances', () => ({
  useNodeTypeInstances: () => ({ instances: state.instances, isLoading: false }),
}));
vi.mock('@/hooks/useAgent', () => ({ useAgents: () => ({ data: { data: [] } }) }));
vi.mock('@/hooks/useGroups', () => ({ useGroups: () => ({ data: [] }) }));
vi.mock('@/hooks/useStation', () => ({
  useStations: () => ({ data: [] }),
  useXsfmDevices: () => ({ data: [] }),
}));
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

import FacilitySchedulePanel from './FacilitySchedulePanel';

const FLOW = 'flow-a';
const NODE = 'trig-1';

function setNode(schedules: unknown[]) {
  state.flowNodes = [{ node_id: NODE, name: 'T', type: 'trigger', state: 'running', config: { schedules } }];
}
function setRunning() {
  state.instances = [{ flowId: FLOW, nodeId: NODE, flowName: '플로우 A', nodeName: '트리거1', processed: 0, errors: 0 }];
}
function renderPanel(config: Record<string, unknown> = { flowId: FLOW, nodeId: NODE, agentId: '' }) {
  return render(
    <FacilitySchedulePanel panelId="p1" title="설비 제어 예약" config={config} onConfigChange={() => {}} onTitleChange={() => {}} />,
  );
}

beforeEach(() => {
  state.flowNodes = [];
  state.instances = [];
  state.notifications = [];
  state.configCalls = [];
  configureNodeMock.mockReset().mockResolvedValue(undefined);
  getFlowMock.mockReset().mockResolvedValue({ config: { nodes: [{ id: NODE, data: {} }] } });
  updateFlowMock.mockReset().mockResolvedValue(undefined);
});

describe('M2 — 6컬럼 테이블 (AC-4)', () => {
  it('규칙 1건의 SCHEDULE/TARGET/PLAN/ACTION/PRIO/STATE 를 렌더한다(무기한/배지/2축 라벨)', () => {
    setRunning();
    setNode([
      {
        type: 'weekly',
        days: ['mon', 'tue', 'wed', 'thu', 'fri'],
        times: ['05:30'],
        name: '평일 정규 가동',
        valid_from: '2026-01-01',
        valid_to: '',
        priority: 1,
        enabled: true,
        payload: { command: 'set_power', line: '2', params: { power: true } },
      },
    ]);
    renderPanel();

    expect(screen.getByText('평일 정규 가동')).toBeInTheDocument();
    expect(screen.getByText('2026-01-01 ~ 무기한')).toBeInTheDocument(); // 무기한
    expect(screen.getByTestId('facility-target-badge-0')).toHaveTextContent('전체'); // TARGET 배지
    expect(screen.getByText('2호선 전체')).toBeInTheDocument(); // TARGET desc
    expect(screen.getByText('주간 · 월화수목금 05:30')).toBeInTheDocument(); // PLAN
    expect(screen.getByTestId('facility-action-0')).toHaveTextContent('전원 ON'); // ACTION 2축 라벨
    expect(screen.getByTestId('facility-prio-0')).toHaveTextContent('1'); // PRIO
    expect(screen.getByTestId('facility-state-0')).toHaveTextContent('활성'); // STATE
  });

  it('PRIO 오름차순 정렬(안정)', () => {
    setRunning();
    setNode([
      { type: 'interval', value: '1s', name: 'A', priority: 2, payload: { command: 'set_power', line: '2', params: { power: true } } },
      { type: 'interval', value: '1s', name: 'B', priority: 0, payload: { command: 'set_power', line: '2', params: { power: true } } },
      { type: 'interval', value: '1s', name: 'C', priority: 1, payload: { command: 'set_power', line: '2', params: { power: true } } },
    ]);
    const { container } = renderPanel();
    const prios = Array.from(container.querySelectorAll('[data-testid^="facility-prio-"]')).map((el) => el.textContent);
    expect(prios).toEqual(['0', '1', '2']);
  });
});

describe('M6 — STATE 토글 dual-write (AC-7)', () => {
  it('STATE 배지 클릭 시 enabled 반전 + configureNode/updateFlow 호출', async () => {
    setRunning();
    setNode([{ type: 'interval', value: '1s', name: 'A', enabled: true, payload: { command: 'set_power', line: '2', params: { power: true } } }]);
    renderPanel();

    fireEvent.click(screen.getByTestId('facility-state-0'));

    await waitFor(() => expect(updateFlowMock).toHaveBeenCalled());
    expect(configureNodeMock).toHaveBeenCalledWith(FLOW, NODE, expect.any(Object));
    const sent = state.configCalls[0]!.config.schedules as Array<Record<string, unknown>>;
    expect(sent[0]!.enabled).toBe(false); // 반전
  });
});

describe('M3/M6 — 모달 저장 dual-write (AC-5)', () => {
  it('규칙 추가 → 모달 저장 시 append + dual-write', async () => {
    setRunning();
    setNode([]);
    renderPanel();

    // 빈 상태의 추가 버튼 → 모달 스텁 → 저장.
    fireEvent.click(screen.getByTestId('facility-empty-add'));
    expect(screen.getByTestId('modal-stub')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('modal-stub-save'));

    await waitFor(() => expect(updateFlowMock).toHaveBeenCalled());
    const sent = state.configCalls[0]!.config.schedules as Array<Record<string, unknown>>;
    expect(sent).toHaveLength(1);
    expect(sent[0]!.name).toBe('신규');
    expect(sent[0]!.priority).toBe(5);
    expect(sent[0]!.payload).toEqual({ command: 'set_power', line: '2', params: { power: true } });
  });
});

describe('M6 — 404 persist-only (AC-11)', () => {
  it('configureNode 404 시 persist-only 폴백 + 통지', async () => {
    // 미실행(instances 비움) + 404
    setNode([{ type: 'interval', value: '1s', name: 'A', enabled: true, payload: { command: 'set_power', line: '2', params: { power: true } } }]);
    configureNodeMock.mockRejectedValue(new APIError('NOT_FOUND', 'not running', 404));
    renderPanel();

    expect(screen.getByTestId('facility-badge-stopped')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('facility-state-0'));

    await waitFor(() => expect(updateFlowMock).toHaveBeenCalled());
    expect(updateFlowMock).toHaveBeenCalledTimes(1);
    expect(state.notifications.some((n) => n.type === 'warning' && n.message.includes('노드 미실행'))).toBe(true);
  });
});

describe('빈 상태 / 미타겟', () => {
  it('규칙 0건이면 빈 상태 + 추가 진입점(REQ-02-08)', () => {
    setRunning();
    setNode([]);
    renderPanel();
    expect(screen.getByTestId('facility-empty')).toBeInTheDocument();
    expect(screen.getByTestId('facility-empty-add')).toBeInTheDocument();
  });

  it('flowId/nodeId 미지정이면 안내', () => {
    renderPanel({ flowId: '', nodeId: '', agentId: '' });
    expect(screen.getByTestId('facility-schedule-untargeted')).toBeInTheDocument();
  });
});
