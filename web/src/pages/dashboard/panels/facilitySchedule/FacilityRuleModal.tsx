// SPEC-TRIGGER-SCHED-001 M3~M5: 설비 제어 예약 규칙 생성/편집 모달.
//
// 폼 필드: 이름, 유효기간(from/to), priority, enabled, PLAN(TriggerScheduleEditor 재사용),
// TARGET(피커, M4), ACTION(편집기, M5). 저장 시 규칙 초안을 검증(REQ-03-05)하고 상위로
// 전달한다(상위 패널이 dual-write 수행). 취소 시 변경을 버린다(REQ-03-07).
//
// TARGET 열거는 xsfm 에이전트 컨텍스트가 필요하다. agentId 가 있으면 useStations/useGroups/
// useXsfmDevices 로 라인/그룹/기기를 열거하고, 없으면 자유 입력(free-form)으로 폴백한다.

import { useMemo, useState } from 'react';
import { createPortal } from 'react-dom';
import { X } from 'lucide-react';

import { TriggerScheduleEditor } from '@/components/property/TriggerScheduleEditor';
import { useStations, useXsfmDevices, type AirDevice, type AirStation } from '@/hooks/useStation';
import { useGroups } from '@/hooks/useGroups';
import { cn } from '@/lib/utils/cn';
import {
  buildActionCommand,
  validateRuleDraft,
  type ActionSpec,
  type RuleDraft,
  type SerializedSchedule,
  type TargetKind,
  type TargetSpec,
  FAN_SPEED_MIN,
  FAN_SPEED_MAX,
} from './facilityScheduleUtils';

/** 대상 노드 선택 옵션(스케줄 뷰 CREATE 모드). key = `flowId:nodeId`. */
export interface FacilityRuleModalNode {
  flowId: string;
  nodeId: string;
  /** 표시 라벨(예: `플로우명 / 노드명`). */
  label: string;
  /** 하류 제어 노드에서 유도한 실행 에이전트 후보(자동 각인/프리필용). */
  derivedAgentIds: string[];
}

interface FacilityRuleModalProps {
  /** 편집 대상 초안(신규는 emptyDraft). */
  initial: RuleDraft;
  /** TARGET 열거용 xsfm 에이전트 ID(없으면 free-form). */
  agentId: string;
  /**
   * 선택 가능한 xsfm 에이전트 목록(선택). 제공하면 상단에 에이전트 선택 드롭다운을 노출하고,
   * 선택된 에이전트가 TARGET 열거(useStations/useGroups/useXsfmDevices)를 구동한다. 미제공 시
   * 기존 동작(고정 `agentId`, 셀렉터 없음)을 유지한다 — 패널(FacilitySchedulePanel) 경로 불변.
   */
  agents?: { id: string; name: string }[];
  /** 선택된 에이전트 id 초기값(agents 제공 시). 상위가 각인(agent_id)에 사용. */
  selectedAgentId?: string;
  /** 에이전트 선택 변경 콜백(상위가 각인 대상을 추적하도록). */
  onAgentChange?: (id: string) => void;
  /**
   * 대상 노드 목록(선택). 제공하면 본문 최상단에 "대상 노드" 셀렉터를 노출하고(스케줄 뷰
   * CREATE 모드), 노드 선택 시 유도 에이전트를 프리필해 TARGET 열거를 구동한다. 노드 선택은
   * 필수이며(미선택 시 저장 비활성 + 오류), 상위가 저장 시점에 대상 노드를 결정한다. 미제공 시
   * 셀렉터 없음 — 패널/행 편집 경로 불변.
   */
  nodes?: FacilityRuleModalNode[];
  /** 선택된 대상 노드 key(`flowId:nodeId`) 초기값(nodes 제공 시). */
  selectedNodeKey?: string;
  /** 대상 노드 선택 변경 콜백(상위가 대상 노드를 추적하도록). */
  onNodeChange?: (key: string) => void;
  /** 저장(검증 통과 시). 상위 패널이 dual-write 수행. */
  onSave: (draft: RuleDraft) => void;
  /** 취소(변경 폐기). */
  onCancel: () => void;
}

/** 노드 옵션 key(`flowId:nodeId`). */
function nodeKey(n: FacilityRuleModalNode): string {
  return `${n.flowId}:${n.nodeId}`;
}

const inputCls =
  'w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500';
const labelCls = 'mb-1 block text-xs font-medium text-(--color-text-muted)';

/** 설비 제어 예약 규칙 모달. */
export default function FacilityRuleModal({
  initial,
  agentId,
  agents,
  selectedAgentId,
  onAgentChange,
  nodes,
  selectedNodeKey,
  onNodeChange,
  onSave,
  onCancel,
}: FacilityRuleModalProps) {
  const [name, setName] = useState(initial.name);
  const [validFrom, setValidFrom] = useState(initial.validFrom);
  const [validTo, setValidTo] = useState(initial.validTo);
  const [priority, setPriority] = useState<number>(initial.priority);
  const [enabled, setEnabled] = useState<boolean>(initial.enabled);
  // PLAN: TriggerScheduleEditor 는 배열을 다룬다. 규칙은 단일 스케줄이므로 첫 원소를 PLAN 으로 쓴다.
  const [planArr, setPlanArr] = useState<SerializedSchedule[]>(
    initial.schedule ? [initial.schedule] : [],
  );
  const [target, setTarget] = useState<TargetSpec>(
    initial.target ?? { kind: 'all', value: '' },
  );
  const [action, setAction] = useState<ActionSpec>(initial.action);
  const [showErrors, setShowErrors] = useState(false);

  // 에이전트 선택(선택 기능). agents 제공 시 상단 드롭다운으로 TARGET 열거 에이전트를 고른다.
  const showAgentSelect = Array.isArray(agents) && agents.length > 0;
  const [agentPick, setAgentPick] = useState(selectedAgentId ?? '');
  // TARGET 열거/각인에 쓰는 실효 에이전트: 셀렉터가 있으면 선택값, 없으면 고정 agentId(패널 경로).
  const enumAgentId = showAgentSelect ? agentPick : agentId;
  const handleAgentPick = (id: string) => {
    setAgentPick(id);
    onAgentChange?.(id);
    // 에이전트가 바뀌면 다른 로스터이므로 TARGET 값을 초기화한다(종류는 유지).
    setTarget((t) => ({ kind: t.kind, value: '', byName: false }));
  };

  // 대상 노드 선택(선택 기능, 스케줄 뷰 CREATE 모드). nodes 제공 시 본문 최상단에 노출한다.
  const showNodeSelect = Array.isArray(nodes) && nodes.length > 0;
  const [nodePick, setNodePick] = useState(selectedNodeKey ?? '');
  const handleNodePick = (key: string) => {
    setNodePick(key);
    onNodeChange?.(key);
    // 노드 선택 시 유도 에이전트 자동 각인: 단일 유도면 에이전트 셀렉터를 그 값으로 프리필하고
    // (TARGET 열거를 구동), 미배선(0)·모호(복수)면 셀렉터를 그대로 두어 수동 선택하게 한다.
    const picked = nodes!.find((n) => nodeKey(n) === key);
    if (picked && picked.derivedAgentIds.length === 1) {
      handleAgentPick(picked.derivedAgentIds[0]!);
    }
  };
  // 노드 셀렉터 모드에서 대상 노드는 필수. 미선택 시 저장 비활성 + 오류 노출.
  const nodeMissing = showNodeSelect && nodePick.length === 0;

  const plan = planArr.length > 0 ? planArr[0]! : undefined;
  const draft: RuleDraft = { name, validFrom, validTo, priority, enabled, schedule: plan, target, action };
  const validation = validateRuleDraft(draft);

  const handleSave = () => {
    if (nodeMissing || !validation.ok) {
      setShowErrors(true);
      return;
    }
    onSave(draft);
  };

  const errCls = (on: boolean | undefined) => (on && showErrors ? 'border-red-400 focus:border-red-400' : '');

  // 전체 화면 기준 가운데 정렬을 위해 document.body 로 포털 렌더한다. 패널 조상의
  // transform/overflow 가 fixed 요소의 컨테이닝 블록이 되어 팝업이 패널 경계에서 잘리는
  // 문제를 방지한다(뷰포트 기준 fixed 보장).
  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={(e) => {
        if (e.target === e.currentTarget) onCancel();
      }}
      role="dialog"
      aria-modal="true"
      data-testid="facility-rule-modal"
    >
      <div className="mx-4 flex max-h-[90vh] w-full max-w-lg flex-col rounded-lg bg-(--color-bg-surface) shadow-xl">
        {/* 헤더 */}
        <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-4">
          <h2 className="text-lg font-semibold text-(--color-text-primary)">
            {initial.name ? '예약 규칙 편집' : '새 예약 규칙'}
          </h2>
          <button
            type="button"
            onClick={onCancel}
            aria-label="닫기"
            data-testid="facility-rule-cancel"
            className="rounded-md p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary)"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* 본문(스크롤) */}
        <div className="flex-1 space-y-4 overflow-y-auto px-5 py-4">
          {/* 대상 노드 선택(선택 기능) — 스케줄 뷰 CREATE 모드. 본문 최상단 필드. */}
          {showNodeSelect && (
            <div>
              <label className={labelCls} htmlFor="fr-node">
                대상 노드 <span className="text-red-500">*</span>
              </label>
              <select
                id="fr-node"
                data-testid="fr-node-select"
                value={nodePick}
                onChange={(e) => handleNodePick(e.target.value)}
                className={cn(inputCls, nodeMissing && showErrors && 'border-red-400 focus:border-red-400')}
              >
                <option value="">노드 선택…</option>
                {nodes!.map((n) => (
                  <option key={nodeKey(n)} value={nodeKey(n)}>
                    {n.label}
                  </option>
                ))}
              </select>
              {showErrors && nodeMissing && (
                <p data-testid="fr-error-node" className="mt-1 text-[11px] text-red-500">
                  대상 노드를 선택하세요.
                </p>
              )}
            </div>
          )}

          {/* 설비 에이전트 선택(선택 기능) — TARGET 열거/각인 대상 */}
          {showAgentSelect && (
            <div>
              <label className={labelCls} htmlFor="fr-agent">
                설비 에이전트
              </label>
              <select
                id="fr-agent"
                data-testid="fr-agent-select"
                value={agentPick}
                onChange={(e) => handleAgentPick(e.target.value)}
                className={inputCls}
              >
                <option value="">(선택 — 자유 입력)</option>
                {agents!.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
              </select>
            </div>
          )}

          {/* 이름 */}
          <div>
            <label className={labelCls} htmlFor="fr-name">
              이름 <span className="text-red-500">*</span>
            </label>
            <input
              id="fr-name"
              data-testid="fr-name"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="예: 평일 정규 가동"
              className={cn(inputCls, errCls(validation.errors.name))}
            />
            {showErrors && validation.errors.name && (
              <p data-testid="fr-error-name" className="mt-1 text-[11px] text-red-500">
                이름을 입력하세요.
              </p>
            )}
          </div>

          {/* 유효기간 */}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className={labelCls} htmlFor="fr-valid-from">
                유효 시작
              </label>
              <input
                id="fr-valid-from"
                data-testid="fr-valid-from"
                type="date"
                value={validFrom}
                onChange={(e) => setValidFrom(e.target.value)}
                className={inputCls}
              />
            </div>
            <div>
              <label className={labelCls} htmlFor="fr-valid-to">
                유효 종료 <span className="text-(--color-text-muted)">(빈 값=무기한)</span>
              </label>
              <input
                id="fr-valid-to"
                data-testid="fr-valid-to"
                type="date"
                value={validTo}
                onChange={(e) => setValidTo(e.target.value)}
                className={inputCls}
              />
            </div>
          </div>

          {/* priority + enabled */}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className={labelCls} htmlFor="fr-priority">
                우선순위
              </label>
              <input
                id="fr-priority"
                data-testid="fr-priority"
                type="number"
                value={priority}
                onChange={(e) => setPriority(Number(e.target.value) || 0)}
                className={inputCls}
              />
            </div>
            <div>
              <label className={labelCls}>활성 상태</label>
              <button
                type="button"
                data-testid="fr-enabled"
                aria-pressed={enabled}
                onClick={() => setEnabled((v) => !v)}
                className={cn(
                  'w-full rounded-md border px-3 py-2 text-sm font-medium transition-colors',
                  enabled
                    ? 'border-green-400 bg-green-50 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                    : 'border-(--color-border-default) bg-(--color-bg-elevated) text-(--color-text-muted)',
                )}
              >
                {enabled ? '활성' : '비활성'}
              </button>
            </div>
          </div>

          {/* PLAN (스케줄) — TriggerScheduleEditor 재사용 */}
          <div>
            <label className={labelCls}>
              실행 계획(PLAN) <span className="text-red-500">*</span>
            </label>
            <TriggerScheduleEditor value={planArr} onChange={(v) => setPlanArr(v as SerializedSchedule[])} />
            {showErrors && validation.errors.plan && (
              <p data-testid="fr-error-plan" className="mt-1 text-[11px] text-red-500">
                실행 계획을 하나 이상 추가하세요.
              </p>
            )}
          </div>

          {/* TARGET (M4) */}
          <TargetPicker
            agentId={enumAgentId}
            value={target}
            onChange={setTarget}
            invalid={showErrors && !!validation.errors.target}
          />

          {/* ACTION (M5) */}
          <ActionEditor
            value={action}
            onChange={setAction}
            invalid={showErrors && !!validation.errors.action}
          />
        </div>

        {/* 푸터 */}
        <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-5 py-3">
          <button
            type="button"
            onClick={onCancel}
            data-testid="facility-rule-cancel-btn"
            className="rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-4 py-1.5 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-border-default)"
          >
            취소
          </button>
          <button
            type="button"
            onClick={handleSave}
            aria-disabled={nodeMissing}
            data-testid="facility-rule-save"
            className={cn(
              'rounded-md bg-blue-600 px-4 py-1.5 text-sm font-medium text-white transition-colors hover:bg-blue-700',
              nodeMissing && 'cursor-not-allowed opacity-50 hover:bg-blue-600',
            )}
          >
            저장
          </button>
        </div>
      </div>
    </div>,
    document.body,
  );
}

// ---------------------------------------------------------------------------
// TARGET 피커 (M4) — 전체/그룹/개별 + 값 선택. agentId 없으면 free-form.
// ---------------------------------------------------------------------------

const TARGET_KINDS: { kind: TargetKind; label: string }[] = [
  { kind: 'all', label: '전체(호선)' },
  { kind: 'group', label: '그룹' },
  { kind: 'device', label: '개별' },
];

function TargetPicker({
  agentId,
  value,
  onChange,
  invalid,
}: {
  agentId: string;
  value: TargetSpec;
  onChange: (v: TargetSpec) => void;
  invalid: boolean;
}) {
  const hasAgent = agentId.length > 0;
  const { data: stations } = useStations(hasAgent ? agentId : '');
  const { data: devices } = useXsfmDevices(hasAgent ? agentId : '');
  const { data: groups } = useGroups(hasAgent ? agentId : '');

  // 전체(all): 로스터의 distinct line 값.
  const lines = useMemo(
    () => Array.from(new Set((stations ?? []).map((s) => s.line).filter(Boolean))),
    [stations],
  );

  const setKind = (kind: TargetKind) => onChange({ kind, value: '', byName: false });

  return (
    <div>
      <label className={labelCls}>
        대상(TARGET) <span className="text-red-500">*</span>
      </label>
      {/* 종류 선택 */}
      <div className="mb-2 flex gap-1" data-testid="fr-target-kinds">
        {TARGET_KINDS.map((k) => (
          <button
            key={k.kind}
            type="button"
            data-testid={`fr-target-kind-${k.kind}`}
            aria-pressed={value.kind === k.kind}
            onClick={() => setKind(k.kind)}
            className={cn(
              'flex-1 rounded-md border px-2 py-1.5 text-xs font-medium transition-colors',
              value.kind === k.kind
                ? 'border-blue-400 bg-blue-500 text-white'
                : 'border-(--color-border-default) bg-(--color-bg-elevated) text-(--color-text-secondary) hover:border-blue-400 hover:text-blue-600',
            )}
          >
            {k.label}
          </button>
        ))}
      </div>

      {/* 값 선택: 에이전트가 있으면 열거(개별=테이블 / 전체·그룹=select), 없으면 free-form */}
      {hasAgent ? (
        value.kind === 'device' ? (
          // 개별(device): 이름/라인/역사/위치 테이블에서 행 선택(REQ — 로스터가 커서 검색 지원).
          <DeviceTargetTable
            devices={devices ?? []}
            stations={stations ?? []}
            selectedId={value.value}
            onSelect={(deviceId) => onChange({ ...value, value: deviceId, byName: false })}
            invalid={invalid}
          />
        ) : (
          <select
            data-testid="fr-target-value"
            value={value.value}
            onChange={(e) => onChange({ ...value, value: e.target.value, byName: false })}
            className={cn(inputCls, invalid && 'border-red-400')}
          >
            <option value="">선택…</option>
            {value.kind === 'all' &&
              lines.map((l) => (
                <option key={l} value={l}>
                  {l}호선 전체
                </option>
              ))}
            {value.kind === 'group' &&
              (groups ?? []).map((g) => (
                <option key={g.id} value={g.id}>
                  {g.name} · {g.member_count}대
                </option>
              ))}
          </select>
        )
      ) : (
        <div className="space-y-1">
          <input
            type="text"
            data-testid="fr-target-value"
            value={value.value}
            onChange={(e) => onChange({ ...value, value: e.target.value })}
            placeholder={
              value.kind === 'all' ? '호선 코드 (예: 2)' : value.kind === 'group' ? '그룹 값' : '기기 값'
            }
            className={cn(inputCls, invalid && 'border-red-400')}
          />
          {value.kind !== 'all' && (
            <label className="flex items-center gap-1 text-[11px] text-(--color-text-muted)">
              <input
                type="checkbox"
                data-testid="fr-target-byname"
                checked={!!value.byName}
                onChange={(e) => onChange({ ...value, byName: e.target.checked })}
              />
              이름으로 지정({value.kind === 'group' ? 'group_name' : 'device_name'})
            </label>
          )}
        </div>
      )}
      {invalid && (
        <p data-testid="fr-error-target" className="mt-1 text-[11px] text-red-500">
          대상을 선택하세요.
        </p>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// 개별(device) TARGET 테이블 — 이름/라인/역사/위치. 검색 필터 + 행 선택.
// ---------------------------------------------------------------------------

/** 디바이스 로스터 × 역사/위치 조인 결과(테이블 한 행). 매칭 실패 시 원본 id 로 폴백. */
interface DeviceRow {
  device: AirDevice;
  name: string;
  line: string;
  stationName: string;
  placeName: string;
}

/**
 * 개별 대상 선택 테이블. device.station 으로 역사(라인·표시명)를, device.place 로 위치 표시명을
 * 조인한다(매칭 실패 시 원본 id 폴백). 상단 검색 입력이 이름/라인/역사/위치 부분일치로 필터링한다.
 * 행 클릭/Enter/Space 로 device_id 를 선택하며(byName 의미 불변), 선택 행을 강조한다.
 */
function DeviceTargetTable({
  devices,
  stations,
  selectedId,
  onSelect,
  invalid,
}: {
  devices: AirDevice[];
  stations: AirStation[];
  selectedId: string;
  onSelect: (deviceId: string) => void;
  invalid: boolean;
}) {
  const [search, setSearch] = useState('');

  const rows = useMemo<DeviceRow[]>(() => {
    const byStation = new Map(stations.map((s) => [s.station, s]));
    return devices.map((d) => {
      const st = byStation.get(d.station);
      const place = st?.places.find((p) => p.place === d.place);
      return {
        device: d,
        name: d.name || d.device_id,
        line: st?.line || d.station || '',
        stationName: st?.display_name || d.station || '',
        placeName: place?.display_name || d.place || '',
      };
    });
  }, [devices, stations]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return rows;
    return rows.filter((r) =>
      [r.name, r.line, r.stationName, r.placeName].some((f) => f.toLowerCase().includes(q)),
    );
  }, [rows, search]);

  // 현재 선택된 기기 요약(전체/그룹의 select 값 표시와 동등한 가시성 제공). 로스터에서
  // 매칭 실패하면 원본 device_id 로 폴백해 항상 선택값을 노출한다(테이블 스크롤과 무관).
  const selectedRow = useMemo(
    () => rows.find((r) => r.device.device_id === selectedId),
    [rows, selectedId],
  );

  return (
    <div className="space-y-1.5">
      {/* 선택값 표시(전체/그룹 select 와 동일한 가시성). 선택 시에만 노출 + 해제 버튼. */}
      {selectedId && (
        <div
          data-testid="fr-target-device-selected"
          className="flex items-center justify-between gap-2 rounded-md border border-blue-400 bg-blue-500/10 px-2.5 py-1.5 text-xs text-(--color-text-primary)"
        >
          <span className="min-w-0 truncate">
            <span className="text-(--color-text-muted)">선택됨: </span>
            {selectedRow
              ? `${selectedRow.name}${selectedRow.line ? ` · ${selectedRow.line}호선` : ''}${
                  selectedRow.stationName ? ` ${selectedRow.stationName}` : ''
                }`
              : selectedId}
          </span>
          <button
            type="button"
            onClick={() => onSelect('')}
            aria-label="선택 해제"
            data-testid="fr-target-device-clear"
            className="shrink-0 rounded p-0.5 text-(--color-text-muted) transition-colors hover:text-red-500"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      )}
      <input
        type="text"
        data-testid="fr-target-device-search"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        placeholder="이름/라인/역사/위치 검색…"
        aria-label="기기 검색"
        className={inputCls}
      />
      <div
        className={cn(
          'max-h-56 overflow-y-auto rounded-md border border-(--color-border-default)',
          invalid && 'border-red-400',
        )}
      >
        <table
          className="w-full border-collapse text-left text-xs"
          data-testid="fr-target-device-table"
        >
          <thead className="sticky top-0 bg-(--color-bg-elevated) text-(--color-text-muted)">
            <tr>
              <th scope="col" className="px-2 py-1.5 font-medium">이름</th>
              <th scope="col" className="px-2 py-1.5 font-medium">라인</th>
              <th scope="col" className="px-2 py-1.5 font-medium">역사</th>
              <th scope="col" className="px-2 py-1.5 font-medium">위치</th>
            </tr>
          </thead>
          <tbody>
            {filtered.length === 0 ? (
              <tr>
                <td
                  colSpan={4}
                  data-testid="fr-target-device-empty"
                  className="px-2 py-3 text-center text-(--color-text-muted)"
                >
                  {rows.length === 0 ? '기기 로스터가 비어 있습니다.' : '검색 결과가 없습니다.'}
                </td>
              </tr>
            ) : (
              filtered.map((r) => {
                const selected = r.device.device_id === selectedId;
                return (
                  <tr
                    key={r.device.device_id}
                    data-testid={`fr-target-device-row-${r.device.device_id}`}
                    role="button"
                    tabIndex={0}
                    aria-pressed={selected}
                    onClick={() => onSelect(r.device.device_id)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        onSelect(r.device.device_id);
                      }
                    }}
                    className={cn(
                      'cursor-pointer border-t border-(--color-border-default) outline-none transition-colors',
                      selected
                        ? 'bg-blue-500 text-white'
                        : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated) focus:bg-(--color-bg-elevated)',
                    )}
                  >
                    <td className="px-2 py-1.5">
                      <span className="flex items-center gap-1.5">
                        <span
                          aria-hidden="true"
                          title={r.device.online ? '온라인' : '오프라인'}
                          className={cn(
                            'inline-block h-1.5 w-1.5 shrink-0 rounded-full',
                            r.device.online ? 'bg-green-500' : 'bg-(--color-status-stopped)',
                          )}
                        />
                        {r.name}
                      </span>
                    </td>
                    <td className="px-2 py-1.5">{r.line}</td>
                    <td className="px-2 py-1.5">{r.stationName}</td>
                    <td className="px-2 py-1.5">{r.placeName}</td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// ACTION 편집기 (M5) — 전원(ON/OFF/무변경) + 풍량(1~3/무변경). 모드 축 없음(RD-4).
// ---------------------------------------------------------------------------

/** 전원 select 값 인코딩: '' = 무변경, 'on' = true, 'off' = false. */
function powerToStr(p: boolean | null): string {
  return p === null ? '' : p ? 'on' : 'off';
}
function strToPower(s: string): boolean | null {
  return s === 'on' ? true : s === 'off' ? false : null;
}

const FAN_OPTIONS = Array.from({ length: FAN_SPEED_MAX - FAN_SPEED_MIN + 1 }, (_, i) => FAN_SPEED_MIN + i);

function ActionEditor({
  value,
  onChange,
  invalid,
}: {
  value: ActionSpec;
  onChange: (v: ActionSpec) => void;
  invalid: boolean;
}) {
  const preview = buildActionCommand(value);
  return (
    <div>
      <label className={labelCls}>
        제어 명령(ACTION) <span className="text-red-500">*</span>
      </label>
      <div className="grid grid-cols-2 gap-3">
        {/* 전원 */}
        <div>
          <span className="mb-1 block text-[11px] text-(--color-text-muted)">전원</span>
          <select
            data-testid="fr-action-power"
            value={powerToStr(value.power)}
            onChange={(e) => onChange({ ...value, power: strToPower(e.target.value) })}
            className={inputCls}
          >
            <option value="">무변경</option>
            <option value="on">ON</option>
            <option value="off">OFF</option>
          </select>
        </div>
        {/* 풍량 */}
        <div>
          <span className="mb-1 block text-[11px] text-(--color-text-muted)">풍량</span>
          <select
            data-testid="fr-action-fan"
            value={value.fanSpeed === null ? '' : String(value.fanSpeed)}
            onChange={(e) =>
              onChange({ ...value, fanSpeed: e.target.value === '' ? null : Number(e.target.value) })
            }
            className={inputCls}
          >
            <option value="">무변경</option>
            {FAN_OPTIONS.map((n) => (
              <option key={n} value={String(n)}>
                풍량 {n}
              </option>
            ))}
          </select>
        </div>
      </div>
      {/* 조립 미리보기(디버그/확인) */}
      <p className="mt-1 text-[11px] text-(--color-text-muted)" data-testid="fr-action-preview">
        {preview ? `${preview.command}` : '전원 또는 풍량을 지정하세요.'}
      </p>
      {invalid && (
        <p data-testid="fr-error-action" className="mt-0.5 text-[11px] text-red-500">
          전원 또는 풍량 중 하나 이상을 지정하세요.
        </p>
      )}
    </div>
  );
}
