// CustomNode output 노드 ON/OFF 라이브 제어 테스트.
//
// toggleOutput 은 (1) 에디터 상태(updateNodeData) 를 갱신하고, (2) currentFlowId
// 가 있으면 configureNode 로 실행 중 플로우에 즉시 적용한다. 404(실행 중 아님)
// 거부는 throw 되지 않고 에디터 상태 갱신은 그대로 유지되어야 한다.

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

const configureNodeMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/nodeService', () => ({
  configureNode: configureNodeMock,
}));

import { APIError } from '@/types/api';
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
      <CustomNode {...props} />
    </MemoryRouter>,
  );
}

describe('CustomNode output ON/OFF 라이브 제어', () => {
  beforeEach(() => {
    configureNodeMock.mockReset();
    useEditorStore.getState().resetEditor();
    useUIStore.getState().clearNotifications();
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
