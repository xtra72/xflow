// Table 패널 (REQ-M4-08).
// 각 행 = 하나의 entry, 열 = columns config 의 field.
// - 헤더 클릭 시 정렬 토글 (asc -> desc -> unsorted)
// - timestamp 열은 YYYY-MM-DD HH:mm:ss.SSS 로 포맷
// - 클라이언트 페이지네이션

import { useMemo, useState } from 'react';
import { clsx } from 'clsx';
import { ChevronDown, ChevronUp } from 'lucide-react';

import {
  getByPath,
  type ChartEntry,
  type SortOrder,
  type StoreSourceConfig,
  type TableColumn,
  type TablePanelConfig,
} from './chartChannelTypes';
import { Table as TableIcon } from 'lucide-react';

import { ConnectionStatusIcon } from './ConnectionStatusIcon';
import { formatTimestamp } from './chartChannelUtils';
import { useChartChannel } from './useChartChannel';
import { useStoreChartData } from './useStoreChartData';
import { usePanelTitleVisible } from '../../panelChromeContext';

interface TablePanelProps {
  panelId: string;
  title?: string;
  config: Record<string, unknown>;
}

const DEFAULT_COLUMNS: TableColumn[] = [
  { field: 'timestamp', header: 'Time', format: 'datetime' },
  { field: 'value', header: 'Value', format: 'string' },
];

const DEFAULT_ROWS_PER_PAGE = 20;
const DEFAULT_MAX_POINTS = 200;

function parseConfig(config: Record<string, unknown>): TablePanelConfig {
  const rawCols = config.columns as TableColumn[] | undefined;
  const columns = Array.isArray(rawCols) && rawCols.length > 0 ? rawCols : DEFAULT_COLUMNS;
  return {
    channel_name: (config.channel_name as string) ?? '',
    columns,
    rows_per_page: (config.rows_per_page as number) ?? DEFAULT_ROWS_PER_PAGE,
    max_points: (config.max_points as number) ?? DEFAULT_MAX_POINTS,
    default_sort: config.default_sort as TablePanelConfig['default_sort'],
  };
}

function formatCell(value: unknown, format: TableColumn['format']): string {
  if (value == null) return '';
  if (format === 'datetime') {
    const n = typeof value === 'number' ? value : Number(value);
    if (!Number.isFinite(n)) return String(value);
    return formatTimestamp(n);
  }
  if (format === 'number') {
    const n = typeof value === 'number' ? value : Number(value);
    return Number.isFinite(n) ? String(n) : String(value);
  }
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

type SortState = { field: string; order: SortOrder } | null;

function compareValues(a: unknown, b: unknown): number {
  if (a == null && b == null) return 0;
  if (a == null) return -1;
  if (b == null) return 1;
  if (typeof a === 'number' && typeof b === 'number') return a - b;
  const as = String(a);
  const bs = String(b);
  if (as < bs) return -1;
  if (as > bs) return 1;
  return 0;
}

function sortEntries(entries: ChartEntry[], sort: SortState): ChartEntry[] {
  if (!sort) return entries;
  const copy = entries.slice();
  copy.sort((a, b) => {
    const av = getByPath(a, sort.field);
    const bv = getByPath(b, sort.field);
    const cmp = compareValues(av, bv);
    return sort.order === 'asc' ? cmp : -cmp;
  });
  return copy;
}

export default function TablePanel({ panelId: _panelId, title, config }: TablePanelProps) {
  const showTitle = usePanelTitleVisible();
  const cfg = parseConfig(config);

  // SPEC-WEB-005: data_source 에 따라 Store 소스 또는 채널 소스를 사용한다(공존).
  const storeSource = config.store_source as StoreSourceConfig | undefined;
  const isStore =
    config.data_source === 'store' && (storeSource?.series?.length ?? 0) > 0;

  const channelRes = useChartChannel(
    isStore ? undefined : cfg.channel_name || undefined,
    { maxPoints: cfg.max_points ?? DEFAULT_MAX_POINTS },
  );
  const storeRes = useStoreChartData(isStore ? storeSource : undefined, isStore);

  const { entries, status, closedReason, errorReason } = isStore
    ? storeRes
    : channelRes;

  const [sortState, setSortState] = useState<SortState>(() =>
    cfg.default_sort ? { field: cfg.default_sort.field, order: cfg.default_sort.order } : null,
  );
  const [page, setPage] = useState(0);

  const sorted = useMemo(() => sortEntries(entries, sortState), [entries, sortState]);

  const rowsPerPage = cfg.rows_per_page ?? DEFAULT_ROWS_PER_PAGE;
  const totalPages = Math.max(1, Math.ceil(sorted.length / rowsPerPage));
  // 데이터가 줄어들 때 페이지 범위 보정
  const safePage = Math.min(page, totalPages - 1);
  const startIdx = safePage * rowsPerPage;
  const pageRows = sorted.slice(startIdx, startIdx + rowsPerPage);

  const handleHeaderClick = (field: string) => {
    setSortState((prev) => {
      // unsorted -> asc -> desc -> unsorted
      if (!prev || prev.field !== field) return { field, order: 'asc' };
      if (prev.order === 'asc') return { field, order: 'desc' };
      return null;
    });
  };

  return (
    <div className="relative flex min-h-0 flex-1 flex-col rounded-2xl bg-(--color-bg-surface) p-4 ring-1 ring-(--color-border-default)">
      <div className="absolute right-3 top-3 z-10">
        <ConnectionStatusIcon status={status} />
      </div>

      {showTitle && (
        <div className="mb-1 flex shrink-0 items-center gap-2 pr-6">
          <TableIcon className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
          <span className="truncate text-sm font-semibold text-(--color-text-primary)">
            {title || cfg.channel_name || '채널 미지정'}
          </span>
        </div>
      )}
      <div className="mb-2 truncate pr-6 text-xs font-medium text-(--color-text-muted)">
        {entries.length}건
      </div>

      <div className="min-h-0 flex-1 overflow-auto">
        <table className="w-full text-left text-xs" data-testid="chart-table">
          <thead className="sticky top-0 bg-(--color-bg-surface)">
            <tr>
              {cfg.columns.map((col) => {
                const isSorted = sortState?.field === col.field;
                return (
                  <th
                    key={col.field}
                    onClick={() => handleHeaderClick(col.field)}
                    className={clsx(
                      'cursor-pointer select-none border-b border-(--color-border-default) px-2 py-1.5',
                      'text-(--color-text-muted) hover:text-(--color-text-primary)',
                    )}
                    data-testid={`table-header-${col.field}`}
                    data-sort-order={isSorted ? sortState!.order : 'none'}
                  >
                    <span className="inline-flex items-center gap-1">
                      {col.header}
                      {isSorted && sortState!.order === 'asc' && (
                        <ChevronUp className="h-3 w-3" />
                      )}
                      {isSorted && sortState!.order === 'desc' && (
                        <ChevronDown className="h-3 w-3" />
                      )}
                    </span>
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody>
            {pageRows.length === 0 ? (
              <tr>
                <td
                  colSpan={cfg.columns.length}
                  className="px-2 py-6 text-center text-(--color-text-muted)"
                >
                  데이터 없음
                </td>
              </tr>
            ) : (
              pageRows.map((entry, idx) => (
                <tr
                  key={`${entry.timestamp}-${idx}`}
                  className="border-b border-(--color-border-subtle) hover:bg-(--color-bg-elevated)"
                >
                  {cfg.columns.map((col) => {
                    const v = getByPath(entry, col.field);
                    return (
                      <td
                        key={col.field}
                        className="px-2 py-1 tabular-nums text-(--color-text-primary)"
                      >
                        {formatCell(v, col.format)}
                      </td>
                    );
                  })}
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* 페이지네이션 */}
      {totalPages > 1 && (
        <div className="mt-2 flex shrink-0 items-center justify-between text-xs text-(--color-text-muted)">
          <span>
            {startIdx + 1}-{Math.min(startIdx + rowsPerPage, sorted.length)} / {sorted.length}
          </span>
          <div className="flex items-center gap-1">
            <button
              type="button"
              onClick={() => setPage((p) => Math.max(0, p - 1))}
              disabled={safePage <= 0}
              className="rounded border border-(--color-border-default) px-2 py-0.5 disabled:opacity-50"
              data-testid="table-page-prev"
            >
              ‹
            </button>
            <span>
              {safePage + 1} / {totalPages}
            </span>
            <button
              type="button"
              onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
              disabled={safePage >= totalPages - 1}
              className="rounded border border-(--color-border-default) px-2 py-0.5 disabled:opacity-50"
              data-testid="table-page-next"
            >
              ›
            </button>
          </div>
        </div>
      )}

      {(status === 'closed' || status === 'error') && (
        <div
          className="absolute inset-0 flex items-center justify-center rounded-2xl bg-black/50 p-4 text-center text-sm text-white"
          data-testid="table-overlay"
        >
          {status === 'closed'
            ? `Channel closed: ${closedReason ?? 'unknown'}`
            : `Error: ${errorReason ?? 'unknown'}`}
        </div>
      )}
    </div>
  );
}
