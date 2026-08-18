// 드래그 핸들(제어 바) 레이아웃 비침습 테스트.
//
// 회귀 방지 대상: 핸들이 흐름 요소로 돌아가면 편집 모드 진입 시 패널 본문이 핸들 높이만큼
// 줄어들어 보이는 그림이 달라진다(히트맵 레터박스/차트 축 재계산).

import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';

import DragHandle from './DragHandle';

describe('DragHandle', () => {
  it('absolute 오버레이라 패널 본문 높이를 잠식하지 않는다', () => {
    render(<DragHandle />);
    const handle = screen.getByTestId('dashboard-drag-handle');
    expect(handle.className).toContain('absolute');
    expect(handle.className).toContain('top-0');
    expect(handle.className).toContain('inset-x-0');
  });

  it('GridLayout 드래그 셀렉터 클래스를 유지한다', () => {
    render(<DragHandle />);
    // dragConfig.handle = '.dashboard-drag-handle' — 클래스가 바뀌면 드래그가 죽는다.
    expect(screen.getByTestId('dashboard-drag-handle').className).toContain(
      'dashboard-drag-handle',
    );
  });

  it('패널 본문 위에 겹치도록 쌓임 순서를 갖는다', () => {
    render(<DragHandle />);
    expect(screen.getByTestId('dashboard-drag-handle').className).toContain('z-10');
  });
});
