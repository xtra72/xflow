// 선의 파선 무늬 — 이름 넷과 그 무늬 (SPEC-CANVAS-012 M2).
//
// ## 008 이 거절한 축을 012 가 받는다
//
// `setLineDash` 는 SPEC-CANVAS-008 이 이름으로 거절한 셋 가운데 하나다. 그 거절의 문장을
// 그대로 옮긴다:
//
//   > `clip` · `lineJoin` · `lineCap` · `setLineDash` — 앞의 하나는 이 SPEC 이 쓰지 않고,
//   > 뒤의 셋은 **기하 축이 아니라 스타일 축**이다. 들이면 `ElementStyle` 이 넓어지고
//   > 규칙 패치와 004 의 캐스케이드가 함께 넓어진다. **008 의 주제가 아니다.**
//
// 마지막 문장이 이 파일이 설 수 있는 이유다 — **영구 금지가 아니라 범위 판정**이었고,
// 012 의 주제가 정확히 그 스타일 축이다. 008 이 계산서로 내민 값 셋(`ElementStyle` 이
// 넓어진다 · 규칙 패치가 넓어진다 · 캐스케이드가 넓어진다)은 **깎지 않고 치른다.** 깎으려
// 들면 "연결선에만 있는 두 번째 스타일 자료형" 이 생기고, 그것은 008 이 막으려던 것보다
// 나쁘다.
//
// ## 왜 이름 넷이고 길이 배열이 아닌가 (§결정 1)
//
// 저장할 수 있는 모양이 둘이었다 — 이름(`'dash'`) 또는 날것의 길이 배열(`[6, 3]`).
// 배열은 세 자리에서 값을 치른다.
//
//   1. **규칙 패치**가 배열을 덮어쓰면 캐스케이드가 "배열 대 배열" 비교를 하게 된다.
//   2. **트윈**이 그 축을 만나면 길이가 다른 두 배열 사이를 보간할 길이가 없다.
//   3. **UI** 가 자유 입력이 되어 `[0.0001]` 같은 값이 들어온다.
//
// 이름 넷은 셋 다 만들지 않는다: 비교는 문자열 동등, 트윈은 이 축을 **구조적으로**
// 건너뛰고(아래), UI 는 `<select>` 다.
//
// ## 이 파일이 잎인 이유 (§결정 D5)
//
// 무늬 표를 `drawElement` 안에 두면 **파서가 유효한 이름을 판정하려고 렌더 모듈을 들이게**
// 되고, 그 방향은 이 저장소가 `connectorTypes` · `anchorTypes` · `pathTypes` 로 세 번 피해
// 온 고리다. 이름 · 기본값 · 판별 · 무늬를 한 잎이 들면 파서와 렌더가 **같은 표**를 본다.
//
// **이 파일은 DOM 도 캔버스도 모른다.** 자료형과 판별과 산술뿐이다.
//
// @spec SPEC-CANVAS-012 REQ-04 · REQ-06

/**
 * 선을 긋는 네 가지 무늬.
 *
 * **`solid` 는 미지정과 같은 그림이되 같은 값이 아니다.** 미지정은 캐스케이드 상위가
 * 정할 수 있는 자리이고 `solid` 는 저술자가 정한 값이다 — `visible` 이 체크박스가 아니라
 * 3지 선택인 것과 **같은 판단**이며, 그 구분이 없으면 "규칙이 점선으로 바꾸게 두겠다" 와
 * "무슨 일이 있어도 실선이다" 를 표현할 방법이 사라진다.
 */
export type StrokeDash = 'solid' | 'dash' | 'dot' | 'dashDot';

/**
 * 고를 수 있는 이름 전부. **이 배열이 유일한 목록이다**(판별 · UI · 시험이 함께 읽는다).
 *
 * 목록을 여기 한 벌만 두는 것에 뜻이 있다. 소비 측이 저마다 넷을 적으면 그중 하나가
 * 다섯째를 얻는 날 "어떤 화면에는 있고 어떤 화면에는 없는 무늬" 가 시작된다 —
 * `connectorTypes` 의 `CONNECTOR_KIND` 가 문자열 하나에 대해 쓴 그 규율이다.
 */
export const STROKE_DASH_NAMES: readonly StrokeDash[] = ['solid', 'dash', 'dot', 'dashDot'];

/** 모르는 값이 떨어지는 자리. 무늬 없는 선이 파선보다 안전한 기본이다. */
export const DEFAULT_STROKE_DASH: StrokeDash = 'solid';

/**
 * 날것의 값이 이름 넷 가운데 하나인가 — **판별의 유일한 자리**다.
 *
 * 파서가 제 손으로 네 문자열을 적으면 목록이 둘이 되고, 그중 하나만 다섯째를 얻는 날
 * 저장에서는 통과한 값이 화면에서 조용히 사라진다.
 */
export function isStrokeDash(value: unknown): value is StrokeDash {
  return typeof value === 'string' && (STROKE_DASH_NAMES as readonly string[]).includes(value);
}

/**
 * 무늬를 **선 두께의 배수로** 낸다 (§결정 3).
 *
 * 절대 길이(`[6, 3]`)를 쓰지 않는 이유는 화면에서 바로 보인다: 두께 8 짜리 선의 점선은
 * **점이 아니라 덩어리**가 되고, 두께 0.5 짜리 선의 점선은 실선과 구분되지 않는다. 무늬는
 * 두께에 대한 **비**여야 어느 두께에서도 같은 선으로 읽힌다.
 *
 * | 이름 | 무늬 |
 * |------|------|
 * | `solid`(미지정 포함) | `[]` — 빈 배열이 곧 실선이다(canvas 명세) |
 * | `dash` | `[3w, 2w]` |
 * | `dot` | `[w, 2w]` |
 * | `dashDot` | `[3w, 2w, w, 2w]` |
 *
 * **두께는 받아 온다 — 여기서 재지 않는다.** 부르는 쪽(`paintStroke`)이 이미
 * `resolveStrokeWidth` 로 구한 값이 있고, 그것을 다시 구하면 **두 번째 측정**이 생긴다.
 * 그 부류가 이 저장소에서 반복해 값을 치른 함정이다(007 §미리보기는 실제 렌더 경로다).
 *
 * 두께가 유한하지 않거나 0 이하면 빈 배열이다 — 그런 선은 어차피 칠해지지 않는다.
 */
export function dashPattern(dash: StrokeDash | undefined, strokeWidth: number): number[] {
  if (dash === undefined || dash === 'solid') return [];
  if (!Number.isFinite(strokeWidth) || strokeWidth <= 0) return [];
  const w = strokeWidth;
  switch (dash) {
    case 'dash':
      return [3 * w, 2 * w];
    case 'dot':
      return [w, 2 * w];
    case 'dashDot':
      return [3 * w, 2 * w, w, 2 * w];
  }
}
