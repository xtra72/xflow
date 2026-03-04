import type { ListOptions } from '@/types/api';
import type {
  AgentCreateRequest,
  AgentExecRequest,
  AgentExecResponse,
  AgentInfo,
  AgentStatsInfo,
  AgentUpdateRequest,
} from '@/types/agent';

import { del, get, getList, post, put } from './client';

// ---- CRUD ----

/**
 * List agents with optional pagination, sorting, and filtering.
 */
export async function getAgents(
  params?: ListOptions,
): Promise<{ data: AgentInfo[]; total: number }> {
  return getList<AgentInfo>('/agents', { params });
}

/**
 * Get a single agent by ID.
 */
export async function getAgent(id: string): Promise<AgentInfo> {
  return get<AgentInfo>(`/agents/${id}`);
}

/**
 * Create a new agent definition.
 */
export async function createAgent(req: AgentCreateRequest): Promise<AgentInfo> {
  return post<AgentInfo>('/agents', req);
}

/**
 * Update an existing agent definition.
 */
export async function updateAgent(id: string, req: AgentUpdateRequest): Promise<AgentInfo> {
  return put<AgentInfo>(`/agents/${id}`, req);
}

/**
 * Delete an agent by ID.
 */
export async function deleteAgent(id: string): Promise<void> {
  return del(`/agents/${id}`);
}

// ---- Lifecycle ----

/**
 * Start an agent.
 */
export async function startAgent(id: string): Promise<void> {
  await post<void>(`/agents/${id}/start`);
}

/**
 * Stop a running agent.
 */
export async function stopAgent(id: string): Promise<void> {
  await post<void>(`/agents/${id}/stop`);
}

/**
 * Restart an agent (stop then start).
 */
export async function restartAgent(id: string): Promise<void> {
  await post<void>(`/agents/${id}/restart`);
}

// ---- Config ----

/**
 * Update agent configuration.
 */
export async function configureAgent(
  id: string,
  config: Record<string, unknown>,
): Promise<void> {
  await put<void>(`/agents/${id}/config`, { config });
}

// ---- Stats ----

/**
 * Get agent runtime statistics (messages processed, uptime, etc.).
 */
export async function getAgentStats(id: string): Promise<AgentStatsInfo> {
  return get<AgentStatsInfo>(`/agents/${id}/stats`);
}

// ---- Exec ----

/**
 * Execute a process command on an agent (e.g. manual publish, diag).
 */
export async function execAgent(
  id: string,
  req: AgentExecRequest,
): Promise<AgentExecResponse> {
  return post<AgentExecResponse>(`/agents/${id}/exec`, req);
}
