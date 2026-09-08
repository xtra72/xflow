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
import {
  AREA_KEYS,
  newDeviceRow,
  newSegment,
  newTypeMapRow,
  nextKey,
  numOr,
  parseBulkGroups,
  parseValue,
  segmentCount,
  toEmit,
  type AreaKey,
  type BulkParseError,
  type DeviceRow,
  type ModbusDevicesEditorProps,
  type SegmentRow,
  type TypeMapRow,
  modelToAreas,
  type DeviceModelOption,
} from './modbusDevicesModel';


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

/** 선택 행 대상 일괄 동작 버튼(삭제 제외 — 삭제는 위험 동작이라 빨간 스타일을 따로 쓴다). */
const selectionActionButton = cn(
  'inline-flex items-center gap-1 rounded px-2 py-1 text-[11px] font-medium',
  'text-(--color-text-muted) transition-colors',
  'hover:bg-(--color-bg-elevated) hover:text-(--color-text-primary)',
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

  /**
   * 영역의 그룹 사용 여부를 일괄 변경한다(SPEC-MODBUS-013 REQ-01).
   * onlySelected 가 true 면 선택된 행만, false 면 영역 전체를 바꾼다.
   */
  const setEnabledForArea = (areaKey: AreaKey, enabled: boolean, onlySelected: boolean) =>
    patchArea(
      areaKey,
      areas[areaKey].map((s) =>
        !onlySelected || selected.has(s.key) ? { ...s, enabled } : s,
      ),
    );

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
  const gridCols = 'grid-cols-[1.75rem_1fr_1fr_1fr_1fr_1.5fr_2.5rem]';

  return (
    <div className="space-y-3">
      {AREA_KEYS.map((area) => {
        const rows = areas[area.key];
        const selectedInArea = rows.filter((s) => selected.has(s.key)).length;
        const allSelected = rows.length > 0 && selectedInArea === rows.length;
        // 사용 열 마스터 체크박스 상태. 일부만 사용이면 indeterminate 로 표시한다.
        const enabledInArea = rows.filter((s) => s.enabled).length;
        const allEnabled = rows.length > 0 && enabledInArea === rows.length;
        const someEnabled = enabledInArea > 0 && !allEnabled;
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
                <div className="flex items-center gap-1">
                  {/* 선택 행 일괄 사용/해제 (SPEC-MODBUS-013 REQ-01) */}
                  <button
                    type="button"
                    onClick={() => setEnabledForArea(area.key, true, true)}
                    className={selectionActionButton}
                  >
                    {t('property.modbusDevices.enableSelected')} ({selectedInArea})
                  </button>
                  <button
                    type="button"
                    onClick={() => setEnabledForArea(area.key, false, true)}
                    className={selectionActionButton}
                  >
                    {t('property.modbusDevices.disableSelected')} ({selectedInArea})
                  </button>
                  <button
                    type="button"
                    onClick={() => deleteSelected(area.key)}
                    className="inline-flex items-center gap-1 rounded px-2 py-1 text-[11px] font-medium text-red-500 transition-colors hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                    {t('property.modbusDevices.deleteSelected')} ({selectedInArea})
                  </button>
                </div>
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
                  {/* 사용 열 마스터 체크박스 — 영역 전체 일괄 사용/해제 */}
                  <div className="flex justify-center">
                    {readOnly ? (
                      <span className={cn(fieldLabel, 'text-center')}>
                        {t('property.modbusDevices.enabledColumn')}
                      </span>
                    ) : (
                      <input
                        type="checkbox"
                        checked={allEnabled}
                        ref={(el) => {
                          if (el) el.indeterminate = someEnabled;
                        }}
                        onChange={(e) => setEnabledForArea(area.key, e.target.checked, false)}
                        aria-label={t('property.modbusDevices.enableAll')}
                        title={t('property.modbusDevices.enableAll')}
                        className="h-3.5 w-3.5"
                      />
                    )}
                  </div>
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

                      {/* 사용 여부 (SPEC-MODBUS-013 REQ-01). 해제하면 백엔드가 폴링에서 제외한다. */}
                      <div className="flex justify-center">
                        <input
                          type="checkbox"
                          checked={seg.enabled}
                          disabled={readOnly}
                          onChange={(e) =>
                            patchSegment(area.key, seg.key, { enabled: e.target.checked })
                          }
                          aria-label={t('property.modbusDevices.enabledColumn')}
                          title={t('property.modbusDevices.enabledHint')}
                          className="h-3.5 w-3.5"
                        />
                      </div>
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

export interface DeviceEditDialogProps {
  initial: DeviceRow;
  transport: string;
  readOnly?: boolean;
  // lockConnection: init 전용 필드(transport override / serial / share_session)를 숨기고
  // id 를 읽기전용으로 만든다. 이 필드들은 트랜스포트 오픈에 귀속되어 백엔드가 런타임 변경을
  // 거부한다(rejectInitOnlyFields).
  //
  // host/port 는 여기서 제외된다 — update_device 가 엔드포인트 변경을 지원하므로
  // (SPEC-MODBUS-013 M7) 수정 폼에서도 편집 가능하다.
  lockConnection?: boolean;
  // requireId: id 를 필수로 강제한다(빈 값이면 저장 불가 + 오류 표시). add_device(백엔드
  // parseDeviceConfig)는 device id 가 필수이므로 장치 탭의 추가 폼에서 사용한다.
  requireId?: boolean;
  // models: 디바이스 모델 카탈로그(SPEC-MODBUS-013 REQ-05). 부모(장치 탭)가 list_models exec 로
  // 조회해 주입한다. 비어 있거나 미지정이면 모델 선택기를 렌더하지 않는다(설정 탭 등 카탈로그
  // 접근 경로가 없는 화면에서 그대로 동작).
  models?: DeviceModelOption[];
  onSave: (device: DeviceRow) => void;
  onClose: () => void;
}

export function DeviceEditDialog({
  initial,
  transport,
  readOnly,
  lockConnection,
  requireId,
  models,
  onSave,
  onClose,
}: DeviceEditDialogProps) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<DeviceRow>(initial);
  const [bulkOpen, setBulkOpen] = useState(false);
  const [bulkText, setBulkText] = useState('');
  const [bulkErrors, setBulkErrors] = useState<BulkParseError[]>([]);
  // 모델 선택 → 덮어쓰기 확인 대기 중인 모델(SPEC-MODBUS-013 AC-21).
  const [pendingModel, setPendingModel] = useState<DeviceModelOption | null>(null);
  // submitted: 저장 버튼을 누르기 전에는 빈 필드에 오류(빨간 테두리)를 표시하지 않는다.
  // 최초 열림 시에는 필수(*) 표식만 노출하고, 저장 시도 후부터 검증 오류를 드러낸다.
  const [submitted, setSubmitted] = useState(false);
  // showPopover: 저장 불가 상태에서 저장을 누르면 누락 항목 말풍선(말풍선)을 띄운다.
  const [showPopover, setShowPopover] = useState(false);
  // 유효 트랜스포트 = per-device 오버라이드 ?? 에이전트 기본(F2). host/port 노출 판정에 사용한다.
  const effTransport = draft.transport !== '' ? draft.transport : transport;
  const isTcp = effTransport !== 'rtu';

  const closeBulk = () => {
    setBulkOpen(false);
    setBulkText('');
    setBulkErrors([]);
  };

  // 모델 적용: 레지스터 그룹을 모델 정의로 **교체**한다(추가가 아니라 덮어쓰기).
  // 기존 그룹이 있으면 먼저 확인을 받는다(AC-21). 저장은 자동으로 하지 않는다(AC-20).
  const applyModel = (model: DeviceModelOption) => {
    setDraft((d) => ({ ...d, areas: modelToAreas(model) }));
    setPendingModel(null);
  };

  const onSelectModel = (id: string) => {
    const model = models?.find((m) => m.id === id);
    if (!model) return;
    if (segmentCount(draft) > 0) {
      setPendingModel(model); // 확인 후 적용
      return;
    }
    applyModel(model);
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
          enabled: g.enabled,
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
  // 연결 잠금(update_device) 시 host/port/serial 은 숨겨지므로 검증 대상에서 제외한다.
  const hostMissing = isTcp && draft.host.trim() === '';
  // per-device rtu 오버라이드는 시리얼 포트가 필수다(백엔드 parseDeviceConfig 검증과 정합, AC-04).
  const serialMissing = !lockConnection && draft.transport === 'rtu' && draft.serialPort.trim() === '';
  // add_device 는 device id 가 필수다(백엔드 parseDeviceConfig 의 ErrMissingDeviceID 와 정합).
  const idMissing = !!requireId && draft.id.trim() === '';
  const canSave = !readOnly && unitIdValid && !hostMissing && !serialMissing && !idMissing;

  // 저장 차단 사유(말풍선에 표시). 현재 실패 중인 항목만 담는다.
  const saveIssues: string[] = [];
  if (idMissing) saveIssues.push(t('property.modbusDevices.idRequired'));
  if (!unitIdValid) saveIssues.push(t('property.modbusDevices.unitIdRange'));
  if (hostMissing) saveIssues.push(t('property.modbusDevices.hostRequired'));
  if (serialMissing) saveIssues.push(t('property.modbusDevices.serialRequired'));

  // 저장 시도: 검증 오류를 드러내고(submitted), 가능하면 저장, 아니면 말풍선을 띄운다.
  const handleSaveClick = (): void => {
    setSubmitted(true);
    if (canSave) {
      onSave(draft);
    } else {
      setShowPopover(true);
    }
  };

  // 필수 필드 표식(*). 라벨 텍스트 뒤에 붙인다.
  const requiredMark = <span className="text-red-500"> *</span>;

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
            className="rounded-md p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary)"
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
              <span className={fieldLabel}>
                {t('property.modbusDevices.id')}
                {requireId && requiredMark}
              </span>
              <input
                type="text"
                value={draft.id}
                // 연결 잠금(update_device) 시 id 는 디바이스 식별 키이므로 읽기 전용으로 표시한다.
                readOnly={readOnly || lockConnection}
                onChange={(e) => setDraft((d) => ({ ...d, id: e.target.value }))}
                className={cn(
                  cellInput,
                  (readOnly || lockConnection) && readOnlyInput,
                  submitted &&
                    idMissing &&
                    'border-red-400 focus:border-red-400 focus:ring-red-400 dark:border-red-500',
                )}
                placeholder="device-1"
              />
            </label>

            <label className="space-y-0.5">
              <span className={fieldLabel}>
                {t('property.modbusDevices.unitId')}
                {requiredMark}
              </span>
              <input
                type="number"
                min={1}
                max={247}
                value={draft.unitId}
                readOnly={readOnly}
                onChange={(e) =>
                  setDraft((d) => ({ ...d, unitId: numOr(e.target.value, 1) }))
                }
                className={cn(
                  cellInput,
                  readOnly && readOnlyInput,
                  submitted &&
                    !unitIdValid &&
                    'border-red-400 focus:border-red-400 focus:ring-red-400 dark:border-red-500',
                )}
              />
            </label>

            {/* host/port 는 편집 모드에서도 노출한다 — update_device 가 엔드포인트 변경을
                지원한다(SPEC-MODBUS-013 M7). 트랜스포트 전환·시리얼 파라미터는 여전히 init 전용이라
                아래 오버라이드 블록에서 계속 잠근다. */}
            {isTcp && (
              <>
                <label className="space-y-0.5">
                  <span className={fieldLabel}>
                    {t('property.modbusDevices.host')}
                    {requiredMark}
                  </span>
                  <input
                    type="text"
                    value={draft.host}
                    readOnly={readOnly}
                    onChange={(e) => setDraft((d) => ({ ...d, host: e.target.value }))}
                    className={cn(
                      cellInput,
                      readOnly && readOnlyInput,
                      submitted &&
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

          {/* 검증 오류는 저장 버튼 말풍선(말풍선)에서 요약 표시한다(상시 인라인 메시지 제거). */}

          {/* per-device 오버라이드 (SPEC-MODBUS-008 F2/F3): 트랜스포트 / 시리얼 포트 / 세션 공유.
              모두 '상속'이 기본이며, 상속일 때는 방출하지 않아 기존 설정과 바이트 동일하게 동작한다.
              연결 잠금(update_device, SPEC-MODBUS-009 F2) 시에는 이 블록 전체를 숨긴다 —
              트랜스포트 전환·시리얼 하드웨어 파라미터는 백엔드가 init 전용으로 거부한다. */}
          {!lockConnection && (
          <div className="grid grid-cols-2 gap-2 border-t border-(--color-border-default) pt-3">
            <label className="space-y-0.5">
              <span className={fieldLabel}>트랜스포트 오버라이드</span>
              <select
                value={draft.transport}
                disabled={readOnly}
                onChange={(e) => setDraft((d) => ({ ...d, transport: e.target.value }))}
                aria-label="per-device transport override"
                className={cn(cellInput, readOnly && readOnlyInput)}
              >
                <option value="">에이전트 기본 상속</option>
                <option value="tcp">tcp</option>
                <option value="rtu">rtu</option>
              </select>
            </label>

            <label className="space-y-0.5">
              <span className={fieldLabel}>세션 공유 오버라이드</span>
              <select
                value={draft.shareSession}
                disabled={readOnly}
                onChange={(e) => setDraft((d) => ({ ...d, shareSession: e.target.value }))}
                aria-label="per-device share_session override"
                className={cn(cellInput, readOnly && readOnlyInput)}
              >
                <option value="">에이전트 기본 상속</option>
                <option value="true">공유</option>
                <option value="false">독립</option>
              </select>
            </label>

            {draft.transport === 'rtu' && (
              <label className="col-span-2 space-y-0.5">
                <span className={fieldLabel}>
                  시리얼 포트 (per-device rtu)
                  {requiredMark}
                </span>
                <input
                  type="text"
                  value={draft.serialPort}
                  readOnly={readOnly}
                  onChange={(e) => setDraft((d) => ({ ...d, serialPort: e.target.value }))}
                  aria-label="per-device serial_port"
                  className={cn(
                    cellInput,
                    readOnly && readOnlyInput,
                    submitted &&
                      serialMissing &&
                      'border-red-400 focus:border-red-400 focus:ring-red-400 dark:border-red-500',
                  )}
                  placeholder="/dev/ttyUSB0"
                />
              </label>
            )}
          </div>
          )}

          {/* 레지스터 그룹 (4개 영역) + 일괄등록 */}
          <div className="space-y-2 border-t border-(--color-border-default) pt-3">
            <div className="flex items-center justify-between">
              <span className={fieldLabel}>
                {t('property.modbusDevices.registerGroups')}
              </span>
              {!readOnly && (
                <div className="flex items-center gap-2">
                  {/* 모델 선택기 — 카탈로그가 주입된 화면에서만 렌더한다(SPEC-MODBUS-013 REQ-05). */}
                  {models && models.length > 0 && (
                    <select
                      value=""
                      onChange={(e) => onSelectModel(e.target.value)}
                      aria-label={t('property.modbusDevices.modelSelect')}
                      title={t('property.modbusDevices.modelHint')}
                      className={cn(cellInput, 'max-w-[14rem]')}
                    >
                      <option value="">{t('property.modbusDevices.modelSelect')}</option>
                      {models.map((m) => (
                        <option key={m.id} value={m.id}>
                          {m.vendor ? `${m.vendor} ${m.name}` : m.name} ({m.register_count})
                        </option>
                      ))}
                    </select>
                  )}
                  <button
                    type="button"
                    onClick={() => (bulkOpen ? closeBulk() : setBulkOpen(true))}
                    className={addButton}
                  >
                    <ClipboardPaste className="h-3.5 w-3.5" />
                    {t('property.modbusDevices.bulkRegister')}
                  </button>
                </div>
              )}
            </div>

            {/* 모델 덮어쓰기 확인 (AC-21) */}
            {!readOnly && pendingModel && (
              <div className="space-y-2 rounded border border-(--color-border-warning,--color-border-default) bg-(--color-bg-elevated) p-2">
                <p className="text-[11px] text-(--color-text-muted)">
                  {`${pendingModel.name}: ${t('property.modbusDevices.modelOverwriteConfirm')} `}
                  {`(${segmentCount(draft)} → ${pendingModel.register_count})`}
                </p>
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => applyModel(pendingModel)}
                    className={addButton}
                  >
                    {t('property.modbusDevices.modelOverwriteApply')}
                  </button>
                  <button
                    type="button"
                    onClick={() => setPendingModel(null)}
                    className={addButton}
                  >
                    {t('property.modbusDevices.cancel')}
                  </button>
                </div>
              </div>
            )}

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
        <div className="relative flex justify-end gap-2 border-t border-(--color-border-default) px-5 py-3">
          {/* 저장 차단 말풍선: 저장 시도(showPopover) + 아직 저장 불가일 때만 표시하고,
              필드를 채워 canSave 가 되면 자동으로 사라진다. */}
          {!readOnly && showPopover && !canSave && (
            <div
              role="alert"
              className="absolute bottom-full right-5 z-10 mb-2 w-64 rounded-md border border-red-300 bg-(--color-bg-surface) p-3 shadow-lg dark:border-red-500"
            >
              <p className="mb-1 text-[11px] font-semibold text-(--color-text-primary)">
                {t('property.modbusDevices.saveBlockedTitle')}
              </p>
              <ul className="space-y-0.5">
                {saveIssues.map((msg) => (
                  <li key={msg} className="text-[11px] text-red-500 dark:text-red-400">
                    {msg}
                  </li>
                ))}
              </ul>
              {/* 말풍선 꼬리(저장 버튼 방향) */}
              <span className="absolute right-8 top-full h-2 w-2 -translate-y-1 rotate-45 border-b border-r border-red-300 bg-(--color-bg-surface) dark:border-red-500" />
            </div>
          )}
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
              onClick={handleSaveClick}
              className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700"
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
