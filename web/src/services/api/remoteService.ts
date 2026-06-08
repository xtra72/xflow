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

import type { DashboardSnapshot } from '@/types/dashboard';
import type {
  CommandRequest,
  CommandResult,
  EnrollmentToken,
  EnrollmentTokenCreated,
  EnrollmentTokenCreateRequest,
  ManagedNode,
  MirroredResource,
  NodeDetail,
  NodeGroup,
  PreRegisterRequest,
  RemoteAgentCreateRequest,
  RemoteAgentUpdateRequest,
  RemoteFlowCreateRequest,
  RemoteFlowUpdateRequest,
  RemoteModeResponse,
  RemoteResourceResult,
} from '@/types/remote';

import { del, get, patch, post, put } from './client';

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

// ---- 노드 그룹핑 + 상세 (v1.4 M9, 그룹 K, REQ-K02~K06/K08/K10) ----
//
// 그룹은 서버 운영 메타데이터이므로(A13) 배정/해제는 managed_nodes.group_name
// 갱신만 수행하고 노드로 명령을 전파하지 않는다. 모든 엔드포인트는 admin 전용이다.

/**
 * distinct 그룹 + 노드 수 목록을 조회한다. GET /remote/groups (REQ-K03)
 *
 * 응답은 항상 "전체"(group_name="") 가상 버킷을 포함한다(그룹 미지정 노드 묶음).
 */
export async function listRemoteGroups(): Promise<NodeGroup[]> {
  return get<NodeGroup[]>('/remote/groups');
}

/**
 * 노드 상세(메타 + BASIC 시스템 정보 + uptime + 운영 요약)를 조회한다.
 * GET /remote/nodes/{instance_id} (REQ-K08/K10)
 *
 * uptime 은 started_at>0 일 때만 채워지며 미보고 노드는 null 이다(하위 호환).
 * 미존재 노드는 404 로 매핑되어 APIError 로 전파된다.
 */
export async function getRemoteNodeDetail(instanceID: string): Promise<NodeDetail> {
  return get<NodeDetail>(`/remote/nodes/${encodeId(instanceID)}`);
}

/**
 * 노드의 그룹을 배정/변경한다. PUT /remote/nodes/{instance_id}/group (REQ-K02)
 *
 * 본문 `{ group_name }`. 빈 문자열은 해제("전체" 환원)와 동일 의미이다(REQ-K05).
 * 미존재 노드는 404 로 매핑된다.
 */
export async function setRemoteNodeGroup(
  instanceID: string,
  groupName: string,
): Promise<void> {
  await put<unknown>(`/remote/nodes/${encodeId(instanceID)}/group`, {
    group_name: groupName,
  });
}

/**
 * 노드의 그룹을 해제하여 "전체"로 환원한다.
 * DELETE /remote/nodes/{instance_id}/group → 204 (REQ-K02/K05)
 */
export async function clearRemoteNodeGroup(instanceID: string): Promise<void> {
  await del(`/remote/nodes/${encodeId(instanceID)}/group`);
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

// ---- 원격 READ/QUERY 프록시 (M8, 그룹 J, REQ-J01/J04/J07/J11) ----
//
// 로컬 디테일 API 와 동형의 자원-타깃 READ 데이터를 노드 경유로 프록시한다.
// 백엔드(remote_query.go)는 노드가 redaction(J06)한 본문을 그대로 통과시키므로,
// 응답 형태는 매칭되는 로컬 디테일 API 와 동일하다(타깃 추상화가 base path 만
// 교체하면 된다 — REQ-J11). 모든 경로 접두사는 /remote/nodes/{instance_id}/... .
//
// 실패 의미(REQ-J07, 백엔드 mapRemoteQueryError → APIError 전파):
//   503=오프라인/미관리, 504=타임아웃, 502=노드 질의 실패, 404=노출 범위 밖,
//   403=비-admin.
//
// 라이브 action(agent.stats / agent.series / device.state)은 SSE 스트림(아래
// remoteStreamUrl)이 1차 소스이며, 본 GET 들은 폴백 폴링 경로로 쓰인다.

/** 한 노드의 원격 자원 디테일 base path 를 구성한다. */
function nodeResourcePath(
  instanceID: string,
  kindPlural: 'flows' | 'agents' | 'devices',
  resourceID: string,
): string {
  return `/remote/nodes/${encodeId(instanceID)}/${kindPlural}/${encodeId(resourceID)}`;
}

// --- flow READ 프록시 ---

/** 원격 플로우 상세(정의)를 조회한다. GET .../flows/{id} → flow/get */
export async function getRemoteFlow<T = unknown>(
  instanceID: string,
  flowID: string,
): Promise<T> {
  return get<T>(nodeResourcePath(instanceID, 'flows', flowID));
}

/** 원격 플로우 상태를 조회한다. GET .../flows/{id}/status → flow/status */
export async function getRemoteFlowStatus<T = unknown>(
  instanceID: string,
  flowID: string,
): Promise<T> {
  return get<T>(`${nodeResourcePath(instanceID, 'flows', flowID)}/status`);
}

/** 원격 플로우 노드 목록을 조회한다. GET .../flows/{id}/nodes → flow/nodes */
export async function getRemoteFlowNodes<T = unknown>(
  instanceID: string,
  flowID: string,
): Promise<T> {
  return get<T>(`${nodeResourcePath(instanceID, 'flows', flowID)}/nodes`);
}

/** 원격 단일 플로우 노드 런타임을 조회한다. GET .../flows/{id}/nodes/{nodeId} → flow/node */
export async function getRemoteFlowNode<T = unknown>(
  instanceID: string,
  flowID: string,
  nodeID: string,
): Promise<T> {
  return get<T>(
    `${nodeResourcePath(instanceID, 'flows', flowID)}/nodes/${encodeId(nodeID)}`,
  );
}

// --- agent READ 프록시 ---

/** 원격 에이전트 상세(detail=full)를 조회한다. GET .../agents/{id} → agent/get */
export async function getRemoteAgent<T = unknown>(
  instanceID: string,
  agentID: string,
): Promise<T> {
  return get<T>(nodeResourcePath(instanceID, 'agents', agentID));
}

/** 원격 에이전트 라이브 통계를 조회한다(폴백 폴링). GET .../agents/{id}/stats → agent/stats */
export async function getRemoteAgentStats<T = unknown>(
  instanceID: string,
  agentID: string,
): Promise<T> {
  return get<T>(`${nodeResourcePath(instanceID, 'agents', agentID)}/stats`);
}

/** 원격 에이전트 설정을 조회한다. GET .../agents/{id}/config → agent/config */
export async function getRemoteAgentConfig<T = unknown>(
  instanceID: string,
  agentID: string,
): Promise<T> {
  return get<T>(`${nodeResourcePath(instanceID, 'agents', agentID)}/config`);
}

/** 원격 에이전트 연결 디바이스를 조회한다. GET .../agents/{id}/devices → agent/devices */
export async function getRemoteAgentDevices<T = unknown>(
  instanceID: string,
  agentID: string,
): Promise<T> {
  return get<T>(`${nodeResourcePath(instanceID, 'agents', agentID)}/devices`);
}

/** 원격 에이전트 토픽을 조회한다. GET .../agents/{id}/topics → agent/topics */
export async function getRemoteAgentTopics<T = unknown>(
  instanceID: string,
  agentID: string,
): Promise<T> {
  return get<T>(`${nodeResourcePath(instanceID, 'agents', agentID)}/topics`);
}

/** 원격 에이전트 store 를 조회한다. GET .../agents/{id}/store → agent/store */
export async function getRemoteAgentStore<T = unknown>(
  instanceID: string,
  agentID: string,
): Promise<T> {
  return get<T>(`${nodeResourcePath(instanceID, 'agents', agentID)}/store`);
}

/** 원격 에이전트 세션을 조회한다. GET .../agents/{id}/sessions → agent/sessions */
export async function getRemoteAgentSessions<T = unknown>(
  instanceID: string,
  agentID: string,
): Promise<T> {
  return get<T>(`${nodeResourcePath(instanceID, 'agents', agentID)}/sessions`);
}

/** 원격 에이전트 시리즈를 조회한다(폴백 폴링). GET .../agents/{id}/series → agent/series */
export async function getRemoteAgentSeries<T = unknown>(
  instanceID: string,
  agentID: string,
): Promise<T> {
  return get<T>(`${nodeResourcePath(instanceID, 'agents', agentID)}/series`);
}

// --- device READ 프록시 ---

/** 원격 디바이스 상세를 조회한다. GET .../devices/{id} → device/get */
export async function getRemoteDevice<T = unknown>(
  instanceID: string,
  deviceID: string,
): Promise<T> {
  return get<T>(nodeResourcePath(instanceID, 'devices', deviceID));
}

/** 원격 디바이스 실시간 상태를 조회한다(폴백 폴링). GET .../devices/{id}/state → device/state */
export async function getRemoteDeviceState<T = unknown>(
  instanceID: string,
  deviceID: string,
): Promise<T> {
  return get<T>(`${nodeResourcePath(instanceID, 'devices', deviceID)}/state`);
}

/** 원격 디바이스 명령 스펙을 조회한다. GET .../devices/{id}/commands → device/commands */
export async function getRemoteDeviceCommands<T = unknown>(
  instanceID: string,
  deviceID: string,
): Promise<T> {
  return get<T>(`${nodeResourcePath(instanceID, 'devices', deviceID)}/commands`);
}

/** 원격 디바이스 메타데이터를 조회한다. GET .../devices/{id}/metadata → device/metadata */
export async function getRemoteDeviceMetadata<T = unknown>(
  instanceID: string,
  deviceID: string,
): Promise<T> {
  return get<T>(`${nodeResourcePath(instanceID, 'devices', deviceID)}/metadata`);
}

// ---- 원격 라이브 스트림 URL (M8, 그룹 J, REQ-J08) ----
//
// SSE 엔드포인트(remote_stream.go)는 EventSource 로 소비한다. EventSource 는
// Authorization 헤더를 설정할 수 없으므로 JWT 를 `?token=` 쿼리로 운반한다
// (백엔드 bearerOrQueryToken 이 Bearer/쿼리 양쪽을 수용). client baseURL(/api/v1)을
// 앞에 붙여 절대 경로를 만든다(EventSource 는 axios 인스턴스를 거치지 않음).

/** 스트림 가능한 라이브 action. */
export type RemoteStreamKind =
  | { domain: 'device'; action: 'state' }
  | { domain: 'agent'; action: 'stats' }
  | { domain: 'agent'; action: 'series' };

/**
 * 원격 라이브 스트림 SSE URL 을 구성한다(REQ-J08). token 이 주어지면 `?token=`
 * 쿼리로 부착한다(EventSource 헤더 제약 우회). 경로는 remote_stream.go 의
 * streamRoutes 와 일치한다.
 */
export function remoteStreamUrl(
  instanceID: string,
  kind: RemoteStreamKind,
  resourceID: string,
  token?: string,
): string {
  const plural = kind.domain === 'device' ? 'devices' : 'agents';
  const base = `/api/v1/remote/nodes/${encodeId(instanceID)}/${plural}/${encodeId(
    resourceID,
  )}/${kind.action}/stream`;
  return token ? `${base}?token=${encodeURIComponent(token)}` : base;
}

// ---- 원격 대시보드 패리티 (M10, 그룹 L, REQ-L01/L05/L06/L07) ----
//
// 대시보드 config(읽기 프록시) + 시스템 메트릭(query-action)은 노드-레벨 READ
// 자원이므로 per-resource 노출 범위가 없다(REQ-L03). 백엔드(remote_query.go)는
// 노드가 redaction(J06)한 본문을 그대로 통과시키므로 응답 형태는 로컬과 동일하다:
//   GET .../dashboards/shared → DashboardSnapshot (dashboard/get_shared)
//   GET .../dashboards/mine   → DashboardSnapshot (dashboard/get_mine, owner=JWT)
//   GET .../metrics           → 메트릭 스냅샷 (monitor/metrics)
// 실패 의미는 그룹 J 와 동일(503/504/502, 미설정 404 — mapRemoteQueryError).

/** 대시보드 config 스코프(로컬 탭과 동일 의미). */
export type RemoteDashboardScope = 'shared' | 'mine';

/**
 * 원격 노드의 대시보드 config(스코프별)를 READ-ONLY 로 조회한다(REQ-L01).
 * GET /remote/nodes/{id}/dashboards/{shared|mine}
 *
 * 노드-로컬 권위(A17): config 내부 deviceId 는 그 노드 기준으로 해석된다(REQ-L03).
 * 미설정 노드는 404 로 매핑되어 APIError 로 전파된다(빈 대시보드 처리는 호출자).
 */
export async function getRemoteDashboard(
  instanceID: string,
  scope: RemoteDashboardScope,
): Promise<DashboardSnapshot> {
  return get<DashboardSnapshot>(
    `/remote/nodes/${encodeId(instanceID)}/dashboards/${scope}`,
  );
}

/**
 * 원격 노드의 시스템 메트릭 스냅샷을 조회한다(REQ-L05, monitor/metrics).
 * GET /remote/nodes/{id}/metrics
 *
 * 로컬 `/monitor/metrics` 와 동형(필드명 동일)이므로 ResourceWidget 이 그대로
 * 소비한다. 서버는 단기 TTL 캐시(REQ-J16)·게이팅(REQ-J05)을 적용한다.
 */
export async function getRemoteMetrics(
  instanceID: string,
): Promise<Record<string, unknown>> {
  return get<Record<string, unknown>>(`/remote/nodes/${encodeId(instanceID)}/metrics`);
}

/**
 * 원격 차트 채널 라이브 스트림 SSE URL 을 구성한다(REQ-L07). 별도 WS 경로를
 * 신설하지 않고 M8 스트림 프록시에 추가된 `chart` stream-action 을 사용한다. 프레임은
 * 로컬 `/ws/chart/{channel}` 와 동일한 chart.backfill/chart.append 형태이다.
 * 경로는 remote_stream.go 의 charts/{channel}/stream 라우트와 일치한다.
 */
export function remoteChartStreamUrl(
  instanceID: string,
  channelName: string,
  token?: string,
): string {
  const base = `/api/v1/remote/nodes/${encodeId(instanceID)}/charts/${encodeId(
    channelName,
  )}/stream`;
  return token ? `${base}?token=${encodeURIComponent(token)}` : base;
}

/**
 * 원격 로그 라이브 스트림 SSE URL 을 구성한다(REQ-L06, monitor/logs). 노드-레벨
 * 스트림(자원 식별자 없음)이며 프레임은 로컬 log.entry 형태이다. 경로는
 * remote_stream.go 의 logs/stream 라우트와 일치한다.
 */
export function remoteLogsStreamUrl(instanceID: string, token?: string): string {
  const base = `/api/v1/remote/nodes/${encodeId(instanceID)}/logs/stream`;
  return token ? `${base}?token=${encodeURIComponent(token)}` : base;
}
