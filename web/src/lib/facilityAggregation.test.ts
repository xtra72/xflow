// facilityAggregation 순수 함수 테스트 (SPEC-FACILITY-DASHBOARD-001 B1, REQ-05-01/02/03).

import { describe, expect, it } from 'vitest';

import type { AirDevice, AirStation } from '@/hooks/useStation';
import {
  aggregateByLine,
  aggregateByStation,
  countStats,
  deviceFanStatus,
  displayDeviceFanStatus,
  displayStationStatus,
  lineDiagramLayout,
  stationDeviceRows,
  stationStatus,
  stationSummary,
} from './facilityAggregation';

// ---- 픽스처 헬퍼 ----

function dev(partial: Partial<AirDevice> & { device_id: string }): AirDevice {
  return {
    name: partial.device_id,
    group_id: '',
    station: '',
    place: '',
    index: 0,
    online: true,
    power: false,
    fan_speed: 0,
    source: 'bridge',
    ...partial,
  };
}

function station(partial: Partial<AirStation> & { station: string }): AirStation {
  return {
    line: '',
    display_name: '',
    order: 0,
    places: [],
    ...partial,
  };
}

// 2호선: ST-A(order 2), ST-B(order 1). 1호선: ST-C(order 1).
const stations: AirStation[] = [
  station({ station: 'ST-A', line: '2호선', display_name: '강남', order: 2 }),
  station({ station: 'ST-B', line: '2호선', display_name: '시청', order: 1 }),
  station({ station: 'ST-C', line: '1호선', display_name: '종로', order: 1 }),
];

describe('countStats', () => {
  it('online/offline, power on/off, 풍량(1/2/3) 원시 분포를 합산한다', () => {
    const devices = [
      dev({ device_id: 'd1', online: true, power: true, fan_speed: 1 }),
      dev({ device_id: 'd2', online: true, power: true, fan_speed: 3 }),
      dev({ device_id: 'd3', online: false, power: false, fan_speed: 0 }),
      dev({ device_id: 'd4', online: true, power: false, fan_speed: 2 }),
    ];
    expect(countStats(devices)).toEqual({
      total: 4,
      online: 3,
      offline: 1,
      powerOn: 2,
      powerOff: 2,
      fan1: 1,
      fan2: 1,
      fan3: 1,
    });
  });

  it('빈 로스터는 모두 0 인 안전한 카운트를 반환한다', () => {
    expect(countStats([])).toEqual({
      total: 0,
      online: 0,
      offline: 0,
      powerOn: 0,
      powerOff: 0,
      fan1: 0,
      fan2: 0,
      fan3: 0,
    });
  });
});

describe('stationSummary', () => {
  it('레지스트리 항목이 있으면 display_name/line/order 를 backfill 한다', () => {
    const s = stationSummary('ST-A', [dev({ device_id: 'd1', station: 'ST-A' })], stations[0]);
    expect(s.displayName).toBe('강남');
    expect(s.line).toBe('2호선');
    expect(s.order).toBe(2);
    expect(s.stats.total).toBe(1);
  });

  it('레지스트리 미등록(entry 없음)이면 line=빈, order=0, displayName=station code', () => {
    const s = stationSummary('ST-X', [dev({ device_id: 'd1', station: 'ST-X' })]);
    expect(s.displayName).toBe('ST-X');
    expect(s.line).toBe('');
    expect(s.order).toBe(0);
  });
});

describe('aggregateByLine', () => {
  it('역사를 호선별로 롤업하고 호선 내 역사는 order→code 로 정렬한다', () => {
    const devices = [
      dev({ device_id: 'a1', station: 'ST-A', power: true, fan_speed: 1 }),
      dev({ device_id: 'a2', station: 'ST-A', online: false }),
      dev({ device_id: 'b1', station: 'ST-B', power: true, fan_speed: 2 }),
      dev({ device_id: 'c1', station: 'ST-C', power: true, fan_speed: 3 }),
    ];
    const { lines } = aggregateByLine(devices, stations);

    // 호선 정렬(line code): '1호선' < '2호선'.
    expect(lines.map((l) => l.line)).toEqual(['1호선', '2호선']);

    const line2 = lines.find((l) => l.line === '2호선');
    expect(line2?.deviceCount).toBe(3);
    // ST-B(order 1) 가 ST-A(order 2) 보다 앞선다.
    expect(line2?.stations.map((s) => s.station)).toEqual(['ST-B', 'ST-A']);
    // 호선 stats 는 소속 전 디바이스 합산.
    expect(line2?.stats.total).toBe(3);
    expect(line2?.stats.powerOn).toBe(2);
    expect(line2?.stats.offline).toBe(1);

    const line1 = lines.find((l) => l.line === '1호선');
    expect(line1?.deviceCount).toBe(1);
    expect(line1?.stats.fan3).toBe(1);
  });

  it('미등록 station 디바이스는 호선 롤업에서 제외하고 unclassified 로 카운트한다(UB-004)', () => {
    const devices = [
      dev({ device_id: 'a1', station: 'ST-A' }),
      dev({ device_id: 'x1', station: 'ST-UNKNOWN' }),
      dev({ device_id: 'x2', station: '' }),
    ];
    const { lines, unclassified } = aggregateByLine(devices, stations);

    // 호선 롤업에는 미등록/빈 station 디바이스가 없다.
    const allRolledUp = lines.flatMap((l) => l.stations.flatMap((s) => s.devices.map((d) => d.device_id)));
    expect(allRolledUp).toEqual(['a1']);

    expect(unclassified.count).toBe(2);
    expect(unclassified.deviceIds).toEqual(['x1', 'x2']);
  });

  it('빈 로스터는 빈 lines + count 0 미분류를 반환한다', () => {
    expect(aggregateByLine([], stations)).toEqual({
      lines: [],
      unclassified: { count: 0, deviceIds: [] },
    });
  });

  it('레지스트리가 비면 모든 디바이스가 미분류가 된다', () => {
    const devices = [dev({ device_id: 'a1', station: 'ST-A' })];
    const { lines, unclassified } = aggregateByLine(devices, []);
    expect(lines).toEqual([]);
    expect(unclassified).toEqual({ count: 1, deviceIds: ['a1'] });
  });
});

describe('aggregateByStation', () => {
  it('로스터를 역사별로 롤업하고 order→code 로 정렬한다', () => {
    const devices = [
      dev({ device_id: 'a1', station: 'ST-A' }),
      dev({ device_id: 'b1', station: 'ST-B' }),
      dev({ device_id: 'b2', station: 'ST-B' }),
    ];
    const summaries = aggregateByStation(devices, stations);
    // ST-B(order 1) → ST-A(order 2).
    expect(summaries.map((s) => s.station)).toEqual(['ST-B', 'ST-A']);
    expect(summaries.find((s) => s.station === 'ST-B')?.stats.total).toBe(2);
  });
});

describe('lineDiagramLayout', () => {
  it('해당 호선의 역사를 order 오름차순(동률 시 code)으로 정렬해 노드로 반환한다', () => {
    const devices = [
      dev({ device_id: 'a1', station: 'ST-A' }),
      dev({ device_id: 'b1', station: 'ST-B' }),
    ];
    const nodes = lineDiagramLayout('2호선', stations, devices);
    // ST-B(order 1) → ST-A(order 2).
    expect(nodes.map((n) => n.station)).toEqual(['ST-B', 'ST-A']);
    expect(nodes[0]?.displayName).toBe('시청');
    expect(nodes[1]?.summary.stats.total).toBe(1);
  });

  it('디바이스가 없는 역사도 레지스트리에 있으면 빈 요약 노드로 포함한다', () => {
    const nodes = lineDiagramLayout('1호선', stations, []);
    expect(nodes.map((n) => n.station)).toEqual(['ST-C']);
    expect(nodes[0]?.summary.stats.total).toBe(0);
  });

  it('레지스트리에 없는 호선은 빈 배열을 반환한다', () => {
    expect(lineDiagramLayout('9호선', stations, [])).toEqual([]);
  });

  it('order 동률이면 station code 오름차순으로 정렬한다', () => {
    const tied: AirStation[] = [
      station({ station: 'ST-Z', line: '3호선', order: 1 }),
      station({ station: 'ST-Y', line: '3호선', order: 1 }),
    ];
    const nodes = lineDiagramLayout('3호선', tied, []);
    expect(nodes.map((n) => n.station)).toEqual(['ST-Y', 'ST-Z']);
  });

  it('노드에 역사 레지스트리의 places 를 실어 상세 모드 해석에 제공한다', () => {
    const withPlaces: AirStation[] = [
      station({
        station: 'ST-P',
        line: '4호선',
        order: 1,
        places: [{ place: 'PL-1', display_name: '승강장', order: 1 }],
      }),
    ];
    const nodes = lineDiagramLayout('4호선', withPlaces, []);
    expect(nodes[0]?.places).toEqual([{ place: 'PL-1', display_name: '승강장', order: 1 }]);
  });
});

describe('stationStatus', () => {
  function stats(partial: Partial<ReturnType<typeof countStats>>) {
    return {
      total: 0,
      online: 0,
      offline: 0,
      powerOn: 0,
      powerOff: 0,
      fan1: 0,
      fan2: 0,
      fan3: 0,
      ...partial,
    };
  }

  it('기기 0대면 empty 를 반환한다', () => {
    expect(stationStatus(stats({}))).toBe('empty');
  });

  it('전부 오프라인이면 offline(우선순위 최상)', () => {
    expect(stationStatus(stats({ total: 3, offline: 3, powerOff: 3 }))).toBe('offline');
  });

  it('일부만 오프라인이면 warning', () => {
    expect(stationStatus(stats({ total: 3, online: 2, offline: 1, powerOn: 2, powerOff: 1 }))).toBe(
      'warning',
    );
  });

  it('전부 온라인·전부 전원 꺼짐이면 off', () => {
    expect(stationStatus(stats({ total: 2, online: 2, offline: 0, powerOff: 2 }))).toBe('off');
  });

  it('온라인 + 최소 1대 가동이면 normal', () => {
    expect(stationStatus(stats({ total: 2, online: 2, offline: 0, powerOn: 1, powerOff: 1 }))).toBe(
      'normal',
    );
  });
});

describe('deviceFanStatus', () => {
  it('오프라인 기기는 offline(전원·풍량 무관)', () => {
    expect(deviceFanStatus(dev({ device_id: 'd', online: false, power: true, fan_speed: 2 }))).toBe(
      'offline',
    );
  });

  it('온라인이나 전원 꺼짐이면 off', () => {
    expect(deviceFanStatus(dev({ device_id: 'd', online: true, power: false, fan_speed: 3 }))).toBe(
      'off',
    );
  });

  it('온라인·전원 켜짐이면 fan_speed 1/2/3 을 fan1/2/3 으로 매핑', () => {
    expect(deviceFanStatus(dev({ device_id: 'd', power: true, fan_speed: 1 }))).toBe('fan1');
    expect(deviceFanStatus(dev({ device_id: 'd', power: true, fan_speed: 2 }))).toBe('fan2');
    expect(deviceFanStatus(dev({ device_id: 'd', power: true, fan_speed: 3 }))).toBe('fan3');
  });

  it('알 수 없는 fan_speed 는 unknown 폴백', () => {
    expect(deviceFanStatus(dev({ device_id: 'd', power: true, fan_speed: 9 }))).toBe('unknown');
  });
});

describe('displayStationStatus (offlineAsOff)', () => {
  it('offlineAsOff 켜짐: offline → off, 그 외 상태는 불변', () => {
    expect(displayStationStatus('offline', true)).toBe('off');
    expect(displayStationStatus('warning', true)).toBe('warning'); // 부분 오프라인은 위장 안 함
    expect(displayStationStatus('normal', true)).toBe('normal');
    expect(displayStationStatus('off', true)).toBe('off');
  });

  it('offlineAsOff 꺼짐: 모든 상태 불변', () => {
    expect(displayStationStatus('offline', false)).toBe('offline');
    expect(displayStationStatus('normal', false)).toBe('normal');
  });
});

describe('displayDeviceFanStatus (offlineAsOff)', () => {
  it('offlineAsOff 켜짐: offline → off, 그 외 상태는 불변', () => {
    expect(displayDeviceFanStatus('offline', true)).toBe('off');
    expect(displayDeviceFanStatus('fan2', true)).toBe('fan2');
    expect(displayDeviceFanStatus('off', true)).toBe('off');
  });

  it('offlineAsOff 꺼짐: 모든 상태 불변', () => {
    expect(displayDeviceFanStatus('offline', false)).toBe('offline');
    expect(displayDeviceFanStatus('fan1', false)).toBe('fan1');
  });
});

describe('stationDeviceRows', () => {
  const places: AirStation['places'] = [
    { place: 'PL-A', display_name: '승강장', order: 2 },
    { place: 'PL-B', display_name: '대합실', order: 1 },
  ];

  it('place order→index 로 정렬하고 place code 를 표시명으로 해석한다', () => {
    const devices = [
      dev({ device_id: 'd1', place: 'PL-A', index: 1, power: true, fan_speed: 1 }),
      dev({ device_id: 'd2', place: 'PL-B', index: 2 }),
      dev({ device_id: 'd3', place: 'PL-B', index: 1 }),
    ];
    const rows = stationDeviceRows(devices, places);
    // PL-B(order 1) 먼저, 내부는 index 오름차순 → d3, d2; 그다음 PL-A(order 2) → d1.
    expect(rows.map((r) => r.device.device_id)).toEqual(['d3', 'd2', 'd1']);
    expect(rows[0]?.placeLabel).toBe('대합실');
    expect(rows[2]?.placeLabel).toBe('승강장');
    expect(rows[2]?.fanStatus).toBe('fan1');
  });

  it('미등록 place 는 목록 끝으로 보내고 place code 로 폴백한다', () => {
    const devices = [
      dev({ device_id: 'd1', place: 'PL-UNKNOWN', index: 1 }),
      dev({ device_id: 'd2', place: 'PL-B', index: 1 }),
    ];
    const rows = stationDeviceRows(devices, places);
    expect(rows.map((r) => r.device.device_id)).toEqual(['d2', 'd1']);
    expect(rows[1]?.placeLabel).toBe('PL-UNKNOWN');
  });

  it('place code 가 비면 device name 으로 폴백한다', () => {
    const rows = stationDeviceRows([dev({ device_id: 'd1', name: '기기1', place: '' })], places);
    expect(rows[0]?.placeLabel).toBe('기기1');
  });
});
