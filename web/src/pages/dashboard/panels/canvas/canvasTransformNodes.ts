// 고른 노드들을 뒤집고 돌린다 — 산술이 아니라 **배선**이다 (SPEC-CANVAS-013 M3).
//
// ## 변환은 두 반쪽이고, 그 둘은 쓰는 통로가 다르다 (§결정 2)
//
// `patchNodeGeometry` 는 이 저장소의 **유일한 기하 쓰기 통로**이고, 그 머리말이 무엇을
// 쓰지 **않는지**까지 적어 두었다:
//
//   > **`parts` 는 여기를 지나지 않는다**(가정 A17). 이 통로가 쓰는 것은 노드의 `geometry`
//   > 뿐이며, 부품의 저장 좌표를 함께 고치는 설계는 "그룹을 늘려도 부품 좌표는 한 자리도
//   > 바뀌지 않는다" 를 깬다.
//
// 그러므로 뒤집기는 그 통로 하나로 끝나지 않는다:
//
//   자리(`geometry`)        → `patchNodeGeometry` — 정수 반올림과 퇴화 방지를 그대로 받는다
//   속(`path`·`parts`·`anchors`·연결선 `points`) → 이 파일 — 그 통로가 일부러 안 쓰는 것들
//
// **자리 반쪽이 기존 통로를 그대로 지나는 것이 요점이다.** 우회하면 정수화와 퇴화 방지를
// 다시 구현하게 되고, 그 두 번째 구현이 갈라지는 날 "뒤집으면 도형이 사라진다" 가 시작된다.
// 속 반쪽은 A17 이 일부러 손대지 않는 자리이므로 그 가정을 깨지 않는다.
//
// ## 문구는 상자가 아니라 **가운데**로 간다 (§결정 6 · §결정 7)
//
// 90° 회전에서 글자는 눕지 않는다(회전 필드가 없다 — 014 의 몫). 그래서 문구의 상자를
// 회전시키면 `w`·`h` 가 맞바뀐 **거짓 상자**가 나온다. 대신 상자의 **가운데**를 점으로
// 옮기고 크기는 그대로 둔다 — 뒤집기에서도 같은 산술이 정확히 맞는다(크기가 안 변하므로
// 가운데를 옮기는 것과 상자를 비추는 것이 같은 값이다).
//
// 기준점을 되짚는 산술도 새로 짓지 않는다. `resolveTextOrigin` 이 "기준점에서 상자 왼쪽
// 끝까지" 의 관계를 이미 소유하므로, **지금 기준점과 지금 상자의 차이**를 재서 새 상자에
// 그대로 얹는다 — 정렬 셋의 갈래를 여기서 다시 적지 않는다는 뜻이다.
//
// @spec SPEC-CANVAS-013 REQ-01 · REQ-02 · REQ-03 · REQ-04

import type { CanvasElement, Geometry, PointGeometry } from './canvasConfig';
import { unprojectBox, type CanvasBox, type CanvasProjection } from './canvasGeometry';
import { patchNodeGeometry } from './canvasEditGeometry';
import { outlineAabb, outlineBox } from './canvasOutline';
import { ANCHOR_LOCAL_EXTENT, type CustomAnchor } from './connector/anchorTypes';
import { isConnector, type ConnectorElement, type ConnectorEnd } from './connector/connectorTypes';
// 붙은 끝과 자유 끝을 가르는 **유일한 판정**(011 AC-45). 제 손으로 `'el' in` 을 적으면
// 그 판정이 둘이 되고 출시된 가드가 곧바로 운다.
import { isAttachedEnd } from './connector/resolveConnector';
import { GROUP_LOCAL_EXTENT, type CanvasNode, type GroupElement } from './group/groupTypes';
import { PATH_LOCAL_EXTENT, type PathCommand } from './shapes/pathTypes';
import {
  swapsExtent,
  transformBoxWithin,
  transformLocalPoint,
  transformPointWithin,
  unionBox,
  type TransformKind,
} from './canvasTransform';

/** 로컬 격자 전체를 덮는 상자 — 부품이 사는 정사각형이다. */
const LOCAL_SQUARE: CanvasBox = { x: 0, y: 0, w: GROUP_LOCAL_EXTENT, h: GROUP_LOCAL_EXTENT };

// --- 속: 로컬 좌표 ---------------------------------------------------------

/**
 * 경로 명령 목록을 변환한다.
 *
 * **`C` 의 제어점 둘도 함께 지난다.** 끝점만 옮기면 곡선이 뒤집힌 껍데기 안에서 옛 방향으로
 * 휘고, 그 결함은 저장 왕복을 견디며 **화면으로만** 드러난다. `Z` 는 좌표가 없으므로
 * 그대로다.
 */
export function transformPathCommands(
  kind: TransformKind,
  cmds: readonly PathCommand[],
): PathCommand[] {
  const at = (p: { x: number; y: number }): { x: number; y: number } =>
    transformLocalPoint(kind, p, PATH_LOCAL_EXTENT);
  return cmds.map((cmd): PathCommand => {
    switch (cmd.c) {
      case 'Z':
        return { c: 'Z' };
      case 'C': {
        const c1 = at({ x: cmd.x1, y: cmd.y1 });
        const c2 = at({ x: cmd.x2, y: cmd.y2 });
        const end = at({ x: cmd.x, y: cmd.y });
        return { c: 'C', x1: c1.x, y1: c1.y, x2: c2.x, y2: c2.y, x: end.x, y: end.y };
      }
      default: {
        const p = at({ x: cmd.x, y: cmd.y });
        return { c: cmd.c, x: p.x, y: p.y };
      }
    }
  });
}

/** 임의 앵커를 변환한다 — 경로 명령과 **같은 격자, 같은 함수**다(세 격자가 같은 값이다). */
export function transformAnchors(
  kind: TransformKind,
  anchors: readonly CustomAnchor[],
): CustomAnchor[] {
  return anchors.map((a) => {
    const p = transformLocalPoint(kind, { x: a.x, y: a.y }, ANCHOR_LOCAL_EXTENT);
    return { ...a, x: p.x, y: p.y };
  });
}

/**
 * 부품 하나를 그룹 로컬 격자 안에서 변환한다 — **그리고 한 겹 더 내려간다.**
 *
 * 부품이 경로면 그 명령도 제 로컬 격자에서 변환되어야 한다. 겹은 **정확히 둘**이다 —
 * 부품에 그룹이 올 수 없기 때문이며(A6 이 타입으로 강제한다) 그래서 재귀가 아니라 두 번의
 * 적용이면 끝난다.
 */
function transformPart(kind: TransformKind, part: CanvasElement): CanvasElement {
  const withContent = transformContent(kind, part);
  const geo = transformGeometry(kind, withContent, LOCAL_SQUARE, localBoxOf(part));
  if (geo === undefined) return withContent;
  // 부품은 `patchNodeGeometry` 를 지나지 않는다 — 그 통로가 쓰는 것은 **최상위 노드**의
  // `geometry` 이고, 부품 좌표는 그 통로가 일부러 손대지 않는 자리다(A17).
  return { ...withContent, geometry: geo } as CanvasElement;
}

/** 속만 바꾼다 — 자리(`geometry`)는 건드리지 않는다. */
function transformContent<T extends CanvasElement | GroupElement>(kind: TransformKind, node: T): T {
  let out = node;
  if (node.anchors !== undefined && node.anchors.length > 0) {
    out = { ...out, anchors: transformAnchors(kind, node.anchors) };
  }
  if (out.kind === 'path') {
    out = { ...out, path: transformPathCommands(kind, out.path) } as T;
  }
  if (out.kind === 'group') {
    out = { ...out, parts: out.parts.map((p) => transformPart(kind, p)) } as T;
  }
  return out;
}

// --- 자리: 상자와 점 -------------------------------------------------------

/** 부품의 상자 — 로컬 격자에서 잰다. 선·문구는 상자가 없으므로 `undefined` 다. */
function localBoxOf(part: CanvasElement): CanvasBox | undefined {
  const g = part.geometry;
  if ('w' in g) return { x: g.x, y: g.y, w: g.w, h: g.h };
  return undefined;
}

/**
 * 노드 하나의 새 기하. 상자형은 상자를, 선은 두 끝점을, 문구는 **가운데**를 옮긴다.
 *
 * `box` 는 이 노드의 윤곽 상자(문구라면 글자를 재어 나온 상자)이고, 없으면 상자형이
 * 아니라는 뜻이다.
 */
function transformGeometry(
  kind: TransformKind,
  node: CanvasElement | GroupElement,
  sel: CanvasBox,
  box: CanvasBox | undefined,
): Geometry | undefined {
  const g = node.geometry;

  if ('w' in g) {
    const next = transformBoxWithin(kind, { x: g.x, y: g.y, w: g.w, h: g.h }, sel);
    return { x: next.x, y: next.y, w: next.w, h: next.h };
  }

  if ('x1' in g) {
    // 선은 상자가 없다 — 두 끝점을 각각 옮기면 자리와 방향이 **한 번에** 맞는다.
    const a = transformPointWithin(kind, { x: g.x1, y: g.y1 }, sel);
    const b = transformPointWithin(kind, { x: g.x2, y: g.y2 }, sel);
    return { x1: a.x, y1: a.y, x2: b.x, y2: b.y };
  }

  // 문구 — 상자의 **가운데**를 옮기고 크기는 그대로 둔다(§결정 6 · §결정 7).
  if (box === undefined) return { x: g.x, y: g.y };
  const center = transformPointWithin(
    kind,
    { x: box.x + box.w / 2, y: box.y + box.h / 2 },
    sel,
  );
  // 글자는 눕지 않으므로 크기는 맞바뀌지 않는다.
  const nextBox: CanvasBox = {
    x: center.x - box.w / 2,
    y: center.y - box.h / 2,
    w: box.w,
    h: box.h,
  };
  // 기준점과 상자의 **지금 차이**를 새 상자에 그대로 얹는다 — 정렬 셋의 갈래를 여기서
  // 다시 적지 않는다(그 관계는 `resolveTextOrigin` 이 소유한다).
  return { x: nextBox.x + (g.x - box.x), y: nextBox.y + (g.y - box.y) };
}

/**
 * 연결선의 한 끝 — **붙은 끝은 건드리지 않는다**(K4). 앵커가 도형과 함께 이미 움직였다.
 *
 * 가르는 일은 `isAttachedEnd` 한 함수가 한다. 여기서 `'el' in` 을 적으면 그 판정이 둘이
 * 되고, 011 이 그 흩어짐을 **가드로** 막아 두었다(AC-45) — 적는 순간 시험이 운다.
 */
function transformEnd(kind: TransformKind, end: ConnectorEnd, sel: CanvasBox): ConnectorEnd {
  if (isAttachedEnd(end)) return end;
  const p = transformPointWithin(kind, end, sel);
  return { x: p.x, y: p.y };
}

function transformConnector(
  kind: TransformKind,
  node: ConnectorElement,
  sel: CanvasBox,
): ConnectorElement {
  const out: ConnectorElement = {
    ...node,
    from: transformEnd(kind, node.from, sel),
    to: transformEnd(kind, node.to, sel),
  };
  if (node.points === undefined) return out;
  const points: PointGeometry[] = node.points.map((p) => {
    const q = transformPointWithin(kind, p, sel);
    return { x: Math.round(q.x), y: Math.round(q.y) };
  });
  return { ...out, points };
}

// --- 입구 -----------------------------------------------------------------

/**
 * 고른 노드들의 **선택 상자**(캔버스 단위). 연결선은 들어가지 않는다.
 *
 * `outlineBox` 가 `OutlinedNode` 만 받는 것이 그 배제의 전부다 — 연결선에는 `geometry` 가
 * 없어 낼 상자가 없고, 두 끝을 감싸는 상자를 지어 넣으면 그 상자가 곧 뒤집기의 축이 되어
 * "선을 고르면 도형이 다른 자리로 간다" 가 된다.
 *
 * 고른 것이 연결선뿐이면 `undefined` 다 — 축이 없으면 뒤집을 수 없다.
 */
export function selectionBox(
  nodes: readonly CanvasNode[],
  selected: ReadonlySet<string>,
  proj: CanvasProjection,
  textWidths: Readonly<Record<string, number>>,
): CanvasBox | undefined {
  // `filter` 는 타입을 좁히지 않는다 — `flatMap` 으로 걸러야 `outlineBox` 가 받는 타입이
  // 된다. 그 함수의 인자가 `OutlinedNode` 인 것이 연결선 배제의 전부다.
  const boxes = nodes.flatMap((n): CanvasBox[] => {
    if (!selected.has(n.id) || isConnector(n)) return [];
    // **축-나란 상자를 읽는다**(014 §결정 2). 뒤집기의 축은 화면에서 보이는 상자여야
    // 하고, 돌아간 도형이 보이는 자리는 그 잉크를 덮는 상자다. 각도가 0 이면 두 상자가
    // 같은 수이므로 013 이 배달한 동작은 한 글자도 바뀌지 않는다(K3).
    return [unprojectBox(outlineAabb(n, proj, textWidths), proj)];
  });
  return unionBox(boxes);
}

/**
 * 고른 노드들을 변환한다. **노드 수도 차례도 바뀌지 않는다**(K3).
 *
 * 바꿀 것이 없으면 받은 배열을 그대로(같은 참조) 돌려준다 — 호출부가 그것으로 "쓸 일이
 * 없다" 를 알아채 헛된 config 쓰기와 렌더 프레임을 만들지 않는다(`moveElementTo` 와 같은
 * 규율).
 */
export function transformNodes(
  kind: TransformKind,
  nodes: readonly CanvasNode[],
  selected: ReadonlySet<string>,
  proj: CanvasProjection,
  textWidths: Readonly<Record<string, number>>,
): CanvasNode[] {
  const sel = selectionBox(nodes, selected, proj, textWidths);
  if (sel === undefined) return [...nodes];

  // ① 속을 먼저 갈아 끼우고 연결선을 처리한다. 자리는 아직 건드리지 않는다.
  const withContent: CanvasNode[] = nodes.map((node) => {
    if (!selected.has(node.id)) return node;
    if (isConnector(node)) return transformConnector(kind, node, sel);
    return transformContent(kind, node);
  });

  // ② 자리는 **기존 통로**를 지난다(§결정 2 · K5). 접으면서 새 배열이 한 번씩 난다.
  let out = withContent;
  for (const node of nodes) {
    if (!selected.has(node.id) || isConnector(node)) continue;
    // 여기서는 `continue` 가 좁혀 준다(`isConnector` 가 타입 가드다).
    const box = unprojectBox(outlineBox(node, proj, textWidths), proj);
    const geo = transformGeometry(kind, node, sel, box);
    if (geo === undefined) continue;
    out = patchNodeGeometry(out, node.id, geo);
  }
  return out;
}

/** 회전이 상자를 세로로 눕히는가 — 띠가 아이콘 방향을 고를 때 읽는다. */
export { swapsExtent };
