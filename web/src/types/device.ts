// Device types matching Go structs in internal/api/handler/device.go

/**
 * Device state information.
 * Maps to Go DeviceState struct.
 */
export interface DeviceState {
  online: boolean;
  ready: boolean;
  last_seen: string;
  error_count: number;
  properties: Record<string, unknown>;
}

/**
 * Device metadata (tags, location, group, labels).
 * Maps to Go DeviceMetadata struct.
 */
export interface DeviceMetadata {
  name: string;
  tags: string[];
  location: string;
  group: string;
  labels: Record<string, string>;
  pinned?: boolean;
}

/**
 * Command parameter specification.
 * Maps to Go ParamSpec struct.
 */
export interface ParamSpec {
  name: string;
  /** Parameter type: string, int, float, bool, enum */
  type: string;
  required: boolean;
  min?: number;
  max?: number;
  enum?: string[];
}

/**
 * Device command specification.
 * Maps to Go CommandSpec struct.
 */
export interface CommandSpec {
  name: string;
  description: string;
  params: ParamSpec[];
}

/**
 * Device information returned by the list API.
 * Maps to Go DeviceResponse struct.
 *
 * SPEC-DEVICE-IDENTITY-001 Phase D (M11):
 * `uid` 가 1급 식별자 (글로벌 유일, 불변, UUID v4). 새 코드는 항상 `uid` 우선 사용.
 * `id` 는 Phase D (xflowd v1.0) 부터 UUID 를 반환하며 (시맨틱 변경),
 * Phase A~C 동안만 composite `agent:local_id` 형식. backend `omitempty`
 * 정책으로 `uid` 가 비어 있을 수 있음 — UUID 발급 저장소 미설정 시.
 * 사용자에게 노출하는 표시 라벨은 `metadata.name` → `name` → `agent_name`
 * 순서로 fallback (절대 `id` / `uid` 직접 노출 금지).
 */
export interface DeviceInfo {
  /** Composite key (Phase A~C) 또는 UUID (Phase D+). 새 코드는 `uid` 우선 사용. */
  id: string;
  /** SPEC-DEVICE-IDENTITY-001 Phase A — 글로벌 유일 UUID v4. backend `omitempty`. */
  uid?: string;
  name: string;
  /** Device type: indoor, outdoor, sensor, etc. */
  type: string;
  /** Communication protocol: samsung_nasa, modbus */
  protocol: string;
  agent_name: string;
  /** Device origin: "config", "auto", "pinned", "bridge" */
  source: string;
  online: boolean;
  last_seen: string;
  capabilities: string[];
  metadata?: DeviceMetadata;
}

/**
 * Device detail response (extends list item with state and commands).
 * Maps to Go DeviceDetailResponse struct.
 */
export interface DeviceDetail extends DeviceInfo {
  state?: DeviceState;
  commands?: CommandSpec[];
}

/**
 * Query parameters for listing devices.
 */
export interface DeviceListParams {
  protocol?: string;
  agent?: string;
  type?: string;
  online?: boolean;
  group?: string;
  tags?: string;
}

/**
 * Request body for executing a device command.
 * Maps to Go ExecuteRequest struct.
 */
export interface DeviceExecuteRequest {
  command: string;
  params?: Record<string, unknown>;
}

/**
 * Request body for updating device metadata.
 */
export interface DeviceMetadataUpdateRequest {
  name?: string;
  tags?: string[];
  location?: string;
  group?: string;
  labels?: Record<string, string>;
  pinned?: boolean;
}
