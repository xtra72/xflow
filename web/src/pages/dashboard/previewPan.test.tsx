// 미리보기 이동(팬) 시험 — 순수 기하 · 훅 · 그리고 **캔버스 편집과의 이음매**.
//
// 이 파일이 못박는 것 다섯:
//   1. **넘칠 때만 움직인다.** 들어맞는 그림에는 잡을 커서도, 탭 정거장도, 방향키 반응도
//      없다. 이 절의 고정 입력은 **넘치는 것**이어야 한다 — 들어맞는 픽스처로는 "움직이지
//      않는다" 의 절반만 재고, 나머지 절반("넘치면 움직인다")은 실패할 수조차 없다.
//   2. **가장자리에서 멈춘다.** 그림이 영역 안으로 물러나 빈 자리를 남기지 않으며,
//      **줌을 낮추면 그 자리에서 다시 죈다**(밖에 남겨 두지 않는다).
//   3. **아무도 가져가지 않은 몸짓만 받는다.** 캔버스에서 도형을 끌면 팬은 가만히 있고,
//      빈 자리를 끌면 팬이 받는다. 방향키도 같다 — 고른 것이 있으면 도형이, 없으면 화면이
//      움직인다. 이 절은 **층을 건너** 세운다: 팬 표면과 진짜 오버레이를 함께 세우지 않으면
//      두 층이 각자 100% 여도 이음매는 덮이지 않는다.
//   4. **캔버스 아닌 패널도 움직인다.** 아무도 몸짓을 가져가지 않으므로 표면 전체가 팬이다.
//   5. **프레임도 기하도 건드리지 않는다.** 팬이 바꾸는 것은 `transform` 문자열 하나다
//      (SPEC-CANVAS-001 REQ-05 유휴 정지 · SPEC-CANVAS-002 REQ-06 기하 쓰기 단일 통로).
//
// jsdom 에는 `PointerEvent` 도 `setPointerCapture` 도 없고 레이아웃도 하지 않는다. 그래서
// 포인터는 `MouseEvent` 로 흉내 내고(이 저장소의 관용구), 크기는 **인자로 넘긴다** —
// 훅이 DOM 을 재지 않는 것이 그 흉내를 정직하게 만든다.

import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import type { CanvasElement } from './panels/canvas/canvasConfig';
import CanvasEditOverlay from './panels/canvas/CanvasEditOverlay';
import {
  CanvasEditSelectionContext,
  useCanvasEditSelectionState,
} from './panels/canvas/canvasEditContext';
import type { CanvasProjection } from './panels/canvas/canvasGeometry';
import {
  PREVIEW_PAN_ARROW_COARSE_PX,
  PREVIEW_PAN_ARROW_PX,
  arrowPanDelta,
  clampPanOffset,
  panBounds,
  panTransform,
  usePreviewPan,
  type PanSize,
} from './previewPan';

afterEach(cleanup);

// --- 고정 입력 -----------------------------------------------------------

/** 담는 영역. 아래 그림보다 **작다** — 그래야 옮길 곳이 있다. */
const AREA: PanSize = { w: 400, h: 200 };
/** 넘치는 그림. 가로로 400, 세로로 200 넘치므로 상한은 (200, 100) 이다. */
const OVERFLOW: PanSize = { w: 800, h: 400 };
/** 들어맞는 그림 — 이때는 팬이 통째로 잠들어 있어야 한다. */
const FITS: PanSize = { w: 300, h: 150 };

/** 캔버스 이음매용 투영. 스테이지 800×400 · 캔버스 500×400. */
const PROJ: CanvasProjection = {
  stage: { width: 800, height: 400 },
  canvas: { width: 500, height: 400 },
};

/** 스테이지 한가운데를 차지하는 사각형 — 정규화 중심이 (0.5, 0.5) 다. */
const CENTER_RECT: CanvasElement = {
  id: 'a',
  kind: 'rect',
  geometry: { x: 200, y: 160, w: 100, h: 80 },
  style: {},
};

// --- 도구 ---------------------------------------------------------------

function pointer(type: string, x: number, y: number, init: MouseEventInit = {}): MouseEvent {
  return new MouseEvent(type, { clientX: x, clientY: y, bubbles: true, cancelable: true, ...init });
}

function sendAt(target: Element, type: string, x: number, y: number, init: MouseEventInit = {}) {
  const evt = pointer(type, x, y, init);
  fireEvent(target, evt);
  return evt;
}

function surface(): HTMLElement {
  return screen.getByTestId('preview-pan-surface');
}

function stage(): HTMLElement {
  return screen.getByTestId('pan-stage');
}

/** 지금 상자에 얹힌 이동량. `translate` 가 없으면 (0, 0) 이다. */
function stageOffset(): { x: number; y: number } {
  const m = /translate\((-?[\d.]+)px, (-?[\d.]+)px\)/.exec(stage().style.transform);
  return m === null ? { x: 0, y: 0 } : { x: Number(m[1]), y: Number(m[2]) };
}

/** 표면을 (dx, dy) 만큼 끈다. 누름 자리는 뜻이 없다(상대 이동이다). */
function dragSurface(dx: number, dy: number, init: MouseEventInit = {}): void {
  sendAt(surface(), 'pointerdown', 100, 100, init);
  sendAt(surface(), 'pointermove', 100 + dx, 100 + dy);
  sendAt(surface(), 'pointerup', 100 + dx, 100 + dy);
}

/**
 * 팬 표면 한 벌 — **설정 미리보기의 형상 그대로**다. 넘치는 상자가 fit 컨테이너 안에 들고,
 * 조작 규칙은 전부 훅이 지며, 이 하네스는 훅이 준 props 를 펼치기만 한다.
 */
function PanHarness({
  content,
  area = AREA,
  children,
}: {
  content: PanSize | null;
  area?: PanSize;
  children?: React.ReactNode;
}) {
  const pan = usePreviewPan(content, area, { surfaceAria: 'pan-aria' });
  return (
    <div {...pan.surfaceProps} className={pan.cursorClass}>
      <div data-testid="pan-stage" style={{ transform: panTransform(pan.offset, 1, 1) }}>
        {children}
      </div>
    </div>
  );
}

/** 캔버스 편집 오버레이를 그 상자 안에 넣은 하네스 — 이음매를 재는 자리다. */
function CanvasPanHarness({ elements }: { elements: readonly CanvasElement[] }) {
  const [live, setLive] = useState<readonly CanvasElement[]>(elements);
  const selection = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={selection}>
      <span data-testid="selection">{[...selection.selection].join(',')}</span>
      <span data-testid="dump">{JSON.stringify(live)}</span>
      <PanHarness content={OVERFLOW}>
        <CanvasEditOverlay
          enabled
          elements={live}
          projection={PROJ}
          textWidths={{}}
          onElementsChange={setLive}
        />
      </PanHarness>
    </CanvasEditSelectionContext>
  );
}

function overlay(): HTMLElement {
  return screen.getByTestId('canvas-edit-overlay');
}

/**
 * 오버레이의 화면 상자를 심는다. 스테이지와 **같은 크기**라 축척 1 이다 — 이 파일이 재는
 * 것은 좌표 환산이 아니라 몸짓의 소유권이므로, 축척을 1 로 두어 좌표를 그대로 읽는다.
 * (비단위 축척의 회귀 게이트는 `CanvasEditOverlay.test.tsx` §축소된 미리보기가 지킨다.)
 */
function stubOverlayRect(): void {
  vi.spyOn(overlay(), 'getBoundingClientRect').mockReturnValue({
    left: 0,
    top: 0,
    width: PROJ.stage.width,
    height: PROJ.stage.height,
    right: PROJ.stage.width,
    bottom: PROJ.stage.height,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  } as DOMRect);
}

function liveElements(): CanvasElement[] {
  return JSON.parse(screen.getByTestId('dump').textContent ?? '[]') as CanvasElement[];
}

async function nextFrame(): Promise<void> {
  await act(async () => {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  });
}

// --- 순수 기하 -----------------------------------------------------------

describe('이동 상한은 넘치는 절반이다', () => {
  it('넘치는 만큼의 절반이 양쪽 상한이다 — 그만큼 옮기면 모서리가 정확히 맞닿는다', () => {
    expect(panBounds(OVERFLOW, AREA)).toEqual({ x: 200, y: 100 });
  });

  it('들어맞으면 0 이다 — 옮길 곳이 없다', () => {
    expect(panBounds(FITS, AREA)).toEqual({ x: 0, y: 0 });
  });

  it('한 축만 넘치면 그 축만 열린다', () => {
    expect(panBounds({ w: 800, h: 150 }, AREA)).toEqual({ x: 200, y: 0 });
  });

  it('아직 재지 못한 상자에는 상한이 없다', () => {
    expect(panBounds(null, AREA)).toEqual({ x: 0, y: 0 });
  });

  it('잴 수 없는 수(NaN · Infinity)에서도 0 이다 — 상한이 NaN 이면 그림이 사라진다', () => {
    expect(panBounds({ w: Number.NaN, h: Number.POSITIVE_INFINITY }, AREA)).toEqual({ x: 0, y: 0 });
  });
});

describe('이동량은 상한 안으로 죈다', () => {
  it('상한을 넘는 값은 상한에서 멈춘다 (양쪽)', () => {
    const bounds = { x: 200, y: 100 };
    expect(clampPanOffset({ x: 999, y: 999 }, bounds)).toEqual({ x: 200, y: 100 });
    expect(clampPanOffset({ x: -999, y: -999 }, bounds)).toEqual({ x: -200, y: -100 });
  });

  it('상한이 0 인 축은 언제나 0 이다', () => {
    expect(clampPanOffset({ x: 50, y: 50 }, { x: 0, y: 100 })).toEqual({ x: 0, y: 50 });
  });

  it('잴 수 없는 이동량은 0 으로 떨어진다', () => {
    expect(clampPanOffset({ x: Number.NaN, y: 10 }, { x: 200, y: 100 })).toEqual({ x: 0, y: 10 });
  });
});

describe('방향키 한 걸음', () => {
  it('네 방향이 각각 한 축만 움직인다 — 축을 뒤바꾸지 않는다', () => {
    expect(arrowPanDelta('ArrowRight', false)).toEqual({ x: PREVIEW_PAN_ARROW_PX, y: 0 });
    expect(arrowPanDelta('ArrowLeft', false)).toEqual({ x: -PREVIEW_PAN_ARROW_PX, y: 0 });
    expect(arrowPanDelta('ArrowDown', false)).toEqual({ x: 0, y: PREVIEW_PAN_ARROW_PX });
    expect(arrowPanDelta('ArrowUp', false)).toEqual({ x: 0, y: -PREVIEW_PAN_ARROW_PX });
  });

  it('Shift 는 다섯 배 걸음이다', () => {
    expect(arrowPanDelta('ArrowRight', true)).toEqual({ x: PREVIEW_PAN_ARROW_COARSE_PX, y: 0 });
    expect(PREVIEW_PAN_ARROW_COARSE_PX).toBe(PREVIEW_PAN_ARROW_PX * 5);
  });

  it('방향키가 아니면 null 이다 — Tab·Esc 가 살아 있어야 한다', () => {
    expect(arrowPanDelta('Tab', false)).toBeNull();
    expect(arrowPanDelta('a', false)).toBeNull();
  });
});

describe('이동량이 0 이면 변환 문자열이 종전과 한 글자도 다르지 않다', () => {
  it('0 에는 translate 가 붙지 않는다', () => {
    expect(panTransform({ x: 0, y: 0 }, 0.5, 0.25)).toBe('scale(0.5, 0.25)');
  });

  it('0 이 아니면 배율 **바깥**에 붙는다 — 이동량이 화면 px 그대로여야 한다', () => {
    expect(panTransform({ x: 12, y: -3 }, 0.5, 0.25)).toBe(
      'translate(12px, -3px) scale(0.5, 0.25)',
    );
  });
});

// --- 넘칠 때만 움직인다 --------------------------------------------------

describe('들어맞는 그림은 움직이지 않는다', () => {
  it('잡을 커서도 탭 정거장도 없다', () => {
    render(<PanHarness content={FITS} />);

    expect(surface().dataset.canPan).toBe('false');
    expect(surface().className).not.toContain('cursor-grab');
    expect(surface().getAttribute('tabindex')).toBeNull();
    expect(surface().getAttribute('aria-keyshortcuts')).toBeNull();
  });

  it('끌어도 그대로다', () => {
    render(<PanHarness content={FITS} />);
    dragSurface(80, 40);

    expect(stageOffset()).toEqual({ x: 0, y: 0 });
    expect(stage().style.transform).toBe('scale(1, 1)');
  });

  it('방향키도 소비하지 않는다 — 눌러도 아무 일이 없으면서 스크롤을 막으면 그것이 결함이다', () => {
    render(<PanHarness content={FITS} />);

    const consumed = !fireEvent.keyDown(surface(), { key: 'ArrowRight' });
    expect(consumed).toBe(false);
    expect(stageOffset()).toEqual({ x: 0, y: 0 });
  });

  it('아직 재지 못한 상자도 같다', () => {
    render(<PanHarness content={null} />);
    dragSurface(80, 40);

    expect(surface().dataset.canPan).toBe('false');
    expect(stageOffset()).toEqual({ x: 0, y: 0 });
  });
});

describe('넘치는 그림은 끌어 옮긴다', () => {
  it('잡을 커서와 탭 정거장이 생기고 이름이 붙는다', () => {
    render(<PanHarness content={OVERFLOW} />);

    expect(surface().dataset.canPan).toBe('true');
    expect(surface().className).toContain('cursor-grab');
    expect(surface().getAttribute('tabindex')).toBe('0');
    expect(surface().getAttribute('role')).toBe('group');
    expect(surface().getAttribute('aria-label')).toBe('pan-aria');
    expect(surface().getAttribute('aria-keyshortcuts')).toContain('ArrowLeft');
  });

  it('끈 만큼 화면 px 그대로 움직인다', () => {
    render(<PanHarness content={OVERFLOW} />);
    dragSurface(60, -30);

    expect(stageOffset()).toEqual({ x: 60, y: -30 });
  });

  it('끄는 동안 커서가 쥔 손이 되고, 놓으면 되돌아온다', () => {
    render(<PanHarness content={OVERFLOW} />);

    sendAt(surface(), 'pointerdown', 100, 100);
    expect(surface().className).toContain('cursor-grabbing');
    expect(surface().dataset.panning).toBe('true');

    sendAt(surface(), 'pointerup', 100, 100);
    expect(surface().className).toContain('cursor-grab');
    expect(surface().className).not.toContain('cursor-grabbing');
  });

  it('취소되어도 마지막 자리를 지키고 끌기는 끝난다', () => {
    render(<PanHarness content={OVERFLOW} />);

    sendAt(surface(), 'pointerdown', 100, 100);
    sendAt(surface(), 'pointermove', 130, 100);
    sendAt(surface(), 'pointercancel', 130, 100);
    expect(stageOffset()).toEqual({ x: 30, y: 0 });

    // 끝났으므로 이어지는 이동은 닿지 않는다.
    sendAt(surface(), 'pointermove', 300, 100);
    expect(stageOffset()).toEqual({ x: 30, y: 0 });
  });

  it('한 축만 넘치면 그 축만 간다 — 들어맞는 축은 0 에 붙박인다', () => {
    render(<PanHarness content={{ w: 800, h: 150 }} />);
    dragSurface(60, 60);

    expect(stageOffset()).toEqual({ x: 60, y: 0 });
  });
});

describe('가장자리에서 멈춘다 — 그림이 영역 안으로 물러나지 않는다', () => {
  it('상한(넘치는 절반)을 넘겨 끌어도 거기서 선다', () => {
    render(<PanHarness content={OVERFLOW} />);
    dragSurface(9999, 9999);

    expect(stageOffset()).toEqual({ x: 200, y: 100 });
  });

  it('반대쪽도 같다', () => {
    render(<PanHarness content={OVERFLOW} />);
    dragSurface(-9999, -9999);

    expect(stageOffset()).toEqual({ x: -200, y: -100 });
  });

  it('끝에 닿은 방향키는 소비하지 않는다', () => {
    render(<PanHarness content={OVERFLOW} />);
    dragSurface(9999, 0);
    expect(stageOffset().x).toBe(200);

    const consumed = !fireEvent.keyDown(surface(), { key: 'ArrowRight' });
    expect(consumed).toBe(false);
    expect(stageOffset().x).toBe(200);
  });
});

describe('줌을 낮추면 그 자리에서 다시 죈다 (밖에 남겨 두지 않는다)', () => {
  it('상한이 줄면 이동량도 함께 줄어든다', () => {
    const { rerender } = render(<PanHarness content={OVERFLOW} />);
    dragSurface(9999, 9999);
    expect(stageOffset()).toEqual({ x: 200, y: 100 });

    // 축소: 500×250 → 상한 (50, 25).
    rerender(<PanHarness content={{ w: 500, h: 250 }} />);
    expect(stageOffset()).toEqual({ x: 50, y: 25 });
  });

  it('들어맞는 크기까지 낮추면 0 으로 돌아가고 조작도 잠긴다', () => {
    const { rerender } = render(<PanHarness content={OVERFLOW} />);
    dragSurface(120, 60);

    rerender(<PanHarness content={FITS} />);
    expect(stageOffset()).toEqual({ x: 0, y: 0 });
    expect(surface().dataset.canPan).toBe('false');
  });

  it('다시 확대해도 옛 자리가 되살아나지 않는다 — 죈 값이 상태에도 남는다', () => {
    const { rerender } = render(<PanHarness content={OVERFLOW} />);
    dragSurface(9999, 9999);

    rerender(<PanHarness content={{ w: 500, h: 250 }} />);
    expect(stageOffset()).toEqual({ x: 50, y: 25 });

    rerender(<PanHarness content={OVERFLOW} />);
    expect(stageOffset()).toEqual({ x: 50, y: 25 });
  });

  it('영역이 넓어져도 같은 규칙이다 (경계 드래그로 미리보기를 키운 경우)', () => {
    const { rerender } = render(<PanHarness content={OVERFLOW} />);
    dragSurface(9999, 0);
    expect(stageOffset().x).toBe(200);

    rerender(<PanHarness content={OVERFLOW} area={{ w: 700, h: 200 }} />);
    expect(stageOffset().x).toBe(50);
  });
});

// --- 방향키 ---------------------------------------------------------------

describe('방향키가 화면을 옮긴다 — 끄는 것과 같은 방향이다', () => {
  it('오른쪽 키는 패널을 오른쪽으로 옮긴다(손으로 끄는 방향과 같다)', () => {
    render(<PanHarness content={OVERFLOW} />);

    fireEvent.keyDown(surface(), { key: 'ArrowRight' });
    expect(stageOffset()).toEqual({ x: PREVIEW_PAN_ARROW_PX, y: 0 });
  });

  it('Shift 는 다섯 배로 옮긴다', () => {
    render(<PanHarness content={OVERFLOW} />);

    // 가로 상한은 200 이라 120 걸음이 죄이지 않는다(세로 상한 100 은 한 걸음에 닿는다).
    fireEvent.keyDown(surface(), { key: 'ArrowRight', shiftKey: true });
    expect(stageOffset()).toEqual({ x: PREVIEW_PAN_ARROW_COARSE_PX, y: 0 });
  });

  it('실제로 옮겼을 때에만 소비한다', () => {
    render(<PanHarness content={OVERFLOW} />);

    const consumed = !fireEvent.keyDown(surface(), { key: 'ArrowRight' });
    expect(consumed).toBe(true);
  });

  it('Ctrl·Cmd·Alt 가 붙은 방향키는 브라우저·OS 의 조작이다', () => {
    render(<PanHarness content={OVERFLOW} />);

    for (const mod of ['ctrlKey', 'metaKey', 'altKey']) {
      fireEvent.keyDown(surface(), { key: 'ArrowRight', [mod]: true });
    }
    expect(stageOffset()).toEqual({ x: 0, y: 0 });
  });
});

// --- 이음매: 캔버스 편집과 몸짓을 나눈다 ----------------------------------

describe('캔버스 편집이 가져간 몸짓은 팬이 받지 않는다 (층을 건너 재는 자리)', () => {
  it('도형을 끌면 도형이 움직이고 화면은 그대로다', async () => {
    render(<CanvasPanHarness elements={[CENTER_RECT]} />);
    stubOverlayRect();

    // 스테이지 (400, 200) = 사각형의 시각적 한가운데(축척 1).
    sendAt(overlay(), 'pointerdown', 400, 200);
    sendAt(overlay(), 'pointermove', 440, 220);
    await nextFrame();

    expect(stageOffset()).toEqual({ x: 0, y: 0 });
    expect(liveElements()[0]?.geometry).not.toEqual(CENTER_RECT.geometry);
  });

  it('빈 자리를 끌면 화면이 움직이고 도형은 그대로다', () => {
    render(<CanvasPanHarness elements={[CENTER_RECT]} />);
    stubOverlayRect();

    // 스테이지 (40, 40) 은 사각형(정규화 0.4~0.6) 밖이다.
    sendAt(overlay(), 'pointerdown', 40, 40);
    sendAt(surface(), 'pointermove', 100, 70);
    sendAt(surface(), 'pointerup', 100, 70);

    expect(stageOffset()).toEqual({ x: 60, y: 30 });
    expect(liveElements()[0]?.geometry).toEqual(CENTER_RECT.geometry);
  });

  it('가운데 버튼은 도형 위에서도 화면을 옮긴다 — 규칙의 명시적 우회로다', async () => {
    render(<CanvasPanHarness elements={[CENTER_RECT]} />);
    stubOverlayRect();

    sendAt(overlay(), 'pointerdown', 400, 200, { button: 1 });
    sendAt(surface(), 'pointermove', 450, 230);
    sendAt(surface(), 'pointerup', 450, 230);
    await nextFrame();

    expect(stageOffset()).toEqual({ x: 50, y: 30 });
    // 아래층이 그 누름을 보지 못했으므로 고른 것도 옮긴 것도 없다.
    expect(screen.getByTestId('selection').textContent).toBe('');
    expect(liveElements()[0]?.geometry).toEqual(CENTER_RECT.geometry);
  });
});

describe('방향키도 같은 규칙으로 갈린다', () => {
  it('고른 것이 있으면 도형이 움직이고 화면은 그대로다', () => {
    render(<CanvasPanHarness elements={[CENTER_RECT]} />);
    stubOverlayRect();
    // 몸짓을 끝까지 마친다 — 끌고 있는 동안에는 손이 이기므로(오버레이의 규율) 놓지
    // 않으면 방향키가 도형에 닿지 않는다.
    sendAt(overlay(), 'pointerdown', 400, 200);
    sendAt(overlay(), 'pointerup', 400, 200);
    expect(screen.getByTestId('selection').textContent).toBe('a');

    fireEvent.keyDown(overlay(), { key: 'ArrowRight' });

    expect((liveElements()[0]?.geometry as { x: number }).x).toBe(CENTER_RECT.geometry.x + 1);
    expect(stageOffset()).toEqual({ x: 0, y: 0 });
  });

  it('고른 것이 없으면 화면이 움직이고 도형은 그대로다', () => {
    render(<CanvasPanHarness elements={[CENTER_RECT]} />);
    stubOverlayRect();

    fireEvent.keyDown(overlay(), { key: 'ArrowRight' });

    expect(stageOffset()).toEqual({ x: PREVIEW_PAN_ARROW_PX, y: 0 });
    expect(liveElements()[0]?.geometry).toEqual(CENTER_RECT.geometry);
  });
});

describe('캔버스가 아닌 패널도 옮겨진다 — 아무도 몸짓을 가져가지 않는다', () => {
  it('평범한 패널 내용 위에서 끌어도 화면이 따라온다', () => {
    render(
      <PanHarness content={OVERFLOW}>
        <div data-testid="plain-panel">게이지</div>
      </PanHarness>,
    );

    sendAt(screen.getByTestId('plain-panel'), 'pointerdown', 100, 100);
    sendAt(surface(), 'pointermove', 175, 145);
    sendAt(surface(), 'pointerup', 175, 145);

    expect(stageOffset()).toEqual({ x: 75, y: 45 });
  });

  it('제 몸짓을 가져간 층 위에서는 시작하지 않는다 (preventDefault 만 한 층도 포함)', () => {
    render(
      <PanHarness content={OVERFLOW}>
        {/* `PanelDragLayer` 가 요소를 잡을 때 하는 일 그대로 — 끊지는 않고 기본만 막는다. */}
        <div
          data-testid="claimed"
          onPointerDown={(event) => event.preventDefault()}
        >
          값
        </div>
      </PanHarness>,
    );

    sendAt(screen.getByTestId('claimed'), 'pointerdown', 100, 100);
    sendAt(surface(), 'pointermove', 200, 200);

    expect(stageOffset()).toEqual({ x: 0, y: 0 });
    expect(surface().dataset.panning).toBe('false');
  });

  it('오른쪽 버튼은 상황 메뉴의 것이다 — 팬이 가져가지 않는다', () => {
    render(<PanHarness content={OVERFLOW} />);

    sendAt(surface(), 'pointerdown', 100, 100, { button: 2 });
    sendAt(surface(), 'pointermove', 200, 200);

    expect(stageOffset()).toEqual({ x: 0, y: 0 });
  });
});

// --- 프레임도 기하도 건드리지 않는다 --------------------------------------

describe('팬은 캔버스 프레임을 0 건 요청하고 기하를 한 글자도 쓰지 않는다', () => {
  it('끌기와 방향키가 rAF 를 부르지 않는다 (REQ-05 유휴 정지)', () => {
    render(<PanHarness content={OVERFLOW} />);
    const raf = vi.spyOn(window, 'requestAnimationFrame');

    dragSurface(60, 30);
    fireEvent.keyDown(surface(), { key: 'ArrowLeft' });

    expect(raf).not.toHaveBeenCalled();
    raf.mockRestore();
  });

  it('캔버스 위에서 화면을 옮겨도 요소 배열이 그대로다 (REQ-06 기하 쓰기 단일 통로)', () => {
    render(<CanvasPanHarness elements={[CENTER_RECT]} />);
    stubOverlayRect();
    const before = screen.getByTestId('dump').textContent;

    sendAt(overlay(), 'pointerdown', 40, 40);
    sendAt(surface(), 'pointermove', 140, 90);
    sendAt(surface(), 'pointerup', 140, 90);
    fireEvent.keyDown(surface(), { key: 'ArrowUp' });

    expect(stageOffset()).not.toEqual({ x: 0, y: 0 });
    expect(screen.getByTestId('dump').textContent).toBe(before);
  });
});
