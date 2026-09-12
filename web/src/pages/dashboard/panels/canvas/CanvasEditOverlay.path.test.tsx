// 경로 요소의 편집 이음매 — 오버레이 (SPEC-CANVAS-008 M5).
//
// M1 이 실측한 침묵 지점 가운데 **오버레이가 소유한 둘**을 여기서 잰다:
//   (e) `outlineBox` — 선택 외곽선이 상자인가(문구 상자가 아닌가)
//   (·) `handleDragState` — **손잡이를 잡으면 실제로 크기 조절이 시작되는가**
//
// 뒤쪽이 이 파일의 무게중심이다. `handlesFor` 와 `handlePositions` 를 고쳐도 이 한 자리를
// 빠뜨리면 **여덟 손잡이가 보이는 채로 아무 반응이 없다** — 잡히지 않는 손잡이는 순수
// 함수 시험으로는 결코 관측되지 않고, 화면에서만 드러난다.
//
// 그리고 (d) `patchNodeGeometry` 가 실제 드래그 경로에서도 도는지 함께 잰다. 순수 시험은
// 그 함수가 상자를 쓴다는 것만 말하고, "그 함수가 이 경로에서 불린다" 는 말하지 않는다.
//
// 축척은 **가로 0.4 · 세로 0.25** 로 갈린다(단위 축척도, 같은 축척도 아니다).
//
// @spec SPEC-CANVAS-008 REQ-02 · AC-03 · D1

import { describe, expect, it, vi, afterEach } from 'vitest';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { BoxGeometry, CanvasElement, CanvasSize, PathElement } from './canvasConfig';
import { CanvasEditDockRegion } from './CanvasEditDock';
import CanvasEditOverlay from './CanvasEditOverlay';
import {
  CanvasEditSelectionContext,
  useCanvasEditSelectionState,
} from './canvasEditContext';
import { BOX_HANDLE_IDS } from './canvasEditGeometry';
import type { StageSize } from './canvasGeometry';
import { PATH_LOCAL_EXTENT, type PathCommand } from './shapes/pathTypes';
import type { CanvasNode } from './group/groupTypes';

// --- 고정 입력 -----------------------------------------------------------

const STAGE: StageSize = { width: 200, height: 100 };
const CANVAS: CanvasSize = { width: 500, height: 400 };

/** 스테이지 px 상자가 (40, 20, 80, 40) 이 되는 캔버스 상자. 비정사각 · 원점 ≠ 0. */
const BOX: BoxGeometry = { x: 100, y: 80, w: 200, h: 160 };

/** 비대칭 삼각형 — 대칭 도형은 축을 뒤바꾼 결함을 감춘다. */
const TRIANGLE: readonly PathCommand[] = [
  { c: 'M', x: 0, y: PATH_LOCAL_EXTENT },
  { c: 'L', x: PATH_LOCAL_EXTENT, y: PATH_LOCAL_EXTENT },
  { c: 'L', x: 0, y: 0 },
  { c: 'Z' },
];

function pathElement(id = 'p', geometry: BoxGeometry = BOX): PathElement {
  return { id, kind: 'path', geometry, path: [...TRIANGLE], style: { fill: '#123456' } };
}

function rect(id: string, geometry: BoxGeometry): CanvasElement {
  return { id, kind: 'rect', geometry, style: {} };
}

// --- 하네스 ---------------------------------------------------------------

function Harness({
  elements,
  onElementsChange,
}: {
  elements: readonly CanvasElement[];
  onElementsChange: (next: CanvasNode[]) => void;
}) {
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <div className="relative">
        <span data-testid="selection">{[...state.selection].join(',')}</span>
        <CanvasEditDockRegion enabled>
          <CanvasEditOverlay
            enabled
            elements={elements}
            projection={{ stage: STAGE, canvas: CANVAS }}
            textWidths={{}}
            onElementsChange={onElementsChange}
          />
        </CanvasEditDockRegion>
      </div>
    </CanvasEditSelectionContext>
  );
}

// --- 포인터 관용구 (기존 파일의 관용구를 그대로 쓴다) ----------------------

function pointer(type: string, x: number, y: number, init: MouseEventInit = {}): Event {
  return new MouseEvent(type, { clientX: x, clientY: y, bubbles: true, cancelable: true, ...init });
}

function send(type: string, x: number, y: number, init: MouseEventInit = {}): Event {
  const evt = pointer(type, x, y, init);
  fireEvent(screen.getByTestId('canvas-edit-overlay'), evt);
  return evt;
}

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

async function nextFrame(): Promise<void> {
  await act(async () => {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  });
}

function handleIds(): string[] {
  const root = screen.getByTestId('canvas-edit-overlay');
  return [...root.querySelectorAll<HTMLElement>('[data-testid^="canvas-handle-"]')].map((node) =>
    (node.dataset.testid ?? '').replace('canvas-handle-', ''),
  );
}

async function dragHandle(id: string, to: { x: number; y: number }): Promise<void> {
  const el = screen.getByTestId(`canvas-handle-${id}`);
  const from = { x: Number.parseFloat(el.style.left), y: Number.parseFloat(el.style.top) };
  fireEvent(el, pointer('pointerdown', from.x, from.y));
  send('pointermove', to.x, to.y);
  await nextFrame();
}

function emittedElement(spy: ReturnType<typeof vi.fn>, id: string): CanvasElement | undefined {
  const last = spy.mock.calls.at(-1)?.[0] as CanvasElement[] | undefined;
  return last?.find((el) => el.id === id);
}

/** 경로 하나를 골라 둔 상태(손잡이가 뜨는 최소 조건). */
function renderSelectedPath(emit = vi.fn()): ReturnType<typeof vi.fn> {
  render(<Harness elements={[pathElement()]} onElementsChange={emit} />);
  stubOverlayRect();
  // 상자 (40,20)~(120,60) 의 한가운데. 삼각형의 안쪽이라 윤곽 판정으로도 잡힌다.
  send('pointerdown', 60, 50);
  return emit;
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// --- 고르기 ---------------------------------------------------------------

describe('경로도 눌러 고른다', () => {
  it('삼각형 안쪽을 누르면 골라진다', () => {
    renderSelectedPath();
    expect(screen.getByTestId('selection').textContent).toBe('p');
  });

  it('상자 안이지만 삼각형 **밖**인 자리(우상단 빈 곳)는 고르지 않는다', () => {
    render(<Harness elements={[pathElement()]} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    // 상자의 우상단 모서리 근처. 삼각형은 좌하 절반이므로 여기는 빈 곳이다.
    send('pointerdown', 118, 22);
    expect(screen.getByTestId('selection').textContent).toBe('');
  });
});

// --- (e) 선택 외곽선 ------------------------------------------------------

describe('outlineBox — 경로의 외곽선은 상자다 (D1(e))', () => {
  it('투영된 상자 그대로 두른다 — 문구 상자가 아니다', () => {
    renderSelectedPath();
    const outline = screen.getByTestId('canvas-selection-p');
    // 문구로 읽히면 실측 폭 0 · 기본 글자 크기의 작은 상자(좌상단 근처)가 나온다.
    expect(outline.style.left).toBe('40px');
    expect(outline.style.top).toBe('20px');
    expect(outline.style.width).toBe('80px');
    expect(outline.style.height).toBe('40px');
  });

  it('명령 목록이 무엇이든 외곽선은 상자에서만 나온다', () => {
    // 명령이 **점 하나뿐**인 경로. 윤곽은 그 점 둘레뿐인데도 외곽선은 상자 전부를 두른다 —
    // 외곽선이 명령 목록에서 나온다면 이 시험이 빨개진다.
    const wild: PathElement = { ...pathElement(), path: [{ c: 'M', x: 5000, y: 5000 }] };
    render(<Harness elements={[wild]} onElementsChange={vi.fn()} />);
    stubOverlayRect();
    send('pointerdown', 80, 40);
    const outline = screen.getByTestId('canvas-selection-p');
    expect(outline.style.left).toBe('40px');
    expect(outline.style.top).toBe('20px');
    expect(outline.style.width).toBe('80px');
    expect(outline.style.height).toBe('40px');
  });
});

// --- 손잡이 집합과 **그 손잡이가 실제로 잡히는가** ------------------------

describe('경로에는 상자 여덟 손잡이가 뜨고, 그 손잡이가 실제로 잡힌다', () => {
  it('여덟이 시계 방향으로 뜬다 — 글자 크기 손잡이가 아니다', () => {
    renderSelectedPath();
    expect(handleIds()).toEqual([...BOX_HANDLE_IDS]);
    expect(screen.queryByTestId('canvas-handle-font')).toBeNull();
  });

  it('손잡이 자리가 상자의 모서리·변에 앉는다', () => {
    renderSelectedPath();
    const at = (id: string) => screen.getByTestId(`canvas-handle-${id}`).style;
    expect([at('nw').left, at('nw').top]).toEqual(['40px', '20px']);
    expect([at('se').left, at('se').top]).toEqual(['120px', '60px']);
    expect([at('n').left, at('n').top]).toEqual(['80px', '20px']);
  });

  it('**se 손잡이를 끌면 상자가 실제로 커진다** — 이 한 줄이 크기 조절의 관문이다', async () => {
    // `handleDragState` 가 경로를 받지 않으면 드래그가 **시작되지 않아** 아무것도
    // 통보되지 않는다. 손잡이는 보이는데 반응이 없는 상태이며, 앞선 두 시험은 여전히
    // 초록이다 — 그것이 이 시험이 따로 서 있는 이유다.
    const emit = renderSelectedPath();
    await dragHandle('se', { x: 160, y: 80 });

    expect(emit).toHaveBeenCalled();
    // 스테이지 (160,80) → 캔버스 (400, 320). nw 는 그대로이므로 상자는 (100,80,300,240).
    expect(emittedElement(emit, 'p')?.geometry).toEqual({ x: 100, y: 80, w: 300, h: 240 });
  });

  it('nw 손잡이를 끌면 좌상단이 움직인다 — 반대 모서리가 앵커다', async () => {
    const emit = renderSelectedPath();
    await dragHandle('nw', { x: 20, y: 10 });
    expect(emittedElement(emit, 'p')?.geometry).toEqual({ x: 50, y: 40, w: 250, h: 200 });
  });

  it('크기를 바꿔도 **명령 목록은 그대로다** — 상자만 달라진다', async () => {
    const emit = renderSelectedPath();
    await dragHandle('se', { x: 160, y: 80 });
    expect((emittedElement(emit, 'p') as PathElement).path).toEqual(TRIANGLE);
  });
});

// --- 몸통 드래그(이동) ----------------------------------------------------

describe('경로를 끌면 실제로 움직인다', () => {
  it('몸통을 끌면 상자가 델타만큼 옮겨진다', async () => {
    // `patchNodeGeometry` 가 경로를 그대로 돌려주면 통보된 기하가 시작값과 같다 —
    // "끌리는데 제자리" 라는 증상이며, 이 단언이 그 유일한 가드다.
    const emit = renderSelectedPath();
    send('pointermove', 80, 60);
    await nextFrame();

    // 스테이지 (60,50) → (80,60) 은 캔버스로 (+50, +40).
    expect(emittedElement(emit, 'p')?.geometry).toEqual({ x: 150, y: 120, w: 200, h: 160 });
  });

  it('무리 이동에서 경로와 사각형이 **같은 델타**로 함께 옮겨진다', async () => {
    const emit = vi.fn();
    render(
      <Harness
        elements={[pathElement('p'), rect('r', { x: 300, y: 80, w: 100, h: 100 })]}
        onElementsChange={emit}
      />,
    );
    stubOverlayRect();
    send('pointerdown', 60, 50);
    send('pointerup', 60, 50);
    // Shift 는 **고르기 전용**이라 여기서는 드래그가 시작되지 않는다(오버레이의 규칙).
    send('pointerdown', 130, 30, { shiftKey: true });
    send('pointerup', 130, 30);
    expect(screen.getByTestId('selection').textContent).toBe('p,r');

    // 이미 골라진 것을 다시 누르면 선택은 그대로이고 **무리 이동**이 시작된다.
    send('pointerdown', 60, 50);
    send('pointermove', 150, 90);
    await nextFrame();

    // 스테이지 (60,50) → (150,90) 은 캔버스로 (+225, +160). 두 요소가 **같은 델타**를 받는다.
    expect(emittedElement(emit, 'p')?.geometry).toEqual({ x: 325, y: 240, w: 200, h: 160 });
    expect(emittedElement(emit, 'r')?.geometry).toEqual({ x: 525, y: 240, w: 100, h: 100 });
  });
});
