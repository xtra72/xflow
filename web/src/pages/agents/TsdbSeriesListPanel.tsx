// TSDB/Store 에이전트 상세에서 표시되는 시리즈 목록 패널.
// 페이지 크기 선택 / 페이지 이동을 제공한다.
//
// SPEC-WEB-005 v0.3.0 에서 행별 "데이터 보기" 버튼이 제거되었고,
// 데이터 뷰어는 상위 `SeriesTab` 의 단일 트리거 버튼으로 통합되었다.
// 본 패널은 순수 정보 표시 용도로만 사용된다.
//
// @spec SPEC-WEB-005

import { useState } from 'react';
import { ChevronLeft, ChevronRight, Database } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import type { SeriesDataSource } from '@/services/api/seriesDataSource';

/** 페이지 크기 옵션. 기본값은 10. */
const PAGE_SIZE_OPTIONS = [10, 25, 50, 100] as const;
type PageSize = (typeof PAGE_SIZE_OPTIONS)[number];

interface SeriesListPanelProps {
  /** TSDB/Store 공용 데이터 소스. 부모가 kind/agent 에 맞춰 생성한다. */
  dataSource: SeriesDataSource;
}

function SeriesListPanelImpl({ dataSource }: SeriesListPanelProps) {
  const { t } = useTranslation();
  // 페이지네이션 상태 — 기본 페이지 크기는 10.
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState<PageSize>(10);

  const { data, isLoading, isError, refetch } = dataSource.useKeys({
    page,
    size: pageSize,
  });

  const series = data?.keys ?? [];
  const pagination = data?.pagination;
  const total = pagination?.total ?? 0;
  const totalPages = pagination?.totalPages ?? 0;
  const isEmpty = !isLoading && !isError && total === 0 && series.length === 0;

  const handlePageSizeChange = (size: PageSize) => {
    setPageSize(size);
    setPage(1); // 페이지 크기 변경 시 1페이지로 리셋
  };

  const canGoPrev = page > 1;
  const canGoNext = page < totalPages;

  return (
    <div className="space-y-3 p-4">
      {/* 헤더: 카운트 + 페이지 크기 셀렉터 */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2 text-xs text-(--color-text-muted)">
          <Database className="h-3.5 w-3.5" aria-hidden="true" />
          {isLoading ? (
            <span>{t('series.loadingSeries')}</span>
          ) : (
            // 총 개수는 강조 span 으로 분리 렌더 — {count} 슬롯 기준으로 좌우 텍스트를 나눈다.
            (() => {
              const [before, after] = t('series.seriesCount').split('{count}');
              return (
                <span>
                  {before}
                  <span className="font-semibold text-(--color-text-primary)">
                    {total}
                  </span>
                  {after}
                </span>
              );
            })()
          )}
        </div>
        <div className="flex items-center gap-2 text-xs text-(--color-text-muted)">
          <label htmlFor="tsdb-series-page-size">{t('series.perPage')}</label>
          <select
            id="tsdb-series-page-size"
            value={pageSize}
            onChange={(e) => handlePageSizeChange(Number(e.target.value) as PageSize)}
            disabled={isEmpty}
            className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1 text-xs text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {PAGE_SIZE_OPTIONS.map((size) => (
              <option key={size} value={size}>
                {size}
              </option>
            ))}
          </select>
        </div>
      </div>

      {/* 에러 상태 */}
      {isError && (
        <div className="rounded-md border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400">
          <p>{t('series.loadError')}</p>
          <button
            type="button"
            onClick={() => refetch()}
            className="mt-2 rounded-md bg-red-600 px-3 py-1 text-xs font-medium text-white hover:bg-red-700 dark:bg-red-500"
          >
            {t('series.retry')}
          </button>
        </div>
      )}

      {/* 로딩 상태 */}
      {isLoading && !data && (
        <div className="space-y-2">
          {Array.from({ length: 5 }).map((_, i) => (
            <div
              key={`skeleton-${i}`}
              data-testid="tsdb-series-skeleton"
              className="h-10 animate-pulse rounded bg-(--color-bg-elevated)"
            />
          ))}
        </div>
      )}

      {/* 빈 상태 */}
      {isEmpty && (
        <div className="rounded-md border border-(--color-border-default) bg-(--color-bg-surface) py-10 text-center">
          <Database className="mx-auto h-8 w-8 text-gray-300 dark:text-gray-600" />
          <p className="mt-2 text-sm text-(--color-text-muted)">{t('series.noStoredSeries')}</p>
        </div>
      )}

      {/* 시리즈 테이블 — 정보 표시 전용 (행 액션 없음) */}
      {!isLoading && !isError && series.length > 0 && (
        <div className="overflow-x-auto rounded-md border border-(--color-border-default)">
          <table className="min-w-full divide-y divide-(--color-border-default) text-sm">
            <thead className="bg-(--color-bg-primary)">
              <tr>
                <th
                  scope="col"
                  className="px-4 py-2 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                >
                  {t('series.seriesKey')}
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-(--color-border-default) bg-(--color-bg-surface)">
              {series.map((key) => (
                <tr key={key} className="hover:bg-(--color-bg-elevated)">
                  <td className="whitespace-nowrap px-4 py-2 font-mono text-xs text-(--color-text-primary)">
                    {key}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* 페이지네이션 컨트롤 */}
      {!isLoading && !isError && totalPages > 0 && (
        <div className="flex items-center justify-between text-xs text-(--color-text-muted)">
          <span>
            {t('series.pageIndicator').replace('{page}', String(page)).replace('{total}', String(totalPages))}
          </span>
          <div className="flex items-center gap-1">
            <button
              type="button"
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={!canGoPrev}
              aria-label={t('series.prevPage')}
              className="rounded-md border border-(--color-border-strong) p-1.5 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-40"
            >
              <ChevronLeft className="h-4 w-4" />
            </button>
            <button
              type="button"
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={!canGoNext}
              aria-label={t('series.nextPage')}
              className="rounded-md border border-(--color-border-strong) p-1.5 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-40"
            >
              <ChevronRight className="h-4 w-4" />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

// ---- Public exports ----

/** TSDB/Store 공용 시리즈 목록 패널 (명시적 명칭). */
export const SeriesListPanel = SeriesListPanelImpl;

/** 기존 콜사이트 호환을 위한 default export. */
export default SeriesListPanelImpl;
