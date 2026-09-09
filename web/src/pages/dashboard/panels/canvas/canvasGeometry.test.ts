// 캔버스 기하 순수 함수 단위 테스트 (SPEC-CANVAS-001 T2).
//
// 고정하는 계약 넷:
//   1) 백킹 버퍼는 정수 device px 이며 손상된 dpr/표시 크기에 예외 없이 방어한다.
//   2) 정수 캔버스 좌표는 `÷ 캔버스 크기 × 스테이지 크기` 로 투영될 뿐 **잘리지 않는다**
//      (캔버스 밖 배치 보존). 역투영은 그 정확한 역이다.
//   3) 0 크기 스테이지·0 크기 캔버스는 NaN 이 아니라 유한한 0 을 낸다.
//   4) 글자 폭은 인자로만 들어온다 — DOM(measureText) 없이 정렬이 결정된다.
// DOM 을 쓰지 않으므로 jsdom 없이도 돈다.

import { describe, it, expect } from 'vitest';

import {
  MAX_DEVICE_PIXEL_RATIO,
  computeBackingSize,
  ellipseParams,
  fitCanvasSizeToStage,
  labelAnchor,
  projectBox,
  projectLine,
  projectPoint,
  resolveTextOrigin,
  stageLattice,
  toCanvasTextAlign,
  unprojectBox,
  unprojectPoint,
  type CanvasProjection,
  type StageSize,
} from './canvasGeometry';
import {
  DEFAULT_CANVAS_SIZE,
  MAX_CANVAS_DIMENSION,
  MIN_CANVAS_DIMENSION,
  type CanvasElement,
  type CanvasSize,
} from './canvasConfig';

/** 대표 스테이지(800x600). */
const STAGE: StageSize = { width: 800, height: 600 };

/**
 * 대표 캔버스 — 기본 크기(500x400) 그대로다.
 *
 * 위 스테이지와 짝지으면 축척이 **가로 1.6 · 세로 1.5** 로 서로 다르다. 그것이 이 픽스처를
 * 고른 이유다: 두 축이 같은 축척이면 축을 뒤바꾼 실수가 시험을 통과한다.
 */
const CANVAS: CanvasSize = { ...DEFAULT_CANVAS_SIZE };

/** 대표 투영 한 벌. */
const PROJ: CanvasProjection = { stage: STAGE, canvas: CANVAS };

/** 크기가 아직 잡히지 않은 스테이지(첫 프레임·접힌 패널). */
const ZERO_STAGE: CanvasProjection = { stage: { width: 0, height: 0 }, canvas: CANVAS };

describe('computeBackingSize', () => {
  it('dpr 1 은 표시 크기를 그대로 쓴다', () => {
    expect(computeBackingSize(800, 600, 1)).toEqual({ width: 800, height: 600, scale: 1 });
  });

  it('dpr 2/3 은 표시 크기를 배율만큼 키운다', () => {
    expect(computeBackingSize(800, 600, 2)).toEqual({ width: 1600, height: 1200, scale: 2 });
    expect(computeBackingSize(800, 600, 3)).toEqual({ width: 2400, height: 1800, scale: 3 });
  });

  it('소수 dpr 은 정수 device px 로 반올림한다', () => {
    // 300.4*1.5 = 450.6 → 451, 200.6*1.5 = 300.9 → 301.
    expect(computeBackingSize(300.4, 200.6, 1.5)).toEqual({
      width: 451,
      height: 301,
      scale: 1.5,
    });
  });

  it('상한(8)까지는 그대로 받아들인다', () => {
    expect(computeBackingSize(100, 100, MAX_DEVICE_PIXEL_RATIO)).toEqual({
      width: 800,
      height: 800,
      scale: MAX_DEVICE_PIXEL_RATIO,
    });
  });

  it('터무니없는 dpr 은 1 로 떨어뜨린다(백킹 버퍼 메모리 폭주 방어)', () => {
    expect(computeBackingSize(800, 600, 99)).toEqual({ width: 800, height: 600, scale: 1 });
  });

  it('0·음수·비유한 dpr 은 1 로 떨어뜨린다', () => {
    expect(computeBackingSize(800, 600, 0)).toEqual({ width: 800, height: 600, scale: 1 });
    expect(computeBackingSize(800, 600, -2)).toEqual({ width: 800, height: 600, scale: 1 });
    expect(computeBackingSize(800, 600, NaN)).toEqual({ width: 800, height: 600, scale: 1 });
    expect(computeBackingSize(800, 600, Infinity)).toEqual({ width: 800, height: 600, scale: 1 });
    expect(computeBackingSize(800, 600, -Infinity)).toEqual({ width: 800, height: 600, scale: 1 });
  });

  it('0·음수·비유한 표시 크기는 0 이다(렌더 층이 프레임을 건너뛴다)', () => {
    expect(computeBackingSize(0, 0, 2)).toEqual({ width: 0, height: 0, scale: 2 });
    expect(computeBackingSize(-100, 600, 2)).toEqual({ width: 0, height: 1200, scale: 2 });
    expect(computeBackingSize(NaN, Infinity, 2)).toEqual({ width: 0, height: 0, scale: 2 });
  });

  it('양수지만 1px 미만인 표시 크기는 최소 1px 을 남긴다', () => {
    expect(computeBackingSize(0.2, 0.4, 1)).toEqual({ width: 1, height: 1, scale: 1 });
  });
});

describe('projectBox / projectLine / projectPoint — 500x400 캔버스를 800x600 스테이지에', () => {
  it('캔버스 좌표를 축마다 제 축척으로 투영한다', () => {
    // 가로 ×1.6, 세로 ×1.5 — 두 축척이 다르므로 축을 뒤바꾼 구현은 여기서 걸린다.
    expect(projectBox({ x: 125, y: 200, w: 250, h: 100 }, PROJ)).toEqual({
      x: 200,
      y: 300,
      w: 400,
      h: 150,
    });
    expect(projectLine({ x1: 0, y1: 0, x2: 500, y2: 400 }, PROJ)).toEqual({
      x1: 0,
      y1: 0,
      x2: 800,
      y2: 600,
    });
    expect(projectPoint({ x: 250, y: 200 }, PROJ)).toEqual({ x: 400, y: 300 });
  });

  it('캔버스 밖 좌표를 clamp 하지 않는다 — 밖으로 걸치는 배치는 정당한 저술이다', () => {
    expect(projectBox({ x: -125, y: 600, w: 1000, h: 40 }, PROJ)).toEqual({
      x: -200,
      y: 900,
      w: 1600,
      h: 60,
    });
    expect(projectLine({ x1: -250, y1: 0, x2: 750, y2: 800 }, PROJ)).toEqual({
      x1: -400,
      y1: 0,
      x2: 1200,
      y2: 1200,
    });
    expect(projectPoint({ x: 625, y: -200 }, PROJ)).toEqual({ x: 1000, y: -300 });
  });

  it('0 크기 스테이지는 NaN 이 아니라 유한한 0 을 낸다', () => {
    expect(projectBox({ x: 250, y: 200, w: 250, h: 200 }, ZERO_STAGE)).toEqual({
      x: 0,
      y: 0,
      w: 0,
      h: 0,
    });
    expect(projectLine({ x1: 50, y1: 80, x2: 450, y2: 320 }, ZERO_STAGE)).toEqual({
      x1: 0,
      y1: 0,
      x2: 0,
      y2: 0,
    });
    expect(projectPoint({ x: 250, y: 200 }, ZERO_STAGE)).toEqual({ x: 0, y: 0 });
  });

  it('0 크기 캔버스도 NaN 이 아니라 유한한 0 을 낸다(0 으로 나누지 않는다)', () => {
    const zeroCanvas: CanvasProjection = { stage: STAGE, canvas: { width: 0, height: 0 } };
    expect(projectPoint({ x: 250, y: 200 }, zeroCanvas)).toEqual({ x: 0, y: 0 });
  });

  it('손상된 스테이지 크기(음수·비유한)도 0 으로 다룬다', () => {
    expect(
      projectPoint({ x: 250, y: 200 }, { stage: { width: -10, height: NaN }, canvas: CANVAS }),
    ).toEqual({ x: 0, y: 0 });
  });

  it('손상된 좌표는 0 으로 보정해 NaN 전파를 막는다', () => {
    expect(projectPoint({ x: NaN, y: 200 }, PROJ)).toEqual({ x: 0, y: 300 });
    expect(projectBox({ x: 250, y: Infinity, w: NaN, h: 200 }, PROJ)).toEqual({
      x: 400,
      y: 0,
      w: 0,
      h: 300,
    });
    expect(projectLine({ x1: NaN, y1: 200, x2: 250, y2: Infinity }, PROJ)).toEqual({
      x1: 0,
      y1: 300,
      x2: 400,
      y2: 0,
    });
  });
});

describe('unprojectPoint / unprojectBox — 투영의 역', () => {
  it('스테이지 px 를 캔버스 단위로 되돌린다', () => {
    expect(unprojectPoint({ x: 200, y: 300 }, PROJ)).toEqual({ x: 125, y: 200 });
    expect(unprojectBox({ x: 200, y: 300, w: 400, h: 150 }, PROJ)).toEqual({
      x: 125,
      y: 200,
      w: 250,
      h: 100,
    });
  });

  it('투영 → 역투영이 제자리로 돌아온다(같은 투영 한 벌을 쓰는 한)', () => {
    const geo = { x: 137, y: 211, w: 43, h: 97 };
    expect(unprojectBox(projectBox(geo, PROJ), PROJ)).toEqual(geo);
  });

  it('정수화하지 않는다 — 반올림은 쓰기 통로의 몫이다', () => {
    // 1px 은 이 축척에서 0.625 단위다. 여기서 0 이나 1 로 접으면 드래그가 계단처럼 튄다.
    const back = unprojectPoint({ x: 1, y: 1 }, PROJ);
    expect(back.x).toBe(0.625);
    expect(back.y).toBeCloseTo(2 / 3, 10);
  });

  it('0 크기 스테이지는 NaN 이 아니라 유한한 0 을 낸다(0 으로 나누지 않는다)', () => {
    expect(unprojectPoint({ x: 100, y: 100 }, ZERO_STAGE)).toEqual({ x: 0, y: 0 });
  });

  it('손상된 px 는 0 으로 보정한다', () => {
    expect(unprojectPoint({ x: NaN, y: 300 }, PROJ)).toEqual({ x: 0, y: 200 });
  });
});

describe('ellipseParams', () => {
  it('좌상단+크기를 중심+반지름으로 바꾼다', () => {
    expect(ellipseParams({ x: 10, y: 20, w: 100, h: 40 })).toEqual({
      cx: 60,
      cy: 40,
      rx: 50,
      ry: 20,
    });
  });

  it('음수 크기 박스도 같은 타원으로 그린다(반지름만 절대값)', () => {
    // 위 테스트와 같은 사각형을 반대 모서리에서 저술한 형태다.
    expect(ellipseParams({ x: 110, y: 60, w: -100, h: -40 })).toEqual({
      cx: 60,
      cy: 40,
      rx: 50,
      ry: 20,
    });
  });

  it('손상된 수치는 0 으로 보정한다', () => {
    expect(ellipseParams({ x: NaN, y: 0, w: Infinity, h: 10 })).toEqual({
      cx: 0,
      cy: 5,
      rx: 0,
      ry: 5,
    });
  });
});

describe('resolveTextOrigin', () => {
  const point = { x: 100, y: 50 };

  it('left 는 기준점이 곧 좌측 끝이다', () => {
    expect(resolveTextOrigin(point, 'left', 40)).toEqual({ x: 100, y: 50 });
  });

  it('center 는 폭의 절반만큼 왼쪽으로 옮긴다', () => {
    expect(resolveTextOrigin(point, 'center', 40)).toEqual({ x: 80, y: 50 });
  });

  it('right 는 폭만큼 왼쪽으로 옮긴다', () => {
    expect(resolveTextOrigin(point, 'right', 40)).toEqual({ x: 60, y: 50 });
  });

  it('측정 실패(비유한 폭)는 0 폭으로 본다 — 글자가 기준점에 남는다', () => {
    expect(resolveTextOrigin(point, 'center', NaN)).toEqual({ x: 100, y: 50 });
    expect(resolveTextOrigin(point, 'right', Infinity)).toEqual({ x: 100, y: 50 });
  });

  it('손상된 기준점도 0 으로 보정한다', () => {
    expect(resolveTextOrigin({ x: NaN, y: NaN }, 'left', 10)).toEqual({ x: 0, y: 0 });
  });
});

describe('toCanvasTextAlign', () => {
  it('세 정렬을 canvas textAlign 값으로 옮긴다', () => {
    expect(toCanvasTextAlign('left')).toBe('left');
    expect(toCanvasTextAlign('center')).toBe('center');
    expect(toCanvasTextAlign('right')).toBe('right');
  });
});

describe('labelAnchor', () => {
  it('rect 는 박스 중심이다', () => {
    const el: CanvasElement = {
      id: 'r',
      kind: 'rect',
      geometry: { x: 125, y: 100, w: 250, h: 200 },
      style: {},
    };
    expect(labelAnchor(el, PROJ)).toEqual({ x: 400, y: 300 });
  });

  it('ellipse 도 박스 중심이다', () => {
    const el: CanvasElement = {
      id: 'e',
      kind: 'ellipse',
      geometry: { x: 125, y: 100, w: 250, h: 200 },
      style: {},
    };
    expect(labelAnchor(el, PROJ)).toEqual({ x: 400, y: 300 });
  });

  it('line 은 두 끝점의 중점이다', () => {
    const el: CanvasElement = {
      id: 'l',
      kind: 'line',
      geometry: { x1: 0, y1: 0, x2: 500, y2: 200 },
      style: {},
    };
    expect(labelAnchor(el, PROJ)).toEqual({ x: 400, y: 150 });
  });

  it('text 는 자기 기준점이다', () => {
    const el: CanvasElement = {
      id: 't',
      kind: 'text',
      geometry: { x: 250, y: 100 },
      style: {},
    };
    expect(labelAnchor(el, PROJ)).toEqual({ x: 400, y: 150 });
  });
});

// --- 그리는 영역을 격자 칸에 맞춘다 (0.9.0 · 사용 시험 "격자가 일정하지 않음") -----
//
// 이 절이 지키는 성질은 **둘**이다.
//   1) **그려질 선이 온전한 픽셀에 앉는다**(0.9.0). 한 칸이 소수 px 이면 1px 선이 두 장치
//      픽셀에 나뉘어 칠해져 선마다 굵기가 달라 보인다.
//   2) **한 칸이 정사각형이다**(0.10.0). 0.9.0 은 칸을 축마다 따로 정수로 만들었고, 그래서
//      1749×796 패널에서 87×49 직사각 칸이 나왔다. 같은 일그러짐이 도형에도 걸려 정사각형이
//      가로로 늘어나고 원이 납작해졌다 — 사용자가 네 번째로 돌려보낸 그림이 그것이다.
//
// **정사각 픽스처나 나누어떨어지는 픽스처로는 이 절이 실패할 수 없다.** 특히 성질 2 는
// **정사각형이 아닌 패널**에서만 실패할 수 있다. 그래서 사용자가 실제로 본 1749×796 을
// 쓴다: 500×400 캔버스(5:4)와 비율이 크게 다른 2.2:1 상자다.
//
// 0.8.0 까지 `canvasEditArrange.test.ts` 가 `gridPercents` 로 지키던 두 성질(칸 수가
// 정수다 · 스테이지를 늘여도 칸 수가 그대로다)도 여기로 옮겨 왔다 — 그 함수가 하던 일을
// 이제 이 함수가 하기 때문이다.

/** 사용자가 반 칸을 본 그 크기. 두 축 모두 기본 간격으로 나누어떨어지지 않는다. */
const ODD_OUTER: StageSize = { width: 1749, height: 796 };

/** 고를 수 있는 간격 넷 — 목록의 주인은 `canvasEditArrange` 지만 값은 여기서도 지나간다. */
const STEPS = [10, 20, 25, 50] as const;

/** 한 축에 그려질 선 자리들(0 부터 영역 끝 직전까지). */
function lineStops(extent: number, cell: number): number[] {
  const stops: number[] = [];
  for (let x = 0; x < extent - 1e-9; x += cell) stops.push(x);
  return stops;
}

describe('stageLattice — 그려질 선이 온전한 픽셀에 앉는다', () => {
  it('나누어떨어지지 않는 상자(1749x796)에서도 한 칸이 정수 px 다', () => {
    const lattice = stageLattice(ODD_OUTER, CANVAS, 25);
    // 맞추기 전 '정확한' 칸은 가로 1749/20 = 87.45 · 세로 796/16 = 49.75 였다.
    // 축척이 하나이므로 **작은 쪽**(49.75)을 골라 내림한 49 를 두 축에 함께 쓴다.
    expect(lattice.cell).toEqual({ x: 49, y: 49 });
    expect(lattice.stage).toEqual({ width: 49 * 20, height: 49 * 16 });
  });

  it('정사각형이 아닌 패널(1749x796)에서 한 칸이 **정사각형**이다', () => {
    // 0.9.0 은 여기서 87×49 를 냈다 — 칸도 도형도 가로로 늘어난 그림이다.
    // 정사각 패널로는 이 시험이 실패할 수 없으므로 사용자가 본 2.2:1 상자를 쓴다.
    for (const step of STEPS) {
      const lattice = stageLattice(ODD_OUTER, CANVAS, step);
      expect(lattice.cell.x, `${step} 단위`).toBe(lattice.cell.y);
      expect(Number.isInteger(lattice.cell.x), `${step} 단위 정수`).toBe(true);
    }
  });

  it('캔버스 단위의 정사각형이 화면에서도 **정사각형**으로 투영된다', () => {
    // 칸이 정사각형이라는 말과 도형이 일그러지지 않는다는 말은 같은 말이며, 그 근거는
    // 투영이 두 축에 같은 축척을 쓴다는 것 하나다. 그것을 도형으로 직접 확인한다.
    const lattice = stageLattice(ODD_OUTER, CANVAS, 25);
    const proj: CanvasProjection = { stage: lattice.stage, canvas: CANVAS };
    const square = projectBox({ x: 100, y: 100, w: 120, h: 120 }, proj);
    expect(square.w).toBeCloseTo(square.h, 9);
    // 그리고 원도 원으로 남는다(반지름 두 개가 같다).
    const { rx, ry } = ellipseParams(square);
    expect(rx).toBeCloseTo(ry, 9);
  });

  it('그려질 모든 선 자리가 정수이고 간격이 전부 같다', () => {
    for (const step of STEPS) {
      const lattice = stageLattice(ODD_OUTER, CANVAS, step);
      for (const axis of ['x', 'y'] as const) {
        const extent = axis === 'x' ? lattice.stage.width : lattice.stage.height;
        const stops = lineStops(extent, lattice.cell[axis]);
        expect(stops.length, `${step} 단위 ${axis} 선 개수`).toBeGreaterThan(1);
        for (const stop of stops) {
          expect(Number.isInteger(stop), `${step} 단위 ${axis} 선 ${stop}`).toBe(true);
        }
        const gaps = stops.slice(1).map((stop, i) => stop - stops[i]!);
        expect(new Set(gaps).size, `${step} 단위 ${axis} 간격 종류`).toBe(1);
      }
    }
  });

  it('영역은 언제나 바깥 상자 이하다 — 마지막 칸이 상자 밖으로 나가지 않는다', () => {
    for (const step of STEPS) {
      const lattice = stageLattice(ODD_OUTER, CANVAS, step);
      expect(lattice.stage.width).toBeLessThanOrEqual(ODD_OUTER.width);
      expect(lattice.stage.height).toBeLessThanOrEqual(ODD_OUTER.height);
    }
  });

  it('자투리는 양쪽에 나눈 **정수** 자리가 된다 (소수 자리면 안쪽 선이 다시 소수다)', () => {
    const lattice = stageLattice(ODD_OUTER, CANVAS, 25);
    expect(Number.isInteger(lattice.offset.x)).toBe(true);
    expect(Number.isInteger(lattice.offset.y)).toBe(true);
    // 축척이 하나이므로 자투리는 **한 축에 몰린다**(여기서는 가로 769px). 그 여백은
    // 캔버스 비율(5:4)과 패널 비율(2.2:1)이 다르다는 사실이 눈에 보이는 것이며,
    // `fitCanvasSizeToStage` 로 캔버스를 패널에 맞추면 사라진다(아래 절).
    expect(lattice.offset.x).toBe(Math.floor((1749 - 980) / 2));
    expect(lattice.offset.y).toBe(Math.floor((796 - 784) / 2));
  });

  it('멱등이다 — 이미 맞춘 영역을 다시 넣으면 그대로이고 자투리가 0 이다', () => {
    // 표면 밖(오버레이 단독)에서 같은 함수로 칸을 물어볼 수 있는 근거다.
    for (const step of STEPS) {
      const once = stageLattice(ODD_OUTER, CANVAS, step);
      const twice = stageLattice(once.stage, CANVAS, step);
      expect(twice.stage, `${step} 단위`).toEqual(once.stage);
      expect(twice.cell, `${step} 단위`).toEqual(once.cell);
      expect(twice.offset, `${step} 단위`).toEqual({ x: 0, y: 0 });
    }
  });

  it('칸 수는 캔버스 크기 ÷ 간격이다 — 스테이지를 보지 않는다', () => {
    // 0.8.0 이 `gridPercents` 로 지키던 성질이다(칸 수의 주인은 캔버스다).
    for (const outer of [ODD_OUTER, { width: 200, height: 100 }, { width: 1024, height: 768 }]) {
      for (const step of STEPS) {
        const lattice = stageLattice(outer, CANVAS, step);
        expect(lattice.stage.width / lattice.cell.x, `${outer.width} / ${step}`).toBeCloseTo(
          CANVAS.width / step,
          9,
        );
        expect(lattice.stage.height / lattice.cell.y, `${outer.height} / ${step}`).toBeCloseTo(
          CANVAS.height / step,
          9,
        );
      }
    }
  });

  it('비율이 같고 나누어떨어지는 상자는 손대지 않는다 — 자투리도 없다', () => {
    // 1000×800 은 캔버스(500×400)와 **같은 5:4** 이고 한 칸이 50px 로 떨어진다.
    // 0.9.0 은 여기서 800×400(2:1)을 썼는데, 축척이 하나가 된 뒤로 비율이 다른 상자는
    // 정의상 한 축에 여백이 남으므로 "손대지 않는다" 를 물을 수 있는 상자가 아니다.
    const lattice = stageLattice({ width: 1000, height: 800 }, CANVAS, 25);
    expect(lattice.stage).toEqual({ width: 1000, height: 800 });
    expect(lattice.cell).toEqual({ x: 50, y: 50 });
    expect(lattice.offset).toEqual({ x: 0, y: 0 });
  });

  it('비율이 다른 상자는 **긴 축에 여백**을 남긴다 (넘쳐서 잘리지 않는다)', () => {
    // 2:1 상자에 5:4 캔버스 → 세로가 먼저 차고 가로에 여백이 남는다.
    const lattice = stageLattice({ width: 800, height: 400 }, CANVAS, 25);
    expect(lattice.cell).toEqual({ x: 25, y: 25 });
    expect(lattice.stage).toEqual({ width: 500, height: 400 });
    expect(lattice.offset).toEqual({ x: 150, y: 0 });
  });

  it('칸 수가 정수가 아닌 캔버스에서도 온전한 칸의 경계는 정수 px 다', () => {
    // 160 단위 축에 25 간격이면 6.4 칸이다 — 마지막 조각은 원래 조각이고, 그것은
    // 사용자가 고른 두 정수에서 곧바로 따라 나오는 결과다(자투리와 다른 종류의 일).
    const lattice = stageLattice({ width: 100, height: 80 }, { width: 200, height: 160 }, 25);
    expect(lattice.cell).toEqual({ x: 12, y: 12 });
    expect(lattice.stage.width).toBe(96);
    expect(lattice.stage.height).toBeCloseTo(76.8, 9);
  });
});

describe('stageLattice — 퇴화 입력에서도 그림이 사라지지 않는다', () => {
  it('한 칸이 1px 도 되지 않으면 맞추지 않는다 (0 을 곱하면 영역이 사라진다)', () => {
    const lattice = stageLattice({ width: 10, height: 8 }, CANVAS, 25);
    // 정확한 칸은 0.5 · 0.5 px 다 — 내림하면 0 이므로 종전 그대로를 쓴다.
    expect(lattice.stage).toEqual({ width: 10, height: 8 });
    expect(lattice.cell.x).toBeCloseTo(0.5, 9);
    expect(lattice.offset).toEqual({ x: 0, y: 0 });
  });

  it('아직 재지 못한 상자(0)는 0 을 낸다 — NaN 이 아니다', () => {
    const lattice = stageLattice({ width: 0, height: 0 }, CANVAS, 25);
    expect(lattice.stage).toEqual({ width: 0, height: 0 });
    expect(Number.isNaN(lattice.cell.x)).toBe(false);
    expect(Number.isNaN(lattice.cell.y)).toBe(false);
    expect(lattice.offset).toEqual({ x: 0, y: 0 });
  });

  it('간격이 0 이하·비유한이면 맞추지 않는다 (0 으로 나누지 않는다)', () => {
    for (const step of [0, -5, Number.NaN, Number.POSITIVE_INFINITY]) {
      const lattice = stageLattice(ODD_OUTER, CANVAS, step);
      expect(lattice.stage, `${step} 간격`).toEqual({ width: 1749, height: 796 });
    }
  });

  it('캔버스 축이 0·비유한이면 맞추지 않는다', () => {
    expect(stageLattice(ODD_OUTER, { width: 0, height: 400 }, 25).stage.width).toBe(1749);
    expect(stageLattice(ODD_OUTER, { width: 500, height: Number.NaN }, 25).stage.height).toBe(796);
  });

  it('비유한 상자는 0 으로 떨어진다 — 어떤 값도 NaN 으로 새어 나가지 않는다', () => {
    const lattice = stageLattice(
      { width: Number.NaN, height: Number.POSITIVE_INFINITY },
      CANVAS,
      25,
    );
    expect(lattice.stage).toEqual({ width: 0, height: 0 });
    expect(lattice.offset).toEqual({ x: 0, y: 0 });
  });
});

// --- 캔버스 크기를 패널 비율에 맞춘다 (0.10.0) --------------------------
//
// 축척이 하나가 되면 비율이 다른 패널에서 한 축에 여백이 남는다. 그 여백을 없애는 길은
// **캔버스 크기를 패널 비율에 맞추는 것**이지 축척을 둘로 되돌리는 것이 아니다.
//
// 이 함수는 **계산만** 한다. 언제 부를지(= 언제 저장할지)는 부르는 쪽의 몫이며, 그
// 구분이 곧 "리사이즈마다 조용히 다시 맞추지 않는다" 의 근거다(`CanvasPanel` 시험).

describe('fitCanvasSizeToStage — 폭을 붙들고 높이를 유도한다', () => {
  it('사용자가 본 패널(1749x796) 비율로 높이를 낸다 — 폭은 그대로다', () => {
    const fitted = fitCanvasSizeToStage(CANVAS, ODD_OUTER);
    expect(fitted.width).toBe(500); // 폭이 기준축이다
    expect(fitted.height).toBe(Math.round((500 * 796) / 1749)); // 228
  });

  it('맞춘 크기는 그 패널에서 **여백을 한 칸 미만**으로 만든다 (맞춤의 존재 이유)', () => {
    const fitted = fitCanvasSizeToStage(CANVAS, ODD_OUTER);
    const lattice = stageLattice(ODD_OUTER, fitted, 25);
    // 칸은 여전히 정사각형이고,
    expect(lattice.cell.x).toBe(lattice.cell.y);
    // 두 축의 여백이 각각 한 칸보다 작다 — 맞추기 전 가로 여백은 769px 였다.
    expect(ODD_OUTER.width - lattice.stage.width).toBeLessThan(lattice.cell.x);
    expect(ODD_OUTER.height - lattice.stage.height).toBeLessThan(lattice.cell.y);
  });

  it('이미 맞아 있으면 **받은 객체를 그대로** 돌려준다 (쓸모없는 저장을 건너뛰는 신호다)', () => {
    const square: CanvasSize = { width: 400, height: 400 };
    expect(fitCanvasSizeToStage(square, { width: 300, height: 300 })).toBe(square);
  });

  it('잴 수 없는 스테이지에는 손대지 않는다 (첫 ResizeObserver 이전 · 접힌 패널)', () => {
    for (const stage of [
      { width: 0, height: 0 },
      { width: 1749, height: 0 },
      { width: Number.NaN, height: 796 },
      { width: 1749, height: Number.POSITIVE_INFINITY },
    ]) {
      expect(fitCanvasSizeToStage(CANVAS, stage), `${stage.width}x${stage.height}`).toBe(CANVAS);
    }
  });

  it('유도한 높이는 캔버스 축의 범위 안에 죈다 — 파서가 죄는 그 범위와 같다', () => {
    // 극단적으로 납작한 패널: 500 × (1/10000) 은 0.05 → 반올림 0 이라 투영이 무너진다.
    const flat = fitCanvasSizeToStage(CANVAS, { width: 10000, height: 1 });
    expect(flat.height).toBe(MIN_CANVAS_DIMENSION);
    // 극단적으로 긴 패널: 상한을 넘지 않는다.
    const tall = fitCanvasSizeToStage(CANVAS, { width: 1, height: 10000 });
    expect(tall.height).toBe(MAX_CANVAS_DIMENSION);
    // 어느 쪽도 NaN 이 아니며 투영이 성립한다.
    expect(Number.isInteger(flat.height) && Number.isInteger(tall.height)).toBe(true);
  });
});
