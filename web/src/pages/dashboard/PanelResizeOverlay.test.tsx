// 미리보기 패널 크기 조절 오버레이 검증.
//
// 오버레이가 지켜야 하는 것 둘:
//   1. 프레임이 패널과 **정확히 겹칠 것** — 어긋나면 어디를 잡아 끄는지 알 수 없다.
//   2. 끈 거리를 **실측된 미리보기 상자**를 기준으로 환산할 것 — 미리보기는 축소되어
//      있으므로 화면 px 를 그대로 칸으로 읽으면 한 칸을 끌었는데 여러 칸이 움직인다.
//
// 그리드 단위 환산 자체는 previewGridSize.test.ts 가 잠근다. 여기서는 그 계산에
// **어떤 기준값이 들어가는가** 를 본다.

import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import { unitsToPx } from './gridGeometry';
import { PanelResizeOverlay } from './PanelResizeOverlay';

const CELL = 100;
const SIZE = { w: 4, h: 3 };
/** 4×3 패널을 절반 배율로 보여주는 미리보기 상자의 화면 크기. */
const BOX = { w: unitsToPx(SIZE.w, CELL) * 0.5, h: unitsToPx(SIZE.h, CELL) * 0.5 };
const ASPECT = unitsToPx(SIZE.w, CELL) / unitsToPx(SIZE.h, CELL);

function renderOverlay(
  over: Partial<React.ComponentProps<typeof PanelResizeOverlay>> = {},
): { onChange: ReturnType<typeof vi.fn> } {
  const onChange = vi.fn();
  render(
    <PanelResizeOverlay
      size={SIZE}
      box={BOX}
      aspect={ASPECT}
      cols={12}
      cell={CELL}
      onChange={onChange}
      {...over}
    />,
  );
  return { onChange };
}

describe('프레임 — 패널과 겹친다', () => {
  it('실측된 미리보기 크기를 그대로 쓴다 — 영역에 가두지 않는다', () => {
    renderOverlay();

    const frame = screen.getByTestId('panel-resize-frame');
    expect(frame.style.width).toBe(`${BOX.w}px`);
    expect(frame.style.height).toBe(`${BOX.h}px`);
    // 채움 모드에서 패널이 영역 밖으로 넘치는 것은 의도된 동작이므로 가두면 안 된다.
    expect(frame.className).not.toContain('max-w-full');
  });

  it('실측 전에는 종횡비로 그린다 — 프레임이 뒤늦게 나타나지 않는다', () => {
    renderOverlay({ box: null });

    const frame = screen.getByTestId('panel-resize-frame');
    expect(frame.style.width).toBe('100%');
    expect(frame.style.aspectRatio).toContain(String(ASPECT));
    // 크기를 모르는 동안에는 영역을 넘지 않게 가둔다.
    expect(frame.className).toContain('max-w-full');
  });
});

describe('손잡이 드래그', () => {
  /** 화면에서 대시보드 한 칸에 해당하는 거리(절반 배율이므로 실제의 절반). */
  const oneCellOnScreen = unitsToPx(1, CELL) * 0.5;

  it('실측된 상자를 기준으로 환산한다 — 한 칸을 끌면 한 칸만 커진다', () => {
    const { onChange } = renderOverlay();

    fireEvent.mouseDown(screen.getByTestId('panel-resize-handle'), { clientX: 0, clientY: 0 });
    fireEvent.mouseMove(window, { clientX: oneCellOnScreen, clientY: 0 });

    // 실측 상자를 무시하고 프레임의 렌더 크기(jsdom 에서 0)를 읽으면 크기가 그대로
    // 남는다. 5 가 나온다는 것이 곧 실측값을 기준으로 삼았다는 뜻이다.
    expect(onChange).toHaveBeenLastCalledWith({ w: 5, h: 3 });
  });

  it('가로·세로를 함께 끌면 두 축이 각각 환산된다', () => {
    const { onChange } = renderOverlay();

    fireEvent.mouseDown(screen.getByTestId('panel-resize-handle'), { clientX: 0, clientY: 0 });
    fireEvent.mouseMove(window, { clientX: oneCellOnScreen, clientY: oneCellOnScreen });

    expect(onChange).toHaveBeenLastCalledWith({ w: 5, h: 4 });
  });

  it('손을 떼면 더 이상 따라오지 않는다', () => {
    const { onChange } = renderOverlay();

    fireEvent.mouseDown(screen.getByTestId('panel-resize-handle'), { clientX: 0, clientY: 0 });
    fireEvent.mouseMove(window, { clientX: oneCellOnScreen, clientY: 0 });
    const afterDrag = onChange.mock.calls.length;

    fireEvent.mouseUp(window);
    fireEvent.mouseMove(window, { clientX: oneCellOnScreen * 3, clientY: 0 });

    expect(onChange).toHaveBeenCalledTimes(afterDrag);
  });
});
