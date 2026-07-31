// SPEC-SCHEDULE-VIEW-001: 스케줄 관리 탭(단일 플랫 표).
//
// useScheduleAggregation 으로 교차-플로우 스케줄을 집계하고, 에이전트별 그룹 섹션 대신
// 모든 노드/스케줄을 하나의 플랫 표로 렌더한다. 표는 상위가 소유한 <thead>(에이전트 |
// 플로우/노드 | 스케줄 | 대상 | 계획 | 동작 | 우선순위 | 상태 | 편집·삭제) + 노드별
// <tbody>(NodeScheduleRows) 로 구성한다. 각 (flowId,nodeId) 노드는 자신의 dual-write
// 인스턴스 하나를 소유하므로 편집/삭제/토글 훅은 행 루프 밖에서 호출된다(Rules of Hooks 준수).
//
// "규칙 추가"는 별도 작성 카드 없이 설정 팝업(FacilityRuleModal)을 직접 연다. 대상 노드는
// 팝업 안 "대상 노드" 셀렉터에서 다른 규칙 설정과 함께 고르며, 노드를 고르면 유도 에이전트가
// 프리필되어 TARGET 열거를 구동한다. 저장 시 상위가 선택 노드에 규칙을 append 하고 저장 시점
// one-shot dual-write(persistScheduleDualWrite)로 각인/지속화한다(훅이 아니므로 Rules of Hooks
// 안전). 부분 실패(AC-18)는 성공 행을 유지한 채 통지 배너를 노출한다.

import { useCallback, useMemo, useState } from 'react';
import { AlertTriangle, CalendarClock, Plus } from 'lucide-react';

import { useAgents } from '@/hooks/useAgent';
import FacilityRuleModal, {
  type FacilityRuleModalNode,
} from '@/pages/dashboard/panels/facilitySchedule/FacilityRuleModal';
import { persistScheduleDualWrite } from '@/pages/dashboard/panels/facilitySchedule/scheduleDualWrite';
import {
  buildScheduleFromDraft,
  emptyDraft,
  upsertScheduleAt,
  type RuleDraft,
  type SerializedSchedule,
} from '@/pages/dashboard/panels/facilitySchedule/facilityScheduleUtils';
import { useUIStore } from '@/stores/uiStore';

import NodeScheduleRows from './NodeScheduleRows';
import { useScheduleAggregation } from './scheduleAggregation';

export default function ScheduleManagementTab() {
  const { nodeEntries, isLoading, failedCount, totalFlows, hasError, refetch } =
    useScheduleAggregation();
  const addNotification = useUIStore((s) => s.addNotification);

  // agent id → name 매핑(에이전트 컬럼 표시용). 실패해도 id 로 폴백하므로 비필수.
  const { data: agentsResult } = useAgents();
  const agentName = useMemo(() => {
    const m = new Map<string, string>();
    for (const a of agentsResult?.data ?? []) m.set(a.id, a.name);
    return m;
  }, [agentsResult]);

  // 규칙 추가 시 TARGET 열거/각인에 쓰는 xsfm 에이전트 목록.
  const xsfmAgents = useMemo(
    () => (agentsResult?.data ?? []).filter((a) => a.type === 'xsfm').map((a) => ({ id: a.id, name: a.name })),
    [agentsResult],
  );

  // 표시 엔트리: 스케줄이 0개인 노드(=최초 규칙 진입점용 빈 노드)는 표에서 숨긴다.
  const displayEntries = useMemo(
    () => nodeEntries.filter((n) => n.schedules.length > 0),
    [nodeEntries],
  );

  // 공유 우선순위 정렬 방향(표 머리글 토글 → 각 노드 tbody 가 자기 스케줄을 정렬).
  const [sortDesc, setSortDesc] = useState(false);

  // --- "규칙 추가" → 설정 팝업(대상 노드 포함)을 직접 연다 ---
  // 대상 노드/각인 에이전트는 팝업 안에서 고르고, 상위는 controlled 상태로 추적한다
  // (저장 시점에 대상 노드를 알기 위함). 저장 시 선택 노드에 append + dual-write.
  const [createOpen, setCreateOpen] = useState(false);
  const [createNodeKey, setCreateNodeKey] = useState('');
  const [createAgent, setCreateAgent] = useState('');

  const openCreate = useCallback(() => {
    setCreateNodeKey('');
    setCreateAgent('');
    setCreateOpen(true);
  }, []);
  const closeCreate = useCallback(() => setCreateOpen(false), []);

  // 팝업 노드 셀렉터 옵션: 모든 trigger 노드(빈 노드 포함). label = `플로우명 / 노드명`.
  const createNodes = useMemo<FacilityRuleModalNode[]>(
    () =>
      nodeEntries.map((e) => ({
        flowId: e.flowId,
        nodeId: e.nodeId,
        label: `${e.flowName} / ${e.nodeName}`,
        derivedAgentIds: e.derivedAgentIds,
      })),
    [nodeEntries],
  );

  // 저장: 선택 노드 엔트리를 해석 → 규칙 조립 → agent_id 각인 → FULL schedules 에 append →
  // 저장 시점 one-shot dual-write. 성공 시 재조회 + 팝업 닫기.
  const handleCreateSave = useCallback(
    (draft: RuleDraft) => {
      const entry = nodeEntries.find((e) => `${e.flowId}:${e.nodeId}` === createNodeKey);
      if (!entry) return;
      const built = buildScheduleFromDraft(draft, undefined);
      if (!built) return; // 유효하지 않으면 모달이 검증으로 막음.
      const stamped: SerializedSchedule = createAgent ? { ...built, agent_id: createAgent } : built;
      const next = upsertScheduleAt(entry.schedules, -1, stamped); // -1 = 말미 append(origin 보존).
      void (async () => {
        const ok = await persistScheduleDualWrite({
          flowId: entry.flowId,
          nodeId: entry.nodeId,
          baseline: entry.nodeConfig,
          nextSchedules: next,
          notify: addNotification,
        });
        if (ok) {
          refetch();
          closeCreate();
        }
      })();
    },
    [nodeEntries, createNodeKey, createAgent, addNotification, refetch, closeCreate],
  );

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
      {/* 상단 액션 바: 규칙 추가 진입점 → 설정 팝업 직접 오픈 */}
      <div className="flex items-center justify-end">
        <button
          type="button"
          onClick={openCreate}
          data-testid="schedule-add-rule"
          className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700"
        >
          <Plus className="h-3.5 w-3.5" /> 규칙 추가
        </button>
      </div>

      {/* 규칙 추가 팝업: 대상 노드 + 규칙 설정을 한 팝업에서. 노드 선택 시 유도 에이전트 프리필. */}
      {createOpen && (
        <FacilityRuleModal
          initial={emptyDraft()}
          agentId=""
          agents={xsfmAgents}
          selectedAgentId={createAgent}
          onAgentChange={setCreateAgent}
          nodes={createNodes}
          selectedNodeKey={createNodeKey}
          onNodeChange={setCreateNodeKey}
          onSave={handleCreateSave}
          onCancel={closeCreate}
        />
      )}

      {/* 부분 실패 통지(AC-18): 성공 행은 유지하고 배너만 노출. */}
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

      {displayEntries.length === 0 ? (
        <div
          className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) py-16 text-center"
          data-testid="schedule-empty"
        >
          <CalendarClock className="mx-auto h-12 w-12 text-gray-300 dark:text-gray-600" />
          <p className="mt-4 text-sm text-(--color-text-muted)">등록된 예약 스케줄이 없습니다.</p>
        </div>
      ) : (
        <div className="overflow-auto rounded-xl border border-(--color-border-default) bg-(--color-bg-surface)">
          <table className="w-full border-collapse text-left text-xs" data-testid="schedule-table">
            <thead>
              <tr className="border-b border-(--color-border-default) text-[11px] text-(--color-text-muted)">
                <th className="px-2 py-1.5 font-medium">에이전트</th>
                <th className="px-2 py-1.5 font-medium">플로우/노드</th>
                <th className="px-2 py-1.5 font-medium">스케줄</th>
                <th className="px-2 py-1.5 font-medium">대상</th>
                <th className="px-2 py-1.5 font-medium">계획</th>
                <th className="px-2 py-1.5 font-medium">동작</th>
                <th
                  className="cursor-pointer select-none px-2 py-1.5 font-medium hover:text-blue-600"
                  onClick={() => setSortDesc((v) => !v)}
                  title="우선순위 정렬"
                >
                  우선순위 {sortDesc ? '▼' : '▲'}
                </th>
                <th className="px-2 py-1.5 font-medium">상태</th>
                <th className="px-2 py-1.5" />
              </tr>
            </thead>
            {displayEntries.map((entry) => (
              <NodeScheduleRows
                key={`${entry.flowId}:${entry.nodeId}`}
                entry={entry}
                agents={xsfmAgents}
                agentName={agentName}
                sortDesc={sortDesc}
                onPersisted={refetch}
              />
            ))}
          </table>
        </div>
      )}
    </div>
  );
}
