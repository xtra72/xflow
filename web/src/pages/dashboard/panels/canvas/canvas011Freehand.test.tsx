// 자유선의 몸짓 — 궤적이 중간점이 된다 (SPEC-CANVAS-011 M11 · AC-74~AC-77).
//
// ## 이 파일이 겨누는 이음매
//
// M11 은 `connector/freehand.ts` 하나로 끝나지 않는다. 그 모듈이 옳아도 **오버레이가 궤적을
// 받지 않으면** 자유선은 M8 그대로 점 없는 선으로 남고, 그 사실은 모듈 시험 전량이 초록인
// 채로 배달된다. 004·009 가 "층을 건너는 시험" 으로 이름 적어 둔 자리이며, 이 파일이 재는
// 것은 그 사이 — **포인터 사건 → 궤적 → 간소화 → 저장 → 그리기 · 잡기** 다.
//
// 그래서 여기의 단언은 "점이 생겼다" 에서 멈추지 않는다. 생긴 점이 **손이 지난 자리를
// 닮았는가**(AC-74)를 기하로 재고, 저장 왕복을 지나도 그대로인지 보고, 그 선이 실제로
// 칠해지고 잡히는지 본다.
//
// ## 고정 입력의 산술 (M8 의 그 좌표 규율 그대로)
//
// 스테이지 200×100 · 캔버스 500×400 → 축척 가로 0.4 · 세로 0.25.
//
//   r1  캔버스 (100,100)-(300,300) → px 40..120 × 25..75      e 캔버스 (300,200) → px (120,50)
//   r2  캔버스 (400, 40)-(480,120) → px 160..192 × 10..30      w 캔버스 (400, 80) → px (160,20)
//
// 아래 궤적은 전부 px x 121..159 안에 있다 — **두 도형의 상자 어디에도 들지 않는 띠**라,
// 잉크 위의 한 점을 집었을 때 잡히는 것이 연결선임을 다른 이유로 참이 되게 하지 않는다.
//
// @spec SPEC-CANVAS-011 REQ-06 · AC-74 · AC-75 · AC-76 · AC-77

import { useState } from 'react';

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import { parseCanvasConfig, type CanvasElement, type CanvasSize, type PointGeometry } from './canvasConfig';
import CanvasEditOverlay from './CanvasEditOverlay';
import { CanvasEditSelectionContext, useCanvasEditSelectionState } from './canvasEditContext';
import { projectPoint, unprojectPoint, type CanvasProjection, type StageSize } from './canvasGeometry';
import { hitTest } from './canvasHitTest';
import { drawElements, type DrawContext2D } from './drawElement';
import { FREEHAND_TOLERANCE } from './connector/freehand';
import { MAX_CONNECTOR_POINTS, isConnector, type ConnectorElement } from './connector/connectorTypes';
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

const ALL: readonly CanvasNode[] = [R1, R2];

type Pt = { x: number; y: number };

const AT = {
  /** r1 의 `e` 앵커 — 캔버스 (300,200). */
  r1East: { x: 120, y: 50 },
  /** r2 의 `w` 앵커 — 캔버스 (400,80). */
  r2West: { x: 160, y: 20 },
  /** 어느 요소에도 어느 앵커에도 닿지 않는 자리 — 캔버스 (325,380). */
  empty: { x: 130, y: 95 },
} as const;

/** 손이 그은 궤적 하나(px). 두 상자 사이의 띠를 지나 r2 로 간다. */
const TRAJECTORY: readonly Pt[] = [
  { x: 126, y: 56 },
  { x: 132, y: 62 },
  { x: 138, y: 68 },
  { x: 144, y: 66 },
  { x: 150, y: 58 },
  { x: 156, y: 40 },
];

/** 거의 직선인 궤적 — 흔들림이 캔버스 단위로 허용 오차 아래다(0.5px = 2 단위). */
function straightTrajectory(count: number): Pt[] {
  return Array.from({ length: count }, (_u, i) => {
    const t = (i + 1) / (count + 1);
    return {
      x: 120 + 40 * t,
      y: 50 - 30 * t + (i % 2 === 0 ? 0 : 0.5),
    };
  });
}

/**
 * 아주 긴 궤적 — 상한(256)보다 한참 많은 **서로 다른** 정수 자리를 지난다.
 *
 * **간소화로는 줄어들지 않는 모양이어야 한다.** 완만한 궤적을 쓰면 허용 오차가 먼저 점을
 * 줄여 버려, 상한을 걷어내도 시험이 초록으로 남는다(실측: 완만한 600 점은 54 점으로
 * 접힌다). 그래서 사건마다 6px 씩 위아래로 꺾는다(순수 시험이 같은 성질을 캔버스 단위로 든다) — 캔버스 단위로 24 라 오차의 여덟 배이고,
 * 꺾임이 전부 살아남는다. 띠(px x 121..158 · y 26..47)를 벗어나지 않으므로 두 도형의
 * 상자에는 여전히 들지 않는다.
 */
function longTrajectory(count: number): Pt[] {
  return Array.from({ length: count }, (_u, i) => ({
    x: 121 + (i % 38),
    y: 26 + (i % 2) * 6 + Math.floor(i / 38),
  }));
}

// --- 하네스 ---------------------------------------------------------------

let live: readonly CanvasNode[] = [];

function Harness({ initial }: { initial: readonly CanvasNode[] }) {
  const [elements, setElements] = useState<readonly CanvasNode[]>(initial);
  live = elements;
  const state = useCanvasEditSelectionState();
  return (
    <CanvasEditSelectionContext value={state}>
      <span data-testid="selection">{[...state.selection].sort().join(',')}</span>
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

function setup(initial: readonly CanvasNode[] = ALL): void {
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

function at(p: Pt): MouseEventInit {
  return { clientX: p.x, clientY: p.y, bubbles: true, cancelable: true, button: 0 };
}

function down(p: Pt): void {
  fireEvent(overlayRoot(), new MouseEvent('pointerdown', at(p)));
}
function move(p: Pt): void {
  fireEvent(overlayRoot(), new MouseEvent('pointermove', at(p)));
}
function up(p: Pt): void {
  fireEvent(overlayRoot(), new MouseEvent('pointerup', at(p)));
}

/** 누르고 · 궤적을 그리고 · 놓는다 — 이 파일이 재는 그 한 몸짓. */
function drag(from: Pt, path: readonly Pt[], to: Pt): void {
  down(from);
  for (const p of path) move(p);
  move(to);
  up(to);
}

function enableFree(): void {
  fireEvent.click(screen.getByTestId('canvas-connector-tool-free'));
  expect(
    screen.getByTestId('canvas-connector-tool-free').getAttribute('aria-pressed'),
    'precondition: 자유선 도구가 켜졌다',
  ).toBe('true');
}

function connectors(): ConnectorElement[] {
  return live.filter((n): n is ConnectorElement => isConnector(n));
}

/** 방금 그은 하나. 없으면 시험이 그 자리에서 선다. */
function drawn(): ConnectorElement {
  const all = connectors();
  expect(all, 'precondition: 연결선이 하나 생겼다').toHaveLength(1);
  const one = all[0];
  if (one === undefined) throw new Error('unreachable');
  return one;
}

function selected(): string[] {
  const text = screen.getByTestId('selection').textContent ?? '';
  return text === '' ? [] : text.split(',');
}

// --- 기하를 재는 자 ---------------------------------------------------------

/** 포인터 자리(px)를 궤적이 저장된 그 공간(캔버스 단위 정수)으로 옮긴다. */
function sampled(p: Pt): PointGeometry {
  const c = unprojectPoint(p, PROJ);
  return { x: Math.round(c.x), y: Math.round(c.y) };
}

function strayFromSegment(p: PointGeometry, a: PointGeometry, b: PointGeometry): number {
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  const spanSq = dx * dx + dy * dy;
  if (spanSq === 0) return Math.hypot(p.x - a.x, p.y - a.y);
  const t = Math.min(1, Math.max(0, ((p.x - a.x) * dx + (p.y - a.y) * dy) / spanSq));
  return Math.hypot(p.x - (a.x + t * dx), p.y - (a.y + t * dy));
}

/** 궤적 전체가 **그려지는 선**에서 벗어난 최대 거리. 선은 두 끝을 포함한 해석 결과다. */
function worstStray(path: readonly Pt[], connector: ConnectorElement): number {
  const poly = resolveConnector(connector, live, PROJ, {});
  expect(poly, 'precondition: 연결이 끊기지 않았다').toBeDefined();
  const line = poly ?? [];
  return path.reduce((worst, raw) => {
    const p = sampled(raw);
    let best = Number.POSITIVE_INFINITY;
    for (let i = 1; i < line.length; i += 1) {
      const a = line[i - 1];
      const b = line[i];
      if (a === undefined || b === undefined) continue;
      best = Math.min(best, strayFromSegment(p, a, b));
    }
    return Math.max(worst, best);
  }, 0);
}

// --- 렌더 기록기 ------------------------------------------------------------

type Recorded = [string, ...unknown[]];

interface Recorder extends DrawContext2D {
  calls: Recorded[];
}

function makeRecorder(): Recorder {
  const calls: Recorded[] = [];
  const rec =
    (op: string) =>
    (...args: unknown[]): void => {
      calls.push([op, ...args]);
    };
  return {
    calls,
    save: rec('save'),
    restore: rec('restore'),
    setTransform: rec('setTransform'),
    beginPath: rec('beginPath'),
    rect: rec('rect'),
    ellipse: rec('ellipse'),
    moveTo: rec('moveTo'),
    lineTo: rec('lineTo'),
    closePath: rec('closePath'),
    bezierCurveTo: rec('bezierCurveTo'),
    stroke: rec('stroke'),
    fill: rec('fill'),
    fillText: rec('fillText'),
    clearRect: rec('clearRect'),
    fillRect: rec('fillRect'),
    measureText: (text: string) => ({ width: text.length * 10 }),
    fillStyle: '',
    strokeStyle: '',
    lineWidth: 1,
    globalAlpha: 1,
    font: '',
    textAlign: 'left',
    textBaseline: 'alphabetic',
  };
}

/** 몸짓이 방금 만든 그 배열을 렌더에 먹인다. */
function renderLive(): Recorder {
  const ctx = makeRecorder();
  drawElements(ctx, live, {}, {}, PROJ);
  return ctx;
}

function ops(ctx: Recorder): string[] {
  return ctx.calls.map((c) => c[0]);
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  live = [];
});

// --- AC-74: 궤적이 중간점이 된다 --------------------------------------------

describe('궤적이 **중간점**이 된다 (AC-74 · REQ-06)', () => {
  it('앵커에서 앵커로 끌면 중간점이 달린 자유선이 난다', () => {
    setup();
    enableFree();
    drag(AT.r1East, TRAJECTORY, AT.r2West);

    const line = drawn();
    expect(line.route).toBe('free');
    expect(line.from).toEqual({ el: 'r1', a: 'e' });
    expect(line.to).toEqual({ el: 'r2', a: 'w' });
    expect(line.points?.length).toBeGreaterThan(0);
  });

  it('**그은 자리를 닮았다** — 궤적의 어느 점도 선에서 허용 오차 밖에 있지 않다', () => {
    // 이 파일의 중심 단언이다. 개수만 재면 "중간점을 아무 데나 하나 심는다" 도 통과한다.
    setup();
    enableFree();
    drag(AT.r1East, TRAJECTORY, AT.r2West);
    expect(worstStray([...TRAJECTORY, AT.r2West], drawn())).toBeLessThanOrEqual(
      FREEHAND_TOLERANCE + 1e-9,
    );
  });

  it('저장된 점은 전부 **손이 실제로 지난 자리**다 — 지어낸 점이 없다', () => {
    setup();
    enableFree();
    drag(AT.r1East, TRAJECTORY, AT.r2West);
    const passed = [...TRAJECTORY, AT.r2West].map(sampled);
    for (const point of drawn().points ?? []) {
      expect(passed, JSON.stringify(point)).toContainEqual(point);
    }
  });

  it('차례가 뒤집히지 않는다 — 그은 순서 그대로다', () => {
    setup();
    enableFree();
    drag(AT.r1East, TRAJECTORY, AT.r2West);
    const order = [...TRAJECTORY, AT.r2West].map((p) => JSON.stringify(sampled(p)));
    const indices = (drawn().points ?? []).map((p) => order.indexOf(JSON.stringify(p)));
    expect(indices).toEqual([...indices].sort((a, b) => a - b));
  });

  it('나머지 셋은 **궤적을 받지 않는다** — M8 의 그 선 그대로다', () => {
    for (const id of ['straight', 'elbow', 'curve'] as const) {
      setup();
      fireEvent.click(screen.getByTestId(`canvas-connector-tool-${id}`));
      drag(AT.r1East, TRAJECTORY, AT.r2West);
      // `points` 키가 **서지도 않는다**(빈 배열도 아니다 — AC-25 와 같은 방향).
      expect(Object.hasOwn(drawn(), 'points'), id).toBe(false);
      cleanup();
      live = [];
    }
  });
});

// --- AC-75: 간소화 -----------------------------------------------------------

describe('거의 직선인 궤적은 **뚜렷이** 줄어든다 (AC-75)', () => {
  it('포인터 사건 40 개가 점 서넛 아래로 접힌다', () => {
    const path = straightTrajectory(40);
    setup();
    enableFree();
    drag(AT.r1East, path, AT.r2West);

    const points = drawn().points ?? [];
    expect(points.length).toBeLessThanOrEqual(4);
    expect(points.length * 8).toBeLessThan(path.length);
  });

  it('접혔어도 **모양은 지킨다** — 옳은 이유로 접혔다', () => {
    // 개수만 보는 단언은 "언제나 점을 버린다" 로도 통과한다. 그래서 같은 몸짓을 기하로
    // 한 번 더 잰다.
    const path = straightTrajectory(40);
    setup();
    enableFree();
    drag(AT.r1East, path, AT.r2West);
    expect(worstStray([...path, AT.r2West], drawn())).toBeLessThanOrEqual(
      FREEHAND_TOLERANCE + 1e-9,
    );
  });
});

// --- AC-76 · AC-77: 상한이 **배선되어 있다** ---------------------------------
//
// ## 여기서 재지 **않는** 것
//
// 상한 자체의 성질(상한에서 끊긴다 · 줄인 것이 오차가 아니라 상한이다 · 저장 왕복에서
// 잘리지 않는다)은 `connector/freehand.test.ts` 가 순수 산술로 든다. 같은 성질을 이 층에서
// 재면 포인터 사건 하나가 React 렌더 하나를 부르므로, 600 사건짜리 시험 셋이 전체 스위트
// 부하에서 5초 천장에 부딪힌다(실측 7418ms — 고립 실행에서는 통과해 **부하에서만** 드러
// 나는 부류다). 저장소에 그런 시험이 하나 더 생기면 "스위트가 빨갛다" 가 뜻을 잃는다.
//
// ## 그래서 여기 남는 것은 **배선**과 **AC-77** 뿐이다
//
// 배선: 오버레이가 제가 쌓은 궤적을 그 모듈에 넘기고, 그 결과가 실제로 저장된다.
// AC-77: 상한에 닿은 뒤로도 몸짓이 끊기지 않는다 — 이것은 본디 몸짓의 성질이라 순수
// 시험으로 내려보낼 수 없다. 둘을 **한 번의 끌기**로 함께 재고, 사건 수는 상한을 막 넘기는
// 최소(`MAX_CONNECTOR_POINTS + 4`)로 둔다. 사건 하나가 표본 하나이므로 상한을 넘기는 데
// 드는 사건 수는 이보다 줄일 수 없다.

describe('상한이 몸짓에 **배선되어 있다** (AC-76 · AC-77)', () => {
  /** 상한을 막 넘기는 최소 사건 수. 사건 하나가 표본 하나라 더 줄일 수 없다. */
  const OVER_CAP = longTrajectory(MAX_CONNECTOR_POINTS + 4);

  it('상한을 넘겨 끌어도 몸짓이 끝까지 이어지고 저장은 상한 아래다', () => {
    setup();
    enableFree();
    // 한 번의 끌기로 네 가지를 함께 잰다 — 같은 몸짓을 네 번 되풀이하면 그 값의 네 배가 든다.
    expect(() => drag(AT.r1East, OVER_CAP, AT.r2West)).not.toThrow();

    const line = drawn();
    // ① 상한이 이 층까지 이어져 있다(개수의 성질 자체는 순수 시험이 든다).
    expect(line.points?.length).toBeLessThanOrEqual(MAX_CONNECTOR_POINTS);
    // ② 궤적이 실제로 실렸다 — 배선이 끊겼다면 `points` 키가 서지도 않는다.
    expect(line.points?.length).toBeGreaterThan(0);
    // ③ AC-77 — 놓은 자리가 그대로 끝이 된다. 상한은 **받기**만 멈춘다.
    expect(line.to).toEqual({ el: 'r2', a: 'w' });
    // ④ 놓은 것이 곧 골라진다(AC-59) — 몸짓이 정상으로 끝났다는 관측 가능한 증거다.
    expect(selected()).toEqual([line.id]);
  });
});

// --- AC-58: 빈 자리에서 끝난다 ----------------------------------------------

describe('빈 자리에서 놓으면 **아무것도 남지 않는다** (016 이 AC-58 을 뒤집는다)', () => {
  // ## 뒤집은 조항이며 지우지 않는다
  //
  // 011 AC-58 은 "빈 자리에서 놓아도 궤적은 남는다" 였다. 016 은 저술 경로에서 자유 끝을
  // 없앴으므로 그 선 자체가 생기지 않는다 — **궤적이 사라진 것이 아니라 선이 생기지 않는
  // 것**이며, 그 차이를 아래 둘이 나눠 붙든다.

  it('빈 자리에서 놓으면 선이 생기지 않는다 (016 REQ-01)', () => {
    setup();
    enableFree();
    drag(AT.r1East, TRAJECTORY, AT.empty);
    expect(connectors()).toHaveLength(0);
  });

  it('**궤적을 닮는 성질은 그대로다** — 앵커에서 끝내면 011 과 같다', () => {
    // 011 이 이 자리에서 지키려던 것은 "손이 그은 자리를 선이 닮는다" 이고, 그 성질은
    // 끝의 종류와 무관하다. 016 이후 저술되는 끝이 앵커뿐이므로 앵커로 끝내어 잰다.
    setup();
    enableFree();
    drag(AT.r1East, TRAJECTORY, AT.r2West);
    const line = drawn();
    expect(line.from).toEqual({ el: 'r1', a: 'e' });
    expect(line.to).toEqual({ el: 'r2', a: 'w' });
    expect(line.points?.length).toBeGreaterThan(0);
    expect(worstStray([...TRAJECTORY, AT.r2West], line)).toBeLessThanOrEqual(
      FREEHAND_TOLERANCE + 1e-9,
    );
  });
});

// --- 움직임 없는 몸짓 --------------------------------------------------------

describe('움직임이 없으면 **점을 지어내지 않는다**', () => {
  it('같은 앵커에서 누르고 뗀 것은 그대로 무동작이다 — M8 의 그 판단이 자유선에도 선다', () => {
    setup();
    enableFree();
    down(AT.r1East);
    up(AT.r1East);
    expect(connectors()).toHaveLength(0);
  });

  it('이동 사건 없이 다른 앵커에서 떼면 **중간점 없는** 선이 난다', () => {
    // 점 없는 네 갈래는 같은 그림이다(AC-50). 빈 배열조차 남기지 않는다.
    setup();
    enableFree();
    down(AT.r1East);
    up(AT.r2West);
    expect(Object.hasOwn(drawn(), 'points')).toBe(false);
  });

  it('앞 몸짓의 궤적이 **다음 선에 묻어나지 않는다**', () => {
    setup();
    enableFree();
    drag(AT.r1East, TRAJECTORY, AT.r2West);
    // 두 번째는 움직임 없이 긋는다. 장부가 비워지지 않았다면 첫 몸짓의 손짓이 실린다.
    down(AT.r2West);
    up(AT.r1East);
    const second = connectors()[1];
    expect(second).toBeDefined();
    expect(Object.hasOwn(second ?? {}, 'points')).toBe(false);
  });
});

// --- 층을 건넌다 — 그리기 · 잡기 · 저장 왕복 ---------------------------------

describe('그은 자유선이 **그려지고 잡힌다**', () => {
  it('중간점 수만큼 구간이 그려진다 — 폴리라인이다(AC-50 의 그 그림)', () => {
    setup();
    enableFree();
    drag(AT.r1East, TRAJECTORY, AT.r2West);

    const ctx = renderLive();
    const line = drawn();
    const count = (line.points ?? []).length;
    // 두 도형은 채우기만 하므로 이 호출을 낼 수 있는 것은 연결선뿐이다.
    expect(ops(ctx)).toContain('stroke');
    expect(ctx.calls.filter((c) => c[0] === 'moveTo')).toHaveLength(1);
    expect(ctx.calls.filter((c) => c[0] === 'lineTo')).toHaveLength(count + 1);
    // 곡선으로 새지 않았다 — `free` 는 `elbow` 와 같은 길을 지난다(M6).
    expect(ops(ctx)).not.toContain('bezierCurveTo');
  });

  it('저장된 중간점 위에서 **그 선이 잡힌다** — 그린 자리와 잡히는 자리가 같다', () => {
    setup();
    enableFree();
    drag(AT.r1East, TRAJECTORY, AT.r2West);

    const line = drawn();
    const point = (line.points ?? [])[0];
    expect(point).toBeDefined();
    const px = projectPoint(point ?? { x: 0, y: 0 }, PROJ);
    expect(hitTest(live, px, PROJ, {})?.nodeId).toBe(line.id);
  });
});

describe('저장 왕복에 점이 **한 자리도** 달라지지 않는다', () => {
  it('좌표가 전부 정수다', () => {
    setup();
    enableFree();
    drag(AT.r1East, TRAJECTORY, AT.r2West);
    for (const point of drawn().points ?? []) {
      expect(Number.isInteger(point.x), JSON.stringify(point)).toBe(true);
      expect(Number.isInteger(point.y), JSON.stringify(point)).toBe(true);
    }
  });

  it('JSON 을 지나 돌아와도 같은 목록이다', () => {
    setup();
    enableFree();
    drag(AT.r1East, TRAJECTORY, AT.r2West);
    const before = drawn();

    const restored = parseCanvasConfig(
      JSON.parse(JSON.stringify({ canvas: CANVAS, elements: live })) as unknown,
    );
    const after = restored.elements.find((n): n is ConnectorElement => isConnector(n));
    expect(after?.points).toEqual(before.points);
    expect(after?.route).toBe('free');
  });
});
