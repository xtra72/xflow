// 사용자·역할 관리 화면의 서버 오류 → 안내 문구 매핑 (SPEC-AUTH-006 M2.4).
//
// 매핑 기준은 오직 `APIError.code` 다. 서버 메시지는 한국어 고정 문자열이라
// 로케일을 따르지 않고 문면도 계약이 아니므로, 메시지 문자열로 분기하지 않는다.
//
// 409 잠금 방지 코드(마지막 관리자·자기 삭제·빌트인 역할 등)는 원인별로 서로 다른
// 코드를 갖는다. 원인을 구분해 안내해야 사용자가 다음 행동을 정할 수 있다 (AC-03).
//
// @spec SPEC-AUTH-006 v0.1.0 (M2.4 — U2, AC-03/AC-04)

import { APIError } from '@/types/api';

/**
 * 서버가 원인별로 내려주는 코드 집합.
 *
 * 각 코드는 `admin.errors.<CODE>` i18n 키를 그대로 갖는다. 새 코드가 서버에
 * 추가되면 이 집합과 ko/en 두 로케일에 함께 넣는다.
 */
const MAPPED_CODES: ReadonlySet<string> = new Set([
  // 사용자 (409)
  'LAST_ADMIN_USER',
  'SELF_DELETION',
  'USER_EXISTS',
  // 역할 (409)
  'BUILTIN_ROLE_IMMUTABLE',
  'ADMIN_ROLE_IMMUTABLE',
  'ROLE_IN_USE',
  'ROLE_EXISTS',
]);

/**
 * 오류를 사용자 안내용 i18n 키로 변환한다.
 *
 * 코드 매핑이 없으면 HTTP 상태로 폴백한다. 상태도 알 수 없으면(네트워크 단절 등)
 * 일반 실패 문구로 수렴시켜, 어떤 실패든 빈 화면이나 무안내로 끝나지 않게 한다.
 */
export function adminErrorMessageKey(error: unknown): string {
  if (!(error instanceof APIError)) return 'admin.errors.unknown';

  if (MAPPED_CODES.has(error.code)) return `admin.errors.${error.code}`;

  switch (error.status) {
    case 400:
    case 422:
      // 400 은 코드가 BAD_REQUEST 하나로 뭉쳐 있어 원인을 코드로 구분할 수 없다.
      // 서버 한국어 메시지를 그대로 노출하는 대신 검증 규칙을 안내한다.
      return 'admin.errors.badRequest';
    case 403:
      return 'admin.errors.forbidden';
    case 404:
      return 'admin.errors.notFound';
    case 409:
      // 매핑되지 않은 신규 409 — 잠금 방지 계열임은 알리되 원인은 특정하지 않는다.
      return 'admin.errors.conflict';
    default:
      return 'admin.errors.unknown';
  }
}
