// 캔버스 편집의 **배치 정돈** — 격자 붙임 · 정렬 · z-order (SPEC-CANVAS-002 T12 · T13 · T14).
//
// 셋을 한 모듈에 두는 이유는 편의가 아니다. 셋 다 **요소의 기하를 새로 발명하지 않고
// "어디에 놓일 차례인가" 만 정하는** 계산이며, 가운데 하나는 `panels/charts/panelEditAlign.ts`
// 의 순수 함수를 그대로 재사용하고 마지막 하나는 목록 편집기가 이미 쓰는 배열 순서 이동을
// 그대로 재사용한다. **재사용의 대가를 한 자리에 모아 두는 것**이 이 파일의 존재 이유다.
//
// ## 위험 R6 — 이 모듈이 존재하는 첫째 이유 (그리고 그 절반이 사라진 이유)
//
// `computeAlignPatches` 는 결과를 `clampPercentOffset(v, limit)` 로 죄고 그 `limit` 의
// **기본값이 `PANEL_OFFSET_LIMIT = 40`** 이다. 그 기본값을 그대로 두고 캔버스에서 부르면
// 조작이 **기준 상자의 40% 지점에 조용히 붙잡힌다** — 오류가 아니라 "왜 더 안 가지" 로만
// 보이는 종류의 결함이다. 캔버스의 기하는 **일부러 상한이 없다**(캔버스 밖 저술이
// 합법이다 — 가정 A5). 그래서 이 모듈은 상한으로 언제나 `Number.POSITIVE_INFINITY` 를
// 넘긴다(`Math.min(Math.max(v, -Inf), Inf) === v` 이므로 무동작이다).
//
// **상한을 넘기는 자리가 하나여야 그 규율이 지켜진다.** 그래서 `panelEditAlign` 을 들이는
// 파일은 `panels/canvas/` 안에서 **이 파일 하나뿐**이며, 그 형상 자체를 이웃한 테스트가
// 가드로 막는다(`canvasEditArrange.test.ts` §바이패스 금지). 원 함수를 직접 부를 수 있는
// 자리가 남아 있으면 규율은 주석일 뿐이고, 주석은 다음 사람이 읽지 않는다.
//
// ### 격자 붙임은 그 함수를 더 이상 부르지 않는다 (0.8.0)
//
// 좌표가 정수 캔버스 단위가 되면서 붙임은 **정수 반올림 한 줄**이 되었다:
// `round(자리 / 간격) × 간격`. `snapOffsetToGrid` 는 그 계산을 "기준 상자에 대한 백분율"
// 공간에서 하고 결과를 `clampPercentOffset` 으로 죄는 함수라, 이제 그것을 부르려면
// 단위 → 백분율 → 단위로 두 번 환산해야 하고 그 왕복이 하는 일은 **±40 함정을 다시
// 들여오는 것**뿐이다. 그래서 붙임에서는 그 함수를 걷어냈다(아래 `snapDelta`).
//
// 걷어낸 자리에 가드를 잃지 않는다. §바이패스 금지 시험이 지키는 성질은 "원 모듈을
// 들이는 파일이 하나" 이고, 그 성질은 **정렬이 여전히 `computeAlignPatches` 를 부르므로
// 그대로 참**이다 — 무동작이 된 시험이 아니라 계속 무언가를 지키는 시험이다. 아울러
// 붙임이 그 함수를 다시 부르지 않는다는 사실 자체도 시험으로 못박는다(§격자 붙임은
// 백분율 공간을 지나지 않는다).
//
// ## 재사용의 대가 (정렬에만 남은 것들)
//
// 1. **상한 무한대** — 위 R6.
// 2. **`base.offset` 은 언제나 0 이다.** 그 모듈은 "흐름상 시작 자리 + 오프셋" 모델이라
//    잡는 순간의 오프셋을 되짚는다. 캔버스에는 흐름이 없고 **저장된 값이 곧 절대 자리**
//    이므로 되짚을 오프셋이 없다.
//
// 정렬의 변환 사슬은 **투영된 px 상자 → 백분율 오프셋 → ÷100 × 캔버스 축 길이 → 캔버스
// 단위 델타**다. 백분율의 분모가 스테이지이고 스테이지가 곧 캔버스 전체의 투영이므로,
// 백분율에 캔버스 축 길이를 곱하면 그대로 단위 이동량이 된다.
//
// ## 격자 어휘는 하나다 — 그리고 이제 자투리가 없다
//
// 격자 간격은 **정수 캔버스 단위** 하나(`gridStep`)다. 그 한 값이 셋을 함께 정한다:
// 그려지는 격자(`gridPercents` 로 축마다 백분율 환산) · 드래그 붙임(`snapDelta`) ·
// Shift+방향키 한 칸(호출부가 그 값을 그대로 더한다). 뒤의 둘은 **환산 없이 그 정수를
// 그대로** 쓰므로 갈라질 여지가 아예 없고, 앞의 하나만 화면에 그리기 위해 백분율로
// 옮겨 적는다.
//
// 정규화 시절에는 이 자리에 `squareGridSteps` 가 있었다. 정사각 칸을 얻으려고 세로
// 백분율을 **스테이지 종횡비**에서 파생했고, 그 값(예: 1749×796 스테이지에서 21.97%)은
// 100 을 나누어떨어지지 않아 **마지막 줄이 반 칸**으로 잘렸다. 이제 칸 수는 스테이지가
// 아니라 캔버스 크기가 정한다 — 500×400 에 10 단위 격자면 **정확히 50 × 40 칸**이고,
// 패널을 어떤 크기로 늘여도 그 칸 수는 변하지 않는다.
//
// **DOM 무의존이다.** React 도 i18n 도 부르지 않으므로 jsdom 없이 전량 단위 테스트된다 —
// `canvasHitTest.ts` · `canvasEditGeometry.ts` · `canvasElementFactory.ts` 와 같은 규율이다.
//
// @spec SPEC-CANVAS-002 REQ-04 / REQ-06

import {
  computeAlignPatches,
  type AlignAxis,
  type AlignMode,
  type Box,
  type PanelElementBox,
} from '../charts/panelEditAlign';
import type { CanvasElement, CanvasSize } from './canvasConfig';
import type { CanvasBox, CanvasDelta, CanvasProjection, PxBox } from './canvasGeometry';

// --- 타입 ---------------------------------------------------------------

export type { AlignAxis, AlignMode };

/** 정렬에 참여하는 요소 하나 — **투영된 px 상자**를 든다(spec.md §스냅·정렬 재사용의 대가). */
export interface AlignTarget {
  /** 최상위 배열 원소의 id. 배열 위치(index)를 들고 다니지 않는다(REQ-06). */
  nodeId: string;
  /** 스테이지 로컬 CSS px 상자. 호출부가 `canvasGeometry` 의 투영으로 만든다. */
  box: PxBox;
}

/** 한 요소에 적용할 캔버스 단위 이동량. */
export interface AlignDelta {
  nodeId: string;
  delta: CanvasDelta;
}

// --- 상수 ---------------------------------------------------------------

/**
 * 캔버스가 `panelEditAlign` 에 넘기는 오프셋 상한 — **없음**.
 *
 * 이름을 값(`Infinity`)이 아니라 뜻으로 둔다. 호출부에서 `Number.POSITIVE_INFINITY` 를
 * 보면 "왜 무한대지" 를 다시 물어야 하지만 `NO_OFFSET_LIMIT` 은 스스로 답한다 —
 * 캔버스에는 상한이 없다(가정 A5 · 위험 R6).
 */
const NO_OFFSET_LIMIT = Number.POSITIVE_INFINITY;

/**
 * 격자 간격의 **기본값**(정수 캔버스 단위).
 *
 * 기본 캔버스(500 × 400)에서 **20 × 16 칸**이 나온다. 아래 목록의 가운데 값이며, 고를 수
 * 있는 넷 가운데 "칸이 뭉개지지도 성기지도 않은" 자리다 — 폭 250px 로 줄어든 패널에서도
 * 한 칸이 12.5px 이라 참조선 구실을 하고, 넓은 패널에서는 스무 칸이 눈으로 셀 수 있는
 * 밀도로 남는다.
 *
 * 정규화 시절의 기본값은 `panelEditAlign.GRID_STEP_PERCENT`(10%)를 그대로 다시 내보낸
 * 것이었다. 이제 단위가 백분율이 아니므로 그 값을 물려받을 수 없다 — 물려받는 시늉을 하면
 * "10" 이 두 화면에서 다른 것을 뜻하게 된다.
 */
export const CANVAS_GRID_STEP_UNITS = 25;

/**
 * 화면에서 고를 수 있는 격자 간격(정수 캔버스 단위) — **캔버스에만 있는 목록**이다.
 *
 * 값 넷을 고른 근거:
 *   - 넷 다 **100 의 약수**다. 기본 캔버스의 두 축이 100 의 배수(500 · 400)이므로, 어떤
 *     값을 골라도 **두 축이 나머지 없이 떨어진다** — 반 칸짜리 자투리가 생기지 않는다.
 *     이것이 사용 시험 "격자가 일정하지 않음" 에 대한 답 그 자체다.
 *   - 100 의 약수 가운데 1 · 2 · 4 · 5 는 기본 캔버스에서 100 칸을 넘겨 선이 뭉개지고,
 *     100 은 한 축에 다섯 칸뿐이라 붙일 자리가 사실상 없다. 남는 것이 이 넷이다.
 *   - 넷이 **2배 간격의 사다리**(10 → 20 → (25) → 50)라 옆 칸이 "두 배 성기게 / 두 배
 *     촘촘하게" 로 읽혀 설명이 필요 없다. 25 가 그 사다리에 끼어 있는 것은 기본 캔버스에서
 *     가장 쓸 만한 밀도(20 × 16)를 주기 때문이다.
 *
 * 사용자가 캔버스 크기를 100 의 배수가 아닌 값으로 정하면 마지막 칸이 잘릴 수 있다.
 * 그것은 이제 **사용자가 고른 두 정수에서 곧바로 따라 나오는 결과**이며 화면에서 예측·
 * 수정할 수 있다 — 스테이지 종횡비에서 파생되어 패널을 늘일 때마다 달라지던 옛 자투리와
 * 다른 종류의 일이다.
 *
 * 이 목록은 **런타임 표시 상태**일 뿐 config 스키마가 아니다(가정 A4) — 격자 토글·행
 * 펼침과 같은 부류이며, 저장되지 않는다.
 */
export const CANVAS_GRID_STEP_CHOICES: readonly number[] = [10, 20, 25, 50];

/**
 * 격자를 **화면에 그리기 위한** 두 축의 백분율.
 *
 * `PanelEditGrid` 는 선을 `repeating-linear-gradient` 로 그리고 그 간격은 제 축 길이에
 * 대한 백분율로만 말할 수 있다. 그래서 단위 하나를 축마다의 백분율로 옮겨 적는데, 그
 * 환산의 분모가 **스테이지가 아니라 캔버스**라는 점이 이 함수의 전부다:
 *
 *   - 분모가 캔버스이므로 백분율은 **패널 크기와 무관**하다. 패널을 늘여도 칸 수가 그대로다.
 *   - 간격이 캔버스 축을 나누어떨어지게 하면 백분율도 100 을 나누어떨어지게 한다.
 *     500 / 25 = 20 이고 100 / 5% = 20 이다 — 같은 사실을 두 번 말한 것뿐이다.
 *
 * 옛 `squareGridSteps` 는 세로 백분율을 **스테이지 종횡비**에서 파생했고, 그래서 칸 수가
 * 패널 크기에 따라 달라지며 대개 정수가 아니었다(반 칸 자투리의 출처다).
 *
 * 화면상 칸은 캔버스와 스테이지의 종횡비가 다르면 직사각형이 된다. 그것은 **일정한**
 * 직사각형이다 — 모든 칸이 같은 크기이고 자투리가 없다. 정사각형을 원하는 사용자에게는
 * 이제 답할 수단이 있다: 캔버스 크기를 패널 모양에 맞추면 된다.
 *
 * 잴 수 없는 값(비유한·비양수)에서는 간격을 **그대로** 돌려준다 — 0 으로 나눈 백분율은
 * 격자를 통째로 지운다.
 */
export function gridPercents(step: number, canvas: CanvasSize): GridPercents {
  if (!isFinite2(step, canvas.width, canvas.height)) return { x: step, y: step };
  if (!(canvas.width > 0) || !(canvas.height > 0)) return { x: step, y: step };
  return { x: (step / canvas.width) * 100, y: (step / canvas.height) * 100 };
}

/** 격자를 그릴 때 쓰는 두 축의 백분율 — **같은 단위 간격 하나**를 축마다 옮겨 적은 것이다. */
export interface GridPercents {
  /** 가로 간격 — 캔버스 **폭**에 대한 백분율. */
  x: number;
  /** 세로 간격 — 캔버스 **높이**에 대한 백분율. */
  y: number;
}

// --- 순수 도우미 ---------------------------------------------------------

/** 유한한 수만 통과시킨다. 포인터 좌표는 NaN/Infinity 가 될 수 있다(§품질 게이트 Secured). */
function isFinite2(...values: number[]): boolean {
  return values.every((v) => Number.isFinite(v));
}

// --- 격자 붙임 (T12) ------------------------------------------------------

/**
 * 한 축의 이동량을 격자에 맞춘다 — **정수 반올림 한 줄**이다.
 *
 * 맞추는 것은 요소의 **중심**이다(다른 패널의 `+` 표식과 같은 기준). 캔버스에서만
 * 좌상단을 맞추면 같은 대시보드 안에서 격자가 두 뜻을 갖는다.
 *
 * 죄지 않는다 — 상한이 없으므로 캔버스 밖으로 끌어도 붙잡히지 않는다(가정 A5 · 위험 R6).
 * 잴 수 없는 값에서는 손대지 않는다: 0 으로 죄면 격자를 켠 순간 요소가 얼어붙는다.
 *
 * 폭이 홀수인 요소의 중심은 반 단위에 있어 결과 좌표가 반 단위만큼 어긋날 수 있다.
 * 쓰기 통로가 정수로 반올림하므로 그 어긋남은 **최대 반 단위**이며(500 단위 캔버스에서
 * 0.1%), 중심 기준을 버리는 값보다 작다.
 */
function snapAxis(delta: number, center: number, step: number): number {
  if (!isFinite2(delta, center, step) || !(step > 0)) return delta;
  return Math.round((center + delta) / step) * step - center;
}

/**
 * 드래그 이동량을 격자에 맞춘다(REQ-04 · AC-E5).
 *
 * `anchor` 는 **잡는 순간** 기준 요소가 두르던 상자를 캔버스 단위로 되돌린 값이다.
 * 무리 이동이면 잡은 요소 하나가 기준이며 나머지는 같은 델타로 따라온다 — 무리의 각
 * 요소를 저마다 격자에 붙이면 무리가 끌려가는 동안 서로 흩어진다.
 *
 * **백분율 공간을 지나지 않는다.** 캔버스 단위와 격자 간격이 같은 공간의 정수이므로
 * 환산할 것이 없고, 환산하지 않으므로 `clampPercentOffset` 의 ±40 함정이 닿을 자리도
 * 없다(파일 머리말 §격자 붙임은 그 함수를 더 이상 부르지 않는다).
 *
 * `step` 은 **화면에 그려진 격자와 같은 값**이다 — 그리는 쪽은 이 정수를 `gridPercents`
 * 로 옮겨 적을 뿐이라 둘이 갈라질 자리가 없다.
 */
export function snapDelta(
  delta: CanvasDelta,
  anchor: CanvasBox,
  step: number = CANVAS_GRID_STEP_UNITS,
): CanvasDelta {
  return {
    dx: snapAxis(delta.dx, anchor.x + anchor.w / 2, step),
    dy: snapAxis(delta.dy, anchor.y + anchor.h / 2, step),
  };
}

// --- 정렬 (T13) -----------------------------------------------------------

/**
 * 고른 요소들을 서로 맞춘다(REQ-04 · AC-08). **원 함수를 부르는 유일한 자리 둘 중 하나다.**
 *
 * 변환 사슬은 **투영된 px 상자 → 백분율 오프셋 → ÷100 × 캔버스 축 길이 → 캔버스 단위
 * 델타**다. `offsetX/offsetY` 를 0 으로 넣기 때문에 돌려받은 오프셋이 곧 **이동량**이며,
 * 그래서 한 번 환산하는 것 말고 되짚을 것이 없다.
 *
 * 기준 상자는 스테이지다 — 스테이지가 곧 캔버스 전체의 투영이므로, 스테이지에 대한
 * 백분율에 캔버스 축 길이를 곱하면 그대로 단위 이동량이 된다. 요소들이 이루는 바깥
 * 상자에 맞추는 것은 `computeAlignPatches` 안의 규칙이며 여기서 다시 정하지 않는다.
 *
 * **2개 미만이면 빈 배열이다.** 맞출 상대가 없기 때문이며, 그 판정도 원 함수가 이미
 * 소유한다 — 여기서 다시 세면 두 곳이 갈라질 수 있다.
 */
export function alignDeltas(
  targets: readonly AlignTarget[],
  proj: CanvasProjection,
  axis: AlignAxis,
  mode: AlignMode,
): AlignDelta[] {
  const boxes: PanelElementBox<string>[] = targets.map((t) => ({
    // `kind` 는 그 모듈에서 "요소를 가리키는 문자열 키" 다 — 캔버스에서는 그것이 노드 id 다.
    kind: t.nodeId,
    rect: { left: t.box.x, top: t.box.y, width: t.box.w, height: t.box.h },
    // 캔버스에는 흐름이 없다. 지금 자리가 곧 절대 자리이므로 되짚을 오프셋이 0 이고,
    // 그래서 돌려받은 오프셋이 그대로 이동량이 된다.
    offsetX: 0,
    offsetY: 0,
    limit: NO_OFFSET_LIMIT,
  }));
  const bounds: Box = { left: 0, top: 0, width: proj.stage.width, height: proj.stage.height };
  return computeAlignPatches(boxes, bounds, axis, mode).map((patch) => ({
    nodeId: patch.kind,
    delta: {
      dx: ((patch.offsetX ?? 0) / 100) * proj.canvas.width,
      dy: ((patch.offsetY ?? 0) / 100) * proj.canvas.height,
    },
  }));
}

// --- z-order (T14) --------------------------------------------------------

/**
 * 요소 하나를 배열의 다른 자리로 옮긴다 — **목록 편집기의 위/아래 이동과 같은 연산**이다.
 *
 * 규칙은 `제거 후 삽입` 하나다. 목표 위치를 `idx ± 1` 로 주면 한 칸 이동이고, 마지막·0 으로
 * 주면 맨 앞·맨 뒤로 보내기다. 두 번째 정렬 규칙을 만들지 않기 위해 그 셋이 이 함수 하나를
 * 지난다(REQ-04).
 *
 * **아무것도 움직이지 않으면 받은 배열을 그대로 돌려준다**(같은 참조). 호출부가 그것으로
 * "쓸 일이 없다" 를 알아채 헛된 config 쓰기와 렌더 프레임을 만들지 않는다.
 *
 * 식별은 언제나 `nodeId` 다(REQ-06).
 */
export function moveElementTo(
  elements: readonly CanvasElement[],
  nodeId: string,
  targetIndex: number,
): readonly CanvasElement[] {
  const idx = elements.findIndex((el) => el.id === nodeId);
  if (idx < 0) return elements;
  const target = Math.max(0, Math.min(Math.trunc(targetIndex), elements.length - 1));
  if (target === idx) return elements;
  const next = elements.slice();
  // 인자가 먼저 평가된다 — 뽑아낸 뒤 넣으므로 목록 편집기의 `moveAt` 과 같은 순서다.
  next.splice(target, 0, ...next.splice(idx, 1));
  return next;
}

/**
 * 고른 것들을 **맨 앞(배열 끝 = 위)** 으로 보낸다.
 *
 * 배열 순서대로 훑으며 하나씩 끝으로 보내면 무리의 **상대 순서가 보존된다** — 무리를
 * 앞으로 꺼냈더니 자기들끼리 뒤섞이면 사용자는 그것을 되돌릴 방법을 화면에서 찾지 못한다.
 */
export function bringToFront(
  elements: readonly CanvasElement[],
  nodeIds: ReadonlySet<string>,
): readonly CanvasElement[] {
  let next = elements;
  for (const el of elements) {
    if (nodeIds.has(el.id)) next = moveElementTo(next, el.id, next.length - 1);
  }
  return next;
}

/**
 * 고른 것들을 **맨 뒤(배열 앞 = 아래)** 로 보낸다.
 *
 * 이쪽은 **뒤에서부터** 훑는다. 앞에서부터 0 으로 보내면 나중 것이 먼저 것을 계속 밀어내
 * 무리의 순서가 뒤집힌다.
 */
export function sendToBack(
  elements: readonly CanvasElement[],
  nodeIds: ReadonlySet<string>,
): readonly CanvasElement[] {
  let next = elements;
  for (let i = elements.length - 1; i >= 0; i -= 1) {
    const el = elements[i];
    if (el !== undefined && nodeIds.has(el.id)) next = moveElementTo(next, el.id, 0);
  }
  return next;
}
