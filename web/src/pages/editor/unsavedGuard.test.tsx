// 미저장 변경 이탈 가드(useBlocker + ConfirmDialog) 배선 테스트.
//
// useBlocker 자체는 데이터 라우터 컨텍스트를 요구해 단위 테스트가 까다롭다.
// 여기서는 EditorPage 가 blocker 상태를 ConfirmDialog 에 매핑하는 "배선"을
// 동일한 형태로 재현한 작은 컴포넌트로 검증한다.
//
//   - blocker.state === 'blocked' 일 때만 다이얼로그가 열린다.
//   - 확인("이동") → blocker.proceed() 호출.
//   - 취소("취소") → blocker.reset() 호출.
//   - isDirty=false 면(차단되지 않음) 다이얼로그가 닫혀 있다.

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import { ConfirmDialog } from '@/components/property/ConfirmDialog';

// EditorPage 의 blocker → ConfirmDialog 배선을 그대로 옮긴 테스트용 컴포넌트.
interface FakeBlocker {
  state: 'blocked' | 'unblocked' | 'proceeding';
  proceed?: () => void;
  reset?: () => void;
}

function UnsavedGuard({ blocker }: { blocker: FakeBlocker }) {
  return (
    <ConfirmDialog
      isOpen={blocker.state === 'blocked'}
      onClose={() => blocker.reset?.()}
      onConfirm={() => blocker.proceed?.()}
      title="저장하지 않은 변경사항"
      message="저장하지 않은 변경사항이 있습니다. 저장하지 않고 이동하시겠습니까?"
      confirmLabel="이동"
      cancelLabel="취소"
      variant="danger"
    />
  );
}

describe('미저장 변경 이탈 가드', () => {
  it('차단 상태가 아니면 다이얼로그를 표시하지 않는다', () => {
    render(<UnsavedGuard blocker={{ state: 'unblocked' }} />);
    expect(
      screen.queryByText('저장하지 않은 변경사항'),
    ).not.toBeInTheDocument();
  });

  it('차단 상태이면 경고 다이얼로그를 표시한다', () => {
    render(
      <UnsavedGuard
        blocker={{ state: 'blocked', proceed: vi.fn(), reset: vi.fn() }}
      />,
    );
    expect(screen.getByText('저장하지 않은 변경사항')).toBeInTheDocument();
    expect(
      screen.getByText(
        '저장하지 않은 변경사항이 있습니다. 저장하지 않고 이동하시겠습니까?',
      ),
    ).toBeInTheDocument();
  });

  it('"이동" 클릭 시 blocker.proceed 를 호출한다', () => {
    const proceed = vi.fn();
    const reset = vi.fn();
    render(<UnsavedGuard blocker={{ state: 'blocked', proceed, reset }} />);

    fireEvent.click(screen.getByRole('button', { name: '이동' }));
    expect(proceed).toHaveBeenCalledTimes(1);
    expect(reset).not.toHaveBeenCalled();
  });

  it('"취소" 클릭 시 blocker.reset 을 호출한다', () => {
    const proceed = vi.fn();
    const reset = vi.fn();
    render(<UnsavedGuard blocker={{ state: 'blocked', proceed, reset }} />);

    fireEvent.click(screen.getByRole('button', { name: '취소' }));
    expect(reset).toHaveBeenCalledTimes(1);
    expect(proceed).not.toHaveBeenCalled();
  });
});
