import { describe, expect, it } from 'vitest';

import {
  deltaRate,
  isCollected,
  isSameSample,
  resolveTargets,
  sumDiskIO,
  sumNetwork,
  sumStorage,
  toSnapshot,
  type SysMetricsSnapshot,
} from './sysMetricsSeries';

/** 에이전트 state 응답 모양의 최소 픽스처 */
function state(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    status: 'running',
    collected_at: 1_700_000_000_000,
    interval_seconds: 5,
    cpu: { usage_percent: 42.5 },
    cpu_cores: 8,
    memory: { total_bytes: 16_000, used_bytes: 8_000, usage_percent: 50 },
    storage: {
      '/': { total_bytes: 100, used_bytes: 30, free_bytes: 70, usage_percent: 30 },
      '/data': { total_bytes: 100, used_bytes: 10, free_bytes: 90, usage_percent: 10 },
    },
    disk_io: {
      disk0: { read_bytes: 100, write_bytes: 200 },
      disk1: { read_bytes: 50, write_bytes: 20 },
    },
    network: {
      en0: { bytes_recv: 1_000, bytes_sent: 500 },
      en1: { bytes_recv: 200, bytes_sent: 100 },
    },
    targets: {
      mountpoints: ['/', '/data'],
      devices: ['disk0', 'disk1'],
      interfaces: ['en0', 'en1'],
    },
    ...overrides,
  };
}

/** 픽스처를 파싱한 스냅샷 (null 이 아님을 단언한다) */
function snap(overrides: Record<string, unknown> = {}): SysMetricsSnapshot {
  const parsed = toSnapshot(state(overrides));
  expect(parsed).not.toBeNull();
  return parsed!;
}

describe('toSnapshot', () => {
  it('state 를 타입 있는 스냅샷으로 정리한다', () => {
    const s = snap();
    expect(s.status).toBe('running');
    expect(s.collectedAt).toBe(1_700_000_000_000);
    expect(s.intervalSeconds).toBe(5);
    expect(s.cpu).toEqual({ usage_percent: 42.5 });
    expect(s.cpuCores).toBe(8);
    expect(s.targets.interfaces).toEqual(['en0', 'en1']);
  });

  it('state 가 없거나 형식이 다르면 null 이다', () => {
    expect(toSnapshot(undefined)).toBeNull();
    expect(toSnapshot(null)).toBeNull();
    expect(toSnapshot('running')).toBeNull();
    expect(toSnapshot([1, 2])).toBeNull();
  });

  it('알 수 없는 status 는 표본 없음으로 본다', () => {
    expect(snap({ status: 'weird' }).status).toBe('no_sample');
  });

  it('수가 아닌 필드는 버린다', () => {
    const s = snap({ cpu: { usage_percent: 'high', cores: 4 } });
    expect(s.cpu).toEqual({ cores: 4 });
  });

  it('표본 이전이면 collectedAt 이 null 이다', () => {
    const s = toSnapshot({ status: 'no_sample', interval_seconds: 5 })!;
    expect(s.collectedAt).toBeNull();
    expect(s.targets.interfaces).toEqual([]);
  });
});

describe('isCollected', () => {
  it('그룹이 부재하면 수집 꺼짐이다', () => {
    const s = snap({ storage: undefined });
    expect(isCollected(s, 'storage')).toBe(false);
    expect(isCollected(s, 'cpu')).toBe(true);
  });

  it('빈 오브젝트는 수집 중(대상 없음)이다', () => {
    // 부재와 빈 오브젝트를 같게 다루면 "관측하지 않는다"와 "대상이 없다"가
    // 화면에서 구별되지 않는다.
    const s = snap({ storage: {} });
    expect(isCollected(s, 'storage')).toBe(true);
  });
});

describe('합산', () => {
  it('디스크 I/O 종합은 전체 장치의 합이다', () => {
    expect(sumDiskIO(snap())).toEqual({ read_bytes: 150, write_bytes: 220 });
  });

  it('네트워크 종합은 전체 인터페이스의 합이다', () => {
    expect(sumNetwork(snap())).toEqual({ bytes_recv: 1_200, bytes_sent: 600 });
  });

  it('일부 인스턴스에 없는 필드도 나머지를 더한다', () => {
    const s = snap({
      network: { en0: { bytes_recv: 10, err_in: 1 }, en1: { bytes_recv: 5 } },
    });
    expect(sumNetwork(s)).toEqual({ bytes_recv: 15, err_in: 1 });
  });

  it('수집이 꺼져 있으면 빈 합이다', () => {
    expect(sumDiskIO(snap({ disk_io: undefined }))).toEqual({});
  });
});

describe('sumStorage', () => {
  it('합계 기준 사용률을 계산한다 (평균이 아니다)', () => {
    const totals = sumStorage(snap());
    expect(totals.usedBytes).toBe(40);
    expect(totals.totalBytes).toBe(200);
    // 30% 와 10% 의 평균 20% 과 우연히 같아지지 않도록 비대칭 픽스처로 다시 확인한다.
    expect(totals.usagePercent).toBe(20);
  });

  it('용량이 큰 쪽이 사용률을 지배한다', () => {
    const totals = sumStorage(
      snap({
        storage: {
          '/': { total_bytes: 1_000, used_bytes: 900, free_bytes: 100, usage_percent: 90 },
          '/ram': { total_bytes: 10, used_bytes: 1, free_bytes: 9, usage_percent: 10 },
        },
      }),
    );
    // 각 사용률의 평균은 50 이지만, 합계 기준은 90 에 가깝다.
    expect(totals.usagePercent).toBeCloseTo((901 / 1_010) * 100, 5);
  });

  it('같은 볼륨의 여러 마운트를 합치지 않는다', () => {
    // macOS 의 / 와 /System/Volumes/Data 처럼 수치가 동일한 두 마운트.
    const totals = sumStorage(
      snap({
        storage: {
          '/': { total_bytes: 100, used_bytes: 30, free_bytes: 70, usage_percent: 30 },
          '/System/Volumes/Data': {
            total_bytes: 100,
            used_bytes: 30,
            free_bytes: 70,
            usage_percent: 30,
          },
        },
      }),
    );
    // 중복 제거 없이 그대로 더한다 — 어떤 마운트가 같은 볼륨인지는 알 수 없다.
    expect(totals.totalBytes).toBe(200);
    expect(totals.usedBytes).toBe(60);
  });

  it('전체 용량이 0 이면 사용률은 0 이다', () => {
    const totals = sumStorage(snap({ storage: { '/dev': { total_bytes: 0, used_bytes: 0 } } }));
    expect(totals.usagePercent).toBe(0);
    expect(Number.isFinite(totals.usagePercent)).toBe(true);
  });
});

describe('deltaRate', () => {
  it('차분을 경과시간으로 나눠 단위시간으로 환산한다', () => {
    const prev = { value: 1_000, at: 0 };
    const next = { value: 3_000, at: 2_000 };
    expect(deltaRate(prev, next, 'sec')).toBe(1_000);
    expect(deltaRate(prev, next, 'min')).toBe(60_000);
    expect(deltaRate(prev, next, 'hour')).toBe(3_600_000);
  });

  it('기준점이 없으면 null 이다', () => {
    expect(deltaRate(null, { value: 100, at: 1_000 }, 'sec')).toBeNull();
  });

  it('카운터가 되감기면 null 이다 (음수 스파이크 방지)', () => {
    const prev = { value: 5_000, at: 0 };
    const next = { value: 100, at: 1_000 };
    expect(deltaRate(prev, next, 'sec')).toBeNull();
  });

  it('경과 시간이 0 이하이면 null 이다', () => {
    expect(deltaRate({ value: 1, at: 1_000 }, { value: 2, at: 1_000 }, 'sec')).toBeNull();
    expect(deltaRate({ value: 1, at: 2_000 }, { value: 2, at: 1_000 }, 'sec')).toBeNull();
  });

  it('변화가 없으면 0 이다 (null 이 아니다)', () => {
    // 트래픽이 실제로 없는 것과 그릴 수 없는 것은 다르다.
    expect(deltaRate({ value: 100, at: 0 }, { value: 100, at: 1_000 }, 'sec')).toBe(0);
  });
});

describe('isSameSample', () => {
  it('collectedAt 이 같으면 같은 표본이다', () => {
    expect(isSameSample(snap(), snap())).toBe(true);
  });

  it('collectedAt 이 다르면 새 표본이다', () => {
    expect(isSameSample(snap(), snap({ collected_at: 1_700_000_005_000 }))).toBe(false);
  });

  it('기준이 없거나 표본 시각이 없으면 새 표본으로 본다', () => {
    expect(isSameSample(null, snap())).toBe(false);
    expect(isSameSample(snap(), toSnapshot({ status: 'no_sample' })!)).toBe(false);
  });
});

describe('resolveTargets', () => {
  it('선택이 비면 빈 배열이다 (종합을 뜻한다)', () => {
    expect(resolveTargets(snap().network, [])).toEqual([]);
  });

  it('선택한 이름만 순서대로 돌려준다', () => {
    expect(resolveTargets(snap().network, ['en1', 'en0'])).toEqual(['en1', 'en0']);
  });

  it('스냅샷에 없는 대상은 건너뛴다', () => {
    // 인터페이스가 사라져도 나머지 시리즈는 계속 그려야 한다.
    expect(resolveTargets(snap().network, ['en0', 'en9'])).toEqual(['en0']);
  });

  it('그룹이 없으면 빈 배열이다', () => {
    expect(resolveTargets(undefined, ['en0'])).toEqual([]);
  });
});
