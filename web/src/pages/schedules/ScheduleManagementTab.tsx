// SPEC-SCHEDULE-VIEW-001 M5: 스케줄 관리 탭(에이전트별 관리).
//
// useScheduleAggregation 으로 교차-플로우 스케줄을 집계하고 agent 별 그룹(AC-7) +
// 미지정 버킷(AC-8)으로 렌더한다. 각 그룹의 노드마다 NodeScheduleSection(dual-write
// 인스턴스 1개)을 렌더해 Rules of Hooks 를 준수한다. 부분 실패(AC-18)는 성공 그룹을
// 유지한 채 통지 배너를 노출하고, 로딩/빈 상태를 명시적으로 처리한다.

import { useMemo } from 'react';
import { AlertTriangle, CalendarClock } from 'lucide-react';

import { useAgents } from '@/hooks/useAgent';

import NodeScheduleSection from './NodeScheduleSection';
import {
  UNASSIGNED_LABEL,
  groupEntriesByAgent,
  useScheduleAggregation,
  type ScheduleAgentGroup,
} from './scheduleAggregation';

/** agent 그룹 라벨(이름 매핑 우선, 없으면 id). 미지정은 UNASSIGNED_LABEL. */
function groupLabel(group: ScheduleAgentGroup, agentName: Map<string, string>): string {
  if (group.key === null) return UNASSIGNED_LABEL;
  return agentName.get(group.key) ?? group.key;
}

export default function ScheduleManagementTab() {
  const { nodeEntries, isLoading, failedCount, totalFlows, hasError, refetch } =
    useScheduleAggregation();

  // agent id → name 매핑(그룹 헤더 라벨용). 실패해도 id 로 폴백하므로 비필수.
  const { data: agentsResult } = useAgents();
  const agentName = useMemo(() => {
    const m = new Map<string, string>();
    for (const a of agentsResult?.data ?? []) m.set(a.id, a.name);
    return m;
  }, [agentsResult]);

  // agent 그룹화 + 정렬(할당 그룹 라벨 오름차순, 미지정 최후).
  const groups = useMemo(() => {
    const g = groupEntriesByAgent(nodeEntries);
    return g.sort((a, b) => {
      if (a.key === null) return 1;
      if (b.key === null) return -1;
      return groupLabel(a, agentName).localeCompare(groupLabel(b, agentName));
    });
  }, [nodeEntries, agentName]);

  // --- 초기 로딩(엔트리 없음) ---
  if (isLoading && nodeEntries.length === 0) {
    return (
      <div className="space-y-3" data-testid="schedule-loading">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="h-24 animate-pulse rounded-xl bg-(--color-bg-elevated)" />
        ))}
      </div>
    );
  }

  // --- 전면 에러(목록 또는 모든 플로우 실패) ---
  if (hasError && nodeEntries.length === 0) {
    return (
      <div
        className="rounded-md border border-red-200 bg-red-50 p-6 text-center dark:border-red-800 dark:bg-red-900/20"
        data-testid="schedule-error"
      >
        <p className="text-sm text-red-700 dark:text-red-400">스케줄을 불러오지 못했습니다.</p>
        <button
          type="button"
          onClick={refetch}
          className="mt-3 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 dark:bg-red-500 dark:hover:bg-red-600"
        >
          다시 시도
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* 부분 실패 통지(AC-18): 성공 그룹은 유지하고 배너만 노출. */}
      {failedCount > 0 && (
        <div
          className="flex items-center gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-400"
          data-testid="schedule-partial-failure"
        >
          <AlertTriangle className="h-4 w-4 shrink-0" />
          <span>
            {totalFlows}개 플로우 중 {failedCount}개를 불러오지 못했습니다. 일부 스케줄이 누락될 수 있습니다.
          </span>
        </div>
      )}

      {groups.length === 0 ? (
        <div
          className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) py-16 text-center"
          data-testid="schedule-empty"
        >
          <CalendarClock className="mx-auto h-12 w-12 text-gray-300 dark:text-gray-600" />
          <p className="mt-4 text-sm text-(--color-text-muted)">등록된 예약 스케줄이 없습니다.</p>
        </div>
      ) : (
        groups.map((group) => {
          const bucketId = group.key ?? 'unassigned';
          return (
            <section
              key={bucketId}
              className="space-y-2"
              data-testid={`schedule-group-${bucketId}`}
            >
              <h2 className="flex items-center gap-2 text-sm font-bold text-(--color-text-primary)">
                <CalendarClock className="h-4 w-4 text-blue-500" />
                {groupLabel(group, agentName)}
                <span className="text-[11px] font-normal text-(--color-text-muted)">
                  노드 {group.nodes.length}개
                </span>
              </h2>
              <div className="space-y-2">
                {group.nodes.map((entry) => (
                  <NodeScheduleSection
                    key={`${bucketId}:${entry.flowId}:${entry.nodeId}`}
                    entry={entry}
                    agentKey={group.key}
                    modalAgentId={group.key ?? ''}
                    onPersisted={refetch}
                  />
                ))}
              </div>
            </section>
          );
        })
      )}
    </div>
  );
}
