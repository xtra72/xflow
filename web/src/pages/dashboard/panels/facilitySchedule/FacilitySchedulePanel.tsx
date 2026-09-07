// SPEC-TRIGGER-SCHED-001 M2~M6: 설비 제어 예약 패널.
//
// 특정 trigger 노드({flowId,nodeId})의 스케줄을 "예약 규칙" 표(SCHEDULE/TARGET/PLAN/
// ACTION/PRIO/STATE)로 렌더한다(읽기 전용, M2). 생성/편집은 모달(M3)에서 하고, 행별
// EDIT 버튼 + STATE 토글을 제공한다. TARGET(M4)/ACTION(M5) 는 모달의 피커/편집기로
// 조립되어 각 스케줄 payload(= xsfm 제어 명령)에 인라인 저장된다.
//
// 저장/토글은 선행 SPEC-TRIGGER-PANEL-001 의 dual-write 를 그대로 재사용한다(M6):
// configureNode(LIVE) + getFlow→patch→updateFlow(PERSIST), 404→persist-only+통지,
// last-write-wins(detectConflict). 신규 저장소/ API 도입 없음(RD-9).
//
// 범용 `trigger-config` 패널과 공존하는 별도 패널 타입(`facility-schedule`, RD-5).

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { CalendarClock, CirclePlay, CircleStop, Pencil, Plus } from 'lucide-react';

import { useFlowNodes } from '@/hooks/useFlow';
import { useNodeTypeInstances } from '@/hooks/useNodeTypeInstances';
import { useAgents } from '@/hooks/useAgent';
import { useGroups } from '@/hooks/useGroups';
import { useXsfmDevices } from '@/hooks/useStation';
import { cn } from '@/lib/utils/cn';
import { useScheduleDualWrite } from './useScheduleDualWrite';
import FacilityRuleModal from './FacilityRuleModal';
import { usePanelTitleStyle, usePanelTitleVisible } from '../../panelChromeContext';
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
} from './facilityScheduleUtils';

interface FacilitySchedulePanelProps {
  panelId: string;
  title: string;
  config: Record<string, unknown>;
  onConfigChange?: (config: Record<string, unknown>) => void;
  onTitleChange?: (title: string) => void;
}

const cardCls =
  'flex min-h-0 flex-1 flex-col gap-3 overflow-auto rounded-2xl bg-(--color-bg-surface) p-4 ring-1 ring-(--color-border-default)';

/** 설비 제어 예약 패널. */
export default function FacilitySchedulePanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange,
  onTitleChange: _onTitleChange,
}: FacilitySchedulePanelProps) {
  const showTitle = usePanelTitleVisible();
  const titleStyle = usePanelTitleStyle();
  const flowId = typeof config.flowId === 'string' ? config.flowId : '';
  const nodeId = typeof config.nodeId === 'string' ? config.nodeId : '';
  const agentId = typeof config.agentId === 'string' ? config.agentId : '';

  const { data: nodes, isLoading } = useFlowNodes(flowId);
  const { instances } = useNodeTypeInstances('trigger');

  // TARGET 이름 매핑용 열거(에이전트 있을 때만). 그룹/기기 id→name.
  const { data: agentsResult } = useAgents();
  const xsfmAgents = useMemo(
    () => (agentsResult?.data ?? []).filter((a) => a.type === 'xsfm'),
    [agentsResult],
  );
  const { data: groups } = useGroups(agentId);
  const { data: devices } = useXsfmDevices(agentId);
  const groupNameById = useMemo(() => {
    const m = new Map<string, string>();
    for (const g of groups ?? []) m.set(g.id, g.name);
    return m;
  }, [groups]);
  const deviceNameById = useMemo(() => {
    const m = new Map<string, string>();
    for (const d of devices ?? []) m.set(d.device_id, d.name || d.device_id);
    return m;
  }, [devices]);

  const targetNode = useMemo(
    () => (nodes ?? []).find((n) => n.node_id === nodeId),
    [nodes, nodeId],
  );
  const isRunning = useMemo(
    () => instances.some((i) => i.flowId === flowId && i.nodeId === nodeId),
    [instances, flowId, nodeId],
  );

  // dual-write 훅(M4 추출): saving/baseline 소유 + LIVE/PERSIST + 통지 재사용.
  const { persist, saving, setBaseline } = useScheduleDualWrite({ flowId, nodeId });

  // 로컬 스케줄 draft(SSOT=노드 config, 최초 1회 하이드레이트). dual-write 후 baseline 갱신.
  const [schedules, setSchedules] = useState<SerializedSchedule[]>([]);
  const hydratedRef = useRef(false);

  useEffect(() => {
    hydratedRef.current = false;
  }, [nodeId, flowId]);

  useEffect(() => {
    if (!targetNode || hydratedRef.current) return;
    const cfg = (targetNode.config ?? {}) as Record<string, unknown>;
    setSchedules(Array.isArray(cfg.schedules) ? (cfg.schedules as SerializedSchedule[]) : []);
    setBaseline(cfg);
    hydratedRef.current = true;
  }, [targetNode, setBaseline]);

  // 정렬 방향(기본 priority 오름차순, PRIO 헤더 클릭 토글).
  const [sortDesc, setSortDesc] = useState(false);
  const rules = useMemo(() => sortRulesByPriority(parseRules(schedules), sortDesc), [schedules, sortDesc]);

  // 모달 상태: editIndex -1 = 신규, >=0 = 편집(원본 인덱스).
  const [modal, setModal] = useState<{ open: boolean; editIndex: number; initial: RuleDraft } | null>(null);

  // ---- STATE 토글 ----
  // dual-write 성공 시에만 로컬 스케줄 반영(onApplied). 조기 반환 오류 경로는 미반영.
  const handleToggle = useCallback(
    (index: number) => {
      void persist(toggleEnabledAt(schedules, index), setSchedules);
    },
    [schedules, persist],
  );

  // ---- 모달 열기/저장 ----
  const openNew = useCallback(() => setModal({ open: true, editIndex: -1, initial: emptyDraft() }), []);
  const openEdit = useCallback(
    (rule: FacilityRule) => setModal({ open: true, editIndex: rule.index, initial: ruleToDraft(rule) }),
    [],
  );
  const closeModal = useCallback(() => setModal(null), []);

  const handleModalSave = useCallback(
    (draft: RuleDraft) => {
      if (!modal) return;
      const base = modal.editIndex >= 0 ? schedules[modal.editIndex] : undefined;
      const next = buildScheduleFromDraft(draft, base);
      if (!next) return; // 유효하지 않으면 모달이 이미 검증으로 막음.
      const nextSchedules = upsertScheduleAt(schedules, modal.editIndex, next);
      setModal(null);
      void persist(nextSchedules, setSchedules);
    },
    [modal, schedules, persist],
  );

  // TARGET desc 이름 해석.
  const resolveTargetName = useCallback(
    (rule: FacilityRule): string | undefined => {
      const t = rule.target;
      if (!t) return undefined;
      if (t.kind === 'group' && !t.byName) return groupNameById.get(t.value);
      if (t.kind === 'device' && !t.byName) return deviceNameById.get(t.value);
      return undefined;
    },
    [groupNameById, deviceNameById],
  );

  // ---- 렌더 ----
  if (!flowId || !nodeId) {
    return (
      <div className={cn(cardCls, 'items-center justify-center')} data-testid="facility-schedule-untargeted">
        <CalendarClock className="mb-1 h-6 w-6 text-(--color-text-muted)" />
        <p className="text-xs text-(--color-text-muted)">대상 트리거 노드가 지정되지 않았습니다.</p>
      </div>
    );
  }

  return (
    <div className={cardCls} data-testid="facility-schedule-panel">
      {/* 헤더 */}
      <div className="flex shrink-0 items-center justify-between gap-2">
        {/* 우측에 규칙 추가 버튼이 함께 있으므로 타이틀 묶음만 숨긴다. */}
        {showTitle && (
          <div className="flex items-center gap-2">
            <CalendarClock className="h-5 w-5 text-blue-500" />
            <span className="truncate text-base font-bold text-(--color-text-primary)" style={titleStyle}>{title}</span>
          </div>
        )}
        <div className="flex items-center gap-2">
          {isRunning ? (
            <span
              data-testid="facility-badge-running"
              className="inline-flex items-center gap-1 rounded-full bg-green-50 px-2 py-1 text-xs font-medium text-green-600 dark:bg-green-900/30 dark:text-green-400"
            >
              <CirclePlay className="h-3.5 w-3.5" /> 실행 중
            </span>
          ) : (
            <span
              data-testid="facility-badge-stopped"
              className="inline-flex items-center gap-1 rounded-full bg-(--color-bg-sunken) px-2 py-1 text-xs font-medium text-(--color-text-muted)"
            >
              <CircleStop className="h-3.5 w-3.5" /> 중지됨
            </span>
          )}
          <button
            type="button"
            onClick={openNew}
            data-testid="facility-add-rule"
            className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-2.5 py-1 text-xs font-medium text-white transition-colors hover:bg-blue-700"
          >
            <Plus className="h-3.5 w-3.5" /> 규칙
          </button>
        </div>
      </div>

      {/* 에이전트 선택(TARGET 열거/이름 해석용, 선택 사항) */}
      <div className="flex shrink-0 items-center gap-2">
        <span className="text-[11px] text-(--color-text-muted)">설비 에이전트</span>
        <select
          data-testid="facility-agent-select"
          value={agentId}
          onChange={(e) => onConfigChange?.({ agentId: e.target.value })}
          className="flex-1 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1 text-xs text-(--color-text-primary) focus:border-blue-400 focus:outline-none"
        >
          <option value="">(미지정 — 자유 입력)</option>
          {xsfmAgents.map((a) => (
            <option key={a.id} value={a.id}>
              {a.name}
            </option>
          ))}
        </select>
      </div>

      {!isRunning && (
        <p
          data-testid="facility-persist-only-notice"
          className="shrink-0 rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-400"
        >
          노드 미실행 — 저장 시 지속화만 적용됩니다 (재배포 시 반영).
        </p>
      )}

      {isLoading ? (
        <div className="flex flex-1 items-center justify-center">
          <div className="h-5 w-5 animate-spin rounded-full border-2 border-(--color-border-default) border-t-blue-600" />
        </div>
      ) : !targetNode ? (
        <p className="text-xs text-(--color-text-muted)" data-testid="facility-node-not-found">
          대상 노드를 찾을 수 없습니다.
        </p>
      ) : rules.length === 0 ? (
        <div
          className="flex flex-1 flex-col items-center justify-center gap-2 text-center"
          data-testid="facility-empty"
        >
          <CalendarClock className="h-6 w-6 text-(--color-text-muted)" />
          <p className="text-xs text-(--color-text-muted)">등록된 예약 규칙이 없습니다.</p>
          <button
            type="button"
            onClick={openNew}
            data-testid="facility-empty-add"
            className="inline-flex items-center gap-1 rounded-md border border-dashed border-(--color-border-default) px-3 py-1.5 text-xs text-(--color-text-muted) transition-colors hover:border-blue-400 hover:text-blue-600"
          >
            <Plus className="h-3.5 w-3.5" /> 규칙 추가
          </button>
        </div>
      ) : (
        <div className="min-h-0 flex-1 overflow-auto">
          <table className="w-full border-collapse text-left text-xs" data-testid="facility-rule-table">
            <thead>
              <tr className="border-b border-(--color-border-default) text-[11px] text-(--color-text-muted)">
                <th className="px-2 py-1.5 font-medium">SCHEDULE</th>
                <th className="px-2 py-1.5 font-medium">TARGET</th>
                <th className="px-2 py-1.5 font-medium">PLAN</th>
                <th className="px-2 py-1.5 font-medium">ACTION</th>
                <th
                  className="cursor-pointer select-none px-2 py-1.5 font-medium hover:text-blue-600"
                  onClick={() => setSortDesc((v) => !v)}
                  data-testid="facility-sort-header"
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
                  data-testid={`facility-rule-row-${rule.index}`}
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
                        <span
                          data-testid={`facility-target-badge-${rule.index}`}
                          className="inline-flex w-fit items-center rounded-full bg-blue-100 px-1.5 py-0.5 text-[10px] font-medium text-blue-700 dark:bg-blue-900/40 dark:text-blue-300"
                        >
                          {targetKindLabel(rule.target.kind)}
                        </span>
                        <span className="text-[11px] text-(--color-text-secondary)">
                          {formatTargetDescription(rule.target, resolveTargetName(rule))}
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
                  <td
                    className="px-2 py-2 text-[11px] text-(--color-text-secondary)"
                    data-testid={`facility-action-${rule.index}`}
                  >
                    {formatActionLabel(rule.action)}
                  </td>
                  {/* PRIO */}
                  <td className="px-2 py-2 text-(--color-text-secondary)" data-testid={`facility-prio-${rule.index}`}>
                    {rule.priority}
                  </td>
                  {/* STATE (토글) */}
                  <td className="px-2 py-2">
                    <button
                      type="button"
                      data-testid={`facility-state-${rule.index}`}
                      onClick={() => handleToggle(rule.index)}
                      disabled={saving}
                      className={cn(
                        'inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-medium transition-colors disabled:opacity-50',
                        rule.enabled
                          ? 'bg-green-100 text-green-700 hover:bg-green-200 dark:bg-green-900/40 dark:text-green-400'
                          : 'bg-(--color-bg-sunken) text-(--color-text-muted) hover:bg-(--color-bg-sunken)',
                      )}
                    >
                      {rule.enabled ? '활성' : '비활성'}
                    </button>
                  </td>
                  {/* EDIT */}
                  <td className="px-2 py-2 text-right">
                    <button
                      type="button"
                      data-testid={`facility-edit-${rule.index}`}
                      onClick={() => openEdit(rule)}
                      aria-label="편집"
                      className="rounded p-1 text-(--color-text-muted) transition-colors hover:text-blue-600"
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* 모달 */}
      {modal?.open && (
        <FacilityRuleModal
          initial={modal.initial}
          agentId={agentId}
          onSave={handleModalSave}
          onCancel={closeModal}
        />
      )}
    </div>
  );
}
