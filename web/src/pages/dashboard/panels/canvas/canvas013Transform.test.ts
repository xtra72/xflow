// 노드 한 벌 변환 (SPEC-CANVAS-013 M3 · M4 · AC-13~AC-24).
//
// **이 파일이 막는 실패는 크래시가 아니다.** 자리만 뒤집히고 속이 그대로면 뒤집힌 화살표가
// 여전히 같은 쪽을 가리킨다 — 예외도 경고도 없이 **화면으로만** 드러나며, 저장 왕복까지
// 견딘다. 그래서 고정 입력이 **비대칭**이어야 한다: 대칭 도형은 그 결함을 통째로 감춘다.
//
// @spec SPEC-CANVAS-013 REQ-01 · REQ-02 · REQ-03 · REQ-04

import { describe, expect, it } from 'vitest';

import type { CanvasElement, CanvasSize, TextElement } from './canvasConfig';
import type { CanvasProjection } from './canvasGeometry';
import type { ConnectorElement } from './connector/connectorTypes';
import { isConnector } from './connector/connectorTypes';
import type { CanvasNode, GroupElement } from './group/groupTypes';
import { PATH_LOCAL_EXTENT, type PathCommand } from './shapes/pathTypes';
import { ANCHOR_LOCAL_EXTENT } from './connector/anchorTypes';
import {
  selectionBox,
  transformAnchors,
  transformNodes,
  transformPathCommands,
} from './canvasTransformNodes';

// --- 고정 입력 -------------------------------------------------------------

const CANVAS: CanvasSize = { width: 500, height: 400 };
/** 두 축의 축척이 서로 다르다 — 한 축만 맞는 결함을 감추지 않는다. */
const PROJ: CanvasProjection = { stage: { width: 250, height: 200 }, canvas: CANVAS };
const E = PATH_LOCAL_EXTENT;

/** 비대칭 · 네 명령 전부. 대칭 도형은 뒤집기 결함을 감춘다(008 시험 규율 D2). */
const ASYMMETRIC: readonly PathCommand[] = [
  { c: 'M', x: 0, y: E },
  { c: 'L', x: 7000, y: E },
  { c: 'C', x1: 9100, y1: 8200, x2: 9700, y2: 5300, x: 8800, y: 1400 },
  { c: 'L', x: 0, y: 0 },
  { c: 'Z' },
];

function pathEl(over: Partial<CanvasElement> = {}): CanvasElement {
  return {
    id: 'p1',
    kind: 'path',
    style: {},
    geometry: { x: 31, y: 57, w: 140, h: 90 },
    path: ASYMMETRIC.map((c) => ({ ...c })),
    ...over,
  } as CanvasElement;
}

function rectEl(id: string, box: { x: number; y: number; w: number; h: number }): CanvasElement {
  return { id, kind: 'rect', style: {}, geometry: { ...box } };
}

const ALL = (nodes: readonly CanvasNode[]): Set<string> => new Set(nodes.map((n) => n.id));

function run(kind: Parameters<typeof transformNodes>[0], nodes: readonly CanvasNode[]): CanvasNode[] {
  return transformNodes(kind, nodes, ALL(nodes), PROJ, {});
}

function byId(nodes: readonly CanvasNode[], id: string): CanvasNode {
  const found = nodes.find((n) => n.id === id);
  if (found === undefined) throw new Error(`${id} 가 사라졌다`);
  return found;
}

/** 상자를 가진 노드로 좁힌다 — 연결선이면 **여기서** 죽는다(조용한 폴백을 두지 않는다). */
function outlinedById(nodes: readonly CanvasNode[], id: string): CanvasElement | GroupElement {
  const node = byId(nodes, id);
  if (isConnector(node)) throw new Error(`${id} 가 연결선이다`);
  return node;
}

// --- ① 속이 함께 돈다 (AC-13 · AC-05 · AC-07) ------------------------------

describe('비대칭 경로가 **모양까지** 뒤집힌다 (AC-13 · REQ-03)', () => {
  it('명령 목록이 거울이다', () => {
    const out = run('flipX', [pathEl()]);
    const p = outlinedById(out, 'p1');
    if (p.kind !== 'path') throw new Error('경로가 아니다');
    // 자리만 뒤집히면 여기가 원본과 같다 — 그것이 REQ-03 이 막는 그 결함이다.
    expect(p.path).not.toEqual(ASYMMETRIC);
    expect(p.path[0]).toEqual({ c: 'M', x: E, y: E });
    expect(p.path[1]).toEqual({ c: 'L', x: E - 7000, y: E });
  });

  it('`C` 의 제어점 둘도 지난다 (AC-05)', () => {
    const out = transformPathCommands('flipX', ASYMMETRIC);
    const c = out[2];
    if (c === undefined || c.c !== 'C') throw new Error('C 가 아니다');
    // **여섯 좌표 전부**다. 끝점만 옮기면 곡선이 뒤집힌 껍데기 안에서 옛 방향으로 휜다.
    expect(c.x1).toBe(E - 9100);
    expect(c.x2).toBe(E - 9700);
    expect(c.x).toBe(E - 8800);
    expect(c.y1).toBe(8200);
    expect(c.y2).toBe(5300);
    expect(c.y).toBe(1400);
  });

  it('`Z` 는 그대로다 (AC-06)', () => {
    expect(transformPathCommands('rotateCW', [{ c: 'Z' }])).toEqual([{ c: 'Z' }]);
  });

  it('같은 거울 두 번이면 명령이 제자리다 (K2)', () => {
    const once = transformPathCommands('flipY', ASYMMETRIC);
    expect(transformPathCommands('flipY', once)).toEqual(ASYMMETRIC.map((c) => ({ ...c })));
  });

  it('네 번 돌리면 명령이 제자리다 (K1)', () => {
    let cmds = ASYMMETRIC.map((c) => ({ ...c })) as PathCommand[];
    for (let i = 0; i < 4; i += 1) cmds = transformPathCommands('rotateCW', cmds);
    expect(cmds).toEqual(ASYMMETRIC.map((c) => ({ ...c })));
  });

  it('임의 앵커가 **같은 격자, 같은 산술**을 지난다 (AC-07)', () => {
    const out = transformAnchors('flipX', [{ id: 'a', x: 1234, y: 8765 }]);
    expect(out[0]).toEqual({ id: 'a', x: ANCHOR_LOCAL_EXTENT - 1234, y: 8765 });
    // 세 로컬 격자가 같은 값이므로 경로 명령과 같은 수가 나온다.
    expect(ANCHOR_LOCAL_EXTENT).toBe(PATH_LOCAL_EXTENT);
  });

  it('요소의 앵커도 함께 뒤집힌다', () => {
    const out = run('flipX', [pathEl({ anchors: [{ id: 'a', x: 2000, y: 3000 }] })]);
    const p = outlinedById(out, 'p1');
    expect(p.anchors?.[0]).toEqual({ id: 'a', x: E - 2000, y: 3000 });
  });
});

// --- ② 그룹은 두 겹 (AC-14 · AC-15) ----------------------------------------

describe('그룹은 두 겹이 함께 돈다 (AC-14 · §결정 5)', () => {
  function group(): GroupElement {
    return {
      id: 'g1',
      kind: 'group',
      geometry: { x: 20, y: 300, w: 80, h: 60 },
      parts: [
        { id: 'body', kind: 'rect', style: {}, geometry: { x: 0, y: 0, w: 3000, h: 2000 } },
        {
          id: 'mark',
          kind: 'path',
          style: {},
          geometry: { x: 4000, y: 1000, w: 2000, h: 3000 },
          path: ASYMMETRIC.map((c) => ({ ...c })),
        } as CanvasElement,
      ],
    };
  }

  it('그룹 상자 · 부품 좌표 · 부품 경로 **셋 다** 변환된다', () => {
    const out = run('flipX', [group()]);
    const g = outlinedById(out, 'g1');
    if (g.kind !== 'group') throw new Error('그룹이 아니다');

    // 부품 좌표(로컬 격자 안의 거울)
    const body = g.parts[0]!;
    expect('w' in body.geometry && body.geometry.x).toBe(PATH_LOCAL_EXTENT - 3000);
    const mark = g.parts[1]!;
    expect('w' in mark.geometry && mark.geometry.x).toBe(PATH_LOCAL_EXTENT - 4000 - 2000);

    // **그리고 그 부품의 경로 명령까지** — 이것이 "두 겹" 의 전부다.
    if (mark.kind !== 'path') throw new Error('부품이 경로가 아니다');
    expect(mark.path).not.toEqual(ASYMMETRIC);
    expect(mark.path[0]).toEqual({ c: 'M', x: E, y: E });
  });

  it('네 번 돌리면 그룹이 통째로 제자리다 (K1)', () => {
    let nodes: CanvasNode[] = [group()];
    for (let i = 0; i < 4; i += 1) nodes = run('rotateCW', nodes);
    expect(nodes[0]).toEqual(group());
  });
});

// --- ③ 연결선 (AC-16 · K4) --------------------------------------------------

describe('연결선의 붙은 끝은 한 글자도 바뀌지 않는다 (AC-16 · K4 · REQ-04)', () => {
  function scene(): CanvasNode[] {
    const link: ConnectorElement = {
      id: 'c1',
      kind: 'connector',
      from: { el: 'r1', a: 'e' },
      to: { x: 300, y: 250 },
      route: 'elbow',
      points: [{ x: 200, y: 120 }],
    };
    return [rectEl('r1', { x: 10, y: 20, w: 60, h: 40 }), link];
  }

  it('붙은 끝은 그대로, 자유 끝과 중간점만 옮긴다', () => {
    const out = run('flipX', scene());
    const c = byId(out, 'c1');
    if (!isConnector(c)) throw new Error('연결선이 아니다');
    // 붙은 끝은 앵커를 가리키고 앵커는 도형과 함께 이미 움직였다. 여기서 또 옮기면 두 번이다.
    expect(c.from).toEqual({ el: 'r1', a: 'e' });
    expect(c.to).not.toEqual({ x: 300, y: 250 });
    expect(c.points?.[0]).not.toEqual({ x: 200, y: 120 });
  });

  it('연결선만 골라 놓으면 축이 없어 아무 일도 없다', () => {
    // `selectionBox` 가 `undefined` 를 낸다 — 두 끝을 감싸는 상자를 지어내지 않는다.
    const nodes = scene();
    expect(selectionBox(nodes, new Set(['c1']), PROJ, {})).toBeUndefined();
  });
});

// --- ④ 배열의 성질 (AC-17 · AC-18 · AC-19) ---------------------------------

describe('배열의 성질', () => {
  const scene = (): CanvasNode[] => [
    rectEl('a', { x: 0, y: 0, w: 20, h: 10 }),
    rectEl('b', { x: 80, y: 0, w: 20, h: 10 }),
    rectEl('c', { x: 200, y: 200, w: 30, h: 30 }),
  ];

  it('노드 수와 차례가 그대로다 (AC-17 · K3)', () => {
    const before = scene();
    const after = run('rotateCW', before);
    expect(after).toHaveLength(before.length);
    expect(after.map((n) => n.id)).toEqual(before.map((n) => n.id));
  });

  it('고르지 않은 것은 **참조까지** 그대로다 (AC-18)', () => {
    const nodes = scene();
    const out = transformNodes('flipX', nodes, new Set(['a', 'b']), PROJ, {});
    expect(out[2]).toBe(nodes[2]);
  });

  it('둘을 가로로 뒤집으면 자리를 맞바꾼다 (AC-09)', () => {
    const out = transformNodes('flipX', scene(), new Set(['a', 'b']), PROJ, {});
    const a = outlinedById(out, 'a');
    const b = outlinedById(out, 'b');
    expect('w' in a.geometry && a.geometry.x).toBe(80);
    expect('w' in b.geometry && b.geometry.x).toBe(0);
  });

  it('고른 것이 없으면 아무것도 바뀌지 않는다 (AC-19)', () => {
    const nodes = scene();
    const out = transformNodes('flipX', nodes, new Set(), PROJ, {});
    expect(out).toEqual(nodes);
  });
});

// --- ⑤ 선 (AC-13 의 선 갈래) ------------------------------------------------

describe('선은 두 끝점이 함께 간다', () => {
  it('기울기가 거울이 된다', () => {
    const line: CanvasElement = {
      id: 'l1',
      kind: 'line',
      style: {},
      geometry: { x1: 10, y1: 10, x2: 90, y2: 50 },
    };
    const out = run('flipX', [line]);
    const l = outlinedById(out, 'l1');
    if (!('x1' in l.geometry)) throw new Error('선이 아니다');
    // 왼쪽 위 → 오른쪽 아래 였던 선이 오른쪽 위 → 왼쪽 아래가 된다.
    expect(l.geometry.x1).toBe(90);
    expect(l.geometry.x2).toBe(10);
    expect(l.geometry.y1).toBe(10);
    expect(l.geometry.y2).toBe(50);
  });
});

// --- ⑥ 문구 (AC-22 · AC-24 · M4) -------------------------------------------

describe('문구 (AC-22 · AC-24 · §결정 6 · §결정 7)', () => {
  function textEl(align: 'left' | 'center' | 'right'): TextElement {
    return {
      id: 't1',
      kind: 'text',
      style: { align, fontSize: 20 },
      text: 'ABCDE',
      geometry: { x: 100, y: 100 },
    } as TextElement;
  }

  /** 글자 폭 장부 — 실측 폭이 섞인 상자를 만들려면 이것이 있어야 한다. */
  const WIDTHS = { t1: 50 };

  it.each(['left', 'center', 'right'] as const)(
    '`%s` 정렬에서 보이는 상자가 거울 자리에 온다 (AC-22)',
    (align) => {
      const el = textEl(align);
      const rect = rectEl('r1', { x: 0, y: 0, w: 400, h: 300 });
      const before = selectionBox([el, rect], new Set(['t1']), PROJ, WIDTHS)!;
      const out = transformNodes('flipX', [el, rect], new Set(['t1', 'r1']), PROJ, WIDTHS);
      const sel = selectionBox([el, rect], new Set(['t1', 'r1']), PROJ, WIDTHS)!;
      const after = selectionBox(out, new Set(['t1']), PROJ, WIDTHS)!;

      // 거울 자리 = 선택 상자 안에서 좌우가 뒤집힌 자리. 기준점만 비추면 정렬에 따라
      // 글자 폭만큼 어긋나고, 그 어긋남은 `left`/`right` 에서만 드러난다.
      const expectedX = sel.x + sel.w - (before.x - sel.x) - before.w;
      expect(Math.abs(after.x - expectedX), align).toBeLessThanOrEqual(1);
      expect(Math.abs(after.w - before.w), align).toBeLessThanOrEqual(1);
    },
  );

  it('90° 회전에서 글자는 서 있다 — 자리만 돈다 (AC-24 · §결정 7)', () => {
    const el = textEl('center');
    const out = run('rotateCW', [el, rectEl('r1', { x: 0, y: 0, w: 400, h: 300 })]);
    const t = outlinedById(out, 't1');
    if (t.kind !== 'text') throw new Error('문구가 아니다');
    // 정렬도 글자도 그대로다. 눕히려면 회전 필드가 있어야 하고 그것이 014 의 주제다.
    expect(t.style.align).toBe('center');
    expect(t.text).toBe('ABCDE');
    // 자리는 실제로 움직였다 — "아무것도 안 했다" 와 구분한다.
    expect(t.geometry).not.toEqual({ x: 100, y: 100 });
  });
});
