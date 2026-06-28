// useEditorFlowTarget 단위 테스트 (SPEC-REMOTE-001 M7, REQ-I08).
//
// EditorPage 의 로컬/원격 저장 대상 추상화를 검증한다:
//   - 로컬: useUpdateFlow(PUT) 위임, isRemote=false.
//   - 원격 수정: updateRemoteFlow(PATCH) 위임 + 시크릿 생략(REQ-I07).
//   - 원격 신규: createRemoteFlow(POST) 위임 + 노드 채번 id 반환.
//   - 원격 로드: 미러에서 flowId 매칭 → 정의 파싱.

import { renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { MirroredResource } from '@/types/remote';

// ---- 의존 훅 mock ----
const useFlowMock = vi.hoisted(() => vi.fn());
const updateLocalMutateAsyncMock = vi.hoisted(() => vi.fn());
const useNodeMirrorMock = vi.hoisted(() => vi.fn());
const createRemoteMutateAsyncMock = vi.hoisted(() => vi.fn());
const updateRemoteMutateAsyncMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useFlow', () => ({
  useFlow: useFlowMock,
  useUpdateFlow: () => ({
    mutateAsync: updateLocalMutateAsyncMock,
    isPending: false,
  }),
}));

vi.mock('@/hooks/useRemote', () => ({
  useNodeMirror: useNodeMirrorMock,
  useCreateRemoteFlow: () => ({
    mutateAsync: createRemoteMutateAsyncMock,
    isPending: false,
  }),
  useUpdateRemoteFlow: () => ({
    mutateAsync: updateRemoteMutateAsyncMock,
    isPending: false,
  }),
}));

import { useEditorFlowTarget } from './useEditorFlowTarget';

function makeMirror(overrides: Partial<MirroredResource> = {}): MirroredResource {
  return {
    id: 'f-1',
    source_instance_id: 'node-a',
    name: 'Flow 1',
    kind: 'flow',
    updated_at: 0,
    online: true,
    ...overrides,
  };
}

beforeEach(() => {
  useFlowMock.mockReset();
  updateLocalMutateAsyncMock.mockReset();
  useNodeMirrorMock.mockReset();
  createRemoteMutateAsyncMock.mockReset();
  updateRemoteMutateAsyncMock.mockReset();

  useFlowMock.mockReturnValue({ data: undefined, isLoading: false, error: null });
  useNodeMirrorMock.mockReturnValue({ data: [], isLoading: false, error: null });
});

describe('useEditorFlowTarget — 로컬 편집', () => {
  it('instanceId 가 없으면 로컬(isRemote=false)이며 PUT 으로 저장한다', async () => {
    useFlowMock.mockReturnValue({
      data: { id: 'f-local', name: 'Local', config: { nodes: [] } },
      isLoading: false,
      error: null,
    });
    updateLocalMutateAsyncMock.mockResolvedValue({ id: 'f-local' });

    const { result } = renderHook(() =>
      useEditorFlowTarget({ flowId: 'f-local' }),
    );

    expect(result.current.isRemote).toBe(false);
    expect(result.current.flowData).toEqual({
      id: 'f-local',
      name: 'Local',
      config: { nodes: [] },
    });

    const res = await result.current.save({ nodes: [{ id: 'n1' }] }, 'Local');
    expect(updateLocalMutateAsyncMock).toHaveBeenCalledWith({
      id: 'f-local',
      req: { definition: { nodes: [{ id: 'n1' }] } },
    });
    expect(res).toEqual({ id: 'f-local' });
    // 원격 경로는 호출되지 않는다.
    expect(updateRemoteMutateAsyncMock).not.toHaveBeenCalled();
  });
});

describe('useEditorFlowTarget — 원격 수정', () => {
  it('미러에서 flowId 정의를 파싱해 로드한다', () => {
    useNodeMirrorMock.mockReturnValue({
      data: [
        makeMirror({
          id: 'f-1',
          name: 'Remote Flow',
          definition: JSON.stringify({ nodes: [{ id: 'rn1' }], edges: [] }),
        }),
      ],
      isLoading: false,
      error: null,
    });

    const { result } = renderHook(() =>
      useEditorFlowTarget({ flowId: 'f-1', instanceId: 'node-a' }),
    );

    expect(result.current.isRemote).toBe(true);
    expect(result.current.flowData).toEqual({
      id: 'f-1',
      name: 'Remote Flow',
      config: { nodes: [{ id: 'rn1' }], edges: [] },
    });
  });

  it('PATCH 로 저장하며 마스킹 시크릿을 생략한다 (REQ-I07)', async () => {
    useNodeMirrorMock.mockReturnValue({
      data: [makeMirror({ id: 'f-1', definition: '{}' })],
      isLoading: false,
      error: null,
    });
    updateRemoteMutateAsyncMock.mockResolvedValue({ id: 'f-1' });

    const { result } = renderHook(() =>
      useEditorFlowTarget({ flowId: 'f-1', instanceId: 'node-a' }),
    );

    // 정의 내부에 빈 password 가 있으면 생략되어야 한다.
    await result.current.save(
      { nodes: [{ id: 'n1', config: { host: 'h', password: '' } }] },
      'Remote Flow',
    );

    expect(updateRemoteMutateAsyncMock).toHaveBeenCalledTimes(1);
    const arg = updateRemoteMutateAsyncMock.mock.calls[0]![0];
    expect(arg.instanceID).toBe('node-a');
    expect(arg.flowID).toBe('f-1');
    expect(arg.req.definition).toEqual({
      nodes: [{ id: 'n1', config: { host: 'h' } }],
    });
    expect(updateLocalMutateAsyncMock).not.toHaveBeenCalled();
  });
});

describe('useEditorFlowTarget — 원격 신규 생성', () => {
  it('isNew 면 빈 정의로 시작하고 POST 로 생성해 채번 id 를 반환한다', async () => {
    createRemoteMutateAsyncMock.mockResolvedValue({ id: 'f-new', name: '', status: '' });

    const { result } = renderHook(() =>
      useEditorFlowTarget({ instanceId: 'node-a', isNew: true }),
    );

    expect(result.current.isRemote).toBe(true);
    expect(result.current.flowData).toEqual({ id: '', name: '', config: {} });

    const res = await result.current.save({ nodes: [] }, 'New Flow');
    expect(createRemoteMutateAsyncMock).toHaveBeenCalledWith({
      instanceID: 'node-a',
      req: { name: 'New Flow', definition: { nodes: [] } },
    });
    expect(res).toEqual({ id: 'f-new' });
  });
});
