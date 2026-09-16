// 불투명도의 **화면 단위**와 저장 단위 사이 환산 (SPEC-CANVAS-012 M5 · REQ-05).
//
// ## 저장은 0..1 그대로다 (§결정 4)
//
// 사용자가 요구한 것은 "투명도를 0~100% 사이의 값으로 지정" 이다. 그 요구는 **칸의
// 단위**에 대한 것이지 저장 형식에 대한 것이 아니며, 저장을 100 눈금으로 바꾸면 셋이
// 한꺼번에 깨진다.
//
//   1. **이미 저장된 대시보드가 전부 100 배 불투명해진다.** 마이그레이션 없이는 `0.5` 가
//      `0.5%` 로 읽힌다. 마이그레이션을 쓰면 그 코드가 영구히 남는다.
//   2. **규칙 패치 · 트윈 · SVG 가져오기가 같은 축을 다른 눈금으로 읽게 된다.** 셋 다
//      `ElementStyle.opacity` 를 직접 읽어 `resolveAlpha` 로 흘려보낸다.
//   3. **`globalAlpha` 가 0..1 이다.** 저장을 100 눈금으로 두면 환산이 렌더 경로로 내려간다.
//
// 그래서 환산은 **편집 칸이 진다.** 읽을 때 ×100, 쓸 때 ÷100.
//
// ## 왜 함수 둘이 한 파일에 사는가 (AC-28)
//
// 이 환산을 쓰는 칸이 **넷**이다 — 요소 · 그룹 · 규칙 표 · 연결선. 네 자리가 제각기
// `* 100` 과 `/ 100` 을 적으면 그중 하나가 반올림을 다르게 하는 날이 오고, 그때 "같은
// 값인데 칸마다 다르게 보인다" 가 시작된다. 두 함수가 한 자리에 있으면 그 갈라짐이
// 표현 불가능하다.
//
// **이 파일은 DOM 도 React 도 모른다.** 산술뿐이다.
//
// @spec SPEC-CANVAS-012 REQ-05

/** 칸이 받는 최댓값. `min` 은 0 이다. */
export const OPACITY_PERCENT_MAX = 100;

/** 0..1 로 죈다. 비유한 값은 여기 오기 전에 걸러진다. */
function clamp01(v: number): number {
  return v < 0 ? 0 : v > 1 ? 1 : v;
}

/**
 * 저장값(0..1) → 칸에 보일 정수 백분율.
 *
 * 미지정과 손상 값은 **빈 칸**이다 — 0 으로 보이면 "지정하지 않음" 과 "완전히 투명함" 이
 * 구분되지 않는다. 그 둘은 `ElementStyle` 이 처음부터 가르는 것이다(미지정 = 렌더측 기본).
 *
 * **범위 밖 저장값은 죄어서 보인다.** 규칙 표는 종전에 0..1 로 죄지 않았으므로 `1.5` 가
 * 저장에 남아 있을 수 있는데, 그 값은 어차피 `resolveAlpha` 가 1 로 죄어 그린다 — 칸이
 * `150` 을 보이면 **그려지지 않는 수**를 말하게 된다. 보이는 수가 그려지는 수와 같은 쪽을
 * 고른다.
 */
export function opacityToPercentInput(opacity: number | undefined): number | '' {
  if (opacity === undefined || !Number.isFinite(opacity)) return '';
  return Math.round(clamp01(opacity) * OPACITY_PERCENT_MAX);
}

/**
 * 칸에 적힌 백분율 → 저장값(0..1).
 *
 * 빈 칸과 읽을 수 없는 입력은 **미지정**이다(키를 지운다). 범위 밖은 죈다(REQ-06).
 *
 * **정수로 반올림한 뒤 나눈다.** 칸이 `step=1` 이므로 사람이 치는 값은 정수지만, 붙여넣기와
 * IME 는 그 검증을 지나간다 — 브라우저의 `step` 에 기대지 않는 것이 이 저장소의 관례다
 * (`GeometryInput` 이 정수 좌표에 대해 같은 판단을 한다).
 */
export function percentInputToOpacity(raw: string): number | undefined {
  const trimmed = raw.trim();
  if (trimmed === '') return undefined;
  const n = Number(trimmed);
  if (!Number.isFinite(n)) return undefined;
  const pct = Math.round(n);
  const clamped = pct < 0 ? 0 : pct > OPACITY_PERCENT_MAX ? OPACITY_PERCENT_MAX : pct;
  return clamped / OPACITY_PERCENT_MAX;
}
