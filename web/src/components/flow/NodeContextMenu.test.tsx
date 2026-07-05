// NodeContextMenu 컴포넌트 테스트.
//
// 검증 대상:
//   - "복제" / "삭제" 메뉴 항목 렌더링
//   - "들어가기" 항목: onEnter 가 전달될 때만 렌더, 클릭 시 onEnter 호출
//   - "복제" 클릭 → onDuplicate 호출
//   - "삭제" 클릭 → onDelete 호출
//   - Escape 키 → onClose 호출
//   - 외부 클릭 → onClose 호출

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

// i18n 은 키를 그대로 반환하도록 모킹한다(메뉴 항목 라벨은 t() 키로 노출된다).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { NodeContextMenu } from './NodeContextMenu';

describe('NodeContextMenu', () => {
  function setup(overrides?: { onEnter?: () => void }) {
    const onDuplicate = vi.fn();
    const onDelete = vi.fn();
    const onClose = vi.fn();
    render(
      <NodeContextMenu
        x={50}
        y={80}
        onEnter={overrides?.onEnter}
        onDuplicate={onDuplicate}
        onDelete={onDelete}
        onClose={onClose}
      />,
    );
    return { onDuplicate, onDelete, onClose };
  }

  it('"복제"와 "삭제" 항목을 렌더링한다', () => {
    setup();
    expect(screen.getByText('editor.node.duplicate')).toBeInTheDocument();
    expect(screen.getByText('editor.node.delete')).toBeInTheDocument();
  });

  it('onEnter 가 없으면 "들어가기" 항목을 렌더링하지 않는다', () => {
    setup();
    expect(screen.queryByText('editor.node.enter')).not.toBeInTheDocument();
  });

  it('onEnter 가 있으면 "들어가기" 항목을 렌더링한다', () => {
    setup({ onEnter: vi.fn() });
    expect(screen.getByText('editor.node.enter')).toBeInTheDocument();
  });

  it('"들어가기" 클릭 시 onEnter 를 호출한다', () => {
    const onEnter = vi.fn();
    setup({ onEnter });
    fireEvent.click(screen.getByText('editor.node.enter'));
    expect(onEnter).toHaveBeenCalledTimes(1);
  });

  it('"복제" 클릭 시 onDuplicate 를 호출한다', () => {
    const { onDuplicate } = setup();
    fireEvent.click(screen.getByText('editor.node.duplicate'));
    expect(onDuplicate).toHaveBeenCalledTimes(1);
  });

  it('"삭제" 클릭 시 onDelete 를 호출한다', () => {
    const { onDelete } = setup();
    fireEvent.click(screen.getByText('editor.node.delete'));
    expect(onDelete).toHaveBeenCalledTimes(1);
  });

  it('Escape 키 입력 시 onClose 를 호출한다', () => {
    const { onClose } = setup();
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('메뉴 외부 클릭 시 onClose 를 호출한다', () => {
    const { onClose } = setup();
    // 메뉴 바깥(document.body)에서 mousedown 발생.
    fireEvent.mouseDown(document.body);
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
