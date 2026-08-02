// Modbus Server(modbus-server) 디바이스 배열 구조화 에디터.
//
// UX (사용자 확정):
//   1) 컴팩트 목록 + 팝업 편집: 목록 행은 unit_id + name 만 보여주고, 레지스터 맵은
//      팝업 모달에서 편집한다(추가/편집/삭제).
//   2) 디바이스 0 = 공유 맵 컨테이너: unit_id 0 의 특수 디바이스로, 와이어로 서빙되지
//      않는 SHARED 저장소(backing store)를 정의한다. 세그먼트는 모두 LOCAL(공유 참조 없음).
//   3) 서빙 디바이스의 세그먼트별 로컬/공유: 서빙 디바이스 팝업에서 각 영역
//      (coils/discrete_inputs/holding_registers/input_registers)에 세그먼트를 추가하며,
//      각 세그먼트는 LOCAL 또는 SHARED 이다. SHARED 세그먼트는 shared_address 로
//      디바이스 0 을 참조한다(주소 변환).
//
// 백엔드 devices 배열 형상(정확히 일치, 세그먼트 키는 이제 `address`):
//
//   LOCAL  세그먼트: { address, count, data_type?, initial_values?, type_map? }  ← shared_address 없음
//   SHARED 세그먼트: { address, count, shared_address }                          ← data_type/initial_values/type_map 생략
//   디바이스 0 컨테이너: { unit_id: 0, name?, register_map: { <area>: [ {address,count,data_type}, ... ] } } ← 전부 local
//   서빙 디바이스:       { unit_id: 1-247, name?, register_map: { <area>: [ <local/shared 혼재> ] } }
//
// data_type 는 local 세그먼트에는 항상 방출하고, shared 세그먼트에는 절대 방출하지 않아
// (address만 공통) 페이로드에서 local/shared 를 명확히 구분한다. initial_values/type_map 은
// UI 에서 편집하지 않지만 라운드트립 보존을 위해 통과(passthrough)시킨다.
//
// 팝업 모달은 이 코드베이스의 표준 모달 패턴(fixed inset overlay + ESC + stopPropagation,
// RenameKeyDialog/EditKeyMetaDialog 와 동형)을 그대로 사용한다.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Pencil, Plus, Trash2, X } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';
import { MODBUS_DATA_TYPE_OPTIONS } from '@/config/agentSchemas';

// ---- 상수 ----

const AREA_KEYS = [
  { key: 'coils', labelKey: 'property.register.areaCoils' },
  { key: 'discrete_inputs', labelKey: 'property.register.areaDiscreteInputs' },
  { key: 'holding_registers', labelKey: 'property.register.areaHoldingRegisters' },
  { key: 'input_registers', labelKey: 'property.register.areaInputRegisters' },
] as const;

type AreaKey = (typeof AREA_KEYS)[number]['key'];

const DEFAULT_DATA_TYPE = 'uint16';
const CONTAINER_UNIT_ID = 0;

// ---- 내부 행 타입 (React 렌더링용 안정 key + UI 전용 상태 포함) ----

interface SegmentRow {
  key: string;
  address: number;
  count: number;
  /** UI 전용: 공유 세그먼트 여부(방출값에는 shared_address 유무로 표현). */
  shared: boolean;
  /** local 전용. */
  dataType: string;
  /** shared 전용: 디바이스 0 내부의 기준 주소. */
  sharedAddress: number;
  /** UI 미편집 통과 필드(라운드트립 보존, local 전용). */
  initialValues?: unknown;
  typeMap?: unknown;
}

interface DeviceRow {
  key: string;
  unitId: number;
  name: string;
  areas: Record<AreaKey, SegmentRow[]>;
}

interface EditorState {
  /** 디바이스 0 공유 맵 컨테이너(선택, 최대 1개). */
  container: DeviceRow | null;
  /** 서빙 디바이스(unit_id 1-247). */
  served: DeviceRow[];
}

// ---- 방출(백엔드) 타입 ----

interface EmittedSegment {
  address: number;
  count: number;
  data_type?: string;
  shared_address?: number;
  initial_values?: unknown[];
  type_map?: unknown[];
}

interface EmittedDevice {
  unit_id: number;
  name?: string;
  register_map: Record<string, EmittedSegment[]>;
}

// ---- Props ----

interface ModbusServerDevicesEditorProps {
  value: unknown;
  onChange: (value: unknown) => void;
  readOnly?: boolean;
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

/** 세그먼트 객체 → SegmentRow. shared_address 유무로 local/shared 를 판정한다.
 *  legacy start_address 키는 방어적으로 address 로 폴백한다. */
function toSegmentRow(item: unknown): SegmentRow {
  const o = asObject(item);
  const isShared = o.shared_address !== undefined && o.shared_address !== null;
  return {
    key: nextKey('seg'),
    address: numOr(o.address ?? o.start_address, 0),
    count: numOr(o.count, 1),
    shared: isShared,
    dataType: asString(o.data_type) || DEFAULT_DATA_TYPE,
    sharedAddress: numOr(o.shared_address, 0),
    initialValues: o.initial_values,
    typeMap: o.type_map,
  };
}

function toAreas(registerMap: unknown): Record<AreaKey, SegmentRow[]> {
  const map = asObject(registerMap);
  const areas = emptyAreas();
  for (const area of AREA_KEYS) {
    const raw = map[area.key];
    if (!raw) continue;
    const segs = Array.isArray(raw) ? raw : [raw];
    areas[area.key] = segs.map(toSegmentRow);
  }
  return areas;
}

function toDeviceRow(item: unknown): DeviceRow {
  const o = asObject(item);
  return {
    key: nextKey('sdev'),
    unitId: numOr(o.unit_id, 1),
    name: asString(o.name),
    areas: toAreas(o.register_map),
  };
}

/** unknown(배열 | JSON 문자열 | 빈 값) → { container, served }. */
function parseValue(value: unknown): EditorState {
  let source: unknown = value;
  if (typeof value === 'string') {
    if (value.trim() === '') return { container: null, served: [] };
    try {
      source = JSON.parse(value);
    } catch {
      return { container: null, served: [] };
    }
  }
  if (!Array.isArray(source)) return { container: null, served: [] };

  let container: DeviceRow | null = null;
  const served: DeviceRow[] = [];
  for (const item of source) {
    const d = toDeviceRow(item);
    if (d.unitId === CONTAINER_UNIT_ID) {
      if (!container) container = d; // 최대 1개, 추가 unit_id 0 은 무시
    } else {
      served.push(d);
    }
  }
  return { container, served };
}

function newServedDevice(): DeviceRow {
  return { key: nextKey('sdev'), unitId: 1, name: '', areas: emptyAreas() };
}

function newContainer(): DeviceRow {
  return {
    key: nextKey('sdev'),
    unitId: CONTAINER_UNIT_ID,
    name: '',
    areas: emptyAreas(),
  };
}

function newSegment(): SegmentRow {
  return {
    key: nextKey('seg'),
    address: 0,
    count: 1,
    shared: false,
    dataType: DEFAULT_DATA_TYPE,
    sharedAddress: 0,
  };
}

/** SegmentRow → 백엔드 세그먼트. 컨테이너(디바이스 0)는 항상 local 로 방출한다.
 *  shared: { address, count, shared_address } (data_type 등 생략)
 *  local:  { address, count, data_type, initial_values?, type_map? } */
function toEmitSegment(s: SegmentRow, isContainer: boolean): EmittedSegment {
  if (s.shared && !isContainer) {
    return { address: s.address, count: s.count, shared_address: s.sharedAddress };
  }
  const out: EmittedSegment = {
    address: s.address,
    count: s.count,
    data_type: s.dataType || DEFAULT_DATA_TYPE,
  };
  if (Array.isArray(s.initialValues)) out.initial_values = s.initialValues;
  if (Array.isArray(s.typeMap)) out.type_map = s.typeMap;
  return out;
}

function toEmitDevice(d: DeviceRow, isContainer: boolean): EmittedDevice {
  const register_map: Record<string, EmittedSegment[]> = {};
  for (const area of AREA_KEYS) {
    const rows = d.areas[area.key];
    if (!rows || rows.length === 0) continue;
    register_map[area.key] = rows.map((s) => toEmitSegment(s, isContainer));
  }
  const name = d.name.trim();
  return name !== ''
    ? { unit_id: d.unitId, name, register_map }
    : { unit_id: d.unitId, register_map };
}

/** { container, served } → 백엔드 devices 배열 (컨테이너 먼저). */
function toEmit(state: EditorState): EmittedDevice[] {
  const out: EmittedDevice[] = [];
  if (state.container) out.push(toEmitDevice(state.container, true));
  for (const d of state.served) out.push(toEmitDevice(d, false));
  return out;
}

/** 디바이스의 총 세그먼트 수(방출 유효성 시각 힌트용). */
function segmentCount(d: DeviceRow): number {
  return AREA_KEYS.reduce((n, a) => n + d.areas[a.key].length, 0);
}

/** 한 영역 내 device-local 주소 범위 겹침 여부(시각 힌트용). */
function hasOverlap(rows: SegmentRow[]): boolean {
  const ranges = rows.map(
    (r) => [r.address, r.address + Math.max(r.count, 1) - 1] as const,
  );
  for (let i = 0; i < ranges.length; i++) {
    for (let j = i + 1; j < ranges.length; j++) {
      const a = ranges[i]!;
      const b = ranges[j]!;
      if (a[0] <= b[1] && b[0] <= a[1]) return true;
    }
  }
  return false;
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
// 영역별 세그먼트 에디터 (팝업 내부에서 사용)
// ──────────────────────────────────────────────────────────────────────────

interface AreaSegmentEditorProps {
  areas: Record<AreaKey, SegmentRow[]>;
  onChange: (areas: Record<AreaKey, SegmentRow[]>) => void;
  /** true 면 세그먼트별 "공유" 토글을 노출(서빙 디바이스). false 면 전부 local(컨테이너). */
  allowShared: boolean;
  readOnly?: boolean;
}

function AreaSegmentEditor({
  areas,
  onChange,
  allowShared,
  readOnly,
}: AreaSegmentEditorProps) {
  const { t } = useTranslation();
  // 선택 상태(UI 전용, 방출값에 영향 없음). 세그먼트 key 는 전역 고유.
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

  // 선택된 행 일괄 삭제. 선택 상태에서도 삭제된 key 를 정리한다.
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

  // 컬럼 정렬용 grid 템플릿: 선택 | 주소 | 개수 | 데이터타입 | (공유 | 공유주소)
  const gridCols = allowShared
    ? 'grid-cols-[1.75rem_1fr_1fr_1fr_2.5rem_1fr]'
    : 'grid-cols-[1.75rem_1fr_1fr_1fr]';

  return (
    <div className="space-y-3">
      {AREA_KEYS.map((area) => {
        const rows = areas[area.key];
        const overlap = hasOverlap(rows);
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
                  {t('property.modbusServerDevices.deleteSelected')} ({selectedInArea})
                </button>
              )}
            </div>

            {rows.length === 0 ? (
              <p className="py-1 text-center text-[11px] text-(--color-text-muted)">
                {t('property.modbusServerDevices.noSegments')}
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
                        aria-label={t('property.modbusServerDevices.selectAll')}
                        className="h-3.5 w-3.5"
                      />
                    )}
                  </div>
                  <span className={fieldLabel}>
                    {t('property.modbusServerDevices.address')}
                  </span>
                  <span className={fieldLabel}>
                    {t('property.modbusServerDevices.count')}
                  </span>
                  <span className={fieldLabel}>
                    {t('property.modbusServerDevices.dataType')}
                  </span>
                  {allowShared && (
                    <span className={cn(fieldLabel, 'text-center')}>
                      {t('property.modbusServerDevices.sharedColumn')}
                    </span>
                  )}
                  {allowShared && (
                    <span className={fieldLabel}>
                      {t('property.modbusServerDevices.sharedAddress')}
                    </span>
                  )}
                </div>

                {/* 세그먼트 행 (한 세그먼트 = 한 행) */}
                {rows.map((seg) => {
                  const isShared = allowShared && seg.shared;
                  return (
                    <div
                      key={seg.key}
                      className={cn(
                        'grid items-center gap-2 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-1 py-1',
                        gridCols,
                      )}
                    >
                      {/* 선택 체크박스 */}
                      <div className="flex justify-center">
                        {!readOnly && (
                          <input
                            type="checkbox"
                            checked={selected.has(seg.key)}
                            onChange={(e) => toggleSelect(seg.key, e.target.checked)}
                            aria-label={t('property.modbusServerDevices.selectRow')}
                            className="h-3.5 w-3.5"
                          />
                        )}
                      </div>

                      {/* 주소 */}
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
                        aria-label={t('property.modbusServerDevices.address')}
                        className={cn(cellInput, readOnly && readOnlyInput)}
                      />

                      {/* 개수 */}
                      <input
                        type="number"
                        min={1}
                        max={65535}
                        value={seg.count}
                        readOnly={readOnly}
                        onChange={(e) =>
                          patchSegment(area.key, seg.key, {
                            count: numOr(e.target.value, 1),
                          })
                        }
                        aria-label={t('property.modbusServerDevices.count')}
                        className={cn(cellInput, readOnly && readOnlyInput)}
                      />

                      {/* 데이터 타입 (공유 ON 이면 비활성/블랭크 — 컨테이너에서 상속) */}
                      {isShared ? (
                        <span className="text-center text-xs text-(--color-text-muted)">
                          —
                        </span>
                      ) : (
                        <select
                          value={seg.dataType}
                          disabled={readOnly}
                          onChange={(e) =>
                            patchSegment(area.key, seg.key, {
                              dataType: e.target.value,
                            })
                          }
                          aria-label={t('property.modbusServerDevices.dataType')}
                          className={cn(cellInput, readOnly && readOnlyInput)}
                        >
                          {MODBUS_DATA_TYPE_OPTIONS.map((dt) => (
                            <option key={dt} value={dt}>
                              {dt}
                            </option>
                          ))}
                        </select>
                      )}

                      {/* 공유 토글 (서빙 디바이스만) */}
                      {allowShared && (
                        <div className="flex justify-center">
                          <input
                            type="checkbox"
                            checked={seg.shared}
                            disabled={readOnly}
                            onChange={(e) =>
                              patchSegment(area.key, seg.key, {
                                shared: e.target.checked,
                              })
                            }
                            aria-label={t('property.modbusServerDevices.sharedColumn')}
                            className="h-3.5 w-3.5"
                          />
                        </div>
                      )}

                      {/* 공유 주소 (공유 ON 일 때만 활성) */}
                      {allowShared &&
                        (isShared ? (
                          <input
                            type="number"
                            min={0}
                            max={65535}
                            value={seg.sharedAddress}
                            readOnly={readOnly}
                            onChange={(e) =>
                              patchSegment(area.key, seg.key, {
                                sharedAddress: numOr(e.target.value, 0),
                              })
                            }
                            aria-label={t('property.modbusServerDevices.sharedAddress')}
                            className={cn(cellInput, readOnly && readOnlyInput)}
                          />
                        ) : (
                          <span className="text-center text-xs text-(--color-text-muted)">
                            —
                          </span>
                        ))}
                    </div>
                  );
                })}
              </div>
            )}

            {overlap && (
              <p className="text-[11px] text-red-500 dark:text-red-400">
                {t('property.modbusServerDevices.overlapHint')}
              </p>
            )}

            {!readOnly && (
              <button
                type="button"
                onClick={() => addSegment(area.key)}
                className={addButton}
              >
                <Plus className="h-3.5 w-3.5" />
                {t('property.modbusServerDevices.addSegment')}
              </button>
            )}
          </div>
        );
      })}
    </div>
  );
}

// ──────────────────────────────────────────────────────────────────────────
// 디바이스 편집 팝업 모달 (RenameKeyDialog 모달 패턴 재사용)
// ──────────────────────────────────────────────────────────────────────────

interface DeviceEditDialogProps {
  /** 편집 대상 초기값(신규는 newServedDevice()/newContainer()). */
  initial: DeviceRow;
  isContainer: boolean;
  readOnly?: boolean;
  /** 서빙 디바이스에서 중복 검사에 사용할 다른 디바이스의 unit_id 집합. */
  otherUnitIds: Set<number>;
  onSave: (device: DeviceRow) => void;
  onClose: () => void;
}

function DeviceEditDialog({
  initial,
  isContainer,
  readOnly,
  otherUnitIds,
  onSave,
  onClose,
}: DeviceEditDialogProps) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<DeviceRow>(initial);

  // ESC 로 닫기.
  useEffect(() => {
    const onKey = (e: globalThis.KeyboardEvent): void => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const duplicate = !isContainer && otherUnitIds.has(draft.unitId);
  const unitIdValid = isContainer || (draft.unitId >= 1 && draft.unitId <= 247);
  const canSave = !readOnly && unitIdValid && !duplicate;

  const title = isContainer
    ? t('property.modbusServerDevices.editSharedTitle')
    : t('property.modbusServerDevices.editServedTitle');

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-labelledby="modbus-device-edit-title"
    >
      <div
        className="mx-4 flex max-h-[85vh] w-full max-w-[560px] flex-col rounded-lg bg-(--color-bg-surface) shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* 헤더 */}
        <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-3">
          <h2
            id="modbus-device-edit-title"
            className="text-base font-semibold text-(--color-text-primary)"
          >
            {title}
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label={t('property.modbusServerDevices.close')}
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* 본문 (스크롤) */}
        <div className="space-y-4 overflow-y-auto px-5 py-4">
          {isContainer && (
            <p className="text-[11px] text-(--color-text-muted)">
              {t('property.modbusServerDevices.sharedMapDesc')}
            </p>
          )}

          <div className="grid grid-cols-2 gap-2">
            <label className="space-y-0.5">
              <span className={fieldLabel}>
                {t('property.modbusServerDevices.unitId')}
              </span>
              <input
                type="number"
                min={isContainer ? 0 : 1}
                max={isContainer ? 0 : 247}
                value={draft.unitId}
                readOnly={readOnly || isContainer}
                onChange={(e) =>
                  setDraft((d) => ({
                    ...d,
                    unitId: numOr(e.target.value, isContainer ? 0 : 1),
                  }))
                }
                className={cn(
                  cellInput,
                  (readOnly || isContainer) && readOnlyInput,
                  duplicate &&
                    'border-red-400 focus:border-red-400 focus:ring-red-400 dark:border-red-500',
                )}
              />
            </label>

            <label className="space-y-0.5">
              <span className={fieldLabel}>
                {t('property.modbusServerDevices.name')}
              </span>
              <input
                type="text"
                value={draft.name}
                readOnly={readOnly}
                onChange={(e) => setDraft((d) => ({ ...d, name: e.target.value }))}
                className={cn(cellInput, readOnly && readOnlyInput)}
                placeholder={isContainer ? 'shared' : 'device-1'}
              />
            </label>
          </div>

          {duplicate && (
            <p className="text-[11px] text-red-500 dark:text-red-400">
              {t('property.modbusServerDevices.unitIdDuplicate')}
            </p>
          )}
          {!unitIdValid && (
            <p className="text-[11px] text-red-500 dark:text-red-400">
              {t('property.modbusServerDevices.unitIdRange')}
            </p>
          )}

          {/* 레지스터 맵 (4개 영역, 세그먼트별 local/shared) */}
          <div className="space-y-2 border-t border-(--color-border-default) pt-3">
            <span className={fieldLabel}>
              {t('property.modbusServerDevices.registerMap')}
            </span>
            <AreaSegmentEditor
              areas={draft.areas}
              onChange={(areas) => setDraft((d) => ({ ...d, areas }))}
              allowShared={!isContainer}
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
            {t('property.modbusServerDevices.cancel')}
          </button>
          {!readOnly && (
            <button
              type="button"
              onClick={() => canSave && onSave(draft)}
              disabled={!canSave}
              className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {t('property.modbusServerDevices.save')}
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

/** 편집 팝업 대상 서술자. servedKey === null 은 신규 서빙 디바이스. */
type EditTarget =
  | { mode: 'container' }
  | { mode: 'served'; servedKey: string | null };

export function ModbusServerDevicesEditor({
  value,
  onChange,
  readOnly,
}: ModbusServerDevicesEditorProps) {
  const { t } = useTranslation();
  // 빈 값이면 시작점으로 서빙 디바이스 1개(unit_id 1)를 시드한다(컨테이너는 선택).
  const [state, setState] = useState<EditorState>(() => {
    const p = parseValue(value);
    if (!p.container && p.served.length === 0) {
      return { container: null, served: [newServedDevice()] };
    }
    return p;
  });
  const internalUpdate = useRef(false);
  const firstRun = useRef(true);
  const [editTarget, setEditTarget] = useState<EditTarget | null>(null);

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
    setState(parseValue(value));
  }, [value]);

  const emit = useCallback(
    (next: EditorState) => {
      setState(next);
      internalUpdate.current = true;
      onChange(toEmit(next));
    },
    [onChange],
  );

  // --- 목록 조작 ---

  const handleRemoveServed = useCallback(
    (devKey: string) =>
      emit({ ...state, served: state.served.filter((d) => d.key !== devKey) }),
    [state, emit],
  );

  const handleRemoveContainer = useCallback(
    () => emit({ ...state, container: null }),
    [state, emit],
  );

  // --- 팝업 저장 ---

  const handleSave = useCallback(
    (device: DeviceRow) => {
      if (!editTarget) return;
      if (editTarget.mode === 'container') {
        emit({ ...state, container: device });
      } else if (editTarget.servedKey === null) {
        emit({ ...state, served: [...state.served, device] });
      } else {
        const key = editTarget.servedKey;
        emit({
          ...state,
          served: state.served.map((d) => (d.key === key ? device : d)),
        });
      }
      setEditTarget(null);
    },
    [editTarget, state, emit],
  );

  // 중복 unit_id 시각 힌트 (서빙 디바이스 목록).
  const duplicateUnitIds = useMemo(() => {
    const seen = new Map<number, number>();
    for (const d of state.served) seen.set(d.unitId, (seen.get(d.unitId) ?? 0) + 1);
    return new Set([...seen.entries()].filter(([, n]) => n > 1).map(([id]) => id));
  }, [state.served]);

  // 팝업 대상 디바이스 + 중복검사용 unit_id 집합 계산.
  const dialogProps = useMemo(() => {
    if (!editTarget) return null;
    if (editTarget.mode === 'container') {
      return {
        initial: state.container ?? newContainer(),
        isContainer: true,
        otherUnitIds: new Set<number>(),
      };
    }
    const key = editTarget.servedKey;
    const initial =
      key === null
        ? newServedDevice()
        : (state.served.find((d) => d.key === key) ?? newServedDevice());
    const otherUnitIds = new Set(
      state.served.filter((d) => d.key !== key).map((d) => d.unitId),
    );
    return { initial, isContainer: false, otherUnitIds };
  }, [editTarget, state]);

  return (
    <div className="space-y-4">
      {/* 공유 맵 (디바이스 0) 섹션 */}
      <div className="space-y-2">
        <span className={fieldLabel}>
          {t('property.modbusServerDevices.sharedMap')}
        </span>

        {state.container ? (
          <div className="flex items-center justify-between rounded-md border border-(--color-border-default) bg-(--color-bg-surface) px-3 py-2">
            <div className="min-w-0">
              <span className="text-xs font-semibold text-(--color-text-secondary)">
                {t('property.modbusServerDevices.sharedMap')}
              </span>
              {state.container.name.trim() !== '' && (
                <span className="ml-2 truncate text-xs text-(--color-text-muted)">
                  {state.container.name}
                </span>
              )}
              <span className="ml-2 text-[11px] text-(--color-text-muted)">
                {t('property.modbusServerDevices.segmentCount')}: {segmentCount(state.container)}
              </span>
            </div>
            <div className="flex items-center gap-1">
              <button
                type="button"
                onClick={() => setEditTarget({ mode: 'container' })}
                className={iconButton}
                aria-label={t('property.modbusServerDevices.editSharedMap')}
              >
                <Pencil className="h-3.5 w-3.5" />
              </button>
              {!readOnly && (
                <button
                  type="button"
                  onClick={handleRemoveContainer}
                  className={removeButton}
                  aria-label={t('property.modbusServerDevices.removeSharedMap')}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              )}
            </div>
          </div>
        ) : (
          !readOnly && (
            <button
              type="button"
              onClick={() => setEditTarget({ mode: 'container' })}
              className={addButton}
            >
              <Plus className="h-3.5 w-3.5" />
              {t('property.modbusServerDevices.addSharedMap')}
            </button>
          )
        )}
      </div>

      {/* 서빙 디바이스 섹션 */}
      <div className="space-y-2 border-t border-(--color-border-default) pt-3">
        <span className={fieldLabel}>
          {t('property.modbusServerDevices.servedDevices')}
        </span>

        {state.served.length === 0 && (
          <p className="py-2 text-center text-xs text-(--color-text-muted)">
            {t('property.modbusServerDevices.empty')}
          </p>
        )}

        {state.served.map((device, idx) => {
          const isDuplicate = duplicateUnitIds.has(device.unitId);
          return (
            <div
              key={device.key}
              className="flex items-center justify-between rounded-md border border-(--color-border-default) bg-(--color-bg-surface) px-3 py-2"
            >
              <div className="min-w-0">
                <span className="text-xs font-semibold text-(--color-text-secondary)">
                  {t('property.modbusServerDevices.device')} {idx + 1}
                </span>
                <span
                  className={cn(
                    'ml-2 text-xs',
                    isDuplicate
                      ? 'text-red-500 dark:text-red-400'
                      : 'text-(--color-text-muted)',
                  )}
                >
                  {t('property.modbusServerDevices.unitId')}: {device.unitId}
                </span>
                {device.name.trim() !== '' && (
                  <span className="ml-2 truncate text-xs text-(--color-text-muted)">
                    {device.name}
                  </span>
                )}
                <span className="ml-2 text-[11px] text-(--color-text-muted)">
                  {t('property.modbusServerDevices.segmentCount')}: {segmentCount(device)}
                </span>
              </div>
              <div className="flex items-center gap-1">
                <button
                  type="button"
                  onClick={() =>
                    setEditTarget({ mode: 'served', servedKey: device.key })
                  }
                  className={iconButton}
                  aria-label={t('property.modbusServerDevices.editDevice')}
                >
                  <Pencil className="h-3.5 w-3.5" />
                </button>
                {!readOnly && (
                  <button
                    type="button"
                    onClick={() => handleRemoveServed(device.key)}
                    className={removeButton}
                    aria-label={t('property.modbusServerDevices.removeDevice')}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                )}
              </div>
            </div>
          );
        })}

        {!readOnly && (
          <button
            type="button"
            onClick={() => setEditTarget({ mode: 'served', servedKey: null })}
            className={addButton}
          >
            <Plus className="h-3.5 w-3.5" />
            {t('property.modbusServerDevices.addDevice')}
          </button>
        )}
      </div>

      {/* 편집 팝업 */}
      {dialogProps && (
        <DeviceEditDialog
          initial={dialogProps.initial}
          isContainer={dialogProps.isContainer}
          otherUnitIds={dialogProps.otherUnitIds}
          readOnly={readOnly}
          onSave={handleSave}
          onClose={() => setEditTarget(null)}
        />
      )}
    </div>
  );
}
