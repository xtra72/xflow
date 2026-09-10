// 캔버스 기하 순수 함수 (SPEC-CANVAS-001 T2).
//
// 정수 캔버스 좌표 → CSS px 투영, 도형별 경로 수치, 텍스트 정렬 기준점,
// devicePixelRatio 백킹 버퍼 산술을 담는다. **DOM 무의존**이다 — `document`/`window`/
// `CanvasRenderingContext2D` 를 참조하지 않으며, 글자 폭은 `measureText` 를 부르지 않고
// **숫자 인자로 받는다**. 재는 쪽(렌더 층)과 계산하는 쪽(이 모듈)을 가르는 것이 이 분리의
// 전부다 — 그래야 정렬·투영 계약을 jsdom 없이 단위 테스트로 고정할 수 있다.
//
// ## 투영에는 두 크기가 모두 필요하다 (SPEC-CANVAS-002 0.8.0)
//
// 좌표가 정규화 분수였을 때 투영은 "스테이지 크기를 곱한다" 한 줄이었고, 그래서 스테이지
// 하나만 있으면 됐다. 정수 캔버스 좌표는 **제 좌표계의 크기**를 알아야 화면 자리를 안다:
// 축마다 `스테이지 px / 캔버스 단위` 를 곱한다. 그래서 이 모듈의 모든 투영은 두 크기를
// 한 묶음(`CanvasProjection`)으로 받는다 — 인자를 둘로 늘어놓지 않고 묶는 것에 뜻이 있다.
// 묶여 있으면 **한쪽만 들고 투영하는 것이 형상 자체로 불가능**하고, 그것이 SPEC-CANVAS-002
// 위험 R1("측정원이 둘이 되면 핸들이 도형에서 미끄러진다")에 대한 이 파일의 답이다.
//
// 좌표를 캔버스 안으로 **clamp 하지 않는다** — 캔버스 밖으로 일부 걸치는 배치도 뜻이 있는
// 저술이며(canvasConfig 의 기하 손상 정책과 같은 이유), 잘라내면 사용자 의도가 조용히 바뀐다.
//
// ## 그리는 영역은 격자 칸의 정수배다 (0.9.0)
//
// 투영의 분자인 스테이지가 **아무 값이나 될 수 있으면** 격자 한 칸은 소수 px 가 되고, 소수
// 자리에서 시작하는 1px 선은 두 픽셀에 걸쳐 칠해져 선마다 굵기가 달라 보인다. 그래서 잰
// 상자를 그대로 투영에 쓰지 않고 `stageLattice` 로 한 번 **칸에 맞춘 뒤** 그 결과를
// 스테이지로 삼는다(아래 §그리는 영역의 격자 정렬). 투영 · 붙임 · 격자가 모두 같은 영역을
// 쓰므로, 화면과 계산이 갈라질 자리는 여기서도 생기지 않는다.
//
// 백킹 버퍼 산술은 `HeatmapCanvas.tsx` 의 DPR 규율(표시 크기 × dpr 을 정수 device px 로
// 반올림)에서 순수 부분만 뽑아낸 것이다.
//
// SPEC-CANVAS-003(값 구동 애니메이션)을 위한 형상 제약: 이 모듈의 입력은 모두 **평범한
// 수치 레코드**이고 출력도 그렇다. 003 은 `BoxGeometry`/`LineGeometry` 의 수치를 보간한 뒤
// 같은 함수에 넣으면 되므로 이 API 를 다시 짤 필요가 없다.
//
// @spec SPEC-CANVAS-001

import { MAX_CANVAS_DIMENSION, MIN_CANVAS_DIMENSION } from './canvasConfig';
import { PATH_LOCAL_EXTENT, type PathCommand } from './shapes/pathTypes';
import type {
  BoxGeometry,
  CanvasElement,
  CanvasSize,
  ElementAlign,
  LineGeometry,
  PointGeometry,
} from './canvasConfig';

// --- 타입 ---------------------------------------------------------------

/**
 * 스테이지의 CSS px 크기. 히트맵 `StageBox`(left/top 을 더 가진다)도 구조적으로 이 자리에
 * 그대로 들어가므로, 캔버스가 나중에 종횡비 스테이지를 쓰게 돼도 호출부만 바뀐다.
 */
export interface StageSize {
  width: number;
  height: number;
}

/**
 * 투영 한 벌 — **잰 스테이지 크기와 저술된 캔버스 크기**.
 *
 * 둘을 묶어 다니는 것이 이 타입의 존재 이유다(파일 머리말 §투영에는 두 크기가 모두 필요하다).
 * 어느 한쪽만으로는 캔버스 단위와 화면 px 사이를 오갈 수 없으므로, 묶어 두면 "스테이지만
 * 들고 투영" 같은 반쪽 호출이 컴파일되지 않는다.
 */
export interface CanvasProjection {
  /** 표면의 `ResizeObserver` 가 잰 스테이지 CSS px 크기. */
  stage: StageSize;
  /** config 가 정한 캔버스 좌표계 크기(정수 단위). */
  canvas: CanvasSize;
}

/** 캔버스 단위 공간의 한 점. `PointGeometry` 와 형상은 같고 뜻이 다르다(자리 vs 기하). */
export interface CanvasPoint {
  x: number;
  y: number;
}

/** 캔버스 단위 공간의 이동량. */
export interface CanvasDelta {
  dx: number;
  dy: number;
}

/** 캔버스 단위 공간의 상자(좌상단 + 크기). */
export interface CanvasBox {
  x: number;
  y: number;
  w: number;
  h: number;
}

/** 백킹 버퍼 크기(정수 device px)와 그때 적용된 배율. */
export interface BackingSize {
  width: number;
  height: number;
  scale: number;
}

/** 투영된 사각형(CSS px). 정규화 `BoxGeometry` 와 형상은 같고 단위가 다르다. */
export interface PxBox {
  x: number;
  y: number;
  w: number;
  h: number;
}

/** 투영된 선분(CSS px). */
export interface PxLine {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
}

/** 투영된 점(CSS px). */
export interface PxPoint {
  x: number;
  y: number;
}

/**
 * 투영된 경로 명령(CSS px). `PathCommand` 와 **형상은 같고 단위가 다르다** —
 * 저쪽은 요소 상자 로컬 정수이고 이쪽은 스테이지 px 다(`BoxGeometry` 와 `PxBox` 의 관계
 * 그대로다). 이름을 갈라 두면 로컬 좌표를 그대로 `ctx` 에 넘기는 실수가 타입에서 걸린다.
 */
export type PxPathCommand =
  | { c: 'M'; x: number; y: number }
  | { c: 'L'; x: number; y: number }
  | { c: 'C'; x1: number; y1: number; x2: number; y2: number; x: number; y: number }
  | { c: 'Z' };

/** `ctx.ellipse()` 인자 형태 — 중심 + 반지름. */
export interface EllipseParams {
  cx: number;
  cy: number;
  rx: number;
  ry: number;
}

// --- 상수 ---------------------------------------------------------------

/**
 * 허용하는 devicePixelRatio 상한. 이보다 큰 값은 손상된 입력으로 보고 1 로 떨어뜨린다 —
 * 백킹 버퍼는 배율의 **제곱**으로 메모리를 먹으므로, 잘못된 큰 배율 하나가 탭을 죽인다.
 * 실물 상한은 3(모바일 고DPI) 근처라 8 은 넉넉한 여유다.
 */
export const MAX_DEVICE_PIXEL_RATIO = 8;

// --- 수치 보정 도우미 ---------------------------------------------------

/** 유한 숫자면 그대로, 아니면 0. 비유한 좌표가 NaN 으로 번지는 것을 막는다. */
function finite(v: number): number {
  return Number.isFinite(v) ? v : 0;
}

/** 유한 양수면 그대로, 아니면 0. 크기(스테이지·표시 영역)에 쓴다. */
function positiveOrZero(v: number): number {
  return Number.isFinite(v) && v > 0 ? v : 0;
}

/**
 * 캔버스 단위 한 축을 화면 px 로 투영한다 — `좌표 / 캔버스 길이 × 스테이지 길이`.
 *
 * 캔버스 축이 0 이면 나눌 수 없으므로 0 을 돌려준다. NaN 을 흘리면 그 프레임의 경로 전체가
 * 조용히 사라져 원인을 찾기 어렵다(파서가 0 축을 막으므로 실제로는 닿지 않는 방어다).
 */
function project(coord: number, canvasExtent: number, stageExtent: number): number {
  const canvas = positiveOrZero(canvasExtent);
  if (canvas === 0) return 0;
  return (finite(coord) / canvas) * positiveOrZero(stageExtent);
}

/** 화면 px 한 축을 캔버스 단위로 되돌린다 — `project` 의 정확한 역이다(정수화하지 않는다). */
function unproject(px: number, canvasExtent: number, stageExtent: number): number {
  const stage = positiveOrZero(stageExtent);
  if (stage === 0) return 0;
  return (finite(px) / stage) * positiveOrZero(canvasExtent);
}

/**
 * CSS px 한 축을 정수 device px 로 반올림한다. 표시 크기가 양수면 최소 1px 은 남긴다 —
 * 0.4px 짜리 패널도 캔버스가 존재는 해야 컨텍스트를 잡을 수 있다.
 */
function devicePixels(cssSize: number, scale: number): number {
  const size = positiveOrZero(cssSize);
  return size > 0 ? Math.max(1, Math.round(size * scale)) : 0;
}

// --- 백킹 버퍼 ----------------------------------------------------------

/**
 * 표시 크기(CSS px)와 devicePixelRatio 로 백킹 버퍼 크기를 구한다(`HeatmapCanvas` 규율).
 *
 * 손상 입력 방어가 이 함수의 절반이다: 비유한·비양수·터무니없이 큰 dpr 은 1 로,
 * 비유한·음수 표시 크기는 0 으로 떨어뜨린다. 렌더 층은 `width === 0` 을 "아직 그릴 수 없음"
 * 으로 읽고 프레임을 건너뛰면 된다.
 */
export function computeBackingSize(cssWidth: number, cssHeight: number, dpr: number): BackingSize {
  const scale =
    Number.isFinite(dpr) && dpr > 0 && dpr <= MAX_DEVICE_PIXEL_RATIO ? dpr : 1;
  return {
    width: devicePixels(cssWidth, scale),
    height: devicePixels(cssHeight, scale),
    scale,
  };
}

// --- 그리는 영역의 격자 정렬 (SPEC-CANVAS-002 0.9.0) --------------------
//
// 격자선이 **온전한 픽셀**에 앉게 만드는 산술이 여기 있다. 뿌리는 이렇다: 한 칸의 화면
// 크기는 `스테이지 / (캔버스 ÷ 간격)` 이라 대개 소수다. 1749px 스테이지 · 500단위 캔버스 ·
// 25단위 격자면 한 칸이 **87.45px** 이고, 선은 0 · 87.45 · 174.90 · 262.35 … 에 선다.
// 소수 자리에서 시작하는 1px 선은 **두 장치 픽셀에 나뉘어 칠해진다**(87.45 는 87 번 픽셀에
// 55% · 88 번 픽셀에 45%). 그래서 선마다 진하기와 굵기가 달라 보이고, 그 들쭉날쭉함이
// 사용자가 세 번 돌려보낸 "격자가 일정하지 않음" 의 정체다. 칸 수가 정수인지(0.8.0 이 고친
// 것)와는 다른 층의 문제다 — 칸 수가 딱 20 칸이어도 각 칸이 87.45px 이면 그렇게 보인다.
//
// **그리는 주기만 정수로 반올림하면 안 된다.** 그러면 선은 87·k 에 서는데 붙임은 여전히
// 87.45·k 에 떨어져, 오른쪽 끝에서 둘이 9px 어긋난다 — 이 기능에서 두 번 걷어낸 거짓말이
// 모양만 바꿔 되돌아온다. 그래서 반올림하는 것은 주기가 아니라 **그리는 영역 자체**다:
// 한 칸을 정수 px 로 내림한 뒤 그 정수배를 그리는 영역으로 삼고, 투영 · 붙임 · 격자가
// **모두 그 영역**을 쓴다. 남는 자투리(한 칸 미만)는 바깥 상자 안의 여백이 된다.
//
// ## 축척은 두 축이 **하나**다 (0.10.0)
//
// 위 산술을 축마다 따로 돌리면 칸은 정수가 되지만 **두 축의 정수가 다르다**. 1749×796
// 패널에 500×400 캔버스면 가로 87px · 세로 49px 이고, 그때 한 칸은 정사각형이 아니라
// 87×49 직사각형이다. 같은 일그러짐이 도형에도 그대로 걸린다 — 캔버스 단위로 정사각형인
// 사각형이 화면에서는 가로로 늘어난 직사각형이 되고, 원은 납작한 타원이 된다. 축척이
// 둘이면 "캔버스 단위" 라는 말이 축마다 다른 뜻을 갖는 셈이다.
//
// 그래서 축척을 **하나**로 둔다: 두 축의 '정확한' 칸 크기 가운데 **작은 쪽**을 골라 그
// 하나를 두 축에 함께 쓴다. 작은 쪽인 이유는 넘치지 않기 위해서다 — 큰 쪽을 고르면 그
// 축에서 그리는 영역이 바깥 상자를 넘어 도형이 잘린다. 정수화(내림)는 고른 **뒤** 한 번만
// 하므로 온전한 픽셀 성질은 그대로 남고, 두 축이 같은 정수를 쓰므로 칸은 정사각형이다.
//
// 남는 자투리는 **한 축에 몰린다**(위 예에서 가로로 769px). 그 여백은 정직한 신호다:
// 캔버스 크기와 패널 모양이 다르다는 사실이 눈에 보이는 것이며, 사용자는 캔버스 크기를
// 패널 비율에 맞춰(`fitCanvasSizeToStage`) 그 여백을 없앨 수 있다. 여백을 없애자고 축척을
// 둘로 되돌리면 도형이 다시 일그러진다 — 조용한 왜곡보다 보이는 여백이 낫다.

/**
 * 한 칸의 CSS px(축마다).
 *
 * 축척이 하나가 된 0.10.0 이후 두 값은 **언제나 같다**. 그럼에도 축마다의 자리를 남겨
 * 두는 것은 `PanelEditGrid` 가 두 축의 간격을 따로 받기 때문이다(그 컴포넌트는 다섯 패널과
 * 공유하며 그쪽은 축마다 다른 백분율을 쓴다). 받는 쪽의 형상을 바꾸지 않으므로 이 파일의
 * 변경이 그 컴포넌트로 번지지 않는다.
 */
export interface StageCell {
  x: number;
  y: number;
}

/**
 * 바깥 상자를 격자 칸에 맞춘 결과 한 벌.
 *
 * 셋을 함께 돌려주는 데 뜻이 있다. 영역 · 칸 · 자투리는 **한 나눗셈에서 함께 나오는 값**
 * 이라, 따로 구하면 그 나눗셈이 두 벌이 되고 한쪽만 고쳐진 채 갈라진다(위험 R1 과 같은
 * 부류다). 받는 쪽은 영역을 상자에, 칸을 격자에, 자투리를 자리에 그대로 쓰면 된다.
 */
export interface StageLattice {
  /** 실제로 그리는 영역(CSS px). 바깥 상자 이하이며 `cell` 의 정수배다. */
  stage: StageSize;
  /** 한 칸의 CSS px. */
  cell: StageCell;
  /** 바깥 상자 안에서 그 영역이 앉는 자리(정수 CSS px). 자투리를 양쪽에 나눈 값이다. */
  offset: StageCell;
}

/**
 * 두 축에 함께 쓸 **정확한**(정수화 이전) 칸 크기. 맞출 수 없으면 `null`.
 *
 * 한 축의 '정확한' 칸 크기는 `바깥 길이 × 간격 ÷ 캔버스 길이` 다. 두 축이 그 값을 따로
 * 가지면 축척이 둘이 되어 칸도 도형도 일그러지므로(머리말 §축척은 두 축이 하나다),
 * **작은 쪽 하나**를 골라 돌려준다. 작은 쪽이라야 두 축 모두 바깥 상자를 넘지 않는다.
 *
 * `null` 은 "맞출 근거가 없다" 는 뜻이다 — 아직 재지 못한 상자(0·비유한), 0 축 캔버스,
 * 0 이하·비유한 간격이 그렇다. 그때 부르는 쪽은 잰 상자를 그대로 쓴다.
 */
function uniformCell(outer: StageSize, canvas: CanvasSize, step: number): number | null {
  const outerW = positiveOrZero(outer.width);
  const outerH = positiveOrZero(outer.height);
  const canvasW = positiveOrZero(canvas.width);
  const canvasH = positiveOrZero(canvas.height);
  if (!Number.isFinite(step) || step <= 0) return null;
  if (outerW === 0 || outerH === 0 || canvasW === 0 || canvasH === 0) return null;
  return Math.min((outerW * step) / canvasW, (outerH * step) / canvasH);
}

/**
 * 잰 바깥 상자를 **격자 칸의 정수배**로 맞춘다(사용 시험: "격자가 일정하지 않음").
 *
 * 부르는 쪽은 표면 하나다 — 그 표면이 돌려받은 `stage` 로 상자를 짓고, 그 상자 안에
 * 캔버스와 오버레이를 함께 넣는다. 그래야 투영이 쓰는 스테이지와 오버레이가 제 상자로
 * 재는 값이 **같은 상자**를 가리키고, 그 사이에 오프셋 보정이 낄 자리가 없다(위험 R1).
 *
 * 결과의 성질 셋:
 *   1. **칸은 정사각형이다** — 두 축이 같은 축척 하나를 쓴다(머리말 §축척은 두 축이 하나다).
 *      그래서 캔버스 단위의 정사각형은 화면에서도 정사각형이고, 원은 원으로 남는다.
 *   2. **칸은 정수 px 다** — 고른 축척을 내림한다. 그래야 선이 두 장치 픽셀에 걸치지 않는다.
 *   3. **영역은 그 칸의 정수배이고 자리는 정수다** — 자투리는 상자 안 여백이 된다.
 *
 * **한 칸이 1px 도 되지 않으면 정수화하지 않는다.** 내림값 0 을 곱하면 그리는 영역이
 * 통째로 사라진다 — 격자를 위해 그림을 지우는 셈이다. 그때는 소수 축척을 그대로 쓴다:
 * 온전한 픽셀 성질은 포기하지만(1px 미만 칸에서는 애초에 뜻이 없다) 축척은 여전히 하나라
 * 도형은 일그러지지 않고, 영역도 사라지지 않는다. 받는 쪽은 1px 미만인 칸을 보고 격자를
 * 그리지 않기로 정할 수 있다.
 *
 * 멱등이다: 이미 맞춰진 영역을 같은 간격으로 다시 넣으면 같은 영역이 나오고 자투리는 0 이
 * 된다(맞춘 영역은 정확히 캔버스 비율이므로 두 축의 '정확한' 칸이 같아진다). 그래서 표면
 * 밖(오버레이 단독 렌더)에서도 같은 함수로 칸을 물을 수 있다.
 */
export function stageLattice(
  outer: StageSize,
  canvas: CanvasSize,
  step: number,
): StageLattice {
  const outerW = positiveOrZero(outer.width);
  const outerH = positiveOrZero(outer.height);
  const exact = uniformCell(outer, canvas, step);
  if (exact === null) {
    return { stage: { width: outerW, height: outerH }, cell: { x: 0, y: 0 }, offset: { x: 0, y: 0 } };
  }
  const cell = exact >= 1 ? Math.floor(exact) : exact;
  const scale = cell / step;
  // `Math.min` 은 부동소수 잔차 방어다 — 내림한 값이므로 수학적으로는 이미 outer 이하다.
  const width = Math.min(positiveOrZero(canvas.width) * scale, outerW);
  const height = Math.min(positiveOrZero(canvas.height) * scale, outerH);
  return {
    stage: { width, height },
    cell: { x: cell, y: cell },
    offset: { x: Math.floor((outerW - width) / 2), y: Math.floor((outerH - height) / 2) },
  };
}

// --- 캔버스 크기를 패널 비율에 맞춘다 (0.10.0) --------------------------
//
// 축척이 하나가 되면 캔버스 비율과 패널 비율이 다를 때 한 축에 여백이 남는다. 그 여백을
// 없애는 정직한 길은 **캔버스 크기 자체를 패널 비율에 맞추는 것**이다 — 축척을 둘로
// 되돌려 도형을 일그러뜨리는 대신, 종이의 모양을 방의 모양에 맞춘다.
//
// **폭을 고정하고 높이를 유도한다.** 두 축 가운데 하나를 붙들어야 하는데, 폭이 자연스러운
// 기준인 이유는 격자 간격 선택지가 이미 폭 기준으로 잡혀 있고(500 은 고를 수 있는 모든
// 간격으로 나누어떨어진다) 사용자가 "가로 몇 칸" 으로 판을 상상하기 때문이다.
//
// **이 함수는 저장하지 않는다.** 순수 계산이며, 언제 부를지는 부르는 쪽의 몫이다. 그
// 구분에 뜻이 있다 — 리사이즈마다 자동으로 부르면 저장된 요소 좌표의 뜻이 조용히 바뀐다
// (`CanvasPanel` §자동 맞춤은 한 번뿐이다).

/**
 * 캔버스 크기를 스테이지(패널) 비율에 맞춘다 — **폭은 그대로, 높이만** 유도한다.
 *
 * 잴 수 없는 스테이지(0·비유한 축)나 이미 맞아 있는 크기에는 **받은 객체를 그대로**
 * 돌려준다. 참조가 같다는 사실이 곧 "바꿀 것이 없다" 는 신호이므로, 부르는 쪽은 그
 * 비교 하나로 쓸모없는 저장을 건너뛸 수 있다(자동 맞춤이 리사이즈마다 config 를 쓰지
 * 않는 근거의 절반이 이 한 줄이다).
 *
 * 유도한 높이는 캔버스 축이 허용하는 범위로 죈다 — 파서가 죄는 그 범위와 같아야 저장
 * 왕복에 값이 바뀌지 않는다.
 */
export function fitCanvasSizeToStage(canvas: CanvasSize, stage: StageSize): CanvasSize {
  const stageW = positiveOrZero(stage.width);
  const stageH = positiveOrZero(stage.height);
  const width = positiveOrZero(canvas.width);
  if (stageW === 0 || stageH === 0 || width === 0) return canvas;
  const raw = Math.round((width * stageH) / stageW);
  const height = Math.min(Math.max(raw, MIN_CANVAS_DIMENSION), MAX_CANVAS_DIMENSION);
  return height === canvas.height ? canvas : { width: canvas.width, height };
}

// --- 투영 ---------------------------------------------------------------

/**
 * rect/ellipse 기하를 스테이지 px 로 투영한다.
 *
 * 스테이지가 0 크기면 모든 결과가 (유한한) 0 이다 — NaN 을 흘리면 그 프레임의 경로 전체가
 * 조용히 사라져 원인을 찾기 어렵다.
 */
export function projectBox(geo: BoxGeometry, proj: CanvasProjection): PxBox {
  const { stage, canvas } = proj;
  return {
    x: project(geo.x, canvas.width, stage.width),
    y: project(geo.y, canvas.height, stage.height),
    w: project(geo.w, canvas.width, stage.width),
    h: project(geo.h, canvas.height, stage.height),
  };
}

/** line 기하를 스테이지 px 로 투영한다(위와 같은 규율). */
export function projectLine(geo: LineGeometry, proj: CanvasProjection): PxLine {
  const { stage, canvas } = proj;
  return {
    x1: project(geo.x1, canvas.width, stage.width),
    y1: project(geo.y1, canvas.height, stage.height),
    x2: project(geo.x2, canvas.width, stage.width),
    y2: project(geo.y2, canvas.height, stage.height),
  };
}

/** text 기준점을 스테이지 px 로 투영한다(위와 같은 규율). */
export function projectPoint(geo: PointGeometry, proj: CanvasProjection): PxPoint {
  const { stage, canvas } = proj;
  return {
    x: project(geo.x, canvas.width, stage.width),
    y: project(geo.y, canvas.height, stage.height),
  };
}

/**
 * 경로 명령 목록을 **요소 상자 안의 스테이지 px** 로 투영한다(SPEC-CANVAS-008 REQ-01).
 *
 * `box` 는 `projectBox(el.geometry, proj)` 의 결과다 — **경로가 스테이지를 다시 재지
 * 않는다.** 상자를 인자로 받는 것이 그 금지를 형상으로 만든다: 이 함수에는 `proj` 가 아예
 * 없으므로 스테이지 축 길이를 볼 방법이 없다.
 *
 * `로컬 ÷ PATH_LOCAL_EXTENT × 상자 길이` 라는 나눗셈이 나타나는 자리는 **이 함수
 * 하나여야 한다**(불변식 J3). 두 곳이 되면 렌더와 히트가 서로 다른 격자를 보게 되고,
 * 그 어긋남은 화면에서만 드러난다.
 *
 * **소비자는 둘이다** — 렌더는 이 결과를 그대로 그리고, 히트는 이 결과를 평탄화해 훑는다
 * (M4). 갈라지는 것은 마지막 한 걸음뿐이며 그 앞은 이 함수 하나다.
 *
 * 인자가 **둘뿐인 것이 이 시그니처의 요구다.** 셋째 인자(스타일 · 배율 · 표시 상태)를
 * 받는 순간 경로 투영이 표시 상태를 보게 되고, 그러면 "그린 자리" 와 "잡히는 자리" 가
 * 서로 다른 입력에서 나온다. 기본값 있는 셋째 인자는 `Function.length` 를 속이므로
 * 시험은 **매개변수 목록의 형상**으로 판정한다(시험 규율 D9).
 *
 * 음수 크기 상자(001 이 읽기 경로의 견고성으로 허용한다)는 그대로 통과한다 — 좌표가
 * 뒤집힐 뿐 NaN 이 되지 않는다. 로컬 좌표를 격자 안으로 clamp 하지 않는다(가정 A5).
 */
export function projectPathPoints(
  path: readonly PathCommand[],
  box: PxBox,
): readonly PxPathCommand[] {
  const originX = finite(box.x);
  const originY = finite(box.y);
  // 상수는 양수 리터럴이라 0 으로 나눌 일이 없다.
  const scaleX = finite(box.w) / PATH_LOCAL_EXTENT;
  const scaleY = finite(box.h) / PATH_LOCAL_EXTENT;
  const px = (local: number): number => originX + finite(local) * scaleX;
  const py = (local: number): number => originY + finite(local) * scaleY;

  return path.map((cmd): PxPathCommand => {
    switch (cmd.c) {
      case 'M':
        return { c: 'M', x: px(cmd.x), y: py(cmd.y) };
      case 'L':
        return { c: 'L', x: px(cmd.x), y: py(cmd.y) };
      case 'C':
        return {
          c: 'C',
          x1: px(cmd.x1),
          y1: py(cmd.y1),
          x2: px(cmd.x2),
          y2: py(cmd.y2),
          x: px(cmd.x),
          y: py(cmd.y),
        };
      case 'Z':
        return { c: 'Z' };
    }
  });
}

// --- 역투영 -------------------------------------------------------------
//
// 역투영이 여기 있는 것과 `canvasHitTest` 가 금지한 "도형별 역산" 은 다른 것이다. 금지된
// 것은 **판정**을 역산으로 푸는 일이고(그러면 집기 여유가 화면 양이 아니게 된다), 여기
// 있는 것은 **자리**를 캔버스 단위로 되돌리는 한 쌍의 산술이다. 저장 좌표가 캔버스 단위인
// 이상 포인터를 그 공간으로 되돌리는 자리는 반드시 있어야 하며, 그 자리는 **하나**여야
// 한다 — 둘이 되면 드래그와 붙임이 서로 다른 자리를 계산한다.

/** 스테이지 로컬 CSS px 점을 캔버스 단위로 되돌린다. 정수화는 쓰는 쪽의 몫이다. */
export function unprojectPoint(px: PxPoint, proj: CanvasProjection): CanvasPoint {
  const { stage, canvas } = proj;
  return {
    x: unproject(px.x, canvas.width, stage.width),
    y: unproject(px.y, canvas.height, stage.height),
  };
}

/** 스테이지 로컬 CSS px 상자를 캔버스 단위로 되돌린다(격자 붙임의 기준 상자에 쓴다). */
export function unprojectBox(px: PxBox, proj: CanvasProjection): CanvasBox {
  const { stage, canvas } = proj;
  return {
    x: unproject(px.x, canvas.width, stage.width),
    y: unproject(px.y, canvas.height, stage.height),
    w: unproject(px.w, canvas.width, stage.width),
    h: unproject(px.h, canvas.height, stage.height),
  };
}

// --- 도형 파라미터 ------------------------------------------------------

/**
 * 좌상단+크기 박스를 `ctx.ellipse()` 의 중심+반지름으로 바꾼다.
 *
 * 폭·높이가 음수인 박스도 그린다 — 저술 UI 가 음수 크기를 막더라도 저장 왕복·003 의 보간
 * 중간값이 음수를 만들 수 있고, 그때 도형이 사라지는 것보다 같은 자리에 그려지는 편이 낫다.
 * 중심은 `x + w/2` 그대로 옳고(음수 폭이면 x 왼쪽이 중심이 된다), 반지름만 절대값을 쓴다.
 *
 * 공간 불변: 캔버스 단위 박스든 투영된 px 박스든 같은 산술이라 어느 쪽에도 쓸 수 있다.
 * 렌더 층은 `projectBox` 결과를 넣는다.
 */
export function ellipseParams(box: PxBox): EllipseParams {
  const x = finite(box.x);
  const y = finite(box.y);
  const w = finite(box.w);
  const h = finite(box.h);
  return { cx: x + w / 2, cy: y + h / 2, rx: Math.abs(w) / 2, ry: Math.abs(h) / 2 };
}

// --- 텍스트 배치 --------------------------------------------------------

/**
 * 정렬 기준점 + 실측 글자 폭 → **좌측 끝 원점**.
 *
 * 폭은 렌더 층이 `measureText` 로 재어 넘긴다(이 모듈이 DOM 을 보지 않는 이유). 좌측 끝을
 * 돌려주므로 렌더 층은 `ctx.textAlign` 을 한 값으로 고정한 채 그릴 수 있고, 라벨 배경·테두리
 * 처럼 사각형이 필요한 후속 작업도 같은 원점에서 시작한다.
 *
 * 비유한 폭(측정 실패)은 0 으로 본다 — 정렬이 무너져도 글자는 기준점에 남는다.
 */
export function resolveTextOrigin(
  point: PxPoint,
  align: ElementAlign,
  measuredWidth: number,
): PxPoint {
  const x = finite(point.x);
  const y = finite(point.y);
  const width = finite(measuredWidth);
  switch (align) {
    case 'center':
      return { x: x - width / 2, y };
    case 'right':
      return { x: x - width, y };
    default:
      return { x, y };
  }
}

/**
 * 요소 정렬 → canvas `textAlign` 값. 지금은 세 값이 그대로 대응하지만, 매핑을 함수로 두어
 * 렌더 층이 "두 어휘가 같다" 는 가정을 코드에 박지 않게 한다(향후 `start`/`end` 도입 지점).
 */
export function toCanvasTextAlign(align: ElementAlign): 'left' | 'center' | 'right' {
  return align;
}

// --- 라벨 기준점 --------------------------------------------------------

/**
 * 요소에 붙는 라벨의 기준점(REQ-02: 도형에도 라벨이 붙을 수 있다).
 *
 * rect/ellipse/path 는 상자의 중심, line 은 중점, text 는 자기 기준점이다. `kind` 로
 * 좁히면 `geometry` 도 함께 좁혀지므로 형상 검사나 캐스팅이 필요 없다(canvasConfig 의
 * 판별 합집합 설계).
 *
 * **경로가 상자 갈래에 붙는 것이 이 함수에 더해진 전부다.** 종전의 `default:` 는 상자
 * 기하를 문구 기준점으로 읽어(구조적으로 대입된다 — 가정 A6) 라벨을 상자의 **좌상단**에
 * 앉혔다. 컴파일러가 울지 않는 자리였고, 그래서 갈래를 이름으로 적는다: `default:` 를
 * `case 'text':` 로 펴 두면 여섯 번째 종류가 들어올 때 컴파일러가 이 자리를 가리킨다.
 */
export function labelAnchor(el: CanvasElement, proj: CanvasProjection): PxPoint {
  switch (el.kind) {
    case 'rect':
    case 'ellipse':
    case 'path': {
      const { cx, cy } = ellipseParams(projectBox(el.geometry, proj));
      return { x: cx, y: cy };
    }
    case 'line': {
      const line = projectLine(el.geometry, proj);
      return { x: (line.x1 + line.x2) / 2, y: (line.y1 + line.y2) / 2 };
    }
    case 'text':
      return projectPoint(el.geometry, proj);
  }
}
