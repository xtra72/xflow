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

// i18n 은 키를 그대로 반환하도록 모킹한다(섹션 제목·버튼 라벨은 t() 키로 노출된다).
// 동적 이름 보간(예: 포트명) 은 키에 {name} 자리표시자가 그대로 남으므로,
// 보간 결과는 키 문자열과 동일하게 단언한다.
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

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
    expect(screen.getByText('editor.port.inputSection')).toBeInTheDocument();
    expect(screen.getByText('editor.port.outputSection')).toBeInTheDocument();
  });

  it('포트 이름을 인라인 편집하면 renameFlowPort 가 반영된다', () => {
    // 미리 포트 1개를 만들어 둔다.
    useEditorStore.getState().addFlowInput();
    render(<FlowPortPanel onClose={vi.fn()} />);

    // 이름 클릭 → 편집 input 진입.
    fireEvent.click(screen.getByText('in1'));
    const input = screen.getByLabelText('editor.port.nameAria') as HTMLInputElement;
    fireEvent.change(input, { target: { value: '센서' } });
    fireEvent.keyDown(input, { key: 'Enter' });

    expect(useEditorStore.getState().flowInputs[0]!.name).toBe('센서');
  });

  it('삭제 버튼이 removeFlowPort 로 포트를 제거한다', () => {
    useEditorStore.getState().addFlowOutput();
    render(<FlowPortPanel onClose={vi.fn()} />);

    // deleteAria 키에는 {name} 자리표시자가 그대로 남아 키 문자열이 접근성 이름이 된다.
    fireEvent.click(screen.getByRole('button', { name: 'editor.port.deleteAria' }));

    expect(useEditorStore.getState().flowOutputs).toHaveLength(0);
  });

  it('닫기 버튼이 onClose 를 호출한다', () => {
    const onClose = vi.fn();
    render(<FlowPortPanel onClose={onClose} />);

    fireEvent.click(
      screen.getByRole('button', { name: 'editor.port.closeAria' }),
    );
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('편집 중 Escape 는 이름을 변경하지 않는다', () => {
    useEditorStore.getState().addFlowInput();
    render(<FlowPortPanel onClose={vi.fn()} />);

    fireEvent.click(screen.getByText('in1'));
    const input = screen.getByLabelText('editor.port.nameAria') as HTMLInputElement;
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
