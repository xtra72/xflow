// 2D 판 — 좌표가 채도·명도로 바뀌는 자리.
//
// jsdom 에는 레이아웃이 없어 `getBoundingClientRect` 가 전부 0 이다. 그래서 판의 상자를
// **직접 꽂아** 잰다. 꽂지 않은 채로 재면 폭 0 으로 나누어 NaN 이 나오는데, 그 갈래도
// 아래에서 함께 본다 — 조용히 `#NaNNaNNaN` 이 흘러나가지 않아야 하기 때문이다.

import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import SaturationField from './SaturationField';
import type { Hsva } from './colorFormat';

/** 파랑(`#3b82f6`)의 HSV. 색상만 쓰므로 채도·명도는 아무 값이나 좋다. */
const BLUE_HSV: Hsva = { h: 217.2, s: 0.76, v: 0.96, a: 1 };

/** 판의 상자. 왼쪽 100 · 위 50 · 폭 200 · 높이 100. */
const RECT = {
  left: 100,
  top: 50,
  right: 300,
  bottom: 150,
  width: 200,
  height: 100,
  x: 100,
  y: 50,
  toJSON: () => ({}),
} as DOMRect;

function field(): HTMLElement {
  return screen.getByTestId('colorpicker-field');
}

/** 판에 상자를 꽂는다. 꽂지 않으면 jsdom 기본값(전부 0)이 그대로 온다. */
function withRect(): void {
  vi.spyOn(field(), 'getBoundingClientRect').mockReturnValue(RECT);
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('2D 판 — 접근성 계약', () => {
  it('`aria-hidden` 이고 `tabindex` 속성을 갖지 않는다', () => {
    render(<SaturationField hsv={BLUE_HSV} onPick={vi.fn()} />);

    expect(field()).toHaveAttribute('aria-hidden', 'true');
    // 속성이 **없어야** 한다. `tabIndex = -1` 로 두면 클릭 시 포커스를 먹어, 팝오버의
    // 탭 순서를 재는 시험이 판을 지나치게 된다.
    expect(field().hasAttribute('tabindex')).toBe(false);
  });
});

describe('2D 판 — 좌표가 채도·명도가 된다', () => {
  it('네 귀퉁이가 각각 (0,1) (1,1) (0,0) (1,0) 이다', () => {
    const onPick = vi.fn();
    render(<SaturationField hsv={BLUE_HSV} onPick={onPick} />);
    withRect();

    fireEvent.mouseDown(field(), { clientX: 100, clientY: 50 }); // 좌상
    fireEvent.mouseDown(field(), { clientX: 300, clientY: 50 }); // 우상
    fireEvent.mouseDown(field(), { clientX: 100, clientY: 150 }); // 좌하
    fireEvent.mouseDown(field(), { clientX: 300, clientY: 150 }); // 우하

    expect(onPick.mock.calls.map(([c]) => c)).toEqual([
      { s: 0, v: 1 },
      { s: 1, v: 1 },
      { s: 0, v: 0 },
      { s: 1, v: 0 },
    ]);
  });

  it('가운데는 (0.5, 0.5) 다', () => {
    const onPick = vi.fn();
    render(<SaturationField hsv={BLUE_HSV} onPick={onPick} />);
    withRect();

    fireEvent.mouseDown(field(), { clientX: 200, clientY: 100 });

    expect(onPick).toHaveBeenCalledWith({ s: 0.5, v: 0.5 });
  });

  it('판 밖으로 끌고 나가도 0–1 안에 가둔다', () => {
    const onPick = vi.fn();
    render(<SaturationField hsv={BLUE_HSV} onPick={onPick} />);
    withRect();

    fireEvent.mouseDown(field(), { clientX: -9999, clientY: -9999 });
    fireEvent.mouseDown(field(), { clientX: 9999, clientY: 9999 });

    expect(onPick.mock.calls.map(([c]) => c)).toEqual([
      { s: 0, v: 1 },
      { s: 1, v: 0 },
    ]);
  });
});

describe('2D 판 — 끌기', () => {
  it('누른 채 움직이면 따라오고, 떼면 멈춘다', () => {
    const onPick = vi.fn();
    render(<SaturationField hsv={BLUE_HSV} onPick={onPick} />);
    withRect();

    fireEvent.mouseDown(field(), { clientX: 100, clientY: 50 });
    fireEvent.mouseMove(window, { clientX: 200, clientY: 100 });
    fireEvent.mouseUp(window);
    fireEvent.mouseMove(window, { clientX: 300, clientY: 150 });

    expect(onPick.mock.calls.map(([c]) => c)).toEqual([
      { s: 0, v: 1 },
      { s: 0.5, v: 0.5 },
    ]);
  });

  it('누르지 않고 지나간 마우스는 색을 바꾸지 않는다', () => {
    const onPick = vi.fn();
    render(<SaturationField hsv={BLUE_HSV} onPick={onPick} />);
    withRect();

    fireEvent.mouseMove(window, { clientX: 200, clientY: 100 });

    expect(onPick).not.toHaveBeenCalled();
  });

  it('언마운트하면 창에 단 리스너가 사라진다', () => {
    const onPick = vi.fn();
    const { unmount } = render(<SaturationField hsv={BLUE_HSV} onPick={onPick} />);
    withRect();
    fireEvent.mouseDown(field(), { clientX: 100, clientY: 50 });
    onPick.mockClear();

    unmount();
    fireEvent.mouseMove(window, { clientX: 200, clientY: 100 });

    expect(onPick).not.toHaveBeenCalled();
  });
});

describe('2D 판 — 재지 못한 상자', () => {
  it('폭·높이가 0 이면 아무 값도 내지 않는다 — NaN 을 흘리지 않는다', () => {
    const onPick = vi.fn();
    // 상자를 꽂지 않는다. jsdom 기본값은 전부 0 이고, 그 상태로 나누면 s·v 가 NaN 이
    // 되어 `#NaNNaNNaN` 같은 무효 문자열이 예외 없이 저장된다.
    render(<SaturationField hsv={BLUE_HSV} onPick={onPick} />);

    fireEvent.mouseDown(field(), { clientX: 200, clientY: 100 });
    fireEvent.mouseMove(window, { clientX: 210, clientY: 110 });

    expect(onPick).not.toHaveBeenCalled();
  });
});

describe('2D 판 — 보이는 것', () => {
  it('배경이 현재 색상의 순색이다', () => {
    render(<SaturationField hsv={{ h: 0, s: 0.1, v: 0.1, a: 0.3 }} onPick={vi.fn()} />);
    // 채도·명도·알파와 무관하게 색상(hue)만 배경을 정한다 — 판은 나머지 두 축을
    // 겹친 그라디언트로 표현한다.
    expect(field().style.backgroundColor).toBe('rgb(255, 0, 0)');
  });

  it('손잡이가 채도·명도 자리에 놓인다', () => {
    render(<SaturationField hsv={{ h: 0, s: 0.25, v: 0.75, a: 1 }} onPick={vi.fn()} />);

    const handle = screen.getByTestId('colorpicker-field-handle');
    expect(handle.style.left).toBe('25%');
    // 명도는 위가 1 이므로 뒤집힌다.
    expect(handle.style.top).toBe('25%');
  });
});
