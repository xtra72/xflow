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

// i18n 은 키를 그대로 반환하도록 모킹한다(I18nProvider 없이 렌더 가능).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

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
