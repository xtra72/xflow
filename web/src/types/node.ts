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
  type: 'string' | 'multiline' | 'number' | 'boolean' | 'select' | 'object' | 'string_list' | 'agent_select' | 'flow_picker' | 'register_map' | 'transform_pipeline' | 'key_value_map' | 'typed_key_value_map' | 'trigger_schedules' | 'compare_fields' | 'routes_editor' | 'metrics_editor';
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
  /** 비밀 값(비밀번호/토큰 등) 필드 여부.
   *  true 이면 입력 위젯이 password 타입으로 렌더링되고(값 마스킹),
   *  플로우 내보내기 시 백엔드에서 값이 제거된다(sensitive_fields 힌트로 노출).
   *  가져오기 시 사용자에게 재입력을 유도하는 데 사용된다. */
  sensitive?: boolean;
  /** 일반 문자열(string) 입력의 placeholder 오버라이드.
   *  미지정 시 기존 동작(default 값을 placeholder 로 표시)을 그대로 유지한다. */
  placeholder?: string;
  /** key_value_map 의 "키" 컬럼 헤더 오버라이드. 미지정 시 "키". */
  keyLabel?: string;
  /** key_value_map 의 "값" 컬럼 헤더 오버라이드. 미지정 시 "값". */
  valueLabel?: string;
  /** key_value_map 키 입력의 placeholder 오버라이드. 미지정 시 "키". */
  keyPlaceholder?: string;
  /** key_value_map 값 입력의 placeholder 오버라이드. 미지정 시 "값". */
  valuePlaceholder?: string;
  /** true 이면 key_value_map 의 값 입력 위에 `$.` JSONPath 빠른 삽입 칩을 표시한다.
   *  influxdb-write 등 값에 JSONPath 를 받는 노드에서 opt-in 으로 사용한다.
   *  미지정/false 면 칩을 표시하지 않아 다른 노드의 동작은 변하지 않는다. */
  pathHelper?: boolean;
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
