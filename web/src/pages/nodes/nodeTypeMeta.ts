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
      { name: 'key', type: 'string', required: false, description: '메시지 그룹핑 키. $. prefix 필수 — $.-경로(예: $.payload.idu_num, $.payload.state.mode, $.metadata.device.id)로 메시지 전체를 대상으로 합니다. 비어있거나 경로 해석 실패 시 전체 메시지 기준' },
      { name: 'window', type: 'string', required: false, description: '중복 억제 시간 창 (예: 30s, 1m)', default: '30s' },
      { name: 'compare_fields', type: 'string', required: false, description: '비교 대상 필드 (콤마 구분). 비어있으면 전체 페이로드 비교 (timestamp/seq/raw_hex 제외). 각 필드는 $. prefix 필수 — $.-경로(예: $.payload.current_temperature, $.payload.state.mode, $.metadata.device.id)로 메시지 전체를 대상으로 합니다' },
      { name: 'on_duplicate', type: 'string', required: false, description: '중복 시 처리: drop (기본, 폐기) 또는 reject_port (reject 포트로 전달)', default: 'drop' },
    ],
    configExample: {
      key: '$.payload.idu_num',
      window: '30s',
      compare_fields: '$.payload.current_temperature,$.payload.target_temperature,$.payload.op_mode,$.payload.fan_byte',
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

  split: {
    description:
      '배열 페이로드를 요소별 N개 메시지로 분리하는 노드입니다. path 로 지정한 배열을 꺼내 각 요소마다 한 개의 메시지로 팬아웃합니다. 요소가 metadata/payload 키를 가진 객체이면 완전한 메시지로 재구성(messages)하고, 그 외에는 payload 로 취급(payloads)합니다. mode=auto 는 요소별로 자동 감지합니다. 입력 배열 순서는 출력에서 보존됩니다.',
    ports: [
      { name: 'in', direction: 'input', description: '배열을 담은 메시지 입력. path 로 배열 위치를 지정합니다.' },
      { name: 'out', direction: 'output', description: '배열 요소마다 1개씩 생성된 메시지(N개) 출력. correlation id 는 <부모ID>#<인덱스>.' },
    ],
    configFields: [
      {
        name: 'path',
        type: 'string',
        required: true,
        description: '분리할 배열의 위치. 평면 top-level 페이로드 키(예: items) 또는 $. 접두 JSONPath(예: $.items[*]).',
      },
      {
        name: 'mode',
        type: 'string',
        required: false,
        description: 'auto: 요소별 자동 감지 / payloads: 요소=payload(부모 메타 공유) / messages: 요소=완전한 메시지(부모 메타에 요소 메타 병합).',
        default: 'auto',
      },
      {
        name: 'share_metadata',
        type: 'boolean',
        required: false,
        description: '분리된 각 메시지에 부모 메타데이터를 공유합니다.',
        default: 'true',
      },
      {
        name: 'on_missing',
        type: 'string',
        required: false,
        description: 'path 누락/비배열 시: passthrough(입력 그대로 통과) 또는 error(노드 에러).',
        default: 'passthrough',
      },
      {
        name: 'on_empty',
        type: 'string',
        required: false,
        description: '빈 배열 시: emit_none(0개 방출) 또는 passthrough(입력 그대로 통과).',
        default: 'emit_none',
      },
      {
        name: 'scalar_key',
        type: 'string',
        required: false,
        description: 'payloads 모드에서 비객체(스칼라) 요소를 감쌀 payload 키.',
        default: 'value',
      },
    ],
    configExample: {
      path: '$.items[*]',
      mode: 'auto',
      share_metadata: true,
      on_missing: 'passthrough',
      on_empty: 'emit_none',
      scalar_key: 'value',
    },
    inputExamples: {
      'payloads 모드 · 스칼라 배열': {
        _comment: 'path=$.readings 배열의 각 요소가 scalar_key(value)로 래핑되어 3개 메시지로 분리',
        payload: { readings: [21.5, 22.0, 22.4] },
      },
      'messages 모드 · 객체 배열': {
        _comment: 'path=$.events 각 요소가 완전한 메시지({metadata, payload})로 재구성',
        payload: {
          events: [
            { metadata: { device_id: 'd1' }, payload: { temp: 25 } },
            { metadata: { device_id: 'd2' }, payload: { temp: 26 } },
          ],
        },
      },
    },
    outputExamples: {
      'out (payloads 모드 첫 번째 요소)': {
        _comment: '스칼라 요소가 scalar_key 로 래핑되고 부모 메타 공유',
        payload: { value: 21.5 },
      },
    },
  },

  enrich: {
    description:
      'slim 된 메시지의 agent/device 그룹을 레지스트리 룩업으로 in-flow 재수화하는 노드입니다. agent 와 device 는 독립 블록으로, 둘 다(또는 하나만) 동시에 보강할 수 있습니다. 각 블록은 id_source 로 얻은 id 를 레지스트리에서 조회하여 type/name 을 얻고, to_metadata 로 해당 메타데이터 그룹을 재수화하거나 to_payload 로 {type,id,name} 객체를 payload 키에 기록합니다. 한 블록은 to_metadata=true 또는 to_payload 값이 있어야 활성화되며, 최소 한 블록이 활성이어야 합니다. id 를 얻지 못하거나 레지스트리에 없으면 그 블록만 원본 그대로 통과합니다(에러 아님, no-op). id 는 소스 값을 항상 보존합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '보강할 메시지 입력. 각 블록의 id_source 로 id 를 해석합니다.' },
      { name: 'out', direction: 'output', description: '보강된 메시지 출력. 룩업 실패/누락 id 는 해당 블록만 원본 유지(pass-through).' },
    ],
    configFields: [
      {
        name: 'agent',
        type: 'object_fields',
        required: false,
        description:
          'agent 레지스트리 룩업 블록(선택, 중첩 오브젝트 — 에디터에서 네이티브 위젯으로 편집). 하위 필드: enabled(bool, 블록 존재 시 기본 true), id_source(string, 기본 $.metadata.agent.id), to_metadata(bool), to_payload(string). to_metadata=true 또는 to_payload 값이 있어야 활성화됩니다.',
      },
      {
        name: 'device',
        type: 'object_fields',
        required: false,
        description:
          'device 레지스트리 룩업 블록(선택, 중첩 오브젝트 — 에디터에서 네이티브 위젯으로 편집). 하위 필드: enabled(bool, 블록 존재 시 기본 true), id_source(string, 기본 $.metadata.device.id), to_metadata(bool), to_payload(string). to_metadata=true 또는 to_payload 값이 있어야 활성화됩니다.',
      },
    ],
    configExample: {
      agent: {
        enabled: true,
        id_source: '$.metadata.agent.id',
        to_metadata: true,
        to_payload: 'agent_info',
      },
      device: {
        enabled: true,
        id_source: '$.metadata.device.id',
        to_metadata: true,
        to_payload: 'device_info',
      },
    },
    inputExamples: {
      in: {
        _comment: 'slim 된 메시지 — agent/device 그룹은 id 만 존재',
        payload: { current_temperature: 25.5 },
        metadata: { agent: { id: 'lg_icp01' }, device: { id: 'lg_icp01:1' } },
      },
    },
    outputExamples: {
      'out (agent + device 블록 모두 활성)': {
        _comment:
          'agent/device 그룹이 각각 type/name 으로 재수화되고, agent_info / device_info 키에 {type,id,name} 기록. 두 블록은 독립 적용.',
        payload: {
          current_temperature: 25.5,
          agent_info: { type: 'lg_hvacr01', id: 'lg_icp01', name: 'LG 캡처 에이전트' },
          device_info: { type: 'HVACR.IDU', id: 'lg_icp01:1', name: 'IDU-1' },
        },
        metadata: {
          agent: { type: 'lg_hvacr01', id: 'lg_icp01', name: 'LG 캡처 에이전트' },
          device: { type: 'HVACR.IDU', id: 'lg_icp01:1', name: 'IDU-1' },
        },
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
      {
        name: 'on_error',
        type: 'string',
        required: false,
        description: 'error: 실패 시 오류 발생 · ignore: 실패 시 원본 메시지 통과(로그 없음) · drop: 실패 시 출력 없음',
        default: 'error',
      },
    ],
    configExample: {
      script: 'return { ...msg, payload: { ...msg.payload, processed: true } }',
      on_error: 'error',
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

  'samsung-hvacr01-status': {
    description:
      'Samsung HVACR-01 에이전트(Samsung NASA 프로토콜)의 push 메시지를 수신하는 노드입니다. 에이전트가 NotifyInterval 마다 디바이스별 상태를 emit 하고, 노드는 ring buffer 를 drain 합니다. inactivity_timeout 동안 무수신 시에만 request_state 명령을 전송합니다. group_id / unit_id 로 특정 외기/내기를 필터링할 수 있습니다.',
    ports: [
      { name: 'in', direction: 'input', description: '상태 수신을 트리거하는 메시지(즉시 drain). payload 키는 무시됩니다.' },
      { name: 'out', direction: 'output', description: '디바이스 상태를 디바이스별 개별 메시지로 출력합니다.' },
      { name: 'error', direction: 'error', description: '에이전트 통신 실패, 타임아웃 등 에러 발생 시 출력합니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '대상 Samsung HVACR-01 에이전트의 이름 또는 ID입니다.',
      },
      {
        name: 'inactivity_timeout',
        type: 'string',
        required: false,
        description: '무수신 임계 시간 (예: "90s"). 에이전트로부터 메시지가 끊긴 시간이 이 값을 넘으면 request_state 자동 전송. 에이전트 NotifyInterval 의 1.5~2배 권장.',
        default: '90s',
      },
      {
        name: 'timeout',
        type: 'string',
        required: false,
        description: 'Agent Process() 호출 타임아웃입니다.',
        default: '5s',
      },
      {
        name: 'batch_size',
        type: 'number',
        required: false,
        description: 'drain 시 한 번에 가져올 최대 프레임 수.',
        default: '32',
      },
      {
        name: 'group_id',
        type: 'string',
        required: false,
        description: '(고급) Samsung NASA 외기 인덱스 hex (예: "00" ~ "0F"). 비우면 전체 그룹.',
      },
      {
        name: 'unit_id',
        type: 'string',
        required: false,
        description: '(고급) Samsung NASA 내기 인덱스 hex (예: "00" ~ "3F"). 비우면 그룹 내 전체 유닛.',
      },
    ],
    configExample: {
      agent_ref: 'samsung_hvacr01-agent',
      inactivity_timeout: '90s',
      timeout: '5s',
      batch_size: 32,
    },
  },

  'samsung-hvacr01-control': {
    description:
      'Samsung HVACR-01 에이전트(Samsung NASA 프로토콜)에 제어 명령을 전송하는 노드입니다. 직접 명령 형식(command 키 포함)과 간편 형식(power, mode 등 제어 키)을 모두 지원합니다. 간편 형식은 자동으로 set_multiple 명령으로 변환됩니다. 모든 설정값(device_id)은 입력 메시지 payload로 런타임 오버라이드할 수 있습니다.',
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
        description: '대상 Samsung HVACR-01 에이전트의 이름 또는 ID입니다.',
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
      agent_ref: 'samsung_hvacr01-agent',
      device_id: 'living-room',
      timeout: '5s',
    },
  },

  'samsung-hvacr01': {
    description:
      'Samsung HVACR-01 에이전트(Samsung NASA 프로토콜)의 상태 수신과 제어를 하나의 노드에서 처리하는 복합 노드입니다. 입력 메시지에 제어 키(power, mode, temperature, target_temperature, fan_speed)가 있으면 제어 명령으로, 없으면 즉시 drain 으로 동작합니다. 무수신 임계 시간(inactivity_timeout) 초과 시 request_state 자동 전송. group_id / unit_id 로 외기/내기 어드레싱 가능.',
    ports: [
      { name: 'in', direction: 'input', description: '상태 수신 트리거 또는 제어 명령 메시지를 수신합니다. 제어 키 유무에 따라 자동 분기됩니다.' },
      { name: 'out', direction: 'output', description: '디바이스 상태 또는 제어 실행 결과를 출력합니다.' },
      { name: 'error', direction: 'error', description: '에이전트 통신 실패, 타임아웃 등 에러 발생 시 출력합니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '대상 Samsung HVACR-01 에이전트의 이름 또는 ID입니다.',
      },
      {
        name: 'inactivity_timeout',
        type: 'string',
        required: false,
        description: '무수신 임계 시간 (예: "90s"). 초과 시 request_state 자동 전송.',
        default: '90s',
      },
      {
        name: 'timeout',
        type: 'string',
        required: false,
        description: 'Agent Process() 호출 타임아웃입니다.',
        default: '5s',
      },
      {
        name: 'batch_size',
        type: 'number',
        required: false,
        description: 'drain 시 한 번에 가져올 최대 프레임 수.',
        default: '32',
      },
      {
        name: 'group_id',
        type: 'string',
        required: false,
        description: '(고급) Samsung NASA 외기 인덱스 hex (예: "00" ~ "0F").',
      },
      {
        name: 'unit_id',
        type: 'string',
        required: false,
        description: '(고급) Samsung NASA 내기 인덱스 hex (예: "00" ~ "3F").',
      },
    ],
    configExample: {
      agent_ref: 'samsung_hvacr01-agent',
      inactivity_timeout: '90s',
      timeout: '5s',
      batch_size: 32,
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

  'modbus-write': {
    description:
      'MODBUS 에이전트(Client/Server)에 연결하여 레지스터에 값을 쓰는 노드입니다. config의 command_set(WriteOp 배열)이 기본값이며, 입력 메시지 payload에 command_set이 있으면 런타임에 오버라이드됩니다.',
    ports: [
      { name: 'in', direction: 'input', description: '쓰기를 트리거하는 메시지를 수신합니다. payload에 command_set이 있으면 config 기본값을 오버라이드합니다.' },
      { name: 'out', direction: 'output', description: '쓰기 결과를 출력합니다.' },
      { name: 'error', direction: 'error', description: '에이전트 통신 실패 등 에러 발생 시 출력합니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '대상 MODBUS 에이전트(Client/Server)의 이름 또는 ID입니다.',
      },
      {
        name: 'command_set',
        type: 'json',
        required: false,
        description: '쓰기 명령(WriteOp) 배열입니다. 각 항목: area, address, value(단일) 또는 values(배열), data_type, byte_order, unit_id(0=공유). config 기본값이며 입력 payload의 command_set으로 오버라이드됩니다.',
      },
    ],
    configExample: {
      agent_ref: 'modbus-gateway-1',
      command_set: [
        { area: 'holding_registers', address: 100, value: 42, data_type: 'uint16', byte_order: 'big_endian' },
        { area: 'holding_registers', address: 200, values: [1, 2, 3], data_type: 'uint16', byte_order: 'big_endian', unit_id: 0 },
      ],
    },
  },

  'modbus-read': {
    description:
      'MODBUS 에이전트(Client/Server)에 연결하여 레지스터를 읽는 노드입니다. config의 command_set(ReadOp 배열)이 기본값이며, 입력 메시지 payload에 command_set이 있으면 런타임에 오버라이드됩니다.',
    ports: [
      { name: 'in', direction: 'input', description: '읽기를 트리거하는 메시지를 수신합니다. payload에 command_set이 있으면 config 기본값을 오버라이드합니다.' },
      { name: 'out', direction: 'output', description: '읽기 결과를 출력합니다.' },
      { name: 'error', direction: 'error', description: '에이전트 통신 실패 등 에러 발생 시 출력합니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '대상 MODBUS 에이전트(Client/Server)의 이름 또는 ID입니다.',
      },
      {
        name: 'command_set',
        type: 'json',
        required: false,
        description: '읽기 명령(ReadOp) 배열입니다. 각 항목: area, address, count, data_type, byte_order, unit_id(0=공유). config 기본값이며 입력 payload의 command_set으로 오버라이드됩니다.',
      },
    ],
    configExample: {
      agent_ref: 'modbus-gateway-1',
      command_set: [
        { area: 'holding_registers', address: 0, count: 10, data_type: 'float32', byte_order: 'big_endian' },
        { area: 'coils', address: 0, count: 8, unit_id: 1 },
      ],
    },
  },

  'modbus-control': {
    description:
      'MODBUS 에이전트(Client/Server)를 제어하는 노드입니다(start/stop/pause/resume/reconnect/add_device/remove_device/set_config/command). config의 command_set(ControlOp 배열)이 기본값이며, 입력 메시지 payload에 command_set이 있으면 런타임에 오버라이드됩니다.',
    ports: [
      { name: 'in', direction: 'input', description: '제어를 트리거하는 메시지를 수신합니다. payload에 command_set이 있으면 config 기본값을 오버라이드합니다.' },
      { name: 'out', direction: 'output', description: '제어 결과를 출력합니다.' },
      { name: 'error', direction: 'error', description: '제어 실패 등 에러 발생 시 출력합니다.' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '제어할 MODBUS 에이전트(Client/Server)의 이름 또는 ID입니다.',
      },
      {
        name: 'command_set',
        type: 'json',
        required: false,
        description: '제어 명령(ControlOp) 배열입니다. 각 항목: action, params(자유형 객체, add_device/remove_device/set_config/command 용). config 기본값이며 입력 payload의 command_set으로 오버라이드됩니다.',
      },
    ],
    configExample: {
      agent_ref: 'modbus-gateway-1',
      command_set: [
        { action: 'reconnect' },
        { action: 'add_device', params: { unit_id: 5, host: '10.0.0.9', port: 502 } },
      ],
    },
  },

  'modbus-remap': {
    description:
      'MODBUS 레지스터 리매퍼 노드입니다. 에이전트와 통신하지 않고, modbus-read 출력 payload({success, values[], agent_type})의 레지스터를 rules/templates 규칙에 따라 재매핑하여 modbus-write 호환 payload({success, values[], errors?, agent_type})로 변환합니다. rules는 From→To 개별 규칙, templates는 정의+적용 축약형입니다.',
    ports: [
      { name: 'in', direction: 'input', description: 'modbus-read 출력 메시지를 수신합니다. payload에 success, values 배열, agent_type이 포함됩니다.' },
      { name: 'out', direction: 'output', description: '재매핑된 modbus-write 호환 메시지를 출력합니다.' },
      { name: 'error', direction: 'error', description: 'count 불일치 등 재매핑 실패 시 에러 메시지를 출력합니다.' },
    ],
    configFields: [
      {
        name: 'rules',
        type: 'json',
        required: false,
        description: '개별 재매핑 규칙(From→To 팬아웃) 배열입니다. 각 항목: source_unit_id(선택), source_area, source_address, count, targets[](최소 1개; 각 target 은 target_unit_id, target_area(선택; 생략 시 source_area 유지), target_address). count는 대응 read 항목의 count와 정확히 일치해야 합니다. legacy 최상위 단일 target_* 형식도 로드 가능하며 targets 배열로 정규화됩니다.',
      },
      {
        name: 'templates',
        type: 'json',
        required: false,
        description: '축약형 규칙 배열입니다. 각 항목: source_unit_id(선택), area, offset(target_address=start+offset, 음수 허용), device_id(→target_unit_id), start(→source_address), count, target_area(선택; 생략 시 area 유지).',
      },
    ],
    configExample: {
      rules: [
        {
          source_unit_id: 1,
          source_area: 'holding_registers',
          source_address: 0,
          count: 10,
          targets: [
            { target_unit_id: 2, target_address: 100 },
            { target_unit_id: 3, target_area: 'input_registers', target_address: 200 },
          ],
        },
      ],
      templates: [
        {
          name: 'sensor-block',
          rules: [
            {
              source_area: 'holding_registers',
              source_offset: 0,
              count: 8,
              targets: [{ target_offset: 100, target_unit_offset: 0 }],
            },
            {
              source_area: 'input_registers',
              source_offset: 8,
              count: 4,
              targets: [{ target_area: 'holding_registers', target_offset: 200, target_unit_offset: 1 }],
            },
          ],
        },
      ],
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
        name: 'namespace',
        type: 'string',
        required: false,
        description: 'Store 네임스페이스입니다 (기본값: "default").',
        default: 'default',
      },
      {
        name: 'key_template',
        type: 'string',
        required: true,
        description: '키 템플릿입니다. {field} 형식 플레이스홀더를 payload 값으로 치환합니다 (예: "{location}:{point}"). 이 키에 metrics 의 각 메트릭이 metric_type 별 시리즈로 기록됩니다.',
      },
      {
        name: 'key_mappings',
        type: 'object',
        required: false,
        description: '한 메시지에서 서로 다른 키에 값을 기록합니다(키 템플릿 → 값 $.경로). 네임스페이스·TTL·태그는 공유 적용됩니다.',
      },
      {
        name: 'tags',
        type: 'object',
        required: false,
        description: '모든 메트릭/키에 공유 적용되는 태그(키=값). 값은 리터럴 또는 $. 경로.',
      },
      {
        name: 'metrics',
        type: 'array',
        required: false,
        description:
          '다중 메트릭 배열. 각 항목은 { metric_type, value_key(기본 $.payload.value), data_type, min_interval, min_change, min_change_percent } 를 가집니다. 같은 key_template 키에 metric_type 별 시리즈로 저장되며, 메트릭마다 독립적인 미세변화 억제(dead-band)가 적용됩니다. min_interval 이 설정된 메트릭만 억제되고, 간격 경과 시 변화가 없어도 1건 저장(heartbeat)합니다.',
      },
      {
        name: 'ttl',
        type: 'string',
        required: false,
        description: 'TTL 기간입니다 (예: "5m", "1h", "24h"). 모든 메트릭/키에 공유 적용. 비워두면 만료 없음.',
      },
    ],
    configExample: {
      agent_ref: 'store-engine',
      namespace: 'sensors',
      key_template: '{location}:{device_id}',
      ttl: '1h',
      metrics: [
        { metric_type: 'temperature', value_key: '$.payload.temperature', data_type: 'float', min_interval: '30s', min_change: 0.5 },
        { metric_type: 'humidity', value_key: '$.payload.humidity', data_type: 'float', min_interval: '1m', min_change: 2 },
      ],
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
      agent_ref: 'serial-lg_hvacr02',
    },
    outputExamples: {
      out: {
        payload: {
          raw: '[]byte (바이너리 원본)',
          data: '562d04445500670445500000204...',
        },
        metadata: { agent: { type: 'serial', id: 'agent-1' }, node_id: 'node-abc-123' },
      },
      raw_out: {
        payload: { raw: '[]byte (프레이밍 이전 원본)' },
        metadata: { agent: { type: 'serial', id: 'agent-1' }, node_id: 'node-abc-123', port: 'raw_out' },
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
      agent_ref: 'serial-lg_hvacr02',
    },
    outputExamples: {
      in_example: {
        _comment: '입력 메시지 예시 (전송 우선순위: raw → data → JSON)',
        payload: { raw: '[56 2d 04 44 55 ...]' },
      },
      out: {
        payload: { raw: '[56 2d 04 44 55 ...]', data: 'V-\\u0004DU...' },
        metadata: { agent: { type: 'serial', id: 'agent-1' }, node_id: 'node-abc-123' },
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
        metadata: { agent: { type: 'tcp-server', id: 'agent-1' }, 'tcp.remote_addr': '192.168.1.100:5678', 'tcp.node_id': 'node-abc', 'tcp.agent_type': 'tcp-server' },
      },
      'out (tcp-client)': {
        payload: {
          raw: '[]byte (바이너리 원본)',
          data: '48656c6c6f2066726f6d20736572766572',
        },
        metadata: { agent: { type: 'tcp-client', id: 'agent-1' }, 'tcp.node_id': 'node-abc', 'tcp.agent_type': 'tcp-client' },
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
        metadata: { agent: { type: 'tcp-server', id: 'agent-1' }, 'tcp.node_id': 'node-abc', 'tcp.remote_addr': '192.168.1.100:5678' },
      },
    },
  },

  'lg-hvacr02-status': {
    description:
      'LG HVACR-02 에이전트에 연결하여 RS-485 버스에서 캡처된 실내기 상태를 조회하는 노드입니다. 주소를 지정하면 해당 실내기만, 미지정 시 전체 실내기를 조회합니다. poll_interval 설정 시 주기적으로 자동 폴링합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '상태 조회 트리거. payload.address로 실내기 지정 가능' },
      { name: 'out', direction: 'output', description: '조회 결과 출력. get_stats 또는 get_recent 응답' },
      { name: 'error', direction: 'error', description: '에이전트 통신 실패 시 출력' },
    ],
    configFields: [
      { name: 'agent_ref', type: 'string', required: true, description: '연결할 LG HVACR-02 에이전트의 이름 또는 ID' },
      { name: 'default_address', type: 'string', required: false, description: '기본 실내기 주소 (예: 01)' },
      { name: 'poll_interval', type: 'string', required: false, description: '자동 폴링 주기 (예: 10s, 1m)', default: '30s' },
      { name: 'timeout', type: 'string', required: false, description: 'Agent Process 타임아웃', default: '5s' },
      { name: 'poll_command', type: 'string', required: false, description: '폴링 명령 (get_stats 또는 get_recent)', default: 'get_stats' },
      { name: 'recent_count', type: 'number', required: false, description: 'get_recent 시 최근 데이터 수', default: '10' },
    ],
    configExample: {
      agent_ref: 'lg_hvacr02-capture',
      default_address: '67',
      poll_interval: '10s',
      poll_command: 'get_stats',
    },
  },

  'lg-hvacr02-control': {
    description:
      'LG ICP-02 프로토콜로 실내기를 제어하는 노드입니다. 전원, 온도, 풍량, 운전모드를 설정합니다. control_enabled가 활성화된 LG HVACR-02 에이전트가 필요합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '제어 명령 입력. payload: {address, command, ...params}' },
      { name: 'out', direction: 'output', description: '제어 결과 출력' },
      { name: 'error', direction: 'error', description: '제어 실패 시 출력' },
    ],
    configFields: [
      { name: 'agent_ref', type: 'string', required: true, description: '연결할 LG HVACR-02 에이전트 (control_enabled 필요)' },
      { name: 'default_address', type: 'string', required: false, description: '기본 실내기 주소 (예: 67)' },
      { name: 'timeout', type: 'string', required: false, description: 'Agent Process 타임아웃', default: '5s' },
    ],
    configExample: {
      agent_ref: 'lg_hvacr02-control',
      default_address: '67',
      timeout: '5s',
    },
  },

  'lg-hvacr02': {
    description:
      'LG HVACR-02 실내기 상태 조회 + 제어 통합 노드입니다. 입력 메시지에 제어 키(power, temperature, fan_speed, mode)가 있으면 제어, 없으면 상태 조회로 동작합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '상태 조회 또는 제어 명령. 제어 키 유무에 따라 자동 분기' },
      { name: 'out', direction: 'output', description: '상태 또는 제어 결과 출력' },
      { name: 'error', direction: 'error', description: '에러 시 출력' },
    ],
    configFields: [
      { name: 'agent_ref', type: 'string', required: true, description: '연결할 LG HVACR-02 에이전트' },
      { name: 'default_address', type: 'string', required: false, description: '기본 실내기 주소' },
      { name: 'poll_interval', type: 'string', required: false, description: '자동 폴링 주기', default: '30s' },
      { name: 'timeout', type: 'string', required: false, description: 'Agent Process 타임아웃', default: '5s' },
      { name: 'poll_command', type: 'string', required: false, description: '폴링 명령', default: 'get_stats' },
      { name: 'recent_count', type: 'number', required: false, description: 'get_recent 시 최근 데이터 수', default: '10' },
    ],
    configExample: {
      agent_ref: 'lg_hvacr02-capture',
      default_address: '67',
      poll_interval: '15s',
    },
  },

  'lg-hvacr01-status': {
    description:
      'LG ICP-01 프로토콜로 에어컨 상태를 push 수신하는 노드입니다. 에이전트가 NotifyInterval 마다 TYPE-A(ODU)/TYPE-B(IDU) 프레임을 emit, 노드는 ring buffer drain. 무수신 임계 시간(inactivity_timeout) 초과 시 request_state 자동 전송. unit_id(STX hex)로 ODU/IDU 단독 필터링 가능.',
    ports: [
      { name: 'in', direction: 'input', description: '상태 수신 트리거(즉시 drain). payload 키는 무시됩니다.' },
      { name: 'out', direction: 'output', description: '캡처된 디바이스 상태를 디바이스별 개별 메시지로 출력 (type=device_state)' },
      { name: 'error', direction: 'error', description: '에러 시 출력' },
    ],
    configFields: [
      { name: 'agent_ref', type: 'string', required: true, description: '연결할 LG HVACR-01 에이전트' },
      { name: 'inactivity_timeout', type: 'string', required: false, description: '무수신 임계 시간 (예: "90s"). 에이전트 NotifyInterval 의 1.5~2배 권장.', default: '90s' },
      { name: 'timeout', type: 'string', required: false, description: 'Agent Process 타임아웃', default: '5s' },
      { name: 'batch_size', type: 'number', required: false, description: 'drain 시 한 번에 가져올 최대 프레임 수', default: '32' },
      { name: 'unit_id', type: 'string', required: false, description: '(고급) LG ICP-01 STX hex (예: ODU="58", IDU="81" ~ "BF"). 비우면 전체.' },
    ],
    configExample: {
      agent_ref: 'lg_hvacr01-capture',
      inactivity_timeout: '90s',
      timeout: '5s',
      batch_size: 32,
    },
  },

  'lg-hvacr01-control': {
    description:
      'LG HVACR-01 디바이스 제어 노드입니다. 현재 LG ICP-01 프로토콜의 쓰기 명령이 확인되지 않아 모든 제어 요청에 미지원 응답을 반환합니다.',
    ports: [
      { name: 'in', direction: 'input', description: '제어 명령 입력' },
      { name: 'out', direction: 'output', description: '미지원 응답 출력' },
      { name: 'error', direction: 'error', description: '에러 시 출력' },
    ],
    configFields: [
      { name: 'agent_ref', type: 'string', required: true, description: '연결할 LG HVACR-01 에이전트' },
      { name: 'timeout', type: 'string', required: false, description: 'Agent Process 타임아웃', default: '5s' },
    ],
    configExample: {
      agent_ref: 'lg_hvacr01-capture',
    },
  },

  'lg-hvacr01': {
    description:
      'LG HVACR-01 상태 수신 + 제어 통합 노드입니다. 입력 메시지에 제어 키가 있으면 미지원 응답을 반환하고, 없으면 즉시 drain. push 모델로 무수신 임계 시간 초과 시 request_state 자동 전송.',
    ports: [
      { name: 'in', direction: 'input', description: '상태 수신 트리거 또는 제어 명령' },
      { name: 'out', direction: 'output', description: '상태 또는 제어 결과 출력' },
      { name: 'error', direction: 'error', description: '에러 시 출력' },
    ],
    configFields: [
      { name: 'agent_ref', type: 'string', required: true, description: '연결할 LG HVACR-01 에이전트' },
      { name: 'inactivity_timeout', type: 'string', required: false, description: '무수신 임계 시간 (예: "90s")', default: '90s' },
      { name: 'timeout', type: 'string', required: false, description: 'Agent Process 타임아웃', default: '5s' },
      { name: 'batch_size', type: 'number', required: false, description: 'drain 시 한 번에 가져올 최대 프레임 수', default: '32' },
      { name: 'unit_id', type: 'string', required: false, description: '(고급) LG ICP-01 STX hex (예: ODU="58", IDU="81" ~ "BF").' },
    ],
    configExample: {
      agent_ref: 'lg_hvacr01-capture',
      inactivity_timeout: '90s',
      timeout: '5s',
      batch_size: 32,
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

  'chirpstack-in': {
    description:
      'ChirpStack LoRaWAN 업링크를 수신하는 소스 노드입니다. 에이전트가 measurement 당 1개로 fan-out 한 레코드를 그대로 플로우 메시지로 방출하므로, 하류에서 별도의 분해(split) 없이 store/influx 소비자에 직접 연결할 수 있습니다. 에이전트에 emit_comm_state 가 켜져 있으면 통신 상태 변화 시 device_state.* 메시지도 같은 출력 포트로 방출됩니다.',
    ports: [
      { name: 'out', direction: 'output', description: 'measurement 당 1건의 업링크 메시지 출력 (통신 상태 변화 시 device_state.* 메시지 포함)' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '연결할 ChirpStack 에이전트의 이름 또는 ID입니다.',
      },
      {
        name: 'emit_node_id',
        type: 'boolean',
        required: false,
        description: '메시지 metadata 에 emit 한 flow 노드 UUID 를 포함합니다.',
        default: 'false',
      },
      {
        name: 'emit_agent',
        type: 'boolean',
        required: false,
        description: '메시지 metadata 에 agent:{type,id} 그룹을 포함합니다.',
        default: 'true',
      },
      {
        name: 'emit_device',
        type: 'boolean',
        required: false,
        description: '메시지 metadata 에 device:{type,id} 그룹을 포함합니다 (unit_id=devEui 승격).',
        default: 'true',
      },
    ],
    configExample: {
      agent_ref: 'chirpstack-lora',
    },
    outputExamples: {
      out: {
        type: 'event',
        timestamp: 1765000000000,
        payload: { value: 23.4 },
        metadata: {
          measurement: 'temperature',
          tags: { application_name: 'site-a', device_profile_name: 'Milesight WS301' },
          device: { id: 'a1b2c3d4e5f60718', name: 'ws301-office-01' },
          agent: { type: 'chirpstack', id: 'agent-cs-1' },
        },
      },
      'out (device_state)': {
        _comment: '에이전트 emit_comm_state=true 일 때 통신 상태 변화 시 같은 포트로 방출',
        type: 'device_state.report',
        payload: {
          last_seen_ms: 1765000000000,
          state: { online: true, rssi: -87, snr: 8.5, gateway_id: 'gw-0001', last_seen_ms: 1765000000000 },
        },
        metadata: { device: { id: 'a1b2c3d4e5f60718' }, agent: { type: 'chirpstack', id: 'agent-cs-1' } },
      },
    },
  },

  'chirpstack-control': {
    description:
      'ChirpStack LoRaWAN 다운링크를 전송하는 제어 노드입니다. 입력 payload 의 command 를 대상 디바이스의 deviceProfile 코덱으로 인코딩하여 application/{applicationId}/device/{devEui}/command/down 토픽에 발행합니다. 대상 디바이스는 설정이 아니라 입력 메시지의 unit_id 로 지정되므로 노드 1개가 N개 디바이스를 담당합니다. 전제조건: applicationId 는 해당 devEui 의 최초 업링크에서 캐시되므로 최초 업링크 수신 이후에만 제어가 가능합니다. 등록된 deviceProfile 코덱만 허용되며(v1: Milesight WS301 — reboot / set_report_interval / query_device_status), 미등록 프로파일이나 알 수 없는 명령은 발행 없이 에러 포트로 전달됩니다.',
    ports: [
      { name: 'in', direction: 'input', description: '다운링크 명령 입력 (unit_id + command + params)' },
      { name: 'out', direction: 'output', description: '발행 완료 후 원본 메시지를 type=response 로 패스스루 (chirpstack_command / chirpstack_downlink_topic 메타데이터 추가)' },
      { name: 'error', direction: 'error', description: '미등록 코덱, 알 수 없는 command, applicationId 미캐시, 발행 실패 시 출력' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '다운링크를 발행할 ChirpStack 에이전트의 이름 또는 ID입니다.',
      },
      {
        name: 'emit_node_id',
        type: 'boolean',
        required: false,
        description: '메시지 metadata 에 emit 한 flow 노드 UUID 를 포함합니다.',
        default: 'false',
      },
      {
        name: 'emit_agent',
        type: 'boolean',
        required: false,
        description: '메시지 metadata 에 agent:{type,id} 그룹을 포함합니다.',
        default: 'true',
      },
    ],
    configExample: {
      agent_ref: 'chirpstack-lora',
    },
    inputExamples: {
      in: {
        payload: {
          unit_id: 'a1b2c3d4e5f60718',
          command: 'set_report_interval',
          params: { interval: 1200 },
        },
      },
    },
    outputExamples: {
      out: {
        type: 'response',
        payload: { unit_id: 'a1b2c3d4e5f60718', command: 'set_report_interval', params: { interval: 1200 } },
        metadata: {
          chirpstack_command: 'set_report_interval',
          chirpstack_downlink_topic: 'application/12/device/a1b2c3d4e5f60718/command/down',
          agent: { type: 'chirpstack', id: 'agent-cs-1' },
        },
      },
    },
  },

  'chirpstack-status': {
    description:
      'ChirpStack 에이전트에 캐시된 마지막 통신 상태를 조회하는 읽기 전용 노드입니다. 입력 payload 의 unit_id(devEui)에 해당하는 캐시 항목을 읽어 입력 1건당 상태 메시지 1건을 방출하며, MQTT 발행도 온디맨드 폴링도 하지 않습니다. 전제조건: 대상 에이전트의 emit_comm_state 가 켜져 있어야 합니다. 꺼져 있으면 통신 상태 캐시가 채워지지 않아 항상 offline/unknown 이 방출됩니다(Init 시 1회 경고). 캐시에 항목이 없는 디바이스는 online=false 로 방출되며 online=true 를 조기 보고하지 않습니다.',
    ports: [
      { name: 'in', direction: 'input', description: '조회 트리거 입력 (unit_id 로 대상 devEui 지정)' },
      { name: 'out', direction: 'output', description: '캐시된 통신 상태를 type=device_state.* 메시지로 출력' },
      { name: 'error', direction: 'error', description: 'unit_id 누락, 에이전트 미초기화, 상태 메시지 빌드 실패 시 출력' },
    ],
    configFields: [
      {
        name: 'agent_ref',
        type: 'string',
        required: true,
        description: '통신 상태를 조회할 ChirpStack 에이전트의 이름 또는 ID입니다 (에이전트의 emit_comm_state 활성 필요).',
      },
      {
        name: 'emit_node_id',
        type: 'boolean',
        required: false,
        description: '메시지 metadata 에 emit 한 flow 노드 UUID 를 포함합니다.',
        default: 'false',
      },
      {
        name: 'emit_agent',
        type: 'boolean',
        required: false,
        description: '메시지 metadata 에 agent:{type,id} 그룹을 포함합니다.',
        default: 'true',
      },
      {
        name: 'emit_device',
        type: 'boolean',
        required: false,
        description: '메시지 metadata 에 device:{type,id} 그룹을 포함합니다 (unit_id=devEui 승격).',
        default: 'true',
      },
    ],
    configExample: {
      agent_ref: 'chirpstack-lora',
    },
    inputExamples: {
      in: {
        payload: { unit_id: 'a1b2c3d4e5f60718' },
      },
    },
    outputExamples: {
      out: {
        type: 'device_state.report',
        payload: {
          last_seen_ms: 1765000000000,
          state: { online: true, rssi: -87, snr: 8.5, gateway_id: 'gw-0001', last_seen_ms: 1765000000000 },
        },
        metadata: {
          device: { id: 'a1b2c3d4e5f60718', name: 'ws301-office-01' },
          agent: { type: 'chirpstack', id: 'agent-cs-1' },
        },
      },
      'out (캐시 없음)': {
        _comment: '캐시에 항목이 없는 devEui — offline/unknown 으로 방출 (online=true 조기 보고 없음)',
        type: 'device_state.report',
        payload: {
          last_seen_ms: 0,
          state: { online: false, rssi: 0, snr: 0, gateway_id: '', last_seen_ms: 0 },
        },
      },
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
        meta: { source: 'modbus-read' },
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

  inventory: {
    description:
      '디바이스/에이전트/노드/플로우 인벤토리 스냅샷을 emit 합니다. trigger 노드와 체이닝하여 주기적 상태 동기화에 사용합니다. condition(조건식 필터), fields(필드 화이트리스트), max_items(청크 분할) 로 출력을 다듬을 수 있습니다.',
    ports: [
      { name: 'in', direction: 'input', description: '스냅샷을 트리거하는 입력 메시지 (payload 무시). 입력 metadata 는 출력에 얕은 복사로 보존됩니다.' },
      { name: 'out', direction: 'output', description: 'payload { items: [...] } 형태의 스냅샷 메시지. max_items 분할 시 여러 메시지로 출력됩니다.' },
      { name: 'error', direction: 'error', description: '인벤토리 수집/조건식 평가 중 에러 발생 시 출력' },
    ],
    configFields: [
      {
        name: 'source',
        type: 'select',
        required: true,
        description: '스냅샷 대상 인벤토리 종류: devices | agents | nodes | flows',
      },
      {
        name: 'condition',
        type: 'multiline',
        required: false,
        description:
          'filter 노드와 동일한 조건식 문법으로 항목을 필터링합니다. 각 항목을 메시지로 감싸 평가하므로 항목 필드는 $.payload.<필드> 로 참조합니다 (예: $.payload.online == true, exists($.payload.uid)). == != > < >= <=, && || !, exists(path) 지원. 비우면 전체 항목 출력. 모든 source 적용.',
      },
      {
        name: 'fields',
        type: 'string',
        required: false,
        description: '쉼표로 구분한 항목 필드 화이트리스트 (예: id, name, online). 비우면 전체 필드.',
      },
      {
        name: 'max_items',
        type: 'number',
        required: false,
        description: '메시지당 최대 항목 수. 0 이면 전체를 한 메시지로, N 이면 N 개씩 분할 출력.',
        default: '0',
      },
    ],
    configExample: {
      source: 'devices',
      condition: '$.payload.online == true',
      fields: 'id, name, online',
      max_items: 2,
    },
    inputExamples: {
      '트리거 (payload 무시 — trigger 노드 출력 연결)': {
        trigger_time: 1713312000000,
      },
    },
    outputExamples: {
      'out · max_items=2 첫 번째 청크 (devices)': {
        _comment:
          'payload.items 는 청크별 항목 배열. metadata.type 은 source 단수형, total_count/offset/count 는 모두 문자열.',
        payload: {
          items: [
            { id: 'lg_icp01:1', name: 'IDU-1', online: true },
            { id: 'lg_icp01:2', name: 'IDU-2', online: true },
          ],
        },
        metadata: { type: 'device', total_count: '5', offset: '0', count: '2' },
      },
      'out · max_items=0 전체를 한 메시지로 (agents)': {
        payload: {
          items: [
            { id: 'agent-mqtt', name: 'MQTT Bridge' },
            { id: 'agent-modbus', name: 'Modbus Poller' },
          ],
        },
        metadata: { type: 'agent', total_count: '2', offset: '0', count: '2' },
      },
    },
  },
};
