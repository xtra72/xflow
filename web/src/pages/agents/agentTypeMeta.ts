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

  'modbus-tcp': {
    description:
      'Modbus TCP 클라이언트로 원격 디바이스의 레지스터를 읽고 쓰는 에이전트. interval/event 모드를 지원하며, 다중 디바이스 폴링과 쓰기 이벤트 처리가 가능합니다.',
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

  'modbus-tcp-server': {
    description:
      'Modbus TCP 서버로 동작하여 외부 클라이언트의 요청을 수신하는 에이전트. 다중 유닛 디바이스를 호스팅하며 레지스터 맵 기반의 읽기/쓰기를 처리합니다.',
    configFields: [
      { name: 'listen_address', type: 'string', required: false, description: '수신 대기 IP 주소', default: '0.0.0.0' },
      { name: 'listen_port', type: 'number', required: true, description: '수신 대기 포트 (1-65535)', default: '502' },
      { name: 'max_connections', type: 'number', required: false, description: '최대 동시 클라이언트 연결 수', default: '10' },
      { name: 'idle_timeout', type: 'string', required: false, description: '유휴 연결 타임아웃', default: '60s' },
    ],
    configExample: {
      listen_address: '0.0.0.0',
      listen_port: 502,
      max_connections: 10,
      idle_timeout: '60s',
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

  'samsung-nasa': {
    description:
      '삼성 NASA(Network Attached System Air-conditioner) 프로토콜로 공조 시스템을 모니터링하고 제어하는 에이전트. 시리얼(RS-485) 및 TCP 연결을 지원하며, 자동 디바이스 발견과 상태 변경 알림 기능을 제공합니다.',
    configFields: [
      { name: 'transport_type', type: 'select', required: true, description: '연결 방식 (serial 또는 tcp)' },
      { name: 'serial_port', type: 'string', required: false, description: '시리얼 포트 경로 (serial 모드)' },
      { name: 'baud_rate', type: 'number', required: false, description: '통신 속도', default: '9600' },
      { name: 'parity', type: 'select', required: false, description: '패리티 (none/even/odd)', default: 'even' },
      { name: 'tcp_host', type: 'string', required: false, description: 'TCP 호스트 (tcp 모드, 예: 192.168.1.100)' },
      { name: 'tcp_port', type: 'number', required: false, default: '4196', description: 'TCP 포트 (tcp 모드, 예: 4196)' },
      { name: 'poll_interval', type: 'string', required: false, description: '상태 확인 주기', default: '30s' },
      { name: 'auto_discovery', type: 'boolean', required: false, description: '자동 디바이스 발견', default: 'true' },
      { name: 'notify_on_change', type: 'boolean', required: false, description: '상태 변경 시 알림 전송', default: 'false' },
    ],
    configExample: {
      transport_type: 'serial',
      serial_port: '/dev/ttyUSB0',
      baud_rate: 9600,
      parity: 'even',
      poll_interval: '30s',
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

  lgcp: {
    description:
      'LG LGCP(LG Central Control Protocol) 프로토콜로 LG 시스템에어컨을 모니터링하고 제어하는 에이전트. RS-485 시리얼 및 TCP(클라이언트/서버) 연결을 지원하며, CRC-16/XMODEM 검증과 능동 제어 기능을 제공합니다.',
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

  lgcnp: {
    description:
      'LG LGCNP-01(LG CN-485 Protocol) 프로토콜로 LG 시스템에어컨을 패시브 모니터링하는 에이전트. RS-485 1200bps 통신으로 TYPE-A(ODU 20B) / TYPE-B(IDU 40B) 이중 프레임을 캡처하며, 6계층 신뢰성 모델(체크섬, 이중기록, 구조, 물리범위 검증)을 적용합니다.',
    configFields: [
      { name: 'transport_type', type: 'select', required: true, description: '연결 방식 (serial / tcp-client / tcp-server)', default: 'serial' },
      { name: 'serial_port', type: 'string', required: false, description: 'RS-485 시리얼 포트 경로 (serial 모드)' },
      { name: 'baud_rate', type: 'number', required: false, description: '통신 속도 (LGCNP-01 기본값: 1200)', default: '1200' },
      { name: 'tcp_host', type: 'string', required: false, description: 'TCP 호스트 주소 (tcp-client: 서버 IP, tcp-server: 바인드 주소)' },
      { name: 'tcp_port', type: 'number', required: false, description: 'TCP 포트 번호' },
      { name: 'verify_redundancy', type: 'boolean', required: false, description: 'TYPE-B 이중 기록 무결성 검증', default: 'true' },
      { name: 'auto_discovery', type: 'boolean', required: false, description: '버스에서 새 디바이스 자동 등록', default: 'true' },
      { name: 'offline_timeout', type: 'string', required: false, description: '디바이스 오프라인 판정 시간', default: '30s' },
    ],
    configExample: {
      transport_type: 'serial',
      serial_port: '/dev/ttyUSB0',
      baud_rate: 1200,
      verify_redundancy: true,
      auto_discovery: true,
      offline_timeout: '30s',
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
};
