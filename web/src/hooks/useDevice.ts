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
  DeviceListParams,
  DeviceMetadataUpdateRequest,
} from '@/types/device';
import * as deviceService from '@/services/api/deviceService';
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
