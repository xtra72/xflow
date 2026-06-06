// RemoteNodesPage 테스트 (SPEC-REMOTE-001 M5, G01 + G02).
//
// useRemote 훅과 uiStore(toast)를 mock 하여 결정적으로 검증한다.
// 범위:
//   - G01: 노드 목록 렌더 + online/offline 인디케이터 + 상태 배지
//   - G02: 승인 → mutation 호출, 거부/폐기 → 확인 다이얼로그 → mutation + toast

import { render, screen, fireEvent, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import type { ManagedNode } from '@/types/remote';

// ---- useRemote mock ----
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
  usePreRegisterNode: () => ({
    mutateAsync: preRegisterMutateAsyncMock,
    isPending: false,
  }),
}));

// EnrollmentTokenSection 은 자체 훅을 사용하므로 스텁으로 격리한다 (별도 테스트로 검증).
vi.mock('@/components/remote/EnrollmentTokenSection', () => ({
  EnrollmentTokenSection: () => null,
}));

// ---- uiStore mock (toast 캡처) ----
const addNotificationMock = vi.hoisted(() => vi.fn());
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector?: (s: { addNotification: typeof addNotificationMock }) => unknown) => {
    const state = { addNotification: addNotificationMock };
    return selector ? selector(state) : state;
  },
}));

import RemoteNodesPage from './RemoteNodesPage';

function renderPage() {
  return render(
    <I18nProvider>
      <RemoteNodesPage />
    </I18nProvider>,
  );
}

function makeNode(overrides: Partial<ManagedNode> = {}): ManagedNode {
  return {
    instance_id: 'node-1',
    hostname: 'host-1',
    version: 'v1.0.0',
    status: 'approved',
    online: true,
    last_seen: Date.now(),
    ...overrides,
  };
}

beforeEach(() => {
  useManagedNodesMock.mockReset();
  useRemoteModeMock.mockReset();
  approveMutateMock.mockReset();
  rejectMutateMock.mockReset();
  revokeMutateMock.mockReset();
  deleteMutateMock.mockReset();
  preRegisterMutateAsyncMock.mockReset();
  addNotificationMock.mockReset();
  // 기본: server 모드 (M5 동작과 동일).
  useRemoteModeMock.mockReturnValue({ data: { mode: 'server' } });
  useManagedNodesMock.mockReturnValue({
    data: [],
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  });
});

// G01 — 목록 렌더링
describe('RemoteNodesPage — G01 목록', () => {
  it('노드 행을 렌더하고 online/offline 인디케이터와 상태 배지를 표시한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [
        makeNode({ instance_id: 'n-online', online: true, status: 'approved' }),
        makeNode({ instance_id: 'n-offline', online: false, status: 'pending' }),
      ],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });

    renderPage();

    const rows = screen.getAllByTestId('remote-node-row');
    expect(rows).toHaveLength(2);

    const onlineRow = screen.getByText('n-online').closest('tr')!;
    expect(within(onlineRow).getByTestId('node-online-indicator')).toHaveAttribute(
      'data-online',
      'true',
    );
    expect(within(onlineRow).getByTestId('node-status-badge')).toHaveAttribute(
      'data-status',
      'approved',
    );

    const offlineRow = screen.getByText('n-offline').closest('tr')!;
    expect(within(offlineRow).getByTestId('node-online-indicator')).toHaveAttribute(
      'data-online',
      'false',
    );
    expect(within(offlineRow).getByTestId('node-status-badge')).toHaveAttribute(
      'data-status',
      'pending',
    );
  });

  it('노드가 없으면 빈 상태를 표시한다', () => {
    renderPage();
    expect(screen.getByTestId('remote-nodes-empty')).toBeInTheDocument();
  });

  it('로딩 중에는 스켈레톤을 표시한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();
    expect(screen.getByTestId('remote-nodes-loading')).toBeInTheDocument();
  });

  it('에러 시 재시도 버튼이 refetch 를 호출한다', () => {
    const refetch = vi.fn();
    useManagedNodesMock.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('boom'),
      refetch,
    });
    renderPage();
    fireEvent.click(screen.getByRole('button', { name: /다시 시도/ }));
    expect(refetch).toHaveBeenCalledTimes(1);
  });
});

// 모드 게이팅 — server 모드가 아니면 안내 표시 + 노드 쿼리 비활성
describe('RemoteNodesPage — 모드 게이팅', () => {
  it.each(['disabled', 'client'] as const)(
    'mode=%s 이면 안내를 표시하고 노드 쿼리를 비활성화한다',
    (mode) => {
      useRemoteModeMock.mockReturnValue({ data: { mode } });
      useManagedNodesMock.mockReturnValue({
        data: undefined,
        isLoading: false,
        error: null,
        refetch: vi.fn(),
      });
      renderPage();

      // 안내 패널이 표시된다.
      expect(screen.getByTestId('remote-not-server')).toBeInTheDocument();
      // 노드 목록 테이블/빈 상태는 렌더되지 않는다.
      expect(screen.queryByTestId('remote-nodes-empty')).not.toBeInTheDocument();
      expect(screen.queryByTestId('remote-node-row')).not.toBeInTheDocument();
      // 노드 쿼리는 enabled=false 로 호출되어 발행되지 않는다.
      const lastCall = useManagedNodesMock.mock.calls.at(-1)!;
      expect(lastCall[1]).toBe(false);
    },
  );

  it('mode 미확정(로딩) 중에는 스켈레톤을 표시한다', () => {
    useRemoteModeMock.mockReturnValue({ data: undefined });
    useManagedNodesMock.mockReturnValue({
      data: undefined,
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();
    expect(screen.getByTestId('remote-nodes-loading')).toBeInTheDocument();
  });
});

// G02 — 승인/거부/폐기
describe('RemoteNodesPage — G02 액션', () => {
  it('pending 노드의 승인 버튼은 approve mutation 을 호출한다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [makeNode({ instance_id: 'p-1', status: 'pending' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    fireEvent.click(screen.getByTestId('node-approve-button'));
    expect(approveMutateMock).toHaveBeenCalledTimes(1);
    expect(approveMutateMock.mock.calls[0]![0]).toBe('p-1');
  });

  it('거부 버튼은 확인 다이얼로그를 열고, 확정 시 reject mutation + toast 를 호출한다', () => {
    rejectMutateMock.mockImplementation(
      (_vars: unknown, opts?: { onSuccess?: () => void }) => opts?.onSuccess?.(),
    );
    useManagedNodesMock.mockReturnValue({
      data: [makeNode({ instance_id: 'p-1', status: 'pending' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    // 확인 다이얼로그는 아직 없음.
    expect(screen.queryByTestId('confirm-dialog')).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId('node-reject-button'));
    expect(screen.getByTestId('confirm-dialog')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('confirm-dialog-confirm'));
    expect(rejectMutateMock).toHaveBeenCalledTimes(1);
    expect(rejectMutateMock.mock.calls[0]![0]).toEqual({ instanceID: 'p-1' });
    expect(addNotificationMock).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'success' }),
    );
  });

  it('approved 노드의 폐기 버튼은 확인 후 revoke mutation 을 호출한다', () => {
    revokeMutateMock.mockImplementation(
      (_vars: unknown, opts?: { onSuccess?: () => void }) => opts?.onSuccess?.(),
    );
    useManagedNodesMock.mockReturnValue({
      data: [makeNode({ instance_id: 'a-1', status: 'approved' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    fireEvent.click(screen.getByTestId('node-revoke-button'));
    expect(screen.getByTestId('confirm-dialog')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('confirm-dialog-confirm'));
    expect(revokeMutateMock).toHaveBeenCalledTimes(1);
    expect(revokeMutateMock.mock.calls[0]![0]).toBe('a-1');
  });

  it('확인 다이얼로그 취소 시 mutation 을 호출하지 않는다', () => {
    useManagedNodesMock.mockReturnValue({
      data: [makeNode({ instance_id: 'a-1', status: 'approved' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    fireEvent.click(screen.getByTestId('node-revoke-button'));
    fireEvent.click(screen.getByRole('button', { name: /취소/ }));
    expect(revokeMutateMock).not.toHaveBeenCalled();
    expect(screen.queryByTestId('confirm-dialog')).not.toBeInTheDocument();
  });
});

// 노드 삭제 — 확인 다이얼로그 → delete mutation
describe('RemoteNodesPage — 노드 삭제', () => {
  it('삭제 버튼은 확인 후 delete mutation 을 호출한다 (폐기와 구별)', () => {
    deleteMutateMock.mockImplementation(
      (_vars: unknown, opts?: { onSuccess?: () => void }) => opts?.onSuccess?.(),
    );
    useManagedNodesMock.mockReturnValue({
      data: [makeNode({ instance_id: 'd-1', status: 'rejected' })],
      isLoading: false,
      error: null,
      refetch: vi.fn(),
    });
    renderPage();

    fireEvent.click(screen.getByTestId('node-delete-button'));
    expect(screen.getByTestId('confirm-dialog')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('confirm-dialog-confirm'));
    expect(deleteMutateMock).toHaveBeenCalledTimes(1);
    expect(deleteMutateMock.mock.calls[0]![0]).toBe('d-1');
    expect(revokeMutateMock).not.toHaveBeenCalled();
    expect(addNotificationMock).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'success' }),
    );
  });
});

// 노드 사전 등록 — 모달 → preRegister mutation + 토스트
describe('RemoteNodesPage — 사전 등록', () => {
  it('사전 등록 버튼은 모달을 열고, 제출 시 preRegister 를 호출하고 토스트를 띄운다', async () => {
    preRegisterMutateAsyncMock.mockResolvedValue({
      instance_id: 'new-1',
      hostname: '',
      version: '',
      status: 'approved',
      online: false,
      last_seen: 0,
    });
    renderPage();

    expect(screen.queryByTestId('pre-register-dialog')).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId('node-pre-register-button'));
    expect(screen.getByTestId('pre-register-dialog')).toBeInTheDocument();

    fireEvent.change(screen.getByTestId('pre-register-instance-id'), {
      target: { value: 'new-1' },
    });
    fireEvent.click(screen.getByTestId('pre-register-submit'));

    await vi.waitFor(() => {
      expect(preRegisterMutateAsyncMock).toHaveBeenCalledWith({ instance_id: 'new-1' });
    });
    await vi.waitFor(() => {
      expect(addNotificationMock).toHaveBeenCalledWith(
        expect.objectContaining({ type: 'success' }),
      );
    });
  });

  it('instance_id 미입력 시 검증 에러를 표시하고 mutation 을 호출하지 않는다', () => {
    renderPage();
    fireEvent.click(screen.getByTestId('node-pre-register-button'));
    fireEvent.click(screen.getByTestId('pre-register-submit'));

    expect(screen.getByTestId('pre-register-error')).toBeInTheDocument();
    expect(preRegisterMutateAsyncMock).not.toHaveBeenCalled();
  });

  it('409 중복 에러는 명확한 한글 메시지로 표시된다', async () => {
    const { APIError } = await import('@/types/api');
    preRegisterMutateAsyncMock.mockRejectedValue(
      new APIError('CONFLICT', 'duplicate', 409),
    );
    renderPage();

    fireEvent.click(screen.getByTestId('node-pre-register-button'));
    fireEvent.change(screen.getByTestId('pre-register-instance-id'), {
      target: { value: 'dup-1' },
    });
    fireEvent.click(screen.getByTestId('pre-register-submit'));

    await vi.waitFor(() => {
      expect(screen.getByTestId('pre-register-error')).toHaveTextContent('이미 존재');
    });
    // 모달은 닫히지 않고 에러를 표시한 채 유지된다.
    expect(screen.getByTestId('pre-register-dialog')).toBeInTheDocument();
  });
});
