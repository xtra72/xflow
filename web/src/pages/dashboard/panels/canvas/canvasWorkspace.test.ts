// 작업 영역 상자 산술 단위 시험 (SPEC-CANVAS-006 M1).
//
// **DOM 무의존 순수 모듈**이라 jsdom 없이 전량 단위 시험한다. 이 파일이 지켜야 하는 것은
// 006 이 새로 더한 함정 넷(acceptance.md §시험 규율 D1~D4)이며, 각 시험의 주석에 **어떤
// 기본값이 그 결함을 감추는지**를 이름으로 적는다. 적어 두지 않으면 다음 사람이 "왜 이런
// 이상한 수를 넣었지" 하고 편한 값으로 갈아 끼우고, 그 순간 시험이 실패할 수 없게 된다.
//
// 고정 입력을 여기 한 번만 적고 이유를 함께 남긴다.
//   - 바깥 상자 **1749 × 796** — 정사각형도 아니고 캔버스 비율(5:4)도 아니며 칸으로
//     나누어떨어지지도 않는다. 사용자가 반 칸을 실제로 본 그 상자다(0.8.0 (O) · 0.9.0 (R) ·
//     0.10.0 (V) 가 각각 정사각형 · 나누어떨어지는 크기 · 같은 비율에 물렸다).
//   - 캔버스 **500 × 400**, 간격 **25**.
//
// 그 조합에서 나오는 수를 미리 적어 둔다(시험이 이 수를 그대로 단언한다).
//   - workspace=false → 칸 49, 영역 980×784, 자리 (384, 6)
//   - workspace=true  → 줄인 상자 1311×597, 칸 37, 영역 740×592, 원점 (504, 102)
//
// **원점 (504, 102) 는 칸 37 의 배수가 아니다**(504 = 37×13 + 23, 102 = 37×2 + 28).
// D3 이 요구하는 조합이 정확히 이것이다 — 원점이 칸의 배수인 고정 입력에서는 격자를
// 작업 영역의 왼쪽 위에 앉힌 결함과 출력 영역의 원점에 앉힌 옳은 구현이 **같은 자리**를
// 내므로 그 시험이 실패할 수 없다.

import { describe, it, expect } from 'vitest';

import type { CanvasSize } from './canvasConfig';
import { stageLattice, type StageSize } from './canvasGeometry';
import { PANEL_REGION_FIT_RATIO, workspaceBox } from './canvasWorkspace';

const OUTER: StageSize = { width: 1749, height: 796 };
const CANVAS: CanvasSize = { width: 500, height: 400 };
const STEP = 25;

describe('canvasWorkspace — 편집이 꺼진 갈래는 오늘과 값이 완전히 같다 (AC-E1 · D1)', () => {
  it('`stageLattice` 결과를 그대로 옮겨 담는다 — 두 갈래를 한 함수가 소유한다', () => {
    const lat = stageLattice(OUTER, CANVAS, STEP);
    const ws = workspaceBox(OUTER, CANVAS, STEP, false);

    // 갈래가 부르는 쪽에 있으면(`if (편집) 이 상자 else 저 상자`) 둘이 각자 자라고 어느 날
    // 한쪽만 고쳐진다. 그래서 꺼진 갈래도 이 함수가 든다.
    expect(ws.box).toEqual(lat.stage);
    expect(ws.stage).toEqual(lat.stage);
    expect(ws.offset).toEqual(lat.offset);
    expect(ws.cell).toEqual(lat.cell);
    expect(ws.origin).toEqual({ x: 0, y: 0 });
  });

  it('그 값이 0.9.0 이 못박은 수 그대로다 (980×784 · 자리 384,6 · 칸 49)', () => {
    const ws = workspaceBox(OUTER, CANVAS, STEP, false);

    expect(ws.stage).toEqual({ width: 980, height: 784 });
    expect(ws.offset).toEqual({ x: 384, y: 6 });
    expect(ws.cell).toEqual({ x: 49, y: 49 });
  });

  it('작업 영역과 출력 영역이 **겹친다** — 상자가 하나 늘었어도 픽셀은 그대로다', () => {
    const ws = workspaceBox(OUTER, CANVAS, STEP, false);

    expect(ws.box).toEqual(ws.stage);
    expect(ws.origin).toEqual({ x: 0, y: 0 });
  });
});

describe('canvasWorkspace — 편집이 켜진 갈래는 상자를 둘로 짓는다 (AC-01 · D1 · D4)', () => {
  it('두 상자의 크기가 **실제로 다르다** — 같으면 이 SPEC 이 아무 일도 하지 않은 것이다', () => {
    // D4: 축소 비율을 시험에서 1.0 으로 갈아 끼우면 원점 · 격자 위상 · 흐림 · 경계가
    // 전부 무의미해진다. 상수를 그대로 쓰고 **다름**을 먼저 단언한 뒤 나머지를 잰다.
    const ws = workspaceBox(OUTER, CANVAS, STEP, true);

    expect(ws.box.width).not.toBe(ws.stage.width);
    expect(ws.box.height).not.toBe(ws.stage.height);
  });

  it('작업 영역은 잰 바깥 상자 **전부**이고 자리는 (0,0) 이다', () => {
    const ws = workspaceBox(OUTER, CANVAS, STEP, true);

    expect(ws.box).toEqual({ width: 1749, height: 796 });
    expect(ws.offset).toEqual({ x: 0, y: 0 });
  });

  it('출력 영역은 **줄인 상자**에 맞춘 격자 결과다 (740×592 · 칸 37)', () => {
    // 줄인 상자는 floor(1749×0.75)=1311 · floor(796×0.75)=597 이고, 그 상자의 '정확한'
    // 칸은 min(1311×25/500, 597×25/400)=37.3125 → 내림 37 이다. 20칸 × 16칸.
    const ws = workspaceBox(OUTER, CANVAS, STEP, true);

    expect(ws.stage).toEqual({ width: 740, height: 592 });
    expect(ws.cell).toEqual({ x: 37, y: 37 });
    expect(ws.stage.width / ws.cell.x).toBe(CANVAS.width / STEP);
    expect(ws.stage.height / ws.cell.y).toBe(CANVAS.height / STEP);
  });

  it('축척은 **하나**다 — 칸이 정사각형이고 출력 영역의 종횡비가 캔버스와 같다 (I6)', () => {
    // 0.10.0 (V) 가 물렸던 기본값은 **비율이 같은 상자**다. 여기 상자는 1749:796 이고
    // 캔버스는 5:4 라 두 축의 '정확한' 칸이 서로 다르다(65.55 vs 37.3125) — 축마다 따로
    // 내림하는 구현은 여기서 87×49 같은 직사각형 칸을 내고 이 단언이 실패한다.
    const ws = workspaceBox(OUTER, CANVAS, STEP, true);

    expect(ws.cell.x).toBe(ws.cell.y);
    expect(ws.stage.width / ws.stage.height).toBeCloseTo(CANVAS.width / CANVAS.height, 12);
  });

  it('출력 영역이 작업 영역 안에서 **가운데**에 서고 원점이 정수 CSS px 다 (AC-01 · R2)', () => {
    const ws = workspaceBox(OUTER, CANVAS, STEP, true);

    expect(ws.origin).toEqual({ x: 504, y: 102 });
    expect(Number.isInteger(ws.origin.x)).toBe(true);
    expect(Number.isInteger(ws.origin.y)).toBe(true);
    // 저술 여백이 사방에 고르다 — 반대쪽 여백과의 차이가 축마다 1px 이내다(내림 때문).
    expect(Math.abs(ws.box.width - ws.stage.width - 2 * ws.origin.x)).toBeLessThanOrEqual(1);
    expect(Math.abs(ws.box.height - ws.stage.height - 2 * ws.origin.y)).toBeLessThanOrEqual(1);
  });

  it('그 원점은 한 칸의 **배수가 아니다** — D3 가 요구하는 고정 입력임을 여기서 못박는다', () => {
    // 504 % 37 = 23, 102 % 37 = 28. 이 사실이 깨지면(예: 축소 비율을 갈아 끼우면)
    // 격자 위상 시험이 실패할 수 없는 픽스처가 되므로, 그 전제를 여기서 지킨다.
    const ws = workspaceBox(OUTER, CANVAS, STEP, true);

    expect(ws.origin.x % ws.cell.x).toBe(23);
    expect(ws.origin.y % ws.cell.y).toBe(28);
  });

  it('출력 영역이 작업 영역을 **넘지 않는다** — 줄이는 방향이 뒤집히지 않는다 (AC-E5)', () => {
    const ws = workspaceBox(OUTER, CANVAS, STEP, true);

    expect(ws.stage.width).toBeLessThanOrEqual(ws.box.width);
    expect(ws.stage.height).toBeLessThanOrEqual(ws.box.height);
    expect(ws.origin.x).toBeGreaterThanOrEqual(0);
    expect(ws.origin.y).toBeGreaterThanOrEqual(0);
  });
});

describe('canvasWorkspace — 축소는 두 축에 같은 비율이다 (가정 A5 · REQ-01)', () => {
  it('비율은 **이름 있는 상수 하나**이며 절대 px 여백이 아니다', () => {
    expect(PANEL_REGION_FIT_RATIO).toBeGreaterThan(0);
    expect(PANEL_REGION_FIT_RATIO).toBeLessThan(1);
  });

  it('바깥 상자를 두 배로 키우면 출력 영역도 (칸 내림 오차 안에서) 같은 비율로 자란다', () => {
    // 절대 px 여백이었다면 상자가 커질수록 여백 비중이 줄어 이 성질이 깨진다. 그리고 그때
    // "패널 비율에 맞춤" 이 조용히 제 일을 못 하게 된다(줄인 상자의 비율 ≠ 바깥 상자의 비율).
    const small = workspaceBox({ width: 800, height: 600 }, CANVAS, STEP, true);
    const large = workspaceBox({ width: 1600, height: 1200 }, CANVAS, STEP, true);

    expect(small.stage.width / small.box.width).toBeCloseTo(large.stage.width / large.box.width, 2);
  });

  it('줄인 상자의 종횡비가 바깥 상자와 같다 — 맞춤 계산이 006 전후로 같은 값을 낸다', () => {
    const ws = workspaceBox(OUTER, CANVAS, STEP, true);
    const reduced = {
      width: Math.floor(OUTER.width * PANEL_REGION_FIT_RATIO),
      height: Math.floor(OUTER.height * PANEL_REGION_FIT_RATIO),
    };

    // 맞춤(`fitCanvasSizeToStage`)은 **바깥** 상자를 쓰지만, 줄인 상자로 계산해도 같은
    // 답이 나온다는 사실이 A5 의 뜻이다(내림 한 픽셀 안에서).
    expect(reduced.width / reduced.height).toBeCloseTo(OUTER.width / OUTER.height, 2);
    expect(ws.stage.width).toBeLessThanOrEqual(reduced.width);
    expect(ws.stage.height).toBeLessThanOrEqual(reduced.height);
  });
});

describe('canvasWorkspace — 퇴화한 크기에서도 NaN 도 예외도 없다 (AC-E5)', () => {
  it('아직 재지 못한 상자(0)는 두 갈래 모두 0 이다', () => {
    for (const workspace of [false, true]) {
      const ws = workspaceBox({ width: 0, height: 0 }, CANVAS, STEP, workspace);
      expect(ws.box).toEqual({ width: 0, height: 0 });
      expect(ws.stage).toEqual({ width: 0, height: 0 });
      expect(ws.origin).toEqual({ x: 0, y: 0 });
      expect(ws.offset).toEqual({ x: 0, y: 0 });
    }
  });

  it('한 칸이 1px 도 되지 않으면 정수화하지 않는다 — 0 을 곱하면 그림이 사라진다', () => {
    // `stageLattice` 가 이미 소유한 처리다. 006 이 그 위에 더한 것은 `floor` 두 번뿐이며
    // 그 둘은 NaN 을 만들지 않는다.
    const ws = workspaceBox({ width: 10, height: 8 }, CANVAS, STEP, true);

    expect(ws.cell.x).toBeGreaterThan(0);
    expect(ws.cell.x).toBeLessThan(1);
    expect(ws.stage.width).toBeGreaterThan(0);
    expect(ws.stage.height).toBeGreaterThan(0);
    expect(ws.stage.width).toBeLessThanOrEqual(ws.box.width);
    expect(Number.isInteger(ws.origin.x)).toBe(true);
    expect(Number.isInteger(ws.origin.y)).toBe(true);
  });

  it('비유한 상자도 0 으로 떨어진다 (NaN 이 상자 산술로 번지지 않는다)', () => {
    const ws = workspaceBox({ width: Number.NaN, height: -5 }, CANVAS, STEP, true);

    expect(Number.isFinite(ws.box.width)).toBe(true);
    expect(Number.isFinite(ws.stage.height)).toBe(true);
    expect(ws.box).toEqual({ width: 0, height: 0 });
    expect(ws.stage).toEqual({ width: 0, height: 0 });
  });

  it('간격이 손상되어도(0 · 비유한) 상자가 사라지지 않는다', () => {
    // `stageLattice` 는 맞출 근거가 없으면 잰 상자를 그대로 돌려준다. 켜진 갈래에서는
    // 그 "잰 상자" 가 **줄인 상자**이므로 출력 영역이 여전히 작업 영역보다 작다.
    const ws = workspaceBox(OUTER, CANVAS, 0, true);

    expect(ws.box).toEqual({ width: 1749, height: 796 });
    expect(ws.stage).toEqual({ width: 1311, height: 597 });
    expect(ws.cell).toEqual({ x: 0, y: 0 });
    expect(ws.origin).toEqual({ x: 219, y: 99 });
  });

  it('극단적으로 납작한 패널에서도 축척은 하나이고 도형이 일그러지지 않는다', () => {
    const ws = workspaceBox({ width: 2000, height: 50 }, CANVAS, STEP, true);

    expect(ws.cell.x).toBe(ws.cell.y);
    expect(ws.stage.width).toBeLessThanOrEqual(ws.box.width);
    expect(ws.stage.height).toBeLessThanOrEqual(ws.box.height);
    expect(ws.origin.x).toBeGreaterThanOrEqual(0);
    expect(ws.origin.y).toBeGreaterThanOrEqual(0);
  });
});
