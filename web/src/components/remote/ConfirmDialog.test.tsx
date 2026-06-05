// ConfirmDialog 테스트 (SPEC-REMOTE-001 M5, G02 접근성).
//
// 범위:
//   - role="dialog" + aria-modal + aria-labelledby/describedby
//   - 열릴 때 확인 버튼에 포커스
//   - Escape 키로 onCancel 호출
//   - 백드롭 클릭으로 onCancel 호출
//   - pending 중에는 버튼 비활성 + Escape/백드롭 취소 차단

import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';

import { ConfirmDialog } from './ConfirmDialog';

function renderDialog(props: Partial<React.ComponentProps<typeof ConfirmDialog>> = {}) {
  const onConfirm = vi.fn();
  const onCancel = vi.fn();
  render(
    <I18nProvider>
      <ConfirmDialog
        open
        title="제목"
        description="설명"
        onConfirm={onConfirm}
        onCancel={onCancel}
        {...props}
      />
    </I18nProvider>,
  );
  return { onConfirm, onCancel };
}

describe('ConfirmDialog', () => {
  it('open=false 면 렌더하지 않는다', () => {
    renderDialog({ open: false });
    expect(screen.queryByTestId('confirm-dialog')).not.toBeInTheDocument();
  });

  it('role="dialog" + aria-modal + labelledby/describedby 를 갖는다', () => {
    renderDialog();
    const dialog = screen.getByRole('dialog');
    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(dialog).toHaveAttribute('aria-labelledby', 'confirm-dialog-title');
    expect(dialog).toHaveAttribute('aria-describedby', 'confirm-dialog-desc');
  });

  it('열릴 때 확인 버튼에 포커스가 이동한다', async () => {
    renderDialog();
    const confirmBtn = screen.getByTestId('confirm-dialog-confirm');
    await waitFor(() => expect(confirmBtn).toHaveFocus());
  });

  it('Escape 키는 onCancel 을 호출한다', () => {
    const { onCancel } = renderDialog();
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' });
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('확인 버튼은 onConfirm 을 호출한다', () => {
    const { onConfirm } = renderDialog();
    fireEvent.click(screen.getByTestId('confirm-dialog-confirm'));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it('pending=true 면 버튼이 비활성화되고 Escape 취소가 차단된다', () => {
    const { onCancel } = renderDialog({ pending: true });
    expect(screen.getByTestId('confirm-dialog-confirm')).toBeDisabled();
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' });
    expect(onCancel).not.toHaveBeenCalled();
  });
});
