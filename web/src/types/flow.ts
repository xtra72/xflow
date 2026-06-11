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
  uptime?: string;
  auto_start?: boolean;
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
  /** 노드가 이 포트로 생성(emit)한 메시지 누계. */
  messages: number;
  /**
   * 연결된 와이어로 실제 전달(delivered)된 메시지 누계.
   * 연결되지 않은 포트는 messages 가 증가해도 delivered 는 0 으로 유지된다.
   * messages - delivered = 큐에 적체(pending)된 수.
   */
  delivered: number;
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
 * 서브플로우 원본 노드 1개의 집계 런타임 통계.
 * Go handler.SubflowNodeStat 구조체와 매핑된다.
 *
 * getFlowNodes(FlowNodeInfo)와 동일한 형태(node_id/state/ports)에 더해
 * 부모 인스턴스 전체 합산값인 processed/errors 를 포함한다. node_id 는
 * 서브플로우 정의 내 ORIGINAL 노드 id 이다(네임스페이스 접두 제거됨).
 */
export interface SubflowNodeStat {
  node_id: string;
  name?: string;
  type?: string;
  state?: string;
  /** 모든 부모/flow-node 인스턴스 합산 처리 건수. */
  processed: number;
  /** 모든 부모/flow-node 인스턴스 합산 에러 건수. */
  errors: number;
  ports?: PortInfo[];
}

/**
 * `GET /flows/{id}/subflow-stats` 응답 데이터.
 * Go handler.SubflowStatsInfo 구조체와 매핑된다.
 *
 * 서브플로우 {id} 를 LOCAL 참조하는 배포 부모 플로우들의 네임스페이스 노드
 * (subflow_<flowNodeID>_*)에서 원본 노드 단위로 집계된다. 참조 부모가 없으면
 * nodes 는 빈 배열([])로 직렬화된다(200).
 */
export interface SubflowStatsInfo {
  flow_id: string;
  nodes: SubflowNodeStat[];
}

/**
 * Request body for creating a new flow.
 * Maps to Go FlowCreateRequest DTO.
 */
export interface FlowCreateRequest {
  name: string;
  description?: string;
  definition: Record<string, unknown>;
  /** true 이면 백엔드가 가져오기 시 노드 id 를 재생성한다(가져오기 전용).
   *  프런트엔드도 별도로 id 를 재생성하므로 belt-and-suspenders 동작이다. */
  regenerate_ids?: boolean;
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
  auto_start?: boolean;
}
