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
import { gapDotSeriesKey, gapSeriesKey, GAP_DASHARRAY } from './gapDash';

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

  it('실선과 점선이 만나는 자리에 점을 찍는다', async () => {
    await renderPanel({
      data_source: 'tsdb',
      tsdb_source: tsdbSource,
      gap_dash_threshold: 2,
    });
    const rendered = lines();
    const seriesKey = [...rendered.keys()].find(
      (k) => !k.startsWith('__gap__') && !k.startsWith('__gapdot__'),
    )!;
    expect(rendered.has(gapDotSeriesKey(seriesKey))).toBe(true);
  });

  it('꺼져 있으면 덧그림이 없다', async () => {
    await renderPanel({ data_source: 'tsdb', tsdb_source: tsdbSource });
    expect([...lines().keys()].some((k) => k.startsWith('__gap'))).toBe(false);
  });

  it('임계보다 짧은 결측에는 덧그림이 없다', async () => {
    await renderPanel({
      data_source: 'tsdb',
      tsdb_source: tsdbSource,
      // 6개 빠졌는데 임계가 7이다.
      gap_dash_threshold: 7,
    });
    expect([...lines().keys()].some((k) => k.startsWith('__gap'))).toBe(false);
  });
});

// 점선이 **뜨지 않는** 두 모양 — 둘 다 의도된 동작이라 회귀 테스트로 못박아 둔다.
//
// "값이 없는 구간이 많은데 점선이 안 보인다" 는 보고가 이 두 모양에서 나온다.
// 배선이 끊긴 것과 구분되지 않으면 매번 처음부터 다시 조사하게 된다.
describe('LineChartPanel — 점선이 뜨지 않는 모양', () => {
  /** i번째 버킷에 값이 있는지를 판정 함수로 받아 매트릭스를 만든다. */
  function matrixWhere(hasValue: (i: number) => boolean, count = 30) {
    const rows = [];
    for (let i = 0; i <= count; i++) {
      rows.push({
        bucketStartMs: i * INTERVAL,
        values: [hasValue(i) ? 20 + i * 0.01 : null],
      });
    }
    return {
      matrix: {
        columns: ['cpu.usage'],
        rows,
        columnOrigins: [0],
        columnLabels: [{ __field__: 'usage' }],
      },
      failures: [],
    };
  }

  async function gapKeysFor(
    hasValue: (i: number) => boolean,
    threshold: number,
  ): Promise<string[]> {
    tsdbMocks.queryTsdbSourceMatrix.mockResolvedValue(matrixWhere(hasValue));
    await renderPanel({
      data_source: 'tsdb',
      tsdb_source: tsdbSource,
      gap_dash_threshold: threshold,
    });
    return [...lines().keys()].filter((k) => k.startsWith('__gap__'));
  }

  it('연속 결측이 임계보다 짧으면 뜨지 않는다 — 한 칸씩 거르는 데이터가 그렇다', async () => {
    // 한 칸 걸러 비면 연속 결측은 항상 1개다. 기본 임계(2)로는 하나도 걸리지 않는다.
    expect(await gapKeysFor((i) => i % 2 === 0, 2)).toHaveLength(0);
  });

  it('같은 데이터라도 임계를 1로 낮추면 뜬다', async () => {
    expect((await gapKeysFor((i) => i % 2 === 0, 1)).length).toBeGreaterThan(0);
  });

  it('빈 구간이 창의 맨 앞이면 뜨지 않는다 — 이을 상대가 없다', async () => {
    // 15번부터 끝까지 연속으로 값이 있다 → 결측은 앞쪽에만 있다.
    expect(await gapKeysFor((i) => i >= 15, 2)).toHaveLength(0);
  });

  it('빈 구간이 창의 맨 뒤여도 뜨지 않는다', async () => {
    expect(await gapKeysFor((i) => i <= 15, 2)).toHaveLength(0);
  });

  it('사이가 통째로 비면 뜬다 — 위 두 경우와의 대조군', async () => {
    expect((await gapKeysFor((i) => i === 0 || i === 30, 2)).length).toBeGreaterThan(0);
  });
});
