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
};
