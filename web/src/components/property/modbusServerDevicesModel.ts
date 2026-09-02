// Modbus Server 디바이스 에디터의 순수 모델 계층 — 상수 / 행 타입 / 방출 타입 /
// 변환·방출 유틸 / 검증 / 일괄등록 파서.
//
// 컴포넌트 파일(ModbusServerDevicesEditor.tsx)에서 분리했다. 컴포넌트 파일이 컴포넌트만
// 내보내야 Fast Refresh 가 동작하기 때문이다(react-refresh/only-export-components).

import { MODBUS_DATA_TYPE_OPTIONS } from '@/config/agentSchemas';

// ---- 상수 ----

export const AREA_KEYS = [
  { key: 'coils', labelKey: 'property.register.areaCoils' },
  { key: 'discrete_inputs', labelKey: 'property.register.areaDiscreteInputs' },
  { key: 'holding_registers', labelKey: 'property.register.areaHoldingRegisters' },
  { key: 'input_registers', labelKey: 'property.register.areaInputRegisters' },
] as const;

export type AreaKey = (typeof AREA_KEYS)[number]['key'];

export const DEFAULT_DATA_TYPE = 'uint16';
export const CONTAINER_UNIT_ID = 0;

// 백킹(upstream) 설정 선택지 (SPEC-MODBUS-010). 백엔드 parseServerSerialConfig 와 동일 키·기본값.
export const BACKING_TRANSPORT_OPTIONS = ['tcp', 'rtu'] as const;
export const BACKING_BAUD_OPTIONS = ['1200', '2400', '4800', '9600', '19200', '38400', '57600', '115200'] as const;
export const BACKING_DATA_BITS_OPTIONS = ['5', '6', '7', '8'] as const;
export const BACKING_STOP_BITS_OPTIONS = ['1', '2'] as const;
export const BACKING_PARITY_OPTIONS = ['none', 'even', 'odd'] as const;

// ---- 내부 행 타입 (React 렌더링용 안정 key + UI 전용 상태 포함) ----

export interface SegmentRow {
  key: string;
  address: number;
  count: number;
  /** UI 전용: 공유 세그먼트 여부(방출값에는 shared_address 유무로 표현). */
  shared: boolean;
  /** local 전용. */
  dataType: string;
  /** shared 전용: 디바이스 0 내부의 기준 주소. */
  sharedAddress: number;
  /** 세그먼트 설명(선택, local/shared 공통). 비어있으면 방출에서 생략. */
  description: string;
  /** UI 미편집 통과 필드(라운드트립 보존, local 전용). */
  initialValues?: unknown;
  typeMap?: unknown;
}

/** 실제(upstream) 디바이스 백킹 설정 UI 상태 (SPEC-MODBUS-010 REQ-06).
 *  null 이면 순수 slave(백킹 없음) — 방출에서 backing 키를 완전히 생략한다(하위 호환). */
export interface BackingRow {
  /** 'tcp' | 'rtu'. */
  transport: string;
  /** TCP endpoint host. */
  host: string;
  /** TCP endpoint port. */
  port: number;
  /** RTU 시리얼 포트 경로. */
  serialPort: string;
  /** RTU 보 레이트(select value, 문자열; 방출 시 number 변환). */
  baudRate: string;
  /** RTU 데이터 비트(select value, 문자열; 방출 시 number 변환). */
  dataBits: string;
  /** RTU 스톱 비트(select value, 문자열; 방출 시 number 변환). */
  stopBits: string;
  /** RTU 패리티('none' | 'even' | 'odd'). */
  parity: string;
  /** upstream 디바이스 unit id (서빙 UnitID 와 독립). */
  unitId: number;
  /** 'direct' | 'indirect'. */
  mode: string;
  /** indirect 폴링 주기(duration 문자열, 예: '1s'). */
  pollInterval: string;
  /** upstream 요청 데드라인 + (indirect) stale 허용 한도(duration 문자열). */
  timeout: string;
}

export interface DeviceRow {
  key: string;
  unitId: number;
  name: string;
  areas: Record<AreaKey, SegmentRow[]>;
  /** upstream 백킹 설정(서빙 디바이스 전용, null = 순수 slave). */
  backing: BackingRow | null;
}

export interface EditorState {
  /** 디바이스 0 공유 맵 컨테이너(선택, 최대 1개). */
  container: DeviceRow | null;
  /** 서빙 디바이스(unit_id 1-247). */
  served: DeviceRow[];
}

// ---- 방출(백엔드) 타입 ----

export interface EmittedSegment {
  address: number;
  count: number;
  data_type?: string;
  shared_address?: number;
  description?: string;
  initial_values?: unknown[];
  type_map?: unknown[];
}

/** 방출(백엔드) 백킹 오브젝트. 백엔드 parseBackingConfig 키와 정확히 일치한다(SPEC-MODBUS-010).
 *  transport/mode/unit_id 는 항상, TCP 는 host/port, RTU 는 serial_port + 시리얼 파라미터,
 *  indirect 는 poll_interval/timeout 을 방출한다. */
export interface EmittedBacking {
  transport: string;
  mode: string;
  unit_id: number;
  host?: string;
  port?: number;
  serial_port?: string;
  baud_rate?: number;
  data_bits?: number;
  stop_bits?: number;
  parity?: string;
  poll_interval?: string;
  timeout?: string;
}

export interface EmittedDevice {
  unit_id: number;
  name?: string;
  register_map: Record<string, EmittedSegment[]>;
  backing?: EmittedBacking;
}

// ---- Props ----

export interface ModbusServerDevicesEditorProps {
  value: unknown;
  onChange: (value: unknown) => void;
  readOnly?: boolean;
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

/** 세그먼트 객체 → SegmentRow. shared_address 유무로 local/shared 를 판정한다.
 *  legacy start_address 키는 방어적으로 address 로 폴백한다. */
export function toSegmentRow(item: unknown): SegmentRow {
  const o = asObject(item);
  const isShared = o.shared_address !== undefined && o.shared_address !== null;
  return {
    key: nextKey('seg'),
    address: numOr(o.address ?? o.start_address, 0),
    count: numOr(o.count, 1),
    shared: isShared,
    dataType: asString(o.data_type) || DEFAULT_DATA_TYPE,
    sharedAddress: numOr(o.shared_address, 0),
    description: asString(o.description),
    initialValues: o.initial_values,
    typeMap: o.type_map,
  };
}

export function toAreas(registerMap: unknown): Record<AreaKey, SegmentRow[]> {
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

/** 백엔드 backing 오브젝트 → BackingRow. 키 부재/비객체이면 null(순수 slave). */
export function toBackingRow(raw: unknown): BackingRow | null {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null;
  const o = raw as Record<string, unknown>;
  const transport = asString(o.transport) === 'rtu' ? 'rtu' : 'tcp';
  const mode = asString(o.mode) === 'indirect' ? 'indirect' : 'direct';
  const parity = asString(o.parity);
  return {
    transport,
    host: asString(o.host),
    port: numOr(o.port, 502),
    serialPort: asString(o.serial_port),
    baudRate: String(numOr(o.baud_rate, 9600)),
    dataBits: String(numOr(o.data_bits, 8)),
    stopBits: String(numOr(o.stop_bits, 1)),
    parity: parity === 'even' || parity === 'odd' ? parity : 'none',
    unitId: numOr(o.unit_id, 1),
    mode,
    pollInterval: asString(o.poll_interval),
    timeout: asString(o.timeout),
  };
}

export function toDeviceRow(item: unknown): DeviceRow {
  const o = asObject(item);
  return {
    key: nextKey('sdev'),
    unitId: numOr(o.unit_id, 1),
    name: asString(o.name),
    areas: toAreas(o.register_map),
    backing: toBackingRow(o.backing),
  };
}

/** unknown(배열 | JSON 문자열 | 빈 값) → { container, served }. */
export function parseValue(value: unknown): EditorState {
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

export function newServedDevice(): DeviceRow {
  return { key: nextKey('sdev'), unitId: 1, name: '', areas: emptyAreas(), backing: null };
}

export function newContainer(): DeviceRow {
  return {
    key: nextKey('sdev'),
    unitId: CONTAINER_UNIT_ID,
    name: '',
    areas: emptyAreas(),
    backing: null,
  };
}

/** 백킹 활성화 시 기본값. direct+tcp, upstream unit 1, indirect 기본 주기/타임아웃(> 0)로 시작한다. */
export function newBacking(): BackingRow {
  return {
    transport: 'tcp',
    host: '',
    port: 502,
    serialPort: '',
    baudRate: '9600',
    dataBits: '8',
    stopBits: '1',
    parity: 'none',
    unitId: 1,
    mode: 'direct',
    pollInterval: '1s',
    timeout: '2s',
  };
}

export function newSegment(): SegmentRow {
  return {
    key: nextKey('seg'),
    address: 0,
    count: 1,
    shared: false,
    dataType: DEFAULT_DATA_TYPE,
    sharedAddress: 0,
    description: '',
  };
}

/** SegmentRow → 백엔드 세그먼트. 컨테이너(디바이스 0)는 항상 local 로 방출한다.
 *  shared: { address, count, shared_address } (data_type 등 생략)
 *  local:  { address, count, data_type, initial_values?, type_map? } */
export function toEmitSegment(s: SegmentRow, isContainer: boolean): EmittedSegment {
  const desc = s.description.trim();
  if (s.shared && !isContainer) {
    const out: EmittedSegment = {
      address: s.address,
      count: s.count,
      shared_address: s.sharedAddress,
    };
    if (desc !== '') out.description = desc;
    return out;
  }
  const out: EmittedSegment = {
    address: s.address,
    count: s.count,
    data_type: s.dataType || DEFAULT_DATA_TYPE,
  };
  if (desc !== '') out.description = desc;
  if (Array.isArray(s.initialValues)) out.initial_values = s.initialValues;
  if (Array.isArray(s.typeMap)) out.type_map = s.typeMap;
  return out;
}

/** BackingRow → 백엔드 backing 오브젝트. 백엔드 parseBackingConfig 가 기대하는 키를 정확히 방출한다:
 *  transport/mode/unit_id 항상; TCP=host(비어있지 않을 때)+port; RTU=serial_port(비어있지 않을 때)+
 *  baud_rate/data_bits/stop_bits/parity(number 변환 — 백엔드 toInt 는 문자열 미수용); indirect=poll_interval+timeout,
 *  direct=timeout(비어있지 않을 때만). 숫자 필드는 number 로 방출한다. */
export function toEmitBacking(b: BackingRow): EmittedBacking {
  const out: EmittedBacking = {
    transport: b.transport,
    mode: b.mode,
    unit_id: b.unitId,
  };
  if (b.transport === 'rtu') {
    if (b.serialPort.trim() !== '') out.serial_port = b.serialPort.trim();
    out.baud_rate = Number(b.baudRate);
    out.data_bits = Number(b.dataBits);
    out.stop_bits = Number(b.stopBits);
    out.parity = b.parity;
  } else {
    if (b.host.trim() !== '') out.host = b.host.trim();
    out.port = b.port;
  }
  if (b.mode === 'indirect') {
    // indirect 는 poll_interval/timeout 이 필수(> 0)이므로 항상 방출한다.
    out.poll_interval = b.pollInterval.trim();
    out.timeout = b.timeout.trim();
  } else if (b.timeout.trim() !== '') {
    // direct 의 timeout 은 선택(upstream 요청 데드라인).
    out.timeout = b.timeout.trim();
  }
  return out;
}

export function toEmitDevice(d: DeviceRow, isContainer: boolean): EmittedDevice {
  const register_map: Record<string, EmittedSegment[]> = {};
  for (const area of AREA_KEYS) {
    const rows = d.areas[area.key];
    if (!rows || rows.length === 0) continue;
    register_map[area.key] = rows.map((s) => toEmitSegment(s, isContainer));
  }
  const name = d.name.trim();
  const base: EmittedDevice =
    name !== ''
      ? { unit_id: d.unitId, name, register_map }
      : { unit_id: d.unitId, register_map };
  // 백킹은 서빙 디바이스 전용. 미설정(null)이면 backing 키를 완전히 생략한다 → 순수 slave(하위 호환).
  if (!isContainer && d.backing) {
    base.backing = toEmitBacking(d.backing);
  }
  return base;
}

/** { container, served } → 백엔드 devices 배열 (컨테이너 먼저). */
export function toEmit(state: EditorState): EmittedDevice[] {
  const out: EmittedDevice[] = [];
  if (state.container) out.push(toEmitDevice(state.container, true));
  for (const d of state.served) out.push(toEmitDevice(d, false));
  return out;
}

/** 디바이스의 총 세그먼트 수(방출 유효성 시각 힌트용). */
export function segmentCount(d: DeviceRow): number {
  return AREA_KEYS.reduce((n, a) => n + d.areas[a.key].length, 0);
}

/**
 * 방출된 devices 값(배열 | JSON 문자열 | 빈 값)이 모두 유효한지 검사한다(부모 저장 게이팅용).
 *
 * 백엔드 parseRegisterMapConfig(internal/agent/modbusserver/config.go:667)의 규칙과 정확히
 * 일치한다: 모든 디바이스(공유 컨테이너 unit 0 + 서빙 디바이스)의 register_map 은 최소 1개
 * 영역(비어있지 않은 세그먼트 배열)을 가져야 한다. 세그먼트가 0개인 디바이스가 하나라도 있으면
 * 백엔드가 자동 재시작 시 `register_map must have at least one area`(ErrInvalidRegisterMap)로
 * 거부하므로, 프론트에서 저장을 차단해 잘못된 config 가 백엔드에 도달하지 못하게 한다.
 *
 * 빈 devices 배열(디바이스 없음)은 유효하다 — 백엔드는 devices 없는 서버 생성을 허용한다.
 * 공유 세그먼트도 영역 행이므로 segmentCount 에 포함된다.
 */
export function modbusServerDevicesValid(value: unknown): boolean {
  const { container, served } = parseValue(value);
  const all = container ? [container, ...served] : served;
  return all.every((d) => segmentCount(d) > 0);
}

/** 한 영역 내 device-local 주소 범위 겹침 여부(시각 힌트용). */
export function hasOverlap(rows: SegmentRow[]): boolean {
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

// ---- 일괄등록(bulk paste) 파서 ----

/** fc(function code) → 영역 매핑. 1=코일 2=이산입력 3=보유레지스터 4=입력레지스터. */
export const FC_TO_AREA: Record<string, AreaKey> = {
  '1': 'coils',
  '2': 'discrete_inputs',
  '3': 'holding_registers',
  '4': 'input_registers',
};

/** 파싱된 세그먼트(순수 데이터, React key 없음). fc 로부터 유도된 area 태그 포함. */
export interface BulkSegment {
  area: AreaKey;
  address: number;
  count: number;
  shared: boolean;
  dataType: string;
  sharedAddress: number;
  description: string;
}

/** 파싱 오류. line 은 원본 1-based 줄 번호, code 는 i18n bulkError.<code> 키. */
export interface BulkParseError {
  line: number;
  code: string;
}

export interface BulkParseResult {
  segments: BulkSegment[];
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
 * 붙여넣기 텍스트 → fc 기반 세그먼트 파싱(순수 함수, 단위 테스트 대상).
 *
 * 한 줄 = 한 세그먼트. 콤마 또는 탭 구분(자동 감지), 셀 트림.
 * 컬럼 개수로 로컬/공유를 구분한다:
 *  - 5열 = LOCAL:  `fc, address, count, data_type, description`
 *  - 7열 = SHARED: `fc, address, count, data_type, shared, shared_address, description`
 *    (공유 형식의 data_type/shared 셀은 존재하지만 방출 시 data_type 은 컨테이너에서
 *     상속되어 생략된다. shared 셀 값은 flag 로서 의미가 강제되지 않는다.)
 *  - 그 외 열 개수 → wrongColumnCount 오류.
 *  - fc 1-4 → coils/discrete_inputs/holding_registers/input_registers, 그 외 → invalidFc.
 *  - address/count/shared_address 는 0 이상 정수(count ≥ 1). data_type 은 로컬에서
 *    MODBUS_DATA_TYPE_OPTIONS 중 하나(빈 셀이면 uint16). description 은 마지막 열, 자유 텍스트(선택).
 *  - 빈 줄 무시. 첫 non-empty 줄의 첫 셀(fc)이 숫자가 아니면 헤더로 보고 무시.
 *  - allowShared=false(컨테이너)에서 7열(공유) 줄은 containerNoShared 오류.
 *
 * 오류가 있어도 유효한 세그먼트는 그대로 담아 반환한다(호출부가 block-on-error 정책 적용).
 */
export function parseBulkSegments(
  text: string,
  allowShared: boolean,
): BulkParseResult {
  const segments: BulkSegment[] = [];
  const errors: BulkParseError[] = [];
  const lines = text.split(/\r?\n/);
  let firstNonEmptySeen = false;

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]!.trim();
    if (line === '') continue;

    const cells = splitCells(line);

    // 첫 non-empty 줄의 첫 셀(fc)이 비숫자면 헤더로 보고 스킵.
    if (!firstNonEmptySeen) {
      firstNonEmptySeen = true;
      if (!isNonNegInt(cells[0] ?? '')) continue;
    }

    const lineNo = i + 1;
    const n = cells.length;
    if (n !== 5 && n !== 7) {
      errors.push({ line: lineNo, code: 'wrongColumnCount' });
      continue;
    }

    const area = FC_TO_AREA[cells[0] ?? ''];
    if (!area) {
      errors.push({ line: lineNo, code: 'invalidFc' });
      continue;
    }

    const isShared = n === 7;
    if (isShared && !allowShared) {
      errors.push({ line: lineNo, code: 'containerNoShared' });
      continue;
    }

    const addrCell = cells[1] ?? '';
    const countCell = cells[2] ?? '';
    const dtCell = cells[3] ?? '';

    if (!isNonNegInt(addrCell)) {
      errors.push({ line: lineNo, code: 'invalidAddress' });
      continue;
    }
    if (!isNonNegInt(countCell) || Number(countCell) < 1) {
      errors.push({ line: lineNo, code: 'invalidCount' });
      continue;
    }

    if (isShared) {
      // 7열: fc, address, count, data_type, shared, shared_address, description
      const sharedAddrCell = cells[5] ?? '';
      const description = cells[6] ?? '';
      if (!isNonNegInt(sharedAddrCell)) {
        errors.push({ line: lineNo, code: 'invalidSharedAddress' });
        continue;
      }
      segments.push({
        area,
        address: Number(addrCell),
        count: Number(countCell),
        shared: true,
        dataType: DEFAULT_DATA_TYPE,
        sharedAddress: Number(sharedAddrCell),
        description,
      });
    } else {
      // 5열: fc, address, count, data_type, description
      const description = cells[4] ?? '';
      const dataType = dtCell === '' ? DEFAULT_DATA_TYPE : dtCell;
      if (!(MODBUS_DATA_TYPE_OPTIONS as readonly string[]).includes(dataType)) {
        errors.push({ line: lineNo, code: 'invalidDataType' });
        continue;
      }
      segments.push({
        area,
        address: Number(addrCell),
        count: Number(countCell),
        shared: false,
        dataType,
        sharedAddress: 0,
        description,
      });
    }
  }

  return { segments, errors };
}
