// Facility 역사 패널 (SPEC-FACILITY-DASHBOARD-001 M3, REQ-FACDASH-001-02-*, 06-*).
//
// config 의 agentId + station 으로 로스터를 조회해 한 역사의 (1) 통계, (2) 기기별 상태 목록,
// (3) 역사 일괄 제어(station 셀렉터 fan-out)를 렌더한다. 집계는 stationSummary(롤업만, UB-001),
// fan-out 은 useAirpurifierControl(FacilityBulkControl)을 호출만 한다. 미등록 역사/기기 없음은
// 안내 + 일괄 제어 비활성(REQ-02-05, 06-03).

import { Activity, HardDrive, Moon } from 'lucide-react';

import { useFacilityRoster } from '@/hooks/useAirpurifierControl';
import { stationSummary } from '@/lib/facilityAggregation';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { FacilityBulkControl, StatTiles } from './facilityShared';

interface FacilityStationPanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

/** Facility 역사 패널. */
export default function FacilityStationPanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange: _onConfigChange,
  onTitleChange: _onTitleChange,
}: FacilityStationPanelProps) {
  const { t } = useTranslation();
  const agentId = (config.agentId as string | undefined) ?? '';
  const station = config.station as string | undefined;
  const refreshMs = config.refreshMs as number | undefined;

  const { devices, stations, isLoading, isError } = useFacilityRoster(agentId, refreshMs);

  if (!station || !agentId) {
    return <Shell title={title}>{notice(t('dashboard.facility.notConfigured'))}</Shell>;
  }
  if (isLoading) {
    return (
      <Shell title={title}>
        <div className="flex flex-1 items-center justify-center">
          <div className="h-5 w-5 animate-spin rounded-full border-2 border-(--color-border-default) border-t-blue-600" />
        </div>
      </Shell>
    );
  }
  if (isError) {
    return <Shell title={title}>{notice(t('dashboard.facility.loadError'))}</Shell>;
  }

  const entry = stations.find((s) => s.station === station);
  const stationDevices = devices.filter((d) => d.station === station);
  const summary = stationSummary(station, stationDevices, entry);
  const unregistered = !entry;
  const empty = stationDevices.length === 0;

  return (
    <Shell title={title} displayName={summary.displayName} line={summary.line}>
      {/* 미등록/기기 없음 안내(REQ-02-05) */}
      {unregistered && (
        <p className="rounded-lg bg-amber-50 px-2.5 py-1.5 text-[11px] text-amber-600 dark:bg-amber-900/20 dark:text-amber-400">
          {t('dashboard.facility.unregisteredStation')}
        </p>
      )}
      {empty && (
        <p className="rounded-lg bg-(--color-bg-elevated) px-2.5 py-1.5 text-[11px] text-(--color-text-muted)">
          {t('dashboard.facility.noDevices')}
        </p>
      )}

      {/* (1) 역사 통계 */}
      <StatTiles stats={summary.stats} />

      {/* (2) 기기별 상태 목록 */}
      <div className="space-y-1">
        <span className="text-xs font-semibold text-(--color-text-secondary)">
          {t('dashboard.facility.station.devices')}
        </span>
        {stationDevices.length > 0 ? (
          <ul className="space-y-1" data-testid="station-device-list">
            {stationDevices.map((d) => (
              <li
                key={d.device_id}
                className="flex items-center justify-between gap-2 rounded-lg border border-(--color-border-default) px-2.5 py-1.5 text-xs"
              >
                <span className="min-w-0 flex-1 truncate font-mono text-(--color-text-primary)" title={d.device_id}>
                  {d.name || d.device_id}
                </span>
                <span className="shrink-0 text-[11px] text-(--color-text-muted)">
                  {(d.place || '-') + ' · #' + d.index}
                </span>
                <span className="shrink-0 text-[11px] text-(--color-text-secondary)">
                  {d.power
                    ? t('dashboard.facility.device.on') + ' · ' + d.fan_speed
                    : t('dashboard.facility.device.off')}
                </span>
                <span
                  className={cn(
                    'inline-flex shrink-0 items-center rounded-full px-1.5 py-0.5',
                    d.online
                      ? 'bg-blue-50 text-blue-500 dark:bg-blue-900/30 dark:text-blue-400'
                      : 'bg-slate-100 text-slate-400 dark:bg-slate-800 dark:text-slate-500',
                  )}
                >
                  {d.online ? (
                    <Activity className="h-3 w-3" aria-label={t('dashboard.facility.device.online')} />
                  ) : (
                    <Moon className="h-3 w-3" aria-label={t('dashboard.facility.device.offline')} />
                  )}
                </span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-[11px] text-(--color-text-muted)">{t('dashboard.facility.noDevices')}</p>
        )}
      </div>

      {/* (3) 역사 일괄 제어(station 셀렉터). 미등록 시 비활성(REQ-02-05). */}
      <FacilityBulkControl
        agentId={agentId}
        selector={{ station }}
        memberCount={stationDevices.length}
        disabled={unregistered}
      />
    </Shell>
  );
}

// ---- 로컬 셸 ----

function Shell({
  title,
  displayName,
  line,
  children,
}: {
  title: string;
  displayName?: string;
  line?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 rounded-lg bg-(--color-bg-surface) p-4 shadow">
      <div className="flex shrink-0 items-baseline justify-between gap-2">
        <span className="truncate text-sm font-medium text-(--color-text-primary)">{title}</span>
        {displayName && (
          <span className="shrink-0 truncate text-xs text-(--color-text-muted)">
            {line ? `${line} · ${displayName}` : displayName}
          </span>
        )}
      </div>
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto">{children}</div>
    </div>
  );
}

function notice(text: string) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center">
      <HardDrive className="mb-2 h-6 w-6 text-(--color-text-muted)" />
      <p className="text-xs text-(--color-text-muted)">{text}</p>
    </div>
  );
}
