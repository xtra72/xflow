// SPEC-TRIGGER-PANEL-001 §B/§C/§D/§E — TriggerConfigPanel.
// B-3(노드 config 로드 렌더) / B-4(not-running) / B-5(running·stopped 배지)
// C-1(스케줄 CRUD add) / C-2(per-schedule 카탈로그 셀렉터)
// D-1(카탈로그 CRUD) / D-2(주입 스냅샷 → save 전송)
// E-1(dual-write) / E-2(FULL config) / E-3(404 persist-only + 배지) / E-4(last-write-wins 통지)

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';

import { APIError } from '@/types/api';

// ---- 모듈 mock ----

const state = vi.hoisted(() => ({
  flowNodes: [] as unknown[],
  instances: [] as Array<{ flowId: string; nodeId: string; flowName: string; nodeName: string; processed: number; errors: number }>,
  notifications: [] as Array<{ type: string; message: string }>,
  configCalls: [] as Array<{ config: Record<string, unknown> }>,
  onConfigChangeCalls: [] as Array<Record<string, unknown>>,
}));

const configureNodeMock = vi.hoisted(() => vi.fn());
const getFlowMock = vi.hoisted(() => vi.fn());
const updateFlowMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useFlow', () => ({
  useFlowNodes: () => ({ data: state.flowNodes, isLoading: false }),
}));
vi.mock('@/hooks/useNodeTypeInstances', () => ({
  useNodeTypeInstances: () => ({ instances: state.instances, isLoading: false }),
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
    selector({
      addNotification: (n: { type: string; message: string }) => state.notifications.push(n),
    }),
}));
vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import TriggerConfigPanel from './TriggerConfigPanel';

const FLOW = 'flow-a';
const NODE = 'trig-1';

/** 대상 노드 하나를 flowNodes 로 세팅한다. */
function setNode(config: Record<string, unknown>) {
  state.flowNodes = [{ node_id: NODE, name: 'T', type: 'trigger', state: 'running', config }];
}

/** 대상 노드를 running 인스턴스로 등록한다. */
function setRunning() {
  state.instances = [{ flowId: FLOW, nodeId: NODE, flowName: '플로우 A', nodeName: '트리거1', processed: 0, errors: 0 }];
}

function renderPanel(config: Record<string, unknown> = { flowId: FLOW, nodeId: NODE, payloadCatalog: {} }) {
  return render(
    <TriggerConfigPanel
      panelId="p1"
      title="트리거 설정"
      config={config}
      onConfigChange={(c) => state.onConfigChangeCalls.push(c)}
      onTitleChange={() => {}}
    />,
  );
}

beforeEach(() => {
  state.flowNodes = [];
  state.instances = [];
  state.notifications = [];
  state.configCalls = [];
  state.onConfigChangeCalls = [];
  configureNodeMock.mockReset().mockResolvedValue(undefined);
  getFlowMock.mockReset();
  updateFlowMock.mockReset().mockResolvedValue(undefined);
});

describe('§B 렌더 / 타겟팅', () => {
  it('B-3: 대상 노드의 현재 스케줄/페이로드를 useFlowNodes 로 읽어 렌더한다', () => {
    setRunning();
    setNode({ schedules: [{ type: 'interval', value: '5s' }, { type: 'cron', value: '0 */5 * * * *' }] });
    renderPanel();

    // 노드 config 의 스케줄 값이 편집기에 렌더된다(패널 config 에 복제하지 않음).
    expect(screen.getByDisplayValue('5s')).toBeInTheDocument();
    expect(screen.getByDisplayValue('0 */5 * * * *')).toBeInTheDocument();
  });

  it('B-4: 대상이 running 목록에 없으면 not-running(persist-only) 표식을 표시한다', () => {
    // instances 비움 → stopped
    setNode({ schedules: [] });
    renderPanel();

    expect(screen.getByTestId('trigger-persist-only-notice')).toBeInTheDocument();
    expect(screen.getByTestId('trigger-badge-stopped')).toBeInTheDocument();
  });

  it('B-5(a): 대상이 running 이면 running 배지', () => {
    setRunning();
    setNode({ schedules: [] });
    renderPanel();
    expect(screen.getByTestId('trigger-badge-running')).toBeInTheDocument();
    expect(screen.queryByTestId('trigger-badge-stopped')).toBeNull();
  });

  it('B-5(b): 대상이 미실행이면 stopped 배지', () => {
    setNode({ schedules: [] });
    renderPanel();
    expect(screen.getByTestId('trigger-badge-stopped')).toBeInTheDocument();
    expect(screen.queryByTestId('trigger-badge-running')).toBeNull();
  });

  it('타겟 미지정이면 안내 메시지를 표시한다', () => {
    renderPanel({ flowId: '', nodeId: '', payloadCatalog: {} });
    expect(screen.getByTestId('trigger-panel-untargeted')).toBeInTheDocument();
  });
});

describe('§C 스케줄 편집 UI', () => {
  it('C-1: TriggerScheduleEditor 로 스케줄을 추가할 수 있다(6종 CRUD 재사용)', () => {
    setRunning();
    setNode({ schedules: [] });
    renderPanel();

    // 빈 상태에서 추가 메뉴 열기 → interval 선택 → 기본값 5s 행 추가.
    fireEvent.click(screen.getByText('property.schedule.add'));
    fireEvent.click(screen.getByText('property.schedule.typeInterval'));
    expect(screen.getByDisplayValue('5s')).toBeInTheDocument();
  });

  it('C-2: per-schedule 페이로드 셀렉터가 카탈로그 이름 목록을 제시한다', () => {
    setRunning();
    setNode({ schedules: [{ type: 'interval', value: '5s' }] });
    renderPanel({ flowId: FLOW, nodeId: NODE, payloadCatalog: { warn: { level: 3 }, info: { level: 1 } } });

    const select = screen.getByTestId('inject-select-0') as HTMLSelectElement;
    const optionValues = Array.from(select.options).map((o) => o.value);
    expect(optionValues).toContain('warn');
    expect(optionValues).toContain('info');
  });
});

describe('§D 카탈로그 + inline 주입', () => {
  it('D-1: 카탈로그 항목을 추가하면 패널 config payloadCatalog 가 갱신된다', () => {
    setRunning();
    setNode({ schedules: [] });
    renderPanel({ flowId: FLOW, nodeId: NODE, payloadCatalog: {} });

    fireEvent.change(screen.getByTestId('catalog-new-name'), { target: { value: 'warn' } });
    fireEvent.click(screen.getByTestId('catalog-add'));

    expect(state.onConfigChangeCalls).toContainEqual({ payloadCatalog: { warn: {} } });
  });

  it('D-1: 카탈로그 항목을 삭제하면 payloadCatalog 에서 제거된다', () => {
    setRunning();
    setNode({ schedules: [] });
    renderPanel({ flowId: FLOW, nodeId: NODE, payloadCatalog: { warn: { level: 3 } } });

    fireEvent.click(screen.getByTestId('catalog-remove-warn'));
    expect(state.onConfigChangeCalls).toContainEqual({ payloadCatalog: {} });
  });

  it('D-1: 카탈로그 payload 편집(JSON) 시 payloadCatalog 가 갱신된다', () => {
    setRunning();
    setNode({ schedules: [] });
    renderPanel({ flowId: FLOW, nodeId: NODE, payloadCatalog: { warn: { level: 3 } } });

    const ta = screen.getByTestId('catalog-edit-warn');
    fireEvent.blur(ta, { target: { value: '{"level":9}' } });
    expect(state.onConfigChangeCalls).toContainEqual({ payloadCatalog: { warn: { level: 9 } } });
  });

  it('D-2: 스케줄에 카탈로그를 주입하면 저장 시 그 스케줄 payload 로 인라인 전송된다', async () => {
    setRunning();
    setNode({ schedules: [{ type: 'interval', value: '5s' }] });
    getFlowMock.mockResolvedValue({ config: { nodes: [{ id: NODE, data: { schedules: [{ type: 'interval', value: '5s' }] } }] } });
    renderPanel({ flowId: FLOW, nodeId: NODE, payloadCatalog: { warn: { level: 3 } } });

    // 스케줄 0 에 warn 주입(스냅샷).
    fireEvent.change(screen.getByTestId('inject-select-0'), { target: { value: 'warn' } });
    fireEvent.click(screen.getByTestId('trigger-save'));

    await waitFor(() => expect(configureNodeMock).toHaveBeenCalled());
    const sentConfig = state.configCalls[0]!.config;
    const sentSchedules = sentConfig.schedules as Array<Record<string, unknown>>;
    expect(sentSchedules[0]!.payload).toEqual({ level: 3 });
    // 카탈로그 이름은 노드로 전달되지 않는다(inline payload 만).
    expect(JSON.stringify(sentConfig)).not.toContain('payloadCatalog');
  });
});

describe('§E dual-write 지속성', () => {
  it('E-1: 저장 시 configureNode(LIVE) + getFlow→patch→updateFlow(PERSIST) 를 수행한다', async () => {
    setRunning();
    setNode({ schedules: [{ type: 'interval', value: '5s' }], payload: { a: 1 } });
    getFlowMock.mockResolvedValue({
      config: {
        nodes: [
          { id: NODE, type: 'custom', data: { nodeType: 'trigger', label: 'T', schedules: [{ type: 'interval', value: '5s' }], payload: { a: 1 } } },
          { id: 'other', type: 'custom', data: { nodeType: 'output', label: 'O' } },
        ],
        edges: [{ id: 'e', source: NODE, target: 'other' }],
        inputs: [],
        outputs: [],
      },
    });
    renderPanel();

    fireEvent.click(screen.getByTestId('trigger-save'));

    await waitFor(() => expect(updateFlowMock).toHaveBeenCalled());
    expect(configureNodeMock).toHaveBeenCalledWith(FLOW, NODE, expect.any(Object));
    expect(getFlowMock).toHaveBeenCalledWith(FLOW);

    const [id, req] = updateFlowMock.mock.calls[0]!;
    expect(id).toBe(FLOW);
    const def = (req as { definition: Record<string, unknown> }).definition;
    const nodes = def.nodes as Array<Record<string, unknown>>;
    // 대상 노드 config 패치 + 다른 노드/와이어 보존.
    expect((nodes[0]!.data as Record<string, unknown>).schedules).toEqual([{ type: 'interval', value: '5s' }]);
    expect(nodes[1]).toEqual({ id: 'other', type: 'custom', data: { nodeType: 'output', label: 'O' } });
    expect(def.edges).toEqual([{ id: 'e', source: NODE, target: 'other' }]);
  });

  it('E-2: configureNode·updateFlow 에 FULL config(스케줄+페이로드) 를 전송한다', async () => {
    setRunning();
    setNode({ schedules: [{ type: 'interval', value: '5s' }], payload: { a: 1 }, source_ch_size: 64 });
    getFlowMock.mockResolvedValue({ config: { nodes: [{ id: NODE, data: { schedules: [{ type: 'interval', value: '5s' }], payload: { a: 1 } } }] } });
    renderPanel();

    fireEvent.click(screen.getByTestId('trigger-save'));

    await waitFor(() => expect(configureNodeMock).toHaveBeenCalled());
    const sent = state.configCalls[0]!.config;
    expect(sent.schedules).toEqual([{ type: 'interval', value: '5s' }]);
    expect(sent.payload).toEqual({ a: 1 });
    expect(sent.source_ch_size).toBe(64);
  });

  it('E-3: live configureNode 404 시 persist-only + 통지, stopped 배지 노출', async () => {
    // 미실행 → instances 비움(stopped 배지). configureNode 404.
    setNode({ schedules: [{ type: 'interval', value: '5s' }] });
    configureNodeMock.mockRejectedValue(new APIError('NOT_FOUND', 'not running', 404));
    getFlowMock.mockResolvedValue({ config: { nodes: [{ id: NODE, data: { schedules: [{ type: 'interval', value: '5s' }] } }] } });
    renderPanel();

    expect(screen.getByTestId('trigger-badge-stopped')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('trigger-save'));

    await waitFor(() => expect(updateFlowMock).toHaveBeenCalled());
    // 오류로 처리하지 않고 persist 는 수행.
    expect(updateFlowMock).toHaveBeenCalledTimes(1);
    expect(state.notifications.some((n) => n.type === 'warning' && n.message.includes('노드 미실행'))).toBe(true);
  });

  it('E-4: 동시 편집 감지 시 last-write-wins 덮어쓰기 통지(잠금 경로 없음)', async () => {
    setRunning();
    setNode({ schedules: [{ type: 'interval', value: '5s' }] });
    // 지속화 직전 읽은 정의의 노드 config 가 baseline 과 다름(외부 편집).
    getFlowMock.mockResolvedValue({ config: { nodes: [{ id: NODE, data: { schedules: [{ type: 'interval', value: '99s' }] } }] } });
    renderPanel();

    fireEvent.click(screen.getByTestId('trigger-save'));

    await waitFor(() => expect(updateFlowMock).toHaveBeenCalled());
    expect(state.notifications.some((n) => n.type === 'warning' && n.message.includes('last-write-wins'))).toBe(true);
  });

  it('E-1: 정상 저장 시 성공 통지', async () => {
    setRunning();
    setNode({ schedules: [{ type: 'interval', value: '5s' }] });
    getFlowMock.mockResolvedValue({ config: { nodes: [{ id: NODE, data: { schedules: [{ type: 'interval', value: '5s' }] } }] } });
    renderPanel();

    fireEvent.click(screen.getByTestId('trigger-save'));

    await waitFor(() => expect(updateFlowMock).toHaveBeenCalled());
    expect(state.notifications.some((n) => n.type === 'success' && n.message === '저장 완료')).toBe(true);
  });
});
