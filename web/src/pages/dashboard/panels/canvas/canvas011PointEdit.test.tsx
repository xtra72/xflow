// 꺾임·곡률 편집의 **몸짓** (SPEC-CANVAS-011 M10 · AC-67 · AC-69 · AC-70 · AC-71 · AC-73).
//
// ## 이 파일이 겨누는 이음매
//
// 자리의 산술은 `connector/connectorEdit.test.ts` 가 전량 붙들고 있다. 여기서 재는 것은
// 그 산술이 **화면의 한 몸짓에 제대로 매여 있는가**이며, 그 이음매에서만 나는 실패가 셋이다.
//
//   ① **더블클릭의 뜻이 대상에 따라 갈린다.** 그룹 부품 위면 진입(009), 연결선 잉크 위면
//      점 생성(011). 둘 다 같은 한 판정(`isSecondPress`)을 지나므로, 갈래를 잘못 세우면
//      한쪽이 조용히 다른 쪽을 먹는다 — AC-73 이 그것을 금지한다.
//
//   ② **손잡이가 누름을 먹는다.** 중간점 손잡이는 진짜 단추라 루트의 갈래가 그 자리를 보지
//      못한다. 그래서 점을 빼는 길이 손잡이 쪽에도 서야 하고, 두 길이 **같은 장부**를 적지
//      않으면 "선 위에서는 되는데 점 위에서는 가끔 안 된다" 가 된다.
//
//   ③ **찍은 점이 선을 바꾸지 못한다.** 배열에는 들었는데 그려지는 모양이 그대로이면
//      사용자에게는 아무 일도 일어나지 않은 것이다. 그래서 이 파일은 층을 건너 재며 —
//      `resolveConnector` → `connectorPath` — 찍고 끌었을 때 **명령 목록이 실제로 갈리는지**
//      를 본다(AC-69 · AC-70).
//
// ## 고정 입력의 산술
//
// 스테이지 500×400 · 캔버스 500×400 → 축척 1. px 값과 캔버스 단위가 글자 그대로 같다.
//
//   r1   캔버스 (100,100)-(200,200)   e 앵커 = (200,150)
//   r2   캔버스 (300,100)-(400,200)   w 앵커 = (300,150)
//   grp-1 캔버스 (50,250)-(150,330)   body 부품 = px 60..100 × 258..282(중심 (80,270))
//
// 연결선의 잉크는 y=150 언저리를 지나고 그룹은 y≥250 에 있다 — 두 몸짓이 서로의 자리를
// 건드리지 않는다.
//
// @spec SPEC-CANVAS-011 REQ-05 · REQ-05-b · REQ-05-c · AC-67 · AC-68 · AC-69 · AC-70 ·
//       AC-71 · AC-73

import { useState } from 'react';

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import type { BoxGeometry, CanvasElement, CanvasSize } from './canvasConfig';
import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import type { CanvasProjection, StageSize } from './canvasGeometry';
import { connectorPath } from './connector/connectorPath';
import { CONNECTOR_KIND, isConnector, type ConnectorElement } from './connector/connectorTypes';
import { resolveConnector } from './connector/resolveConnector';
import type { CanvasNode, GroupElement } from './group/groupTypes';

// --- 고정 입력 -------------------------------------------------------------

const STAGE: StageSize = { width: 500, height: 400 };
const CANVAS: CanvasSize = { width: 500, height: 400 };
const PROJ: CanvasProjection = { stage: STAGE, canvas: CANVAS };

function rect(id: string, geometry: BoxGeometry): CanvasElement {
  return { id, kind: 'rect', geometry, style: { fill: '#111' } };
}

const R1 = rect('r1', { x: 100, y: 100, w: 100, h: 100 });
const R2 = rect('r2', { x: 300, y: 100, w: 100, h: 100 });

/** 부품 둘인 그룹 — AC-73 의 "그룹 부품 위" 가 이것이다(009 의 그 배치 그대로). */
const GROUP: GroupElement = {
  id: 'grp-1',
  kind: 'group',
  geometry: { x: 50, y: 250, w: 100, h: 80 },
  parts: [
    rect('body', { x: 1000, y: 1000, w: 4000, h: 4000 }),
    rect('head', { x: 5000, y: 5000, w: 4000, h: 4000 }),
  ],
};

const SEED_STYLE = { stroke: '#3b82f6', strokeWidth: 2 } as const;

function connector(over: Partial<ConnectorElement> & { id: string }): ConnectorElement {
  return {
    kind: CONNECTOR_KIND,
    from: { el: 'r1', a: 'e' },
    to: { el: 'r2', a: 'w' },
    route: 'elbow',
    style: { ...SEED_STYLE },
    ...over,
  };
}

/** 중간점이 없는 꺾은 선 — 잉크는 (200,150)-(300,150) 수평선이다. */
const C_PLAIN = connector({ id: 'c-plain' });

/** 중간점 둘 — 잉크가 (200,150)→(230,110)→(270,110)→(300,150) 이다. */
const C_MID = connector({
  id: 'c-mid',
  points: [
    { x: 230, y: 110 },
    { x: 270, y: 110 },
  ],
});

/** 곡선 — 중간점이 없는 동안은 직선과 같은 그림이다(AC-50). */
const C_CURVE = connector({ id: 'c-curve', route: 'curve' });

/** 직선 — 중간점을 들지 않는 갈래다(REQ-04). */
const C_STRAIGHT = connector({ id: 'c-straight', route: 'straight' });

/** 가리킨 요소가 배열에 없는 선 — 그려지지도 잡히지도 않는다(REQ-08). */
const C_BROKEN = connector({ id: 'c-broken', from: { el: 'ghost', a: 'e' } });

const AT = {
  /** `C_PLAIN`·`C_CURVE`·`C_STRAIGHT` 잉크의 가운데. */
  ink: { x: 250, y: 150 },
  /** `C_MID` 의 가운데 구간(첫 중간점과 둘째 중간점 사이) 위. */
  midSpan: { x: 250, y: 110 },
  /** `C_MID` 의 첫 중간점 자리. */
  mid0: { x: 230, y: 110 },
  /** 그룹 `body` 부품의 한가운데. */
  part: { x: 80, y: 270 },
  /** 어느 요소에도 닿지 않는 자리. */
  empty: { x: 460, y: 380 },
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
      {initial.filter(isConnector).map((c) => (
        <button
          key={c.id}
          type="button"
          data-testid={`pick-${c.id}`}
          onClick={() => state.setSelection(new Set([c.id]))}
        />
      ))}
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

/** 누르고 뗀다 — 놓지 않으면 끄는 중이라 다음 몸짓이 닿지 않는다(009 의 그 하네스). */
function click(target: HTMLElement, p: Pt): void {
  fireEvent(target, new MouseEvent('pointerdown', at(p)));
  fireEvent(overlayRoot(), new MouseEvent('pointerup', at(p)));
}

/** 같은 자리를 연달아 두 번 누른다 — 두 누름이 한 프레임 안이라 판정 창 안이다. */
function doubleClick(target: HTMLElement, p: Pt): void {
  click(target, p);
  click(target, p);
}

function drag(target: HTMLElement, from: Pt, to: Pt): void {
  fireEvent(target, new MouseEvent('pointerdown', at(from)));
  fireEvent(overlayRoot(), new MouseEvent('pointermove', at(to)));
  fireEvent(overlayRoot(), new MouseEvent('pointerup', at(to)));
}

function pick(id: string): void {
  fireEvent.click(screen.getByTestId(`pick-${id}`));
}

function handle(name: string): HTMLElement {
  return screen.getByTestId(`canvas-connector-handle-${name}`);
}

function handleNames(): string[] {
  return screen
    .queryAllByTestId(/^canvas-connector-handle-/)
    .map((el) => el.getAttribute('data-testid')!.replace('canvas-connector-handle-', ''));
}

function pointOf(el: HTMLElement): Pt {
  return { x: Number.parseFloat(el.style.left), y: Number.parseFloat(el.style.top) };
}

function conn(id: string): ConnectorElement {
  const found = live.find((n) => n.id === id);
  if (found === undefined || !isConnector(found)) throw new Error(`${id} 가 연결선이 아니다`);
  return found;
}

function midpoints(id: string): { x: number; y: number }[] {
  return [...(conn(id).points ?? [])];
}

/** 지금 그려지는 명령 목록 — 그리는 쪽·잡는 쪽이 보는 그 모양이다. */
function drawn(id: string): ReturnType<typeof connectorPath> {
  const c = conn(id);
  const points = resolveConnector(c, live, PROJ, {});
  return connectorPath(points ?? [], c.route, PROJ);
}

function selected(): string[] {
  const text = screen.getByTestId('selection').textContent ?? '';
  return text === '' ? [] : text.split(',');
}

function enableAnchorTool(): void {
  const button = screen.getByTestId('canvas-anchor-tool');
  fireEvent.click(button);
  expect(button.getAttribute('aria-pressed'), 'precondition: 앵커 도구가 켜졌다').toBe('true');
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  live = [];
});

// --- AC-67: 선 위 더블클릭이 점을 만든다 -------------------------------------

describe('선 위 더블클릭이 점을 만든다 (AC-67 · REQ-05)', () => {
  it('중간점이 하나 는다 — 그 자리는 눌린 잉크 위다', () => {
    setup([R1, R2, C_PLAIN]);
    pick('c-plain');
    expect(midpoints('c-plain'), 'precondition: 점이 없다').toEqual([]);

    doubleClick(overlayRoot(), AT.ink);
    expect(midpoints('c-plain')).toEqual([{ x: 250, y: 150 }]);
  });

  it('새 점에 손잡이가 선다 — 만든 것을 곧바로 끌 수 있다', () => {
    setup([R1, R2, C_PLAIN]);
    pick('c-plain');
    expect(handleNames()).toEqual(['from', 'to']);

    doubleClick(overlayRoot(), AT.ink);
    expect(handleNames()).toEqual(['from', 'mid-0', 'to']);
    expect(pointOf(handle('mid-0'))).toEqual(AT.ink);
  });

  it('한 번 누른 것은 점을 만들지 않는다 — 고르기 그대로다', () => {
    setup([R1, R2, C_PLAIN]);
    click(overlayRoot(), AT.ink);
    expect(selected()).toEqual(['c-plain']);
    expect(midpoints('c-plain')).toEqual([]);
  });

  it('세 번 눌러도 점은 하나다 — 먹은 몸짓이 연타 사슬을 끊는다', () => {
    // 끊지 않으면 셋째 누름이 둘째와 짝을 지어 방금 찍은 점을 도로 뺀다(홀짝으로 갈린다).
    setup([R1, R2, C_PLAIN]);
    pick('c-plain');
    click(overlayRoot(), AT.ink);
    click(overlayRoot(), AT.ink);
    click(overlayRoot(), AT.ink);
    expect(midpoints('c-plain')).toEqual([{ x: 250, y: 150 }]);
  });

  it('직선에는 점이 들지 않는다 — 두 끝을 잇는 직선 하나 그대로다 (REQ-04)', () => {
    setup([R1, R2, C_STRAIGHT]);
    pick('c-straight');
    doubleClick(overlayRoot(), AT.ink);
    expect(midpoints('c-straight')).toEqual([]);
    expect(drawn('c-straight').map((cmd) => cmd.c)).toEqual(['M', 'L']);
  });

  it('**고르지 않은** 선을 더블클릭하면 첫 누름이 고르고 둘째가 찍는다', () => {
    // REQ-05 의 "선택된 연결선" 은 이렇게 성립한다 — 두 누름 가운데 앞의 것이 고르기다.
    // 그래서 "고르지 않았으므로 아무 일도 없다" 는 갈래를 따로 두지 않는다: 그 갈래는
    // 결코 거짓이 될 수 없는 검사이고, 그런 검사는 읽는 사람에게 거짓말한다.
    setup([R1, R2, C_PLAIN]);
    expect(selected(), 'precondition: 아무것도 고르지 않았다').toEqual([]);

    doubleClick(overlayRoot(), AT.ink);
    expect(selected()).toEqual(['c-plain']);
    expect(midpoints('c-plain')).toEqual([{ x: 250, y: 150 }]);
  });

  it('끊긴 연결은 잡히지 않으므로 이 몸짓에 닿지 않는다 — 예외도 없다 (REQ-08)', () => {
    setup([R2, C_BROKEN]);
    pick('c-broken');
    expect(handleNames(), 'precondition: 해석이 부재라 손잡이도 없다').toEqual([]);

    // 그 선이 **이어졌다면** 지났을 자리를 두 번 누른다.
    expect(() => doubleClick(overlayRoot(), AT.ink)).not.toThrow();
    expect(conn('c-broken').points).toBeUndefined();
  });
});

// --- AC-68: 끼워 넣는 자리가 눌린 구간의 뒤다 --------------------------------

describe('끼워 넣는 자리가 **눌린 구간의 뒤**다 (AC-68 · REQ-05-a)', () => {
  it('가운데 구간을 누르면 두 점 **사이**에 든다 — 끝에 붙지 않는다', () => {
    setup([R1, R2, C_MID]);
    pick('c-mid');
    doubleClick(overlayRoot(), AT.midSpan);
    expect(midpoints('c-mid')).toEqual([
      { x: 230, y: 110 },
      { x: 250, y: 110 },
      { x: 270, y: 110 },
    ]);
  });

  it('마지막 구간을 누르면 맨 뒤에 든다', () => {
    setup([R1, R2, C_MID]);
    pick('c-mid');
    // (270,110) 에서 (300,150) 으로 내려가는 마지막 구간 위의 한 점.
    doubleClick(overlayRoot(), { x: 285, y: 130 });
    expect(midpoints('c-mid').map((p) => p.x)).toEqual([230, 270, 285]);
  });

  it('첫 구간을 누르면 맨 앞에 든다', () => {
    setup([R1, R2, C_MID]);
    pick('c-mid');
    doubleClick(overlayRoot(), { x: 215, y: 130 });
    expect(midpoints('c-mid').map((p) => p.x)).toEqual([215, 230, 270]);
  });
});

// --- AC-69 · AC-70: 만든 점이 선을 바꾼다 ------------------------------------

describe('만든 점을 끌면 선이 그 자리에서 **꺾인다** (AC-69)', () => {
  it('꺾은 선의 명령 목록이 그 점을 지난다', () => {
    setup([R1, R2, C_PLAIN]);
    pick('c-plain');
    expect(drawn('c-plain').map((cmd) => cmd.c), 'precondition: 곧은 선이다').toEqual(['M', 'L']);

    doubleClick(overlayRoot(), AT.ink);
    drag(handle('mid-0'), AT.ink, { x: 250, y: 60 });

    expect(midpoints('c-plain')).toEqual([{ x: 250, y: 60 }]);
    expect(drawn('c-plain')).toEqual([
      { c: 'M', x: 200, y: 150 },
      { c: 'L', x: 250, y: 60 },
      { c: 'L', x: 300, y: 150 },
    ]);
  });

  it('끄는 동안 다른 점은 움직이지 않는다 (AC-65 의 그 성질)', () => {
    setup([R1, R2, C_MID]);
    pick('c-mid');
    doubleClick(overlayRoot(), AT.midSpan);
    drag(handle('mid-1'), AT.midSpan, { x: 250, y: 40 });

    expect(midpoints('c-mid')).toEqual([
      { x: 230, y: 110 },
      { x: 250, y: 40 },
      { x: 270, y: 110 },
    ]);
  });
});

describe('곡선은 만든 점을 **제어점**으로 삼아 휜다 (AC-70 · REQ-04)', () => {
  it('A-P-B 에서 P 를 옮기면 A→B 3차의 제어점이 P 다', () => {
    setup([R1, R2, C_CURVE]);
    pick('c-curve');
    expect(drawn('c-curve').map((cmd) => cmd.c), 'precondition: 점이 없으면 직선이다').toEqual([
      'M',
      'L',
    ]);

    doubleClick(overlayRoot(), AT.ink);
    drag(handle('mid-0'), AT.ink, { x: 250, y: 50 });

    const a = { x: 200, y: 150 };
    const b = { x: 300, y: 150 };
    const p = { x: 250, y: 50 };
    expect(midpoints('c-curve')).toEqual([p]);
    // 2차(A · P · B)를 3차로 **정확히** 옮긴 그 값이다 — `c = end + ⅔(P − end)`.
    expect(drawn('c-curve')).toEqual([
      { c: 'M', x: a.x, y: a.y },
      {
        c: 'C',
        x1: a.x + (2 / 3) * (p.x - a.x),
        y1: a.y + (2 / 3) * (p.y - a.y),
        x2: b.x + (2 / 3) * (p.x - b.x),
        y2: b.y + (2 / 3) * (p.y - b.y),
        x: b.x,
        y: b.y,
      },
    ]);
  });
});

// --- AC-71: 중간점 더블클릭이 그 점을 뺀다 -----------------------------------

describe('중간점 더블클릭이 그 점을 뺀다 (AC-71 · REQ-05-b)', () => {
  it('손잡이 위를 두 번 누르면 그 점이 빠진다 — 단추가 누름을 먹어도 닿는다', () => {
    setup([R1, R2, C_MID]);
    pick('c-mid');
    doubleClick(handle('mid-0'), AT.mid0);
    expect(midpoints('c-mid')).toEqual([{ x: 270, y: 110 }]);
    expect(handleNames()).toEqual(['from', 'mid-0', 'to']);
  });

  it('손잡이를 빗맞아 잉크를 눌러도 빠진다 — 오차가 앵커의 그것과 같다', () => {
    // 집는 오차(9px)는 연타 오차(5px)보다 넓다. 그래서 두 누름 가운데 하나가 단추 밖으로
    // 떨어질 수 있고, 그때도 뜻이 같아야 한다.
    setup([R1, R2, C_MID]);
    pick('c-mid');
    doubleClick(overlayRoot(), { x: 234, y: 112 });
    expect(midpoints('c-mid')).toEqual([{ x: 270, y: 110 }]);
  });

  it('한 번 누른 것은 빼지 않는다 — 끄는 몸짓 그대로다', () => {
    setup([R1, R2, C_MID]);
    pick('c-mid');
    click(handle('mid-0'), AT.mid0);
    expect(midpoints('c-mid')).toHaveLength(2);
  });

  it('**끝점 손잡이**의 더블클릭은 점을 하나도 빼지 않는다', () => {
    // 끝은 빼는 것이 아니라 갈아 끼우는 것이다(REQ-07-a). 그 갈아 끼움이 손을 뗄 때
    // 무엇을 하는지는 M9 의 몫이므로(`repointConnector`) 여기서 다시 재지 않는다 — 이
    // 시험이 붙드는 것은 **M10 이 끝점에 손대지 않는다**는 사실 하나다.
    setup([R1, R2, C_MID]);
    pick('c-mid');
    doubleClick(handle('from'), { x: 200, y: 150 });
    expect(midpoints('c-mid')).toEqual([
      { x: 230, y: 110 },
      { x: 270, y: 110 },
    ]);
    expect(handleNames()).toEqual(['from', 'mid-0', 'mid-1', 'to']);
  });

  it('점을 전부 빼면 곡선이 **직선으로 돌아온다** (AC-50)', () => {
    setup([R1, R2, connector({ id: 'c-bent', route: 'curve', points: [{ x: 250, y: 50 }] })]);
    pick('c-bent');
    expect(drawn('c-bent').map((cmd) => cmd.c), 'precondition: 휘어 있다').toEqual(['M', 'C']);

    doubleClick(handle('mid-0'), { x: 250, y: 50 });
    expect(conn('c-bent').points).toBeUndefined();
    expect(drawn('c-bent').map((cmd) => cmd.c)).toEqual(['M', 'L']);
  });

  it('앵커 도구가 켜진 동안에는 손잡이의 더블클릭이 점을 빼지 않는다', () => {
    // 그 도구의 더블클릭은 **언제나 앵커**다(M3'b). 화면에서는 손잡이가 포인터를 먹지
    // 않으므로 이 자리에 닿지도 않지만, 닿았을 때 두 갈래가 서로 다른 답을 내면
    // "빼기는 되는데 만들기는 안 된다" 가 된다 — 답은 표 하나에서 나온다.
    setup([R1, R2, C_MID]);
    pick('c-mid');
    enableAnchorTool();
    doubleClick(handle('mid-0'), AT.mid0);
    expect(midpoints('c-mid')).toHaveLength(2);
  });
});

// --- AC-73: 그룹 진입과 섞이지 않는다 ----------------------------------------

describe('그룹 부품 위면 진입, 연결선 잉크 위면 점 생성 (AC-73 · REQ-05-c)', () => {
  it('그룹 부품 위의 더블클릭은 **그룹 진입**이다 — 009 그대로다', () => {
    setup([R1, R2, GROUP, C_PLAIN]);
    doubleClick(overlayRoot(), AT.part);
    expect(selected()).toEqual(['grp-1/body']);
    expect(midpoints('c-plain'), '그 몸짓은 선을 건드리지 않는다').toEqual([]);
  });

  it('연결선 잉크 위의 더블클릭은 **점 생성**이다 — 같은 장면, 다른 대상', () => {
    setup([R1, R2, GROUP, C_PLAIN]);
    doubleClick(overlayRoot(), AT.ink);
    expect(midpoints('c-plain')).toEqual([{ x: 250, y: 150 }]);
    expect(selected(), '고른 것은 그 선이다 — 그룹으로 들어가지 않았다').toEqual(['c-plain']);
  });

  it('두 몸짓을 잇달아 해도 서로를 먹지 않는다', () => {
    setup([R1, R2, GROUP, C_PLAIN]);
    doubleClick(overlayRoot(), AT.part);
    doubleClick(overlayRoot(), AT.ink);
    expect(selected()).toEqual(['c-plain']);
    expect(midpoints('c-plain')).toEqual([{ x: 250, y: 150 }]);

    doubleClick(overlayRoot(), AT.part);
    expect(selected()).toEqual(['grp-1/body']);
    expect(midpoints('c-plain'), '들어가는 몸짓이 점을 늘리지 않는다').toHaveLength(1);
  });

  it('빈 자리를 거쳐 누른 것은 어느 쪽도 아니다 — 연타 사슬이 끊긴다', () => {
    setup([R1, R2, GROUP, C_PLAIN]);
    click(overlayRoot(), AT.ink);
    click(overlayRoot(), AT.empty);
    click(overlayRoot(), AT.ink);
    expect(midpoints('c-plain')).toEqual([]);
  });
});
