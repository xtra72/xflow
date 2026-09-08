// 캔버스 히트 테스트 단위 테스트 (SPEC-CANVAS-002 T1).
//
// 고정하는 계약 여섯:
//   1) 순회는 **배열 역순**이다 — 뒤가 위이고, 위에 있는 것이 이긴다(AC-01).
//   2) 판정은 **순방향 투영 후 스테이지 px 공간**에서 이뤄지며 집기 여유는 화면 양이다(AC-02).
//   3) 타원은 **바운딩 박스가 아니라 타원 방정식**으로 잡는다(박스 모서리는 빗나간다).
//   4) 텍스트 상자의 세로 기준은 `drawElement.TEXT_BASELINE === 'middle'` 이므로
//      기준점이 상자의 **세로 중심**이다(위험 R8 — 위·아래 경계를 각각 확인한다).
//   5) 퇴화 기하(크기 0 · 길이 0 · 비유한 좌표)는 던지지 않고 여유 크기 영역으로 폴백한다.
//   6) 라벨에는 **별도의 히트 영역이 없다**(라벨은 자기 기하가 없어 골라도 옮길 수 없다).
// DOM 을 쓰지 않으므로 jsdom 없이도 돈다.

import { describe, it, expect } from 'vitest';

import {
  HIT_TOLERANCE_PX,
  hitTest,
  type CanvasHit,
} from './canvasHitTest';
import { TEXT_BASELINE } from './drawElement';
import type {
  BoxGeometry,
  CanvasElement,
  ElementStyle,
  LineGeometry,
  PointGeometry,
} from './canvasConfig';
import type { PxPoint, StageSize } from './canvasGeometry';

/** 대표 스테이지(800x600). 정규화 좌표가 딱 떨어지는 px 가 되도록 고른 크기다. */
const STAGE: StageSize = { width: 800, height: 600 };

/** 판정할 때마다 넘길 빈 실측 폭 표(도형은 폭을 쓰지 않는다). */
const NO_WIDTHS: Record<string, number> = {};

const at = (x: number, y: number): PxPoint => ({ x, y });

function rect(id: string, geometry: BoxGeometry, style: ElementStyle = {}): CanvasElement {
  return { id, kind: 'rect', geometry, style };
}

function ellipse(id: string, geometry: BoxGeometry, style: ElementStyle = {}): CanvasElement {
  return { id, kind: 'ellipse', geometry, style };
}

function line(id: string, geometry: LineGeometry, style: ElementStyle = {}): CanvasElement {
  return { id, kind: 'line', geometry, style };
}

function text(
  id: string,
  geometry: PointGeometry,
  style: ElementStyle = {},
  body = '문구',
): CanvasElement {
  return { id, kind: 'text', geometry, style, text: body };
}

/** 하나만 담아 판정한다 — 도형별 경계 시험이 이웃 요소에 오염되지 않게 한다. */
function hitOne(
  el: CanvasElement,
  point: PxPoint,
  widths: Record<string, number> = NO_WIDTHS,
): CanvasHit | undefined {
  return hitTest([el], point, STAGE, widths);
}

describe('HIT_TOLERANCE_PX', () => {
  it('명세가 정한 6px 이다(두께 1px 선을 누를 수 있게 하는 값)', () => {
    expect(HIT_TOLERANCE_PX).toBe(6);
  });
});

describe('hitTest — 빈 입력과 손상 입력', () => {
  it('요소가 없으면 아무것도 맞지 않는다', () => {
    expect(hitTest([], at(400, 300), STAGE, NO_WIDTHS)).toBeUndefined();
  });

  it('비유한 포인터 좌표는 판정하지 않는다(NaN 비교가 조용히 빗나가는 것을 막는다)', () => {
    const el = rect('r', { x: 0, y: 0, w: 1, h: 1 });
    expect(hitOne(el, at(NaN, 300))).toBeUndefined();
    expect(hitOne(el, at(400, NaN))).toBeUndefined();
    expect(hitOne(el, at(Infinity, 300))).toBeUndefined();
    expect(hitOne(el, at(400, -Infinity))).toBeUndefined();
  });

  it('비유한 기하는 던지지 않고 0 으로 떨어진 자리에서 판정된다', () => {
    const el = rect('r', { x: NaN, y: 0.5, w: Infinity, h: 0.2 });
    // projectBox 가 비유한 좌표를 0 으로 떨어뜨리므로 상자는 x 0, y 300, w 0, h 120 이다.
    expect(() => hitOne(el, at(0, 300))).not.toThrow();
    expect(hitOne(el, at(0, 300))).toEqual({ nodeId: 'r' });
    expect(hitOne(el, at(20, 300))).toBeUndefined();
  });

  it('크기가 아직 잡히지 않은 스테이지에서도 원점 둘레의 여유 상자로 잡힌다', () => {
    const el = rect('r', { x: 0.25, y: 0.25, w: 0.5, h: 0.5 });
    expect(hitTest([el], at(0, 0), { width: 0, height: 0 }, NO_WIDTHS)).toEqual({ nodeId: 'r' });
    expect(hitTest([el], at(20, 0), { width: 0, height: 0 }, NO_WIDTHS)).toBeUndefined();
  });
});

describe('hitTest — rect', () => {
  // 정규화 0.25/0.25/0.5/0.5 → px 200..600 × 150..450. 여유 6px 을 더하면 194..606 × 144..456.
  const el = rect('r', { x: 0.25, y: 0.25, w: 0.5, h: 0.5 });

  it('상자 안을 누르면 맞는다', () => {
    expect(hitOne(el, at(400, 300))).toEqual({ nodeId: 'r' });
  });

  it('여유를 더한 네 경계 바로 안쪽은 맞는다', () => {
    expect(hitOne(el, at(194, 300))).toEqual({ nodeId: 'r' });
    expect(hitOne(el, at(606, 300))).toEqual({ nodeId: 'r' });
    expect(hitOne(el, at(400, 144))).toEqual({ nodeId: 'r' });
    expect(hitOne(el, at(400, 456))).toEqual({ nodeId: 'r' });
  });

  it('여유를 더한 네 경계 바로 바깥은 빗나간다', () => {
    expect(hitOne(el, at(193.9, 300))).toBeUndefined();
    expect(hitOne(el, at(606.1, 300))).toBeUndefined();
    expect(hitOne(el, at(400, 143.9))).toBeUndefined();
    expect(hitOne(el, at(400, 456.1))).toBeUndefined();
  });

  it('음수 크기 상자는 양수 범위로 정규화되어 같은 자리를 잡는다', () => {
    const flipped = rect('r', { x: 0.75, y: 0.75, w: -0.5, h: -0.5 });
    expect(hitOne(flipped, at(400, 300))).toEqual({ nodeId: 'r' });
    expect(hitOne(flipped, at(194, 144))).toEqual({ nodeId: 'r' });
    expect(hitOne(flipped, at(193, 300))).toBeUndefined();
  });

  it('폭·높이가 0 인 퇴화 상자도 여유 크기 상자로 잡힌다(화면에서 되살릴 수 있다)', () => {
    const degenerate = rect('r', { x: 0.5, y: 0.5, w: 0, h: 0 });
    expect(hitOne(degenerate, at(400, 300))).toEqual({ nodeId: 'r' });
    expect(hitOne(degenerate, at(406, 306))).toEqual({ nodeId: 'r' });
    expect(hitOne(degenerate, at(407, 300))).toBeUndefined();
  });
});

describe('hitTest — ellipse', () => {
  // px 200..600 × 150..450 → 중심 (400,300), 반지름 200×150. 여유를 더해 206×156.
  const el = ellipse('e', { x: 0.25, y: 0.25, w: 0.5, h: 0.5 });

  it('중심을 누르면 맞는다', () => {
    expect(hitOne(el, at(400, 300))).toEqual({ nodeId: 'e' });
  });

  it('바운딩 박스 안이지만 타원 밖인 모서리는 빗나간다(박스 판정이 아님을 증명한다)', () => {
    // (206,156) 은 여유를 더한 박스(194..606 × 144..456) 안이다.
    expect(hitOne(el, at(206, 156))).toBeUndefined();
    expect(hitOne(el, at(594, 444))).toBeUndefined();
  });

  it('축 위 경계는 여유만큼 부푼 반지름에서 갈린다', () => {
    expect(hitOne(el, at(606, 300))).toEqual({ nodeId: 'e' });
    expect(hitOne(el, at(607, 300))).toBeUndefined();
    expect(hitOne(el, at(400, 456))).toEqual({ nodeId: 'e' });
    expect(hitOne(el, at(400, 457))).toBeUndefined();
  });

  it('반지름 하나가 0 인 퇴화 타원은 여유 반지름의 점 판정으로 폴백한다', () => {
    const flat = ellipse('e', { x: 0.5, y: 0.25, w: 0, h: 0.5 });
    expect(hitOne(flat, at(404, 300))).toEqual({ nodeId: 'e' });
    expect(hitOne(flat, at(408, 300))).toBeUndefined();
  });

  it('두 반지름이 모두 0 인 퇴화 타원도 여유 반지름의 점 판정이다', () => {
    const dot = ellipse('e', { x: 0.5, y: 0.5, w: 0, h: 0 });
    expect(hitOne(dot, at(400, 300))).toEqual({ nodeId: 'e' });
    expect(hitOne(dot, at(404, 300))).toEqual({ nodeId: 'e' });
    expect(hitOne(dot, at(407, 300))).toBeUndefined();
  });
});

describe('hitTest — line', () => {
  // px (200,300) → (600,300) 인 수평선.
  const geo: LineGeometry = { x1: 0.25, y1: 0.5, x2: 0.75, y2: 0.5 };

  it('두께가 미지정이면 기본 1px 이고, 임계는 여유 6px 이다(머리카락 선을 누를 수 있다)', () => {
    const el = line('l', geo);
    expect(hitOne(el, at(400, 300))).toEqual({ nodeId: 'l' });
    expect(hitOne(el, at(400, 305))).toEqual({ nodeId: 'l' });
    expect(hitOne(el, at(400, 307))).toBeUndefined();
  });

  it('두께 1px 을 명시해도 임계는 여유 6px 이다', () => {
    const el = line('l', geo, { strokeWidth: 1 });
    expect(hitOne(el, at(400, 306))).toEqual({ nodeId: 'l' });
    expect(hitOne(el, at(400, 306.1))).toBeUndefined();
  });

  it('두꺼운 선은 제 두께의 절반까지 잡힌다', () => {
    const el = line('l', geo, { strokeWidth: 40 });
    expect(hitOne(el, at(400, 320))).toEqual({ nodeId: 'l' });
    expect(hitOne(el, at(400, 320.1))).toBeUndefined();
  });

  it('손상된 두께는 0 으로 보고 여유만 남긴다', () => {
    for (const strokeWidth of [NaN, -5, 0, Infinity]) {
      const el = line('l', geo, { strokeWidth });
      expect(hitOne(el, at(400, 306))).toEqual({ nodeId: 'l' });
      expect(hitOne(el, at(400, 307))).toBeUndefined();
    }
  });

  it('선분 밖으로 벗어난 지점은 끝점까지의 거리로 판정된다', () => {
    const el = line('l', geo);
    // 시작점 앞쪽(투영 매개변수 < 0).
    expect(hitOne(el, at(195, 300))).toEqual({ nodeId: 'l' });
    expect(hitOne(el, at(150, 300))).toBeUndefined();
    // 끝점 뒤쪽(투영 매개변수 > 1).
    expect(hitOne(el, at(605, 300))).toEqual({ nodeId: 'l' });
    expect(hitOne(el, at(650, 300))).toBeUndefined();
  });

  it('두 끝점이 같은 퇴화 선분은 점 판정으로 폴백한다', () => {
    const el = line('l', { x1: 0.5, y1: 0.5, x2: 0.5, y2: 0.5 });
    expect(hitOne(el, at(400, 300))).toEqual({ nodeId: 'l' });
    expect(hitOne(el, at(400, 305))).toEqual({ nodeId: 'l' });
    expect(hitOne(el, at(400, 310))).toBeUndefined();
  });

  it('비스듬한 선도 수직 거리로 판정한다', () => {
    // px (200,150) → (600,450). 방향 (400,300), 길이 500.
    const el = line('l', { x1: 0.25, y1: 0.25, x2: 0.75, y2: 0.75 });
    expect(hitOne(el, at(400, 300))).toEqual({ nodeId: 'l' });
    // 중점에서 법선 방향(0.6,-0.8)으로 5px → 임계 6px 안.
    expect(hitOne(el, at(403, 296))).toEqual({ nodeId: 'l' });
    // 같은 방향으로 10px → 임계 밖.
    expect(hitOne(el, at(406, 292))).toBeUndefined();
  });
});

describe('hitTest — text (세로 기준이 상자의 중심이다)', () => {
  // 기준점 (400,300), 글자 크기 20, 실측 폭 120.
  const geo: PointGeometry = { x: 0.5, y: 0.5 };
  const widths: Record<string, number> = { t: 120 };

  it('drawElement 의 기준선은 middle 이다(이 파일의 기대값이 딛고 선 전제)', () => {
    expect(TEXT_BASELINE).toBe('middle');
  });

  it('좌측 정렬: 좌측 끝 원점에서 실측 폭만큼 뻗은 상자를 잡는다', () => {
    const el = text('t', geo, { fontSize: 20 });
    // 상자 x 400..520 → 여유를 더해 394..526.
    expect(hitOne(el, at(400, 300), widths)).toEqual({ nodeId: 't' });
    expect(hitOne(el, at(394, 300), widths)).toEqual({ nodeId: 't' });
    expect(hitOne(el, at(393, 300), widths)).toBeUndefined();
    expect(hitOne(el, at(526, 300), widths)).toEqual({ nodeId: 't' });
    expect(hitOne(el, at(527, 300), widths)).toBeUndefined();
  });

  it('기준점 y 는 상자의 세로 중심이다 — 위·아래 경계가 대칭이다(위험 R8)', () => {
    const el = text('t', geo, { fontSize: 20 });
    // 상자 y 290..310 → 여유를 더해 284..316.
    expect(hitOne(el, at(400, 284), widths)).toEqual({ nodeId: 't' });
    expect(hitOne(el, at(400, 283), widths)).toBeUndefined();
    expect(hitOne(el, at(400, 316), widths)).toEqual({ nodeId: 't' });
    expect(hitOne(el, at(400, 317), widths)).toBeUndefined();
  });

  it('기준점을 상자 상단으로 착각했다면 나올 판정을 배제한다', () => {
    const el = text('t', geo, { fontSize: 20 });
    // 상단 기준이면 상자는 300..320(여유 포함 294..326)이 되어,
    // 아래 두 기대값이 정확히 반대로 뒤집힌다.
    expect(hitOne(el, at(400, 285), widths)).toEqual({ nodeId: 't' });
    expect(hitOne(el, at(400, 325), widths)).toBeUndefined();
  });

  it('가운데 정렬: 원점이 실측 폭의 절반만큼 왼쪽으로 물러난다', () => {
    const el = text('t', geo, { fontSize: 20, align: 'center' });
    // 상자 x 340..460 → 여유를 더해 334..466.
    expect(hitOne(el, at(334, 300), widths)).toEqual({ nodeId: 't' });
    expect(hitOne(el, at(333, 300), widths)).toBeUndefined();
    expect(hitOne(el, at(466, 300), widths)).toEqual({ nodeId: 't' });
    expect(hitOne(el, at(467, 300), widths)).toBeUndefined();
  });

  it('우측 정렬: 원점이 실측 폭만큼 왼쪽으로 물러난다', () => {
    const el = text('t', geo, { fontSize: 20, align: 'right' });
    // 상자 x 280..400 → 여유를 더해 274..406.
    expect(hitOne(el, at(274, 300), widths)).toEqual({ nodeId: 't' });
    expect(hitOne(el, at(273, 300), widths)).toBeUndefined();
    expect(hitOne(el, at(406, 300), widths)).toEqual({ nodeId: 't' });
    expect(hitOne(el, at(407, 300), widths)).toBeUndefined();
  });

  it('실측 폭이 아직 없으면 기준점 둘레의 여유 상자로 폴백한다(AC-E7)', () => {
    const el = text('t', geo, { fontSize: 20 });
    // 폭 0 이어도 여유가 상자를 남긴다 — 잡을 수 없는 폭 0 상자를 만들지 않는다.
    expect(hitOne(el, at(400, 300), NO_WIDTHS)).toEqual({ nodeId: 't' });
    expect(hitOne(el, at(406, 300), NO_WIDTHS)).toEqual({ nodeId: 't' });
    expect(hitOne(el, at(407, 300), NO_WIDTHS)).toBeUndefined();
    // 세로는 여전히 글자 크기로 정해진다.
    expect(hitOne(el, at(400, 284), NO_WIDTHS)).toEqual({ nodeId: 't' });
    expect(hitOne(el, at(400, 283), NO_WIDTHS)).toBeUndefined();
  });

  it('손상된 실측 폭도 같은 폴백을 탄다', () => {
    const el = text('t', geo, { fontSize: 20 });
    for (const width of [0, -30, NaN, Infinity]) {
      expect(hitOne(el, at(400, 300), { t: width })).toEqual({ nodeId: 't' });
      expect(hitOne(el, at(407, 300), { t: width })).toBeUndefined();
    }
  });

  it('다른 요소의 실측 폭을 빌려 쓰지 않는다(폭은 요소 id 로만 찾는다)', () => {
    const el = text('t', geo, { fontSize: 20 });
    expect(hitOne(el, at(450, 300), { other: 120 })).toBeUndefined();
    expect(hitOne(el, at(450, 300), { t: 120 })).toEqual({ nodeId: 't' });
  });

  it('글자 크기가 미지정·손상이면 기본 14px 로 상자를 세운다', () => {
    // 기본 14 → 상자 y 293..307, 여유를 더해 287..313.
    for (const style of [
      {},
      { fontSize: 0 },
      { fontSize: -8 },
      { fontSize: NaN },
    ] as ElementStyle[]) {
      const el = text('t', geo, style);
      expect(hitOne(el, at(400, 287), widths)).toEqual({ nodeId: 't' });
      expect(hitOne(el, at(400, 286), widths)).toBeUndefined();
    }
  });
});

describe('hitTest — z-order 와 가시성', () => {
  const lower = rect('a', { x: 0.25, y: 0.25, w: 0.5, h: 0.5 });
  const upper = rect('b', { x: 0.25, y: 0.25, w: 0.5, h: 0.5 });

  it('겹친 자리에서는 배열 뒤(=위)에 있는 요소가 이긴다', () => {
    expect(hitTest([lower, upper], at(400, 300), STAGE, NO_WIDTHS)).toEqual({ nodeId: 'b' });
    // 순서를 뒤집으면 승자도 뒤집힌다 — 배열 순서가 유일한 z-order 임을 보인다.
    expect(hitTest([upper, lower], at(400, 300), STAGE, NO_WIDTHS)).toEqual({ nodeId: 'a' });
  });

  it('히트 결과는 레코드이며 002 는 partId 를 채우지 않는다(REQ-06)', () => {
    const hit = hitTest([lower, upper], at(400, 300), STAGE, NO_WIDTHS);
    expect(hit).toEqual({ nodeId: 'b' });
    expect(hit && 'partId' in hit).toBe(false);
  });

  it('visible === false 인 요소는 최상위여도 건너뛴다', () => {
    const hidden = rect('b', { x: 0.25, y: 0.25, w: 0.5, h: 0.5 }, { visible: false });
    expect(hitTest([lower, hidden], at(400, 300), STAGE, NO_WIDTHS)).toEqual({ nodeId: 'a' });
  });

  it('맞는 요소가 모두 숨겨져 있으면 아무것도 맞지 않는다', () => {
    const hidden = rect('a', { x: 0, y: 0, w: 1, h: 1 }, { visible: false });
    expect(hitTest([hidden], at(400, 300), STAGE, NO_WIDTHS)).toBeUndefined();
  });

  it('visible 이 true 이거나 미지정이면 정상 판정된다', () => {
    const shown = rect('a', { x: 0.25, y: 0.25, w: 0.5, h: 0.5 }, { visible: true });
    expect(hitTest([shown], at(400, 300), STAGE, NO_WIDTHS)).toEqual({ nodeId: 'a' });
  });
});

describe('hitTest — 스테이지 밖 배치(clamp 하지 않는다)', () => {
  it('음수 좌표에 놓인 요소도 그 자리에서 잡힌다', () => {
    const el = rect('r', { x: -0.5, y: -0.5, w: 0.2, h: 0.2 });
    // px -400..-240 × -300..-180.
    expect(hitOne(el, at(-300, -200))).toEqual({ nodeId: 'r' });
    expect(hitOne(el, at(0, 0))).toBeUndefined();
  });

  it('1 을 넘는 좌표에 놓인 요소도 그 자리에서 잡힌다', () => {
    const el = ellipse('e', { x: 1.1, y: 1.1, w: 0.2, h: 0.2 });
    // px 880..1040 × 660..780 → 중심 (960,720).
    expect(hitOne(el, at(960, 720))).toEqual({ nodeId: 'e' });
    expect(hitOne(el, at(400, 300))).toBeUndefined();
  });

  it('선 끝점이 스테이지 밖으로 나가도 그대로 판정한다', () => {
    const el = line('l', { x1: -0.5, y1: 0.5, x2: 0.5, y2: 0.5 });
    // px (-400,300) → (400,300).
    expect(hitOne(el, at(-200, 300))).toEqual({ nodeId: 'l' });
    expect(hitOne(el, at(-200, 320))).toBeUndefined();
  });
});

describe('hitTest — 라벨에는 별도 히트 영역이 없다', () => {
  it('라벨이 붙은 도형은 제 기하 안에서만 잡힌다', () => {
    // 중심 (400,300)의 작은 사각형에 긴 라벨이 붙어 있다. 라벨은 labelAnchor(중심)에서
    // 파생될 뿐 자기 기하가 없으므로, 라벨 글자가 뻗어 나갈 자리를 눌러도 잡히지 않는다.
    const el: CanvasElement = {
      id: 'r',
      kind: 'rect',
      geometry: { x: 0.49, y: 0.49, w: 0.02, h: 0.02 },
      style: {},
      text: '아주 긴 라벨 문구가 여기에 붙어 있다',
    };
    const widths: Record<string, number> = { r: 400 };
    expect(hitTest([el], at(400, 300), STAGE, widths)).toEqual({ nodeId: 'r' });
    // 라벨이 상자였다면 잡혔을 지점(도형 밖).
    expect(hitTest([el], at(560, 300), STAGE, widths)).toBeUndefined();
  });

  it('라벨이 붙은 선도 선분 둘레에서만 잡힌다', () => {
    const el: CanvasElement = {
      id: 'l',
      kind: 'line',
      geometry: { x1: 0.25, y1: 0.5, x2: 0.75, y2: 0.5 },
      style: {},
      text: '라벨',
    };
    const widths: Record<string, number> = { l: 200 };
    expect(hitTest([el], at(400, 300), STAGE, widths)).toEqual({ nodeId: 'l' });
    expect(hitTest([el], at(400, 340), STAGE, widths)).toBeUndefined();
  });
});

describe('hitTest — 순수성', () => {
  it('요소 배열·요소·지점·폭 표를 하나도 바꾸지 않는다', () => {
    const elements: CanvasElement[] = [
      rect('a', { x: 0.25, y: 0.25, w: 0.5, h: 0.5 }),
      text('t', { x: 0.5, y: 0.5 }, { fontSize: 20 }),
    ];
    const point = at(400, 300);
    const stage: StageSize = { width: 800, height: 600 };
    const widths: Record<string, number> = { t: 120 };
    const before = structuredClone({ elements, point, stage, widths });

    expect(hitTest(elements, point, stage, widths)).toEqual({ nodeId: 't' });

    expect({ elements, point, stage, widths }).toEqual(before);
    // 역순 순회가 원본 배열의 z-order 를 뒤집지 않았는지 따로 못박는다.
    expect(elements.map((el) => el.id)).toEqual(['a', 't']);
  });
});
