// 묶기와 풀기 (SPEC-CANVAS-004 M5).
//
// **이 목표의 하중이 여기 있다.** 묶기의 가장 쓰기 쉬운 고정 입력은 거의 전부 **항등**이다
// (acceptance.md §시험 규율): 부품 하나면 합집합이 곧 그 부품이고, 부품이 전부 같은 상자면
// 델타가 전부 0 이며, 그룹이 원점에 있으면 빼는 수가 0 이고, `opacity` 가 전부 1 이면
// 곱셈이 항등이다. 그 넷으로만 만든 고정 입력은 004 의 산술을 하나도 시험하지 않는다.
//
// 그래서 이 파일의 고정 입력은 다음을 **전부** 갖춘다:
//   - **E-A** 고른 것 셋, 상자가 서로 다르고 겹치지 않으며, 하나(`t`)는 합집합 상자의
//     가장자리에 닿지 않는다. 그리고 **하나만 고른 경우를 따로 두어 거절을 단언한다.**
//   - **E-B** 부품마다 다른 상자. 묶은 뒤 **실제로 움직였음**을 값으로 단언한다.
//   - **E-C** 합집합 상자 `317 × 181` — 둘 다 10000 을 나누어떨어뜨리지 않는다.
//   - **E-D** 원점 `(73, 41)` — 0 이 아니고 두 축이 다르다. **음수 원점도 따로 둔다.**
//   - **E-F** `opacity` 네 조합 전부. (없음, 없음)은 **키가 없음**을 단언한다.
//   - **E-G** 왕복을 **양쪽 다** 단언한다 — 상자변 ≤ 10 000 에서 정확, 100 000 에서
//     정확하지 않되 그 차이가 상한(5 단위) 안.
//   - **E-H** 가로선 둘만 묶은 퇴화 상자. **파서 왕복 뒤에도** 부품이 모서리에 있지 않다.
//
// @spec SPEC-CANVAS-004 REQ-07 · AC-11 · AC-12 · AC-13 · AC-14 · AC-E10

import { describe, expect, it } from 'vitest';

import {
  MIN_ELEMENT_EXTENT,
  parseCanvasConfig,
  type CanvasElement,
  type ElementStyle,
  type LineGeometry,
  type PathElement,
  type RuleRow,
} from '../canvasConfig';
import {
  bakeStyle,
  groupNodes,
  rulesLostByUngroup,
  ungroupNode,
} from './groupOps';
import { GROUP_LOCAL_EXTENT, isGroup, type CanvasNode, type GroupElement } from './groupTypes';

// --- 고정 입력 -------------------------------------------------------------

/**
 * 고를 셋. 합집합이 정확히 `{ x: 73, y: 41, w: 317, h: 181 }` 이 되도록 놓았다.
 *
 *   a  rect  73,41,100,60   — 왼쪽·위 **가장자리에 닿는다**
 *   t  text  200,120        — **안쪽에 떠 있다**(넓이 0 인 문구 — `elementsBounds` 의 규칙)
 *   p  path  290,150,100,72 — 오른쪽·아래 **가장자리에 닿는다**
 *
 * 셋의 상자가 서로 다르고 겹치지 않는다(E-A · E-B).
 */
const A: CanvasElement = {
  id: 'a',
  kind: 'rect',
  geometry: { x: 73, y: 41, w: 100, h: 60 },
  style: { fill: '#111' },
};

const T: CanvasElement = {
  id: 't',
  kind: 'text',
  geometry: { x: 200, y: 120 },
  style: { textColor: '#222' },
  text: '{value}',
};

const PATH_COMMANDS: PathElement['path'] = [
  { c: 'M', x: 0, y: 0 },
  { c: 'L', x: 10000, y: 4200 },
  { c: 'C', x1: 900, y1: 800, x2: 700, y2: 600, x: 500, y: 400 },
  { c: 'Z' },
];

const P: CanvasElement = {
  id: 'p',
  kind: 'path',
  geometry: { x: 290, y: 150, w: 100, h: 72 },
  path: PATH_COMMANDS.map((c) => ({ ...c })) as PathElement['path'],
  style: { stroke: '#333' },
};

/** 고르지 **않는** 둘. 기하가 한 자리도 바뀌지 않아야 한다. */
const KEEP_FRONT: CanvasElement = {
  id: 'keep-front',
  kind: 'rect',
  geometry: { x: 5, y: 7, w: 11, h: 13 },
  style: {},
};
const KEEP_BACK: CanvasElement = {
  id: 'keep-back',
  kind: 'ellipse',
  geometry: { x: 400, y: 300, w: 20, h: 30 },
  style: {},
};

/** 배열 순서: 앞이 아래. 고른 셋 가운데 **가장 뒤**는 `p` 다(index 3). */
function scene(): CanvasNode[] {
  return [KEEP_FRONT, A, T, P, KEEP_BACK].map((el) => structuredClone(el));
}

const pick = (...ids: string[]): ReadonlySet<string> => new Set(ids);

function onlyGroup(nodes: readonly CanvasNode[]): GroupElement {
  const found = nodes.find(isGroup);
  expect(found, '그룹이 만들어져야 한다').toBeDefined();
  if (found === undefined) throw new Error('그룹이 없다');
  return found;
}

function partById(g: GroupElement, id: string): CanvasElement {
  const found = g.parts.find((p) => p.id === id);
  expect(found, `부품 ${id}`).toBeDefined();
  if (found === undefined) throw new Error('부품이 없다');
  return found;
}

// --- ① 묶기 (AC-11) --------------------------------------------------------

describe('고른 것들을 묶는다 (AC-11)', () => {
  it('그룹 하나가 생기고 고른 셋이 최상위에서 사라진다', () => {
    const out = groupNodes(scene(), pick('a', 't', 'p'));
    expect(out.refusal).toBeUndefined();
    expect(out.nodes.map((n) => n.id)).toEqual(['keep-front', out.groupId, 'keep-back']);
    expect(out.nodes.filter(isGroup)).toHaveLength(1);
  });

  it('그룹 상자가 합집합이다 — `{ 73, 41, 317, 181 }`', () => {
    const g = onlyGroup(groupNodes(scene(), pick('a', 't', 'p')).nodes);
    expect(g.geometry).toEqual({ x: 73, y: 41, w: 317, h: 181 });
  });

  it('부품 기하가 그룹 로컬 정수 격자로 옮겨졌다 (E-B — 실제로 움직였다)', () => {
    const g = onlyGroup(groupNodes(scene(), pick('a', 't', 'p')).nodes);
    // 리터럴 값으로 못박는다. `round((abs - origin) / span × 10000)` 의 결과다.
    expect(partById(g, 'a').geometry).toEqual({ x: 0, y: 0, w: 3155, h: 3315 });
    expect(partById(g, 't').geometry).toEqual({ x: 4006, y: 4365 });
    expect(partById(g, 'p').geometry).toEqual({ x: 6845, y: 6022, w: 3155, h: 3978 });
    // "옮겨졌다" 만 재면 "애초에 같아서 아무 일도 없었다" 와 구분되지 않는다.
    expect(partById(g, 'a').geometry).not.toEqual(A.geometry);
    expect(partById(g, 't').geometry).not.toEqual(T.geometry);
    expect(partById(g, 'p').geometry).not.toEqual(P.geometry);
  });

  it('가장자리에 닿지 않는 부품의 로컬 좌표가 **0 도 EXTENT 도 아니다** (E-A)', () => {
    const g = onlyGroup(groupNodes(scene(), pick('a', 't', 'p')).nodes);
    const t = partById(g, 't').geometry;
    expect('x' in t && 'y' in t).toBe(true);
    for (const v of Object.values(t)) {
      expect(v).not.toBe(0);
      expect(v).not.toBe(GROUP_LOCAL_EXTENT);
    }
  });

  it('경로 부품의 `path` 명령은 **한 글자도 바뀌지 않았다**', () => {
    // 경로 명령은 **제 상자 로컬**이라 그룹과 무관하다(008 불변식 J2 와 같은 자리).
    const g = onlyGroup(groupNodes(scene(), pick('a', 't', 'p')).nodes);
    const part = partById(g, 'p');
    expect(part.kind).toBe('path');
    expect((part as PathElement).path).toEqual(PATH_COMMANDS);
  });

  it('그룹이 **고른 것들 가운데 가장 뒤**의 자리에 선다 — z-order 가 튀지 않는다', () => {
    const out = groupNodes(scene(), pick('a', 't', 'p'));
    // 원본에서 `p` 는 `keep-back` 바로 앞이었다. 그룹도 그 자리다.
    expect(out.nodes.map((n) => n.id).indexOf(out.groupId!)).toBe(1);
    expect(out.nodes[2]?.id).toBe('keep-back');
  });

  it('가장 뒤가 배열 끝이 아니어도 그 자리를 지킨다', () => {
    // `keep-front` 와 `a` 만 묶으면 그룹은 `a` 의 자리(index 1)에 선다.
    const out = groupNodes(scene(), pick('keep-front', 'a'));
    expect(out.nodes.map((n) => n.id)).toEqual([out.groupId, 't', 'p', 'keep-back']);
  });

  it('**띄엄띄엄 고른** 경우에도 가장 뒤의 자리다 — 붙어 있는 선택은 이 규칙을 재지 못한다', () => {
    // **잇달아 붙은 선택으로는 이 성질이 관측되지 않는다.** 고른 것이 연속이면 첫 번째
    // 자리에 끼우든 마지막 자리에 끼우든 **같은 배열**이 나오기 때문이다 — 묶기에서
    // "합집합이 곧 그 부품" 과 같은 부류의 숨은 기본값이다.
    //
    // `a`(1) 와 `p`(3) 만 고르면 그 사이에 `t`(2) 가 남는다. 첫 자리에 끼우는 구현은
    // 그룹을 `t` **아래**로 내려보내고, 그것이 곧 "묶었더니 그림이 뒤로 튀었다" 다.
    const out = groupNodes(scene(), pick('a', 'p'));
    expect(out.nodes.map((n) => n.id)).toEqual(['keep-front', 't', out.groupId, 'keep-back']);
    // 원래 `p` 는 `t` 위였다. 그룹도 `t` 위다.
    expect(out.nodes.map((n) => n.id).indexOf(out.groupId!)).toBeGreaterThan(
      out.nodes.map((n) => n.id).indexOf('t'),
    );
  });

  it('띄엄띄엄 골라도 부품 순서는 **배열 순서**를 따른다', () => {
    const g = onlyGroup(groupNodes(scene(), pick('p', 'a')).nodes);
    expect(g.parts.map((part) => part.id)).toEqual(['a', 'p']);
  });

  it('남은 최상위 요소 둘의 기하는 한 자리도 바뀌지 않았다', () => {
    const out = groupNodes(scene(), pick('a', 't', 'p'));
    expect(out.nodes[0]).toEqual(KEEP_FRONT);
    expect(out.nodes[2]).toEqual(KEEP_BACK);
  });

  it('선택으로 세울 것은 **새 그룹 하나**다 — id 가 기존과 충돌하지 않는다', () => {
    const out = groupNodes(scene(), pick('a', 't', 'p'));
    expect(out.groupId).toBeDefined();
    expect(scene().map((n) => n.id)).not.toContain(out.groupId);
  });

  it('입력 배열을 **바꾸지 않는다** — 순수 함수다', () => {
    const nodes = scene();
    const snapshot = structuredClone(nodes);
    groupNodes(nodes, pick('a', 't', 'p'));
    expect(nodes).toEqual(snapshot);
  });

  it('부품은 원본과 **다른 객체**다 — 사본을 고쳐도 원본이 따라 바뀌지 않는다', () => {
    const nodes = scene();
    const g = onlyGroup(groupNodes(nodes, pick('a', 't', 'p')).nodes);
    expect(partById(g, 'a')).not.toBe(nodes[1]);
  });

  it('`style` · `text` 는 그대로 실려 간다 — 묶기는 겉모습을 건드리지 않는다', () => {
    const g = onlyGroup(groupNodes(scene(), pick('a', 't', 'p')).nodes);
    expect(partById(g, 'a').style).toEqual({ fill: '#111' });
    expect(partById(g, 't').text).toBe('{value}');
    // 그룹 자신은 겉모습을 갖지 않는다(묶기가 만들어 내지 않는다).
    expect(g.style).toBeUndefined();
    expect(g.rules).toBeUndefined();
    expect(g.binding).toBeUndefined();
  });

  it('**음수 원점**에서도 원점이 빠진다 (E-D — 캔버스 밖 저술은 합법이다)', () => {
    const left: CanvasElement = { ...A, id: 'a', geometry: { x: -120, y: -37, w: 100, h: 60 } };
    const right: CanvasElement = { ...A, id: 'b', geometry: { x: 100, y: 100, w: 97, h: 107 } };
    const out = groupNodes([left, right], pick('a', 'b'));
    const g = onlyGroup(out.nodes);
    expect(g.geometry).toEqual({ x: -120, y: -37, w: 317, h: 244 });
    // 원점을 빼지 않았다면 `a` 의 로컬이 음수로 남는다.
    expect(partById(g, 'a').geometry).toMatchObject({ x: 0, y: 0 });
    expect(partById(g, 'b').geometry).toMatchObject({ x: 6940, y: 5615 });
  });

  it('선 부품의 끝점 **둘 다** 원점을 잃는다', () => {
    const l1: CanvasElement = {
      id: 'l1',
      kind: 'line',
      geometry: { x1: 73, y1: 41, x2: 173, y2: 101 },
      style: {},
    };
    const l2: CanvasElement = {
      id: 'l2',
      kind: 'line',
      geometry: { x1: 290, y1: 150, x2: 390, y2: 222 },
      style: {},
    };
    const g = onlyGroup(groupNodes([l1, l2], pick('l1', 'l2')).nodes);
    expect(g.geometry).toEqual({ x: 73, y: 41, w: 317, h: 181 });
    expect(partById(g, 'l1').geometry).toEqual({ x1: 0, y1: 0, x2: 3155, y2: 3315 });
    expect(partById(g, 'l2').geometry).toEqual({
      x1: 6845,
      y1: 6022,
      x2: GROUP_LOCAL_EXTENT,
      y2: GROUP_LOCAL_EXTENT,
    });
  });
});

// --- ② 묶기의 거절 둘 (AC-12) ----------------------------------------------

describe('묶기의 거절 둘 (AC-12 · A6)', () => {
  it('하나만 고르면 거절한다 — 부품 하나짜리 그룹은 만들지 않는다', () => {
    const nodes = scene();
    const out = groupNodes(nodes, pick('a'));
    expect(out.refusal).toBe('tooFew');
    expect(out.groupId).toBeUndefined();
    // **같은 참조**를 돌려준다 — "한 바이트도 바뀌지 않았다" 를 `toBe` 하나로 잰다.
    expect(out.nodes).toBe(nodes);
  });

  it('아무것도 고르지 않아도 거절이다', () => {
    const nodes = scene();
    expect(groupNodes(nodes, pick()).refusal).toBe('tooFew');
    expect(groupNodes(nodes, pick()).nodes).toBe(nodes);
  });

  it('없는 id 를 고르면 세어지지 않는다 — 유령 선택이 그룹을 만들지 않는다', () => {
    const nodes = scene();
    const out = groupNodes(nodes, pick('a', 'ghost'));
    expect(out.refusal).toBe('tooFew');
    expect(out.nodes).toBe(nodes);
  });

  it('선택에 그룹이 섞이면 거절한다 — 중첩도 평평하게 펴기도 아니다', () => {
    const grouped = groupNodes(scene(), pick('a', 't')).nodes;
    const gid = onlyGroup(grouped).id;
    const out = groupNodes(grouped, pick(gid, 'p'));
    expect(out.refusal).toBe('nested');
    expect(out.nodes).toBe(grouped);
    // 그룹 안에 그룹이 들어간 상태가 만들어지지 않았다.
    expect(onlyGroup(out.nodes).parts.some((p) => (p as { kind: string }).kind === 'group')).toBe(
      false,
    );
    // 조용히 평평하게 펴지지도 않았다 — 그룹이 여전히 하나다.
    expect(out.nodes.filter(isGroup)).toHaveLength(1);
  });

  it('그룹 하나만 고른 경우의 사유는 `nested` 다 — 고칠 것이 "더 고르라" 가 아니다', () => {
    const grouped = groupNodes(scene(), pick('a', 't')).nodes;
    const gid = onlyGroup(grouped).id;
    expect(groupNodes(grouped, pick(gid)).refusal).toBe('nested');
  });
});

// --- ③ 퇴화 상자 (AC-E10 · E-H) ---------------------------------------------

describe('가로선 둘을 묶어도 부품이 모서리에 찌부러지지 않는다 (AC-E10)', () => {
  const FLAT_Y = 50;
  const l1: CanvasElement = {
    id: 'l1',
    kind: 'line',
    geometry: { x1: 10, y1: FLAT_Y, x2: 60, y2: FLAT_Y },
    style: {},
  };
  const l2: CanvasElement = {
    id: 'l2',
    kind: 'line',
    geometry: { x1: 80, y1: FLAT_Y, x2: 140, y2: FLAT_Y },
    style: {},
  };

  const flatGroup = (): GroupElement => onlyGroup(groupNodes([l1, l2], pick('l1', 'l2')).nodes);

  function assertNotCrushed(g: GroupElement): void {
    // (a) 상자 두 변이 최소 크기 이상이다.
    expect(g.geometry.w).toBeGreaterThanOrEqual(MIN_ELEMENT_EXTENT);
    expect(g.geometry.h).toBeGreaterThanOrEqual(MIN_ELEMENT_EXTENT);
    // 넓히기가 **가운데를 지킨다** — 세로 중심이 원래 y 그대로다.
    expect(g.geometry.y + g.geometry.h / 2).toBe(FLAT_Y);
    // (b) 두 부품의 로컬 y 가 상자 **가운데**를 지난다(모서리가 아니다).
    for (const id of ['l1', 'l2']) {
      const geo = partById(g, id).geometry as LineGeometry;
      expect(geo.y1).toBe(GROUP_LOCAL_EXTENT / 2);
      expect(geo.y2).toBe(GROUP_LOCAL_EXTENT / 2);
      // 좌표에 ±Infinity 도 NaN 도 섞이지 않았다.
      for (const v of Object.values(geo)) expect(Number.isFinite(v)).toBe(true);
    }
  }

  it('묶은 직후', () => {
    assertNotCrushed(flatGroup());
  });

  it('**파서 왕복(JSON → parseCanvasConfig) 뒤에도** 그대로다', () => {
    // 이 결함은 예외를 내지 않고 저장 왕복을 견디며 화면으로만 드러나는 부류다.
    const raw = JSON.parse(
      JSON.stringify({
        channel_name: '',
        canvas: { width: 500, height: 400 },
        elements: [flatGroup()],
      }),
    ) as unknown;
    const parsed = parseCanvasConfig(raw);
    assertNotCrushed(onlyGroup(parsed.elements));
  });

  it('가로 폭은 멀쩡하므로 넓히지 않는다 — 넓히기가 성한 축을 건드리지 않는다', () => {
    const g = flatGroup();
    expect(g.geometry.x).toBe(10);
    expect(g.geometry.w).toBe(130);
  });

  it('푸는 쪽도 되살린다 — 두 선의 y 가 원래 자리로 돌아온다', () => {
    const grouped = groupNodes([l1, l2], pick('l1', 'l2')).nodes;
    const back = ungroupNode(grouped, onlyGroup(grouped).id).nodes;
    for (const el of back) {
      const geo = el.geometry as LineGeometry;
      expect(geo.y1).toBe(FLAT_Y);
      expect(geo.y2).toBe(FLAT_Y);
    }
  });
});

// --- ④ 겉모습 접기 (AC-13 · E-F) -------------------------------------------

describe('bakeStyle — 2단이고 `opacity` 만 곱셈이다 (E-F)', () => {
  it('부품이 적은 것이 그룹이 적은 것을 덮는다(속성별)', () => {
    expect(bakeStyle({ fill: '#g', stroke: '#gs' }, { fill: '#p' })).toEqual({
      fill: '#p',
      stroke: '#gs',
    });
  });

  it('`opacity` 는 **곱셈**이다 — 0.3 × 0.9 = 0.27 (0.3 도 0.9 도 아니다)', () => {
    const baked = bakeStyle({ opacity: 0.3 }, { opacity: 0.9 });
    expect(baked.opacity).toBeCloseTo(0.27, 10);
    expect(baked.opacity).not.toBe(0.3);
    expect(baked.opacity).not.toBe(0.9);
  });

  it('한쪽만 있으면 그 값이다 — 그러나 **키는 생긴다**', () => {
    expect(bakeStyle({ opacity: 0.3 }, {}).opacity).toBe(0.3);
    expect(bakeStyle({}, { opacity: 0.9 }).opacity).toBe(0.9);
  });

  it('**양쪽 미지정이면 `opacity` 키가 없다** — 값이 1 이 아니라 키가 없다', () => {
    const baked = bakeStyle({ fill: '#g' }, { stroke: '#p' });
    expect('opacity' in baked).toBe(false);
    expect(Object.keys(baked).sort()).toEqual(['fill', 'stroke']);
  });

  it('그룹 스타일이 아예 없어도 같다 — 부품 스타일이 그대로다', () => {
    expect(bakeStyle(undefined, { fill: '#p' })).toEqual({ fill: '#p' });
    expect('opacity' in bakeStyle(undefined, {})).toBe(false);
  });

  it('결과를 0..1 로 죈다 — 손상 값이 부품에 눌러앉지 않는다', () => {
    expect(bakeStyle({ opacity: 5 }, { opacity: 5 }).opacity).toBe(1);
    expect(bakeStyle({ opacity: -1 }, {}).opacity).toBe(0);
    expect(Number.isFinite(bakeStyle({ opacity: Number.NaN }, { opacity: 0.5 }).opacity!)).toBe(
      true,
    );
  });

  it('부품이 든 명시적 `undefined` 가 그룹 값을 지우지 않는다', () => {
    const partStyle = { fill: undefined } as unknown as ElementStyle;
    expect(bakeStyle({ fill: '#g' }, partStyle).fill).toBe('#g');
  });
});

// --- ⑤ 풀기 (AC-13) --------------------------------------------------------

describe('그룹을 푼다 (AC-13 · A19)', () => {
  const RULES: RuleRow[] = [
    { op: 'lt', value: 10, patch: { fill: '#low' } },
    { op: 'nodata', value: 0, patch: { visible: false } },
  ];

  function dressed(): CanvasNode[] {
    const grouped = groupNodes(scene(), pick('a', 't', 'p')).nodes;
    return grouped.map((n): CanvasNode =>
      isGroup(n)
        ? {
            ...n,
            style: { stroke: '#000', opacity: 0.3 },
            binding: { series: 's1', agg: 'last' },
            rules: RULES,
            tween: { duration_ms: 120, easing: 'linear' },
          }
        : n,
    );
  }

  /** 목록 속 그룹 하나만 갈아 끼운다 — 타입을 `GroupElement` 로 못박아 넓어지지 않게 한다. */
  function editGroup(nodes: readonly CanvasNode[], edit: (g: GroupElement) => GroupElement): CanvasNode[] {
    return nodes.map((n): CanvasNode => (isGroup(n) ? edit(n) : n));
  }

  /** 부품 하나만 갈아 끼운다. */
  function editPart(
    g: GroupElement,
    id: string,
    edit: (p: CanvasElement) => CanvasElement,
  ): GroupElement {
    return { ...g, parts: g.parts.map((p) => (p.id === id ? edit(p) : p)) };
  }

  it('부품 셋이 그룹이 있던 **배열 자리에 순서대로** 올라온다', () => {
    const nodes = dressed();
    const out = ungroupNode(nodes, onlyGroup(nodes).id);
    expect(out.refusal).toBeUndefined();
    expect(out.nodes).toHaveLength(5);
    expect(out.nodes[0]?.id).toBe('keep-front');
    expect(out.nodes[4]?.id).toBe('keep-back');
    expect(out.nodes.some(isGroup)).toBe(false);
    // 부품 순서가 보존된다 — `a` · `t` · `p` 순이다.
    expect(out.nodes.slice(1, 4).map((n) => n.kind)).toEqual(['rect', 'text', 'path']);
  });

  it('기하가 **정수 캔버스 좌표**로 되돌아온다', () => {
    const nodes = dressed();
    const out = ungroupNode(nodes, onlyGroup(nodes).id);
    expect(out.nodes[1]?.geometry).toEqual({ x: 73, y: 41, w: 100, h: 60 });
    expect(out.nodes[2]?.geometry).toEqual({ x: 200, y: 120 });
    expect(out.nodes[3]?.geometry).toEqual({ x: 290, y: 150, w: 100, h: 72 });
  });

  it('부품 id 가 최상위에서 유일하도록 **새로 발급**된다', () => {
    const nodes = dressed();
    const out = ungroupNode(nodes, onlyGroup(nodes).id);
    const ids = out.nodes.map((n) => n.id);
    expect(new Set(ids).size).toBe(ids.length);
    // 발급이 **자라는 배열**을 본다 — 고정된 원본을 보면 셋이 전부 같은 id 를 받는다.
    expect(new Set(out.liftedIds).size).toBe(3);
    expect(out.liftedIds).toEqual(ids.slice(1, 4));
  });

  it('자기 스타일을 저술한 부품의 `opacity` 가 **0.27** 로 베이킹된다 (0.3 × 0.9)', () => {
    const nodes = editGroup(dressed(), (g) =>
      editPart(g, 'a', (p) => ({ ...p, style: { fill: '#00f', opacity: 0.9 } })),
    );
    const out = ungroupNode(nodes, onlyGroup(nodes).id);
    expect(out.nodes[1]?.style?.opacity).toBeCloseTo(0.27, 10);
    expect(out.nodes[1]?.style?.fill).toBe('#00f');
    // 스타일을 저술하지 않은 쪽은 그룹 값 그대로다.
    expect(out.nodes[2]?.style?.opacity).toBe(0.3);
    expect(out.nodes[2]?.style?.stroke).toBe('#000');
  });

  it('그룹의 바인딩과 트윈이 **자기 것이 없던 부품에만** 베이킹된다', () => {
    const own = { series: 's-own', agg: 'last' as const };
    const nodes = editGroup(dressed(), (g) =>
      editPart(g, 'a', (p) => ({
        ...p,
        binding: own,
        tween: { duration_ms: 7, easing: 'linear' as const },
      })),
    );
    const out = ungroupNode(nodes, onlyGroup(nodes).id);
    expect(out.nodes[1]?.binding).toEqual(own);
    expect(out.nodes[1]?.tween).toEqual({ duration_ms: 7, easing: 'linear' });
    expect(out.nodes[2]?.binding).toEqual({ series: 's1', agg: 'last' });
    expect(out.nodes[1]?.binding?.series).toBe('s-own');
    expect(out.nodes[2]?.tween).toEqual({ duration_ms: 120, easing: 'linear' });
  });

  it('**`group.rules` 는 어느 부품에도 복사되지 않는다** (A19)', () => {
    const nodes = dressed();
    const out = ungroupNode(nodes, onlyGroup(nodes).id);
    for (const el of out.nodes) expect(el.rules).toBeUndefined();
  });

  it('부품이 제 `rules` 를 갖고 있었다면 그것은 살아남는다 — 버리는 것은 그룹의 표다', () => {
    const nodes = editGroup(dressed(), (g) => editPart(g, 't', (p) => ({ ...p, rules: RULES })));
    const out = ungroupNode(nodes, onlyGroup(nodes).id);
    expect(out.nodes[2]?.rules).toEqual(RULES);
    expect(out.nodes[1]?.rules).toBeUndefined();
  });

  it('풀린 부품 **전부**가 선택으로 설 수 있다 — `liftedIds` 가 셋이다', () => {
    const nodes = dressed();
    const out = ungroupNode(nodes, onlyGroup(nodes).id);
    expect(out.liftedIds).toHaveLength(3);
    for (const id of out.liftedIds) expect(out.nodes.some((n) => n.id === id)).toBe(true);
  });

  it('그룹 전용 필드가 부품에 **흘러 들어가지 않는다** (스프레드가 초과 속성 검사를 우회한다)', () => {
    const nodes = editGroup(dressed(), (g) => ({ ...g, symbol: { catalog_id: 'v', version: '1' } }));
    const out = ungroupNode(nodes, onlyGroup(nodes).id);
    for (const el of out.nodes.slice(1, 4)) {
      const leaky = el as unknown as Record<string, unknown>;
      expect(leaky.parts).toBeUndefined();
      expect(leaky.symbol).toBeUndefined();
      expect(leaky.kind).not.toBe('group');
    }
  });

  it('입력 배열을 바꾸지 않는다 — 순수 함수다', () => {
    const nodes = dressed();
    const snapshot = structuredClone(nodes);
    ungroupNode(nodes, onlyGroup(nodes).id);
    expect(nodes).toEqual(snapshot);
  });

  it('그룹이 아닌 id 나 없는 id 는 거절이고 **같은 참조**를 돌려준다', () => {
    const nodes = dressed();
    for (const id of ['keep-front', 'nope']) {
      const out = ungroupNode(nodes, id);
      expect(out.refusal).toBe('notGroup');
      expect(out.nodes).toBe(nodes);
      expect(out.liftedIds).toHaveLength(0);
    }
  });
});

// --- ⑥ 풀기가 버리는 것을 화면이 말할 수 있다 (REQ-07 · A19) ----------------

describe('rulesLostByUngroup — 풀기 전에 말할 사실 (A19)', () => {
  it('그룹의 규칙 행 수를 낸다', () => {
    const g: GroupElement = {
      id: 'g',
      kind: 'group',
      geometry: { x: 0, y: 0, w: 10, h: 10 },
      parts: [],
      rules: [
        { op: 'lt', value: 1, patch: {} },
        { op: 'gt', value: 2, patch: {} },
      ],
    };
    expect(rulesLostByUngroup(g)).toBe(2);
  });

  it('규칙이 없으면 0 이다 — 잃을 것이 없으면 안내도 없다', () => {
    const g: GroupElement = {
      id: 'g',
      kind: 'group',
      geometry: { x: 0, y: 0, w: 10, h: 10 },
      parts: [],
    };
    expect(rulesLostByUngroup(g)).toBe(0);
    expect(rulesLostByUngroup({ ...g, rules: [] })).toBe(0);
  });

  it('그룹이 아니거나 없으면 0 이다 — 요소의 규칙 표는 풀기가 건드리지 않는다', () => {
    expect(rulesLostByUngroup(undefined)).toBe(0);
    expect(
      rulesLostByUngroup({ ...A, rules: [{ op: 'lt', value: 1, patch: {} }] } as CanvasNode),
    ).toBe(0);
  });

  it('그 수가 실제로 버려지는 수와 **같다** — 안내와 동작이 갈라지지 않는다', () => {
    const grouped = groupNodes(scene(), pick('a', 't')).nodes;
    const withRules = grouped.map((n): CanvasNode =>
      isGroup(n) ? { ...n, rules: [{ op: 'lt', value: 1, patch: {} }] } : n,
    );
    const g = onlyGroup(withRules);
    expect(rulesLostByUngroup(g)).toBe(1);
    const out = ungroupNode(withRules, g.id);
    expect(out.nodes.every((n) => n.rules === undefined)).toBe(true);
  });
});

// --- ⑦ 왕복과 그 상한 (AC-14 · E-G) ----------------------------------------

describe('묶기 → 풀기 왕복과 그 상한 (AC-14 · E-G)', () => {
  it('상자변이 10 000 이하면 기하가 **정수까지 원본과 정확히 같다**', () => {
    const nodes = scene();
    const grouped = groupNodes(nodes, pick('a', 't', 'p')).nodes;
    const back = ungroupNode(grouped, onlyGroup(grouped).id).nodes;
    expect(back[1]?.geometry).toEqual(A.geometry);
    expect(back[2]?.geometry).toEqual(T.geometry);
    expect(back[3]?.geometry).toEqual(P.geometry);
    // 배열 순서도 원본과 같다.
    expect(back.map((n) => n.kind)).toEqual(['rect', 'rect', 'text', 'path', 'ellipse']);
  });

  it('경로 명령과 스타일도 왕복을 지난다', () => {
    const grouped = groupNodes(scene(), pick('a', 't', 'p')).nodes;
    const back = ungroupNode(grouped, onlyGroup(grouped).id).nodes;
    expect((back[3] as PathElement).path).toEqual(PATH_COMMANDS);
    expect(back[1]?.style).toEqual({ fill: '#111' });
  });

  it('상자변이 **100 000** 이면 같지 **않고**, 차이가 상한(5 단위) 안이다', () => {
    // 성립하는 쪽만 재면 상한을 넘긴 구현도 통과한다. 성립하지 **않는** 쪽이 상한을 잰다.
    const wide: CanvasElement[] = [
      { id: 'w1', kind: 'rect', geometry: { x: 0, y: 0, w: 10, h: 10 }, style: {} },
      { id: 'w2', kind: 'rect', geometry: { x: 33333, y: 20, w: 7, h: 9 }, style: {} },
      { id: 'w3', kind: 'rect', geometry: { x: 99990, y: 0, w: 10, h: 30 }, style: {} },
    ];
    const grouped = groupNodes(wide, pick('w1', 'w2', 'w3')).nodes;
    const g = onlyGroup(grouped);
    expect(g.geometry.w).toBe(100000);

    const back = ungroupNode(grouped, g.id).nodes;
    const bound = g.geometry.w / (2 * GROUP_LOCAL_EXTENT);
    expect(bound).toBe(5);

    // ① 같지 않다.
    expect(back.map((n) => n.geometry)).not.toEqual(wide.map((n) => n.geometry));
    // ② 그러나 축마다의 차이가 상한 안이다.
    for (let i = 0; i < wide.length; i++) {
      const before = wide[i]!.geometry as unknown as Record<string, number>;
      const after = back[i]!.geometry as unknown as Record<string, number>;
      for (const k of Object.keys(before)) {
        expect(Math.abs(after[k]! - before[k]!), `${wide[i]!.id}.${k}`).toBeLessThanOrEqual(bound);
      }
    }
    // ③ 그리고 **적어도 한 축은 실제로 달라졌다** — ①이 빈 주장이 아니다.
    const changed = wide.some((el, i) => {
      const before = el.geometry as unknown as Record<string, number>;
      const after = back[i]!.geometry as unknown as Record<string, number>;
      return Object.keys(before).some((k) => after[k] !== before[k]);
    });
    expect(changed).toBe(true);
  });

  it('겉모습은 왕복하지 않는다 — 묶기와 풀기는 서로의 역이 아니다', () => {
    // 좌표는 왕복하지만 그룹 규칙은 버려진다. 그 비대칭을 값으로 못박는다.
    const grouped = groupNodes(scene(), pick('a', 't')).nodes;
    const withRules = grouped.map((n): CanvasNode =>
      isGroup(n) ? { ...n, rules: [{ op: 'lt', value: 1, patch: { fill: '#x' } }] } : n,
    );
    const back = ungroupNode(withRules, onlyGroup(withRules).id).nodes;
    const regrouped = groupNodes(back, pick(back[0]!.id, back[1]!.id)).nodes;
    expect(onlyGroup(regrouped).rules).toBeUndefined();
  });
});
