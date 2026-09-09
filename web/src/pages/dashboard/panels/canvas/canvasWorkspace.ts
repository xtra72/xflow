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
// @spec SPEC-CANVAS-006

import type { CanvasSize } from './canvasConfig';
import { stageLattice, type StageCell, type StageSize } from './canvasGeometry';

/**
 * 출력 영역이 잰 바깥 상자에서 차지하는 **선형** 비율.
 *
 * 곱으로 보면 출력 영역은 상자 넓이의 56%(0.75²)를 차지하므로 여전히 화면의 주인이고,
 * 각 변에는 출력 영역 제 길이의 약 1/6 에 해당하는 저술 여백이 생긴다.
 *
 * **이름 하나로 모아 두는 것이 요점이다**(위험 R10). 축소가 부담스럽다는 것이 드러나면
 * 한 줄 수정이 되어야 한다 — 그리고 더 크게 보는 길은 이미 화면에 있다(미리보기 확대와
 * 이동). 캔버스 안에 두 번째 확대·이동 기구를 만들지 않는다(가정 A8).
 */
export const PANEL_REGION_FIT_RATIO = 0.75;

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
 *
 * `workspace === true`(편집 중)에서는 잰 상자를 `PANEL_REGION_FIT_RATIO` 로 줄인 상자에
 * 격자를 맞추고, 그 결과를 잰 상자 **가운데**에 앉힌다. 가운데 정렬은 저술 여백을 사방에
 * 고르게 남길 뿐 아니라 `PanelEditGrid` 의 중심 표식(`+`)이 **출력 영역의 중심**을
 * 가리키게 하는 근거이기도 하다 — 자리 계산을 바꾸면 그 표식이 조용히 다른 뜻이 된다.
 *
 * 퇴화 처리는 새로 만들지 않는다. `stageLattice` 가 이미 소유한다(잰 상자 0 → 결과 0,
 * 한 칸이 1px 미만 → 정수화하지 않고 소수 축척 하나를 그대로 쓴다). 006 이 그 위에 더한
 * 것은 `floor` 넷뿐이며 그 넷은 NaN 을 만들지 않는다.
 */
export function workspaceBox(
  outer: StageSize,
  canvas: CanvasSize,
  step: number,
  workspace: boolean,
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
      width: Math.floor(outerW * PANEL_REGION_FIT_RATIO),
      height: Math.floor(outerH * PANEL_REGION_FIT_RATIO),
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
