// AgentDetailPanel.ConfigTab — 로컬·원격 인라인 설정 편집 분기 테스트
// (SPEC-REMOTE-001 review, 그룹 I/J, REQ-I04/I07/J05).
//
// 범위:
//   - 설정 탭에서 편집(Pencil) → 저장(Save) 인라인 UX 가 로컬·원격 동형으로 동작한다.
//   - LOCAL: 저장은 useConfigureAgent 로 호출되고, useUpdateRemoteAgent 는 호출되지 않는다.
//   - REMOTE: 저장은 useUpdateRemoteAgent 로 호출되며, 마스킹/미입력 시크릿 필드는
//     omitMaskedSecrets 로 생략된다(비시크릿 host 유지, 빈 password 생략). 로컬 경로는
//     호출되지 않는다.
//   - REMOTE: 노드가 미-ready(승인/온라인 아님)면 편집 버튼이 비활성된다(REQ-J05 게이팅).
//
// 데이터/뮤테이션/게이팅 훅과 로그레벨/시리즈 등 부수 의존은 스텁으로 격리한다.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { TargetProvider } from '@/lib/remote/TargetContext';
import type { ResourceTarget } from '@/lib/remote/target';
import type { AgentInfo } from '@/types/agent';

const REMOTE_TARGET: ResourceTarget = { type: 'remote', instanceId: 'node-a' };
const LOCAL_TARGET: ResourceTarget = { type: 'local' };

// config 에는 비-시크릿(host)과 사용자가 새 값을 입력하지 않은(빈) 시크릿(password)을 둔다.
// 원격 저장 시 omitMaskedSecrets 가 password 를 생략하고 host 는 유지해야 한다(REQ-I07).
const AGENT: AgentInfo = {
  id: 'a-1',
  name: 'mqtt-agent',
  type: 'mqtt-client',
  status: 'running',
  connected: true,
  uptime: '5m',
  config: { broker: 'tcp://broker.local:1883', password: '' },
  stats: undefined,
};

// --- mutation 스파이 ---
const configureMutateAsync = vi.hoisted(() => vi.fn());
const updateRemoteMutateAsync = vi.hoisted(() => vi.fn());
const useTargetGatingMock = vi.hoisted(() => vi.fn());

// 설정 읽기는 항상 동일 AGENT 를 반환(로컬/원격 공통 스텁).
vi.mock('@/hooks/useDetailTargets', () => ({
  useAgentDetailTarget: () => ({ data: AGENT, isLoading: false, error: null }),
  useAgentStatsTarget: () => ({ data: undefined, isLoading: false, error: null }),
}));

vi.mock('@/hooks/useAgent', () => ({
  useAgent: () => ({ data: AGENT, isLoading: false }),
  useConfigureAgent: () => ({
    isPending: false,
    isError: false,
    mutateAsync: configureMutateAsync,
  }),
  useExecAgent: () => ({ isPending: false, mutate: vi.fn() }),
}));

vi.mock('@/hooks/useRemote', () => ({
  useUpdateRemoteAgent: () => ({ isPending: false, mutateAsync: updateRemoteMutateAsync }),
}));

vi.mock('@/hooks/useTargetGating', () => ({
  useTargetGating: useTargetGatingMock,
}));

vi.mock('@/hooks/useDevice', () => ({
  useDevicesRealtime: () => ({ data: { data: [] }, isLoading: false }),
}));

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

// 로그레벨 로드/변경은 부수효과이므로 무력화한다.
vi.mock('@/services/api/monitorService', () => ({
  getLogLevels: () => Promise.resolve({ components: {} }),
  setComponentLogLevel: () => Promise.resolve(),
  resetComponentLogLevel: () => Promise.resolve(),
}));

vi.mock('@/services/api/seriesDataSource', () => ({
  useSeriesDataSource: () => ({ useKeys: () => ({ data: { keys: [] } }) }),
}));

// uiStore 알림 스텁.
const addNotification = vi.hoisted(() => vi.fn());
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector: (s: { addNotification: () => void }) => unknown) =>
    selector({ addNotification }),
}));

import AgentDetailPanel from './AgentDetailPanel';

function renderPanel(target: ResourceTarget) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <TargetProvider target={target}>
        <AgentDetailPanel agentId={AGENT.id} agentType={AGENT.type} agentName={AGENT.name} />
      </TargetProvider>
    </QueryClientProvider>,
  );
}

/** 설정 탭으로 전환한다. */
function openConfigTab() {
  fireEvent.click(screen.getByRole('button', { name: '설정' }));
}

beforeEach(() => {
  vi.clearAllMocks();
  useTargetGatingMock.mockReturnValue({
    isRemote: false,
    nodeReady: true,
    nodeLabel: undefined,
    canControl: () => true,
  });
});

describe('ConfigTab — 로컬 인라인 편집', () => {
  it('편집 버튼이 노출되며, 저장은 useConfigureAgent 로 호출된다(원격 경로 미호출)', async () => {
    configureMutateAsync.mockResolvedValueOnce(undefined);
    renderPanel(LOCAL_TARGET);
    openConfigTab();

    const editBtn = screen.getByTestId('agent-config-edit-button');
    expect(editBtn).not.toBeDisabled();
    fireEvent.click(editBtn);

    await act(async () => {
      fireEvent.click(screen.getByTestId('agent-config-save-button'));
    });

    expect(configureMutateAsync).toHaveBeenCalledTimes(1);
    expect(updateRemoteMutateAsync).not.toHaveBeenCalled();
    const arg = configureMutateAsync.mock.calls[0]![0] as {
      id: string;
      config: Record<string, unknown>;
    };
    expect(arg.id).toBe('a-1');
    expect(arg.config.broker).toBe('tcp://broker.local:1883');
  });
});

describe('ConfigTab — 원격 인라인 편집', () => {
  beforeEach(() => {
    useTargetGatingMock.mockReturnValue({
      isRemote: true,
      nodeReady: true,
      nodeLabel: 'gw-1',
      canControl: () => true,
    });
  });

  it('노드 ready 면 편집 버튼이 노출·활성된다', async () => {
    renderPanel(REMOTE_TARGET);
    openConfigTab();
    // 마운트 시 비동기 로그레벨 로드 effect 를 flush 한다.
    await act(async () => {});
    expect(screen.getByTestId('agent-config-edit-button')).not.toBeDisabled();
  });

  it('노드 미-ready 면 편집 버튼이 비활성된다(REQ-J05)', async () => {
    useTargetGatingMock.mockReturnValue({
      isRemote: true,
      nodeReady: false,
      nodeLabel: 'gw-1',
      canControl: () => false,
    });
    renderPanel(REMOTE_TARGET);
    openConfigTab();
    await act(async () => {});
    expect(screen.getByTestId('agent-config-edit-button')).toBeDisabled();
  });

  it('저장은 useUpdateRemoteAgent 로 마스킹 시크릿이 생략된 config 로 호출된다(로컬 경로 미호출)', async () => {
    updateRemoteMutateAsync.mockResolvedValueOnce({ id: 'a-1' });
    renderPanel(REMOTE_TARGET);
    openConfigTab();

    fireEvent.click(screen.getByTestId('agent-config-edit-button'));
    await act(async () => {
      fireEvent.click(screen.getByTestId('agent-config-save-button'));
    });

    expect(updateRemoteMutateAsync).toHaveBeenCalledTimes(1);
    expect(configureMutateAsync).not.toHaveBeenCalled();
    const arg = updateRemoteMutateAsync.mock.calls[0]![0] as {
      instanceID: string;
      agentID: string;
      req: { config?: Record<string, unknown> };
    };
    expect(arg.instanceID).toBe('node-a');
    expect(arg.agentID).toBe('a-1');
    // 비시크릿 필드는 유지된다.
    expect(arg.req.config?.broker).toBe('tcp://broker.local:1883');
    // 사용자가 새 값을 입력하지 않은(빈) 시크릿 필드는 생략된다 → 노드 backfill.
    expect(arg.req.config && 'password' in arg.req.config).toBe(false);
  });
});
