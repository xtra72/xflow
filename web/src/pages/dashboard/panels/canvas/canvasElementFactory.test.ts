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
  DEFAULT_CANVAS_SIZE,
  DEFAULT_LINE_GEOMETRY,
  DEFAULT_POINT_GEOMETRY,
  type CanvasElement,
  type CanvasPrimitiveKind,
} from './canvasConfig';
import {
  appendElement,
  newElement,
  nextElementId,
  seedOffset,
  SEED_COLOR,
  SEED_STROKE_WIDTH,
  SEED_TEXT,
  SEED_TEXT_COLOR,
  withElementText,
} from './canvasElementFactory';

// `newElement` 가 받는 것은 **원시형 넷**이다. 경로는 명령 목록 없이 만들어지지 않으므로
// 이 목록에 오지 않는다(그 사실 자체를 아래 §원시형만 받는다 절이 형상으로 잰다).
const KINDS: readonly CanvasPrimitiveKind[] = ['rect', 'ellipse', 'line', 'text'];

/** 도형 3종 — 라벨이 붙을 수 있으나 `textColor` 없이는 칠해지지 않는 종류들이다. */
const SHAPE_KINDS = ['rect', 'ellipse', 'line'] as const;

/** 사각형 하나(주어진 id 로). 배열을 만드는 데만 쓴다. */
function rect(id: string): CanvasElement {
  return { id, kind: 'rect', geometry: { x: 0, y: 0, w: 50, h: 40 }, style: {} };
}

/** 종류별 도형 하나. 씨앗이 심는 것과 같은 스타일을 입되 글자색은 없다. */
function shape(kind: (typeof SHAPE_KINDS)[number]): CanvasElement {
  return kind === 'line'
    ? {
        id: 's1',
        kind: 'line',
        geometry: { x1: 50, y1: 200, x2: 450, y2: 200 },
        style: { stroke: SEED_COLOR, strokeWidth: SEED_STROKE_WIDTH },
      }
    : { id: 's1', kind, geometry: { x: 50, y: 40, w: 100, h: 80 }, style: { fill: SEED_COLOR } };
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
  it('한 칸은 25 단위이고 요소 수에 비례해 커진다', () => {
    expect(seedOffset(0)).toBe(0);
    expect(seedOffset(1)).toBe(25);
    expect(seedOffset(3)).toBe(75);
  });

  it('여덟 칸에서 되감긴다 — 계단이 캔버스 밖으로 행진하지 않는다', () => {
    expect(seedOffset(8)).toBe(seedOffset(0));
    expect(seedOffset(9)).toBe(seedOffset(1));
  });

  it('계단은 정수다 — 좌표계가 정수이므로 씨앗도 정수여야 한다', () => {
    for (let n = 0; n < 16; n += 1) expect(Number.isInteger(seedOffset(n))).toBe(true);
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
    // 가로는 선이 이미 캔버스를 가로지르고 문구는 오른쪽으로 흐르므로 건드리지 않는다.
    expect(newElement('a', 'rect', 1).geometry).toEqual({ x: 75, y: 65, w: 100, h: 80 });
    expect(newElement('a', 'line', 1).geometry).toEqual({ x1: 50, y1: 225, x2: 450, y2: 225 });
    expect(newElement('a', 'text', 1).geometry).toEqual({ x: 250, y: 225 });
  });

  it('씨앗 좌표는 언제나 정수다 (정수 좌표계)', () => {
    for (const kind of KINDS) {
      for (let n = 0; n < 9; n += 1) {
        const geo = newElement('a', kind, n).geometry as unknown as Record<string, number>;
        for (const v of Object.values(geo)) expect(Number.isInteger(v)).toBe(true);
      }
    }
    expect((newElement('a', 'rect', 3).geometry as { x: number }).x).toBe(125);
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

  it('기존 요소의 좌표는 건드리지 않는다 — 캔버스 밖 저술은 합법이다', () => {
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
    expect(geos[0]).toBe(JSON.stringify({ x: 50, y: 40, w: 100, h: 80 }));
    expect(geos[1]).toBe(JSON.stringify({ x: 75, y: 65, w: 100, h: 80 }));
    expect(geos[2]).toBe(JSON.stringify({ x: 100, y: 90, w: 100, h: 80 }));
  });

  it('아홉 번을 붙여도 기본 캔버스 밖으로 행진하지 않는다', () => {
    let els: CanvasElement[] = [];
    for (let i = 0; i < 9; i++) els = appendElement(els, 'rect').next;
    for (const el of els) {
      const geo = el.geometry as unknown as Record<string, number>;
      expect(geo.x!).toBeGreaterThanOrEqual(0);
      expect(geo.y!).toBeGreaterThanOrEqual(0);
      expect(geo.x! + geo.w!).toBeLessThanOrEqual(DEFAULT_CANVAS_SIZE.width);
      expect(geo.y! + geo.h!).toBeLessThanOrEqual(DEFAULT_CANVAS_SIZE.height);
    }
    // 아홉 번째는 첫 번째 자리로 되감긴다(되감기 폭 8).
    expect(els[8]!.geometry).toEqual(els[0]!.geometry);
  });
});

// --- 문구 편집 -----------------------------------------------------------

describe('canvasElementFactory — withElementText', () => {
  it('도형에 문구가 생기면 글자색을 함께 심는다 — 없으면 렌더 층이 라벨을 건너뛴다', () => {
    for (const kind of SHAPE_KINDS) {
      const next = withElementText(shape(kind), '{value}');
      expect(next.text, `${kind} 의 문구가 실리지 않았다`).toBe('{value}');
      expect(next.style.textColor, `${kind} 에 글자색이 심기지 않았다`).toBe(SEED_TEXT_COLOR);
    }
  });

  it('심은 글자색은 도형 자신의 색이 아니다 — 같으면 라벨이 제 도형에 묻힌다', () => {
    // 렌더 층이 `fill` 로 폴백하지 않는 이유가 바로 이것이며(`drawElement` 주석), 저술
    // 시점에 폴백과 같은 값을 심으면 그 규율을 우회해 같은 결함을 되돌려 놓게 된다.
    expect(SEED_TEXT_COLOR).not.toBe(SEED_COLOR);
  });

  it('심은 글자색은 2D context 가 받을 수 있는 실제 색 문자열이다', () => {
    // CSS 변수(`var(--...)`)는 canvas 2D 가 해석하지 못한다 — `SEED_COLOR` 와 같은 규율이다.
    expect(SEED_TEXT_COLOR).toMatch(/^#[0-9a-f]{6}$/i);
  });

  it('심은 글자색은 도형 채움·밝은 표면·어두운 표면 어디서도 사라지지 않는다', () => {
    // 세 배경을 한 색으로 만족시킬 수 있는 상한이 1.998:1 이다(`SEED_TEXT_COLOR` 주석의
    // 계산). 흰색(밝은 표면에서 1.03:1)이나 검정에 가까운 색(어두운 표면에서 1.22:1)으로
    // 바꾸면 절반의 사용자에게 결함이 되돌아오므로, 그 선택을 이 단언이 막는다.
    for (const background of [SEED_COLOR, '#fbfcfe', '#1f2937']) {
      expect(contrastRatio(SEED_TEXT_COLOR, background), `${background} 위에서 묻힌다`)
        .toBeGreaterThan(1.9);
    }
  });

  it('사용자가 고른 글자색은 덮지 않는다', () => {
    const chosen: CanvasElement = { ...shape('rect'), style: { fill: SEED_COLOR, textColor: '#ff0000' } };
    expect(withElementText(chosen, '{value}').style.textColor).toBe('#ff0000');
  });

  it('문구를 지워도 그때 심긴 색은 남는다 — 다시 적을 때 색을 잃지 않는다', () => {
    const seeded = withElementText(shape('rect'), '{value}');
    const cleared = withElementText(seeded, undefined);
    expect('text' in cleared).toBe(false);
    expect(cleared.style.textColor).toBe(SEED_TEXT_COLOR);
  });

  it('빈 문구는 색을 심지 않는다 — 렌더 층이 빈 문구를 그리지 않는 것과 같은 판정이다', () => {
    expect(withElementText(shape('rect'), undefined).style.textColor).toBeUndefined();
    // 호출부가 `''` 를 접어 준다고 가정하지 않는다.
    expect(withElementText(shape('rect'), '').style.textColor).toBeUndefined();
  });

  it("kind:'text' 는 그대로다 — 문구 요소는 textColor ?? fill 로 칠해진다", () => {
    const text: CanvasElement = { id: 't1', kind: 'text', geometry: { x: 0.5, y: 0.5 }, style: {} };
    const next = withElementText(text, '{value}');
    expect(next.text).toBe('{value}');
    expect(next.style.textColor).toBeUndefined();
  });

  it('받은 요소를 제자리에서 고치지 않는다', () => {
    const before = shape('rect');
    withElementText(before, '{value}');
    expect(before.text).toBeUndefined();
    expect(before.style.textColor).toBeUndefined();
  });
});

/** sRGB 채널 하나를 선형 광량으로(WCAG 2.x 상대 명도 정의). */
function channelLuminance(byte: number): number {
  const c = byte / 255;
  return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
}

/** `#rrggbb` 의 상대 명도. */
function relativeLuminance(hex: string): number {
  const n = Number.parseInt(hex.slice(1), 16);
  return (
    0.2126 * channelLuminance((n >> 16) & 0xff) +
    0.7152 * channelLuminance((n >> 8) & 0xff) +
    0.0722 * channelLuminance(n & 0xff)
  );
}

/** 두 색의 WCAG 대비율. */
function contrastRatio(a: string, b: string): number {
  const la = relativeLuminance(a);
  const lb = relativeLuminance(b);
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05);
}
