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
// **SPEC-CANVAS-002 가 이 파일에 더한 것은 `drawElements` 의 반환 타입 하나뿐이다**(T2).
// `void → Record<string, number>`(`kind:'text'` 요소 id → 실측 글자 폭)로 넓혔을 뿐,
// 인자도 `DrawContext2D` 도 그리기 순서도 그려진 결과도 001 그대로다. 시각 편집기의 히트
// 테스트는 텍스트 상자를 잡으려면 실측 폭이 필요한데 그 값은 이 층이 프레임마다 이미 재고
// 있었다 — 새로 재는 대신 **이미 잰 값을 돌려주는 쪽**을 택한 이유는 스테이지·글자 폭의
// 두 번째 측정원을 만들지 않는다는 002 REQ-05 금지 조항이다. 측정 횟수는 프레임당 1회로
// 변함이 없고, 반환값을 무시하는 기존 호출부는 영향을 받지 않는다.
//
// @spec SPEC-CANVAS-001 · SPEC-CANVAS-002 (T2 — drawElements 반환 타입)

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
  projectPathPoints,
  projectPoint,
  resolveTextOrigin,
  type BackingSize,
  type CanvasProjection,
  type PxPoint,
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
  /**
   * 지금 부분 경로를 닫는다(SPEC-CANVAS-008 REQ-01 — 더하는 둘 가운데 하나).
   *
   * **채움에는 없어도 되고 선에는 없으면 안 된다.** `fill()` 은 부분 경로를 암묵적으로
   * 닫으므로 채운 별은 멀쩡해 보이지만, 마지막 정점에서 첫 정점으로 `lineTo` 로 돌아가면
   * 그 자리는 **이음(join)이 아니라 두 끝점**이라 두께 2px 이상에서 뾰족한 꼭짓점에 홈이
   * 파인다. 다각형 23종에도 걸리는 요구라 곡선 도형만의 문제가 아니다.
   */
  closePath(): void;
  /**
   * 3차 베지어 한 구간(같은 REQ-01 — 더하는 둘 가운데 둘).
   *
   * 카탈로그 30종 가운데 곡선을 요구하는 7종(둥근 사각형 · 문서 · 원통 · 말풍선 · 액터 ·
   * 구름 · 곡선 화살표)이 이것 하나로 전부 적힌다. `quadraticCurveTo` 를 더하지 않는 것은
   * 2차가 3차로 **정확히** 표현되기 때문이고(`c1 = p0 + 2/3(q−p0)`), `arcTo` 를 더하지
   * 않는 것은 4분원이 `k ≈ 0.5523` 오프셋에서 최대 반경 오차 0.027% 로 근사되며 접선
   * 기반 의미가 "모서리를 적는 두 번째 방법" 을 만들기 때문이다.
   *
   * **둘 다 동기다.** 값을 돌려주지도 기다리지도 않으므로 프레임을 예약하지 않고, 그래서
   * 001 REQ-05 의 유휴 정지가 한 줄도 바뀌지 않는다 — 004 가 `drawImage` 를 기각한 근거가
   * 경로에 옮겨 오지 않는 이유가 정확히 이것이다.
   */
  bezierCurveTo(
    cp1x: number,
    cp1y: number,
    cp2x: number,
    cp2y: number,
    x: number,
    y: number,
  ): void;
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
 * 문구 한 줄을 기준점에 그린다. **잰 폭(CSS px)을 돌려준다.**
 *
 * `paint` 는 호출자가 정한다 — text 요소와 도형 라벨의 색 규칙이 다르기 때문이다
 * (`drawElement` 주석 참조). 색이 없으면 그리지 않는다.
 *
 * 폭을 돌려주는 것은 SPEC-CANVAS-002 의 요구다 — 히트 테스트가 텍스트 상자를 잡으려면
 * 실측 폭이 필요한데, 그 값은 여기서 이미 재고 있다. 두 번째 측정원을 만들지 않기 위해
 * **이미 잰 값을 흘려보낼 뿐** 새로 재지 않는다. 그리지 않은 경우(문구·색 없음)는
 * 잰 적이 없으므로 `undefined` 이며, 그때 히트 테스트는 기준점 둘레 여유 상자로 폴백한다.
 */
function paintText(
  ctx: DrawContext2D,
  text: string | undefined,
  anchor: PxPoint,
  style: ResolvedStyle,
  paint: string | undefined,
): number | undefined {
  if (text === undefined || text === '' || paint === undefined) return undefined;
  ctx.font = fontSpec(style);
  ctx.textAlign = FIXED_TEXT_ALIGN;
  ctx.textBaseline = TEXT_BASELINE;
  // 폭을 재는 쪽은 렌더 층, 그 폭으로 원점을 정하는 쪽은 순수 기하 모듈이다.
  const measured = ctx.measureText(text).width;
  const origin = resolveTextOrigin(anchor, style.align ?? 'left', measured);
  ctx.fillStyle = paint;
  ctx.fillText(text, origin.x, origin.y);
  return measured;
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
  proj: CanvasProjection,
): void {
  // 시그니처는 001 그대로다(반환 없음). 잰 폭이 필요한 쪽은 `drawElements` 뿐이므로
  // 폭을 흘려보내는 통로는 아래 내부 함수에 두고, 이 공개 함수는 001 의 형상을 지킨다.
  drawMeasuredElement(ctx, el, style, text, proj);
}

/**
 * `drawElement` 의 본체. 그린 글자의 **실측 폭**을 돌려준다는 점만 다르다.
 *
 * `kind: 'text'` 일 때만 값이 나온다. 도형 라벨의 폭은 재고도 버린다 — 라벨은 자기 기하가
 * 없어 히트 영역도 핸들도 갖지 않으므로(`canvasHitTest` · `canvasEditGeometry` 가 둘 다
 * `kind:'text'` 에만 폭을 쓴다), 도형 id 로 라벨 폭을 흘려보내면 받는 쪽이 "이 도형에도
 * 잡을 수 있는 글자 상자가 있다" 고 잘못 읽는다. 값을 버리는 비용은 0 이고, 잘못 읽힐
 * 여지를 남기는 비용은 0 이 아니다.
 */
function drawMeasuredElement(
  ctx: DrawContext2D,
  el: CanvasElement,
  style: ResolvedStyle,
  text: string | undefined,
  proj: CanvasProjection,
): number | undefined {
  if (style.visible === false) return undefined;
  let measured: number | undefined;
  ctx.save();
  try {
    ctx.globalAlpha = resolveAlpha(style.opacity);
    switch (el.kind) {
      case 'rect': {
        const box = projectBox(el.geometry, proj);
        ctx.beginPath();
        ctx.rect(box.x, box.y, box.w, box.h);
        paintFill(ctx, style);
        paintStroke(ctx, style);
        break;
      }
      case 'ellipse': {
        const { cx, cy, rx, ry } = ellipseParams(projectBox(el.geometry, proj));
        ctx.beginPath();
        ctx.ellipse(cx, cy, rx, ry, 0, 0, FULL_TURN);
        paintFill(ctx, style);
        paintStroke(ctx, style);
        break;
      }
      case 'line': {
        const line = projectLine(el.geometry, proj);
        ctx.beginPath();
        ctx.moveTo(line.x1, line.y1);
        ctx.lineTo(line.x2, line.y2);
        // 선은 채우지 않는다 — 열린 경로의 fill 은 뜻이 없다.
        paintStroke(ctx, style);
        break;
      }
      case 'path': {
        // 상자는 **한 번만** 잰다 — 투영은 `projectPathPoints` 의 입력이며, 경로가
        // 스테이지를 다시 재는 자리는 없다(불변식 J3).
        const box = projectBox(el.geometry, proj);
        ctx.beginPath();
        for (const cmd of projectPathPoints(el.path, box)) {
          switch (cmd.c) {
            case 'M':
              ctx.moveTo(cmd.x, cmd.y);
              break;
            case 'L':
              ctx.lineTo(cmd.x, cmd.y);
              break;
            case 'C':
              ctx.bezierCurveTo(cmd.x1, cmd.y1, cmd.x2, cmd.y2, cmd.x, cmd.y);
              break;
            case 'Z':
              ctx.closePath();
              break;
          }
        }
        // 색 없는 요소를 그리지 않는 규율도, 두께 0 을 선 없음으로 읽는 규율도 그대로
        // 걸린다 — 이 둘은 **한 글자도 고치지 않는다**.
        paintFill(ctx, style);
        paintStroke(ctx, style);
        break;
      }
      case 'text': {
        measured = paintText(
          ctx,
          text,
          projectPoint(el.geometry, proj),
          style,
          style.textColor ?? style.fill,
        );
        break;
      }
    }
    // 도형에 붙은 라벨(REQ-02). text 요소는 위에서 이미 그렸다.
    if (el.kind !== 'text') {
      paintText(ctx, text, labelAnchor(el, proj), style, style.textColor);
    }
  } catch {
    // 손상 요소 하나가 프레임 전체를 무너뜨리지 않는다(REQ-05). 재다 만 폭은 버린다 —
    // 그 프레임의 히트 상자는 폴백으로 떨어지고, 다음 프레임이 성공하면 되돌아온다.
  } finally {
    ctx.restore();
  }
  return measured;
}

/**
 * 요소 목록을 **배열 순서대로** 그린다(뒤가 위, REQ-02). 돌려주는 값은 이 프레임에서
 * 실제로 잰 **글자 폭 장부**(`kind:'text'` 요소 id → CSS px)다.
 *
 * 스타일·문구 맵에 항목이 없으면 요소의 기본값으로 떨어진다 — 바인딩 시리즈가 없거나
 * 규칙이 하나도 일치하지 않는 경우가 정상 경로이기 때문이다(AC-E2/AC-E3).
 *
 * **반환 타입이 002 가 이 파일에 더한 변경의 전부다**(SPEC-CANVAS-002 T2). 인자도,
 * `DrawContext2D` 도, 그리기 순서도, 그려진 결과도 001 그대로이며, 반환값을 무시하는 기존
 * 호출부는 아무 영향을 받지 않는다(AC-E1).
 *
 * 장부에 담기는 것과 담기지 않는 것:
 * - 담긴다 — `kind:'text'` 요소가 실제로 그려져 `measureText` 를 지난 경우. 폭이 0 으로
 *   측정되어도 그대로 담는다. "재어 보니 0" 과 "잰 적 없음" 은 다른 사실이고, 이 장부는
 *   사실만 나른다.
 * - 담기지 않는다 — 도형에 붙은 **라벨**의 폭(라벨은 자기 기하가 없어 히트 영역도 핸들도
 *   없다), `visible:false` 로 아예 그리지 않은 요소, 문구나 색이 없어 그리지 않은 요소.
 *   담기지 않은 요소는 히트 테스트가 기준점 둘레 여유 상자로 폴백한다(AC-E7).
 *
 * 측정 횟수는 001 과 같은 **프레임당 1회**다. 이 함수는 새로 재지 않고 이미 잰 값을 모으기만
 * 하므로 "스테이지의 두 번째 측정원을 만들지 않는다" 는 REQ-05 금지 조항이 지켜진다.
 */
export function drawElements(
  ctx: DrawContext2D,
  elements: readonly CanvasElement[],
  styles: Record<string, ResolvedStyle>,
  texts: Record<string, string | undefined>,
  proj: CanvasProjection,
): Record<string, number> {
  const textWidths: Record<string, number> = {};
  for (const el of elements) {
    const measured = drawMeasuredElement(
      ctx,
      el,
      styles[el.id] ?? el.style,
      texts[el.id] ?? el.text,
      proj,
    );
    if (measured !== undefined) textWidths[el.id] = measured;
  }
  return textWidths;
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
