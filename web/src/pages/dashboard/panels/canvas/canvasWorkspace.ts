// 작업 영역 상자 산술 — 패널보다 넓은 저술 공간과 그 안의 패널 출력 영역
// (SPEC-CANVAS-006 M1).
//
// **DOM 무의존**이다. `document`/`window`/`CanvasRenderingContext2D` 를 참조하지 않으며,
// 잰 상자와 저술된 캔버스 크기 두 수치 묶음만 받아 상자 넷을 낸다. 004(그룹) · 005(배경
// 에셋) · 007(벡터 가져오기)이 모두 이 상자 위에 서므로, 산술이 컴포넌트 안으로 흩어지면
// 셋이 각자 다시 파생한다(REQ-06).
//
// ## 두 상자가 무엇인가
//
// 오늘까지 **그리는 영역이 곧 패널**이었다. `stageLattice` 가 잰 상자를 격자 칸에 맞춰
// 안쪽 상자 하나를 짓고, 캔버스와 편집 오버레이가 그 상자 안에 함께 살았다. 그래서 저술
// 공간과 출력 공간이 같은 하나였고, 캔버스 밖에 놓인 요소는 **저장은 되지만 보이지 않았다**
// (비트맵이 제 상자 밖의 그리기를 버린다). 006 이 그 하나를 둘로 가른다.
//
//   - **작업 영역(`box`)** — 편집 중일 때 캔버스 비트맵이 덮는 상자. 잰 바깥 상자 전부다.
//   - **패널 출력 영역(`stage`)** — 그 안에 그려지는 사각형. 캔버스 좌표계
//     `(0,0)–(canvas.width, canvas.height)` 그 자체이며 **`projection.stage` 의 뜻이 한
//     글자도 바뀌지 않는다**.
//
// 원점(`origin`)은 작업 영역 안에서 출력 영역이 앉는 자리이고, 그것이 곧 캔버스 좌표
// `(0,0)` 의 px 자리다. 비트맵의 원점 이동은 `CanvasSurface.drawFrame` 이 이미 부르고 있는
// `setTransform` 의 마지막 두 인자가 진다 — `drawElement.ts` 도 `DrawContext2D` 도 한 글자도
// 바뀌지 않는다.
//
// ## 왜 비율로 줄이고 절대 px 로 줄이지 않는가 (가정 A5)
//
// 축소가 두 축에 **같은 비율**로 걸리므로 작업 영역과 출력 영역은 닮은 직사각형이다.
// 그 성질에서 셋이 함께 나온다.
//   1. **축척이 하나다.** `stageLattice` 를 줄어든 상자에 그대로 부르므로, 두 축의 '정확한'
//      칸 가운데 작은 쪽을 한 번만 내리는 그 규율이 그대로 성립한다 — 칸은 정사각형이고
//      도형은 일그러지지 않는다.
//   2. **패널 비율 맞춤이 계속 옳다.** 맞춤은 잰 **바깥** 상자의 비율을 쓰는데, 줄어든
//      상자의 비율이 바깥 상자와 같으므로 그 계산이 한 줄도 바뀌지 않는다. 절대 px 여백을
//      뺐다면 두 비율이 달라져 맞춤이 **조용히 제 일을 못 하게** 되었을 것이다.
//   3. **원점이 정수다.** `floor` 로 정수화하고 남는 자투리는 **여백 안에서** 사라진다.
//      자투리를 출력 영역에 밀어 넣으면 `projection.stage` 가 어림값이 되고, 그것은 곧
//      오버레이가 제 상자로 재는 값과 갈라진다는 뜻이다(위험 R1).
//
// 여백을 캔버스 단위로 적는 길(`margin = canvas × ratio`)도 있었으나 기각했다 — 여백의 px
// 크기가 `marginUnits × cell / step` 이라 **정수가 아닐 수 있고**, 그러면 원점이 소수가
// 되어 격자선이 두 장치 픽셀에 걸친다(0.9.0 이 고친 "격자가 일정하지 않음" 의 네 번째 얼굴).
//
// ## 두 갈래를 한 함수가 소유한다 (REQ-06)
//
// 편집이 꺼진 갈래는 `stageLattice` 결과를 그대로 옮겨 담아 **오늘과 값이 완전히 같다**.
// 그 갈래를 부르는 쪽에 두면(`if (편집) 이 상자 else 저 상자`) 둘이 각자 자라고 어느 날
// 한쪽만 고쳐진다. 그래서 한 함수가 두 경우를 모두 든다.
//
// ## 형제 하나 — 크기 유도 (M8 · REQ-07)
//
// `derivedCanvasSize(canvas, outer)` 는 편집 중 캔버스 크기를 잰 상자에서 유도한다. 그
// 유도는 **항등**이라 어떤 축척도 곱하지 않으며, 그래서 축소 상자 식의 소유자는 위
// `workspaceBox` 의 편집 갈래 **하나뿐**이다 — 같은 상자를 두 곳에서 파생할 자리가 형상
// 자체로 없다(위험 R1 · R4 가 이 자리에서 닫힌다).
//
// ## 보기 배율 (M9 · REQ-09)
//
// 배율은 **새 기구가 아니라 이미 있는 산술의 입력**이다. `workspaceBox` 의 편집 갈래에서
// `reduced` 를 내는 **그 한 줄**에만 들어가고, 축척은 여전히 `stageLattice` 가 한 번만
// 내려 만드는 그 하나다. 그 좁음이 곧 안전이며, 넓어지려는 변경(배율이 `derivedCanvasSize`
// 로 · 배율이 `stageLattice` 안으로 · 배율에 몸짓이)은 전부 이 SPEC 이 이미 기각한 것들과
// 만난다.
//
// @spec SPEC-CANVAS-006

import {
  MAX_CANVAS_DIMENSION,
  MIN_CANVAS_DIMENSION,
  type CanvasSize,
} from './canvasConfig';
import { stageLattice, type StageCell, type StageSize } from './canvasGeometry';

/**
 * 편집기가 작업 영역을 보여 주는 **기본** 축척 — 보기 배율의 기본값(REQ-09 · 0.6.0).
 *
 * ## 왜 이름이 바뀌었는가 (`PANEL_REGION_FIT_RATIO` 은퇴)
 *
 * 0.5.0 이 이 상수의 **뜻**을 한 번 옮겼다: "출력 영역이 잰 상자에서 차지하는 비율" 에서
 * **"편집기가 작업 영역을 보여 주는 축척"** 으로. 그때는 값이 하나뿐이라 옛 이름이
 * **부정확할** 뿐이었다. 0.6.0 에서 그 값이 범위 안의 여럿 가운데 **기본 하나**가 되므로
 * 옛 이름은 부정확한 정도를 넘어 **거짓**이 된다 — 맞추는(fit) 일이 없고 고정된
 * 비율(ratio)도 아니다. 한 이름에 세 뜻이 쌓이면 다음 사람은 문서를 읽어야만 코드를 읽을
 * 수 있게 되므로 개명한다.
 *
 * 개명이 지는 대가는 **R21 의 이름 기반 가드가 조용히 무장 해제되는 것**이었다(위험 R23).
 * 그 가드는 같은 회차에 **형상 가드**로 옮겨 갔다 — 아래 `derivedCanvasSize` §인자가 둘인
 * 것이 가드다.
 *
 * 기본값 `0.75` 는 **0.5.0 까지의 그 값 그대로**다. 배율을 넘기지 않은 모든 호출이 종전과
 * 한 픽셀도 다르지 않은 이유가 이 한 줄이다(AC-09 (AR)).
 */
export const DEFAULT_WORKSPACE_ZOOM = 0.75;

/**
 * 보기 배율의 **하한**.
 *
 * 근거는 격자다. 실현 가능한 축척은 `floor(z × step) ÷ step` 이라 `z × step < 1` 이면
 * 한 칸이 1px 도 되지 않고, 그때 오버레이의 `gridCell.x >= 1` 게이트가 격자를 끈다.
 * `z ≥ 0.25` 는 도크가 제안하는 네 간격(10 · 20 · 25 · 50)에서 칸을 살려 두는 값이다.
 *
 * **"격자는 언제나 그려진다" 를 주장하지 않는다** — 아주 작은 간격에서는 범위 안에서도
 * 꺼지며, 그것은 배율이 만든 새 부류가 아니다(M8 뒤에는 기본 배율에서도 `step = 1` 이면
 * 이미 꺼진다). 가정 A20.
 */
export const MIN_WORKSPACE_ZOOM = 0.25;

/**
 * 보기 배율의 **상한**. **확대는 없다** — 이 값이 1 을 넘으면 안 된다(불변식 I22).
 *
 * `z ≤ 1` 이면 `reduced ≤ outer` 이고 `stage ≤ reduced ≤ outer` 이므로 `origin ≥ 0` 이며
 * 출력 영역이 작업 영역 **안**에 있다. `z > 1` 이면 셋이 한꺼번에 깨져 출력 영역 일부가
 * 컨테이너의 `overflow-hidden` 에 잘리는데, **잘린 자리를 가져올 팬이 없다** — 팬에는 이미
 * 주인이 있다(`previewPan`, 가정 A8). 그래서 상한이 곧 "캔버스 안에 시야를 만들지 않는다"
 * 는 기각을 범위로 번역한 것이다.
 */
export const MAX_WORKSPACE_ZOOM = 1;

/**
 * 화면이 곁들이는 제안 배율 넷(분수). **고를 수 있는 값의 전부가 아니다** — 자유 입력이고
 * 이 넷은 빈 칸 앞에서 "몇을 적지" 를 묻지 않게 하는 곁들이다(격자 간격이 세운 선례 그대로).
 *
 * 하한 · 절반 · 기본값 · 상한이다.
 */
export const WORKSPACE_ZOOM_CHOICES: readonly number[] = [0.25, 0.5, 0.75, 1];

/** 빈 칸과 글자를 NaN 으로 접는다(`canvasEditArrange.readNumber` 와 같은 규율). */
function readNumber(raw: string): number {
  return raw.trim() === '' ? Number.NaN : Number(raw);
}

/**
 * **백분율과 분수가 만나는 자리다.** 화면은 백분율 정수로만 말하고 산술은 분수로만 말하는데,
 * 그 둘이 만나는 곳을 이 모듈 밖으로 새게 두지 않는다(`stagePoint` 가 좌표 공간에 대해
 * 그러한 것과 같은 규율 — 불변식 I4). 쓰는 쪽이 이 함수, 읽는 쪽이 아래
 * `workspaceZoomPercent` 이며, 도크에는 `100` 이라는 수가 한 번도 나오지 않는다.
 *
 * 읽을 수 없는 입력(빈 칸 · 글자 · NaN)에는 **`fallback` 을 그대로** 돌려준다 — 한 글자를
 * 지우는 동안 화면이 무너지지 않아야 한다(`clampGridStep` 의 계약 그대로).
 *
 * **적힌 값을 실현 가능한 축척으로 되죄지 않는다.** 되죄면 격자 간격이 배율을 조용히
 * 고치게 되고, 그것은 불변식 I20 이 유도에 대해 금지한 결합의 거울상이다(위험 R24).
 * 여기서 하는 일은 **범위 죔 하나**뿐이다.
 */
export function clampWorkspaceZoom(percent: string | number, fallback: number): number {
  const raw = typeof percent === 'string' ? readNumber(percent) : percent;
  if (!Number.isFinite(raw)) return fallback;
  return Math.min(MAX_WORKSPACE_ZOOM, Math.max(MIN_WORKSPACE_ZOOM, raw / 100));
}

/** 분수를 화면이 쓰는 백분율 정수로 옮긴다. 위 함수의 반대 방향이며 짝은 이 둘뿐이다. */
export function workspaceZoomPercent(zoom: number): number {
  return Math.round(zoom * 100);
}

/**
 * 한 벌의 상자 넷.
 *
 * 넷을 함께 돌려주는 데 뜻이 있다 — 넷은 **한 번의 격자 맞춤에서 함께 나오는 값**이라,
 * 따로 구하면 그 맞춤이 두 벌이 되고 한쪽만 고쳐진 채 갈라진다(`StageLattice` 가 셋을
 * 함께 돌려주는 것과 같은 이유이며, 위험 R1 · R4 와 같은 부류다).
 */
export interface CanvasWorkspaceBox {
  /** 작업 영역(캔버스 비트맵이 덮는 상자)의 CSS px 크기. */
  box: StageSize;
  /** 잰 바깥 상자 안에서 `box` 가 앉는 정수 자리. */
  offset: StageCell;
  /** 패널 출력 영역의 CSS px 크기. **이것이 `projection.stage` 다.** */
  stage: StageSize;
  /** `box` 안에서 `stage` 가 앉는 정수 자리 = 캔버스 좌표 (0,0) 의 px 자리. */
  origin: StageCell;
  /** 한 칸의 CSS px. 두 축이 같은 값이다(축척이 하나다). */
  cell: StageCell;
}

/** 유한 양수면 그대로, 아니면 0. NaN 이 상자 산술로 번지는 것을 막는다. */
function positiveOrZero(v: number): number {
  return Number.isFinite(v) && v > 0 ? v : 0;
}

/**
 * 잰 바깥 상자와 캔버스 크기에서 상자 한 벌을 낸다.
 *
 * `workspace === false`(대시보드 · 편집 꺼짐)에서는 `stageLattice` 결과를 그대로 옮겨
 * 담는다: `box === stage` 이고 `origin === {0,0}` 이므로 **오늘과 값이 완전히 같다**.
 * 그 갈래는 배율을 **보지 않는다** — 배율은 편집기의 시야이고, 편집이 꺼진 자리에는 시야를
 * 고를 사람이 없다(REQ-05 · 가정 A3).
 *
 * `workspace === true`(편집 중)에서는 잰 상자를 **보기 배율** `zoom` 으로 줄인 상자에
 * 격자를 맞추고, 그 결과를 잰 상자 **가운데**에 앉힌다. 가운데 정렬은 저술 여백을 사방에
 * 고르게 남길 뿐 아니라 `PanelEditGrid` 의 중심 표식(`+`)이 **출력 영역의 중심**을
 * 가리키게 하는 근거이기도 하다 — 자리 계산을 바꾸면 그 표식이 조용히 다른 뜻이 된다.
 *
 * ## 배율이 들어가는 자리는 `reduced` 한 줄뿐이다 (REQ-09 · M9)
 *
 * 기본값이 종전 동작이므로 배율을 넘기지 않은 호출은 0.5.0 과 **한 픽셀도 다르지 않다**.
 * `box` 는 배율과 무관하게 여전히 `outer` 이고(그래서 작업 영역이 잰 상자를 넘는 일이
 * 없다 — 불변식 I22), `origin` 은 여전히 `floor` 라 정수이며, 축척은 여전히 `stageLattice`
 * 가 한 번만 내려 만든 그 하나다. **배율은 새 축척을 만들지 않는다 — 이미 있는 축척의
 * 입력을 바꿀 뿐이다.**
 *
 * **배율은 요청이고 축척은 결과이며 둘은 같지 않다**(가정 A20). `stageLattice` 가 칸을
 * 정수 px 로 한 번 내리므로 실현 가능한 축척은 사실상 `1/step` 눈금이고, 이웃한 백분율이
 * **같은 그림**을 낼 수 있다. 그 `floor` 는 걷어내지 않는다 — 사용자가 세 번 되돌려보낸
 * "격자가 일정하지 않음" 을 막고 있는 것이 그 한 줄이다(불변식 I5). 대가는 숨기지 않고
 * 화면의 상시 도움말이 말한다(위험 R24).
 *
 * 퇴화 처리는 새로 만들지 않는다. `stageLattice` 가 이미 소유한다(잰 상자 0 → 결과 0,
 * 한 칸이 1px 미만 → 정수화하지 않고 소수 축척 하나를 그대로 쓴다). 006 이 그 위에 더한
 * 것은 `floor` 넷뿐이며 그 넷은 NaN 을 만들지 않는다. 범위 죔은 여기가 아니라
 * `clampWorkspaceZoom` 이 진다 — 죈 값만 이 자리에 들어오고, 그 아래는 종전의 퇴화 처리가
 * 그대로 받친다.
 */
export function workspaceBox(
  outer: StageSize,
  canvas: CanvasSize,
  step: number,
  workspace: boolean,
  zoom: number = DEFAULT_WORKSPACE_ZOOM,
): CanvasWorkspaceBox {
  const outerW = positiveOrZero(outer.width);
  const outerH = positiveOrZero(outer.height);

  if (!workspace) {
    const lat = stageLattice({ width: outerW, height: outerH }, canvas, step);
    return {
      box: lat.stage,
      offset: lat.offset,
      stage: lat.stage,
      origin: { x: 0, y: 0 },
      cell: lat.cell,
    };
  }

  const lat = stageLattice(
    {
      width: Math.floor(outerW * zoom),
      height: Math.floor(outerH * zoom),
    },
    canvas,
    step,
  );
  return {
    box: { width: outerW, height: outerH },
    offset: { x: 0, y: 0 },
    stage: lat.stage,
    origin: {
      x: Math.floor((outerW - lat.stage.width) / 2),
      y: Math.floor((outerH - lat.stage.height) / 2),
    },
    cell: lat.cell,
  };
}

// --- 크기 유도 (SPEC-CANVAS-006 M8 · REQ-07) -----------------------------

/**
 * 잰 한 축을 파서가 죄는 범위로 옮긴다. 범위 밖(아직 재지 못한 축)은 **0** 이며, 그 0 은
 * "1 로 올려라" 가 아니라 **"손대지 말라"** 는 신호다 — 아래 `derivedCanvasSize` 가 그것을
 * 받아 저장값을 그대로 돌려준다.
 *
 * `Math.floor` 는 방어적 정수화 한 겹이다. 표면이 이미 `Math.floor(contentRect)` 로
 * 정수를 넘기지만, 이 함수는 그 사실을 전제하지 않는다 — 소수 캔버스는 파서 왕복에서
 * 반올림되어 저장 왕복이 값을 바꾼다.
 */
function clampDimension(v: number): number {
  if (!Number.isFinite(v)) return 0;
  const n = Math.floor(v);
  if (n < MIN_CANVAS_DIMENSION) return 0;
  return Math.min(n, MAX_CANVAS_DIMENSION);
}

/**
 * 편집 중 캔버스 크기를 잰 바깥 상자에서 유도한다 — **항등이다**(REQ-07 · 0.5.0).
 *
 * ```
 * derivedCanvasSize(canvas, outer) = outer      // MIN..MAX 로 죈다. 어떤 축척도 곱하지 않는다
 * ```
 *
 * ## `canvas` 인자는 계산에 쓰이지 않는다 — 되돌려주기 위해서만 있다
 *
 * 이 함수는 `fitCanvasSizeToStage` 의 계약을 **그대로 물려받는다**: 바꿀 것이 없거나
 * 잴 수 없으면 **받은 그 객체를** 돌려준다. 부르는 쪽의 참조 비교(`derived === stored`)가
 * 곧 쓰기 억제의 전부이며, 새 비교를 만들지 않는다 — 파서가 매 렌더 새 객체를 내므로
 * 값 비교로는 억제되지 않는다.
 *
 * 그래서 `canvas` 를 **인자로 받는 것은 정상이고 값을 읽는 것은 위반**이다(불변식 I19).
 * 저장값이 계산의 입력이 되면 쓰기가 다음 유도의 입력이 되어, 리사이즈 한 번이 최대 세
 * 번의 연쇄 쓰기가 되고 "많아야 한 건"(AC-07 (AL))이 깨진다.
 *
 * ## 인자가 둘인 것이 가드다 (불변식 I20 · 위험 R21 · R22)
 *
 * 0.4.0 의 폐기된 규칙은 `floor(outer × 축척)` 이었고, 그 규칙에서는
 * 캔버스가 영원히 패널의 R 배라 REQ-07 이 제 이름("캔버스 크기 = 패널 크기")을 지킬 수
 * 없었다. 그 부활을 막는 가드는 이제 **이름이 아니라 형상**이다:
 *
 * > 이 함수의 인자는 `(canvas, outer)` **둘뿐**이다. 셋째 인자(격자 간격 · 보기 배율)가
 * > 나타나거나 본문에 어떤 축척을 곱하는 자리가 생기면, 그것이 곧 폐기된 규칙의 부활이다.
 *
 * 형상 가드는 상수를 개명해도 죽지 않는다 — 이름 기반 grep 가드가 개명 한 번에 조용히
 * 무장 해제되는 것과 갈리는 자리다(위험 R23).
 *
 * ## 축척 1 은 여기서 나오지 않는다
 *
 * 편집 중 투영 축척은 `cell ÷ step` 이고 **보기 배율 이하**다 — 작업 영역
 * 전체가 잰 상자에 담기느라 화면이 물러나 있기 때문이며, 그 물러남이 곧 저술 여백이다.
 * 축척 1 은 **편집이 꺼진 채 저술 크기의 패널에 놓였을 때**의 성질이다(불변식 I21).
 */
export function derivedCanvasSize(canvas: CanvasSize, outer: StageSize): CanvasSize {
  const width = clampDimension(outer.width);
  const height = clampDimension(outer.height);
  // "모르면 근사한다" 가 아니라 **"모르면 손대지 않는다"** — 0 은 파서가 죄는 범위 밖이라
  // 저장 왕복에 값이 달라진다.
  if (width === 0 || height === 0) return canvas;
  if (width === canvas.width && height === canvas.height) return canvas;
  return { width, height };
}
