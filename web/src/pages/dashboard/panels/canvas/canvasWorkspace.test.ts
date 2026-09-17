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

import { readFileSync } from 'node:fs';
import { join } from 'node:path';

import { describe, it, expect } from 'vitest';

import {
  MAX_CANVAS_DIMENSION,
  MIN_CANVAS_DIMENSION,
  type CanvasSize,
} from './canvasConfig';
import { CANVAS_GRID_STEP_CHOICES } from './canvasEditArrange';
import { stageLattice, type StageSize } from './canvasGeometry';
import {
  DEFAULT_WORKSPACE_ZOOM,
  MAX_WORKSPACE_ZOOM,
  MIN_WORKSPACE_ZOOM,
  NO_WORKSPACE_PAN,
  WORKSPACE_ZOOM_CHOICES,
  clampWorkspacePan,
  clampWorkspaceZoom,
  derivedCanvasSize,
  workspaceBox,
  workspaceZoomPercent,
} from './canvasWorkspace';

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
    // 0.6.0 개명: 옛 이름 → `DEFAULT_WORKSPACE_ZOOM`. 단언은 이름만 갈릴 뿐 한 글자도
    // 약해지지 않는다(plan.md §M12 가 여는 0.6.0 예외 하나).
    expect(DEFAULT_WORKSPACE_ZOOM).toBeGreaterThan(0);
    expect(DEFAULT_WORKSPACE_ZOOM).toBeLessThan(1);
  });

  it('그 상수는 이제 범위 안의 **기본값**이다 — 하한 이상 상한 이하다 (REQ-09)', () => {
    expect(DEFAULT_WORKSPACE_ZOOM).toBeGreaterThanOrEqual(MIN_WORKSPACE_ZOOM);
    expect(DEFAULT_WORKSPACE_ZOOM).toBeLessThanOrEqual(MAX_WORKSPACE_ZOOM);
    expect(MIN_WORKSPACE_ZOOM).toBeGreaterThan(0);
    // **상한이 1 을 넘지 않는다.** 넘으면 출력 영역이 잰 상자를 넘어 일부가 화면 밖으로
    // 나가는데 그것을 가져올 팬이 없다(불변식 I22 · §기각한 안 — 캔버스 안의 시야 근거 1).
    expect(MAX_WORKSPACE_ZOOM).toBe(1);
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
      width: Math.floor(OUTER.width * DEFAULT_WORKSPACE_ZOOM),
      height: Math.floor(OUTER.height * DEFAULT_WORKSPACE_ZOOM),
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
      width: Math.floor(OUTER.width * DEFAULT_WORKSPACE_ZOOM),
      height: Math.floor(OUTER.height * DEFAULT_WORKSPACE_ZOOM),
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

  it('서명이 `(canvas, outer)` **글자 그대로**다 — 기본값 붙은 셋째 인자도 여기서 걸린다', () => {
    // **`Function.length` 만으로는 이 가드가 절반이다.** 그 값은 기본값이 붙기 **전**의
    // 인자 수만 세므로, `zoom: number = 1` 처럼 기본값을 단 셋째 인자는 `.length` 를 여전히
    // 2 로 남긴다 — 그리고 그것이 배율을 들이는 **가장 자연스러운 형상**이다(바로 위
    // `workspaceBox` 가 정확히 그 모양으로 배율을 받는다). M9 에서 그 구멍을 실제 변이로
    // 확인했다: 기본값 없는 셋째 인자는 아래 `.length` 가 잡았지만 기본값 붙은 셋째 인자는
    // **한 건도 울리지 않았다.**
    //
    // 그래서 형상을 **글자로** 지킨다. 이 가드는 이름이 아니라 서명을 보므로 상수 개명에도
    // 죽지 않는다(위험 R23).
    const source = readFileSync(join(__dirname, 'canvasWorkspace.ts'), 'utf-8');
    const start = source.indexOf('export function derivedCanvasSize');
    expect(start).toBeGreaterThan(0);
    const body = source.slice(start, source.indexOf('\n}', start));

    expect(body).toContain(
      'export function derivedCanvasSize(canvas: CanvasSize, outer: StageSize): CanvasSize {',
    );
    // 그리고 **본문에 어떤 축척을 곱하는 자리도 없다** — 0.4.0 의 폐기된 규칙은 곱셈
    // 하나였고, 인자를 늘리지 않고 상수를 곱해도 같은 부활이다(위험 R21).
    const code = body
      .split('\n')
      .filter((line) => !line.trim().startsWith('//'))
      .join('\n');
    expect(code).not.toContain('*');
  });

  it('인자가 `(canvas, outer)` **둘뿐**이다 — 이것이 폐기된 규칙의 형상 가드다 (I20 · R22)', () => {
    // 0.6.0 이 상수를 개명했고(옛 이름 → `DEFAULT_WORKSPACE_ZOOM`) 그 순간 이름 기반
    // grep 가드는 조용히 무장 해제되었다(위험 R23). 그래서 가드가 **형상**으로 옮겨 와
    // 여기 서 있다: 셋째 인자(격자 간격 · 보기 배율)가 나타나는 순간 저장되지 않는 표시
    // 상태가 저장되는 값을 고치게 되고, 그것이 곧 I20 의 위반이다. 이 가드는 개명에
    // 죽지 않았다 — 그 사실이 M9 에서 실제로 확인된 자리다.
    expect(derivedCanvasSize.length).toBe(2);
  });
});

// --- 보기 배율 (SPEC-CANVAS-006 M9 · REQ-09 · AC-09) ---------------------
//
// 이 절의 고정 입력은 위 상자 시험과 **다르다**. 배율의 수를 재려면 M8 이 만든 고정점
// (`canvas === outer`)에 서야 하기 때문이다 — 편집을 켠 실제 경로에서는 첫 측정이 캔버스를
// 잰 상자로 덮으므로, `canvas ≠ outer` 인 짝으로 적어 둔 수는 실제와 다른 세계의 수다(D7).
// 그래서 여기서는 **캔버스 = 잰 상자 = 1749 × 796** 을 쓴다.
//
// 그 짝에서 나오는 수를 미리 적어 둔다(시험이 이 수를 그대로 단언한다).
//   - z = 0.25(하한) → 줄인 상자 437×199 · 칸 6 · 축척 0.24 · 영역 419.76×191.04 · 원점 (664, 302)
//   - z = 0.50       → 줄인 상자 874×398 · 칸 12 · 축척 0.48 · 영역 839.52×382.08 · 원점 (454, 206)
//   - z = 0.75(기본) → 줄인 상자 1311×597 · 칸 18 · 축척 0.72 · 영역 1259.28×573.12 · 원점 (244, 111)
//   - z = 1.00(상한) → 칸 25 · 축척 **정확히 1** · 영역 1749×796 · 원점 (0, 0)
//
// **D8 이 요구하는 것이 이 표의 존재 이유다.** 기본 배율만 재는 시험은 배율 인자를 통째로
// 무시하는 구현에서도 초록이고, 기본값과 **같은 칸**을 내는 배율(아래 §양자화의 그 넷)을
// 고른 시험도 마찬가지다. 그래서 기본값이 아니면서 **칸 · 축척 · 원점 셋이 모두 다른**
// `z = 0.50` 을 쓴다.

/** 배율 시험의 고정점 짝 — 캔버스가 잰 상자 그 자체다(M8 뒤의 실제 세계). */
const FIXED_CANVAS: CanvasSize = { width: 1749, height: 796 };

describe('canvasWorkspace — 기본 배율에서는 0.5.0 과 한 픽셀도 다르지 않다 (AC-09 (AR))', () => {
  it('배율을 넘기지 않으면 상자 넷이 기본값의 그 수 그대로다', () => {
    const ws = workspaceBox(OUTER, FIXED_CANVAS, STEP, true);

    expect(ws.cell).toEqual({ x: 18, y: 18 });
    expect(ws.cell.x / STEP).toBe(0.72);
    expect(ws.stage).toEqual({ width: 1259.28, height: 573.12 });
    expect(ws.origin).toEqual({ x: 244, y: 111 });
    expect(ws.box).toEqual({ width: 1749, height: 796 });
  });

  it('기본 배율을 **명시해도** 같은 값이다 — 덧붙임의 기본값이 종전 동작이라는 뜻이다', () => {
    expect(workspaceBox(OUTER, FIXED_CANVAS, STEP, true, DEFAULT_WORKSPACE_ZOOM)).toEqual(
      workspaceBox(OUTER, FIXED_CANVAS, STEP, true),
    );
  });

  it('꺼진 갈래는 배율을 **보지 않는다** — 편집이 꺼진 자리에는 시야를 고를 사람이 없다', () => {
    // 배율이 꺼진 갈래로 새면 대시보드의 그림이 저장되지 않는 표시 상태에 매인다(REQ-05).
    const base = workspaceBox(OUTER, CANVAS, STEP, false);
    for (const z of [MIN_WORKSPACE_ZOOM, 0.5, MAX_WORKSPACE_ZOOM]) {
      expect(workspaceBox(OUTER, CANVAS, STEP, false, z)).toEqual(base);
    }
  });
});

describe('canvasWorkspace — 배율이 실제로 시야를 바꾼다 (AC-09 (AS) · D8)', () => {
  it('`z = 0.50` 은 칸 · 축척 · 원점 셋이 **모두** 기본값과 다르다', () => {
    const base = workspaceBox(OUTER, FIXED_CANVAS, STEP, true);
    const ws = workspaceBox(OUTER, FIXED_CANVAS, STEP, true, 0.5);

    expect(ws.cell).toEqual({ x: 12, y: 12 });
    expect(ws.cell.x / STEP).toBe(0.48);
    expect(ws.stage).toEqual({ width: 839.52, height: 382.08 });
    expect(ws.origin).toEqual({ x: 454, y: 206 });
    // 셋이 **모두** 달라야 이 시험이 배선을 재는 것이 된다(D8 — 0.73 같은 값은 같은
    // 칸을 내므로 배선이 끊겨 있어도 초록이다).
    expect(ws.cell.x).not.toBe(base.cell.x);
    expect(ws.stage.width).not.toBe(base.stage.width);
    expect(ws.origin.x).not.toBe(base.origin.x);
  });

  it('저술 여백이 넓어졌다 — 캔버스 단위 작업 영역이 기본값보다 크다', () => {
    const units = (z: number) => {
      const ws = workspaceBox(OUTER, FIXED_CANVAS, STEP, true, z);
      const scale = ws.cell.x / STEP;
      return Math.round(ws.box.width / scale);
    };

    // 기본값 약 2429 → 0.50 에서 약 3644 (spec.md §상자 산술의 표 그대로).
    expect(units(DEFAULT_WORKSPACE_ZOOM)).toBe(2429);
    expect(units(0.5)).toBe(3644);
  });

  it('`z = 1.00` 에서 축척이 **정확히 1** 이고 여백이 0 이다 — 불변식 I21 의 **명시된 예외**다', () => {
    // I21 이 금지하는 것은 **기본 배율에서** 축척 1 을 단언하는 것이다. 배율을 1.00 으로
    // **명시한** 이 자리에서 축척 1 은 산술이 실제로 그러하며, 0.5.0 이 "보기에서만
    // 관측된다" 고 적은 등식(출력 영역 = 캔버스 = 패널 몸통)을 편집기를 떠나지 않고
    // 눈으로 확인하는 자리다.
    const ws = workspaceBox(OUTER, FIXED_CANVAS, STEP, true, MAX_WORKSPACE_ZOOM);

    expect(ws.cell).toEqual({ x: STEP, y: STEP });
    expect(ws.cell.x / STEP).toBe(1);
    expect(ws.stage).toEqual({ width: 1749, height: 796 });
    expect(ws.stage).toEqual(FIXED_CANVAS);
    expect(ws.stage).toEqual(ws.box);
    expect(ws.origin).toEqual({ x: 0, y: 0 });
  });

  it('하한 `z = 0.25` 에서도 그리는 영역이 사라지지 않고 격자가 살아 있다', () => {
    const ws = workspaceBox(OUTER, FIXED_CANVAS, STEP, true, MIN_WORKSPACE_ZOOM);

    expect(ws.cell).toEqual({ x: 6, y: 6 });
    expect(ws.cell.x).toBeGreaterThanOrEqual(1);
    expect(ws.stage).toEqual({ width: 419.76, height: 191.04 });
    expect(ws.origin).toEqual({ x: 664, y: 302 });
    expect(ws.stage.width).toBeGreaterThan(0);
    expect(ws.stage.height).toBeGreaterThan(0);
  });

  it('어떤 배율에서도 축척은 **하나**이고 `origin` 과 `cell` 은 **정수**다 (I5 · I6)', () => {
    for (const z of [MIN_WORKSPACE_ZOOM, 0.4, 0.5, DEFAULT_WORKSPACE_ZOOM, 0.9, MAX_WORKSPACE_ZOOM]) {
      const ws = workspaceBox(OUTER, FIXED_CANVAS, STEP, true, z);
      expect(ws.cell.x, `z=${z}`).toBe(ws.cell.y);
      expect(Number.isInteger(ws.cell.x), `z=${z}`).toBe(true);
      expect(Number.isInteger(ws.origin.x), `z=${z}`).toBe(true);
      expect(Number.isInteger(ws.origin.y), `z=${z}`).toBe(true);
      // 레터박스 0 — 출력 영역이 잰 상자와 닮았다.
      expect(ws.stage.width / ws.box.width, `z=${z}`).toBeCloseTo(
        ws.stage.height / ws.box.height,
        12,
      );
    }
  });
});

describe('canvasWorkspace — 배율은 담기는 값만 취한다 (AC-09 (AT) · 불변식 I22)', () => {
  it('범위 안의 어떤 배율에서도 `stage ≤ box === outer` 이고 `origin ≥ 0` 이다', () => {
    // **이 시험이 상한의 가드다.** `MAX_WORKSPACE_ZOOM` 을 1 보다 크게 하면 줄인 상자가
    // 잰 상자를 넘어 `origin` 이 음수가 되고 여기서 걸린다 — 그때 출력 영역 일부가
    // 컨테이너의 `overflow-hidden` 에 잘리는데 그것을 가져올 팬이 없다(가정 A8).
    for (const z of [MIN_WORKSPACE_ZOOM, 0.33, 0.5, DEFAULT_WORKSPACE_ZOOM, MAX_WORKSPACE_ZOOM]) {
      const ws = workspaceBox(OUTER, FIXED_CANVAS, STEP, true, z);
      expect(ws.box, `z=${z}`).toEqual({ width: OUTER.width, height: OUTER.height });
      expect(ws.stage.width, `z=${z}`).toBeLessThanOrEqual(ws.box.width);
      expect(ws.stage.height, `z=${z}`).toBeLessThanOrEqual(ws.box.height);
      expect(ws.origin.x, `z=${z}`).toBeGreaterThanOrEqual(0);
      expect(ws.origin.y, `z=${z}`).toBeGreaterThanOrEqual(0);
    }
  });

  it('제안값 넷이 모두 범위 안이고 하한 · 기본값 · 상한을 포함한다', () => {
    for (const choice of WORKSPACE_ZOOM_CHOICES) {
      expect(choice).toBeGreaterThanOrEqual(MIN_WORKSPACE_ZOOM);
      expect(choice).toBeLessThanOrEqual(MAX_WORKSPACE_ZOOM);
    }
    expect(WORKSPACE_ZOOM_CHOICES).toContain(MIN_WORKSPACE_ZOOM);
    expect(WORKSPACE_ZOOM_CHOICES).toContain(DEFAULT_WORKSPACE_ZOOM);
    expect(WORKSPACE_ZOOM_CHOICES).toContain(MAX_WORKSPACE_ZOOM);
  });
});

describe('canvasWorkspace — 격자 간격이 배율을 양자화한다 (가정 A20 · 위험 R24)', () => {
  it('이웃한 네 배율이 **같은 그림**을 낸다 — 숨기지 않고 수로 적는다', () => {
    // 실현 가능한 축척은 사실상 `1/step` 눈금이라 이웃한 백분율이 같은 칸을 낸다. 그
    // `floor` 는 걷어내지 않는다 — 사용자가 세 번 되돌려보낸 "격자가 일정하지 않음" 을
    // 막고 있는 것이 그 한 줄이다(불변식 I5). 화면은 이 대가를 상시 도움말로 말한다.
    //
    // **spec.md 가정 A20 이 적은 넷(0.72·0.73·0.74·0.75)은 이 고정 입력에서 틀렸다.**
    // A20 은 `cell ≈ floor(z × step)` 로 어림했으나 실제로는 `reduced = floor(outer × z)`
    // 가 축마다 최대 1px 을 먼저 잃으므로 축척이 그 어림값보다 조금 낮다. `z = 0.72` 는
    // 경계에 정확히 앉아 아래로 떨어진다 — 아래 시험이 그 사실을 수로 못박는다.
    const base = workspaceBox(OUTER, FIXED_CANVAS, STEP, true);
    for (const z of [0.73, 0.74, 0.75, 0.76]) {
      expect(workspaceBox(OUTER, FIXED_CANVAS, STEP, true, z), `z=${z}`).toEqual(base);
    }
  });

  it('`z = 0.72` 는 그 넷에 들지 않는다 — 칸이 17 이라 A20 의 어림이 여기서 깨진다', () => {
    const ws = workspaceBox(OUTER, FIXED_CANVAS, STEP, true, 0.72);

    expect(ws.cell).toEqual({ x: 17, y: 17 });
    expect(ws.cell.x).not.toBe(18);
  });

  it('눈금을 넘어서면 그림이 실제로 달라진다 — 양자화는 먹통이 아니라 눈금이다', () => {
    const base = workspaceBox(OUTER, FIXED_CANVAS, STEP, true);
    const beyond = workspaceBox(OUTER, FIXED_CANVAS, STEP, true, 0.77);

    expect(beyond.cell).toEqual({ x: 19, y: 19 });
    expect(beyond.cell.x).not.toBe(base.cell.x);
  });

  it('하한에서도 도크가 제안하는 네 간격은 칸을 살려 둔다 — 하한을 그렇게 골랐다', () => {
    // 근거를 수로 적는다: 간격 10 · 20 · 25 · 50 에서 칸이 각각 2 · 4 · 6 · 12 다.
    const cells = CANVAS_GRID_STEP_CHOICES.map(
      (step) => workspaceBox(OUTER, FIXED_CANVAS, step, true, MIN_WORKSPACE_ZOOM).cell.x,
    );
    expect(cells).toEqual([2, 4, 6, 12]);
    for (const cell of cells) expect(cell).toBeGreaterThanOrEqual(1);
  });

  it('**"격자는 언제나 그려진다" 를 주장하지 않는다** — 아주 작은 간격에서는 범위 안에서도 꺼진다', () => {
    // spec.md 가정 A20 은 하한의 근거를 "`step ≥ 4` 인 모든 간격에서 `cell ≥ 1`" 로 적었는데,
    // 그 경계는 이 고정 입력에서 **아슬아슬하게 거짓**이다 — `reduced` 의 `floor` 가 먼저
    // 1px 을 잃으므로 `step = 4` · `z = 0.25` 에서 칸이 0.9994 로 1 에 미치지 못한다.
    // 실제로 지켜지는 것은 **도크가 제안하는 네 간격**이며(위 시험), 그 위의 값에서도
    // 여유가 있다. 이 갈래의 주인은 배율이 아니라 `stageLattice` 와 오버레이의 `cell ≥ 1`
    // 게이트이고, 그때도 그림은 사라지지 않는다.
    const ws = workspaceBox(OUTER, FIXED_CANVAS, 4, true, MIN_WORKSPACE_ZOOM);

    expect(ws.cell.x).toBeLessThan(1);
    expect(ws.cell.x).toBeGreaterThan(0);
    expect(ws.stage.width).toBeGreaterThan(0);
    expect(ws.origin.x).toBeGreaterThanOrEqual(0);
    expect(Number.isInteger(ws.origin.x)).toBe(true);
  });
});

describe('canvasWorkspace — 백분율과 분수가 만나는 자리 (AC-09 (AT) · 불변식 I4)', () => {
  it('백분율 정수를 분수로 옮기고 범위로 죈다', () => {
    expect(clampWorkspaceZoom(50, DEFAULT_WORKSPACE_ZOOM)).toBe(0.5);
    expect(clampWorkspaceZoom('75', DEFAULT_WORKSPACE_ZOOM)).toBe(0.75);
    expect(clampWorkspaceZoom(100, DEFAULT_WORKSPACE_ZOOM)).toBe(MAX_WORKSPACE_ZOOM);
    // 범위 밖은 죈다 — 확대는 없고(상한 1.00), 격자를 죽이는 쪽으로도 내려가지 않는다.
    expect(clampWorkspaceZoom(400, DEFAULT_WORKSPACE_ZOOM)).toBe(MAX_WORKSPACE_ZOOM);
    expect(clampWorkspaceZoom(1, DEFAULT_WORKSPACE_ZOOM)).toBe(MIN_WORKSPACE_ZOOM);
    expect(clampWorkspaceZoom(-30, DEFAULT_WORKSPACE_ZOOM)).toBe(MIN_WORKSPACE_ZOOM);
  });

  it('읽을 수 없는 입력에는 **지금 값을 그대로** 돌려준다 — 한 글자를 지우는 동안 무너지지 않는다', () => {
    for (const bad of ['', '   ', 'abc', '7%', Number.NaN, Number.POSITIVE_INFINITY]) {
      expect(clampWorkspaceZoom(bad, 0.5), String(bad)).toBe(0.5);
    }
  });

  it('**적힌 값을 실현 가능한 축척으로 되죄지 않는다** — 간격이 배율을 조용히 고치지 못한다', () => {
    // 되죄면 격자 간격이 배율을 고치게 되고, 그것은 I20 이 유도에 대해 금지한 결합의
    // 거울상이다(위험 R24). 73% 는 간격 25 에서 기본값과 같은 그림을 내지만 **값 자체는
    // 73% 로 남는다** — 화면이 74 나 72 로 고쳐 적지 않는다.
    expect(clampWorkspaceZoom(73, DEFAULT_WORKSPACE_ZOOM)).toBe(0.73);
  });

  it('분수를 백분율 정수로 되옮기는 자리도 이 모듈 하나다', () => {
    expect(workspaceZoomPercent(DEFAULT_WORKSPACE_ZOOM)).toBe(75);
    expect(workspaceZoomPercent(MIN_WORKSPACE_ZOOM)).toBe(25);
    expect(workspaceZoomPercent(MAX_WORKSPACE_ZOOM)).toBe(100);
    // 왕복이 제안값 넷에서 항등이다 — 화면이 적은 값을 되읽어도 값이 흔들리지 않는다.
    for (const choice of WORKSPACE_ZOOM_CHOICES) {
      expect(clampWorkspaceZoom(workspaceZoomPercent(choice), DEFAULT_WORKSPACE_ZOOM)).toBe(choice);
    }
  });
});

describe('canvasWorkspace — 배율은 크기 유도에 닿지 않는다 (AC-09 (AU) · I20 · R22)', () => {
  it('배율을 아무 값으로 바꿔도 유도한 캔버스 크기가 한 글자도 달라지지 않는다', () => {
    // 형상 자체로 성립한다 — `derivedCanvasSize` 는 배율을 **인자로도 받지 않는다**.
    // 그럼에도 이 자리를 재는 것은, 배선이 언젠가 배율을 유도로 흘려보내려 할 때
    // 그 시도가 어디서 걸려야 하는지를 이름으로 적어 두기 위해서다.
    for (const z of [MIN_WORKSPACE_ZOOM, 0.5, DEFAULT_WORKSPACE_ZOOM, MAX_WORKSPACE_ZOOM]) {
      const ws = workspaceBox(OUTER, FIXED_CANVAS, STEP, true, z);
      expect(derivedCanvasSize(CANVAS, { width: ws.box.width, height: ws.box.height })).toEqual({
        width: 1749,
        height: 796,
      });
    }
  });
});

// --- 보기 팬 (사용자 신고 2026-09-16 · 006 영역) -------------------------
//
// 고정 입력은 위 배율 절의 그것을 그대로 쓴다(`FIXED_CANVAS` — 캔버스 = 잰 상자). 팬이
// 답하는 물음이 **"캔버스 밖에 놓은 것에 닿을 수 있는가"** 인데, 그 물음은 캔버스 단위로만
// 물을 수 있고 그 환산에는 축척이 필요하기 때문이다.

/**
 * 작업 영역이 실제로 보여 주는 **캔버스 좌표** 범위(가로).
 *
 * 상자 밖은 표면 컨테이너의 `overflow-hidden` 에 잘리므로, 이 범위 밖의 요소는 저술은
 * 되지만 **화면에 없다.** 시험이 "닿는다/닿지 않는다" 를 재는 자리가 여기 하나다.
 */
function visibleCanvasX(
  ws: ReturnType<typeof workspaceBox>,
  canvas: CanvasSize,
): { from: number; to: number } {
  const scale = ws.stage.width / canvas.width;
  return { from: -ws.origin.x / scale, to: (ws.box.width - ws.origin.x) / scale };
}

describe('canvasWorkspace — 팬이 출력 영역 밖을 가져온다', () => {
  it('배율 1 에서 캔버스 왼쪽 밖 100 단위가 화면에 **든다** — 팬이 없으면 그 자리는 잘린 채다', () => {
    // **이 시험이 이 기능의 존재 이유 전부다.** 배율 1 에서는 출력 영역이 잰 상자를 가득
    // 채우므로 저술 여백이 **0** 이고, 좌표를 죄지 않는 파서가 허용한 캔버스 밖 저술
    // (가정 A5)에 닿을 길이 한 뼘도 없다. 팬을 넘기지 않은 아래 형제가 그 사실을 잰다.
    const panned = workspaceBox(OUTER, FIXED_CANVAS, STEP, true, MAX_WORKSPACE_ZOOM, {
      x: 300,
      y: 0,
    });

    expect(visibleCanvasX(panned, FIXED_CANVAS).from).toBeLessThanOrEqual(-100);
  });

  it('같은 배율에서 팬이 쉬면 캔버스 **딱 그만큼**만 보인다 — 고친 것이 무엇인지 여기서 갈린다', () => {
    const still = workspaceBox(OUTER, FIXED_CANVAS, STEP, true, MAX_WORKSPACE_ZOOM);

    // 왼쪽 밖 1 단위조차 들어오지 않는다. 이 수가 0 이 아닌 값으로 바뀌면 그때는 팬이
    // 아니라 **배율이나 상자**가 달라진 것이다.
    // `-0` 과 `0` 을 가르지 않는다 — 원점이 0 이면 나눗셈이 `-0` 을 내는데, 그 부호는
    // 이 시험이 묻는 것("캔버스 밖이 한 뼘도 보이지 않는다")과 아무 상관이 없다.
    const seen = visibleCanvasX(still, FIXED_CANVAS);
    expect(seen.from + 0).toBe(0);
    expect(seen.to).toBe(FIXED_CANVAS.width);
  });

  it('팬을 넘기지 않은 호출은 상자 넷이 **한 픽셀도** 다르지 않다 (덧붙임의 기본값이 종전 동작이다)', () => {
    for (const z of [MIN_WORKSPACE_ZOOM, 0.5, DEFAULT_WORKSPACE_ZOOM, MAX_WORKSPACE_ZOOM]) {
      expect(workspaceBox(OUTER, FIXED_CANVAS, STEP, true, z, NO_WORKSPACE_PAN), `z=${z}`).toEqual(
        workspaceBox(OUTER, FIXED_CANVAS, STEP, true, z),
      );
    }
  });

  it('꺼진 갈래는 팬을 **보지 않는다** — 편집이 꺼진 자리에는 시야를 고를 사람이 없다', () => {
    const panned = workspaceBox(OUTER, CANVAS, STEP, false, DEFAULT_WORKSPACE_ZOOM, {
      x: 400,
      y: -200,
    });

    // 배율에 대해 세운 그 성질 그대로다 — 값이 0.9.0 이 못박은 수 그대로여야 한다.
    expect(panned.origin).toEqual({ x: 0, y: 0 });
    expect(panned.stage).toEqual({ width: 980, height: 784 });
    expect(panned.box).toEqual(panned.stage);
  });

  it('팬이 붙어도 작업 영역은 여전히 잰 상자 **그 자체**다 (불변식 I22)', () => {
    // 팬을 상자를 키워 구현하면 이 시험이 걸린다 — 키운 자리는 컨테이너의
    // `overflow-hidden` 이 그대로 잘라 내므로 그 구현은 아무것도 가져오지 못한다.
    for (const pan of [
      { x: 0, y: 0 },
      { x: 300, y: -120 },
      { x: -874, y: 398 },
    ]) {
      const ws = workspaceBox(OUTER, FIXED_CANVAS, STEP, true, 0.5, clampWorkspacePan(pan, OUTER));
      expect(ws.box, JSON.stringify(pan)).toEqual({ width: OUTER.width, height: OUTER.height });
      expect(ws.offset, JSON.stringify(pan)).toEqual({ x: 0, y: 0 });
    }
  });

  it('팬이 축척을 바꾸지 않는다 — 칸도 출력 영역도 팬과 무관하다', () => {
    // 팬이 `stageLattice` 에 닿으면(줄인 상자를 팬으로 고치면) 시야가 아니라 **배율**이
    // 달라진다. 그것은 팬이 아니라 조용한 배율이고, 격자가 함께 흔들린다.
    const still = workspaceBox(OUTER, FIXED_CANVAS, STEP, true, 0.5);
    const moved = workspaceBox(OUTER, FIXED_CANVAS, STEP, true, 0.5, { x: 217, y: -99 });

    expect(moved.cell).toEqual(still.cell);
    expect(moved.stage).toEqual(still.stage);
    // 옮긴 것은 자리 하나뿐이고, 그 자리는 정확히 팬만큼 옮겨졌다.
    expect(moved.origin).toEqual({ x: still.origin.x + 217, y: still.origin.y - 99 });
  });

  it('어떤 팬에서도 원점이 **정수**다 — 팬은 `floor` 바깥이 아니라 안쪽에 들어간다 (불변식 I5)', () => {
    // 소수 팬이 실제로 일어난다 — 포인터 좌표는 소수일 수 있고 `clientX` 의 차이도 그렇다.
    // 원점이 소수가 되면 격자선이 두 장치 픽셀에 걸치고, 그것이 사용자가 세 번 돌려보낸
    // "격자가 일정하지 않음" 의 네 번째 얼굴이다.
    for (const pan of [
      { x: 0.5, y: -0.5 },
      { x: 12.25, y: 99.75 },
      { x: -7.125, y: -0.001 },
    ]) {
      const ws = workspaceBox(OUTER, FIXED_CANVAS, STEP, true, DEFAULT_WORKSPACE_ZOOM, pan);
      expect(Number.isInteger(ws.origin.x), JSON.stringify(pan)).toBe(true);
      expect(Number.isInteger(ws.origin.y), JSON.stringify(pan)).toBe(true);
    }
  });
});

describe('canvasWorkspace — 팬의 범위는 출력 영역의 중심이 정한다 (`clampWorkspacePan`)', () => {
  /** 상자 좌표에서 출력 영역의 중심. 죔이 지키는 것이 이 점 하나다. */
  const stageCentre = (ws: ReturnType<typeof workspaceBox>) => ({
    x: ws.origin.x + ws.stage.width / 2,
    y: ws.origin.y + ws.stage.height / 2,
  });

  it('상한은 상자의 절반이다 — 넘겨 적은 값이 그 자리에서 멈춘다', () => {
    expect(clampWorkspacePan({ x: 99999, y: 99999 }, OUTER)).toEqual({
      x: OUTER.width / 2,
      y: OUTER.height / 2,
    });
    expect(clampWorkspacePan({ x: -99999, y: -99999 }, OUTER)).toEqual({
      x: -OUTER.width / 2,
      y: -OUTER.height / 2,
    });
  });

  it('범위 안의 값은 **그대로** 지난다 — 죔이 손짓을 몰래 고치지 않는다', () => {
    expect(clampWorkspacePan({ x: 300, y: -120 }, OUTER)).toEqual({ x: 300, y: -120 });
  });

  it('**상한이 배율과 무관하다** — 배율을 바꿔도 팬이 범위를 벗어나 튀지 않는다', () => {
    // `previewPan` 은 줌을 낮출 때마다 옛 이동량을 되죄어야 한다(상한이 줌에 딸려 있다).
    // 여기서 `stage` 가 식에서 사라지는 것이 그 일을 통째로 없앤다.
    const far = clampWorkspacePan({ x: 874, y: 398 }, OUTER);
    for (const z of [MIN_WORKSPACE_ZOOM, 0.5, DEFAULT_WORKSPACE_ZOOM, MAX_WORKSPACE_ZOOM]) {
      const ws = workspaceBox(OUTER, FIXED_CANVAS, STEP, true, z, far);
      const centre = stageCentre(ws);
      // 중심이 상자 안이다 — `floor` 가 원점을 최대 1px 내리므로 그만큼의 여유를 둔다.
      expect(centre.x, `z=${z}`).toBeGreaterThanOrEqual(-1);
      expect(centre.x, `z=${z}`).toBeLessThanOrEqual(ws.box.width);
      expect(centre.y, `z=${z}`).toBeGreaterThanOrEqual(-1);
      expect(centre.y, `z=${z}`).toBeLessThanOrEqual(ws.box.height);
    }
  });

  it('상한에서도 출력 영역의 **절반이 남는다** — 어디를 보고 있는지 잃지 않는다', () => {
    const ws = workspaceBox(
      OUTER,
      FIXED_CANVAS,
      STEP,
      true,
      MAX_WORKSPACE_ZOOM,
      clampWorkspacePan({ x: 99999, y: 0 }, OUTER),
    );

    // 상자 안에 남은 출력 영역의 폭 = `box.width - origin.x` 이고, 그것이 절반 이상이다.
    expect(ws.box.width - ws.origin.x).toBeGreaterThanOrEqual(ws.stage.width / 2 - 1);
  });

  it('잴 수 없는 값과 퇴화한 상자는 **쉬는 자리**로 떨어진다 (NaN 이 상자 산술로 번지지 않는다)', () => {
    expect(clampWorkspacePan({ x: Number.NaN, y: Number.POSITIVE_INFINITY }, OUTER)).toEqual({
      x: 0,
      y: 0,
    });
    // 아직 재지 못한 상자(0)에는 옮길 곳이 없다 — `previewPan.axisBound` 가 0 을 그렇게 읽는다.
    expect(clampWorkspacePan({ x: 300, y: 300 }, { width: 0, height: 0 })).toEqual({ x: 0, y: 0 });
    expect(
      clampWorkspacePan({ x: 300, y: 300 }, { width: Number.NaN, height: -10 }),
    ).toEqual({ x: 0, y: 0 });
  });

  it('배율을 낮추면 **더 멀리** 닿는다 — 팬과 배율이 같은 방향으로 겹친다', () => {
    const reach = (z: number) => {
      const ws = workspaceBox(OUTER, FIXED_CANVAS, STEP, true, z, clampWorkspacePan({ x: 99999, y: 0 }, OUTER));
      return visibleCanvasX(ws, FIXED_CANVAS).from;
    };

    // 상한까지 민 상태에서 왼쪽으로 닿는 거리. 배율이 낮을수록 캔버스 단위로 더 멀다.
    expect(reach(0.5)).toBeLessThan(reach(MAX_WORKSPACE_ZOOM));
    expect(reach(MIN_WORKSPACE_ZOOM)).toBeLessThan(reach(0.5));
    // 어느 배율에서도 캔버스 **반 변**은 넘어간다(상한이 `box/2` 이고 축척이 1 이하다).
    // `floor` 가 원점을 최대 1px 내리므로 그 한 칸만큼 여유를 둔다 — 여유를 두지 않으면
    // 상자 폭이 홀수일 때만 빨개지는 시험이 되고, 그 빨강은 결함이 아니라 반올림이다.
    expect(reach(MAX_WORKSPACE_ZOOM)).toBeLessThanOrEqual(-FIXED_CANVAS.width / 2 + 1);
  });
});
