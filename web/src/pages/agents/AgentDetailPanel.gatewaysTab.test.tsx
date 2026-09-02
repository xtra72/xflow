// AgentDetailPanel — "게이트웨이" 탭의 에이전트 타입 게이트 테스트
// (SPEC-CHIRPSTACK-003 AC-8: HAS_GATEWAYS_TAB = {'chirpstack-client'}).
//
// 범위: chirpstack 에이전트에서만 탭 버튼이 노출되고, 다른 타입(xsfm / mqtt-client)에서는
// 노출되지 않는다. 탭 본문 렌더는 ChirpstackGatewaysTab.test.tsx 가 담당한다.
//
// 데이터/뮤테이션/게이팅 훅과 로그레벨/시리즈 등 부수 의존은 AgentDetailPanel.configTab
// 테스트와 동일하게 스텁으로 격리한다.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { AgentInfo } from '@/types/agent';

const AGENT: AgentInfo = {
  id: 'a-1',
  name: 'chirp-agent',
  type: 'chirpstack-client',
  status: 'running',
  connected: true,
  uptime: '5m',
  config: {},
  stats: undefined,
};

vi.mock('@/hooks/useDetailTargets', () => ({
  useAgentDetailTarget: () => ({ data: AGENT, isLoading: false, error: null }),
  useAgentStatsTarget: () => ({ data: undefined, isLoading: false, error: null }),
}));

vi.mock('@/hooks/useAgent', () => ({
  useAgent: () => ({ data: AGENT, isLoading: false }),
  useConfigureAgent: () => ({ isPending: false, isError: false, mutateAsync: vi.fn() }),
  useExecAgent: () => ({ isPending: false, mutate: vi.fn() }),
}));

vi.mock('@/hooks/useRemote', () => ({
  useUpdateRemoteAgent: () => ({ isPending: false, mutateAsync: vi.fn() }),
}));

vi.mock('@/hooks/useTargetGating', () => ({
  useTargetGating: () => ({
    isRemote: false,
    nodeReady: true,
    nodeLabel: undefined,
    canControl: () => true,
  }),
}));

vi.mock('@/hooks/useDevice', () => ({
  useDevicesRealtime: () => ({ data: { data: [] }, isLoading: false }),
  useDeleteDevice: () => ({ isPending: false, mutate: vi.fn() }),
  useSetDeviceReport: () => ({ isPending: false, mutate: vi.fn() }),
}));

// 탭 라벨을 i18n 키 그대로 렌더해 게이트 여부만 단정한다.
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

vi.mock('@/services/api/monitorService', () => ({
  getLogLevels: () => Promise.resolve({ components: {} }),
  setComponentLogLevel: () => Promise.resolve(),
  resetComponentLogLevel: () => Promise.resolve(),
}));

vi.mock('@/services/api/seriesDataSource', () => ({
  useSeriesDataSource: () => ({ useKeys: () => ({ data: { keys: [] } }) }),
}));

vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector: (s: { addNotification: () => void }) => unknown) =>
    selector({ addNotification: vi.fn() }),
}));

import AgentDetailPanel from './AgentDetailPanel';

function renderPanel(agentType: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <AgentDetailPanel agentId={AGENT.id} agentType={agentType} agentName={AGENT.name} />
    </QueryClientProvider>,
  );
}

const GATEWAYS_TAB = 'agents.detail.tabs.gateways';

beforeEach(() => {
  vi.clearAllMocks();
});

describe('AgentDetailPanel — 게이트웨이 탭 게이트', () => {
  it('chirpstack 에이전트에서는 게이트웨이 탭이 노출된다', () => {
    renderPanel('chirpstack-client');
    expect(screen.getByRole('button', { name: GATEWAYS_TAB })).toBeTruthy();
  });

  it('xsfm 에이전트에서는 게이트웨이 탭이 노출되지 않는다', () => {
    renderPanel('xsfm');
    expect(screen.queryByRole('button', { name: GATEWAYS_TAB })).toBeNull();
    // 대조군: xsfm 전용 탭은 그대로 노출된다(게이트 로직 자체가 살아 있음을 확인).
    expect(screen.getByRole('button', { name: 'agents.detail.tabs.stations' })).toBeTruthy();
  });

  it('mqtt-client 에이전트에서는 게이트웨이 탭이 노출되지 않는다', () => {
    renderPanel('mqtt-client');
    expect(screen.queryByRole('button', { name: GATEWAYS_TAB })).toBeNull();
  });
});
