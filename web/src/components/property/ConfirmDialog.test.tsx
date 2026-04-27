// ConfirmDialog 컴포넌트 테스트.
//
// 검증 대상:
//   - isOpen=false 일 때 렌더링되지 않음 / true 일 때 렌더링됨
//   - title 과 message 가 표시됨 (문자열 + ReactNode)
//   - 확인 버튼 클릭 → onConfirm 호출
//   - 취소 버튼 클릭 → onClose 호출
//   - Esc 키 → onClose 호출 (제출 중 아님)
//   - 배경 클릭 → onClose 호출 (제출 중 아님)
//   - isSubmitting=true 일 때:
//     - 양쪽 버튼 비활성
//     - 확인 버튼 스피너 표시
//     - Esc / 배경 클릭 무시
//   - variant='danger' 일 때 confirm 버튼이 빨간 색상 토큰 사용
//   - confirmLabel / cancelLabel 커스터마이즈 적용
//
// @spec SPEC-STORE-003

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import { ConfirmDialog } from './ConfirmDialog';

describe('ConfirmDialog', () => {
  it('isOpen=false 면 렌더링되지 않는다', () => {
    const { container } = render(
      <ConfirmDialog
        isOpen={false}
        onClose={vi.fn()}
        onConfirm={vi.fn()}
        title="제목"
        message="본문"
      />,
    );
    expect(container.firstChild).toBeNull();
  });

  it('isOpen=true 면 모달 + title + 문자열 message 가 표시된다', () => {
    render(
      <ConfirmDialog
        isOpen={true}
        onClose={vi.fn()}
        onConfirm={vi.fn()}
        title="저장소 전체 초기화"
        message="이 작업은 되돌릴 수 없습니다."
      />,
    );
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText('저장소 전체 초기화')).toBeInTheDocument();
    expect(screen.getByText('이 작업은 되돌릴 수 없습니다.')).toBeInTheDocument();
  });

  it('ReactNode 형태의 message 도 그대로 렌더링된다', () => {
    render(
      <ConfirmDialog
        isOpen={true}
        onClose={vi.fn()}
        onConfirm={vi.fn()}
        title="확인"
        message={
          <div>
            <p data-testid="custom-message">커스텀 메시지</p>
            <ul>
              <li>항목 1</li>
              <li>항목 2</li>
            </ul>
          </div>
        }
      />,
    );
    expect(screen.getByTestId('custom-message')).toBeInTheDocument();
    expect(screen.getByText('항목 1')).toBeInTheDocument();
    expect(screen.getByText('항목 2')).toBeInTheDocument();
  });

  it('확인 버튼 클릭 시 onConfirm 이 호출된다', () => {
    const onConfirm = vi.fn();
    render(
      <ConfirmDialog
        isOpen={true}
        onClose={vi.fn()}
        onConfirm={onConfirm}
        title="제목"
        message="본문"
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: '확인' }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it('취소 버튼 클릭 시 onClose 가 호출된다', () => {
    const onClose = vi.fn();
    render(
      <ConfirmDialog
        isOpen={true}
        onClose={onClose}
        onConfirm={vi.fn()}
        title="제목"
        message="본문"
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: '취소' }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('Esc 키로 onClose 가 호출된다 (제출 중 아님)', () => {
    const onClose = vi.fn();
    render(
      <ConfirmDialog
        isOpen={true}
        onClose={onClose}
        onConfirm={vi.fn()}
        title="제목"
        message="본문"
      />,
    );
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('배경 클릭 시 onClose 가 호출된다 (제출 중 아님)', () => {
    const onClose = vi.fn();
    render(
      <ConfirmDialog
        isOpen={true}
        onClose={onClose}
        onConfirm={vi.fn()}
        title="제목"
        message="본문"
      />,
    );
    const dialog = screen.getByRole('dialog');
    fireEvent.click(dialog);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('isSubmitting=true 일 때 양쪽 버튼이 비활성화되고 스피너가 표시된다', () => {
    render(
      <ConfirmDialog
        isOpen={true}
        onClose={vi.fn()}
        onConfirm={vi.fn()}
        title="제목"
        message="본문"
        confirmLabel="초기화"
        isSubmitting={true}
      />,
    );
    const confirmBtn = screen.getByRole('button', {
      name: /초기화/,
    }) as HTMLButtonElement;
    const cancelBtn = screen.getByRole('button', {
      name: '취소',
    }) as HTMLButtonElement;
    expect(confirmBtn.disabled).toBe(true);
    expect(cancelBtn.disabled).toBe(true);
    // Loader2 아이콘은 svg.animate-spin 으로 렌더된다.
    expect(confirmBtn.querySelector('svg.animate-spin')).not.toBeNull();
  });

  it('isSubmitting=true 일 때 Esc 와 배경 클릭이 무시된다', () => {
    const onClose = vi.fn();
    render(
      <ConfirmDialog
        isOpen={true}
        onClose={onClose}
        onConfirm={vi.fn()}
        title="제목"
        message="본문"
        isSubmitting={true}
      />,
    );
    fireEvent.keyDown(window, { key: 'Escape' });
    fireEvent.click(screen.getByRole('dialog'));
    expect(onClose).not.toHaveBeenCalled();
  });

  it('isSubmitting=true 일 때 확인 버튼 클릭은 onConfirm 을 호출하지 않는다', () => {
    const onConfirm = vi.fn();
    render(
      <ConfirmDialog
        isOpen={true}
        onClose={vi.fn()}
        onConfirm={onConfirm}
        title="제목"
        message="본문"
        confirmLabel="초기화"
        isSubmitting={true}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /초기화/ }));
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("variant='danger' 면 confirm 버튼에 빨간 배경 클래스를 적용한다", () => {
    render(
      <ConfirmDialog
        isOpen={true}
        onClose={vi.fn()}
        onConfirm={vi.fn()}
        title="제목"
        message="본문"
        confirmLabel="초기화"
        variant="danger"
      />,
    );
    const confirmBtn = screen.getByRole('button', { name: '초기화' });
    expect(confirmBtn.className).toMatch(/bg-red-/);
  });

  it("variant='default'(기본값) 일 때는 파란 배경 클래스를 적용한다", () => {
    render(
      <ConfirmDialog
        isOpen={true}
        onClose={vi.fn()}
        onConfirm={vi.fn()}
        title="제목"
        message="본문"
      />,
    );
    const confirmBtn = screen.getByRole('button', { name: '확인' });
    expect(confirmBtn.className).toMatch(/bg-blue-/);
  });

  it('confirmLabel / cancelLabel 을 커스터마이즈할 수 있다', () => {
    render(
      <ConfirmDialog
        isOpen={true}
        onClose={vi.fn()}
        onConfirm={vi.fn()}
        title="제목"
        message="본문"
        confirmLabel="삭제"
        cancelLabel="아니요"
      />,
    );
    expect(screen.getByRole('button', { name: '삭제' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: '아니요' })).toBeInTheDocument();
  });

  it('async onConfirm 도 await 된다', async () => {
    // resolveFn 의 정확한 타입(() => void)을 유지하면서 TS 의 narrow-to-never
    // 분석을 우회하기 위해 ref 객체 안에 보관한다.
    const resolveRef: { current: (() => void) | null } = { current: null };
    const onConfirm = vi.fn().mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          resolveRef.current = resolve;
        }),
    );
    render(
      <ConfirmDialog
        isOpen={true}
        onClose={vi.fn()}
        onConfirm={onConfirm}
        title="제목"
        message="본문"
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: '확인' }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
    // resolve 해서 dangling promise 를 정리한다.
    resolveRef.current?.();
  });
});
