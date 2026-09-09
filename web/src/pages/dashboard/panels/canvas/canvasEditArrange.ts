// 캔버스 편집의 **배치 정돈** — 격자 붙임 · 정렬 · z-order (SPEC-CANVAS-002 T12 · T13 · T14).
//
// 셋을 한 모듈에 두는 이유는 편의가 아니다. 셋 다 **요소의 기하를 새로 발명하지 않고
// "어디에 놓일 차례인가" 만 정하는** 계산이며, 앞의 둘은 `panels/charts/panelEditAlign.ts`
// 의 순수 함수를 그대로 재사용하고 마지막 하나는 목록 편집기가 이미 쓰는 배열 순서 이동을
// 그대로 재사용한다. **재사용의 대가를 한 자리에 모아 두는 것**이 이 파일의 존재 이유다.
//
// ## 위험 R6 — 이 모듈이 존재하는 첫째 이유
//
// `snapOffsetToGrid` 와 `computeAlignPatches` 는 결과를 `clampPercentOffset(v, limit)` 로
// 죄고 그 `limit` 의 **기본값이 `PANEL_OFFSET_LIMIT = 40`** 이다. 그 기본값을 그대로 두고
// 캔버스에서 부르면 드래그가 **스테이지의 40% 지점에 조용히 붙잡힌다** — 오류가 아니라
// "왜 더 안 가지" 로만 보이는 종류의 결함이다. 캔버스의 기하는 **일부러 상한이 없다**
// (스테이지 밖 저술이 합법이다 — 가정 A5). 그래서 이 모듈은 상한으로 언제나
// `Number.POSITIVE_INFINITY` 를 넘긴다(`Math.min(Math.max(v, -Inf), Inf) === v` 이므로
// 무동작이다).
//
// **상한을 넘기는 자리가 하나여야 그 규율이 지켜진다.** 그래서 `panelEditAlign` 을 들이는
// 파일은 `panels/canvas/` 안에서 **이 파일 하나뿐**이며, 그 형상 자체를 이웃한 테스트가
// 가드로 막는다(`canvasEditArrange.test.ts` §바이패스 금지). 원 함수를 직접 부를 수 있는
// 자리가 남아 있으면 규율은 주석일 뿐이고, 주석은 다음 사람이 읽지 않는다.
//
// ## 재사용의 대가 셋 (spec.md §스냅·정렬 재사용의 대가)
//
// 1. **상한 무한대** — 위 R6.
// 2. **스냅 기준은 요소의 중심이다.** `snapOffsetToGrid` 가 중심을 격자에 맞춘다(다른
//    패널의 `+` 표식과 같은 기준). 캔버스에서만 좌상단을 맞추면 같은 대시보드 안에서
//    격자가 두 뜻을 갖는다.
// 3. **`base.offset` 은 언제나 0 이다.** 그 모듈은 "흐름상 시작 자리 + 오프셋" 모델이라
//    잡는 순간의 오프셋을 되짚는다. 캔버스에는 흐름이 없고 **저장된 값이 곧 절대 자리**
//    이므로 되짚을 오프셋이 없다.
//
// 정규화(0..1) ↔ 백분율 변환은 **×100 한 번**이다. 정규화 좌표가 스테이지 축 길이에 대한
// 분수이고 백분율 오프셋이 같은 기준 상자(= 스테이지)에 대한 백분율이므로, 두 축이 같은
// 것을 가리킨다 — 그래서 나누고 곱하는 것 말고 할 일이 없다.
//
// 격자 간격의 **기본값**은 `GRID_STEP_PERCENT`(10%, 축당 10칸)를 그대로 쓴다. 캔버스 전용
// 기본 간격을 따로 만들지 않는다 — 격자 어휘가 둘이 되는 비용이 더 크다.
//
// 캔버스에서만 그 간격을 **고를 수 있다**(`CANVAS_GRID_STEP_CHOICES`). 다른 패널의 요소는
// 몇 개뿐이라 10% 한 벌로 충분하지만, 캔버스는 사용자가 스스로 도형을 늘어놓는 자리라
// 촘촘하게/성기게 붙일 이유가 실제로 있다. 그래도 **간격을 넘기는 자리는 여전히 하나**다:
// `snapDelta` 의 `step` 인자 하나가 붙임을 정하고, 호출부는 **그 값을 격자 표시에도 그대로**
// 넘긴다. 보이는 간격과 붙는 간격이 갈라지면 화면이 거짓말을 한다.
//
// ## 칸은 정사각형이다 — 그래서 축마다 백분율이 다르다
//
// 백분율은 언제나 제 축 길이에 대한 값이라, 두 축에 같은 10% 를 주면 800×400 스테이지에서
// 칸이 80×40 px 이 된다. 캔버스는 그림을 그리는 자리이므로 칸이 정사각이어야 한다. 그래서
// `squareGridSteps` 가 고른 값 하나(폭 기준)에서 **한 물리 칸**을 정하고 그것을 두 축의
// 백분율로 옮겨 적는다. 그리는 쪽(`PanelEditGrid` 의 `step`/`stepY`) · 붙는 쪽(`snapDelta`)
// · Shift+방향키 한 칸이 **모두 그 함수 하나를 지나므로**, 백분율이 둘이 되어도 칸은
// 여전히 하나다.
//
// ## z-order — 두 번째 규칙을 만들지 않는다
//
// 001 의 z-order 수단은 **배열 순서 하나뿐**이다(뒤 = 위). 목록 편집기의 위/아래 이동
// 버튼이 하는 일은 `제거 후 삽입`(splice-splice) 한 규칙이고, 맨 앞/맨 뒤로 보내기는
// 그 **같은 연산에 목표 위치만 끝값(마지막 / 0)으로 준 것**이다. 그래서 이 모듈은
// `moveElementTo` 하나를 두고 앞/뒤 보내기를 그 위에 얹는다 — 정렬 규칙을 두 벌 쓰면
// "어디서 눌렀는가" 에 따라 결과가 달라진다(REQ-04).
//
// **DOM 무의존이다.** React 도 i18n 도 부르지 않으므로 jsdom 없이 전량 단위 테스트된다 —
// `canvasHitTest.ts` · `canvasEditGeometry.ts` · `canvasElementFactory.ts` 와 같은 규율이다.
//
// @spec SPEC-CANVAS-002 REQ-04 / REQ-06

import {
  GRID_STEP_PERCENT,
  computeAlignPatches,
  snapOffsetToGrid,
  type AlignAxis,
  type AlignMode,
  type Box,
  type PanelElementBox,
} from '../charts/panelEditAlign';
import type { CanvasElement } from './canvasConfig';
import type { NormalizedDelta } from './canvasEditGeometry';
import type { PxBox, StageSize } from './canvasGeometry';

// --- 타입 ---------------------------------------------------------------

export type { AlignAxis, AlignMode };

/** 정렬에 참여하는 요소 하나 — **투영된 px 상자**를 든다(spec.md §스냅·정렬 재사용의 대가). */
export interface AlignTarget {
  /** 최상위 배열 원소의 id. 배열 위치(index)를 들고 다니지 않는다(REQ-06). */
  nodeId: string;
  /** 스테이지 로컬 CSS px 상자. 호출부가 `canvasGeometry` 의 투영으로 만든다. */
  box: PxBox;
}

/** 한 요소에 적용할 정규화 이동량. */
export interface AlignDelta {
  nodeId: string;
  delta: NormalizedDelta;
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

/** 격자 간격(%)의 **기본값**. `panelEditAlign` 이 소유하는 값을 그대로 다시 내보낸다. */
export const CANVAS_GRID_STEP_PERCENT = GRID_STEP_PERCENT;

/**
 * 화면에서 고를 수 있는 격자 간격(%) — **캔버스에만 있는 목록**이다.
 *
 * 기본값을 바꾸는 것이 아니라 **고를 수 있게** 하는 것이므로 격자 어휘는 여전히 하나다:
 * 목록의 가운데 값이 곧 `CANVAS_GRID_STEP_PERCENT` 이고(이웃한 테스트가 그 형상을
 * 지킨다), 다른 패널은 이 목록을 보지 않는다.
 *
 * 값 셋을 고른 근거:
 *   - 셋 다 **100 을 나누어떨어지게** 한다. 그래야 마지막 선이 스테이지 모서리에 앉고,
 *     반 칸짜리 자투리 칸이 생기지 않는다.
 *   - 기본값 10 의 **절반과 두 배**다. 옆 칸이 "두 배 성기게 / 두 배 촘촘하게" 로 읽혀
 *     설명이 필요 없다.
 *   - 5 보다 촘촘하면 작은 패널에서 선이 뭉개져 참조선 구실을 못하고(`GRID_STEP_PERCENT`
 *     주석이 10 을 고른 그 이유), 20 보다 성기면 축당 붙을 자리가 다섯 곳뿐이다.
 *
 * 이 목록은 **런타임 표시 상태**일 뿐 config 스키마가 아니다(가정 A4) — 격자 토글·행
 * 펼침과 같은 부류이며, 저장되지 않는다.
 */
export const CANVAS_GRID_STEP_CHOICES: readonly number[] = [5, 10, 20];

/**
 * 두 축의 격자 간격(%) — **같은 물리 칸 하나**를 축마다의 백분율로 옮겨 적은 것이다.
 *
 * 백분율이 두 개인 것이 칸이 두 종류라는 뜻은 아니다. 칸은 하나(px)이고, 그것을 그리는
 * 자리(`repeating-linear-gradient`)와 붙이는 자리(`snapOffsetToGrid`)가 **둘 다 제 축
 * 길이에 대한 백분율**로만 말할 수 있어서 두 번 옮겨 적을 뿐이다.
 */
export interface GridSteps {
  /** 가로 간격 — 스테이지 **폭**에 대한 백분율. 고른 값 그대로다. */
  x: number;
  /** 세로 간격 — 스테이지 **높이**에 대한 백분율. 같은 px 칸이 되도록 다시 잰 값이다. */
  y: number;
}

/**
 * 고른 간격 하나에서 **정사각 칸**의 두 축 백분율을 만든다.
 *
 * ## 왜 필요한가
 *
 * 백분율은 언제나 **제 축 길이에 대한** 값이다. 그래서 두 축에 같은 10% 를 주면
 * 800×400 스테이지에서 칸이 80×40 px 이 된다 — 눈에 보이는 그대로 직사각형이다.
 * 통계·바·파이·게이지·선에서는 그것이 옳다(그 패널의 요소 자리가 축마다의 백분율
 * 오프셋이므로 격자도 같은 축을 그려야 눈금과 선이 만난다). **캔버스는 그림을 그리는
 * 자리이고, 그리는 사람은 정사각 칸을 기대한다.**
 *
 * ## 기준 축이 폭인 이유
 *
 * 칸 하나의 크기를 정하려면 어느 축을 그대로 두고 어느 축을 따라가게 할지 골라야 한다.
 * 여기서는 **폭**을 그대로 둔다.
 *
 *   1. `step` 의 뜻이 보존된다. 고르는 값 5·10·20 은 "100 을 나누어떨어지게 해서 마지막
 *      선이 모서리에 앉는다" 는 근거로 뽑혔고(`CANVAS_GRID_STEP_CHOICES`), 그 성질은
 *      기준 축에서만 성립한다. 폭을 기준으로 두면 "10% = 가로 10칸" 이라는 화면의 말이
 *      그대로 남는다.
 *   2. **지나치게 촘촘해지지 않는다.** 대시보드 패널은 대개 세로보다 가로가 길다. 높이를
 *      기준으로 삼으면 그 패널에서 가로 칸 수가 비율만큼 늘어 선이 뭉개지고, 그것은
 *      `GRID_STEP_PERCENT` 주석이 10 을 고르며 피한 바로 그 상태다. 반대로 폭을 기준으로
 *      두면 세로 칸이 성겨질 뿐이며, 성긴 격자는 붙을 자리가 줄 뿐 참조선 구실은 남는다.
 *      실패의 대가가 한쪽으로 크게 기운다.
 *
 * 대가는 명확하다: 세로 백분율은 100 을 나누어떨어지지 않을 수 있어 **마지막 가로선이
 * 아래 모서리에 앉지 않는다**. 그것을 감수하는 것이 위 두 이유이며, 어느 축을 고르든
 * 종횡비가 정수가 아닌 한 한쪽은 반드시 자투리를 갖는다.
 *
 * ## 잴 수 없는 스테이지
 *
 * 축 길이를 모르면 두 축에 **같은 값**을 돌려준다 — 종전 동작이며, 그때는 붙임 자체가
 * 손대지 않는 경로(`snapAxis`)로 빠지므로 값이 쓰이는 곳은 Shift+방향키 한 칸뿐이다.
 * 거기서는 "한 칸 = 축 길이의 `step`%" 가 여전히 뜻이 성립한다.
 *
 * 소수 넷째 자리에서 턴다. `snapOffsetToGrid` 가 결과 좌표에 대해 하는 것과 같은 규율이고
 * (화면 해상도보다 훨씬 곱다), **그리는 쪽과 붙는 쪽이 이 함수 하나를 지나므로** 턴 값도
 * 둘에 똑같이 간다 — 한쪽만 턴 값을 쓰면 그 차이가 곧 화면의 거짓말이 된다.
 */
export function squareGridSteps(step: number, stage: StageSize): GridSteps {
  if (!isFinite2(step, stage.width, stage.height)) return { x: step, y: step };
  if (!(stage.width > 0) || !(stage.height > 0)) return { x: step, y: step };
  return { x: step, y: Math.round(((step * stage.width) / stage.height) * 1e4) / 1e4 };
}

// --- 순수 도우미 ---------------------------------------------------------

/** 유한한 수만 통과시킨다. 포인터 좌표는 NaN/Infinity 가 될 수 있다(§품질 게이트 Secured). */
function isFinite2(...values: number[]): boolean {
  return values.every((v) => Number.isFinite(v));
}

// --- 격자 붙임 (T12) ------------------------------------------------------

/**
 * 한 축의 이동량을 격자에 맞춘다. **원 함수를 부르는 유일한 자리 둘 중 하나다.**
 *
 * `base.offset` 이 0 인 것이 캔버스의 성질 그 자체다 — 저장된 값이 곧 절대 자리이므로
 * 되짚을 흐름상 시작 자리가 따로 없고, `rectStart` 가 곧 그 자리다.
 */
function snapAxis(
  delta: number,
  rectStart: number,
  rectLen: number,
  stageLen: number,
  step: number,
): number {
  // 잴 수 없는 축에서는 손대지 않는다 — 0 으로 죄면 격자를 켠 순간 요소가 얼어붙는다.
  if (!isFinite2(delta, rectStart, rectLen) || !(stageLen > 0)) return delta;
  const snapped = snapOffsetToGrid(
    delta * 100,
    { offset: 0, rectStart, rectLen },
    { start: 0, len: stageLen },
    // 위험 R6 — 이 인자를 빠뜨리면 드래그가 스테이지의 40% 에서 조용히 멈춘다.
    // 간격을 넘기려면 이 자리를 지나야 하므로, 간격을 고르는 순간에도 상한은 함께 간다.
    NO_OFFSET_LIMIT,
    step,
  );
  return snapped / 100;
}

/**
 * 드래그 이동량을 격자에 맞춘다(REQ-04 · AC-E5).
 *
 * `anchor` 는 **잡는 순간** 화면에서 잰 기준 요소의 상자다(양수 범위로 정규화된 px).
 * 무리 이동이면 잡은 요소 하나가 기준이며 나머지는 같은 델타로 따라온다 — 무리의 각
 * 요소를 저마다 격자에 붙이면 무리가 끌려가는 동안 서로 흩어진다.
 *
 * **clamp 하지 않는다.** 상한을 무한대로 넘기므로 스테이지 밖으로 끌어도 붙잡히지
 * 않는다(위험 R6 · 가정 A5).
 *
 * `step` 은 **화면에 그려진 격자와 같은 값이어야 한다.** 보이는 간격과 붙는 간격이
 * 갈라지면 화면이 거짓말을 한다 — 그래서 호출부는 격자에 넘기는 그 값을 여기에도
 * 넘긴다(둘을 각자 정하는 자리를 만들지 않는다).
 *
 * **두 축은 같은 백분율을 쓰지 않는다.** `squareGridSteps` 가 고른 값 하나에서 정사각
 * 칸의 두 축 백분율을 만들고, 이 함수는 그것을 **여기서 다시 파생한다** — 축 쌍을 인자로
 * 받지 않는 것이 요점이다. 받으면 부르는 쪽이 그 쌍을 스스로 조립할 수 있고, 그 조립이
 * 곧 그림과 어긋날 수 있는 두 번째 자리가 된다. 같은 `step`·같은 `stage` 를 주면 그리는
 * 쪽과 이 자리가 **정의상** 같은 두 수를 본다.
 */
export function snapDelta(
  delta: NormalizedDelta,
  anchor: PxBox,
  stage: StageSize,
  step: number = CANVAS_GRID_STEP_PERCENT,
): NormalizedDelta {
  const cell = squareGridSteps(step, stage);
  return {
    dx: snapAxis(delta.dx, anchor.x, anchor.w, stage.width, cell.x),
    dy: snapAxis(delta.dy, anchor.y, anchor.h, stage.height, cell.y),
  };
}

// --- 정렬 (T13) -----------------------------------------------------------

/**
 * 고른 요소들을 서로 맞춘다(REQ-04 · AC-08). **원 함수를 부르는 유일한 자리 둘 중 하나다.**
 *
 * 변환 사슬은 spec.md 가 적은 그대로다: **투영된 px 상자 → 백분율 오프셋 → ÷100 → 정규화
 * 델타**. `offsetX/offsetY` 를 0 으로 넣기 때문에 돌려받은 오프셋이 곧 **이동량**이며,
 * 그래서 한 번 나누는 것 말고 되짚을 것이 없다.
 *
 * 기준 상자는 스테이지다 — 백분율의 분모가 스테이지 축 길이여야 ÷100 이 곧 정규화 델타가
 * 된다. 요소들이 이루는 바깥 상자에 맞추는 것은 `computeAlignPatches` 안의 규칙이며
 * 여기서 다시 정하지 않는다.
 *
 * **2개 미만이면 빈 배열이다.** 맞출 상대가 없기 때문이며, 그 판정도 원 함수가 이미
 * 소유한다 — 여기서 다시 세면 두 곳이 갈라질 수 있다.
 */
export function alignDeltas(
  targets: readonly AlignTarget[],
  stage: StageSize,
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
  const bounds: Box = { left: 0, top: 0, width: stage.width, height: stage.height };
  return computeAlignPatches(boxes, bounds, axis, mode).map((patch) => ({
    nodeId: patch.kind,
    delta: { dx: (patch.offsetX ?? 0) / 100, dy: (patch.offsetY ?? 0) / 100 },
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
