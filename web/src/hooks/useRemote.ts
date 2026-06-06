// 원격 관리 React Query 훅 (SPEC-REMOTE-001 M5).
//
// 노드 목록/pending, 노드별/통합 미러 조회 쿼리와 승인/거부/폐기/명령 뮤테이션을
// 제공한다. online/offline 신선도를 위해 합리적인 refetchInterval 을 둔다.
//
// queryKey 규약:
//   ['remote', 'nodes']                          — 전체 노드 목록
//   ['remote', 'nodes', 'pending']               — pending 큐
//   ['remote', 'nodes', id, 'flows'|'agents'|'devices'] — 노드별 미러
//   ['remote', 'flows'|'agents'|'devices']       — 통합 미러

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import type {
  CommandRequest,
  EnrollmentTokenCreateRequest,
  MirroredResourceKind,
  PreRegisterRequest,
  RemoteAgentCreateRequest,
  RemoteAgentUpdateRequest,
  RemoteFlowCreateRequest,
  RemoteFlowUpdateRequest,
} from '@/types/remote';
import * as remoteService from '@/services/api/remoteService';

// 노드 라이브 상태(online/offline)는 빠르게 변하므로 짧은 폴링 주기를 둔다.
const NODES_REFETCH_MS = 5000;
// 미러 목록은 상대적으로 덜 빈번하게 변하므로 더 긴 주기를 둔다.
const MIRROR_REFETCH_MS = 10000;
// 동작 모드는 재시작 전에는 바뀌지 않으므로 길게 캐시한다.
const MODE_STALE_MS = 5 * 60 * 1000;

// ---- 동작 모드 쿼리 ----

/**
 * 인스턴스의 원격 관리 동작 모드 쿼리.
 *
 * 모드는 재시작 전에는 변하지 않으므로 staleTime 을 길게 두고 폴링하지 않는다.
 * 페이지/사이드바는 `data?.mode === 'server'` 로 admin 쿼리/메뉴 노출을 결정한다.
 * server 모드가 아닐 때 admin `/remote/*` 쿼리를 막아 404 노이즈를 방지한다.
 */
export function useRemoteMode() {
  return useQuery({
    queryKey: ['remote', 'mode'],
    queryFn: () => remoteService.getRemoteMode(),
    staleTime: MODE_STALE_MS,
    gcTime: MODE_STALE_MS,
    refetchOnWindowFocus: false,
  });
}

// ---- 쿼리 ----

/**
 * 전체 관리 노드 목록 쿼리 (G01). online/offline 신선도를 위해 5초 폴링.
 *
 * @param refetchInterval - 폴링 주기(ms) 오버라이드. 미지정 시 기본 5초.
 * @param enabled - 쿼리 활성 여부. server 모드가 아니면 false 로 발행을 막는다.
 */
export function useManagedNodes(
  refetchInterval: number = NODES_REFETCH_MS,
  enabled = true,
) {
  return useQuery({
    queryKey: ['remote', 'nodes'],
    queryFn: () => remoteService.listNodes(),
    refetchInterval,
    enabled,
  });
}

/**
 * 승인 대기(pending) 노드 큐 쿼리 (G02). 5초 폴링.
 *
 * @param refetchInterval - 폴링 주기(ms) 오버라이드. 미지정 시 기본 5초.
 * @param enabled - 쿼리 활성 여부. server 모드가 아니면 false 로 발행을 막는다.
 */
export function usePendingNodes(
  refetchInterval: number = NODES_REFETCH_MS,
  enabled = true,
) {
  return useQuery({
    queryKey: ['remote', 'nodes', 'pending'],
    queryFn: () => remoteService.listPendingNodes(),
    refetchInterval,
    enabled,
  });
}

/** 종류별 노드별 미러 조회 함수 매핑. */
const NODE_MIRROR_FN = {
  flow: remoteService.listNodeFlows,
  agent: remoteService.listNodeAgents,
  device: remoteService.listNodeDevices,
} as const;

/** 종류별 통합 미러 조회 함수 매핑. */
const ALL_MIRROR_FN = {
  flow: remoteService.listAllFlows,
  agent: remoteService.listAllAgents,
  device: remoteService.listAllDevices,
} as const;

/** 종류 → 통합 미러 쿼리 키 세그먼트 (복수형). */
const ALL_MIRROR_KEY: Record<MirroredResourceKind, string> = {
  flow: 'flows',
  agent: 'agents',
  device: 'devices',
};

/**
 * 한 노드의 종류별 미러 목록 쿼리 (G03).
 *
 * @param instanceID - 출처 노드 식별자. 비어 있으면 쿼리 비활성.
 * @param kind - 미러 종류 (flow/agent/device).
 * @param enabled - 쿼리 활성 여부. server 모드가 아니면 false 로 발행을 막는다.
 */
export function useNodeMirror(
  instanceID: string,
  kind: MirroredResourceKind,
  enabled = true,
) {
  return useQuery({
    queryKey: ['remote', 'nodes', instanceID, ALL_MIRROR_KEY[kind]],
    queryFn: () => NODE_MIRROR_FN[kind](instanceID),
    enabled: enabled && !!instanceID,
    refetchInterval: MIRROR_REFETCH_MS,
  });
}

/**
 * 전 노드의 종류별 통합 미러 목록 쿼리 (G03, REQ-E05).
 * 각 행은 source_instance_id 와 online 으로 태깅된다.
 *
 * @param kind - 미러 종류 (flow/agent/device).
 * @param enabled - 쿼리 활성 여부. server 모드가 아니면 false 로 발행을 막는다.
 */
export function useAllMirror(kind: MirroredResourceKind, enabled = true) {
  return useQuery({
    queryKey: ['remote', ALL_MIRROR_KEY[kind]],
    queryFn: () => ALL_MIRROR_FN[kind](),
    refetchInterval: MIRROR_REFETCH_MS,
    enabled,
  });
}

// ---- 뮤테이션 ----

/** 노드 관련 쿼리 전체를 무효화한다 (목록 + pending). */
function invalidateNodeQueries(
  queryClient: ReturnType<typeof useQueryClient>,
): void {
  queryClient.invalidateQueries({ queryKey: ['remote', 'nodes'] });
}

/**
 * 노드 사전 등록(수동 등록) 뮤테이션. 성공 시 노드 쿼리 무효화.
 *
 * 성공하면 status="approved", online=false 인 신규 노드가 목록에 나타난다.
 * 중복(409)/누락(400) 등 에러는 APIError 로 호출자에게 전파된다.
 */
export function usePreRegisterNode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (req: PreRegisterRequest) => remoteService.preRegisterNode(req),
    onSuccess: () => invalidateNodeQueries(queryClient),
  });
}

/**
 * 노드 삭제 뮤테이션. 성공 시 노드 쿼리 무효화.
 *
 * 폐기(revoke)와 달리 항목 자체를 제거한다. 미존재(404)는 APIError 로 전파된다.
 */
export function useDeleteNode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (instanceID: string) => remoteService.deleteNode(instanceID),
    onSuccess: () => invalidateNodeQueries(queryClient),
  });
}

// ---- Enrollment 토큰 쿼리/뮤테이션 ----

/** enrollment 토큰 목록 쿼리 키. */
const ENROLLMENT_TOKENS_KEY = ['remote', 'enrollment-tokens'] as const;

/** enrollment 토큰 목록 쿼리를 무효화한다. */
function invalidateEnrollmentTokens(
  queryClient: ReturnType<typeof useQueryClient>,
): void {
  queryClient.invalidateQueries({ queryKey: ENROLLMENT_TOKENS_KEY });
}

/**
 * Enrollment 토큰 메타데이터 목록 쿼리.
 *
 * @param enabled - 쿼리 활성 여부. server 모드가 아니면 false 로 발행을 막는다.
 */
export function useEnrollmentTokens(enabled = true) {
  return useQuery({
    queryKey: ENROLLMENT_TOKENS_KEY,
    queryFn: () => remoteService.listEnrollmentTokens(),
    enabled,
  });
}

/**
 * Enrollment 토큰 발급 뮤테이션. 성공 시 토큰 목록 무효화.
 *
 * 응답의 raw `token` 은 호출자가 1회 표시 후 폐기해야 한다.
 */
export function useCreateEnrollmentToken() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (req: EnrollmentTokenCreateRequest) =>
      remoteService.createEnrollmentToken(req),
    onSuccess: () => invalidateEnrollmentTokens(queryClient),
  });
}

/**
 * Enrollment 토큰 폐기 뮤테이션. 성공 시 토큰 목록 무효화.
 */
export function useRevokeEnrollmentToken() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => remoteService.revokeEnrollmentToken(id),
    onSuccess: () => invalidateEnrollmentTokens(queryClient),
  });
}

/**
 * 노드 승인 뮤테이션 (G02, REQ-C03). 성공 시 노드 쿼리 무효화.
 */
export function useApproveNode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (instanceID: string) => remoteService.approveNode(instanceID),
    onSuccess: () => invalidateNodeQueries(queryClient),
  });
}

/**
 * 노드 거부 뮤테이션 (G02, REQ-C03). 성공 시 노드 쿼리 무효화.
 */
export function useRejectNode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ instanceID, reason }: { instanceID: string; reason?: string }) =>
      remoteService.rejectNode(instanceID, reason),
    onSuccess: () => invalidateNodeQueries(queryClient),
  });
}

/**
 * 노드 폐기 뮤테이션 (G02, REQ-C07). 성공 시 노드 쿼리 무효화.
 */
export function useRevokeNode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (instanceID: string) => remoteService.revokeNode(instanceID),
    onSuccess: () => invalidateNodeQueries(queryClient),
  });
}

/**
 * 원격 명령 발행 뮤테이션 (G04, REQ-D01).
 *
 * 명령 직후 미러 목록을 무효화하여 상태 변화를 반영한다 (단, 실제 상태 변경은
 * 노드의 inventory delta 로 비동기 전파되므로 즉시 반영되지 않을 수 있다).
 */
export function useSendCommand() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ instanceID, req }: { instanceID: string; req: CommandRequest }) =>
      remoteService.sendCommand(instanceID, req),
    onSuccess: (_data, variables) => {
      // 대상 노드 미러 + 통합 미러 갱신 유도.
      queryClient.invalidateQueries({
        queryKey: ['remote', 'nodes', variables.instanceID],
      });
      queryClient.invalidateQueries({ queryKey: ['remote', 'flows'] });
      queryClient.invalidateQueries({ queryKey: ['remote', 'agents'] });
      queryClient.invalidateQueries({ queryKey: ['remote', 'devices'] });
    },
  });
}

// ---- 원격 자원 편집 뮤테이션 (M7, 그룹 I, REQ-I01~I04/I08~I11) ----

/**
 * 편집 성공 후 대상 노드 미러 + 통합 미러 쿼리를 무효화한다.
 *
 * 미러 캐시는 명령 결과 수신 후에만 서버에서 갱신되므로(REQ-E08), 무효화하면
 * 다음 폴링/refetch 에서 최신 상태가 반영된다.
 */
function invalidateMirrorQueries(
  queryClient: ReturnType<typeof useQueryClient>,
  instanceID: string,
  kind: 'flows' | 'agents',
): void {
  queryClient.invalidateQueries({ queryKey: ['remote', 'nodes', instanceID] });
  queryClient.invalidateQueries({ queryKey: ['remote', kind] });
}

/**
 * 원격 플로우 생성 뮤테이션 (REQ-I01). 성공 시 미러 쿼리 무효화.
 *
 * 응답의 `id` 는 노드가 채번한 식별자이다(node-assigned). 신규 자원은 자동
 * 노출되지 않으므로(opt-in 보존), 즉시 미러 목록에 나타나지 않을 수 있다.
 * 503/504/502 등 실패는 APIError 로 호출자에게 전파된다.
 */
export function useCreateRemoteFlow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      instanceID,
      req,
    }: {
      instanceID: string;
      req: RemoteFlowCreateRequest;
    }) => remoteService.createRemoteFlow(instanceID, req),
    onSuccess: (_data, variables) =>
      invalidateMirrorQueries(queryClient, variables.instanceID, 'flows'),
  });
}

/**
 * 원격 플로우 수정 뮤테이션 (REQ-I02). 성공 시 미러 쿼리 무효화.
 *
 * 본문 definition 의 마스킹/미변경 시크릿 필드는 호출 전 생략되어야 한다(REQ-I07).
 */
export function useUpdateRemoteFlow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      instanceID,
      flowID,
      req,
    }: {
      instanceID: string;
      flowID: string;
      req: RemoteFlowUpdateRequest;
    }) => remoteService.updateRemoteFlow(instanceID, flowID, req),
    onSuccess: (_data, variables) =>
      invalidateMirrorQueries(queryClient, variables.instanceID, 'flows'),
  });
}

/**
 * 원격 플로우 삭제 뮤테이션 (REQ-I03). 성공 시 미러 쿼리 무효화.
 */
export function useDeleteRemoteFlow() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ instanceID, flowID }: { instanceID: string; flowID: string }) =>
      remoteService.deleteRemoteFlow(instanceID, flowID),
    onSuccess: (_data, variables) =>
      invalidateMirrorQueries(queryClient, variables.instanceID, 'flows'),
  });
}

/**
 * 원격 에이전트 생성 뮤테이션 (REQ-I04). 성공 시 미러 쿼리 무효화.
 */
export function useCreateRemoteAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      instanceID,
      req,
    }: {
      instanceID: string;
      req: RemoteAgentCreateRequest;
    }) => remoteService.createRemoteAgent(instanceID, req),
    onSuccess: (_data, variables) =>
      invalidateMirrorQueries(queryClient, variables.instanceID, 'agents'),
  });
}

/**
 * 원격 에이전트 수정 뮤테이션 (REQ-I04). 성공 시 미러 쿼리 무효화.
 *
 * 본문 config 의 마스킹/미변경 시크릿 필드는 호출 전 생략되어야 한다(REQ-I07).
 */
export function useUpdateRemoteAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      instanceID,
      agentID,
      req,
    }: {
      instanceID: string;
      agentID: string;
      req: RemoteAgentUpdateRequest;
    }) => remoteService.updateRemoteAgent(instanceID, agentID, req),
    onSuccess: (_data, variables) =>
      invalidateMirrorQueries(queryClient, variables.instanceID, 'agents'),
  });
}

/**
 * 원격 에이전트 삭제 뮤테이션 (REQ-I04). 성공 시 미러 쿼리 무효화.
 */
export function useDeleteRemoteAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ instanceID, agentID }: { instanceID: string; agentID: string }) =>
      remoteService.deleteRemoteAgent(instanceID, agentID),
    onSuccess: (_data, variables) =>
      invalidateMirrorQueries(queryClient, variables.instanceID, 'agents'),
  });
}
