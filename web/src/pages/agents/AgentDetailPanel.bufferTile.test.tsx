// AgentDetailPanel — 통계 탭 "메시지 버퍼" 타일 테스트
//
// 범위:
//  1) buffer.capacity > 0 이면 pending/capacity 원값과 파생 사용률(%)이 함께 렌더된다.
//  2) 버퍼가 없는 에이전트(capacity 0 — BufferInfoProvider 미구현)에서는 타일 자체가 없다.
//     상세 통계 응답은 buffer 객체를 항상 채워 보내므로(omitempty 아님) 이 경우가 실제로 발생한다.
//  3) 임계 구간(경고/위험)은 색상 + 경고 문구로 구분된다.
//
// 데이터/뮤테이션/게이팅 훅 등 부수 의존은 gatewaysTab / configTab 테스트와 동일하게 스텁으로
// 격리하고, useAgentStatsTarget 만 케이스별로 바꿔 끼운다.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { AgentInfo, AgentStatsInfo } from '@/types/agent';

const AGENT: AgentInfo = {
  id: 'a-1',
  name: 'buf-agent',
  type: 'mqtt-client',
  status: 'running',
  connected: true,
  uptime: '5m',
  config: {},
  stats: undefined,
};

/** 케이스별로 교체되는 통계 응답. */
const statsRef: { current: AgentStatsInfo | undefined } = { current: undefined };

/** 버퍼 외 필드는 고정값으로 채운 기본 통계. */
function baseStats(overrides: Partial<AgentStatsInfo> = {}): AgentStatsInfo {
  return {
    id: AGENT.id,
    status: 'running',
    uptime: '5m',
    messages_in: 10,
    messages_out: 5,
    error_count: 0,
    connected: true,
    dropped_messages: 0,
    restart_count: 0,
    connections: [],
    node_refs: [],
    ...overrides,
  };
}

vi.mock('@/hooks/useDetailTargets', () => ({
  useAgentDetailTarget: () => ({ data: AGENT, isLoading: false, error: null }),
  useAgentStatsTarget: () => ({ data: statsRef.current, isLoading: false, error: null }),
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

// 라벨은 i18n 키 그대로 렌더해 키 존재 여부와 무관하게 구조만 단정한다.
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

function renderStatsTab(stats: AgentStatsInfo | undefined) {
  statsRef.current = stats;
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <AgentDetailPanel agentId={AGENT.id} agentType={AGENT.type} agentName={AGENT.name} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.clearAllMocks();
  statsRef.current = undefined;
});

describe('AgentDetailPanel — 메시지 버퍼 타일', () => {
  it('버퍼가 있는 에이전트: pending/capacity 원값과 파생 사용률을 함께 표시한다', () => {
    renderStatsTab(baseStats({ buffer: { pending: 256, capacity: 1024 } }));

    expect(screen.getByTestId('agent-buffer-card')).toBeTruthy();
    // 원값: 천 단위 구분자 포함
    expect(screen.getByTestId('agent-buffer-raw').textContent).toBe('256 / 1,024');
    // 파생 사용률: 256/1024 = 25.0%
    expect(screen.getByTestId('agent-buffer-percent').textContent).toBe('25.0%');

    const bar = screen.getByTestId('agent-buffer-bar');
    expect(bar.style.width).toBe('25%');
    expect(bar.getAttribute('aria-valuenow')).toBe('256');
    expect(bar.getAttribute('aria-valuemax')).toBe('1024');
  });

  it('평시 구간에서는 경고 문구를 띄우지 않는다', () => {
    renderStatsTab(baseStats({ buffer: { pending: 256, capacity: 1024 } }));

    expect(screen.getByTestId('agent-buffer-card').getAttribute('data-buffer-level')).toBe('normal');
    expect(screen.queryByTestId('agent-buffer-alert')).toBeNull();
  });

  it('버퍼가 없는 에이전트(capacity 0): 타일을 전혀 렌더하지 않는다', () => {
    // BufferInfoProvider 미구현 에이전트의 실제 응답 형상 — 필드는 존재하되 0/0 이다.
    renderStatsTab(baseStats({ buffer: { pending: 0, capacity: 0 } }));

    expect(screen.queryByTestId('agent-buffer-card')).toBeNull();
    expect(screen.queryByTestId('agent-buffer-bar')).toBeNull();
    // 대조군: 통계 탭 자체는 정상 렌더된다(게이트 로직만 동작했음을 확인).
    expect(screen.getByText('agents.detail.stats.operationStats')).toBeTruthy();
  });

  it('buffer 객체가 아예 없으면 flat 필드로 대체하고, 그마저 없으면 렌더하지 않는다', () => {
    const { unmount } = renderStatsTab(
      baseStats({ buffer_pending: 40, buffer_capacity: 50 }),
    );
    expect(screen.getByTestId('agent-buffer-raw').textContent).toBe('40 / 50');
    unmount();

    renderStatsTab(baseStats());
    expect(screen.queryByTestId('agent-buffer-card')).toBeNull();
  });

  it('경고 임계(80% 이상): 색상이 warning 으로 바뀌고 경고 문구가 붙는다', () => {
    renderStatsTab(baseStats({ buffer: { pending: 850, capacity: 1000 } }));

    const card = screen.getByTestId('agent-buffer-card');
    expect(card.getAttribute('data-buffer-level')).toBe('warning');
    expect(screen.getByTestId('agent-buffer-bar').style.backgroundColor).toBe(
      'var(--color-status-warning)',
    );
    expect(screen.getByTestId('agent-buffer-alert').textContent).toBe(
      'agents.detail.stats.bufferNearFull',
    );
  });

  it('위험 임계(95% 이상): 색상이 error 로 바뀐다', () => {
    renderStatsTab(baseStats({ buffer: { pending: 990, capacity: 1000 } }));

    const card = screen.getByTestId('agent-buffer-card');
    expect(card.getAttribute('data-buffer-level')).toBe('critical');
    expect(screen.getByTestId('agent-buffer-bar').style.backgroundColor).toBe(
      'var(--color-status-error)',
    );
    expect(screen.getByTestId('agent-buffer-alert')).toBeTruthy();
  });

  it('평시/경고/위험 구간의 색상은 서로 구분된다', () => {
    const colorAt = (pending: number) => {
      statsRef.current = baseStats({ buffer: { pending, capacity: 1000 } });
      const view = renderStatsTab(statsRef.current);
      const color = screen.getByTestId('agent-buffer-bar').style.backgroundColor;
      view.unmount();
      return color;
    };

    const normal = colorAt(100);
    const warning = colorAt(850);
    const critical = colorAt(990);

    expect(new Set([normal, warning, critical]).size).toBe(3);
  });
});
