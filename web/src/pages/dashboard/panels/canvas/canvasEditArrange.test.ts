// 배치 정돈 순수 모듈 테스트 — 격자 붙임 · 정렬 · z-order (SPEC-CANVAS-002 T12 · T13 · T14).
//
// 덮는 인수 기준: AC-E5(상한에 붙잡히지 않는다 · 중심 기준 스냅), AC-08(정렬은 2개 이상),
// REQ-04(z-order 는 목록 편집기와 같은 연산), REQ-06(식별은 언제나 nodeId).
//
// 이 파일에서 가장 중요한 것은 **위험 R6** 두 갈래다: 스냅과 정렬 어느 쪽이든 원 함수의
// 기본 상한(±40)이 새어 들어오면 캔버스 조작이 스테이지의 40% 지점에서 조용히 멈춘다.
// 그래서 40% 를 **넘어가는** 값을 일부러 만들어 그 값이 그대로 살아 나오는지 본다 —
// 붙잡히면 0.4 가, 살아 나오면 원래 값이 나온다. 두 값이 다르므로 실패가 곧 진단이다.

import { readFileSync, readdirSync } from 'node:fs';
import { join } from 'node:path';

import { describe, expect, it } from 'vitest';

import type { BoxGeometry, CanvasElement } from './canvasConfig';
import {
  CANVAS_GRID_STEP_PERCENT,
  alignDeltas,
  bringToFront,
  moveElementTo,
  sendToBack,
  snapDelta,
  type AlignTarget,
} from './canvasEditArrange';
import type { StageSize } from './canvasGeometry';

// --- 고정 입력 -----------------------------------------------------------

/** 축이 서로 달라야 축을 뒤바꾼 계산이 드러난다. 격자 한 칸은 x 20px · y 10px 이다. */
const STAGE: StageSize = { width: 200, height: 100 };

/** 중심이 (40, 20) px = 스테이지의 (20%, 20%) 에 있는 상자. */
const ANCHOR = { x: 20, y: 10, w: 40, h: 20 };

function rect(id: string, geometry: BoxGeometry = { x: 0, y: 0, w: 0.1, h: 0.1 }): CanvasElement {
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

  it('격자 간격은 panelEditAlign 의 값을 그대로 다시 내보낸다 (캔버스 전용 상수가 없다)', () => {
    expect(CANVAS_GRID_STEP_PERCENT).toBe(10);
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

// --- 격자 붙임 (T12 · AC-E5) ----------------------------------------------

describe('격자 붙임은 요소의 **중심**을 격자에 맞춘다 (AC-E5)', () => {
  it('중심이 다음 눈금으로 끌려간 만큼만 이동량이 늘어난다', () => {
    // 중심 20% + 이동 6.25% = 26.25% → 30% 로 붙는다 → 이동량은 10% = 0.1.
    expect(snapDelta({ dx: 0.0625, dy: 0 }, ANCHOR, STAGE).dx).toBeCloseTo(0.1, 9);
  });

  it('가까운 눈금으로 되돌아오기도 한다 (반올림이지 올림이 아니다)', () => {
    // 중심 20% + 12.5% = 32.5% → 30% 로 붙는다 → 이동량 10%.
    expect(snapDelta({ dx: 0.125, dy: 0 }, ANCHOR, STAGE).dx).toBeCloseTo(0.1, 9);
  });

  it('두 축이 저마다의 스테이지 길이로 죄인다 (축을 뒤바꾸지 않는다)', () => {
    // y 는 스테이지가 100px 이라 격자 한 칸이 10px 이다. 중심 20% + 7% = 27% → 30%.
    const snapped = snapDelta({ dx: 0, dy: 0.07 }, ANCHOR, STAGE);
    expect(snapped.dy).toBeCloseTo(0.1, 9);
    expect(snapped.dx).toBeCloseTo(0, 9);
  });

  it('이미 눈금 위면 이동량이 그대로다', () => {
    expect(snapDelta({ dx: 0.1, dy: 0.1 }, ANCHOR, STAGE)).toEqual({ dx: 0.1, dy: 0.1 });
  });

  it('**스테이지의 40% 를 넘어가도 붙잡히지 않는다** — 상한을 무한대로 넘긴다 (위험 R6)', () => {
    // 기본 상한(±40)이 새어 들어오면 0.4 가 나온다. 그것이 이 SPEC 이 이름 붙인 함정이다.
    expect(snapDelta({ dx: 0.6, dy: 0 }, ANCHOR, STAGE).dx).toBeCloseTo(0.6, 9);
  });

  it('스테이지를 통째로 벗어나는 이동량도 그대로 살아 나온다', () => {
    const snapped = snapDelta({ dx: 1.3, dy: 0.9 }, ANCHOR, STAGE);
    expect(snapped.dx).toBeCloseTo(1.3, 9);
    expect(snapped.dy).toBeCloseTo(0.9, 9);
  });

  it('음의 방향도 마찬가지다 (상한은 양쪽에 없다)', () => {
    expect(snapDelta({ dx: -0.7, dy: 0 }, ANCHOR, STAGE).dx).toBeCloseTo(-0.7, 9);
  });

  it('스테이지를 잴 수 없으면 이동량에 손대지 않는다 (0 으로 죄면 요소가 얼어붙는다)', () => {
    const zero: StageSize = { width: 0, height: 0 };
    expect(snapDelta({ dx: 0.33, dy: 0.44 }, ANCHOR, zero)).toEqual({ dx: 0.33, dy: 0.44 });
  });

  it('수가 아닌 이동량·기준 상자에는 손대지 않는다', () => {
    expect(snapDelta({ dx: Number.NaN, dy: 0.0625 }, ANCHOR, STAGE).dx).toBeNaN();
    const broken = { x: Number.NaN, y: 10, w: 40, h: 20 };
    expect(snapDelta({ dx: 0.0625, dy: 0 }, broken, STAGE).dx).toBeCloseTo(0.0625, 9);
  });
});

// --- 정렬 (T13 · AC-08) ---------------------------------------------------

describe('정렬은 맞출 상대가 있어야 한다 (AC-08)', () => {
  const a: AlignTarget = { nodeId: 'a', box: { x: 10, y: 10, w: 40, h: 20 } };
  const b: AlignTarget = { nodeId: 'b', box: { x: 100, y: 50, w: 20, h: 10 } };

  it('0개면 빈 배열이다', () => {
    expect(alignDeltas([], STAGE, 'horizontal', 'start')).toEqual([]);
  });

  it('1개면 빈 배열이다 — 하나만 골라 놓고 맞출 곳은 없다', () => {
    expect(alignDeltas([a], STAGE, 'horizontal', 'start')).toEqual([]);
  });

  it('스테이지를 잴 수 없으면 빈 배열이다', () => {
    expect(alignDeltas([a, b], { width: 0, height: 0 }, 'horizontal', 'start')).toEqual([]);
  });
});

describe('정렬 변환 사슬 — 투영 px 상자 → 백분율 → ÷100 → 정규화 델타 (T13)', () => {
  const a: AlignTarget = { nodeId: 'a', box: { x: 10, y: 10, w: 40, h: 20 } };
  const b: AlignTarget = { nodeId: 'b', box: { x: 100, y: 50, w: 20, h: 10 } };

  /** 축 하나만 담긴 델타를 읽기 쉽게 편다. */
  function deltaOf(axis: 'horizontal' | 'vertical', mode: 'start' | 'center' | 'end') {
    const out = alignDeltas([a, b], STAGE, axis, mode);
    return Object.fromEntries(out.map((d) => [d.nodeId, d.delta]));
  }

  it('왼쪽 맞춤 — 바깥 상자의 왼쪽 변(10px)에 붙는다', () => {
    const d = deltaOf('horizontal', 'start');
    expect(d.a!.dx).toBeCloseTo(0, 9);
    // b 는 100px → 10px 로 90px 왼쪽 = 스테이지 폭의 -45% = -0.45.
    expect(d.b!.dx).toBeCloseTo(-0.45, 9);
  });

  it('왼쪽 맞춤의 -0.45 가 **상한 ±40 에 붙잡히지 않는다** (위험 R6 의 정렬 쪽)', () => {
    // 기본 상한이 새어 들어오면 -0.4 가 나온다. 두 값이 다르므로 실패가 곧 진단이다.
    expect(deltaOf('horizontal', 'start').b!.dx).not.toBeCloseTo(-0.4, 3);
  });

  it('오른쪽 맞춤 — 바깥 상자의 오른쪽 변(120px)에 붙는다', () => {
    const d = deltaOf('horizontal', 'end');
    expect(d.a!.dx).toBeCloseTo(0.35, 9);
    expect(d.b!.dx).toBeCloseTo(0, 9);
  });

  it('가로 가운데 맞춤 — 바깥 상자의 가운데(65px)에 중심을 맞춘다', () => {
    const d = deltaOf('horizontal', 'center');
    expect(d.a!.dx).toBeCloseTo(0.175, 9);
    expect(d.b!.dx).toBeCloseTo(-0.225, 9);
  });

  it('가로 정렬은 세로를 건드리지 않는다', () => {
    for (const mode of ['start', 'center', 'end'] as const) {
      const d = deltaOf('horizontal', mode);
      expect(d.a!.dy).toBe(0);
      expect(d.b!.dy).toBe(0);
    }
  });

  it('위 맞춤 — 세로는 스테이지 높이(100px)로 나눈다 (축을 뒤바꾸지 않는다)', () => {
    const d = deltaOf('vertical', 'start');
    expect(d.a!.dy).toBeCloseTo(0, 9);
    expect(d.b!.dy).toBeCloseTo(-0.4, 9);
  });

  it('아래 맞춤 — 바깥 상자의 아래 변(60px)에 붙는다', () => {
    const d = deltaOf('vertical', 'end');
    expect(d.a!.dy).toBeCloseTo(0.3, 9);
    expect(d.b!.dy).toBeCloseTo(0, 9);
  });

  it('세로 가운데 맞춤 — 바깥 상자의 가운데(35px)에 중심을 맞춘다', () => {
    const d = deltaOf('vertical', 'center');
    expect(d.a!.dy).toBeCloseTo(0.15, 9);
    expect(d.b!.dy).toBeCloseTo(-0.2, 9);
  });

  it('세로 정렬은 가로를 건드리지 않는다', () => {
    for (const mode of ['start', 'center', 'end'] as const) {
      const d = deltaOf('vertical', mode);
      expect(d.a!.dx).toBe(0);
      expect(d.b!.dx).toBe(0);
    }
  });

  it('결과는 넘긴 순서대로 노드 id 를 달고 나온다 (배열 위치를 들고 다니지 않는다)', () => {
    expect(alignDeltas([a, b], STAGE, 'horizontal', 'start').map((d) => d.nodeId)).toEqual([
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
