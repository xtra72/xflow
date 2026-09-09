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

import {
  MAX_CANVAS_DIMENSION,
  MIN_CANVAS_DIMENSION,
  type CanvasSize,
} from './canvasConfig';
import { stageLattice, type StageSize } from './canvasGeometry';
import { PANEL_REGION_FIT_RATIO, derivedCanvasSize, workspaceBox } from './canvasWorkspace';

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

// --- 크기 유도 (SPEC-CANVAS-006 M8 · REQ-07 · AC-07) ---------------------
//
// 이 절이 지는 성질은 여섯이고(plan.md §M8 (a)), 그 위에 **형상 가드 하나**가 선다.
// 여섯은 전부 "유도는 잰 상자만의 함수다"(불변식 I19)와 "유도는 어떤 표시 상태도 보지
// 않는다"(I20)의 관측 가능한 얼굴이다.
//
// 그리고 이 절은 **걷어낸 화면의 단언을 물려받는다**(acceptance.md AC-07 (AN)). 폭·높이
// 수치 칸이 지키던 셋 — 정수화 · 0/빈 칸 방어 · MIN/MAX 죔 — 가운데 **읽는 쪽**은
// `canvasConfig.test.ts` 의 `parseCanvasSize` 시험이 이미 지고 있고, **쓰는 쪽**을 여기서
// 진다. 칸이 사라졌다고 그 사실들까지 사라지면 회귀 가드가 조용히 하나 줄어든다.

describe('canvasWorkspace — 크기 유도는 항등이다 (REQ-07 · I19 · D7)', () => {
  it('유도값이 잰 바깥 상자 **그 자체**다 — 어떤 축척도 곱하지 않는다', () => {
    // 0.4.0 의 폐기된 규칙(`floor(outer × R)`)이 되살아나면 여기서 1311×597 이 나온다.
    // 그 규칙에서는 캔버스가 영원히 패널의 R 배라 REQ-07 이 제 이름을 지킬 수 없었고,
    // 사용자가 화면을 보고 정확히 그것을 잡아냈다(위험 R21).
    expect(derivedCanvasSize(CANVAS, OUTER)).toEqual({ width: 1749, height: 796 });
    expect(derivedCanvasSize(CANVAS, OUTER)).not.toEqual({
      width: Math.floor(OUTER.width * PANEL_REGION_FIT_RATIO),
      height: Math.floor(OUTER.height * PANEL_REGION_FIT_RATIO),
    });
  });

  it('두 축이 정수이고 파서가 죄는 범위 안이다 — 저장 왕복에 값이 달라지지 않는다', () => {
    // 걷어낸 폭·높이 칸이 `parseCanvasDimension` 으로 지키던 성질을 쓰는 쪽에서 잇는다.
    const derived = derivedCanvasSize(CANVAS, { width: 640.4, height: 480.6 });

    expect(Number.isInteger(derived.width)).toBe(true);
    expect(Number.isInteger(derived.height)).toBe(true);
    expect(derived.width).toBeGreaterThanOrEqual(MIN_CANVAS_DIMENSION);
    expect(derived.height).toBeGreaterThanOrEqual(MIN_CANVAS_DIMENSION);
    // 소수를 그대로 실어 보내면 파서가 반올림해 왕복이 값을 바꾼다.
    expect(derived).toEqual({ width: 640, height: 480 });
  });

  it('상한을 넘는 상자는 상한으로 죈다 — 그때도 값이 진동하지 않는다', () => {
    const huge = { width: 1e9, height: 1e9 };
    const derived = derivedCanvasSize(CANVAS, huge);

    expect(derived).toEqual({ width: MAX_CANVAS_DIMENSION, height: MAX_CANVAS_DIMENSION });
    // 죈 값을 다시 저장값으로 놓고 같은 상자를 물으면 **같은 참조**가 돌아온다 —
    // 죔에 걸려도 두 번째 쓰기가 없다(가정 A18 의 셋째 조건).
    expect(derivedCanvasSize(derived, huge)).toBe(derived);
  });

  it('아직 재지 못한 축(0 · 음수 · 비유한)에는 **받은 객체를 그대로** 돌려준다', () => {
    // "모르면 근사한다" 가 아니라 "모르면 손대지 않는다" — 0 은 파서 범위 밖이라
    // 저장 왕복에 값이 달라진다. 그리고 이 갈래가 **같은 참조**여야 부르는 쪽의
    // 참조 비교가 쓰기를 삼킨다.
    for (const outer of [
      { width: 0, height: 0 },
      { width: 1749, height: 0 },
      { width: 0.4, height: 796 },
      { width: Number.NaN, height: 796 },
      { width: 1749, height: -5 },
    ]) {
      expect(derivedCanvasSize(CANVAS, outer), `${outer.width}x${outer.height}`).toBe(CANVAS);
    }
  });

  it('이미 맞아 있으면 **받은 객체를 그대로** 돌려준다 — 새 비교를 만들지 않는다', () => {
    // `fitCanvasSizeToStage` 의 계약을 그대로 물려받는다. 파서가 매 렌더 새 객체를 내므로
    // 값 비교로는 쓰기가 억제되지 않는다 — 참조 비교여야 한다.
    const fixed = { width: 1749, height: 796 };

    expect(derivedCanvasSize(fixed, OUTER)).toBe(fixed);
  });

  it('같은 상자를 두 번 물으면 같은 값이다 — 유도가 제 결과를 되먹지 않는다 (I19)', () => {
    const first = derivedCanvasSize(CANVAS, OUTER);
    // 첫 결과를 저장값으로 되먹인다(실제 경로가 하는 일 그대로다).
    const second = derivedCanvasSize(first, OUTER);

    expect(second).toBe(first);
  });

  it('저장된 `canvas` 를 아무 값으로 바꿔도 결과가 같다 — 저장값은 입력이 아니다 (I19)', () => {
    // 이 단언이 실패하면 "출력 영역을 그대로 받아 적는" 기각된 안으로 되돌아간 것이며,
    // 그때 리사이즈 한 번이 최대 세 번의 연쇄 쓰기가 된다(위험 R20 · AC-07 (AL)).
    const stored = [
      { width: 500, height: 400 },
      { width: 1, height: 1 },
      { width: 99999, height: 3 },
    ];

    for (const canvas of stored) {
      expect(derivedCanvasSize(canvas, OUTER)).toEqual({ width: 1749, height: 796 });
    }
  });

  it('인자가 `(canvas, outer)` **둘뿐**이다 — 이것이 폐기된 규칙의 형상 가드다 (I20 · R22)', () => {
    // 0.6.0 이 `PANEL_REGION_FIT_RATIO` 를 개명하면 이름 기반 grep 가드는 조용히 무장
    // 해제된다(위험 R23). 그래서 가드를 **형상**으로 옮겨 적는다: 셋째 인자(격자 간격 ·
    // 보기 배율)가 나타나는 순간 저장되지 않는 표시 상태가 저장되는 값을 고치게 되고,
    // 그것이 곧 I20 의 위반이다.
    expect(derivedCanvasSize.length).toBe(2);
  });
});
