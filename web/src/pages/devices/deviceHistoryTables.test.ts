// 디바이스 이력 전용 표(게이트웨이 / 측정치) 변환 + CSV 직렬화 테스트.
//
// 핵심 검증 축:
//   - 게이트웨이 평탄화: (엔트리 × 게이트웨이) 곱집합, 순서 보존
//   - 측정치 컬럼 합집합: 병합 캐시라 옛 엔트리가 키를 적게 갖는 것이 정상
//   - 이월(carried-over) 판정: 셀 time_ms < 엔트리 timestamp
//   - CSV: RFC 4180 이스케이프 + 원본 정밀도 보존

import { describe, expect, it } from 'vitest';

import type { DeviceHistoryEntry } from '@/types/device';

import {
  buildMeasurementHistory,
  escapeCsvCell,
  flattenGatewayHistory,
  gatewayHistoryToCsv,
  isCarriedOver,
  measurementHistoryToCsv,
  toCsvValue,
  type GatewayCsvHeaders,
} from './deviceHistoryTables';

function entry(overrides: Partial<DeviceHistoryEntry> = {}): DeviceHistoryEntry {
  return {
    timestamp: 1_700_000_000_000,
    online: true,
    last_seen: 1_700_000_000_000,
    properties: {},
    ...overrides,
  };
}

function gw(id: string, over: Record<string, unknown> = {}) {
  return {
    gateway_id: id,
    rssi: -95,
    snr: 9.25,
    channel: 3,
    frequency_hz: 922_100_000,
    spreading_factor: 7,
    bandwidth: 125_000,
    last_seen_ms: 1_700_000_000_000,
    stale: false,
    ...over,
  };
}

const GW_HEADERS: GatewayCsvHeaders = {
  time: '시각',
  gatewayId: '게이트웨이 ID',
  rssi: 'RSSI (dBm)',
  snr: 'SNR (dB)',
  channel: '게이트웨이 IF 채널',
  frequencyHz: '주파수 (Hz)',
  spreadingFactor: '확산 계수 (SF)',
  bandwidthHz: '대역폭 (Hz)',
  linkLastSeen: '게이트웨이 수신 시각',
};

describe('flattenGatewayHistory', () => {
  it('엔트리 하나의 게이트웨이 배열을 게이트웨이마다 한 행으로 펼친다', () => {
    const rows = flattenGatewayHistory([
      entry({ properties: { gateways: [gw('aa'), gw('bb'), gw('cc')] } }),
    ]);

    expect(rows).toHaveLength(3);
    expect(rows.map((r) => r.link.gateway_id)).toEqual(['aa', 'bb', 'cc']);
    // 같은 엔트리에서 나온 행은 같은 시각을 갖는다.
    expect(new Set(rows.map((r) => r.timestamp)).size).toBe(1);
  });

  it('여러 엔트리를 (엔트리 × 게이트웨이) 로 평탄화하고 엔트리 순서를 유지한다', () => {
    const rows = flattenGatewayHistory([
      entry({ timestamp: 300, properties: { gateways: [gw('aa'), gw('bb')] } }),
      entry({ timestamp: 200, properties: { gateways: [gw('aa')] } }),
    ]);

    expect(rows).toHaveLength(3);
    expect(rows.map((r) => [r.timestamp, r.link.gateway_id])).toEqual([
      [300, 'aa'],
      [300, 'bb'],
      [200, 'aa'],
    ]);
  });

  it('gateways 속성이 없는 엔트리는 건너뛴다', () => {
    const rows = flattenGatewayHistory([
      entry({ timestamp: 300, properties: { temperature: 24 } }),
      entry({ timestamp: 200, properties: { gateways: [gw('aa')] } }),
    ]);

    expect(rows).toHaveLength(1);
    expect(rows[0]!.timestamp).toBe(200);
  });

  it('게이트웨이 정보가 전혀 없으면 빈 배열을 반환한다 (빈 표를 만들지 않는다)', () => {
    expect(flattenGatewayHistory([entry({ properties: { temperature: 24 } })])).toEqual([]);
    expect(flattenGatewayHistory([])).toEqual([]);
  });

  it('같은 시각에 여러 게이트웨이가 있어도 행마다 고유 키를 만들 수 있다', () => {
    const rows = flattenGatewayHistory([
      entry({ timestamp: 300, properties: { gateways: [gw('aa'), gw('bb')] } }),
      entry({ timestamp: 300, properties: { gateways: [gw('aa')] } }),
    ]);

    const keys = rows.map((r) => `${r.timestamp}-${r.entryIndex}-${r.link.gateway_id}`);
    expect(new Set(keys).size).toBe(keys.length);
  });
});

describe('isCarriedOver', () => {
  it('셀 시각이 엔트리 시각보다 이전이면 이월 값이다', () => {
    expect(isCarriedOver(900, 1000)).toBe(true);
  });

  it('셀 시각이 엔트리 시각과 같으면 이번 업링크에서 갱신된 값이다', () => {
    expect(isCarriedOver(1000, 1000)).toBe(false);
  });

  it('시각을 모르면 이월로 단정하지 않는다', () => {
    expect(isCarriedOver(undefined, 1000)).toBe(false);
    expect(isCarriedOver(0, 1000)).toBe(false);
  });

  it('엔트리 시각이 유효하지 않으면 판정하지 않는다', () => {
    expect(isCarriedOver(900, 0)).toBe(false);
  });
});

describe('buildMeasurementHistory', () => {
  it('측정치를 컬럼으로 펼치고 값/시각을 담는다', () => {
    const table = buildMeasurementHistory([
      entry({
        timestamp: 1000,
        properties: {
          measurements: {
            temperature: { value: 29.8, time_ms: 1000 },
            humidity: { value: 55, time_ms: 1000 },
          },
        },
      }),
    ]);

    expect(table.columns).toEqual(['temperature', 'humidity']);
    expect(table.rows).toHaveLength(1);
    expect(table.rows[0]!.cells['temperature']).toEqual({
      value: 29.8,
      timeMs: 1000,
      carriedOver: false,
    });
  });

  it('컬럼 집합은 전체 엔트리의 합집합이다 (옛 엔트리가 키를 적게 가져도 누락되지 않는다)', () => {
    // measurements 는 병합 캐시라 나중 엔트리일수록 키가 많아진다.
    // 엔트리는 최신순으로 내려오므로 첫 엔트리가 키가 가장 많다.
    const table = buildMeasurementHistory([
      entry({
        timestamp: 2000,
        properties: {
          measurements: {
            temperature: { value: 30, time_ms: 2000 },
            battery: { value: 91, time_ms: 1000 },
          },
        },
      }),
      entry({
        timestamp: 1000,
        properties: {
          measurements: { temperature: { value: 29, time_ms: 1000 } },
        },
      }),
    ]);

    expect(table.columns).toEqual(['temperature', 'battery']);
    // 옛 엔트리에 없는 키는 셀 자체가 없다(호출부가 '-' 로 렌더).
    expect(table.rows[1]!.cells['battery']).toBeUndefined();
  });

  it('나중 엔트리에서만 등장한 키도 컬럼에 추가한다', () => {
    const table = buildMeasurementHistory([
      entry({ timestamp: 2000, properties: { measurements: { a: { value: 1, time_ms: 2000 } } } }),
      entry({ timestamp: 1000, properties: { measurements: { z: { value: 9, time_ms: 1000 } } } }),
    ]);

    expect(table.columns).toEqual(['a', 'z']);
  });

  it('갱신되지 않은 측정치를 이월 값으로 표시한다 (같은 값 반복의 정체를 드러낸다)', () => {
    // battery 는 1000 에 한 번 수신된 뒤 2000 업링크에서는 갱신되지 않았다.
    // 병합 캐시라 값과 time_ms 를 그대로 갖고 다시 나타난다.
    const table = buildMeasurementHistory([
      entry({
        timestamp: 2000,
        properties: {
          measurements: {
            temperature: { value: 30, time_ms: 2000 },
            battery: { value: 91, time_ms: 1000 },
          },
        },
      }),
    ]);

    expect(table.rows[0]!.cells['temperature']!.carriedOver).toBe(false);
    expect(table.rows[0]!.cells['battery']!.carriedOver).toBe(true);
    // 이월 값도 실제 수신 시각은 그대로 보존한다(툴팁에 쓰인다).
    expect(table.rows[0]!.cells['battery']!.timeMs).toBe(1000);
  });

  it('디코딩된 측정치가 없는 업링크에서는 모든 측정치가 이월 값이다', () => {
    // 링크만 갱신된 업링크: 엔트리 시각은 게이트웨이 last_seen 에서 파생되어
    // 모든 측정치 time_ms 보다 나중이 된다.
    const table = buildMeasurementHistory([
      entry({
        timestamp: 5000,
        properties: { measurements: { temperature: { value: 30, time_ms: 2000 } } },
      }),
    ]);

    expect(table.rows[0]!.cells['temperature']!.carriedOver).toBe(true);
  });

  it('measurements 가 없으면 빈 표를 반환한다 (빈 표를 렌더하지 않도록)', () => {
    const table = buildMeasurementHistory([entry({ properties: { temperature: 24 } })]);
    expect(table.columns).toEqual([]);
    expect(table.rows).toEqual([]);
  });

  it('measurements 가 객체가 아니면 무시한다', () => {
    expect(buildMeasurementHistory([entry({ properties: { measurements: 'nope' } })]).columns)
      .toEqual([]);
    expect(buildMeasurementHistory([entry({ properties: { measurements: [1, 2] } })]).columns)
      .toEqual([]);
  });

  it('시각 없는 스칼라 측정치(구형 데이터)는 이월로 단정하지 않는다', () => {
    const table = buildMeasurementHistory([
      entry({ timestamp: 2000, properties: { measurements: { temperature: 29.8 } } }),
    ]);

    expect(table.rows[0]!.cells['temperature']).toEqual({
      value: 29.8,
      timeMs: undefined,
      carriedOver: false,
    });
  });
});

describe('escapeCsvCell', () => {
  it('콤마가 있으면 따옴표로 감싼다', () => {
    expect(escapeCsvCell('a,b')).toBe('"a,b"');
  });

  it('따옴표는 이중화하고 전체를 감싼다', () => {
    expect(escapeCsvCell('say "hi"')).toBe('"say ""hi"""');
  });

  it('개행(LF/CR)이 있으면 따옴표로 감싼다', () => {
    expect(escapeCsvCell('a\nb')).toBe('"a\nb"');
    expect(escapeCsvCell('a\r\nb')).toBe('"a\r\nb"');
  });

  it('콤마/따옴표/개행이 셋 다 있는 값도 안전하게 감싼다', () => {
    expect(escapeCsvCell('a,"b"\nc')).toBe('"a,""b""\nc"');
  });

  it('특수 문자가 없으면 그대로 둔다', () => {
    expect(escapeCsvCell('plain')).toBe('plain');
  });
});

describe('toCsvValue', () => {
  it('원본 정밀도를 유지한다 (표시용 반올림을 하지 않는다)', () => {
    expect(toCsvValue(9.257812)).toBe('9.257812');
    expect(toCsvValue(-95)).toBe('-95');
  });

  it('null/undefined/비유한 숫자는 빈 문자열이다', () => {
    expect(toCsvValue(null)).toBe('');
    expect(toCsvValue(undefined)).toBe('');
    expect(toCsvValue(NaN)).toBe('');
    expect(toCsvValue(Infinity)).toBe('');
  });

  it('boolean 은 true/false 로 쓴다', () => {
    expect(toCsvValue(true)).toBe('true');
    expect(toCsvValue(false)).toBe('false');
  });

  it('객체는 말줄임 없는 compact JSON 이다', () => {
    expect(toCsvValue({ a: 1 })).toBe('{"a":1}');
  });
});

describe('gatewayHistoryToCsv', () => {
  const rows = flattenGatewayHistory([
    entry({ timestamp: 1000, properties: { gateways: [gw('aa'), gw('bb', { rssi: -80 })] } }),
  ]);
  const csv = gatewayHistoryToCsv(rows, GW_HEADERS);
  const lines = csv.trimEnd().split('\n');

  it('헤더 + 게이트웨이마다 한 줄을 쓴다', () => {
    expect(lines).toHaveLength(3);
    expect(lines[0]).toBe(
      '시각,게이트웨이 ID,RSSI (dBm),SNR (dB),게이트웨이 IF 채널,주파수 (Hz),확산 계수 (SF),대역폭 (Hz),게이트웨이 수신 시각',
    );
  });

  it('주파수/대역폭은 표시 단위가 아니라 원본 Hz 로 내보낸다', () => {
    expect(lines[1]).toContain('922100000');
    expect(lines[1]).toContain('125000');
    expect(csv).not.toContain('MHz');
    expect(csv).not.toContain('kHz');
  });

  it('SNR 은 표시용 반올림 없이 원본 값으로 내보낸다', () => {
    expect(lines[1]).toContain('9.25');
  });

  it('조회 시각 파생값인 stale 컬럼은 내보내지 않는다', () => {
    expect(csv.toLowerCase()).not.toContain('stale');
  });

  it('행마다 게이트웨이별 값이 반영된다', () => {
    expect(lines[1]).toContain('aa');
    expect(lines[2]).toContain('bb');
    expect(lines[2]).toContain('-80');
  });

  it('마지막에 개행을 붙인다', () => {
    expect(csv.endsWith('\n')).toBe(true);
  });

  it('게이트웨이 ID 에 콤마가 있어도 컬럼이 밀리지 않는다', () => {
    const weird = gatewayHistoryToCsv(
      flattenGatewayHistory([entry({ properties: { gateways: [gw('a,b')] } })]),
      GW_HEADERS,
    );
    expect(weird).toContain('"a,b"');
  });
});

describe('measurementHistoryToCsv', () => {
  const table = buildMeasurementHistory([
    entry({
      timestamp: 2000,
      properties: {
        measurements: {
          temperature: { value: 29.875, time_ms: 2000 },
          battery: { value: 91, time_ms: 1000 },
        },
      },
    }),
    entry({
      timestamp: 1000,
      properties: { measurements: { temperature: { value: 29, time_ms: 1000 } } },
    }),
  ]);

  const csv = measurementHistoryToCsv(
    table,
    { time: '시각', measuredAtSuffix: '수신 시각' },
    (key) => (key === 'temperature' ? '온도' : key),
  );
  const lines = csv.trimEnd().split('\n');

  it('측정치마다 값 컬럼과 수신 시각 컬럼을 쌍으로 내보낸다', () => {
    const header = lines[0]!.split(',');
    expect(header[0]).toBe('시각');
    // 라벨이 키와 다르면 원본 키를 병기해 humanize 충돌을 방지한다.
    expect(header[1]).toBe('온도 (temperature)');
    expect(header[2]).toBe('온도 (temperature) 수신 시각');
    // 라벨이 키와 같으면 키만 쓴다.
    expect(header[3]).toBe('battery');
    expect(header[4]).toBe('battery 수신 시각');
  });

  it('원본 정밀도를 유지한다', () => {
    expect(lines[1]).toContain('29.875');
  });

  it('해당 엔트리에 없는 측정치는 값/시각 모두 빈 칸이다', () => {
    // 두 번째 행에는 battery 가 없다 → 마지막 두 칸이 빈 문자열.
    expect(lines[2]!.endsWith(',,')).toBe(true);
  });

  it('이월 여부를 재현할 수 있도록 셀별 수신 시각을 싣는다', () => {
    // battery 의 수신 시각(1000)은 행 시각(2000)과 다르다 → 스프레드시트에서 이월 판정 가능.
    const cells = lines[1]!.split(',');
    expect(cells[2]).not.toBe(cells[4]);
  });

  it('마지막에 개행을 붙인다', () => {
    expect(csv.endsWith('\n')).toBe(true);
  });

  it('측정치 값에 콤마/따옴표/개행이 있어도 컬럼이 밀리지 않는다', () => {
    const t2 = buildMeasurementHistory([
      entry({
        timestamp: 1000,
        properties: { measurements: { note: { value: 'a,"b"\nc', time_ms: 1000 } } },
      }),
    ]);
    const out = measurementHistoryToCsv(
      t2,
      { time: '시각', measuredAtSuffix: '수신 시각' },
      (k) => k,
    );
    expect(out).toContain('"a,""b""\nc"');
  });
});
