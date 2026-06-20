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

import type { AgentInfo } from '@/types/agent';
import type { DeviceInfo } from '@/types/device';
import type { FlowInfo } from '@/types/flow';
import type {
  CommandRequest,
  EnrollmentTokenCreateRequest,
  MirroredResourceKind,
  PreRegisterRequest,
  ReleaseCreateRequest,
  RemoteAgentCreateRequest,
  RemoteAgentUpdateRequest,
  RemoteFlowCreateRequest,
  RemoteFlowUpdateRequest,
  UpdateSource,
} from '@/types/remote';
import type {
  GroupUpdateRequest,
  NodeDetail,
  NodeGroup,
  NodeUpdateRequest,
} from '@/types/remote';
import * as remoteService from '@/services/api/remoteService';

// 노드 라이브 상태(online/offline)는 빠르게 변하므로 짧은 폴링 주기를 둔다.
const NODES_REFETCH_MS = 5000;
// 미러 목록은 상대적으로 덜 빈번하게 변하므로 더 긴 주기를 둔다.
const MIRROR_REFETCH_MS = 10000;
// 노드 상세(시스템 정보+운영 요약)는 uptime 갱신을 위해 적당한 주기로 폴링한다.
const NODE_DETAIL_REFETCH_MS = 5000;
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

// ---- 노드 그룹핑 + 상세 쿼리 (v1.4 M9, 그룹 K, REQ-K03/K08/K10) ----

/** distinct 그룹 목록 쿼리 키. */
const GROUPS_KEY = ['remote', 'groups'] as const;

/**
 * distinct 그룹 + 노드 수 쿼리 (REQ-K03). "전체" 가상 버킷을 항상 포함한다.
 *
 * @param enabled - 쿼리 활성 여부. server 모드가 아니면 false 로 발행을 막는다.
 */
export function useRemoteGroups(enabled = true) {
  return useQuery<NodeGroup[]>({
    queryKey: GROUPS_KEY,
    queryFn: () => remoteService.listRemoteGroups(),
    refetchInterval: NODES_REFETCH_MS,
    enabled,
  });
}

/**
 * 노드 상세(메타 + BASIC 시스템 정보 + uptime + 운영 요약) 쿼리 (REQ-K08/K10).
 *
 * @param instanceID - 노드 식별자. 비어 있으면 쿼리 비활성.
 * @param enabled - 쿼리 활성 여부. server 모드가 아니면 false 로 발행을 막는다.
 */
export function useRemoteNodeDetail(instanceID: string, enabled = true) {
  return useQuery<NodeDetail>({
    queryKey: ['remote', 'nodes', instanceID, 'detail'],
    queryFn: () => remoteService.getRemoteNodeDetail(instanceID),
    enabled: enabled && !!instanceID,
    refetchInterval: NODE_DETAIL_REFETCH_MS,
  });
}

/** 종류별 노드별 미러 조회 함수 매핑. */
const NODE_MIRROR_FN = {
  flow: remoteService.listNodeFlows,
  agent: remoteService.listNodeAgents,
  device: remoteService.listNodeDevices,
} as const;

/** 종류 → 미러 쿼리 키 세그먼트 (복수형). */
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

// ---- 노드별 라이브 목록 쿼리 (M8 보강, 그룹 J, REQ-J04) ----
//
// 미러 요약(useNodeMirror)이 결여하는 connected/uptime/stats 등 런타임 필드를 노드의
// FULL 로컬 목록(로컬 GET /agents|/flows|/devices 와 동형)으로 운반한다. 서비스
// 함수가 이중 중첩 본문의 `.data` 를 언래핑하므로 본 훅은 타입 배열을 그대로 반환한다.
//
// 쿼리 키는 미러 키(['remote','nodes',id,'agents'])와 충돌하지 않도록 'live' 세그먼트를
// 덧붙인다(['remote','nodes',id,'agents','live']). 편집 뮤테이션이 노드 prefix
// (['remote','nodes',id])로 무효화하므로 편집 후 라이브 목록도 함께 갱신된다.

/** 종류 → 라이브 목록 아이템 타입 매핑. */
interface NodeLiveItemMap {
  flow: FlowInfo;
  agent: AgentInfo;
  device: DeviceInfo;
}

/** 종류별 라이브 목록 조회 함수 매핑(각 종류의 타입 배열을 반환). */
const NODE_LIVE_FN: {
  [K in MirroredResourceKind]: (instanceID: string) => Promise<NodeLiveItemMap[K][]>;
} = {
  flow: remoteService.getRemoteFlowsLive,
  agent: remoteService.getRemoteAgentsLive,
  device: remoteService.getRemoteDevicesLive,
};

/**
 * 한 노드의 종류별 라이브 목록 쿼리(런타임 필드 포함, M8 보강).
 *
 * 미러 요약(useNodeMirror)이 결여하는 connected/uptime/stats 등 런타임 필드를 노드의
 * FULL 로컬 목록(로컬 GET /agents|/flows|/devices 와 동형)으로 운반한다. 반환 타입은
 * kind 에 따라 FlowInfo[]/AgentInfo[]/DeviceInfo[] 로 좁혀진다.
 *
 * @param instanceID - 출처 노드 식별자. 비어 있으면 쿼리 비활성.
 * @param kind - 목록 종류 (flow/agent/device).
 * @param enabled - 쿼리 활성 여부. remote ∧ nodeReady 일 때만 발행하도록 호출자가
 *   게이트한다(미관리/오프라인 노드의 503 노이즈 방지). 미지원(구버전 노드)/실패 시
 *   호출자가 에러를 잡아 미러 매핑으로 폴백한다(graceful degradation).
 */
export function useNodeLiveList<K extends MirroredResourceKind>(
  instanceID: string,
  kind: K,
  enabled = true,
) {
  const fn = NODE_LIVE_FN[kind];
  return useQuery<NodeLiveItemMap[K][]>({
    queryKey: ['remote', 'nodes', instanceID, ALL_MIRROR_KEY[kind], 'live'],
    queryFn: () => fn(instanceID),
    enabled: enabled && !!instanceID,
    refetchInterval: MIRROR_REFETCH_MS,
    // 구버전 노드(엔드포인트 부재)는 즉시 미러로 폴백하므로 재시도하지 않는다.
    retry: false,
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

// ---- 노드 그룹 배정/해제 뮤테이션 (v1.4 M9, 그룹 K, REQ-K02/K05) ----

/**
 * 그룹 변경 후 노드 목록 + 그룹 목록 쿼리를 무효화한다.
 *
 * 노드의 group_name 과 distinct 그룹 집계가 함께 바뀌므로 둘 다 무효화한다.
 */
function invalidateGroupQueries(
  queryClient: ReturnType<typeof useQueryClient>,
): void {
  queryClient.invalidateQueries({ queryKey: ['remote', 'nodes'] });
  queryClient.invalidateQueries({ queryKey: GROUPS_KEY });
}

/**
 * 노드 그룹 배정/변경 뮤테이션 (REQ-K02). 성공 시 노드/그룹 쿼리 무효화.
 *
 * 빈 group_name 은 해제("전체" 환원)와 동일 의미이다(REQ-K05). 미존재(404) 등
 * 에러는 APIError 로 호출자에게 전파된다.
 */
export function useSetNodeGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      instanceID,
      groupName,
    }: {
      instanceID: string;
      groupName: string;
    }) => remoteService.setRemoteNodeGroup(instanceID, groupName),
    onSuccess: () => invalidateGroupQueries(queryClient),
  });
}

/**
 * 노드 그룹 해제 뮤테이션 ("전체" 환원, REQ-K02/K05). 성공 시 노드/그룹 쿼리 무효화.
 */
export function useClearNodeGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (instanceID: string) =>
      remoteService.clearRemoteNodeGroup(instanceID),
    onSuccess: () => invalidateGroupQueries(queryClient),
  });
}

// ---- 노드 디스플레이 해상도 오버라이드 뮤테이션 (v1.6 M12) ----

/**
 * 디스플레이 오버라이드 변경 후 해당 노드의 상세 쿼리 + 노드 목록을 무효화한다.
 *
 * 노드-상세(EFFECTIVE 해상도 포함)와 목록을 갱신한다. 상세 쿼리 무효화는 같은
 * 캐시를 공유하는 고정 캔버스(DashboardCanvas)가 새 해상도로 재렌더되도록 한다
 * (캔버스 코드 변경 없이 — REQ-M03). 부분 일치를 위해 prefix 키로 무효화한다.
 */
function invalidateNodeDisplayQueries(
  queryClient: ReturnType<typeof useQueryClient>,
  instanceID: string,
): void {
  // 노드-상세 쿼리(['remote','nodes',id,'detail'])는 노드 prefix 로 함께 무효화된다.
  queryClient.invalidateQueries({ queryKey: ['remote', 'nodes', instanceID] });
  queryClient.invalidateQueries({ queryKey: ['remote', 'nodes'] });
}

/**
 * 노드 디스플레이 해상도 오버라이드 설정/변경 뮤테이션 (v1.6 M12).
 * 성공 시 노드-상세 + 목록 쿼리 무효화 → 고정 캔버스가 새 해상도로 재렌더된다.
 *
 * width/height 는 양의 정수여야 한다(비양수는 400). 미존재(404)/비-admin(403)/
 * 미관리(503) 등 에러는 APIError 로 호출자에게 전파된다(editError 로 매핑 표시).
 */
export function useSetNodeDisplay() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      instanceID,
      width,
      height,
    }: {
      instanceID: string;
      width: number;
      height: number;
    }) => remoteService.setRemoteNodeDisplay(instanceID, width, height),
    onSuccess: (_data, variables) =>
      invalidateNodeDisplayQueries(queryClient, variables.instanceID),
  });
}

/**
 * 노드 디스플레이 해상도 오버라이드 해제 뮤테이션 (v1.6 M12).
 * 성공 시 노드-상세 + 목록 쿼리 무효화 → EFFECTIVE 해상도가 노드 보고값/폴백으로 환원.
 */
export function useClearNodeDisplay() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (instanceID: string) =>
      remoteService.clearRemoteNodeDisplay(instanceID),
    onSuccess: (_data, instanceID) =>
      invalidateNodeDisplayQueries(queryClient, instanceID),
  });
}

// ---- 버전 관리 (Phase 1/2) ----

const TARGET_VERSION_KEY = ['remote', 'target-version'] as const;
const VERSION_HISTORY_KEY = (instanceID: string) =>
  ['remote', 'nodes', instanceID, 'version-history'] as const;

/** 서버 전역 목표 버전 조회 쿼리. */
export function useTargetVersion(enabled = true) {
  return useQuery({
    queryKey: TARGET_VERSION_KEY,
    queryFn: () => remoteService.getTargetVersion(),
    enabled,
  });
}

/**
 * 서버 전역 목표 버전 설정 뮤테이션. 성공 시 목표 버전 + 노드 목록(outdated 재계산)을
 * 무효화한다.
 */
export function useSetTargetVersion() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (version: string) => remoteService.setTargetVersion(version),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: TARGET_VERSION_KEY });
      queryClient.invalidateQueries({ queryKey: ['remote', 'nodes'] });
    },
  });
}

const UPDATE_SOURCE_KEY = ['remote', 'update-source'] as const;

/** 서버 저장 업데이트 소스(GitHub/자체 호스팅) 조회 쿼리. */
export function useUpdateSource(enabled = true) {
  return useQuery({
    queryKey: UPDATE_SOURCE_KEY,
    queryFn: () => remoteService.getUpdateSource(),
    enabled,
  });
}

/** 서버 저장 업데이트 소스 설정 뮤테이션. 성공 시 소스 쿼리를 무효화한다. */
export function useSetUpdateSource() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (source: UpdateSource) => remoteService.setUpdateSource(source),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: UPDATE_SOURCE_KEY });
    },
  });
}

// ---- 릴리스 저장소 (관리 서버 호스팅 프로그램 이미지) ----

/** 릴리스 목록 쿼리 키. */
const RELEASES_KEY = ['remote', 'releases'] as const;

/** 릴리스 뮤테이션 성공 시 릴리스 목록 쿼리를 무효화한다. */
function invalidateReleases(
  queryClient: ReturnType<typeof useQueryClient>,
): void {
  queryClient.invalidateQueries({ queryKey: RELEASES_KEY });
}

/**
 * 릴리스 버전 목록 쿼리.
 *
 * @param enabled - 쿼리 활성 여부. server 모드가 아니면 false 로 발행을 막는다.
 */
export function useReleases(enabled = true) {
  return useQuery({
    queryKey: RELEASES_KEY,
    queryFn: () => remoteService.listReleases(),
    enabled,
  });
}

/**
 * 릴리스 버전 생성/갱신 뮤테이션. 성공 시 릴리스 목록 무효화.
 *
 * 잘못된 semver(400) 등 에러는 APIError 로 호출자에게 전파된다.
 */
export function useCreateRelease() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (req: ReleaseCreateRequest) => remoteService.createRelease(req),
    onSuccess: () => invalidateReleases(queryClient),
  });
}

/**
 * 릴리스 자산(아키텍처별 바이너리 + 서명) 업로드 뮤테이션. 성공 시 릴리스 목록 무효화.
 *
 * 미존재 버전(404) 등 에러는 APIError 로 호출자에게 전파된다.
 */
export function useUploadReleaseAsset() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      version,
      os,
      arch,
      binary,
      signature,
    }: {
      version: string;
      os: string;
      arch: string;
      binary: File;
      signature: File;
    }) => remoteService.uploadReleaseAsset(version, os, arch, binary, signature),
    onSuccess: () => invalidateReleases(queryClient),
  });
}

/** 릴리스 버전 삭제 뮤테이션. 성공 시 릴리스 목록 무효화. */
export function useDeleteRelease() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (version: string) => remoteService.deleteRelease(version),
    onSuccess: () => invalidateReleases(queryClient),
  });
}

/** 릴리스 자산(한 아키텍처) 삭제 뮤테이션. 성공 시 릴리스 목록 무효화. */
export function useDeleteReleaseAsset() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      version,
      os,
      arch,
    }: {
      version: string;
      os: string;
      arch: string;
    }) => remoteService.deleteReleaseAsset(version, os, arch),
    onSuccess: () => invalidateReleases(queryClient),
  });
}

/** 노드 버전 변경 이력 조회 쿼리(최신순). */
export function useNodeVersionHistory(instanceID: string, enabled = true) {
  return useQuery({
    queryKey: VERSION_HISTORY_KEY(instanceID),
    queryFn: () => remoteService.getNodeVersionHistory(instanceID),
    enabled: enabled && !!instanceID,
  });
}

/**
 * 노드 원격 업데이트(system/update) 뮤테이션 (Phase 2). 성공 시 노드 prefix(상세/버전)
 * + 목록 + 해당 노드 버전 이력을 무효화한다. 미승인/오프라인(503)·타임아웃(504)·적용
 * 실패(502) 는 APIError 로 호출자에게 전파된다.
 */
export function useUpdateNode() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      instanceID,
      req,
    }: {
      instanceID: string;
      req: NodeUpdateRequest;
    }) => remoteService.updateNode(instanceID, req),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['remote', 'nodes', variables.instanceID] });
      queryClient.invalidateQueries({ queryKey: ['remote', 'nodes'] });
      queryClient.invalidateQueries({ queryKey: VERSION_HISTORY_KEY(variables.instanceID) });
    },
  });
}

// ---- 그룹 관리(일괄) ----

/** 그룹 변경 뮤테이션 성공 시 노드 목록 + 그룹 목록을 무효화한다. */
function invalidateGroupOps(queryClient: ReturnType<typeof useQueryClient>): void {
  queryClient.invalidateQueries({ queryKey: ['remote', 'nodes'] });
  queryClient.invalidateQueries({ queryKey: GROUPS_KEY });
}

/** 그룹 일괄 이름변경 뮤테이션. */
export function useRenameGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ oldName, newName }: { oldName: string; newName: string }) =>
      remoteService.renameGroup(oldName, newName),
    onSuccess: () => invalidateGroupOps(queryClient),
  });
}

/** 그룹 삭제(멤버를 "전체"로 이동) 뮤테이션. */
export function useDeleteGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => remoteService.deleteGroup(name),
    onSuccess: () => invalidateGroupOps(queryClient),
  });
}

/** 그룹 일괄 원격 업데이트 뮤테이션. 성공 시 노드/그룹 무효화(버전 변동 반영). */
export function useUpdateGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, req }: { name: string; req: GroupUpdateRequest }) =>
      remoteService.updateGroup(name, req),
    onSuccess: () => invalidateGroupOps(queryClient),
  });
}

/** 그룹 일괄 명령 뮤테이션. */
export function useCommandGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, req }: { name: string; req: CommandRequest }) =>
      remoteService.commandGroup(name, req),
    onSuccess: () => invalidateGroupOps(queryClient),
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
