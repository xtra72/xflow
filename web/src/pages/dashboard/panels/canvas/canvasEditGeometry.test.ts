// 캔버스 편집 수학 단위 테스트 (SPEC-CANVAS-002 T7).
//
// 고정하는 계약 여섯:
//   1) 종류별 핸들 집합 — 특히 `text` 는 글자 크기 핸들 **하나뿐이며 박스 핸들이 없다**.
//   2) 핸들 px 좌표가 `canvasGeometry` 의 투영과 **정확히 일치한다**(위험 R1 방어).
//      그래서 기대값을 손으로 쓴 숫자가 아니라 투영 함수의 결과로 쓴다 — 손으로 쓰면
//      투영이 바뀌어도 테스트가 통과해 두 벌이 갈라진 것을 잡지 못한다.
//   3) 크기 조절 결과는 **언제나 양수 크기**다. 반대 변을 넘어 뒤집어도 그렇다(위험 R7).
//   4) 이동은 **clamp 하지 않는다** — 캔버스 밖으로 나가도 좌표가 잘리지 않는다(가정 A5).
//   5) `patchNodeGeometryZ` 가 아니라 `patchNodeGeometry` **하나**가 기하 쓰기 통로이며,
//      식별은 `nodeId` 로만 한다(REQ-06). 그 통로가 **정수화와 퇴화 방지를 함께 소유한다** —
//      조작 함수들은 실수를 그대로 돌려주고 쓰기 직전 한 곳에서만 정수가 된다.
//   6) 비유한 입력이 NaN 기하를 만들지 않는다.
// DOM 을 쓰지 않으므로 jsdom 없이도 돈다.

import { describe, it, expect } from 'vitest';

import {
  BOX_CORNER_HANDLE_IDS,
  BOX_HANDLE_IDS,
  CANVAS_FONT_SIZE_MAX,
  CANVAS_FONT_SIZE_MIN,
  LINE_HANDLE_IDS,
  TEXT_HANDLE_IDS,
  clampCanvasFontSize,
  handlePositions,
  handleRole,
  handlesFor,
  isCornerHandle,
  moveGeometry,
  patchNodeGeometry,
  resizeBox,
  resizeFontSize,
  resizeLine,
  type BoxHandleId,
} from './canvasEditGeometry';
import {
  projectBox,
  projectLine,
  projectPoint,
  resolveTextOrigin,
  type CanvasProjection,
  type StageSize,
} from './canvasGeometry';
import {
  DEFAULT_FONT_SIZE,
  MIN_ELEMENT_EXTENT,
  type BoxGeometry,
  type CanvasElement,
  type CanvasSize,
  type ElementStyle,
  type LineGeometry,
  type PointGeometry,
} from './canvasConfig';

/** 대표 스테이지(800x600) — 001 기하 테스트와 같은 크기다. */
const STAGE: StageSize = { width: 800, height: 600 };

/**
 * 대표 캔버스(1000x1000) — 축척이 **가로 0.8 · 세로 0.6** 으로 갈린다.
 *
 * 두 축척이 다른 것이 요점이다: 같으면 축을 뒤바꾼 투영이 시험을 통과한다.
 */
const CANVAS: CanvasSize = { width: 1000, height: 1000 };

/** 대표 투영 한 벌. */
const PROJ: CanvasProjection = { stage: STAGE, canvas: CANVAS };

/** 기준 박스 — 네 변이 200/600 이라 뒤집힘을 눈으로 따라가기 쉽다. */
const BASE_BOX: BoxGeometry = { x: 200, y: 200, w: 400, h: 400 };

/** 종횡비 2:1 기준 박스 — Shift 유지 여부가 결과로 드러나는 크기다. */
const RATIO_BOX: BoxGeometry = { x: 200, y: 200, w: 400, h: 200 };

/** 기준 선 — 대각선이라 각도 죔의 결과가 축과 구분된다. */
const BASE_LINE: LineGeometry = { x1: 200, y1: 200, x2: 600, y2: 600 };

function boxEl(id: string, geometry: BoxGeometry, kind: 'rect' | 'ellipse' = 'rect'): CanvasElement {
  return { id, kind, geometry, style: {} } as CanvasElement;
}

function lineEl(id: string, geometry: LineGeometry): CanvasElement {
  return { id, kind: 'line', geometry, style: {} };
}

function textEl(id: string, geometry: PointGeometry, style: ElementStyle = {}): CanvasElement {
  return { id, kind: 'text', geometry, style, text: '온도' };
}

/**
 * 부동소수 오차를 흡수하는 박스 비교.
 *
 * 좌표는 정수지만 이 모듈의 조작 함수들은 **반올림하지 않는다** — 정수화는 쓰기
 * 통로(`patchNodeGeometry`)의 몫이다. 그래서 종횡비 유지처럼 나눗셈이 끼는 계산은
 * 여기서 소수를 낸다.
 */
function expectBox(actual: BoxGeometry, expected: BoxGeometry): void {
  expect(actual.x).toBeCloseTo(expected.x, 10);
  expect(actual.y).toBeCloseTo(expected.y, 10);
  expect(actual.w).toBeCloseTo(expected.w, 10);
  expect(actual.h).toBeCloseTo(expected.h, 10);
}

/** 같은 규율의 선 비교. */
function expectLine(actual: LineGeometry, expected: LineGeometry): void {
  expect(actual.x1).toBeCloseTo(expected.x1, 10);
  expect(actual.y1).toBeCloseTo(expected.y1, 10);
  expect(actual.x2).toBeCloseTo(expected.x2, 10);
  expect(actual.y2).toBeCloseTo(expected.y2, 10);
}

// --- 핸들 집합 ----------------------------------------------------------

describe('handlesFor', () => {
  it('rect 는 모서리 4 + 변 4 = 8개 핸들을 갖는다', () => {
    expect(handlesFor('rect')).toEqual(['nw', 'n', 'ne', 'e', 'se', 's', 'sw', 'w']);
    expect(handlesFor('rect')).toHaveLength(8);
  });

  it('ellipse 는 rect 와 같은 8개 핸들을 갖는다', () => {
    expect(handlesFor('ellipse')).toEqual(handlesFor('rect'));
  });

  it('line 은 끝점 2개뿐이다', () => {
    expect(handlesFor('line')).toEqual(['p1', 'p2']);
  });

  it('text 는 글자 크기 핸들 하나뿐이며 박스 핸들이 없다', () => {
    const ids = handlesFor('text');
    expect(ids).toEqual(['font']);
    expect(ids).toHaveLength(1);
    for (const boxId of BOX_HANDLE_IDS) expect(ids).not.toContain(boxId);
  });

  it('상수 집합과 반환값이 같은 것을 가리킨다(어휘가 둘이 되지 않는다)', () => {
    expect(handlesFor('rect')).toBe(BOX_HANDLE_IDS);
    expect(handlesFor('line')).toBe(LINE_HANDLE_IDS);
    expect(handlesFor('text')).toBe(TEXT_HANDLE_IDS);
  });
});

describe('handleRole', () => {
  it('글자 크기 핸들만 style.fontSize 를 쓴다', () => {
    expect(handleRole('font')).toBe('fontSize');
  });

  it('나머지 핸들은 모두 기하를 쓴다', () => {
    for (const id of [...BOX_HANDLE_IDS, ...LINE_HANDLE_IDS]) {
      expect(handleRole(id)).toBe('geometry');
    }
  });
});

describe('isCornerHandle', () => {
  it('모서리 넷만 참이다(종횡비 유지가 뜻을 갖는 자리)', () => {
    for (const id of BOX_CORNER_HANDLE_IDS) expect(isCornerHandle(id)).toBe(true);
  });

  it('변 핸들·선 끝점·글자 크기 핸들은 거짓이다', () => {
    for (const id of ['n', 'e', 's', 'w', 'p1', 'p2', 'font'] as const) {
      expect(isCornerHandle(id)).toBe(false);
    }
  });
});

// --- 핸들 좌표: 투영 단일 출처(위험 R1) ---------------------------------

describe('handlePositions — 투영과의 일치(위험 R1)', () => {
  it('rect 8개 핸들이 projectBox 결과의 변·모서리와 정확히 일치한다', () => {
    const geo: BoxGeometry = { x: 100, y: 200, w: 500, h: 250 };
    const pb = projectBox(geo, PROJ);
    const handles = handlePositions(boxEl('a', geo), PROJ);

    expect(handles.map((h) => h.id)).toEqual([...BOX_HANDLE_IDS]);
    const at = (id: BoxHandleId) => handles.find((h) => h.id === id)!.point;

    expect(at('nw')).toEqual({ x: pb.x, y: pb.y });
    expect(at('n')).toEqual({ x: pb.x + pb.w * 0.5, y: pb.y });
    expect(at('ne')).toEqual({ x: pb.x + pb.w, y: pb.y });
    expect(at('e')).toEqual({ x: pb.x + pb.w, y: pb.y + pb.h * 0.5 });
    expect(at('se')).toEqual({ x: pb.x + pb.w, y: pb.y + pb.h });
    expect(at('s')).toEqual({ x: pb.x + pb.w * 0.5, y: pb.y + pb.h });
    expect(at('sw')).toEqual({ x: pb.x, y: pb.y + pb.h });
    expect(at('w')).toEqual({ x: pb.x, y: pb.y + pb.h * 0.5 });
  });

  it('ellipse 도 같은 박스 핸들을 같은 자리에 낸다', () => {
    const geo: BoxGeometry = { x: 100, y: 200, w: 500, h: 250 };
    expect(handlePositions(boxEl('a', geo, 'ellipse'), PROJ)).toEqual(
      handlePositions(boxEl('a', geo), PROJ),
    );
  });

  it('스테이지가 커지면 핸들도 함께 옮겨 간다(측정원이 하나다)', () => {
    const geo: BoxGeometry = { x: 100, y: 200, w: 500, h: 250 };
    const bigger: CanvasProjection = { stage: { width: 1600, height: 1200 }, canvas: CANVAS };
    const pb = projectBox(geo, bigger);
    const handles = handlePositions(boxEl('a', geo), bigger);
    expect(handles[0]!.point).toEqual({ x: pb.x, y: pb.y });
    expect(handles[4]!.point).toEqual({ x: pb.x + pb.w, y: pb.y + pb.h });
  });

  it('음수 크기 박스의 핸들은 화면에 보이는 양수 상자 위에 놓인다', () => {
    const geo: BoxGeometry = { x: 600, y: 600, w: -200, h: -100 };
    const handles = handlePositions(boxEl('a', geo), PROJ);
    expect(handles.find((h) => h.id === 'nw')!.point).toEqual({ x: 320, y: 300 });
    expect(handles.find((h) => h.id === 'se')!.point).toEqual({ x: 480, y: 360 });
  });

  it('line 두 핸들이 projectLine 의 끝점과 정확히 일치한다', () => {
    const geo: LineGeometry = { x1: 100, y1: 200, x2: 900, y2: 800 };
    const pl = projectLine(geo, PROJ);
    const handles = handlePositions(lineEl('l', geo), PROJ);

    expect(handles.map((h) => h.id)).toEqual(['p1', 'p2']);
    expect(handles[0]!.point).toEqual({ x: pl.x1, y: pl.y1 });
    expect(handles[1]!.point).toEqual({ x: pl.x2, y: pl.y2 });
    expect(handles.every((h) => h.role === 'geometry')).toBe(true);
  });

  it('text 핸들은 하나이며 role 이 fontSize 다(기하를 쓰지 않는다)', () => {
    const geo: PointGeometry = { x: 500, y: 500 };
    const handles = handlePositions(textEl('t', geo, { fontSize: 20 }), PROJ, {
      measuredWidth: 60,
    });
    expect(handles).toHaveLength(1);
    expect(handles[0]!.id).toBe('font');
    expect(handles[0]!.role).toBe('fontSize');
  });

  it('text 핸들 자리는 resolveTextOrigin 이 낸 원점 + 실측 폭 + fontSize/2 다', () => {
    const geo: PointGeometry = { x: 500, y: 500 };
    const width = 60;
    const origin = resolveTextOrigin(projectPoint(geo, PROJ), 'left', width);
    const handles = handlePositions(textEl('t', geo, { fontSize: 20 }), PROJ, {
      measuredWidth: width,
    });
    expect(handles[0]!.point).toEqual({ x: origin.x + width, y: origin.y + 10 });
  });

  it('text 정렬이 center 면 원점이 왼쪽으로 밀리고 핸들도 함께 밀린다', () => {
    const geo: PointGeometry = { x: 500, y: 500 };
    const width = 60;
    const origin = resolveTextOrigin(projectPoint(geo, PROJ), 'center', width);
    const handles = handlePositions(textEl('t', geo, { fontSize: 20, align: 'center' }), PROJ, {
      measuredWidth: width,
    });
    expect(handles[0]!.point).toEqual({ x: origin.x + width, y: origin.y + 10 });
    expect(handles[0]!.point.x).toBe(430);
  });

  it('fontSize 미지정이면 기본 크기(14)로 자리를 잡는다', () => {
    const handles = handlePositions(textEl('t', { x: 500, y: 500 }), PROJ, { measuredWidth: 0 });
    expect(handles[0]!.point).toEqual({ x: 400, y: 300 + DEFAULT_FONT_SIZE / 2 });
  });

  it('실측 폭이 아직 없으면(옵션 자체 생략) 기준점에 붙는다', () => {
    const handles = handlePositions(textEl('t', { x: 500, y: 500 }, { fontSize: 20 }), PROJ);
    expect(handles[0]!.point).toEqual({ x: 400, y: 310 });
  });

  it('손상된 실측 폭(NaN·음수)은 0 으로 본다', () => {
    const base = handlePositions(textEl('t', { x: 500, y: 500 }, { fontSize: 20 }), PROJ, {
      measuredWidth: Number.NaN,
    });
    expect(base[0]!.point).toEqual({ x: 400, y: 310 });
    const negative = handlePositions(textEl('t', { x: 500, y: 500 }, { fontSize: 20 }), PROJ, {
      measuredWidth: -5,
    });
    expect(negative[0]!.point).toEqual({ x: 400, y: 310 });
  });

  it('범위 밖 fontSize 는 핸들 자리 계산에서 범위로 죄인다', () => {
    const huge = handlePositions(textEl('t', { x: 500, y: 500 }, { fontSize: 500 }), PROJ);
    expect(huge[0]!.point.y).toBe(300 + CANVAS_FONT_SIZE_MAX / 2);
    const zero = handlePositions(textEl('t', { x: 500, y: 500 }, { fontSize: 0 }), PROJ);
    expect(zero[0]!.point.y).toBe(300 + CANVAS_FONT_SIZE_MIN / 2);
  });

  it('크기가 아직 잡히지 않은 스테이지에서도 NaN 이 아니라 0 을 낸다', () => {
    const handles = handlePositions(boxEl('a', BASE_BOX), {
      stage: { width: 0, height: 0 },
      canvas: CANVAS,
    });
    for (const h of handles) {
      expect(Number.isFinite(h.point.x)).toBe(true);
      expect(Number.isFinite(h.point.y)).toBe(true);
    }
  });
});

// --- 이동 ---------------------------------------------------------------

describe('moveGeometry', () => {
  it('박스는 좌상단만 옮기고 크기를 유지한다', () => {
    const out = moveGeometry(BASE_BOX, { dx: 50, dy: -50 });
    expect(out).toEqual({ x: 200 + 50, y: 200 - 50, w: 400, h: 400 });
  });

  it('선은 두 끝점이 함께 움직인다(몸통 드래그)', () => {
    const out = moveGeometry(BASE_LINE, { dx: 100, dy: 200 });
    expect(out).toEqual({
      x1: 200 + 100,
      y1: 200 + 200,
      x2: 600 + 100,
      y2: 600 + 200,
    });
  });

  it('문구는 기준점을 옮긴다', () => {
    expect(moveGeometry({ x: 500, y: 500 }, { dx: -250, dy: 250 })).toEqual({
      x: 500 - 250,
      y: 500 + 250,
    });
  });

  it('캔버스 밖으로 나가도 clamp 하지 않는다(가정 A5)', () => {
    const right = moveGeometry({ x: 900, y: 900, w: 200, h: 200 }, { dx: 500, dy: 500 });
    expect(right.x).toBe(1400);
    expect(right.y).toBe(1400);
    expect(right.x).toBeGreaterThan(CANVAS.width);

    const left = moveGeometry({ x: 100, y: 100, w: 200, h: 200 }, { dx: -2000, dy: -3000 });
    expect(left.x).toBe(-1900);
    expect(left.y).toBe(-2900);
    expect(left.x).toBeLessThan(0);
  });

  it('선·문구도 마찬가지로 잘리지 않는다', () => {
    expect(moveGeometry(BASE_LINE, { dx: 3000, dy: 3000 }).x2).toBe(3600);
    expect(moveGeometry({ x: 500, y: 500 }, { dx: -4000, dy: 4000 }).x).toBe(-3500);
  });

  it('비유한 델타는 그 축의 이동을 없던 일로 본다(NaN 기하를 만들지 않는다)', () => {
    expect(moveGeometry(BASE_BOX, { dx: Number.NaN, dy: 100 })).toEqual({
      x: 200,
      y: 200 + 100,
      w: 400,
      h: 400,
    });
    expect(moveGeometry(BASE_BOX, { dx: 100, dy: Number.POSITIVE_INFINITY })).toEqual({
      x: 200 + 100,
      y: 200,
      w: 400,
      h: 400,
    });
  });

  it('손상된 기하 필드는 0 으로 떨어뜨린 뒤 옮긴다', () => {
    expect(moveGeometry({ x: Number.NaN, y: 200, w: 400, h: Number.NaN }, { dx: 100, dy: 0 })).toEqual(
      { x: 100, y: 200, w: 400, h: 0 },
    );
    expect(
      moveGeometry({ x1: Number.NaN, y1: 200, x2: 600, y2: Number.NaN }, { dx: 0, dy: 0 }),
    ).toEqual({ x1: 0, y1: 200, x2: 600, y2: 0 });
    expect(moveGeometry({ x: Number.NaN, y: Number.NaN }, { dx: 0, dy: 0 })).toEqual({
      x: 0,
      y: 0,
    });
  });

  it('입력을 건드리지 않고 새 객체를 낸다', () => {
    const input: BoxGeometry = { ...BASE_BOX };
    const out = moveGeometry(input, { dx: 100, dy: 100 });
    expect(input).toEqual(BASE_BOX);
    expect(out).not.toBe(input);
  });
});

// --- 크기 조절: 박스 ----------------------------------------------------

describe('resizeBox — 8개 핸들', () => {
  const inside = { x: 500, y: 500 };

  it('nw 는 좌·상 변을 함께 옮긴다', () => {
    expectBox(resizeBox(BASE_BOX, 'nw', inside), { x: 500, y: 500, w: 100, h: 100 });
  });

  it('n 은 위쪽 변만 옮긴다', () => {
    expectBox(resizeBox(BASE_BOX, 'n', inside), { x: 200, y: 500, w: 400, h: 100 });
  });

  it('ne 는 우·상 변을 함께 옮긴다', () => {
    expectBox(resizeBox(BASE_BOX, 'ne', inside), { x: 200, y: 500, w: 300, h: 100 });
  });

  it('e 는 오른쪽 변만 옮긴다', () => {
    expectBox(resizeBox(BASE_BOX, 'e', inside), { x: 200, y: 200, w: 300, h: 400 });
  });

  it('se 는 우·하 변을 함께 옮긴다', () => {
    expectBox(resizeBox(BASE_BOX, 'se', inside), { x: 200, y: 200, w: 300, h: 300 });
  });

  it('s 는 아래쪽 변만 옮긴다', () => {
    expectBox(resizeBox(BASE_BOX, 's', inside), { x: 200, y: 200, w: 400, h: 300 });
  });

  it('sw 는 좌·하 변을 함께 옮긴다', () => {
    expectBox(resizeBox(BASE_BOX, 'sw', inside), { x: 500, y: 200, w: 100, h: 300 });
  });

  it('w 는 왼쪽 변만 옮긴다', () => {
    expectBox(resizeBox(BASE_BOX, 'w', inside), { x: 500, y: 200, w: 100, h: 400 });
  });

  it('스테이지 밖으로 끌어도 clamp 하지 않는다', () => {
    const out = resizeBox(BASE_BOX, 'se', { x: 1800, y: 2400 });
    expectBox(out, { x: 200, y: 200, w: 1600, h: 2200 });
    expect(out.w).toBeGreaterThan(CANVAS.width);
  });
});

describe('resizeBox — 뒤집힘 정규화(위험 R7)', () => {
  it('se 를 좌상단 너머로 끌면 박스가 양수로 정규화된다', () => {
    const out = resizeBox(BASE_BOX, 'se', { x: 50, y: 0 });
    expectBox(out, { x: 50, y: 0, w: 150, h: 200 });
    expect(out.w).toBeGreaterThan(0);
    expect(out.h).toBeGreaterThan(0);
  });

  it('nw 를 우하단 너머로 끌어도 마찬가지다', () => {
    const out = resizeBox(BASE_BOX, 'nw', { x: 900, y: 950 });
    expectBox(out, { x: 600, y: 600, w: 300, h: 350 });
  });

  it('한 축만 뒤집혀도(변 핸들) 음수 크기가 나오지 않는다', () => {
    expectBox(resizeBox(BASE_BOX, 'e', { x: 50, y: 500 }), {
      x: 50,
      y: 200,
      w: 150,
      h: 400,
    });
    expectBox(resizeBox(BASE_BOX, 'n', { x: 500, y: 950 }), {
      x: 200,
      y: 600,
      w: 400,
      h: 350,
    });
  });

  it('8개 핸들 어디를 반대 끝 너머로 끌어도 음수 크기가 저장되지 않는다', () => {
    for (const handle of BOX_HANDLE_IDS) {
      for (const pointer of [
        { x: -900, y: -900 },
        { x: 1900, y: 1900 },
      ]) {
        const out = resizeBox(BASE_BOX, handle, pointer);
        expect(out.w).toBeGreaterThanOrEqual(0);
        expect(out.h).toBeGreaterThanOrEqual(0);
      }
    }
  });

  it('음수 크기 박스를 받아도 결과는 정규화된 양수 박스다(읽기 관용 · 쓰기 엄격)', () => {
    const flipped: BoxGeometry = { x: 600, y: 600, w: -400, h: -400 };
    expectBox(resizeBox(flipped, 'se', { x: 500, y: 500 }), {
      x: 200,
      y: 200,
      w: 300,
      h: 300,
    });
  });
});

describe('resizeBox — 종횡비 유지(Shift)', () => {
  it('모서리에서 Shift 를 누르면 원래 비(2:1)를 지킨다 — 가로가 이기는 경우', () => {
    const out = resizeBox(RATIO_BOX, 'se', { x: 800, y: 500 }, { preserveAspect: true });
    expectBox(out, { x: 200, y: 200, w: 600, h: 300 });
    expect(out.w / out.h).toBeCloseTo(2, 10);
  });

  it('세로가 더 멀리 가면 세로가 비를 정한다', () => {
    const out = resizeBox(RATIO_BOX, 'se', { x: 400, y: 900 }, { preserveAspect: true });
    expectBox(out, { x: 200, y: 200, w: 1400, h: 700 });
    expect(out.w / out.h).toBeCloseTo(2, 10);
  });

  it('반대 모서리(nw)를 잡아 음의 방향으로 끌어도 비가 지켜진다', () => {
    const out = resizeBox(RATIO_BOX, 'nw', { x: 200, y: 300 }, { preserveAspect: true });
    expectBox(out, { x: 200, y: 200, w: 400, h: 200 });
    expect(out.w / out.h).toBeCloseTo(2, 10);
  });

  it('Shift 를 누르지 않으면 같은 드래그가 비를 깬다', () => {
    const free = resizeBox(RATIO_BOX, 'se', { x: 400, y: 900 });
    expectBox(free, { x: 200, y: 200, w: 200, h: 700 });
    expect(free.w / free.h).not.toBeCloseTo(2, 3);
  });

  it('변 핸들에서는 Shift 가 아무 일도 하지 않는다(유지할 모서리가 없다)', () => {
    const shifted = resizeBox(RATIO_BOX, 'e', { x: 800, y: 900 }, { preserveAspect: true });
    const free = resizeBox(RATIO_BOX, 'e', { x: 800, y: 900 });
    expect(shifted).toEqual(free);
    expectBox(shifted, { x: 200, y: 200, w: 600, h: 200 });
  });

  it('퇴화 박스(폭 0 또는 높이 0)에는 유지할 비가 없어 자유 조절이 된다', () => {
    const zeroW = resizeBox(
      { x: 200, y: 200, w: 0, h: 200 },
      'se',
      { x: 600, y: 600 },
      { preserveAspect: true },
    );
    expectBox(zeroW, { x: 200, y: 200, w: 400, h: 400 });

    const zeroH = resizeBox(
      { x: 200, y: 200, w: 400, h: 0 },
      'se',
      { x: 600, y: 600 },
      { preserveAspect: true },
    );
    expectBox(zeroH, { x: 200, y: 200, w: 400, h: 400 });
  });
});

describe('resizeBox — 견고성', () => {
  it('비유한 포인터는 조작을 무시하고 정규화된 원본을 돌려준다', () => {
    expectBox(resizeBox(BASE_BOX, 'se', { x: Number.NaN, y: 500 }), BASE_BOX);
    expectBox(resizeBox(BASE_BOX, 'se', { x: 500, y: Number.POSITIVE_INFINITY }), BASE_BOX);
  });

  it('비유한 포인터라도 뒤집힌 입력 박스는 정규화해서 돌려준다', () => {
    expectBox(resizeBox({ x: 600, y: 600, w: -400, h: -400 }, 'nw', { x: Number.NaN, y: 0 }), {
      x: 200,
      y: 200,
      w: 400,
      h: 400,
    });
  });

  it('손상된 박스 필드는 0 으로 떨어뜨린 뒤 조절한다', () => {
    expectBox(resizeBox({ x: Number.NaN, y: 200, w: 400, h: 400 }, 'e', { x: 500, y: 500 }), {
      x: 0,
      y: 200,
      w: 500,
      h: 400,
    });
  });

  it('입력을 건드리지 않고 새 객체를 낸다', () => {
    const input: BoxGeometry = { ...BASE_BOX };
    const out = resizeBox(input, 'se', { x: 500, y: 500 });
    expect(input).toEqual(BASE_BOX);
    expect(out).not.toBe(input);
  });
});

// --- 크기 조절: 선 ------------------------------------------------------

describe('resizeLine', () => {
  it('p1 핸들은 첫 끝점만 옮긴다', () => {
    expectLine(resizeLine(BASE_LINE, 'p1', { x: 100, y: 900 }), {
      x1: 100,
      y1: 900,
      x2: 600,
      y2: 600,
    });
  });

  it('p2 핸들은 둘째 끝점만 옮긴다', () => {
    expectLine(resizeLine(BASE_LINE, 'p2', { x: 100, y: 900 }), {
      x1: 200,
      y1: 200,
      x2: 100,
      y2: 900,
    });
  });

  it('끝점이 스테이지 밖으로 나가도 clamp 하지 않는다', () => {
    expectLine(resizeLine(BASE_LINE, 'p2', { x: 1800, y: -400 }), {
      x1: 200,
      y1: 200,
      x2: 1800,
      y2: -400,
    });
  });

  it('Shift 는 거의 수평인 드래그를 0° 로 죈다', () => {
    const out = resizeLine(BASE_LINE, 'p2', { x: 700, y: 220 }, { constrainAngle: true });
    expect(out.y2).toBe(200);
    expect(out.x2).toBeCloseTo(200 + Math.hypot(500, 20), 10);
  });

  it('Shift 는 거의 수직인 드래그를 90° 로 죈다', () => {
    const out = resizeLine(BASE_LINE, 'p2', { x: 220, y: 700 }, { constrainAngle: true });
    expect(out.x2).toBeCloseTo(200, 10);
    expect(out.y2).toBeCloseTo(200 + Math.hypot(20, 500), 10);
  });

  it('Shift 는 대각선 드래그를 45° 로 죄어 두 축 변위를 같게 만든다', () => {
    const out = resizeLine(BASE_LINE, 'p2', { x: 700, y: 680 }, { constrainAngle: true });
    expect(out.x2 - 200).toBeCloseTo(out.y2 - 200, 10);
    expect(Math.hypot(out.x2 - 200, out.y2 - 200)).toBeCloseTo(Math.hypot(500, 480), 10);
  });

  it('Shift 죔의 기준은 잡지 않은 반대 끝점이다(p1 을 끌면 p2 가 앵커)', () => {
    const out = resizeLine(BASE_LINE, 'p1', { x: 100, y: 580 }, { constrainAngle: true });
    expect(out.y1).toBeCloseTo(600, 10);
    expect(out.x1).toBeCloseTo(600 - Math.hypot(500, 20), 10);
    expect(out.x2).toBe(600);
    expect(out.y2).toBe(600);
  });

  it('앵커와 같은 지점으로 끌면(길이 0) 그 자리에 남는다', () => {
    const out = resizeLine(BASE_LINE, 'p2', { x: 200, y: 200 }, { constrainAngle: true });
    expectLine(out, { x1: 200, y1: 200, x2: 200, y2: 200 });
  });

  it('Shift 없이는 죄지 않는다', () => {
    expectLine(resizeLine(BASE_LINE, 'p2', { x: 700, y: 220 }), {
      x1: 200,
      y1: 200,
      x2: 700,
      y2: 220,
    });
  });

  it('비유한 포인터는 조작을 무시한다', () => {
    expectLine(resizeLine(BASE_LINE, 'p1', { x: Number.NaN, y: 500 }), BASE_LINE);
    expectLine(resizeLine(BASE_LINE, 'p2', { x: 500, y: Number.NEGATIVE_INFINITY }), BASE_LINE);
  });

  it('손상된 선 필드는 0 으로 떨어뜨린 뒤 조절한다', () => {
    expectLine(
      resizeLine({ x1: 200, y1: 200, x2: Number.NaN, y2: 600 }, 'p1', { x: 400, y: 400 }),
      { x1: 400, y1: 400, x2: 0, y2: 600 },
    );
  });

  it('입력을 건드리지 않고 새 객체를 낸다', () => {
    const input: LineGeometry = { ...BASE_LINE };
    const out = resizeLine(input, 'p1', { x: 400, y: 400 });
    expect(input).toEqual(BASE_LINE);
    expect(out).not.toBe(input);
  });
});

// --- 글자 크기 ----------------------------------------------------------

describe('clampCanvasFontSize', () => {
  it('범위 안의 값은 그대로 둔다', () => {
    expect(clampCanvasFontSize(20)).toBe(20);
    expect(clampCanvasFontSize(CANVAS_FONT_SIZE_MIN)).toBe(CANVAS_FONT_SIZE_MIN);
    expect(clampCanvasFontSize(CANVAS_FONT_SIZE_MAX)).toBe(CANVAS_FONT_SIZE_MAX);
  });

  it('하한 아래는 하한으로 죈다(글자가 사라져 되돌릴 수단이 없어지는 것을 막는다)', () => {
    expect(clampCanvasFontSize(0)).toBe(CANVAS_FONT_SIZE_MIN);
    expect(clampCanvasFontSize(-40)).toBe(CANVAS_FONT_SIZE_MIN);
  });

  it('상한 위는 상한으로 죈다', () => {
    expect(clampCanvasFontSize(1000)).toBe(CANVAS_FONT_SIZE_MAX);
  });

  it('비유한 값은 기본 크기로 떨어뜨린다', () => {
    expect(clampCanvasFontSize(Number.NaN)).toBe(DEFAULT_FONT_SIZE);
    expect(clampCanvasFontSize(Number.POSITIVE_INFINITY)).toBe(DEFAULT_FONT_SIZE);
  });

  it('허용 범위는 6~160 이다(통계 패널 손잡이와 같은 어휘)', () => {
    expect(CANVAS_FONT_SIZE_MIN).toBe(6);
    expect(CANVAS_FONT_SIZE_MAX).toBe(160);
  });
});

describe('resizeFontSize', () => {
  it('오른쪽·아래로 끌면 커진다(두 축의 평균만큼)', () => {
    expect(resizeFontSize(20, { dx: 10, dy: 10 })).toBe(30);
    expect(resizeFontSize(20, { dx: 10, dy: -4 })).toBe(23);
  });

  it('왼쪽·위로 끌면 작아진다', () => {
    expect(resizeFontSize(20, { dx: -10, dy: -10 })).toBe(10);
  });

  it('하한·상한을 넘지 않는다', () => {
    expect(resizeFontSize(8, { dx: -100, dy: -100 })).toBe(CANVAS_FONT_SIZE_MIN);
    expect(resizeFontSize(100, { dx: 200, dy: 200 })).toBe(CANVAS_FONT_SIZE_MAX);
  });

  it('시작 크기가 0·음수·비유한이면 기본 크기에서 출발한다', () => {
    expect(resizeFontSize(0, { dx: 0, dy: 0 })).toBe(DEFAULT_FONT_SIZE);
    expect(resizeFontSize(-5, { dx: 0, dy: 0 })).toBe(DEFAULT_FONT_SIZE);
    expect(resizeFontSize(Number.NaN, { dx: 2, dy: 2 })).toBe(DEFAULT_FONT_SIZE + 2);
  });

  it('비유한 델타는 그 축을 없던 일로 본다', () => {
    expect(resizeFontSize(20, { dx: Number.NaN, dy: 10 })).toBe(25);
    expect(resizeFontSize(20, { dx: 10, dy: Number.POSITIVE_INFINITY })).toBe(25);
  });
});

// --- 기하 쓰기 단일 통로 -------------------------------------------------

describe('patchNodeGeometry — REQ-06 단일 통로', () => {
  const elements: readonly CanvasElement[] = [
    boxEl('a', { x: 100, y: 100, w: 200, h: 200 }),
    lineEl('b', BASE_LINE),
    textEl('c', { x: 500, y: 500 }),
    boxEl('d', { x: 700, y: 700, w: 200, h: 200 }, 'ellipse'),
  ];

  it('대상만 바꾸고 형제는 참조 그대로 두며 새 배열을 낸다', () => {
    const out = patchNodeGeometry(elements, 'a', { x: 300, y: 400, w: 500, h: 600 });
    expect(out).not.toBe(elements);
    expect(out).toHaveLength(4);
    expect(out[0]).not.toBe(elements[0]);
    expect(out[0]!.geometry).toEqual({ x: 300, y: 400, w: 500, h: 600 });
    expect(out[1]).toBe(elements[1]);
    expect(out[2]).toBe(elements[2]);
    expect(out[3]).toBe(elements[3]);
  });

  it('기하 외의 필드(id·kind·style)는 그대로 실려 간다', () => {
    const out = patchNodeGeometry(elements, 'a', { x: 300, y: 400, w: 500, h: 600 });
    expect(out[0]!.id).toBe('a');
    expect(out[0]!.kind).toBe('rect');
    expect(out[0]!.style).toBe(elements[0]!.style);
  });

  it('입력 배열과 입력 요소를 건드리지 않는다', () => {
    const before = structuredClone(elements) as CanvasElement[];
    patchNodeGeometry(elements, 'a', { x: 9000, y: 9000, w: 9000, h: 9000 });
    expect(elements).toEqual(before);
  });

  it('모르는 id 는 무동작이되 여전히 새 배열을 낸다', () => {
    const out = patchNodeGeometry(elements, 'nope', { x: 300, y: 400, w: 500, h: 600 });
    expect(out).not.toBe(elements);
    expect(out).toEqual(elements);
    for (let i = 0; i < out.length; i += 1) expect(out[i]).toBe(elements[i]);
  });

  it('식별은 배열 위치가 아니라 nodeId 로만 한다(드래그 중 재정렬 방어)', () => {
    const reordered = [elements[3]!, elements[2]!, elements[1]!, elements[0]!];
    const out = patchNodeGeometry(reordered, 'a', { x: 300, y: 400, w: 500, h: 600 });
    expect(out[3]!.geometry).toEqual({ x: 300, y: 400, w: 500, h: 600 });
    expect(out[0]).toBe(reordered[0]);
  });

  it('ellipse 도 박스 기하를 받는다', () => {
    const out = patchNodeGeometry(elements, 'd', { x: 0, y: 0, w: 1000, h: 1000 });
    expect(out[3]!.geometry).toEqual({ x: 0, y: 0, w: 1000, h: 1000 });
  });

  it('line 은 선 기하를 받는다', () => {
    const out = patchNodeGeometry(elements, 'b', { x1: 0, y1: 0, x2: 1000, y2: 1000 });
    expect(out[1]!.geometry).toEqual({ x1: 0, y1: 0, x2: 1000, y2: 1000 });
  });

  it('text 는 점 기하를 받는다', () => {
    const out = patchNodeGeometry(elements, 'c', { x: 250, y: 750 });
    expect(out[2]!.geometry).toEqual({ x: 250, y: 750 });
  });

  it('종류와 어긋난 기하는 요소를 그대로 둔다', () => {
    expect(patchNodeGeometry(elements, 'a', { x1: 0, y1: 0, x2: 1000, y2: 1000 })[0]).toBe(elements[0]!);
    expect(patchNodeGeometry(elements, 'a', { x: 500, y: 500 })[0]).toBe(elements[0]);
    expect(patchNodeGeometry(elements, 'b', { x: 0, y: 0, w: 1000, h: 1000 })[1]).toBe(elements[1]);
    expect(patchNodeGeometry(elements, 'b', { x: 500, y: 500 })[1]).toBe(elements[1]);
    expect(patchNodeGeometry(elements, 'c', { x: 0, y: 0, w: 1000, h: 1000 })[2]).toBe(elements[2]);
    expect(patchNodeGeometry(elements, 'c', { x1: 0, y1: 0, x2: 1000, y2: 1000 })[2]).toBe(elements[2]);
  });

  it('손상된 좌표는 통로에서 0 으로 떨어져 NaN 이 config 에 들어가지 않는다', () => {
    const box = patchNodeGeometry(elements, 'a', {
      x: Number.NaN,
      y: 200,
      w: Number.POSITIVE_INFINITY,
      h: 300,
    });
    // 손상된 폭은 0 으로 떨어진 뒤 **최소 크기로 올라선다** — 통로는 화면에서 사라지는
    // 기하를 만들지 않는다(퇴화 방지). 그 자리가 0 이면 파서가 읽을 때 씨앗 기하로
    // 되살아나 요소가 순간이동한다.
    expect(box[0]!.geometry).toEqual({ x: 0, y: 200, w: MIN_ELEMENT_EXTENT, h: 300 });

    const line = patchNodeGeometry(elements, 'b', {
      x1: Number.NaN,
      y1: 100,
      x2: 900,
      y2: Number.NaN,
    });
    expect(line[1]!.geometry).toEqual({ x1: 0, y1: 100, x2: 900, y2: 0 });

    const point = patchNodeGeometry(elements, 'c', { x: Number.NaN, y: Number.NaN });
    expect(point[2]!.geometry).toEqual({ x: 0, y: 0 });
  });

  it('통로는 clamp 하지 않는다 — 스테이지 밖 좌표가 그대로 저장된다', () => {
    const out = patchNodeGeometry(elements, 'a', { x: -1500, y: 2500, w: 3000, h: 4000 });
    expect(out[0]!.geometry).toEqual({ x: -1500, y: 2500, w: 3000, h: 4000 });
  });

  it('빈 배열도 견딘다', () => {
    expect(patchNodeGeometry([], 'a', { x: 0, y: 0, w: 1000, h: 1000 })).toEqual([]);
  });

  it('편집 수학의 결과를 그대로 받아 쓴다(이동 → 통로)', () => {
    const moved = moveGeometry(elements[0]!.geometry as BoxGeometry, { dx: 100, dy: 100 });
    const out = patchNodeGeometry(elements, 'a', moved);
    expect(out[0]!.geometry).toEqual(moved);
    // 통로가 값을 그대로 싣지 않고 정제한 **새 객체**를 심는다(호출부와 참조를 공유하지 않는다).
    expect(out[0]!.geometry).not.toBe(moved);
  });
});

// --- 정수화와 퇴화 방지: 쓰기 통로가 소유한다 (SPEC-CANVAS-002 0.8.0) ----
//
// 이 절이 고정하는 것은 **책임의 자리**다. 조작 함수(이동·크기 조절)는 실수를 그대로
// 돌려주고, 정수가 되는 것도 최소 크기가 보장되는 것도 `patchNodeGeometry` **한 곳**에서
// 일어난다. 조작 함수마다 반올림하면 같은 손짓이 경로에 따라 한 단위씩 다른 곳에 떨어진다.

describe('patchNodeGeometry — 정수화', () => {
  const box = boxEl('a', { x: 100, y: 100, w: 200, h: 200 });
  const line = lineEl('b', BASE_LINE);
  const point = textEl('c', { x: 500, y: 500 });
  const elements: readonly CanvasElement[] = [box, line, point];

  it('소수 좌표를 정수로 반올림해 심는다', () => {
    const out = patchNodeGeometry(elements, 'a', { x: 10.4, y: 10.6, w: 99.5, h: 100.49 });
    expect(out[0]!.geometry).toEqual({ x: 10, y: 11, w: 100, h: 100 });
  });

  it('선 끝점도 정수로 반올림한다', () => {
    const out = patchNodeGeometry(elements, 'b', { x1: 1.2, y1: 2.7, x2: 300.5, y2: 4.4 });
    expect(out[1]!.geometry).toEqual({ x1: 1, y1: 3, x2: 301, y2: 4 });
  });

  it('기준점도 정수로 반올림한다', () => {
    const out = patchNodeGeometry(elements, 'c', { x: 249.6, y: 200.2 });
    expect(out[2]!.geometry).toEqual({ x: 250, y: 200 });
  });

  it('조작 함수 자체는 반올림하지 않는다(정수화의 자리는 통로 하나다)', () => {
    // 드래그 한 프레임의 이동량은 화면 px 를 역투영한 실수다. 여기서 접으면 축척이
    // 1 이 아닌 패널에서 드래그가 계단처럼 튄다.
    const moved = moveGeometry({ x: 100, y: 100, w: 200, h: 200 }, { dx: 0.4, dy: 0.6 });
    expect(moved.x).toBeCloseTo(100.4, 10);
    // 통로를 지나야 정수가 된다.
    const out = patchNodeGeometry(elements, 'a', moved);
    expect(out[0]!.geometry).toEqual({ x: 100, y: 101, w: 200, h: 200 });
  });
});

describe('patchNodeGeometry — 퇴화를 만들지 않는다', () => {
  const elements: readonly CanvasElement[] = [
    boxEl('a', { x: 100, y: 100, w: 200, h: 200 }),
    lineEl('b', BASE_LINE),
  ];

  it('크기 0 인 상자는 최소 크기로 올라선다(자리는 그대로)', () => {
    const out = patchNodeGeometry(elements, 'a', { x: 40, y: 50, w: 0, h: 0 });
    expect(out[0]!.geometry).toEqual({
      x: 40,
      y: 50,
      w: MIN_ELEMENT_EXTENT,
      h: MIN_ELEMENT_EXTENT,
    });
  });

  it('반올림해서 0 이 되는 크기도 최소 크기로 올라선다', () => {
    const out = patchNodeGeometry(elements, 'a', { x: 40, y: 50, w: 0.4, h: 300 });
    expect((out[0]!.geometry as BoxGeometry).w).toBe(MIN_ELEMENT_EXTENT);
  });

  it('음수 크기는 절대값으로 편 뒤 최소 크기를 지킨다(쓰기는 음수를 만들지 않는다)', () => {
    const out = patchNodeGeometry(elements, 'a', { x: 40, y: 50, w: -80, h: -0.2 });
    expect(out[0]!.geometry).toEqual({ x: 40, y: 50, w: 80, h: MIN_ELEMENT_EXTENT });
  });

  it('핸들을 반대 변까지 끌어 상자를 접어도 저장되는 크기는 0 이 아니다', () => {
    // 손잡이 드래그의 실제 경로다: resizeBox 가 폭 0 을 낼 수 있고, 그 값이 그대로
    // 저장되면 파서가 다음 읽기에서 씨앗 기하로 되살려 요소가 순간이동한다.
    const collapsed = resizeBox({ x: 100, y: 100, w: 200, h: 200 }, 'e', { x: 100, y: 150 });
    expect(collapsed.w).toBe(0);
    const out = patchNodeGeometry(elements, 'a', collapsed);
    expect((out[0]!.geometry as BoxGeometry).w).toBe(MIN_ELEMENT_EXTENT);
  });

  it('길이 0 인 선은 한 단위 벌어진다 — 씨앗 선으로 되돌리지 않는다', () => {
    // 되돌리면 지금 손에 쥐고 끄는 선이 캔버스를 가로질러 튄다. 한 단위 선은 손을
    // 조금만 더 움직이면 곧바로 자란다.
    const out = patchNodeGeometry(elements, 'b', { x1: 300, y1: 300, x2: 300, y2: 300 });
    expect(out[1]!.geometry).toEqual({
      x1: 300,
      y1: 300,
      x2: 300 + MIN_ELEMENT_EXTENT,
      y2: 300,
    });
  });

  it('길이가 있는 선은 손대지 않는다', () => {
    const out = patchNodeGeometry(elements, 'b', { x1: 300, y1: 300, x2: 301, y2: 300 });
    expect(out[1]!.geometry).toEqual({ x1: 300, y1: 300, x2: 301, y2: 300 });
  });

  it('통로가 쓴 기하는 파서의 퇴화 폴백을 깨우지 않는다(두 규율이 만나는 자리)', () => {
    // 이 시험이 두 모듈의 계약을 잇는다: 쓰기가 만들지 않으므로 읽기의 되살림은 옛
    // config 에서만 일어난다.
    const out = patchNodeGeometry(elements, 'a', { x: 40, y: 50, w: 0, h: 0 });
    const geometry = out[0]!.geometry as BoxGeometry;
    expect(geometry.w).toBeGreaterThanOrEqual(MIN_ELEMENT_EXTENT);
    expect(geometry.h).toBeGreaterThanOrEqual(MIN_ELEMENT_EXTENT);
  });
});

// --- 순수성 -------------------------------------------------------------

describe('순수성 — 어떤 호출도 입력을 바꾸지 않는다', () => {
  it('모든 공개 함수가 인자를 그대로 남긴다', () => {
    const box: BoxGeometry = { x: 200, y: 200, w: 400, h: 400 };
    const line: LineGeometry = { x1: 200, y1: 200, x2: 600, y2: 600 };
    const point: PointGeometry = { x: 500, y: 500 };
    const el = boxEl('a', box);
    const list: CanvasElement[] = [el, lineEl('b', line), textEl('c', point)];

    const snapshot = structuredClone({ box, line, point, list });

    handlePositions(el, PROJ, { measuredWidth: 40 });
    handlePositions(list[1]!, PROJ);
    handlePositions(list[2]!, PROJ, { measuredWidth: 40 });
    handlesFor('rect');
    handleRole('nw');
    isCornerHandle('se');
    moveGeometry(box, { dx: 100, dy: 100 });
    moveGeometry(line, { dx: 100, dy: 100 });
    moveGeometry(point, { dx: 100, dy: 100 });
    resizeBox(box, 'se', { x: 900, y: 900 }, { preserveAspect: true });
    resizeLine(line, 'p2', { x: 900, y: 900 }, { constrainAngle: true });
    resizeFontSize(20, { dx: 5, dy: 5 });
    clampCanvasFontSize(20);
    patchNodeGeometry(list, 'a', { x: 0, y: 0, w: 1000, h: 1000 });

    expect({ box, line, point, list }).toEqual(snapshot);
  });
});
