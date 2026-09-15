// 한 누름이 뜻하는 것 — 더하기 · 빼기 · 거절 (SPEC-CANVAS-011 M3'b · AC-17 · AC-23 · AC-24).
//
// ## 여기서 재는 것과 재지 않는 것
//
// **더블클릭 판정은 여기 없다.** 그것은 오버레이의 `isSecondPress` 하나가 소유하고
// (AC-28), 이 모듈은 그 판정이 "두 번째 누름" 이라고 이미 말한 **뒤에** 불린다. 그래서
// 이 파일에는 시각도 연타도 나오지 않는다 — 나오면 판정이 둘이 된 것이다.
//
// 재는 것은 **자리의 산술** 하나다: 이 px 지점이 이미 있는 임의 앵커 위인가, 아닌가,
// 아니면 애초에 놓을 수 없는 요소인가.
//
// ## 투영을 1000×800 로 잡은 까닭
//
// `customAnchors.test.ts` 와 같다 — 캔버스 한 단위가 화면 2 px 이라, 오차를 px 로 말할 때
// 그 수가 캔버스 단위로 얼마인지 곧바로 환산된다.
//
// @spec SPEC-CANVAS-011 REQ-02' · REQ-02'-c · REQ-02'-d

import { describe, expect, it } from 'vitest';

import type { CanvasElement, RectElement } from '../canvasConfig';
import { projectPoint, type CanvasProjection } from '../canvasGeometry';
import {
  FIXED_ANCHOR_IDS,
  addAnchorAt,
  anchorGestureAt,
  anchorPoints,
  nextAnchorId,
} from './anchors';
import type { CustomAnchor } from './anchorTypes';

/** 캔버스 한 단위 = 화면 2 px. */
const PROJ: CanvasProjection = {
  stage: { width: 1000, height: 800 },
  canvas: { width: 500, height: 400 },
};

const NO_WIDTHS: Readonly<Record<string, number>> = {};

/** 상자 (100,100)-(300,300) → 화면 px (200,200)-(600,600). */
const BOX: RectElement = {
  id: 'r',
  kind: 'rect',
  geometry: { x: 100, y: 100, w: 200, h: 200 },
  style: {},
};

const LINE: CanvasElement = {
  id: 'l',
  kind: 'line',
  geometry: { x1: 100, y1: 100, x2: 300, y2: 300 },
  style: {},
};

const TEXT: CanvasElement = {
  id: 't',
  kind: 'text',
  geometry: { x: 100, y: 100 },
  style: {},
  text: 'abc',
};

/** 오버레이가 쓰는 그 값(`ANCHOR_DOT_PX / 2 + DOUBLE_PRESS_SLOP_PX`). */
const SLOP = 9;

/** 앵커 자리를 화면 px 로. 그리는 쪽이 쓰는 그 지도에서 나온다. */
function dotAt(node: Parameters<typeof anchorPoints>[0], id: string): { x: number; y: number } {
  const point = anchorPoints(node, PROJ, NO_WIDTHS).get(id);
  expect(point, `precondition: ${id} 자리가 있다`).toBeDefined();
  return projectPoint(point!, PROJ);
}

function gesture(node: Parameters<typeof anchorPoints>[0], at: { x: number; y: number }) {
  return anchorGestureAt(node, anchorPoints(node, PROJ, NO_WIDTHS), at, PROJ, SLOP);
}

// --- 거절: 상자형이 아니다 (REQ-02'-d · AC-24) -----------------------------

describe('선과 텍스트는 앵커를 받지 않는다 (AC-24)', () => {
  it.each([
    ['line', LINE],
    ['text', TEXT],
  ])('%s 위의 누름은 거절이다 — 조용한 무효가 아니다', (name, node) => {
    const out = gesture(node, { x: 400, y: 400 });
    expect(out.kind, name).toBe('refuse');
    expect(out).toEqual({ kind: 'refuse', reason: 'notBoxed' });
  });

  it('거절은 예외가 아니다 — 어디를 눌러도 던지지 않는다', () => {
    for (const at of [
      { x: 0, y: 0 },
      { x: 200, y: 200 },
      { x: 1e9, y: -1e9 },
      { x: Number.NaN, y: 0 },
    ]) {
      expect(() => gesture(LINE, at)).not.toThrow();
    }
  });
});

// --- 더하기 (REQ-02' · AC-17) ----------------------------------------------

describe('빈 자리의 누름은 **더하기**다 (AC-17)', () => {
  it('앵커가 하나도 없으면 언제나 더하기다', () => {
    expect(gesture(BOX, { x: 400, y: 400 }).kind).toBe('add');
  });

  it('돌려주는 자리는 그 px 지점을 **역투영**한 캔버스 값이다', () => {
    // 상자 한가운데 px (400,400) → 캔버스 (200,200). 지도가 아니라 누른 자리에서 나온다 —
    // 앵커는 사용자가 **고른 자리**이지 아홉 중 하나로 접히는 값이 아니다(A10).
    expect(gesture(BOX, { x: 400, y: 400 })).toEqual({ kind: 'add', at: { x: 200, y: 200 } });
  });

  it('고정 아홉 **위**를 눌러도 더하기다 — 아홉은 빠지지 않는다(A1)', () => {
    // 고정은 파생이라 뺄 것이 없다. 여기서 빼기가 나오면 A1 이 깨진 것이다.
    for (const id of FIXED_ANCHOR_IDS) {
      expect(gesture(BOX, dotAt(BOX, id)).kind, id).toBe('add');
    }
  });

  it('상자형 넷이 모두 더하기를 낸다 — 그룹도 포함이다', () => {
    const group = {
      id: 'g',
      kind: 'group' as const,
      geometry: { x: 100, y: 100, w: 200, h: 200 },
      parts: [],
    };
    expect(gesture(group, { x: 400, y: 400 }).kind).toBe('add');
  });
});

// --- 빼기 (REQ-02'-c · AC-23) ----------------------------------------------

describe('임의 앵커 위의 누름은 **빼기**다 (AC-23)', () => {
  /** 한가운데에 앵커 하나를 둔 상자. 그 점은 화면 px (400,400) 이다. */
  const WITH_ONE = addAnchorAt(BOX, { x: 200, y: 200 }, 'a1');

  it('점 한가운데를 누르면 그 id 가 나온다', () => {
    expect(dotAt(WITH_ONE, 'a1')).toEqual({ x: 400, y: 400 });
    expect(gesture(WITH_ONE, { x: 400, y: 400 })).toEqual({ kind: 'remove', id: 'a1' });
  });

  it('오차 **안**이면 빼기다 — 보이는 점보다 좁게 집히지 않는다', () => {
    // 4px 은 그려지는 점의 반지름이다. 여기서 더하기가 나오면 점 위를 눌렀는데 옆에
    // 새 앵커가 서는 화면이 된다.
    for (const d of [0, 1, 4, 8, 9]) {
      expect(gesture(WITH_ONE, { x: 400 + d, y: 400 }).kind, `+${d}px`).toBe('remove');
    }
  });

  it('오차 **밖**이면 더하기다 — 곁에 새 앵커를 놓을 길이 남는다', () => {
    for (const d of [9.5, 12, 40]) {
      expect(gesture(WITH_ONE, { x: 400 + d, y: 400 }).kind, `+${d}px`).toBe('add');
    }
  });

  it('오차는 **원**이다 — 대각선도 같은 자로 잰다', () => {
    // 가로·세로 축척이 달라도 견주기가 px 에서 일어나므로 원이 유지된다.
    const d = SLOP / Math.SQRT2; // 대각선 거리가 정확히 SLOP
    expect(gesture(WITH_ONE, { x: 400 + d, y: 400 + d }).kind).toBe('remove');
    expect(gesture(WITH_ONE, { x: 400 + d + 1, y: 400 + d + 1 }).kind).toBe('add');
  });

  it('둘이 가까이 있으면 **더 가까운 쪽**이 빠진다', () => {
    const two = addAnchorAt(WITH_ONE, { x: 205, y: 200 }, 'a2'); // px (410,400)
    expect(gesture(two, { x: 402, y: 400 })).toEqual({ kind: 'remove', id: 'a1' });
    expect(gesture(two, { x: 408, y: 400 })).toEqual({ kind: 'remove', id: 'a2' });
  });

  it('고정 이름에 **가려진** 임의 앵커는 고르지 않는다', () => {
    // `anchorPoints` 는 이름이 부딪히면 고정을 남긴다 — 그 임의 앵커는 그려지지 않으므로
    // 여기서 빼면 눈에 보이는 점은 그대로인데 config 만 달라진다.
    const shadowed = { ...BOX, anchors: [{ id: 'c', x: 5000, y: 5000 }] as CustomAnchor[] };
    expect(gesture(shadowed, dotAt(shadowed, 'c')).kind).toBe('add');
  });
});

// --- id 짓기 ---------------------------------------------------------------

describe('새 앵커의 이름', () => {
  it('첫 앵커는 `a1` 이다', () => {
    expect(nextAnchorId(BOX)).toBe('a1');
  });

  it('쓰던 번호를 건너뛴다', () => {
    const two = addAnchorAt(addAnchorAt(BOX, { x: 110, y: 110 }, 'a1'), { x: 120, y: 120 }, 'a2');
    expect(nextAnchorId(two)).toBe('a3');
  });

  it('가운데가 비면 **그 자리를 메운다** — 번호가 끝없이 자라지 않는다', () => {
    const only2 = { ...BOX, anchors: [{ id: 'a2', x: 0, y: 0 }] as CustomAnchor[] };
    expect(nextAnchorId(only2)).toBe('a1');
  });

  it('고정 아홉의 이름과 부딪히지 않는다 — 열 번을 이어 지어도', () => {
    let node: Parameters<typeof nextAnchorId>[0] = BOX;
    const made: string[] = [];
    for (let i = 0; i < 10; i += 1) {
      const id = nextAnchorId(node);
      made.push(id);
      node = addAnchorAt(node, { x: 100 + i, y: 100 + i }, id);
    }
    expect(made).toEqual(['a1', 'a2', 'a3', 'a4', 'a5', 'a6', 'a7', 'a8', 'a9', 'a10']);
    for (const id of made) expect(FIXED_ANCHOR_IDS).not.toContain(id);
  });
});
