// 데이터 소스 설정 **순서** 테스트 — Store · TSDB · 시스템 지표가 같은 차례로 늘어선다.
//
// 소스를 갈아탄 사용자가 같은 설정을 같은 자리에서 찾게 하려는 것이다. 순서가 갈라지면
// 소스 수만큼 화면을 다시 익혀야 한다. 주석만으로는 다시 어긋나므로 렌더 순서를 잠근다.
//
// 정본 순서(`ChartPanelSections.StoreSourceSection` 머리말):
//
//   1. 에이전트 선택
//   2. 소스 고유 축      TSDB: bucket · 드릴다운 / 시스템 지표: 이력 없음 고지
//   3. 조회 창           범위 → 인터벌 → 집계 → 빈 구간 처리 (없는 축은 건너뛴다)
//   4. 구간 대표값       해당 패널에서만
//   5. 시리즈 이름 형식
//   6. 시리즈 표
//   7. 선택 요약

import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({
    data: {
      data: [
        { id: 'st', name: 'store-1', type: 'store' },
        { id: 'ix', name: 'influx-1', type: 'influxdb', config: { version: '2' } },
        { id: 'sy', name: 'host-1', type: 'sysmetrics' },
      ],
    },
  }),
  useAgent: () => ({
    data: {
      state: {
        status: 'running',
        collected_at: 1_000,
        interval_seconds: 5,
        network: { en0: { bytes_recv: 1 } },
        targets: { mountpoints: ['/'], devices: ['disk0'], interfaces: ['en0'] },
      },
    },
  }),
}));

vi.mock('@/services/api/charts', () => ({
  listChartChannels: () => Promise.resolve([]),
}));
vi.mock('@/services/api/store', () => ({
  useStoreKeysWithTags: () => ({ data: [], isLoading: false }),
  useStoreTagPairs: () => ({ data: [], isLoading: false }),
}));
// TSDB 디스커버리는 네트워크를 타므로 빈 결과로 둔다 — 순서만 보면 된다.
vi.mock('@/services/api/influxdbManagement', () => ({
  fetchInfluxBuckets: () => Promise.resolve([]),
  fetchInfluxMeasurements: () => Promise.resolve([]),
  fetchInfluxFieldKeys: () => Promise.resolve([]),
  fetchInfluxTagKeys: () => Promise.resolve([]),
  fetchInfluxTagValues: () => Promise.resolve([]),
}));

import type { PanelConfig } from '@/stores/uiStore';
import { StoreSourceSection } from './ChartPanelSections';

/** 문서 순서상 a 가 b 보다 앞에 있는가. */
function precedes(a: Element, b: Element): boolean {
  // DOCUMENT_POSITION_FOLLOWING(4) = b 가 a 뒤에 온다.
  return (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;
}

/** 렌더된 순서대로 testid 를 훑어 존재하는 것만 남긴다. */
function orderOf(ids: string[]): string[] {
  const found = ids
    .map((id) => ({ id, el: screen.queryByTestId(id) }))
    .filter((x): x is { id: string; el: HTMLElement } => x.el !== null);
  return found
    .slice()
    .sort((x, y) => (precedes(x.el, y.el) ? -1 : 1))
    .map((x) => x.id);
}

function renderSection(config: Record<string, unknown>) {
  const panel = { id: 'p1', type: 'graph-chart', title: 't', config } as PanelConfig;
  render(<StoreSourceSection panel={panel} onConfigChange={() => {}} />);
}

describe('데이터 소스 설정 순서 — 세 소스가 같은 차례다', () => {
  it('Store: 범위 → 인터벌 → 빈 구간 → 이름 형식', () => {
    renderSection({
      data_source: 'store',
      store_source: {
        agent_id: 'st',
        agent_name: 'store-1',
        series: [{ key: 'k1' }],
        time_window_ms: 60_000,
        interval_ms: 10_000,
      },
    });

    expect(
      orderOf([
        'chart-store-agent-select',
        'chart-store-range-window',
        'chart-store-interval',
        'chart-store-fill',
        'chart-store-series-name-format-input',
      ]),
    ).toEqual([
      'chart-store-agent-select',
      'chart-store-range-window',
      'chart-store-interval',
      'chart-store-fill',
      'chart-store-series-name-format-input',
    ]);
  });

  it('시스템 지표: 에이전트 → 범위 → 인터벌 → 빈 구간 → 이름 형식 → 시리즈 표', () => {
    // 에이전트가 이력을 들고 있으므로 이 소스도 Store 와 **같은 조회 창 컨트롤**을 쓴다.
    // 종전의 새로고침 주기·표시 창 쌍은 브라우저 누적 모델의 잔재였다.
    renderSection({
      data_source: 'sysmetrics',
      sysmetrics_source: {
        agent_id: 'sy',
        agent_name: 'host-1',
        series: [{ key: 'cpu.usage_percent' }],
        time_window_ms: 60 * 60_000,
        interval_ms: 60_000,
      },
    });

    expect(
      orderOf([
        'chart-sysmetrics-agent-select',
        'chart-sysmetrics-range-window',
        'chart-sysmetrics-interval',
        'chart-sysmetrics-fill',
        'chart-sysmetrics-series-name-format-input',
        'chart-sysmetrics-series-table',
        'chart-sysmetrics-selected-count',
      ]),
    ).toEqual([
      'chart-sysmetrics-agent-select',
      'chart-sysmetrics-range-window',
      'chart-sysmetrics-interval',
      'chart-sysmetrics-fill',
      'chart-sysmetrics-series-name-format-input',
      'chart-sysmetrics-series-table',
      'chart-sysmetrics-selected-count',
    ]);
  });

  it('TSDB: 에이전트 → bucket → 범위 → 인터벌 → 집계 → 빈 구간 → 이름 형식 → 표 → 요약', () => {
    // TSDB 가 정본 순서의 기준이다 — 축이 가장 많아 다른 둘이 이 차례의 부분집합이 된다.
    renderSection({
      data_source: 'tsdb',
      tsdb_source: {
        backend: 'influxdb',
        agent_id: 'ix',
        agent_name: 'influx-1',
        series: [{ key: 'm', field: 'f' }],
        time_window_ms: 60_000,
        interval_ms: 10_000,
        aggregation: 'average',
      },
    });

    const expected = [
      'chart-tsdb-agent-select',
      'chart-tsdb-bucket-select',
      'chart-tsdb-range-window',
      'chart-tsdb-interval',
      'chart-tsdb-aggregation',
      'chart-tsdb-fill',
      'chart-tsdb-series-name-format-input',
      'chart-tsdb-series-select',
      'chart-tsdb-registered',
    ];
    expect(orderOf(expected)).toEqual(expected);
  });

  it('세 소스 모두 이름 형식이 시리즈 표보다 앞이다', () => {
    // 표는 목록이라 길다 — 뒤에 두지 않으면 그 아래 설정이 스크롤 너머로 밀린다.
    renderSection({
      data_source: 'sysmetrics',
      sysmetrics_source: {
        agent_id: 'sy',
        agent_name: 'host-1',
        series: [{ key: 'cpu.usage_percent' }],
      },
    });

    const name = screen.getByTestId('chart-sysmetrics-series-name-format-input');
    const table = screen.getByTestId('chart-sysmetrics-series-table');
    expect(precedes(name, table)).toBe(true);
  });
});
