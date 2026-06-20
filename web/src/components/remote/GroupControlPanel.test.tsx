// GroupControlPanel 테스트 (단일 그룹 제어).
import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import type { ManagedNode, ReleaseRecord } from '@/types/remote';

const renameMutate = vi.hoisted(() => vi.fn());
const deleteMutate = vi.hoisted(() => vi.fn());
const updateMutate = vi.hoisted(() => vi.fn());
const commandMutate = vi.hoisted(() => vi.fn());

// 스토어 릴리스: amd64 는 v1.3.0/v1.2.0 모두, arm 은 v1.2.0 만 자산을 가진다.
// → per_arch 기본값: linux/amd64=v1.3.0(최신), linux/arm=v1.2.0.
// → pin v1.3.0 선택 시 linux/arm 이 건너뜀 경고에 잡힌다.
const releasesData = vi.hoisted<() => ReleaseRecord[]>(() => () => [
  {
    version: 'v1.3.0',
    channel: 'stable',
    notes: '',
    published_at: 200,
    assets: [
      { os: 'linux', arch: 'amd64', filename: 'x', size: 1, sha256: '', has_sig: true, uploaded_at: 0 },
    ],
  },
  {
    version: 'v1.2.0',
    channel: 'stable',
    notes: '',
    published_at: 100,
    assets: [
      { os: 'linux', arch: 'amd64', filename: 'x', size: 1, sha256: '', has_sig: true, uploaded_at: 0 },
      { os: 'linux', arch: 'arm', filename: 'x', size: 1, sha256: '', has_sig: true, uploaded_at: 0 },
    ],
  },
]);

vi.mock('@/hooks/useRemote', () => ({
  useTargetVersion: () => ({ data: { version: 'v1.3.0' } }),
  useReleases: () => ({ data: releasesData(), isLoading: false }),
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

  it('기본(pin) 전략 → 확인 시 목표 버전+restart 로 pin 요청을 만든다(하위 호환)', () => {
    renderPanel('prod');
    fireEvent.click(screen.getByTestId('group-control-restart'));
    fireEvent.click(screen.getByTestId('group-control-update'));
    const buttons = screen.getAllByRole('button');
    fireEvent.click(buttons[buttons.length - 1]!);
    expect(updateMutate.mock.calls[0]![0]).toEqual({
      name: 'prod',
      req: { strategy: 'pin', version: 'v1.3.0', restart: true },
    });
  });

  it('latest 전략 → 확인 시 strategy/channel 로 요청한다', () => {
    renderPanel('prod');
    fireEvent.click(screen.getByTestId('group-control-strategy-latest'));
    fireEvent.change(screen.getByTestId('group-control-channel'), {
      target: { value: 'beta' },
    });
    fireEvent.click(screen.getByTestId('group-control-update'));
    const buttons = screen.getAllByRole('button');
    fireEvent.click(buttons[buttons.length - 1]!);
    expect(updateMutate.mock.calls[0]![0]).toEqual({
      name: 'prod',
      req: { strategy: 'latest', channel: 'beta', restart: false },
    });
  });

  it('per_arch 전략 → 확인 시 version_by_arch 매핑으로 요청한다', () => {
    renderPanel('prod');
    fireEvent.click(screen.getByTestId('group-control-strategy-per_arch'));
    fireEvent.click(screen.getByTestId('group-control-update'));
    const buttons = screen.getAllByRole('button');
    fireEvent.click(buttons[buttons.length - 1]!);
    expect(updateMutate.mock.calls[0]![0]).toEqual({
      name: 'prod',
      req: {
        strategy: 'per_arch',
        version_by_arch: { 'linux/amd64': 'v1.3.0', 'linux/arm': 'v1.2.0' },
        restart: false,
      },
    });
  });

  it('pin 전략에서 자산 없는 아키텍처에 대한 건너뜀 경고를 표시한다', () => {
    renderPanel('prod');
    // 기본이 pin. v1.3.0 선택 시 linux/arm 자산이 없어 경고가 나타난다.
    fireEvent.change(screen.getByTestId('group-control-pin-version'), {
      target: { value: 'v1.3.0' },
    });
    const warn = screen.getByTestId('group-control-missing-warn');
    expect(warn.textContent).toContain('linux/arm');
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
