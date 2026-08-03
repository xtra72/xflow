// MODBUS Register Remapper(modbus-remap) 노드 편집기 — SPEC-MODBUS-007.
//
// 단일 결합 필드(modbus_remap)로 두 배열을 관리하고 { rules, templates } 복합 객체를
// 방출한다. DynamicForm 이 이를 config.rules / config.templates 두 최상위 키로 spread
// 한다(agent_select 과 동일 패턴).
//
// 백엔드 rules 형상(런타임 처리, 정확히 일치):
//   { source_unit_id?, source_area, source_address, count,
//     targets: [ { target_unit_id, target_area?, target_address } ] }   // 1..N 팬아웃
//   area ∈ coils|discrete_inputs|holding_registers|input_registers
//   target_area 생략 → source_area 유지. legacy 최상위 단일 target_* 는 1-원소 targets 로 정규화.
//
// templates 형상(에디터 관리, 백엔드 미처리, config 저장):
//   { name, rules: [ { source_unit_id?, source_area, source_offset, count,
//     targets: [ { target_area?, target_offset, target_unit_offset? } ] } ] }
//   적용(apply, start+device_id): 패턴을 구체 rules 로 materialize 하여 rules 에 append.
//     source_address = start + source_offset
//     target_address = start + target_offset
//     target_unit_id = device_id + (target_unit_offset||0)
//
// UI: rules 는 컴팩트 목록(C/D/H/I 약어) + 팝업 편집 + 일괄등록. templates 는 이름 지정
// 다중 패턴 규칙 관리 + 적용(start/device_id 다이얼로그)으로 rules materialize.

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { ClipboardPaste, Pencil, Plus, Trash2, X, Play } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';

// ---- 상수 ----

const AREA_OPTIONS = [
  'coils',
  'discrete_inputs',
  'holding_registers',
  'input_registers',
] as const;

type AreaKey = (typeof AREA_OPTIONS)[number];

const DEFAULT_AREA: AreaKey = 'holding_registers';

/** 영역 → 1글자 약어(목록 표시용). */
const AREA_ABBREV: Record<string, string> = {
  coils: 'C',
  discrete_inputs: 'D',
  holding_registers: 'H',
  input_registers: 'I',
};

/** 약어(대소문자 무관) → 영역. */
const ABBREV_TO_AREA: Record<string, AreaKey> = {
  c: 'coils',
  d: 'discrete_inputs',
  h: 'holding_registers',
  i: 'input_registers',
};

/** 약어 또는 전체 이름 문자열 → 영역(없으면 null). */
function resolveArea(cell: string): AreaKey | null {
  const s = cell.trim();
  if ((AREA_OPTIONS as readonly string[]).includes(s)) return s as AreaKey;
  const ab = ABBREV_TO_AREA[s.toLowerCase()];
  return ab ?? null;
}

function abbrev(area: string): string {
  return AREA_ABBREV[area] ?? area;
}

// ---- 내부 행 타입 (rules: 구체/절대) ----

interface TargetRow {
  key: string;
  targetUnitId: number;
  targetArea: string; // '' = keep source_area
  targetAddress: number;
}

interface RuleRow {
  key: string;
  sourceUnitId: string; // '' = omit
  sourceArea: string;
  sourceAddress: number;
  count: number;
  targets: TargetRow[]; // ≥1
}

// ---- 내부 행 타입 (templates: 패턴/상대) ----

interface PatternTargetRow {
  key: string;
  targetArea: string; // '' = keep
  targetOffset: number;
  targetUnitOffset: string; // '' = omit(default 0)
}

interface PatternRuleRow {
  key: string;
  sourceUnitId: string; // '' = omit
  sourceArea: string;
  sourceOffset: number;
  count: number;
  targets: PatternTargetRow[]; // ≥1
}

interface TemplateDef {
  key: string;
  name: string;
  rules: PatternRuleRow[];
}

// ---- 방출 타입 ----

interface EmittedTarget {
  target_unit_id: number;
  target_area?: string;
  target_address: number;
}
interface EmittedRule {
  source_unit_id?: number;
  source_area: string;
  source_address: number;
  count: number;
  targets: EmittedTarget[];
}
interface EmittedPatternTarget {
  target_area?: string;
  target_offset: number;
  target_unit_offset?: number;
}
interface EmittedPatternRule {
  source_unit_id?: number;
  source_area: string;
  source_offset: number;
  count: number;
  targets: EmittedPatternTarget[];
}
interface EmittedTemplate {
  name: string;
  rules: EmittedPatternRule[];
}

// ---- 변환 유틸 ----

let keyCounter = 0;
function nextKey(prefix: string): string {
  return `${prefix}-${++keyCounter}-${Date.now()}`;
}

function asObject(v: unknown): Record<string, unknown> {
  return v && typeof v === 'object' && !Array.isArray(v)
    ? (v as Record<string, unknown>)
    : {};
}
function asString(v: unknown): string {
  return typeof v === 'string' ? v : '';
}
function numOr(v: unknown, def: number): number {
  if (typeof v === 'number' && Number.isFinite(v)) return v;
  if (typeof v === 'string' && v.trim() !== '') {
    const n = Number(v);
    if (Number.isFinite(n)) return n;
  }
  return def;
}
function optNumToText(v: unknown): string {
  return typeof v === 'number' && Number.isFinite(v) ? String(v) : '';
}
function isNonNegInt(s: string): boolean {
  return /^\d+$/.test(s);
}
function toArray(value: unknown): unknown[] {
  let source: unknown = value;
  if (typeof value === 'string') {
    if (value.trim() === '') return [];
    try {
      source = JSON.parse(value);
    } catch {
      return [];
    }
  }
  return Array.isArray(source) ? source : [];
}

// --- rules 파싱/방출 ---

function toTargetRow(item: unknown): TargetRow {
  const o = asObject(item);
  return {
    key: nextKey('tgt'),
    targetUnitId: numOr(o.target_unit_id, 1),
    targetArea: asString(o.target_area),
    targetAddress: numOr(o.target_address, 0),
  };
}
function newTargetRow(): TargetRow {
  return { key: nextKey('tgt'), targetUnitId: 1, targetArea: '', targetAddress: 0 };
}
function toRuleRow(item: unknown): RuleRow {
  const o = asObject(item);
  let targets: TargetRow[];
  if (Array.isArray(o.targets) && o.targets.length > 0) {
    targets = o.targets.map(toTargetRow);
  } else if (
    o.target_unit_id !== undefined ||
    o.target_area !== undefined ||
    o.target_address !== undefined
  ) {
    targets = [
      {
        key: nextKey('tgt'),
        targetUnitId: numOr(o.target_unit_id, 1),
        targetArea: asString(o.target_area),
        targetAddress: numOr(o.target_address, 0),
      },
    ];
  } else {
    targets = [newTargetRow()];
  }
  return {
    key: nextKey('rule'),
    sourceUnitId: optNumToText(o.source_unit_id),
    sourceArea: asString(o.source_area) || DEFAULT_AREA,
    sourceAddress: numOr(o.source_address, 0),
    count: numOr(o.count, 1),
    targets,
  };
}
function toRuleRows(value: unknown): RuleRow[] {
  return toArray(value).map(toRuleRow);
}
function newRuleRow(): RuleRow {
  return {
    key: nextKey('rule'),
    sourceUnitId: '',
    sourceArea: DEFAULT_AREA,
    sourceAddress: 0,
    count: 1,
    targets: [newTargetRow()],
  };
}
function toEmitTarget(t: TargetRow): EmittedTarget {
  const out: EmittedTarget = {
    target_unit_id: t.targetUnitId,
    target_address: t.targetAddress,
  };
  if (t.targetArea.trim() !== '') out.target_area = t.targetArea;
  return out;
}
function toEmitRule(r: RuleRow): EmittedRule {
  const out: EmittedRule = {
    source_area: r.sourceArea,
    source_address: r.sourceAddress,
    count: r.count,
    targets: r.targets.map(toEmitTarget),
  };
  if (r.sourceUnitId.trim() !== '') out.source_unit_id = numOr(r.sourceUnitId, 0);
  return out;
}

// --- templates 파싱/방출 ---

function toPatternTargetRow(item: unknown): PatternTargetRow {
  const o = asObject(item);
  return {
    key: nextKey('ptgt'),
    targetArea: asString(o.target_area),
    targetOffset: numOr(o.target_offset, 0),
    targetUnitOffset: optNumToText(o.target_unit_offset),
  };
}
function newPatternTargetRow(): PatternTargetRow {
  return { key: nextKey('ptgt'), targetArea: '', targetOffset: 0, targetUnitOffset: '' };
}
function toPatternRuleRow(item: unknown): PatternRuleRow {
  const o = asObject(item);
  const targets =
    Array.isArray(o.targets) && o.targets.length > 0
      ? o.targets.map(toPatternTargetRow)
      : [newPatternTargetRow()];
  return {
    key: nextKey('prule'),
    sourceUnitId: optNumToText(o.source_unit_id),
    sourceArea: asString(o.source_area) || DEFAULT_AREA,
    sourceOffset: numOr(o.source_offset, 0),
    count: numOr(o.count, 1),
    targets,
  };
}
function newPatternRuleRow(): PatternRuleRow {
  return {
    key: nextKey('prule'),
    sourceUnitId: '',
    sourceArea: DEFAULT_AREA,
    sourceOffset: 0,
    count: 1,
    targets: [newPatternTargetRow()],
  };
}
function toTemplateDef(item: unknown): TemplateDef {
  const o = asObject(item);
  return {
    key: nextKey('tpl'),
    name: asString(o.name),
    rules: Array.isArray(o.rules) ? o.rules.map(toPatternRuleRow) : [],
  };
}
function toTemplateDefs(value: unknown): TemplateDef[] {
  return toArray(value).map(toTemplateDef);
}
function newTemplateDef(name: string): TemplateDef {
  return { key: nextKey('tpl'), name, rules: [newPatternRuleRow()] };
}
function toEmitPatternTarget(t: PatternTargetRow): EmittedPatternTarget {
  const out: EmittedPatternTarget = { target_offset: t.targetOffset };
  if (t.targetArea.trim() !== '') out.target_area = t.targetArea;
  if (t.targetUnitOffset.trim() !== '') out.target_unit_offset = numOr(t.targetUnitOffset, 0);
  return out;
}
function toEmitPatternRule(r: PatternRuleRow): EmittedPatternRule {
  const out: EmittedPatternRule = {
    source_area: r.sourceArea,
    source_offset: r.sourceOffset,
    count: r.count,
    targets: r.targets.map(toEmitPatternTarget),
  };
  if (r.sourceUnitId.trim() !== '') out.source_unit_id = numOr(r.sourceUnitId, 0);
  return out;
}
function toEmitTemplate(tpl: TemplateDef): EmittedTemplate {
  return { name: tpl.name, rules: tpl.rules.map(toEmitPatternRule) };
}

// --- apply: 템플릿 패턴 → 구체 rules (순수 함수, 단위 테스트 대상) ---

/**
 * 템플릿 패턴을 start/device_id 로 구체 rules 로 materialize 한다.
 *   source_address = start + source_offset
 *   target_address = start + target_offset
 *   target_unit_id = device_id + (target_unit_offset||0)
 */
export function materializeTemplate(
  tpl: EmittedTemplate,
  start: number,
  deviceId: number,
): EmittedRule[] {
  return tpl.rules.map((pr) => {
    const out: EmittedRule = {
      source_area: pr.source_area,
      source_address: start + pr.source_offset,
      count: pr.count,
      targets: pr.targets.map((pt) => {
        const t: EmittedTarget = {
          target_unit_id: deviceId + (pt.target_unit_offset ?? 0),
          target_address: start + pt.target_offset,
        };
        if (pt.target_area) t.target_area = pt.target_area;
        return t;
      }),
    };
    if (pr.source_unit_id !== undefined) out.source_unit_id = pr.source_unit_id;
    return out;
  });
}

// --- 일괄등록 파서 (순수 함수, 단위 테스트 대상) ---

export interface BulkParseError {
  line: number;
  code: string;
}
export interface BulkParseResult {
  rules: EmittedRule[];
  errors: BulkParseError[];
}

function splitCells(line: string): string[] {
  const parts = line.includes('\t') ? line.split('\t') : line.split(',');
  return parts.map((c) => c.trim());
}

/**
 * 붙여넣기 텍스트 → rules(각 1 타깃). 순수 함수.
 * 컬럼: source_area, source_address, count, target_unit_id, target_area, target_address [, source_unit_id]
 *  - area 는 약어(C/D/H/I) 또는 전체 이름 모두 허용. target_area 빈 셀 → 생략(keep).
 *  - 6 또는 7 컬럼(7번째 = source_unit_id, 선택). 그 외 → wrongColumnCount.
 *  - 빈 줄 무시. 첫 non-empty 줄의 첫 셀이 영역으로 해석 안 되면 헤더로 보고 무시.
 *  - 오류가 있어도 유효 rule 은 담아 반환(호출부 block-on-error).
 */
export function parseBulkRules(text: string): BulkParseResult {
  const rules: EmittedRule[] = [];
  const errors: BulkParseError[] = [];
  const lines = text.split(/\r?\n/);
  let firstSeen = false;

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]!.trim();
    if (line === '') continue;
    const cells = splitCells(line);

    if (!firstSeen) {
      firstSeen = true;
      if (resolveArea(cells[0] ?? '') === null) continue; // 헤더 스킵
    }

    const lineNo = i + 1;
    const n = cells.length;
    if (n !== 6 && n !== 7) {
      errors.push({ line: lineNo, code: 'wrongColumnCount' });
      continue;
    }
    const srcArea = resolveArea(cells[0] ?? '');
    if (!srcArea) {
      errors.push({ line: lineNo, code: 'invalidSourceArea' });
      continue;
    }
    const srcAddr = cells[1] ?? '';
    const countCell = cells[2] ?? '';
    const tgtUnit = cells[3] ?? '';
    const tgtAreaCell = cells[4] ?? '';
    const tgtAddr = cells[5] ?? '';
    const srcUnitCell = cells[6] ?? '';

    if (!isNonNegInt(srcAddr)) {
      errors.push({ line: lineNo, code: 'invalidAddress' });
      continue;
    }
    if (!isNonNegInt(countCell) || Number(countCell) < 1) {
      errors.push({ line: lineNo, code: 'invalidCount' });
      continue;
    }
    if (!isNonNegInt(tgtUnit)) {
      errors.push({ line: lineNo, code: 'invalidTargetUnitId' });
      continue;
    }
    let tgtArea: AreaKey | null = null;
    if (tgtAreaCell !== '') {
      tgtArea = resolveArea(tgtAreaCell);
      if (!tgtArea) {
        errors.push({ line: lineNo, code: 'invalidTargetArea' });
        continue;
      }
    }
    if (!isNonNegInt(tgtAddr)) {
      errors.push({ line: lineNo, code: 'invalidTargetAddress' });
      continue;
    }
    if (n === 7 && srcUnitCell !== '' && !isNonNegInt(srcUnitCell)) {
      errors.push({ line: lineNo, code: 'invalidSourceUnitId' });
      continue;
    }

    const target: EmittedTarget = {
      target_unit_id: Number(tgtUnit),
      target_address: Number(tgtAddr),
    };
    if (tgtArea) target.target_area = tgtArea;
    const rule: EmittedRule = {
      source_area: srcArea,
      source_address: Number(srcAddr),
      count: Number(countCell),
      targets: [target],
    };
    if (n === 7 && srcUnitCell !== '') rule.source_unit_id = Number(srcUnitCell);
    rules.push(rule);
  }
  return { rules, errors };
}

// ---- 스타일 ----

const cellInput = cn(
  'w-full rounded border px-2 py-1 text-sm',
  'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-primary)',
  'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
  'dark:focus:border-blue-500',
);
const readOnlyInput = 'cursor-not-allowed bg-(--color-bg-elevated)';
const fieldLabel =
  'block text-[10px] font-medium uppercase tracking-wide text-(--color-text-muted)';
const addButton = cn(
  'inline-flex items-center gap-1 rounded-md border border-dashed border-(--color-border-default) px-3 py-1.5 text-xs font-medium',
  'text-(--color-text-muted) transition-colors hover:border-blue-400 hover:text-blue-600',
  'dark:hover:border-blue-500 dark:hover:text-blue-400',
);
const iconButton = cn(
  'shrink-0 rounded p-1 text-gray-400 transition-colors',
  'hover:bg-(--color-bg-elevated) hover:text-(--color-text-primary)',
);
const removeButton = cn(
  'shrink-0 rounded p-1 text-gray-400 transition-colors',
  'hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400',
);
const primaryButton =
  'rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50';
const secondaryButton =
  'rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-secondary)';

// ---- 공통 위젯 ----

function AreaSelect({
  value,
  disabled,
  onChange,
  ariaLabel,
}: {
  value: string;
  disabled?: boolean;
  onChange: (v: string) => void;
  ariaLabel: string;
}) {
  return (
    <select
      value={value}
      disabled={disabled}
      onChange={(e) => onChange(e.target.value)}
      aria-label={ariaLabel}
      className={cn(cellInput, disabled && readOnlyInput)}
    >
      {AREA_OPTIONS.map((a) => (
        <option key={a} value={a}>
          {a}
        </option>
      ))}
    </select>
  );
}

function TargetAreaSelect({
  value,
  disabled,
  onChange,
  ariaLabel,
  t,
}: {
  value: string;
  disabled?: boolean;
  onChange: (v: string) => void;
  ariaLabel: string;
  t: (k: string) => string;
}) {
  return (
    <select
      value={value}
      disabled={disabled}
      onChange={(e) => onChange(e.target.value)}
      aria-label={ariaLabel}
      className={cn(cellInput, disabled && readOnlyInput)}
    >
      <option value="">{t('property.modbusRemap.keepArea')}</option>
      {AREA_OPTIONS.map((a) => (
        <option key={a} value={a}>
          {a}
        </option>
      ))}
    </select>
  );
}

/** 표준 모달 셸 (RenameKeyDialog 패턴). */
function ModalShell({
  title,
  onClose,
  children,
  footer,
  closeLabel,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  footer: ReactNode;
  closeLabel: string;
}) {
  useEffect(() => {
    const onKey = (e: globalThis.KeyboardEvent): void => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
    >
      <div
        className="mx-4 flex max-h-[85vh] w-full max-w-[560px] flex-col rounded-lg bg-(--color-bg-surface) shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-3">
          <h2 className="text-base font-semibold text-(--color-text-primary)">{title}</h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label={closeLabel}
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="space-y-4 overflow-y-auto px-5 py-4">{children}</div>
        <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-5 py-3">
          {footer}
        </div>
      </div>
    </div>
  );
}

// ══════════════════════════════════════════════════════════════════════════
// 규칙 편집 팝업 (구체/절대)
// ══════════════════════════════════════════════════════════════════════════

function RuleEditDialog({
  initial,
  onSave,
  onClose,
}: {
  initial: RuleRow;
  onSave: (rule: RuleRow) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<RuleRow>(initial);

  const patchTarget = (tgtKey: string, patch: Partial<TargetRow>) =>
    setDraft((d) => ({
      ...d,
      targets: d.targets.map((tg) => (tg.key === tgtKey ? { ...tg, ...patch } : tg)),
    }));
  const addTarget = () =>
    setDraft((d) => ({ ...d, targets: [...d.targets, newTargetRow()] }));
  const removeTarget = (tgtKey: string) =>
    setDraft((d) =>
      d.targets.length <= 1
        ? d
        : { ...d, targets: d.targets.filter((tg) => tg.key !== tgtKey) },
    );

  return (
    <ModalShell
      title={t('property.modbusRemap.ruleDialogTitle')}
      onClose={onClose}
      closeLabel={t('property.modbusRemap.close')}
      footer={
        <>
          <button type="button" onClick={onClose} className={secondaryButton}>
            {t('property.modbusRemap.cancel')}
          </button>
          <button type="button" onClick={() => onSave(draft)} className={primaryButton}>
            {t('property.modbusRemap.save')}
          </button>
        </>
      }
    >
      {/* From */}
      <div className="space-y-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) p-2">
        <span className="text-[10px] font-semibold uppercase tracking-wide text-(--color-text-secondary)">
          {t('property.modbusRemap.from')}
        </span>
        <div className="grid grid-cols-[1fr_1.3fr_1fr_1fr] gap-2">
          <label className="space-y-0.5">
            <span className={fieldLabel}>{t('property.modbusRemap.sourceUnitId')}</span>
            <input
              type="number"
              min={0}
              max={247}
              value={draft.sourceUnitId}
              onChange={(e) => setDraft((d) => ({ ...d, sourceUnitId: e.target.value }))}
              aria-label={t('property.modbusRemap.sourceUnitId')}
              placeholder={t('property.modbusRemap.optional')}
              className={cellInput}
            />
          </label>
          <label className="space-y-0.5">
            <span className={fieldLabel}>{t('property.modbusRemap.sourceArea')}</span>
            <AreaSelect
              value={draft.sourceArea}
              onChange={(v) => setDraft((d) => ({ ...d, sourceArea: v }))}
              ariaLabel={t('property.modbusRemap.sourceArea')}
            />
          </label>
          <label className="space-y-0.5">
            <span className={fieldLabel}>{t('property.modbusRemap.sourceAddress')}</span>
            <input
              type="number"
              min={0}
              max={65535}
              value={draft.sourceAddress}
              onChange={(e) =>
                setDraft((d) => ({ ...d, sourceAddress: numOr(e.target.value, 0) }))
              }
              aria-label={t('property.modbusRemap.sourceAddress')}
              className={cellInput}
            />
          </label>
          <label className="space-y-0.5">
            <span className={fieldLabel}>{t('property.modbusRemap.count')}</span>
            <input
              type="number"
              min={1}
              max={65535}
              value={draft.count}
              onChange={(e) => setDraft((d) => ({ ...d, count: numOr(e.target.value, 1) }))}
              aria-label={t('property.modbusRemap.count')}
              className={cellInput}
            />
          </label>
        </div>
        <p className="text-[10px] text-(--color-text-muted)">
          {t('property.modbusRemap.sourceUnitIdHint')}
        </p>
      </div>

      {/* To (multi) */}
      <div className="space-y-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) p-2">
        <span className="text-[10px] font-semibold uppercase tracking-wide text-(--color-text-secondary)">
          {t('property.modbusRemap.to')}
        </span>
        <div className="grid grid-cols-[1fr_1.3fr_1fr_1.75rem] gap-2 px-0.5">
          <span className={fieldLabel}>{t('property.modbusRemap.targetUnitId')}</span>
          <span className={fieldLabel}>{t('property.modbusRemap.targetArea')}</span>
          <span className={fieldLabel}>{t('property.modbusRemap.targetAddress')}</span>
          <span />
        </div>
        {draft.targets.map((tg) => (
          <div key={tg.key} className="grid grid-cols-[1fr_1.3fr_1fr_1.75rem] items-center gap-2">
            <input
              type="number"
              min={0}
              max={247}
              value={tg.targetUnitId}
              onChange={(e) =>
                patchTarget(tg.key, { targetUnitId: numOr(e.target.value, 1) })
              }
              aria-label={t('property.modbusRemap.targetUnitId')}
              className={cellInput}
            />
            <TargetAreaSelect
              value={tg.targetArea}
              onChange={(v) => patchTarget(tg.key, { targetArea: v })}
              ariaLabel={t('property.modbusRemap.targetArea')}
              t={t}
            />
            <input
              type="number"
              min={0}
              max={65535}
              value={tg.targetAddress}
              onChange={(e) =>
                patchTarget(tg.key, { targetAddress: numOr(e.target.value, 0) })
              }
              aria-label={t('property.modbusRemap.targetAddress')}
              className={cellInput}
            />
            {draft.targets.length > 1 ? (
              <button
                type="button"
                onClick={() => removeTarget(tg.key)}
                className={removeButton}
                aria-label={t('property.modbusRemap.removeTarget')}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            ) : (
              <span />
            )}
          </div>
        ))}
        <button type="button" onClick={addTarget} className={addButton}>
          <Plus className="h-3.5 w-3.5" />
          {t('property.modbusRemap.addTarget')}
        </button>
      </div>
    </ModalShell>
  );
}

// ══════════════════════════════════════════════════════════════════════════
// 패턴 규칙 편집 팝업 (템플릿, 상대/오프셋)
// ══════════════════════════════════════════════════════════════════════════

function PatternRuleEditDialog({
  initial,
  onSave,
  onClose,
}: {
  initial: PatternRuleRow;
  onSave: (rule: PatternRuleRow) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<PatternRuleRow>(initial);

  const patchTarget = (tgtKey: string, patch: Partial<PatternTargetRow>) =>
    setDraft((d) => ({
      ...d,
      targets: d.targets.map((tg) => (tg.key === tgtKey ? { ...tg, ...patch } : tg)),
    }));
  const addTarget = () =>
    setDraft((d) => ({ ...d, targets: [...d.targets, newPatternTargetRow()] }));
  const removeTarget = (tgtKey: string) =>
    setDraft((d) =>
      d.targets.length <= 1
        ? d
        : { ...d, targets: d.targets.filter((tg) => tg.key !== tgtKey) },
    );

  return (
    <ModalShell
      title={t('property.modbusRemap.patternDialogTitle')}
      onClose={onClose}
      closeLabel={t('property.modbusRemap.close')}
      footer={
        <>
          <button type="button" onClick={onClose} className={secondaryButton}>
            {t('property.modbusRemap.cancel')}
          </button>
          <button type="button" onClick={() => onSave(draft)} className={primaryButton}>
            {t('property.modbusRemap.save')}
          </button>
        </>
      }
    >
      <div className="space-y-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) p-2">
        <span className="text-[10px] font-semibold uppercase tracking-wide text-(--color-text-secondary)">
          {t('property.modbusRemap.from')}
        </span>
        <div className="grid grid-cols-[1fr_1.3fr_1fr_1fr] gap-2">
          <label className="space-y-0.5">
            <span className={fieldLabel}>{t('property.modbusRemap.sourceUnitId')}</span>
            <input
              type="number"
              min={0}
              max={247}
              value={draft.sourceUnitId}
              onChange={(e) => setDraft((d) => ({ ...d, sourceUnitId: e.target.value }))}
              aria-label={t('property.modbusRemap.sourceUnitId')}
              placeholder={t('property.modbusRemap.optional')}
              className={cellInput}
            />
          </label>
          <label className="space-y-0.5">
            <span className={fieldLabel}>{t('property.modbusRemap.sourceArea')}</span>
            <AreaSelect
              value={draft.sourceArea}
              onChange={(v) => setDraft((d) => ({ ...d, sourceArea: v }))}
              ariaLabel={t('property.modbusRemap.sourceArea')}
            />
          </label>
          <label className="space-y-0.5">
            <span className={fieldLabel}>{t('property.modbusRemap.sourceOffset')}</span>
            <input
              type="number"
              value={draft.sourceOffset}
              onChange={(e) =>
                setDraft((d) => ({ ...d, sourceOffset: numOr(e.target.value, 0) }))
              }
              aria-label={t('property.modbusRemap.sourceOffset')}
              className={cellInput}
            />
          </label>
          <label className="space-y-0.5">
            <span className={fieldLabel}>{t('property.modbusRemap.count')}</span>
            <input
              type="number"
              min={1}
              max={65535}
              value={draft.count}
              onChange={(e) => setDraft((d) => ({ ...d, count: numOr(e.target.value, 1) }))}
              aria-label={t('property.modbusRemap.count')}
              className={cellInput}
            />
          </label>
        </div>
      </div>

      <div className="space-y-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) p-2">
        <span className="text-[10px] font-semibold uppercase tracking-wide text-(--color-text-secondary)">
          {t('property.modbusRemap.to')}
        </span>
        <div className="grid grid-cols-[1.3fr_1fr_1fr_1.75rem] gap-2 px-0.5">
          <span className={fieldLabel}>{t('property.modbusRemap.targetArea')}</span>
          <span className={fieldLabel}>{t('property.modbusRemap.targetOffset')}</span>
          <span className={fieldLabel}>{t('property.modbusRemap.targetUnitOffset')}</span>
          <span />
        </div>
        {draft.targets.map((tg) => (
          <div key={tg.key} className="grid grid-cols-[1.3fr_1fr_1fr_1.75rem] items-center gap-2">
            <TargetAreaSelect
              value={tg.targetArea}
              onChange={(v) => patchTarget(tg.key, { targetArea: v })}
              ariaLabel={t('property.modbusRemap.targetArea')}
              t={t}
            />
            <input
              type="number"
              value={tg.targetOffset}
              onChange={(e) =>
                patchTarget(tg.key, { targetOffset: numOr(e.target.value, 0) })
              }
              aria-label={t('property.modbusRemap.targetOffset')}
              className={cellInput}
            />
            <input
              type="number"
              value={tg.targetUnitOffset}
              onChange={(e) => patchTarget(tg.key, { targetUnitOffset: e.target.value })}
              aria-label={t('property.modbusRemap.targetUnitOffset')}
              placeholder={t('property.modbusRemap.optional')}
              className={cellInput}
            />
            {draft.targets.length > 1 ? (
              <button
                type="button"
                onClick={() => removeTarget(tg.key)}
                className={removeButton}
                aria-label={t('property.modbusRemap.removeTarget')}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            ) : (
              <span />
            )}
          </div>
        ))}
        <button type="button" onClick={addTarget} className={addButton}>
          <Plus className="h-3.5 w-3.5" />
          {t('property.modbusRemap.addTarget')}
        </button>
      </div>
    </ModalShell>
  );
}

// ══════════════════════════════════════════════════════════════════════════
// 적용 다이얼로그 (start + device_id)
// ══════════════════════════════════════════════════════════════════════════

function ApplyDialog({
  onConfirm,
  onClose,
}: {
  onConfirm: (start: number, deviceId: number) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [start, setStart] = useState('0');
  const [deviceId, setDeviceId] = useState('1');

  return (
    <ModalShell
      title={t('property.modbusRemap.applyDialogTitle')}
      onClose={onClose}
      closeLabel={t('property.modbusRemap.close')}
      footer={
        <>
          <button type="button" onClick={onClose} className={secondaryButton}>
            {t('property.modbusRemap.cancel')}
          </button>
          <button
            type="button"
            onClick={() => onConfirm(numOr(start, 0), numOr(deviceId, 1))}
            className={primaryButton}
          >
            {t('property.modbusRemap.applyConfirm')}
          </button>
        </>
      }
    >
      <div className="grid grid-cols-2 gap-2">
        <label className="space-y-0.5">
          <span className={fieldLabel}>{t('property.modbusRemap.applyStart')}</span>
          <input
            type="number"
            min={0}
            max={65535}
            value={start}
            onChange={(e) => setStart(e.target.value)}
            aria-label={t('property.modbusRemap.applyStart')}
            className={cellInput}
          />
        </label>
        <label className="space-y-0.5">
          <span className={fieldLabel}>{t('property.modbusRemap.applyDeviceId')}</span>
          <input
            type="number"
            min={0}
            max={247}
            value={deviceId}
            onChange={(e) => setDeviceId(e.target.value)}
            aria-label={t('property.modbusRemap.applyDeviceId')}
            className={cellInput}
          />
        </label>
      </div>
    </ModalShell>
  );
}

// ══════════════════════════════════════════════════════════════════════════
// 메인 결합 편집기
// ══════════════════════════════════════════════════════════════════════════

interface RegisterRemapEditorProps {
  rulesValue: unknown;
  templatesValue: unknown;
  onChange: (value: { rules: unknown; templates: unknown }) => void;
  readOnly?: boolean;
}

type ActiveDialog =
  | null
  | { kind: 'rule'; ruleKey: string | null }
  | { kind: 'pattern'; tplKey: string; patKey: string | null }
  | { kind: 'apply'; tplKey: string };

export function RegisterRemapEditor({
  rulesValue,
  templatesValue,
  onChange,
  readOnly,
}: RegisterRemapEditorProps) {
  const { t } = useTranslation();
  const [rules, setRules] = useState<RuleRow[]>(() => toRuleRows(rulesValue));
  const [templates, setTemplates] = useState<TemplateDef[]>(() =>
    toTemplateDefs(templatesValue),
  );
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [dialog, setDialog] = useState<ActiveDialog>(null);
  const [bulkOpen, setBulkOpen] = useState(false);
  const [bulkText, setBulkText] = useState('');
  const [bulkErrors, setBulkErrors] = useState<BulkParseError[]>([]);
  const internalUpdate = useRef(false);
  const firstRun = useRef(true);

  useEffect(() => {
    if (firstRun.current) {
      firstRun.current = false;
      return;
    }
    if (internalUpdate.current) {
      internalUpdate.current = false;
      return;
    }
    setRules(toRuleRows(rulesValue));
    setTemplates(toTemplateDefs(templatesValue));
  }, [rulesValue, templatesValue]);

  const emit = useCallback(
    (nextRules: RuleRow[], nextTemplates: TemplateDef[]) => {
      setRules(nextRules);
      setTemplates(nextTemplates);
      internalUpdate.current = true;
      onChange({
        rules: nextRules.map(toEmitRule),
        templates: nextTemplates.map(toEmitTemplate),
      });
    },
    [onChange],
  );

  // --- rules ---
  const saveRule = (rule: RuleRow, ruleKey: string | null) => {
    if (ruleKey === null) emit([...rules, rule], templates);
    else emit(rules.map((r) => (r.key === ruleKey ? rule : r)), templates);
    setDialog(null);
  };
  const toggleSelect = (key: string, on: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (on) next.add(key);
      else next.delete(key);
      return next;
    });
  const toggleSelectAll = (on: boolean) =>
    setSelected(() => (on ? new Set(rules.map((r) => r.key)) : new Set()));
  const deleteSelected = () => {
    emit(rules.filter((r) => !selected.has(r.key)), templates);
    setSelected(new Set());
  };
  const selectedCount = rules.filter((r) => selected.has(r.key)).length;
  const allSelected = rules.length > 0 && selectedCount === rules.length;

  // --- bulk ---
  const closeBulk = () => {
    setBulkOpen(false);
    setBulkText('');
    setBulkErrors([]);
  };
  const applyBulk = () => {
    const result = parseBulkRules(bulkText);
    if (result.errors.length > 0) {
      setBulkErrors(result.errors);
      return;
    }
    if (result.rules.length === 0) {
      closeBulk();
      return;
    }
    emit([...rules, ...result.rules.map(toRuleRow)], templates);
    closeBulk();
  };

  // --- templates ---
  const addTemplate = () =>
    emit(rules, [...templates, newTemplateDef(t('property.modbusRemap.newTemplateName'))]);
  const renameTemplate = (tplKey: string, name: string) =>
    emit(rules, templates.map((tp) => (tp.key === tplKey ? { ...tp, name } : tp)));
  const removeTemplate = (tplKey: string) =>
    emit(rules, templates.filter((tp) => tp.key !== tplKey));
  const savePatternRule = (
    tplKey: string,
    patKey: string | null,
    pat: PatternRuleRow,
  ) => {
    emit(
      rules,
      templates.map((tp) =>
        tp.key !== tplKey
          ? tp
          : {
              ...tp,
              rules:
                patKey === null
                  ? [...tp.rules, pat]
                  : tp.rules.map((p) => (p.key === patKey ? pat : p)),
            },
      ),
    );
    setDialog(null);
  };
  const removePatternRule = (tplKey: string, patKey: string) =>
    emit(
      rules,
      templates.map((tp) =>
        tp.key !== tplKey
          ? tp
          : { ...tp, rules: tp.rules.filter((p) => p.key !== patKey) },
      ),
    );
  const applyTemplate = (tplKey: string, start: number, deviceId: number) => {
    const tpl = templates.find((tp) => tp.key === tplKey);
    if (!tpl) return;
    const concrete = materializeTemplate(toEmitTemplate(tpl), start, deviceId);
    emit([...rules, ...concrete.map(toRuleRow)], templates);
    setDialog(null);
  };

  // --- 다이얼로그 초기값 계산 ---
  const currentRule = (): RuleRow => {
    if (dialog?.kind === 'rule' && dialog.ruleKey !== null) {
      return rules.find((r) => r.key === dialog.ruleKey) ?? newRuleRow();
    }
    return newRuleRow();
  };
  const currentPattern = (): PatternRuleRow => {
    if (dialog?.kind === 'pattern' && dialog.patKey !== null) {
      const tpl = templates.find((tp) => tp.key === dialog.tplKey);
      return tpl?.rules.find((p) => p.key === dialog.patKey) ?? newPatternRuleRow();
    }
    return newPatternRuleRow();
  };

  const targetsSummary = (r: RuleRow): string =>
    r.targets
      .map(
        (tg) =>
          `u${tg.targetUnitId}·${abbrev(tg.targetArea || r.sourceArea)}${tg.targetAddress}`,
      )
      .join(', ');
  const patternSummary = (p: PatternRuleRow): string =>
    `${abbrev(p.sourceArea)}+${p.sourceOffset} ×${p.count} → ` +
    p.targets
      .map((tg) => `${abbrev(tg.targetArea || p.sourceArea)}+${tg.targetOffset}`)
      .join(', ');

  return (
    <div className="space-y-4">
      {/* ===== Rules ===== */}
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            {!readOnly && rules.length > 0 && (
              <input
                type="checkbox"
                checked={allSelected}
                onChange={(e) => toggleSelectAll(e.target.checked)}
                aria-label={t('property.modbusRemap.selectAll')}
                className="h-3.5 w-3.5"
              />
            )}
            <span className={fieldLabel}>{t('property.modbusRemap.rules')}</span>
          </div>
          {!readOnly && selectedCount > 0 && (
            <button
              type="button"
              onClick={deleteSelected}
              className="inline-flex items-center gap-1 rounded px-2 py-1 text-[11px] font-medium text-red-500 transition-colors hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
            >
              <Trash2 className="h-3.5 w-3.5" />
              {t('property.modbusRemap.deleteSelected')} ({selectedCount})
            </button>
          )}
        </div>

        {rules.length === 0 ? (
          <p className="py-2 text-center text-xs text-(--color-text-muted)">
            {t('property.modbusRemap.rulesEmpty')}
          </p>
        ) : (
          <div className="space-y-1">
            {rules.map((r, idx) => (
              <div
                key={r.key}
                className="flex items-center gap-2 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1.5"
              >
                {!readOnly && (
                  <input
                    type="checkbox"
                    checked={selected.has(r.key)}
                    onChange={(e) => toggleSelect(r.key, e.target.checked)}
                    aria-label={t('property.modbusRemap.selectRow')}
                    className="h-3.5 w-3.5"
                  />
                )}
                <span className="w-5 shrink-0 text-[11px] text-(--color-text-muted)">
                  #{idx + 1}
                </span>
                <span className="min-w-0 flex-1 truncate font-mono text-xs text-(--color-text-primary)">
                  {r.sourceUnitId.trim() !== '' ? `u${r.sourceUnitId} ` : ''}
                  {abbrev(r.sourceArea)}
                  {r.sourceAddress} ×{r.count}
                  <span className="text-(--color-text-muted)"> → {targetsSummary(r)}</span>
                </span>
                {!readOnly && (
                  <button
                    type="button"
                    onClick={() => setDialog({ kind: 'rule', ruleKey: r.key })}
                    className={iconButton}
                    aria-label={t('property.modbusRemap.editRule')}
                  >
                    <Pencil className="h-3.5 w-3.5" />
                  </button>
                )}
              </div>
            ))}
          </div>
        )}

        {!readOnly && (
          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              onClick={() => setDialog({ kind: 'rule', ruleKey: null })}
              className={addButton}
            >
              <Plus className="h-3.5 w-3.5" />
              {t('property.modbusRemap.addRule')}
            </button>
            <button
              type="button"
              onClick={() => (bulkOpen ? closeBulk() : setBulkOpen(true))}
              className={addButton}
            >
              <ClipboardPaste className="h-3.5 w-3.5" />
              {t('property.modbusRemap.bulkRegister')}
            </button>
          </div>
        )}

        {/* 일괄등록 패널 */}
        {!readOnly && bulkOpen && (
          <div className="space-y-2 rounded border border-(--color-border-default) bg-(--color-bg-elevated) p-2">
            <p className="whitespace-pre-line text-[11px] text-(--color-text-muted)">
              {t('property.modbusRemap.bulkHelp')}
            </p>
            <textarea
              value={bulkText}
              onChange={(e) => setBulkText(e.target.value)}
              rows={5}
              aria-label={t('property.modbusRemap.bulkRegister')}
              placeholder={t('property.modbusRemap.bulkPlaceholder')}
              className={cn(cellInput, 'font-mono')}
            />
            {bulkErrors.length > 0 && (
              <ul className="space-y-0.5">
                {bulkErrors.map((er) => (
                  <li key={er.line} className="text-[11px] text-red-500 dark:text-red-400">
                    {t('property.modbusRemap.bulkLinePrefix')} {er.line}:{' '}
                    {t(`property.modbusRemap.bulkError.${er.code}`)}
                  </li>
                ))}
              </ul>
            )}
            <div className="flex justify-end gap-2">
              <button type="button" onClick={closeBulk} className={secondaryButton}>
                {t('property.modbusRemap.cancel')}
              </button>
              <button type="button" onClick={applyBulk} className={primaryButton}>
                {t('property.modbusRemap.bulkApply')}
              </button>
            </div>
          </div>
        )}
      </div>

      {/* ===== Templates ===== */}
      <div className="space-y-2 border-t border-(--color-border-default) pt-3">
        <span className={fieldLabel}>{t('property.modbusRemap.templates')}</span>

        {templates.length === 0 && (
          <p className="py-2 text-center text-xs text-(--color-text-muted)">
            {t('property.modbusRemap.templatesEmpty')}
          </p>
        )}

        {templates.map((tpl) => (
          <div
            key={tpl.key}
            className="space-y-2 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-2"
          >
            <div className="flex items-center gap-2">
              <input
                type="text"
                value={tpl.name}
                readOnly={readOnly}
                onChange={(e) => renameTemplate(tpl.key, e.target.value)}
                aria-label={t('property.modbusRemap.templateName')}
                placeholder={t('property.modbusRemap.templateName')}
                className={cn(cellInput, 'flex-1', readOnly && readOnlyInput)}
              />
              {!readOnly && (
                <>
                  <button
                    type="button"
                    onClick={() => setDialog({ kind: 'apply', tplKey: tpl.key })}
                    className="inline-flex items-center gap-1 rounded-md bg-blue-600 px-2 py-1 text-[11px] font-medium text-white hover:bg-blue-700"
                    aria-label={t('property.modbusRemap.apply')}
                  >
                    <Play className="h-3 w-3" />
                    {t('property.modbusRemap.apply')}
                  </button>
                  <button
                    type="button"
                    onClick={() => removeTemplate(tpl.key)}
                    className={removeButton}
                    aria-label={t('property.modbusRemap.removeTemplate')}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                </>
              )}
            </div>

            {/* 패턴 규칙 목록 */}
            <div className="space-y-1 pl-1">
              {tpl.rules.length === 0 && (
                <p className="text-[11px] text-(--color-text-muted)">
                  {t('property.modbusRemap.patternRulesEmpty')}
                </p>
              )}
              {tpl.rules.map((p) => (
                <div key={p.key} className="flex items-center gap-2">
                  <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-(--color-text-secondary)">
                    {patternSummary(p)}
                  </span>
                  {!readOnly && (
                    <>
                      <button
                        type="button"
                        onClick={() =>
                          setDialog({ kind: 'pattern', tplKey: tpl.key, patKey: p.key })
                        }
                        className={iconButton}
                        aria-label={t('property.modbusRemap.editRule')}
                      >
                        <Pencil className="h-3 w-3" />
                      </button>
                      <button
                        type="button"
                        onClick={() => removePatternRule(tpl.key, p.key)}
                        className={removeButton}
                        aria-label={t('property.modbusRemap.removeTarget')}
                      >
                        <Trash2 className="h-3 w-3" />
                      </button>
                    </>
                  )}
                </div>
              ))}
              {!readOnly && (
                <button
                  type="button"
                  onClick={() =>
                    setDialog({ kind: 'pattern', tplKey: tpl.key, patKey: null })
                  }
                  className={addButton}
                >
                  <Plus className="h-3.5 w-3.5" />
                  {t('property.modbusRemap.addPatternRule')}
                </button>
              )}
            </div>
          </div>
        ))}

        {!readOnly && (
          <button type="button" onClick={addTemplate} className={addButton}>
            <Plus className="h-3.5 w-3.5" />
            {t('property.modbusRemap.addTemplate')}
          </button>
        )}
      </div>

      {/* ===== 다이얼로그 ===== */}
      {dialog?.kind === 'rule' && (
        <RuleEditDialog
          initial={currentRule()}
          onSave={(rule) => saveRule(rule, dialog.ruleKey)}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.kind === 'pattern' && (
        <PatternRuleEditDialog
          initial={currentPattern()}
          onSave={(pat) => savePatternRule(dialog.tplKey, dialog.patKey, pat)}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.kind === 'apply' && (
        <ApplyDialog
          onConfirm={(start, deviceId) => applyTemplate(dialog.tplKey, start, deviceId)}
          onClose={() => setDialog(null)}
        />
      )}
    </div>
  );
}
