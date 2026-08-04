// MODBUS Client·Gateway 디바이스 일괄 등록 (SPEC-MODBUS-011).
//
// 붙여넣기 텍스트/CSV → 순수 파서(파싱·검증) + best-effort 실행 훅(행별 add_device 순차 호출).
// 백엔드는 기존 `add_device` exec 를 행별로 반복 호출하므로 Go 코드 변경이 없다.
//
//   - 파서는 순수 함수(부수효과 없음)로, 유효 행은 방출 형상으로, 무효 행은 원본 1-based
//     줄 번호를 가진 BulkFailure 로 수집한다(throw 하지 않음).
//   - 실행 훅은 파서의 유효 행만 백엔드로 보내고, 파서 실패 + 백엔드 실패를 단일
//     BulkFailure[] 로 합류한다(부분 성공 허용, 원자적 롤백 없음).
//
// 셀 구분은 xsfm 선례(parseDelimitedRows)를 재사용한다: 콤마 또는 탭 자동 감지(탭 우선),
// 셀 trim, 빈 줄 무시, 1-based 줄 번호 보존. 그룹/세그먼트 서브 구분자(`;`/`:`)는 행 구분자
// (콤마/탭)와 절대 충돌하지 않으므로, 셀 분해 후 groups/segments 셀만 서브 파싱한다.

import { useMutation } from '@tanstack/react-query';

import * as agentService from '@/services/api/agentService';
import { MODBUS_DATA_TYPE_OPTIONS } from '@/config/agentSchemas';
import {
  EMPTY_REQUIRED,
  parseDelimitedRows,
  type BulkFailure,
  type BulkResult,
} from '@/hooks/useStation';

// ---- 실패 사유 sentinel (컴포넌트에서 i18n 으로 치환) ----

/** unit_id 가 비정수이거나 1-247 범위를 벗어남. */
export const INVALID_UNIT_ID = 'INVALID_UNIT_ID';

/** client register_groups 서브 필드(fc/start/qty/data_type)가 무효. */
export const INVALID_GROUP = 'INVALID_GROUP';

/** port 가 비정수. */
export const INVALID_PORT = 'INVALID_PORT';

/** gateway segments 서브 필드(area/start/count/data_type)가 무효. */
export const INVALID_SEGMENT = 'INVALID_SEGMENT';

/** gateway 세그먼트 누락(register_map 백엔드 필수 위반). */
export const EMPTY_SEGMENTS = 'EMPTY_SEGMENTS';

const DEFAULT_DATA_TYPE = 'uint16';

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
}

/** client 디바이스 방출 형상(ModbusDevicesEditor EmittedDevice 계약, per-device 오버라이드 제외). */
export interface ClientBulkDevice {
  id?: string;
  host?: string;
  port?: number;
  unit_id: number;
  register_groups: ClientBulkGroup[];
}

/** gateway local 세그먼트 방출 형상(shared_address 제외 — 초기 범위 local 전용). */
export interface GatewaySegment {
  address: number;
  count: number;
  data_type: string;
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

/** function_code(또는 gateway 숫자 area) 1-4 여부. */
function isFc(s: string): boolean {
  return /^[1-4]$/.test(s);
}

/** data_type 이 옵션 목록에 속하는지. */
function isValidDataType(s: string): boolean {
  return (MODBUS_DATA_TYPE_OPTIONS as readonly string[]).includes(s);
}

/** gateway area 토큰(1-4 또는 영역명) → 정규 영역명. 무효면 null. */
const GATEWAY_AREA_ALIASES: Record<string, string> = {
  '1': 'coils',
  '2': 'discrete_inputs',
  '3': 'holding_registers',
  '4': 'input_registers',
  coils: 'coils',
  discrete_inputs: 'discrete_inputs',
  holding_registers: 'holding_registers',
  input_registers: 'input_registers',
};

// ---- 서브 파서(그룹 / 세그먼트) ----

/**
 * client register_groups 셀 파싱. `;`로 그룹 구분, 각 그룹 = `fc:start:qty[:data_type[:poll]]`.
 * 공란이면 빈 배열(그룹 없음, 이후 개별 편집). 무효 토큰이 하나라도 있으면 null(행 실패).
 */
function parseClientGroups(raw: string): ClientBulkGroup[] | null {
  if (raw === '') return [];
  const groups: ClientBulkGroup[] = [];
  for (const chunk of raw.split(';')) {
    const g = chunk.trim();
    if (g === '') continue;
    const toks = g.split(':').map((s) => s.trim());
    if (toks.length < 3 || toks.length > 5) return null;
    const fcRaw = toks[0] ?? '';
    const startRaw = toks[1] ?? '';
    const qtyRaw = toks[2] ?? '';
    const dtRaw = toks[3] ?? '';
    const pollRaw = toks[4] ?? '';
    if (!isFc(fcRaw)) return null;
    if (!isNonNegInt(startRaw)) return null;
    if (!isNonNegInt(qtyRaw) || Number(qtyRaw) < 1) return null;
    const group: ClientBulkGroup = {
      function_code: Number(fcRaw),
      start_address: Number(startRaw),
      quantity: Number(qtyRaw),
    };
    if (dtRaw !== '') {
      if (!isValidDataType(dtRaw)) return null;
      group.data_type = dtRaw;
    }
    if (pollRaw !== '') group.poll_interval = pollRaw;
    groups.push(group);
  }
  return groups;
}

/**
 * gateway segments 셀 파싱. `;`로 세그먼트 구분, 각 세그먼트 = `area:start:count[:data_type]`.
 * local 세그먼트만 방출(data_type 항상 방출, 공란이면 uint16). 유효 세그먼트가 0개이거나
 * 무효 토큰이 있으면 null(행 실패 — 백엔드 register_map 필수 위반 사전 차단).
 */
function parseGatewaySegments(raw: string): Record<string, GatewaySegment[]> | null {
  const map: Record<string, GatewaySegment[]> = {};
  let count = 0;
  for (const chunk of raw.split(';')) {
    const s = chunk.trim();
    if (s === '') continue;
    const toks = s.split(':').map((x) => x.trim());
    if (toks.length < 3 || toks.length > 4) return null;
    const area = GATEWAY_AREA_ALIASES[toks[0] ?? ''];
    if (!area) return null;
    const startRaw = toks[1] ?? '';
    const countRaw = toks[2] ?? '';
    const dtRaw = toks[3] ?? '';
    if (!isNonNegInt(startRaw)) return null;
    if (!isNonNegInt(countRaw) || Number(countRaw) < 1) return null;
    let dataType = DEFAULT_DATA_TYPE;
    if (dtRaw !== '') {
      if (!isValidDataType(dtRaw)) return null;
      dataType = dtRaw;
    }
    (map[area] ??= []).push({ address: Number(startRaw), count: Number(countRaw), data_type: dataType });
    count += 1;
  }
  if (count === 0) return null;
  return map;
}

// ---- 행 파서(내부 — line 보존) ----

/**
 * client 붙여넣기 → 리치 행 + 실패. transport 상속이 rtu 면 host 를 요구하지 않고 방출하지 않는다.
 * 첫 non-empty 행의 unit_id 셀(3열)이 정수가 아니면 헤더로 보고 스킵한다.
 */
function parseClientRows(text: string, transport: string): { rows: ClientRow[]; failures: BulkFailure[] } {
  const parsed = parseDelimitedRows(text);
  const rows: ClientRow[] = [];
  const failures: BulkFailure[] = [];
  const isRtu = transport === 'rtu';
  let first = true;
  for (const r of parsed) {
    if (first) {
      first = false;
      // 헤더 스킵: unit_id 셀이 정수가 아니면 헤더행으로 간주.
      if (!isNonNegInt(r.cells[2] ?? '')) continue;
    }
    const host = (r.cells[0] ?? '').trim();
    const portRaw = (r.cells[1] ?? '').trim();
    const unitRaw = (r.cells[2] ?? '').trim();
    const idRaw = (r.cells[3] ?? '').trim();
    const groupsRaw = (r.cells[4] ?? '').trim();

    // host: tcp 상속 시 필수. rtu 상속이면 무시.
    if (!isRtu && host === '') {
      failures.push({ line: r.line, input: r.raw, reason: EMPTY_REQUIRED });
      continue;
    }
    // unit_id: 1-247 정수 필수.
    if (!isNonNegInt(unitRaw)) {
      failures.push({ line: r.line, input: r.raw, reason: unitRaw === '' ? EMPTY_REQUIRED : INVALID_UNIT_ID });
      continue;
    }
    const unitId = Number(unitRaw);
    if (unitId < 1 || unitId > 247) {
      failures.push({ line: r.line, input: r.raw, reason: INVALID_UNIT_ID });
      continue;
    }
    // port: 있으면 정수여야 함(공란이면 방출 생략 → 백엔드 502 기본).
    let port: number | undefined;
    if (portRaw !== '') {
      if (!isNonNegInt(portRaw)) {
        failures.push({ line: r.line, input: r.raw, reason: INVALID_PORT });
        continue;
      }
      port = Number(portRaw);
    }
    // groups: 서브 파싱.
    const groups = parseClientGroups(groupsRaw);
    if (groups === null) {
      failures.push({ line: r.line, input: r.raw, reason: INVALID_GROUP });
      continue;
    }
    const device: ClientBulkDevice = { unit_id: unitId, register_groups: groups };
    if (!isRtu && host !== '') device.host = host;
    if (port !== undefined) device.port = port; // 공란이면 생략(백엔드 502 기본).
    if (idRaw !== '') device.id = idRaw;
    rows.push({ line: r.line, input: r.raw, device });
  }
  return { rows, failures };
}

/**
 * gateway 붙여넣기 → 리치 행 + 실패. segments 는 필수(백엔드 register_map 필수).
 * 첫 non-empty 행의 unit_id 셀(1열)이 정수가 아니면 헤더로 보고 스킵한다.
 */
function parseGatewayRows(text: string): { rows: GatewayRow[]; failures: BulkFailure[] } {
  const parsed = parseDelimitedRows(text);
  const rows: GatewayRow[] = [];
  const failures: BulkFailure[] = [];
  let first = true;
  for (const r of parsed) {
    if (first) {
      first = false;
      if (!isNonNegInt(r.cells[0] ?? '')) continue; // 헤더 스킵
    }
    const unitRaw = (r.cells[0] ?? '').trim();
    const name = (r.cells[1] ?? '').trim();
    const segRaw = (r.cells[2] ?? '').trim();

    if (!isNonNegInt(unitRaw)) {
      failures.push({ line: r.line, input: r.raw, reason: unitRaw === '' ? EMPTY_REQUIRED : INVALID_UNIT_ID });
      continue;
    }
    const unitId = Number(unitRaw);
    if (unitId < 1 || unitId > 247) {
      failures.push({ line: r.line, input: r.raw, reason: INVALID_UNIT_ID });
      continue;
    }
    // segments 필수(백엔드 register_map 필수) — 공란이면 백엔드 미호출로 사전 실패.
    if (segRaw === '') {
      failures.push({ line: r.line, input: r.raw, reason: EMPTY_SEGMENTS });
      continue;
    }
    const registerMap = parseGatewaySegments(segRaw);
    if (registerMap === null) {
      failures.push({ line: r.line, input: r.raw, reason: INVALID_SEGMENT });
      continue;
    }
    const device: GatewayBulkDevice = { unit_id: unitId, register_map: registerMap };
    if (name !== '') device.name = name;
    rows.push({ line: r.line, input: r.raw, device });
  }
  return { rows, failures };
}

// ---- 공개 순수 파서 ----

/**
 * client 붙여넣기 텍스트 → { devices, failures }(순수 함수, 단위 테스트 대상).
 * 컬럼: `host, port, unit_id, id, groups`(콤마 또는 탭 자동 감지). host 는 tcp 상속 시 필수.
 */
export function parseModbusClientBulk(text: string, transport: string = 'tcp'): ClientBulkParse {
  const { rows, failures } = parseClientRows(text, transport);
  return { devices: rows.map((r) => r.device), failures };
}

/**
 * gateway 붙여넣기 텍스트 → { devices, failures }(순수 함수, 단위 테스트 대상).
 * 컬럼: `unit_id, name, segments`. segments 필수(최소 1개 세그먼트).
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
 * client 일괄 등록. 붙여넣은 텍스트를 파싱해 각 유효 행마다 add_device 를 순차 호출한다.
 * 개별 행 실패(중복 ID·백엔드 검증 실패 등)에도 중단하지 않고 나머지 행을 계속 등록한다.
 * 파서 실패 + 백엔드 실패를 단일 BulkFailure[] 로 합류하고, total 은 전체 입력 행 수이다.
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
 * gateway 일괄 등록. client 와 동일한 best-effort 구조. 각 유효 행마다 add_device 를 순차 호출한다.
 * register_map 이 필수이므로 세그먼트 없는 행은 파서가 사전 실패(EMPTY_SEGMENTS)로 집계한다.
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
