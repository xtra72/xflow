// sysmetrics 패널 3종 테스트.
//
// 검증 대상은 화면 분기다 — 항목 선택, 종합/개별 전환, 수집 꺼짐 표시, 에이전트
// 부재 폴백. rate 계산 자체는 sysMetricsSeries.test.ts 가 이미 덮으므로 여기서는
// "그 결과가 어느 칸에 그려지는가"만 본다.

import { describe, expect, it, beforeEach, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (key: string) => key, locale: 'ko', setLocale: () => {} }),
}));

// recharts 실렌더는 불필요하다 — 계열 구성만 속성으로 드러낸다.
vi.mock('@/pages/monitoring/MultiSeriesChart', () => ({
  default: ({
    title,
    series,
    style,
    height,
    legend,
    smooth,
    stacked,
    fillParent,
  }: {
    title: string;
    series: { name: string }[];
    style?: string;
    height?: number;
    legend?: string;
    smooth?: boolean;
    stacked?: boolean;
    fillParent?: boolean;
  }) => (
    <div
      data-testid={`chart-${title}`}
      data-series={series.map((s) => s.name).join(',')}
      data-chart-style={style ?? ''}
      data-height={height === undefined ? '' : String(height)}
      data-legend={legend ?? ''}
      data-smooth={String(!!smooth)}
      data-stacked={String(!!stacked)}
      data-fill={String(!!fillParent)}
    />
  ),
}));

/** useQuery 가 돌려줄 응답 (테스트가 갈아 끼운다) */
const queryRef = vi.hoisted(() => ({
  current: { data: undefined as unknown, isLoading: false, isError: false },
}));

// 훅은 Provider 부재를 견디려고 `QueryClientContext` / `QueryClient` 를 쓴다
// (inertQueryClient 패턴). 원본을 펼치고 `useQuery` 만 덮어 그 export 들을 살려 둔다.
vi.mock('@tanstack/react-query', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-query')>()),
  useQuery: () => queryRef.current,
}));

vi.mock('@/services/api/agentService', () => ({
  getAgent: vi.fn(),
}));

import SysMetricsSystemPanel from './SysMetricsSystemPanel';
import SysMetricsNetworkPanel from './SysMetricsNetworkPanel';
import SysMetricsStoragePanel from './SysMetricsStoragePanel';

/** 정상 스냅샷을 담은 에이전트 응답 */
function agent(stateOverrides: Record<string, unknown> = {}) {
  return {
    data: {
      state: {
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
        disk_io: { disk0: { read_bytes: 100, write_bytes: 200 } },
        network: {
          en0: { bytes_recv: 1_000, bytes_sent: 500, packets_recv: 10, packets_sent: 5 },
          en1: { bytes_recv: 200, bytes_sent: 100, packets_recv: 2, packets_sent: 1 },
        },
        targets: {
          mountpoints: ['/', '/data'],
          devices: ['disk0'],
          interfaces: ['en0', 'en1'],
        },
        ...stateOverrides,
      },
    },
    isLoading: false,
    isError: false,
  };
}

/** 패널 공통 props */
const base = { panelId: 'p1', title: '제목' };
/** 에이전트에 바인딩된 config */
const bound = { agent_id: 'a1', agent_name: 'sys' };

describe('SysMetricsSystemPanel', () => {
  beforeEach(() => {
    queryRef.current = agent();
  });

  it('기본 네 값을 그린다', () => {
    // 값 단위 항목의 기본은 옛 네 타일이 보여 주던 대표 값들이다.
    render(<SysMetricsSystemPanel {...base} config={bound} />);

    expect(screen.getByTestId('sysmetrics-field-cpu.usage_percent')).toBeInTheDocument();
    expect(screen.getByTestId('sysmetrics-field-memory.usage_percent')).toBeInTheDocument();
    expect(screen.getByTestId('sysmetrics-field-disk_io.read_bytes')).toBeInTheDocument();
    expect(screen.getByTestId('sysmetrics-field-network.bytes_recv')).toBeInTheDocument();
  });

  it('items 로 고른 항목만 그린다', () => {
    render(<SysMetricsSystemPanel {...base} config={{ ...bound, items: ['cpu.usage_percent'] }} />);

    expect(screen.getByTestId('sysmetrics-field-cpu.usage_percent')).toBeInTheDocument();
    expect(screen.queryByTestId('sysmetrics-field-memory.usage_percent')).not.toBeInTheDocument();
  });

  it('빈 items 는 "모두 껐다"로 존중한다 (기본값으로 되돌리지 않는다)', () => {
    render(<SysMetricsSystemPanel {...base} config={{ ...bound, items: [] }} />);

    expect(screen.getByTestId('sysmetrics-system-empty')).toBeInTheDocument();
    expect(screen.queryByTestId('sysmetrics-field-cpu.usage_percent')).not.toBeInTheDocument();
  });

  it('CPU 사용률을 표시한다', () => {
    render(<SysMetricsSystemPanel {...base} config={{ ...bound, items: ['cpu.usage_percent'] }} />);

    expect(screen.getByTestId('sysmetrics-field-cpu.usage_percent')).toHaveTextContent('42.5%');
  });

  it('메모리 사용량만 따로 고를 수 있다', () => {
    // 값 단위 항목의 요점 — 그룹으로 묶여 있으면 불가능했다.
    render(<SysMetricsSystemPanel {...base} config={{ ...bound, items: ['memory.used_bytes'] }} />);

    expect(screen.getByTestId('sysmetrics-field-memory.used_bytes')).toBeInTheDocument();
    expect(screen.queryByTestId('sysmetrics-field-memory.usage_percent')).not.toBeInTheDocument();
  });

  it('옛 그룹 키를 담은 설정도 그대로 열린다', () => {
    // 저장된 대시보드를 빈 화면으로 만들지 않는다.
    render(<SysMetricsSystemPanel {...base} config={{ ...bound, items: ['cpu', 'memory'] }} />);

    expect(screen.getByTestId('sysmetrics-field-cpu.usage_percent')).toBeInTheDocument();
    expect(screen.getByTestId('sysmetrics-field-memory.usage_percent')).toBeInTheDocument();
  });

  it('수집이 꺼진 항목은 값 0 이 아니라 "수집 꺼짐"으로 표시한다', () => {
    // 키 부재 = 수집 꺼짐. 0 으로 그리면 "디스크가 비어 있다"와 구별되지 않는다.
    queryRef.current = agent({ disk_io: undefined });
    render(<SysMetricsSystemPanel {...base} config={{ ...bound, items: ['disk_io.read_bytes'] }} />);

    const tile = screen.getByTestId('sysmetrics-field-disk_io.read_bytes');
    expect(tile.dataset.disabled).toBe('true');
    expect(tile).toHaveTextContent('sysmetrics.state.notCollected');
  });

  it('기준점이 없으면 rate 는 그리지 않는다 (0 으로 채우지 않는다)', () => {
    // 첫 표본에는 직전 값이 없다. 0 으로 채우면 트래픽이 없는 것처럼 보인다.
    render(<SysMetricsSystemPanel {...base} config={{ ...bound, items: ['network.bytes_recv'] }} />);

    expect(screen.getByTestId('sysmetrics-field-network.bytes_recv')).toHaveTextContent('-');
  });

  it('누적 모드는 첫 표본부터 원값을 그린다', () => {
    // 증가량과 달리 기준점이 필요 없다 — 원값이 곧 그릴 값이다.
    render(
      <SysMetricsSystemPanel
        {...base}
        config={{
          ...bound,
          items: ['network.bytes_recv'],
          itemOptions: { 'network.bytes_recv': { counterMode: 'total' } },
        }}
      />,
    );

    // en0 1000 + en1 200 = 1200 B (종합). 단위시간(`/s`)이 붙지 않는다.
    const tile = screen.getByTestId('sysmetrics-field-network.bytes_recv');
    expect(tile).not.toHaveTextContent('-');
    expect(tile.textContent).not.toContain('/s');
  });

  it('누적 모드는 항목별로 갈린다 (패널 전체를 바꾸지 않는다)', () => {
    render(
      <SysMetricsSystemPanel
        {...base}
        config={{
          ...bound,
          items: ['network.bytes_recv', 'network.bytes_sent'],
          itemOptions: { 'network.bytes_recv': { counterMode: 'total' } },
        }}
      />,
    );

    // 고른 항목만 누적으로 바뀌고, 나머지는 기준점이 없어 그대로 `-` 다.
    expect(screen.getByTestId('sysmetrics-field-network.bytes_recv')).not.toHaveTextContent('-');
    expect(screen.getByTestId('sysmetrics-field-network.bytes_sent')).toHaveTextContent('-');
  });
});

describe('SysMetricsNetworkPanel', () => {
  beforeEach(() => {
    queryRef.current = agent();
  });

  it('대상을 고르지 않으면 합산 계열 하나만 그린다', () => {
    render(<SysMetricsNetworkPanel {...base} config={bound} />);

    expect(screen.getByTestId('sysmetrics-network-grid').dataset.series).toBe('total');
  });

  it('고른 인터페이스마다 계열을 하나씩 그린다', () => {
    render(
      <SysMetricsNetworkPanel {...base} config={{ ...bound, interfaces: ['en0', 'en1'] }} />,
    );

    expect(screen.getByTestId('sysmetrics-network-grid').dataset.series).toBe('en0,en1');
  });

  it('사라진 인터페이스는 건너뛰고 안내한다', () => {
    // en9 는 스냅샷에 없다. 나머지 선은 계속 그려야 한다.
    render(
      <SysMetricsNetworkPanel {...base} config={{ ...bound, interfaces: ['en0', 'en9'] }} />,
    );

    expect(screen.getByTestId('sysmetrics-network-grid').dataset.series).toBe('en0');
    expect(screen.getByTestId('sysmetrics-network-missing')).toHaveTextContent('en9');
  });

  it('네 채널 차트를 그린다', () => {
    render(<SysMetricsNetworkPanel {...base} config={bound} />);

    expect(screen.getByTestId('chart-sysmetrics.channels.rxBytes')).toBeInTheDocument();
    expect(screen.getByTestId('chart-sysmetrics.channels.txBytes')).toBeInTheDocument();
    expect(screen.getByTestId('chart-sysmetrics.channels.rxPackets')).toBeInTheDocument();
    expect(screen.getByTestId('chart-sysmetrics.channels.txPackets')).toBeInTheDocument();
  });

  it('수집이 꺼져 있으면 차트 대신 안내를 그린다', () => {
    queryRef.current = agent({ network: undefined });
    render(<SysMetricsNetworkPanel {...base} config={bound} />);

    expect(screen.getByTestId('sysmetrics-network-not-collected')).toBeInTheDocument();
    expect(screen.queryByTestId('sysmetrics-network-grid')).not.toBeInTheDocument();
  });
});

describe('SysMetricsStoragePanel', () => {
  beforeEach(() => {
    queryRef.current = agent();
  });

  it('대상을 고르지 않으면 합계 한 줄을 그린다', () => {
    render(<SysMetricsStoragePanel {...base} config={bound} />);

    expect(screen.getByTestId('sysmetrics-storage-rows').dataset.rows).toBe('1');
    // 합계 기준 사용률 — (30+10)/(100+100) = 20%. 각 사용률의 평균(20%)과 우연히
    // 같아지지 않도록 아래 테스트가 비대칭 케이스를 따로 본다.
    expect(screen.getByTestId('sysmetrics-storage-row')).toHaveTextContent('20.0%');
  });

  it('합계 사용률은 용량이 큰 쪽이 지배한다 (평균이 아니다)', () => {
    queryRef.current = agent({
      storage: {
        '/': { total_bytes: 1_000, used_bytes: 900, free_bytes: 100, usage_percent: 90 },
        '/ram': { total_bytes: 10, used_bytes: 1, free_bytes: 9, usage_percent: 10 },
      },
    });
    render(<SysMetricsStoragePanel {...base} config={bound} />);

    // 각 사용률의 평균은 50% 지만 합계 기준은 89.2% 다.
    expect(screen.getByTestId('sysmetrics-storage-row')).toHaveTextContent('89.2%');
  });

  it('고른 마운트마다 행을 그린다', () => {
    render(
      <SysMetricsStoragePanel {...base} config={{ ...bound, mountpoints: ['/', '/data'] }} />,
    );

    const rows = screen.getAllByTestId('sysmetrics-storage-row');
    expect(rows).toHaveLength(2);
    expect(rows.map((r) => r.dataset.name)).toEqual(['/', '/data']);
  });

  it('같은 볼륨의 여러 마운트를 합치지 않는다', () => {
    // macOS 의 / 와 /System/Volumes/Data 처럼 수치가 같은 두 마운트.
    queryRef.current = agent({
      storage: {
        '/': { total_bytes: 100, used_bytes: 30, free_bytes: 70, usage_percent: 30 },
        '/System/Volumes/Data': {
          total_bytes: 100,
          used_bytes: 30,
          free_bytes: 70,
          usage_percent: 30,
        },
      },
    });
    render(
      <SysMetricsStoragePanel
        {...base}
        config={{ ...bound, mountpoints: ['/', '/System/Volumes/Data'] }}
      />,
    );

    // 둘 다 그대로 보인다 — 임의로 합치면 사용자가 고른 대상이 사라진다.
    expect(screen.getAllByTestId('sysmetrics-storage-row')).toHaveLength(2);
  });

  it('items 로 표시 값을 고른다', () => {
    render(
      <SysMetricsStoragePanel {...base} config={{ ...bound, items: ['usage'] }} />,
    );

    // 사용률 막대는 이제 항목 뷰가 그린다(스타일 progress).
    expect(screen.getByTestId('sysmetrics-storage-row').querySelector('[data-style="progress"]'))
      .toBeInTheDocument();
    expect(screen.getByTestId('sysmetrics-storage-row')).not.toHaveTextContent(
      'sysmetrics.storage.used',
    );
  });

  it('수집이 꺼져 있으면 안내를 그린다', () => {
    queryRef.current = agent({ storage: undefined });
    render(<SysMetricsStoragePanel {...base} config={bound} />);

    expect(screen.getByTestId('sysmetrics-storage-not-collected')).toBeInTheDocument();
  });
});

describe('폴백 (세 패널 공통)', () => {
  it('에이전트가 바인딩되지 않으면 안내를 그린다', () => {
    queryRef.current = { data: undefined, isLoading: false, isError: false };
    render(<SysMetricsSystemPanel {...base} config={{}} />);

    expect(screen.getByTestId('sysmetrics-panel-state').dataset.state).toBe('unavailable');
  });

  it('조회에 실패하면 안내를 그린다 (오류를 던지지 않는다)', () => {
    queryRef.current = { data: undefined, isLoading: false, isError: true };
    render(<SysMetricsStoragePanel {...base} config={bound} />);

    expect(screen.getByTestId('sysmetrics-panel-state').dataset.state).toBe('unavailable');
  });

  it('표본 이전에는 대기 안내를 그린다', () => {
    queryRef.current = {
      data: { state: { status: 'no_sample', interval_seconds: 5 } },
      isLoading: false,
      isError: false,
    };
    render(<SysMetricsNetworkPanel {...base} config={bound} />);

    expect(screen.getByTestId('sysmetrics-panel-state').dataset.state).toBe('no_sample');
  });

  it('멈춘 에이전트는 마지막 값을 지우지 않고 배너만 얹는다', () => {
    queryRef.current = agent({ status: 'stopped' });
    render(<SysMetricsSystemPanel {...base} config={{ ...bound, items: ['cpu.usage_percent'] }} />);

    expect(screen.getByTestId('sysmetrics-panel-stopped')).toBeInTheDocument();
    // 값이 지워지면 "멈췄다"와 "0 이 되었다"가 구별되지 않는다.
    expect(screen.getByTestId('sysmetrics-field-cpu.usage_percent')).toHaveTextContent('42.5%');
  });
});

describe('항목별 스타일 (SPEC-SYSMETRICS-PANEL-001 후속)', () => {
  beforeEach(() => {
    queryRef.current = agent();
  });

  it('패널 기본 스타일을 항목이 따라간다', () => {
    render(
      <SysMetricsSystemPanel {...base} config={{ ...bound, items: ['cpu'], style: 'gauge' }} />,
    );

    expect(screen.getByTestId('sysmetrics-field-cpu.usage_percent').dataset.style).toBe('gauge');
  });

  it('항목 덮어쓰기가 패널 기본을 이긴다', () => {
    render(
      <SysMetricsSystemPanel
        {...base}
        config={{
          ...bound,
          items: ['cpu.usage_percent', 'memory.usage_percent'],
          style: 'gauge',
          itemOptions: { 'memory.usage_percent': { style: 'progress' } },
        }}
      />,
    );

    expect(screen.getByTestId('sysmetrics-field-cpu.usage_percent').dataset.style).toBe('gauge');
    expect(screen.getByTestId('sysmetrics-field-memory.usage_percent').dataset.style).toBe('progress');
  });

  it('증가량 항목에 게이지를 지정해도 타일로 떨어진다', () => {
    // 상한이 없는 값에 게이지를 그리면 바늘이 무엇을 가리키는지 말할 수 없다.
    render(
      <SysMetricsSystemPanel
        {...base}
        config={{ ...bound, items: ['network'], itemOptions: { 'network.bytes_recv': { style: 'gauge' } } }}
      />,
    );

    // 타일 폴백에는 data-style 이 없다(게이지·진행막대만 붙인다).
    expect(screen.getByTestId('sysmetrics-field-network.bytes_recv').dataset.style).toBeUndefined();
  });

  it('높이와 범례 위치가 차트로 전달된다', () => {
    render(
      <SysMetricsNetworkPanel
        {...base}
        config={{ ...bound, style: 'area', height: 320, legend: 'right' }}
      />,
    );

    const chart = screen.getByTestId('chart-sysmetrics.channels.rxBytes');
    expect(chart.dataset.chartStyle).toBe('area');
    expect(chart.dataset.height).toBe('320');
    expect(chart.dataset.legend).toBe('right');
  });

  it('채널마다 스타일을 따로 고를 수 있다', () => {
    render(
      <SysMetricsNetworkPanel
        {...base}
        config={{ ...bound, itemOptions: { bytes_sent: { style: 'bar' } } }}
      />,
    );

    expect(screen.getByTestId('chart-sysmetrics.channels.rxBytes').dataset.chartStyle).toBe('line');
    expect(screen.getByTestId('chart-sysmetrics.channels.txBytes').dataset.chartStyle).toBe('bar');
  });

  it('스토리지는 마운트마다 스타일을 고를 수 있다', () => {
    render(
      <SysMetricsStoragePanel
        {...base}
        config={{
          ...bound,
          mountpoints: ['/', '/data'],
          itemOptions: { '/data': { style: 'gauge' } },
        }}
      />,
    );

    const rows = screen.getAllByTestId('sysmetrics-storage-row');
    expect(rows[0]!.querySelector('[data-style="progress"]')).toBeInTheDocument();
    expect(rows[1]!.querySelector('[data-style="gauge"]')).toBeInTheDocument();
  });

  it('열 수 설정이 그리드에 반영된다', () => {
    render(<SysMetricsSystemPanel {...base} config={{ ...bound, maxCols: 2 }} />);

    // 실제 열 수는 폭에 따라 더 줄 수 있으나 상한을 넘지는 않는다.
    const cols = Number(screen.getByTestId('sysmetrics-system-grid').dataset.cols);
    expect(cols).toBeLessThanOrEqual(2);
  });
});

describe('다중 대상 표시', () => {
  beforeEach(() => {
    queryRef.current = agent();
  });

  it('타일 스타일에서 고른 대상을 모두 보여준다', () => {
    // 첫 대상만 그리면 사용자가 고른 나머지가 조용히 사라진다.
    render(
      <SysMetricsNetworkPanel
        {...base}
        config={{ ...bound, interfaces: ['en0', 'en1'], style: 'tile' }}
      />,
    );

    const tiles = screen.getAllByTestId('sysmetrics-tile-rows');
    expect(tiles.length).toBeGreaterThan(0);
    expect(tiles[0]!.dataset.rows).toBe('2');
    expect(tiles[0]!).toHaveTextContent('en0');
    expect(tiles[0]!).toHaveTextContent('en1');
  });

  it('대상이 하나면 큰 숫자 하나로 보여준다', () => {
    render(
      <SysMetricsNetworkPanel {...base} config={{ ...bound, interfaces: ['en0'], style: 'tile' }} />,
    );

    expect(screen.queryByTestId('sysmetrics-tile-rows')).not.toBeInTheDocument();
  });

  it('차트 스타일에서는 계열이 대상 수만큼 간다', () => {
    render(
      <SysMetricsNetworkPanel
        {...base}
        config={{ ...bound, interfaces: ['en0', 'en1'], style: 'line' }}
      />,
    );

    expect(screen.getByTestId('chart-sysmetrics.channels.rxBytes').dataset.series).toBe('en0,en1');
  });

  it('스토리지는 고른 마운트마다 행을 만든다', () => {
    render(
      <SysMetricsStoragePanel
        {...base}
        config={{ ...bound, mountpoints: ['/', '/data'] }}
      />,
    );

    expect(screen.getByTestId('sysmetrics-storage-rows').dataset.rows).toBe('2');
  });
});

describe('패널 유형별 기본 스타일이 화면에 반영된다', () => {
  beforeEach(() => {
    queryRef.current = agent();
  });

  it('스토리지는 설정 없이도 진행 막대다', () => {
    render(<SysMetricsStoragePanel {...base} config={bound} />);

    expect(
      screen.getByTestId('sysmetrics-storage-row').querySelector('[data-style="progress"]'),
    ).toBeInTheDocument();
  });

  it('네트워크는 설정 없이도 라인이다', () => {
    render(<SysMetricsNetworkPanel {...base} config={bound} />);

    expect(screen.getByTestId('chart-sysmetrics.channels.rxBytes').dataset.chartStyle).toBe('line');
  });

  it('시스템은 설정 없이도 타일이다', () => {
    render(<SysMetricsSystemPanel {...base} config={{ ...bound, items: ['cpu.usage_percent'] }} />);

    // 타일에는 게이지·진행막대가 붙이는 data-style 이 없다.
    expect(screen.getByTestId('sysmetrics-field-cpu.usage_percent').dataset.style).toBeUndefined();
  });
});

describe('게이지·진행 막대가 실제로 그려진다', () => {
  beforeEach(() => {
    queryRef.current = agent();
  });

  it('시스템 패널의 CPU 를 게이지로 그린다', () => {
    render(
      <SysMetricsSystemPanel {...base} config={{ ...bound, items: ['cpu'], style: 'gauge' }} />,
    );

    const tile = screen.getByTestId('sysmetrics-field-cpu.usage_percent');
    expect(tile.dataset.style).toBe('gauge');
    expect(tile.dataset.percent).toBe('42.5');
  });

  it('시스템 패널의 메모리를 진행 막대로 그린다', () => {
    render(
      <SysMetricsSystemPanel
        {...base}
        config={{ ...bound, items: ['memory'], style: 'progress' }}
      />,
    );

    expect(screen.getByTestId('sysmetrics-field-memory.usage_percent').dataset.style).toBe('progress');
  });

  it('스토리지를 게이지로 그린다', () => {
    render(<SysMetricsStoragePanel {...base} config={{ ...bound, style: 'gauge' }} />);

    expect(
      screen.getByTestId('sysmetrics-storage-row').querySelector('[data-style="gauge"]'),
    ).toBeInTheDocument();
  });

  it('"사용률" 항목을 꺼도 게이지는 그려진다', () => {
    // 게이지는 그 자체가 사용률 표시다. 항목 토글로 값을 끊으면 타일로 떨어져
    // 스타일을 골라도 아무 일이 없다.
    render(
      <SysMetricsStoragePanel
        {...base}
        config={{ ...bound, style: 'gauge', items: ['used', 'free'] }}
      />,
    );

    expect(
      screen.getByTestId('sysmetrics-storage-row').querySelector('[data-style="gauge"]'),
    ).toBeInTheDocument();
  });

  it('증가량 항목에 게이지를 걸면 타일로 떨어진다 (규약 유지)', () => {
    render(
      <SysMetricsSystemPanel
        {...base}
        config={{ ...bound, items: ['network'], style: 'gauge' }}
      />,
    );

    expect(screen.getByTestId('sysmetrics-field-network.bytes_recv').dataset.style).toBeUndefined();
  });
});

describe('게이지·차트가 기존 패널과 같은 것을 쓴다', () => {
  beforeEach(() => {
    queryRef.current = agent();
  });

  it('게이지 모양을 고른 대로 그린다', () => {
    render(
      <SysMetricsSystemPanel
        {...base}
        config={{ ...bound, items: ['cpu'], style: 'gauge', gaugeType: 'needle' }}
      />,
    );

    const tile = screen.getByTestId('sysmetrics-field-cpu.usage_percent');
    expect(tile.dataset.gaugeType).toBe('needle');
    // 게이지 패널과 같은 렌더러를 쓰므로 SVG 가 실제로 그려진다.
    expect(tile.querySelector('svg')).toBeInTheDocument();
  });

  it('항목마다 게이지 모양을 달리 고를 수 있다', () => {
    render(
      <SysMetricsSystemPanel
        {...base}
        config={{
          ...bound,
          items: ['cpu.usage_percent', 'memory.usage_percent'],
          style: 'gauge',
          gaugeType: 'simple',
          itemOptions: { 'memory.usage_percent': { gaugeType: 'half' } },
        }}
      />,
    );

    expect(screen.getByTestId('sysmetrics-field-cpu.usage_percent').dataset.gaugeType).toBe('simple');
    expect(screen.getByTestId('sysmetrics-field-memory.usage_percent').dataset.gaugeType).toBe('half');
  });

  it('곡선·누적이 차트로 전달된다', () => {
    render(
      <SysMetricsNetworkPanel
        {...base}
        config={{ ...bound, style: 'area', smooth: true, stacked: true }}
      />,
    );

    const chart = screen.getByTestId('chart-sysmetrics.channels.rxBytes');
    expect(chart.dataset.smooth).toBe('true');
    expect(chart.dataset.stacked).toBe('true');
  });
});

describe('패널을 채운다', () => {
  beforeEach(() => {
    queryRef.current = agent();
  });

  it('격자 행이 남은 높이를 나눠 갖는다', () => {
    // auto-rows-min 이면 내용 높이만 차지해 패널을 늘려도 아래가 빈 채로 남는다.
    render(<SysMetricsSystemPanel {...base} config={bound} />);

    const grid = screen.getByTestId('sysmetrics-system-grid');
    expect(grid.style.gridAutoRows).toContain('1fr');
    expect(grid.className).not.toContain('auto-rows-min');
  });

  it('세 패널 모두 채움 행을 쓴다', () => {
    const { unmount } = render(<SysMetricsNetworkPanel {...base} config={bound} />);
    expect(screen.getByTestId('sysmetrics-network-grid').style.gridAutoRows).toContain('1fr');
    unmount();

    render(<SysMetricsStoragePanel {...base} config={bound} />);
    expect(screen.getByTestId('sysmetrics-storage-rows').style.gridAutoRows).toContain('1fr');
  });

  it('높이를 지정하지 않으면 차트가 부모를 채운다', () => {
    render(<SysMetricsNetworkPanel {...base} config={bound} />);

    expect(screen.getByTestId('chart-sysmetrics.channels.rxBytes').dataset.fill).toBe('true');
  });

  it('높이를 지정하면 그 높이로 고정된다', () => {
    render(<SysMetricsNetworkPanel {...base} config={{ ...bound, height: 240 }} />);

    const chart = screen.getByTestId('chart-sysmetrics.channels.rxBytes');
    expect(chart.dataset.fill).toBe('false');
    expect(chart.dataset.height).toBe('240');
  });
});

describe('타일 정렬·글자·색', () => {
  beforeEach(() => {
    queryRef.current = agent();
  });

  const cpu = { ...bound, items: ['cpu.usage_percent'] };

  it('기본은 왼쪽 정렬이다', () => {
    render(<SysMetricsSystemPanel {...base} config={cpu} />);

    expect(screen.getByTestId('sysmetrics-field-cpu.usage_percent').className)
      .toContain('text-left');
  });

  it('정렬을 가운데로 바꿀 수 있다', () => {
    render(<SysMetricsSystemPanel {...base} config={{ ...cpu, align: 'center' }} />);

    const tile = screen.getByTestId('sysmetrics-field-cpu.usage_percent');
    expect(tile.className).toContain('text-center');
    expect(tile.className).not.toContain('text-left');
  });

  it('값 글자 크기를 바꿀 수 있다', () => {
    render(<SysMetricsSystemPanel {...base} config={{ ...cpu, valueSize: '2xl' }} />);

    expect(screen.getByTestId('sysmetrics-field-cpu.usage_percent-value').className)
      .toContain('text-3xl');
  });

  it('라벨 굵기를 바꿀 수 있다', () => {
    render(<SysMetricsSystemPanel {...base} config={{ ...cpu, labelWeight: 'bold' }} />);

    const tile = screen.getByTestId('sysmetrics-field-cpu.usage_percent');
    expect(tile.querySelector('p')?.className).toContain('font-bold');
  });

  it('값 색을 지정할 수 있다', () => {
    render(<SysMetricsSystemPanel {...base} config={{ ...cpu, valueColor: '#ff0000' }} />);

    expect(screen.getByTestId('sysmetrics-field-cpu.usage_percent-value'))
      .toHaveStyle({ color: '#ff0000' });
  });

  it('값이 범위에 들면 그 색으로 그린다', () => {
    // CPU 42.5% — 아래 범위(40~80)에 든다.
    render(
      <SysMetricsSystemPanel
        {...base}
        config={{
          ...cpu,
          valueColor: '#0000ff',
          thresholds: [{ name: '경고', color: '#f59e0b', from: 40, to: 80 }],
        }}
      />,
    );

    expect(screen.getByTestId('sysmetrics-field-cpu.usage_percent-value'))
      .toHaveStyle({ color: '#f59e0b' });
  });

  it('항목마다 정렬을 달리할 수 있다', () => {
    render(
      <SysMetricsSystemPanel
        {...base}
        config={{
          ...bound,
          items: ['cpu.usage_percent', 'memory.usage_percent'],
          align: 'left',
          itemOptions: { 'memory.usage_percent': { align: 'right' } },
        }}
      />,
    );

    expect(screen.getByTestId('sysmetrics-field-cpu.usage_percent').className).toContain('text-left');
    expect(screen.getByTestId('sysmetrics-field-memory.usage_percent').className).toContain('text-right');
  });
});
