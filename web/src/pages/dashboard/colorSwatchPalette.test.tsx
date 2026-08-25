// 컬러 스와치 팝오버의 배치 테스트.
//
// 보고된 결함: 표(`max-h-[45vh] overflow-auto`) 안에서 팝오버가 아래로 열려
// 스크롤 컨테이너에 잘렸다. absolute 배치는 그 컨테이너를 벗어날 수 없다.

import { describe, it, expect, vi, afterEach } from 'vitest';
import { fireEvent, render, screen, cleanup } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import ColorSwatchButton from './colorSwatchPalette';

afterEach(cleanup);

/** 트리거의 화면 좌표를 고정한다(jsdom 은 레이아웃을 계산하지 않는다). */
function stubTrigger(rect: Partial<DOMRect>): void {
  const btn = screen.getByTestId('sw') as HTMLElement;
  btn.getBoundingClientRect = () =>
    ({ top: 0, bottom: 0, left: 0, right: 0, width: 14, height: 14, ...rect }) as DOMRect;
}

function openPopover(): HTMLElement {
  fireEvent.click(screen.getByTestId('sw'));
  return screen.getByRole('dialog');
}

describe('ColorSwatchButton — 팝오버 배치', () => {
  it('스크롤 컨테이너에 잘리지 않도록 fixed 로 띄운다', () => {
    render(<ColorSwatchButton color={undefined} onChange={() => {}} ariaLabel="색" testId="sw" />);
    stubTrigger({ top: 100, bottom: 114, right: 400 });
    expect(openPopover().style.position).toBe('fixed');
  });

  it('아래 공간이 부족하면 위로 뒤집는다', () => {
    window.innerHeight = 500;
    render(<ColorSwatchButton color={undefined} onChange={() => {}} ariaLabel="색" testId="sw" />);
    // 트리거가 화면 아래쪽에 있어 아래로 열면 잘린다.
    stubTrigger({ top: 470, bottom: 484, right: 400 });
    const pop = openPopover();
    // 위로 뒤집히면 top 이 트리거보다 위(=470 미만)에 놓인다.
    expect(parseFloat(pop.style.top)).toBeLessThan(470);
  });

  it('아래 공간이 넉넉하면 아래로 연다', () => {
    window.innerHeight = 900;
    render(<ColorSwatchButton color={undefined} onChange={() => {}} ariaLabel="색" testId="sw" />);
    stubTrigger({ top: 100, bottom: 114, right: 400 });
    expect(parseFloat(openPopover().style.top)).toBeGreaterThanOrEqual(114);
  });

  it('스크롤하면 닫는다 — fixed 는 스크롤을 따라가지 않는다', () => {
    render(<ColorSwatchButton color={undefined} onChange={() => {}} ariaLabel="색" testId="sw" />);
    stubTrigger({ top: 100, bottom: 114, right: 400 });
    openPopover();
    fireEvent.scroll(document, {});
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('왼쪽으로 넘치지 않게 가둔다', () => {
    render(<ColorSwatchButton color={undefined} onChange={() => {}} ariaLabel="색" testId="sw" />);
    // 트리거가 화면 맨 왼쪽 — 우측 정렬하면 음수 left 가 된다.
    stubTrigger({ top: 100, bottom: 114, right: 20 });
    expect(parseFloat(openPopover().style.left)).toBeGreaterThanOrEqual(0);
  });

  it('색을 고르면 값을 올리고 닫는다', () => {
    const onChange = vi.fn();
    render(<ColorSwatchButton color={undefined} onChange={onChange} ariaLabel="색" testId="sw" />);
    stubTrigger({ top: 100, bottom: 114, right: 400 });
    const pop = openPopover();
    const first = pop.querySelector('button') as HTMLButtonElement;
    fireEvent.click(first);
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('dialog')).toBeNull();
  });
});
