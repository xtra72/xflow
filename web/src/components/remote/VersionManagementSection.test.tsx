// VersionManagementSection 테스트 (버전 관리 Phase 1/2).
//
// useRemote 의 target/history 쿼리 + setTarget/update 뮤테이션, uiStore(toast)를 mock 한다.
// 범위: outdated 배지, 목표 버전 저장, 업데이트 확인→트리거, 오프라인 비활성, 이력 표시.

import { fireEvent, render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import type { NodeVersionHistoryEntry, TargetVersion } from '@/types/remote';

// ---- useRemote mock ----
const setTargetMutate = vi.hoisted(() => vi.fn());
const updateMutate = vi.hoisted(() => vi.fn());
const targetRef = vi.hoisted(() => ({ value: { version: '' } as TargetVersion }));
const historyRef = vi.hoisted(() => ({ value: [] as NodeVersionHistoryEntry[] }));

vi.mock('@/hooks/useRemote', () => ({
  useTargetVersion: () => ({ data: targetRef.value }),
  useNodeVersionHistory: () => ({ data: historyRef.value }),
  useSetTargetVersion: () => ({ mutate: setTargetMutate, isPending: false }),
  useUpdateNode: () => ({ mutate: updateMutate, isPending: false }),
}));

// ---- uiStore mock ----
const addNotificationMock = vi.hoisted(() => vi.fn());
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (
    selector?: (s: { addNotification: typeof addNotificationMock }) => unknown,
  ) => {
    const state = { addNotification: addNotificationMock };
    return selector ? selector(state) : state;
  },
}));

import { VersionManagementSection } from './VersionManagementSection';

function renderSection(props: {
  currentVersion: string;
  online: boolean;
}): void {
  render(
    <I18nProvider>
      <VersionManagementSection
        instanceId="node-1"
        currentVersion={props.currentVersion}
        online={props.online}
      />
    </I18nProvider>,
  );
}

beforeEach(() => {
  setTargetMutate.mockReset();
  updateMutate.mockReset();
  addNotificationMock.mockReset();
  targetRef.value = { version: '' };
  historyRef.value = [];
});

describe('VersionManagementSection', () => {
  it('목표 버전보다 낮으면 outdated 배지를 표시한다', () => {
    targetRef.value = { version: 'v1.3.0' };
    renderSection({ currentVersion: 'v1.0.0', online: true });
    expect(screen.getByTestId('version-outdated-badge')).toBeInTheDocument();
    expect(screen.queryByTestId('version-uptodate-badge')).toBeNull();
  });

  it('목표 버전과 같거나 높으면 up-to-date 배지를 표시한다', () => {
    targetRef.value = { version: 'v1.3.0' };
    renderSection({ currentVersion: 'v1.3.0', online: true });
    expect(screen.getByTestId('version-uptodate-badge')).toBeInTheDocument();
    expect(screen.queryByTestId('version-outdated-badge')).toBeNull();
  });

  it('목표 버전 미설정이면 어떤 버전 배지도 표시하지 않는다', () => {
    renderSection({ currentVersion: 'v1.0.0', online: true });
    expect(screen.queryByTestId('version-outdated-badge')).toBeNull();
    expect(screen.queryByTestId('version-uptodate-badge')).toBeNull();
  });

  it('목표 버전 저장 시 입력값으로 setTarget 을 호출한다', () => {
    renderSection({ currentVersion: 'v1.0.0', online: true });
    fireEvent.change(screen.getByTestId('target-version-input'), {
      target: { value: 'v2.0.0' },
    });
    fireEvent.click(screen.getByTestId('target-version-save'));
    expect(setTargetMutate).toHaveBeenCalledTimes(1);
    expect(setTargetMutate.mock.calls[0]![0]).toBe('v2.0.0');
  });

  it('업데이트 버튼 → 확인 시 target+restart 로 updateNode 를 호출한다', () => {
    targetRef.value = { version: 'v1.3.0' };
    renderSection({ currentVersion: 'v1.0.0', online: true });
    // 재시작 옵션 체크.
    fireEvent.click(screen.getByTestId('update-restart-checkbox'));
    // 업데이트 버튼 → 확인 다이얼로그.
    fireEvent.click(screen.getByTestId('update-node-button'));
    // 확인 다이얼로그의 확정 버튼(라벨='업데이트') 클릭.
    const confirmBtns = screen.getAllByRole('button', { name: '업데이트' });
    fireEvent.click(confirmBtns[confirmBtns.length - 1]!);
    expect(updateMutate).toHaveBeenCalledTimes(1);
    expect(updateMutate.mock.calls[0]![0]).toEqual({
      instanceID: 'node-1',
      req: { version: 'v1.3.0', restart: true },
    });
  });

  it('오프라인 노드는 업데이트 버튼이 비활성화된다', () => {
    renderSection({ currentVersion: 'v1.0.0', online: false });
    expect(screen.getByTestId('update-node-button')).toBeDisabled();
  });

  it('이력이 있으면 목록을, 없으면 빈 상태를 표시한다', () => {
    historyRef.value = [
      { version: 'v1.2.0', changed_at: 3000 },
      { version: 'v1.1.0', changed_at: 2000 },
    ];
    renderSection({ currentVersion: 'v1.2.0', online: true });
    const list = screen.getByTestId('version-history-list');
    expect(list).toBeInTheDocument();
    expect(within(list).getByText('v1.2.0')).toBeInTheDocument();
    expect(within(list).getByText('v1.1.0')).toBeInTheDocument();
    expect(screen.queryByTestId('version-history-empty')).toBeNull();
  });

  it('이력이 없으면 빈 상태 메시지를 표시한다', () => {
    renderSection({ currentVersion: 'v1.0.0', online: true });
    expect(screen.getByTestId('version-history-empty')).toBeInTheDocument();
    expect(screen.queryByTestId('version-history-list')).toBeNull();
  });
});
