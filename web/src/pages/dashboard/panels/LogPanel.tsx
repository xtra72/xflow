// 로그 패널 컴포넌트.
// WebSocket을 통해 실시간 로그 스트림을 수신하고,
// 소스/레벨 필터, 자동 스크롤, 최대 라인 수 제한을 지원한다.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Palette, Settings, X } from 'lucide-react';
import { createPortal } from 'react-dom';

import { useWebSocket } from '@/hooks';
import { WS_MESSAGE_TYPES } from '@/services/ws/wsHandlers';
import type { LogLevel } from '@/pages/monitoring/LogViewer';

const LOG_COLOR_PRESETS = ['#3b82f6','#8b5cf6','#06b6d4','#10b981','#f59e0b','#ef4444','#ec4899','#6b7280'];

/** 로그 항목 */
interface LogEntry {
  id: string;
  timestamp: string;
  level: LogLevel;
  message: string;
  source?: string;
}

/** 레벨별 뱃지 스타일 */
const LEVEL_STYLES: Record<LogLevel, string> = {
  DEBUG: 'bg-gray-200 text-gray-700 dark:bg-gray-700 dark:text-gray-300',
  INFO: 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300',
  WARN: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900 dark:text-yellow-300',
  ERROR: 'bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300',
};

/** 레벨 필터 옵션 */
const ALL_LEVELS: LogLevel[] = ['DEBUG', 'INFO', 'WARN', 'ERROR'];

/** 타임스탬프를 HH:MM:SS 형태로 포맷 */
function formatTime(ts: string | Date): string {
  const d = ts instanceof Date ? ts : new Date(ts);
  const h = String(d.getHours()).padStart(2, '0');
  const m = String(d.getMinutes()).padStart(2, '0');
  const s = String(d.getSeconds()).padStart(2, '0');
  return `${h}:${m}:${s}`;
}

/** 고유 ID 생성 */
let logIdCounter = 0;
function nextLogId(): string {
  logIdCounter += 1;
  return `lp-${logIdCounter}`;
}

interface LogPanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;  // { maxLines?: number }
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

/** 실시간 로그 스트림 패널 */
export default function LogPanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange,
  onTitleChange,
}: LogPanelProps) {
  const maxLines = (config.maxLines as number) || 100;
  const panelColor = config.panelColor as string | undefined;
  const accentElements = (config.accentElements as Record<string, string | boolean>) ?? {};
  const acColor = (group: string): string | undefined => {
    if (accentElements[group] === false) return undefined;
    const val = accentElements[group];
    if (typeof val === 'string') return val;
    return panelColor;
  };
  const { client } = useWebSocket();

  // 로그 상태
  const [logs, setLogs] = useState<LogEntry[]>([]);

  // 필터 상태
  const [sourceFilter, setSourceFilter] = useState<string>('');
  const [levelFilter, setLevelFilter] = useState<string>('');

  // 자동 스크롤 상태
  const [autoScroll, setAutoScroll] = useState(true);
  const scrollRef = useRef<HTMLDivElement>(null);
  const userScrolledRef = useRef(false);

  // 설정 드롭다운 상태
  const [settingsOpen, setSettingsOpen] = useState(false);
  const settingsButtonRef = useRef<HTMLButtonElement>(null);
  const settingsMenuRef = useRef<HTMLDivElement>(null);
  const [settingsPos, setSettingsPos] = useState({ top: 0, left: 0 });

  // 타이틀 편집 상태
  const [draftTitle, setDraftTitle] = useState(title);
  const [draftMaxLines, setDraftMaxLines] = useState(String(maxLines));

  // 외부 prop 변경 시 동기화
  useEffect(() => {
    setDraftTitle(title);
  }, [title]);

  useEffect(() => {
    setDraftMaxLines(String(maxLines));
  }, [maxLines]);

  // WebSocket 로그 수신 핸들러
  const handleLog = useCallback((data: unknown) => {
    const d = data as {
      level?: string;
      message?: string;
      timestamp?: string;
      source?: string;
    };

    const entry: LogEntry = {
      id: nextLogId(),
      timestamp: d.timestamp ? formatTime(d.timestamp) : formatTime(new Date()),
      level: (d.level?.toUpperCase() as LogLevel) ?? 'INFO',
      message: d.message ?? '',
      source: d.source ?? '',
    };

    setLogs((prev) => {
      const next = [...prev, entry];
      // maxLines 제한 적용
      return next.length > maxLines ? next.slice(next.length - maxLines) : next;
    });
  }, [maxLines]);

  // WebSocket 핸들러 등록/해제
  useEffect(() => {
    if (!client) return;

    client.on(WS_MESSAGE_TYPES.LOG_ENTRY, handleLog);
    return () => {
      client.off(WS_MESSAGE_TYPES.LOG_ENTRY, handleLog);
    };
  }, [client, handleLog]);

  // 새 로그 수신 시 자동 스크롤
  useEffect(() => {
    if (autoScroll && !userScrolledRef.current && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [logs, autoScroll]);

  // 스크롤 이벤트 처리 (사용자가 위로 스크롤하면 자동 스크롤 비활성화)
  const handleScroll = useCallback(() => {
    if (!scrollRef.current) return;
    const { scrollTop, scrollHeight, clientHeight } = scrollRef.current;
    const isAtBottom = scrollHeight - scrollTop - clientHeight < 30;

    if (isAtBottom) {
      userScrolledRef.current = false;
      setAutoScroll(true);
    } else {
      userScrolledRef.current = true;
      setAutoScroll(false);
    }
  }, []);

  // 필터 적용된 로그
  const filteredLogs = useMemo(() => {
    return logs.filter((entry) => {
      if (levelFilter && entry.level !== levelFilter) return false;
      if (sourceFilter && entry.source !== sourceFilter) return false;
      return true;
    });
  }, [logs, levelFilter, sourceFilter]);

  // 고유 소스 목록 (필터 드롭다운용)
  const uniqueSources = useMemo(() => {
    const sources = new Set<string>();
    for (const entry of logs) {
      if (entry.source) {
        sources.add(entry.source);
      }
    }
    return Array.from(sources).sort();
  }, [logs]);

  // ---- 설정 드롭다운 로직 ----

  // 메뉴 위치 계산
  const updateSettingsPos = useCallback(() => {
    if (!settingsButtonRef.current) return;
    const rect = settingsButtonRef.current.getBoundingClientRect();
    const menuWidth = 224; // w-56
    setSettingsPos({
      top: rect.bottom + 4,
      left: rect.right - menuWidth,
    });
  }, []);

  // 열릴 때 위치 계산 + 스크롤/리사이즈 추적
  useEffect(() => {
    if (!settingsOpen) return;
    updateSettingsPos();

    window.addEventListener('scroll', updateSettingsPos, true);
    window.addEventListener('resize', updateSettingsPos);
    return () => {
      window.removeEventListener('scroll', updateSettingsPos, true);
      window.removeEventListener('resize', updateSettingsPos);
    };
  }, [settingsOpen, updateSettingsPos]);

  // 외부 클릭 닫기
  useEffect(() => {
    if (!settingsOpen) return;
    const handler = (e: MouseEvent) => {
      const target = e.target as Node;
      if (
        settingsButtonRef.current?.contains(target) ||
        settingsMenuRef.current?.contains(target)
      ) return;
      setSettingsOpen(false);
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [settingsOpen]);

  // 타이틀 변경 적용
  const handleTitleBlur = () => {
    const trimmed = draftTitle.trim();
    if (trimmed) {
      onTitleChange?.(trimmed);
    } else {
      setDraftTitle(title);
    }
  };

  // 최대 라인 수 변경 적용
  const handleMaxLinesBlur = () => {
    const parsed = parseInt(draftMaxLines, 10);
    if (!isNaN(parsed) && parsed > 0) {
      onConfigChange?.({ ...config, maxLines: parsed });
    } else {
      setDraftMaxLines(String(maxLines));
    }
  };

  // 패널 컬러 변경
  const handlePanelColorChange = (color: string | undefined) => {
    onConfigChange?.({ ...config, panelColor: color });
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-6 shadow">
      {/* 헤더: 타이틀 + 필터 + 설정 */}
      <div className="mb-3 flex shrink-0 items-center justify-between">
        <h3
          className="text-lg font-semibold text-(--color-text-primary)"
          style={acColor('header') ? { color: acColor('header')! } : undefined}
        >
          {title}
        </h3>
        <div className="flex items-center gap-2">
          {/* 레벨 필터 */}
          <select
            value={levelFilter}
            onChange={(e) => setLevelFilter(e.target.value)}
            className="rounded-md border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1 text-xs text-(--color-text-secondary) focus:border-blue-500 focus:outline-none"
          >
            <option value="">전체 레벨</option>
            {ALL_LEVELS.map((level) => (
              <option key={level} value={level}>{level}</option>
            ))}
          </select>

          {/* 소스 필터 */}
          <select
            value={sourceFilter}
            onChange={(e) => setSourceFilter(e.target.value)}
            className="rounded-md border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1 text-xs text-(--color-text-secondary) focus:border-blue-500 focus:outline-none"
          >
            <option value="">전체 소스</option>
            {uniqueSources.map((source) => (
              <option key={source} value={source}>{source}</option>
            ))}
          </select>

          {/* 실시간 토글 */}
          <button
            type="button"
            onClick={() => {
              setAutoScroll((prev) => !prev);
              userScrolledRef.current = false;
              if (!autoScroll && scrollRef.current) {
                scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
              }
            }}
            className={`rounded-md px-2 py-1 text-xs font-medium transition-colors ${
              autoScroll
                ? 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300'
                : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)'
            }`}
          >
            실시간 {autoScroll ? 'ON' : 'OFF'}
          </button>

          {/* 설정 버튼 */}
          <button
            ref={settingsButtonRef}
            type="button"
            onClick={() => setSettingsOpen(!settingsOpen)}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            style={acColor('header') ? { color: acColor('header')! } : undefined}
            aria-label="패널 설정"
          >
            <Settings className="h-4 w-4" />
          </button>
        </div>
      </div>

      {/* 로그 건수 표시 */}
      <div className="mb-2 flex shrink-0 items-center justify-between text-xs text-(--color-text-muted)">
        <span>
          {filteredLogs.length === logs.length
            ? `${logs.length}건`
            : `${filteredLogs.length} / ${logs.length}건`}
        </span>
        <span>최대 {maxLines}줄</span>
      </div>

      {/* 로그 스트림 */}
      <div
        ref={scrollRef}
        onScroll={handleScroll}
        className="min-h-0 flex-1 overflow-y-auto rounded border border-(--color-border-default) bg-(--color-bg-sunken) font-mono text-xs"
      >
        {filteredLogs.length === 0 ? (
          <div className="flex items-center justify-center py-8 text-sm text-(--color-text-muted)">
            {logs.length === 0 ? '수신된 로그가 없습니다' : '필터 조건에 맞는 로그가 없습니다'}
          </div>
        ) : (
          filteredLogs.map((entry) => (
            <div
              key={entry.id}
              className="flex items-center gap-2 border-b border-(--color-border-subtle) px-3 hover:bg-(--color-bg-elevated)"
              style={{ height: 26 }}
            >
              {/* 타임스탬프 */}
              <span className="w-[60px] shrink-0 text-(--color-text-muted)" style={acColor('timestamp') ? { color: acColor('timestamp')! } : undefined}>
                {entry.timestamp}
              </span>
              {/* 레벨 뱃지 */}
              <span
                className={`w-[44px] shrink-0 rounded px-1 py-0.5 text-center text-[10px] font-semibold ${LEVEL_STYLES[entry.level]}`}
                style={acColor('levels') ? { backgroundColor: `${acColor('levels')}20`, color: acColor('levels')! } : undefined}
              >
                {entry.level}
              </span>
              {/* 소스 */}
              {entry.source && (
                <span className="w-[55px] shrink-0 truncate text-[10px] text-purple-600 dark:text-purple-400"
                  style={acColor('source') ? { color: acColor('source')! } : undefined}>
                  {entry.source}
                </span>
              )}
              {/* 메시지 */}
              <span className="flex-1 truncate text-(--color-text-primary)">
                {entry.message}
              </span>
            </div>
          ))
        )}
      </div>

      {/* 설정 드롭다운 (Portal) */}
      {settingsOpen &&
        createPortal(
          <div
            ref={settingsMenuRef}
            className="fixed z-50 w-56 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) py-2 shadow-lg"
            style={{ top: settingsPos.top, left: settingsPos.left }}
          >
            {/* 타이틀 편집 */}
            <div className="px-3 pb-2">
              <label className="mb-1 block text-xs font-medium text-(--color-text-muted)">
                타이틀
              </label>
              <input
                type="text"
                value={draftTitle}
                onChange={(e) => setDraftTitle(e.target.value)}
                onBlur={handleTitleBlur}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') handleTitleBlur();
                }}
                className="w-full rounded border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none"
              />
            </div>

            <div className="my-1 border-t border-(--color-border-default)" />

            {/* 최대 라인 수 설정 */}
            <div className="px-3 pt-1">
              <label className="mb-1 block text-xs font-medium text-(--color-text-muted)">
                최대 라인 수
              </label>
              <input
                type="number"
                min={10}
                max={10000}
                value={draftMaxLines}
                onChange={(e) => setDraftMaxLines(e.target.value)}
                onBlur={handleMaxLinesBlur}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') handleMaxLinesBlur();
                }}
                className="w-full rounded border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none"
              />
            </div>

            {/* 패널 컬러 */}
            <div className="my-1 border-t border-(--color-border-default)" />
            <div className="px-3 pt-1 pb-1">
              <span className="mb-2 block text-xs font-medium text-(--color-text-muted)">
                패널 컬러
              </span>
              <div className="flex flex-wrap items-center gap-1.5">
                {LOG_COLOR_PRESETS.map((color) => (
                  <button
                    key={color}
                    type="button"
                    onClick={() => handlePanelColorChange(color)}
                    className={`h-5 w-5 rounded-full border-2 transition-transform hover:scale-110 ${
                      panelColor === color ? 'border-white ring-2 ring-blue-500' : 'border-transparent'
                    }`}
                    style={{ backgroundColor: color }}
                    aria-label={color}
                  />
                ))}
                {/* 커스텀 컬러 피커 */}
                <label
                  className={`relative flex h-5 w-5 cursor-pointer items-center justify-center rounded-full border-2 transition-transform hover:scale-110 ${
                    panelColor && !LOG_COLOR_PRESETS.includes(panelColor)
                      ? 'border-white ring-2 ring-blue-500'
                      : 'border-dashed border-gray-300 dark:border-gray-600'
                  }`}
                  style={
                    panelColor && !LOG_COLOR_PRESETS.includes(panelColor)
                      ? { backgroundColor: panelColor }
                      : undefined
                  }
                  title="직접 선택"
                >
                  {!(panelColor && !LOG_COLOR_PRESETS.includes(panelColor)) && (
                    <Palette className="h-2.5 w-2.5 text-gray-400" />
                  )}
                  <input
                    type="color"
                    value={panelColor ?? '#3b82f6'}
                    onChange={(e) => handlePanelColorChange(e.target.value)}
                    className="absolute inset-0 cursor-pointer opacity-0"
                  />
                </label>
                {/* 리셋 */}
                {panelColor && (
                  <button
                    type="button"
                    onClick={() => handlePanelColorChange(undefined)}
                    className="flex h-5 w-5 items-center justify-center rounded-full border-2 border-gray-300 text-gray-400 transition-transform hover:scale-110 dark:border-gray-600"
                    title="초기화"
                  >
                    <X className="h-2.5 w-2.5" />
                  </button>
                )}
              </div>
            </div>
          </div>,
          document.body,
        )}
    </div>
  );
}
