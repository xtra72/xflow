// 끝점 해석을 **값으로** 못박는다 (SPEC-CANVAS-011 M5 · AC-40 ~ AC-45).
//
// ## 무엇을 재는가
//
// 011 의 전부는 "끝이 좌표가 아니라 **참조**" 라는 한 문장이다. 그 문장이 참이면 요소를
// 옮기거나 늘렸을 때 선이 **코드 한 줄 없이** 따라오고, 거짓이면 선이 옛 자리에 남는다.
// 뒤의 실패는 예외도 경고도 내지 않고 **화면에서만** 드러나므로, 여기서 좌표로 붙든다.
//
// ## 두 투영을 쓰는 까닭
//
// `PROJ` 는 캔버스 한 단위가 화면 2 px 인 깔끔한 축척이고 `ODD` 는 어느 축도 나누어
// 떨어지지 않는다. 앞의 것만 쓰면 px↔캔버스 왕복의 부동소수 꼬리가 **전부 0 으로** 나와
// 아래 허용치가 무엇을 가려내는지 알 수 없게 된다(§허용치 참조).
//
// ## 소비자 셋은 여기서 재지 않는다
//
// AC-45 의 "그리기 · 히트 · 손잡이가 모두 이 함수를 지난다" 는 그 셋이 선 뒤에야 잴 수
// 있다(M6 · M7 · M9). 소스 형상으로 잴 수 있는 절반 — **참조를 푸는 자리가 여기 하나뿐**
// 이다 — 은 `canvas011Guards.test.ts` 가 든다.
//
// @spec SPEC-CANVAS-011 REQ-03-a · REQ-08

import { describe, expect, it } from 'vitest';

import type { CanvasElement, PathElement, RectElement } from '../canvasConfig';
import type { CanvasPoint, CanvasProjection } from '../canvasGeometry';
import type { CanvasNode, GroupElement, OutlinedNode } from '../group/groupTypes';
import { addAnchorAt, anchorPoints, removeAnchor, FIXED_ANCHOR_IDS } from './anchors';
import {
  CONNECTOR_KIND,
  type ConnectorElement,
  type ConnectorEnd,
  type ConnectorRoute,
} from './connectorTypes';
import { resolveConnector } from './resolveConnector';

/** 캔버스 한 단위 = 화면 2 px. 나누어 떨어지는 축척. */
const PROJ: CanvasProjection = {
  stage: { width: 1000, height: 800 },
  canvas: { width: 500, height: 400 },
};

/** 어느 축도 나누어 떨어지지 않는 축척. 부동소수 꼬리가 실제로 나오는 자리다. */
const ODD: CanvasProjection = {
  stage: { width: 977, height: 613 },
  canvas: { width: 503, height: 397 },
};

const NO_WIDTHS: Readonly<Record<string, number>> = {};

// ## 허용치
//
// 잰 값: 아래 두 투영 · 세 상자에서 해석 결과와 **직접 적은 기댓값**(`x + w` 꼴)의
// 어긋남이 최대 **5.68e-14** 였다. 같은 셈 순서를 지나는 비교(해석 결과 대 `anchorPoints`
// 의 지도)는 **정확히 0** 이다 — 둘 다 같은 함수에서 나오기 때문이다.
//
// 그래서 허용치는 잰 값의 **열몇 배**인 1e-12 이고, `toBeCloseTo(…, 9)` 같은 관용 허용치를
// 쓰지 않는다. 그쪽은 실제 오차보다 네다섯 자릿수 헐거워서 정수화가 되살아나도 통과한다
// (`customAnchors.test.ts` 가 같은 자리에서 같은 값을 쟀다).
const FLOAT_TAIL_BOUND = 1e-12;

/**
 * 목록의 `i` 번째 점. 없으면 **그 자리에서** 이름을 대고 멎는다.
 *
 * 맨 `!` 로 집으면 해석이 끊겼거나 목록이 짧아졌을 때 그 다음 줄의 산술이 `undefined` 를
 * 지나며 엉뚱한 자리에서 울고, 무엇이 없었는지가 메시지에 남지 않는다.
 */
function at(points: readonly CanvasPoint[] | undefined, i: number): CanvasPoint {
  expect(points, '해석이 끊겼다').toBeDefined();
  const point = points?.[i];
  expect(point, `${i} 번째 점이 없다`).toBeDefined();
  return point!;
}

function expectNear(got: CanvasPoint, want: CanvasPoint, label: string): void {
  expect(Math.abs(got.x - want.x), `${label}.x`).toBeLessThan(FLOAT_TAIL_BOUND);
  expect(Math.abs(got.y - want.y), `${label}.y`).toBeLessThan(FLOAT_TAIL_BOUND);
}

// --- 무대 ------------------------------------------------------------------

const A: RectElement = {
  id: 'el-1',
  kind: 'rect',
  geometry: { x: 100, y: 100, w: 200, h: 200 },
  style: {},
};

const B: RectElement = {
  id: 'el-2',
  kind: 'rect',
  geometry: { x: 400, y: 300, w: 100, h: 100 },
  style: {},
};

const PART: CanvasElement = {
  id: 'body',
  kind: 'rect',
  geometry: { x: 0, y: 0, w: 5000, h: 5000 },
  style: {},
};

const GROUP: GroupElement = {
  id: 'grp-1',
  kind: 'group',
  geometry: { x: 600, y: 100, w: 200, h: 200 },
  parts: [PART],
};

const OTHER_LINE: ConnectorElement = {
  id: 'c-other',
  kind: CONNECTOR_KIND,
  from: { el: 'el-1', a: 'e' },
  to: { el: 'el-2', a: 'w' },
  route: 'straight',
};

/** 끝 둘만 갈아 끼우는 연결선 하나. `route` 는 해석에 닿지 않는다(그리기만 가른다). */
function link(from: ConnectorEnd, to: ConnectorEnd, route: ConnectorRoute = 'straight'): ConnectorElement {
  return { id: 'c', kind: CONNECTOR_KIND, from, to, route };
}

const SCENE: readonly CanvasNode[] = [A, B, GROUP, OTHER_LINE];

/** 노드 하나를 옮긴 무대. 저장 좌표를 손으로 고치는, 손이 하는 그 일이다. */
function moved(node: OutlinedNode, dx: number, dy: number): OutlinedNode {
  const geo = node.geometry as { x: number; y: number };
  return { ...node, geometry: { ...node.geometry, x: geo.x + dx, y: geo.y + dy } } as OutlinedNode;
}

/** 노드 하나의 상자를 갈아 끼운 무대. 크기 조절이 닿는 그 필드다. */
function sized(node: OutlinedNode, w: number, h: number): OutlinedNode {
  return { ...node, geometry: { ...node.geometry, w, h } } as OutlinedNode;
}

function replace(nodes: readonly CanvasNode[], next: CanvasNode): readonly CanvasNode[] {
  return nodes.map((node) => (node.id === next.id ? next : node));
}

// --- AC-40: 참조가 앵커 자리로 풀린다 ---------------------------------------

describe('참조가 앵커 자리로 풀린다 (AC-40)', () => {
  it('`{ el: \'el-1\', a: \'e\' }` 가 `el-1` 의 **동쪽** 자리다', () => {
    // (100,100)-(300,300) 의 동쪽 변 가운데 — 직접 적은 기댓값이라 투영 왕복을 실제로 잰다.
    const points = resolveConnector(link({ el: 'el-1', a: 'e' }, { x: 0, y: 0 }), SCENE, PROJ, NO_WIDTHS);
    expect(points).toBeDefined();
    expect(points).toHaveLength(2);
    expectNear(at(points, 0), { x: 300, y: 200 }, '동쪽');
  });

  it('아홉 자리가 **전부** `anchorPoints` 의 그 자리다 — 한 자리도 어긋나지 않는다', () => {
    const map = anchorPoints(A, ODD, NO_WIDTHS);
    let compared = 0;
    for (const id of FIXED_ANCHOR_IDS) {
      const points = resolveConnector(link({ el: 'el-1', a: id }, { x: 0, y: 0 }), SCENE, ODD, NO_WIDTHS);
      expect(points, id).toBeDefined();
      // 같은 함수에서 나오므로 **정확히** 같다. 여기서 허용치를 쓰면 두 번째 산술이
      // 자라기 시작해도 초록으로 지나간다.
      expect(points![0], id).toEqual(map.get(id));
      compared += 1;
    }
    // 헛돌지 않는다 — 실제로 아홉을 다 견주었다.
    expect(compared).toBe(9);
    expect(FIXED_ANCHOR_IDS).toHaveLength(9);
  });

  it('임의 앵커 이름도 같은 길로 풀린다', () => {
    const withAnchor = addAnchorAt(A, { x: 150, y: 120 }, 'a1');
    const scene = replace(SCENE, withAnchor);
    const points = resolveConnector(link({ el: 'el-1', a: 'a1' }, { x: 0, y: 0 }), scene, PROJ, NO_WIDTHS);
    expect(points![0]).toEqual(anchorPoints(withAnchor, PROJ, NO_WIDTHS).get('a1'));
  });

  it('그룹도 제 상자로 자리를 낸다 — 상자를 가진 노드면 종류를 가리지 않는다', () => {
    const points = resolveConnector(link({ el: 'grp-1', a: 'w' }, { x: 0, y: 0 }), SCENE, PROJ, NO_WIDTHS);
    expect(points).toBeDefined();
    expectNear(at(points, 0), { x: 600, y: 200 }, '그룹 서쪽');
  });
});

// --- AC-41: 요소를 옮기면 끝점이 따라간다 -----------------------------------

describe('요소가 움직이면 끝점이 따라간다 (AC-41 · REQ-03-a)', () => {
  const connector = link({ el: 'el-1', a: 'e' }, { el: 'el-2', a: 'w' });

  it('옮긴 만큼 **그대로** 따라간다', () => {
    const before = resolveConnector(connector, SCENE, PROJ, NO_WIDTHS)!;
    const after = resolveConnector(connector, replace(SCENE, moved(A, 50, -30)), PROJ, NO_WIDTHS)!;
    expectNear(at(after, 0), { x: at(before, 0).x + 50, y: at(before, 0).y - 30 }, '따라온 끝');
    // 다른 쪽 끝은 제 요소가 가만히 있으므로 한 자리도 움직이지 않는다.
    expect(after[1]).toEqual(before[1]);
  });

  it('**크기**를 바꿔도 따라간다 — 자리는 상자에서 파생되지 저장된 값이 아니다', () => {
    // 옮기기만 재면 "끝점에 좌표를 적어 두고 델타를 더한다" 는 구현도 초록이다.
    // 늘리기는 그 구현을 가려낸다 — 상자의 **변**이 옮겨 가기 때문이다.
    const grown = sized(A, 440, 260);
    const after = resolveConnector(connector, replace(SCENE, grown), ODD, NO_WIDTHS)!;
    expectNear(at(after, 0), { x: 100 + 440, y: 100 + 260 / 2 }, '늘어난 상자의 동쪽');
  });

  it('연결선에는 그 좌표가 **적히지 않는다** — 해석이 저술을 건드리지 않는다', () => {
    const snapshot = JSON.stringify(connector);
    resolveConnector(connector, SCENE, PROJ, NO_WIDTHS);
    resolveConnector(connector, replace(SCENE, moved(A, 50, -30)), PROJ, NO_WIDTHS);
    expect(JSON.stringify(connector)).toBe(snapshot);
  });
});

// --- AC-42: 자유 끝점은 제자리다 --------------------------------------------

describe('자유 끝점은 제자리다 (AC-42)', () => {
  it('다른 요소를 옮겨도 좌표가 **한 자리도** 바뀌지 않는다', () => {
    const connector = link({ el: 'el-1', a: 'e' }, { x: 470, y: 330 });
    const before = resolveConnector(connector, SCENE, PROJ, NO_WIDTHS)!;
    const after = resolveConnector(connector, replace(SCENE, moved(A, 50, -30)), PROJ, NO_WIDTHS)!;
    expect(before[1]).toEqual({ x: 470, y: 330 });
    expect(after[1]).toEqual({ x: 470, y: 330 });
    // 헛돌지 않는다 — 같은 호출에서 붙은 끝은 실제로 움직였다.
    expect(after[0]).not.toEqual(before[0]);
  });

  it('투영이 달라져도 자유 끝점은 그 값 그대로다 — 이미 캔버스 단위이기 때문이다', () => {
    const connector = link({ x: 11, y: 13 }, { x: 470, y: 330 });
    expect(resolveConnector(connector, SCENE, PROJ, NO_WIDTHS)).toEqual([
      { x: 11, y: 13 },
      { x: 470, y: 330 },
    ]);
    expect(resolveConnector(connector, SCENE, ODD, NO_WIDTHS)).toEqual([
      { x: 11, y: 13 },
      { x: 470, y: 330 },
    ]);
  });
});

// --- AC-43 · AC-44 · A6 · REQ-02'-e: 끊긴 연결 -------------------------------

describe('끊긴 연결은 `undefined` 다 (REQ-08)', () => {
  it('없는 요소를 가리키면 `undefined` 이고 **예외가 없다** (AC-43)', () => {
    const connector = link({ el: 'gone', a: 'e' }, { el: 'el-2', a: 'w' });
    expect(() => resolveConnector(connector, SCENE, PROJ, NO_WIDTHS)).not.toThrow();
    expect(resolveConnector(connector, SCENE, PROJ, NO_WIDTHS)).toBeUndefined();
  });

  it('**끝나는 쪽**이 끊겨도 똑같이 `undefined` 다 — 반쪽 목록을 내지 않는다', () => {
    // 반쪽 목록은 소비자가 그릴 수 있는 형상이라, 한쪽만 붙은 선이 화면에 남는다.
    const connector = link({ el: 'el-1', a: 'e' }, { el: 'gone', a: 'w' });
    expect(resolveConnector(connector, SCENE, PROJ, NO_WIDTHS)).toBeUndefined();
  });

  it('빈 무대에서도 `undefined` 다 — 목록이 비어 있는 것이 예외가 아니다', () => {
    expect(resolveConnector(link({ el: 'el-1', a: 'e' }, { x: 0, y: 0 }), [], PROJ, NO_WIDTHS)).toBeUndefined();
  });

  it('부품 복합 키는 `undefined` 다 (AC-44 · A3)', () => {
    const connector = link({ el: 'grp-1/body', a: 'e' }, { x: 0, y: 0 });
    expect(resolveConnector(connector, SCENE, PROJ, NO_WIDTHS)).toBeUndefined();
  });

  it('그 그룹 **자신**은 풀린다 — 위 `undefined` 가 "그룹을 못 푼다" 가 아니다', () => {
    // 헛돌지 않는다: 복합 키가 막히는 까닭은 최상위에 그 id 가 없기 때문이지
    // 그룹이 앵커를 못 내기 때문이 아니다.
    expect(resolveConnector(link({ el: 'grp-1', a: 'e' }, { x: 0, y: 0 }), SCENE, PROJ, NO_WIDTHS)).toBeDefined();
    // 부품 id 가 최상위에 없다는 사실 자체도 못박는다.
    expect(SCENE.some((node) => node.id === 'body')).toBe(false);
  });

  it('연결선을 가리키면 `undefined` 다 (A6 · REQ-03-b)', () => {
    const connector = link({ el: 'c-other', a: 'e' }, { x: 0, y: 0 });
    expect(resolveConnector(connector, SCENE, PROJ, NO_WIDTHS)).toBeUndefined();
    // 헛돌지 않는다 — 그 id 는 무대에 **있다**. 걸러낸 것은 부재가 아니라 종류다.
    expect(SCENE.some((node) => node.id === 'c-other')).toBe(true);
  });

  it('요소는 있으나 그 이름의 자리가 없으면 `undefined` 다 (REQ-02\'-e)', () => {
    const withAnchor = addAnchorAt(A, { x: 150, y: 120 }, 'a1');
    const connector = link({ el: 'el-1', a: 'a1' }, { x: 0, y: 0 });
    expect(resolveConnector(connector, replace(SCENE, withAnchor), PROJ, NO_WIDTHS)).toBeDefined();

    const stripped = removeAnchor(withAnchor, 'a1');
    expect(resolveConnector(connector, replace(SCENE, stripped), PROJ, NO_WIDTHS)).toBeUndefined();
  });

  it('모르는 이름이 중심으로 **떨어지지 않는다** — 그어 둔 선이 조용히 옮겨 앉지 않는다', () => {
    const points = resolveConnector(link({ el: 'el-1', a: 'nope' }, { x: 0, y: 0 }), SCENE, PROJ, NO_WIDTHS);
    expect(points).toBeUndefined();
    // 헛돌지 않는다 — 떨어질 만한 자리(중심)가 실제로 존재하는데도 쓰지 않았다.
    expect(anchorPoints(A, PROJ, NO_WIDTHS).get('c')).toBeDefined();
  });
});

// --- 두 끝이 같은 요소 --------------------------------------------------------

describe('두 끝이 **같은 요소**를 가리켜도 된다', () => {
  it('서쪽과 동쪽이 그 요소의 두 자리로 풀린다', () => {
    const points = resolveConnector(link({ el: 'el-1', a: 'w' }, { el: 'el-1', a: 'e' }), SCENE, ODD, NO_WIDTHS)!;
    const map = anchorPoints(A, ODD, NO_WIDTHS);
    expect(points).toHaveLength(2);
    expect(points[0]).toEqual(map.get('w'));
    expect(points[1]).toEqual(map.get('e'));
    // 차례가 뒤집히지 않는다 — 서쪽이 동쪽보다 왼쪽이다.
    expect(at(points, 0).x).toBeLessThan(at(points, 1).x);
  });

  it('같은 이름을 둘 다 가리키면 두 끝이 **같은 점**이다', () => {
    const points = resolveConnector(link({ el: 'el-1', a: 'c' }, { el: 'el-1', a: 'c' }), SCENE, ODD, NO_WIDTHS)!;
    expect(points[0]).toEqual(points[1]);
  });
});

// --- 중간점 ------------------------------------------------------------------

describe('중간점은 그대로 지나간다', () => {
  const ENDS: [ConnectorEnd, ConnectorEnd] = [{ el: 'el-1', a: 'e' }, { el: 'el-2', a: 'w' }];

  it('`points` 가 **없을 때**와 **빈 목록일 때**가 같은 두 점 목록이다', () => {
    const absent = link(...ENDS);
    const empty: ConnectorElement = { ...absent, points: [] };
    const a = resolveConnector(absent, SCENE, PROJ, NO_WIDTHS);
    const b = resolveConnector(empty, SCENE, PROJ, NO_WIDTHS);
    expect(a).toHaveLength(2);
    expect(b).toEqual(a);
  });

  it('찍은 차례 그대로 끝점 사이에 들어간다', () => {
    const mid = [
      { x: 320, y: 210 },
      { x: 360, y: 260 },
      { x: 380, y: 300 },
    ];
    const points = resolveConnector({ ...link(...ENDS), points: mid }, SCENE, PROJ, NO_WIDTHS)!;
    expect(points).toHaveLength(5);
    expect(points.slice(1, 4)).toEqual(mid);
    expectNear(at(points, 0), { x: 300, y: 200 }, '시작');
    expectNear(at(points, 4), { x: 400, y: 350 }, '끝');
  });

  it('`route` 는 해석에 닿지 않는다 — 넷이 **같은 점 목록**을 낸다', () => {
    const mid = [{ x: 350, y: 250 }];
    const routes: ConnectorRoute[] = ['straight', 'elbow', 'curve', 'free'];
    const lists = routes.map(
      (route) => resolveConnector({ ...link(ENDS[0], ENDS[1], route), points: mid }, SCENE, PROJ, NO_WIDTHS)!,
    );
    for (const list of lists) expect(list).toEqual(lists[0]);
    expect(lists).toHaveLength(4);
  });

  it('낸 점은 **사본**이다 — 소비자가 손잡이를 끌어도 저술이 제자리에서 바뀌지 않는다', () => {
    const mid = [{ x: 350, y: 250 }];
    const connector: ConnectorElement = { ...link(...ENDS), points: mid };
    const points = resolveConnector(connector, SCENE, PROJ, NO_WIDTHS)!;
    (points[1] as { x: number }).x = -999;
    expect(mid[0]).toEqual({ x: 350, y: 250 });
    expect(connector.points![0]).toEqual({ x: 350, y: 250 });
  });
});

// --- 무대가 실제로 쓰였는가 --------------------------------------------------

describe('그물이 성기지 않다', () => {
  it('무대에 네 종류가 다 있고 그중 셋만 상자를 갖는다', () => {
    expect(SCENE).toHaveLength(4);
    expect(SCENE.map((node) => node.kind)).toEqual(['rect', 'rect', 'group', CONNECTOR_KIND]);
  });

  it('경로 요소도 같은 길로 풀린다 — 종류별 갈래가 없다는 사실', () => {
    const star: PathElement = {
      id: 'p',
      kind: 'path',
      geometry: { x: 10, y: 20, w: 100, h: 60 },
      path: [{ c: 'M', x: 0, y: 0 }, { c: 'L', x: 5000, y: 5000 }, { c: 'Z' }],
      style: {},
    };
    const points = resolveConnector(link({ el: 'p', a: 'se' }, { x: 0, y: 0 }), [star], PROJ, NO_WIDTHS);
    expect(points).toBeDefined();
    expectNear(at(points, 0), { x: 110, y: 80 }, '경로 남동');
  });
});
