// 디자인 팝오버가 잘라내는 영역(설정 컬럼) 안에 들어오는지 검증한다.
//
// 종전에는 왼쪽/오른쪽 어느 변에 붙일지만 골랐다. 그러면 상자가 영역보다 좁을 때만 통하고,
// 넓은 상자는 어느 쪽에 붙여도 밖으로 나간다. 지금은 폭을 영역에 맞춰 줄인 뒤 들어오는
// 자리를 직접 계산하므로, 아래 검사는 "항상 안에 들어온다" 하나로 모인다.

import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import { DesignPopover } from './ChartPanelSections';

/** 상자와 경계 사이에 남기는 여백 — 구현과 같은 값. */
const MARGIN = 16;

/** jsdom 은 레이아웃을 계산하지 않으므로 필요한 좌표만 심는다. */
function stubRect(el: Element, rect: Partial<DOMRect>): void {
  el.getBoundingClientRect = () =>
    ({
      left: 0, right: 0, top: 0, bottom: 0, width: 0, height: 0, x: 0, y: 0,
      toJSON: () => ({}), ...rect,
    }) as DOMRect;
}

/** 왼쪽 `clipLeft`, 오른쪽 `clipRight` 인 영역 안 `triggerLeft` 자리에 단추를 둔다. */
function openAt(opts: {
  triggerLeft: number;
  clipLeft?: number;
  clipRight: number;
  clipBottom?: number;
  triggerBottom?: number;
  width?: number;
}): HTMLElement {
  const { triggerLeft, clipLeft = 0, clipRight, clipBottom = 600, triggerBottom = 20, width } = opts;
  render(
    <div data-testid="column" style={{ overflowX: 'auto' }}>
      <DesignPopover testId="t" width={width}>
        <span>내용</span>
      </DesignPopover>
    </div>,
  );
  stubRect(screen.getByTestId('column'), { left: clipLeft, right: clipRight, bottom: clipBottom });
  stubRect(screen.getByTestId('t-button').parentElement as Element, {
    left: triggerLeft,
    bottom: triggerBottom,
  });
  fireEvent.click(screen.getByTestId('t-button'));
  return screen.getByTestId('t-popover');
}

/** 팝오버의 화면상 왼쪽/오른쪽 끝. `left` 는 단추 기준 어긋남이다. */
function edges(popover: HTMLElement, triggerLeft: number): { left: number; right: number } {
  const left = triggerLeft + parseFloat(popover.style.left);
  return { left, right: left + parseFloat(popover.style.width) };
}

describe('DesignPopover 자리', () => {
  it('자리가 넉넉하면 단추 왼쪽에 맞춰 연다', () => {
    const popover = openAt({ triggerLeft: 100, clipRight: 900 });
    expect(edges(popover, 100).left).toBe(100);
    expect(popover).toHaveStyle({ width: '288px' });
  });

  it('오른쪽 경계를 넘길 자리에서는 안쪽으로 끌어당긴다', () => {
    const popover = openAt({ triggerLeft: 300, clipRight: 400 });
    const { left, right } = edges(popover, 300);
    expect(right).toBeLessThanOrEqual(400 - MARGIN);
    expect(left).toBeGreaterThanOrEqual(MARGIN);
  });

  it('영역보다 넓은 상자는 영역에 맞춰 줄인다 — 뒤집어도 들어가지 않는 상자다', () => {
    const popover = openAt({ triggerLeft: 100, clipRight: 300, width: 384 });
    // 300 - 16*2 = 268
    expect(popover).toHaveStyle({ width: '268px' });
    const { left, right } = edges(popover, 100);
    expect(left).toBeGreaterThanOrEqual(MARGIN);
    expect(right).toBeLessThanOrEqual(300 - MARGIN);
  });

  it('영역이 왼쪽에서 시작하지 않아도 안에 들어온다', () => {
    const popover = openAt({ triggerLeft: 700, clipLeft: 500, clipRight: 900, width: 384 });
    const { left, right } = edges(popover, 700);
    expect(left).toBeGreaterThanOrEqual(500 + MARGIN);
    expect(right).toBeLessThanOrEqual(900 - MARGIN);
  });

  it('단추가 영역 왼쪽 끝에 붙어 있어도 여백을 지킨다', () => {
    const popover = openAt({ triggerLeft: 0, clipLeft: 0, clipRight: 500 });
    expect(edges(popover, 0).left).toBeGreaterThanOrEqual(MARGIN);
  });

  it('실측이 안 되면 바라는 폭을 그대로 쓴다 — 자리는 단추 기준', () => {
    const popover = openAt({ triggerLeft: 0, clipRight: 0, width: 384 });
    expect(popover).toHaveStyle({ width: '384px', left: '0px' });
  });

  it('아래 여백이 좁아도 최소 높이는 준다 — 컬럼이 스크롤되므로 닿을 수 있다', () => {
    const popover = openAt({
      triggerLeft: 0,
      clipRight: 800,
      clipBottom: 100,
      triggerBottom: 95,
    });
    expect(popover).toHaveStyle({ maxHeight: '200px' });
  });

  it('아래가 넉넉하면 그만큼 높이를 준다', () => {
    const popover = openAt({ triggerLeft: 0, clipRight: 800, clipBottom: 900, triggerBottom: 100 });
    // 900 - 100 - 16
    expect(popover).toHaveStyle({ maxHeight: '784px' });
  });
});
