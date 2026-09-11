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
  type BoxGeometry,
  DEFAULT_LINE_GEOMETRY,
  DEFAULT_POINT_GEOMETRY,
  type CanvasElement,
  type CanvasPrimitiveKind,
  type ElementStyle,
  type PathElement,
} from './canvasConfig';
// SPEC-CANVAS-004 M6 — **담는 그릇만 넓어졌다.** 만드는 것은 여전히 `CanvasElement` 이고
// (`created` 의 타입이 그 사실을 든다) 새 입구도 늘지 않았다 — 008 불변식 J9 그대로다.
// 넓히지 않았다면 팔레트로 도형 하나를 놓는 순간 손으로 저술한 그룹이 배열에서 떨어진다.
import type { CanvasNode } from './group/groupTypes';
import type { PathCommand } from './shapes/pathTypes';

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

/** 신규 선의 두께(px — 화면 양이지 캔버스 단위가 아니다). 1 은 고DPI 표면에서 실오라기다. */
export const SEED_STROKE_WIDTH = 2;

/**
 * 신규 문구 템플릿. 토큰 3종을 한 줄에 모아 **문구 칸 자체가 사용법이 되게** 한다.
 *
 * 번역하지 않는다 — 이 문자열은 config 에 저장되어 대시보드를 함께 쓰는 다른 로케일의
 * 사용자에게도 그대로 그려진다. 로케일이 config 에 박히는 것은 바인딩을 표시 이름이
 * 아니라 동일성 키로 참조하는 것과 같은 이유로 피한다.
 */
export const SEED_TEXT = '{name} {value}{unit}';

/**
 * 도형(rect · ellipse · line)에 **문구가 처음 붙을 때** 심는 글자색.
 *
 * 왜 만드는 쪽이 심는가: 렌더 층은 도형 라벨을 `textColor` **하나로만** 칠하고 `fill` 로
 * 폴백하지 않는다 — 폴백하면 라벨이 제 도형과 같은 색이 되어 보이지 않기 때문이다
 * (`drawElement.drawMeasuredElement` 의 "문구 색 규칙" 주석). 그 거절은 옳으므로 색은
 * 저술 시점에 심는다. 그리는 쪽이 지어내면 화면에만 있고 config 에는 없어, 편집기의
 * 글자색 칸이 계속 비어 사용자가 그 색을 갈아입힐 수도 없다.
 *
 * 값의 근거(WCAG 상대 명도 대비):
 *   - 도형의 씨앗 채움 blue-500(#3b82f6) 위  **2.00:1** — rect·ellipse 라벨은 도형 한가운데 앉는다
 *   - 밝은 패널 표면(#fbfcfe) 위             **7.15:1** — 선 라벨은 2px 선 위, 즉 사실상 표면 위에 앉는다
 *   - 어두운 패널 표면(#1f2937) 위           **2.00:1**
 *
 * 세 배경을 **한 색으로** 동시에 만족시킬 수 있는 상한 자체가 1.998:1 이다. blue-500 의
 * 상대 명도(0.2355)가 두 표면의 명도(0.9729 · 0.0215) **사이**에 놓여 있어, 셋 모두에서
 * 멀어지려면 어두운 표면과 blue-500 사이의 기하 중간(상대 명도 0.0929)에 서는 수밖에
 * 없고 그 지점의 대비가 1.998:1 이다. 이 값이 정확히 그 지점이므로 **어느 배경 위에서도
 * 사라지지 않는다.** 더 센 대비를 한쪽에서 얻으려면 다른 쪽을 버려야 한다: 흰색은 밝은
 * 표면에서 1.03:1, 검정에 가까운 색은 어두운 표면에서 1.22:1 이라 각각 절반의 사용자에게
 * **지금 고치는 그 결함**(문구를 적었는데 아무것도 보이지 않는다)을 그대로 되돌려준다.
 *
 * 무채색인 것에도 뜻이 있다. 명도 대비가 2.00:1 로 묶여 있으므로 blue-500 과의 분리는
 * 채도가 마저 맡는다 — 같은 명도의 회청색을 고르면 "도형 색의 어두운 버전" 으로 읽혀,
 * 폴백 금지 규율이 막으려던 그 모습에 다시 가까워진다.
 *
 * `SEED_COLOR` 와 같이 어차피 사용자가 갈아입힐 씨앗이다. 다만 갈아입히기 전에도 보인다.
 */
export const SEED_TEXT_COLOR = '#565656';

/**
 * 겹침 방지 계단의 한 칸(정수 캔버스 단위).
 *
 * 기본 캔버스(500 × 400)의 5% 자리 — 정규화 시절의 `0.05` 를 그 크기로 옮겨 적은 값이라
 * 연속으로 놓았을 때 흩어지는 모습이 종전과 같다. 캔버스 크기와 무관한 상수인 것에 뜻이
 * 있다: 계단은 "겹치지 않을 만큼만" 어긋나면 되고, 캔버스가 작으면 되감기가 더 빨리
 * 돌아올 뿐이다.
 */
const SEED_OFFSET_STEP = 25;

/** 계단이 캔버스를 벗어나기 전에 처음으로 되감는 칸 수. */
const SEED_OFFSET_WRAP = 8;

// --- 순수 도우미 ---------------------------------------------------------

/**
 * 배열 안에서 쓰이지 않은 요소 id 를 만든다(결정적 — 테스트가 값을 예측할 수 있다).
 *
 * 인자가 **`id` 만 요구**하는 것에 뜻이 있다(SPEC-CANVAS-004 M5). 그룹해제는 요소와
 * 그룹이 섞인 최상위 배열을 상대로 id 를 발급해야 하고, 그때 **id 규칙이 둘이 되지 않는
 * 것**이 요구다. 읽는 것이 `e.id` 하나뿐이므로 이 넓히기는 동작을 한 글자도 바꾸지 않는다.
 */
export function nextElementId(elements: readonly { id: string }[]): string {
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

/**
 * 계단을 더한 좌표.
 *
 * 정수끼리의 덧셈이라 자를 부동소수 꼬리가 없다 — 정규화 시절 이 함수가 `0.1 + 0.15` 를
 * `0.25000000000000006` 으로 흘리지 않으려고 하던 반올림은 이제 항등이다. 그래도 함수를
 * 남기는 것은 씨앗 상수가 손으로 고쳐질 때(예: 소수 한 칸) 정수 좌표계로 들어오는 문이
 * 여기 하나이기 때문이다.
 */
function shifted(base: number, off: number): number {
  return Math.round(base + off);
}

/**
 * 신규 요소. **보이는 스타일을 심어** 내보낸다 — 위 `SEED_COLOR` 주석 참조.
 *
 * 계단은 종류마다 여유가 있는 축으로만 준다. 상자는 대각선(우하), 선은 가로로 이미
 * 캔버스를 가로지르므로 세로로만, 문구는 오른쪽으로 흘러가므로 세로로만 내린다.
 * 어느 쪽도 되감기 전에 기본 캔버스를 벗어나지 않는다(최대 7칸 × 25 = 175 단위).
 *
 * **받는 것은 원시형 넷뿐이다**(`CanvasPrimitiveKind`). 경로는 씨앗 기하만으로 만들어지지
 * 않는다 — 무슨 명령 목록을 지어낼 것인가에 대한 답이 이 함수에는 없기 때문이다. 경로를
 * 만드는 입구는 카탈로그가 명령 목록을 들고 오는 자리이며 같은 모듈에 선다(M6). 입구가
 * 여전히 이 모듈 하나인 것이 요점이고(불변식 J9), 인자를 넷으로 좁혀 두면 "씨앗 경로로
 * 아무거나 만들어 두자" 는 우회로가 타입에서 막힌다.
 */
export function newElement(id: string, kind: CanvasPrimitiveKind, count: number): CanvasElement {
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
    // `default:` 가 아니라 이름으로 적는다 — 다섯 번째 원시형이 들어오면 컴파일러가 이
    // 자리를 가리켜야 하고, `default:` 는 그 요소를 조용히 문구로 만들어 버린다.
    case 'text':
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

// --- 경로 요소 -----------------------------------------------------------

/**
 * 카탈로그 도형이 입고 나오는 씨앗 스타일. **명령 목록이 닫혔는가**로 갈린다.
 *
 * 두 갈래 다 **새 상수를 만들지 않는다** — 닫힌 것은 `rect` 의 씨앗 그대로(채움만), 열린
 * 것은 `line` 의 씨앗 그대로(선과 두께)다. 원시형이 이미 쓰는 두 벌을 그대로 고르는 것이라
 * 카탈로그가 제 색 규칙을 갖지 않는다.
 *
 * 갈라야 하는 이유는 `fill()` 의 규칙에 있다 — 그 함수는 열린 부분 경로를 **암묵적으로
 * 닫아** 채우므로, 곡선 화살표처럼 닫히지 않은 윤곽을 채우면 **저술한 적 없는 변**이 하나
 * 생겨 화면에 정체 모를 덩어리가 나온다.
 *
 * **대가를 적어 둔다:** 정육면체의 모서리 선 · 메모의 접힘 선 · 원통의 앞쪽 테두리는 선으로만
 * 그려지는 부분 경로라, 채움만 심긴 씨앗에서는 **보이지 않는다**(셋 다 채움과 같은 색이 될
 * 테두리를 그리는 셈이기 때문이다). 사용자가 테두리 색을 고르면 그때 드러난다. 대비되는
 * 두 번째 씨앗 색을 여기서 지어내는 쪽을 **기각한다** — 그 색은 채움색과 표면색 양쪽에
 * 대해 근거를 대야 하고, 그것은 `SEED_TEXT_COLOR` 주석이 한 번 치른 값이다.
 */
export function pathSeedStyle(commands: readonly PathCommand[]): ElementStyle {
  return commands.some((cmd) => cmd.c === 'Z')
    ? { fill: SEED_COLOR }
    : { stroke: SEED_COLOR, strokeWidth: SEED_STROKE_WIDTH };
}

/**
 * 카탈로그 도형 하나로 만든 경로 요소.
 *
 * `newElement` 와 갈라 두는 이유는 인자에 있다 — 경로는 **씨앗 기하만으로 만들어지지
 * 않는다.** 무슨 명령 목록을 지어낼 것인가에 대한 답이 그 함수에는 없고, 답을 들고 오는
 * 것은 카탈로그다. 입구가 여전히 이 모듈 하나인 것이 요점이다(불변식 J9).
 *
 * **명령을 사본으로 싣는다.** 카탈로그의 배열은 얼려 있으므로 그대로 실으면 요소가
 * 카탈로그를 **가리키게** 되고, 그때부터 "이미 놓은 도형이 앱 갱신에 따라 달라지는가" 라는
 * 질문이 생긴다. 값이지 참조가 아니다(spec.md §경로 자료는 값인가 참조인가 · REQ-02).
 *
 * 상자는 `rect` 와 **같은 씨앗 기하 · 같은 계단**을 쓴다. 경로의 기하가 `BoxGeometry` 인
 * 덕에 여기서 따로 정할 것이 없다.
 */
export function newPathElement(
  id: string,
  catalogId: string,
  commands: readonly PathCommand[],
  count: number,
): PathElement {
  const off = seedOffset(count);
  return {
    id,
    kind: 'path',
    geometry: {
      x: shifted(DEFAULT_BOX_GEOMETRY.x, off),
      y: shifted(DEFAULT_BOX_GEOMETRY.y, off),
      w: DEFAULT_BOX_GEOMETRY.w,
      h: DEFAULT_BOX_GEOMETRY.h,
    },
    path: commands.map((cmd) => ({ ...cmd })),
    // 표시·감사용이다. 렌더가 읽지 않으므로 결측이거나 모르는 값이어도 그림은 완전하다.
    catalog_id: catalogId,
    style: pathSeedStyle(commands),
  };
}

/**
 * 새 요소를 **배열 끝에 붙인** 결과. 요소를 만드는 **유일한 입구**다 — 종전에는 목록
 * 편집기의 추가 버튼도 함께 불렀으나, 그 줄이 사라지고 캔버스 팔레트 하나만 남았다.
 *
 * 붙이는 자리까지 여기서 정하는 이유: id 규칙(`nextElementId`)과 계단 오프셋
 * (`seedOffset`)이 **둘 다 현재 배열을 본다.** 호출부가 각자 `newElement` 를 부르면서
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
  elements: readonly CanvasNode[],
  kind: CanvasPrimitiveKind,
): { next: CanvasNode[]; created: CanvasElement } {
  const created = newElement(nextElementId(elements), kind, elements.length);
  return { next: [...elements, created], created };
}

/**
 * 카탈로그 도형을 배열 끝에 붙인 결과. `appendElement` 와 **같은 규칙**을 쓴다 — id 발급도
 * 계단 오프셋도 현재 배열을 보고, 끝에 붙는 것이 곧 맨 위다.
 */
export function appendPathElement(
  elements: readonly CanvasNode[],
  catalogId: string,
  commands: readonly PathCommand[],
): { next: CanvasNode[]; created: CanvasElement } {
  const created = newPathElement(
    nextElementId(elements),
    catalogId,
    commands,
    elements.length,
  );
  return { next: [...elements, created], created };
}

// --- 가져온 요소 ---------------------------------------------------------

/**
 * 가져오기가 들고 오는 도형 하나. **`svgimport/` 의 타입을 수입하지 않는다.**
 *
 * 구조가 같으면 통과하므로 형상만 여기 적는다. 수입하면 `svgimport/svgDocument` 가 이미
 * 이 모듈의 `SEED_COLOR` 를 값으로 가져가고 있어 **모듈 순환**이 생기고, 무엇보다 요소를
 * 만드는 모듈이 **가져오기라는 특정 출처를 알게 된다** — 훗날 다른 벡터 형식이 들어와도
 * 이 입구가 그대로 서 있으려면 몰라야 하는 사실이다.
 */
export interface ImportedPathSource {
  /** 요소 상자 로컬 정수. 이미 정규화되어 있다. */
  readonly commands: readonly PathCommand[];
  /** 이 도형이 차지하는 상자. **계단 오프셋은 아직 더해지지 않았다** — 아래가 더한다. */
  readonly box: BoxGeometry;
  /** 알파가 색에 접힌 스타일. 비어 있을 수 있다. */
  readonly style: ElementStyle;
  /** 원본이 칠을 한 마디라도 말했는가 — 아니면 `pathSeedStyle` 이 선다. */
  readonly hasOwnStyle: boolean;
}

/**
 * 가져온 도형들을 배열 **끝에 문서 순서대로** 붙인 결과.
 *
 * **계단 오프셋을 무리 전체에 한 번만 더한다**(REQ-06). 요소마다 더하면 문서에서 겹쳐
 * 그려지던 조각들이 25 단위씩 어긋나 **그림이 흩어진다** — 카탈로그 도형을 연달아 놓을
 * 때 계단이 하는 바로 그 일이, 한 그림을 이루는 조각들에는 결함이 된다.
 *
 * **상자는 도형마다 다르되 오프셋은 하나다.** 도형은 제 기하가 차지하는 최소 영역을 상자로
 * 들고 오고(그래야 손잡이가 제 잉크에 닿고 정렬이 산다), 이 함수는 그 **모든** 상자에 **같은**
 * 오프셋을 더한다. 같은 값을 더하므로 문서에서의 상대 배치가 그대로 보존된다 — 요소마다
 * 다른 오프셋을 주면 그때 그림이 25 단위씩 흩어진다.
 *
 * **`catalog_id` 를 심지 않는다.** 그 필드는 "어느 카탈로그 도형에서 나왔는가" 를 뜻하고
 * 가져온 요소에는 카탈로그가 없다. 예약값을 넣으면 한 필드가 두 뜻을 갖는다. 대가는 요소
 * 목록에서 일반적인 "경로" 이름으로 보인다는 것이며, 새 필드를 만드는 것보다 싸다.
 *
 * **명령을 사본으로 싣는다.** 계획이 들고 있는 배열을 그대로 실으면 요소가 그 배열을
 * **가리키게** 되고, 미리보기와 놓인 요소가 같은 목록을 공유한다.
 *
 * 만든 요소들을 함께 돌려주는 것은 호출부가 그 뒤에 **전량을 선택으로 세우기** 때문이다
 * (REQ-06) — 배열에서 다시 찾게 하면 그 탐색이 또 하나의 규칙이 된다.
 */
export function appendImportedElements(
  elements: readonly CanvasNode[],
  shapes: readonly ImportedPathSource[],
): { next: CanvasNode[]; created: PathElement[] } {
  // **한 번 센다.** 안에서 `next.length` 로 세면 요소가 하나 붙을 때마다 오프셋이 자라
  // 조각들이 계단으로 흩어진다 — REQ-06 이 금지하는 그것이다.
  const off = seedOffset(elements.length);
  const next: CanvasNode[] = [...elements];
  const created: PathElement[] = [];
  for (const shape of shapes) {
    const path = shape.commands.map((cmd) => ({ ...cmd }));
    const element: PathElement = {
      id: nextElementId(next),
      kind: 'path',
      geometry: {
        x: shifted(shape.box.x, off),
        y: shifted(shape.box.y, off),
        w: shape.box.w,
        h: shape.box.h,
      },
      path,
      // 원본이 아무 칠도 말하지 않았으면 **두 번째 씨앗 규칙을 만들지 않고** 008 의 것을
      // 쓴다(닫힘이면 채움, 열림이면 선). 말한 것이 있으면 그것이 이긴다.
      style: shape.hasOwnStyle ? { ...shape.style } : { ...pathSeedStyle(path), ...shape.style },
    };
    next.push(element);
    created.push(element);
  }
  return { next, created };
}

// --- 문구 편집 -----------------------------------------------------------

/**
 * 도형 라벨이 그려질 조건을 다 갖췄는데 **글자색만 없는가.**
 *
 * `paintText` 의 거절 조건(`text === undefined || text === '' || paint === undefined`)을
 * 그대로 뒤집어 적었다 — 이 술어가 참인 요소가 곧 렌더 층이 조용히 건너뛰는 요소다.
 * 빈 문자열을 따로 보는 것은 호출부가 `''` 를 접어 준다는 가정을 여기서 하지 않기
 * 위해서다. 렌더 층이 두 값을 똑같이 거절하므로 여기서도 똑같이 본다.
 *
 * `kind: 'text'` 는 제외한다. 문구 요소는 `textColor ?? fill` 로 칠해지고 씨앗도 이미
 * 글자색을 심으므로(위 `newElement`), 이 자리에서 더 심을 것이 없다.
 */
function needsLabelColor(el: CanvasElement): boolean {
  return (
    el.kind !== 'text' &&
    el.text !== undefined &&
    el.text !== '' &&
    el.style.textColor === undefined
  );
}

/**
 * 요소의 문구 템플릿을 갈아끼운다. **도형에 문구가 생기는 순간 글자색을 함께 심는다.**
 *
 * 이 함수가 있는 이유가 곧 고쳐진 결함이다. 사용자가 rect 의 문구 칸에 `{value}` 를 적어도
 * 화면에는 아무것도 나오지 않았다 — 씨앗은 `style: { fill }` 만 심고(`newElement`), 렌더
 * 층은 색 없는 라벨을 (설계대로) 건너뛰기 때문이다. 두 쪽 다 자기 계약을 지켰는데 사이가
 * 비어 있었고, 그 사이를 메우는 자리는 **저술 시점**이다(`SEED_TEXT_COLOR` 주석).
 *
 * 심는 규칙이 지켜야 하는 것 셋:
 *   1. **사용자가 고른 색을 덮지 않는다** — 이미 `textColor` 가 있으면 손대지 않는다.
 *   2. **문구를 지운다고 색을 걷어 가지 않는다** — 지웠다 다시 적는 흔한 편집에서 색이
 *      사라지면, 사용자는 자기가 고른 색을 잃는다. 지우기는 `text` 키만 지운다.
 *   3. **여기 한 곳에만 있다** — 씨앗 스타일·계단 오프셋·id 규칙과 같은 자리다(가정 A7).
 *      편집기가 자기 판단으로 색을 심으면 규칙이 둘이 되고, 그때부터 "어디서 적었는가" 에
 *      따라 결과가 달라진다.
 *
 * 규칙 1 의 여집합에는 **이미 config 에 들어 있는 색 없는 라벨**도 들어온다. 그 요소의
 * 문구를 손대면 그때 색이 심긴다 — 결함이 만들어 둔 보이지 않는 라벨이 손대는 순간
 * 고쳐지는 것이며, 그 편이 "이 요소만은 영원히 보이지 않는다" 보다 낫다.
 */
export function withElementText(el: CanvasElement, text: string | undefined): CanvasElement {
  const next: CanvasElement = { ...el };
  if (text === undefined) delete next.text;
  else next.text = text;
  // 스타일은 새 객체로 갈아끼운다 — 받은 요소의 style 을 제자리에서 고치면 호출부가 아직
  // 들고 있는 이전 배열의 요소까지 함께 바뀐다.
  if (needsLabelColor(next)) next.style = { ...next.style, textColor: SEED_TEXT_COLOR };
  return next;
}
