// FlowSettingsDialog 의 이름·설명 편집 + 표시 토글 테스트.
//
// 검증 대상:
//   - 헤더 제목이 "플로우 설정" 으로 표시된다.
//   - 이름/설명 input 이 서버 값으로 채워진다(useFlow).
//   - 이름 blur 시 trim 후 변경되었으면 updateFlow.mutate({name}) 호출.
//   - 이름이 빈 값이면 저장하지 않고 원래 이름으로 복원한다.
//   - 설명 blur 시 변경되었으면 updateFlow.mutate({description}) 호출(빈 값 허용).
//   - 표시 토글(포트 이름/통계)이 계속 동작한다.

import { describe, expect, it, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import type { FlowInfo } from '@/types/flow';

// 플로우 훅 모킹 — useFlow 는 고정 데이터, useUpdateFlow 는 mutate 스파이.
const updateMutate = vi.fn();
const flow: FlowInfo = {
  id: 'flow-1',
  name: '원래 이름',
  description: '원래 설명',
  status: 'stored',
  node_count: 0,
};

vi.mock('@/hooks/useFlow', () => ({
  useFlow: () => ({ data: flow }),
  useUpdateFlow: () => ({ mutate: updateMutate, isPending: false }),
}));

import { FlowSettingsDialog } from './FlowSettingsDialog';

const renderDialog = () =>
  render(
    <FlowSettingsDialog isOpen onClose={vi.fn()} flowId="flow-1" />,
  );

describe('FlowSettingsDialog - 이름·설명 편집', () => {
  beforeEach(() => {
    updateMutate.mockClear();
  });

  it('헤더 제목이 "플로우 설정" 으로 표시된다', () => {
    renderDialog();
    expect(
      screen.getByRole('heading', { name: '플로우 설정' }),
    ).toBeInTheDocument();
  });

  it('이름/설명 input 이 서버 값으로 채워진다', () => {
    renderDialog();
    expect(screen.getByLabelText('플로우 이름')).toHaveValue('원래 이름');
    expect(screen.getByLabelText('설명')).toHaveValue('원래 설명');
  });

  it('이름을 변경하고 blur 하면 updateFlow.mutate({name}) 를 호출한다', () => {
    renderDialog();

    const input = screen.getByLabelText('플로우 이름');
    fireEvent.change(input, { target: { value: '  새 이름  ' } });
    fireEvent.blur(input);

    expect(updateMutate).toHaveBeenCalledTimes(1);
    expect(updateMutate).toHaveBeenCalledWith({
      id: 'flow-1',
      req: { name: '새 이름' },
    });
  });

  it('이름이 변경되지 않았으면 저장하지 않는다', () => {
    renderDialog();

    const input = screen.getByLabelText('플로우 이름');
    fireEvent.blur(input);

    expect(updateMutate).not.toHaveBeenCalled();
  });

  it('이름이 빈 값이면 저장하지 않고 원래 이름으로 복원한다', () => {
    renderDialog();

    const input = screen.getByLabelText('플로우 이름');
    fireEvent.change(input, { target: { value: '   ' } });
    fireEvent.blur(input);

    expect(updateMutate).not.toHaveBeenCalled();
    expect(input).toHaveValue('원래 이름');
  });

  it('설명을 변경하고 blur 하면 updateFlow.mutate({description}) 를 호출한다', () => {
    renderDialog();

    const textarea = screen.getByLabelText('설명');
    fireEvent.change(textarea, { target: { value: '바뀐 설명' } });
    fireEvent.blur(textarea);

    expect(updateMutate).toHaveBeenCalledWith({
      id: 'flow-1',
      req: { description: '바뀐 설명' },
    });
  });

  it('설명을 비워서 blur 하면 빈 설명으로 저장한다', () => {
    renderDialog();

    const textarea = screen.getByLabelText('설명');
    fireEvent.change(textarea, { target: { value: '' } });
    fireEvent.blur(textarea);

    expect(updateMutate).toHaveBeenCalledWith({
      id: 'flow-1',
      req: { description: '' },
    });
  });

  it('표시 토글(포트 이름/통계)을 계속 렌더링한다', () => {
    renderDialog();
    expect(screen.getByText('포트 이름 표시')).toBeInTheDocument();
    expect(screen.getByText('포트별 통계 표시')).toBeInTheDocument();
  });
});
