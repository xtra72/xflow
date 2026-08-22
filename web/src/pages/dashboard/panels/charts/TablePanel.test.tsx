// TablePanel 테스트.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

import type { ChartEntry, TableColumn } from './chartChannelTypes';

const mockResult = vi.hoisted(() => ({
  current: {
    entries: [] as ChartEntry[],
    status: 'connected' as const,
    closedReason: undefined as string | undefined,
    errorReason: undefined as string | undefined,
  },
}));

vi.mock('./useChartChannel', () => ({
  useChartChannel: () => mockResult.current,
}));

// useStoreChartData 가 내부에서 useAgents(React Query)를 호출하므로, QueryClient
// 없이 렌더 가능하도록 빈 목록으로 모킹한다. 목록이 비면 store 소스는 저장된
// 이름을 그대로 사용(fallback)해 기존 동작이 유지된다. (SPEC-WEB-006)
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [] } }),
}));

// i18n 은 키를 그대로 반환하도록 모킹한다(I18nProvider 없이 렌더 가능).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

// SPEC-TSDB-002 M2 특성화용 store 훅 기록기. 실제 export 는 그대로 두고 훅만 교체해
// (소스, 활성) 인자를 관측한다. 채널 모드 테스트에서는 idle 결과를 반환하므로 기존
// 동작(비활성 store 훅)과 동일하다.
const storeHookCalls = vi.hoisted(() => ({
  args: [] as Array<{ source: unknown; enabled: unknown }>,
}));
const storeHookResult = vi.hoisted(() => ({
  current: {
    entries: [] as Array<{ timestamp: number; value: unknown; labels?: Record<string, string> }>,
    seriesEntries: new Map<string, unknown>(),
    seriesStyles: new Map<string, unknown>(),
    seriesNames: [] as string[],
    booleanSeries: new Set<string>(),
    status: 'connected' as const,
    closedReason: undefined as string | undefined,
    errorReason: undefined as string | undefined,
  },
}));

vi.mock('./useStoreChartData', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./useStoreChartData')>();
  return {
    ...actual,
    useStoreChartData: (source: unknown, enabled: unknown) => {
      storeHookCalls.args.push({ source, enabled });
      return storeHookResult.current;
    },
  };
});

import TablePanel from './TablePanel';

function makeEntries(n: number): ChartEntry[] {
  const arr: ChartEntry[] = [];
  // 2026-04-16 14:30:00.000 local
  const base = new Date(2026, 3, 16, 14, 30, 0, 0).getTime();
  for (let i = 0; i < n; i++) {
    arr.push({ timestamp: base + i * 1000, value: i + 1 });
  }
  return arr;
}

describe('TablePanel', () => {
  beforeEach(() => {
    mockResult.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });

  it('entries 비어있을 때 "데이터 없음" 표시', () => {
    render(
      <TablePanel panelId="p1" config={{ channel_name: 'c' }} />,
    );
    expect(screen.getByText('데이터 없음')).toBeInTheDocument();
  });

  it('기본 컬럼 timestamp + value 표시', () => {
    mockResult.current.entries = makeEntries(3);
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c' }} />,
    );
    // 헤더
    const headers = container.querySelectorAll('th');
    expect(headers.length).toBe(2);
    expect(headers[0]!.textContent).toContain('Time');
    expect(headers[1]!.textContent).toContain('Value');

    // body 행
    const rows = container.querySelectorAll('tbody tr');
    expect(rows.length).toBe(3);
  });

  it('timestamp 컬럼이 YYYY-MM-DD HH:mm:ss.SSS 포맷', () => {
    mockResult.current.entries = makeEntries(1);
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c' }} />,
    );
    const firstCell = container.querySelector('tbody tr td');
    expect(firstCell?.textContent).toBe('2026-04-16 14:30:00.000');
  });

  it('헤더 클릭 시 정렬 순서 토글 (unsorted -> asc -> desc -> unsorted)', () => {
    mockResult.current.entries = makeEntries(5);
    render(<TablePanel panelId="p1" config={{ channel_name: 'c' }} />);
    const header = screen.getByTestId('table-header-value');

    expect(header.getAttribute('data-sort-order')).toBe('none');
    fireEvent.click(header);
    expect(header.getAttribute('data-sort-order')).toBe('asc');
    fireEvent.click(header);
    expect(header.getAttribute('data-sort-order')).toBe('desc');
    fireEvent.click(header);
    expect(header.getAttribute('data-sort-order')).toBe('none');
  });

  it('asc 정렬 후 행 값이 오름차순', () => {
    mockResult.current.entries = [
      { timestamp: 3, value: 30 },
      { timestamp: 1, value: 10 },
      { timestamp: 2, value: 20 },
    ];
    const { container } = render(
      <TablePanel
        panelId="p1"
        config={{
          channel_name: 'c',
          columns: [
            { field: 'value', header: 'Value', format: 'number' },
          ] as TableColumn[],
        }}
      />,
    );
    const header = screen.getByTestId('table-header-value');
    fireEvent.click(header);
    const cells = Array.from(container.querySelectorAll('tbody tr td')).map(
      (c) => c.textContent,
    );
    expect(cells).toEqual(['10', '20', '30']);
  });

  it('페이지네이션: rows_per_page=2 에 5개 데이터 -> 3페이지', () => {
    mockResult.current.entries = makeEntries(5);
    render(
      <TablePanel
        panelId="p1"
        config={{ channel_name: 'c', rows_per_page: 2 }}
      />,
    );
    // 페이지 표시에 1 / 3
    expect(screen.getByText('1 / 3')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('table-page-next'));
    expect(screen.getByText('2 / 3')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('table-page-next'));
    expect(screen.getByText('3 / 3')).toBeInTheDocument();

    // 마지막 페이지에서 next 비활성화
    const next = screen.getByTestId('table-page-next') as HTMLButtonElement;
    expect(next.disabled).toBe(true);
  });

  it('default_sort 적용된 상태로 초기화', () => {
    mockResult.current.entries = makeEntries(3);
    render(
      <TablePanel
        panelId="p1"
        config={{
          channel_name: 'c',
          default_sort: { field: 'value', order: 'desc' },
        }}
      />,
    );
    const header = screen.getByTestId('table-header-value');
    expect(header.getAttribute('data-sort-order')).toBe('desc');
  });

  it('closed overlay', () => {
    mockResult.current.status = 'closed' as unknown as 'connected';
    mockResult.current.closedReason = 'flow_undeployed';
    render(<TablePanel panelId="p1" config={{ channel_name: 'c' }} />);
    expect(screen.getByTestId('table-overlay').textContent).toContain('flow_undeployed');
  });

  it('연결 상태 아이콘 렌더', () => {
    render(<TablePanel panelId="p1" config={{ channel_name: 'c' }} />);
    expect(screen.getByTestId('chart-status-icon')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// SPEC-TSDB-002 M2 — 특성화 테스트 (DDD PRESERVE).
//
// `TablePanel.tsx:101` 의 소스 활성 판정을 5분기로 잠근다.
//
// @spec SPEC-TSDB-002 §2.3 (U3) · §2.4 (U4) — plan.md §3.1 CT-01 ~ CT-05 / AC-09
// ---------------------------------------------------------------------------
describe('TablePanel 소스 활성 판정 특성화 (SPEC-TSDB-002 M2, CT-01~CT-05)', () => {
  function lastStoreCall(): { source: unknown; enabled: unknown } {
    return storeHookCalls.args.at(-1)!;
  }
  /** 첫 행의 Value 셀 — 채널/store 어느 쪽 entries 를 소비했는지 드러낸다. */
  function firstValue(container: HTMLElement): string {
    return container.querySelectorAll('tbody tr td')[1]?.textContent ?? '';
  }

  const TS = new Date(2026, 3, 16, 14, 30, 0, 0).getTime();

  beforeEach(() => {
    storeHookCalls.args = [];
    mockResult.current = {
      entries: [{ timestamp: TS, value: 11 }],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
    storeHookResult.current = {
      ...storeHookResult.current,
      entries: [{ timestamp: TS, value: 99 }],
      status: 'connected',
    };
  });

  it('CT-01: config 가 비어 있으면 채널 경로다(store 훅은 idle)', () => {
    const { container } = render(<TablePanel panelId="p1" config={{ channel_name: 'c1' }} />);
    expect(lastStoreCall()).toEqual({ source: undefined, enabled: false });
    expect(firstValue(container)).toBe('11');
  });

  it("CT-02: data_source:'channel' 이면 채널 경로다", () => {
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c1', data_source: 'channel' }} />,
    );
    expect(lastStoreCall()).toEqual({ source: undefined, enabled: false });
    expect(firstValue(container)).toBe('11');
  });

  it("CT-03: data_source:'store' 인데 store_source 가 없으면 채널 경로로 폴백한다", () => {
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c1', data_source: 'store' }} />,
    );
    expect(lastStoreCall()).toEqual({ source: undefined, enabled: false });
    expect(firstValue(container)).toBe('11');
  });

  it("CT-04: data_source:'store' + 시리즈 0개면 채널 경로로 폴백한다", () => {
    const { container } = render(
      <TablePanel
        panelId="p1"
        config={{
          channel_name: 'c1',
          data_source: 'store',
          store_source: { agent_name: 'store-1', series: [] },
        }}
      />,
    );
    expect(lastStoreCall()).toEqual({ source: undefined, enabled: false });
    expect(firstValue(container)).toBe('11');
  });

  it("CT-05: data_source:'store' + 시리즈 N개면 store 경로다", () => {
    const store_source = { agent_name: 'store-1', series: [{ key: 'k1' }] };
    const { container } = render(
      <TablePanel
        panelId="p1"
        config={{ channel_name: 'c1', data_source: 'store', store_source }}
      />,
    );
    expect(lastStoreCall()).toEqual({ source: store_source, enabled: true });
    expect(firstValue(container)).toBe('99');
  });
});
