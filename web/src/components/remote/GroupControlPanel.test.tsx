// GroupControlPanel 테스트 (단일 그룹 제어).
import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import type { ManagedNode } from '@/types/remote';

const renameMutate = vi.hoisted(() => vi.fn());
const deleteMutate = vi.hoisted(() => vi.fn());
const updateMutate = vi.hoisted(() => vi.fn());
const commandMutate = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useRemote', () => ({
  useTargetVersion: () => ({ data: { version: 'v1.3.0' } }),
  useRenameGroup: () => ({ mutate: renameMutate, isPending: false }),
  useDeleteGroup: () => ({ mutate: deleteMutate, isPending: false }),
  useUpdateGroup: () => ({ mutate: updateMutate, isPending: false }),
  useCommandGroup: () => ({ mutate: commandMutate, isPending: false }),
}));

const addNotificationMock = vi.hoisted(() => vi.fn());
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector?: (s: { addNotification: typeof addNotificationMock }) => unknown) => {
    const state = { addNotification: addNotificationMock };
    return selector ? selector(state) : state;
  },
}));

import { GroupControlPanel } from './GroupControlPanel';

function member(id: string): ManagedNode {
  return { instance_id: id, hostname: `host-${id}`, version: 'v1', status: 'approved', online: true, last_seen: 0, group_name: 'prod' };
}

function renderPanel(groupName: string, onMutated = vi.fn(), onSelectNode = vi.fn()): void {
  render(
    <I18nProvider>
      <GroupControlPanel
        groupName={groupName}
        members={[member('p1'), member('p2')]}
        onSelectNode={onSelectNode}
        onMutated={onMutated}
      />
    </I18nProvider>,
  );
}

beforeEach(() => {
  renameMutate.mockReset();
  deleteMutate.mockReset();
  updateMutate.mockReset();
  commandMutate.mockReset();
  addNotificationMock.mockReset();
});

describe('GroupControlPanel', () => {
  it('이름변경 시 rename 을 호출한다', () => {
    renderPanel('prod');
    fireEvent.change(screen.getByTestId('group-control-rename-input'), {
      target: { value: 'production' },
    });
    fireEvent.click(screen.getByTestId('group-control-rename'));
    expect(renameMutate.mock.calls[0]![0]).toEqual({ oldName: 'prod', newName: 'production' });
  });

  it('삭제 → 확인 시 deleteGroup 을 호출한다', () => {
    renderPanel('prod');
    fireEvent.click(screen.getByTestId('group-control-delete'));
    const buttons = screen.getAllByRole('button');
    fireEvent.click(buttons[buttons.length - 1]!);
    expect(deleteMutate.mock.calls[0]![0]).toBe('prod');
  });

  it('일괄 업데이트 → 확인 시 목표 버전+restart 로 updateGroup 을 호출한다', () => {
    renderPanel('prod');
    fireEvent.click(screen.getByTestId('group-control-restart'));
    fireEvent.click(screen.getByTestId('group-control-update'));
    const buttons = screen.getAllByRole('button');
    fireEvent.click(buttons[buttons.length - 1]!);
    expect(updateMutate.mock.calls[0]![0]).toEqual({
      name: 'prod',
      req: { version: 'v1.3.0', restart: true },
    });
  });

  it('일괄 명령 → 확인 시 domain/action/args 로 commandGroup 을 호출한다', () => {
    renderPanel('prod');
    fireEvent.change(screen.getByTestId('group-control-domain'), { target: { value: 'flow' } });
    fireEvent.change(screen.getByTestId('group-control-action'), { target: { value: 'stop' } });
    fireEvent.click(screen.getByTestId('group-control-command-send'));
    const buttons = screen.getAllByRole('button');
    fireEvent.click(buttons[buttons.length - 1]!);
    expect(commandMutate.mock.calls[0]![0]).toEqual({
      name: 'prod',
      req: { domain: 'flow', action: 'stop', args: undefined },
    });
  });

  it('미분류(빈 그룹)는 이름변경/삭제를 숨기고 멤버 설명만 표시한다', () => {
    renderPanel('');
    expect(screen.queryByTestId('group-control-rename')).toBeNull();
    expect(screen.queryByTestId('group-control-delete')).toBeNull();
    // 업데이트/명령은 여전히 가능.
    expect(screen.getByTestId('group-control-update')).toBeInTheDocument();
  });

  it('멤버 노드 클릭 시 onSelectNode 를 호출한다', () => {
    const onSelectNode = vi.fn();
    renderPanel('prod', vi.fn(), onSelectNode);
    fireEvent.click(screen.getByText('host-p1'));
    expect(onSelectNode).toHaveBeenCalledWith('p1');
  });
});
