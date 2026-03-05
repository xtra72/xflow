// Flow domain types matching Go structs in internal/api/handler/flow.go
// and request DTOs in internal/api/dto/request.go

/**
 * Flow lifecycle status values.
 */
export type FlowStatus = 'stored' | 'loaded' | 'running' | 'stopped' | 'error';

/**
 * Flow information returned by the API.
 * Maps to Go FlowInfo struct in handler/flow.go.
 */
export interface FlowInfo {
  id: string;
  name: string;
  description?: string;
  status: string;
  created_at?: string;
  updated_at?: string;
  node_count: number;
  config?: Record<string, unknown>;
}

/**
 * Per-node statistics within a flow.
 * Maps to Go NodeStatInfo struct.
 */
export interface NodeStatInfo {
  node_id: string;
  node_name: string;
  node_type: string;
  processed: number;
  errors: number;
}

/**
 * Detailed flow status including runtime statistics.
 * Maps to Go FlowStatusInfo struct.
 */
export interface FlowStatusInfo {
  id: string;
  status: string;
  uptime?: string;
  node_stats?: NodeStatInfo[];
  message_count: number;
  error_count: number;
}

/**
 * Port runtime information for a node instance.
 * Maps to Go PortInfo struct.
 */
export interface PortInfo {
  id: string;
  name: string;
  direction: string;
  connected: boolean;
  messages: number;
  throughput: string;
  active_for: string;
}

/**
 * Node instance runtime information within a flow.
 * Maps to Go FlowNodeInfo struct.
 */
export interface FlowNodeInfo {
  node_id: string;
  name: string;
  type: string;
  state: string;
  config?: Record<string, unknown>;
  ports?: PortInfo[];
  extra?: Record<string, unknown>;
}

/**
 * Request body for creating a new flow.
 * Maps to Go FlowCreateRequest DTO.
 */
export interface FlowCreateRequest {
  name: string;
  description?: string;
  definition: Record<string, unknown>;
}

/**
 * Request body for updating an existing flow.
 * All fields are optional for partial updates.
 * Maps to Go FlowUpdateRequest DTO.
 */
export interface FlowUpdateRequest {
  name?: string;
  description?: string;
  definition?: Record<string, unknown>;
}
