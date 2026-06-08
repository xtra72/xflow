// NodeManagementPage(관리자 뷰) 테스트 (SPEC-REMOTE-001 M11, 그룹 M, REQ-M04~M06/M10).
//
// 범위:
//   - 상단 바: 노드 피커(그룹 묶음) + 서브탭 네비 + 나가기.
//   - 노드 선택 → ?node= 갱신, 서브탭 전환 → ?tab= 갱신(딥링크 보존 — M10).
//   - 풀폭 노드 화면(좌측 디렉토리 제거 → 왜곡 방지).
//   - 전역 사이드바: 진입 시 자동으로 접지 않는다(사용자 제어).
//   - 그룹 배정/해제 mutation 호출(선택 노드 대상).
//   - 비-server 모드 게이팅.

import { fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter, useLocation } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { ManagedNode, NodeGroup } from '@/types/remote';

const useRemoteModeMock = vi.hoisted(() => vi.fn());
const useManagedNodesMock = vi.hoisted(() => vi.fn());
const useRemoteGroupsMock = vi.hoisted(() => vi.fn());
const setGroupMutateMock = vi.hoisted(() => vi.fn());
const clearGroupMutateMock = vi.hoisted(() => vi.fn());
const setSidebarCollapsedMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useRemote', () => ({
  useRemoteMode: useRemoteModeMock,
  useManagedNodes: useManagedNodesMock,
  useRemoteGroups: useRemoteGroupsMock,
  useSetNodeGroup: () => ({ mutate: setGroupMutateMock, isPending: false }),
  useClearNodeGroup: () => ({ mutate: clearGroupMutateMock, isPending: false }),
}));

// 대시보드는 자체 훅을 쓰므로 스텁으로 격리한다(별도 테스트).
vi.mock('@/components/remote/NodeDashboard', () => ({
  NodeDashboard: ({ instanceId, hideTabNav }: { instanceId: string; hideTabNav?: boolean }) => (
    <div
      data-testid="node-dashboard-stub"
      data-instance-id={instanceId}
      data-hide-tab-nav={String(hideTabNav ?? false)}
    />
  ),
}));

vi.mock('@/components/remote/RemoteNotServerNotice', () => ({
  RemoteNotServerNotice: () => <div data-testid="not-server" />,
}));

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

// uiStore: setSidebarCollapsed 캡처 + getState().sidebarCollapsed 제공(복원 검증용).
vi.mock('@/stores/uiStore', () => {
  type State = { addNotification: () => void; setSidebarCollapsed: (v: boolean) => void; sidebarCollapsed: boolean };
  const state: State = {
    addNotification: vi.fn(),
    setSidebarCollapsed: setSidebarCollapsedMock,
    sidebarCollapsed: false,
  };
  const useUIStore = (selector?: (s: State) => unknown) => (selector ? selector(state) : state);
  useUIStore.getState = () => state;
  return { useUIStore };
});

import NodeManagementPage from './NodeManagementPage';

function node(o: Partial<ManagedNode> = {}): ManagedNode {
  return {
    instance_id: 'node-a',
    hostname: 'gw-1',
    version: '1.0',
    status: 'approved',
    online: true,
    group_name: '',
    last_seen: 0,
    ...o,
  };
}

// 현재 URL(쿼리)을 노출하는 프로브 — `?node=`/`?tab=` 동기화를 검증한다.
let currentSearch = '';
function LocationProbe(): null {
  const location = useLocation();
  currentSearch = location.search;
  return null;
}

function renderPage(initialEntry = '/admin/remote') {
  currentSearch = '';
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <NodeManagementPage />
      <LocationProbe />
    </MemoryRouter>,
  );
}

beforeEach(() => {
  useRemoteModeMock.mockReset().mockReturnValue({ data: { mode: 'server' } });
  useManagedNodesMock.mockReset().mockReturnValue({
    data: [],
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  });
  useRemoteGroupsMock.mockReset().mockReturnValue({ data: [] as NodeGroup[] });
  setGroupMutateMock.mockReset();
  clearGroupMutateMock.mockReset();
  setSidebarCollapsedMock.mockReset();
});

describe('NodeManagementPage — 상단 바 + 노드 피커', () => {
  it('상단 바를 렌더하고 좌측 디렉토리는 제거된다(풀폭 — 왜곡 방지)', () => {
    useManagedNodesMock.mockReturnValue({
      data: [node({ instance_id: 'a' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    expect(screen.getByTestId('manager-view-top-bar')).toBeInTheDocument();
    expect(screen.getByTestId('manager-view-screen')).toBeInTheDocument();
    // M9 좌측 디렉토리는 더 이상 렌더되지 않는다.
    expect(screen.queryByTestId('node-directory')).not.toBeInTheDocument();
  });

  it('노드 피커는 그룹 묶음 드롭다운으로 "전체" 먼저 + 그룹별 노드를 렌더한다(REQ-M05)', () => {
    useRemoteGroupsMock.mockReturnValue({
      data: [
        { group_name: 'prod', node_count: 1 },
        { group_name: '', node_count: 1 },
      ],
    });
    useManagedNodesMock.mockReturnValue({
      data: [
        node({ instance_id: 'a', group_name: '' }),
        node({ instance_id: 'b', group_name: 'prod', hostname: 'gw-2' }),
      ],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    // 드롭다운을 연다.
    fireEvent.click(screen.getByTestId('node-picker-button'));

    const groups = screen.getAllByTestId('picker-group');
    const [allGroup, prodGroup] = groups;
    if (!allGroup || !prodGroup) throw new Error('expected two picker groups');
    // "전체"(빈 라벨) 그룹이 항상 먼저 정렬된다(M9 그룹핑 보존).
    expect(allGroup).toHaveAttribute('data-group', '');
    expect(prodGroup).toHaveAttribute('data-group', 'prod');

    // 각 그룹에 올바른 노드가 묶인다.
    expect(within(allGroup).getByTestId('picker-node')).toHaveAttribute('data-instance-id', 'a');
    expect(within(prodGroup).getByTestId('picker-node')).toHaveAttribute('data-instance-id', 'b');
  });

  it('노드를 선택하면 풀폭 노드 화면을 표시하고 URL `?node=` 를 갱신한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [node({ instance_id: 'a' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    expect(screen.getByTestId('node-management-no-selection')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('node-picker-button'));
    fireEvent.click(screen.getByTestId('picker-node'));

    const dash = screen.getByTestId('node-dashboard-stub');
    expect(dash).toHaveAttribute('data-instance-id', 'a');
    // 관리자 뷰: 내부 서브탭 네비는 상단 바로 호이스팅되므로 숨긴다(REQ-M06).
    expect(dash).toHaveAttribute('data-hide-tab-nav', 'true');
    expect(new URLSearchParams(currentSearch).get('node')).toBe('a');
  });
});

describe('NodeManagementPage — 서브탭 호이스팅 + 딥링크(`?tab=`)', () => {
  it('선택 노드의 서브탭 네비를 상단 바에 렌더한다(개요/대시보드/플로우/에이전트/디바이스)', () => {
    useManagedNodesMock.mockReturnValue({
      data: [node({ instance_id: 'a' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage('/admin/remote?node=a');

    expect(screen.getByTestId('manager-view-tabs')).toBeInTheDocument();
    for (const id of ['overview', 'dashboard', 'flows', 'agents', 'devices']) {
      expect(screen.getByTestId(`manager-view-tab-${id}`)).toBeInTheDocument();
    }
  });

  it('서브탭을 전환하면 URL `?tab=` 을 갱신하고 `?node=` 를 보존한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [node({ instance_id: 'a' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage('/admin/remote?node=a');

    fireEvent.click(screen.getByTestId('manager-view-tab-flows'));
    const params = new URLSearchParams(currentSearch);
    expect(params.get('tab')).toBe('flows');
    expect(params.get('node')).toBe('a');
  });

  it('`?node`/`?tab` 딥링크는 마운트 시 노드 선택 + 활성 탭을 복원한다(M10 보존)', () => {
    useManagedNodesMock.mockReturnValue({
      data: [node({ instance_id: 'a' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage('/admin/remote?node=a&tab=flows');

    expect(screen.getByTestId('node-dashboard-stub')).toHaveAttribute('data-instance-id', 'a');
    expect(screen.getByTestId('manager-view-tab-flows')).toHaveAttribute('aria-selected', 'true');
  });

  it('나가기는 선택을 해제하고 `?node`/`?tab` 을 제거한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [node({ instance_id: 'a' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage('/admin/remote?node=a&tab=flows');

    fireEvent.click(screen.getByTestId('manager-view-exit'));
    const params = new URLSearchParams(currentSearch);
    expect(params.get('node')).toBeNull();
    expect(params.get('tab')).toBeNull();
    expect(screen.getByTestId('node-management-no-selection')).toBeInTheDocument();
  });
});

describe('NodeManagementPage — 전역 사이드바 비간섭', () => {
  it('진입 시 전역 사이드바를 자동으로 접지 않는다(사용자 제어)', () => {
    renderPage();
    // 노드 관리 진입이 사이드바 접힘 상태를 강제로 바꾸지 않는다.
    expect(setSidebarCollapsedMock).not.toHaveBeenCalled();
  });
});

describe('NodeManagementPage — 그룹 배정/해제(관리 액션)', () => {
  it('선택 노드에 새 그룹 입력 → setGroup mutation 을 호출한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [node({ instance_id: 'a', group_name: '' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage('/admin/remote?node=a');

    fireEvent.click(screen.getByTestId('node-group-menu-button'));
    fireEvent.change(screen.getByTestId('node-group-new-input'), {
      target: { value: 'staging' },
    });
    fireEvent.click(screen.getByTestId('node-group-new-submit'));

    expect(setGroupMutateMock).toHaveBeenNthCalledWith(
      1,
      { instanceID: 'a', groupName: 'staging' },
      expect.anything(),
    );
  });

  it('선택 노드를 "전체"로 이동 → clearGroup mutation 을 호출한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [node({ instance_id: 'a', group_name: 'prod' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    useRemoteGroupsMock.mockReturnValue({
      data: [{ group_name: 'prod', node_count: 1 }],
    });
    renderPage('/admin/remote?node=a');

    fireEvent.click(screen.getByTestId('node-group-menu-button'));
    fireEvent.click(screen.getByTestId('node-group-clear'));

    expect(clearGroupMutateMock).toHaveBeenNthCalledWith(1, 'a', expect.anything());
  });
});

describe('NodeManagementPage — 모드 게이팅', () => {
  it('server 모드가 아니면 안내만 표시하고 노드 쿼리를 막는다', () => {
    useRemoteModeMock.mockReturnValue({ data: { mode: 'client' } });
    renderPage();
    expect(screen.getByTestId('not-server')).toBeInTheDocument();
    expect(useManagedNodesMock).toHaveBeenCalledWith(undefined, false);
  });
});
