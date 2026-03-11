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
  type: 'string' | 'number' | 'boolean' | 'select' | 'object' | 'agent_select' | 'register_map' | 'transform_pipeline' | 'key_value_map';
  label: string;
  required?: boolean;
  default?: unknown;
  options?: string[];
  description?: string;
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
