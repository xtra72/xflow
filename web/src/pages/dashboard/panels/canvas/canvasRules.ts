// 조건 규칙 표 평가기 (SPEC-CANVAS-001 T5).
//
// 규칙 표가 존재하는 이유는 **식 언어를 도입하지 않기 위해서**다. 한 행은
// `[바인딩 값][비교 연산자][임계값] → [스타일·문구 패치]` 이고, 표의 위에서부터
// 처음 일치한 행 하나만 이긴다(first-match-wins, REQ-04). 그래서 이 파일에는
// 파서도, `eval` 도, 동적 함수 생성도 없다 — 비교 연산자 7종과 결측 판정 1종이
// 전부다.
//
// 이 어휘는 새로 만든 것이 아니라 코드베이스에 이미 있는 임계값 어휘와 같다:
// `charts/thresholdFill.ts` 의 경계 채우기, `acControlColors.ts` 의
// `resolveValueColor`(위에서부터 처음 매치되는 구간이 이긴다)와 같은 개념이며,
// 사용자가 새 문법을 배우지 않도록 의도적으로 그 규율을 그대로 따른다.
//
// DOM 무의존이다 — `document` · `window` · `canvas` 를 쓰지 않는다. 그리기는
// `drawElement.ts`(T7)가, 살아 있는 일치 표시는 설정 UI(T8)가 이 모듈의 결과만
// 소비한다.
//
// @spec SPEC-CANVAS-001

import type { ElementStyle, RuleOp, RuleRow, RuleValue, StylePatch } from './canvasConfig';

/**
 * `eq` · `ne` 의 허용 오차 계수.
 *
 * 부동소수 비교라 정확 일치를 요구하면 `0.1 + 0.2 === 0.3` 이 거짓이 되어 사용자가
 * 화면에서 설명할 수 없는 불일치를 본다(§명세 "eq/ne 는 부동소수 비교이므로 구현은
 * 허용 오차를 둔다").
 *
 * 오차는 **상대·절대 혼합**이다: `|a-b| <= EPSILON * max(1, |a|, |b|)`.
 * - 0 부근에서는 `max(1, ...)` 가 1 이라 절대 오차 1e-9 로 동작한다(0.1+0.2 대 0.3 의
 *   오차는 약 5.6e-17 이므로 통과).
 * - 큰 수(예: 1e12)에서는 상대 오차로 커져 배정밀도가 애초에 표현하지 못하는 자리를
 *   같다고 본다. 순수 절대 오차라면 큰 수에서 영원히 불일치하고, 순수 상대 오차라면
 *   0 근처에서 영원히 불일치한다 — 둘 다 사용자가 임계값을 못 맞추는 형태다.
 *
 * 배정밀도의 상대 정밀도(~2.2e-16)보다 넉넉한 1e-9 를 고른 이유는, 이 값이 센서
 * 원값이 아니라 **사용자가 손으로 적은 임계값**과 비교되기 때문이다. 소수 몇 자리
 * 수준의 저술 오차는 흡수하고, 사용자가 의미 있게 구분하는 자리(0.001 단위 등)는
 * 그대로 구분한다.
 */
export const RULE_EPSILON = 1e-9;

/**
 * 규칙 평가가 확정한 최종 스타일.
 *
 * `ElementStyle` 을 그대로 확장하므로 **`ElementStyle` 에 대입 가능**하다 — 트윈
 * 엔진(T9)이 `ElementStyle` 로 타입 지어져 있고 이 결과를 그대로 먹기 때문이다.
 * 필수 속성을 새로 추가하면 그 대입 가능성이 깨지므로 추가하지 않는다.
 *
 * `text` 는 **일치한 행이 문구 패치를 담았을 때만** 존재한다. 없으면 "요소의 기본
 * 문구 템플릿을 쓰라" 는 뜻이다 — 기본 문구는 스타일이 아니라 요소(`CanvasElement.text`)
 * 에 있으므로 이 함수는 그것을 알지 못하고, 알 필요도 없다.
 */
export type ResolvedStyle = ElementStyle & { text?: string };

/**
 * 결측을 하나의 형태로 접는다. null · undefined · NaN 은 모두 "값이 없다" 이며,
 * 이것이 `nodata` 연산자의 정의다(REQ-05, AC-E2).
 *
 * Infinity 는 결측이 **아니다** — 값이 있고 그 값이 무한대다. 그래서 `gt` 같은
 * 비교로 정상 흐르며, 어디서도 NaN 을 만들지 않는다.
 */
function readValue(value: number | null | undefined): number | null {
  if (value === null || value === undefined || Number.isNaN(value)) return null;
  return value;
}

/**
 * 허용 오차를 둔 동등 비교.
 *
 * `a === b` 를 먼저 보는 이유는 두 가지다: 정확히 같은 값을 곱셈 없이 통과시키고,
 * `Infinity === Infinity` 를 참으로 만든다. 그 뒤 비유한 값을 걸러 내지 않으면
 * `EPSILON * max(1, Infinity, 80)` 이 Infinity 가 되어 **무한대가 모든 수와 같아진다**.
 */
function approxEquals(a: number, b: number): boolean {
  if (a === b) return true;
  if (!Number.isFinite(a) || !Number.isFinite(b)) return false;
  const scale = Math.max(1, Math.abs(a), Math.abs(b));
  return Math.abs(a - b) <= RULE_EPSILON * scale;
}

/**
 * 스칼라 연산자의 임계값을 꺼낸다.
 *
 * 파서는 스칼라 연산자 행에 유한 숫자만 남기지만, 설정 UI(T8)는 사용자가 연산자를
 * `between` 에서 `gt` 로 바꾸는 **편집 도중**의 행을 그대로 넘길 수 있다. 그때 판정을
 * 던지는 대신 첫 원소를 임계값으로 본다 — 화면의 살아 있는 표시가 예외로 죽지 않는다.
 */
function scalarThreshold(value: RuleValue): number {
  return Array.isArray(value) ? value[0] : value;
}

/**
 * `between` 의 [하한, 상한]. 저술 순서를 여기서 정규화한다.
 *
 * 파서는 사용자가 적은 순서를 그대로 보존하므로(`[80, 50]` 도 유효한 행이다),
 * 순서 뒤집기는 평가기의 몫이다. 스칼라 값이 잘못 들어오면 그 값 하나만의 축퇴
 * 구간으로 본다(`scalarThreshold` 와 같은 편집 도중 방어).
 */
function betweenBounds(value: RuleValue): readonly [number, number] {
  if (!Array.isArray(value)) return [value, value];
  const [a, b] = value;
  return a <= b ? [a, b] : [b, a];
}

/**
 * 결측이 아닌 값에 대한 비교. `nodata` 를 제외한 7종이 전부이므로 switch 가 완전하며,
 * 도달 불가 분기를 남기지 않는다.
 */
function compare(v: number, op: Exclude<RuleOp, 'nodata'>, threshold: RuleValue): boolean {
  switch (op) {
    case 'gt':
      return v > scalarThreshold(threshold);
    case 'gte':
      return v >= scalarThreshold(threshold);
    case 'lt':
      return v < scalarThreshold(threshold);
    case 'lte':
      return v <= scalarThreshold(threshold);
    case 'eq':
      return approxEquals(v, scalarThreshold(threshold));
    case 'ne':
      return !approxEquals(v, scalarThreshold(threshold));
    case 'between': {
      // 경계 포함이다(§명세 "between(경계 포함)").
      const [lo, hi] = betweenBounds(threshold);
      return v >= lo && v <= hi;
    }
  }
}

/**
 * 규칙 행 하나가 이 값에 일치하는가.
 *
 * 설정 UI(T8)가 "이 행이 지금 일치 중" 표시를 그리려고 비교 규칙을 다시 구현하지
 * 않도록 공개한다 — 표시와 실제 렌더가 갈라지면 사용자는 화면을 믿을 수 없다.
 *
 * `nodata` 는 표의 어느 위치에 두어도 되며, 그 순서가 곧 "결측 우선순위" 다. 그래서
 * 결측 판정을 규칙 표 밖의 별도 개념으로 두지 않는다(§명세).
 */
export function matchesRule(value: number | null | undefined, row: RuleRow): boolean {
  const v = readValue(value);
  if (row.op === 'nodata') return v === null;
  // 나머지 연산자는 값이 있어야 성립한다. 결측은 오류가 아니라 조용한 불일치다(AC-E2).
  if (v === null) return false;
  return compare(v, row.op, row.value);
}

/**
 * 패치를 기본 스타일 위에 덮어쓴다.
 *
 * 규율은 둘이다. (1) 패치에 없는 속성은 기본 스타일을 그대로 유지한다. (2) 키는
 * 있는데 값이 `undefined` 인 것도 "덮어쓰지 않음" 이다 — JSON 왕복이나 UI 편집에서
 * 흔히 생기는 형태이며, 그것으로 기본값을 지워 버리면 사용자가 지운 적 없는 색이
 * 사라진다.
 *
 * `ElementStyle` 전 필드를 복사한다. 001 이 설정 UI 에 노출하는 패치 대상은 명세대로
 * 8종(fill · stroke · strokeWidth · opacity · textColor · fontWeight · visible · text)이지만
 * `StylePatch` 타입은 그보다 넓다(`Partial<ElementStyle>`). 좁은 8종만 복사하면 타입이
 * 허용하는 `fontSize` · `align` 패치가 **조용히 무시되어** 003 이 노출 범위를 넓힐 때
 * 원인 없는 버그가 된다. 노출 범위를 좁히는 축은 설정 UI 이지 평가기가 아니다.
 */
function mergePatch(base: ElementStyle, patch: StylePatch): ResolvedStyle {
  const out: ResolvedStyle = { ...base };
  if (patch.fill !== undefined) out.fill = patch.fill;
  if (patch.stroke !== undefined) out.stroke = patch.stroke;
  if (patch.strokeWidth !== undefined) out.strokeWidth = patch.strokeWidth;
  if (patch.opacity !== undefined) out.opacity = patch.opacity;
  if (patch.fontSize !== undefined) out.fontSize = patch.fontSize;
  if (patch.fontWeight !== undefined) out.fontWeight = patch.fontWeight;
  if (patch.textColor !== undefined) out.textColor = patch.textColor;
  if (patch.align !== undefined) out.align = patch.align;
  if (patch.visible !== undefined) out.visible = patch.visible;
  if (patch.text !== undefined) out.text = patch.text;
  return out;
}

/**
 * 규칙 표를 평가해 최종 스타일을 낸다.
 *
 * - **첫 일치 승리**: 위에서부터 평가하다 처음 일치한 행에서 멈춘다. 뒤 행은 평가되지
 *   않으므로 그 행의 패치는 결과에 섞이지 않는다(AC-02).
 * - **미일치 폴백**: 어떤 행도 일치하지 않거나 표가 없으면 기본 스타일 그대로다.
 *   이것은 오류가 아니라 정상 경로다(AC-E3, 가정 A8).
 * - **순수 함수**: `baseStyle` · 행 · 패치를 변형하지 않고 매번 새 객체를 돌려준다.
 *   렌더 루프가 프레임마다 이 함수를 부르므로, 여기서 입력을 건드리면 다음 프레임의
 *   기본 스타일이 오염된다.
 */
export function evaluateRules(
  value: number | null | undefined,
  rules: RuleRow[] | undefined,
  baseStyle: ElementStyle,
): ResolvedStyle {
  for (const row of rules ?? []) {
    if (matchesRule(value, row)) return mergePatch(baseStyle, row.patch);
  }
  return { ...baseStyle };
}
