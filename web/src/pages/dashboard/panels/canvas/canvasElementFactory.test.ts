// 신규 요소 생성 순수 모듈 테스트 (SPEC-CANVAS-002 T9).
//
// 이 모듈이 지는 약속은 하나다: **어디서 더하든 같은 것이 나온다**(가정 A7). 목록 편집기와
// 캔버스 팔레트가 이 함수를 부르므로, 여기서 재는 성질(씨앗 스타일 · 계단 오프셋 · id 규칙 ·
// 끝에 붙이기)이 두 자리 모두의 성질이 된다.
//
// DOM 무의존이라 jsdom 없이 전량 단위 테스트한다 — `canvasHitTest.ts` ·
// `canvasEditGeometry.ts` 와 같은 규율이다.
//
// 여기서 만든 요소가 **실제로 칠해지는가**(편집기↔렌더 이음매)는 `CanvasElementsEditor.
// test.tsx` 가 `drawElement` 를 직접 불러 이미 재고 있으므로 되풀이하지 않는다.
//
// @spec SPEC-CANVAS-002 REQ-01

import { describe, expect, it } from 'vitest';

import {
  DEFAULT_BOX_GEOMETRY,
  DEFAULT_LINE_GEOMETRY,
  DEFAULT_POINT_GEOMETRY,
  type CanvasElement,
  type CanvasElementKind,
} from './canvasConfig';
import {
  appendElement,
  newElement,
  nextElementId,
  seedOffset,
  SEED_COLOR,
  SEED_STROKE_WIDTH,
  SEED_TEXT,
} from './canvasElementFactory';

const KINDS: readonly CanvasElementKind[] = ['rect', 'ellipse', 'line', 'text'];

/** 사각형 하나(주어진 id 로). 배열을 만드는 데만 쓴다. */
function rect(id: string): CanvasElement {
  return { id, kind: 'rect', geometry: { x: 0, y: 0, w: 0.1, h: 0.1 }, style: {} };
}

// --- id 규칙 -------------------------------------------------------------

describe('canvasElementFactory — nextElementId', () => {
  it('빈 배열에서는 el-1 이다', () => {
    expect(nextElementId([])).toBe('el-1');
  });

  it('이미 쓰인 id 를 피해 결정적으로 붙는다', () => {
    expect(nextElementId([rect('el-1'), rect('el-2')])).toBe('el-3');
  });

  it('사용자가 지은 id 사이의 빈자리를 채운다 — 순번이 아니라 미사용 id 다', () => {
    // `el-1` 이 지워진 뒤라면 그 자리를 다시 쓴다. 배열 길이로 이름을 지으면 충돌한다.
    expect(nextElementId([rect('el-2'), rect('el-3')])).toBe('el-1');
  });

  it('001 식이 아닌 id 는 셈에 끼어들지 않는다', () => {
    expect(nextElementId([rect('보일러'), rect('pump-a')])).toBe('el-1');
  });
});

// --- 계단 오프셋 ---------------------------------------------------------

describe('canvasElementFactory — seedOffset', () => {
  it('한 칸은 0.05 이고 요소 수에 비례해 커진다', () => {
    expect(seedOffset(0)).toBe(0);
    expect(seedOffset(1)).toBeCloseTo(0.05, 10);
    expect(seedOffset(3)).toBeCloseTo(0.15, 10);
  });

  it('여덟 칸에서 되감긴다 — 계단이 스테이지 밖으로 행진하지 않는다', () => {
    expect(seedOffset(8)).toBe(seedOffset(0));
    expect(seedOffset(9)).toBeCloseTo(seedOffset(1), 10);
  });
});

// --- 씨앗 요소 -----------------------------------------------------------

describe('canvasElementFactory — newElement', () => {
  it('네 종류 모두 보이는 스타일을 심는다 — 더했는데 빈 캔버스가 되지 않는다', () => {
    const hex = /^#[0-9a-f]{6}$/i;
    expect(newElement('a', 'rect', 0).style.fill).toMatch(hex);
    expect(newElement('a', 'ellipse', 0).style.fill).toMatch(hex);

    const line = newElement('a', 'line', 0);
    // 선은 열린 경로라 채움이 뜻이 없다 — 색만으로 부족하고 두께가 함께 있어야 그어진다.
    expect(line.style.stroke).toBe(SEED_COLOR);
    expect(line.style.strokeWidth).toBe(SEED_STROKE_WIDTH);

    const text = newElement('a', 'text', 0);
    expect(text.style.textColor).toBe(SEED_COLOR);
    // 색만 심으면 여전히 보이지 않는다 — 그릴 글자가 없기 때문이다.
    expect(text.text).toBe(SEED_TEXT);
  });

  it('심어 둔 문구는 토큰 3종을 그대로 보여 문구 칸이 곧 사용법이 된다', () => {
    expect(SEED_TEXT).toContain('{name}');
    expect(SEED_TEXT).toContain('{value}');
    expect(SEED_TEXT).toContain('{unit}');
  });

  it('오프셋 0 이면 그 종류의 기본 기하 그대로다', () => {
    expect(newElement('a', 'rect', 0).geometry).toEqual(DEFAULT_BOX_GEOMETRY);
    expect(newElement('a', 'ellipse', 0).geometry).toEqual(DEFAULT_BOX_GEOMETRY);
    expect(newElement('a', 'line', 0).geometry).toEqual(DEFAULT_LINE_GEOMETRY);
    expect(newElement('a', 'text', 0).geometry).toEqual(DEFAULT_POINT_GEOMETRY);
  });

  it('상자는 대각선으로, 선과 문구는 여유가 있는 세로 축으로만 내려온다', () => {
    // 가로는 선이 이미 스테이지를 가로지르고 문구는 오른쪽으로 흐르므로 건드리지 않는다.
    expect(newElement('a', 'rect', 1).geometry).toEqual({ x: 0.15, y: 0.15, w: 0.2, h: 0.2 });
    expect(newElement('a', 'line', 1).geometry).toEqual({ x1: 0.1, y1: 0.55, x2: 0.9, y2: 0.55 });
    expect(newElement('a', 'text', 1).geometry).toEqual({ x: 0.5, y: 0.55 });
  });

  it('계단이 붙어도 좌표에 부동소수 찌꺼기가 남지 않는다', () => {
    // 0.1 + 0.15 를 그대로 두면 0.25000000000000006 이 숫자 칸에 그대로 뜬다.
    expect((newElement('a', 'rect', 3).geometry as { x: number }).x).toBe(0.25);
  });

  it('id 와 kind 는 받은 것을 그대로 쓴다', () => {
    for (const kind of KINDS) {
      const el = newElement('보일러-1', kind, 0);
      expect(el.id).toBe('보일러-1');
      expect(el.kind).toBe(kind);
    }
  });
});

// --- 배열에 붙이기 -------------------------------------------------------

describe('canvasElementFactory — appendElement', () => {
  it('끝에 붙인다 — 배열 순서가 z-order 이므로 새로 만든 것이 맨 위다', () => {
    const before = [rect('el-1')];
    const { next, created } = appendElement(before, 'rect');
    expect(next).toHaveLength(2);
    expect(next[1]).toBe(created);
    expect(created.id).toBe('el-2');
  });

  it('원본 배열을 건드리지 않는다', () => {
    const before = [rect('el-1')];
    appendElement(before, 'rect');
    expect(before).toHaveLength(1);
  });

  it('기존 요소의 좌표는 건드리지 않는다 — 스테이지 밖 저술은 합법이다', () => {
    const far: CanvasElement = {
      id: 'far',
      kind: 'rect',
      geometry: { x: -0.5, y: 2, w: 3, h: 4 },
      style: {},
    };
    const { next } = appendElement([far], 'rect');
    expect(next[0]!.geometry).toEqual({ x: -0.5, y: 2, w: 3, h: 4 });
  });

  it('연속으로 붙이면 자리가 겹치지 않는다 — 계단이 배열 길이를 보기 때문이다', () => {
    let els: CanvasElement[] = [];
    for (let i = 0; i < 3; i++) els = appendElement(els, 'rect').next;
    const geos = els.map((e) => JSON.stringify(e.geometry));
    expect(new Set(geos).size).toBe(3);
    expect(geos[0]).toBe(JSON.stringify({ x: 0.1, y: 0.1, w: 0.2, h: 0.2 }));
    expect(geos[1]).toBe(JSON.stringify({ x: 0.15, y: 0.15, w: 0.2, h: 0.2 }));
    expect(geos[2]).toBe(JSON.stringify({ x: 0.2, y: 0.2, w: 0.2, h: 0.2 }));
  });

  it('아홉 번을 붙여도 스테이지 밖으로 행진하지 않는다', () => {
    let els: CanvasElement[] = [];
    for (let i = 0; i < 9; i++) els = appendElement(els, 'rect').next;
    for (const el of els) {
      for (const v of Object.values(el.geometry as unknown as Record<string, number>)) {
        expect(v).toBeGreaterThanOrEqual(0);
        expect(v).toBeLessThanOrEqual(1);
      }
    }
    // 아홉 번째는 첫 번째 자리로 되감긴다(되감기 폭 8).
    expect(els[8]!.geometry).toEqual(els[0]!.geometry);
  });
});
