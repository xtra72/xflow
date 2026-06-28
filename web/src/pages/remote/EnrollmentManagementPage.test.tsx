// EnrollmentManagementPage 테스트 (SPEC-REMOTE-001 M9, 그룹 K, REQ-K15).
//
// 범위:
//   - 노드 등록 관리: 승인 큐 렌더 + 승인/거부 액션 + 토큰 섹션 마운트.
//   - 사전 등록 모달 진입.
//   - 비-server 모드 게이팅.
//
// EnrollmentTokenSection 은 자체 훅을 쓰므로 스텁으로 격리(별도 테스트로 검증).

import { fireEvent, render, screen, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import type { ManagedNode } from '@/types/remote';

const useManagedNodesMock = vi.hoisted(() => vi.fn());
const useRemoteModeMock = vi.hoisted(() => vi.fn());
const approveMutateMock = vi.hoisted(() => vi.fn());
const rejectMutateMock = vi.hoisted(() => vi.fn());
const revokeMutateMock = vi.hoisted(() => vi.fn());
const deleteMutateMock = vi.hoisted(() => vi.fn());
const preRegisterMutateAsyncMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useRemote', () => ({
  useManagedNodes: useManagedNodesMock,
  useRemoteMode: useRemoteModeMock,
  useApproveNode: () => ({ mutate: approveMutateMock, isPending: false, variables: undefined }),
  useRejectNode: () => ({ mutate: rejectMutateMock, isPending: false, variables: undefined }),
  useRevokeNode: () => ({ mutate: revokeMutateMock, isPending: false, variables: undefined }),
  useDeleteNode: () => ({ mutate: deleteMutateMock, isPending: false, variables: undefined }),
  usePreRegisterNode: () => ({ mutateAsync: preRegisterMutateAsyncMock, isPending: false }),
}));

vi.mock('@/components/remote/EnrollmentTokenSection', () => ({
  EnrollmentTokenSection: () => <div data-testid="enrollment-token-section" />,
}));

vi.mock('@/components/remote/RemoteNotServerNotice', () => ({
  RemoteNotServerNotice: () => <div data-testid="not-server" />,
}));

const addNotificationMock = vi.hoisted(() => vi.fn());
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector?: (s: { addNotification: typeof addNotificationMock }) => unknown) => {
    const state = { addNotification: addNotificationMock };
    return selector ? selector(state) : state;
  },
}));

import EnrollmentManagementPage from './EnrollmentManagementPage';

function makeNode(o: Partial<ManagedNode> = {}): ManagedNode {
  return {
    instance_id: 'node-a',
    hostname: 'gw-1',
    version: '1.0',
    status: 'pending',
    online: false,
    group_name: '',
    last_seen: 0,
    ...o,
  };
}

function renderPage() {
  return render(
    <I18nProvider>
      <EnrollmentManagementPage />
    </I18nProvider>,
  );
}

beforeEach(() => {
  useManagedNodesMock.mockReset().mockReturnValue({
    data: [],
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  });
  useRemoteModeMock.mockReset().mockReturnValue({ data: { mode: 'server' } });
  approveMutateMock.mockReset();
  rejectMutateMock.mockReset();
  preRegisterMutateAsyncMock.mockReset();
  addNotificationMock.mockReset();
});

describe('EnrollmentManagementPage', () => {
  it('토큰 섹션과 등록 관리 헤더를 렌더한다', () => {
    renderPage();
    expect(screen.getByTestId('enrollment-management-header')).toBeInTheDocument();
    expect(screen.getByTestId('enrollment-token-section')).toBeInTheDocument();
  });

  it('pending 노드의 승인 버튼이 approve mutation 을 호출한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [makeNode({ instance_id: 'p-1', status: 'pending' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    fireEvent.click(screen.getByTestId('node-approve-button'));
    expect(approveMutateMock).toHaveBeenCalledTimes(1);
    expect(approveMutateMock).toHaveBeenNthCalledWith(1, 'p-1', expect.anything());
  });

  it('거부는 확인 다이얼로그를 거쳐 reject mutation 을 호출한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [makeNode({ instance_id: 'p-1', status: 'pending' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    fireEvent.click(screen.getByTestId('node-reject-button'));
    // 확인 다이얼로그의 확정 버튼(거부 라벨).
    const dialog = screen.getByRole('dialog');
    fireEvent.click(within(dialog).getByText('거부'));
    expect(rejectMutateMock).toHaveBeenCalledTimes(1);
  });

  it('사전 등록 버튼이 모달을 연다', () => {
    renderPage();
    fireEvent.click(screen.getByTestId('node-pre-register-button'));
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('server 모드가 아니면 안내만 표시하고 노드 쿼리를 막는다', () => {
    useRemoteModeMock.mockReturnValue({ data: { mode: 'client' } });
    renderPage();
    expect(screen.getByTestId('not-server')).toBeInTheDocument();
    expect(useManagedNodesMock).toHaveBeenCalledWith(undefined, false);
  });
});
