// React Query hooks for device queries and mutations.

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

export function useDevice(id: string, refetchInterval?: number) {
  return useQuery({
    queryKey: ['devices', id],
    queryFn: () => deviceService.getDevice(id),
    enabled: !!id,
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
export function useDeviceRealtime(id: string) {
  const queryClient = useQueryClient();
  const { client } = useWebSocket();
  const query = useDevice(id);

  useEffect(() => {
    if (!client) return;
    return onDeviceStatus(client, () => {
      queryClient.invalidateQueries({ queryKey: ['devices', id] });
    });
  }, [client, queryClient, id]);

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
