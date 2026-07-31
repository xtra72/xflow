// SPEC-SCHEDULE-VIEW-001: 스케줄 관리 탭 — 단일 플랫 표의 노드별 행 그룹(tbody).
//
// 관리 탭은 더 이상 에이전트별 섹션 카드로 그룹핑하지 않고 모든 노드/스케줄을 하나의
// 플랫 표로 렌더한다. 표의 <thead>(에이전트 | 플로우/노드 | 스케줄 | 대상 | 계획 | 동작 |
// 우선순위 | 상태 | 편집·삭제)는 상위 탭이 소유하고, 각 (flowId,nodeId) 노드는 이
// 컴포넌트가 자신의 <tbody> 안에 스케줄을 <tr> 로 렌더한다. 하나의 <table> 안에 여러
// <tbody> 를 두는 것은 유효한 HTML 이며 연속된 하나의 표로 렌더된다(섹션 헤더 없음).
//
// Rules of Hooks: (flowId,nodeId) 별 useScheduleDualWrite 인스턴스 하나가 필요하므로
// 훅은 행 루프가 아니라 노드 단위 컴포넌트에서 호출한다. 각 행은 스케줄의 해석 에이전트를
// 이름으로 표시하고(agent id → 이름), 편집/삭제/토글은 FULL schedules 배열의 원본 인덱스
// (rule.index)로 매핑해 origin 을 보존한다. dual-write 성공 시 onPersisted 로 상위 재조회.

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Pencil, Trash2 } from 'lucide-react';

import FacilityRuleModal from '@/pages/dashboard/panels/facilitySchedule/FacilityRuleModal';
import { useScheduleDualWrite } from '@/pages/dashboard/panels/facilitySchedule/useScheduleDualWrite';
import {
  buildScheduleFromDraft,
  formatActionLabel,
  formatPlanSummary,
  formatTargetDescription,
  formatValidity,
  parseRules,
  ruleToDraft,
  sortRulesByPriority,
  targetKindLabel,
  toggleEnabledAt,
  upsertScheduleAt,
  type FacilityRule,
  type RuleDraft,
  type SerializedSchedule,
} from '@/pages/dashboard/panels/facilitySchedule/facilityScheduleUtils';
import { cn } from '@/lib/utils/cn';

import { UNASSIGNED_LABEL, resolveScheduleAgent, type ScheduleNodeEntry } from './scheduleAggregation';

/** 플랫 표 컬럼 수(에이전트·플로우/노드·스케줄·대상·계획·동작·우선순위·상태·편집삭제). */
export const SCHEDULE_TABLE_COLUMNS = 9;

/** agent id → 표시 이름(없으면 id, null 이면 미지정). */
function agentDisplayName(agentId: string | null, agentName: Map<string, string>): string {
  if (agentId === null) return UNASSIGNED_LABEL;
  return agentName.get(agentId) ?? agentId;
}

interface NodeScheduleRowsProps {
  /** 소유 노드 엔트리(FULL schedules + baseline config + origin). */
  entry: ScheduleNodeEntry;
  /** 모달 TARGET 열거/각인용 선택 가능 xsfm 에이전트 목록. */
  agents: { id: string; name: string }[];
  /** agent id → 이름 매핑(에이전트 컬럼 표시용). */
  agentName: Map<string, string>;
  /** 상위 공유 우선순위 정렬 방향(표 머리글 토글). */
  sortDesc: boolean;
  /** dual-write 성공 시 상위 집계 재조회 트리거. */
  onPersisted: () => void;
}

/**
 * 한 노드의 스케줄을 플랫 표의 <tbody> 로 렌더한다. 그룹 필터 없이 노드의 모든 스케줄을
 * 표시하며, 각 행은 자신의 해석 에이전트를 이름으로 노출한다. 편집 시 모달의 에이전트
 * 셀렉터로 대상을 열거/각인한다(agent_id).
 */
export default function NodeScheduleRows({
  entry,
  agents,
  agentName,
  sortDesc,
  onPersisted,
}: NodeScheduleRowsProps) {
  const { flowId, nodeId, nodeName, flowName, nodeConfig, schedules } = entry;

  // dual-write 훅 — (flowId,nodeId) 별 인스턴스 하나. Rules of Hooks 준수(행 루프 밖).
  const { persist, saving, setBaseline } = useScheduleDualWrite({ flowId, nodeId });

  // baseline = 노드 정의 config(FULL config 조립/충돌 감지 기준). 정의 갱신 시 재시드.
  useEffect(() => {
    setBaseline(nodeConfig);
  }, [nodeConfig, setBaseline]);

  // 노드의 모든 규칙(그룹 필터 없음). origin(원본 인덱스)은 rule.index 로 보존.
  const rules = useMemo(
    () => sortRulesByPriority(parseRules(schedules), sortDesc),
    [schedules, sortDesc],
  );

  // 모달 상태: editIndex -1 = 신규, >=0 = 편집(FULL 배열 원본 인덱스).
  const [modal, setModal] = useState<{ editIndex: number; initial: RuleDraft } | null>(null);

  // 모달이 고른(또는 편집 시작 시 프리셋된) 에이전트. TARGET 열거 + agent_id 각인 대상.
  const [pickedAgent, setPickedAgent] = useState('');
  const effectiveAgent = pickedAgent.length > 0 ? pickedAgent : null;

  // 신규/편집 스케줄에 agent_id 를 각인한다(REQ-01-05). 미지정(선택 없음)이면 각인하지 않음.
  const stamp = useCallback(
    (s: SerializedSchedule): SerializedSchedule =>
      effectiveAgent ? { ...s, agent_id: effectiveAgent } : s,
    [effectiveAgent],
  );

  const handleToggle = useCallback(
    (index: number) => {
      void persist(toggleEnabledAt(schedules, index), () => onPersisted());
    },
    [schedules, persist, onPersisted],
  );

  const handleDelete = useCallback(
    (index: number) => {
      void persist(
        schedules.filter((_, i) => i !== index),
        () => onPersisted(),
      );
    },
    [schedules, persist, onPersisted],
  );

  const openEdit = useCallback(
    (rule: FacilityRule) => {
      // 편집 시작 시 이 행의 현재 에이전트를 셀렉터 초기값으로 프리셋(사용자가 변경 가능).
      setPickedAgent(resolveScheduleAgent(rule.raw, nodeConfig) ?? '');
      setModal({ editIndex: rule.index, initial: ruleToDraft(rule) });
    },
    [nodeConfig],
  );
  const closeModal = useCallback(() => setModal(null), []);

  const handleModalSave = useCallback(
    (draft: RuleDraft) => {
      if (!modal) return;
      const base = modal.editIndex >= 0 ? schedules[modal.editIndex] : undefined;
      const built = buildScheduleFromDraft(draft, base);
      if (!built) return; // 유효하지 않으면 모달이 이미 검증으로 막음.
      const next = upsertScheduleAt(schedules, modal.editIndex, stamp(built));
      setModal(null);
      void persist(next, () => onPersisted());
    },
    [modal, schedules, stamp, persist, onPersisted],
  );

  const canSelectAgent = agents.length > 0;

  return (
    <tbody data-testid={`schedule-node-${flowId}-${nodeId}`}>
      {rules.map((rule) => {
        const rowAgent = resolveScheduleAgent(rule.raw, nodeConfig);
        return (
          <tr
            key={rule.index}
            data-testid={`schedule-row-${flowId}-${nodeId}-${rule.index}`}
            className="border-b border-(--color-border-default)/60 align-top"
          >
            {/* 에이전트 */}
            <td
              className="px-2 py-2 text-(--color-text-primary)"
              data-testid={`schedule-agent-${flowId}-${nodeId}-${rule.index}`}
            >
              <span className={cn('text-[11px] font-medium', rowAgent === null && 'text-(--color-text-muted)')}>
                {agentDisplayName(rowAgent, agentName)}
              </span>
            </td>
            {/* 플로우/노드 */}
            <td className="px-2 py-2">
              <div className="flex flex-col gap-0.5">
                <span className="truncate text-[11px] font-medium text-(--color-text-secondary)">{nodeName}</span>
                <span className="truncate text-[10px] text-(--color-text-muted)">{flowName}</span>
              </div>
            </td>
            {/* 스케줄(이름 + 유효기간) */}
            <td className="px-2 py-2">
              <div className="font-medium text-(--color-text-primary)">{rule.name || '(이름 없음)'}</div>
              <div className="text-[11px] text-(--color-text-muted)">
                {formatValidity(rule.validFrom, rule.validTo)}
              </div>
            </td>
            {/* 대상(TARGET) */}
            <td className="px-2 py-2">
              {rule.target ? (
                <div className="flex flex-col gap-0.5">
                  <span className="inline-flex w-fit items-center rounded-full bg-blue-100 px-1.5 py-0.5 text-[10px] font-medium text-blue-700 dark:bg-blue-900/40 dark:text-blue-300">
                    {targetKindLabel(rule.target.kind)}
                  </span>
                  <span className="text-[11px] text-(--color-text-secondary)">
                    {formatTargetDescription(rule.target)}
                  </span>
                </div>
              ) : (
                <span className="text-[11px] text-(--color-text-muted)">-</span>
              )}
            </td>
            {/* 계획(PLAN) */}
            <td className="px-2 py-2 text-[11px] text-(--color-text-secondary)">
              {formatPlanSummary(rule.schedule)}
            </td>
            {/* 동작(ACTION) */}
            <td
              className="px-2 py-2 text-[11px] text-(--color-text-secondary)"
              data-testid={`schedule-action-${flowId}-${nodeId}-${rule.index}`}
            >
              {formatActionLabel(rule.action)}
            </td>
            {/* 우선순위 */}
            <td className="px-2 py-2 text-(--color-text-secondary)">{rule.priority}</td>
            {/* 상태 */}
            <td className="px-2 py-2">
              <button
                type="button"
                data-testid={`schedule-state-${flowId}-${nodeId}-${rule.index}`}
                onClick={() => handleToggle(rule.index)}
                disabled={saving}
                className={cn(
                  'inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium transition-colors disabled:opacity-50',
                  rule.enabled
                    ? 'bg-green-100 text-green-700 hover:bg-green-200 dark:bg-green-900/40 dark:text-green-400'
                    : 'bg-slate-100 text-slate-500 hover:bg-slate-200 dark:bg-slate-800 dark:text-slate-400',
                )}
              >
                {rule.enabled ? '활성' : '비활성'}
              </button>
            </td>
            {/* 편집 / 삭제 */}
            <td className="px-2 py-2 text-right">
              <div className="flex items-center justify-end gap-1">
                <button
                  type="button"
                  data-testid={`schedule-edit-${flowId}-${nodeId}-${rule.index}`}
                  onClick={() => openEdit(rule)}
                  aria-label="편집"
                  className="rounded p-1 text-gray-400 transition-colors hover:text-blue-600"
                >
                  <Pencil className="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  data-testid={`schedule-delete-${flowId}-${nodeId}-${rule.index}`}
                  onClick={() => handleDelete(rule.index)}
                  aria-label="삭제"
                  disabled={saving}
                  className="rounded p-1 text-gray-400 transition-colors hover:text-red-500 disabled:opacity-50"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            </td>
          </tr>
        );
      })}

      {/* 편집 모달(재사용). fixed 오버레이지만 유효한 HTML 을 위해 colSpan 셀 안에 렌더한다.
          에이전트 셀렉터로 TARGET 열거/각인, agent_id 는 저장 시 stamp 로 각인. */}
      {modal && (
        <tr>
          <td colSpan={SCHEDULE_TABLE_COLUMNS} className="p-0">
            <FacilityRuleModal
              initial={modal.initial}
              agentId={pickedAgent}
              agents={canSelectAgent ? agents : undefined}
              selectedAgentId={canSelectAgent ? pickedAgent : undefined}
              onAgentChange={canSelectAgent ? setPickedAgent : undefined}
              onSave={handleModalSave}
              onCancel={closeModal}
            />
          </td>
        </tr>
      )}
    </tbody>
  );
}
