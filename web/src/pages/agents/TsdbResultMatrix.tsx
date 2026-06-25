// 시리즈 쿼리 결과 매트릭스 렌더러.
// 시리즈 키를 컬럼으로, 시간 버킷을 행으로 표시한다.
// 누락된 버킷×시리즈 교차점은 em-dash(—) 로 표시한다.
//
// SPEC-WEB-005 v0.2.0 에서 `SeriesMatrix` (columns + rows 형태) 를 직접
// 받도록 리팩터되었다.
//
// SPEC-WEB-005 v0.3.0 Wave 2 UI/UX 개선:
//   - 헤더 우측에 "CSV 내보내기" 버튼 추가 (rows.length > 0 일 때만 노출).
//
// SPEC-WEB-005 v0.4.0 UI/UX 개선:
//   - 평균 집계일 때 셀/CSV 값에 사용자 지정 소수점 자릿수 적용.
//   - 행 수가 많아도 명시적인 페이지네이션(10/25/50/100)으로 일정한 응답성을 보장.
//     기존 react-window 가상 스크롤은 페이지 크기 상한이 100 이라 불필요해 제거.
//
// @spec SPEC-WEB-005

import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Download,
  LineChart as LineChartIcon,
  Table as TableIcon,
} from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { formatLocalTimestamp } from '@/services/api/tsdb';
import type { SeriesMatrix, SeriesMatrixQuery } from '@/services/api/seriesDataSource';

import { downloadSeriesMatrixCsv } from './tsdbCsvExport';
import TsdbResultChart from './TsdbResultChart';

/** 결과 표시 모드. 사용자가 헤더 토글로 전환한다. */
type ResultViewMode = 'table' | 'chart';

/** 페이지 크기 옵션. 기본값은 25. */
const PAGE_SIZE_OPTIONS = [10, 25, 50, 100] as const;
type PageSize = (typeof PAGE_SIZE_OPTIONS)[number];
const DEFAULT_PAGE_SIZE: PageSize = 25;

interface SeriesResultMatrixProps {
  /** 쿼리 응답 — 이미 컬럼/행으로 pivot 된 매트릭스. */
  matrix: SeriesMatrix;
  /** 누락 셀 대체 문자. 기본 "—". */
  emptyCellPlaceholder?: string;
  /**
   * CSV 파일명 구성에 사용할 에이전트 이름. 미지정 시 `series` 로 대체된다.
   * 모달 상위 컴포넌트가 TSDB/Store 에이전트 이름을 주입한다.
   */
  agentName?: string;
  /** CSV 파일명 구성에 사용할 범위 시작 (epoch ms). */
  exportStartMs?: number;
  /** CSV 파일명 구성에 사용할 범위 종료 (epoch ms). */
  exportEndMs?: number;
  /**
   * 집계 함수. `'average'` 일 때만 `decimalPrecision` 이 적용된다.
   * 미지정 시 `'average'` 로 가정한다 (기본 동작 보존).
   */
  aggregation?: SeriesMatrixQuery['aggregation'];
  /**
   * 평균 집계 시 표시할 소수점 자릿수 (0-6).
   * 미지정 시 1.
   */
  decimalPrecision?: number;
  /**
   * 표시 모드 (controlled). 미지정 시 내부 state 로 관리된다.
   * 모달 상위에서 mutation 사이클 사이에 모드를 보존하기 위해 사용한다.
   */
  viewMode?: ResultViewMode;
  /** 표시 모드 변경 콜백 (controlled 모드 전용). */
  onViewModeChange?: (next: ResultViewMode) => void;
}

/**
 * 셀 값을 사람이 읽기 좋은 문자열로 변환한다.
 *
 * - `aggregation === 'average'` 이고 값이 유한 수이면 `toFixed(precision)` 적용.
 * - 그 외에는 정수는 그대로, 소수는 최대 4자리까지 toLocaleString.
 *
 * 0-6 범위 밖의 precision 은 안전을 위해 클램프된다.
 *
 * `react-refresh/only-export-components` 규칙을 준수하기 위해 모듈 내부에서만
 * 사용하며 외부로 노출하지 않는다.
 */
function formatMatrixValue(
  value: number,
  aggregation: SeriesMatrixQuery['aggregation'] | undefined,
  precision: number,
): string {
  if (aggregation === 'average' && Number.isFinite(value)) {
    const safe = Math.max(0, Math.min(6, Math.floor(precision)));
    return value.toFixed(safe);
  }
  if (Number.isInteger(value)) {
    return value.toLocaleString();
  }
  return value.toLocaleString(undefined, { maximumFractionDigits: 4 });
}

/**
 * 매트릭스 렌더러 본체.
 * 컬럼이 0개이거나 행이 0개이면 안내 메시지를 렌더링한다.
 */
function SeriesResultMatrixImpl({
  matrix,
  emptyCellPlaceholder = '—',
  agentName,
  exportStartMs,
  exportEndMs,
  aggregation,
  decimalPrecision = 1,
  viewMode: viewModeProp,
  onViewModeChange,
}: SeriesResultMatrixProps) {
  const { t } = useTranslation();
  const { columns, rows } = matrix;

  // 페이지네이션 상태.
  const [pageSize, setPageSize] = useState<PageSize>(DEFAULT_PAGE_SIZE);
  const [page, setPage] = useState<number>(1);

  // 결과 표시 모드.
  // - controlled: 부모가 viewMode prop 으로 제어 (mutation 사이에도 보존).
  // - uncontrolled (기본): 내부 state 사용 (기존 동작 보존).
  const [internalViewMode, setInternalViewMode] = useState<ResultViewMode>('table');
  const viewMode = viewModeProp ?? internalViewMode;
  const setViewMode = useCallback(
    (next: ResultViewMode) => {
      if (onViewModeChange) onViewModeChange(next);
      else setInternalViewMode(next);
    },
    [onViewModeChange],
  );

  // 행 수 변동 시 currentPage 가 totalPages 범위를 넘지 않도록 보정.
  const total = rows.length;
  const totalPages = total === 0 ? 0 : Math.ceil(total / pageSize);
  useEffect(() => {
    if (totalPages === 0) {
      // 결과가 비면 page 를 1로 리셋.
      if (page !== 1) setPage(1);
      return;
    }
    if (page > totalPages) {
      setPage(totalPages);
    }
  }, [page, totalPages]);

  const startIdx = total === 0 ? 0 : (page - 1) * pageSize;
  const endIdx = Math.min(startIdx + pageSize, total);
  const visibleRows = useMemo(
    () => rows.slice(startIdx, endIdx),
    [rows, startIdx, endIdx],
  );

  const handleExportClick = useCallback(() => {
    // CSV 파일명 필드가 일부 빠져도 안전한 기본값으로 대체한다.
    downloadSeriesMatrixCsv(matrix, {
      agentName: agentName ?? 'series',
      startMs: exportStartMs ?? matrix.rows[0]?.bucketStartMs ?? Date.now(),
      endMs:
        exportEndMs ?? matrix.rows[matrix.rows.length - 1]?.bucketStartMs ?? Date.now(),
      aggregation,
      decimalPrecision,
    });
  }, [matrix, agentName, exportStartMs, exportEndMs, aggregation, decimalPrecision]);

  const handlePageSizeChange = useCallback(
    (e: React.ChangeEvent<HTMLSelectElement>) => {
      const next = Number(e.target.value);
      if (PAGE_SIZE_OPTIONS.includes(next as PageSize)) {
        setPageSize(next as PageSize);
        setPage(1);
      }
    },
    [],
  );

  const handlePrev = useCallback(() => {
    setPage((p) => Math.max(1, p - 1));
  }, []);

  const handleNext = useCallback(() => {
    setPage((p) => Math.min(totalPages, p + 1));
  }, [totalPages]);

  if (columns.length === 0) {
    return (
      <div className="p-4 text-center text-sm text-(--color-text-muted)">
        {t('series.noSelectedSeries')}
      </div>
    );
  }

  const hasRows = total > 0;

  // 헤더 (뷰 토글 + CSV 내보내기 버튼 포함) —
  //   rows 0 일 때도 토글은 노출하되, CSV 버튼은 숨긴다.
  const headerBar = (
    <div className="mb-2 flex items-center justify-between gap-2">
      <h4 className="text-xs font-medium text-(--color-text-muted)">
        {hasRows ? t('series.rowCount').replace('{count}', total.toLocaleString()) : t('series.noRows')}
      </h4>
      <div className="flex items-center gap-2">
        {/*
          뷰 모드 토글 — `테이블` 과 `차트` 양 옵션을 항상 노출하여 데이터가
          비어있어도 사용자가 모드를 미리 선택해둘 수 있게 한다.
        */}
        <div
          role="tablist"
          aria-label={t('series.resultViewMode')}
          className="inline-flex rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) p-0.5"
        >
          {(['table', 'chart'] as const).map((mode) => {
            const selected = viewMode === mode;
            const Icon = mode === 'table' ? TableIcon : LineChartIcon;
            const label = mode === 'table' ? t('series.viewTable') : t('series.viewChart');
            return (
              <button
                key={mode}
                type="button"
                role="tab"
                aria-selected={selected}
                data-testid={`tsdb-result-view-${mode}`}
                onClick={() => setViewMode(mode)}
                className={`inline-flex items-center gap-1 rounded px-2.5 py-0.5 text-xs font-medium transition-colors ${
                  selected
                    ? 'bg-blue-600 text-white'
                    : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)'
                }`}
              >
                <Icon className="h-3 w-3" aria-hidden="true" />
                {label}
              </button>
            );
          })}
        </div>
        {hasRows && (
          <button
            type="button"
            onClick={handleExportClick}
            className="inline-flex items-center gap-1.5 rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-1 text-xs font-medium text-(--color-text-primary) transition-colors hover:bg-(--color-bg-elevated)"
            data-testid="tsdb-result-csv-export"
          >
            <Download className="h-3.5 w-3.5" aria-hidden="true" />
            {t('series.csvExport')}
          </button>
        )}
      </div>
    </div>
  );

  if (!hasRows) {
    return (
      <div>
        {headerBar}
        <div className="p-4 text-center text-sm text-(--color-text-muted)">
          {t('series.emptyResult')}
        </div>
      </div>
    );
  }

  // 차트 뷰 — 페이지네이션은 차트에 의미가 없으므로 노출하지 않는다.
  if (viewMode === 'chart') {
    return (
      <div>
        {headerBar}
        <TsdbResultChart
          matrix={matrix}
          aggregation={aggregation}
          decimalPrecision={decimalPrecision}
        />
      </div>
    );
  }

  return (
    <div>
      {headerBar}
      <div className="overflow-hidden rounded-md border border-(--color-border-default)">
        <div className="max-h-[60vh] overflow-auto">
          <table className="min-w-full border-collapse text-xs" role="table">
            <thead className="sticky top-0 bg-(--color-bg-primary)">
              <tr>
                <th
                  scope="col"
                  className="sticky left-0 z-10 border-b border-r border-(--color-border-default) bg-(--color-bg-primary) px-3 py-2 text-left font-medium text-(--color-text-muted)"
                >
                  {t('series.timestampLocal')}
                </th>
                {columns.map((k) => (
                  <th
                    key={k}
                    scope="col"
                    className="whitespace-nowrap border-b border-(--color-border-default) px-3 py-2 text-right font-mono font-medium text-(--color-text-muted)"
                  >
                    {k}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {visibleRows.map((row) => (
                <tr key={row.bucketStartMs} className="hover:bg-(--color-bg-elevated)">
                  <th
                    scope="row"
                    className="sticky left-0 whitespace-nowrap border-b border-r border-(--color-border-default) bg-(--color-bg-surface) px-3 py-1.5 text-left font-mono text-(--color-text-secondary)"
                  >
                    {formatLocalTimestamp(row.bucketStartMs)}
                  </th>
                  {row.values.map((value, colIdx) => {
                    // 동일 버킷의 컬럼은 columns 배열 인덱스로 안정적인 key 를 만든다.
                    // 컬럼 순서는 SeriesMatrix 계약에 의해 렌더 간 고정되므로 index key 안전.
                    const columnKey = columns[colIdx] ?? `col-${colIdx}`;
                    return (
                      <td
                        key={columnKey}
                        className="whitespace-nowrap border-b border-(--color-border-default) px-3 py-1.5 text-right font-mono text-(--color-text-primary)"
                      >
                        {value === null
                          ? emptyCellPlaceholder
                          : formatMatrixValue(value, aggregation, decimalPrecision)}
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {/*
          페이지네이션 컨트롤 — 매트릭스 컨테이너 내부에 두어 시각적으로
          테이블과 한 묶음으로 보이게 한다. 푸터 sticky 처리는 하지 않는다.
        */}
        <div
          className="flex flex-wrap items-center justify-between gap-2 border-t border-(--color-border-default) bg-(--color-bg-surface) px-3 py-2 text-xs text-(--color-text-secondary)"
          data-testid="tsdb-result-pagination"
        >
          <div className="flex items-center gap-2">
            <label
              htmlFor="tsdb-page-size"
              className="inline-flex items-center gap-1.5"
            >
              <span>{t('series.pageSize')}</span>
              <select
                id="tsdb-page-size"
                data-testid="tsdb-page-size"
                value={pageSize}
                onChange={handlePageSizeChange}
                className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-0.5 text-xs text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
              >
                {PAGE_SIZE_OPTIONS.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <div className="flex items-center gap-2">
            <span data-testid="tsdb-page-range">
              {(startIdx + 1).toLocaleString()}-{endIdx.toLocaleString()} /{' '}
              {t('series.rowsTotal').replace('{count}', total.toLocaleString())}
            </span>
            <button
              type="button"
              onClick={handlePrev}
              disabled={page <= 1}
              data-testid="tsdb-page-prev"
              className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-0.5 text-xs font-medium text-(--color-text-primary) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-50"
            >
              {t('agents.detail.store.prev')}
            </button>
            <span data-testid="tsdb-page-indicator">
              {page} / {Math.max(1, totalPages)}
            </span>
            <button
              type="button"
              onClick={handleNext}
              disabled={page >= totalPages}
              data-testid="tsdb-page-next"
              className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-0.5 text-xs font-medium text-(--color-text-primary) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-50"
            >
              {t('agents.detail.store.next')}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

// ---- Public exports ----

/** TSDB/Store 공용 매트릭스 렌더러 (명시적 명칭). */
export const SeriesResultMatrix = SeriesResultMatrixImpl;

/**
 * 기존 콜사이트 호환을 위한 default export.
 * SPEC-WEB-005 v0.2.0 이전에는 `keys` + `TsdbQueryResponse` 를 받았으나,
 * 현재는 pivot 된 `SeriesMatrix` 를 받는다.
 */
export default SeriesResultMatrixImpl;
