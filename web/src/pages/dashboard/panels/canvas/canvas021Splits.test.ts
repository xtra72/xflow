// 구간마다 하나인 고정값 목록 (SPEC-CANVAS-021 M1 · M2).
//
// 019 는 수 하나였다. 논리 구간이 하나뿐일 때는 족했지만, 사용자 점이 `N` 개면 구간은
// `N+1` 개이고 직각 갈래는 **구간마다 Z 를 하나씩** 그린다 — 고정값도 구간마다 하나다.
//
// 몸짓(손잡이·차림표)은 `canvas021Menu.test.tsx` 가 진다.
//
// @spec SPEC-CANVAS-021

import { describe, expect, it } from 'vitest';

import { parseCanvasConfig, type CanvasSize } from './canvasConfig';
import type { CanvasProjection, CanvasPoint } from './canvasGeometry';
import { connectorPath } from './connector/connectorPath';
import { connectorObstacles } from './connector/connectorObstacles';
import { resolveConnector } from './connector/resolveConnector';
import { insertPointAt, removePointAt } from './connector/connectorEdit';
import {
  parseOrthoSplits,
  splitAt,
  splitInserted,
  splitRemoved,
  trimAuto,
  withSplit,
} from './connector/orthoSplits';
import { isConnector, type ConnectorElement } from './connector/connectorTypes';
import type { CanvasNode } from './group/groupTypes';

const CANVAS: CanvasSize = { width: 400, height: 300 };
const PROJ: CanvasProjection = { stage: { width: 400, height: 300 }, canvas: CANVAS };

function scene(over: Record<string, unknown> = {}): CanvasNode[] {
  return parseCanvasConfig({
    canvas: { ...CANVAS },
    elements: [
      { id: 'a', kind: 'rect', geometry: { x: 40, y: 40, w: 80, h: 50 }, style: {} },
      { id: 'b', kind: 'rect', geometry: { x: 230, y: 180, w: 80, h: 50 }, style: {} },
      {
        id: 'c1',
        kind: 'connector',
        from: { el: 'a', a: 'e' },
        to: { el: 'b', a: 'w' },
        route: 'ortho',
        ...over,
      },
    ],
  }).elements;
}

function connectorOf(els: readonly CanvasNode[]): ConnectorElement {
  const c = els.find((n) => n.id === 'c1');
  if (c === undefined || !isConnector(c)) throw new Error('연결선이 없다');
  return c;
}

function resolved(els: readonly CanvasNode[]): readonly CanvasPoint[] {
  const points = resolveConnector(connectorOf(els), els, PROJ, {});
  if (points === undefined) throw new Error('연결이 끊겼다');
  return points;
}

/** 그려진 점들 — `C` 는 이 SPEC 의 장면에 나오지 않는다. */
function drawnPoints(els: readonly CanvasNode[]): { x: number; y: number }[] {
  const c = connectorOf(els);
  return connectorPath(
    resolved(els),
    c.route,
    PROJ,
    connectorObstacles(c, els, PROJ, {}),
    c.ortho_split,
  ).map((cmd) => (cmd.c === 'C' ? { x: NaN, y: NaN } : { x: cmd.x, y: cmd.y }));
}

// --- ① 목록 다루기 ----------------------------------------------------------

describe('목록을 다루는 셈 (§결정 4)', () => {
  it('`null` 이 자동이다 — `0` 은 값이다', () => {
    expect(splitAt([null, 0], 0)).toBeUndefined();
    expect(splitAt([null, 0], 1)).toBe(0);
  });

  it('없는 자리는 자동이다 — 던지지 않는다 (REQ-07)', () => {
    expect(splitAt(undefined, 0)).toBeUndefined();
    expect(splitAt([175], 5)).toBeUndefined();
    expect(splitAt([175], -1)).toBeUndefined();
  });

  it('뒤쪽의 자동은 잘라내고, 전부 자동이면 부재다', () => {
    expect(trimAuto([175, null, null])).toEqual([175]);
    expect(trimAuto([null, 190])).toEqual([null, 190]);
    expect(trimAuto([null, null])).toBeUndefined();
    expect(trimAuto([])).toBeUndefined();
  });

  it('갈아 끼우면 모자란 앞자리가 자동으로 찬다', () => {
    expect(withSplit(undefined, 2, 190)).toEqual([null, null, 190]);
    expect(withSplit([175], 0, 200)).toEqual([200]);
    // 마지막 자리를 자동으로 되돌리면 **목록이 줄어든다**(빈 키를 남기지 않는다).
    expect(withSplit([175, 190], 1, null)).toEqual([175]);
    expect(withSplit([175], 0, null)).toBeUndefined();
  });
});

// --- ② 낡지 않는다 (K1) -----------------------------------------------------

describe('점을 더하고 빼면 목록이 함께 옮긴다 (K1)', () => {
  it('구간을 가르면 **새 두 구간은 모두 자동**이다', () => {
    // 고정값은 "이 구간의 Z 를 여기에 세워라" 이고, 구간이 갈리면 그 Z 는 더 이상 없다.
    // 옛 값을 한쪽에 물려주면 사용자가 찍지 않은 자리에 선이 꺾인다.
    expect(splitInserted([175, 190], 0)).toEqual([null, null, 190]);
    expect(splitInserted([175, 190], 1)).toEqual([175]);
    // 목록이 그 자리에 닿지 못하면 이미 자동이다 — 늘려 둘 까닭이 없다.
    expect(splitInserted([175], 3)).toEqual([175]);
    expect(splitInserted(undefined, 0)).toBeUndefined();
  });

  it('구간을 합치면 **합쳐진 구간도 자동**이다', () => {
    expect(splitRemoved([175, 190, 200], 0)).toEqual([null, 200]);
    expect(splitRemoved([175, 190, 200], 1)).toEqual([175]);
    expect(splitRemoved([175], 3)).toEqual([175]);
    expect(splitRemoved(undefined, 0)).toBeUndefined();
  });

  it('`insertPointAt` 이 목록을 **함께** 옮긴다', () => {
    const c = connectorOf(scene({ points: [{ x: 175, y: 150 }], ortho_split: [175, 190] }));
    const next = insertPointAt(c, 0, { x: 150, y: 65 });
    expect(next.points).toHaveLength(2);
    expect(next.ortho_split).toEqual([null, null, 190]);
  });

  it('`removePointAt` 이 목록을 **함께** 옮긴다', () => {
    const c = connectorOf(scene({ points: [{ x: 175, y: 150 }], ortho_split: [175, 190] }));
    const next = removePointAt(c, 0);
    expect(next.points).toBeUndefined();
    // 두 구간이 하나로 합쳐졌고 그 하나는 자동이다 — 전부 자동이므로 키가 없다.
    expect('ortho_split' in next).toBe(false);
  });

  it('마지막 점을 빼도 **키가 남지 않는다**', () => {
    const c = connectorOf(scene({ points: [{ x: 175, y: 150 }], ortho_split: [null, 190] }));
    expect('ortho_split' in removePointAt(c, 0)).toBe(false);
  });
});

// --- ③ 저장 규율 (REQ-07) ---------------------------------------------------

describe('저장에서 읽어 들인다 (REQ-07)', () => {
  it('019 가 적은 **수 하나도 그대로 읽는다**', () => {
    expect(parseOrthoSplits(175)).toEqual([175]);
    expect(connectorOf(scene({ ortho_split: 175 })).ortho_split).toEqual([175]);
  });

  it('손상된 자리는 **그 자리만** 자동이 된다', () => {
    expect(parseOrthoSplits([175, 'x', null, Number.NaN, 190])).toEqual([175, null, null, null, 190]);
    expect(connectorOf(scene({ ortho_split: [175, 'x'] })).ortho_split).toEqual([175]);
  });

  it.each([Number.NaN, Number.POSITIVE_INFINITY, 'x', {}, true, null])(
    '%s — 목록이 아니고 수도 아니면 키를 버린다',
    (bad) => {
      expect(parseOrthoSplits(bad)).toBeUndefined();
    },
  );

  it('직각이 아니면 키를 만들지 않는다 (019 그대로)', () => {
    expect(connectorOf(scene({ route: 'elbow', ortho_split: [175] })).ortho_split).toBeUndefined();
  });

  it('왕복한다', () => {
    const els = scene({ points: [{ x: 175, y: 150 }], ortho_split: [175, 190] });
    const again = parseCanvasConfig(JSON.parse(JSON.stringify({ canvas: { ...CANVAS }, elements: els })));
    expect(connectorOf(again.elements).ortho_split).toEqual([175, 190]);
  });
});

// --- ④ 구간마다 고정이 걸린다 (REQ-05) --------------------------------------

describe('구간마다 제 고정값을 읽는다 (REQ-05)', () => {
  it('점이 없으면 019 와 **같은 답**이다 (K2)', () => {
    expect(drawnPoints(scene({ ortho_split: [200] })).map((p) => p.x)).toEqual([
      120, 200, 200, 229.99999999999997,
    ]);
  });

  it('점이 하나면 **두 구간이 따로** 고정된다', () => {
    const els = scene({ points: [{ x: 175, y: 150 }], ortho_split: [140, 210] });
    const xs = drawnPoints(els).map((p) => p.x);
    // 첫 구간의 Z 는 x=140, 둘째 구간의 Z 는 x=210 에 선다.
    expect(xs).toContain(140);
    expect(xs).toContain(210);
  });

  it('한 구간만 고정하면 **다른 구간은 자동**이다', () => {
    const pinned = drawnPoints(scene({ points: [{ x: 175, y: 150 }], ortho_split: [140] }));
    const auto = drawnPoints(scene({ points: [{ x: 175, y: 150 }] }));
    expect(pinned.map((p) => p.x)).toContain(140);
    // 둘째 구간의 모서리는 고정하지 않은 쪽과 **같은 자리**다.
    expect(pinned.at(-2)).toEqual(auto.at(-2));
  });

  it('목록이 구간 수보다 길어도 던지지 않는다 (REQ-07)', () => {
    expect(() => drawnPoints(scene({ ortho_split: [200, 210, 220] }))).not.toThrow();
  });
});
