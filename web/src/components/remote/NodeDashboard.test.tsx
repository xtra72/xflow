// NodeDashboard 테스트 (SPEC-REMOTE-001 M9, 그룹 K, REQ-K13/K14).
//
// 범위:
//   - 개요: 시스템 정보(hostname/os/arch/version/uptime) + 운영 요약 렌더.
//   - uptime null(미보고) → "미보고" 표시(하위 호환).
//   - Flow/Agent/Device 서브탭이 M8 통합 페이지를 target=remote:{id} 로 재사용.

import { fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router';
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
// 대시보드 서브탭(M10, 그룹 L): 로컬 DashboardPage 를 target 으로 재사용한다.
vi.mock('@/pages/dashboard/DashboardPage', () => ({
  default: ({ target }: StubProps) => (
    <div data-testid="dashboard-page-stub" data-target={JSON.stringify(target)} />
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
    display_width: 1920,
    display_height: 1080,
    summary: {
      flows: { total: 3, running: 2, stopped: 1 },
      agents: { total: 2, connected: 1 },
      devices: { total: 5, online: 4 },
    },
    ...o,
  };
}

// 현재 URL 쿼리를 노출하는 프로브 — `?tab=` 동기화를 검증한다.
let currentSearch = '';
function LocationProbe(): null {
  const location = useLocation();
  currentSearch = location.search;
  return null;
}

function renderDashboard(initialEntry = '/admin/remote?node=node-a') {
  currentSearch = '';
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <NodeDashboard instanceId="node-a" enabled />
      <LocationProbe />
    </MemoryRouter>,
  );
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

describe('NodeDashboard — 대시보드 서브탭(M10, 그룹 L)', () => {
  it('대시보드 탭이 존재한다', () => {
    renderDashboard();
    expect(screen.getByTestId('node-dashboard-tab-dashboard')).toBeInTheDocument();
  });

  it('대시보드 서브탭은 로컬 DashboardPage 를 target=remote:node-a 로 렌더한다', async () => {
    renderDashboard();
    fireEvent.click(screen.getByTestId('node-dashboard-tab-dashboard'));

    const el = await screen.findByTestId('dashboard-page-stub');
    expect(JSON.parse(el.getAttribute('data-target') ?? 'null')).toEqual({
      type: 'remote',
      instanceId: 'node-a',
    });
  });

  it('`?tab=dashboard` 딥링크는 마운트 시 대시보드 탭을 복원한다', async () => {
    renderDashboard('/admin/remote?node=node-a&tab=dashboard');
    expect(await screen.findByTestId('dashboard-page-stub')).toBeInTheDocument();
    expect(screen.queryByTestId('node-overview')).not.toBeInTheDocument();
  });

  it('대시보드 탭만 고정 캔버스로 감싸고 노드 해상도를 적용한다(REQ-M07/M08)', async () => {
    renderDashboard('/admin/remote?node=node-a&tab=dashboard');
    const canvas = await screen.findByTestId('fixed-canvas');
    // 노드 보고 해상도(1920×1080)가 고정 캔버스 크기로 적용된다.
    expect(canvas).toHaveAttribute('data-canvas-width', '1920');
    expect(canvas).toHaveAttribute('data-canvas-height', '1080');
    // DashboardPage 가 캔버스 내부에 렌더된다.
    expect(within(canvas).getByTestId('dashboard-page-stub')).toBeInTheDocument();
  });

  it('해상도 미보고(0) 시 폴백 1920×1080 을 사용한다(REQ-M03)', async () => {
    useRemoteNodeDetailMock.mockReturnValue({
      data: detail({ display_width: 0, display_height: 0 }),
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderDashboard('/admin/remote?node=node-a&tab=dashboard');
    const canvas = await screen.findByTestId('fixed-canvas');
    expect(canvas).toHaveAttribute('data-canvas-width', '1920');
    expect(canvas).toHaveAttribute('data-canvas-height', '1080');
  });

  it('플로우/에이전트/디바이스 서브탭은 고정 캔버스를 적용하지 않는다(풀폭 반응형 — OQ-M2)', async () => {
    renderDashboard('/admin/remote?node=node-a&tab=flows');
    expect(await screen.findByTestId('flow-list-stub')).toBeInTheDocument();
    // 대시보드 전용 고정 캔버스가 다른 서브탭에는 존재하지 않는다.
    expect(screen.queryByTestId('fixed-canvas')).not.toBeInTheDocument();
  });
});

describe('NodeDashboard — 활성 탭 URL 동기화(`?tab=`)', () => {
  it('파라미터가 없으면 기본 탭(overview)을 표시한다', () => {
    renderDashboard('/admin/remote?node=node-a');
    expect(screen.getByTestId('node-overview')).toBeInTheDocument();
    // overview 는 기본값이므로 URL 에 tab 파라미터를 추가하지 않는다.
    expect(new URLSearchParams(currentSearch).get('tab')).toBeNull();
  });

  it('`?tab=flows` 가 있으면 마운트 시 flows 탭을 복원한다', async () => {
    renderDashboard('/admin/remote?node=node-a&tab=flows');
    // overview 가 아니라 flows 서브탭(통합 페이지 스텁)이 곧장 렌더된다.
    expect(await screen.findByTestId('flow-list-stub')).toBeInTheDocument();
    expect(screen.queryByTestId('node-overview')).not.toBeInTheDocument();
  });

  it('무효한 `?tab` 값은 overview 로 폴백한다', () => {
    renderDashboard('/admin/remote?node=node-a&tab=bogus');
    expect(screen.getByTestId('node-overview')).toBeInTheDocument();
  });

  it('탭을 전환하면 URL `?tab=` 을 갱신하고 `?node=` 는 보존한다', async () => {
    renderDashboard('/admin/remote?node=node-a');
    fireEvent.click(screen.getByTestId('node-dashboard-tab-agents'));

    expect(await screen.findByTestId('agent-list-stub')).toBeInTheDocument();
    const params = new URLSearchParams(currentSearch);
    expect(params.get('tab')).toBe('agents');
    // 노드 선택(다른 파라미터)은 보존된다(딥링크 일관).
    expect(params.get('node')).toBe('node-a');
  });

  it('overview 로 되돌리면 `?tab` 파라미터를 제거한다(URL 청결 유지)', () => {
    renderDashboard('/admin/remote?node=node-a&tab=devices');
    fireEvent.click(screen.getByTestId('node-dashboard-tab-overview'));

    const params = new URLSearchParams(currentSearch);
    expect(params.get('tab')).toBeNull();
    expect(params.get('node')).toBe('node-a');
  });
});
