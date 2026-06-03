// NodeContextMenu 컴포넌트 테스트.
//
// 검증 대상:
//   - "복제" / "삭제" 메뉴 항목 렌더링
//   - "복제" 클릭 → onDuplicate 호출
//   - "삭제" 클릭 → onDelete 호출
//   - Escape 키 → onClose 호출
//   - 외부 클릭 → onClose 호출

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import { NodeContextMenu } from './NodeContextMenu';

describe('NodeContextMenu', () => {
  function setup() {
    const onDuplicate = vi.fn();
    const onDelete = vi.fn();
    const onClose = vi.fn();
    render(
      <NodeContextMenu
        x={50}
        y={80}
        onDuplicate={onDuplicate}
        onDelete={onDelete}
        onClose={onClose}
      />,
    );
    return { onDuplicate, onDelete, onClose };
  }

  it('"복제"와 "삭제" 항목을 렌더링한다', () => {
    setup();
    expect(screen.getByText('복제')).toBeInTheDocument();
    expect(screen.getByText('삭제')).toBeInTheDocument();
  });

  it('"복제" 클릭 시 onDuplicate 를 호출한다', () => {
    const { onDuplicate } = setup();
    fireEvent.click(screen.getByText('복제'));
    expect(onDuplicate).toHaveBeenCalledTimes(1);
  });

  it('"삭제" 클릭 시 onDelete 를 호출한다', () => {
    const { onDelete } = setup();
    fireEvent.click(screen.getByText('삭제'));
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
