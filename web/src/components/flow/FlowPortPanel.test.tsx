// SPEC-SUBFLOW-001 그룹 B: 플로우 포트 관리 패널 컴포넌트 테스트.
//
// 검증 대상:
//   - 입력/출력 포트 섹션 렌더
//   - 인라인 이름 변경 → renameFlowPort
//   - "삭제" → removeFlowPort
//   - 닫기 버튼 → onClose
// 포트 추가는 에디터 툴바의 빠른 추가 버튼에서 수행되므로 패널 테스트에서 제거됨.

import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import { FlowPortPanel } from './FlowPortPanel';
import { useEditorStore } from '@/stores/editorStore';

function resetStore(): void {
  useEditorStore.getState().resetEditor();
}

describe('FlowPortPanel', () => {
  beforeEach(() => {
    resetStore();
  });

  it('입력 포트 / 출력 포트 섹션을 렌더한다', () => {
    render(<FlowPortPanel onClose={vi.fn()} />);
    expect(screen.getByText('입력 포트')).toBeInTheDocument();
    expect(screen.getByText('출력 포트')).toBeInTheDocument();
  });

  it('포트 이름을 인라인 편집하면 renameFlowPort 가 반영된다', () => {
    // 미리 포트 1개를 만들어 둔다.
    useEditorStore.getState().addFlowInput();
    render(<FlowPortPanel onClose={vi.fn()} />);

    // 이름 클릭 → 편집 input 진입.
    fireEvent.click(screen.getByText('in1'));
    const input = screen.getByLabelText('포트 이름') as HTMLInputElement;
    fireEvent.change(input, { target: { value: '센서' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    expect(useEditorStore.getState().flowInputs[0]!.name).toBe('센서');
  });

  it('삭제 버튼이 removeFlowPort 로 포트를 제거한다', () => {
    useEditorStore.getState().addFlowOutput();
    render(<FlowPortPanel onClose={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: 'out1 삭제' }));

    expect(useEditorStore.getState().flowOutputs).toHaveLength(0);
  });

  it('닫기 버튼이 onClose 를 호출한다', () => {
    const onClose = vi.fn();
    render(<FlowPortPanel onClose={onClose} />);

    fireEvent.click(
      screen.getByRole('button', { name: '플로우 포트 패널 닫기' }),
    );
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('편집 중 Escape 는 이름을 변경하지 않는다', () => {
    useEditorStore.getState().addFlowInput();
    render(<FlowPortPanel onClose={vi.fn()} />);

    fireEvent.click(screen.getByText('in1'));
    const input = screen.getByLabelText('포트 이름') as HTMLInputElement;
    fireEvent.change(input, { target: { value: '취소될이름' } });
    fireEvent.keyDown(input, { key: 'Escape' });

    expect(useEditorStore.getState().flowInputs[0]!.name).toBe('in1');
  });

  it('포트가 여러 개일 때 각 행이 렌더된다', () => {
    useEditorStore.getState().addFlowInput();
    useEditorStore.getState().addFlowInput();
    render(<FlowPortPanel onClose={vi.fn()} />);

    // 입력 섹션 영역 내에 in1, in2 가 모두 보인다.
    expect(screen.getByText('in1')).toBeInTheDocument();
    expect(screen.getByText('in2')).toBeInTheDocument();
  });
});
