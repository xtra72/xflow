// React Query hooks for flow CRUD and lifecycle operations.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import type { ListOptions } from '@/types/api';
import type { FlowCreateRequest, FlowUpdateRequest } from '@/types/flow';
import * as flowService from '@/services/api/flowService';

// ---- Queries ----

export function useFlows(params?: ListOptions) {
  return useQuery({
    queryKey: ['flows', params],
    queryFn: () => flowService.getFlows(params),
  });
}

export function useFlow(id: string) {
  return useQuery({
    queryKey: ['flows', id],
    queryFn: () => flowService.getFlow(id),
    enabled: !!id,
  });
}

export function useFlowStatus(id: string) {
  return useQuery({
    queryKey: ['flows', id, 'status'],
    queryFn: () => flowService.getFlowStatus(id),
    refetchInterval: 5000,
    enabled: !!id,
  });
}

export function useFlowNodes(flowId: string, refetchInterval?: number) {
  return useQuery({
    queryKey: ['flows', flowId, 'nodes'],
    queryFn: () => flowService.getFlowNodes(flowId),
    enabled: !!flowId,
    refetchInterval,
  });
}

// ---- Mutations ----

export function useCreateFlow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (req: FlowCreateRequest) => flowService.createFlow(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['flows'] });
    },
  });
}

export function useUpdateFlow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, req }: { id: string; req: FlowUpdateRequest }) =>
      flowService.updateFlow(id, req),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['flows'] });
      queryClient.invalidateQueries({ queryKey: ['flows', variables.id] });
    },
  });
}

export function useDeleteFlow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => flowService.deleteFlow(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['flows'] });
    },
  });
}

export function useDeployFlow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => flowService.deployFlow(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['flows'] });
      queryClient.invalidateQueries({ queryKey: ['flows', id] });
      queryClient.invalidateQueries({ queryKey: ['flows', id, 'status'] });
    },
  });
}

export function useStartFlow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => flowService.startFlow(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['flows'] });
      queryClient.invalidateQueries({ queryKey: ['flows', id, 'status'] });
    },
  });
}

export function useStopFlow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => flowService.stopFlow(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['flows'] });
      queryClient.invalidateQueries({ queryKey: ['flows', id, 'status'] });
    },
  });
}

export function useRestartFlow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => flowService.restartFlow(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['flows'] });
      queryClient.invalidateQueries({ queryKey: ['flows', id, 'status'] });
    },
  });
}
