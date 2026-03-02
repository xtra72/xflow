// Agent types matching Go structs in internal/api/handler/agent.go
// and request DTOs in internal/api/dto/request.go

/**
 * Agent health status summary.
 * Maps to Go AgentHealthInfo struct.
 */
export interface AgentHealthInfo {
  status: string;
  last_check: string;
}

/**
 * Agent message statistics summary.
 * Maps to Go AgentStatsResponse struct.
 */
export interface AgentStatsResponse {
  messages_in: number;
  messages_out: number;
  errors: number;
}

/**
 * Agent shared reference information.
 * Maps to Go AgentSharedInfo struct.
 */
export interface AgentSharedInfo {
  ref_count: number;
  flows: string[];
}

/**
 * Agent detailed statistics.
 * Maps to Go AgentStatsInfo struct.
 */
export interface AgentStatsInfo {
  id: string;
  status: string;
  uptime?: string;
  messages_in: number;
  messages_out: number;
  error_count: number;
  connected: boolean;
}

/**
 * Agent information returned by the API.
 * Maps to Go AgentInfo struct in handler/agent.go.
 */
export interface AgentInfo {
  id: string;
  name: string;
  type: string;
  status: string;
  config?: Record<string, unknown>;
  health?: AgentHealthInfo;
  stats?: AgentStatsResponse;
  uptime?: string;
  started_at?: string;
  created_at?: string;
  connected?: boolean;
  shared_info?: AgentSharedInfo;
  state?: Record<string, unknown>;
}

/**
 * Request body for creating a new agent.
 * Maps to Go AgentCreateRequest DTO.
 */
export interface AgentCreateRequest {
  name: string;
  type: string;
  config?: Record<string, unknown>;
}

/**
 * Request body for updating an existing agent.
 * Maps to Go AgentUpdateRequest DTO.
 */
export interface AgentUpdateRequest {
  name?: string;
  config?: Record<string, unknown>;
}

/**
 * Request body for executing an agent command.
 * Maps to Go AgentExecRequest DTO.
 */
export interface AgentExecRequest {
  command: string;
  params?: Record<string, unknown>;
}

/**
 * Response from agent command execution.
 */
export interface AgentExecResponse {
  result: unknown;
}
