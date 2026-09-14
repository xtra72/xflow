// 부품 분리 — 풀기의 부분 적용 (SPEC-CANVAS-009 M6).
//
// ## 이 파일이 지키는 것 둘
//
//   1. **분리와 풀기가 갈라지지 않는다.** 좌표 환산과 스타일 굽기를 분리가 새로 적으면
//      "풀었을 때와 하나만 뺐을 때 좌표가 1 다르다" 나 "겉모습이 다르다" 가 표현
//      가능해진다. 그래서 이 파일은 두 경로의 결과를 **서로 견준다**(AC-33).
//   2. **부품 1 개짜리 그룹을 만들지 않는다.** 004 는 그 상태를 읽기는 허용하되 만드는
//      것은 거절한다. 분리가 그 금지된 상태를 새로 지으면 004 의 거절이 우회된다.
//
// ## 고정 상자
//
// `(100,100)-(300,300)` — 한 변 200, 로컬 격자 10000 이므로 로컬 5000 이 캔버스 100 에
// 정확히 대응한다. 반올림이 개입하지 않아 기대값이 좌표 산술 자체를 잰다.
//
// @spec SPEC-CANVAS-009 REQ-05 · AC-24 ~ AC-33 · AC-42

import { describe, expect, it } from 'vitest';

import type { CanvasElement, RuleRow } from '../canvasConfig';
import { detachPart, rulesLostByDetach, rulesLostByUngroup, ungroupNode } from './groupOps';
import { GROUP_LOCAL_EXTENT, isGroup, type CanvasNode, type GroupElement } from './groupTypes';

const BOX = { x: 100, y: 100, w: 200, h: 200 } as const;

/** `nodata` 행 하나. 파서가 `value` 를 0 으로 정규화하므로 여기서도 그 형상을 따른다. */
const NODATA_ROW: RuleRow = { op: 'nodata', value: 0, patch: {} };
const HALF = GROUP_LOCAL_EXTENT / 2;

function part(id: string, over: Partial<CanvasElement> = {}): CanvasElement {
  return {
    id,
    kind: 'rect',
    geometry: { x: 0, y: 0, w: HALF, h: HALF },
    style: {},
    ...over,
  } as CanvasElement;
}

function group(partIds: readonly string[], over: Partial<GroupElement> = {}): GroupElement {
  return {
    id: 'grp-1',
    kind: 'group',
    geometry: { ...BOX },
    parts: partIds.map((id) => part(id)),
    ...over,
  };
}

function rect(id: string): CanvasElement {
  return { id, kind: 'rect', geometry: { x: 5, y: 6, w: 7, h: 8 }, style: {} };
}

/** 앞에도 뒤에도 형제가 있다 — "그룹 바로 뒤" 는 끝에 붙이기와 구별되어야 한다. */
function nodes(g: GroupElement = group(['a', 'b', 'c'])): CanvasNode[] {
  return [rect('before'), g, rect('after')];
}

function ids(list: readonly CanvasNode[]): string[] {
  return list.map((n) => n.id);
}

function groupIn(list: readonly CanvasNode[]): GroupElement | undefined {
  return list.find(isGroup);
}

// --- 올라오는 자리와 수 (AC-24 · AC-25) ------------------------------------

describe('분리한 부품이 최상위로 올라온다 (AC-24)', () => {
  it('부품 3 개 그룹에서 하나를 빼면 그룹의 부품은 2 개가 된다', () => {
    const out = detachPart(nodes(), 'grp-1', 'b');
    expect(out.refusal).toBeUndefined();
    expect(groupIn(out.nodes)!.parts.map((p) => p.id)).toEqual(['a', 'c']);
    expect(out.liftedIds).toHaveLength(1);
  });

  it('올라온 요소가 최상위 배열에 실재한다', () => {
    const out = detachPart(nodes(), 'grp-1', 'b');
    expect(ids(out.nodes)).toContain(out.liftedIds[0]);
  });

  it('부품 id 가 아니라 **새 id** 를 받는다 — 부품 id 는 그룹 안에서만 유일했다', () => {
    const out = detachPart(nodes(), 'grp-1', 'b');
    expect(out.liftedIds[0]).not.toBe('b');
    // 최상위에 이미 있는 id 와 부딪치지 않는다.
    expect(ids(out.nodes).filter((id) => id === out.liftedIds[0])).toHaveLength(1);
  });
});

describe('분리된 부품이 그룹 바로 뒤에 놓인다 (AC-25)', () => {
  it('맨 뒤가 아니라 그룹 다음 자리다 — 그림이 다른 요소 위로 튀지 않는다', () => {
    const out = detachPart(nodes(), 'grp-1', 'b');
    expect(ids(out.nodes)).toEqual(['before', 'grp-1', out.liftedIds[0]!, 'after']);
  });

  it('형제 노드는 참조 그대로 실려 간다', () => {
    const list = nodes();
    const out = detachPart(list, 'grp-1', 'b');
    expect(out.nodes[0]).toBe(list[0]);
    expect(out.nodes[3]).toBe(list[2]);
  });

  it('남은 부품의 참조는 유지된다 — 하나를 뺐다고 나머지를 다시 짓지 않는다', () => {
    const list = nodes();
    const before = (list[1] as GroupElement).parts;
    const out = detachPart(list, 'grp-1', 'b');
    const after = groupIn(out.nodes)!.parts;
    expect(after[0]).toBe(before[0]);
    expect(after[1]).toBe(before[2]);
  });
});

// --- 좌표와 겉모습 (AC-26 ~ AC-28) -----------------------------------------

describe('좌표가 절대 좌표로 환산된다 (AC-26)', () => {
  it('로컬 (0,0)-(5000,5000) 이 캔버스 (100,100)-(200,200) 이 된다 — 자리가 움직이지 않는다', () => {
    const out = detachPart(nodes(), 'grp-1', 'b');
    const lifted = out.nodes.find((n) => n.id === out.liftedIds[0])!;
    expect(lifted.geometry).toEqual({ x: 100, y: 100, w: 100, h: 100 });
  });
});

describe('겉모습이 보존된다 (AC-27 · AC-28)', () => {
  it('그룹 저술과 부품 저술이 함께 구워진다 — 구체성이 이긴다', () => {
    const g = group(['a', 'b', 'c'], { style: { fill: 'red' } });
    g.parts[1]!.style = { stroke: 'blue' };
    const out = detachPart(nodes(g), 'grp-1', 'b');
    const lifted = out.nodes.find((n) => n.id === out.liftedIds[0])!;
    expect(lifted.style).toMatchObject({ fill: 'red', stroke: 'blue' });
  });

  it('투명도는 **곱해져서** 구워진다 — 0.5 × 0.5 = 0.25', () => {
    const g = group(['a', 'b', 'c'], { style: { opacity: 0.5 } });
    g.parts[1]!.style = { opacity: 0.5 };
    const out = detachPart(nodes(g), 'grp-1', 'b');
    const lifted = out.nodes.find((n) => n.id === out.liftedIds[0])!;
    expect(lifted.style?.opacity).toBeCloseTo(0.25, 10);
  });

  it('그룹 바인딩과 트윈은 부품이 적지 않았을 때만 내려온다', () => {
    const g = group(['a', 'b', 'c'], {
      binding: { series: 's-group', agg: 'last' },
      tween: { duration_ms: 300, easing: 'linear' },
    });
    g.parts[1]!.binding = { series: 's-part', agg: 'last' };
    const out = detachPart(nodes(g), 'grp-1', 'b');
    const lifted = out.nodes.find((n) => n.id === out.liftedIds[0])!;
    expect(lifted.binding?.series).toBe('s-part');
    expect(lifted.tween?.duration_ms).toBe(300);
  });

  it('남은 그룹의 저술은 그대로다 — 하나를 빼도 그룹이 제 값을 잃지 않는다', () => {
    const g = group(['a', 'b', 'c'], { style: { fill: 'red' }, rules: [NODATA_ROW] });
    const out = detachPart(nodes(g), 'grp-1', 'b');
    expect(groupIn(out.nodes)!.style).toEqual({ fill: 'red' });
    expect(groupIn(out.nodes)!.rules).toHaveLength(1);
  });
});

// --- 그룹이 사라지는 두 갈래 (AC-29 · AC-30) -------------------------------

describe('부품 0 개가 되면 그룹이 사라진다 (AC-29)', () => {
  it('부품 1 개 그룹에서 그 하나를 분리하면 그룹 노드가 제거된다', () => {
    const out = detachPart(nodes(group(['only'])), 'grp-1', 'only');
    expect(groupIn(out.nodes)).toBeUndefined();
    expect(out.nodes).toHaveLength(3);
    expect(out.liftedIds).toHaveLength(1);
  });
});

describe('부품 1 개가 남으면 그것도 함께 올라온다 (AC-30)', () => {
  it('부품 2 개 그룹에서 하나를 분리하면 둘 다 올라오고 그룹이 사라진다', () => {
    const out = detachPart(nodes(group(['a', 'b'])), 'grp-1', 'a');
    expect(groupIn(out.nodes)).toBeUndefined();
    expect(out.liftedIds).toHaveLength(2);
    expect(ids(out.nodes)).toHaveLength(4);
  });

  it('**부품 1 개짜리 그룹을 만들지 않는다** — 004 가 거절하는 그 상태다', () => {
    for (const partIds of [['a', 'b'], ['only']]) {
      const out = detachPart(nodes(group(partIds)), 'grp-1', partIds[0]!);
      const survivor = groupIn(out.nodes);
      expect(survivor, partIds.join(',')).toBeUndefined();
    }
  });

  it('부품 3 개에서는 그룹이 살아남는다 — 경계가 2 와 3 사이다', () => {
    const out = detachPart(nodes(group(['a', 'b', 'c'])), 'grp-1', 'a');
    expect(groupIn(out.nodes)).toBeDefined();
    expect(groupIn(out.nodes)!.parts).toHaveLength(2);
  });
});

// --- 같은 함수를 쓴다 (AC-33) ----------------------------------------------

describe('분리와 풀기가 같은 함수를 쓴다 (AC-33)', () => {
  it('부품 2 개 그룹의 분리 결과가 **풀기 결과와 같다**', () => {
    const g = group(['a', 'b'], { style: { opacity: 0.5, fill: 'red' } });
    expect(detachPart(nodes(g), 'grp-1', 'a')).toEqual(ungroupNode(nodes(g), 'grp-1'));
  });

  it('≥ 3 갈래의 좌표·스타일도 풀기의 그것과 같다', () => {
    const g = group(['a', 'b', 'c'], { style: { fill: 'red', opacity: 0.5 } });
    g.parts[1]!.style = { stroke: 'blue', opacity: 0.5 };

    const detached = detachPart(nodes(g), 'grp-1', 'b');
    const lifted = detached.nodes.find((n) => n.id === detached.liftedIds[0])!;

    const ungrouped = ungroupNode(nodes(g), 'grp-1');
    // 풀기가 낸 것들 가운데 같은 부품에서 나온 것(둘째)과 견준다.
    const counterpart = ungrouped.nodes[2]!;

    expect(lifted.geometry).toEqual(counterpart.geometry);
    expect(lifted.style).toEqual(counterpart.style);
    expect(lifted.kind).toBe(counterpart.kind);
  });
});

// --- 규칙 손실 안내 (AC-31) ------------------------------------------------

describe('규칙 손실을 알린다 (AC-31 · REQ-05-c)', () => {
  it('잃게 될 규칙 수가 `rulesLostByUngroup` 의 값과 일치한다', () => {
    const rules: RuleRow[] = [{ op: 'gt', value: 1, patch: {} }, NODATA_ROW];
    const g = group(['a', 'b', 'c'], { rules });
    const list = nodes(g);
    expect(rulesLostByDetach(list, 'grp-1', 'b')).toBe(rulesLostByUngroup(g));
    expect(rulesLostByDetach(list, 'grp-1', 'b')).toBe(2);
  });

  it('규칙이 없으면 0 이다 — 잃을 것이 없는 분리에는 확인을 붙이지 않는다', () => {
    expect(rulesLostByDetach(nodes(), 'grp-1', 'b')).toBe(0);
  });

  it('없는 그룹·없는 부품이면 0 이다', () => {
    const list = nodes(group(['a', 'b', 'c'], { rules: [NODATA_ROW] }));
    expect(rulesLostByDetach(list, 'nope', 'b')).toBe(0);
    expect(rulesLostByDetach(list, 'grp-1', 'nope')).toBe(0);
    expect(rulesLostByDetach(list, 'before', 'b')).toBe(0);
  });
});

// --- 거절 (AC-32 · AC-42) --------------------------------------------------

describe('분리가 아무 일도 하지 않는 자리 (AC-32 · AC-42)', () => {
  it('최상위 요소를 대상으로 하면 배열이 그대로다 — **같은 참조**다', () => {
    const list = nodes();
    const out = detachPart(list, 'before', 'anything');
    expect(out.nodes).toBe(list);
    expect(out.refusal).toBe('notGroup');
    expect(out.liftedIds).toEqual([]);
  });

  it('없는 그룹이면 배열이 그대로 돌아온다', () => {
    const list = nodes();
    expect(detachPart(list, 'nope', 'b').nodes).toBe(list);
  });

  it('없는 부품이면 배열이 그대로 돌아온다', () => {
    const list = nodes();
    expect(detachPart(list, 'grp-1', 'nope').nodes).toBe(list);
  });

  it('부품 0 개 그룹이어도 예외가 아니다 (AC-41)', () => {
    const list = nodes(group([]));
    expect(detachPart(list, 'grp-1', 'anything').nodes).toBe(list);
  });
});
