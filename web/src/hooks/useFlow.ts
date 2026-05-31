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
    onSuccess: (data, variables) => {
      // 목록은 무효화해 최신화한다.
      queryClient.invalidateQueries({ queryKey: ['flows'] });
      // 상세는 재조회(invalidate)하지 않고 응답으로 캐시를 직접 갱신한다.
      // 재조회가 일어나면 에디터가 다시 hydrate 되어 저장 직후 isDirty 가
      // 되살아나 저장 버튼 빨간점이 사라지지 않는 문제가 생기기 때문이다.
      queryClient.setQueryData(['flows', variables.id], data);
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

export function useUndeployFlow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => flowService.undeployFlow(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['flows'] });
      queryClient.invalidateQueries({ queryKey: ['flows', id] });
      queryClient.invalidateQueries({ queryKey: ['flows', id, 'status'] });
    },
  });
}
