import type { ListOptions } from '@/types/api';
import type {
  FlowCreateRequest,
  FlowInfo,
  FlowNodeInfo,
  FlowStatusInfo,
  FlowUpdateRequest,
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

// ---- Export ----

export interface FlowExport {
  name: string;
  description?: string;
  definition: { nodes: unknown[]; wires: unknown[] };
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
