// SPEC-SCHEDULE-VIEW-001 M5: 노드 단위 스케줄 섹션(관리 탭 구성단위).
//
// 한 (flowId,nodeId) trigger 노드를 소유하며 useScheduleDualWrite 인스턴스 하나를 가진다.
// Rules of Hooks 준수: 그룹/노드 반복은 상위 탭이 컴포넌트로 렌더하므로 훅이 루프/조건
// 안에서 호출되지 않는다. 이 섹션은 FacilitySchedulePanel(단일 노드)의 교차-플로우
// 일반화이며 표/모달/dual-write 배선을 그대로 재사용한다(로직 중복 없음).
//
// 스케줄 SSOT 는 상위 집계(react-query 캐시)이다. 이 섹션은 로컬 사본을 두지 않고
// entry.schedules(props)에서 규칙을 파생하며, dual-write 성공 시 onPersisted 로 상위
// 재조회를 트리거한다(발산 없음). agentKey 로 이 그룹에 속하는 규칙만 표시하되(혼합
// agent 노드 지원) CRUD 는 FULL schedules 배열의 원본 인덱스로 매핑해 origin 을 보존한다.

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Pencil, Plus, Trash2 } from 'lucide-react';

import FacilityRuleModal from '@/pages/dashboard/panels/facilitySchedule/FacilityRuleModal';
import { useScheduleDualWrite } from '@/pages/dashboard/panels/facilitySchedule/useScheduleDualWrite';
import {
  buildScheduleFromDraft,
  emptyDraft,
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

import { resolveScheduleAgent, type ScheduleNodeEntry } from './scheduleAggregation';

interface NodeScheduleSectionProps {
  /** 소유 노드 엔트리(FULL schedules + baseline config + origin). */
  entry: ScheduleNodeEntry;
  /** 이 섹션이 표시할 그룹 키(해석 agent). null = 미지정. */
  agentKey: string | null;
  /** 모달 TARGET 열거용 agent id(그룹 키가 문자열이면 그 값, 아니면 빈 문자열=자유 입력). */
  modalAgentId: string;
  /** dual-write 성공 시 상위 집계 재조회 트리거. */
  onPersisted: () => void;
}

/** 노드 단위 예약 규칙 섹션(표 + 모달 + dual-write). */
export default function NodeScheduleSection({
  entry,
  agentKey,
  modalAgentId,
  onPersisted,
}: NodeScheduleSectionProps) {
  const { flowId, nodeId, nodeName, flowName, nodeConfig, schedules } = entry;

  // dual-write 훅(M4). (flowId,nodeId) 별 인스턴스 하나 — Rules of Hooks 준수.
  const { persist, saving, setBaseline } = useScheduleDualWrite({ flowId, nodeId });

  // baseline = 노드 정의 config(FULL config 조립/충돌 감지 기준). 정의 갱신 시 재시드.
  useEffect(() => {
    setBaseline(nodeConfig);
  }, [nodeConfig, setBaseline]);

  const [sortDesc, setSortDesc] = useState(false);

  // 이 그룹(agentKey)에 속하는 규칙만 표시하되 origin(원본 인덱스)은 rule.index 로 보존.
  const rules = useMemo(() => {
    const mine = parseRules(schedules).filter(
      (r) => resolveScheduleAgent(r.raw, nodeConfig) === agentKey,
    );
    return sortRulesByPriority(mine, sortDesc);
  }, [schedules, nodeConfig, agentKey, sortDesc]);

  // 모달 상태: editIndex -1 = 신규, >=0 = 편집(FULL 배열 원본 인덱스).
  const [modal, setModal] = useState<{ editIndex: number; initial: RuleDraft } | null>(null);

  // 신규/편집 스케줄에 agent_id 를 각인해 그룹핑을 안정화한다(REQ-01-05). 미지정이면 각인하지 않음.
  const stamp = useCallback(
    (s: SerializedSchedule): SerializedSchedule => (agentKey ? { ...s, agent_id: agentKey } : s),
    [agentKey],
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

  const openNew = useCallback(() => setModal({ editIndex: -1, initial: emptyDraft() }), []);
  const openEdit = useCallback(
    (rule: FacilityRule) => setModal({ editIndex: rule.index, initial: ruleToDraft(rule) }),
    [],
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

  return (
    <div
      className="rounded-xl border border-(--color-border-default) bg-(--color-bg-surface) p-3"
      data-testid={`schedule-node-${flowId}-${nodeId}`}
    >
      {/* 섹션 헤더: 노드/플로우 명 + 규칙 추가 */}
      <div className="mb-2 flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate text-sm font-semibold text-(--color-text-primary)">{nodeName}</span>
          <span className="truncate text-[11px] text-(--color-text-muted)">{flowName}</span>
        </div>
        <button
          type="button"
          onClick={openNew}
          data-testid={`schedule-add-${flowId}-${nodeId}`}
          disabled={saving}
          className="inline-flex shrink-0 items-center gap-1 rounded-md bg-blue-600 px-2.5 py-1 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50"
        >
          <Plus className="h-3.5 w-3.5" /> 규칙
        </button>
      </div>

      {rules.length === 0 ? (
        <p className="px-1 py-3 text-center text-[11px] text-(--color-text-muted)" data-testid={`schedule-empty-${flowId}-${nodeId}`}>
          등록된 예약 규칙이 없습니다.
        </p>
      ) : (
        <div className="overflow-auto">
          <table className="w-full border-collapse text-left text-xs" data-testid={`schedule-table-${flowId}-${nodeId}`}>
            <thead>
              <tr className="border-b border-(--color-border-default) text-[11px] text-(--color-text-muted)">
                <th className="px-2 py-1.5 font-medium">SCHEDULE</th>
                <th className="px-2 py-1.5 font-medium">TARGET</th>
                <th className="px-2 py-1.5 font-medium">PLAN</th>
                <th className="px-2 py-1.5 font-medium">ACTION</th>
                <th
                  className="cursor-pointer select-none px-2 py-1.5 font-medium hover:text-blue-600"
                  onClick={() => setSortDesc((v) => !v)}
                  title="우선순위 정렬"
                >
                  PRIO {sortDesc ? '▼' : '▲'}
                </th>
                <th className="px-2 py-1.5 font-medium">STATE</th>
                <th className="px-2 py-1.5" />
              </tr>
            </thead>
            <tbody>
              {rules.map((rule) => (
                <tr
                  key={rule.index}
                  data-testid={`schedule-row-${flowId}-${nodeId}-${rule.index}`}
                  className="border-b border-(--color-border-default)/60 align-top"
                >
                  {/* SCHEDULE */}
                  <td className="px-2 py-2">
                    <div className="font-medium text-(--color-text-primary)">{rule.name || '(이름 없음)'}</div>
                    <div className="text-[11px] text-(--color-text-muted)">
                      {formatValidity(rule.validFrom, rule.validTo)}
                    </div>
                  </td>
                  {/* TARGET */}
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
                  {/* PLAN */}
                  <td className="px-2 py-2 text-[11px] text-(--color-text-secondary)">
                    {formatPlanSummary(rule.schedule)}
                  </td>
                  {/* ACTION */}
                  <td className="px-2 py-2 text-[11px] text-(--color-text-secondary)" data-testid={`schedule-action-${flowId}-${nodeId}-${rule.index}`}>
                    {formatActionLabel(rule.action)}
                  </td>
                  {/* PRIO */}
                  <td className="px-2 py-2 text-(--color-text-secondary)">{rule.priority}</td>
                  {/* STATE */}
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
                  {/* EDIT / DELETE */}
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
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* 생성/편집 모달(재사용). 미지정 그룹은 modalAgentId='' 로 자유 입력 폴백. */}
      {modal && (
        <FacilityRuleModal
          initial={modal.initial}
          agentId={modalAgentId}
          onSave={handleModalSave}
          onCancel={closeModal}
        />
      )}
    </div>
  );
}
