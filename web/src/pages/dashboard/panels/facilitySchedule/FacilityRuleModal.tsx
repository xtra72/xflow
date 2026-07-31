// SPEC-TRIGGER-SCHED-001 M3~M5: 설비 제어 예약 규칙 생성/편집 모달.
//
// 폼 필드: 이름, 유효기간(from/to), priority, enabled, PLAN(TriggerScheduleEditor 재사용),
// TARGET(피커, M4), ACTION(편집기, M5). 저장 시 규칙 초안을 검증(REQ-03-05)하고 상위로
// 전달한다(상위 패널이 dual-write 수행). 취소 시 변경을 버린다(REQ-03-07).
//
// TARGET 열거는 xsfm 에이전트 컨텍스트가 필요하다. agentId 가 있으면 useStations/useGroups/
// useXsfmDevices 로 라인/그룹/기기를 열거하고, 없으면 자유 입력(free-form)으로 폴백한다.

import { useMemo, useState } from 'react';
import { X } from 'lucide-react';

import { TriggerScheduleEditor } from '@/components/property/TriggerScheduleEditor';
import { useStations, useXsfmDevices } from '@/hooks/useStation';
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

interface FacilityRuleModalProps {
  /** 편집 대상 초안(신규는 emptyDraft). */
  initial: RuleDraft;
  /** TARGET 열거용 xsfm 에이전트 ID(없으면 free-form). */
  agentId: string;
  /** 저장(검증 통과 시). 상위 패널이 dual-write 수행. */
  onSave: (draft: RuleDraft) => void;
  /** 취소(변경 폐기). */
  onCancel: () => void;
}

const inputCls =
  'w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500';
const labelCls = 'mb-1 block text-xs font-medium text-(--color-text-muted)';

/** 설비 제어 예약 규칙 모달. */
export default function FacilityRuleModal({ initial, agentId, onSave, onCancel }: FacilityRuleModalProps) {
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

  const plan = planArr.length > 0 ? planArr[0]! : undefined;
  const draft: RuleDraft = { name, validFrom, validTo, priority, enabled, schedule: plan, target, action };
  const validation = validateRuleDraft(draft);

  const handleSave = () => {
    if (!validation.ok) {
      setShowErrors(true);
      return;
    }
    onSave(draft);
  };

  const errCls = (on: boolean | undefined) => (on && showErrors ? 'border-red-400 focus:border-red-400' : '');

  return (
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
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* 본문(스크롤) */}
        <div className="flex-1 space-y-4 overflow-y-auto px-5 py-4">
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
            agentId={agentId}
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
            data-testid="facility-rule-save"
            className="rounded-md bg-blue-600 px-4 py-1.5 text-sm font-medium text-white transition-colors hover:bg-blue-700"
          >
            저장
          </button>
        </div>
      </div>
    </div>
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

      {/* 값 선택 (에이전트 있으면 열거 select, 없으면 free-form) */}
      {hasAgent ? (
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
          {value.kind === 'device' &&
            (devices ?? []).map((d) => (
              <option key={d.device_id} value={d.device_id}>
                {d.name || d.device_id}
              </option>
            ))}
        </select>
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
