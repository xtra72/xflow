// Facility 그룹 패널 (SPEC-XSFM-GROUP-001 Module 6 M7, REQ-06-05).
//
// 기존 "설비 역사 패널"을 그룹의 한 종류(type=station)로 일반화한 "설비 그룹 패널"이다.
// config 의 agentId 로 그룹 레지스트리(list_groups)와 로스터(list_devices)를 조회해, 모든 그룹을
// 한 종류로 표시한다: 역사(type=station) · 라인(type=line) · 커스텀(type=custom). 각 그룹은
// (1) type 배지 + 멤버 수, (2) 온라인/전원/풍량 통계(StatTiles, REQ-06-06), (3) 그룹 일괄 제어
// (group_id 셀렉터 fan-out)를 렌더한다. 집계는 countStats(롤업만, UB-001), fan-out 은
// FacilityBulkControl(useXsfmControl) 을 호출만 한다(재구현 없음). 멤버는 로스터 대조로 실재
// 디바이스만 통계에 반영한다(백엔드 유령 멤버 필터와 정합).

import { HardDrive, Layers, Lock, Users } from 'lucide-react';

import { useGroups, type Group } from '@/hooks/useGroups';
import { useFacilityRoster } from '@/hooks/useXsfmControl';
import type { AirDevice } from '@/hooks/useStation';
import { countStats } from '@/lib/facilityAggregation';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { FacilityBulkControl, StatTiles } from './facilityShared';

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

/** Facility 그룹 패널. */
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

  const { devices, isLoading, isError } = useFacilityRoster(agentId, refreshMs);
  const { data: groups = [], isLoading: groupsLoading } = useGroups(agentId, refreshMs);

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

  // device_id → device 맵(멤버 통계 롤업용). 로스터에 실재하는 device 만 통계에 포함(유령 멤버 무시).
  const byId = new Map<string, AirDevice>();
  for (const d of devices) byId.set(d.device_id, d);

  return (
    <Shell title={title}>
      <ul className="space-y-2" data-testid="facility-group-list">
        {groups.map((g) => {
          const memberDevices = g.members.map((id) => byId.get(id)).filter((d): d is AirDevice => !!d);
          const stats = countStats(memberDevices);
          const custom = g.type === 'custom';
          return (
            <li
              key={g.id}
              data-testid={`facility-group-item-${g.id}`}
              className="space-y-2 rounded-lg border border-(--color-border-default) px-2.5 py-2"
            >
              <div className="flex items-center justify-between gap-2">
                <div className="flex min-w-0 items-center gap-2">
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
                  <span className="truncate text-sm font-medium text-(--color-text-primary)" title={g.name}>
                    {g.name}
                  </span>
                  <span className="inline-flex shrink-0 items-center gap-1 text-[11px] text-(--color-text-muted)">
                    <Users className="h-3 w-3" aria-hidden="true" />
                    {g.member_count}
                  </span>
                </div>
                {/* 그룹 일괄 제어(group_id 셀렉터). 멤버 0 이면 비활성. */}
                <FacilityBulkControl agentId={agentId} selector={{ group_id: g.id }} memberCount={g.member_count} />
              </div>
              {showStats && <StatTiles stats={stats} />}
            </li>
          );
        })}
      </ul>
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
