// 점이 드는 순간 갈래가 함께 오른다 — **문 셋 전부**를 잰다
// (SPEC-CANVAS-011 REQ-04 · REQ-05 · 사용자 신고 2026-09-16).
//
// ## 이 파일이 겨누는 결함
//
// 011 M10 은 직선의 더블클릭을 **말없이** 거절했다(`ROUTE_TAKES_POINTS.straight === false`).
// 직선은 도구 넷 가운데 첫째라 사람이 가장 먼저 긋는 선이고, 그래서 배달된 것은 "꺾이지도
// 않고 왜 안 되는지도 말하지 않는 선" 이었다.
//
// 고침은 거절을 없애고 **승격**을 두는 것이다. 그 고침이 지키기로 한 문장은 종전과 한 글자도
// 다르지 않다:
//
//   > `route === 'straight'` 인 연결선은 `points` 를 **결코** 들지 않는다.
//
// `connectorPath` 가 `curve` 가 아닌 갈래를 점 목록 그대로 이어 그리기 때문이다 — 직선이
// 점을 하나라도 들면 그 선은 폴리라인으로 그려지고, REQ-04("두 끝을 직선으로 잇는다")가 그
// 순간 거짓이 된다. 그 거짓은 예외도 경고도 없이 **화면에서만** 드러난다.
//
// ## 왜 문을 **세어서** 재는가
//
// 불변식은 그것을 깰 수 있는 자리를 **전부** 막을 때만 사실이다. 한 문만 열려 있으면 그것은
// 지켜지기를 바라는 문장일 뿐이고, 그 문으로 들어온 값 하나가 조용히 화면을 거짓으로 만든다.
//
// 연결선에 `points` 를 **쓰는** 자리는 셋이고 이 파일이 셋을 모두 연다:
//
//   1. **만드는 쪽** — `appendConnector` (M8 · 자유선 M11 이 궤적을 싣는 문).
//   2. **고치는 쪽** — `insertPointAt` (M10 · 사용자가 잉크를 두 번 누르는 문).
//   3. **읽어 들이는 쪽** — `parseConnector` (손으로 적은 config 가 들어오는 문).
//
// `moveConnectorPoint` 는 넷째 문이 아니다 — 있는 점을 옮길 뿐 **첫 점을 만들지 못한다**
// (없는 색인은 그대로 돌아간다). `removePointAt` 도 빼기만 한다.
//
// 세는 일도 함께 한다: 갈래 목록과 표의 키가 같은지를 재므로, 다섯째 `route` 가 느는 날 이
// 파일이 "그 갈래의 점은 어디 사는가" 를 묻지 않은 채 지나가지 않는다.
//
// @spec SPEC-CANVAS-011 REQ-04 · REQ-05 · REQ-05-b

import { describe, expect, it } from 'vitest';

import { parseCanvasConfig, type PointGeometry } from '../canvasConfig';
import { appendConnector, moveConnectorPoint } from '../canvasElementFactory';
import type { CanvasNode } from '../group/groupTypes';
import { insertPointAt, removePointAt } from './connectorEdit';
import {
  CONNECTOR_KIND,
  isConnector,
  ROUTE_POINT_HOST,
  routeHosting,
  type ConnectorElement,
  type ConnectorRoute,
} from './connectorTypes';

// --- 고정 입력 -------------------------------------------------------------

/** 갈래 넷. **표의 키에서 파생시킨다** — 손으로 적으면 다섯째가 늘 때 조용히 넷만 잰다. */
const ROUTES = Object.keys(ROUTE_POINT_HOST) as ConnectorRoute[];

const ENDS = {
  from: { el: 'r1', a: 'e' },
  to: { el: 'r2', a: 'w' },
} as const;

function connector(route: ConnectorRoute, points?: PointGeometry[]): ConnectorElement {
  return {
    id: 'c1',
    kind: CONNECTOR_KIND,
    ...ENDS,
    route,
    ...(points !== undefined ? { points } : {}),
  };
}

/** 저술 · 파싱을 통틀어 "직선이면서 점을 든" 연결선이 하나라도 있는가. */
function breaksInvariant(node: ConnectorElement): boolean {
  return node.route === 'straight' && (node.points?.length ?? 0) > 0;
}

// --- 표 자신 ---------------------------------------------------------------

describe('`ROUTE_POINT_HOST` 가 갈래 전량에 답한다', () => {
  it('키가 갈래 **다섯**과 정확히 같다 — 빠진 갈래도 남은 갈래도 없다 (015)', () => {
    // 015 가 직각을 더해 넷이 다섯이 되었다. 이 파일이 지키는 것은 수가 아니라 **표가
    // 갈래 전량을 덮는다**는 사실이고, 그 사실은 갈래가 늘어도 그대로다.
    expect([...ROUTES].sort()).toEqual(['curve', 'elbow', 'free', 'ortho', 'straight']);
  });

  it('가리키는 갈래는 **제 점을 들 수 있는** 갈래다 — 승격이 한 번에 끝난다', () => {
    // 승격의 목적지가 또 승격해야 하는 갈래면 "한 번 올려도 여전히 못 드는" 상태가 열리고,
    // 그 상태는 부르는 쪽이 고정점을 찾을 때까지 돌게 만든다.
    for (const route of ROUTES) {
      const host = ROUTE_POINT_HOST[route];
      expect(ROUTE_POINT_HOST[host], route).toBe(host);
    }
  });

  it('직선만 제 이름이 아니다 — 나머지 셋은 제 점을 제가 든다', () => {
    expect(ROUTE_POINT_HOST.straight).toBe('elbow');
    expect(ROUTES.filter((r) => ROUTE_POINT_HOST[r] !== r)).toEqual(['straight']);
  });

  it('점이 없으면 아무것도 바꾸지 않는다 — 빈 선의 갈래는 사용자가 고른 그대로다', () => {
    for (const route of ROUTES) {
      expect(routeHosting(route, 0), route).toBe(route);
    }
  });
});

// --- 문 1: 만드는 쪽 --------------------------------------------------------

describe('문 1 — `appendConnector` 는 점을 받으면 갈래도 함께 죈다', () => {
  it.each(ROUTES)('%s 로 점 둘을 실으면 그 점이 사는 갈래로 선다', (route) => {
    const { created } = appendConnector([], ENDS.from, ENDS.to, route, [
      { x: 10, y: 10 },
      { x: 20, y: 20 },
    ]);
    expect(created.route).toBe(ROUTE_POINT_HOST[route]);
    expect(breaksInvariant(created)).toBe(false);
  });

  it.each(ROUTES)('%s 로 점 없이 만들면 갈래가 그대로다 — 승격은 점이 드는 일이다', (route) => {
    const { created } = appendConnector([], ENDS.from, ENDS.to, route);
    expect(created.route).toBe(route);
    expect(created.points).toBeUndefined();
  });
});

// --- 문 2: 고치는 쪽 --------------------------------------------------------

describe('문 2 — `insertPointAt` 은 점과 갈래를 **한 객체에서** 바꾼다', () => {
  it.each(ROUTES)('%s 위의 첫 점이 갈래를 함께 올린다', (route) => {
    const next = insertPointAt(connector(route), 0, { x: 5, y: 5 });
    expect(next.points).toEqual([{ x: 5, y: 5 }]);
    expect(next.route).toBe(ROUTE_POINT_HOST[route]);
    expect(breaksInvariant(next)).toBe(false);
  });

  it('직선이 꺾은 선이 된다 — 신고된 그 몸짓의 산술', () => {
    const next = insertPointAt(connector('straight'), 0, { x: 250, y: 150 });
    expect(next.route).toBe('elbow');
    expect(next.points).toEqual([{ x: 250, y: 150 }]);
  });

  it('자리가 범위 밖이면 갈래도 그대로다 — 하지 않은 일이 갈래를 바꾸지 않는다', () => {
    const c = connector('straight');
    expect(insertPointAt(c, -1, { x: 0, y: 0 })).toBe(c);
    expect(insertPointAt(c, 1, { x: 0, y: 0 })).toBe(c);
  });

  it('마지막 점을 빼도 **강등되지 않는다** — 되돌릴 값을 알 수 없기 때문이다', () => {
    const promoted = insertPointAt(connector('straight'), 0, { x: 250, y: 150 });
    const flat = removePointAt(promoted, 0);
    expect(flat.points, '키가 지워진다').toBeUndefined();
    expect(flat.route, '갈래는 그 자리다').toBe('elbow');
    // 대가를 숨기지 않는다: 값은 되돌아오지 않는다. 화면이 같은 것이 그 대가를 갚는다 —
    // 점 없는 `elbow` 는 직선과 같은 그림이다(AC-50).
    expect(flat.route).not.toBe('straight');
  });

  it('점을 옮기는 일은 넷째 문이 아니다 — 없는 점은 만들어지지 않는다', () => {
    const els: readonly CanvasNode[] = [connector('straight')];
    const next = moveConnectorPoint(els, 'c1', 0, { x: 9, y: 9 });
    const c = next.find(isConnector)!;
    expect(c.points).toBeUndefined();
    expect(c.route).toBe('straight');
  });
});

// --- 문 3: 읽어 들이는 쪽 ---------------------------------------------------

describe('문 3 — `parseConnector` 도 같은 문을 지난다', () => {
  const parse = (raw: unknown): ConnectorElement => {
    const parsed = parseCanvasConfig({ elements: [raw] });
    const node = parsed.elements.find(isConnector);
    if (node === undefined) throw new Error('연결선이 살아남지 않았다');
    return node;
  };

  it('손으로 적은 `straight` + `points` 는 **점을 버리지 않고** 갈래가 오른다', () => {
    // 버리는 쪽을 고르지 않는 근거: 사용자가 찍어 둔 꺾임은 되돌릴 길이 없지만 그리는
    // 법은 한 번 더 고르면 된다(`parseConnectorRoute` 가 모르는 값을 버리지 않는 그 방향).
    const c = parse({
      id: 'c1',
      kind: CONNECTOR_KIND,
      ...ENDS,
      route: 'straight',
      points: [
        { x: 30, y: 40 },
        { x: 50, y: 60 },
      ],
    });
    expect(c.route).toBe('elbow');
    expect(c.points).toEqual([
      { x: 30, y: 40 },
      { x: 50, y: 60 },
    ]);
  });

  it('모르는 `route` + `points` 도 마찬가지다 — 직선으로 떨어진 뒤 승격한다 (AC-37)', () => {
    const c = parse({
      id: 'c1',
      kind: CONNECTOR_KIND,
      ...ENDS,
      route: 'zigzag',
      points: [{ x: 1, y: 2 }],
    });
    expect(c.route).toBe('elbow');
    expect(breaksInvariant(c)).toBe(false);
  });

  it('점이 전부 손상되면 갈래는 그대로다 — 살아남은 점이 없으면 승격도 없다', () => {
    const c = parse({
      id: 'c1',
      kind: CONNECTOR_KIND,
      ...ENDS,
      route: 'straight',
      points: ['쓰레기', null, 7],
    });
    expect(c.points).toBeUndefined();
    expect(c.route).toBe('straight');
  });

  it.each(ROUTES)('%s 는 점 없이 저장 왕복을 그대로 견딘다', (route) => {
    const c = parse({ id: 'c1', kind: CONNECTOR_KIND, ...ENDS, route });
    expect(c.route).toBe(route);
  });
});

// --- 불변식: 저장 왕복까지 ---------------------------------------------------

describe('직선은 **어느 문으로도** 점을 들지 못한다 (REQ-04)', () => {
  /** 저장 → 읽기 한 바퀴. 저술한 값이 파일을 지나 그대로 돌아오는지를 본다. */
  const roundTrip = (node: ConnectorElement): ConnectorElement => {
    const parsed = parseCanvasConfig(JSON.parse(JSON.stringify({ elements: [node] })));
    const back = parsed.elements.find(isConnector);
    if (back === undefined) throw new Error('왕복에서 연결선이 사라졌다');
    return back;
  };

  it.each(ROUTES)('%s: 세 문이 낸 값 전부가 불변식을 지킨다', (route) => {
    const made = appendConnector([], ENDS.from, ENDS.to, route, [{ x: 7, y: 8 }]).created;
    const edited = insertPointAt(connector(route), 0, { x: 7, y: 8 });
    const loaded = roundTrip(connector(route, [{ x: 7, y: 8 }]));

    for (const [door, node] of [
      ['append', made],
      ['insert', edited],
      ['parse', loaded],
    ] as const) {
      expect(breaksInvariant(node), door).toBe(false);
      expect(node.points, door).toHaveLength(1);
    }
  });

  it('승격해 저장한 선은 **그 갈래로** 돌아온다 — 왕복이 값을 되돌리지 않는다', () => {
    const promoted = insertPointAt(connector('straight'), 0, { x: 250, y: 150 });
    const back = roundTrip(promoted);
    expect(back.route).toBe('elbow');
    expect(back.points).toEqual([{ x: 250, y: 150 }]);
  });

  it('점을 다 뺀 뒤 저장해도 `elbow` 그대로다 — 강등이 파서 쪽에도 없다', () => {
    const flat = removePointAt(insertPointAt(connector('straight'), 0, { x: 1, y: 1 }), 0);
    expect(roundTrip(flat).route).toBe('elbow');
  });
});
