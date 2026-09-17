// 구간마다 서는 손잡이와, 꺾임점을 지우는 차림표 (SPEC-CANVAS-021 M3 · M4).
//
// @spec SPEC-CANVAS-021

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useState } from 'react';
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';

import { I18nProvider } from '@/lib/i18n';
import en from '@/lib/i18n/en.json';
import ko from '@/lib/i18n/ko.json';

import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditDockRegion } from './CanvasEditDock';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import { parseCanvasConfig, type CanvasSize } from './canvasConfig';
import type { CanvasProjection } from './canvasGeometry';
import { isConnector } from './connector/connectorTypes';
import type { CanvasNode } from './group/groupTypes';

const CANVAS: CanvasSize = { width: 400, height: 300 };
const PROJ: CanvasProjection = { stage: { width: 400, height: 300 }, canvas: CANVAS };

function scene(over: Record<string, unknown> = {}): CanvasNode[] {
  return parseCanvasConfig({
    canvas: { ...CANVAS },
    elements: [
      { id: 'a', kind: 'rect', geometry: { x: 40, y: 40, w: 80, h: 50 }, style: {} },
      { id: 'b', kind: 'rect', geometry: { x: 230, y: 180, w: 80, h: 50 }, style: {} },
      {
        id: 'c1',
        kind: 'connector',
        from: { el: 'a', a: 'e' },
        to: { el: 'b', a: 'w' },
        route: 'ortho',
        style: { stroke: '#4a8', strokeWidth: 2 },
        ...over,
      },
    ],
  }).elements;
}

function Harness({ initial }: { initial: readonly CanvasNode[] }) {
  const [elements, setElements] = useState<readonly CanvasNode[]>(initial);
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <button type="button" data-testid="pick" onClick={() => state.setSelection(new Set(['c1']))} />
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

function show(initial: readonly CanvasNode[], locale: 'ko' | 'en' = 'ko'): void {
  cleanup();
  globalThis.localStorage.clear();
  globalThis.localStorage.setItem('xflow-locale', locale);
  render(
    <I18nProvider>
      <Harness initial={initial} />
    </I18nProvider>,
  );
  fireEvent.click(screen.getByTestId('pick'));
  stubRect();
}

/** `MouseEvent` 로 짓는다 — jsdom 의 `PointerEvent` 는 init 의 `clientX` 를 싣지 않는다. */
function pointerEvent(type: string, x: number, y: number): Event {
  return new MouseEvent(type, { clientX: x, clientY: y, bubbles: true, cancelable: true });
}

async function nextFrame(): Promise<void> {
  await act(async () => {
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  });
}

function live(): CanvasNode[] {
  return JSON.parse(screen.getByTestId('dump').textContent ?? '[]') as CanvasNode[];
}

function pointsOf(): unknown {
  const c = live().find((n) => n.id === 'c1');
  return c !== undefined && isConnector(c) ? c.points : undefined;
}

function splitsOf(): unknown {
  const c = live().find((n) => n.id === 'c1');
  return c !== undefined && isConnector(c) ? c.ortho_split : undefined;
}

function knobs(): HTMLElement[] {
  return screen.queryAllByTestId(/^canvas-ortho-split-handle-/);
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

// --- ① 구간마다 손잡이 (REQ-04 · K2) ----------------------------------------

describe('곧은 구간마다 손잡이가 선다 (REQ-04)', () => {
  it('점이 없으면 **하나** 선다 — 019 와 같은 답이다 (K2)', () => {
    show(scene());
    expect(knobs()).toHaveLength(1);
    expect(knobs()[0]?.getAttribute('data-testid')).toBe('canvas-ortho-split-handle-0');
  });

  it('**점이 하나면 손잡이가 는다** — 019 는 여기서 하나도 세우지 않았다', () => {
    show(scene({ points: [{ x: 175, y: 150 }] }));
    expect(knobs().length).toBeGreaterThan(1);
  });

  it('이름에 **구간 번호**가 붙는다 — 배열 차례가 아니다', () => {
    show(scene({ points: [{ x: 175, y: 150 }] }));
    const ids = knobs().map((k) => k.getAttribute('data-testid'));
    expect(ids).toContain('canvas-ortho-split-handle-0');
    expect(ids).toContain('canvas-ortho-split-handle-1');
  });

  it('직각이 아니면 하나도 서지 않는다', () => {
    show(scene({ route: 'elbow', points: [{ x: 175, y: 150 }] }));
    expect(knobs()).toHaveLength(0);
  });

  it('곧은 선에는 서지 않는다 — 가운데 구간이 없다', () => {
    const flat = parseCanvasConfig({
      canvas: { ...CANVAS },
      elements: [
        { id: 'a', kind: 'rect', geometry: { x: 40, y: 40, w: 80, h: 50 }, style: {} },
        { id: 'b', kind: 'rect', geometry: { x: 230, y: 40, w: 80, h: 50 }, style: {} },
        { id: 'c1', kind: 'connector', from: { el: 'a', a: 'e' }, to: { el: 'b', a: 'w' }, route: 'ortho' },
      ],
    }).elements;
    show(flat);
    expect(knobs()).toHaveLength(0);
  });

  it('**계단을 돌아간 구간에는 서지 않는다** (§결정 3)', () => {
    // 사이를 막으면 018 의 곧은 길이 닫히고 라우터가 계단을 낸다. 고정값은 구간당 하나인데
    // 계단은 꺾임이 여럿이라, 옮긴 뒤 무엇이 남는지 말할 수 없다.
    const blocked = parseCanvasConfig({
      canvas: { ...CANVAS },
      elements: [
        { id: 'a', kind: 'rect', geometry: { x: 20, y: 40, w: 60, h: 40 }, style: {} },
        { id: 'wall', kind: 'rect', geometry: { x: 150, y: 0, w: 40, h: 260 }, style: {} },
        { id: 'b', kind: 'rect', geometry: { x: 300, y: 40, w: 60, h: 40 }, style: {} },
        { id: 'c1', kind: 'connector', from: { el: 'a', a: 'e' }, to: { el: 'b', a: 'w' }, route: 'ortho' },
      ],
    }).elements;
    show(blocked);
    expect(knobs()).toHaveLength(0);
  });
});

// --- ② 끌기·되돌리기가 구간마다 (REQ-05 · REQ-06) ---------------------------

describe('구간마다 따로 옮기고 따로 되돌린다 (REQ-05 · REQ-06)', () => {
  async function dragKnob(segment: number, to: { x: number; y: number }): Promise<void> {
    const knob = screen.getByTestId(`canvas-ortho-split-handle-${segment}`);
    const root = screen.getByTestId('canvas-edit-overlay');
    const from = { x: Number.parseFloat(knob.style.left), y: Number.parseFloat(knob.style.top) };
    fireEvent(knob, pointerEvent('pointerdown', from.x, from.y));
    fireEvent(root, pointerEvent('pointermove', to.x, to.y));
    await nextFrame();
    fireEvent(root, pointerEvent('pointerup', to.x, to.y));
  }

  it('구간 1 을 끌면 **그 자리에만** 실린다', async () => {
    show(scene({ points: [{ x: 175, y: 150 }] }));
    const knob = screen.getByTestId('canvas-ortho-split-handle-1');
    const y = Number.parseFloat(knob.style.top);
    await dragKnob(1, { x: 215, y });
    expect(splitsOf()).toEqual([null, 215]);
  });

  it('누르기만 하면 **그 구간만** 자동으로 돌아간다', async () => {
    show(scene({ points: [{ x: 175, y: 150 }], ortho_split: [140, 210] }));
    const knob = screen.getByTestId('canvas-ortho-split-handle-0');
    const root = screen.getByTestId('canvas-edit-overlay');
    const at = { x: Number.parseFloat(knob.style.left), y: Number.parseFloat(knob.style.top) };
    fireEvent(knob, pointerEvent('pointerdown', at.x, at.y));
    fireEvent(root, pointerEvent('pointerup', at.x, at.y));
    await nextFrame();
    // 구간 0 만 자동이 되고 구간 1 의 210 은 **남는다**.
    expect(splitsOf()).toEqual([null, 210]);
  });
});

// --- ③ 지움 차림표 (REQ-01 · REQ-02 · REQ-03) -------------------------------

describe('꺾임점에 지움 차림표가 열린다 (REQ-01)', () => {
  function rightClickBend(index = 0): void {
    const knob = screen.getByTestId(`canvas-connector-handle-mid-${index}`);
    fireEvent.contextMenu(knob);
  }

  it('꺾임점 손잡이에서 오른쪽을 누르면 열린다', () => {
    show(scene({ points: [{ x: 175, y: 150 }] }));
    expect(screen.queryByTestId('canvas-point-menu')).toBeNull();
    rightClickBend();
    expect(screen.getByTestId('canvas-point-menu')).toBeTruthy();
  });

  it('**끝점에는 열리지 않는다** — 끝은 앵커에 매여 있다', () => {
    show(scene({ points: [{ x: 175, y: 150 }] }));
    fireEvent.contextMenu(screen.getByTestId('canvas-connector-handle-from'));
    expect(screen.queryByTestId('canvas-point-menu')).toBeNull();
    fireEvent.contextMenu(screen.getByTestId('canvas-connector-handle-to'));
    expect(screen.queryByTestId('canvas-point-menu')).toBeNull();
  });

  it.each(['ko', 'en'] as const)('%s — 줄의 문구가 번역된다', (locale) => {
    show(scene({ points: [{ x: 175, y: 150 }] }), locale);
    rightClickBend();
    const label = lookup(locale === 'ko' ? ko : en, 'dashboard.canvas.edit.removeBendPoint');
    expect(label).toBeTypeOf('string');
    expect(screen.getByTestId('canvas-point-menu-remove').textContent).toBe(label);
  });

  it('고르면 **그 점만** 빠지고 차림표가 닫힌다 (REQ-02)', () => {
    show(scene({ points: [{ x: 160, y: 65 }, { x: 175, y: 190 }] }));
    rightClickBend(0);
    fireEvent.click(screen.getByTestId('canvas-point-menu-remove'));
    expect(pointsOf()).toEqual([{ x: 175, y: 190 }]);
    expect(screen.queryByTestId('canvas-point-menu')).toBeNull();
  });

  it('마지막 하나를 빼면 **키가 사라진다** — 연타와 같은 결과다 (K3)', () => {
    show(scene({ points: [{ x: 175, y: 150 }] }));
    rightClickBend(0);
    fireEvent.click(screen.getByTestId('canvas-point-menu-remove'));
    const c = live().find((n) => n.id === 'c1')!;
    expect('points' in c).toBe(false);
  });

  it('바깥을 누르면 아무 일 없이 닫힌다 (REQ-03)', () => {
    show(scene({ points: [{ x: 175, y: 150 }] }));
    rightClickBend();
    fireEvent(
      screen.getByTestId('canvas-edit-overlay'),
      pointerEvent('pointerdown', 10, 280),
    );
    expect(screen.queryByTestId('canvas-point-menu')).toBeNull();
    expect(pointsOf()).toEqual([{ x: 175, y: 150 }]);
  });

  it('Escape 를 누르면 아무 일 없이 닫힌다 (REQ-03)', () => {
    show(scene({ points: [{ x: 175, y: 150 }] }));
    rightClickBend();
    fireEvent.keyDown(screen.getByTestId('canvas-edit-overlay'), { key: 'Escape' });
    expect(screen.queryByTestId('canvas-point-menu')).toBeNull();
    expect(pointsOf()).toEqual([{ x: 175, y: 150 }]);
  });

  it('**조작키를 짚은 Escape 도 닫는다** — 닫는 몸짓은 무엇을 짚든 닫는다', () => {
    show(scene({ points: [{ x: 175, y: 150 }] }));
    rightClickBend();
    fireEvent.keyDown(screen.getByTestId('canvas-edit-overlay'), { key: 'Escape', ctrlKey: true });
    expect(screen.queryByTestId('canvas-point-menu')).toBeNull();
  });
});
