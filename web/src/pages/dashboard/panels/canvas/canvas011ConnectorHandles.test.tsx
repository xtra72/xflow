// 고르기와 손잡이 — 끝점 · 중간점 (SPEC-CANVAS-011 M9 · AC-62~AC-66).
//
// ## 이 파일이 겨누는 이음매
//
// M9 는 연결선이 **잡히는** 자리다. 그 전까지 연결선은 그려지고(M6) 잡히고(M7) 그어졌지만
// (M8), 그어 놓은 뒤 고칠 길이 없었다. 여기서부터 손잡이가 서고, 그래서 새 실패 형상이
// 둘 열린다.
//
//   ① **보이는 자리와 잡히는 자리가 다르다.** 손잡이가 제 손으로 참조를 풀면 그린 선과
//      다른 곳에 선다 — 002 위험 R1 이며, AC-45 가 "해석은 한 함수뿐" 으로 막은 그것이다.
//      그래서 이 파일은 손잡이의 `left`/`top` 을 `resolveConnector` 의 출력과 **직접**
//      견준다. 층을 건너지 않는 시험으로는 덮이지 않는다 — 양쪽이 저마다 옳기 때문이다.
//
//   ② **8핸들이 연결선에 선다.** 연결선에는 늘릴 상자가 없으므로(REQ-07) 그 손잡이를
//      끌었을 때 무엇이 일어나는지 화면이 답하지 못한다. AC-63 이 그것을 금지한다.
//
// ## 고정 입력의 산술 (M8 의 그 좌표 규율 그대로)
//
// 스테이지 200×100 · 캔버스 500×400 → 축척 가로 0.4 · 세로 0.25.
//
//   r1   캔버스 (100,100)-(300,300) → px  40..120 × 25..75
//          e  캔버스 (300,200) → px (120,50)     w 캔버스 (100,200) → px ( 40,50)
//   r2   캔버스 (400, 40)-(480,120) → px 160..192 × 10..30
//          w  캔버스 (400, 80) → px (160,20)     e 캔버스 (480, 80) → px (192,20)
//   el-3 캔버스 (100,360)-(160,400) → px  40.. 64 × 90..100
//          n  캔버스 (130,360) → px ( 52,90)
//
//   중간점 캔버스 (320,160) → px (128,40) · (360,120) → px (144,30)
//
// el-3 의 `n` 은 다른 어느 앵커와도 집는 오차(9px)의 두 배 넘게 떨어져 있다 — 가까우면
// "el-3 의 앵커에 놓았다" 가 옳은 이유로 참이 되지 않는다.
//
// @spec SPEC-CANVAS-011 REQ-07 · REQ-07-a · REQ-07-b ·
//       AC-45 · AC-62 · AC-63 · AC-64 · AC-65 · AC-66

import { useState } from 'react';

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

// **키를 그대로 돌려주는 흉내를 쓰지 않는다.** 중간점 문구는 번호 치환자를 들고 있고,
// 키를 그대로 내면 그 치환이 **일어나든 말든** 시험이 같은 값을 본다 — 세 손잡이가
// 구별 불가능한 이름을 읽는 결함이 조용히 지나간다. 그래서 실제 번역 트리를 조회한다.
vi.mock('@/lib/i18n', async () => {
  const tree = (await import('@/lib/i18n/ko.json')).default as Record<string, unknown>;
  const t = (key: string): string => {
    const value = key
      .split('.')
      .reduce<unknown>(
        (node, seg) =>
          node && typeof node === 'object' ? (node as Record<string, unknown>)[seg] : undefined,
        tree,
      );
    return typeof value === 'string' ? value : key;
  };
  return { useTranslation: () => ({ t }) };
});

import type { CanvasElement, CanvasSize } from './canvasConfig';
import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import { projectPoint, type CanvasProjection, type StageSize } from './canvasGeometry';
import {
  CONNECTOR_KIND,
  isConnector,
  type ConnectorElement,
} from './connector/connectorTypes';
import { resolveConnector } from './connector/resolveConnector';
import type { CanvasNode } from './group/groupTypes';

// --- 고정 입력 -------------------------------------------------------------

const STAGE: StageSize = { width: 200, height: 100 };
const CANVAS: CanvasSize = { width: 500, height: 400 };
const PROJ: CanvasProjection = { stage: STAGE, canvas: CANVAS };

const R1: CanvasElement = {
  id: 'r1',
  kind: 'rect',
  geometry: { x: 100, y: 100, w: 200, h: 200 },
  style: { fill: '#111' },
};

const R2: CanvasElement = {
  id: 'r2',
  kind: 'rect',
  geometry: { x: 400, y: 40, w: 80, h: 80 },
  style: { fill: '#222' },
};

/** AC-64 가 이름으로 겨누는 셋째 도형. */
const EL3: CanvasElement = {
  id: 'el-3',
  kind: 'rect',
  geometry: { x: 100, y: 360, w: 60, h: 40 },
  style: { fill: '#333' },
};

const SEED_STYLE = { stroke: '#3b82f6', strokeWidth: 2 } as const;

/** 중간점 **둘**인 연결선 — AC-62 가 손잡이 넷을 세는 그 선이다. */
const C_MID: ConnectorElement = {
  id: 'c-mid',
  kind: CONNECTOR_KIND,
  from: { el: 'r1', a: 'e' },
  to: { el: 'r2', a: 'w' },
  route: 'elbow',
  points: [
    { x: 320, y: 160 },
    { x: 360, y: 120 },
  ],
  style: { ...SEED_STYLE },
};

/** 중간점이 **하나도 없는** 연결선 — 손잡이가 둘뿐이어야 한다. */
const C_PLAIN: ConnectorElement = {
  id: 'c-plain',
  kind: CONNECTOR_KIND,
  from: { el: 'r1', a: 'e' },
  to: { el: 'r2', a: 'w' },
  route: 'straight',
  style: { ...SEED_STYLE },
};

/** 가리킨 요소가 배열에 없는 선 — 해석이 부재다(REQ-08). */
const C_BROKEN: ConnectorElement = {
  id: 'c-broken',
  kind: CONNECTOR_KIND,
  from: { el: 'ghost', a: 'e' },
  to: { el: 'r2', a: 'w' },
  route: 'straight',
  style: { ...SEED_STYLE },
};

/** 화면 px 지점들 — 위 머리말의 산술에서 나온 값이다. */
const AT = {
  /** r1 의 `e` 앵커 = `C_MID`·`C_PLAIN` 의 시작 손잡이. */
  r1East: { x: 120, y: 50 },
  /** r1 의 `w` 앵커. */
  r1West: { x: 40, y: 50 },
  /** r2 의 `w` 앵커 = 두 선의 끝 손잡이. */
  r2West: { x: 160, y: 20 },
  /** r2 의 `e` 앵커. */
  r2East: { x: 192, y: 20 },
  /** el-3 의 `n` 앵커 — AC-64 가 끝점을 놓는 자리. */
  el3North: { x: 52, y: 90 },
  /** `C_MID` 의 첫 중간점. */
  mid0: { x: 128, y: 40 },
  /** `C_MID` 의 둘째 중간점. */
  mid1: { x: 144, y: 30 },
  /** 어느 요소에도 어느 앵커에도 닿지 않는 자리. */
  empty: { x: 100, y: 5 },
  /** `C_PLAIN` 잉크의 중점 — 두 끝의 가운데다. */
  plainInk: { x: 140, y: 35 },
} as const;

// --- 하네스 ---------------------------------------------------------------

let live: readonly CanvasNode[] = [];

function Harness({ initial }: { initial: readonly CanvasNode[] }) {
  const [elements, setElements] = useState<readonly CanvasNode[]>(initial);
  live = elements;
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <span data-testid="selection">{[...state.selection].sort().join(',')}</span>
      {/* 고르는 몸짓을 재는 시험은 잉크를 누르고(AC-66), 손잡이를 재는 시험은 이 단추로
          곧장 고른다 — 손잡이의 형상을 재려는데 히트 오차가 전제에 끼면 그 시험이 틀린
          이유로 빨개진다. */}
      {initial.filter(isConnector).map((c) => (
        <button
          key={c.id}
          type="button"
          data-testid={`pick-${c.id}`}
          onClick={() => state.setSelection(new Set([c.id]))}
        />
      ))}
      <button
        type="button"
        data-testid="pick-r1"
        onClick={() => state.setSelection(new Set(['r1']))}
      />
      <CanvasEditOverlay
        enabled
        elements={elements}
        projection={PROJ}
        textWidths={{}}
        onElementsChange={setElements}
      />
    </CanvasEditSelectionContext>
  );
}

function overlayRoot(): HTMLElement {
  return screen.getByTestId('canvas-edit-overlay');
}

function setup(initial: readonly CanvasNode[]): void {
  live = initial;
  render(<Harness initial={initial} />);
  vi.spyOn(overlayRoot(), 'getBoundingClientRect').mockReturnValue({
    left: 0,
    top: 0,
    width: STAGE.width,
    height: STAGE.height,
    right: STAGE.width,
    bottom: STAGE.height,
    x: 0,
    y: 0,
    toJSON: () => ({}),
  } as DOMRect);
}

type Pt = { x: number; y: number };

function at(p: Pt): MouseEventInit {
  return { clientX: p.x, clientY: p.y, bubbles: true, cancelable: true, button: 0 };
}

function down(target: HTMLElement, p: Pt): void {
  fireEvent(target, new MouseEvent('pointerdown', at(p)));
}
function move(p: Pt): void {
  fireEvent(overlayRoot(), new MouseEvent('pointermove', at(p)));
}
function up(p: Pt): void {
  fireEvent(overlayRoot(), new MouseEvent('pointerup', at(p)));
}

function pick(id: string): void {
  fireEvent.click(screen.getByTestId(`pick-${id}`));
}

function handle(name: string): HTMLElement {
  return screen.getByTestId(`canvas-connector-handle-${name}`);
}

/** 손잡이를 잡아 끌어 놓는다 — 이 파일이 재는 그 한 몸짓. */
function dragHandle(name: string, to: Pt): void {
  const el = handle(name);
  const from = pointOf(el);
  down(el, from);
  move(to);
  up(to);
}

function pointOf(el: HTMLElement): Pt {
  return { x: Number.parseFloat(el.style.left), y: Number.parseFloat(el.style.top) };
}

function handleNames(): string[] {
  return screen
    .queryAllByTestId(/^canvas-connector-handle-/)
    .map((el) => el.getAttribute('data-testid')!.replace('canvas-connector-handle-', ''));
}

function boxHandleNames(): string[] {
  return screen
    .queryAllByTestId(/^canvas-handle-/)
    .map((el) => el.getAttribute('data-testid')!.replace('canvas-handle-', ''));
}

function connector(id: string): ConnectorElement {
  const found = live.find((n) => n.id === id);
  if (found === undefined || !isConnector(found)) throw new Error(`${id} 가 연결선이 아니다`);
  return found;
}

function selected(): string[] {
  const text = screen.getByTestId('selection').textContent ?? '';
  return text === '' ? [] : text.split(',');
}

/** 앵커 도구 — 켜야 앵커가 보이고, 보이는 앵커에만 끝점이 붙는다(위험 R1). */
function enableConnectorTool(): void {
  const button = screen.getByTestId('canvas-connector-tool-straight');
  fireEvent.click(button);
  expect(button.getAttribute('aria-pressed'), 'precondition: 도구가 켜졌다').toBe('true');
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  live = [];
});

// --- AC-62: 끝점과 중간점마다 손잡이 ----------------------------------------

describe('끝점과 중간점마다 손잡이가 선다 (AC-62 · REQ-07)', () => {
  it('중간점 둘인 연결선의 손잡이가 **넷**이다', () => {
    setup([R1, R2, EL3, C_MID]);
    expect(handleNames(), 'precondition: 고르기 전에는 없다').toEqual([]);
    pick('c-mid');
    expect(handleNames()).toEqual(['from', 'mid-0', 'mid-1', 'to']);
  });

  it('중간점이 **없으면** 둘뿐이다 — 끝점만 남는다', () => {
    setup([R1, R2, C_PLAIN]);
    pick('c-plain');
    expect(handleNames()).toEqual(['from', 'to']);
  });

  it('중간점 이름에 **번호가 붙는다** — 둘 이상일 때 겨눌 수단이 남는다', () => {
    setup([R1, R2, C_MID]);
    pick('c-mid');
    // 이름이 같은 단추가 둘이면 AC-65("그 점만 움직인다")를 잴 길이 없다.
    expect(new Set(handleNames()).size).toBe(handleNames().length);
  });

  it('고르기를 풀면 손잡이가 사라진다', () => {
    setup([R1, R2, C_MID]);
    pick('c-mid');
    expect(handleNames()).toHaveLength(4);
    down(overlayRoot(), AT.empty); // 빈 자리 누름 = 선택 비우기
    up(AT.empty);
    expect(selected()).toEqual([]);
    expect(handleNames()).toEqual([]);
  });

  it('손잡이는 진짜 `<button>` 이고 이름을 읽는다 — 칠한 그림이 아니다 (REQ-01)', () => {
    setup([R1, R2, C_MID]);
    pick('c-mid');
    for (const name of handleNames()) {
      const el = handle(name);
      expect(el.tagName, name).toBe('BUTTON');
      expect(el.getAttribute('aria-label'), name).toBeTruthy();
    }
    // 중간점 둘의 이름이 서로 다르다 — 치환자가 **실제로** 채워졌다.
    const first = handle('mid-0').getAttribute('aria-label')!;
    const second = handle('mid-1').getAttribute('aria-label')!;
    expect(first).not.toBe(second);
    expect(first).toContain('1');
    expect(second).toContain('2');
    // 벌거벗은 치환자가 화면에 남지 않는다 — `replaceAll` 을 지났다는 사실이다.
    for (const label of [first, second]) {
      expect(label, label).not.toContain('{index}');
      expect(label.startsWith('dashboard.'), label).toBe(false);
    }
  });
});

// --- AC-45 ①의 셋째: 자리가 `resolveConnector` 에서 나온다 -------------------

describe('손잡이 자리가 **그린 그 함수**에서 나온다 (AC-45 · 위험 R1)', () => {
  it('넷의 좌표가 해석된 점 목록을 투영한 값과 한 자리도 다르지 않다', () => {
    setup([R1, R2, EL3, C_MID]);
    pick('c-mid');

    const points = resolveConnector(C_MID, live, PROJ, {})!;
    expect(points, 'precondition: 해석이 넷을 낸다').toHaveLength(4);

    const names = ['from', 'mid-0', 'mid-1', 'to'];
    points.forEach((point, index) => {
      const expected = projectPoint(point, PROJ);
      expect(pointOf(handle(names[index]!)), names[index]).toEqual(expected);
    });
  });

  it('참조된 도형을 옮기면 끝 손잡이가 **따라간다** — 좌표를 적어 두지 않았다 (REQ-03-a)', () => {
    setup([R1, R2, C_PLAIN]);
    pick('c-plain');
    expect(pointOf(handle('from'))).toEqual(AT.r1East);

    // r1 을 몸통으로 잡아 오른쪽 아래로 옮긴다(캔버스 단위 +50,+40 → px +20,+10).
    pick('r1');
    down(overlayRoot(), { x: 80, y: 50 });
    move({ x: 100, y: 60 });
    up({ x: 100, y: 60 });

    pick('c-plain');
    expect(pointOf(handle('from'))).toEqual({ x: 140, y: 60 });
    // 저술에는 여전히 참조뿐이다.
    expect(connector('c-plain').from).toEqual({ el: 'r1', a: 'e' });
  });

  it('**끊긴 연결에는 손잡이가 서지 않는다** — 예외도 아니다 (REQ-08)', () => {
    setup([R1, R2, C_BROKEN]);
    expect(resolveConnector(C_BROKEN, live, PROJ, {}), 'precondition: 해석이 부재다').toBeUndefined();
    pick('c-broken');
    expect(selected()).toEqual(['c-broken']);
    expect(handleNames()).toEqual([]);
    expect(boxHandleNames()).toEqual([]);
    // 저술은 그대로다 — 그리지 않되 버리지 않는다.
    expect(connector('c-broken')).toEqual(C_BROKEN);
  });
});

// --- AC-63: 8핸들이 서지 않는다 ---------------------------------------------

describe('연결선에 8핸들이 서지 않는다 (AC-63 · REQ-07)', () => {
  it('`canvas-handle-nw` 따위가 하나도 없다', () => {
    setup([R1, R2, C_MID]);
    pick('c-mid');
    expect(boxHandleNames()).toEqual([]);
    expect(screen.queryByTestId('canvas-handle-nw')).toBeNull();
  });

  it('**그물이 성기지 않다** — 보통 도형에서는 여덟이 선다', () => {
    // 이 단언이 없으면 위 "없다" 는 조회 이름이 틀려도 초록이 된다.
    setup([R1, R2, C_MID]);
    pick('r1');
    expect(boxHandleNames().sort()).toEqual(['e', 'n', 'ne', 'nw', 's', 'se', 'sw', 'w']);
    expect(handleNames()).toEqual([]);
  });

  it('두 갈래가 **함께 서지 않는다** — 한 키가 둘 다를 채우지 못한다', () => {
    setup([R1, R2, C_MID]);
    for (const id of ['c-mid', 'r1']) {
      pick(id);
      const both = handleNames().length > 0 && boxHandleNames().length > 0;
      expect(both, id).toBe(false);
    }
  });
});

// --- AC-64: 끝점을 다른 앵커에 놓으면 참조가 갈린다 --------------------------

describe('끝점 손잡이를 다른 앵커에 놓으면 참조가 갈린다 (AC-64 · REQ-07-a)', () => {
  it('`el-3` 의 앵커에 놓으면 그 끝의 `el` 이 `el-3` 이다', () => {
    setup([R1, R2, EL3, C_PLAIN]);
    enableConnectorTool();
    pick('c-plain');
    expect(connector('c-plain').to, 'precondition: 처음에는 r2 를 가리킨다').toEqual({
      el: 'r2',
      a: 'w',
    });

    dragHandle('to', AT.el3North);

    expect(connector('c-plain').to).toEqual({ el: 'el-3', a: 'n' });
    // 반대 끝은 한 글자도 달라지지 않는다.
    expect(connector('c-plain').from).toEqual({ el: 'r1', a: 'e' });
  });

  it('시작 끝도 같은 규칙이다 — 두 끝이 한 함수를 지난다', () => {
    setup([R1, R2, EL3, C_PLAIN]);
    enableConnectorTool();
    pick('c-plain');
    dragHandle('from', AT.el3North);
    expect(connector('c-plain').from).toEqual({ el: 'el-3', a: 'n' });
    expect(connector('c-plain').to).toEqual({ el: 'r2', a: 'w' });
  });

  it('**반대 끝이 이미 가리키는 요소**의 앵커에도 붙는다 — 두 끝이 한 도형에 설 수 있다', () => {
    // M5 가 이미 푸는 형상이다(상자를 노드마다 한 번만 재는 그 장부가 이 경우를 위해 있다).
    // 막으면 한 상자의 `nw` 와 `se` 를 잇는 대각선이 표현 불가능해진다.
    setup([R1, R2, EL3, C_PLAIN]);
    enableConnectorTool();
    pick('c-plain');
    dragHandle('from', AT.r2East);

    expect(connector('c-plain').from).toEqual({ el: 'r2', a: 'e' });
    expect(connector('c-plain').to).toEqual({ el: 'r2', a: 'w' });
    // 해석도 멀쩡하다 — 끊긴 연결이 아니다.
    expect(resolveConnector(connector('c-plain'), live, PROJ, {})).toHaveLength(2);
  });

  it('**빈 곳에 놓으면 자유 끝점**이다 — 그은 몸짓과 같은 규칙이다 (AC-58 대칭)', () => {
    setup([R1, R2, EL3, C_PLAIN]);
    enableConnectorTool();
    pick('c-plain');
    dragHandle('to', AT.empty);

    // px (100,5) → 캔버스 (250,20). 정수로 죄어 실린다.
    expect(connector('c-plain').to).toEqual({ x: 250, y: 20 });
    expect(Object.keys(connector('c-plain').to).sort()).toEqual(['x', 'y']);
  });

  it('**연결선에는 붙지 않는다** — 앵커를 내는 목록에 선이 없다 (A6 · REQ-03-b)', () => {
    // 막는 자는 검사가 아니라 **타입**이다: `anchorHitAt` 의 인자가 `OutlinedNode[]` 이고
    // 오버레이가 넘기는 목록은 `isConnector` 로 걸러진 것이라, 연결선을 넘기는 것이
    // 컴파일되지 않는다. 그래서 다른 선의 잉크 한가운데에 놓아도 참조가 생기지 않는다.
    setup([R1, R2, EL3, C_PLAIN, C_MID]);
    enableConnectorTool();
    pick('c-mid');
    dragHandle('to', AT.plainInk);

    const end = connector('c-mid').to;
    expect(Object.keys(end).sort(), '자유 끝점이어야 한다').toEqual(['x', 'y']);
    expect('el' in end).toBe(false);
  });

  it('**도구가 꺼져 있으면 붙지 않는다** — 붙는 자리는 보이는 자리뿐이다 (위험 R1)', () => {
    // 이 단언은 결정을 **못박는 것**이다. 붙일 후보를 도구와 무관하게 전부 열면 화면에
    // 점 하나 없는 자리에서 선이 붙고, 사용자에게는 "가끔 달라붙는다" 로만 보인다. 그래서
    // 끝점이 보는 목록은 앵커를 **그리는 그 목록**이고(`anchorHosts`), 도구가 꺼져 있으면
    // 그 목록이 비어 있어 놓은 자리가 그대로 자유 끝점이 된다.
    //
    // 되붙이는 길은 닫히지 않는다 — 도구를 켜면 점이 보이고, 보이는 점에 붙는다.
    setup([R1, R2, EL3, C_PLAIN]);
    pick('c-plain');
    expect(screen.queryAllByTestId(/^canvas-anchor-dot-/), 'precondition: 점이 없다').toHaveLength(
      0,
    );

    dragHandle('to', AT.el3North);
    // px (52,90) → 캔버스 (130,360).
    expect(connector('c-plain').to).toEqual({ x: 130, y: 360 });
  });

  it('붙였다 떼는 길이 **한 몸짓**이다 — 다시 빈 곳에 놓으면 자유 끝점으로 돌아온다', () => {
    setup([R1, R2, EL3, C_PLAIN]);
    enableConnectorTool();
    pick('c-plain');
    dragHandle('to', AT.el3North);
    expect(connector('c-plain').to).toEqual({ el: 'el-3', a: 'n' });

    dragHandle('to', AT.empty);
    expect(connector('c-plain').to).toEqual({ x: 250, y: 20 });
  });
});

// --- AC-65: 중간점을 끌면 그 점만 움직인다 -----------------------------------

describe('중간점을 끌면 **그 점만** 움직인다 (AC-65)', () => {
  it('이웃의 좌표가 바뀌지 않는다 — 객체까지 그대로다', () => {
    setup([R1, R2, C_MID]);
    pick('c-mid');
    const before = connector('c-mid').points!;

    // px (144,30) → px (160,50) = 캔버스 (400,200).
    dragHandle('mid-1', { x: 160, y: 50 });

    const after = connector('c-mid').points!;
    expect(after).toHaveLength(2);
    expect(after[1]).toEqual({ x: 400, y: 200 });
    // **값이 아니라 객체로** 견준다 — 값만 같고 새 객체면 렌더 비교가 매 프레임 "달라졌다"
    // 고 읽는다.
    expect(after[0]).toBe(before[0]);
  });

  it('첫 점을 끌어도 같다 — 자리는 index 하나로만 갈린다', () => {
    setup([R1, R2, C_MID]);
    pick('c-mid');
    const before = connector('c-mid').points!;

    dragHandle('mid-0', { x: 80, y: 75 }); // 캔버스 (200,300)

    const after = connector('c-mid').points!;
    expect(after[0]).toEqual({ x: 200, y: 300 });
    expect(after[1]).toBe(before[1]);
  });

  it('**끝점은 건드리지 않는다** — 참조가 그대로다', () => {
    setup([R1, R2, C_MID]);
    pick('c-mid');
    dragHandle('mid-0', { x: 80, y: 75 });
    expect(connector('c-mid').from).toEqual({ el: 'r1', a: 'e' });
    expect(connector('c-mid').to).toEqual({ el: 'r2', a: 'w' });
  });

  it('저장값이 **캔버스 단위 정수**다 — 저장 왕복에 반 칸 밀리지 않는다 (A5 · AC-36)', () => {
    setup([R1, R2, C_MID]);
    pick('c-mid');
    // px (101,31) → 캔버스 (252.5, 124) — 가로가 소수다.
    dragHandle('mid-0', { x: 101, y: 31 });
    const moved = connector('c-mid').points![0]!;
    expect(Number.isInteger(moved.x), `x=${moved.x}`).toBe(true);
    expect(Number.isInteger(moved.y), `y=${moved.y}`).toBe(true);
    expect(moved).toEqual({ x: 253, y: 124 });
  });

  it('**도형은 한 글자도 달라지지 않는다** — 중간점은 절대 좌표라 아무것도 끌고 가지 않는다', () => {
    setup([R1, R2, C_MID]);
    pick('c-mid');
    dragHandle('mid-0', { x: 80, y: 75 });
    expect(live.filter((n) => !isConnector(n))).toEqual([R1, R2]);
  });
});

// --- AC-66: 선택 키가 평평하다 ----------------------------------------------

describe('연결선의 선택 키가 **평평하다** (AC-66)', () => {
  it('잉크를 눌러 고르면 키에 구분자가 없다', () => {
    setup([R1, R2, C_PLAIN]);
    down(overlayRoot(), AT.plainInk);
    up(AT.plainInk);
    expect(selected()).toEqual(['c-plain']);
    expect(selected()[0]).not.toContain('/');
  });

  it('그 키가 그대로 손잡이를 세운다 — 009 의 선택 모델을 지나간다', () => {
    setup([R1, R2, C_PLAIN]);
    down(overlayRoot(), AT.plainInk);
    up(AT.plainInk);
    expect(handleNames()).toEqual(['from', 'to']);
  });
});

// --- 손잡이가 다른 몸짓을 깨우지 않는다 --------------------------------------

describe('손잡이를 잡으면 **그것만** 일어난다', () => {
  it('사각형(마키)이 서지 않는다 — 누름이 루트에 닿지 않는다', () => {
    setup([R1, R2, C_MID]);
    pick('c-mid');
    const el = handle('mid-0');
    down(el, pointOf(el));
    move({ x: 60, y: 60 });
    expect(screen.queryByTestId('canvas-marquee')).toBeNull();
    up({ x: 60, y: 60 });
    expect(screen.queryByTestId('canvas-marquee')).toBeNull();
  });

  it('선택이 갈리지 않는다 — 손잡이는 이미 골라진 것에만 뜬다', () => {
    setup([R1, R2, C_MID]);
    pick('c-mid');
    dragHandle('mid-0', { x: 60, y: 60 });
    expect(selected()).toEqual(['c-mid']);
  });

  it('**무리 이동이 시작되지 않는다** — 손잡이 뒤의 도형이 따라 움직이지 않는다', () => {
    setup([R1, R2, C_MID]);
    pick('c-mid');
    // `from` 손잡이는 r1 의 `e` 앵커 위, 즉 r1 의 잉크 경계 위에 있다. 이벤트가 루트로
    // 새면 그 누름이 r1 을 새로 고르고 이동 드래그를 시작한다.
    dragHandle('from', { x: 100, y: 60 });
    expect(live.filter((n) => !isConnector(n))).toEqual([R1, R2]);
  });

  it('**앵커 도구가 켜지면 물러선다** — 끝 손잡이가 제 앵커를 가리지 않는다', () => {
    // 겹침이 가설이 아니라 사실임을 **먼저** 확인한다: 끝 손잡이는 제가 붙은 앵커와 정확히
    // 같은 자리에 선다. 그래서 손잡이가 포인터를 먹으면 그 앵커를 겨눈 누름이 손잡이에
    // 잡히고, 히트 층에서 막은 구멍(§앵커가 잉크를 이긴다)이 DOM 층에서 되살아난다.
    setup([R1, R2, EL3, C_PLAIN]);
    pick('c-plain');
    expect(handle('from').className, '도구가 꺼져 있으면 잡힌다').toContain('pointer-events-auto');

    fireEvent.click(screen.getByTestId('canvas-anchor-tool'));
    const dot = screen.getByTestId('canvas-anchor-dot-r1-e');
    expect(pointOf(dot), 'precondition: 손잡이와 앵커가 같은 자리다').toEqual(
      pointOf(handle('from')),
    );
    expect(handle('from').className).toContain('pointer-events-none');
    // 중간점도 같다 — 도구가 켜진 동안 이 표면은 앵커의 것이다.
    expect(handle('to').className).toContain('pointer-events-none');
  });

  it('긋는 몸짓도 시작되지 않는다 — 도구가 켜져 있어도 손잡이가 임자다', () => {
    setup([R1, R2, EL3, C_PLAIN]);
    enableConnectorTool();
    pick('c-plain');
    const before = live.filter(isConnector).length;
    dragHandle('to', AT.el3North);
    // 새 선이 생기지 않았다 — 있던 선의 끝이 갈렸을 뿐이다.
    expect(live.filter(isConnector)).toHaveLength(before);
    expect(screen.queryByTestId('canvas-connector-preview')).toBeNull();
  });
});

// --- 없애는 길이 **보인다** (사용자 신고 2026-09-16) --------------------------

describe('중간점 손잡이가 **제가 무엇인지** 말한다 (REQ-05-b)', () => {
  // 신고는 "중간점을 없앨 수 없다" 였고, 길은 M10 부터 있었다(그 손잡이 위의 더블클릭 —
  // `canvas011PointEdit.test.tsx` 가 그것을 재고 있다). 없던 것은 **길을 알릴 화면**이다.
  //
  // 사용자가 고른 고침은 "몸짓은 그대로 두고 보이게 하라" 였다. 없애는 두 번째 컨트롤을
  // 세우지 않는 근거는 SPEC 이 REQ-05-b 에 적어 두었다 — 그 컨트롤은 어디에 서는지 · 점이
  // 몰리면 무엇이 되는지 · 끝점 손잡이 밑에서 무엇이 되는지를 다시 묻게 한다.
  //
  // 그래서 이 절이 재는 것은 셋이다: **이름이 두 조작을 모두 말하는가** · **보는 사람에게도
  // 같은 말을 하는가**(`title`) · **끝점과 눈으로 갈리는가**.

  it('이름이 끄는 일과 두 번 누르는 일을 **모두** 말한다', () => {
    setup([R1, R2, EL3, C_MID]);
    pick('c-mid');
    const label = handle('mid-0').getAttribute('aria-label') ?? '';

    // 번호가 먼저 든다 — 점이 둘이면 이름도 둘이어야 한다(AC-62 의 그 성질).
    expect(label).toContain('1');
    // 끄는 일이 먼저다: 손이 먼저 닿는 조작이고, 빼기를 앞세우면 이름이 "지우는 단추" 로
    // 읽혀 끌어 옮기려던 손이 망설인다.
    expect(label).toContain('끌면');
    expect(label).toContain('두 번');
    expect(label.indexOf('끌면')).toBeLessThan(label.indexOf('두 번'));
  });

  it('두 점의 이름이 서로 다르다 — 번호가 실제로 채워진다', () => {
    setup([R1, R2, EL3, C_MID]);
    pick('c-mid');
    expect(handle('mid-0').getAttribute('aria-label')).not.toBe(
      handle('mid-1').getAttribute('aria-label'),
    );
  });

  it('`title` 이 **같은 문장**을 나른다 — 보는 사람에게도 길이 있다', () => {
    // `aria-label` 만 두면 그 사실이 보조기기를 쓰는 사람에게만 있고, 마우스를 쓰는
    // 사람에게는 길이 **없는 것과 같다** — 그것이 신고된 형상이다.
    setup([R1, R2, EL3, C_MID]);
    pick('c-mid');
    for (const name of ['from', 'mid-0', 'mid-1', 'to']) {
      const el = handle(name);
      expect(el.getAttribute('title'), name).toBe(el.getAttribute('aria-label'));
      expect((el.getAttribute('title') ?? '').length, name).toBeGreaterThan(0);
    }
  });

  it('끝점 이름은 **갈아 끼우는 일**을 말하고 빼기를 말하지 않는다', () => {
    // 끝점은 빠지지 않는다(REQ-07-a) — 두 끝이 없는 연결선은 표현될 수 없다. 이름이 그
    // 일을 말하면 화면이 지키지 못할 약속을 한다.
    setup([R1, R2, EL3, C_MID]);
    pick('c-mid');
    for (const name of ['from', 'to']) {
      const label = handle(name).getAttribute('aria-label') ?? '';
      expect(label, name).toContain('끌어서');
      expect(label.includes('두 번'), name).toBe(false);
    }
  });

  it('중간점은 **동그라미**, 끝점은 **네모**다 — 눈으로 갈린다', () => {
    setup([R1, R2, EL3, C_MID]);
    pick('c-mid');
    for (const name of ['mid-0', 'mid-1']) {
      expect(handle(name).className, name).toContain('rounded-full');
      expect(handle(name).className.includes('rounded-sm'), name).toBe(false);
    }
    for (const name of ['from', 'to']) {
      expect(handle(name).className, name).toContain('rounded-sm');
      expect(handle(name).className.includes('rounded-full'), name).toBe(false);
    }
  });

  it('갈리는 것은 **모양뿐**이다 — 크기·칠·초점 테두리는 한 벌이다', () => {
    // 몸통까지 갈리면 두 손잡이가 서로 다른 무리로 읽히고, 무엇보다 크기를 한 번 바꾸는
    // 날 둘이 어긋난다. 집는 오차(`ANCHOR_PICK_SLOP_PX`)는 그 크기에서 나온 하나뿐이다.
    setup([R1, R2, EL3, C_MID]);
    pick('c-mid');
    const shared = ['h-2.5', 'w-2.5', 'bg-blue-500', 'border-white', 'focus:ring-blue-300'];
    for (const name of ['from', 'mid-0', 'mid-1', 'to']) {
      for (const token of shared) {
        expect(handle(name).className, `${name}:${token}`).toContain(token);
      }
    }
  });

  it('커서는 여전히 **하나**다 — 끌면 무엇이 되는가에 대한 답은 둘이 같다', () => {
    // 중간점에 다른 커서를 주면 그 커서는 "이것은 끄는 것이 아니다" 를 뜻하게 되는데
    // 그것은 거짓이다. 끌기 말고도 할 수 있는 일은 커서가 아니라 이름이 나른다.
    setup([R1, R2, EL3, C_MID]);
    pick('c-mid');
    for (const name of ['from', 'mid-0', 'mid-1', 'to']) {
      expect(handle(name).className, name).toContain('cursor-move');
    }
  });

  it('없애는 **두 번째 컨트롤**이 서지 않았다 (REQ-05-b)', () => {
    // 손잡이 넷 말고 다른 단추가 서면 "점을 없애는 길" 이 둘이 되고, 그 둘은 자리도
    // 문턱도 따로 들게 된다. 넷이 전부임을 세어서 못박는다.
    setup([R1, R2, EL3, C_MID]);
    pick('c-mid');
    expect(handleNames()).toEqual(['from', 'mid-0', 'mid-1', 'to']);
    expect(boxHandleNames(), '연결선에는 8핸들도 서지 않는다(AC-63)').toEqual([]);
  });

  it('보이게 만든 뒤에도 그 몸짓은 **그대로 먹는다** — 알림이 길을 바꾸지 않았다', () => {
    setup([R1, R2, EL3, C_MID]);
    pick('c-mid');
    const el = handle('mid-0');
    down(el, AT.mid0);
    up(AT.mid0);
    down(el, AT.mid0);
    up(AT.mid0);
    expect(connector('c-mid').points).toEqual([{ x: 360, y: 120 }]);
  });
});
