// sysmetrics 소스 순수 변환 테스트.
//
// 잠그는 것:
//   - 시리즈 하나가 줄 하나다(1:1). 값 목록 × 대상 목록의 곱이 아니다.
//   - Store 어휘 매핑 (measurement = 값 카탈로그 키, tags = 대상 축)
//   - 이름 우선순위 (별칭 > 형식 > 내장 표기) 와 로케일 독립성
//   - 값 추출 (상태값 그대로 / 누적 카운터는 증가량 / 종합 사용률의 재계산)
//   - 누적 원값 모드(`mode: 'total'`) — 태그 축·값 추출·행 id 안정성
//   - 시리즈 표의 후보 행 구성

import { describe, expect, it } from 'vitest';

import type { SysMetricsSnapshot, SysMetricsTargets } from '../sysmetrics/sysMetricsSeries';
import type { SysmetricsSourceConfig } from './chartChannelTypes';
import {
  SYSMETRIC_MISSING_BADGE,
  describeSysmetricSeries,
  hasInstanceAxis,
  readRawValue,
  readSeriesValue,
  resolveSysmetricsSeries,
  sysmetricRowId,
  sysmetricsCandidateRows,
} from './sysmetricsSource';
import { findChartField } from '../sysmetrics/sysMetricsFields';

/** 스냅샷 픽스처. 인스턴스 축이 있는 그룹은 인터페이스/장치/마운트를 둘씩 준다. */
function snapshot(at: number, bytesRecv = { en0: 1_000, en1: 2_000 }): SysMetricsSnapshot {
  return {
    status: 'running',
    collectedAt: at,
    intervalSeconds: 5,
    cpu: { usage_percent: 42 },
    memory: { usage_percent: 50, used_bytes: 8_000, total_bytes: 16_000 },
    storage: {
      '/': { usage_percent: 90, used_bytes: 900, free_bytes: 100, total_bytes: 1_000 },
      '/data': { usage_percent: 10, used_bytes: 100, free_bytes: 900, total_bytes: 1_000 },
    },
    diskIo: { disk0: { read_bytes: 500 } },
    network: {
      en0: { bytes_recv: bytesRecv.en0 },
      en1: { bytes_recv: bytesRecv.en1 },
    },
    targets: { mountpoints: ['/', '/data'], devices: ['disk0'], interfaces: ['en0', 'en1'] },
  };
}

/** 조회 창은 Store 와 같은 필드를 쓴다. 이 파일의 관심사가 아니므로 기본값으로 채운다. */
function source(over: Partial<SysmetricsSourceConfig> = {}): SysmetricsSourceConfig {
  return {
    agent_id: 'a1',
    agent_name: 'host',
    series: [],
    time_window_ms: 60 * 60_000,
    interval_ms: 60_000,
    aggregation: 'average',
    ...over,
  };
}

describe('hasInstanceAxis', () => {
  it('cpu·memory 는 인스턴스 축이 없고 나머지 셋은 있다', () => {
    expect(hasInstanceAxis('cpu')).toBe(false);
    expect(hasInstanceAxis('memory')).toBe(false);
    expect(hasInstanceAxis('network')).toBe(true);
    expect(hasInstanceAxis('diskIo')).toBe(true);
    expect(hasInstanceAxis('storage')).toBe(true);
  });
});

describe('resolveSysmetricsSeries — 시리즈 하나가 줄 하나다', () => {
  it('고른 순서 그대로 줄이 만들어진다', () => {
    const lines = resolveSysmetricsSeries(
      source({
        series: [
          { key: 'network.bytes_recv', target: 'en0' },
          { key: 'network.bytes_recv', target: 'en1' },
          { key: 'cpu.usage_percent' },
        ],
      }),
    );
    expect(lines.map((l) => l.name)).toEqual([
      'bytes_recv · {category=network, interface=en0}',
      'bytes_recv · {category=network, interface=en1}',
      'usage_percent · {category=cpu}',
    ]);
  });

  it('값마다 다른 대상을 고를 수 있다 (곱 모델로는 표현할 수 없던 조합)', () => {
    const lines = resolveSysmetricsSeries(
      source({
        series: [
          { key: 'network.bytes_recv', target: 'en0' },
          { key: 'network.bytes_sent', target: 'en1' },
        ],
      }),
    );
    expect(lines).toHaveLength(2);
    expect(lines.map((l) => l.target)).toEqual(['en0', 'en1']);
  });

  it('대상이 없으면 종합 한 줄이다', () => {
    const [line] = resolveSysmetricsSeries(source({ series: [{ key: 'network.bytes_recv' }] }));
    expect(line!.target).toBeUndefined();
    expect(line!.name).toBe('bytes_recv · {category=network}');
  });

  it('인스턴스 축이 없는 값에 남은 대상은 무시한다', () => {
    // 값을 바꾸며 대상이 남을 수 있다. 태그로 실으면 있지도 않은 축이 생긴다.
    const [line] = resolveSysmetricsSeries(
      source({ series: [{ key: 'cpu.usage_percent', target: 'en0' }] }),
    );
    expect(line!.target).toBeUndefined();
    // 인스턴스 태그는 없지만 분류 태그는 언제나 있다.
    expect(line!.tags).toEqual({ category: 'cpu' });
  });

  it('카탈로그에 없는 키는 버린다', () => {
    const lines = resolveSysmetricsSeries(
      source({ series: [{ key: 'nope.nothing' }, { key: 'cpu.usage_percent' }] }),
    );
    expect(lines.map((l) => l.name)).toEqual(['usage_percent · {category=cpu}']);
  });

  it('이름이 겹치면 뒤에 온 줄에 번호를 붙여 가른다 (Map 키 충돌 방지)', () => {
    const lines = resolveSysmetricsSeries(
      source({
        series: [
          { key: 'cpu.usage_percent', alias: '같은이름' },
          { key: 'memory.usage_percent', alias: '같은이름' },
        ],
      }),
    );
    expect(lines.map((l) => l.name)).toEqual(['같은이름', '같은이름 #2']);
  });

  it('색은 지정이 있으면 그대로, 없으면 줄 순서 기준 자동 팔레트다', () => {
    const lines = resolveSysmetricsSeries(
      source({
        series: [
          { key: 'network.bytes_recv', target: 'en0', color: '#abcdef' },
          { key: 'network.bytes_recv', target: 'en1' },
        ],
      }),
    );
    expect(lines[0]!.color).toBe('#abcdef');
    expect(lines[1]!.color).toMatch(/^#/);
    expect(lines[1]!.color).not.toBe('#abcdef');
  });
});

describe('Store 어휘 매핑 (measurement / tags)', () => {
  it('measurement 는 그룹을 뗀 필드 이름이다', () => {
    const [line] = resolveSysmetricsSeries(
      source({ series: [{ key: 'network.bytes_recv', target: 'en0' }] }),
    );
    expect(line!.measurement).toBe('bytes_recv');
  });

  it('그룹은 measurement 가 아니라 category 태그로 간다', () => {
    // 그룹이 measurement 안에 눌러붙으면 태그로 거를 수 없다.
    const cpu = resolveSysmetricsSeries(source({ series: [{ key: 'cpu.usage_percent' }] }));
    const mem = resolveSysmetricsSeries(source({ series: [{ key: 'memory.usage_percent' }] }));

    // 같은 measurement 를 category 가 가른다.
    expect(cpu[0]!.measurement).toBe('usage_percent');
    expect(mem[0]!.measurement).toBe('usage_percent');
    expect(cpu[0]!.tags).toEqual({ category: 'cpu' });
    expect(mem[0]!.tags).toEqual({ category: 'memory' });
    expect(cpu[0]!.name).not.toBe(mem[0]!.name);
  });

  it('category 는 에이전트 배치의 그룹 이름과 같다 (camelCase 가 아니다)', () => {
    // 스냅샷 형상의 키는 `diskIo` 지만 배치 그룹 이름은 `disk_io` 다. 저장 경로로
    // 옮겨 Store 소스로 볼 때 같은 값이어야 한다.
    const [line] = resolveSysmetricsSeries(
      source({ series: [{ key: 'disk_io.read_bytes', target: 'disk0' }] }),
    );
    expect(line!.tags).toEqual({ category: 'disk_io', device: 'disk0' });
  });

  it('대상 축마다 태그 키가 다르다', () => {
    const net = resolveSysmetricsSeries(
      source({ series: [{ key: 'network.bytes_recv', target: 'en0' }] }),
    );
    const disk = resolveSysmetricsSeries(
      source({ series: [{ key: 'disk_io.read_bytes', target: 'disk0' }] }),
    );
    const store = resolveSysmetricsSeries(
      source({ series: [{ key: 'storage.used_bytes', target: '/data' }] }),
    );

    expect(net[0]!.tags).toEqual({ category: 'network', interface: 'en0' });
    expect(disk[0]!.tags).toEqual({ category: 'disk_io', device: 'disk0' });
    expect(store[0]!.tags).toEqual({ category: 'storage', mountpoint: '/data' });
  });

  it('대상이 없으면 인스턴스 태그가 없다 (분류 태그는 남는다)', () => {
    const total = resolveSysmetricsSeries(source({ series: [{ key: 'network.bytes_recv' }] }));
    const cpu = resolveSysmetricsSeries(source({ series: [{ key: 'cpu.usage_percent' }] }));

    expect(total[0]!.tags).toEqual({ category: 'network' });
    expect(cpu[0]!.tags).toEqual({ category: 'cpu' });
  });

  it('내장 표기는 Store 와 같은 `measurement · {k=v}` 형식이다', () => {
    const field = findChartField('storage.used_bytes')!;
    expect(describeSysmetricSeries(field, '/data')).toBe(
      'used_bytes · {category=storage, mountpoint=/data}',
    );
    expect(describeSysmetricSeries(field, undefined)).toBe('used_bytes · {category=storage}');
  });
});

describe('resolveSysmetricsSeries — 이름 우선순위', () => {
  it('별칭이 형식과 내장 표기를 모두 이긴다', () => {
    const [line] = resolveSysmetricsSeries(
      source({
        series: [{ key: 'cpu.usage_percent', alias: '내가 붙인 이름' }],
        series_name_format: '{$.measurement}',
      }),
    );
    expect(line!.name).toBe('내가 붙인 이름');
  });

  it('형식은 내장 표기를 이기고 태그 토큰이 치환된다', () => {
    const lines = resolveSysmetricsSeries(
      source({
        series: [
          { key: 'storage.used_bytes', target: '/' },
          { key: 'storage.used_bytes', target: '/data' },
        ],
        series_name_format: '{$.tags.mountpoint} 사용량',
      }),
    );
    expect(lines.map((l) => l.name)).toEqual(['/ 사용량', '/data 사용량']);
  });

  it('형식 해석 결과가 비면 내장 표기로 폴백한다', () => {
    const [line] = resolveSysmetricsSeries(
      source({ series: [{ key: 'cpu.usage_percent' }], series_name_format: '{$.tags.nope}' }),
    );
    expect(line!.name).toBe('usage_percent · {category=cpu}');
  });

  it('defaultName 은 별칭·형식을 빼고 계산한다 (placeholder 용)', () => {
    const [line] = resolveSysmetricsSeries(
      source({
        series: [{ key: 'network.bytes_recv', target: 'en0', alias: '내부망' }],
        series_name_format: '{$.measurement}!',
      }),
    );
    expect(line!.name).toBe('내부망');
    expect(line!.defaultName).toBe('bytes_recv · {category=network, interface=en0}');
  });
});

describe('readRawValue — 원값 추출', () => {
  const snap = snapshot(1_000);

  it('인스턴스 축이 없는 값은 그룹에서 바로 읽는다', () => {
    const [line] = resolveSysmetricsSeries(source({ series: [{ key: 'cpu.usage_percent' }] }));
    expect(readRawValue(snap, line!)).toBe(42);
  });

  it('대상을 고르면 그 인스턴스의 값을 읽는다', () => {
    const [line] = resolveSysmetricsSeries(
      source({ series: [{ key: 'network.bytes_recv', target: 'en1' }] }),
    );
    expect(readRawValue(snap, line!)).toBe(2_000);
  });

  it('종합은 인스턴스 전체 합이다', () => {
    const [line] = resolveSysmetricsSeries(source({ series: [{ key: 'network.bytes_recv' }] }));
    expect(readRawValue(snap, line!)).toBe(3_000);
  });

  it('스토리지 종합 사용률은 퍼센트 합이 아니라 합계 용량 기준으로 재계산한다', () => {
    const [line] = resolveSysmetricsSeries(source({ series: [{ key: 'storage.usage_percent' }] }));
    // 90% + 10% = 100% 가 아니라 (900+100)/(1000+1000) = 50% 여야 한다.
    expect(readRawValue(snap, line!)).toBe(50);
  });

  it('수집하지 않는 그룹은 undefined 다 (0 이 아니다)', () => {
    const noNetwork = { ...snapshot(1_000), network: undefined };
    const [line] = resolveSysmetricsSeries(source({ series: [{ key: 'network.bytes_recv' }] }));
    expect(readRawValue(noNetwork, line!)).toBeUndefined();
  });

  it('사라진 대상은 undefined 다', () => {
    const [line] = resolveSysmetricsSeries(
      source({ series: [{ key: 'network.bytes_recv', target: 'gone0' }] }),
    );
    expect(readRawValue(snap, line!)).toBeUndefined();
  });
});

describe('readSeriesValue — 그릴 값', () => {
  const [cpu] = resolveSysmetricsSeries(source({ series: [{ key: 'cpu.usage_percent' }] }));
  const [net] = resolveSysmetricsSeries(
    source({ series: [{ key: 'network.bytes_recv', target: 'en0' }] }),
  );

  it('상태값은 기준점 없이도 그대로 나온다', () => {
    expect(readSeriesValue(snapshot(1_000), null, cpu!, 'sec')).toBe(42);
  });

  it('누적 카운터는 첫 표본에서 null 이다 (기준점이 없다)', () => {
    expect(readSeriesValue(snapshot(1_000), null, net!, 'sec')).toBeNull();
  });

  it('누적 카운터는 두 표본의 증가량을 단위시간으로 환산한다', () => {
    const prev = snapshot(1_000, { en0: 1_000, en1: 0 });
    const next = snapshot(3_000, { en0: 3_000, en1: 0 });
    // 2000 bytes / 2 s = 1000 B/s
    expect(readSeriesValue(next, prev, net!, 'sec')).toBe(1_000);
    // 분당은 60배
    expect(readSeriesValue(next, prev, net!, 'min')).toBe(60_000);
  });

  it('카운터가 되감기면 null 이다 (거대한 음수 스파이크를 만들지 않는다)', () => {
    const prev = snapshot(1_000, { en0: 5_000, en1: 0 });
    const next = snapshot(3_000, { en0: 10, en1: 0 });
    expect(readSeriesValue(next, prev, net!, 'sec')).toBeNull();
  });

  it('누적 모드는 원값을 그대로 낸다 — 기준점이 필요 없다', () => {
    const [total] = resolveSysmetricsSeries(
      source({ series: [{ key: 'network.bytes_recv', target: 'en0', mode: 'total' }] }),
    );
    // 증가량과 달리 첫 표본부터 값이 있다.
    expect(readSeriesValue(snapshot(1_000), null, total!, 'sec')).toBe(1_000);
    // 되감겨도 원값 그대로다 — 스파이크를 만들 환산이 없다.
    const prev = snapshot(1_000, { en0: 5_000, en1: 0 });
    const next = snapshot(3_000, { en0: 10, en1: 0 });
    expect(readSeriesValue(next, prev, total!, 'sec')).toBe(10);
  });

  it('상태값에 남은 mode 는 무시한다', () => {
    // 값을 바꾸며 남을 수 있다. 환산할 것이 없는 값에 표현 축이 생기면 안 된다.
    const [line] = resolveSysmetricsSeries(
      source({ series: [{ key: 'cpu.usage_percent', mode: 'total' }] }),
    );
    expect(line!.mode).toBe('rate');
    expect(line!.tags).toEqual({ category: 'cpu' });
    expect(readSeriesValue(snapshot(1_000), null, line!, 'sec')).toBe(42);
  });

  it('경과가 0 이면 null 이다', () => {
    const prev = snapshot(1_000, { en0: 1_000, en1: 0 });
    const next = snapshot(1_000, { en0: 2_000, en1: 0 });
    expect(readSeriesValue(next, prev, net!, 'sec')).toBeNull();
  });
});

describe('sysmetricRowId — 표 선택 키', () => {
  it('같은 (값, 대상)은 언제나 같은 id 다', () => {
    expect(sysmetricRowId({ key: 'network.bytes_recv', target: 'en0' })).toBe(
      sysmetricRowId({ key: 'network.bytes_recv', target: 'en0' }),
    );
  });

  it('대상이 다르면 id 도 다르다 (종합과 개별도 다르다)', () => {
    const total = sysmetricRowId({ key: 'network.bytes_recv' });
    const en0 = sysmetricRowId({ key: 'network.bytes_recv', target: 'en0' });
    const en1 = sysmetricRowId({ key: 'network.bytes_recv', target: 'en1' });
    expect(new Set([total, en0, en1]).size).toBe(3);
  });
});

describe('sysmetricsCandidateRows — 시리즈 표 후보 행', () => {
  const targets: SysMetricsTargets = {
    mountpoints: ['/'],
    devices: ['disk0'],
    interfaces: ['en0', 'en1'],
  };

  /** 특정 값의 행만 추린다. */
  const rowsOf = (key: string, rows: ReturnType<typeof sysmetricsCandidateRows>) =>
    rows.filter((r) => r.metricKey === key);

  it('인스턴스 축이 없는 값은 행 하나다', () => {
    const rows = rowsOf('cpu.usage_percent', sysmetricsCandidateRows(targets, []));
    expect(rows).toHaveLength(1);
    expect(rows[0]!.tags).toEqual({ category: 'cpu' });
  });

  it('인스턴스 축이 있는 값은 종합 + 대상마다 한 행이다', () => {
    const rows = rowsOf('network.bytes_recv', sysmetricsCandidateRows(targets, []));
    expect(rows.map((r) => r.target)).toEqual([undefined, 'en0', 'en1']);
    expect(rows[0]!.tags).toEqual({ category: 'network' });
    expect(rows[1]!.tags).toEqual({ category: 'network', interface: 'en0' });
  });

  it('key 컬럼은 measurement 이고 카탈로그 키는 metricKey 에 따로 실린다', () => {
    // 둘을 같은 자리에 두면 표시용 이름이 그대로 config 에 저장돼 카탈로그를 못 찾는다.
    const [row] = rowsOf('cpu.usage_percent', sysmetricsCandidateRows(targets, []));
    expect(row!.key).toBe('usage_percent');
    expect(row!.metricKey).toBe('cpu.usage_percent');
    expect(row!.field).toBe('');
    // 데이터타입 자리에는 표기 방식이 들어간다.
    expect(row!.dataType).toBe('percent');
  });

  it('보고되지 않는데 이미 고른 대상도 행으로 남고 배지가 붙는다', () => {
    // 인터페이스가 내려갔거나 수집 필터가 좁혀졌을 때 — 행이 사라지면 해제할 수도 없다.
    const rows = rowsOf(
      'network.bytes_recv',
      sysmetricsCandidateRows(targets, [{ key: 'network.bytes_recv', target: 'gone0' }]),
    );
    const gone = rows.find((r) => r.target === 'gone0');
    expect(gone).toBeDefined();
    expect(gone!.badge).toBe(SYSMETRIC_MISSING_BADGE);
    // 보고되는 행에는 배지가 없다.
    expect(rows.find((r) => r.target === 'en0')!.badge).toBeUndefined();
  });

  it('첫 표본 이전(보고 목록 없음)에도 고른 대상은 행으로 남는다', () => {
    const rows = rowsOf(
      'network.bytes_recv',
      sysmetricsCandidateRows(undefined, [{ key: 'network.bytes_recv', target: 'en0' }]),
    );
    // 종합 행 + 고른 대상 행.
    expect(rows.map((r) => r.target)).toEqual([undefined, 'en0']);
  });

  it('행 id 가 config 시리즈의 id 와 일치한다 (체크 상태가 어긋나지 않는다)', () => {
    const picked = { key: 'network.bytes_recv', target: 'en1' };
    const rows = sysmetricsCandidateRows(targets, [picked]);
    expect(rows.some((r) => r.id === sysmetricRowId(picked))).toBe(true);
  });

  it('행 id 는 서로 겹치지 않는다', () => {
    const rows = sysmetricsCandidateRows(targets, []);
    expect(new Set(rows.map((r) => r.id)).size).toBe(rows.length);
  });
});

describe('누적 원값 모드 — 태그 축과 행 id', () => {
  it('누적 원값만 mode 태그를 싣는다 (증가량은 싣지 않는다)', () => {
    // 증가량에 태그를 실으면 이미 저장된 패널이 시리즈를 찾지 못해 빈 차트가 된다.
    const [rate] = resolveSysmetricsSeries(
      source({ series: [{ key: 'network.bytes_recv', target: 'en0' }] }),
    );
    expect(rate!.tags).toEqual({ category: 'network', interface: 'en0' });

    const [total] = resolveSysmetricsSeries(
      source({ series: [{ key: 'network.bytes_recv', target: 'en0', mode: 'total' }] }),
    );
    expect(total!.tags).toEqual({ category: 'network', interface: 'en0', mode: 'total' });
  });

  it('두 표현은 이름이 갈린다 — 한 패널에 나란히 놓아도 겹치지 않는다', () => {
    const lines = resolveSysmetricsSeries(
      source({
        series: [
          { key: 'network.bytes_recv', target: 'en0' },
          { key: 'network.bytes_recv', target: 'en0', mode: 'total' },
        ],
      }),
    );
    expect(lines).toHaveLength(2);
    expect(lines[0]!.name).not.toBe(lines[1]!.name);
    // 겹쳤다면 `#2` 접미가 붙었을 것이다.
    expect(lines[1]!.name).not.toContain('#2');
  });

  it('행 id 는 표현과 무관하다 — 모드를 바꿔도 표의 선택이 풀리지 않는다', () => {
    expect(sysmetricRowId({ key: 'network.bytes_recv', target: 'en0' })).toBe(
      sysmetricRowId({ key: 'network.bytes_recv', target: 'en0', mode: 'total' } as never),
    );
  });
});
