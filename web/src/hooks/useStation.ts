// React Query hooks for airpurifier station / place / device management.
//
// SPEC-AIRPURIFIER-001 Wave 2 (frontend).
// 모든 명령은 표준 exec 계약 `execAgent(id, { command, params })` 로 전송한다.
// 명령 인자는 `params` 아래에 중첩한다(samsung/modbus 와 동일 — HTTP /exec 핸들러가
// {command, params} 만 에이전트로 전달하며, airpurifier 백엔드가 params 로부터 내부
// 구조체 필드를 backfill 한다).
//
// 응답 형태(execAgent 는 API 엔벨로프를 벗겨 Process 결과를 그대로 반환):
//   - list_stations → { status, stations: [{ station, line, display_name, order, places: [...] }] }
//   - list_devices  → { status, devices:  [{ device_id, name, group_id, station, place, index, online, power, fan_speed, source }] }

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import * as agentService from '@/services/api/agentService';

// ---- 타입 ----

/** 역사 내 위치(place) 항목 (list_stations 에 임베드되거나 list_places 로 조회). */
export interface AirPlace {
  place: string;
  display_name: string;
  order: number;
}

/** 역사(station) 항목. places 는 해당 역사에 등록된 위치 목록(Order 정렬). */
export interface AirStation {
  station: string;
  line: string;
  display_name: string;
  order: number;
  places: AirPlace[];
}

/** airpurifier 디바이스 로스터 항목 (list_devices 응답). */
export interface AirDevice {
  device_id: string;
  name: string;
  group_id: string;
  station: string;
  place: string;
  index: number;
  online: boolean;
  power: boolean;
  fan_speed: number;
  source: string;
}

// ---- 일괄 등록 (delimited-text paste) ----

/** 파싱된 한 행. line 은 붙여넣은 텍스트의 원본 1-based 줄 번호(오류 표기용). */
export interface ParsedRow {
  line: number;
  cells: string[];
  raw: string;
}

/** 일괄 등록 개별 행 실패. reason 이 'EMPTY_REQUIRED' 면 필수값 누락(컴포넌트가 i18n 매핑),
 *  그 외에는 백엔드 오류 메시지 원문. */
export interface BulkFailure {
  line: number;
  input: string;
  reason: string;
}

/** 일괄 등록 결과 요약(best-effort). */
export interface BulkResult {
  total: number;
  ok: number;
  failed: BulkFailure[];
}

/** 필수값 누락 사유 sentinel(컴포넌트에서 i18n 으로 치환). */
export const EMPTY_REQUIRED = 'EMPTY_REQUIRED';

/**
 * 구분자 텍스트를 행 단위로 파싱한다.
 *   - 개행(\n)으로 분리, \r 제거(스프레드시트/윈도우 붙여넣기), 공백만인 줄은 스킵.
 *   - 줄마다 구분자 자동 감지: TAB 이 있으면 tab-split, 없으면 comma-split.
 *   - 각 셀 trim.
 * 원본 줄 번호(1-based)를 보존해 오류 표기에 사용한다.
 */
export function parseDelimitedRows(text: string): ParsedRow[] {
  const out: ParsedRow[] = [];
  const lines = text.split('\n');
  for (let i = 0; i < lines.length; i++) {
    const raw = (lines[i] ?? '').replace(/\r$/, '');
    if (raw.trim() === '') continue;
    const delim = raw.includes('\t') ? '\t' : ',';
    out.push({ line: i + 1, cells: raw.split(delim).map((c) => c.trim()), raw: raw.trim() });
  }
  return out;
}

/** 정수 파싱(빈 값/비숫자 → 0). */
function toIntOrZero(s: string | undefined): number {
  const n = parseInt((s ?? '').trim(), 10);
  return Number.isNaN(n) ? 0 : n;
}

// ---- 쿼리 키 ----

const stationsKey = (agentId: string) => ['airpurifier-stations', agentId] as const;
const devicesKey = (agentId: string) => ['airpurifier-devices', agentId] as const;

// ---- 역사(station) 쿼리 ----

/** 역사 목록 조회 (list_stations, Order 정렬). 각 역사에 위치(places)가 임베드된다. */
export function useStations(agentId: string) {
  return useQuery({
    queryKey: stationsKey(agentId),
    queryFn: async () => {
      const res = await agentService.execAgent(agentId, { command: 'list_stations' });
      const stations = (res as unknown as { stations?: AirStation[] }).stations ?? [];
      // places 누락 방어(빈 역사).
      return stations.map((s) => ({ ...s, places: s.places ?? [] }));
    },
    enabled: !!agentId,
  });
}

// ---- 역사(station) 뮤테이션 ----

export interface AddStationVariables {
  station: string;
  line?: string;
  display_name?: string;
  order?: number;
}

/** 역사 추가/갱신 (add_station upsert). */
export function useAddStation(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (v: AddStationVariables) =>
      agentService.execAgent(agentId, { command: 'add_station', params: { ...v } }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: stationsKey(agentId) }),
  });
}

/** 역사 삭제 (remove_station). */
export function useRemoveStation(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (station: string) =>
      agentService.execAgent(agentId, { command: 'remove_station', params: { station } }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: stationsKey(agentId) }),
  });
}

// ---- 위치(place) 뮤테이션 ----

export interface AddPlaceVariables {
  station: string;
  place: string;
  display_name?: string;
  order?: number;
}

/** 역사 내 위치 추가/갱신 (add_place upsert). 역사가 미등록이면 백엔드가 오류를 반환한다. */
export function useAddPlace(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (v: AddPlaceVariables) =>
      agentService.execAgent(agentId, { command: 'add_place', params: { ...v } }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: stationsKey(agentId) }),
  });
}

export interface RemovePlaceVariables {
  station: string;
  place: string;
}

/** 역사 내 위치 삭제 (remove_place). */
export function useRemovePlace(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (v: RemovePlaceVariables) =>
      agentService.execAgent(agentId, { command: 'remove_place', params: { ...v } }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: stationsKey(agentId) }),
  });
}

// ---- 일괄 등록 뮤테이션 (best-effort 순차 루프) ----

/**
 * 역사 일괄 등록. 붙여넣은 텍스트를 파싱해 각 행마다 add_station 을 순차 호출한다.
 * 컬럼 순서: station(필수), line, display_name, order(int). best-effort — 개별 실패는
 * 수집하고 계속 진행하며, 마지막에 한 번만 stations 쿼리를 무효화한다.
 */
export function useBulkAddStations(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (text: string): Promise<BulkResult> => {
      const rows = parseDelimitedRows(text);
      const failed: BulkFailure[] = [];
      let ok = 0;
      for (const r of rows) {
        const station = r.cells[0] ?? '';
        if (!station) {
          failed.push({ line: r.line, input: r.raw, reason: EMPTY_REQUIRED });
          continue;
        }
        try {
          await agentService.execAgent(agentId, {
            command: 'add_station',
            params: {
              station,
              line: r.cells[1] ?? '',
              display_name: r.cells[2] ?? '',
              order: toIntOrZero(r.cells[3]),
            },
          });
          ok += 1;
        } catch (e) {
          failed.push({ line: r.line, input: r.raw, reason: e instanceof Error ? e.message : EMPTY_REQUIRED });
        }
      }
      return { total: rows.length, ok, failed };
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: stationsKey(agentId) }),
  });
}

/**
 * 특정 역사에 위치 일괄 등록. 컬럼 순서: place(필수), display_name, order(int).
 * best-effort — 개별 실패 수집 후 계속, 마지막에 stations 쿼리 1회 무효화.
 */
export function useBulkAddPlaces(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ station, text }: { station: string; text: string }): Promise<BulkResult> => {
      const rows = parseDelimitedRows(text);
      const failed: BulkFailure[] = [];
      let ok = 0;
      for (const r of rows) {
        const place = r.cells[0] ?? '';
        if (!place) {
          failed.push({ line: r.line, input: r.raw, reason: EMPTY_REQUIRED });
          continue;
        }
        try {
          await agentService.execAgent(agentId, {
            command: 'add_place',
            params: {
              station,
              place,
              display_name: r.cells[1] ?? '',
              order: toIntOrZero(r.cells[2]),
            },
          });
          ok += 1;
        } catch (e) {
          failed.push({ line: r.line, input: r.raw, reason: e instanceof Error ? e.message : EMPTY_REQUIRED });
        }
      }
      return { total: rows.length, ok, failed };
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: stationsKey(agentId) }),
  });
}

/**
 * 위치 일괄 등록(최상위) — 행마다 STATION 을 포함해 어떤 역사든 한 번에 등록한다.
 * 컬럼 순서: station(필수), place(필수), display_name, order(int). station 또는 place 가
 * 비면 EMPTY_REQUIRED 로 실패 집계(백엔드 미호출). best-effort 순차 루프, 마지막에 1회 무효화.
 */
export function useBulkAddPlacesTop(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (text: string): Promise<BulkResult> => {
      const rows = parseDelimitedRows(text);
      const failed: BulkFailure[] = [];
      let ok = 0;
      for (const r of rows) {
        const station = r.cells[0] ?? '';
        const place = r.cells[1] ?? '';
        if (!station || !place) {
          failed.push({ line: r.line, input: r.raw, reason: EMPTY_REQUIRED });
          continue;
        }
        try {
          await agentService.execAgent(agentId, {
            command: 'add_place',
            params: {
              station,
              place,
              display_name: r.cells[2] ?? '',
              order: toIntOrZero(r.cells[3]),
            },
          });
          ok += 1;
        } catch (e) {
          failed.push({ line: r.line, input: r.raw, reason: e instanceof Error ? e.message : EMPTY_REQUIRED });
        }
      }
      return { total: rows.length, ok, failed };
    },
    onSuccess: () => queryClient.invalidateQueries({ queryKey: stationsKey(agentId) }),
  });
}

// ---- 디바이스(airpurifier) ----

/** airpurifier 디바이스 로스터 조회 (list_devices). */
export function useAirpurifierDevices(agentId: string) {
  return useQuery({
    queryKey: devicesKey(agentId),
    queryFn: async () => {
      const res = await agentService.execAgent(agentId, { command: 'list_devices' });
      return (res as unknown as { devices?: AirDevice[] }).devices ?? [];
    },
    enabled: !!agentId,
  });
}

/**
 * 디바이스 생성 파라미터. 백엔드가 device_id(UUID)를 생성하고 name(복합 키)을 계산하므로
 * 생성 요청에는 device_id / name 을 보내지 않는다.
 */
export interface AirDeviceCreate {
  station: string;
  place: string;
  index: number;
  group_id?: string;
}

/**
 * 디바이스 수정 파라미터. device_id(UUID)로 대상을 지정하고, 편집 가능한 필드를 보낸다.
 * 백엔드가 name 을 재계산한다.
 */
export interface AirDeviceUpdate {
  device_id: string;
  station?: string;
  place?: string;
  index?: number;
  group_id?: string;
}

/** add_device 응답 형태(execAgent 는 엔벨로프를 벗겨 Process 결과를 그대로 반환). */
export interface AddDeviceResult {
  status: string;
  device_id: string;
  name: string;
  source: string;
}

/**
 * 디바이스 추가 (add_device, Source="bridge"). device_id(UUID)와 name(복합 키)은 백엔드가
 * 생성/계산한다. 응답의 name 을 성공 토스트에 노출하기 위해 결과를 반환한다.
 */
export function useAddAirpurifierDevice(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (v: AirDeviceCreate): Promise<AddDeviceResult> => {
      const res = await agentService.execAgent(agentId, { command: 'add_device', params: { ...v } });
      return res as unknown as AddDeviceResult;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: devicesKey(agentId) });
      queryClient.invalidateQueries({ queryKey: ['devices'] });
    },
  });
}

/**
 * 디바이스 부분 갱신 (set_device). device_id(UUID)로 대상을 지정하고, 존재하는 키만
 * 갱신된다(백엔드가 name 재계산).
 */
export function useSetAirpurifierDevice(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (v: AirDeviceUpdate) =>
      agentService.execAgent(agentId, { command: 'set_device', params: { ...v } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: devicesKey(agentId) });
      queryClient.invalidateQueries({ queryKey: ['devices'] });
    },
  });
}

/** 디바이스 삭제 (remove_device). Source="config" 디바이스는 백엔드가 보호한다. */
export function useRemoveAirpurifierDevice(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (deviceId: string) =>
      agentService.execAgent(agentId, { command: 'remove_device', params: { device_id: deviceId } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: devicesKey(agentId) });
      queryClient.invalidateQueries({ queryKey: ['devices'] });
    },
  });
}
