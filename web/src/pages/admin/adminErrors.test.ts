// 서버 오류 → 안내 키 매핑 단위 테스트 (SPEC-AUTH-006 M2.4 — AC-03/AC-04).
//
// 매핑은 code 기준이어야 한다. 서버 메시지는 한국어 고정 문자열이라 로케일을
// 따르지 않으며 문면도 계약이 아니다.
//
// @spec SPEC-AUTH-006 v0.1.0 (M2.4)

import { describe, expect, it } from 'vitest';

import { APIError } from '@/types/api';
import { adminErrorMessageKey } from './adminErrors';

describe('adminErrorMessageKey', () => {
  it.each([
    'LAST_ADMIN_USER',
    'SELF_DELETION',
    'USER_EXISTS',
    'BUILTIN_ROLE_IMMUTABLE',
    'ADMIN_ROLE_IMMUTABLE',
    'ROLE_IN_USE',
    'ROLE_EXISTS',
  ])('409 잠금 방지 코드 %s 를 원인별 키로 매핑한다', (code) => {
    const error = new APIError(code, '서버 한국어 메시지', 409);
    expect(adminErrorMessageKey(error)).toBe(`admin.errors.${code}`);
  });

  it('매핑되지 않은 409 는 일반 충돌 안내로 수렴한다', () => {
    const error = new APIError('CONFLICT', 'conflict', 409);
    expect(adminErrorMessageKey(error)).toBe('admin.errors.conflict');
  });

  it('400 은 코드가 BAD_REQUEST 하나로 뭉쳐 있어 검증 규칙 안내로 매핑한다', () => {
    const error = new APIError('BAD_REQUEST', 'password 는 최소 8자', 400);
    expect(adminErrorMessageKey(error)).toBe('admin.errors.badRequest');
  });

  it('422 도 검증 실패로 함께 다룬다', () => {
    expect(adminErrorMessageKey(new APIError('VALIDATION_FAILED', '', 422))).toBe(
      'admin.errors.badRequest',
    );
  });

  it('403 은 권한 부족 안내로 매핑한다', () => {
    expect(adminErrorMessageKey(new APIError('FORBIDDEN', '', 403))).toBe(
      'admin.errors.forbidden',
    );
  });

  it('404 는 대상 없음 안내로 매핑한다', () => {
    expect(adminErrorMessageKey(new APIError('NOT_FOUND', '', 404))).toBe(
      'admin.errors.notFound',
    );
  });

  it('알 수 없는 상태는 일반 실패 안내로 수렴한다', () => {
    expect(adminErrorMessageKey(new APIError('INTERNAL', '', 500))).toBe(
      'admin.errors.unknown',
    );
  });

  it('APIError 가 아닌 실패(네트워크 단절 등)도 안내 없이 끝나지 않는다', () => {
    expect(adminErrorMessageKey(new Error('Network Error'))).toBe('admin.errors.unknown');
    expect(adminErrorMessageKey(undefined)).toBe('admin.errors.unknown');
  });

  it('메시지 문자열이 아니라 code 로 분기한다', () => {
    // 같은 메시지라도 code 가 다르면 다른 키가 나온다.
    const a = new APIError('SELF_DELETION', '동일한 메시지', 409);
    const b = new APIError('USER_EXISTS', '동일한 메시지', 409);
    expect(adminErrorMessageKey(a)).not.toBe(adminErrorMessageKey(b));
  });
});
