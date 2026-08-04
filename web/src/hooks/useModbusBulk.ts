// MODBUS Client·Gateway 디바이스 일괄 등록 (SPEC-MODBUS-011).
//
// 붙여넣기 텍스트/CSV → 순수 파서(파싱·검증) + best-effort 실행 훅(디바이스별 add_device 순차 호출).
// 백엔드는 기존 `add_device` exec 를 디바이스별로 반복 호출하므로 Go 코드 변경이 없다.
//
//   - 파서는 순수 함수(부수효과 없음)로, 유효 행은 디바이스 방출 형상으로 그룹핑하고,
//     무효 행은 원본 1-based 줄 번호를 가진 BulkFailure 로 수집한다(throw 하지 않음).
//   - 실행 훅은 파서의 유효 디바이스만 백엔드로 보내고, 파서 실패 + 백엔드 실패를 단일
//     BulkFailure[] 로 합류한다(부분 성공 허용, 원자적 롤백 없음).
//
// 포맷(v0.2.0): 한 줄 = 레지스터 그룹/세그먼트 하나. 신원 컬럼(host/port/unit_id · unit_id/name)이
// 채워진 행은 새 디바이스를 시작하고, 신원 컬럼이 모두 빈 행은 직전 디바이스에 그룹/세그먼트를
// 이어 붙인다. 셀 구분은 xsfm 선례(parseDelimitedRows)를 재사용한다: 콤마 또는 탭 자동 감지(탭 우선),
// 셀 trim, 빈 줄 무시, 1-based 줄 번호 보존.

import { useMutation } from '@tanstack/react-query';

import * as agentService from '@/services/api/agentService';
import { MODBUS_DATA_TYPE_OPTIONS } from '@/config/agentSchemas';
import {
  EMPTY_REQUIRED,
  parseDelimitedRows,
  type BulkFailure,
  type BulkResult,
  type ParsedRow,
} from '@/hooks/useStation';

// ---- 실패 사유 sentinel (컴포넌트에서 i18n 으로 치환) ----

/** unit_id 가 비정수이거나 1-247 범위를 벗어남. */
export const INVALID_UNIT_ID = 'INVALID_UNIT_ID';

/** port 가 비정수. */
export const INVALID_PORT = 'INVALID_PORT';

/** fc(function_code) 가 1-4 가 아님. */
export const INVALID_FC = 'INVALID_FC';

/** client register_group 서브 필드(start/qty/data_type)가 무효. */
export const INVALID_GROUP = 'INVALID_GROUP';

/** gateway segment 서브 필드(start/count/data_type)가 무효. */
export const INVALID_SEGMENT = 'INVALID_SEGMENT';

/** shared 마커가 있으나 shared_address 가 비정수/누락. */
export const INVALID_SHARED_ADDRESS = 'INVALID_SHARED_ADDRESS';

/** 신원 컬럼이 빈 그룹/세그먼트 행이나 시작할 디바이스가 아직 없음(선행 이어붙임 행). */
export const NO_CURRENT_DEVICE = 'NO_CURRENT_DEVICE';

/** gateway 디바이스가 유효 세그먼트 0개로 종료(register_map 백엔드 필수 위반). */
export const EMPTY_SEGMENTS = 'EMPTY_SEGMENTS';

const DEFAULT_DATA_TYPE = 'uint16';

/** gateway shared 세그먼트를 표시하는 리터럴 토큰(data_type 셀 다음, 대소문자 무시). */
const SHARED_MARKER = 'shared';

// ---- 방출(백엔드) 형상 ----

/** client register_group 방출 형상(ModbusDevicesEditor EmittedGroup 계약). */
export interface ClientBulkGroup {
  function_code: number;
  start_address: number;
  quantity: number;
  /** 공란이면 방출 생략(백엔드 uint16 기본). */
  data_type?: string;
  /** Go duration 자유 텍스트(검증 안 함). 공란이면 방출 생략. */
  poll_interval?: string;
  /** comment 컬럼 → 그룹 이름. 공란이면 방출 생략. */
  name?: string;
}

/** client 디바이스 방출 형상(ModbusDevicesEditor EmittedDevice 계약, per-device 오버라이드 제외). */
export interface ClientBulkDevice {
  host?: string;
  port?: number;
  unit_id: number;
  register_groups: ClientBulkGroup[];
}

/**
 * gateway 세그먼트 방출 형상(ModbusServerDevicesEditor EmittedSegment 계약).
 * local 세그먼트는 `data_type`(기본 uint16)을, shared 세그먼트는 `shared_address`를 방출한다.
 */
export interface GatewaySegment {
  address: number;
  count: number;
  /** local 전용(shared 이면 생략). 공란이면 uint16. */
  data_type?: string;
  /** shared 전용(local 이면 생략). 디바이스 0 내부의 기준 주소. */
  shared_address?: number;
  /** comment 컬럼 → 세그먼트 설명. 공란이면 방출 생략. */
  description?: string;
}

/** gateway 디바이스 방출 형상(processAddDevice params 계약). */
export interface GatewayBulkDevice {
  unit_id: number;
  name?: string;
  register_map: Record<string, GatewaySegment[]>;
}

/** 파서 결과. devices 는 깨끗한 방출 형상, failures 는 행별 파싱 실패. */
export interface ClientBulkParse {
  devices: ClientBulkDevice[];
  failures: BulkFailure[];
}

export interface GatewayBulkParse {
  devices: GatewayBulkDevice[];
  failures: BulkFailure[];
}

// ---- 내부 리치 행(실행 훅이 백엔드 실패에 원본 줄 번호를 부여하기 위해 line 을 보존) ----

interface ClientRow {
  line: number;
  input: string;
  device: ClientBulkDevice;
}

interface GatewayRow {
  line: number;
  input: string;
  device: GatewayBulkDevice;
}

// ---- 검증 유틸 ----

/** 0 이상 정수 문자열 여부. */
function isNonNegInt(s: string): boolean {
  return /^\d+$/.test(s);
}

/** function_code 1-4 여부. */
function isFc(s: string): boolean {
  return /^[1-4]$/.test(s);
}

/** data_type 이 옵션 목록에 속하는지. */
function isValidDataType(s: string): boolean {
  return (MODBUS_DATA_TYPE_OPTIONS as readonly string[]).includes(s);
}

/** fc(1-4) → gateway 정규 영역명. */
const FC_TO_AREA: Record<string, string> = {
  '1': 'coils',
  '2': 'discrete_inputs',
  '3': 'holding_registers',
  '4': 'input_registers',
};

// ---- 셀 그룹/세그먼트 파서 ----
//
// 각 파서는 세 결과 중 하나를 돌려준다:
//   { skip: true }  — 신원 행인데 그룹/세그먼트 셀이 비어있음(디바이스만 선언, 인라인 그룹 없음).
//   { fail }        — 그룹/세그먼트 셀이 무효(해당 행 실패 사유).
//   { seg }         — 유효 그룹/세그먼트.

type GroupParse = { seg: ClientBulkGroup } | { fail: string } | { skip: true };
type SegmentParse = { seg: { area: string; segment: GatewaySegment } } | { fail: string } | { skip: true };

/**
 * client 그룹 셀 파싱. 컬럼: fc(3), address(4), count(5), data_type(6), polling_interval(7), comment(8).
 * 신원 행에서 fc 가 비면 인라인 그룹 없음(skip). 이어붙임 행에서 fc 가 비면 필수 누락(fail).
 */
function parseClientGroup(cells: string[], isIdentity: boolean): GroupParse {
  const fcRaw = (cells[3] ?? '').trim();
  const addrRaw = (cells[4] ?? '').trim();
  const cntRaw = (cells[5] ?? '').trim();
  const dtRaw = (cells[6] ?? '').trim();
  const pollRaw = (cells[7] ?? '').trim();
  const comment = (cells[8] ?? '').trim();

  if (fcRaw === '') return isIdentity ? { skip: true } : { fail: EMPTY_REQUIRED };
  if (!isFc(fcRaw)) return { fail: INVALID_FC };
  if (addrRaw === '' || cntRaw === '') return { fail: EMPTY_REQUIRED };
  if (!isNonNegInt(addrRaw)) return { fail: INVALID_GROUP };
  if (!isNonNegInt(cntRaw) || Number(cntRaw) < 1) return { fail: INVALID_GROUP };

  const group: ClientBulkGroup = {
    function_code: Number(fcRaw),
    start_address: Number(addrRaw),
    quantity: Number(cntRaw),
  };
  if (dtRaw !== '') {
    if (!isValidDataType(dtRaw)) return { fail: INVALID_GROUP };
    group.data_type = dtRaw; // 공란이면 방출 생략(백엔드 uint16 기본).
  }
  if (pollRaw !== '') group.poll_interval = pollRaw;
  if (comment !== '') group.name = comment;
  return { seg: group };
}

/**
 * gateway 세그먼트 셀 파싱. 컬럼: fc(2), address(3), count(4), data_type(5), [shared(6), shared_address(7),] comment.
 * data_type 셀 다음이 리터럴 `shared`(대소문자 무시)면 shared 세그먼트(shared_address 필수, data_type 미방출),
 * 아니면 local 세그먼트(data_type 기본 uint16, 그 셀이 comment). area 는 fc 로 결정한다.
 * 신원 행에서 fc 가 비면 인라인 세그먼트 없음(skip). 이어붙임 행에서 fc 가 비면 필수 누락(fail).
 */
function parseGatewaySegment(cells: string[], isIdentity: boolean): SegmentParse {
  const fcRaw = (cells[2] ?? '').trim();
  const addrRaw = (cells[3] ?? '').trim();
  const cntRaw = (cells[4] ?? '').trim();
  const dtRaw = (cells[5] ?? '').trim();

  if (fcRaw === '') return isIdentity ? { skip: true } : { fail: EMPTY_REQUIRED };
  if (!isFc(fcRaw)) return { fail: INVALID_FC };
  const area = FC_TO_AREA[fcRaw]!; // fc 는 1-4 로 검증됨.
  if (addrRaw === '' || cntRaw === '') return { fail: EMPTY_REQUIRED };
  if (!isNonNegInt(addrRaw)) return { fail: INVALID_SEGMENT };
  if (!isNonNegInt(cntRaw) || Number(cntRaw) < 1) return { fail: INVALID_SEGMENT };
  const address = Number(addrRaw);
  const count = Number(cntRaw);

  // shared 감지: data_type 셀 다음 셀이 리터럴 `shared`.
  const nextCell = (cells[6] ?? '').trim();
  if (nextCell.toLowerCase() === SHARED_MARKER) {
    const sharedRaw = (cells[7] ?? '').trim();
    const comment = (cells[8] ?? '').trim();
    if (!isNonNegInt(sharedRaw)) return { fail: INVALID_SHARED_ADDRESS };
    const segment: GatewaySegment = { address, count, shared_address: Number(sharedRaw) };
    if (comment !== '') segment.description = comment;
    return { seg: { area, segment } };
  }

  // local: nextCell(6) 이 comment.
  const comment = nextCell;
  let dataType = DEFAULT_DATA_TYPE;
  if (dtRaw !== '') {
    if (!isValidDataType(dtRaw)) return { fail: INVALID_SEGMENT };
    dataType = dtRaw;
  }
  const segment: GatewaySegment = { address, count, data_type: dataType };
  if (comment !== '') segment.description = comment;
  return { seg: { area, segment } };
}

// ---- 헤더 판정 ----

/** 첫 non-empty 행의 신원 셀이 비어있지 않은 비정수(예: `unit_id`)면 헤더로 간주. */
function isHeaderRow(unitCell: string): boolean {
  return unitCell !== '' && !isNonNegInt(unitCell);
}

// ---- 행 파서(내부 — 그룹핑 + line 보존) ----

/**
 * client 붙여넣기 → 리치 행 + 실패. 신원 컬럼(host/port/unit_id)이 채워진 행이 새 디바이스를 시작하고,
 * 모두 빈 행은 직전 디바이스에 register_group 을 이어 붙인다. transport 상속이 rtu 면 host 를
 * 요구하지 않고 방출하지 않는다. 유효 그룹 0개인 디바이스는 방출하지 않는다(행별 실패로 집계).
 */
function parseClientRows(text: string, transport: string): { rows: ClientRow[]; failures: BulkFailure[] } {
  const parsed = parseDelimitedRows(text);
  const rows: ClientRow[] = [];
  const failures: BulkFailure[] = [];
  const isRtu = transport === 'rtu';
  let current: ClientRow | null = null;

  // 직전 디바이스를 devices 로 확정. 새 포맷은 "한 행 = 한 그룹"이므로 유효 그룹 0개
  // (신원만 있고 그룹 셀이 비었거나 모두 무효)인 디바이스는 방출하지 않는다(행별 실패는 이미 집계됨).
  const finalize = () => {
    if (current) {
      if (current.device.register_groups.length > 0) rows.push(current);
      current = null;
    }
  };

  const appendGroup = (r: ParsedRow, isIdentity: boolean, dev: ClientBulkDevice) => {
    const res = parseClientGroup(r.cells, isIdentity);
    if ('skip' in res) return;
    if ('fail' in res) {
      failures.push({ line: r.line, input: r.raw, reason: res.fail });
      return;
    }
    dev.register_groups.push(res.seg);
  };

  let first = true;
  for (const r of parsed) {
    const host = (r.cells[0] ?? '').trim();
    const portRaw = (r.cells[1] ?? '').trim();
    const unitRaw = (r.cells[2] ?? '').trim();

    if (first) {
      first = false;
      if (isHeaderRow(unitRaw)) continue; // 헤더 스킵
    }

    const isIdentity = host !== '' || portRaw !== '' || unitRaw !== '';
    if (isIdentity) {
      finalize(); // 직전 디바이스 확정 후 새 디바이스 시작.
      if (!isRtu && host === '') {
        failures.push({ line: r.line, input: r.raw, reason: EMPTY_REQUIRED });
        continue;
      }
      if (!isNonNegInt(unitRaw)) {
        failures.push({ line: r.line, input: r.raw, reason: unitRaw === '' ? EMPTY_REQUIRED : INVALID_UNIT_ID });
        continue;
      }
      const unitId = Number(unitRaw);
      if (unitId < 1 || unitId > 247) {
        failures.push({ line: r.line, input: r.raw, reason: INVALID_UNIT_ID });
        continue;
      }
      let port: number | undefined;
      if (portRaw !== '') {
        if (!isNonNegInt(portRaw)) {
          failures.push({ line: r.line, input: r.raw, reason: INVALID_PORT });
          continue;
        }
        port = Number(portRaw);
      }
      const device: ClientBulkDevice = { unit_id: unitId, register_groups: [] };
      if (!isRtu && host !== '') device.host = host;
      if (port !== undefined) device.port = port; // 공란이면 생략(백엔드 502 기본).
      current = { line: r.line, input: r.raw, device };
      appendGroup(r, true, device); // 신원 행의 인라인 그룹.
      continue;
    }

    // 이어붙임 행: 현재 디바이스에 그룹 추가.
    if (!current) {
      failures.push({ line: r.line, input: r.raw, reason: NO_CURRENT_DEVICE });
      continue;
    }
    appendGroup(r, false, current.device);
  }
  finalize();
  return { rows, failures };
}

/**
 * gateway 붙여넣기 → 리치 행 + 실패. 신원 컬럼(unit_id/name)이 채워진 행이 새 디바이스를 시작하고,
 * 모두 빈 행은 직전 디바이스에 세그먼트를 이어 붙인다. register_map 은 백엔드 필수이므로 유효
 * 세그먼트 0개로 끝난 디바이스는 EMPTY_SEGMENTS 로 사전 실패한다(단, 이미 세그먼트 실패가 있으면
 * 중복 집계하지 않음).
 */
function parseGatewayRows(text: string): { rows: GatewayRow[]; failures: BulkFailure[] } {
  const parsed = parseDelimitedRows(text);
  const rows: GatewayRow[] = [];
  const failures: BulkFailure[] = [];
  // segCount: 유효 세그먼트 수, failed: 해당 디바이스에서 세그먼트 실패가 있었는지(중복 EMPTY_SEGMENTS 방지).
  let current: { line: number; input: string; device: GatewayBulkDevice; segCount: number; failed: boolean } | null =
    null;

  const finalize = () => {
    if (!current) return;
    if (current.segCount === 0) {
      // 유효 세그먼트가 없으면 디바이스 미방출. 세그먼트 실패가 없었을 때만 EMPTY_SEGMENTS 집계.
      if (!current.failed) {
        failures.push({ line: current.line, input: current.input, reason: EMPTY_SEGMENTS });
      }
    } else {
      rows.push({ line: current.line, input: current.input, device: current.device });
    }
    current = null;
  };

  const appendSegment = (r: ParsedRow, isIdentity: boolean, cur: NonNullable<typeof current>) => {
    const res = parseGatewaySegment(r.cells, isIdentity);
    if ('skip' in res) return;
    if ('fail' in res) {
      failures.push({ line: r.line, input: r.raw, reason: res.fail });
      cur.failed = true;
      return;
    }
    const { area, segment } = res.seg;
    (cur.device.register_map[area] ??= []).push(segment);
    cur.segCount += 1;
  };

  let first = true;
  for (const r of parsed) {
    const unitRaw = (r.cells[0] ?? '').trim();
    const name = (r.cells[1] ?? '').trim();

    if (first) {
      first = false;
      if (isHeaderRow(unitRaw)) continue; // 헤더 스킵
    }

    const isIdentity = unitRaw !== '' || name !== '';
    if (isIdentity) {
      finalize(); // 직전 디바이스 확정 후 새 디바이스 시작.
      if (!isNonNegInt(unitRaw)) {
        failures.push({ line: r.line, input: r.raw, reason: unitRaw === '' ? EMPTY_REQUIRED : INVALID_UNIT_ID });
        continue;
      }
      const unitId = Number(unitRaw);
      if (unitId < 1 || unitId > 247) {
        failures.push({ line: r.line, input: r.raw, reason: INVALID_UNIT_ID });
        continue;
      }
      const device: GatewayBulkDevice = { unit_id: unitId, register_map: {} };
      if (name !== '') device.name = name;
      current = { line: r.line, input: r.raw, device, segCount: 0, failed: false };
      appendSegment(r, true, current); // 신원 행의 인라인 세그먼트.
      continue;
    }

    // 이어붙임 행: 현재 디바이스에 세그먼트 추가.
    if (!current) {
      failures.push({ line: r.line, input: r.raw, reason: NO_CURRENT_DEVICE });
      continue;
    }
    appendSegment(r, false, current);
  }
  finalize();
  return { rows, failures };
}

// ---- 공개 순수 파서 ----

/**
 * client 붙여넣기 텍스트 → { devices, failures }(순수 함수, 단위 테스트 대상).
 * 컬럼: `host, port, unit_id, fc, address, count, data_type, polling_interval, comment`
 * (콤마 또는 탭 자동 감지). 신원 컬럼(host/port/unit_id)이 채워진 행이 디바이스를 시작하고,
 * 빈 신원 행은 직전 디바이스에 register_group 을 이어 붙인다. host 는 tcp 상속 시 필수.
 */
export function parseModbusClientBulk(text: string, transport: string = 'tcp'): ClientBulkParse {
  const { rows, failures } = parseClientRows(text, transport);
  return { devices: rows.map((r) => r.device), failures };
}

/**
 * gateway 붙여넣기 텍스트 → { devices, failures }(순수 함수, 단위 테스트 대상).
 * 컬럼: `unit_id, name, fc, address, count, data_type, comment`. shared 세그먼트는 data_type 다음에
 * `shared, <shared_address>` 두 셀을 삽입한다. 신원 컬럼(unit_id/name)이 채워진 행이 디바이스를
 * 시작하고, 빈 신원 행은 직전 디바이스에 세그먼트를 이어 붙인다. 디바이스마다 세그먼트 최소 1개 필수.
 */
export function parseModbusGatewayBulk(text: string): GatewayBulkParse {
  const { rows, failures } = parseGatewayRows(text);
  return { devices: rows.map((r) => r.device), failures };
}

// ---- best-effort 실행 훅 ----

/** 실패 목록을 줄 번호 오름차순으로 정렬(파서 실패 + 백엔드 실패 합류 후 표시 일관성). */
function sortByLine(failures: BulkFailure[]): BulkFailure[] {
  return [...failures].sort((a, b) => a.line - b.line);
}

/**
 * client 일괄 등록. 붙여넣은 텍스트를 파싱해 각 유효 디바이스마다 add_device 를 순차 호출한다.
 * 개별 디바이스 실패(중복 ID·백엔드 검증 실패 등)에도 중단하지 않고 나머지를 계속 등록한다.
 * 파서 실패 + 백엔드 실패를 단일 BulkFailure[] 로 합류하고, total 은 디바이스 수 + 파서 실패 수이다.
 * 목록 갱신은 호출부(섹션)가 mutateAsync 해결 후 1회만 수행한다(client 는 로컬 state + fetchDevices).
 */
export function useModbusClientBulkAdd(agentId: string) {
  return useMutation({
    mutationFn: async ({ text, transport }: { text: string; transport: string }): Promise<BulkResult> => {
      const { rows, failures } = parseClientRows(text, transport);
      const merged: BulkFailure[] = [...failures];
      let ok = 0;
      for (const row of rows) {
        try {
          await agentService.execAgent(agentId, { command: 'add_device', params: { ...row.device } });
          ok += 1;
        } catch (e) {
          merged.push({ line: row.line, input: row.input, reason: e instanceof Error ? e.message : EMPTY_REQUIRED });
        }
      }
      return { total: rows.length + failures.length, ok, failed: sortByLine(merged) };
    },
  });
}

/**
 * gateway 일괄 등록. client 와 동일한 best-effort 구조. 각 유효 디바이스마다 add_device 를 순차 호출한다.
 * register_map 이 필수이므로 유효 세그먼트 없는 디바이스는 파서가 사전 실패(EMPTY_SEGMENTS)로 집계한다.
 */
export function useModbusGatewayBulkAdd(agentId: string) {
  return useMutation({
    mutationFn: async (text: string): Promise<BulkResult> => {
      const { rows, failures } = parseGatewayRows(text);
      const merged: BulkFailure[] = [...failures];
      let ok = 0;
      for (const row of rows) {
        try {
          await agentService.execAgent(agentId, { command: 'add_device', params: { ...row.device } });
          ok += 1;
        } catch (e) {
          merged.push({ line: row.line, input: row.input, reason: e instanceof Error ? e.message : EMPTY_REQUIRED });
        }
      }
      return { total: rows.length + failures.length, ok, failed: sortByLine(merged) };
    },
  });
}
