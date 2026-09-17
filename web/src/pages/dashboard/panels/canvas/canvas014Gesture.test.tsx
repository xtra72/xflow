// 회전 손잡이와 돌아간 것의 크기 조절 (SPEC-CANVAS-014 M7 · M9 · AC-21 · AC-25~AC-27).
//
// @spec SPEC-CANVAS-014 REQ-02 · REQ-05

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useState } from 'react';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';

import { I18nProvider } from '@/lib/i18n';
import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';

import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditDockRegion } from './CanvasEditDock';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import type { CanvasElement } from './canvasConfig';
import type { CanvasProjection } from './canvasGeometry';
import type { CanvasNode } from './group/groupTypes';
import { resizeRotatedBox } from './canvasEditGeometry';
import { rotatePoint } from './canvasRotation';

/** 축척이 하나다(A1) — px 와 캔버스 단위가 1:1 이라 시험이 환산을 들고 다니지 않는다. */
const PROJ: CanvasProjection = { stage: { width: 400, height: 400 }, canvas: { width: 400, height: 400 } };

const BOX = { x: 100, y: 100, w: 200, h: 100 } as const;

function rect(rotation?: number): CanvasElement {
  return {
    id: 'r1',
    kind: 'rect',
    style: {},
    geometry: { ...BOX },
    ...(rotation !== undefined ? { rotation } : {}),
  } as CanvasElement;
}

function Harness({ initial }: { initial: readonly CanvasNode[] }) {
  const [elements, setElements] = useState<readonly CanvasNode[]>(initial);
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <button
        type="button"
        data-testid="select-r1"
        onClick={() => state.setSelection(new Set(['r1']))}
      >
        pick
      </button>
      <span data-testid="dump">{JSON.stringify(elements)}</span>
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

/**
 * 포인터 사건 하나. **`MouseEvent` 로 짓는다** — jsdom 의 `PointerEvent` 는 init 의
 * `clientX`/`clientY` 를 싣지 않아, `fireEvent.pointerDown(el, { clientX })` 로는 좌표가
 * `undefined` 로 들어가고 그 자리에서 **모든 각도가 `NaN`** 이 된다(첫 판에서 실제로 그랬다).
 *
 * `pointerId` 는 `MouseEvent` 에 없는 필드라 심지 않는다 — 누름과 이동 양쪽이 `undefined`
 * 라 짝이 맞는다(출시된 오버레이 시험이 쓰는 그 형상 그대로다).
 */
function pointerEvent(type: string, x: number, y: number, init: MouseEventInit = {}): Event {
  return new MouseEvent(type, { clientX: x, clientY: y, bubbles: true, cancelable: true, ...init });
}

/**
 * jsdom 은 레이아웃을 하지 않으므로 오버레이의 화면 자리를 직접 심는다.
 *
 * **심지 않으면 몸짓 시험이 거짓 초록이 된다.** 사각형이 0 이면 축척이 무너져 포인터 자리가
 * `NaN` 이 되고, `NaN` 각도는 저장 규율상 "각도 없음" 으로 떨어진다 — "제자리로 돌아오면
 * 키가 사라진다" 가 **아무 일도 일어나지 않아서** 통과한다. 첫 판에서 실제로 그랬다.
 */
function stubOverlayRect(): void {
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

function show(nodes: readonly CanvasNode[], locale: 'ko' | 'en' = 'ko'): void {
  cleanup();
  globalThis.localStorage.clear();
  globalThis.localStorage.setItem('xflow-locale', locale);
  render(
    <I18nProvider>
      <Harness initial={nodes} />
    </I18nProvider>,
  );
  fireEvent.click(screen.getByTestId('select-r1'));
  stubOverlayRect();
}

function live(): CanvasNode[] {
  return JSON.parse(screen.getByTestId('dump').textContent ?? '[]') as CanvasNode[];
}

function rotationOf(): number | undefined {
  const n = live()[0];
  return n !== undefined && 'rotation' in n ? (n as { rotation?: number }).rotation : undefined;
}

const lookup = (tree: unknown, key: string): unknown =>
  key.split('.').reduce<unknown>(
    (node, seg) => (node && typeof node === 'object' ? (node as Record<string, unknown>)[seg] : undefined),
    tree,
  );

/** 이동은 프레임당 한 번으로 모인다 — 프레임을 기다려야 실제와 같은 시점이다. */
async function nextFrame(): Promise<void> {
  await act(async () => {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  });
}

beforeEach(() => globalThis.localStorage.clear());
afterEach(() => {
  cleanup();
  globalThis.localStorage.clear();
  vi.restoreAllMocks();
});

// --- ① 손잡이가 선다 (AC-25) ------------------------------------------------

describe('회전 손잡이 (AC-25 · AC-27)', () => {
  it('고른 상자형에 선다', () => {
    show([rect()]);
    expect(screen.getByTestId('canvas-rotate-handle')).toBeTruthy();
  });

  it('**선에는 서지 않는다** — 선의 각도는 두 끝점이 말한다 (§D6)', () => {
    const line: CanvasElement = {
      id: 'r1',
      kind: 'line',
      style: {},
      geometry: { x1: 10, y1: 10, x2: 90, y2: 90 },
    };
    show([line]);
    // 눌러도 아무 일이 없는 단추를 세우지 않는다.
    expect(screen.queryByTestId('canvas-rotate-handle')).toBeNull();
  });

  it('8핸들과 **이름 공간이 다르다**', () => {
    show([rect()]);
    // 섞으면 8핸들을 세는 출시된 가드들이 이 손잡이를 함께 세게 된다.
    expect(screen.queryByTestId('canvas-handle-rotate')).toBeNull();
    expect(screen.getByTestId('canvas-rotate-handle')).toBeTruthy();
  });

  it.each(['ko', 'en'] as const)('%s — 이름이 번역된다 (AC-27)', (locale) => {
    show([rect()], locale);
    const label = lookup(locale === 'ko' ? ko : en, 'dashboard.canvas.edit.rotateHandle');
    expect(label).toBeTypeOf('string');
    expect(screen.getByTestId('canvas-rotate-handle').getAttribute('aria-label')).toBe(label);
  });

  it('손잡이가 **상자와 함께 돈다**', () => {
    // 축-나란 자리에 두면 90° 돌린 뒤 손잡이가 옆구리에 붙는다.
    show([rect()]);
    const flat = screen.getByTestId('canvas-rotate-handle').style.left;
    cleanup();
    show([rect(90)]);
    const turned = screen.getByTestId('canvas-rotate-handle').style.left;
    expect(turned).not.toBe(flat);
  });
});

// --- ② 끌면 각도가 따라온다 (AC-25 · AC-26) --------------------------------

describe('끌면 각도가 따라온다 (REQ-02)', () => {
  /** 축은 상자 가운데 (200, 150) 이다. */
  const PIVOT = { x: 200, y: 150 };

  /**
   * 손잡이를 잡아 끈다. 누름은 **손잡이에**, 이동은 **루트에** 간다 — 실제 브라우저에서
   * `setPointerCapture` 가 이후 이벤트를 루트로 보내는 그 경로다(출시된 오버레이 시험이
   * 8핸들에 대해 쓰는 그 형상 그대로다).
   */
  async function drag(
    from: { x: number; y: number },
    to: { x: number; y: number },
    shift = false,
  ): Promise<void> {
    const knob = screen.getByTestId('canvas-rotate-handle');
    const root = screen.getByTestId('canvas-edit-overlay');
    fireEvent(knob, pointerEvent('pointerdown', from.x, from.y));
    fireEvent(root, pointerEvent('pointermove', to.x, to.y, { shiftKey: shift }));
    await nextFrame();
    // **놓을 때도 Shift 를 쥔 채다.** 놓기가 마지막 한 번을 다시 흘리므로(`finishDrag`),
    // 여기서 Shift 를 떼면 눈금이 풀린 값으로 확정된다 — 실제 사용자도 눈금에 붙이려면
    // 놓는 순간까지 쥐고 있다.
    fireEvent(root, pointerEvent('pointerup', to.x, to.y, { shiftKey: shift }));
  }

  /** 손잡이를 잡는 자리 — 축 바로 위(각도 −90°). */
  const ABOVE = { x: PIVOT.x, y: PIVOT.y - 70 };

  it('오른쪽으로 끌면 각도가 생긴다', async () => {
    show([rect()]);
    expect(rotationOf()).toBeUndefined();
    // 축의 오른쪽(각도 0°)으로 끌면 −90° 에서 0° 로 90 만큼 돈다.
    await drag(ABOVE, { x: PIVOT.x + 70, y: PIVOT.y });
    expect(rotationOf()).toBe(90);
  });

  it('Shift 는 **15° 눈금**이다 (AC-26)', async () => {
    show([rect()]);
    await drag(ABOVE, { x: PIVOT.x + 70, y: PIVOT.y + 3 }, true);
    const deg = rotationOf() ?? 0;
    expect(deg % 15).toBe(0);
    expect(deg).toBeGreaterThan(0);
  });

  it('제자리로 돌아오면 **키가 사라진다** (§결정 7)', async () => {
    show([rect(30)]);
    expect(rotationOf()).toBe(30);
    // 30° 돌아간 손잡이를 잡아 **축 바로 위**로 끌면 각도가 0 이 된다.
    const start = rotatePoint(ABOVE, PIVOT, 30);
    await drag(start, ABOVE);
    // 한 바퀴를 돌아 제자리로 온 요소가 `rotation: 0` 을 얻으면 저장이 014 이전과
    // 바이트 동일하지 않다.
    const n = live()[0]!;
    expect('rotation' in n).toBe(false);
  });
});

// --- ③ 돌아간 것의 크기 조절 (M7 · AC-21) ----------------------------------

describe('돌아간 것은 **제 축 방향으로** 늘어난다 (AC-21 · AC-22)', () => {
  it('각도 0 이면 `resizeBox` 와 바이트 동일하다 (K3)', () => {
    const a = resizeRotatedBox({ ...BOX }, 0, 'e', { x: 350, y: 150 });
    const b = resizeRotatedBox({ ...BOX }, 0, 'e', { x: 350, y: 150 });
    expect(a).toEqual(b);
    expect(a.w).toBe(250);
    expect(a.h).toBe(BOX.h);
  });

  it('90° 돌아간 요소의 오른쪽 핸들이 **세로**를 바꾼다', () => {
    // 90° 돌면 요소의 로컬 +x 축이 화면의 +y 를 향한다. 그래서 화면에서 아래로 끌면
    // 로컬에서는 오른쪽으로 끈 것이고, 폭이 늘어난다.
    const pivot = { x: BOX.x + BOX.w / 2, y: BOX.y + BOX.h / 2 };
    const screenPointer = rotatePoint({ x: 350, y: 150 }, pivot, 90);
    const out = resizeRotatedBox({ ...BOX }, 90, 'e', screenPointer);
    expect(out.w).toBeCloseTo(250, 6);
    expect(out.h).toBeCloseTo(BOX.h, 6);
  });

  it('고정 모서리가 **화면에서 미끄러지지 않는다**', () => {
    // `resizeBox` 는 반대쪽을 **로컬**에서 고정하는데 회전 축은 상자 가운데이므로, 크기가
    // 바뀌면 축도 옮겨 가 고정 모서리가 화면에서 기어간다. 그 밀기를 재는 단언이다.
    const deg = 37;
    const before = { ...BOX };
    const pivotBefore = { x: before.x + before.w / 2, y: before.y + before.h / 2 };
    // 'e' 손잡이의 고정 자리는 왼쪽 변 위의 한 점(left, top)이다.
    const fixedBefore = rotatePoint({ x: before.x, y: before.y }, pivotBefore, deg);

    const pointer = rotatePoint({ x: 380, y: 150 }, pivotBefore, deg);
    const after = resizeRotatedBox(before, deg, 'e', pointer);
    const pivotAfter = { x: after.x + after.w / 2, y: after.y + after.h / 2 };
    const fixedAfter = rotatePoint({ x: after.x, y: after.y }, pivotAfter, deg);

    expect(fixedAfter.x).toBeCloseTo(fixedBefore.x, 6);
    expect(fixedAfter.y).toBeCloseTo(fixedBefore.y, 6);
    // 그리고 실제로 늘어났다 — "아무것도 안 했다" 와 구분한다.
    expect(after.w).toBeGreaterThan(before.w);
  });
});
