// React Query hooks for device queries and mutations.
//
// SPEC-DEVICE-IDENTITY-001 Phase D (M11):
// 디바이스 식별자 (`id` 인자, React Query key) 는 UUID v4 형식 (Phase D+) 또는
// composite `agent:local_id` (Phase A~C) 를 받아들인다. PR4 (backend
// composite 제거) 이후 모든 식별자는 UUID 가 된다. backend `id` 응답 필드가
// UUID 로 시맨틱 변경되므로 frontend 는 호출자가 `device.id` 또는 `device.uid`
// 어느 것을 넘겨도 동작한다 (PR4 후 두 값이 동일).

import { useEffect } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import type {
  DeviceExecuteRequest,
  DeviceInfo,
  DeviceListParams,
  DeviceMetadataUpdateRequest,
} from '@/types/device';
import * as deviceService from '@/services/api/deviceService';
import * as agentService from '@/services/api/agentService';
import { useWebSocket } from '@/hooks/useWebSocket';
import { onDeviceStatus } from '@/services/ws/wsHandlers';

// ---- Queries ----

export function useDevices(params?: DeviceListParams, refetchInterval?: number) {
  return useQuery({
    queryKey: ['devices', params],
    queryFn: () => deviceService.getDevices(params),
    refetchInterval,
  });
}

/**
 * 단일 디바이스 상세 조회.
 *
 * @param uid - 디바이스 식별자 (UUID v4 권장, Phase A~C 는 composite 호환).
 */
export function useDevice(uid: string, refetchInterval?: number) {
  return useQuery({
    queryKey: ['devices', uid],
    queryFn: () => deviceService.getDevice(uid),
    enabled: !!uid,
    refetchInterval,
  });
}

/**
 * 디바이스 수신 데이터 이력(주기 스냅샷) 조회.
 *
 * 상세 섹션 진입(enabled) 시 또는 limit 변경 시 조회한다. 이력은 best-effort
 * 관측 데이터이므로 짧은 staleTime 으로 캐싱한다(과도한 폴링 회피, 수동 재조회 위주).
 *
 * @param id - 디바이스 식별자 (uid/UUID 우선)
 * @param limit - 조회 개수 (서버가 max 로 clamp). 기본 100.
 * @param enabled - 섹션이 보일 때만 조회하도록 게이팅.
 */
export function useDeviceHistory(id: string, limit = 100, enabled = true) {
  return useQuery({
    queryKey: ['devices', id, 'history', limit],
    queryFn: () => deviceService.getDeviceHistory(id, limit),
    enabled: enabled && !!id,
    // 이력 비활성(404)은 한 번만 시도(반복 재시도 무의미).
    retry: false,
    staleTime: 10_000,
  });
}

/** useDevices + WebSocket 실시간 갱신. device.status 수신 시 자동 refetch. */
export function useDevicesRealtime(params?: DeviceListParams) {
  const queryClient = useQueryClient();
  const { client } = useWebSocket();
  const query = useDevices(params);

  useEffect(() => {
    if (!client) return;
    return onDeviceStatus(client, () => {
      queryClient.invalidateQueries({ queryKey: ['devices'] });
    });
  }, [client, queryClient]);

  return query;
}

/** useDevice + WebSocket 실시간 갱신. device.status 수신 시 자동 refetch. */
export function useDeviceRealtime(uid: string) {
  const queryClient = useQueryClient();
  const { client } = useWebSocket();
  const query = useDevice(uid);

  useEffect(() => {
    if (!client) return;
    return onDeviceStatus(client, () => {
      queryClient.invalidateQueries({ queryKey: ['devices', uid] });
    });
  }, [client, queryClient, uid]);

  return query;
}

// ---- Mutations ----

export function useExecuteCommand() {
  return useMutation({
    mutationFn: ({ id, req }: { id: string; req: DeviceExecuteRequest }) =>
      deviceService.executeCommand(id, req),
    // onSuccess 에서 invalidateQueries 를 호출하지 않는다.
    // 제어 명령 직후 refetch 하면 하드웨어 응답 전의 이전 상태를 가져오기 때문이다.
    // 실제 상태 변경은 WebSocket device.status 이벤트로 전달되며,
    // useDevicesRealtime / useDeviceRealtime 이 자동으로 refetch 한다.
  });
}

export function useUpdateMetadata() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, metadata }: { id: string; metadata: DeviceMetadataUpdateRequest }) =>
      deviceService.updateMetadata(id, metadata),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['devices'] });
      queryClient.invalidateQueries({ queryKey: ['devices', variables.id] });
    },
  });
}

export function useDeleteMetadata() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deviceService.deleteMetadata(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['devices'] });
      queryClient.invalidateQueries({ queryKey: ['devices', id] });
    },
  });
}

/** {@link useSetDeviceReport} 뮤테이션 변수. */
export interface SetDeviceReportVariables {
  /** 디바이스를 소유한 에이전트 ID (`set_device` exec 대상). */
  agentId: string;
  /**
   * 디바이스 UUID(`device.uid` 우선). `set_device` 의 `device_id` 파라미터로 사용된다.
   * add_device/remove_device 와 동일한 식별자 방식(백엔드가 params 내 device_id 를 해석).
   */
  deviceId: string;
  /** 설정할 상태 전송 활성화 여부. */
  reportEnabled: boolean;
}

/**
 * 디바이스별 상태 전송 on/off 토글 훅 (`set_device` exec).
 *
 * 백엔드는 `samsung_hvacr01` / `lgap` 에이전트에서 `set_device` 를 지원한다.
 * `report_enabled=false` 로 설정하면 이후 해당 디바이스의 device_state 및
 * 디바이스 이벤트 방출이 모두 억제되며, 설정은 재시작 후에도
 * 영속된다(백엔드가 자동 저장).
 *
 * 낙관적 업데이트: 즉시 `['devices']` 캐시의 해당 디바이스 `report_enabled` 를 갱신해
 * 토글 UI 가 지연 없이 반영되게 하고, 실패 시 이전 캐시로 롤백한다. 성공/실패 후
 * `['devices']` 를 무효화해 권위 응답으로 동기화한다.
 */
export function useSetDeviceReport() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ agentId, deviceId, reportEnabled }: SetDeviceReportVariables) =>
      agentService.execAgent(agentId, {
        command: 'set_device',
        params: { device_id: deviceId, report_enabled: reportEnabled },
      }),
    onMutate: async ({ deviceId, reportEnabled }) => {
      // 진행 중인 ['devices'] 리페치를 취소해 낙관적 갱신 덮어쓰기를 방지한다.
      await queryClient.cancelQueries({ queryKey: ['devices'] });
      const snapshots = queryClient.getQueriesData<DeviceListResult>({ queryKey: ['devices'] });
      // uid/id 어느 쪽이 넘어와도 매칭되도록 두 값을 모두 비교한다.
      for (const [key, value] of snapshots) {
        if (!value?.data) continue;
        queryClient.setQueryData(key, {
          ...value,
          data: value.data.map((d) =>
            d.uid === deviceId || d.id === deviceId
              ? { ...d, report_enabled: reportEnabled }
              : d,
          ),
        });
      }
      return { snapshots };
    },
    onError: (_err, _vars, context) => {
      // 낙관적 갱신 롤백.
      for (const [key, value] of context?.snapshots ?? []) {
        queryClient.setQueryData(key, value);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ['devices'] });
    },
  });
}

/**
 * `['devices']` 목록 쿼리의 캐시 형태. getDevices 응답(`{ data: DeviceInfo[] }`).
 * 낙관적 업데이트에서 개별 디바이스의 report_enabled 를 패치할 때 사용한다.
 */
interface DeviceListResult {
  data?: DeviceInfo[];
}

/** {@link useDeleteDevice} 뮤테이션 변수. */
export interface DeleteDeviceVariables {
  /** 디바이스를 소유한 에이전트 ID (`remove_device` exec 대상). */
  agentId: string;
  /**
   * 디바이스 UUID(`device.uid` 우선). 지정 시 `remove_device` 의 `device_id`
   * 파라미터로 사용되며, 성공 후 동일 식별자로 저장된 메타데이터를 정리한다.
   */
  deviceId?: string;
  /**
   * 버스 주소(Samsung NASA "20.00.01" / LGAP zone "01"). `deviceId` 가 없을 때만
   * `remove_device` 의 `address` 파라미터 fallback 으로 사용된다.
   */
  address?: string;
}

/**
 * 디바이스 삭제 오케스트레이션 훅.
 *
 * 백엔드는 `samsung_hvacr01` / `lgap` 에이전트에서만 `remove_device` 를 지원한다
 * (Century/system/modbus 미지원). `source === 'config'` 디바이스는 서버가 보호하므로
 * 호출자가 사전에 삭제 대상에서 제외해야 한다.
 *
 * 순서:
 *   1. `POST /agents/{agentId}/exec` `remove_device` (device_id 우선, address fallback).
 *   2. 성공 시 `deviceId` 로 저장된 메타데이터 삭제(best-effort — 404/오류 무시).
 *   3. `['devices']` 쿼리 무효화로 목록 갱신.
 *
 * remove_device 실패는 mutation 을 reject 하여 호출자의 onError 로 전파된다.
 * 메타데이터 정리 실패는 사용자 흐름을 막지 않도록 삼킨다.
 */
export function useDeleteDevice() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ agentId, deviceId, address }: DeleteDeviceVariables) => {
      const params: Record<string, unknown> = {};
      if (deviceId) params.device_id = deviceId;
      else if (address) params.address = address;

      await agentService.execAgent(agentId, { command: 'remove_device', params });

      // 저장된 사용자 메타데이터(이름/태그/pin) 정리. 메타데이터가 없으면 404 가
      // 반환될 수 있으므로 best-effort 로 처리하고 오류를 무시한다.
      if (deviceId) {
        try {
          await deviceService.deleteMetadata(deviceId);
        } catch {
          // ignore — 메타데이터가 없을 수 있음(404 등).
        }
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['devices'] });
    },
  });
}
