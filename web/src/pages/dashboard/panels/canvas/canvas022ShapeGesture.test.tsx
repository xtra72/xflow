// 도형 모드의 몸짓 — 켜기 · 찍기 · 닫기 · 버리기 (SPEC-CANVAS-022 M3 · M4).
//
// @spec SPEC-CANVAS-022

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useState } from 'react';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';

import { I18nProvider } from '@/lib/i18n';
import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';

import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditDockRegion } from './CanvasEditDock';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import { parseCanvasConfig, type CanvasSize } from './canvasConfig';
import type { CanvasProjection } from './canvasGeometry';
import type { CanvasNode } from './group/groupTypes';

const CANVAS: CanvasSize = { width: 400, height: 300 };
/** 축척이 하나이고 1:1 이다 — 시험이 환산을 들고 다니지 않는다. */
const PROJ: CanvasProjection = { stage: { width: 400, height: 300 }, canvas: CANVAS };

function scene(): CanvasNode[] {
  return parseCanvasConfig({
    canvas: { ...CANVAS },
    elements: [{ id: 'a', kind: 'rect', geometry: { x: 20, y: 20, w: 40, h: 30 }, style: {} }],
  }).elements;
}

function Harness({ initial }: { initial: readonly CanvasNode[] }) {
  const [elements, setElements] = useState<readonly CanvasNode[]>(initial);
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <span data-testid="dump">{JSON.stringify(elements)}</span>
      <span data-testid="picked">{[...state.selection].join(',')}</span>
      <CanvasEditDockRegion enabled>
        <div data-testid="scaled-panel">
          <CanvasEditOverlay
            enabled
            elements={elements}
            projection={PROJ}
            textWidths={{}}
            onElementsChange={setElements}
          />
        </div>
      </CanvasEditDockRegion>
    </CanvasEditSelectionContext>
  );
}

/** jsdom 은 레이아웃을 하지 않으므로 오버레이의 화면 자리를 직접 심는다(014 의 함정). */
function stubRect(): void {
  vi.spyOn(screen.getByTestId('canvas-edit-overlay'), 'getBoundingClientRect').mockReturnValue({
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

function show(locale: 'ko' | 'en' = 'ko'): void {
  cleanup();
  globalThis.localStorage.clear();
  globalThis.localStorage.setItem('xflow-locale', locale);
  render(
    <I18nProvider>
      <Harness initial={scene()} />
    </I18nProvider>,
  );
  stubRect();
}

/** `MouseEvent` 로 짓는다 — jsdom 의 `PointerEvent` 는 init 의 `clientX` 를 싣지 않는다. */
function pointerEvent(type: string, x: number, y: number): Event {
  return new MouseEvent(type, { clientX: x, clientY: y, bubbles: true, button: 0, cancelable: true });
}

/** 도형 단추는 도크와 떠 있는 줄 **둘 다**에 선다 — 첫 번째를 누른다. */
function shapeButton(): HTMLElement {
  const all = screen.getAllByTestId('canvas-shape-tool');
  const first = all[0];
  if (first === undefined) throw new Error('도형 단추가 없다');
  return first;
}

function tap(x: number, y: number): void {
  fireEvent(screen.getByTestId('canvas-edit-overlay'), pointerEvent('pointerdown', x, y));
}

function live(): CanvasNode[] {
  return JSON.parse(screen.getByTestId('dump').textContent ?? '[]') as CanvasNode[];
}

function madeShape(): CanvasNode | undefined {
  return live().find((n) => n.kind === 'path');
}

const lookup = (tree: unknown, key: string): unknown =>
  key.split('.').reduce<unknown>(
    (n, seg) => (n && typeof n === 'object' ? (n as Record<string, unknown>)[seg] : undefined),
    tree,
  );

beforeEach(() => globalThis.localStorage.clear());
afterEach(() => {
  cleanup();
  globalThis.localStorage.clear();
  vi.restoreAllMocks();
});

// --- ① 모드 (REQ-01) --------------------------------------------------------

describe('도형 모드를 켜고 끈다 (REQ-01)', () => {
  it('단추가 서고, 처음에는 꺼져 있다', () => {
    show();
    expect(shapeButton().getAttribute('aria-pressed')).toBe('false');
  });

  it('**도크와 떠 있는 줄 둘 다**에 선다 (불변식 I23)', () => {
    show();
    // **연결선 도구와 같은 수만큼** 선다. 절대 수를 적지 않는 것에 뜻이 있다 — 이 시험
    // 틀에서 도크가 몇 벌 서는지는 이 SPEC 이 정한 것이 아니고, 정해야 할 것은 "도형
    // 단추가 연결선 도구와 **같은 자리들**에 선다" 이다. 한쪽에만 서면 대시보드에 놓인
    // 패널에서 도형을 그릴 수 없고, 그것이 006 이 배달한 결함의 형상이다(불변식 I23).
    expect(screen.getAllByTestId('canvas-shape-tool')).toHaveLength(
      screen.getAllByTestId('canvas-connector-tool-straight').length,
    );
  });

  it('누르면 켜지고 다시 누르면 꺼진다', () => {
    show();
    fireEvent.click(shapeButton());
    expect(shapeButton().getAttribute('aria-pressed')).toBe('true');
    fireEvent.click(shapeButton());
    expect(shapeButton().getAttribute('aria-pressed')).toBe('false');
  });

  it('**모양을 정할 도구가 함께 켜진다** — 눌러도 아무 일 없는 단추를 만들지 않는다', () => {
    show();
    fireEvent.click(shapeButton());
    const straight = screen.getAllByTestId('canvas-connector-tool-straight')[0];
    expect(straight?.getAttribute('aria-pressed')).toBe('true');
  });

  it('이미 도구가 켜져 있으면 **그것을 빼앗지 않는다**', () => {
    show();
    fireEvent.click(screen.getAllByTestId('canvas-connector-tool-ortho')[0]!);
    fireEvent.click(shapeButton());
    expect(screen.getAllByTestId('canvas-connector-tool-ortho')[0]?.getAttribute('aria-pressed')).toBe(
      'true',
    );
  });

  it.each(['ko', 'en'] as const)('%s — 이름이 번역된다', (locale) => {
    show(locale);
    const label = lookup(locale === 'ko' ? ko : en, 'dashboard.canvas.edit.toolShape');
    expect(label).toBeTypeOf('string');
    expect(shapeButton().getAttribute('aria-label')).toBe(label);
  });
});

// --- ② 찍기와 밑그림 (REQ-02 · REQ-04) --------------------------------------

describe('점을 찍어 윤곽을 그린다 (REQ-02 · REQ-04)', () => {
  it('찍으면 꼭짓점이 선다', () => {
    show();
    fireEvent.click(shapeButton());
    expect(screen.queryByTestId('canvas-shape-preview')).toBeNull();
    tap(100, 100);
    expect(screen.getByTestId('canvas-shape-vertex-0')).toBeTruthy();
    tap(200, 100);
    expect(screen.getByTestId('canvas-shape-vertex-1')).toBeTruthy();
  });

  it('밑그림 선이 함께 보인다', () => {
    show();
    fireEvent.click(shapeButton());
    tap(100, 100);
    expect(screen.getByTestId('canvas-shape-preview')).toBeTruthy();
  });

  it('**모드가 꺼져 있으면 아무 일도 없다** (K4)', () => {
    show();
    tap(100, 100);
    expect(screen.queryByTestId('canvas-shape-preview')).toBeNull();
    expect(madeShape()).toBeUndefined();
  });

  it('**연결선 도구를 켠 채 모드가 꺼져 있으면** 꼭짓점이 서지 않는다 (K4)', () => {
    // 이것이 K4 가 지키는 자리다. 기본 도구(`select`)에서만 재면 "모드를 보지 않는다" 는
    // 변이가 같은 결과를 내어 아무도 울지 않는다 — 그때는 모양이 `null` 이라 갈래가 어차피
    // 지나가기 때문이다. 모양이 **정해져 있는데** 모드가 꺼진 자리를 재야 문다.
    show();
    fireEvent.click(screen.getAllByTestId('canvas-connector-tool-straight')[0]!);
    tap(100, 100);
    expect(screen.queryByTestId('canvas-shape-preview')).toBeNull();
    expect(screen.queryByTestId('canvas-shape-vertex-0')).toBeNull();
    expect(madeShape()).toBeUndefined();
  });

  it('**앵커 위를 눌러도 꼭짓점이다** — 도형은 무엇에도 매이지 않는다 (REQ-02)', () => {
    show();
    fireEvent.click(shapeButton());
    // 상자 a 의 동쪽 앵커 자리(60, 35) 를 누른다.
    tap(60, 35);
    expect(screen.getByTestId('canvas-shape-vertex-0')).toBeTruthy();
    // 연결선은 나지 않았다.
    expect(live().some((n) => n.kind === 'connector')).toBe(false);
  });
});

// --- ③ 닫기 (REQ-03 · REQ-06) -----------------------------------------------

describe('시작점을 다시 누르면 닫힌다 (REQ-03)', () => {
  function drawTriangle(): void {
    fireEvent.click(shapeButton());
    tap(100, 100);
    tap(200, 100);
    tap(150, 200);
    tap(100, 100);
  }

  it('도형 하나가 생기고 밑그림이 사라진다', () => {
    show();
    drawTriangle();
    const made = madeShape();
    expect(made).toBeDefined();
    expect(screen.queryByTestId('canvas-shape-preview')).toBeNull();
  });

  it('만들어진 것은 **그린 자리**에 있다 — 씨앗 자리로 뛰지 않는다', () => {
    show();
    drawTriangle();
    const made = madeShape()!;
    if (made.kind !== 'path') throw new Error('경로가 아니다');
    expect(made.geometry).toEqual({ x: 100, y: 100, w: 100, h: 100 });
  });

  it('**만들자마자 골라진다** — 팔레트·카탈로그와 같은 세 줄이다', () => {
    show();
    drawTriangle();
    expect(screen.getByTestId('picked').textContent).toBe(madeShape()?.id);
  });

  it('**꼭짓점이 둘이면 닫히지 않는다** — 겹친 선은 도형이 아니다', () => {
    show();
    fireEvent.click(shapeButton());
    tap(100, 100);
    tap(200, 100);
    tap(100, 100);
    expect(madeShape()).toBeUndefined();
    // 세 번째 누름은 **꼭짓점으로 남는다** — 닫지 못한 누름을 버리지 않는다.
    expect(screen.getByTestId('canvas-shape-vertex-2')).toBeTruthy();
  });

  it('시작점이 아닌 곳을 누르면 **꼭짓점이 더 늘 뿐**이다', () => {
    show();
    fireEvent.click(shapeButton());
    tap(100, 100);
    tap(200, 100);
    tap(150, 200);
    tap(120, 150);
    expect(madeShape()).toBeUndefined();
    expect(screen.getByTestId('canvas-shape-vertex-3')).toBeTruthy();
  });
});

// --- ④ 버리기 (REQ-05) ------------------------------------------------------

describe('버릴 수 있다 (REQ-05)', () => {
  it('Escape 를 누르면 밑그림이 사라지고 **아무것도 만들어지지 않는다**', () => {
    show();
    fireEvent.click(shapeButton());
    tap(100, 100);
    tap(200, 100);
    fireEvent.keyDown(screen.getByTestId('canvas-edit-overlay'), { key: 'Escape' });
    expect(screen.queryByTestId('canvas-shape-preview')).toBeNull();
    expect(madeShape()).toBeUndefined();
  });

  it('모드를 끄면 밑그림이 사라진다 — 다시 켜도 딸려 오지 않는다', () => {
    show();
    fireEvent.click(shapeButton());
    tap(100, 100);
    tap(200, 100);
    fireEvent.click(shapeButton());
    fireEvent.click(shapeButton());
    expect(screen.queryByTestId('canvas-shape-vertex-0')).toBeNull();
  });
});
