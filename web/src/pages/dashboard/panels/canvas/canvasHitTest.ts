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
  type BoxGeometry,
  type CanvasElement,
  type PointGeometry,
} from './canvasConfig';
import {
  closestPointOnSegment,
  ellipseParams,
  projectBox,
  projectBoxIn,
  projectLine,
  projectLineIn,
  projectPathPoints,
  projectPoint,
  projectPointIn,
  resolveTextOrigin,
  type CanvasProjection,
  type PxBox,
  type PxLine,
  type PxPoint,
} from './canvasGeometry';
import { TEXT_BASELINE } from './drawElement';
// 각도의 판정과 점 회전은 잎 모듈이, **축**은 윤곽 모듈이 소유한다(SPEC-CANVAS-014 K1) —
// 그리는 쪽이 부르는 그 함수를 여기서도 부른다.
import { isRotated, rotatePoint } from './canvasRotation';
import { rotationPivotIn } from './canvasOutline';
import { frameKey } from './group/frameKey';
import { isGroup, type CanvasNode, type GroupElement } from './group/groupTypes';
import { connectorPath } from './connector/connectorPath';
import { isConnector, type ConnectorElement } from './connector/connectorTypes';
import { resolveConnector } from './connector/resolveConnector';
import {
  FLATTEN_TOLERANCE_PX,
  flattenPath,
  isInsidePath,
  type FlatSubpath,
} from './shapes/pathFlatten';

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

/**
 * 점–선분 거리. 두 끝점이 같은 퇴화 선분은 점 거리로 떨어진다.
 *
 * **자리는 여기서 셈하지 않는다**(SPEC-CANVAS-011 M10). 가장 가까운 자리를 내는 산술은
 * `canvasGeometry.closestPointOnSegment` 한 곳에 있고, 이 함수는 그 자리까지의 거리를 잴
 * 뿐이다 — 판정은 이 파일의 것이고 자리는 그 파일의 것이라는 머리말의 그 구분이다. 꺾임을
 * 끼워 넣는 쪽(M10)이 **같은 자리**를 새 점으로 쓰므로, 잡히는 자리와 점이 놓이는 자리가
 * 두 벌로 갈라질 수 없다(위험 R1).
 *
 * **내보내는 이유**(SPEC-CANVAS-011 M11): 자유선의 간소화가 점을 버릴지 정할 때 재는 것이
 * 바로 이 양이다. 그쪽이 제 손으로 같은 산술을 적으면 이 저장소에 점–선분 거리가 두 벌이
 * 되고, 둘이 갈라지는 날 "그은 대로 잡히지 않는" 자리가 열린다 — 머리말이 금지한 두 번째
 * 측정원이다. `closestPointOnSegment` 를 직접 부르는 자리는 여전히 **이름으로 적은 둘**
 * 뿐이므로(M10 의 허용목록) 이 문을 여는 값은 거기에 닿지 않는다.
 *
 * 인자 이름이 `Px…` 인 것은 **역사이지 제약이 아니다.** 이 산술에는 화면 고유의 것이 한
 * 줄도 없고(정사영을 구간에 가두는 일이 전부다), 캔버스 단위의 점을 넘겨도 같은 뜻의 값이
 * 나온다. 다만 두 자료형이 구조적으로 같아 타입이 공간 혼동을 잡아 주지 못하므로, 넘기는
 * 쪽이 제가 어느 공간에 있는지 주석으로 밝히는 것이 이 문의 대가다.
 */
export function distanceToSegment(line: PxLine, point: PxPoint): number {
  const on = closestPointOnSegment(line, point);
  return Math.hypot(point.x - on.x, point.y - on.y);
}

/**
 * 선에 점이 드는가. 임계는 `max(선 두께 / 2, 집기 여유)` 다 — 두께 1px 선은 여유 없이는
 * 누를 수 없고, 아주 두꺼운 선은 제 두께만큼은 잡혀야 그림과 손이 맞는다.
 */
function hitsLine(line: PxLine, point: PxPoint, strokeWidth: number, pad: number): boolean {
  return distanceToSegment(line, point) <= Math.max(strokeWidth / 2, pad);
}

/**
 * 평탄화한 경로에 점이 드는가 — **(안쪽인가) 또는 (어느 변까지의 거리 ≤ 임계)**.
 *
 * 임계는 선 판정과 **같은 식**이다(`max(두께/2, 여유)`) — 경로의 변도 결국 선이고, 두
 * 자가 갈라지면 "같은 두께인데 도형에 따라 다르게 잡힌다" 가 된다. 변 거리를 여기서 새로
 * 재지 않고 `distanceToSegment` 를 그대로 쓰는 것도 같은 이유다(두 번째 측정원 금지).
 *
 * 안쪽 판정과 변 판정을 **또는**으로 묶는 것이 이 함수의 전부다. 안쪽만 보면 두께 1px
 * 짜리 빈 별의 선을 누를 수 없고, 변만 보면 채워진 도형의 한가운데가 잡히지 않는다.
 *
 * 점이 하나뿐인 부분 경로는 변이 없으므로 그 점까지의 거리로 떨어진다 —
 * `distanceToSegment` 가 길이 0 선분에 대해 하는 그대로이며, 퇴화한 경로도 화면에서
 * 되찾을 수 있게 한다.
 */
function hitsPath(
  subpaths: readonly FlatSubpath[],
  point: PxPoint,
  strokeWidth: number,
  pad: number,
): boolean {
  if (isInsidePath(subpaths, point)) return true;
  return hitsEdges(subpaths, point, Math.max(strokeWidth / 2, pad));
}

/**
 * 평탄화한 부분 경로의 **어느 변까지의 거리**가 임계 안인가 — 위 `hitsPath` 의 뒷절반이자,
 * 연결선(SPEC-CANVAS-011 M7)이 쓰는 판정의 **전부**다.
 *
 * 따로 이름을 갖는 까닭은 연결선에 **안쪽이 없기** 때문이다. 연결선은 두 자리를 잇는 열린
 * 선이라 내부라는 개념이 없고, 그래서 `isInsidePath` 를 지나지 않는다. 그렇다고 변 순회를
 * 저쪽에 한 벌 더 적으면 이 파일 안에 자가 둘이 생긴다 — 머리말이 금지한 그 "두 번째
 * 측정원" 이 바깥이 아니라 **안쪽**에 서는 꼴이다. 그러니 나누되 **복사하지 않는다.**
 */
function hitsEdges(
  subpaths: readonly FlatSubpath[],
  point: PxPoint,
  threshold: number,
): boolean {
  for (const sub of subpaths) {
    const { points, closed } = sub;
    if (points.length === 1) {
      const only = points[0];
      if (only !== undefined && Math.hypot(point.x - only.x, point.y - only.y) <= threshold) {
        return true;
      }
      continue;
    }
    // 닫힌 부분 경로는 마지막 점에서 첫 점으로 돌아오는 **닫힘 변**을 하나 더 갖는다.
    // 그 변을 빠뜨리면 별의 마지막 한 변만 잡히지 않는 결함이 되고, 그것은 화면에서만
    // 드러난다(`closePath` 를 인터페이스에 들인 것과 같은 부류의 자리다).
    const edges = closed ? points.length : points.length - 1;
    for (let i = 0; i < edges; i++) {
      const a = points[i];
      const b = points[(i + 1) % points.length];
      if (a === undefined || b === undefined) continue;
      if (distanceToSegment({ x1: a.x, y1: a.y, x2: b.x, y2: b.y }, point) <= threshold) {
        return true;
      }
    }
  }
  return false;
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
  key: string,
  host?: PxBox,
): boolean {
  // **투영을 고르는 자리는 이 모듈에서도 여기 셋뿐이다**(SPEC-CANVAS-004 M4). 부품이면
  // 그룹의 px 상자 안으로, 최상위 원소면 종전 그대로 스테이지로 간다 —
  // `drawElement.drawMeasuredElement` 가 같은 형상을 쓴다. 아래 판정 갈래는 어느 쪽인지
  // 알지 못하며, 그래서 **그린 자리와 잡히는 자리**가 두 벌로 갈라지지 않는다.
  const pxBox = (geo: BoxGeometry): PxBox =>
    host === undefined ? projectBox(geo, proj) : projectBoxIn(geo, host);
  const pxPoint = (geo: PointGeometry): PxPoint =>
    host === undefined ? projectPoint(geo, proj) : projectPointIn(geo, host);

  // **점을 되돌린다 — 도형마다 판정을 다시 쓰지 않는다**(SPEC-CANVAS-014 §결정 3).
  //
  // 돌아간 사각형·타원·경로를 잡는 판정을 종류마다 새로 쓰면 다섯 갈래가 열이 된다. 대신
  // 점을 요소의 **돌지 않은 좌표계**로 되돌린 뒤 아래 다섯을 **그대로** 부른다. 한 줄이
  // 늘고 다섯이 산다.
  //
  // 축은 그리는 쪽이 쓰는 그 상자에서 나온다(`drawElement.rotationPivot` 과 같은 식) —
  // 그래서 그려진 자리와 잡히는 자리가 **각도 하나**를 함께 본다(K1). 두 자리가 축을 따로
  // 구하면 그 등식이 깨지고, 그 어긋남은 각도가 0 일 때 보이지 않는다.
  const deg = 'rotation' in el ? el.rotation : undefined;
  const at = ((): PxPoint => {
    if (!isRotated(deg)) return point;
    const pivot = rotationPivotIn(el, pxBox, pxPoint, resolveMeasuredWidth(textWidths[key]));
    // 축이 없으면(선) 돌지 않은 것과 같다 — 필드가 서지 않는 종류다.
    return pivot === undefined ? point : rotatePoint(point, pivot, -(deg as number));
  })();

  switch (el.kind) {
    case 'rect':
      return hitsBox(pxBox(el.geometry), at, HIT_TOLERANCE_PX);
    case 'ellipse':
      return hitsEllipse(pxBox(el.geometry), at, HIT_TOLERANCE_PX);
    case 'line':
      return hitsLine(
        host === undefined ? projectLine(el.geometry, proj) : projectLineIn(el.geometry, host),
        at,
        resolveStrokeWidth(el.style.strokeWidth),
        HIT_TOLERANCE_PX,
      );
    case 'text': {
      // 좌측 끝 원점은 렌더가 쓰는 그 함수에서 나온다(정렬이 두 벌이 되지 않는다).
      // 폭은 **프레임 키**로 찾는다 — 부품이면 `그룹id/부품id` 다. 평평한 `el.id` 로
      // 찾으면 폭이 없어 기준점 둘레 여유 상자로 조용히 폴백한다(§프레임 키 넷째 표면).
      const width = resolveMeasuredWidth(textWidths[key]);
      const fontSize = resolveFontSize(el.style.fontSize);
      const origin = resolveTextOrigin(
        host === undefined ? projectPoint(el.geometry, proj) : projectPointIn(el.geometry, host),
        el.style.align ?? 'left',
        width,
      );
      const box: PxBox = {
        x: origin.x,
        y: origin.y - fontSize * TEXT_BOX_TOP_RATIO[TEXT_BASELINE],
        w: width,
        h: fontSize,
      };
      return hitsBox(box, at, HIT_TOLERANCE_PX);
    }
    case 'path': {
      // **상자는 여기서 한 번만 잰다.** 그 상자가 곧 `projectPathPoints` 의 입력이며,
      // 렌더가 쓰는 것과 **같은 함수의 같은 결과**다 — 그래서 그린 자리와 잡히는 자리가
      // 갈라질 수 없다. 갈라지는 것은 마지막 한 걸음(곡선을 그대로 그릴 것인가, 잘게
      // 나눌 것인가)뿐이고, 그 어긋남은 평탄화 허용 오차만큼이며 집기 여유의 1/12 이다.
      //
      // **바운딩 박스로 두지 않는다**(REQ-07). 위 `hitsEllipse` 가 이미 적어 둔 이유가
      // 그대로 걸린다 — 별의 오목한 사이, 십자의 겨드랑이는 상자 안이지만 도형 밖이고,
      // 거기서 잡히면 겹쳐 놓은 요소의 선택이 눈에 보이는 그림과 어긋난다.
      // 부품 경로는 **두 겹의 로컬 격자를 합성**한다 — 바깥 겹이 상자를 그룹 안으로
      // 옮기고, 안쪽 겹(`projectPathPoints`)이 명령을 그 상자 안으로 옮긴다. 두 격자는
      // 합성될 뿐 섞이지 않으며, 그래서 나눗셈 자리가 늘지 않는다(불변식 G2 · 008 J3).
      const box = pxBox(el.geometry);
      const subpaths = flattenPath(projectPathPoints(el.path, box), FLATTEN_TOLERANCE_PX);
      return hitsPath(
        subpaths,
        at,
        resolveStrokeWidth(el.style.strokeWidth),
        HIT_TOLERANCE_PX,
      );
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
 * SPEC-CANVAS-004 M4 — 원소 타입이 `CanvasNode` 로 넓어졌다. 그룹은 **제 상자가 아니라
 * 부품으로** 판정되며(`hitsGroup`), 맞으면 `partId` 가 채워진다. 호출부의 시그니처는
 * 한 글자도 바뀌지 않는다 — 002 가 `CanvasHit` 를 레코드로 두고 `partId` 자리를 비워 둔
 * 것이 그것을 위해서였다.
 *
 * @param elements 배열 순서가 z-order 다(뒤가 위). 그룹 안 부품 배열도 같은 규칙이다.
 * @param point 스테이지 로컬 CSS px 지점.
 * @param proj 표면이 잰 스테이지 크기 + config 의 캔버스 크기. 이 모듈은 **스스로 재지 않는다**.
 * @param textWidths 렌더 층이 잰 글자 폭(**프레임 키** → px). 부품은 `그룹id/부품id` 로
 *   찾는다. 없는 항목은 폴백된다(AC-E7).
 */
export function hitTest(
  elements: readonly CanvasNode[],
  point: PxPoint,
  proj: CanvasProjection,
  textWidths: Readonly<Record<string, number>>,
): CanvasHit | undefined {
  // 손상된 포인터 좌표 방어. 비유한 좌표로 판정하면 모든 비교가 false 가 되어 "아무것도
  // 맞지 않음" 과 구분되지 않으므로, 들어오는 자리에서 한 번에 끊는다.
  if (!Number.isFinite(point.x) || !Number.isFinite(point.y)) return undefined;
  for (const node of [...elements].reverse()) {
    // 연결선은 **잉크와의 거리**로 잡힌다(SPEC-CANVAS-011 REQ-07-b). 갈래가 여기 먼저 서는
    // 것은 `node.style` 이 **없을 수 있는** 유일한 노드이기 때문이다 — 아래 가시성 판정에
    // 닿으면 그 자리에서 던진다. 가시성도 두께도 `hitsConnector` 안에서 본다.
    if (isConnector(node)) {
      if (hitsConnector(node, point, elements, proj, textWidths)) return { nodeId: node.id };
      continue;
    }
    if (isGroup(node)) {
      const hit = hitsGroup(node, point, proj, textWidths);
      if (hit !== undefined) return hit;
      continue;
    }
    if (node.style.visible === false) continue;
    if (hitsElement(node, point, proj, textWidths, frameKey(node.id))) return { nodeId: node.id };
  }
  return undefined;
}

/**
 * 그룹은 **제 상자가 아니라 부품으로** 잡힌다(REQ-08 · AC-E11).
 *
 * **상자로 잡지 않는 이유는 이 파일이 이미 적어 두었다.** `hitsEllipse`·`hitsPath` 의 근거가
 * 그대로 걸린다 — 별의 오목한 사이, 십자의 겨드랑이는 상자 안이지만 도형 밖이고, 거기서
 * 잡히면 겹쳐 놓은 요소의 선택이 눈에 보이는 그림과 어긋난다. 밸브 심볼의 바깥 상자는
 * **거의 전부 빈 공간**이므로, 상자로 잡으면 그 빈 공간에서 뒤에 놓인 요소가 영원히
 * 잡히지 않는다.
 *
 * 값으로 치르는 대가는 하나이고 숨기지 않는다: 히트 비용이 `O(최상위 수)` 에서
 * `O(최상위 수 + 부품 총수)` 로 늘어난다. 여전히 **포인터 사건당 1회** 선형이고 프레임당이
 * 아니므로(002 가정 A10) 받아들인다.
 *
 * 부품 순회는 **역순**이다 — 부품 배열 순서가 곧 그리기 순서(뒤가 위)이므로 역순의 첫
 * 일치가 눈에 보이는 맨 위다. 입력 배열을 뒤집지 않고 사본을 뒤집는 것도 최상위 순회와
 * 같은 이유다.
 *
 * **그룹 자신의 `style.visible` 은 보지 않는다.** M3 의 `drawElements` 도 보지 않기 때문이며,
 * 여기서만 보면 "그려지는데 잡히지 않는" 어긋남이 생긴다. 그룹 겉모습이 부품으로 내려오는
 * 것은 캐스케이드(M8·M9)의 몫이고, 그때는 **그리는 쪽과 잡는 쪽이 함께** 그 값을 본다.
 */
function hitsGroup(
  group: GroupElement,
  point: PxPoint,
  proj: CanvasProjection,
  textWidths: Readonly<Record<string, number>>,
): CanvasHit | undefined {
  // 상자는 **그룹마다 한 번만** 잰다. 부품마다 다시 재도 값은 같지만(순수 함수) 부품 수만큼
  // 같은 계산을 되풀이한다.
  const box = projectBox(group.geometry, proj);
  // **돌아간 그룹은 점을 한 번 더 되돌린다**(SPEC-CANVAS-014 M5).
  //
  // 그리는 쪽은 그룹의 각도로 부품 묶음 **전체**를 감싼다(`drawElements` §돌아간 그룹).
  // 그래서 잡는 쪽도 부품을 보기 **전에** 그 각도를 되돌려야 하고, 축은 그리는 쪽이 쓰는
  // 그 상자의 가운데다 — 상자를 여기서 한 번만 재는 성질이 그 등식을 그대로 살린다.
  //
  // 부품 자신의 각도는 아래 `hitsElement` 가 제 축으로 다시 되돌린다. 두 겹이 **합성될 뿐
  // 섞이지 않는 것**은 그리는 쪽의 두 `save` 가 겹치는 것과 같은 모양이다.
  const at = isRotated(group.rotation)
    ? rotatePoint(
        point,
        { x: box.x + box.w / 2, y: box.y + box.h / 2 },
        -(group.rotation as number),
      )
    : point;
  for (const part of [...group.parts].reverse()) {
    if (part.style.visible === false) continue;
    if (hitsElement(part, at, proj, textWidths, frameKey(group.id, part.id), box)) {
      // **선택 키는 여전히 `nodeId` 하나다**(002 의 규칙 불변). `partId` 는 목록 편집기가
      // 그 부품 행을 먼저 펼치는 데에만 쓰인다.
      return { nodeId: group.id, partId: part.id };
    }
  }
  return undefined;
}

// --- 연결선 판정 (SPEC-CANVAS-011 M7) -------------------------------------

/**
 * 연결선에 점이 드는가 — **잉크와의 거리**다(REQ-07-b · AC-53).
 *
 * ## 상자 판정이 **한 줄도 없다**
 *
 * 빠른 걸러내기로도 두지 않는다. 크게 꺾인 연결선의 윤곽 상자는 **거의 전부 빈 공간**이라
 * (AC-54 가 재는 그 자리) 걸러내기가 실제로 걸러 주는 것이 거의 없고, 대신 다음 사람에게
 * "여기 상자 판정이 이미 있다" 는 발판을 남긴다. 그 발판 위에서 걸러내기가 판정으로 자라는
 * 것이 이 파일이 `hitsEllipse`·`hitsPath`·`hitsGroup` 세 자리에 걸쳐 막아 온 그 결함이다.
 *
 * ## 그린 곡선과 **같은 곡선**을 잡는다
 *
 * 점 목록은 M5 의 `resolveConnector` 에서, 그 점들이 이루는 모양은 M6 과 **같은**
 * `connectorPath` 에서 온다. 잡는 쪽이 제 손으로 참조를 풀거나 제 손으로 곡선을 지으면
 * "그려진 자리와 잡히는 자리가 다르다" 가 시작된다(002 위험 R1). 곡선은 008 의
 * `flattenPath` 로 폴리라인이 되고 그 다음은 요소가 쓰는 그 변 거리 판정을 그대로 지난다 —
 * **베지어 거리 산술을 새로 적지 않는다**(AC-55).
 *
 * `isInsidePath` 는 부르지 않는다. 연결선은 열린 선이라 안쪽이라는 개념이 없고,
 * `flattenPath` 도 `closed:false` 로 낸다 — 불러도 늘 거짓인 판정을 두느니 갈래를 두지
 * 않는다(위 `hitsEdges` 가 그래서 따로 섰다).
 *
 * ## 끊긴 연결은 **잡히지 않는다** (AC-56)
 *
 * `resolveConnector` 가 `undefined` 를 내면 그대로 거짓이다. 그리는 쪽이 아무것도 그리지
 * 않았으므로(AC-52) 잡을 잉크도 없다 — 없는 선이 잡히면 사용자는 **보이지 않는 것**을
 * 손에 쥔다. 던지지도 않는다.
 *
 * ## `visible:false` 와 두께는 **저술된 값**으로 본다
 *
 * 그리지 않는 것은 잡히지 않는다 — 요소가 `node.style.visible` 로 지켜 온 그 규율이고,
 * `drawConnector` 가 그 값에서 바로 되돌아가는 그 값이다. 규칙 캐스케이드가 덮은 결과가
 * 아니라 **저술된** 값을 보는 것도 요소와 같다: 이 모듈은 스타일 맵을 받지 않으며, 받게
 * 두면 히트가 프레임마다 달라져 "가끔 안 잡힌다" 가 된다.
 *
 * 두께의 임계도 `line` 과 **같은 식**이다(`max(두께/2, 여유)`) — 같은 두께인데 종류에 따라
 * 다르게 잡히면 손과 그림이 어긋난다.
 */
function hitsConnector(
  connector: ConnectorElement,
  point: PxPoint,
  nodes: readonly CanvasNode[],
  proj: CanvasProjection,
  textWidths: Readonly<Record<string, number>>,
): boolean {
  if (connector.style?.visible === false) return false;
  const points = resolveConnector(connector, nodes, proj, textWidths);
  if (points === undefined) return false;
  const subpaths = flattenPath(connectorPath(points, connector.route, proj), FLATTEN_TOLERANCE_PX);
  const threshold = Math.max(
    resolveStrokeWidth(connector.style?.strokeWidth) / 2,
    HIT_TOLERANCE_PX,
  );
  return hitsEdges(subpaths, point, threshold);
}
