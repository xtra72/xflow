// 경로 히트 테스트 (SPEC-CANVAS-008 M4).
//
// 이 파일이 겨누는 함정은 **D5** 다: 도형 **안쪽 깊은 곳만** 찍는 히트 시험은 바운딩 박스
// 판정으로도 전부 초록이다. 그래서 여기서 잰다고 말할 수 있는 유일한 것은 **오목 지역이
// 빗나가는가**이며, 그 한 줄이 REQ-07("바운딩 박스로 두지 않는다")의 유일한 가드다.
//
// 고정 도형은 **볼록이 아니다.** 볼록한 도형을 쓰면 바운딩 박스뿐 아니라 볼록 껍질 구현도
// 시험을 통과한다. 그래서 별과 십자를 쓰고, 그 둘의 오목 지역은 **바운딩 박스 안이면서
// 볼록 껍질 안이기도** 하다 — 두 잘못된 구현이 동시에 빨개지는 자리다.
//
// 고정 입력이 기본값 모양이 아니다:
//   - 축척 **가로 1.6 · 세로 1.5** — 단위 축척도 아니고 두 축이 같지도 않다. 축을 뒤바꾼
//     구현과 한 축만 쓰는 구현이 여기서 갈린다
//   - 상자 `{x:37, y:61, w:160, h:90}` — 원점 ≠ 0 · `x ≠ y` · **비정사각**(D2·D3)
//   - **비대칭 · 오목** 명령 목록. 대칭 볼록 도형은 아무것도 재지 못한다
//   - 곡선 경계 시험은 **곡선 구간**에서 찍는다 — 직선 구간은 평탄화가 항등이라 오차를
//     재지 못한다(D6)
//
// DOM 무의존이라 jsdom 없이 잰다 — 그것이 `isPointInPath` 를 기각한 이유 그 자체다.
//
// @spec SPEC-CANVAS-008 REQ-07 · AC-04 · 불변식 J4

import { describe, expect, it } from 'vitest';

import type { BoxGeometry, ElementStyle, PathElement } from './canvasConfig';
import { projectBox, type CanvasProjection, type PxPoint } from './canvasGeometry';
import { HIT_TOLERANCE_PX, hitTest } from './canvasHitTest';
import type { DrawContext2D } from './drawElement';
import { drawElement } from './drawElement';
import { FLATTEN_TOLERANCE_PX, flattenPath, isInsidePath } from './shapes/pathFlatten';
import { PATH_LOCAL_EXTENT, type PathCommand } from './shapes/pathTypes';

// --- 고정 입력 -----------------------------------------------------------

/** 축척 가로 1.6 · 세로 1.5. 두 축이 다른 것이 이 픽스처의 요점이다. */
const PROJ: CanvasProjection = {
  stage: { width: 800, height: 600 },
  canvas: { width: 500, height: 400 },
};

/** 원점 ≠ 0 · `x ≠ y` · 비정사각(D2·D3). */
const BOX: BoxGeometry = { x: 37, y: 61, w: 160, h: 90 };

/** 도형은 폭을 쓰지 않는다. */
const NO_WIDTHS: Record<string, number> = {};

/**
 * 로컬 격자 → 스테이지 px. **구현을 부르지 않고 시험이 스스로 센다** — 투영을 구현에서
 * 빌려 오면 투영이 통째로 틀려도 시험이 함께 틀려 초록이 된다.
 */
function toPx(x: number, y: number): PxPoint {
  const box = { x: 59.2, y: 91.5, w: 256, h: 135 };
  return {
    x: box.x + (x * box.w) / PATH_LOCAL_EXTENT,
    y: box.y + (y * box.h) / PATH_LOCAL_EXTENT,
  };
}

function path(id: string, cmds: readonly PathCommand[], style: ElementStyle = {}): PathElement {
  return { id, kind: 'path', geometry: { ...BOX }, path: [...cmds], style };
}

/**
 * 오목 지역 12종의 대표 — 꼭짓점 다섯의 별(중심 5000 · 바깥 반지름 5000 · 안 반지름 1910).
 *
 * 바깥 꼭짓점 다섯과 안쪽 꼭짓점 다섯이 번갈아 서고, 그 사이가 **상자 안이면서 도형 밖**
 * 인 자리다. 좌표는 손으로 적어 둔다 — 시험 안에서 계산하면 같은 삼각함수를 두 번 쓰는
 * 셈이라 부호를 뒤집은 결함이 양쪽에서 똑같이 뒤집혀 통과한다.
 */
const STAR5: readonly PathCommand[] = [
  { c: 'M', x: 5000, y: 0 },
  { c: 'L', x: 6123, y: 3455 },
  { c: 'L', x: 9755, y: 3455 },
  { c: 'L', x: 6817, y: 5590 },
  { c: 'L', x: 7939, y: 9045 },
  { c: 'L', x: 5000, y: 6910 },
  { c: 'L', x: 2061, y: 9045 },
  { c: 'L', x: 3183, y: 5590 },
  { c: 'L', x: 245, y: 3455 },
  { c: 'L', x: 3877, y: 3455 },
  { c: 'Z' },
];

/** 십자 — 겨드랑이 넷이 상자 모서리 안쪽의 빈 곳이다. 좌표가 전부 정수라 손으로 읽힌다. */
const CROSS: readonly PathCommand[] = [
  { c: 'M', x: 3400, y: 0 },
  { c: 'L', x: 6600, y: 0 },
  { c: 'L', x: 6600, y: 3400 },
  { c: 'L', x: 10000, y: 3400 },
  { c: 'L', x: 10000, y: 6600 },
  { c: 'L', x: 6600, y: 6600 },
  { c: 'L', x: 6600, y: 10000 },
  { c: 'L', x: 3400, y: 10000 },
  { c: 'L', x: 3400, y: 6600 },
  { c: 'L', x: 0, y: 6600 },
  { c: 'L', x: 0, y: 3400 },
  { c: 'L', x: 3400, y: 3400 },
  { c: 'Z' },
];

/**
 * **열린** 곡선 — px 공간에서 중심 (200,160) · 반지름 60 인 사분원이다.
 *
 * 로컬 좌표를 역으로 구해 적었기 때문에 격자 위에서는 찌그러져 보이지만, 상자의 두 축척이
 * 다시 늘여 놓으므로 **화면에서는 참된 원호**다. 그래야 "곡선에서 얼마나 떨어졌는가" 를
 * 동심원 반지름으로 정확히 말할 수 있고, 평탄화 오차가 경계를 얼마나 흔드는지 재진다(D6).
 * 정수로 반올림한 대가는 px 로 0.011 이하이며 아래 여유(0.1 이상)보다 한 자릿수 작다.
 *
 * `Z` 가 없다 — 열린 경로의 **내부 판정은 실패해야** 하고, 그래도 선 위에서는 잡혀야 한다.
 */
const OPEN_ARC: readonly PathCommand[] = [
  { c: 'M', x: 3156, y: 5074 },
  { c: 'C', x1: 3156, y1: 2619, x2: 4206, y2: 630, x: 5500, y: 630 },
];

/** 원호의 px 중심과 반지름 — 위 명령 목록이 역으로 나온 출처다. */
const ARC_CENTER: PxPoint = { x: 200, y: 160 };
const ARC_RADIUS = 60;

/** 원호 한가운데(225°) 바깥으로 `d` px 떨어진 자리. */
function outsideArc(d: number): PxPoint {
  const u = Math.SQRT1_2;
  return { x: ARC_CENTER.x - (ARC_RADIUS + d) * u, y: ARC_CENTER.y - (ARC_RADIUS + d) * u };
}

// --- 고정 입력이 제 모습인지부터 못박는다 --------------------------------

describe('고정 입력', () => {
  it('상자가 잰 그대로다 — 이 파일이 딛는 수치를 먼저 고정한다', () => {
    // `37 × 1.6` 은 이진 부동소수에서 59.199999999999996 이다. 아래 `toPx` 가 쓰는
    // 59.2 와의 차이는 4e-15 px 로 집기 여유(6px)보다 15 자릿수 작다.
    expect(projectBox(BOX, PROJ)).toMatchObject({
      x: expect.closeTo(59.2, 12),
      y: 91.5,
      w: 256,
      h: 135,
    });
  });

  it('고정 도형이 **오목**하다 — 볼록 도형이면 이 파일 전체가 아무것도 재지 못한다', () => {
    // 별의 오목 지역: 안쪽 꼭짓점 하나를 사이에 둔 두 바깥 꼭짓점의 이등분선 위, 안 반지름
    // (1910)보다 바깥인 자리. 볼록 껍질(바깥 꼭짓점 다섯의 오각형) 안이기도 하다.
    const notchLocal = { x: 7057, y: 2168 };
    const outerA = { x: 5000, y: 0 };
    const outerB = { x: 9755, y: 3455 };
    const cross =
      (outerB.x - outerA.x) * (notchLocal.y - outerA.y) -
      (notchLocal.x - outerA.x) * (outerB.y - outerA.y);
    const centerCross =
      (outerB.x - outerA.x) * (5000 - outerA.y) - (5000 - outerA.x) * (outerB.y - outerA.y);
    // 중심과 같은 쪽 = 볼록 껍질 **안쪽**. 그런데 아래 시험은 이 점이 빗나감을 단언한다.
    expect(Math.sign(cross)).toBe(Math.sign(centerCross));
  });
});

// --- 오목 지역 (D5) ------------------------------------------------------

describe('hitsPath — 경로는 제 윤곽으로 잡힌다, 상자로 잡히지 않는다 (AC-04 · D5)', () => {
  const star = path('star', STAR5);

  it('별의 한가운데는 잡힌다', () => {
    expect(hitTest([star], toPx(5000, 5000), PROJ, NO_WIDTHS)).toEqual({ nodeId: 'star' });
  });

  it('바깥 꼭짓점 안쪽도 잡힌다 — 팔 안까지 윤곽이 따라간다', () => {
    // 위 꼭짓점(5000,0)에서 중심 쪽으로 조금 들어온 자리.
    expect(hitTest([star], toPx(5000, 900), PROJ, NO_WIDTHS)).toEqual({ nodeId: 'star' });
  });

  it('**오목한 사이는 빗나간다** — 상자 안이지만 도형 밖이다 (이 결정의 유일한 가드)', () => {
    // 바운딩 박스 판정이면 잡힌다. 볼록 껍질 판정이어도 잡힌다. 윤곽 판정만 빗나간다.
    // 가장 가까운 변까지 로컬 1280 여 단위 = px 로 가로 32.9 · 세로 17.4 이므로, 집기
    // 여유 6px 로는 어느 축으로도 닿지 않는다.
    expect(hitTest([star], toPx(7057, 2168), PROJ, NO_WIDTHS)).toBeUndefined();
  });

  it('상자의 네 모서리도 빗나간다', () => {
    for (const [x, y] of [
      [0, 0],
      [10000, 0],
      [10000, 10000],
      [0, 10000],
    ] as const) {
      expect(hitTest([star], toPx(x, y), PROJ, NO_WIDTHS), `모서리 (${x},${y})`).toBeUndefined();
    }
  });

  it('십자의 겨드랑이도 빗나간다 — 오목이 하나가 아님을 확인한다', () => {
    const cross = path('cross', CROSS);
    expect(hitTest([cross], toPx(5000, 5000), PROJ, NO_WIDTHS)).toEqual({ nodeId: 'cross' });
    for (const [x, y] of [
      [1000, 1000],
      [9000, 1000],
      [9000, 9000],
      [1000, 9000],
    ] as const) {
      expect(hitTest([cross], toPx(x, y), PROJ, NO_WIDTHS), `겨드랑이 (${x},${y})`).toBeUndefined();
    }
  });

  it('윤곽 위(변)에서는 잡힌다 — 채우지 않아도 선은 잡혀야 한다', () => {
    // 십자의 위쪽 변(y=0, x 3400..6600) 한가운데.
    expect(hitTest([path('c', CROSS)], toPx(5000, 0), PROJ, NO_WIDTHS)).toEqual({ nodeId: 'c' });
  });

  it('**닫힘 변**도 다른 변과 똑같이 잡는다 — 마지막 한 변만 빠지는 결함을 막는다', () => {
    // 십자의 닫힘 변은 마지막 점 (3400,3400) 에서 첫 점 (3400,0) 으로 돌아오는 세로 변이다.
    // 그 변에서 로컬 100 단위(= 2.56px) 바깥, 도형 **밖**인 자리를 찍는다. 안쪽 판정은
    // 실패하고 다른 변들은 22.9px 넘게 떨어져 있으므로, **닫힘 변만이** 이 점을 잡는다.
    expect(hitTest([path('c', CROSS)], toPx(3300, 1700), PROJ, NO_WIDTHS)).toEqual({ nodeId: 'c' });
    // 같은 방향으로 더 나가면(로컬 400 = 10.24px) 빗나간다 — 여유가 무한하지 않다.
    expect(hitTest([path('c', CROSS)], toPx(3000, 1700), PROJ, NO_WIDTHS)).toBeUndefined();
  });
});

// --- 곡선 경계 (D6) ------------------------------------------------------

describe('hitsPath — 곡선 경계는 평탄화 오차만큼만 흔들린다 (AC-04 · D6 · J4)', () => {
  const arc = path('arc', OPEN_ARC);

  it('집기 여유 안쪽(5.4px)은 잡힌다 — 평탄화가 참 곡선을 0.5px 안쪽까지 따라간다', () => {
    // 평탄화가 없으면(현 하나로 접으면) 이 점은 현에서 23px 떨어져 있어 빗나간다.
    expect(hitTest([arc], outsideArc(5.4), PROJ, NO_WIDTHS)).toEqual({ nodeId: 'arc' });
  });

  it('집기 여유 + 평탄화 오차 바깥(6.6px)은 빗나간다', () => {
    expect(hitTest([arc], outsideArc(6.6), PROJ, NO_WIDTHS)).toBeUndefined();
  });

  it('직선 구간이 아니라 **곡선 구간**에서 쟀다 — 그 사실을 수로 적는다', () => {
    // 원호 한가운데는 두 끝점을 잇는 현에서 sagitta 만큼 떨어져 있다: 60(1−cos45°) ≈ 17.6px.
    // 이 값이 0 이면(곧 직선이면) 평탄화가 항등이라 위 두 시험이 아무것도 재지 못한다.
    const sagitta = ARC_RADIUS * (1 - Math.SQRT1_2);
    expect(sagitta).toBeGreaterThan(17);
  });

  it('열린 곡선의 **안쪽**은 잡히지 않는다 — 내부 판정에 참여할 자격이 없다', () => {
    // 원호를 암묵적으로 닫으면 생기는 렌즈 모양 영역의 한가운데. 참 곡선에서 17px 이상
    // 떨어져 있으므로 변 거리로도 닿지 않는다.
    const inside = {
      x: ARC_CENTER.x - (ARC_RADIUS - 8) * Math.SQRT1_2,
      y: ARC_CENTER.y - (ARC_RADIUS - 8) * Math.SQRT1_2,
    };
    expect(hitTest([arc], inside, PROJ, NO_WIDTHS)).toBeUndefined();
  });

  it('열린 곡선도 **선 위에서는** 잡힌다', () => {
    expect(hitTest([arc], outsideArc(0), PROJ, NO_WIDTHS)).toEqual({ nodeId: 'arc' });
  });
});

// --- 두께와 여유 ---------------------------------------------------------

describe('hitsPath — 임계는 선 판정과 같은 식이다', () => {
  it('두꺼운 선은 제 두께만큼 잡힌다', () => {
    const thick = path('arc', OPEN_ARC, { strokeWidth: 40 });
    // 20px(두께의 절반)는 집기 여유 6px 보다 크다 — 두께를 무시하는 구현이면 빗나간다.
    expect(hitTest([thick], outsideArc(18), PROJ, NO_WIDTHS)).toEqual({ nodeId: 'arc' });
    expect(hitTest([thick], outsideArc(24), PROJ, NO_WIDTHS)).toBeUndefined();
  });

  it('두께 0 도 집기 여유만큼은 잡힌다 — 누를 수 없는 요소를 만들지 않는다', () => {
    const hairline = path('arc', OPEN_ARC, { strokeWidth: 0 });
    expect(hitTest([hairline], outsideArc(4), PROJ, NO_WIDTHS)).toEqual({ nodeId: 'arc' });
  });
});

// --- 견고성 --------------------------------------------------------------

describe('hitsPath — 퇴화와 가시성', () => {
  it('빈 명령 목록은 어디를 찍어도 빗나가고 던지지 않는다', () => {
    const empty = path('empty', []);
    expect(hitTest([empty], toPx(5000, 5000), PROJ, NO_WIDTHS)).toBeUndefined();
  });

  it('점 하나짜리 부분 경로는 **점 거리**로 잡힌다 — 되찾을 수 없는 요소를 만들지 않는다', () => {
    const dot = path('dot', [{ c: 'M', x: 5000, y: 5000 }]);
    expect(hitTest([dot], toPx(5000, 5000), PROJ, NO_WIDTHS)).toEqual({ nodeId: 'dot' });
    const near = toPx(5000, 5000);
    expect(hitTest([dot], { x: near.x + 4, y: near.y }, PROJ, NO_WIDTHS)).toEqual({ nodeId: 'dot' });
    expect(hitTest([dot], { x: near.x + 9, y: near.y }, PROJ, NO_WIDTHS)).toBeUndefined();
  });

  it('`style.visible === false` 인 경로는 잡히지 않는다(001 의 규율)', () => {
    const hidden = path('hidden', STAR5, { visible: false });
    expect(hitTest([hidden], toPx(5000, 5000), PROJ, NO_WIDTHS)).toBeUndefined();
  });

  it('겹친 경로는 **뒤에 있는 것**이 이긴다(배열 순서 = z-order)', () => {
    const below = path('below', CROSS);
    const above = { ...path('above', CROSS), id: 'above' };
    expect(hitTest([below, above], toPx(5000, 5000), PROJ, NO_WIDTHS)).toEqual({ nodeId: 'above' });
  });

  it('음수 크기 상자에서도 던지지 않는다 — 좌표가 뒤집힐 뿐이다', () => {
    const flipped: PathElement = { ...path('f', CROSS), geometry: { x: 197, y: 151, w: -160, h: -90 } };
    expect(() => hitTest([flipped], toPx(5000, 5000), PROJ, NO_WIDTHS)).not.toThrow();
    // 뒤집힌 상자의 중심은 제자리다 — 십자의 한가운데는 그대로 잡힌다.
    expect(hitTest([flipped], toPx(5000, 5000), PROJ, NO_WIDTHS)).toEqual({ nodeId: 'f' });
  });

  it('경로 요소는 입력 배열도 명령 목록도 바꾸지 않는다', () => {
    const el = path('star', STAR5);
    const before = JSON.stringify(el);
    hitTest([el], toPx(7057, 2168), PROJ, NO_WIDTHS);
    expect(JSON.stringify(el)).toBe(before);
  });
});

// --- 평탄화 자체 ---------------------------------------------------------

describe('flattenPath', () => {
  it('`Z` 로 닫힌 부분 경로만 닫힘으로 표시된다', () => {
    const closed = flattenPath(
      [
        { c: 'M', x: 0, y: 0 },
        { c: 'L', x: 10, y: 0 },
        { c: 'L', x: 10, y: 10 },
        { c: 'Z' },
      ],
      FLATTEN_TOLERANCE_PX,
    );
    expect(closed).toHaveLength(1);
    expect(closed[0]?.closed).toBe(true);
    // 닫힘 변의 끝점을 중복해 담지 않는다 — 담으면 변이 하나 더 세어진다.
    expect(closed[0]?.points).toHaveLength(3);
  });

  it('`Z` 로 끝난 목록에 점 하나짜리 꼬리가 붙지 않는다', () => {
    const out = flattenPath(
      [
        { c: 'M', x: 0, y: 0 },
        { c: 'L', x: 10, y: 0 },
        { c: 'Z' },
      ],
      FLATTEN_TOLERANCE_PX,
    );
    expect(out).toHaveLength(1);
  });

  it('`M` 마다 부분 경로가 갈린다', () => {
    const out = flattenPath(
      [
        { c: 'M', x: 0, y: 0 },
        { c: 'L', x: 10, y: 0 },
        { c: 'M', x: 50, y: 50 },
        { c: 'L', x: 60, y: 50 },
        { c: 'Z' },
      ],
      FLATTEN_TOLERANCE_PX,
    );
    expect(out.map((s) => s.closed)).toEqual([false, true]);
  });

  it('`Z` 뒤에 이어지는 명령은 **닫은 자리에서** 자란다', () => {
    const out = flattenPath(
      [
        { c: 'M', x: 0, y: 0 },
        { c: 'L', x: 10, y: 0 },
        { c: 'Z' },
        { c: 'L', x: 0, y: 10 },
      ],
      FLATTEN_TOLERANCE_PX,
    );
    expect(out).toHaveLength(2);
    // 두 번째 부분 경로가 (0,0) 에서 시작하지 않으면 없는 자리에서 선이 뻗어 나온다.
    expect(out[1]?.points).toEqual([
      { x: 0, y: 0 },
      { x: 0, y: 10 },
    ]);
  });

  it('허용 오차를 죄면 곡선이 더 잘게 나뉜다 — 이 인자가 무동작이 아니다', () => {
    const cmds = [
      { c: 'M', x: 140, y: 160 },
      { c: 'C', x1: 140, y1: 126.86, x2: 166.86, y2: 100, x: 200, y: 100 },
    ] as const;
    const coarse = flattenPath(cmds, 8)[0]?.points.length ?? 0;
    const fine = flattenPath(cmds, FLATTEN_TOLERANCE_PX)[0]?.points.length ?? 0;
    expect(fine).toBeGreaterThan(coarse);
    expect(coarse).toBeGreaterThan(1);
  });

  it('한 제어점만 멀리 나간 곡선도 평평하다고 보지 않는다', () => {
    // 두 제어점 중 **큰 쪽**을 보지 않으면 이 곡선이 현 하나로 접힌다.
    const out = flattenPath(
      [
        { c: 'M', x: 0, y: 0 },
        { c: 'C', x1: 0, y1: 100, x2: 100, y2: 100, x: 100, y: 0 },
      ],
      FLATTEN_TOLERANCE_PX,
    );
    expect(out[0]?.points.length ?? 0).toBeGreaterThan(8);
  });

  it('명령 없이 시작하는 목록도 견딘다 — 첫 좌표 자신이 원점이다', () => {
    const out = flattenPath(
      [
        { c: 'L', x: 5, y: 5 },
        { c: 'L', x: 9, y: 5 },
      ],
      FLATTEN_TOLERANCE_PX,
    );
    expect(out[0]?.points).toEqual([
      { x: 5, y: 5 },
      { x: 9, y: 5 },
    ]);
  });
});

describe('isInsidePath — nonzero winding', () => {
  const square = flattenPath(
    [
      { c: 'M', x: 0, y: 0 },
      { c: 'L', x: 100, y: 0 },
      { c: 'L', x: 100, y: 100 },
      { c: 'L', x: 0, y: 100 },
      { c: 'Z' },
    ],
    FLATTEN_TOLERANCE_PX,
  );

  it('닫힌 사각형의 안쪽은 안쪽이고 바깥은 바깥이다', () => {
    expect(isInsidePath(square, { x: 50, y: 50 })).toBe(true);
    expect(isInsidePath(square, { x: 150, y: 50 })).toBe(false);
  });

  it('정점을 지나는 수평선이 두 번 세어지지 않는다', () => {
    // 마름모의 좌우 꼭짓점이 정확히 `y = 50` 에 있다. 그 높이의 수평선은 **정점 둘을
    // 지나므로**, 정점을 두 변 모두에 세는 구현에서는 감기 수가 2 나 0 이 되어 판정이
    // 뒤집힌다. 사각형으로는 이 결함을 잴 수 없다 — 그 정점들은 수평 변 위에 있어서
    // 어느 변도 수평선을 "가로지르지" 않기 때문이다.
    const diamond = flattenPath(
      [
        { c: 'M', x: 50, y: 0 },
        { c: 'L', x: 100, y: 50 },
        { c: 'L', x: 50, y: 100 },
        { c: 'L', x: 0, y: 50 },
        { c: 'Z' },
      ],
      FLATTEN_TOLERANCE_PX,
    );
    expect(isInsidePath(diamond, { x: 50, y: 50 })).toBe(true);
    expect(isInsidePath(diamond, { x: 120, y: 50 })).toBe(false);
    expect(isInsidePath(diamond, { x: -20, y: 50 })).toBe(false);
    // 같은 높이에서 마름모 밖이지만 **상자 안**인 자리(왼쪽 위 비탈 바깥).
    expect(isInsidePath(diamond, { x: 10, y: 10 })).toBe(false);
  });

  it('윤곽 **위**의 점은 안쪽으로 떨어진다 — 히트 판정에는 영향이 없다', () => {
    // 변 위는 거리 0 이라 어차피 잡힌다. 이 성질을 시험이 알고 있어야 다음 사람이
    // "경계가 왜 안쪽인가" 를 다시 발굴하지 않는다.
    expect(isInsidePath(square, { x: 50, y: 0 })).toBe(true);
  });

  it('열린 부분 경로는 안쪽 판정에 **참여하지 않는다**', () => {
    const open = flattenPath(
      [
        { c: 'M', x: 0, y: 0 },
        { c: 'L', x: 100, y: 0 },
        { c: 'L', x: 100, y: 100 },
        { c: 'L', x: 0, y: 100 },
      ],
      FLATTEN_TOLERANCE_PX,
    );
    expect(isInsidePath(open, { x: 50, y: 50 })).toBe(false);
  });

  it('반대로 감은 안쪽 윤곽은 구멍을 뚫는다 — 감기 수를 **더한다**', () => {
    const donut = flattenPath(
      [
        { c: 'M', x: 0, y: 0 },
        { c: 'L', x: 100, y: 0 },
        { c: 'L', x: 100, y: 100 },
        { c: 'L', x: 0, y: 100 },
        { c: 'Z' },
        { c: 'M', x: 30, y: 30 },
        { c: 'L', x: 30, y: 70 },
        { c: 'L', x: 70, y: 70 },
        { c: 'L', x: 70, y: 30 },
        { c: 'Z' },
      ],
      FLATTEN_TOLERANCE_PX,
    );
    expect(isInsidePath(donut, { x: 50, y: 50 })).toBe(false);
    expect(isInsidePath(donut, { x: 10, y: 50 })).toBe(true);
  });
});

// --- 그림과 손을 한 시험에서 함께 잰다 -----------------------------------

describe('그린 자리와 잡히는 자리는 같은 상자에서 나온다 (이음매)', () => {
  it('렌더가 기록한 정점을 그대로 찍으면 잡힌다', () => {
    // 한 층만 세운 시험은 두 층이 서로 다른 상자를 봐도 각자 초록이다. 여기서는 렌더가
    // 실제로 기록한 좌표를 히트 판정에 그대로 넣어 **두 층이 같은 상자를 보는지** 잰다.
    const seen: PxPoint[] = [];
    const stub = {
      save() {},
      restore() {},
      setTransform() {},
      beginPath() {},
      rect() {},
      ellipse() {},
      moveTo(x: number, y: number) {
        seen.push({ x, y });
      },
      lineTo(x: number, y: number) {
        seen.push({ x, y });
      },
      closePath() {},
      bezierCurveTo() {},
      stroke() {},
      fill() {},
      fillText() {},
      measureText: () => ({ width: 0 }),
      clearRect() {},
      fillRect() {},
      fillStyle: '',
      strokeStyle: '',
      lineWidth: 1,
      globalAlpha: 1,
      font: '',
      textAlign: 'left' as CanvasTextAlign,
      textBaseline: 'middle' as CanvasTextBaseline,
    } satisfies DrawContext2D;

    const el = path('star', STAR5, { stroke: '#000' });
    drawElement(stub, el, { stroke: '#000' }, undefined, PROJ);

    expect(seen.length).toBeGreaterThan(9);
    for (const point of seen) {
      expect(hitTest([el], point, PROJ, NO_WIDTHS), `정점 ${point.x},${point.y}`).toEqual({
        nodeId: 'star',
      });
    }
  });
});

// --- 불변식 J4 -----------------------------------------------------------

describe('불변식 J4 — 평탄화 오차 ≪ 집기 여유', () => {
  it('**관계**를 단언한다 — 두 수를 따로 세지 않는다', () => {
    // 두 상수를 각각 0.5 · 6 으로 못박으면 한쪽을 고칠 때 다른 쪽을 함께 고치라는 말을
    // 아무도 하지 않는다. 깨지는 신호는 값이 아니라 **비**다(plan.md 불변식 J4).
    expect(FLATTEN_TOLERANCE_PX * 4).toBeLessThan(HIT_TOLERANCE_PX);
  });

  it('그 관계가 무동작이 아니다 — 두 상수가 실제로 양수다', () => {
    expect(FLATTEN_TOLERANCE_PX).toBeGreaterThan(0);
    expect(HIT_TOLERANCE_PX).toBeGreaterThan(0);
  });
});
