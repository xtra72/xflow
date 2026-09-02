// 시스템 통계 대시보드 패널.
//
// 런타임 실측(GET /monitor/metrics) · 플로우 · WS 수신량 · 연결 상태를 한 패널에서
// 카드로 보여준다.
//
// 설정 (`config`):
//   - items:     표시할 통계 항목
//   - maxCols:   열 개수 상한 (좁아지면 자동으로 줄어든다)
//   - refreshMs: 갱신 주기 — 런타임 폴링 주기이자 스트림 카운터 반영 주기
//   - panelColor / accentElements: 대시보드 공통 악센트 색 규약

import { useMemo } from 'react';
import { Activity } from 'lucide-react';

import { useFlows, useWebSocket } from '@/hooks';
import { useTranslation } from '@/lib/i18n';
import { useSystemMetrics } from '@/services/api/monitorService';
import StatItem, { type StatSources } from '@/pages/monitoring/StatItem';
import { useMonitorStream } from '@/pages/monitoring/monitorStream';
import type { StatItemKey } from '@/pages/monitoring/monitoringLayout';

import { usePanelTitleStyle, usePanelTitleVisible } from '../../panelChromeContext';
import { readAccent, readMaxCols, readPanelItems, readRefreshMs } from './monitorPanelConfig';
import { useAutoColumns } from './useAutoColumns';
import { useThrottledValue } from './useThrottledValue';

/** 통계 카드 하나가 읽히려면 필요한 최소 폭(px) */
const MIN_CARD_WIDTH = 130;
/** 기본 열 개수 상한 */
const DEFAULT_MAX_COLS = 3;
/** 기본 갱신 주기 — 기존 useSystemMetrics 의 5초를 유지한다. */
const DEFAULT_REFRESH_MS = 5_000;

interface MonitorStatsPanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

export default function MonitorStatsPanel({ title, config }: MonitorStatsPanelProps) {
  const { t } = useTranslation();
  const showTitle = usePanelTitleVisible();
  const titleStyle = usePanelTitleStyle();
  const items = useMemo(() => readPanelItems('stats', config), [config]);
  const maxCols = readMaxCols(config, DEFAULT_MAX_COLS, items.length);
  const refreshMs = readRefreshMs(config, DEFAULT_REFRESH_MS);
  const { accentColor } = readAccent(config);

  const [gridRef, cols] = useAutoColumns(maxCols, MIN_CARD_WIDTH);

  const { state: wsState } = useWebSocket();
  const { data: flowsData } = useFlows();
  const { data: runtime, isLoading: runtimeLoading } = useSystemMetrics(refreshMs);
  const stream = useMonitorStream();

  // 스트림 카운터는 초당 여러 번 바뀔 수 있다. 런타임 폴링과 같은 주기로 조여
  // 패널 전체가 한 박자로 갱신되게 한다.
  // 매 렌더 새 객체를 넘기면 throttle 이 값 변화 없이도 계속 재예약된다.
  const rawCounts = useMemo(
    () => ({ logs: stream.logsReceived, events: stream.eventsReceived }),
    [stream.logsReceived, stream.eventsReceived],
  );
  const counts = useThrottledValue(rawCounts, refreshMs);

  const flows = flowsData?.data ?? [];
  const sources: StatSources = {
    runtime,
    runtimeLoading,
    totalFlows: flows.length,
    runningFlows: flows.filter((f) => f.status === 'Running').length,
    logsReceived: counts.logs,
    eventsReceived: counts.events,
    wsState,
  };

  const labelColor = accentColor('label');
  const valueColor = accentColor('value');

  return (
    <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-4 shadow">
      {showTitle && (
        <div className="mb-3 flex shrink-0 items-center gap-2">
          <Activity
            className="h-4 w-4 shrink-0 text-(--color-text-muted)"
            style={accentColor('header') ? { color: accentColor('header') } : undefined}
          />
          <span
            className="truncate text-sm font-medium text-(--color-text-primary)"
            style={{ ...(accentColor('header') ? { color: accentColor('header') } : undefined), ...titleStyle }}
          >
            {title}
          </span>
        </div>
      )}
      {items.length === 0 ? (
        <p
          data-testid="monitor-stats-panel-empty"
          className="flex flex-1 items-center justify-center text-xs text-(--color-text-muted)"
        >
          {t('monitoring.emptyPanel')}
        </p>
      ) : (
        <div
          ref={gridRef}
          data-testid="monitor-stats-panel-grid"
          data-cols={cols}
          className="grid min-h-0 flex-1 auto-rows-min gap-2 overflow-y-auto"
          style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }}
        >
          {items.map((key) => (
            <StatItem
              key={key}
              itemKey={key as StatItemKey}
              sources={sources}
              labelColor={labelColor}
              valueColor={valueColor}
            />
          ))}
        </div>
      )}
    </div>
  );
}
