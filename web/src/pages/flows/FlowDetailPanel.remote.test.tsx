// FlowDetailPanel 원격 타깃 렌더링 테스트 (SPEC-REMOTE-001 M8, REQ-J10/J11).
//
// 동일한 상세 패널이 원격 타깃(TargetProvider)에서 READ 프록시 데이터를 받아
// 로컬과 동일하게 노드 목록을 렌더링하는지, 그리고 원격에서는 로그 레벨(쓰기)
// 컬럼이 숨겨지는지 검증한다.

import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { TargetProvider } from '@/lib/remote/TargetProvider';
import { LOCAL_TARGET } from '@/lib/remote/target';
import type { FlowNodeInfo, FlowStatusInfo } from '@/types/flow';

// 상세 타깃 훅을 mock 하여 원격/로컬 데이터를 주입한다.
const useFlowStatusTargetMock = vi.hoisted(() => vi.fn());
const useFlowNodesTargetMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useDetailTargets', () => ({
  useFlowStatusTarget: useFlowStatusTargetMock,
  useFlowNodesTarget: useFlowNodesTargetMock,
}));

// i18n / monitorService 는 본 패널이 로그레벨 셀렉트에서만 쓰며, 원격에서는
// 렌더되지 않으므로 가벼운 stub 으로 둔다.
vi.mock('@/services/api/monitorService', () => ({
  getLogLevels: () => Promise.resolve({ components: {} }),
  setComponentLogLevel: vi.fn(),
  resetComponentLogLevel: vi.fn(),
}));
vi.mock('@/stores/uiStore', () => ({
  useUIStore: () => vi.fn(),
}));
// i18n 은 키를 그대로 반환하는 stub 으로 둔다 (단언은 키로 검증).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import FlowDetailPanel from './FlowDetailPanel';

function makeNode(o: Partial<FlowNodeInfo> = {}): FlowNodeInfo {
  return {
    node_id: 'n1',
    name: 'Inject',
    type: 'inject',
    state: 'running',
    ports: [],
    ...o,
  } as FlowNodeInfo;
}

beforeEach(() => {
  useFlowStatusTargetMock.mockReset().mockReturnValue({
    data: { status: 'running' } as FlowStatusInfo,
    isLoading: false,
    error: null,
  });
  useFlowNodesTargetMock.mockReset().mockReturnValue({
    data: [makeNode()],
    isLoading: false,
    error: null,
  });
});

describe('FlowDetailPanel — 원격 타깃 렌더링', () => {
  it('원격 타깃에서 READ 프록시 노드 데이터를 렌더링한다', () => {
    render(
      <TargetProvider target={{ type: 'remote', instanceId: 'node-a' }}>
        <FlowDetailPanel flowId="f1" />
      </TargetProvider>,
    );
    // 타깃 훅이 원격 타깃으로 호출된다.
    expect(useFlowNodesTargetMock).toHaveBeenCalledWith(
      { type: 'remote', instanceId: 'node-a' },
      'f1',
      3000,
    );
    // 노드 이름이 렌더링된다(로컬과 동일).
    expect(screen.getByText('Inject')).toBeInTheDocument();
  });

  it('원격 타깃에서는 로그 레벨(쓰기) 컬럼을 숨긴다', () => {
    render(
      <TargetProvider target={{ type: 'remote', instanceId: 'node-a' }}>
        <FlowDetailPanel flowId="f1" />
      </TargetProvider>,
    );
    expect(screen.queryByText('flows.detail.logLevel')).not.toBeInTheDocument();
  });

  it('로컬 타깃에서는 로그 레벨 컬럼을 표시한다(회귀 없음)', () => {
    render(
      <TargetProvider target={LOCAL_TARGET}>
        <FlowDetailPanel flowId="f1" />
      </TargetProvider>,
    );
    expect(useFlowNodesTargetMock).toHaveBeenCalledWith(LOCAL_TARGET, 'f1', 3000);
    expect(screen.getByText('flows.detail.logLevel')).toBeInTheDocument();
  });
});
