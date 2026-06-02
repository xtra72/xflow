// SPEC-WEB-005 v0.7.0 (M15) — Backend Error UX Mapping
// SPEC-STORE-003 v0.3.0 백엔드 에러를 사용자 친화 한글 메시지로 매핑하는
// 유틸리티의 사양 테스트.
//
// @spec SPEC-WEB-005 v0.7.0 (M15)

import { describe, it, expect } from 'vitest';

import { APIError } from '@/types/api';

import {
  classifyStoreError,
  mapStoreError,
  type StoreErrorKind,
  type StoreErrorMapped,
} from './storeErrorMapper';

describe('mapStoreError', () => {
  // ── ErrTypeMismatch ────────────────────────────────────────────
  describe('ErrTypeMismatch', () => {
    it('표준 백엔드 메시지를 일반 메시지로 매핑한다', () => {
      const err = new Error(
        'store: value type does not match registered data_type for key',
      );
      const result = mapStoreError(err);
      expect(result.kind).toBe<StoreErrorKind>('type_mismatch');
      expect(result.userMessage).toContain('등록된 타입과 일치하지 않습니다');
      expect(result.userMessage).toContain('삭제 후 재등록');
    });

    it('ErrTypeMismatch 식별자를 포함한 메시지도 매핑한다', () => {
      const err = new Error('lg_hvacr02/store.ErrTypeMismatch occurred');
      const result = mapStoreError(err);
      expect(result.kind).toBe('type_mismatch');
    });

    it('키 이름이 추출 가능하면 메시지 prefix 에 포함한다', () => {
      const err = new Error(
        "store: value type does not match registered data_type for key 'sensor1' (registered=int)",
      );
      const result = mapStoreError(err);
      expect(result.kind).toBe('type_mismatch');
      // 키가 추출되면 "키 'sensor1' 의" 같은 prefix 가 메시지에 포함된다.
      expect(result.userMessage).toMatch(/sensor1/);
    });

    it('키 이름 추출 실패 시에도 일반 메시지로 매핑된다', () => {
      const err = new Error('value type does not match registered data_type');
      const result = mapStoreError(err);
      expect(result.kind).toBe('type_mismatch');
      expect(result.userMessage).toContain('등록된 타입과 일치하지 않습니다');
    });

    it('키 이름은 추출되지만 등록 타입은 추출되지 않을 때 prefix 만 키 이름만 포함', () => {
      // "for key 'X'" 만 있고 "registered=Y" 가 없는 케이스.
      const err = new Error(
        "store: value type does not match registered data_type for key 'sensor2'",
      );
      const result = mapStoreError(err);
      expect(result.kind).toBe('type_mismatch');
      expect(result.userMessage).toMatch(/sensor2/);
      // 등록 타입 정보가 없으면 "(등록 타입: …)" 부분은 빠진다.
      expect(result.userMessage).not.toMatch(/등록 타입:/);
    });
  });

  // ── ErrUnsupportedValueType ────────────────────────────────────
  describe('ErrUnsupportedValueType', () => {
    it('표준 메시지를 매핑한다', () => {
      const err = new Error('store: unsupported value type for key');
      const result = mapStoreError(err);
      expect(result.kind).toBe('unsupported_value_type');
      expect(result.userMessage).toContain('지원하지 않는 값 타입');
      expect(result.userMessage).toContain('nil');
    });

    it('ErrUnsupportedValueType 식별자도 매핑한다', () => {
      const err = new Error('Got ErrUnsupportedValueType from backend');
      const result = mapStoreError(err);
      expect(result.kind).toBe('unsupported_value_type');
    });
  });

  // ── ErrInvalidDataType ─────────────────────────────────────────
  describe('ErrInvalidDataType', () => {
    it('표준 메시지를 매핑한다', () => {
      const err = new Error('store: invalid or missing data_type');
      const result = mapStoreError(err);
      expect(result.kind).toBe('invalid_data_type');
      expect(result.userMessage).toContain('data_type');
      expect(result.userMessage).toContain('int, float, string, boolean, bytes, json');
    });

    it('ErrInvalidDataType 식별자도 매핑한다', () => {
      const err = new Error('lg_hvacr02.ErrInvalidDataType: missing data_type field');
      const result = mapStoreError(err);
      expect(result.kind).toBe('invalid_data_type');
    });
  });

  // ── ErrInvalidMetricType ───────────────────────────────────────
  describe('ErrInvalidMetricType', () => {
    it('표준 메시지를 매핑한다', () => {
      const err = new Error('store: invalid metric_type "sensor!@#"');
      const result = mapStoreError(err);
      expect(result.kind).toBe('invalid_metric_type');
      expect(result.userMessage).toContain('metric_type');
      expect(result.userMessage).toContain('영문');
    });

    it('ErrInvalidMetricType 식별자도 매핑한다', () => {
      const err = new Error('ErrInvalidMetricType: contains invalid characters');
      const result = mapStoreError(err);
      expect(result.kind).toBe('invalid_metric_type');
    });
  });

  // ── 마이그레이션 에러 ──────────────────────────────────────────
  describe('migration_required (allow_dynamic_keys)', () => {
    it('removed 패턴: 명시적 메시지 + 가이드 링크 제공', () => {
      const err = new Error(
        'config: allow_dynamic_keys is removed in v0.3.0, use registration_type instead',
      );
      const result = mapStoreError(err);
      expect(result.kind).toBe('migration_required');
      expect(result.userMessage).toContain('마이그레이션');
      expect(result.userMessage).toContain('registration_type');
      expect(result.guideLink).toBe('/docs/migration/v0.3.0-store-keys.md');
    });

    it('removed 키워드(단순)도 마이그레이션으로 분류한다', () => {
      const err = new Error('legacy field allow_dynamic_keys removed');
      const result = mapStoreError(err);
      expect(result.kind).toBe('migration_required');
      expect(result.guideLink).toBe('/docs/migration/v0.3.0-store-keys.md');
    });

    it('allow_dynamic_keys 만 있고 removed 가 없으면 unknown 으로 분류한다', () => {
      // 마이그레이션 매칭은 두 키워드가 모두 있어야 한다.
      const err = new Error('allow_dynamic_keys is set to true');
      const result = mapStoreError(err);
      expect(result.kind).toBe('unknown');
    });
  });

  // ── 알 수 없는 에러 / fallback ─────────────────────────────────
  describe('unknown (fallback)', () => {
    it('알 수 없는 백엔드 에러에 대해 fallback 메시지를 반환한다', () => {
      const err = new Error('something completely unexpected went wrong');
      const result = mapStoreError(err);
      expect(result.kind).toBe('unknown');
      expect(result.userMessage).toContain('오류가 발생했습니다');
      expect(result.userMessage).toContain('something completely unexpected');
    });

    it('빈 문자열 에러는 fallback + Unknown error 로 처리한다', () => {
      const err = new Error('');
      const result = mapStoreError(err);
      expect(result.kind).toBe('unknown');
      expect(result.userMessage).toContain('Unknown error');
    });

    it('null 입력은 fallback 으로 처리한다', () => {
      const result = mapStoreError(null);
      expect(result.kind).toBe('unknown');
      expect(result.userMessage).toContain('Unknown error');
    });

    it('undefined 입력은 fallback 으로 처리한다', () => {
      const result = mapStoreError(undefined);
      expect(result.kind).toBe('unknown');
      expect(result.userMessage).toContain('Unknown error');
    });

    it('숫자 등 비정형 입력은 fallback 으로 처리한다', () => {
      const result = mapStoreError(42 as unknown);
      expect(result.kind).toBe('unknown');
      expect(result.userMessage).toContain('Unknown error');
    });
  });

  // ── APIError / Error 인스턴스 ──────────────────────────────────
  describe('APIError 처리', () => {
    it('APIError 인스턴스는 message 를 사용해 매핑한다', () => {
      const err = new APIError(
        'TYPE_MISMATCH',
        'value type does not match registered data_type',
        400,
      );
      const result = mapStoreError(err);
      expect(result.kind).toBe('type_mismatch');
    });

    it('APIError + unknown message 는 fallback + 원본 메시지를 노출한다', () => {
      const err = new APIError('UNKNOWN_FAILURE', 'database connection lost', 500);
      const result = mapStoreError(err);
      expect(result.kind).toBe('unknown');
      expect(result.userMessage).toContain('database connection lost');
    });

    it('일반 Error 인스턴스는 message 만 사용해 매핑한다', () => {
      const err = new Error('store: unsupported value type');
      const result = mapStoreError(err);
      expect(result.kind).toBe('unsupported_value_type');
    });

    it('문자열 입력도 message 로 직접 사용한다', () => {
      const result = mapStoreError('invalid metric_type detected');
      expect(result.kind).toBe('invalid_metric_type');
    });

    it('object with message 필드(plain object) 도 메시지를 추출한다', () => {
      const result = mapStoreError({ message: 'invalid or missing data_type' });
      expect(result.kind).toBe('invalid_data_type');
    });

    it('object with non-string message 필드는 String 화 후 매핑한다', () => {
      // message 가 string 이 아닐 때 (예: 숫자) String() 으로 변환하는 분기.
      const result = mapStoreError({ message: 42 });
      expect(result.kind).toBe('unknown');
      expect(result.userMessage).toContain('42');
    });

    it('raw 필드는 원본 에러 객체를 보존한다', () => {
      const err = new Error('store: unsupported value type');
      const result = mapStoreError(err);
      expect(result.raw).toBe(err);
    });
  });

  // ── 입력 형식 다양성 ──────────────────────────────────────────
  describe('입력 형식 다양성', () => {
    it('대소문자 무관 매칭: 대문자 메시지', () => {
      const err = new Error('STORE: VALUE TYPE DOES NOT MATCH REGISTERED DATA_TYPE');
      const result = mapStoreError(err);
      expect(result.kind).toBe('type_mismatch');
    });

    it('부분 매칭: 키워드가 더 긴 메시지에 포함되어도 매핑된다', () => {
      const err = new Error(
        'Backend response: 400 Bad Request — store: invalid metric_type "abc!"; please retry',
      );
      const result = mapStoreError(err);
      expect(result.kind).toBe('invalid_metric_type');
    });
  });

  // ── 매칭 우선순위 ─────────────────────────────────────────────
  describe('매칭 우선순위', () => {
    it('마이그레이션 에러는 다른 4종 키워드보다 우선한다', () => {
      // allow_dynamic_keys removed + invalid data_type 동시 포함 시 마이그레이션이 이긴다.
      const err = new Error(
        'allow_dynamic_keys is removed; also invalid or missing data_type',
      );
      const result = mapStoreError(err);
      expect(result.kind).toBe('migration_required');
    });
  });
});

describe('classifyStoreError', () => {
  it('각 에러를 enum 으로 분류한다 (type_mismatch)', () => {
    expect(
      classifyStoreError(new Error('value type does not match registered data_type')),
    ).toBe<StoreErrorKind>('type_mismatch');
  });

  it('각 에러를 enum 으로 분류한다 (unsupported_value_type)', () => {
    expect(classifyStoreError(new Error('unsupported value type'))).toBe(
      'unsupported_value_type',
    );
  });

  it('각 에러를 enum 으로 분류한다 (invalid_data_type)', () => {
    expect(classifyStoreError(new Error('invalid or missing data_type'))).toBe(
      'invalid_data_type',
    );
  });

  it('각 에러를 enum 으로 분류한다 (invalid_metric_type)', () => {
    expect(classifyStoreError(new Error('invalid metric_type'))).toBe(
      'invalid_metric_type',
    );
  });

  it('각 에러를 enum 으로 분류한다 (migration_required)', () => {
    expect(
      classifyStoreError(new Error('allow_dynamic_keys removed in v0.3.0')),
    ).toBe('migration_required');
  });

  it('알 수 없는 에러는 unknown 으로 분류한다', () => {
    expect(classifyStoreError(new Error('random failure'))).toBe('unknown');
    expect(classifyStoreError(null)).toBe('unknown');
    expect(classifyStoreError(undefined)).toBe('unknown');
  });
});

// 타입 export 검증 (compile-time)
describe('타입 export', () => {
  it('StoreErrorKind / StoreErrorMapped 가 사용 가능하다', () => {
    const sample: StoreErrorMapped = {
      kind: 'unknown',
      userMessage: '테스트',
    };
    expect(sample.kind).toBe('unknown');
  });
});
