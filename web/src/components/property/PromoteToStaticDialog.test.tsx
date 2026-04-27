// PromoteToStaticDialog 컴포넌트 테스트.
//
// 검증 대상:
//   - isOpen=false 일 때 렌더링되지 않음 / true 일 때 렌더링됨
//   - 키 이름이 읽기 전용으로 표시됨
//   - Esc 키로 모달 닫힘
//   - 배경 클릭 시 모달 닫힘
//   - 태그 추가 → 변환 버튼 → onConfirm 이 태그 맵으로 호출됨
//   - 잘못된 형식의 태그 키 입력 시 경고 표시 + 변환 버튼 비활성화
//   - 태그 없이 변환 버튼 → onConfirm 이 빈 객체로 호출됨
//   - 부분 입력(키만 있고 값 없음) 시 변환 버튼 비활성화
//   - isSubmitting=true 일 때 변환 버튼 비활성 + 스피너 + 취소/닫기 비활성
//
// @spec SPEC-STORE-003

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import { PromoteToStaticDialog } from './PromoteToStaticDialog';

describe('PromoteToStaticDialog', () => {
  it('isOpen=false 면 렌더링되지 않는다', () => {
    const { container } = render(
      <PromoteToStaticDialog
        isOpen={false}
        onClose={vi.fn()}
        keyName="indoor:1:room_temp"
        onConfirm={vi.fn()}
      />,
    );
    expect(container.firstChild).toBeNull();
  });

  it('isOpen=true 면 모달과 키 이름이 표시된다', () => {
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="indoor:1:room_temp"
        onConfirm={vi.fn()}
      />,
    );
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText('정적 키로 변환')).toBeInTheDocument();
    expect(screen.getByText('indoor:1:room_temp')).toBeInTheDocument();
    // 초기 상태: 태그 행 없음
    expect(screen.getByText('태그가 없습니다')).toBeInTheDocument();
  });

  it('Esc 키로 모달이 닫힌다', () => {
    const onClose = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={onClose}
        keyName="some:key"
        onConfirm={vi.fn()}
      />,
    );
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('배경 클릭 시 모달이 닫힌다', () => {
    const onClose = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={onClose}
        keyName="some:key"
        onConfirm={vi.fn()}
      />,
    );
    // 다이얼로그 요소(배경)를 직접 클릭
    const dialog = screen.getByRole('dialog');
    fireEvent.click(dialog);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('취소 버튼 클릭 시 onClose 호출', () => {
    const onClose = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={onClose}
        keyName="some:key"
        onConfirm={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: '취소' }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('태그 없이 변환 → onConfirm 이 빈 객체로 호출된다', () => {
    const onConfirm = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="some:key"
        onConfirm={onConfirm}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /변환/ }));
    expect(onConfirm).toHaveBeenCalledWith({});
  });

  it('태그 추가 → 변환 → onConfirm 이 태그 맵으로 호출된다', () => {
    const onConfirm = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="some:key"
        onConfirm={onConfirm}
      />,
    );
    // 태그 행 추가
    fireEvent.click(screen.getByRole('button', { name: /태그 추가/ }));

    // 키와 값 입력
    const keyInput = screen.getByLabelText('태그 키');
    const valInput = screen.getByLabelText('태그 값');
    fireEvent.change(keyInput, { target: { value: 'room' } });
    fireEvent.change(valInput, { target: { value: 'kitchen' } });

    // 변환 클릭
    fireEvent.click(screen.getByRole('button', { name: /^변환$/ }));
    expect(onConfirm).toHaveBeenCalledWith({ room: 'kitchen' });
  });

  it('잘못된 태그 키 형식(예: room.1)은 경고 표시 + 변환 비활성', () => {
    const onConfirm = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="some:key"
        onConfirm={onConfirm}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /태그 추가/ }));
    const keyInput = screen.getByLabelText('태그 키');
    const valInput = screen.getByLabelText('태그 값');
    fireEvent.change(keyInput, { target: { value: 'room.1' } });
    fireEvent.change(valInput, { target: { value: 'kitchen' } });

    // 경고 메시지가 보여야 함
    expect(
      screen.getByText('태그 키는 영문/숫자/언더스코어/하이픈만 허용됩니다'),
    ).toBeInTheDocument();

    // 변환 버튼이 비활성화되어 있어야 함
    const confirmBtn = screen.getByRole('button', { name: /^변환$/ }) as HTMLButtonElement;
    expect(confirmBtn.disabled).toBe(true);

    fireEvent.click(confirmBtn);
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it('태그 키만 입력하고 값을 비워두면 변환 버튼이 비활성화된다', () => {
    const onConfirm = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="some:key"
        onConfirm={onConfirm}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /태그 추가/ }));
    const keyInput = screen.getByLabelText('태그 키');
    fireEvent.change(keyInput, { target: { value: 'room' } });
    // 값은 비워둠

    expect(screen.getByText('태그 값을 입력하세요')).toBeInTheDocument();

    const confirmBtn = screen.getByRole('button', { name: /^변환$/ }) as HTMLButtonElement;
    expect(confirmBtn.disabled).toBe(true);
  });

  it('isSubmitting=true 일 때 변환 버튼 비활성 + 스피너 + 텍스트 변경', () => {
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="some:key"
        onConfirm={vi.fn()}
        isSubmitting={true}
      />,
    );
    const confirmBtn = screen.getByRole('button', { name: /변환 중/ }) as HTMLButtonElement;
    expect(confirmBtn.disabled).toBe(true);
    expect(screen.getByText('변환 중...')).toBeInTheDocument();
  });

  it('isSubmitting=true 일 때 Esc 와 배경 클릭이 무시된다', () => {
    const onClose = vi.fn();
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={onClose}
        keyName="some:key"
        onConfirm={vi.fn()}
        isSubmitting={true}
      />,
    );
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('dialog'));
    expect(onClose).not.toHaveBeenCalled();
  });

  it('태그 행 삭제 버튼 클릭 시 행이 제거된다', () => {
    render(
      <PromoteToStaticDialog
        isOpen={true}
        onClose={vi.fn()}
        keyName="some:key"
        onConfirm={vi.fn()}
      />,
    );
    fireEvent.click(screen.getByRole('button', { name: /태그 추가/ }));
    expect(screen.getByLabelText('태그 키')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '태그 삭제' }));
    expect(screen.queryByLabelText('태그 키')).not.toBeInTheDocument();
    // 삭제 후 빈 상태 메시지 다시 표시
    expect(screen.getByText('태그가 없습니다')).toBeInTheDocument();
  });
});
