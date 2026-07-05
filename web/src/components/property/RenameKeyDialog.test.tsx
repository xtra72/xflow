// RenameKeyDialog 컴포넌트 테스트.
//
// 검증 대상:
//   - isOpen=false 일 때 렌더링되지 않음
//   - 현재 키를 사전 채움 → 변경 없으면 확인 비활성
//   - 새 값 입력 시 확인 활성 → onConfirm(newKey)
//   - 빈 값이면 확인 비활성
//   - 취소/닫기 호출
//
// @spec SPEC-STORE-004

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import RenameKeyDialog from './RenameKeyDialog';

vi.mock('@/lib/i18n', async () => {
  const ko = (await import('@/lib/i18n/ko.json')).default as Record<string, unknown>;
  const resolve = (key: string): string => {
    const v = key.split('.').reduce<unknown>(
      (o, p) => (o && typeof o === 'object' ? (o as Record<string, unknown>)[p] : undefined),
      ko,
    );
    return typeof v === 'string' ? v : key;
  };
  return {
    useTranslation: () => ({ t: resolve, locale: 'ko' as const, setLocale: () => {} }),
  };
});

describe('RenameKeyDialog', () => {
  it('isOpen=false 면 렌더링되지 않는다', () => {
    const { container } = render(
      <RenameKeyDialog isOpen={false} onClose={vi.fn()} currentKey="room" onConfirm={vi.fn()} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it('현재 키를 사전 채우고, 변경 없으면 확인 버튼이 비활성이다', () => {
    render(<RenameKeyDialog isOpen onClose={vi.fn()} currentKey="room" onConfirm={vi.fn()} />);
    const input = screen.getByTestId('rename-key-input') as HTMLInputElement;
    expect(input.value).toBe('room');
    expect(screen.getByTestId('rename-key-confirm')).toBeDisabled();
  });

  it('새 값 입력 시 확인 활성 → onConfirm(newKey) 호출', () => {
    const onConfirm = vi.fn();
    render(<RenameKeyDialog isOpen onClose={vi.fn()} currentKey="room" onConfirm={onConfirm} />);
    const input = screen.getByTestId('rename-key-input');
    fireEvent.change(input, { target: { value: 'living' } });
    const confirm = screen.getByTestId('rename-key-confirm');
    expect(confirm).not.toBeDisabled();
    fireEvent.click(confirm);
    expect(onConfirm).toHaveBeenCalledWith('living');
  });

  it('빈 값이면 확인 비활성', () => {
    render(<RenameKeyDialog isOpen onClose={vi.fn()} currentKey="room" onConfirm={vi.fn()} />);
    fireEvent.change(screen.getByTestId('rename-key-input'), { target: { value: '  ' } });
    expect(screen.getByTestId('rename-key-confirm')).toBeDisabled();
  });

  it('Enter 로 새 키를 확정한다', () => {
    const onConfirm = vi.fn();
    render(<RenameKeyDialog isOpen onClose={vi.fn()} currentKey="room" onConfirm={onConfirm} />);
    const input = screen.getByTestId('rename-key-input');
    fireEvent.change(input, { target: { value: 'living' } });
    fireEvent.keyDown(input, { key: 'Enter' });
    expect(onConfirm).toHaveBeenCalledWith('living');
  });

  it('isSubmitting 이면 확인/입력이 비활성', () => {
    render(
      <RenameKeyDialog isOpen onClose={vi.fn()} currentKey="room" onConfirm={vi.fn()} isSubmitting />,
    );
    fireEvent.change(screen.getByTestId('rename-key-input'), { target: { value: 'living' } });
    expect(screen.getByTestId('rename-key-confirm')).toBeDisabled();
  });
});
