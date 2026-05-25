// 빌트인 노드 타입의 상세 메타데이터.
// Go 소스(internal/node/*.go)의 Configure() 메소드에서 추출한 포트, 설정 필드, 예제 정보.

export interface PortMeta {
  name: string;
  direction: 'input' | 'output' | 'error';
  description: string;
}

export interface ConfigFieldMeta {
  name: string;
  type: string;
  required: boolean;
  description: string;
  default?: string;
}

export interface NodeTypeDetailMeta {
  description: string;
  ports: PortMeta[];
  configFields: ConfigFieldMeta[];
  configExample: Record<string, unknown>;
  /** 포트별 입력 메시지 예제 (선택적). 키는 포트 이름(예: "in") 또는 의미 있는 라벨. */
  inputExamples?: Record<string, unknown>;
  /** 포트별 출력 메시지 예제 */
  outputExamples?: Record<string, unknown>;
}

export const NODE_TYPE_META: Record<string, NodeTypeDetailMeta> = {
  deduplicate: {
    description:
      '시간 창(window) 내에서 동일한 메시지를 제거합니다. key 필드로 메시지를 그룹핑하고, 비교 필드가 모두 동일하면 중복으로 판정하여 폐기합니다. window 시간이 초과되면 동일 값이어도 강제 통과합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '중복 검사할 메시지 입력' },
      { name: 'out', direction: 'output', description: '중복이 아닌 메시지 출력' },
      { name: 'reject', direction: 'output', description: '중복 메시지 출력 (on_duplicate=reject_port 시, 연결 없으면 폐기)' },
      { name: 'error', direction: 'error', description: '처리 중 에러 발생 시 출력' },
    ],
    configFields: [
      { name: 'key', type: 'string', required: false, description: '메시지 그룹핑 키 필드명 (예: idu_num). 비어있으면 전체 메시지 기준' },
      { name: 'window', type: 'string', required: false, description: '중복 억제 시간 창 (예: 30s, 1m)', default: '30s' },
      { name: 'compare_fields', type: 'string', required: false, description: '비교 대상 필드 (콤마 구분). 비어있으면 전체 페이로드 비교 (timestamp/seq/raw_hex 제외)' },
      { name: 'on_duplicate', type: 'string', required: false, description: '중복 시 처리: drop (기본, 폐기) 또는 reject_port (reject 포트로 전달)', default: 'drop' },
    ],
    configExample: {
      key: 'idu_num',
      window: '30s',
      compare_fields: 'current_temperature,target_temperature,op_mode,fan_byte',
      on_duplicate: 'drop',
    },
  },

  filter: {
    description:
      '조건식을 평가하여 메시지를 필터링합니다. 조건이 true이면 out 포트로, false이면 reject 포트로 전달됩니다. reject 포트에 연결이 없으면 메시지가 폐기됩니다.',
    ports: [
      { name: 'in', direction: 'input', description: '필터링할 메시지 입력' },
      { name: 'out', direction: 'output', description: '조건을 통과한 메시지 출력' },
      { name: 'reject', direction: 'output', description: '조건 불일치 메시지 출력 (연결 없으면 폐기)' },
      { name: 'error', direction: 'error', description: '처리 중 에러 발생 시 출력 (on_reject=error_port 시 거부 메시지도 전달)' },
    ],
    configFields: [
      {
        name: 'condition',
        type: 'string',
        required: false,
        description: '필터 조건식. 미설정 시 모든 메시지를 통과시킵니다 (pass-through).',
      },
      {
        name: 'on_reject',
        type: 'string',
        required: false,
        description: '거부 시 처리: reject_port (기본, reject 포트로) 또는 error_port (에러 포트로)',
        default: 'reject_port',
      },
    ],
    configExample: {
      condition: 'payload.level == "error"',
    },
    outputExamples: {
      out: {
        _comment: '조건 통과 시 원본 메시지 그대로 출력',
        payload: { level: 'error', message: 'connection timeout' },
      },
    },
  },

  transform: {
    description:
      '메시지 데이터를 변환합니다. JSONPath 표현식으로 필드를 선택(select)하거나 매핑(map)할 수 있으며, null 값 제거 옵션을 지원합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '변환할 메시지 입력' },
      { name: 'out', direction: 'output', description: '변환된 메시지 출력' },
    ],
    configFields: [
      {
        name: 'expression',
        type: 'string',
        required: true,
        description: '변환 표현식 (JSONPath 또는 매핑 규칙)',
      },
      {
        name: 'mode',
        type: 'string',
        required: false,
        description: '변환 모드',
        default: 'select',
      },
      {
        name: 'strip_nulls',
        type: 'boolean',
        required: false,
        description: '결과에서 null 값 제거 여부',
        default: 'false',
      },
    ],
    configExample: {
      expression: '$.data.temperature',
      mode: 'select',
      strip_nulls: true,
    },
    outputExamples: {
      out: {
        _comment: 'select 모드: 지정 경로의 값만 추출',
        payload: { temperature: 25.5 },
      },
    },
  },

  switch: {
    description:
      '조건에 따라 메시지를 서로 다른 출력 포트로 라우팅합니다. 라우트를 순서대로 평가하여 첫 번째 매칭되는 포트로 전달합니다. 매칭 없으면 기본 포트 또는 드롭됩니다.',
    ports: [
      { name: 'in', direction: 'input', description: '라우팅할 메시지 입력' },
      { name: 'out', direction: 'output', description: '기본 출력 포트 (라우트별 동적 포트 가능)' },
    ],
    configFields: [
      {
        name: 'routes',
        type: 'array',
        required: false,
        description: '라우팅 규칙 목록. 각 규칙에 condition과 target_port를 지정합니다.',
      },
      {
        name: 'default_port',
        type: 'string',
        required: false,
        description: '매칭되는 라우트가 없을 때 사용할 기본 포트 이름',
      },
    ],
    configExample: {
      routes: [
        { condition: 'payload.type == "alert"', target_port: 'alerts' },
        { condition: 'payload.type == "metric"', target_port: 'metrics' },
      ],
      default_port: 'other',
    },
  },

  bridge: {
    description:
      '외부 에이전트와 메시지를 송수신하는 브릿지 노드입니다. direction에 따라 포트 구성이 달라지며, 에이전트를 통해 외부 시스템과 통합합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '에이전트로 전송할 메시지 입력 (out/inout 모드)' },
      { name: 'out', direction: 'output', description: '에이전트에서 수신한 메시지 출력 (in/inout 모드)' },
    ],
    configFields: [
      {
        name: 'payload_format',
        type: 'string',
        required: false,
        description: '메시지 페이로드 직렬화 형식',
        default: 'json',
      },
      {
        name: 'topics',
        type: 'string[]',
        required: false,
        description: '에이전트에 자동 구독 요청할 토픽 목록',
      },
      {
        name: 'polling_interval_ms',
        type: 'number',
        required: false,
        description: '폴링 간격 (밀리초)',
      },
    ],
    configExample: {
      payload_format: 'json',
      topics: ['sensor/temperature', 'sensor/humidity'],
      polling_interval_ms: 1000,
    },
  },

  script: {
    description:
      '스크립트를 실행하여 메시지를 처리합니다. JavaScript 스크립트 엔진을 사용하며, 커스텀 변환이나 복잡한 로직을 구현할 수 있습니다.',
    ports: [
      { name: 'in', direction: 'input', description: '스크립트로 처리할 메시지 입력' },
      { name: 'out', direction: 'output', description: '스크립트 처리 결과 메시지 출력' },
    ],
    configFields: [
      {
        name: 'script',
        type: 'string',
        required: true,
        description: '실행할 스크립트 소스 코드',
      },
    ],
    configExample: {
      script: 'return { ...msg, payload: { ...msg.payload, processed: true } }',
    },
  },

  catch: {
    description:
      '에러 메시지를 캐치하여 처리합니다. 특정 에러 타입만 필터링하거나, 모든 에러를 캐치할 수 있습니다.',
    ports: [
      { name: 'in', direction: 'input', description: '에러 메시지 입력' },
      { name: 'out', direction: 'output', description: '캐치된 에러 메시지 출력' },
    ],
    configFields: [
      {
        name: 'catch_types',
        type: 'string[]',
        required: false,
        description: '캐치할 에러 타입 목록. 미설정 시 모든 에러를 캐치합니다.',
      },
    ],
    configExample: {
      catch_types: ['timeout', 'validation'],
    },
  },

  aggregate: {
    description:
      '여러 메시지를 윈도우 기반으로 집계합니다. 시간/카운트 윈도우를 지원하며, sum/avg/count/min/max 등의 집계 함수를 사용할 수 있습니다.',
    ports: [
      { name: 'in', direction: 'input', description: '집계할 메시지 입력' },
      { name: 'out', direction: 'output', description: '집계 결과 메시지 출력' },
    ],
    configFields: [
      {
        name: 'window_type',
        type: 'string',
        required: false,
        description: '윈도우 타입 (tumbling, sliding, count)',
        default: 'tumbling',
      },
      {
        name: 'window_size',
        type: 'string',
        required: true,
        description: '윈도우 크기 (예: "10s", "1m", "100")',
      },
      {
        name: 'aggregate_fn',
        type: 'string | string[]',
        required: false,
        description: '집계 함수 (sum, avg, count, min, max)',
        default: 'count',
      },
      {
        name: 'fields',
        type: 'string[]',
        required: false,
        description: '집계 대상 필드 목록',
        default: 'value',
      },
      {
        name: 'group_by',
        type: 'string | string[]',
        required: false,
        description: '그룹핑 키 필드',
      },
      {
        name: 'max_groups',
        type: 'number',
        required: false,
        description: '최대 그룹 수 제한',
      },
    ],
    configExample: {
      window_type: 'tumbling',
      window_size: '30s',
      aggregate_fn: ['avg', 'max'],
      fields: ['temperature', 'humidity'],
      group_by: 'sensor_id',
    },
  },

  output: {
    description:
      '메시지를 포맷팅하여 출력합니다. 출력 대상(터미널/파일/에디터/로거)을 선택하고, 출력 필드와 형식을 지정할 수 있습니다. 메시지는 그대로 다음 노드로 전달됩니다 (pass-through).',
    ports: [
      { name: 'in', direction: 'input', description: '출력할 메시지 입력. property로 특정 경로 지정 가능 (.payload, .payload.name, .metadata, .id)' },
      { name: 'out', direction: 'output', description: '원본 메시지를 그대로 전달 (pass-through)' },
    ],
    configFields: [
      {
        name: 'level',
        type: 'string',
        required: false,
        description: '로그 레벨 (debug, info, warn)',
        default: 'debug',
      },
      {
        name: 'output',
        type: 'string',
        required: false,
        description: '출력 대상 (slog: 서버 로그, terminal: stdout, file: 파일, editor: 에디터 패널, logger: 에이전트 로거)',
        default: 'slog',
      },
      {
        name: 'property',
        type: 'string',
        required: false,
        description: '출력할 메시지 경로 (예: .payload, .payload.raw, .metadata, .id). 미지정 시 메시지 전체',
      },
      {
        name: 'format',
        type: 'string',
        required: false,
        description: '출력 형식. json: JSON 포맷, plain: 텍스트 (바이너리→hex), raw: 바이너리 그대로',
        default: 'json',
      },
      {
        name: 'display_fields',
        type: 'string',
        required: false,
        description: '표시 항목 (쉼표 구분). time, level, name, payload, metadata, id 및 payload 내 키. 미지정 시 기본: 시간 레벨 이름 메시지값',
      },
      {
        name: 'prefix',
        type: 'string',
        required: false,
        description: '출력 접두어. 미지정 시 노드 이름 사용',
      },
    ],
    configExample: {
      output: 'terminal',
      property: '.payload.raw',
      format: 'plain',
      display_fields: 'time,level,name,payload',
      prefix: '[sensor]',
    },
  },

  mapping: {
    description:
      '메시지 필드 값을 키로 사용하여 매핑 테이블에서 대응하는 값을 조회합니다. 센서 코드를 이름으로 변환하거나, 상태 코드를 메시지로 변환하는 등의 룩업 패턴을 지원합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '매핑할 메시지 입력' },
      { name: 'out', direction: 'output', description: '매핑 결과 메시지 출력' },
      { name: 'error', direction: 'error', description: '키 미발견/필드 미발견 에러 시 출력' },
    ],
    configFields: [
      {
        name: 'field',
        type: 'string',
        required: true,
        description: '매핑할 소스 필드 JSONPath (예: $.payload.status_code)',
      },
      {
        name: 'mappings',
        type: 'json',
        required: true,
        description: '키-값 매핑 테이블 (JSON 객체). 키는 문자열, 값은 모든 JSON 호환 타입 가능.',
      },
      {
        name: 'default',
        type: 'string',
        required: false,
        description: '매핑 키가 없을 때 사용할 기본값. 미설정 시 에러를 반환합니다.',
      },
      {
        name: 'target',
        type: 'string',
        required: false,
        description: '결과를 기록할 필드명. 미지정 시 소스 필드를 덮어씁니다.',
      },
    ],
    configExample: {
      field: '$.payload.status_code',
      mappings: {
        '0': '정상',
        '1': '경고',
        '2': '위험',
        '3': '긴급',
      },
      default: '알 수 없음',
      target: 'status_text',
    },
  },

  deadletter: {
    description:
      '처리에 실패한 메시지를 보관하는 데드레터 노드입니다. 실패 원인과 함께 메시지를 저장하여 나중에 재처리하거나 분석할 수 있습니다.',
    ports: [
      { name: 'in', direction: 'input', description: '실패한 메시지 입력' },
      { name: 'out', direction: 'output', description: '보관 처리된 메시지 출력' },
    ],
    configFields: [
      {
        name: 'strategy',
        type: 'string',
        required: false,
        description: '데드레터 처리 전략 (store, log, forward)',
        default: 'store',
      },
    ],
    configExample: {
      strategy: 'store',
    },
  },

  'nasa-status': {
    description:
      'Samsung NASA 에이전트에 연결하여 HVAC 디바이스 상태를 조회하는 노드입니다. device_id를 지정하면 해당 디바이스만, 미지정 시 전체 디바이스 상태를 조회합니다. poll_interval 설정 시 SourceNode로서 주기적 자동 폴링을 수행합니다. 모든 설정값(device_id)은 입력 메시지 payload로 런타임 오버라이드할 수 있습니다.',
    ports: [
      { name: 'in', direction: 'input', description: '상태 조회를 트리거하는 메시지를 수신합니다. payload에 device_id가 있으면 노드 설정을 오버라이드합니다.' },
      { name: 'out', direction: 'output', description: '디바이스 상태 조회 결과를 출력합니다. get_state 또는 get_all 응답이 포함됩니다.' },
      { name: 'error', direction: 'error', description: '에이전트 통신 실패, 타임아웃 등 에러 발생 시 출력합니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '대상 Samsung NASA 에이전트의 이름 또는 ID입니다.',
      },
      {
        name: 'device_id',
        type: 'string',
        required: false,
        description: '조회할 디바이스 ID입니다 (예: "living-room"). 미지정 시 get_all 로 전체 디바이스를 조회합니다.',
      },
      {
        name: 'poll_interval',
        type: 'string',
        required: false,
        description: '자동 폴링 주기입니다 (예: "10s", "1m"). 설정 시 SourceNode로서 주기적으로 상태를 조회합니다.',
        default: '30s',
      },
      {
        name: 'timeout',
        type: 'string',
        required: false,
        description: 'Agent Process() 호출 타임아웃입니다.',
        default: '5s',
      },
    ],
    configExample: {
      agent_ref: 'samsung-nasa-agent',
      device_id: 'living-room',
      poll_interval: '10s',
      timeout: '5s',
    },
  },

  'nasa-control': {
    description:
      'Samsung NASA 에이전트에 제어 명령을 전송하는 노드입니다. 직접 명령 형식(command 키 포함)과 간편 형식(power, mode 등 제어 키)을 모두 지원합니다. 간편 형식은 자동으로 set_multiple 명령으로 변환됩니다. 모든 설정값(device_id)은 입력 메시지 payload로 런타임 오버라이드할 수 있습니다.',
    ports: [
      { name: 'in', direction: 'input', description: '제어 명령 메시지를 수신합니다. 직접 명령 형식 또는 간편 형식 모두 가능합니다. payload에 device_id가 있으면 노드 설정을 오버라이드합니다.' },
      { name: 'out', direction: 'output', description: '제어 명령 실행 결과를 출력합니다.' },
      { name: 'error', direction: 'error', description: '에이전트 통신 실패, 타임아웃 등 에러 발생 시 출력합니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '대상 Samsung NASA 에이전트의 이름 또는 ID입니다.',
      },
      {
        name: 'device_id',
        type: 'string',
        required: false,
        description: '기본 대상 디바이스 ID입니다. 입력 메시지 payload의 device_id로 오버라이드 가능합니다.',
      },
      {
        name: 'timeout',
        type: 'string',
        required: false,
        description: 'Agent Process() 호출 타임아웃입니다.',
        default: '5s',
      },
    ],
    configExample: {
      agent_ref: 'samsung-nasa-agent',
      device_id: 'living-room',
      timeout: '5s',
    },
  },

  nasa: {
    description:
      'Samsung NASA 에이전트의 상태 조회와 제어를 하나의 노드에서 처리하는 복합 노드입니다. 입력 메시지의 페이로드를 분석하여 자동으로 상태 조회 또는 제어 명령을 판별합니다. 제어 키(power, mode, temperature, target_temperature, fan_speed)가 포함되면 제어, 그 외에는 상태 조회로 동작합니다. poll_interval 설정 시 SourceNode로서 주기적 상태 폴링도 수행합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '상태 조회 또는 제어 명령 메시지를 수신합니다. 제어 키 유무에 따라 자동 분기됩니다.' },
      { name: 'out', direction: 'output', description: '상태 조회 결과 또는 제어 실행 결과를 출력합니다.' },
      { name: 'error', direction: 'error', description: '에이전트 통신 실패, 타임아웃 등 에러 발생 시 출력합니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '대상 Samsung NASA 에이전트의 이름 또는 ID입니다.',
      },
      {
        name: 'device_id',
        type: 'string',
        required: false,
        description: '기본 대상 디바이스 ID입니다. 입력 메시지 payload의 device_id로 오버라이드 가능합니다.',
      },
      {
        name: 'poll_interval',
        type: 'string',
        required: false,
        description: '자동 폴링 주기입니다 (예: "15s", "1m"). 설정 시 SourceNode로서 주기적으로 상태를 조회합니다.',
        default: '30s',
      },
      {
        name: 'timeout',
        type: 'string',
        required: false,
        description: 'Agent Process() 호출 타임아웃입니다.',
        default: '5s',
      },
    ],
    configExample: {
      agent_ref: 'samsung-nasa-agent',
      device_id: 'living-room',
      poll_interval: '15s',
      timeout: '5s',
    },
  },

  'lgap-status': {
    description:
      'LG LGAP 에이전트에 연결하여 HVAC 디바이스 상태를 조회하는 노드입니다. device_id를 지정하면 해당 디바이스만, 미지정 시 전체 디바이스 상태를 조회합니다. poll_interval 설정 시 SourceNode로서 주기적 자동 폴링을 수행합니다. 모든 설정값(device_id)은 입력 메시지 payload로 런타임 오버라이드할 수 있습니다.',
    ports: [
      { name: 'in', direction: 'input', description: '상태 조회를 트리거하는 메시지를 수신합니다. payload에 device_id가 있으면 노드 설정을 오버라이드합니다.' },
      { name: 'out', direction: 'output', description: '디바이스 상태 조회 결과를 출력합니다. get_state 또는 get_all 응답이 포함됩니다.' },
      { name: 'error', direction: 'error', description: '에이전트 통신 실패, 타임아웃 등 에러 발생 시 출력합니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '대상 LG LGAP 에이전트의 이름 또는 ID입니다.',
      },
      {
        name: 'device_id',
        type: 'string',
        required: false,
        description: '조회할 디바이스 ID입니다 (예: "0x11"). 미지정 시 get_all 로 전체 디바이스를 조회합니다.',
      },
      {
        name: 'poll_interval',
        type: 'string',
        required: false,
        description: '자동 폴링 주기입니다 (예: "10s", "1m"). 설정 시 SourceNode로서 주기적으로 상태를 조회합니다.',
        default: '30s',
      },
      {
        name: 'timeout',
        type: 'string',
        required: false,
        description: 'Agent Process() 호출 타임아웃입니다.',
        default: '5s',
      },
    ],
    configExample: {
      agent_ref: 'lgap-agent',
      device_id: '0x11',
      poll_interval: '10s',
      timeout: '5s',
    },
  },

  'lgap-control': {
    description:
      'LG LGAP 에이전트에 제어 명령을 전송하는 노드입니다. 직접 명령 형식(command 키 포함)과 간편 형식(power, mode, temperature 등 제어 키)을 모두 지원합니다. 간편 형식은 자동으로 set_multiple 명령으로 변환됩니다. 모든 설정값(device_id)은 입력 메시지 payload로 런타임 오버라이드할 수 있습니다.',
    ports: [
      { name: 'in', direction: 'input', description: '제어 명령 메시지를 수신합니다. 직접 명령 형식 또는 간편 형식 모두 가능합니다. payload에 device_id가 있으면 노드 설정을 오버라이드합니다.' },
      { name: 'out', direction: 'output', description: '제어 명령 실행 결과를 출력합니다.' },
      { name: 'error', direction: 'error', description: '에이전트 통신 실패, 타임아웃 등 에러 발생 시 출력합니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '대상 LG LGAP 에이전트의 이름 또는 ID입니다.',
      },
      {
        name: 'device_id',
        type: 'string',
        required: false,
        description: '기본 대상 디바이스 ID입니다. 입력 메시지 payload의 device_id로 오버라이드 가능합니다.',
      },
      {
        name: 'timeout',
        type: 'string',
        required: false,
        description: 'Agent Process() 호출 타임아웃입니다.',
        default: '5s',
      },
    ],
    configExample: {
      agent_ref: 'lgap-agent',
      device_id: '0x11',
      timeout: '5s',
    },
  },

  lgap: {
    description:
      'LG LGAP 에이전트의 상태 조회와 제어를 하나의 노드에서 처리하는 복합 노드입니다. 입력 메시지의 페이로드를 분석하여 자동으로 상태 조회 또는 제어 명령을 판별합니다. 제어 키(power, mode, temperature, target_temperature, fan_speed)가 포함되면 제어, 그 외에는 상태 조회로 동작합니다. poll_interval 설정 시 SourceNode로서 주기적 상태 폴링도 수행합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '상태 조회 또는 제어 명령 메시지를 수신합니다. 제어 키 유무에 따라 자동 분기됩니다.' },
      { name: 'out', direction: 'output', description: '상태 조회 결과 또는 제어 실행 결과를 출력합니다.' },
      { name: 'error', direction: 'error', description: '에이전트 통신 실패, 타임아웃 등 에러 발생 시 출력합니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '대상 LG LGAP 에이전트의 이름 또는 ID입니다.',
      },
      {
        name: 'device_id',
        type: 'string',
        required: false,
        description: '기본 대상 디바이스 ID입니다. 입력 메시지 payload의 device_id로 오버라이드 가능합니다.',
      },
      {
        name: 'poll_interval',
        type: 'string',
        required: false,
        description: '자동 폴링 주기입니다 (예: "15s", "1m"). 설정 시 SourceNode로서 주기적으로 상태를 조회합니다.',
        default: '30s',
      },
      {
        name: 'timeout',
        type: 'string',
        required: false,
        description: 'Agent Process() 호출 타임아웃입니다.',
        default: '5s',
      },
    ],
    configExample: {
      agent_ref: 'lgap-agent',
      device_id: '0x11',
      poll_interval: '15s',
      timeout: '5s',
    },
  },

  modbus: {
    description:
      'MODBUS 에이전트(Server/Client)에 연결하여 레지스터를 읽거나 쓰는 처리 노드입니다. 입력 메시지가 도착하면 설정된 연산(읽기/쓰기)을 수행하고 결과를 출력합니다. 모든 설정값(operation, register_area, address, count, data_type, byte_order, device_id)은 입력 메시지 payload로 런타임 오버라이드할 수 있습니다. 노드 config은 기본값이며, 메시지에 동일 키가 있으면 해당 값이 우선 적용됩니다.',
    ports: [
      { name: 'input', direction: 'input', description: '읽기/쓰기 연산을 트리거하는 메시지를 수신합니다. 쓰기 시 payload에 value 또는 values 키가 필요합니다. payload에 operation, register_area, address, count, data_type, byte_order, device_id 키가 있으면 노드 설정을 오버라이드합니다.' },
      { name: 'output', direction: 'output', description: '읽기 결과 또는 쓰기 확인 메시지를 출력합니다. 원본 payload가 보존됩니다.' },
      { name: 'error', direction: 'error', description: '에이전트 통신 실패, timeout, 잘못된 설정 등 에러 발생 시 에러 메시지를 출력합니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '대상 MODBUS 에이전트의 이름 또는 ID입니다. Server Agent와 Client Agent를 모두 지원합니다.',
      },
      {
        name: 'operation',
        type: 'string',
        required: true,
        description: '수행할 연산입니다. "read"는 레지스터를 읽고, "write"는 레지스터에 값을 씁니다. 입력 메시지 payload의 operation으로 오버라이드 가능합니다.',
      },
      {
        name: 'register_area',
        type: 'string',
        required: true,
        description: 'MODBUS 레지스터 영역입니다. coils, discrete_inputs, holding_registers, input_registers를 지원합니다. 입력 메시지 payload의 register_area로 오버라이드 가능합니다.',
      },
      {
        name: 'address',
        type: 'number',
        required: true,
        description: '시작 레지스터 주소입니다 (0-65535). 입력 메시지 payload의 address로 오버라이드 가능합니다.',
      },
      {
        name: 'count',
        type: 'number',
        required: false,
        description: '읽기/쓰기할 레지스터 수입니다. 입력 메시지 payload의 count로 오버라이드 가능합니다.',
        default: '1',
      },
      {
        name: 'data_type',
        type: 'string',
        required: false,
        description: '레지스터 데이터 타입입니다. Holding/Input Registers에만 적용됩니다. 지원: uint16, int16, float32, uint32, int32. 입력 메시지 payload의 data_type으로 오버라이드 가능합니다.',
        default: 'uint16',
      },
      {
        name: 'byte_order',
        type: 'string',
        required: false,
        description: '다중 레지스터 타입(float32, uint32, int32)의 바이트 순서입니다. 입력 메시지 payload의 byte_order로 오버라이드 가능합니다.',
        default: 'big_endian',
      },
      {
        name: 'device_id',
        type: 'number',
        required: false,
        description: 'MODBUS Client 에이전트 전용 대상 디바이스 ID입니다. 입력 메시지 payload의 device_id로 오버라이드 가능합니다.',
        default: '1',
      },
    ],
    configExample: {
      agent_ref: 'modbus-server-1',
      operation: 'read',
      register_area: 'holding_registers',
      address: 100,
      count: 10,
      data_type: 'float32',
      byte_order: 'big_endian',
      device_id: 1,
    },
  },

  'modbus-poller': {
    description:
      'MODBUS 에이전트(Server/Client)에 연결하여 register_map에 정의된 레지스터를 주기적으로 폴링하는 SourceNode입니다. poll_interval 주기로 자동 읽기를 수행하며, 각 레지스터 항목별로 영역, 주소, 데이터 타입, 디바이스 ID를 개별 지정할 수 있습니다. in 포트로 메시지를 보내면 device_id, poll_interval, register_map을 런타임에 동적으로 변경할 수 있습니다.',
    ports: [
      { name: 'in', direction: 'input', description: '설정 변경 메시지를 수신합니다. payload에 device_id, poll_interval, register_map을 포함하면 폴링 설정이 동적으로 변경됩니다.' },
      { name: 'out', direction: 'output', description: '폴링 읽기 결과를 출력합니다. register_map의 이름별 키로 값이 포함됩니다.' },
      { name: 'error', direction: 'error', description: '에이전트 통신 실패, 타임아웃 등 에러 발생 시 출력합니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '대상 MODBUS 에이전트의 이름 또는 ID입니다.',
      },
      {
        name: 'device_id',
        type: 'number',
        required: false,
        description: '기본 디바이스 ID입니다. register_map 항목에서 개별 지정하지 않으면 이 값이 사용됩니다.',
        default: '1',
      },
      {
        name: 'poll_interval',
        type: 'string',
        required: false,
        description: '폴링 주기입니다 (예: "1s", "5s", "1m"). 최소 100ms.',
        default: '5s',
      },
      {
        name: 'register_map',
        type: 'json',
        required: true,
        description: '폴링할 레지스터 정의입니다. 각 항목에 name, register_area, address, count, data_type, byte_order, device_id를 지정합니다.',
      },
    ],
    configExample: {
      agent_ref: 'modbus-server-1',
      poll_interval: '5s',
      register_map: [
        { name: 'temperature', register_area: 'input_registers', address: 0, count: 2, data_type: 'float32' },
        { name: 'humidity', register_area: 'input_registers', address: 2, count: 2, data_type: 'float32' },
        { name: 'battery', register_area: 'input_registers', address: 6, count: 1, data_type: 'uint16' },
      ],
    },
  },

  'modbus-writer': {
    description:
      'MODBUS 에이전트(Server/Client)에 연결하여 레지스터에 값을 쓰는 전용 ProcessNode입니다. 쓰기 가능 영역(coils, holding_registers)만 허용합니다. 입력 메시지의 payload에서 value 또는 values를 추출하여 레지스터에 씁니다. address, data_type, byte_order, device_id, register_area는 입력 메시지 payload로 런타임 오버라이드할 수 있습니다.',
    ports: [
      { name: 'in', direction: 'input', description: '쓰기 연산을 트리거하는 메시지를 수신합니다. payload에 value 또는 values 키가 필요합니다.' },
      { name: 'out', direction: 'output', description: '쓰기 완료 후 결과 메시지를 출력합니다. success, register_area, address, data_type 등이 포함됩니다.' },
      { name: 'error', direction: 'error', description: '에이전트 통신 실패 등 에러 발생 시 출력합니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '대상 MODBUS 에이전트의 이름 또는 ID입니다.',
      },
      {
        name: 'register_area',
        type: 'string',
        required: false,
        description: '쓰기 가능 레지스터 영역입니다. coils, holding_registers만 허용합니다.',
        default: 'holding_registers',
      },
      {
        name: 'address',
        type: 'number',
        required: false,
        description: '시작 레지스터 주소입니다 (0-65535).',
        default: '0',
      },
      {
        name: 'data_type',
        type: 'string',
        required: false,
        description: '레지스터 데이터 타입입니다. uint16, int16, float32, uint32, int32을 지원합니다.',
        default: 'uint16',
      },
      {
        name: 'byte_order',
        type: 'string',
        required: false,
        description: '다중 레지스터 타입의 바이트 순서입니다.',
        default: 'big_endian',
      },
      {
        name: 'device_id',
        type: 'number',
        required: false,
        description: 'MODBUS Client 에이전트 전용 대상 디바이스 ID입니다.',
        default: '1',
      },
    ],
    configExample: {
      agent_ref: 'modbus-server-1',
      register_area: 'holding_registers',
      address: 100,
      data_type: 'float32',
    },
  },

  'tsdb-write': {
    description:
      'TSDB 에이전트에 시계열 데이터를 기록하는 노드입니다. 입력 메시지의 페이로드에서 tag_mappings과 field_mappings에 따라 태그와 필드를 추출하여 measurement에 기록합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '기록할 메시지 입력. 페이로드에서 매핑에 따라 태그/필드를 추출합니다.' },
      { name: 'out', direction: 'output', description: '기록 완료 후 원본 메시지를 passthrough로 출력합니다.' },
      { name: 'error', direction: 'error', description: 'TSDB 기록 실패 시 에러 메시지를 출력합니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '연결할 TSDB 에이전트의 이름 또는 ID입니다.',
      },
      {
        name: 'measurement',
        type: 'string',
        required: true,
        description: '기록할 measurement 이름입니다.',
      },
      {
        name: 'measurement_key',
        type: 'string',
        required: false,
        description: '페이로드에서 measurement 이름을 동적으로 가져올 키입니다.',
      },
      {
        name: 'tag_mappings',
        type: 'key_value_map',
        required: false,
        description: '페이로드 필드를 TSDB 태그로 매핑합니다. 키: 태그 이름, 값: 페이로드 필드 경로.',
      },
      {
        name: 'field_mappings',
        type: 'key_value_map',
        required: false,
        description: '페이로드 필드를 TSDB 필드로 매핑합니다. 키: 필드 이름, 값: 페이로드 필드 경로.',
      },
    ],
    configExample: {
      agent_ref: 'tsdb-engine',
      measurement: 'sensor_data',
      tag_mappings: { sensor_id: 'id', location: 'location' },
      field_mappings: { temperature: 'temp', humidity: 'humidity' },
    },
  },

  'tsdb-query': {
    description:
      'TSDB 에이전트에서 시계열 데이터를 조회하는 노드입니다. measurement, 태그 필터, 시간 범위, 집계 함수 등을 설정하여 데이터를 쿼리합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '쿼리를 트리거하는 메시지 입력. 페이로드로 쿼리 파라미터를 오버라이드할 수 있습니다.' },
      { name: 'out', direction: 'output', description: '쿼리 결과를 출력합니다.' },
      { name: 'error', direction: 'error', description: '쿼리 실패 시 에러 메시지를 출력합니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '연결할 TSDB 에이전트의 이름 또는 ID입니다.',
      },
      {
        name: 'measurement',
        type: 'string',
        required: true,
        description: '조회할 measurement 이름입니다.',
      },
      {
        name: 'series_key',
        type: 'string',
        required: false,
        description: '조회할 시리즈 키입니다. 미지정 시 measurement로 자동 구성됩니다.',
      },
      {
        name: 'tags',
        type: 'key_value_map',
        required: false,
        description: '태그 필터입니다. 키: 태그 이름, 값: 필터 값.',
      },
      {
        name: 'time_range',
        type: 'string',
        required: false,
        description: '조회 시간 범위입니다 (예: "1h", "30m", "24h").',
        default: '1h',
      },
      {
        name: 'aggregation',
        type: 'string',
        required: false,
        description: '집계 함수입니다 (avg, sum, min, max, count, last).',
      },
      {
        name: 'field',
        type: 'string',
        required: false,
        description: '집계 대상 필드입니다.',
      },
      {
        name: 'bucket',
        type: 'string',
        required: false,
        description: '집계 버킷 크기입니다 (예: "1m", "5m", "1h").',
      },
      {
        name: 'limit',
        type: 'number',
        required: false,
        description: '최대 반환 포인트 수입니다.',
      },
    ],
    configExample: {
      agent_ref: 'tsdb-engine',
      measurement: 'sensor_data',
      tags: { sensor_id: 'sensor-001' },
      time_range: '1h',
      aggregation: 'avg',
      field: 'temperature',
      bucket: '5m',
    },
  },

  'influxdb-write': {
    description:
      'InfluxDB에 시계열 데이터를 기록하는 노드입니다. payload에서 measurement, tags, fields를 추출하여 InfluxDB 에이전트의 쓰기 API를 호출합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '기록할 데이터가 담긴 메시지 입력' },
      { name: 'out', direction: 'output', description: '기록 성공 후 원본 메시지 pass-through' },
      { name: 'error', direction: 'error', description: '기록 실패 시 에러 출력' },
    ],
    configFields: [
      { name: 'agent_ref', type: 'string', required: true, description: 'InfluxDB 에이전트 이름 또는 ID' },
      { name: 'measurement', type: 'string', required: false, description: '고정 measurement 이름' },
      { name: 'measurement_key', type: 'string', required: false, description: 'payload에서 measurement를 추출할 키' },
      { name: 'tag_mappings', type: 'string', required: false, description: '태그 매핑 (tag_name: payload_key)' },
      { name: 'field_mappings', type: 'string', required: false, description: '필드 매핑 (field_name: payload_key). 비어있으면 전체 payload' },
      { name: 'timestamp_key', type: 'string', required: false, description: '타임스탬프 추출 키 (Unix ms)' },
    ],
    configExample: {
      agent_ref: 'my-influxdb',
      measurement: 'temperature',
      tag_mappings: { location: 'room' },
      field_mappings: { value: 'temp_celsius' },
    },
  },

  'influxdb-read': {
    description:
      'InfluxDB를 주기적으로 쿼리하여 결과를 개별 메시지로 출력하는 SourceNode입니다. Flux, SQL, InfluxQL 쿼리를 지원합니다.',
    ports: [
      { name: 'out', direction: 'output', description: '쿼리 결과의 각 행이 개별 메시지로 출력' },
      { name: 'error', direction: 'error', description: '쿼리 실패 시 에러 출력' },
    ],
    configFields: [
      { name: 'agent_ref', type: 'string', required: true, description: 'InfluxDB 에이전트 이름 또는 ID' },
      { name: 'query', type: 'string', required: true, description: '실행할 쿼리 (Flux/SQL/InfluxQL)' },
      { name: 'language', type: 'string', required: false, description: '쿼리 언어 (flux/sql/influxql)', default: 'flux' },
      { name: 'poll_interval', type: 'string', required: false, description: '폴링 주기', default: '30s' },
      { name: 'timeout', type: 'string', required: false, description: '쿼리 타임아웃', default: '10s' },
    ],
    configExample: {
      agent_ref: 'my-influxdb',
      query: 'from(bucket:"sensors") |> range(start: -1h) |> filter(fn:(r) => r._measurement == "temperature")',
      language: 'flux',
      poll_interval: '30s',
    },
  },

  'influxdb-query': {
    description:
      '입력 메시지를 트리거로 InfluxDB 쿼리를 실행하는 노드입니다. $variable 패턴으로 payload 값을 쿼리에 치환할 수 있습니다.',
    ports: [
      { name: 'in', direction: 'input', description: '쿼리를 트리거할 메시지. $variable 치환 소스' },
      { name: 'out', direction: 'output', description: '쿼리 결과를 담은 새 메시지' },
      { name: 'error', direction: 'error', description: '쿼리 실패 시 에러 출력' },
    ],
    configFields: [
      { name: 'agent_ref', type: 'string', required: true, description: 'InfluxDB 에이전트 이름 또는 ID' },
      { name: 'query', type: 'string', required: false, description: '쿼리 ($variable 치환 지원). 비어있으면 payload.query 사용' },
      { name: 'language', type: 'string', required: false, description: '쿼리 언어', default: 'flux' },
      { name: 'timeout', type: 'string', required: false, description: '쿼리 타임아웃', default: '10s' },
      { name: 'result_key', type: 'string', required: false, description: '결과 저장 키', default: 'results' },
    ],
    configExample: {
      agent_ref: 'my-influxdb',
      query: 'SELECT * FROM $measurement WHERE location = $location LIMIT $limit',
      language: 'influxql',
      result_key: 'results',
    },
  },

  'store-write': {
    description:
      '메시지 데이터를 키-값 저장소에 기록하는 노드입니다. key_template으로 복합 키를 생성하고, value_key로 지정된 값 또는 전체 payload를 저장한 뒤 원본 메시지를 그대로 다음 노드로 전달합니다 (pass-through).',
    ports: [
      { name: 'in', direction: 'input', description: '저장할 메시지 입력. 페이로드에서 키 템플릿 필드와 값을 추출합니다.' },
      { name: 'out', direction: 'output', description: '저장 완료 후 원본 메시지를 passthrough로 출력합니다.' },
      { name: 'error', direction: 'error', description: 'Store 기록 실패 시 에러 메시지를 출력합니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '연결할 Store 에이전트의 이름 또는 ID입니다.',
      },
      {
        name: 'key_template',
        type: 'string',
        required: true,
        description: '키 템플릿입니다. {field} 형식 플레이스홀더를 payload 값으로 치환합니다 (예: "{location}:{point}:{sensor_type}").',
      },
      {
        name: 'value_key',
        type: 'string',
        required: false,
        description: 'payload에서 저장할 값의 키입니다. 비워두면 전체 payload를 저장합니다.',
      },
      {
        name: 'namespace',
        type: 'string',
        required: false,
        description: 'Store 네임스페이스입니다 (기본값: "default").',
        default: 'default',
      },
      {
        name: 'ttl',
        type: 'string',
        required: false,
        description: 'TTL 기간입니다 (예: "5m", "1h", "24h"). 비워두면 만료 없음.',
      },
    ],
    configExample: {
      agent_ref: 'store-engine',
      key_template: '{location}:{point}:{sensor_type}',
      value_key: 'value',
      namespace: 'sensors',
      ttl: '1h',
    },
  },

  'store-read': {
    description:
      '키-값 저장소에서 시계열 데이터를 조회하는 노드입니다. read_mode에 따라 현재값 1개 또는 최근 N개, 기간, 절대 시간 구간, 특정 시점부터의 엔트리를 조회할 수 있습니다. 결과는 항상 [{value, timestamp}, ...] 배열 형태로 output_key에 기록되며, timestamp는 epoch 밀리초(int64)입니다. 최신순이고 현재값을 포함합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '조회를 트리거하는 메시지 입력. 페이로드에서 키 템플릿 필드와 (필요 시) 동적 시간 참조 필드를 추출합니다.' },
      { name: 'out', direction: 'output', description: '조회 결과 배열이 추가된 메시지를 출력합니다.' },
      { name: 'error', direction: 'error', description: 'Store 조회 실패 또는 동적 참조 해석 실패 시 에러 메시지를 출력합니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '연결할 Store 에이전트의 이름 또는 ID입니다.',
      },
      {
        name: 'key_template',
        type: 'string',
        required: true,
        description: '키 템플릿입니다. {field} 형식 플레이스홀더를 payload 값으로 치환합니다.',
      },
      {
        name: 'namespace',
        type: 'string',
        required: false,
        description: 'Store 네임스페이스입니다 (기본값: "default").',
        default: 'default',
      },
      {
        name: 'output_key',
        type: 'string',
        required: false,
        description: '조회 결과 배열을 저장할 payload 키입니다 (기본값: "store_value"). 모든 모드에서 [{value, timestamp}, ...] 형태로 기록됩니다.',
        default: 'store_value',
      },
      {
        name: 'read_mode',
        type: 'string',
        required: false,
        description: '조회 모드 (기본값: "latest"). 옵션: latest (현재값 1개) / last_n (최신 N개) / duration (최근 기간) / time_range (절대 시간 구간) / since_n (특정 시점부터 N개).',
        default: 'latest',
      },
      {
        name: 'count',
        type: 'number',
        required: false,
        description: 'last_n, since_n 모드에서 반환할 엔트리 수 (1 이상).',
      },
      {
        name: 'duration',
        type: 'string',
        required: false,
        description: 'duration 모드에서 현재부터 역순 조회할 시간 길이 (예: "5m", "1h", "24h").',
      },
      {
        name: 'from',
        type: 'string',
        required: false,
        description: 'time_range 모드의 시작 시각. epoch ms 리터럴 또는 {payload_field} 동적 참조. RFC3339 문자열 호환 지원.',
      },
      {
        name: 'to',
        type: 'string',
        required: false,
        description: 'time_range 모드의 끝 시각. epoch ms 리터럴 또는 {payload_field} 동적 참조. RFC3339 문자열 호환 지원.',
      },
      {
        name: 'since',
        type: 'string',
        required: false,
        description: 'since_n 모드의 기준 시각. epoch ms 리터럴 또는 {payload_field} 동적 참조. RFC3339 문자열 호환 지원.',
      },
      {
        name: 'include_metadata',
        type: 'boolean',
        required: false,
        description: 'true 시 store_count, store_created_at, store_updated_at, store_oldest_at 메타데이터 필드를 payload에 추가합니다 (기본값: false).',
        default: 'false',
      },
      {
        name: 'entries_field',
        type: 'string',
        required: false,
        description: '배치 읽기: payload에서 배열을 추출할 필드명. 지정 시 배열 각 요소별로 key_template의 변수를 치환하여 다중 키를 조회합니다. 결과는 output_key에 map[요소값→엔트리배열] 형태로 기록됩니다.',
      },
      {
        name: 'entries_var',
        type: 'string',
        required: false,
        description: '배치 읽기 변수명. 배열 각 요소를 key_template의 {변수명} 플레이스홀더에 매핑합니다 (기본값: "item").',
        default: 'item',
      },
    ],
    configExample: {
      agent_ref: 'store-engine',
      key_template: '{location}:{point}:{sensor_type}',
      namespace: 'sensors',
      output_key: 'recent_values',
      read_mode: 'last_n',
      count: 10,
    },
  },

  'mqtt-subscriber': {
    description:
      'MQTT 에이전트에 직접 연결하여 설정된 토픽의 메시지를 구독 수신하는 노드입니다. SourceNode로서 Init 시 설정 토픽을 자동 구독하고, 수신 메시지를 플로우 메시지로 변환하여 출력합니다. Bridge 노드와 달리 MQTT 전용 설정(다중 토픽, QoS)을 직접 노출합니다.',
    ports: [
      { name: 'out', direction: 'output', description: 'MQTT 에이전트에서 수신한 메시지를 출력합니다. JSON 페이로드는 자동 파싱됩니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '연결할 MQTT 에이전트의 이름 또는 ID입니다.',
      },
      {
        name: 'topics',
        type: 'json',
        required: true,
        description: '구독할 토픽 목록입니다. 예: [{"topic": "sensor/temp", "qos": 0}]',
      },
      {
        name: 'payload_format',
        type: 'string',
        required: false,
        description: '수신 페이로드 형식입니다. "json" (기본값) 또는 "raw".',
      },
      {
        name: 'buffer_size',
        type: 'number',
        required: false,
        description: '수신 메시지 버퍼 크기입니다 (기본값: 64).',
      },
    ],
    configExample: {
      agent_ref: 'mqtt-sensor',
      topics: [{ topic: 'sensor/+/data', qos: 0 }],
      payload_format: 'json',
      buffer_size: 64,
    },
  },

  'serial-in': {
    description:
      '시리얼 포트에서 데이터를 수신하는 소스 노드입니다. 에이전트의 프레이밍 설정(raw, newline, frame 등)에 따라 프레임 단위로 데이터를 조립하여 전달합니다. out 포트는 프레이밍된 프레임을, raw_out 포트는 프레이밍 이전 원시 바이트를 출력합니다.',
    ports: [
      { name: 'out', direction: 'output', description: '프레이밍된 시리얼 데이터 출력' },
      { name: 'raw_out', direction: 'output', description: '프레이밍 이전 원시 바이트 출력' },
      { name: 'error', direction: 'error', description: '수신 에러 시 출력' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '연결할 시리얼 에이전트의 이름 또는 ID입니다.',
      },
    ],
    configExample: {
      agent_ref: 'serial-lgcp',
    },
    outputExamples: {
      out: {
        payload: {
          raw: '[]byte (바이너리 원본)',
          data: '562d04445500670445500000204...',
        },
        metadata: { 'serial.node_id': 'node-abc-123', 'serial.agent_type': 'serial' },
      },
      raw_out: {
        payload: { raw: '[]byte (프레이밍 이전 원본)' },
        metadata: { 'serial.node_id': 'node-abc-123', 'serial.port': 'raw_out' },
      },
    },
  },

  'serial-out': {
    description:
      '시리얼 포트로 데이터를 전송하는 노드입니다. 입력 메시지의 payload에서 raw([]byte) → data(string) → JSON 직렬화 순서로 전송 데이터를 결정합니다. 전송 후 원본 메시지를 clone하여 다음 노드로 전달합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '전송할 데이터. payload.raw([]byte) 우선, payload.data(string) 차선' },
      { name: 'out', direction: 'output', description: '전송 후 원본 메시지 clone 출력' },
      { name: 'error', direction: 'error', description: '전송 실패 시 출력' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '연결할 시리얼 에이전트의 이름 또는 ID입니다.',
      },
    ],
    configExample: {
      agent_ref: 'serial-lgcp',
    },
    outputExamples: {
      in_example: {
        _comment: '입력 메시지 예시 (전송 우선순위: raw → data → JSON)',
        payload: { raw: '[56 2d 04 44 55 ...]' },
      },
      out: {
        payload: { raw: '[56 2d 04 44 55 ...]', data: 'V-\\u0004DU...' },
        metadata: { 'serial.node_id': 'node-abc-123' },
      },
    },
  },

  'tcp-in': {
    description:
      'TCP 에이전트로부터 메시지를 수신하는 소스 노드입니다. TCP 서버 에이전트 사용 시 클라이언트 연결 정보(remote_addr)가 메타데이터에 포함되며, TCP 클라이언트 에이전트 사용 시 서버에서 수신한 데이터를 전달합니다.',
    ports: [
      { name: 'out', direction: 'output', description: 'TCP 수신 데이터 출력' },
      { name: 'error', direction: 'error', description: '수신 에러 시 출력' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '연결할 TCP 에이전트(tcp-server 또는 tcp-client)의 이름 또는 ID입니다.',
      },
    ],
    configExample: {
      agent_ref: 'tcp-server-gateway',
    },
    outputExamples: {
      'out (tcp-server)': {
        payload: {
          raw: '[]byte (바이너리 원본)',
          data: '48656c6c6f2066726f6d20636c69656e74',
        },
        metadata: { 'tcp.remote_addr': '192.168.1.100:5678', 'tcp.node_id': 'node-abc', 'tcp.agent_type': 'tcp-server' },
      },
      'out (tcp-client)': {
        payload: {
          raw: '[]byte (바이너리 원본)',
          data: '48656c6c6f2066726f6d20736572766572',
        },
        metadata: { 'tcp.node_id': 'node-abc', 'tcp.agent_type': 'tcp-client' },
      },
    },
  },

  'tcp-out': {
    description:
      'TCP 에이전트를 통해 데이터를 전송하는 노드입니다. payload에서 raw([]byte) → data(string) → JSON 직렬화 순서로 전송 데이터를 결정합니다. 메타데이터의 tcp.remote_addr로 특정 클라이언트에 응답하거나, 비어있으면 전체 브로드캐스트합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '전송할 데이터. payload.raw([]byte) 우선. metadata.tcp.remote_addr: 대상 지정 (없으면 브로드캐스트)' },
      { name: 'out', direction: 'output', description: '전송 후 원본 메시지 clone 출력' },
      { name: 'error', direction: 'error', description: '전송 실패 시 출력' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '연결할 TCP 에이전트(tcp-server 또는 tcp-client)의 이름 또는 ID입니다.',
      },
    ],
    configExample: {
      agent_ref: 'tcp-server-gateway',
    },
    outputExamples: {
      in_example: {
        _comment: '입력 메시지 예시 (특정 클라이언트에 응답)',
        payload: { raw: '[4f 4b]', data: 'OK' },
        metadata: { 'tcp.remote_addr': '192.168.1.100:5678' },
      },
      out: {
        payload: { raw: '[4f 4b]', data: 'OK' },
        metadata: { 'tcp.node_id': 'node-abc', 'tcp.remote_addr': '192.168.1.100:5678' },
      },
    },
  },

  'lgcp-status': {
    description:
      'LG LGCP 에이전트에 연결하여 RS-485 버스에서 캡처된 실내기 상태를 조회하는 노드입니다. 주소를 지정하면 해당 실내기만, 미지정 시 전체 실내기를 조회합니다. poll_interval 설정 시 주기적으로 자동 폴링합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '상태 조회 트리거. payload.address로 실내기 지정 가능' },
      { name: 'out', direction: 'output', description: '조회 결과 출력. get_stats 또는 get_recent 응답' },
      { name: 'error', direction: 'error', description: '에이전트 통신 실패 시 출력' },
    ],
    configFields: [
      { name: 'agent_ref', type: 'string', required: true, description: '연결할 LGCP 에이전트의 이름 또는 ID' },
      { name: 'default_address', type: 'string', required: false, description: '기본 실내기 주소 (예: 01)' },
      { name: 'poll_interval', type: 'string', required: false, description: '자동 폴링 주기 (예: 10s, 1m)', default: '30s' },
      { name: 'timeout', type: 'string', required: false, description: 'Agent Process 타임아웃', default: '5s' },
      { name: 'poll_command', type: 'string', required: false, description: '폴링 명령 (get_stats 또는 get_recent)', default: 'get_stats' },
      { name: 'recent_count', type: 'number', required: false, description: 'get_recent 시 최근 데이터 수', default: '10' },
    ],
    configExample: {
      agent_ref: 'lgcp-capture',
      default_address: '67',
      poll_interval: '10s',
      poll_command: 'get_stats',
    },
  },

  'lgcp-control': {
    description:
      'LG LGCP 프로토콜로 실내기를 제어하는 노드입니다. 전원, 온도, 풍량, 운전모드를 설정합니다. control_enabled가 활성화된 LGCP 에이전트가 필요합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '제어 명령 입력. payload: {address, command, ...params}' },
      { name: 'out', direction: 'output', description: '제어 결과 출력' },
      { name: 'error', direction: 'error', description: '제어 실패 시 출력' },
    ],
    configFields: [
      { name: 'agent_ref', type: 'string', required: true, description: '연결할 LGCP 에이전트 (control_enabled 필요)' },
      { name: 'default_address', type: 'string', required: false, description: '기본 실내기 주소 (예: 67)' },
      { name: 'timeout', type: 'string', required: false, description: 'Agent Process 타임아웃', default: '5s' },
    ],
    configExample: {
      agent_ref: 'lgcp-control',
      default_address: '67',
      timeout: '5s',
    },
  },

  lgcp: {
    description:
      'LG LGCP 실내기 상태 조회 + 제어 통합 노드입니다. 입력 메시지에 제어 키(power, temperature, fan_speed, mode)가 있으면 제어, 없으면 상태 조회로 동작합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '상태 조회 또는 제어 명령. 제어 키 유무에 따라 자동 분기' },
      { name: 'out', direction: 'output', description: '상태 또는 제어 결과 출력' },
      { name: 'error', direction: 'error', description: '에러 시 출력' },
    ],
    configFields: [
      { name: 'agent_ref', type: 'string', required: true, description: '연결할 LGCP 에이전트' },
      { name: 'default_address', type: 'string', required: false, description: '기본 실내기 주소' },
      { name: 'poll_interval', type: 'string', required: false, description: '자동 폴링 주기', default: '30s' },
      { name: 'timeout', type: 'string', required: false, description: 'Agent Process 타임아웃', default: '5s' },
      { name: 'poll_command', type: 'string', required: false, description: '폴링 명령', default: 'get_stats' },
      { name: 'recent_count', type: 'number', required: false, description: 'get_recent 시 최근 데이터 수', default: '10' },
    ],
    configExample: {
      agent_ref: 'lgcp-capture',
      default_address: '67',
      poll_interval: '15s',
    },
  },

  'lgcnp-status': {
    description:
      'LG LGCNP-01 프로토콜로 에어컨 상태를 조회하는 노드입니다. 에이전트의 캡처 버퍼에서 TYPE-A(ODU)/TYPE-B(IDU) 프레임을 폴링하여 개별 메시지로 출력합니다.',
    ports: [
      { name: 'out', direction: 'output', description: '캡처된 디바이스 상태 출력 (type=device_state, dev_id=odu/idu-N, trigger=change/report)' },
      { name: 'error', direction: 'error', description: '에러 시 출력' },
    ],
    configFields: [
      { name: 'agent_ref', type: 'string', required: true, description: '연결할 LGCNP 에이전트' },
      { name: 'poll_interval', type: 'string', required: false, description: '폴링 주기', default: '100ms' },
      { name: 'timeout', type: 'string', required: false, description: 'Agent Process 타임아웃', default: '5s' },
      { name: 'poll_command', type: 'string', required: false, description: '폴링 명령 (get_recent / get_stats). v0.7.1: get_recent + count=0 = drain (전체 반환 + 버퍼 비움)', default: 'get_recent' },
      { name: 'recent_count', type: 'number', required: false, description: 'get_recent 시 최근 프레임 수', default: '10' },
      { name: 'batch_size', type: 'number', required: false, description: '배치 크기', default: '32' },
    ],
    configExample: {
      agent_ref: 'lgcnp-capture',
      poll_interval: '100ms',
      poll_command: 'get_recent',
    },
  },

  'lgcnp-control': {
    description:
      'LG LGCNP-01 디바이스 제어 노드입니다. 현재 LGCNP-01 프로토콜의 쓰기 명령이 확인되지 않아 모든 제어 요청에 미지원 응답을 반환합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '제어 명령 입력' },
      { name: 'out', direction: 'output', description: '미지원 응답 출력' },
      { name: 'error', direction: 'error', description: '에러 시 출력' },
    ],
    configFields: [
      { name: 'agent_ref', type: 'string', required: true, description: '연결할 LGCNP 에이전트' },
      { name: 'timeout', type: 'string', required: false, description: 'Agent Process 타임아웃', default: '5s' },
    ],
    configExample: {
      agent_ref: 'lgcnp-capture',
    },
  },

  lgcnp: {
    description:
      'LG LGCNP-01 상태 조회 + 제어 통합 노드입니다. 입력 메시지에 제어 키가 있으면 미지원 응답을, 없으면 상태 조회로 동작합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '상태 조회 또는 제어 명령' },
      { name: 'out', direction: 'output', description: '상태 또는 제어 결과 출력' },
      { name: 'error', direction: 'error', description: '에러 시 출력' },
    ],
    configFields: [
      { name: 'agent_ref', type: 'string', required: true, description: '연결할 LGCNP 에이전트' },
      { name: 'poll_interval', type: 'string', required: false, description: '폴링 주기', default: '100ms' },
      { name: 'timeout', type: 'string', required: false, description: 'Agent Process 타임아웃', default: '5s' },
      { name: 'poll_command', type: 'string', required: false, description: '폴링 명령 (get_recent / get_stats). v0.7.1: get_recent + count=0 = drain (전체 반환 + 버퍼 비움)', default: 'get_recent' },
      { name: 'recent_count', type: 'number', required: false, description: 'get_recent 시 최근 프레임 수', default: '10' },
      { name: 'batch_size', type: 'number', required: false, description: '배치 크기', default: '32' },
    ],
    configExample: {
      agent_ref: 'lgcnp-capture',
      poll_interval: '100ms',
      poll_command: 'get_recent',
    },
  },

  'mqtt-publisher': {
    description:
      'MQTT 에이전트에 직접 연결하여 메시지를 발행하는 노드입니다. 입력 메시지를 MQTT 페이로드로 변환하여 지정된 토픽에 발행합니다. 토픽은 설정 기본값, 메시지 메타데이터(mqtt.topic), 또는 페이로드의 _mqtt.topic 객체로 지정할 수 있습니다. 발행 후 원본 메시지를 passthrough로 출력합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '발행할 메시지를 수신합니다. 페이로드가 MQTT 메시지로 변환됩니다.' },
      { name: 'out', direction: 'output', description: '발행 완료 후 원본 메시지를 passthrough로 출력합니다. mqtt.published_topic 메타데이터가 추가됩니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '연결할 MQTT 에이전트의 이름 또는 ID입니다.',
      },
      {
        name: 'default_topic',
        type: 'string',
        required: false,
        description: '기본 발행 토픽입니다. 메시지의 _mqtt.topic 또는 metadata mqtt.topic으로 오버라이드 가능합니다.',
      },
      {
        name: 'default_qos',
        type: 'number',
        required: false,
        description: '기본 QoS 레벨입니다 (0, 1, 2).',
      },
      {
        name: 'default_retained',
        type: 'boolean',
        required: false,
        description: '기본 Retained 플래그입니다.',
      },
      {
        name: 'payload_format',
        type: 'string',
        required: false,
        description: '발행 페이로드 형식입니다. "json" (기본값) 또는 "raw".',
      },
    ],
    configExample: {
      agent_ref: 'mqtt-sensor',
      default_topic: 'device/command',
      default_qos: 1,
      default_retained: false,
      payload_format: 'json',
    },
  },

  'chart-emitter': {
    description:
      '입력 메시지를 WebSocket 차트 채널로 발행하고 링버퍼에 보관합니다. 대시보드의 차트 패널(Stat/Line/Bar/Pie/Table)이 이 채널을 구독해 실시간 데이터를 표시합니다. 두 가지 입력 모드를 지원합니다: 단일 엔트리(실시간 append) 와 배치(entries_field 설정 시 배열 분해). 필터/집계/정렬은 filter, aggregate, mapping 등 기존 노드와 조합해 앞단에 배치합니다. 종단 노드이므로 출력 포트가 없습니다.',
    ports: [
      { name: 'in', direction: 'input', description: '차트 채널로 발행할 메시지 입력 (단일 또는 배치)' },
    ],
    configFields: [
      {
        name: 'channel_name',
        type: 'string',
        required: false,
        description:
          '단일 채널 모드: 차트 패널이 구독할 고유 채널 이름. 멀티채널 모드(channels_field) 사용 시 불필요.',
      },
      {
        name: 'buffer_size',
        type: 'number',
        required: false,
        description: '링버퍼가 보관할 최근 메시지 개수 (1-10000). 신규 구독자 연결 시 이 크기만큼 backfill 로 즉시 전송합니다.',
        default: '100',
      },
      {
        name: 'retention_sec',
        type: 'number',
        required: false,
        description: '링버퍼 항목 최대 보존 시간 (초, 0-86400). 0 이면 시간 기반 만료를 비활성화합니다.',
        default: '3600',
      },
      {
        name: 'entries_field',
        type: 'string',
        required: false,
        description:
          '단일 채널 배치 모드: payload 내 엔트리 배열 필드명. 각 element를 개별 차트 엔트리로 분해하여 publish. store-read 기본 output_key "store_value" 사용 가능.',
      },
      {
        name: 'channels_field',
        type: 'string',
        required: false,
        description:
          '멀티채널 모드: payload 내 map[string]entries 필드명. 각 키별로 별도 채널을 생성하여 발행합니다. store-read 의 entries_field 배치 출력(map 형태)과 직접 연결됩니다. channel_name 대신 사용.',
      },
      {
        name: 'channel_prefix',
        type: 'string',
        required: false,
        description:
          '멀티채널 모드에서 각 키 앞에 붙일 접두사. 예: "temp_" → map 키 "room1" → 채널 "temp_room1".',
      },
    ],
    configExample: {
      channel_name: 'room1_temp',
      buffer_size: 500,
      retention_sec: 3600,
      entries_field: 'store_value',
    },
    inputExamples: {
      '배치 모드 · store-read 출력을 라인 차트에 공급 (entries_field="store_value")': {
        store_value: [
          { timestamp: 1776339916504, value: 21 },
          { timestamp: 1776339912023, value: 21 },
          { timestamp: 1776339907563, value: 21 },
          { timestamp: 1776339903066, value: 21 },
          { timestamp: 1776339898649, value: 21.5 },
          '... (store-read 의 last_n/duration/time_range 결과 배열, 최신순 허용)',
        ],
      },
      '배치 모드 · 커스텀 필드명 (entries_field="rows")': {
        rows: [
          { timestamp: 1713312000000, value: 25.5, labels: { room: 'room1' } },
          { timestamp: 1713312001000, value: 25.7, labels: { room: 'room1' } },
        ],
      },
      '배치 모드 · primitive 배열 (timestamp 자동 주입, entries_field="values")': {
        values: [21, 22, 23, 24],
      },
      '단일 엔트리 · 실시간 append (정규 형식)': {
        timestamp: 1713312000000,
        value: 25.5,
        labels: { room: 'room1', sensor: 'temp' },
        meta: { source: 'modbus-poller' },
      },
      '단일 엔트리 · timestamp 생략 → 현재 epoch ms 주입': {
        value: 42.5,
      },
      '단일 엔트리 · value 생략 → payload 전체를 value 로 래핑': {
        room: 'room1',
        temp: 25,
        humidity: 60,
      },
      '단일 엔트리 · 카테고리 분포용 labels 지정 (pie/bar)': {
        timestamp: 1713312000000,
        value: 1,
        labels: { category: 'error' },
      },
      '멀티채널 · store-read 배치 출력 (channels_field="store_value", channel_prefix="temp_")': {
        store_value: {
          room1: [
            { timestamp: 1713312000000, value: 23.5 },
            { timestamp: 1713312001000, value: 23.7 },
          ],
          room2: [
            { timestamp: 1713312000000, value: 24.1 },
          ],
        },
      },
    },
    outputExamples: {
      _note:
        '출력 포트 없음 (sink). WebSocket 프레임 예: { type: "chart.append", channel: "room1_temp", entry: { timestamp: 1713312000000, value: 25.5, labels: { room: "room1" } } }. 배치 모드에서는 각 엔트리가 개별 chart.append 로 브로드캐스트됩니다.',
    },
  },
};
