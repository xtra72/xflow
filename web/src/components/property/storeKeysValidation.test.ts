// storeKeysValidation 순수 함수 테스트.
//
// 검증 대상:
//   - validateDataType: manual 필수 / auto 선택 / 알 수 없는 enum 거절
//   - validateMetricType: 빈 문자열 통과 / 정규식 위반 거절
//
// @spec SPEC-WEB-005 v0.7.0 (M13)

import { describe, expect, it } from 'vitest';

import {
  DATA_TYPE_OPTIONS,
  validateDataType,
  validateMetricType,
} from './storeKeysValidation';

describe('storeKeysValidation', () => {
  describe('DATA_TYPE_OPTIONS', () => {
    it('6종 enum 을 정확히 노출한다 (백엔드 SPEC-STORE-003 v0.3.0 와 1:1 매핑)', () => {
      expect(DATA_TYPE_OPTIONS).toEqual([
        'int',
        'float',
        'string',
        'boolean',
        'bytes',
        'json',
      ]);
    });
  });

  describe('validateDataType', () => {
    describe('manual 모드', () => {
      it('빈 문자열은 에러를 반환한다', () => {
        const result = validateDataType('', 'manual');
        expect(result.valid).toBe(false);
        expect(result.error).toBe('data_type 은 manual 모드에서 필수입니다');
      });

      it('공백만 입력해도 빈 문자열로 정규화되어 에러를 반환한다', () => {
        const result = validateDataType('   ', 'manual');
        expect(result.valid).toBe(false);
      });

      it('유효한 enum 값(float)은 통과한다', () => {
        expect(validateDataType('float', 'manual').valid).toBe(true);
      });

      it('유효한 enum 값(json)은 통과한다', () => {
        expect(validateDataType('json', 'manual').valid).toBe(true);
      });

      it('알 수 없는 값(decimal)은 에러를 반환한다', () => {
        const result = validateDataType('decimal', 'manual');
        expect(result.valid).toBe(false);
        expect(result.error).toContain('허용되지 않는 data_type 값');
      });
    });

    describe('auto 모드', () => {
      it('빈 문자열은 통과한다 (선택 사항)', () => {
        expect(validateDataType('', 'auto').valid).toBe(true);
      });

      it('유효한 enum 값(int)은 통과한다', () => {
        expect(validateDataType('int', 'auto').valid).toBe(true);
      });

      it('알 수 없는 값은 auto 모드에서도 에러를 반환한다', () => {
        const result = validateDataType('unknown_type', 'auto');
        expect(result.valid).toBe(false);
      });
    });
  });

  describe('validateMetricType', () => {
    it('빈 문자열은 통과한다 (백엔드 default unknown 적용)', () => {
      expect(validateMetricType('').valid).toBe(true);
    });

    it('공백만 입력해도 빈 문자열로 정규화되어 통과한다', () => {
      expect(validateMetricType('   ').valid).toBe(true);
    });

    it('영문자만 (temperature) 통과', () => {
      expect(validateMetricType('temperature').valid).toBe(true);
    });

    it('영문자+숫자 (count2) 통과', () => {
      expect(validateMetricType('count2').valid).toBe(true);
    });

    it('언더스코어 (room_temp) 통과', () => {
      expect(validateMetricType('room_temp').valid).toBe(true);
    });

    it('하이픈 (room-temp) 통과', () => {
      expect(validateMetricType('room-temp').valid).toBe(true);
    });

    it('점(.) 포함 시 에러를 반환한다', () => {
      const result = validateMetricType('room.temp');
      expect(result.valid).toBe(false);
      expect(result.error).toContain('허용되지 않는 문자');
    });

    it('공백 포함 시 에러를 반환한다', () => {
      const result = validateMetricType('room temp');
      expect(result.valid).toBe(false);
    });

    it('한글 포함 시 에러를 반환한다', () => {
      const result = validateMetricType('온도');
      expect(result.valid).toBe(false);
    });

    it('슬래시 포함 시 에러를 반환한다', () => {
      const result = validateMetricType('room/temp');
      expect(result.valid).toBe(false);
    });

    it('특수문자(!@#) 포함 시 에러를 반환한다', () => {
      expect(validateMetricType('temp!').valid).toBe(false);
      expect(validateMetricType('temp@').valid).toBe(false);
      expect(validateMetricType('temp#').valid).toBe(false);
    });
  });
});
