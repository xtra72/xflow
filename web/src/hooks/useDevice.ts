// React Query hooks for device queries and mutations.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import type {
  DeviceExecuteRequest,
  DeviceListParams,
  DeviceMetadataUpdateRequest,
} from '@/types/device';
import * as deviceService from '@/services/api/deviceService';

// ---- Queries ----

export function useDevices(params?: DeviceListParams, refetchInterval?: number) {
  return useQuery({
    queryKey: ['devices', params],
    queryFn: () => deviceService.getDevices(params),
    refetchInterval,
  });
}

export function useDevice(id: string) {
  return useQuery({
    queryKey: ['devices', id],
    queryFn: () => deviceService.getDevice(id),
    enabled: !!id,
  });
}

// ---- Mutations ----

export function useExecuteCommand() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, req }: { id: string; req: DeviceExecuteRequest }) =>
      deviceService.executeCommand(id, req),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['devices', variables.id] });
    },
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
