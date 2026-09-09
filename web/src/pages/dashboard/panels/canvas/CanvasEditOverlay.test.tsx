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
import { describe, expect, it, vi, afterEach } from 'vitest';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';

// i18n 은 키를 그대로 돌려준다(I18nProvider 없이 렌더 가능 — CanvasPanel.test.tsx 선례).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { BoxGeometry, CanvasElement } from './canvasConfig';
import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasEditOverlay from './CanvasEditOverlay';
import {
  CanvasEditSelectionContext,
  useCanvasEditSelectionState,
} from './canvasEditContext';
import { appendElement } from './canvasElementFactory';
import type { StageSize } from './canvasGeometry';

// --- 고정 입력 -----------------------------------------------------------

/** 기본 스테이지 — 축이 서로 달라야 축을 뒤바꾼 계산이 드러난다. */
const STAGE: StageSize = { width: 200, height: 100 };

function rect(id: string, geometry: BoxGeometry): CanvasElement {
  return { id, kind: 'rect', geometry, style: {} };
}

// --- 시험 대상 하네스 ----------------------------------------------------

interface HarnessProps {
  enabled?: boolean;
  elements: readonly CanvasElement[];
  stage?: StageSize;
  textWidths?: Record<string, number>;
  onElementsChange: (next: CanvasElement[]) => void;
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
            stage={stage}
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
    render(<Harness enabled={false} elements={[rect('a', { x: 0, y: 0, w: 1, h: 1 })]} onElementsChange={vi.fn()} />);
    expect(screen.queryByTestId('canvas-edit-overlay')).toBeNull();
  });

  it('편집을 끄면 골라져 있던 것이 풀린다', () => {
    const elements = [rect('a', { x: 0, y: 0, w: 1, h: 1 })];
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
        elements={[rect('a', { x: 0, y: 0, w: 1, h: 1 }), rect('b', { x: 0, y: 0, w: 1, h: 1 })]}
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
    render(<Harness elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]} onElementsChange={vi.fn()} />);
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
          rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 }),
          rect('b', { x: 0.5, y: 0.5, w: 0.2, h: 0.2 }),
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
          rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 }),
          rect('b', { x: 0.5, y: 0.5, w: 0.2, h: 0.2 }),
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
      geometry: { x: 0, y: 0, w: 1, h: 1 },
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
  it('맞는 것이 없으면 선택만 비우고 이벤트를 소비하지 않는다', () => {
    const parent = vi.fn();
    render(
      <Harness
        elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]}
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

    expect(selectionText()).toBe('');
    expect(evt.defaultPrevented).toBe(false);
    expect(parent).toHaveBeenCalledTimes(1);
  });

  it('요소 위 누름은 그 이벤트만 소비해 상위가 함께 반응하지 않는다', () => {
    const parent = vi.fn();
    render(
      <Harness
        elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]}
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
    render(<Harness elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 30, 15);
    send('pointermove', 50, 25);
    await nextFrame();

    // 폭 200 에서 20px = 0.1, 높이 100 에서 10px = 0.1. 크기는 그대로다.
    expect(emittedGeometry(emit, 'a')).toEqual({ x: 0.2, y: 0.2, w: 0.2, h: 0.2 });
  });

  it('한 프레임 사이의 여러 이동은 한 번만 쓰인다', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 30, 15);
    send('pointermove', 35, 17);
    send('pointermove', 45, 21);
    send('pointermove', 50, 25);
    await nextFrame();

    expect(emit).toHaveBeenCalledTimes(1);
    // 합류해도 결과는 **마지막 자리**다(누적이 아니라 시작점 대비 절대량이다).
    expect(emittedGeometry(emit, 'a')).toEqual({ x: 0.2, y: 0.2, w: 0.2, h: 0.2 });
  });

  it('요소가 둘 이상 골라져 있으면 같은 델타가 전부에 적용된다', async () => {
    const emit = vi.fn();
    render(
      <Harness
        elements={[
          rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 }),
          rect('b', { x: 0.5, y: 0.5, w: 0.2, h: 0.2 }),
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

    expect(emittedGeometry(emit, 'a')).toEqual({ x: 0.2, y: 0.2, w: 0.2, h: 0.2 });
    expect(emittedGeometry(emit, 'b')).toEqual({ x: 0.6, y: 0.6, w: 0.2, h: 0.2 });
  });

  it('보조키를 누른 채로는 끌리지 않는다 (고르기 전용 조작이다)', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 30, 15, { shiftKey: true });
    send('pointermove', 50, 25);
    await nextFrame();

    expect(emit).not.toHaveBeenCalled();
  });

  it('빈 지점에서 시작한 움직임은 아무것도 옮기지 않는다', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]} onElementsChange={emit} />);
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
        elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]}
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
    render(<Harness elements={[rect('a', { x: 0.8, y: 0.1, w: 0.2, h: 0.2 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 170, 15);
    send('pointermove', 370, 15);
    await nextFrame();

    expect(emittedGeometry(emit, 'a')).toEqual({ x: 1.8, y: 0.1, w: 0.2, h: 0.2 });
  });

  it('음수 쪽으로 나가도 0 으로 붙잡히지 않는다', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 30, 15);
    send('pointermove', -70, -35);
    await nextFrame();

    expect(emittedGeometry(emit, 'a')).toEqual({ x: -0.4, y: -0.4, w: 0.2, h: 0.2 });
  });
});

// --- 확정 (AC-03) ---------------------------------------------------------

describe('드래그의 끝 — 마지막 유효 위치를 확정한다 (AC-03)', () => {
  it('pointerup 은 그 이벤트의 자리로 확정한다', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 30, 15);
    send('pointerup', 50, 25);

    expect(emittedGeometry(emit, 'a')).toEqual({ x: 0.2, y: 0.2, w: 0.2, h: 0.2 });
  });

  it('pointercancel 은 되돌리지 않고 마지막 자리를 그대로 확정한다', () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 30, 15);
    // 프레임을 기다리지 않고 끊는다 — 대기 중이던 이동이 사라지면 안 된다.
    send('pointermove', 50, 25);
    send('pointercancel', 0, 0);

    expect(emittedGeometry(emit, 'a')).toEqual({ x: 0.2, y: 0.2, w: 0.2, h: 0.2 });
  });

  it('손을 뗀 뒤의 움직임은 더 이상 옮기지 않는다', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]} onElementsChange={emit} />);
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
    render(<Harness elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.4 })]} onElementsChange={vi.fn()} />);
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
      geometry: { x1: 0.1, y1: 0.2, x2: 0.5, y2: 0.6 },
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
      geometry: { x: 0.5, y: 0.5 },
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
      geometry: { x: 0.5, y: 0.5 },
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
    render(<Harness elements={[rect('a', { x: 0.5, y: 0.5, w: 0, h: 0 })]} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    send('pointerdown', 100, 50);

    const outline = screen.getByTestId('canvas-selection-a');
    expect(outline.style.left).toBe('100px');
    expect(outline.style.width).toBe('2px');
    expect(outline.style.height).toBe('2px');
  });

  it('음수 크기 박스도 양수 범위로 펴서 두른다', () => {
    render(<Harness elements={[rect('a', { x: 0.5, y: 0.5, w: -0.2, h: -0.2 })]} onElementsChange={vi.fn()} />);
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
      <Harness elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]} onElementsChange={emit} />,
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
    render(<Harness elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]} onElementsChange={emit} />);
    stubOverlayRect();
    const api = stubPointerCapture();

    send('pointerdown', 30, 15, {}, 7);
    expect(api.setPointerCapture).toHaveBeenCalledWith(7);

    send('pointerup', 50, 25, {}, 7);
    expect(api.hasPointerCapture).toHaveBeenCalledWith(7);
    expect(api.releasePointerCapture).toHaveBeenCalledWith(7);
    expect(emittedGeometry(emit, 'a')).toEqual({ x: 0.2, y: 0.2, w: 0.2, h: 0.2 });
  });

  it('잡은 적이 없으면 놓아 달라고 하지 않는다 (브라우저가 예외를 던진다)', () => {
    render(<Harness elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    const api = stubPointerCapture(false);

    send('pointerdown', 30, 15, {}, 7);
    send('pointerup', 50, 25, {}, 7);

    expect(api.releasePointerCapture).not.toHaveBeenCalled();
  });

  it('다른 포인터의 이동·놓기·취소는 무시한다 (멀티터치 방어)', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]} onElementsChange={emit} />);
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
    render(<Harness elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]} onElementsChange={emit} />);
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
      geometry: { x: 0.5, y: 0.5 },
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
      geometry: { x: 0.5, y: 0.5 },
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
    render(<Harness elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]} onElementsChange={emit} />);
    stubOverlayRect();

    send('pointerdown', 30, 15);
    send('pointercancel', 0, 0);

    expect(emit).not.toHaveBeenCalled();
  });

  it('끄는 도중 스테이지가 무너지면(높이 0) 쓰지 않는다', async () => {
    const emit = vi.fn();
    const elements = [rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })];
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
  CANVAS_GRID_STEP_PERCENT,
  squareGridSteps,
} from './canvasEditArrange';
import {
  BOX_HANDLE_IDS,
  CANVAS_FONT_SIZE_MAX,
  CANVAS_FONT_SIZE_MIN,
  handlePositions,
} from './canvasEditGeometry';

// --- 고정 입력 -----------------------------------------------------------

/** 스테이지 위 px 상자가 (40,20,80,40) 이 되는 사각형 — 여덟 핸들 자리가 모두 정수다. */
const BOX_GEOMETRY: BoxGeometry = { x: 0.2, y: 0.2, w: 0.4, h: 0.4 };

const LINE_ELEMENT: CanvasElement = {
  id: 'l',
  kind: 'line',
  geometry: { x1: 0.1, y1: 0.2, x2: 0.5, y2: 0.6 },
  style: {},
};

function textElement(id = 't', fontSize = 20): CanvasElement {
  return { id, kind: 'text', geometry: { x: 0.5, y: 0.5 }, style: { fontSize }, text: 'abc' };
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
        elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 }), rect('b', { x: 0.5, y: 0.5, w: 0.2, h: 0.2 })]}
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

    for (const handle of handlePositions(element, STAGE)) {
      const node = handleEl(handle.id);
      expect(node.style.left).toBe(`${handle.point.x}px`);
      expect(node.style.top).toBe(`${handle.point.y}px`);
    }
  });

  it('선 끝점 핸들이 projectLine 결과에 앉는다', () => {
    render(<Harness elements={[LINE_ELEMENT]} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    send('pointerdown', 60, 40);

    for (const handle of handlePositions(LINE_ELEMENT, STAGE)) {
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

    const [handle] = handlePositions(element, STAGE, { measuredWidth: 30 });
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

    for (const handle of handlePositions(elements[0]!, wider)) {
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

    expectBox(emittedGeometry(emit, 'a'), { x: 0.2, y: 0.2, w: 0.6, h: 0.6 });
  });

  it('왼쪽 위 모서리는 왼쪽 변과 위 변을 함께 옮긴다', async () => {
    const emit = renderSelectedBox();
    await dragHandle('nw', { x: 20, y: 10 });

    expectBox(emittedGeometry(emit, 'a'), { x: 0.1, y: 0.1, w: 0.5, h: 0.5 });
  });

  it('오른쪽 변 핸들은 세로를 건드리지 않는다', async () => {
    const emit = renderSelectedBox();
    await dragHandle('e', { x: 180, y: 90 });

    expectBox(emittedGeometry(emit, 'a'), { x: 0.2, y: 0.2, w: 0.7, h: 0.4 });
  });

  it('위쪽 변 핸들은 가로를 건드리지 않는다', async () => {
    const emit = renderSelectedBox();
    await dragHandle('n', { x: 10, y: 5 });

    expectBox(emittedGeometry(emit, 'a'), { x: 0.2, y: 0.05, w: 0.4, h: 0.55 });
  });

  it('반대 모서리 너머로 끌면 박스를 정규화해 **음수 크기를 저장하지 않는다** (AC-E6)', async () => {
    const emit = renderSelectedBox();
    // 오른쪽 아래를 왼쪽 위 바깥까지 끈다 — 뒤집힌 뒤에도 잡은 손잡이가 커서 아래에 남아야 한다.
    await dragHandle('se', { x: -40, y: -20 });

    const stored = emittedGeometry(emit, 'a') as BoxGeometry;
    expect(stored.w).toBeGreaterThan(0);
    expect(stored.h).toBeGreaterThan(0);
    // 좌표는 음수로 남는다 — 스테이지 밖은 합법이므로 clamp 하지 않는다(AC-E5).
    expectBox(stored, { x: -0.2, y: -0.2, w: 0.4, h: 0.4 });
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
    expectBox(emittedGeometry(emit, 'a'), { x: 0.2, y: 0.2, w: 0.6, h: 0.6 });
  });

  it('손을 떼면 그 자리로 확정한다', () => {
    const emit = renderSelectedBox();
    sendAt(handleEl('se'), 'pointerdown', 120, 60);
    send('pointerup', 160, 80);

    expectBox(emittedGeometry(emit, 'a'), { x: 0.2, y: 0.2, w: 0.6, h: 0.6 });
  });
});

describe('Shift 는 모서리에서 종횡비를 지킨다 (AC-04)', () => {
  /** 가로:세로 = 2:1 인 사각형. 비가 1 이면 유지 여부를 구별할 수 없다. */
  const WIDE: BoxGeometry = { x: 0.2, y: 0.2, w: 0.4, h: 0.2 };

  it('Shift 없이 끌면 비가 자유롭게 바뀐다', async () => {
    const emit = renderSelectedBox(WIDE);
    await dragHandle('se', { x: 160, y: 90 });

    expectBox(emittedGeometry(emit, 'a'), { x: 0.2, y: 0.2, w: 0.6, h: 0.7 });
  });

  it('Shift 를 누른 채 모서리를 끌면 원래 비(2:1)가 유지된다', async () => {
    const emit = renderSelectedBox(WIDE);
    await dragHandle('se', { x: 160, y: 90 }, { shiftKey: true });

    const stored = emittedGeometry(emit, 'a') as BoxGeometry;
    expect(stored.w / stored.h).toBeCloseTo(2, 10);
    expectBox(stored, { x: 0.2, y: 0.2, w: 1.4, h: 0.7 });
  });

  it('끌던 도중에 Shift 를 눌러도 그 자리에서 죄인다 (잡을 때 값을 얼리지 않는다)', async () => {
    const emit = renderSelectedBox(WIDE);
    sendAt(handleEl('se'), 'pointerdown', 120, 40);
    send('pointermove', 160, 90);
    await nextFrame();
    expectBox(emittedGeometry(emit, 'a'), { x: 0.2, y: 0.2, w: 0.6, h: 0.7 });

    send('pointermove', 160, 90, { shiftKey: true });
    await nextFrame();

    expectBox(emittedGeometry(emit, 'a'), { x: 0.2, y: 0.2, w: 1.4, h: 0.7 });
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
    expect(g.x1).toBeCloseTo(0.1, 10);
    expect(g.y1).toBeCloseTo(0.2, 10);
    expect(g.x2).toBeCloseTo(0.9, 10);
    expect(g.y2).toBeCloseTo(0.2, 10);
  });

  it('시작점 핸들은 시작점만 옮긴다', async () => {
    const emit = renderSelectedLine();
    await dragHandle('p1', { x: 20, y: 80 });

    const g = emittedGeometry(emit, 'l') as LineGeometry;
    expect(g.x1).toBeCloseTo(0.1, 10);
    expect(g.y1).toBeCloseTo(0.8, 10);
    expect(g.x2).toBeCloseTo(0.5, 10);
    expect(g.y2).toBeCloseTo(0.6, 10);
  });

  it('Shift 는 끝점 방향을 0°/45°/90° 로 죈다', async () => {
    const flat: CanvasElement = {
      id: 'l',
      kind: 'line',
      geometry: { x1: 0.5, y1: 0.5, x2: 0.6, y2: 0.5 },
      style: {},
    };
    const emit = vi.fn();
    render(<Harness elements={[flat]} onElementsChange={emit} />);
    stubOverlayRect();
    send('pointerdown', 110, 50);

    // 살짝 아래로 벌어진 방향 → 0° 로 죄여 y 가 시작점과 같아진다.
    await dragHandle('p2', { x: 160, y: 55 }, { shiftKey: true });
    expect((emittedGeometry(emit, 'l') as LineGeometry).y2).toBeCloseTo(0.5, 10);

    // 45° 근처 방향 → 두 축의 변위가 같아진다.
    await dragHandle('p2', { x: 160, y: 70 }, { shiftKey: true });
    const g = emittedGeometry(emit, 'l') as LineGeometry;
    expect(g.x2 - g.x1).toBeCloseTo(g.y2 - g.y1, 10);
  });

  it('Shift 가 없으면 끌린 자리를 그대로 쓴다', async () => {
    const emit = renderSelectedLine();
    await dragHandle('p2', { x: 160, y: 55 });

    const g = emittedGeometry(emit, 'l') as LineGeometry;
    expect(g.x2).toBeCloseTo(0.8, 10);
    expect(g.y2).toBeCloseTo(0.55, 10);
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
    expect(el?.geometry as PointGeometry).toEqual({ x: 0.5, y: 0.5 });
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
      geometry: { x: 0.5, y: 0.5 },
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
      geometry: { x: 0.1, y: 0.1 },
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
      geometry: { x: 0.5, y: 0.5 },
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
        elements={[rect('a', BOX_GEOMETRY), rect('b', { x: 0.55, y: 0.55, w: 0.2, h: 0.2 })]}
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
        elements={[rect('a', BOX_GEOMETRY), rect('b', { x: 0.55, y: 0.55, w: 0.2, h: 0.2 })]}
        onElementsChange={emit}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 80, 40);
    await dragHandle('se', { x: 160, y: 80 });

    expect(emittedGeometry(emit, 'b')).toEqual({ x: 0.55, y: 0.55, w: 0.2, h: 0.2 });
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
        elements={[rect('a', BOX_GEOMETRY), rect('b', { x: 0.75, y: 0.05, w: 0.2, h: 0.2 })]}
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
  const [elements, setElements] = useState<readonly CanvasElement[]>(initial);
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <span data-testid="selection">{[...state.selection].join(',')}</span>
      <span data-testid="dump">{JSON.stringify(elements)}</span>
      <CanvasEditDockRegion enabled>
        <CanvasEditOverlay
          enabled
          elements={elements}
          stage={STAGE}
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
    expect(geos[1]).toBe(JSON.stringify({ x: 0.15, y: 0.15, w: 0.2, h: 0.2 }));
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
    render(<PaletteHarness initial={[rect('a', { x: 0, y: 0, w: 1, h: 1 })]} />);
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
    expect(moved.x).toBeCloseTo(0.2, 6);
    expect(moved.y).toBeCloseTo(0.2, 6);
  });
});


// --- 격자 붙임 · 정렬 · z-order (T12 · T13 · T14) --------------------------
//
// 이 절의 중심은 **위험 R6** 이다. `snapOffsetToGrid` 의 오프셋 상한 기본값이 ±40 이라,
// 래퍼가 상한을 무한대로 넘기지 않으면 캔버스 드래그가 스테이지의 40% 지점에서 조용히
// 멈춘다 — 오류가 아니라 "왜 더 안 가지" 로만 보인다. 그래서 아래에는 **40% 를 한참
// 넘겨 계속 끄는** 시험이 있고, 붙잡히면 0.4 가 나와 즉시 갈린다.
//
// 정렬·순서는 순수 모듈(`canvasEditArrange`)이 규칙을 소유하므로 여기서는 **배선**을 본다:
// 버튼이 어떤 축·방식을 부르는가, 몇 개 골랐을 때 쓸 수 있는가, 쓰기가 실제로 나가는가.

/** 격자 붙임 토글 버튼. */
function gridToggle(): HTMLElement {
  return screen.getByTestId('canvas-grid-toggle');
}

/** 스테이지 px 상자가 (10,10,40,20) 이 되는 사각형 — 중심 x 가 격자 위가 **아니다**. */
const SNAP_GEOMETRY: BoxGeometry = { x: 0.05, y: 0.1, w: 0.2, h: 0.2 };

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

    // 40px = 스테이지 폭의 0.2. 붙임이 꺼져 있으므로 그대로다.
    expect((emittedGeometry(emit, 'a') as BoxGeometry).x).toBeCloseTo(0.25, 9);
  });

  it('켜져 있으면 **중심**이 격자에 붙는다 (다른 패널의 격자와 같은 기준)', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={emit} />);
    stubOverlayRect();
    fireEvent.click(gridToggle());

    send('pointerdown', 20, 20);
    send('pointermove', 60, 20);
    await nextFrame();

    // 중심 15% + 20% = 35% → 40% 로 붙는다 → 이동량 25% → x = 0.05 + 0.25 = 0.30.
    const moved = emittedGeometry(emit, 'a') as BoxGeometry;
    expect(moved.x).toBeCloseTo(0.3, 9);
    // 중심이 실제로 눈금(40%) 위에 앉았다.
    expect((moved.x + moved.w / 2) * 100).toBeCloseTo(40, 6);
  });

  it('세로도 자기 축 길이로 죈다 (축을 뒤바꾸지 않는다)', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={emit} />);
    stubOverlayRect();
    fireEvent.click(gridToggle());

    // 칸은 정사각이라 세로도 20px 이다(폭 200 의 10%). 중심 20px + 12px = 32px → 40px.
    send('pointerdown', 20, 20);
    send('pointermove', 20, 32);
    await nextFrame();

    const moved = emittedGeometry(emit, 'a') as BoxGeometry;
    expect(moved.y).toBeCloseTo(0.3, 9);
    // 중심이 실제로 20px 칸의 배수(40px) 위에 앉았다 — 가로 칸과 같은 px 다.
    expect((moved.y + moved.h / 2) * 100).toBeCloseTo(40, 6);
  });

  it('**스테이지의 40% 를 한참 넘겨도 계속 간다** — 상한이 새어 들어오면 0.4 에서 멈춘다 (위험 R6 · AC-E5)', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={emit} />);
    stubOverlayRect();
    fireEvent.click(gridToggle());

    send('pointerdown', 20, 20);
    // 180px = 스테이지 폭의 90% — 이미 상한(40%)의 두 배가 넘는다.
    send('pointermove', 200, 20);
    await nextFrame();
    const half = emittedGeometry(emit, 'a') as BoxGeometry;
    expect(half.x).toBeCloseTo(1.0, 9);
    expect(half.x).toBeGreaterThan(0.4);

    // 그리고 **계속 간다** — 스테이지를 통째로 벗어난 뒤에도 붙잡히지 않는다.
    // 390px = 스테이지 폭의 195% 이며, 중심 15% + 195% = 210% 로 눈금 위에 그대로 앉는다.
    send('pointermove', 410, 20);
    await nextFrame();
    const far = emittedGeometry(emit, 'a') as BoxGeometry;
    expect(far.x).toBeCloseTo(2.0, 9);

    // clamp 도 없다 — 0..1 로 잘리지 않고 그대로 저장된다(가정 A5).
    expect(far.x).toBeGreaterThan(1);
  });

  it('무리 이동은 잡은 요소 하나를 기준으로 죈다 (무리가 흩어지지 않는다)', async () => {
    const emit = vi.fn();
    render(
      <Harness
        elements={[rect('a', SNAP_GEOMETRY), rect('b', { x: 0.6, y: 0.6, w: 0.1, h: 0.1 })]}
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

    // 기준은 잡은 a 다. 둘 다 **같은 이동량**(0.25)을 받는다.
    expect((emittedGeometry(emit, 'a') as BoxGeometry).x).toBeCloseTo(0.3, 9);
    expect((emittedGeometry(emit, 'b') as BoxGeometry).x).toBeCloseTo(0.85, 9);
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

/** 격자 간격 고르개. */
function gridStepSelect(): HTMLSelectElement {
  return screen.getByTestId('canvas-grid-step') as HTMLSelectElement;
}

/** 격자 간격을 고른다. */
function pickGridStep(percent: number): void {
  fireEvent.change(gridStepSelect(), { target: { value: String(percent) } });
}

/** 지금 그려진 격자의 `background-image`. */
function gridImage(): string {
  return screen.getByTestId('panel-edit-grid').style.backgroundImage;
}

/**
 * 격자를 켜고 간격을 고른 뒤 20px(= 스테이지 폭의 10%) 를 끈다.
 *
 * 중심 15% + 10% = 25% 는 간격마다 다른 눈금으로 붙는다 — 5% → 25%, 10% → 30%, 20% → 20%.
 * 셋이 서로 다르므로 "간격이 실제로 붙임까지 갔는가" 가 좌표 하나로 갈린다.
 */
async function dragTenPercent(step?: number): Promise<ReturnType<typeof vi.fn>> {
  const emit = vi.fn();
  render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={emit} />);
  stubOverlayRect();
  fireEvent.click(gridToggle());
  if (step !== undefined) pickGridStep(step);

  send('pointerdown', 20, 20);
  send('pointermove', 40, 20);
  await nextFrame();
  return emit;
}

describe('격자 간격 — 고른 값 하나가 그림과 붙임을 함께 정한다', () => {
  it('처음에는 공용 기본 간격이며, 그 간격으로 그려진다', () => {
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={vi.fn()} />);
    fireEvent.click(gridToggle());

    expect(gridStepSelect().value).toBe(String(CANVAS_GRID_STEP_PERCENT));
    expect(gridImage()).toContain(`transparent 1px ${CANVAS_GRID_STEP_PERCENT}%`);
  });

  it('고를 수 있는 값은 canvasEditArrange 가 소유한 목록 그대로다', () => {
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={vi.fn()} />);

    const values = [...gridStepSelect().options].map((o) => o.value);
    expect(values).toEqual(CANVAS_GRID_STEP_CHOICES.map(String));
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
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={vi.fn()} />);
    fireEvent.click(gridToggle());

    pickGridStep(20);
    expect(gridImage()).toContain('transparent 1px 20%');
    expect(gridImage()).not.toContain('transparent 1px 10%');

    pickGridStep(5);
    expect(gridImage()).toContain('transparent 1px 5%');
    expect(gridImage()).not.toContain('transparent 1px 20%');
  });

  it('간격을 바꾸면 **붙는 자리**도 함께 바뀐다 (CSS 만 바뀌고 붙임이 남으면 화면이 거짓말이다)', async () => {
    // 중심 15% + 10% = 25%.
    // 5% → 25% 에 붙는다 → 이동량 10% → x = 0.15.
    const fine = await dragTenPercent(5);
    expect((emittedGeometry(fine, 'a') as BoxGeometry).x).toBeCloseTo(0.15, 9);
    cleanup();

    // 10%(기본) → 30% 에 붙는다 → 이동량 15% → x = 0.20.
    const base = await dragTenPercent();
    expect((emittedGeometry(base, 'a') as BoxGeometry).x).toBeCloseTo(0.2, 9);
    cleanup();

    // 20% → 20% 로 되돌아간다 → 이동량 5% → x = 0.10.
    const coarse = await dragTenPercent(20);
    expect((emittedGeometry(coarse, 'a') as BoxGeometry).x).toBeCloseTo(0.1, 9);
  });

  it('붙은 중심이 **그려진 선 위**에 앉는다 (두 값이 같은 값인지 좌표로 확인한다)', async () => {
    const emit = await dragTenPercent(20);

    const moved = emittedGeometry(emit, 'a') as BoxGeometry;
    const centerPercent = (moved.x + moved.w / 2) * 100;
    // 20% 간격의 선은 0·20·40·… 에 있다. 그 위에 앉지 않으면 그림과 붙임이 갈라진 것이다.
    expect(centerPercent % 20).toBeCloseTo(0, 6);
    expect(gridImage()).toContain('transparent 1px 20%');
  });

  it('간격을 바꿔도 상한은 여전히 무한대다 (위험 R6 은 간격마다 되살아날 수 있다)', async () => {
    const emit = vi.fn();
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={emit} />);
    stubOverlayRect();
    fireEvent.click(gridToggle());
    pickGridStep(20);

    send('pointerdown', 20, 20);
    // 180px = 스테이지 폭의 90% — 상한(40%)의 두 배가 넘는다.
    send('pointermove', 200, 20);
    await nextFrame();

    expect((emittedGeometry(emit, 'a') as BoxGeometry).x).toBeGreaterThan(0.4);
  });

  it('Shift+방향키의 "한 칸" 도 고른 간격을 따른다 — 그리지 않은 칸으로 뛰지 않는다', () => {
    const emit = renderPickedBox(SNAP_GEOMETRY, { x: 30, y: 20 });
    fireEvent.click(gridToggle());
    pickGridStep(20);
    emit.mockClear();

    sendKey('ArrowRight', { shiftKey: true });

    expect((emittedGeometry(emit, 'a') as BoxGeometry).x).toBeCloseTo(0.05 + 0.2, 10);
  });

  it('간격을 바꿔도 프레임을 예약하지 않는다 — 표시 상태일 뿐이다 (AC-E4)', () => {
    render(<Harness elements={[rect('a', SNAP_GEOMETRY)]} onElementsChange={vi.fn()} />);
    fireEvent.click(gridToggle());
    const raf = vi.spyOn(globalThis, 'requestAnimationFrame');

    pickGridStep(20);
    pickGridStep(5);

    expect(raf).not.toHaveBeenCalled();
  });
});

// --- 격자 칸은 정사각형이다 (사용 시험: "격자가 일정하지 않음") -------------
//
// 그라디언트의 백분율은 **제 축 길이에 대한** 값이다. 그래서 두 축에 같은 10% 를 주면
// 800×400 패널에서 칸이 80×40 px 이 된다 — 화면에서 본 그대로 직사각형이다.
//
// **픽스처가 정사각형이면 이 절은 실패할 수 없다.** 400×400 에서는 옛 방식과 새 방식이
// 같은 값을 낸다. 그래서 여기서만 스테이지를 800×400 으로 두고, `stubOverlayRect` 도 같은
// 치수로 심는다(포인터 좌표가 스테이지 공간으로 되돌아오는 그 비를 1 로 만든다).
//
// 그리고 **그린 것과 붙는 것을 함께** 본다. 정사각 칸을 그리면서 옛 백분율로 붙으면
// 화면이 거짓말을 하는데, 그 거짓말은 CSS 만 보는 시험도 좌표만 보는 시험도 보지 못한다.

/** 가로가 세로의 두 배인 스테이지. 정사각 픽스처로는 이 절의 시험이 성립하지 않는다. */
const WIDE_STAGE: StageSize = { width: 800, height: 400 };

/** 그려진 격자의 두 축 백분율을 CSS 에서 그대로 읽는다. */
function drawnSteps(): { x: number; y: number } {
  const image = gridImage();
  const right = /to right,.*?transparent 1px ([\d.]+)%\)/.exec(image);
  const bottom = /to bottom,.*?transparent 1px ([\d.]+)%\)/.exec(image);
  expect(right, `가로 그라디언트를 읽지 못했다: ${image}`).not.toBeNull();
  expect(bottom, `세로 그라디언트를 읽지 못했다: ${image}`).not.toBeNull();
  return { x: Number(right![1]), y: Number(bottom![1]) };
}

/** 그려진 칸 하나의 실제 크기(px). 두 축이 같아야 칸이 정사각형이다. */
function drawnCellPx(): { x: number; y: number } {
  const steps = drawnSteps();
  return {
    x: (steps.x / 100) * WIDE_STAGE.width,
    y: (steps.y / 100) * WIDE_STAGE.height,
  };
}

/** 800×400 스테이지에 사각형 하나를 세우고 격자를 켠다. px 상자는 (40,40,160,80) 이다. */
function renderWide(step?: number): ReturnType<typeof vi.fn> {
  const emit = vi.fn();
  render(
    <Harness elements={[rect('a', SNAP_GEOMETRY)]} stage={WIDE_STAGE} onElementsChange={emit} />,
  );
  stubOverlayRect(0, 0, WIDE_STAGE.width, WIDE_STAGE.height);
  fireEvent.click(gridToggle());
  if (step !== undefined) pickGridStep(step);
  return emit;
}

/** 지금 통보된 사각형의 중심(스테이지 로컬 px). */
function centerPx(emit: ReturnType<typeof vi.fn>): { x: number; y: number } {
  const g = emittedGeometry(emit, 'a') as BoxGeometry;
  return {
    x: (g.x + g.w / 2) * WIDE_STAGE.width,
    y: (g.y + g.h / 2) * WIDE_STAGE.height,
  };
}

describe('격자 칸은 정사각형이다 (사용 시험: "격자가 일정하지 않음")', () => {
  it('가로세로 비가 다른 패널에서도 그려진 칸의 두 변이 **같은 px** 이다', () => {
    for (const step of CANVAS_GRID_STEP_CHOICES) {
      cleanup();
      renderWide(step);
      const cell = drawnCellPx();
      expect(cell.x, `${step}% 에서 칸이 직사각형이다`).toBeCloseTo(cell.y, 6);
    }
  });

  it('두 축의 **백분율**은 서로 다르다 — 그것이 정사각형이 되는 방식이다', () => {
    renderWide();
    // 800×400 이면 세로 백분율이 두 배다. 같은 수를 두 축에 쓰던 것이 결함이었다.
    expect(drawnSteps()).toEqual({ x: 10, y: 20 });
  });

  it('붙는 자리가 **그려진 칸**의 배수다 — 간격을 바꿔도 그렇다', async () => {
    // 그림과 붙임이 갈라지면 "붙긴 붙는데 선하고 안 맞는다" 가 된다. 간격을 여러 개
    // 지나는 것이 중요하다 — 한 값에서만 맞는 파생은 파생이 아니라 우연이다.
    for (const step of CANVAS_GRID_STEP_CHOICES) {
      cleanup();
      const emit = renderWide(step);
      const cell = drawnCellPx();

      // 상자 안(120,80)을 잡아 대각선으로 끈다 — 두 축이 함께 걸린다.
      send('pointerdown', 120, 80);
      send('pointermove', 170, 130);
      await nextFrame();

      const center = centerPx(emit);
      expect(center.x % cell.x, `${step}% 가로`).toBeCloseTo(0, 6);
      expect(center.y % cell.y, `${step}% 세로`).toBeCloseTo(0, 6);
    }
  });

  it('붙은 뒤 두 축이 같은 눈금 값을 가리킨다 (축을 뒤바꾸지 않는다)', async () => {
    const emit = renderWide();
    const cell = drawnCellPx();

    send('pointerdown', 120, 80);
    send('pointermove', 170, 130);
    await nextFrame();

    // 칸 80px · 중심 (120,80) → (170,130) → 가장 가까운 눈금은 두 축 모두 160px 이다.
    expect(cell.x).toBe(80);
    const center = centerPx(emit);
    expect(center.x).toBeCloseTo(160, 6);
    expect(center.y).toBeCloseTo(160, 6);
  });

  it('Shift+방향키가 **그려진 칸 하나**만큼 옮긴다 (두 축이 같은 px 다)', () => {
    const emit = renderWide();
    const cell = drawnCellPx();

    // 눌러서 고른 뒤 손을 뗀다(놓기가 "제자리 확정" 쓰기를 한 번 낸다). 하네스는 통보를
    // 되먹이지 않으므로 **매 키의 기준은 언제나 초기 기하**이며, 그 중심이 (120,80) 이다.
    send('pointerdown', 120, 80);
    send('pointerup', 120, 80);
    emit.mockClear();

    sendKey('ArrowRight', { shiftKey: true });
    expect(centerPx(emit).x - 120).toBeCloseTo(cell.x, 6);

    sendKey('ArrowDown', { shiftKey: true });
    expect(centerPx(emit).y - 80).toBeCloseTo(cell.y, 6);
  });
});

// --- 정렬 (T13) -----------------------------------------------------------

/** 정렬 버튼 하나. */
function alignBtn(id: string): HTMLElement {
  return screen.getByTestId(`canvas-align-${id}`);
}

/** 정렬 시험용 두 사각형 — px 상자가 (10,10,40,20) 과 (100,50,20,10) 이다. */
const ALIGN_A: BoxGeometry = { x: 0.05, y: 0.1, w: 0.2, h: 0.2 };
const ALIGN_B: BoxGeometry = { x: 0.5, y: 0.5, w: 0.1, h: 0.1 };

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

    expect((emittedGeometry(emit, 'a') as BoxGeometry).x).toBeCloseTo(0.05, 9);
    expect((emittedGeometry(emit, 'b') as BoxGeometry).x).toBeCloseTo(0.05, 9);
  });

  it('오른쪽 맞춤 — 오른쪽 변이 같아진다', () => {
    const emit = vi.fn();
    renderTwoSelected(emit);
    fireEvent.click(alignBtn('right'));

    const a = emittedGeometry(emit, 'a') as BoxGeometry;
    const b = emittedGeometry(emit, 'b') as BoxGeometry;
    expect(a.x + a.w).toBeCloseTo(0.6, 9);
    expect(b.x + b.w).toBeCloseTo(0.6, 9);
  });

  it('가로 가운데 맞춤 — 중심의 가로가 같아진다', () => {
    const emit = vi.fn();
    renderTwoSelected(emit);
    fireEvent.click(alignBtn('center-x'));

    const a = emittedGeometry(emit, 'a') as BoxGeometry;
    const b = emittedGeometry(emit, 'b') as BoxGeometry;
    expect(a.x + a.w / 2).toBeCloseTo(0.325, 9);
    expect(b.x + b.w / 2).toBeCloseTo(0.325, 9);
  });

  it('가로 정렬은 세로를 건드리지 않는다', () => {
    const emit = vi.fn();
    renderTwoSelected(emit);
    fireEvent.click(alignBtn('center-x'));

    expect((emittedGeometry(emit, 'a') as BoxGeometry).y).toBeCloseTo(0.1, 9);
    expect((emittedGeometry(emit, 'b') as BoxGeometry).y).toBeCloseTo(0.5, 9);
  });

  it('위 맞춤 — 위 변이 같아진다', () => {
    const emit = vi.fn();
    renderTwoSelected(emit);
    fireEvent.click(alignBtn('top'));

    expect((emittedGeometry(emit, 'a') as BoxGeometry).y).toBeCloseTo(0.1, 9);
    expect((emittedGeometry(emit, 'b') as BoxGeometry).y).toBeCloseTo(0.1, 9);
  });

  it('아래 맞춤 — 아래 변이 같아진다', () => {
    const emit = vi.fn();
    renderTwoSelected(emit);
    fireEvent.click(alignBtn('bottom'));

    const a = emittedGeometry(emit, 'a') as BoxGeometry;
    const b = emittedGeometry(emit, 'b') as BoxGeometry;
    expect(a.y + a.h).toBeCloseTo(0.6, 9);
    expect(b.y + b.h).toBeCloseTo(0.6, 9);
  });

  it('세로 가운데 맞춤 — 중심의 세로가 같아진다', () => {
    const emit = vi.fn();
    renderTwoSelected(emit);
    fireEvent.click(alignBtn('center-y'));

    const a = emittedGeometry(emit, 'a') as BoxGeometry;
    const b = emittedGeometry(emit, 'b') as BoxGeometry;
    expect(a.y + a.h / 2).toBeCloseTo(0.35, 9);
    expect(b.y + b.h / 2).toBeCloseTo(0.35, 9);
  });

  it('세로 정렬은 가로를 건드리지 않는다', () => {
    const emit = vi.fn();
    renderTwoSelected(emit);
    fireEvent.click(alignBtn('top'));

    expect((emittedGeometry(emit, 'a') as BoxGeometry).x).toBeCloseTo(0.05, 9);
    expect((emittedGeometry(emit, 'b') as BoxGeometry).x).toBeCloseTo(0.5, 9);
  });

  it('고르지 않은 요소는 제자리에 남는다', () => {
    const emit = vi.fn();
    render(
      <Harness
        elements={[rect('a', ALIGN_A), rect('b', ALIGN_B), rect('c', { x: 0.9, y: 0.9, w: 0.05, h: 0.05 })]}
        onElementsChange={emit}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 30, 20);
    send('pointerdown', 110, 55, { shiftKey: true });
    fireEvent.click(alignBtn('left'));

    expect((emittedGeometry(emit, 'c') as BoxGeometry).x).toBeCloseTo(0.9, 9);
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
    expect(moved.x).toBeCloseTo(0.05, 9);
  });
});

// --- z-order (T14) --------------------------------------------------------

/** 앞/뒤로 보내기 버튼. */
function orderBtn(dir: 'front' | 'back'): HTMLButtonElement {
  return screen.getByTestId(`canvas-order-${dir}`) as HTMLButtonElement;
}

/** 겹치지 않는 세 사각형 — 하나씩 눌러 고를 수 있다. */
const ORDER_ELEMENTS: CanvasElement[] = [
  rect('a', { x: 0.05, y: 0.1, w: 0.1, h: 0.2 }),
  rect('b', { x: 0.3, y: 0.1, w: 0.1, h: 0.2 }),
  rect('c', { x: 0.6, y: 0.1, w: 0.1, h: 0.2 }),
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
    expect(hint!.textContent).toBe('dashboard.canvas.edit.keyboardHint');
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

  it('빈 지점을 누르면 초점을 빼앗지 않는다 (우리 조작이 아니다)', () => {
    render(<Harness elements={[rect('a', BOX_GEOMETRY)]} onElementsChange={vi.fn()} />);
    stubOverlayRect();

    send('pointerdown', 5, 5);

    expect(document.activeElement).not.toBe(overlayRoot());
  });
});

describe('방향키가 고른 것을 옮긴다 (AC-08)', () => {
  it('한 번에 1 CSS px 씩 옮긴다 — 축마다 자기 길이로 나눈다', () => {
    const emit = renderPickedBox();

    expect(sendKey('ArrowRight')).toBe(false); // 소비했다
    expectBox(emittedGeometry(emit, 'a'), { x: 0.2 + 1 / 200, y: 0.2, w: 0.4, h: 0.4 });

    sendKey('ArrowDown');
    expectBox(emittedGeometry(emit, 'a'), { x: 0.2, y: 0.2 + 1 / 100, w: 0.4, h: 0.4 });
  });

  it('왼쪽·위는 음의 방향이다 (축을 뒤바꾸지 않는다)', () => {
    const emit = renderPickedBox();

    sendKey('ArrowLeft');
    expectBox(emittedGeometry(emit, 'a'), { x: 0.2 - 1 / 200, y: 0.2, w: 0.4, h: 0.4 });

    sendKey('ArrowUp');
    expectBox(emittedGeometry(emit, 'a'), { x: 0.2, y: 0.2 - 1 / 100, w: 0.4, h: 0.4 });
  });

  it('Shift 는 한 격자 칸을 옮긴다 — 칸은 정사각이라 축마다 분수가 다르다', () => {
    const emit = renderPickedBox();
    // 200×100 스테이지에서 10% 칸은 20px 이다. 가로로는 스테이지의 0.1, 세로로는 0.2 —
    // **같은 px** 를 두 축의 분수로 옮기면 두 수가 갈린다.
    const cell = squareGridSteps(CANVAS_GRID_STEP_PERCENT, STAGE);
    expect((cell.x / 100) * STAGE.width).toBeCloseTo((cell.y / 100) * STAGE.height, 6);

    sendKey('ArrowRight', { shiftKey: true });
    expectBox(emittedGeometry(emit, 'a'), { x: 0.2 + cell.x / 100, y: 0.2, w: 0.4, h: 0.4 });

    sendKey('ArrowUp', { shiftKey: true });
    expectBox(emittedGeometry(emit, 'a'), { x: 0.2, y: 0.2 - cell.y / 100, w: 0.4, h: 0.4 });
  });

  it('크기는 건드리지 않는다 — 방향키는 이동이지 크기 조절이 아니다', () => {
    // px 상자 (20,10,60,50) — 가운데는 (50,35) 다.
    const emit = renderPickedBox({ x: 0.1, y: 0.1, w: 0.3, h: 0.5 }, { x: 50, y: 35 });

    sendKey('ArrowRight');

    const g = emittedGeometry(emit, 'a') as BoxGeometry;
    expect(g.w).toBeCloseTo(0.3, 10);
    expect(g.h).toBeCloseTo(0.5, 10);
  });

  it('둘 이상 골랐으면 같은 델타가 전부에 적용된다 (드래그와 같은 규칙)', () => {
    const emit = vi.fn();
    render(
      <Harness
        elements={[rect('a', BOX_GEOMETRY), rect('b', { x: 0.75, y: 0.05, w: 0.2, h: 0.2 })]}
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

    const cell = CANVAS_GRID_STEP_PERCENT / 100;
    expectBox(emittedGeometry(emit, 'a'), { x: 0.2 + cell, y: 0.2, w: 0.4, h: 0.4 });
    expectBox(emittedGeometry(emit, 'b'), { x: 0.75 + cell, y: 0.05, w: 0.2, h: 0.2 });
  });

  it('고르지 않은 요소는 제자리에 남는다', () => {
    const emit = vi.fn();
    render(
      <Harness
        elements={[rect('a', BOX_GEOMETRY), rect('b', { x: 0.75, y: 0.05, w: 0.2, h: 0.2 })]}
        onElementsChange={emit}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 80, 40);
    send('pointerup', 80, 40);
    emit.mockClear();

    sendKey('ArrowRight');

    expectBox(emittedGeometry(emit, 'b'), { x: 0.75, y: 0.05, w: 0.2, h: 0.2 });
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

    const cell = CANVAS_GRID_STEP_PERCENT / 100;
    const g = emittedGeometry(emit, 'l') as LineGeometry;
    expect(g.x1).toBeCloseTo(0.1 + cell, 10);
    expect(g.x2).toBeCloseTo(0.5 + cell, 10);
    expect(g.y1).toBeCloseTo(0.2, 10);
    expect(g.y2).toBeCloseTo(0.6, 10);
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

    const cell = squareGridSteps(CANVAS_GRID_STEP_PERCENT, STAGE);
    const g = emittedGeometry(emit, 't') as PointGeometry;
    expect(g.x).toBeCloseTo(0.5, 10);
    expect(g.y).toBeCloseTo(0.5 + cell.y / 100, 10);
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
  it('오른쪽 끝을 넘어가도 1 에서 붙잡히지 않는다', () => {
    // px 상자 (190,20,80,40) — 가운데는 (230,40) 으로 스테이지 밖이지만 판정은 px 공간이다.
    const emit = renderPickedBox({ x: 0.95, y: 0.2, w: 0.4, h: 0.4 }, { x: 230, y: 40 });

    sendKey('ArrowRight', { shiftKey: true });

    const g = emittedGeometry(emit, 'a') as BoxGeometry;
    expect(g.x).toBeCloseTo(1.05, 10);
  });

  it('왼쪽으로 나가도 0 에서 붙잡히지 않는다', () => {
    // px 상자 (4,20,80,40) — 가운데는 (44,40) 이다.
    const emit = renderPickedBox({ x: 0.02, y: 0.2, w: 0.4, h: 0.4 }, { x: 44, y: 40 });

    sendKey('ArrowLeft', { shiftKey: true });

    const g = emittedGeometry(emit, 'a') as BoxGeometry;
    expect(g.x).toBeCloseTo(-0.08, 10);
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

    for (const key of ['Tab', 'Escape', 'Enter', ' ', 'a']) {
      expect(sendKey(key)).toBe(true);
    }
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

  it('스테이지를 아직 재지 못했으면 미세 이동은 하지 않는다 (0 으로 나누지 않는다)', () => {
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

    expect(sendKey('ArrowRight')).toBe(true);
    expect(emit).not.toHaveBeenCalled();
  });

  it('그래도 한 격자 칸 이동은 성립한다 — 칸은 축 길이에 대한 분수라 나눌 것이 없다', () => {
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
      x: 0.2 + CANVAS_GRID_STEP_PERCENT / 100,
      y: 0.2,
      w: 0.4,
      h: 0.4,
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
    expect(after.x).toBeCloseTo(before.x + CANVAS_GRID_STEP_PERCENT / 100, 10);
    expect(after.y).toBeCloseTo(before.y, 10);
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
    expectBox(emittedGeometry(emit, 'a'), { x: 0.2 + 1 / 200, y: 0.2, w: 0.4, h: 0.4 });
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

  it('골라 둔 것이 있어도 빈 지점 누름은 소비되지 않는다 (패널 크기 조절이 살아 있어야 한다)', () => {
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

    expect(evt.defaultPrevented).toBe(false);
    expect(onParentDown).toHaveBeenCalledTimes(1);
    expect(selectionText()).toBe('');
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
const CENTER_BOX: BoxGeometry = { x: 0.4, y: 0.4, w: 0.2, h: 0.2 };

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

    expectBox(emittedGeometry(emit, 'a'), { x: 0.5, y: 0.5, w: 0.2, h: 0.2 });
  });

  it('축척 1 에서는 같은 화면 이동량이 그대로 스테이지 이동량이다 (되돌림 방어)', async () => {
    const emit = renderScaled([rect('a', CENTER_BOX)], vi.fn(), 1);

    // 축척 1 이므로 스테이지 px = 화면 px - 원점이다.
    const from = { x: ZOOM_RECT.left + 400, y: ZOOM_RECT.top + 200 };
    send('pointerdown', from.x, from.y);
    send('pointermove', from.x + 40, from.y + 20);
    await nextFrame();

    // 화면 (40, 20) = 스테이지 (40, 20) → 정규화 (0.05, 0.05).
    expectBox(emittedGeometry(emit, 'a'), { x: 0.45, y: 0.45, w: 0.2, h: 0.2 });
  });

  it('놓는 순간의 좌표도 같은 공간으로 옮긴다 (pointerup 이 마지막 자리를 확정한다)', async () => {
    const emit = renderScaled([rect('a', CENTER_BOX)], vi.fn());

    const from = toScreen(400, 200);
    send('pointerdown', from.x, from.y);
    // 합류 프레임을 기다리지 않고 바로 뗀다 — 확정 경로가 같은 변환을 쓰는지 본다.
    send('pointerup', from.x + 40, from.y + 20);

    expectBox(emittedGeometry(emit, 'a'), { x: 0.5, y: 0.5, w: 0.2, h: 0.2 });
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

    expectBox(emittedGeometry(emit, 'a'), { x: 0.4, y: 0.4, w: 0.35, h: 0.35 });
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
    render(<Harness elements={[rect('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 })]} onElementsChange={vi.fn()} />);
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
