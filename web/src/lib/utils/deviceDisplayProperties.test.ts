// 속성 그리드가 그리는 항목 목록 검증.
//
// 설정의 "표시 항목" 과 패널의 카드는 같은 함수(buildDisplayEntries)를 쓴다. 종전에는
// 둘이 각자의 규칙을 갖고 있어 어긋났다:
//   - 프로토콜 라벨표로 목록을 채워, 이 디바이스가 보고하지 않는 항목까지 고를 수 있었다.
//   - measurements 컨테이너와 그 하위(온도·습도)가 함께 떴고, 필터는 컨테이너로만 걸렸다.
//   - 전용 섹션이 그리는 키(gateways)가 목록에 남아, 골라도 카드가 되지 않았다.

import { describe, expect, it } from 'vitest';

import {
  buildDisplayEntries,
  defaultUnitOf,
  formatPropertyValue,
  getPropertyLabel,
  isDerivedPropertyKey,
  listDisplayableProperties,
} from './deviceLabels';

/** 파생 카드를 뺀 속성 키만 — 종전 계약을 그대로 검사하기 위한 헬퍼. */
function propertyKeys(source: Parameters<typeof listDisplayableProperties>[0]): string[] {
  return listDisplayableProperties(source).filter((k) => !isDerivedPropertyKey(k));
}

describe('속성 카드', () => {
  it('measurements 컨테이너 대신 하위 측정치를 낸다', () => {
    const keys = propertyKeys({
      properties: {
        power: true,
        measurements: {
          temperature: { value: 29.8, time_ms: 1 },
          humidity: { value: 40, time_ms: 2 },
        },
      },
    });

    expect(keys).toContain('temperature');
    expect(keys).toContain('humidity');
    expect(keys).not.toContain('measurements');
    expect(keys).toContain('power');
  });

  it('전용 섹션이 그리는 키는 속성 카드가 되지 않는다', () => {
    expect(propertyKeys({ properties: { power: true, gateways: [{ id: 'gw' }] } })).toEqual([
      'power',
    ]);
  });

  it('이 디바이스가 보고하지 않는 항목은 내지 않는다', () => {
    expect(propertyKeys({ properties: { temperature: 21.5 } })).toEqual(['temperature']);
  });

  it('원본이 없으면 빈 목록', () => {
    expect(listDisplayableProperties(undefined)).toEqual([]);
  });

  it('measurements 가 객체가 아니면 그대로 둔다 — 구형/타 프로바이더 데이터', () => {
    expect(propertyKeys({ properties: { measurements: 'n/a' } })).toEqual(['measurements']);
  });

  it('빈 measurements 는 아무 항목도 만들지 않는다', () => {
    expect(propertyKeys({ properties: { measurements: {} } })).toEqual([]);
  });
});

describe('게이트웨이 수신 정보 카드', () => {
  const link = {
    gateway_id: '24e124fffef5dccc',
    rssi: -47,
    snr: 10.2,
    channel: 5,
    frequency_hz: 923_100_000,
    spreading_factor: 7,
    bandwidth: 125,
    last_seen_ms: 1_700_000_000_000,
    stale: false,
  };

  it('수신 정보를 카드로 낸다', () => {
    const entries = buildDisplayEntries({ properties: { gateways: [link] } });
    const byKey = Object.fromEntries(entries.map((e) => [e.key, e.value]));

    expect(byKey['gw.gateway_id']).toBe('24e124fffef5dccc');
    expect(byKey['gw.rssi']).toBe(-47);
    expect(byKey['gw.snr']).toBe(10.2);
    expect(byKey['gw.channel']).toBe(5);
    // Hz 원값은 자릿수가 많아 카드에서 읽히지 않는다.
    expect(byKey['gw.frequency_hz']).toBe('923.1 MHz');
  });

  it('각 카드에 그 링크의 마지막 수신 시각을 싣는다', () => {
    const entries = buildDisplayEntries({ properties: { gateways: [link] } });
    const rssi = entries.find((e) => e.key === 'gw.rssi')!;
    expect(rssi.timeMs).toBe(link.last_seen_ms);
  });

  it('게이트웨이가 여럿이면 첫 링크를 쓰고 개수를 알린다', () => {
    const second = { ...link, gateway_id: 'ffff0000ffff0000', rssi: -20 };
    const entries = buildDisplayEntries({ properties: { gateways: [link, second] } });
    const byKey = Object.fromEntries(entries.map((e) => [e.key, e.value]));

    // 신호가 센 링크를 고르면 업링크마다 값이 다른 게이트웨이로 튄다.
    expect(byKey['gw.gateway_id']).toBe('24e124fffef5dccc');
    expect(byKey['gw.count']).toBe(2);
  });

  // 첫 업링크 전에 수신 정보가 통째로 사라지면 고를 수도, 자리를 잡아 둘 수도 없다.
  it('수신 게이트웨이가 없어도 항목은 낸다 — 값만 비운다', () => {
    const entries = buildDisplayEntries({ properties: { power: true } }).filter((e) =>
      e.key.startsWith('gw.'),
    );
    expect(entries.map((e) => e.key)).toEqual([
      'gw.gateway_id',
      'gw.rssi',
      'gw.snr',
      'gw.channel',
      'gw.frequency_hz',
    ]);
    expect(entries.every((e) => e.value === undefined)).toBe(true);
  });

  it('수신 게이트웨이가 없으면 수신 개수 카드는 내지 않는다 — 셀 것이 없다', () => {
    const keys = buildDisplayEntries({ properties: { power: true } }).map((e) => e.key);
    expect(keys).not.toContain('gw.count');
  });
});

describe('메타데이터 카드', () => {
  const source = {
    properties: {},
    id: '17ed4b08-018c-469d-8c4b-9650eb432ac6',
    metadata: {
      name: 'AM103-081175',
      location: '사무실',
      group: '',
      tags: [],
      labels: { dev_eui: '24e124725d081175', point: '업무 공간 안쪽' },
    },
  };

  it('이름·ID·위치를 카드로 낸다', () => {
    const byKey = Object.fromEntries(
      buildDisplayEntries(source).map((e) => [e.key, e.value]),
    );

    expect(byKey['meta.name']).toBe('AM103-081175');
    expect(byKey['meta.id']).toBe('17ed4b08-018c-469d-8c4b-9650eb432ac6');
    expect(byKey['meta.location']).toBe('사무실');
  });

  it('사용자 라벨도 각각 카드가 된다', () => {
    const byKey = Object.fromEntries(
      buildDisplayEntries(source).map((e) => [e.key, e.value]),
    );

    expect(byKey['meta.label.dev_eui']).toBe('24e124725d081175');
    expect(byKey['meta.label.point']).toBe('업무 공간 안쪽');
  });

  it('빈 값은 - 로 채운다 — 빈 카드는 무엇을 보는 자리인지 알 수 없다', () => {
    const byKey = Object.fromEntries(
      buildDisplayEntries(source).map((e) => [e.key, e.value]),
    );

    expect(byKey['meta.group']).toBe('-');
    // 태그는 카드로 내지 않는다 — 위치·spot 등이 모두 태그의 한 종류다.
    expect(byKey['meta.tags']).toBeUndefined();
  });

  it('고정 데이터라 갱신 시각을 붙이지 않는다', () => {
    for (const entry of buildDisplayEntries(source)) {
      if (entry.key.startsWith('meta.')) {
        expect(entry.timeMs).toBeUndefined();
      }
    }
  });
});

describe('파생 카드는 골랐을 때만 그린다', () => {
  it('게이트웨이·메타데이터 키를 파생으로 판정한다', () => {
    expect(isDerivedPropertyKey('gw.rssi')).toBe(true);
    expect(isDerivedPropertyKey('meta.name')).toBe(true);
    expect(isDerivedPropertyKey('meta.label.dev_eui')).toBe(true);
  });

  it('디바이스가 보고하는 속성은 파생이 아니다', () => {
    expect(isDerivedPropertyKey('temperature')).toBe(false);
    expect(isDerivedPropertyKey('power')).toBe(false);
  });
});

describe('buildDisplayEntries — 설정 목록과 카드의 단일 원천', () => {
  it('목록의 키는 카드의 키와 정확히 같다', () => {
    const source = {
      properties: {
        power: true,
        gateways: [
          {
            gateway_id: 'gw1',
            rssi: -1,
            snr: 1,
            channel: 1,
            frequency_hz: 1,
            spreading_factor: 1,
            bandwidth: 1,
            last_seen_ms: 1,
            stale: false,
          },
        ],
        measurements: { temperature: { value: 1, time_ms: 2 }, humidity: { value: 3 } },
      },
      id: 'dev-1',
      metadata: { name: 'n', location: '', group: '', tags: [], labels: {} },
    };

    const cardKeys = buildDisplayEntries(source).map((e) => e.key);
    expect(listDisplayableProperties(source)).toEqual(cardKeys);
  });

  it('측정치는 갱신 시각을 함께 싣는다 — 측정치마다 시각이 다르다', () => {
    const entries = buildDisplayEntries({
      properties: { measurements: { temperature: { value: 1, time_ms: 111 } } },
    });
    expect(entries[0]).toMatchObject({ key: 'temperature', timeMs: 111 });
  });
});

describe('단위 붙이기', () => {
  it('글자로 시작하는 단위는 한 칸 띄운다', () => {
    expect(formatPropertyValue('co2', 812, { unit: 'ppm' })).toBe('812 ppm');
    expect(formatPropertyValue('voltage', 3.6, { unit: 'V' })).toBe('3.6 V');
  });

  it('기호로 시작하는 단위는 붙여 쓴다', () => {
    expect(formatPropertyValue('battery', 87, { unit: '%' })).toBe('87%');
    expect(formatPropertyValue('x', 21, { unit: '°C' })).toBe('21°C');
  });

  // 그러지 않으면 "26.4°C ppm" 처럼 단위가 둘 붙는다.
  it('단위를 정하면 키 이름으로 짐작한 단위를 건너뛴다', () => {
    expect(formatPropertyValue('temperature', 26.4)).toBe('26.4°C');
    expect(formatPropertyValue('temperature', 26.4, { unit: 'K' })).toBe('26.4 K');
  });

  it('앞뒤 공백은 다듬는다', () => {
    expect(formatPropertyValue('co2', 812, { unit: '  ppm  ' })).toBe('812 ppm');
  });

  it('빈 단위는 정하지 않은 것과 같다', () => {
    expect(formatPropertyValue('temperature', 26.4, { unit: '   ' })).toBe('26.4°C');
  });

  it('숫자가 아닌 값에는 붙이지 않는다 — "ON ppm" 은 뜻이 없다', () => {
    expect(formatPropertyValue('power', true, { unit: 'ppm' })).toBe('ON');
    expect(formatPropertyValue('x', undefined, { unit: 'ppm' })).toBe('-');
  });
});

describe('키가 원래 갖는 단위', () => {
  // 이름과 단위가 한 덩어리면(RSSI (dBm)) 단위만 바꿀 수 없고, 이름을 바꾸면 단위가 함께 사라진다.
  it('RSSI · SNR 은 이름에서 떼고 값에 붙인다', () => {
    expect(getPropertyLabel('gw.rssi', '', '')).toBe('RSSI');
    expect(getPropertyLabel('gw.snr', '', '')).toBe('SNR');
    expect(formatPropertyValue('gw.rssi', -89)).toBe('-89 dBm');
    expect(formatPropertyValue('gw.snr', 7.5)).toBe('7.5 dB');
  });

  it('정한 단위가 원래 단위를 이긴다', () => {
    expect(formatPropertyValue('gw.rssi', -89, { unit: 'dB' })).toBe('-89 dB');
  });

  it('defaultUnitOf 는 없는 키에 undefined', () => {
    expect(defaultUnitOf('gw.rssi')).toBe('dBm');
    expect(defaultUnitOf('temperature')).toBeUndefined();
  });

  it('값이 없으면 단위를 붙이지 않는다 — "- dBm" 은 읽히지 않는다', () => {
    expect(formatPropertyValue('gw.rssi', undefined)).toBe('-');
  });
});
