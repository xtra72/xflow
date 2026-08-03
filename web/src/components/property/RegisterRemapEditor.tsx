// MODBUS Register Remapper(modbus-remap) 노드 편집기 — SPEC-MODBUS-007.
//
// 이 노드는 에이전트와 통신하지 않고, modbus-read 출력 payload({success, values[], ...})의
// 레지스터를 재매핑(remap)해 modbus-write 호환 payload 로 변환한다. config 에는 두 개의
// op-list 가 있다: rules(From→To 개별 규칙)와 templates(정의+적용 축약형).
//
// 백엔드 형상(internal/node/modbus_remap.go, 정확히 일치):
//   RuleRow(rules[]):
//     { source_area, source_address, count, target_unit_id, target_area?, target_address }
//     - source_area: From 영역, source_address: From 시작, count: 읽기 count 와 정확히 일치
//     - target_unit_id: To device_id/unit_id, target_address: To 시작
//     - target_area OPTIONAL — 생략 시 source_area 유지
//   TemplateRow(templates[]):
//     { area, offset, device_id, start, count, target_area? }
//     - target_address = start + offset (offset 음수 허용)
//     - device_id → target_unit_id, start → source_address, count → count
//     - target_area OPTIONAL — 생략 시 area 유지
//
// UI: rules 는 좌(From)/우(To) 두 테이블을 나란히 둔 행 페어링 형식(요청: "두개의 테이블을
// 양쪽에 두고 From To"), templates 는 별도 한 줄 행 테이블. 두 필드(rules/templates)는
// 각각 자신의 배열을 방출하며 config.rules / config.templates 로 직결된다.

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

interface RuleRow {
  key: string;
  sourceArea: string;
  sourceAddress: number;
  count: number;
  targetUnitId: number;
  /** '' = source_area 유지(방출에서 생략). */
  targetArea: string;
  targetAddress: number;
}

interface TemplateRow {
  key: string;
  area: string;
  offset: number;
  deviceId: number;
  start: number;
  count: number;
  /** '' = area 유지(방출에서 생략). */
  targetArea: string;
}

// ---- 방출 타입 ----

interface EmittedRule {
  source_area: string;
  source_address: number;
  count: number;
  target_unit_id: number;
  target_area?: string;
  target_address: number;
}

interface EmittedTemplate {
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

function toRuleRow(item: unknown): RuleRow {
  const o = asObject(item);
  return {
    key: nextKey('rule'),
    sourceArea: asString(o.source_area) || DEFAULT_AREA,
    sourceAddress: numOr(o.source_address, 0),
    count: numOr(o.count, 1),
    targetUnitId: numOr(o.target_unit_id, 1),
    targetArea: asString(o.target_area),
    targetAddress: numOr(o.target_address, 0),
  };
}

function toRuleRows(value: unknown): RuleRow[] {
  return toArray(value).map(toRuleRow);
}

function newRuleRow(): RuleRow {
  return {
    key: nextKey('rule'),
    sourceArea: DEFAULT_AREA,
    sourceAddress: 0,
    count: 1,
    targetUnitId: 1,
    targetArea: '',
    targetAddress: 0,
  };
}

function toEmitRule(r: RuleRow): EmittedRule {
  const out: EmittedRule = {
    source_area: r.sourceArea,
    source_address: r.sourceAddress,
    count: r.count,
    target_unit_id: r.targetUnitId,
    target_address: r.targetAddress,
  };
  if (r.targetArea.trim() !== '') out.target_area = r.targetArea;
  return out;
}

// --- templates ---

function toTemplateRow(item: unknown): TemplateRow {
  const o = asObject(item);
  return {
    key: nextKey('tpl'),
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
// Rules 편집기 (좌 From / 우 To 두 테이블 페어링)
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

  const addRow = () => emit([...rows, newRuleRow()]);
  const patchRow = (key: string, patch: Partial<RuleRow>) =>
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

  // 컬럼: 선택 | From(영역 주소 개수) | To(유닛ID 영역 주소)
  const gridCols =
    'grid-cols-[1.75rem_1.3fr_0.9fr_0.9fr_0.9fr_1.3fr_0.9fr]';

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className={fieldLabel}>{t('property.modbusRemap.rules')}</span>
        {!readOnly && selectedCount > 0 && (
          <DeleteSelectedButton count={selectedCount} onClick={deleteSelected} t={t} />
        )}
      </div>

      {rows.length === 0 ? (
        <p className="py-2 text-center text-xs text-(--color-text-muted)">
          {t('property.modbusRemap.rulesEmpty')}
        </p>
      ) : (
        <div className="space-y-1">
          {/* From / To 밴드 */}
          <div className={cn('grid gap-2 px-1', gridCols)}>
            <div />
            <span className="col-span-3 rounded bg-(--color-bg-elevated) px-1 text-center text-[10px] font-semibold uppercase tracking-wide text-(--color-text-secondary)">
              {t('property.modbusRemap.from')}
            </span>
            <span className="col-span-3 rounded bg-(--color-bg-elevated) px-1 text-center text-[10px] font-semibold uppercase tracking-wide text-(--color-text-secondary)">
              {t('property.modbusRemap.to')}
            </span>
          </div>

          {/* 컬럼 헤더 */}
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
            <span className={fieldLabel}>{t('property.modbusRemap.sourceArea')}</span>
            <span className={fieldLabel}>{t('property.modbusRemap.sourceAddress')}</span>
            <span className={fieldLabel}>{t('property.modbusRemap.count')}</span>
            <span className={fieldLabel}>{t('property.modbusRemap.targetUnitId')}</span>
            <span className={fieldLabel}>{t('property.modbusRemap.targetArea')}</span>
            <span className={fieldLabel}>{t('property.modbusRemap.targetAddress')}</span>
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

              {/* From */}
              <select
                value={row.sourceArea}
                disabled={readOnly}
                onChange={(e) => patchRow(row.key, { sourceArea: e.target.value })}
                aria-label={t('property.modbusRemap.sourceArea')}
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
                min={0}
                max={65535}
                value={row.sourceAddress}
                readOnly={readOnly}
                onChange={(e) =>
                  patchRow(row.key, { sourceAddress: numOr(e.target.value, 0) })
                }
                aria-label={t('property.modbusRemap.sourceAddress')}
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

              {/* To */}
              <input
                type="number"
                min={0}
                max={247}
                value={row.targetUnitId}
                readOnly={readOnly}
                onChange={(e) =>
                  patchRow(row.key, { targetUnitId: numOr(e.target.value, 1) })
                }
                aria-label={t('property.modbusRemap.targetUnitId')}
                className={cn(cellInput, readOnly && readOnlyInput)}
              />
              <TargetAreaSelect
                value={row.targetArea}
                disabled={readOnly}
                onChange={(v) => patchRow(row.key, { targetArea: v })}
                ariaLabel={t('property.modbusRemap.targetArea')}
                t={t}
              />
              <input
                type="number"
                min={0}
                max={65535}
                value={row.targetAddress}
                readOnly={readOnly}
                onChange={(e) =>
                  patchRow(row.key, { targetAddress: numOr(e.target.value, 0) })
                }
                aria-label={t('property.modbusRemap.targetAddress')}
                className={cn(cellInput, readOnly && readOnlyInput)}
              />
            </div>
          ))}
        </div>
      )}

      {!readOnly && (
        <button type="button" onClick={addRow} className={addButton}>
          <Plus className="h-3.5 w-3.5" />
          {t('property.modbusRemap.addRule')}
        </button>
      )}
    </div>
  );
}

// ══════════════════════════════════════════════════════════════════════════
// Templates 편집기 (한 줄 행 테이블)
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

  // 컬럼: 선택 | area | offset | device_id | start | count | target_area
  const gridCols = 'grid-cols-[1.75rem_1.3fr_0.9fr_0.9fr_0.9fr_0.9fr_1.3fr]';

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
