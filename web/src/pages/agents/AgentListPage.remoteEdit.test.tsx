// AgentListPage — 원격 에이전트 설정 편집 어포던스 테스트
// (SPEC-REMOTE-001 M8/M7, 그룹 J/I, REQ-I04/I07/J05).
//
// 범위:
//   - 원격 타깃: 행 액션 영역에 설정 편집 버튼을 노출한다(로컬은 미노출).
//   - 편집 버튼 클릭 → RemoteAgentEditDialog(update) 가 현재 이름/종류/config 로
//     프리필되어 열린다.
//   - 저장 → useUpdateRemoteAgent 가 마스킹 시크릿이 생략된 config 로 호출된다.
//   - 노드 미-ready(승인/온라인 아님) → 편집 버튼이 비활성된다(REQ-J05 게이팅).
//
// 데이터/게이팅/뮤테이션 훅과 무거운 하위 컴포넌트는 스텁으로 격리한다.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ResourceTarget } from '@/lib/remote/target';
import type { AgentInfo } from '@/types/agent';

const REMOTE_TARGET: ResourceTarget = { type: 'remote', instanceId: 'node-a' };
const LOCAL_TARGET: ResourceTarget = { type: 'local' };

// --- mutation 스파이 ---
const updateMutateAsync = vi.hoisted(() => vi.fn());
const createMutateAsync = vi.hoisted(() => vi.fn());

// 라이브 에이전트. config 에는 비-시크릿 필드(host)와, 사용자가 새 값을 입력하지
// 않은(빈 문자열) 시크릿 필드(password)를 둔다 → 저장 시 omitMaskedSecrets 가
// password 를 생략하고 host 는 유지해야 한다(REQ-I07).
const REMOTE_AGENT: AgentInfo = {
  id: 'a-1',
  name: 'Remote Agent',
  type: 'mqtt-client',
  status: 'running',
  connected: true,
  uptime: '5m',
  config: { host: 'broker.local', password: '' },
  stats: undefined,
};

const useAgentsTargetMock = vi.hoisted(() => vi.fn());
const useTargetGatingMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useResourceTargets', () => ({
  useAgentsTarget: useAgentsTargetMock,
}));
vi.mock('@/hooks/useTargetGating', () => ({
  useTargetGating: useTargetGatingMock,
}));
vi.mock('@/hooks/useTargetParam', () => ({
  useTargetParam: () => ({ type: 'local' }),
}));
vi.mock('@/hooks/useRemote', () => ({
  useCreateRemoteAgent: () => ({ isPending: false, mutateAsync: createMutateAsync }),
  useUpdateRemoteAgent: () => ({ isPending: false, mutateAsync: updateMutateAsync }),
}));
vi.mock('@/hooks/useAgent', () => ({
  useUpdateAgent: () => ({ isPending: false, mutate: vi.fn() }),
  // AgentActionButtons / 기타 하위가 참조하는 훅들(스텁).
  useEnableAgent: () => ({ isPending: false, mutateAsync: vi.fn() }),
  useStartAgent: () => ({ isPending: false, mutateAsync: vi.fn() }),
}));

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

// 마스킹 시크릿 생략은 실제 구현을 사용(저장 페이로드 검증).
// (omitMaskedSecrets / TargetProvider 는 실제 모듈 사용)

vi.mock('@/components/remote/RemoteTargetBanner', () => ({
  RemoteTargetBanner: () => <div data-testid="remote-target-banner" />,
}));
vi.mock('@/components/common/ImportDialog', () => ({ default: () => null }));
vi.mock('./CreateAgentModal', () => ({ default: () => null }));
vi.mock('./AgentSearchFilter', () => ({ default: () => null }));
vi.mock('./AgentDetailPanel', () => ({ default: () => null }));
vi.mock('./AgentActionButtons', () => ({ default: () => <div data-testid="agent-action-buttons" /> }));
vi.mock('./AgentEnabledBadge', () => ({ default: () => null }));

// uiStore 알림 스텁.
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector: (s: { addNotification: () => void; dashboardRefreshInterval: number }) => unknown) =>
    selector({ addNotification: vi.fn(), dashboardRefreshInterval: 0 }),
}));

import AgentListPage from './AgentListPage';

function renderPage(target: ResourceTarget) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <AgentListPage target={target} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  useAgentsTargetMock.mockReturnValue({
    data: { data: [REMOTE_AGENT], total: 1 },
    isLoading: false,
    error: null,
    refetch: vi.fn(),
    isRemote: true,
  });
  useTargetGatingMock.mockReturnValue({
    isRemote: true,
    nodeReady: true,
    nodeLabel: 'gw-1',
    canControl: () => true,
  });
});

describe('AgentListPage — 원격 설정 편집 버튼', () => {
  it('원격 타깃에서 편집 버튼이 노출된다', () => {
    renderPage(REMOTE_TARGET);
    expect(screen.getByTestId('remote-agent-edit-button')).toBeInTheDocument();
  });

  it('로컬 타깃에서는 편집 버튼이 노출되지 않는다', () => {
    useTargetGatingMock.mockReturnValue({
      isRemote: false,
      nodeReady: true,
      nodeLabel: undefined,
      canControl: () => true,
    });
    renderPage(LOCAL_TARGET);
    expect(screen.queryByTestId('remote-agent-edit-button')).not.toBeInTheDocument();
  });

  it('노드 미-ready 면 편집 버튼이 비활성된다', () => {
    useTargetGatingMock.mockReturnValue({
      isRemote: true,
      nodeReady: false,
      nodeLabel: 'gw-1',
      canControl: () => false,
    });
    renderPage(REMOTE_TARGET);
    const btn = screen.getByTestId('remote-agent-edit-button');
    expect(btn).toBeDisabled();
  });

  it('편집 버튼 클릭 시 update 다이얼로그가 현재 값으로 프리필되어 열린다', () => {
    renderPage(REMOTE_TARGET);
    fireEvent.click(screen.getByTestId('remote-agent-edit-button'));
    expect(screen.getByTestId('remote-agent-edit-dialog')).toBeInTheDocument();
    // 이름/종류 프리필.
    const nameInput = screen.getByTestId('remote-agent-name') as HTMLInputElement;
    const typeInput = screen.getByTestId('remote-agent-type') as HTMLInputElement;
    expect(nameInput.value).toBe('Remote Agent');
    expect(typeInput.value).toBe('mqtt-client');
    // 종류는 update 모드에서 읽기 전용.
    expect(typeInput).toBeDisabled();
    // config 프리필(JSON 텍스트에 host 포함).
    const configArea = screen.getByTestId('remote-agent-config') as HTMLTextAreaElement;
    expect(configArea.value).toContain('broker.local');
  });

  it('저장 시 useUpdateRemoteAgent 를 마스킹 시크릿이 생략된 config 로 호출한다', async () => {
    updateMutateAsync.mockResolvedValueOnce({ id: 'a-1' });
    renderPage(REMOTE_TARGET);
    fireEvent.click(screen.getByTestId('remote-agent-edit-button'));
    await act(async () => {
      fireEvent.submit(screen.getByTestId('remote-agent-edit-submit').closest('form')!);
    });

    // mutateAsync 호출 페이로드 검증.
    expect(updateMutateAsync).toHaveBeenCalledTimes(1);
    const arg = updateMutateAsync.mock.calls[0]![0] as {
      instanceID: string;
      agentID: string;
      req: { name?: string; config?: Record<string, unknown> };
    };
    expect(arg.instanceID).toBe('node-a');
    expect(arg.agentID).toBe('a-1');
    expect(arg.req.name).toBe('Remote Agent');
    // 비시크릿 필드는 유지된다.
    expect(arg.req.config?.host).toBe('broker.local');
    // 사용자가 새 값을 입력하지 않은(빈) 시크릿 필드는 페이로드에서 생략된다 → 노드 backfill.
    expect(arg.req.config && 'password' in arg.req.config).toBe(false);
  });
});
