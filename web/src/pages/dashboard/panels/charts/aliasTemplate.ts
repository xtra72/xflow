// 시리즈 이름(alias) 템플릿 리졸버 (SPEC-WEB-005).
//
// 사용자가 데이터 소스에서 선택한 시리즈의 표시 이름을 토큰 + 리터럴 문자열 조합으로
// 조립할 수 있다. 토큰은 그 시리즈가 가진 값으로 치환된다.
//
// 토큰 문법 (프로젝트 공통 `$.` 경로 관례를 따른다):
//   {$.measurement} measurement (시리즈 이름)
//   {$.metric}      field (측정 종류)
//   {$.tags.NAME}   태그 NAME 의 값 (명시 형식)
//   {$.NAME}        태그 NAME 의 값 (단축 형식, 하위 호환)
//
// 예) measurement = "dev-1", field = "temperature", tags = { name: "TempSensor", type: "inside" }
//     "{$.name}-{$.type}"        → "TempSensor-inside"
//     "{$.measurement}/{$.field}"  → "dev-1/temperature"
//     "[{$.tags.type}] {$.measurement}" → "[inside] dev-1"
//
// 규칙:
//   - 단축 형식 `{$.NAME}` 은 태그만 가리킨다. `measurement` / `field` 는 예약어라
//     단축 형식으로 태그를 가리킬 수 없으며, 그 이름의 태그가 실제로 있으면
//     명시 형식 `{$.tags.measurement}` / `{$.tags.field}` 으로 접근한다.
//   - 누락/빈 값 → 빈 문자열로 치환(토큰 사이 리터럴은 그대로 유지).
//   - 토큰이 없는 템플릿 → 입력 문자열을 그대로 반환(plain 텍스트 alias 하위 호환).
//   - 잘못된 중괄호(예: "{$.}", "{name}", "{$ .name}")는 토큰으로 인식되지 않아
//     리터럴 그대로 남는다.
//
// 순수 함수로 유지해 단위 테스트가 가능하도록 한다.
//
// @spec SPEC-WEB-005

/** 시리즈 이름 템플릿을 해석할 때 참조하는 시리즈 값들. */
export interface AliasContext {
  /** measurement (시리즈 이름). */
  measurement?: string;
  /** field (측정 종류). */
  field?: string;
  /** 시리즈 태그. */
  tags?: Record<string, string>;
}

/** 단축 형식 `{$.NAME}` 으로 태그를 가리킬 수 없는 예약 토큰 이름. */
export const RESERVED_ALIAS_TOKENS = ['measurement', 'field'] as const;

/**
 * alias 템플릿의 토큰 정규식.
 * `{$.PATH}` 형태에서 PATH = 토큰 경로([A-Za-z0-9_-]+ 를 `.` 로 이은 형태)를 매칭한다.
 * 캡처 그룹 1 = 토큰 경로 (예: "measurement", "field", "room", "tags.room").
 */
export const ALIAS_TOKEN_REGEX = /\{\$\.([A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+)*)\}/g;

/**
 * 템플릿 문자열에 등장하는 토큰 경로 목록을 등장 순서대로(중복 제거) 반환한다.
 * 토큰이 없으면 빈 배열. 경로는 원문 그대로다(예: "measurement", "tags.room", "room").
 */
export function extractAliasTokens(template: string): string[] {
  if (!template) return [];
  const out: string[] = [];
  const seen = new Set<string>();
  // 전역 정규식 재사용 시 lastIndex 누적을 피하려고 매 호출마다 새 정규식을 만든다.
  const re = new RegExp(ALIAS_TOKEN_REGEX.source, 'g');
  let m: RegExpExecArray | null;
  while ((m = re.exec(template)) !== null) {
    const key = m[1]!;
    if (!seen.has(key)) {
      seen.add(key);
      out.push(key);
    }
  }
  return out;
}

/**
 * 토큰 경로 하나를 시리즈 컨텍스트에서 해석한다. 없으면 빈 문자열.
 *
 * 우선순위: 예약어(key/metric) → 명시 태그(tags.NAME) → 단축 태그(NAME).
 */
function resolveAliasToken(path: string, ctx: AliasContext): string {
  if (path === 'measurement') return ctx.measurement ?? '';
  if (path === 'field') return ctx.field ?? '';
  if (path.startsWith('tags.')) {
    const name = path.slice('tags.'.length);
    return ctx.tags?.[name] ?? '';
  }
  // 단축 형식은 태그만 가리킨다. 알 수 없는 경로도 빈 문자열로 떨어진다.
  return ctx.tags?.[path] ?? '';
}

/**
 * alias 템플릿의 `{$.…}` 토큰을 시리즈 값으로 치환한다.
 *
 * - 누락/빈 값 → 빈 문자열.
 * - 토큰이 아닌 리터럴 텍스트는 그대로 유지한다.
 * - 토큰이 전혀 없으면 입력을 그대로 반환한다(plain 텍스트 alias 하위 호환).
 *
 * @param template 사용자 입력 alias(토큰 포함 가능).
 * @param ctx 시리즈의 키 / field / 태그. 하위 호환을 위해 태그 맵만 넘겨도 된다.
 */
export function resolveSeriesAlias(
  template: string,
  ctx: AliasContext | Record<string, string>,
): string {
  if (!template) return template;
  // 태그 맵만 넘어온 기존 호출 형태를 컨텍스트로 승격한다(하위 호환).
  const context: AliasContext =
    'tags' in ctx || 'measurement' in ctx || 'field' in ctx
      ? (ctx as AliasContext)
      : { tags: ctx as Record<string, string> };
  // 전역 플래그 정규식은 lastIndex 상태를 가지므로 매 호출마다 새로 만든다.
  const re = new RegExp(ALIAS_TOKEN_REGEX.source, 'g');
  return template.replace(re, (_match, path: string) => resolveAliasToken(path, context));
}

/**
 * 토큰 경로에 대한 토큰 문자열을 만든다(`{$.PATH}`).
 * 에디터의 "토큰 삽입" 버튼 라벨/삽입 값에 사용한다.
 */
export function makeAliasToken(path: string): string {
  return `{$.${path}}`;
}

/**
 * 시리즈 컨텍스트에서 삽입 가능한 토큰 경로 목록을 만든다.
 *
 * 키 / field 이 있으면 예약 토큰을 먼저 놓고, 이어서 태그 토큰을 붙인다.
 * 태그 이름이 예약어와 겹치면 단축 형식이 예약어에 가려지므로 명시 형식
 * (`tags.NAME`)으로 노출한다.
 */
export function availableAliasTokens(ctx: AliasContext): string[] {
  const out: string[] = [];
  if (ctx.measurement) out.push('measurement');
  if (ctx.field) out.push('field');
  for (const name of Object.keys(ctx.tags ?? {})) {
    out.push(
      (RESERVED_ALIAS_TOKENS as readonly string[]).includes(name) ? `tags.${name}` : name,
    );
  }
  return out;
}

/**
 * 템플릿에서 **실재하지 않는 대상**을 가리키는 토큰 경로를 등장 순서대로 반환한다.
 *
 * 알 수 없는 토큰은 빈 문자열로 치환되므로, 오타나 없는 태그 이름을 쓰면 결과에서
 * 조용히 사라진다 — `{$.location}-{$.device.id}` 가 `실습실-` 이 되어 "형식이
 * 동작하지 않는다" 로 보인다. 편집기가 그 사실을 알려 줄 수 있도록 판정만 제공한다.
 *
 * 값이 비어 있는 것과 대상이 없는 것은 다르다. 여기서는 **대상의 존재**만 본다.
 */
export function unknownAliasTokens(template: string, ctx: AliasContext): string[] {
  const tags = ctx.tags ?? {};
  return extractAliasTokens(template).filter((path) => {
    if (path === 'measurement' || path === 'field') return false;
    if (path.startsWith('tags.')) return !(path.slice('tags.'.length) in tags);
    return !(path in tags);
  });
}
