// 임의 앵커를 **값으로** 못박는다 (SPEC-CANVAS-011 M3' · AC-18 ~ AC-33).
//
// 고정 아홉(`anchors.test.ts`)과 갈라 두는 까닭은 성질이 반대이기 때문이다 — 저쪽은
// "저장하지 않는다"(A1)를 재고 이쪽은 "저장한다"(A10)를 잰다. 한 파일에 두면 두 규율이
// 섞여 읽히고, 다음 사람이 어느 쪽 규율을 고치는 중인지 알기 어려워진다.
//
// ## 투영을 1000×800 로 잡은 까닭
//
// 캔버스 500×400 에 스테이지 1000×800 이면 캔버스 한 단위가 화면 **2 px** 이다. AC-22 의
// 어긋남을 화면 px 로 환산해 말할 수 있어야 그 수가 뜻을 갖는다 — "0.5 단위" 만으로는
// 그것이 눈에 보이는 양인지 알 수 없다.
//
// ## 도구는 여기서 재지 않는다
//
// AC-17 · AC-23 · AC-26 · AC-27 · AC-28 은 **몸짓**(더블클릭 · 도구 켜짐)의 성질이고,
// 그 표면은 M3'b 가 세운다. 여기서 재는 것은 그 몸짓이 부를 **순수 함수**들이다.
//
// @spec SPEC-CANVAS-011 REQ-02'

import fs from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';

import {
  parseNodes,
  type BoxGeometry,
  type CanvasElement,
  type PathElement,
  type RectElement,
} from '../canvasConfig';
import {
  projectPointIn,
  unprojectBox,
  unprojectPoint,
  type CanvasPoint,
  type CanvasProjection,
  type PxBox,
} from '../canvasGeometry';
import { patchNodeGeometry, resizeBox } from '../canvasEditGeometry';
import { outlineBox } from '../canvasOutline';
import {
  GROUP_LOCAL_EXTENT,
  type GroupElement,
  type OutlinedNode,
} from '../group/groupTypes';
import { isConnector } from './connectorTypes';
import { PATH_LOCAL_EXTENT, type PathCommand } from '../shapes/pathTypes';
import {
  toAbsolutePointExact,
  toAbsolutePointRounded,
} from '../group/groupCoords';
import { addAnchorAt, anchorPoints, removeAnchor } from './anchors';
import { ANCHOR_LOCAL_EXTENT, ANCHOR_SNAP_LOCAL, type CustomAnchor } from './anchorTypes';

/** 캔버스 한 단위 = 화면 2 px. 머리말 참조. */
const PROJ: CanvasProjection = {
  stage: { width: 1000, height: 800 },
  canvas: { width: 500, height: 400 },
};
const PX_PER_UNIT = 2;

const NO_WIDTHS: Readonly<Record<string, number>> = {};

/** SPEC 이 AC-18 에 적은 그 상자 — (100,100)-(300,300). */
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

const GROUP: GroupElement = {
  id: 'g',
  kind: 'group',
  geometry: { x: 100, y: 100, w: 200, h: 200 },
  parts: [],
};

/** 5각 별 열 꼭지점. 로컬 격자 위의 정수라 붙임의 "정확히" 를 잴 수 있다. */
function starCommands(): PathCommand[] {
  const out: PathCommand[] = [];
  for (let i = 0; i < 10; i += 1) {
    const radius = i % 2 === 0 ? 5000 : 2000;
    const angle = (Math.PI / 5) * i - Math.PI / 2;
    out.push({
      c: i === 0 ? 'M' : 'L',
      x: Math.round(5000 + radius * Math.cos(angle)),
      y: Math.round(5000 + radius * Math.sin(angle)),
    });
  }
  out.push({ c: 'Z' });
  return out;
}

const STAR_COMMANDS = starCommands();

/** 좌표를 가진 명령만 — `Z` 는 붙일 자리가 없다. */
const STAR_VERTICES = STAR_COMMANDS.filter(
  (cmd): cmd is Extract<PathCommand, { x: number; y: number }> => 'x' in cmd,
);

const STAR: PathElement = {
  id: 'p',
  kind: 'path',
  geometry: { x: 100, y: 100, w: 200, h: 200 },
  path: STAR_COMMANDS,
  style: {},
};

/** 경로 명령 하나가 **그려지는** 캔버스 단위 자리. 렌더가 지나는 그 산술이다. */
function drawnAt(node: OutlinedNode, local: { x: number; y: number }): CanvasPoint {
  const px: PxBox = outlineBox(node, PROJ, NO_WIDTHS);
  return unprojectPoint(projectPointIn(local, px), PROJ);
}

/** 8핸들 크기 조절의 **실제 통로**. 몸짓만 빼면 손이 하는 일과 같은 경로다. */
function resizeTo(node: OutlinedNode, geometry: BoxGeometry): OutlinedNode {
  const resized = resizeBox(node.geometry as BoxGeometry, 'se', {
    x: geometry.x + geometry.w,
    y: geometry.y + geometry.h,
  });
  const out = patchNodeGeometry([node], node.id, resized)[0]!;
  // 통로는 받은 배열을 그대로 map 하므로 연결선이 나올 길이 없다(011 M4). 단언(`as`)
  // 대신 던지는 것은 그 사실이 깨졌을 때 조용히 지나가지 않게 하려는 것이다.
  if (isConnector(out)) throw new Error('상자 노드만 들어간다');
  return out;
}

/**
 * 파싱된 첫 노드의 임의 앵커. 연결선에는 그 필드가 없으므로(011 M4) 상자 노드만 읽는다.
 *
 * 헬퍼로 둔 것은 아래 셋이 같은 것을 재기 때문이다 — 좁히기를 자리마다 적으면 그중 하나가
 * 다른 좁히기를 쓰는 날 같은 단언이 다른 것을 재기 시작한다.
 */
function parsedAnchors(raw: readonly unknown[]): CustomAnchor[] | undefined {
  const node = parseNodes(raw)[0];
  return node !== undefined && !isConnector(node) ? node.anchors : undefined;
}

// --- 격자를 파생시켰다 (A14 규율) -------------------------------------------

describe('앵커 격자가 파생값이다', () => {
  it('경로 격자에서 나온다 — 값을 베끼지 않았다', () => {
    expect(ANCHOR_LOCAL_EXTENT).toBe(PATH_LOCAL_EXTENT);
  });

  it('그 줄에 숫자 리터럴이 없다', () => {
    // 004 가 `GROUP_LOCAL_EXTENT` 에 세운 그 요구다. 값을 적어 두면 언젠가 한쪽만 바뀐다.
    const line = anchorTypesSource()
      .split('\n')
      .find((l) => l.includes('export const ANCHOR_LOCAL_EXTENT'));
    expect(line).toBeDefined();
    expect(line).not.toMatch(/\d/);
  });

  it('그룹 격자와도 같은 값이다 — 재사용한 산술이 그 격자로 매개변수화되어 있다', () => {
    // 되돌림 한 쌍과 `toLocalPoint` 은 `localProjection` 을 지나고 그 `canvas` 축은
    // `GROUP_LOCAL_SIZE` 다. 셋이 갈라지는 날 앵커는 **그룹 격자**로 셈하면서 상수는
    // 다른 값을 말하게 되고, 그 어긋남은 예외 없이 화면에서만 드러난다.
    expect(ANCHOR_LOCAL_EXTENT).toBe(GROUP_LOCAL_EXTENT);
  });
});

// --- AC-18: 로컬 격자로 저장된다 --------------------------------------------

describe('좌표가 로컬 격자로 저장된다 (AC-18 · A10)', () => {
  it('상자 (100,100)-(300,300) 의 한가운데가 로컬 (5000,5000) 이다', () => {
    const node = addAnchorAt(BOX, { x: 200, y: 200 }, 'a');
    expect(node.anchors).toEqual([{ id: 'a', x: 5000, y: 5000 }]);
  });

  it('캔버스 값(200)이 저장되지 않는다', () => {
    // 캔버스 좌표로 저장했다면 (200,200) 이 남고, 도형을 늘려도 따라오지 않는다.
    const node = addAnchorAt(BOX, { x: 200, y: 200 }, 'a');
    expect(node.anchors?.[0]?.x).not.toBe(200);
    expect(node.anchors?.[0]?.y).not.toBe(200);
  });

  it('모서리는 격자의 끝이다', () => {
    const nw = addAnchorAt(BOX, { x: 100, y: 100 }, 'a');
    const se = addAnchorAt(BOX, { x: 300, y: 300 }, 'a');
    expect(nw.anchors?.[0]).toEqual({ id: 'a', x: 0, y: 0 });
    expect(se.anchors?.[0]).toEqual({
      id: 'a',
      x: ANCHOR_LOCAL_EXTENT,
      y: ANCHOR_LOCAL_EXTENT,
    });
  });

  it('상자 밖도 저술이다 — 격자 밖 값을 죄지 않는다', () => {
    // 캔버스 밖 좌표가 합법인 저술이라는 001 규율(가정 A5)과 같은 방향이다.
    const node = addAnchorAt(BOX, { x: 400, y: 50 }, 'a');
    expect(node.anchors?.[0]).toEqual({ id: 'a', x: 15000, y: -2500 });
  });
});

// --- AC-19 + AC-20: 저장은 그대로, 캔버스 자리는 따라온다 --------------------

describe('늘려도 저장 좌표가 그대로이고 캔버스 자리는 따라온다 (AC-19 · AC-20)', () => {
  // **둘을 한 시험에서 함께 잰다.** 하나만 재면 절반만 잰 것이다 — 저장 좌표만 재면
  // 앵커가 얼어붙은 것과 구분되지 않고, 캔버스 자리만 재면 크기 조절 때마다 저장 좌표를
  // 고쳐 쓰는 구현도 통과한다.
  //
  // 크기 조절은 **실제 통로**(`resizeBox` → `patchNodeGeometry`)를 지난다. 기하 쓰기가
  // 그 한 함수를 지나므로, 훗날 누군가 "친절하게" 앵커까지 함께 옮기는 코드를 그 자리에
  // 넣으면 아래 첫 단언이 빨개진다. 펼침(`{...node, geometry}`)으로 늘리면 그 손길이
  // 시험을 비껴간다.
  it('두 배로 늘려도 저장 좌표는 한 자리도 바뀌지 않고, 캔버스 자리는 새 상자에서 나온다', () => {
    const node = addAnchorAt(BOX, { x: 200, y: 200 }, 'a');
    const stored = JSON.stringify(node.anchors);
    const before = anchorPoints(node, PROJ, NO_WIDTHS).get('a');

    const grown = resizeTo(node, { x: 100, y: 100, w: 400, h: 400 });

    // AC-19 — 저장 좌표 불변. 그 성질을 지키는 **코드가 한 줄도 없다**(로컬 격자의 결과다).
    expect(grown.geometry).toEqual({ x: 100, y: 100, w: 400, h: 400 });
    expect(JSON.stringify(grown.anchors)).toBe(stored);

    // AC-20 — 그런데 캔버스 자리는 새 상자에서 다시 나온다.
    const after = anchorPoints(grown, PROJ, NO_WIDTHS).get('a');
    expect(before).toEqual({ x: 200, y: 200 });
    expect(after).toEqual({ x: 300, y: 300 });
    expect(after).not.toEqual(before);
  });

  it('상자를 옮겨도 같은 성질이다 — 저장은 그대로, 자리는 따라간다', () => {
    const node = addAnchorAt(BOX, { x: 150, y: 250 }, 'a');
    const stored = JSON.stringify(node.anchors);
    const moved = patchNodeGeometry([node], node.id, { x: 300, y: 300, w: 200, h: 200 })[0]!;
    expect(JSON.stringify(moved.anchors)).toBe(stored);
    expect(anchorPoints(moved, PROJ, NO_WIDTHS).get('a')).toEqual({ x: 350, y: 450 });
  });
});

describe('크기 조절 코드가 앵커를 **모른다** (AC-19 의 구조적 근거)', () => {
  // 위 두 시험은 동작을 재고, 이것은 **구조**를 잰다. 둘 다 있어야 하는 까닭은 동작
  // 시험이 "지금 안 옮긴다" 만 말하기 때문이다 — 훗날 누군가 기하 쓰기 통로 안에서
  // 앵커를 함께 옮기는 친절을 베풀면 그 순간 A10 의 근거(로컬 격자라서 코드가 없다)가
  // 사라지고, 크기 조절 경로가 둘이 되는 문이 열린다. 004 가 `parts` 에 세운 가드
  // (`canvasEditGeometry.ts` 가 `parts` 를 모른다)와 같은 자리, 같은 규율이다.
  it('`canvasEditGeometry.ts` 에 `anchors` 라는 글자가 없다', () => {
    const text = stripComments(read('..', 'canvasEditGeometry.ts'));
    expect(text).not.toContain('anchors');
  });

  it('`group/groupOps.ts` 도 앵커를 만지지 않는다 — 묶기·풀기는 상자만 옮긴다', () => {
    const text = stripComments(read('..', 'group', 'groupOps.ts'));
    expect(text).not.toContain('anchors');
  });
});

// --- AC-21 · AC-22: 경로 꼭지점 -------------------------------------------

describe('경로에서는 꼭지점에 붙는다 (AC-21)', () => {
  it('꼭지점 근처를 누르면 저장 좌표가 그 명령의 좌표와 **정확히** 같다', () => {
    for (const [i, vertex] of STAR_VERTICES.entries()) {
      const near = drawnAt(STAR, vertex);
      const node = addAnchorAt(STAR, near, `v${i}`);
      expect(node.anchors?.[0], `v${i}`).toEqual({ id: `v${i}`, x: vertex.x, y: vertex.y });
    }
  });

  it('허용 오차 밖이면 붙지 않는다 — 누른 자리를 그대로 저장한다', () => {
    // 꼭대기 꼭지점 (5000,0) 에서 오차의 세 배만큼 가로로 벗어난다. 다른 꼭지점은 훨씬
    // 멀리 있으므로 붙을 후보가 없고, 저장 좌표는 누른 자리 **그대로**여야 한다.
    const vertex = STAR_VERTICES[0]!;
    const off = { x: vertex.x + ANCHOR_SNAP_LOCAL * 3, y: vertex.y };
    const node = addAnchorAt(STAR, drawnAt(STAR, off), 'a');
    expect(node.anchors?.[0]).toEqual({ id: 'a', x: off.x, y: off.y });
  });

  it('오차 **안**이면 붙는다 — 위 시험이 무조건 초록이 아님을 못박는다', () => {
    const vertex = STAR_VERTICES[0]!;
    const near = { x: vertex.x + ANCHOR_SNAP_LOCAL / 2, y: vertex.y };
    const node = addAnchorAt(STAR, drawnAt(STAR, near), 'a');
    expect(node.anchors?.[0]).toEqual({ id: 'a', x: vertex.x, y: vertex.y });
  });

  it('제어점에는 붙지 않는다 — 곡선 위의 점이 아니다', () => {
    const curve: PathElement = {
      ...STAR,
      path: [
        { c: 'M', x: 0, y: 0 },
        { c: 'C', x1: 2000, y1: 9000, x2: 8000, y2: 9000, x: 10000, y: 0 },
      ],
    };
    // 제어점 (2000,9000) 에서 오차 **안**으로 살짝 벗어난 자리를 누른다. 제어점을 후보로
    // 보았다면 저장 좌표가 (2000,9000) 으로 당겨진다. 곡선 위의 두 점(0,0)·(10000,0)은
    // 오차 밖이므로 붙을 것이 없고, 누른 자리가 그대로 남아야 한다.
    const off = { x: 2000 + ANCHOR_SNAP_LOCAL / 4, y: 9000 };
    const node = addAnchorAt(curve, drawnAt(curve, off), 'a');
    expect(node.anchors?.[0]).toEqual({ id: 'a', x: off.x, y: off.y });
    expect(node.anchors?.[0]?.x).not.toBe(2000);
  });

  it('곡선 명령의 **끝점**에는 붙는다 — `C` 가 통째로 빠지지 않았다', () => {
    const curve: PathElement = {
      ...STAR,
      path: [
        { c: 'M', x: 0, y: 0 },
        { c: 'C', x1: 2000, y1: 9000, x2: 8000, y2: 9000, x: 10000, y: 0 },
      ],
    };
    const off = { x: 10000 - ANCHOR_SNAP_LOCAL / 2, y: 0 };
    expect(addAnchorAt(curve, drawnAt(curve, off), 'a').anchors?.[0]).toEqual({
      id: 'a',
      x: 10000,
      y: 0,
    });
  });

  it('상자형이어도 경로가 아니면 붙임이 없다', () => {
    const node = addAnchorAt(BOX, { x: 100, y: 100 }, 'a');
    expect(node.anchors?.[0]).toEqual({ id: 'a', x: 0, y: 0 });
  });
});

describe('늘려도 꼭지점에 남는다 (AC-22)', () => {
  // ## 재어 본 어긋남과 그 출처
  //
  // 처음 구현은 되돌림에 `toAbsolutePointRounded` 를 썼다(004 가 그룹 풀기에 지은 함수).
  // 저장되는 기하에는 옳은 정수화지만 **앵커는 파생값**이라 담을 곳의 제약이 없고, 그
  // 정수화가 그대로 어긋남이 되었다. 재어 본 값:
  //
  //   | 되돌림                      | 축마다 최대 어긋남 | 화면(2 px/단위) |
  //   | --------------------------- | ------------------ | --------------- |
  //   | `toAbsolutePointRounded`    | **0.5** 캔버스 단위 | 1 px            |
  //   | `toAbsolutePointExact`      | **5.7e-14**        | 사실상 0        |
  //
  // 즉 어긋남은 **전부 정수화였고 산술은 처음부터 정확했다.** 그래서 011 은 정수화 없는
  // 짝(`toAbsolutePointExact`)을 `groupCoords.ts` 에 세우고 앵커를 그쪽으로 옮겼다 —
  // 산술을 새로 적은 것이 아니라 **반올림을 쓰는 쪽의 성질로 되돌린 것**이다.
  //
  // 남은 잔차는 부동소수 꼬리뿐이다. 두 값이 **다른 셈 순서**를 지나기 때문이며
  // (앵커는 로컬→캔버스상자, 그려지는 자리는 로컬→px→캔버스), 그 차이는 ulp 단위다.
  // 허용치를 잰 값의 열 몇 배에 두는 것이 요점이다 — `toBeCloseTo(…, 9)` 같은 관용
  // 허용치는 실제 오차보다 **네다섯 자릿수** 헐거워서, 정수화가 되살아나도 통과한다.
  const FLOAT_TAIL_BOUND = 1e-12;

  const sizes: BoxGeometry[] = [
    { x: 100, y: 100, w: 400, h: 400 },
    { x: 100, y: 100, w: 437, h: 311 },
    { x: 37, y: 91, w: 163, h: 289 },
    { x: 3, y: 7, w: 997, h: 719 },
  ];

  /** 열 꼭지점 전부에 앵커를 붙인 뒤 주어진 크기로 늘린 노드. */
  function starWithAnchorsResizedTo(geometry: BoxGeometry): OutlinedNode {
    let node: OutlinedNode = STAR;
    STAR_VERTICES.forEach((vertex, i) => {
      node = addAnchorAt(node, drawnAt(STAR, vertex), `v${i}`);
    });
    return resizeTo(node, geometry);
  }

  it.each(sizes.map((s) => [JSON.stringify(s), s] as const))(
    '%s 로 늘려도 앵커가 그려지는 꼭지점 위에 남는다',
    (_label, geometry) => {
      const grown = starWithAnchorsResizedTo(geometry);
      const points = anchorPoints(grown, PROJ, NO_WIDTHS);

      let compared = 0;
      STAR_VERTICES.forEach((vertex, i) => {
        const got = points.get(`v${i}`);
        // 앵커가 하나라도 풀리지 않으면 아래 비교가 조용히 건너뛰어진다.
        expect(got, `v${i} 가 풀렸다`).toBeDefined();
        const drawn = drawnAt(grown, vertex);
        expect(Math.abs(got!.x - drawn.x), `v${i}.x`).toBeLessThan(FLOAT_TAIL_BOUND);
        expect(Math.abs(got!.y - drawn.y), `v${i}.y`).toBeLessThan(FLOAT_TAIL_BOUND);
        compared += 1;
      });
      // 헛돌지 않는다 ① — 실제로 열 자리를 다 견주었다.
      expect(compared).toBe(10);
      expect(STAR_VERTICES).toHaveLength(10);
    },
  );

  it('허용치가 정수화를 **가려내는** 폭이다 — 옛 되돌림이었다면 빨개진다', () => {
    // 헛돌지 않는다 ② — 위 단언이 "어차피 늘 0 이라 초록" 이 아님을 보인다. 같은 자료에
    // 정수화 쪽을 끼우면 어긋남이 허용치보다 **열 자릿수** 넘게 커진다. 그러니 이 허용치는
    // 고친 것과 고치지 않은 것을 실제로 가른다.
    //
    // 시험 파일이라 AC-32 가드(제품 파일만 훑는다)에 걸리지 않고 옛 짝을 불러 볼 수 있다.
    const geometry = { x: 37, y: 91, w: 163, h: 289 };
    const grown = starWithAnchorsResizedTo(geometry);
    const canvasBox = unprojectBox(outlineBox(grown, PROJ, NO_WIDTHS), PROJ);

    let worstRounded = 0;
    let worstExact = 0;
    STAR_VERTICES.forEach((vertex, i) => {
      const anchor = grown.anchors!.find((a) => a.id === `v${i}`)!;
      const drawn = drawnAt(grown, vertex);
      const rounded = toAbsolutePointRounded({ x: anchor.x, y: anchor.y }, canvasBox);
      const exact = toAbsolutePointExact({ x: anchor.x, y: anchor.y }, canvasBox);
      worstRounded = Math.max(
        worstRounded,
        Math.abs(rounded.x - drawn.x),
        Math.abs(rounded.y - drawn.y),
      );
      worstExact = Math.max(worstExact, Math.abs(exact.x - drawn.x), Math.abs(exact.y - drawn.y));
    });

    // 잰 값: 정수화 쪽 0.5 단위(화면 1 px), 정확 쪽 5.7e-14.
    expect(worstRounded).toBeGreaterThan(FLOAT_TAIL_BOUND * 1e6);
    expect(worstRounded).toBeLessThanOrEqual(0.5);
    expect(worstExact).toBeLessThan(FLOAT_TAIL_BOUND);
    // 화면에서 잰 옛 어긋남 — 반 픽셀 이하가 아니었다는 사실을 값으로 남긴다.
    expect(worstRounded * PX_PER_UNIT).toBeGreaterThan(1 / PX_PER_UNIT);
  });

  it('앵커는 **자리**를 내지 기하를 내지 않는다 — 정수로 죄지 않는다', () => {
    // 고정 아홉이 이미 갖는 성질이다(M3: "정수화는 쓰는 쪽의 몫"). 임의 앵커가 그 규율에서
    // 벗어나면 같은 지도 위에 두 종류의 자리가 생긴다.
    const grown = starWithAnchorsResizedTo({ x: 3, y: 7, w: 997, h: 719 });
    const points = anchorPoints(grown, PROJ, NO_WIDTHS);
    const fractional = STAR_VERTICES.map((_v, i) => points.get(`v${i}`)!).filter(
      (p) => !Number.isInteger(p.x) || !Number.isInteger(p.y),
    );
    expect(fractional.length).toBeGreaterThan(0);
  });
});

// --- AC-24: 상자형에만 선다 -------------------------------------------------

describe('선·문구에는 더할 수 없다 (AC-24 · A11)', () => {
  it.each([
    ['line', LINE],
    ['text', TEXT],
  ] as const)('%s 에 더하면 아무 일도 없다', (_name, node) => {
    const after = addAnchorAt(node, { x: 150, y: 150 }, 'a');
    expect(after).toBe(node);
    expect(after.anchors).toBeUndefined();
  });

  it.each([
    ['rect', BOX],
    ['path', STAR],
    ['group', GROUP],
  ] as const)('%s 에는 선다', (_name, node) => {
    expect(addAnchorAt(node, { x: 150, y: 150 }, 'a').anchors).toHaveLength(1);
  });

  it('손으로 적어 넣은 `anchors` 도 선·문구에서는 읽히지 않는다', () => {
    // 쓰기 경로만 막으면 config 를 손으로 고친 값이 **한 번은 그려지고** 다음 읽기에서
    // 사라진다. 파서가 같은 금지를 들고 있어야 그 상태가 아예 서지 않는다.
    const nodes = parseNodes([
      { id: 'l', kind: 'line', geometry: { x1: 0, y1: 0, x2: 9, y2: 9 }, anchors: [{ id: 'a', x: 1, y: 2 }] },
      { id: 't', kind: 'text', geometry: { x: 0, y: 0 }, anchors: [{ id: 'a', x: 1, y: 2 }] },
      { id: 'r', kind: 'rect', geometry: { x: 0, y: 0, w: 9, h: 9 }, anchors: [{ id: 'a', x: 1, y: 2 }] },
      { id: 'g', kind: 'group', geometry: { x: 0, y: 0, w: 9, h: 9 }, anchors: [{ id: 'a', x: 1, y: 2 }] },
    ]);
    // 연결선에는 `anchors` 가 없다(011 M4) — 이 시험이 재는 넷은 전부 상자를 가진 쪽이다.
    const byId = new Map(nodes.filter((n) => !isConnector(n)).map((n) => [n.id, n]));
    expect(byId.get('l')?.anchors).toBeUndefined();
    expect(byId.get('t')?.anchors).toBeUndefined();
    expect(byId.get('r')?.anchors).toEqual([{ id: 'a', x: 1, y: 2 }]);
    expect(byId.get('g')?.anchors).toEqual([{ id: 'a', x: 1, y: 2 }]);
  });

  it('파서와 쓰기 경로가 **같은 넷**을 상자형으로 본다', () => {
    // A11 이 두 곳에 적혀 있으므로(파서의 갈래 · `addAnchorAt` 의 상자 판정) 둘이 갈릴 수
    // 있다. 갈리면 "더해지는데 저장되지 않는" 혹은 그 반대의 종류가 생긴다.
    const samples: ReadonlyArray<readonly [string, OutlinedNode, Record<string, unknown>]> = [
      ['rect', BOX, { id: 'x', kind: 'rect', geometry: { x: 0, y: 0, w: 9, h: 9 } }],
      [
        'ellipse',
        { ...BOX, kind: 'ellipse' } as CanvasElement,
        { id: 'x', kind: 'ellipse', geometry: { x: 0, y: 0, w: 9, h: 9 } },
      ],
      ['path', STAR, { id: 'x', kind: 'path', geometry: { x: 0, y: 0, w: 9, h: 9 } }],
      ['line', LINE, { id: 'x', kind: 'line', geometry: { x1: 0, y1: 0, x2: 9, y2: 9 } }],
      ['text', TEXT, { id: 'x', kind: 'text', geometry: { x: 0, y: 0 } }],
      ['group', GROUP, { id: 'x', kind: 'group', geometry: { x: 0, y: 0, w: 9, h: 9 } }],
    ];
    for (const [name, node, raw] of samples) {
      const writeAccepts = addAnchorAt(node, { x: 150, y: 150 }, 'a').anchors !== undefined;
      const parseKeeps = parsedAnchors([{ ...raw, anchors: [{ id: 'a', x: 1, y: 2 }] }]) !== undefined;
      expect(`${name}:${writeAccepts}`, name).toBe(`${name}:${parseKeeps}`);
    }
  });
});

// --- AC-25: 쓰지 않으면 키가 없다 -------------------------------------------

describe('쓰지 않으면 키가 생기지 않는다 (AC-25)', () => {
  it('앵커를 한 번도 더하지 않은 요소의 왕복에 `anchors` 키가 없다', () => {
    const raw = { id: 'r', kind: 'rect', geometry: { x: 0, y: 0, w: 9, h: 9 }, style: {} };
    const round = JSON.parse(JSON.stringify(parseNodes([raw])[0]));
    expect('anchors' in round).toBe(false);
  });

  it('빈 배열도 키를 남기지 않는다', () => {
    const parsed = parseNodes([
      { id: 'r', kind: 'rect', geometry: { x: 0, y: 0, w: 9, h: 9 }, anchors: [] },
    ])[0]!;
    expect('anchors' in parsed).toBe(false);
  });

  it('마지막 하나를 빼면 키가 사라진다', () => {
    // `[]` 를 남기면 "쓴 적 없음" 과 "다 지웠음" 이 구분되지 않고, 없던 키가 config 에 생긴다.
    const node = addAnchorAt(BOX, { x: 200, y: 200 }, 'a');
    const emptied = removeAnchor(node, 'a');
    expect('anchors' in emptied).toBe(false);
    expect(emptied).toEqual(BOX);
  });

  it('둘 중 하나만 빼면 나머지가 남는다', () => {
    const node = removeAnchor(
      addAnchorAt(addAnchorAt(BOX, { x: 150, y: 150 }, 'a'), { x: 250, y: 250 }, 'b'),
      'a',
    );
    expect(node.anchors).toEqual([{ id: 'b', x: 7500, y: 7500 }]);
  });

  it('없는 id 를 빼면 받은 노드 그대로다', () => {
    const node = addAnchorAt(BOX, { x: 200, y: 200 }, 'a');
    expect(removeAnchor(node, 'zzz')).toBe(node);
    expect(removeAnchor(BOX, 'a')).toBe(BOX);
  });
});

// --- AC-29 · AC-30: 파서의 관용 ---------------------------------------------

describe('손상된 앵커 항목만 버린다 (AC-29)', () => {
  const parseRectAnchors = (anchors: unknown): unknown =>
    parsedAnchors([{ id: 'r', kind: 'rect', geometry: { x: 0, y: 0, w: 9, h: 9 }, anchors }]);

  it('좌표가 손상된 항목만 빠지고 나머지는 남는다', () => {
    expect(
      parseRectAnchors([
        { id: 'ok1', x: 10, y: 20 },
        { id: 'bad-x', x: 'zzz', y: 20 },
        { id: 'bad-nan', x: Number.NaN, y: 20 },
        { id: 'bad-inf', x: 10, y: Number.POSITIVE_INFINITY },
        { id: 'ok2', x: 30, y: 40 },
      ]),
    ).toEqual([
      { id: 'ok1', x: 10, y: 20 },
      { id: 'ok2', x: 30, y: 40 },
    ]);
  });

  it('정체성이 없는 항목이 빠진다', () => {
    expect(
      parseRectAnchors([{ x: 1, y: 2 }, { id: '', x: 1, y: 2 }, { id: 'ok', x: 1, y: 2 }]),
    ).toEqual([{ id: 'ok', x: 1, y: 2 }]);
  });

  it('항목이 객체가 아니어도 예외가 없다', () => {
    expect(parseRectAnchors([null, 3, 'x', [], { id: 'ok', x: 1, y: 2 }])).toEqual([
      { id: 'ok', x: 1, y: 2 },
    ]);
  });

  it('배열이 아니면 미지정이다 — 요소를 버리지 않는다', () => {
    for (const bad of [null, 3, 'x', {}, true]) {
      expect(parseRectAnchors(bad), JSON.stringify(bad)).toBeUndefined();
    }
    expect(
      parseNodes([{ id: 'r', kind: 'rect', geometry: { x: 0, y: 0, w: 9, h: 9 }, anchors: 3 }]),
    ).toHaveLength(1);
  });

  it('소수 좌표는 정수로 반올림한다 — 001 의 좌표 규율 그대로다', () => {
    expect(parseRectAnchors([{ id: 'a', x: 10.4, y: 20.6 }])).toEqual([{ id: 'a', x: 10, y: 21 }]);
  });
});

describe('앵커 id 중복은 먼저 온 것이 이긴다 (AC-30)', () => {
  it('같은 id 가 둘이면 하나만 남는다', () => {
    const anchors = parsedAnchors([
      {
        id: 'r',
        kind: 'rect',
        geometry: { x: 0, y: 0, w: 9, h: 9 },
        anchors: [
          { id: 'a', x: 1, y: 2 },
          { id: 'a', x: 99, y: 99 },
        ],
      },
    ]);
    expect(anchors).toEqual([{ id: 'a', x: 1, y: 2 }]);
  });

  it('쓰기 경로도 같은 규율이다 — 이미 있는 id 는 더해지지 않는다', () => {
    const once = addAnchorAt(BOX, { x: 150, y: 150 }, 'a');
    expect(addAnchorAt(once, { x: 250, y: 250 }, 'a')).toBe(once);
  });

  it('정체성이 없는 id 로는 더해지지 않는다', () => {
    expect(addAnchorAt(BOX, { x: 150, y: 150 }, '   ')).toBe(BOX);
    expect(addAnchorAt(BOX, { x: 150, y: 150 }, '')).toBe(BOX);
  });
});

// --- AC-33: 고정과 임의가 한 상자에서 나온다 --------------------------------

describe('고정 아홉과 임의 앵커가 같은 상자에서 나온다 (AC-33)', () => {
  it('임의 앵커가 고정 아홉 뒤에 붙는다 — 아홉은 그대로다', () => {
    const node = addAnchorAt(BOX, { x: 200, y: 200 }, 'a');
    const points = anchorPoints(node, PROJ, NO_WIDTHS);
    expect(points.size).toBe(10);
    expect([...points.keys()].slice(9)).toEqual(['a']);
    // 한가운데 앵커는 중심(`c`)과 **같은 자리**에서 나온다. 상자가 둘이면 갈라진다.
    expect(points.get('a')).toEqual(points.get('c'));
  });

  it('고정 이름과 부딪히면 고정이 이긴다', () => {
    const node = addAnchorAt(BOX, { x: 100, y: 100 }, 'c');
    const points = anchorPoints(node, PROJ, NO_WIDTHS);
    expect(points.size).toBe(9);
    expect(points.get('c')).toEqual({ x: 200, y: 200 });
  });

  it('그룹도 제 상자로 임의 앵커를 낸다', () => {
    const node = addAnchorAt(GROUP, { x: 150, y: 150 }, 'a');
    expect(node.anchors).toEqual([{ id: 'a', x: 2500, y: 2500 }]);
    expect(anchorPoints(node, PROJ, NO_WIDTHS).get('a')).toEqual({ x: 150, y: 150 });
  });

  it('퇴화 상자에서 임의 앵커가 고정 아홉과 **같이** 접힌다', () => {
    // 여기서만 상자를 넓히면 퇴화한 도형에서 임의 앵커만 윤곽 밖으로 벌어진다.
    const flat: RectElement = { ...BOX, geometry: { x: 100, y: 100, w: 200, h: 0 } };
    const node = addAnchorAt(flat, { x: 200, y: 100 }, 'a');
    const points = anchorPoints(node, PROJ, NO_WIDTHS);
    expect(points.get('a')?.y).toBe(points.get('c')?.y);
    expect(Number.isFinite(points.get('a')?.x)).toBe(true);
  });
});

// --- 순수성 ----------------------------------------------------------------

describe('더하기·빼기가 순수하다', () => {
  it('받은 노드를 한 글자도 건드리지 않는다', () => {
    const before = JSON.stringify(BOX);
    const added = addAnchorAt(BOX, { x: 200, y: 200 }, 'a');
    removeAnchor(added, 'a');
    expect(JSON.stringify(BOX)).toBe(before);
  });

  it('앞서 저장된 앵커 배열을 제자리에서 늘리지 않는다', () => {
    const one = addAnchorAt(BOX, { x: 150, y: 150 }, 'a');
    const two = addAnchorAt(one, { x: 250, y: 250 }, 'b');
    expect(one.anchors).toHaveLength(1);
    expect(two.anchors).toHaveLength(2);
    expect(two.anchors).not.toBe(one.anchors);
  });
});

// --- 소스 읽기 도우미 -------------------------------------------------------

//
// 주석은 걷어내고 읽는다: 산문에 적힌 낱말이 가드를 헛되이 울리면 다음 사람은 주석을
// 고쳐 지나가고, 그때 가드는 이미 죽은 것이다(009 가드가 세운 규율 그대로).

function stripComments(text: string): string {
  return text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:])\/\/.*$/gm, '$1');
}

function read(...parts: string[]): string {
  return fs.readFileSync(path.join(__dirname, ...parts), 'utf8');
}

function anchorTypesSource(): string {
  return read('anchorTypes.ts');
}
