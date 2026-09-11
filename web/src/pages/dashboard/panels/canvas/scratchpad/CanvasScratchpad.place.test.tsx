// 스크래치패드에서 놓기 (SPEC-CANVAS-008 M10 · REQ-04 · AC-07 · AC-06).
//
// 무게중심 셋:
//   ① **상대 배치가 산다** — 저장한 묶음의 요소 사이 변위가 놓은 뒤에도 같다.
//   ② **붙임은 기존 규칙 하나뿐이다** — 결과가 `snapDelta` 가 내는 값과 **정확히** 같다.
//      두 번째 붙임 계산이 생기면 그 값이 갈라지고 이 시험이 빨개진다.
//   ③ **놓은 id 전부가 선택이 된다** — 그것이 평평한 붙여넣기가 치른 값의 유일한 완화이며,
//      그 완화가 실제로 도는지는 "놓자마자 방향키 한 번이 둘 다 옮기는가" 로만 확인된다.
//
// 작업 영역을 **켜고** 잰다(시험 규율 D8): `origin ≠ (0,0)` 임을 먼저 단언한다. 놓기는
// 오버레이 루트의 상자와 `stagePoint` 만 쓰므로 그 원점에 흔들리지 않아야 하고, 원점을
// (0,0) 으로 두면 "흔들리지 않는다" 를 잴 수 없다.
//
// 축척은 가로 0.4 · 세로 0.25 로 갈린다. 같은 축척이면 축을 뒤바꾼 결함이 감춰진다.
//
// @spec SPEC-CANVAS-008 REQ-04 · AC-06 · AC-07

import fs from 'node:fs';
import path from 'node:path';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { useState } from 'react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { CanvasEditDockRegion } from '../CanvasEditDock';
import CanvasEditOverlay from '../CanvasEditOverlay';
import type { BoxGeometry, CanvasElement, CanvasSize } from '../canvasConfig';
import { CANVAS_GRID_STEP_UNITS, snapDelta } from '../canvasEditArrange';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from '../canvasEditContext';
import { seedOffset } from '../canvasElementFactory';
import type { StageSize } from '../canvasGeometry';
import { CanvasStageGridContext, type CanvasStageGrid } from '../canvasStageGrid';
import { useScratchpadStore } from './scratchpadStore';
import { cloneElements, elementsBounds, type ScratchpadEntry } from './scratchpadTypes';
import type { CanvasNode } from '../group/groupTypes';

// --- 고정 입력 -----------------------------------------------------------

const STAGE: StageSize = { width: 200, height: 100 };
const CANVAS: CanvasSize = { width: 500, height: 400 };

/** 작업 영역 원점. **(0,0) 이 아니다** — D8. */
const WORKSPACE_ORIGIN = { x: 30, y: 18 } as const;

/** 캔버스에 이미 있는 요소 하나. id 재발급이 실제로 비켜 가는지 재려면 하나는 있어야 한다. */
const EXISTING: CanvasElement = {
  id: 'el-1',
  kind: 'rect',
  geometry: { x: 20, y: 30, w: 40, h: 30 },
  style: { fill: '#c' },
};

/**
 * 저장된 묶음 — 요소 **둘**이고 상대 변위가 `(30, -20)` 이다. 하나짜리 묶음은 상대 배치가
 * 사는지 아닌지를 아예 묻지 못하고, 두 요소가 같은 자리면 번역의 결함이 감춰진다.
 */
const A_GEO: BoxGeometry = { x: 200, y: 150, w: 60, h: 40 };
const B_GEO: BoxGeometry = { x: 230, y: 130, w: 20, h: 20 };

const ENTRY: ScratchpadEntry = {
  id: 'sp-1',
  name: '펌프',
  created: 1_757_000_000_000,
  origin: { x: 200, y: 130 },
  elements: [
    { id: 'el-1', kind: 'rect', geometry: { ...A_GEO }, style: { fill: '#a' } },
    { id: 'el-2', kind: 'ellipse', geometry: { ...B_GEO }, style: { fill: '#b' } },
  ],
};

// --- 하네스 ---------------------------------------------------------------

/**
  * 마지막으로 화면에 걸린 **살아 있는** 요소 배열.
  *
  * DOM 에 찍은 JSON 을 되읽으면 `JSON.parse` 가 객체를 새로 만들므로 **참조가 언제나
  * 달라진다** — "서랍의 객체를 나눠 갖는가" 를 그 값으로 재면 시험이 공허해진다(실측:
  * 사본을 빼는 뮤테이션이 통과했다). 그래서 그 판정만은 이 배열로 잰다.
  */
let live: readonly CanvasNode[] = [];

function Harness() {
  const [elements, setElements] = useState<readonly CanvasNode[]>([EXISTING]);
  live = elements;
  const [step, setStep] = useState(CANVAS_GRID_STEP_UNITS);
  const state = useCanvasEditSelectionState();
  const grid: CanvasStageGrid = {
    step,
    setStep,
    zoom: 0.8,
    setZoom: () => {},
    cell: { x: 10, y: 6.25 },
    origin: { ...WORKSPACE_ORIGIN },
    box: { width: 260, height: 136 },
  };
  return (
    <CanvasStageGridContext value={grid}>
      <CanvasEditSelectionContext value={state}>
        <span data-testid="dump">{JSON.stringify(elements)}</span>
        <span data-testid="selection">{[...state.selection].join(',')}</span>
        <CanvasEditDockRegion enabled>
          <CanvasEditOverlay
            enabled
            elements={elements}
            projection={{ stage: STAGE, canvas: CANVAS }}
            textWidths={{}}
            onElementsChange={setElements}
          />
        </CanvasEditDockRegion>
      </CanvasEditSelectionContext>
    </CanvasStageGridContext>
  );
}

function liveElements(): CanvasElement[] {
  return JSON.parse(screen.getByTestId('dump').textContent ?? '[]') as CanvasElement[];
}

function selection(): string[] {
  const text = screen.getByTestId('selection').textContent ?? '';
  return text === '' ? [] : text.split(',');
}

function stubOverlayRect(): void {
  vi.spyOn(screen.getByTestId('canvas-edit-overlay'), 'getBoundingClientRect').mockReturnValue({
    left: 0,
    top: 0,
    width: 200,
    height: 100,
    right: 200,
    bottom: 100,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  } as DOMRect);
}

/** 항목의 놓기 단추에서 시작해 캔버스의 `(x, y)` 에서 뗀다. */
function dragEntryTo(x: number, y: number, id = ENTRY.id): void {
  const button = screen.getByTestId(`canvas-scratchpad-place-${id}`);
  fireEvent(
    button,
    new MouseEvent('pointerdown', { clientX: 0, clientY: 0, bubbles: true, cancelable: true }),
  );
  fireEvent(
    button,
    new MouseEvent('pointerup', { clientX: x, clientY: y, bubbles: true, cancelable: true }),
  );
  // 브라우저는 잡힌 포인터의 뗌 뒤에도 누름을 이 단추로 보낸다 — 그 꼬리까지 흉내낸다.
  fireEvent.click(button);
}

/** 격자 붙임을 켠다(도크의 그 토글 하나가 표시와 붙임을 함께 켠다). */
function turnSnapOn(): void {
  fireEvent.click(screen.getByTestId('canvas-grid-toggle'));
}

beforeEach(() => {
  localStorage.clear();
  useScratchpadStore.setState({ entries: [cloneEntry()], notice: null });
  localStorage.clear();
});

function cloneEntry(): ScratchpadEntry {
  return { ...ENTRY, elements: cloneElements(ENTRY.elements) };
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// --- AC-07 끌어 놓기 ------------------------------------------------------

describe('항목을 캔버스로 끌어 놓는다 (AC-07 · REQ-04)', () => {
  it('작업 영역이 켜져 있다 — 원점이 (0,0) 이 아니다 (시험 규율 D8)', () => {
    // 이 단언이 먼저 서지 않으면 아래 시험들은 "원점에 흔들리지 않는다" 를 재지 못한다.
    expect(WORKSPACE_ORIGIN).not.toEqual({ x: 0, y: 0 });
    render(<Harness />);
    // 그리고 그 원점이 실제로 화면에 걸려 있다.
    const gridBox = screen.getByTestId('canvas-workspace-grid');
    expect(gridBox.style.left).toBe(`-${WORKSPACE_ORIGIN.x}px`);
    expect(gridBox.style.top).toBe(`-${WORKSPACE_ORIGIN.y}px`);
  });

  it('상대 변위가 그대로 살고 묶음의 좌상단이 놓은 자리에 앉는다', () => {
    render(<Harness />);
    stubOverlayRect();

    // 클라이언트 (80, 40) → 스테이지 px 같은 값 → 캔버스 단위 (200, 160).
    dragEntryTo(80, 40);

    const placed = liveElements().slice(1);
    expect(placed).toHaveLength(2);
    const [a, b] = placed.map((el) => el.geometry as BoxGeometry);
    // ① 상대 변위 (30, -20) 이 그대로다.
    expect({ dx: b!.x - a!.x, dy: b!.y - a!.y }).toEqual({ dx: 30, dy: -20 });
    // ② 묶음의 좌상단이 놓은 자리다. 저장 좌상단 (200,130) → 놓은 자리 (200,160).
    const bounds = elementsBounds(placed);
    expect({ x: bounds.x, y: bounds.y }).toEqual({ x: 200, y: 160 });
  });

  it('id 는 새로 발급되어 기존 요소와 겹치지 않는다', () => {
    render(<Harness />);
    stubOverlayRect();
    dragEntryTo(80, 40);

    const ids = liveElements().map((el) => el.id);
    // 저장된 몸체의 id 는 `el-1`·`el-2` 이고 캔버스에 이미 `el-1` 이 있다. 그대로 썼다면
    // 파서가 중복을 먼저 온 것으로 접어 놓은 것이 조용히 사라진다.
    expect(new Set(ids).size).toBe(ids.length);
    expect(ids[0]).toBe('el-1');
    expect(ids.slice(1)).toEqual(['el-2', 'el-3']);
  });

  it('배열 끝에 **순서대로** 붙는다 — 저장할 때의 앞뒤가 살아난다', () => {
    render(<Harness />);
    stubOverlayRect();
    dragEntryTo(80, 40);

    expect(liveElements().map((el) => el.kind)).toEqual(['rect', 'rect', 'ellipse']);
  });

  it('놓은 직후 **만들어진 id 전부**가 선택이고, 방향키 한 번이 둘 다 옮긴다', () => {
    render(<Harness />);
    stubOverlayRect();
    dragEntryTo(80, 40);

    expect(selection()).toEqual(['el-2', 'el-3']);

    const before = liveElements().slice(1).map((el) => ({ ...(el.geometry as BoxGeometry) }));
    fireEvent.keyDown(screen.getByTestId('canvas-edit-overlay'), { key: 'ArrowRight' });
    const after = liveElements().slice(1).map((el) => el.geometry as BoxGeometry);

    // 둘 다 같은 델타로 움직였다 — 한 덩어리로 끌린다는 뜻이다.
    for (const [i, geo] of after.entries()) {
      expect(geo.x - before[i]!.x, `요소 ${i} 가 따라오지 않았다`).toBeGreaterThan(0);
      expect(geo.x - before[i]!.x).toBe(after[0]!.x - before[0]!.x);
    }
    // 이미 있던 요소는 움직이지 않는다.
    expect(liveElements()[0]?.geometry).toEqual(EXISTING.geometry);
  });

  it('캔버스 **밖**에서 떼면 그 자리에 놓지 않고 계단 자리로 떨어진다', () => {
    render(<Harness />);
    stubOverlayRect();
    // 클라이언트 (540, 40) 은 오버레이 상자(0..200 × 0..100) 밖이다.
    dragEntryTo(540, 40);

    // 놓기 자체는 뒤따르는 누름이 해낸다 — 계단 자리다.
    const off = seedOffset(1);
    const placed = liveElements().slice(1);
    expect(placed).toHaveLength(2);
    expect(placed[0]?.geometry).toEqual({ ...A_GEO, x: A_GEO.x + off, y: A_GEO.y + off });
  });
});

// --- 붙임 ---------------------------------------------------------------

describe('붙임은 기존 규칙 하나를 쓴다 (AC-07)', () => {
  it('결과가 `snapDelta` 가 내는 값과 **정확히** 같다', () => {
    render(<Harness />);
    stubOverlayRect();
    turnSnapOn();

    // 격자에 딱 떨어지지 않는 자리를 고른다 — 떨어지는 자리에서는 붙임이 있으나 없으나 같다.
    dragEntryTo(83, 41);

    // 시험이 스스로 기대값을 만든다: 같은 원 함수 · 같은 기준 상자 · 같은 간격.
    const point = { x: (83 / 200) * CANVAS.width, y: (41 / 100) * CANVAS.height };
    const raw = { dx: point.x - ENTRY.origin.x, dy: point.y - ENTRY.origin.y };
    const delta = snapDelta(raw, elementsBounds(ENTRY.elements), CANVAS_GRID_STEP_UNITS);

    const placed = liveElements().slice(1);
    expect(placed[0]?.geometry).toEqual({
      ...A_GEO,
      x: Math.round(A_GEO.x + delta.dx),
      y: Math.round(A_GEO.y + delta.dy),
    });
    // 붙임이 실제로 무언가를 했다 — 하지 않았다면 이 시험은 붙임을 재지 못한다.
    expect(delta).not.toEqual(raw);
  });

  it('붙임이 꺼져 있으면 놓은 자리 그대로다', () => {
    render(<Harness />);
    stubOverlayRect();
    dragEntryTo(83, 41);

    const point = { x: (83 / 200) * CANVAS.width, y: (41 / 100) * CANVAS.height };
    const raw = { dx: point.x - ENTRY.origin.x, dy: point.y - ENTRY.origin.y };
    const placed = liveElements().slice(1);
    expect(placed[0]?.geometry).toEqual({
      ...A_GEO,
      x: Math.round(A_GEO.x + raw.dx),
      y: Math.round(A_GEO.y + raw.dy),
    });
  });
});

// --- AC-06 끌지 않는 길 ---------------------------------------------------

describe('끌지 않고도 놓인다 (AC-06 · WCAG 2.2 SC 2.5.7)', () => {
  it('단추를 누르면 계단 자리에 놓이고 만들어진 id 전부가 선택된다', () => {
    render(<Harness />);
    stubOverlayRect();

    fireEvent.click(screen.getByTestId(`canvas-scratchpad-place-${ENTRY.id}`));

    const off = seedOffset(1);
    const placed = liveElements().slice(1);
    expect(placed[0]?.geometry).toEqual({ ...A_GEO, x: A_GEO.x + off, y: A_GEO.y + off });
    expect(placed[1]?.geometry).toEqual({ ...B_GEO, x: B_GEO.x + off, y: B_GEO.y + off });
    expect(selection()).toEqual(['el-2', 'el-3']);
  });

  it('잇달아 누르면 계단이 벌어져 정확히 겹치지 않는다', () => {
    render(<Harness />);
    stubOverlayRect();
    const button = screen.getByTestId(`canvas-scratchpad-place-${ENTRY.id}`);

    fireEvent.click(button);
    const first = (liveElements()[1]?.geometry as BoxGeometry).x;
    fireEvent.click(button);
    const second = (liveElements()[3]?.geometry as BoxGeometry).x;

    expect(second).not.toBe(first);
    expect(liveElements()).toHaveLength(5);
  });

  it('한 몸짓이 요소를 **두 벌** 만들지 않는다', () => {
    // 잡힌 포인터의 뗌 뒤에도 누름이 같은 단추로 오므로, 삼키지 않으면 끌어 놓기 한 번이
    // 요소 넷을 만든다.
    render(<Harness />);
    stubOverlayRect();
    dragEntryTo(80, 40);

    expect(liveElements()).toHaveLength(3);
  });

  it('놓아도 서랍의 항목은 그대로 남는다 — 놓기는 꺼내기가 아니다', () => {
    render(<Harness />);
    stubOverlayRect();
    dragEntryTo(80, 40);

    expect(useScratchpadStore.getState().entries).toHaveLength(1);
    expect(useScratchpadStore.getState().entries[0]?.elements[0]?.geometry).toEqual(A_GEO);
  });

  it('놓인 요소는 서랍의 객체를 **나눠 갖지 않는다**', () => {
    // 서랍은 영속되는 자료다. `style`·`path`·`rules` 를 그대로 나눠 가지면 캔버스 쪽에서
    // 그중 하나를 제자리에서 고치는 순간 서랍 속 원본이 함께 바뀐다. 기하는 쓰기 통로가
    // 어차피 새 값을 넣으므로, 이 결함이 드러나는 자리는 **기하가 아닌 필드**뿐이다.
    render(<Harness />);
    stubOverlayRect();
    dragEntryTo(80, 40);

    const stored = useScratchpadStore.getState().entries[0]?.elements ?? [];
    // **살아 있는 배열**로 잰다 — JSON 왕복을 지나면 참조가 언제나 새것이라 아무것도 재지
    // 못한다(위 `live` 주석).
    const placed = live.slice(1);
    expect(placed).toHaveLength(2);
    for (const [i, el] of placed.entries()) {
      expect(el.style, `요소 ${i} 의 style 을 서랍과 나눠 갖는다`).not.toBe(stored[i]?.style);
      expect(el, `요소 ${i} 자체를 서랍과 나눠 갖는다`).not.toBe(stored[i]);
      expect(el.style).toEqual(stored[i]?.style);
    }
  });

  it('놓은 요소를 고쳐도 서랍 속 원본이 따라 바뀌지 않는다', () => {
    render(<Harness />);
    stubOverlayRect();
    dragEntryTo(80, 40);
    // 놓인 요소를 방향키로 옮긴다(= 기하 쓰기).
    fireEvent.keyDown(screen.getByTestId('canvas-edit-overlay'), { key: 'ArrowRight' });

    expect(useScratchpadStore.getState().entries[0]?.elements[0]?.geometry).toEqual(A_GEO);
  });
});

// --- 형상 가드 -----------------------------------------------------------

describe('좌표 공간을 넘는 함수는 여전히 하나다 (AC-E7 · 불변식 J7)', () => {
  const source = (): string =>
    fs.readFileSync(path.resolve(__dirname, '../CanvasEditOverlay.tsx'), 'utf8');

  it('`stagePoint` 의 정의는 하나뿐이고 놓기는 그것을 부른다', () => {
    const text = source();
    expect(text.split('function stagePoint(').length - 1, '정의가 하나가 아니다').toBe(1);

    const start = text.indexOf('  const placeFromScratchpad = (');
    expect(start).toBeGreaterThanOrEqual(0);
    const body = text.slice(start, text.indexOf('\n  };', start));
    expect(body).toContain('stagePoint(');
    expect(body).toContain('snapDelta(');
  });

  it('화면 좌표를 스테이지로 옮기는 산술이 `stagePoint` 안에만 있다', () => {
    // 두 번째 변환이 생기면 이 수가 늘어난다. 나눗셈의 형상(`/ frame.scale…`)으로 센다.
    const text = source();
    expect(text.split('frame.scaleX').length - 1).toBe(1);
    expect(text.split('frame.scaleY').length - 1).toBe(1);
  });
});
