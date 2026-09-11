// 그룹 히트 테스트 — 상자가 아니라 부품으로 잡는다 (SPEC-CANVAS-004 M4).
//
// **잡히는 성질은 히트로 잰다** — 그리기 기록으로 재면 엉뚱한 자로 재는 것이다(008 이
// 감김 가드를 히트 규칙으로 재려다 걸린 자리의 반대편이다).
//
// 이 파일의 하중은 **한 시험**이다: 그룹 상자 안이지만 어느 부품에도 닿지 않는 지점에서
// 그룹이 **잡히지 않고 뒤 요소가 잡힌다**(AC-E11). 그 한 시험만이 "상자로 잡기" 구현을
// 통과시키지 않는다 — 나머지 단언은 상자로 잡아도 전부 초록이다.
//
// 고정 입력은 시험 규율이 요구하는 형상이다:
//   - **비단위 축척** — 가로 0.8 · 세로 0.5. 축척 1 이면 나눗셈이 항등이라 결함이 안 보인다.
//   - **E-A** 부품 넷, 상자가 서로 다르고 겹치지 않으며, `stem` 은 가장자리에 닿지 않는다
//     (가장자리에 닿는 부품만 있으면 로컬 좌표가 전부 0 이나 EXTENT 라 축척이 관측되지 않는다).
//   - **E-C** 그룹 상자 `317 × 181` — 둘 다 10000 을 나누어떨어뜨리지 않고 비정사각이다.
//   - **E-D** 원점 `(73, 41)` — 0 이 아니고 두 축이 다르다. **음수 원점 그룹도 하나 둔다.**
//   - **E-I** 같은 부품 id 를 가진 그룹 둘.
//   - **E-L** 문구 부품 하나 — 그 히트 영역이 **폴백이 아니라 실측 폭**에서 나옴을 잰다.
//
// **탐침 지점은 리터럴 px 다.** 투영 함수로 탐침을 계산하면 그 함수를 망가뜨리는 뮤테이션이
// 탐침과 구현을 함께 옮겨 물지 않는다.
//
// @spec SPEC-CANVAS-004 REQ-08 · AC-E11

import { describe, expect, it } from 'vitest';

import type { CanvasElement, ElementStyle } from './canvasConfig';
import type { CanvasProjection, PxPoint } from './canvasGeometry';
import { hitTest, type CanvasHit } from './canvasHitTest';
import type { CanvasNode, GroupElement } from './group/groupTypes';

// --- 고정 입력 -------------------------------------------------------------

/**
 * 스테이지 400×200 · 캔버스 500×400 → 축척 **가로 0.8 · 세로 0.5**.
 *
 * 두 축이 다르고 둘 다 1 이 아니다. 축척 1 은 앞선 결함이 숨었던 바로 그 기본값이다.
 */
const PROJ: CanvasProjection = {
  stage: { width: 400, height: 200 },
  canvas: { width: 500, height: 400 },
};

/** 그룹 상자(캔버스 단위). px 로는 `x 58.4 · y 20.5 · w 253.6 · h 90.5` 다. */
const GROUP_GEO = { x: 73, y: 41, w: 317, h: 181 } as const;

const at = (x: number, y: number): PxPoint => ({ x, y });

function part(
  id: string,
  kind: 'rect' | 'ellipse',
  geometry: { x: number; y: number; w: number; h: number },
  style: ElementStyle = {},
): CanvasElement {
  return { id, kind, geometry, style };
}

/**
 * 부품 넷. 로컬 좌표 → px 는 `원점 + 로컬 ÷ 10000 × 상자변` 이다.
 *
 *   body  0,0,1500,1500      → px  58.40 .. 96.44  ×  20.500 ..  34.075
 *   stem  4100,3300,1700,900 → px 162.38 ..205.49  ×  50.365 ..  58.510  (가장자리에 안 닿는다)
 *   label 1000,9000          → px  83.76             , 101.950            (기준점)
 *   tip   8500,8500,1500,1500→ px 273.96 ..312.00  ×  97.425 .. 111.000
 */
function parts(): CanvasElement[] {
  return [
    part('body', 'rect', { x: 0, y: 0, w: 1500, h: 1500 }),
    part('stem', 'ellipse', { x: 4100, y: 3300, w: 1700, h: 900 }),
    {
      id: 'label',
      kind: 'text',
      geometry: { x: 1000, y: 9000 },
      style: {},
      text: 'abcdefghij',
    },
    part('tip', 'rect', { x: 8500, y: 8500, w: 1500, h: 1500 }),
  ];
}

function group(over: Partial<GroupElement> = {}): GroupElement {
  return { id: 'grp-1', kind: 'group', geometry: { ...GROUP_GEO }, parts: parts(), ...over };
}

/** 캔버스를 가득 덮는 사각형. 그룹보다 **앞 자리**(= 아래)에 둔다. */
function background(id = 'rect-bg'): CanvasElement {
  return { id, kind: 'rect', geometry: { x: 0, y: 0, w: 500, h: 400 }, style: {} };
}

const NO_WIDTHS: Readonly<Record<string, number>> = {};

function hit(
  nodes: CanvasNode[],
  point: PxPoint,
  widths: Readonly<Record<string, number>> = NO_WIDTHS,
): CanvasHit | undefined {
  return hitTest(nodes, point, PROJ, widths);
}

// --- ① 하중: 상자가 아니라 부품으로 잡힌다 (AC-E11) ------------------------

describe('그룹은 상자가 아니라 부품으로 잡힌다 (AC-E11)', () => {
  /**
   * 그룹 상자 안(`58.4..312 × 20.5..111`)이면서 네 부품 어디에도 — 집기 여유 6px 까지
   * 포함해 — 닿지 않는 지점이다.
   */
  const EMPTY_INSIDE = at(80, 85);

  it('상자 안이지만 부품 밖인 지점에서 **그룹이 아니라 뒤 요소**가 잡힌다', () => {
    // 배열 앞자리 = 아래. 역순 순회라 그룹이 **먼저** 판정되고, 맞지 않아야 배경이 잡힌다.
    expect(hit([background(), group()], EMPTY_INSIDE)).toEqual({ nodeId: 'rect-bg' });
  });

  it('배경이 없으면 그 지점에서는 **아무것도** 잡히지 않는다', () => {
    // 배경 없이 재는 것이 위 시험의 짝이다 — 배경이 이기는 것과 그룹이 지는 것은 다른 말이다.
    expect(hit([group()], EMPTY_INSIDE)).toBeUndefined();
  });

  it('그 지점이 정말로 **그룹 상자 안**이다 — 시험이 빈 주장이 아니다', () => {
    // 상자 전체를 덮는 부품 하나짜리 그룹으로 바꾸면 같은 지점이 잡힌다. 이 한 줄이
    // "애초에 상자 밖이라 안 잡힌 것" 을 배제한다.
    const filling = group({
      parts: [part('all', 'rect', { x: 0, y: 0, w: 10000, h: 10000 })],
    });
    expect(hit([background(), filling], EMPTY_INSIDE)).toEqual({
      nodeId: 'grp-1',
      partId: 'all',
    });
  });
});

// --- ② 부품 위에서는 잡힌다 — 원점과 축척을 둘 다 지난다 (E-A · E-D) --------

describe('부품 위에서 잡히면 nodeId 는 그룹, partId 는 그 부품이다', () => {
  it('가장자리에 닿는 부품(`body`)', () => {
    // px 중심 (77.42, 27.29).
    expect(hit([group()], at(77, 27))).toEqual({ nodeId: 'grp-1', partId: 'body' });
  });

  it('**안쪽에 떠 있는** 부품(`stem`) — 원점 더하기와 축척 곱하기를 둘 다 실행시킨다', () => {
    // px 중심 (183.93, 54.44). 원점을 잊으면 (125.5, 33.9) 로 가고, 축척을 잊으면
    // 화면 밖으로 나간다. 어느 쪽이든 이 탐침은 빗나간다.
    expect(hit([group()], at(184, 54))).toEqual({ nodeId: 'grp-1', partId: 'stem' });
  });

  it('두 축의 축척이 서로 다르다 — 축을 뒤바꾼 구현이 통과하지 않는다', () => {
    // `stem` 의 px 상자는 가로 43.1 · 세로 8.1 이다(로컬은 1700 × 900). 축척이 뒤바뀌면
    // 세로가 가로보다 커진다. 가로로 멀리(중심에서 18px) 떨어진 지점이 여전히 잡히고,
    // 세로로 같은 거리(18px) 떨어진 지점은 잡히지 않는다.
    expect(hit([group()], at(202, 54))).toEqual({ nodeId: 'grp-1', partId: 'stem' });
    expect(hit([group()], at(184, 72))).toBeUndefined();
  });

  it('오른쪽 아래 구석 부품(`tip`)', () => {
    expect(hit([group()], at(293, 104))).toEqual({ nodeId: 'grp-1', partId: 'tip' });
  });

  it('**음수 원점** 그룹에서도 원점이 더해진다 (E-D — 캔버스 밖 저술은 합법이다)', () => {
    // 상자 px: x = -40 × 0.8 = -32, y = -25 × 0.5 = -12.5, w 253.6, h 90.5.
    // `stem` px 중심 = (-32 + 103.98, -12.5 + 33.94) = (71.98, 21.44).
    const shifted = group({ geometry: { x: -40, y: -25, w: 317, h: 181 } });
    expect(hit([shifted], at(72, 21))).toEqual({ nodeId: 'grp-1', partId: 'stem' });
    // 원점을 더하지 않았다면 이 자리에서 잡혔을 것이다.
    expect(hit([shifted], at(184, 54))).toBeUndefined();
  });

  it('선택 키로 쓰이는 것은 **nodeId 하나**다 — 부품 id 가 선택을 대신하지 않는다', () => {
    const result = hit([group()], at(184, 54));
    expect(result?.nodeId).toBe('grp-1');
    expect(result?.partId).toBe('stem');
    // 최상위 원소는 `partId` 를 갖지 않는다(002 와 바이트 동일한 결과).
    expect(hit([background()], at(10, 10))).toEqual({ nodeId: 'rect-bg' });
    expect(hit([background()], at(10, 10))).not.toHaveProperty('partId');
  });
});

// --- ③ 순회 규칙 — 역순 · 보이지 않는 부품 (AC-E11) ------------------------

describe('부품 순회는 역순이고 보이지 않는 부품은 건너뛴다 (AC-E11)', () => {
  /** 정확히 겹치는 부품 둘. 배열 뒤가 위이므로 `over` 가 이겨야 한다. */
  function stacked(overStyle: ElementStyle = {}): GroupElement {
    const geo = { x: 2000, y: 2000, w: 4000, h: 4000 };
    return group({
      parts: [part('under', 'rect', geo), part('over', 'rect', geo, overStyle)],
    });
  }

  // px: x 58.4 + 0.2×253.6 = 109.12 .. 58.4 + 0.6×253.6 = 210.56
  //     y 20.5 + 0.2× 90.5 =  38.60 .. 20.5 + 0.6× 90.5 =  74.80
  const INSIDE_STACK = at(160, 56);

  it('뒤에 그려진 부품이 먼저 잡힌다', () => {
    expect(hit([stacked()], INSIDE_STACK)).toEqual({ nodeId: 'grp-1', partId: 'over' });
  });

  it('`visible === false` 인 부품은 건너뛰고 **그 아래 부품**이 잡힌다', () => {
    expect(hit([stacked({ visible: false })], INSIDE_STACK)).toEqual({
      nodeId: 'grp-1',
      partId: 'under',
    });
  });

  it('부품이 전부 보이지 않으면 그룹은 잡히지 않고 뒤 요소가 잡힌다', () => {
    const invisible = group({
      parts: parts().map((p) => ({ ...p, style: { visible: false } })),
    });
    expect(hit([background(), invisible], at(77, 27))).toEqual({ nodeId: 'rect-bg' });
  });

  it('빈 그룹은 잡히지 않되 순회를 끊지도 않는다 (AC-E1)', () => {
    expect(hit([background(), group({ parts: [] })], at(77, 27))).toEqual({ nodeId: 'rect-bg' });
  });

  it('최상위 역순도 그대로다 — 뒤 그룹이 앞 그룹을 이긴다', () => {
    const under = group({ id: 'grp-under' });
    const over = group({ id: 'grp-over' });
    expect(hit([under, over], at(77, 27))).toEqual({ nodeId: 'grp-over', partId: 'body' });
  });
});

// --- ④ 넷째 프레임 표면 — 문구 부품의 폭 (E-L) -----------------------------

describe('문구 부품의 히트 영역은 **실측 폭**에서 나온다 (E-L · AC-04)', () => {
  // `label` 기준점 px = (83.76, 101.95). 세로 기준은 `middle` 이라 상자는
  // y 101.95 ± 7 (기본 글자 크기 14) 이고, 가로는 기준점에서 실측 폭만큼 오른쪽이다.
  const FAR_RIGHT = at(190, 103);

  it('복합 키로 담긴 폭을 읽어 **기준점에서 멀리 떨어진** 지점도 잡는다', () => {
    expect(hit([group()], FAR_RIGHT, { 'grp-1/label': 120 })).toEqual({
      nodeId: 'grp-1',
      partId: 'label',
    });
  });

  it('평평한 키로 담긴 폭은 **읽히지 않는다** — 폴백으로 내려앉는다 (E-I · G11)', () => {
    // 이 한 줄이 조용한 저하를 잡는다: 폭을 못 찾으면 예외도 빈 화면도 아니고
    // "글자를 클릭하면 가끔 안 잡힌다" 로만 보고된다.
    expect(hit([group()], FAR_RIGHT, { label: 120 })).toBeUndefined();
  });

  it('폭이 없으면 기준점 둘레 여유 상자로 폴백한다 — 잡을 수 없게 되지는 않는다 (AC-E7)', () => {
    expect(hit([group()], FAR_RIGHT)).toBeUndefined();
    expect(hit([group()], at(84, 102))).toEqual({ nodeId: 'grp-1', partId: 'label' });
  });

  it('최상위 문구는 여전히 평평한 키로 읽는다 — 002 와 바이트 동일 (G11)', () => {
    const top: CanvasElement = {
      id: 'txt-1',
      kind: 'text',
      geometry: { x: 100, y: 200 },
      style: {},
      text: 'abcdefghij',
    };
    // px 기준점 = (80, 100). 폭 120 이면 x 80..200 이다.
    expect(hitTest([top], at(190, 100), PROJ, { 'txt-1': 120 })).toEqual({ nodeId: 'txt-1' });
    expect(hitTest([top], at(190, 100), PROJ, { 'top/txt-1': 120 })).toBeUndefined();
  });
});

// --- ④-b 선·경로 부품도 상자 안에서 판정된다 (REQ-03 · REQ-08) --------------

describe('선 부품과 경로 부품도 그룹 상자 안에서 잡힌다', () => {
  /**
   * 가로 선 하나. 로컬 `(500, 5000) → (3500, 5000)` 이므로
   * px 는 `(71.08, 65.75) → (147.16, 65.75)` 다.
   */
  const withLine = (): GroupElement =>
    group({
      parts: [
        {
          id: 'pipe',
          kind: 'line',
          geometry: { x1: 500, y1: 5000, x2: 3500, y2: 5000 },
          style: { strokeWidth: 2 },
        },
      ],
    });

  it('선 부품의 **두 끝점이 모두** 상자 안으로 옮겨진다', () => {
    // 중점에서 잡히고, 두 끝의 바로 바깥(여유 6px 밖)에서는 잡히지 않는다.
    expect(hit([withLine()], at(109, 66))).toEqual({ nodeId: 'grp-1', partId: 'pipe' });
    expect(hit([withLine()], at(63, 66))).toBeUndefined();
    expect(hit([withLine()], at(155, 66))).toBeUndefined();
    // 한 끝만 옮긴 구현은 이 셋 가운데 하나를 반드시 깬다.
    expect(hit([withLine()], at(72, 66))).toEqual({ nodeId: 'grp-1', partId: 'pipe' });
    expect(hit([withLine()], at(146, 66))).toEqual({ nodeId: 'grp-1', partId: 'pipe' });
  });

  /**
   * 제 상자를 가득 채우는 닫힌 네모 경로. 상자는 로컬 `(1000,1000,4000,4000)` 이므로
   * px 로 `83.76 .. 185.20 × 29.55 .. 65.75` 다. **두 겹의 로컬 격자가 합성된다.**
   */
  const withPath = (): GroupElement =>
    group({
      parts: [
        {
          id: 'glyph',
          kind: 'path',
          geometry: { x: 1000, y: 1000, w: 4000, h: 4000 },
          path: [
            { c: 'M', x: 0, y: 0 },
            { c: 'L', x: 10000, y: 0 },
            { c: 'L', x: 10000, y: 10000 },
            { c: 'L', x: 0, y: 10000 },
            { c: 'Z' },
          ],
          style: { fill: '#000' },
        },
      ],
    });

  it('경로 부품이 **두 겹의 로컬 격자**를 합성한 자리에서 잡힌다', () => {
    expect(hit([withPath()], at(134, 47))).toEqual({ nodeId: 'grp-1', partId: 'glyph' });
    // 제 상자 밖(그러나 **그룹 상자 안**)에서는 잡히지 않는다 — 경로도 상자로 잡지 않는다.
    expect(hit([withPath()], at(250, 100))).toBeUndefined();
  });
});

// --- ⑤ 그룹 둘의 교차 오염 (E-I) -------------------------------------------

describe('그룹 둘이 같은 부품 id 를 가져도 섞이지 않는다 (E-I)', () => {
  it('각 그룹이 **제 상자**로 판정되고 제 id 를 돌려준다', () => {
    const a = group({ id: 'grp-A' });
    const b = group({ id: 'grp-B', geometry: { x: 300, y: 250, w: 133, h: 97 } });
    // A 의 `body` px 중심 (77.42, 27.29).
    expect(hit([a, b], at(77, 27))).toEqual({ nodeId: 'grp-A', partId: 'body' });
    // B 의 `body` px: x 240 .. 240 + 0.15×106.4 = 255.96, y 125 .. 125 + 0.15×48.5 = 132.275.
    expect(hit([a, b], at(248, 128))).toEqual({ nodeId: 'grp-B', partId: 'body' });
  });

  it('한 그룹의 폭 장부가 다른 그룹의 문구에 쓰이지 않는다', () => {
    const a = group({ id: 'grp-A' });
    const b = group({ id: 'grp-B' });
    // 두 그룹의 상자가 같으므로 `label` 의 px 자리도 같다. 폭은 A 에만 담는다.
    // 역순이라 B 가 먼저 판정되고, B 의 폭이 없으므로 B 의 문구는 폴백이라 빗나간다.
    // 그 뒤 A 가 실측 폭으로 잡는다.
    expect(hit([a, b], at(190, 103), { 'grp-A/label': 120 })).toEqual({
      nodeId: 'grp-A',
      partId: 'label',
    });
  });
});

// --- ⑥ 견고성 (REQ-05) -----------------------------------------------------

describe('그룹 히트가 던지지 않는다 (REQ-05)', () => {
  it('퇴화 상자(두 변 0)에서도 던지지 않는다', () => {
    const flat = group({ geometry: { x: 100, y: 100, w: 0, h: 0 } });
    expect(() => hit([flat], at(80, 50))).not.toThrow();
  });

  it('비유한 상자에서도 던지지 않는다', () => {
    const broken = group({
      geometry: { x: Number.NaN, y: 0, w: Number.POSITIVE_INFINITY, h: 181 },
    });
    expect(() => hit([broken], at(80, 50))).not.toThrow();
  });

  it('비유한 포인터는 들어오는 자리에서 끊긴다 — 그룹이 있어도 같다', () => {
    expect(hit([group()], at(Number.NaN, 27))).toBeUndefined();
    expect(hit([group()], at(77, Number.POSITIVE_INFINITY))).toBeUndefined();
  });

  it('입력 배열을 뒤집지 않는다 — 호출부의 z-order 가 조용히 바뀌지 않는다', () => {
    const nodes: CanvasNode[] = [background(), group()];
    const before = nodes.map((n) => n.id);
    const partsBefore = group().parts.map((p) => p.id);
    hit(nodes, at(77, 27));
    expect(nodes.map((n) => n.id)).toEqual(before);
    expect((nodes[1] as GroupElement).parts.map((p) => p.id)).toEqual(partsBefore);
  });
});
