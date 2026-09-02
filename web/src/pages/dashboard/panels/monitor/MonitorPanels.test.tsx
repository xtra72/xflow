// 모니터링 대시보드 패널 4종 테스트.
// config.items 로 표시 항목을 고르는 동작과, 로그/이벤트의 필터 의미를 검증한다.

import { describe, it, expect, vi } from 'vitest';
import { render as rtlRender, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (key: string) => key, locale: 'ko', setLocale: () => {} }),
}));

// 차트는 recharts 실렌더가 불필요하므로 스텁으로 대체한다.
vi.mock('@/pages/monitoring/MetricsChart', () => ({
  MetricChannelChart: ({ channel }: { channel: string }) => (
    <div data-testid={`metric-chart-${channel}`} />
  ),
}));

vi.mock('@/pages/monitoring/LogViewer', () => ({
  default: ({ title, entries }: { title?: string; entries: { level: string }[] }) => (
    <div
      data-testid="log-viewer"
      data-title={title ?? ''}
      data-levels={entries.map((e) => e.level).join(',')}
    />
  ),
}));

vi.mock('@/pages/monitoring/EventTimeline', () => ({
  default: ({ title, events }: { title?: string; events: { type: string }[] }) => (
    <div
      data-testid="event-timeline"
      data-title={title ?? ''}
      data-types={events.map((e) => e.type).join(',')}
    />
  ),
}));

vi.mock('@/hooks', () => ({
  useWebSocket: () => ({ state: 'connected', client: null }),
  useFlows: () => ({ data: { data: [{ id: 'f1', status: 'Running' }] } }),
}));

vi.mock('@/services/api/monitorService', () => ({
  useNetworkStats: () => ({ data: undefined, isLoading: false }),
  useSystemMetrics: () => ({
    data: { go_routines: 42, uptime_seconds: 60, memory_usage_percent: 10 },
    isLoading: false,
  }),
}));

// 스트림은 실제 모듈을 쓰되 스냅샷만 고정 주입한다.
const streamMock = vi.hoisted(() => ({
  current: {
    metrics: { cpu: [], memory: [], throughput: [], errorRate: [] },
    logs: [
      { id: '1', timestamp: '', level: 'ERROR', message: 'e', component: '', source: 'system' },
      { id: '2', timestamp: '', level: 'INFO', message: 'i', component: '', source: 'system' },
      { id: '3', timestamp: '', level: 'WARN', message: 'w', component: '', source: 'system' },
    ],
    events: [
      { id: '1', type: 'error', message: 'e', timestamp: '2026-01-01T00:00:00Z' },
      { id: '2', type: 'deployment', message: 'd', timestamp: '2026-01-01T00:00:01Z' },
      { id: '3', type: 'system', message: 's', timestamp: '2026-01-01T00:00:02Z' },
    ],
    logsReceived: 7,
    eventsReceived: 3,
  },
}));

vi.mock('@/pages/monitoring/monitorStream', () => ({
  useMonitorStream: () => streamMock.current,
}));

import MonitorStatsPanel from './MonitorStatsPanel';
import MonitorMetricsPanel from './MonitorMetricsPanel';
import MonitorLogsPanel from './MonitorLogsPanel';
import MonitorEventsPanel from './MonitorEventsPanel';
import { readPanelItems } from './monitorPanelConfig';

/** 로그 패널의 이름 링크가 useNavigate 를 쓰므로 Router 안에서 렌더한다. */
function render(ui: React.ReactElement) {
  return rtlRender(<MemoryRouter>{ui}</MemoryRouter>);
}
import { DEFAULT_LAYOUT } from '@/pages/monitoring/monitoringLayout';

describe('readPanelItems', () => {
  it('items 가 없으면 섹션 기본 항목을 쓴다', () => {
    expect(readPanelItems('metrics', {})).toEqual(DEFAULT_LAYOUT.metrics);
    expect(readPanelItems('stats', undefined)).toEqual(DEFAULT_LAYOUT.stats);
  });

  it('알 수 없는 항목 키는 버린다', () => {
    expect(readPanelItems('metrics', { items: ['cpu', 'bogus'] })).toEqual(['cpu']);
  });

  it('빈 배열은 "모두 껐다"는 정상 상태로 존중한다', () => {
    expect(readPanelItems('metrics', { items: [] })).toEqual([]);
  });
});

describe('MonitorStatsPanel', () => {
  it('config.items 에 고른 통계만 렌더한다', () => {
    render(
      <MonitorStatsPanel
        panelId="p1"
        title="통계"
        config={{ items: ['goRoutines', 'runningFlows'] }}
      />,
    );

    expect(screen.getByTestId('monitor-stat-value-goRoutines')).toHaveTextContent('42');
    expect(screen.getByTestId('monitor-stat-value-runningFlows')).toHaveTextContent('1');
    expect(screen.queryByTestId('monitor-stat-value-uptime')).not.toBeInTheDocument();
  });

  it('수신량 통계는 스트림의 누적 카운트를 읽는다', () => {
    render(
      <MonitorStatsPanel panelId="p1" title="통계" config={{ items: ['logsReceived'] }} />,
    );

    expect(screen.getByTestId('monitor-stat-value-logsReceived')).toHaveTextContent('7');
  });

  it('항목을 모두 끄면 빈 상태 안내를 보여준다', () => {
    render(<MonitorStatsPanel panelId="p1" title="통계" config={{ items: [] }} />);

    expect(screen.getByTestId('monitor-stats-panel-empty')).toBeInTheDocument();
  });
});

describe('MonitorMetricsPanel', () => {
  it('고른 채널의 차트만 렌더한다', () => {
    render(
      <MonitorMetricsPanel panelId="p2" title="메트릭" config={{ items: ['cpu', 'errorRate'] }} />,
    );

    expect(screen.getByTestId('metric-chart-cpu')).toBeInTheDocument();
    expect(screen.getByTestId('metric-chart-errorRate')).toBeInTheDocument();
    expect(screen.queryByTestId('metric-chart-memory')).not.toBeInTheDocument();
  });

  it('items 가 없으면 기본 4채널을 모두 렌더한다', () => {
    render(<MonitorMetricsPanel panelId="p2" title="메트릭" config={{}} />);

    for (const channel of DEFAULT_LAYOUT.metrics) {
      expect(screen.getByTestId(`metric-chart-${channel}`)).toBeInTheDocument();
    }
  });
});

describe('MonitorLogsPanel', () => {
  it("'all' 이면 레벨 필터를 걸지 않는다", () => {
    render(<MonitorLogsPanel panelId="p3" title="로그" config={{ items: ['all'] }} />);

    expect(screen.getByTestId('log-viewer')).toHaveAttribute('data-levels', 'ERROR,INFO,WARN');
  });

  it('고른 레벨의 합집합으로 거른다', () => {
    render(<MonitorLogsPanel panelId="p3" title="로그" config={{ items: ['error', 'warn'] }} />);

    expect(screen.getByTestId('log-viewer')).toHaveAttribute('data-levels', 'ERROR,WARN');
  });

  it('아무 레벨도 고르지 않으면 전부 보여준다', () => {
    render(<MonitorLogsPanel panelId="p3" title="로그" config={{ items: [] }} />);

    expect(screen.getByTestId('log-viewer')).toHaveAttribute('data-levels', 'ERROR,INFO,WARN');
  });

  it('패널 제목을 뷰어 헤더로 넘긴다', () => {
    render(<MonitorLogsPanel panelId="p3" title="시스템 로그" config={{}} />);

    expect(screen.getByTestId('log-viewer')).toHaveAttribute('data-title', '시스템 로그');
  });
});

describe('MonitorEventsPanel', () => {
  it("'all' 이면 유형 필터를 걸지 않는다", () => {
    render(<MonitorEventsPanel panelId="p4" title="이벤트" config={{ items: ['all'] }} />);

    expect(screen.getByTestId('event-timeline')).toHaveAttribute(
      'data-types',
      'error,deployment,system',
    );
  });

  it('고른 유형만 남긴다', () => {
    render(
      <MonitorEventsPanel panelId="p4" title="이벤트" config={{ items: ['error', 'system'] }} />,
    );

    expect(screen.getByTestId('event-timeline')).toHaveAttribute('data-types', 'error,system');
  });
});

// --- 네트워크 패널 ---

const networkSeriesSpy = vi.hoisted(() => vi.fn());
vi.mock('@/pages/monitoring/useNetworkSeries', () => ({
  useNetworkSeries: (refreshMs: number, interfaces: string[], unit: string) => {
    networkSeriesSpy({ refreshMs, interfaces, unit });
    return {
      series: { total: { rxBytes: [{ time: 't', value: 1 }] } },
      available: ['total', 'en0'],
      isLoading: false,
      missing: [] as string[],
    };
  },
}));

vi.mock('@/pages/monitoring/NetworkChart', () => ({
  default: (p: { channel: string; interfaces: string[]; unit: string }) => (
    <div
      data-testid={`network-chart-${p.channel}`}
      data-ifaces={p.interfaces.join(',')}
      data-unit={p.unit}
    />
  ),
}));

import MonitorNetworkPanel from './MonitorNetworkPanel';

describe('MonitorNetworkPanel', () => {
  it('config.items 에 고른 채널만 렌더한다', () => {
    render(
      <MonitorNetworkPanel
        panelId="n"
        title="네트워크"
        config={{ items: ['rxBytes', 'txPacketsTotal'] }}
      />,
    );

    expect(screen.getByTestId('network-chart-rxBytes')).toBeInTheDocument();
    expect(screen.getByTestId('network-chart-txPacketsTotal')).toBeInTheDocument();
    expect(screen.queryByTestId('network-chart-txBytes')).not.toBeInTheDocument();
  });

  it('인터페이스를 고르지 않으면 전체 합산 하나만 그린다', () => {
    render(<MonitorNetworkPanel panelId="n" title="n" config={{ items: ['rxBytes'] }} />);

    expect(screen.getByTestId('network-chart-rxBytes')).toHaveAttribute('data-ifaces', 'total');
  });

  it('고른 인터페이스가 모두 차트로 넘어간다', () => {
    render(
      <MonitorNetworkPanel
        panelId="n"
        title="n"
        config={{ items: ['rxBytes'], interfaces: ['en0', 'lo0'] }}
      />,
    );

    expect(screen.getByTestId('network-chart-rxBytes')).toHaveAttribute('data-ifaces', 'en0,lo0');
  });

  it('단위시간 설정이 차트로 전달된다', () => {
    render(
      <MonitorNetworkPanel panelId="n" title="n" config={{ items: ['rxBytes'], unitTime: 'min' }} />,
    );

    expect(screen.getByTestId('network-chart-rxBytes')).toHaveAttribute('data-unit', 'min');
  });

  it('알 수 없는 단위시간은 초로 떨어진다', () => {
    render(
      <MonitorNetworkPanel panelId="n" title="n" config={{ items: ['rxBytes'], unitTime: 'day' }} />,
    );

    expect(screen.getByTestId('network-chart-rxBytes')).toHaveAttribute('data-unit', 'sec');
  });

  it('항목을 모두 끄면 빈 상태 안내를 보여준다', () => {
    render(<MonitorNetworkPanel panelId="n" title="n" config={{ items: [] }} />);

    expect(screen.getByTestId('monitor-network-panel-empty')).toBeInTheDocument();
  });

  it('열 개수 상한이 항목 수까지로 죄어진다', () => {
    render(
      <MonitorNetworkPanel panelId="n" title="n" config={{ items: ['rxBytes'], maxCols: 4 }} />,
    );

    expect(screen.getByTestId('monitor-network-panel-grid')).toHaveAttribute('data-cols', '1');
  });
});
