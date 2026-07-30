// 클라이언트 사이드 목록 페이지네이션 컨트롤 (공용).
//
// AgentListPage 의 페이지네이션 idiom(페이지 크기 select + "N-M / 총 T" 범위 라벨 +
// 이전/다음 버튼)을 공유 컴포넌트로 추출한 것. 슬라이스 자체는 부모가 담당하고
// (page/pageSize state 소유 + sortedItems.slice), 이 컴포넌트는 컨트롤만 렌더한다.
// BulkRegisterPanel 추출과 동일한 DRY 방식 — 최소 컴포넌트.

import { ChevronLeft, ChevronRight } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';

/** 기본 페이지 크기 옵션. */
const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100];

interface TablePaginationProps {
  /** 현재 페이지(1-base). */
  page: number;
  /** 페이지 크기. */
  pageSize: number;
  /** 정렬/필터가 적용된 전체 항목 수. */
  totalItems: number;
  /** 페이지 이동 콜백(clamp 된 1-base 페이지). */
  onPageChange: (page: number) => void;
  /** 페이지 크기 변경 콜백. */
  onPageSizeChange: (size: number) => void;
  /** 페이지 크기 옵션(기본 10/25/50/100). */
  pageSizeOptions?: number[];
}

/** 목록 페이지네이션 컨트롤. totalPages/safePage/startIndex 는 props 로부터 자체 계산한다. */
export default function TablePagination({
  page,
  pageSize,
  totalItems,
  onPageChange,
  onPageSizeChange,
  pageSizeOptions = DEFAULT_PAGE_SIZE_OPTIONS,
}: TablePaginationProps) {
  const { t } = useTranslation();

  const totalPages = Math.max(1, Math.ceil(totalItems / pageSize));
  const safePage = Math.min(page, totalPages);
  const startIndex = (safePage - 1) * pageSize;

  return (
    <div className="flex items-center justify-between">
      {/* 페이지 크기 선택 + 범위 라벨 */}
      <div className="flex items-center gap-2 text-sm text-(--color-text-muted)">
        <span>{t('common.pagination.perPage')}</span>
        <select
          value={pageSize}
          onChange={(e) => onPageSizeChange(Number(e.target.value))}
          aria-label={t('common.pagination.perPage')}
          className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
        >
          {pageSizeOptions.map((size) => (
            <option key={size} value={size}>
              {size}
            </option>
          ))}
        </select>
        <span>{t('common.pagination.unit')}</span>
        <span className="ml-2 text-gray-400">|</span>
        <span className="ml-2">
          {t('common.pagination.range')
            .replace('{total}', String(totalItems))
            .replace('{start}', String(totalItems === 0 ? 0 : startIndex + 1))
            .replace('{end}', String(Math.min(startIndex + pageSize, totalItems)))}
        </span>
      </div>

      {/* 페이지 이동 버튼 */}
      <div className="flex items-center gap-1">
        <button
          type="button"
          disabled={safePage <= 1}
          onClick={() => onPageChange(Math.max(1, safePage - 1))}
          className="rounded-md border border-(--color-border-strong) p-1.5 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-40"
          aria-label={t('common.pagination.prev')}
        >
          <ChevronLeft className="h-4 w-4" />
        </button>
        <span className="px-3 text-sm text-(--color-text-muted)">
          {safePage} / {totalPages}
        </span>
        <button
          type="button"
          disabled={safePage >= totalPages}
          onClick={() => onPageChange(Math.min(totalPages, safePage + 1))}
          className="rounded-md border border-(--color-border-strong) p-1.5 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-40"
          aria-label={t('common.pagination.next')}
        >
          <ChevronRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}
