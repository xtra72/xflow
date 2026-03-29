// 노드 타입별 설정 스키마 및 기본 포트 정의.
// 백엔드 Configure() 메서드의 config 키에 매핑된다.

import type { ConfigField, ConfigSchema } from '@/types/node';
import { getBridgeAdapterFields } from './bridgeAdapterSchemas';

export type PortDef = { name: string; direction: 'input' | 'output' | 'error' };

interface NodeTypeSchema {
  configSchema: ConfigSchema;
  defaultPorts: PortDef[];
}

/** 에이전트 타입별 브릿지 기본 설정 */
const BRIDGE_AGENT_DEFAULTS: Record<string, { direction: string; showTopics: boolean; showPayloadFormat: boolean; showPublishTopic: boolean }> = {
  'mqtt': { direction: 'inout', showTopics: true, showPayloadFormat: true, showPublishTopic: true },
  'modbus-tcp': { direction: 'in', showTopics: false, showPayloadFormat: false, showPublishTopic: false },
  'modbus-rtu': { direction: 'in', showTopics: false, showPayloadFormat: false, showPublishTopic: false },
  'modbus-tcp-server': { direction: 'in', showTopics: false, showPayloadFormat: false, showPublishTopic: false },
  'http': { direction: 'in', showTopics: false, showPayloadFormat: true, showPublishTopic: false },
  'logger': { direction: 'out', showTopics: false, showPayloadFormat: true, showPublishTopic: true },
  'error-logger': { direction: 'out', showTopics: false, showPayloadFormat: false, showPublishTopic: false },
  'influxdb': { direction: 'out', showTopics: false, showPayloadFormat: true, showPublishTopic: false },
  'custom': { direction: 'inout', showTopics: false, showPayloadFormat: true, showPublishTopic: true },
};

/** 에이전트 타입에 따른 브릿지 설정 스키마를 동적 생성한다 */
function getBridgeConfigFields(agentType?: string): ConfigField[] {
  const defaults = agentType ? BRIDGE_AGENT_DEFAULTS[agentType] : undefined;

  const fields: ConfigField[] = [
    {
      name: 'agent_id',
      type: 'agent_select',
      label: '에이전트',
      required: true,
      description: '연결할 에이전트를 선택합니다',
    },
    {
      name: 'direction',
      type: 'select',
      label: '방향',
      options: ['in', 'out', 'inout', 'request_reply'],
      default: defaults?.direction ?? 'inout',
      description: '데이터 흐름 방향',
    },
  ];

  // payload_format은 데이터 변환이 필요한 에이전트에서만 표시
  if (defaults?.showPayloadFormat ?? true) {
    fields.push({
      name: 'payload_format',
      type: 'select',
      label: '페이로드 형식',
      options: ['json', 'raw', 'text'],
      default: 'json',
      description: '수신 데이터 변환 방식',
    });
  }

  // publish_topic은 파일 저장 또는 발행 토픽이 필요한 에이전트에서 표시
  if (defaults?.showPublishTopic ?? false) {
    fields.push({
      name: 'publish_topic',
      type: 'string',
      label: '발행 토픽 / 파일 경로',
      description: '발행 토픽 또는 파일 저장 경로 (예: ./data/capture.jsonl)',
    });
  }

  // topics는 구독 기능이 있는 에이전트(mqtt 등)에서만 표시
  if (defaults?.showTopics ?? true) {
    fields.push({
      name: 'topics',
      type: 'string',
      label: '토픽',
      description: '구독 토픽 (쉼표로 구분)',
    });
  }

  // 에이전트 타입별 어댑터 전용 설정 필드 추가
  const adapterFields = getBridgeAdapterFields(agentType);
  if (adapterFields.length > 0) {
    fields.push(...adapterFields);
  }

  return fields;
}

const BRIDGE_DEFAULT_PORTS: PortDef[] = [
  { name: 'in', direction: 'input' },
  { name: 'out', direction: 'output' },
];

/** 노드 타입별 설정 스키마 레지스트리 */
const NODE_SCHEMAS: Record<string, NodeTypeSchema> = {

  // --- Processing ---
  filter: {
    configSchema: {
      fields: [
        {
          name: 'condition',
          type: 'string',
          label: '조건식',
          required: true,
          description: '메시지 필터링 조건 (예: $.payload.temperature > 30)',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ],
  },

  transform: {
    configSchema: {
      fields: [
        {
          name: 'expression',
          type: 'transform_pipeline',
          label: '변환 파이프라인',
          required: true,
          description: '단계별 데이터 변환 (select/merge/exclude)',
        },
        {
          name: 'metadata_expression',
          type: 'transform_pipeline',
          label: '메타데이터 파이프라인',
          required: false,
          description: '메시지 메타데이터 구성 (mqtt.topic, mqtt.qos 등)',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ],
  },

  script: {
    configSchema: {
      fields: [
        {
          name: 'script',
          type: 'string',
          label: '스크립트',
          required: true,
          description: '실행할 스크립트 코드',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ],
  },

  mapping: {
    configSchema: {
      fields: [
        {
          name: 'field',
          type: 'string',
          label: '소스 필드',
          required: true,
          description: '매핑할 소스 필드 JSONPath (예: $.payload.status_code)',
        },
        {
          name: 'mappings',
          type: 'key_value_map',
          label: '매핑 테이블',
          required: true,
          description: '키-값 매핑 테이블 (JSON 객체)',
        },
        {
          name: 'default',
          type: 'string',
          label: '기본값',
          description: '매핑 키가 없을 때 사용할 기본값',
        },
        {
          name: 'target',
          type: 'string',
          label: '출력 필드',
          description: '결과를 기록할 필드명 (미지정 시 소스 필드 덮어쓰기)',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ],
  },

  aggregate: {
    configSchema: {
      fields: [
        {
          name: 'window_type',
          type: 'select',
          label: '윈도우 타입',
          options: ['count', 'time', 'sliding'],
          default: 'count',
          required: true,
          description: 'count: 메시지 수, time: 시간 기반, sliding: 슬라이딩 윈도우',
        },
        {
          name: 'window_size',
          type: 'string',
          label: '윈도우 크기',
          required: true,
          description: 'count: 숫자, time/sliding: 기간 (예: 5s, 1m)',
        },
        {
          name: 'aggregate_fn',
          type: 'select',
          label: '집계 함수',
          options: ['sum', 'avg', 'min', 'max', 'count', 'first', 'last', 'collect'],
          default: 'sum',
          description: '적용할 집계 함수',
        },
        {
          name: 'field',
          type: 'string',
          label: '대상 필드',
          default: 'value',
          description: '집계 대상 필드명',
        },
        {
          name: 'group_by',
          type: 'string',
          label: '그룹 기준',
          description: '그룹별 집계를 위한 필드명',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ],
  },

  // --- IO: Samsung NASA ---
  'nasa-status': {
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'NASA 에이전트',
          required: true,
          options: ['samsung-nasa'],
          description: '연결할 Samsung NASA 에이전트를 선택합니다',
        },
        {
          name: 'device_id',
          type: 'string',
          label: '디바이스 ID',
          description: '조회할 디바이스 ID (미지정 시 전체 조회)',
        },
        {
          name: 'poll_interval',
          type: 'string',
          label: '폴링 주기',
          default: '30s',
          description: '자동 상태 폴링 주기 (예: 10s, 1m)',
        },
        {
          name: 'timeout',
          type: 'string',
          label: '타임아웃',
          default: '5s',
          description: 'Agent Process 호출 타임아웃',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  'nasa-control': {
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'NASA 에이전트',
          required: true,
          options: ['samsung-nasa'],
          description: '연결할 Samsung NASA 에이전트를 선택합니다',
        },
        {
          name: 'device_id',
          type: 'string',
          label: '디바이스 ID',
          description: '기본 대상 디바이스 ID (메시지에서 오버라이드 가능)',
        },
        {
          name: 'timeout',
          type: 'string',
          label: '타임아웃',
          default: '5s',
          description: 'Agent Process 호출 타임아웃',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  nasa: {
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'NASA 에이전트',
          required: true,
          options: ['samsung-nasa'],
          description: '연결할 Samsung NASA 에이전트를 선택합니다',
        },
        {
          name: 'device_id',
          type: 'string',
          label: '디바이스 ID',
          description: '기본 대상 디바이스 ID (메시지에서 오버라이드 가능)',
        },
        {
          name: 'poll_interval',
          type: 'string',
          label: '폴링 주기',
          default: '30s',
          description: '자동 상태 폴링 주기 (예: 15s, 1m)',
        },
        {
          name: 'timeout',
          type: 'string',
          label: '타임아웃',
          default: '5s',
          description: 'Agent Process 호출 타임아웃',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  // --- IO: LG LGAP ---
  'lgap-status': {
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'LGAP 에이전트',
          required: true,
          options: ['lgap'],
          description: '연결할 LG LGAP 에이전트를 선택합니다',
        },
        {
          name: 'device_id',
          type: 'string',
          label: '디바이스 ID',
          description: '조회할 디바이스 ID (미지정 시 전체 조회)',
        },
        {
          name: 'poll_interval',
          type: 'string',
          label: '폴링 주기',
          default: '30s',
          description: '자동 상태 폴링 주기 (예: 10s, 1m)',
        },
        {
          name: 'timeout',
          type: 'string',
          label: '타임아웃',
          default: '5s',
          description: 'Agent Process 호출 타임아웃',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  'lgap-control': {
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'LGAP 에이전트',
          required: true,
          options: ['lgap'],
          description: '연결할 LG LGAP 에이전트를 선택합니다',
        },
        {
          name: 'device_id',
          type: 'string',
          label: '디바이스 ID',
          description: '기본 대상 디바이스 ID (메시지에서 오버라이드 가능)',
        },
        {
          name: 'timeout',
          type: 'string',
          label: '타임아웃',
          default: '5s',
          description: 'Agent Process 호출 타임아웃',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  lgap: {
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'LGAP 에이전트',
          required: true,
          options: ['lgap'],
          description: '연결할 LG LGAP 에이전트를 선택합니다',
        },
        {
          name: 'device_id',
          type: 'string',
          label: '디바이스 ID',
          description: '기본 대상 디바이스 ID (메시지에서 오버라이드 가능)',
        },
        {
          name: 'poll_interval',
          type: 'string',
          label: '폴링 주기',
          default: '30s',
          description: '자동 상태 폴링 주기 (예: 15s, 1m)',
        },
        {
          name: 'timeout',
          type: 'string',
          label: '타임아웃',
          default: '5s',
          description: 'Agent Process 호출 타임아웃',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  // --- IO: MODBUS ---
  modbus: {
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'MODBUS 에이전트',
          required: true,
          options: ['modbus-rtu', 'modbus-tcp'],
          description: '연결할 MODBUS 에이전트를 선택합니다',
        },
        {
          name: 'operation',
          type: 'select',
          label: '연산',
          options: ['read', 'write'],
          default: 'read',
          required: true,
          description: '읽기 또는 쓰기 연산을 선택합니다',
        },
        {
          name: 'register_area',
          type: 'select',
          label: '레지스터 영역',
          options: ['coils', 'discrete_inputs', 'holding_registers', 'input_registers'],
          default: 'holding_registers',
          required: true,
          description: 'MODBUS 레지스터 영역을 선택합니다',
        },
        {
          name: 'address',
          type: 'number',
          label: '시작 주소',
          required: true,
          default: 0,
          description: '시작 레지스터 주소 (0-65535)',
        },
        {
          name: 'count',
          type: 'number',
          label: '레지스터 수',
          default: 1,
          description: '읽기/쓰기할 레지스터 수',
        },
        {
          name: 'data_type',
          type: 'select',
          label: '데이터 타입',
          options: ['uint16', 'int16', 'float32', 'uint32', 'int32'],
          default: 'uint16',
          description: '레지스터 데이터 타입 (Holding/Input Registers 전용)',
        },
        {
          name: 'byte_order',
          type: 'select',
          label: '바이트 순서',
          options: ['big_endian', 'little_endian'],
          default: 'big_endian',
          description: '다중 레지스터 타입의 바이트 순서',
        },
        {
          name: 'device_id',
          type: 'number',
          label: '디바이스 ID',
          default: 1,
          description: 'MODBUS Client 에이전트 전용 대상 디바이스 ID',
        },
      ],
    },
    defaultPorts: [
      { name: 'input', direction: 'input' as const },
      { name: 'output', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  // --- Routing ---
  switch: {
    configSchema: {
      fields: [
        {
          name: 'routes',
          type: 'object',
          label: '라우팅 규칙',
          description: '조건식과 출력 포트를 매핑하는 규칙 배열 (JSON)',
        },
        {
          name: 'default_port',
          type: 'string',
          label: '기본 포트',
          default: 'out',
          description: '일치하는 조건이 없을 때 사용할 출력 포트',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ],
  },

  // --- Error ---
  catch: {
    configSchema: {
      fields: [
        {
          name: 'catch_types',
          type: 'string',
          label: '에러 타입',
          description: '처리할 에러 타입 (쉼표로 구분, 비워두면 모든 에러)',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ],
  },

  deadletter: {
    configSchema: {
      fields: [
        {
          name: 'strategy',
          type: 'select',
          label: '처리 전략',
          options: ['store', 'log', 'discard'],
          default: 'store',
          description: 'store: 저장, log: 로그 기록, discard: 폐기',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' },
    ],
  },

  // --- Debug ---
  debug: {
    configSchema: {
      fields: [
        {
          name: 'level',
          type: 'select',
          label: '로그 레벨',
          options: ['debug', 'info', 'warn', 'error'],
          default: 'debug',
          description: '디버그 출력 로그 레벨',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ],
  },

  output: {
    configSchema: {
      fields: [
        {
          name: 'prefix',
          type: 'string',
          label: '접두어',
          default: '[output]',
          description: '로그 출력 시 접두어',
        },
        {
          name: 'template',
          type: 'string',
          label: '메시지 템플릿',
          description: 'Go text/template 형식 (예: 온도={{.temperature}}). 미지정 시 전체 페이로드 JSON 출력',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ],
  },

  status: {
    configSchema: {
      fields: [
        {
          name: 'watch_nodes',
          type: 'string',
          label: '감시 노드',
          description: '상태를 감시할 노드 ID 목록 (쉼표로 구분)',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ],
  },

  // --- MQTT ---
  'mqtt-subscriber': {
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'MQTT 에이전트',
          required: true,
          options: ['mqtt-client'],
          description: '연결할 MQTT 에이전트',
        },
        {
          name: 'topics',
          type: 'string_list',
          label: '구독 토픽',
          required: true,
          description: 'sensor/temp, device/# 등 MQTT 토픽',
        },
        {
          name: 'payload_format',
          type: 'select',
          label: '페이로드 형식',
          options: ['json', 'raw'],
          default: 'json',
          description: '수신 메시지 페이로드 형식',
        },
        {
          name: 'buffer_size',
          type: 'number',
          label: '버퍼 크기',
          default: 64,
          description: '수신 메시지 버퍼 크기',
        },
      ],
    },
    defaultPorts: [
      { name: 'out', direction: 'output' },
    ],
  },

  // --- IO: MODBUS Poller ---
  'modbus-poller': {
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'MODBUS 에이전트',
          required: true,
          options: ['modbus-rtu', 'modbus-tcp', 'modbus-tcp-server'],
          description: '연결할 MODBUS 에이전트를 선택합니다',
        },
        {
          name: 'device_id',
          type: 'number',
          label: '디바이스 ID',
          default: 1,
          description: '기본 디바이스 ID (register_map 항목에서 개별 지정 가능)',
        },
        {
          name: 'poll_interval',
          type: 'string',
          label: '폴링 주기',
          default: '5s',
          description: '레지스터 폴링 주기 (예: 1s, 5s, 1m)',
        },
        {
          name: 'register_map',
          type: 'register_map',
          label: '레지스터 맵',
          required: true,
          description: '폴링할 레지스터 정의 (이름, 영역, 주소, 수, 타입, 디바이스ID)',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  // --- IO: MODBUS Writer ---
  'modbus-writer': {
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'MODBUS 에이전트',
          required: true,
          options: ['modbus-rtu', 'modbus-tcp', 'modbus-tcp-server'],
          description: '연결할 MODBUS 에이전트를 선택합니다',
        },
        {
          name: 'register_area',
          type: 'select',
          label: '레지스터 영역',
          options: ['coils', 'holding_registers'],
          default: 'holding_registers',
          description: '쓰기 가능 레지스터 영역 (coils, holding_registers)',
        },
        {
          name: 'address',
          type: 'number',
          label: '시작 주소',
          default: 0,
          description: '시작 레지스터 주소 (0-65535)',
        },
        {
          name: 'data_type',
          type: 'select',
          label: '데이터 타입',
          options: ['uint16', 'int16', 'float32', 'uint32', 'int32'],
          default: 'uint16',
          description: '레지스터 데이터 타입',
        },
        {
          name: 'byte_order',
          type: 'select',
          label: '바이트 순서',
          options: ['big_endian', 'little_endian'],
          default: 'big_endian',
          description: '다중 레지스터의 바이트 순서',
        },
        {
          name: 'device_id',
          type: 'number',
          label: '디바이스 ID',
          default: 1,
          description: 'MODBUS Client 에이전트 전용 디바이스 ID',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  'mqtt-publisher': {
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'MQTT 에이전트',
          required: true,
          options: ['mqtt-client'],
          description: '연결할 MQTT 에이전트',
        },
        {
          name: 'default_topic',
          type: 'string',
          label: '기본 토픽',
          description: '기본 발행 토픽. 메시지의 _mqtt.topic 또는 metadata mqtt.topic으로 오버라이드 가능',
        },
        {
          name: 'default_qos',
          type: 'select',
          label: '기본 QoS',
          options: ['0', '1', '2'],
          default: '0',
          description: 'MQTT QoS 레벨 (0: At most once, 1: At least once, 2: Exactly once)',
        },
        {
          name: 'default_retained',
          type: 'boolean',
          label: 'Retained',
          default: false,
          description: '기본 Retained 플래그',
        },
        {
          name: 'payload_format',
          type: 'select',
          label: '페이로드 형식',
          options: ['json', 'raw'],
          default: 'json',
          description: '발행 메시지 페이로드 형식',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ],
  },

  // --- IO: TSDB ---
  'tsdb-write': {
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'TSDB 에이전트',
          required: true,
          options: ['tsdb'],
          description: '연결할 TSDB 에이전트를 선택합니다',
        },
        {
          name: 'measurement',
          type: 'string',
          label: 'Measurement',
          description: '고정 measurement 이름 (비워두면 measurement_key에서 추출)',
        },
        {
          name: 'measurement_key',
          type: 'string',
          label: 'Measurement 키',
          description: 'payload에서 measurement를 추출할 키 (measurement가 비어있을 때 사용)',
        },
        {
          name: 'tag_mappings',
          type: 'key_value_map',
          label: '태그 매핑',
          description: '태그 이름 → payload 키 매핑 (시리즈 키 구성에 사용)',
        },
        {
          name: 'field_mappings',
          type: 'key_value_map',
          label: '필드 매핑',
          description: '필드 이름 → payload 키 매핑 (비워두면 전체 payload를 필드로 사용)',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  'tsdb-query': {
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'TSDB 에이전트',
          required: true,
          options: ['tsdb'],
          description: '연결할 TSDB 에이전트를 선택합니다',
        },
        {
          name: 'measurement',
          type: 'string',
          label: 'Measurement',
          description: '조회 대상 measurement 이름',
        },
        {
          name: 'series_key',
          type: 'string',
          label: '시리즈 키',
          description: '직접 시리즈 키 지정 (measurement + tags 대신)',
        },
        {
          name: 'tags',
          type: 'key_value_map',
          label: '태그 필터',
          description: '시리즈 필터링용 태그 조건',
        },
        {
          name: 'time_range',
          type: 'string',
          label: '시간 범위',
          default: '1h',
          description: '현재 시각 기준 과거 시간 범위 (예: 30m, 1h, 24h)',
        },
        {
          name: 'aggregation',
          type: 'select',
          label: '집계 함수',
          options: ['', 'min', 'max', 'avg', 'sum', 'count', 'first', 'last'],
          default: '',
          description: '집계 함수 (비워두면 raw 데이터)',
        },
        {
          name: 'field',
          type: 'string',
          label: '집계 대상 필드',
          description: '집계할 필드 이름',
        },
        {
          name: 'bucket',
          type: 'string',
          label: '다운샘플링 간격',
          description: '버킷 간격 (예: 5m, 15m, 1h)',
        },
        {
          name: 'limit',
          type: 'number',
          label: '최대 포인트 수',
          description: '반환할 최대 포인트 수',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },
};

/**
 * 노드 타입에 해당하는 설정 스키마를 반환한다.
 * bridge 타입은 연결된 에이전트 타입에 따라 동적 스키마를 반환한다.
 */
export function getNodeSchema(nodeType: string, agentType?: string): NodeTypeSchema | undefined {
  if (nodeType === 'bridge') {
    return {
      configSchema: { fields: getBridgeConfigFields(agentType) },
      defaultPorts: BRIDGE_DEFAULT_PORTS,
    };
  }
  return NODE_SCHEMAS[nodeType];
}

/**
 * 노드 타입에 해당하는 기본 포트 목록을 반환한다.
 * 등록되지 않은 타입이면 기본 in/out 포트를 반환한다.
 */
export function getDefaultPorts(nodeType: string): PortDef[] {
  if (nodeType === 'bridge') return BRIDGE_DEFAULT_PORTS;
  return NODE_SCHEMAS[nodeType]?.defaultPorts ?? [
    { name: 'in', direction: 'input' },
    { name: 'out', direction: 'output' },
  ];
}

/**
 * 노드 타입과 설정에 따라 포트를 동적으로 계산한다.
 * bridge: direction에 따라 포트 결정, switch: routes에 따라 동적 출력 포트.
 * 기타 노드 타입은 getDefaultPorts로 위임한다.
 */
export function computePortsForNode(nodeType: string, config?: Record<string, unknown>): PortDef[] {
  if (nodeType === 'bridge') {
    const direction = config?.direction as string | undefined;
    switch (direction) {
      case 'in':
        return [{ name: 'out', direction: 'output' }];
      case 'out':
        return [{ name: 'in', direction: 'input' }];
      case 'inout':
      case 'request_reply':
        return [
          { name: 'in', direction: 'input' },
          { name: 'out', direction: 'output' },
        ];
      default:
        return [
          { name: 'in', direction: 'input' },
          { name: 'out', direction: 'output' },
        ];
    }
  }

  if (nodeType === 'switch') {
    const routes = config?.routes as Array<{ name: string }> | undefined;
    const ports: PortDef[] = [{ name: 'in', direction: 'input' }];
    if (routes && routes.length > 0) {
      for (const r of routes) {
        ports.push({ name: r.name, direction: 'output' });
      }
      ports.push({ name: 'default', direction: 'output' });
    } else {
      ports.push({ name: 'out', direction: 'output' });
    }
    return ports;
  }

  return getDefaultPorts(nodeType);
}

/**
 * 노드 타입에 해당하는 ConfigSchema를 반환한다.
 * bridge 타입은 연결된 에이전트 타입에 따라 동적 필드를 반환한다.
 */
export function getConfigSchema(nodeType: string, agentType?: string): ConfigSchema | undefined {
  if (nodeType === 'bridge') {
    return { fields: getBridgeConfigFields(agentType) };
  }
  return NODE_SCHEMAS[nodeType]?.configSchema;
}
