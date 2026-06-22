// AgentListPage — 원격 에이전트 생성 어포던스 + 행 업데이트 제거 회귀 테스트
// (SPEC-REMOTE-001 review, 그룹 I/J, REQ-I04/I07/J05).
//
// 범위:
//   - 원격 타깃: "새 에이전트" 생성은 RemoteAgentEditDialog(create) 로 수행한다.
//   - 행 액션 영역의 설정 편집(update) 버튼/다이얼로그는 제거되었다(상세 패널의
//     설정 탭 인라인 편집으로 대체). 행에 update 어포던스가 없어야 한다.
//   - 생성 저장 → useCreateRemoteAgent 가 마스킹 시크릿이 생략된 config 로 호출된다.
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
const createMutateAsync = vi.hoisted(() => vi.fn());

// 라이브 에이전트. config 에는 비-시크릿 필드(host)와, 사용자가 새 값을 입력하지
// 않은(빈 문자열) 시크릿 필드(password)를 둔다 → 생성 저장 시 omitMaskedSecrets 가
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
// (omitMaskedSecrets / TargetProvider / RemoteAgentEditDialog 는 실제 모듈 사용)

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

describe('AgentListPage — 행 업데이트 어포던스 제거(회귀)', () => {
  it('원격 타깃에서 행 설정 편집(update) 버튼이 더 이상 노출되지 않는다', () => {
    renderPage(REMOTE_TARGET);
    // 설정 수정은 상세 패널의 설정 탭에서 인라인으로 수행하므로 행 버튼은 없어야 한다.
    expect(screen.queryByTestId('remote-agent-edit-button')).not.toBeInTheDocument();
  });

  it('로컬 타깃에서도 행 설정 편집 버튼이 노출되지 않는다', () => {
    useTargetGatingMock.mockReturnValue({
      isRemote: false,
      nodeReady: true,
      nodeLabel: undefined,
      canControl: () => true,
    });
    renderPage(LOCAL_TARGET);
    expect(screen.queryByTestId('remote-agent-edit-button')).not.toBeInTheDocument();
  });
});

describe('AgentListPage — 원격 에이전트 생성(유지)', () => {
  it('"새 에이전트" 클릭 시 create 다이얼로그가 열린다', () => {
    renderPage(REMOTE_TARGET);
    fireEvent.click(screen.getByText('agents.newAgent'));
    expect(screen.getByTestId('remote-agent-edit-dialog')).toBeInTheDocument();
    // create 모드: 종류 입력이 활성(편집 가능)이어야 한다.
    const typeInput = screen.getByTestId('remote-agent-type') as HTMLInputElement;
    expect(typeInput).not.toBeDisabled();
  });

  it('생성 저장 시 useCreateRemoteAgent 를 마스킹 시크릿이 생략된 config 로 호출한다', async () => {
    createMutateAsync.mockResolvedValueOnce({ id: 'a-new' });
    renderPage(REMOTE_TARGET);
    fireEvent.click(screen.getByText('agents.newAgent'));

    // 필수 입력(이름/종류) 채우기 + config 에 비시크릿+빈 시크릿 작성.
    fireEvent.change(screen.getByTestId('remote-agent-name'), {
      target: { value: 'New Agent' },
    });
    fireEvent.change(screen.getByTestId('remote-agent-type'), {
      target: { value: 'mqtt-client' },
    });
    fireEvent.change(screen.getByTestId('remote-agent-config'), {
      target: { value: JSON.stringify({ host: 'broker.local', password: '' }) },
    });

    await act(async () => {
      fireEvent.submit(screen.getByTestId('remote-agent-edit-submit').closest('form')!);
    });

    expect(createMutateAsync).toHaveBeenCalledTimes(1);
    const arg = createMutateAsync.mock.calls[0]![0] as {
      instanceID: string;
      req: { name: string; type: string; config?: Record<string, unknown> };
    };
    expect(arg.instanceID).toBe('node-a');
    expect(arg.req.name).toBe('New Agent');
    expect(arg.req.type).toBe('mqtt-client');
    // 비시크릿 필드는 유지된다.
    expect(arg.req.config?.host).toBe('broker.local');
    // 사용자가 새 값을 입력하지 않은(빈) 시크릿 필드는 페이로드에서 생략된다 → 노드 backfill.
    expect(arg.req.config && 'password' in arg.req.config).toBe(false);
  });
});
