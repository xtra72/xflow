// 캔버스 포인터 히트 테스트 (SPEC-CANVAS-002 T1).
//
// **역산하지 않는다.** 001 은 "요소 선택은 좌표 역산 히트 테스트다" 라고 적어 두었지만,
// 002 는 그 문제를 반대 방향으로 푼다 — 요소를 `canvasGeometry` 의 **기존 투영 함수로
// 순방향 투영**한 뒤, 스테이지 로컬 CSS px 공간에서 점을 판정한다. 도형별 역산 함수는
// 이 파일에 하나도 없다(`canvasGeometry.unprojectPoint` 는 **자리**를 되돌리는 산술이지
// 판정이 아니다 — 그 구분은 그 모듈의 §역투영 주석에 있다).
//
// 그렇게 정한 이유는 하나다: **집기 여유는 화면 양이다.** `HIT_TOLERANCE_PX` 는 패널이
// 크든 작든 6px 이어야 한다. 캔버스 단위 공간에서 여유를 주면 같은 숫자가 큰 패널에서는
// 헐거워지고 작은 패널에서는 잡히지 않는다. 부수 효과로 투영 구현이 하나로 유지된다 —
// 렌더·히트·핸들이 **같은 함수**를 쓰므로 셋이 갈라질 수 없다(REQ-05 의 "두 번째 측정원을
// 만들지 않는다" 와 같은 규율).
//
// **DOM 무의존이다.** `document`/`window`/`CanvasRenderingContext2D`/`measureText` 를
// 참조하지 않는다. 글자 폭은 `canvasGeometry` 와 같은 축으로 **인자로 받는다**(렌더 층이
// 프레임마다 이미 재고 있는 값이며, 두 번째 측정원을 만들지 않기 위한 결정이다).
//
// @spec SPEC-CANVAS-002

import {
  DEFAULT_FONT_SIZE,
  DEFAULT_STROKE_WIDTH,
  type CanvasElement,
} from './canvasConfig';
import {
  ellipseParams,
  projectBox,
  projectLine,
  projectPoint,
  resolveTextOrigin,
  type CanvasProjection,
  type PxBox,
  type PxLine,
  type PxPoint,
} from './canvasGeometry';
import { TEXT_BASELINE } from './drawElement';

// --- 타입 ---------------------------------------------------------------

/**
 * 히트 결과. **벌거벗은 문자열이 아니라 레코드다**(REQ-06 · AC-E8).
 *
 * 002 는 `partId` 를 **절대 채우지 않는다**. 이 자리는 SPEC-CANVAS-004 가 `group` 노드의
 * 부품을 지목할 때 채울 몫이며, 지금 레코드 형상을 잡아 두면 004 가 호출부의 시그니처를
 * 바꾸지 않아도 된다. 선택 키는 언제나 **최상위 배열 원소의 id**(`nodeId`)다.
 */
export interface CanvasHit {
  nodeId: string;
  partId?: string;
}

// --- 상수 ---------------------------------------------------------------

/**
 * 집기 여유(CSS px).
 *
 * 두께 1px 선은 이 여유가 없으면 사실상 누를 수 없다. 정규화 공간이 아니라 **화면 공간**의
 * 값이라는 점이 이 모듈이 순방향 투영을 택한 이유 그 자체다(파일 머리말).
 */
export const HIT_TOLERANCE_PX = 6;

/**
 * 기준선 이름 → 글자 상자 **상단**이 기준점보다 얼마나 위에 있는지(글자 크기 배수).
 *
 * 이 모듈에서 가장 틀리기 쉬운 지점이다(SPEC 위험 R8). `drawElement.TEXT_BASELINE` 이
 * `'middle'` 이므로 기준점 y 는 상자의 **세로 중심**이고 비율은 0.5 다. 상단으로 착각하면
 * 글자 위쪽 절반을 눌러도 잡히지 않고, 글자 아래 빈 곳을 누르면 잡힌다.
 *
 * 표를 총망라로 둔 것은 기준선 상수가 바뀌었을 때 이 모듈이 **조용히** 어긋나지 않게 하기
 * 위해서다. `'middle'` 밖의 값은 한 줄 상자를 `fontSize` 높이로 본 거친 근사이며(폰트
 * 메트릭 없이는 정확할 수 없다), 실제로 쓰이는 값은 지금 `'middle'` 하나뿐이다.
 */
const TEXT_BOX_TOP_RATIO: Record<CanvasTextBaseline, number> = {
  top: 0,
  hanging: 0,
  middle: 0.5,
  alphabetic: 1,
  ideographic: 1,
  bottom: 1,
};

// --- 수치 도우미 --------------------------------------------------------

/**
 * 유효한 선 두께(px). `drawElement.resolveStrokeWidth` 와 같은 판정이다 — 그리는 두께와
 * 잡는 두께가 갈라지면 "보이는 것보다 두껍게/얇게 잡힌다" 는 결함이 된다.
 */
function resolveStrokeWidth(width: number | undefined): number {
  if (width === undefined) return DEFAULT_STROKE_WIDTH;
  return Number.isFinite(width) && width > 0 ? width : 0;
}

/** 유효한 글자 크기(px). 미지정·손상 값은 기본 크기다(`drawElement.fontSpec` 과 같은 판정). */
function resolveFontSize(size: number | undefined): number {
  if (size === undefined) return DEFAULT_FONT_SIZE;
  return Number.isFinite(size) && size > 0 ? size : DEFAULT_FONT_SIZE;
}

/**
 * 실측 글자 폭. 아직 한 프레임도 그려지지 않았거나(AC-E7) 측정이 손상되었으면 **0** 이다.
 * 0 이 되어도 상자가 사라지지 않는다 — 아래 `hitsBox` 가 여유만큼 부풀리므로 기준점 둘레의
 * 여유 크기 상자로 자연히 폴백한다. 폭 0 짜리 잡을 수 없는 상자를 만들지 않기 위한 축이다.
 */
function resolveMeasuredWidth(width: number | undefined): number {
  return width !== undefined && Number.isFinite(width) && width > 0 ? width : 0;
}

// --- 도형별 판정 --------------------------------------------------------

/**
 * 박스에 점이 드는가. 음수 크기 박스를 **양수 범위로 정규화**한 뒤 사방으로 여유만큼
 * 부풀린다. 001 이 음수 크기를 그릴 수 있게 해 둔 것은 읽기 경로의 견고성이므로, 잡는
 * 쪽도 같은 박스를 잡아야 한다.
 *
 * 부풀리기가 퇴화 처리를 겸한다 — 폭·높이가 0 이어도 여유 크기 상자가 남아 화면에서
 * 되살릴 수 없는 요소가 생기지 않는다.
 */
function hitsBox(box: PxBox, point: PxPoint, pad: number): boolean {
  const left = Math.min(box.x, box.x + box.w) - pad;
  const right = Math.max(box.x, box.x + box.w) + pad;
  const top = Math.min(box.y, box.y + box.h) - pad;
  const bottom = Math.max(box.y, box.y + box.h) + pad;
  return point.x >= left && point.x <= right && point.y >= top && point.y <= bottom;
}

/**
 * 타원 방정식 판정. **바운딩 박스가 아니다** — 박스 모서리의 빈 곳을 눌러도 잡히면
 * 겹쳐 놓은 요소의 선택이 눈에 보이는 그림과 어긋난다.
 *
 * 반지름에 각각 여유를 더하므로 두 반지름은 언제나 여유 이상이며, 따라서 0 으로 나누는
 * 일이 없다. 반지름 0 인 퇴화 타원은 여유 반지름의 점 판정이 된다.
 */
function hitsEllipse(box: PxBox, point: PxPoint, pad: number): boolean {
  const { cx, cy, rx, ry } = ellipseParams(box);
  const nx = (point.x - cx) / (rx + pad);
  const ny = (point.y - cy) / (ry + pad);
  return nx * nx + ny * ny <= 1;
}

/** 점–선분 거리. 두 끝점이 같은 퇴화 선분은 점 거리로 떨어진다. */
function distanceToSegment(line: PxLine, point: PxPoint): number {
  const dx = line.x2 - line.x1;
  const dy = line.y2 - line.y1;
  const lengthSq = dx * dx + dy * dy;
  if (lengthSq === 0) return Math.hypot(point.x - line.x1, point.y - line.y1);
  const raw = ((point.x - line.x1) * dx + (point.y - line.y1) * dy) / lengthSq;
  const t = raw < 0 ? 0 : raw > 1 ? 1 : raw;
  return Math.hypot(point.x - (line.x1 + t * dx), point.y - (line.y1 + t * dy));
}

/**
 * 선에 점이 드는가. 임계는 `max(선 두께 / 2, 집기 여유)` 다 — 두께 1px 선은 여유 없이는
 * 누를 수 없고, 아주 두꺼운 선은 제 두께만큼은 잡혀야 그림과 손이 맞는다.
 */
function hitsLine(line: PxLine, point: PxPoint, strokeWidth: number, pad: number): boolean {
  return distanceToSegment(line, point) <= Math.max(strokeWidth / 2, pad);
}

/**
 * 요소 하나에 점이 드는가.
 *
 * 라벨은 여기에 **참여하지 않는다.** 도형의 라벨은 `labelAnchor` 에서 파생되는 위치를
 * 가질 뿐 자기 기하가 없어서, 따로 고를 수 있게 만들면 사용자는 고를 수는 있으나 옮길 수
 * 없는 것을 손에 쥐게 된다. 라벨 위를 눌러도 그 도형이 선택된다. `kind:'text'` 는 글자가
 * 곧 기하이므로 예외가 아니다.
 */
function hitsElement(
  el: CanvasElement,
  point: PxPoint,
  proj: CanvasProjection,
  textWidths: Readonly<Record<string, number>>,
): boolean {
  switch (el.kind) {
    case 'rect':
      return hitsBox(projectBox(el.geometry, proj), point, HIT_TOLERANCE_PX);
    case 'ellipse':
      return hitsEllipse(projectBox(el.geometry, proj), point, HIT_TOLERANCE_PX);
    case 'line':
      return hitsLine(
        projectLine(el.geometry, proj),
        point,
        resolveStrokeWidth(el.style.strokeWidth),
        HIT_TOLERANCE_PX,
      );
    case 'text': {
      // 좌측 끝 원점은 렌더가 쓰는 그 함수에서 나온다(정렬이 두 벌이 되지 않는다).
      const width = resolveMeasuredWidth(textWidths[el.id]);
      const fontSize = resolveFontSize(el.style.fontSize);
      const origin = resolveTextOrigin(projectPoint(el.geometry, proj), el.style.align ?? 'left', width);
      const box: PxBox = {
        x: origin.x,
        y: origin.y - fontSize * TEXT_BOX_TOP_RATIO[TEXT_BASELINE],
        w: width,
        h: fontSize,
      };
      return hitsBox(box, point, HIT_TOLERANCE_PX);
    }
  }
}

// --- 진입점 -------------------------------------------------------------

/**
 * 스테이지 로컬 CSS px 지점에 맞는 **최상위** 요소 하나를 고른다.
 *
 * - 순회는 **배열 역순**이다. 배열 순서 = 그리기 순서(뒤가 위)이고 그것이 001 의 유일한
 *   z-order 수단이므로, 역순의 첫 일치가 곧 최상위다(REQ-02 · AC-01).
 * - `style.visible === false` 인 요소는 건너뛴다 — 보이지 않는 것을 잡을 수는 없다.
 * - 맞는 것이 없으면 `undefined` 다. 호출부는 이때 선택만 비우고 **이벤트를 소비하지
 *   않는다**(REQ-05 · AC-E3) — 그 판단은 오버레이의 몫이며 이 모듈은 사실만 돌려준다.
 * - 순수하다. 인자를 바꾸지 않으며 역순 순회도 복사본을 뒤집는다(입력 배열을 뒤집으면
 *   호출부의 z-order 가 조용히 바뀐다).
 * - 비용은 프레임당이 아니라 **포인터 이벤트당 1회** 선형 순회다(가정 A10 · 위험 R9).
 *   공간 색인은 도입하지 않는다.
 *
 * @param elements 배열 순서가 z-order 다(뒤가 위).
 * @param point 스테이지 로컬 CSS px 지점.
 * @param proj 표면이 잰 스테이지 크기 + config 의 캔버스 크기. 이 모듈은 **스스로 재지 않는다**.
 * @param textWidths 렌더 층이 잰 글자 폭(요소 id → px). 없는 항목은 폴백된다(AC-E7).
 */
export function hitTest(
  elements: readonly CanvasElement[],
  point: PxPoint,
  proj: CanvasProjection,
  textWidths: Readonly<Record<string, number>>,
): CanvasHit | undefined {
  // 손상된 포인터 좌표 방어. 비유한 좌표로 판정하면 모든 비교가 false 가 되어 "아무것도
  // 맞지 않음" 과 구분되지 않으므로, 들어오는 자리에서 한 번에 끊는다.
  if (!Number.isFinite(point.x) || !Number.isFinite(point.y)) return undefined;
  for (const el of [...elements].reverse()) {
    if (el.style.visible === false) continue;
    if (hitsElement(el, point, proj, textWidths)) return { nodeId: el.id };
  }
  return undefined;
}
