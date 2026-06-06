// FlowActionMenu 타깃 인지 동작 테스트 (SPEC-REMOTE-001 M8, 그룹 J).
//
// 검증 항목:
//   - 로컬 타깃: 모든 액션 버튼이 상태 기반으로 활성/비활성된다(회귀 없음).
//   - 원격 타깃 + 노드 미제어(오프라인): 모든 라이프사이클 버튼이 비활성된다.
//   - 원격 미지원 액션(restart/undeploy): 노드가 ready 여도 비활성된다.
//   - 원격 start 클릭 시 perform('start', id) 가 호출된다.

import { render, screen, fireEvent } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { TargetProvider } from '@/lib/remote/TargetContext';
import { LOCAL_TARGET, type ResourceTarget } from '@/lib/remote/target';
import type { FlowInfo } from '@/types/flow';

// 액션 훅/게이팅 훅을 모킹하여 라우팅/비활성 동작만 검증한다.
const useFlowActionsTargetMock = vi.hoisted(() => vi.fn());
const useTargetGatingMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useResourceActions', () => ({
  useFlowActionsTarget: useFlowActionsTargetMock,
}));
vi.mock('@/hooks/useTargetGating', () => ({
  useTargetGating: useTargetGatingMock,
}));
vi.mock('@/stores/uiStore', () => ({
  useUIStore: () => vi.fn(),
}));
// t 가 키를 그대로 반환하도록 모킹(원격 안내 키를 제목으로 검증).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import FlowActionMenu from './FlowActionMenu';

const REMOTE: ResourceTarget = { type: 'remote', instanceId: 'node-1' };

function flow(o: Partial<FlowInfo> = {}): FlowInfo {
  return { id: 'f1', name: 'Flow 1', status: 'running', node_count: 0, ...o } as FlowInfo;
}

function makeHandle(over: Partial<ReturnType<typeof baseHandle>> = {}) {
  return { ...baseHandle(), ...over };
}
function baseHandle() {
  return {
    isRemote: false,
    perform: vi.fn().mockResolvedValue(undefined),
    supports: (_a: string) => true,
    pending: {} as Record<string, boolean>,
  };
}

function gating(canControl: boolean, nodeReady = canControl) {
  return { isRemote: true, nodeReady, nodeLabel: 'gw', canControl: () => canControl };
}

beforeEach(() => {
  useFlowActionsTargetMock.mockReset();
  useTargetGatingMock.mockReset().mockReturnValue({
    isRemote: false,
    nodeReady: true,
    nodeLabel: undefined,
    canControl: () => true,
  });
});

function renderMenu(target: ResourceTarget, f: FlowInfo) {
  return render(
    <TargetProvider target={target}>
      <FlowActionMenu flow={f} />
    </TargetProvider>,
  );
}

describe('FlowActionMenu — 로컬', () => {
  it('실행 중 플로우는 중지/재시작이 활성, 시작은 비활성', () => {
    useFlowActionsTargetMock.mockReturnValue(makeHandle({ isRemote: false }));
    renderMenu(LOCAL_TARGET, flow({ status: 'running' }));
    expect(screen.getByTitle('시작')).toBeDisabled();
    expect(screen.getByTitle('중지')).not.toBeDisabled();
    expect(screen.getByTitle('재시작')).not.toBeDisabled();
    // 로컬은 내보내기 버튼이 노출된다.
    expect(screen.getByTitle('내보내기')).toBeInTheDocument();
  });
});

describe('FlowActionMenu — 원격(노드 오프라인)', () => {
  it('노드 미제어면 라이프사이클 버튼이 모두 비활성 + 게이트 안내', () => {
    useFlowActionsTargetMock.mockReturnValue(makeHandle({ isRemote: true }));
    useTargetGatingMock.mockReturnValue(gating(false, false));
    renderMenu(REMOTE, flow({ status: 'running' }));
    // 노드 미제어로 게이트 안내가 붙은 버튼이 1개 이상이며 모두 비활성.
    const gated = screen.getAllByTitle('remote.edit.actionGateHint');
    expect(gated.length).toBeGreaterThanOrEqual(1);
    gated.forEach((b) => expect(b).toBeDisabled());
    // 내보내기는 원격에서 숨겨진다.
    expect(screen.queryByTitle('내보내기')).not.toBeInTheDocument();
  });
});

describe('FlowActionMenu — 원격(노드 온라인)', () => {
  it('미지원 액션(재시작/배포해제)은 노드 ready 여도 비활성', () => {
    useFlowActionsTargetMock.mockReturnValue(
      makeHandle({ isRemote: true, supports: (a: string) => a !== 'restart' && a !== 'undeploy' }),
    );
    useTargetGatingMock.mockReturnValue(gating(true, true));
    renderMenu(REMOTE, flow({ status: 'running' }));
    // 미지원 버튼은 unsupportedOnRemote 안내로 비활성.
    const unsupported = screen.getAllByTitle('remote.edit.unsupportedOnRemote');
    expect(unsupported.length).toBeGreaterThanOrEqual(1);
    unsupported.forEach((b) => expect(b).toBeDisabled());
  });

  it('start 클릭 시 perform(start, id) 가 호출된다', () => {
    const handle = makeHandle({ isRemote: true });
    useFlowActionsTargetMock.mockReturnValue(handle);
    useTargetGatingMock.mockReturnValue(gating(true, true));
    // stored 상태여야 시작 버튼이 상태상 활성.
    renderMenu(REMOTE, flow({ status: 'stored' }));
    fireEvent.click(screen.getByTitle('시작'));
    expect(handle.perform).toHaveBeenCalledWith('start', 'f1');
  });
});
