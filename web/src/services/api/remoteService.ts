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
  EnrollmentToken,
  EnrollmentTokenCreated,
  EnrollmentTokenCreateRequest,
  ManagedNode,
  MirroredResource,
  PreRegisterRequest,
  RemoteAgentCreateRequest,
  RemoteAgentUpdateRequest,
  RemoteFlowCreateRequest,
  RemoteFlowUpdateRequest,
  RemoteModeResponse,
  RemoteResourceResult,
} from '@/types/remote';

import { del, get, patch, post } from './client';

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

// ---- 노드 수동 등록 / 삭제 (수동 enrollment) ----

/**
 * 노드를 서버에서 직접 사전 등록한다. POST /remote/nodes
 *
 * 성공 시 status="approved", online=false 인 신규 노드가 생성된다 (201).
 * 누락된 instance_id 는 400, 중복은 409 로 매핑되어 APIError 로 전파된다.
 */
export async function preRegisterNode(
  req: PreRegisterRequest,
): Promise<ManagedNode> {
  return post<ManagedNode>('/remote/nodes', req);
}

/**
 * 등록된 노드를 완전히 삭제한다. DELETE /remote/nodes/{instance_id}
 *
 * 폐기(revoke)와 달리 항목 자체를 제거한다 (204). 미존재 시 404 로 매핑된다.
 */
export async function deleteNode(instanceID: string): Promise<void> {
  await del(`/remote/nodes/${encodeId(instanceID)}`);
}

// ---- Enrollment 토큰 (수동 enrollment) ----

/**
 * Enrollment 토큰을 발급한다. POST /remote/enrollment-tokens
 *
 * 응답 `token` 은 raw 값으로 1회만 노출된다 (201). 잘못된 duration 은 400.
 */
export async function createEnrollmentToken(
  req: EnrollmentTokenCreateRequest,
): Promise<EnrollmentTokenCreated> {
  return post<EnrollmentTokenCreated>('/remote/enrollment-tokens', req);
}

/**
 * Enrollment 토큰 메타데이터 목록을 조회한다. GET /remote/enrollment-tokens
 *
 * 보안상 raw 토큰 값은 포함되지 않는다 (메타데이터만 반환).
 */
export async function listEnrollmentTokens(): Promise<EnrollmentToken[]> {
  return get<EnrollmentToken[]>('/remote/enrollment-tokens');
}

/**
 * Enrollment 토큰을 폐기한다. DELETE /remote/enrollment-tokens/{id}
 *
 * 성공 시 204. 미존재 시 404 로 매핑된다.
 */
export async function revokeEnrollmentToken(id: string): Promise<void> {
  await del(`/remote/enrollment-tokens/${encodeId(id)}`);
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

// ---- 원격 자원 편집 (M7, 그룹 I, REQ-I01~I04/I08~I11) ----
//
// 모든 편집은 그룹 D 명령 경유로 (온라인) 노드에 전파되어 노드 어댑터가 로컬
// 적용한다. 서버는 결과 수신 후에만 미러 캐시를 갱신한다(REQ-E08/A4 — 서버
// 단독 영속 금지). 실패 의미: 오프라인=503, 타임아웃=504, 노드 적용 실패=502,
// 노출 범위 밖/미존재=404 (백엔드 mapRemoteCommandError / requireExposed).
//
// 시크릿(REQ-I07): 수정 페이로드의 마스킹/미변경 시크릿 필드는 호출자가
// 생략해야 한다(omitMaskedSecrets). 본 서비스 함수는 전달받은 본문을 그대로
// 전송하므로, 시크릿 생략은 호출 측(훅/페이지)에서 수행한다.

/**
 * 원격 노드에 새 플로우를 생성한다 (REQ-I01).
 * POST /remote/nodes/{instance_id}/flows  본문: { name, definition, description? }
 *
 * 성공 시 201 + 노드 채번 결과(RemoteResourceResult, `id` = node-assigned).
 */
export async function createRemoteFlow(
  instanceID: string,
  req: RemoteFlowCreateRequest,
): Promise<RemoteResourceResult> {
  return post<RemoteResourceResult>(`/remote/nodes/${encodeId(instanceID)}/flows`, req);
}

/**
 * 원격 노드의 기존 플로우를 수정한다 (REQ-I02).
 * PATCH /remote/nodes/{instance_id}/flows/{flow_id}  본문: { definition, name? }
 *
 * 본문의 마스킹/미변경 시크릿 필드는 호출 전 생략되어야 한다(REQ-I07).
 */
export async function updateRemoteFlow(
  instanceID: string,
  flowID: string,
  req: RemoteFlowUpdateRequest,
): Promise<RemoteResourceResult> {
  return patch<RemoteResourceResult>(
    `/remote/nodes/${encodeId(instanceID)}/flows/${encodeId(flowID)}`,
    req,
  );
}

/**
 * 원격 노드의 플로우를 삭제한다 (REQ-I03).
 * DELETE /remote/nodes/{instance_id}/flows/{flow_id} → 204
 */
export async function deleteRemoteFlow(
  instanceID: string,
  flowID: string,
): Promise<void> {
  await del(`/remote/nodes/${encodeId(instanceID)}/flows/${encodeId(flowID)}`);
}

/**
 * 원격 노드에 새 에이전트를 생성한다 (REQ-I04).
 * POST /remote/nodes/{instance_id}/agents  본문: { name, type, config? }
 *
 * 성공 시 201 + 노드 채번 결과(RemoteResourceResult, `id` = node-assigned).
 */
export async function createRemoteAgent(
  instanceID: string,
  req: RemoteAgentCreateRequest,
): Promise<RemoteResourceResult> {
  return post<RemoteResourceResult>(`/remote/nodes/${encodeId(instanceID)}/agents`, req);
}

/**
 * 원격 노드의 기존 에이전트를 수정한다 (REQ-I04).
 * PATCH /remote/nodes/{instance_id}/agents/{agent_id}  본문: { config?, name?, log_level? }
 *
 * 본문 config 의 마스킹/미변경 시크릿 필드는 호출 전 생략되어야 한다(REQ-I07).
 */
export async function updateRemoteAgent(
  instanceID: string,
  agentID: string,
  req: RemoteAgentUpdateRequest,
): Promise<RemoteResourceResult> {
  return patch<RemoteResourceResult>(
    `/remote/nodes/${encodeId(instanceID)}/agents/${encodeId(agentID)}`,
    req,
  );
}

/**
 * 원격 노드의 에이전트를 삭제한다 (REQ-I04).
 * DELETE /remote/nodes/{instance_id}/agents/{agent_id} → 204
 */
export async function deleteRemoteAgent(
  instanceID: string,
  agentID: string,
): Promise<void> {
  await del(`/remote/nodes/${encodeId(instanceID)}/agents/${encodeId(agentID)}`);
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
