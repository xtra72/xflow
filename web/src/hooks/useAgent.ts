// React Query hooks for agent CRUD, lifecycle, and exec operations.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import type { ListOptions } from '@/types/api';
import type { AgentCreateRequest, AgentExecRequest, AgentUpdateRequest } from '@/types/agent';
import type { AgentQueryRequest } from '@/services/api/agentService';
import * as agentService from '@/services/api/agentService';

// ---- Queries ----

export function useAgents(params?: ListOptions, refetchInterval?: number) {
  return useQuery({
    queryKey: ['agents', params],
    queryFn: () => agentService.getAgents(params),
    refetchInterval,
  });
}

export function useAgent(id: string, detail: 'summary' | 'full' = 'summary') {
  return useQuery({
    queryKey: ['agents', id, detail],
    queryFn: () => agentService.getAgent(id, detail),
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

/**
 * 에이전트를 영속적으로 활성화한다 (SPEC-AGENT-005).
 * 현재 정지된 에이전트를 자동으로 시작하지 않으며, 다음 데몬 재시작 시 자동 시작 대상이 된다.
 */
export function useEnableAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => agentService.enableAgent(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['agents'] });
      queryClient.invalidateQueries({ queryKey: ['agents', id] });
    },
  });
}

/**
 * 에이전트를 영속적으로 비활성화한다 (SPEC-AGENT-005).
 * 현재 실행 중인 에이전트를 정지하지 않으며, 다음 데몬 재시작 시 자동 시작에서 제외된다.
 */
export function useDisableAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => agentService.disableAgent(id),
    onSuccess: (_data, id) => {
      queryClient.invalidateQueries({ queryKey: ['agents'] });
      queryClient.invalidateQueries({ queryKey: ['agents', id] });
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
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, req }: { id: string; req: AgentExecRequest }) =>
      agentService.execAgent(id, req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['devices'] });
    },
  });
}

/**
 * 읽기 전용 커맨드용 뮤테이션 훅. `POST /agents/{id}/query` 로 보내므로
 * `agent.read` 권한만 있으면 동작한다(exec 는 `agent.execute` 를 요구).
 *
 * 명령 자체는 상태를 바꾸지 않지만 onSuccess 의 ['devices'] 무효화는
 * useExecAgent 와 동일하게 유지한다 — 호출부(AgentDetailPanel 의 목록 새로고침)가
 * 쓰기 직후 이 조회로 캐시 갱신을 유발해 왔고, 여기서 빼면 갱신이 조용히 사라진다.
 */
export function useQueryAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, req }: { id: string; req: AgentQueryRequest }) =>
      agentService.queryAgent(id, req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['devices'] });
    },
  });
}
