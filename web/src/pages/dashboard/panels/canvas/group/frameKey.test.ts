// 프레임 키와 2단 순회 (SPEC-CANVAS-004 M3).
//
// 이 파일이 지키는 것은 불변식 G11 의 **순수한 절반**이다 — 최상위 원소의 키가 002 와
// 바이트 동일하고, 부품만 복합 키를 얻는다. 나머지 절반(네 표면이 실제로 그 키를 쓴다)은
// `drawElement.group.test.ts` 와 `CanvasSurface.group.test.tsx` 가 잰다.
//
// @spec SPEC-CANVAS-004 REQ-03 · AC-04 · 불변식 G11

import { describe, expect, it } from 'vitest';

import type { CanvasElement } from '../canvasConfig';
import { frameKey, walkDrawables } from './frameKey';
import type { CanvasNode, GroupElement } from './groupTypes';

function rect(id: string): CanvasElement {
  return { id, kind: 'rect', style: {}, geometry: { x: 0, y: 0, w: 10, h: 10 } };
}

function group(id: string, partIds: readonly string[]): GroupElement {
  return {
    id,
    kind: 'group',
    geometry: { x: 73, y: 41, w: 317, h: 181 },
    parts: partIds.map(rect),
  };
}

describe('frameKey — 최상위는 002 와 바이트 동일하다 (G11)', () => {
  it('부품 id 가 없으면 노드 id **그대로**다', () => {
    for (const id of ['el-1', 'rect-1', 'a/b', '', 'grp-1']) {
      expect(frameKey(id)).toBe(id);
    }
  });

  it('부품 id 가 있으면 `그룹id/부품id` 다', () => {
    expect(frameKey('grp-1', 'stem')).toBe('grp-1/stem');
    expect(frameKey('grp-1', 'label')).toBe('grp-1/label');
  });

  it('같은 부품 id 라도 그룹이 다르면 다른 키다 (E-I — 교차 오염 방어)', () => {
    expect(frameKey('grp-A', 'body')).not.toBe(frameKey('grp-B', 'body'));
  });

  it('빈 문자열 부품 id 는 **부재가 아니다** — 복합 키를 낸다', () => {
    // `partId === undefined` 로 가르는 것이 요점이다. 참 판정(`partId ? ... : ...`)으로
    // 적으면 빈 문자열 부품이 최상위 키를 얻어 다른 노드와 충돌한다.
    expect(frameKey('grp-1', '')).toBe('grp-1/');
  });
});

describe('walkDrawables — 그리기 순서는 2단이다 (AC-E4)', () => {
  const nodes: CanvasNode[] = [rect('rect-A'), group('grp-1', ['p1', 'p2']), rect('rect-B')];

  it('최상위 순서 → 그룹 안 부품 순서로 훑는다', () => {
    expect([...walkDrawables(nodes)].map((d) => d.key)).toEqual([
      'rect-A',
      'grp-1/p1',
      'grp-1/p2',
      'rect-B',
    ]);
  });

  it('부품이 그룹 뒤의 최상위 요소보다 **위로 올라오지 않는다**', () => {
    const keys = [...walkDrawables(nodes)].map((d) => d.key);
    expect(keys.indexOf('grp-1/p2')).toBeLessThan(keys.indexOf('rect-B'));
  });

  it('그룹 자신은 나오지 않는다 — 그릴 도형이 없다', () => {
    expect([...walkDrawables(nodes)].map((d) => d.element.id)).toEqual([
      'rect-A',
      'p1',
      'p2',
      'rect-B',
    ]);
    expect([...walkDrawables(nodes)].every((d) => d.element.kind !== ('group' as string))).toBe(true);
  });

  it('연결선은 **아직** 나오지 않는다 — 그러나 이웃의 차례는 흔들리지 않는다', () => {
    // SPEC-CANVAS-011 M4. 연결선은 최상위 노드이지만 그릴 법이 M6 에서야 선다. 그 전까지는
    // 순회에서 빠지며, 이 단언이 그 사실을 **값으로** 붙든다 — M6 이 이 줄을 뒤집을 때
    // 그것이 의도된 변경임이 드러난다.
    //
    // 함께 재는 것이 하나 더 있다: 연결선이 섞여 있어도 **나머지의 키가 한 글자도 달라지지
    // 않는다**(G11). 키가 흔들리면 트윈 장부와 글자 폭 장부가 함께 어긋난다.
    const withConnector: CanvasNode[] = [
      rect('rect-A'),
      {
        id: 'c1',
        kind: 'connector',
        from: { el: 'rect-A', a: 'e' },
        to: { el: 'rect-B', a: 'w' },
        route: 'straight',
      },
      group('grp-1', ['p1', 'p2']),
      rect('rect-B'),
    ];
    expect([...walkDrawables(withConnector)].map((d) => d.key)).toEqual([
      'rect-A',
      'grp-1/p1',
      'grp-1/p2',
      'rect-B',
    ]);
    expect([...walkDrawables(withConnector)].some((d) => d.element.id === 'c1')).toBe(false);
  });

  it('부품 항목은 제 그룹을 들고 나오고 최상위 항목은 들지 않는다', () => {
    const visits = [...walkDrawables(nodes)];
    expect(visits[0]?.group).toBeUndefined();
    expect(visits[1]?.group?.id).toBe('grp-1');
    expect(visits[2]?.group).toBe(visits[1]?.group);
    expect(visits[3]?.group).toBeUndefined();
  });

  it('빈 그룹은 아무것도 내지 않되 순회를 끊지도 않는다 (AC-E1)', () => {
    const withEmpty: CanvasNode[] = [rect('a'), group('empty', []), rect('b')];
    expect([...walkDrawables(withEmpty)].map((d) => d.key)).toEqual(['a', 'b']);
  });

  it('그룹이 둘이어도 키가 섞이지 않는다 (E-I)', () => {
    const two: CanvasNode[] = [group('grp-A', ['body']), group('grp-B', ['body'])];
    expect([...walkDrawables(two)].map((d) => d.key)).toEqual(['grp-A/body', 'grp-B/body']);
  });

  it('그룹이 없는 배열에서는 키가 `el.id` 뿐이다 — 001/002 의 형상 그대로', () => {
    const flat: CanvasNode[] = [rect('a'), rect('b'), rect('c')];
    expect([...walkDrawables(flat)].map((d) => d.key)).toEqual(['a', 'b', 'c']);
  });
});
