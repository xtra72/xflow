// 데이터 량 자동 환산 규칙.
//
// 자동 단위는 다른 단위와 성격이 다르다 — 나머지는 값 뒤에 붙는 접미사인데 이것은
// **값까지 바꾸는 규칙**이다. 저장값이 그대로 화면에 붙거나, 값과 접미사가 따로
// 계산되어 어긋나는 사고를 여기서 잠근다.

import { describe, expect, it } from 'vitest';

import {
  AUTO_BYTES_RATE_UNIT,
  AUTO_BYTES_UNIT,
  formatTickValue,
  formatValueWithUnit,
  isAutoBytesRateUnit,
  isAutoBytesUnit,
  isAutoScaledUnit,
  isPresetUnit,
  scaleValueUnit,
  tickDecimals,
  UNIT_OPTIONS,
  withUnit,
} from './unitOptions';

describe('데이터 량 단위 목록', () => {
  it('목록에 데이터 량 그룹이 있다', () => {
    const group = UNIT_OPTIONS.find((g) => g.labelKey.endsWith('.dataSize'));
    expect(group).toBeDefined();
    expect(group!.units.map((u) => u.value)).toEqual([
      AUTO_BYTES_UNIT,
      'B',
      'KB',
      'MB',
      'GB',
      'TB',
      'PB',
    ]);
  });

  it('자동 단위도 목록 단위다 — 직접 입력으로 새지 않는다', () => {
    expect(isPresetUnit(AUTO_BYTES_UNIT)).toBe(true);
    expect(isAutoBytesUnit(AUTO_BYTES_UNIT)).toBe(true);
    expect(isAutoBytesUnit('KB')).toBe(false);
    expect(isAutoBytesUnit(undefined)).toBe(false);
  });
});

describe('고정 단위는 종전과 같다', () => {
  it('값을 자릿수로 찍고 단위를 그대로 붙인다', () => {
    expect(scaleValueUnit(21.533, 2, '°C')).toEqual({ text: '21.53', suffix: '°C' });
    expect(formatValueWithUnit(21.533, 2, '°C')).toBe('21.53°C');
  });

  it('단위가 없으면 값만 남는다', () => {
    expect(formatValueWithUnit(21.5, 1, undefined)).toBe('21.5');
    expect(formatValueWithUnit(21.5, 1, '')).toBe('21.5');
  });

  it('고정 KB 는 접지 않는다 — 고른 단위가 곧 그 단위다', () => {
    expect(formatValueWithUnit(2048, 2, 'KB')).toBe('2048.00KB');
  });
});

describe('자동 데이터 량 환산', () => {
  it('1024 미만은 B 그대로다', () => {
    expect(scaleValueUnit(512, 2, AUTO_BYTES_UNIT)).toEqual({ text: '512.00', suffix: 'B' });
  });

  it('경계에서 한 칸 올라간다', () => {
    expect(formatValueWithUnit(1023, 0, AUTO_BYTES_UNIT)).toBe('1023B');
    expect(formatValueWithUnit(1024, 0, AUTO_BYTES_UNIT)).toBe('1KB');
  });

  it('크기에 따라 KB · MB · GB · TB · PB 로 접는다', () => {
    expect(formatValueWithUnit(1536, 1, AUTO_BYTES_UNIT)).toBe('1.5KB');
    expect(formatValueWithUnit(1024 ** 2 * 3, 0, AUTO_BYTES_UNIT)).toBe('3MB');
    expect(formatValueWithUnit(1024 ** 3 * 2.5, 1, AUTO_BYTES_UNIT)).toBe('2.5GB');
    expect(formatValueWithUnit(1024 ** 4, 0, AUTO_BYTES_UNIT)).toBe('1TB');
    expect(formatValueWithUnit(1024 ** 5, 0, AUTO_BYTES_UNIT)).toBe('1PB');
  });

  it('PB 를 넘어도 더 접지 않는다 — 목록에 없는 단위를 만들지 않는다', () => {
    expect(scaleValueUnit(1024 ** 6, 0, AUTO_BYTES_UNIT)).toEqual({
      text: '1024',
      suffix: 'PB',
    });
  });

  it('0 은 B 다', () => {
    expect(formatValueWithUnit(0, 0, AUTO_BYTES_UNIT)).toBe('0B');
  });

  it('음수도 크기에 맞춰 접고 부호를 지킨다 — 증감이 음수로 온다', () => {
    expect(formatValueWithUnit(-1536, 1, AUTO_BYTES_UNIT)).toBe('-1.5KB');
    expect(formatValueWithUnit(-512, 0, AUTO_BYTES_UNIT)).toBe('-512B');
  });

  it('자릿수는 모든 자리에서 사용자 설정을 따른다', () => {
    expect(formatValueWithUnit(1536, 3, AUTO_BYTES_UNIT)).toBe('1.500KB');
    expect(formatValueWithUnit(512, 0, AUTO_BYTES_UNIT)).toBe('512B');
  });

  it('수가 아니면 접지 않고 원문을 남긴다', () => {
    expect(scaleValueUnit(Number.NaN, 2, AUTO_BYTES_UNIT)).toEqual({ text: 'NaN', suffix: '' });
    expect(scaleValueUnit(Number.POSITIVE_INFINITY, 2, AUTO_BYTES_UNIT).suffix).toBe('');
  });
});

describe('withUnit 은 자동 단위를 붙이지 않는다', () => {
  it('저장값이 그대로 화면에 새지 않는다', () => {
    // 이미 문자열이 된 값에는 접는 규칙을 걸 수 없다 — 붙이면 `1.5auto:bytes` 가 된다.
    expect(withUnit('1.5', AUTO_BYTES_UNIT)).toBe('1.5');
  });

  it('고정 단위는 종전대로 붙인다', () => {
    expect(withUnit('21.5', '°C')).toBe('21.5°C');
  });
});

describe('데이터 전송률 단위', () => {
  it('목록에 전송률 그룹이 있다', () => {
    const group = UNIT_OPTIONS.find((g) => g.labelKey.endsWith('.dataRate'));
    expect(group).toBeDefined();
    expect(group!.units.map((u) => u.value)).toEqual([
      AUTO_BYTES_RATE_UNIT,
      'B/s',
      'KB/s',
      'MB/s',
      'GB/s',
      'TB/s',
    ]);
  });

  it('데이터 량과 같은 규칙으로 접고 /s 를 붙인다', () => {
    // 누적 카운터를 초당 증가량으로 환산한 값은 개수가 아니라 속도다. `B` 로 적으면
    // 개수로 읽혀 "왜 바이트에 소수점이 붙나" 가 된다.
    expect(formatValueWithUnit(11252.8, 2, AUTO_BYTES_RATE_UNIT)).toBe('10.99KB/s');
    expect(formatValueWithUnit(512, 0, AUTO_BYTES_RATE_UNIT)).toBe('512B/s');
    expect(formatValueWithUnit(1024 ** 2, 1, AUTO_BYTES_RATE_UNIT)).toBe('1.0MB/s');
  });

  it('접는 자리는 데이터 량과 정확히 같다 — 꼬리만 다르다', () => {
    for (const v of [0, 1023, 1024, 1536, 1024 ** 3]) {
      const size = formatValueWithUnit(v, 2, AUTO_BYTES_UNIT);
      const rate = formatValueWithUnit(v, 2, AUTO_BYTES_RATE_UNIT);
      expect(rate).toBe(`${size}/s`);
    }
  });

  it('음수도 부호를 지킨다', () => {
    expect(formatValueWithUnit(-1536, 1, AUTO_BYTES_RATE_UNIT)).toBe('-1.5KB/s');
  });

  it('고정 전송률 단위는 접지 않는다 — 고른 단위가 곧 그 단위다', () => {
    expect(formatValueWithUnit(11252.8, 1, 'KB/s')).toBe('11252.8KB/s');
  });

  it('두 자동 단위를 구분하고, 둘 다 자동으로 판정한다', () => {
    expect(isAutoBytesUnit(AUTO_BYTES_UNIT)).toBe(true);
    expect(isAutoBytesUnit(AUTO_BYTES_RATE_UNIT)).toBe(false);
    expect(isAutoBytesRateUnit(AUTO_BYTES_RATE_UNIT)).toBe(true);
    expect(isAutoBytesRateUnit(AUTO_BYTES_UNIT)).toBe(false);
    for (const u of [AUTO_BYTES_UNIT, AUTO_BYTES_RATE_UNIT]) {
      expect(isAutoScaledUnit(u)).toBe(true);
    }
    for (const u of ['KB', 'KB/s', '', undefined]) {
      expect(isAutoScaledUnit(u)).toBe(false);
    }
  });

  it('목록 단위이므로 직접 입력으로 새지 않는다', () => {
    expect(isPresetUnit(AUTO_BYTES_RATE_UNIT)).toBe(true);
    expect(isPresetUnit('KB/s')).toBe(true);
  });

  it('withUnit 은 전송률 저장값도 붙이지 않는다', () => {
    expect(withUnit('10.99', AUTO_BYTES_RATE_UNIT)).toBe('10.99');
  });
});

describe('눈금 값 자릿수 — 크기가 정한다', () => {
  it('|v| ≤ 1 은 두 자리', () => {
    expect(tickDecimals(0)).toBe(2);
    expect(tickDecimals(0.5)).toBe(2);
    expect(tickDecimals(1)).toBe(2);
    expect(tickDecimals(-1)).toBe(2);
  });

  it('|v| ≤ 10 은 한 자리', () => {
    expect(tickDecimals(1.5)).toBe(1);
    expect(tickDecimals(10)).toBe(1);
    expect(tickDecimals(-10)).toBe(1);
  });

  it('10 초과는 정수', () => {
    expect(tickDecimals(10.5)).toBe(0);
    expect(tickDecimals(100)).toBe(0);
    expect(tickDecimals(-11252.8)).toBe(0);
  });

  it('경계값이 아래 구간에 든다 — 1 과 10 은 각각 두 자리·한 자리', () => {
    expect(formatTickValue(1, undefined)).toBe('1.00');
    expect(formatTickValue(10, undefined)).toBe('10.0');
    expect(formatTickValue(10.5, undefined)).toBe('11');
  });

  it('눈금에는 단위를 붙이지 않는다 — 축을 따라 반복되므로 글자가 겹친다', () => {
    expect(formatTickValue(50, '°C')).toBe('50');
    expect(formatTickValue(0.5, '%')).toBe('0.50');
  });

  it('자동 환산은 **접은 뒤의** 크기로 자릿수를 정한다', () => {
    // 1536 로 자릿수를 정하면 정수(1536 > 10)가 되어 `2KB` 로 뭉개진다.
    // 접은 뒤 1.5 로 정해야 `1.5KB` 가 된다.
    // 접미사는 붙지 않지만 **값은 접는다** — 접지 않으면 눈금이 원시 바이트로 남아
    // 값 표기와 자릿수가 어긋난다.
    expect(formatTickValue(1536, AUTO_BYTES_UNIT)).toBe('1.5');
    expect(formatTickValue(11252.8, AUTO_BYTES_RATE_UNIT)).toBe('11');
    expect(formatTickValue(512, AUTO_BYTES_UNIT)).toBe('512');
  });

  it('자릿수를 명시하면 그 값이 이긴다 — 축과 값이 같은 자릿수여야 한다는 규약', () => {
    expect(formatTickValue(11252.8, AUTO_BYTES_RATE_UNIT, 2)).toBe('10.99');
    expect(formatTickValue(50, '°C', 3)).toBe('50.000');
  });

  it('수가 아니면 원문을 남긴다', () => {
    expect(formatTickValue(Number.NaN, '°C')).toBe('NaN');
  });
});
