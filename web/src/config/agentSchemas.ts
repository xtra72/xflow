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
  { value: 'lgcnp', label: 'LG LGCNP-01 Capture' },
  { value: 'century-hvac', label: 'Century HVAC (passive)' },
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
  { name: 'format', type: 'select', label: '출력 형식', options: ['text', 'json', 'binary'], default: 'text' },
  { name: 'content_mode', type: 'select', label: '출력 콘텐츠 모드', options: ['full', 'payload'], default: 'full', description: 'full=전체 메시지, payload=페이로드만' },
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
  { name: 'tcp_host', type: 'string', label: 'TCP 호스트', required: true, description: '예: 192.168.1.100', visibleWhen: { field: 'transport_type', value: 'tcp' } },
  { name: 'tcp_port', type: 'number', label: 'TCP 포트', required: true, default: 4196, description: '예: 4196', visibleWhen: { field: 'transport_type', value: 'tcp' } },
  // 즉시 적용 설정
  { name: 'poll_interval', type: 'string', label: '상태 확인 요청 간격', default: '30s' },
  { name: 'buzzer_on_control', type: 'boolean', label: '제어 시 부저', default: false },
  { name: 'notify_on_change', type: 'boolean', label: '상태 변경 알람 전송', default: false },
  { name: 'auto_discovery', type: 'boolean', label: '자동 디바이스 발견', default: true },
  { name: 'include_raw_message_sets', type: 'boolean', label: 'Raw 메시지셋 포함', default: false, description: '상태 출력에 raw_message_sets(원본 NASA 메시지 전체)를 포함. 페이로드가 커지므로 디버깅 시에만 권장' },
  { name: 'log_decode_errors', type: 'boolean', label: '디코드 오류 로그 출력', default: false, description: '디코딩 실패 시 WARN 로그 출력 (디버깅 용). 운영 환경에서는 비활성 권장' },
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
  // 전송 방식 선택
  { name: 'transport_type', type: 'select', label: '연결 방식', options: ['serial', 'tcp-client', 'tcp-server'], default: 'serial', required: true, description: '통신 전송 방식 (serial: RS-485, tcp-client: TCP 클라이언트, tcp-server: TCP 서버)' },
  // 시리얼 설정 (transport_type=serial)
  { name: 'serial_port', type: 'string', label: '시리얼 포트', required: true, description: 'RS-485 시리얼 포트 경로 (예: /dev/ttyUSB1)', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'baud_rate', type: 'number', label: '통신 속도 (Baud Rate)', default: 9600, description: 'LGCP 기본값 9600bps', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'data_bits', type: 'number', label: '데이터 비트', default: 8, description: '데이터 비트 수 (기본: 8)', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'stop_bits', type: 'number', label: '스톱 비트', default: 1, description: '스톱 비트 수 (기본: 1)', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'parity', type: 'select', label: '패리티', options: ['none', 'even', 'odd'], default: 'none', description: '패리티 검사 방식', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'read_timeout', type: 'string', label: '읽기 타임아웃', default: '500ms', description: '시리얼 읽기 대기 시간', visibleWhen: { field: 'transport_type', value: 'serial' } },
  // TCP 공통 설정 (transport_type=tcp-client 또는 tcp-server)
  { name: 'tcp_host', type: 'string', label: 'TCP 호스트', description: 'tcp-client: 서버 IP (예: 192.168.1.100), tcp-server: 바인드 주소 (예: 0.0.0.0)', visibleWhen: { field: 'transport_type', value: ['tcp-client', 'tcp-server'] } },
  { name: 'tcp_port', type: 'number', label: 'TCP 포트', default: 8899, description: 'TCP 포트 번호', visibleWhen: { field: 'transport_type', value: ['tcp-client', 'tcp-server'] } },
  { name: 'tcp_read_timeout', type: 'string', label: 'TCP 읽기 타임아웃', default: '500ms', description: 'TCP 소켓 읽기 대기 시간', visibleWhen: { field: 'transport_type', value: ['tcp-client', 'tcp-server'] } },
  { name: 'tcp_write_timeout', type: 'string', label: 'TCP 쓰기 타임아웃', default: '1s', description: 'TCP 소켓 쓰기 대기 시간', visibleWhen: { field: 'transport_type', value: ['tcp-client', 'tcp-server'] } },
  { name: 'tcp_connect_timeout', type: 'string', label: 'TCP 연결 타임아웃', default: '5s', description: 'TCP 서버 연결 대기 시간 (tcp-client 전용)', visibleWhen: { field: 'transport_type', value: 'tcp-client' } },
  // 공통 LGCP 프로토콜 설정
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

const LG_LGCNP_FIELDS: ConfigField[] = [
  // 전송 방식 선택
  { name: 'transport_type', type: 'select', label: '연결 방식', options: ['serial', 'tcp-client', 'tcp-server'], default: 'serial', required: true, description: '통신 전송 방식 (serial: RS-485, tcp-client: TCP 클라이언트, tcp-server: TCP 서버)' },
  // 시리얼 설정 (transport_type=serial)
  { name: 'serial_port', type: 'string', label: '시리얼 포트', required: true, description: 'RS-485 시리얼 포트 경로 (예: /dev/ttyUSB1)', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'baud_rate', type: 'number', label: '통신 속도 (Baud Rate)', default: 1200, description: 'LGCNP-01 기본값 1200bps', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'data_bits', type: 'number', label: '데이터 비트', default: 8, description: '데이터 비트 수 (기본: 8)', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'stop_bits', type: 'number', label: '스톱 비트', default: 1, description: '스톱 비트 수 (기본: 1)', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'parity', type: 'select', label: '패리티', options: ['none', 'even', 'odd'], default: 'none', description: '패리티 검사 방식', visibleWhen: { field: 'transport_type', value: 'serial' } },
  // TCP 공통 설정 (transport_type=tcp-client 또는 tcp-server)
  { name: 'tcp_host', type: 'string', label: 'TCP 호스트', description: 'tcp-client: 서버 IP (예: 192.168.1.100), tcp-server: 바인드 주소 (예: 0.0.0.0)', visibleWhen: { field: 'transport_type', value: ['tcp-client', 'tcp-server'] } },
  { name: 'tcp_port', type: 'number', label: 'TCP 포트', default: 8899, description: 'TCP 포트 번호', visibleWhen: { field: 'transport_type', value: ['tcp-client', 'tcp-server'] } },
  // 공통 LGCNP 프로토콜 설정
  { name: 'verify_redundancy', type: 'boolean', label: '이중 기록 검증', default: true, description: 'LGCNP-01 이중 기록(dual-record) 무결성 검증' },
  { name: 'auto_discovery', type: 'boolean', label: '자동 디바이스 발견', default: true, description: '버스에서 새 디바이스 자동 등록' },
  { name: 'offline_timeout', type: 'string', label: '오프라인 타임아웃', default: '30s', description: '디바이스 오프라인 판정 시간' },
  { name: 'notify_interval', type: 'string', label: '상태 보고 주기', default: '0s', description: '주기적 상태 보고 간격 (예: 30s). 0s이면 변경 시에만 보고' },
  { name: 'devices', type: 'string', label: '사전 등록 디바이스', description: '설정 기반 디바이스 목록 (address, name)' },
  { name: 'control_enabled', type: 'boolean', label: '제어 기능 활성화', default: false, description: '제어 기능 (현재 미지원 - 프로토콜 분석 진행 중)' },
];

// ---- Century HVAC (passive sniff) — SPEC-CENTURY-001 v0.2.0 ----
const CENTURY_HVAC_FIELDS: ConfigField[] = [
  // 전송 방식 선택 (v0.2.0: serial / tcp-client / tcp-server)
  { name: 'transport_type', type: 'select', label: '연결 방식', options: ['serial', 'tcp-client', 'tcp-server'], default: 'serial', required: true, description: '통신 전송 방식 — serial: RS-485 직결, tcp-client: 컨버터 IP에 접속, tcp-server: 컨버터 push 수신' },
  // ── 시리얼 모드 필드 ──
  { name: 'serial_port', type: 'string', label: '시리얼 포트', required: true, description: 'RS-485 시리얼 포트 경로 (예: /dev/ttyUSB0)', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'baud_rate', type: 'number', label: '통신 속도 (Baud Rate)', default: 9600, description: '캡처 환경에 따라 사용자 측정 — 프로토콜 문서가 보레이트를 명시하지 않음', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'data_bits', type: 'number', label: '데이터 비트', default: 8, visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'stop_bits', type: 'number', label: '스톱 비트', default: 1, visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'parity', type: 'select', label: '패리티', options: ['none', 'even', 'odd'], default: 'none', visibleWhen: { field: 'transport_type', value: 'serial' } },
  // ── TCP 모드 필드 (v0.2.0 신규) ──
  { name: 'tcp_host', type: 'string', label: 'TCP 호스트', required: true, default: '0.0.0.0', description: 'tcp-client: 컨버터 IP (필수). tcp-server: 바인드 주소 (0.0.0.0 = 모든 인터페이스)', visibleWhen: { field: 'transport_type', value: ['tcp-client', 'tcp-server'] } },
  { name: 'tcp_port', type: 'number', label: 'TCP 포트', required: true, description: '1-65535 범위. 시리얼-Ethernet 컨버터 기본값 예: Moxa NPort 4001, USR-N520 4196', visibleWhen: { field: 'transport_type', value: ['tcp-client', 'tcp-server'] } },
  { name: 'tcp_connect_timeout', type: 'string', label: 'TCP 연결 타임아웃', default: '5s', description: 'net.Dialer.Timeout (tcp-client 전용)', visibleWhen: { field: 'transport_type', value: 'tcp-client' } },
  { name: 'tcp_read_timeout', type: 'string', label: 'TCP 읽기 타임아웃', default: '3s', description: '매 Read 직전 SetReadDeadline 갱신. 초과 시 연결 종료 후 재연결', visibleWhen: { field: 'transport_type', value: ['tcp-client', 'tcp-server'] } },
  { name: 'reconnect_initial', type: 'string', label: '재연결 초기 간격', default: '5s', description: 'Exponential backoff 시작값 (tcp-client 전용). 매 실패 시 2배 증가', visibleWhen: { field: 'transport_type', value: 'tcp-client' } },
  { name: 'max_reconnect_backoff', type: 'string', label: '재연결 backoff 상한', default: '5m', description: 'Exponential backoff 상한 (tcp-client 전용)', visibleWhen: { field: 'transport_type', value: 'tcp-client' } },
  // ── Century 프로토콜 공통 필드 ──
  { name: 'master_address', type: 'string', label: '마스터 주소', default: '0x0030', description: 'LE u16 마스터 주소 (hex/dec 입력 허용, 예: 0x0030 또는 48)' },
  { name: 'slave_address', type: 'string', label: '슬레이브 주소', default: '0x0001', description: 'LE u16 슬레이브 주소 (hex/dec 입력 허용)' },
  { name: 'sub_dev_id', type: 'string', label: 'Sub Device ID', default: '0x3B', description: 'payload prefix 의 sub_dev_id (indoor unit ID, 다중 IDU 자동 발견 시 키)' },
  { name: 'ring_buffer_size', type: 'number', label: 'Ring Buffer 크기', default: 128, description: '캡처 프레임 ring buffer 크기 (폴링 ~512ms 기준 약 65초 분량)' },
  { name: 'offline_timeout', type: 'string', label: '오프라인 타임아웃', default: '5s', description: '폴링 주기 약 512ms 의 약 10배 — 이 시간 동안 프레임 미수신 시 디바이스 오프라인 전이' },
  { name: 'cycle_idle_timeout', type: 'string', label: 'Cycle Idle 타임아웃', description: 'inter-frame idle 의 새 cycle 판정 임계값 (REQ-CENTURY-027 2차 신호). 미설정 시 transport-aware default: serial 100ms / tcp-* 200ms (REQ-CENTURY-032)' },
  { name: 'auto_discovery', type: 'boolean', label: '자동 디바이스 발견', default: true, description: '회선상 관측된 sub_dev_id 를 디바이스로 자동 등록 (다중 IDU 지원)' },
  { name: 'dedupe_writes', type: 'boolean', label: 'WRITE 중복 제거', default: true, description: '동일 cycle 내 중복 WRITE 프레임을 1개로 합침. raw frame 노드는 dedupe 와 무관하게 모든 프레임 emit' },
  // ── v0.3.0 출력 정책 (REQ-CENTURY-033/034/035) ──
  { name: 'emit_device_state', type: 'boolean', label: 'Device state emit (기본)', default: true, description: '통합 device state event (전원/모드/풍량/설정온도/현재온도 + 증발기 온도)를 변경 감지 시 emit. v0.3.0 기본 출력' },
  { name: 'emit_register_decoded', type: 'boolean', label: 'Register decoded emit (v0.2 호환)', default: false, description: 'register 단위 decoded 메시지도 emit (Reg02/Reg03/Reg04). v0.2.x 호환용. 두 옵션 모두 false 면 시작 실패' },
  { name: 'keepalive_interval', type: 'string', label: 'Keepalive 간격', default: '60s', description: '변경 없을 때 N 초마다 keepalive emit (0=비활성). 너무 짧으면(<30s) cycle 주기와 상호작용으로 매 cycle emit 됨, 권장 ≥30s' },
  { name: 'keepalive_mode', type: 'select', label: 'Keepalive 모드', options: ['relative', 'absolute'], default: 'relative', description: 'relative: 마지막 emit 으로부터 interval 만큼 경과 시 emit. absolute: wall-clock 정렬 (매 분/5분/시 등 interval 정수 배수 시점에 emit, linux crontab 패턴). 디바이스 다중 운영 시 absolute 가 로그 정렬에 유리' },
  { name: 'include_inferred_fields', type: 'boolean', label: '추정 필드 포함 (모니터링)', default: false, description: 'register-decoded 메시지에 inferred 필드 (op_val_*, status_bits, temp_A_c, reg04_const_*, reg02_live_*, reg02_word_* 등 추정 의미 필드) 포함 여부. 활성 시 별도 "inferred" 그룹으로 출력. 운영=false, 검증/모니터링=true' },
  { name: 'include_unknown_fields', type: 'boolean', label: '미분석 필드 포함 (디버깅)', default: false, description: 'register-decoded 메시지에 unknown 필드 (reg03_pad_*, reg04_byte_3..6, write_byte_* 등 padding/reserved 바이트) 포함 여부. 활성 시 별도 "unknown" 그룹으로 출력. 운영=false, 프로토콜 RE/디버깅=true' },
  { name: 'include_register_info', type: 'boolean', label: '레지스터 정보 포함', default: false, description: 'register-decoded 메시지에 register 번호 + direction 등 register 메타 포함 여부. 운영=false, 프로토콜 분석=true. dev_id / timestamp_ms / state 그룹은 옵션과 무관 항상 출력' },
  { name: 'include_raw_hex', type: 'boolean', label: 'raw_hex 포함', default: false, description: 'register-decoded 메시지에 raw_hex (원시 바이트 hex) 포함 여부. 운영=false, RE/디버깅=true. century-raw-frame 노드는 자체 목적이라 옵션 무관 항상 emit' },
  { name: 'log_decode_errors', type: 'boolean', label: '디코드 에러 로그', default: false, description: 'per-error WARN 로그 (CRC 불일치, 페이로드 prefix 위반 등). 통계 카운터는 항상 증가' },
  { name: 'log_drops', type: 'boolean', label: '드롭 로그', default: false, description: 'ring buffer 가득 참으로 인한 프레임 드롭 시 per-drop WARN 로그' },
  { name: 'log_unconfirmed_fields', type: 'boolean', label: '미확정 필드 로그', default: false, description: '미확정 (unknown/inferred) 바이트가 알려진 값 외로 관측될 때 DEBUG 로그' },
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
  { name: 'log_drops', type: 'boolean', label: '드롭 로그 출력', default: false, description: '수신 버퍼가 가득 차 메시지를 드롭할 때 WARN 로그 출력 (디버깅 용). 운영 환경에서는 비활성 권장 — 로그 폭주 방지' },
  // 프레이밍 설정
  { name: 'framing', type: 'select', label: '프레이밍 모드', options: ['raw', 'newline', 'length_prefix', 'fixed_size', 'stream', 'frame'], default: 'raw', description: '수신 데이터 구분 방식' },
  { name: 'delimiter', type: 'number', label: '구분자 (바이트 값)', default: 10, description: '0x0A = LF, 0x0D = CR', visibleWhen: { field: 'framing', value: 'newline' } },
  { name: 'fixed_size', type: 'number', label: '고정 크기 (바이트)', description: '프레임당 고정 바이트 수', visibleWhen: { field: 'framing', value: 'fixed_size' } },
  { name: 'idle_timeout', type: 'string', label: '유휴 타임아웃', default: '50ms', description: '바이트 수신 중단 후 프레임 완료 대기', visibleWhen: { field: 'framing', value: 'stream' } },
  { name: 'gap_timeout', type: 'string', label: '프레임 간격', description: '프레임 사이 무수신 판별 시간 (예: 500ns, 1ms, 100ms, 1s). 설정 시 해당 시간 동안 데이터 없으면 프레임 완료' },
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

const TCP_SERVER_FIELDS: ConfigField[] = [
  // 연결 설정
  { name: 'host', type: 'string', label: '호스트', default: '0.0.0.0', description: '바인드 주소' },
  { name: 'port', type: 'number', label: '포트', required: true, description: 'TCP 리스닝 포트' },
  { name: 'buffer_size', type: 'number', label: '버퍼 크기 (바이트)', default: 4096 },
  // 운영 설정
  { name: 'max_connections', type: 'number', label: '최대 연결 수', default: 0, description: '0 = 무제한' },
  { name: 'max_message_size', type: 'number', label: '최대 메시지 크기', default: 0, description: '0 = 무제한' },
  // 프레이밍 설정
  { name: 'framing', type: 'select', label: '프레이밍 모드', options: ['raw', 'newline', 'length_prefix', 'fixed_size'], default: 'raw', description: '수신 데이터 구분 방식' },
  { name: 'delimiter', type: 'number', label: '구분자 (바이트 값)', default: 10, description: '0x0A = LF, 0x0D = CR', visibleWhen: { field: 'framing', value: 'newline' } },
  { name: 'fixed_size', type: 'number', label: '고정 크기 (바이트)', description: '프레임당 고정 바이트 수', visibleWhen: { field: 'framing', value: 'fixed_size' } },
];

/**
 * Store 에이전트 필드 정의.
 *
 * SPEC-STORE-003 이후 필드는 UI 상 두 섹션으로 나뉘어 렌더링된다:
 *   - 운영 섹션: history_ttl, max_history_size, max_key_length, scan_interval, default_ttl
 *   - 데이터 섹션: registration_type, keys (정적 키 + data_type + metric_type + 태그)
 *
 * 섹션 분리는 `AgentDetailPanel.tsx` 의 `StoreConfigEditor` 컴포넌트가 담당하며,
 * 여기서는 필드 메타데이터만 정의한다. `keys` 필드는 별도의 커스텀 UI 로
 * 렌더링되므로 이 스키마에는 포함되지 않는다 (StoreConfigEditor 에서 직접 관리).
 *
 * v0.7.0 진화 (M12, M13):
 *   - `allow_dynamic_keys: bool` → `registration_type: enum` ('auto' | 'manual')
 *   - keys[] 항목에 `data_type` 와 `metric_type` 추가 (StoreKeysEditor 에서 처리)
 *
 * @spec SPEC-WEB-005 v0.7.0 (M12)
 * @spec SPEC-STORE-003 v0.3.0
 */
const STORE_FIELDS: ConfigField[] = [
  // --- 운영 섹션 ---
  { name: 'max_key_length', type: 'number', label: '최대 키 길이 (바이트)', default: 512, description: '키 문자열 최대 바이트 수' },
  { name: 'scan_interval', type: 'string', label: 'TTL 스캔 간격', default: '30s', description: '만료 키 정리 주기 (예: 30s, 1m)' },
  { name: 'default_ttl', type: 'string', label: '기본 TTL', description: '키 기본 만료 시간 (예: 1h). 미설정 시 만료 없음' },
  { name: 'max_history_size', type: 'number', label: '히스토리 최대 갯수', default: 0, description: '키당 보관할 이전 값 최대 수 (0: 비활성화)' },
  { name: 'history_ttl', type: 'string', label: '히스토리 보관 시간', description: '히스토리 항목 보관 기간 (예: 30m, 1h). 미설정 시 시간 제한 없음' },
  // --- 데이터 섹션 ---
  // v0.7.0 (M12): `allow_dynamic_keys` 토글이 `registration_type` enum 으로 진화.
  // - auto: 미등록 키 자동 등록 (이전 allow_dynamic_keys=true 에 대응)
  // - manual: 정적 키만 허용 (이전 allow_dynamic_keys=false 에 대응)
  // StoreConfigEditor 가 이 값을 읽어 StoreKeysEditor 의 `registrationType` prop 으로 전달한다.
  {
    name: 'registration_type',
    type: 'select',
    label: '등록 방식',
    options: ['auto', 'manual'],
    default: 'auto',
    description: 'auto: 미등록 키 자동 등록 / manual: 정적 키만 허용 (data_type 필수)',
  },
];

/**
 * Store 에이전트 "운영 섹션" 에 속하는 필드 이름 집합.
 * AgentDetailPanel 의 StoreConfigEditor 가 섹션을 분리할 때 참조한다.
 *
 * @spec SPEC-STORE-003
 */
export const STORE_OPERATION_FIELDS = new Set([
  'max_key_length',
  'scan_interval',
  'default_ttl',
  'max_history_size',
  'history_ttl',
]);

/**
 * Store 에이전트 "데이터 섹션" 에 속하는 필드 이름 집합.
 * `keys` 는 커스텀 에디터로 별도 렌더링되므로 여기 포함되지 않는다.
 *
 * v0.7.0 (M12): `allow_dynamic_keys` → `registration_type` 마이그레이션.
 *
 * @spec SPEC-WEB-005 v0.7.0 (M12)
 * @spec SPEC-STORE-003
 */
export const STORE_DATA_FIELDS = new Set(['registration_type']);

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
  'lgcnp': LG_LGCNP_FIELDS,
  'century-hvac': CENTURY_HVAC_FIELDS,
  'serial': SERIAL_FIELDS,
  'tcp-server': TCP_SERVER_FIELDS,
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
