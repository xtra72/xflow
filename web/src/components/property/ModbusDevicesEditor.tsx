// Modbus Client(modbus-client) 디바이스 배열 구조화 에디터.
//
// 기존 raw JSON textarea(type:'object') 를 대체한다. 백엔드
// internal/agent/modbus/config.go 의 devices 배열 형상을 그대로 방출한다:
//
//   devices: [{
//     id?, host?(TCP 필수), port?(TCP, 기본 502), unit_id(1-247, 기본 1),
//     register_groups: [{
//       name?, function_code(1|2|3|4), start_address, quantity,
//       data_type?(기본 uint16), poll_interval?(Go duration),
//       type_map?: [{ address, data_type, byte_order(기본 big_endian) }]
//     }]
//   }]
//
// 트랜스포트 조건부: transport==='tcp'(또는 미지정) 는 host+port 를 노출하고
// 방출하며, transport==='rtu' 는 host/port 를 숨기고 방출에서도 제외한다
// (시리얼 버스는 unit_id 로만 디바이스를 식별). byte_order 는 백엔드와 동일하게
// type_map 엔트리 레벨에만 존재한다(그룹 레벨에는 data_type 만).

import { useCallback, useEffect, useRef, useState } from 'react';
import { ChevronDown, ChevronRight, Plus, Trash2 } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';
import {
  MODBUS_DATA_TYPE_OPTIONS,
  MODBUS_BYTE_ORDER_OPTIONS,
} from '@/config/agentSchemas';

// ---- 상수 ----

/** function_code 옵션 (백엔드: 1-4 만 유효). 라벨은 i18n 키로 렌더한다. */
const FUNCTION_CODE_OPTIONS = [
  { code: 1, labelKey: 'property.modbusDevices.fc1' },
  { code: 2, labelKey: 'property.modbusDevices.fc2' },
  { code: 3, labelKey: 'property.modbusDevices.fc3' },
  { code: 4, labelKey: 'property.modbusDevices.fc4' },
] as const;

const DEFAULT_DATA_TYPE = 'uint16';
const DEFAULT_BYTE_ORDER = 'big_endian';

// ---- 내부 행 타입 (React 렌더링용 안정 key + UI 전용 상태 포함) ----

interface TypeMapRow {
  key: string;
  address: number;
  dataType: string;
  byteOrder: string;
}

interface GroupRow {
  key: string;
  name: string;
  functionCode: number;
  startAddress: number;
  quantity: number;
  dataType: string;
  pollInterval: string;
  typeMap: TypeMapRow[];
  /** 고급 type_map 섹션 펼침 여부 (UI 전용, 방출값에 포함되지 않음). */
  advancedOpen: boolean;
}

interface DeviceRow {
  key: string;
  id: string;
  host: string;
  port: number;
  unitId: number;
  groups: GroupRow[];
}

// ---- 방출(백엔드) 타입 ----

interface EmittedTypeMapEntry {
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
  type_map?: EmittedTypeMapEntry[];
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

/** 숫자로 변환하되 실패하면 기본값 반환. 백엔드는 JSON number 로 보내지만
 *  legacy 문자열 값도 방어적으로 처리한다. */
function numOr(v: unknown, def: number): number {
  if (typeof v === 'number' && Number.isFinite(v)) return v;
  if (typeof v === 'string' && v.trim() !== '') {
    const n = Number(v);
    if (Number.isFinite(n)) return n;
  }
  return def;
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

function toGroupRow(item: unknown): GroupRow {
  const o = asObject(item);
  const typeMap = Array.isArray(o.type_map) ? o.type_map.map(toTypeMapRow) : [];
  return {
    key: nextKey('grp'),
    name: asString(o.name),
    functionCode: numOr(o.function_code, 3),
    startAddress: numOr(o.start_address, 0),
    quantity: numOr(o.quantity, 1),
    dataType: asString(o.data_type) || DEFAULT_DATA_TYPE,
    pollInterval: asString(o.poll_interval),
    typeMap,
    advancedOpen: typeMap.length > 0,
  };
}

function toDeviceRow(item: unknown): DeviceRow {
  const o = asObject(item);
  return {
    key: nextKey('dev'),
    id: asString(o.id),
    host: asString(o.host),
    port: numOr(o.port, 502),
    unitId: numOr(o.unit_id, 1),
    groups: Array.isArray(o.register_groups)
      ? o.register_groups.map(toGroupRow)
      : [],
  };
}

/** unknown(배열 | 객체 | JSON 문자열 | 빈 값) → DeviceRow[]. 항상 깨끗한 배열을 반환한다. */
function toDevices(value: unknown): DeviceRow[] {
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

function toEmitTypeMap(rows: TypeMapRow[]): EmittedTypeMapEntry[] {
  return rows.map((e) => ({
    address: e.address,
    data_type: e.dataType,
    byte_order: e.byteOrder,
  }));
}

function toEmitGroup(g: GroupRow): EmittedGroup {
  const out: EmittedGroup = {
    function_code: g.functionCode,
    start_address: g.startAddress,
    quantity: g.quantity,
  };
  if (g.name.trim() !== '') out.name = g.name.trim();
  if (g.dataType.trim() !== '') out.data_type = g.dataType.trim();
  if (g.pollInterval.trim() !== '') out.poll_interval = g.pollInterval.trim();
  if (g.typeMap.length > 0) out.type_map = toEmitTypeMap(g.typeMap);
  return out;
}

/** DeviceRow[] → 백엔드 devices 배열. 빈 선택 필드는 생략한다.
 *  transport==='rtu' 면 host/port 를 방출하지 않는다(시리얼 버스). */
function toEmit(devices: DeviceRow[], transport: string): EmittedDevice[] {
  const isRtu = transport === 'rtu';
  return devices.map((d) => {
    const out: EmittedDevice = {
      unit_id: d.unitId,
      register_groups: d.groups.map(toEmitGroup),
    };
    if (d.id.trim() !== '') out.id = d.id.trim();
    if (!isRtu) {
      if (d.host.trim() !== '') out.host = d.host.trim();
      out.port = d.port;
    }
    return out;
  });
}

function newTypeMapRow(): TypeMapRow {
  return {
    key: nextKey('tm'),
    address: 0,
    dataType: DEFAULT_DATA_TYPE,
    byteOrder: DEFAULT_BYTE_ORDER,
  };
}

function newGroupRow(): GroupRow {
  return {
    key: nextKey('grp'),
    name: '',
    functionCode: 3,
    startAddress: 0,
    quantity: 1,
    dataType: DEFAULT_DATA_TYPE,
    pollInterval: '',
    typeMap: [],
    advancedOpen: false,
  };
}

function newDeviceRow(): DeviceRow {
  return { key: nextKey('dev'), id: '', host: '', port: 502, unitId: 1, groups: [] };
}

// ---- 스타일 ----

const cellInput = cn(
  'w-full rounded border px-2 py-1 text-sm',
  'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-primary)',
  'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
  'dark:focus:border-blue-500',
);

// readOnly 스타일: input 의 readOnly attr 와 함께 사용. disabled 는 다크모드에서
// 텍스트를 흐리게 렌더링하므로 배경만 살짝 다르게 표시한다 (RegisterMapEditor 참조).
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

// ---- 컴포넌트 ----

export function ModbusDevicesEditor({
  value,
  onChange,
  readOnly,
  transport = 'tcp',
}: ModbusDevicesEditorProps) {
  const { t } = useTranslation();
  const [devices, setDevices] = useState<DeviceRow[]>(() => toDevices(value));
  const internalUpdate = useRef(false);
  const isRtu = transport === 'rtu';

  // 외부 value 변경 시 내부 동기화 (내부 emit 이 아닌 경우만).
  useEffect(() => {
    if (internalUpdate.current) {
      internalUpdate.current = false;
      return;
    }
    setDevices(toDevices(value));
  }, [value]);

  const emit = useCallback(
    (updated: DeviceRow[]) => {
      setDevices(updated);
      internalUpdate.current = true;
      onChange(toEmit(updated, transport));
    },
    [onChange, transport],
  );

  // --- 디바이스 ---

  const handleAddDevice = useCallback(
    () => emit([...devices, newDeviceRow()]),
    [devices, emit],
  );

  const handleRemoveDevice = useCallback(
    (devKey: string) => emit(devices.filter((d) => d.key !== devKey)),
    [devices, emit],
  );

  const patchDevice = useCallback(
    (devKey: string, patch: Partial<DeviceRow>) =>
      emit(devices.map((d) => (d.key === devKey ? { ...d, ...patch } : d))),
    [devices, emit],
  );

  // --- 그룹 ---

  const patchGroups = useCallback(
    (devKey: string, groups: GroupRow[]) => patchDevice(devKey, { groups }),
    [patchDevice],
  );

  const handleAddGroup = useCallback(
    (devKey: string) => {
      const dev = devices.find((d) => d.key === devKey);
      if (!dev) return;
      patchGroups(devKey, [...dev.groups, newGroupRow()]);
    },
    [devices, patchGroups],
  );

  const handleRemoveGroup = useCallback(
    (devKey: string, grpKey: string) => {
      const dev = devices.find((d) => d.key === devKey);
      if (!dev) return;
      patchGroups(devKey, dev.groups.filter((g) => g.key !== grpKey));
    },
    [devices, patchGroups],
  );

  const patchGroup = useCallback(
    (devKey: string, grpKey: string, patch: Partial<GroupRow>) => {
      const dev = devices.find((d) => d.key === devKey);
      if (!dev) return;
      patchGroups(
        devKey,
        dev.groups.map((g) => (g.key === grpKey ? { ...g, ...patch } : g)),
      );
    },
    [devices, patchGroups],
  );

  // 고급 섹션 펼침 토글: UI 전용 상태이므로 onChange 를 호출하지 않는다.
  const toggleAdvanced = useCallback((devKey: string, grpKey: string) => {
    setDevices((prev) =>
      prev.map((d) =>
        d.key === devKey
          ? {
              ...d,
              groups: d.groups.map((g) =>
                g.key === grpKey ? { ...g, advancedOpen: !g.advancedOpen } : g,
              ),
            }
          : d,
      ),
    );
  }, []);

  // --- type_map ---

  const patchTypeMap = useCallback(
    (devKey: string, grpKey: string, typeMap: TypeMapRow[]) =>
      patchGroup(devKey, grpKey, { typeMap }),
    [patchGroup],
  );

  const handleAddTypeEntry = useCallback(
    (devKey: string, grpKey: string) => {
      const grp = devices
        .find((d) => d.key === devKey)
        ?.groups.find((g) => g.key === grpKey);
      if (!grp) return;
      patchTypeMap(devKey, grpKey, [...grp.typeMap, newTypeMapRow()]);
    },
    [devices, patchTypeMap],
  );

  const handleRemoveTypeEntry = useCallback(
    (devKey: string, grpKey: string, tmKey: string) => {
      const grp = devices
        .find((d) => d.key === devKey)
        ?.groups.find((g) => g.key === grpKey);
      if (!grp) return;
      patchTypeMap(devKey, grpKey, grp.typeMap.filter((e) => e.key !== tmKey));
    },
    [devices, patchTypeMap],
  );

  const patchTypeEntry = useCallback(
    (devKey: string, grpKey: string, tmKey: string, patch: Partial<TypeMapRow>) => {
      const grp = devices
        .find((d) => d.key === devKey)
        ?.groups.find((g) => g.key === grpKey);
      if (!grp) return;
      patchTypeMap(
        devKey,
        grpKey,
        grp.typeMap.map((e) => (e.key === tmKey ? { ...e, ...patch } : e)),
      );
    },
    [devices, patchTypeMap],
  );

  return (
    <div className="space-y-3">
      {devices.length === 0 && (
        <p className="py-2 text-center text-xs text-(--color-text-muted)">
          {t('property.modbusDevices.empty')}
        </p>
      )}

      {devices.map((device, devIdx) => (
        <div
          key={device.key}
          className="space-y-3 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-3"
        >
          {/* 디바이스 헤더 */}
          <div className="flex items-center justify-between">
            <span className="text-xs font-semibold text-(--color-text-secondary)">
              {t('property.modbusDevices.device')} {devIdx + 1}
            </span>
            {!readOnly && (
              <button
                type="button"
                onClick={() => handleRemoveDevice(device.key)}
                className={removeButton}
                aria-label={t('property.modbusDevices.removeDevice')}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            )}
          </div>

          {/* 디바이스 필드: id, (tcp) host+port, unit_id */}
          <div className="grid grid-cols-2 gap-2">
            <label className="space-y-0.5">
              <span className={fieldLabel}>{t('property.modbusDevices.id')}</span>
              <input
                type="text"
                value={device.id}
                readOnly={readOnly}
                onChange={(e) => patchDevice(device.key, { id: e.target.value })}
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
                value={device.unitId}
                readOnly={readOnly}
                onChange={(e) =>
                  patchDevice(device.key, { unitId: numOr(e.target.value, 1) })
                }
                className={cn(cellInput, readOnly && readOnlyInput)}
              />
            </label>

            {!isRtu && (
              <>
                <label className="space-y-0.5">
                  <span className={fieldLabel}>
                    {t('property.modbusDevices.host')}
                  </span>
                  <input
                    type="text"
                    value={device.host}
                    readOnly={readOnly}
                    onChange={(e) =>
                      patchDevice(device.key, { host: e.target.value })
                    }
                    className={cn(cellInput, readOnly && readOnlyInput)}
                    placeholder="192.168.1.10"
                  />
                </label>

                <label className="space-y-0.5">
                  <span className={fieldLabel}>
                    {t('property.modbusDevices.port')}
                  </span>
                  <input
                    type="number"
                    min={1}
                    max={65535}
                    value={device.port}
                    readOnly={readOnly}
                    onChange={(e) =>
                      patchDevice(device.key, { port: numOr(e.target.value, 502) })
                    }
                    className={cn(cellInput, readOnly && readOnlyInput)}
                  />
                </label>
              </>
            )}
          </div>

          {/* 레지스터 그룹 */}
          <div className="space-y-2 border-t border-(--color-border-default) pt-2">
            <span className={fieldLabel}>
              {t('property.modbusDevices.registerGroups')}
            </span>

            {device.groups.length === 0 && (
              <p className="py-1 text-center text-[11px] text-(--color-text-muted)">
                {t('property.modbusDevices.groupsEmpty')}
              </p>
            )}

            {device.groups.map((group) => (
              <div
                key={group.key}
                className="space-y-2 rounded border border-(--color-border-default) bg-(--color-bg-elevated) p-2"
              >
                <div className="grid grid-cols-2 gap-2">
                  <label className="space-y-0.5">
                    <span className={fieldLabel}>
                      {t('property.modbusDevices.name')}
                    </span>
                    <input
                      type="text"
                      value={group.name}
                      readOnly={readOnly}
                      onChange={(e) =>
                        patchGroup(device.key, group.key, { name: e.target.value })
                      }
                      className={cn(cellInput, readOnly && readOnlyInput)}
                    />
                  </label>

                  <label className="space-y-0.5">
                    <span className={fieldLabel}>
                      {t('property.modbusDevices.functionCode')}
                    </span>
                    <select
                      value={String(group.functionCode)}
                      disabled={readOnly}
                      onChange={(e) =>
                        patchGroup(device.key, group.key, {
                          functionCode: numOr(e.target.value, 3),
                        })
                      }
                      className={cn(cellInput, readOnly && readOnlyInput)}
                    >
                      {FUNCTION_CODE_OPTIONS.map((opt) => (
                        <option key={opt.code} value={String(opt.code)}>
                          {t(opt.labelKey)}
                        </option>
                      ))}
                    </select>
                  </label>

                  <label className="space-y-0.5">
                    <span className={fieldLabel}>
                      {t('property.modbusDevices.startAddress')}
                    </span>
                    <input
                      type="number"
                      min={0}
                      max={65535}
                      value={group.startAddress}
                      readOnly={readOnly}
                      onChange={(e) =>
                        patchGroup(device.key, group.key, {
                          startAddress: numOr(e.target.value, 0),
                        })
                      }
                      className={cn(cellInput, readOnly && readOnlyInput)}
                    />
                  </label>

                  <label className="space-y-0.5">
                    <span className={fieldLabel}>
                      {t('property.modbusDevices.quantity')}
                    </span>
                    <input
                      type="number"
                      min={1}
                      max={65535}
                      value={group.quantity}
                      readOnly={readOnly}
                      onChange={(e) =>
                        patchGroup(device.key, group.key, {
                          quantity: numOr(e.target.value, 1),
                        })
                      }
                      className={cn(cellInput, readOnly && readOnlyInput)}
                    />
                  </label>

                  <label className="space-y-0.5">
                    <span className={fieldLabel}>
                      {t('property.modbusDevices.dataType')}
                    </span>
                    <select
                      value={group.dataType}
                      disabled={readOnly}
                      onChange={(e) =>
                        patchGroup(device.key, group.key, {
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

                  <label className="space-y-0.5">
                    <span className={fieldLabel}>
                      {t('property.modbusDevices.pollInterval')}
                    </span>
                    <input
                      type="text"
                      value={group.pollInterval}
                      readOnly={readOnly}
                      onChange={(e) =>
                        patchGroup(device.key, group.key, {
                          pollInterval: e.target.value,
                        })
                      }
                      className={cn(cellInput, readOnly && readOnlyInput)}
                      placeholder={t('property.modbusDevices.pollIntervalPlaceholder')}
                    />
                  </label>
                </div>

                {/* 고급: type_map (주소별 타입 오버라이드) */}
                <div>
                  <button
                    type="button"
                    onClick={() => toggleAdvanced(device.key, group.key)}
                    className="flex items-center gap-1 text-[11px] font-medium text-(--color-text-secondary) hover:text-(--color-text-primary)"
                    aria-expanded={group.advancedOpen}
                  >
                    {group.advancedOpen ? (
                      <ChevronDown className="h-3.5 w-3.5" />
                    ) : (
                      <ChevronRight className="h-3.5 w-3.5" />
                    )}
                    {t('property.modbusDevices.advanced')}
                  </button>

                  {group.advancedOpen && (
                    <div className="mt-2 space-y-2 border-t border-(--color-border-default) pt-2">
                      {group.typeMap.length === 0 && (
                        <p className="py-1 text-center text-[11px] text-(--color-text-muted)">
                          {t('property.modbusDevices.typeMapEmpty')}
                        </p>
                      )}

                      {group.typeMap.map((entry) => (
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
                                patchTypeEntry(device.key, group.key, entry.key, {
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
                                patchTypeEntry(device.key, group.key, entry.key, {
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
                                patchTypeEntry(device.key, group.key, entry.key, {
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
                              onClick={() =>
                                handleRemoveTypeEntry(device.key, group.key, entry.key)
                              }
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
                          onClick={() => handleAddTypeEntry(device.key, group.key)}
                          className={addButton}
                        >
                          <Plus className="h-3.5 w-3.5" />
                          {t('property.modbusDevices.addTypeEntry')}
                        </button>
                      )}
                    </div>
                  )}
                </div>

                {!readOnly && (
                  <button
                    type="button"
                    onClick={() => handleRemoveGroup(device.key, group.key)}
                    className="inline-flex items-center gap-1 text-[11px] font-medium text-gray-400 transition-colors hover:text-red-500 dark:hover:text-red-400"
                  >
                    <Trash2 className="h-3 w-3" />
                    {t('property.modbusDevices.removeGroup')}
                  </button>
                )}
              </div>
            ))}

            {!readOnly && (
              <button
                type="button"
                onClick={() => handleAddGroup(device.key)}
                className={addButton}
              >
                <Plus className="h-3.5 w-3.5" />
                {t('property.modbusDevices.addGroup')}
              </button>
            )}
          </div>
        </div>
      ))}

      {!readOnly && (
        <button type="button" onClick={handleAddDevice} className={addButton}>
          <Plus className="h-3.5 w-3.5" />
          {t('property.modbusDevices.addDevice')}
        </button>
      )}
    </div>
  );
}
