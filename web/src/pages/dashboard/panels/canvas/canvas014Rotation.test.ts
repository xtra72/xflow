// 회전 필드의 저장 규율 (SPEC-CANVAS-014 M1 · AC-01 · AC-03 · AC-04 · AC-05).
//
// **이 파일이 막는 가장 큰 실패는 새 기능이 아니라 옛 저장이 흔들리는 것이다.** 파서가
// `rotation: 0` 을 만들어 채우면 저장 왕복 한 번에 **모든 요소가 키를 하나씩 얻는다.**
//
// @spec SPEC-CANVAS-014 REQ-01 · REQ-08

import { describe, expect, it } from 'vitest';

import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { parseCanvasConfig, type CanvasElement } from './canvasConfig';
import { stageLattice } from './canvasGeometry';
import { outlineAabb, outlineAngle, outlineBox } from './canvasOutline';
import { isGroup, type CanvasNode } from './group/groupTypes';

const OUTLINE_SRC = 'src/pages/dashboard/panels/canvas/canvasOutline.ts';
const OVERLAY_SRC = 'src/pages/dashboard/panels/canvas/CanvasEditOverlay.tsx';
const TRANSFORM_SRC = 'src/pages/dashboard/panels/canvas/canvasTransformNodes.ts';

const BOX = { x: 31, y: 57, w: 140, h: 90 } as const;

function config(nodes: readonly Record<string, unknown>[]): Record<string, unknown> {
  return { canvas: { width: 500, height: 400 }, elements: nodes.map((n) => ({ ...n })) };
}

function first(nodes: readonly Record<string, unknown>[]): CanvasNode {
  const out = parseCanvasConfig(config(nodes)).elements[0];
  if (out === undefined) throw new Error('요소가 사라졌다');
  return out;
}

function rotationOf(node: CanvasNode): number | undefined {
  return 'rotation' in node ? (node as { rotation?: number }).rotation : undefined;
}

describe('왕복 (AC-01)', () => {
  it.each(['rect', 'ellipse', 'path', 'text'] as const)('`%s` 가 각도를 싣는다', (kind) => {
    const raw: Record<string, unknown> = {
      id: 'el-1',
      kind,
      geometry: kind === 'text' ? { x: 100, y: 100 } : { ...BOX },
      rotation: 30,
    };
    if (kind === 'path') raw.path = [{ c: 'M', x: 0, y: 0 }, { c: 'L', x: 100, y: 100 }];
    expect(rotationOf(first([raw]))).toBe(30);
  });

  it('그룹도 각도를 싣는다', () => {
    const g = first([{ id: 'g1', kind: 'group', geometry: { ...BOX }, parts: [], rotation: 45 }]);
    expect(isGroup(g)).toBe(true);
    expect(rotationOf(g)).toBe(45);
  });

  it('정규화를 지나서 실린다', () => {
    expect(rotationOf(first([{ id: 'a', kind: 'rect', geometry: { ...BOX }, rotation: -90 }]))).toBe(
      270,
    );
  });
});

describe('0 은 키를 만들지 않는다 (AC-04 · §결정 7)', () => {
  it.each([0, 360, -360])('`rotation: %o` 는 키가 없다', (v) => {
    expect('rotation' in first([{ id: 'a', kind: 'rect', geometry: { ...BOX }, rotation: v }])).toBe(
      false,
    );
  });

  it('**014 이전 저장이 바이트 동일하게 왕복한다**', () => {
    // 이 단언이 §결정 7 의 전부다. 파서가 0 을 만들어 채우면 여기가 빨개진다.
    const legacy = config([
      { id: 'a', kind: 'rect', geometry: { ...BOX }, style: { fill: '#123456' } },
      { id: 'b', kind: 'line', geometry: { x1: 1, y1: 2, x2: 3, y2: 4 }, style: {} },
      { id: 'c', kind: 'text', geometry: { x: 10, y: 20 }, style: {}, text: 'hi' },
      { id: 'g', kind: 'group', geometry: { ...BOX }, parts: [] },
    ]);
    const once = parseCanvasConfig(legacy);
    const twice = parseCanvasConfig(JSON.parse(JSON.stringify(once)));
    expect(JSON.stringify(once)).toBe(JSON.stringify(twice));
    // 그리고 어느 노드에도 `rotation` 이 생기지 않았다.
    expect(JSON.stringify(once)).not.toContain('rotation');
  });
});

describe('손상된 값은 키를 버린다 (AC-03 · REQ-08)', () => {
  it.each([Number.NaN, '30', null, [], { deg: 30 }] as unknown[])('%o 는 미지정이다', (bad) => {
    const el = first([
      { id: 'a', kind: 'rect', geometry: { ...BOX }, style: { fill: '#abc' }, rotation: bad },
    ]) as CanvasElement;
    expect(rotationOf(el)).toBeUndefined();
    // **요소를 통째로 떨어뜨리지 않는다** — 001 이래의 파서 규율이다.
    expect(el.style.fill).toBe('#abc');
    expect('w' in el.geometry && el.geometry.w).toBe(BOX.w);
  });
});

describe('`line` 에는 필드가 서지 않는다 (AC-05 · D6)', () => {
  it('손으로 적어 넣어도 읽는 순간 사라진다', () => {
    // 선의 임의 각도는 두 끝점으로 표현된다. 필드를 주면 **같은 그림을 두 가지로 적을 수**
    // 있게 되고, 그때 어느 쪽이 참인지 화면이 답하지 못한다. `anchors` 가 상자형에만 서는
    // 것(A11)과 같은 규율이고, 그 금지도 같은 자리(파서의 갈래)가 진다.
    const el = first([
      { id: 'l1', kind: 'line', geometry: { x1: 1, y1: 2, x2: 3, y2: 4 }, rotation: 45 },
    ]);
    expect('rotation' in el).toBe(false);
  });

  it('선의 나머지는 살아 있다', () => {
    const el = first([
      { id: 'l1', kind: 'line', geometry: { x1: 1, y1: 2, x2: 3, y2: 4 }, rotation: 45 },
    ]) as CanvasElement;
    expect('x1' in el.geometry && el.geometry.x2).toBe(3);
  });
});

// --- 전제와 두 상자 (M2 · AC-00 · AC-06 · AC-08 · AC-09) --------------------

describe('투영이 각도를 보존한다 (AC-00 · A1)', () => {
  it('두 축 축척이 **언제나 같다**', () => {
    // **이 가드가 014 전체를 떠받친다.** 두 축이 갈라지면 캔버스 단위의 회전이 화면에서
    // 기울어짐으로 나타나고, 그러면 잡기가 각도 하나를 되돌리는 것으로 성립하지 않는다.
    //
    // 002 0.10.0 이 축척을 하나로 만들었고(`canvasGeometry` §StageCell: "두 값은 **언제나
    // 같다**"), `stageLattice` 가 `scale` 하나로 두 축을 낸다. 그 사실을 값으로 고정한다.
    for (const [outer, canvas, step] of [
      [{ width: 800, height: 600 }, { width: 500, height: 400 }, 25],
      [{ width: 1000, height: 300 }, { width: 500, height: 400 }, 10],
      [{ width: 317, height: 911 }, { width: 640, height: 480 }, 20],
    ] as const) {
      const { stage } = stageLattice(outer, canvas, step);
      if (stage.width === 0 || stage.height === 0) continue;
      const sx = stage.width / canvas.width;
      const sy = stage.height / canvas.height;
      expect(Math.abs(sx - sy), JSON.stringify({ outer, canvas })).toBeLessThan(1e-9);
    }
  });
});

describe('두 상자 (AC-06 · AC-08 · AC-09)', () => {
  const PROJ = { stage: { width: 250, height: 200 }, canvas: { width: 500, height: 400 } };

  function rectNode(rotation?: number): CanvasElement {
    return {
      id: 'r1',
      kind: 'rect',
      style: {},
      geometry: { ...BOX },
      ...(rotation !== undefined ? { rotation } : {}),
    } as CanvasElement;
  }

  it('각도 0 이면 방향 상자와 축-나란 상자가 **같은 수**다 (AC-06 · K3)', () => {
    // 014 이전의 모든 화면이 이 등식 위에 서 있다.
    const el = rectNode();
    expect(outlineAabb(el, PROJ, {})).toEqual(outlineBox(el, PROJ, {}));
    expect(outlineAngle(el)).toBe(0);
  });

  it('`line` 의 각도는 언제나 0 이다 — 필드가 서지 않기 때문이다', () => {
    const line: CanvasElement = {
      id: 'l1',
      kind: 'line',
      style: {},
      geometry: { x1: 1, y1: 2, x2: 3, y2: 4 },
    };
    expect(outlineAngle(line)).toBe(0);
    expect(outlineAabb(line, PROJ, {})).toEqual(outlineBox(line, PROJ, {}));
  });

  it('돌아가면 축-나란 상자가 커진다', () => {
    const plain = outlineBox(rectNode(), PROJ, {});
    const turned = outlineAabb(rectNode(45), PROJ, {});
    expect(turned.w).toBeGreaterThan(plain.w);
    expect(turned.h).toBeGreaterThan(plain.h);
    // 가운데는 그대로다 — 축이 상자 가운데이기 때문이다.
    expect(turned.x + turned.w / 2).toBeCloseTo(plain.x + plain.w / 2, 6);
    expect(turned.y + turned.h / 2).toBeCloseTo(plain.y + plain.h / 2, 6);
  });

  it('축-나란 상자는 방향 상자에서 **파생된다** (AC-08 · K2)', () => {
    // 제 손으로 다시 재면 두 번째 측정이 생기고, 그 갈라짐은 크기나 각도를 바꾼 뒤에야
    // 화면에서만 드러난다 — `anchors.ts` AC-33 의 그 규율이다.
    const src = readFileSync(resolve(process.cwd(), OUTLINE_SRC), 'utf8');
    const body = src.slice(src.indexOf('export function outlineAabb'));
    expect(body).toContain('outlineBox(');
    // 그 안에서 투영을 다시 부르지 않는다(두 번째 측정의 문형이다).
    const fnBody = body.slice(0, body.indexOf('\n}'));
    expect(fnBody).not.toContain('projectBox');
    expect(fnBody).not.toContain('projectPoint');
  });

  it('소비자가 어느 상자를 읽는지 갈라져 있다 (AC-09)', () => {
    // 마키 · 정렬 · 013 의 선택 상자는 **축-나란**, 손잡이 · 앵커 · 선택 윤곽은 **방향**.
    const overlay = readFileSync(resolve(process.cwd(), OVERLAY_SRC), 'utf8');
    expect(overlay).toContain('marqueeCandidates(elements, (el) => outlineAabb(');
    expect(overlay).toContain('box: outlineAabb(');
    const transform = readFileSync(resolve(process.cwd(), TRANSFORM_SRC), 'utf8');
    expect(transform).toContain('outlineAabb(');
  });
});
