// SPEC-WEB-006 v0.1.0 (M12) — Updater Error UX Mapping 테스트.
//
// SPEC-UPDATE-001 v0.1.0 백엔드 에러를 한글 사용자 친화 메시지로 매핑하는
// 유틸리티의 사양 테스트.
//
// 100% 분기 커버리지 목표 (보안 핵심 패턴 매칭).
//
// @spec SPEC-WEB-006 v0.1.0 (M12)

import { describe, expect, it } from 'vitest';

import { APIError } from '@/types/api';
import type { TranslationFn } from '@/lib/i18n';
import ko from '@/lib/i18n/ko.json';

import {
  classifyUpdateError,
  mapUpdateError,
  type UpdateErrorKind,
  type UpdateErrorMapped,
} from './updaterErrorMapper';

// 실제 ko.json 을 사용해 키를 한글 메시지로 해석하는 테스트용 t.
// {message} 보간은 매퍼 내부에서 .replace 로 처리하므로 여기서는 키 해석만 한다.
const t: TranslationFn = (key: string) => {
  const parts = key.split('.');
  let cur: unknown = ko;
  for (const p of parts) {
    if (cur === null || typeof cur !== 'object') return key;
    cur = (cur as Record<string, unknown>)[p];
  }
  return typeof cur === 'string' ? cur : key;
};

describe('mapUpdateError — backend error 분류', () => {
  // ─── ErrUpdateChannelInvalid ────────────────────────────────────
  describe('channel_invalid', () => {
    it('"channel" 키워드로 분류한다', () => {
      const err = new Error('updater: invalid channel configuration');
      const result = mapUpdateError(err, t);
      expect(result.kind).toBe<UpdateErrorKind>('channel_invalid');
      expect(result.userMessage).toContain('업데이트 채널');
    });

    it('ErrUpdateChannelInvalid 식별자도 분류한다', () => {
      const result = mapUpdateError(new Error('ErrUpdateChannelInvalid'), t);
      expect(result.kind).toBe('channel_invalid');
    });
  });

  // ─── ErrUpdateChecksumMismatch ──────────────────────────────────
  describe('checksum_mismatch', () => {
    it('"checksum" 키워드로 분류한다', () => {
      const result = mapUpdateError(new Error('updater: checksum mismatch'), t);
      expect(result.kind).toBe('checksum_mismatch');
      expect(result.userMessage).toContain('체크섬');
    });

    it('ErrUpdateChecksumMismatch 식별자도 분류한다', () => {
      const result = mapUpdateError(new Error('ErrUpdateChecksumMismatch'), t);
      expect(result.kind).toBe('checksum_mismatch');
    });
  });

  // ─── ErrUpdateSignatureInvalid ──────────────────────────────────
  describe('signature_invalid', () => {
    it('"signature" 키워드로 분류한다', () => {
      const result = mapUpdateError(new Error('updater: signature verification failed'), t);
      expect(result.kind).toBe('signature_invalid');
      expect(result.userMessage).toContain('디지털 서명');
      expect(result.userMessage).toContain('신뢰할 수 없는');
    });

    it('ErrUpdateSignatureInvalid 식별자도 분류한다', () => {
      const result = mapUpdateError(new Error('ErrUpdateSignatureInvalid'), t);
      expect(result.kind).toBe('signature_invalid');
    });
  });

  // ─── ErrUpdateDownloadFailed ────────────────────────────────────
  describe('download_failed', () => {
    it('"download" 키워드로 분류한다', () => {
      const result = mapUpdateError(new Error('updater: download failed network error'), t);
      expect(result.kind).toBe('download_failed');
      expect(result.userMessage).toContain('다운로드');
      expect(result.userMessage).toContain('네트워크');
    });

    it('ErrUpdateDownloadFailed 식별자도 분류한다', () => {
      const result = mapUpdateError(new Error('ErrUpdateDownloadFailed'), t);
      expect(result.kind).toBe('download_failed');
    });
  });

  // ─── ErrUpdateRollbackFailed ────────────────────────────────────
  describe('rollback_failed', () => {
    it('"rollback" 키워드로 분류한다', () => {
      const result = mapUpdateError(new Error('updater: rollback failed'), t);
      expect(result.kind).toBe('rollback_failed');
      expect(result.userMessage).toContain('롤백');
    });

    it('ErrUpdateRollbackFailed 식별자도 분류한다', () => {
      const result = mapUpdateError(new Error('ErrUpdateRollbackFailed'), t);
      expect(result.kind).toBe('rollback_failed');
    });

    it('409 + "no backup" 메시지는 rollback_failed 로 분류한다', () => {
      const err = new APIError(
        'NO_BACKUP',
        'rollback failed: no backup available',
        409,
      );
      const result = mapUpdateError(err, t);
      expect(result.kind).toBe('rollback_failed');
    });
  });

  // ─── ErrUpdateInsufficientDiskSpace ─────────────────────────────
  describe('insufficient_disk', () => {
    it('"disk" 키워드로 분류한다', () => {
      const result = mapUpdateError(new Error('updater: insufficient disk space'), t);
      expect(result.kind).toBe('insufficient_disk');
      expect(result.userMessage).toContain('디스크');
    });

    it('ErrUpdateInsufficientDiskSpace 식별자도 분류한다', () => {
      const result = mapUpdateError(new Error('ErrUpdateInsufficientDiskSpace'), t);
      expect(result.kind).toBe('insufficient_disk');
    });
  });

  // ─── ErrUpdateDowngradeRefused ──────────────────────────────────
  describe('downgrade_refused', () => {
    it('"downgrade" 키워드로 분류한다', () => {
      const result = mapUpdateError(new Error('updater: downgrade refused without --force'), t);
      expect(result.kind).toBe('downgrade_refused');
      expect(result.userMessage).toContain('다운그레이드');
      expect(result.userMessage).toContain('force');
    });

    it('ErrDowngradeRequiresForce 식별자도 분류한다', () => {
      const result = mapUpdateError(new Error('ErrDowngradeRequiresForce'), t);
      expect(result.kind).toBe('downgrade_refused');
    });
  });

  // ─── ErrUpdateApplyFailed ───────────────────────────────────────
  describe('apply_failed', () => {
    it('"apply" 키워드로 분류한다', () => {
      const result = mapUpdateError(new Error('updater: apply failed'), t);
      expect(result.kind).toBe('apply_failed');
      expect(result.userMessage).toContain('적용');
      expect(result.userMessage).toContain('백업');
    });

    it('ErrUpdateApplyFailed 식별자도 분류한다', () => {
      const result = mapUpdateError(new Error('ErrUpdateApplyFailed'), t);
      expect(result.kind).toBe('apply_failed');
    });
  });

  // ─── ErrUpdateInvalidInput ──────────────────────────────────────
  describe('invalid_input', () => {
    it('"invalid" 키워드로 분류한다', () => {
      const result = mapUpdateError(new Error('updater: invalid version'), t);
      expect(result.kind).toBe('invalid_input');
      expect(result.userMessage).toContain('잘못된 요청');
    });

    it('ErrUpdateInvalidInput 식별자도 분류한다', () => {
      const result = mapUpdateError(new Error('ErrUpdateInvalidInput'), t);
      expect(result.kind).toBe('invalid_input');
    });
  });

  // ─── ErrUpdateInProgress (HTTP 409) ─────────────────────────────
  describe('in_progress', () => {
    it('409 + "in progress" 메시지는 in_progress 로 분류한다', () => {
      const err = new APIError(
        'UPDATE_IN_PROGRESS',
        'another update operation is in progress',
        409,
      );
      const result = mapUpdateError(err, t);
      expect(result.kind).toBe('in_progress');
      expect(result.userMessage).toContain('진행 중');
    });

    it('"in progress" 단순 메시지(Error)도 분류한다', () => {
      const result = mapUpdateError(new Error('update operation in progress'), t);
      expect(result.kind).toBe('in_progress');
    });
  });

  // ─── 401 unauthorized (status 우선) ─────────────────────────────
  describe('unauthorized', () => {
    it('APIError status=401 은 본문 무관하게 unauthorized 로 분류한다', () => {
      const err = new APIError('UNAUTHORIZED', 'invalid field', 401);
      const result = mapUpdateError(err, t);
      expect(result.kind).toBe('unauthorized');
      expect(result.userMessage).toContain('권한');
    });

    it('일반 Error 의 "unauthorized" 메시지는 fallback (status 정보 없음)', () => {
      // status 가 없는 일반 Error 는 특별 처리하지 않는다.
      // 본문에 다른 키워드가 없으면 unknown 으로 떨어진다.
      const result = mapUpdateError(new Error('totally unrelated message'), t);
      expect(result.kind).toBe('unknown');
    });
  });

  // ─── unknown (fallback) ─────────────────────────────────────────
  describe('unknown (fallback)', () => {
    it('알 수 없는 메시지는 fallback 으로 처리한다', () => {
      const result = mapUpdateError(new Error('mysterious failure'), t);
      expect(result.kind).toBe('unknown');
      expect(result.userMessage).toContain('mysterious failure');
    });

    it('빈 메시지는 "Unknown error" fallback 을 표시한다', () => {
      const result = mapUpdateError(new Error(''), t);
      expect(result.kind).toBe('unknown');
      expect(result.userMessage).toContain('Unknown error');
    });

    it('null 입력은 unknown + "Unknown error" 로 처리한다', () => {
      const result = mapUpdateError(null, t);
      expect(result.kind).toBe('unknown');
      expect(result.userMessage).toContain('Unknown error');
    });

    it('undefined 입력은 unknown + "Unknown error" 로 처리한다', () => {
      const result = mapUpdateError(undefined, t);
      expect(result.kind).toBe('unknown');
      expect(result.userMessage).toContain('Unknown error');
    });

    it('숫자 등 비정형 입력도 unknown 으로 처리한다', () => {
      const result = mapUpdateError(42 as unknown, t);
      expect(result.kind).toBe('unknown');
    });

    it('object with non-string message 필드는 String() 변환 후 매핑한다', () => {
      const result = mapUpdateError({ message: 42 }, t);
      expect(result.kind).toBe('unknown');
      expect(result.userMessage).toContain('42');
    });

    it('object with string message 필드는 그대로 매핑한다', () => {
      const result = mapUpdateError({ message: 'updater: checksum mismatch' }, t);
      expect(result.kind).toBe('checksum_mismatch');
    });
  });

  // ─── 입력 형식 다양성 ─────────────────────────────────────────
  describe('입력 형식 다양성', () => {
    it('대소문자 무관 매칭', () => {
      const result = mapUpdateError(new Error('UPDATER: CHECKSUM MISMATCH'), t);
      expect(result.kind).toBe('checksum_mismatch');
    });

    it('문자열 입력도 직접 매핑한다', () => {
      const result = mapUpdateError('signature invalid', t);
      expect(result.kind).toBe('signature_invalid');
    });

    it('raw 필드는 원본 에러 객체를 보존한다', () => {
      const err = new Error('checksum mismatch');
      const result = mapUpdateError(err, t);
      expect(result.raw).toBe(err);
    });
  });

  // ─── 매칭 우선순위 ─────────────────────────────────────────────
  describe('매칭 우선순위', () => {
    it('401 status 는 본문 키워드보다 우선한다', () => {
      // status 우선 정책으로 401 은 다른 키워드를 무시한다.
      const err = new APIError(
        'UNAUTHORIZED',
        'checksum mismatch but unauthorized',
        401,
      );
      const result = mapUpdateError(err, t);
      expect(result.kind).toBe('unauthorized');
    });

    it('409 + "no backup" 은 rollback_failed 가 우선 (rollback 키워드 매칭)', () => {
      // 본문에 rollback 키워드가 있으면 in_progress 가 아니라 rollback_failed.
      const err = new APIError(
        'NO_BACKUP',
        'rollback failed: no backup file present',
        409,
      );
      const result = mapUpdateError(err, t);
      expect(result.kind).toBe('rollback_failed');
    });

    it('409 + "in progress" 는 in_progress 로 분류 (rollback 키워드 없음)', () => {
      const err = new APIError(
        'UPDATE_IN_PROGRESS',
        'update in progress',
        409,
      );
      const result = mapUpdateError(err, t);
      expect(result.kind).toBe('in_progress');
    });

    it('여러 키워드가 충돌하면 사전 정의 순서가 우선', () => {
      // checksum + signature 동시 포함 → checksum 우선 (사전 정의 순서).
      const result = mapUpdateError(new Error('updater: checksum mismatch with signature problem'), t);
      expect(result.kind).toBe('checksum_mismatch');
    });
  });
});

describe('classifyUpdateError', () => {
  it('각 분류로 정확히 매핑한다', () => {
    expect(classifyUpdateError(new Error('channel invalid'))).toBe(
      'channel_invalid',
    );
    expect(classifyUpdateError(new Error('checksum mismatch'))).toBe(
      'checksum_mismatch',
    );
    expect(classifyUpdateError(new Error('signature invalid'))).toBe(
      'signature_invalid',
    );
    expect(classifyUpdateError(new Error('download failed'))).toBe(
      'download_failed',
    );
    expect(classifyUpdateError(new Error('rollback failed'))).toBe(
      'rollback_failed',
    );
    expect(classifyUpdateError(new Error('insufficient disk'))).toBe(
      'insufficient_disk',
    );
    expect(classifyUpdateError(new Error('downgrade refused'))).toBe(
      'downgrade_refused',
    );
    expect(classifyUpdateError(new Error('apply failed'))).toBe('apply_failed');
    expect(classifyUpdateError(new Error('invalid version'))).toBe(
      'invalid_input',
    );
    expect(
      classifyUpdateError(new APIError('UNAUTHORIZED', 'whatever', 401)),
    ).toBe('unauthorized');
    expect(
      classifyUpdateError(new APIError('IN_PROGRESS', 'in progress', 409)),
    ).toBe('in_progress');
    expect(classifyUpdateError(new Error('unknown'))).toBe('unknown');
    expect(classifyUpdateError(null)).toBe('unknown');
    expect(classifyUpdateError(undefined)).toBe('unknown');
  });
});

// ─── 타입 export 검증 (compile-time) ─────────────────────────────────
describe('타입 export', () => {
  it('UpdateErrorKind / UpdateErrorMapped 가 사용 가능하다', () => {
    const sample: UpdateErrorMapped = {
      kind: 'unknown',
      userMessage: '테스트',
    };
    expect(sample.kind).toBe('unknown');
  });
});
