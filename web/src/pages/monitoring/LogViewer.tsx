// 실시간 로그 뷰어 (페이지네이션 방식).
// 레벨/소스/컴포넌트 필터와 페이지 단위 탐색을 지원한다.

import { useCallback, useEffect, useMemo, useState } from 'react';
import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight, Search } from 'lucide-react';
import { useTranslation } from '@/lib/i18n';

/** 로그 레벨 타입 */
export type LogLevel = 'DEBUG' | 'INFO' | 'WARN' | 'ERROR';

/** 소스 카테고리 타입 */
export type SourceType = 'agent' | 'node' | 'flow' | 'api' | 'engine' | 'system';

/** 단일 로그 항목 */
export interface LogEntry {
  id: string;
  timestamp: string;
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
  DEBUG: 'bg-gray-200 text-gray-700 dark:bg-gray-700 dark:text-gray-300',
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
  system: 'bg-gray-200 text-gray-600 dark:bg-gray-700 dark:text-gray-400',
};

const ALL_LEVELS: LogLevel[] = ['DEBUG', 'INFO', 'WARN', 'ERROR'];
const ALL_SOURCES: SourceType[] = ['agent', 'node', 'flow', 'api', 'engine', 'system'];

interface LogViewerProps {
  entries: LogEntry[];
}

/** 로그 뷰어 컴포넌트 (페이지네이션) */
export default function LogViewer({ entries }: LogViewerProps) {
  const { t } = useTranslation();
  const [filter, setFilter] = useState<LogLevel | null>(null);
  const [sourceFilter, setSourceFilter] = useState<Set<SourceType>>(new Set());
  const [componentSearch, setComponentSearch] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const [autoScroll, setAutoScroll] = useState(true);
  const [pageSize, setPageSize] = useState<number>(100);
  const [currentPage, setCurrentPage] = useState(1);

  // 컴포넌트 검색 디바운스 (300ms)
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(componentSearch);
    }, 300);
    return () => clearTimeout(timer);
  }, [componentSearch]);

  // 소스 필터 토글
  const toggleSource = useCallback((source: SourceType) => {
    setSourceFilter((prev) => {
      const next = new Set(prev);
      if (next.has(source)) {
        next.delete(source);
      } else {
        next.add(source);
      }
      return next;
    });
  }, []);

  // 필터 적용 (레벨 + 소스 + 컴포넌트 AND 조합)
  const filtered = useMemo(() => {
    return entries.filter((e) => {
      if (filter !== null && e.level !== filter) return false;
      if (sourceFilter.size > 0 && e.source && !sourceFilter.has(e.source as SourceType))
        return false;
      if (debouncedSearch) {
        const q = debouncedSearch.toLowerCase();
        const matchKind = e.componentKind?.toLowerCase().includes(q);
        const matchName = e.componentName?.toLowerCase().includes(q);
        const matchComp = e.component?.toLowerCase().includes(q);
        if (!matchKind && !matchName && !matchComp) return false;
      }
      return true;
    });
  }, [entries, filter, sourceFilter, debouncedSearch]);

  // 페이지 계산
  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize));

  // 필터 변경 시 첫 페이지로 이동
  useEffect(() => {
    setCurrentPage(1);
  }, [filter, sourceFilter, debouncedSearch, pageSize]);

  // 자동 스크롤: 새 로그 수신 시 마지막 페이지로 이동
  useEffect(() => {
    if (autoScroll) {
      setCurrentPage(totalPages);
    }
  }, [autoScroll, totalPages]);

  // 현재 페이지의 로그 항목
  const pageEntries = useMemo(() => {
    const start = (currentPage - 1) * pageSize;
    return filtered.slice(start, start + pageSize);
  }, [filtered, currentPage, pageSize]);

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
      if (!prev) {
        setCurrentPage(totalPages);
      }
      return !prev;
    });
  }, [totalPages]);

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

  return (
    <div className="bg-(--color-bg-surface) rounded-lg shadow">
      {/* 타이틀 + 건수 */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-(--color-border-default)">
        <h3 className="text-sm font-semibold text-(--color-text-primary)">
          {t('monitoring.systemLog')}
        </h3>
        <span className="text-xs text-(--color-text-muted)">
          {filtered.length === entries.length
            ? `${entries.length.toLocaleString()}${t('monitoring.countUnit')}`
            : `${filtered.length.toLocaleString()} / ${entries.length.toLocaleString()}${t('monitoring.countUnit')}`}
        </span>
      </div>

      {/* 필터 Row 1: 레벨 + 소스 + 자동 스크롤 */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-(--color-border-default)">
        <div className="flex items-center gap-1">
          {/* 레벨 필터 */}
          <button
            type="button"
            onClick={() => setFilter(null)}
            className={`px-2 py-1 text-xs rounded font-medium transition-colors ${
              filter === null
                ? 'bg-gray-900 text-white dark:bg-white dark:text-gray-900'
                : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)'
            }`}
          >
            {t('monitoring.allLevels')}
          </button>
          {ALL_LEVELS.map((level) => (
            <button
              key={level}
              type="button"
              onClick={() => setFilter(level)}
              className={`px-2 py-1 text-xs rounded font-medium transition-colors ${
                filter === level
                  ? LEVEL_STYLES[level]
                  : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)'
              }`}
            >
              {level}
            </button>
          ))}

          {/* 구분선 */}
          <span className="mx-1 text-(--color-border-strong)">|</span>

          {/* 소스 필터 */}
          {ALL_SOURCES.map((source) => (
            <button
              key={source}
              type="button"
              onClick={() => toggleSource(source)}
              className={`px-2 py-1 text-[10px] rounded font-medium transition-colors ${
                sourceFilter.has(source)
                  ? SOURCE_STYLES[source]
                  : 'text-(--color-text-muted) hover:bg-(--color-bg-elevated)'
              }`}
            >
              {source}
            </button>
          ))}
        </div>
        <button
          type="button"
          onClick={toggleAutoScroll}
          className={`px-2 py-1 text-xs rounded font-medium transition-colors ${
            autoScroll
              ? 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300'
              : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)'
          }`}
        >
          {t('monitoring.realtime')} {autoScroll ? 'ON' : 'OFF'}
        </button>
      </div>

      {/* 필터 Row 2: 컴포넌트 검색 */}
      <div className="flex items-center gap-2 px-3 py-2 border-b border-(--color-border-default)">
        <Search className="w-3.5 h-3.5 text-(--color-text-muted) shrink-0" />
        <input
          type="text"
          value={componentSearch}
          onChange={(e) => setComponentSearch(e.target.value)}
          placeholder={t('monitoring.componentFilter')}
          className="w-48 px-2 py-1 text-xs bg-transparent border border-(--color-border-default) rounded text-(--color-text-primary) placeholder-(--color-text-muted) focus:outline-none focus:border-blue-400"
        />
      </div>

      {/* 페이지네이션 */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-(--color-border-default) bg-(--color-bg-sunken)">
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

      {/* 테이블 헤더 */}
      <div className="flex items-center gap-2 px-3 py-1.5 border-b border-(--color-border-default) bg-(--color-bg-sunken) font-mono text-[10px] font-semibold text-(--color-text-muted) uppercase tracking-wider">
        <span className="shrink-0 w-[140px]">{t('monitoring.colTime')}</span>
        <span className="shrink-0 w-[50px] text-center">{t('monitoring.colLevel')}</span>
        <span className="shrink-0 w-[55px] text-center">{t('monitoring.colSource')}</span>
        <span className="shrink-0 w-[100px]">{t('monitoring.colType')}</span>
        <span className="shrink-0 w-[100px]">{t('monitoring.colName')}</span>
        <span className="flex-1">{t('monitoring.colMessage')}</span>
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
              className="flex items-center gap-2 px-3 border-b border-(--color-border-subtle) hover:bg-(--color-bg-elevated)"
              style={{ height: 28 }}
            >
              {/* 타임스탬프 */}
              <span className="text-(--color-text-muted) shrink-0 w-[140px]">
                {entry.timestamp}
              </span>
              {/* 레벨 뱃지 */}
              <span
                className={`px-1.5 py-0.5 rounded text-[10px] font-semibold shrink-0 w-[50px] text-center ${LEVEL_STYLES[entry.level]}`}
              >
                {entry.level}
              </span>
              {/* 소스 뱃지 */}
              <span className="shrink-0 w-[55px] text-center">
                {entry.source ? (
                  <span
                    className={`px-1 py-0.5 rounded text-[10px] font-medium ${SOURCE_STYLES[entry.source as SourceType] ?? ''}`}
                  >
                    {entry.source}
                  </span>
                ) : null}
              </span>
              {/* 타입 */}
              <span className="text-purple-600 dark:text-purple-400 shrink-0 w-[100px] truncate">
                {entry.componentKind ?? ''}
              </span>
              {/* 이름 */}
              <span className="text-(--color-text-muted) shrink-0 w-[100px] truncate">
                {entry.componentName ?? ''}
              </span>
              {/* 메시지 */}
              <span className="text-(--color-text-primary) truncate">
                {entry.message}
              </span>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
