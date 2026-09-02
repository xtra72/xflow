// Store keys 행(行) 검증 헬퍼.
//
// `StoreKeysEditor` 와 `PromoteToStaticDialog` 가 공통으로 사용하는 클라이언트
// 측 검증 로직. 외부 의존성 없이 순수 함수만 노출하여 unit test 가 용이하다.
//
// 검증 대상:
//   - data_type: 6종 enum (`int|float|string|boolean|bytes|json`).
//     manual 모드에서는 필수, auto 모드에서는 선택.
//   - field: 정규식 `^[a-zA-Z0-9_-]+$`.
//     빈 문자열은 통과 (백엔드가 default `"unknown"` 적용).
//
// @spec SPEC-WEB-005 v0.7.0 (M12, M13)
// @spec SPEC-STORE-003 v0.3.0

import type { DataType, RegistrationSource } from '@/services/api/store';

// ---- 상수 ----

/**
 * 백엔드가 허용하는 data_type enum 값. SPEC-STORE-003 v0.3.0 와 1:1 동기.
 * 순서는 UI 셀렉트 옵션 노출 순서이기도 하다 (대중적인 타입 → 특수 타입).
 *
 * @spec SPEC-WEB-005 v0.7.0 (M13)
 */
export const DATA_TYPE_OPTIONS = [
  'int',
  'float',
  'string',
  'boolean',
  'bytes',
  'json',
] as const satisfies readonly DataType[];

/**
 * field 허용 문자: 영문/숫자/언더스코어/하이픈.
 * 빈 문자열은 통과 (백엔드 default `"unknown"` 적용).
 *
 * @spec SPEC-WEB-005 v0.7.0 (M13)
 */
const METRIC_TYPE_REGEX = /^[a-zA-Z0-9_-]+$/;

// ---- 검증 결과 타입 ----

/** 단일 필드 검증 결과. valid 가 false 이면 error 한글 메시지를 동반한다. */
export interface FieldValidation {
  valid: boolean;
  /** 사용자 노출 한글 에러 메시지. valid=true 이면 undefined. */
  error?: string;
}

// ---- 검증 함수 ----

/**
 * data_type 값을 검증한다.
 *
 * - manual 모드: 비어있으면 에러 ("data_type 은 manual 모드에서 필수입니다").
 * - auto 모드: 비어있어도 통과 (백엔드가 추론 또는 default 적용).
 * - 둘 다: 알 수 없는 enum 값이면 에러.
 *
 * @param value 사용자 입력 값. unset/공백은 빈 문자열로 정규화된다.
 * @param registrationType 'manual' 또는 'auto'.
 *
 * @spec SPEC-WEB-005 v0.7.0 (M13)
 */
export function validateDataType(
  value: string,
  registrationType: RegistrationSource,
): FieldValidation {
  const v = value.trim();
  if (v === '') {
    if (registrationType === 'manual') {
      return {
        valid: false,
        error: 'data_type 은 manual 모드에서 필수입니다',
      };
    }
    return { valid: true };
  }
  if (!(DATA_TYPE_OPTIONS as readonly string[]).includes(v)) {
    return {
      valid: false,
      error: `허용되지 않는 data_type 값입니다 (허용: ${DATA_TYPE_OPTIONS.join(', ')})`,
    };
  }
  return { valid: true };
}

/**
 * field 값을 검증한다.
 *
 * - 빈 문자열: 통과 (백엔드 default `"unknown"` 적용).
 * - 정규식 `^[a-zA-Z0-9_-]+$` 위반: 에러.
 *
 * @spec SPEC-WEB-005 v0.7.0 (M13)
 */
export function validateMetricType(value: string): FieldValidation {
  const v = value.trim();
  if (v === '') return { valid: true };
  if (!METRIC_TYPE_REGEX.test(v)) {
    return {
      valid: false,
      error: '허용되지 않는 문자가 포함되었습니다 (영숫자, _, - 만 허용)',
    };
  }
  return { valid: true };
}
