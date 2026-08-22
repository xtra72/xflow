// LineChartPanel 의 TSDB 상태 표시 — 백엔드 불일치 오버레이 (AC-54).
//
// 기존 `LineChartPanel.test.tsx` 는 `useStoreChartData` 를 모킹해 데이터 계층을 통째로
// 걷어낸다. 여기서는 반대로 **실제 훅 경로**를 태워야 한다 — "질의가 나가지 않는다" 는
// 단언이 어댑터까지 도달해야 의미가 있기 때문이다. 그래서 별도 파일로 둔다.
//
// @spec SPEC-TSDB-002 §2.14 (S2) · §2.18 (U11) · UB2-7

import type { ReactNode } from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import type { TsdbSourceConfig } from './chartChannelTypes';

// 어댑터의 매트릭스 조회만 스파이로 갈아끼운다. 백엔드 파생(`resolveTsdbBackend`)과
// 불일치 오류 타입은 실제 구현을 그대로 쓴다 — 그것이 이 테스트의 검증 대상이다.
const tsdbMocks = vi.hoisted(() => ({ queryTsdbSourceMatrix: vi.fn() }));
vi.mock('@/services/api/tsdbSource', async () => {
  const actual =
    await vi.importActual<typeof import('@/services/api/tsdbSource')>(
      '@/services/api/tsdbSource',
    );
  return { ...actual, queryTsdbSourceMatrix: tsdbMocks.queryTsdbSourceMatrix };
});

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

vi.mock('recharts', async () => await import('./__mocks__/rechartsStub'));

import LineChartPanel from './LineChartPanel';

/** 지정한 에이전트 타입을 심은 QueryClient 래퍼를 만든다. */
function makeWrapper(agentType: string) {
  return function Wrapper({ children }: { children: ReactNode }) {
    const client = new QueryClient({
      defaultOptions: { queries: { staleTime: Infinity, retry: false } },
    });
    client.setQueryData(['agents', undefined], {
      data: [{ id: 'a-1', name: 'ix', type: agentType }],
      total: 1,
    });
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

/** 참조된 에이전트가 store 타입인 목록. TSDB 소스가 지원하지 않는 백엔드다. */
function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { staleTime: Infinity, retry: false } },
  });
  client.setQueryData(['agents', undefined], {
    data: [{ id: 'a-1', name: 'ix', type: 'store' }],
    total: 1,
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

const tsdbSource: TsdbSourceConfig = {
  backend: 'influxdb',
  agent_id: 'a-1',
  agent_name: 'ix',
  series: [{ key: 'cpu', field: 'usage' }],
  time_window_ms: 60_000,
  interval_ms: 10_000,
  aggregation: 'average',
  refresh_interval_ms: 5_000,
};

beforeEach(() => {
  tsdbMocks.queryTsdbSourceMatrix.mockReset();
});

describe('LineChartPanel — TSDB 백엔드 불일치 (AC-54)', () => {
  it('미지원 백엔드 에이전트 참조 시 불일치 오류를 표시하고 질의하지 않는다', async () => {
    render(
      <LineChartPanel
        panelId="p1"
        config={{ data_source: 'tsdb', tsdb_source: tsdbSource }}
      />,
      { wrapper },
    );
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    // 오버레이 문구가 일반 조회 실패가 아니라 **불일치 전용 문구**다 — 사용자가 고칠
    // 대상(에이전트 선택)을 알 수 있어야 한다.
    const overlay = screen.getByTestId('line-chart-overlay');
    expect(overlay).toHaveTextContent('dashboard.chart.dataSourceTsdbBackendMismatch');

    // 조용히 Store 로 질의하지 않는다(§2.18 · UB2-7) — 질의 자체가 나가지 않는다.
    expect(tsdbMocks.queryTsdbSourceMatrix).not.toHaveBeenCalled();
  });
});

describe('LineChartPanel — TSDB 상태 표시 (§2.14 [S2])', () => {
  it('부분 실패는 오버레이가 아니라 실패 개수 배지로 표시한다', async () => {
    tsdbMocks.queryTsdbSourceMatrix.mockResolvedValue({
      matrix: {
        columns: ['cpu'],
        rows: [
          { bucketStartMs: 1000, values: [21.5] },
          { bucketStartMs: 2000, values: [22] },
        ],
      },
      failures: [{ index: 1, error: new Error('boom') }],
    });
    render(
      <LineChartPanel
        panelId="p1"
        config={{ data_source: 'tsdb', tsdb_source: tsdbSource }}
      />,
      { wrapper: makeWrapper('influxdb') },
    );
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });

    const badge = screen.getByTestId('panel-series-partial-badge');
    expect(badge).toHaveTextContent('dashboard.chart.seriesPartialFailure');
    // 성공 시리즈는 계속 보여야 하므로 오류 오버레이는 뜨지 않는다(§2.14).
    expect(screen.queryByTestId('line-chart-overlay')).not.toBeInTheDocument();
  });

  it('빈 선택은 오류가 아니라 설정 유도 안내로 표시한다', async () => {
    render(
      <LineChartPanel
        panelId="p1"
        config={{
          data_source: 'tsdb',
          tsdb_source: { ...tsdbSource, series: [] },
        }}
      />,
      { wrapper: makeWrapper('influxdb') },
    );
    await act(async () => {
      await Promise.resolve();
    });

    expect(screen.getByTestId('panel-series-empty-selection')).toBeInTheDocument();
    expect(screen.queryByTestId('line-chart-overlay')).not.toBeInTheDocument();
    expect(tsdbMocks.queryTsdbSourceMatrix).not.toHaveBeenCalled();
  });
});
