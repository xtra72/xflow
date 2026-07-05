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
