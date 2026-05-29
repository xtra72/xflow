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
  { value: 'samsung_hvacr01', label: 'Samsung HVACR-01' },
  { value: 'lgap', label: 'LG LGAP' },
  { value: 'lgcp', label: 'LG LGCP Capture' },
  { value: 'lg_hvacr01', label: 'LG HVACR-01 Capture' },
  { value: 'century_hvacr01', label: 'Century HVACR-01 (passive)' },
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
  { name: 'debug', type: 'boolean', label: '디버그 로그', default: false, description: 'true 면 InfluxDB 로 전송되는 write / query 요청을 DEBUG 레벨로 출력 (운영 환경에서는 false 권장)' },
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

// ──────────────────────────────────────────────────────────────────────────
// Samsung HVACR-01 (NASA, SPEC-SAMSUNG-HVACR-01)
// Note: backend (samsung/transport.go) 만 'serial' / 'tcp' 두 모드를 지원한다.
// LG/Century 와 달리 tcp-client / tcp-server 가 분리되어 있지 않다.
// Samsung agent 는 state change 를 항상 emit 한다 (master toggle 없음).
// ──────────────────────────────────────────────────────────────────────────
const SAMSUNG_HVACR01_FIELDS: ConfigField[] = [
  // ── Transport ──
  { name: 'transport_type', type: 'select', label: '연결 방식', options: ['serial', 'tcp'], required: true, description: '통신 전송 방식 — serial: RS-485 직결 / tcp: TCP 소켓 (Samsung backend 는 client/server 구분 없이 단일 tcp 모드)' },
  { name: 'serial_port', type: 'string', label: '시리얼 포트', required: true, description: 'RS-485 시리얼 포트 경로 (예: /dev/ttyUSB0)', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'baud_rate', type: 'number', label: '통신 속도 (Baud Rate)', default: 9600, description: '통신 속도 (이 프로토콜 기본값: 9600bps)', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'data_bits', type: 'number', label: '데이터 비트', default: 8, visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'stop_bits', type: 'number', label: '스톱 비트', default: 1, visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'parity', type: 'select', label: '패리티', options: ['none', 'even', 'odd'], default: 'even', description: '패리티 검사 방식 (이 프로토콜 표준: even — 8E1)', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'tcp_host', type: 'string', label: 'TCP 호스트', required: true, description: 'TCP 서버 IP (예: 192.168.1.100, EW11 등 RS-485 변환기)', visibleWhen: { field: 'transport_type', value: 'tcp' } },
  { name: 'tcp_port', type: 'number', label: 'TCP 포트', required: true, default: 4196, description: 'TCP 포트 번호 (시리얼-Ethernet 컨버터 기본값 예: 4196)', visibleWhen: { field: 'transport_type', value: 'tcp' } },
  // ── Protocol-specific (Samsung NASA) ──
  { name: 'status_query_enabled', type: 'boolean', label: '상태 확인 요청 활성', default: true, description: '주기적 상태 확인 요청 (BuildStatusQuery) 송신 여부. false 면 passive sniff only (수동 감청 전용 모드, 컨트롤러 부담 감소)' },
  { name: 'poll_interval', type: 'string', label: '상태 확인 요청 간격', default: '30s', description: 'status_query_enabled=true 일 때만 의미 있음. 디바이스마다 status query 송신' },
  { name: 'buzzer_on_control', type: 'boolean', label: '제어 시 부저', default: false, description: '제어 명령 시 실내기 부저 울림' },
  // ── Device discovery ──
  { name: 'auto_discovery', type: 'boolean', label: '자동 디바이스 발견', default: true, description: '버스에서 새 디바이스 자동 등록' },
  { name: 'offline_timeout', type: 'string', label: '오프라인 타임아웃', default: '30s', description: '이 시간 동안 통신 미수신 시 디바이스 오프라인 판정 (예: 30s, 1m)' },
  // ── State reporting ──
  // Samsung agent 는 device state 변경 시 항상 emit (master toggle 없음).
  // 따라서 emit_device_state 또는 notify_on_change 같은 토글이 없다.
  { name: 'report_interval', type: 'string', label: '상태보고 주기', default: '60s', description: '주기적 상태보고 간격 (0=비활성, 권장: ≥30s)' },
  { name: 'report_mode', type: 'select', label: '상태보고 정렬', options: ['relative', 'absolute'], default: 'relative', description: 'relative: 마지막 emit 이후 interval 경과 / absolute: wall-clock (crontab 패턴)' },
  // ── Output / logging (advanced) ──
  { name: 'include_raw_hex', type: 'boolean', label: 'raw_hex 포함', default: false, description: '출력에 원시 바이트 hex 포함 (운영: false, 디버깅: true)', advanced: true },
  { name: 'log_decode_errors', type: 'boolean', label: '디코드 오류 로그', default: false, description: '디코드 실패 시 WARN 로그 출력', advanced: true },
  { name: 'log_drops', type: 'boolean', label: '드롭 로그', default: false, description: '버퍼 가득 참으로 인한 프레임 드롭 시 WARN 로그', advanced: true },
  { name: 'log_state_updates', type: 'boolean', label: '상태 갱신 로그', default: false, description: '디바이스 state 갱신마다 디코드 값 + payload hex 를 INFO 로그로 출력 (진단용, 운영 환경 비활성 권장)', advanced: true },
  // ── Diagnostic (advanced) ──
  { name: 'event_temp_threshold', type: 'number', label: '이벤트 온도 임계값 (℃)', default: 1.0, description: '실내온도(current_temp)만 변경된 경우 |Δ| ≥ 임계값일 때만 이벤트 보고 (0 이하=비활성)', advanced: true },
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
  // v0.6.0 공통 옵션 (5 agent 통일):
  { name: 'report_interval', type: 'string', label: '상태보고 주기', default: '', description: '주기적 상태보고 간격 (예: 60s, 0=비활성). v0.6.0 통합 옵션' },
  { name: 'report_mode', type: 'select', label: '상태보고 정렬', options: ['relative', 'absolute'], default: 'relative', description: 'relative: 마지막 emit 으로부터 interval 경과 시. absolute: wall-clock 정렬 (crontab 패턴)' },
  { name: 'include_raw_hex', type: 'boolean', label: 'raw_hex 포함', default: false, description: '메시지에 raw_hex (원시 바이트 hex) 포함 여부. 운영=false, RE/디버깅=true' },
  { name: 'event_temp_threshold', type: 'number', label: '이벤트 온도 임계값 (℃)', default: 1.0, description: 'v0.6.6: 실내온도(current_temp)만 변경된 경우 |Δ| ≥ 임계값일 때만 이벤트 보고. 0 이하=비활성' },
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
  // v0.6.0 공통 옵션 (5 agent 통일):
  { name: 'report_interval', type: 'string', label: '상태보고 주기', default: '', description: '주기적 상태보고 간격 (0 또는 빈 값=비활성). 이전 notify_interval, deprecation alias 유지' },
  { name: 'report_mode', type: 'select', label: '상태보고 정렬', options: ['relative', 'absolute'], default: 'relative', description: 'relative: 마지막 emit 으로부터 interval 경과 시. absolute: wall-clock 정렬 (crontab 패턴)' },
  { name: 'include_raw_hex', type: 'boolean', label: 'raw_hex 포함', default: false, description: '메시지에 raw_hex (원시 바이트 hex) 포함 여부. 운영=false, RE/디버깅=true' },
  { name: 'reconnect_interval', type: 'string', label: '재연결 간격', default: '5s', description: '연결 끊김 시 재시도 간격' },
  { name: 'max_reconnect_backoff', type: 'string', label: '최대 재연결 대기', default: '5m', description: '재연결 백오프 상한' },
  { name: 'msg_channel_size', type: 'number', label: '메시지 버퍼 크기', default: 256, description: '내부 메시지 채널 버퍼' },
  { name: 'control_enabled', type: 'boolean', label: '제어 기능 활성화', default: false, description: '실내기 능동 제어 기능 (전원, 온도, 풍량, 모드)' },
  { name: 'controller_address', type: 'string', label: '컨트롤러 주소', default: '44550000', description: '컨트롤러 SA 주소 (8자리 HEX). control_enabled 시 필수' },
  { name: 'control_verify_timeout', type: 'string', label: '제어 검증 타임아웃', default: '3s', description: '제어 명령 후 상태 변경 확인 대기 시간' },
  { name: 'event_temp_threshold', type: 'number', label: '이벤트 온도 임계값 (℃)', default: 1.0, description: 'v0.6.6: 실내온도(current_temp)만 변경된 경우 |Δ| ≥ 임계값일 때만 이벤트 보고. 0 이하=비활성' },
];

// ──────────────────────────────────────────────────────────────────────────
// LG HVACR-01 (ICP-01, SPEC-LG-HVACR-01)
// LG agent 는 device state 변경 시 항상 emit (master toggle 없음).
// dedupe_frames 는 동일 state 반복 emit 차단용으로 별도 운영.
// ──────────────────────────────────────────────────────────────────────────
const LG_HVACR01_FIELDS: ConfigField[] = [
  // ── Transport ──
  { name: 'transport_type', type: 'select', label: '연결 방식', options: ['serial', 'tcp-client', 'tcp-server'], default: 'serial', required: true, description: '통신 전송 방식 — serial: RS-485 직결 / tcp-client: TCP 클라이언트 / tcp-server: TCP 서버' },
  { name: 'serial_port', type: 'string', label: '시리얼 포트', required: true, description: 'RS-485 시리얼 포트 경로 (예: /dev/ttyUSB0)', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'baud_rate', type: 'number', label: '통신 속도 (Baud Rate)', default: 1200, description: '통신 속도 (이 프로토콜 기본값: 1200bps)', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'data_bits', type: 'number', label: '데이터 비트', default: 8, visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'stop_bits', type: 'number', label: '스톱 비트', default: 1, visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'parity', type: 'select', label: '패리티', options: ['none', 'even', 'odd'], default: 'none', description: '패리티 검사 방식 (이 프로토콜 표준: none)', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'tcp_host', type: 'string', label: 'TCP 호스트', description: 'tcp-client: 서버 IP, tcp-server: 바인드 주소 (0.0.0.0 = 모든 인터페이스)', visibleWhen: { field: 'transport_type', value: ['tcp-client', 'tcp-server'] } },
  { name: 'tcp_port', type: 'number', label: 'TCP 포트', default: 8899, description: 'TCP 포트 번호', visibleWhen: { field: 'transport_type', value: ['tcp-client', 'tcp-server'] } },
  // ── Protocol-specific (LG ICP-01) ──
  // v0.18.1: verify_odu_checksum — 일부 디바이스 변형이 SEQ=04 b[19] 를 fixed marker 로 사용해 표준 SUM checksum 불일치를 우회하기 위한 옵션.
  { name: 'verify_redundancy', type: 'boolean', label: 'IDU 이중 기록 검증', default: true, description: 'TYPE-B (IDU) 40바이트 long frame 의 b[9]==b[29] / b[11]==b[31] / b[23]==b[36] 검증. 20바이트 short 변형 디바이스는 자동 우회됨 (v0.18.1).' },
  { name: 'verify_odu_checksum', type: 'boolean', label: 'ODU 체크섬 검증', default: true, description: 'TYPE-A (ODU) frame 의 SEQ=01/04/05 체크섬 검증. 일부 디바이스 변형은 SEQ=04 b[19] 가 fixed 0x55 marker — 이 경우 false 로 설정 (v0.18.1).' },
  { name: 'control_enabled', type: 'boolean', label: '제어 기능 활성화', default: false, description: '제어 기능 (현재 미지원 - 프로토콜 분석 진행 중)' },
  // ── Device discovery ──
  { name: 'auto_discovery', type: 'boolean', label: '자동 디바이스 발견', default: true, description: '버스에서 새 디바이스 자동 등록' },
  { name: 'offline_timeout', type: 'string', label: '오프라인 타임아웃', default: '30s', description: '이 시간 동안 통신 미수신 시 디바이스 오프라인 판정 (예: 30s, 1m)' },
  // ── State reporting ──
  // LG agent 는 device state 변경 시 항상 emit (master toggle 없음, Samsung 과 동일).
  { name: 'report_interval', type: 'string', label: '상태보고 주기', default: '60s', description: '주기적 상태보고 간격 (0=비활성, 권장: ≥30s)' },
  { name: 'report_mode', type: 'select', label: '상태보고 정렬', options: ['relative', 'absolute'], default: 'relative', description: 'relative: 마지막 emit 이후 interval 경과 / absolute: wall-clock (crontab 패턴)' },
  // ── Output / logging (advanced) ──
  { name: 'include_raw_hex', type: 'boolean', label: 'raw_hex 포함', default: false, description: '출력에 원시 바이트 hex 포함 (운영: false, 디버깅: true)', advanced: true },
  { name: 'log_decode_errors', type: 'boolean', label: '디코드 오류 로그', default: false, description: '디코드 실패 시 WARN 로그 출력', advanced: true },
  { name: 'log_drops', type: 'boolean', label: '드롭 로그', default: false, description: '버퍼 가득 참으로 인한 프레임 드롭 시 WARN 로그', advanced: true },
  { name: 'log_state_updates', type: 'boolean', label: '상태 갱신 로그', default: false, description: '디바이스 state 갱신마다 디코드 값 + payload hex 를 INFO 로그로 출력 (진단용, 운영 환경 비활성 권장)', advanced: true },
  // ── Diagnostic (advanced) ──
  { name: 'event_temp_threshold', type: 'number', label: '이벤트 온도 임계값 (℃)', default: 1.0, description: '실내온도(current_temp)만 변경된 경우 |Δ| ≥ 임계값일 때만 이벤트 보고 (0 이하=비활성)', advanced: true },
];

// ──────────────────────────────────────────────────────────────────────────
// Century HVACR-01 (ICP-01, SPEC-CENTURY-HVACR-001 v0.2.0, passive sniff)
// Century 는 3 HVACR 중 유일하게 emit_device_state master toggle 을 가진다
// (false 시 전체 device state event 차단). Samsung/LG 와 의미가 다름.
// ──────────────────────────────────────────────────────────────────────────
const CENTURY_HVACR01_FIELDS: ConfigField[] = [
  // ── Transport ──
  { name: 'transport_type', type: 'select', label: '연결 방식', options: ['serial', 'tcp-client', 'tcp-server'], default: 'serial', required: true, description: '통신 전송 방식 — serial: RS-485 직결 / tcp-client: TCP 클라이언트 / tcp-server: TCP 서버' },
  { name: 'serial_port', type: 'string', label: '시리얼 포트', required: true, description: 'RS-485 시리얼 포트 경로 (예: /dev/ttyUSB0)', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'baud_rate', type: 'number', label: '통신 속도 (Baud Rate)', default: 9600, description: '통신 속도 (이 프로토콜 기본값: 9600bps — 캡처 환경에 따라 사용자 측정)', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'data_bits', type: 'number', label: '데이터 비트', default: 8, visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'stop_bits', type: 'number', label: '스톱 비트', default: 1, visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'parity', type: 'select', label: '패리티', options: ['none', 'even', 'odd'], default: 'none', description: '패리티 검사 방식 (이 프로토콜 표준: none)', visibleWhen: { field: 'transport_type', value: 'serial' } },
  { name: 'tcp_host', type: 'string', label: 'TCP 호스트', required: true, default: '0.0.0.0', description: 'tcp-client: 서버 IP, tcp-server: 바인드 주소 (0.0.0.0 = 모든 인터페이스)', visibleWhen: { field: 'transport_type', value: ['tcp-client', 'tcp-server'] } },
  { name: 'tcp_port', type: 'number', label: 'TCP 포트', required: true, description: '1-65535 범위. 시리얼-Ethernet 컨버터 기본값 예: Moxa NPort 4001, USR-N520 4196', visibleWhen: { field: 'transport_type', value: ['tcp-client', 'tcp-server'] } },
  { name: 'tcp_connect_timeout', type: 'string', label: 'TCP 연결 타임아웃', default: '5s', description: 'net.Dialer.Timeout (tcp-client 전용)', visibleWhen: { field: 'transport_type', value: 'tcp-client' } },
  { name: 'tcp_read_timeout', type: 'string', label: 'TCP 읽기 타임아웃', default: '3s', description: '매 Read 직전 SetReadDeadline 갱신. 초과 시 연결 종료 후 재연결', visibleWhen: { field: 'transport_type', value: ['tcp-client', 'tcp-server'] } },
  { name: 'reconnect_interval', type: 'string', label: '재연결 초기 간격', default: '5s', description: 'Exponential backoff 시작값 (tcp-client 전용). 매 실패 시 2배 증가', visibleWhen: { field: 'transport_type', value: 'tcp-client' } },
  { name: 'max_reconnect_backoff', type: 'string', label: '재연결 backoff 상한', default: '5m', description: 'Exponential backoff 상한 (tcp-client 전용)', visibleWhen: { field: 'transport_type', value: 'tcp-client' } },
  // ── Protocol-specific (Century ICP-01) ──
  // 제거: ring_buffer_size, cycle_idle_timeout, dedupe_writes (운영자가 거의 안 만짐 — backend 기본값으로 충분)
  // 제거: devices (사전 등록 디바이스) — 디바이스 탭에서 처리 (Samsung NASA 패턴)
  { name: 'master_address', type: 'string', label: '마스터 주소', default: '0x0030', description: 'LE u16 마스터 주소 (hex/dec 입력 허용, 예: 0x0030 또는 48)' },
  { name: 'slave_address', type: 'string', label: '슬레이브 주소', default: '0x0001', description: 'LE u16 슬레이브 주소 (hex/dec 입력 허용)' },
  { name: 'sub_dev_id', type: 'string', label: 'Sub Device ID', default: '0x3B', description: 'payload prefix 의 sub_dev_id (indoor unit ID, 다중 IDU 자동 발견 시 키)' },
  // ── Device discovery ──
  { name: 'auto_discovery', type: 'boolean', label: '자동 디바이스 발견', default: true, description: '버스에서 새 디바이스 자동 등록 (회선상 관측된 sub_dev_id → 다중 IDU 지원)' },
  { name: 'offline_timeout', type: 'string', label: '오프라인 타임아웃', default: '30s', description: '이 시간 동안 통신 미수신 시 디바이스 오프라인 판정 (예: 30s, 1m). 폴링 주기 약 512ms' },
  // ── State reporting ──
  // Century 만의 master toggle: false 면 device state event 전체 차단 (Samsung/LG 에는 없는 옵션).
  { name: 'emit_device_state', type: 'boolean', label: '상태 변경 알림', default: true, description: '통합 device state event (전원/모드/풍량/설정온도/현재온도 + 증발기 온도)를 변경 감지 시 emit. false 면 device state event 전체 차단 (Century 전용 master toggle)' },
  { name: 'report_interval', type: 'string', label: '상태보고 주기', default: '60s', description: '주기적 상태보고 간격 (0=비활성, 권장: ≥30s)' },
  { name: 'report_mode', type: 'select', label: '상태보고 정렬', options: ['relative', 'absolute'], default: 'relative', description: 'relative: 마지막 emit 이후 interval 경과 / absolute: wall-clock (crontab 패턴)' },
  // ── Output / logging (advanced) ──
  { name: 'include_raw_hex', type: 'boolean', label: 'raw_hex 포함', default: false, description: '출력에 원시 바이트 hex 포함 (운영: false, 디버깅: true)', advanced: true },
  { name: 'include_register_info', type: 'boolean', label: '레지스터 정보', default: false, description: '출력에 register 번호 + direction 등 register 메타 포함 (운영=false, 프로토콜 분석=true)', advanced: true },
  { name: 'log_decode_errors', type: 'boolean', label: '디코드 오류 로그', default: false, description: '디코드 실패 시 WARN 로그 출력', advanced: true },
  { name: 'log_drops', type: 'boolean', label: '드롭 로그', default: false, description: '버퍼 가득 참으로 인한 프레임 드롭 시 WARN 로그', advanced: true },
  { name: 'log_state_updates', type: 'boolean', label: '상태 갱신 로그', default: false, description: '디바이스 state 갱신마다 디코드 값 + payload hex 를 INFO 로그로 출력 (진단용, 운영 환경 비활성 권장)', advanced: true },
  // ── Diagnostic (advanced) ──
  { name: 'event_temp_threshold', type: 'number', label: '이벤트 온도 임계값 (℃)', default: 1.0, description: '실내온도(current_temp)만 변경된 경우 |Δ| ≥ 임계값일 때만 이벤트 보고 (0 이하=비활성)', advanced: true },
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
  { name: 'length_adjustment', type: 'number', label: '길이 보정값', default: 0, description: '디코딩된 길이에 더할 보정값 (예: Samsung NASA 프로토콜은 -1)', visibleWhen: { field: 'framing', value: 'frame' } },
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
  'samsung_hvacr01': SAMSUNG_HVACR01_FIELDS,
  'lgap': LG_LGAP_FIELDS,
  'lgcp': LG_LGCP_FIELDS,
  'lg_hvacr01': LG_HVACR01_FIELDS,
  'century_hvacr01': CENTURY_HVACR01_FIELDS,
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
