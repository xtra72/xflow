// SPEC-WEB-005 v0.7.0 (M15) — Backend Error UX Mapping
// SPEC-STORE-003 v0.3.0 백엔드 에러를 사용자 친화 한글 메시지로 매핑한다.
//
// 매핑 우선순위:
//   1. 마이그레이션 에러 (allow_dynamic_keys removed) — 다른 키워드보다 우선
//   2. 4종 신규 에러 (TypeMismatch, UnsupportedValueType, InvalidDataType,
//      InvalidField)
//   3. fallback (unknown)
//
// 본 모듈은 외부 의존성 없이 순수 문자열 패턴 매칭만 수행한다.
// UI 측 (예: TsdbDataViewerModal) 에서 try/catch + toast 형태로 호출하면 된다.
//
// @spec SPEC-WEB-005 v0.7.0 (M15)

import { APIError } from '@/types/api';
import type { TranslationFn } from '@/lib/i18n';

// ─────────────────────────────────────────────────────────────────────
// Public types
// ─────────────────────────────────────────────────────────────────────

/**
 * 매핑 결과의 분류 enum.
 * UI 측에서 토스트/다이얼로그/배지 색상 등을 구분하는 용도.
 */
export type StoreErrorKind =
  | 'type_mismatch'
  | 'unsupported_value_type'
  | 'invalid_data_type'
  | 'invalid_field'
  | 'migration_required'
  | 'unknown';

/**
 * 매핑된 에러 정보.
 */
export interface StoreErrorMapped {
  /** 에러 분류. */
  kind: StoreErrorKind;
  /** 사용자 노출 한글 메시지. */
  userMessage: string;
  /** 추가 가이드 링크 (마이그레이션 시 등). */
  guideLink?: string;
  /** 원본 에러 객체 (디버깅용). */
  raw?: unknown;
}

// ─────────────────────────────────────────────────────────────────────
// Internal constants
// ─────────────────────────────────────────────────────────────────────

/** 마이그레이션 가이드 문서 경로. */
const MIGRATION_GUIDE_LINK = '/docs/migration/v0.3.0-store-keys.md';

/**
 * 백엔드 에러 키워드 사전.
 *
 * 모두 소문자로 비교하기 위해 lower-case 로 저장한다.
 * 하나의 분류는 여러 키워드 중 하나라도 매칭되면 해당 분류로 판정된다.
 *
 * 매칭 순서가 중요한 분류는 별도 처리한다 (migration_required).
 */
const ERROR_KEYWORDS: ReadonlyArray<{
  kind: Exclude<StoreErrorKind, 'unknown' | 'migration_required'>;
  patterns: readonly string[];
}> = [
  {
    kind: 'type_mismatch',
    patterns: ['value type does not match', 'errtypemismatch'],
  },
  {
    kind: 'unsupported_value_type',
    patterns: ['unsupported value type', 'errunsupportedvaluetype'],
  },
  {
    kind: 'invalid_data_type',
    patterns: ['invalid or missing data_type', 'errinvaliddatatype'],
  },
  {
    kind: 'invalid_field',
    patterns: ['invalid field', 'errinvalidfield'],
  },
];

/**
 * type_mismatch 시 키 이름을 추출하기 위한 정규식.
 * 예: "for key 'sensor1'" → "sensor1"
 */
const KEY_NAME_REGEX = /for key ['"]([^'"]+)['"]/i;

/**
 * type_mismatch 시 등록된 데이터 타입을 추출하기 위한 정규식.
 * 예: "registered=int" → "int"
 */
const REGISTERED_TYPE_REGEX = /registered\s*=\s*([a-z_]+)/i;

// ─────────────────────────────────────────────────────────────────────
// Public API
// ─────────────────────────────────────────────────────────────────────

/**
 * 백엔드 에러를 사용자 친화 메시지로 매핑한다.
 *
 * @param err - 어떤 형태의 에러든 안전하게 받는다 (Error, APIError, string,
 *              plain object, null, undefined 등).
 * @returns 매핑 결과. 매칭에 실패하면 `kind: 'unknown'` + 원본 메시지를 노출한다.
 */
export function mapStoreError(err: unknown, t: TranslationFn): StoreErrorMapped {
  const raw = err;
  const message = getErrorMessage(err);
  const normalized = message.toLowerCase();

  // 1. 마이그레이션 에러 (가장 우선).
  if (
    normalized.includes('allow_dynamic_keys') &&
    normalized.includes('removed')
  ) {
    return {
      kind: 'migration_required',
      userMessage: t('error.store.migrationRequired'),
      guideLink: MIGRATION_GUIDE_LINK,
      raw,
    };
  }

  // 2. 4종 신규 에러.
  for (const entry of ERROR_KEYWORDS) {
    if (entry.patterns.some((p) => normalized.includes(p))) {
      return buildKnownError(entry.kind, message, raw, t);
    }
  }

  // 3. fallback.
  return {
    kind: 'unknown',
    userMessage: t('error.store.unknown').replace(
      '{message}',
      message || 'Unknown error',
    ),
    raw,
  };
}

/**
 * 에러 분류만 필요한 호출자용 (메시지 생성 비용 절약).
 *
 * 분류만 수행하므로 메시지 생성을 위한 t 가 필요 없다.
 *
 * @param err - 어떤 형태의 에러든 안전하게 받는다.
 * @returns 에러 분류 enum.
 */
export function classifyStoreError(err: unknown): StoreErrorKind {
  const message = getErrorMessage(err);
  const normalized = message.toLowerCase();

  if (
    normalized.includes('allow_dynamic_keys') &&
    normalized.includes('removed')
  ) {
    return 'migration_required';
  }
  for (const entry of ERROR_KEYWORDS) {
    if (entry.patterns.some((p) => normalized.includes(p))) {
      return entry.kind;
    }
  }
  return 'unknown';
}

// ─────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────

/**
 * 알려진 분류에 대한 사용자 친화 메시지를 생성한다.
 */
function buildKnownError(
  kind: Exclude<StoreErrorKind, 'unknown' | 'migration_required'>,
  rawMessage: string,
  raw: unknown,
  t: TranslationFn,
): StoreErrorMapped {
  switch (kind) {
    case 'type_mismatch': {
      const keyName = extractKeyFromMessage(rawMessage);
      const registeredType = extractRegisteredTypeFromMessage(rawMessage);
      const baseMessage = t('error.store.typeMismatch');
      // 키 이름/등록 타입이 추출되면 접두 문구를 덧붙인다.
      const prefix = keyName
        ? registeredType
          ? t('error.store.typeMismatchPrefixWithType')
              .replace('{key}', keyName)
              .replace('{type}', registeredType)
          : t('error.store.typeMismatchPrefix').replace('{key}', keyName)
        : '';
      return {
        kind,
        userMessage: prefix + baseMessage,
        raw,
      };
    }
    case 'unsupported_value_type':
      return {
        kind,
        userMessage: t('error.store.unsupportedValueType'),
        raw,
      };
    case 'invalid_data_type':
      return {
        kind,
        userMessage: t('error.store.invalidDataType'),
        raw,
      };
    case 'invalid_field':
      return {
        kind,
        userMessage: t('error.store.invalidField'),
        raw,
      };
  }
}

/**
 * 다양한 형태의 에러 입력에서 메시지를 추출한다.
 *
 * - string: 그대로 사용
 * - Error / APIError: `.message` 사용
 * - plain object with `message` 필드: `.message` 사용
 * - 그 외 (null, undefined, number 등): 빈 문자열
 */
function getErrorMessage(err: unknown): string {
  if (typeof err === 'string') {
    return err;
  }
  if (err instanceof APIError) {
    // APIError 도 Error 의 서브클래스지만 명시적으로 분기해 가독성을 높인다.
    return err.message;
  }
  if (err instanceof Error) {
    return err.message;
  }
  if (err && typeof err === 'object' && 'message' in err) {
    const m = (err as { message: unknown }).message;
    if (typeof m === 'string') {
      return m;
    }
    return String(m);
  }
  return '';
}

/**
 * 에러 메시지에서 키 이름을 추출한다 (가능한 경우).
 * 예: "store: value type does not match registered data_type for key 'sensor1'" → "sensor1"
 */
function extractKeyFromMessage(message: string): string | null {
  const match = message.match(KEY_NAME_REGEX);
  return match && match[1] ? match[1] : null;
}

/**
 * 에러 메시지에서 등록된 데이터 타입을 추출한다 (가능한 경우).
 * 예: "registered=int" → "int"
 */
function extractRegisteredTypeFromMessage(message: string): string | null {
  const match = message.match(REGISTERED_TYPE_REGEX);
  return match && match[1] ? match[1] : null;
}
