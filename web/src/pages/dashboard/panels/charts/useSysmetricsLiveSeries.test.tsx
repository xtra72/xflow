// sysmetrics 실시간 계열 누적 훅 테스트.
//
// 잠그는 것:
//   - 스냅샷이 올 때마다 점이 한 개씩 쌓인다(계열 이름·스타일은 설정 표와 같은 어휘)
//   - 누적 카운터는 **초당** 증가량이다 — 이력이 초당으로 저장하므로 모드를 바꿔도 값이 같아야 한다
//   - 표시 창보다 오래된 점은 버린다
//   - 비활성(enabled=false)이면 폴링도 누적도 하지 않는다

import { describe, expect, it, vi, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';

/** 훅이 돌려줄 스냅샷. 테스트가 갈아 끼운다. */
const snap = vi.hoisted(() => ({
  current: null as unknown,
  previous: null as unknown,
  state: 'ready' as string,
  calls: [] as Array<{ agentId: string; refreshMs: number }>,
}));

vi.mock('@/pages/dashboard/panels/sysmetrics/useSysMetricsSnapshot', () => ({
  useSysMetricsSnapshot: (agentId: string, refreshMs: number) => {
    snap.calls.push({ agentId, refreshMs });
    return { snapshot: snap.current, previous: snap.previous, state: snap.state };
  },
}));

import { useSysmetricsLiveSeries } from './useSysmetricsLiveSeries';
import type { SysmetricsSourceConfig } from './chartChannelTypes';

/** cpu 사용률(상태값) + 네트워크 수신량(누적 카운터) 스냅샷. */
function snapshot(at: number, cpu: number, bytesRecv: number) {
  return {
    status: 'running',
    collectedAt: at,
    intervalSeconds: 5,
    cpu: { usage_percent: cpu },
    network: { en0: { bytes_recv: bytesRecv } },
    targets: { mountpoints: [], devices: [], interfaces: ['en0'] },
  };
}

function source(extra: Partial<SysmetricsSourceConfig> = {}): SysmetricsSourceConfig {
  return {
    agent_id: 'a1',
    agent_name: 'host-1',
    query_mode: 'live',
    series: [
      { key: 'cpu.usage_percent', alias: 'CPU' },
      { key: 'network.bytes_recv', target: 'en0', alias: 'RX' },
    ],
    time_window_ms: 60_000,
    interval_ms: 5_000,
    aggregation: 'last',
    refresh_interval_ms: 2_000,
    ...extra,
  } as SysmetricsSourceConfig;
}

beforeEach(() => {
  snap.current = null;
  snap.previous = null;
  snap.state = 'ready';
  snap.calls.length = 0;
});

describe('useSysmetricsLiveSeries', () => {
  it('비활성이면 폴링하지 않고 idle 을 돌려준다', () => {
    const { result } = renderHook(() => useSysmetricsLiveSeries(source(), false));
    expect(result.current.status).toBe('idle');
    expect(result.current.seriesNames).toEqual([]);
    // 에이전트 id 를 비워 넘겨 폴링 자체를 끈다.
    expect(snap.calls.every((c) => c.agentId === '')).toBe(true);
  });

  it('설정한 폴링 주기로 에이전트를 본다', () => {
    renderHook(() => useSysmetricsLiveSeries(source(), true));
    expect(snap.calls[0]).toEqual({ agentId: 'a1', refreshMs: 2_000 });
  });

  it('스냅샷이 올 때마다 계열에 점이 쌓인다', () => {
    snap.current = snapshot(10_000, 12, 1_000);
    const { result, rerender } = renderHook(() => useSysmetricsLiveSeries(source(), true));
    expect(result.current.seriesNames).toEqual(['CPU', 'RX']);
    // 첫 표본: 상태값은 그리고, 누적 카운터는 기준점이 없어 아직 못 그린다.
    expect(result.current.seriesEntries.get('CPU')).toHaveLength(1);
    expect(result.current.seriesEntries.get('RX')).toHaveLength(0);

    act(() => {
      snap.previous = snapshot(10_000, 12, 1_000);
      snap.current = snapshot(12_000, 20, 3_000);
    });
    rerender();
    expect(result.current.seriesEntries.get('CPU')).toHaveLength(2);
    expect(result.current.seriesEntries.get('RX')).toHaveLength(1);
  });

  it('누적 카운터는 초당 증가량이다(이력과 같은 자릿수여야 모드를 바꿔도 값이 안 튄다)', () => {
    snap.current = snapshot(10_000, 5, 1_000);
    const { result, rerender } = renderHook(() => useSysmetricsLiveSeries(source(), true));
    act(() => {
      snap.previous = snapshot(10_000, 5, 1_000);
      // 2초 동안 2000 증가 → 1000/s.
      snap.current = snapshot(12_000, 5, 3_000);
    });
    rerender();
    expect(result.current.seriesEntries.get('RX')?.[0]?.value).toBe(1_000);
  });

  it('표시 창보다 오래된 점은 버린다', () => {
    snap.current = snapshot(0, 1, 0);
    const { result, rerender } = renderHook(() =>
      useSysmetricsLiveSeries(source({ time_window_ms: 5_000 }), true),
    );
    act(() => {
      snap.current = snapshot(100_000, 2, 0);
    });
    rerender();
    // 창(5초)을 훌쩍 넘긴 첫 점은 남지 않는다.
    const cpu = result.current.seriesEntries.get('CPU') ?? [];
    expect(cpu).toHaveLength(1);
    expect(cpu[0]?.timestamp).toBe(100_000);
  });

  it('에이전트를 못 읽으면 오류, 표본 이전은 오류가 아니다', () => {
    snap.state = 'unavailable';
    const { result, rerender } = renderHook(() => useSysmetricsLiveSeries(source(), true));
    expect(result.current.status).toBe('error');
    act(() => {
      snap.state = 'no_sample';
    });
    rerender();
    // 조회는 되고 있고 그릴 점이 아직 없을 뿐이다 — 오류로 올리면 마지막 렌더를 버린다.
    expect(result.current.status).toBe('connected');
  });
});
