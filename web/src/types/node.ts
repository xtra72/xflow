// Node type definitions matching Go structs in internal/api/handler/node.go

/**
 * Node category classification.
 */
export type NodeCategory = 'input' | 'output' | 'process' | 'bridge' | 'special';

/**
 * Node type information from the registry.
 * Maps to Go NodeTypeInfo struct in handler/node.go.
 */
export interface NodeTypeInfo {
  type: string;
  category: string;
  description: string;
  source: string;
}

/**
 * Configuration field grouping section (HVACR 4-quadrant layout).
 *
 * - transport: 연결 방식, serial/TCP 파라미터, 타임아웃, 재연결
 * - protocol: 프로토콜별 옵션 (verify_*, master/slave addr, status_query_* 등)
 * - operation: 디바이스 발견, 오프라인 판정, 상태보고 / 이벤트 보고
 * - logging: raw_hex / decode / drop / state log 등 진단 옵션
 *
 * HVACR 외 에이전트는 기존 TwoColumnConfigLayout 이 그대로 사용한다 (optional 필드).
 */
export type ConfigSection = 'transport' | 'protocol' | 'operation' | 'logging';

/**
 * Individual configuration field definition for a node type.
 */
export interface ConfigField {
  name: string;
  type: 'string' | 'multiline' | 'number' | 'boolean' | 'select' | 'object' | 'string_list' | 'agent_select' | 'register_map' | 'transform_pipeline' | 'key_value_map' | 'typed_key_value_map' | 'trigger_schedules' | 'compare_fields';
  label: string;
  required?: boolean;
  default?: unknown;
  options?: string[];
  description?: string;
  /** 다른 필드 값에 따라 조건부 표시.
   *  value: 값 일치 / notEmpty: 비어있지 않을 때 표시 */
  visibleWhen?: { field: string; value?: unknown | unknown[]; notEmpty?: boolean };
  /** true 이면 고급 설정 섹션으로 분리되어 기본 접힘 상태로 표시된다. */
  advanced?: boolean;
  /** HVACR 4-quadrant 레이아웃에서 어느 분면에 속하는지를 지정한다.
   *  HVACR 외 에이전트는 미설정으로 둘 수 있다. */
  section?: ConfigSection;
}

/**
 * Configuration schema describing the fields a node type accepts.
 */
export interface ConfigSchema {
  fields: ConfigField[];
}

/**
 * Extended node type definition with UI metadata.
 * Extends NodeTypeInfo with icon, port definitions, and config schema.
 */
export interface NodeTypeDefinition extends NodeTypeInfo {
  icon?: string;
  ports?: {
    name: string;
    direction: 'input' | 'output' | 'error';
  }[];
  config_schema?: ConfigSchema;
}
