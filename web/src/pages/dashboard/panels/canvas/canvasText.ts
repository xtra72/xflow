// 문구 템플릿 토큰 치환 (SPEC-CANVAS-001 T6).
//
// 명세 §문구 템플릿은 이것을 "토큰 3종의 단순 치환" 으로 못박는다 — `{value}` ·
// `{name}` · `{unit}` 만 바꾸고 그 밖의 것은 손대지 않는다. 식(expression) 평가가
// 아니며 `eval` · `Function()` · 템플릿 리터럴 평가 · 서식 미니 언어를 쓰지 않는다
// (§품질 게이트 Secured). 규칙 표가 파서를 대신하는 이유와 같은 이유다.
//
// DOM 무의존이고 **로케일 무의존**이다. `toLocaleString` 계열을 쓰지 않는 것은
// 취향이 아니라 제약이다 — 이 모듈을 소비하는 `usePanelSeriesData` 경로의 훅은
// I18n Provider 없이도 테스트에서 돌아야 한다(REQ-03 및 기존 훅 제약).
//
// @spec SPEC-CANVAS-001

import { DEFAULT_DECIMALS } from './canvasConfig';

// --- 상수 ---------------------------------------------------------------

/** 값이 결측일 때 `{value}` 자리에 들어가는 기본 표기(명세 §문구 템플릿, AC-E2). */
export const DEFAULT_MISSING_MARKER = '-';

/**
 * `decimals` 상한. `Number.prototype.toFixed` 는 인자가 0..100 밖이면 RangeError 를
 * 던진다 — 손상된 config 한 칸이 렌더 예외가 되는 길이므로 죄어 둔다(REQ-05).
 * 20 은 모든 런타임에서 유효한 보수적 상한이며, 소수 20자리 위는 배정도 부동소수의
 * 유효 자릿수를 이미 넘어서 화면에 뜻이 없다.
 */
export const MAX_DECIMALS = 20;

// --- 타입 ---------------------------------------------------------------

/** 토큰 치환 입력. 모든 필드가 옵셔널이며, 결측은 예외가 아니라 정상 폴백이다. */
export interface TextTemplateContext {
  /** 바인딩 시리즈의 최신값. 결측(null/undefined/NaN/Infinity)이면 `missing` 으로 간다. */
  value?: number | null;
  /** 시리즈 표시명. `{name}` 치환 값. */
  name?: string;
  /** 요소의 단위. `{unit}` 치환 값. */
  unit?: string;
  /** `{value}` 반올림 소수 자리. 미지정·손상이면 `DEFAULT_DECIMALS`. */
  decimals?: number;
  /** 결측 표기. 미지정이면 `DEFAULT_MISSING_MARKER`. */
  missing?: string;
}

// --- 내부 도우미 ---------------------------------------------------------

/**
 * 치환 대상 토큰만 잡는 패턴. **정의된 3종만** 나열한다 — 미지 토큰(`{foo}`)은
 * 애초에 일치하지 않으므로 원문 그대로 남고, 사용자가 자신의 오타를 화면에서 본다
 * (명세 §문구 템플릿). 이것은 폴백이 아니라 의도된 동작이다.
 *
 * 정규식 리터럴은 `lastIndex` 상태를 갖는 `g` 플래그 객체라 모듈 상수로 재사용하면
 * 호출 간 상태가 샌다 — `String.prototype.replace` 는 매 호출 `lastIndex` 를 0 으로
 * 되돌리므로 안전하지만, 그 사실에 기대지 않도록 매 호출 새로 만든다.
 */
function tokenPattern(): RegExp {
  return /\{(value|name|unit)\}/g;
}

/** 부재·비문자열은 빈 문자열로 본다. 이름/단위 축의 폴백이다. */
function asText(raw: unknown): string {
  return typeof raw === 'string' ? raw : '';
}

/**
 * 소수 자리를 죈다. 부재·비유한(NaN/Infinity)·음수는 기본값으로 떨어뜨리고,
 * 소수 입력은 잘라내며(toFixed 가 어차피 버린다), 상한을 넘으면 clamp 한다.
 */
function resolveDecimals(raw: unknown): number {
  if (typeof raw !== 'number' || !Number.isFinite(raw) || raw < 0) return DEFAULT_DECIMALS;
  const truncated = Math.trunc(raw);
  return truncated > MAX_DECIMALS ? MAX_DECIMALS : truncated;
}

/** 결측 표기. 빈 문자열도 뜻이 있어(아무것도 안 보이기) 길이 검사를 하지 않는다. */
function resolveMissing(raw: unknown): string {
  return typeof raw === 'string' ? raw : DEFAULT_MISSING_MARKER;
}

/**
 * `{value}` 자리에 들어갈 문자열. 유한 숫자만 서식하고 나머지는 결측 표기다.
 * `toFixed` 는 로케일 비의존이라 어느 로케일에서도 같은 문자열을 준다 — 이 모듈이
 * `toLocaleString` 을 쓰지 않는 이유이며, 자리 구분자·소수점 기호가 환경마다
 * 달라지면 스냅샷도 인수 기준도 성립하지 않는다.
 */
function formatValue(value: unknown, decimals: number, missing: string): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return missing;
  return value.toFixed(decimals);
}

// --- 공개 API ------------------------------------------------------------

/**
 * 문구 템플릿의 토큰 3종(`{value}` · `{name}` · `{unit}`)을 치환한다.
 *
 * 계약:
 * - **단순 치환**이다. 식 평가가 아니며 동적 코드 생성을 하지 않는다.
 * - **미지 토큰은 원문 유지**다(`{foo}` → `{foo}`).
 * - **단일 패스**다. 치환 결과에 다시 토큰 모양이 들어 있어도(예: `name` 이
 *   `"{value}"` 인 경우) 재귀 확장하지 않는다 — `replace` 는 치환 결과를 다시
 *   훑지 않으므로 이 성질은 구조가 보장한다.
 * - **치환 문자열의 `$` 특수 시퀀스가 발동하지 않는다**. `replace` 의 두 번째 인자를
 *   문자열로 주면 `$&`·`$1`·``$` `` 이 치환 결과 안에서 해석되어, 이름이나 단위에
 *   `$&` 가 섞이면 출력이 조용히 망가진다. 그래서 **치환자 함수**를 쓴다 — 함수의
 *   반환값은 있는 그대로 삽입된다.
 * - **예외를 던지지 않는다**. config 는 사용자 데이터이므로 템플릿이 문자열이 아니면
 *   빈 문자열을 돌려준다(REQ-05 견고성).
 */
export function renderTextTemplate(template: string, ctx: TextTemplateContext = {}): string {
  // 방어: config 왕복에서 숫자·null·객체가 올 수 있다. 그릴 것이 없다는 뜻이므로 빈 문구다.
  if (typeof template !== 'string') return '';

  const decimals = resolveDecimals(ctx.decimals);
  const missing = resolveMissing(ctx.missing);
  const valueText = formatValue(ctx.value, decimals, missing);
  const nameText = asText(ctx.name);
  const unitText = asText(ctx.unit);

  return template.replace(tokenPattern(), (_match, token: string) => {
    switch (token) {
      case 'value':
        return valueText;
      case 'name':
        return nameText;
      // 이름·단위 미지정은 빈 문자열로 간다(결측 표기가 아니다). 결측 표기는 "값이
      // 있어야 하는데 없다" 는 값 축의 신호이고, 단위 미지정은 "붙일 단위가 없다" 는
      // 정상 저술이라 `-` 를 넣으면 없는 결측을 화면에 만들어낸다.
      default:
        return unitText;
    }
  });
}
