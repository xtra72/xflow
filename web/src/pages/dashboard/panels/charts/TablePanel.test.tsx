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
      const store = storeHookResult.current;
      // 채널이 패널 소스에서 빠지면서 데이터 이음매가 하나로 줄었다. 이 파일의 오래된
      // 테스트들은 채널 형상(`mockResult.current`)을 심으므로, store 쪽에 심은 것이 없으면
      // 그 형상을 시리즈 소스 결과로 옮겨 준다 — 테스트 본문을 그대로 두기 위한 어댑터다.
      const storeHasData =
        (store.entries?.length ?? 0) > 0 || (store.seriesEntries?.size ?? 0) > 0;
      return storeHasData ? store : { ...store, ...mockResult.current };
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
    // 값 열은 기본 2자리로 포맷된다(decimal_places 미지정).
    expect(cells).toEqual(['10.00', '20.00', '30.00']);
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
describe('TablePanel — 열 폭 비율 / 정렬 허용 / 열 필터', () => {
  beforeEach(() => {
    mockResult.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });

  /** 이름 열을 가진 엔트리(필터 검증용). */
  function namedEntries(): ChartEntry[] {
    const base = new Date(2026, 3, 16, 14, 30, 0, 0).getTime();
    return [
      { timestamp: base, value: 10, name: 'alpha' },
      { timestamp: base + 1000, value: 25, name: 'bravo' },
      { timestamp: base + 2000, value: 30, name: 'Alpha-2' },
    ] as unknown as ChartEntry[];
  }

  const NAME_COLUMNS: TableColumn[] = [
    { field: 'name', header: 'Name', format: 'string', filterable: true },
    { field: 'value', header: 'Value', format: 'number' },
  ];

  it('폭 비율을 지정하면 colgroup 으로 폭을 나눈다', () => {
    mockResult.current.entries = makeEntries(1);
    const columns: TableColumn[] = [
      { field: 'timestamp', header: 'Time', format: 'datetime', width: 1 },
      { field: 'value', header: 'Value', format: 'number', width: 3 },
    ];
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c', columns }} />,
    );
    const cols = container.querySelectorAll('colgroup col');
    expect(cols).toHaveLength(2);
    expect((cols[0] as HTMLElement).style.width).toBe('25%');
    expect((cols[1] as HTMLElement).style.width).toBe('75%');
    expect(container.querySelector('table')?.className).toContain('table-fixed');
  });

  it('폭 미지정이면 colgroup 을 렌더하지 않는다 (자동 폭 유지)', () => {
    mockResult.current.entries = makeEntries(1);
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c' }} />,
    );
    expect(container.querySelector('colgroup')).toBeNull();
    expect(container.querySelector('table')?.className).not.toContain('table-fixed');
  });

  it('sortable: false 인 열은 헤더를 눌러도 정렬되지 않는다', () => {
    mockResult.current.entries = makeEntries(5);
    const columns: TableColumn[] = [
      { field: 'timestamp', header: 'Time', format: 'datetime' },
      { field: 'value', header: 'Value', format: 'number', sortable: false },
    ];
    render(<TablePanel panelId="p1" config={{ channel_name: 'c', columns }} />);
    const header = screen.getByTestId('table-header-value');
    expect(header.getAttribute('data-sortable')).toBe('false');
    fireEvent.click(header);
    expect(header.getAttribute('data-sort-order')).toBe('none');
  });

  it('sortable 미지정 열은 기존대로 정렬 가능하다 (하위 호환)', () => {
    mockResult.current.entries = makeEntries(5);
    render(<TablePanel panelId="p1" config={{ channel_name: 'c' }} />);
    const header = screen.getByTestId('table-header-value');
    expect(header.getAttribute('data-sortable')).toBe('true');
    fireEvent.click(header);
    expect(header.getAttribute('data-sort-order')).toBe('asc');
  });

  /** 필터 드롭다운을 연다(값 목록이 나열된다). */
  function openFilter(field: string) {
    const host = screen.getByTestId(`table-filter-${field}`);
    fireEvent.click(host.querySelector('button')!);
  }

  /** 드롭다운에 나열된 값 하나를 고른다. */
  function pickValue(label: string) {
    fireEvent.click(screen.getByLabelText(label));
  }

  it('filterable 열에만 필터 버튼을 렌더한다', () => {
    mockResult.current.entries = namedEntries();
    render(<TablePanel panelId="p1" config={{ channel_name: 'c', columns: NAME_COLUMNS }} />);
    expect(screen.getByTestId('table-filter-name')).toBeInTheDocument();
    expect(screen.queryByTestId('table-filter-value')).toBeNull();
  });

  it('filterable 열이 없으면 필터 버튼이 하나도 없다', () => {
    mockResult.current.entries = makeEntries(3);
    render(<TablePanel panelId="p1" config={{ channel_name: 'c' }} />);
    expect(screen.queryByTestId('table-filter-timestamp')).toBeNull();
    expect(screen.queryByTestId('table-filter-value')).toBeNull();
  });

  it('드롭다운에 그 열의 고유 값이 나열된다 (자유 입력이 아니다)', () => {
    mockResult.current.entries = namedEntries();
    render(<TablePanel panelId="p1" config={{ channel_name: 'c', columns: NAME_COLUMNS }} />);
    openFilter('name');
    // 중복 없이 정렬된 값 목록.
    expect(screen.getByLabelText('Alpha-2')).toBeInTheDocument();
    expect(screen.getByLabelText('alpha')).toBeInTheDocument();
    expect(screen.getByLabelText('bravo')).toBeInTheDocument();
  });

  it('나열된 값을 고르면 그 행만 남고 건수 표시도 줄어든다', () => {
    mockResult.current.entries = namedEntries();
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c', columns: NAME_COLUMNS }} />,
    );
    expect(container.querySelectorAll('tbody tr')).toHaveLength(3);

    openFilter('name');
    pickValue('alpha');
    expect(container.querySelectorAll('tbody tr')).toHaveLength(1);
    expect(screen.getByText('1건')).toBeInTheDocument();
  });

  it('값을 여러 개 고르면 OR 로 합쳐진다', () => {
    mockResult.current.entries = namedEntries();
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c', columns: NAME_COLUMNS }} />,
    );
    openFilter('name');
    pickValue('alpha');
    pickValue('bravo');
    expect(container.querySelectorAll('tbody tr')).toHaveLength(2);
  });

  it('고른 값을 다시 눌러 해제하면 전체로 돌아온다', () => {
    mockResult.current.entries = namedEntries();
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c', columns: NAME_COLUMNS }} />,
    );
    openFilter('name');
    pickValue('alpha');
    expect(container.querySelectorAll('tbody tr')).toHaveLength(1);
    pickValue('alpha');
    expect(container.querySelectorAll('tbody tr')).toHaveLength(3);
  });

  it('값 목록은 필터 적용 후에도 줄어들지 않는다 (되돌릴 수 있어야 한다)', () => {
    mockResult.current.entries = namedEntries();
    render(<TablePanel panelId="p1" config={{ channel_name: 'c', columns: NAME_COLUMNS }} />);
    openFilter('name');
    pickValue('alpha');
    // 선택 후에도 나머지 선택지가 그대로 남아 있어야 한다.
    expect(screen.getByLabelText('bravo')).toBeInTheDocument();
    expect(screen.getByLabelText('Alpha-2')).toBeInTheDocument();
  });

  it('필터 버튼 클릭이 헤더 정렬을 건드리지 않는다', () => {
    mockResult.current.entries = namedEntries();
    render(<TablePanel panelId="p1" config={{ channel_name: 'c', columns: NAME_COLUMNS }} />);
    openFilter('name');
    expect(screen.getByTestId('table-header-name').getAttribute('data-sort-order')).toBe('none');
  });

  it('필터와 정렬이 함께 적용된다', () => {
    mockResult.current.entries = namedEntries();
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c', columns: NAME_COLUMNS }} />,
    );
    openFilter('name');
    pickValue('alpha');
    pickValue('Alpha-2');
    fireEvent.click(screen.getByTestId('table-header-value'));
    const firstRowValue = container.querySelectorAll('tbody tr')[0]!.querySelectorAll('td')[1];
    expect(firstRowValue?.textContent).toBe('10.00');
  });
});

describe('TablePanel — 범위 필터 / 태그 컬럼', () => {
  beforeEach(() => {
    mockResult.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });

  /** 태그를 실은 엔트리(태그 컬럼 검증용). */
  function taggedEntries(): ChartEntry[] {
    const base = new Date(2026, 3, 16, 14, 30, 0, 0).getTime();
    return [
      { timestamp: base, value: 10, labels: { location: 'roomA' } },
      { timestamp: base + 60_000, value: 25, labels: { location: 'roomB' } },
      { timestamp: base + 120_000, value: 30, labels: { location: 'roomA' } },
    ] as unknown as ChartEntry[];
  }

  function openFilter(field: string) {
    const host = screen.getByTestId(`table-filter-${field}`);
    fireEvent.click(host.querySelector('button')!);
  }

  it('number 열은 값 목록이 아니라 최소/최대 입력을 연다', () => {
    mockResult.current.entries = makeEntries(3);
    const columns: TableColumn[] = [
      { field: 'timestamp', header: 'Time', format: 'datetime' },
      { field: 'value', header: 'Value', format: 'number', filterable: true },
    ];
    render(<TablePanel panelId="p1" config={{ channel_name: 'c', columns }} />);
    openFilter('value');
    expect(screen.getByLabelText('dashboard.chart.rangeMin')).toBeInTheDocument();
    expect(screen.getByLabelText('dashboard.chart.rangeMax')).toBeInTheDocument();
  });

  it('value 범위로 행을 좁힌다 (경계 포함)', () => {
    mockResult.current.entries = makeEntries(5); // value 1..5
    const columns: TableColumn[] = [
      { field: 'value', header: 'Value', format: 'number', filterable: true },
    ];
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c', columns }} />,
    );
    openFilter('value');
    fireEvent.change(screen.getByLabelText('dashboard.chart.rangeMin'), { target: { value: '2' } });
    fireEvent.change(screen.getByLabelText('dashboard.chart.rangeMax'), { target: { value: '4' } });
    expect(container.querySelectorAll('tbody tr')).toHaveLength(3);
    expect(screen.getByText('3건')).toBeInTheDocument();
  });

  it('한쪽 경계만 지정해도 동작하고, 비우면 해제된다', () => {
    mockResult.current.entries = makeEntries(5);
    const columns: TableColumn[] = [
      { field: 'value', header: 'Value', format: 'number', filterable: true },
    ];
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c', columns }} />,
    );
    openFilter('value');
    const min = screen.getByLabelText('dashboard.chart.rangeMin');
    fireEvent.change(min, { target: { value: '4' } });
    expect(container.querySelectorAll('tbody tr')).toHaveLength(2);
    fireEvent.change(min, { target: { value: '' } });
    expect(container.querySelectorAll('tbody tr')).toHaveLength(5);
  });

  it('datetime 열은 datetime-local 입력으로 구간을 받는다', () => {
    mockResult.current.entries = makeEntries(3);
    const columns: TableColumn[] = [
      { field: 'timestamp', header: 'Time', format: 'datetime', filterable: true },
    ];
    render(<TablePanel panelId="p1" config={{ channel_name: 'c', columns }} />);
    openFilter('timestamp');
    const min = screen.getByLabelText('dashboard.chart.rangeMin') as HTMLInputElement;
    expect(min.type).toBe('datetime-local');
  });

  it('범위 초기화 버튼이 구간을 지운다', () => {
    mockResult.current.entries = makeEntries(5);
    const columns: TableColumn[] = [
      { field: 'value', header: 'Value', format: 'number', filterable: true },
    ];
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c', columns }} />,
    );
    openFilter('value');
    fireEvent.change(screen.getByLabelText('dashboard.chart.rangeMin'), { target: { value: '4' } });
    expect(container.querySelectorAll('tbody tr')).toHaveLength(2);
    fireEvent.click(screen.getByText('dashboard.chart.rangeClear'));
    expect(container.querySelectorAll('tbody tr')).toHaveLength(5);
  });

  it('$.tags.<키> 열은 태그 값을 셀에 렌더한다', () => {
    mockResult.current.entries = taggedEntries();
    const columns: TableColumn[] = [
      { field: '$.tags.location', header: '위치', format: 'string' },
      { field: 'value', header: 'Value', format: 'number' },
    ];
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c', columns }} />,
    );
    const firstCell = container.querySelectorAll('tbody tr')[0]!.querySelector('td');
    expect(firstCell?.textContent).toBe('roomA');
  });

  it('태그 열은 값 선택 필터를 쓴다 (범위가 아니다)', () => {
    mockResult.current.entries = taggedEntries();
    const columns: TableColumn[] = [
      { field: '$.tags.location', header: '위치', format: 'string', filterable: true },
    ];
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c', columns }} />,
    );
    openFilter('$.tags.location');
    expect(screen.queryByLabelText('dashboard.chart.rangeMin')).toBeNull();
    // 태그 값이 목록으로 나열된다.
    fireEvent.click(screen.getByLabelText('roomA'));
    expect(container.querySelectorAll('tbody tr')).toHaveLength(2);
  });

  it('태그 열도 헤더 클릭으로 정렬된다', () => {
    mockResult.current.entries = taggedEntries();
    const columns: TableColumn[] = [
      { field: '$.tags.location', header: '위치', format: 'string' },
    ];
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c', columns }} />,
    );
    fireEvent.click(screen.getByTestId('table-header-$.tags.location'));
    const cells = Array.from(container.querySelectorAll('tbody tr td')).map((c) => c.textContent);
    expect(cells).toEqual(['roomA', 'roomA', 'roomB']);
  });
});

describe('TablePanel — 열 경계 드래그 리사이즈', () => {
  beforeEach(() => {
    mockResult.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });

  const COLUMNS: TableColumn[] = [
    { field: 'timestamp', header: 'Time', format: 'datetime' },
    { field: 'value', header: 'Value', format: 'number' },
  ];

  /** jsdom 은 레이아웃이 없어 rect 가 전부 0 이다 — 헤더 셀 폭을 주입한다. */
  function stubHeaderWidths(container: HTMLElement, widths: number[]) {
    const cells = container.querySelectorAll('thead tr th');
    cells.forEach((cell, i) => {
      const w = widths[i] ?? 0;
      (cell as HTMLElement).getBoundingClientRect = () =>
        ({ width: w, height: 20, top: 0, left: 0, right: w, bottom: 20, x: 0, y: 0, toJSON: () => ({}) }) as DOMRect;
    });
  }

  // jsdom 의 PointerEvent 는 fireEvent init 의 clientX 를 싣지 못한다 —
  // 같은 타입의 MouseEvent 로 좌표를 실어 보낸다(런타임은 포인터 이벤트 그대로).
  function pointer(type: string, clientX: number): MouseEvent {
    return new MouseEvent(type, { clientX, bubbles: true });
  }

  function drag(handle: Element, fromX: number, toX: number) {
    fireEvent(handle, pointer('pointerdown', fromX));
    fireEvent(document, pointer('pointermove', toX));
    fireEvent(document, pointer('pointerup', toX));
  }

  it('onColumnsChange 가 없으면 드래그 손잡이를 렌더하지 않는다 (대시보드 패널)', () => {
    mockResult.current.entries = makeEntries(2);
    render(<TablePanel panelId="p1" config={{ channel_name: 'c', columns: COLUMNS }} />);
    expect(screen.queryByTestId('table-resize-timestamp')).toBeNull();
  });

  it('onColumnsChange 가 있으면 마지막 열을 제외한 경계에 손잡이를 렌더한다', () => {
    mockResult.current.entries = makeEntries(2);
    render(
      <TablePanel
        panelId="p1"
        config={{ channel_name: 'c', columns: COLUMNS }}
        onColumnsChange={vi.fn()}
      />,
    );
    expect(screen.getByTestId('table-resize-timestamp')).toBeInTheDocument();
    // 마지막 열 뒤에는 주고받을 상대가 없다.
    expect(screen.queryByTestId('table-resize-value')).toBeNull();
  });

  it('경계를 끌면 인접 두 열의 폭을 주고받아 콜백한다', () => {
    mockResult.current.entries = makeEntries(2);
    const onColumnsChange = vi.fn();
    const { container } = render(
      <TablePanel
        panelId="p1"
        config={{ channel_name: 'c', columns: COLUMNS }}
        onColumnsChange={onColumnsChange}
      />,
    );
    stubHeaderWidths(container, [100, 300]);
    drag(screen.getByTestId('table-resize-timestamp'), 100, 150);

    expect(onColumnsChange).toHaveBeenCalled();
    const last = onColumnsChange.mock.calls.at(-1)![0] as TableColumn[];
    expect(last.map((c) => c.width)).toEqual([150, 250]);
  });

  it('최소 폭 아래로는 줄지 않는다', () => {
    mockResult.current.entries = makeEntries(2);
    const onColumnsChange = vi.fn();
    const { container } = render(
      <TablePanel
        panelId="p1"
        config={{ channel_name: 'c', columns: COLUMNS }}
        onColumnsChange={onColumnsChange}
      />,
    );
    stubHeaderWidths(container, [100, 300]);
    drag(screen.getByTestId('table-resize-timestamp'), 100, -9999);

    const last = onColumnsChange.mock.calls.at(-1)![0] as TableColumn[];
    expect(last[0]!.width).toBe(32); // MIN_COLUMN_PX
    expect(last[0]!.width! + last[1]!.width!).toBe(400); // 전체 폭 보존
  });

  it('폭을 잴 수 없으면(레이아웃 전) 드래그를 시작하지 않는다', () => {
    mockResult.current.entries = makeEntries(2);
    const onColumnsChange = vi.fn();
    render(
      <TablePanel
        panelId="p1"
        config={{ channel_name: 'c', columns: COLUMNS }}
        onColumnsChange={onColumnsChange}
      />,
    );
    // rect 스텁 없음 → jsdom 기본 0 폭.
    drag(screen.getByTestId('table-resize-timestamp'), 100, 150);
    expect(onColumnsChange).not.toHaveBeenCalled();
  });

  it('손잡이 클릭이 헤더 정렬을 건드리지 않는다', () => {
    mockResult.current.entries = makeEntries(2);
    const { container } = render(
      <TablePanel
        panelId="p1"
        config={{ channel_name: 'c', columns: COLUMNS }}
        onColumnsChange={vi.fn()}
      />,
    );
    stubHeaderWidths(container, [100, 300]);
    fireEvent(screen.getByTestId('table-resize-timestamp'), pointer('pointerdown', 100));
    fireEvent(document, pointer('pointerup', 100));
    expect(screen.getByTestId('table-header-timestamp').getAttribute('data-sort-order')).toBe('none');
  });

  it('포인터를 뗀 뒤의 이동은 폭을 바꾸지 않는다', () => {
    mockResult.current.entries = makeEntries(2);
    const onColumnsChange = vi.fn();
    const { container } = render(
      <TablePanel
        panelId="p1"
        config={{ channel_name: 'c', columns: COLUMNS }}
        onColumnsChange={onColumnsChange}
      />,
    );
    stubHeaderWidths(container, [100, 300]);
    drag(screen.getByTestId('table-resize-timestamp'), 100, 150);
    const callsAfterDrag = onColumnsChange.mock.calls.length;

    fireEvent(document, pointer('pointermove', 400));
    expect(onColumnsChange.mock.calls.length).toBe(callsAfterDrag);
  });
});

describe('TablePanel — 시각 기준 행 (넓은 형식)', () => {
  beforeEach(() => {
    mockResult.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });

  /** 같은 시각에 두 시리즈, 다음 시각에 한 시리즈. */
  function twoSeries(): ChartEntry[] {
    const base = new Date(2026, 3, 16, 14, 30, 0, 0).getTime();
    return [
      { timestamp: base, value: 21.5, labels: { name: '온도' }, meta: { seriesName: '온도' } },
      { timestamp: base, value: 40, labels: { name: '습도' }, meta: { seriesName: '습도' } },
      { timestamp: base + 1000, value: 22, labels: { name: '온도' }, meta: { seriesName: '온도' } },
    ];
  }

  it('시리즈마다 열이 하나 생기고 헤더는 시리즈 이름이다', () => {
    mockResult.current.entries = twoSeries();
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c', row_mode: 'timestamp' }} />,
    );
    const headers = Array.from(container.querySelectorAll('th')).map((h) => h.textContent ?? '');
    expect(headers).toHaveLength(3);
    expect(headers[0]).toContain('dashboard.chart.colTime');
    expect(headers[1]).toContain('온도');
    expect(headers[2]).toContain('습도');
  });

  it('같은 시각의 값들이 한 행에 모인다', () => {
    mockResult.current.entries = twoSeries();
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c', row_mode: 'timestamp' }} />,
    );
    const rows = container.querySelectorAll('tbody tr');
    expect(rows.length).toBe(2);
    const first = Array.from(rows[0]!.querySelectorAll('td')).map((td) => td.textContent);
    expect(first).toEqual(['2026-04-16 14:30:00.000', '21.50', '40.00']);
  });

  it('그 시각에 값이 없던 시리즈는 빈칸이다', () => {
    mockResult.current.entries = twoSeries();
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c', row_mode: 'timestamp' }} />,
    );
    const second = Array.from(container.querySelectorAll('tbody tr')[1]!.querySelectorAll('td'));
    expect(second.map((td) => td.textContent)).toEqual([
      '2026-04-16 14:30:01.000',
      '22.00',
      '',
    ]);
  });

  it('자릿수와 공통 단위를 시리즈 열에 적용한다', () => {
    mockResult.current.entries = twoSeries();
    const { container } = render(
      <TablePanel
        panelId="p1"
        config={{ channel_name: 'c', row_mode: 'timestamp', decimal_places: 1, pivot_unit: '°C' }}
      />,
    );
    const first = Array.from(container.querySelectorAll('tbody tr td')).map((td) => td.textContent);
    // 시각 열에는 단위가 붙지 않는다.
    expect(first[0]).toBe('2026-04-16 14:30:00.000');
    expect(first[1]).toBe('21.5°C');
  });

  it('시각 열 이름을 바꿀 수 있다', () => {
    mockResult.current.entries = twoSeries();
    const { container } = render(
      <TablePanel
        panelId="p1"
        config={{ channel_name: 'c', row_mode: 'timestamp', pivot_time_header: '측정 시각' }}
      />,
    );
    expect(container.querySelector('th')?.textContent).toContain('측정 시각');
  });

  it('시리즈 열 헤더를 눌러 정렬한다 — 값이 없는 행도 순서가 정해진다', () => {
    mockResult.current.entries = twoSeries();
    const { container } = render(
      <TablePanel panelId="p1" config={{ channel_name: 'c', row_mode: 'timestamp' }} />,
    );
    const header = screen.getByTestId('table-header-$.series.온도');
    fireEvent.click(header);
    expect(header.getAttribute('data-sort-order')).toBe('asc');
    const firstCell = container.querySelectorAll('tbody tr')[0]!.querySelectorAll('td')[1];
    expect(firstCell?.textContent).toBe('21.50');
  });

  it('row_mode 를 주지 않으면 종전대로 엔트리별 행이다', () => {
    mockResult.current.entries = twoSeries();
    const { container } = render(<TablePanel panelId="p1" config={{ channel_name: 'c' }} />);
    // 기본 열은 timestamp + value 두 개, 행은 엔트리 수만큼이다.
    expect(container.querySelectorAll('th').length).toBe(2);
    expect(container.querySelectorAll('tbody tr').length).toBe(3);
  });
});
