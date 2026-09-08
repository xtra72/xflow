// 신규 캔버스 요소 생성 — 목록 편집기와 도형 팔레트가 **함께 쓰는 한 구현**
// (SPEC-CANVAS-002 T9 · 가정 A7).
//
// 이 모듈이 왜 따로 있는가: 001 은 요소 생성을 `CanvasElementsEditor.tsx` 안의 모듈 지역
// 함수(`nextElementId` · `seedOffset` · `newElement`)로 두었다. 002 의 팔레트는 캔버스
// 옆에서 **같은 것을 만들어야** 하므로, 그 함수들을 여기로 옮겨 두 호출부가 하나를
// 부르게 한다. 팔레트가 자기 생성 규칙을 갖는 순간 "어디서 더했는가" 에 따라 씨앗 기하·
// 계단 오프셋·id 규칙이 달라지고, 그것은 사용자에게 **원인 없는 차이**로 보인다
// (spec.md §도형 팔레트 · 가정 A7).
//
// **DOM 무의존이다.** React 도 i18n 도 부르지 않으므로 jsdom 없이 전량 단위 테스트된다 —
// `canvasHitTest.ts` · `canvasEditGeometry.ts` 와 같은 규율이다.
//
// **여기서 심는 값은 전부 config 에 실린다.** 렌더 층은 색 없는 요소를 아무것도 그리지
// 않으므로(`drawElement.paintFill` — "기본 색을 지어내면 사용자가 색을 지정하지 않음을
// 표현할 수 없다") 만드는 쪽이 보이는 스타일을 심는다. 그릴 때 지어내면 화면에만 있고
// 어디에도 없다.
//
// @spec SPEC-CANVAS-002 REQ-01

import {
  DEFAULT_BOX_GEOMETRY,
  DEFAULT_LINE_GEOMETRY,
  DEFAULT_POINT_GEOMETRY,
  type CanvasElement,
  type CanvasElementKind,
} from './canvasConfig';

// --- 씨앗 상수 -----------------------------------------------------------

/**
 * 신규 요소가 입고 나오는 색.
 *
 * 값은 팔레트 첫 칸과 같은 blue-500 이다(`colorPalette.ts`). 2D context 는 실제 색
 * 문자열을 요구하므로 CSS 변수를 쓸 수 없고, 그래서 히트맵 프리셋과 같은 hex 리터럴이다
 * (`heatmapColorPresets.ts`).
 *
 * 이 색은 밝은 패널 표면(#fbfcfe)에 3.60:1, 어두운 표면(#1f2937)에 3.99:1 로 닿는다.
 * 한 색으로 두 표면을 동시에 만족시킬 수 있는 상한 자체가 3.79:1 이므로(두 표면의 명도가
 * 양 끝이라 그 이상은 대수적으로 불가능하다) 이 값이 그 상한에 가장 가까운 축이며,
 * 두 표면 모두에서 비문자 대비 기준 3:1 을 넘는다. 어차피 사용자가 갈아입힐 씨앗 색이다.
 */
export const SEED_COLOR = '#3b82f6';

/** 신규 선의 두께(px). 기본값 1 은 고DPI 표면에서 실오라기라 "그려졌다" 로 읽히지 않는다. */
export const SEED_STROKE_WIDTH = 2;

/**
 * 신규 문구 템플릿. 토큰 3종을 한 줄에 모아 **문구 칸 자체가 사용법이 되게** 한다.
 *
 * 번역하지 않는다 — 이 문자열은 config 에 저장되어 대시보드를 함께 쓰는 다른 로케일의
 * 사용자에게도 그대로 그려진다. 로케일이 config 에 박히는 것은 바인딩을 표시 이름이
 * 아니라 동일성 키로 참조하는 것과 같은 이유로 피한다.
 */
export const SEED_TEXT = '{name} {value}{unit}';

/** 겹침 방지 계단의 한 칸(정규화 좌표). */
const SEED_OFFSET_STEP = 0.05;

/** 계단이 스테이지를 벗어나기 전에 처음으로 되감는 칸 수. */
const SEED_OFFSET_WRAP = 8;

// --- 순수 도우미 ---------------------------------------------------------

/** 배열 안에서 쓰이지 않은 요소 id 를 만든다(결정적 — 테스트가 값을 예측할 수 있다). */
export function nextElementId(elements: readonly CanvasElement[]): string {
  const used = new Set(elements.map((e) => e.id));
  let n = 1;
  while (used.has(`el-${n}`)) n++;
  return `el-${n}`;
}

/**
 * n 번째 신규 요소의 계단 변위. 되감으므로 **스테이지 밖으로 행진하지 않는다.**
 *
 * 이미 있는 요소 수만 보므로 결정적이다 — 같은 순서로 누르면 같은 자리가 나온다.
 * 기존 요소의 좌표는 건드리지 않는다(스테이지 밖 저술은 합법이다). 새 요소가 어디서
 * 시작하는지만 정한다.
 */
export function seedOffset(count: number): number {
  return (count % SEED_OFFSET_WRAP) * SEED_OFFSET_STEP;
}

/** 계단을 더한 좌표. 0.1 + 0.15 가 0.25000000000000006 으로 새지 않게 자른다. */
function shifted(base: number, off: number): number {
  return Math.round((base + off) * 1000) / 1000;
}

/**
 * 신규 요소. **보이는 스타일을 심어** 내보낸다 — 위 `SEED_COLOR` 주석 참조.
 *
 * 계단은 종류마다 여유가 있는 축으로만 준다. 상자는 대각선(우하), 선은 가로로 이미
 * 스테이지를 가로지르므로 세로로만, 문구는 오른쪽으로 흘러가므로 세로로만 내린다.
 * 어느 쪽도 되감기 전에 1 을 넘지 않는다.
 */
export function newElement(id: string, kind: CanvasElementKind, count: number): CanvasElement {
  const off = seedOffset(count);
  switch (kind) {
    case 'rect':
      return {
        id,
        kind,
        geometry: {
          x: shifted(DEFAULT_BOX_GEOMETRY.x, off),
          y: shifted(DEFAULT_BOX_GEOMETRY.y, off),
          w: DEFAULT_BOX_GEOMETRY.w,
          h: DEFAULT_BOX_GEOMETRY.h,
        },
        style: { fill: SEED_COLOR },
      };
    case 'ellipse':
      return {
        id,
        kind,
        geometry: {
          x: shifted(DEFAULT_BOX_GEOMETRY.x, off),
          y: shifted(DEFAULT_BOX_GEOMETRY.y, off),
          w: DEFAULT_BOX_GEOMETRY.w,
          h: DEFAULT_BOX_GEOMETRY.h,
        },
        style: { fill: SEED_COLOR },
      };
    case 'line':
      return {
        id,
        kind,
        geometry: {
          x1: DEFAULT_LINE_GEOMETRY.x1,
          y1: shifted(DEFAULT_LINE_GEOMETRY.y1, off),
          x2: DEFAULT_LINE_GEOMETRY.x2,
          y2: shifted(DEFAULT_LINE_GEOMETRY.y2, off),
        },
        // 선은 채우지 않으므로(열린 경로) 색만으로는 그려지지 않는다 — 두께를 함께 심는다.
        style: { stroke: SEED_COLOR, strokeWidth: SEED_STROKE_WIDTH },
      };
    default:
      return {
        id,
        kind,
        geometry: {
          x: DEFAULT_POINT_GEOMETRY.x,
          y: shifted(DEFAULT_POINT_GEOMETRY.y, off),
        },
        style: { textColor: SEED_COLOR },
        // 색만 심으면 여전히 보이지 않는다 — 그릴 글자가 없기 때문이다.
        text: SEED_TEXT,
      };
  }
}

/**
 * 새 요소를 **배열 끝에 붙인** 결과. 목록 편집기의 추가 버튼과 캔버스 팔레트가 **둘 다
 * 이 함수를 부른다**(가정 A7).
 *
 * 붙이는 자리까지 여기서 정하는 이유: id 규칙(`nextElementId`)과 계단 오프셋
 * (`seedOffset`)이 **둘 다 현재 배열을 본다.** 두 호출부가 각자 `newElement` 를 부르면서
 * 인자를 스스로 조립하면, 그 조립이 곧 갈라질 수 있는 두 번째 지점이 된다.
 *
 * 끝에 붙이는 것 자체가 뜻이 있다 — 배열 순서가 001 의 유일한 z-order 이므로 **새로 만든
 * 것이 맨 위에 온다**. 만든 직후 보이지 않으면 사용자는 다시 누른다.
 *
 * 만든 요소를 함께 돌려주는 것은 호출부마다 **그 뒤에 할 일이 다르기** 때문이다: 목록
 * 편집기는 그 행을 펼치고, 팔레트는 그것을 캔버스에서 고른다. 배열에서 다시 찾게 하면
 * 그 탐색이 또 하나의 규칙이 된다.
 */
export function appendElement(
  elements: readonly CanvasElement[],
  kind: CanvasElementKind,
): { next: CanvasElement[]; created: CanvasElement } {
  const created = newElement(nextElementId(elements), kind, elements.length);
  return { next: [...elements, created], created };
}
