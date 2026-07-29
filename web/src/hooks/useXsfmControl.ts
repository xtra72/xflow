// Facility dashboard 제어 훅 (SPEC-FACILITY-DASHBOARD-001 B1 / Module 6 호출 경로).
//
// 제어 명령(set_power / set_fan_speed / set_multiple)을 표준 exec 계약으로 전송한다.
// 대상은 단일 device_id 또는 셀렉터(station / line / group_id) 중 하나이며, fan-out(멤버별
// 실행·응답 대기·집계)은 백엔드가 수행한다(internal/agent/xsfm/group.go). 이 훅은
// fan-out 을 재구현하지 않고(UB-001) exec 를 호출한 뒤 fan-out 응답 형태로 타입만 부여해,
// 패널이 멤버별 ok/error/timeout 을 렌더할 수 있게 한다(REQ-FACDASH-001-06-02).
//
// 응답 형태(execAgent 는 엔벨로프를 벗겨 Process 결과를 그대로 반환):
//   - 셀렉터 fan-out → { selector:{type,value}, results:[{device_id,status,error?}], excluded?, status }
//   - 단일 device_id → { device_id, status, ... }

import { useMutation, useQueryClient } from '@tanstack/react-query';

import * as agentService from '@/services/api/agentService';
import { useXsfmDevices, useStations } from '@/hooks/useStation';
import type { AirDevice, AirStation } from '@/hooks/useStation';

// ---- 응답 타입 ----

/** fan-out 대상 셀렉터 종류·값 (백엔드 selectorRef 와 대응). */
export interface SelectorRef {
  type: 'station' | 'line' | 'group_id';
  value: string;
}

/** fan-out 멤버별 결과 (백엔드 groupResult 와 대응). error 는 status != 'ok' 일 때만 존재. */
export interface GroupMemberResult {
  device_id: string;
  status: 'ok' | 'error' | 'timeout';
  error?: string;
}

/** 셀렉터 fan-out 집계 응답 (백엔드 fanOutResponse 와 대응). */
export interface FanOutResponse {
  selector: SelectorRef;
  results: GroupMemberResult[];
  excluded?: string[];
  status: 'ok' | 'partial' | 'error';
}

/** 단일 device_id 제어 응답. status 외 추가 필드가 올 수 있어 인덱스 시그니처를 둔다. */
export interface SingleDeviceResult {
  device_id: string;
  status: string;
  [key: string]: unknown;
}

/** 제어 응답: 셀렉터 fan-out 또는 단일 디바이스 결과의 판별 유니온. */
export type ControlResponse = FanOutResponse | SingleDeviceResult;

/** fan-out 응답 여부 판별 타입가드(results 배열 + selector 존재로 구분). */
export function isFanOutResponse(res: ControlResponse): res is FanOutResponse {
  return Array.isArray((res as FanOutResponse).results) && 'selector' in res;
}

// ---- 요청 타입 ----

/** 제어 대상 셀렉터: device_id / station / line / group_id 중 정확히 하나. */
export type ControlSelector =
  | { device_id: string }
  | { station: string }
  | { line: string }
  | { group_id: string };

export type SetPowerVariables = ControlSelector & { power: boolean };
export type SetFanSpeedVariables = ControlSelector & { fan_speed: number };
export type SetMultipleVariables = ControlSelector & { power?: boolean; fan_speed?: number };

// ---- 훅 ----

const devicesKey = (agentId: string) => ['xsfm-devices', agentId] as const;

/**
 * xsfm 제어 뮤테이션 훅. set_power / set_fan_speed / set_multiple 을 노출한다.
 * 각 뮤테이션 변수는 셀렉터(하나) + 제어 값이며, exec 로 전송 후 fan-out/단일 응답을 타입화해
 * 반환한다. 성공 시 로스터 쿼리를 무효화해 UI 를 갱신한다(선택적, best-effort).
 */
export function useXsfmControl(agentId: string) {
  const queryClient = useQueryClient();
  const invalidateRoster = () => {
    queryClient.invalidateQueries({ queryKey: devicesKey(agentId) });
  };

  const setPower = useMutation({
    mutationFn: async (v: SetPowerVariables): Promise<ControlResponse> => {
      const res = await agentService.execAgent(agentId, { command: 'set_power', params: { ...v } });
      return res as unknown as ControlResponse;
    },
    onSuccess: invalidateRoster,
  });

  const setFanSpeed = useMutation({
    mutationFn: async (v: SetFanSpeedVariables): Promise<ControlResponse> => {
      const res = await agentService.execAgent(agentId, { command: 'set_fan_speed', params: { ...v } });
      return res as unknown as ControlResponse;
    },
    onSuccess: invalidateRoster,
  });

  const setMultiple = useMutation({
    mutationFn: async (v: SetMultipleVariables): Promise<ControlResponse> => {
      const res = await agentService.execAgent(agentId, { command: 'set_multiple', params: { ...v } });
      return res as unknown as ControlResponse;
    },
    onSuccess: invalidateRoster,
  });

  return { setPower, setFanSpeed, setMultiple };
}

// ---- Facility 로스터(주기 폴링) ----

/** useFacilityRoster 반환: 로스터 + 레지스트리 + 로딩/에러 상태 + 원본 쿼리 핸들. */
export interface FacilityRoster {
  devices: AirDevice[];
  stations: AirStation[];
  isLoading: boolean;
  isError: boolean;
  refetch: () => void;
  devicesQuery: ReturnType<typeof useXsfmDevices>;
  stationsQuery: ReturnType<typeof useStations>;
}

/**
 * Facility 대시보드용 로스터 읽기 훅(REQ-FACDASH-001-05-04). list_devices + list_stations 를
 * 함께 조회하며, refreshMs 지정 시 두 쿼리를 주기 폴링한다(xsfm 로스터는 WebSocket
 * 푸시가 아닌 exec 폴링). 기존 useXsfmDevices / useStations 를 재사용해 DRY 를 유지한다.
 */
export function useFacilityRoster(agentId: string, refreshMs?: number): FacilityRoster {
  const devicesQuery = useXsfmDevices(agentId, refreshMs);
  const stationsQuery = useStations(agentId, refreshMs);
  return {
    devices: devicesQuery.data ?? [],
    stations: stationsQuery.data ?? [],
    isLoading: devicesQuery.isLoading || stationsQuery.isLoading,
    isError: devicesQuery.isError || stationsQuery.isError,
    refetch: () => {
      void devicesQuery.refetch();
      void stationsQuery.refetch();
    },
    devicesQuery,
    stationsQuery,
  };
}
