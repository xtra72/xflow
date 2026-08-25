// 결측 구간 점선 표기의 **패널 통합** 검증 (SPEC-TSDB-004 §2.19).
//
// gapDash.test.ts 는 순수 판정을 검증한다. 여기서는 그 결과가 실제로 Line 으로
// 렌더되는지 — 즉 config → 훅 → 행 구성 → 덧그림 → recharts 까지의 배선을 본다.
// 두 번의 "점선 표시 안 됨" 보고가 모두 판정이 아니라 배선 구간에서 났다.

import type { ReactNode } from 'react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

import type { TsdbSourceConfig } from './chartChannelTypes';
import { gapSeriesKey, GAP_DASHARRAY } from './gapDash';

const tsdbMocks = vi.hoisted(() => ({ queryTsdbSourceMatrix: vi.fn() }));
vi.mock('@/services/api/tsdbSource', async () => {
  const actual =
    await vi.importActual<typeof import('@/services/api/tsdbSource')>(
      '@/services/api/tsdbSource',
    );
  return { ...actual, queryTsdbSourceMatrix: tsdbMocks.queryTsdbSourceMatrix };
});
vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock('recharts', async () => await import('./__mocks__/rechartsStub'));

import LineChartPanel from './LineChartPanel';

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { staleTime: Infinity, retry: false } },
  });
  client.setQueryData(['agents', undefined], {
    data: [{ id: 'a-1', name: 'ix', type: 'influxdb' }],
    total: 1,
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

const INTERVAL = 60_000;

const tsdbSource: TsdbSourceConfig = {
  backend: 'influxdb',
  agent_id: 'a-1',
  agent_name: 'ix',
  series: [{ key: 'cpu', field: 'usage' }],
  time_window_ms: 3_600_000,
  interval_ms: INTERVAL,
  aggregation: 'average',
  refresh_interval_ms: 5_000,
};

/** 0분·1분·8분 — 2분~7분(6개)이 통째로 빠진 데이터(`채우지 않음` 의 모습). */
function sparseMatrix() {
  return {
    matrix: {
      columns: ['cpu.usage'],
      rows: [
        { bucketStartMs: 0, values: [10] },
        { bucketStartMs: INTERVAL, values: [12] },
        { bucketStartMs: INTERVAL * 8, values: [80] },
      ],
      columnOrigins: [0],
      columnLabels: [{ __field__: 'usage' }],
    },
    failures: [],
  };
}

async function renderPanel(config: Record<string, unknown>): Promise<void> {
  render(<LineChartPanel panelId="p1" config={config} />, { wrapper });
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

/** 렌더된 Line 들을 dataKey → dasharray 로 모은다. */
function lines(): Map<string, string> {
  const out = new Map<string, string>();
  for (const el of screen.queryAllByTestId('rc-line')) {
    out.set(el.getAttribute('data-line-key') ?? '', el.getAttribute('data-line-dash') ?? '');
  }
  return out;
}

beforeEach(() => {
  tsdbMocks.queryTsdbSourceMatrix.mockReset();
  tsdbMocks.queryTsdbSourceMatrix.mockResolvedValue(sparseMatrix());
});

describe('LineChartPanel — 결측 구간 점선', () => {
  it('임계를 켜면 결측 구간 덧그림 라인이 렌더된다', async () => {
    await renderPanel({
      data_source: 'tsdb',
      tsdb_source: tsdbSource,
      gap_dash_threshold: 2,
    });
    const rendered = lines();
    const seriesKey = [...rendered.keys()].find((k) => !k.startsWith('__gap__'));
    expect(seriesKey).toBeTruthy();
    // 덧그림이 실제로 존재하고 점선 패턴을 갖는다.
    const gk = gapSeriesKey(seriesKey!);
    expect(rendered.has(gk)).toBe(true);
    expect(rendered.get(gk)).toBe(GAP_DASHARRAY);
  });

  it('꺼져 있으면 덧그림이 없다', async () => {
    await renderPanel({ data_source: 'tsdb', tsdb_source: tsdbSource });
    expect([...lines().keys()].some((k) => k.startsWith('__gap__'))).toBe(false);
  });

  it('임계보다 짧은 결측에는 덧그림이 없다', async () => {
    await renderPanel({
      data_source: 'tsdb',
      tsdb_source: tsdbSource,
      // 6개 빠졌는데 임계가 7이다.
      gap_dash_threshold: 7,
    });
    expect([...lines().keys()].some((k) => k.startsWith('__gap__'))).toBe(false);
  });
});
