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
 * Individual configuration field definition for a node type.
 */
export interface ConfigField {
  name: string;
  type: 'string' | 'number' | 'boolean' | 'select' | 'object' | 'string_list' | 'agent_select' | 'register_map' | 'transform_pipeline' | 'key_value_map' | 'trigger_schedules';
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
