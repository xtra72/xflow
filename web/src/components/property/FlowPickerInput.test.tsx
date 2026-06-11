// SPEC-SUBFLOW-001 v1.2 그룹 RU — flow_picker 의 로컬 편집 → 원격 노드 플로우 참조.
//
// 로컬 편집(target=local)에서 picker 가:
//   - 승인+온라인 원격 노드를 노드 선택기에 제공한다.
//   - 원격 노드를 고르면 그 노드의 라이브 플로우 목록을 나열한다.
//   - 원격 플로우를 고르면 flow_id 를 remote://{instanceId}/{flowId} 로 저장한다.
//   - 기존 remote:// 값을 노드 사전 선택 + "원격" 배지 + 노드/플로우명으로 렌더한다.
//   - 로컬 선택은 bare id 를 저장한다(회귀 없음).
// 또한 동일노드 원격 편집(target=remote) + 로컬 전용(non-server) 환경에서
// 노드 선택기가 노출되지 않음을 검증한다(회귀 없음).

import { render, screen, fireEvent, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import { LOCAL_TARGET, type ResourceTarget } from '@/lib/remote/target';
import type { FlowInfo } from '@/types/flow';
import type { ManagedNode } from '@/types/remote';

import { FormField } from './FormField';
import type { ConfigField } from '@/types/node';

// ---- 타깃 컨텍스트 mock (로컬 기본, 케이스별 오버라이드) ----
const useTargetContextMock = vi.hoisted(() => vi.fn<() => ResourceTarget>());
vi.mock('@/lib/remote/TargetContext', () => ({
  useTargetContext: () => useTargetContextMock(),
}));

// ---- 타깃 인지 플로우 목록(로컬/동일노드) mock ----
const useFlowsTargetMock = vi.hoisted(() =>
  vi.fn<() => { data: { data: FlowInfo[]; total: number } | undefined; isLoading: boolean }>(),
);
vi.mock('@/hooks/useResourceTargets', () => ({
  useFlowsTarget: () => useFlowsTargetMock(),
}));

// ---- 원격 훅 mock (노드 목록 / 모드 / 노드 라이브 플로우) ----
const useManagedNodesMock = vi.hoisted(() => vi.fn());
const useRemoteModeMock = vi.hoisted(() => vi.fn());
const useNodeLiveListMock = vi.hoisted(() => vi.fn());
vi.mock('@/hooks/useRemote', () => ({
  useManagedNodes: (...args: unknown[]) => useManagedNodesMock(...args),
  useRemoteMode: () => useRemoteModeMock(),
  useNodeLiveList: (...args: unknown[]) => useNodeLiveListMock(...args),
}));

// ---- editorStore.currentFlowId mock (자기참조 제외 검증용) ----
const currentFlowIdRef = vi.hoisted(() => ({ value: null as string | null }));
vi.mock('@/stores/editorStore', () => ({
  useEditorStore: (selector: (s: { currentFlowId: string | null }) => unknown) =>
    selector({ currentFlowId: currentFlowIdRef.value }),
}));

// useAgents 는 flow_picker 가 직접 쓰지 않지만 FormField import 그래프에 포함될 수
// 있으므로 안전하게 빈 결과로 mock 한다.
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [] }, isLoading: false }),
}));

function flowField(): ConfigField {
  return { name: 'flow_id', type: 'flow_picker', label: '참조 플로우' };
}

function localFlow(id: string, name: string): FlowInfo {
  return { id, name, status: 'stored', node_count: 0 };
}

function makeNode(overrides: Partial<ManagedNode> = {}): ManagedNode {
  return {
    instance_id: 'node-a',
    hostname: 'edge-host-a',
    version: '1.0.0',
    status: 'approved',
    online: true,
    last_seen: Date.now(),
    ...overrides,
  };
}

function renderPicker(props: {
  value: string;
  flowName?: string;
  onChange: (v: unknown) => void;
}) {
  return render(
    <I18nProvider>
      <FormField
        field={flowField()}
        value={props.value}
        flowName={props.flowName}
        onChange={props.onChange}
      />
    </I18nProvider>,
  );
}

beforeEach(() => {
  useTargetContextMock.mockReset();
  useFlowsTargetMock.mockReset();
  useManagedNodesMock.mockReset();
  useRemoteModeMock.mockReset();
  useNodeLiveListMock.mockReset();
  currentFlowIdRef.value = null;

  // 기본: 로컬 편집 + server 모드 + 승인/온라인 노드 1개 + 로컬 플로우 2개.
  useTargetContextMock.mockReturnValue(LOCAL_TARGET);
  useFlowsTargetMock.mockReturnValue({
    data: { data: [localFlow('lf1', '로컬 플로우 1'), localFlow('lf2', '로컬 플로우 2')], total: 2 },
    isLoading: false,
  });
  useRemoteModeMock.mockReturnValue({ data: { mode: 'server' } });
  useManagedNodesMock.mockReturnValue({ data: [makeNode()] });
  // 노드 라이브 플로우 기본값(원격 미선택 시 호출되어도 안전).
  useNodeLiveListMock.mockReturnValue({ data: [], isLoading: false });
});

describe('FlowPickerInput — 로컬 편집에서 원격 노드 플로우 선택 (그룹 RU)', () => {
  it('승인+온라인 원격 노드가 노드 선택기에 제공된다(로컬 + 노드)', () => {
    renderPicker({ value: '', onChange: vi.fn() });
    const nodeSelect = screen.getByLabelText('플로우 위치') as HTMLSelectElement;
    const options = within(nodeSelect).getAllByRole('option') as HTMLOptionElement[];
    expect(options.map((o) => o.textContent)).toEqual(['로컬', 'edge-host-a']);
    expect(options.map((o) => o.value)).toEqual(['local', 'node-a']);
  });

  it('오프라인/미승인 노드는 후보에서 제외된다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [
        makeNode({ instance_id: 'on', hostname: 'on-host', online: true, status: 'approved' }),
        makeNode({ instance_id: 'off', hostname: 'off-host', online: false }),
        makeNode({ instance_id: 'pend', hostname: 'pend-host', status: 'pending' }),
      ],
    });
    renderPicker({ value: '', onChange: vi.fn() });
    const nodeSelect = screen.getByLabelText('플로우 위치') as HTMLSelectElement;
    const labels = (within(nodeSelect).getAllByRole('option') as HTMLOptionElement[]).map(
      (o) => o.value,
    );
    expect(labels).toEqual(['local', 'on']);
  });

  it('원격 노드를 고르면 그 노드의 라이브 플로우 목록을 나열한다', () => {
    useNodeLiveListMock.mockReturnValue({
      data: [localFlow('rf1', '원격 플로우 1'), localFlow('rf2', '원격 플로우 2')],
      isLoading: false,
    });
    renderPicker({ value: '', onChange: vi.fn() });

    const nodeSelect = screen.getByLabelText('플로우 위치') as HTMLSelectElement;
    fireEvent.change(nodeSelect, { target: { value: 'node-a' } });

    // useNodeLiveList 가 node-a + 'flow' + enabled=true 로 호출되었는지.
    expect(useNodeLiveListMock).toHaveBeenCalledWith('node-a', 'flow', true);

    // 플로우 select(라벨 없는 두 번째 select) 에 원격 플로우가 나열된다.
    const flowSelect = screen.getByLabelText('참조 플로우') as HTMLSelectElement;
    const flowOpts = (within(flowSelect).getAllByRole('option') as HTMLOptionElement[]).map(
      (o) => o.textContent,
    );
    expect(flowOpts).toContain('원격 플로우 1');
    expect(flowOpts).toContain('원격 플로우 2');
  });

  it('원격 플로우 선택 시 flow_id 를 remote://node/flow 로 저장한다', () => {
    useNodeLiveListMock.mockReturnValue({
      data: [localFlow('rf1', '원격 플로우 1')],
      isLoading: false,
    });
    const onChange = vi.fn();
    renderPicker({ value: '', onChange });

    fireEvent.change(screen.getByLabelText('플로우 위치'), {
      target: { value: 'node-a' },
    });
    // 노드 변경은 선택 초기화를 트리거(빈 flow_id).
    onChange.mockClear();

    fireEvent.change(screen.getByLabelText('참조 플로우'), {
      target: { value: 'rf1' },
    });
    // 원격 선택은 라벨 자동 산출용 호스트 라벨(remote_node_label)도 함께 전달한다.
    // node-a 의 hostname 은 edge-host-a 이므로 selectedNodeLabel = 'edge-host-a'.
    expect(onChange).toHaveBeenCalledWith({
      flow_id: 'remote://node-a/rf1',
      flow_name: '원격 플로우 1',
      remote_node_label: 'edge-host-a',
    });
  });

  it('로컬 선택은 bare id 를 저장한다(회귀 없음)', () => {
    const onChange = vi.fn();
    renderPicker({ value: '', onChange });
    fireEvent.change(screen.getByLabelText('참조 플로우'), {
      target: { value: 'lf2' },
    });
    expect(onChange).toHaveBeenCalledWith({ flow_id: 'lf2', flow_name: '로컬 플로우 2' });
  });

  it('기존 remote:// 값을 노드 사전 선택 + 배지 + 노드/플로우명으로 렌더한다', () => {
    useNodeLiveListMock.mockReturnValue({
      data: [localFlow('rf1', '원격 플로우 1')],
      isLoading: false,
    });
    renderPicker({
      value: 'remote://node-a/rf1',
      flowName: '원격 플로우 1',
      onChange: vi.fn(),
    });

    // 노드 선택기가 node-a 로 사전 선택된다.
    const nodeSelect = screen.getByLabelText('플로우 위치') as HTMLSelectElement;
    expect(nodeSelect.value).toBe('node-a');

    // "원격" 배지 + 노드 라벨이 표시된다(RU04). 배지 안에 노드 라벨이 포함된다.
    expect(screen.getByText('원격')).toBeInTheDocument();
    const badge = screen.getByTitle('원격 노드의 플로우 (배포 시 인라인 확장)');
    expect(badge.textContent).toMatch(/원격/);
    expect(badge.textContent).toMatch(/edge-host-a/);

    // 플로우 select 는 ref 의 flowId(bare) 로 표시되며 플로우명이 보인다.
    const flowSelect = screen.getByLabelText('참조 플로우') as HTMLSelectElement;
    expect(flowSelect.value).toBe('rf1');
    expect(within(flowSelect).getByText('원격 플로우 1')).toBeInTheDocument();
  });

  it('자기참조: currentFlowId 는 로컬 후보에서 제외되지만 원격 후보에는 영향 없다', () => {
    currentFlowIdRef.value = 'lf1';
    useNodeLiveListMock.mockReturnValue({
      data: [localFlow('lf1', '동명 원격 플로우')],
      isLoading: false,
    });
    renderPicker({ value: '', onChange: vi.fn() });

    // 로컬 후보에서 lf1 제외.
    const flowSelectLocal = screen.getByLabelText('참조 플로우') as HTMLSelectElement;
    const localOpts = (
      within(flowSelectLocal).getAllByRole('option') as HTMLOptionElement[]
    ).map((o) => o.value);
    expect(localOpts).not.toContain('lf1');
    expect(localOpts).toContain('lf2');

    // 원격 노드 선택 시 lf1(원격 스코프) 은 제외되지 않는다.
    fireEvent.change(screen.getByLabelText('플로우 위치'), {
      target: { value: 'node-a' },
    });
    const flowSelectRemote = screen.getByLabelText('참조 플로우') as HTMLSelectElement;
    const remoteOpts = (
      within(flowSelectRemote).getAllByRole('option') as HTMLOptionElement[]
    ).map((o) => o.value);
    expect(remoteOpts).toContain('lf1');
  });
});

describe('FlowPickerInput — 회귀: 동일노드 원격 편집 / 로컬 전용 환경', () => {
  it('동일노드 원격 편집(target=remote)은 노드 선택기를 노출하지 않는다', () => {
    useTargetContextMock.mockReturnValue({ type: 'remote', instanceId: 'node-a' });
    renderPicker({ value: '', onChange: vi.fn() });
    expect(screen.queryByLabelText('플로우 위치')).toBeNull();
    // 플로우 select 는 그대로 존재(그 노드의 플로우 = useFlowsTarget 위임).
    expect(screen.getByLabelText('참조 플로우')).toBeInTheDocument();
  });

  it('non-server 모드(원격 관리 비활성)는 노드 선택기를 숨긴다(로컬 전용 회귀)', () => {
    useRemoteModeMock.mockReturnValue({ data: { mode: 'disabled' } });
    useManagedNodesMock.mockReturnValue({ data: undefined });
    renderPicker({ value: '', onChange: vi.fn() });
    expect(screen.queryByLabelText('플로우 위치')).toBeNull();
  });

  it('server 모드라도 승인+온라인 원격 노드가 없으면 노드 선택기를 숨긴다', () => {
    useManagedNodesMock.mockReturnValue({ data: [] });
    renderPicker({ value: '', onChange: vi.fn() });
    expect(screen.queryByLabelText('플로우 위치')).toBeNull();
    // 로컬 플로우 선택은 그대로 동작.
    expect(screen.getByLabelText('참조 플로우')).toBeInTheDocument();
  });
});
