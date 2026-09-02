// 모니터링 패널 표시 옵션 테스트 — 열 개수 상한 / 갱신 주기 / 악센트 색.

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, act } from '@testing-library/react';
import { renderHook } from '@testing-library/react';

import {
  joinDuration,
  maxColsOptions,
  readAccent,
  readInterfaces,
  readMaxCols,
  readRefreshMs,
  readWindowSec,
  splitDuration,
} from './monitorPanelConfig';
import { useThrottledValue } from './useThrottledValue';

describe('maxColsOptions', () => {
  it('고를 수 있는 열 수는 표시 항목 수까지다', () => {
    // 항목이 3개인데 6열을 고를 수 있으면 3칸은 영원히 비어 있다.
    expect(maxColsOptions(3)).toEqual([1, 2, 3]);
    expect(maxColsOptions(1)).toEqual([1]);
  });

  it('항목이 없어도 최소 1개는 남긴다', () => {
    expect(maxColsOptions(0)).toEqual([1]);
  });

  it('절대 상한을 넘지 않는다', () => {
    expect(maxColsOptions(50)).toHaveLength(12);
  });
});

describe('readMaxCols', () => {
  it('값이 없으면 기본값', () => {
    expect(readMaxCols(undefined, 3)).toBe(3);
    expect(readMaxCols({}, 2)).toBe(2);
  });

  it('저장된 값을 받는다', () => {
    expect(readMaxCols({ maxCols: 1 }, 3)).toBe(1);
    expect(readMaxCols({ maxCols: 6 }, 3)).toBe(6);
  });

  it('범위 밖이거나 숫자가 아니면 기본값으로 되돌린다', () => {
    expect(readMaxCols({ maxCols: 0 }, 3)).toBe(3);
    expect(readMaxCols({ maxCols: 99 }, 3)).toBe(3);
    expect(readMaxCols({ maxCols: 'two' }, 3)).toBe(3);
  });

  it('표시 항목 수보다 많은 열은 항목 수로 죈다', () => {
    // 항목을 줄여도 예전 설정이 남아 빈 칸이 생기는 것을 막는다.
    expect(readMaxCols({ maxCols: 6 }, 3, 2)).toBe(2);
    expect(readMaxCols({ maxCols: 2 }, 3, 5)).toBe(2);
  });
});

describe('splitDuration / joinDuration', () => {
  it('ms 를 시/분/초로 나눈다', () => {
    expect(splitDuration(5_000)).toEqual({ hours: 0, minutes: 0, seconds: 5 });
    expect(splitDuration(3_661_000)).toEqual({ hours: 1, minutes: 1, seconds: 1 });
  });

  it('시/분/초를 ms 로 합친다', () => {
    expect(joinDuration(0, 1, 30)).toBe(90_000);
    expect(joinDuration(2, 0, 0)).toBe(7_200_000);
  });

  it('하한 아래로는 내려가지 않는다 (폴링이 스스로를 앞지르는 것 방지)', () => {
    expect(joinDuration(0, 0, 0)).toBe(1_000);
  });

  it('상한을 넘지 않는다', () => {
    expect(joinDuration(48, 0, 0)).toBe(24 * 60 * 60 * 1_000);
  });

  it('음수·비정상 입력은 0으로 본다', () => {
    expect(joinDuration(-5, 0, 10)).toBe(10_000);
    expect(joinDuration(Number.NaN, 0, 30)).toBe(30_000);
  });

  it('나눈 뒤 다시 합치면 같은 값이다', () => {
    const ms = 3_723_000;
    const d = splitDuration(ms);
    expect(joinDuration(d.hours, d.minutes, d.seconds)).toBe(ms);
  });
});

describe('readWindowSec', () => {
  it('값이 없으면 기본값', () => {
    expect(readWindowSec({}, 300)).toBe(300);
  });

  it('허용 범위의 값을 받는다', () => {
    expect(readWindowSec({ windowSec: 60 }, 300)).toBe(60);
    expect(readWindowSec({ windowSec: 3_600 }, 300)).toBe(3_600);
  });

  it('범위 밖은 기본값으로 되돌린다', () => {
    expect(readWindowSec({ windowSec: 1 }, 300)).toBe(300);
    expect(readWindowSec({ windowSec: 999_999 }, 300)).toBe(300);
  });
});

describe('readInterfaces', () => {
  it('배열이 아니면 빈 목록 (전체 합산만 본다)', () => {
    expect(readInterfaces({})).toEqual([]);
    expect(readInterfaces({ interfaces: 'en0' })).toEqual([]);
  });

  it('문자열만 남긴다', () => {
    expect(readInterfaces({ interfaces: ['en0', 3, '', null, 'lo0'] })).toEqual(['en0', 'lo0']);
  });
});

describe('readRefreshMs', () => {
  it('값이 없으면 기본값', () => {
    expect(readRefreshMs({}, 5_000)).toBe(5_000);
  });

  it('허용 범위의 값을 받는다', () => {
    expect(readRefreshMs({ refreshMs: 1_000 }, 5_000)).toBe(1_000);
    expect(readRefreshMs({ refreshMs: 3_600_000 }, 5_000)).toBe(3_600_000);
  });

  it('사실상 갱신하지 않는 값은 기본값으로 되돌린다', () => {
    expect(readRefreshMs({ refreshMs: 0 }, 5_000)).toBe(5_000);
    expect(readRefreshMs({ refreshMs: -1 }, 5_000)).toBe(5_000);
    expect(readRefreshMs({ refreshMs: 500 }, 5_000)).toBe(5_000);
    expect(readRefreshMs({ refreshMs: 999_999_999 }, 5_000)).toBe(5_000);
  });
});

describe('readAccent', () => {
  it('그룹 색이 없으면 패널 색으로 떨어진다', () => {
    const { accentColor } = readAccent({ panelColor: '#ff0000' });
    expect(accentColor('header')).toBe('#ff0000');
  });

  it('그룹별 색이 패널 색을 이긴다', () => {
    const { accentColor } = readAccent({
      panelColor: '#ff0000',
      accentElements: { header: '#00ff00' },
    });
    expect(accentColor('header')).toBe('#00ff00');
    expect(accentColor('value')).toBe('#ff0000');
  });

  it('그룹이 false 면 그 그룹만 색을 뗀다', () => {
    const { accentColor } = readAccent({
      panelColor: '#ff0000',
      accentElements: { header: false },
    });
    expect(accentColor('header')).toBeUndefined();
    expect(accentColor('value')).toBe('#ff0000');
  });

  it('설정이 전혀 없으면 색이 없다', () => {
    const { accentColor } = readAccent(undefined);
    expect(accentColor('header')).toBeUndefined();
  });
});

describe('useThrottledValue', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('첫 값은 지연 없이 통과한다', () => {
    const { result } = renderHook(() => useThrottledValue('a', 1_000));
    expect(result.current).toBe('a');
  });

  it('주기 안의 변경은 미뤘다가 최신값으로 한 번에 반영한다', () => {
    const { result, rerender } = renderHook(
      ({ value }) => useThrottledValue(value, 1_000),
      { initialProps: { value: 'a' } },
    );

    rerender({ value: 'b' });
    rerender({ value: 'c' });
    // 아직 주기가 지나지 않아 화면 값은 그대로다.
    expect(result.current).toBe('a');

    act(() => {
      vi.advanceTimersByTime(1_000);
    });
    // 중간값 'b' 는 건너뛰고 최신값 'c' 가 반영된다.
    expect(result.current).toBe('c');
  });

  it('주기가 0 이하면 throttle 하지 않는다', () => {
    const { result, rerender } = renderHook(
      ({ value }) => useThrottledValue(value, 0),
      { initialProps: { value: 'a' } },
    );

    rerender({ value: 'b' });
    expect(result.current).toBe('b');
  });
});

// --- 패널 렌더 레벨 검증 ---

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (key: string) => key, locale: 'ko', setLocale: () => {} }),
}));

const metricChartSpy = vi.hoisted(() => vi.fn());
vi.mock('@/pages/monitoring/MetricsChart', () => ({
  MetricChannelChart: (props: { channel: string; color?: string }) => {
    metricChartSpy(props);
    return <div data-testid={`metric-chart-${props.channel}`} data-color={props.color ?? ''} />;
  },
}));

const systemMetricsSpy = vi.hoisted(() => vi.fn());
vi.mock('@/services/api/monitorService', () => ({
  useNetworkStats: () => ({ data: undefined, isLoading: false }),
  useSystemMetrics: (ms?: number) => {
    systemMetricsSpy(ms);
    return { data: { go_routines: 7 }, isLoading: false };
  },
}));

vi.mock('@/hooks', () => ({
  useWebSocket: () => ({ state: 'connected', client: null }),
  useFlows: () => ({ data: { data: [] } }),
}));

vi.mock('@/pages/monitoring/monitorStream', () => ({
  useMonitorStream: () => ({
    metrics: { cpu: [], memory: [], throughput: [], errorRate: [] },
    logs: [],
    events: [],
    logsReceived: 0,
    eventsReceived: 0,
  }),
}));

import MonitorStatsPanel from './MonitorStatsPanel';
import MonitorMetricsPanel from './MonitorMetricsPanel';

describe('패널에 설정이 반영된다', () => {
  beforeEach(() => {
    systemMetricsSpy.mockClear();
    metricChartSpy.mockClear();
  });

  it('통계 패널의 열 개수 상한이 그리드에 적용된다', () => {
    render(
      <MonitorStatsPanel
        panelId="p"
        title="t"
        config={{ items: ['goRoutines', 'uptime', 'heapAlloc', 'memSys'], maxCols: 4 }}
      />,
    );

    const grid = screen.getByTestId('monitor-stats-panel-grid');
    // ResizeObserver 가 폭을 알려주기 전에는 상한을 그대로 쓴다.
    expect(grid).toHaveAttribute('data-cols', '4');
    expect(grid.style.gridTemplateColumns).toBe('repeat(4, minmax(0, 1fr))');
  });

  it('항목보다 많은 열을 저장해 두었어도 항목 수까지만 쓴다', () => {
    render(
      <MonitorStatsPanel panelId="p" title="t" config={{ items: ['goRoutines'], maxCols: 4 }} />,
    );

    expect(screen.getByTestId('monitor-stats-panel-grid')).toHaveAttribute('data-cols', '1');
  });

  it('통계 패널의 갱신 주기가 런타임 폴링 주기로 전달된다', () => {
    render(
      <MonitorStatsPanel panelId="p" title="t" config={{ items: ['goRoutines'], refreshMs: 30_000 }} />,
    );

    expect(systemMetricsSpy).toHaveBeenCalledWith(30_000);
  });

  it('갱신 주기를 지정하지 않으면 기존 5초를 유지한다', () => {
    render(<MonitorStatsPanel panelId="p" title="t" config={{ items: ['goRoutines'] }} />);

    expect(systemMetricsSpy).toHaveBeenCalledWith(5_000);
  });

  it('메트릭 패널의 열 개수 상한이 그리드에 적용된다', () => {
    render(<MonitorMetricsPanel panelId="p" title="t" config={{ items: ['cpu'], maxCols: 1 }} />);

    expect(screen.getByTestId('monitor-metrics-panel-grid')).toHaveAttribute('data-cols', '1');
  });

  it('메트릭 패널은 표시 구간만큼 잘라 차트에 넘긴다', () => {
    render(
      <MonitorMetricsPanel panelId="p" title="t" config={{ items: ['cpu'], windowSec: 2 }} />,
    );

    // 스트림 mock 은 빈 배열이라 자른 결과도 빈 배열이다 — 자르기가 터지지 않는지 확인한다.
    expect(metricChartSpy).toHaveBeenCalledWith(
      expect.objectContaining({ data: expect.objectContaining({ cpu: [] }) }),
    );
  });

  it('메트릭 패널의 채널 색 설정이 차트로 전달된다', () => {
    render(
      <MonitorMetricsPanel
        panelId="p"
        title="t"
        config={{ items: ['cpu'], accentElements: { cpu: '#123456' } }}
      />,
    );

    expect(screen.getByTestId('metric-chart-cpu')).toHaveAttribute('data-color', '#123456');
  });

  it('색 설정이 없으면 차트가 채널 기본색을 쓰도록 비워 둔다', () => {
    render(<MonitorMetricsPanel panelId="p" title="t" config={{ items: ['cpu'] }} />);

    expect(screen.getByTestId('metric-chart-cpu')).toHaveAttribute('data-color', '');
  });

  it('통계 패널의 값 악센트 색이 카드에 적용된다', () => {
    render(
      <MonitorStatsPanel
        panelId="p"
        title="t"
        config={{ items: ['goRoutines'], accentElements: { value: '#abcdef' } }}
      />,
    );

    expect(screen.getByTestId('monitor-stat-value-goRoutines')).toHaveStyle({ color: '#abcdef' });
  });
});
