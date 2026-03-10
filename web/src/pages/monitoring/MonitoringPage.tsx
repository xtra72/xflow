// 모니터링 페이지.
// WebSocket을 통해 실시간 메트릭, 로그, 이벤트를 수신하고
// 탭 레이아웃으로 각 섹션을 표시한다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { Activity, FileText, Radio, Wifi, WifiOff } from 'lucide-react';

import { useWebSocket } from '@/hooks';
import { useFlows } from '@/hooks';
import { WS_MESSAGE_TYPES } from '@/services/ws/wsHandlers';
import MetricsChart, { type MetricDataPoint, type MetricsData } from './MetricsChart';
import LogViewer, { appendLog, type LogEntry, type LogLevel } from './LogViewer';
import EventTimeline, { type SystemEvent } from './EventTimeline';

/** 탭 유형 */
type Tab = 'metrics' | 'logs' | 'events';

/** 탭 설정 */
const TABS: { key: Tab; label: string; icon: React.ComponentType<{ className?: string }> }[] = [
  { key: 'metrics', label: '메트릭', icon: Activity },
  { key: 'logs', label: '로그', icon: FileText },
  { key: 'events', label: '이벤트', icon: Radio },
];

// 메트릭 데이터 보관 기간 (5분 = 300초)
const METRICS_WINDOW_SEC = 300;
// 초 단위 보관 포인트 (1초 간격 가정)
const MAX_METRIC_POINTS = METRICS_WINDOW_SEC;

/** 타임스탬프를 HH:MM:SS 형태로 포맷 */
function formatTime(ts: string | Date): string {
  const d = ts instanceof Date ? ts : new Date(ts);
  const h = String(d.getHours()).padStart(2, '0');
  const m = String(d.getMinutes()).padStart(2, '0');
  const s = String(d.getSeconds()).padStart(2, '0');
  return `${h}:${m}:${s}`;
}

/** 메트릭 데이터 포인트 추가 (최대 개수 제한) */
function appendMetric(
  prev: MetricDataPoint[],
  point: MetricDataPoint,
): MetricDataPoint[] {
  const next = [...prev, point];
  return next.length > MAX_METRIC_POINTS
    ? next.slice(next.length - MAX_METRIC_POINTS)
    : next;
}

/** 고유 ID 생성 (로그/이벤트용) */
let idCounter = 0;
function nextId(): string {
  idCounter += 1;
  return `m-${idCounter}`;
}

/**
 * 시스템 모니터링 메인 페이지.
 * WebSocket 연결을 관리하고 실시간 데이터를 하위 컴포넌트에 전달한다.
 */
export default function MonitoringPage() {
  const { state: wsState, client } = useWebSocket();
  const { data: flowsData } = useFlows();

  // 활성 탭
  const [activeTab, setActiveTab] = useState<Tab>('metrics');

  // 메트릭 데이터
  const [metrics, setMetrics] = useState<MetricsData>({
    cpu: [],
    memory: [],
    throughput: [],
    errorRate: [],
  });

  // 로그 항목
  const [logs, setLogs] = useState<LogEntry[]>([]);

  // 시스템 이벤트
  const [events, setEvents] = useState<SystemEvent[]>([]);

  // WebSocket 핸들러 등록/해제를 위한 ref
  const handlersRef = useRef<{
    metrics: (data: unknown) => void;
    log: (data: unknown) => void;
    event: (data: unknown) => void;
  } | null>(null);

  // WS 메트릭 핸들러
  const handleMetrics = useCallback((data: unknown) => {
    const d = data as Record<string, number>;
    const time = formatTime(new Date());

    setMetrics((prev) => ({
      cpu: appendMetric(prev.cpu, { time, value: d.cpu ?? 0 }),
      memory: appendMetric(prev.memory, { time, value: d.memory ?? 0 }),
      throughput: appendMetric(prev.throughput, { time, value: d.throughput ?? 0 }),
      errorRate: appendMetric(prev.errorRate, { time, value: d.error_rate ?? 0 }),
    }));
  }, []);

  // WS 로그 핸들러
  const handleLog = useCallback((data: unknown) => {
    const d = data as { level?: string; message?: string; timestamp?: string; component?: string; source?: string; componentKind?: string; componentName?: string };
    const entry: LogEntry = {
      id: nextId(),
      timestamp: d.timestamp ? formatTime(d.timestamp) : formatTime(new Date()),
      level: (d.level?.toUpperCase() as LogLevel) ?? 'INFO',
      message: d.message ?? '',
      component: d.component ?? '',
      source: d.source ?? 'system',
      componentKind: d.componentKind ?? '',
      componentName: d.componentName ?? '',
    };
    setLogs((prev) => appendLog(prev, entry));
  }, []);

  // WS 이벤트 핸들러
  const handleEvent = useCallback((data: unknown) => {
    const d = data as {
      type?: string;
      message?: string;
      timestamp?: string;
      details?: string;
    };
    const event: SystemEvent = {
      id: nextId(),
      type: (d.type as SystemEvent['type']) ?? 'system',
      message: d.message ?? '시스템 이벤트',
      timestamp: d.timestamp ?? new Date().toISOString(),
      details: d.details,
    };
    setEvents((prev) => [...prev, event]);
  }, []);

  // WebSocket 핸들러 등록 및 정리 (REQ-07-02, REQ-07-06)
  useEffect(() => {
    if (!client) return;

    const handlers = {
      metrics: handleMetrics,
      log: handleLog,
      event: handleEvent,
    };
    handlersRef.current = handlers;

    client.on(WS_MESSAGE_TYPES.FLOW_METRICS, handlers.metrics);
    client.on(WS_MESSAGE_TYPES.LOG_ENTRY, handlers.log);
    client.on(WS_MESSAGE_TYPES.SYSTEM_EVENT, handlers.event);

    return () => {
      client.off(WS_MESSAGE_TYPES.FLOW_METRICS, handlers.metrics);
      client.off(WS_MESSAGE_TYPES.LOG_ENTRY, handlers.log);
      client.off(WS_MESSAGE_TYPES.SYSTEM_EVENT, handlers.event);
      handlersRef.current = null;
    };
  }, [client, handleMetrics, handleLog, handleEvent]);

  // 플로우 상태 요약
  const flows = flowsData?.data ?? [];
  const runningFlows = flows.filter((f) => f.status === 'Running').length;

  return (
    <div className="space-y-4">
      {/* 헤더 */}
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-white">
          모니터링
        </h2>
        <div className="flex items-center gap-2">
          {wsState === 'connected' ? (
            <span className="flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
              <Wifi className="w-3.5 h-3.5" />
              연결됨
            </span>
          ) : (
            <span className="flex items-center gap-1 text-xs text-red-500 dark:text-red-400">
              <WifiOff className="w-3.5 h-3.5" />
              {wsState === 'connecting' || wsState === 'reconnecting'
                ? '연결 중...'
                : '연결 끊김'}
            </span>
          )}
        </div>
      </div>

      {/* 플로우 상태 요약 */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
        <SummaryCard label="전체 플로우" value={flows.length} />
        <SummaryCard label="실행 중" value={runningFlows} accent />
        <SummaryCard label="로그 수신" value={logs.length} />
        <SummaryCard label="이벤트" value={events.length} />
      </div>

      {/* 탭 헤더 */}
      <div className="border-b border-gray-200 dark:border-gray-700">
        <div className="flex gap-4">
          {TABS.map(({ key, label, icon: Icon }) => (
            <button
              key={key}
              type="button"
              onClick={() => setActiveTab(key)}
              className={`flex items-center gap-1.5 px-1 pb-2 text-sm font-medium border-b-2 transition-colors ${
                activeTab === key
                  ? 'border-blue-500 text-blue-600 dark:text-blue-400'
                  : 'border-transparent text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300'
              }`}
            >
              <Icon className="w-4 h-4" />
              {label}
            </button>
          ))}
        </div>
      </div>

      {/* 탭 콘텐츠 */}
      {activeTab === 'metrics' && <MetricsChart data={metrics} />}
      {activeTab === 'logs' && <LogViewer entries={logs} />}
      {activeTab === 'events' && <EventTimeline events={events} />}
    </div>
  );
}

/** 상단 요약 카드 */
function SummaryCard({
  label,
  value,
  accent = false,
}: {
  label: string;
  value: number;
  accent?: boolean;
}) {
  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow p-3">
      <p className="text-xs text-gray-500 dark:text-gray-400">{label}</p>
      <p
        className={`text-xl font-bold mt-1 ${
          accent
            ? 'text-blue-600 dark:text-blue-400'
            : 'text-gray-900 dark:text-white'
        }`}
      >
        {value.toLocaleString()}
      </p>
    </div>
  );
}
