// formatPropertyValue 의 전원 OFF 처리 테스트.
// 전원 OFF 시 운전 계열 속성(모드/설정온도/현재온도/풍량/스윙)은 정규화된 기본값이라
// 실제 값이 아니므로 '-' 로 표시하고, 진단 필드(에러코드/필터알람)와 전원 자체는 유지한다.

import { describe, expect, it } from 'vitest';

import { formatPropertyValue } from './deviceLabels';

describe('formatPropertyValue powerOff', () => {
  it('전원 OFF 시 운전 계열 속성은 "-" 로 표시한다', () => {
    const opts = { powerOff: true };
    expect(formatPropertyValue('mode', 0, opts)).toBe('-');
    expect(formatPropertyValue('target_temperature', 0, opts)).toBe('-');
    expect(formatPropertyValue('current_temperature', 0, opts)).toBe('-');
    expect(formatPropertyValue('fan_speed', 0, opts)).toBe('-');
    expect(formatPropertyValue('swing_vertical', false, opts)).toBe('-');
  });

  it('전원 OFF 여도 진단 필드와 전원 자체는 실제 값을 표시한다', () => {
    const opts = { powerOff: true };
    expect(formatPropertyValue('power', false, opts)).toBe('OFF');
    expect(formatPropertyValue('error_code', 0, opts)).toBe('0');
    expect(formatPropertyValue('filter_alarm', false, opts)).toBe('OFF');
  });

  it('전원 ON(또는 옵션 미지정) 이면 운전 계열도 실제 값을 표시한다', () => {
    // 현재 온도 25 → "25°C", mode 1 → 냉방 라벨 등 기존 포맷 유지.
    expect(formatPropertyValue('current_temperature', 25, { powerOff: false })).toBe('25°C');
    expect(formatPropertyValue('current_temperature', 25)).toBe('25°C');
    expect(formatPropertyValue('target_temperature', 24, { powerOff: false })).toBe('24°C');
  });
});
