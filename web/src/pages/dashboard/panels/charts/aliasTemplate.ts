// 시리즈 별칭(alias) 태그 토큰 템플릿 리졸버 (SPEC-WEB-005).
//
// 사용자가 데이터 소스의 선택된 시리즈 이름을 `{$.<tagKey>}` 토큰으로 지정할 수 있다.
// 각 토큰은 그 시리즈에 설정된 태그 값(StoreSeriesRef.tags[tagKey])으로 치환된다.
// 여러 토큰 + 토큰 사이의 리터럴 텍스트를 함께 사용할 수 있다.
//
// 예) tags = { name: "TempSensor", type: "inside" }
//     템플릿 "{$.name}-{$.type}" → "TempSensor-inside"
//
// 규칙:
//   - 토큰 문법: `{$.KEY}` (KEY 는 태그 키 문자셋 [A-Za-z0-9_-]+).
//   - 누락/빈 태그 키 → 빈 문자열로 치환(리터럴은 그대로 유지).
//   - 토큰이 없는 템플릿 → 입력 문자열을 그대로 반환.
//   - 잘못된 중괄호(예: "{$.}", "{name}", "{$ .name}")는 토큰으로 인식되지 않아
//     리터럴 그대로 남는다.
//
// 순수 함수로 유지해 단위 테스트가 가능하도록 한다.
//
// @spec SPEC-WEB-005

/**
 * alias 템플릿의 태그 토큰 정규식.
 * `{$.KEY}` 형태에서 KEY = 태그 키 문자셋([A-Za-z0-9_-]+) 만 매칭한다.
 * 캡처 그룹 1 = 태그 키.
 */
export const ALIAS_TOKEN_REGEX = /\{\$\.([A-Za-z0-9_-]+)\}/g;

/**
 * 템플릿 문자열에 등장하는 태그 토큰 키 목록을 등장 순서대로(중복 제거) 반환한다.
 * 토큰이 없으면 빈 배열.
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
 * alias 템플릿의 `{$.KEY}` 토큰을 tags[KEY] 값으로 치환한다.
 *
 * - 누락/빈 태그 → 빈 문자열.
 * - 토큰이 아닌 리터럴 텍스트는 그대로 유지한다.
 * - 토큰이 전혀 없으면 입력을 그대로 반환한다(plain 텍스트 alias 하위 호환).
 *
 * @param template 사용자 입력 alias(토큰 포함 가능).
 * @param tags 시리즈에 설정된 태그 맵.
 */
export function resolveSeriesAlias(
  template: string,
  tags: Record<string, string>,
): string {
  if (!template) return template;
  // 전역 플래그 정규식은 lastIndex 상태를 가지므로 매 호출마다 새로 만든다.
  const re = new RegExp(ALIAS_TOKEN_REGEX.source, 'g');
  return template.replace(re, (_match, key: string) => {
    const v = tags[key];
    return v === undefined || v === null ? '' : v;
  });
}

/**
 * 단일 태그 키에 대한 토큰 문자열을 만든다(`{$.key}`).
 * 에디터의 "토큰 삽입" 버튼 라벨/삽입 값에 사용한다.
 */
export function makeAliasToken(tagKey: string): string {
  return `{$.${tagKey}}`;
}
