// 실시간 로그 뷰어 (페이지네이션 방식).
//
// 정렬은 시간 기준 하나뿐이며(오름차순/내림차순), 필터는 모두 컬럼 헤더에 있다:
//   - 시간: 구간(시작~종료, 자정을 넘는 구간도 지원)
//   - 레벨 / 소스 / 타입 / 이름: 다중 선택
//   - 메시지: 필터 없음
//
// 정렬·필터의 순수 로직은 logFilters.ts 가 갖는다.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  ArrowDown,
  ArrowUp,
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
  Filter,
} from 'lucide-react';
import { useTranslation } from '@/lib/i18n';

import { isLinkableLogComponent } from './logNavigation';
import {
  ALL_LEVELS,
  ALL_SOURCES,
  collectOptions,
  emptyFilterState,
  filterLogs,
  hasAnyFilter,
  sortLogs,
  type LogFilterState,
  type SortDir,
} from './logFilters';

/** 로그 레벨 타입 */
export type LogLevel = 'DEBUG' | 'INFO' | 'WARN' | 'ERROR';

/** 소스 카테고리 타입 */
export type SourceType = 'agent' | 'node' | 'flow' | 'api' | 'engine' | 'system';

/** 단일 로그 항목 */
export interface LogEntry {
  id: string;
  /** 표시용 시각 "HH:MM:SS" */
  timestamp: string;
  /**
   * 수신 시각(epoch ms). 정렬과 구간 필터가 쓴다.
   *
   * 표시용 `timestamp` 는 날짜를 버린 문자열이라 정렬 기준으로 삼기 어렵다.
   * 선택 필드로 둔 것은 이 값을 채우지 않는 기존 호출부·픽스처를 깨지 않기 위함이며,
   * 없을 때는 수신 순서를 시간 순서로 본다(logFilters.sortLogs).
   */
  ts?: number;
  level: LogLevel;
  message: string;
  component?: string;
  source?: string;
  componentKind?: string;
  componentName?: string;
}

// 페이지 크기 옵션
const PAGE_SIZE_OPTIONS = [50, 100, 200] as const;

/** 레벨별 뱃지 스타일 */
const LEVEL_STYLES: Record<LogLevel, string> = {
  DEBUG: 'bg-(--color-bg-sunken) text-(--color-text-secondary)',
  INFO: 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300',
  WARN: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900 dark:text-yellow-300',
  ERROR: 'bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300',
};

/** 소스별 뱃지 스타일 */
const SOURCE_STYLES: Record<SourceType, string> = {
  agent: 'bg-purple-100 text-purple-700 dark:bg-purple-900 dark:text-purple-300',
  node: 'bg-teal-100 text-teal-700 dark:bg-teal-900 dark:text-teal-300',
  flow: 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300',
  api: 'bg-orange-100 text-orange-700 dark:bg-orange-900 dark:text-orange-300',
  engine: 'bg-indigo-100 text-indigo-700 dark:bg-indigo-900 dark:text-indigo-300',
  system: 'bg-(--color-bg-sunken) text-(--color-text-secondary)',
};

/** 컬럼 폭 — 헤더와 본문이 같은 값을 써야 열이 어긋나지 않는다. */
const COL = {
  time: 'w-[150px]',
  level: 'w-[86px]',
  source: 'w-[86px]',
  kind: 'w-[112px]',
  name: 'w-[112px]',
} as const;

interface LogViewerProps {
  entries: LogEntry[];
  /**
   * 헤더 제목 (미지정 시 기본 "시스템 로그").
   * 모니터링 보드가 레벨별 뷰어를 여러 개 배치할 때 서로를 구분하기 위해 쓴다.
   */
  title?: string;
  /**
   * 이름 칸 클릭 시 호출 — 에이전트/노드/플로우 메뉴로 이동한다.
   *
   * 라우터 훅을 이 컴포넌트가 직접 쓰지 않는 이유는, 라우터 밖에서 단독으로
   * 렌더되는 경우(테스트·프리뷰)에도 그대로 동작해야 하기 때문이다. 핸들러가
   * 없으면 이름은 지금처럼 평범한 텍스트로 남는다.
   */
  onSelectComponent?: (source: string, name: string) => void;
}

/** 로그 뷰어 컴포넌트 (페이지네이션) */
export default function LogViewer({ entries, title, onSelectComponent }: LogViewerProps) {
  const { t } = useTranslation();
  const [filters, setFilters] = useState<LogFilterState>(emptyFilterState);
  const [sortDir, setSortDir] = useState<SortDir>('asc');
  const [autoScroll, setAutoScroll] = useState(true);
  const [pageSize, setPageSize] = useState<number>(100);
  const [currentPage, setCurrentPage] = useState(1);

  /** 선택형 컬럼 필터 토글 */
  const toggleSelection = useCallback(
    (key: 'levels' | 'sources' | 'kinds' | 'names', value: string) => {
      setFilters((prev) => {
        const next = new Set(prev[key]);
        if (next.has(value)) next.delete(value);
        else next.add(value);
        return { ...prev, [key]: next };
      });
    },
    [],
  );

  /** 선택형 컬럼 필터 해제(전체 보기) */
  const clearSelection = useCallback((key: 'levels' | 'sources' | 'kinds' | 'names') => {
    setFilters((prev) => ({ ...prev, [key]: new Set<string>() }));
  }, []);

  /** 시간 구간 변경 */
  const setTimeRange = useCallback((from: string, to: string) => {
    setFilters((prev) => ({ ...prev, timeFrom: from, timeTo: to }));
  }, []);

  const toggleSort = useCallback(() => {
    setSortDir((prev) => (prev === 'asc' ? 'desc' : 'asc'));
  }, []);

  // 타입·이름 옵션은 현재 버퍼에서 뽑는다(값 집합이 런타임에 정해진다).
  const kindOptions = useMemo(() => collectOptions(entries, (e) => e.componentKind), [entries]);
  const nameOptions = useMemo(() => collectOptions(entries, (e) => e.componentName), [entries]);

  // 필터 → 정렬 순서. 정렬을 먼저 하면 버린 줄까지 정렬하게 된다.
  const visible = useMemo(
    () => sortLogs(filterLogs(entries, filters), sortDir),
    [entries, filters, sortDir],
  );

  // 페이지 계산
  const totalPages = Math.max(1, Math.ceil(visible.length / pageSize));

  // 최신 로그가 있는 페이지 — 오름차순이면 마지막, 내림차순이면 첫 페이지다.
  const newestPage = sortDir === 'asc' ? totalPages : 1;

  // 필터/정렬 변경 시 첫 페이지로 이동
  useEffect(() => {
    setCurrentPage(1);
  }, [filters, sortDir, pageSize]);

  // 자동 스크롤: 새 로그 수신 시 최신 페이지로 이동
  useEffect(() => {
    if (autoScroll) setCurrentPage(newestPage);
  }, [autoScroll, newestPage]);

  // 현재 페이지의 로그 항목
  const pageEntries = useMemo(() => {
    const start = (currentPage - 1) * pageSize;
    return visible.slice(start, start + pageSize);
  }, [visible, currentPage, pageSize]);

  // 페이지 이동
  const goToPage = useCallback((page: number) => {
    setAutoScroll(false);
    setCurrentPage(page);
  }, []);

  const goFirst = useCallback(() => goToPage(1), [goToPage]);
  const goPrev = useCallback(() => goToPage(Math.max(1, currentPage - 1)), [goToPage, currentPage]);
  const goNext = useCallback(() => goToPage(Math.min(totalPages, currentPage + 1)), [goToPage, currentPage, totalPages]);
  const goLast = useCallback(() => goToPage(totalPages), [goToPage, totalPages]);

  // 자동 스크롤 토글
  const toggleAutoScroll = useCallback(() => {
    setAutoScroll((prev) => {
      if (!prev) setCurrentPage(newestPage);
      return !prev;
    });
  }, [newestPage]);

  // 페이지 번호 목록 생성 (최대 5개)
  const pageNumbers = useMemo(() => {
    const pages: number[] = [];
    let start = Math.max(1, currentPage - 2);
    const end = Math.min(totalPages, start + 4);
    start = Math.max(1, end - 4);
    for (let i = start; i <= end; i++) {
      pages.push(i);
    }
    return pages;
  }, [currentPage, totalPages]);

  const filtered = hasAnyFilter(filters);

  return (
    <div className="bg-(--color-bg-surface) rounded-lg shadow">
      {/* 타이틀 + 건수 */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-(--color-border-default)">
        <h3 className="text-sm font-semibold text-(--color-text-primary)">
          {title ?? t('monitoring.systemLog')}
        </h3>
        <div className="flex items-center gap-2">
          {filtered && (
            <button
              type="button"
              onClick={() => setFilters(emptyFilterState())}
              data-testid="log-clear-all-filters"
              className="rounded px-1.5 py-0.5 text-[10px] font-medium text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-primary)"
            >
              {t('monitoring.clearFilters')}
            </button>
          )}
          <span className="text-xs text-(--color-text-muted)">
            {visible.length === entries.length
              ? `${entries.length.toLocaleString()}${t('monitoring.countUnit')}`
              : `${visible.length.toLocaleString()} / ${entries.length.toLocaleString()}${t('monitoring.countUnit')}`}
          </span>
        </div>
      </div>

      {/* 페이지네이션 + 표시 옵션 */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-(--color-border-default) bg-(--color-bg-sunken)">
        <div className="flex items-center gap-3">
          {/* 페이지 크기 선택 */}
          <div className="flex items-center gap-1.5">
            <span className="text-[10px] text-(--color-text-muted)">{t('monitoring.display')}</span>
            {PAGE_SIZE_OPTIONS.map((size) => (
              <button
                key={size}
                type="button"
                onClick={() => setPageSize(size)}
                className={`px-1.5 py-0.5 text-[10px] rounded font-medium transition-colors ${
                  pageSize === size
                    ? 'bg-gray-900 text-white dark:bg-white dark:text-gray-900'
                    : 'text-(--color-text-muted) hover:bg-(--color-bg-elevated)'
                }`}
              >
                {size}
              </button>
            ))}
          </div>

          {/* 자동 스크롤 */}
          <button
            type="button"
            onClick={toggleAutoScroll}
            data-testid="log-autoscroll-toggle"
            className={`px-2 py-0.5 text-[10px] rounded font-medium transition-colors ${
              autoScroll
                ? 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300'
                : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)'
            }`}
          >
            {t('monitoring.realtime')} {autoScroll ? 'ON' : 'OFF'}
          </button>
        </div>

        {/* 페이지 네비게이션 */}
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={goFirst}
            disabled={currentPage <= 1}
            className="p-0.5 rounded text-(--color-text-muted) hover:bg-(--color-bg-elevated) disabled:opacity-30 disabled:cursor-not-allowed"
          >
            <ChevronsLeft className="w-3.5 h-3.5" />
          </button>
          <button
            type="button"
            onClick={goPrev}
            disabled={currentPage <= 1}
            className="p-0.5 rounded text-(--color-text-muted) hover:bg-(--color-bg-elevated) disabled:opacity-30 disabled:cursor-not-allowed"
          >
            <ChevronLeft className="w-3.5 h-3.5" />
          </button>

          {pageNumbers.map((page) => (
            <button
              key={page}
              type="button"
              onClick={() => goToPage(page)}
              className={`min-w-[24px] px-1 py-0.5 text-[10px] rounded font-medium transition-colors ${
                page === currentPage
                  ? 'bg-blue-600 text-white dark:bg-blue-500'
                  : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)'
              }`}
            >
              {page}
            </button>
          ))}

          <button
            type="button"
            onClick={goNext}
            disabled={currentPage >= totalPages}
            className="p-0.5 rounded text-(--color-text-muted) hover:bg-(--color-bg-elevated) disabled:opacity-30 disabled:cursor-not-allowed"
          >
            <ChevronRight className="w-3.5 h-3.5" />
          </button>
          <button
            type="button"
            onClick={goLast}
            disabled={currentPage >= totalPages}
            className="p-0.5 rounded text-(--color-text-muted) hover:bg-(--color-bg-elevated) disabled:opacity-30 disabled:cursor-not-allowed"
          >
            <ChevronsRight className="w-3.5 h-3.5" />
          </button>

          <span className="ml-1 text-[10px] text-(--color-text-muted)">
            {currentPage} / {totalPages}
          </span>
        </div>
      </div>

      {/* 테이블 헤더 — 정렬(시간)과 컬럼별 필터가 모두 여기에 있다. */}
      <div className="flex items-center gap-2 px-3 py-1 border-b border-(--color-border-default) bg-(--color-bg-sunken) font-mono text-[10px] font-semibold text-(--color-text-muted) uppercase tracking-wider">
        <div className={`shrink-0 ${COL.time} flex items-center gap-1`}>
          {/* 정렬은 시간 기준 하나뿐이므로 토글 버튼도 이 컬럼에만 있다. */}
          <button
            type="button"
            onClick={toggleSort}
            data-testid="log-sort-time"
            aria-label={`${t('monitoring.colTime')} ${t(
              sortDir === 'asc' ? 'monitoring.sortAsc' : 'monitoring.sortDesc',
            )}`}
            className="flex items-center gap-0.5 rounded px-1 py-0.5 uppercase tracking-wider transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-primary)"
          >
            {t('monitoring.colTime')}
            {sortDir === 'asc' ? (
              <ArrowUp className="h-3 w-3" />
            ) : (
              <ArrowDown className="h-3 w-3" />
            )}
          </button>
          <TimeRangeFilter
            from={filters.timeFrom}
            to={filters.timeTo}
            onChange={setTimeRange}
          />
        </div>

        <ColumnSelectFilter
          className={`shrink-0 ${COL.level}`}
          label={t('monitoring.colLevel')}
          options={ALL_LEVELS}
          selected={filters.levels}
          testId="level"
          onToggle={(v) => toggleSelection('levels', v)}
          onClear={() => clearSelection('levels')}
        />
        <ColumnSelectFilter
          className={`shrink-0 ${COL.source}`}
          label={t('monitoring.colSource')}
          options={ALL_SOURCES}
          selected={filters.sources}
          testId="source"
          onToggle={(v) => toggleSelection('sources', v)}
          onClear={() => clearSelection('sources')}
        />
        <ColumnSelectFilter
          className={`shrink-0 ${COL.kind}`}
          label={t('monitoring.colType')}
          options={kindOptions}
          selected={filters.kinds}
          testId="kind"
          onToggle={(v) => toggleSelection('kinds', v)}
          onClear={() => clearSelection('kinds')}
        />
        <ColumnSelectFilter
          className={`shrink-0 ${COL.name}`}
          label={t('monitoring.colName')}
          options={nameOptions}
          selected={filters.names}
          testId="name"
          onToggle={(v) => toggleSelection('names', v)}
          onClear={() => clearSelection('names')}
        />
        {/* 메시지는 자유 텍스트라 선택 필터가 성립하지 않는다. */}
        <span className="flex-1 px-1">{t('monitoring.colMessage')}</span>
      </div>

      {/* 로그 목록 */}
      <div className="overflow-auto font-mono text-xs" style={{ maxHeight: 500 }}>
        {pageEntries.length === 0 ? (
          <div className="flex items-center justify-center py-12 text-sm text-(--color-text-muted)">
            {entries.length === 0 ? t('monitoring.noLogs') : t('monitoring.noFilteredLogs')}
          </div>
        ) : (
          pageEntries.map((entry) => (
            <div
              key={entry.id}
              data-testid="log-row"
              className="flex items-center gap-2 px-3 border-b border-(--color-border-subtle) hover:bg-(--color-bg-elevated)"
              style={{ height: 28 }}
            >
              {/* 타임스탬프 */}
              <span className={`text-(--color-text-muted) shrink-0 px-1 ${COL.time}`}>
                {entry.timestamp}
              </span>
              {/* 레벨 뱃지 */}
              <span className={`shrink-0 px-1 text-center ${COL.level}`}>
                <span
                  className={`px-1.5 py-0.5 rounded text-[10px] font-semibold ${LEVEL_STYLES[entry.level]}`}
                >
                  {entry.level}
                </span>
              </span>
              {/* 소스 뱃지 */}
              <span className={`shrink-0 px-1 text-center ${COL.source}`}>
                {entry.source ? (
                  <span
                    className={`px-1 py-0.5 rounded text-[10px] font-medium ${SOURCE_STYLES[entry.source as SourceType] ?? ''}`}
                  >
                    {entry.source}
                  </span>
                ) : null}
              </span>
              {/* 타입 */}
              <span className={`text-purple-600 dark:text-purple-400 shrink-0 truncate px-1 ${COL.kind}`}>
                {entry.componentKind ?? ''}
              </span>
              {/* 이름 — 에이전트/노드/플로우는 해당 메뉴로 이동할 수 있다. */}
              <span className={`shrink-0 truncate px-1 ${COL.name}`}>
                {onSelectComponent && isLinkableLogComponent(entry.source, entry.componentName) ? (
                  <button
                    type="button"
                    data-testid="log-component-link"
                    title={t('monitoring.openInMenu')}
                    onClick={() => onSelectComponent(entry.source!, entry.componentName!)}
                    className="max-w-full truncate text-blue-600 underline decoration-dotted underline-offset-2 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300"
                  >
                    {entry.componentName}
                  </button>
                ) : (
                  <span className="text-(--color-text-muted)">{entry.componentName ?? ''}</span>
                )}
              </span>
              {/* 메시지 */}
              <span className="text-(--color-text-primary) truncate px-1">
                {entry.message}
              </span>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

/**
 * 팝오버 바깥 클릭/Esc 로 닫기.
 *
 * 컬럼 헤더 팝오버가 여러 개라 각자 닫히는 규칙이 같아야 한다.
 */
function useDismiss(open: boolean, onDismiss: () => void) {
  const ref = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onDismiss();
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onDismiss();
    };
    document.addEventListener('mousedown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('mousedown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [open, onDismiss]);

  return ref;
}

/** 컬럼 헤더의 다중 선택 필터 (레벨 / 소스 / 타입 / 이름) */
function ColumnSelectFilter({
  className,
  label,
  options,
  selected,
  testId,
  onToggle,
  onClear,
}: {
  className: string;
  label: string;
  options: string[];
  selected: Set<string>;
  testId: string;
  onToggle: (value: string) => void;
  onClear: () => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const close = useCallback(() => setOpen(false), []);
  const ref = useDismiss(open, close);
  const active = selected.size > 0;

  return (
    <div className={`relative ${className}`} ref={ref}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        data-testid={`log-filter-${testId}`}
        aria-expanded={open}
        className={`flex w-full items-center gap-1 rounded px-1 py-0.5 uppercase tracking-wider transition-colors hover:bg-(--color-bg-elevated) ${
          active ? 'text-blue-600 dark:text-blue-400' : 'hover:text-(--color-text-primary)'
        }`}
      >
        <span className="truncate">{label}</span>
        <Filter className="h-3 w-3 shrink-0" />
        {active && (
          <span
            data-testid={`log-filter-count-${testId}`}
            className="ml-auto shrink-0 rounded bg-blue-100 px-1 text-[9px] text-blue-700 dark:bg-blue-900 dark:text-blue-300"
          >
            {selected.size}
          </span>
        )}
      </button>

      {open && (
        <div
          data-testid={`log-filter-menu-${testId}`}
          className="absolute left-0 top-full z-20 mt-1 max-h-64 w-44 overflow-y-auto rounded-md border border-(--color-border-default) bg-(--color-bg-surface) py-1 font-sans text-xs normal-case tracking-normal shadow-lg"
        >
          <button
            type="button"
            onClick={onClear}
            data-testid={`log-filter-all-${testId}`}
            className="flex w-full items-center px-2 py-1 text-left text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
          >
            {t('monitoring.filterAll')}
          </button>
          {options.length === 0 ? (
            <p className="px-2 py-1 text-(--color-text-muted)">{t('monitoring.filterNoOptions')}</p>
          ) : (
            options.map((option) => (
              <label
                key={option}
                className="flex cursor-pointer items-center gap-2 px-2 py-1 transition-colors hover:bg-(--color-bg-elevated)"
              >
                <input
                  type="checkbox"
                  checked={selected.has(option)}
                  onChange={() => onToggle(option)}
                  data-testid={`log-filter-option-${testId}-${option}`}
                  className="h-3.5 w-3.5 rounded border-(--color-border-strong) text-blue-600 focus:ring-blue-500"
                />
                <span className="truncate text-(--color-text-primary)">{option}</span>
              </label>
            ))
          )}
        </div>
      )}
    </div>
  );
}

/** 컬럼 헤더의 시간 구간 필터 */
function TimeRangeFilter({
  from,
  to,
  onChange,
}: {
  from: string;
  to: string;
  onChange: (from: string, to: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const close = useCallback(() => setOpen(false), []);
  const ref = useDismiss(open, close);
  const active = from !== '' || to !== '';

  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        data-testid="log-filter-time"
        aria-expanded={open}
        className={`flex items-center rounded px-1 py-0.5 transition-colors hover:bg-(--color-bg-elevated) ${
          active ? 'text-blue-600 dark:text-blue-400' : 'hover:text-(--color-text-primary)'
        }`}
      >
        <Filter className="h-3 w-3" />
      </button>

      {open && (
        <div
          data-testid="log-filter-menu-time"
          className="absolute left-0 top-full z-20 mt-1 w-52 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-2 font-sans text-xs normal-case tracking-normal shadow-lg"
        >
          <label className="mb-1 block text-[10px] text-(--color-text-muted)">
            {t('monitoring.timeFrom')}
          </label>
          <input
            type="time"
            step={1}
            value={from}
            data-testid="log-filter-time-from"
            onChange={(e) => onChange(e.target.value, to)}
            className="mb-2 w-full rounded border border-(--color-border-default) bg-transparent px-2 py-1 text-(--color-text-primary) focus:border-blue-400 focus:outline-none"
          />
          <label className="mb-1 block text-[10px] text-(--color-text-muted)">
            {t('monitoring.timeTo')}
          </label>
          <input
            type="time"
            step={1}
            value={to}
            data-testid="log-filter-time-to"
            onChange={(e) => onChange(from, e.target.value)}
            className="w-full rounded border border-(--color-border-default) bg-transparent px-2 py-1 text-(--color-text-primary) focus:border-blue-400 focus:outline-none"
          />
          {active && (
            <button
              type="button"
              onClick={() => onChange('', '')}
              data-testid="log-filter-time-clear"
              className="mt-2 w-full rounded px-2 py-1 text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
            >
              {t('monitoring.filterAll')}
            </button>
          )}
        </div>
      )}
    </div>
  );
}
