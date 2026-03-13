// 에이전트 타입별 설정 스키마 정의.
// 백엔드 Transport.Options 키와 1:1 매핑된다.
// nodeSchemas.ts의 getBridgeConfigFields() 패턴을 따른다.

import type { ConfigField, ConfigSchema } from '@/types/node';

/** 백엔드에 등록된 에이전트 타입 목록 */
export const AGENT_TYPES = [
  { value: 'mqtt', label: 'MQTT' },
  { value: 'modbus-tcp', label: 'Modbus TCP' },
  { value: 'modbus-tcp-server', label: 'Modbus TCP Server' },
  { value: 'http', label: 'HTTP Receiver' },
  { value: 'http-sender', label: 'HTTP Sender' },
  { value: 'influxdb', label: 'InfluxDB' },
  { value: 'console-logger', label: 'Console Logger' },
  { value: 'samsung-nasa', label: 'Samsung NASA' },
] as const;

// ---- 타입별 ConfigField 정의 ----

const MQTT_FIELDS: ConfigField[] = [
  { name: 'broker', type: 'string', label: '브로커 주소', required: true, default: 'tcp://localhost:1883', description: 'MQTT 브로커 주소 (예: tcp://localhost:1883)' },
  { name: 'client_id', type: 'string', label: '클라이언트 ID', description: '빈 값이면 자동 생성' },
  { name: 'username', type: 'string', label: '사용자명' },
  { name: 'password', type: 'string', label: '비밀번호' },
  { name: 'topics', type: 'string', label: '구독 토픽', description: '쉼표로 구분 (예: sensor/+/data, device/#)' },
  { name: 'qos', type: 'select', label: 'QoS', options: ['0', '1', '2'], default: '1', description: '메시지 전달 보증 레벨' },
  { name: 'keep_alive_sec', type: 'number', label: 'Keep Alive (초)', default: 60 },
  { name: 'connect_timeout_sec', type: 'number', label: '연결 타임아웃 (초)', default: 10 },
  { name: 'auto_reconnect', type: 'boolean', label: '자동 재연결', default: true },
  { name: 'clean_session', type: 'boolean', label: '클린 세션', default: true },
  { name: 'buffer_size', type: 'number', label: '버퍼 크기', default: 256 },
];

const MODBUS_TCP_FIELDS: ConfigField[] = [
  { name: 'mode', type: 'select', label: '모드', options: ['interval', 'event'], default: 'interval', description: 'interval: 주기적 폴링, event: 변경 감지' },
  { name: 'read_mode', type: 'select', label: '읽기 모드', options: ['direct', 'cached'], default: 'cached' },
  { name: 'poll_interval', type: 'string', label: '폴링 간격', default: '5s', description: 'Go duration 형식 (예: 5s, 1m)' },
  { name: 'reconnect_interval', type: 'string', label: '재연결 간격', default: '10s' },
  { name: 'request_timeout', type: 'string', label: '요청 타임아웃', default: '3s' },
  { name: 'max_retries', type: 'number', label: '최대 재시도', default: 3 },
  { name: 'enable_write_events', type: 'boolean', label: '쓰기 이벤트', default: true },
  { name: 'devices', type: 'object', label: '디바이스 설정', required: true, description: '디바이스 배열 (JSON)' },
];

const MODBUS_TCP_SERVER_FIELDS: ConfigField[] = [
  { name: 'listen_address', type: 'string', label: '수신 주소', default: '0.0.0.0' },
  { name: 'listen_port', type: 'number', label: '수신 포트', default: 502, required: true, description: '범위: 1-65535' },
  { name: 'unit_id', type: 'number', label: '유닛 ID', default: 1, description: '범위: 0-247' },
  { name: 'max_connections', type: 'number', label: '최대 연결 수', default: 10 },
  { name: 'idle_timeout', type: 'string', label: '유휴 타임아웃', default: '60s' },
  { name: 'register_map', type: 'register_map', label: '레지스터 맵', required: true, description: '영역별 레지스터 세그먼트 설정' },
];

const HTTP_RECEIVER_FIELDS: ConfigField[] = [
  { name: 'listen_addr', type: 'string', label: '수신 주소', default: ':8080', required: true, description: '예: :8081' },
  { name: 'path', type: 'string', label: '수신 경로', default: '/' },
  { name: 'method', type: 'select', label: 'HTTP 메서드', options: ['GET', 'POST', 'PUT', 'PATCH'], default: 'POST' },
  { name: 'timeout_sec', type: 'number', label: '타임아웃 (초)', default: 30 },
  { name: 'content_type', type: 'string', label: 'Content-Type', description: '비어있으면 모두 허용' },
  { name: 'buffer_size', type: 'number', label: '버퍼 크기', default: 256 },
  { name: 'max_body_bytes', type: 'number', label: '최대 본문 크기 (bytes)', default: 1048576 },
];

const HTTP_SENDER_FIELDS: ConfigField[] = [
  { name: 'url', type: 'string', label: 'URL', required: true, description: '전송 대상 URL' },
  { name: 'method', type: 'select', label: 'HTTP 메서드', options: ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'], default: 'POST' },
  { name: 'content_type', type: 'string', label: 'Content-Type', default: 'application/json' },
  { name: 'timeout_sec', type: 'number', label: '타임아웃 (초)', default: 30 },
  { name: 'headers', type: 'object', label: '추가 헤더', description: 'HTTP 헤더 (JSON)' },
];

const INFLUXDB_FIELDS: ConfigField[] = [
  { name: 'url', type: 'string', label: 'URL', required: true, default: 'http://localhost:8086', description: 'InfluxDB 서버 주소' },
  { name: 'token', type: 'string', label: '인증 토큰', required: true },
  { name: 'org', type: 'string', label: '조직', description: 'v2에서 필수, v3에서 선택' },
  { name: 'bucket', type: 'string', label: '버킷', required: true },
  { name: 'version', type: 'select', label: 'InfluxDB 버전', options: ['2', '3'], required: true },
  { name: 'precision', type: 'select', label: '타임스탬프 정밀도', options: ['ns', 'us', 'ms', 's'], default: 'ns' },
  { name: 'batch_size', type: 'number', label: '배치 크기', default: 1000 },
  { name: 'flush_interval_ms', type: 'number', label: '플러시 간격 (ms)', default: 1000 },
  { name: 'query_language', type: 'select', label: '쿼리 언어', options: ['flux', 'influxql', 'sql'] },
  { name: 'timeout_sec', type: 'number', label: '타임아웃 (초)', default: 10 },
  { name: 'buffer_size', type: 'number', label: '버퍼 크기', default: 256 },
];

const CONSOLE_LOGGER_FIELDS: ConfigField[] = [
  { name: 'prefix', type: 'string', label: '접두어', default: '[console-logger]', description: '로그 출력 시 접두어' },
  { name: 'level', type: 'select', label: '로그 레벨', options: ['debug', 'info', 'warn', 'error'], default: 'info' },
];

const SAMSUNG_NASA_FIELDS: ConfigField[] = [
  { name: 'transport_type', type: 'select', label: '전송 방식', options: ['serial', 'tcp'], required: true },
  { name: 'serial_port', type: 'string', label: '시리얼 포트', description: 'serial 모드 시 (예: /dev/ttyUSB0)' },
  { name: 'baud_rate', type: 'number', label: '보 레이트', default: 9600 },
  { name: 'tcp_addr', type: 'string', label: 'TCP 주소', description: 'tcp 모드 시 (예: 192.168.1.100:502)' },
  { name: 'poll_interval', type: 'string', label: '폴링 간격', default: '30s' },
];

/** 에이전트 타입별 설정 스키마 레지스트리 */
const AGENT_CONFIG_SCHEMAS: Record<string, ConfigField[]> = {
  'mqtt': MQTT_FIELDS,
  'modbus-tcp': MODBUS_TCP_FIELDS,
  'modbus-tcp-server': MODBUS_TCP_SERVER_FIELDS,
  'http': HTTP_RECEIVER_FIELDS,
  'http-sender': HTTP_SENDER_FIELDS,
  'influxdb': INFLUXDB_FIELDS,
  'console-logger': CONSOLE_LOGGER_FIELDS,
  'samsung-nasa': SAMSUNG_NASA_FIELDS,
};

/**
 * 에이전트 타입에 해당하는 ConfigSchema를 반환한다.
 * 미등록 타입이면 undefined를 반환하고, DynamicForm이 key-value 모드로 폴백한다.
 */
export function getAgentConfigSchema(agentType: string): ConfigSchema | undefined {
  const fields = AGENT_CONFIG_SCHEMAS[agentType];
  if (!fields) return undefined;
  return { fields };
}

/**
 * 에이전트 타입의 기본 설정 값을 반환한다.
 * 생성 모달에서 폼 초기화 시 사용한다.
 */
export function getAgentConfigDefaults(agentType: string): Record<string, unknown> {
  const fields = AGENT_CONFIG_SCHEMAS[agentType];
  if (!fields) return {};
  const defaults: Record<string, unknown> = {};
  for (const field of fields) {
    if (field.default !== undefined) {
      defaults[field.name] = field.default;
    }
  }
  return defaults;
}
