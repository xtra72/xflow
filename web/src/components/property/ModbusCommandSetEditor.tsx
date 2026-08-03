// Modbus 명령셋(command_set) 에디터 — modbus-write / modbus-read / modbus-control 노드용.
//
// 세 노드는 config 의 command_set(기본값)을 편집한다. 런타임에 입력 payload 의
// command_set 이 이를 오버라이드하지만, UI 는 config 기본값만 작성한다.
//
// 백엔드 형상(정확히 일치, internal/agent/modbus... command sets):
//   WriteOp   = { area, address, value?|values?, data_type?, byte_order?, unit_id? }
//   ReadOp    = { area, address, count, data_type?, byte_order?, unit_id? }
//   ControlOp = { action, params? }
//   area ∈ coils|discrete_inputs|holding_registers|input_registers
//   unit_id 0 = 공유(shared) 컨테이너. 빈 값이면 생략, 0 은 허용.
//
// Server/Client device 에디터와 동일한 one-line-row + 선택/선택삭제 패턴을 재사용한다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { Plus, Trash2 } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';
import {
  MODBUS_DATA_TYPE_OPTIONS,
  MODBUS_BYTE_ORDER_OPTIONS,
} from '@/config/agentSchemas';

// ---- 상수 ----

const AREA_OPTIONS = [
  'coils',
  'discrete_inputs',
  'holding_registers',
  'input_registers',
] as const;

const CONTROL_ACTIONS = [
  'start',
  'stop',
  'pause',
  'resume',
  'reconnect',
  'add_device',
  'remove_device',
  'set_config',
  'command',
] as const;

const DEFAULT_DATA_TYPE = 'uint16';
const DEFAULT_BYTE_ORDER = 'big_endian';

// ---- 값(value/values) 파싱 ----

/** 콤마/공백 구분 문자열 → 토큰 배열. 숫자면 number, 아니면 string 으로 변환. */
function tokenizeValues(text: string): Array<number | string> {
  return text
    .split(/[\s,]+/)
    .map((t) => t.trim())
    .filter((t) => t !== '')
    .map((t) => {
      const n = Number(t);
      return Number.isFinite(n) && t !== '' ? n : t;
    });
}

/**
 * 값 입력 문자열 → WriteOp 의 value/values 조각(순수 함수, 단위 테스트 대상).
 * 토큰 0개 → {} (생략), 1개 → { value }, 2개 이상 → { values: [...] }.
 */
export function parseValuesInput(
  text: string,
): { value: number | string } | { values: Array<number | string> } | Record<string, never> {
  const tokens = tokenizeValues(text);
  if (tokens.length === 0) return {};
  if (tokens.length === 1) return { value: tokens[0]! };
  return { values: tokens };
}

// ---- 내부 행 타입 ----

interface RwRow {
  key: string;
  area: string;
  address: number;
  /** write 전용: 값 입력(콤마/공백 구분). */
  valuesText: string;
  /** read 전용. */
  count: number;
  dataType: string;
  byteOrder: string;
  /** unit_id 입력(빈 문자열이면 생략, 0 허용). */
  unitId: string;
}

interface ControlRow {
  key: string;
  action: string;
  /** params JSON 텍스트(빈 값이면 생략). */
  paramsText: string;
}

// ---- 방출 타입 ----

interface EmittedWriteOp {
  area: string;
  address: number;
  value?: number | string;
  values?: Array<number | string>;
  data_type?: string;
  byte_order?: string;
  unit_id?: number;
}

interface EmittedReadOp {
  area: string;
  address: number;
  count: number;
  data_type?: string;
  byte_order?: string;
  unit_id?: number;
}

interface EmittedControlOp {
  action: string;
  params?: Record<string, unknown>;
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

/** unit_id 표시 문자열: number 이면 문자열화(0 포함), 없으면 빈 문자열. */
function unitIdToText(v: unknown): string {
  return typeof v === 'number' && Number.isFinite(v) ? String(v) : '';
}

// --- Read/Write 파싱 ---

function toRwRow(item: unknown): RwRow {
  const o = asObject(item);
  // value/values → 표시 텍스트로 역직렬화.
  let valuesText = '';
  if (Array.isArray(o.values)) {
    valuesText = o.values.map((x) => String(x)).join(', ');
  } else if (o.value !== undefined && o.value !== null) {
    valuesText = String(o.value);
  }
  return {
    key: nextKey('op'),
    area: asString(o.area) || 'holding_registers',
    address: numOr(o.address, 0),
    valuesText,
    count: numOr(o.count, 1),
    dataType: asString(o.data_type) || DEFAULT_DATA_TYPE,
    byteOrder: asString(o.byte_order) || DEFAULT_BYTE_ORDER,
    unitId: unitIdToText(o.unit_id),
  };
}

function toRwRows(value: unknown): RwRow[] {
  return toArray(value).map(toRwRow);
}

function newRwRow(): RwRow {
  return {
    key: nextKey('op'),
    area: 'holding_registers',
    address: 0,
    valuesText: '',
    count: 1,
    dataType: DEFAULT_DATA_TYPE,
    byteOrder: DEFAULT_BYTE_ORDER,
    unitId: '',
  };
}

function toEmitWrite(row: RwRow): EmittedWriteOp {
  const out: EmittedWriteOp = { area: row.area, address: row.address };
  const v = parseValuesInput(row.valuesText);
  if ('value' in v) out.value = v.value;
  else if ('values' in v) out.values = v.values;
  if (row.dataType.trim() !== '') out.data_type = row.dataType;
  if (row.byteOrder.trim() !== '') out.byte_order = row.byteOrder;
  if (row.unitId.trim() !== '') out.unit_id = numOr(row.unitId, 0);
  return out;
}

function toEmitRead(row: RwRow): EmittedReadOp {
  const out: EmittedReadOp = {
    area: row.area,
    address: row.address,
    count: row.count,
  };
  if (row.dataType.trim() !== '') out.data_type = row.dataType;
  if (row.byteOrder.trim() !== '') out.byte_order = row.byteOrder;
  if (row.unitId.trim() !== '') out.unit_id = numOr(row.unitId, 0);
  return out;
}

// --- Control 파싱 ---

function toControlRow(item: unknown): ControlRow {
  const o = asObject(item);
  const params = o.params;
  const paramsText =
    params && typeof params === 'object'
      ? JSON.stringify(params)
      : '';
  return {
    key: nextKey('ctl'),
    action: asString(o.action) || 'start',
    paramsText,
  };
}

function toControlRows(value: unknown): ControlRow[] {
  return toArray(value).map(toControlRow);
}

function newControlRow(): ControlRow {
  return { key: nextKey('ctl'), action: 'start', paramsText: '' };
}

function toEmitControl(row: ControlRow): EmittedControlOp {
  const out: EmittedControlOp = { action: row.action };
  const text = row.paramsText.trim();
  if (text !== '') {
    try {
      const parsed = JSON.parse(text);
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        out.params = parsed as Record<string, unknown>;
      }
    } catch {
      // 유효하지 않은 JSON 은 params 를 생략한다(부분 입력 방어).
    }
  }
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

// ---- 선택/삭제 헤더 ----

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
      {t('property.modbusCommandSet.deleteSelected')} ({count})
    </button>
  );
}

// ══════════════════════════════════════════════════════════════════════════
// Write / Read 명령셋 에디터
// ══════════════════════════════════════════════════════════════════════════

interface RwCommandSetEditorProps {
  value: unknown;
  onChange: (value: unknown) => void;
  readOnly?: boolean;
  mode: 'write' | 'read';
}

export function ModbusRwCommandSetEditor({
  value,
  onChange,
  readOnly,
  mode,
}: RwCommandSetEditorProps) {
  const { t } = useTranslation();
  const [rows, setRows] = useState<RwRow[]>(() => toRwRows(value));
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const internalUpdate = useRef(false);
  const firstRun = useRef(true);
  const isWrite = mode === 'write';

  useEffect(() => {
    if (firstRun.current) {
      firstRun.current = false;
      return;
    }
    if (internalUpdate.current) {
      internalUpdate.current = false;
      return;
    }
    setRows(toRwRows(value));
  }, [value]);

  const emit = useCallback(
    (next: RwRow[]) => {
      setRows(next);
      internalUpdate.current = true;
      onChange(next.map((r) => (isWrite ? toEmitWrite(r) : toEmitRead(r))));
    },
    [onChange, isWrite],
  );

  const addRow = () => emit([...rows, newRwRow()]);
  const patchRow = (key: string, patch: Partial<RwRow>) =>
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

  // 컬럼: 선택 | area | address | (write:값 / read:개수) | data_type | byte_order | unit_id
  const gridCols = 'grid-cols-[1.75rem_1.4fr_1fr_1.4fr_1fr_1.2fr_0.9fr]';

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className={fieldLabel}>{t('property.modbusCommandSet.commandSet')}</span>
        {!readOnly && selectedCount > 0 && (
          <DeleteSelectedButton count={selectedCount} onClick={deleteSelected} t={t} />
        )}
      </div>

      {rows.length === 0 ? (
        <p className="py-2 text-center text-xs text-(--color-text-muted)">
          {t('property.modbusCommandSet.empty')}
        </p>
      ) : (
        <div className="space-y-1">
          {/* 헤더 */}
          <div className={cn('grid items-center gap-2 px-1', gridCols)}>
            <div className="flex justify-center">
              {!readOnly && (
                <input
                  type="checkbox"
                  checked={allSelected}
                  onChange={(e) => toggleSelectAll(e.target.checked)}
                  aria-label={t('property.modbusCommandSet.selectAll')}
                  className="h-3.5 w-3.5"
                />
              )}
            </div>
            <span className={fieldLabel}>{t('property.modbusCommandSet.area')}</span>
            <span className={fieldLabel}>{t('property.modbusCommandSet.address')}</span>
            <span className={fieldLabel}>
              {isWrite
                ? t('property.modbusCommandSet.value')
                : t('property.modbusCommandSet.count')}
            </span>
            <span className={fieldLabel}>{t('property.modbusCommandSet.dataType')}</span>
            <span className={fieldLabel}>{t('property.modbusCommandSet.byteOrder')}</span>
            <span className={fieldLabel}>{t('property.modbusCommandSet.unitId')}</span>
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
                    aria-label={t('property.modbusCommandSet.selectRow')}
                    className="h-3.5 w-3.5"
                  />
                )}
              </div>

              <select
                value={row.area}
                disabled={readOnly}
                onChange={(e) => patchRow(row.key, { area: e.target.value })}
                aria-label={t('property.modbusCommandSet.area')}
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
                value={row.address}
                readOnly={readOnly}
                onChange={(e) => patchRow(row.key, { address: numOr(e.target.value, 0) })}
                aria-label={t('property.modbusCommandSet.address')}
                className={cn(cellInput, readOnly && readOnlyInput)}
              />

              {isWrite ? (
                <input
                  type="text"
                  value={row.valuesText}
                  readOnly={readOnly}
                  onChange={(e) => patchRow(row.key, { valuesText: e.target.value })}
                  aria-label={t('property.modbusCommandSet.value')}
                  placeholder={t('property.modbusCommandSet.valuePlaceholder')}
                  className={cn(cellInput, readOnly && readOnlyInput)}
                />
              ) : (
                <input
                  type="number"
                  min={1}
                  max={65535}
                  value={row.count}
                  readOnly={readOnly}
                  onChange={(e) => patchRow(row.key, { count: numOr(e.target.value, 1) })}
                  aria-label={t('property.modbusCommandSet.count')}
                  className={cn(cellInput, readOnly && readOnlyInput)}
                />
              )}

              <select
                value={row.dataType}
                disabled={readOnly}
                onChange={(e) => patchRow(row.key, { dataType: e.target.value })}
                aria-label={t('property.modbusCommandSet.dataType')}
                className={cn(cellInput, readOnly && readOnlyInput)}
              >
                {MODBUS_DATA_TYPE_OPTIONS.map((dt) => (
                  <option key={dt} value={dt}>
                    {dt}
                  </option>
                ))}
              </select>

              <select
                value={row.byteOrder}
                disabled={readOnly}
                onChange={(e) => patchRow(row.key, { byteOrder: e.target.value })}
                aria-label={t('property.modbusCommandSet.byteOrder')}
                className={cn(cellInput, readOnly && readOnlyInput)}
              >
                {MODBUS_BYTE_ORDER_OPTIONS.map((bo) => (
                  <option key={bo} value={bo}>
                    {bo}
                  </option>
                ))}
              </select>

              <input
                type="number"
                min={0}
                max={247}
                value={row.unitId}
                readOnly={readOnly}
                onChange={(e) => patchRow(row.key, { unitId: e.target.value })}
                aria-label={t('property.modbusCommandSet.unitId')}
                placeholder={t('property.modbusCommandSet.unitIdPlaceholder')}
                className={cn(cellInput, readOnly && readOnlyInput)}
              />
            </div>
          ))}
        </div>
      )}

      {!readOnly && (
        <button type="button" onClick={addRow} className={addButton}>
          <Plus className="h-3.5 w-3.5" />
          {t('property.modbusCommandSet.addOp')}
        </button>
      )}
    </div>
  );
}

// ══════════════════════════════════════════════════════════════════════════
// Control 명령셋 에디터
// ══════════════════════════════════════════════════════════════════════════

interface ControlCommandSetEditorProps {
  value: unknown;
  onChange: (value: unknown) => void;
  readOnly?: boolean;
}

export function ModbusControlCommandSetEditor({
  value,
  onChange,
  readOnly,
}: ControlCommandSetEditorProps) {
  const { t } = useTranslation();
  const [rows, setRows] = useState<ControlRow[]>(() => toControlRows(value));
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
    setRows(toControlRows(value));
  }, [value]);

  const emit = useCallback(
    (next: ControlRow[]) => {
      setRows(next);
      internalUpdate.current = true;
      onChange(next.map(toEmitControl));
    },
    [onChange],
  );

  const addRow = () => emit([...rows, newControlRow()]);
  const patchRow = (key: string, patch: Partial<ControlRow>) =>
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

  // 컬럼: 선택 | action | params(JSON)
  const gridCols = 'grid-cols-[1.75rem_1fr_2fr]';

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className={fieldLabel}>{t('property.modbusCommandSet.commandSet')}</span>
        {!readOnly && selectedCount > 0 && (
          <DeleteSelectedButton count={selectedCount} onClick={deleteSelected} t={t} />
        )}
      </div>

      {rows.length === 0 ? (
        <p className="py-2 text-center text-xs text-(--color-text-muted)">
          {t('property.modbusCommandSet.empty')}
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
                  aria-label={t('property.modbusCommandSet.selectAll')}
                  className="h-3.5 w-3.5"
                />
              )}
            </div>
            <span className={fieldLabel}>{t('property.modbusCommandSet.action')}</span>
            <span className={fieldLabel}>{t('property.modbusCommandSet.params')}</span>
          </div>

          {rows.map((row) => (
            <div
              key={row.key}
              className={cn(
                'grid items-start gap-2 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-1 py-1',
                gridCols,
              )}
            >
              <div className="flex justify-center pt-1">
                {!readOnly && (
                  <input
                    type="checkbox"
                    checked={selected.has(row.key)}
                    onChange={(e) => toggleSelect(row.key, e.target.checked)}
                    aria-label={t('property.modbusCommandSet.selectRow')}
                    className="h-3.5 w-3.5"
                  />
                )}
              </div>

              <select
                value={row.action}
                disabled={readOnly}
                onChange={(e) => patchRow(row.key, { action: e.target.value })}
                aria-label={t('property.modbusCommandSet.action')}
                className={cn(cellInput, readOnly && readOnlyInput)}
              >
                {CONTROL_ACTIONS.map((a) => (
                  <option key={a} value={a}>
                    {a}
                  </option>
                ))}
              </select>

              <textarea
                value={row.paramsText}
                readOnly={readOnly}
                rows={2}
                onChange={(e) => patchRow(row.key, { paramsText: e.target.value })}
                aria-label={t('property.modbusCommandSet.params')}
                placeholder={t('property.modbusCommandSet.paramsPlaceholder')}
                className={cn(cellInput, 'font-mono text-xs', readOnly && readOnlyInput)}
              />
            </div>
          ))}
        </div>
      )}

      {!readOnly && (
        <button type="button" onClick={addRow} className={addButton}>
          <Plus className="h-3.5 w-3.5" />
          {t('property.modbusCommandSet.addOp')}
        </button>
      )}
    </div>
  );
}
