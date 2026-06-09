// CustomNode output 노드 ON/OFF 라이브 제어 테스트.
//
// toggleOutput 은 (1) 에디터 상태(updateNodeData) 를 갱신하고, (2) currentFlowId
// 가 있으면 configureNode 로 실행 중 플로우에 즉시 적용한다. 404(실행 중 아님)
// 거부는 throw 되지 않고 에디터 상태 갱신은 그대로 유지되어야 한다.

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

const configureNodeMock = vi.hoisted(() => vi.fn());
const useManagedNodesMock = vi.hoisted(() => vi.fn());
const useRemoteModeMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/nodeService', () => ({
  configureNode: configureNodeMock,
}));

// 원격 브릿지 표시명({hostname}.{flowName}) 해석에 쓰이는 원격 훅을 모킹한다.
// CustomNode 는 이 훅들을 무조건 호출(enabled 게이팅)하므로 QueryClientProvider
// 없이도 렌더되도록 반환값을 직접 제어한다.
vi.mock('@/hooks/useRemote', () => ({
  useManagedNodes: (...args: unknown[]) => useManagedNodesMock(...args),
  useRemoteMode: () => useRemoteModeMock(),
}));

import { APIError } from '@/types/api';
import { I18nProvider } from '@/lib/i18n';
import {
  RuntimeStatsContext,
  type NodeRuntimeStats,
} from '@/contexts/RuntimeStatsContext';
import { useEditorStore } from '@/stores/editorStore';
import { useUIStore } from '@/stores/uiStore';
import { CustomNode } from './CustomNode';

const NODE_ID = 'out-1';

/** output 노드를 에디터 스토어에 심고 CustomNode 를 렌더한다. */
function renderOutputNode(outputEnabled: boolean) {
  // 스토어에 노드를 등록해 updateNodeData 가 반영될 대상이 존재하도록 한다.
  useEditorStore.setState({
    nodes: [
      {
        id: NODE_ID,
        type: 'custom',
        position: { x: 0, y: 0 },
        data: {
          label: 'sink',
          nodeType: 'output',
          category: 'io',
          output_enabled: outputEnabled,
        },
      },
    ],
  });

  const props = {
    id: NODE_ID,
    data: {
      label: 'sink',
      nodeType: 'output',
      category: 'io',
      output_enabled: outputEnabled,
    },
    selected: false,
  } as unknown as React.ComponentProps<typeof CustomNode>;

  return render(
    <MemoryRouter>
      <I18nProvider>
        <CustomNode {...props} />
      </I18nProvider>
    </MemoryRouter>,
  );
}

/** flow-node 를 (선택적 런타임 state 와 함께) 렌더한다 — 원격 브릿지 인디케이터 검증용. */
function renderFlowNode(
  flowId: string,
  opts: { flowName?: string; runtimeState?: string } = {},
) {
  const data = {
    label: 'sub',
    nodeType: 'flow-node',
    category: 'special',
    flow_id: flowId,
    ...(opts.flowName ? { flow_name: opts.flowName } : {}),
  };
  useEditorStore.setState({
    nodes: [
      {
        id: NODE_ID,
        type: 'custom',
        position: { x: 0, y: 0 },
        data,
      },
    ],
  });
  const props = {
    id: NODE_ID,
    data,
    selected: false,
  } as unknown as React.ComponentProps<typeof CustomNode>;

  const statsMap: Record<string, NodeRuntimeStats> =
    opts.runtimeState !== undefined
      ? {
          [NODE_ID]: {
            inMessages: 0,
            outMessages: 0,
            state: opts.runtimeState,
            ports: [],
          },
        }
      : {};

  return render(
    <MemoryRouter>
      <I18nProvider>
        <RuntimeStatsContext.Provider value={statsMap}>
          <CustomNode {...props} />
        </RuntimeStatsContext.Provider>
      </I18nProvider>
    </MemoryRouter>,
  );
}

describe('CustomNode output ON/OFF 라이브 제어', () => {
  beforeEach(() => {
    configureNodeMock.mockReset();
    useEditorStore.getState().resetEditor();
    useUIStore.getState().clearNotifications();
    // output 노드 렌더에서도 원격 훅이 (게이트되어) 호출되므로 안전한 기본값을 둔다.
    useManagedNodesMock.mockReset().mockReturnValue({ data: [] });
    useRemoteModeMock.mockReset().mockReturnValue({ data: { mode: 'disabled' } });
  });

  it('currentFlowId 가 있으면 toggleOutput 이 configureNode 를 next 값으로 호출한다', async () => {
    configureNodeMock.mockResolvedValueOnce(undefined);
    useEditorStore.getState().setCurrentFlowId('flow-1');

    renderOutputNode(true); // 현재 ON → 클릭 시 next=false

    fireEvent.click(screen.getByRole('button', { name: '출력 비활성화' }));

    await waitFor(() => {
      expect(configureNodeMock).toHaveBeenCalledWith('flow-1', NODE_ID, {
        output_enabled: false,
      });
    });

    // 에디터 상태도 next 값으로 갱신된다.
    const node = useEditorStore.getState().nodes.find((n) => n.id === NODE_ID);
    expect(node?.data.output_enabled).toBe(false);
  });

  it('currentFlowId 가 없으면 configureNode 를 호출하지 않고 에디터 상태만 갱신한다', () => {
    // resetEditor 로 currentFlowId 는 null 인 상태.
    renderOutputNode(false); // 현재 OFF → 클릭 시 next=true

    fireEvent.click(screen.getByRole('button', { name: '출력 활성화' }));

    expect(configureNodeMock).not.toHaveBeenCalled();
    const node = useEditorStore.getState().nodes.find((n) => n.id === NODE_ID);
    expect(node?.data.output_enabled).toBe(true);
  });

  it('404(실행 중 아님) 거부는 throw 되지 않고 알림도 띄우지 않으며 에디터 상태는 유지된다', async () => {
    configureNodeMock.mockRejectedValueOnce(
      new APIError('NOT_FOUND', 'not running', 404),
    );
    useEditorStore.getState().setCurrentFlowId('flow-1');

    renderOutputNode(true);

    fireEvent.click(screen.getByRole('button', { name: '출력 비활성화' }));

    await waitFor(() => {
      expect(configureNodeMock).toHaveBeenCalledTimes(1);
    });

    // 에디터 상태는 그대로 갱신 (다음 배포에서 반영).
    const node = useEditorStore.getState().nodes.find((n) => n.id === NODE_ID);
    expect(node?.data.output_enabled).toBe(false);
    // 404 는 조용히 무시 — 알림 없음.
    expect(useUIStore.getState().notifications).toHaveLength(0);
  });

  it('404 가 아닌 오류는 경고 알림을 띄운다', async () => {
    configureNodeMock.mockRejectedValueOnce(
      new APIError('INTERNAL', 'boom', 500),
    );
    useEditorStore.getState().setCurrentFlowId('flow-1');

    renderOutputNode(true);

    fireEvent.click(screen.getByRole('button', { name: '출력 비활성화' }));

    await waitFor(() => {
      expect(useUIStore.getState().notifications).toHaveLength(1);
    });
    expect(useUIStore.getState().notifications[0]?.type).toBe('warning');
  });
});

// SPEC-SUBFLOW-001 v1.3 (REQ-RU06): flow-node 원격 브릿지 인디케이터.
describe('CustomNode flow-node 원격 브릿지 인디케이터', () => {
  beforeEach(() => {
    useEditorStore.getState().resetEditor();
    useUIStore.getState().clearNotifications();
    // 기본: 관리 노드 없음(호스트명 미해석) + server 모드.
    useManagedNodesMock.mockReset().mockReturnValue({ data: [] });
    useRemoteModeMock.mockReset().mockReturnValue({ data: { mode: 'server' } });
  });

  it('hostname 과 flow_name 이 있으면 `{hostname}.{flowName}` 표시명을 렌더한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [{ instance_id: 'inst-uuid-1234', hostname: 'xagent04' }],
    });
    renderFlowNode('remote://inst-uuid-1234/flow-9', {
      flowName: 'HVACR Control',
    });

    const indicator = screen.getByText('원격 브릿지').closest('[data-remote-bridge]');
    expect(indicator).not.toBeNull();
    // 해석된 표시명: hostname.flowName.
    expect(screen.getByText('· xagent04.HVACR Control')).toBeInTheDocument();
    // 전체 표시명이 툴팁(title)에도 포함된다(절단 시 호버로 확인 가능).
    expect(indicator?.getAttribute('title')).toContain(
      'xagent04.HVACR Control',
    );
  });

  it('hostname 미해석 시 단축 instanceId 로, flow_name 부재 시 단축 flowId 로 폴백한다', () => {
    // 관리 노드 없음(기본) → hostname 폴백; flow_name 미지정 → flowId 폴백.
    renderFlowNode('remote://inst-uuid-1234/flow-uuid-5678');

    const indicator = screen.getByText('원격 브릿지').closest('[data-remote-bridge]');
    expect(indicator).not.toBeNull();
    // hostname 폴백: 단축 instanceId(앞 8자 + 생략부호) = "inst-uui…".
    // flowId 폴백: 단축 flowId(앞 8자 + 생략부호) = "flow-uui…".
    expect(screen.getByText('· inst-uui….flow-uui…')).toBeInTheDocument();
  });

  it('hostname 은 있으나 flow_name 이 없으면 `{hostname}.{단축 flowId}` 로 표시한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [{ instance_id: 'inst-uuid-1234', hostname: 'xagent04' }],
    });
    renderFlowNode('remote://inst-uuid-1234/flow-9');

    // flow-9 는 8자 이하라 단축되지 않는다.
    expect(screen.getByText('· xagent04.flow-9')).toBeInTheDocument();
  });

  it('평문(local) flow_id 면 원격 브릿지 인디케이터를 표시하지 않는다', () => {
    renderFlowNode('flow-9', { flowName: '로컬 서브플로우' });

    expect(screen.queryByText('원격 브릿지')).toBeNull();
    expect(
      document.querySelector('[data-remote-bridge]'),
    ).toBeNull();
    // 로컬 서브플로우 표시(참조 플로우 이름)는 그대로 유지된다.
    expect(screen.getByText('로컬 서브플로우')).toBeInTheDocument();
  });

  it('런타임 state 가 없으면(플로우 미실행) 상태 점 없이 정적 인디케이터만 표시한다', () => {
    renderFlowNode('remote://inst-uuid-1234/flow-9');

    expect(screen.getByText('원격 브릿지')).toBeInTheDocument();
    expect(document.querySelector('[data-bridge-status]')).toBeNull();
  });

  it('런타임 state=running 이면 running 상태 점을 반영한다', () => {
    renderFlowNode('remote://inst-uuid-1234/flow-9', {
      runtimeState: 'running',
    });

    const dot = document.querySelector('[data-bridge-status]');
    expect(dot).not.toBeNull();
    expect(dot?.getAttribute('data-bridge-status')).toBe('running');
  });

  it('런타임 state=error 이면 error 상태 점을 반영한다', () => {
    renderFlowNode('remote://inst-uuid-1234/flow-9', {
      runtimeState: 'error',
    });

    const dot = document.querySelector('[data-bridge-status]');
    expect(dot?.getAttribute('data-bridge-status')).toBe('error');
  });
});
