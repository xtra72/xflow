// 부품 기하 쓰기 — 캔버스 절대 좌표에서 그룹 로컬 격자까지 (SPEC-CANVAS-009 M4).
//
// ## 이 파일이 겨누는 이음매
//
// 캔버스 드래그는 **절대 델타**로 말하고 부품은 **로컬 격자**에 적혀 있다. 그 사이를
// 잇는 것이 `patchPartGeometry` 이며, 이 파일은 그 통로가 004 가 세운 셋을 깨지 않는지
// 잰다 — 그룹 상자 불변(A16 · A17), 형제 참조 유지, clamp 없음.
//
// ## 고정 상자를 이렇게 고른 이유
//
// 그룹 상자 `(100,100)-(300,300)` 은 한 변이 200 이고 로컬 격자가 10000 이므로 **로컬
// 1000 이 캔버스 20 에 정확히 대응한다.** 나누어떨어지므로 반올림이 개입하지 않고, 그래서
// 이 파일의 기대값은 좌표 산술 자체를 잰다(반올림 규율은 `groupCoords.test.ts` 의 몫이다).
//
// @spec SPEC-CANVAS-009 REQ-03 · AC-13 ~ AC-18

import { describe, expect, it } from 'vitest';

import type { CanvasElement } from './canvasConfig';
import { frameKey } from './group/frameKey';
import { findPart, partInCanvasUnits, patchPartGeometry } from './group/groupOps';
import { GROUP_LOCAL_EXTENT, type CanvasNode, type GroupElement } from './group/groupTypes';

// --- 고정 입력 -------------------------------------------------------------

/** 그룹 상자 — 한 변 200, 원점이 0 이 아니다(원점 더하기를 빠뜨리면 여기서 드러난다). */
const BOX = { x: 100, y: 100, w: 200, h: 200 } as const;

/** 로컬 5000 = 상자의 한가운데. 두 축이 같아 축을 맞바꾼 실수는 이 값으로 드러나지 않으므로 */
const HALF = GROUP_LOCAL_EXTENT / 2;

function parts(): CanvasElement[] {
  return [
    { id: 'body', kind: 'rect', geometry: { x: 0, y: 0, w: HALF, h: HALF }, style: {} },
    { id: 'edge', kind: 'line', geometry: { x1: 0, y1: 0, x2: HALF, y2: HALF }, style: {} },
    { id: 'label', kind: 'text', geometry: { x: HALF, y: HALF }, style: {}, text: 'T' },
  ];
}

function group(over: Partial<GroupElement> = {}): GroupElement {
  return { id: 'grp-1', kind: 'group', geometry: { ...BOX }, parts: parts(), ...over };
}

function rect(id: string): CanvasElement {
  return { id, kind: 'rect', geometry: { x: 5, y: 6, w: 7, h: 8 }, style: {} };
}

/** 앞에도 뒤에도 형제가 있다 — 참조 유지를 한쪽에서만 재면 절반만 재는 것이다. */
function nodes(): CanvasNode[] {
  return [rect('a'), group(), rect('b')];
}

function groupOf(list: readonly CanvasNode[]): GroupElement {
  const g = list[1];
  expect(g!.kind).toBe('group');
  return g as GroupElement;
}

function partOf(list: readonly CanvasNode[], partId: string): CanvasElement {
  const p = groupOf(list).parts.find((x) => x.id === partId);
  expect(p, partId).toBeDefined();
  return p!;
}

// --- 읽기 통로 -------------------------------------------------------------

describe('partInCanvasUnits — 부품을 캔버스 단위 의사 노드로 본다', () => {
  it('로컬 (0,0)-(5000,5000) 이 상자 안에서 (100,100)-(200,200) 이다', () => {
    const el = partInCanvasUnits(nodes(), 'grp-1', 'body')!;
    expect(el.geometry).toEqual({ x: 100, y: 100, w: 100, h: 100 });
  });

  it('id 가 **복합 키**다 — 오버레이의 네 통로가 이 키로 프레임 상태를 뒤진다', () => {
    expect(partInCanvasUnits(nodes(), 'grp-1', 'label')!.id).toBe(frameKey('grp-1', 'label'));
  });

  it('종류와 나머지 필드는 그대로 실려 간다', () => {
    const el = partInCanvasUnits(nodes(), 'grp-1', 'label')!;
    expect(el.kind).toBe('text');
    expect(el.text).toBe('T');
  });

  it('선 부품은 끝점 둘 다 원점을 얻는다', () => {
    const el = partInCanvasUnits(nodes(), 'grp-1', 'edge')!;
    expect(el.geometry).toEqual({ x1: 100, y1: 100, x2: 200, y2: 200 });
  });

  it('원본 노드를 한 글자도 건드리지 않는다 — 읽기다', () => {
    const list = nodes();
    const before = JSON.stringify(list);
    partInCanvasUnits(list, 'grp-1', 'body');
    expect(JSON.stringify(list)).toBe(before);
  });

  it('없는 그룹·없는 부품·그룹이 아닌 노드는 `undefined` 이고 예외가 아니다 (REQ-07)', () => {
    expect(partInCanvasUnits(nodes(), 'nope', 'body')).toBeUndefined();
    expect(partInCanvasUnits(nodes(), 'grp-1', 'nope')).toBeUndefined();
    expect(partInCanvasUnits(nodes(), 'a', 'body')).toBeUndefined();
  });
});

describe('findPart — 찾기의 유일한 자리', () => {
  it('그룹과 부품과 두 자리를 함께 낸다', () => {
    const list = nodes();
    const found = findPart(list, 'grp-1', 'edge')!;
    expect(found.groupIndex).toBe(1);
    expect(found.partIndex).toBe(1);
    expect(found.part.id).toBe('edge');
    // 그룹은 **참조 그대로**다 — 사본을 내면 상자를 읽는 쪽이 옛 값을 볼 수 있다.
    expect(found.group).toBe(list[1]);
  });

  it('부품 0 개 그룹에서도 예외가 아니다 (REQ-07 · AC-41)', () => {
    expect(findPart([group({ parts: [] })], 'grp-1', 'body')).toBeUndefined();
  });
});

// --- 쓰기 통로 (AC-13 ~ AC-15) ---------------------------------------------

describe('부품을 끌면 저장 좌표가 바뀐다 (AC-13)', () => {
  it('캔버스 절대 (+20,+20) 이 로컬 (+1000,+1000) 이다', () => {
    const next = patchPartGeometry(nodes(), 'grp-1', 'body', {
      x: 120,
      y: 120,
      w: 100,
      h: 100,
    });
    expect(partOf(next, 'body').geometry).toEqual({ x: 1000, y: 1000, w: HALF, h: HALF });
  });

  it('왕복한다 — 읽어서 그대로 되쓰면 저장 좌표가 그대로다', () => {
    const list = nodes();
    const read = partInCanvasUnits(list, 'grp-1', 'body')!;
    const next = patchPartGeometry(list, 'grp-1', 'body', read.geometry);
    expect(partOf(next, 'body').geometry).toEqual(partOf(list, 'body').geometry);
  });

  it('선 부품도 같은 통로를 지난다', () => {
    const next = patchPartGeometry(nodes(), 'grp-1', 'edge', {
      x1: 120,
      y1: 120,
      x2: 220,
      y2: 220,
    });
    expect(partOf(next, 'edge').geometry).toEqual({ x1: 1000, y1: 1000, x2: 6000, y2: 6000 });
  });

  it('문구 부품도 같은 통로를 지난다', () => {
    const next = patchPartGeometry(nodes(), 'grp-1', 'label', { x: 120, y: 140 });
    expect(partOf(next, 'label').geometry).toEqual({ x: 1000, y: 2000 });
  });
});

describe('그룹 상자는 바뀌지 않는다 (AC-14 · 004 A16)', () => {
  it('부품 기하를 고쳐도 `geometry` 가 한 자리도 바뀌지 않는다', () => {
    const next = patchPartGeometry(nodes(), 'grp-1', 'body', { x: 0, y: 0, w: 400, h: 400 });
    expect(groupOf(next).geometry).toEqual(BOX);
  });

  it('상자를 부품 합집합으로 다시 재지 않는다 — 밖으로 밀어도 상자는 그대로다', () => {
    const next = patchPartGeometry(nodes(), 'grp-1', 'body', {
      x: -500,
      y: -500,
      w: 100,
      h: 100,
    });
    expect(groupOf(next).geometry).toEqual(BOX);
  });
});

describe('형제는 참조 그대로다 (AC-15)', () => {
  it('새 배열이 나오고 형제 노드의 참조는 유지된다', () => {
    const list = nodes();
    const next = patchPartGeometry(list, 'grp-1', 'body', { x: 120, y: 120, w: 100, h: 100 });
    expect(next).not.toBe(list);
    expect(next[0]).toBe(list[0]);
    expect(next[2]).toBe(list[2]);
    // 그룹 자신은 갈렸다 — 그 안이 바뀌었기 때문이다.
    expect(next[1]).not.toBe(list[1]);
  });

  it('형제 **부품**의 참조도 유지된다', () => {
    const list = nodes();
    const before = groupOf(list).parts;
    const next = patchPartGeometry(list, 'grp-1', 'body', { x: 120, y: 120, w: 100, h: 100 });
    const after = groupOf(next).parts;
    expect(after[1]).toBe(before[1]);
    expect(after[2]).toBe(before[2]);
    expect(after[0]).not.toBe(before[0]);
  });

  it('바꿀 것이 없으면 **받은 배열 그 참조**를 돌려준다', () => {
    const list = nodes();
    expect(patchPartGeometry(list, 'nope', 'body', { x: 0, y: 0, w: 1, h: 1 })).toBe(list);
    expect(patchPartGeometry(list, 'grp-1', 'nope', { x: 0, y: 0, w: 1, h: 1 })).toBe(list);
    // 최상위 요소에 부품 쓰기를 시도해도 아무 일도 하지 않는다.
    expect(patchPartGeometry(list, 'a', 'body', { x: 0, y: 0, w: 1, h: 1 })).toBe(list);
  });

  it('형상이 맞지 않으면 아무 일도 하지 않는다 — 선 기하가 상자 자리에 앉지 않는다', () => {
    const list = nodes();
    // 사각형 부품에 선 기하를 준다. 갈아 끼우면 `geometry` 에 `x1/y1` 이 앉는다.
    expect(patchPartGeometry(list, 'grp-1', 'body', { x1: 0, y1: 0, x2: 1, y2: 1 })).toBe(list);
    // 문구 부품에 상자 기하를 준다.
    expect(patchPartGeometry(list, 'grp-1', 'label', { x: 0, y: 0, w: 1, h: 1 })).toBe(list);
  });
});

// --- clamp 없음 (AC-18) ----------------------------------------------------

describe('부품이 그룹 상자를 넘어도 막지 않는다 (AC-18 · REQ-03-b)', () => {
  it('상자 왼쪽 위로 밀어내면 로컬 좌표가 **음수**로 저장된다', () => {
    const next = patchPartGeometry(nodes(), 'grp-1', 'body', { x: 0, y: 0, w: 100, h: 100 });
    // (0 - 100) / 200 × 10000 = -5000
    expect(partOf(next, 'body').geometry).toEqual({ x: -HALF, y: -HALF, w: HALF, h: HALF });
  });

  it('상자 오른쪽 아래로 밀어내면 격자 상한을 **넘어** 저장된다', () => {
    const next = patchPartGeometry(nodes(), 'grp-1', 'body', { x: 400, y: 400, w: 100, h: 100 });
    // (400 - 100) / 200 × 10000 = 15000 > GROUP_LOCAL_EXTENT
    const geo = partOf(next, 'body').geometry as { x: number; y: number };
    expect(geo.x).toBeGreaterThan(GROUP_LOCAL_EXTENT);
    expect(geo.y).toBeGreaterThan(GROUP_LOCAL_EXTENT);
  });
});
