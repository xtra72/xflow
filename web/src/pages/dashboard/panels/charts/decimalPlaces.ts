// 값 표기 소수점 자릿수 — 차트 계열 패널이 공유하는 단일 정본.
//
// 종전에는 자릿수 규칙이 패널마다 흩어져 있었다. 통계는 `?? 2`, 라인은 "미지정이면
// 원값 그대로", 게이지·바·파이·테이블은 아예 반올림이 없어 `21.533333333333335` 가
// 그대로 나왔고, 히트맵은 `toFixed(1)` 이 박혀 있었다. 같은 대시보드 안에서 같은
// 시리즈가 패널마다 다른 자릿수로 읽히는 상태다.
//
// 규칙을 여기 하나로 모은다. 패널은 `readDecimalPlaces(config)` 로 자릿수를 받고
// `formatDecimal` 로 찍는다.
//
// **축 눈금은 이 기본값을 따르지 않는다.** 눈금은 값 읽기가 아니라 눈금자이고,
// Recharts 가 이미 "보기 좋은 수"(0 · 25 · 50)를 고른다. 거기에 기본 2자리를 걸면
// 아무 설정도 안 한 패널의 축이 `0.00 · 25.00 · 50.00` 이 된다. 그래서 축은
// `hasExplicitDecimalPlaces` 로 **사용자가 값을 직접 넣었을 때만** 자릿수를 따른다 —
// 사용자가 의견을 냈으면 축과 값이 같은 자릿수여야 한다는 기존 규약은 그대로다.

/** 설정이 비어 있을 때의 자릿수. */
export const DEFAULT_DECIMAL_PLACES = 2;

/**
 * 허용 상한. `toFixed` 는 0~100 을 받지만 그 이상은 표기가 아니라 잡음이다.
 * 하한은 0(정수 표기)이다.
 */
export const MAX_DECIMAL_PLACES = 10;

/** 수를 자릿수 범위 안으로 죈다. 수가 아니면 기본값. */
function clampDecimals(raw: unknown): number {
  if (typeof raw !== 'number' || !Number.isFinite(raw)) return DEFAULT_DECIMAL_PLACES;
  return Math.min(Math.max(Math.trunc(raw), 0), MAX_DECIMAL_PLACES);
}

/**
 * 패널 config 에서 값 표기 자릿수를 읽는다. 미지정이면 {@link DEFAULT_DECIMAL_PLACES}.
 *
 * 범위를 벗어난 값(음수 · 100 · NaN)도 여기서 걸러진다 — 손으로 편집한 config 나
 * 구버전 config 가 `toFixed` 를 던지게 두지 않는다.
 */
export function readDecimalPlaces(config: Record<string, unknown> | undefined): number {
  return clampDecimals(config?.decimal_places);
}

/**
 * 사용자가 자릿수를 **직접 지정**했는가.
 *
 * 축 눈금처럼 "기본값은 따르지 않지만 명시 설정은 따르는" 자리에서 쓴다. 값이
 * 수가 아니면(미지정 · 빈 문자열) 거짓이다.
 */
export function hasExplicitDecimalPlaces(config: Record<string, unknown> | undefined): boolean {
  const raw = config?.decimal_places;
  return typeof raw === 'number' && Number.isFinite(raw);
}

/**
 * 수를 지정 자릿수로 찍는다. 유한수가 아니면 원문을 그대로 돌려준다.
 *
 * `NaN`/`Infinity` 에 `toFixed` 를 걸면 `"NaN"`/`"Infinity"` 가 나오는데, 그것은
 * "값이 없다"와 구별되지 않는다. 호출부가 빈 상태를 따로 그릴 수 있도록 원문을 남긴다.
 */
export function formatDecimal(value: number, decimals: number): string {
  if (!Number.isFinite(value)) return String(value);
  return value.toFixed(clampDecimals(decimals));
}
