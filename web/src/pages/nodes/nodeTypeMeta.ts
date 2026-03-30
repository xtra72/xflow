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
}

export const NODE_TYPE_META: Record<string, NodeTypeDetailMeta> = {
  filter: {
    description:
      '조건식을 평가하여 메시지를 필터링합니다. 조건이 true이면 메시지를 출력 포트로 전달하고, false이면 드롭합니다. 에러 발생 시 에러 포트로 전달됩니다.',
    ports: [
      { name: 'in', direction: 'input', description: '필터링할 메시지 입력' },
      { name: 'out', direction: 'output', description: '조건을 통과한 메시지 출력' },
      { name: 'error', direction: 'error', description: '처리 중 에러 발생 시 출력' },
    ],
    configFields: [
      {
        name: 'condition',
        type: 'string',
        required: false,
        description: '필터 조건식. 미설정 시 모든 메시지를 통과시킵니다 (pass-through).',
      },
    ],
    configExample: {
      condition: 'payload.level == "error"',
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

  debug: {
    description:
      '메시지를 디버그 로그로 출력합니다. 개발 및 테스트 시 메시지 흐름을 추적하는 데 유용합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '디버그할 메시지 입력' },
      { name: 'out', direction: 'output', description: '메시지를 그대로 전달 (pass-through)' },
    ],
    configFields: [
      {
        name: 'level',
        type: 'string',
        required: false,
        description: '디버그 출력 레벨 (debug, info, warn)',
        default: 'debug',
      },
    ],
    configExample: {
      level: 'info',
    },
  },

  output: {
    description:
      '메시지를 포맷팅하여 출력합니다. Go text/template 형식의 템플릿을 지정하면 페이로드 필드를 포맷팅하여 출력하고, 미지정 시 전체 페이로드를 JSON으로 출력합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '출력할 메시지 입력' },
      { name: 'out', direction: 'output', description: '메시지를 그대로 전달 (pass-through)' },
    ],
    configFields: [
      {
        name: 'prefix',
        type: 'string',
        required: false,
        description: '로그 출력 시 접두어',
        default: '[output]',
      },
      {
        name: 'template',
        type: 'string',
        required: false,
        description: 'Go text/template 형식의 메시지 템플릿. 미지정 시 전체 페이로드 JSON 출력.',
      },
    ],
    configExample: {
      prefix: '[output]',
      template: '온도={{.temperature}}, 습도={{.humidity}}',
    },
  },

  status: {
    description:
      '플로우의 런타임 상태를 모니터링합니다. 특정 노드들의 처리 현황을 감시하고 상태 리포트를 출력합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '상태 조회 트리거 메시지 입력' },
      { name: 'out', direction: 'output', description: '상태 리포트 메시지 출력' },
    ],
    configFields: [
      {
        name: 'watch_nodes',
        type: 'string[]',
        required: false,
        description: '감시할 노드 이름 목록. 미설정 시 전체 노드를 감시합니다.',
      },
    ],
    configExample: {
      watch_nodes: ['filter-1', 'transform-1'],
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
      { name: 'out', direction: 'output', description: '디바이스 상태 조회 결과를 출력합니다. get_state 또는 get_all_states 응답이 포함됩니다.' },
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
        description: '조회할 디바이스 ID입니다 (예: "living-room"). 미지정 시 get_all_states로 전체 디바이스를 조회합니다.',
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
      'Samsung NASA 에이전트의 상태 조회와 제어를 하나의 노드에서 처리하는 복합 노드입니다. 입력 메시지의 페이로드를 분석하여 자동으로 상태 조회 또는 제어 명령을 판별합니다. 제어 키(power, mode, temperature, target_temp, fan_speed)가 포함되면 제어, 그 외에는 상태 조회로 동작합니다. poll_interval 설정 시 SourceNode로서 주기적 상태 폴링도 수행합니다.',
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
      { name: 'out', direction: 'output', description: '디바이스 상태 조회 결과를 출력합니다. get_state 또는 get_all_states 응답이 포함됩니다.' },
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
        description: '조회할 디바이스 ID입니다 (예: "0x11"). 미지정 시 get_all_states로 전체 디바이스를 조회합니다.',
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
      'LG LGAP 에이전트의 상태 조회와 제어를 하나의 노드에서 처리하는 복합 노드입니다. 입력 메시지의 페이로드를 분석하여 자동으로 상태 조회 또는 제어 명령을 판별합니다. 제어 키(power, mode, temperature, target_temp, fan_speed)가 포함되면 제어, 그 외에는 상태 조회로 동작합니다. poll_interval 설정 시 SourceNode로서 주기적 상태 폴링도 수행합니다.',
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
      '키-값 저장소에서 데이터를 조회하는 노드입니다. key_template으로 키를 생성하여 Store에서 값을 조회하고, 결과를 payload의 output_key에 추가한 뒤 메시지를 다음 노드로 전달합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '조회를 트리거하는 메시지 입력. 페이로드에서 키 템플릿 필드를 추출합니다.' },
      { name: 'out', direction: 'output', description: '조회 결과가 추가된 메시지를 출력합니다.' },
      { name: 'error', direction: 'error', description: 'Store 조회 실패 시 에러 메시지를 출력합니다.' },
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
        description: '조회된 값을 저장할 payload 키입니다 (기본값: "store_value").',
        default: 'store_value',
      },
    ],
    configExample: {
      agent_ref: 'store-engine',
      key_template: '{location}:{point}:{sensor_type}',
      namespace: 'sensors',
      output_key: 'stored_value',
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
};
