// Modbus Gateway(modbus-gateway) 디바이스 배열 구조화 에디터.
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
import { AlertTriangle, ClipboardPaste, Pencil, Plus, Trash2, X } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';
import { MODBUS_DATA_TYPE_OPTIONS } from '@/config/agentSchemas';
import {
  AREA_KEYS,
  BACKING_BAUD_OPTIONS,
  BACKING_DATA_BITS_OPTIONS,
  BACKING_PARITY_OPTIONS,
  BACKING_STOP_BITS_OPTIONS,
  BACKING_TRANSPORT_OPTIONS,
  hasOverlap,
  newBacking,
  newContainer,
  newSegment,
  newServedDevice,
  nextKey,
  numOr,
  parseBulkSegments,
  parseValue,
  segmentCount,
  toEmit,
  type AreaKey,
  type BackingRow,
  type BulkParseError,
  type DeviceRow,
  type EditorState,
  type ModbusServerDevicesEditorProps,
  type SegmentRow,
} from './modbusServerDevicesModel';


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
  'shrink-0 rounded p-1 text-(--color-text-muted) transition-colors',
  'hover:bg-(--color-bg-elevated) hover:text-(--color-text-primary)',
);

const removeButton = cn(
  'shrink-0 rounded p-1 text-(--color-text-muted) transition-colors',
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

  // 컬럼 정렬용 grid 템플릿: 선택 | 주소 | 개수 | 데이터타입 | (공유 | 공유주소) | 설명
  const gridCols = allowShared
    ? 'grid-cols-[1.75rem_1fr_1fr_1fr_2.5rem_1fr_1.5fr]'
    : 'grid-cols-[1.75rem_1fr_1fr_1fr_1.5fr]';

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
                  <span className={fieldLabel}>
                    {t('property.modbusServerDevices.descriptionColumn')}
                  </span>
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

                      {/* 설명 (local/shared 공통) */}
                      <input
                        type="text"
                        value={seg.description}
                        readOnly={readOnly}
                        onChange={(e) =>
                          patchSegment(area.key, seg.key, {
                            description: e.target.value,
                          })
                        }
                        aria-label={t('property.modbusServerDevices.descriptionColumn')}
                        className={cn(cellInput, readOnly && readOnlyInput)}
                      />
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
// 실제(upstream) 디바이스 백킹 설정 에디터 (SPEC-MODBUS-010 REQ-06, 서빙 디바이스 전용)
// ──────────────────────────────────────────────────────────────────────────

interface BackingConfigEditorProps {
  backing: BackingRow | null;
  onChange: (backing: BackingRow | null) => void;
  readOnly?: boolean;
}

function BackingConfigEditor({ backing, onChange, readOnly }: BackingConfigEditorProps) {
  const { t } = useTranslation();
  const enabled = backing !== null;

  // 활성화 토글: 켜면 기본 백킹, 끄면 null(백킹 키 미방출 → 순수 slave).
  const toggleEnabled = (on: boolean) => onChange(on ? newBacking() : null);

  // 필드 패치(활성 상태에서만 유효).
  const patch = (p: Partial<BackingRow>) => {
    if (!backing) return;
    onChange({ ...backing, ...p });
  };

  const isRtu = backing?.transport === 'rtu';
  const isIndirect = backing?.mode === 'indirect';

  return (
    <div className="space-y-2 border-t border-(--color-border-default) pt-3">
      <label className="flex items-center gap-2">
        <input
          type="checkbox"
          checked={enabled}
          disabled={readOnly}
          onChange={(e) => toggleEnabled(e.target.checked)}
          aria-label={t('property.modbusServerDevices.backingEnable')}
          className="h-3.5 w-3.5"
        />
        <span className={fieldLabel}>{t('property.modbusServerDevices.backingSection')}</span>
      </label>

      <p className="text-[11px] text-(--color-text-muted)">
        {t('property.modbusServerDevices.backingDesc')}
      </p>

      {enabled && backing && (
        <div className="space-y-3 rounded border border-(--color-border-default) bg-(--color-bg-elevated) p-2">
          <div className="grid grid-cols-2 gap-2">
            {/* 트랜스포트 */}
            <label className="space-y-0.5">
              <span className={fieldLabel}>{t('property.modbusServerDevices.backingTransport')}</span>
              <select
                value={backing.transport}
                disabled={readOnly}
                onChange={(e) => patch({ transport: e.target.value })}
                aria-label={t('property.modbusServerDevices.backingTransport')}
                className={cn(cellInput, readOnly && readOnlyInput)}
              >
                {BACKING_TRANSPORT_OPTIONS.map((o) => (
                  <option key={o} value={o}>
                    {o}
                  </option>
                ))}
              </select>
            </label>

            {/* 모드 */}
            <label className="space-y-0.5">
              <span className={fieldLabel}>{t('property.modbusServerDevices.backingMode')}</span>
              <select
                value={backing.mode}
                disabled={readOnly}
                onChange={(e) => patch({ mode: e.target.value })}
                aria-label={t('property.modbusServerDevices.backingMode')}
                className={cn(cellInput, readOnly && readOnlyInput)}
              >
                <option value="direct">
                  {t('property.modbusServerDevices.backingModeDirect')}
                </option>
                <option value="indirect">
                  {t('property.modbusServerDevices.backingModeIndirect')}
                </option>
              </select>
            </label>

            {/* upstream unit id */}
            <label className="space-y-0.5">
              <span className={fieldLabel}>{t('property.modbusServerDevices.backingUnitId')}</span>
              <input
                type="number"
                min={0}
                max={247}
                value={backing.unitId}
                readOnly={readOnly}
                onChange={(e) => patch({ unitId: numOr(e.target.value, 1) })}
                aria-label={t('property.modbusServerDevices.backingUnitId')}
                className={cn(cellInput, readOnly && readOnlyInput)}
              />
            </label>

            {/* TCP endpoint */}
            {!isRtu && (
              <label className="space-y-0.5">
                <span className={fieldLabel}>{t('property.modbusServerDevices.backingHost')}</span>
                <input
                  type="text"
                  value={backing.host}
                  readOnly={readOnly}
                  onChange={(e) => patch({ host: e.target.value })}
                  aria-label={t('property.modbusServerDevices.backingHost')}
                  placeholder="192.168.0.10"
                  className={cn(cellInput, readOnly && readOnlyInput)}
                />
              </label>
            )}
            {!isRtu && (
              <label className="space-y-0.5">
                <span className={fieldLabel}>{t('property.modbusServerDevices.backingPort')}</span>
                <input
                  type="number"
                  min={1}
                  max={65535}
                  value={backing.port}
                  readOnly={readOnly}
                  onChange={(e) => patch({ port: numOr(e.target.value, 502) })}
                  aria-label={t('property.modbusServerDevices.backingPort')}
                  className={cn(cellInput, readOnly && readOnlyInput)}
                />
              </label>
            )}

            {/* RTU 시리얼 파라미터 */}
            {isRtu && (
              <label className="space-y-0.5">
                <span className={fieldLabel}>
                  {t('property.modbusServerDevices.backingSerialPort')}
                </span>
                <input
                  type="text"
                  value={backing.serialPort}
                  readOnly={readOnly}
                  onChange={(e) => patch({ serialPort: e.target.value })}
                  aria-label={t('property.modbusServerDevices.backingSerialPort')}
                  placeholder="/dev/ttyUSB0"
                  className={cn(cellInput, readOnly && readOnlyInput)}
                />
              </label>
            )}
            {isRtu && (
              <label className="space-y-0.5">
                <span className={fieldLabel}>{t('property.modbusServerDevices.backingBaudRate')}</span>
                <select
                  value={backing.baudRate}
                  disabled={readOnly}
                  onChange={(e) => patch({ baudRate: e.target.value })}
                  aria-label={t('property.modbusServerDevices.backingBaudRate')}
                  className={cn(cellInput, readOnly && readOnlyInput)}
                >
                  {BACKING_BAUD_OPTIONS.map((o) => (
                    <option key={o} value={o}>
                      {o}
                    </option>
                  ))}
                </select>
              </label>
            )}
            {isRtu && (
              <label className="space-y-0.5">
                <span className={fieldLabel}>{t('property.modbusServerDevices.backingDataBits')}</span>
                <select
                  value={backing.dataBits}
                  disabled={readOnly}
                  onChange={(e) => patch({ dataBits: e.target.value })}
                  aria-label={t('property.modbusServerDevices.backingDataBits')}
                  className={cn(cellInput, readOnly && readOnlyInput)}
                >
                  {BACKING_DATA_BITS_OPTIONS.map((o) => (
                    <option key={o} value={o}>
                      {o}
                    </option>
                  ))}
                </select>
              </label>
            )}
            {isRtu && (
              <label className="space-y-0.5">
                <span className={fieldLabel}>{t('property.modbusServerDevices.backingStopBits')}</span>
                <select
                  value={backing.stopBits}
                  disabled={readOnly}
                  onChange={(e) => patch({ stopBits: e.target.value })}
                  aria-label={t('property.modbusServerDevices.backingStopBits')}
                  className={cn(cellInput, readOnly && readOnlyInput)}
                >
                  {BACKING_STOP_BITS_OPTIONS.map((o) => (
                    <option key={o} value={o}>
                      {o}
                    </option>
                  ))}
                </select>
              </label>
            )}
            {isRtu && (
              <label className="space-y-0.5">
                <span className={fieldLabel}>{t('property.modbusServerDevices.backingParity')}</span>
                <select
                  value={backing.parity}
                  disabled={readOnly}
                  onChange={(e) => patch({ parity: e.target.value })}
                  aria-label={t('property.modbusServerDevices.backingParity')}
                  className={cn(cellInput, readOnly && readOnlyInput)}
                >
                  {BACKING_PARITY_OPTIONS.map((o) => (
                    <option key={o} value={o}>
                      {o}
                    </option>
                  ))}
                </select>
              </label>
            )}

            {/* 타임아웃(양 모드; indirect 는 stale 허용 한도 겸용) */}
            <label className="space-y-0.5">
              <span className={fieldLabel}>{t('property.modbusServerDevices.backingTimeout')}</span>
              <input
                type="text"
                value={backing.timeout}
                readOnly={readOnly}
                onChange={(e) => patch({ timeout: e.target.value })}
                aria-label={t('property.modbusServerDevices.backingTimeout')}
                placeholder="2s"
                className={cn(cellInput, readOnly && readOnlyInput)}
              />
            </label>

            {/* 폴링 주기(indirect 전용) */}
            {isIndirect && (
              <label className="space-y-0.5">
                <span className={fieldLabel}>
                  {t('property.modbusServerDevices.backingPollInterval')}
                </span>
                <input
                  type="text"
                  value={backing.pollInterval}
                  readOnly={readOnly}
                  onChange={(e) => patch({ pollInterval: e.target.value })}
                  aria-label={t('property.modbusServerDevices.backingPollInterval')}
                  placeholder="1s"
                  className={cn(cellInput, readOnly && readOnlyInput)}
                />
              </label>
            )}
          </div>
        </div>
      )}
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
  // 디바이스 레벨 일괄등록 패널 상태(fc 로 영역에 분배).
  const [bulkOpen, setBulkOpen] = useState(false);
  const [bulkText, setBulkText] = useState('');
  const [bulkErrors, setBulkErrors] = useState<BulkParseError[]>([]);

  const closeBulk = () => {
    setBulkOpen(false);
    setBulkText('');
    setBulkErrors([]);
  };

  // block-on-error: 오류가 하나라도 있으면 아무것도 추가하지 않고 오류만 표시한다.
  // 파싱된 세그먼트를 fc 로 유도된 영역별 목록에 append 한다.
  const applyBulk = () => {
    const result = parseBulkSegments(bulkText, !isContainer);
    if (result.errors.length > 0) {
      setBulkErrors(result.errors);
      return;
    }
    if (result.segments.length === 0) {
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
      for (const s of result.segments) {
        next[s.area].push({
          key: nextKey('seg'),
          address: s.address,
          count: s.count,
          shared: s.shared,
          dataType: s.dataType,
          sharedAddress: s.sharedAddress,
          description: s.description,
        });
      }
      return { ...d, areas: next };
    });
    closeBulk();
  };

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
            className="rounded-md p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary)"
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
            <div className="flex items-center justify-between">
              <span className={fieldLabel}>
                {t('property.modbusServerDevices.registerMap')}
              </span>
              {!readOnly && (
                <button
                  type="button"
                  onClick={() => (bulkOpen ? closeBulk() : setBulkOpen(true))}
                  className={addButton}
                >
                  <ClipboardPaste className="h-3.5 w-3.5" />
                  {t('property.modbusServerDevices.bulkRegister')}
                </button>
              )}
            </div>

            {/* 일괄등록 패널 (디바이스 레벨, fc 로 영역 분배) */}
            {!readOnly && bulkOpen && (
              <div className="space-y-2 rounded border border-(--color-border-default) bg-(--color-bg-elevated) p-2">
                <p className="whitespace-pre-line text-[11px] text-(--color-text-muted)">
                  {isContainer
                    ? t('property.modbusServerDevices.bulkHelpContainer')
                    : t('property.modbusServerDevices.bulkHelp')}
                </p>
                <textarea
                  value={bulkText}
                  onChange={(e) => setBulkText(e.target.value)}
                  rows={5}
                  aria-label={t('property.modbusServerDevices.bulkRegister')}
                  placeholder={t('property.modbusServerDevices.bulkPlaceholder')}
                  className={cn(cellInput, 'font-mono')}
                />
                {bulkErrors.length > 0 && (
                  <ul className="space-y-0.5">
                    {bulkErrors.map((er) => (
                      <li
                        key={er.line}
                        className="text-[11px] text-red-500 dark:text-red-400"
                      >
                        {t('property.modbusServerDevices.bulkLinePrefix')} {er.line}:{' '}
                        {t(`property.modbusServerDevices.bulkError.${er.code}`)}
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
                    {t('property.modbusServerDevices.cancel')}
                  </button>
                  <button
                    type="button"
                    onClick={applyBulk}
                    className="rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700"
                  >
                    {t('property.modbusServerDevices.bulkApply')}
                  </button>
                </div>
              </div>
            )}

            <AreaSegmentEditor
              areas={draft.areas}
              onChange={(areas) => setDraft((d) => ({ ...d, areas }))}
              allowShared={!isContainer}
              readOnly={readOnly}
            />
          </div>

          {/* 실제(upstream) 디바이스 백킹 — 서빙 디바이스 전용(컨테이너=공유 저장소는 백킹 불가). */}
          {!isContainer && (
            <BackingConfigEditor
              backing={draft.backing}
              onChange={(backing) => setDraft((d) => ({ ...d, backing }))}
              readOnly={readOnly}
            />
          )}
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

  // 세그먼트 0개 디바이스 검증 (백엔드 register_map ≥1 영역 규칙, config.go:667).
  // 컨테이너(unit 0) + 서빙 디바이스 모두 대상 — 백엔드 검증은 디바이스별로 동일하게 적용된다.
  // 세그먼트가 없는 디바이스가 방출되면 백엔드가 자동 재시작 시 거부하므로, 여기서 시각적으로
  // 표시하고 부모(ModbusDevicesSection)의 저장 버튼을 게이팅한다(modbusServerDevicesValid).
  const invalidContainer =
    state.container !== null && segmentCount(state.container) === 0;
  const invalidServedKeys = useMemo(
    () =>
      new Set(
        state.served.filter((d) => segmentCount(d) === 0).map((d) => d.key),
      ),
    [state.served],
  );
  const hasInvalidDevice = invalidContainer || invalidServedKeys.size > 0;

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
      {/* 세그먼트 0개 디바이스 경고 배너 (백엔드 register_map ≥1 영역 규칙 위반 → 저장 차단) */}
      {hasInvalidDevice && (
        <div
          role="alert"
          className="flex items-start gap-2 rounded-md border border-red-400 bg-red-50 px-3 py-2 text-[11px] text-red-600 dark:border-red-500/50 dark:bg-red-900/20 dark:text-red-400"
        >
          <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
          <span>{t('property.modbusServerDevices.emptyRegisterMapBanner')}</span>
        </div>
      )}

      {/* 공유 맵 (디바이스 0) 섹션 */}
      <div className="space-y-2">
        <span className={fieldLabel}>
          {t('property.modbusServerDevices.sharedMap')}
        </span>

        {state.container ? (
          <>
          <div
            className={cn(
              'flex items-center justify-between rounded-md border bg-(--color-bg-surface) px-3 py-2',
              invalidContainer
                ? 'border-red-400 dark:border-red-500'
                : 'border-(--color-border-default)',
            )}
          >
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
          {invalidContainer && (
            <p className="text-[11px] text-red-500 dark:text-red-400">
              {t('property.modbusServerDevices.emptyRegisterMapError')}
            </p>
          )}
          </>
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
          const isEmpty = invalidServedKeys.has(device.key);
          return (
            <div key={device.key} className="space-y-1">
            <div
              className={cn(
                'flex items-center justify-between rounded-md border bg-(--color-bg-surface) px-3 py-2',
                isEmpty
                  ? 'border-red-400 dark:border-red-500'
                  : 'border-(--color-border-default)',
              )}
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
            {isEmpty && (
              <p className="text-[11px] text-red-500 dark:text-red-400">
                {t('property.modbusServerDevices.emptyRegisterMapError')}
              </p>
            )}
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
