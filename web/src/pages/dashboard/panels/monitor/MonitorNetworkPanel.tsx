// 네트워크 인터페이스 대시보드 패널.
//
// `GET /monitor/network` 의 누적 카운터를 두 가지로 그린다:
//   - 단위시간당 증가량(rx/tx 데이터량·패킷) — 단위시간은 초/분/시 설정
//   - 부팅 이후 누적(*Total)
// 선택한 인터페이스마다 선을 하나씩 겹쳐 그린다.
//
// 설정 (`config`):
//   - items:      표시할 채널
//   - interfaces: 그릴 인터페이스 (비면 전체 합산만)
//   - unitTime:   rate 계열의 단위시간
//   - maxCols:    열 개수 상한 (표시 항목 수까지)
//   - refreshMs:  폴링 주기
//   - windowSec:  표시 구간

import { useMemo } from 'react';
import { Network } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import NetworkChart from '@/pages/monitoring/NetworkChart';
import { useNetworkSeries } from '@/pages/monitoring/useNetworkSeries';
import {
  TOTAL_INTERFACE,
  interfaceColors,
  normalizeUnitTime,
  type NetworkChannel,
} from '@/pages/monitoring/networkSeries';

import { usePanelTitleVisible } from '../../panelChromeContext';
import {
  readInterfaces,
  readMaxCols,
  readPanelItems,
  readRefreshMs,
  readWindowSec,
} from './monitorPanelConfig';
import { useAutoColumns } from './useAutoColumns';

/** 차트 하나가 읽히려면 필요한 최소 폭(px) — 범례가 붙어 메트릭보다 넓다. */
const MIN_CHART_WIDTH = 300;
/** 기본 열 개수 상한 */
const DEFAULT_MAX_COLS = 2;
/** 기본 폴링 주기 */
const DEFAULT_REFRESH_MS = 5_000;
/** 기본 표시 구간 (5분) */
const DEFAULT_WINDOW_SEC = 300;

interface MonitorNetworkPanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

export default function MonitorNetworkPanel({ title, config }: MonitorNetworkPanelProps) {
  const { t } = useTranslation();
  const showTitle = usePanelTitleVisible();

  // config 파생 배열은 매 렌더 새로 만들어진다. 훅 의존성으로 흘러가면
  // "effect → setState → 리렌더 → 새 배열 → effect" 루프가 되므로 여기서 고정한다.
  const items = useMemo(() => readPanelItems('network', config), [config]);
  const selected = useMemo(() => readInterfaces(config), [config]);
  const maxCols = readMaxCols(config, DEFAULT_MAX_COLS, items.length);
  const refreshMs = readRefreshMs(config, DEFAULT_REFRESH_MS);
  const windowSec = readWindowSec(config, DEFAULT_WINDOW_SEC);
  const unit = normalizeUnitTime(config?.unitTime);

  const [gridRef, cols] = useAutoColumns(maxCols, MIN_CHART_WIDTH);

  const { series, missing } = useNetworkSeries(refreshMs, selected, unit);

  // 대상이 없으면 합산 하나만 본다(훅과 같은 규칙).
  const interfaces = useMemo(
    () => (selected.length > 0 ? selected : [TOTAL_INTERFACE]),
    [selected],
  );
  const colors = useMemo(() => interfaceColors(interfaces), [interfaces]);

  // 축은 표시 구간 전체를 그린다 — 데이터가 덜 모여도 좁아지지 않는다.
  const windowMs = windowSec * 1_000;

  return (
    <div className="flex min-h-0 flex-1 flex-col rounded-lg bg-(--color-bg-surface) p-4 shadow">
      {showTitle && (
        <div className="mb-3 flex shrink-0 items-center gap-2">
          <Network className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
          <span className="truncate text-sm font-medium text-(--color-text-primary)">{title}</span>
        </div>
      )}

      {missing.length > 0 && (
        <p
          data-testid="monitor-network-missing"
          className="mb-2 shrink-0 rounded bg-yellow-50 px-2 py-1 text-[11px] text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-300"
        >
          {`${t('monitoring.networkMissing')}: ${missing.join(', ')}`}
        </p>
      )}

      {items.length === 0 ? (
        <p
          data-testid="monitor-network-panel-empty"
          className="flex flex-1 items-center justify-center text-xs text-(--color-text-muted)"
        >
          {t('monitoring.emptyPanel')}
        </p>
      ) : (
        <div
          ref={gridRef}
          data-testid="monitor-network-panel-grid"
          data-cols={cols}
          className="grid min-h-0 flex-1 auto-rows-min gap-3 overflow-y-auto"
          style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))` }}
        >
          {items.map((key) => (
            <NetworkChart
              key={key}
              channel={key as NetworkChannel}
              series={series}
              interfaces={interfaces}
              colors={colors}
              unit={unit}
              windowMs={windowMs}
            />
          ))}
        </div>
      )}
    </div>
  );
}
