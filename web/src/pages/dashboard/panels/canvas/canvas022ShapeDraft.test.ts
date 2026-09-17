// 선을 그어 짓는 도형 — 순수 층 (SPEC-CANVAS-022 M1 · M2).
//
// 몸짓(모드·찍기·닫기·버리기)은 `canvas022ShapeGesture.test.tsx` 가 진다.
//
// @spec SPEC-CANVAS-022

import { describe, expect, it } from 'vitest';

import { MIN_ELEMENT_EXTENT, type CanvasSize, type PointGeometry } from './canvasConfig';
import { appendDrawnPath } from './canvasElementFactory';
import { SHAPE_MIN_VERTICES, draftBox, draftCommands, identityProjection, shapeDraftOf } from './shapeDraft';
import { isConnector } from './connector/connectorTypes';
import type { PathCommand } from './shapes/pathTypes';
import type { CanvasNode } from './group/groupTypes';

const CANVAS: CanvasSize = { width: 400, height: 300 };

/** 삼각형 — 가장 작은 진짜 도형이다. */
const TRIANGLE: PointGeometry[] = [
  { x: 100, y: 100 },
  { x: 200, y: 100 },
  { x: 150, y: 200 },
];

// --- ① 투영 (§결정 3) -------------------------------------------------------

describe('캔버스 단위에서 정한다 (§결정 3)', () => {
  it('항등 투영은 무대와 캔버스가 같다 — 왕복이 없다', () => {
    const proj = identityProjection(CANVAS);
    expect(proj.stage).toEqual({ width: CANVAS.width, height: CANVAS.height });
    expect(proj.canvas).toBe(CANVAS);
  });

  it('곧은선 윤곽의 명령이 **찍은 자리 그대로**다', () => {
    expect(draftCommands(TRIANGLE, 'straight', CANVAS)).toEqual([
      { c: 'M', x: 100, y: 100 },
      { c: 'L', x: 200, y: 100 },
      { c: 'L', x: 150, y: 200 },
    ]);
  });
});

// --- ② 모양은 연결선이 낸다 (§결정 2 · K3) ----------------------------------

describe('윤곽의 모양이 `connectorPath` 에서 난다 (K3)', () => {
  it('직각을 켜 두면 **꺾여서** 이어진다 — 명령이 는다', () => {
    const straight = draftCommands(TRIANGLE, 'straight', CANVAS);
    const ortho = draftCommands(TRIANGLE, 'ortho', CANVAS);
    expect(ortho.length).toBeGreaterThan(straight.length);
    // 모든 구간이 한 축 위다 — 직각의 K1 이 도형 윤곽에서도 저절로 참이다.
    const pts = ortho.flatMap((cmd) => (cmd.c === 'Z' || cmd.c === 'C' ? [] : [{ x: cmd.x, y: cmd.y }]));
    for (let i = 0; i + 1 < pts.length; i += 1) {
      const a = pts[i]!;
      const b = pts[i + 1]!;
      expect(a.x === b.x || a.y === b.y).toBe(true);
    }
  });

  it('곡선을 켜 두면 `C` 가 난다', () => {
    expect(draftCommands(TRIANGLE, 'curve', CANVAS).some((cmd) => cmd.c === 'C')).toBe(true);
  });

  it('손그림은 **곧은선과 같은 그림**이다 (§결정 2) — 깨지지 않는다', () => {
    expect(draftCommands(TRIANGLE, 'free', CANVAS)).toEqual(draftCommands(TRIANGLE, 'straight', CANVAS));
  });
});

// --- ③ 상자 (REQ-07 · K5 · §결정 4) -----------------------------------------

describe('윤곽을 감싸는 상자 (K5)', () => {
  it('찍은 자리를 감싼다', () => {
    expect(draftBox(draftCommands(TRIANGLE, 'straight', CANVAS))).toEqual({
      x: 100,
      y: 100,
      w: 100,
      h: 100,
    });
  });

  it('**한 점에 모아도 0 이 되지 않는다** (REQ-07)', () => {
    const dot: PointGeometry[] = [
      { x: 50, y: 50 },
      { x: 50, y: 50 },
      { x: 50, y: 50 },
    ];
    const box = draftBox(draftCommands(dot, 'straight', CANVAS));
    expect(box.w).toBe(MIN_ELEMENT_EXTENT);
    expect(box.h).toBe(MIN_ELEMENT_EXTENT);
  });

  it('빈 목록에도 던지지 않는다', () => {
    expect(draftBox([])).toEqual({ x: 0, y: 0, w: MIN_ELEMENT_EXTENT, h: MIN_ELEMENT_EXTENT });
  });

  it('곡선은 **그려지는 것**을 감싼다 — 찍은 자리의 껍질이 아니다 (§결정 4)', () => {
    // 곡선 갈래는 가운데 꼭짓점을 **끝점으로 쓰지 않는다**(`curveSegments` 가 세 점에서
    // 3차 곡선 하나를 낸다). 그래서 상자가 찍은 자리의 껍질보다 **좁을 수 있다** — 여기서
    // 실제로 그렇다(너비 83 < 100).
    //
    // 그래도 그림은 상자 밖으로 나가지 않는다: 3차 베지에는 제 제어 다각형의 볼록 껍질
    // 안에 있고, 상자는 끝점과 제어점을 모두 감싼다. 재는 것이 "찍은 자리" 가 아니라
    // "그려지는 것" 이라는 사실을 값으로 적어 둔다.
    const curve = draftCommands(TRIANGLE, 'curve', CANVAS);
    const box = draftBox(curve);
    const plain = draftBox(draftCommands(TRIANGLE, 'straight', CANVAS));
    expect(box.w).toBeLessThan(plain.w);
    // 끝점과 제어점이 모두 상자 안이다 — **반 칸의 여유를 둔다.**
    //
    // 상자는 정수 캔버스 단위이므로(`coordinate` 가 반올림한다) 소수 극값은 최대 반 칸까지
    // 밖에 설 수 있다. 그것이 결함이 아닌 근거는 `PathElement` 가 이미 적어 두었다 —
    // 로컬 좌표는 **공칭** 0..`PATH_LOCAL_EXTENT` 이고 clamp 하지 않는다.
    const SLACK = 0.5;
    for (const cmd of curve) {
      if (cmd.c === 'Z') continue;
      const xs = cmd.c === 'C' ? [cmd.x1, cmd.x2, cmd.x] : [cmd.x];
      const ys = cmd.c === 'C' ? [cmd.y1, cmd.y2, cmd.y] : [cmd.y];
      for (const x of xs) {
        expect(x).toBeGreaterThanOrEqual(box.x - SLACK);
        expect(x).toBeLessThanOrEqual(box.x + box.w + SLACK);
      }
      for (const y of ys) {
        expect(y).toBeGreaterThanOrEqual(box.y - SLACK);
        expect(y).toBeLessThanOrEqual(box.y + box.h + SLACK);
      }
    }
  });
});

// --- ④ 닫힌 도형의 재료 (REQ-03 · REQ-06) -----------------------------------

describe('닫힌 도형의 재료 (REQ-03)', () => {
  it.each([0, 1, 2])('꼭짓점 %i 개는 도형이 아니다', (count) => {
    expect(shapeDraftOf(TRIANGLE.slice(0, count), 'straight', CANVAS)).toBeUndefined();
  });

  it('셋이면 지어진다 — 그 수가 `SHAPE_MIN_VERTICES` 다', () => {
    expect(SHAPE_MIN_VERTICES).toBe(3);
    expect(shapeDraftOf(TRIANGLE, 'straight', CANVAS)).toBeDefined();
  });

  it('**`Z` 로 끝난다** — 그 한 줄이 선과 도형을 가른다', () => {
    const draft = shapeDraftOf(TRIANGLE, 'straight', CANVAS)!;
    expect(draft.path.at(-1)).toEqual({ c: 'Z' });
    expect(draft.path.filter((cmd) => cmd.c === 'Z')).toHaveLength(1);
  });

  it('명령이 **상자 로컬 0..10000** 으로 옮겨진다', () => {
    const draft = shapeDraftOf(TRIANGLE, 'straight', CANVAS)!;
    expect(draft.path).toEqual([
      { c: 'M', x: 0, y: 0 },
      { c: 'L', x: 10000, y: 0 },
      { c: 'L', x: 5000, y: 10000 },
      { c: 'Z' },
    ]);
  });

  it('상자는 그린 자리다 — 씨앗 자리로 뛰지 않는다', () => {
    expect(shapeDraftOf(TRIANGLE, 'straight', CANVAS)!.geometry).toEqual({
      x: 100,
      y: 100,
      w: 100,
      h: 100,
    });
  });
});

// --- ⑤ 만들어진 것은 평범한 도형이다 (REQ-06 · K1 · K2) ----------------------

describe('만들어진 것은 평범한 도형이다 (REQ-06)', () => {
  const draft = shapeDraftOf(TRIANGLE, 'straight', CANVAS)!;

  function made(before: readonly CanvasNode[] = []): CanvasNode {
    return appendDrawnPath(before, draft.geometry, draft.path).created;
  }

  it('`kind` 가 `path` 이고 기하가 그린 상자다', () => {
    const el = made();
    expect(el.kind).toBe('path');
    expect(isConnector(el)).toBe(false);
    if (el.kind !== 'path') throw new Error('경로가 아니다');
    expect(el.geometry).toEqual(draft.geometry);
  });

  it('**겉모습이 닫힌 도형의 씨앗**이다 (K1) — 칠이 있고 테두리는 없다', () => {
    const el = made();
    if (el.kind !== 'path') throw new Error('경로가 아니다');
    expect(el.style?.fill).toBeTypeOf('string');
    expect(el.style?.stroke).toBeUndefined();
  });

  it('**`catalog_id` 를 심지 않는다** — 카탈로그에서 나오지 않았다', () => {
    const el = made();
    if (el.kind !== 'path') throw new Error('경로가 아니다');
    expect('catalog_id' in el).toBe(false);
  });

  it('명령을 **사본**으로 싣는다 — 값이지 참조가 아니다', () => {
    const el = made();
    if (el.kind !== 'path') throw new Error('경로가 아니다');
    expect(el.path).not.toBe(draft.path);
    expect(el.path[0]).not.toBe(draft.path[0]);
    expect(el.path).toEqual(draft.path);
  });

  it('배열 **끝**에 붙는다 — 끝이 곧 맨 위다', () => {
    const before: CanvasNode[] = [
      { id: 'e1', kind: 'rect', geometry: { x: 0, y: 0, w: 10, h: 10 }, style: {} },
    ];
    const { next, created } = appendDrawnPath(before, draft.geometry, draft.path);
    expect(next).toHaveLength(2);
    expect(next.at(-1)).toBe(created);
    expect(created.id).not.toBe('e1');
  });

  it('닫히지 않은 명령을 받으면 **열린 씨앗**이 된다 — 규칙이 하나다', () => {
    const open: PathCommand[] = [
      { c: 'M', x: 0, y: 0 },
      { c: 'L', x: 100, y: 100 },
    ];
    const el = appendDrawnPath([], draft.geometry, open).created;
    if (el.kind !== 'path') throw new Error('경로가 아니다');
    expect(el.style?.stroke).toBeTypeOf('string');
    expect(el.style?.fill).toBeUndefined();
  });
});
