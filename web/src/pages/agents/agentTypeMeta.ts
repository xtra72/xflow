// 에이전트 타입의 상세 메타데이터.

export interface ConfigFieldMeta {
  name: string;
  type: string;
  required: boolean;
  description: string;
  default?: string;
}

export interface AgentTypeDetailMeta {
  description: string;
  configFields: ConfigFieldMeta[];
  configExample: Record<string, unknown>;
}

export const AGENT_TYPE_META: Record<string, AgentTypeDetailMeta> = {
  'mqtt-client': {
    description:
      'MQTT 브로커에 연결하여 메시지를 구독/발행하는 에이전트. QoS 레벨, 자동 재연결, 클린 세션 등을 지원하며 IoT 센서 데이터 수집이나 디바이스 제어에 사용됩니다.',
    configFields: [
      { name: 'broker', type: 'string', required: true, description: 'MQTT 브로커 주소 (예: tcp://localhost:1883)' },
      { name: 'topics', type: 'string', required: false, description: '구독 토픽 (쉼표 구분, 예: sensor/+/data,device/#)' },
      { name: 'qos', type: 'select', required: false, description: '메시지 전달 보증 레벨 (0/1/2)', default: '1' },
      { name: 'auto_reconnect', type: 'boolean', required: false, description: '연결 끊김 시 자동 재연결', default: 'true' },
      { name: 'buffer_size', type: 'number', required: false, description: '수신 메시지 버퍼 크기', default: '256' },
      { name: 'keep_alive_sec', type: 'number', required: false, description: 'Keep Alive 간격 (초)', default: '60' },
      { name: 'clean_session', type: 'boolean', required: false, description: '클린 세션 모드', default: 'true' },
    ],
    configExample: {
      broker: 'tcp://localhost:1883',
      topics: 'sensor/+/data,device/#',
      qos: '1',
      auto_reconnect: true,
      buffer_size: 256,
    },
  },

  'modbus-client': {
    description:
      'Modbus 클라이언트 에이전트. TCP(MBAP) 또는 RTU(시리얼, CRC-16) 트랜스포트를 선택해 원격 디바이스의 레지스터를 읽고 쓴다. interval/event 모드, 다중 디바이스 폴링, 그룹별 폴링 주기, 쓰기 이벤트, 런타임 재구성(set_config)을 지원합니다.',
    configFields: [
      { name: 'mode', type: 'select', required: false, description: '동작 모드 (interval: 주기적 폴링, event: 변경 감지)', default: 'interval' },
      { name: 'poll_interval', type: 'string', required: false, description: '폴링 간격 (Go duration 형식)', default: '5s' },
      { name: 'request_timeout', type: 'string', required: false, description: '요청 타임아웃', default: '3s' },
      { name: 'max_retries', type: 'number', required: false, description: '최대 재시도 횟수', default: '3' },
      { name: 'enable_write_events', type: 'boolean', required: false, description: '쓰기 이벤트 활성화', default: 'true' },
      { name: 'devices', type: 'object', required: true, description: '디바이스 배열 (주소, unit_id, register_map 포함)' },
      { name: 'read_mode', type: 'select', required: false, description: '읽기 모드 (direct/cached)', default: 'cached' },
    ],
    configExample: {
      mode: 'interval',
      poll_interval: '5s',
      request_timeout: '3s',
      max_retries: 3,
      devices: [
        {
          address: '192.168.1.100:502',
          unit_id: 1,
          register_map: {
            holding_registers: { start_address: 0, count: 100 },
          },
        },
      ],
    },
  },

  'modbus-gateway': {
    description:
      'Modbus 게이트웨이 에이전트. Modbus 서버로 동작하여 외부 클라이언트의 요청을 수신합니다. transport(tcp/rtu)로 TCP 수신 또는 시리얼(RTU) 수신을 선택합니다. 다중 유닛 디바이스를 호스팅하며 레지스터 맵 기반의 읽기/쓰기를 처리합니다.',
    configFields: [
      { name: 'transport', type: 'string', required: false, description: 'tcp(MBAP) 또는 rtu(시리얼)', default: 'tcp' },
      { name: 'listen_address', type: 'string', required: false, description: '수신 대기 IP 주소 (transport=tcp)', default: '0.0.0.0' },
      { name: 'listen_port', type: 'number', required: true, description: '수신 대기 포트 (1-65535, transport=tcp)', default: '502' },
      { name: 'serial_port', type: 'string', required: false, description: 'RTU 시리얼 포트 경로 (transport=rtu)', default: '/dev/ttyUSB0' },
      { name: 'max_connections', type: 'number', required: false, description: '최대 동시 클라이언트 연결 수', default: '10' },
      { name: 'idle_timeout', type: 'string', required: false, description: '유휴 연결 타임아웃', default: '60s' },
      { name: 'devices', type: 'object', required: false, description: 'role=main 필수. 호스팅할 디바이스 목록(각 unit_id + register_map)' },
      { name: 'notify_on_write', type: 'boolean', required: false, description: '외부 마스터의 통신 쓰기로 레지스터 변경 시 register_change 알림 발행(다음 재시작 시 적용)', default: 'false' },
      { name: 'log_frames', type: 'boolean', required: false, description: '송/수신 MODBUS 프레임 요약을 로그에 기록(변경 즉시 적용)', default: 'false' },
      { name: 'log_raw_frames', type: 'boolean', required: false, description: '프레임 로그에 전체 ADU를 hex 로 포함(log_frames 켜짐일 때만 의미, 변경 즉시 적용)', default: 'false' },
    ],
    configExample: {
      transport: 'tcp',
      listen_address: '0.0.0.0',
      listen_port: 502,
      role: 'main',
      max_connections: 10,
      idle_timeout: '60s',
      devices: [
        {
          unit_id: 0,
          name: 'shared',
          register_map: {
            holding_registers: [{ address: 0, count: 100, data_type: 'uint16' }],
          },
        },
        {
          unit_id: 1,
          name: 'device-1',
          register_map: {
            coils: [{ address: 0, count: 8, data_type: 'uint16', description: 'door sensor' }],
            holding_registers: [{ address: 0, count: 10, shared_address: 0, description: 'pump status' }],
          },
        },
      ],
    },
  },

  http: {
    description:
      'HTTP 서버로 동작하여 외부 웹훅이나 REST API 요청을 수신하는 에이전트. 지정된 경로와 메서드로 들어오는 요청을 메시지로 변환하여 플로우에 전달합니다.',
    configFields: [
      { name: 'listen_addr', type: 'string', required: true, description: '수신 주소 (예: :8081)', default: ':8080' },
      { name: 'path', type: 'string', required: false, description: '수신 경로', default: '/' },
      { name: 'method', type: 'select', required: false, description: 'HTTP 메서드 (GET/POST/PUT/PATCH)', default: 'POST' },
      { name: 'timeout_sec', type: 'number', required: false, description: '요청 처리 타임아웃 (초)', default: '30' },
      { name: 'max_body_bytes', type: 'number', required: false, description: '최대 요청 본문 크기 (바이트)', default: '1048576' },
      { name: 'buffer_size', type: 'number', required: false, description: '수신 메시지 버퍼 크기', default: '256' },
    ],
    configExample: {
      listen_addr: ':8081',
      path: '/webhook',
      method: 'POST',
      timeout_sec: 30,
      max_body_bytes: 1048576,
    },
  },

  'http-sender': {
    description:
      'HTTP 클라이언트로 동작하여 플로우의 메시지를 외부 서버로 전송하는 에이전트. REST API 호출, 웹훅 발송 등에 사용되며 커스텀 헤더를 지원합니다.',
    configFields: [
      { name: 'url', type: 'string', required: true, description: '전송 대상 URL' },
      { name: 'method', type: 'select', required: false, description: 'HTTP 메서드 (GET/POST/PUT/PATCH/DELETE)', default: 'POST' },
      { name: 'content_type', type: 'string', required: false, description: 'Content-Type 헤더 값', default: 'application/json' },
      { name: 'timeout_sec', type: 'number', required: false, description: '요청 타임아웃 (초)', default: '30' },
      { name: 'headers', type: 'object', required: false, description: '추가 HTTP 헤더 (JSON 객체)' },
    ],
    configExample: {
      url: 'https://api.example.com/data',
      method: 'POST',
      content_type: 'application/json',
      timeout_sec: 30,
      headers: { Authorization: 'Bearer token123' },
    },
  },

  influxdb: {
    description:
      'InfluxDB 시계열 데이터베이스에 데이터를 기록하고 조회하는 에이전트. InfluxDB v2/v3를 지원하며, 배치 쓰기와 다양한 쿼리 언어(Flux, InfluxQL, SQL)를 지원합니다.',
    configFields: [
      { name: 'url', type: 'string', required: true, description: 'InfluxDB 서버 주소', default: 'http://localhost:8086' },
      { name: 'token', type: 'string', required: true, description: '인증 토큰' },
      { name: 'org', type: 'string', required: false, description: '조직 이름 (v2 필수, v3 선택)' },
      { name: 'bucket', type: 'string', required: true, description: '데이터 버킷 이름' },
      { name: 'version', type: 'select', required: true, description: 'InfluxDB 버전 (2 또는 3)' },
      { name: 'precision', type: 'select', required: false, description: '타임스탬프 정밀도 (ns/us/ms/s)', default: 'ns' },
      { name: 'batch_size', type: 'number', required: false, description: '배치 쓰기 크기', default: '1000' },
      { name: 'flush_interval_ms', type: 'number', required: false, description: '배치 플러시 간격 (밀리초)', default: '1000' },
    ],
    configExample: {
      url: 'http://localhost:8086',
      token: 'my-token',
      org: 'my-org',
      bucket: 'iot-data',
      version: '2',
      precision: 'ms',
      batch_size: 1000,
      flush_interval_ms: 1000,
    },
  },

  logger: {
    description:
      '메시지를 콘솔(stdout/stderr) 또는 파일로 출력하는 로깅 에이전트. 파일 로테이션, gzip 압축, JSON/텍스트 포맷을 지원하며 디버깅 및 모니터링 용도로 사용됩니다.',
    configFields: [
      { name: 'output', type: 'select', required: false, description: '출력 대상 (stdout/stderr/file)', default: 'stdout' },
      { name: 'output_path', type: 'string', required: false, description: '파일 경로 (output=file 일 때)' },
      { name: 'format', type: 'select', required: false, description: '출력 형식 (text/json/binary)', default: 'text' },
      { name: 'content_mode', type: 'select', required: false, description: '출력 콘텐츠 모드 (full=전체 메시지, payload=페이로드만)', default: 'full' },
      { name: 'max_size', type: 'number', required: false, description: '파일 최대 크기 (MB, output=file)', default: '10' },
      { name: 'max_age', type: 'number', required: false, description: '파일 보관 기간 (일, 0=무제한)', default: '0' },
      { name: 'compress', type: 'boolean', required: false, description: '백업 파일 gzip 압축', default: 'false' },
      { name: 'prefix', type: 'string', required: false, description: '로그 출력 접두어', default: '[logger]' },
    ],
    configExample: {
      output: 'file',
      output_path: '/var/log/xflow/agent.log',
      format: 'json',
      content_mode: 'full',
      max_size: 10,
      max_age: 7,
      compress: true,
      prefix: '[logger]',
    },
  },

  'samsung_hvacr01': {
    description:
      'Samsung HVACR-01 에이전트. 삼성 NASA(Network Attached System Air-conditioner) 프로토콜로 공조 시스템을 모니터링하고 제어합니다. 3가지 transport (시리얼 RS-485 직결, tcp-client 컨버터 접속, tcp-server 컨버터 push 수신) 를 지원하며, 자동 디바이스 발견과 상태 변경 알림 기능을 제공합니다.',
    configFields: [
      { name: 'transport_type', type: 'select', required: true, description: '연결 방식 (serial / tcp-client / tcp-server)' },
      { name: 'serial_port', type: 'string', required: false, description: '시리얼 포트 경로 (serial 모드)' },
      { name: 'baud_rate', type: 'number', required: false, description: '통신 속도', default: '9600' },
      { name: 'parity', type: 'select', required: false, description: '패리티 (none/even/odd)', default: 'even' },
      { name: 'tcp_host', type: 'string', required: false, description: 'TCP 호스트 (tcp-client: 서버 IP, tcp-server: 바인드 주소, 기본 0.0.0.0)' },
      { name: 'tcp_port', type: 'number', required: false, default: '4196', description: 'TCP 포트 (tcp-client / tcp-server 모드)' },
      { name: 'status_query_enabled', type: 'boolean', required: false, description: '상태 확인 요청 활성. false 면 passive sniff only (v0.6.1)', default: 'true' },
      { name: 'poll_interval', type: 'string', required: false, description: '상태 확인 요청 간격 (status_query_enabled=true 시)', default: '30s' },
      { name: 'auto_discovery', type: 'boolean', required: false, description: '자동 디바이스 발견', default: 'true' },
      { name: 'offline_timeout', type: 'string', required: false, description: '오프라인 타임아웃 (디바이스 통신 없음 → 오프라인 판정 시간, v0.6.2)', default: '30s' },
    ],
    configExample: {
      transport_type: 'serial',
      serial_port: '/dev/ttyUSB0',
      baud_rate: 9600,
      parity: 'even',
      poll_interval: '30s',
      offline_timeout: '30s',
      auto_discovery: true,
    },
  },

  lgap: {
    description:
      'LG LGAP(LG Gateway Access Protocol) 프로토콜로 LG 공조 시스템을 모니터링하는 에이전트. RS-485 시리얼 통신으로 실내기 상태를 수집하며, 자동 디바이스 발견과 재연결을 지원합니다.',
    configFields: [
      { name: 'transport_type', type: 'select', required: true, description: '연결 방식 (serial 전용)', default: 'serial' },
      { name: 'serial_port', type: 'string', required: false, description: '시리얼 포트 경로' },
      { name: 'baud_rate', type: 'number', required: false, description: '통신 속도 (LGAP 기본값: 4800)', default: '4800' },
      { name: 'poll_interval', type: 'string', required: false, description: '폴링 간격', default: '30s' },
      { name: 'read_timeout', type: 'string', required: false, description: '읽기 타임아웃', default: '500ms' },
      { name: 'reconnect_interval', type: 'string', required: false, description: '재연결 기본 간격', default: '5s' },
      { name: 'max_reconnect_backoff', type: 'string', required: false, description: '재연결 최대 백오프', default: '5m' },
    ],
    configExample: {
      transport_type: 'serial',
      serial_port: '/dev/ttyUSB0',
      baud_rate: 4800,
      poll_interval: '30s',
      read_timeout: '500ms',
    },
  },

  lg_hvacr02: {
    description:
      'LG HVACR-02 에이전트 (LG ICP-02 프로토콜) 로 LG 시스템에어컨을 모니터링하고 제어합니다. RS-485 시리얼 및 TCP(클라이언트/서버) 연결을 지원하며, CRC-16/XMODEM 검증과 능동 제어 기능을 제공합니다.',
    configFields: [
      { name: 'transport_type', type: 'select', required: true, description: '연결 방식 (serial / tcp-client / tcp-server)', default: 'serial' },
      { name: 'serial_port', type: 'string', required: false, description: 'RS-485 시리얼 포트 경로 (serial 모드)' },
      { name: 'baud_rate', type: 'number', required: false, description: '통신 속도 (serial 모드)', default: '9600' },
      { name: 'parity', type: 'select', required: false, description: '패리티 검사 방식 (serial 모드)', default: 'none' },
      { name: 'tcp_host', type: 'string', required: false, description: 'TCP 호스트 주소 (tcp-client: 서버 IP, tcp-server: 바인드 주소)' },
      { name: 'tcp_port', type: 'number', required: false, description: 'TCP 포트 번호', default: '8899' },
      { name: 'tcp_read_timeout', type: 'string', required: false, description: 'TCP 읽기 타임아웃', default: '500ms' },
      { name: 'tcp_write_timeout', type: 'string', required: false, description: 'TCP 쓰기 타임아웃', default: '1s' },
      { name: 'tcp_connect_timeout', type: 'string', required: false, description: 'TCP 연결 타임아웃 (tcp-client 전용)', default: '5s' },
      { name: 'verify_crc', type: 'boolean', required: false, description: 'CRC-16/XMODEM 무결성 검증', default: 'true' },
      { name: 'auto_discovery', type: 'boolean', required: false, description: '버스에서 새 디바이스 자동 등록', default: 'true' },
      { name: 'control_enabled', type: 'boolean', required: false, description: '실내기 능동 제어 기능 활성화', default: 'false' },
      { name: 'controller_address', type: 'string', required: false, description: '컨트롤러 SA 주소 (8자리 HEX)', default: '44550000' },
      { name: 'msg_channel_size', type: 'number', required: false, description: '내부 메시지 채널 버퍼 크기', default: '256' },
    ],
    configExample: {
      transport_type: 'tcp-client',
      tcp_host: '192.168.1.100',
      tcp_port: 8899,
      tcp_read_timeout: '500ms',
      tcp_write_timeout: '1s',
      tcp_connect_timeout: '5s',
      verify_crc: true,
      auto_discovery: true,
      control_enabled: false,
    },
  },

  lg_hvacr01: {
    description:
      'LG HVACR-01(LG ICP-01 프로토콜) 으로 LG 시스템에어컨을 패시브 모니터링하는 에이전트. RS-485 1200bps 통신으로 TYPE-A(ODU 20B) / TYPE-B(IDU 40B) 이중 프레임을 캡처하며, 6계층 신뢰성 모델(체크섬, 이중기록, 구조, 물리범위 검증)을 적용합니다.',
    configFields: [
      { name: 'transport_type', type: 'select', required: true, description: '연결 방식 (serial / tcp-client / tcp-server)', default: 'serial' },
      { name: 'serial_port', type: 'string', required: false, description: 'RS-485 시리얼 포트 경로 (serial 모드)' },
      { name: 'baud_rate', type: 'number', required: false, description: '통신 속도 (LG ICP-01 기본값: 1200)', default: '1200' },
      { name: 'tcp_host', type: 'string', required: false, description: 'TCP 호스트 주소 (tcp-client: 서버 IP, tcp-server: 바인드 주소)' },
      { name: 'tcp_port', type: 'number', required: false, description: 'TCP 포트 번호' },
      // v0.6.2 Web UI 정리 — verify_redundancy 제거 (backend default true 로 운영 충분).
      { name: 'auto_discovery', type: 'boolean', required: false, description: '버스에서 새 디바이스 자동 등록 (디바이스 탭에서 사전 등록 관리)', default: 'true' },
      { name: 'offline_timeout', type: 'string', required: false, description: '디바이스 오프라인 판정 시간', default: '30s' },
      { name: 'report_interval', type: 'string', required: false, description: '주기적 상태보고 간격 (0=비활성)', default: '60s' },
      { name: 'dedupe_frames', type: 'boolean', required: false, description: '동일 state 의 중복 frame emit 차단 (변경 감지)', default: 'true' },
      { name: 'log_io', type: 'boolean', required: false, description: '입출력 진단 로그 (frame parse/ring push/periodic report 의 주요 이벤트 INFO 로그). 상태보고 누락 등 진단 시 일시 활성화. 운영 시 false 권장.', default: 'false' },
    ],
    configExample: {
      transport_type: 'serial',
      serial_port: '/dev/ttyUSB0',
      baud_rate: 1200,
      auto_discovery: true,
      offline_timeout: '30s',
      report_interval: '60s',
    },
  },

  'century_hvacr01': {
    description:
      'Century HVACR-01 에어컨 RS-485 프로토콜(Century ICP-01)을 패시브 모니터링하는 에이전트(SPEC-CENTURY-HVACR-001 v0.2.0). 마스터-슬레이브 폴링 통신(약 512ms 주기, CRC-16/ARC init=0x0000)을 가로채 register 0x02(설정 readback) / 0x03(증발기 냉매 배관 온도) / 0x04(운전 상태 + WRITE 제어 명령)를 디코딩합니다. 3가지 transport (serial 직결, tcp-client 컨버터 접속, tcp-server 컨버터 push 수신)를 지원하며, transport.Write() 는 절대 호출하지 않습니다(불변식). 동일 cycle 내 중복 WRITE 프레임을 자동으로 1개로 합쳐 noise 를 제거합니다.',
    configFields: [
      { name: 'transport_type', type: 'select', required: true, description: '연결 방식 (serial / tcp-client / tcp-server)', default: 'serial' },
      // ── Serial 모드 필드 ──
      { name: 'serial_port', type: 'string', required: false, description: 'RS-485 시리얼 포트 경로 (serial 모드 필수, 예: /dev/ttyUSB0)' },
      { name: 'baud_rate', type: 'number', required: false, description: '통신 속도 (serial 모드)', default: '9600' },
      { name: 'data_bits', type: 'number', required: false, description: '데이터 비트 (serial 모드)', default: '8' },
      { name: 'stop_bits', type: 'number', required: false, description: '스톱 비트 (serial 모드)', default: '1' },
      { name: 'parity', type: 'select', required: false, description: '패리티 (serial 모드, none/even/odd)', default: 'none' },
      // ── TCP 모드 필드 (v0.2.0 신규) ──
      { name: 'tcp_host', type: 'string', required: false, description: 'TCP 호스트 (tcp-client: 컨버터 IP 필수, tcp-server: 바인드 주소)', default: '0.0.0.0' },
      { name: 'tcp_port', type: 'number', required: false, description: 'TCP 포트 (tcp-* 모드 필수, 1-65535)' },
      { name: 'tcp_connect_timeout', type: 'string', required: false, description: 'TCP dial 타임아웃 (tcp-client)', default: '5s' },
      { name: 'tcp_read_timeout', type: 'string', required: false, description: 'TCP read 타임아웃', default: '3s' },
      { name: 'reconnect_interval', type: 'string', required: false, description: '재연결 backoff 초기 간격 (tcp-client)', default: '5s' },
      { name: 'max_reconnect_backoff', type: 'string', required: false, description: '재연결 backoff 상한 (tcp-client, exponential)', default: '5m' },
      // ── Century 프로토콜 공통 필드 ──
      // v0.6.2 Web UI 정리 — ring_buffer_size / cycle_idle_timeout / dedupe_writes /
      // emit_register_decoded / include_inferred_fields / include_unknown_fields /
      // log_unconfirmed_fields 제거 (운영자 친화 — 거의 안 만지는 필드).
      { name: 'master_address', type: 'string', required: false, description: '마스터 주소 (LE u16, hex 또는 십진수)', default: '0x0030' },
      { name: 'slave_address', type: 'string', required: false, description: '슬레이브 주소 (LE u16, hex 또는 십진수)', default: '0x0001' },
      { name: 'sub_dev_id', type: 'string', required: false, description: '예상 sub_dev_id (실내기 ID 추정, hex 또는 십진수)', default: '0x3B' },
      { name: 'offline_timeout', type: 'string', required: false, description: '디바이스 오프라인 판정 시간', default: '30s' },
      { name: 'auto_discovery', type: 'boolean', required: false, description: '버스에서 새 sub_dev_id 자동 등록 (다중 IDU 지원)', default: 'true' },
      // 상태 변경 알림 / 주기적 상태보고:
      { name: 'emit_device_state', type: 'boolean', required: false, description: '통합 device state event emit (상태 변경 알림)', default: 'true' },
      { name: 'report_interval', type: 'string', required: false, description: '주기적 상태보고 간격 (0=비활성, 권장 ≥30s)', default: '60s' },
      { name: 'report_mode', type: 'select', required: false, description: 'relative: 마지막 emit 으로부터 interval 경과 시. absolute: wall-clock 정렬 (crontab 패턴)', default: 'relative' },
      // 출력 옵션 (운영자 친화 라벨):
      { name: 'include_register_info', type: 'boolean', required: false, description: '레지스터 정보 (register 번호 + direction) 포함 여부. 운영=false, 분석=true', default: 'false' },
      { name: 'include_raw_hex', type: 'boolean', required: false, description: '원시 프레임 (raw_hex) 포함 여부. 운영=false, RE/디버깅=true', default: 'false' },
      { name: 'log_decode_errors', type: 'boolean', required: false, description: '에러 (디코드 오류 WARN 로그)', default: 'false' },
      { name: 'log_drops', type: 'boolean', required: false, description: '드롭 로그 (ring buffer overflow WARN)', default: 'false' },
    ],
    configExample: {
      transport_type: 'tcp-client',
      tcp_host: '192.168.1.100',
      tcp_port: 4196,
      tcp_connect_timeout: '5s',
      tcp_read_timeout: '3s',
      reconnect_interval: '5s',
      max_reconnect_backoff: '5m',
      master_address: '0x0030',
      slave_address: '0x0001',
      sub_dev_id: '0x3B',
      auto_discovery: true,
      offline_timeout: '30s',
    },
  },

  'chirpstack': {
    description:
      'ChirpStack LoRaWAN Network Server 의 MQTT integration 이벤트를 패시브로 수신하는 에이전트(SPEC-CHIRPSTACK-001). ChirpStack 이 application/<id>/device/<devEui>/event/<type> 토픽으로 발행하는 업링크 이벤트를 구독하여, 업링크 payload 의 object(디코딩된 센서 값)를 측정치별로 fan-out 합니다. 디바이스는 devEui 기준으로 자동 생성되며, MQTT 트랜스포트 서브셋(broker/topics/qos/재연결)만 설정합니다. 선택적으로 comm-state(device_state 이벤트)를 발행해 업링크 staleness 기반 online/offline 을 판정합니다(emit_comm_state 게이트). transport.Write() 는 호출하지 않는 수신 전용 에이전트입니다.',
    configFields: [
      { name: 'broker', type: 'string', required: true, description: 'MQTT 브로커 주소 (예: tcp://localhost:1883)', default: 'tcp://localhost:1883' },
      { name: 'client_id', type: 'string', required: false, description: '빈 값이면 자동 생성 (xflow-chirpstack-<uuid>)' },
      { name: 'username', type: 'string', required: false, description: 'MQTT 사용자명' },
      { name: 'password', type: 'string', required: false, description: 'MQTT 비밀번호' },
      { name: 'topics', type: 'string', required: false, description: '구독 토픽 (쉼표 구분, ChirpStack application 이벤트)', default: 'application/#' },
      { name: 'qos', type: 'select', required: false, description: '메시지 전달 보증 레벨 (0/1/2)', default: '1' },
      { name: 'keep_alive_sec', type: 'number', required: false, description: 'Keep Alive 간격 (초)', default: '60' },
      { name: 'auto_reconnect', type: 'boolean', required: false, description: '연결 끊김 시 자동 재연결', default: 'true' },
      { name: 'clean_session', type: 'boolean', required: false, description: '클린 세션 모드', default: 'true' },
      { name: 'buffer_size', type: 'number', required: false, description: '수신 메시지 버퍼 크기', default: '1024' },
      { name: 'connect_timeout_sec', type: 'number', required: false, description: '연결 타임아웃 (초)', default: '10' },
      { name: 'measurement_emit_mode', type: 'select', required: false, description: '측정치 방출 모드. per_measurement: 측정치마다 메시지 1개(payload.value + metadata.measurement). combined: 업링크 1건을 메시지 1개로 합침(payload 최상위에 측정치 이름별 값, metadata.measurement 없음)', default: 'per_measurement' },
      { name: 'timestamp_source', type: 'select', required: false, description: '메시지 타임스탬프 소스. uplink: 업링크 payload 의 time 값(디바이스/게이트웨이 시각). server: 서버가 업링크를 받은 시각. 장비 시계가 틀어져 순서가 어긋날 때 server 를 쓴다(업링크 1건의 모든 측정치가 동일 수신 시각을 공유)', default: 'uplink' },
      { name: 'emit_comm_state', type: 'boolean', required: false, description: 'device_state 이벤트 발행 게이트 (comm-state)', default: 'false' },
      { name: 'comm_report_interval', type: 'string', required: false, description: 'comm-state 주기 report 간격 (예: 60s, 0 이면 주기 report off, change 는 유지)' },
      { name: 'offline_threshold', type: 'string', required: false, description: '마지막 업링크 후 이 시간 경과 시 offline 판정', default: '300s' },
    ],
    configExample: {
      broker: 'tcp://localhost:1883',
      topics: 'application/#',
      qos: '1',
      auto_reconnect: true,
      buffer_size: 1024,
      emit_comm_state: false,
      offline_threshold: '300s',
    },
  },

  xsfm: {
    description:
      '지하철 역사 설비 관리 에이전트(SPEC-XSFM-001). thingplus MQTT 트랜스포트 셸과 samsung 로스터/관측 상태 모델을 결합합니다. transport_mode 로 direct(에이전트가 브로커를 직접 소유) / port(외부 노드가 I/O 담당) 두 모드를 선택하며, payload_mapping 으로 설정 주도 페이로드 시임(power/fan_speed/online 필드 매핑)을 정의합니다. 2축 제어(set_power / set_fan_speed 1·2·3)를 지원하고, 관측 기반 emit("확인된 값만 전송")로 상태를 방출합니다. 역사(station)→호선(line) 레지스트리로 위치 계층을 해석합니다.',
    configFields: [
      { name: 'transport_mode', type: 'select', required: true, description: 'I/O 경계 선택 (direct: 브로커 직접 소유 / port: 외부 노드 I/O)', default: 'direct' },
      { name: 'broker', type: 'string', required: false, description: 'MQTT 브로커 주소 (direct 모드 필수, 예: tcp://localhost:1883)' },
      { name: 'tls', type: 'boolean', required: false, description: 'TLS 사용 (direct 모드)', default: 'false' },
      { name: 'ca_cert', type: 'string', required: false, description: 'TLS CA 인증서 PEM 또는 경로 (tls=true 일 때)' },
      { name: 'client_id', type: 'string', required: false, description: '빈 값이면 자동 생성 (xflow-xsfm-<uuid>)' },
      { name: 'username', type: 'string', required: false, description: 'MQTT 사용자명 (direct 모드)' },
      { name: 'password', type: 'string', required: false, description: 'MQTT 비밀번호 (direct 모드)' },
      { name: 'qos', type: 'select', required: false, description: '메시지 전달 보증 레벨 (0/1/2)', default: '1' },
      { name: 'state_topic_template', type: 'string', required: false, description: '{device_id} placeholder 포함 상태 토픽 (direct 모드 필수)' },
      { name: 'command_topic_template', type: 'string', required: false, description: '{device_id} placeholder 포함 명령 토픽 (direct 모드 필수)' },
      { name: 'payload_mapping', type: 'object', required: true, description: '설정 주도 페이로드 시임 (양 모드 공통 필수, power_field/fan_speed_field/online_field 등)' },
      { name: 'offline_timeout', type: 'string', required: false, description: '상태 미수신 시 오프라인 판정 시간', default: '60s' },
      { name: 'control_response_timeout', type: 'string', required: false, description: '제어 명령 후 상태 반영 대기 시간', default: '5s' },
      { name: 'lwt_enabled', type: 'boolean', required: false, description: 'LWT(유언) 사용 — 비정상 종료 시 오프라인 통지', default: 'true' },
      { name: 'registry_path', type: 'string', required: false, description: '런타임 등록 디바이스(bridge/auto) 로스터 파일 경로 (빈 값=영속화 비활성)' },
      { name: 'station_registry_path', type: 'string', required: false, description: '역사(station)→호선(line) 레지스트리 파일 경로 (빈 값=영속화 비활성)' },
    ],
    configExample: {
      transport_mode: 'direct',
      broker: 'tcp://localhost:1883',
      qos: '1',
      state_topic_template: 'xsfm/{device_id}/state',
      command_topic_template: 'xsfm/{device_id}/cmd',
      payload_mapping: {
        power_field: 'power',
        fan_speed_field: 'fan_speed',
      },
      offline_timeout: '60s',
      control_response_timeout: '5s',
    },
  },

  serial: {
    description:
      '범용 시리얼 통신 에이전트. 다양한 프레이밍 모드(raw, newline, length_prefix, fixed_size, stream, frame)를 지원하며, STX/ETX/길이/체크섬 기반의 프로토콜 프레임 감지가 가능합니다. 산업용 장비, 센서, 임베디드 시스템과의 통신에 사용됩니다.',
    configFields: [
      { name: 'port', type: 'string', required: true, description: '시리얼 포트 경로 (예: /dev/ttyUSB0, COM3)' },
      { name: 'baud_rate', type: 'select', required: false, description: '통신 속도 (1200~115200)', default: '9600' },
      { name: 'parity', type: 'select', required: false, description: '패리티 (none/even/odd/mark/space)', default: 'none' },
      { name: 'framing', type: 'select', required: false, description: '프레이밍 모드 (raw/newline/length_prefix/fixed_size/stream/frame)', default: 'raw' },
      { name: 'stx', type: 'string', required: false, description: 'STX 프레임 시작 (16진수, frame 모드)' },
      { name: 'etx', type: 'string', required: false, description: 'ETX 프레임 종료 (16진수, frame 모드)' },
      { name: 'length_offset', type: 'number', required: false, description: '길이 필드 오프셋 (frame 모드)' },
      { name: 'checksum', type: 'select', required: false, description: '체크섬 방식 (none/sum8/xor)', default: 'none' },
    ],
    configExample: {
      port: '/dev/ttyUSB0',
      baud_rate: '9600',
      framing: 'frame',
      stx: '32',
      etx: '34',
      length_offset: 1,
      length_size: '2',
      length_endian: 'big',
      length_includes_header: true,
      length_adjustment: -1,
      checksum: 'none',
    },
  },

  store: {
    description:
      '인메모리 키-값 저장소 에이전트. TTL 기반 키 만료, 히스토리 관리, 네임스페이스 분리를 지원하며, 플로우 간 상태 공유나 캐싱에 사용됩니다.',
    configFields: [
      { name: 'max_key_length', type: 'number', required: false, description: '키 문자열 최대 바이트 수', default: '512' },
      { name: 'scan_interval', type: 'string', required: false, description: '만료 키 정리 주기', default: '30s' },
      { name: 'default_ttl', type: 'string', required: false, description: '키 기본 만료 시간 (예: 1h). 미설정 시 만료 없음' },
      { name: 'max_history_size', type: 'number', required: false, description: '키당 이전 값 최대 보관 수 (0: 비활성화)', default: '0' },
      { name: 'history_ttl', type: 'string', required: false, description: '히스토리 항목 보관 기간' },
    ],
    configExample: {
      max_key_length: 512,
      scan_interval: '30s',
      default_ttl: '1h',
      max_history_size: 100,
      history_ttl: '30m',
    },
  },

  'tcp-server': {
    description:
      'TCP 서버로 동작하여 클라이언트 연결을 수신하는 에이전트. 다중 클라이언트 연결을 관리하며, 수신된 데이터를 프레이밍 모드에 따라 메시지로 변환하여 플로우에 전달합니다.',
    configFields: [
      { name: 'host', type: 'string', required: false, default: '0.0.0.0', description: '바인드 주소' },
      { name: 'port', type: 'number', required: true, description: 'TCP 리스닝 포트' },
      { name: 'buffer_size', type: 'number', required: false, default: '4096', description: '읽기 버퍼 크기 (바이트)' },
      { name: 'max_connections', type: 'number', required: false, default: '0', description: '최대 동시 연결 수 (0=무제한)' },
      { name: 'max_message_size', type: 'number', required: false, default: '0', description: '최대 메시지 크기 (0=무제한)' },
      { name: 'framing', type: 'string', required: false, default: 'raw', description: '프레이밍 모드: raw, newline, length_prefix, fixed_size' },
      { name: 'delimiter', type: 'number', required: false, default: '10', description: '구분자 바이트 값 (framing=newline)' },
      { name: 'fixed_size', type: 'number', required: false, description: '고정 프레임 크기 (framing=fixed_size)' },
    ],
    configExample: {
      host: '0.0.0.0',
      port: 9000,
      buffer_size: 4096,
      max_connections: 10,
      framing: 'newline',
    },
  },

  'tcp-client': {
    description:
      'TCP 클라이언트로 동작하여 원격 서버에 연결하는 에이전트. 서버로부터 수신한 데이터를 메시지로 변환하며, 플로우에서 생성된 메시지를 서버로 전송할 수 있습니다. (설정 스키마 준비 중)',
    configFields: [],
    configExample: {},
  },

  'udp-server': {
    description:
      'UDP 서버로 동작하여 데이터그램을 수신하는 에이전트. 피어를 추적하며, 수신한 데이터그램을 메시지로 변환하여 플로우에 전달합니다. UDP 는 데이터그램 기반이라 프레이밍 설정이 없습니다.',
    configFields: [
      { name: 'host', type: 'string', required: false, default: '0.0.0.0', description: '바인드 주소' },
      { name: 'port', type: 'number', required: true, description: 'UDP 수신 포트' },
      { name: 'buffer_size', type: 'number', required: false, default: '4096', description: '데이터그램 읽기 버퍼 크기 (바이트)' },
      { name: 'log_messages', type: 'boolean', required: false, default: 'false', description: '송/수신 데이터그램을 hex 로 INFO 로그 (패킷 단위 진단용)' },
    ],
    configExample: {
      host: '0.0.0.0',
      port: 9000,
      buffer_size: 4096,
    },
  },

  'udp-client': {
    description:
      'UDP 클라이언트로 동작하여 대상 서버로 데이터그램을 전송하고 응답을 수신하는 에이전트. 플로우에서 생성된 메시지를 서버로 전송하며, 응답 데이터그램을 메시지로 변환합니다.',
    configFields: [
      { name: 'host', type: 'string', required: true, description: '전송 대상 서버 IP' },
      { name: 'port', type: 'number', required: true, description: '전송 대상 서버 UDP 포트' },
      { name: 'buffer_size', type: 'number', required: false, default: '4096', description: '응답 데이터그램 읽기 버퍼 크기 (바이트)' },
      { name: 'log_messages', type: 'boolean', required: false, default: 'false', description: '송/수신 데이터그램을 hex 로 INFO 로그 (패킷 단위 진단용)' },
    ],
    configExample: {
      host: '192.168.1.100',
      port: 9000,
      buffer_size: 4096,
    },
  },
};
