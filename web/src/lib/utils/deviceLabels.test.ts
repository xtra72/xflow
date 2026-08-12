// formatPropertyValue 의 전원 OFF 처리 테스트.
// 전원 OFF 시 운전 계열 속성(모드/설정온도/현재온도/풍량/스윙)은 정규화된 기본값이라
// 실제 값이 아니므로 '-' 로 표시하고, 진단 필드(에러코드/필터알람)와 전원 자체는 유지한다.

import { describe, expect, it } from 'vitest';

import { expandMeasurementEntries, formatPropertyValue } from './deviceLabels';

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

// 중첩 객체 방어적 폴백 — String(value) 가 "[object Object]" 를 만드는 문제.
// chirpstack 로스터의 measurements 처럼 예상치 못한 중첩 값이 어느 위치로 들어와도
// (이력 테이블 셀 포함) 읽을 수 있는 문자열로 저하되어야 한다.
describe('formatPropertyValue 객체 폴백', () => {
  it('중첩 객체를 "[object Object]" 가 아닌 compact JSON 으로 표시한다', () => {
    const measurements = {
      temperature: { value: 29.8, time_ms: 1786491121129 },
    };
    const out = formatPropertyValue('measurements', measurements);
    expect(out).not.toContain('[object Object]');
    expect(out).toContain('temperature');
    expect(out).toContain('29.8');
  });

  it('배열도 "[object Object]" 를 만들지 않는다', () => {
    const out = formatPropertyValue('items', [{ a: 1 }, { a: 2 }]);
    expect(out).not.toContain('[object Object]');
    expect(out).toBe('[{"a":1},{"a":2}]');
  });

  it('긴 객체는 말줄임한다(그리드 셀 레이아웃 보호)', () => {
    const big: Record<string, number> = {};
    for (let i = 0; i < 50; i++) big[`key_${i}`] = i;
    const out = formatPropertyValue('blob', big);
    expect(out.length).toBeLessThanOrEqual(81); // 80자 + '…'
    expect(out.endsWith('…')).toBe(true);
  });

  it('순환 참조여도 예외를 던지지 않는다', () => {
    const circular: Record<string, unknown> = { a: 1 };
    circular.self = circular;
    expect(() => formatPropertyValue('circular', circular)).not.toThrow();
    expect(formatPropertyValue('circular', circular)).toBe('{…}');
  });

  it('null 은 기존대로 "-" 를 유지한다(typeof null === "object" 회귀 방지)', () => {
    expect(formatPropertyValue('anything', null)).toBe('-');
  });
});

describe('expandMeasurementEntries', () => {
  it('measurements 를 측정치별 개별 항목으로 펼친다', () => {
    const entries: [string, unknown][] = [
      ['rssi', -57],
      [
        'measurements',
        {
          temperature: { value: 29.8, time_ms: 1786491121129 },
          humidity: { value: 55.2, time_ms: 1786491121130 },
        },
      ],
    ];
    expect(expandMeasurementEntries(entries)).toEqual([
      { id: 'rssi', key: 'rssi', value: -57 },
      { id: 'measurements.temperature', key: 'temperature', value: 29.8, timeMs: 1786491121129 },
      { id: 'measurements.humidity', key: 'humidity', value: 55.2, timeMs: 1786491121130 },
    ]);
  });

  it('펼쳐진 측정치는 기존 포맷터로 단위/라벨이 유지된다', () => {
    const entries = expandMeasurementEntries([
      ['measurements', { temperature: { value: 29.8, time_ms: 1 } }],
    ]);
    expect(entries).toHaveLength(1);
    expect(formatPropertyValue(entries[0]!.key, entries[0]!.value)).toBe('29.8°C');
  });

  it('measurements 가 없으면 원본을 그대로 반환한다', () => {
    expect(expandMeasurementEntries([['rssi', -57]])).toEqual([
      { id: 'rssi', key: 'rssi', value: -57 },
    ]);
  });

  it('빈 measurements 객체는 항목을 만들지 않는다', () => {
    expect(expandMeasurementEntries([['measurements', {}]])).toEqual([]);
  });

  it('하위 값이 {value,time_ms} 가 아닌 단순 스칼라면 값만 사용한다', () => {
    expect(expandMeasurementEntries([['measurements', { temperature: 29.8 }]])).toEqual([
      { id: 'measurements.temperature', key: 'temperature', value: 29.8 },
    ]);
  });

  it('time_ms 가 숫자가 아니면 시각을 생략한다', () => {
    expect(
      expandMeasurementEntries([['measurements', { temperature: { value: 1, time_ms: 'bad' } }]]),
    ).toEqual([{ id: 'measurements.temperature', key: 'temperature', value: 1, timeMs: undefined }]);
  });

  it('measurements 가 객체가 아니면(문자열/배열/null) 원본 항목을 유지한다', () => {
    expect(expandMeasurementEntries([['measurements', 'n/a']])).toEqual([
      { id: 'measurements', key: 'measurements', value: 'n/a' },
    ]);
    expect(expandMeasurementEntries([['measurements', null]])).toEqual([
      { id: 'measurements', key: 'measurements', value: null },
    ]);
    expect(expandMeasurementEntries([['measurements', [1, 2]]])).toEqual([
      { id: 'measurements', key: 'measurements', value: [1, 2] },
    ]);
  });

  it('최상위 속성과 측정치 이름이 겹쳐도 id 가 충돌하지 않는다', () => {
    const out = expandMeasurementEntries([
      ['temperature', 21],
      ['measurements', { temperature: { value: 29.8, time_ms: 1 } }],
    ]);
    expect(out.map((e) => e.id)).toEqual(['temperature', 'measurements.temperature']);
  });
});
