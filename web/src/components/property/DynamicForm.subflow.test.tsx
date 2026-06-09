// SPEC-SUBFLOW-001 v1.2 그룹 RU — DynamicForm 포트 해석 분기 검증.
//
// flow-node 의 flow_id 가 remote://{instanceId}/{flowId} 정규화 참조면, 에디터
// 타깃(로컬/원격)과 무관하게 그 참조가 가리키는 노드에서 포트를 해석해야 한다
// (resolveRemoteFlowNodePorts(ref.instanceId, ref.flowId)). 평문 로컬 id 는
// resolveFlowNodePorts(로컬), 동일노드 원격 편집은 resolveRemoteFlowNodePorts(타깃 노드).

import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import { LOCAL_TARGET, type ResourceTarget } from '@/lib/remote/target';
import type { ConfigSchema } from '@/types/node';
import type { FlowInfo } from '@/types/flow';
import type { ManagedNode } from '@/types/remote';
import type { ResolvedFlowNodePorts } from '@/lib/flow/subflowPorts';

// ---- 포트 해석 함수 mock (실제 buildRemoteFlowRef/parseRemoteFlowRef 는 보존) ----
const resolveFlowNodePortsMock = vi.hoisted(() =>
  vi.fn<(flowId: string) => Promise<ResolvedFlowNodePorts>>(),
);
const resolveRemoteFlowNodePortsMock = vi.hoisted(() =>
  vi.fn<(instanceId: string, flowId: string) => Promise<ResolvedFlowNodePorts>>(),
);
vi.mock('@/lib/flow/subflowPorts', async () => {
  const actual = await vi.importActual<typeof import('@/lib/flow/subflowPorts')>(
    '@/lib/flow/subflowPorts',
  );
  return {
    ...actual,
    resolveFlowNodePorts: (flowId: string) => resolveFlowNodePortsMock(flowId),
    resolveRemoteFlowNodePorts: (instanceId: string, flowId: string) =>
      resolveRemoteFlowNodePortsMock(instanceId, flowId),
  };
});

// ---- 타깃 컨텍스트 mock ----
const useTargetContextMock = vi.hoisted(() => vi.fn<() => ResourceTarget>());
vi.mock('@/lib/remote/TargetContext', () => ({
  useTargetContext: () => useTargetContextMock(),
}));

// ---- 타깃 인지 플로우 목록 + 원격 훅 mock (FlowPickerInput 의존성) ----
const useFlowsTargetMock = vi.hoisted(() =>
  vi.fn<() => { data: { data: FlowInfo[]; total: number } | undefined; isLoading: boolean }>(),
);
vi.mock('@/hooks/useResourceTargets', () => ({
  useFlowsTarget: () => useFlowsTargetMock(),
}));

const useManagedNodesMock = vi.hoisted(() => vi.fn());
const useRemoteModeMock = vi.hoisted(() => vi.fn());
const useNodeLiveListMock = vi.hoisted(() => vi.fn());
vi.mock('@/hooks/useRemote', () => ({
  useManagedNodes: (...a: unknown[]) => useManagedNodesMock(...a),
  useRemoteMode: () => useRemoteModeMock(),
  useNodeLiveList: (...a: unknown[]) => useNodeLiveListMock(...a),
}));

vi.mock('@/stores/editorStore', () => ({
  useEditorStore: (selector: (s: { currentFlowId: string | null }) => unknown) =>
    selector({ currentFlowId: null }),
}));

vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [] }, isLoading: false }),
}));

import { DynamicForm } from './DynamicForm';

const schema: ConfigSchema = {
  fields: [{ name: 'flow_id', type: 'flow_picker', label: '참조 플로우' }],
};

function flow(id: string, name: string): FlowInfo {
  return { id, name, status: 'stored', node_count: 0 };
}

function node(): ManagedNode {
  return {
    instance_id: 'node-a',
    hostname: 'edge-host-a',
    version: '1.0.0',
    status: 'approved',
    online: true,
    last_seen: Date.now(),
  };
}

function renderForm(data: Record<string, unknown>) {
  const onChange = vi.fn();
  render(
    <I18nProvider>
      <DynamicForm nodeId="n1" data={data} schema={schema} onChange={onChange} />
    </I18nProvider>,
  );
  return onChange;
}

const ports: ResolvedFlowNodePorts = {
  input_ports: ['in'],
  output_ports: ['out'],
  flow_name: 'resolved',
};

beforeEach(() => {
  resolveFlowNodePortsMock.mockReset().mockResolvedValue(ports);
  resolveRemoteFlowNodePortsMock.mockReset().mockResolvedValue(ports);
  useTargetContextMock.mockReset().mockReturnValue(LOCAL_TARGET);
  useFlowsTargetMock.mockReset().mockReturnValue({
    data: { data: [flow('lf1', '로컬 플로우 1')], total: 1 },
    isLoading: false,
  });
  useManagedNodesMock.mockReset().mockReturnValue({ data: [node()] });
  useRemoteModeMock.mockReset().mockReturnValue({ data: { mode: 'server' } });
  useNodeLiveListMock.mockReset().mockReturnValue({
    data: [flow('rf1', '원격 플로우 1')],
    isLoading: false,
  });
});

describe('DynamicForm — flow-node 포트 해석 분기 (그룹 RU)', () => {
  it('로컬 편집에서 원격 노드 플로우 선택 시 ref 의 노드로 원격 포트를 해석한다', async () => {
    renderForm({ flow_id: '' });

    fireEvent.change(screen.getByLabelText('플로우 위치'), {
      target: { value: 'node-a' },
    });
    fireEvent.change(screen.getByLabelText('참조 플로우'), {
      target: { value: 'rf1' },
    });

    // remote://node-a/rf1 → resolveRemoteFlowNodePorts('node-a', 'rf1') 호출(에디터는 로컬).
    await waitFor(() => {
      expect(resolveRemoteFlowNodePortsMock).toHaveBeenCalledWith('node-a', 'rf1');
    });
    expect(resolveFlowNodePortsMock).not.toHaveBeenCalled();
  });

  it('로컬 플로우 선택은 로컬 포트 해석(resolveFlowNodePorts)을 사용한다', async () => {
    renderForm({ flow_id: '' });

    fireEvent.change(screen.getByLabelText('참조 플로우'), {
      target: { value: 'lf1' },
    });

    await waitFor(() => {
      expect(resolveFlowNodePortsMock).toHaveBeenCalledWith('lf1');
    });
    expect(resolveRemoteFlowNodePortsMock).not.toHaveBeenCalled();
  });

  it('기존 remote:// 값의 "포트 갱신"은 ref 의 노드로 원격 포트를 해석한다', async () => {
    renderForm({ flow_id: 'remote://node-a/rf1', flow_name: '원격 플로우 1' });

    fireEvent.click(screen.getByLabelText('포트 갱신'));

    await waitFor(() => {
      expect(resolveRemoteFlowNodePortsMock).toHaveBeenCalledWith('node-a', 'rf1');
    });
    expect(resolveFlowNodePortsMock).not.toHaveBeenCalled();
  });

  it('동일노드 원격 편집(target=remote)의 bare id 는 타깃 노드로 해석한다(회귀)', async () => {
    useTargetContextMock.mockReturnValue({ type: 'remote', instanceId: 'node-z' });
    // 동일노드 편집은 useFlowsTarget 이 그 노드의 플로우를 위임한다.
    useFlowsTargetMock.mockReturnValue({
      data: { data: [flow('nf1', '노드 플로우 1')], total: 1 },
      isLoading: false,
    });
    renderForm({ flow_id: '' });

    fireEvent.change(screen.getByLabelText('참조 플로우'), {
      target: { value: 'nf1' },
    });

    await waitFor(() => {
      expect(resolveRemoteFlowNodePortsMock).toHaveBeenCalledWith('node-z', 'nf1');
    });
    expect(resolveFlowNodePortsMock).not.toHaveBeenCalled();
  });
});
