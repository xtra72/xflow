// 원격 관리 (remote management) REST API 서비스 레이어 (SPEC-REMOTE-001 M5).
//
// admin 전용 `/remote/*` 엔드포인트를 타입 안전하게 래핑한다. 모든 함수는
// `./client` 의 envelope 언래핑 헬퍼(get/post)를 사용하며, 백엔드는 api/v1
// 그룹 하위에 라우트를 등록한다 (client baseURL = /api/v1 이므로 경로는
// `/remote/...` 로 시작한다).
//
// 라우트 (internal/api/handler/remote_admin.go):
//   GET  /remote/nodes
//   GET  /remote/nodes/pending
//   POST /remote/nodes/{instance_id}/approve
//   POST /remote/nodes/{instance_id}/reject
//   POST /remote/nodes/{instance_id}/revoke
//   POST /remote/nodes/{instance_id}/command
//   GET  /remote/nodes/{instance_id}/flows|agents|devices
//   GET  /remote/flows|agents|devices

import type {
  CommandRequest,
  CommandResult,
  ManagedNode,
  MirroredResource,
  RemoteModeResponse,
} from '@/types/remote';

import { get, post } from './client';

/** instance_id 를 URL 경로에 안전하게 인코딩한다. */
function encodeId(instanceID: string): string {
  return encodeURIComponent(instanceID);
}

// ---- 동작 모드 조회 ----

/**
 * 인스턴스의 원격 관리 동작 모드를 조회한다. GET /remote/mode
 *
 * 표준 인증이 적용되며 server/client/disabled 모든 모드에서 200 을 반환한다.
 * UI 는 이 값으로 admin `/remote/*` 쿼리(server 전용) 발행 여부를 결정한다.
 */
export async function getRemoteMode(): Promise<RemoteModeResponse> {
  return get<RemoteModeResponse>('/remote/mode');
}

// ---- 노드 목록 조회 (G01/G02) ----

/**
 * 전체 관리 노드 목록을 조회한다. GET /remote/nodes
 */
export async function listNodes(): Promise<ManagedNode[]> {
  return get<ManagedNode[]>('/remote/nodes');
}

/**
 * 승인 대기(pending) 노드 큐를 조회한다. GET /remote/nodes/pending
 */
export async function listPendingNodes(): Promise<ManagedNode[]> {
  return get<ManagedNode[]>('/remote/nodes/pending');
}

// ---- 노드 상태 머신 액션 (G02) ----

/**
 * 노드를 승인한다 (REQ-C03). POST /remote/nodes/{instance_id}/approve
 */
export async function approveNode(instanceID: string): Promise<void> {
  await post<unknown>(`/remote/nodes/${encodeId(instanceID)}/approve`);
}

/**
 * 노드를 거부한다 (REQ-C03). POST /remote/nodes/{instance_id}/reject
 *
 * @param reason - 거부 사유 (선택적). 지정 시 본문 `{ reason }` 으로 전송된다.
 */
export async function rejectNode(instanceID: string, reason?: string): Promise<void> {
  await post<unknown>(
    `/remote/nodes/${encodeId(instanceID)}/reject`,
    reason ? { reason } : undefined,
  );
}

/**
 * 승인된 노드를 폐기한다 (REQ-C07). POST /remote/nodes/{instance_id}/revoke
 */
export async function revokeNode(instanceID: string): Promise<void> {
  await post<unknown>(`/remote/nodes/${encodeId(instanceID)}/revoke`);
}

// ---- 원격 명령 발행 (G04) ----

/**
 * 승인+온라인 노드에 원격 명령을 발행하고 결과를 기다린다 (M3, REQ-D01/D08).
 * POST /remote/nodes/{instance_id}/command  본문: { domain, action, args }
 *
 * 미승인/오프라인이면 503, 타임아웃이면 504, 노드 적용 실패면 502 로 매핑된다
 * (백엔드 mapRemoteCommandError). 에러는 APIError 로 호출자에게 전파된다.
 */
export async function sendCommand(
  instanceID: string,
  req: CommandRequest,
): Promise<CommandResult> {
  return post<CommandResult>(`/remote/nodes/${encodeId(instanceID)}/command`, req);
}

// ---- 노드별 미러 조회 (G03) ----

/**
 * 한 노드의 flow 미러 목록을 조회한다. GET /remote/nodes/{instance_id}/flows
 */
export async function listNodeFlows(instanceID: string): Promise<MirroredResource[]> {
  return get<MirroredResource[]>(`/remote/nodes/${encodeId(instanceID)}/flows`);
}

/**
 * 한 노드의 agent 미러 목록을 조회한다. GET /remote/nodes/{instance_id}/agents
 */
export async function listNodeAgents(instanceID: string): Promise<MirroredResource[]> {
  return get<MirroredResource[]>(`/remote/nodes/${encodeId(instanceID)}/agents`);
}

/**
 * 한 노드의 device 미러 목록을 조회한다. GET /remote/nodes/{instance_id}/devices
 */
export async function listNodeDevices(instanceID: string): Promise<MirroredResource[]> {
  return get<MirroredResource[]>(`/remote/nodes/${encodeId(instanceID)}/devices`);
}

// ---- 통합(전 노드) 미러 조회 (G03, REQ-E05) ----

/**
 * 전 노드의 flow 미러를 출처 태그와 함께 조회한다. GET /remote/flows
 */
export async function listAllFlows(): Promise<MirroredResource[]> {
  return get<MirroredResource[]>('/remote/flows');
}

/**
 * 전 노드의 agent 미러를 조회한다. GET /remote/agents
 */
export async function listAllAgents(): Promise<MirroredResource[]> {
  return get<MirroredResource[]>('/remote/agents');
}

/**
 * 전 노드의 device 미러를 조회한다. GET /remote/devices
 */
export async function listAllDevices(): Promise<MirroredResource[]> {
  return get<MirroredResource[]>('/remote/devices');
}
