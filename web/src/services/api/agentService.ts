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
  return getList<AgentInfo>('/agents', { params: { ...params, detail: 'summary' } });
}

/**
 * Get a single agent by ID.
 */
export async function getAgent(id: string, detail: 'summary' | 'full' = 'summary'): Promise<AgentInfo> {
  return get<AgentInfo>(`/agents/${id}?detail=${detail}`);
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

/**
 * Enable an agent (persistently mark as auto-start eligible).
 * Does NOT start the agent if currently stopped (SPEC-AGENT-005 R3.8).
 */
export async function enableAgent(id: string): Promise<AgentInfo> {
  return post<AgentInfo>(`/agents/${id}/enable`);
}

/**
 * Disable an agent (persistently mark as auto-start excluded).
 * Does NOT stop the agent if currently running (SPEC-AGENT-005 R3.7).
 */
export async function disableAgent(id: string): Promise<AgentInfo> {
  return post<AgentInfo>(`/agents/${id}/disable`);
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
 *
 * `POST /agents/{id}/exec` 는 `agent.execute` 권한을 요구하며 모든 커맨드를 수용한다.
 * 상태를 바꾸는 커맨드(add_* / remove_* / set_* / update_*)는 반드시 이 경로로 보낸다.
 * 읽기 전용 커맨드는 queryAgent 를 사용한다(아래).
 */
export async function execAgent(
  id: string,
  req: AgentExecRequest,
): Promise<AgentExecResponse> {
  return post<AgentExecResponse>(`/agents/${id}/exec`, req);
}

// ---- Query (읽기 전용) ----

/**
 * `POST /agents/{id}/query` 가 수용하는 읽기 전용 커맨드 목록 — 프론트엔드의 단일 정의처.
 *
 * 권한의 실제 판정은 서버가 한다. 서버 화이트리스트
 * (internal/api/handler/agent.go 의 `queryReadOnlyCommands`)가 유일한 권위이며,
 * 여기 있는 목록은 그 사본일 뿐이다. 서버에 추가하지 않은 이름을 여기에만 넣으면
 * 해당 호출은 400 으로 거부되고 패널이 비어 보인다. 반대로 서버에만 추가하고
 * 여기 넣지 않으면 타입 검사에서 queryAgent 호출이 막힌다.
 * 따라서 이 목록을 고칠 때는 반드시 서버 화이트리스트를 함께 고쳐야 한다.
 *
 * 목록이 서버와 어긋나는 것을 조용히 넘기지 않도록 agentService.query.test.ts 가
 * 이 배열을 12개 이름으로 고정(pin)한다.
 */
export const AGENT_QUERY_COMMANDS = [
  'list_devices',
  'list_clients',
  'list_stations',
  'list_lines',
  'list_groups',
  'list_gateways',
  'list_connections',
  'list_models',
  'get_status',
  'get_map',
  'get_device_status',
  // sysmetrics 에이전트의 표본 이력 조회. 메모리 버퍼를 읽기만 하며 상태를 바꾸지
  // 않는다 — 대시보드 차트 패널이 이 커맨드로 시계열을 가져온다.
  'get_history',
] as const;

/** queryAgent 가 허용하는 커맨드 이름(위 목록에서 파생). */
export type AgentQueryCommand = (typeof AGENT_QUERY_COMMANDS)[number];

/**
 * query 요청 형태. AgentExecRequest 와 구조가 같고 command 만 읽기 전용 목록으로 좁힌다.
 * 응답 형태는 exec 와 완전히 동일하므로(AgentExecResponse) 호출부의 응답 처리는 그대로다.
 */
export interface AgentQueryRequest {
  command: AgentQueryCommand;
  params?: Record<string, unknown>;
}

/**
 * 에이전트에 읽기 전용 커맨드를 보낸다.
 *
 * exec 와 실행 경로·응답 형태가 같지만 `agent.read` 권한만 요구하므로, 조회 전용
 * 역할(대시보드 패널 등)이 403 없이 데이터를 받을 수 있다. 거부(400/403)를 exec 로
 * 재시도하지 않는다 — 재시도는 권한 있는 사용자에게만 예전 동작을 되살려 목록 어긋남을
 * 가려버린다. 실패는 그대로 드러나야 한다.
 */
export async function queryAgent(
  id: string,
  req: AgentQueryRequest,
): Promise<AgentExecResponse> {
  return post<AgentExecResponse>(`/agents/${id}/query`, req);
}

// ---- Export ----

export interface AgentExport {
  name: string;
  type: string;
  config?: Record<string, unknown>;
}

/**
 * 단일 에이전트를 내보내기용 포맷으로 조회한다.
 */
export async function exportAgent(id: string): Promise<AgentExport> {
  return get<AgentExport>(`/agents/${id}/export`);
}

/**
 * 모든 에이전트를 내보내기용 포맷으로 조회한다.
 */
export async function exportAllAgents(): Promise<AgentExport[]> {
  return get<AgentExport[]>(`/agents/export`);
}
