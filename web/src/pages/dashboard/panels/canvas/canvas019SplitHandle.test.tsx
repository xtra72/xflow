// 가운데 구간 손잡이의 몸짓 (SPEC-CANVAS-019 M3 · REQ-01 · REQ-02 · REQ-05).
//
// @spec SPEC-CANVAS-019

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
/** 축척이 하나이고 1:1 이다 — 시험이 환산을 들고 다니지 않는다. */
const PROJ: CanvasProjection = { stage: { width: 400, height: 300 }, canvas: CANVAS };

function nodes(over: Record<string, unknown> = {}): CanvasNode[] {
  return parseCanvasConfig({
    canvas: { ...CANVAS },
    elements: [
      { id: 'a', kind: 'rect', geometry: { x: 40, y: 40, w: 80, h: 50 }, style: { fill: '#39f' } },
      { id: 'b', kind: 'rect', geometry: { x: 230, y: 180, w: 80, h: 50 }, style: { fill: '#39f' } },
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
      <button type="button" data-testid="pick" onClick={() => state.setSelection(new Set(['c1']))}>
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

/** jsdom 은 레이아웃을 하지 않으므로 오버레이의 화면 자리를 직접 심는다(014 가 값을 치른 그 함정). */
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

/**
 * **SPEC-CANVAS-021 이 형상을 넓혔다** — 고정값은 수 하나가 아니라 **구간마다 하나**이고,
 * 손잡이의 이름에도 그 구간 번호가 붙는다(`…-handle-0`).
 *
 * 019 가 다루는 장면은 논리 구간이 하나뿐이므로 목록의 첫 자리가 019 의 그 수다 — 019 가
 * 물은 질문(서는가 · 끌리는가 · 눌러 되돌아가는가)은 한 글자도 달라지지 않는다.
 */
function splitOf(): number | undefined {
  const c = live().find((n) => n.id === 'c1');
  if (c === undefined || !isConnector(c)) return undefined;
  const first = c.ortho_split?.[0];
  return typeof first === 'number' ? first : undefined;
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

// --- ① 손잡이가 선다 (REQ-01 · §결정 3) -------------------------------------

describe('손잡이가 선다 (REQ-01)', () => {
  it('세 구간인 직각 선에 하나 선다', () => {
    show(nodes());
    expect(screen.getByTestId('canvas-ortho-split-handle-0')).toBeTruthy();
  });

  it('**곧은 선에는 서지 않는다** — 가운데 구간이 없다', () => {
    // 두 앵커의 `y` 가 같으면 018 이 곧은 두 점을 낸다. 그때 "가운데 구간" 이 무엇인지
    // 화면이 답하지 못하므로 손잡이도 없다.
    const flat = parseCanvasConfig({
      canvas: { ...CANVAS },
      elements: [
        { id: 'a', kind: 'rect', geometry: { x: 40, y: 40, w: 80, h: 50 }, style: {} },
        { id: 'b', kind: 'rect', geometry: { x: 230, y: 40, w: 80, h: 50 }, style: {} },
        { id: 'c1', kind: 'connector', from: { el: 'a', a: 'e' }, to: { el: 'b', a: 'w' }, route: 'ortho' },
      ],
    }).elements;
    show(flat);
    expect(screen.queryByTestId('canvas-ortho-split-handle-0')).toBeNull();
  });

  it('**꺾임이 둘을 넘으면 서지 않는다** — 옮길 "가운데" 가 하나가 아니다', () => {
    // 사이를 막으면 018 의 곧은 길이 닫히고 라우터가 계단을 낸다. 그때 구간은 셋이 아니라
    // 다섯이고, "가운데 직선 하나" 라는 말이 가리킬 곳이 없다 — 손잡이도 없다.
    //
    // 이 시험이 없으면 구간 수를 세는 `cmds.length !== 4` 를 `< 3` 으로 풀어도 아무도
    // 울지 않는다(곧은 선은 2점이라 양쪽 모두 걸러내므로). **세는 자를 재는 시험이다.**
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
    expect(screen.queryByTestId('canvas-ortho-split-handle-0')).toBeNull();
  });

  it('직각이 아니면 서지 않는다', () => {
    show(nodes({ route: 'elbow' }));
    expect(screen.queryByTestId('canvas-ortho-split-handle-0')).toBeNull();
  });

  it('8핸들·연결선 손잡이와 **이름 공간이 다르다**', () => {
    show(nodes());
    expect(screen.queryByTestId('canvas-handle-ortho-split-0')).toBeNull();
    expect(screen.queryByTestId('canvas-connector-handle-ortho-split-0')).toBeNull();
  });

  it.each(['ko', 'en'] as const)('%s — 이름이 번역된다', (locale) => {
    show(nodes(), locale);
    const label = lookup(locale === 'ko' ? ko : en, 'dashboard.canvas.edit.orthoSplitHandle');
    expect(label).toBeTypeOf('string');
    expect(screen.getByTestId('canvas-ortho-split-handle-0').getAttribute('aria-label')).toBe(label);
  });
});

// --- ② 끌면 옮긴다 (REQ-02) -------------------------------------------------

describe('끌면 가운데 구간이 따라온다 (REQ-02)', () => {
  async function drag(from: { x: number; y: number }, to: { x: number; y: number }): Promise<void> {
    const knob = screen.getByTestId('canvas-ortho-split-handle-0');
    const root = screen.getByTestId('canvas-edit-overlay');
    fireEvent(knob, pointerEvent('pointerdown', from.x, from.y));
    fireEvent(root, pointerEvent('pointermove', to.x, to.y));
    await nextFrame();
    fireEvent(root, pointerEvent('pointerup', to.x, to.y));
  }

  it('좌우로 끌면 그 x 가 실린다', async () => {
    show(nodes());
    const knob = screen.getByTestId('canvas-ortho-split-handle-0');
    const from = {
      x: Number.parseFloat(knob.style.left),
      y: Number.parseFloat(knob.style.top),
    };
    expect(splitOf()).toBeUndefined();
    await drag(from, { x: 200, y: from.y });
    expect(splitOf()).toBe(200);
  });

  it('**다른 축은 무시한다** — 세로로만 끌면 x 가 그대로다', async () => {
    show(nodes());
    const knob = screen.getByTestId('canvas-ortho-split-handle-0');
    const from = {
      x: Number.parseFloat(knob.style.left),
      y: Number.parseFloat(knob.style.top),
    };
    await drag(from, { x: 200, y: from.y + 60 });
    // 가로축 손잡이이므로 실리는 것은 x 뿐이다.
    expect(splitOf()).toBe(200);
  });
});

// --- ③ 누르면 되돌아간다 (REQ-05) -------------------------------------------

describe('누르기만 하면 자동으로 되돌아간다 (REQ-05)', () => {
  it('고정된 선을 누르면 키가 사라진다', async () => {
    // **더블클릭을 쓰지 않는다** — `onDoubleClick` 은 입력 장치에 따라 채워지지 않아
    // 몸짓이 조용히 죽고, 011 이 그 의존을 가드로 막아 두었다. 016 이 끝 손잡이에 대해
    // 세운 "움직이지 않았으면" 규칙을 여기서도 쓴다.
    show(nodes({ ortho_split: 190 }));
    expect(splitOf()).toBe(190);
    const knob = screen.getByTestId('canvas-ortho-split-handle-0');
    const root = screen.getByTestId('canvas-edit-overlay');
    const at = { x: Number.parseFloat(knob.style.left), y: Number.parseFloat(knob.style.top) };
    fireEvent(knob, pointerEvent('pointerdown', at.x, at.y));
    fireEvent(root, pointerEvent('pointerup', at.x, at.y));
    await nextFrame();
    const c = live().find((n) => n.id === 'c1')!;
    expect('ortho_split' in c).toBe(false);
  });
});
