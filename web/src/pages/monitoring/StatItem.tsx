// 통계 섹션의 항목 렌더러.
//
// 항목 키 하나를 받아 카드 한 장을 그린다. 값의 출처는 네 갈래이며(런타임 실측 /
// 플로우 / WS 수신 누적 / 연결 상태) 상위에서 StatSources 로 한 번에 주입한다.

import { useTranslation } from '@/lib/i18n';
import type { ConnectionState } from '@/services/ws/wsClient';
import type { SystemMetrics } from '@/services/api/monitorService';

import { findItemMeta } from './monitoringCatalog';
import type { StatItemKey } from './monitoringLayout';

/** 통계 항목이 참조하는 값 묶음 */
export interface StatSources {
  /** GET /monitor/metrics 응답 (미도착 시 undefined) */
  runtime?: SystemMetrics;
  /** 런타임 메트릭 최초 로딩 여부 */
  runtimeLoading: boolean;
  totalFlows: number;
  runningFlows: number;
  logsReceived: number;
  eventsReceived: number;
  wsState: ConnectionState;
}

/** 표시할 값과 색 강조 여부 */
interface StatValue {
  text: string;
  accent?: 'blue' | 'green' | 'red';
}

/** uptime(초)을 `2d 3h 4m` 형태로 압축한다. 1분 미만은 초로 표시한다. */
function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return '-';
  const total = Math.floor(seconds);
  if (total < 60) return `${total}s`;
  const d = Math.floor(total / 86_400);
  const h = Math.floor((total % 86_400) / 3_600);
  const m = Math.floor((total % 3_600) / 60);
  const parts: string[] = [];
  if (d > 0) parts.push(`${d}d`);
  if (h > 0) parts.push(`${h}h`);
  // 일 단위가 붙으면 분은 노이즈에 가까워 생략한다.
  if (m > 0 && d === 0) parts.push(`${m}m`);
  return parts.join(' ');
}

/** 숫자를 소수 1자리 + 단위로 표기한다. 값이 없으면 대시. */
function formatNumeric(value: unknown, unit: string): string {
  return typeof value === 'number' && Number.isFinite(value)
    ? `${value.toFixed(1)}${unit}`
    : '-';
}

/** 정수 카운트 표기. 값이 없으면 대시. */
function formatCount(value: unknown): string {
  return typeof value === 'number' && Number.isFinite(value)
    ? Math.round(value).toLocaleString()
    : '-';
}

/** WebSocket 연결 상태 → 표시 문자열 + 색 */
function formatWsState(state: ConnectionState, t: (k: string) => string): StatValue {
  if (state === 'connected') {
    return { text: t('monitoring.wsConnected'), accent: 'green' };
  }
  if (state === 'connecting' || state === 'reconnecting') {
    return { text: t('monitoring.wsConnecting'), accent: 'blue' };
  }
  return { text: t('monitoring.wsDisconnected'), accent: 'red' };
}

/** 항목 키 → 표시할 값 */
function resolveValue(
  key: StatItemKey,
  src: StatSources,
  t: (k: string) => string,
): StatValue {
  switch (key) {
    case 'cpuUsage':
      return { text: formatNumeric(src.runtime?.cpu_usage_percent, '%') };
    case 'memoryUsage':
      return { text: formatNumeric(src.runtime?.memory_usage_percent, '%') };
    case 'goRoutines':
      return { text: formatCount(src.runtime?.go_routines) };
    case 'heapAlloc':
      return { text: formatNumeric(src.runtime?.go_mem_alloc_mb, ' MB') };
    case 'memSys':
      return { text: formatNumeric(src.runtime?.go_mem_sys_mb, ' MB') };
    case 'uptime':
      return {
        text:
          typeof src.runtime?.uptime_seconds === 'number'
            ? formatUptime(src.runtime.uptime_seconds)
            : '-',
      };
    case 'totalFlows':
      return { text: formatCount(src.totalFlows) };
    case 'runningFlows':
      return { text: formatCount(src.runningFlows), accent: 'blue' };
    case 'logsReceived':
      return { text: formatCount(src.logsReceived) };
    case 'eventsReceived':
      return { text: formatCount(src.eventsReceived) };
    case 'wsState':
      return formatWsState(src.wsState, t);
    default:
      return { text: '-' };
  }
}

/** 런타임 API 를 값의 출처로 삼는 항목 (로딩 표시 대상) */
const RUNTIME_KEYS: ReadonlySet<string> = new Set([
  'cpuUsage',
  'memoryUsage',
  'goRoutines',
  'heapAlloc',
  'memSys',
  'uptime',
]);

const ACCENT_CLASS: Record<NonNullable<StatValue['accent']>, string> = {
  blue: 'text-blue-600 dark:text-blue-400',
  green: 'text-green-600 dark:text-green-400',
  red: 'text-red-500 dark:text-red-400',
};

interface StatItemProps {
  itemKey: StatItemKey;
  sources: StatSources;
  /** 라벨 악센트 색 (대시보드 패널의 색 설정에서 내려온다) */
  labelColor?: string;
  /** 값 악센트 색. 지정하면 상태별 색(파랑/초록/빨강)보다 우선한다. */
  valueColor?: string;
}

export default function StatItem({ itemKey, sources, labelColor, valueColor }: StatItemProps) {
  const { t } = useTranslation();
  const meta = findItemMeta('stats', itemKey);
  const label = meta ? t(meta.labelKey) : itemKey;

  // 런타임 메트릭이 아직 도착하지 않은 동안 대시 대신 로딩 표시를 준다.
  const loading = sources.runtimeLoading && RUNTIME_KEYS.has(itemKey);
  const value = loading
    ? { text: '…' as string, accent: undefined }
    : resolveValue(itemKey, sources, t);

  return (
    <div className="rounded-lg bg-(--color-bg-surface) p-3 shadow">
      {/* 삭제 버튼이 우상단을 덮으므로 라벨에 오른쪽 여백을 준다. */}
      <p
        className="pr-6 text-xs text-(--color-text-muted)"
        style={labelColor ? { color: labelColor } : undefined}
      >
        {label}
      </p>
      <p
        data-testid={`monitor-stat-value-${itemKey}`}
        // 색을 직접 지정했으면 상태별 색 클래스는 얹지 않는다 — 둘이 겹치면
        // 인라인 스타일이 이기지만, 클래스가 남아 있으면 의도가 흐려진다.
        className={`mt-1 truncate text-xl font-bold ${
          valueColor ? '' : value.accent ? ACCENT_CLASS[value.accent] : 'text-(--color-text-primary)'
        }`}
        style={valueColor ? { color: valueColor } : undefined}
      >
        {value.text}
      </p>
    </div>
  );
}
