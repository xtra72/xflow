// 캔버스 요소 렌더 층 (SPEC-CANVAS-001 T4).
//
// 순수 모듈들(canvasGeometry / canvasRules / canvasTween / canvasText)이 끝낸 계산을
// 2D context 에 **그리기만** 하는 얇은 층이다. 여기서 새로 계산하는 것은 셋뿐이며,
// 셋 다 DOM 없이는 결정할 수 없는 것들이다:
//   1) `measureText` 로 잰 글자 폭 — canvasGeometry 가 인자로 받기로 한 그 값이다.
//   2) `font` 문자열 — 굵기·크기를 브라우저 어휘로 옮긴다.
//   3) `textBaseline` — canvasGeometry 는 y 축을 다루지 않는다고 명시했으므로 여기서 정한다.
//
// **타입 축이 이 파일의 설계 결정이다.** `CanvasRenderingContext2D` 대신 실제로 쓰는
// 멤버만 모은 최소 구조 인터페이스 `DrawContext2D` 로 타입 짓는다. 진짜 context 는 구조적
// 으로 이 인터페이스를 만족하므로 런타임 동작은 같고, 테스트는 jsdom canvas(`node-canvas`
// 없이는 null 을 돌려준다) 없이 기록 스텁만으로 렌더 경로 전체를 검증할 수 있다.
//
// 그리기 순서는 배열 순서다(뒤가 위) — 001 의 유일한 z-order 수단이다(REQ-02).
//
// @spec SPEC-CANVAS-001

import {
  DEFAULT_FONT_SIZE,
  DEFAULT_OPACITY,
  DEFAULT_STROKE_WIDTH,
  type CanvasElement,
} from './canvasConfig';
import {
  ellipseParams,
  labelAnchor,
  projectBox,
  projectLine,
  projectPoint,
  resolveTextOrigin,
  type BackingSize,
  type PxPoint,
  type StageSize,
} from './canvasGeometry';
import type { ResolvedStyle } from './canvasRules';

// --- 최소 context 인터페이스 ---------------------------------------------

/**
 * 이 모듈이 실제로 만지는 2D context 멤버만 모은 구조 인터페이스.
 *
 * 진짜 `CanvasRenderingContext2D` 는 이 형상을 구조적으로 만족한다(속성 타입을 DOM 정의와
 * 같은 합집합으로 둔 이유 — `fillStyle: string` 으로 좁히면 진짜 context 가 대입되지 않는다).
 * 반대로 테스트는 이 형상만 갖춘 기록 스텁을 넘겨 호출 순서·인자·설정된 속성을 그대로
 * 관찰한다. "그리기"는 부수효과라 결과를 볼 방법이 호출 기록뿐인데, 이 인터페이스가 그
 * 기록을 1급 관찰 대상으로 만든다.
 */
export interface DrawContext2D {
  save(): void;
  restore(): void;
  /** 매 프레임 좌표계를 다시 세운다(DPR 배율 적용/해제). */
  setTransform(a: number, b: number, c: number, d: number, e: number, f: number): void;
  beginPath(): void;
  rect(x: number, y: number, w: number, h: number): void;
  ellipse(
    x: number,
    y: number,
    radiusX: number,
    radiusY: number,
    rotation: number,
    startAngle: number,
    endAngle: number,
  ): void;
  moveTo(x: number, y: number): void;
  lineTo(x: number, y: number): void;
  stroke(): void;
  fill(): void;
  fillText(text: string, x: number, y: number): void;
  measureText(text: string): { readonly width: number };
  clearRect(x: number, y: number, w: number, h: number): void;
  fillRect(x: number, y: number, w: number, h: number): void;
  fillStyle: string | CanvasGradient | CanvasPattern;
  strokeStyle: string | CanvasGradient | CanvasPattern;
  lineWidth: number;
  globalAlpha: number;
  font: string;
  textAlign: CanvasTextAlign;
  textBaseline: CanvasTextBaseline;
}

// --- 상수 ---------------------------------------------------------------

/** 글꼴 계열. 패널이 폰트를 책임진다는 REQ-02 의 요구를 한 곳에 모은다. */
export const DEFAULT_FONT_FAMILY = 'sans-serif';

/**
 * 문구의 세로 기준.
 *
 * canvasGeometry 는 가로 정렬(좌측 끝 원점)만 계산하고 **y 축은 다루지 않는다**고 명시했다.
 * 그 빈자리를 여기서 `'middle'` 로 메운다. 이유는 라벨 기준점의 뜻과 맞물린다 —
 * `labelAnchor` 는 rect/ellipse 의 **중심**, line 의 **중점**을 돌려준다. canvas 기본값
 * `'alphabetic'` 로 그리면 글자가 그 중심보다 위로 올라앉아 "중심에 붙인 라벨"이 중심에
 * 있지 않게 된다. `'middle'` 이면 기준점이 곧 글자 상자의 세로 중앙이라 도형 라벨과
 * 독립 text 요소가 같은 y 의미("이 점이 글줄의 세로 가운데")를 갖는다.
 */
export const TEXT_BASELINE: CanvasTextBaseline = 'middle';

/**
 * 좌측 끝 원점으로 그리므로 `textAlign` 은 `'left'` 하나로 고정한다.
 * 정렬은 `resolveTextOrigin` 이 이미 원점에 반영해 두었다 — 여기서 다시 정렬하면 두 번
 * 적용된다.
 */
const FIXED_TEXT_ALIGN: CanvasTextAlign = 'left';

/** 타원 한 바퀴. */
const FULL_TURN = Math.PI * 2;

// --- 스타일 해석 도우미 --------------------------------------------------

/** 불투명도를 0..1 유한값으로 죈다. 미지정·손상 값은 기본 불투명도다. */
function resolveAlpha(opacity: number | undefined): number {
  if (opacity === undefined || !Number.isFinite(opacity)) return DEFAULT_OPACITY;
  return opacity < 0 ? 0 : opacity > 1 ? 1 : opacity;
}

/** 유효한 선 두께(px). 미지정은 기본값, 손상·0·음수는 "선 없음"을 뜻하는 0 이다. */
function resolveStrokeWidth(width: number | undefined): number {
  if (width === undefined) return DEFAULT_STROKE_WIDTH;
  return Number.isFinite(width) && width > 0 ? width : 0;
}

/** `font` 문자열. 굵기는 즉시 전환 속성이라 트윈을 거치지 않고 그대로 들어온다. */
function fontSpec(style: ResolvedStyle): string {
  const size =
    style.fontSize !== undefined && Number.isFinite(style.fontSize) && style.fontSize > 0
      ? style.fontSize
      : DEFAULT_FONT_SIZE;
  const weight = style.fontWeight === 'bold' ? 'bold ' : '';
  return `${weight}${size}px ${DEFAULT_FONT_FAMILY}`;
}

// --- 칠하기 -------------------------------------------------------------

/**
 * 현재 경로를 채운다. `fill` 이 없으면 **아무것도 하지 않는다** — 기본 색을 지어내면
 * 사용자가 "색을 지정하지 않음"을 표현할 수 없다.
 */
function paintFill(ctx: DrawContext2D, style: ResolvedStyle): void {
  if (style.fill === undefined) return;
  ctx.fillStyle = style.fill;
  ctx.fill();
}

/** 현재 경로에 선을 긋는다. 색이 없거나 두께가 0 이하면 긋지 않는다. */
function paintStroke(ctx: DrawContext2D, style: ResolvedStyle): void {
  const width = resolveStrokeWidth(style.strokeWidth);
  if (style.stroke === undefined || width <= 0) return;
  ctx.strokeStyle = style.stroke;
  ctx.lineWidth = width;
  ctx.stroke();
}

/**
 * 문구 한 줄을 기준점에 그린다.
 *
 * `paint` 는 호출자가 정한다 — text 요소와 도형 라벨의 색 규칙이 다르기 때문이다
 * (`drawElement` 주석 참조). 색이 없으면 그리지 않는다.
 */
function paintText(
  ctx: DrawContext2D,
  text: string | undefined,
  anchor: PxPoint,
  style: ResolvedStyle,
  paint: string | undefined,
): void {
  if (text === undefined || text === '' || paint === undefined) return;
  ctx.font = fontSpec(style);
  ctx.textAlign = FIXED_TEXT_ALIGN;
  ctx.textBaseline = TEXT_BASELINE;
  // 폭을 재는 쪽은 렌더 층, 그 폭으로 원점을 정하는 쪽은 순수 기하 모듈이다.
  const measured = ctx.measureText(text).width;
  const origin = resolveTextOrigin(anchor, style.align ?? 'left', measured);
  ctx.fillStyle = paint;
  ctx.fillText(text, origin.x, origin.y);
}

// --- 요소 그리기 ---------------------------------------------------------

/**
 * 요소 하나를 그린다.
 *
 * - `visible: false` 는 **아무것도 그리지 않는다**(`save`/`restore` 조차 하지 않는다).
 * - 모든 상태 변경은 `save`/`restore` 사이에 갇힌다. 한 요소의 색·투명도·글꼴이 다음
 *   요소로 새면 배열 순서에 따라 그림이 달라지는데, 그건 재현 불가능한 버그가 된다.
 * - **던지지 않는다.** 손상된 요소 하나가 프레임 전체를 지우면 AC-E4("마지막 프레임을
 *   유지한다")가 무너진다. 그리다 실패한 요소는 조용히 건너뛰고 나머지를 계속 그린다.
 *
 * 문구 색 규칙(두 갈래인 이유):
 * - `kind: 'text'` — `textColor` 가 없으면 `fill` 을 쓴다. 채울 도형이 없으므로 `fill` 은
 *   글자색으로 읽는 것이 자연스럽고, 색을 새로 지어내는 것도 아니다.
 * - 도형 라벨 — `textColor` 만 쓴다. 도형의 `fill` 로 폴백하면 라벨이 제 도형과 같은 색이
 *   되어 보이지 않는다.
 */
export function drawElement(
  ctx: DrawContext2D,
  el: CanvasElement,
  style: ResolvedStyle,
  text: string | undefined,
  stage: StageSize,
): void {
  if (style.visible === false) return;
  ctx.save();
  try {
    ctx.globalAlpha = resolveAlpha(style.opacity);
    switch (el.kind) {
      case 'rect': {
        const box = projectBox(el.geometry, stage);
        ctx.beginPath();
        ctx.rect(box.x, box.y, box.w, box.h);
        paintFill(ctx, style);
        paintStroke(ctx, style);
        break;
      }
      case 'ellipse': {
        const { cx, cy, rx, ry } = ellipseParams(projectBox(el.geometry, stage));
        ctx.beginPath();
        ctx.ellipse(cx, cy, rx, ry, 0, 0, FULL_TURN);
        paintFill(ctx, style);
        paintStroke(ctx, style);
        break;
      }
      case 'line': {
        const line = projectLine(el.geometry, stage);
        ctx.beginPath();
        ctx.moveTo(line.x1, line.y1);
        ctx.lineTo(line.x2, line.y2);
        // 선은 채우지 않는다 — 열린 경로의 fill 은 뜻이 없다.
        paintStroke(ctx, style);
        break;
      }
      case 'text': {
        paintText(ctx, text, projectPoint(el.geometry, stage), style, style.textColor ?? style.fill);
        break;
      }
    }
    // 도형에 붙은 라벨(REQ-02). text 요소는 위에서 이미 그렸다.
    if (el.kind !== 'text') {
      paintText(ctx, text, labelAnchor(el, stage), style, style.textColor);
    }
  } catch {
    // 손상 요소 하나가 프레임 전체를 무너뜨리지 않는다(REQ-05).
  } finally {
    ctx.restore();
  }
}

/**
 * 요소 목록을 **배열 순서대로** 그린다(뒤가 위, REQ-02).
 *
 * 스타일·문구 맵에 항목이 없으면 요소의 기본값으로 떨어진다 — 바인딩 시리즈가 없거나
 * 규칙이 하나도 일치하지 않는 경우가 정상 경로이기 때문이다(AC-E2/AC-E3).
 */
export function drawElements(
  ctx: DrawContext2D,
  elements: readonly CanvasElement[],
  styles: Record<string, ResolvedStyle>,
  texts: Record<string, string | undefined>,
  stage: StageSize,
): void {
  for (const el of elements) {
    drawElement(ctx, el, styles[el.id] ?? el.style, texts[el.id] ?? el.text, stage);
  }
}

/**
 * 프레임 시작 시 표면을 비운다.
 *
 * **항등 변환(= device px) 기준**이다. 스스로 변환을 초기화하므로 호출자는 배경을 지운 뒤
 * DPR 배율을 다시 걸면 된다. 배경색이 없으면 지우기만 한다 — 그래야 패널 표면색이 비쳐
 * 보인다(config §background "미지정 시 패널 표면색").
 */
export function clearSurface(
  ctx: DrawContext2D,
  backing: BackingSize,
  background?: string,
): void {
  ctx.setTransform(1, 0, 0, 1, 0, 0);
  ctx.globalAlpha = 1;
  ctx.clearRect(0, 0, backing.width, backing.height);
  if (background === undefined) return;
  ctx.fillStyle = background;
  ctx.fillRect(0, 0, backing.width, backing.height);
}
