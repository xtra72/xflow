// 캔버스 편집 수학 단위 테스트 (SPEC-CANVAS-002 T7).
//
// 고정하는 계약 여섯:
//   1) 종류별 핸들 집합 — 특히 `text` 는 글자 크기 핸들 **하나뿐이며 박스 핸들이 없다**.
//   2) 핸들 px 좌표가 `canvasGeometry` 의 투영과 **정확히 일치한다**(위험 R1 방어).
//      그래서 기대값을 손으로 쓴 숫자가 아니라 투영 함수의 결과로 쓴다 — 손으로 쓰면
//      투영이 바뀌어도 테스트가 통과해 두 벌이 갈라진 것을 잡지 못한다.
//   3) 크기 조절 결과는 **언제나 양수 크기**다. 반대 변을 넘어 뒤집어도 그렇다(위험 R7).
//   4) 이동은 **clamp 하지 않는다** — 스테이지 밖으로 나가도 좌표가 잘리지 않는다(가정 A5).
//   5) `patchNodeGeometryZ` 가 아니라 `patchNodeGeometry` **하나**가 기하 쓰기 통로이며,
//      식별은 `nodeId` 로만 한다(REQ-06).
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
  type StageSize,
} from './canvasGeometry';
import {
  DEFAULT_FONT_SIZE,
  type BoxGeometry,
  type CanvasElement,
  type ElementStyle,
  type LineGeometry,
  type PointGeometry,
} from './canvasConfig';

/** 대표 스테이지(800x600) — 001 기하 테스트와 같은 크기다. */
const STAGE: StageSize = { width: 800, height: 600 };

/** 기준 박스 — 네 변이 0.2/0.6 이라 뒤집힘을 눈으로 따라가기 쉽다. */
const BASE_BOX: BoxGeometry = { x: 0.2, y: 0.2, w: 0.4, h: 0.4 };

/** 종횡비 2:1 기준 박스 — Shift 유지 여부가 결과로 드러나는 크기다. */
const RATIO_BOX: BoxGeometry = { x: 0.2, y: 0.2, w: 0.4, h: 0.2 };

/** 기준 선 — 대각선이라 각도 죔의 결과가 축과 구분된다. */
const BASE_LINE: LineGeometry = { x1: 0.2, y1: 0.2, x2: 0.6, y2: 0.6 };

function boxEl(id: string, geometry: BoxGeometry, kind: 'rect' | 'ellipse' = 'rect'): CanvasElement {
  return { id, kind, geometry, style: {} } as CanvasElement;
}

function lineEl(id: string, geometry: LineGeometry): CanvasElement {
  return { id, kind: 'line', geometry, style: {} };
}

function textEl(id: string, geometry: PointGeometry, style: ElementStyle = {}): CanvasElement {
  return { id, kind: 'text', geometry, style, text: '온도' };
}

/** 부동소수 오차를 흡수하는 박스 비교(정규화 좌표는 0.1 단위 합산이 잦다). */
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
    const geo: BoxGeometry = { x: 0.1, y: 0.2, w: 0.5, h: 0.25 };
    const pb = projectBox(geo, STAGE);
    const handles = handlePositions(boxEl('a', geo), STAGE);

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
    const geo: BoxGeometry = { x: 0.1, y: 0.2, w: 0.5, h: 0.25 };
    expect(handlePositions(boxEl('a', geo, 'ellipse'), STAGE)).toEqual(
      handlePositions(boxEl('a', geo), STAGE),
    );
  });

  it('스테이지가 커지면 핸들도 함께 옮겨 간다(측정원이 하나다)', () => {
    const geo: BoxGeometry = { x: 0.1, y: 0.2, w: 0.5, h: 0.25 };
    const bigger: StageSize = { width: 1600, height: 1200 };
    const pb = projectBox(geo, bigger);
    const handles = handlePositions(boxEl('a', geo), bigger);
    expect(handles[0]!.point).toEqual({ x: pb.x, y: pb.y });
    expect(handles[4]!.point).toEqual({ x: pb.x + pb.w, y: pb.y + pb.h });
  });

  it('음수 크기 박스의 핸들은 화면에 보이는 양수 상자 위에 놓인다', () => {
    const geo: BoxGeometry = { x: 0.6, y: 0.6, w: -0.2, h: -0.1 };
    const handles = handlePositions(boxEl('a', geo), STAGE);
    expect(handles.find((h) => h.id === 'nw')!.point).toEqual({ x: 320, y: 300 });
    expect(handles.find((h) => h.id === 'se')!.point).toEqual({ x: 480, y: 360 });
  });

  it('line 두 핸들이 projectLine 의 끝점과 정확히 일치한다', () => {
    const geo: LineGeometry = { x1: 0.1, y1: 0.2, x2: 0.9, y2: 0.8 };
    const pl = projectLine(geo, STAGE);
    const handles = handlePositions(lineEl('l', geo), STAGE);

    expect(handles.map((h) => h.id)).toEqual(['p1', 'p2']);
    expect(handles[0]!.point).toEqual({ x: pl.x1, y: pl.y1 });
    expect(handles[1]!.point).toEqual({ x: pl.x2, y: pl.y2 });
    expect(handles.every((h) => h.role === 'geometry')).toBe(true);
  });

  it('text 핸들은 하나이며 role 이 fontSize 다(기하를 쓰지 않는다)', () => {
    const geo: PointGeometry = { x: 0.5, y: 0.5 };
    const handles = handlePositions(textEl('t', geo, { fontSize: 20 }), STAGE, {
      measuredWidth: 60,
    });
    expect(handles).toHaveLength(1);
    expect(handles[0]!.id).toBe('font');
    expect(handles[0]!.role).toBe('fontSize');
  });

  it('text 핸들 자리는 resolveTextOrigin 이 낸 원점 + 실측 폭 + fontSize/2 다', () => {
    const geo: PointGeometry = { x: 0.5, y: 0.5 };
    const width = 60;
    const origin = resolveTextOrigin(projectPoint(geo, STAGE), 'left', width);
    const handles = handlePositions(textEl('t', geo, { fontSize: 20 }), STAGE, {
      measuredWidth: width,
    });
    expect(handles[0]!.point).toEqual({ x: origin.x + width, y: origin.y + 10 });
  });

  it('text 정렬이 center 면 원점이 왼쪽으로 밀리고 핸들도 함께 밀린다', () => {
    const geo: PointGeometry = { x: 0.5, y: 0.5 };
    const width = 60;
    const origin = resolveTextOrigin(projectPoint(geo, STAGE), 'center', width);
    const handles = handlePositions(textEl('t', geo, { fontSize: 20, align: 'center' }), STAGE, {
      measuredWidth: width,
    });
    expect(handles[0]!.point).toEqual({ x: origin.x + width, y: origin.y + 10 });
    expect(handles[0]!.point.x).toBe(430);
  });

  it('fontSize 미지정이면 기본 크기(14)로 자리를 잡는다', () => {
    const handles = handlePositions(textEl('t', { x: 0.5, y: 0.5 }), STAGE, { measuredWidth: 0 });
    expect(handles[0]!.point).toEqual({ x: 400, y: 300 + DEFAULT_FONT_SIZE / 2 });
  });

  it('실측 폭이 아직 없으면(옵션 자체 생략) 기준점에 붙는다', () => {
    const handles = handlePositions(textEl('t', { x: 0.5, y: 0.5 }, { fontSize: 20 }), STAGE);
    expect(handles[0]!.point).toEqual({ x: 400, y: 310 });
  });

  it('손상된 실측 폭(NaN·음수)은 0 으로 본다', () => {
    const base = handlePositions(textEl('t', { x: 0.5, y: 0.5 }, { fontSize: 20 }), STAGE, {
      measuredWidth: Number.NaN,
    });
    expect(base[0]!.point).toEqual({ x: 400, y: 310 });
    const negative = handlePositions(textEl('t', { x: 0.5, y: 0.5 }, { fontSize: 20 }), STAGE, {
      measuredWidth: -5,
    });
    expect(negative[0]!.point).toEqual({ x: 400, y: 310 });
  });

  it('범위 밖 fontSize 는 핸들 자리 계산에서 범위로 죄인다', () => {
    const huge = handlePositions(textEl('t', { x: 0.5, y: 0.5 }, { fontSize: 500 }), STAGE);
    expect(huge[0]!.point.y).toBe(300 + CANVAS_FONT_SIZE_MAX / 2);
    const zero = handlePositions(textEl('t', { x: 0.5, y: 0.5 }, { fontSize: 0 }), STAGE);
    expect(zero[0]!.point.y).toBe(300 + CANVAS_FONT_SIZE_MIN / 2);
  });

  it('크기가 아직 잡히지 않은 스테이지에서도 NaN 이 아니라 0 을 낸다', () => {
    const handles = handlePositions(boxEl('a', BASE_BOX), { width: 0, height: 0 });
    for (const h of handles) {
      expect(Number.isFinite(h.point.x)).toBe(true);
      expect(Number.isFinite(h.point.y)).toBe(true);
    }
  });
});

// --- 이동 ---------------------------------------------------------------

describe('moveGeometry', () => {
  it('박스는 좌상단만 옮기고 크기를 유지한다', () => {
    const out = moveGeometry(BASE_BOX, { dx: 0.05, dy: -0.05 });
    expect(out).toEqual({ x: 0.2 + 0.05, y: 0.2 - 0.05, w: 0.4, h: 0.4 });
  });

  it('선은 두 끝점이 함께 움직인다(몸통 드래그)', () => {
    const out = moveGeometry(BASE_LINE, { dx: 0.1, dy: 0.2 });
    expect(out).toEqual({
      x1: 0.2 + 0.1,
      y1: 0.2 + 0.2,
      x2: 0.6 + 0.1,
      y2: 0.6 + 0.2,
    });
  });

  it('문구는 기준점을 옮긴다', () => {
    expect(moveGeometry({ x: 0.5, y: 0.5 }, { dx: -0.25, dy: 0.25 })).toEqual({
      x: 0.5 - 0.25,
      y: 0.5 + 0.25,
    });
  });

  it('스테이지 밖으로 나가도 clamp 하지 않는다(가정 A5)', () => {
    const right = moveGeometry({ x: 0.9, y: 0.9, w: 0.2, h: 0.2 }, { dx: 0.5, dy: 0.5 });
    expect(right.x).toBeCloseTo(1.4, 10);
    expect(right.y).toBeCloseTo(1.4, 10);
    expect(right.x).toBeGreaterThan(1);

    const left = moveGeometry({ x: 0.1, y: 0.1, w: 0.2, h: 0.2 }, { dx: -2, dy: -3 });
    expect(left.x).toBeCloseTo(-1.9, 10);
    expect(left.y).toBeCloseTo(-2.9, 10);
    expect(left.x).toBeLessThan(0);
  });

  it('선·문구도 마찬가지로 잘리지 않는다', () => {
    expect(moveGeometry(BASE_LINE, { dx: 3, dy: 3 }).x2).toBeCloseTo(3.6, 10);
    expect(moveGeometry({ x: 0.5, y: 0.5 }, { dx: -4, dy: 4 }).x).toBeCloseTo(-3.5, 10);
  });

  it('비유한 델타는 그 축의 이동을 없던 일로 본다(NaN 기하를 만들지 않는다)', () => {
    expect(moveGeometry(BASE_BOX, { dx: Number.NaN, dy: 0.1 })).toEqual({
      x: 0.2,
      y: 0.2 + 0.1,
      w: 0.4,
      h: 0.4,
    });
    expect(moveGeometry(BASE_BOX, { dx: 0.1, dy: Number.POSITIVE_INFINITY })).toEqual({
      x: 0.2 + 0.1,
      y: 0.2,
      w: 0.4,
      h: 0.4,
    });
  });

  it('손상된 기하 필드는 0 으로 떨어뜨린 뒤 옮긴다', () => {
    expect(moveGeometry({ x: Number.NaN, y: 0.2, w: 0.4, h: Number.NaN }, { dx: 0.1, dy: 0 })).toEqual(
      { x: 0.1, y: 0.2, w: 0.4, h: 0 },
    );
    expect(
      moveGeometry({ x1: Number.NaN, y1: 0.2, x2: 0.6, y2: Number.NaN }, { dx: 0, dy: 0 }),
    ).toEqual({ x1: 0, y1: 0.2, x2: 0.6, y2: 0 });
    expect(moveGeometry({ x: Number.NaN, y: Number.NaN }, { dx: 0, dy: 0 })).toEqual({
      x: 0,
      y: 0,
    });
  });

  it('입력을 건드리지 않고 새 객체를 낸다', () => {
    const input: BoxGeometry = { ...BASE_BOX };
    const out = moveGeometry(input, { dx: 0.1, dy: 0.1 });
    expect(input).toEqual(BASE_BOX);
    expect(out).not.toBe(input);
  });
});

// --- 크기 조절: 박스 ----------------------------------------------------

describe('resizeBox — 8개 핸들', () => {
  const inside = { x: 0.5, y: 0.5 };

  it('nw 는 좌·상 변을 함께 옮긴다', () => {
    expectBox(resizeBox(BASE_BOX, 'nw', inside), { x: 0.5, y: 0.5, w: 0.1, h: 0.1 });
  });

  it('n 은 위쪽 변만 옮긴다', () => {
    expectBox(resizeBox(BASE_BOX, 'n', inside), { x: 0.2, y: 0.5, w: 0.4, h: 0.1 });
  });

  it('ne 는 우·상 변을 함께 옮긴다', () => {
    expectBox(resizeBox(BASE_BOX, 'ne', inside), { x: 0.2, y: 0.5, w: 0.3, h: 0.1 });
  });

  it('e 는 오른쪽 변만 옮긴다', () => {
    expectBox(resizeBox(BASE_BOX, 'e', inside), { x: 0.2, y: 0.2, w: 0.3, h: 0.4 });
  });

  it('se 는 우·하 변을 함께 옮긴다', () => {
    expectBox(resizeBox(BASE_BOX, 'se', inside), { x: 0.2, y: 0.2, w: 0.3, h: 0.3 });
  });

  it('s 는 아래쪽 변만 옮긴다', () => {
    expectBox(resizeBox(BASE_BOX, 's', inside), { x: 0.2, y: 0.2, w: 0.4, h: 0.3 });
  });

  it('sw 는 좌·하 변을 함께 옮긴다', () => {
    expectBox(resizeBox(BASE_BOX, 'sw', inside), { x: 0.5, y: 0.2, w: 0.1, h: 0.3 });
  });

  it('w 는 왼쪽 변만 옮긴다', () => {
    expectBox(resizeBox(BASE_BOX, 'w', inside), { x: 0.5, y: 0.2, w: 0.1, h: 0.4 });
  });

  it('스테이지 밖으로 끌어도 clamp 하지 않는다', () => {
    const out = resizeBox(BASE_BOX, 'se', { x: 1.8, y: 2.4 });
    expectBox(out, { x: 0.2, y: 0.2, w: 1.6, h: 2.2 });
    expect(out.w).toBeGreaterThan(1);
  });
});

describe('resizeBox — 뒤집힘 정규화(위험 R7)', () => {
  it('se 를 좌상단 너머로 끌면 박스가 양수로 정규화된다', () => {
    const out = resizeBox(BASE_BOX, 'se', { x: 0.05, y: 0 });
    expectBox(out, { x: 0.05, y: 0, w: 0.15, h: 0.2 });
    expect(out.w).toBeGreaterThan(0);
    expect(out.h).toBeGreaterThan(0);
  });

  it('nw 를 우하단 너머로 끌어도 마찬가지다', () => {
    const out = resizeBox(BASE_BOX, 'nw', { x: 0.9, y: 0.95 });
    expectBox(out, { x: 0.6, y: 0.6, w: 0.3, h: 0.35 });
  });

  it('한 축만 뒤집혀도(변 핸들) 음수 크기가 나오지 않는다', () => {
    expectBox(resizeBox(BASE_BOX, 'e', { x: 0.05, y: 0.5 }), {
      x: 0.05,
      y: 0.2,
      w: 0.15,
      h: 0.4,
    });
    expectBox(resizeBox(BASE_BOX, 'n', { x: 0.5, y: 0.95 }), {
      x: 0.2,
      y: 0.6,
      w: 0.4,
      h: 0.35,
    });
  });

  it('8개 핸들 어디를 반대 끝 너머로 끌어도 음수 크기가 저장되지 않는다', () => {
    for (const handle of BOX_HANDLE_IDS) {
      for (const pointer of [
        { x: -0.9, y: -0.9 },
        { x: 1.9, y: 1.9 },
      ]) {
        const out = resizeBox(BASE_BOX, handle, pointer);
        expect(out.w).toBeGreaterThanOrEqual(0);
        expect(out.h).toBeGreaterThanOrEqual(0);
      }
    }
  });

  it('음수 크기 박스를 받아도 결과는 정규화된 양수 박스다(읽기 관용 · 쓰기 엄격)', () => {
    const flipped: BoxGeometry = { x: 0.6, y: 0.6, w: -0.4, h: -0.4 };
    expectBox(resizeBox(flipped, 'se', { x: 0.5, y: 0.5 }), {
      x: 0.2,
      y: 0.2,
      w: 0.3,
      h: 0.3,
    });
  });
});

describe('resizeBox — 종횡비 유지(Shift)', () => {
  it('모서리에서 Shift 를 누르면 원래 비(2:1)를 지킨다 — 가로가 이기는 경우', () => {
    const out = resizeBox(RATIO_BOX, 'se', { x: 0.8, y: 0.5 }, { preserveAspect: true });
    expectBox(out, { x: 0.2, y: 0.2, w: 0.6, h: 0.3 });
    expect(out.w / out.h).toBeCloseTo(2, 10);
  });

  it('세로가 더 멀리 가면 세로가 비를 정한다', () => {
    const out = resizeBox(RATIO_BOX, 'se', { x: 0.4, y: 0.9 }, { preserveAspect: true });
    expectBox(out, { x: 0.2, y: 0.2, w: 1.4, h: 0.7 });
    expect(out.w / out.h).toBeCloseTo(2, 10);
  });

  it('반대 모서리(nw)를 잡아 음의 방향으로 끌어도 비가 지켜진다', () => {
    const out = resizeBox(RATIO_BOX, 'nw', { x: 0.2, y: 0.3 }, { preserveAspect: true });
    expectBox(out, { x: 0.2, y: 0.2, w: 0.4, h: 0.2 });
    expect(out.w / out.h).toBeCloseTo(2, 10);
  });

  it('Shift 를 누르지 않으면 같은 드래그가 비를 깬다', () => {
    const free = resizeBox(RATIO_BOX, 'se', { x: 0.4, y: 0.9 });
    expectBox(free, { x: 0.2, y: 0.2, w: 0.2, h: 0.7 });
    expect(free.w / free.h).not.toBeCloseTo(2, 3);
  });

  it('변 핸들에서는 Shift 가 아무 일도 하지 않는다(유지할 모서리가 없다)', () => {
    const shifted = resizeBox(RATIO_BOX, 'e', { x: 0.8, y: 0.9 }, { preserveAspect: true });
    const free = resizeBox(RATIO_BOX, 'e', { x: 0.8, y: 0.9 });
    expect(shifted).toEqual(free);
    expectBox(shifted, { x: 0.2, y: 0.2, w: 0.6, h: 0.2 });
  });

  it('퇴화 박스(폭 0 또는 높이 0)에는 유지할 비가 없어 자유 조절이 된다', () => {
    const zeroW = resizeBox(
      { x: 0.2, y: 0.2, w: 0, h: 0.2 },
      'se',
      { x: 0.6, y: 0.6 },
      { preserveAspect: true },
    );
    expectBox(zeroW, { x: 0.2, y: 0.2, w: 0.4, h: 0.4 });

    const zeroH = resizeBox(
      { x: 0.2, y: 0.2, w: 0.4, h: 0 },
      'se',
      { x: 0.6, y: 0.6 },
      { preserveAspect: true },
    );
    expectBox(zeroH, { x: 0.2, y: 0.2, w: 0.4, h: 0.4 });
  });
});

describe('resizeBox — 견고성', () => {
  it('비유한 포인터는 조작을 무시하고 정규화된 원본을 돌려준다', () => {
    expectBox(resizeBox(BASE_BOX, 'se', { x: Number.NaN, y: 0.5 }), BASE_BOX);
    expectBox(resizeBox(BASE_BOX, 'se', { x: 0.5, y: Number.POSITIVE_INFINITY }), BASE_BOX);
  });

  it('비유한 포인터라도 뒤집힌 입력 박스는 정규화해서 돌려준다', () => {
    expectBox(resizeBox({ x: 0.6, y: 0.6, w: -0.4, h: -0.4 }, 'nw', { x: Number.NaN, y: 0 }), {
      x: 0.2,
      y: 0.2,
      w: 0.4,
      h: 0.4,
    });
  });

  it('손상된 박스 필드는 0 으로 떨어뜨린 뒤 조절한다', () => {
    expectBox(resizeBox({ x: Number.NaN, y: 0.2, w: 0.4, h: 0.4 }, 'e', { x: 0.5, y: 0.5 }), {
      x: 0,
      y: 0.2,
      w: 0.5,
      h: 0.4,
    });
  });

  it('입력을 건드리지 않고 새 객체를 낸다', () => {
    const input: BoxGeometry = { ...BASE_BOX };
    const out = resizeBox(input, 'se', { x: 0.5, y: 0.5 });
    expect(input).toEqual(BASE_BOX);
    expect(out).not.toBe(input);
  });
});

// --- 크기 조절: 선 ------------------------------------------------------

describe('resizeLine', () => {
  it('p1 핸들은 첫 끝점만 옮긴다', () => {
    expectLine(resizeLine(BASE_LINE, 'p1', { x: 0.1, y: 0.9 }), {
      x1: 0.1,
      y1: 0.9,
      x2: 0.6,
      y2: 0.6,
    });
  });

  it('p2 핸들은 둘째 끝점만 옮긴다', () => {
    expectLine(resizeLine(BASE_LINE, 'p2', { x: 0.1, y: 0.9 }), {
      x1: 0.2,
      y1: 0.2,
      x2: 0.1,
      y2: 0.9,
    });
  });

  it('끝점이 스테이지 밖으로 나가도 clamp 하지 않는다', () => {
    expectLine(resizeLine(BASE_LINE, 'p2', { x: 1.8, y: -0.4 }), {
      x1: 0.2,
      y1: 0.2,
      x2: 1.8,
      y2: -0.4,
    });
  });

  it('Shift 는 거의 수평인 드래그를 0° 로 죈다', () => {
    const out = resizeLine(BASE_LINE, 'p2', { x: 0.7, y: 0.22 }, { constrainAngle: true });
    expect(out.y2).toBe(0.2);
    expect(out.x2).toBeCloseTo(0.2 + Math.hypot(0.5, 0.02), 10);
  });

  it('Shift 는 거의 수직인 드래그를 90° 로 죈다', () => {
    const out = resizeLine(BASE_LINE, 'p2', { x: 0.22, y: 0.7 }, { constrainAngle: true });
    expect(out.x2).toBeCloseTo(0.2, 10);
    expect(out.y2).toBeCloseTo(0.2 + Math.hypot(0.02, 0.5), 10);
  });

  it('Shift 는 대각선 드래그를 45° 로 죄어 두 축 변위를 같게 만든다', () => {
    const out = resizeLine(BASE_LINE, 'p2', { x: 0.7, y: 0.68 }, { constrainAngle: true });
    expect(out.x2 - 0.2).toBeCloseTo(out.y2 - 0.2, 10);
    expect(Math.hypot(out.x2 - 0.2, out.y2 - 0.2)).toBeCloseTo(Math.hypot(0.5, 0.48), 10);
  });

  it('Shift 죔의 기준은 잡지 않은 반대 끝점이다(p1 을 끌면 p2 가 앵커)', () => {
    const out = resizeLine(BASE_LINE, 'p1', { x: 0.1, y: 0.58 }, { constrainAngle: true });
    expect(out.y1).toBeCloseTo(0.6, 10);
    expect(out.x1).toBeCloseTo(0.6 - Math.hypot(0.5, 0.02), 10);
    expect(out.x2).toBe(0.6);
    expect(out.y2).toBe(0.6);
  });

  it('앵커와 같은 지점으로 끌면(길이 0) 그 자리에 남는다', () => {
    const out = resizeLine(BASE_LINE, 'p2', { x: 0.2, y: 0.2 }, { constrainAngle: true });
    expectLine(out, { x1: 0.2, y1: 0.2, x2: 0.2, y2: 0.2 });
  });

  it('Shift 없이는 죄지 않는다', () => {
    expectLine(resizeLine(BASE_LINE, 'p2', { x: 0.7, y: 0.22 }), {
      x1: 0.2,
      y1: 0.2,
      x2: 0.7,
      y2: 0.22,
    });
  });

  it('비유한 포인터는 조작을 무시한다', () => {
    expectLine(resizeLine(BASE_LINE, 'p1', { x: Number.NaN, y: 0.5 }), BASE_LINE);
    expectLine(resizeLine(BASE_LINE, 'p2', { x: 0.5, y: Number.NEGATIVE_INFINITY }), BASE_LINE);
  });

  it('손상된 선 필드는 0 으로 떨어뜨린 뒤 조절한다', () => {
    expectLine(
      resizeLine({ x1: 0.2, y1: 0.2, x2: Number.NaN, y2: 0.6 }, 'p1', { x: 0.4, y: 0.4 }),
      { x1: 0.4, y1: 0.4, x2: 0, y2: 0.6 },
    );
  });

  it('입력을 건드리지 않고 새 객체를 낸다', () => {
    const input: LineGeometry = { ...BASE_LINE };
    const out = resizeLine(input, 'p1', { x: 0.4, y: 0.4 });
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
    boxEl('a', { x: 0.1, y: 0.1, w: 0.2, h: 0.2 }),
    lineEl('b', BASE_LINE),
    textEl('c', { x: 0.5, y: 0.5 }),
    boxEl('d', { x: 0.7, y: 0.7, w: 0.2, h: 0.2 }, 'ellipse'),
  ];

  it('대상만 바꾸고 형제는 참조 그대로 두며 새 배열을 낸다', () => {
    const out = patchNodeGeometry(elements, 'a', { x: 0.3, y: 0.4, w: 0.5, h: 0.6 });
    expect(out).not.toBe(elements);
    expect(out).toHaveLength(4);
    expect(out[0]).not.toBe(elements[0]);
    expect(out[0]!.geometry).toEqual({ x: 0.3, y: 0.4, w: 0.5, h: 0.6 });
    expect(out[1]).toBe(elements[1]);
    expect(out[2]).toBe(elements[2]);
    expect(out[3]).toBe(elements[3]);
  });

  it('기하 외의 필드(id·kind·style)는 그대로 실려 간다', () => {
    const out = patchNodeGeometry(elements, 'a', { x: 0.3, y: 0.4, w: 0.5, h: 0.6 });
    expect(out[0]!.id).toBe('a');
    expect(out[0]!.kind).toBe('rect');
    expect(out[0]!.style).toBe(elements[0]!.style);
  });

  it('입력 배열과 입력 요소를 건드리지 않는다', () => {
    const before = structuredClone(elements) as CanvasElement[];
    patchNodeGeometry(elements, 'a', { x: 9, y: 9, w: 9, h: 9 });
    expect(elements).toEqual(before);
  });

  it('모르는 id 는 무동작이되 여전히 새 배열을 낸다', () => {
    const out = patchNodeGeometry(elements, 'nope', { x: 0.3, y: 0.4, w: 0.5, h: 0.6 });
    expect(out).not.toBe(elements);
    expect(out).toEqual(elements);
    for (let i = 0; i < out.length; i += 1) expect(out[i]).toBe(elements[i]);
  });

  it('식별은 배열 위치가 아니라 nodeId 로만 한다(드래그 중 재정렬 방어)', () => {
    const reordered = [elements[3]!, elements[2]!, elements[1]!, elements[0]!];
    const out = patchNodeGeometry(reordered, 'a', { x: 0.3, y: 0.4, w: 0.5, h: 0.6 });
    expect(out[3]!.geometry).toEqual({ x: 0.3, y: 0.4, w: 0.5, h: 0.6 });
    expect(out[0]).toBe(reordered[0]);
  });

  it('ellipse 도 박스 기하를 받는다', () => {
    const out = patchNodeGeometry(elements, 'd', { x: 0, y: 0, w: 1, h: 1 });
    expect(out[3]!.geometry).toEqual({ x: 0, y: 0, w: 1, h: 1 });
  });

  it('line 은 선 기하를 받는다', () => {
    const out = patchNodeGeometry(elements, 'b', { x1: 0, y1: 0, x2: 1, y2: 1 });
    expect(out[1]!.geometry).toEqual({ x1: 0, y1: 0, x2: 1, y2: 1 });
  });

  it('text 는 점 기하를 받는다', () => {
    const out = patchNodeGeometry(elements, 'c', { x: 0.25, y: 0.75 });
    expect(out[2]!.geometry).toEqual({ x: 0.25, y: 0.75 });
  });

  it('종류와 어긋난 기하는 요소를 그대로 둔다', () => {
    expect(patchNodeGeometry(elements, 'a', { x1: 0, y1: 0, x2: 1, y2: 1 })[0]).toBe(elements[0]!);
    expect(patchNodeGeometry(elements, 'a', { x: 0.5, y: 0.5 })[0]).toBe(elements[0]);
    expect(patchNodeGeometry(elements, 'b', { x: 0, y: 0, w: 1, h: 1 })[1]).toBe(elements[1]);
    expect(patchNodeGeometry(elements, 'b', { x: 0.5, y: 0.5 })[1]).toBe(elements[1]);
    expect(patchNodeGeometry(elements, 'c', { x: 0, y: 0, w: 1, h: 1 })[2]).toBe(elements[2]);
    expect(patchNodeGeometry(elements, 'c', { x1: 0, y1: 0, x2: 1, y2: 1 })[2]).toBe(elements[2]);
  });

  it('손상된 좌표는 통로에서 0 으로 떨어져 NaN 이 config 에 들어가지 않는다', () => {
    const box = patchNodeGeometry(elements, 'a', {
      x: Number.NaN,
      y: 0.2,
      w: Number.POSITIVE_INFINITY,
      h: 0.3,
    });
    expect(box[0]!.geometry).toEqual({ x: 0, y: 0.2, w: 0, h: 0.3 });

    const line = patchNodeGeometry(elements, 'b', {
      x1: Number.NaN,
      y1: 0.1,
      x2: 0.9,
      y2: Number.NaN,
    });
    expect(line[1]!.geometry).toEqual({ x1: 0, y1: 0.1, x2: 0.9, y2: 0 });

    const point = patchNodeGeometry(elements, 'c', { x: Number.NaN, y: Number.NaN });
    expect(point[2]!.geometry).toEqual({ x: 0, y: 0 });
  });

  it('통로는 clamp 하지 않는다 — 스테이지 밖 좌표가 그대로 저장된다', () => {
    const out = patchNodeGeometry(elements, 'a', { x: -1.5, y: 2.5, w: 3, h: 4 });
    expect(out[0]!.geometry).toEqual({ x: -1.5, y: 2.5, w: 3, h: 4 });
  });

  it('빈 배열도 견딘다', () => {
    expect(patchNodeGeometry([], 'a', { x: 0, y: 0, w: 1, h: 1 })).toEqual([]);
  });

  it('편집 수학의 결과를 그대로 받아 쓴다(이동 → 통로)', () => {
    const moved = moveGeometry(elements[0]!.geometry as BoxGeometry, { dx: 0.1, dy: 0.1 });
    const out = patchNodeGeometry(elements, 'a', moved);
    expect(out[0]!.geometry).toEqual(moved);
    // 통로가 값을 그대로 싣지 않고 정제한 **새 객체**를 심는다(호출부와 참조를 공유하지 않는다).
    expect(out[0]!.geometry).not.toBe(moved);
  });
});

// --- 순수성 -------------------------------------------------------------

describe('순수성 — 어떤 호출도 입력을 바꾸지 않는다', () => {
  it('모든 공개 함수가 인자를 그대로 남긴다', () => {
    const box: BoxGeometry = { x: 0.2, y: 0.2, w: 0.4, h: 0.4 };
    const line: LineGeometry = { x1: 0.2, y1: 0.2, x2: 0.6, y2: 0.6 };
    const point: PointGeometry = { x: 0.5, y: 0.5 };
    const el = boxEl('a', box);
    const list: CanvasElement[] = [el, lineEl('b', line), textEl('c', point)];

    const snapshot = structuredClone({ box, line, point, list });

    handlePositions(el, STAGE, { measuredWidth: 40 });
    handlePositions(list[1]!, STAGE);
    handlePositions(list[2]!, STAGE, { measuredWidth: 40 });
    handlesFor('rect');
    handleRole('nw');
    isCornerHandle('se');
    moveGeometry(box, { dx: 0.1, dy: 0.1 });
    moveGeometry(line, { dx: 0.1, dy: 0.1 });
    moveGeometry(point, { dx: 0.1, dy: 0.1 });
    resizeBox(box, 'se', { x: 0.9, y: 0.9 }, { preserveAspect: true });
    resizeLine(line, 'p2', { x: 0.9, y: 0.9 }, { constrainAngle: true });
    resizeFontSize(20, { dx: 5, dy: 5 });
    clampCanvasFontSize(20);
    patchNodeGeometry(list, 'a', { x: 0, y: 0, w: 1, h: 1 });

    expect({ box, line, point, list }).toEqual(snapshot);
  });
});
