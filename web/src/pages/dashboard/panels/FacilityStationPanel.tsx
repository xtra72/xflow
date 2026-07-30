// Facility 역사 패널 (SPEC-FACILITY-DASHBOARD-001 M3, REQ-FACDASH-001-02-*, 06-*).
//
// config 의 agentId + station 으로 로스터를 조회해 한 역사의 (1) 통계, (2) 기기별 상태 목록,
// (3) 역사 일괄 제어(station 셀렉터 fan-out)를 렌더한다. 집계는 stationSummary(롤업만, UB-001),
// fan-out 은 useXsfmControl(FacilityBulkControl)을 호출만 한다. 미등록 역사/기기 없음은
// 안내 + 일괄 제어 비활성(REQ-02-05, 06-03).

import { HardDrive } from 'lucide-react';

import { useFacilityRoster } from '@/hooks/useXsfmControl';
import { stationSummary } from '@/lib/facilityAggregation';
import { useTranslation } from '@/lib/i18n';
import {
  FacilityBulkControl,
  FacilityDeviceRow,
  StatTiles,
  type DeviceLabelMode,
} from './facilityShared';

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
  // C4: 역사 통계(StatTiles) 표시 여부(config, 영속, 기본 true).
  const showStats = (config.showStats as boolean | undefined) ?? true;
  // 개별 기기 라벨 표시 방식(config, 영속, 기본 placeIndex).
  const deviceLabelMode = (config.deviceLabelMode as DeviceLabelMode | undefined) ?? 'placeIndex';
  // 오프라인을 꺼짐으로 표시(config, 영속, 기본 false — item 2). 라인 패널과 동일한 옵션.
  const offlineAsOff = (config.offlineAsOff as boolean | undefined) ?? false;

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
    // C1: 우상단 식별자는 호선(line)만 표시한다(레지스트리 해석값).
    <Shell title={title} line={summary.line}>
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

      {/* (1) 역사 통계(C4: showStats 로 토글). */}
      {showStats && <StatTiles stats={summary.stats} />}

      {/* (2) 기기별 상태 목록 + 목록 제목 우측 일괄 제어(C5). */}
      <div className="space-y-1">
        {/* 헤더 행에 기기 카드와 동일한 px-2.5 를 주어 제목↔기기명, 일괄 제어 버튼↔개별 제어 버튼을 세로로 정렬한다. */}
        <div className="flex items-center justify-between gap-2 px-2.5">
          <span className="text-xs font-semibold text-(--color-text-secondary)">
            {t('dashboard.facility.station.devices')}
          </span>
          {/* C5: 역사 일괄 제어(station 셀렉터)를 "기기 목록" 제목 우측에 인라인 배치. 미등록 비활성. */}
          <FacilityBulkControl
            agentId={agentId}
            selector={{ station }}
            memberCount={stationDevices.length}
            disabled={unregistered}
          />
        </div>
        {stationDevices.length > 0 ? (
          <ul className="space-y-1" data-testid="station-device-list">
            {stationDevices.map((d) => (
              <FacilityDeviceRow
                key={d.device_id}
                agentId={agentId}
                device={d}
                entry={entry}
                labelMode={deviceLabelMode}
                offlineAsOff={offlineAsOff}
              />
            ))}
          </ul>
        ) : (
          <p className="text-[11px] text-(--color-text-muted)">{t('dashboard.facility.noDevices')}</p>
        )}
      </div>
    </Shell>
  );
}

// ---- 로컬 셸 ----

function Shell({
  title,
  line,
  children,
}: {
  title: string;
  line?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 rounded-lg bg-(--color-bg-surface) p-4 shadow">
      <div className="flex shrink-0 items-baseline justify-between gap-2">
        <span className="truncate text-sm font-medium text-(--color-text-primary)">{title}</span>
        {/* C1: 우상단 식별자는 호선만 표시. */}
        {line && <span className="shrink-0 truncate text-xs text-(--color-text-muted)">{line}</span>}
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
