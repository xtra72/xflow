// GroupManagementPage 테스트 — 2분할 구성 + 선택 라우팅(자식 스텁).
import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import type { ManagedNode, NodeGroup } from '@/types/remote';

const modeRef = vi.hoisted(() => ({ value: { mode: 'server' } as { mode: string } }));
const groupsRef = vi.hoisted(() => ({ value: [] as NodeGroup[] }));
const nodesRef = vi.hoisted(() => ({ value: [] as ManagedNode[] | undefined }));

vi.mock('@/hooks/useRemote', () => ({
  useRemoteMode: () => ({ data: modeRef.value }),
  useRemoteGroups: () => ({ data: groupsRef.value }),
  useManagedNodes: () => ({ data: nodesRef.value }),
}));

// 자식은 props 캡처 스텁으로 대체(라우팅 검증 격리).
vi.mock('@/components/remote/GroupTree', () => ({
  GroupTree: (props: {
    selectedGroup: string | null;
    onSelectNode: (id: string) => void;
    onSelectCreate: () => void;
  }) => (
    <div data-testid="tree-stub" data-selected={props.selectedGroup}>
      <button data-testid="stub-select-node" onClick={() => props.onSelectNode('n1')} />
      <button data-testid="stub-select-create" onClick={() => props.onSelectCreate()} />
    </div>
  ),
}));
vi.mock('@/components/remote/GroupControlPanel', () => ({
  GroupControlPanel: (props: { groupName: string }) => (
    <div data-testid="control-stub" data-group={props.groupName} />
  ),
}));
vi.mock('@/components/remote/GroupCreatePanel', () => ({
  GroupCreatePanel: () => <div data-testid="create-stub" />,
}));
vi.mock('@/components/remote/NodeDashboard', () => ({
  NodeDashboard: (props: { instanceId: string }) => (
    <div data-testid="dashboard-stub" data-id={props.instanceId} />
  ),
}));
vi.mock('@/components/remote/RemoteNotServerNotice', () => ({
  RemoteNotServerNotice: () => <div data-testid="not-server" />,
}));
vi.mock('@/components/remote/UpdateSourceSettings', () => ({
  UpdateSourceSettings: () => <div data-testid="update-source-stub" />,
}));

import GroupManagementPage from './GroupManagementPage';

function renderPage(): void {
  render(
    <I18nProvider>
      <GroupManagementPage />
    </I18nProvider>,
  );
}

const NODES: ManagedNode[] = [
  { instance_id: 'n1', hostname: 'h1', version: 'v1', status: 'approved', online: true, last_seen: 0, group_name: 'prod' },
];

beforeEach(() => {
  modeRef.value = { mode: 'server' };
  groupsRef.value = [{ group_name: 'prod', node_count: 1 }];
  nodesRef.value = NODES;
});

describe('GroupManagementPage', () => {
  it('비-server 모드면 안내를 표시한다', () => {
    modeRef.value = { mode: 'client' };
    renderPage();
    expect(screen.getByTestId('not-server')).toBeInTheDocument();
  });

  it('server 모드: 트리 + 기본 그룹 제어를 렌더한다(첫 명명 그룹 선택)', () => {
    renderPage();
    expect(screen.getByTestId('tree-stub')).toBeInTheDocument();
    const control = screen.getByTestId('control-stub');
    expect(control).toBeInTheDocument();
    expect(control.getAttribute('data-group')).toBe('prod');
  });

  it('노드 선택 시 우측이 NodeDashboard 로 전환된다', () => {
    renderPage();
    fireEvent.click(screen.getByTestId('stub-select-node'));
    const dash = screen.getByTestId('dashboard-stub');
    expect(dash).toBeInTheDocument();
    expect(dash.getAttribute('data-id')).toBe('n1');
  });

  it('신규 그룹 선택 시 우측이 생성 패널로 전환된다', () => {
    renderPage();
    fireEvent.click(screen.getByTestId('stub-select-create'));
    expect(screen.getByTestId('create-stub')).toBeInTheDocument();
  });

  it('명명 그룹이 없으면 미분류를 기본 선택한다', () => {
    groupsRef.value = [];
    nodesRef.value = [
      { instance_id: 'u1', hostname: 'u', version: 'v1', status: 'approved', online: true, last_seen: 0, group_name: '' },
    ];
    renderPage();
    expect(screen.getByTestId('control-stub').getAttribute('data-group')).toBe('');
  });
});
