// React Query hooks for agent CRUD, lifecycle, and exec operations.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import type { ListOptions } from '@/types/api';
import type { AgentCreateRequest, AgentExecRequest, AgentUpdateRequest } from '@/types/agent';
import * as agentService from '@/services/api/agentService';

// ---- Queries ----

export function useAgents(params?: ListOptions, refetchInterval?: number) {
  return useQuery({
    queryKey: ['agents', params],
    queryFn: () => agentService.getAgents(params),
    refetchInterval,
  });
}

export function useAgent(id: string) {
  return useQuery({
    queryKey: ['agents', id],
    queryFn: () => agentService.getAgent(id),
    enabled: !!id,
  });
}

export function useAgentStats(id: string) {
  return useQuery({
    queryKey: ['agents', id, 'stats'],
    queryFn: () => agentService.getAgentStats(id),
    refetchInterval: 5000,
    enabled: !!id,
  });
}

// ---- Mutations ----

export function useCreateAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (req: AgentCreateRequest) => agentService.createAgent(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agents'] });
    },
  });
}

export function useUpdateAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, req }: { id: string; req: AgentUpdateRequest }) =>
      agentService.updateAgent(id, req),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['agents'] });
      queryClient.invalidateQueries({ queryKey: ['agents', variables.id] });
    },
  });
}

export function useDeleteAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => agentService.deleteAgent(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['agents'] });
    },
  });
}

export function useStartAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => agentService.startAgent(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['agents', id] });
      queryClient.invalidateQueries({ queryKey: ['agents', id, 'stats'] });
    },
  });
}

export function useStopAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => agentService.stopAgent(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['agents', id] });
      queryClient.invalidateQueries({ queryKey: ['agents', id, 'stats'] });
    },
  });
}

export function useRestartAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => agentService.restartAgent(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['agents', id] });
      queryClient.invalidateQueries({ queryKey: ['agents', id, 'stats'] });
    },
  });
}

export function useConfigureAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, config }: { id: string; config: Record<string, unknown> }) =>
      agentService.configureAgent(id, config),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['agents'] });
      queryClient.invalidateQueries({ queryKey: ['agents', variables.id] });
    },
  });
}

export function useExecAgent() {
  return useMutation({
    mutationFn: ({ id, req }: { id: string; req: AgentExecRequest }) =>
      agentService.execAgent(id, req),
  });
}
