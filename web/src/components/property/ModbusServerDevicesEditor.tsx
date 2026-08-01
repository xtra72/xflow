// Modbus Server(modbus-server) 디바이스 배열 구조화 에디터.
//
// role=main 서버는 devices(각 unit_id + register_map)를 반드시 정의해야 한다
// (백엔드 internal/agent/modbusserver/config.go: main 은 register_map 또는 devices 필수).
// 이 에디터는 생성 시점에 디바이스를 정의하도록 한다. role=sub 는 main 의 맵을
// 상속하므로 이 필드는 visibleWhen(role=main)으로 숨겨진다.
//
// 백엔드 devices 배열 형상을 그대로 방출한다:
//
//   devices: [{
//     unit_id(1-247, 필수),
//     name?(선택),
//     register_map: {
//       coils?, discrete_inputs?, holding_registers?, input_registers?
//         각 영역은 segment 배열 [{ start_address, count, data_type? }, ...]
//     }
//   }]
//
// 영역별 register_map 편집은 기존 RegisterMapEditor 를 그대로 재사용한다
// (RegisterMapEditor 의 value/onChange 형상 { coils:[{start_address,count,data_type}],... }
//  가 백엔드 register_map 형상과 일치하므로 변환 없이 그대로 방출한다).

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Plus, Trash2 } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';
import { RegisterMapEditor } from './RegisterMapEditor';

// ---- 내부 행 타입 (React 렌더링용 안정 key 포함) ----

interface DeviceRow {
  key: string;
  unitId: number;
  name: string;
  /** RegisterMapEditor 가 다루는 register_map 객체 형상: { coils:[...], holding_registers:[...], ... } */
  registerMap: Record<string, unknown>;
}

// ---- 방출(백엔드) 타입 ----

interface EmittedDevice {
  unit_id: number;
  name?: string;
  register_map: Record<string, unknown>;
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

function toDeviceRow(item: unknown): DeviceRow {
  const o = asObject(item);
  return {
    key: nextKey('sdev'),
    unitId: numOr(o.unit_id, 1),
    name: asString(o.name),
    registerMap: asObject(o.register_map),
  };
}

/** unknown(배열 | JSON 문자열 | 빈 값) → DeviceRow[]. 항상 깨끗한 배열을 반환한다. */
function toDeviceRows(value: unknown): DeviceRow[] {
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

function newDeviceRow(): DeviceRow {
  return { key: nextKey('sdev'), unitId: 1, name: '', registerMap: {} };
}

/** DeviceRow[] → 백엔드 devices 배열. 빈 name 은 생략하고, register_map 은
 *  RegisterMapEditor 방출 형상(숫자 필드는 이미 number)을 변환 없이 그대로 방출한다. */
function toEmit(devices: DeviceRow[]): EmittedDevice[] {
  return devices.map((d) => {
    const out: EmittedDevice = {
      unit_id: d.unitId,
      register_map: d.registerMap ?? {},
    };
    if (d.name.trim() !== '') out.name = d.name.trim();
    return out;
  });
}

/** register_map 에 non-empty 영역이 하나라도 있는지(방출 유효성 시각 힌트용). */
function hasAnyArea(registerMap: Record<string, unknown>): boolean {
  return Object.values(registerMap).some(
    (v) => (Array.isArray(v) ? v.length > 0 : Boolean(v)),
  );
}

// ---- 스타일 (ModbusDevicesEditor 와 동일 팔레트) ----

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

export function ModbusServerDevicesEditor({
  value,
  onChange,
  readOnly,
}: ModbusServerDevicesEditorProps) {
  const { t } = useTranslation();
  // 빈 값이면 시작점으로 기본 디바이스 1개를 시드한다(방출은 하지 않음 —
  // 사용자가 unit_id/영역을 채우며 상호작용하면 그때 emit 된다).
  const [devices, setDevices] = useState<DeviceRow[]>(() => {
    const parsed = toDeviceRows(value);
    return parsed.length > 0 ? parsed : [newDeviceRow()];
  });
  const internalUpdate = useRef(false);
  // 초기 렌더는 useState 이니셜라이저가 이미 value 를 소비(빈 값이면 시드)했으므로
  // 첫 effect 실행에서 다시 setDevices(toDeviceRows(value)) 로 시드를 지우지 않는다.
  const firstRun = useRef(true);

  // 외부 value 변경 시 내부 동기화 (내부 emit 이 아닌 경우만).
  useEffect(() => {
    if (firstRun.current) {
      firstRun.current = false;
      return;
    }
    if (internalUpdate.current) {
      internalUpdate.current = false;
      return;
    }
    setDevices(toDeviceRows(value));
  }, [value]);

  const emit = useCallback(
    (updated: DeviceRow[]) => {
      setDevices(updated);
      internalUpdate.current = true;
      onChange(toEmit(updated));
    },
    [onChange],
  );

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

  // 중복 unit_id 시각 힌트 (백엔드도 검증하지만 UI 에서 미리 표시).
  const duplicateUnitIds = useMemo(() => {
    const seen = new Map<number, number>();
    for (const d of devices) seen.set(d.unitId, (seen.get(d.unitId) ?? 0) + 1);
    return new Set(
      [...seen.entries()].filter(([, n]) => n > 1).map(([id]) => id),
    );
  }, [devices]);

  return (
    <div className="space-y-3">
      {devices.length === 0 && (
        <p className="py-2 text-center text-xs text-(--color-text-muted)">
          {t('property.modbusServerDevices.empty')}
        </p>
      )}

      {devices.map((device, devIdx) => {
        const isDuplicate = duplicateUnitIds.has(device.unitId);
        return (
          <div
            key={device.key}
            className="space-y-3 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-3"
          >
            {/* 디바이스 헤더 */}
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold text-(--color-text-secondary)">
                {t('property.modbusServerDevices.device')} {devIdx + 1}
              </span>
              {!readOnly && (
                <button
                  type="button"
                  onClick={() => handleRemoveDevice(device.key)}
                  className={removeButton}
                  aria-label={t('property.modbusServerDevices.removeDevice')}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              )}
            </div>

            {/* 디바이스 필드: unit_id, name */}
            <div className="grid grid-cols-2 gap-2">
              <label className="space-y-0.5">
                <span className={fieldLabel}>
                  {t('property.modbusServerDevices.unitId')}
                </span>
                <input
                  type="number"
                  min={1}
                  max={247}
                  value={device.unitId}
                  readOnly={readOnly}
                  onChange={(e) =>
                    patchDevice(device.key, { unitId: numOr(e.target.value, 1) })
                  }
                  className={cn(
                    cellInput,
                    readOnly && readOnlyInput,
                    isDuplicate &&
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
                  value={device.name}
                  readOnly={readOnly}
                  onChange={(e) =>
                    patchDevice(device.key, { name: e.target.value })
                  }
                  className={cn(cellInput, readOnly && readOnlyInput)}
                  placeholder="device-1"
                />
              </label>
            </div>

            {isDuplicate && (
              <p className="text-[11px] text-red-500 dark:text-red-400">
                {t('property.modbusServerDevices.unitIdDuplicate')}
              </p>
            )}

            {/* 레지스터 맵 (4개 영역) — 기존 RegisterMapEditor 재사용 */}
            <div className="space-y-2 border-t border-(--color-border-default) pt-2">
              <span className={fieldLabel}>
                {t('property.modbusServerDevices.registerMap')}
              </span>
              <RegisterMapEditor
                value={device.registerMap}
                onChange={(rm) =>
                  patchDevice(device.key, { registerMap: asObject(rm) })
                }
                readOnly={readOnly}
              />
              {!hasAnyArea(device.registerMap) && (
                <p className="text-[11px] text-amber-600 dark:text-amber-400">
                  {t('property.modbusServerDevices.emptyMapHint')}
                </p>
              )}
            </div>
          </div>
        );
      })}

      {!readOnly && (
        <button type="button" onClick={handleAddDevice} className={addButton}>
          <Plus className="h-3.5 w-3.5" />
          {t('property.modbusServerDevices.addDevice')}
        </button>
      )}
    </div>
  );
}
