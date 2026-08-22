// Modbus Client 디바이스 에디터의 순수 모델 계층 — 상수 / 행 타입 / 방출 타입 /
// 변환·방출 유틸 / 일괄등록 파서.
//
// 컴포넌트 파일(ModbusDevicesEditor.tsx)에서 분리했다. 컴포넌트 파일이 컴포넌트만
// 내보내야 Fast Refresh 가 동작하기 때문이다(react-refresh/only-export-components).

import {
  MODBUS_DATA_TYPE_OPTIONS,
} from '@/config/agentSchemas';

// ---- 상수 ----

/** 4개 영역과 그 function_code. read-master 는 영역으로 fc 를 유도한다. */
export const AREA_KEYS = [
  { key: 'coils', fc: 1, labelKey: 'property.register.areaCoils' },
  { key: 'discrete_inputs', fc: 2, labelKey: 'property.register.areaDiscreteInputs' },
  { key: 'holding_registers', fc: 3, labelKey: 'property.register.areaHoldingRegisters' },
  { key: 'input_registers', fc: 4, labelKey: 'property.register.areaInputRegisters' },
] as const;

export type AreaKey = (typeof AREA_KEYS)[number]['key'];

/** function_code → 영역. */
export const FC_TO_AREA: Record<string, AreaKey> = {
  '1': 'coils',
  '2': 'discrete_inputs',
  '3': 'holding_registers',
  '4': 'input_registers',
};

/** 영역 → function_code. */
export const AREA_TO_FC: Record<AreaKey, number> = {
  coils: 1,
  discrete_inputs: 2,
  holding_registers: 3,
  input_registers: 4,
};

export const DEFAULT_DATA_TYPE = 'uint16';
export const DEFAULT_BYTE_ORDER = 'big_endian';

// ---- 내부 행 타입 ----

export interface TypeMapRow {
  key: string;
  address: number;
  dataType: string;
  byteOrder: string;
}

/** 하나의 register_group(연속 블록)을 나타내는 세그먼트 행. */
export interface SegmentRow {
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

export interface DeviceRow {
  key: string;
  id: string;
  host: string;
  port: number;
  unitId: number;
  // per-device transport 오버라이드 (SPEC-MODBUS-008 F2). ''=에이전트 기본 상속, 'tcp'|'rtu'.
  transport: string;
  // per-device RTU 오버라이드(transport==='rtu')의 시리얼 포트. 상속 rtu 는 에이전트 값을 쓰므로 비운다.
  serialPort: string;
  // per-device 세션 공유 오버라이드 (SPEC-MODBUS-008 F3). ''=상속, 'true'|'false'=오버라이드.
  shareSession: string;
  areas: Record<AreaKey, SegmentRow[]>;
}

// ---- 방출(백엔드) 타입 ----

export interface EmittedTypeMap {
  address: number;
  data_type: string;
  byte_order: string;
}

export interface EmittedGroup {
  name?: string;
  function_code: number;
  start_address: number;
  quantity: number;
  data_type?: string;
  poll_interval?: string;
  type_map?: EmittedTypeMap[];
}

export interface EmittedDevice {
  id?: string;
  host?: string;
  port?: number;
  unit_id: number;
  // per-device 오버라이드 (SPEC-MODBUS-008 F2/F3). 미설정(상속) 시 방출하지 않아 하위 호환을 유지한다.
  transport?: string;
  serial_port?: string;
  share_session?: boolean;
  register_groups: EmittedGroup[];
}

// ---- Props ----

export interface ModbusDevicesEditorProps {
  value: unknown;
  onChange: (value: unknown) => void;
  readOnly?: boolean;
  /** 형제 필드 transport 값 ('tcp' | 'rtu'). 미지정 시 'tcp'. */
  transport?: string;
}

// ---- 변환 유틸 ----

let keyCounter = 0;
export function nextKey(prefix: string): string {
  return `${prefix}-${++keyCounter}-${Date.now()}`;
}

export function asObject(v: unknown): Record<string, unknown> {
  return v && typeof v === 'object' && !Array.isArray(v)
    ? (v as Record<string, unknown>)
    : {};
}

export function asString(v: unknown): string {
  return typeof v === 'string' ? v : '';
}

export function numOr(v: unknown, def: number): number {
  if (typeof v === 'number' && Number.isFinite(v)) return v;
  if (typeof v === 'string' && v.trim() !== '') {
    const n = Number(v);
    if (Number.isFinite(n)) return n;
  }
  return def;
}

export function emptyAreas(): Record<AreaKey, SegmentRow[]> {
  return {
    coils: [],
    discrete_inputs: [],
    holding_registers: [],
    input_registers: [],
  };
}

export function toTypeMapRow(item: unknown): TypeMapRow {
  const o = asObject(item);
  return {
    key: nextKey('tm'),
    address: numOr(o.address, 0),
    dataType: asString(o.data_type) || DEFAULT_DATA_TYPE,
    byteOrder: asString(o.byte_order) || DEFAULT_BYTE_ORDER,
  };
}

export function toSegmentRow(group: unknown): SegmentRow {
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
export function toAreas(groups: unknown): Record<AreaKey, SegmentRow[]> {
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

/** unknown 을 per-device share_session 삼상태 문자열로 변환한다(true/false → 'true'/'false', 그 외 → ''=상속). */
export function toShareOverride(v: unknown): string {
  if (v === true) return 'true';
  if (v === false) return 'false';
  return '';
}

export function toDeviceRow(item: unknown): DeviceRow {
  const o = asObject(item);
  const transport = asString(o.transport);
  return {
    key: nextKey('dev'),
    id: asString(o.id),
    host: asString(o.host),
    port: numOr(o.port, 502),
    unitId: numOr(o.unit_id, 1),
    transport: transport === 'tcp' || transport === 'rtu' ? transport : '',
    serialPort: asString(o.serial_port),
    shareSession: toShareOverride(o.share_session),
    areas: toAreas(o.register_groups),
  };
}

/** unknown(배열 | JSON 문자열 | 빈 값) → DeviceRow[]. 항상 깨끗한 배열을 반환한다. */
export function parseValue(value: unknown): DeviceRow[] {
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

export function newSegment(): SegmentRow {
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

export function newTypeMapRow(): TypeMapRow {
  return {
    key: nextKey('tm'),
    address: 0,
    dataType: DEFAULT_DATA_TYPE,
    byteOrder: DEFAULT_BYTE_ORDER,
  };
}

export function newDeviceRow(): DeviceRow {
  return {
    key: nextKey('dev'),
    id: '',
    host: '',
    port: 502,
    unitId: 1,
    transport: '',
    serialPort: '',
    shareSession: '',
    areas: emptyAreas(),
  };
}

// ---- 방출 ----

export function toEmitTypeMap(rows: TypeMapRow[]): EmittedTypeMap[] {
  return rows.map((e) => ({
    address: e.address,
    data_type: e.dataType,
    byte_order: e.byteOrder,
  }));
}

/** 세그먼트(SegmentRow) → register_group. function_code 는 영역에서 유도한다. */
export function toEmitGroup(area: AreaKey, seg: SegmentRow): EmittedGroup {
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

/** DeviceRow → 백엔드 device. 유효 트랜스포트(per-device override ?? 에이전트 기본)가 rtu 면
 *  host/port 를 방출하지 않는다. per-device 오버라이드(transport/serial_port/share_session)는
 *  설정된 경우에만 방출하여 하위 호환(미설정 시 기존 형상과 바이트 동일)을 유지한다(SPEC-MODBUS-008). */
export function toEmitDevice(d: DeviceRow, agentTransport: string): EmittedDevice {
  const effTransport = d.transport !== '' ? d.transport : agentTransport;
  const isRtu = effTransport === 'rtu';
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
  // per-device transport 오버라이드(상속이 아니면 방출).
  if (d.transport !== '') out.transport = d.transport;
  // per-device RTU 오버라이드의 시리얼 포트(transport==='rtu' 이고 값이 있으면 방출).
  if (d.transport === 'rtu' && d.serialPort.trim() !== '') out.serial_port = d.serialPort.trim();
  // per-device share_session 오버라이드(상속이 아니면 방출).
  if (d.shareSession === 'true') out.share_session = true;
  else if (d.shareSession === 'false') out.share_session = false;
  return out;
}

export function toEmit(devices: DeviceRow[], transport: string): EmittedDevice[] {
  return devices.map((d) => toEmitDevice(d, transport));
}

/** 디바이스의 총 세그먼트(register_group) 수. */
export function segmentCount(d: DeviceRow): number {
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
export function splitCells(line: string): string[] {
  const parts = line.includes('\t') ? line.split('\t') : line.split(',');
  return parts.map((c) => c.trim());
}

/** 0 이상 정수 문자열 여부. */
export function isNonNegInt(s: string): boolean {
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
