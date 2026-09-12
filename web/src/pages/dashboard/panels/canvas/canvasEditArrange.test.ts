// 배치 정돈 순수 모듈 테스트 — 격자 붙임 · 정렬 · z-order (SPEC-CANVAS-002 T12 · T13 · T14).
//
// 덮는 인수 기준: AC-E5(상한에 붙잡히지 않는다 · 중심 기준 스냅), AC-08(정렬은 2개 이상),
// REQ-04(z-order 는 목록 편집기와 같은 연산), REQ-06(식별은 언제나 nodeId).
//
// 이 파일에서 가장 중요한 것은 **위험 R6** 이다: 정렬에 원 함수의 기본 상한(±40)이 새어
// 들어오면 캔버스 조작이 기준 상자의 40% 지점에서 조용히 멈춘다. 그래서 40% 를
// **넘어가는** 값을 일부러 만들어 그 값이 그대로 살아 나오는지 본다 — 붙잡히면 40% 에
// 해당하는 값이, 살아 나오면 원래 값이 나온다. 두 값이 다르므로 실패가 곧 진단이다.
//
// 붙임 쪽은 0.8.0 에서 그 위험을 **구조적으로** 없앴다. 좌표가 정수 캔버스 단위가 되면서
// 붙임이 백분율 공간을 아예 지나지 않게 되었고, 지나지 않는 함정에는 걸릴 수 없다.
// 그래서 이 파일은 그 사실 자체(§격자 붙임은 백분율 공간을 지나지 않는다)를 시험한다.

import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';

import { describe, expect, it } from 'vitest';

import { DEFAULT_CANVAS_SIZE, type BoxGeometry, type CanvasElement, type CanvasSize } from './canvasConfig';
import {
  CANVAS_GRID_STEP_CHOICES,
  CANVAS_GRID_STEP_MAX,
  CANVAS_GRID_STEP_MIN,
  CANVAS_GRID_STEP_UNITS,
  alignDeltas,
  bringToFront,
  clampGridStep,
  gridDividesCanvas,
  moveElementTo,
  removeNodes,
  sendToBack,
  snapDelta,
  type AlignTarget,
} from './canvasEditArrange';
import { stageLattice, type CanvasProjection, type StageSize } from './canvasGeometry';

// --- 고정 입력 -----------------------------------------------------------

/** 축이 서로 달라야 축을 뒤바꾼 계산이 드러난다. */
const STAGE: StageSize = { width: 200, height: 100 };

/** 기본 캔버스(500x400). 위 스테이지와 짝지으면 축척이 가로 0.4 · 세로 0.25 로 갈린다. */
const CANVAS: CanvasSize = { ...DEFAULT_CANVAS_SIZE };

/** 대표 투영 한 벌. */
const PROJ: CanvasProjection = { stage: STAGE, canvas: CANVAS };

/** 중심이 (100, 80) 캔버스 단위에 있는 상자 — 어느 간격으로도 눈금 위가 아니다. */
const ANCHOR = { x: 50, y: 40, w: 100, h: 80 };

function rect(id: string, geometry: BoxGeometry = { x: 0, y: 0, w: 50, h: 40 }): CanvasElement {
  return { id, kind: 'rect', geometry, style: {} };
}

function ids(elements: readonly CanvasElement[]): string[] {
  return elements.map((el) => el.id);
}

// --- 바이패스 금지 (위험 R6) ----------------------------------------------

describe('원 함수를 부르는 자리는 이 모듈 하나뿐이다 (위험 R6)', () => {
  it('panels/canvas 안에서 panelEditAlign 을 들이는 파일은 canvasEditArrange 뿐이다', () => {
    // 상한을 넘기는 자리가 둘이 되는 순간, 한쪽이 그것을 잊어도 화면에서만 드러난다 —
    // 오류가 아니라 "왜 더 안 가지" 로 보이므로 여기서 형상 자체를 막는다.
    const dir = __dirname;
    const offenders = readdirSync(dir)
      .filter((name) => /\.tsx?$/.test(name))
      .filter((name) => name !== 'canvasEditArrange.ts' && name !== 'canvasEditArrange.test.ts')
      .filter((name) => /from '.*panelEditAlign'/.test(readFileSync(join(dir, name), 'utf-8')));

    expect(offenders).toEqual([]);
  });

  it('원 모듈을 들이는 시험이 무동작이 아니다 — 정렬이 여전히 그 함수를 부른다', () => {
    // 붙임이 `snapOffsetToGrid` 를 걷어낸 뒤에도 위 가드가 **무언가를 지키고 있는지**를
    // 여기서 확인한다. 이 모듈이 원 모듈을 아예 들이지 않게 되면 위 시험은 언제나
    // 통과하는 빈 가드가 되므로, 그때는 가드를 지울 것이 아니라 이 사실이 먼저 깨져야 한다.
    const source = readFileSync(join(__dirname, 'canvasEditArrange.ts'), 'utf-8');
    expect(source).toMatch(/from '.*panelEditAlign'/);
    expect(source).toMatch(/computeAlignPatches/);
  });

  it('격자 붙임은 백분율 공간을 지나지 않는다 — ±40 함정이 닿을 자리가 없다', () => {
    // 0.8.0 이 붙임에서 `snapOffsetToGrid` 를 걷어낸 것이 위험 R6 의 절반을 **구조적으로**
    // 없앤 조치다. 그 함수를 다시 들이면 단위 → 백분율 → 단위 왕복이 생기고, 그 왕복이
    // 하는 일은 상한 인자를 다시 요구하는 것뿐이다.
    // 주석은 걷어내고 **코드만** 본다 — 머리말이 그 함수를 이름으로 설명하고 있으며,
    // 설명하는 것과 부르는 것은 다른 일이다.
    const code = readFileSync(join(__dirname, 'canvasEditArrange.ts'), 'utf-8')
      .split('\n')
      .filter((line) => !/^\s*(\/\/|\*|\/\*)/.test(line))
      .join('\n');
    expect(code).not.toMatch(/snapOffsetToGrid/);
    // `clampPercentOffset` 이 사는 모듈도 마찬가지다(위 panelGeometry 가드와 짝을 이룬다).
    expect(code).not.toMatch(/clampPercentOffset/);
  });

  it('격자 간격의 기본값은 고를 수 있는 목록 안에 있다 — 목록이 기본값을 밀어내지 않는다', () => {
    // 목록이 기본값을 품지 않으면 처음 켠 격자가 목록의 어느 칸과도 맞지 않아,
    // 고르개가 빈 값을 보여 준다(그리고 사용자는 자기가 고르지 않은 간격을 보게 된다).
    expect(CANVAS_GRID_STEP_CHOICES).toContain(CANVAS_GRID_STEP_UNITS);
  });

  it('기본 간격은 정수다 — 좌표계가 정수이므로 간격도 정수여야 눈금이 정수에 앉는다', () => {
    expect(Number.isInteger(CANVAS_GRID_STEP_UNITS)).toBe(true);
    for (const step of CANVAS_GRID_STEP_CHOICES) expect(Number.isInteger(step)).toBe(true);
  });

  it('panels/canvas 안에서 죄기 도우미(panelGeometry)를 직접 들이는 파일은 없다', () => {
    // `clampPercentOffset` · `PANEL_OFFSET_LIMIT` 이 사는 모듈이다. 캔버스의 기하는
    // **일부러 상한이 없으므로**(가정 A5 · AC-E5) 그 도우미를 직접 들이는 자리가 생기면
    // 그 순간 위험 R6 이 되살아난다 — 이 모듈이 무한대를 넘기는 것과 정면으로 어긋난다.
    const dir = __dirname;
    const offenders = readdirSync(dir)
      .filter((name) => /\.tsx?$/.test(name))
      .filter((name) => name !== 'canvasEditArrange.test.ts')
      .filter((name) => /from '.*panelGeometry'/.test(readFileSync(join(dir, name), 'utf-8')));

    expect(offenders).toEqual([]);
  });
});

// --- 격자 칸은 이 모듈이 내지 않는다 (0.9.0) ------------------------------
//
// 0.8.0 까지 여기 있던 두 절(§격자 칸 수는 정수다 · §칸 수는 스테이지 크기와 무관하다)은
// `gridPercents` 를 시험했다. 그 함수는 0.9.0 에서 사라졌다 — 백분율은 브라우저가 상자 폭에
// 곱하는 순간 소수 px 가 되고, 소수 자리에서 시작하는 1px 선은 두 픽셀에 나뉘어 칠해져
// 선마다 굵기가 달라 보였기 때문이다(사용 시험 "격자가 일정하지 않음" 세 번째 회차).
//
// **두 절이 지키던 성질은 사라지지 않고 자리를 옮겼다**: 칸 수가 정수인가 · 스테이지를
// 늘여도 그대로인가는 이제 `canvasGeometry.stageLattice` 의 성질이며 그쪽 시험이 지킨다
// (§그리는 영역을 격자 칸에 맞춘다). 여기서는 이 모듈에 남은 격자 어휘 — **정수 간격
// 하나** — 만 시험한다.

describe('이 모듈이 아는 격자 어휘는 정수 간격 하나뿐이다 (0.9.0)', () => {
  it('고를 수 있는 간격은 모두 캔버스 두 축을 나누어떨어지게 한다', () => {
    for (const step of CANVAS_GRID_STEP_CHOICES) {
      expect(CANVAS.width % step, `${step} 단위 가로`).toBe(0);
      expect(CANVAS.height % step, `${step} 단위 세로`).toBe(0);
    }
  });

  it('기본 간격 25 는 기본 캔버스에서 20 x 16 칸이다', () => {
    expect(CANVAS.width / CANVAS_GRID_STEP_UNITS).toBe(20);
    expect(CANVAS.height / CANVAS_GRID_STEP_UNITS).toBe(16);
  });

  it('백분율 환산을 더 이상 내보내지 않는다 — 그리는 쪽이 px 를 그대로 받는다', () => {
    // 이 모듈이 다시 백분율을 내기 시작하면 그리는 값과 붙는 값이 갈라질 자리가
    // 되살아난다. 이름이 아니라 **코드**를 본다 — 위 머리말이 그 함수를 설명하고 있다.
    const code = readFileSync(join(__dirname, 'canvasEditArrange.ts'), 'utf-8')
      .split('\n')
      .filter((line) => !/^\s*(\/\/|\*|\/\*)/.test(line))
      .join('\n');
    expect(code).not.toMatch(/gridPercents/);
  });
});

// --- 격자 붙임 (T12 · AC-E5) ----------------------------------------------

describe('격자 붙임은 요소의 **중심**을 격자에 맞춘다 (AC-E5)', () => {
  // ANCHOR 의 중심은 (100, 80). 기본 간격 25 의 눈금은 ... 75 · 100 · 125 ... 다.
  it('중심이 다음 눈금으로 끌려간 만큼만 이동량이 늘어난다', () => {
    // 중심 100 + 15 = 115 → 125 로 붙는다 → 이동량 25.
    expect(snapDelta({ dx: 15, dy: 0 }, ANCHOR).dx).toBe(25);
  });

  it('가까운 눈금으로 되돌아오기도 한다 (반올림이지 올림이 아니다)', () => {
    // 중심 100 + 10 = 110 → 100 으로 되돌아온다 → 이동량 0.
    expect(snapDelta({ dx: 10, dy: 0 }, ANCHOR).dx).toBe(0);
  });

  it('두 축이 같은 정수 간격으로 죄인다 (축마다 다른 눈금이 아니다)', () => {
    // 세로 중심 80 + 15 = 95 → 100 으로 붙는다 → 이동량 20.
    const snapped = snapDelta({ dx: 15, dy: 15 }, ANCHOR);
    expect(snapped.dx).toBe(25);
    expect(snapped.dy).toBe(20);
  });

  it('이미 눈금 위면 이동량이 그대로다', () => {
    // x: 100 + 25 = 125, y: 80 + 20 = 100 — 둘 다 25 의 배수다.
    expect(snapDelta({ dx: 25, dy: 20 }, ANCHOR)).toEqual({ dx: 25, dy: 20 });
  });

  it('**캔버스의 40% 를 넘어가도 붙잡히지 않는다** (위험 R6)', () => {
    // 옛 상한(±40%)이 새어 들어오면 200 단위(= 500 의 40%)에서 멈춘다.
    // 중심 100 + 300 = 400 은 이미 눈금이므로 이동량이 그대로 살아 나온다.
    expect(snapDelta({ dx: 300, dy: 0 }, ANCHOR).dx).toBe(300);
  });

  it('캔버스를 통째로 벗어나는 이동량도 그대로 살아 나온다', () => {
    // 중심 100 + 650 = 750 → 25 의 배수다. 세로 80 + 520 = 600 도 그렇다.
    expect(snapDelta({ dx: 650, dy: 520 }, ANCHOR)).toEqual({ dx: 650, dy: 520 });
  });

  it('음의 방향도 마찬가지다 (상한은 양쪽에 없다)', () => {
    // 중심 100 - 350 = -250 → 25 의 배수다.
    expect(snapDelta({ dx: -350, dy: 0 }, ANCHOR).dx).toBe(-350);
  });

  it('수가 아닌 이동량·기준 상자에는 손대지 않는다', () => {
    expect(snapDelta({ dx: Number.NaN, dy: 15 }, ANCHOR).dx).toBeNaN();
    const broken = { x: Number.NaN, y: 40, w: 100, h: 80 };
    expect(snapDelta({ dx: 15, dy: 0 }, broken).dx).toBe(15);
  });
});

describe('붙는 눈금은 넘긴 간격을 따른다 (화면에 그린 간격과 같은 값이다)', () => {
  // 중심 100 에서 15 를 끈 자리(115)는 간격마다 다른 눈금으로 붙는다 —
  // 10 → 120, 20 → 120, 25 → 125, 50 → 100. 넷이 갈리므로 간격이 실제로 쓰였는지 드러난다.
  it('10 단위 간격이면 가장 가까운 10 의 배수로 붙는다', () => {
    expect(snapDelta({ dx: 15, dy: 0 }, ANCHOR, 10).dx).toBe(20);
  });

  it('50 단위 간격이면 되돌아온다 (가장 가까운 50 의 배수는 뒤쪽이다)', () => {
    expect(snapDelta({ dx: 15, dy: 0 }, ANCHOR, 50).dx).toBe(0);
  });

  it('간격을 생략하면 기본값으로 붙는다 — 명시한 기본값과 결과가 같다', () => {
    const implicit = snapDelta({ dx: 15, dy: 17 }, ANCHOR);
    const explicit = snapDelta({ dx: 15, dy: 17 }, ANCHOR, CANVAS_GRID_STEP_UNITS);
    expect(implicit).toEqual(explicit);
  });

  it('**간격을 바꿔도 상한은 여전히 없다** — 위험 R6 은 간격마다 되살아날 수 있다', () => {
    for (const step of CANVAS_GRID_STEP_CHOICES) {
      expect(snapDelta({ dx: 300, dy: 0 }, ANCHOR, step).dx).toBeGreaterThan(200);
      expect(snapDelta({ dx: -350, dy: 0 }, ANCHOR, step).dx).toBeLessThan(-200);
    }
  });

  it('간격이 0 이하면 붙이지 않는다 (0 으로 죄면 요소가 얼어붙는다)', () => {
    expect(snapDelta({ dx: 15, dy: 17 }, ANCHOR, 0)).toEqual({ dx: 15, dy: 17 });
    expect(snapDelta({ dx: 15, dy: 17 }, ANCHOR, -5)).toEqual({ dx: 15, dy: 17 });
  });

  it('붙은 자리는 **그려진 선 위**다 — 그림과 붙임이 같은 정수를 본다', () => {
    // 사용자가 반 칸을 본 그 크기를 쓴다. 붙임은 캔버스 단위에서만 일어나지만 사용자가
    // 보는 것은 px 이므로, **투영한 뒤** 그려진 칸의 배수인지까지 본다 — 캔버스 단위에서만
    // 확인하면 화면에서 어긋나는 결함이 그대로 통과한다.
    const outer: StageSize = { width: 1749, height: 796 };
    for (const step of CANVAS_GRID_STEP_CHOICES) {
      const lattice = stageLattice(outer, CANVAS, step);
      const snapped = snapDelta({ dx: 17, dy: 23 }, ANCHOR, step);
      const centerX = ANCHOR.x + ANCHOR.w / 2 + snapped.dx;
      const centerY = ANCHOR.y + ANCHOR.h / 2 + snapped.dy;
      expect(centerX % step, `${step} 단위 가로`).toBe(0);
      expect(centerY % step, `${step} 단위 세로`).toBe(0);
      // 투영: 캔버스 단위 → px. 그려진 칸의 정수배 자리에 앉아야 선 위다.
      const pxX = (centerX / CANVAS.width) * lattice.stage.width;
      const pxY = (centerY / CANVAS.height) * lattice.stage.height;
      expect(pxX % lattice.cell.x).toBeCloseTo(0, 9);
      expect(pxY % lattice.cell.y).toBeCloseTo(0, 9);
    }
  });
});

// --- 정렬 (T13 · AC-08) ---------------------------------------------------

describe('정렬은 맞출 상대가 있어야 한다 (AC-08)', () => {
  const a: AlignTarget = { nodeId: 'a', box: { x: 10, y: 10, w: 40, h: 20 } };
  const b: AlignTarget = { nodeId: 'b', box: { x: 100, y: 50, w: 20, h: 10 } };

  it('0개면 빈 배열이다', () => {
    expect(alignDeltas([], PROJ, 'horizontal', 'start')).toEqual([]);
  });

  it('1개면 빈 배열이다 — 하나만 골라 놓고 맞출 곳은 없다', () => {
    expect(alignDeltas([a], PROJ, 'horizontal', 'start')).toEqual([]);
  });

  it('스테이지를 잴 수 없으면 빈 배열이다', () => {
    expect(
      alignDeltas([a, b], { stage: { width: 0, height: 0 }, canvas: CANVAS }, 'horizontal', 'start'),
    ).toEqual([]);
  });
});

describe('정렬 변환 사슬 — 투영 px 상자 → 백분율 → ÷100 × 캔버스 축 길이 (T13)', () => {
  const a: AlignTarget = { nodeId: 'a', box: { x: 10, y: 10, w: 40, h: 20 } };
  const b: AlignTarget = { nodeId: 'b', box: { x: 100, y: 50, w: 20, h: 10 } };

  /** 축 하나만 담긴 델타를 읽기 쉽게 편다. */
  function deltaOf(axis: 'horizontal' | 'vertical', mode: 'start' | 'center' | 'end') {
    const out = alignDeltas([a, b], PROJ, axis, mode);
    return Object.fromEntries(out.map((d) => [d.nodeId, d.delta]));
  }

  it('왼쪽 맞춤 — 바깥 상자의 왼쪽 변(10px)에 붙는다', () => {
    const d = deltaOf('horizontal', 'start');
    expect(d.a!.dx).toBeCloseTo(0, 9);
    // b 는 100px → 10px 로 90px 왼쪽 = 스테이지 폭의 -45% = 캔버스 폭 500 의 -225 단위.
    expect(d.b!.dx).toBeCloseTo(-225, 9);
  });

  it('왼쪽 맞춤의 -225 가 **상한 ±40% 에 붙잡히지 않는다** (위험 R6 의 정렬 쪽)', () => {
    // 기본 상한이 새어 들어오면 -40% = -200 단위가 나온다. 두 값이 다르므로 실패가 곧 진단이다.
    expect(deltaOf('horizontal', 'start').b!.dx).not.toBeCloseTo(-200, 3);
  });

  it('오른쪽 맞춤 — 바깥 상자의 오른쪽 변(120px)에 붙는다', () => {
    const d = deltaOf('horizontal', 'end');
    expect(d.a!.dx).toBeCloseTo(175, 9);
    expect(d.b!.dx).toBeCloseTo(0, 9);
  });

  it('가로 가운데 맞춤 — 바깥 상자의 가운데(65px)에 중심을 맞춘다', () => {
    const d = deltaOf('horizontal', 'center');
    expect(d.a!.dx).toBeCloseTo(87.5, 9);
    expect(d.b!.dx).toBeCloseTo(-112.5, 9);
  });

  it('가로 정렬은 세로를 건드리지 않는다', () => {
    for (const mode of ['start', 'center', 'end'] as const) {
      const d = deltaOf('horizontal', mode);
      expect(d.a!.dy).toBe(0);
      expect(d.b!.dy).toBe(0);
    }
  });

  it('위 맞춤 — 세로는 스테이지 높이(100px)로 나누고 캔버스 높이(400)를 곱한다', () => {
    const d = deltaOf('vertical', 'start');
    expect(d.a!.dy).toBeCloseTo(0, 9);
    expect(d.b!.dy).toBeCloseTo(-160, 9);
  });

  it('아래 맞춤 — 바깥 상자의 아래 변(60px)에 붙는다', () => {
    const d = deltaOf('vertical', 'end');
    expect(d.a!.dy).toBeCloseTo(120, 9);
    expect(d.b!.dy).toBeCloseTo(0, 9);
  });

  it('세로 가운데 맞춤 — 바깥 상자의 가운데(35px)에 중심을 맞춘다', () => {
    const d = deltaOf('vertical', 'center');
    expect(d.a!.dy).toBeCloseTo(60, 9);
    expect(d.b!.dy).toBeCloseTo(-80, 9);
  });

  it('세로 정렬은 가로를 건드리지 않는다', () => {
    for (const mode of ['start', 'center', 'end'] as const) {
      const d = deltaOf('vertical', mode);
      expect(d.a!.dx).toBe(0);
      expect(d.b!.dx).toBe(0);
    }
  });

  it('결과는 넘긴 순서대로 노드 id 를 달고 나온다 (배열 위치를 들고 다니지 않는다)', () => {
    expect(alignDeltas([a, b], PROJ, 'horizontal', 'start').map((d) => d.nodeId)).toEqual([
      'a',
      'b',
    ]);
  });
});

// --- z-order (T14 · REQ-04) -----------------------------------------------

describe('배열 순서 이동 — 목록 편집기와 같은 연산 하나를 지난다 (REQ-04)', () => {
  const three = [rect('a'), rect('b'), rect('c')];

  it('한 칸 앞으로 = 목표 위치를 idx+1 로 준 것이다', () => {
    expect(ids(moveElementTo(three, 'a', 1))).toEqual(['b', 'a', 'c']);
  });

  it('한 칸 뒤로 = 목표 위치를 idx-1 로 준 것이다', () => {
    expect(ids(moveElementTo(three, 'c', 1))).toEqual(['a', 'c', 'b']);
  });

  it('맨 앞(배열 끝)으로 보낸다', () => {
    expect(ids(moveElementTo(three, 'a', 2))).toEqual(['b', 'c', 'a']);
  });

  it('맨 뒤(배열 앞)로 보낸다', () => {
    expect(ids(moveElementTo(three, 'c', 0))).toEqual(['c', 'a', 'b']);
  });

  it('범위를 넘는 목표는 양 끝으로 죈다', () => {
    expect(ids(moveElementTo(three, 'a', 99))).toEqual(['b', 'c', 'a']);
    expect(ids(moveElementTo(three, 'c', -99))).toEqual(['c', 'a', 'b']);
  });

  it('제자리면 **받은 배열을 그대로** 돌려준다 (헛된 쓰기를 만들지 않는다)', () => {
    expect(moveElementTo(three, 'a', 0)).toBe(three);
  });

  it('없는 id 면 받은 배열을 그대로 돌려준다', () => {
    expect(moveElementTo(three, 'zzz', 0)).toBe(three);
  });

  it('원본을 건드리지 않는다', () => {
    moveElementTo(three, 'a', 2);
    expect(ids(three)).toEqual(['a', 'b', 'c']);
  });
});

describe('맨 앞·맨 뒤로 보내기는 무리의 상대 순서를 보존한다 (T14)', () => {
  const four = [rect('a'), rect('b'), rect('c'), rect('d')];

  it('하나를 맨 앞으로 보낸다', () => {
    expect(ids(bringToFront(four, new Set(['a'])))).toEqual(['b', 'c', 'd', 'a']);
  });

  it('여럿을 맨 앞으로 보내도 자기들끼리 뒤섞이지 않는다', () => {
    expect(ids(bringToFront(four, new Set(['a', 'b'])))).toEqual(['c', 'd', 'a', 'b']);
  });

  it('하나를 맨 뒤로 보낸다', () => {
    expect(ids(sendToBack(four, new Set(['d'])))).toEqual(['d', 'a', 'b', 'c']);
  });

  it('여럿을 맨 뒤로 보내도 순서가 뒤집히지 않는다', () => {
    expect(ids(sendToBack(four, new Set(['c', 'd'])))).toEqual(['c', 'd', 'a', 'b']);
  });

  it('이미 맨 앞·맨 뒤에 있으면 받은 배열을 그대로 돌려준다', () => {
    expect(bringToFront(four, new Set(['d']))).toBe(four);
    expect(sendToBack(four, new Set(['a']))).toBe(four);
  });

  it('빈 선택이면 아무 일도 없다', () => {
    expect(bringToFront(four, new Set())).toBe(four);
    expect(sendToBack(four, new Set())).toBe(four);
  });

  it('선택에 없는 id 가 섞여 있어도 있는 것만 옮긴다', () => {
    expect(ids(bringToFront(four, new Set(['a', 'zzz'])))).toEqual(['b', 'c', 'd', 'a']);
    expect(ids(sendToBack(four, new Set(['d', 'zzz'])))).toEqual(['d', 'a', 'b', 'c']);
  });
});

// --- 자유 간격의 난간과 그 대가 (0.11.0) ---------------------------------
//
// 목록 넷이 지키던 성질("두 축이 나머지 없이 떨어진다")은 **기본 캔버스에 한해서만** 참인
// 성질이었다. 자유 입력을 열면서 그 성질은 보장이 아니라 **고지의 대상**이 되었고, 이 절이
// 재는 것은 그 둘이다: 난간이 실제로 막는가(`clampGridStep`), 그리고 자투리를 정확히
// 판별하는가(`gridDividesCanvas`).

describe('격자 간격의 난간 — 자유 입력이 통과시키는 것과 막는 것', () => {
  it('범위 안의 정수는 그대로 지난다', () => {
    for (const step of [1, 7, 25, 137, 1000]) {
      expect(clampGridStep(step, CANVAS_GRID_STEP_UNITS)).toBe(step);
    }
  });

  it('범위 밖은 가까운 난간으로 죈다', () => {
    expect(clampGridStep(0, 25)).toBe(CANVAS_GRID_STEP_MIN);
    expect(clampGridStep(-40, 25)).toBe(CANVAS_GRID_STEP_MIN);
    expect(clampGridStep(CANVAS_GRID_STEP_MAX + 1, 25)).toBe(CANVAS_GRID_STEP_MAX);
    expect(clampGridStep(1e9, 25)).toBe(CANVAS_GRID_STEP_MAX);
  });

  it('소수는 정수로 내린다 — 좌표계가 정수이므로 간격도 정수여야 눈금이 정수에 앉는다', () => {
    expect(clampGridStep(12.9, 25)).toBe(12);
  });

  it('읽을 수 없는 입력에는 옛 값을 지킨다 (빈 칸 · 공백 · 글자 · NaN)', () => {
    // `Number('')` 이 0 이라는 사실이 이 시험의 존재 이유다 — 0 으로 떨어뜨리면 사용자가
    // 한 글자를 지우는 순간 격자가 1 단위로 무너진다.
    expect(clampGridStep('', 25)).toBe(25);
    expect(clampGridStep('   ', 25)).toBe(25);
    expect(clampGridStep('abc', 25)).toBe(25);
    expect(clampGridStep(Number.NaN, 25)).toBe(25);
    expect(clampGridStep(Number.POSITIVE_INFINITY, 25)).toBe(25);
  });

  it('문자열 숫자도 같은 규칙으로 읽는다 — 부르는 쪽이 손수 거를 자리를 남기지 않는다', () => {
    expect(clampGridStep('7', 25)).toBe(7);
    expect(clampGridStep('0', 25)).toBe(CANVAS_GRID_STEP_MIN);
  });

  it('난간은 하한이 1 · 상한이 1000 이다', () => {
    expect(CANVAS_GRID_STEP_MIN).toBe(1);
    expect(CANVAS_GRID_STEP_MAX).toBe(1000);
    expect(Number.isInteger(CANVAS_GRID_STEP_MIN)).toBe(true);
    expect(Number.isInteger(CANVAS_GRID_STEP_MAX)).toBe(true);
  });

  it('기본값과 제안값 넷은 모두 난간 안에 있다 — 난간이 제 기본값을 막으면 안 된다', () => {
    for (const step of [CANVAS_GRID_STEP_UNITS, ...CANVAS_GRID_STEP_CHOICES]) {
      expect(clampGridStep(step, CANVAS_GRID_STEP_UNITS)).toBe(step);
    }
  });
});

describe('자투리 판별 — 자유 입력이 새로 지는 고지의 근거', () => {
  it('두 축을 모두 나누면 참이다', () => {
    expect(gridDividesCanvas(25, CANVAS)).toBe(true);
    expect(gridDividesCanvas(10, CANVAS)).toBe(true);
  });

  it('한 축만 나누어도 거짓이다 — 자투리는 한 축에만 생겨도 눈에 보인다', () => {
    // 500 % 100 === 0 이지만 400 % 100 === 0 이므로 둘 다 나눈다. 250 은 가로만 나눈다.
    expect(CANVAS.width % 250).toBe(0);
    expect(CANVAS.height % 250).not.toBe(0);
    expect(gridDividesCanvas(250, CANVAS)).toBe(false);
  });

  it('어느 축도 나누지 못하면 거짓이다', () => {
    expect(gridDividesCanvas(7, CANVAS)).toBe(false);
  });

  it('캔버스를 바꾸면 제안값 넷도 자투리를 낼 수 있다 — 넷은 보장이 아니라 곁들이다', () => {
    // 640 % 25 === 15 — 흔한 화면비 하나만 잡아도 제안값이 자투리를 낸다.
    const odd: CanvasSize = { width: 640, height: 480 };
    expect(gridDividesCanvas(25, odd)).toBe(false);
    expect(gridDividesCanvas(10, odd)).toBe(true);
  });

  it('잴 수 없는 값에서는 거짓이다 — 알 수 없으면 고지하는 편이 안전하다', () => {
    expect(gridDividesCanvas(0, CANVAS)).toBe(false);
    expect(gridDividesCanvas(Number.NaN, CANVAS)).toBe(false);
    expect(gridDividesCanvas(25, { width: Number.NaN, height: 400 })).toBe(false);
  });
});

// --- 지우기 (SPEC-CANVAS-010) --------------------------------------------
//
// 목록 편집기의 휴지통과 캔버스의 Delete·Backspace 가 함께 지나는 그 한 함수다. 값으로
// 못박는 것 넷: 골라진 것만 빠지고 · 나머지 **순서가 보존되고** · 여럿을 한 번에 빼며 ·
// 뺄 것이 없으면 **같은 참조**를 돌려준다(호출부가 그것으로 헛된 쓰기를 피한다).

describe('고른 것들을 배열에서 뺀다', () => {
  const ITEMS = [{ id: 'a' }, { id: 'b' }, { id: 'c' }, { id: 'd' }] as const;

  it('고른 것만 빠지고 나머지 순서는 그대로다 — 배열 순서가 곧 z-order 다', () => {
    expect(removeNodes(ITEMS, new Set(['b']))).toEqual([{ id: 'a' }, { id: 'c' }, { id: 'd' }]);
  });

  it('여럿을 한 번에 뺀다 — 흩어진 자리도 함께 간다', () => {
    expect(removeNodes(ITEMS, new Set(['a', 'c']))).toEqual([{ id: 'b' }, { id: 'd' }]);
  });

  it('빈 집합이면 **같은 참조**다 — 쓸 일이 없다는 신호가 곧 그 참조다', () => {
    expect(removeNodes(ITEMS, new Set())).toBe(ITEMS);
  });

  it('배열에 없는 id 만 골라도 같은 참조다 — 선택에는 지워진 id 가 남을 수 있다', () => {
    // 실제로 오는 경우다: 목록 편집기에서 지우면 선택에 그 id 가 남는다.
    expect(removeNodes(ITEMS, new Set(['zzz']))).toBe(ITEMS);
  });

  it('있는 것과 없는 것이 섞여 있으면 **있는 것만** 빠진다', () => {
    expect(removeNodes(ITEMS, new Set(['c', 'zzz']))).toEqual([
      { id: 'a' },
      { id: 'b' },
      { id: 'd' },
    ]);
  });

  it('전부 고르면 빈 배열이다 — 남길 것이 없는 것도 유효한 결과다', () => {
    expect(removeNodes(ITEMS, new Set(['a', 'b', 'c', 'd']))).toEqual([]);
  });

  it('받은 배열을 제자리에서 고치지 않는다 — 호출부가 옛 배열을 들고 있을 수 있다', () => {
    const before = [...ITEMS];
    removeNodes(ITEMS, new Set(['b']));
    expect(ITEMS).toEqual(before);
  });
});
