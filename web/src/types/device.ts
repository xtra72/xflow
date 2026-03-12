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
  tags: string[];
  location: string;
  group: string;
  labels: Record<string, string>;
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
 */
export interface DeviceInfo {
  id: string;
  name: string;
  /** Device type: indoor, outdoor, sensor, etc. */
  type: string;
  /** Communication protocol: nasa, modbus */
  protocol: string;
  agent_name: string;
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
  tags?: string[];
  location?: string;
  group?: string;
  labels?: Record<string, string>;
}
