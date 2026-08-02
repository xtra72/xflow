// Modbus Client(modbus-client) 디바이스 배열 구조화 에디터.
//
// Server 에디터(ModbusServerDevicesEditor)와 동일한 UX 로 재구성한다:
//   1) 컴팩트 디바이스 목록 + 팝업 편집(추가/편집/삭제).
//   2) 레지스터 설정을 4개 AREA(코일/이산입력/보유레지스터/입력레지스터)로 조직한다.
//      영역이 function_code 를 유도한다(read-master: 1=coils 2=discrete_inputs
//      3=holding_registers 4=input_registers). 각 영역은 SEGMENT(=register_group,
//      한 번의 폴에서 읽는 연속 블록) 목록을 가진다.
//   3) 디바이스 레벨 일괄등록(fc 기반 붙여넣기)로 여러 register_group 을 한 번에 추가.
//
// 백엔드 devices 형상(internal/agent/modbus/config.go, 변경 없음)을 그대로 방출한다:
//   devices: [{
//     id?, host?(TCP 필수), port?(TCP, 기본 502), unit_id(1-247, 기본 1),
//     register_groups: [{
//       name?, function_code(1|2|3|4), start_address, quantity,
//       data_type?(기본 uint16), poll_interval?(Go duration),
//       type_map?: [{ address, data_type, byte_order(기본 big_endian) }]
//     }]
//   }]
//
// 트랜스포트 조건부: transport==='tcp'(또는 미지정) 는 host+port 를 노출/방출하고,
// transport==='rtu' 는 host/port 를 숨기고 방출에서 제외한다(시리얼 버스는 unit_id 로 식별).
// Server 와 달리 shared map / 디바이스 0 / shared_address 개념은 없다(클라이언트 전용).

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  ChevronDown,
  ChevronRight,
  ClipboardPaste,
  Pencil,
  Plus,
  Trash2,
  X,
} from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';
import {
  MODBUS_DATA_TYPE_OPTIONS,
  MODBUS_BYTE_ORDER_OPTIONS,
} from '@/config/agentSchemas';

// ---- 상수 ----

/** 4개 영역과 그 function_code. read-master 는 영역으로 fc 를 유도한다. */
const AREA_KEYS = [
  { key: 'coils', fc: 1, labelKey: 'property.register.areaCoils' },
  { key: 'discrete_inputs', fc: 2, labelKey: 'property.register.areaDiscreteInputs' },
  { key: 'holding_registers', fc: 3, labelKey: 'property.register.areaHoldingRegisters' },
  { key: 'input_registers', fc: 4, labelKey: 'property.register.areaInputRegisters' },
] as const;

type AreaKey = (typeof AREA_KEYS)[number]['key'];

/** function_code → 영역. */
const FC_TO_AREA: Record<string, AreaKey> = {
  '1': 'coils',
  '2': 'discrete_inputs',
  '3': 'holding_registers',
  '4': 'input_registers',
};

/** 영역 → function_code. */
const AREA_TO_FC: Record<AreaKey, number> = {
  coils: 1,
  discrete_inputs: 2,
  holding_registers: 3,
  input_registers: 4,
};

const DEFAULT_DATA_TYPE = 'uint16';
const DEFAULT_BYTE_ORDER = 'big_endian';

// ---- 내부 행 타입 ----

interface TypeMapRow {
  key: string;
  address: number;
  dataType: string;
  byteOrder: string;
}

/** 하나의 register_group(연속 블록)을 나타내는 세그먼트 행. */
interface SegmentRow {
  key: string;
  address: number; // start_address
  quantity: number;
  dataType: string;
  pollInterval: string; // Go duration 문자열
  name: string; // "설명"
  typeMap: TypeMapRow[];
  /** 고급 type_map 섹션 펼침(UI 전용, 방출 제외). */
  advancedOpen: boolean;
}

interface DeviceRow {
  key: string;
  id: string;
  host: string;
  port: number;
  unitId: number;
  areas: Record<AreaKey, SegmentRow[]>;
}

// ---- 방출(백엔드) 타입 ----

interface EmittedTypeMap {
  address: number;
  data_type: string;
  byte_order: string;
}

interface EmittedGroup {
  name?: string;
  function_code: number;
  start_address: number;
  quantity: number;
  data_type?: string;
  poll_interval?: string;
  type_map?: EmittedTypeMap[];
}

interface EmittedDevice {
  id?: string;
  host?: string;
  port?: number;
  unit_id: number;
  register_groups: EmittedGroup[];
}

// ---- Props ----

interface ModbusDevicesEditorProps {
  value: unknown;
  onChange: (value: unknown) => void;
  readOnly?: boolean;
  /** 형제 필드 transport 값 ('tcp' | 'rtu'). 미지정 시 'tcp'. */
  transport?: string;
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

function emptyAreas(): Record<AreaKey, SegmentRow[]> {
  return {
    coils: [],
    discrete_inputs: [],
    holding_registers: [],
    input_registers: [],
  };
}

function toTypeMapRow(item: unknown): TypeMapRow {
  const o = asObject(item);
  return {
    key: nextKey('tm'),
    address: numOr(o.address, 0),
    dataType: asString(o.data_type) || DEFAULT_DATA_TYPE,
    byteOrder: asString(o.byte_order) || DEFAULT_BYTE_ORDER,
  };
}

function toSegmentRow(group: unknown): SegmentRow {
  const o = asObject(group);
  const typeMap = Array.isArray(o.type_map) ? o.type_map.map(toTypeMapRow) : [];
  return {
    key: nextKey('seg'),
    address: numOr(o.start_address, 0),
    quantity: numOr(o.quantity, 1),
    dataType: asString(o.data_type) || DEFAULT_DATA_TYPE,
    pollInterval: asString(o.poll_interval),
    name: asString(o.name),
    typeMap,
    advancedOpen: typeMap.length > 0,
  };
}

/** register_groups → function_code 로 영역별 그룹핑. fc 1-4 외에는 holding_registers 로 폴백. */
function toAreas(groups: unknown): Record<AreaKey, SegmentRow[]> {
  const areas = emptyAreas();
  if (Array.isArray(groups)) {
    for (const g of groups) {
      const o = asObject(g);
      const fc = numOr(o.function_code, 3);
      const area = FC_TO_AREA[String(fc)] ?? 'holding_registers';
      areas[area].push(toSegmentRow(o));
    }
  }
  return areas;
}

function toDeviceRow(item: unknown): DeviceRow {
  const o = asObject(item);
  return {
    key: nextKey('dev'),
    id: asString(o.id),
    host: asString(o.host),
    port: numOr(o.port, 502),
    unitId: numOr(o.unit_id, 1),
    areas: toAreas(o.register_groups),
  };
}

/** unknown(배열 | JSON 문자열 | 빈 값) → DeviceRow[]. 항상 깨끗한 배열을 반환한다. */
function parseValue(value: unknown): DeviceRow[] {
  let source: unknown = value;
  if (typeof value === 'string') {
    if (value.trim() === '') return [];
    try {
      source = JSON.parse(value);
    } catch {
      return [];
    }
  }
  if (!Array.isArray(source)) return [];
  return source.map(toDeviceRow);
}

function newSegment(): SegmentRow {
  return {
    key: nextKey('seg'),
    address: 0,
    quantity: 1,
    dataType: DEFAULT_DATA_TYPE,
    pollInterval: '',
    name: '',
    typeMap: [],
    advancedOpen: false,
  };
}

function newTypeMapRow(): TypeMapRow {
  return {
    key: nextKey('tm'),
    address: 0,
    dataType: DEFAULT_DATA_TYPE,
    byteOrder: DEFAULT_BYTE_ORDER,
  };
}

function newDeviceRow(): DeviceRow {
  return {
    key: nextKey('dev'),
    id: '',
    host: '',
    port: 502,
    unitId: 1,
    areas: emptyAreas(),
  };
}

// ---- 방출 ----

function toEmitTypeMap(rows: TypeMapRow[]): EmittedTypeMap[] {
  return rows.map((e) => ({
    address: e.address,
    data_type: e.dataType,
    byte_order: e.byteOrder,
  }));
}

/** 세그먼트(SegmentRow) → register_group. function_code 는 영역에서 유도한다. */
function toEmitGroup(area: AreaKey, seg: SegmentRow): EmittedGroup {
  const out: EmittedGroup = {
    function_code: AREA_TO_FC[area],
    start_address: seg.address,
    quantity: seg.quantity,
  };
  if (seg.name.trim() !== '') out.name = seg.name.trim();
  const dt = seg.dataType.trim();
  if (dt !== '') out.data_type = dt;
  if (seg.pollInterval.trim() !== '') out.poll_interval = seg.pollInterval.trim();
  if (seg.typeMap.length > 0) out.type_map = toEmitTypeMap(seg.typeMap);
  return out;
}

/** DeviceRow → 백엔드 device. transport==='rtu' 면 host/port 를 방출하지 않는다. */
function toEmitDevice(d: DeviceRow, transport: string): EmittedDevice {
  const isRtu = transport === 'rtu';
  const register_groups: EmittedGroup[] = [];
  for (const area of AREA_KEYS) {
    for (const seg of d.areas[area.key]) {
      register_groups.push(toEmitGroup(area.key, seg));
    }
  }
  const out: EmittedDevice = { unit_id: d.unitId, register_groups };
  if (d.id.trim() !== '') out.id = d.id.trim();
  if (!isRtu) {
    if (d.host.trim() !== '') out.host = d.host.trim();
    out.port = d.port;
  }
  return out;
}

function toEmit(devices: DeviceRow[], transport: string): EmittedDevice[] {
  return devices.map((d) => toEmitDevice(d, transport));
}

/** 디바이스의 총 세그먼트(register_group) 수. */
function segmentCount(d: DeviceRow): number {
  return AREA_KEYS.reduce((n, a) => n + d.areas[a.key].length, 0);
}

// ---- 일괄등록(bulk paste) 파서 ----

/** 파싱된 register_group(순수 데이터, React key 없음). fc 로부터 유도된 area 태그 포함. */
export interface BulkGroup {
  area: AreaKey;
  address: number; // start_address
  quantity: number;
  dataType: string;
  pollInterval: string;
  name: string;
}

/** 파싱 오류. line 은 원본 1-based 줄 번호, code 는 i18n bulkError.<code> 키. */
export interface BulkParseError {
  line: number;
  code: string;
}

export interface BulkParseResult {
  groups: BulkGroup[];
  errors: BulkParseError[];
}

/** 콤마 또는 탭으로 셀 분리(탭 우선 자동 감지). 각 셀 트림. */
function splitCells(line: string): string[] {
  const parts = line.includes('\t') ? line.split('\t') : line.split(',');
  return parts.map((c) => c.trim());
}

/** 0 이상 정수 문자열 여부. */
function isNonNegInt(s: string): boolean {
  return /^\d+$/.test(s);
}

/**
 * 붙여넣기 텍스트 → fc 기반 register_group 파싱(순수 함수, 단위 테스트 대상).
 *
 * 한 줄 = 한 register_group. 콤마 또는 탭 구분(자동 감지), 셀 트림.
 * 컬럼: `fc, address, quantity, data_type, poll_interval, 설명`
 *  - 5열 또는 6열(설명 생략 시 5열). 그 외 열 개수 → wrongColumnCount.
 *  - fc 1-4 → coils/discrete_inputs/holding_registers/input_registers, 그 외 → invalidFc.
 *  - address(int ≥0), quantity(int ≥1). data_type 은 MODBUS_DATA_TYPE_OPTIONS 중 하나
 *    (빈 셀이면 uint16). poll_interval 은 자유 텍스트(비어도 됨, 검증하지 않음).
 *    설명(name)은 마지막 열, 선택.
 *  - 빈 줄 무시. 첫 non-empty 줄의 첫 셀(fc)이 숫자가 아니면 헤더로 보고 무시.
 *
 * 오류가 있어도 유효한 그룹은 그대로 담아 반환한다(호출부가 block-on-error 정책 적용).
 */
export function parseBulkGroups(text: string): BulkParseResult {
  const groups: BulkGroup[] = [];
  const errors: BulkParseError[] = [];
  const lines = text.split(/\r?\n/);
  let firstNonEmptySeen = false;

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]!.trim();
    if (line === '') continue;

    const cells = splitCells(line);

    if (!firstNonEmptySeen) {
      firstNonEmptySeen = true;
      if (!isNonNegInt(cells[0] ?? '')) continue; // 헤더 스킵
    }

    const lineNo = i + 1;
    const n = cells.length;
    if (n !== 5 && n !== 6) {
      errors.push({ line: lineNo, code: 'wrongColumnCount' });
      continue;
    }

    const area = FC_TO_AREA[cells[0] ?? ''];
    if (!area) {
      errors.push({ line: lineNo, code: 'invalidFc' });
      continue;
    }

    const addrCell = cells[1] ?? '';
    const qtyCell = cells[2] ?? '';
    const dtCell = cells[3] ?? '';
    const pollCell = cells[4] ?? '';
    const nameCell = cells[5] ?? '';

    if (!isNonNegInt(addrCell)) {
      errors.push({ line: lineNo, code: 'invalidAddress' });
      continue;
    }
    if (!isNonNegInt(qtyCell) || Number(qtyCell) < 1) {
      errors.push({ line: lineNo, code: 'invalidCount' });
      continue;
    }
    const dataType = dtCell === '' ? DEFAULT_DATA_TYPE : dtCell;
    if (!(MODBUS_DATA_TYPE_OPTIONS as readonly string[]).includes(dataType)) {
      errors.push({ line: lineNo, code: 'invalidDataType' });
      continue;
    }

    groups.push({
      area,
      address: Number(addrCell),
      quantity: Number(qtyCell),
      dataType,
      pollInterval: pollCell,
      name: nameCell,
    });
  }

  return { groups, errors };
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

// ──────────────────────────────────────────────────────────────────────────
// 영역별 세그먼트 에디터 (팝업 내부)
// ──────────────────────────────────────────────────────────────────────────

interface AreaSegmentEditorProps {
  areas: Record<AreaKey, SegmentRow[]>;
  onChange: (areas: Record<AreaKey, SegmentRow[]>) => void;
  readOnly?: boolean;
}

function AreaSegmentEditor({ areas, onChange, readOnly }: AreaSegmentEditorProps) {
  const { t } = useTranslation();
  const [selected, setSelected] = useState<Set<string>>(() => new Set());

  const patchArea = (areaKey: AreaKey, rows: SegmentRow[]) =>
    onChange({ ...areas, [areaKey]: rows });

  const addSegment = (areaKey: AreaKey) =>
    patchArea(areaKey, [...areas[areaKey], newSegment()]);

  const patchSegment = (
    areaKey: AreaKey,
    segKey: string,
    patch: Partial<SegmentRow>,
  ) =>
    patchArea(
      areaKey,
      areas[areaKey].map((s) => (s.key === segKey ? { ...s, ...patch } : s)),
    );

  const toggleSelect = (segKey: string, on: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev);
      if (on) next.add(segKey);
      else next.delete(segKey);
      return next;
    });

  const toggleSelectAll = (areaKey: AreaKey, on: boolean) =>
    setSelected((prev) => {
      const next = new Set(prev);
      for (const s of areas[areaKey]) {
        if (on) next.add(s.key);
        else next.delete(s.key);
      }
      return next;
    });

  const deleteSelected = (areaKey: AreaKey) => {
    const rows = areas[areaKey];
    setSelected((prev) => {
      const next = new Set(prev);
      for (const s of rows) next.delete(s.key);
      return next;
    });
    patchArea(
      areaKey,
      rows.filter((s) => !selected.has(s.key)),
    );
  };

  // --- type_map (고급) ---

  const patchTypeMap = (areaKey: AreaKey, segKey: string, typeMap: TypeMapRow[]) =>
    patchSegment(areaKey, segKey, { typeMap });

  const addTypeEntry = (areaKey: AreaKey, seg: SegmentRow) =>
    patchTypeMap(areaKey, seg.key, [...seg.typeMap, newTypeMapRow()]);

  const removeTypeEntry = (areaKey: AreaKey, seg: SegmentRow, tmKey: string) =>
    patchTypeMap(
      areaKey,
      seg.key,
      seg.typeMap.filter((e) => e.key !== tmKey),
    );

  const patchTypeEntry = (
    areaKey: AreaKey,
    seg: SegmentRow,
    tmKey: string,
    patch: Partial<TypeMapRow>,
  ) =>
    patchTypeMap(
      areaKey,
      seg.key,
      seg.typeMap.map((e) => (e.key === tmKey ? { ...e, ...patch } : e)),
    );

  // 컬럼 정렬용 grid 템플릿: 선택 | 주소 | 개수 | 데이터타입 | 폴링간격 | 설명
  const gridCols = 'grid-cols-[1.75rem_1fr_1fr_1fr_1fr_1.5fr]';

  return (
    <div className="space-y-3">
      {AREA_KEYS.map((area) => {
        const rows = areas[area.key];
        const selectedInArea = rows.filter((s) => selected.has(s.key)).length;
        const allSelected = rows.length > 0 && selectedInArea === rows.length;
        return (
          <div
            key={area.key}
            className="space-y-2 rounded border border-(--color-border-default) bg-(--color-bg-elevated) p-2"
          >
            {/* 영역 헤더 + 선택 삭제 */}
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold text-(--color-text-secondary)">
                {t(area.labelKey)}
              </span>
              {!readOnly && selectedInArea > 0 && (
                <button
                  type="button"
                  onClick={() => deleteSelected(area.key)}
                  className="inline-flex items-center gap-1 rounded px-2 py-1 text-[11px] font-medium text-red-500 transition-colors hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                  {t('property.modbusDevices.deleteSelected')} ({selectedInArea})
                </button>
              )}
            </div>

            {rows.length === 0 ? (
              <p className="py-1 text-center text-[11px] text-(--color-text-muted)">
                {t('property.modbusDevices.noSegments')}
              </p>
            ) : (
              <div className="space-y-1">
                {/* 컬럼 헤더 행 */}
                <div className={cn('grid items-center gap-2 px-1', gridCols)}>
                  <div className="flex justify-center">
                    {!readOnly && (
                      <input
                        type="checkbox"
                        checked={allSelected}
                        onChange={(e) => toggleSelectAll(area.key, e.target.checked)}
                        aria-label={t('property.modbusDevices.selectAll')}
                        className="h-3.5 w-3.5"
                      />
                    )}
                  </div>
                  <span className={fieldLabel}>{t('property.modbusDevices.address')}</span>
                  <span className={fieldLabel}>{t('property.modbusDevices.quantity')}</span>
                  <span className={fieldLabel}>{t('property.modbusDevices.dataType')}</span>
                  <span className={fieldLabel}>{t('property.modbusDevices.pollInterval')}</span>
                  <span className={fieldLabel}>
                    {t('property.modbusDevices.descriptionColumn')}
                  </span>
                </div>

                {/* 세그먼트 (한 세그먼트 = 한 행 + 선택적 고급 type_map) */}
                {rows.map((seg) => (
                  <div key={seg.key} className="space-y-1">
                    <div
                      className={cn(
                        'grid items-center gap-2 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-1 py-1',
                        gridCols,
                      )}
                    >
                      {/* 선택 */}
                      <div className="flex justify-center">
                        {!readOnly && (
                          <input
                            type="checkbox"
                            checked={selected.has(seg.key)}
                            onChange={(e) => toggleSelect(seg.key, e.target.checked)}
                            aria-label={t('property.modbusDevices.selectRow')}
                            className="h-3.5 w-3.5"
                          />
                        )}
                      </div>

                      {/* 주소 (start_address) */}
                      <input
                        type="number"
                        min={0}
                        max={65535}
                        value={seg.address}
                        readOnly={readOnly}
                        onChange={(e) =>
                          patchSegment(area.key, seg.key, {
                            address: numOr(e.target.value, 0),
                          })
                        }
                        aria-label={t('property.modbusDevices.address')}
                        className={cn(cellInput, readOnly && readOnlyInput)}
                      />

                      {/* 개수 (quantity) */}
                      <input
                        type="number"
                        min={1}
                        max={65535}
                        value={seg.quantity}
                        readOnly={readOnly}
                        onChange={(e) =>
                          patchSegment(area.key, seg.key, {
                            quantity: numOr(e.target.value, 1),
                          })
                        }
                        aria-label={t('property.modbusDevices.quantity')}
                        className={cn(cellInput, readOnly && readOnlyInput)}
                      />

                      {/* 데이터 타입 */}
                      <select
                        value={seg.dataType}
                        disabled={readOnly}
                        onChange={(e) =>
                          patchSegment(area.key, seg.key, { dataType: e.target.value })
                        }
                        aria-label={t('property.modbusDevices.dataType')}
                        className={cn(cellInput, readOnly && readOnlyInput)}
                      >
                        {MODBUS_DATA_TYPE_OPTIONS.map((dt) => (
                          <option key={dt} value={dt}>
                            {dt}
                          </option>
                        ))}
                      </select>

                      {/* 폴링 간격 (poll_interval) */}
                      <input
                        type="text"
                        value={seg.pollInterval}
                        readOnly={readOnly}
                        onChange={(e) =>
                          patchSegment(area.key, seg.key, {
                            pollInterval: e.target.value,
                          })
                        }
                        aria-label={t('property.modbusDevices.pollInterval')}
                        placeholder={t('property.modbusDevices.pollIntervalPlaceholder')}
                        className={cn(cellInput, readOnly && readOnlyInput)}
                      />

                      {/* 설명 (name) */}
                      <input
                        type="text"
                        value={seg.name}
                        readOnly={readOnly}
                        onChange={(e) =>
                          patchSegment(area.key, seg.key, { name: e.target.value })
                        }
                        aria-label={t('property.modbusDevices.descriptionColumn')}
                        className={cn(cellInput, readOnly && readOnlyInput)}
                      />
                    </div>

                    {/* 고급: type_map (주소별 data_type/byte_order 오버라이드) */}
                    <div className="pl-1">
                      <button
                        type="button"
                        onClick={() =>
                          patchSegment(area.key, seg.key, {
                            advancedOpen: !seg.advancedOpen,
                          })
                        }
                        className="flex items-center gap-1 text-[11px] font-medium text-(--color-text-secondary) hover:text-(--color-text-primary)"
                        aria-expanded={seg.advancedOpen}
                      >
                        {seg.advancedOpen ? (
                          <ChevronDown className="h-3.5 w-3.5" />
                        ) : (
                          <ChevronRight className="h-3.5 w-3.5" />
                        )}
                        {t('property.modbusDevices.advanced')}
                      </button>

                      {seg.advancedOpen && (
                        <div className="mt-2 space-y-2 border-t border-(--color-border-default) pt-2">
                          {seg.typeMap.length === 0 && (
                            <p className="py-1 text-center text-[11px] text-(--color-text-muted)">
                              {t('property.modbusDevices.typeMapEmpty')}
                            </p>
                          )}

                          {seg.typeMap.map((entry) => (
                            <div key={entry.key} className="flex items-end gap-2">
                              <label className="flex-1 space-y-0.5">
                                <span className={fieldLabel}>
                                  {t('property.modbusDevices.address')}
                                </span>
                                <input
                                  type="number"
                                  min={0}
                                  max={65535}
                                  value={entry.address}
                                  readOnly={readOnly}
                                  onChange={(e) =>
                                    patchTypeEntry(area.key, seg, entry.key, {
                                      address: numOr(e.target.value, 0),
                                    })
                                  }
                                  className={cn(cellInput, readOnly && readOnlyInput)}
                                />
                              </label>

                              <label className="flex-1 space-y-0.5">
                                <span className={fieldLabel}>
                                  {t('property.modbusDevices.dataType')}
                                </span>
                                <select
                                  value={entry.dataType}
                                  disabled={readOnly}
                                  onChange={(e) =>
                                    patchTypeEntry(area.key, seg, entry.key, {
                                      dataType: e.target.value,
                                    })
                                  }
                                  className={cn(cellInput, readOnly && readOnlyInput)}
                                >
                                  {MODBUS_DATA_TYPE_OPTIONS.map((dt) => (
                                    <option key={dt} value={dt}>
                                      {dt}
                                    </option>
                                  ))}
                                </select>
                              </label>

                              <label className="flex-1 space-y-0.5">
                                <span className={fieldLabel}>
                                  {t('property.modbusDevices.byteOrder')}
                                </span>
                                <select
                                  value={entry.byteOrder}
                                  disabled={readOnly}
                                  onChange={(e) =>
                                    patchTypeEntry(area.key, seg, entry.key, {
                                      byteOrder: e.target.value,
                                    })
                                  }
                                  className={cn(cellInput, readOnly && readOnlyInput)}
                                >
                                  {MODBUS_BYTE_ORDER_OPTIONS.map((bo) => (
                                    <option key={bo} value={bo}>
                                      {bo}
                                    </option>
                                  ))}
                                </select>
                              </label>

                              {!readOnly && (
                                <button
                                  type="button"
                                  onClick={() => removeTypeEntry(area.key, seg, entry.key)}
                                  className={cn(removeButton, 'mb-1')}
                                  aria-label={t('property.modbusDevices.removeTypeEntry')}
                                >
                                  <Trash2 className="h-3.5 w-3.5" />
                                </button>
                              )}
                            </div>
                          ))}

                          {!readOnly && (
                            <button
                              type="button"
                              onClick={() => addTypeEntry(area.key, seg)}
                              className={addButton}
                            >
                              <Plus className="h-3.5 w-3.5" />
                              {t('property.modbusDevices.addTypeEntry')}
                            </button>
                          )}
                        </div>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}

            {!readOnly && (
              <button
                type="button"
                onClick={() => addSegment(area.key)}
                className={addButton}
              >
                <Plus className="h-3.5 w-3.5" />
                {t('property.modbusDevices.addSegment')}
              </button>
            )}
          </div>
        );
      })}
    </div>
  );
}

// ──────────────────────────────────────────────────────────────────────────
// 디바이스 편집 팝업 모달
// ──────────────────────────────────────────────────────────────────────────

interface DeviceEditDialogProps {
  initial: DeviceRow;
  transport: string;
  readOnly?: boolean;
  onSave: (device: DeviceRow) => void;
  onClose: () => void;
}

function DeviceEditDialog({
  initial,
  transport,
  readOnly,
  onSave,
  onClose,
}: DeviceEditDialogProps) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<DeviceRow>(initial);
  const [bulkOpen, setBulkOpen] = useState(false);
  const [bulkText, setBulkText] = useState('');
  const [bulkErrors, setBulkErrors] = useState<BulkParseError[]>([]);
  const isTcp = transport !== 'rtu';

  const closeBulk = () => {
    setBulkOpen(false);
    setBulkText('');
    setBulkErrors([]);
  };

  // block-on-error: 오류가 하나라도 있으면 아무것도 추가하지 않고 오류만 표시한다.
  // 파싱된 그룹을 fc 로 유도된 영역별 목록에 append 한다.
  const applyBulk = () => {
    const result = parseBulkGroups(bulkText);
    if (result.errors.length > 0) {
      setBulkErrors(result.errors);
      return;
    }
    if (result.groups.length === 0) {
      closeBulk();
      return;
    }
    setDraft((d) => {
      const next: Record<AreaKey, SegmentRow[]> = {
        coils: [...d.areas.coils],
        discrete_inputs: [...d.areas.discrete_inputs],
        holding_registers: [...d.areas.holding_registers],
        input_registers: [...d.areas.input_registers],
      };
      for (const g of result.groups) {
        next[g.area].push({
          key: nextKey('seg'),
          address: g.address,
          quantity: g.quantity,
          dataType: g.dataType,
          pollInterval: g.pollInterval,
          name: g.name,
          typeMap: [],
          advancedOpen: false,
        });
      }
      return { ...d, areas: next };
    });
    closeBulk();
  };

  useEffect(() => {
    const onKey = (e: globalThis.KeyboardEvent): void => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const unitIdValid = draft.unitId >= 1 && draft.unitId <= 247;
  const hostMissing = isTcp && draft.host.trim() === '';
  const canSave = !readOnly && unitIdValid && !hostMissing;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-labelledby="modbus-client-device-edit-title"
    >
      <div
        className="mx-4 flex max-h-[85vh] w-full max-w-[600px] flex-col rounded-lg bg-(--color-bg-surface) shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* 헤더 */}
        <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-3">
          <h2
            id="modbus-client-device-edit-title"
            className="text-base font-semibold text-(--color-text-primary)"
          >
            {t('property.modbusDevices.editDeviceTitle')}
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label={t('property.modbusDevices.close')}
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* 본문 */}
        <div className="space-y-4 overflow-y-auto px-5 py-4">
          {/* 디바이스 필드: id, (tcp) host+port, unit_id */}
          <div className="grid grid-cols-2 gap-2">
            <label className="space-y-0.5">
              <span className={fieldLabel}>{t('property.modbusDevices.id')}</span>
              <input
                type="text"
                value={draft.id}
                readOnly={readOnly}
                onChange={(e) => setDraft((d) => ({ ...d, id: e.target.value }))}
                className={cn(cellInput, readOnly && readOnlyInput)}
                placeholder="device-1"
              />
            </label>

            <label className="space-y-0.5">
              <span className={fieldLabel}>{t('property.modbusDevices.unitId')}</span>
              <input
                type="number"
                min={1}
                max={247}
                value={draft.unitId}
                readOnly={readOnly}
                onChange={(e) =>
                  setDraft((d) => ({ ...d, unitId: numOr(e.target.value, 1) }))
                }
                className={cn(cellInput, readOnly && readOnlyInput)}
              />
            </label>

            {isTcp && (
              <>
                <label className="space-y-0.5">
                  <span className={fieldLabel}>{t('property.modbusDevices.host')}</span>
                  <input
                    type="text"
                    value={draft.host}
                    readOnly={readOnly}
                    onChange={(e) => setDraft((d) => ({ ...d, host: e.target.value }))}
                    className={cn(
                      cellInput,
                      readOnly && readOnlyInput,
                      hostMissing &&
                        'border-red-400 focus:border-red-400 focus:ring-red-400 dark:border-red-500',
                    )}
                    placeholder="192.168.1.10"
                  />
                </label>

                <label className="space-y-0.5">
                  <span className={fieldLabel}>{t('property.modbusDevices.port')}</span>
                  <input
                    type="number"
                    min={1}
                    max={65535}
                    value={draft.port}
                    readOnly={readOnly}
                    onChange={(e) =>
                      setDraft((d) => ({ ...d, port: numOr(e.target.value, 502) }))
                    }
                    className={cn(cellInput, readOnly && readOnlyInput)}
                  />
                </label>
              </>
            )}
          </div>

          {!unitIdValid && (
            <p className="text-[11px] text-red-500 dark:text-red-400">
              {t('property.modbusDevices.unitIdRange')}
            </p>
          )}
          {hostMissing && (
            <p className="text-[11px] text-red-500 dark:text-red-400">
              {t('property.modbusDevices.hostRequired')}
            </p>
          )}

          {/* 레지스터 그룹 (4개 영역) + 일괄등록 */}
          <div className="space-y-2 border-t border-(--color-border-default) pt-3">
            <div className="flex items-center justify-between">
              <span className={fieldLabel}>
                {t('property.modbusDevices.registerGroups')}
              </span>
              {!readOnly && (
                <button
                  type="button"
                  onClick={() => (bulkOpen ? closeBulk() : setBulkOpen(true))}
                  className={addButton}
                >
                  <ClipboardPaste className="h-3.5 w-3.5" />
                  {t('property.modbusDevices.bulkRegister')}
                </button>
              )}
            </div>

            {/* 일괄등록 패널 (fc 로 영역 분배) */}
            {!readOnly && bulkOpen && (
              <div className="space-y-2 rounded border border-(--color-border-default) bg-(--color-bg-elevated) p-2">
                <p className="whitespace-pre-line text-[11px] text-(--color-text-muted)">
                  {t('property.modbusDevices.bulkHelp')}
                </p>
                <textarea
                  value={bulkText}
                  onChange={(e) => setBulkText(e.target.value)}
                  rows={5}
                  aria-label={t('property.modbusDevices.bulkRegister')}
                  placeholder={t('property.modbusDevices.bulkPlaceholder')}
                  className={cn(cellInput, 'font-mono')}
                />
                {bulkErrors.length > 0 && (
                  <ul className="space-y-0.5">
                    {bulkErrors.map((er) => (
                      <li
                        key={er.line}
                        className="text-[11px] text-red-500 dark:text-red-400"
                      >
                        {t('property.modbusDevices.bulkLinePrefix')} {er.line}:{' '}
                        {t(`property.modbusDevices.bulkError.${er.code}`)}
                      </li>
                    ))}
                  </ul>
                )}
                <div className="flex justify-end gap-2">
                  <button
                    type="button"
                    onClick={closeBulk}
                    className="rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-secondary)"
                  >
                    {t('property.modbusDevices.cancel')}
                  </button>
                  <button
                    type="button"
                    onClick={applyBulk}
                    className="rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700"
                  >
                    {t('property.modbusDevices.bulkApply')}
                  </button>
                </div>
              </div>
            )}

            <AreaSegmentEditor
              areas={draft.areas}
              onChange={(areas) => setDraft((d) => ({ ...d, areas }))}
              readOnly={readOnly}
            />
          </div>
        </div>

        {/* 푸터 */}
        <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-5 py-3">
          <button
            type="button"
            onClick={onClose}
            className="rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-secondary)"
          >
            {t('property.modbusDevices.cancel')}
          </button>
          {!readOnly && (
            <button
              type="button"
              onClick={() => canSave && onSave(draft)}
              disabled={!canSave}
              className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {t('property.modbusDevices.save')}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

// ──────────────────────────────────────────────────────────────────────────
// 메인 컴포넌트 (컴팩트 목록 + 팝업 편집)
// ──────────────────────────────────────────────────────────────────────────

/** 편집 팝업 대상. deviceKey === null 은 신규 디바이스. */
interface EditTarget {
  deviceKey: string | null;
}

export function ModbusDevicesEditor({
  value,
  onChange,
  readOnly,
  transport = 'tcp',
}: ModbusDevicesEditorProps) {
  const { t } = useTranslation();
  const [devices, setDevices] = useState<DeviceRow[]>(() => parseValue(value));
  const internalUpdate = useRef(false);
  const firstRun = useRef(true);
  const [editTarget, setEditTarget] = useState<EditTarget | null>(null);
  const isTcp = transport !== 'rtu';

  // 외부 value 변경 시 내부 동기화 (초기 렌더/내부 emit 제외).
  useEffect(() => {
    if (firstRun.current) {
      firstRun.current = false;
      return;
    }
    if (internalUpdate.current) {
      internalUpdate.current = false;
      return;
    }
    setDevices(parseValue(value));
  }, [value]);

  const emit = useCallback(
    (next: DeviceRow[]) => {
      setDevices(next);
      internalUpdate.current = true;
      onChange(toEmit(next, transport));
    },
    [onChange, transport],
  );

  const handleRemove = useCallback(
    (devKey: string) => emit(devices.filter((d) => d.key !== devKey)),
    [devices, emit],
  );

  const handleSave = useCallback(
    (device: DeviceRow) => {
      if (!editTarget) return;
      if (editTarget.deviceKey === null) {
        emit([...devices, device]);
      } else {
        const key = editTarget.deviceKey;
        emit(devices.map((d) => (d.key === key ? device : d)));
      }
      setEditTarget(null);
    },
    [editTarget, devices, emit],
  );

  const dialogInitial = useMemo(() => {
    if (!editTarget) return null;
    if (editTarget.deviceKey === null) return newDeviceRow();
    return devices.find((d) => d.key === editTarget.deviceKey) ?? newDeviceRow();
  }, [editTarget, devices]);

  return (
    <div className="space-y-3">
      {devices.length === 0 && (
        <p className="py-2 text-center text-xs text-(--color-text-muted)">
          {t('property.modbusDevices.empty')}
        </p>
      )}

      {devices.map((device, idx) => (
        <div
          key={device.key}
          className="flex items-center justify-between rounded-md border border-(--color-border-default) bg-(--color-bg-surface) px-3 py-2"
        >
          <div className="min-w-0">
            <span className="text-xs font-semibold text-(--color-text-secondary)">
              {t('property.modbusDevices.device')} {idx + 1}
            </span>
            {device.id.trim() !== '' && (
              <span className="ml-2 truncate text-xs text-(--color-text-muted)">
                {device.id}
              </span>
            )}
            <span className="ml-2 text-xs text-(--color-text-muted)">
              {t('property.modbusDevices.unitId')}: {device.unitId}
            </span>
            {isTcp && device.host.trim() !== '' && (
              <span className="ml-2 text-[11px] text-(--color-text-muted)">
                {device.host}:{device.port}
              </span>
            )}
            <span className="ml-2 text-[11px] text-(--color-text-muted)">
              {t('property.modbusDevices.segmentCount')}: {segmentCount(device)}
            </span>
          </div>
          <div className="flex items-center gap-1">
            <button
              type="button"
              onClick={() => setEditTarget({ deviceKey: device.key })}
              className={iconButton}
              aria-label={t('property.modbusDevices.editDevice')}
            >
              <Pencil className="h-3.5 w-3.5" />
            </button>
            {!readOnly && (
              <button
                type="button"
                onClick={() => handleRemove(device.key)}
                className={removeButton}
                aria-label={t('property.modbusDevices.removeDevice')}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            )}
          </div>
        </div>
      ))}

      {!readOnly && (
        <button
          type="button"
          onClick={() => setEditTarget({ deviceKey: null })}
          className={addButton}
        >
          <Plus className="h-3.5 w-3.5" />
          {t('property.modbusDevices.addDevice')}
        </button>
      )}

      {dialogInitial && (
        <DeviceEditDialog
          initial={dialogInitial}
          transport={transport}
          readOnly={readOnly}
          onSave={handleSave}
          onClose={() => setEditTarget(null)}
        />
      )}
    </div>
  );
}
