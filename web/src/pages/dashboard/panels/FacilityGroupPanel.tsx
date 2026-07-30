// Facility 그룹 패널 (SPEC-XSFM-GROUP-001 Module 6 M7, REQ-06-05).
//
// "설비 역사 패널"을 그룹의 한 종류로 일반화한 "설비 그룹 패널"이다. 기존 평면(flat) 목록을
// 개별 그룹 드릴다운으로 개편해 역사 패널과 동형(同型)으로 만든다:
//   (1) 그룹 선택기: 모든 그룹(역사=station · 라인=line · 커스텀=custom)을 type 배지 + 멤버 수와
//       함께 나열하고, 하나를 선택하면 상세를 보여준다.
//   (2) 선택된 그룹 상세 = 역사 패널 처치: (a) 그룹 통계(StatTiles, countStats 롤업만), (b) 그룹
//       일괄 제어(FacilityBulkControl, group_id 셀렉터 fan-out), (c) 소속 디바이스 개별 제어
//       (FacilityDeviceRow 재사용 — 기기별 전원 OFF/풍량 1/2/3).
// 집계·fan-out·개별 제어는 재구현하지 않고(UB-001) 공용 컴포넌트/함수를 호출만 한다. 멤버는
// 로스터 대조로 실재 디바이스만 통계·목록에 반영한다(백엔드 유령 멤버 필터 RD-3 와 정합).
// 선택 그룹은 런타임 로컬 상태이며(라인 패널 심플/상세 토글과 동일한 비영속 패턴), config.groupId 가
// 있으면 초기 선택으로 사용한다. 패널 등록 타입(facility-group)/config 시맨틱(agentId/refreshMs/showStats)은 보존한다.

import { useMemo, useState } from 'react';

import { HardDrive, Layers, Lock, Users } from 'lucide-react';

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

/** Facility 그룹 패널(개별 그룹 드릴다운). */
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
  // 그룹별 통계(StatTiles) 표시 여부(config, 영속, 기본 true). 역사 패널 showStats 와 동형.
  const showStats = (config.showStats as boolean | undefined) ?? true;
  // 개별 기기 라벨 표시 방식(config, 영속, 기본 placeIndex — 역사 패널과 동일).
  const deviceLabelMode = (config.deviceLabelMode as DeviceLabelMode | undefined) ?? 'placeIndex';
  // 오프라인을 꺼짐으로 표시(config, 영속, 기본 false — 역사 패널과 동일).
  const offlineAsOff = (config.offlineAsOff as boolean | undefined) ?? false;

  const { devices, stations, isLoading, isError } = useFacilityRoster(agentId, refreshMs);
  const { data: groups = [], isLoading: groupsLoading } = useGroups(agentId, refreshMs);

  // 선택된 그룹 id(런타임 로컬 상태, 비영속). config.groupId 가 있으면 초기 선택으로 사용한다.
  const [selectedId, setSelectedId] = useState<string | null>(
    (config.groupId as string | undefined) ?? null,
  );

  // 실제로 표시할 그룹: 선택 id 가 유효하면 그 그룹, 아니면 첫 그룹으로 자동 선택(드릴다운 기본 진입).
  const selectedGroup = useMemo(() => {
    if (groups.length === 0) return undefined;
    const found = selectedId ? groups.find((g) => g.id === selectedId) : undefined;
    return found ?? groups[0];
  }, [groups, selectedId]);

  if (!agentId) {
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

  if (groups.length === 0) {
    return <Shell title={title}>{notice(t('dashboard.facility.group.noGroups'))}</Shell>;
  }

  // device_id → device 맵(멤버 통계 롤업/개별 제어용). 로스터에 실재하는 device 만 포함(유령 멤버 무시).
  const byId = new Map<string, AirDevice>();
  for (const d of devices) byId.set(d.device_id, d);
  // station code → 레지스트리 엔트리 맵(개별 기기 라벨 place 해석용).
  const stationById = new Map<string, AirStation>();
  for (const s of stations) if (s.station) stationById.set(s.station, s);

  // 선택 그룹의 소속 디바이스(로스터 대조 — 실재하는 것만, 유령 멤버 제외).
  const memberDevices = selectedGroup
    ? selectedGroup.members.map((id) => byId.get(id)).filter((d): d is AirDevice => !!d)
    : [];
  const stats = countStats(memberDevices);

  return (
    <Shell title={title}>
      {/* (1) 그룹 선택기: 모든 그룹을 type 배지 + 멤버 수와 함께 나열. 선택 시 상세 표시. */}
      <div className="space-y-1">
        <span className="text-xs font-semibold text-(--color-text-secondary)">
          {t('dashboard.facility.group.selectGroup')}
        </span>
        <ul className="space-y-1" data-testid="facility-group-list">
          {groups.map((g) => {
            const active = selectedGroup?.id === g.id;
            const custom = g.type === 'custom';
            return (
              <li key={g.id}>
                <button
                  type="button"
                  data-testid={`facility-group-item-${g.id}`}
                  aria-pressed={active}
                  onClick={() => setSelectedId(g.id)}
                  className={cn(
                    'flex w-full items-center gap-2 rounded-lg border px-2.5 py-1.5 text-left transition-colors',
                    active
                      ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20'
                      : 'border-(--color-border-default) hover:bg-(--color-bg-elevated)',
                  )}
                >
                  <span
                    data-testid={`facility-group-type-${g.id}`}
                    className={cn(
                      'inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-medium',
                      TYPE_BADGE[g.type],
                    )}
                  >
                    {!custom && <Lock className="h-2.5 w-2.5" aria-hidden="true" />}
                    {t(`dashboard.facility.group.type.${g.type}`)}
                  </span>
                  <span
                    className="truncate text-sm font-medium text-(--color-text-primary)"
                    title={g.name}
                  >
                    {g.name}
                  </span>
                  <span className="ml-auto inline-flex shrink-0 items-center gap-1 text-[11px] text-(--color-text-muted)">
                    <Users className="h-3 w-3" aria-hidden="true" />
                    {g.member_count}
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      </div>

      {/* (2) 선택된 그룹 상세: 통계 + 일괄 제어 + 소속 디바이스 개별 제어(역사 패널 동형). */}
      {selectedGroup && (
        <div className="space-y-2" data-testid="facility-group-detail">
          {/* 상세 헤더: 그룹명 + 그룹 일괄 제어(group_id 셀렉터). 멤버 0 이면 비활성. */}
          <div className="flex items-center justify-between gap-2">
            <span
              className="truncate text-sm font-semibold text-(--color-text-primary)"
              title={selectedGroup.name}
            >
              {selectedGroup.name}
            </span>
            <FacilityBulkControl
              agentId={agentId}
              selector={{ group_id: selectedGroup.id }}
              memberCount={selectedGroup.member_count}
            />
          </div>

          {/* (a) 그룹 통계(로스터 대조 롤업). */}
          {showStats && <StatTiles stats={stats} />}

          {/* (c) 소속 디바이스 개별 제어(FacilityDeviceRow 재사용). */}
          <div className="space-y-1">
            <span className="text-xs font-semibold text-(--color-text-secondary)">
              {t('dashboard.facility.group.members')}
            </span>
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
        </div>
      )}
    </Shell>
  );
}

// ---- 로컬 셸 (역사/라인 패널과 동형) ----

function Shell({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 rounded-lg bg-(--color-bg-surface) p-4 shadow">
      <div className="flex shrink-0 items-baseline justify-between gap-2">
        <span className="truncate text-sm font-medium text-(--color-text-primary)">{title}</span>
        <Layers className="h-4 w-4 shrink-0 text-(--color-text-muted)" aria-hidden="true" />
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
