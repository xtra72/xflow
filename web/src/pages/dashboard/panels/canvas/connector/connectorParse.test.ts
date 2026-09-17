// 연결선의 자료형과 파서를 **값으로** 못박는다 (SPEC-CANVAS-011 M4 · AC-34 ~ AC-39 · AC-83).
//
// ## 이 파일이 재는 것은 **왕복**이다
//
// 011 에는 직렬화기가 없다. 파서가 양방향의 스키마 권위이며, 그것이 되세우지 않는 필드는
// 다시 읽는 순간 사라진다 — 예외도 경고도 없이 사용자의 저술이 없어지는, 008 이 이름 적어
// 둔 가장 나쁜 실패다. 그래서 아래 단언은 값 하나하나가 아니라 **`JSON.stringify` 왕복**을
// 재는 쪽으로 적혀 있다: 필드가 하나 빠지면 값 비교는 통과할 수 있어도 왕복은 통과하지
// 못한다.
//
// ## 키 집합까지 본다 (AC-25 의 규율 · AC-83 의 전제)
//
// "없던 키가 생기지 않는다" 는 화면에서 보이지 않는 성질이라 사람이 알아차리지 못한 채
// config 에만 남는다. 그래서 `Object.keys` 를 직접 견주는 단언을 둔다 — 연결선을 하나도
// 쓰지 않은 config 가 011 이전과 **키 집합까지** 같아야 한다는 REQ-09 가 그 위에 선다.
//
// @spec SPEC-CANVAS-011 REQ-03 · REQ-04 · REQ-09

import { describe, expect, it } from 'vitest';

import { DEFAULT_CANVAS_SIZE, parseCanvasConfig, parseElements } from '../canvasConfig';
import { MAX_PATH_COMMANDS } from '../shapes/pathTypes';
import { isConnector, type ConnectorElement } from './connectorTypes';

/** 최상위 배열을 담은 날것 config. 크기는 언제나 기본값이라 이 파일의 축은 요소뿐이다. */
function cfg(elements: readonly unknown[]): Record<string, unknown> {
  return { channel_name: '', canvas: { ...DEFAULT_CANVAS_SIZE }, elements };
}

/** 날것 배열을 파서에 태우고 나온 최상위 노드들. */
function parseNodesOf(elements: readonly unknown[]): ReturnType<typeof parseCanvasConfig>['elements'] {
  return parseCanvasConfig(cfg(elements)).elements;
}

/** 나온 것들 가운데 연결선만. 판별은 **그 한 함수**를 지난다(AC-39). */
function connectorsOf(elements: readonly unknown[]): ConnectorElement[] {
  return parseNodesOf(elements).filter(isConnector);
}

/** 연결선 하나를 읽는다. 버려졌으면 `undefined` 다. */
function parseOne(raw: Record<string, unknown>): ConnectorElement | undefined {
  return connectorsOf([raw])[0];
}

/** JSON 왕복 — 저장했다가 다시 읽는 실제 경로와 같은 모양이다. */
function reread(node: unknown): unknown {
  return connectorsOf([JSON.parse(JSON.stringify(node)) as unknown])[0];
}

const MINIMAL = {
  id: 'c1',
  kind: 'connector',
  from: { el: 'el-1', a: 'e' },
  to: { el: 'el-2', a: 'w' },
  route: 'straight',
} as const;

// --- AC-34 · AC-35: 앞의 두 이름을 넓히지 않았다 -----------------------------
//
// 유니온의 원소를 세는 일 자체는 출시된 가드 둘(`canvasElementKind.test.tsx` 의
// `Record<CanvasElementKind, true>` 와 `canvas007Regression.test.tsx` 의 소스 텍스트 판정)이
// 이미 하고 있고, 011 은 그 둘을 **한 글자도 고치지 않는다**. 여기서는 그 둘이 재지 못하는
// 것 하나를 잰다: 연결선이 **요소 파서를 지나지 않는다**는 사실.

describe('연결선은 요소가 아니다 (AC-34 · AC-35)', () => {
  it('`parseElements` 는 연결선을 읽지 못한다 — `kind` 화이트리스트가 그대로다', () => {
    // 이 단언이 곧 "`parseElement` 가 한 글자도 바뀌지 않았다" 이다. 만약 그 함수에
    // 여섯째 갈래가 생겼다면 여기서 하나가 나온다.
    expect(parseElements([{ ...MINIMAL }])).toEqual([]);
  });

  it('그룹 부품에 섞여 들어온 연결선은 **저절로** 버려진다 (A3)', () => {
    // `parts` 는 `parseElements` 를 그대로 부르므로 런타임 깊이 검사가 필요 없다 —
    // 004 가 그룹 중첩에 대해 쓴 그 형상이다.
    const nodes = parseNodesOf([
      {
        id: 'g',
        kind: 'group',
        geometry: { x: 0, y: 0, w: 10, h: 10 },
        parts: [
          { ...MINIMAL },
          { id: 'r', kind: 'rect', geometry: { x: 0, y: 0, w: 1, h: 1 } },
        ],
      },
    ]);
    const group = nodes[0];
    if (group?.kind !== 'group') throw new Error('그룹이어야 한다');
    expect(group.parts.map((p) => p.id)).toEqual(['r']);
  });

  it('최상위에서는 읽힌다 — 갈래가 요소 파서 **위 한 층**에 선다', () => {
    expect(parseOne({ ...MINIMAL })?.id).toBe('c1');
  });
});

// --- AC-36: 저장 왕복에 값이 바뀌지 않는다 -----------------------------------

describe('저장 왕복에 값이 바뀌지 않는다 (AC-36)', () => {
  it('최소 연결선이 그대로 돌아온다 — 키가 늘지도 줄지도 않는다', () => {
    const once = parseOne({ ...MINIMAL })!;
    expect(reread(once)).toEqual(once);
    // 키 집합까지 본다 — 값이 같아도 키가 하나 늘면 config 만 조용히 자란다.
    expect(Object.keys(once).sort()).toEqual(['from', 'id', 'kind', 'route', 'to']);
  });

  it('선택 필드를 전부 채운 연결선도 그대로 돌아온다', () => {
    const full = {
      id: 'c2',
      kind: 'connector',
      from: { el: 'el-1', a: 'a1' },
      to: { x: 300, y: 200 },
      route: 'curve',
      points: [
        { x: 120, y: 130 },
        { x: 210, y: 90 },
      ],
      style: { stroke: '#ff0000', strokeWidth: 3 },
      binding: { series: 's1' },
      rules: [{ op: 'gt', value: 10, patch: { stroke: '#00ff00' } }],
      tween: { duration_ms: 120, easing: 'linear' },
    };
    const once = parseOne(full)!;
    expect(reread(once)).toEqual(once);
    expect(once.points).toEqual([
      { x: 120, y: 130 },
      { x: 210, y: 90 },
    ]);
    expect(once.to).toEqual({ x: 300, y: 200 });
    expect(once.style).toEqual({ stroke: '#ff0000', strokeWidth: 3 });
    expect(once.rules).toHaveLength(1);
    expect(once.tween).toEqual({ duration_ms: 120, easing: 'linear' });
  });

  it('쓰지 않은 선택 필드는 **키가 생기지 않는다** (AC-25 와 같은 규율)', () => {
    const bare = parseOne({ ...MINIMAL })!;
    for (const key of ['points', 'style', 'binding', 'rules', 'tween']) {
      expect(key in bare, key).toBe(false);
    }
  });

  it('빈 목록은 부재로 떨어진다 — "찍은 적 없음" 과 "다 지웠음" 이 같은 값이 아니다', () => {
    const emptied = parseOne({ ...MINIMAL, points: [], rules: [] })!;
    expect('points' in emptied).toBe(false);
    expect('rules' in emptied).toBe(false);
  });

  it('모르는 필드는 보존하지 않는다 — 파서가 스키마 권위다', () => {
    const withJunk = parseOne({ ...MINIMAL, nonsense: 42, label: 'x' })!;
    expect('nonsense' in withJunk).toBe(false);
    expect('label' in withJunk).toBe(false);
  });

  it('id 중복은 먼저 온 것이 이긴다 — 요소·그룹과 **같은 id 공간**이다', () => {
    const nodes = parseNodesOf([
      { ...MINIMAL, route: 'elbow' },
      { id: 'c1', kind: 'rect', geometry: { x: 0, y: 0, w: 5, h: 5 } },
    ]);
    expect(nodes).toHaveLength(1);
    expect(nodes[0]?.kind).toBe('connector');
  });
});

// --- AC-37: 모르는 `route` 는 직선으로 떨어진다 ------------------------------

describe('모르는 `route` 는 직선으로 떨어진다 (AC-37)', () => {
  it.each([
    ['spiral', 'spiral'],
    ['빈 문자열', ''],
    ['숫자', 7],
    ['부재', undefined],
    ['null', null],
    ['객체', { kind: 'curve' }],
  ])('%s → straight 이고 예외가 없다', (_name, route) => {
    expect(() => parseOne({ ...MINIMAL, route })).not.toThrow();
    expect(parseOne({ ...MINIMAL, route })?.route).toBe('straight');
  });

  it('아는 넷은 그대로 산다', () => {
    for (const route of ['straight', 'elbow', 'curve', 'free'] as const) {
      expect(parseOne({ ...MINIMAL, route })?.route, route).toBe(route);
    }
  });

  it('노드를 버리지 않는다 — 그리는 법은 한 번 더 고르면 되지만 사라진 선은 못 되돌린다', () => {
    expect(parseOne({ ...MINIMAL, route: 'spiral' })?.id).toBe('c1');
  });
});

// --- AC-38: 끝점이 없으면 노드를 버린다 --------------------------------------

describe('끝점이 없으면 노드를 버린다 (AC-38)', () => {
  it.each([
    ['from 이 없다', { to: { el: 'el-2', a: 'w' } }],
    ['to 가 없다', { from: { el: 'el-1', a: 'e' } }],
    ['둘 다 없다', {}],
  ])('%s → 배열에 들지 않는다', (_name, ends) => {
    const raw = { id: 'c1', kind: 'connector', route: 'straight', ...ends };
    expect(parseNodesOf([raw])).toEqual([]);
  });

  it.each([
    ['끝점이 객체가 아니다', 'el-2'],
    ['끝점이 배열이다', []],
    ['끝점이 null 이다', null],
    ['참조에 앵커 자리가 없다', { el: 'el-2' }],
    ['참조에 요소 id 가 없다', { a: 'w' }],
    ['참조 id 가 빈 문자열이다', { el: '   ', a: 'w' }],
    ['자유 끝점의 한 축이 손상됐다', { x: 10, y: 'zzz' }],
    ['자유 끝점이 비유한이다', { x: Number.NaN, y: 3 }],
  ])('%s → 노드 자체를 버린다', (_name, to) => {
    expect(parseNodesOf([{ ...MINIMAL, to }])).toEqual([]);
  });

  it('id 가 없어도 버린다 — 001 의 정체성 규율 그대로다', () => {
    expect(parseNodesOf([{ ...MINIMAL, id: '' }])).toEqual([]);
    expect(parseNodesOf([{ ...MINIMAL, id: undefined }])).toEqual([]);
  });

  it('버리는 것은 **그 노드 하나**다 — 이웃은 그대로 산다', () => {
    const nodes = parseNodesOf([
      { id: 'r', kind: 'rect', geometry: { x: 0, y: 0, w: 5, h: 5 } },
      { id: 'c-bad', kind: 'connector', route: 'straight' },
      { ...MINIMAL },
    ]);
    expect(nodes.map((n) => n.id)).toEqual(['r', 'c1']);
  });

  it('예외를 던지지 않는다 — 어떤 손상 입력에도', () => {
    for (const raw of [
      { id: 'c', kind: 'connector' },
      { id: 'c', kind: 'connector', from: 1, to: 2 },
      { id: 'c', kind: 'connector', from: { el: 'a', a: 'e' }, to: { x: 1, y: 2 }, points: 'zzz' },
      { id: 'c', kind: 'connector', from: { el: 'a', a: 'e' }, to: { x: 1, y: 2 }, style: 7 },
    ]) {
      expect(() => parseNodesOf([raw]), JSON.stringify(raw)).not.toThrow();
    }
  });
});

// --- 끝점과 중간점의 관용 ----------------------------------------------------

describe('끝점은 참조이거나 자유 좌표다', () => {
  it('참조는 두 문자열을 그대로 보존한다', () => {
    expect(parseOne({ ...MINIMAL, from: { el: 'el-9', a: 'a3' } })?.from).toEqual({
      el: 'el-9',
      a: 'a3',
    });
  });

  it('자유 끝점은 **정수 캔버스 단위**로 반올림된다 — 001 의 좌표 규율 그대로다', () => {
    expect(parseOne({ ...MINIMAL, from: { x: 10.4, y: 20.6 } })?.from).toEqual({ x: 10, y: 21 });
  });

  it('둘이 한 객체에 섞여 오면 **참조가 이긴다**', () => {
    // 참조는 사용자가 앵커를 골랐다는 사실이고 좌표는 그 결과의 흔적이다. 좌표를 택하면
    // 붙여 둔 선이 저장 왕복 한 번에 떨어져 나온다.
    expect(parseOne({ ...MINIMAL, from: { el: 'el-1', a: 'e', x: 5, y: 5 } })?.from).toEqual({
      el: 'el-1',
      a: 'e',
    });
  });
});

describe('중간점은 손상 항목만 빠진다', () => {
  it('손상된 점만 빠지고 나머지는 자리 차례대로 남는다', () => {
    const points = parseOne({
      ...MINIMAL,
      route: 'elbow',
      points: [{ x: 1, y: 2 }, { x: 'zz', y: 4 }, null, { x: 5, y: 6 }],
    })?.points;
    expect(points).toEqual([
      { x: 1, y: 2 },
      { x: 5, y: 6 },
    ]);
  });

  it('배열이 아니면 미지정이다 — 노드는 살아남는다', () => {
    const node = parseOne({ ...MINIMAL, route: 'elbow', points: 'zzz' })!;
    expect('points' in node).toBe(false);
    expect(node.route).toBe('elbow');
  });

  it('살아남은 점이 하나도 없으면 키가 생기지 않는다', () => {
    const node = parseOne({ ...MINIMAL, route: 'curve', points: [null, { x: 'a', y: 'b' }] })!;
    expect('points' in node).toBe(false);
  });

  it('상한을 넘으면 **앞에서부터** 살린다 — 전부 버리면 선이 사라진다', () => {
    const many = Array.from({ length: MAX_PATH_COMMANDS + 50 }, (_v, i) => ({ x: i, y: i }));
    const points = parseOne({ ...MINIMAL, route: 'free', points: many })?.points;
    expect(points).toHaveLength(MAX_PATH_COMMANDS);
    expect(points?.[0]).toEqual({ x: 0, y: 0 });
  });

  it('중간점은 `route` 와 무관하게 읽힌다 — `route` 는 **그리기만** 가른다', () => {
    for (const route of ['straight', 'elbow', 'curve', 'free'] as const) {
      const node = parseOne({ ...MINIMAL, route, points: [{ x: 7, y: 8 }] })!;
      expect(node.points, route).toEqual([{ x: 7, y: 8 }]);
    }
  });
});

// --- AC-83 의 전제: 연결선을 쓰지 않은 config 가 이전과 같다 (REQ-09) ---------

describe('연결선을 쓰지 않은 config 가 **키 집합까지** 이전과 같다 (REQ-09 · AC-83 전제)', () => {
  // 011 이전의 다섯 종 + 그룹을 한 장면에 담은 것이다. 이 왕복이 흔들리면 011 은 쓰지도
  // 않은 기능으로 기존 대시보드를 바꾼 것이 된다.
  const LEGACY = [
    { id: 'r', kind: 'rect', geometry: { x: 1, y: 2, w: 3, h: 4 }, style: { fill: '#123456' } },
    { id: 'e', kind: 'ellipse', geometry: { x: 5, y: 6, w: 7, h: 8 }, style: {} },
    { id: 'l', kind: 'line', geometry: { x1: 0, y1: 1, x2: 9, y2: 10 }, style: {} },
    { id: 't', kind: 'text', geometry: { x: 11, y: 12 }, style: {}, text: '{value}' },
    {
      id: 'p',
      kind: 'path',
      geometry: { x: 13, y: 14, w: 15, h: 16 },
      path: [
        { c: 'M', x: 0, y: 0 },
        { c: 'L', x: 100, y: 100 },
      ],
      style: {},
      anchors: [{ id: 'a1', x: 10, y: 20 }],
    },
    {
      id: 'g',
      kind: 'group',
      geometry: { x: 17, y: 18, w: 19, h: 20 },
      parts: [{ id: 'part', kind: 'rect', geometry: { x: 0, y: 0, w: 100, h: 100 }, style: {} }],
    },
  ];

  it('왕복에 값이 바뀌지 않는다', () => {
    const once = parseCanvasConfig(cfg(LEGACY));
    const twice = parseCanvasConfig(JSON.parse(JSON.stringify(once)) as unknown);
    expect(twice).toEqual(once);
  });

  it('노드마다 키 집합이 한 글자도 달라지지 않는다', () => {
    const once = parseCanvasConfig(cfg(LEGACY));
    const keysOf = (node: object): string[] => Object.keys(node).sort();
    expect(once.elements.map((n) => [n.id, keysOf(n)])).toEqual([
      ['r', ['geometry', 'id', 'kind', 'style']],
      ['e', ['geometry', 'id', 'kind', 'style']],
      ['l', ['geometry', 'id', 'kind', 'style']],
      ['t', ['geometry', 'id', 'kind', 'style', 'text']],
      ['p', ['anchors', 'geometry', 'id', 'kind', 'path', 'style']],
      ['g', ['geometry', 'id', 'kind', 'parts']],
    ]);
  });

  it('연결선 관련 키가 **하나도** 생기지 않는다', () => {
    const once = parseCanvasConfig(cfg(LEGACY));
    const serialized = JSON.stringify(once);
    for (const key of ['"connector"', '"from"', '"to"', '"route"', '"points"']) {
      expect(serialized.includes(key), key).toBe(false);
    }
  });

  it('연결선이 없으면 `isConnector` 가 어느 노드에도 참이 아니다', () => {
    expect(parseCanvasConfig(cfg(LEGACY)).elements.some(isConnector)).toBe(false);
  });
});
