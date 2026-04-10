// 노드 타입별 설정 스키마 및 기본 포트 정의.
// 백엔드 Configure() 메서드의 config 키에 매핑된다.

import type { ConfigField, ConfigSchema } from '@/types/node';
import { getBridgeAdapterFields } from './bridgeAdapterSchemas';

export type PortDef = { name: string; direction: 'input' | 'output' | 'error' };

export interface NodeTypeSchema {
  /** 노드 설명 */
  description?: string;
  configSchema: ConfigSchema;
  defaultPorts: PortDef[];
  /** 입력 메시지에서 사용하는 필드 설명 */
  inputDesc?: string;
  /** 출력 메시지 형식 설명 */
  outputDesc?: string;
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
    description: '조건에 따라 메시지를 필터링합니다. 조건을 만족하는 메시지만 통과합니다.',
    inputDesc: '모든 메시지. 조건식에서 $.payload.* 경로로 필드 참조',
    outputDesc: '조건을 만족하는 메시지만 통과 (원본 그대로)',
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
    description: '메시지 데이터를 변환합니다. select/merge/exclude 파이프라인으로 payload와 metadata를 재구성합니다.',
    inputDesc: '모든 메시지. 파이프라인에서 $.payload.*, $.metadata.* 경로로 참조',
    outputDesc: '변환된 payload/metadata를 가진 메시지',
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
    description: '스크립트로 메시지를 처리합니다. 자유로운 로직 구현이 가능합니다.',
    inputDesc: '모든 메시지. 스크립트 내에서 msg.payload, msg.metadata 접근',
    outputDesc: '스크립트가 반환한 메시지',
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
    description: '키-값 매핑 테이블로 필드 값을 변환합니다. 코드→이름 변환 등에 사용합니다.',
    inputDesc: 'payload에서 소스 필드(JSONPath)의 값을 매핑 테이블에서 조회',
    outputDesc: '매핑 결과를 target 필드에 기록한 메시지 (원본 payload 유지)',
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

  framer: {
    description: '바이트 스트림에서 프로토콜 프레임을 분리하여 완성된 프레임을 출력합니다.',
    inputDesc: 'payload.raw ([]byte): 프레이밍할 바이트 스트림. metadata의 stream key로 다중 스트림 분리',
    outputDesc: 'payload: {raw: []byte, data: string, frame_index: number}. metadata: framer.stream_key',
    configSchema: {
      fields: [
        {
          name: 'framing',
          type: 'select',
          label: '프레이밍 모드',
          options: ['raw', 'newline', 'length_prefix', 'fixed_size', 'stream', 'frame'],
          required: true,
          description: '바이트 스트림 프레이밍 방식',
        },
        // --- newline 모드 전용 ---
        {
          name: 'delimiter',
          type: 'string',
          label: '구분자',
          default: '\n',
          description: '프레임 구분 문자',
          visibleWhen: { field: 'framing', value: 'newline' },
        },
        // --- length_prefix 모드 전용 ---
        {
          name: 'length_endian',
          type: 'select',
          label: '바이트 순서',
          options: ['big', 'little'],
          default: 'big',
          description: '길이 필드 바이트 순서',
          visibleWhen: { field: 'framing', value: ['length_prefix', 'frame'] },
        },
        {
          name: 'max_message_size',
          type: 'number',
          label: '최대 메시지 크기',
          default: 0,
          description: '최대 메시지 크기 (0=무제한)',
          visibleWhen: { field: 'framing', value: ['length_prefix', 'frame'] },
        },
        // --- fixed_size 모드 전용 ---
        {
          name: 'fixed_size',
          type: 'number',
          label: '고정 프레임 크기',
          required: true,
          description: '고정 프레임 크기 (바이트)',
          visibleWhen: { field: 'framing', value: 'fixed_size' },
        },
        // --- frame 모드 전용 (STX + 길이 필드 기반 프로토콜) ---
        {
          name: 'stx',
          type: 'string',
          label: 'STX (시작 바이트)',
          required: true,
          description: '시작 바이트 hex (예: 02, LGCP: 56)',
          visibleWhen: { field: 'framing', value: 'frame' },
        },
        {
          name: 'etx',
          type: 'string',
          label: 'ETX (종료 바이트)',
          required: false,
          description: '종료 바이트 hex (예: 03). 비워두면 ETX 검증을 건너뜀 (LGCP 등 ETX 없는 프로토콜)',
          visibleWhen: { field: 'framing', value: 'frame' },
        },
        {
          name: 'length_offset',
          type: 'number',
          label: '길이 필드 오프셋',
          default: 0,
          description: 'STX로부터 길이 필드까지의 바이트 오프셋',
          visibleWhen: { field: 'framing', value: 'frame' },
        },
        {
          name: 'length_size',
          type: 'number',
          label: '길이 필드 크기',
          default: 2,
          description: '길이 필드 바이트 수',
          visibleWhen: { field: 'framing', value: 'frame' },
        },
        {
          name: 'length_includes_header',
          type: 'boolean',
          label: '헤더 포함 길이',
          default: false,
          description: '길이 값에 헤더 바이트 포함 여부',
          visibleWhen: { field: 'framing', value: 'frame' },
        },
        {
          name: 'length_adjustment',
          type: 'number',
          label: '길이 보정값',
          default: 0,
          description: '길이 필드 보정값',
          visibleWhen: { field: 'framing', value: 'frame' },
        },
        {
          name: 'checksum',
          type: 'select',
          label: '체크섬',
          options: ['', 'sum8', 'xor'],
          default: '',
          description: '체크섬 알고리즘 (비워두면 사용 안 함)',
          visibleWhen: { field: 'framing', value: 'frame' },
        },
        // --- 공통 옵션 ---
        {
          name: 'buffer_size',
          type: 'number',
          label: '버퍼 크기',
          default: 4096,
          description: '내부 버퍼 크기 (바이트)',
        },
        {
          name: 'stream_key_metadata',
          type: 'string',
          label: '스트림 키 메타데이터',
          default: 'connection_id',
          description: '다중 스트림 분리에 사용할 메타데이터 키',
        },
        {
          name: 'max_streams',
          type: 'number',
          label: '최대 스트림 수',
          default: 0,
          description: '최대 동시 스트림 수 (0=무제한)',
        },
        {
          name: 'stream_idle_timeout',
          type: 'string',
          label: '스트림 유휴 타임아웃',
          description: '유휴 스트림 타임아웃 (예: 30s, 5m)',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
      { name: 'error', direction: 'error' },
    ],
  },

  aggregate: {
    description: '여러 메시지를 윈도우 단위로 집계합니다. sum, avg, min, max 등 집계 함수를 지원합니다.',
    inputDesc: 'payload에서 대상 필드(field)의 숫자 값. group_by 설정 시 해당 필드로 그룹 분리',
    outputDesc: '집계 결과: {result: 값, count: 수, window_type, aggregate_fn, field}',
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
    description: 'Samsung NASA 에어컨 상태를 주기적으로 조회합니다.',
    inputDesc: 'payload.device_id (선택): 특정 디바이스 조회. 미지정 시 전체 조회',
    outputDesc: 'payload: {devices: [{id, name, power, mode, temperature, fan_speed, ...}]}',
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
    description: 'Samsung NASA 에어컨을 제어합니다. 전원, 온도, 풍량, 모드 등을 설정합니다.',
    inputDesc: 'payload: {device_id, command, ...params} (예: {device_id:"01", command:"set_power", power:true})',
    outputDesc: 'payload: 에이전트 응답 (성공/실패 상태, 제어 결과)',
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
    description: 'Samsung NASA 에어컨 상태 조회 + 제어 통합 노드입니다.',
    inputDesc: 'payload.device_id (조회/제어 대상), payload.command + params (제어 시)',
    outputDesc: 'payload: 디바이스 상태 또는 제어 결과 JSON',
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
    description: 'LG LGAP 에어컨 상태를 주기적으로 조회합니다.',
    inputDesc: 'payload.device_id (선택): 특정 디바이스 조회. 미지정 시 전체 조회',
    outputDesc: 'payload: {devices: [{id, name, power, mode, set_temp, cur_temp, fan_speed, ...}]}',
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
    description: 'LG LGAP 에어컨을 제어합니다. 전원, 온도, 풍량, 모드 등을 설정합니다.',
    inputDesc: 'payload: {device_id, command, ...params} (예: {device_id:"01", command:"set_power", power:true})',
    outputDesc: 'payload: 에이전트 응답 (성공/실패 상태, 제어 결과)',
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
    description: 'LG LGAP 에어컨 상태 조회 + 제어 통합 노드입니다.',
    inputDesc: 'payload.device_id (조회/제어 대상), payload.command + params (제어 시)',
    outputDesc: 'payload: 디바이스 상태 또는 제어 결과 JSON',
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

  // --- IO: LGCP ---
  'lgcp-status': {
    description: 'LG LGCP 프로토콜로 실내기 상태를 조회합니다. RS-485 버스에서 캡처된 프레임을 해석합니다.',
    inputDesc: 'payload.address (선택): 특정 실내기 주소. 미지정 시 전체 조회',
    outputDesc: 'payload: {devices: [{address, power, mode, set_temp, cur_temp, fan_speed, ...}]} 또는 통계/최근 프레임',
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'LGCP 에이전트',
          required: true,
          options: ['lgcp'],
          description: '연결할 LGCP 에이전트를 선택합니다',
        },
        {
          name: 'default_address',
          type: 'string',
          label: '기본 주소',
          description: '기본 실내기 주소 (예: 01)',
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
        {
          name: 'poll_command',
          type: 'select',
          label: '폴링 명령',
          default: 'get_stats',
          options: ['get_stats', 'get_recent'],
          description: '폴링 시 실행할 명령 (get_stats: 통계, get_recent: 최근 데이터)',
        },
        {
          name: 'recent_count',
          type: 'number',
          label: '최근 데이터 수',
          default: 10,
          description: 'get_recent 명령 시 조회할 최근 데이터 수',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  'lgcp-control': {
    description: 'LG LGCP 프로토콜로 실내기를 제어합니다. 전원, 온도, 풍량, 모드를 설정합니다.',
    inputDesc: 'payload: {address, command, ...params} (예: {address:"67", command:"set_power", power:true})',
    outputDesc: 'payload: 에이전트 응답 (성공/실패 상태, 제어 결과)',
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'LGCP 에이전트',
          required: true,
          options: ['lgcp'],
          description: '연결할 LGCP 에이전트를 선택합니다',
        },
        {
          name: 'default_address',
          type: 'string',
          label: '기본 주소',
          description: '기본 실내기 주소 (예: 01)',
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

  lgcp: {
    description: 'LG LGCP 실내기 상태 조회 + 제어 통합 노드입니다.',
    inputDesc: 'payload.address (조회/제어 대상), payload.command + params (제어 시)',
    outputDesc: 'payload: 디바이스 상태 또는 제어 결과 JSON',
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'LGCP 에이전트',
          required: true,
          options: ['lgcp'],
          description: '연결할 LGCP 에이전트를 선택합니다',
        },
        {
          name: 'default_address',
          type: 'string',
          label: '기본 주소',
          description: '기본 실내기 주소 (예: 01)',
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
        {
          name: 'poll_command',
          type: 'select',
          label: '폴링 명령',
          default: 'get_stats',
          options: ['get_stats', 'get_recent'],
          description: '폴링 시 실행할 명령 (get_stats: 통계, get_recent: 최근 데이터)',
        },
        {
          name: 'recent_count',
          type: 'number',
          label: '최근 데이터 수',
          default: 10,
          description: 'get_recent 명령 시 조회할 최근 데이터 수',
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
    description: 'MODBUS 레지스터를 읽거나 씁니다. RTU/TCP 에이전트를 통해 통신합니다.',
    inputDesc: 'write 시: payload.values (쓸 값 배열). read 시: 입력 불필요 (설정값 사용)',
    outputDesc: 'read: payload.values (레지스터 값 배열). write: payload.success (성공 여부)',
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
    description: '조건에 따라 메시지를 다른 출력 포트로 라우팅합니다.',
    inputDesc: '모든 메시지. 라우팅 규칙에서 $.payload.* 경로로 필드 참조',
    outputDesc: '조건에 매칭된 포트로 메시지 전달 (원본 그대로). 미매칭 시 기본 포트',
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
    description: '에러 메시지를 캐치하여 에러 처리 플로우로 전달합니다.',
    inputDesc: '에러 메시지. metadata: _error (에러 내용), _errorNodeID (발생 노드)',
    outputDesc: '에러 메시지를 그대로 전달 (에러 처리 플로우로 라우팅)',
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
    description: '처리 실패한 메시지를 보관합니다. 저장, 로그 기록, 또는 폐기할 수 있습니다.',
    inputDesc: '처리 실패 메시지. metadata: _error, _errorNodeID',
    outputDesc: '없음 (종단 노드). 전략에 따라 저장/로그/폐기',
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


  // --- MQTT ---
  'mqtt-subscriber': {
    description: 'MQTT 토픽을 구독하여 메시지를 수신합니다. 소스 노드로 플로우의 시작점이 됩니다.',
    inputDesc: '없음 (소스 노드). 에이전트가 구독한 토픽에서 자동 수신',
    outputDesc: 'payload: 수신 데이터 (json: 파싱된 객체, raw: {raw: []byte}). metadata: mqtt.topic, mqtt.qos',
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
    description: 'MODBUS 레지스터를 주기적으로 폴링합니다. register_map에 정의된 레지스터를 일괄 읽기합니다.',
    inputDesc: '없음 (소스 노드). poll_interval 주기로 자동 폴링. 입력 메시지 수신 시 즉시 폴링 트리거',
    outputDesc: 'payload: register_map에 정의된 이름을 키로 한 값 맵 (예: {temperature: 25.5, humidity: 60})',
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
    description: 'MODBUS 레지스터에 값을 씁니다. Coils 또는 Holding Registers에 쓸 수 있습니다.',
    inputDesc: 'payload.value 또는 payload.values: 쓸 값 (단일 또는 배열)',
    outputDesc: 'payload: {success: bool, address, count, values} 쓰기 결과',
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
    description: 'MQTT 토픽으로 메시지를 발행합니다. 토픽, QoS, Retained를 설정할 수 있습니다.',
    inputDesc: 'payload: 발행할 데이터. metadata: mqtt.topic (토픽 오버라이드), mqtt.qos, mqtt.retained',
    outputDesc: '원본 메시지 패스스루',
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
    description: '메시지 데이터를 시계열 DB에 기록합니다. measurement, 태그, 필드를 매핑하여 저장합니다.',
    inputDesc: 'payload: 저장할 필드 데이터. measurement/tag_mappings/field_mappings로 매핑',
    outputDesc: '원본 메시지 패스스루',
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
    description: '시계열 DB에서 데이터를 조회합니다. 시간 범위, 집계 함수, 다운샘플링을 지원합니다.',
    inputDesc: '입력 메시지 트리거 (payload 내용 무관). 설정값으로 조회 실행',
    outputDesc: 'payload: {points: [{timestamp, fields: {...}}], count, measurement, time_range}',
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
  'store-write': {
    description: '메시지 데이터를 키-값 저장소에 기록합니다. 키 템플릿으로 동적 키를 생성합니다.',
    inputDesc: 'payload: key_template의 {field} 플레이스홀더 값 + value_key로 저장할 값',
    outputDesc: '원본 메시지 패스스루',
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'Store 에이전트',
          required: true,
          options: ['store'],
          description: '연결할 Store 에이전트를 선택합니다',
        },
        {
          name: 'key_template',
          type: 'string',
          label: '키 템플릿',
          required: true,
          description: '{field} 형식 플레이스홀더를 payload 값으로 치환 (예: {location}:{point}:{sensor_type})',
        },
        {
          name: 'value_key',
          type: 'string',
          label: '값 키',
          description: 'payload에서 저장할 값의 키 (비워두면 전체 payload 저장)',
        },
        {
          name: 'namespace',
          type: 'string',
          label: '네임스페이스',
          default: 'default',
          description: 'Store 네임스페이스',
        },
        {
          name: 'ttl',
          type: 'string',
          label: 'TTL',
          description: '만료 시간 (예: 5m, 1h, 24h). 비워두면 만료 없음',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  // --- IO: Output ---
  output: {
    description: '메시지를 포맷팅하여 출력합니다. 터미널, 파일, 에디터 등에 표시하며 메시지는 그대로 통과합니다.',
    inputDesc: '모든 메시지. property로 특정 경로 지정 가능 (.payload, .payload.name, .metadata, .id)',
    outputDesc: '원본 메시지 패스스루. 출력 대상(터미널/파일/에디터)에 포맷팅된 텍스트 출력',
    configSchema: {
      fields: [
        {
          name: 'level',
          type: 'select',
          label: '로그 레벨',
          options: ['debug', 'info', 'warn'],
          default: 'debug',
          description: '로거 출력 시 로그 레벨',
        },
        {
          name: 'output',
          type: 'select',
          label: '출력 대상',
          options: ['slog', 'logger', 'editor', 'terminal', 'file'],
          default: 'slog',
          description: '출력 대상 (slog: 서버 로그, logger: 에이전트 로거, editor: 에디터 패널, terminal: stdout, file: 파일)',
        },
        {
          name: 'property',
          type: 'string',
          label: '출력 필드',
          description: '메시지 경로 지정 (예: .payload, .payload.name, .metadata, .id). 미지정 시 메시지 전체 출력',
        },
        {
          name: 'format',
          type: 'select',
          label: '출력 형식',
          options: ['json', 'plain', 'raw'],
          default: 'json',
          description: 'json: JSON 포맷 / plain: 읽기 쉬운 텍스트 (숫자, 문자열, 바이너리→hex) / raw: 바이너리 그대로',
        },
        {
          name: 'display_fields',
          type: 'string',
          label: '표시 항목',
          description: '출력에 포함할 항목 (쉼표로 구분, 예: time,level,name,message). 미지정 시 전체',
        },
        {
          name: 'prefix',
          type: 'string',
          label: '접두어',
          description: '출력 접두어. 미지정 시 노드 이름 사용',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ],
  },

  // --- Serial I/O ---
  'serial-in': {
    description: '시리얼 포트에서 데이터를 수신합니다. 에이전트의 프레이밍 설정에 따라 프레임 단위로 전달합니다.',
    inputDesc: '없음 (소스 노드). 시리얼 에이전트가 프레이밍된 데이터를 자동 수신',
    outputDesc: 'out: payload {raw: []byte, data: string}. raw_out: 프레이밍 이전 원시 바이트 {raw: []byte}',
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: '시리얼 에이전트',
          required: true,
          options: ['serial'],
          description: '연결할 시리얼 에이전트를 선택합니다',
        },
      ],
    },
    defaultPorts: [
      { name: 'out', direction: 'output' as const },
      { name: 'raw_out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  'serial-out': {
    description: '시리얼 포트로 데이터를 전송합니다. payload의 raw 또는 data 필드를 바이트로 전송합니다.',
    inputDesc: 'payload.raw ([]byte, 우선) 또는 payload.data (string). 없으면 payload 전체 JSON 전송',
    outputDesc: '원본 메시지 clone 패스스루. metadata: serial.node_id 추가',
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: '시리얼 에이전트',
          required: true,
          options: ['serial'],
          description: '연결할 시리얼 에이전트를 선택합니다',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  // --- TCP I/O ---
  'tcp-in': {
    description: 'TCP 에이전트로부터 메시지를 수신합니다. 서버 모드에서는 클라이언트 연결 정보를 포함합니다.',
    inputDesc: '없음 (소스 노드). TCP 에이전트가 수신한 데이터를 자동 전달',
    outputDesc: 'payload: {raw: []byte, data: string}. metadata: tcp.remote_addr (서버 모드), tcp.agent_type',
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'TCP 에이전트',
          required: true,
          options: ['tcp-server', 'tcp-client'],
          description: '연결할 TCP 에이전트를 선택합니다',
        },
      ],
    },
    defaultPorts: [
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  'tcp-out': {
    description: 'TCP 에이전트를 통해 데이터를 전송합니다. 특정 클라이언트 또는 브로드캐스트로 전송합니다.',
    inputDesc: 'payload.raw ([]byte, 우선) 또는 payload.data (string). metadata.tcp.remote_addr: 대상 클라이언트 (없으면 브로드캐스트)',
    outputDesc: '원본 메시지 clone 패스스루. metadata: tcp.node_id 추가',
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'TCP 에이전트',
          required: true,
          options: ['tcp-server', 'tcp-client'],
          description: '연결할 TCP 에이전트를 선택합니다',
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  // --- Storage ---
  'store-read': {
    description: '키-값 저장소에서 데이터를 조회합니다. 조회된 값을 payload에 추가합니다.',
    inputDesc: 'payload: key_template의 {field} 플레이스홀더 값 (조회 키 생성용)',
    outputDesc: 'payload에 output_key(기본: store_value) 필드 추가. 원본 payload 유지',
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'Store 에이전트',
          required: true,
          options: ['store'],
          description: '연결할 Store 에이전트를 선택합니다',
        },
        {
          name: 'key_template',
          type: 'string',
          label: '키 템플릿',
          required: true,
          description: '{field} 형식 플레이스홀더를 payload 값으로 치환',
        },
        {
          name: 'namespace',
          type: 'string',
          label: '네임스페이스',
          default: 'default',
          description: 'Store 네임스페이스',
        },
        {
          name: 'output_key',
          type: 'string',
          label: '출력 키',
          default: 'store_value',
          description: '조회된 값을 저장할 payload 키',
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
/**
 * 노드 타입의 설명을 반환한다.
 */
export function getNodeDescription(nodeType: string): string | undefined {
  if (nodeType === 'bridge') {
    return '외부 에이전트와 메시지를 송수신하는 브릿지 노드입니다.';
  }
  return NODE_SCHEMAS[nodeType]?.description;
}

export function getConfigSchema(nodeType: string, agentType?: string): ConfigSchema | undefined {
  if (nodeType === 'bridge') {
    return { fields: getBridgeConfigFields(agentType) };
  }
  return NODE_SCHEMAS[nodeType]?.configSchema;
}

/**
 * 노드 타입의 입출력 메시지 설명을 반환한다.
 * bridge 타입은 direction에 따라 동적 설명을 반환한다.
 */
export function getNodeIODesc(nodeType: string, direction?: string): { inputDesc?: string; outputDesc?: string } {
  if (nodeType === 'bridge') {
    switch (direction) {
      case 'in':
        return {
          inputDesc: '없음 (소스). 에이전트가 수신한 데이터를 자동 전달',
          outputDesc: 'payload: {raw: []byte} (기본). payload_format에 따라 JSON 파싱 가능',
        };
      case 'out':
        return {
          inputDesc: '모든 메시지. payload를 JSON 직렬화하여 에이전트에 전송',
          outputDesc: '없음 (종단). 에이전트로 전송만 수행',
        };
      case 'inout':
        return {
          inputDesc: '에이전트로 전송할 메시지 + 에이전트에서 수신',
          outputDesc: '에이전트에서 수신한 메시지',
        };
      case 'request_reply':
        return {
          inputDesc: '요청 메시지. 에이전트에 전송 후 응답 대기',
          outputDesc: '에이전트의 응답 메시지',
        };
      default:
        return {};
    }
  }
  const schema = NODE_SCHEMAS[nodeType];
  return { inputDesc: schema?.inputDesc, outputDesc: schema?.outputDesc };
}
