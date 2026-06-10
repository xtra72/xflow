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
  deduplicate: {
    description: '시간 창(window) 내에서 동일한 메시지를 제거합니다. key 필드로 그룹핑하고, 비교 필드가 모두 동일하면 중복으로 판정하여 폐기합니다.',
    inputDesc: '모든 메시지',
    outputDesc: '중복이 아닌 메시지만 통과 (값 변경 또는 window 초과 시)',
    configSchema: {
      fields: [
        { name: 'key', type: 'string', label: '그룹핑 키', description: '메시지를 그룹핑할 키. bare name(예: idu_num) = 최상위 payload 필드(레거시). $.-경로(예: $.payload.state.mode, $.metadata.device.id)는 메시지 전체 대상. 비어있거나 경로 해석 실패 시 전체 메시지 기준' },
        { name: 'window', type: 'string', label: '억제 시간', default: '30s', description: '중복 억제 시간 창 (예: 30s, 1m). 초과 시 동일 값도 강제 통과' },
        { name: 'compare_fields', type: 'compare_fields', label: '비교 필드', description: '비교 대상 필드 목록. 빈 목록이면 전체 페이로드 비교. 각 필드는 bare name(예: current_temperature) = 최상위 payload 키(레거시), 또는 $.-경로(예: $.payload.state.mode, $.metadata.device.id) = 메시지 전체 대상. 허용오차(0 이상)를 지정하면 |현재-이전| ≤ 오차 일 때만 동일로 판정.' },
        { name: 'missing_field_as_different', type: 'boolean', label: '필드 부재 시 다름으로 처리', default: false, description: '활성화 시 신규 메시지에 비교 필드 중 하나라도 부재하면 즉시 통과 (중복 판정 안 함). 비활성 시 부재 필드는 nil 로 비교됨 (v0.18.4).' },
        { name: 'on_duplicate', type: 'select', label: '중복 시 처리', options: ['drop', 'reject_port'], default: 'drop', description: 'drop: 폐기, reject_port: reject 포트로 전달' },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ],
  },

  filter: {
    description: '조건에 따라 메시지를 필터링합니다. 조건을 만족하는 메시지만 out 포트로 통과하고, 불일치 메시지는 reject 포트로 전달됩니다.',
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
        {
          name: 'on_reject',
          type: 'select',
          label: '거부 시 처리',
          options: ['reject_port', 'error_port'],
          default: 'reject_port',
          description: 'reject_port: reject 포트로 전달 (연결 없으면 폐기), error_port: 엔진 에러 포트로 전달',
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
          type: 'multiline',
          label: '스크립트',
          required: true,
          description: '실행할 Lua 스크립트 코드. 입력: 전역 `msg` (id, timestamp, payload, metadata). 반환: 변환된 msg 테이블. 예: `msg.payload.x = msg.payload.x * 2; return msg`',
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

  'select-field': {
    description:
      '메시지에서 지정한 경로의 필드만 남깁니다. payload / metadata / type 을 `$.` 경로 화이트리스트로 통합 지정하고, 누락 필드는 유지/드랍/채움 처리합니다.',
    inputDesc: '모든 메시지. `$.` 경로 화이트리스트로 payload·metadata·type 을 통합 필터링',
    outputDesc:
      '지정한 경로만 남긴 메시지(out). on_missing=drop + drop 포트 전송 옵션이 켜지면 드랍된 메시지는 drop 포트로.',
    configSchema: {
      fields: [
        {
          name: 'fields',
          type: 'key_value_map',
          label: '선택 필드(경로)',
          description:
            '남길 필드의 `$.` 경로 화이트리스트입니다(store/mqtt 노드와 동일한 경로 문법).\n' +
            '• `$.payload.<dotpath>` — 임의 깊이의 payload 필드 ($.payload.temperature, 중첩 $.payload.state.mode, 서브트리 전체 $.payload.state)\n' +
            '• `$.metadata.<key>` — 최상위 metadata 문자열 또는 그룹 전체 ($.metadata.node_id, 그룹 전체 $.metadata.device)\n' +
            '• `$.metadata.<group>.<field>` — 그룹 내 한 필드 ($.metadata.device.id)\n' +
            '• `$.type` — 메시지 타입 유지(미지정 시 type 은 제거됨)\n' +
            '• `$.id`, `$.timestamp` — 항상 보존(나열해도 무동작)\n' +
            '화이트리스트 의미: fields 가 비어있지 않으면 나열되지 않은 모든 것(나열 안 된 payload 키, metadata 키/그룹, 그리고 $.type 미지정 시 type)이 제거됩니다. fields 가 비어있으면 그대로 통과(pass-through)합니다.\n' +
            '값은 on_missing=fill 모드일 때만 채울 기본값으로 사용됩니다.',
          keyLabel: '경로 ($.payload.x / $.metadata.device.id / $.type)',
          valueLabel: '채울 값 (fill 모드)',
          keyPlaceholder: '예: $.payload.temperature',
          valuePlaceholder: '예: 0',
        },
        {
          name: 'on_missing',
          type: 'select',
          label: '누락 필드 처리',
          options: ['keep', 'drop', 'fill'],
          default: 'keep',
          description: 'keep: 필드 생략 / drop: 메시지 전체 드랍 / fill: 지정한 기본값으로 채움',
        },
        {
          name: 'drop_to_port',
          type: 'boolean',
          label: '드랍 메시지를 drop 포트로 전송',
          default: false,
          description:
            'on_missing=drop 으로 메시지가 드랍될 때, 버리지 않고 drop 출력 포트로 전송합니다. 끄면 메시지를 폐기합니다.',
          visibleWhen: { field: 'on_missing', value: 'drop' },
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
      { name: 'drop', direction: 'output' },
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
          description: '시작 바이트 hex (예: 02, LG ICP-02: 56)',
          visibleWhen: { field: 'framing', value: 'frame' },
        },
        {
          name: 'etx',
          type: 'string',
          label: 'ETX (종료 바이트)',
          required: false,
          description: '종료 바이트 hex (예: 03). 비워두면 ETX 검증을 건너뜀 (LG ICP-02 등 ETX 없는 프로토콜)',
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

  // --- IO: Samsung HVACR-01 (Samsung NASA 프로토콜) ---
  'samsung_hvacr01_status': {
    description: 'Samsung HVACR-01 에이전트(Samsung NASA 프로토콜)의 에어컨 상태를 수신합니다. 에이전트의 NotifyInterval 마다 디바이스별 상태를 push 받고, inactivity_timeout 동안 무수신 시에만 request_state 명령을 전송합니다.',
    inputDesc: '없음 (push 모델). 입력 메시지 수신 시 즉시 drain.',
    outputDesc: 'payload: 디바이스 상태 (전원, 모드, 온도, 풍량 등). metadata: agent:{type,id} 그룹 (기본) + device:{type,id} 그룹 (디바이스 노드, 기본) + device_id 평탄 키 필수 + node_id (옵션). 그룹은 emit_agent/emit_device 토글로 끌 수 있음',
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'Samsung HVACR-01 에이전트',
          required: true,
          options: ['samsung_hvacr01'],
          description: '연결할 Samsung HVACR-01 에이전트를 선택합니다',
        },
        {
          name: 'inactivity_timeout',
          type: 'string',
          label: '무수신 임계 시간',
          default: '90s',
          description: '이 시간 동안 에이전트로부터 메시지가 오지 않으면 request_state 명령을 전송. NotifyInterval (에이전트 설정) 보다 1.5x ~ 2x 권장.',
        },
        {
          name: 'timeout',
          type: 'string',
          label: 'Process 타임아웃',
          default: '5s',
          description: 'Agent Process 호출 타임아웃',
        },
        {
          name: 'batch_size',
          type: 'number',
          label: '배치 크기',
          default: 32,
          description: 'drain 시 한 번에 가져올 최대 프레임 수',
        },
        {
          name: 'omit_state_when_off',
          type: 'boolean',
          label: 'OFF 상태 시 상태 필드 제거',
          default: false,
          description: 'power=false 일 때 신뢰할 수 없는 상태 (current_temperature, mode, fan_speed) 를 메시지에서 제거',
        },
        // 고급: 어드레싱 (group_id / unit_id). 미지정 시 모든 디바이스 broadcast.
        {
          name: 'group_id',
          type: 'string',
          label: '그룹 ID (외기 인덱스)',
          description: 'Samsung NASA 외기 인덱스 hex (예: "00" ~ "0F"). 비우면 전체 그룹. 실외기 단독 조회 시 unit_id 와 동일 값 지정',
          advanced: true,
        },
        {
          name: 'unit_id',
          type: 'string',
          label: '유닛 ID (내기 인덱스)',
          description: 'Samsung NASA 내기 인덱스 hex (예: "00" ~ "3F"). 비우면 그룹 내 전체 유닛',
          advanced: true,
        },
        // 고급: 메타데이터 토글 (device_id 는 항상 emit, 나머지는 default OFF).
        { name: 'emit_node_id', type: 'boolean', label: '메타데이터: node_id', default: false, description: '메시지 metadata 에 emit 한 flow 노드 UUID 포함', advanced: true },
        { name: 'emit_device_type', type: 'boolean', label: '메타데이터: device_type', default: false, description: '메시지 metadata 에 device_type 포함 (예: HVACR.IDU / HVACR.ODU)', advanced: true },
        // P3: agent / device 그룹은 기본 ON. 토글 OFF 시에만 emit_agent/emit_device=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_device', type: 'boolean', label: '메타데이터: device 그룹', default: true, description: '메시지 metadata 에 device:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_name', type: 'boolean', label: '메타데이터: name', default: false, description: '메시지 metadata 에 사용자 이름(라벨) 포함', advanced: true },
        { name: 'emit_node_source', type: 'boolean', label: '메타데이터: node_source', default: false, description: '메시지 metadata 에 emit 경로 식별자 (notify / inactivity_request 등) 포함', advanced: true },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  'samsung_hvacr01_control': {
    description: 'Samsung HVACR-01 에이전트(Samsung NASA 프로토콜)의 에어컨을 제어합니다. 전원, 온도, 풍량, 모드 등을 설정합니다.',
    inputDesc: 'payload: {device_id, command, ...params} (예: {device_id:"01", command:"set_power", power:true})',
    outputDesc: 'payload: 에이전트 응답 (성공/실패 상태, 제어 결과)',
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'Samsung HVACR-01 에이전트',
          required: true,
          options: ['samsung_hvacr01'],
          description: '연결할 Samsung HVACR-01 에이전트를 선택합니다',
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
        {
          name: 'omit_state_when_off',
          type: 'boolean',
          label: 'OFF 상태 시 상태 필드 제거',
          default: false,
          description: 'power=false 일 때 신뢰할 수 없는 상태 (current_temperature, mode, fan_speed) 를 응답에서 제거',
        },
        // v0.18.8: emit_metadata 옵션 — device_id 만 항상 emit, 나머지는 default OFF.
        // v0.18.12: unit_id / node_id 도 옵션화 (이전엔 unit_id 필수 + node_id 자동).
        { name: 'emit_node_id', type: 'boolean', label: '메타데이터: node_id', default: false, description: '메시지 metadata 에 emit 한 노드 UUID 포함', advanced: true },
        { name: 'emit_device_type', type: 'boolean', label: '메타데이터: device_type', default: false, description: '메시지 metadata 에 device_type 포함 (예: HVACR.IDU / HVACR.ODU)', advanced: true },
        // P3: agent / device 그룹은 기본 ON. 토글 OFF 시에만 emit_agent/emit_device=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_device', type: 'boolean', label: '메타데이터: device 그룹', default: true, description: '메시지 metadata 에 device:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_name', type: 'boolean', label: '메타데이터: name', default: false, description: '메시지 metadata 에 사용자 이름(라벨) 포함', advanced: true },
        { name: 'emit_node_source', type: 'boolean', label: '메타데이터: node_source', default: false, description: '메시지 metadata 에 emit 경로 식별자 (poll / poll_bulk 등) 포함', advanced: true },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  samsung_hvacr01: {
    description: 'Samsung HVACR-01 에이전트(Samsung NASA 프로토콜)의 에어컨 상태 수신 + 제어 통합 노드입니다. push 모델로 동작하며, 무수신 임계 시간 초과 시 request_state 자동 전송.',
    inputDesc: '상태 조회 트리거 또는 제어 명령. payload 에 제어 키(power, mode, temperature 등) 가 있으면 제어, 없으면 즉시 drain.',
    outputDesc: 'payload: 디바이스 상태 또는 제어 결과 JSON',
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'Samsung HVACR-01 에이전트',
          required: true,
          options: ['samsung_hvacr01'],
          description: '연결할 Samsung HVACR-01 에이전트를 선택합니다',
        },
        {
          name: 'inactivity_timeout',
          type: 'string',
          label: '무수신 임계 시간',
          default: '90s',
          description: '이 시간 동안 에이전트로부터 메시지가 오지 않으면 request_state 명령을 전송',
        },
        {
          name: 'timeout',
          type: 'string',
          label: 'Process 타임아웃',
          default: '5s',
          description: 'Agent Process 호출 타임아웃',
        },
        {
          name: 'batch_size',
          type: 'number',
          label: '배치 크기',
          default: 32,
          description: 'drain 시 한 번에 가져올 최대 프레임 수',
        },
        {
          name: 'omit_state_when_off',
          type: 'boolean',
          label: 'OFF 상태 시 상태 필드 제거',
          default: false,
          description: 'power=false 일 때 신뢰할 수 없는 상태 (current_temperature, mode, fan_speed) 를 메시지에서 제거',
        },
        // 고급: 어드레싱 (group_id / unit_id). 미지정 시 모든 디바이스 broadcast.
        {
          name: 'group_id',
          type: 'string',
          label: '그룹 ID (외기 인덱스)',
          description: 'Samsung NASA 외기 인덱스 hex (예: "00" ~ "0F"). 비우면 전체 그룹',
          advanced: true,
        },
        {
          name: 'unit_id',
          type: 'string',
          label: '유닛 ID (내기 인덱스)',
          description: 'Samsung NASA 내기 인덱스 hex (예: "00" ~ "3F"). 비우면 그룹 내 전체 유닛',
          advanced: true,
        },
        // 고급: 메타데이터 토글 (device_id 는 항상 emit, 나머지는 default OFF).
        { name: 'emit_node_id', type: 'boolean', label: '메타데이터: node_id', default: false, description: '메시지 metadata 에 emit 한 flow 노드 UUID 포함', advanced: true },
        { name: 'emit_device_type', type: 'boolean', label: '메타데이터: device_type', default: false, description: '메시지 metadata 에 device_type 포함 (예: HVACR.IDU / HVACR.ODU)', advanced: true },
        // P3: agent / device 그룹은 기본 ON. 토글 OFF 시에만 emit_agent/emit_device=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_device', type: 'boolean', label: '메타데이터: device 그룹', default: true, description: '메시지 metadata 에 device:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_name', type: 'boolean', label: '메타데이터: name', default: false, description: '메시지 metadata 에 사용자 이름(라벨) 포함', advanced: true },
        { name: 'emit_node_source', type: 'boolean', label: '메타데이터: node_source', default: false, description: '메시지 metadata 에 emit 경로 식별자 (notify / inactivity_request 등) 포함', advanced: true },
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
    outputDesc: 'payload: {devices: [{id, name, power, mode, target_temperature, current_temperature, fan_speed, ...}]}',
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
          description: '조회할 디바이스 ID (미지정 시 get_all 로 전체 조회)',
        },
        {
          name: 'poll_command',
          type: 'select',
          label: '폴링 명령',
          default: 'get_recent',
          options: ['get_recent', 'get_all', 'get_state', 'get_stats'],
          description: 'get_recent (변경 누적, lastSeq cursor) / get_all (모든 디바이스 즉시) / get_state (단일 디바이스, device_id 필요) / get_stats (에이전트 통계)',
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
          name: 'omit_state_when_off',
          type: 'boolean',
          label: 'OFF 상태 시 상태 필드 제거',
          default: false,
          description: 'power=false 일 때 신뢰할 수 없는 상태 (current_temperature, mode, fan_speed) 를 메시지에서 제거',
        },
        // v0.18.8: emit_metadata 옵션 — device_id 만 항상 emit, 나머지는 default OFF.
        // v0.18.12: unit_id / node_id 도 옵션화 (이전엔 unit_id 필수 + node_id 자동).
        { name: 'emit_node_id', type: 'boolean', label: '메타데이터: node_id', default: false, description: '메시지 metadata 에 emit 한 노드 UUID 포함', advanced: true },
        { name: 'emit_device_type', type: 'boolean', label: '메타데이터: device_type', default: false, description: '메시지 metadata 에 device_type 포함 (예: HVACR.IDU / HVACR.ODU)', advanced: true },
        // P3: agent / device 그룹은 기본 ON. 토글 OFF 시에만 emit_agent/emit_device=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_device', type: 'boolean', label: '메타데이터: device 그룹', default: true, description: '메시지 metadata 에 device:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_name', type: 'boolean', label: '메타데이터: name', default: false, description: '메시지 metadata 에 사용자 이름(라벨) 포함', advanced: true },
        { name: 'emit_node_source', type: 'boolean', label: '메타데이터: node_source', default: false, description: '메시지 metadata 에 emit 경로 식별자 (poll / poll_bulk 등) 포함', advanced: true },
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
        {
          name: 'omit_state_when_off',
          type: 'boolean',
          label: 'OFF 상태 시 상태 필드 제거',
          default: false,
          description: 'power=false 일 때 신뢰할 수 없는 상태 (current_temperature, mode, fan_speed) 를 응답에서 제거',
        },
        // v0.18.8: emit_metadata 옵션 — device_id 만 항상 emit, 나머지는 default OFF.
        // v0.18.12: unit_id / node_id 도 옵션화 (이전엔 unit_id 필수 + node_id 자동).
        { name: 'emit_node_id', type: 'boolean', label: '메타데이터: node_id', default: false, description: '메시지 metadata 에 emit 한 노드 UUID 포함', advanced: true },
        { name: 'emit_device_type', type: 'boolean', label: '메타데이터: device_type', default: false, description: '메시지 metadata 에 device_type 포함 (예: HVACR.IDU / HVACR.ODU)', advanced: true },
        // P3: agent / device 그룹은 기본 ON. 토글 OFF 시에만 emit_agent/emit_device=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_device', type: 'boolean', label: '메타데이터: device 그룹', default: true, description: '메시지 metadata 에 device:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_name', type: 'boolean', label: '메타데이터: name', default: false, description: '메시지 metadata 에 사용자 이름(라벨) 포함', advanced: true },
        { name: 'emit_node_source', type: 'boolean', label: '메타데이터: node_source', default: false, description: '메시지 metadata 에 emit 경로 식별자 (poll / poll_bulk 등) 포함', advanced: true },
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
        {
          name: 'omit_state_when_off',
          type: 'boolean',
          label: 'OFF 상태 시 상태 필드 제거',
          default: false,
          description: 'power=false 일 때 신뢰할 수 없는 상태 (current_temperature, mode, fan_speed) 를 메시지에서 제거',
        },
        // v0.18.8: emit_metadata 옵션 — device_id 만 항상 emit, 나머지는 default OFF.
        // v0.18.12: unit_id / node_id 도 옵션화 (이전엔 unit_id 필수 + node_id 자동).
        { name: 'emit_node_id', type: 'boolean', label: '메타데이터: node_id', default: false, description: '메시지 metadata 에 emit 한 노드 UUID 포함', advanced: true },
        { name: 'emit_device_type', type: 'boolean', label: '메타데이터: device_type', default: false, description: '메시지 metadata 에 device_type 포함 (예: HVACR.IDU / HVACR.ODU)', advanced: true },
        // P3: agent / device 그룹은 기본 ON. 토글 OFF 시에만 emit_agent/emit_device=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_device', type: 'boolean', label: '메타데이터: device 그룹', default: true, description: '메시지 metadata 에 device:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_name', type: 'boolean', label: '메타데이터: name', default: false, description: '메시지 metadata 에 사용자 이름(라벨) 포함', advanced: true },
        { name: 'emit_node_source', type: 'boolean', label: '메타데이터: node_source', default: false, description: '메시지 metadata 에 emit 경로 식별자 (poll / poll_bulk 등) 포함', advanced: true },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  // --- IO: LG HVACR-02 (ICP-02 protocol) ---
  'lg_hvacr02_status': {
    description: 'LG HVACR-02 에이전트(LG ICP-02 프로토콜)의 에어컨 상태를 수신합니다. 에이전트의 FrameNotifyCh 신호 수신 시 디바이스별 상태를 push 받고, inactivity_timeout 동안 무수신 시에만 request_state 명령을 전송합니다. (2026-05-30 LG HVACR-01 통일 패턴)',
    inputDesc: '없음 (push 모델). 입력 메시지 수신 시 즉시 drain.',
    outputDesc: 'payload: 디바이스 상태 (전원, 모드, 온도, 풍량 등). metadata: agent:{type,id} 그룹 (기본) + device:{type,id} 그룹 (디바이스 노드, 기본) + device_id 평탄 키 필수 + node_id (옵션). 그룹은 emit_agent/emit_device 토글로 끌 수 있음',
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'LG HVACR-02 에이전트',
          required: true,
          options: ['lg_hvacr02'],
          description: '연결할 LG HVACR-02 에이전트를 선택합니다',
        },
        {
          name: 'inactivity_timeout',
          type: 'string',
          label: '무수신 임계 시간',
          default: '90s',
          description: '이 시간 동안 에이전트로부터 메시지가 오지 않으면 request_state 명령을 전송. report_interval (에이전트 설정) 보다 1.5x ~ 2x 권장.',
        },
        {
          name: 'timeout',
          type: 'string',
          label: 'Process 타임아웃',
          default: '5s',
          description: 'Agent Process 호출 타임아웃',
        },
        {
          name: 'batch_size',
          type: 'number',
          label: '배치 크기',
          default: 32,
          description: 'drain 시 한 번에 가져올 최대 프레임 수',
        },
        {
          name: 'omit_state_when_off',
          type: 'boolean',
          label: 'OFF 상태 시 상태 필드 제거',
          default: false,
          description: 'power=false 일 때 신뢰할 수 없는 상태 (current_temperature, mode, fan_speed) 를 메시지에서 제거',
        },
        // 고급: 어드레싱 (unit_id). 미지정 시 모든 디바이스 broadcast.
        {
          name: 'unit_id',
          type: 'string',
          label: '유닛 ID (hex)',
          description: 'LG ICP-02 디바이스 주소 (hex byte, 예: "58"). 비우면 전체 디바이스 수신',
          advanced: true,
        },
        // 고급: 메타데이터 토글 (device_id 는 항상 emit, 나머지는 default OFF).
        { name: 'emit_node_id', type: 'boolean', label: '메타데이터: node_id', default: false, description: '메시지 metadata 에 emit 한 flow 노드 UUID 포함', advanced: true },
        { name: 'emit_device_type', type: 'boolean', label: '메타데이터: device_type', default: false, description: '메시지 metadata 에 device_type 포함 (예: HVACR.IDU / HVACR.ODU)', advanced: true },
        // P3: agent / device 그룹은 기본 ON. 토글 OFF 시에만 emit_agent/emit_device=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_device', type: 'boolean', label: '메타데이터: device 그룹', default: true, description: '메시지 metadata 에 device:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_name', type: 'boolean', label: '메타데이터: name', default: false, description: '메시지 metadata 에 사용자 이름(라벨) 포함', advanced: true },
        { name: 'emit_node_source', type: 'boolean', label: '메타데이터: node_source', default: false, description: '메시지 metadata 에 emit 경로 식별자 (push / inactivity_request 등) 포함', advanced: true },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  'lg_hvacr02_control': {
    description: 'LG ICP-02 프로토콜로 실내기를 제어합니다. 전원, 온도, 풍량, 모드를 설정합니다.',
    inputDesc: 'payload: {address, command, ...params} (예: {address:"67", command:"set_power", power:true})',
    outputDesc: 'payload: 에이전트 응답 (성공/실패 상태, 제어 결과)',
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'LG HVACR-02 에이전트',
          required: true,
          options: ['lg_hvacr02'],
          description: '연결할 LG HVACR-02 에이전트를 선택합니다',
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
        {
          name: 'omit_state_when_off',
          type: 'boolean',
          label: 'OFF 상태 시 상태 필드 제거',
          default: false,
          description: 'power=false 일 때 신뢰할 수 없는 상태 (current_temperature, mode, fan_speed) 를 응답에서 제거',
        },
        // v0.18.8: emit_metadata 옵션 — device_id 만 항상 emit, 나머지는 default OFF.
        // v0.18.12: unit_id / node_id 도 옵션화 (이전엔 unit_id 필수 + node_id 자동).
        { name: 'emit_node_id', type: 'boolean', label: '메타데이터: node_id', default: false, description: '메시지 metadata 에 emit 한 노드 UUID 포함', advanced: true },
        { name: 'emit_device_type', type: 'boolean', label: '메타데이터: device_type', default: false, description: '메시지 metadata 에 device_type 포함 (예: HVACR.IDU / HVACR.ODU)', advanced: true },
        // P3: agent / device 그룹은 기본 ON. 토글 OFF 시에만 emit_agent/emit_device=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_device', type: 'boolean', label: '메타데이터: device 그룹', default: true, description: '메시지 metadata 에 device:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_name', type: 'boolean', label: '메타데이터: name', default: false, description: '메시지 metadata 에 사용자 이름(라벨) 포함', advanced: true },
        { name: 'emit_node_source', type: 'boolean', label: '메타데이터: node_source', default: false, description: '메시지 metadata 에 emit 경로 식별자 (poll / poll_bulk 등) 포함', advanced: true },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  lg_hvacr02: {
    description: 'LG HVACR-02 실내기 상태 조회 + 제어 통합 노드입니다.',
    inputDesc: 'payload.address (조회/제어 대상), payload.command + params (제어 시)',
    outputDesc: 'payload: 디바이스 상태 또는 제어 결과 JSON',
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'LG HVACR-02 에이전트',
          required: true,
          options: ['lg_hvacr02'],
          description: '연결할 LG HVACR-02 에이전트를 선택합니다',
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
          default: 'get_recent',
          options: ['get_recent', 'get_all', 'get_state', 'get_stats'],
          description: 'get_recent (count=0=drain) / get_all (모든 device 즉시) / get_state (단일 device) / get_stats (통계)',
        },
        {
          name: 'recent_count',
          type: 'number',
          label: '최근 데이터 수',
          default: 10,
          description: 'get_recent 명령 시 조회할 최근 데이터 수',
        },
        {
          name: 'omit_state_when_off',
          type: 'boolean',
          label: 'OFF 상태 시 상태 필드 제거',
          default: false,
          description: 'power=false 일 때 신뢰할 수 없는 상태 (current_temperature, mode, fan_speed) 를 메시지에서 제거',
        },
        // v0.18.8: emit_metadata 옵션 — device_id 만 항상 emit, 나머지는 default OFF.
        // v0.18.12: unit_id / node_id 도 옵션화 (이전엔 unit_id 필수 + node_id 자동).
        { name: 'emit_node_id', type: 'boolean', label: '메타데이터: node_id', default: false, description: '메시지 metadata 에 emit 한 노드 UUID 포함', advanced: true },
        { name: 'emit_device_type', type: 'boolean', label: '메타데이터: device_type', default: false, description: '메시지 metadata 에 device_type 포함 (예: HVACR.IDU / HVACR.ODU)', advanced: true },
        // P3: agent / device 그룹은 기본 ON. 토글 OFF 시에만 emit_agent/emit_device=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_device', type: 'boolean', label: '메타데이터: device 그룹', default: true, description: '메시지 metadata 에 device:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_name', type: 'boolean', label: '메타데이터: name', default: false, description: '메시지 metadata 에 사용자 이름(라벨) 포함', advanced: true },
        { name: 'emit_node_source', type: 'boolean', label: '메타데이터: node_source', default: false, description: '메시지 metadata 에 emit 경로 식별자 (poll / poll_bulk 등) 포함', advanced: true },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  // --- IO: LG HVACR-01 (LG ICP-01 protocol) ---
  'lg_hvacr01_status': {
    description: 'LG HVACR-01 에이전트(LG ICP-01 프로토콜)의 에어컨 상태를 수신합니다. 에이전트의 NotifyInterval 마다 디바이스별 상태를 push 받고, inactivity_timeout 동안 무수신 시에만 request_state 명령을 전송합니다.',
    inputDesc: '없음 (push 모델). 입력 메시지 수신 시 즉시 drain.',
    outputDesc: 'payload: 디바이스 상태 (전원, 모드, 온도, 풍량 등). metadata: agent:{type,id} 그룹 (기본) + device:{type,id} 그룹 (디바이스 노드, 기본) + device_id 평탄 키 필수 + node_id (옵션). 그룹은 emit_agent/emit_device 토글로 끌 수 있음',
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'LG HVACR-01 에이전트',
          required: true,
          options: ['lg_hvacr01'],
          description: '연결할 LG HVACR-01 에이전트를 선택합니다',
        },
        {
          name: 'inactivity_timeout',
          type: 'string',
          label: '무수신 임계 시간',
          default: '90s',
          description: '이 시간 동안 에이전트로부터 메시지가 오지 않으면 request_state 명령을 전송. NotifyInterval (에이전트 설정) 보다 1.5x ~ 2x 권장.',
        },
        {
          name: 'timeout',
          type: 'string',
          label: 'Process 타임아웃',
          default: '5s',
          description: 'Agent Process 호출 타임아웃',
        },
        {
          name: 'batch_size',
          type: 'number',
          label: '배치 크기',
          default: 32,
          description: 'drain 시 한 번에 가져올 최대 프레임 수',
        },
        {
          name: 'omit_state_when_off',
          type: 'boolean',
          label: 'OFF 상태 시 상태 필드 제거',
          default: false,
          description: 'power=false 일 때 신뢰할 수 없는 상태 (current_temperature, mode, fan_speed) 를 메시지에서 제거',
        },
        // 고급: 어드레싱 (unit_id). LG ICP-01 은 group_id 를 사용하지 않음. 미지정 시 모든 디바이스 broadcast.
        {
          name: 'unit_id',
          type: 'string',
          label: '유닛 ID (STX hex)',
          description: 'LG ICP-01 STX hex (예: ODU="58", IDU="81" ~ "BF"). 비우면 전체 디바이스 수신',
          advanced: true,
        },
        // 고급: 메타데이터 토글 (device_id 는 항상 emit, 나머지는 default OFF).
        { name: 'emit_node_id', type: 'boolean', label: '메타데이터: node_id', default: false, description: '메시지 metadata 에 emit 한 flow 노드 UUID 포함', advanced: true },
        { name: 'emit_device_type', type: 'boolean', label: '메타데이터: device_type', default: false, description: '메시지 metadata 에 device_type 포함 (예: HVACR.IDU / HVACR.ODU)', advanced: true },
        // P3: agent / device 그룹은 기본 ON. 토글 OFF 시에만 emit_agent/emit_device=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_device', type: 'boolean', label: '메타데이터: device 그룹', default: true, description: '메시지 metadata 에 device:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_name', type: 'boolean', label: '메타데이터: name', default: false, description: '메시지 metadata 에 사용자 이름(라벨) 포함', advanced: true },
        { name: 'emit_node_source', type: 'boolean', label: '메타데이터: node_source', default: false, description: '메시지 metadata 에 emit 경로 식별자 (notify / inactivity_request 등) 포함', advanced: true },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  'lg_hvacr01_control': {
    description: 'LG HVACR-01 디바이스 제어 (현재 미지원 - 프로토콜 분석 진행 중)',
    configSchema: {
      fields: [
        { name: 'agent_ref', type: 'agent_select', label: 'LG HVACR-01 에이전트', required: true, options: ['lg_hvacr01'] },
        { name: 'timeout', type: 'string', label: '타임아웃', default: '5s' },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  lg_hvacr01: {
    description: 'LG HVACR-01 상태 수신 + 제어 통합 노드. push 모델 (lg_hvacr01_status 와 동일). 제어는 현재 미지원.',
    configSchema: {
      fields: [
        { name: 'agent_ref', type: 'agent_select', label: 'LG HVACR-01 에이전트', required: true, options: ['lg_hvacr01'] },
        { name: 'inactivity_timeout', type: 'string', label: '무수신 임계 시간', default: '90s', description: '이 시간 동안 에이전트로부터 메시지가 오지 않으면 request_state 명령을 전송' },
        { name: 'timeout', type: 'string', label: 'Process 타임아웃', default: '5s' },
        { name: 'batch_size', type: 'number', label: '배치 크기', default: 32, description: 'drain 시 한 번에 가져올 최대 프레임 수' },
        { name: 'omit_state_when_off', type: 'boolean', label: 'OFF 상태 시 상태 필드 제거', default: false, description: 'power=false 일 때 신뢰할 수 없는 상태 (current_temperature, mode, fan_speed) 를 메시지에서 제거' },
        // 고급: 어드레싱 (unit_id). LG ICP-01 은 group_id 를 사용하지 않음.
        { name: 'unit_id', type: 'string', label: '유닛 ID (STX hex)', description: 'LG ICP-01 STX hex (예: ODU="58", IDU="81" ~ "BF"). 비우면 전체 디바이스 수신', advanced: true },
        // 고급: 메타데이터 토글 (device_id 는 항상 emit, 나머지는 default OFF).
        { name: 'emit_node_id', type: 'boolean', label: '메타데이터: node_id', default: false, description: '메시지 metadata 에 emit 한 flow 노드 UUID 포함', advanced: true },
        { name: 'emit_device_type', type: 'boolean', label: '메타데이터: device_type', default: false, description: '메시지 metadata 에 device_type 포함 (예: HVACR.IDU / HVACR.ODU)', advanced: true },
        // P3: agent / device 그룹은 기본 ON. 토글 OFF 시에만 emit_agent/emit_device=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_device', type: 'boolean', label: '메타데이터: device 그룹', default: true, description: '메시지 metadata 에 device:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_name', type: 'boolean', label: '메타데이터: name', default: false, description: '메시지 metadata 에 사용자 이름(라벨) 포함', advanced: true },
        { name: 'emit_node_source', type: 'boolean', label: '메타데이터: node_source', default: false, description: '메시지 metadata 에 emit 경로 식별자 (notify / inactivity_request 등) 포함', advanced: true },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  // --- IO: Century HVACR-01 (SPEC-CENTURY-HVACR-001) ---
  'century_hvacr01_status': {
    description: 'Century HVACR-01 에이전트(Century ICP-01 프로토콜)의 에어컨 상태를 수신합니다. 에이전트의 NotifyInterval 마다 디바이스별 상태를 push 받고, inactivity_timeout 동안 무수신 시에만 request_state 명령을 전송합니다.',
    inputDesc: '없음 (push 모델). 입력 메시지 수신 시 즉시 drain.',
    outputDesc: 'payload: 디바이스 상태 (전원, 모드, 온도, 풍량 등). metadata: agent:{type,id} 그룹 (기본) + device:{type,id} 그룹 (디바이스 노드, 기본) + device_id 평탄 키 필수 + node_id (옵션). 그룹은 emit_agent/emit_device 토글로 끌 수 있음',
    configSchema: {
      fields: [
        {
          name: 'agent_ref',
          type: 'agent_select',
          label: 'Century HVACR-01 에이전트',
          required: true,
          options: ['century_hvacr01'],
          description: '연결할 Century HVACR-01 에이전트를 선택합니다',
        },
        {
          name: 'inactivity_timeout',
          type: 'string',
          label: '무수신 임계 시간',
          default: '90s',
          description: '이 시간 동안 에이전트로부터 메시지가 오지 않으면 request_state 명령을 전송. NotifyInterval (에이전트 설정) 보다 1.5x ~ 2x 권장.',
        },
        {
          name: 'timeout',
          type: 'string',
          label: 'Process 타임아웃',
          default: '5s',
          description: 'Agent Process 호출 타임아웃',
        },
        {
          name: 'batch_size',
          type: 'number',
          label: '배치 크기',
          default: 32,
          description: 'drain 시 한 번에 가져올 최대 프레임 수',
        },
        {
          name: 'omit_state_when_off',
          type: 'boolean',
          label: 'OFF 상태 시 상태 필드 제거',
          default: false,
          description: 'power=false 일 때 신뢰할 수 없는 상태 (current_temperature, mode, fan_speed) 를 메시지에서 제거',
        },
        // 고급: 어드레싱 (unit_id). Century ICP-01 은 group_id 를 사용하지 않음. 미지정 시 모든 디바이스 broadcast.
        {
          name: 'unit_id',
          type: 'string',
          label: '유닛 ID (sub_dev_id hex)',
          description: 'Century sub_dev_id hex (예: "3B"). 비우면 전체 디바이스 수신',
          advanced: true,
        },
        // 고급: 메타데이터 토글 (device_id 는 항상 emit, 나머지는 default OFF).
        { name: 'emit_node_id', type: 'boolean', label: '메타데이터: node_id', default: false, description: '메시지 metadata 에 emit 한 flow 노드 UUID 포함', advanced: true },
        { name: 'emit_device_type', type: 'boolean', label: '메타데이터: device_type', default: false, description: '메시지 metadata 에 device_type 포함 (예: HVACR.IDU / HVACR.ODU)', advanced: true },
        // P3: agent / device 그룹은 기본 ON. 토글 OFF 시에만 emit_agent/emit_device=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_device', type: 'boolean', label: '메타데이터: device 그룹', default: true, description: '메시지 metadata 에 device:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_name', type: 'boolean', label: '메타데이터: name', default: false, description: '메시지 metadata 에 사용자 이름(라벨) 포함', advanced: true },
        { name: 'emit_node_source', type: 'boolean', label: '메타데이터: node_source', default: false, description: '메시지 metadata 에 emit 경로 식별자 (notify / inactivity_request 등) 포함', advanced: true },
        // 디버그/분석 (Century 전용 — 출력 폭주 우려, 운영 환경 비활성 권장):
        {
          name: 'emit_raw_frames',
          type: 'boolean',
          label: 'Raw frame 송출 모드',
          default: false,
          description: 'true 시 device_state 대신 ring buffer 전체 frame 을 raw 형태로 송출 (디버그용). dedupe 와 무관하게 모든 프레임 emit, drain 강제',
          advanced: true,
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  'century_hvacr01_control': {
    description: 'Century HVACR-01 디바이스 제어 (미지원 — 패시브 전용)',
    configSchema: {
      fields: [
        { name: 'agent_ref', type: 'agent_select', label: 'Century 에이전트', required: true, options: ['century_hvacr01'] },
        { name: 'timeout', type: 'string', label: '타임아웃', default: '5s' },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  century_hvacr01: {
    description: 'Century HVACR-01 상태 수신 + 제어 통합 노드 (제어는 항상 not_supported 반환). push 모델.',
    configSchema: {
      fields: [
        { name: 'agent_ref', type: 'agent_select', label: 'Century 에이전트', required: true, options: ['century_hvacr01'] },
        { name: 'inactivity_timeout', type: 'string', label: '무수신 임계 시간', default: '90s', description: '이 시간 동안 에이전트로부터 메시지가 오지 않으면 request_state 명령을 전송' },
        { name: 'timeout', type: 'string', label: 'Process 타임아웃', default: '5s', description: 'Agent Process 호출 타임아웃' },
        { name: 'batch_size', type: 'number', label: '배치 크기', default: 32, description: 'drain 시 한 번에 가져올 최대 프레임 수' },
        { name: 'omit_state_when_off', type: 'boolean', label: 'OFF 상태 시 상태 필드 제거', default: false, description: 'power=false 일 때 신뢰할 수 없는 상태 (current_temperature, mode, fan_speed) 를 메시지에서 제거' },
        { name: 'emit_raw_frames', type: 'boolean', label: 'Raw frame 송출 모드', default: false, description: 'true 시 device_state 대신 ring buffer 전체 frame 을 raw 형태로 송출 (디버그용). dedupe 와 무관하게 모든 프레임 emit, drain 강제' },
        // 고급: 어드레싱 (unit_id). Century ICP-01 은 group_id 를 사용하지 않음.
        { name: 'unit_id', type: 'string', label: '유닛 ID (sub_dev_id hex)', description: 'Century sub_dev_id hex (예: "3B"). 비우면 전체 디바이스 수신', advanced: true },
        // 고급: 메타데이터 토글 (device_id 는 항상 emit, 나머지는 default OFF).
        { name: 'emit_node_id', type: 'boolean', label: '메타데이터: node_id', default: false, description: '메시지 metadata 에 emit 한 flow 노드 UUID 포함', advanced: true },
        { name: 'emit_device_type', type: 'boolean', label: '메타데이터: device_type', default: false, description: '메시지 metadata 에 device_type 포함 (예: HVACR.IDU / HVACR.ODU)', advanced: true },
        // P3: agent / device 그룹은 기본 ON. 토글 OFF 시에만 emit_agent/emit_device=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_device', type: 'boolean', label: '메타데이터: device 그룹', default: true, description: '메시지 metadata 에 device:{type,id} 그룹 포함 (기본 ON)', advanced: true },
        { name: 'emit_name', type: 'boolean', label: '메타데이터: name', default: false, description: '메시지 metadata 에 사용자 이름(라벨) 포함', advanced: true },
        { name: 'emit_node_source', type: 'boolean', label: '메타데이터: node_source', default: false, description: '메시지 metadata 에 emit 경로 식별자 (notify / inactivity_request 등) 포함', advanced: true },
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
        // P3: agent 그룹은 기본 ON. 토글 OFF 시에만 emit_agent=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
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
    outputDesc: '조건에 매칭된 포트로 메시지 전달 (원본 Clone). 미매칭 시 기본 포트(설정 시) 또는 드랍',
    configSchema: {
      fields: [
        {
          name: 'routes',
          type: 'routes_editor',
          label: '라우팅 규칙',
          description:
            '위에서부터 순서대로 평가됩니다. 각 행은 (조건식, 출력 포트명) 쌍입니다. 조건식은 $.payload.* / $.metadata.* 경로와 == != > < >= <=, && || !, exists(path) 를 지원합니다.',
        },
        {
          name: 'match_mode',
          type: 'select',
          label: '매칭 모드',
          options: ['first', 'all'],
          default: 'first',
          description:
            '첫 매칭(first): 처음 일치하는 라우트 1개로만 전달. 모두 매칭(all): 일치하는 모든 라우트로 팬아웃.',
        },
        {
          name: 'pass_mode',
          type: 'select',
          label: '조건 일치 시 전송 방식',
          options: ['copy', 'original'],
          default: 'copy',
          description:
            'copy: 복사본 전송(메시지 id 새로 부여) / original: 원본 전송(메시지 id 유지). all 모드에서 한 메시지가 2개 이상 포트로 가는 경우는 복사본이 강제됩니다.',
        },
        {
          name: 'default_port',
          type: 'string',
          label: '기본 포트(미매칭)',
          placeholder: '예: other',
          description: '일치하는 조건이 없을 때 사용할 출력 포트명. 비우면 미매칭 메시지를 드랍합니다.',
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
    outputDesc: 'payload: 수신 데이터 (json: 파싱된 객체, raw: {raw: []byte}). metadata: agent:{type,id} 그룹 (기본, emit_agent 토글로 OFF) + mqtt.topic + mqtt.qos',
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
        // P3: agent 그룹은 기본 ON. 토글 OFF 시에만 emit_agent=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
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
        // P3: agent 그룹은 기본 ON. 토글 OFF 시에만 emit_agent=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
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
          description: '기본 발행 토픽. {expr} 형식으로 메시지 필드 보간 지원 — JSONPath ($.payload.X, $.metadata.X, 그룹 중첩 $.metadata.device.X, $.type, $.timestamp) 또는 페이로드 직접 키. 예: `xflow/{$.metadata.device.type}/{$.metadata.device.id}/status`. 메시지의 metadata.mqtt.topic 으로 오버라이드 가능.',
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
        // P3: agent 그룹은 기본 ON. 토글 OFF 시에만 emit_agent=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
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
    inputDesc: 'payload: 저장할 필드 데이터, metadata: 태그. tag_mappings (tag → metadata 키) / field_mappings 로 세부 매핑.',
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
          description: 'InfluxDB 태그 이름 → metadata 키 (예: { device: device_id }). 비워두면 모든 metadata 가 동일 이름으로 tag 로 매핑됨.',
        },
        {
          name: 'field_mappings',
          type: 'key_value_map',
          label: '필드 매핑',
          description: '필드 이름 → JSONPath ($.payload.X / $.metadata.X / $.type / $.timestamp). 비워두면 전체 payload 를 필드로 사용.',
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
  // --- IO: InfluxDB ---
  'influxdb-write': {
    description: 'InfluxDB에 시계열 데이터를 기록합니다. measurement, tags, fields를 payload에서 추출하여 기록합니다.',
    inputDesc: '기록할 데이터. measurement, tags, fields를 payload에서 추출',
    outputDesc: '원본 메시지 pass-through',
    configSchema: {
      fields: [
        { name: 'agent_ref', type: 'agent_select', label: 'InfluxDB 에이전트', required: true, options: ['influxdb'] },
        { name: 'measurement', type: 'string', label: 'Measurement', description: '고정 measurement 이름. 비어있으면 measurement_key 사용' },
        {
          name: 'measurement_key',
          type: 'string',
          label: 'Measurement 키',
          description: 'measurement 를 추출할 키. JSONPath 지원 ($.payload.X / $.metadata.X / $.type). measurement 가 비어있을 때 사용.',
          placeholder: '$.payload.metric_name',
        },
        {
          name: 'tag_mappings',
          type: 'key_value_map',
          label: '태그 매핑',
          description: 'InfluxDB 태그 이름 → 값. 값은 $. JSONPath 로 메시지 내 임의 키 참조 ($.payload.X / $.metadata.X / $.type). $. 없으면 metadata 키로 해석(하위 호환). 오브젝트 값은 JSON 문자열로 변환. 비워두면 모든 metadata 를 동일 이름의 tag 로 매핑.',
          keyLabel: '태그 이름',
          valueLabel: '값 ($. JSONPath / metadata 키)',
          valuePlaceholder: '$.payload.region 또는 metadata 키',
          pathHelper: true,
        },
        {
          name: 'field_mappings',
          type: 'key_value_map',
          label: '필드 매핑',
          description: '필드 이름 → 값. 값은 $. JSONPath 로 임의 키 참조 ($.payload.X / $.metadata.X / $.type / $.timestamp). 오브젝트 값은 JSON 문자열로 변환. 비워두면 전체 payload 를 필드로 사용.',
          keyLabel: '필드 이름',
          valueLabel: '값 ($. JSONPath)',
          valuePlaceholder: '$.payload.temperature',
          pathHelper: true,
        },
        { name: 'timestamp_key', type: 'string', label: '타임스탬프 키', description: 'payload에서 Unix 밀리초 타임스탬프를 추출할 키' },
        { name: 'bool_to_int', type: 'boolean', label: 'Boolean → 정수 변환', default: false, description: 'true/false 값을 1/0 정수로 변환하여 기록' },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
    ],
  },

  'influxdb-read': {
    description: 'InfluxDB를 주기적으로 쿼리하여 결과를 메시지로 출력합니다. Flux, SQL, InfluxQL 쿼리를 지원합니다.',
    outputDesc: '쿼리 결과의 각 행이 개별 메시지로 출력',
    configSchema: {
      fields: [
        { name: 'agent_ref', type: 'agent_select', label: 'InfluxDB 에이전트', required: true, options: ['influxdb'] },
        { name: 'query', type: 'string', label: '쿼리', required: true, description: 'Flux, SQL, 또는 InfluxQL 쿼리' },
        { name: 'language', type: 'select', label: '쿼리 언어', options: ['flux', 'sql', 'influxql'], default: 'flux' },
        { name: 'poll_interval', type: 'string', label: '폴링 주기', default: '30s', description: '쿼리 실행 간격 (예: 10s, 1m, 5m)' },
        { name: 'timeout', type: 'string', label: '타임아웃', default: '10s' },
        { name: 'output_mode', type: 'select', label: '출력 모드', options: ['rows', 'batch', 'grouped'], default: 'rows', description: 'rows: 행별 개별 메시지, batch: 전체 결과 단일 메시지, grouped: 필드별 시계열 배열' },
      ],
    },
    defaultPorts: [
      { name: 'out', direction: 'output' },
    ],
  },

  'influxdb-query': {
    description: '입력 메시지를 트리거로 InfluxDB 쿼리를 실행합니다. $variable 패턴으로 payload 값을 쿼리에 치환할 수 있습니다.',
    inputDesc: '쿼리를 트리거할 메시지. $variable 치환 소스',
    outputDesc: '쿼리 결과를 result_key에 담은 새 메시지',
    configSchema: {
      fields: [
        { name: 'agent_ref', type: 'agent_select', label: 'InfluxDB 에이전트', required: true, options: ['influxdb'] },
        { name: 'query', type: 'string', label: '쿼리', description: 'Flux/SQL/InfluxQL 쿼리. $variable로 payload 값 치환 가능. 비어있으면 payload의 query 키 사용' },
        { name: 'language', type: 'select', label: '쿼리 언어', options: ['flux', 'sql', 'influxql'], default: 'flux' },
        { name: 'timeout', type: 'string', label: '타임아웃', default: '10s' },
        { name: 'result_key', type: 'string', label: '결과 키', default: 'results', description: '쿼리 결과를 저장할 payload 키' },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' },
      { name: 'out', direction: 'output' },
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
          description:
            '저장할 값의 경로. bare 이름(field, item.nested)=payload 필드, $.payload.x / $.metadata.device.id / $.metadata.node_id / $.id / $.type / $.timestamp = 메시지 전체 경로. 비워두면 전체 payload 저장.',
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
          name: 'output_enabled',
          type: 'boolean',
          label: '출력 활성화',
          default: true,
          description: 'OFF 시 출력은 건너뛰고 메시지는 그대로 통과시킵니다. 노드 카드의 ON/OFF 버튼과 연동됩니다.',
        },
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
    outputDesc: 'out: payload {raw: []byte, data: string}. raw_out: 프레이밍 이전 원시 바이트 {raw: []byte}. metadata: agent:{type,id} 그룹 (기본, emit_agent 토글로 OFF)',
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
        // P3: agent 그룹은 기본 ON. 토글 OFF 시에만 emit_agent=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
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
    outputDesc: '원본 메시지 clone 패스스루. metadata: agent:{type,id} 그룹 (기본, emit_agent 토글로 OFF) + node_id 추가',
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
        {
          name: 'input_encoding',
          type: 'select',
          label: '입력 인코딩',
          options: ['auto', 'hex', 'text', 'base64'],
          default: 'auto',
          description:
            'data/raw 문자열 페이로드를 바이트로 변환하는 방식. auto: hex 추론(하위호환), hex: 항상 hex 디코딩, text: 평문 그대로, base64: base64 디코딩',
        },
        // P3: agent 그룹은 기본 ON. 토글 OFF 시에만 emit_agent=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
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
    outputDesc: 'payload: {raw: []byte, data: string}. metadata: agent:{type,id} 그룹 (기본, emit_agent 토글로 OFF) + tcp.remote_addr (서버 모드) + tcp.agent_type',
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
        // P3: agent 그룹은 기본 ON. 토글 OFF 시에만 emit_agent=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
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
    outputDesc: '원본 메시지 clone 패스스루. metadata: agent:{type,id} 그룹 (기본, emit_agent 토글로 OFF) + tcp.node_id 추가',
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
        // P3: agent 그룹은 기본 ON. 토글 OFF 시에만 emit_agent=false 가 직렬화되어 백엔드가 비활성화한다 (absent=ON).
        { name: 'emit_agent', type: 'boolean', label: '메타데이터: agent 그룹', default: true, description: '메시지 metadata 에 agent:{type,id} 그룹 포함 (기본 ON)', advanced: true },
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
    description: '키-값 저장소에서 데이터를 조회합니다. read_mode에 따라 현재값 또는 시계열 엔트리 배열을 payload에 기록합니다. 타임스탬프는 epoch ms(int64).',
    inputDesc: 'payload: key_template의 {field} 플레이스홀더 값, 필요 시 from/to/since 동적 참조 필드 (epoch ms)',
    outputDesc: 'payload[output_key]에 [{value, timestamp}, ...] 배열 기록. timestamp는 epoch 밀리초(int64). 최신순, 현재값 포함. 원본 payload 유지',
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
          description: '조회 결과 배열을 저장할 payload 키',
        },
        {
          name: 'read_mode',
          type: 'select',
          label: '조회 모드',
          options: ['latest', 'last_n', 'duration', 'time_range', 'since_n'],
          default: 'latest',
          description: 'latest: 현재값 / last_n: 최신 N개 / duration: 최근 기간 / time_range: 절대 시간 구간 / since_n: 특정 시점부터 N개',
        },
        {
          name: 'count',
          type: 'number',
          label: '개수 (count)',
          description: 'last_n, since_n 모드에서 반환할 엔트리 수 (1 이상)',
          visibleWhen: { field: 'read_mode', value: ['last_n', 'since_n'] },
        },
        {
          name: 'duration',
          type: 'string',
          label: '기간 (duration)',
          description: '예: "5m", "1h", "24h". duration 모드에서 현재부터 역순 조회할 구간',
          visibleWhen: { field: 'read_mode', value: 'duration' },
        },
        {
          name: 'from',
          type: 'string',
          label: '시작 시각 (from)',
          description: 'epoch ms 리터럴(예: "1713225600000") 또는 {payload_field} 동적 참조. RFC3339 문자열도 호환 지원',
          visibleWhen: { field: 'read_mode', value: 'time_range' },
        },
        {
          name: 'to',
          type: 'string',
          label: '끝 시각 (to)',
          description: 'epoch ms 리터럴 또는 {payload_field} 동적 참조. RFC3339 문자열도 호환 지원',
          visibleWhen: { field: 'read_mode', value: 'time_range' },
        },
        {
          name: 'since',
          type: 'string',
          label: '기준 시각 (since)',
          description: 'epoch ms 리터럴 또는 {payload_field} 동적 참조. since_n 모드에서 이 시각 이후 최신순 count 개 반환',
          visibleWhen: { field: 'read_mode', value: 'since_n' },
        },
        {
          name: 'include_metadata',
          type: 'boolean',
          label: '메타데이터 포함',
          default: false,
          description: 'store_count, store_created_at, store_updated_at, store_oldest_at 을 payload 에 추가',
        },
        {
          name: 'entries_field',
          type: 'string',
          label: '배치 입력 필드',
          description: 'payload에서 배열을 추출할 필드명. 지정 시 배열 각 요소별로 키를 해석하여 다중 키를 조회합니다. 결과 map(output_key)의 키는 resolved store 키 사용 (예: "device.room1.temp"). 예: "rooms"',
        },
        {
          name: 'entries_var',
          type: 'string',
          label: '배치 변수명',
          default: 'item',
          description: '배열 각 요소를 매핑할 변수명. primitive 배열은 {item} 으로 참조 (예: "item" → {item}). 객체 배열은 {item.id} 또는 {item.location.zone} 처럼 dot notation 으로 nested 접근.',
          visibleWhen: { field: 'entries_field', notEmpty: true },
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
      { name: 'error', direction: 'error' as const },
    ],
  },

  // --- Inventory (SPEC-INVENTORY-001) ---
  inventory: {
    description:
      '디바이스/에이전트/노드/플로우 인벤토리 스냅샷을 emit. trigger 와 체이닝하여 주기적 상태 동기화에 사용합니다. 4종 source × 2종 emit_shape × 선택적 DeviceFilter 매트릭스를 지원합니다.',
    inputDesc:
      '임의의 트리거 메시지 (페이로드 무시). 보통 trigger 노드의 출력을 입력으로 사용합니다. 입력 metadata 는 출력에 얕은 복사로 보존됩니다.',
    outputDesc:
      'array 모드: payload { source, count, items[] } 의 단일 메시지. per_item 모드: payload 가 단일 item 객체인 N 개 메시지 fan-out. ' +
      'metadata: inventory.source, inventory.count. per_item 모드에서 추가로 inventory.index, inventory.total. ' +
      'devices 항목은 id (composite key, address 역할) 외에 uid (글로벌 UUID, identity 역할, v1.0 1급 키) 를 함께 노출하며, UUID 매핑이 없으면 uid 키는 생략됩니다. ' +
      '시계열 tag 키 / MQTT topic 에는 uid 권장. ' +
      'v0.2.0 호환 alias device_uuid 는 v1.0 (Phase D § D-T18) 에서 제거되었습니다.',
    configSchema: {
      fields: [
        {
          name: 'source',
          type: 'select',
          label: '소스',
          required: true,
          options: ['devices', 'agents', 'nodes', 'flows'],
          description: '스냅샷 대상 인벤토리 종류',
        },
        {
          name: 'emit_shape',
          type: 'select',
          label: 'emit 형태',
          options: ['array', 'per_item'],
          default: 'array',
          description:
            'array: 단일 메시지에 배열을 담음 / per_item: 항목별 N 개 메시지 fan-out (inventory.index/total metadata 포함)',
        },
        {
          name: 'include_metadata',
          type: 'boolean',
          label: '메타데이터 포함',
          default: true,
          description:
            'true: 풍부한 메타데이터(device.metadata/state, agent.info/stats, flow.extra 등) 포함 / false: 핵심 식별 필드만',
        },
        // DeviceFilter — source=devices 한정. JSON 객체 직접 편집.
        // 백엔드 device.DeviceFilter 와 동일 schema (protocol/agent_name/type/online/group/tags).
        // ConfigField 의 'object' 타입은 nested 평면 필드 표현을 지원하지 않으므로 단일 JSON 편집기로 노출.
        {
          name: 'filter',
          type: 'object',
          label: '디바이스 필터 (JSON)',
          description:
            'source=devices 일 때만 적용. DeviceFilter 와 동일 스키마.\n' +
            '예: {"protocol": "lg_icp01", "online": true, "group": "production", "tags": ["critical"]}\n' +
            '지원 필드: protocol (string), agent_name (string), type (string), online (bool), group (string), tags (string array).',
          visibleWhen: { field: 'source', value: 'devices' },
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
      { name: 'out', direction: 'output' as const },
    ],
  },

  // --- Input ---
  trigger: {
    description: '스케줄(주기/cron/1회/매일 시각)에 따라 메시지를 자동으로 생성합니다. 입력이 없는 소스 노드이며, 플로우의 시작점으로 사용합니다.',
    inputDesc: '없음 (소스 노드)',
    outputDesc: 'payload: 설정에 따라 다름 (정적 값, 템플릿, 또는 기본 {trigger_time}). metadata: trigger.schedule_type, trigger.schedule_id, trigger.tick_count, trigger.trigger_time, trigger.node_name',
    configSchema: {
      fields: [
        {
          name: 'schedules',
          type: 'trigger_schedules',
          label: '스케줄',
          required: true,
          description: '1개 이상의 스케줄을 지정합니다. 여러 스케줄이 동시에 실행될 수 있습니다.',
        },
        {
          name: 'payload_mode',
          type: 'select',
          label: '페이로드 모드',
          options: ['none', 'static', 'json', 'template'],
          default: 'none',
          description: 'none: 기본 / static: 키-값 입력 / json: JSON 직접 편집 / template: 변수 치환',
        },
        {
          name: 'payload',
          type: 'typed_key_value_map',
          label: '정적 페이로드',
          description: '키-값 쌍 입력. 값 타입을 string/number/boolean/array/json 중 선택',
          visibleWhen: { field: 'payload_mode', value: 'static' },
        },
        {
          name: 'payload',
          type: 'object',
          label: '페이로드 (JSON)',
          description: 'JSON 형식으로 직접 편집. 예: {"rooms": ["room1", "room2"], "count": 10}',
          visibleWhen: { field: 'payload_mode', value: 'json' },
        },
        {
          name: 'payload_template',
          type: 'key_value_map',
          label: '템플릿 페이로드',
          description: '사용 가능 변수: $.trigger_time, $.tick_count, $.schedule_id, $.trigger_id',
          visibleWhen: { field: 'payload_mode', value: 'template' },
        },
        {
          name: 'source_ch_size',
          type: 'number',
          label: '출력 버퍼 크기',
          default: 64,
          description: '출력 채널의 버퍼 크기. 버퍼가 가득 차면 새 메시지가 드롭됩니다.',
          advanced: true,
        },
      ],
    },
    defaultPorts: [
      { name: 'out', direction: 'output' as const },
    ],
  },

  // --- Output ---
  'chart-emitter': {
    description:
      '입력 메시지를 WebSocket 차트 채널(/ws/chart/{channel_name})로 발행하고 링버퍼에 보관합니다. 대시보드 차트 패널이 이 채널을 구독해 실시간 데이터를 표시합니다. 종단 노드이므로 출력 포트가 없습니다. 단일 메시지(실시간 append) 또는 배열(backfill 용 과거 이력) 모두 지원합니다.',
    inputDesc:
      '두 가지 모드: (1) 단일 엔트리 — payload { timestamp?, value?, labels?, meta? }. (2) 배치 — entries_field 설정 시 payload[entries_field] 의 배열을 개별 엔트리로 분해. 배치는 timestamp 오름차순으로 정렬되어 publish 되므로 FIFO 링버퍼에 최신 항목이 남습니다. store-read(last_n/duration/time_range) 의 배열 출력을 라인/바 차트에 공급할 때 반드시 entries_field 를 설정하세요.',
    outputDesc: '출력 포트 없음 (sink). 링버퍼는 buffer_size 개 FIFO, retention_sec 초 이내만 보관.',
    configSchema: {
      fields: [
        {
          name: 'channel_name',
          type: 'string',
          label: '채널 이름',
          description:
            '단일 채널 모드: 차트 패널이 구독할 고유 채널 이름. 멀티채널 모드(channels_field) 사용 시 불필요.',
        },
        {
          name: 'buffer_size',
          type: 'number',
          label: '버퍼 크기',
          default: 100,
          description: '링버퍼가 보관할 최근 메시지 개수 (1-10000). 신규 구독자는 이 크기만큼 backfill 수신.',
        },
        {
          name: 'retention_sec',
          type: 'number',
          label: '보존 시간 (초)',
          default: 3600,
          description: '링버퍼 항목 최대 보존 시간 (0-86400, 0 이면 시간 기반 만료 비활성).',
        },
        {
          name: 'entries_field',
          type: 'string',
          label: '배치 입력 필드',
          description:
            '단일 채널 모드: payload 내 엔트리 배열 필드명. 배열의 각 element를 개별 차트 엔트리로 발행. 예: "store_value".',
        },
        {
          name: 'channels_field',
          type: 'string',
          label: '멀티채널 입력 필드',
          description:
            '멀티채널 모드: payload 내 map[string]entries 필드명. 각 키별로 별도 채널에 발행. 예: store-read entries_field 출력인 "store_value".',
        },
        {
          name: 'channel_prefix',
          type: 'string',
          label: '채널 접두사',
          description:
            '멀티채널 모드에서 각 키 앞에 붙일 접두사. 예: "temp_" → "temp_room1", "temp_room2".',
          visibleWhen: { field: 'channels_field', notEmpty: true },
        },
      ],
    },
    defaultPorts: [
      { name: 'in', direction: 'input' as const },
    ],
  },

  // --- Composition: flow-node (SPEC-SUBFLOW-001 그룹 C) ---
  // 다른 플로우를 참조하여 합성하는 특수 노드.
  // - flow_id: 참조 플로우 id (flow_picker 로 선택, 현재 편집 중 플로우는 후보에서 제외).
  // - flow_name: 참조 플로우 표시 이름 (denormalize, 노드 카드 표시 전용).
  // - input_ports / output_ports: 참조 플로우의 플로우 레벨 포트 이름 배열.
  //   flow_id 선택 시 참조 플로우 정의를 조회하여 비정규화(denormalize)한 값으로,
  //   핸들 렌더링(computePortsForNode) 에만 사용되는 에디터 표시 전용 캐시이다.
  //   백엔드는 배포 시점에 참조 플로우의 실제 정의에서 포트를 재해석하므로
  //   config 의 input_ports / output_ports 는 무시한다(REQ-SUBFLOW-C03/D04).
  'flow-node': {
    description:
      '다른 플로우를 참조하여 서브플로우로 합성합니다. 참조 플로우의 입출력 포트가 이 노드의 핸들로 표시됩니다. 핸들은 배포 시점에 참조 플로우의 현재 정의로 항상 최신화됩니다.',
    inputDesc: '참조 플로우의 입력 포트로 라우팅됩니다.',
    outputDesc: '참조 플로우의 출력 포트에서 나옵니다.',
    configSchema: {
      fields: [
        {
          name: 'flow_id',
          type: 'flow_picker',
          label: '참조 플로우',
          required: true,
          description:
            '서브플로우로 참조할 플로우를 선택합니다. 현재 편집 중인 플로우는 자기참조 방지를 위해 후보에서 제외됩니다. 선택하면 참조 플로우의 입출력 포트가 이 노드의 핸들로 표시됩니다.',
        },
      ],
    },
    // 초기(미해결) 기본 포트는 없음 — flow_id 선택 후 참조 플로우 포트로 채워진다.
    defaultPorts: [],
  },
};

/**
 * 비정규화된 참조 플로우 포트 이름 배열을 PortDef 로 변환한다.
 * 문자열 배열 안의 비문자열/빈 문자열/중복은 안전하게 걸러낸다.
 */
function flowNodePortDefs(
  names: unknown,
  direction: 'input' | 'output',
): PortDef[] {
  if (!Array.isArray(names)) return [];
  const seen = new Set<string>();
  const ports: PortDef[] = [];
  for (const raw of names) {
    const name = typeof raw === 'string' ? raw.trim() : '';
    if (name === '' || seen.has(name)) continue;
    seen.add(name);
    ports.push({ name, direction });
  }
  return ports;
}

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
    // 백엔드 SwitchNode.Ports() 와 정렬되는 동적 출력 포트 파생(SPEC-SWITCH-001 §5.3):
    //   입력: 항상 'in'.
    //   출력: routes 가 비면 'out'(폴백);
    //         아니면 각 route.name(빈 이름 제외, 중복 제거)
    //         + default_port 가 비어있지 않으면 그 값(중복 제거).
    // 'default' 고정 이름이 아니라 실제 default_port 값을 포트명으로 사용한다.
    const routes = config?.routes as Array<{ name?: unknown }> | undefined;
    const defaultPort =
      typeof config?.default_port === 'string' ? config.default_port.trim() : '';
    const ports: PortDef[] = [{ name: 'in', direction: 'input' }];

    const outputNames: string[] = [];
    const seen = new Set<string>();
    const pushOutput = (name: string) => {
      if (name === '' || seen.has(name)) return;
      seen.add(name);
      outputNames.push(name);
    };

    if (Array.isArray(routes)) {
      for (const r of routes) {
        const name = typeof r?.name === 'string' ? r.name.trim() : '';
        pushOutput(name);
      }
    }

    if (outputNames.length === 0) {
      // routes 가 비었거나 유효한 이름이 없으면 폴백.
      ports.push({ name: 'out', direction: 'output' });
      return ports;
    }

    // default_port 는 설정된 경우에만 출력 포트로 포함(중복 제거).
    pushOutput(defaultPort);

    for (const name of outputNames) {
      ports.push({ name, direction: 'output' });
    }
    return ports;
  }

  if (nodeType === 'flow-node') {
    // SPEC-SUBFLOW-001 REQ-SUBFLOW-C02: flow-node 핸들 = 참조 플로우의 플로우 레벨 포트.
    //   입력 핸들 ← 참조 플로우 inputs, 출력 핸들 ← 참조 플로우 outputs.
    // computePortsForNode 는 동기(sync) 이고 노드 config 만 가지므로, flow_id 선택 시점에
    // 참조 플로우 정의를 조회해 input_ports / output_ports (string[]) 로 비정규화해 둔다.
    // 이 값들은 에디터 표시 전용 캐시이며, 백엔드는 배포 시점에 참조 플로우의 실제
    // 정의에서 포트를 재해석하므로 config 의 input_ports / output_ports 를 무시한다.
    const inputPorts = flowNodePortDefs(config?.input_ports, 'input');
    const outputPorts = flowNodePortDefs(config?.output_ports, 'output');
    // 아직 미해결(flow_id 미선택 또는 포트 0개)이면 핸들 없이 둔다(REQ-SUBFLOW-C05 dangling 방지).
    return [...inputPorts, ...outputPorts];
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

/**
 * 노드 설정에서 누락된 필수 필드를 식별한다.
 *
 * visibleWhen 조건을 고려하여 현재 보이는 필드만 검증한다.
 * 빈 문자열, null, undefined, 빈 배열, 빈 객체를 "값 없음"으로 간주한다.
 *
 * @returns 누락된 필드의 { name, label } 배열. 에러가 없으면 빈 배열
 */
export function getRequiredFieldErrors(
  nodeType: string,
  data: Record<string, unknown>,
  agentType?: string,
): { name: string; label: string }[] {
  const schema = getConfigSchema(nodeType, agentType);
  if (!schema) return [];

  const errors: { name: string; label: string }[] = [];
  for (const field of schema.fields) {
    if (!field.required) continue;

    // visibleWhen 조건에 맞지 않으면 검증 대상에서 제외
    if (field.visibleWhen) {
      const actual = data[field.visibleWhen.field];
      const expected = field.visibleWhen.value;
      const isVisible = Array.isArray(expected)
        ? expected.includes(actual)
        : actual === expected;
      if (!isVisible) continue;
    }

    const value = data[field.name];
    if (isEmptyFieldValue(value)) {
      errors.push({ name: field.name, label: field.label });
    }
  }
  return errors;
}

/** 필드 값이 "없음" 으로 간주되는지 판정한다. */
function isEmptyFieldValue(value: unknown): boolean {
  if (value == null) return true;
  if (typeof value === 'string') return value.trim() === '';
  if (Array.isArray(value)) return value.length === 0;
  if (typeof value === 'object') return Object.keys(value as object).length === 0;
  return false;
}
