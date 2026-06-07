// NodeManagementPage 테스트 (SPEC-REMOTE-001 M9, 그룹 K, REQ-K12).
//
// 범위:
//   - 디렉토리 렌더: 그룹 트리("전체" 먼저) + 그룹별 노드 + online/status.
//   - 그룹 배정/해제 mutation 호출(setGroup/clearGroup).
//   - 노드 선택 → 노드 대시보드 진입(NodeDashboard 스텁으로 격리).
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

vi.mock('@/hooks/useRemote', () => ({
  useRemoteMode: useRemoteModeMock,
  useManagedNodes: useManagedNodesMock,
  useRemoteGroups: useRemoteGroupsMock,
  useSetNodeGroup: () => ({ mutate: setGroupMutateMock, isPending: false }),
  useClearNodeGroup: () => ({ mutate: clearGroupMutateMock, isPending: false }),
}));

// 대시보드는 자체 훅(useRemoteNodeDetail)을 쓰므로 스텁으로 격리한다(별도 테스트).
vi.mock('@/components/remote/NodeDashboard', () => ({
  NodeDashboard: ({ instanceId }: { instanceId: string }) => (
    <div data-testid="node-dashboard-stub" data-instance-id={instanceId} />
  ),
}));

vi.mock('@/components/remote/RemoteNotServerNotice', () => ({
  RemoteNotServerNotice: () => <div data-testid="not-server" />,
}));

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector?: (s: { addNotification: () => void }) => unknown) => {
    const state = { addNotification: vi.fn() };
    return selector ? selector(state) : state;
  },
}));

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

// 현재 URL(경로 + 쿼리)을 노출하는 프로브 — `?node=`/`?tab=` 동기화를 검증한다.
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
});

describe('NodeManagementPage — 디렉토리', () => {
  it('그룹 트리를 "전체" 먼저 렌더하고 그룹별 노드를 묶는다', () => {
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

    const groups = screen.getAllByTestId('directory-group');
    const [allGroup, prodGroup] = groups;
    if (!allGroup || !prodGroup) throw new Error('expected two groups');
    // "전체"(빈 라벨) 그룹이 항상 먼저 정렬된다.
    expect(allGroup).toHaveAttribute('data-group', '');
    expect(prodGroup).toHaveAttribute('data-group', 'prod');

    // 각 그룹에 올바른 노드가 들어간다.
    expect(within(allGroup).getByTestId('directory-node')).toHaveAttribute(
      'data-instance-id',
      'a',
    );
    expect(within(prodGroup).getByTestId('directory-node')).toHaveAttribute(
      'data-instance-id',
      'b',
    );
  });

  it('노드를 선택하면 대시보드를 표시하고 URL `?node=` 를 갱신한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [node({ instance_id: 'a' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    expect(screen.getByTestId('node-management-no-selection')).toBeInTheDocument();

    const row = screen.getByTestId('directory-node');
    fireEvent.click(within(row).getByText('gw-1'));

    const dash = screen.getByTestId('node-dashboard-stub');
    expect(dash).toHaveAttribute('data-instance-id', 'a');
    // 선택은 URL `?node=` 로 동기화된다(딥링크/뒤로가기 지원).
    expect(new URLSearchParams(currentSearch).get('node')).toBe('a');
  });
});

describe('NodeManagementPage — URL 선택 동기화(`?node=`)', () => {
  it('`?node={id}` 가 있으면 마운트 시 해당 노드를 선택 복원한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [node({ instance_id: 'a' }), node({ instance_id: 'b', hostname: 'gw-2' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage('/admin/remote?node=b');

    // 선택 안내 없이 곧장 대시보드가 표시되며, 대상은 ?node 가 가리키는 노드다.
    expect(screen.queryByTestId('node-management-no-selection')).not.toBeInTheDocument();
    expect(screen.getByTestId('node-dashboard-stub')).toHaveAttribute(
      'data-instance-id',
      'b',
    );
  });

  it('`?node={id}` 가 목록에 없으면 선택 없음(기본 안내)으로 둔다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [node({ instance_id: 'a' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage('/admin/remote?node=missing');

    expect(screen.getByTestId('node-management-no-selection')).toBeInTheDocument();
    expect(screen.queryByTestId('node-dashboard-stub')).not.toBeInTheDocument();
  });

  it('파라미터가 없으면 기본(선택 없음)으로 렌더한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [node({ instance_id: 'a' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();
    expect(screen.getByTestId('node-management-no-selection')).toBeInTheDocument();
  });

  it('`?node` 와 함께 온 `?tab` 은 보존되어 대시보드 딥링크가 유지된다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [node({ instance_id: 'a' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage('/admin/remote?node=a&tab=flows');

    expect(screen.getByTestId('node-dashboard-stub')).toHaveAttribute(
      'data-instance-id',
      'a',
    );
    const params = new URLSearchParams(currentSearch);
    expect(params.get('node')).toBe('a');
    expect(params.get('tab')).toBe('flows');
  });
});

describe('NodeManagementPage — 그룹 배정/해제', () => {
  it('새 그룹 입력 → setGroup mutation 을 호출한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [node({ instance_id: 'a', group_name: '' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    fireEvent.click(screen.getByTestId('node-group-menu-button'));
    fireEvent.change(screen.getByTestId('node-group-new-input'), {
      target: { value: 'staging' },
    });
    fireEvent.click(screen.getByTestId('node-group-new-submit'));

    expect(setGroupMutateMock).toHaveBeenCalledTimes(1);
    expect(setGroupMutateMock).toHaveBeenNthCalledWith(
      1,
      { instanceID: 'a', groupName: 'staging' },
      expect.anything(),
    );
  });

  it('"전체"로 이동 → clearGroup mutation 을 호출한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [node({ instance_id: 'a', group_name: 'prod' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    useRemoteGroupsMock.mockReturnValue({
      data: [{ group_name: 'prod', node_count: 1 }],
    });
    renderPage();

    fireEvent.click(screen.getByTestId('node-group-menu-button'));
    fireEvent.click(screen.getByTestId('node-group-clear'));

    expect(clearGroupMutateMock).toHaveBeenCalledTimes(1);
    expect(clearGroupMutateMock).toHaveBeenNthCalledWith(1, 'a', expect.anything());
  });

  it('기존 그룹 선택 → setGroup mutation 을 해당 라벨로 호출한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [node({ instance_id: 'a', group_name: '' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    useRemoteGroupsMock.mockReturnValue({
      data: [
        { group_name: '', node_count: 1 },
        { group_name: 'prod', node_count: 0 },
      ],
    });
    renderPage();

    fireEvent.click(screen.getByTestId('node-group-menu-button'));
    fireEvent.click(screen.getByTestId('node-group-option'));

    expect(setGroupMutateMock).toHaveBeenNthCalledWith(
      1,
      { instanceID: 'a', groupName: 'prod' },
      expect.anything(),
    );
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
