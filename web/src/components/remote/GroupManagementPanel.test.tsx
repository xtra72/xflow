// GroupManagementPanel 테스트 (그룹 관리).
//
// useRemote 의 rename/delete/update 뮤테이션 + target 쿼리, uiStore(toast)를 mock 한다.
// 범위: 명명된 그룹만 표시, 이름변경, 삭제 확인, 일괄 업데이트 확인.

import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import type { ManagedNode, NodeGroup } from '@/types/remote';

const renameMutate = vi.hoisted(() => vi.fn());
const deleteMutate = vi.hoisted(() => vi.fn());
const updateMutate = vi.hoisted(() => vi.fn());
const commandMutate = vi.hoisted(() => vi.fn());
const setNodeGroupAsync = vi.hoisted(() => vi.fn(async () => undefined));

vi.mock('@/hooks/useRemote', () => ({
  useTargetVersion: () => ({ data: { version: 'v1.3.0' } }),
  useRenameGroup: () => ({ mutate: renameMutate, isPending: false }),
  useDeleteGroup: () => ({ mutate: deleteMutate, isPending: false }),
  useUpdateGroup: () => ({ mutate: updateMutate, isPending: false }),
  useCommandGroup: () => ({ mutate: commandMutate, isPending: false }),
  useSetNodeGroup: () => ({ mutateAsync: setNodeGroupAsync, isPending: false }),
}));

const addNotificationMock = vi.hoisted(() => vi.fn());
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector?: (s: { addNotification: typeof addNotificationMock }) => unknown) => {
    const state = { addNotification: addNotificationMock };
    return selector ? selector(state) : state;
  },
}));

import { GroupManagementPanel } from './GroupManagementPanel';

const NODES: ManagedNode[] = [
  { instance_id: 'n1', hostname: 'host-1', version: 'v1.0.0', status: 'approved', online: true, last_seen: 0, group_name: '' },
  { instance_id: 'n2', hostname: 'host-2', version: 'v1.0.0', status: 'approved', online: true, last_seen: 0, group_name: 'dev' },
];

function renderPanel(groups: NodeGroup[], nodes: ManagedNode[] = NODES): void {
  render(
    <I18nProvider>
      <GroupManagementPanel groups={groups} nodes={nodes} />
    </I18nProvider>,
  );
}

const GROUPS: NodeGroup[] = [
  { group_name: '', node_count: 2 }, // "전체" — 제외 대상
  { group_name: 'prod', node_count: 3 },
  { group_name: 'dev', node_count: 1 },
];

beforeEach(() => {
  renameMutate.mockReset();
  deleteMutate.mockReset();
  updateMutate.mockReset();
  commandMutate.mockReset();
  setNodeGroupAsync.mockClear();
  addNotificationMock.mockReset();
});

describe('GroupManagementPanel', () => {
  it('명명된 그룹만 표시하고 "전체" 버킷은 제외한다', () => {
    renderPanel(GROUPS);
    expect(screen.getByTestId('group-row-prod')).toBeInTheDocument();
    expect(screen.getByTestId('group-row-dev')).toBeInTheDocument();
    // 빈 라벨("전체") 행은 없어야 한다.
    expect(screen.queryByTestId('group-row-')).toBeNull();
  });

  it('그룹이 없으면 빈 상태를 표시한다', () => {
    renderPanel([{ group_name: '', node_count: 5 }]);
    expect(screen.getByTestId('group-management-empty')).toBeInTheDocument();
  });

  it('이름을 바꾸고 이름변경을 누르면 rename 을 호출한다', () => {
    renderPanel(GROUPS);
    fireEvent.change(screen.getByTestId('group-rename-input-prod'), {
      target: { value: 'production' },
    });
    fireEvent.click(screen.getByTestId('group-rename-prod'));
    expect(renameMutate).toHaveBeenCalledTimes(1);
    expect(renameMutate.mock.calls[0]![0]).toEqual({ oldName: 'prod', newName: 'production' });
  });

  it('이름이 그대로면 이름변경 버튼이 비활성화된다', () => {
    renderPanel(GROUPS);
    expect(screen.getByTestId('group-rename-prod')).toBeDisabled();
  });

  it('삭제 → 확인 시 deleteGroup 을 호출한다', () => {
    renderPanel(GROUPS);
    fireEvent.click(screen.getByTestId('group-delete-prod'));
    // 확인 다이얼로그 확정(행 '삭제' 버튼과 라벨이 겹치므로 마지막 버튼 클릭).
    const buttons = screen.getAllByRole('button');
    fireEvent.click(buttons[buttons.length - 1]!);
    expect(deleteMutate).toHaveBeenCalledTimes(1);
    expect(deleteMutate.mock.calls[0]![0]).toBe('prod');
  });

  it('일괄 업데이트 → 확인 시 목표 버전+restart 로 updateGroup 을 호출한다', () => {
    renderPanel(GROUPS);
    fireEvent.click(screen.getByTestId('group-update-restart'));
    fireEvent.click(screen.getByTestId('group-update-prod'));
    // 확인 다이얼로그 확정(기본 라벨='확인'이 아닐 수 있으므로 마지막 버튼 클릭).
    const buttons = screen.getAllByRole('button');
    fireEvent.click(buttons[buttons.length - 1]!);
    expect(updateMutate).toHaveBeenCalledTimes(1);
    expect(updateMutate.mock.calls[0]![0]).toEqual({
      name: 'prod',
      req: { version: 'v1.3.0', restart: true },
    });
  });

  it('일괄 명령 → 확인 시 도메인/액션/인자로 commandGroup 을 호출한다', () => {
    renderPanel(GROUPS);
    fireEvent.change(screen.getByTestId('group-command-group'), { target: { value: 'prod' } });
    fireEvent.change(screen.getByTestId('group-command-domain'), { target: { value: 'flow' } });
    fireEvent.change(screen.getByTestId('group-command-action'), { target: { value: 'stop' } });
    fireEvent.change(screen.getByTestId('group-command-args'), {
      target: { value: '{"id":"f1"}' },
    });
    fireEvent.click(screen.getByTestId('group-command-send'));
    // 확인 다이얼로그 확정.
    const buttons = screen.getAllByRole('button');
    fireEvent.click(buttons[buttons.length - 1]!);
    expect(commandMutate).toHaveBeenCalledTimes(1);
    expect(commandMutate.mock.calls[0]![0]).toEqual({
      name: 'prod',
      req: { domain: 'flow', action: 'stop', args: { id: 'f1' } },
    });
  });

  it('잘못된 JSON 인자는 거부하고 명령을 전송하지 않는다', () => {
    renderPanel(GROUPS);
    fireEvent.change(screen.getByTestId('group-command-group'), { target: { value: 'prod' } });
    fireEvent.change(screen.getByTestId('group-command-action'), { target: { value: 'stop' } });
    fireEvent.change(screen.getByTestId('group-command-args'), {
      target: { value: 'not-json' },
    });
    fireEvent.click(screen.getByTestId('group-command-send'));
    expect(commandMutate).not.toHaveBeenCalled();
    expect(addNotificationMock).toHaveBeenCalled();
  });

  it('대상 그룹/액션 미입력 시 전송 버튼이 비활성화된다', () => {
    renderPanel(GROUPS);
    expect(screen.getByTestId('group-command-send')).toBeDisabled();
  });

  it('그룹 생성: 이름 + 노드 선택 후 생성 시 각 노드를 새 그룹으로 배정한다', async () => {
    renderPanel(GROUPS);
    fireEvent.change(screen.getByTestId('group-create-name'), {
      target: { value: 'lobby' },
    });
    fireEvent.click(screen.getByTestId('group-create-node-n1'));
    fireEvent.click(screen.getByTestId('group-create-node-n2'));
    fireEvent.click(screen.getByTestId('group-create-submit'));
    await waitFor(() => expect(setNodeGroupAsync).toHaveBeenCalledTimes(2));
    expect(setNodeGroupAsync).toHaveBeenCalledWith({ instanceID: 'n1', groupName: 'lobby' });
    expect(setNodeGroupAsync).toHaveBeenCalledWith({ instanceID: 'n2', groupName: 'lobby' });
  });

  it('그룹 생성: 이름 비었거나 노드 미선택이면 버튼 비활성', () => {
    renderPanel(GROUPS);
    // 초기: 이름 없음 + 선택 없음 → 비활성.
    expect(screen.getByTestId('group-create-submit')).toBeDisabled();
    // 이름만 입력 → 여전히 비활성(노드 미선택).
    fireEvent.change(screen.getByTestId('group-create-name'), { target: { value: 'x' } });
    expect(screen.getByTestId('group-create-submit')).toBeDisabled();
  });

  it('그룹 생성: 노드가 없으면 안내 메시지를 표시한다', () => {
    renderPanel(GROUPS, []);
    expect(screen.getByTestId('group-create-nonodes')).toBeInTheDocument();
  });
});
