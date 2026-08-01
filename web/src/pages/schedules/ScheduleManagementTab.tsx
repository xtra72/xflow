// SPEC-SCHEDULE-VIEW-001: 스케줄 관리 탭(단일 플랫 · 전역 정렬 · 필터 표).
//
// useScheduleAggregation 으로 교차-플로우 스케줄을 집계하고, 모든 노드/스케줄을 하나의
// 플랫 행 배열로 펼쳐(FlatScheduleRow) 전역 정렬/필터를 적용한 뒤 단일 <tbody> 로 렌더한다.
// 컬럼 순서: 스케줄 이름 | 상태 | 대상 | 계획 | 동작 | 기간 | 우선순위 | 에이전트 | 플로우/노드 | 제어.
// 9개 정렬 컬럼은 공용 SortableHeader 를 사용하고, 제어는 비정렬 헤더다. 필터 바(상태/에이전트/
// 검색)는 정렬 이전에 AND 로 적용한다.
//
// 단일 <tbody> 에서는 행마다 훅을 호출할 수 없으므로(Rules of Hooks) CRUD 는 훅이 아닌
// persistScheduleDualWrite one-shot async 로 각인/지속화한다. 편집 모달은 상위가 소유해
// 표 밖 오버레이로 렌더한다. "규칙 추가"는 작성 카드 없이 설정 팝업을 직접 연다.
// 부분 실패(AC-18)는 성공 행을 유지한 채 통지 배너를 노출한다.

import { useCallback, useMemo, useState } from 'react';
import { AlertTriangle, CalendarClock, Pencil, Plus, Trash2 } from 'lucide-react';

import SortableHeader, { type SortState } from '@/components/common/SortableHeader';
import { useAgents } from '@/hooks/useAgent';
import FacilityRuleModal, {
  type FacilityRuleModalNode,
} from '@/pages/dashboard/panels/facilitySchedule/FacilityRuleModal';
import { persistScheduleDualWrite } from '@/pages/dashboard/panels/facilitySchedule/scheduleDualWrite';
import {
  buildScheduleFromDraft,
  emptyDraft,
  formatActionLabel,
  formatPlanSummary,
  formatTargetDescription,
  formatValidity,
  parseRules,
  ruleToDraft,
  targetKindLabel,
  toggleEnabledAt,
  upsertScheduleAt,
  type FacilityRule,
  type RuleDraft,
  type SerializedSchedule,
} from '@/pages/dashboard/panels/facilitySchedule/facilityScheduleUtils';
import { cn } from '@/lib/utils/cn';
import { useUIStore } from '@/stores/uiStore';

import {
  resolveScheduleAgent,
  UNASSIGNED_LABEL,
  useScheduleAggregation,
} from './scheduleAggregation';

/** 에이전트 미해석 행의 에이전트 필터 센티널 값. */
const UNASSIGNED_AGENT_VALUE = '__unassigned__';

/** 하나의 규칙을 소유 노드 컨텍스트와 함께 담는 플랫 행(전역 정렬/필터 단위). */
interface FlatScheduleRow {
  flowId: string;
  nodeId: string;
  nodeName: string;
  flowName: string;
  /** dual-write baseline(노드 config) + node-level agentId 소스. */
  nodeConfig: Record<string, unknown>;
  /** 소유 노드의 FULL schedules 배열(원본 인덱스 = CRUD origin). */
  schedules: SerializedSchedule[];
  /** 파싱된 규칙(rule.index = FULL 배열 원본 인덱스). */
  rule: FacilityRule;
  /** 해석된 에이전트 id(null = 미지정). */
  agentId: string | null;
  /** 에이전트 표시 라벨(이름 → id → "미지정"). */
  agentLabel: string;
}

/** agent id → 표시 이름(없으면 id, null 이면 미지정). */
function agentDisplayName(agentId: string | null, agentName: Map<string, string>): string {
  if (agentId === null) return UNASSIGNED_LABEL;
  return agentName.get(agentId) ?? agentId;
}

/** 행의 TARGET 설명 문자열(없으면 빈 문자열) — 검색/정렬용. */
function rowTargetDescription(row: FlatScheduleRow): string {
  return row.rule.target ? formatTargetDescription(row.rule.target) : '';
}

/**
 * 전역 정렬 비교자(방향 미적용). 동률은 호출부의 원본 인덱스 tiebreaker 로 안정성을 보장한다.
 * status 는 asc 기준 활성 우선(active-first).
 */
function compareRows(a: FlatScheduleRow, b: FlatScheduleRow, field: string): number {
  switch (field) {
    case 'name':
      return a.rule.name.localeCompare(b.rule.name);
    case 'status':
      return a.rule.enabled === b.rule.enabled ? 0 : a.rule.enabled ? -1 : 1;
    case 'target':
      return rowTargetDescription(a).localeCompare(rowTargetDescription(b));
    case 'plan':
      return formatPlanSummary(a.rule.schedule).localeCompare(formatPlanSummary(b.rule.schedule));
    case 'action':
      return formatActionLabel(a.rule.action).localeCompare(formatActionLabel(b.rule.action));
    case 'period':
      return a.rule.validFrom.localeCompare(b.rule.validFrom);
    case 'priority':
      return a.rule.priority - b.rule.priority;
    case 'agent':
      return a.agentLabel.localeCompare(b.agentLabel);
    case 'flownode':
      return `${a.nodeName}/${a.flowName}`.localeCompare(`${b.nodeName}/${b.flowName}`);
    default:
      return 0;
  }
}

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

  // 규칙 추가/편집 시 TARGET 열거/각인에 쓰는 xsfm 에이전트 목록.
  const xsfmAgents = useMemo(
    () => (agentsResult?.data ?? []).filter((a) => a.type === 'xsfm').map((a) => ({ id: a.id, name: a.name })),
    [agentsResult],
  );

  // 표시 엔트리: 스케줄이 0개인 노드(=최초 규칙 진입점용 빈 노드)는 표에서 숨긴다.
  const displayEntries = useMemo(
    () => nodeEntries.filter((n) => n.schedules.length > 0),
    [nodeEntries],
  );

  // 모든 노드/스케줄을 하나의 플랫 행 배열로 펼친다(전역 정렬/필터 단위).
  const flatRows = useMemo<FlatScheduleRow[]>(() => {
    const out: FlatScheduleRow[] = [];
    for (const entry of displayEntries) {
      for (const rule of parseRules(entry.schedules)) {
        const agentId = resolveScheduleAgent(rule.raw, entry.nodeConfig);
        out.push({
          flowId: entry.flowId,
          nodeId: entry.nodeId,
          nodeName: entry.nodeName,
          flowName: entry.flowName,
          nodeConfig: entry.nodeConfig,
          schedules: entry.schedules,
          rule,
          agentId,
          agentLabel: agentDisplayName(agentId, agentName),
        });
      }
    }
    return out;
  }, [displayEntries, agentName]);

  // --- 필터 바 상태(상태/에이전트/검색) ---
  const [statusFilter, setStatusFilter] = useState('');
  const [agentFilter, setAgentFilter] = useState('');
  const [search, setSearch] = useState('');

  // 에이전트 필터 옵션(플랫 행에 등장하는 distinct 에이전트). 미지정은 센티널로 묶는다.
  const agentOptions = useMemo(() => {
    const seen = new Map<string, string>();
    for (const row of flatRows) {
      const value = row.agentId ?? UNASSIGNED_AGENT_VALUE;
      if (!seen.has(value)) seen.set(value, row.agentLabel);
    }
    return Array.from(seen, ([value, label]) => ({ value, label }));
  }, [flatRows]);

  // 필터 적용(AND). 빈/전체 = 무제약.
  const filteredRows = useMemo(() => {
    let result = flatRows;
    if (statusFilter) {
      const want = statusFilter === 'enabled';
      result = result.filter((r) => r.rule.enabled === want);
    }
    if (agentFilter) {
      result = result.filter((r) => (r.agentId ?? UNASSIGNED_AGENT_VALUE) === agentFilter);
    }
    const q = search.trim().toLowerCase();
    if (q) {
      result = result.filter((r) =>
        `${r.rule.name} ${rowTargetDescription(r)} ${r.flowName} ${r.nodeName}`
          .toLowerCase()
          .includes(q),
      );
    }
    return result;
  }, [flatRows, statusFilter, agentFilter, search]);

  // --- 전역 정렬 상태(기본 우선순위 오름차순 — 기존 기본 유지) ---
  const [sort, setSort] = useState<SortState>({ field: 'priority', direction: 'asc' });

  const sortedRows = useMemo(() => {
    const indexed = filteredRows.map((r, i) => ({ r, i }));
    const { field, direction } = sort;
    const mul = direction === 'asc' ? 1 : -1;
    indexed.sort((a, b) => {
      const diff = compareRows(a.r, b.r, field);
      return diff !== 0 ? diff * mul : a.i - b.i; // 동률은 원본 순서 유지(안정 정렬).
    });
    return indexed.map((x) => x.r);
  }, [filteredRows, sort]);

  /** 정렬 필드 변경. 같은 필드 클릭 시 방향 토글, 다른 필드는 asc. */
  const handleSort = useCallback((field: string) => {
    setSort((prev) =>
      prev.field === field
        ? { field, direction: prev.direction === 'asc' ? 'desc' : 'asc' }
        : { field, direction: 'asc' },
    );
  }, []);

  // --- CRUD(one-shot dual-write) ---
  // 진행 중인 행 키(해당 행 버튼 비활성). `${flowId}:${nodeId}:${index}`.
  const [savingKey, setSavingKey] = useState<string | null>(null);
  const rowKey = (row: FlatScheduleRow) => `${row.flowId}:${row.nodeId}:${row.rule.index}`;

  const persistRow = useCallback(
    async (row: FlatScheduleRow, nextSchedules: SerializedSchedule[]) => {
      setSavingKey(rowKey(row));
      try {
        const ok = await persistScheduleDualWrite({
          flowId: row.flowId,
          nodeId: row.nodeId,
          baseline: row.nodeConfig,
          nextSchedules,
          notify: addNotification,
        });
        if (ok) refetch();
      } finally {
        setSavingKey(null);
      }
    },
    [addNotification, refetch],
  );

  const handleToggle = useCallback(
    (row: FlatScheduleRow) => {
      void persistRow(row, toggleEnabledAt(row.schedules, row.rule.index));
    },
    [persistRow],
  );

  const handleDelete = useCallback(
    (row: FlatScheduleRow) => {
      void persistRow(
        row,
        row.schedules.filter((_, i) => i !== row.rule.index),
      );
    },
    [persistRow],
  );

  // --- 편집 모달(상위 소유, 표 밖 오버레이) ---
  const [editModal, setEditModal] = useState<{
    row: FlatScheduleRow;
    editIndex: number;
    initial: RuleDraft;
  } | null>(null);
  // 편집 모달이 고른(또는 편집 시작 시 프리셋된) 에이전트. TARGET 열거 + agent_id 각인 대상.
  const [pickedAgent, setPickedAgent] = useState('');

  const openEdit = useCallback((row: FlatScheduleRow) => {
    setPickedAgent(resolveScheduleAgent(row.rule.raw, row.nodeConfig) ?? '');
    setEditModal({ row, editIndex: row.rule.index, initial: ruleToDraft(row.rule) });
  }, []);
  const closeEdit = useCallback(() => setEditModal(null), []);

  const handleEditSave = useCallback(
    (draft: RuleDraft) => {
      if (!editModal) return;
      const { row, editIndex } = editModal;
      const base = row.schedules[editIndex];
      const built = buildScheduleFromDraft(draft, base);
      if (!built) return; // 유효하지 않으면 모달이 이미 검증으로 막음.
      const stamped: SerializedSchedule = pickedAgent ? { ...built, agent_id: pickedAgent } : built;
      const next = upsertScheduleAt(row.schedules, editIndex, stamped);
      setEditModal(null);
      void persistRow(row, next);
    },
    [editModal, pickedAgent, persistRow],
  );
  const canSelectAgent = xsfmAgents.length > 0;

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

      {/* 편집 팝업(상위 소유, 표 밖 오버레이). 에이전트 셀렉터로 TARGET 열거/각인. */}
      {editModal && (
        <FacilityRuleModal
          initial={editModal.initial}
          agentId={pickedAgent}
          agents={canSelectAgent ? xsfmAgents : undefined}
          selectedAgentId={canSelectAgent ? pickedAgent : undefined}
          onAgentChange={canSelectAgent ? setPickedAgent : undefined}
          onSave={handleEditSave}
          onCancel={closeEdit}
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
        <>
          {/* 필터 바(상태/에이전트/검색) — ScheduleLogTab 필터 바와 동형. */}
          <div className="flex flex-wrap items-center gap-2">
            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value)}
              aria-label="상태 필터"
              data-testid="schedule-filter-status"
              className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1.5 text-xs text-(--color-text-primary)"
            >
              <option value="">전체 상태</option>
              <option value="enabled">활성</option>
              <option value="disabled">비활성</option>
            </select>
            <select
              value={agentFilter}
              onChange={(e) => setAgentFilter(e.target.value)}
              aria-label="에이전트 필터"
              data-testid="schedule-filter-agent"
              className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1.5 text-xs text-(--color-text-primary)"
            >
              <option value="">전체 에이전트</option>
              {agentOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="검색"
              aria-label="스케줄 검색"
              data-testid="schedule-filter-search"
              className="min-w-40 flex-1 rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1.5 text-xs text-(--color-text-primary)"
            />
          </div>

          <div className="overflow-auto rounded-xl border border-(--color-border-default) bg-(--color-bg-surface)">
            <table className="w-full border-collapse text-left text-xs" data-testid="schedule-table">
              <thead>
                <tr className="border-b border-(--color-border-default)">
                  <SortableHeader label="스케줄 이름" field="name" currentSort={sort} onSort={handleSort} className="px-2 py-1.5" />
                  <SortableHeader label="상태" field="status" currentSort={sort} onSort={handleSort} className="px-2 py-1.5" />
                  <SortableHeader label="대상" field="target" currentSort={sort} onSort={handleSort} className="px-2 py-1.5" />
                  <SortableHeader label="계획" field="plan" currentSort={sort} onSort={handleSort} className="px-2 py-1.5" />
                  <SortableHeader label="동작" field="action" currentSort={sort} onSort={handleSort} className="px-2 py-1.5" />
                  <SortableHeader label="기간" field="period" currentSort={sort} onSort={handleSort} className="px-2 py-1.5" />
                  <SortableHeader label="우선순위" field="priority" currentSort={sort} onSort={handleSort} className="px-2 py-1.5" />
                  <SortableHeader label="에이전트" field="agent" currentSort={sort} onSort={handleSort} className="px-2 py-1.5" />
                  <SortableHeader label="플로우/노드" field="flownode" currentSort={sort} onSort={handleSort} className="px-2 py-1.5" />
                  <th className="px-2 py-1.5 text-right text-[11px] font-medium text-(--color-text-muted)">제어</th>
                </tr>
              </thead>
              <tbody>
                {sortedRows.length === 0 ? (
                  <tr>
                    <td
                      colSpan={10}
                      className="px-2 py-8 text-center text-[11px] text-(--color-text-muted)"
                      data-testid="schedule-no-match"
                    >
                      일치하는 스케줄이 없습니다.
                    </td>
                  </tr>
                ) : (
                  sortedRows.map((row) => {
                    const { flowId, nodeId, rule } = row;
                    const saving = savingKey !== null;
                    return (
                      <tr
                        key={rowKey(row)}
                        data-testid={`schedule-row-${flowId}-${nodeId}-${rule.index}`}
                        className="border-b border-(--color-border-default)/60 align-top"
                      >
                        {/* 스케줄 이름 */}
                        <td className="px-2 py-2">
                          <div className="font-medium text-(--color-text-primary)">
                            {rule.name || '(이름 없음)'}
                          </div>
                        </td>
                        {/* 상태 */}
                        <td className="px-2 py-2">
                          <button
                            type="button"
                            data-testid={`schedule-state-${flowId}-${nodeId}-${rule.index}`}
                            onClick={() => handleToggle(row)}
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
                        {/* 기간(유효기간) */}
                        <td className="px-2 py-2 text-[11px] text-(--color-text-muted)">
                          {formatValidity(rule.validFrom, rule.validTo)}
                        </td>
                        {/* 우선순위 */}
                        <td className="px-2 py-2 text-(--color-text-secondary)">{rule.priority}</td>
                        {/* 에이전트 */}
                        <td
                          className="px-2 py-2 text-(--color-text-primary)"
                          data-testid={`schedule-agent-${flowId}-${nodeId}-${rule.index}`}
                        >
                          <span
                            className={cn(
                              'text-[11px] font-medium',
                              row.agentId === null && 'text-(--color-text-muted)',
                            )}
                          >
                            {row.agentLabel}
                          </span>
                        </td>
                        {/* 플로우/노드 */}
                        <td className="px-2 py-2">
                          <div className="flex flex-col gap-0.5">
                            <span className="truncate text-[11px] font-medium text-(--color-text-secondary)">
                              {row.nodeName}
                            </span>
                            <span className="truncate text-[10px] text-(--color-text-muted)">
                              {row.flowName}
                            </span>
                          </div>
                        </td>
                        {/* 제어(편집 / 삭제) */}
                        <td className="px-2 py-2 text-right">
                          <div className="flex items-center justify-end gap-1">
                            <button
                              type="button"
                              data-testid={`schedule-edit-${flowId}-${nodeId}-${rule.index}`}
                              onClick={() => openEdit(row)}
                              aria-label="편집"
                              className="rounded p-1 text-gray-400 transition-colors hover:text-blue-600"
                            >
                              <Pencil className="h-3.5 w-3.5" />
                            </button>
                            <button
                              type="button"
                              data-testid={`schedule-delete-${flowId}-${nodeId}-${rule.index}`}
                              onClick={() => handleDelete(row)}
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
                  })
                )}
              </tbody>
            </table>
          </div>
        </>
      )}
    </div>
  );
}
