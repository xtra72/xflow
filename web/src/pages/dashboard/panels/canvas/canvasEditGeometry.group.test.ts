// 그룹의 기하 쓰기 통로와 손잡이 (SPEC-CANVAS-004 M3).
//
// 004 가 이 모듈에 더한 것은 **갈래 둘**뿐이다 — `handlesFor`/`handlePositions` 의 그룹
// (여덟 손잡이)과 `patchNodeGeometry` 의 `case 'group':` 하나. 그 한 갈래로 이동 · 8핸들
// 크기 조절 · 정렬 · 격자 붙임 · 방향키 미세 이동이 전부 그룹에 걸린다(불변식 G3).
//
// **이 파일이 가장 무겁게 재는 것은 "쓰지 않는 것"이다**: 그룹 상자를 아무리 늘려도
// `parts` 의 저장 좌표는 한 자리도 바뀌지 않는다(가정 A17 · 008 불변식 J2 와 같은 자리).
//
// @spec SPEC-CANVAS-004 REQ-03 · REQ-08 · 불변식 G3

import { readFileSync } from 'node:fs';
import { join } from 'node:path';

import { describe, expect, it } from 'vitest';

import { MIN_ELEMENT_EXTENT, type CanvasElement, type Geometry } from './canvasConfig';
import { isConnector } from './connector/connectorTypes';
import {
  BOX_HANDLE_IDS,
  handlePositions,
  handlesFor,
  patchNodeGeometry,
  type CanvasHandleId,
} from './canvasEditGeometry';
import { projectBox, type CanvasProjection } from './canvasGeometry';
import type { CanvasNode, GroupElement, OutlinedNodeKind } from './group/groupTypes';

const PROJ: CanvasProjection = {
  stage: { width: 400, height: 200 },
  canvas: { width: 500, height: 400 },
};

function part(id: string, x: number, y: number): CanvasElement {
  return { id, kind: 'rect', style: {}, geometry: { x, y, w: 1700, h: 900 } };
}

function group(over: Partial<GroupElement> = {}): GroupElement {
  return {
    id: 'grp-1',
    kind: 'group',
    geometry: { x: 73, y: 41, w: 317, h: 181 },
    // 상자가 서로 다르고 하나는 안쪽에 떠 있다(E-A · E-B).
    parts: [part('body', 0, 0), part('stem', 4100, 3300), part('tip', 8000, 7000)],
    ...over,
  };
}

// --- 손잡이 (REQ-08) -------------------------------------------------------

describe('handlesFor — 그룹은 제 상자에 여덟 손잡이를 갖는다', () => {
  it('그룹이 `BOX_HANDLE_IDS` 를 **같은 참조로** 돌려준다(rect 와 같은 표)', () => {
    expect(handlesFor('group')).toBe(BOX_HANDLE_IDS);
    expect(handlesFor('group')).toBe(handlesFor('rect'));
    expect(handlesFor('group')).toHaveLength(8);
  });

  it('상자를 가진 여섯 종류 전부에 답한다 — 답하지 못하는 종류가 없다', () => {
    // SPEC-CANVAS-011 M4 — 표의 이름이 `CanvasNodeKind` 에서 `OutlinedNodeKind` 로
    // 좁아졌다. 최상위 종류는 일곱이 되었지만 `handlesFor` 의 **범위**는 여섯 그대로다:
    // 연결선에는 늘릴 상자가 없어 8핸들이 서지 않고(REQ-07), 그래서 인자 타입이 애초에
    // 그것을 받지 않는다. 총망라 판정은 그대로 살아 있다 — **상자를 가진** 일곱째 종류가
    // 들어오면 이 `Record` 가 컴파일에서 운다.
    const kinds: Record<OutlinedNodeKind, true> = {
      rect: true,
      ellipse: true,
      line: true,
      text: true,
      path: true,
      group: true,
    };
    for (const kind of Object.keys(kinds) as OutlinedNodeKind[]) {
      expect(handlesFor(kind).length, kind).toBeGreaterThan(0);
    }
  });

  it('`handlePositions` 가 그룹 **상자**에 여덟 자리를 낸다 — 부품에는 서지 않는다(A18)', () => {
    const g = group();
    const points = handlePositions(g, PROJ);
    expect(points).toHaveLength(handlesFor('group').length);
    const box = projectBox(g.geometry, PROJ);
    const ids = points.map((h) => h.id as CanvasHandleId);
    expect(ids).toEqual([...BOX_HANDLE_IDS]);
    // 네 모서리가 그룹 상자의 네 모서리다.
    const nw = points.find((h) => h.id === 'nw')?.point;
    const se = points.find((h) => h.id === 'se')?.point;
    expect(nw).toEqual({ x: box.x, y: box.y });
    expect(se).toEqual({ x: box.x + box.w, y: box.y + box.h });
    // 부품 자리에 선 손잡이는 하나도 없다 — 전부 상자 둘레다.
    for (const h of points) {
      expect(h.point.x).toBeGreaterThanOrEqual(box.x);
      expect(h.point.x).toBeLessThanOrEqual(box.x + box.w);
    }
  });
});

// --- 기하 쓰기 통로 (불변식 G3 · A17) -------------------------------------

/**
 * 배열의 한 자리에서 기하를 읽는다. 연결선에는 기하가 없으므로(SPEC-CANVAS-011 M4) 좁혀
 * 읽고, 좁히기가 실패하면 `undefined` 라 단언이 조용히 통과하지 않는다.
 */
function geometryAt(nodes: readonly CanvasNode[], idx: number): Geometry | undefined {
  const node = nodes[idx];
  return node !== undefined && !isConnector(node) ? node.geometry : undefined;
}

describe('patchNodeGeometry — 그룹 갈래 하나 (G3)', () => {
  const nodes: readonly CanvasNode[] = [
    { id: 'r1', kind: 'rect', style: {}, geometry: { x: 0, y: 0, w: 10, h: 10 } },
    group(),
    { id: 'r2', kind: 'rect', style: {}, geometry: { x: 20, y: 20, w: 10, h: 10 } },
  ];

  it('그룹 상자를 실제로 쓴다 — 그대로 돌려주지 않는다', () => {
    const out = patchNodeGeometry(nodes, 'grp-1', { x: 100, y: 200, w: 300, h: 400 });
    expect(out[1]).not.toBe(nodes[1]);
    expect(geometryAt(out, 1)).toEqual({ x: 100, y: 200, w: 300, h: 400 });
  });

  it('**부품의 저장 좌표는 한 자리도 바뀌지 않는다** (A17 · J2 와 같은 자리)', () => {
    const before = structuredClone(group().parts);
    const out = patchNodeGeometry(nodes, 'grp-1', { x: 0, y: 0, w: 9000, h: 3 });
    const after = out[1];
    expect(after?.kind).toBe('group');
    if (after?.kind !== 'group') throw new Error('그룹이어야 한다');
    expect(after.parts).toEqual(before);
    // 배열도 **같은 참조**로 실려 간다 — 부품을 새로 짓는 순간 "안 바뀐다" 가 우연이 된다.
    expect(after.parts).toBe((nodes[1] as GroupElement).parts);
  });

  it('정수로 반올림하고 퇴화를 만들지 않는다(쓰기 통로의 규율 그대로)', () => {
    // 그룹이 rect 와 **같은 `writableBox`** 를 지난다는 것이 이 단언의 내용이다 —
    // 반올림 · 음수 크기의 절대값 · 최소 크기 보장이 전부 그 함수의 규율이고, 004 는
    // 그것을 한 글자도 다시 적지 않는다.
    const out = patchNodeGeometry(nodes, 'grp-1', { x: 10.6, y: 20.4, w: 0, h: -5 });
    expect(geometryAt(out, 1)).toEqual({ x: 11, y: 20, w: MIN_ELEMENT_EXTENT, h: 5 });
    const rectOut = patchNodeGeometry(nodes, 'r1', { x: 10.6, y: 20.4, w: 0, h: -5 });
    expect(geometryAt(out, 1)).toEqual(geometryAt(rectOut, 0));
  });

  it('형상이 맞지 않으면 그룹을 그대로 둔다(선·점 기하)', () => {
    expect(patchNodeGeometry(nodes, 'grp-1', { x1: 0, y1: 0, x2: 5, y2: 5 })[1]).toBe(nodes[1]);
    expect(patchNodeGeometry(nodes, 'grp-1', { x: 1, y: 2 })[1]).toBe(nodes[1]);
  });

  it('형제는 참조 그대로 두고 새 배열을 낸다', () => {
    const out = patchNodeGeometry(nodes, 'grp-1', { x: 1, y: 2, w: 3, h: 4 });
    expect(out).not.toBe(nodes);
    expect(out[0]).toBe(nodes[0]);
    expect(out[2]).toBe(nodes[2]);
  });

  it('기하 외의 필드(id·kind·style·symbol)는 그대로 실려 간다', () => {
    const stamped = group({ style: { opacity: 0.3 }, symbol: { catalog_id: 'v', version: '1' } });
    const out = patchNodeGeometry([stamped], 'grp-1', { x: 1, y: 2, w: 3, h: 4 });
    const after = out[0];
    if (after?.kind !== 'group') throw new Error('그룹이어야 한다');
    expect(after.style).toBe(stamped.style);
    expect(after.symbol).toBe(stamped.symbol);
  });
});

// --- 통로가 여전히 하나다 --------------------------------------------------

describe('기하 쓰기 규칙이 둘이 되지 않았다 (G3)', () => {
  const source = readFileSync(join(__dirname, 'canvasEditGeometry.ts'), 'utf-8')
    .split('\n')
    .filter((line) => !/^\s*(\/\/|\*|\/\*)/.test(line))
    .join('\n');

  it('그 모듈 안에 `parts` 필드를 **대입하는** 자리가 없다', () => {
    // 008 이 `path` 에 대해 세운 가드와 같은 형상이다. `parts:` 로 시작하는 객체 리터럴
    // 항목이 하나라도 생기면 부품 좌표가 기하 통로를 타기 시작한 것이다.
    expect(source).not.toMatch(/[{,]\s*parts\s*:/);
  });

  it('노드 배열을 돌려주는 export 가 **하나뿐이다**', () => {
    const signatures = [...source.matchAll(/export function (\w+)\([^)]*\):\s*([^{;\n]+)/g)];
    const arrayExports = signatures
      .filter(([, , ret]) => {
        const t = (ret ?? '').trim();
        return t === 'CanvasNode[]' || t === 'CanvasElement[]';
      })
      .map(([, name]) => name);
    expect([...new Set(arrayExports)]).toEqual(['patchNodeGeometry']);
  });

  it('그 가드가 무동작이 아니다 — 그 함수가 실제로 그룹 갈래를 갖는다', () => {
    expect(source).toMatch(/case 'group':/);
  });
});
