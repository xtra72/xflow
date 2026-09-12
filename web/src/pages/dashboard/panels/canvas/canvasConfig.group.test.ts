// 그룹 노드의 관용 파서 (SPEC-CANVAS-004 M1).
//
// **이 파일이 가장 먼저 재는 것은 왕복이다.** 008 이 적어 둔 대로, 여기서 가장 나쁜 실패는
// 예외가 아니라 **조용한 소실**이다 — `parseElement` 의 `kind` 화이트리스트는 모르는 종류를
// 말없이 버리므로, 그룹 갈래가 없으면 손으로 저술한 그룹이 **새로 고침 한 번에 사라진다.**
// 예외도 경고도 없다.
//
// 고정 입력은 004 의 시험 규율이 요구하는 형상을 쓴다(acceptance.md §시험 규율):
//   - **E-A** 부품이 셋 이상이고 상자가 서로 다르며 겹치지 않는다. 하나는 가장자리에 닿지
//     않고 안쪽에 떠 있다.
//   - **E-C** 그룹 상자 변이 `317 × 181` — 둘 다 `GROUP_LOCAL_EXTENT` 를 나누어떨어뜨리지
//     않고, 서로 다르며(비정사각), 소수 축척을 만든다.
//   - **E-D** 원점이 0 이 아니고 두 축이 다르다. 음수 원점도 따로 둔다.
//   - **E-L** 부품에 문구가 하나 섞여 있다.
//
// @spec SPEC-CANVAS-004 REQ-01 · REQ-05 · AC-E1 · AC-E2 · AC-E3

import { describe, expect, it } from 'vitest';

import {
  DEFAULT_BOX_GEOMETRY,
  MAX_CANVAS_DIMENSION,
  parseCanvasConfig,
  type CanvasElement,
} from './canvasConfig';
import { GROUP_LOCAL_EXTENT, isGroup, type GroupElement } from './group/groupTypes';

// --- 고정 입력 -------------------------------------------------------------

/** 그룹 로컬 좌표를 가진 부품 넷. 상자가 서로 다르고 겹치지 않는다(E-A · E-B · E-L). */
const RAW_PARTS: ReadonlyArray<Record<string, unknown>> = [
  // 왼쪽 위 — 상자 가장자리에 닿는다.
  { id: 'body', kind: 'rect', geometry: { x: 0, y: 0, w: 3000, h: 2000 } },
  // **안쪽에 떠 있다** — 어느 좌표도 0 도 EXTENT 도 아니라 축척 산술이 관측된다.
  { id: 'stem', kind: 'ellipse', geometry: { x: 4100, y: 3300, w: 1700, h: 900 } },
  // 선 — 끝점 둘이 서로 다른 사분면에 있다.
  { id: 'pipe', kind: 'line', geometry: { x1: 1200, y1: 7400, x2: 8800, y2: 6100 } },
  // 문구 — 넷째 프레임 표면(글자 폭 장부)이 자지 않도록 모든 고정 입력에 하나 둔다.
  { id: 'label', kind: 'text', geometry: { x: 5000, y: 9200 }, text: '{value}' },
];

function rawGroup(over: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    id: 'grp-1',
    kind: 'group',
    // E-C · E-D: 원점이 0 이 아니고 두 축이 다르며, 변이 10000 을 나누어떨어뜨리지 않는다.
    geometry: { x: 73, y: 41, w: 317, h: 181 },
    parts: RAW_PARTS.map((p) => ({ ...p })),
    ...over,
  };
}

function parse(nodes: readonly unknown[]): ReturnType<typeof parseCanvasConfig> {
  return parseCanvasConfig({ channel_name: '', canvas: { width: 500, height: 400 }, elements: nodes });
}

/** 파싱 결과에서 그룹 하나를 꺼낸다. 없으면 시험이 그 자리에서 죽는다. */
function onlyGroup(nodes: readonly unknown[]): GroupElement {
  const node = parse(nodes).elements[0];
  expect(node, '그룹이 파서를 지나 살아남아야 한다').toBeDefined();
  if (node === undefined || !isGroup(node)) throw new Error('그룹이 아니다');
  return node;
}

// --- 왕복 — 조용한 소실이 없다 ---------------------------------------------

describe('그룹은 저장 왕복을 지나 살아남는다 (REQ-01 · AC-E3)', () => {
  it('손으로 저술한 그룹이 새로 고침에 사라지지 않는다', () => {
    const g = onlyGroup([rawGroup()]);
    expect(g.kind).toBe('group');
    expect(g.geometry).toEqual({ x: 73, y: 41, w: 317, h: 181 });
    expect(g.parts.map((p) => p.id)).toEqual(['body', 'stem', 'pipe', 'label']);
    expect(g.parts.map((p) => p.kind)).toEqual(['rect', 'ellipse', 'line', 'text']);
  });

  it('두 번 읽어도 같다(멱등) — 파서의 산출이 파서의 입력으로 성립한다', () => {
    const once = parse([rawGroup(), { id: 'r1', kind: 'rect', geometry: { x: 5, y: 6, w: 7, h: 8 } }]);
    expect(parseCanvasConfig(once)).toEqual(once);
  });

  it('부품 좌표가 그룹 로컬 격자 그대로다 — 파서가 캔버스 단위로 옮기지 않는다', () => {
    const g = onlyGroup([rawGroup()]);
    const stem = g.parts[1];
    expect(stem?.kind).toBe('ellipse');
    // 이 값이 `{ x: 4100, ... }` 이 아니게 되는 순간 부품 좌표계가 둘이 된 것이다.
    expect(stem && 'w' in stem.geometry ? stem.geometry : null).toEqual({
      x: 4100,
      y: 3300,
      w: 1700,
      h: 900,
    });
    // 격자를 넘는 좌표도 **clamp 하지 않는다**(경로 로컬 좌표와 같은 규율).
    const over = onlyGroup([
      rawGroup({ parts: [{ id: 'a', kind: 'rect', geometry: { x: -500, y: 0, w: GROUP_LOCAL_EXTENT * 3, h: 10 } }] }),
    ]);
    expect(over.parts[0] && 'w' in over.parts[0].geometry ? over.parts[0].geometry.x : null).toBe(-500);
    expect(over.parts[0] && 'w' in over.parts[0].geometry ? over.parts[0].geometry.w : null).toBe(
      GROUP_LOCAL_EXTENT * 3,
    );
  });

  it('원점이 음수인 그룹도 그대로 읽힌다 — 캔버스 밖 저술은 합법이다(가정 A5 · E-D)', () => {
    const g = onlyGroup([rawGroup({ geometry: { x: -120, y: 41, w: 317, h: 181 } })]);
    expect(g.geometry).toEqual({ x: -120, y: 41, w: 317, h: 181 });
  });
});

// --- 선택 필드 -------------------------------------------------------------

describe('그룹의 선택 필드 — 만들어 채우지 않는다', () => {
  it('`style`·`rules` 가 없는 그룹에는 그 키가 **없다**', () => {
    const g = onlyGroup([rawGroup()]);
    expect('style' in g).toBe(false);
    expect('rules' in g).toBe(false);
    expect('binding' in g).toBe(false);
    expect('tween' in g).toBe(false);
    expect('symbol' in g).toBe(false);
  });

  it('빈 객체 `style: {}` 도 미지정으로 떨어진다 — "미사용" 과 "빈 캐스케이드" 를 가르지 않는다', () => {
    expect('style' in onlyGroup([rawGroup({ style: {} })])).toBe(false);
    // 손상 값만 든 스타일도 마찬가지다(파싱 결과가 비면 미지정이다).
    expect('style' in onlyGroup([rawGroup({ style: { fill: 42, align: 'sideways' } })])).toBe(false);
  });

  it('쓸 만한 스타일 한 속성이 있으면 그 속성만 남는다', () => {
    const g = onlyGroup([rawGroup({ style: { opacity: 0.3, fill: 7 } })]);
    expect(g.style).toEqual({ opacity: 0.3 });
  });

  it('`binding`·`rules`·`tween` 은 001 의 파서를 그대로 지난다', () => {
    const g = onlyGroup([
      rawGroup({
        binding: { series: 'valve.open', agg: 'nonsense' },
        rules: [{ op: 'gte', value: 1, patch: { stroke: '#0f0' } }, { op: 'bogus', value: 0, patch: {} }],
        tween: { duration_ms: 120, easing: 'ease-in' },
      }),
    ]);
    expect(g.binding).toEqual({ series: 'valve.open', agg: 'last' });
    expect(g.rules).toEqual([{ op: 'gte', value: 1, patch: { stroke: '#0f0' } }]);
    expect(g.tween).toEqual({ duration_ms: 120, easing: 'ease-in' });
  });

  it('`symbol` 은 두 문자열이 다 있을 때만 남는다 — 렌더가 읽지 않는 값이다', () => {
    expect(onlyGroup([rawGroup({ symbol: { catalog_id: 'valve-two-way', version: '1' } })]).symbol).toEqual({
      catalog_id: 'valve-two-way',
      version: '1',
    });
    expect('symbol' in onlyGroup([rawGroup({ symbol: { catalog_id: 'valve-two-way' } })])).toBe(false);
    expect('symbol' in onlyGroup([rawGroup({ symbol: 'valve' })])).toBe(false);
  });
});

// --- 견고성 (REQ-05) -------------------------------------------------------

describe('그룹 갈래는 어떤 입력에도 예외를 던지지 않는다 (REQ-05 · AC-E1 · AC-E2)', () => {
  it('`parts` 안의 그룹은 **형상 덕에** 버려진다 — 재귀 렌더로 들어가지 않는다(A6)', () => {
    const g = onlyGroup([
      rawGroup({
        parts: [
          { id: 'a', kind: 'rect', geometry: { x: 0, y: 0, w: 10, h: 10 } },
          { id: 'inner', kind: 'group', geometry: { x: 0, y: 0, w: 10, h: 10 }, parts: [] },
          { id: 'b', kind: 'rect', geometry: { x: 20, y: 20, w: 10, h: 10 } },
        ],
      }),
    ]);
    expect(g.parts.map((p) => p.id)).toEqual(['a', 'b']);
    // 타입으로도 형상으로도 그룹은 부품이 될 수 없다.
    expect(g.parts.every((p: CanvasElement) => p.kind !== ('group' as string))).toBe(true);
  });

  it('부품 id 중복은 **먼저 온 것이 이긴다**(001 규칙 동일)', () => {
    const g = onlyGroup([
      rawGroup({
        parts: [
          { id: 'dup', kind: 'rect', geometry: { x: 0, y: 0, w: 10, h: 10 }, unit: 'first' },
          { id: 'dup', kind: 'rect', geometry: { x: 0, y: 0, w: 20, h: 20 }, unit: 'second' },
          { id: 'other', kind: 'rect', geometry: { x: 30, y: 30, w: 10, h: 10 } },
        ],
      }),
    ]);
    expect(g.parts.map((p) => p.id)).toEqual(['dup', 'other']);
    expect(g.parts[0]?.unit).toBe('first');
  });

  it('최상위 id 중복도 먼저 온 것이 이긴다 — 그룹과 요소가 **같은 id 공간**을 쓴다', () => {
    const cfg = parse([
      rawGroup({ id: 'same' }),
      { id: 'same', kind: 'rect', geometry: { x: 0, y: 0, w: 10, h: 10 } },
      { id: 'other', kind: 'rect', geometry: { x: 0, y: 0, w: 10, h: 10 } },
    ]);
    expect(cfg.elements.map((n) => n.id)).toEqual(['same', 'other']);
    expect(cfg.elements[0]?.kind).toBe('group');
  });

  it('id 가 없는 그룹은 버린다(정체성이 없다)', () => {
    expect(parse([rawGroup({ id: undefined }), rawGroup({ id: 'ok' })]).elements.map((n) => n.id)).toEqual([
      'ok',
    ]);
  });

  it('`parts` 가 배열이 아니면 **빈 그룹**이다 — 오류가 아니다(AC-E1)', () => {
    for (const bad of [undefined, null, 42, 'parts', { a: 1 }]) {
      const g = onlyGroup([rawGroup({ parts: bad })]);
      expect(g.parts).toEqual([]);
    }
  });

  it('퇴화·손상 기하는 001 의 씨앗 상자로 떨어진다(같은 함수를 지난다)', () => {
    expect(onlyGroup([rawGroup({ geometry: { x: 10, y: 10, w: 0, h: 50 } })]).geometry).toEqual(
      DEFAULT_BOX_GEOMETRY,
    );
    expect(onlyGroup([rawGroup({ geometry: 'nope' })]).geometry).toEqual(DEFAULT_BOX_GEOMETRY);
    // 좌표는 반올림되지만 범위는 죄지 않는다.
    expect(onlyGroup([rawGroup({ geometry: { x: 73.6, y: 41.2, w: 317.4, h: 181.5 } })]).geometry).toEqual({
      x: 74,
      y: 41,
      w: 317,
      h: 182,
    });
  });

  it('상한을 넘는 그룹 상자도 좌표는 clamp 되지 않는다', () => {
    const g = onlyGroup([
      rawGroup({ geometry: { x: 0, y: 0, w: MAX_CANVAS_DIMENSION, h: MAX_CANVAS_DIMENSION } }),
    ]);
    expect(g.geometry.w).toBe(MAX_CANVAS_DIMENSION);
  });

  it('어떤 손상 입력에도 던지지 않는다', () => {
    const CORRUPT: unknown[] = [
      { kind: 'group' },
      { id: 'g', kind: 'group' },
      { id: 'g', kind: 'group', parts: [null, 1, 'x', [], { kind: 'group' }] },
      { id: 'g', kind: 'group', geometry: { x: Number.NaN, y: Number.POSITIVE_INFINITY } },
      { id: 'g', kind: 'group', rules: 'nope', binding: 3, tween: [], symbol: null, style: 7 },
      { id: 'g', kind: 'GROUP' },
      { id: 'g', kind: 'group', parts: RAW_PARTS },
    ];
    for (const raw of CORRUPT) {
      expect(() => parse([raw])).not.toThrow();
    }
    // 대문자 종류는 그룹도 요소도 아니므로 버려진다(화이트리스트 규율 그대로).
    expect(parse([{ id: 'g', kind: 'GROUP' }]).elements).toEqual([]);
  });
});

// --- 상·하위 호환 (AC-E3) --------------------------------------------------

describe('상위 호환 — 001 시절 config 는 한 글자도 달라지지 않는다', () => {
  it('그룹이 없는 config 의 파싱 결과가 종전 그대로다', () => {
    const cfg = parse([
      { id: 'r1', kind: 'rect', geometry: { x: 5, y: 6, w: 7, h: 8 } },
      { id: 't1', kind: 'text', geometry: { x: 1, y: 2 }, text: 'hi' },
    ]);
    expect(cfg.elements.map((n) => n.kind)).toEqual(['rect', 'text']);
    expect(cfg.elements.some(isGroup)).toBe(false);
  });
});
