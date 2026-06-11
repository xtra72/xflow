import type { ListOptions } from '@/types/api';
import type {
  FlowCreateRequest,
  FlowInfo,
  FlowNodeInfo,
  FlowStatusInfo,
  FlowUpdateRequest,
  SubflowStatsInfo,
} from '@/types/flow';

import { del, get, getList, post, put } from './client';

// ---- CRUD ----

/**
 * List flows with optional pagination, sorting, and filtering.
 */
export async function getFlows(
  params?: ListOptions,
): Promise<{ data: FlowInfo[]; total: number }> {
  return getList<FlowInfo>('/flows', { params });
}

/**
 * Get a single flow by ID.
 */
export async function getFlow(id: string): Promise<FlowInfo> {
  return get<FlowInfo>(`/flows/${id}`);
}

/**
 * Create a new flow definition.
 */
export async function createFlow(req: FlowCreateRequest): Promise<FlowInfo> {
  return post<FlowInfo>('/flows', req);
}

/**
 * Update an existing flow definition.
 */
export async function updateFlow(id: string, req: FlowUpdateRequest): Promise<FlowInfo> {
  return put<FlowInfo>(`/flows/${id}`, req);
}

/**
 * Delete a flow by ID.
 */
export async function deleteFlow(id: string): Promise<void> {
  return del(`/flows/${id}`);
}

// ---- Lifecycle ----

/**
 * Deploy a flow, making it ready to start.
 */
export async function deployFlow(id: string): Promise<void> {
  await post<void>(`/flows/${id}/deploy`);
}

/**
 * Start a deployed flow.
 */
export async function startFlow(id: string): Promise<void> {
  await post<void>(`/flows/${id}/start`);
}

/**
 * Stop a running flow.
 */
export async function stopFlow(id: string): Promise<void> {
  await post<void>(`/flows/${id}/stop`);
}

/**
 * Restart a flow (stop then start).
 */
export async function restartFlow(id: string): Promise<void> {
  await post<void>(`/flows/${id}/restart`);
}

/**
 * Undeploy a flow, removing it from engine memory back to stored state.
 */
export async function undeployFlow(id: string): Promise<void> {
  await post<void>(`/flows/${id}/undeploy`);
}

// ---- Config ----

/**
 * Update flow configuration without replacing the entire flow.
 */
export async function updateFlowConfig(
  id: string,
  config: Record<string, unknown>,
): Promise<void> {
  await put<void>(`/flows/${id}/config`, config);
}

// ---- Status & Nodes ----

/**
 * Get flow runtime status including node stats.
 */
export async function getFlowStatus(id: string): Promise<FlowStatusInfo> {
  return get<FlowStatusInfo>(`/flows/${id}/status`);
}

/**
 * List all nodes belonging to a flow.
 */
export async function getFlowNodes(flowId: string): Promise<FlowNodeInfo[]> {
  return get<FlowNodeInfo[]>(`/flows/${flowId}/nodes`);
}

/**
 * Get a single node within a flow.
 */
export async function getFlowNode(flowId: string, nodeId: string): Promise<FlowNodeInfo> {
  return get<FlowNodeInfo>(`/flows/${flowId}/nodes/${nodeId}`);
}

/**
 * 서브플로우 {flowId} 의 LIVE per-original-node 통계를 조회한다(Fix 2).
 *
 * {flowId} 를 LOCAL 참조하는 배포 부모 플로우의 네임스페이스 노드
 * (subflow_<flowNodeID>_*)에서 원본 노드 단위로 집계된 통계를 반환한다.
 * 서브플로우가 단독 배포되지 않고 부모 안에서만 실행될 때, 에디터가 서브플로우를
 * 단독으로 열어도 실제 처리 중인 per-node 카운트를 표시하기 위해 사용한다.
 * 참조하는 실행 부모가 없으면 nodes 가 빈 배열인 응답을 반환한다(200).
 */
export async function getSubflowStats(flowId: string): Promise<SubflowStatsInfo> {
  return get<SubflowStatsInfo>(`/flows/${flowId}/subflow-stats`);
}

// ---- Node Output Tap (관찰) ----

/** `POST /flows/{id}/nodes/{nodeId}/tap` 응답 데이터. */
export interface NodeTapResult {
  flow_id: string;
  node_id: string;
  enabled: boolean;
}

/** `GET /flows/{id}/taps` 응답 데이터. */
export interface FlowTaps {
  flow_id: string;
  node_ids: string[];
}

/**
 * 노드 출력 tap(관찰) 상태를 토글한다.
 *
 * 와이어 연결 없이 임의 노드의 출력 메시지를 WebSocket(`node.output`)으로
 * 스트리밍하도록 런타임에 설정한다. 백엔드는 advisory 로 동작하므로 알 수
 * 없는 노드라도 hard-fail 하지 않는다. tap 상태는 런타임/인메모리 전용이며
 * 서버 재시작 시 초기화된다.
 *
 * @param flowId 대상 플로우 ID
 * @param nodeId 대상 노드 ID
 * @param enabled true=관찰 시작, false=관찰 중지
 */
export async function setNodeTap(
  flowId: string,
  nodeId: string,
  enabled: boolean,
): Promise<NodeTapResult> {
  return post<NodeTapResult>(`/flows/${flowId}/nodes/${nodeId}/tap`, { enabled });
}

/**
 * 현재 tap(관찰) 중인 노드 ID 목록을 조회한다.
 *
 * 에디터가 배포된 플로우를 열 때 서버의 현재 tap 상태를 복원하는 데 사용한다.
 *
 * @param flowId 대상 플로우 ID
 */
export async function getFlowTaps(flowId: string): Promise<FlowTaps> {
  return get<FlowTaps>(`/flows/${flowId}/taps`);
}

// ---- Export ----

/** 플로우가 참조하는 에이전트 내보내기 정보. */
export interface AgentExportRef {
  name: string;
  type?: string;
  config?: Record<string, unknown>;
}

export interface FlowExport {
  name: string;
  description?: string;
  definition: { nodes: unknown[]; wires: unknown[] };
  required_agents?: AgentExportRef[];
}

/**
 * 단일 플로우를 내보내기용 포맷으로 조회한다.
 */
export async function exportFlow(id: string): Promise<FlowExport> {
  return get<FlowExport>(`/flows/${id}/export`);
}

/**
 * 모든 플로우를 내보내기용 포맷으로 조회한다.
 */
export async function exportAllFlows(): Promise<FlowExport[]> {
  return get<FlowExport[]>(`/flows/export`);
}
