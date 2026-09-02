// 스토리지 지표 패널.
//
// 마운트를 고르면 고른 것마다 한 행씩, 고르지 않으면 전체 합계 한 줄을 그린다.
// 네트워크 패널과 같은 규약이다 — 대상 선택이 "종합"과 "파티션별"을 가른다.
//
// 같은 볼륨의 여러 마운트(macOS 의 `/` 와 `/System/Volumes/Data`)를 합치지 않는다.
// 어떤 마운트가 같은 볼륨인지는 OS 마다 다르고 에이전트도 알지 못하므로, 임의로
// 합치면 사용자가 고른 대상이 화면에서 사라진다. 정확한 종합이 필요하면 에이전트
// 설정에서 마운트를 골라 두는 것이 옳은 해법이다.

import { useMemo } from 'react';
import { HardDrive } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { formatBytes } from '@/pages/monitoring/networkSeries';

import { SysMetricsPanelShell } from './SysMetricsPanelShell';
import { useAutoColumns } from '../monitor/useAutoColumns';
import {
  STORAGE_ITEMS,
  readAccent,
  readAgentRef,
  readItems,
  readMaxCols,
  readRefreshMs,
  readTargets,
  type StorageItemKey,
} from './sysMetricsPanelConfig';
import { isRatioStyle, resolveItemOptions } from './sysMetricsItemOptions';
import { SysMetricsItemView } from './SysMetricsItemView';
import {
  isCollected,
  resolveTargets,
  sumStorage,
  type MetricGroup,
} from './sysMetricsSeries';
import { useSysMetricsSnapshot } from './useSysMetricsSnapshot';

/** 기본 폴링 주기 */
const DEFAULT_REFRESH_MS = 5_000;
/** 한 행이 읽히려면 필요한 최소 폭(px) */
const MIN_ROW_WIDTH = 220;

interface SysMetricsStoragePanelProps {
  panelId: string;
  title: string;
  config?: Record<string, unknown>;
}

/** 한 행이 표시하는 값 */
interface StorageRow {
  name: string;
  usagePercent: number;
  usedBytes: number;
  freeBytes: number;
  totalBytes: number;
}

/** 인스턴스 지표에서 행 하나를 만든다. */
function toRow(name: string, metrics: MetricGroup): StorageRow {
  return {
    name,
    usagePercent: metrics.usage_percent ?? 0,
    usedBytes: metrics.used_bytes ?? 0,
    freeBytes: metrics.free_bytes ?? 0,
    totalBytes: metrics.total_bytes ?? 0,
  };
}

export function SysMetricsStoragePanel({ panelId, title, config }: SysMetricsStoragePanelProps) {
  const { t } = useTranslation();

  const { agentId, agentName } = readAgentRef(config);
  const refreshMs = readRefreshMs(config, DEFAULT_REFRESH_MS);
  const selected = readTargets(config, 'mountpoints');
  const items = readItems<StorageItemKey>(config, STORAGE_ITEMS, STORAGE_ITEMS);
  const maxCols = readMaxCols(config, 1);
  const [gridRef, cols] = useAutoColumns(maxCols, MIN_ROW_WIDTH);
  const { accentColor } = readAccent(config);

  const { snapshot, state } = useSysMetricsSnapshot(agentId, refreshMs);

  const collected = snapshot ? isCollected(snapshot, 'storage') : true;
  const targets = snapshot ? resolveTargets(snapshot.storage, selected) : [];
  const missing = selected.filter((name) => !targets.includes(name));

  /** 그릴 행 목록. 대상을 고르지 않았으면 합계 한 줄이다. */
  const rows: StorageRow[] = (() => {
    if (!snapshot) return [];
    if (targets.length > 0) {
      return targets.map((name) => toRow(name, snapshot.storage?.[name] ?? {}));
    }
    const totals = sumStorage(snapshot);
    return [
      {
        name: t('sysmetrics.storage.total'),
        usagePercent: totals.usagePercent,
        usedBytes: totals.usedBytes,
        freeBytes: totals.freeBytes,
        totalBytes: totals.totalBytes,
      },
    ];
  })();

  // 마운트별 확정 옵션. 마운트 이름이 곧 항목 키다.
  const rowOptions = useMemo(
    () => (name: string) =>
      resolveItemOptions(config, name, 'ratio', 'sysmetrics-storage'),
    [config],
  );

  const labelColor = accentColor('label');

  return (
    <SysMetricsPanelShell
      panelId={panelId}
      title={title}
      icon={HardDrive}
      headerColor={accentColor('header')}
      state={state}
      agentName={agentName}
    >
      {!collected ? (
        <p
          data-testid="sysmetrics-storage-not-collected"
          className="flex flex-1 items-center justify-center text-xs text-(--color-text-muted)"
        >
          {t('sysmetrics.state.notCollected')}
        </p>
      ) : (
        <>
          {missing.length > 0 && (
            <p
              data-testid="sysmetrics-storage-missing"
              className="mb-2 shrink-0 text-xs text-(--color-text-muted)"
            >
              {t('sysmetrics.state.missingTargets')}: {missing.join(', ')}
            </p>
          )}
          <div
            ref={gridRef}
            data-testid="sysmetrics-storage-rows"
            data-rows={String(rows.length)}
            data-cols={cols}
            className="grid min-h-0 flex-1 gap-3 overflow-y-auto"
            style={{
              gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
              // 행이 남은 높이를 나눠 갖는다 — auto-rows-min 이면 내용 높이만
              // 차지해 패널을 늘려도 아래가 빈 채로 남는다. 읽을 수 있는 최소
              // 높이는 지키고, 그보다 좁아지면 스크롤한다.
              gridAutoRows: 'minmax(90px, 1fr)',
            }}
          >
            {rows.map((row) => {
              // 마운트마다 스타일을 따로 고를 수 있다. 사용률은 0~100 비율이므로
              // 게이지·진행막대가 모두 뜻이 통한다.
              const opts = rowOptions(row.name);
              const detail = (
                <div className="flex flex-wrap gap-x-3">
                  {items.includes('used') && (
                    <span>
                      {t('sysmetrics.storage.used')} {formatBytes(row.usedBytes)}
                    </span>
                  )}
                  {items.includes('free') && (
                    <span>
                      {t('sysmetrics.storage.free')} {formatBytes(row.freeBytes)}
                    </span>
                  )}
                  {items.includes('total') && (
                    <span>
                      {t('sysmetrics.storage.capacity')} {formatBytes(row.totalBytes)}
                    </span>
                  )}
                </div>
              );

              return (
                <div key={row.name} data-testid="sysmetrics-storage-row" data-name={row.name}>
                  <SysMetricsItemView
                    options={opts}
                    label={row.name}
                    value={`${row.usagePercent.toFixed(1)}%`}
                    // 게이지·진행 막대는 그 자체가 사용률 표시다. '사용률' 항목
                    // 토글로 값을 끊으면 스타일을 골라도 타일로 떨어진다 — 토글은
                    // 타일에서 숫자를 보일지만 정한다.
                    percent={
                      isRatioStyle(opts.style) || items.includes('usage')
                        ? row.usagePercent
                        : undefined
                    }
                    hint={detail}
                    labelColor={labelColor}
                  />
                </div>
              );
            })}
          </div>
        </>
      )}
    </SysMetricsPanelShell>
  );
}

export default SysMetricsStoragePanel;
