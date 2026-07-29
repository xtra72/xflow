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

export interface AirDeviceUpsert {
  device_id: string;
  name?: string;
  station?: string;
  place?: string;
  index?: number;
  group_id?: string;
}

/** 디바이스 추가 (add_device, Source="bridge"). device_id 필수. */
export function useAddAirpurifierDevice(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (v: AirDeviceUpsert) =>
      agentService.execAgent(agentId, { command: 'add_device', params: { ...v } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: devicesKey(agentId) });
      queryClient.invalidateQueries({ queryKey: ['devices'] });
    },
  });
}

/**
 * 디바이스 부분 갱신 (set_device). 백엔드는 요청 본문에 존재하는 키만 갱신하므로
 * 변경할 필드만 전달한다(device_id 는 항상 필요).
 */
export function useSetAirpurifierDevice(agentId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (v: AirDeviceUpsert) =>
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
