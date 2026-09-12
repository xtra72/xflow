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
// 그려지는 격자 · 드래그 붙임(`snapDelta`) · Shift+방향키 한 칸(호출부가 그 값을 그대로
// 더한다). 셋 다 **환산 없이 그 정수를** 쓴다 — 0.9.0 이 그리는 쪽의 백분율 환산마저
// 걷어냈기 때문이다(아래 §격자를 화면에 그리는 간격). 환산이 없는 곳은 어긋날 수도 없다.
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
import type { CanvasBox, CanvasDelta, CanvasProjection, PxBox } from './canvasGeometry';

// SPEC-CANVAS-004 M1 — 이 파일의 배열 재정렬 셋(`moveElementTo`·`bringToFront`·
// `sendToBack`)은 원소의 `id` 밖에 보지 않는다. 004 가 최상위 배열에 그룹 노드를 더하면서
// 그 셋의 인자 타입을 `<T extends { id: string }>` 로 **일반화**했다 — 004 의 계획은
// "인자 타입이 이미 맞는다" 고 적었으나 TypeScript 는 구조적이어도 유니온이 제 원소로
// 대입되지는 않으므로(`CanvasNode[]` 는 `CanvasElement[]` 가 아니다) 그 문장은 참이
// 아니었다. 몸체는 한 글자도 바뀌지 않았고, 두 번째 재정렬 규칙도 생기지 않는다.

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
 * 기본 캔버스(500 × 400)에서 **20 × 16 칸**이 나온다. 아래 제안 목록의 가운데 값이며,
 * 넷 가운데 "칸이 뭉개지지도 성기지도 않은" 자리다 — 폭 250px 로 줄어든 패널에서도
 * 한 칸이 12.5px 이라 참조선 구실을 하고, 넓은 패널에서는 스무 칸이 눈으로 셀 수 있는
 * 밀도로 남는다.
 *
 * 정규화 시절의 기본값은 `panelEditAlign.GRID_STEP_PERCENT`(10%)를 그대로 다시 내보낸
 * 것이었다. 이제 단위가 백분율이 아니므로 그 값을 물려받을 수 없다 — 물려받는 시늉을 하면
 * "10" 이 두 화면에서 다른 것을 뜻하게 된다.
 */
export const CANVAS_GRID_STEP_UNITS = 25;

/**
 * 격자 간격의 범위 — **자유 입력의 난간**이다(0.11.0).
 *
 * 0.10.0 까지 이 자리에는 고른 값 넷(10 · 20 · 25 · 50)의 목록이 있었다. 넷을 고른 근거는
 * "넷 다 100 의 약수라 기본 캔버스(500 × 400)의 두 축을 나머지 없이 나눈다" 였고, 그것은
 * 사용 시험 "격자가 일정하지 않음" 에 대한 답의 일부였다. 그러나 그 성질은 **기본 캔버스에
 * 한해서만** 참이다 — 사용자가 캔버스를 640 × 480 으로 잡는 순간 25 도 640 을 나누지
 * 못하고, 목록은 지키려던 것을 지키지 못한 채 고를 자유만 빼앗는다.
 *
 * 그래서 목록을 걷고 정수 하나를 받는다. 남는 것은 두 가지다:
 *   - **난간**(이 상수 둘) — 0·음수·NaN·천문학적 수가 `stageLattice` 에 닿지 않게 한다.
 *   - **결과의 고지**(`gridDividesCanvas`) — 나누어떨어지지 않는 조합을 화면이 미리 말한다.
 *
 * 하한이 1 인 것은 캔버스 좌표가 정수이기 때문이다(1 보다 촘촘한 격자는 붙을 자리가 없다).
 * 한 칸이 1px 도 되지 않으면 그리는 쪽이 스스로 격자를 끄므로(`stageLattice` §한 칸이 1px
 * 미만이면), 하한을 더 올려 사용자를 막을 이유가 없다.
 *
 * 상한 1000 은 "화면이 뜻을 잃는 자리" 다 — 기본 캔버스 폭의 두 배라 선이 한 줄도 남지
 * 않는다. 이 값을 캔버스 크기에서 파생하지 않는 것에 뜻이 있다: 파생하면 캔버스를 줄일
 * 때마다 사용자가 적어 둔 간격이 조용히 달라진다.
 *
 * 이 값들은 **런타임 표시 상태**의 난간일 뿐 config 스키마가 아니다(가정 A4) — 격자 토글·
 * 행 펼침과 같은 부류이며, 저장되지 않는다.
 */
export const CANVAS_GRID_STEP_MIN = 1;
export const CANVAS_GRID_STEP_MAX = 1000;

/**
 * 입력된 격자 간격을 쓸 수 있는 정수로 만든다.
 *
 * 읽을 수 없는 입력(빈 칸 · 글자 · NaN)에는 **`fallback` 을 그대로** 돌려준다 — 0 으로
 * 떨어뜨리면 격자가 사라지고, 그것은 사용자가 한 글자를 지우는 순간 화면이 무너진다는
 * 뜻이다. 지우는 도중은 잘못된 상태가 아니라 **아직 끝나지 않은 상태**다.
 *
 * **문자열도 받는다.** 그러지 않으면 부르는 쪽이 `Number('')` 가 0 이라는 사실을 알고
 * 손수 걸러야 하고, 그 한 줄이 곧 격자 어휘의 두 번째 자리가 된다 — 이 모듈이 어휘를
 * 혼자 갖는다는 규율이 그 한 줄에서 깨진다.
 *
 * 읽을 수 있으면 정수로 내림하고 난간 안으로 죈다. 죄는 것을 조용히 하는 것에 뜻이 있다 —
 * 화면의 `min`/`max` 가 이미 같은 수를 적어 두었으므로, 그 위에 경고문을 얹으면 사용자가
 * 이미 아는 것을 두 번 말하는 셈이다.
 */
export function clampGridStep(value: string | number, fallback: number): number {
  const raw = typeof value === 'string' ? readNumber(value) : value;
  if (!Number.isFinite(raw)) return fallback;
  const stepped = Math.trunc(raw);
  return Math.min(CANVAS_GRID_STEP_MAX, Math.max(CANVAS_GRID_STEP_MIN, stepped));
}

/** 빈 칸은 0 이 아니라 **읽을 수 없음**이다 — `Number('')` 가 0 인 것이 이 함수의 존재 이유다. */
function readNumber(raw: string): number {
  return raw.trim() === '' ? Number.NaN : Number(raw);
}

/**
 * 이 간격이 캔버스 두 축을 나머지 없이 나누는가.
 *
 * 거짓이면 **마지막 칸이 반 칸으로 남는다**. 그것은 사용자가 세 번 돌려보낸 그 결함
 * (스테이지 종횡비에서 파생되어 패널을 늘일 때마다 달라지던 자투리)과 **다른 종류의
 * 일이다**: 이제 자투리는 사용자가 고른 두 정수에서 곧바로 따라 나오고, 패널을 어떻게
 * 늘여도 변하지 않으며, 값을 바꾸면 사라진다.
 *
 * 그래도 **화면이 먼저 말해야 한다** — 말하지 않으면 사용자는 그것을 고쳐진 줄 알았던 그
 * 결함으로 읽는다. 이 함수는 그 고지의 조건이며, 자유 입력을 연 대가로 새로 진 빚이다.
 */
export function gridDividesCanvas(step: number, canvas: { width: number; height: number }): boolean {
  if (!Number.isFinite(step) || !(step > 0)) return false;
  if (!Number.isFinite(canvas.width) || !Number.isFinite(canvas.height)) return false;
  return canvas.width % step === 0 && canvas.height % step === 0;
}

/**
 * 화면이 **곁들여 제안하는** 격자 간격(정수 캔버스 단위).
 *
 * 0.10.0 까지 이 목록은 고를 수 있는 값의 **전부**였다. 0.11.0 은 자유 입력을 열면서
 * 목록을 지우는 대신 **제안**으로 격을 낮췄다 — 지웠다면 사용자는 빈 칸 앞에서 "몇을
 * 적어야 하지" 를 처음부터 생각해야 하고, 넷이 답해 주던 그 물음은 자유와 아무 상관이
 * 없다.
 *
 * 넷을 고른 근거는 그대로다:
 *   - 넷 다 **100 의 약수**다. 기본 캔버스의 두 축이 100 의 배수(500 · 400)이므로, 넷 가운데
 *     무엇을 골라도 **두 축이 나머지 없이 떨어진다** — 반 칸짜리 자투리가 생기지 않는다.
 *   - 100 의 약수 가운데 1 · 2 · 4 · 5 는 기본 캔버스에서 100 칸을 넘겨 선이 뭉개지고,
 *     100 은 한 축에 다섯 칸뿐이라 붙일 자리가 사실상 없다. 남는 것이 이 넷이다.
 *   - 넷이 **2배 간격의 사다리**(10 → 20 → (25) → 50)라 옆 칸이 "두 배 성기게 / 두 배
 *     촘촘하게" 로 읽혀 설명이 필요 없다.
 *
 * **넷이 언제나 나누어떨어진다는 보장은 아니다** — 그것은 기본 캔버스에 한한 성질이며,
 * 캔버스를 바꾸면 넷도 자투리를 낼 수 있다. 그 사실을 말하는 것은 이 목록이 아니라
 * `gridDividesCanvas` 이고, 그래서 고지는 고른 값이 목록 안이든 밖이든 **똑같이** 뜬다.
 */
export const CANVAS_GRID_STEP_CHOICES: readonly number[] = [10, 20, 25, 50];

/**
 * 격자를 **화면에 그리는** 간격은 이제 이 모듈이 내지 않는다 (0.9.0).
 *
 * 0.8.0 까지는 여기 `gridPercents` 가 있어 정수 간격 하나를 축마다의 백분율로 옮겨 적었다.
 * 그 백분율은 브라우저가 상자 폭에 곱하는 순간 **소수 px** 가 되고(1749px 상자에 5% =
 * 87.45px), 소수 자리에서 시작하는 1px 선은 두 장치 픽셀에 나뉘어 칠해져 선마다 굵기가
 * 달라 보였다 — 사용자가 세 번째로 돌려보낸 "격자가 일정하지 않음" 이 그것이다.
 *
 * 고친 자리는 여기가 아니라 **그리는 영역**이다: 표면이 영역을 칸의 정수배로 맞추고
 * (`canvasGeometry.stageLattice`) 그 정수 칸을 px 로 건넨다. 그래서 백분율로 옮겨 적는
 * 단계가 통째로 사라졌고, 이 모듈에 남는 격자 어휘는 **정수 간격 하나**(`gridStep`)뿐이다
 * — 붙임과 Shift 가 이미 그것을 그대로 쓰던 그 값이다.
 */

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
 * `step` 은 **화면에 그려진 격자와 같은 값**이다 — 그리는 쪽은 표면이 이 정수로 맞춰 둔
 * 칸을 그대로 받아 그리므로 둘이 갈라질 자리가 없다.
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
export function moveElementTo<T extends { readonly id: string }>(
  elements: readonly T[],
  nodeId: string,
  targetIndex: number,
): readonly T[] {
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
 * 고른 것들을 배열에서 **뺀다** — 지우는 규칙은 이 함수 하나다(SPEC-CANVAS-010).
 *
 * 목록 편집기의 휴지통과 오버레이의 Delete·Backspace 가 같은 이 함수를 지난다. 두 벌이
 * 되면 "목록에서 지웠는가 캔버스에서 지웠는가" 에 따라 결과가 갈리고, 그 차이는 저장된
 * 뒤에야 드러난다 — `moveElementTo` 가 순서 이동에 대해 세운 그 규율과 같은 자다.
 *
 * **그룹은 부품을 따로 다루지 않는다.** 부품은 최상위 배열이 아니라 그룹 항목 **안**에
 * 살므로(SPEC-CANVAS-004), 항목 하나를 빼는 것이 곧 그 무리를 통째로 빼는 것이다.
 *
 * **아무것도 빠지지 않으면 받은 배열을 그대로 돌려준다**(같은 참조). 호출부가 그것으로
 * "쓸 일이 없다" 를 알아채 헛된 config 쓰기와 렌더 프레임을 만들지 않는다 — 위 함수와
 * 같은 계약이다. 선택에는 이미 지워진 id 가 남아 있을 수 있으므로 이 경우는 실제로 온다.
 *
 * 식별은 언제나 `nodeId` 다(REQ-06) — 배열 위치가 아니다.
 */
export function removeNodes<T extends { readonly id: string }>(
  elements: readonly T[],
  nodeIds: ReadonlySet<string>,
): readonly T[] {
  // 빈 집합에 대한 지름길을 두지 않는다 — 그때도 아래 걸러 내기가 전부를 남기고 길이가
  // 같으므로 **같은 참조**가 나온다. 넣어 보면 어떤 시험도 빨개지지 않는 줄이고, 무언가를
  // 막는 것처럼 읽히는 그런 줄이 다음 사람에게는 "빈 집합은 다르게 다뤄진다" 고 말한다.
  const next = elements.filter((el) => !nodeIds.has(el.id));
  return next.length === elements.length ? elements : next;
}

/**
 * 고른 것들을 **맨 앞(배열 끝 = 위)** 으로 보낸다.
 *
 * 배열 순서대로 훑으며 하나씩 끝으로 보내면 무리의 **상대 순서가 보존된다** — 무리를
 * 앞으로 꺼냈더니 자기들끼리 뒤섞이면 사용자는 그것을 되돌릴 방법을 화면에서 찾지 못한다.
 */
export function bringToFront<T extends { readonly id: string }>(
  elements: readonly T[],
  nodeIds: ReadonlySet<string>,
): readonly T[] {
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
export function sendToBack<T extends { readonly id: string }>(
  elements: readonly T[],
  nodeIds: ReadonlySet<string>,
): readonly T[] {
  let next = elements;
  for (let i = elements.length - 1; i >= 0; i -= 1) {
    const el = elements[i];
    if (el !== undefined && nodeIds.has(el.id)) next = moveElementTo(next, el.id, 0);
  }
  return next;
}
