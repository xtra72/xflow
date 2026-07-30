// Facility 그룹 패널 (SPEC-XSFM-GROUP-001 Module 6, REQ-06-05).
//
// config.groupId 로 지정된 "단일 그룹"(역사=station / 라인=line / 커스텀=custom)을 기존 설비 역사
// 패널과 동형으로 렌더한다. 즉 역사 패널을 "그룹의 한 종류(station)"로 일반화한 것으로, 인패널
// 드릴다운(그룹 선택기)은 두지 않는다 — 대상 그룹은 패널 생성 시 선택되어 config.groupId 로 영속되며
// 설정 다이얼로그에서 변경한다.
//
// 렌더 구성(역사 패널 동형):
//   - 헤더 우상단: 그룹 type 배지(역사 패널의 호선 표기 자리를 대체).
//   - (a) 그룹 통계(StatTiles, showStats 토글, countStats 롤업).
//   - (b) 소속 디바이스 목록 제목 우측 그룹 일괄 제어(FacilityBulkControl, group_id 셀렉터 fan-out).
//   - (c) 소속 디바이스 개별 제어(FacilityDeviceRow 재사용 — 기기별 전원 OFF/풍량 1/2/3).
// 집계·fan-out·개별 제어는 재구현하지 않고(UB-001) 공용 컴포넌트/함수를 호출만 한다. 멤버는 로스터
// 대조로 실재 디바이스만 통계·목록에 반영한다(RD-3, 백엔드 유령 멤버 필터와 정합).
//
// 하위호환: config.groupId 가 없고 레거시 config.station 이 있으면 groupId = "station:<station>" 로
// 파생한다. 이로써 기존 설비 역사 패널({type:'facility-station', config:{station}}) 스냅샷이
// facility-group(또는 facility-station 별칭 디스패치)로도 그대로 렌더된다.

import { HardDrive, Layers, Lock } from 'lucide-react';

import { useGroups, type Group } from '@/hooks/useGroups';
import { useFacilityRoster } from '@/hooks/useXsfmControl';
import type { AirDevice, AirStation } from '@/hooks/useStation';
import { countStats } from '@/lib/facilityAggregation';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import {
  FacilityBulkControl,
  FacilityDeviceRow,
  StatTiles,
  type DeviceLabelMode,
} from './facilityShared';

/** type 배지 색상(custom 파랑 · station 초록 · line 보라) — 그룹 탭과 동일 팔레트. */
const TYPE_BADGE: Record<Group['type'], string> = {
  custom: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
  station: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
  line: 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400',
};

interface FacilityGroupPanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

/** Facility 그룹 패널(config.groupId 로 지정된 단일 그룹). */
export default function FacilityGroupPanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange: _onConfigChange,
  onTitleChange: _onTitleChange,
}: FacilityGroupPanelProps) {
  const { t } = useTranslation();
  const agentId = (config.agentId as string | undefined) ?? '';
  const refreshMs = config.refreshMs as number | undefined;
  // 그룹 통계(StatTiles) 표시 여부(config, 영속, 기본 true). 역사 패널 showStats 와 동형.
  const showStats = (config.showStats as boolean | undefined) ?? true;
  // 개별 기기 라벨 표시 방식(config, 영속, 기본 placeIndex — 역사 패널과 동일).
  const deviceLabelMode = (config.deviceLabelMode as DeviceLabelMode | undefined) ?? 'placeIndex';
  // 오프라인을 꺼짐으로 표시(config, 영속, 기본 false — 역사 패널과 동일).
  const offlineAsOff = (config.offlineAsOff as boolean | undefined) ?? false;

  // 대상 그룹 id: config.groupId 우선, 없으면 레거시 config.station → "station:<code>" 파생(하위호환).
  const rawGroupId = config.groupId as string | undefined;
  const legacyStation = config.station as string | undefined;
  const groupId = rawGroupId || (legacyStation ? `station:${legacyStation}` : undefined);

  const { devices, stations, isLoading, isError } = useFacilityRoster(agentId, refreshMs);
  const { data: groups = [], isLoading: groupsLoading } = useGroups(agentId, refreshMs);

  if (!agentId || !groupId) {
    return <Shell title={title}>{notice(t('dashboard.facility.notConfigured'))}</Shell>;
  }
  if (isLoading || groupsLoading) {
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

  // 대상 그룹 조회(단일). 미존재(미등록/삭제된 groupId) 시 안내.
  const group = groups.find((g) => g.id === groupId);
  if (!group) {
    return <Shell title={title}>{notice(t('dashboard.facility.group.noGroups'))}</Shell>;
  }

  // device_id → device 맵(멤버 통계 롤업/개별 제어용). 로스터에 실재하는 device 만 포함(유령 멤버 무시).
  const byId = new Map<string, AirDevice>();
  for (const d of devices) byId.set(d.device_id, d);
  // station code → 레지스트리 엔트리 맵(개별 기기 라벨 place 해석용, 멤버가 여러 역사에 걸칠 수 있음).
  const stationById = new Map<string, AirStation>();
  for (const s of stations) if (s.station) stationById.set(s.station, s);

  // 그룹 소속 디바이스(로스터 대조 — 실재하는 것만, 유령 멤버 제외 RD-3).
  const memberDevices = group.members
    .map((id) => byId.get(id))
    .filter((d): d is AirDevice => !!d);
  const stats = countStats(memberDevices);

  return (
    <Shell title={title} badge={<TypeBadge group={group} label={t(`dashboard.facility.group.type.${group.type}`)} />}>
      {/* (a) 그룹 통계(showStats 로 토글, 로스터 대조 롤업). */}
      {showStats && <StatTiles stats={stats} />}

      {/* (b/c) 소속 디바이스 목록 + 목록 제목 우측 그룹 일괄 제어(group_id 셀렉터). */}
      <div className="space-y-1">
        {/* 헤더 행에 기기 카드와 동일한 px-2.5 를 주어 제목↔기기명, 일괄 제어 버튼↔개별 제어 버튼을 세로 정렬한다. */}
        <div className="flex items-center justify-between gap-2 px-2.5">
          <span className="text-xs font-semibold text-(--color-text-secondary)">
            {t('dashboard.facility.group.members')}
          </span>
          {/* 그룹 일괄 제어(group_id 셀렉터 fan-out). 멤버 0 이면 비활성(REQ-06-03). */}
          <FacilityBulkControl
            agentId={agentId}
            selector={{ group_id: group.id }}
            memberCount={group.member_count}
          />
        </div>
        {memberDevices.length > 0 ? (
          <ul className="space-y-1" data-testid="facility-group-device-list">
            {memberDevices.map((d) => (
              <FacilityDeviceRow
                key={d.device_id}
                agentId={agentId}
                device={d}
                entry={stationById.get(d.station)}
                labelMode={deviceLabelMode}
                offlineAsOff={offlineAsOff}
              />
            ))}
          </ul>
        ) : (
          <p className="text-[11px] text-(--color-text-muted)">
            {t('dashboard.facility.group.noMembers')}
          </p>
        )}
      </div>
    </Shell>
  );
}

// ---- 로컬 셸 (역사/라인 패널과 동형 — 우상단 식별자 자리에 그룹 type 배지) ----

function TypeBadge({ group, label }: { group: Group; label: string }) {
  const custom = group.type === 'custom';
  return (
    <span
      data-testid="facility-group-type"
      className={cn(
        'inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-medium',
        TYPE_BADGE[group.type],
      )}
    >
      {!custom && <Lock className="h-2.5 w-2.5" aria-hidden="true" />}
      {label}
    </span>
  );
}

function Shell({
  title,
  badge,
  children,
}: {
  title: string;
  badge?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 rounded-lg bg-(--color-bg-surface) p-4 shadow">
      <div className="flex shrink-0 items-baseline justify-between gap-2">
        <span className="truncate text-sm font-medium text-(--color-text-primary)">{title}</span>
        {/* 우상단 식별자: 그룹 type 배지(없으면 레이어 아이콘 폴백 — notConfigured/로딩 등). */}
        {badge ?? <Layers className="h-4 w-4 shrink-0 text-(--color-text-muted)" aria-hidden="true" />}
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
