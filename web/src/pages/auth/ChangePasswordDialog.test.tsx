// 비밀번호 변경 다이얼로그 테스트.
//
// 이 화면에는 원래 확인란과 불일치 검증이 있었고, 이번에 표시/숨김 토글만
// 더해졌다. 따라서 검증 대상은 두 갈래다 — 토글이 세 입력에서 서로 독립으로
// 동작하는지, 그리고 기존 검증·제출 동작이 그대로인지.
//
// 최소 길이(4자)는 관리자 경로(8자)와 다르다. 서버가 의도적으로 비대칭이므로
// (internal/auth/password.go vs internal/api/handler/user.go) 여기서도 4자를
// 유지하는지 함께 잠근다.

import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import ChangePasswordDialog from './ChangePasswordDialog';

const changePassword = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/authService', async () => {
  const actual =
    await vi.importActual<typeof import('@/services/api/authService')>(
      '@/services/api/authService',
    );
  return { ...actual, changePassword };
});

function renderDialog(open = true) {
  const onClose = vi.fn();
  const result = render(
    <I18nProvider>
      <ChangePasswordDialog open={open} onClose={onClose} />
    </I18nProvider>,
  );
  return { ...result, onClose };
}

/** 세 입력을 라벨로 집는다. */
const current = () => screen.getByLabelText('현재 비밀번호');
const next = () => screen.getByLabelText('새 비밀번호');
const confirm = () => screen.getByLabelText('비밀번호 확인');

beforeEach(() => {
  vi.clearAllMocks();
  changePassword.mockResolvedValue(undefined);
});

describe('ChangePasswordDialog 표시 토글', () => {
  it('세 입력 모두 기본은 가려진 상태다', () => {
    renderDialog();

    for (const input of [current(), next(), confirm()]) {
      expect(input).toHaveAttribute('type', 'password');
    }
    expect(screen.getAllByRole('button', { name: /비밀번호 표시/ })).toHaveLength(3);
  });

  it('한 입력을 드러내도 나머지는 가려진 채로 남는다', () => {
    renderDialog();

    fireEvent.click(screen.getByRole('button', { name: '비밀번호 표시 새 비밀번호' }));

    expect(next()).toHaveAttribute('type', 'text');
    // 토글이 컴포넌트 바깥의 상태를 공유하면 여기서 함께 드러난다.
    expect(current()).toHaveAttribute('type', 'password');
    expect(confirm()).toHaveAttribute('type', 'password');
  });

  it('세 입력을 각각 독립적으로 드러내고 되돌릴 수 있다', () => {
    renderDialog();

    fireEvent.click(screen.getByRole('button', { name: '비밀번호 표시 현재 비밀번호' }));
    fireEvent.click(screen.getByRole('button', { name: '비밀번호 표시 비밀번호 확인' }));

    expect(current()).toHaveAttribute('type', 'text');
    expect(next()).toHaveAttribute('type', 'password');
    expect(confirm()).toHaveAttribute('type', 'text');

    // 드러낸 것만 되돌린다.
    fireEvent.click(screen.getByRole('button', { name: '비밀번호 숨기기 현재 비밀번호' }));
    expect(current()).toHaveAttribute('type', 'password');
    expect(confirm()).toHaveAttribute('type', 'text');
  });

  it('토글을 눌러도 폼이 제출되지 않는다', () => {
    renderDialog();

    fireEvent.click(screen.getByRole('button', { name: '비밀번호 표시 현재 비밀번호' }));

    expect(changePassword).not.toHaveBeenCalled();
  });

  it('다이얼로그를 닫았다 다시 열면 표시 상태가 초기화된다', () => {
    const onClose = vi.fn();
    const { rerender } = render(
      <I18nProvider>
        <ChangePasswordDialog open onClose={onClose} />
      </I18nProvider>,
    );

    fireEvent.change(next(), { target: { value: 'revealed-secret' } });
    fireEvent.click(screen.getByRole('button', { name: '비밀번호 표시 새 비밀번호' }));
    expect(next()).toHaveAttribute('type', 'text');

    // 닫고 다시 연다 — 이전에 드러낸 비밀번호가 그대로 보이면 유출이다.
    const view = (open: boolean) => (
      <I18nProvider>
        <ChangePasswordDialog open={open} onClose={onClose} />
      </I18nProvider>
    );
    rerender(view(false));
    rerender(view(true));

    expect(next()).toHaveAttribute('type', 'password');
    expect(
      screen.getByRole('button', { name: '비밀번호 표시 새 비밀번호' }),
    ).toHaveAttribute('aria-pressed', 'false');

    // 참고: 표시 상태는 초기화되지만 입력 값 자체는 이 경로에서 남는다.
    // 이 컴포넌트는 open=false 일 때 null 을 반환할 뿐 언마운트되지 않으므로
    // 자신의 useState 는 유지되고, 자식인 PasswordField 만 언마운트되어 표시
    // 상태가 초기화된다. 값 초기화는 handleClose -> resetForm 이 담당하며
    // 현재 호출부(SidebarUserMenu)는 항상 그 경로로 닫으므로 실제로 값이 남는
    // 경우는 없다. 부모가 handleClose 를 거치지 않고 open 을 내리도록 바뀌면
    // 값이 남게 되므로, 그때는 resetForm 을 open 변화에 묶어야 한다.
  });
});

describe('ChangePasswordDialog 기존 검증 (회귀 방지)', () => {
  it('새 비밀번호와 확인이 다르면 요청하지 않고 사유를 알린다', async () => {
    renderDialog();

    fireEvent.change(current(), { target: { value: 'old-password' } });
    fireEvent.change(next(), { target: { value: 'new-password' } });
    fireEvent.change(confirm(), { target: { value: 'other-password' } });
    fireEvent.click(screen.getByRole('button', { name: '확인' }));

    expect(await screen.findByRole('alert')).toHaveTextContent(
      '비밀번호가 일치하지 않습니다',
    );
    expect(changePassword).not.toHaveBeenCalled();
  });

  // 본인 변경 경로의 하한은 4자다 — 관리자 경로(8자)와 다르며, 서버가 그렇다.
  it('4자 미만이면 요청하지 않는다', async () => {
    renderDialog();

    fireEvent.change(current(), { target: { value: 'old-password' } });
    fireEvent.change(next(), { target: { value: 'abc' } });
    fireEvent.change(confirm(), { target: { value: 'abc' } });
    fireEvent.click(screen.getByRole('button', { name: '확인' }));

    expect(await screen.findByRole('alert')).toHaveTextContent(
      '비밀번호는 4자 이상이어야 합니다',
    );
    expect(changePassword).not.toHaveBeenCalled();
  });

  it('4자 이상 8자 미만도 받아들인다 — 관리자 경로 규칙을 끌어오지 않는다', async () => {
    renderDialog();

    fireEvent.change(current(), { target: { value: 'old-password' } });
    fireEvent.change(next(), { target: { value: 'abcd' } });
    fireEvent.change(confirm(), { target: { value: 'abcd' } });
    fireEvent.click(screen.getByRole('button', { name: '확인' }));

    await waitFor(() =>
      expect(changePassword).toHaveBeenCalledWith({
        current_password: 'old-password',
        new_password: 'abcd',
      }),
    );
  });

  it('입력이 맞으면 현재/새 비밀번호를 담아 변경을 요청한다', async () => {
    renderDialog();

    fireEvent.change(current(), { target: { value: 'old-password' } });
    fireEvent.change(next(), { target: { value: 'new-password' } });
    fireEvent.change(confirm(), { target: { value: 'new-password' } });
    fireEvent.click(screen.getByRole('button', { name: '확인' }));

    await waitFor(() =>
      expect(changePassword).toHaveBeenCalledWith({
        current_password: 'old-password',
        new_password: 'new-password',
      }),
    );
    expect(await screen.findByRole('status')).toHaveTextContent(
      '비밀번호가 변경되었습니다',
    );
  });

  it('서버가 거부하면 실패를 알린다', async () => {
    changePassword.mockRejectedValue(new Error('nope'));
    renderDialog();

    fireEvent.change(current(), { target: { value: 'old-password' } });
    fireEvent.change(next(), { target: { value: 'new-password' } });
    fireEvent.change(confirm(), { target: { value: 'new-password' } });
    fireEvent.click(screen.getByRole('button', { name: '확인' }));

    expect(await screen.findByRole('alert')).toHaveTextContent(
      '비밀번호 변경에 실패했습니다',
    );
  });

  it('open 이 false 면 아무것도 그리지 않는다', () => {
    const { container } = renderDialog(false);
    expect(container).toBeEmptyDOMElement();
  });

  it('autoComplete 힌트가 경로에 맞게 유지된다', () => {
    renderDialog();

    expect(current()).toHaveAttribute('autocomplete', 'current-password');
    expect(next()).toHaveAttribute('autocomplete', 'new-password');
    expect(confirm()).toHaveAttribute('autocomplete', 'new-password');
  });
});
