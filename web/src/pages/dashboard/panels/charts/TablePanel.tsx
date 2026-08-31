// Table 패널 (REQ-M4-08).
// 각 행 = 하나의 entry, 열 = columns config 의 field.
// - 헤더 클릭 시 정렬 토글 (asc -> desc -> unsorted)
// - timestamp 열은 YYYY-MM-DD HH:mm:ss.SSS 로 포맷
// - 클라이언트 페이지네이션

import { useEffect, useMemo, useRef, useState } from 'react';
import { clsx } from 'clsx';
import { ChevronDown, ChevronUp } from 'lucide-react';

import {
  type ChartEntry,
  type SortOrder,
  type TableColumn,
  type TablePanelConfig,
  type TableRowMode,
} from './chartChannelTypes';
import { Table as TableIcon } from 'lucide-react';

import { ConnectionStatusIcon } from './ConnectionStatusIcon';
import { ColumnFilterButton } from '@/pages/agents/storeColumns';

import { TableRangeFilterButton } from './TableRangeFilterButton';
import {
  applyColumnWidths,
  applyTableColumnFilters,
  columnWidthPercents,
  formatCell,
  isRangeFilterColumn,
  resizeColumnWidths,
  resolveCellValue,
  uniqueTableColumnValues,
  type ColumnFilters,
} from './tableColumns';
// 값 표기 자릿수는 차트 계열 공용 규칙을 따른다.
import { readDecimalPlaces } from './decimalPlaces';
import { pivotByTimestamp, pivotColumns } from './tablePivot';
import { useChartChannel } from './useChartChannel';
import { resolvePanelSourceBinding } from './panelDataSource';
import { isPanelSeriesSource, usePanelSeriesData } from './usePanelSeriesData';
import { usePanelTitleVisible } from '../../panelChromeContext';
import { useTranslation } from '@/lib/i18n';

interface TablePanelProps {
  panelId: string;
  title?: string;
  config: Record<string, unknown>;
  /**
   * 열 폭 드래그 조절 결과를 받는 콜백. **주어질 때만** 헤더 경계에 드래그 손잡이가
   * 생긴다 — 대시보드에 놓인 패널은 설정을 쓸 수 없으므로 넘기지 않고, 설정 화면의
   * 미리보기만 draft writer 를 넘겨 조절을 활성화한다(히트맵 센서 배치와 같은 방식).
   */
  onColumnsChange?: (columns: TableColumn[]) => void;
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
    row_mode: (config.row_mode as TableRowMode | undefined) ?? 'entry',
    pivot_time_header: config.pivot_time_header as string | undefined,
    pivot_unit: config.pivot_unit as string | undefined,
  };
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
    // 정렬도 태그 경로(`$.tags.x`)를 알아야 한다 — 아니면 태그 열은 헤더를 눌러도
    // 전부 undefined 로 비교되어 순서가 바뀌지 않는다.
    const av = resolveCellValue(a, sort.field);
    const bv = resolveCellValue(b, sort.field);
    const cmp = compareValues(av, bv);
    return sort.order === 'asc' ? cmp : -cmp;
  });
  return copy;
}

export default function TablePanel({
  panelId: _panelId,
  title,
  config,
  onColumnsChange,
}: TablePanelProps) {
  const showTitle = usePanelTitleVisible();
  const { t } = useTranslation();
  const cfg = parseConfig(config);
  // 값 표기 자릿수 — 셀·필터 목록·필터 판정이 **같은 값**을 써야 표시와 필터가 어긋나지 않는다.
  const decimals = readDecimalPlaces(config);

  // SPEC-WEB-005: data_source 에 따라 Store 소스 또는 채널 소스를 사용한다(공존).
  // SPEC-TSDB-002 §2.3 [U3]: 소스 판정은 `panelDataSource` 계약이 소유한다. 패널은
  // `data_source` 를 직접 비교하지 않는다 — 소스 종류가 늘어도 이 지점이 종류만큼
  // 곱해지지 않게 하기 위함이다(UB1-1).
  // `isStore` 는 "채널이 아닌 시리즈 소스가 활성인가" 를 뜻한다. M3 시점에는 store 만
  // 그 조건을 만족하며, tsdb 는 M6 에서 같은 이름을 통해 합류한다.
  const isStore = isPanelSeriesSource(resolvePanelSourceBinding(config));

  const channelRes = useChartChannel(
    isStore ? undefined : cfg.channel_name || undefined,
    { maxPoints: cfg.max_points ?? DEFAULT_MAX_POINTS },
  );
  const storeRes = usePanelSeriesData(config);

  const { entries, status, closedReason, errorReason } = isStore
    ? storeRes
    : channelRes;

  const [sortState, setSortState] = useState<SortState>(() =>
    cfg.default_sort ? { field: cfg.default_sort.field, order: cfg.default_sort.order } : null,
  );
  const [page, setPage] = useState(0);
  const [filters, setFilters] = useState<ColumnFilters>({});

  // ---- 시각 기준 행(넓은 형식) ----
  //
  // 행과 열을 여기서 한 번만 정하고, 아래 파이프라인(필터·정렬·페이지·렌더)은 어느
  // 모드인지 몰라도 되게 한다. 두 모드가 아래로 갈라지면 필터·정렬 규칙이 모드마다
  // 조용히 어긋난다.
  const pivot = useMemo(
    () =>
      cfg.row_mode === 'timestamp'
        ? pivotByTimestamp(entries, t('dashboard.chart.colValue'))
        : null,
    [entries, cfg.row_mode, t],
  );
  const rows: ChartEntry[] = pivot ? pivot.rows : entries;
  const columns = useMemo(
    () =>
      pivot
        ? pivotColumns(
            pivot.seriesNames,
            cfg.columns,
            cfg.pivot_time_header?.trim() || t('dashboard.chart.colTime'),
            cfg.pivot_unit,
          )
        : cfg.columns,
    [pivot, cfg.columns, cfg.pivot_time_header, cfg.pivot_unit, t],
  );

  const widths = useMemo(() => columnWidthPercents(columns), [columns]);

  // 필터 드롭다운에 나열할 고유 값. **필터 적용 전 전체 엔트리**에서 뽑는다 — 적용 후에서
  // 뽑으면 한 값을 고르는 순간 나머지 선택지가 목록에서 사라져 되돌릴 수 없다
  // (Store 엔트리 표가 allEntries 로 목록을 만드는 것과 같은 이유).
  const uniqueValues = useMemo(() => {
    const map = new Map<string, string[]>();
    for (const col of columns) {
      if (col.filterable === true && !isRangeFilterColumn(col)) {
        map.set(col.field, uniqueTableColumnValues(rows, col, decimals));
      }
    }
    return map;
  }, [rows, columns, decimals]);

  const filtered = useMemo(
    () => applyTableColumnFilters(rows, columns, filters, decimals),
    [rows, columns, filters, decimals],
  );
  const sorted = useMemo(() => sortEntries(filtered, sortState), [filtered, sortState]);

  const rowsPerPage = cfg.rows_per_page ?? DEFAULT_ROWS_PER_PAGE;
  const totalPages = Math.max(1, Math.ceil(sorted.length / rowsPerPage));
  // 데이터가 줄어들 때 페이지 범위 보정
  const safePage = Math.min(page, totalPages - 1);
  const startIdx = safePage * rowsPerPage;
  const pageRows = sorted.slice(startIdx, startIdx + rowsPerPage);

  // ---- 열 경계 드래그 리사이즈(미리보기 전용) ----
  const canResize = typeof onColumnsChange === 'function';
  const headRowRef = useRef<HTMLTableRowElement>(null);
  // 드래그 중에만 존재하는 상태. 시작 시점의 실제 렌더 폭(px)을 캡처해 두고, 이후에는
  // 그 스냅샷 + 이동량으로만 계산한다 — 매 프레임 DOM 을 다시 재면 리렌더와 얽혀 떨린다.
  const dragRef = useRef<{ index: number; startX: number; widths: number[] } | null>(null);

  const beginResize = (index: number, clientX: number): void => {
    const row = headRowRef.current;
    if (!row || !onColumnsChange) return;
    const cells = Array.from(row.querySelectorAll('th'));
    const widths = cells.map((c) => c.getBoundingClientRect().width);
    // 폭을 잴 수 없는 환경(레이아웃 전)에서는 드래그를 시작하지 않는다 — 0 으로
    // 시작하면 모든 열이 최소 폭으로 붕괴한다.
    if (widths.some((w) => !(w > 0))) return;
    dragRef.current = { index, startX: clientX, widths };
  };

  const moveResize = (clientX: number): void => {
    const d = dragRef.current;
    if (!d || !onColumnsChange) return;
    const next = resizeColumnWidths(d.widths, d.index, clientX - d.startX);
    onColumnsChange(applyColumnWidths(columns, next));
  };

  const endResize = (): void => {
    dragRef.current = null;
  };

  // 포인터가 손잡이 밖으로 나가도 드래그가 이어지도록 document 에 붙인다.
  useEffect(() => {
    if (!canResize) return;
    const onMove = (e: PointerEvent) => {
      if (dragRef.current) moveResize(e.clientX);
    };
    const onUp = () => endResize();
    document.addEventListener('pointermove', onMove);
    document.addEventListener('pointerup', onUp);
    return () => {
      document.removeEventListener('pointermove', onMove);
      document.removeEventListener('pointerup', onUp);
    };
  });

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
        {sorted.length}건
      </div>

      <div className="min-h-0 flex-1 overflow-auto">
        <table
          className={clsx('w-full text-left text-xs', widths && 'table-fixed')}
          data-testid="chart-table"
        >
          {/* 폭 비율. 지정된 열이 하나도 없으면 colgroup 자체를 렌더하지 않는다(자동 폭 유지). */}
          {widths && (
            <colgroup>
              {columns.map((col, i) => (
                <col key={col.field} style={widths[i] ? { width: widths[i] } : undefined} />
              ))}
            </colgroup>
          )}
          <thead className="sticky top-0 bg-(--color-bg-surface)">
            <tr ref={headRowRef}>
              {columns.map((col, i) => {
                const isSorted = sortState?.field === col.field;
                const sortable = col.sortable !== false;
                return (
                  <th
                    key={col.field}
                    onClick={sortable ? () => handleHeaderClick(col.field) : undefined}
                    className={clsx(
                      'relative select-none border-b border-(--color-border-default) px-2 py-1.5',
                      'text-(--color-text-muted)',
                      sortable && 'cursor-pointer hover:text-(--color-text-primary)',
                    )}
                    data-testid={`table-header-${col.field}`}
                    data-sort-order={isSorted ? sortState!.order : 'none'}
                    data-sortable={sortable ? 'true' : 'false'}
                  >
                    <span className="inline-flex items-center gap-1">
                      {col.header}
                      {isSorted && sortState!.order === 'asc' && (
                        <ChevronUp className="h-3 w-3" />
                      )}
                      {isSorted && sortState!.order === 'desc' && (
                        <ChevronDown className="h-3 w-3" />
                      )}
                      {col.filterable === true && (
                        // 헤더 정렬 토글과 겹치지 않도록 클릭을 여기서 멈춘다.
                        <span
                          onClick={(e) => e.stopPropagation()}
                          data-testid={`table-filter-${col.field}`}
                        >
                          {isRangeFilterColumn(col) ? (
                            // 수치/시각 열은 값이 행마다 거의 다 달라 목록으로 고를 수
                            // 없다 — 하한/상한으로 거른다.
                            <TableRangeFilterButton
                              label={col.header}
                              format={col.format}
                              filter={filters[col.field]}
                              onChange={(next) => {
                                setFilters((prev) => ({ ...prev, [col.field]: next }));
                                setPage(0);
                              }}
                              t={t}
                            />
                          ) : (
                            <ColumnFilterButton
                              label={col.header}
                              uniqueValues={uniqueValues.get(col.field) ?? []}
                              filter={filters[col.field]}
                              onChange={(next) => {
                                setFilters((prev) => ({
                                  ...prev,
                                  [col.field]: { ...prev[col.field], ...next },
                                }));
                                setPage(0);
                              }}
                              t={t}
                            />
                          )}
                        </span>
                      )}
                    </span>
                    {/* 열 경계 드래그 손잡이. 마지막 열 뒤에는 주고받을 상대가 없어 두지 않는다. */}
                    {canResize && i < columns.length - 1 && (
                      <span
                        role="separator"
                        aria-orientation="vertical"
                        aria-label={t('dashboard.chart.resizeColumnAria').replace(
                          '{header}',
                          col.header,
                        )}
                        data-testid={`table-resize-${col.field}`}
                        onPointerDown={(e) => {
                          e.preventDefault();
                          e.stopPropagation(); // 헤더 정렬 토글과 겹치지 않게.
                          beginResize(i, e.clientX);
                        }}
                        className="absolute right-0 top-0 h-full w-1.5 cursor-col-resize select-none hover:bg-blue-500/40"
                      />
                    )}
                  </th>
                );
              })}
            </tr>
          </thead>
          <tbody>
            {pageRows.length === 0 ? (
              <tr>
                <td
                  colSpan={columns.length}
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
                  {columns.map((col) => {
                    const v = resolveCellValue(entry, col.field);
                    return (
                      <td
                        key={col.field}
                        className="px-2 py-1 tabular-nums text-(--color-text-primary)"
                      >
                        {formatCell(v, col.format, decimals, col.unit)}
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
