// 네트워크 지표 패널.
//
// 인터페이스를 고르면 고른 것마다 선을 하나씩 겹쳐 그리고, 고르지 않으면 전체
// 합산 하나만 그린다. "종합"과 "인터페이스별"을 별도 패널 유형으로 나누지 않은
// 이유가 여기 있다 — 같은 패널을 두 번 배치해 하나는 비우고 하나는 고르면 둘이
// 나란히 놓인다.

import { useCallback, useMemo } from 'react';
import { Network } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { type ChartSeries } from '@/pages/monitoring/MultiSeriesChart';
import {
  formatBytes,
  formatPackets,
  interfaceColors,
  normalizeUnitTime,
} from '@/pages/monitoring/networkSeries';

import { useAutoColumns } from '../monitor/useAutoColumns';
import { SysMetricsPanelShell } from './SysMetricsPanelShell';
import {
  readAccent,
  readAgentRef,
  readMaxCols,
  readRefreshMs,
  readTargets,
  readWindowSec,
} from './sysMetricsPanelConfig';
import { resolveItemOptions } from './sysMetricsItemOptions';
import { SysMetricsItemView } from './SysMetricsItemView';
import { isCollected, resolveTargets, type SysMetricsSnapshot } from './sysMetricsSeries';
import { TOTAL_TARGET, useSysMetricsRateSeries } from './useSysMetricsNetworkSeries';
import { useSysMetricsSnapshot } from './useSysMetricsSnapshot';

/** 차트 하나가 읽히려면 필요한 최소 폭(px) — 범례가 붙어 타일보다 넓다. */
const MIN_CHART_WIDTH = 300;
/** 기본 열 개수 상한 */
const DEFAULT_MAX_COLS = 2;
/** 기본 폴링 주기 */
const DEFAULT_REFRESH_MS = 5_000;
/** 기본 표시 구간 (5분) */
const DEFAULT_WINDOW_SEC = 300;

/** 그릴 필드와 그 표기 방식 */
const CHANNELS = [
  { field: 'bytes_recv', labelKey: 'sysmetrics.channels.rxBytes', bytes: true },
  { field: 'bytes_sent', labelKey: 'sysmetrics.channels.txBytes', bytes: true },
  { field: 'packets_recv', labelKey: 'sysmetrics.channels.rxPackets', bytes: false },
  { field: 'packets_sent', labelKey: 'sysmetrics.channels.txPackets', bytes: false },
] as const;

const CHANNEL_FIELDS = CHANNELS.map((c) => c.field);

interface SysMetricsNetworkPanelProps {
  panelId: string;
  title: string;
  config?: Record<string, unknown>;
}

export function SysMetricsNetworkPanel({ panelId, title, config }: SysMetricsNetworkPanelProps) {
  const { t } = useTranslation();

  const { agentId, agentName } = readAgentRef(config);
  const refreshMs = readRefreshMs(config, DEFAULT_REFRESH_MS);
  const windowSec = readWindowSec(config, DEFAULT_WINDOW_SEC);
  const selected = readTargets(config, 'interfaces');
  const unit = normalizeUnitTime(config?.unitTime);
  const { accentColor } = readAccent(config);

  const maxCols = readMaxCols(config, DEFAULT_MAX_COLS, CHANNELS.length);
  const [gridRef, cols] = useAutoColumns(maxCols, MIN_CHART_WIDTH);

  const { snapshot, previous, state } = useSysMetricsSnapshot(agentId, refreshMs);

  // 선택한 인터페이스 중 스냅샷에 실제로 있는 것만 그린다. 사라진 인터페이스가
  // 있어도 나머지 선은 계속 그려야 한다.
  const targets = useMemo(
    () => (snapshot ? resolveTargets(snapshot.network, selected) : []),
    [snapshot, selected],
  );
  const missing = selected.filter((name) => !targets.includes(name));

  const pickNetwork = useCallback((s: SysMetricsSnapshot) => s.network, []);
  const series = useSysMetricsRateSeries(
    snapshot,
    previous,
    pickNetwork,
    targets,
    CHANNEL_FIELDS as unknown as string[],
    unit,
  );

  // 계열 이름은 대상 이름이고, 대상을 고르지 않았으면 합산 하나다.
  const seriesNames = targets.length > 0 ? targets : [TOTAL_TARGET];
  const colors = interfaceColors(seriesNames);

  const now = snapshot?.collectedAt ?? Date.now();

  // 채널마다 스타일·높이·범례·구간을 따로 고를 수 있다. 고르지 않은 채널은 패널
  // 기본값을 따른다. 네트워크 채널은 모두 증가량이라 값 성격은 counter 로 고정이다.
  const channelOptions = useMemo(
    () =>
      Object.fromEntries(
        CHANNELS.map((c) => [
          c.field,
          resolveItemOptions(config, c.field, 'counter', 'sysmetrics-network'),
        ]),
      ),
    [config, windowSec],
  );

  const collected = snapshot ? isCollected(snapshot, 'network') : true;

  return (
    <SysMetricsPanelShell
      panelId={panelId}
      title={title}
      icon={Network}
      headerColor={accentColor('header')}
      state={state}
      agentName={agentName}
    >
      {!collected ? (
        <p
          data-testid="sysmetrics-network-not-collected"
          className="flex flex-1 items-center justify-center text-xs text-(--color-text-muted)"
        >
          {t('sysmetrics.state.notCollected')}
        </p>
      ) : (
        <>
          {missing.length > 0 && (
            <p
              data-testid="sysmetrics-network-missing"
              className="mb-2 shrink-0 text-xs text-(--color-text-muted)"
            >
              {t('sysmetrics.state.missingTargets')}: {missing.join(', ')}
            </p>
          )}
          <div
            ref={gridRef}
            data-testid="sysmetrics-network-grid"
            data-cols={cols}
            data-series={seriesNames.join(',')}
            className="grid min-h-0 flex-1 gap-2 overflow-y-auto"
            style={{
              gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
              // 행이 남은 높이를 나눠 갖는다 — auto-rows-min 이면 내용 높이만
              // 차지해 패널을 늘려도 아래가 빈 채로 남는다. 읽을 수 있는 최소
              // 높이는 지키고, 그보다 좁아지면 스크롤한다.
              gridAutoRows: 'minmax(160px, 1fr)',
            }}
          >
            {CHANNELS.map((channel) => {
              const opts = channelOptions[channel.field]!;
              const chartSeries: ChartSeries[] = seriesNames.map((name) => ({
                name,
                color: colors[name]!,
                data: series[name]?.[channel.field] ?? [],
              }));
              const format = channel.bytes
                ? (v: number) => formatBytes(v, unit)
                : (v: number) => formatPackets(v, unit);
              // 타일 스타일의 지금 값은 항목 뷰가 계열에서 뽑는다. 여기서 첫 계열만
              // 넘기면 대상을 여럿 골랐을 때 나머지가 조용히 사라진다.
              return (
                <SysMetricsItemView
                  key={channel.field}
                  testId={`sysmetrics-network-item-${channel.field}`}
                  options={opts}
                  label={t(channel.labelKey)}
                  series={chartSeries}
                  timeDomain={[now - opts.windowSec * 1_000, now]}
                  format={format}
                />
              );
            })}
          </div>
        </>
      )}
    </SysMetricsPanelShell>
  );
}

export default SysMetricsNetworkPanel;
