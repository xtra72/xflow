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
/** 메시지 카운터 그룹 */
export interface MessageCounters {
  received: number;
  sent: number;
  errored: number;
}

/** 전체/외부/내부 메시지 통계 */
export interface EnhancedMessagesStats {
  total: MessageCounters;
  external: MessageCounters;
  internal: MessageCounters;
}

/** 바이트 I/O 통계 */
export interface BytesStats {
  read: number;
  written: number;
}

/** 메시지 버퍼 상태 */
export interface BufferStatsInfo {
  pending: number;
  capacity: number;
}

/** 에이전트 타입별 외부 연결 통계 */
export interface ConnectionStatsResponse {
  id: string;
  messages_received: number;
  messages_sent: number;
  messages_errored: number;
  bytes_read: number;
  bytes_written: number;
  connected_at: string;
  last_activity_at: string;
}

/** 노드 참조별 내부 통계 */
export interface NodeRefStatsResponse {
  node_id: string;
  node_name?: string;
  flow_id: string;
  flow_name?: string;
  messages_received: number;
  messages_sent: number;
  messages_errored: number;
  last_activity_at: string;
}

export interface AgentStatsInfo {
  // 기존 flat 필드 (하위 호환성)
  id: string;
  status: string;
  uptime?: string;
  messages_in: number;
  messages_out: number;
  error_count: number;
  connected: boolean;
  buffer_pending?: number;
  buffer_capacity?: number;

  // 새 중첩 구조 (SPEC-AGENT-004)
  messages?: EnhancedMessagesStats;
  bytes?: BytesStats;
  buffer?: BufferStatsInfo;
  dropped_messages?: number;
  load_time?: string;
  avg_processing_latency?: string;
  restart_count?: number;
  last_activity_at?: string;
  connections?: ConnectionStatsResponse[];
  node_refs?: NodeRefStatsResponse[];
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
