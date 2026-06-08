// EnrollmentTokenSection 테스트 (수동 enrollment).
//
// useRemote 훅과 uiStore(toast), navigator.clipboard 를 mock 하여 검증한다.
// 범위:
//   - 토큰 목록 렌더 (id/label/상태)
//   - 발급: 모달 제출 → create mutation → raw 토큰 1회 표시 + 복사
//   - 폐기: 행 액션 → 확인 다이얼로그 → revoke mutation + toast

import { render, screen, fireEvent, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import type { EnrollmentToken } from '@/types/remote';

// ---- useRemote mock ----
const useEnrollmentTokensMock = vi.hoisted(() => vi.fn());
const createMutateAsyncMock = vi.hoisted(() => vi.fn());
const revokeMutateMock = vi.hoisted(() => vi.fn());

vi.mock('@/hooks/useRemote', () => ({
  useEnrollmentTokens: useEnrollmentTokensMock,
  useCreateEnrollmentToken: () => ({
    mutateAsync: createMutateAsyncMock,
    isPending: false,
  }),
  useRevokeEnrollmentToken: () => ({ mutate: revokeMutateMock, isPending: false }),
}));

// ---- uiStore mock (toast 캡처) ----
const addNotificationMock = vi.hoisted(() => vi.fn());
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (selector?: (s: { addNotification: typeof addNotificationMock }) => unknown) => {
    const state = { addNotification: addNotificationMock };
    return selector ? selector(state) : state;
  },
}));

import { EnrollmentTokenSection } from './EnrollmentTokenSection';

function renderSection() {
  return render(
    <I18nProvider>
      <EnrollmentTokenSection enabled={true} />
    </I18nProvider>,
  );
}

function makeToken(overrides: Partial<EnrollmentToken> = {}): EnrollmentToken {
  return {
    id: 'tok-abcdefgh-1234',
    label: '1층 노드용',
    created_at: Date.now(),
    uses: 0,
    revoked: false,
    ...overrides,
  };
}

beforeEach(() => {
  useEnrollmentTokensMock.mockReset();
  createMutateAsyncMock.mockReset();
  revokeMutateMock.mockReset();
  addNotificationMock.mockReset();
  useEnrollmentTokensMock.mockReturnValue({ data: [], isLoading: false, error: null });

  // navigator.clipboard mock.
  Object.assign(navigator, {
    clipboard: { writeText: vi.fn().mockResolvedValue(undefined) },
  });
});

describe('EnrollmentTokenSection — 목록', () => {
  it('토큰이 없으면 빈 상태를 표시한다', () => {
    renderSection();
    expect(screen.getByTestId('enrollment-tokens-empty')).toBeInTheDocument();
  });

  it('토큰 행을 렌더하고 상태 배지를 표시한다', () => {
    useEnrollmentTokensMock.mockReturnValue({
      data: [
        makeToken({ id: 't-active', revoked: false }),
        makeToken({ id: 't-revoked', revoked: true }),
      ],
      isLoading: false,
      error: null,
    });
    renderSection();

    const rows = screen.getAllByTestId('enrollment-token-row');
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveAttribute('data-token-id', 't-active');
    expect(rows[1]).toHaveAttribute('data-token-id', 't-revoked');

    const states = screen.getAllByTestId('enrollment-token-state');
    expect(states.map((s) => s.getAttribute('data-state'))).toEqual(['active', 'revoked']);
  });
});

describe('EnrollmentTokenSection — 발급', () => {
  it('발급 모달 제출 시 create 를 호출하고 raw 토큰을 1회 표시한다 + 복사', async () => {
    createMutateAsyncMock.mockResolvedValue({
      id: 'tok-1',
      token: 'RAW-SECRET-TOKEN',
      label: '1층',
    });
    renderSection();

    fireEvent.click(screen.getByTestId('enrollment-create-button'));
    expect(screen.getByTestId('create-enrollment-token-dialog')).toBeInTheDocument();

    fireEvent.change(screen.getByTestId('create-token-label'), {
      target: { value: '1층' },
    });
    fireEvent.change(screen.getByTestId('create-token-expires'), {
      target: { value: '24h' },
    });
    fireEvent.click(screen.getByTestId('create-token-submit'));

    // create mutation 이 매핑된 expires_in 으로 호출된다.
    await vi.waitFor(() => {
      expect(createMutateAsyncMock).toHaveBeenCalledWith({
        label: '1층',
        expires_in: '24h',
      });
    });

    // raw 토큰이 1회 표시 모달에 나타난다.
    await vi.waitFor(() => {
      expect(screen.getByTestId('enrollment-token-created-dialog')).toBeInTheDocument();
    });
    const tokenField = screen.getByTestId('enrollment-token-value') as HTMLInputElement;
    expect(tokenField.value).toBe('RAW-SECRET-TOKEN');

    // 복사 버튼이 clipboard.writeText 를 호출한다.
    fireEvent.click(screen.getByTestId('enrollment-token-copy'));
    await vi.waitFor(() => {
      expect(navigator.clipboard.writeText).toHaveBeenCalledWith('RAW-SECRET-TOKEN');
    });

    // 닫으면 모달이 사라진다 (토큰은 다시 표시되지 않는다).
    fireEvent.click(screen.getByTestId('enrollment-token-created-done'));
    expect(
      screen.queryByTestId('enrollment-token-created-dialog'),
    ).not.toBeInTheDocument();
  });

  it('설정 스니펫에 raw 토큰이 포함된다', async () => {
    createMutateAsyncMock.mockResolvedValue({ id: 'tok-1', token: 'SNIP-TOKEN' });
    renderSection();

    fireEvent.click(screen.getByTestId('enrollment-create-button'));
    fireEvent.click(screen.getByTestId('create-token-submit'));

    await vi.waitFor(() => {
      expect(screen.getByTestId('enrollment-snippet')).toBeInTheDocument();
    });
    expect(screen.getByTestId('enrollment-snippet')).toHaveTextContent('SNIP-TOKEN');
    expect(screen.getByTestId('enrollment-snippet')).toHaveTextContent('mode: client');
  });
});

describe('EnrollmentTokenSection — 폐기', () => {
  it('행 폐기 액션은 확인 다이얼로그를 열고, 확정 시 revoke + toast 를 호출한다', () => {
    revokeMutateMock.mockImplementation(
      (_id: string, opts?: { onSuccess?: () => void }) => opts?.onSuccess?.(),
    );
    useEnrollmentTokensMock.mockReturnValue({
      data: [makeToken({ id: 'tok-revoke-me' })],
      isLoading: false,
      error: null,
    });
    renderSection();

    fireEvent.click(screen.getByTestId('enrollment-token-revoke-button'));
    const dialog = screen.getByTestId('confirm-dialog');
    expect(dialog).toBeInTheDocument();

    fireEvent.click(within(dialog).getByTestId('confirm-dialog-confirm'));
    expect(revokeMutateMock).toHaveBeenCalledTimes(1);
    expect(revokeMutateMock.mock.calls[0]![0]).toBe('tok-revoke-me');
    expect(addNotificationMock).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'success' }),
    );
  });

  it('폐기된 토큰 행에는 폐기 버튼이 없다', () => {
    useEnrollmentTokensMock.mockReturnValue({
      data: [makeToken({ id: 'tok-gone', revoked: true })],
      isLoading: false,
      error: null,
    });
    renderSection();
    expect(
      screen.queryByTestId('enrollment-token-revoke-button'),
    ).not.toBeInTheDocument();
  });
});
