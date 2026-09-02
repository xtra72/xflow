// 실시간 메트릭 차트 대시보드 패널.
//
// WebSocket `flow.metrics` 스트림의 채널(CPU/메모리/처리량/에러율)을 최근 5분
// 라인 차트로 그린다.
//
// 설정 (`config`):
//   - items:     표시할 채널
//   - maxCols:   열 개수 상한 (좁아지면 자동으로 줄어든다)
//   - refreshMs: 차트 갱신 주기 — 스트림이 더 빨리 와도 이 주기로만 다시 그린다
//   - windowSec: 표시 구간 — 차트에 보일 최근 구간
//
// 네트워크는 성격(출처·단위·누적)이 달라 별도 패널(MonitorNetworkPanel)로 나눴다.
//   - panelColor / accentElements: 채널별 선 색 (대시보드 공통 악센트 색 규약)

import { useMemo } from 'react';
import { TrendingUp } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { MetricChannelChart, type MetricChannel } from '@/pages/monitoring/MetricsChart';
import { useMonitorStream } from '@/pages/monitoring/monitorStream';

import { usePanelTitleStyle, usePanelTitleVisible } from '../../panelChromeContext';
import {
  readAccent,
  readMaxCols,
  readPanelItems,
  readRefreshMs,
  readWindowSec,
} from './monitorPanelConfig';
import { useAutoColumns } from './useAutoColumns';

import { useThrottledValue } from './useThrottledValue';

/** 차트 하나가 읽히려면 필요한 최소 폭(px) */
const MIN_CHART_WIDTH = 260;
/** 기본 열 개수 상한 — 기존 2열 배치를 유지한다. */
const DEFAULT_MAX_COLS = 2;
/** 기본 차트 갱신 주기 */
const DEFAULT_REFRESH_MS = 1_000;
/** 기본 표시 구간 (5분) — 스트림 버퍼 전체 길이와 같다. */
const DEFAULT_WINDOW_SEC = 300;

interface MonitorMetricsPanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

export default function MonitorMetricsPanel({ title, config }: MonitorMetricsPanelProps) {
  const { t } = useTranslation();
  const showTitle = usePanelTitleVisible();
  const titleStyle = usePanelTitleStyle();
  const items = useMemo(() => readPanelItems('metrics', config), [config]);
  const maxCols = readMaxCols(config, DEFAULT_MAX_COLS, items.length);
  const refreshMs = readRefreshMs(config, DEFAULT_REFRESH_MS);
  const { accentColor } = readAccent(config);

  const [gridRef, cols] = useAutoColumns(maxCols, MIN_CHART_WIDTH);

  const { metrics } = useMonitorStream();
  // 스트림은 초당 여러 번 올 수 있다. 차트 다시 그리기는 설정한 주기로 조인다.
  const throttled = useThrottledValue(metrics, refreshMs);

  // 축은 표시 구간 전체를 그리고, 구간 밖 포인트만 차트가 잘라 낸다. 버퍼(5분)는
  // 그대로 두므로 구간을 늘리면 이미 쌓인 데이터가 그대로 드러난다.
  const windowSec = readWindowSec(config, DEFAULT_WINDOW_SEC);
  const windowMs = windowSec * 1_000;

  return (
    <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-4 shadow">
      {showTitle && (
        <div className="mb-3 flex shrink-0 items-center gap-2">
          <TrendingUp
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
          data-testid="monitor-metrics-panel-empty"
          className="flex flex-1 items-center justify-center text-xs text-(--color-text-muted)"
        >
          {t('monitoring.emptyPanel')}
        </p>
      ) : (
        <div
          ref={gridRef}
          data-testid="monitor-metrics-panel-grid"
          data-cols={cols}
          className="grid min-h-0 flex-1 auto-rows-min gap-3 overflow-y-auto"
          style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }}
        >
          {items.map((key) => (
            <MetricChannelChart
              key={key}
              channel={key as MetricChannel}
              data={throttled}
              color={accentColor(key)}
              windowMs={windowMs}
            />
          ))}
        </div>
      )}
    </div>
  );
}
