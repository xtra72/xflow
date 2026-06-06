// 원격 자원 편집 시 마스킹 시크릿 생략(REQ-I07, §5.9-6 "필드 부재 + 노드 backfill").
//
// 미러링된 정의는 노드가 redaction(F06)하여 시크릿 키 자체가 제거된 상태로
// 서버 웹 UI 에 도달한다(internal/api/handler/secret_fields.go 의
// RedactSensitiveConfig 는 값을 자리표시자로 치환하지 않고 키를 통째로 삭제한다).
//
// 따라서 원격 편집 저장(PATCH)에서 UI 가 지켜야 할 규칙은:
//   - 사용자가 새 값을 입력하지 않은 시크릿 필드는 갱신 페이로드에서 완전히 생략한다.
//     (마스킹 자리표시자/빈 값을 와이어로 보내지 않는다.) 노드가 어댑터 Create/Update
//     호출 전 기존 정의를 로드하여 부재한 시크릿 필드를 기존값으로 backfill 한다.
//   - 사용자가 실제로 새 값을 입력한 시크릿 필드는 그대로 포함한다(노드가 덮어쓴다).
//
// 본 모듈은 백엔드 secret_fields.go 의 sensitiveConfigKeys 와 정확히 동일한 키 집합을
// 유지해야 하는 단일 진실 공급원(프론트 측)이다. 변경 시 양쪽을 함께 수정한다.
// 키 비교는 항상 대소문자 무시(case-insensitive)로 수행한다.

/**
 * 보안 민감 config 키 집합 (소문자 정규).
 *
 * 백엔드 `internal/api/handler/secret_fields.go` 의 `sensitiveConfigKeys` 와
 * 정확히 동일해야 한다.
 */
export const SENSITIVE_CONFIG_KEYS: ReadonlySet<string> = new Set([
  'password',
  'token',
  'secret',
  'api_key',
  'apikey',
  'access_token',
  'auth_token',
  'client_secret',
  'private_key',
  'passphrase',
  'username',
]);

/**
 * 주어진 키가 보안 민감 키인지 대소문자 무시로 판별한다.
 */
export function isSensitiveConfigKey(key: string): boolean {
  return SENSITIVE_CONFIG_KEYS.has(key.toLowerCase());
}

/**
 * 시크릿 필드 값이 "비어 있음(=사용자가 새 값을 입력하지 않음)" 인지 판별한다.
 *
 * 빈 값으로 간주하는 경우: undefined, null, 빈 문자열, 공백만 있는 문자열.
 * 그 외(0, false, 객체, 비어 있지 않은 문자열)는 사용자가 의도한 값으로 본다.
 */
function isBlankSecretValue(value: unknown): boolean {
  if (value === undefined || value === null) return true;
  if (typeof value === 'string') return value.trim() === '';
  return false;
}

/**
 * 갱신 페이로드에서 마스킹/미변경 시크릿 필드를 재귀적으로 생략한다(REQ-I07).
 *
 * 동작:
 *   - 시크릿 키이고 값이 비어 있으면(사용자 미입력) → 키를 완전히 제거(필드 부재).
 *   - 시크릿 키이고 값이 비어 있지 않으면(사용자 신규 입력) → 그대로 유지.
 *   - 비시크릿 키 → 중첩 맵/배열을 재귀 처리한 사본으로 유지.
 *
 * 입력을 변경하지 않고 깊은 사본을 반환한다(라이브 편집 상태 보호).
 * 배열 내부의 객체 요소도 재귀 처리한다.
 *
 * @param value - 임의 JSON 값(정의/설정 객체 또는 그 일부).
 * @returns 시크릿이 생략된 깊은 사본.
 */
export function omitMaskedSecrets<T>(value: T): T {
  return omit(value) as T;
}

/** 내부 재귀 헬퍼 — unknown 값을 처리한다. */
function omit(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map((item) => omit(item));
  }
  if (value !== null && typeof value === 'object') {
    const out: Record<string, unknown> = {};
    for (const [key, raw] of Object.entries(value as Record<string, unknown>)) {
      if (isSensitiveConfigKey(key)) {
        // 시크릿 키: 비어 있으면 생략, 새 값이면 유지(노드가 덮어씀).
        if (isBlankSecretValue(raw)) {
          continue;
        }
        out[key] = raw;
        continue;
      }
      out[key] = omit(raw);
    }
    return out;
  }
  return value;
}
