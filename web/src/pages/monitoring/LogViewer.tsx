// 가상화된 실시간 로그 뷰어.
// 1000+/초 로그 스트림에서도 성능 저하 없이 동작하도록
// 고정 높이 윈도잉 방식으로 보이는 행만 렌더링한다.

import { useCallback, useEffect, useRef, useState } from 'react';

/** 로그 레벨 타입 */
export type LogLevel = 'DEBUG' | 'INFO' | 'WARN' | 'ERROR';

/** 단일 로그 항목 */
export interface LogEntry {
  id: string;
  timestamp: string;
  level: LogLevel;
  message: string;
}

// 로그 행 높이 (px)
const ROW_HEIGHT = 28;
// 컨테이너 높이 (px)
const CONTAINER_HEIGHT = 500;
// 위아래로 추가 렌더링할 버퍼 행 수
const BUFFER_ROWS = 10;
// 최대 보관 로그 수
const MAX_ENTRIES = 10_000;

/** 레벨별 뱃지 스타일 */
const LEVEL_STYLES: Record<LogLevel, string> = {
  DEBUG: 'bg-gray-200 text-gray-700 dark:bg-gray-700 dark:text-gray-300',
  INFO: 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300',
  WARN: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900 dark:text-yellow-300',
  ERROR: 'bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300',
};

const ALL_LEVELS: LogLevel[] = ['DEBUG', 'INFO', 'WARN', 'ERROR'];

interface LogViewerProps {
  entries: LogEntry[];
}

/** 가상화된 로그 뷰어 컴포넌트 */
export default function LogViewer({ entries }: LogViewerProps) {
  const [filter, setFilter] = useState<LogLevel | null>(null);
  const [autoScroll, setAutoScroll] = useState(true);
  const containerRef = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);

  // 필터 적용
  const filtered = filter
    ? entries.filter((e) => e.level === filter)
    : entries;

  const totalHeight = filtered.length * ROW_HEIGHT;
  const visibleCount = Math.ceil(CONTAINER_HEIGHT / ROW_HEIGHT);

  // 보이는 행 범위 계산
  const startIdx = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - BUFFER_ROWS);
  const endIdx = Math.min(
    filtered.length,
    Math.floor(scrollTop / ROW_HEIGHT) + visibleCount + BUFFER_ROWS,
  );
  const visibleEntries = filtered.slice(startIdx, endIdx);
  const offsetTop = startIdx * ROW_HEIGHT;

  // 스크롤 핸들러
  const handleScroll = useCallback(() => {
    const el = containerRef.current;
    if (!el) return;

    setScrollTop(el.scrollTop);

    // 사용자가 수동으로 위로 스크롤하면 자동 스크롤 비활성화
    const isAtBottom =
      el.scrollHeight - el.scrollTop - el.clientHeight < ROW_HEIGHT * 2;
    if (!isAtBottom && autoScroll) {
      setAutoScroll(false);
    }
  }, [autoScroll]);

  // 자동 스크롤: 새 로그가 추가되면 하단으로 이동
  useEffect(() => {
    if (autoScroll && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [entries.length, autoScroll]);

  // 자동 스크롤 토글
  const toggleAutoScroll = useCallback(() => {
    setAutoScroll((prev) => {
      const next = !prev;
      if (next && containerRef.current) {
        containerRef.current.scrollTop = containerRef.current.scrollHeight;
      }
      return next;
    });
  }, []);

  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow">
      {/* 툴바: 필터 + 자동 스크롤 토글 */}
      <div className="flex items-center justify-between p-3 border-b border-gray-200 dark:border-gray-700">
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={() => setFilter(null)}
            className={`px-2 py-1 text-xs rounded font-medium transition-colors ${
              filter === null
                ? 'bg-gray-900 text-white dark:bg-white dark:text-gray-900'
                : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700'
            }`}
          >
            전체
          </button>
          {ALL_LEVELS.map((level) => (
            <button
              key={level}
              type="button"
              onClick={() => setFilter(level)}
              className={`px-2 py-1 text-xs rounded font-medium transition-colors ${
                filter === level
                  ? LEVEL_STYLES[level]
                  : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700'
              }`}
            >
              {level}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-gray-500 dark:text-gray-400">
            {filtered.length.toLocaleString()}건
          </span>
          <button
            type="button"
            onClick={toggleAutoScroll}
            className={`px-2 py-1 text-xs rounded font-medium transition-colors ${
              autoScroll
                ? 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300'
                : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700'
            }`}
          >
            자동 스크롤 {autoScroll ? 'ON' : 'OFF'}
          </button>
        </div>
      </div>

      {/* 가상화된 로그 목록 */}
      <div
        ref={containerRef}
        onScroll={handleScroll}
        className="overflow-auto font-mono text-xs"
        style={{ height: CONTAINER_HEIGHT }}
      >
        <div style={{ height: totalHeight, position: 'relative' }}>
          <div
            style={{
              position: 'absolute',
              top: offsetTop,
              left: 0,
              right: 0,
            }}
          >
            {visibleEntries.map((entry) => (
              <div
                key={entry.id}
                className="flex items-center gap-2 px-3 border-b border-gray-50 dark:border-gray-750 hover:bg-gray-50 dark:hover:bg-gray-750"
                style={{ height: ROW_HEIGHT }}
              >
                <span className="text-gray-400 dark:text-gray-500 shrink-0 w-[140px]">
                  {entry.timestamp}
                </span>
                <span
                  className={`px-1.5 py-0.5 rounded text-[10px] font-semibold shrink-0 w-[50px] text-center ${LEVEL_STYLES[entry.level]}`}
                >
                  {entry.level}
                </span>
                <span className="text-gray-800 dark:text-gray-200 truncate">
                  {entry.message}
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

/**
 * 로그 배열에 새 항목을 추가하면서 MAX_ENTRIES 제한을 적용한다.
 * MonitoringPage에서 상태 업데이트에 사용할 유틸리티 함수.
 */
export function appendLog(
  prev: LogEntry[],
  entry: LogEntry,
): LogEntry[] {
  const next = [...prev, entry];
  return next.length > MAX_ENTRIES ? next.slice(next.length - MAX_ENTRIES) : next;
}
