// 네트워크 누적 카운터 → 시계열 변환 테스트.

import { describe, it, expect } from 'vitest';

import {
  NETWORK_CHANNELS,
  TOTAL_INTERFACE,
  appendPoint,
  channelFormatter,
  formatBytes,
  formatPackets,
  indexInterfaces,
  interfaceColors,
  isByteChannel,
  isNetworkChannel,
  isTotalChannel,
  normalizeUnitTime,
  toChannelValue,
  type NetworkSample,
} from './networkSeries';
import type { NetworkInterfaceStat, NetworkStats } from '@/services/api/monitorService';

function stat(over: Partial<NetworkInterfaceStat> & { name: string }): NetworkInterfaceStat {
  return {
    bytes_sent: 0, bytes_recv: 0, packets_sent: 0, packets_recv: 0,
    err_in: 0, err_out: 0, drop_in: 0, drop_out: 0,
    ...over,
  };
}

function sample(s: NetworkInterfaceStat, at: number): NetworkSample {
  return { stat: s, at };
}

describe('채널 분류', () => {
  it('net 계열 8종을 모두 인식한다', () => {
    expect(NETWORK_CHANNELS).toHaveLength(8);
    for (const c of NETWORK_CHANNELS) expect(isNetworkChannel(c)).toBe(true);
    expect(isNetworkChannel('cpu')).toBe(false);
  });

  it('Total 접미사로 누적 계열을 가른다', () => {
    expect(isTotalChannel('rxBytesTotal')).toBe(true);
    expect(isTotalChannel('rxBytes')).toBe(false);
  });

  it('바이트 계열과 패킷 계열을 가른다', () => {
    expect(isByteChannel('rxBytes')).toBe(true);
    expect(isByteChannel('txBytesTotal')).toBe(true);
    expect(isByteChannel('rxPackets')).toBe(false);
    expect(isByteChannel('txPacketsTotal')).toBe(false);
  });
});

describe('normalizeUnitTime', () => {
  it('알 수 없는 값은 초로 떨어진다', () => {
    expect(normalizeUnitTime('min')).toBe('min');
    expect(normalizeUnitTime('hour')).toBe('hour');
    expect(normalizeUnitTime('sec')).toBe('sec');
    expect(normalizeUnitTime('day')).toBe('sec');
    expect(normalizeUnitTime(undefined)).toBe('sec');
  });
});

describe('indexInterfaces', () => {
  it('합산을 total 이라는 이름으로 함께 담는다', () => {
    const stats: NetworkStats = {
      total: stat({ name: 'total', bytes_recv: 300 }),
      interfaces: [stat({ name: 'en0', bytes_recv: 200 })],
    };
    const map = indexInterfaces(stats);

    expect(map[TOTAL_INTERFACE]?.bytes_recv).toBe(300);
    expect(map.en0?.bytes_recv).toBe(200);
  });
});

describe('toChannelValue — rate 계열', () => {
  it('두 표본의 차이를 초 단위로 환산한다', () => {
    const prev = sample(stat({ name: 'en0', bytes_recv: 1_000 }), 0);
    const next = sample(stat({ name: 'en0', bytes_recv: 3_000 }), 2_000);

    // 2초에 2,000 바이트 → 1,000 B/s
    expect(toChannelValue(prev, next, 'rxBytes', 'sec')).toBe(1_000);
  });

  it('단위시간이 분이면 60배가 된다', () => {
    const prev = sample(stat({ name: 'en0', bytes_recv: 0 }), 0);
    const next = sample(stat({ name: 'en0', bytes_recv: 100 }), 1_000);

    expect(toChannelValue(prev, next, 'rxBytes', 'min')).toBe(6_000);
    expect(toChannelValue(prev, next, 'rxBytes', 'hour')).toBe(360_000);
  });

  it('첫 표본은 값을 만들지 않는다', () => {
    const next = sample(stat({ name: 'en0', bytes_recv: 100 }), 1_000);
    expect(toChannelValue(null, next, 'rxBytes', 'sec')).toBeNull();
  });

  it('경과 시간이 0 이하면 만들지 않는다 (0으로 나누기 방지)', () => {
    const prev = sample(stat({ name: 'en0', bytes_recv: 100 }), 1_000);
    const next = sample(stat({ name: 'en0', bytes_recv: 200 }), 1_000);
    expect(toChannelValue(prev, next, 'rxBytes', 'sec')).toBeNull();
  });

  it('카운터가 뒤로 가면 만들지 않는다 (재부팅·인터페이스 재설정)', () => {
    // 그대로 두면 거대한 음수 스파이크가 차트를 망가뜨린다.
    const prev = sample(stat({ name: 'en0', bytes_recv: 5_000 }), 0);
    const next = sample(stat({ name: 'en0', bytes_recv: 10 }), 1_000);
    expect(toChannelValue(prev, next, 'rxBytes', 'sec')).toBeNull();
  });

  it('채널마다 다른 카운터를 읽는다', () => {
    const prev = sample(stat({ name: 'en0', packets_sent: 10, bytes_sent: 100 }), 0);
    const next = sample(stat({ name: 'en0', packets_sent: 20, bytes_sent: 900 }), 1_000);

    expect(toChannelValue(prev, next, 'txPackets', 'sec')).toBe(10);
    expect(toChannelValue(prev, next, 'txBytes', 'sec')).toBe(800);
  });
});

describe('toChannelValue — 누적 계열', () => {
  it('직전 표본 없이 카운터 원값을 그대로 쓴다', () => {
    const next = sample(stat({ name: 'en0', bytes_recv: 4_096 }), 1_000);
    expect(toChannelValue(null, next, 'rxBytesTotal', 'sec')).toBe(4_096);
  });

  it('단위시간에 영향받지 않는다 (총량이지 비율이 아니다)', () => {
    const next = sample(stat({ name: 'en0', packets_sent: 77 }), 1_000);
    expect(toChannelValue(null, next, 'txPacketsTotal', 'hour')).toBe(77);
  });

  it('카운터가 뒤로 가도 그대로 반영한다 (재부팅 후 총량이 실제로 줄어든다)', () => {
    const prev = sample(stat({ name: 'en0', bytes_recv: 9_000 }), 0);
    const next = sample(stat({ name: 'en0', bytes_recv: 10 }), 1_000);
    expect(toChannelValue(prev, next, 'rxBytesTotal', 'sec')).toBe(10);
  });
});

describe('appendPoint', () => {
  it('상한을 넘으면 오래된 포인트를 버린다', () => {
    const series = [{ ts: 1, value: 1 }, { ts: 2, value: 2 }];
    const next = appendPoint(series, { ts: 3, value: 3 }, 2);
    expect(next.map((p) => p.ts)).toEqual([2, 3]);
  });

  it('원본을 바꾸지 않는다', () => {
    const series = [{ ts: 1, value: 1 }];
    appendPoint(series, { ts: 2, value: 2 }, 10);
    expect(series).toHaveLength(1);
  });
});

describe('표기', () => {
  it('바이트는 단위를 접고 단위시간 접미사를 붙인다', () => {
    expect(formatBytes(512)).toBe('512 B');
    expect(formatBytes(2_048, 'sec')).toBe('2.0 KB/s');
    expect(formatBytes(5 * 1024 * 1024, 'min')).toBe('5.0 MB/min');
    expect(formatBytes(3 * 1024 ** 4, 'hour')).toBe('3.0 TB/h');
  });

  it('패킷은 천·백만 단위를 접는다', () => {
    expect(formatPackets(120, 'sec')).toBe('120 p/s');
    expect(formatPackets(2_500, 'sec')).toBe('2.5k p/s');
    expect(formatPackets(3_000_000)).toBe('3.0M p');
  });

  it('음수·비정상 값은 대시', () => {
    expect(formatBytes(-1)).toBe('-');
    expect(formatPackets(Number.NaN)).toBe('-');
  });

  it('누적 계열 포맷터는 단위시간 접미사를 붙이지 않는다', () => {
    expect(channelFormatter('rxBytesTotal', 'min')(1_024)).toBe('1.0 KB');
    expect(channelFormatter('rxBytes', 'min')(1_024)).toBe('1.0 KB/min');
  });
});

describe('interfaceColors', () => {
  it('이름마다 색을 배정하고 목록이 길면 순환한다', () => {
    const names = Array.from({ length: 10 }, (_, i) => `if${i}`);
    const colors = interfaceColors(names);

    expect(Object.keys(colors)).toHaveLength(10);
    // 색상표는 8개라 9번째부터 처음 색으로 돌아온다.
    expect(colors.if8).toBe(colors.if0);
  });
});
