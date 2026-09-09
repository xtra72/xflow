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
// 백킹 버퍼 산술은 `HeatmapCanvas.tsx` 의 DPR 규율(표시 크기 × dpr 을 정수 device px 로
// 반올림)에서 순수 부분만 뽑아낸 것이다.
//
// SPEC-CANVAS-003(값 구동 애니메이션)을 위한 형상 제약: 이 모듈의 입력은 모두 **평범한
// 수치 레코드**이고 출력도 그렇다. 003 은 `BoxGeometry`/`LineGeometry` 의 수치를 보간한 뒤
// 같은 함수에 넣으면 되므로 이 API 를 다시 짤 필요가 없다.
//
// @spec SPEC-CANVAS-001

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
 * rect/ellipse 는 중심, line 은 중점, text 는 자기 기준점이다. `kind` 로 좁히면 `geometry`
 * 도 함께 좁혀지므로 형상 검사나 캐스팅이 필요 없다(canvasConfig 의 판별 합집합 설계).
 */
export function labelAnchor(el: CanvasElement, proj: CanvasProjection): PxPoint {
  switch (el.kind) {
    case 'rect':
    case 'ellipse': {
      const { cx, cy } = ellipseParams(projectBox(el.geometry, proj));
      return { x: cx, y: cy };
    }
    case 'line': {
      const line = projectLine(el.geometry, proj);
      return { x: (line.x1 + line.x2) / 2, y: (line.y1 + line.y2) / 2 };
    }
    default:
      return projectPoint(el.geometry, proj);
  }
}
