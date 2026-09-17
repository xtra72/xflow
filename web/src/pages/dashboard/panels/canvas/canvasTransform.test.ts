// 뒤집기·회전 산술의 불변식 (SPEC-CANVAS-013 M1 · M2 · AC-01~AC-12d).
//
// **이 파일이 지키는 것은 좌표가 아니라 반올림 방식이다.** `transformSelectionBox` 의
// `Math.trunc` 를 `Math.round` 로 바꾸면 §4회전 항등에서 열하나 중 여섯이 빨개진다 —
// 주석이 아니라 이 시험이 그 금지를 지킨다.
//
// 고정 입력은 기본값 모양이 아니다(시험 규율 D2·D3): 원점 ≠ 0 · `x ≠ y` · 비정사각이며,
// `w − h` 가 홀수인 것과 짝수인 것을 **함께** 담는다 — 짝수만 담으면 반올림이 개입하는
// 갈래가 통째로 빠진다.
//
// @spec SPEC-CANVAS-013 REQ-01 · REQ-02

import { describe, expect, it } from 'vitest';

import type { CanvasBox } from './canvasGeometry';
import {
  swapsExtent,
  transformBoxWithin,
  transformLocalPoint,
  transformPointWithin,
  transformSelectionBox,
  unionBox,
  type TransformKind,
} from './canvasTransform';

const KINDS: readonly TransformKind[] = ['flipX', 'flipY', 'rotateCW', 'rotateCCW'];

/** §결정 3 이 실측한 그 열하나. 홀수·짝수 · 음수 원점 · 정사각을 함께 담는다. */
const BOXES: readonly CanvasBox[] = [
  { x: 0, y: 0, w: 5, h: 3 },
  { x: 7, y: 11, w: 160, h: 90 },
  { x: 3, y: 4, w: 4, h: 4 },
  { x: 10, y: 20, w: 7, h: 2 },
  { x: -5, y: -9, w: 13, h: 8 },
  { x: 0, y: 0, w: 1, h: 2 },
  { x: 50, y: 40, w: 100, h: 80 },
  { x: 0, y: 0, w: 2, h: 2 },
  { x: -3, y: 7, w: 9, h: 4 },
  { x: 12, y: -8, w: 3, h: 14 },
  { x: 100, y: 100, w: 101, h: 100 },
];

const E = 10000;

/** 비대칭 점 — 대칭 점은 뒤집기 결함을 통째로 감춘다. */
const LOCAL = { x: 1234, y: 8765 };

function rotateN(sel: CanvasBox, kind: TransformKind, times: number): CanvasBox {
  let out = sel;
  for (let i = 0; i < times; i += 1) out = transformSelectionBox(kind, out);
  return out;
}

// --- 로컬 격자 (M1) --------------------------------------------------------

describe('로컬 격자 변환 (AC-01 · AC-02 · AC-03 · AC-04)', () => {
  it.each(KINDS)('`%s` 가 정수를 낸다 — 나눗셈이 없다', (kind) => {
    const out = transformLocalPoint(kind, LOCAL, E);
    expect(Number.isInteger(out.x)).toBe(true);
    expect(Number.isInteger(out.y)).toBe(true);
  });

  it.each(['flipX', 'flipY'] as const)('같은 거울 `%s` 두 번은 제자리다 (K2)', (kind) => {
    const once = transformLocalPoint(kind, LOCAL, E);
    expect(transformLocalPoint(kind, once, E)).toEqual(LOCAL);
  });

  it.each(['rotateCW', 'rotateCCW'] as const)('`%s` 네 번은 제자리다 (K1)', (kind) => {
    let p = LOCAL;
    for (let i = 0; i < 4; i += 1) p = transformLocalPoint(kind, p, E);
    expect(p).toEqual(LOCAL);
  });

  it('오른쪽 한 번 → 왼쪽 한 번은 제자리다 (K1`)', () => {
    const cw = transformLocalPoint('rotateCW', LOCAL, E);
    expect(transformLocalPoint('rotateCCW', cw, E)).toEqual(LOCAL);
  });

  it('화면 좌표는 y 가 아래다 — `rotateCW` 에서 왼쪽 위가 오른쪽 위로 간다', () => {
    // 이 단언이 방향을 못박는다. 부호 하나가 뒤집히면 회전이 반대로 돌고, 그 결함은
    // 왕복·4회전 단언을 **전부 통과한다**(둘 다 방향에 무관하기 때문이다).
    expect(transformLocalPoint('rotateCW', { x: 0, y: 0 }, E)).toEqual({ x: E, y: 0 });
    expect(transformLocalPoint('rotateCW', { x: E, y: 0 }, E)).toEqual({ x: E, y: E });
  });

  it('`flipX` 는 세로축 거울이다 — 왼쪽 것이 오른쪽으로 간다', () => {
    expect(transformLocalPoint('flipX', { x: 0, y: 3000 }, E)).toEqual({ x: E, y: 3000 });
  });
});

// --- 선택 상자 (M2) --------------------------------------------------------

describe('선택 상자 (AC-10 · AC-11)', () => {
  it.each(['flipX', 'flipY'] as const)('`%s` 는 선택 상자를 바꾸지 않는다 (AC-10)', (kind) => {
    for (const sel of BOXES) expect(transformSelectionBox(kind, sel)).toEqual({ ...sel });
  });

  it('회전은 가로·세로를 맞바꾼다', () => {
    for (const sel of BOXES) {
      const out = transformSelectionBox('rotateCW', sel);
      expect(out.w, JSON.stringify(sel)).toBe(sel.h);
      expect(out.h, JSON.stringify(sel)).toBe(sel.w);
    }
  });

  it('회전이 중심을 **0.5 단위 이내**로 지킨다 (AC-11)', () => {
    for (const kind of ['rotateCW', 'rotateCCW'] as const) {
      for (const sel of BOXES) {
        const out = transformSelectionBox(kind, sel);
        const label = `${kind} ${JSON.stringify(sel)}`;
        expect(Math.abs(sel.x + sel.w / 2 - (out.x + out.w / 2)), label).toBeLessThanOrEqual(0.5);
        expect(Math.abs(sel.y + sel.h / 2 - (out.y + out.h / 2)), label).toBeLessThanOrEqual(0.5);
      }
    }
  });

  it('`swapsExtent` 가 회전 둘만 참이다', () => {
    expect(KINDS.filter(swapsExtent)).toEqual(['rotateCW', 'rotateCCW']);
  });
});

describe('반올림이 **0 쪽으로 버림**이다 — `Math.round` 가 아니다 (AC-12)', () => {
  it.each(['rotateCW', 'rotateCCW'] as const)('`%s` 네 번이면 열하나 모두 제자리다', (kind) => {
    // **이 단언이 `Math.trunc` 를 지킨다.** `Math.round`·`floor`·`ceil` 로 바꾸면 열하나 중
    // 여섯이 여기서 빨개진다 — 홀함수가 아니라 다음 회전의 오프셋이 앞 회전을 상쇄하지
    // 못하기 때문이다. 되돌리기가 없는 패널이므로(SPEC-CANVAS-010) 밀린 단위는 손으로
    // 되돌릴 수 없다.
    for (const sel of BOXES) {
      expect(rotateN(sel, kind, 4), JSON.stringify(sel)).toEqual({ ...sel });
    }
  });

  it('선택 상자는 두 번 돌리면 제자리다 — 상자는 180° 에 불변이다', () => {
    for (const sel of BOXES) {
      expect(rotateN(sel, 'rotateCW', 2), JSON.stringify(sel)).toEqual({ ...sel });
    }
  });

  it('`w − h` 가 홀수인 상자가 고정 입력에 실제로 있다', () => {
    // 짝수만 담으면 반올림이 개입하는 갈래가 통째로 빠지고, 위 단언이 **아무것도 재지
    // 않으면서** 초록이 된다.
    expect(BOXES.some((b) => Math.abs(b.w - b.h) % 2 === 1)).toBe(true);
  });
});

// --- 요소 층 (M2 · AC-12b · AC-12c · AC-12d) --------------------------------

/** 요소 여럿을 담은 장면 여섯. 선택 상자는 이 상자들의 합집합이다. */
const SCENES: readonly (readonly CanvasBox[])[] = [
  [{ x: 0, y: 0, w: 5, h: 3 }],
  [{ x: 7, y: 11, w: 160, h: 90 }],
  [
    { x: 10, y: 20, w: 7, h: 2 },
    { x: 30, y: 25, w: 4, h: 9 },
  ],
  [
    { x: -5, y: -9, w: 13, h: 8 },
    { x: 20, y: 3, w: 2, h: 2 },
    { x: 0, y: 0, w: 1, h: 2 },
  ],
  [
    { x: 50, y: 40, w: 100, h: 80 },
    { x: 200, y: 40, w: 30, h: 120 },
  ],
  [
    { x: 0, y: 0, w: 1, h: 1 },
    { x: 1, y: 1, w: 1, h: 1 },
    { x: 5, y: 0, w: 2, h: 7 },
  ],
];

/** 장면 한 번 변환 — 선택 상자를 매번 합집합에서 다시 구한다(화면이 하는 그대로다). */
function stepScene(kind: TransformKind, scene: readonly CanvasBox[]): CanvasBox[] {
  const sel = unionBox(scene);
  if (sel === undefined) return [...scene];
  return scene.map((b) => transformBoxWithin(kind, b, sel));
}

function runScene(kind: TransformKind, scene: readonly CanvasBox[], times: number): CanvasBox[] {
  let out = [...scene];
  for (let i = 0; i < times; i += 1) out = stepScene(kind, out);
  return out;
}

describe('요소 층 (AC-12b · AC-12c · AC-12d)', () => {
  it.each(['rotateCW', 'rotateCCW'] as const)('`%s` 네 번이면 모든 요소가 제자리다 (K1)', (kind) => {
    // **상자 하나만 재면 자식마다 반올림하는 구현이 초록으로 지나간다.** 산술을 선택 상자
    // 로컬 정수로 옮긴 것이 그 결함을 표현 불가능하게 만든다.
    for (const scene of SCENES) {
      expect(runScene(kind, scene, 4), JSON.stringify(scene)).toEqual(scene.map((b) => ({ ...b })));
    }
  });

  it('오른쪽 한 번 → 왼쪽 한 번이 제자리다 (K1`· AC-12d)', () => {
    // 사용자가 실제로 하는 몸짓이다 — 반대로 돌렸음을 깨닫고 되돌린다.
    for (const scene of SCENES) {
      const back = stepScene('rotateCCW', stepScene('rotateCW', scene));
      expect(back, JSON.stringify(scene)).toEqual(scene.map((b) => ({ ...b })));
    }
  });

  it('두 번 돌린 **요소**는 제자리가 아니다 (AC-12c)', () => {
    // 180° 돌렸는데 요소가 그대로면 그것이야말로 고장이다. 선택 상자는 불변이지만 그 안의
    // 요소는 아니며, 상자만 재는 단언이 이 사실을 감추지 못하게 한다.
    const moved = SCENES.filter((scene) => {
      const twice = runScene('rotateCW', scene, 2);
      return JSON.stringify(twice) !== JSON.stringify(scene);
    });
    expect(moved.length).toBeGreaterThan(0);
  });

  it.each(['flipX', 'flipY'] as const)('같은 거울 `%s` 두 번이면 제자리다 (K2)', (kind) => {
    for (const scene of SCENES) {
      expect(runScene(kind, scene, 2), JSON.stringify(scene)).toEqual(
        scene.map((b) => ({ ...b })),
      );
    }
  });

  it('둘을 가로로 뒤집으면 **자리를 맞바꾼다** (AC-09 · §결정 1)', () => {
    const left: CanvasBox = { x: 0, y: 10, w: 20, h: 10 };
    const right: CanvasBox = { x: 80, y: 10, w: 20, h: 10 };
    const [a, b] = stepScene('flipX', [left, right]);
    expect(a).toEqual(right);
    expect(b).toEqual(left);
  });

  it('선택 상자 자체는 뒤집어도 그대로다 (AC-10)', () => {
    for (const scene of SCENES) {
      const before = unionBox(scene);
      const after = unionBox(stepScene('flipX', scene));
      expect(after, JSON.stringify(scene)).toEqual(before);
    }
  });
});

// --- 점과 합집합 -----------------------------------------------------------

describe('점 변환과 합집합 (AC-08)', () => {
  it('점은 넓이 0 인 상자와 같은 산술을 지난다', () => {
    const sel: CanvasBox = { x: 5, y: 7, w: 40, h: 20 };
    const p = { x: 11, y: 13 };
    const viaBox = transformBoxWithin('rotateCW', { ...p, w: 0, h: 0 }, sel);
    expect(transformPointWithin('rotateCW', p, sel)).toEqual({ x: viaBox.x, y: viaBox.y });
  });

  it('합집합이 모두를 덮는 가장 작은 상자다', () => {
    expect(
      unionBox([
        { x: 10, y: 20, w: 5, h: 5 },
        { x: 0, y: 30, w: 4, h: 4 },
      ]),
    ).toEqual({ x: 0, y: 20, w: 15, h: 14 });
  });

  it('빈 목록은 **부재**다 — 0 크기 상자를 지어내지 않는다', () => {
    // 지어내면 그것이 곧 "아무것도 안 골랐는데 뒤집을 축이 있다" 가 된다.
    expect(unionBox([])).toBeUndefined();
  });
});
