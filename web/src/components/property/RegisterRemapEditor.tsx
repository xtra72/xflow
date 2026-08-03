// MODBUS Register Remapper(modbus-remap) 노드 편집기 — SPEC-MODBUS-007.
//
// 이 노드는 에이전트와 통신하지 않고, modbus-read 출력 payload({success, values[], ...})의
// 레지스터를 재매핑(remap)해 modbus-write 호환 payload 로 변환한다. config 에는 두 개의
// op-list 가 있다: rules(From→To, 1 From → N To 팬아웃)와 templates(정의+적용 축약형).
//
// 백엔드 형상(internal/node/modbus_remap.go, 정확히 일치):
//   RuleRow(rules[]):
//     { source_unit_id?, source_area, source_address, count,
//       targets: [ { target_unit_id, target_area?, target_address } ]  (최소 1개) }
//     - source_unit_id OPTIONAL(From unit_id, 비우면 생략 — 유닛 제약 없음)
//     - targets: 1..N 개(팬아웃). 각 target 의 target_area OPTIONAL(생략 시 source_area 유지)
//     - legacy: 최상위 단일 target_*(target_unit_id/target_area/target_address, targets 없음)
//       입력은 로드 시 1-원소 targets 로 정규화하고, 방출은 항상 targets 배열로 한다.
//   TemplateRow(templates[]):
//     { source_unit_id?, area, offset, device_id, start, count, target_area? }
//     - target_address = start + offset(음수 허용), device_id → target_unit_id,
//       start → source_address, count → count, target_area OPTIONAL(생략 시 area 유지)
//
// UI: rules 는 규칙마다 카드 하나 — 상단 From(원본) 블록 + 하단 To(대상) 다중 타깃 테이블.
// templates 는 별도 한 줄 행 테이블. 두 필드(rules/templates)는 각각 자신의 배열을 방출하며
// config.rules / config.templates 로 직결된다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { Plus, Trash2 } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';

// ---- 상수 ----

const AREA_OPTIONS = [
  'coils',
  'discrete_inputs',
  'holding_registers',
  'input_registers',
] as const;

const DEFAULT_AREA = 'holding_registers';

// ---- 내부 행 타입 ----

interface TargetRow {
  key: string;
  targetUnitId: number;
  /** '' = source_area 유지(방출에서 생략). */
  targetArea: string;
  targetAddress: number;
}

interface RuleRow {
  key: string;
  /** '' = source_unit_id 생략(유닛 제약 없음). */
  sourceUnitId: string;
  sourceArea: string;
  sourceAddress: number;
  count: number;
  targets: TargetRow[];
}

interface TemplateRow {
  key: string;
  /** '' = source_unit_id 생략. */
  sourceUnitId: string;
  area: string;
  offset: number;
  deviceId: number;
  start: number;
  count: number;
  /** '' = area 유지(방출에서 생략). */
  targetArea: string;
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

interface EmittedTemplate {
  source_unit_id?: number;
  area: string;
  offset: number;
  device_id: number;
  start: number;
  count: number;
  target_area?: string;
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

/** number 이면 문자열화(0 포함), 없으면 빈 문자열(선택 필드 표시용). */
function optNumToText(v: unknown): string {
  return typeof v === 'number' && Number.isFinite(v) ? String(v) : '';
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

// --- rules ---

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
    // legacy 단일 target_* → 1-원소 targets 로 정규화.
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

// --- templates ---

function toTemplateRow(item: unknown): TemplateRow {
  const o = asObject(item);
  return {
    key: nextKey('tpl'),
    sourceUnitId: optNumToText(o.source_unit_id),
    area: asString(o.area) || DEFAULT_AREA,
    offset: numOr(o.offset, 0),
    deviceId: numOr(o.device_id, 1),
    start: numOr(o.start, 0),
    count: numOr(o.count, 1),
    targetArea: asString(o.target_area),
  };
}

function toTemplateRows(value: unknown): TemplateRow[] {
  return toArray(value).map(toTemplateRow);
}

function newTemplateRow(): TemplateRow {
  return {
    key: nextKey('tpl'),
    sourceUnitId: '',
    area: DEFAULT_AREA,
    offset: 0,
    deviceId: 1,
    start: 0,
    count: 1,
    targetArea: '',
  };
}

function toEmitTemplate(r: TemplateRow): EmittedTemplate {
  const out: EmittedTemplate = {
    area: r.area,
    offset: r.offset,
    device_id: r.deviceId,
    start: r.start,
    count: r.count,
  };
  if (r.sourceUnitId.trim() !== '') out.source_unit_id = numOr(r.sourceUnitId, 0);
  if (r.targetArea.trim() !== '') out.target_area = r.targetArea;
  return out;
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

const removeButton = cn(
  'shrink-0 rounded p-1 text-gray-400 transition-colors',
  'hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400',
);

function DeleteSelectedButton({
  count,
  onClick,
  t,
}: {
  count: number;
  onClick: () => void;
  t: (k: string) => string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="inline-flex items-center gap-1 rounded px-2 py-1 text-[11px] font-medium text-red-500 transition-colors hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
    >
      <Trash2 className="h-3.5 w-3.5" />
      {t('property.modbusRemap.deleteSelected')} ({count})
    </button>
  );
}

/** '(유지)' + 4개 영역을 렌더하는 target_area select. */
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

// ══════════════════════════════════════════════════════════════════════════
// Rules 편집기 (규칙 = 카드: 상단 From, 하단 다중 To)
// ══════════════════════════════════════════════════════════════════════════

interface RemapEditorProps {
  value: unknown;
  onChange: (value: unknown) => void;
  readOnly?: boolean;
}

export function RemapRulesEditor({ value, onChange, readOnly }: RemapEditorProps) {
  const { t } = useTranslation();
  const [rows, setRows] = useState<RuleRow[]>(() => toRuleRows(value));
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
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
    setRows(toRuleRows(value));
  }, [value]);

  const emit = useCallback(
    (next: RuleRow[]) => {
      setRows(next);
      internalUpdate.current = true;
      onChange(next.map(toEmitRule));
    },
    [onChange],
  );

  const addRule = () => emit([...rows, newRuleRow()]);
  const patchRule = (key: string, patch: Partial<RuleRow>) =>
    emit(rows.map((r) => (r.key === key ? { ...r, ...patch } : r)));

  const patchTarget = (ruleKey: string, tgtKey: string, patch: Partial<TargetRow>) => {
    const rule = rows.find((r) => r.key === ruleKey);
    if (!rule) return;
    patchRule(ruleKey, {
      targets: rule.targets.map((tg) => (tg.key === tgtKey ? { ...tg, ...patch } : tg)),
    });
  };
  const addTarget = (ruleKey: string) => {
    const rule = rows.find((r) => r.key === ruleKey);
    if (!rule) return;
    patchRule(ruleKey, { targets: [...rule.targets, newTargetRow()] });
  };
  const removeTarget = (ruleKey: string, tgtKey: string) => {
    const rule = rows.find((r) => r.key === ruleKey);
    if (!rule || rule.targets.length <= 1) return; // 최소 1개 유지
    patchRule(ruleKey, { targets: rule.targets.filter((tg) => tg.key !== tgtKey) });
  };

  const toggleSelect = (key: string, on: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (on) next.add(key);
      else next.delete(key);
      return next;
    });
  const toggleSelectAll = (on: boolean) =>
    setSelected(() => (on ? new Set(rows.map((r) => r.key)) : new Set()));
  const deleteSelected = () => {
    emit(rows.filter((r) => !selected.has(r.key)));
    setSelected(new Set());
  };

  const selectedCount = rows.filter((r) => selected.has(r.key)).length;
  const allSelected = rows.length > 0 && selectedCount === rows.length;

  // From 서브그리드: 소스유닛ID | 소스영역 | 소스주소 | 개수
  const fromGrid = 'grid-cols-[1fr_1.3fr_1fr_1fr]';
  // To 타깃 행: 유닛ID | 대상영역 | 대상주소 | (삭제)
  const toGrid = 'grid-cols-[1fr_1.3fr_1fr_1.75rem]';

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          {!readOnly && rows.length > 0 && (
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
          <DeleteSelectedButton count={selectedCount} onClick={deleteSelected} t={t} />
        )}
      </div>

      {rows.length === 0 ? (
        <p className="py-2 text-center text-xs text-(--color-text-muted)">
          {t('property.modbusRemap.rulesEmpty')}
        </p>
      ) : (
        <div className="space-y-2">
          {rows.map((rule, idx) => (
            <div
              key={rule.key}
              className="space-y-2 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-2"
            >
              {/* 규칙 헤더 (선택 체크박스 + 라벨) */}
              <div className="flex items-center gap-2">
                {!readOnly && (
                  <input
                    type="checkbox"
                    checked={selected.has(rule.key)}
                    onChange={(e) => toggleSelect(rule.key, e.target.checked)}
                    aria-label={t('property.modbusRemap.selectRow')}
                    className="h-3.5 w-3.5"
                  />
                )}
                <span className="text-xs font-semibold text-(--color-text-secondary)">
                  {t('property.modbusRemap.rule')} {idx + 1}
                </span>
              </div>

              {/* From (원본) */}
              <div className="space-y-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) p-2">
                <span className="text-[10px] font-semibold uppercase tracking-wide text-(--color-text-secondary)">
                  {t('property.modbusRemap.from')}
                </span>
                <div className={cn('grid gap-2', fromGrid)}>
                  <label className="space-y-0.5">
                    <span className={fieldLabel}>
                      {t('property.modbusRemap.sourceUnitId')}
                    </span>
                    <input
                      type="number"
                      min={0}
                      max={247}
                      value={rule.sourceUnitId}
                      readOnly={readOnly}
                      onChange={(e) =>
                        patchRule(rule.key, { sourceUnitId: e.target.value })
                      }
                      aria-label={t('property.modbusRemap.sourceUnitId')}
                      placeholder={t('property.modbusRemap.optional')}
                      className={cn(cellInput, readOnly && readOnlyInput)}
                    />
                  </label>
                  <label className="space-y-0.5">
                    <span className={fieldLabel}>
                      {t('property.modbusRemap.sourceArea')}
                    </span>
                    <select
                      value={rule.sourceArea}
                      disabled={readOnly}
                      onChange={(e) => patchRule(rule.key, { sourceArea: e.target.value })}
                      aria-label={t('property.modbusRemap.sourceArea')}
                      className={cn(cellInput, readOnly && readOnlyInput)}
                    >
                      {AREA_OPTIONS.map((a) => (
                        <option key={a} value={a}>
                          {a}
                        </option>
                      ))}
                    </select>
                  </label>
                  <label className="space-y-0.5">
                    <span className={fieldLabel}>
                      {t('property.modbusRemap.sourceAddress')}
                    </span>
                    <input
                      type="number"
                      min={0}
                      max={65535}
                      value={rule.sourceAddress}
                      readOnly={readOnly}
                      onChange={(e) =>
                        patchRule(rule.key, { sourceAddress: numOr(e.target.value, 0) })
                      }
                      aria-label={t('property.modbusRemap.sourceAddress')}
                      className={cn(cellInput, readOnly && readOnlyInput)}
                    />
                  </label>
                  <label className="space-y-0.5">
                    <span className={fieldLabel}>{t('property.modbusRemap.count')}</span>
                    <input
                      type="number"
                      min={1}
                      max={65535}
                      value={rule.count}
                      readOnly={readOnly}
                      onChange={(e) =>
                        patchRule(rule.key, { count: numOr(e.target.value, 1) })
                      }
                      aria-label={t('property.modbusRemap.count')}
                      className={cn(cellInput, readOnly && readOnlyInput)}
                    />
                  </label>
                </div>
                <p className="text-[10px] text-(--color-text-muted)">
                  {t('property.modbusRemap.sourceUnitIdHint')}
                </p>
              </div>

              {/* To (대상) — 다중 타깃 */}
              <div className="space-y-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) p-2">
                <span className="text-[10px] font-semibold uppercase tracking-wide text-(--color-text-secondary)">
                  {t('property.modbusRemap.to')}
                </span>
                {/* 타깃 헤더 */}
                <div className={cn('grid gap-2 px-0.5', toGrid)}>
                  <span className={fieldLabel}>
                    {t('property.modbusRemap.targetUnitId')}
                  </span>
                  <span className={fieldLabel}>{t('property.modbusRemap.targetArea')}</span>
                  <span className={fieldLabel}>
                    {t('property.modbusRemap.targetAddress')}
                  </span>
                  <span />
                </div>
                {rule.targets.map((tg) => (
                  <div key={tg.key} className={cn('grid items-center gap-2', toGrid)}>
                    <input
                      type="number"
                      min={0}
                      max={247}
                      value={tg.targetUnitId}
                      readOnly={readOnly}
                      onChange={(e) =>
                        patchTarget(rule.key, tg.key, {
                          targetUnitId: numOr(e.target.value, 1),
                        })
                      }
                      aria-label={t('property.modbusRemap.targetUnitId')}
                      className={cn(cellInput, readOnly && readOnlyInput)}
                    />
                    <TargetAreaSelect
                      value={tg.targetArea}
                      disabled={readOnly}
                      onChange={(v) => patchTarget(rule.key, tg.key, { targetArea: v })}
                      ariaLabel={t('property.modbusRemap.targetArea')}
                      t={t}
                    />
                    <input
                      type="number"
                      min={0}
                      max={65535}
                      value={tg.targetAddress}
                      readOnly={readOnly}
                      onChange={(e) =>
                        patchTarget(rule.key, tg.key, {
                          targetAddress: numOr(e.target.value, 0),
                        })
                      }
                      aria-label={t('property.modbusRemap.targetAddress')}
                      className={cn(cellInput, readOnly && readOnlyInput)}
                    />
                    {!readOnly && rule.targets.length > 1 ? (
                      <button
                        type="button"
                        onClick={() => removeTarget(rule.key, tg.key)}
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
                {!readOnly && (
                  <button
                    type="button"
                    onClick={() => addTarget(rule.key)}
                    className={addButton}
                  >
                    <Plus className="h-3.5 w-3.5" />
                    {t('property.modbusRemap.addTarget')}
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {!readOnly && (
        <button type="button" onClick={addRule} className={addButton}>
          <Plus className="h-3.5 w-3.5" />
          {t('property.modbusRemap.addRule')}
        </button>
      )}
    </div>
  );
}

// ══════════════════════════════════════════════════════════════════════════
// Templates 편집기 (한 줄 행 테이블, 단일 타깃)
// ══════════════════════════════════════════════════════════════════════════

export function RemapTemplatesEditor({ value, onChange, readOnly }: RemapEditorProps) {
  const { t } = useTranslation();
  const [rows, setRows] = useState<TemplateRow[]>(() => toTemplateRows(value));
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
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
    setRows(toTemplateRows(value));
  }, [value]);

  const emit = useCallback(
    (next: TemplateRow[]) => {
      setRows(next);
      internalUpdate.current = true;
      onChange(next.map(toEmitTemplate));
    },
    [onChange],
  );

  const addRow = () => emit([...rows, newTemplateRow()]);
  const patchRow = (key: string, patch: Partial<TemplateRow>) =>
    emit(rows.map((r) => (r.key === key ? { ...r, ...patch } : r)));

  const toggleSelect = (key: string, on: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (on) next.add(key);
      else next.delete(key);
      return next;
    });
  const toggleSelectAll = (on: boolean) =>
    setSelected(() => (on ? new Set(rows.map((r) => r.key)) : new Set()));
  const deleteSelected = () => {
    emit(rows.filter((r) => !selected.has(r.key)));
    setSelected(new Set());
  };

  const selectedCount = rows.filter((r) => selected.has(r.key)).length;
  const allSelected = rows.length > 0 && selectedCount === rows.length;

  // 컬럼: 선택 | source_unit_id | area | offset | device_id | start | count | target_area
  const gridCols = 'grid-cols-[1.75rem_1fr_1.3fr_0.9fr_0.9fr_0.9fr_0.9fr_1.3fr]';

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className={fieldLabel}>{t('property.modbusRemap.templates')}</span>
        {!readOnly && selectedCount > 0 && (
          <DeleteSelectedButton count={selectedCount} onClick={deleteSelected} t={t} />
        )}
      </div>

      {rows.length === 0 ? (
        <p className="py-2 text-center text-xs text-(--color-text-muted)">
          {t('property.modbusRemap.templatesEmpty')}
        </p>
      ) : (
        <div className="space-y-1">
          <div className={cn('grid items-center gap-2 px-1', gridCols)}>
            <div className="flex justify-center">
              {!readOnly && (
                <input
                  type="checkbox"
                  checked={allSelected}
                  onChange={(e) => toggleSelectAll(e.target.checked)}
                  aria-label={t('property.modbusRemap.selectAll')}
                  className="h-3.5 w-3.5"
                />
              )}
            </div>
            <span className={fieldLabel}>{t('property.modbusRemap.sourceUnitId')}</span>
            <span className={fieldLabel}>{t('property.modbusRemap.area')}</span>
            <span className={fieldLabel}>{t('property.modbusRemap.offset')}</span>
            <span className={fieldLabel}>{t('property.modbusRemap.deviceId')}</span>
            <span className={fieldLabel}>{t('property.modbusRemap.start')}</span>
            <span className={fieldLabel}>{t('property.modbusRemap.count')}</span>
            <span className={fieldLabel}>{t('property.modbusRemap.targetArea')}</span>
          </div>

          {rows.map((row) => (
            <div
              key={row.key}
              className={cn(
                'grid items-center gap-2 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-1 py-1',
                gridCols,
              )}
            >
              <div className="flex justify-center">
                {!readOnly && (
                  <input
                    type="checkbox"
                    checked={selected.has(row.key)}
                    onChange={(e) => toggleSelect(row.key, e.target.checked)}
                    aria-label={t('property.modbusRemap.selectRow')}
                    className="h-3.5 w-3.5"
                  />
                )}
              </div>

              <input
                type="number"
                min={0}
                max={247}
                value={row.sourceUnitId}
                readOnly={readOnly}
                onChange={(e) => patchRow(row.key, { sourceUnitId: e.target.value })}
                aria-label={t('property.modbusRemap.sourceUnitId')}
                placeholder={t('property.modbusRemap.optional')}
                className={cn(cellInput, readOnly && readOnlyInput)}
              />
              <select
                value={row.area}
                disabled={readOnly}
                onChange={(e) => patchRow(row.key, { area: e.target.value })}
                aria-label={t('property.modbusRemap.area')}
                className={cn(cellInput, readOnly && readOnlyInput)}
              >
                {AREA_OPTIONS.map((a) => (
                  <option key={a} value={a}>
                    {a}
                  </option>
                ))}
              </select>
              <input
                type="number"
                value={row.offset}
                readOnly={readOnly}
                onChange={(e) => patchRow(row.key, { offset: numOr(e.target.value, 0) })}
                aria-label={t('property.modbusRemap.offset')}
                className={cn(cellInput, readOnly && readOnlyInput)}
              />
              <input
                type="number"
                min={0}
                max={247}
                value={row.deviceId}
                readOnly={readOnly}
                onChange={(e) => patchRow(row.key, { deviceId: numOr(e.target.value, 1) })}
                aria-label={t('property.modbusRemap.deviceId')}
                className={cn(cellInput, readOnly && readOnlyInput)}
              />
              <input
                type="number"
                min={0}
                max={65535}
                value={row.start}
                readOnly={readOnly}
                onChange={(e) => patchRow(row.key, { start: numOr(e.target.value, 0) })}
                aria-label={t('property.modbusRemap.start')}
                className={cn(cellInput, readOnly && readOnlyInput)}
              />
              <input
                type="number"
                min={1}
                max={65535}
                value={row.count}
                readOnly={readOnly}
                onChange={(e) => patchRow(row.key, { count: numOr(e.target.value, 1) })}
                aria-label={t('property.modbusRemap.count')}
                className={cn(cellInput, readOnly && readOnlyInput)}
              />
              <TargetAreaSelect
                value={row.targetArea}
                disabled={readOnly}
                onChange={(v) => patchRow(row.key, { targetArea: v })}
                ariaLabel={t('property.modbusRemap.targetArea')}
                t={t}
              />
            </div>
          ))}
        </div>
      )}

      {!readOnly && (
        <button type="button" onClick={addRow} className={addButton}>
          <Plus className="h-3.5 w-3.5" />
          {t('property.modbusRemap.addTemplate')}
        </button>
      )}
    </div>
  );
}
