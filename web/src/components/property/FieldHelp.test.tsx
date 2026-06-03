import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

import { FieldHelp } from './FieldHelp';

describe('FieldHelp (라벨 도움말 ? 아이콘)', () => {
  it('기본 상태에서는 팝오버가 보이지 않고 ? 버튼만 존재한다', () => {
    render(<FieldHelp text="도움말 설명" />);
    expect(screen.queryByRole('tooltip')).toBeNull();
    expect(screen.getByRole('button', { name: '설명 보기' })).toBeInTheDocument();
  });

  it('? 클릭 시 설명 팝오버가 표시되고 다시 클릭하면 닫힌다', () => {
    render(<FieldHelp text="도움말 설명" />);
    const btn = screen.getByRole('button', { name: '설명 보기' });

    fireEvent.click(btn);
    const tip = screen.getByRole('tooltip');
    expect(tip).toHaveTextContent('도움말 설명');
    expect(btn).toHaveAttribute('aria-expanded', 'true');

    fireEvent.click(btn);
    expect(screen.queryByRole('tooltip')).toBeNull();
    expect(btn).toHaveAttribute('aria-expanded', 'false');
  });

  it('외부 클릭 시 팝오버가 닫힌다', () => {
    render(
      <div>
        <FieldHelp text="설명" />
        <button type="button">바깥</button>
      </div>,
    );
    fireEvent.click(screen.getByRole('button', { name: '설명 보기' }));
    expect(screen.getByRole('tooltip')).toBeInTheDocument();

    fireEvent.mouseDown(screen.getByText('바깥'));
    expect(screen.queryByRole('tooltip')).toBeNull();
  });

  it('접근성: 설명 텍스트가 describedById 로 항상 DOM 에 존재한다(sr-only)', () => {
    const { container } = render(<FieldHelp text="에이리아 설명" describedById="desc-1" />);
    const sr = container.querySelector('#desc-1');
    expect(sr).not.toBeNull();
    expect(sr).toHaveTextContent('에이리아 설명');
  });
});
