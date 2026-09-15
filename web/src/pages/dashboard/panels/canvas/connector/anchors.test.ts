// 고정 앵커 아홉을 **값으로** 못박는다 (SPEC-CANVAS-011 M3 · AC-09 ~ AC-15).
//
// 고정 입력은 `canvasOutline.test.ts` 와 **같은 투영**이다 — 스테이지 200×100 · 캔버스
// 500×400. 그래야 두 파일이 같은 상자를 말하는지 눈으로 견줄 수 있다. 이 축척에서 px→캔버스
// 되돌림은 가로 ×2.5 · 세로 ×4 이므로 아래 **리터럴** 기대값은 전부 정확한 수다.
//
// 다만 파생값끼리 견주는 자리(중심이 nw·se 의 한가운데인가, 옮긴 만큼 옮겨 갔는가)는
// 두 값이 서로 다른 셈 순서를 지나 마지막 비트가 갈린다 — 실제로 `220` 이 `220.00000000000003`
// 으로 나온다. 그 자리만 `expectPointNear` 로 잰다. 그것을 정확 비교로 우겨 두면 다음 사람이
// 기대값에 그 꼬리를 적어 넣게 되고, 그때 시험은 투영이 아니라 부동소수를 재게 된다.
//
// AC-16(중심 앵커에 붙은 선이 잘리지 않는다)은 여기서 재지 않는다 — 그것은 **그리는**
// 성질이고 연결선이 생기는 M6 전에는 관측할 대상이 없다.
//
// @spec SPEC-CANVAS-011 REQ-02

import fs from 'node:fs';
import path from 'node:path';
import { describe, expect, it } from 'vitest';

import type { CanvasElement } from '../canvasConfig';
import { unprojectPoint, type CanvasPoint, type CanvasProjection } from '../canvasGeometry';
import { BOX_HANDLE_IDS, handlePositions } from '../canvasEditGeometry';
import type { CanvasNode, GroupElement } from '../group/groupTypes';
import {
  addAnchorAt,
  anchorPoints,
  ANCHOR_CENTER_ID,
  FIXED_ANCHOR_IDS,
  removeAnchor,
} from './anchors';

const PROJ: CanvasProjection = {
  stage: { width: 200, height: 100 },
  canvas: { width: 500, height: 400 },
};

/** 글자 폭 장부가 비었다는 뜻 — 아직 한 프레임도 그리지 않은 상태다. */
const NO_WIDTHS: Readonly<Record<string, number>> = {};

/** 파생값끼리 견주는 자리에 쓴다. 리터럴 기대값은 정확 비교 그대로다(머리말 참조). */
function expectPointNear(
  got: CanvasPoint | undefined,
  want: CanvasPoint,
  hint: string,
): void {
  expect(got, hint).toBeDefined();
  expect(got!.x, `${hint}.x`).toBeCloseTo(want.x, 9);
  expect(got!.y, `${hint}.y`).toBeCloseTo(want.y, 9);
}

const RECT: CanvasElement = {
  id: 'r',
  kind: 'rect',
  geometry: { x: 50, y: 40, w: 100, h: 160 },
  style: {},
};

const ELLIPSE: CanvasElement = {
  id: 'e',
  kind: 'ellipse',
  geometry: { x: 100, y: 80, w: 200, h: 120 },
  style: {},
};

const PATH: CanvasElement = {
  id: 'p',
  kind: 'path',
  geometry: { x: 150, y: 120, w: 250, h: 200 },
  path: [
    { c: 'M', x: 0, y: 0 },
    { c: 'L', x: 1, y: 1 },
  ],
  style: {},
};

const LINE: CanvasElement = {
  id: 'l',
  kind: 'line',
  geometry: { x1: 50, y1: 80, x2: 250, y2: 240 },
  style: {},
};

const TEXT: CanvasElement = {
  id: 't',
  kind: 'text',
  geometry: { x: 250, y: 200 },
  style: { fontSize: 20 },
  text: 'abc',
};

/** 문구의 상자는 **글자를 재어 나온 값**이다 — 장부 없이는 폭 0 이다. */
const TEXT_WIDTHS: Readonly<Record<string, number>> = { t: 30 };

const GROUP: GroupElement = {
  id: 'g',
  kind: 'group',
  geometry: { x: 200, y: 160, w: 150, h: 80 },
  // 그룹 상자 밖으로 크게 비어져 나간 부품. 앵커가 합집합에서 나온다면 아홉 수가 전부 달라진다.
  parts: [{ id: 'gp', kind: 'rect', geometry: { x: -500, y: -500, w: 3000, h: 3000 }, style: {} }],
};

/** 여섯 갈래 × 그 갈래를 살리는 장부. */
const ALL_KINDS: ReadonlyArray<readonly [string, CanvasNode, Readonly<Record<string, number>>]> = [
  ['rect', RECT, NO_WIDTHS],
  ['ellipse', ELLIPSE, NO_WIDTHS],
  ['line', LINE, NO_WIDTHS],
  ['text', TEXT, TEXT_WIDTHS],
  ['path', PATH, NO_WIDTHS],
  ['group', GROUP, NO_WIDTHS],
];

// --- AC-09: 종류 불문 아홉이다 ---------------------------------------------

describe('아홉이다 — 종류로 갈리지 않는다 (AC-09 · A2)', () => {
  it.each(ALL_KINDS)('%s 가 아홉 자리를 낸다', (_name, node, widths) => {
    const points = anchorPoints(node, PROJ, widths);
    expect(points.size).toBe(9);
    expect([...points.keys()]).toEqual([...FIXED_ANCHOR_IDS]);
  });

  it('여덟은 8핸들의 이름 그대로이고 아홉째가 중심이다', () => {
    expect(FIXED_ANCHOR_IDS.slice(0, 8)).toEqual([...BOX_HANDLE_IDS]);
    expect(FIXED_ANCHOR_IDS[8]).toBe(ANCHOR_CENTER_ID);
  });

  it('상자가 없는 선도 아홉을 낸다 — 두 끝점을 감싼 상자에서 나온다', () => {
    // 선의 상자 갈래는 이미 `outlineBox` 안에 있다. 여기서 다시 가르지 않는 것이 A2 다.
    const points = anchorPoints(LINE, PROJ, NO_WIDTHS);
    expect(points.get('nw')).toEqual({ x: 50, y: 80 });
    expect(points.get('se')).toEqual({ x: 250, y: 240 });
    expect(points.get(ANCHOR_CENTER_ID)).toEqual({ x: 150, y: 160 });
  });

  it('끝점을 거꾸로 적은 선도 같은 아홉을 낸다', () => {
    // `normalizeBox` 가 음수 크기를 편다. 접지 않으면 nw 와 se 가 뒤바뀌어 나온다.
    const reversed: CanvasElement = {
      ...LINE,
      geometry: { x1: 250, y1: 240, x2: 50, y2: 80 },
    };
    expect([...anchorPoints(reversed, PROJ, NO_WIDTHS)]).toEqual([
      ...anchorPoints(LINE, PROJ, NO_WIDTHS),
    ]);
  });

  it('문구도 아홉을 낸다 — 세로 기준이 **중심**이다', () => {
    // 기준점 px (100,50), 폭 30 · 크기 20 → 상자 px (100,40)-(130,60).
    // 상단을 기준점으로 착각했다면 nw.y 가 200 으로 나온다.
    const points = anchorPoints(TEXT, PROJ, TEXT_WIDTHS);
    expect(points.get('nw')).toEqual({ x: 250, y: 160 });
    expect(points.get('se')).toEqual({ x: 325, y: 240 });
    expect(points.get(ANCHOR_CENTER_ID)).toEqual({ x: 250 + 37.5, y: 200 });
  });

  it('장부가 비면 문구의 아홉이 폭 0 인 상자에서 나온다', () => {
    // 실측 폭은 화면의 값이다. 아직 한 프레임도 그리지 않았으면 가로로 겹친 아홉이 나오며,
    // 그 사실을 감추지 않는다 — 여전히 아홉이다.
    const points = anchorPoints(TEXT, PROJ, NO_WIDTHS);
    expect(points.size).toBe(9);
    expect(points.get('nw')).toEqual({ x: 250, y: 160 });
    expect(points.get('ne')).toEqual({ x: 250, y: 160 });
  });

  it('종류별 갈래가 소스에 없다 (A2)', () => {
    // 갈래가 하나라도 생기면 "이 도형만 붙는 자리가 다르다" 가 표현 가능해진다.
    expect(anchorsSource()).not.toMatch(/\bswitch\s*\(/);
    expect(anchorsSource()).not.toMatch(/\.kind\b/);
  });
});

// --- AC-10: 여덟이 8핸들과 같은 좌표다 --------------------------------------

describe('여덟이 8핸들과 같은 자리다 (AC-10)', () => {
  // 두 함수는 **다른 공간**을 낸다 — `handlePositions` 는 스테이지 px, `anchorPoints` 는
  // 캔버스 단위다. 그래서 비교는 한쪽 공간을 골라야 하고, 여기서는 손잡이를 **되돌려**
  // 캔버스 단위에서 견준다. 그 방향이 정확한 비교이기 때문이다: 양쪽이 같은
  // `unprojectPoint` 하나를 지나므로 상자 산술이 같기만 하면 값이 **비트 단위로** 같고,
  // 반대 방향(앵커를 다시 투영)은 왕복 부동소수 오차를 비교에 끌어들인다.
  const boxKinds: ReadonlyArray<readonly [string, CanvasNode]> = [
    ['rect', RECT],
    ['ellipse', ELLIPSE],
    ['path', PATH],
    ['group', GROUP],
  ];

  it.each(boxKinds)('%s 의 여덟이 손잡이 여덟과 같다', (_name, node) => {
    const anchors = anchorPoints(node, PROJ, NO_WIDTHS);
    const handles = new Map(handlePositions(node, PROJ).map((h) => [h.id, h.point]));
    expect(handles.size).toBe(8);
    for (const id of BOX_HANDLE_IDS) {
      expect(anchors.get(id), id).toEqual(unprojectPoint(handles.get(id)!, PROJ));
    }
  });

  it('px 공간에서 견주어도 같다 — 왕복 오차만큼만 벌어진다', () => {
    // 위 단언이 "같은 함수를 지났으니 같다" 로 읽히지 않도록, 반대 방향도 한 번 잰다.
    // 이 축척에서 캔버스→px 는 가로 ÷2.5 · 세로 ÷4 다.
    const anchors = anchorPoints(RECT, PROJ, NO_WIDTHS);
    for (const handle of handlePositions(RECT, PROJ)) {
      const anchor = anchors.get(handle.id as (typeof BOX_HANDLE_IDS)[number]);
      expect(anchor, handle.id).toBeDefined();
      expect(anchor!.x / 2.5, handle.id).toBeCloseTo(handle.point.x, 10);
      expect(anchor!.y / 4, handle.id).toBeCloseTo(handle.point.y, 10);
    }
  });
});

// --- AC-11: 자리 이름을 하나만 새로 지었다 ----------------------------------

describe('새 이름은 중심 하나뿐이다 (AC-11)', () => {
  it('여덟의 출처가 `BOX_HANDLE_IDS` 다 — 손으로 적지 않았다', () => {
    const src = anchorsSource();
    expect(src).toContain('BOX_HANDLE_IDS');
    expect(src).toContain('BOX_HANDLE_FACTORS');
  });

  it('소스에 적힌 앵커 이름 리터럴이 `c` 하나뿐이다', () => {
    // 여덟 중 하나라도 손으로 적히면 표가 둘이 되고, 그중 하나가 바뀌는 날 조용히 갈라진다.
    const names = anchorsSource().match(/(['"])(?:nw|ne|se|sw|n|e|s|w|c)\1/g) ?? [];
    expect(names).toEqual(["'c'"]);
  });

  it('비율 표를 베끼지 않았다 — 적힌 비율이 중심의 두 축뿐이다', () => {
    // `BOX_HANDLE_FACTORS` 를 베꼈다면 0.5 가 **넷** 나타난다(n · e · s · w).
    const halves = anchorsSource().match(/\b0\.5\b/g) ?? [];
    expect(halves).toHaveLength(2);
  });
});

// --- AC-12 · AC-13: 중심과 되파생 ------------------------------------------

describe('중심이 상자 한가운데다 (AC-12)', () => {
  it('상자 (100,100)-(300,200) 의 중심은 (200,150) 이다', () => {
    const el: CanvasElement = {
      id: 'c1',
      kind: 'rect',
      geometry: { x: 100, y: 100, w: 200, h: 100 },
      style: {},
    };
    expect(anchorPoints(el, PROJ, NO_WIDTHS).get(ANCHOR_CENTER_ID)).toEqual({ x: 200, y: 150 });
  });

  it('중심은 nw 와 se 의 한가운데다 — 종류를 가리지 않는다', () => {
    for (const [name, node, widths] of ALL_KINDS) {
      const points = anchorPoints(node, PROJ, widths);
      const nw = points.get('nw');
      const se = points.get('se');
      const c = points.get(ANCHOR_CENTER_ID);
      expectPointNear(c, { x: (nw!.x + se!.x) / 2, y: (nw!.y + se!.y) / 2 }, name);
    }
  });

  it('정수로 죄지 않는다 — 파생값이므로 쓰는 쪽이 없다', () => {
    // 폭 30px 인 문구의 가로 중심은 캔버스 단위로 287.5 다. 여기서 반올림하면 그 사실이
    // 가려지고, `unprojectPoint` 가 적어 둔 "정수화는 쓰는 쪽의 몫" 이 둘로 갈린다.
    expect(anchorPoints(TEXT, PROJ, TEXT_WIDTHS).get(ANCHOR_CENTER_ID)?.x).toBe(287.5);
  });
});

describe('도형을 늘리면 아홉이 따라간다 (AC-13)', () => {
  it('새 상자에서 아홉이 다시 나온다', () => {
    const before = anchorPoints(RECT, PROJ, NO_WIDTHS);
    const grown: CanvasElement = { ...RECT, geometry: { x: 50, y: 40, w: 200, h: 320 } };
    const after = anchorPoints(grown, PROJ, NO_WIDTHS);

    // 원점에 붙은 nw 만 제자리다 — 나머지 여덟은 전부 움직인다.
    expect(after.get('nw')).toEqual(before.get('nw'));
    for (const id of FIXED_ANCHOR_IDS.filter((v) => v !== 'nw')) {
      expect(after.get(id), id).not.toEqual(before.get(id));
    }
    expect(after.get('se')).toEqual({ x: 250, y: 360 });
    expect(after.get(ANCHOR_CENTER_ID)).toEqual({ x: 150, y: 200 });
  });

  it('상자를 옮기면 아홉이 통째로 같은 양만큼 옮겨 간다', () => {
    const before = anchorPoints(RECT, PROJ, NO_WIDTHS);
    const moved: CanvasElement = { ...RECT, geometry: { x: 150, y: 140, w: 100, h: 160 } };
    const after = anchorPoints(moved, PROJ, NO_WIDTHS);
    for (const id of FIXED_ANCHOR_IDS) {
      expectPointNear(after.get(id), { x: before.get(id)!.x + 100, y: before.get(id)!.y + 100 }, id);
    }
  });
});

// --- AC-14: 저장되지 않는다 -------------------------------------------------

describe('고정 앵커가 저장되지 않는다 (AC-14 · A1)', () => {
  it('앵커를 구해도 노드가 한 글자도 바뀌지 않는다', () => {
    // 파생이라는 말의 관측 가능한 뜻이 이것이다 — 구하는 일이 config 에 흔적을 남기지 않는다.
    for (const [name, node, widths] of ALL_KINDS) {
      const before = JSON.stringify(node);
      anchorPoints(node, PROJ, widths);
      expect(JSON.stringify(node), name).toBe(before);
    }
  });

  it('같은 노드를 두 번 물어도 같은 답이 나온다 — 첫 답을 어디에도 기억하지 않는다', () => {
    expect([...anchorPoints(RECT, PROJ, NO_WIDTHS)]).toEqual([
      ...anchorPoints(RECT, PROJ, NO_WIDTHS),
    ]);
  });

  it('`canvasConfig.ts` 가 고정 앵커를 모른다', () => {
    // 자료형에도 파서에도 아홉이 앉을 자리가 없다. 있으면 언젠가 누군가 적는다.
    const config = stripComments(read('canvasConfig.ts'));
    for (const name of ['FixedAnchorId', 'FIXED_ANCHOR_IDS', 'ANCHOR_CENTER_ID', 'anchorPoints']) {
      expect(config, name).not.toContain(name);
    }
  });

  it('내보내는 함수가 셋뿐이다 — 읽기 하나와 임의 앵커 쓰기 둘', () => {
    // M3 에서는 이 목록이 하나였다. M3′ 가 **임의 앵커**(A10 — 저장한다)의 더하기·빼기를
    // 더하면서 둘이 늘었고, 목록이 닫혀 있다는 성질은 그대로다: 넷째가 생기면 빨개진다.
    // 고정 아홉이 저장되지 않는다는 A1 은 아래 두 단언이 대신 붙든다.
    const fns = anchorsSource().match(/export function (\w+)/g) ?? [];
    expect(fns).toEqual([
      'export function anchorPoints',
      'export function addAnchorAt',
      'export function removeAnchor',
    ]);
  });

  it('쓰기 둘이 만지는 필드가 `anchors` 하나뿐이다', () => {
    // A1 의 관측 가능한 뜻이 이것이다 — 쓰기 경로가 생겨도 아홉은 여전히 파생이고,
    // 기하도 종류도 스타일도 그 경로를 지나 바뀌지 않는다.
    const added = addAnchorAt(RECT, { x: 100, y: 120 }, 'a1');
    expect({ ...added, anchors: undefined }).toEqual({ ...RECT, anchors: undefined });
    expect(removeAnchor(added, 'a1')).toEqual(RECT);
  });

  it('저장된 앵커에 고정 아홉의 이름이 섞이지 않는다', () => {
    const added = addAnchorAt(RECT, { x: 100, y: 120 }, 'a1');
    expect(added.anchors?.map((a) => a.id)).toEqual(['a1']);
  });
});

// --- AC-15: 부품에는 앵커가 없다 --------------------------------------------

describe('부품은 앵커를 내지 않는다 (AC-15 · A3)', () => {
  // `anchorPoints` 의 인자는 **최상위 노드**다. 부품은 그룹 로컬 격자에 살고 최상위 순회에
  // 나오지 않으므로 이 함수에 닿을 길이 없다 — 막을 런타임 검사를 두지 않는 근거다. 다만
  // 부품과 `CanvasElement` 가 구조적으로 같은 타입이라 그 금지가 **타입으로는** 서지
  // 않는다. 그래서 여기서는 관측 가능한 짝을 잰다: 그룹의 아홉은 부품이 무엇이든 **제
  // 상자**에서만 나온다. 부품이 한 자리라도 기여한다면 아래가 빨개진다.
  it('그룹의 아홉이 부품이 아니라 제 상자에서 나온다', () => {
    const points = anchorPoints(GROUP, PROJ, NO_WIDTHS);
    expect(points.get('nw')).toEqual({ x: 200, y: 160 });
    expect(points.get('se')).toEqual({ x: 350, y: 240 });
    expect(points.get(ANCHOR_CENTER_ID)).toEqual({ x: 275, y: 200 });
  });

  it('부품을 바꾸거나 비워도 아홉이 움직이지 않는다', () => {
    const base = [...anchorPoints(GROUP, PROJ, NO_WIDTHS)];
    const empty: GroupElement = { ...GROUP, parts: [] };
    const many: GroupElement = {
      ...GROUP,
      parts: [
        { id: 'a', kind: 'rect', geometry: { x: 0, y: 0, w: 10, h: 10 }, style: {} },
        { id: 'b', kind: 'line', geometry: { x1: 0, y1: 0, x2: 999, y2: 999 }, style: {} },
      ],
    };
    expect([...anchorPoints(empty, PROJ, NO_WIDTHS)]).toEqual(base);
    expect([...anchorPoints(many, PROJ, NO_WIDTHS)]).toEqual(base);
  });

  it('부품 수가 아홉을 늘리지도 줄이지도 않는다', () => {
    expect(anchorPoints({ ...GROUP, parts: [] }, PROJ, NO_WIDTHS).size).toBe(9);
  });
});

// --- 상자를 두 번 재지 않는다 -----------------------------------------------

describe('상자는 한 번만 잰다 (AC-33 — M3′ 가 물려받을 불변식)', () => {
  it('`outlineBox` 호출이 소스에 한 번뿐이다', () => {
    // 두 번 재면 M3′ 의 임의 앵커가 고정 아홉과 **다른 상자**에서 나올 수 있고, 그 갈라짐은
    // 크기를 바꾼 뒤에야 보인다.
    const calls = anchorsSource().match(/outlineBox\(/g) ?? [];
    expect(calls).toHaveLength(1);
  });
});

// --- 모듈 경계 --------------------------------------------------------------

describe('모듈의 형상', () => {
  it('anchors 는 document · window 를 모른다', () => {
    const src = anchorsSource();
    expect(src).not.toMatch(/\bdocument\b/);
    expect(src).not.toMatch(/\bwindow\b/);
  });
});

// --- 소스 읽기 도우미 -------------------------------------------------------
//
// 주석은 걷어내고 읽는다: 산문에 적힌 낱말이 가드를 헛되이 울리면 다음 사람은 주석을 고쳐
// 지나가고, 그때 가드는 이미 죽은 것이다(009 가드가 세운 규율 그대로).

const CANVAS_DIR = path.join(__dirname, '..');

function stripComments(text: string): string {
  return text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/(^|[^:])\/\/.*$/gm, '$1');
}

function read(...parts: string[]): string {
  return fs.readFileSync(path.join(CANVAS_DIR, ...parts), 'utf8');
}

function anchorsSource(): string {
  return stripComments(read('connector', 'anchors.ts'));
}
