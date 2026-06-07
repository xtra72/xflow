// NodeDashboard 테스트 (SPEC-REMOTE-001 M9, 그룹 K, REQ-K13/K14).
//
// 범위:
//   - 개요: 시스템 정보(hostname/os/arch/version/uptime) + 운영 요약 렌더.
//   - uptime null(미보고) → "미보고" 표시(하위 호환).
//   - Flow/Agent/Device 서브탭이 M8 통합 페이지를 target=remote:{id} 로 재사용.

import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { NodeDetail } from '@/types/remote';

const useRemoteNodeDetailMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useRemote', () => ({
  useRemoteNodeDetail: useRemoteNodeDetailMock,
}));

// 통합 페이지는 target/hideRemoteBanner prop 을 캡처하는 스텁으로 대체한다
// (라우팅·배너 위임 검증 격리). 스텁은 hideRemoteBanner 가 true 인 동안 실제
// 페이지가 배너를 렌더하지 않음을 표현하기 위해 배너 자체는 그리지 않는다.
type StubProps = {
  target?: { type: string; instanceId?: string };
  hideRemoteBanner?: boolean;
};
vi.mock('@/pages/flows/FlowListPage', () => ({
  default: ({ target, hideRemoteBanner }: StubProps) => (
    <div
      data-testid="flow-list-stub"
      data-target={JSON.stringify(target)}
      data-hide-remote-banner={String(hideRemoteBanner ?? false)}
    />
  ),
}));
vi.mock('@/pages/agents/AgentListPage', () => ({
  default: ({ target, hideRemoteBanner }: StubProps) => (
    <div
      data-testid="agent-list-stub"
      data-target={JSON.stringify(target)}
      data-hide-remote-banner={String(hideRemoteBanner ?? false)}
    />
  ),
}));
vi.mock('@/pages/devices/DeviceListPage', () => ({
  default: ({ target, hideRemoteBanner }: StubProps) => (
    <div
      data-testid="device-list-stub"
      data-target={JSON.stringify(target)}
      data-hide-remote-banner={String(hideRemoteBanner ?? false)}
    />
  ),
}));

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

// 인디케이터/배지는 자체 i18n 을 쓰므로 단순 스텁.
vi.mock('@/components/remote/NodeOnlineIndicator', () => ({
  NodeOnlineIndicator: ({ online }: { online: boolean }) => (
    <span data-testid="online" data-online={online} />
  ),
}));
vi.mock('@/components/remote/NodeStatusBadge', () => ({
  NodeStatusBadge: ({ status }: { status: string }) => (
    <span data-testid="status" data-status={status} />
  ),
}));

import { NodeDashboard } from './NodeDashboard';

function detail(o: Partial<NodeDetail> = {}): NodeDetail {
  return {
    instance_id: 'node-a',
    hostname: 'gw-1',
    version: '1.2.3',
    status: 'approved',
    online: true,
    group_name: 'prod',
    os: 'linux',
    arch: 'arm64',
    started_at: 1_700_000_000_000,
    uptime: 3_600_000,
    last_seen: 1_700_000_100_000,
    summary: {
      flows: { total: 3, running: 2, stopped: 1 },
      agents: { total: 2, connected: 1 },
      devices: { total: 5, online: 4 },
    },
    ...o,
  };
}

function renderDashboard() {
  return render(<NodeDashboard instanceId="node-a" enabled />);
}

beforeEach(() => {
  useRemoteNodeDetailMock.mockReset().mockReturnValue({
    data: detail(),
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  });
});

describe('NodeDashboard — 개요', () => {
  it('시스템 정보와 운영 요약을 렌더한다', async () => {
    renderDashboard();
    expect(screen.getByTestId('node-overview')).toBeInTheDocument();
    expect(screen.getByText('gw-1')).toBeInTheDocument();
    expect(screen.getByText('linux')).toBeInTheDocument();
    expect(screen.getByText('arm64')).toBeInTheDocument();
    expect(screen.getByText('1.2.3')).toBeInTheDocument();

    // 운영 요약 카드(플로우/에이전트/디바이스).
    expect(screen.getByTestId('summary-flows')).toBeInTheDocument();
    expect(screen.getByTestId('summary-agents')).toBeInTheDocument();
    expect(screen.getByTestId('summary-devices')).toBeInTheDocument();
  });

  it('uptime 이 null(미보고)이면 "미보고"로 표시한다', () => {
    useRemoteNodeDetailMock.mockReturnValue({
      data: detail({ uptime: null, started_at: 0 }),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderDashboard();
    expect(screen.getByTestId('node-uptime')).toHaveTextContent(
      'remote.dashboard.uptimeUnreported',
    );
  });

  it('uptime 이 있으면 사람이 읽는 기간으로 표시한다', () => {
    renderDashboard();
    // 3,600,000ms = 3600s = 1h.
    expect(screen.getByTestId('node-uptime')).toHaveTextContent('1h');
  });
});

describe('NodeDashboard — 서브탭(M8 통합 제어 재사용)', () => {
  it.each([
    ['flows', 'flow-list-stub'],
    ['agents', 'agent-list-stub'],
    ['devices', 'device-list-stub'],
  ])('%s 서브탭은 통합 페이지를 target=remote:node-a 로 렌더한다', async (tab, stub) => {
    renderDashboard();
    fireEvent.click(screen.getByTestId(`node-dashboard-tab-${tab}`));

    const el = await screen.findByTestId(stub);
    expect(JSON.parse(el.getAttribute('data-target') ?? 'null')).toEqual({
      type: 'remote',
      instanceId: 'node-a',
    });
  });

  it.each([
    ['flows', 'flow-list-stub'],
    ['agents', 'agent-list-stub'],
    ['devices', 'device-list-stub'],
  ])(
    '%s 서브탭은 통합 페이지에 hideRemoteBanner 를 주입하고 원격 배너를 렌더하지 않는다',
    async (tab, stub) => {
      renderDashboard();
      fireEvent.click(screen.getByTestId(`node-dashboard-tab-${tab}`));

      const el = await screen.findByTestId(stub);
      // 대시보드는 임베드 컨텍스트에서 배너를 숨기도록 위임한다(REQ-K14).
      expect(el).toHaveAttribute('data-hide-remote-banner', 'true');
      // 임베드 컨텍스트에 중복 원격 배너가 존재하지 않는다.
      expect(screen.queryByTestId('remote-target-banner')).not.toBeInTheDocument();
    },
  );
});
