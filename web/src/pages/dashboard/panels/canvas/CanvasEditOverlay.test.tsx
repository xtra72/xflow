// 캔버스 편집 오버레이 테스트 — 선택 + 드래그 이동 (SPEC-CANVAS-002 T5, 1차).
//
// 덮는 인수 기준: AC-01(히트·z-order), AC-03(드래그 이동·무리 이동·`pointercancel` 확정),
// AC-07(편집 게이팅), AC-E2(외곽선 px 가 투영 px 와 일치), AC-E3(빈 지점 비가로채기),
// AC-E5(스테이지 밖 clamp 금지), AC-E6(퇴화 도형도 보인다), AC-E7(실측 폭 결측 폴백).
//
// **jsdom 에는 `PointerEvent` 가 없다** — `typeof PointerEvent === 'undefined'` 이고
// `setPointerCapture`/`hasPointerCapture` 도 없다(이 저장소에서 직접 확인했다). 그래서
// `fireEvent.pointerDown(el, { clientX })` 는 **좌표를 조용히 버린다** — 그 형태로 쓴
// 드래그 테스트는 아무것도 주장하지 못한다. 이 파일은 `PanelDragLayer.test.tsx` 가 세운
// 관용구를 그대로 쓴다: `new MouseEvent('pointerdown', { clientX, clientY, ... })` 를 만들어
// `fireEvent(el, evt)` 로 직접 디스패치한다. React 는 타입이 `pointerdown` 이면 합성
// 포인터 이벤트로 감싸고, `clientX`/`clientY`/`shiftKey` 는 MouseEvent 의 필드라 그대로
// 전달된다(`pointerId` 는 `undefined` 이며 down/move 양쪽이 같으므로 짝이 맞는다).
//
// 이동은 프레임당 한 번으로 모이므로 실제와 같은 시점을 보려면 프레임을 기다려야 한다.

import { useState } from 'react';
import { describe, expect, it, vi, afterEach, beforeEach } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react';

// i18n 은 키를 그대로 돌려준다(I18nProvider 없이 렌더 가능 — CanvasPanel.test.tsx 선례).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { BoxGeometry, CanvasElement, CanvasSize } from './canvasConfig';
import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasEditOverlay from './CanvasEditOverlay';
import CanvasSurface, { type FrameScheduler } from './CanvasSurface';
import {
  CanvasEditSelectionContext,
  useCanvasEditSelectionState,
} from './canvasEditContext';
import { appendElement } from './canvasElementFactory';
import type { CanvasProjection, StageSize } from './canvasGeometry';

// --- 고정 입력 -----------------------------------------------------------

/** 기본 스테이지 — 축이 서로 달라야 축을 뒤바꾼 계산이 드러난다. */
const STAGE: StageSize = { width: 200, height: 100 };

/**
 * 기본 캔버스 — 500x400(기본 크기 그대로).
 *
 * 위 스테이지와 짝지으면 축척이 **가로 0.4 · 세로 0.25** 로 갈린다. 1:1 로 두지 않는 것에
 * 뜻이 있다: 축척이 1 이면 캔버스 크기를 무시한 투영도 이 파일의 기대값을 통과한다.
 */
const CANVAS: CanvasSize = { width: 500, height: 400 };

/** 기본 투영 한 벌 — 위 둘을 묶은 것이다. */
const PROJ: CanvasProjection = { stage: STAGE, canvas: CANVAS };

function rect(id: string, geometry: BoxGeometry): CanvasElement {
  return { id, kind: 'rect', geometry, style: {} };
}

// --- 시험 대상 하네스 ----------------------------------------------------

interface HarnessProps {
  enabled?: boolean;
  elements: readonly CanvasElement[];
  stage?: StageSize;
  canvas?: CanvasSize;
  textWidths?: Record<string, number>;
  onElementsChange: (next: CanvasNode[]) => void;
  onParentDown?: () => void;
  /** 미리보기 래퍼의 `onWheel={handlePreviewWheel}`(휠 확대)을 흉내 내는 눈이다(AC-E3). */
  onParentWheel?: () => void;
}

/**
 * 오버레이를 공유 선택 provider 안에 둔다 — 선택을 화면으로 관측하기 위해서다.
 * 부모의 `onPointerDown` 은 **전파 소비 여부**를 재는 눈이다(AC-E3).
 */
function Harness({
  enabled = true,
  elements,
  stage = STAGE,
  canvas = CANVAS,
  textWidths = {},
  onElementsChange,
  onParentDown,
  onParentWheel,
}: HarnessProps) {
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <div
        data-testid="parent"
        onPointerDown={onParentDown}
        onWheel={onParentWheel}
        className="relative"
      >
        <span data-testid="selection">{[...state.selection].join(',')}</span>
        {/* 도구는 스테이지 위가 아니라 **도크 자리**에만 그려진다 — 그 자리를 내지 않으면
            (대시보드에 놓인 패널이 그렇다) 팔레트·격자·정렬·순서가 아예 없다. 하네스는
            설정 미리보기와 같은 형상을 흉내 내므로 자리를 낸다. */}
        <CanvasEditDockRegion enabled>
          <CanvasEditOverlay
            enabled={enabled}
            elements={elements}
            projection={{ stage, canvas }}
            textWidths={textWidths}
            onElementsChange={onElementsChange}
          />
        </CanvasEditDockRegion>
      </div>
    </CanvasEditSelectionContext>
  );
}

// --- 포인터 관용구 --------------------------------------------------------

/**
 * 좌표를 실제로 실어 나르는 포인터 이벤트. jsdom 에 `PointerEvent` 가 없으므로
 * `MouseEvent` 로 만들어 타입만 포인터로 둔다(위 머리말).
 */
function pointer(type: string, x: number, y: number, init: MouseEventInit = {}): Event {
  return new MouseEvent(type, {
    clientX: x,
    clientY: y,
    bubbles: true,
    cancelable: true,
    ...init,
  });
}

/**
 * 오버레이 루트에 이벤트를 보낸다. 보낸 이벤트를 돌려주어 소비 여부를 잴 수 있다.
 *
 * `pointerId` 는 MouseEvent 에 없는 필드라 필요할 때만 심는다 — 심지 않으면 down/move
 * 양쪽이 `undefined` 라 짝이 맞고, 심으면 "다른 포인터의 이벤트" 를 흉내 낼 수 있다.
 */
function send(
  type: string,
  x: number,
  y: number,
  init: MouseEventInit = {},
  pointerId?: number,
): Event {
  const evt = pointer(type, x, y, init);
  if (pointerId !== undefined) Object.defineProperty(evt, 'pointerId', { value: pointerId });
  fireEvent(screen.getByTestId('canvas-edit-overlay'), evt);
  return evt;
}

/** 포인터 캡처 API 를 심는다 — jsdom 에는 없다(직접 확인했다). */
function stubPointerCapture(captured = true) {
  const overlay = screen.getByTestId('canvas-edit-overlay');
  const api = {
    setPointerCapture: vi.fn(),
    hasPointerCapture: vi.fn(() => captured),
    releasePointerCapture: vi.fn(),
  };
  Object.assign(overlay, api);
  return api;
}

/** jsdom 은 레이아웃을 하지 않으므로 오버레이의 화면 자리를 직접 심는다. */
function stubOverlayRect(left = 0, top = 0, width = 200, height = 100): void {
  vi.spyOn(screen.getByTestId('canvas-edit-overlay'), 'getBoundingClientRect').mockReturnValue({
    left,
    top,
    width,
    height,
    right: left + width,
    bottom: top + height,
    x: left,
    y: top,
    toJSON: () => ({}),
  } as DOMRect);
}

/** 이동은 프레임당 한 번으로 모인다 — 프레임을 기다려야 실제와 같은 시점이다. */
async function nextFrame(): Promise<void> {
  await act(async () => {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  });
}

/** 마지막으로 통보된 요소 배열에서 한 요소의 기하를 꺼낸다. */
function emittedGeometry(spy: ReturnType<typeof vi.fn>, id: string): unknown {
  const last = spy.mock.calls.at(-1)?.[0] as CanvasElement[] | undefined;
  return last?.find((el) => el.id === id)?.geometry;
}

/** 선택 표시(하네스의 관측 창). */
function selectionText(): string {
  return screen.getByTestId('selection').textContent ?? '';
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// --- 편집 게이팅 (AC-07) --------------------------------------------------

describe('편집 게이팅 — 꺼져 있으면 표시 전용이다 (AC-07)', () => {
  it('편집이 꺼져 있으면 오버레이가 DOM 에 없다', () => {
    render(<Harness enabled={false} elements={[rect('a', { x: 0, y: 0, w: 500, h: 400 })]} onElementsChange={vi.fn()} />);
    expect(screen.queryByTestId('canvas-edit-overlay')).toBeNull();
  });

  it('편집을 끄면 골라져 있던 것이 풀린다', () => {
    const elements = [rect('a', { x: 0, y: 0, w: 500, h: 400 })];
    const emit = vi.fn();
    const view = render(<Harness elements={elements} onElementsChange={emit} />);
    stubOverlayRect();
    send('pointerdown', 100, 50);
    expect(selectionText()).toBe('a');

    view.rerender(<Harness enabled={false} elements={elements} onElementsChange={emit} />);
    expect(selectionText()).toBe('');
  });
});

// --- 히트와 선택 (AC-01) --------------------------------------------------

describe('누르면 고른다 — 배열 뒤가 위다 (AC-01)', () => {
  it('겹친 자리에서는 배열 뒤(위)에 있는 요소가 이긴다', () => {
    render(
      <Harness
        elements={[rect('a', { x: 0, y: 0, w: 500, h: 400 }), rect('b', { x: 0, y: 0, w: 500, h: 400 })]}
        onElementsChange={vi.fn()}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 100, 50);

    expect(selectionText()).toBe('b');
    expect(screen.getByTestId('canvas-selection-b')).toBeTruthy();
    expect(screen.queryByTestId('canvas-selection-a')).toBeNull();
  });

  it('컨테이너 원점이 0 이 아니어도 스테이지 로컬 좌표로 옮겨 판정한다', () => {
    render(<Harness elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]} onElementsChange={vi.fn()} />);
    // 오버레이가 화면 (10, 20) 에 있다 — 그만큼 빼야 스테이지 로컬 px 이다.
    stubOverlayRect(10, 20);
    // 스테이지 로컬 (30, 15) = 요소 상자(20,10,40,20) 안.
    send('pointerdown', 40, 35);
    expect(selectionText()).toBe('a');
  });

  it('보조키를 누르면 선택에 더한다', () => {
    render(
      <Harness
        elements={[
          rect('a', { x: 50, y: 40, w: 100, h: 80 }),
          rect('b', { x: 250, y: 200, w: 100, h: 80 }),
        ]}
        onElementsChange={vi.fn()}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 30, 15);
    send('pointerdown', 110, 55, { shiftKey: true });

    expect(selectionText()).toBe('a,b');
    expect(screen.getByTestId('canvas-selection-a')).toBeTruthy();
    expect(screen.getByTestId('canvas-selection-b')).toBeTruthy();
  });

  it('보조키로 이미 고른 것을 다시 누르면 뺀다', () => {
    render(
      <Harness
        elements={[
          rect('a', { x: 50, y: 40, w: 100, h: 80 }),
          rect('b', { x: 250, y: 200, w: 100, h: 80 }),
        ]}
        onElementsChange={vi.fn()}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 30, 15);
    send('pointerdown', 110, 55, { shiftKey: true });
    send('pointerdown', 30, 15, { shiftKey: true });

    expect(selectionText()).toBe('b');
  });

  it('보이지 않는 요소는 잡히지 않는다', () => {
    const hidden: CanvasElement = {
      id: 'h',
      kind: 'rect',
      geometry: { x: 0, y: 0, w: 500, h: 400 },
      style: { visible: false },
    };
    render(<Harness elements={[hidden]} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    send('pointerdown', 100, 50);
    expect(selectionText()).toBe('');
  });
});

// --- 빈 지점 비가로채기 (AC-E3) -------------------------------------------

describe('빈 지점 누름은 상위 조작으로 흘려보낸다 (AC-E3)', () => {
  // **AC-E3 이 지키는 문장이 SPEC-CANVAS-010 에서 좁아졌다.** 종전에는 "빈 지점 누름은
  // 소비하지 않는다" 였고, 지금은 **"주 버튼이 아닌 빈 지점 누름은 소비하지 않는다"** 다.
  // 주 버튼은 사각형이 가져갔다(`CanvasEditOverlay.marquee.test.tsx` §몸짓의 소유권).
  // **선택을 비운다는 절반은 두 갈래 모두에서 그대로 살아 있으며**, 아래 두 시험이 그 절반을
  // 각각 따로 못박는다 — 소비 여부만 갈리고 선택의 뜻은 갈리지 않는다.
  //
  // **011 이 그 절반의 *시점*을 옮겼다**(빈 자리 몸짓 뒤집기). 주 버튼 맨손 누름은 이제
  // 팬을 시작하므로 누르는 순간에 비울 수 없다 — 비우면 화면을 옮기는 동안 고른 것이
  // 사라진다. 비우는 일은 **뗌**으로 옮겨 갔고(움직이지 않은 팬이 곧 클릭이다), 그래서
  // 아래 시험은 누름 뒤에 뗌을 함께 쏜다. 주 버튼이 아닌 갈래는 팬을 시작하지 않으므로
  // 종전대로 **누르는 순간** 비운다.
  it('맞는 것이 없으면 선택을 비우고, **주 버튼이면** 팬을 위해 소비한다', () => {
    const parent = vi.fn();
    render(
      <Harness
        elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]}
        onElementsChange={vi.fn()}
        onParentDown={parent}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 30, 15);
    expect(selectionText()).toBe('a');
    parent.mockClear();

    // 요소에서 멀리 떨어진 빈 자리(집기 여유 6px 밖).
    const evt = send('pointerdown', 180, 90);
    // **누르는 순간에는 아직 그대로다**(011 이 뒤집은 자리). 이 한 줄이 시점을 못박는다 —
    // 없으면 "뗌에서 비운다" 와 "누름에서 비운다" 가 같은 초록을 낸다.
    expect(selectionText()).toBe('a');

    // 움직이지 않고 떼면 그 몸짓은 클릭이다 — 여기서 비워진다.
    send('pointerup', 180, 90);

    // 종전 그대로인 절반(시점만 옮겨 왔다).
    expect(selectionText()).toBe('');
    // 010 이 바꾼 절반 — 이 누름은 우리 것이다(011 에서는 사각형이 아니라 팬의 시작이다).
    expect(evt.defaultPrevented).toBe(true);
    expect(parent).not.toHaveBeenCalled();
  });

  it('주 버튼이 아닌 빈 지점 누름은 **여전히** 흘러간다 — 상황 메뉴가 살아 있어야 한다', () => {
    // 위 시험이 옮겨 온 절반이다. 이 갈래가 남아 있지 않으면 "빈 지점을 소비하지 않는다"
    // 는 문장이 통째로 사라지고, 캔버스 위에서만 오른쪽 버튼이 죽는 화면이 된다.
    const parent = vi.fn();
    render(
      <Harness
        elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]}
        onElementsChange={vi.fn()}
        onParentDown={parent}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 30, 15);
    expect(selectionText()).toBe('a');
    parent.mockClear();

    const evt = send('pointerdown', 180, 90, { button: 2 });

    expect(selectionText()).toBe('');
    expect(evt.defaultPrevented).toBe(false);
    expect(parent).toHaveBeenCalledTimes(1);
  });

  it('요소 위 누름은 그 이벤트만 소비해 상위가 함께 반응하지 않는다', () => {
    const parent = vi.fn();
    render(
      <Harness
        elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]}
        onElementsChange={vi.fn()}
        onParentDown={parent}
      />,
    );
    stubOverlayRect();
    const evt = send('pointerdown', 30, 15);

    expect(evt.defaultPrevented).toBe(true);
    expect(parent).not.toHaveBeenCalled();
  });
});

// --- 드래그 이동 (AC-03) --------------------------------------------------

describe('끌어 옮긴다 (AC-03)', () => {
  it('이동량을 정규화 델타로 바꿔 기하에 더한다', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 30, 15);
    send('pointermove', 50, 25);
    await nextFrame();

    // 폭 200 에서 20px = 0.1, 높이 100 에서 10px = 0.1. 크기는 그대로다.
    expect(emittedGeometry(emit, 'a')).toEqual({ x: 100, y: 80, w: 100, h: 80 });
  });

  it('한 프레임 사이의 여러 이동은 한 번만 쓰인다', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 30, 15);
    send('pointermove', 35, 17);
    send('pointermove', 45, 21);
    send('pointermove', 50, 25);
    await nextFrame();

    expect(emit).toHaveBeenCalledTimes(1);
    // 합류해도 결과는 **마지막 자리**다(누적이 아니라 시작점 대비 절대량이다).
    expect(emittedGeometry(emit, 'a')).toEqual({ x: 100, y: 80, w: 100, h: 80 });
  });

  it('요소가 둘 이상 골라져 있으면 같은 델타가 전부에 적용된다', async () => {
    const emit = vi.fn();
    render(
      <Harness
        elements={[
          rect('a', { x: 50, y: 40, w: 100, h: 80 }),
          rect('b', { x: 250, y: 200, w: 100, h: 80 }),
        ]}
        onElementsChange={emit}
      />,
    );
    stubOverlayRect();

    send('pointerdown', 30, 15);
    send('pointerdown', 110, 55, { shiftKey: true });
    // 이미 골라진 b 를 그냥 누르면 선택이 유지된 채 무리가 함께 움직인다.
    send('pointerdown', 110, 55);
    send('pointermove', 130, 65);
    await nextFrame();

    expect(emittedGeometry(emit, 'a')).toEqual({ x: 100, y: 80, w: 100, h: 80 });
    expect(emittedGeometry(emit, 'b')).toEqual({ x: 300, y: 240, w: 100, h: 80 });
  });

  it('보조키를 누른 채로는 끌리지 않는다 (고르기 전용 조작이다)', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 30, 15, { shiftKey: true });
    send('pointermove', 50, 25);
    await nextFrame();

    expect(emit).not.toHaveBeenCalled();
  });

  it('빈 지점에서 시작한 움직임은 아무것도 옮기지 않는다', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 180, 90);
    send('pointermove', 190, 95);
    await nextFrame();

    expect(emit).not.toHaveBeenCalled();
  });

  it('스테이지 크기가 0 이면 드래그를 시작하지 않는다 (0 으로 나누지 않는다)', async () => {
    const emit = vi.fn();
    render(
      <Harness
        elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]}
        stage={{ width: 0, height: 0 }}
        onElementsChange={emit}
      />,
    );
    stubOverlayRect(0, 0, 0, 0);
    // 크기 0 에서는 모든 요소가 한 점으로 투영되므로 집기 여유 안이면 골라진다.
    send('pointerdown', 0, 0);
    send('pointermove', 20, 10);
    await nextFrame();

    expect(selectionText()).toBe('a');
    expect(emit).not.toHaveBeenCalled();
  });
});

// --- clamp 금지 (AC-E5) ---------------------------------------------------

describe('스테이지 밖으로 나가도 잘라내지 않는다 (AC-E5)', () => {
  it('정규화 좌표가 1 을 넘어도 그대로 저장된다', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 400, y: 40, w: 100, h: 80 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 170, 15);
    send('pointermove', 370, 15);
    await nextFrame();

    expect(emittedGeometry(emit, 'a')).toEqual({ x: 900, y: 40, w: 100, h: 80 });
  });

  it('음수 쪽으로 나가도 0 으로 붙잡히지 않는다', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 30, 15);
    send('pointermove', -70, -35);
    await nextFrame();

    expect(emittedGeometry(emit, 'a')).toEqual({ x: -200, y: -160, w: 100, h: 80 });
  });
});

// --- 확정 (AC-03) ---------------------------------------------------------

describe('드래그의 끝 — 마지막 유효 위치를 확정한다 (AC-03)', () => {
  it('pointerup 은 그 이벤트의 자리로 확정한다', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 30, 15);
    send('pointerup', 50, 25);

    expect(emittedGeometry(emit, 'a')).toEqual({ x: 100, y: 80, w: 100, h: 80 });
  });

  it('pointercancel 은 되돌리지 않고 마지막 자리를 그대로 확정한다', () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 30, 15);
    // 프레임을 기다리지 않고 끊는다 — 대기 중이던 이동이 사라지면 안 된다.
    send('pointermove', 50, 25);
    send('pointercancel', 0, 0);

    expect(emittedGeometry(emit, 'a')).toEqual({ x: 100, y: 80, w: 100, h: 80 });
  });

  it('손을 뗀 뒤의 움직임은 더 이상 옮기지 않는다', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 30, 15);
    send('pointerup', 50, 25);
    const after = emit.mock.calls.length;
    send('pointermove', 90, 45);
    await nextFrame();

    expect(emit).toHaveBeenCalledTimes(after);
  });
});

// --- 외곽선 자리 (AC-E2 · AC-E6 · AC-E7) ----------------------------------

describe('선택 외곽선은 투영 결과와 같은 자리에 선다 (AC-E2)', () => {
  it('사각형 외곽선이 projectBox 결과와 일치한다', () => {
    render(<Harness elements={[rect('a', { x: 50, y: 40, w: 100, h: 160 })]} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    send('pointerdown', 30, 15);

    const outline = screen.getByTestId('canvas-selection-a');
    expect(outline.style.left).toBe('20px');
    expect(outline.style.top).toBe('10px');
    expect(outline.style.width).toBe('40px');
    expect(outline.style.height).toBe('40px');
  });

  it('선 외곽선은 두 끝점을 감싼다', () => {
    const line: CanvasElement = {
      id: 'l',
      kind: 'line',
      geometry: { x1: 50, y1: 80, x2: 250, y2: 240 },
      style: {},
    };
    render(<Harness elements={[line]} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    send('pointerdown', 60, 40);

    const outline = screen.getByTestId('canvas-selection-l');
    expect(outline.style.left).toBe('20px');
    expect(outline.style.top).toBe('20px');
    expect(outline.style.width).toBe('80px');
    expect(outline.style.height).toBe('40px');
  });

  it('문구 외곽선의 세로 중심이 기준점이다 (textBaseline 이 middle 이다)', () => {
    const text: CanvasElement = {
      id: 't',
      kind: 'text',
      geometry: { x: 250, y: 200 },
      style: { fontSize: 20 },
      text: 'abc',
    };
    render(<Harness elements={[text]} textWidths={{ t: 30 }} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    send('pointerdown', 110, 50);

    const outline = screen.getByTestId('canvas-selection-t');
    // 기준점 (100, 50). 상단은 50 - 20/2 = 40 이며, 상단으로 착각하면 50 이 된다.
    expect(outline.style.left).toBe('100px');
    expect(outline.style.top).toBe('40px');
    expect(outline.style.width).toBe('30px');
    expect(outline.style.height).toBe('20px');
  });

  it('실측 폭이 아직 없으면 폭 0 으로 보되 외곽선은 보인다 (AC-E7 · AC-E6)', () => {
    const text: CanvasElement = {
      id: 't',
      kind: 'text',
      geometry: { x: 250, y: 200 },
      style: { fontSize: 20 },
      text: 'abc',
    };
    render(<Harness elements={[text]} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    // 폭이 없으면 기준점 둘레의 여유 상자로 폴백해 골라진다.
    send('pointerdown', 100, 50);

    const outline = screen.getByTestId('canvas-selection-t');
    expect(outline.style.left).toBe('100px');
    expect(outline.style.width).toBe('2px');
  });

  it('크기 0 인 퇴화 도형도 외곽선이 보인다 (AC-E6)', () => {
    render(<Harness elements={[rect('a', { x: 250, y: 200, w: 0, h: 0 })]} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    send('pointerdown', 100, 50);

    const outline = screen.getByTestId('canvas-selection-a');
    expect(outline.style.left).toBe('100px');
    expect(outline.style.width).toBe('2px');
    expect(outline.style.height).toBe('2px');
  });

  it('음수 크기 박스도 양수 범위로 펴서 두른다', () => {
    render(<Harness elements={[rect('a', { x: 250, y: 200, w: -100, h: -80 })]} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    send('pointerdown', 100, 50);

    const outline = screen.getByTestId('canvas-selection-a');
    expect(outline.style.left).toBe('60px');
    expect(outline.style.top).toBe('30px');
    expect(outline.style.width).toBe('40px');
    expect(outline.style.height).toBe('20px');
  });
});

// --- 정리 ---------------------------------------------------------------

describe('언마운트 정리', () => {
  it('예약된 합류 프레임을 남기지 않는다', async () => {
    const emit = vi.fn();
    const cancel = vi.spyOn(globalThis, 'cancelAnimationFrame');
    const view = render(
      <Harness elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]} onElementsChange={emit} />,
    );
    stubOverlayRect();

    send('pointerdown', 30, 15);
    send('pointermove', 50, 25);
    view.unmount();

    expect(cancel).toHaveBeenCalled();
    await nextFrame();
    expect(emit).not.toHaveBeenCalled();
  });
});

// --- 포인터 캡처와 포인터 짝맞춤 -----------------------------------------

describe('포인터 캡처 — 스테이지를 벗어나도 이벤트가 이어진다', () => {
  it('드래그를 시작하면 포인터를 잡고, 손을 떼면 놓아준다', () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]} onElementsChange={emit} />);
    stubOverlayRect();
    const api = stubPointerCapture();

    send('pointerdown', 30, 15, {}, 7);
    expect(api.setPointerCapture).toHaveBeenCalledWith(7);

    send('pointerup', 50, 25, {}, 7);
    expect(api.hasPointerCapture).toHaveBeenCalledWith(7);
    expect(api.releasePointerCapture).toHaveBeenCalledWith(7);
    expect(emittedGeometry(emit, 'a')).toEqual({ x: 100, y: 80, w: 100, h: 80 });
  });

  it('잡은 적이 없으면 놓아 달라고 하지 않는다 (브라우저가 예외를 던진다)', () => {
    render(<Harness elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    const api = stubPointerCapture(false);

    send('pointerdown', 30, 15, {}, 7);
    send('pointerup', 50, 25, {}, 7);

    expect(api.releasePointerCapture).not.toHaveBeenCalled();
  });

  it('다른 포인터의 이동·놓기·취소는 무시한다 (멀티터치 방어)', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 30, 15, {}, 1);
    send('pointermove', 50, 25, {}, 2);
    send('pointerup', 90, 45, {}, 2);
    send('pointercancel', 0, 0, {}, 2);
    await nextFrame();

    expect(emit).not.toHaveBeenCalled();
  });

  it('드래그가 없을 때의 놓기·취소는 아무 일도 하지 않는다', () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]} onElementsChange={emit} />);
    stubOverlayRect();

    const up = send('pointerup', 50, 25);
    const cancel = send('pointercancel', 50, 25);

    expect(up.defaultPrevented).toBe(false);
    expect(cancel.defaultPrevented).toBe(false);
    expect(emit).not.toHaveBeenCalled();
  });
});

// --- 손상·결측 값 방어 ----------------------------------------------------

describe('결측·손상 값에도 외곽선과 쓰기가 무너지지 않는다', () => {
  it('글자 크기를 밝히지 않으면 기본 크기(14px)로 상자를 잡는다', () => {
    const text: CanvasElement = {
      id: 't',
      kind: 'text',
      geometry: { x: 250, y: 200 },
      style: {},
      text: 'abc',
    };
    render(<Harness elements={[text]} textWidths={{ t: 30 }} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    send('pointerdown', 110, 50);

    const outline = screen.getByTestId('canvas-selection-t');
    expect(outline.style.height).toBe('14px');
    expect(outline.style.top).toBe('43px');
  });

  it('글자 크기가 손상되었으면(0) 기본 크기로 폴백한다', () => {
    const text: CanvasElement = {
      id: 't',
      kind: 'text',
      geometry: { x: 250, y: 200 },
      style: { fontSize: 0 },
      text: 'abc',
    };
    render(<Harness elements={[text]} textWidths={{ t: 30 }} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    send('pointerdown', 110, 50);

    expect(screen.getByTestId('canvas-selection-t').style.height).toBe('14px');
  });

  it('움직이지 않은 채 취소되면 쓸 것이 없다', () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 30, 15);
    send('pointercancel', 0, 0);

    expect(emit).not.toHaveBeenCalled();
  });

  it('끄는 도중 스테이지가 무너지면(높이 0) 쓰지 않는다', async () => {
    const emit = vi.fn();
    const elements = [rect('a', { x: 50, y: 40, w: 100, h: 80 })];
    const view = render(<Harness elements={elements} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 30, 15);
    view.rerender(
      <Harness elements={elements} stage={{ width: 200, height: 0 }} onElementsChange={emit} />,
    );
    send('pointermove', 50, 25);
    await nextFrame();

    expect(emit).not.toHaveBeenCalled();
  });
});

// =========================================================================
// 크기 조절 핸들 (SPEC-CANVAS-002 T8)
// =========================================================================
//
// 덮는 인수 기준: AC-04(종류마다 다른 핸들 집합·Shift 종횡비·각도 죔),
// AC-08(핸들은 초점을 받고 `aria-label` 을 가진다), AC-E2(핸들 px 가 투영 px 와 일치),
// AC-E4(선택·호버·초점은 프레임을 예약하지 않는다), AC-E5(clamp 금지),
// AC-E6(뒤집혀도 음수 크기가 저장되지 않는다).
//
// 위 1차 테스트가 세운 관용구를 그대로 쓴다 — 좌표를 싣는 포인터 이벤트는 `MouseEvent` 로
// 만든다(jsdom 에 `PointerEvent` 가 없어 `fireEvent.pointerDown` 은 좌표를 버린다).
// 핸들 자리는 **하드코딩하지 않고** `handlePositions` 로 계산해 맞춘다 — 숫자를 베껴 두면
// 그 숫자가 곧 두 번째 투영이 되어 AC-E2 가 지키려던 성질이 테스트 안에서 깨진다.

import koMessages from '@/lib/i18n/ko.json';
import enMessages from '@/lib/i18n/en.json';

import type { LineGeometry, PointGeometry } from './canvasConfig';
import {
  CANVAS_GRID_STEP_CHOICES,
  CANVAS_GRID_STEP_MAX,
  CANVAS_GRID_STEP_MIN,
  CANVAS_GRID_STEP_UNITS,
} from './canvasEditArrange';
import {
  BOX_HANDLE_IDS,
  CANVAS_FONT_SIZE_MAX,
  CANVAS_FONT_SIZE_MIN,
  handlePositions,
} from './canvasEditGeometry';
import type { CanvasNode } from './group/groupTypes';

// --- 고정 입력 -----------------------------------------------------------

/** 스테이지 위 px 상자가 (40,20,80,40) 이 되는 사각형 — 여덟 핸들 자리가 모두 정수다. */
const BOX_GEOMETRY: BoxGeometry = { x: 100, y: 80, w: 200, h: 160 };

const LINE_ELEMENT: CanvasElement = {
  id: 'l',
  kind: 'line',
  geometry: { x1: 50, y1: 80, x2: 250, y2: 240 },
  style: {},
};

function textElement(id = 't', fontSize = 20): CanvasElement {
  return { id, kind: 'text', geometry: { x: 250, y: 200 }, style: { fontSize }, text: 'abc' };
}

// --- 핸들 관용구 ----------------------------------------------------------

/** 렌더된 핸들의 id 들(DOM 순서 = 탭 순서). */
function handleIds(): string[] {
  const root = screen.getByTestId('canvas-edit-overlay');
  return [...root.querySelectorAll<HTMLElement>('[data-testid^="canvas-handle-"]')].map((node) =>
    (node.dataset.testid ?? '').replace('canvas-handle-', ''),
  );
}

/** 핸들 버튼 하나. */
function handleEl(id: string): HTMLElement {
  return screen.getByTestId(`canvas-handle-${id}`);
}

/** 임의의 요소에 좌표를 실은 포인터 이벤트를 보낸다(핸들처럼 루트가 아닌 대상). */
function sendAt(target: Element, type: string, x: number, y: number, init: MouseEventInit = {}) {
  const evt = pointer(type, x, y, init);
  fireEvent(target, evt);
  return evt;
}

/**
 * 핸들을 잡아 끈다. 누름은 **핸들에**, 이동·놓기는 **루트에** 간다 — 실제 브라우저에서
 * `setPointerCapture` 가 이후 이벤트를 루트로 보내는 것과 같은 경로다.
 */
async function dragHandle(id: string, to: { x: number; y: number }, init: MouseEventInit = {}) {
  const el = handleEl(id);
  const from = { x: Number.parseFloat(el.style.left), y: Number.parseFloat(el.style.top) };
  sendAt(el, 'pointerdown', from.x, from.y, init);
  send('pointermove', to.x, to.y, init);
  await nextFrame();
}

/** 마지막으로 통보된 배열에서 한 요소를 꺼낸다. */
function emittedElement(spy: ReturnType<typeof vi.fn>, id: string): CanvasElement | undefined {
  const last = spy.mock.calls.at(-1)?.[0] as CanvasElement[] | undefined;
  return last?.find((el) => el.id === id);
}

/** 부동소수 오차를 견디는 박스 비교. `0.8 - 0.2 !== 0.6` 이므로 정확 비교는 쓸 수 없다. */
function expectBox(actual: unknown, expected: BoxGeometry): void {
  const g = actual as BoxGeometry;
  expect(g.x).toBeCloseTo(expected.x, 10);
  expect(g.y).toBeCloseTo(expected.y, 10);
  expect(g.w).toBeCloseTo(expected.w, 10);
  expect(g.h).toBeCloseTo(expected.h, 10);
}

/** 사각형 하나를 골라 둔 상태로 만든다(핸들이 뜨는 최소 조건). */
function renderSelectedBox(
  geometry: BoxGeometry = BOX_GEOMETRY,
  emit: ReturnType<typeof vi.fn> = vi.fn(),
) {
  render(<Harness elements={[rect('a', geometry)]} onElementsChange={emit} />);
  stubOverlayRect();
  send('pointerdown', 80, 40);
  return emit;
}

// --- 핸들 집합 (AC-04) ----------------------------------------------------

describe('종류마다 다른 핸들 집합이 뜬다 (AC-04)', () => {
  it('사각형은 모서리 4 + 변 4 = 8개이며 순서가 시계 방향이다', () => {
    renderSelectedBox();
    expect(handleIds()).toEqual([...BOX_HANDLE_IDS]);
  });

  it('타원도 같은 8개를 갖는다', () => {
    const ellipse: CanvasElement = { id: 'a', kind: 'ellipse', geometry: BOX_GEOMETRY, style: {} };
    render(<Harness elements={[ellipse]} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    send('pointerdown', 80, 40);

    expect(handleIds()).toEqual([...BOX_HANDLE_IDS]);
  });

  it('선은 끝점 2개뿐이다', () => {
    render(<Harness elements={[LINE_ELEMENT]} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    send('pointerdown', 60, 40);

    expect(handleIds()).toEqual(['p1', 'p2']);
  });

  it('문구는 **하나뿐이며 그것이 글자 크기 핸들이다** (박스 핸들이 없다)', () => {
    render(<Harness elements={[textElement()]} textWidths={{ t: 30 }} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    send('pointerdown', 110, 50);

    expect(handleIds()).toEqual(['font']);
  });

  it('아무것도 고르지 않았으면 핸들이 없다', () => {
    render(<Harness elements={[rect('a', BOX_GEOMETRY)]} onElementsChange={vi.fn()} />);
    stubOverlayRect();

    expect(handleIds()).toEqual([]);
  });

  it('둘 이상 고르면 핸들이 없다 — 무리의 조작은 이동과 정렬이다', () => {
    render(
      <Harness
        elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 }), rect('b', { x: 250, y: 200, w: 100, h: 80 })]}
        onElementsChange={vi.fn()}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 30, 15);
    expect(handleIds()).toEqual([...BOX_HANDLE_IDS]);

    send('pointerdown', 110, 55, { shiftKey: true });

    expect(selectionText()).toBe('a,b');
    expect(handleIds()).toEqual([]);
  });

  it('골라 둔 요소가 사라지면 핸들도 사라진다 (선택에는 id 가 남는다)', () => {
    const emit = vi.fn();
    const view = render(<Harness elements={[rect('a', BOX_GEOMETRY)]} onElementsChange={emit} />);
    stubOverlayRect();
    send('pointerdown', 80, 40);
    expect(handleIds()).toHaveLength(8);

    view.rerender(<Harness elements={[]} onElementsChange={emit} />);

    expect(selectionText()).toBe('a');
    expect(handleIds()).toEqual([]);
  });
});

// --- 핸들 자리 (AC-E2) ----------------------------------------------------

describe('핸들 px 는 투영 결과와 정확히 같다 (AC-E2)', () => {
  it('사각형 여덟 핸들이 handlePositions 와 일치한다', () => {
    const element = rect('a', BOX_GEOMETRY);
    renderSelectedBox();

    for (const handle of handlePositions(element, PROJ)) {
      const node = handleEl(handle.id);
      expect(node.style.left).toBe(`${handle.point.x}px`);
      expect(node.style.top).toBe(`${handle.point.y}px`);
    }
  });

  it('선 끝점 핸들이 projectLine 결과에 앉는다', () => {
    render(<Harness elements={[LINE_ELEMENT]} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    send('pointerdown', 60, 40);

    for (const handle of handlePositions(LINE_ELEMENT, PROJ)) {
      expect(handleEl(handle.id).style.left).toBe(`${handle.point.x}px`);
      expect(handleEl(handle.id).style.top).toBe(`${handle.point.y}px`);
    }
    // 투영이 실제로 두 끝점을 가리키는지 한 번은 눈으로 확인해 둔다.
    expect(handleEl('p1').style.left).toBe('20px');
    expect(handleEl('p2').style.left).toBe('100px');
  });

  it('문구의 글자 크기 핸들은 실측 폭을 넘겨받아 자리를 잡는다', () => {
    const element = textElement();
    render(<Harness elements={[element]} textWidths={{ t: 30 }} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    send('pointerdown', 110, 50);

    const [handle] = handlePositions(element, PROJ, { measuredWidth: 30 });
    expect(handleEl('font').style.left).toBe(`${handle!.point.x}px`);
    expect(handleEl('font').style.top).toBe(`${handle!.point.y}px`);
  });

  it('스테이지가 바뀌면 핸들이 요소와 함께 옮겨 간다 (스스로 재지 않는다)', () => {
    const emit = vi.fn();
    const elements = [rect('a', BOX_GEOMETRY)];
    const view = render(<Harness elements={elements} onElementsChange={emit} />);
    stubOverlayRect();
    send('pointerdown', 80, 40);
    expect(handleEl('se').style.left).toBe('120px');

    const wider: StageSize = { width: 400, height: 200 };
    view.rerender(<Harness elements={elements} stage={wider} onElementsChange={emit} />);

    for (const handle of handlePositions(elements[0]!, { stage: wider, canvas: CANVAS })) {
      expect(handleEl(handle.id).style.left).toBe(`${handle.point.x}px`);
      expect(handleEl(handle.id).style.top).toBe(`${handle.point.y}px`);
    }
    expect(handleEl('se').style.left).toBe('240px');
  });
});

// --- 박스 크기 조절 (AC-04 · AC-E5 · AC-E6) -------------------------------

describe('박스 핸들을 끌면 그 모서리·변만 움직인다 (AC-04)', () => {
  it('오른쪽 아래 모서리는 오른쪽 변과 아래 변을 함께 옮긴다', async () => {
    const emit = renderSelectedBox();
    await dragHandle('se', { x: 160, y: 80 });

    expectBox(emittedGeometry(emit, 'a'), { x: 100, y: 80, w: 300, h: 240 });
  });

  it('왼쪽 위 모서리는 왼쪽 변과 위 변을 함께 옮긴다', async () => {
    const emit = renderSelectedBox();
    await dragHandle('nw', { x: 20, y: 10 });

    expectBox(emittedGeometry(emit, 'a'), { x: 50, y: 40, w: 250, h: 200 });
  });

  it('오른쪽 변 핸들은 세로를 건드리지 않는다', async () => {
    const emit = renderSelectedBox();
    await dragHandle('e', { x: 180, y: 90 });

    expectBox(emittedGeometry(emit, 'a'), { x: 100, y: 80, w: 350, h: 160 });
  });

  it('위쪽 변 핸들은 가로를 건드리지 않는다', async () => {
    const emit = renderSelectedBox();
    await dragHandle('n', { x: 10, y: 5 });

    expectBox(emittedGeometry(emit, 'a'), { x: 100, y: 20, w: 200, h: 220.0 });
  });

  it('반대 모서리 너머로 끌면 박스를 정규화해 **음수 크기를 저장하지 않는다** (AC-E6)', async () => {
    const emit = renderSelectedBox();
    // 오른쪽 아래를 왼쪽 위 바깥까지 끈다 — 뒤집힌 뒤에도 잡은 손잡이가 커서 아래에 남아야 한다.
    await dragHandle('se', { x: -40, y: -20 });

    const stored = emittedGeometry(emit, 'a') as BoxGeometry;
    expect(stored.w).toBeGreaterThan(0);
    expect(stored.h).toBeGreaterThan(0);
    // 좌표는 음수로 남는다 — 스테이지 밖은 합법이므로 clamp 하지 않는다(AC-E5).
    expectBox(stored, { x: -100, y: -80, w: 200, h: 160 });
  });

  it('크기 조절도 한 프레임에 한 번만 쓴다', async () => {
    const emit = renderSelectedBox();
    const el = handleEl('se');
    sendAt(el, 'pointerdown', 120, 60);
    send('pointermove', 130, 65);
    send('pointermove', 150, 75);
    send('pointermove', 160, 80);
    await nextFrame();

    expect(emit).toHaveBeenCalledTimes(1);
    expectBox(emittedGeometry(emit, 'a'), { x: 100, y: 80, w: 300, h: 240 });
  });

  it('손을 떼면 그 자리로 확정한다', () => {
    const emit = renderSelectedBox();
    sendAt(handleEl('se'), 'pointerdown', 120, 60);
    send('pointerup', 160, 80);

    expectBox(emittedGeometry(emit, 'a'), { x: 100, y: 80, w: 300, h: 240 });
  });
});

describe('Shift 는 모서리에서 종횡비를 지킨다 (AC-04)', () => {
  /** 가로:세로 = 2:1 인 사각형(캔버스 단위). 비가 1 이면 유지 여부를 구별할 수 없다. */
  const WIDE: BoxGeometry = { x: 100, y: 80, w: 200, h: 100 };

  it('Shift 없이 끌면 비가 자유롭게 바뀐다', async () => {
    const emit = renderSelectedBox(WIDE);
    await dragHandle('se', { x: 160, y: 90 });

    expectBox(emittedGeometry(emit, 'a'), { x: 100, y: 80, w: 300, h: 280 });
  });

  it('Shift 를 누른 채 모서리를 끌면 원래 비(2:1)가 유지된다', async () => {
    const emit = renderSelectedBox(WIDE);
    await dragHandle('se', { x: 160, y: 90 }, { shiftKey: true });

    const stored = emittedGeometry(emit, 'a') as BoxGeometry;
    expect(stored.w / stored.h).toBe(2);
    // 세로가 더 멀리 갔으므로 세로가 비를 정한다 — 280 × 2 = 560.
    expectBox(stored, { x: 100, y: 80, w: 560, h: 280 });
  });

  it('끌던 도중에 Shift 를 눌러도 그 자리에서 죄인다 (잡을 때 값을 얼리지 않는다)', async () => {
    const emit = renderSelectedBox(WIDE);
    sendAt(handleEl('se'), 'pointerdown', 120, 40);
    send('pointermove', 160, 90);
    await nextFrame();
    expectBox(emittedGeometry(emit, 'a'), { x: 100, y: 80, w: 300, h: 280 });

    send('pointermove', 160, 90, { shiftKey: true });
    await nextFrame();

    expectBox(emittedGeometry(emit, 'a'), { x: 100, y: 80, w: 560, h: 280 });
  });
});

// --- 선 끝점 (AC-04) ------------------------------------------------------

describe('선은 끝점만 움직인다 (AC-04)', () => {
  function renderSelectedLine(emit = vi.fn()) {
    render(<Harness elements={[LINE_ELEMENT]} onElementsChange={emit} />);
    stubOverlayRect();
    send('pointerdown', 60, 40);
    return emit;
  }

  it('끝점 하나를 끌면 반대 끝점은 그대로다', async () => {
    const emit = renderSelectedLine();
    await dragHandle('p2', { x: 180, y: 20 });

    const g = emittedGeometry(emit, 'l') as LineGeometry;
    expect(g.x1).toBeCloseTo(50, 10);
    expect(g.y1).toBeCloseTo(80, 10);
    expect(g.x2).toBeCloseTo(450, 10);
    expect(g.y2).toBeCloseTo(80, 10);
  });

  it('시작점 핸들은 시작점만 옮긴다', async () => {
    const emit = renderSelectedLine();
    await dragHandle('p1', { x: 20, y: 80 });

    const g = emittedGeometry(emit, 'l') as LineGeometry;
    expect(g.x1).toBeCloseTo(50, 10);
    expect(g.y1).toBeCloseTo(320, 10);
    expect(g.x2).toBeCloseTo(250, 10);
    expect(g.y2).toBeCloseTo(240, 10);
  });

  it('Shift 는 끝점 방향을 0°/45°/90° 로 죈다', async () => {
    const flat: CanvasElement = {
      id: 'l',
      kind: 'line',
      geometry: { x1: 250, y1: 200, x2: 300, y2: 200 },
      style: {},
    };
    const emit = vi.fn();
    render(<Harness elements={[flat]} onElementsChange={emit} />);
    stubOverlayRect();
    send('pointerdown', 110, 50);

    // 살짝 아래로 벌어진 방향 → 0° 로 죄여 y 가 시작점과 같아진다.
    await dragHandle('p2', { x: 160, y: 55 }, { shiftKey: true });
    expect((emittedGeometry(emit, 'l') as LineGeometry).y2).toBeCloseTo(200, 10);

    // 45° 근처 방향 → 두 축의 변위가 같아진다.
    await dragHandle('p2', { x: 160, y: 70 }, { shiftKey: true });
    const g = emittedGeometry(emit, 'l') as LineGeometry;
    expect(g.x2 - g.x1).toBeCloseTo(g.y2 - g.y1, 10);
  });

  it('Shift 가 없으면 끌린 자리를 그대로 쓴다', async () => {
    const emit = renderSelectedLine();
    await dragHandle('p2', { x: 160, y: 55 });

    const g = emittedGeometry(emit, 'l') as LineGeometry;
    expect(g.x2).toBeCloseTo(400, 10);
    expect(g.y2).toBeCloseTo(220.0, 10);
  });
});

// --- 글자 크기 (AC-04) ----------------------------------------------------

describe('문구의 크기 핸들은 기하가 아니라 글자 크기를 쓴다 (AC-04)', () => {
  function renderSelectedText(element = textElement(), emit = vi.fn()) {
    render(<Harness elements={[element]} textWidths={{ t: 30 }} onElementsChange={emit} />);
    stubOverlayRect();
    send('pointerdown', 110, 50);
    return emit;
  }

  it('끌면 style.fontSize 가 바뀌고 geometry 는 그대로다', async () => {
    const emit = renderSelectedText();
    // 핸들은 (130, 60) 에 있다. 대각선으로 40px 씩 끌면 두 축의 평균만큼 커진다.
    await dragHandle('font', { x: 170, y: 100 });

    const el = emittedElement(emit, 't');
    expect(el?.style.fontSize).toBeCloseTo(60, 10);
    expect(el?.geometry as PointGeometry).toEqual({ x: 250, y: 200 });
  });

  it('안쪽으로 끌면 작아진다', async () => {
    const emit = renderSelectedText();
    await dragHandle('font', { x: 120, y: 50 });

    expect(emittedElement(emit, 't')?.style.fontSize).toBeCloseTo(10, 10);
  });

  it('하한 아래로 끌어도 하한에서 멈춘다', async () => {
    const emit = renderSelectedText();
    await dragHandle('font', { x: -270, y: -340 });

    expect(emittedElement(emit, 't')?.style.fontSize).toBe(CANVAS_FONT_SIZE_MIN);
  });

  it('상한 위로 끌어도 상한에서 멈춘다', async () => {
    const emit = renderSelectedText();
    await dragHandle('font', { x: 530, y: 460 });

    expect(emittedElement(emit, 't')?.style.fontSize).toBe(CANVAS_FONT_SIZE_MAX);
  });

  it('글자 크기를 밝히지 않았으면 기본 크기(14px)에서 시작한다', async () => {
    const bare: CanvasElement = {
      id: 't',
      kind: 'text',
      geometry: { x: 250, y: 200 },
      style: {},
      text: 'abc',
    };
    const emit = renderSelectedText(bare);
    const from = handleEl('font');
    const x = Number.parseFloat(from.style.left);
    const y = Number.parseFloat(from.style.top);
    sendAt(from, 'pointerdown', x, y);
    send('pointermove', x + 10, y + 10);
    await nextFrame();

    expect(emittedElement(emit, 't')?.style.fontSize).toBeCloseTo(24, 10);
  });

  it('다른 요소의 글자 크기는 건드리지 않는다', async () => {
    const emit = vi.fn();
    const other: CanvasElement = {
      id: 'u',
      kind: 'text',
      geometry: { x: 50, y: 40 },
      style: { fontSize: 12 },
      text: 'abc',
    };
    render(
      <Harness
        elements={[textElement(), other]}
        textWidths={{ t: 30, u: 30 }}
        onElementsChange={emit}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 110, 50);
    await dragHandle('font', { x: 170, y: 100 });

    expect(emittedElement(emit, 't')?.style.fontSize).toBeCloseTo(60, 10);
    expect(emittedElement(emit, 'u')?.style.fontSize).toBe(12);
  });

  it('다른 스타일 값은 보존한다', async () => {
    const styled: CanvasElement = {
      id: 't',
      kind: 'text',
      geometry: { x: 250, y: 200 },
      style: { fontSize: 20, textColor: '#ff0000', fontWeight: 'bold' },
      text: 'abc',
    };
    const emit = renderSelectedText(styled);
    await dragHandle('font', { x: 170, y: 100 });

    const el = emittedElement(emit, 't');
    expect(el?.style.textColor).toBe('#ff0000');
    expect(el?.style.fontWeight).toBe('bold');
  });
});

// --- 핸들과 몸통의 경계 ----------------------------------------------------

describe('핸들을 잡은 포인터는 몸통 히트 테스트에 닿지 않는다', () => {
  it('핸들 누름은 선택을 바꾸지 않고 상위로 흘러가지도 않는다', () => {
    const parent = vi.fn();
    render(
      <Harness
        elements={[rect('a', BOX_GEOMETRY), rect('b', { x: 275, y: 220.0, w: 100, h: 80 })]}
        onElementsChange={vi.fn()}
        onParentDown={parent}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 80, 40);
    expect(selectionText()).toBe('a');
    parent.mockClear();

    // se 핸들 (120,60) 은 요소 b 의 상자(110,55,40,20) 안에 있다 — 몸통 판정이 돌면 b 로
    // 선택이 옮겨 가고 손잡이 대신 b 가 끌린다.
    const evt = sendAt(handleEl('se'), 'pointerdown', 120, 60);

    expect(selectionText()).toBe('a');
    expect(evt.defaultPrevented).toBe(true);
    expect(parent).not.toHaveBeenCalled();
  });

  it('핸들 드래그 중에는 다른 요소가 함께 움직이지 않는다', async () => {
    const emit = vi.fn();
    render(
      <Harness
        elements={[rect('a', BOX_GEOMETRY), rect('b', { x: 275, y: 220.0, w: 100, h: 80 })]}
        onElementsChange={emit}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 80, 40);
    await dragHandle('se', { x: 160, y: 80 });

    expect(emittedGeometry(emit, 'b')).toEqual({ x: 275, y: 220.0, w: 100, h: 80 });
  });

  it('드래그를 시작하면 **루트에서** 포인터를 잡는다', () => {
    renderSelectedBox();
    const api = stubPointerCapture();
    // pointerId 는 MouseEvent 에 없어 undefined 다 — 잡는 대상이 루트인지가 요점이다.
    sendAt(handleEl('se'), 'pointerdown', 120, 60);

    expect(api.setPointerCapture).toHaveBeenCalledTimes(1);
  });
});

// --- 접근성 (REQ-01 · AC-08) ----------------------------------------------

describe('핸들은 초점을 받는 진짜 요소다 (AC-08)', () => {
  it('버튼이며 초점을 받을 수 있다', () => {
    renderSelectedBox();
    const node = handleEl('se');

    expect(node.tagName).toBe('BUTTON');
    expect(node.getAttribute('type')).toBe('button');
    node.focus();
    expect(document.activeElement).toBe(node);
  });

  it('여덟 핸들이 저마다 다른 aria-label 을 가진다', () => {
    renderSelectedBox();
    const labels = [...BOX_HANDLE_IDS].map((id) => handleEl(id).getAttribute('aria-label'));

    expect(new Set(labels).size).toBe(8);
    // i18n 은 이 파일에서 키를 그대로 돌려준다 — 라벨의 출처가 번역이라는 뜻이다.
    expect(handleEl('se').getAttribute('aria-label')).toBe('dashboard.canvas.edit.handleSe');
  });

  it('선 끝점과 글자 크기 핸들도 라벨을 가진다', () => {
    render(<Harness elements={[LINE_ELEMENT]} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    send('pointerdown', 60, 40);

    expect(handleEl('p1').getAttribute('aria-label')).toBe('dashboard.canvas.edit.handleP1');
    expect(handleEl('p2').getAttribute('aria-label')).toBe('dashboard.canvas.edit.handleP2');

    cleanup();
    render(<Harness elements={[textElement()]} textWidths={{ t: 30 }} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    send('pointerdown', 110, 50);

    expect(handleEl('font').getAttribute('aria-label')).toBe('dashboard.canvas.edit.handleFont');
  });

  it('쓰인 라벨 키가 ko 와 en 양쪽에 실제로 있다', () => {
    renderSelectedBox();
    const keys = [...BOX_HANDLE_IDS, 'p1', 'p2', 'font'].map(
      (id) => `dashboard.canvas.edit.handle${id.charAt(0).toUpperCase()}${id.slice(1)}`,
    );
    const resolve = (tree: unknown, key: string): unknown =>
      key
        .split('.')
        .reduce<unknown>(
          (node, seg) =>
            node && typeof node === 'object' ? (node as Record<string, unknown>)[seg] : undefined,
          tree,
        );

    for (const key of keys) {
      expect(typeof resolve(koMessages, key)).toBe('string');
      expect(typeof resolve(enMessages, key)).toBe('string');
    }
    // 화면에 붙은 라벨이 그 키 집합과 정확히 같은지도 확인한다(오타 방지).
    expect([...BOX_HANDLE_IDS].map((id) => handleEl(id).getAttribute('aria-label'))).toEqual(
      keys.slice(0, 8),
    );
  });
});

// --- 유휴 정지 (AC-E4) ----------------------------------------------------

describe('선택·호버·초점은 프레임을 예약하지 않는다 (AC-E4)', () => {
  it('고르고 · 선택을 옮기고 · 핸들에 초점을 주고 · 호버해도 프레임 요청이 0 건이다', () => {
    render(
      <Harness
        elements={[rect('a', BOX_GEOMETRY), rect('b', { x: 375, y: 20, w: 100, h: 80 })]}
        onElementsChange={vi.fn()}
      />,
    );
    stubOverlayRect();
    const raf = vi.spyOn(globalThis, 'requestAnimationFrame');

    send('pointerdown', 80, 40); // 고른다
    send('pointerdown', 170, 15); // 다른 요소로 옮긴다
    send('pointerdown', 80, 40); // 도로 옮긴다
    handleEl('se').focus(); // 핸들에 초점
    fireEvent.pointerOver(handleEl('se')); // 핸들 위를 지나간다
    fireEvent.mouseOver(handleEl('se'));

    expect(handleIds()).toHaveLength(8);
    expect(raf).not.toHaveBeenCalled();
  });

  it('핸들을 실제로 끌 때에만 프레임이 예약된다 (기하가 바뀌기 때문이다)', () => {
    renderSelectedBox();
    const raf = vi.spyOn(globalThis, 'requestAnimationFrame');

    sendAt(handleEl('se'), 'pointerdown', 120, 60);
    expect(raf).not.toHaveBeenCalled();

    send('pointermove', 160, 80);

    expect(raf).toHaveBeenCalledTimes(1);
  });
});


// --- 도형 팔레트 (AC-05) --------------------------------------------------

/**
 * 팔레트 시험용 하네스. `elements` 를 **상태로** 든다 — 팔레트가 연속으로 놓을 때 계단
 * 오프셋이 배열 길이를 보므로, 배열이 되돌아오지 않으면 두 번째 누름이 첫 번째와 같은
 * 자리를 낸다(목록 편집기 시험의 `setupStateful` 과 같은 이유다).
 */
function PaletteHarness({ initial }: { initial: readonly CanvasElement[] }) {
  const [elements, setElements] = useState<readonly CanvasNode[]>(initial);
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <span data-testid="selection">{[...state.selection].join(',')}</span>
      <span data-testid="dump">{JSON.stringify(elements)}</span>
      <CanvasEditDockRegion enabled>
        <CanvasEditOverlay
          enabled
          elements={elements}
          projection={PROJ}
          textWidths={{}}
          onElementsChange={setElements}
        />
      </CanvasEditDockRegion>
    </CanvasEditSelectionContext>
  );
}

/** 하네스가 들고 있는 현재 요소 배열. */
function liveElements(): CanvasElement[] {
  return JSON.parse(screen.getByTestId('dump').textContent ?? '[]') as CanvasElement[];
}

/** 팔레트 버튼 하나를 누른다. */
function place(kind: string): void {
  fireEvent.click(screen.getByTestId(`canvas-palette-add-${kind}`));
}

describe('도형 팔레트 (AC-05)', () => {
  it('편집이 켜지면 네 종류의 버튼이 뜨고, 꺼지면 도구 자체가 없다', () => {
    render(<Harness enabled={false} elements={[]} onElementsChange={vi.fn()} />);
    expect(screen.queryByTestId('canvas-dock-panel')).toBeNull();

    cleanup();
    render(<PaletteHarness initial={[]} />);
    expect(screen.getByTestId('canvas-dock-panel')).toBeTruthy();
    for (const kind of ['rect', 'ellipse', 'line', 'text']) {
      expect(screen.getByTestId(`canvas-palette-add-${kind}`)).toBeTruthy();
    }
  });

  it('스테이지 위에 떠 있던 띠는 남아 있지 않다 — 도구는 도크 한 자리에만 있다', () => {
    // 둘이 남으면 같은 것을 두 자리에서 눌러야 하고, 그중 하나는 그림을 가린다.
    render(<PaletteHarness initial={[]} />);
    expect(screen.queryByTestId('canvas-palette')).toBeNull();
    // 도구는 오버레이 **밖**(도크)에 그려진다 — 스테이지를 한 뼘도 먹지 않는다.
    const overlay = screen.getByTestId('canvas-edit-overlay');
    expect(overlay.contains(screen.getByTestId('canvas-dock-panel'))).toBe(false);
    expect(
      screen.getByTestId('canvas-dock').contains(screen.getByTestId('canvas-dock-panel')),
    ).toBe(true);
  });

  it('버튼마다 aria-label 이 있고, 그 옆에 도형 이름이 보인다', () => {
    render(<PaletteHarness initial={[]} />);
    const button = screen.getByTestId('canvas-palette-add-rect');
    // 하는 일은 `aria-label` 이, 무엇인지는 보이는 글자가 말한다. 접근성 이름이 보이는
    // 라벨을 포함하므로 음성 조작이 보이는 대로 통한다(WCAG 2.5.3).
    expect(button.getAttribute('aria-label')).toBe('dashboard.canvas.edit.paletteRect');
    expect(button.textContent).toContain('dashboard.canvas.edit.shapeRect');
    expect(screen.getByTestId('canvas-dock-panel').getAttribute('aria-label')).toBe(
      'dashboard.canvas.edit.paletteAria',
    );
  });

  it('목록 편집기의 추가 버튼과 **같은 생성 경로**를 부른다 (가정 A7)', () => {
    // 팔레트가 낸 결과를 공유 모듈의 결과와 직접 맞춰 본다. 두 값이 같다는 것이 곧
    // "규칙이 하나" 라는 뜻이다 — 팔레트가 자기 씨앗·자기 계단을 가지면 여기서 갈라진다.
    const before = [rect('el-1', BOX_GEOMETRY)];
    render(<PaletteHarness initial={before} />);

    for (const kind of ['rect', 'ellipse', 'line', 'text'] as const) {
      cleanup();
      render(<PaletteHarness initial={before} />);
      place(kind);
      expect(liveElements()).toEqual(appendElement(before, kind).next);
    }
  });

  it('연속으로 놓아도 같은 자리에 겹쳐 쌓이지 않는다 (계단 오프셋)', () => {
    render(<PaletteHarness initial={[]} />);
    place('rect');
    place('rect');
    place('rect');

    const geos = liveElements().map((e) => JSON.stringify(e.geometry));
    expect(geos).toHaveLength(3);
    expect(new Set(geos).size).toBe(3);
    // 목록 편집기가 내는 자리와 같다.
    expect(geos[1]).toBe(JSON.stringify({ x: 75, y: 65, w: 100, h: 80 }));
  });

  it('새 요소가 배열 끝(맨 위)에 붙고 **선택된다** — 팔레트가 추가로 하는 유일한 일이다', () => {
    render(<PaletteHarness initial={[rect('el-1', BOX_GEOMETRY)]} />);
    place('rect');

    const els = liveElements();
    expect(els.map((e) => e.id)).toEqual(['el-1', 'el-2']);
    expect(screen.getByTestId('selection').textContent).toBe('el-2');
  });

  it('선택되었으므로 그 자리에 외곽선과 손잡이가 붙는다 — 다음 몸짓이 배치 드래그다', () => {
    render(<PaletteHarness initial={[]} />);
    place('rect');

    expect(screen.getByTestId('canvas-selection-el-1')).toBeTruthy();
    expect(handleIds()).toHaveLength(8);
  });

  it('놓을 때마다 선택이 **새 것 하나로** 갈린다', () => {
    render(<PaletteHarness initial={[]} />);
    place('rect');
    place('text');
    expect(screen.getByTestId('selection').textContent).toBe('el-2');
  });

  it('팔레트 위의 누름은 아래 캔버스의 히트 테스트까지 흘러가지 않는다', () => {
    // 끊지 않으면 버튼을 눌렀는데 그 뒤 도형이 함께 골라지고 이동 드래그까지 시작된다.
    render(<PaletteHarness initial={[rect('a', { x: 0, y: 0, w: 500, h: 400 })]} />);
    stubOverlayRect();

    const evt = pointer('pointerdown', 5, 5);
    fireEvent(screen.getByTestId('canvas-dock-panel'), evt);

    // 스테이지를 가득 채운 사각형 위인데도 아무것도 골라지지 않았다.
    expect(screen.getByTestId('selection').textContent).toBe('');
  });

  it('팔레트로 놓은 뒤에는 그 요소를 곧바로 끌 수 있다', async () => {
    render(<PaletteHarness initial={[]} />);
    stubOverlayRect();
    place('rect');

    // 씨앗 사각형은 정규화 {0.1,0.1,0.2,0.2} → 200×100 스테이지에서 x 20..60, y 10..30.
    send('pointerdown', 40, 20);
    send('pointermove', 60, 30);
    await nextFrame();

    const moved = liveElements()[0]!.geometry as { x: number; y: number };
    expect(moved.x).toBeCloseTo(100, 6);
    expect(moved.y).toBeCloseTo(80, 6);
  });
});


// --- 격자 붙임 · 정렬 · z-order (T12 · T13 · T14) --------------------------
//
// 이 절의 중심은 **위험 R6** 이다. 백분율 공간의 오프셋 상한 기본값이 ±40 이라, 붙임이
// 그 공간을 지나면 캔버스 드래그가 기준 상자의 40% 지점에서 조용히 멈춘다 — 오류가
// 아니라 "왜 더 안 가지" 로만 보인다. 0.8.0 은 붙임을 정수 반올림으로 바꿔 그 공간을
// 아예 지나지 않게 했지만, 시험은 남긴다: 구조가 다시 바뀔 수 있고 그때 이 시험이
// 먼저 깨져야 한다. 그래서 아래에는 **40% 를 한참 넘겨 계속 끄는** 시험이 있다.
//
// 정렬·순서는 순수 모듈(`canvasEditArrange`)이 규칙을 소유하므로 여기서는 **배선**을 본다:
// 버튼이 어떤 축·방식을 부르는가, 몇 개 골랐을 때 쓸 수 있는가, 쓰기가 실제로 나가는가.

/** 격자 붙임 토글 버튼. */
function gridToggle(): HTMLElement {
  return screen.getByTestId('canvas-grid-toggle');
}

/**
 * 캔버스 (25,40,90,80) — 스테이지 px 상자는 (10,10,36,20) 이다.
 *
 * 중심이 (70, 80) 으로 **어느 격자 눈금 위도 아니다**(10·20·25·50 어느 것으로도
 * 나누어떨어지지 않는다). 눈금 위에서 시작하면 붙임이 무동작이 되어 시험이 성립하지 않는다.
 */
const SNAP_GEOMETRY: BoxGeometry = { x: 25, y: 40, w: 90, h: 80 };

describe('격자 붙임 — 토글 하나가 표시와 붙임을 함께 켠다 (T12)', () => {
  it('처음에는 꺼져 있고 격자도 없다 — 붙임은 켜야 하는 것이지 기본값이 아니다', () => {
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={vi.fn()} />);
    expect(gridToggle().getAttribute('aria-pressed')).toBe('false');
    expect(screen.queryByTestId('panel-edit-grid')).toBeNull();
  });

  it('누르면 켜지고 **기존 `PanelEditGrid`** 가 그대로 뜬다 (신규 격자 컴포넌트가 없다)', () => {
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={vi.fn()} />);
    fireEvent.click(gridToggle());

    expect(gridToggle().getAttribute('aria-pressed')).toBe('true');
    // `panel-edit-grid` / `panel-edit-center` 는 그 공용 컴포넌트가 스스로 다는 표식이다.
    expect(screen.getByTestId('panel-edit-grid')).toBeTruthy();
    expect(screen.getByTestId('panel-edit-center')).toBeTruthy();
  });

  it('다시 누르면 꺼진다', () => {
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={vi.fn()} />);
    fireEvent.click(gridToggle());
    fireEvent.click(gridToggle());

    expect(gridToggle().getAttribute('aria-pressed')).toBe('false');
    expect(screen.queryByTestId('panel-edit-grid')).toBeNull();
  });

  it('격자는 포인터를 받지 않는다 — 참조선이 이벤트를 먹으면 그 위를 지나는 드래그가 끊긴다', () => {
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={vi.fn()} />);
    fireEvent.click(gridToggle());

    expect(screen.getByTestId('panel-edit-grid').className).toContain('pointer-events-none');
  });

  it('토글은 프레임을 예약하지 않는다 — DOM 뿐이라 캔버스 props 를 건드리지 않는다 (AC-E4)', () => {
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={vi.fn()} />);
    const raf = vi.spyOn(globalThis, 'requestAnimationFrame');

    fireEvent.click(gridToggle());
    fireEvent.click(gridToggle());

    expect(raf).not.toHaveBeenCalled();
  });

  it('꺼져 있으면 끈 만큼 그대로 간다 (죄지 않는 경로)', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 20, 20);
    send('pointermove', 60, 20);
    await nextFrame();

    // 40px = 캔버스 100 단위. 붙임이 꺼져 있으므로 그대로다.
    expect((emittedGeometry(emit, 'a') as BoxGeometry).x).toBe(125);
  });

  it('켜져 있으면 **중심**이 격자에 붙는다 (다른 패널의 격자와 같은 기준)', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={emit} />);
    stubOverlayRect();
    fireEvent.click(gridToggle());

    send('pointerdown', 20, 20);
    send('pointermove', 60, 20);
    await nextFrame();

    // 중심 70 + 100 = 170 → 기본 간격 25 의 눈금 175 로 붙는다 → 이동량 105 → x = 130.
    const moved = emittedGeometry(emit, 'a') as BoxGeometry;
    expect(moved.x).toBe(130);
    // 중심이 실제로 눈금(175) 위에 앉았다.
    expect((moved.x + moved.w / 2) % CANVAS_GRID_STEP_UNITS).toBe(0);
  });

  it('세로도 자기 축 길이로 죈다 (축을 뒤바꾸지 않는다)', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={emit} />);
    stubOverlayRect();
    fireEvent.click(gridToggle());

    // 세로 12px = 캔버스 48 단위. 중심 80 + 48 = 128 → 눈금 125 로 되돌아 붙는다.
    send('pointerdown', 20, 20);
    send('pointermove', 20, 32);
    await nextFrame();

    const moved = emittedGeometry(emit, 'a') as BoxGeometry;
    expect(moved.y).toBe(85);
    // 두 축이 **같은 정수 간격**을 쓴다 — 축마다 다른 백분율이 아니다.
    expect((moved.y + moved.h / 2) % CANVAS_GRID_STEP_UNITS).toBe(0);
  });

  it('**스테이지의 40% 를 한참 넘겨도 계속 간다** — 상한이 새어 들어오면 0.4 에서 멈춘다 (위험 R6 · AC-E5)', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={emit} />);
    stubOverlayRect();
    fireEvent.click(gridToggle());

    send('pointerdown', 20, 20);
    // 180px = 캔버스 450 단위 — 이미 옛 상한(캔버스 폭의 40% = 200)의 두 배가 넘는다.
    send('pointermove', 200, 20);
    await nextFrame();
    const half = emittedGeometry(emit, 'a') as BoxGeometry;
    expect(half.x).toBe(480);
    expect(half.x).toBeGreaterThan(CANVAS.width * 0.4);

    // 그리고 **계속 간다** — 캔버스를 통째로 벗어난 뒤에도 붙잡히지 않는다.
    send('pointermove', 410, 20);
    await nextFrame();
    const far = emittedGeometry(emit, 'a') as BoxGeometry;
    expect(far.x).toBe(1005);

    // clamp 도 없다 — 캔버스 안으로 잘리지 않고 그대로 저장된다(가정 A5).
    expect(far.x).toBeGreaterThan(CANVAS.width);
  });

  it('무리 이동은 잡은 요소 하나를 기준으로 죈다 (무리가 흩어지지 않는다)', async () => {
    const emit = vi.fn();
    render(
      <Harness
        elements={[rect('a', SNAP_GEOMETRY), rect('b', { x: 300, y: 240, w: 50, h: 40 })]}
        onElementsChange={emit}
      />,
    );
    stubOverlayRect();
    fireEvent.click(gridToggle());

    send('pointerdown', 20, 20);
    send('pointerdown', 130, 65, { shiftKey: true });
    expect(selectionText()).toBe('a,b');

    send('pointerdown', 20, 20);
    send('pointermove', 60, 20);
    await nextFrame();

    // 기준은 잡은 a 다. 둘 다 **같은 이동량**(105 단위)을 받는다.
    expect((emittedGeometry(emit, 'a') as BoxGeometry).x).toBe(130);
    expect((emittedGeometry(emit, 'b') as BoxGeometry).x).toBe(405);
  });
});

// --- 격자 간격 -------------------------------------------------------------
//
// 이 절이 지키는 성질은 **하나**다: 화면에 그려진 간격과 실제로 붙는 간격이 같다.
// 둘이 갈라지면 화면이 거짓말을 하고, 그 거짓말은 "붙긴 붙는데 선하고 안 맞는다" 로만
// 보고되어 원인을 찾기 어렵다. 그래서 간격을 바꿀 때마다 **그린 것과 붙은 좌표를 함께**
// 본다 — CSS 만 보는 시험은 둘이 갈라지는 순간을 보지 못한다.
//
// (jsdom 은 Tailwind 를 돌리지 않으므로 클래스 이름을 픽셀로 바꿀 수 없다. 여기서 CSS 를
// 보는 자리는 인라인 `style` 로 들어가는 `background-image` 뿐이며, 그것은 이 컴포넌트가
// 실제로 쓴 값 그대로다.)

/**
 * 격자 간격 칸. 0.11.0 에서 목록(`<select>`)이 자유 입력(`<input type="number">`)이 되었다 —
 * 재는 것(값 · 잠김 · 바꾸면 그림과 붙임이 따라오는가)은 그대로이므로 픽스처만 갈아탄다.
 */
function gridStepSelect(): HTMLInputElement {
  return screen.getByTestId('canvas-grid-step') as HTMLInputElement;
}

/** 격자 간격을 고른다(정수 캔버스 단위). */
function pickGridStep(step: number): void {
  fireEvent.change(gridStepSelect(), { target: { value: String(step) } });
}

/** 지금 그려진 격자의 `background-image`. */
function gridImage(): string {
  return screen.getByTestId('panel-edit-grid').style.backgroundImage;
}

/**
 * **그린 격자를 보는 시험이 쓰는 스테이지** — 캔버스(500x400)와 같은 5:4 다.
 *
 * 기본 `STAGE`(200x100, 2:1)는 축을 뒤바꾼 계산을 드러내려고 일부러 비율을 어긋내 둔
 * 것인데(위 §고정 입력), 0.10.0 이후 표면은 **축척 하나**로 그리므로 실제 화면에서
 * `projection.stage` 는 언제나 캔버스 비율이다. 비율이 어긋난 스테이지를 손으로 넘기면
 * 그때만 나오는 칸(짧은 축이 정한 값)을 보게 되어, "그린 간격과 붙는 간격이 같다" 라는
 * 이 절의 성질을 물을 수 없다. 그래서 격자를 **그림으로** 보는 자리에서만 비율을 맞춘다.
 */
const GRID_STAGE: StageSize = { width: 200, height: 160 };

/** 위 스테이지로 오버레이를 세우고 화면 자리까지 심는다(축척 1). */
function renderGridHarness(emit: (next: CanvasNode[]) => void = vi.fn()): void {
  render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} stage={GRID_STAGE} onElementsChange={emit} />);
  stubOverlayRect(0, 0, GRID_STAGE.width, GRID_STAGE.height);
}

/**
 * 격자를 켜고 간격을 고른 뒤 24px(= 캔버스 60 단위)를 끈다.
 *
 * 중심 70 + 60 = 130 은 **네 간격이 모두 다른 눈금**으로 붙는 자리다 —
 * 10 → 130, 20 → 140, 25 → 125, 50 → 150. 넷이 갈리므로 "간격이 실제로 붙임까지
 * 갔는가" 가 좌표 하나로 드러난다.
 */
async function dragOffGrid(step?: number): Promise<ReturnType<typeof vi.fn>> {
  const emit = vi.fn();
  renderGridHarness(emit);
  fireEvent.click(gridToggle());
  if (step !== undefined) pickGridStep(step);

  send('pointerdown', 20, 20);
  send('pointermove', 44, 20);
  await nextFrame();
  return emit;
}

describe('격자 간격 — 고른 값 하나가 그림과 붙임을 함께 정한다', () => {
  it('처음에는 공용 기본 간격이며, 그 간격으로 그려진다', () => {
    renderGridHarness();
    fireEvent.click(gridToggle());

    expect(gridStepSelect().value).toBe(String(CANVAS_GRID_STEP_UNITS));
    // 그려지는 값은 **px** 다(0.9.0). 백분율이면 브라우저가 상자 폭에 곱하는 순간
    // 소수가 되어 선이 두 픽셀에 걸친다. 25 단위 → 200×25/500 = 160×25/400 = 10px 이며,
    // **두 축이 같은 값이다**(0.10.0 — 칸은 정사각형이다).
    expect(gridImage()).toContain('transparent 1px 10px');
    expect(gridImage().match(/transparent 1px 10px/g)).toHaveLength(2);
    expect(gridImage()).not.toContain('%');
  });

  it('제안값은 canvasEditArrange 가 소유한 목록 그대로다', () => {
    // 0.11.0: 목록은 **고를 수 있는 값의 전부**에서 **곁들이는 제안**으로 격이 바뀌었다.
    // 재는 것은 그대로다 — 그 넷을 화면이 실제로 내놓는가, 그리고 그 출처가 여전히
    // `canvasEditArrange` 한 곳인가.
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={vi.fn()} />);

    const list = screen.getByTestId('canvas-grid-step-suggestions');
    const values = [...list.querySelectorAll('option')].map((o) => o.value);
    expect(values).toEqual(CANVAS_GRID_STEP_CHOICES.map(String));
    // 칸이 그 목록을 실제로 가리킨다 — 이어져 있지 않으면 제안은 화면에 뜨지 않는다.
    expect(gridStepSelect().getAttribute('list')).toBe(list.id);
  });

  it('목록 밖의 정수도 그대로 받는다 — 목록은 이제 난간이 아니다', () => {
    renderGridHarness();
    fireEvent.click(gridToggle());

    // 7 은 목록에 없고 캔버스 두 축(500 × 400)을 나누지도 못한다. 그래도 값은 그대로 선다.
    pickGridStep(7);
    expect(gridStepSelect().value).toBe('7');
  });

  it('난간 밖의 입력은 범위 안으로 죈다 (0 · 음수 · 천문학적 수)', () => {
    renderGridHarness();
    fireEvent.click(gridToggle());

    pickGridStep(0);
    expect(gridStepSelect().value).toBe(String(CANVAS_GRID_STEP_MIN));
    pickGridStep(-40);
    expect(gridStepSelect().value).toBe(String(CANVAS_GRID_STEP_MIN));
    pickGridStep(999999);
    expect(gridStepSelect().value).toBe(String(CANVAS_GRID_STEP_MAX));
    // 화면도 같은 수를 적어 둔다 — 죄는 것을 조용히 하려면 난간이 보여야 한다.
    expect(gridStepSelect().getAttribute('min')).toBe(String(CANVAS_GRID_STEP_MIN));
    expect(gridStepSelect().getAttribute('max')).toBe(String(CANVAS_GRID_STEP_MAX));
  });

  it('읽을 수 없는 입력은 옛 값을 지킨다 — 한 글자를 지우는 도중은 잘못된 상태가 아니다', () => {
    renderGridHarness();
    fireEvent.click(gridToggle());
    pickGridStep(20);

    fireEvent.change(gridStepSelect(), { target: { value: '' } });
    expect(gridStepSelect().value).toBe('20');
  });

  it('나누어떨어지지 않는 값에는 `?` 고지가 붙고, 떨어지면 사라진다', () => {
    renderGridHarness();
    fireEvent.click(gridToggle());

    // 25 는 기본 캔버스(500 × 400)의 두 축을 모두 나눈다 — 자투리가 없다.
    expect(screen.queryByTestId('canvas-grid-step-partial')).toBeNull();

    pickGridStep(7);
    const hint = screen.getByTestId('canvas-grid-step-partial');
    // 고지는 **읽히도록** 붙는다 — 눈으로 보는 사람에게는 `?`, 보조기기에게는 이 연결이다.
    // (문구에 실제 수가 들어가는지는 `CanvasEditDock.gridStep.test.tsx` 가 본다 — 이 파일의
    // i18n 은 키를 그대로 돌려주므로 자리표시자가 채워지지 않는다.)
    expect(gridStepSelect().getAttribute('aria-describedby')).toBe(hint.id);

    pickGridStep(50);
    expect(screen.queryByTestId('canvas-grid-step-partial')).toBeNull();
  });

  it('격자가 꺼져 있으면 고르개도 꺼진다 — 눌러도 화면이 그대로인 컨트롤은 고장으로 보인다', () => {
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={vi.fn()} />);
    expect(gridStepSelect().disabled).toBe(true);

    fireEvent.click(gridToggle());
    expect(gridStepSelect().disabled).toBe(false);
  });

  it('캔버스는 **진한** 격자를 쓴다 — 칠해진 도형 위에 얹히기 때문이다', () => {
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={vi.fn()} />);
    fireEvent.click(gridToggle());

    // 다른 패널이 쓰는 기본 진하기(28%)가 그대로 나오면 도형 위에서 사실상 보이지 않는다.
    expect(gridImage()).not.toContain('rgba(148, 163, 184, 0.28)');
    expect(gridImage()).toContain('rgba(148, 163, 184, 0.6)');
  });

  it('간격을 바꾸면 **그린 격자**가 함께 바뀐다', () => {
    renderGridHarness();
    fireEvent.click(gridToggle());

    // 20 단위 → 200×20/500 = 160×20/400 = 8px(두 축 같다).
    pickGridStep(20);
    expect(gridImage().match(/transparent 1px 8px/g)).toHaveLength(2);

    // 50 단위 → 20px.
    pickGridStep(50);
    expect(gridImage().match(/transparent 1px 20px/g)).toHaveLength(2);
    expect(gridImage()).not.toContain('transparent 1px 8px');
  });

  it('간격을 바꾸면 **붙는 자리**도 함께 바뀐다 (CSS 만 바뀌고 붙임이 남으면 화면이 거짓말이다)', async () => {
    // 중심 70 + 60 = 130. 네 간격이 저마다 다른 눈금을 고른다.
    const fine = await dragOffGrid(10);
    expect((emittedGeometry(fine, 'a') as BoxGeometry).x).toBe(85); // 중심 130
    cleanup();

    const mid = await dragOffGrid(20);
    expect((emittedGeometry(mid, 'a') as BoxGeometry).x).toBe(95); // 중심 140
    cleanup();

    const base = await dragOffGrid(); // 기본 25
    expect((emittedGeometry(base, 'a') as BoxGeometry).x).toBe(80); // 중심 125
    cleanup();

    const coarse = await dragOffGrid(50);
    expect((emittedGeometry(coarse, 'a') as BoxGeometry).x).toBe(105); // 중심 150
  });

  it('붙은 중심이 **그려진 선 위**에 앉는다 (두 값이 같은 값인지 좌표로 확인한다)', async () => {
    const emit = await dragOffGrid(20);

    const moved = emittedGeometry(emit, 'a') as BoxGeometry;
    const center = moved.x + moved.w / 2;
    // 20 단위 간격의 선은 0·20·40·… 에 있다. 그 위에 앉지 않으면 그림과 붙임이 갈라진 것이다.
    expect(center % 20).toBe(0);
    // 그리고 그려진 px 가 실제로 그 정수를 투영한 값이다: 20 단위 × (200/500) = 8px.
    // 세로도 같은 축척이므로 20 × (160/400) 역시 8px 이다(칸이 정사각형이라는 뜻이다).
    const projected = 20 * (GRID_STAGE.width / CANVAS.width);
    expect(projected).toBe(20 * (GRID_STAGE.height / CANVAS.height));
    expect(projected).toBe(8);
    expect(gridImage()).toContain(`transparent 1px ${projected}px`);
  });

  it('간격을 바꿔도 상한은 여전히 무한대다 (위험 R6 은 간격마다 되살아날 수 있다)', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={emit} />);
    stubOverlayRect();
    fireEvent.click(gridToggle());
    pickGridStep(20);

    send('pointerdown', 20, 20);
    // 180px = 캔버스 450 단위 — 옛 상한(캔버스 폭의 40% = 200)의 두 배가 넘는다.
    send('pointermove', 200, 20);
    await nextFrame();

    expect((emittedGeometry(emit, 'a') as BoxGeometry).x).toBeGreaterThan(CANVAS.width * 0.4);
  });

  it('Shift+방향키의 "한 칸" 도 고른 간격을 따른다 — 그리지 않은 칸으로 뛰지 않는다', () => {
    const emit = renderPickedBox(SNAP_GEOMETRY, { x: 30, y: 20 });
    fireEvent.click(gridToggle());
    pickGridStep(20);
    emit.mockClear();

    sendKey('ArrowRight', { shiftKey: true });

    expect((emittedGeometry(emit, 'a') as BoxGeometry).x).toBe(25 + 20);
  });

  it('간격을 바꿔도 **이 층은** 캔버스 props 를 건드리지 않는다 (AC-E4)', () => {
    // 이 층이 재는 것은 "오버레이가 스스로 루프를 깨우는가" 하나다 — 간격은 저장하지 않는
    // 표시 상태이고, 이 층은 그것으로 캔버스 props 를 갈지 않는다.
    //
    // **표면과 함께 세우면 이야기가 하나 더 있다**(0.9.0): 그리는 영역이 간격의 정수배라
    // 간격을 바꾸면 영역이 다시 맞춰지고, 그림이 실제로 달라지므로 프레임이 한 장 필요하다
    // — 리사이즈와 같은 부류이며, 선택·호버·초점·격자 **토글**은 여전히 0 건이다. 그 사실은
    // §격자 선이 온전한 픽셀에 앉는다 절이 표면과 함께 세워 따로 확인한다.
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={vi.fn()} />);
    fireEvent.click(gridToggle());
    const raf = vi.spyOn(globalThis, 'requestAnimationFrame');

    pickGridStep(20);
    pickGridStep(50);

    expect(raf).not.toHaveBeenCalled();
  });
});

// --- 격자 선이 온전한 픽셀에 앉는다 (사용 시험: "격자가 일정하지 않음") -------
//
// 사용자가 같은 자리를 세 번 돌려보냈고, 세 번째의 원인은 앞의 둘과 다른 층에 있었다.
//   1회차: 두 축의 칸이 물리적으로 다른 크기였다(세로 백분율을 스테이지 종횡비에서 파생).
//   2회차: 칸 수가 정수가 아니라 마지막 줄이 반 칸으로 잘렸다(정규화 좌표의 필연).
//   3회차: 칸 수가 딱 20 칸이어도 **한 칸이 87.45px** 이라, 선이 두 장치 픽셀에 나뉘어
//          칠해져 굵기와 진하기가 선마다 달라 보였다.
//
// 그래서 이 절이 재는 것은 칸 수가 아니라 **그려진 선의 자리**다: 전부 정수인가, 간격이
// 전부 같은가. 그리고 그 선 위에 붙임과 Shift 가 실제로 떨어지는가.
//
// **이 절만 표면과 오버레이를 함께 세운다.** 위 절들의 하네스는 스테이지를 손으로 넘기지만,
// 선을 정수에 앉히는 일은 표면이 상자를 짓는 층에서 일어난다 — 오버레이만 세우면 그 층이
// 통째로 빠져 결함이 그대로 통과한다(두 층이 각자 100% 여도 이음매는 덮이지 않는다).
//
// 고정 입력은 **나누어떨어지지 않는** 1749×796 이다. 500 단위 캔버스에 25 간격이면 한 칸이
// 87.45px 이라 사용자가 본 그 값이 그대로 들어온다. 나누어떨어지는 픽스처로는 이 절이
// 실패할 수 없다.

/** 사용자가 반 칸을 본 그 크기. 두 축 모두 기본 간격으로 나누어떨어지지 않는다. */
const ODD_OUTER: StageSize = { width: 1749, height: 796 };

/**
 * 나누어떨어지는 대조군 — 1000×800 은 캔버스(500×400)와 **같은 5:4** 이고 네 간격 모두에서
 * 자투리가 0 이다(한 칸이 2×간격 px 로 떨어진다).
 *
 * 0.9.0 까지는 800×400 이었다. 축척이 하나가 된 뒤로는 비율이 다른 상자에 정의상 여백이
 * 남으므로, "자투리가 없으면 손대지 않는다" 를 물으려면 비율이 같아야 한다.
 */
const EVEN_OUTER: StageSize = { width: 1000, height: 800 };

let composedOuter: StageSize = ODD_OUTER;

/** 관찰 즉시 현재 크기를 통보하는 ResizeObserver(HeatmapCanvas.test 선례). */
class ComposedResizeObserver {
  cb: (entries: Array<{ contentRect: { width: number; height: number } }>) => void;
  constructor(cb: ComposedResizeObserver['cb']) {
    this.cb = cb;
  }
  observe() {
    this.cb([{ contentRect: { ...composedOuter } }]);
  }
  unobserve() {}
  disconnect() {}
}

/**
 * **진짜 표면 위에** 오버레이를 얹은 하네스. 이 절에서만 쓴다.
 *
 * 위 절들의 `Harness` 는 스테이지를 props 로 받는다 — 그것으로는 "표면이 그리는 영역을
 * 칸에 맞추는가" 를 잴 수 없다. 여기서는 표면이 제 `ResizeObserver` 로 재고 제 상자를
 * 짓게 두고, 오버레이는 그 상자 안에서 표면이 준 투영을 받는다(실제 배선 그대로다).
 */
function Composed({
  elements,
  onElementsChange,
}: {
  elements: readonly CanvasElement[];
  onElementsChange: (next: CanvasNode[]) => void;
}) {
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <div data-testid="parent" className="relative">
        <span data-testid="selection">{[...state.selection].join(',')}</span>
        <CanvasEditDockRegion enabled>
          <CanvasSurface
            elements={[...elements]}
            canvas={CANVAS}
            targetStyles={{}}
            texts={{}}
            overlay={({ projection, textWidths }) => (
              <CanvasEditOverlay
                enabled
                elements={elements}
                projection={projection}
                textWidths={textWidths}
                onElementsChange={onElementsChange}
              />
            )}
          />
        </CanvasEditDockRegion>
      </div>
    </CanvasEditSelectionContext>
  );
}

/** 표면이 실제로 지은 그리는 상자. */
function stageBoxSize(): { width: number; height: number } {
  const box = screen.getByTestId('canvas-stage');
  return { width: Number.parseFloat(box.style.width), height: Number.parseFloat(box.style.height) };
}

/** 그려진 격자의 두 축 주기(px)를 CSS 에서 그대로 읽는다. */
function drawnCellPx(): { x: number; y: number } {
  const image = gridImage();
  const right = /to right,.*?transparent 1px ([\d.]+)px\)/.exec(image);
  const bottom = /to bottom,.*?transparent 1px ([\d.]+)px\)/.exec(image);
  expect(right, `가로 그라디언트를 읽지 못했다: ${image}`).not.toBeNull();
  expect(bottom, `세로 그라디언트를 읽지 못했다: ${image}`).not.toBeNull();
  return { x: Number(right![1]), y: Number(bottom![1]) };
}

/** 그려진 격자의 축별 칸 수 — 그리는 영역 ÷ 주기다. */
function drawnCellCount(): { x: number; y: number } {
  const cell = drawnCellPx();
  const box = stageBoxSize();
  return { x: box.width / cell.x, y: box.height / cell.y };
}

/** 한 축에 실제로 그려질 선 자리들(0 부터 영역 끝 직전까지). */
function lineStops(extent: number, cell: number): number[] {
  const stops: number[] = [];
  for (let at = 0; at < extent - 1e-9; at += cell) stops.push(at);
  return stops;
}

/** 표면 위에 오버레이를 세우고 격자를 켠다. 포인터 축척은 1 이다(상자를 그대로 심는다). */
function renderComposed(step?: number): ReturnType<typeof vi.fn> {
  const emit = vi.fn();
  render(<Composed elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={emit} />);
  const box = stageBoxSize();
  stubOverlayRect(0, 0, box.width, box.height);
  fireEvent.click(gridToggle());
  if (step !== undefined) pickGridStep(step);
  return emit;
}

/** 지금 통보된 사각형의 중심(캔버스 단위). */
function centerUnits(emit: ReturnType<typeof vi.fn>): { x: number; y: number } {
  const g = emittedGeometry(emit, 'a') as BoxGeometry;
  return { x: g.x + g.w / 2, y: g.y + g.h / 2 };
}

describe('격자 선이 온전한 픽셀에 앉는다 (사용 시험: "격자가 일정하지 않음")', () => {
  beforeEach(() => {
    composedOuter = ODD_OUTER;
    vi.stubGlobal('ResizeObserver', ComposedResizeObserver as unknown as typeof ResizeObserver);
    // jsdom 은 2D context 를 주지 않는다. 표면은 null 을 받으면 조용히 그리지 않으며,
    // 이 절이 재는 것은 칠해진 픽셀이 아니라 **상자와 CSS** 라 그것으로 충분하다.
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('사용자가 본 그 크기(1749×796)에서 한 칸이 정수 px 다', () => {
    renderComposed();

    // 맞추기 전 '정확한' 칸은 1749/20 = 87.45px · 796/16 = 49.75px 였다. 축척이 하나이므로
    // 작은 쪽을 내림한 49 를 두 축이 함께 쓴다.
    expect(drawnCellPx()).toEqual({ x: 49, y: 49 });
    expect(stageBoxSize()).toEqual({ width: 49 * 20, height: 49 * 16 });
  });

  it('그 크기에서 그려진 칸이 **정사각형**이다 (정사각 상자로는 실패할 수 없는 시험이다)', () => {
    // 0.9.0 은 여기서 87×49 를 그렸다 — 격자도 도형도 가로로 늘어난 그림이다.
    for (const step of CANVAS_GRID_STEP_CHOICES) {
      cleanup();
      renderComposed(step);
      const cell = drawnCellPx();
      expect(cell.x, `${step} 단위`).toBe(cell.y);
    }
  });

  it('그려질 모든 선 자리가 정수이고 간격이 전부 같다 — 어느 간격을 골라도', () => {
    for (const step of CANVAS_GRID_STEP_CHOICES) {
      cleanup();
      renderComposed(step);
      const cell = drawnCellPx();
      const box = stageBoxSize();

      for (const axis of ['x', 'y'] as const) {
        const extent = axis === 'x' ? box.width : box.height;
        const stops = lineStops(extent, cell[axis]);
        expect(stops.length, `${step} 단위 ${axis} 선 개수`).toBeGreaterThan(1);
        for (const stop of stops) {
          // 소수 자리에서 시작하는 1px 선은 두 장치 픽셀에 나뉘어 칠해진다.
          expect(Number.isInteger(stop), `${step} 단위 ${axis} 선 ${stop}`).toBe(true);
        }
        const gaps = stops.slice(1).map((stop, i) => stop - stops[i]!);
        expect(new Set(gaps).size, `${step} 단위 ${axis} 간격 종류`).toBe(1);
      }
    }
  });

  it('격자를 px 로 그린다 — 백분율은 브라우저가 다시 곱해 소수를 만든다', () => {
    renderComposed();

    expect(gridImage()).not.toContain('%');
  });

  it('그려진 칸 수가 두 축 모두 정수다 — 반 칸짜리 자투리가 없다', () => {
    for (const step of CANVAS_GRID_STEP_CHOICES) {
      cleanup();
      renderComposed(step);
      const count = drawnCellCount();
      expect(Number.isInteger(count.x), `${step} 단위 가로 칸 수 ${count.x}`).toBe(true);
      expect(Number.isInteger(count.y), `${step} 단위 세로 칸 수 ${count.y}`).toBe(true);
    }
  });

  it('칸 수는 **캔버스 크기 ÷ 간격**이다 — 스테이지를 보지 않는다', () => {
    for (const step of CANVAS_GRID_STEP_CHOICES) {
      cleanup();
      renderComposed(step);
      expect(drawnCellCount()).toEqual({ x: CANVAS.width / step, y: CANVAS.height / step });
    }
  });

  it('스테이지가 달라져도 같은 칸 수를 그린다 (옛 모델에서는 달라졌다)', () => {
    renderComposed();
    const odd = drawnCellCount();
    cleanup();

    composedOuter = EVEN_OUTER;
    renderComposed();

    expect(drawnCellCount()).toEqual(odd);
    // 다만 한 칸의 **px** 는 다르다 — 같은 칸 수를 다른 크기의 상자에 그렸기 때문이다.
    expect(drawnCellPx()).toEqual({ x: 50, y: 50 });
  });

  it('나누어떨어지는 상자는 줄이지 않는다 — 자투리가 없으면 손대지 않는다', () => {
    composedOuter = EVEN_OUTER;
    renderComposed();

    expect(stageBoxSize()).toEqual({ width: 1000, height: 800 });
  });

  it('모든 칸이 같은 크기다 — 마지막 칸도 온전하다', () => {
    for (const step of CANVAS_GRID_STEP_CHOICES) {
      cleanup();
      renderComposed(step);
      const cell = drawnCellPx();
      const count = drawnCellCount();
      const box = stageBoxSize();
      // 축 길이가 칸 크기의 정수배다 = 마지막 칸이 잘리지 않는다.
      expect(cell.x * count.x).toBeCloseTo(box.width, 6);
      expect(cell.y * count.y).toBeCloseTo(box.height, 6);
    }
  });

  it('붙는 자리가 **그려진 선** 위다 — 간격을 바꿔도 그렇다', async () => {
    // 그림과 붙임이 갈라지면 "붙긴 붙는데 선하고 안 맞는다" 가 된다. 간격을 여러 개
    // 지나는 것이 중요하다 — 한 값에서만 맞는 파생은 파생이 아니라 우연이다.
    for (const step of CANVAS_GRID_STEP_CHOICES) {
      cleanup();
      const emit = renderComposed(step);
      const cell = drawnCellPx();
      const box = stageBoxSize();

      // 상자 안을 잡아 대각선으로 끈다 — 두 축이 함께 걸린다.
      send('pointerdown', 120, 120);
      send('pointermove', 260, 220);
      await nextFrame();

      const center = centerUnits(emit);
      expect(center.x % step, `${step} 단위 가로`).toBe(0);
      expect(center.y % step, `${step} 단위 세로`).toBe(0);
      // 그리고 그 자리를 화면으로 투영하면 **그려진 선 자리와 같은 px** 다. 캔버스 단위
      // 에서만 확인하면 화면에서 어긋나는 결함이 그대로 통과한다.
      const pxX = (center.x / CANVAS.width) * box.width;
      const pxY = (center.y / CANVAS.height) * box.height;
      expect(pxX, `${step} 단위 가로 px`).toBeCloseTo(Math.round(pxX / cell.x) * cell.x, 9);
      expect(pxY, `${step} 단위 세로 px`).toBeCloseTo(Math.round(pxY / cell.y) * cell.y, 9);
      expect(Number.isInteger(Math.round(pxX)), `${step} 단위 가로 정수`).toBe(true);
    }
  });

  it('간격을 바꾸면 그리는 영역이 **새 칸에 다시 맞춰진다** (선을 정수에 앉히려면 그래야 한다)', () => {
    // 이것이 "간격은 표시 상태일 뿐" 에 0.9.0 이 더한 단서다. 영역이 간격의 정수배여야
    // 선이 정수에 앉으므로, 간격을 바꾸면 영역도 바뀐다 — 리사이즈와 같은 부류의 변화이며
    // 그림이 실제로 달라지는 유일한 격자 조작이다(토글은 여전히 아무것도 바꾸지 않는다).
    renderComposed();
    expect(stageBoxSize()).toEqual({ width: 980, height: 784 }); // 49×20 · 49×16

    pickGridStep(10);

    // 정확한 칸은 1749 ÷ 50칸 = 34.98 · 796 ÷ 40칸 = 19.9 이며, 작은 쪽을 내림한 19 를
    // 두 축이 함께 쓴다.
    expect(stageBoxSize()).toEqual({ width: 19 * 50, height: 19 * 40 });
    expect(drawnCellPx()).toEqual({ x: 19, y: 19 });
  });

  it('한 칸이 1px 도 안 되면 격자를 아예 그리지 않는다 (선이 아니라 꽉 찬 사각형이 된다)', () => {
    // 10px 짜리 상자에 20 칸이면 한 칸이 0.5px 이다. 1px 선에 0.5px 주기는 격자가 아니라
    // 통짜 사각형이며, 참조선이라고 내놓을 수 없는 그림이다. 그림은 그대로 남는다.
    composedOuter = { width: 10, height: 8 };
    renderComposed();

    expect(screen.queryByTestId('panel-edit-grid')).toBeNull();
    expect(stageBoxSize()).toEqual({ width: 10, height: 8 });
  });

  it('Shift+방향키가 **그려진 칸 하나**만큼 옮긴다', () => {
    const emit = renderComposed();

    // 눌러서 고른 뒤 손을 뗀다(놓기가 "제자리 확정" 쓰기를 한 번 낸다). 하네스는 통보를
    // 되먹이지 않으므로 **매 키의 기준은 언제나 초기 기하**다.
    send('pointerdown', 120, 120);
    send('pointerup', 120, 120);
    emit.mockClear();

    sendKey('ArrowRight', { shiftKey: true });
    expect(centerUnits(emit).x - 70).toBe(CANVAS_GRID_STEP_UNITS);

    sendKey('ArrowDown', { shiftKey: true });
    expect(centerUnits(emit).y - 80).toBe(CANVAS_GRID_STEP_UNITS);
  });
});

// --- 정렬 (T13) -----------------------------------------------------------

/** 정렬 버튼 하나. */
function alignBtn(id: string): HTMLElement {
  return screen.getByTestId(`canvas-align-${id}`);
}

/** 정렬 시험용 두 사각형 — px 상자가 (10,10,40,20) 과 (100,50,20,10) 이다. */
const ALIGN_A: BoxGeometry = { x: 25, y: 40, w: 100, h: 80 };
const ALIGN_B: BoxGeometry = { x: 250, y: 200, w: 50, h: 40 };

/** 두 사각형을 함께 골라 둔 상태로 만든다. */
function renderTwoSelected(emit: ReturnType<typeof vi.fn>) {
  render(
    <Harness elements={[rect('a', ALIGN_A), rect('b', ALIGN_B)]} onElementsChange={emit} />,
  );
  stubOverlayRect();
  send('pointerdown', 30, 20);
  send('pointerdown', 110, 55, { shiftKey: true });
  expect(selectionText()).toBe('a,b');
}

describe('정렬은 2개 이상 골랐을 때만 뜻이 있다 (T13 · AC-08)', () => {
  it('여섯 방향 버튼이 모두 있고 저마다 다른 aria-label 을 가진다', () => {
    render(<Harness elements={[rect('a', ALIGN_A)]} onElementsChange={vi.fn()} />);
    const labels = ['left', 'center-x', 'right', 'top', 'center-y', 'bottom'].map((id) =>
      alignBtn(id).getAttribute('aria-label'),
    );

    expect(new Set(labels).size).toBe(6);
    expect(labels[0]).toBe('dashboard.canvas.edit.alignLeft');
    expect(labels[5]).toBe('dashboard.canvas.edit.alignBottom');
  });

  it('아무것도 고르지 않았으면 쓸 수 없다', () => {
    render(<Harness elements={[rect('a', ALIGN_A)]} onElementsChange={vi.fn()} />);
    expect((alignBtn('left') as HTMLButtonElement).disabled).toBe(true);
  });

  it('하나만 골랐으면 쓸 수 없고, 눌러도 아무 일이 없다', () => {
    const emit = vi.fn();
    render(
      <Harness elements={[rect('a', ALIGN_A), rect('b', ALIGN_B)]} onElementsChange={emit} />,
    );
    stubOverlayRect();
    send('pointerdown', 30, 20);

    expect((alignBtn('left') as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(alignBtn('left'));
    expect(emit).not.toHaveBeenCalled();
  });

  it('둘 고르면 쓸 수 있게 된다', () => {
    renderTwoSelected(vi.fn());
    expect((alignBtn('left') as HTMLButtonElement).disabled).toBe(false);
  });

  it('스테이지를 아직 재지 못했으면 아무 일도 없다 (0 으로 나눈 자리를 쓰지 않는다)', () => {
    const emit = vi.fn();
    const elements = [rect('a', ALIGN_A), rect('b', ALIGN_B)];
    const view = render(<Harness elements={elements} onElementsChange={emit} />);
    stubOverlayRect();
    send('pointerdown', 30, 20);
    send('pointerdown', 110, 55, { shiftKey: true });

    // 표면이 아직 재지 못한 순간(첫 레이아웃 전)에는 스테이지가 0×0 이다.
    view.rerender(
      <Harness elements={elements} stage={{ width: 0, height: 0 }} onElementsChange={emit} />,
    );
    fireEvent.click(alignBtn('left'));

    expect(emit).not.toHaveBeenCalled();
  });

  it('왼쪽 맞춤 — 왼쪽 변이 같아진다 (이동량 45%p 가 상한 40 에 붙잡히지 않는다)', () => {
    const emit = vi.fn();
    renderTwoSelected(emit);
    fireEvent.click(alignBtn('left'));

    expect((emittedGeometry(emit, 'a') as BoxGeometry).x).toBeCloseTo(25, 9);
    expect((emittedGeometry(emit, 'b') as BoxGeometry).x).toBeCloseTo(25, 9);
  });

  it('오른쪽 맞춤 — 오른쪽 변이 같아진다', () => {
    const emit = vi.fn();
    renderTwoSelected(emit);
    fireEvent.click(alignBtn('right'));

    const a = emittedGeometry(emit, 'a') as BoxGeometry;
    const b = emittedGeometry(emit, 'b') as BoxGeometry;
    expect(a.x + a.w).toBeCloseTo(300, 9);
    expect(b.x + b.w).toBeCloseTo(300, 9);
  });

  it('가로 가운데 맞춤 — 중심의 가로가 같아진다', () => {
    const emit = vi.fn();
    renderTwoSelected(emit);
    fireEvent.click(alignBtn('center-x'));

    const a = emittedGeometry(emit, 'a') as BoxGeometry;
    const b = emittedGeometry(emit, 'b') as BoxGeometry;
    // 바깥 상자의 가운데는 162.5 단위지만 좌표는 정수로 저장되므로 두 중심이 같은
    // **정수 눈금**(163)에 앉는다 — 맞춰졌다는 사실은 두 값이 같다는 데 있다.
    expect(a.x + a.w / 2).toBe(b.x + b.w / 2);
    expect(Math.abs(a.x + a.w / 2 - 162.5)).toBeLessThanOrEqual(0.5);
  });

  it('가로 정렬은 세로를 건드리지 않는다', () => {
    const emit = vi.fn();
    renderTwoSelected(emit);
    fireEvent.click(alignBtn('center-x'));

    expect((emittedGeometry(emit, 'a') as BoxGeometry).y).toBeCloseTo(40, 9);
    expect((emittedGeometry(emit, 'b') as BoxGeometry).y).toBeCloseTo(200, 9);
  });

  it('위 맞춤 — 위 변이 같아진다', () => {
    const emit = vi.fn();
    renderTwoSelected(emit);
    fireEvent.click(alignBtn('top'));

    expect((emittedGeometry(emit, 'a') as BoxGeometry).y).toBeCloseTo(40, 9);
    expect((emittedGeometry(emit, 'b') as BoxGeometry).y).toBeCloseTo(40, 9);
  });

  it('아래 맞춤 — 아래 변이 같아진다', () => {
    const emit = vi.fn();
    renderTwoSelected(emit);
    fireEvent.click(alignBtn('bottom'));

    const a = emittedGeometry(emit, 'a') as BoxGeometry;
    const b = emittedGeometry(emit, 'b') as BoxGeometry;
    expect(a.y + a.h).toBeCloseTo(240, 9);
    expect(b.y + b.h).toBeCloseTo(240, 9);
  });

  it('세로 가운데 맞춤 — 중심의 세로가 같아진다', () => {
    const emit = vi.fn();
    renderTwoSelected(emit);
    fireEvent.click(alignBtn('center-y'));

    const a = emittedGeometry(emit, 'a') as BoxGeometry;
    const b = emittedGeometry(emit, 'b') as BoxGeometry;
    expect(a.y + a.h / 2).toBeCloseTo(140, 9);
    expect(b.y + b.h / 2).toBeCloseTo(140, 9);
  });

  it('세로 정렬은 가로를 건드리지 않는다', () => {
    const emit = vi.fn();
    renderTwoSelected(emit);
    fireEvent.click(alignBtn('top'));

    expect((emittedGeometry(emit, 'a') as BoxGeometry).x).toBeCloseTo(25, 9);
    expect((emittedGeometry(emit, 'b') as BoxGeometry).x).toBeCloseTo(250, 9);
  });

  it('고르지 않은 요소는 제자리에 남는다', () => {
    const emit = vi.fn();
    render(
      <Harness
        elements={[rect('a', ALIGN_A), rect('b', ALIGN_B), rect('c', { x: 450, y: 360, w: 25, h: 20 })]}
        onElementsChange={emit}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 30, 20);
    send('pointerdown', 110, 55, { shiftKey: true });
    fireEvent.click(alignBtn('left'));

    expect((emittedGeometry(emit, 'c') as BoxGeometry).x).toBeCloseTo(450, 9);
  });

  it('쓰기는 기하 통로(`patchNodeGeometry`)를 지나 스타일·문구를 보존한다 (REQ-06)', () => {
    const emit = vi.fn();
    const styled: CanvasElement = {
      id: 'b',
      kind: 'rect',
      geometry: ALIGN_B,
      style: { fill: '#ff0000', visible: true },
    };
    render(<Harness elements={[rect('a', ALIGN_A), styled]} onElementsChange={emit} />);
    stubOverlayRect();
    send('pointerdown', 30, 20);
    send('pointerdown', 110, 55, { shiftKey: true });
    fireEvent.click(alignBtn('left'));

    const after = emittedElement(emit, 'b');
    expect(after?.style).toEqual({ fill: '#ff0000', visible: true });
    expect(after?.kind).toBe('rect');
  });

  it('문구 요소는 실측 폭으로 잰 상자에 맞춰진다 (보이는 테두리와 같은 상자다)', () => {
    const emit = vi.fn();
    render(
      <Harness
        elements={[rect('a', ALIGN_A), textElement('t', 20)]}
        textWidths={{ t: 30 }}
        onElementsChange={emit}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 30, 20);
    // 문구 기준점 (0.5,0.5) → px (100,50), 상자 x 100..130 · y 40..60.
    send('pointerdown', 110, 50, { shiftKey: true });
    expect(selectionText()).toBe('a,t');

    fireEvent.click(alignBtn('left'));

    // 바깥 상자의 왼쪽 변은 a 의 10px 이므로 문구는 90px = 0.45 만큼 왼쪽으로 간다.
    const moved = emittedGeometry(emit, 't') as PointGeometry;
    expect(moved.x).toBeCloseTo(25, 9);
  });
});

// --- z-order (T14) --------------------------------------------------------

/** 앞/뒤로 보내기 버튼. */
function orderBtn(dir: 'front' | 'back'): HTMLButtonElement {
  return screen.getByTestId(`canvas-order-${dir}`) as HTMLButtonElement;
}

/** 겹치지 않는 세 사각형 — 하나씩 눌러 고를 수 있다. */
const ORDER_ELEMENTS: CanvasElement[] = [
  rect('a', { x: 25, y: 40, w: 50, h: 80 }),
  rect('b', { x: 150, y: 40, w: 50, h: 80 }),
  rect('c', { x: 300, y: 40, w: 50, h: 80 }),
];

/** 마지막으로 통보된 배열의 그리기 순서(= 배열 순서 = 001 의 유일한 z-order). */
function emittedOrder(spy: ReturnType<typeof vi.fn>): string[] {
  const last = spy.mock.calls.at(-1)?.[0] as CanvasElement[] | undefined;
  return (last ?? []).map((el) => el.id);
}

describe('앞/뒤로 보내기는 목록 편집기와 같은 배열 순서 이동이다 (T14 · REQ-04)', () => {
  function renderOrder(emit: ReturnType<typeof vi.fn>) {
    render(<Harness elements={ORDER_ELEMENTS} onElementsChange={emit} />);
    stubOverlayRect();
  }

  it('버튼 둘이 있고 라벨을 가진다', () => {
    renderOrder(vi.fn());
    expect(orderBtn('front').getAttribute('aria-label')).toBe('dashboard.canvas.edit.bringToFront');
    expect(orderBtn('back').getAttribute('aria-label')).toBe('dashboard.canvas.edit.sendToBack');
  });

  it('아무것도 고르지 않았으면 쓸 수 없고, 눌러도 아무 일이 없다', () => {
    const emit = vi.fn();
    renderOrder(emit);

    expect(orderBtn('front').disabled).toBe(true);
    expect(orderBtn('back').disabled).toBe(true);
    fireEvent.click(orderBtn('front'));
    expect(emit).not.toHaveBeenCalled();
  });

  it('하나만 골라도 쓸 수 있다 — 순서 이동은 상대가 필요 없다', () => {
    renderOrder(vi.fn());
    send('pointerdown', 20, 20);
    expect(orderBtn('front').disabled).toBe(false);
  });

  it('맨 앞으로 보내면 **배열 끝**으로 간다 (뒤가 위다)', () => {
    const emit = vi.fn();
    renderOrder(emit);
    send('pointerdown', 20, 20);
    fireEvent.click(orderBtn('front'));

    expect(emittedOrder(emit)).toEqual(['b', 'c', 'a']);
  });

  it('맨 뒤로 보내면 **배열 앞**으로 간다', () => {
    const emit = vi.fn();
    renderOrder(emit);
    send('pointerdown', 130, 20);
    fireEvent.click(orderBtn('back'));

    expect(emittedOrder(emit)).toEqual(['c', 'a', 'b']);
  });

  it('이미 그 자리에 있으면 쓰지 않는다 — 헛된 쓰기는 곧 헛된 프레임이다', () => {
    const emit = vi.fn();
    renderOrder(emit);
    send('pointerdown', 130, 20);
    fireEvent.click(orderBtn('front'));

    expect(emit).not.toHaveBeenCalled();
  });

  it('무리를 함께 보내도 자기들끼리 뒤섞이지 않는다', () => {
    const emit = vi.fn();
    renderOrder(emit);
    send('pointerdown', 20, 20);
    send('pointerdown', 70, 20, { shiftKey: true });
    expect(selectionText()).toBe('a,b');

    fireEvent.click(orderBtn('front'));
    expect(emittedOrder(emit)).toEqual(['c', 'a', 'b']);
  });

  it('무리를 맨 뒤로 보내도 순서가 뒤집히지 않는다', () => {
    const emit = vi.fn();
    renderOrder(emit);
    send('pointerdown', 70, 20);
    send('pointerdown', 130, 20, { shiftKey: true });

    fireEvent.click(orderBtn('back'));
    expect(emittedOrder(emit)).toEqual(['b', 'c', 'a']);
  });

  it('순서를 바꿔도 기하·스타일은 그대로다 (자리를 옮기는 것이지 고치는 것이 아니다)', () => {
    const emit = vi.fn();
    renderOrder(emit);
    send('pointerdown', 20, 20);
    fireEvent.click(orderBtn('front'));

    expect(emittedElement(emit, 'a')).toEqual(ORDER_ELEMENTS[0]);
  });

  it('선택은 그대로 남는다 — 앞으로 꺼낸 것을 이어서 끌 수 있다', () => {
    renderOrder(vi.fn());
    send('pointerdown', 20, 20);
    fireEvent.click(orderBtn('front'));

    expect(selectionText()).toBe('a');
  });
});

// --- 신규 라벨의 i18n (REQ-01) --------------------------------------------

describe('T12~T14 가 더한 라벨 키가 ko 와 en 양쪽에 있다 (REQ-01)', () => {
  it('격자·정렬·순서 라벨이 두 언어에 모두 있고 키 이름에 점이 없다', () => {
    const names = [
      'gridSnap',
      'alignLeft',
      'alignCenterX',
      'alignRight',
      'alignTop',
      'alignCenterY',
      'alignBottom',
      'bringToFront',
      'sendToBack',
    ];
    // 위 핸들 라벨 시험이 쓴 조회와 같은 형태다 — `t()` 가 점으로 쪼개 내려가는 그 경로다.
    const resolve = (tree: unknown, key: string): unknown =>
      key
        .split('.')
        .reduce<unknown>(
          (node, seg) =>
            node && typeof node === 'object' ? (node as Record<string, unknown>)[seg] : undefined,
          tree,
        );

    for (const name of names) {
      // 이름 안에 점이 들면 `t()` 의 어떤 조회 경로로도 닿지 않는다(프로젝트 규약).
      expect(name).not.toContain('.');
      const key = `dashboard.canvas.edit.${name}`;
      expect(typeof resolve(koMessages, key)).toBe('string');
      expect(typeof resolve(enMessages, key)).toBe('string');
    }
  });
});

// =========================================================================
// 접근성 — 드래그가 유일한 수단이 아니다 (SPEC-CANVAS-002 T15)
// =========================================================================
//
// 덮는 인수 기준: AC-08(방향키 미세 이동 · Shift+방향키 한 격자 칸 · 핸들은 초점을 받고
// `aria-label` 을 가진다), AC-E5(방향키도 clamp 하지 않는다), REQ-01·REQ-05(수치 입력은
// 그대로 남는다 — 그쪽 절반은 `CanvasElementsEditor.test.tsx` 가 진다).
//
// **위험 R10 이 이 블록의 존재 이유다.** 끌 수 없는 사용자가 캔버스를 저술하지 못하면
// 002 는 001 보다 나빠진 것이다. 그래서 여기서는 "옮겨졌는가" 만 보지 않고 **포인터를 전혀
// 쓰지 않는 경로가 실제로 끝까지 이어지는가**(팔레트 → 선택 → 방향키)를 함께 본다.

/** 오버레이 루트에 키를 보낸다. **false 를 돌려주면 그 이벤트를 소비했다는 뜻이다.** */
function sendKey(key: string, init: KeyboardEventInit = {}): boolean {
  return fireEvent.keyDown(screen.getByTestId('canvas-edit-overlay'), { key, ...init });
}

/** 오버레이 루트. */
function overlayRoot(): HTMLElement {
  return screen.getByTestId('canvas-edit-overlay');
}

/**
 * 사각형 하나를 골라 두고 **손까지 뗀** 상태 — 키보드 시험의 출발점이다.
 *
 * 놓기까지 마치는 것이 핵심이다. 누른 채로는 몸통 드래그가 진행 중이고, 그동안 방향키는
 * 손에게 양보한다(둘이 서로를 덮어쓰면 튄다). 실제 클릭도 down 다음에 up 이 온다.
 * 놓기 자체가 "제자리 확정" 쓰기를 한 번 내므로 세기 전에 장부를 비운다.
 */
function renderPickedBox(
  geometry: BoxGeometry = BOX_GEOMETRY,
  at: { x: number; y: number } = { x: 80, y: 40 },
): ReturnType<typeof vi.fn> {
  const emit = vi.fn();
  render(<Harness elements={[rect('a', geometry)]} onElementsChange={emit} />);
  stubOverlayRect();
  send('pointerdown', at.x, at.y);
  send('pointerup', at.x, at.y);
  emit.mockClear();
  return emit;
}

describe('키보드로 닿는다 — 루트가 초점을 받는 자리다 (AC-08)', () => {
  it('루트는 탭으로 닿고 단축키를 스스로 알린다', () => {
    render(<Harness elements={[rect('a', BOX_GEOMETRY)]} onElementsChange={vi.fn()} />);
    const root = overlayRoot();

    expect(root.getAttribute('tabindex')).toBe('0');
    // 키 이름은 W3C 가 정한 값이라 번역하지 않는다 — 번역하면 보조기기가 못 알아듣는다.
    expect(root.getAttribute('aria-keyshortcuts')).toContain('ArrowUp');
    expect(root.getAttribute('aria-keyshortcuts')).toContain('Shift+ArrowUp');
  });

  it('설명문이 DOM 에 항상 있고 aria-describedby 가 그것을 가리킨다', () => {
    render(<Harness elements={[rect('a', BOX_GEOMETRY)]} onElementsChange={vi.fn()} />);
    const id = overlayRoot().getAttribute('aria-describedby');

    expect(id).toBeTruthy();
    const hint = document.getElementById(id!);
    expect(hint).toBeTruthy();
    // **여전히 정확히 일치로 잰다**(`toContain` 으로 늦추지 않는다 — 그러면 문단에 엉뚱한
    // 문구가 하나 더 끼어도 통과한다). 009 가 영역 선택 안내를 같은 문단에 이어 붙였고
    // 010 이 지우기 안내를 그 뒤에 이었으므로 기대값이 세 키가 된다: 사각형은 `aria-hidden`
    // 인 장식이고 지우기는 눈에 보이는 컨트롤이 아예 없으므로, 이 문단이 보조기기에게 두
    // 몸짓을 알리는 **유일한 통로**다(006 M11 이 흐림·경계를 두고 세운 논리 그대로다).
    // **네 키가 되었다**(사용자 신고 2026-09-16 — 보기 팬). 팬도 앞의 둘과 같은 부류다:
    // 눈으로 보는 사람에게는 커서가 손 모양으로 바뀌어 알리지만 **커서는 보조기기에게
    // 아무 말도 하지 않고**, 팬에는 눌러 볼 컨트롤이 아예 없다(짚는 키 하나가 전부다).
    // 그래서 이 문단이 그 몸짓을 알리는 유일한 통로이며, 006 M11 이 흐림·경계를 두고
    // 세운 그 논리가 여기서 세 번째로 그대로 성립한다.
    //
    // **011 은 키를 늘리지 않고 두 키의 *내용*을 갈아 끼웠다**(빈 자리 몸짓 뒤집기).
    // 맨손 끌기가 팬으로, 사각형이 Ctrl/Cmd 로 옮겨 앉았고 그 사실은 `marqueeHint` 와
    // `panHint` 가 각각 제 몫만큼 말한다 — 몸짓이 하나 더 생긴 것이 아니라 이미 있던 두
    // 몸짓의 배정이 바뀐 것이므로, 다섯째 키를 세우면 같은 사실이 두 자리에서 두 번
    // 말해진다. 그래서 이 기대값은 **네 키 그대로**이고, 바뀐 문장 자체는 JSON 을 직접
    // 읽는 `CanvasEditOverlay.marquee.test.tsx` §i18n 이 낱말로 못박는다(이 파일의 i18n
    // 대체는 키를 그대로 돌려주므로 여기서는 문장을 잴 수 없다).
    expect(hint!.textContent).toBe(
      'dashboard.canvas.edit.keyboardHint dashboard.canvas.edit.marqueeHint ' +
        'dashboard.canvas.edit.deleteHint dashboard.canvas.edit.panHint',
    );
    // 눈에는 보이지 않아야 한다 — 스테이지 위에 안내문이 떠 있으면 그림을 가린다.
    expect(hint!.className).toContain('sr-only');
  });

  it('패널이 둘이면 설명문 id 도 둘이다 (겹치면 남의 설명을 가리킨다)', () => {
    render(
      <>
        <Harness elements={[rect('a', BOX_GEOMETRY)]} onElementsChange={vi.fn()} />
        <Harness elements={[rect('a', BOX_GEOMETRY)]} onElementsChange={vi.fn()} />
      </>,
    );
    const ids = screen
      .getAllByTestId('canvas-edit-overlay')
      .map((node) => node.getAttribute('aria-describedby'));

    expect(ids).toHaveLength(2);
    expect(ids[0]).not.toBe(ids[1]);
  });

  it('눌러서 고르면 루트로 초점이 옮겨 온다 — preventDefault 가 기본 초점 이동을 막기 때문이다', () => {
    render(<Harness elements={[rect('a', BOX_GEOMETRY)]} onElementsChange={vi.fn()} />);
    stubOverlayRect();

    send('pointerdown', 80, 40);

    expect(document.activeElement).toBe(overlayRoot());
  });

  it('빈 지점의 주 버튼 누름은 **초점을 든다** (그 몸짓이 우리 것이 되었다)', () => {
    // SPEC-CANVAS-010 이전에는 반대였다("우리 조작이 아니므로 빼앗지 않는다"). 규칙은
    // 바뀌지 않았다 — "우리 몸짓이면 `preventDefault` 가 막은 초점을 손으로 옮긴다" 그대로이고,
    // 바뀐 것은 무엇이 우리 몸짓인가다. 값도 있다: 빈 자리를 한 번 누르면 선택이 비고 초점이
    // 이 탭 정거장에 앉으므로, 가운데 버튼이 없는 손도 곧바로 방향키로 화면을 옮길 수 있다.
    render(<Harness elements={[rect('a', BOX_GEOMETRY)]} onElementsChange={vi.fn()} />);
    stubOverlayRect();

    send('pointerdown', 5, 5);

    expect(document.activeElement).toBe(overlayRoot());
  });

  it('주 버튼이 아닌 빈 지점 누름은 **여전히** 초점을 빼앗지 않는다 — 우리 조작이 아니다', () => {
    // 위 시험이 옮겨 온 절반이다. 이 갈래가 없으면 "우리 것이 아니면 건드리지 않는다" 는
    // 규칙이 이 파일에서 더 이상 재어지지 않는다.
    render(<Harness elements={[rect('a', BOX_GEOMETRY)]} onElementsChange={vi.fn()} />);
    stubOverlayRect();

    send('pointerdown', 5, 5, { button: 2 });

    expect(document.activeElement).not.toBe(overlayRoot());
  });
});

describe('방향키가 고른 것을 옮긴다 (AC-08)', () => {
  it('한 번에 **1 캔버스 단위**씩 옮긴다 — 어느 패널에서 눌러도 같은 값이다', () => {
    // 정수 좌표계에서 1 단위는 저술할 수 있는 가장 작은 차이다. 종전에는 1 CSS px 을
    // 분수로 환산해 더했으므로 패널 크기에 따라 저장되는 값이 달라졌다.
    const emit = renderPickedBox();

    expect(sendKey('ArrowRight')).toBe(false); // 소비했다
    expectBox(emittedGeometry(emit, 'a'), { x: 101, y: 80, w: 200, h: 160 });

    sendKey('ArrowDown');
    expectBox(emittedGeometry(emit, 'a'), { x: 100, y: 81, w: 200, h: 160 });
  });

  it('왼쪽·위는 음의 방향이다 (축을 뒤바꾸지 않는다)', () => {
    const emit = renderPickedBox();

    sendKey('ArrowLeft');
    expectBox(emittedGeometry(emit, 'a'), { x: 99, y: 80, w: 200, h: 160 });

    sendKey('ArrowUp');
    expectBox(emittedGeometry(emit, 'a'), { x: 100, y: 79, w: 200, h: 160 });
  });

  it('Shift 는 한 격자 칸을 옮긴다 — 두 축이 **같은 정수**를 쓴다', () => {
    // 격자 간격이 캔버스 단위이므로 환산이 없다. 종전에는 축마다 백분율이 갈려 두 수를
    // 따로 파생해야 했고, 그 파생이 곧 그림과 어긋날 수 있는 자리였다.
    const emit = renderPickedBox();

    sendKey('ArrowRight', { shiftKey: true });
    expectBox(emittedGeometry(emit, 'a'), {
      x: 100 + CANVAS_GRID_STEP_UNITS,
      y: 80,
      w: 200,
      h: 160,
    });

    sendKey('ArrowUp', { shiftKey: true });
    expectBox(emittedGeometry(emit, 'a'), {
      x: 100,
      y: 80 - CANVAS_GRID_STEP_UNITS,
      w: 200,
      h: 160,
    });
  });

  it('크기는 건드리지 않는다 — 방향키는 이동이지 크기 조절이 아니다', () => {
    // px 상자 (20,10,60,50) — 가운데는 (50,35) 다.
    const emit = renderPickedBox({ x: 50, y: 40, w: 150, h: 200 }, { x: 50, y: 35 });

    sendKey('ArrowRight');

    const g = emittedGeometry(emit, 'a') as BoxGeometry;
    expect(g.w).toBe(150);
    expect(g.h).toBe(200);
  });

  it('둘 이상 골랐으면 같은 델타가 전부에 적용된다 (드래그와 같은 규칙)', () => {
    const emit = vi.fn();
    render(
      <Harness
        elements={[rect('a', BOX_GEOMETRY), rect('b', { x: 375, y: 20, w: 100, h: 80 })]}
        onElementsChange={emit}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 80, 40);
    send('pointerup', 80, 40);
    // 보조키는 고르기 전용 조작이라 드래그를 시작하지 않는다 — 놓기도 필요 없다.
    send('pointerdown', 170, 15, { shiftKey: true });
    expect(selectionText()).toBe('a,b');
    emit.mockClear();

    sendKey('ArrowRight', { shiftKey: true });

    const cell = CANVAS_GRID_STEP_UNITS;
    expectBox(emittedGeometry(emit, 'a'), { x: 100 + cell, y: 80, w: 200, h: 160 });
    expectBox(emittedGeometry(emit, 'b'), { x: 375 + cell, y: 20, w: 100, h: 80 });
  });

  it('고르지 않은 요소는 제자리에 남는다', () => {
    const emit = vi.fn();
    render(
      <Harness
        elements={[rect('a', BOX_GEOMETRY), rect('b', { x: 375, y: 20, w: 100, h: 80 })]}
        onElementsChange={emit}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 80, 40);
    send('pointerup', 80, 40);
    emit.mockClear();

    sendKey('ArrowRight');

    expectBox(emittedGeometry(emit, 'b'), { x: 375, y: 20, w: 100, h: 80 });
  });

  it('선도 같은 통로로 옮긴다 — 두 끝점이 함께 간다 (종류별 이동 규칙을 만들지 않는다)', () => {
    const emit = vi.fn();
    render(<Harness elements={[LINE_ELEMENT]} onElementsChange={emit} />);
    stubOverlayRect();
    // 선 몸통의 중점 부근을 눌러 고른다.
    send('pointerdown', 60, 40);
    send('pointerup', 60, 40);
    expect(selectionText()).toBe('l');
    emit.mockClear();

    sendKey('ArrowRight', { shiftKey: true });

    const cell = CANVAS_GRID_STEP_UNITS;
    const g = emittedGeometry(emit, 'l') as LineGeometry;
    expect(g.x1).toBe(50 + cell);
    expect(g.x2).toBe(250 + cell);
    expect(g.y1).toBe(80);
    expect(g.y2).toBe(240);
  });

  it('문구도 자기 기준점만 옮긴다 (기하가 점 하나뿐인 종류)', () => {
    const emit = vi.fn();
    render(
      <Harness elements={[textElement('t', 20)]} textWidths={{ t: 30 }} onElementsChange={emit} />,
    );
    stubOverlayRect();
    // 기준점 (100,50) · 폭 30 · 글자 20 → 상자는 (100,40,30,20) 이고 가운데는 (115,50) 이다.
    send('pointerdown', 115, 50);
    send('pointerup', 115, 50);
    expect(selectionText()).toBe('t');
    emit.mockClear();

    sendKey('ArrowDown', { shiftKey: true });

    const g = emittedGeometry(emit, 't') as PointGeometry;
    expect(g.x).toBe(250);
    expect(g.y).toBe(200 + CANVAS_GRID_STEP_UNITS);
  });

  it('쓰기는 기하 통로를 지나 스타일·문구를 보존한다 (REQ-06)', () => {
    const emit = vi.fn();
    const styled: CanvasElement = {
      id: 'a',
      kind: 'rect',
      geometry: BOX_GEOMETRY,
      style: { fill: '#123456', opacity: 0.5 },
      text: '{value}',
    };
    render(<Harness elements={[styled]} onElementsChange={emit} />);
    stubOverlayRect();
    send('pointerdown', 80, 40);
    send('pointerup', 80, 40);
    emit.mockClear();

    sendKey('ArrowRight');

    const el = emittedElement(emit, 'a')!;
    expect(el.style).toEqual({ fill: '#123456', opacity: 0.5 });
    expect(el.text).toBe('{value}');
    expect(el.kind).toBe('rect');
  });
});

describe('방향키도 잘라내지 않는다 (AC-E5)', () => {
  it('오른쪽 끝을 넘어가도 캔버스 폭에서 붙잡히지 않는다', () => {
    // px 상자 (192,20,80,40) — 가운데는 (232,40) 으로 스테이지 밖이지만 판정은 px 공간이다.
    const emit = renderPickedBox({ x: 480, y: 80, w: 200, h: 160 }, { x: 230, y: 40 });

    sendKey('ArrowRight', { shiftKey: true });

    const g = emittedGeometry(emit, 'a') as BoxGeometry;
    expect(g.x).toBe(480 + CANVAS_GRID_STEP_UNITS);
    expect(g.x).toBeGreaterThan(CANVAS.width);
  });

  it('왼쪽으로 나가도 0 에서 붙잡히지 않는다', () => {
    // px 상자 (4,20,80,40) — 가운데는 (44,40) 이다.
    const emit = renderPickedBox({ x: 10, y: 80, w: 200, h: 160 }, { x: 44, y: 40 });

    sendKey('ArrowLeft', { shiftKey: true });

    const g = emittedGeometry(emit, 'a') as BoxGeometry;
    expect(g.x).toBe(10 - CANVAS_GRID_STEP_UNITS);
    expect(g.x).toBeLessThan(0);
  });
});

describe('우리 것이 아닌 키는 소비하지 않는다 (AC-E3 과 같은 뜻)', () => {
  it('고른 것이 없으면 아무 일도 없고 이벤트도 그대로 흘러간다', () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', BOX_GEOMETRY)]} onElementsChange={emit} />);

    expect(sendKey('ArrowRight')).toBe(true); // 소비하지 않았다
    expect(emit).not.toHaveBeenCalled();
  });

  it('방향키가 아닌 키는 손대지 않는다 (Tab·Esc 가 살아 있어야 한다)', () => {
    const emit = renderPickedBox();

    // **Space 가 이 목록에서 빠졌다**(사용자 신고 2026-09-16 — 보기 팬). 종전에는 다섯이었고
    // 그 다섯째가 `' '` 였다 — 그때 Space 는 이 표면에서 아무 뜻도 없었기 때문이다. 팬이
    // 그것을 **구분자로** 가져갔으므로(`SPACE_KEY`) 이제 소비되며, 그 사실은 아래 형제
    // 시험이 제 이름으로 단언한다. 목록에서 **지우기만 하고 옮기지 않으면** "Space 는 이제
    // 아무도 재지 않는 키" 가 되어, 팬을 지우는 변이가 여기서도 저기서도 걸리지 않는다.
    for (const key of ['Tab', 'Escape', 'Enter', 'a']) {
      expect(sendKey(key)).toBe(true);
    }
    expect(emit).not.toHaveBeenCalled();
  });

  it('Space 는 이제 **우리 것이다** — 팬의 구분자라 페이지 스크롤을 막는다', () => {
    const emit = renderPickedBox();

    // 소비한다(= `preventDefault`). 짚는 일이 실제로 무언가를 켰기 때문이며, 켜지 않았다면
    // 이 층의 규율대로 흘려보내야 한다.
    expect(sendKey(' ')).toBe(false);
    // **고른 것은 건드리지 않는다.** 팬은 시야를 옮길 뿐이므로 요소가 움직이면 그것은
    // 방향키 갈래로 잘못 떨어진 것이다.
    expect(emit).not.toHaveBeenCalled();
  });

  it('Ctrl·Cmd·Alt 가 붙은 방향키는 브라우저·OS 의 조작이다', () => {
    const emit = renderPickedBox();

    expect(sendKey('ArrowRight', { ctrlKey: true })).toBe(true);
    expect(sendKey('ArrowRight', { metaKey: true })).toBe(true);
    expect(sendKey('ArrowRight', { altKey: true })).toBe(true);
    expect(emit).not.toHaveBeenCalled();
  });

  it('편집이 꺼져 있으면 오버레이 자체가 없으므로 키가 닿을 곳도 없다', () => {
    render(
      <Harness enabled={false} elements={[rect('a', BOX_GEOMETRY)]} onElementsChange={vi.fn()} />,
    );
    expect(screen.queryByTestId('canvas-edit-overlay')).toBeNull();
  });

  it('스테이지를 아직 재지 못했어도 미세 이동은 성립한다 (나눌 것이 없다)', () => {
    // 종전에는 1 CSS px 을 스테이지 길이로 나눠야 했으므로 재기 전에는 이동을 포기했다.
    // 이제 이동량이 캔버스 단위 정수라 나눗셈이 아예 없다 — 접힌 패널에서도 방향키가
    // 살아 있고, 이것은 잃은 성질이 아니라 되찾은 성질이다.
    const emit = vi.fn();
    render(
      <Harness
        elements={[rect('a', BOX_GEOMETRY)]}
        stage={{ width: 0, height: 0 }}
        onElementsChange={emit}
      />,
    );
    stubOverlayRect();
    // 히트 테스트는 스테이지 0 에서도 돌지만 드래그는 시작하지 않는다(0 으로 나누지 않는다).
    send('pointerdown', 0, 0);
    expect(selectionText()).toBe('a');
    emit.mockClear();

    expect(sendKey('ArrowRight')).toBe(false);
    expectBox(emittedGeometry(emit, 'a'), { x: 101, y: 80, w: 200, h: 160 });
  });

  it('한 격자 칸 이동도 마찬가지다 — 간격이 캔버스 단위라 스테이지를 보지 않는다', () => {
    const emit = vi.fn();
    render(
      <Harness
        elements={[rect('a', BOX_GEOMETRY)]}
        stage={{ width: 0, height: 0 }}
        onElementsChange={emit}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 0, 0);
    emit.mockClear();

    expect(sendKey('ArrowRight', { shiftKey: true })).toBe(false);
    expectBox(emittedGeometry(emit, 'a'), {
      x: 100 + CANVAS_GRID_STEP_UNITS,
      y: 80,
      w: 200,
      h: 160,
    });
  });

  it('골라 둔 요소가 배열에서 사라졌으면 쓰지 않는다 (헛된 쓰기는 헛된 프레임이다)', () => {
    const emit = vi.fn();
    const view = render(<Harness elements={[rect('a', BOX_GEOMETRY)]} onElementsChange={emit} />);
    stubOverlayRect();
    send('pointerdown', 80, 40);
    send('pointerup', 80, 40);
    // 목록 편집기에서 지운 상황 — 선택에는 id 가 남고 배열에는 없다.
    view.rerender(<Harness elements={[]} onElementsChange={emit} />);
    emit.mockClear();

    expect(sendKey('ArrowRight')).toBe(true);
    expect(emit).not.toHaveBeenCalled();
  });

  it('끌고 있는 동안에는 손이 이긴다 (둘이 서로를 덮어쓰면 튄다)', () => {
    const emit = renderPickedBox();
    // 이번에는 손잡이를 **잡은 채로** 방향키를 누른다.
    sendAt(handleEl('se'), 'pointerdown', 120, 60);
    emit.mockClear();

    expect(sendKey('ArrowRight')).toBe(true);
    expect(emit).not.toHaveBeenCalled();
  });
});

describe('포인터를 전혀 쓰지 않는 경로가 끝까지 이어진다 (AC-08 · 위험 R10)', () => {
  it('팔레트로 놓고 → 방향키로 옮긴다', () => {
    render(<PaletteHarness initial={[]} />);

    // 팔레트 버튼은 진짜 <button> 이라 클릭이 곧 키보드 활성화(Enter/Space)와 같은 경로다.
    place('rect');
    const created = liveElements()[0]!;
    expect(selectionText()).toBe(created.id);

    const before = created.geometry as BoxGeometry;
    sendKey('ArrowRight', { shiftKey: true });

    const after = liveElements()[0]!.geometry as BoxGeometry;
    expect(after.x).toBe(before.x + CANVAS_GRID_STEP_UNITS);
    expect(after.y).toBe(before.y);
  });

  it('핸들은 초점을 받고 라벨을 가진다 — 골라 둔 것 위에 진짜 버튼이 선다', () => {
    renderPickedBox();
    const se = handleEl('se');

    se.focus();

    expect(document.activeElement).toBe(se);
    expect(se.tagName).toBe('BUTTON');
    expect(se.getAttribute('aria-label')).toBe('dashboard.canvas.edit.handleSe');
  });

  it('핸들에 초점을 준 채 방향키를 눌러도 고른 것이 움직인다 (초점이 함정이 되지 않는다)', () => {
    const emit = renderPickedBox();
    handleEl('se').focus();

    // 핸들은 루트의 자식이므로 키가 루트까지 올라온다.
    expect(fireEvent.keyDown(handleEl('se'), { key: 'ArrowRight' })).toBe(false);
    expectBox(emittedGeometry(emit, 'a'), { x: 101, y: 80, w: 200, h: 160 });
  });
});

// =========================================================================
// 견고성 게이트 (SPEC-CANVAS-002 T16)
// =========================================================================
//
// 위 블록들이 저마다의 기능을 지켰다면, 여기서는 **그 기능들이 001 의 규율을 깨지 않았음**
// 을 한 자리에서 다시 센다. 덮는 인수 기준: AC-E3(빈 지점은 상위 조작으로 흘려보낸다 —
// 휠 확대까지), AC-E4(유휴 정지).

describe('빈 지점은 미리보기의 다른 조작을 막지 않는다 (AC-E3 · 위험 R2)', () => {
  it('휠은 언제나 상위로 흘러간다 — 미리보기의 휠 확대가 살아 있어야 한다', () => {
    const onParentWheel = vi.fn();
    render(
      <Harness
        elements={[rect('a', BOX_GEOMETRY)]}
        onElementsChange={vi.fn()}
        onParentWheel={onParentWheel}
      />,
    );
    stubOverlayRect();

    // 빈 지점 위에서도,
    fireEvent.wheel(overlayRoot(), { deltaY: -100 });
    expect(onParentWheel).toHaveBeenCalledTimes(1);

    // 요소를 골라 둔 채 그 요소 위에서도 마찬가지다 — 오버레이는 휠에 손대지 않는다.
    send('pointerdown', 80, 40);
    send('pointerup', 80, 40);
    fireEvent.wheel(overlayRoot(), { deltaY: -100 });
    expect(onParentWheel).toHaveBeenCalledTimes(2);

    // 핸들 위에서도 그대로다(핸들은 포인터만 끊는다).
    fireEvent.wheel(handleEl('se'), { deltaY: -100 });
    expect(onParentWheel).toHaveBeenCalledTimes(3);

    // 도크의 도구 위에서도 그대로다(포털이라 React 트리 전파는 종전과 같다).
    fireEvent.wheel(screen.getByTestId('canvas-dock-panel'), { deltaY: -100 });
    expect(onParentWheel).toHaveBeenCalledTimes(4);
  });

  it('골라 둔 것이 있어도 빈 지점 누름은 **선택을 비운다** (010: 주 버튼은 소비한다)', () => {
    const onParentDown = vi.fn();
    render(
      <Harness
        elements={[rect('a', BOX_GEOMETRY)]}
        onElementsChange={vi.fn()}
        onParentDown={onParentDown}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 80, 40); // 먼저 고른다
    send('pointerup', 80, 40);
    expect(selectionText()).toBe('a');
    onParentDown.mockClear();

    const evt = send('pointerdown', 5, 5); // 빈 지점

    // 010 이 바꾼 절반 — 주 버튼 누름은 우리가 가져간다(011 에서는 팬이 가져간다).
    expect(evt.defaultPrevented).toBe(true);
    expect(onParentDown).not.toHaveBeenCalled();
    // **011 이 시점을 옮겼다** — 비우는 일은 뗌의 몫이다(움직이지 않은 팬이 곧 클릭이다).
    // 누르는 순간에 비우면 화면을 옮기는 내내 선택이 사라지므로 팬이 제 일이 아닌 것을
    // 하게 된다. 뜻은 그대로이고 자리만 한 칸 뒤로 갔다.
    expect(selectionText()).toBe('a');
    send('pointerup', 5, 5);
    expect(selectionText()).toBe('');

    // **흘러가는 갈래는 그대로 있다.** 이 절이 지키는 것은 "미리보기의 다른 조작이 죽지
    // 않는다" 이고, 휠(위 시험)과 주 버튼이 아닌 누름이 그 문장을 계속 지탱한다. 캔버스
    // 안에서 주 버튼 끌기는 011 이 **팬으로 돌려주었다**(010 이 그것을 사각형에 내주었다).
    onParentDown.mockClear();
    const other = send('pointerdown', 5, 5, { button: 2 });
    expect(other.defaultPrevented).toBe(false);
    expect(onParentDown).toHaveBeenCalledTimes(1);
  });
});

describe('편집기 조작은 프레임을 예약하지 않는다 (AC-E4)', () => {
  it('격자 토글 · 팔레트 호버·초점 · 핸들 초점 · 소비하지 않는 키는 프레임 요청이 0 건이다', () => {
    render(<PaletteHarness initial={[rect('a', BOX_GEOMETRY)]} />);
    stubOverlayRect();
    send('pointerdown', 80, 40);
    send('pointerup', 80, 40);
    const raf = vi.spyOn(globalThis, 'requestAnimationFrame');

    fireEvent.click(screen.getByTestId('canvas-grid-toggle')); // 격자 켜기
    fireEvent.pointerOver(screen.getByTestId('canvas-palette-add-rect')); // 팔레트 호버
    fireEvent.mouseOver(screen.getByTestId('canvas-palette-add-rect'));
    screen.getByTestId('canvas-palette-add-rect').focus(); // 팔레트 초점
    handleEl('se').focus(); // 핸들 초점
    fireEvent.click(screen.getByTestId('canvas-grid-toggle')); // 격자 끄기
    sendKey('ArrowRight', { ctrlKey: true }); // 우리 것이 아닌 키

    expect(screen.getByTestId('canvas-selection-a')).toBeTruthy();
    expect(raf).not.toHaveBeenCalled();
  });

  it('방향키 이동은 합류 프레임을 쓰지 않는다 — 한 번 눌러 한 번 쓴다', () => {
    const emit = renderPickedBox();
    const raf = vi.spyOn(globalThis, 'requestAnimationFrame');

    sendKey('ArrowRight');

    // 루프를 깨우는 것은 `elements` 변경 하나뿐이며 그 경로는 숙주가 진다 — 오버레이가
    // 스스로 프레임을 잡지는 않는다(합류 프레임은 포인터 이동에만 쓴다).
    expect(raf).not.toHaveBeenCalled();
    expect(emit).toHaveBeenCalledTimes(1);
  });
});

// --- 축소된 미리보기 안의 포인터 (AC-E9 · 위험 R1) ------------------------
//
// **이 결함이 빠져나간 이유는 여기 있던 모든 시험이 축척 1 에서 쟀기 때문이다.**
// `stubOverlayRect()` 의 기본 크기가 `STAGE` 와 같으면 `rect.width / stage.width === 1`
// 이라 나눗셈이 항등이 되어, 변환을 아예 하지 않는 코드와 하는 코드가 구별되지 않는다.
//
// 설정 다이얼로그의 미리보기는 패널을 대시보드에서의 **실제 픽셀 크기로 렌더한 뒤 통째로
// 축소**한다(`PanelSettingsDialog` §미리보기). CSS 변환이므로 `getBoundingClientRect()` 는
// 변환 **뒤**를, 표면의 `ResizeObserver` 는 변환 **앞**을 준다. 그래서 아래 고정 입력은
// **축척을 1 이 아닌 값으로 둔다**: 스테이지 800×400 을 400×200 상자로 재게 하여 **0.5** 다.
// 이 값이 1 이면 아래 시험들은 결함을 통과시킨다.

/** 축소 미리보기 고정 입력 — 스테이지는 변환 **앞**, 상자는 변환 **뒤**의 크기다. */
const ZOOM_STAGE: StageSize = { width: 800, height: 400 };
const ZOOM_RECT = { left: 50, top: 30, width: 400, height: 200 };
/** 이 파일이 쓰는 유일한 비단위 축척. 0.5 = 400/800 = 200/400. */
const ZOOM_SCALE = 0.5;

/** 스테이지 로컬 px → 화면 px(축소 미리보기의 정방향 변환). 시험이 손을 놓을 자리를 만든다. */
function toScreen(stageX: number, stageY: number): { x: number; y: number } {
  return { x: ZOOM_RECT.left + stageX * ZOOM_SCALE, y: ZOOM_RECT.top + stageY * ZOOM_SCALE };
}

/** 스테이지 한가운데를 차지하는 사각형 — 정규화 중심이 (0.5, 0.5) 다. */
const CENTER_BOX: BoxGeometry = { x: 200, y: 160, w: 100, h: 80 };

/**
 * 축척 s 로 렌더된 오버레이. `stage` 는 표면이 잰 값(변환 앞)이고 심어 주는 상자는
 * 화면에서 잰 값(변환 뒤)이다.
 */
function renderScaled(
  elements: readonly CanvasElement[],
  emit: ReturnType<typeof vi.fn>,
  scale = ZOOM_SCALE,
) {
  render(<Harness elements={elements} stage={ZOOM_STAGE} onElementsChange={emit} />);
  stubOverlayRect(
    ZOOM_RECT.left,
    ZOOM_RECT.top,
    ZOOM_STAGE.width * scale,
    ZOOM_STAGE.height * scale,
  );
  return emit;
}

describe('축소된 미리보기 안에서도 포인터가 도형과 같은 공간에 있다 (AC-E9)', () => {
  it('축척 0.5 에서 도형의 시각적 한가운데를 누르면 그 도형이 골라진다', () => {
    renderScaled([rect('a', CENTER_BOX)], vi.fn());

    // 정규화 (0.5, 0.5) = 스테이지 (400, 200) = 화면 (250, 130).
    const at = toScreen(400, 200);
    expect(at).toEqual({ x: 250, y: 130 });
    send('pointerdown', at.x, at.y);

    expect(selectionText()).toBe('a');
  });

  it('축척을 무시했다면 맞았을 자리는 빗나간다 (결함의 거울상)', () => {
    renderScaled([rect('a', CENTER_BOX)], vi.fn());

    // 화면 (400, 200) 을 누른다.
    //   - 옳은 읽기: (400-50)/0.5 = 700, (200-30)/0.5 = 340 → 정규화 (0.875, 0.85).
    //     상자(0.4..0.6) 밖이므로 아무것도 골라지지 않는다.
    //   - 결함 당시의 읽기: 원점만 빼 (350, 170) → 정규화 (0.4375, 0.425). 상자 **안**이다.
    // 즉 이 한 줄이 결함의 유무를 정확히 가른다.
    send('pointerdown', 400, 200);

    expect(selectionText()).toBe('');
  });

  it('축척 0.5 에서 화면 N px 를 끌면 요소는 스테이지 N/0.5 px 만큼 움직인다', async () => {
    const emit = renderScaled([rect('a', CENTER_BOX)], vi.fn());

    const from = toScreen(400, 200);
    // 화면에서 (40, 20) px 를 끈다 → 스테이지 (80, 40) px → 정규화 (0.1, 0.1).
    send('pointerdown', from.x, from.y);
    send('pointermove', from.x + 40, from.y + 20);
    await nextFrame();

    expectBox(emittedGeometry(emit, 'a'), { x: 250, y: 200, w: 100, h: 80 });
  });

  it('축척 1 에서는 같은 화면 이동량이 그대로 스테이지 이동량이다 (되돌림 방어)', async () => {
    const emit = renderScaled([rect('a', CENTER_BOX)], vi.fn(), 1);

    // 축척 1 이므로 스테이지 px = 화면 px - 원점이다.
    const from = { x: ZOOM_RECT.left + 400, y: ZOOM_RECT.top + 200 };
    send('pointerdown', from.x, from.y);
    send('pointermove', from.x + 40, from.y + 20);
    await nextFrame();

    // 화면 (40, 20) = 스테이지 (40, 20) → 정규화 (0.05, 0.05).
    expectBox(emittedGeometry(emit, 'a'), { x: 225, y: 180, w: 100, h: 80 });
  });

  it('놓는 순간의 좌표도 같은 공간으로 옮긴다 (pointerup 이 마지막 자리를 확정한다)', async () => {
    const emit = renderScaled([rect('a', CENTER_BOX)], vi.fn());

    const from = toScreen(400, 200);
    send('pointerdown', from.x, from.y);
    // 합류 프레임을 기다리지 않고 바로 뗀다 — 확정 경로가 같은 변환을 쓰는지 본다.
    send('pointerup', from.x + 40, from.y + 20);

    expectBox(emittedGeometry(emit, 'a'), { x: 250, y: 200, w: 100, h: 80 });
  });

  it('핸들 드래그도 같은 공간을 쓴다 — 손잡이가 커서 아래에 남는다', async () => {
    const emit = renderScaled([rect('a', CENTER_BOX)], vi.fn());
    // 먼저 골라야 핸들이 뜬다.
    const center = toScreen(400, 200);
    send('pointerdown', center.x, center.y);
    send('pointerup', center.x, center.y);

    // 우하단 핸들의 CSS 자리는 **스테이지 px** 이므로, 손은 그것을 화면으로 옮겨 잡는다.
    const se = handleEl('se');
    const grip = toScreen(Number.parseFloat(se.style.left), Number.parseFloat(se.style.top));
    sendAt(se, 'pointerdown', grip.x, grip.y);
    // 스테이지 (600, 300) = 정규화 (0.75, 0.75) 로 끈다.
    const to = toScreen(600, 300);
    send('pointermove', to.x, to.y);
    await nextFrame();

    expectBox(emittedGeometry(emit, 'a'), { x: 200, y: 160, w: 175, h: 140 });
  });

  it('스테이지를 아직 재지 못했으면 축척 1 로 떨어진다 (NaN·Infinity 를 흘리지 않는다)', () => {
    const emit = vi.fn();
    // 스테이지 0×0 + 화면 상자 0×0 — 두 몫이 모두 0/0 이다.
    render(
      <Harness elements={[rect('a', CENTER_BOX)]} stage={{ width: 0, height: 0 }} onElementsChange={emit} />,
    );
    stubOverlayRect(0, 0, 0, 0);

    // 판정 자체가 NaN 으로 조용히 무너지지 않는지만 본다 — 크기가 0 이면 드래그는
    // 어차피 시작되지 않는다(기존 0 나눗셈 가드).
    expect(() => send('pointerdown', 10, 10)).not.toThrow();
    expect(emit).not.toHaveBeenCalled();
  });

  it('상자 크기가 비유한이면 축척 1 로 떨어져 판정이 종전과 같아진다', () => {
    render(<Harness elements={[rect('a', { x: 50, y: 40, w: 100, h: 80 })]} onElementsChange={vi.fn()} />);
    // 잰 적 없는 상자(분리된 노드 등)가 NaN 을 주는 자리다. 축척이 NaN 이 되면 모든 좌표가
    // NaN 이라 아무것도 고를 수 없게 되므로, 1 로 떨어져 원점만 뺀 값이 남아야 한다.
    stubOverlayRect(0, 0, Number.NaN, Number.NaN);

    // STAGE 200×100 위의 상자 (20,10,40,20) 안 — 축척 1 로 읽어야 맞는 자리다.
    send('pointerdown', 30, 15);

    expect(selectionText()).toBe('a');
  });
});

// --- 한국어 어휘 가드 ------------------------------------------------------
//
// 화면 어휘는 코드 어휘가 아니다. 캔버스의 한국어 문자열에는 한때 구현 용어가 그대로
// 새어 나와 있었다 — 텍스트 요소를 "문구" 라 불렀고, 상태 전이 보간을 "트윈"·"이징"
// 이라 불렀다. 사용자는 그 셋을 알아보지 못했다.
//
// 되돌아오는 길은 **키를 새로 만들 때**다: 이웃한 문자열을 베껴 쓰면 그 어휘도 함께
// 온다. 그래서 사람이 아니라 이 가드가 막는다. 막는 것은 **한국어 표시 문자열**뿐이며,
// i18n 키 이름(영어)·`kind: 'text'` 같은 config 값·코드 식별자는 그대로 둔다.

/** 캔버스 한국어 문자열을 (경로, 값) 쌍으로 모두 펼친다. */
function flattenKoStrings(node: unknown, prefix: string): [string, string][] {
  if (typeof node === 'string') return [[prefix, node]];
  if (node === null || typeof node !== 'object') return [];
  return Object.entries(node as Record<string, unknown>).flatMap(([k, v]) =>
    flattenKoStrings(v, prefix === '' ? k : `${prefix}.${k}`),
  );
}

describe('캔버스 한국어 어휘에 구현 용어가 남아 있지 않다', () => {
  /** 사용자가 알아보지 못한 세 낱말. */
  const BANNED = ['문구', '트윈', '이징'];

  it('dashboard.canvas 아래 한국어 문자열에 문구·트윈·이징이 없다', () => {
    const strings = flattenKoStrings(
      (koMessages as Record<string, Record<string, unknown>>)['dashboard']?.['canvas'],
      'dashboard.canvas',
    );
    // 펼치기 자체가 비면 가드가 아무것도 보지 않고 통과한다 — 그 침묵을 먼저 막는다.
    expect(strings.length).toBeGreaterThan(50);

    const offenders = strings.filter(([, value]) => BANNED.some((word) => value.includes(word)));
    expect(offenders).toEqual([]);
  });

  it('패널 종류 설명(캔버스)에도 남아 있지 않다 — 같은 기능을 가리키는 다른 자리다', () => {
    const strings = flattenKoStrings(koMessages, '').filter(
      ([key]) => key.endsWith('.canvas') && key.includes('escription'),
    );
    expect(strings.length).toBeGreaterThan(0);

    const offenders = strings.filter(([, value]) => BANNED.some((word) => value.includes(word)));
    expect(offenders).toEqual([]);
  });

  it('바꿔 넣은 낱말이 실제로 화면 문자열에 있다 (지우기만 하고 끝내지 않았다)', () => {
    const canvas = (koMessages as Record<string, Record<string, unknown>>)['dashboard']?.['canvas'];
    const strings = flattenKoStrings(canvas, 'dashboard.canvas');
    const has = (word: string) => strings.some(([, value]) => value.includes(word));

    expect(has('텍스트')).toBe(true);
    expect(has('전환')).toBe(true);
  });

  it('새 키도 ko·en 양쪽에 있다 (키 짝이 맞는다)', () => {
    const keys = [
      'dashboard.canvas.edit.gridStep',
      'dashboard.canvas.elements.tweenHint',
      'dashboard.canvas.elements.panelTweenHint',
    ];
    const resolve = (tree: unknown, key: string): unknown =>
      key
        .split('.')
        .reduce<unknown>(
          (node, seg) =>
            node && typeof node === 'object' ? (node as Record<string, unknown>)[seg] : undefined,
          tree,
        );

    for (const key of keys) {
      expect(typeof resolve(koMessages, key)).toBe('string');
      expect(typeof resolve(enMessages, key)).toBe('string');
    }
  });
});

// --- 출력 영역의 표시 (SPEC-CANVAS-006 M6) ---------------------------------
//
// 여기서 재는 것 셋: **작업 영역 전체에 펴지되 출력 영역의 원점에 앉은 격자**, 출력 영역
// **밖**을 덮는 흐림, 출력 영역의 **경계**. 셋 다 DOM 이고 셋 다 포인터를 먹지 않는다.
//
// **이 절은 표면과 진짜 오버레이를 함께 세운다.** 원점을 짓는 것은 표면이고 그것을 쓰는
// 것은 오버레이라, 어느 한쪽만 세우면 이음매가 통째로 빠져 결함이 그대로 통과한다
// (0.4.0 결함 E 가 세운 규율). 그리고 `workspace` 를 **켠** 시험과 **끈** 시험을 둘 다
// 둔다 — 켜지 않으면 006 의 코드가 한 줄도 실행되지 않고(D1), 끈 갈래가 없으면 오늘과
// 같음을 아무도 지키지 않는다.
//
// 고정 입력 1749×796 × 5:4 캔버스에서 표면이 짓는 값은 `canvasWorkspace.test.ts` 가 이미
// 못 박은 그대로다: 칸 37 · 출력 영역 740×592 · 원점 (504, 102).
//
//   **504 % 37 = 23 · 102 % 37 = 28 — 원점은 칸의 배수가 아니다**(D3).
//
// 이 사실이 이 절의 전부다. 원점이 칸의 배수인 고정 입력에서는 격자를 작업 영역의 왼쪽
// 위에 앉히나 출력 영역의 원점에 앉히나 같은 자리가 되어, 위험 R3 의 결함이 **실패할 수
// 없다**.

/** 표면이 짓는 값들 — 위 머리말의 그 수다. */
const WS_OUTER: StageSize = { width: 1749, height: 796 };
const WS_CELL = 37;
const WS_STAGE = { width: 740, height: 592 };
const WS_ORIGIN = { x: 504, y: 102 };

/** 표면 위에 오버레이를 얹되 **작업 영역을 켠** 하네스. */
function WorkspaceComposed({
  elements,
  onElementsChange,
  workspace = true,
  docked = true,
  canvas = CANVAS,
  scheduler,
}: {
  elements: readonly CanvasElement[];
  onElementsChange: (next: CanvasNode[]) => void;
  workspace?: boolean;
  /**
   * 도크 자리를 펴는가(SPEC-CANVAS-006 M10 · 시험 규율 D9). **기본은 편다** — 설정
   * 다이얼로그의 형상이며 M6·M7 절이 재는 그 표면이다. 끄면 대시보드에 놓인 패널이고,
   * 그때 배율 컨트롤은 도크가 아니라 **떠 있는 줄**에 선다.
   *
   * 이 인자가 이 파일에 새로 생긴 축이다 — M9 까지의 시험은 **한 표면만** 재었고, 그래서
   * "층이 서는 자리에 손잡이가 없다" 는 부류가 구조적으로 보이지 않았다(D9).
   */
  docked?: boolean;
  /** 캔버스 좌표계 크기. 기본은 이 파일의 5:4 고정 입력이다(M10 절만 고정점을 쓴다). */
  canvas?: CanvasSize;
  /** 프레임 계수를 재는 시험만 넘긴다. 기본은 브라우저 구현이다. */
  scheduler?: FrameScheduler;
}) {
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <div data-testid="parent" className="relative">
        <span data-testid="selection">{[...state.selection].join(',')}</span>
        <CanvasEditDockRegion enabled={docked}>
          <CanvasSurface
            elements={[...elements]}
            canvas={canvas}
            targetStyles={{}}
            texts={{}}
            workspace={workspace}
            scheduler={scheduler}
            overlay={({ projection, textWidths }) => (
              <CanvasEditOverlay
                enabled
                elements={elements}
                projection={projection}
                textWidths={textWidths}
                onElementsChange={onElementsChange}
              />
            )}
          />
        </CanvasEditDockRegion>
      </div>
    </CanvasEditSelectionContext>
  );
}

/** 자리와 크기를 CSS 에서 그대로 읽는다(jsdom 은 레이아웃을 하지 않는다). */
function styleBox(testId: string): { left: number; top: number; width: number; height: number } {
  const el = screen.getByTestId(testId);
  return {
    left: Number.parseFloat(el.style.left),
    top: Number.parseFloat(el.style.top),
    width: Number.parseFloat(el.style.width),
    height: Number.parseFloat(el.style.height),
  };
}

/** 격자 이미지가 시작하는 자리(px 두 축). */
function gridPositionPx(): { x: number; y: number } {
  const raw = screen.getByTestId('panel-edit-grid').style.backgroundPosition;
  const m = /^(-?[\d.]+)px (-?[\d.]+)px$/.exec(raw);
  expect(m, `배경 자리를 px 로 읽지 못했다: ${raw}`).not.toBeNull();
  return { x: Number(m![1]), y: Number(m![2]) };
}

/**
 * 격자선이 실제로 서는 자리의 **위상** — 출력 영역의 원점(오버레이 좌표 0)을 기준으로 잰다.
 *
 * 격자 상자는 오버레이 루트에서 `-원점` 에 놓이고 이미지는 그 상자 안에서 `배경 자리`
 * 만큼 밀려 시작하므로, 첫 선이 서는 오버레이 좌표는 두 값의 합이다. 그 합이 0 이면
 * 선이 **출력 영역의 원점에서** 시작한다는 뜻이고, 붙임이 죄는 자리(`k × 칸`)와 같은
 * 자리가 된다.
 */
function gridPhase(): { x: number; y: number } {
  const box = styleBox('canvas-workspace-grid');
  const pos = gridPositionPx();
  return { x: box.left + pos.x, y: box.top + pos.y };
}

function renderWorkspace(
  elements: readonly CanvasElement[] = [],
  workspace = true,
  opts: { docked?: boolean; canvas?: CanvasSize; scheduler?: FrameScheduler } = {},
): ReturnType<typeof vi.fn> {
  const emit = vi.fn();
  render(
    <WorkspaceComposed
      elements={elements}
      onElementsChange={emit}
      workspace={workspace}
      {...opts}
    />,
  );
  return emit;
}

describe('작업 영역과 출력 영역이 눈으로 갈린다 (SPEC-CANVAS-006 M6)', () => {
  beforeEach(() => {
    composedOuter = WS_OUTER;
    vi.stubGlobal('ResizeObserver', ComposedResizeObserver as unknown as typeof ResizeObserver);
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('두 상자의 크기가 **실제로 다르다** — 같으면 이 SPEC 이 아무 일도 하지 않은 것이다', () => {
    renderWorkspace();

    // D4: 축소 비율을 시험에서 1.0 으로 갈아 끼우면 아래 시험 전부가 무의미해진다.
    // 그래서 나머지를 재기 **전에** 두 상자가 다름을 먼저 못 박는다.
    expect(styleBox('canvas-workspace')).toMatchObject({ width: 1749, height: 796 });
    expect(stageBoxSize()).toEqual(WS_STAGE);
    expect(stageBoxSize().width).not.toBe(1749);
  });

  it('격자는 **작업 영역 전체**에 펴진다 (음수 인셋으로 출력 영역을 넘어선다)', () => {
    renderWorkspace();
    fireEvent.click(gridToggle());

    const box = styleBox('canvas-workspace-grid');
    // 오버레이 루트는 출력 영역 상자다. 격자 상자는 거기서 원점만큼 **되돌아 나가** 잰
    // 상자 전부를 덮는다 — 저술 여백에도 칸이 보여야 밖에 놓은 요소가 어디에 붙는지 읽힌다.
    expect(box).toEqual({ left: -504, top: -102, width: 1749, height: 796 });
  });

  it('격자선은 **출력 영역의 원점**에서 시작한다 (위험 R3 — 세 번 걷어낸 그 거짓말)', () => {
    renderWorkspace();
    fireEvent.click(gridToggle());

    // 원점이 칸의 배수가 아니라는 사실을 시험이 **수로** 든다(D3).
    expect(WS_ORIGIN.x % WS_CELL).toBe(23);
    expect(WS_ORIGIN.y % WS_CELL).toBe(28);

    // 배경 자리가 원점과 **같은 값**이다 — 이것이 AC-04 가 요구하는 곧은 단언이다.
    expect(gridPositionPx()).toEqual(WS_ORIGIN);
    // 그리고 그 결과 첫 선이 오버레이 좌표 0(= 출력 영역의 원점)에 선다. 격자를 작업
    // 영역의 왼쪽 위에 앉히면 이 값이 -504 가 되어 붙임과 갈라진다.
    expect(gridPhase()).toEqual({ x: 0, y: 0 });
  });

  it('그려진 칸은 여전히 정사각형 정수 px 다 (AC-E19 (R) · AC-E20 (V) 유지)', () => {
    renderWorkspace();
    fireEvent.click(gridToggle());

    const cell = drawnCellPx();
    expect(cell).toEqual({ x: WS_CELL, y: WS_CELL });
    expect(Number.isInteger(cell.x)).toBe(true);
    // 타일이 한 칸이므로 상자 크기(1749 — 칸의 배수가 아니다)가 위상을 흔들지 못한다.
    expect(screen.getByTestId('panel-edit-grid').style.backgroundSize).toBe('37px 37px');
  });

  it('중심 표식 `+` 는 **출력 영역의 중심**에 선다 (작업 영역의 중심이 아니다)', () => {
    renderWorkspace();
    fireEvent.click(gridToggle());

    // `+` 는 제 상자(= 격자 상자 = 작업 영역)의 정중앙에 선다. 출력 영역이 작업 영역
    // 안에서 **가운데 정렬**되므로 두 중심이 같은 자리다(축마다 floor 로 인한 1px 이내).
    // 자리 계산을 가운데 정렬이 아닌 것으로 바꾸면 이 단언이 함께 실패해야 한다.
    const box = styleBox('canvas-workspace-grid');
    const markX = box.left + box.width / 2;
    const markY = box.top + box.height / 2;
    expect(Math.abs(markX - WS_STAGE.width / 2)).toBeLessThanOrEqual(1);
    expect(Math.abs(markY - WS_STAGE.height / 2)).toBeLessThanOrEqual(1);
  });

  it('출력 영역 **밖**으로 붙은 요소의 중심이 그려진 선 위다 (안쪽만 재면 통과하는 결함이다)', async () => {
    const emit = renderWorkspace([rect('a', SNAP_GEOMETRY)]);
    stubOverlayRect(0, 0, WS_STAGE.width, WS_STAGE.height);
    fireEvent.click(gridToggle());

    // 도형 안(px 상자 37,59.2 ~ 170.2,177.6)을 잡아 출력 영역 밖으로 끈다.
    send('pointerdown', 103, 118);
    send('pointermove', -197, -132);
    await nextFrame();
    send('pointerup', -197, -132);

    const g = emittedGeometry(emit, 'a') as BoxGeometry;
    const center = { x: g.x + g.w / 2, y: g.y + g.h / 2 };
    // 실제로 **밖**으로 나갔는가 — 안에 남았으면 이 시험은 제 이름값을 하지 못한다.
    expect(center.x).toBeLessThan(0);
    expect(center.y).toBeLessThan(0);

    // 중심을 화면으로 투영한 px(오버레이 좌표)가 그려진 선 위인가.
    const px = {
      x: (center.x * WS_STAGE.width) / CANVAS.width,
      y: (center.y * WS_STAGE.height) / CANVAS.height,
    };
    const phase = gridPhase();
    // `Math.abs` 는 음수 나머지의 `-0` 을 접기 위한 것이다 — 재는 것은 나머지가 0 인가다.
    expect(Math.abs((px.x - phase.x) % WS_CELL)).toBe(0);
    expect(Math.abs((px.y - phase.y) % WS_CELL)).toBe(0);
  });

  it('경계와 흐림이 그려지고, 파랑이 아니라 편집 보조선의 회색이다', () => {
    renderWorkspace();

    const scrim = screen.getByTestId('canvas-region-scrim');
    const bounds = screen.getByTestId('canvas-region-bounds');
    // 흐림은 상자 **하나**에 바깥으로 퍼지는 그림자다 — 사각형 넷을 좌표로 두르는 길은
    // 같은 상자를 네 번 다시 파생하는 일이라 위험 R1 의 축소판이다.
    expect(scrim.style.boxShadow).toBe('0 0 0 9999px rgba(148, 163, 184, 0.22)');
    expect(bounds.style.borderColor).toBe('rgba(148, 163, 184, 0.95)');
    // 파랑은 이 화면에서 이미 "고른 것"(선택 윤곽선)과 "가운데"(중심 표식)를 뜻한다.
    expect(scrim.style.boxShadow).not.toContain('59, 130, 246');
    expect(bounds.style.borderColor).not.toContain('59, 130, 246');
    // 경계는 실선이라 격자선(점선이 아닌 반투명 선)과 헷갈리지 않는다.
    expect(bounds.className).not.toContain('dashed');
  });

  it('경계·흐림·격자 상자는 장식이며 포인터를 먹지 않는다 (위험 R8)', () => {
    renderWorkspace();
    fireEvent.click(gridToggle());

    for (const id of ['canvas-region-scrim', 'canvas-region-bounds', 'canvas-workspace-grid']) {
      const el = screen.getByTestId(id);
      expect(el.className, id).toContain('pointer-events-none');
      expect(el.getAttribute('aria-hidden'), id).toBe('true');
    }
  });

  it('요소 0개 + 편집에서 팔레트와 캔버스 누름이 그대로 통한다 (AC-E10 재확인)', () => {
    const emit = renderWorkspace([]);
    stubOverlayRect(0, 0, WS_STAGE.width, WS_STAGE.height);

    // 재려는 것이 켜져 있음을 먼저 단언한다 — 표시가 없으면 이 시험은 틀린 이유로 통과한다.
    expect(screen.getByTestId('canvas-region-scrim')).toBeTruthy();
    expect(screen.getByTestId('canvas-region-bounds')).toBeTruthy();

    fireEvent.click(screen.getByTestId('canvas-palette-add-rect'));
    expect(emit).toHaveBeenCalledTimes(1);
    expect((emit.mock.calls[0]?.[0] as CanvasElement[]).length).toBe(1);

    // 빈 자리 누름의 임자는 흐림이 그 위에 있어도 달라지지 않는다 — 주 버튼은 사각형이
    // 가져가고(010), 그 밖의 버튼은 종전대로 흘러간다. 재는 것은 "흐림이 규칙을 바꾸지
    // 않는다" 이므로 **두 갈래를 모두** 본다.
    expect(send('pointerdown', 300, 300).defaultPrevented).toBe(true);
    expect(send('pointerdown', 300, 300, { button: 2 }).defaultPrevented).toBe(false);
  });

  it('격자를 꺼도 경계와 흐림은 남는다 (둘은 다른 축이다)', () => {
    renderWorkspace();

    expect(screen.queryByTestId('panel-edit-grid')).toBeNull();
    expect(screen.getByTestId('canvas-region-scrim')).toBeTruthy();
    expect(screen.getByTestId('canvas-region-bounds')).toBeTruthy();
  });

  it('작업 영역을 끄면 격자 상자가 출력 영역과 겹친다 (D1 — 끈 갈래도 함께 잰다)', () => {
    renderWorkspace([], false);
    fireEvent.click(gridToggle());

    const stage = stageBoxSize();
    expect(styleBox('canvas-workspace')).toMatchObject(stage);
    expect(styleBox('canvas-workspace-grid')).toEqual({
      left: 0,
      top: 0,
      width: stage.width,
      height: stage.height,
    });
    expect(gridPositionPx()).toEqual({ x: 0, y: 0 });
  });
});

describe('표면 밖에서는 오늘의 값으로 떨어진다 (컨텍스트 폴백)', () => {
  it('격자 상자가 투영이 든 출력 영역과 같고 원점이 (0,0) 이다', () => {
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={vi.fn()} />);
    fireEvent.click(gridToggle());

    // 표면이 없으면 컨텍스트가 `null` 이므로 오버레이는 `origin = {0,0}` ·
    // `box = projection.stage` 로 떨어진다 — 그것이 곧 "상자가 하나뿐이던 시절" 의 값이다.
    expect(styleBox('canvas-workspace-grid')).toEqual({
      left: 0,
      top: 0,
      width: STAGE.width,
      height: STAGE.height,
    });
    expect(gridPositionPx()).toEqual({ x: 0, y: 0 });
  });

  it('경계와 흐림은 표면 밖에서도 그려진다 (편집이 켜져 있으면 언제나 뜬다)', () => {
    render(<Harness elements={[]} onElementsChange={vi.fn()} />);

    expect(screen.getByTestId('canvas-region-scrim')).toBeTruthy();
    expect(screen.getByTestId('canvas-region-bounds')).toBeTruthy();
  });

  it('편집이 꺼져 있으면 셋 다 DOM 에 없다', () => {
    render(<Harness enabled={false} elements={[]} onElementsChange={vi.fn()} />);

    expect(screen.queryByTestId('canvas-region-scrim')).toBeNull();
    expect(screen.queryByTestId('canvas-region-bounds')).toBeNull();
    expect(screen.queryByTestId('canvas-workspace-grid')).toBeNull();
  });
});

// --- 저술 여백이 포인터를 받는다 (SPEC-CANVAS-006 M7 · REQ-08) ---------------
//
// **이 절이 재는 것은 "닿음" 이다.** M6 이 비트맵과 표시 층을 작업 영역만큼 넓혀 저술
// 여백의 요소가 **칠해지게** 되었으나, 포인터 처리자를 단 노드는 오버레이 루트 하나뿐이고
// 그 루트는 `canvas-stage` 안의 `absolute inset-0` 이라 **닿는 면이 정확히 출력 영역**이었다.
// 여백을 누르면 그 사건은 `canvas-workspace` 나 `<canvas>` 에 떨어지는데 둘 다 처리자를
// 달지 않으므로 `handlePointerDown` 에 영영 닿지 않았다 — REQ-03 의 두 조항과 AC-02 가
// 형상만 있고 성립하지 않았다.
//
// **그 결함이 전량 green 인 스위트를 통과했다**(시험 규율 D6). 이 파일의 모든 포인터
// 시험이 처리자를 단 그 노드에 **직접** 쏘고(`fireEvent(getByTestId('canvas-edit-overlay'),
// …)`), jsdom 은 레이아웃을 하지 않아 히트 테스트가 통째로 건너뛰어지기 때문이다. 루트에
// 직접 쏜 시험은 **닿음에 대해 아무것도 말하지 않는다.**
//
// 그래서 주장을 둘로 갈라 잰다.
//   ① **구조**(AC-08 (AO)) — 닿는 노드의 상자가 작업 영역과 같고, `pointer-events-none` 이
//      **없으며**, 그 노드와 작업 영역 사이에 클리핑이 없다. "브라우저가 이 노드를 히트
//      테스트로 고를 것인가" 는 jsdom 이 답할 수 없는 질문이므로, 그 답이 참일 **조건들**을
//      대신 못박는다.
//   ② **경로**(AC-08 (AP)) — **처리자를 달지 않은 노드**(닿는 층)에 쏜 사건이 버블링으로
//      루트의 처리자에 닿는다. jsdom 에서도 버블링은 진짜다. 쏘는 자리를 `canvas-workspace`
//      나 `<canvas>` 로 잡으면 그 둘은 오버레이의 **조상**이라 사건이 아래로 내려오지 않아
//      결함이 없어도 실패한다.
//
// **고정 입력은 M6 절의 그것 그대로다**(outer 1749×796 · 캔버스 500×400 · 간격 25 →
// 칸 37 · 출력 영역 740×592 · 원점 (504, 102) · 작업 영역 1749×796). 여기에 둘을 더한다.
//
//   - **포인터 축척 0.5**(AC-E3 계승 · 위험 R1). 오버레이 루트의 화면 상자를 370×296 으로
//     심어 `rect.width ÷ stage.width = 0.5` 로 둔다. 축척이 1 이면 나눗셈이 항등이 되어
//     "재는 상자를 작업 영역으로 넓혔다" 는 결함이 **보이지 않는다.** 재는 상자가 작업
//     영역이 되면 축척(1749÷740)과 원점이 함께 틀리므로 이 절의 히트가 전부 빗나간다.
//   - **완전히 출력 영역 밖에 있는 요소**(D2). 캔버스 좌표가 두 축 모두 음수라, 안쪽
//     요소만으로는 잴 수 없는 것을 잰다.
//
// 투영 축척은 두 축 모두 740÷500 = 592÷400 = **1.48** 이다.

/** 완전히 출력 영역 **밖**(두 축 모두 음수)에 있는 사각형 — px 로 -236.8..-88.8 × -118.4..-29.6. */
const OUTSIDE_GEOMETRY: BoxGeometry = { x: -160, y: -80, w: 100, h: 60 };

/** 오버레이 루트에 심을 화면 상자 — 스테이지의 **절반**이라 포인터 축척이 0.5 다. */
const WS_POINTER_RECT = { width: WS_STAGE.width / 2, height: WS_STAGE.height / 2 };

/** 캔버스 단위 → 오버레이 px(투영) → 화면 client px(축척 0.5). */
function clientFromCanvas(x: number, y: number): { x: number; y: number } {
  const px = { x: (x * WS_STAGE.width) / CANVAS.width, y: (y * WS_STAGE.height) / CANVAS.height };
  return { x: px.x / 2, y: px.y / 2 };
}

/** 닿는 층. **루트가 아니다** — 이 절의 경로 시험이 성립하는 유일한 자리다. */
function hitLayer(): HTMLElement {
  return screen.getByTestId('canvas-workspace-hit');
}

/** 닿는 층에 이벤트를 쏜다. 돌려주는 이벤트로 소비 여부를 잰다. */
function sendToHit(type: string, x: number, y: number, init: MouseEventInit = {}): Event {
  return sendAt(hitLayer(), type, x, y, init);
}

describe('저술 여백 전체가 포인터를 받는다 (SPEC-CANVAS-006 M7 · REQ-08 · AC-08)', () => {
  beforeEach(() => {
    composedOuter = WS_OUTER;
    vi.stubGlobal('ResizeObserver', ComposedResizeObserver as unknown as typeof ResizeObserver);
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // --- ① 구조 (AC-08 (AO)) ------------------------------------------------

  it('닿는 면의 상자가 **작업 영역**과 같다 — 출력 영역이 아니다 (AC-08 (AO))', () => {
    renderWorkspace();

    // 두 상자가 실제로 다름을 먼저 단언한다(D4). 같으면 이 시험이 아무것도 재지 않는다 —
    // `inset-0` 짜리 층도 통과해 버린다.
    expect(stageBoxSize()).toEqual(WS_STAGE);
    expect(WS_OUTER).not.toEqual(WS_STAGE);

    // 음수 인셋이 이 단언의 전부다. `left/top` 을 0 으로 바꾸면(= `inset-0`) 실패한다.
    expect(styleBox('canvas-workspace-hit')).toEqual({
      left: -WS_ORIGIN.x,
      top: -WS_ORIGIN.y,
      width: WS_OUTER.width,
      height: WS_OUTER.height,
    });
    // 격자 상자와 **같은 상자**다 — 두 값을 각자 파생하면 어느 날 한쪽만 고쳐진다(위험 R1).
    expect(styleBox('canvas-workspace-hit')).toEqual(styleBox('canvas-workspace-grid'));
  });

  it('닿는 면은 포인터를 **먹으라고** 있다 — 표시 층 셋과 반대다 (AC-08 (AO) · 위험 R16)', () => {
    renderWorkspace();

    const hit = hitLayer();
    // 이 한 줄이 이 밀레스톤의 전부다. `pointer-events-none` 을 붙이면 여백이 다시 죽는다.
    expect(hit.className).not.toContain('pointer-events-none');
    // `touch-action` 은 **상속되지 않는 속성**이라 루트의 `touch-none` 이 여기로 내려오지
    // 않는다. 빠뜨리면 터치에서만 여백의 드래그가 스크롤에 먹힌다.
    expect(hit.className).toContain('touch-none');
    expect(screen.getByTestId('canvas-edit-overlay').className).toContain('touch-none');
  });

  it('닿는 면은 **아무것도 그리지 않는다** — 배경도 테두리도 이름도 없다 (AC-08 (AO))', () => {
    renderWorkspace();

    const hit = hitLayer();
    expect(hit.getAttribute('aria-hidden')).toBe('true');
    expect(hit.getAttribute('aria-label')).toBeNull();
    expect(hit.getAttribute('role')).toBeNull();
    expect(hit.textContent).toBe('');
    expect(hit.children.length).toBe(0);
    // 그리는 속성이 하나라도 붙으면 이 층은 표시 층이 되고, 그러면 위험 R8 의 가드가
    // 지키는 문장("그리는 층은 포인터를 먹지 않는다")과 정면으로 부딪친다.
    expect(hit.style.backgroundColor).toBe('');
    expect(hit.style.borderColor).toBe('');
    expect(hit.style.boxShadow).toBe('');
    expect(hit.className).not.toContain('border');
    expect(hit.className).not.toContain('bg-');
  });

  it('표시 층 셋은 **여전히** 포인터를 먹지 않고, 닿는 면은 그 목록 밖이다 (위험 R8 가드 유지)', () => {
    renderWorkspace();
    fireEvent.click(gridToggle());

    // 위험 R8 의 가드가 열거하는 그 세 이름 — 한 글자도 달라지지 않는다.
    for (const id of ['canvas-region-scrim', 'canvas-region-bounds', 'canvas-workspace-grid']) {
      expect(screen.getByTestId(id).className, id).toContain('pointer-events-none');
    }
    // 새 층을 그 목록에 더하면 가드가 곧 이 기능을 금지한다. 그래서 더하지 않는다.
    expect(hitLayer().className).not.toContain('pointer-events-none');
  });

  it('닿는 면과 작업 영역 사이에 **클리핑이 없다** — 잘리는 자리는 표면 컨테이너 하나다 (AC-08 (AO))', () => {
    renderWorkspace();

    const workspace = screen.getByTestId('canvas-workspace');
    const between: HTMLElement[] = [];
    for (let node = hitLayer().parentElement; node !== null && node !== workspace; ) {
      between.push(node);
      node = node.parentElement;
    }
    // 오버레이 루트와 `canvas-stage` 를 실제로 지나갔는가 — 0개면 이 순회가 아무것도 재지 않는다.
    expect(between.length).toBeGreaterThanOrEqual(2);
    for (const node of between) {
      expect(node.className, node.dataset.testid ?? node.tagName).not.toContain('overflow');
      expect(node.style.overflow).toBe('');
    }
    // 잘리는 자리는 표면 컨테이너 하나이며, 그것이 **비트맵이 잘리는 그 자리**다.
    expect(workspace.parentElement?.className).toContain('overflow-hidden');
  });

  it('닿는 면은 루트의 **첫 자식**이라 표시 층과 손잡이가 그 위에 얹힌다 (AC-08 (AQ))', () => {
    const emit = renderWorkspace([rect('far', OUTSIDE_GEOMETRY)]);
    stubOverlayRect(0, 0, WS_POINTER_RECT.width, WS_POINTER_RECT.height);
    const center = clientFromCanvas(-110, -50);
    sendToHit('pointerdown', center.x, center.y);
    expect(selectionText()).toBe('far');
    expect(emit).not.toHaveBeenCalled();

    const root = screen.getByTestId('canvas-edit-overlay');
    const order = [...root.children];
    expect(root.firstElementChild).toBe(hitLayer());
    // 뒤에 오는 형제가 위에 얹혀야 손잡이를 잡을 수 있다 — 닿는 면이 손잡이를 덮으면
    // 크기 조절이 통째로 죽는다.
    for (const later of [
      screen.getByTestId('canvas-region-scrim'),
      screen.getByTestId('canvas-region-bounds'),
      screen.getByTestId('canvas-selection-far'),
    ]) {
      expect(order.indexOf(hitLayer())).toBeLessThan(order.indexOf(later));
    }
    expect(order.indexOf(hitLayer())).toBeLessThan(order.indexOf(handleEl('e')));
  });

  it('작업 영역을 끄면 닿는 면이 출력 영역과 겹친다 (D1 — 끈 갈래도 함께 잰다)', () => {
    renderWorkspace([], false);

    const stage = stageBoxSize();
    expect(styleBox('canvas-workspace-hit')).toEqual({
      left: 0,
      top: 0,
      width: stage.width,
      height: stage.height,
    });
  });

  it('편집이 꺼지면 닿는 면도 없다 — 오버레이 자체가 DOM 에 없다 (AC-06 유지)', () => {
    render(<Harness enabled={false} elements={[]} onElementsChange={vi.fn()} />);

    expect(screen.queryByTestId('canvas-workspace-hit')).toBeNull();
  });

  // --- ② 경로 (AC-08 (AP)) ------------------------------------------------

  it('닿는 면에서 시작한 누름이 **버블링으로** 루트의 처리자에 닿아 밖의 요소가 골라진다 (AC-08 (AP))', () => {
    renderWorkspace([rect('far', OUTSIDE_GEOMETRY)]);
    stubOverlayRect(0, 0, WS_POINTER_RECT.width, WS_POINTER_RECT.height);

    // 재려는 것이 성립하는 조건 셋을 먼저 못박는다.
    // (1) 요소가 **완전히** 출력 영역 밖이다(D2) — 걸쳐 있으면 안쪽 히트로도 통과한다.
    expect(OUTSIDE_GEOMETRY.x + OUTSIDE_GEOMETRY.w).toBeLessThan(0);
    expect(OUTSIDE_GEOMETRY.y + OUTSIDE_GEOMETRY.h).toBeLessThan(0);
    // (2) 포인터 축척이 **1 이 아니다**(AC-E3 계승) — 1 이면 재는 상자를 넓힌 결함이 보이지 않는다.
    expect(WS_POINTER_RECT.width / WS_STAGE.width).toBe(0.5);
    // (3) 그 요소가 작업 영역 **안**에는 있다 — 밖이면 닿는 면이 넓어져도 소용이 없다.
    expect((OUTSIDE_GEOMETRY.x * WS_STAGE.width) / CANVAS.width).toBeGreaterThan(-WS_ORIGIN.x);

    // 축척 0.5 를 **무시했을 때** 쓰게 되는 좌표(= 오버레이 px 를 그대로 client 로 쓴 값).
    // 여기서 골라지면 이 시험은 축척에 대해 아무것도 재지 않는 것이다.
    // 주 버튼이 아닌 누름으로 잰다 — 010 이후 주 버튼은 빈 자리에서도 소비되므로
    // `defaultPrevented` 가 더 이상 "골랐는가" 를 가리지 못한다. 이 시험이 재려는 것은
    // **좌표 환산**이고, 그 눈은 `defaultPrevented` 가 아니라 선택이다.
    const naive = { x: -162.8, y: -74 };
    expect(sendToHit('pointerdown', naive.x, naive.y, { button: 2 }).defaultPrevented).toBe(false);
    expect(selectionText()).toBe('');

    // 축척을 되돌린 진짜 좌표. 이 사건은 **처리자가 없는 노드**에서 시작한다.
    const center = clientFromCanvas(-110, -50);
    expect(center).toEqual({ x: naive.x / 2, y: naive.y / 2 });
    const down = sendToHit('pointerdown', center.x, center.y);

    expect(selectionText()).toBe('far');
    // 히트가 있으면 오늘처럼 소비한다 — 밖을 위한 분기가 없다는 뜻이다.
    expect(down.defaultPrevented).toBe(true);
  });

  it('밖의 요소를 닿는 면에서 잡아 끌면 좌표가 손을 따라오고 **clamp 되지 않는다** (AC-08 (AP))', async () => {
    const emit = renderWorkspace([rect('far', OUTSIDE_GEOMETRY)]);
    stubOverlayRect(0, 0, WS_POINTER_RECT.width, WS_POINTER_RECT.height);

    const from = clientFromCanvas(-110, -50);
    // client 로 +37 은 스테이지 px 로 +74, 캔버스 단위로 정확히 +50 이다(74 ÷ 1.48).
    sendToHit('pointerdown', from.x, from.y);
    send('pointermove', from.x + 37, from.y);
    await nextFrame();
    send('pointerup', from.x + 37, from.y);

    const g = emittedGeometry(emit, 'far') as BoxGeometry;
    expectBox(g, { x: -110, y: -80, w: 100, h: 60 });
    // 여전히 밖이다 — 어딘가에서 죄었다면 이 단언이 실패한다(가정 A5 · A15).
    expect(g.x + g.w).toBeLessThan(0);
  });

  it('밖의 **빈 자리** 누름도 안쪽과 **같은 사건**이다 (REQ-03 셋째 조항 · AC-08 (AP))', () => {
    renderWorkspace([rect('far', OUTSIDE_GEOMETRY)]);
    stubOverlayRect(0, 0, WS_POINTER_RECT.width, WS_POINTER_RECT.height);

    const center = clientFromCanvas(-110, -50);
    sendToHit('pointerdown', center.x, center.y);
    expect(selectionText()).toBe('far');

    // 같은 저술 여백의 빈 자리(캔버스 -300, -40 — 위 요소의 왼쪽).
    const empty = clientFromCanvas(-300, -40);
    const down = sendToHit('pointerdown', empty.x, empty.y);

    // A8 · I8 이 말하는 것은 "저술 여백의 빈 자리와 출력 영역 **안**의 빈 자리가 같은
    // 사건이다" 이며, 그 진술은 011 뒤에도 그대로 참이다 — 다만 그 한 사건이 이제 **팬의**
    // 시작이다(010 에서는 사각형이었다). 밖을 위한 분기가 없다는 것이 여기서 드러난다.
    expect(down.defaultPrevented).toBe(true);
    // **011 이 뒤집은 자리다.** 종전에는 여백의 맨손 끌기가 사각형을 세웠고, 이제 그것은
    // 팬이다 — 그래서 사각형이 서지 **않는** 것이 옳다. 비우는 일도 누름이 아니라 뗌의
    // 몫으로 옮겨 갔다(움직이지 않은 팬이 곧 클릭이다).
    sendToHit('pointermove', empty.x + 40, empty.y + 30);
    expect(screen.queryByTestId('canvas-marquee')).toBeNull();
    expect(selectionText()).toBe('far');
    sendToHit('pointerup', empty.x + 40, empty.y + 30);
    // 끌었으므로 클릭이 아니다 — 선택은 팬에 살아남는다.
    expect(selectionText()).toBe('far');

    // 사각형은 여백에서도 선다 — 조작키를 짚으면 안쪽과 **같은 사건**이라는 것이 여전히
    // 이 절의 문장이다(006 M7 이 고친 결함의 그 갈래가 Ctrl 로 옮겨 앉았을 뿐이다).
    sendToHit('pointerdown', empty.x, empty.y, { ctrlKey: true });
    sendToHit('pointermove', empty.x + 40, empty.y + 30, { ctrlKey: true });
    expect(screen.queryByTestId('canvas-marquee')).not.toBeNull();
    sendToHit('pointerup', empty.x + 40, empty.y + 30, { ctrlKey: true });
    // 갈아 끼우는 사각형이므로 밖에 있던 `far` 가 남지 않는다.
    expect(selectionText()).toBe('');

    // 빈 자리를 **끌지 않고** 누르면 종전 그대로 선택이 풀린다 — 뜻은 뗌으로만 옮겨 갔다.
    sendToHit('pointerdown', center.x, center.y);
    sendToHit('pointerup', center.x, center.y);
    expect(selectionText()).toBe('far');
    sendToHit('pointerdown', empty.x, empty.y);
    sendToHit('pointerup', empty.x, empty.y);
    expect(selectionText()).toBe('');

    // 주 버튼이 아닌 갈래는 여전히 흘러간다 — `previewPan` 이 읽는 그 표시 그대로다.
    expect(sendToHit('pointerdown', empty.x, empty.y, { button: 2 }).defaultPrevented).toBe(false);
  });

  // --- ③ 바뀌지 않은 것 (AC-08 (AQ)) ---------------------------------------

  it('탭 정지점은 **여전히 루트**다 — 닿는 면은 초점을 받지 않는다 (AC-08 (AQ) · T15)', () => {
    renderWorkspace();

    const root = screen.getByTestId('canvas-edit-overlay');
    expect(root.getAttribute('tabindex')).toBe('0');
    expect(hitLayer().getAttribute('tabindex')).toBeNull();
    // 초점을 받을 수 있는 자식이 늘지 않았다 — 닿는 면은 `<div>` 이고 이름도 없다.
    expect(hitLayer().tagName).toBe('DIV');
  });

  it('손잡이는 닿는 면보다 **위**에 있어 크기 조절이 그대로 시작된다 (AC-08 (AQ))', async () => {
    const emit = renderWorkspace([rect('far', OUTSIDE_GEOMETRY)]);
    stubOverlayRect(0, 0, WS_POINTER_RECT.width, WS_POINTER_RECT.height);

    const center = clientFromCanvas(-110, -50);
    sendToHit('pointerdown', center.x, center.y);
    expect(selectionText()).toBe('far');

    // 손잡이는 오버레이 px 자리에 서고, 화면 좌표는 그 절반이다(축척 0.5).
    const east = handleEl('e');
    const at = {
      x: Number.parseFloat(east.style.left) / 2,
      y: Number.parseFloat(east.style.top) / 2,
    };
    const grab = sendAt(east, 'pointerdown', at.x, at.y);
    // 손잡이가 사건을 끊는다 — 닿는 면이 손잡이를 덮었다면 여기서 몸통 이동이 시작된다.
    expect(grab.defaultPrevented).toBe(true);
    send('pointermove', at.x + 37, at.y);
    await nextFrame();
    send('pointerup', at.x + 37, at.y);

    // 오른쪽 변만 +50 — 옮겨진 것이 아니라 **늘어났다**.
    expectBox(emittedGeometry(emit, 'far'), { x: -160, y: -80, w: 150, h: 60 });
  });
});

// --- 도크가 없는 자리의 배율 줄 (SPEC-CANVAS-006 M10 · REQ-10 · AC-10) --------
//
// **이 절의 무게중심은 컨트롤이 아니라 관계다.** 배율 칸이 그려지는지만 재면 이 회차는
// M9 의 시험을 한 번 더 쓰는 일에 지나지 않는다. 실제로 잡아야 할 것은 **"층이 서는
// 자리에 손잡이가 없다"** 는 부류이며, 그것은 **두 표면을 갈아 끼우며 층과 컨트롤을 함께**
// 재야만 보인다(시험 규율 D9 · 불변식 I23).
//
// 006 이 그 원리를 깬 방식이 그대로 이 절의 정의다: 표시 층 셋은 **조건 없이** 그려지는데
// (`canvas-workspace-grid` · `canvas-region-scrim` · `canvas-region-bounds`) 컨트롤 전부는
// `dockHost !== null` 뒤에 있었고, 도크를 펴는 곳은 설정 다이얼로그 한 자리뿐이었다.
// **전량 green 인 스위트가 그것을 잡지 못한 이유는 두 조건이 같은 자리에서 비교된 적이
// 없어서다** — 층의 시험은 층만 보고 컨트롤의 시험은 컨트롤만 본다.
//
// **고정 입력은 이 SPEC 의 고정점이다**(시험 규율 D7): `canvas === outer === 1749×796` ·
// 간격 25. 그 짝의 유도값을 수로 적는다.
//
//   | z    | reduced   | exact    | cell | 축척 | stage            | origin     |
//   |------|-----------|----------|------|------|------------------|------------|
//   | 0.75 | 1311×597  | 18.7392  | 18   | 0.72 | 1259.28×573.12   | (244, 111) |
//   | 0.50 | 874×398   | 12.4928  | 12   | 0.48 | 839.52×382.08    | (454, 206) |
//   | 1.00 | 1749×796  | 25       | 25   | 1.00 | 1749×796         | (0, 0)     |
//
// **D8 을 만족하는 짝을 고른 것이다**: `z = 0.50` 은 기본값과 `cell`·축척·`origin` 이
// **셋 다** 다르다. 기본값과 같은 그림을 내는 이웃(0.73~0.76)을 골랐다면 배선이 끊겨 있어도
// 초록이었을 것이다. 그리고 `z = 1.00` 행이 이 절에서만 관측 가능한 것 하나를 준다 —
// 저술 여백이 **0** 이 되어 작업 영역의 왼쪽 아래가 곧 출력 영역의 왼쪽 아래가 되고,
// 그때 줄이 경계의 한 모서리를 덮는다(대가를 숨기지 않는다 · AC-10 (BB)).
//
// 이 절이 재지 **않는** 것 둘을 미리 적는다: 줄은 **위험 R19 의 완화가 아니고**(배율을
// 닿을 수 있게 할 뿐 배율이 못 하는 일을 하게 만들지 않는다), **대시보드의 나머지 넷**
// (팔레트 · 격자 토글 · 격자 간격 · 정렬 · 순서)을 고치지 않는다 — 그 넷을 주는 안은
// 사용자에게 제시되었고 고르지 않았다.

/** 고정점 고정 입력 — `canvas === outer` 다(D7). 이 절만 쓴다. */
const FP_CANVAS: CanvasSize = { width: 1749, height: 796 };
const FP_STAGE = { width: 1259.28, height: 573.12 };
const FP_ORIGIN = { x: 244, y: 111 };
/** `z = 0.50` 의 값들 — 기본값과 셋이 모두 다르다(D8). */
const FP_STAGE_HALF = { width: 839.52, height: 382.08 };
const FP_ORIGIN_HALF = { x: 454, y: 206 };
/** 줄이 작업 영역 모서리에서 떨어지는 px — 구현의 `ZOOM_BAR_INSET_PX` 와 같은 수다. */
const FP_BAR_INSET = 8;

/**
 * **표시 층 → 그 층을 다스리는 컨트롤들.** 불변식 I23 의 그 표이며, 이 절의 가장 무거운
 * 배달물이다.
 *
 * 격자 층에 배율이 함께 있는 것에 뜻이 있다 — 배율은 격자 상자의 **크기**를 정하므로
 * 그 층을 다스리는 손잡이가 맞다. 그래서 대시보드에서도 격자 층은 손잡이가 **0 이 아니다**
 * (격자 토글·간격은 여전히 없고, 그 비대칭은 이 회차가 고치지 않는다 — 부분 덮임과
 * 무덮임의 차이가 이 불변식의 전부다).
 */
const LAYER_CONTROLS: Readonly<Record<string, readonly string[]>> = {
  'canvas-workspace-grid': ['canvas-workspace-zoom', 'canvas-grid-toggle', 'canvas-grid-step'],
  'canvas-region-scrim': ['canvas-workspace-zoom'],
  'canvas-region-bounds': ['canvas-workspace-zoom'],
};

/**
 * 이 노드나 그 자손이 **무언가를 칠하는가**. 위험 R8 가드를 이름에서 형상으로 옮기는 판정
 * 이며, 닿는 면이 "아무것도 그리지 않는다" 를 단언할 때 이미 쓴 그 속성들이다.
 *
 * 자손까지 보는 이유: 격자 층 자신은 빈 상자이고 칠하는 것은 그 안의 `PanelEditGrid` 다.
 */
function paintsSomething(el: HTMLElement): boolean {
  const nodes: HTMLElement[] = [el, ...el.querySelectorAll<HTMLElement>('*')];
  return nodes.some(
    (n) =>
      n.style.backgroundColor !== '' ||
      n.style.backgroundImage !== '' ||
      n.style.borderColor !== '' ||
      n.style.boxShadow !== '' ||
      /(?:^|\s)(?:border|bg-)/.test(n.className),
  );
}

/** 배율 칸. 두 표면에서 **같은 이름**이다 — 그래야 한 질의로 물을 수 있다. */
function zoomInput(): HTMLInputElement {
  return screen.getByTestId('canvas-workspace-zoom') as HTMLInputElement;
}

/** 떠 있는 배율 줄. */
function zoomBar(): HTMLElement {
  return screen.getByTestId('canvas-workspace-zoom-bar');
}

/** 고정점 고정 입력으로 표면 + 진짜 오버레이를 세운다. */
function renderFixedPoint(
  opts: { docked?: boolean; elements?: readonly CanvasElement[]; scheduler?: FrameScheduler } = {},
): ReturnType<typeof vi.fn> {
  return renderWorkspace(opts.elements ?? [], true, {
    docked: opts.docked ?? false,
    canvas: FP_CANVAS,
    scheduler: opts.scheduler,
  });
}

/** 표면이 지은 출력 영역이 앉은 자리 = 캔버스 좌표 원점. */
function stageOrigin(): { x: number; y: number } {
  const box = screen.getByTestId('canvas-stage');
  return { x: Number.parseFloat(box.style.left), y: Number.parseFloat(box.style.top) };
}

/** 프레임 계수를 재는 최소 시계(`CanvasSurface.test.tsx` 의 그것과 같은 형상). */
function makeClock() {
  let requested = 0;
  let nextHandle = 1;
  const pending = new Map<number, (nowMs: number) => void>();
  const scheduler: FrameScheduler = {
    request(cb) {
      requested += 1;
      const handle = nextHandle++;
      pending.set(handle, cb);
      return handle;
    },
    cancel(handle) {
      pending.delete(handle);
    },
  };
  return {
    scheduler,
    get requested() {
      return requested;
    },
    get pending() {
      return pending.size;
    },
    flush(nowMs: number) {
      const due = [...pending.values()];
      pending.clear();
      act(() => {
        for (const cb of due) cb(nowMs);
      });
    },
  };
}

describe('층이 서는 자리에 손잡이가 선다 (SPEC-CANVAS-006 M10 · 불변식 I23 · AC-10 (AX))', () => {
  beforeEach(() => {
    composedOuter = WS_OUTER;
    vi.stubGlobal('ResizeObserver', ComposedResizeObserver as unknown as typeof ResizeObserver);
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('①덮임 — 층이 DOM 에 있는 **모든 표면**에서 그 층의 컨트롤 가운데 적어도 하나가 있다', () => {
    // 이 시험의 전부가 `for (const docked of [true, false])` 한 줄이다. 한 표면만 재면
    // 0.6.0 을 통과시킨 그 형상이 그대로 돌아온다(D9).
    for (const docked of [true, false] as const) {
      cleanup();
      renderFixedPoint({ docked });
      // 도크가 있는 표면에서는 격자를 **켠다** — 재려는 층이 실제로 칠하고 있어야 한다.
      if (docked) fireEvent.click(gridToggle());

      for (const [layer, controls] of Object.entries(LAYER_CONTROLS)) {
        expect(screen.queryByTestId(layer), `${layer} @docked=${docked}`).not.toBeNull();
        const reachable = controls.filter((id) => screen.queryByTestId(id) !== null);
        expect(reachable.length, `${layer} @docked=${docked} 의 손잡이가 0 이다`).toBeGreaterThan(
          0,
        );
      }
    }
  });

  it('②완전성 — `aria-hidden` 이면서 칠하는 오버레이 자식은 **전부** 표에 있다', () => {
    // ①만 있으면 다음 층을 표에 적지 않고 넘어갈 수 있고, 그때 덮임 가드는 **조용히
    // 아무것도 지키지 않는다.** 둘이 함께 있어야 다음 층이 이 결함을 되풀이할 수 없다.
    for (const docked of [true, false] as const) {
      cleanup();
      renderFixedPoint({ docked, elements: [rect('under', { x: 100, y: 100, w: 200, h: 150 })] });
      if (docked) {
        fireEvent.click(gridToggle());
        // 선택 파생 층(윤곽선)이 실제로 서는 상태로 한 번 잰다 — 아래 면제가 형상이
        // 아니라 우연이면 여기서 드러난다.
        stubOverlayRect(0, 0, FP_STAGE.width, FP_STAGE.height);
        send('pointerdown', 150, 130);
        expect(selectionText()).toBe('under');
        // **그리고 마키가 떠 있는 상태로도 한 번 잰다**(SPEC-CANVAS-009). 이 두 줄이
        // 없으면 아래 면제는 **한 번도 걸리지 않는 죽은 필터**이고, 그 초록은 마키에
        // 대해 아무것도 말하지 않는다("없어서 통과하는 시험은 엉뚱한 이유로 초록이다").
        // 실제로 이 상태에서 면제를 빼면 이 시험은 `canvas-marquee` 로 빨개진다(실측).
        send('pointerup', 150, 130);
        send('pointerdown', 10, 10, { shiftKey: true });
        send('pointermove', 400, 300, { shiftKey: true });
        expect(screen.queryByTestId('canvas-marquee')).not.toBeNull();
      }

      const root = screen.getByTestId('canvas-edit-overlay');
      const painting = [...root.children]
        .filter(
          (child): child is HTMLElement =>
            child instanceof HTMLElement &&
            child.getAttribute('aria-hidden') === 'true' &&
            paintsSomething(child),
        )
        // **몸짓과 선택의 그림자는 표시 층이 아니다.**
        //
        // 선택 윤곽선을 면제한 근거는 "표시 **층**이 아니라 선택의 그림자다 — 그것을
        // 다스리는 것은 컨트롤이 아니라 선택 자체이고, 선택은 두 표면에 다 있다" 였다.
        // 마키 사각형에는 그 근거가 한 걸음 더 곧게 걸린다: **손이 눌려 있는 동안에만**
        // 존재하고, 그리는 것도 거두는 것도 그 몸짓 자신이다. I23 이 막는 결함의 형상은
        // "**조건 없이** 그려지는 층인데 이 표면에는 그 층을 다스릴 손잡이가 없다" 인데,
        // 다스릴 지속 상태가 아예 없으므로 손잡이를 지어 붙이면 **누를 시간이 존재하지
        // 않는 단추**가 된다. 그래서 표에 행을 더하는 대신 같은 면제를 준다.
        //
        // 면제가 표시 층으로 **번지지 않는다**는 것은 이웃 파일이 따로 지킨다
        // (`CanvasEditOverlay.marquee.test.tsx` §I23 — 마키가 이 열거에 실제로 걸린다는
        // 것과, 손을 떼면 DOM 에서 사라진다는 것을 함께 잰다).
        .filter((child) => {
          const id = child.dataset.testid ?? '';
          return !id.startsWith('canvas-selection-') && id !== 'canvas-marquee';
        });

      expect(painting.length, `@docked=${docked}`).toBeGreaterThan(0);
      for (const child of painting) {
        const id = child.dataset.testid ?? '(이름 없음)';
        expect(Object.keys(LAYER_CONTROLS), `표에 없는 칠하는 층: ${id} @docked=${docked}`).toContain(
          id,
        );
      }
    }
  });

  it('줄은 도크가 없을 때만 서고, 한 표면 안에 배율 칸이 **정확히 하나**다 (불변식 I24)', () => {
    renderFixedPoint({ docked: false });
    expect(screen.getAllByTestId('canvas-workspace-zoom').length).toBe(1);
    expect(zoomBar().contains(zoomInput())).toBe(true);
    // 도크가 대시보드로 오는 것이 아니다 — 온 것은 배율 하나다.
    expect(screen.queryByTestId('canvas-dock-panel')).toBeNull();

    cleanup();
    renderFixedPoint({ docked: true });
    expect(screen.queryByTestId('canvas-workspace-zoom-bar')).toBeNull();
    expect(screen.getAllByTestId('canvas-workspace-zoom').length).toBe(1);
    // **그릇이 바뀌었다**(2026-09-16 — 도구 띠): 도크 표면에서 그 한 칸이 사는 곳은 이제
    // 미리보기 제목 아래 띠다. 재는 것(한 표면에 하나)은 한 글자도 바뀌지 않았다.
    expect(screen.getByTestId('canvas-toolbar-panel').contains(zoomInput())).toBe(true);
  });
});

describe('줄과 도크는 같은 값을 읽고 쓴다 (SPEC-CANVAS-006 M10 · AC-10 (AY))', () => {
  beforeEach(() => {
    composedOuter = WS_OUTER;
    vi.stubGlobal('ResizeObserver', ComposedResizeObserver as unknown as typeof ResizeObserver);
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('줄에서 적은 배율이 **표면의 상자**를 다시 짓는다 — 줄은 제 상태를 갖지 않는다', () => {
    const emit = renderFixedPoint({ elements: [rect('a', { x: 100, y: 100, w: 200, h: 150 })] });

    // 기본 배율의 상자를 **먼저** 못박는다 — 바뀌지 않으면 이 시험은 틀린 이유로 통과한다.
    expect(zoomInput().value).toBe('75');
    expect(stageBoxSize()).toEqual(FP_STAGE);
    expect(stageOrigin()).toEqual(FP_ORIGIN);

    fireEvent.change(zoomInput(), { target: { value: '50' } });

    // 칸 · 상자 · 원점 셋이 **모두** 달라졌다(D8). 그리고 그 값은 도크에서 같은 값을
    // 적었을 때의 수와 **한 글자도 다르지 않다** — 값의 주인이 하나이기 때문이다.
    expect(zoomInput().value).toBe('50');
    expect(stageBoxSize()).toEqual(FP_STAGE_HALF);
    expect(stageOrigin()).toEqual(FP_ORIGIN_HALF);
    // 줄이 제 `useState` 를 들고 있다면 표면의 상자는 한 픽셀도 움직이지 않았을 것이다.

    // config 는 한 글자도 쓰이지 않는다 — 바뀌는 것은 시야뿐이다.
    expect(emit).not.toHaveBeenCalled();
  });

  it('범위를 죄고, 읽을 수 없는 입력에는 지금 값이 그대로 남는다 (도크와 같은 계약)', () => {
    renderFixedPoint();

    fireEvent.change(zoomInput(), { target: { value: '400' } });
    expect(zoomInput().value).toBe('100'); // 확대는 없다(불변식 I22)
    fireEvent.change(zoomInput(), { target: { value: '1' } });
    expect(zoomInput().value).toBe('25');
    fireEvent.change(zoomInput(), { target: { value: '' } });
    expect(zoomInput().value).toBe('25'); // 한 글자를 지우는 동안 화면이 무너지지 않는다
  });

  it('줄이 서 있기만 하면 프레임을 **0 건**, 배율을 바꾸면 **한 장** 부른다 (AC-E4)', () => {
    const clock = makeClock();
    renderFixedPoint({ scheduler: clock.scheduler });
    clock.flush(0);
    expect(clock.pending).toBe(0); // 유휴에 들었음을 **먼저** 단언한다
    const before = clock.requested;

    fireEvent.change(zoomInput(), { target: { value: '50' } });

    // 그리는 상자가 실제로 달라지므로 한 장이다 — 격자 간격 변경과 **같은 부류**이며
    // 새 깨우기 경로가 아니다. 도크로 바꿀 때와 같은 수다.
    expect(clock.requested).toBe(before + 1);
    clock.flush(16);
    expect(clock.pending).toBe(0);
  });
});

describe('줄은 포인터를 받고 빗나간 누름은 종전 그대로 흐른다 (M10 · AC-10 (AZ))', () => {
  /** 기본 배율에서 줄 **밑**에 앉는 요소. px 로 x −230.4..−172.8 · y 648..676.8 이다. */
  const UNDER_BAR: BoxGeometry = { x: -320, y: 900, w: 80, h: 40 };
  /** 그 요소의 중심을 오버레이 px 로 옮긴 자리(축척 18÷25 = 0.72). */
  const UNDER_BAR_CENTER = { x: -201.6, y: 662.4 };

  beforeEach(() => {
    composedOuter = WS_OUTER;
    vi.stubGlobal('ResizeObserver', ComposedResizeObserver as unknown as typeof ResizeObserver);
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('줄에는 `pointer-events-none` 이 **없다** — 그것이 이 컨트롤의 전부다', () => {
    renderFixedPoint();

    expect(zoomBar().className).not.toContain('pointer-events-none');
    // 표시 층 셋의 가드는 **그대로 통과한다** — 줄을 그 목록에 넣으면 그 가드가 곧 이
    // 기능을 금지한다(M7 의 닿는 면이 밟을 뻔한 그 함정 · 위험 R16).
    for (const id of ['canvas-region-scrim', 'canvas-region-bounds', 'canvas-workspace-grid']) {
      expect(screen.getByTestId(id).className, id).toContain('pointer-events-none');
    }
    expect(Object.keys(LAYER_CONTROLS)).not.toContain('canvas-workspace-zoom-bar');
  });

  it('위험 R8 가드를 **형상으로** 다시 쓴다 — 장식은 포인터를 먹지 않는다', () => {
    // 지키는 문장이 "그리는 층은 포인터를 먹지 않는다" 에서 **"장식은 포인터를 먹지
    // 않는다"** 로 좁아진다. 셋이 이 한 문장으로 갈린다: 표시 층 셋은 걸리고(칠하고
    // `aria-hidden` 이다), 닿는 면은 빠지며(`aria-hidden` 이지만 칠하지 않는다), 줄은
    // 빠진다(칠하지만 이름을 가진 컨트롤이라 `aria-hidden` 이 아니다).
    //
    // **세 이름을 손으로 적은 기존 두 시험은 지우지 않고 옆에 둔다** — 형상 판정이 잘못
    // 넓어지면 이름 쪽이 먼저 운다.
    for (const docked of [true, false] as const) {
      cleanup();
      renderFixedPoint({ docked });
      if (docked) fireEvent.click(gridToggle());

      const root = screen.getByTestId('canvas-edit-overlay');
      let checked = 0;
      for (const child of [...root.children]) {
        if (!(child instanceof HTMLElement)) continue;
        if (child.getAttribute('aria-hidden') !== 'true') continue;
        if (!paintsSomething(child)) continue;
        checked += 1;
        expect(child.className, child.dataset.testid).toContain('pointer-events-none');
      }
      expect(checked, `@docked=${docked}`).toBeGreaterThan(0);

      // 닿는 면은 `aria-hidden` 이지만 칠하지 않으므로 이 판정 밖이고, 그래서 포인터를 먹는다.
      expect(screen.getByTestId('canvas-workspace-hit').className).not.toContain(
        'pointer-events-none',
      );
      if (!docked) {
        // 줄은 칠하지만 `aria-hidden` 이 아니다 — 이름을 가진 컨트롤이기 때문이다.
        expect(zoomBar().getAttribute('aria-hidden')).toBeNull();
        expect(zoomBar().getAttribute('role')).toBe('group');
      }
    }
  });

  it('줄 **위**의 누름은 아래 도형을 고르지 않고, `defaultPrevented` 도 세우지 않는다', () => {
    renderFixedPoint({ elements: [rect('under', UNDER_BAR)] });
    stubOverlayRect(0, 0, FP_STAGE.width, FP_STAGE.height);

    // **먼저 그 좌표가 실제로 요소를 맞힘을 확인한다.** 이것이 없으면 이 시험은 끊음이
    // 없어도 통과한다 — 빗나간 좌표로 "안 골라졌다" 를 재는 초록이 된다.
    const onHit = sendToHit('pointerdown', UNDER_BAR_CENTER.x, UNDER_BAR_CENTER.y);
    expect(selectionText()).toBe('under');
    expect(onHit.defaultPrevented).toBe(true);
    send('pointerup', UNDER_BAR_CENTER.x, UNDER_BAR_CENTER.y);
    fireEvent.keyDown(overlayRoot(), { key: 'Escape' });

    // 이제 같은 좌표를 **줄 위에서** 누른다. 끊지 않으면 배율을 적으려는 손짓이 그 뒤
    // 도형을 고르고 이동 드래그까지 시작한다.
    const before = selectionText();
    const onBar = sendAt(zoomBar(), 'pointerdown', UNDER_BAR_CENTER.x, UNDER_BAR_CENTER.y);

    expect(selectionText()).toBe(before);
    // `preventDefault` 가 아니라 `stopPropagation` 이다 — 칸의 초점과 캐럿이 살아 있어야
    // 하고, `defaultPrevented` 는 `previewPan` 이 읽는 표시라 뜻이 번진다.
    expect(onBar.defaultPrevented).toBe(false);
  });

  it('줄을 **빗나간** 빈 자리 누름은 여전히 선택을 비운다 (불변식 I8)', () => {
    renderFixedPoint({ elements: [rect('far', { x: -300, y: -200, w: 100, h: 60 })] });
    stubOverlayRect(0, 0, FP_STAGE.width, FP_STAGE.height);

    // 저술 여백의 요소는 여전히 골라지고(AC-08 (AP) 가 그대로 통과한다)
    sendToHit('pointerdown', -180, -122.4);
    expect(selectionText()).toBe('far');

    // 저술 여백의 빈 자리는 여전히 선택을 비운다 — 줄이 생겼다고 이 성질이 달라지지
    // 않는다는 것이 I8 이다. 임자는 011 의 규칙대로 갈린다: 주 버튼은 팬(010 에서는
    // 사각형이었다), 나머지는 종전대로 흘러간다. 줄 **위**의 누름과 갈리는 자리가
    // 여기다(바로 위 시험).
    //
    // **011 이 비우는 시점을 뗌으로 옮겼다** — 그래서 뗌까지 쏜다. I8 이 말하는 "비운다"
    // 는 몸짓 하나(누르고 떼기)에 대한 문장이고, 그 몸짓은 여전히 선택을 비운다.
    const down = sendToHit('pointerdown', -60, -60);
    sendToHit('pointerup', -60, -60);
    expect(selectionText()).toBe('');
    expect(down.defaultPrevented).toBe(true);
    expect(sendToHit('pointerdown', -60, -60, { button: 2 }).defaultPrevented).toBe(false);
  });
});

describe('문은 하나이고 방향키는 두 주인을 갖지 않는다 (M10 · AC-10 (BA))', () => {
  const PICKED: BoxGeometry = { x: 100, y: 100, w: 200, h: 150 };

  beforeEach(() => {
    composedOuter = WS_OUTER;
    vi.stubGlobal('ResizeObserver', ComposedResizeObserver as unknown as typeof ResizeObserver);
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('탭 정지점의 **문**은 여전히 루트 하나다 — "정지점이 하나" 라는 뜻이 아니다', () => {
    const emit = renderFixedPoint({ elements: [rect('a', PICKED)] });
    stubOverlayRect(0, 0, FP_STAGE.width, FP_STAGE.height);

    const root = overlayRoot();
    expect(root.getAttribute('tabindex')).toBe('0');
    expect(root.getAttribute('aria-keyshortcuts')).not.toBeNull();
    // 줄의 칸은 루트의 **자손**이며, 탭 순서는 DOM 순서 그대로다.
    expect(root.contains(zoomInput())).toBe(true);
    expect(zoomInput().getAttribute('tabindex')).toBeNull();

    // **오버레이 안의 정지점이 하나라는 뜻이 아니다**(가정 A22). 손잡이 `<button>` 넷이
    // 이미 정지점이며, 그 사실을 여기 한 줄로 적어 다음 사람이 T15 를 잘못 읽지 않게 한다.
    send('pointerdown', 150, 130);
    send('pointerup', 150, 130);
    expect(selectionText()).toBe('a');
    expect(handleEl('se').tagName).toBe('BUTTON');
    expect(handleEl('se').getAttribute('tabindex')).toBeNull();
    expect(emit).toHaveBeenCalled();
  });

  it('줄의 DOM 자리 — 닿는 면 뒤 · 표시 층 셋 뒤 · 선택 윤곽선과 손잡이 **앞**', () => {
    renderFixedPoint({ elements: [rect('a', PICKED)] });
    stubOverlayRect(0, 0, FP_STAGE.width, FP_STAGE.height);

    const root = overlayRoot();
    const orderOf = (el: Element) => [...root.children].indexOf(el);

    // 선택이 **없을 때**의 자리를 먼저 잰다.
    const barIndexEmpty = orderOf(zoomBar());
    expect(root.firstElementChild).toBe(screen.getByTestId('canvas-workspace-hit'));
    for (const id of ['canvas-workspace-grid', 'canvas-region-scrim', 'canvas-region-bounds']) {
      expect(orderOf(screen.getByTestId(id)), id).toBeLessThan(barIndexEmpty);
    }

    send('pointerdown', 150, 130);
    send('pointerup', 150, 130);
    expect(selectionText()).toBe('a');

    // 손잡이가 줄보다 **뒤**에 있어야 왼쪽 아래 근처에서도 크기 조절이 그대로 시작된다 —
    // 줄이 마지막이면 손잡이를 덮어 조절이 죽는다(AC-08 (AQ) 가 닿는 면에 대해 지키는 그 성질).
    const barIndex = orderOf(zoomBar());
    expect(barIndex).toBeLessThan(orderOf(screen.getByTestId('canvas-selection-a')));
    expect(barIndex).toBeLessThan(orderOf(handleEl('sw')));
    // 그리고 **무엇을 골랐든 줄의 탭 자리가 달라지지 않는다** — 손잡이는 선택에 따라
    // 나타났다 사라지므로, 줄이 그 뒤에 있으면 배율의 탭 순서가 선택마다 달라진다.
    expect(barIndex).toBe(barIndexEmpty);
  });

  it('요소를 **골라 둔 채** 줄의 칸에서 방향키를 눌러도 고른 것이 움직이지 않는다 (위험 R26)', () => {
    const emit = renderFixedPoint({ elements: [rect('a', PICKED)] });
    stubOverlayRect(0, 0, FP_STAGE.width, FP_STAGE.height);

    // **선택이 있는 상태로 잰다.** 선택이 0 이면 `handleKeyDown` 이 일찍 돌아가므로 결함이
    // 없어도 통과한다 — 꺼져 있어서 통과하는 초록이다.
    send('pointerdown', 150, 130);
    send('pointerup', 150, 130);
    expect(selectionText()).toBe('a');
    emit.mockClear();

    // 루트에 직접 쏘면 오늘도 움직인다는 사실을 먼저 확인한다(재려는 경로가 살아 있다).
    sendKey('ArrowRight');
    expect(emittedGeometry(emit, 'a')).toMatchObject({ x: 101 });
    emit.mockClear();

    // 같은 키를 **줄의 칸에서** 쏘면 아무 일도 일어나지 않는다 — 줄이 제 자리에서 끊는다.
    fireEvent.keyDown(zoomInput(), { key: 'ArrowRight' });
    fireEvent.keyDown(zoomInput(), { key: 'ArrowUp' });
    expect(emit).not.toHaveBeenCalled();
  });

  it('`handleKeyDown` 에 표적 가드가 생기지 않았다 — 끊는 자리는 컨트롤 쪽이다', () => {
    // REQ-08 이 "한 줄도 바뀌지 않는다" 로 이름 적어 둔 경로다. 여기 가드가 생겼다면
    // 끊을 자리를 잘못 고른 것이다(도크가 포인터에 대해 이미 컨트롤 쪽을 골랐다).
    //
    // **보기 팬이 이 가드에 걸렸고, 걸린 쪽이 물러났다**(사용자 신고 2026-09-16). 팬의
    // 첫 구현은 Space 를 루트가 직접 초점을 든 동안에만 가져가려고 `event.target` 을
    // 보았는데, 그 한 줄이 곧 여기 적힌 "잘못 고른 자리" 였다. 물러날 수 있었던 근거는
    // 루트 안쪽의 초점 가능한 컨트롤이 손잡이 둘뿐이고 둘 다 Space 로 하는 일이 없다는
    // 사실이다 — 가드를 늦추지 않고 **구현이 규칙을 따랐다**.
    const source = readFileSync(join(__dirname, 'CanvasEditOverlay.tsx'), 'utf-8');
    const start = source.indexOf('const handleKeyDown');
    expect(start).toBeGreaterThan(0);
    const end = source.indexOf('\n  };', start);
    expect(end).toBeGreaterThan(start);
    const body = source.slice(start, end);
    expect(body).not.toMatch(/event\.target|\.closest\(|instanceof HTMLInputElement|tagName/);
  });
});

describe('줄의 자리와 가림 — 무엇을 덮고 무엇으로 되찾는가 (M10 · AC-10 (BB) · 위험 R25)', () => {
  const UNDER_BAR: BoxGeometry = { x: -320, y: 900, w: 80, h: 40 };

  beforeEach(() => {
    composedOuter = WS_OUTER;
    vi.stubGlobal('ResizeObserver', ComposedResizeObserver as unknown as typeof ResizeObserver);
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('작업 영역의 **왼쪽 아래**에 앉는다 — 상자는 손에 든 두 값 그대로다 (불변식 I10)', () => {
    renderFixedPoint();

    // 작업 영역은 오버레이 좌표로 `(-origin, box)` 다 — 격자·닿는 면이 쓰는 그 상자.
    const area = styleBox('canvas-workspace-hit');
    expect(area).toEqual({
      left: -FP_ORIGIN.x,
      top: -FP_ORIGIN.y,
      width: WS_OUTER.width,
      height: WS_OUTER.height,
    });

    // 줄의 **아래 변**이 작업 영역의 아래 변에서 8px 위에 앉고, 왼쪽 변이 왼쪽 변에서
    // 8px 오른쪽에 앉는다. `outer` 와 축척으로 되짚지 않는다 — 그것이 두 번째 측정원이다.
    const bar = zoomBar();
    expect(Number.parseFloat(bar.style.left)).toBe(area.left + FP_BAR_INSET);
    expect(Number.parseFloat(bar.style.top)).toBe(area.top + area.height - FP_BAR_INSET);
    expect(bar.style.transform).toBe('translateY(-100%)');
  });

  it('나머지 세 모서리에는 **임자가 있다** — 왼쪽 아래는 고른 것이 아니라 남은 것이다', () => {
    // 셋 다 `editMode && canEdit` 에서만 뜨는데 **그 상태가 곧 캔버스 편집이 가능한 상태**다.
    // 그래서 "지금은 비어 있다" 가 아니라 "언제나 함께 있다" 이며, 자리는 하나로 정해진다.
    const dashboard = readFileSync(join(__dirname, '../../DashboardPage.tsx'), 'utf-8');
    const dragHandle = readFileSync(join(__dirname, '../../DragHandle.tsx'), 'utf-8');
    expect(dragHandle).toMatch(/absolute inset-x-0 top-0[^"]*h-6/); // 위쪽 띠 전부
    expect(dashboard).toMatch(/absolute right-1 top-1 z-20/); // 오른쪽 위
    expect(dashboard).toMatch(/handles: \['se'\]/); // 오른쪽 아래 20×20
    // 왼쪽 **위**도 아니다 — 걷어낸 아이콘 띠가 살던 자리이고(사용 시험 "도형 팔레트
    // 크기가 너무 작음"), M8 뒤 옛 그림이 몰리는 자리다(위험 R19).
    renderFixedPoint();
    expect(Number.parseFloat(zoomBar().style.top)).toBeGreaterThan(FP_STAGE.height);
  });

  it('기본 배율에서는 **저술 여백만** 덮고, `z = 1.00` 에서는 경계의 한 모서리를 덮는다', () => {
    renderFixedPoint();

    // 출력 영역은 오버레이 좌표로 `(0,0)–(stage)` 다. 기본 배율에서 줄의 두 변은 **둘 다
    // 그 밖**이다 — 왼쪽으로 236px, 아래로 104px 나가 있다.
    const bar = () => zoomBar();
    expect(Number.parseFloat(bar().style.left)).toBe(-FP_ORIGIN.x + FP_BAR_INSET);
    expect(Number.parseFloat(bar().style.left)).toBeLessThan(0);
    expect(Number.parseFloat(bar().style.top)).toBeGreaterThan(FP_STAGE.height);

    // `z = 1.00` 에서는 저술 여백이 **0** 이라 작업 영역의 왼쪽 아래가 곧 출력 영역의
    // 왼쪽 아래다. 그때 줄은 경계의 한 모서리를 덮는다 — **대가를 숨기지 않는다.** 그
    // 배율은 사용자가 여백을 0 으로 하겠다고 **고른** 자리이므로, 덮을 여백이 없다는
    // 사실 자체가 그 선택의 결과다.
    fireEvent.change(zoomInput(), { target: { value: '100' } });

    expect(stageOrigin()).toEqual({ x: 0, y: 0 });
    expect(stageBoxSize()).toEqual({ width: WS_OUTER.width, height: WS_OUTER.height });
    expect(Number.parseFloat(bar().style.left)).toBe(FP_BAR_INSET);
    expect(Number.parseFloat(bar().style.top)).toBe(WS_OUTER.height - FP_BAR_INSET);
  });

  it('줄 아래의 요소는 그 자리에서 고를 수 없고, 회수 경로 **셋**이 새 기구 없이 선다', () => {
    const emit = renderFixedPoint({ elements: [rect('under', UNDER_BAR)] });
    stubOverlayRect(0, 0, FP_STAGE.width, FP_STAGE.height);

    // (0) 가린다는 사실을 **숨기지 않는다** — 줄 위를 누르면 아래 요소가 골라지지 않는다.
    const center = { x: -201.6, y: 662.4 }; // 축척 0.72 를 건 그 요소의 중심
    sendAt(zoomBar(), 'pointerdown', center.x, center.y);
    expect(selectionText()).toBe('');

    // (1) **배율 자체가 첫 회수 경로다.** 줄은 작업 영역 모서리에 고정이고 요소는 축척을
    //     따라 움직이므로, 배율을 낮추면 그림이 가운데로 물러나 줄 밑에서 빠져나온다 —
    //     가리는 손잡이가 곧 벗어나는 손잡이다. 수로 적는다: 요소 중심과 줄의 왼쪽 변
    //     사이가 34.4px 에서 311.6px 로, 아래 변과의 거리가 14.6px 에서 140.4px 로 벌어진다.
    const gapBefore = {
      x: center.x - Number.parseFloat(zoomBar().style.left),
      y: Number.parseFloat(zoomBar().style.top) - center.y,
    };
    fireEvent.change(zoomInput(), { target: { value: '50' } });
    const half = { x: (-320 + 40) * 0.48, y: (900 + 20) * 0.48 }; // 축척 12÷25
    const gapAfter = {
      x: half.x - Number.parseFloat(zoomBar().style.left),
      y: Number.parseFloat(zoomBar().style.top) - half.y,
    };
    expect(gapAfter.x).toBeGreaterThan(gapBefore.x);
    expect(gapAfter.y).toBeGreaterThan(gapBefore.y);
    expect(gapBefore.x).toBeCloseTo(34.4, 6);
    expect(gapBefore.y).toBeCloseTo(14.6, 6);
    expect(gapAfter.x).toBeCloseTo(311.6, 6);
    expect(gapAfter.y).toBeCloseTo(140.4, 6);

    // (2) **방향키 미세 이동** — 골라 둔 뒤에는 포인터가 필요 없다(루트가 문이다 · T15).
    fireEvent.change(zoomInput(), { target: { value: '75' } });
    sendToHit('pointerdown', center.x, center.y);
    send('pointerup', center.x, center.y);
    expect(selectionText()).toBe('under');
    emit.mockClear();
    sendKey('ArrowUp');
    expect(emittedGeometry(emit, 'under')).toMatchObject({ x: -320, y: 899 });

    // (3) 목록 편집기의 **요소 기하 수치 칸**은 이 SPEC 이 처음부터 세워 둔 회수 경로이며
    //     (불변식 I14 · 가정 A10) `CanvasElementsEditor` 의 시험과 AC-E10 이 진다.
  });

  it('도움말은 `aria-describedby` 로 이어진 **상시 문구**이고 클릭 팝오버가 아니다', () => {
    renderFixedPoint();

    const hint = screen.getByTestId('canvas-workspace-zoom-hint');
    expect(zoomInput().getAttribute('aria-describedby')).toBe(hint.id);
    expect(hint.textContent).toContain('dashboard.canvas.edit.workspaceZoomHint');
    expect(hint.className).toContain('sr-only');
    // `FieldHelp` 의 팝오버는 `absolute left-0 top-full w-64` 로 아래·오른쪽에 열리는데 줄은
    // 왼쪽 아래 모서리에 살고 표면 컨테이너에 `overflow-hidden` 이 있어 **잘린다.** 열어도
    // 보이지 않는 `?` 는 화면이 지키지 못할 약속이다.
    expect(
      within(zoomBar()).queryByRole('button', { name: 'property.fieldHelp.viewDescription' }),
    ).toBeNull();
    // 눈으로 보는 사람에게는 칸의 `title` 이 이름을 나른다 — 네이티브 툴팁은 DOM 이 아니라
    // 브라우저 크롬이라 `overflow-hidden` 에 잘리지 않는다.
    expect(zoomInput().getAttribute('title')).toBe('dashboard.canvas.edit.workspaceZoom');
  });

  it('줄에 이름이 있고, 새 키는 **하나**이며 나머지 셋은 그대로 다시 쓰인다', () => {
    renderFixedPoint();

    expect(zoomBar().getAttribute('role')).toBe('group');
    expect(zoomBar().getAttribute('aria-label')).toBe('dashboard.canvas.edit.workspaceZoomBar');
    // 나머지 셋은 **같은 칸이므로 같은 문구**를 그대로 쓴다 — 새 키를 만들면 두 표면의
    // 문구가 갈라진다.
    expect(zoomInput().getAttribute('aria-label')).toBe('dashboard.canvas.edit.workspaceZoom');
    expect(
      screen.getByTestId('canvas-workspace-zoom-suggestions').querySelectorAll('option').length,
    ).toBe(4);

    // 두 언어에 **모두** 있고 키 이름 안에 점이 없다(프로젝트 규약 — 이름에 점이 든 키는
    // 어떤 조회 경로로도 닿지 않는다).
    for (const messages of [koMessages, enMessages]) {
      const edit = (messages as unknown as Record<string, never>)['dashboard'] as unknown as {
        canvas: { edit: Record<string, string> };
      };
      for (const key of ['workspaceZoomBar', 'workspaceZoom', 'workspaceZoomOption', 'workspaceZoomHint']) {
        expect(typeof edit.canvas.edit[key], key).toBe('string');
        expect(key).not.toContain('.');
      }
    }
  });
});

// ============================================================================
// M12 — 견고성: 퇴화한 크기에서 이음매가 무엇을 내는가
// ============================================================================
//
// **이 절이 재는 것은 이음매다.** 상자 산술(`canvasWorkspace.test.ts`)과 표면
// (`CanvasSurface.test.ts`)은 퇴화 입력을 이미 각자 재고 있다 — 0 상자 · 비유한 상자 ·
// 손상된 간격 · 극단 종횡비에서 NaN 도 예외도 없다는 사실은 그 두 파일이 진다. 여기서
// 묻는 것은 그 다음 질문이다: **표면이 지은 그 값이 오버레이의 층으로 내려갔을 때 화면에
// 무엇이 적히는가.**
//
// 두 층이 각자 초록인데 이음매가 비어 있는 부류를 이 저장소는 이미 한 번 맞았다
// (0.4.0 결함 E — 편집기만 보면 통과, 렌더만 보면 통과). 006 의 상자는 **표면이 짓고
// 오버레이가 쓴다**. 그래서 퇴화 입력도 두 층을 **함께 세워** 재야 한다(시험 규율
// 0.4.0 행).
//
// 그리고 **줄을 세고 나서야 완전하다**(plan.md §M10 "M12(견고성)도 줄을 세고 나서야
// 완전하다"). 0 크기 작업 영역의 모서리에 앉는 컨트롤이 어디에 어떤 수로 앉는지는 M10
// 이전에는 물을 수 없던 질문이다.

describe('퇴화한 크기에서 이음매가 유한한 수만 내린다 (M12 · AC-E5)', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', ComposedResizeObserver as unknown as typeof ResizeObserver);
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  /** 층의 CSS 자리·크기 넷이 전부 유한한 수인가. `NaN` 은 `Number.parseFloat` 로 새어 온다. */
  function finiteBox(testId: string): { left: number; top: number; width: number; height: number } {
    const box = styleBox(testId);
    for (const [axis, value] of Object.entries(box)) {
      expect(Number.isFinite(value), `${testId}.${axis} = ${value}`).toBe(true);
    }
    return box;
  }

  it('아직 재지 못한 상자(0)에서 층 둘이 **0** 이고 음수 크기가 되지 않는다', () => {
    // 닿는 면과 격자 상자는 `(-origin, box)` 를 그대로 쓴다. 상자가 0 이면 원점도 0 이므로
    // 두 층도 0 이며, 그 처리는 `workspaceBox` 가 이미 소유한다 — 여기서 확인하는 것은
    // 그 소유가 **화면까지 이어지는가**다. 음수 크기는 CSS 에서 무시되어 층이 통째로
    // 사라지고, 그때 닿는 면이 사라지면 여백의 누름이 다시 죽는다(REQ-08 · 불변식 I18).
    composedOuter = { width: 0, height: 0 };
    renderWorkspace([], true);

    for (const id of ['canvas-workspace-hit', 'canvas-workspace-grid']) {
      const box = finiteBox(id);
      expect(box.width, `${id}.width`).toBe(0);
      expect(box.height, `${id}.height`).toBe(0);
      // `-0` 도 0 이다 — 원점이 0 이므로 `-origin` 이 `-0` 이 되는 갈래를 함께 덮는다.
      expect(Math.abs(box.left), `${id}.left`).toBe(0);
      expect(Math.abs(box.top), `${id}.top`).toBe(0);
      expect(box.width, `${id}.width 음수 아님`).toBeGreaterThanOrEqual(0);
      expect(box.height, `${id}.height 음수 아님`).toBeGreaterThanOrEqual(0);
    }
  });

  it('그 상자에서도 흐림과 경계는 **DOM 에 그대로 있다** — 격자와 다른 축이다', () => {
    // 둘은 `inset-0` 이라 오버레이 루트(=출력 영역)를 따라가고 제 수를 갖지 않는다.
    // 크기가 0 이라고 걷어내면 "어디가 패널에 나오나" 의 답이 측정 전에 깜빡인다.
    composedOuter = { width: 0, height: 0 };
    renderWorkspace([], true);

    for (const id of ['canvas-region-scrim', 'canvas-region-bounds']) {
      const node = screen.getByTestId(id);
      expect(node, id).toBeTruthy();
      expect(node.className, id).toContain('inset-0');
      // 제 수를 갖지 않으므로 NaN 이 적힐 자리도 없다.
      expect(node.style.width, id).toBe('');
      expect(node.style.height, id).toBe('');
    }
  });

  it('그 상자에서 화면 전체에 `NaN` · `Infinity` 가 한 글자도 적히지 않는다', () => {
    // 층을 이름으로 세는 대신 오버레이가 낸 **모든** 인라인 style 을 훑는다. 다음 사람이
    // 층을 하나 더 더하고 퇴화 갈래를 잊으면 여기서 걸린다.
    composedOuter = { width: 0, height: 0 };
    renderWorkspace([rect('a', { x: 10, y: 10, w: 20, h: 20 })], true);

    const root = screen.getByTestId('canvas-edit-overlay');
    for (const node of [root, ...root.querySelectorAll<HTMLElement>('[style]')]) {
      const css = node.getAttribute('style') ?? '';
      expect(css, node.dataset.testid ?? css.slice(0, 40)).not.toMatch(/NaN|Infinity/);
    }
  });

  it('극단적으로 납작한 · 긴 상자에서도 출력 영역이 작업 영역을 넘지 않는다', () => {
    // 축척이 뒤집히면 출력 영역이 작업 영역보다 커져 "줄여서 여백을 만든다" 가 거짓이
    // 되고, 그때 저술 여백이 음수가 되어 원점도 음수가 된다.
    for (const outer of [
      { width: 2000, height: 50 },
      { width: 50, height: 2000 },
      { width: 3, height: 3 },
    ]) {
      cleanup();
      composedOuter = outer;
      renderWorkspace([], true);

      const area = finiteBox('canvas-workspace-hit');
      const stage = stageBoxSize();
      const label = `${outer.width}x${outer.height}`;
      expect(Number.isFinite(stage.width), label).toBe(true);
      expect(Number.isFinite(stage.height), label).toBe(true);
      expect(stage.width, label).toBeLessThanOrEqual(area.width);
      expect(stage.height, label).toBeLessThanOrEqual(area.height);
      // 원점은 `-left` 다. 작업 영역이 출력 영역을 감싸므로 그 값이 0 이상이어야 한다.
      expect(-area.left, `${label} origin.x`).toBeGreaterThanOrEqual(0);
      expect(-area.top, `${label} origin.y`).toBeGreaterThanOrEqual(0);
    }
  });

  it('한 칸이 1px 도 되지 않으면 격자는 꺼지되 **그림도 경계도 그대로다** (AC-E5)', () => {
    // `stageLattice` 가 정수화하지 않고 소수 축척 하나를 그대로 쓰는 그 갈래다. 정수화하면
    // 칸이 0 이 되고 `repeating-linear-gradient` 의 주기가 0 이 되어 격자가 **꽉 찬
    // 사각형**이 된다 — 참조선이라고 내놓을 수 없는 그림이다.
    composedOuter = { width: 10, height: 8 };
    renderWorkspace([], true);

    fireEvent.click(screen.getByTestId('canvas-grid-toggle'));

    // 격자는 그려지지 않는다 — `enabled` 게이트가 `cell >= 1` 을 요구하고, `PanelEditGrid`
    // 는 꺼지면 `null` 을 돌려주므로 그 노드 자체가 DOM 에 없다.
    expect(screen.queryByTestId('panel-edit-grid')).toBeNull();
    // 그럼에도 격자를 **담는 상자**는 남아 작업 영역을 그대로 덮는다(닿는 면과 같은 수).
    expect(styleBox('canvas-workspace-grid')).toEqual(styleBox('canvas-workspace-hit'));
    // 그럼에도 출력 영역은 살아 있고 경계·흐림이 그것을 두른다.
    expect(stageBoxSize().width).toBeGreaterThan(0);
    expect(screen.getByTestId('canvas-region-bounds')).toBeTruthy();
    expect(screen.getByTestId('canvas-region-scrim')).toBeTruthy();
  });
});

describe('퇴화한 상자에서도 배율 줄이 유한한 자리에 앉는다 (M12 · 0.7.0 이 더한 몫)', () => {
  beforeEach(() => {
    vi.stubGlobal('ResizeObserver', ComposedResizeObserver as unknown as typeof ResizeObserver);
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('0 크기 작업 영역에서도 줄이 서고 두 수가 유한하다 — 사라지지도 NaN 도 아니다', () => {
    // 줄의 자리는 `(-origin.x + 8, -origin.y + box.height - 8)` 이다. 상자가 0 이면 그 값이
    // `(8, -8)` 로 **작업 영역 위쪽 밖**이 되는데, 그것은 측정 전 한 프레임의 모습이며
    // 곧 첫 측정이 덮는다. 여기서 못박는 것은 **그 갈래가 수를 잃지 않는다**는 사실이다 —
    // 줄이 DOM 에서 사라지면 첫 측정 뒤 다시 마운트되어 초점이 튄다.
    composedOuter = { width: 0, height: 0 };
    renderWorkspace([], true, { docked: false });

    const bar = zoomBar();
    const left = Number.parseFloat(bar.style.left);
    const top = Number.parseFloat(bar.style.top);
    expect(Number.isFinite(left)).toBe(true);
    expect(Number.isFinite(top)).toBe(true);
    expect(left).toBe(FP_BAR_INSET);
    expect(top).toBe(-FP_BAR_INSET);
    expect(bar.style.transform).toBe('translateY(-100%)');
  });

  it('그 상태에서 배율을 바꿔도 예외가 없고 수가 유한하게 따라온다', () => {
    // 0 상자에서 배율을 바꾸면 `reduced` 도 0 이고 `cell` 도 0 이 되는 갈래로 들어간다.
    // 그 곱셈이 NaN 을 내면 화면에 `NaNpx` 가 적힌다.
    composedOuter = { width: 0, height: 0 };
    renderWorkspace([], true, { docked: false });

    fireEvent.change(zoomInput(), { target: { value: '25' } });

    expect(zoomInput().value).toBe('25');
    for (const id of ['canvas-workspace-hit', 'canvas-workspace-grid']) {
      const css = screen.getByTestId(id).getAttribute('style') ?? '';
      expect(css, id).not.toMatch(/NaN|Infinity/);
    }
    expect(Number.isFinite(Number.parseFloat(zoomBar().style.top))).toBe(true);
  });

  it('납작한 상자에서도 줄은 작업 영역 **안쪽**에 앉는다 — 왼쪽 아래가 사라지지 않는다', () => {
    composedOuter = { width: 2000, height: 50 };
    renderWorkspace([], true, { docked: false });

    const area = styleBox('canvas-workspace-hit');
    const bar = zoomBar();
    expect(Number.parseFloat(bar.style.left)).toBe(area.left + FP_BAR_INSET);
    expect(Number.parseFloat(bar.style.top)).toBe(area.top + area.height - FP_BAR_INSET);
  });
});

describe('유도한 캔버스는 간격의 배수가 아니다 — 그래도 선 자리는 정수다 (M12 · 가정 A19)', () => {
  beforeEach(() => {
    composedOuter = WS_OUTER;
    vi.stubGlobal('ResizeObserver', ComposedResizeObserver as unknown as typeof ResizeObserver);
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('`stage` 는 소수인데 `origin` 과 `cell` 은 정수이고, 두 여백 차가 **2px 미만**이다', () => {
    // 이 셋이 A19 의 전부다. `stage` 가 소수여도(1259.28 × 573.12) 격자선이 서는 자리
    // `origin + k × cell` 은 전부 정수여야 한다 — 0.9.0 이 고친 "선이 두 장치 픽셀에
    // 걸친다" 를 막는 성질이 그것이다.
    //
    // **여백의 상한이 1px 이 아니라 2px 인 이유를 수로 적는다.** 자투리는
    // `outer − stage − 2 × floor((outer − stage) / 2)` 이고, `stage` 가 소수이면 그 값이
    // 1 을 넘을 수 있다: 가로는 `1749 − 1259.28 − 488 = 1.72`. 정수 `stage` 를 가정한
    // 1px 상한(`canvasWorkspace.test.ts` 의 5:4 짝이 쓰는 그 상한)은 이 자리에서 **거짓**
    // 이며, 그것이 0.5.0 이 상한을 다시 적은 이유다.
    renderWorkspace([], true, { canvas: FP_CANVAS });

    const area = styleBox('canvas-workspace-hit');
    const stage = stageBoxSize();
    const origin = { x: -area.left, y: -area.top };

    expect(stage).toEqual(FP_STAGE);
    expect(Number.isInteger(stage.width)).toBe(false);
    expect(Number.isInteger(stage.height)).toBe(false);

    expect(origin).toEqual(FP_ORIGIN);
    expect(Number.isInteger(origin.x)).toBe(true);
    expect(Number.isInteger(origin.y)).toBe(true);

    const slackX = area.width - stage.width - 2 * origin.x;
    const slackY = area.height - stage.height - 2 * origin.y;
    expect(slackX).toBeCloseTo(1.72, 10);
    expect(slackY).toBeCloseTo(0.88, 10);
    for (const slack of [slackX, slackY]) {
      expect(slack).toBeGreaterThanOrEqual(0);
      expect(slack).toBeLessThan(2);
    }
  });

  it('격자 한 칸은 정수이고 선의 첫 자리는 출력 영역의 원점이다 (불변식 I5)', () => {
    renderWorkspace([], true, { canvas: FP_CANVAS });
    fireEvent.click(screen.getByTestId('canvas-grid-toggle'));

    const size = screen.getByTestId('panel-edit-grid').style.backgroundSize;
    const m = /^([\d.]+)px ([\d.]+)px$/.exec(size);
    expect(m, `격자 칸을 px 로 읽지 못했다: ${size}`).not.toBeNull();
    expect(Number(m![1])).toBe(18);
    expect(Number(m![2])).toBe(18);
    // 위상은 출력 영역의 원점이다 — 작업 영역의 왼쪽 위가 아니다(위험 R3).
    expect(gridPhase()).toEqual({ x: 0, y: 0 });
  });

  it('`z = 1.00` 에서는 여백이 **둘 다 0** 이다 — 자투리는 배율과 무관하다', () => {
    // 자투리 `outer − stage − 2·floor(…)` 는 배율을 인자로 갖지 않는다. 그럼에도
    // `z = 1.00` 은 `stage = outer` 라 그 식이 0 이 되는 유일한 자리다(불변식 I21 의
    // 명시된 예외 — `canvas === outer` 인 고정점에서만 성립한다).
    renderWorkspace([], true, { docked: false, canvas: FP_CANVAS });
    fireEvent.change(zoomInput(), { target: { value: '100' } });

    const area = styleBox('canvas-workspace-hit');
    const stage = stageBoxSize();
    expect(stage).toEqual({ width: WS_OUTER.width, height: WS_OUTER.height });
    expect(Math.abs(area.left)).toBe(0);
    expect(Math.abs(area.top)).toBe(0);
    expect(area.width - stage.width).toBe(0);
    expect(area.height - stage.height).toBe(0);
  });
});
