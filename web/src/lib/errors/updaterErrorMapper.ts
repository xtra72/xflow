// SPEC-WEB-006 v0.1.0 (M12) — Updater Error UX Mapping.
//
// SPEC-UPDATE-001 v0.1.0 백엔드 에러를 한글 사용자 친화 메시지로 매핑한다.
//
// 매핑 우선순위:
//   1. APIError status=401 → unauthorized (본문 무관, 권한 게이트 우선)
//   2. APIError status=409 + body 에 "in progress" 포함 → in_progress
//   3. body 키워드 매칭 (사전 정의 순서):
//        checksum → signature → channel → download → rollback →
//        insufficient_disk → downgrade_refused → apply_failed → invalid_input
//   4. fallback (unknown)
//
// 본 모듈은 외부 의존성 없이 순수 문자열/상태 코드 패턴 매칭만 수행한다.
// UI 측 (UpdateDialog 등) 에서 try/catch + toast 형태로 호출하면 된다.
//
// @spec SPEC-WEB-006 v0.1.0 (M12)

import { APIError } from '@/types/api';
import type { TranslationFn } from '@/lib/i18n';

// ─────────────────────────────────────────────────────────────────────
// Public types
// ─────────────────────────────────────────────────────────────────────

/**
 * 매핑 결과의 분류 enum.
 *
 * UI 측에서 토스트/배지/inline 에러 색상 등을 구분하는 용도. 12 종 분류는
 * SPEC-UPDATE-001 의 9 개 백엔드 에러 + 401/409 상태 + unknown 으로 구성.
 */
export type UpdateErrorKind =
  | 'channel_invalid'
  | 'checksum_mismatch'
  | 'signature_invalid'
  | 'download_failed'
  | 'rollback_failed'
  | 'insufficient_disk'
  | 'downgrade_refused'
  | 'apply_failed'
  | 'invalid_input'
  | 'in_progress'
  | 'unauthorized'
  | 'unknown';

/**
 * 매핑된 에러 정보.
 */
export interface UpdateErrorMapped {
  /** 에러 분류. */
  kind: UpdateErrorKind;
  /** 사용자 노출 한글 메시지. */
  userMessage: string;
  /** 추가 가이드 링크 (필요 시). */
  guideLink?: string;
  /** 원본 에러 객체 (디버깅용). */
  raw?: unknown;
}

// ─────────────────────────────────────────────────────────────────────
// Internal constants
// ─────────────────────────────────────────────────────────────────────

/**
 * 본문 키워드 → 분류 매핑 사전.
 *
 * 모두 소문자로 비교한다. 사전 정의 순서가 우선순위이며, 첫 매칭에서 종료한다.
 * checksum/signature 가 동시에 등장하면 checksum 이 이긴다 (보안 신호 강도).
 */
const BODY_KEYWORD_RULES: ReadonlyArray<{
  kind: Exclude<
    UpdateErrorKind,
    'unknown' | 'unauthorized' | 'in_progress'
  >;
  patterns: readonly string[];
}> = [
  {
    kind: 'checksum_mismatch',
    patterns: ['checksum', 'errupdatechecksummismatch'],
  },
  {
    kind: 'signature_invalid',
    patterns: ['signature', 'errupdatesignatureinvalid'],
  },
  {
    kind: 'rollback_failed',
    // rollback 키워드는 channel/download 보다 먼저 검사한다 (409+rollback 우선순위).
    patterns: ['rollback', 'errupdaterollbackfailed'],
  },
  {
    kind: 'channel_invalid',
    patterns: ['channel', 'errupdatechannelinvalid'],
  },
  {
    kind: 'download_failed',
    patterns: ['download', 'errupdatedownloadfailed'],
  },
  {
    kind: 'insufficient_disk',
    patterns: ['disk', 'errupdateinsufficientdiskspace'],
  },
  {
    kind: 'downgrade_refused',
    patterns: ['downgrade', 'errdowngraderequiresforce'],
  },
  {
    kind: 'apply_failed',
    patterns: ['apply', 'errupdateapplyfailed'],
  },
  {
    kind: 'invalid_input',
    patterns: ['invalid', 'errupdateinvalidinput'],
  },
];

// ─────────────────────────────────────────────────────────────────────
// Public API
// ─────────────────────────────────────────────────────────────────────

/**
 * 백엔드 에러를 사용자 친화 메시지로 매핑한다.
 *
 * @param err - 어떤 형태의 에러든 안전하게 받는다 (APIError, Error, string,
 *              plain object, null, undefined 등).
 * @returns 매핑 결과. 매칭에 실패하면 `kind: 'unknown'` + 원본 메시지를 노출한다.
 */
export function mapUpdateError(err: unknown, t: TranslationFn): UpdateErrorMapped {
  const raw = err;
  const message = getErrorMessage(err);
  const normalized = message.toLowerCase();
  const status = err instanceof APIError ? err.status : null;

  // 1. 401 unauthorized — body 와 무관하게 우선 분기 (권한 게이트).
  if (status === 401) {
    return {
      kind: 'unauthorized',
      userMessage: t('error.update.unauthorized'),
      raw,
    };
  }

  // 2. 409 + body 키워드 우선 처리.
  //    - "rollback" 등 BODY_KEYWORD_RULES 의 다른 분류가 매칭되면 그 쪽이 우선.
  //    - 그 외 "in progress" 는 in_progress 로 분류.
  if (status === 409) {
    for (const entry of BODY_KEYWORD_RULES) {
      if (entry.patterns.some((p) => normalized.includes(p))) {
        return buildKnownError(entry.kind, raw, t);
      }
    }
    if (normalized.includes('in progress')) {
      return {
        kind: 'in_progress',
        userMessage: t('error.update.inProgress'),
        raw,
      };
    }
  }

  // 3. status 없는 일반 에러 + body 키워드 매칭 (사전 정의 순서).
  if (normalized.includes('in progress')) {
    return {
      kind: 'in_progress',
      userMessage: t('error.update.inProgress'),
      raw,
    };
  }

  for (const entry of BODY_KEYWORD_RULES) {
    if (entry.patterns.some((p) => normalized.includes(p))) {
      return buildKnownError(entry.kind, raw, t);
    }
  }

  // 4. fallback.
  return {
    kind: 'unknown',
    userMessage: t('error.update.unknown').replace(
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
 */
export function classifyUpdateError(err: unknown): UpdateErrorKind {
  const message = getErrorMessage(err);
  const normalized = message.toLowerCase();
  const status = err instanceof APIError ? err.status : null;

  if (status === 401) return 'unauthorized';
  if (status === 409) {
    for (const entry of BODY_KEYWORD_RULES) {
      if (entry.patterns.some((p) => normalized.includes(p))) return entry.kind;
    }
    if (normalized.includes('in progress')) return 'in_progress';
  }
  if (normalized.includes('in progress')) return 'in_progress';
  for (const entry of BODY_KEYWORD_RULES) {
    if (entry.patterns.some((p) => normalized.includes(p))) return entry.kind;
  }
  return 'unknown';
}

// ─────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────

/**
 * 알려진 분류에 대한 사용자 친화 메시지를 생성한다.
 *
 * SPEC-WEB-006 M12 의 매핑 표에 따라 각 분류별로 한글 메시지를 반환한다.
 */
function buildKnownError(
  kind: Exclude<UpdateErrorKind, 'unknown' | 'unauthorized' | 'in_progress'>,
  raw: unknown,
  t: TranslationFn,
): UpdateErrorMapped {
  switch (kind) {
    case 'channel_invalid':
      return { kind, userMessage: t('error.update.channelInvalid'), raw };
    case 'checksum_mismatch':
      return { kind, userMessage: t('error.update.checksumMismatch'), raw };
    case 'signature_invalid':
      return { kind, userMessage: t('error.update.signatureInvalid'), raw };
    case 'download_failed':
      return { kind, userMessage: t('error.update.downloadFailed'), raw };
    case 'rollback_failed':
      return { kind, userMessage: t('error.update.rollbackFailed'), raw };
    case 'insufficient_disk':
      return { kind, userMessage: t('error.update.insufficientDisk'), raw };
    case 'downgrade_refused':
      return { kind, userMessage: t('error.update.downgradeRefused'), raw };
    case 'apply_failed':
      return { kind, userMessage: t('error.update.applyFailed'), raw };
    case 'invalid_input':
      return { kind, userMessage: t('error.update.invalidInput'), raw };
  }
}

/**
 * 다양한 형태의 에러 입력에서 메시지를 추출한다.
 *
 * - string: 그대로 사용
 * - APIError / Error: `.message` 사용
 * - plain object with `message` 필드: `.message` 사용 (비-string 은 String() 변환)
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
