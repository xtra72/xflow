// 에이전트 타입별 설정 스키마 정의.
// 백엔드 Transport.Options 키와 1:1 매핑된다.
// nodeSchemas.ts의 getBridgeConfigFields() 패턴을 따른다.

import type { ConfigField, ConfigSchema } from '@/types/node';

/** 백엔드에 등록된 에이전트 타입 목록 */
export const AGENT_TYPES = [
  { value: 'mqtt-client', label: 'MQTT' },
  { value: 'modbus-tcp', label: 'Modbus TCP' },
  { value: 'modbus-tcp-server', label: 'Modbus TCP Server' },
  { value: 'http', label: 'HTTP Receiver' },
  { value: 'http-sender', label: 'HTTP Sender' },
  { value: 'influxdb', label: 'InfluxDB' },
  { value: 'logger', label: 'Logger' },
  { value: 'samsung-nasa', label: 'Samsung NASA' },
  { value: 'lgap', label: 'LG LGAP' },
  { value: 'lgcp', label: 'LG LGCP Capture' },
  { value: 'store', label: 'Store' },
  { value: 'serial', label: 'Serial' },
  { value: 'tcp-server', label: 'TCP Server' },
  { value: 'tcp-client', label: 'TCP Client' },
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
  { name: 'max_pub_topics', type: 'number', label: '발행 토픽 최대 추적 수', default: 100, description: '초과 시 가장 오래된 토픽 삭제' },
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
  { name: 'max_connections', type: 'number', label: '최대 연결 수', default: 10 },
  { name: 'idle_timeout', type: 'string', label: '유휴 타임아웃', default: '60s' },
  // 디바이스(unit_id + register_map)는 디바이스 탭에서 관리
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
  // 출력 설정
  { name: 'output', type: 'select', label: '출력 대상', options: ['stdout', 'stderr', 'file'], default: 'stdout' },
  { name: 'output_path', type: 'string', label: '파일 경로', description: '예: /var/log/xflow/agent.log', visibleWhen: { field: 'output', value: 'file' } },
  { name: 'format', type: 'select', label: '출력 형식', options: ['text', 'json'], default: 'text' },
  { name: 'max_size', type: 'number', label: '최대 크기 (MB)', default: 10, visibleWhen: { field: 'output', value: 'file' } },
  { name: 'max_age', type: 'number', label: '보관 기간 (일)', default: 0, description: '0 = 무제한', visibleWhen: { field: 'output', value: 'file' } },
  { name: 'max_backups', type: 'number', label: '최대 백업 수', default: 0, description: '0 = 무제한', visibleWhen: { field: 'output', value: 'file' } },
  { name: 'compress', type: 'boolean', label: '백업 파일 gzip 압축', default: false, visibleWhen: { field: 'output', value: 'file' } },
  // 운영 설정
  { name: 'prefix', type: 'string', label: '접두어', default: '[logger]' },
];

const SAMSUNG_NASA_FIELDS: ConfigField[] = [
  // 연결 설정 (변경 시 재시작 필요)
  { name: 'transport_type', type: 'select', label: '연결 방식', options: ['serial', 'tcp'], required: true },
  { name: 'serial_port', type: 'string', label: '시리얼 포트', required: true, description: '예: /dev/ttyUSB0', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'baud_rate', type: 'number', label: '보 레이트', default: 9600, visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'data_bits', type: 'number', label: '데이터 비트', default: 8, visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'stop_bits', type: 'number', label: '스톱 비트', default: 1, visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'parity', type: 'select', label: '패리티', options: ['none', 'even', 'odd'], default: 'even', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'tcp_addr', type: 'string', label: 'TCP 주소', required: true, description: '예: 192.168.1.100:502', visibleWhen: { field: 'transport_type', value: 'tcp' } },
  // 즉시 적용 설정
  { name: 'poll_interval', type: 'string', label: '상태 확인 요청 간격', default: '30s' },
  { name: 'buzzer_on_control', type: 'boolean', label: '제어 시 부저', default: false },
  { name: 'notify_on_change', type: 'boolean', label: '상태 변경 알람 전송', default: false },
  { name: 'auto_discovery', type: 'boolean', label: '자동 디바이스 발견', default: true },
];

const LG_LGAP_FIELDS: ConfigField[] = [
  { name: 'transport_type', type: 'select', label: '연결 방식', options: ['serial'], required: true, default: 'serial', description: 'RS-485 시리얼 통신' },
  { name: 'serial_port', type: 'string', label: '시리얼 포트', description: '시리얼 포트 (예: /dev/ttyUSB0)' },
  { name: 'baud_rate', type: 'number', label: '보 레이트', default: 4800, description: '통신 속도 (LGAP 기본값: 4800)' },
  { name: 'poll_interval', type: 'string', label: '폴링 간격', default: '30s', description: '폴링 간격 (예: 30s, 1m)' },
  { name: 'devices', type: 'string', label: '디바이스 목록', description: '디바이스 목록 (address, name)' },
  { name: 'connect_timeout', type: 'string', label: '연결 타임아웃', default: '5s', description: '연결 타임아웃' },
  { name: 'read_timeout', type: 'string', label: '읽기 타임아웃', default: '500ms', description: '읽기 타임아웃' },
  { name: 'inter_command_delay', type: 'string', label: '명령 간 딜레이', default: '50ms', description: '명령 간 딜레이' },
  { name: 'reconnect_interval', type: 'string', label: '재연결 간격', default: '5s', description: '재연결 기본 간격' },
  { name: 'max_reconnect_backoff', type: 'string', label: '최대 재연결 백오프', default: '5m', description: '재연결 최대 백오프' },
];

const LG_LGCP_FIELDS: ConfigField[] = [
  { name: 'serial_port', type: 'string', label: '시리얼 포트', required: true, description: 'RS-485 시리얼 포트 경로 (예: /dev/ttyUSB1)' },
  { name: 'baud_rate', type: 'number', label: '통신 속도 (Baud Rate)', default: 9600, description: 'LGCP 기본값 9600bps' },
  { name: 'data_bits', type: 'number', label: '데이터 비트', default: 8, description: '데이터 비트 수 (기본: 8)' },
  { name: 'stop_bits', type: 'number', label: '스톱 비트', default: 1, description: '스톱 비트 수 (기본: 1)' },
  { name: 'parity', type: 'select', label: '패리티', options: ['none', 'even', 'odd'], default: 'none', description: '패리티 검사 방식' },
  { name: 'read_timeout', type: 'string', label: '읽기 타임아웃', default: '500ms', description: '시리얼 읽기 대기 시간' },
  { name: 'verify_crc', type: 'boolean', label: 'CRC 검증 활성화', default: true, description: 'CRC-16/XMODEM 무결성 검증' },
  { name: 'auto_discovery', type: 'boolean', label: '자동 디바이스 발견', default: true, description: '버스에서 새 디바이스 자동 등록' },
  { name: 'devices', type: 'string', label: '사전 등록 디바이스', description: '설정 기반 디바이스 목록 (address, name)' },
  { name: 'notify_interval', type: 'string', label: '상태 보고 주기', default: '', description: '주기적 상태 보고 간격 (예: 30s). 미설정 시 변경 시에만 보고' },
  { name: 'reconnect_interval', type: 'string', label: '재연결 간격', default: '5s', description: '연결 끊김 시 재시도 간격' },
  { name: 'max_reconnect_backoff', type: 'string', label: '최대 재연결 대기', default: '5m', description: '재연결 백오프 상한' },
  { name: 'msg_channel_size', type: 'number', label: '메시지 버퍼 크기', default: 256, description: '내부 메시지 채널 버퍼' },
  { name: 'control_enabled', type: 'boolean', label: '제어 기능 활성화', default: false, description: '실내기 능동 제어 기능 (전원, 온도, 풍량, 모드)' },
  { name: 'controller_address', type: 'string', label: '컨트롤러 주소', default: '44550000', description: '컨트롤러 SA 주소 (8자리 HEX). control_enabled 시 필수' },
  { name: 'control_verify_timeout', type: 'string', label: '제어 검증 타임아웃', default: '3s', description: '제어 명령 후 상태 변경 확인 대기 시간' },
];

const SERIAL_FIELDS: ConfigField[] = [
  // 시리얼 포트 설정
  { name: 'port', type: 'string', label: '시리얼 포트', required: true, description: '예: /dev/ttyUSB0, COM3' },
  { name: 'baud_rate', type: 'select', label: '보 레이트', options: ['1200', '2400', '4800', '9600', '19200', '38400', '57600', '115200'], default: '9600' },
  { name: 'data_bits', type: 'select', label: '데이터 비트', options: ['5', '6', '7', '8'], default: '8' },
  { name: 'stop_bits', type: 'select', label: '스톱 비트', options: ['1', '2'], default: '1' },
  { name: 'parity', type: 'select', label: '패리티', options: ['none', 'even', 'odd', 'mark', 'space'], default: 'none' },
  // 통신 설정
  { name: 'read_timeout', type: 'string', label: '읽기 타임아웃', default: '1s', description: 'Go duration 형식 (예: 500ms, 1s)' },
  { name: 'buffer_size', type: 'number', label: '버퍼 크기 (바이트)', default: 4096 },
  { name: 'max_message_size', type: 'number', label: '최대 메시지 크기', default: 0, description: '0 = 무제한' },
  // 프레이밍 설정
  { name: 'framing', type: 'select', label: '프레이밍 모드', options: ['raw', 'newline', 'length_prefix', 'fixed_size', 'stream', 'frame'], default: 'raw', description: '수신 데이터 구분 방식' },
  { name: 'delimiter', type: 'number', label: '구분자 (바이트 값)', default: 10, description: '0x0A = LF, 0x0D = CR', visibleWhen: { field: 'framing', value: 'newline' } },
  { name: 'fixed_size', type: 'number', label: '고정 크기 (바이트)', description: '프레임당 고정 바이트 수', visibleWhen: { field: 'framing', value: 'fixed_size' } },
  { name: 'idle_timeout', type: 'string', label: '유휴 타임아웃', default: '50ms', description: '바이트 수신 중단 후 프레임 완료 대기', visibleWhen: { field: 'framing', value: 'stream' } },
  // frame 프레이밍 전용 설정
  { name: 'stx', type: 'string', label: 'STX (프레임 시작)', required: true, description: '16진수 문자열 (예: 02, 32, AA55)', visibleWhen: { field: 'framing', value: 'frame' } },
  { name: 'etx', type: 'string', label: 'ETX (프레임 종료)', description: '16진수 문자열 (예: 03, 34). 비어있으면 검증 생략', visibleWhen: { field: 'framing', value: 'frame' } },
  { name: 'length_offset', type: 'number', label: '길이 필드 오프셋', description: 'STX 시작부터 길이 필드까지 바이트 수. 미지정 시 STX 길이 사용', visibleWhen: { field: 'framing', value: 'frame' } },
  { name: 'length_size', type: 'select', label: '길이 필드 크기', options: ['1', '2'], default: '1', description: '길이 필드 바이트 수', visibleWhen: { field: 'framing', value: 'frame' } },
  { name: 'length_endian', type: 'select', label: '길이 필드 엔디안', options: ['big', 'little'], default: 'big', visibleWhen: { field: 'framing', value: 'frame' } },
  { name: 'length_includes_header', type: 'boolean', label: '길이에 헤더 포함', default: false, description: 'true: 길이 = 헤더+페이로드, false: 길이 = 페이로드만', visibleWhen: { field: 'framing', value: 'frame' } },
  { name: 'length_adjustment', type: 'number', label: '길이 보정값', default: 0, description: '디코딩된 길이에 더할 보정값 (예: NASA 프로토콜은 -1)', visibleWhen: { field: 'framing', value: 'frame' } },
  { name: 'checksum', type: 'select', label: '체크섬', options: ['none', 'sum8', 'xor'], default: 'none', description: '프레임 끝 1바이트 체크섬 검증', visibleWhen: { field: 'framing', value: 'frame' } },
];

const STORE_FIELDS: ConfigField[] = [
  { name: 'max_key_length', type: 'number', label: '최대 키 길이 (바이트)', default: 512, description: '키 문자열 최대 바이트 수' },
  { name: 'scan_interval', type: 'string', label: 'TTL 스캔 간격', default: '30s', description: '만료 키 정리 주기 (예: 30s, 1m)' },
  { name: 'default_ttl', type: 'string', label: '기본 TTL', description: '키 기본 만료 시간 (예: 1h). 미설정 시 만료 없음' },
  { name: 'max_history_size', type: 'number', label: '히스토리 최대 갯수', default: 0, description: '키당 보관할 이전 값 최대 수 (0: 비활성화)' },
  { name: 'history_ttl', type: 'string', label: '히스토리 보관 시간', description: '히스토리 항목 보관 기간 (예: 30m, 1h). 미설정 시 시간 제한 없음' },
];

/** 에이전트 타입별 설정 스키마 레지스트리 */
const AGENT_CONFIG_SCHEMAS: Record<string, ConfigField[]> = {
  'mqtt-client': MQTT_FIELDS,
  'modbus-tcp': MODBUS_TCP_FIELDS,
  'modbus-tcp-server': MODBUS_TCP_SERVER_FIELDS,
  'http': HTTP_RECEIVER_FIELDS,
  'http-sender': HTTP_SENDER_FIELDS,
  'influxdb': INFLUXDB_FIELDS,
  'logger': CONSOLE_LOGGER_FIELDS,
  'samsung-nasa': SAMSUNG_NASA_FIELDS,
  'lgap': LG_LGAP_FIELDS,
  'lgcp': LG_LGCP_FIELDS,
  'serial': SERIAL_FIELDS,
  'store': STORE_FIELDS,
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
