package century

import (
	"fmt"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// Default values exposed as public constants so tests and call sites can refer to
// the canonical timings instead of inline magic numbers (REQ-CENTURY-029,
// REQ-CENTURY-031, REQ-CENTURY-032).
const (
	// DefaultCycleIdleTimeoutSerial 은 transport_type=serial 의 cycle_idle_timeout
	// 기본값이다 (REQ-CENTURY-027 의 2차 신호 임계).
	DefaultCycleIdleTimeoutSerial = 100 * time.Millisecond

	// DefaultCycleIdleTimeoutTCP 는 transport_type=tcp-* 의 cycle_idle_timeout 기본값이다.
	// Nagle / packetization / jitter 로 TCP 의 inter-frame 간격이 변동할 수 있어 200ms 사용
	// (REQ-CENTURY-032, A12).
	DefaultCycleIdleTimeoutTCP = 200 * time.Millisecond

	// DefaultTCPConnectTimeout 는 tcp-client 의 net.Dialer.Timeout 기본값이다 (REQ-CENTURY-029).
	DefaultTCPConnectTimeout = 5 * time.Second

	// DefaultTCPReadTimeout 는 TCP read 의 SetReadDeadline 기본값이다 (REQ-CENTURY-029).
	DefaultTCPReadTimeout = 3 * time.Second

	// DefaultReconnectInterval 은 tcp-client 의 exponential backoff 초기 슬립이다 (REQ-CENTURY-031).
	// 2026-05-29: 이전 이름 DefaultReconnectInitial 에서 통일 (Samsung / LG 의 reconnect_interval 과 일치).
	DefaultReconnectInterval = 5 * time.Second

	// DefaultMaxReconnectBackoff 는 tcp-client backoff 의 상한이다 (REQ-CENTURY-031).
	DefaultMaxReconnectBackoff = 5 * time.Minute

	// DefaultReportInterval 는 device_state 의 fallback emit 주기 기본값이다 (REQ-CENTURY-035).
	// 0 이면 keepalive 비활성 (change-only 모드).
	DefaultReportInterval = 60 * time.Second
)

// Hvacr01Config 는 Century HVAC 패시브 캡처 에이전트의 설정이다 (REQ-CENTURY-002, REQ-CENTURY-028).
//
// 모든 필드는 AgentConfig.Transport.Options 맵에서 parseHvacr01Config 로 채워지며,
// SPEC-CENTURY-HVACR-001 §4 의 YAML 예시와 1:1 매핑된다.
//
// v0.2.0 (M6): TCP transport 지원 — TransportType 이 "serial" / "tcp-client" / "tcp-server"
// 중 하나를 가질 수 있으며, tcp-* 모드에서는 SerialPort 가 무시되고 TCPHost / TCPPort
// 등이 사용된다.
type Hvacr01Config struct {
	// TransportType 는 트랜스포트 종류이다.
	// v0.1.x: "serial" 만 지원.
	// v0.2.0+: "serial", "tcp-client", "tcp-server" 지원 (REQ-CENTURY-028).
	TransportType string

	// SerialPort 는 시리얼 트랜스포트의 디바이스 경로이다 (예: /dev/ttyUSB0).
	// transport_type=serial 인 경우 필수. tcp-* 모드에서는 무시된다 (REQ-CENTURY-028).
	SerialPort string

	// BaudRate / DataBits / StopBits / Parity 는 시리얼 라인 파라미터이다.
	BaudRate int
	DataBits int
	StopBits int
	Parity   string

	// TCPHost 는 TCP 호스트이다.
	//   - tcp-client: 원격 서버 호스트 (필수).
	//   - tcp-server: 바인드 주소 (기본 "0.0.0.0").
	// (REQ-CENTURY-028, REQ-CENTURY-029, REQ-CENTURY-030)
	TCPHost string

	// TCPPort 는 TCP 포트이다 (1..65535).
	// tcp-client / tcp-server 모드에서 필수 (REQ-CENTURY-028).
	TCPPort int

	// TCPConnectTimeout 는 tcp-client 의 net.Dialer.Timeout 이다 (REQ-CENTURY-029).
	// 기본값: DefaultTCPConnectTimeout (5s).
	TCPConnectTimeout time.Duration

	// TCPReadTimeout 는 TCP read 의 SetReadDeadline 갱신값이다 (REQ-CENTURY-029).
	// timeout 발생 시 연결이 종료되고 재연결된다 (tcp-client) 또는 accept loop 로 복귀한다 (tcp-server).
	// 기본값: DefaultTCPReadTimeout (3s).
	TCPReadTimeout time.Duration

	// ReconnectInterval 는 tcp-client 의 exponential backoff 초기 슬립이다 (REQ-CENTURY-031).
	// 2026-05-29: 이전 이름 ReconnectInitial 에서 통일 (Samsung / LG 의 ReconnectInterval 과 일치).
	// 기본값: DefaultReconnectInterval (5s).
	ReconnectInterval time.Duration

	// MaxReconnectBackoff 는 tcp-client backoff 상한이다 (REQ-CENTURY-031).
	// 기본값: DefaultMaxReconnectBackoff (5m).
	MaxReconnectBackoff time.Duration

	// MasterAddress / SlaveAddress 는 마스터 / 슬레이브의 LE u16 주소이다.
	MasterAddress uint16
	SlaveAddress  uint16

	// SubDevID 는 기본 indoor unit 의 sub_dev_id 이다 (AutoDiscovery=true 이면
	// 다른 sub_dev_id 도 자동 등록).
	SubDevID byte

	// RingBufferSize 는 캡처된 프레임을 보관하는 ring buffer 크기이다 (REQ-CENTURY-012).
	RingBufferSize int

	// OfflineTimeout 은 디바이스가 오프라인으로 전이되는 무통신 시간이다 (REQ-CENTURY-014).
	OfflineTimeout time.Duration

	// AutoDiscovery 는 회선상 관측된 sub_dev_id 를 자동으로 디바이스로 등록한다 (REQ-CENTURY-013).
	AutoDiscovery bool

	// DedupeWrites 는 동일 polling cycle 내 중복 WRITE 프레임을 1개로 합친다 (REQ-CENTURY-027).
	DedupeWrites bool

	// CycleIdleTimeout 은 inter-frame idle 의 새 cycle 판정 임계값이다 (REQ-CENTURY-027).
	CycleIdleTimeout time.Duration

	// LogDecodeErrors 는 디코드 에러를 WARN 로그로 출력할지 여부이다.
	LogDecodeErrors bool

	// LogDrops 는 ring buffer drop 을 per-drop WARN 로그로 출력할지 여부이다.
	LogDrops bool

	// LogUnconfirmedFields 는 미확정 필드의 새 관측값을 DEBUG 로그로 출력할지 여부이다.
	LogUnconfirmedFields bool

	// LogStateUpdates 는 디바이스 state 갱신마다 디코드된 값(setpoint, current_temp, mode, fan,
	// evaporator temps 등)을 INFO 로그로 출력할지 여부이다. 운영 시 OFF, 진단 시 ON 권장.
	LogStateUpdates bool

	// LogStateChangesOnly 는 LogStateUpdates 와 함께 사용되는 진단 분석 모드 옵션이다 (v0.5).
	// true 이면 (sub_dev_id, register, role) 별로 직전에 로그된 raw payload 와 byte-equal
	// 비교하여 동일한 경우 로그 출력을 생략한다. 변경된 경우에는 변화한 byte 위치
	// 리스트(예: "data[2]", "data[7..8]") 를 추가 필드로 함께 출력한다.
	// 프로토콜 RE / fan 인코딩 탐색 등 byte 변화 탐지가 목적인 진단 작업에 유용.
	// LogStateUpdates=false 일 때는 효과 없음.
	LogStateChangesOnly bool

	// Devices 는 설정 파일에서 사전 등록된 디바이스 목록이다.
	// AutoDiscovery 가 false 여도 여기에 등재된 디바이스는 시작 시 등록된다.
	Devices []agent.DeviceEntry

	// EmitDeviceState 는 msgCh 에 device-centric DeviceStateEvent 를 emit 할지 여부이다 (REQ-CENTURY-034).
	// v0.3.0 기본값: true (1차 출력).
	//
	// v0.5.1 Breaking: register-decoded 별도 stream 이 제거되어 본 옵션이 사실상 항상 true.
	// false 로 설정하면 어떠한 device 정보도 출력되지 않는다 — 운영에서 권장하지 않음.
	EmitDeviceState bool

	// ControlEnabled 는 능동 제어 활성 여부이다 (placeholder, 2026-05-29).
	// 현재 Century 는 제어 미지원 (REQ-CENTURY-017) — control 노드는 항상 not_supported 반환.
	// LG/Samsung 와 UI 일관성 위해 config 필드만 노출. 항상 false.
	ControlEnabled bool

	// ReportInterval 은 device_state 의 fallback emit 주기이다 (REQ-CENTURY-035).
	// 변경 감지 없이 이 시간 경과 시 `trigger="keepalive"` emit. 0 이면 비활성 (change-only).
	// 권장 최소 30s (A16). EmitDeviceState=false 시 무시됨.
	ReportInterval time.Duration

	// ReportMode 는 keepalive emit 시점 계산 방식이다 (v0.3.9).
	//   - "relative" (기본): 마지막 emit 후 ReportInterval 경과 시 emit.
	//     agent.Start 시점부터 상대적인 간격으로 emit 된다.
	//   - "absolute": wall-clock 정렬 — 매 ReportInterval 의 정수 배수 시점에 emit
	//     (예: 60s 면 매 분 0초, 5m 면 0/5/10/15... 분 0초). linux crontab 패턴.
	//     디바이스가 여러 대일 때 emit 시점이 동기화되어 모니터링/로그 정렬에 유리.
	//
	// "absolute" 의 부작용: agent 시작 시점에 따라 첫 emit 까지 최대 ReportInterval 만큼
	// 대기할 수 있다 (다음 정렬 시점까지).
	ReportMode string

	// IncludeUnknownFields 는 register-decoded 메시지 페이로드에 confirmation_status="unknown"
	// 필드 (reg02_byte_*, reg03_pad_*, reg04_byte_*, write_byte_* 등 padding/reserved 바이트) 를
	// 포함할지 여부이다.
	//
	// v0.3.2 기본값: false — 운영 환경에서는 의미 없는 padding 바이트들이 페이로드 크기만
	// 늘려 trace 가독성을 해친다. 프로토콜 리버스 엔지니어링 / 디버깅 시에만 true 로 활성화.
	// EmitRegisterDecoded=false 시 무시됨 (register 메시지 자체가 emit 안 됨).
	//
	// v0.3.3: true 일 때 출력에 "unknown" 객체로 그룹화되어 노출된다.
	IncludeUnknownFields bool

	// IncludeInferredFields 는 register-decoded 메시지 페이로드에 confirmation_status="inferred"
	// 필드 (op_val_1, op_val_2, status_bits, reg04_word_10, reg04_const_*, reg02_live_*
	// 등 추정 의미 필드) 를 포함할지 여부이다.
	//
	// v0.3.3 기본값: false — 운영 환경에서는 추정값이 잡음으로 작용하여 trace 가독성을
	// 해친다. 추정 의미의 검증/모니터링 시에만 true 로 활성화.
	// true 일 때 출력에 "inferred" 객체로 그룹화되어 노출된다.
	IncludeInferredFields bool

	// IncludeRegisterInfo 는 register-decoded 메시지에 register 메타데이터 (register 번호,
	// direction) 를 포함할지 여부이다.
	//
	// v0.3.5 기본값: false — 운영 환경에서 device-state 중심 출력 시 register 번호는
	// 노이즈. 프로토콜 분석 / 디버깅 시에만 true 로 활성화. dev_id, timestamp_ms, state
	// 그룹은 옵션과 무관하게 항상 출력.
	IncludeRegisterInfo bool

	// IncludeRawHex 는 register-decoded 메시지 페이로드에 raw_hex (캡처된 원시 바이트의
	// hex 표현) 를 포함할지 여부이다.
	//
	// v0.3.5 기본값: false — 운영 환경에서 raw bytes 는 페이로드 크기를 늘리고 trace
	// 가독성을 해친다. 프로토콜 RE / 디버깅 시에만 true. century-raw-frame 노드는
	// 자체 목적이 raw bytes 노출이므로 이 옵션과 무관하게 항상 raw_hex 를 emit 한다.
	IncludeRawHex bool

	// EventTempThreshold 는 change 트리거 event 보고의 실내온도 변화 임계값이다 (단위: ℃, v0.6.6).
	//
	// change 감지 시 변경된 필드가 실내온도(current_temp)뿐이면
	// |curr_temp − lastReportTemp| >= EventTempThreshold 일 때만 emit 한다.
	// 온도 외 필드(power/mode/target_temp/fan_speed)가 함께 변경되면
	// 임계값과 무관하게 즉시 emit (기존 동작 유지).
	//
	// 기본 1.0℃. 0 이하면 게이트 비활성 (모든 change 즉시 emit — 이전 동작과 동일).
	// 정기 보고(report) emit 시점에도 lastReportTemp 가 갱신되어 임계값 누적 효과를 방지한다.
	EventTempThreshold float64
}

// parseHvacr01Config 는 AgentConfig.Transport.Options 맵에서 Hvacr01Config 를 파싱한다.
//
// 모든 필드는 선택적이며, 누락된 값은 SPEC §4 의 기본값으로 채워진다.
// transport_type 에 따라 필수 필드가 달라진다:
//   - serial:     serial_port 필수 (AC-D2)
//   - tcp-client: tcp_host + tcp_port 필수 (REQ-CENTURY-028)
//   - tcp-server: tcp_port 필수, tcp_host 기본 "0.0.0.0" (REQ-CENTURY-028)
//
// hex 문자열 ("0x0030") 과 정수 (48) 모두 주소 필드에 허용된다 (AC-D3).
//
// v0.2.0 (REQ-CENTURY-032): cycle_idle_timeout 의 default 는 transport-aware —
// serial=100ms, tcp-*=200ms. 사용자가 명시하면 transport 와 무관하게 그 값 사용.
func parseHvacr01Config(opts map[string]any) (Hvacr01Config, error) {
	// Reject deprecated alias keys with clear errors (2026-05-29 breaking).
	if _, ok := opts["reconnect_initial"]; ok {
		return Hvacr01Config{}, fmt.Errorf("century_hvacr01: deprecated option 'reconnect_initial' is removed; use 'reconnect_interval' instead")
	}
	if _, ok := opts["keepalive_interval"]; ok {
		return Hvacr01Config{}, fmt.Errorf("century_hvacr01: deprecated option 'keepalive_interval' is removed; use 'report_interval' instead")
	}
	if _, ok := opts["keepalive_mode"]; ok {
		return Hvacr01Config{}, fmt.Errorf("century_hvacr01: deprecated option 'keepalive_mode' is removed; use 'report_mode' instead")
	}

	cfg := Hvacr01Config{
		TransportType:  "serial",
		BaudRate:       9600,
		DataBits:       8,
		StopBits:       1,
		Parity:         "none",
		MasterAddress:  0x0030,
		SlaveAddress:   0x0001,
		SubDevID:       0x3B,
		RingBufferSize: 128,
		// 2026-05-29: offline_timeout 기본 5s → 30s (LG / Samsung 통일).
		OfflineTimeout:      30 * time.Second,
		AutoDiscovery:       true,
		DedupeWrites:        true,
		TCPConnectTimeout:   DefaultTCPConnectTimeout,
		TCPReadTimeout:      DefaultTCPReadTimeout,
		ReconnectInterval:   DefaultReconnectInterval,
		MaxReconnectBackoff: DefaultMaxReconnectBackoff,
		// v0.5.1 통합 schema: device_state 단일 출력 (register-decoded 제거됨).
		EmitDeviceState:       true,
		ReportInterval:        DefaultReportInterval,
		ReportMode:            "relative",
		IncludeUnknownFields:  false,
		IncludeInferredFields: false,
		IncludeRegisterInfo:   false,
		IncludeRawHex:         false,
		EventTempThreshold:    1.0,
		// CycleIdleTimeout intentionally left zero — resolved at the end based on
		// transport_type (REQ-CENTURY-032) unless explicitly set by the user.
	}

	if v, ok := opts["transport_type"]; ok {
		s, sok := v.(string)
		if !sok {
			return Hvacr01Config{}, fmt.Errorf("%w: transport_type must be a string", ErrUnknownTransportType)
		}
		cfg.TransportType = s
	}
	switch cfg.TransportType {
	case "serial", "tcp-client", "tcp-server":
		// valid transports (REQ-CENTURY-028)
	default:
		return Hvacr01Config{}, fmt.Errorf("%w: got %q", ErrUnknownTransportType, cfg.TransportType)
	}

	if v, ok := opts["serial_port"]; ok {
		if s, sok := v.(string); sok {
			cfg.SerialPort = s
		}
	}
	// serial_port is only required for serial transport (REQ-CENTURY-028).
	if cfg.TransportType == "serial" && cfg.SerialPort == "" {
		return Hvacr01Config{}, ErrSerialPortRequired
	}

	// --- TCP fields (REQ-CENTURY-028) ---
	if v, ok := opts["tcp_host"]; ok {
		if s, sok := v.(string); sok {
			cfg.TCPHost = s
		}
	}
	if v, ok := opts["tcp_port"]; ok {
		cfg.TCPPort = toInt(v)
	}
	if cfg.TransportType == "tcp-server" && cfg.TCPHost == "" {
		// Default bind for tcp-server.
		cfg.TCPHost = "0.0.0.0"
	}
	if cfg.TransportType == "tcp-client" && cfg.TCPHost == "" {
		return Hvacr01Config{}, ErrHvacr01TCPHostRequired
	}
	if cfg.TransportType == "tcp-client" || cfg.TransportType == "tcp-server" {
		if cfg.TCPPort < 1 || cfg.TCPPort > 65535 {
			return Hvacr01Config{}, fmt.Errorf("%w: got %d", ErrHvacr01TCPPortRequired, cfg.TCPPort)
		}
	}

	if v, ok := opts["tcp_connect_timeout"]; ok {
		d, err := parseDurationValue(v)
		if err != nil {
			return Hvacr01Config{}, fmt.Errorf("century_hvacr01: invalid tcp_connect_timeout: %w", err)
		}
		if d <= 0 {
			return Hvacr01Config{}, fmt.Errorf("century_hvacr01: tcp_connect_timeout must be > 0, got %s", d)
		}
		cfg.TCPConnectTimeout = d
	}
	if v, ok := opts["tcp_read_timeout"]; ok {
		d, err := parseDurationValue(v)
		if err != nil {
			return Hvacr01Config{}, fmt.Errorf("century_hvacr01: invalid tcp_read_timeout: %w", err)
		}
		if d <= 0 {
			return Hvacr01Config{}, fmt.Errorf("century_hvacr01: tcp_read_timeout must be > 0, got %s", d)
		}
		cfg.TCPReadTimeout = d
	}
	if v, ok := opts["reconnect_interval"]; ok {
		d, err := parseDurationValue(v)
		if err != nil {
			return Hvacr01Config{}, fmt.Errorf("century_hvacr01: invalid reconnect_interval: %w", err)
		}
		if d <= 0 {
			return Hvacr01Config{}, fmt.Errorf("century_hvacr01: reconnect_interval must be > 0, got %s", d)
		}
		cfg.ReconnectInterval = d
	}
	if v, ok := opts["max_reconnect_backoff"]; ok {
		d, err := parseDurationValue(v)
		if err != nil {
			return Hvacr01Config{}, fmt.Errorf("century_hvacr01: invalid max_reconnect_backoff: %w", err)
		}
		if d <= 0 {
			return Hvacr01Config{}, fmt.Errorf("century_hvacr01: max_reconnect_backoff must be > 0, got %s", d)
		}
		cfg.MaxReconnectBackoff = d
	}

	if v, ok := opts["baud_rate"]; ok {
		br := toInt(v)
		if br < 300 {
			return Hvacr01Config{}, fmt.Errorf("%w: got %d", ErrInvalidBaudRate, br)
		}
		cfg.BaudRate = br
	}
	if v, ok := opts["data_bits"]; ok {
		cfg.DataBits = toInt(v)
	}
	if v, ok := opts["stop_bits"]; ok {
		cfg.StopBits = toInt(v)
	}
	if v, ok := opts["parity"]; ok {
		if s, sok := v.(string); sok {
			cfg.Parity = s
		}
	}

	if v, ok := opts["master_address"]; ok {
		n, err := parseHexOrInt(v)
		if err != nil {
			return Hvacr01Config{}, fmt.Errorf("%w: master_address: %v", ErrInvalidAddress, err)
		}
		cfg.MasterAddress = uint16(n)
	}
	if v, ok := opts["slave_address"]; ok {
		n, err := parseHexOrInt(v)
		if err != nil {
			return Hvacr01Config{}, fmt.Errorf("%w: slave_address: %v", ErrInvalidAddress, err)
		}
		cfg.SlaveAddress = uint16(n)
	}
	if v, ok := opts["sub_dev_id"]; ok {
		n, err := parseHexOrInt(v)
		if err != nil {
			return Hvacr01Config{}, fmt.Errorf("%w: sub_dev_id: %v", ErrInvalidAddress, err)
		}
		cfg.SubDevID = byte(n)
	}

	if v, ok := opts["ring_buffer_size"]; ok {
		n := toInt(v)
		if n < 16 {
			return Hvacr01Config{}, fmt.Errorf("%w: got %d", ErrInvalidRingBufferSize, n)
		}
		cfg.RingBufferSize = n
	}

	if v, ok := opts["offline_timeout"]; ok {
		d, err := parseDurationValue(v)
		if err != nil {
			return Hvacr01Config{}, fmt.Errorf("century_hvacr01: invalid offline_timeout: %w", err)
		}
		if d <= 0 {
			return Hvacr01Config{}, fmt.Errorf("%w: got %s", ErrInvalidOfflineTimeout, d)
		}
		cfg.OfflineTimeout = d
	}

	// cycle_idle_timeout — explicit user value wins regardless of transport (REQ-CENTURY-032).
	// If unset, the default is resolved below based on TransportType.
	cycleIdleExplicit := false
	if v, ok := opts["cycle_idle_timeout"]; ok {
		d, err := parseDurationValue(v)
		if err != nil {
			return Hvacr01Config{}, fmt.Errorf("century_hvacr01: invalid cycle_idle_timeout: %w", err)
		}
		if d <= 0 {
			return Hvacr01Config{}, fmt.Errorf("%w: got %s", ErrInvalidCycleIdleTimeout, d)
		}
		cfg.CycleIdleTimeout = d
		cycleIdleExplicit = true
	}
	if !cycleIdleExplicit {
		// Transport-aware default (REQ-CENTURY-032).
		switch cfg.TransportType {
		case "tcp-client", "tcp-server":
			cfg.CycleIdleTimeout = DefaultCycleIdleTimeoutTCP
		default:
			cfg.CycleIdleTimeout = DefaultCycleIdleTimeoutSerial
		}
	}

	if v, ok := opts["auto_discovery"]; ok {
		if b, bok := v.(bool); bok {
			cfg.AutoDiscovery = b
		}
	}
	if v, ok := opts["dedupe_writes"]; ok {
		if b, bok := v.(bool); bok {
			cfg.DedupeWrites = b
		}
	}
	if v, ok := opts["log_decode_errors"]; ok {
		if b, bok := v.(bool); bok {
			cfg.LogDecodeErrors = b
		}
	}
	if v, ok := opts["log_drops"]; ok {
		if b, bok := v.(bool); bok {
			cfg.LogDrops = b
		}
	}
	if v, ok := opts["log_unconfirmed_fields"]; ok {
		if b, bok := v.(bool); bok {
			cfg.LogUnconfirmedFields = b
		}
	}
	if v, ok := opts["log_state_updates"]; ok {
		if b, bok := v.(bool); bok {
			cfg.LogStateUpdates = b
		}
	}
	if v, ok := opts["log_state_changes_only"]; ok {
		if b, bok := v.(bool); bok {
			cfg.LogStateChangesOnly = b
		}
	}

	cfg.Devices = agent.ParseDevices(opts)

	// v0.3.0 device-centric emit options (REQ-CENTURY-034, REQ-CENTURY-035).
	if v, ok := opts["emit_device_state"]; ok {
		if b, bok := v.(bool); bok {
			cfg.EmitDeviceState = b
		}
	}
	// control_enabled placeholder (2026-05-29) — Century 미지원이지만 UI 일관성 위해 파서 노출.
	if v, ok := opts["control_enabled"]; ok {
		if b, bok := v.(bool); bok {
			cfg.ControlEnabled = b
		}
	}
	// v0.5.1: emit_register_decoded 옵션 제거 — register-decoded stream 폐기.
	// 기존 옵션이 들어와도 silent ignore (deprecation grace).
	//
	// v0.6.0: report_interval — 상태보고 주기. 2026-05-29 단일화 (keepalive_interval alias 제거).
	//   "keepalive" 라는 명칭은 향후 세션 연결 관리 (TCP keepalive 등) 에 사용 예약.
	if v, ok := opts["report_interval"]; ok {
		d, err := parseDurationValue(v)
		if err != nil {
			return Hvacr01Config{}, fmt.Errorf("century_hvacr01: invalid report_interval: %w", err)
		}
		if d < 0 {
			return Hvacr01Config{}, fmt.Errorf("century_hvacr01: report_interval must be >= 0 (0=disabled), got %s", d)
		}
		cfg.ReportInterval = d
	}
	if v, ok := opts["include_unknown_fields"]; ok {
		if b, bok := v.(bool); bok {
			cfg.IncludeUnknownFields = b
		}
	}
	if v, ok := opts["include_inferred_fields"]; ok {
		if b, bok := v.(bool); bok {
			cfg.IncludeInferredFields = b
		}
	}
	if v, ok := opts["include_register_info"]; ok {
		if b, bok := v.(bool); bok {
			cfg.IncludeRegisterInfo = b
		}
	}
	if v, ok := opts["include_raw_hex"]; ok {
		if b, bok := v.(bool); bok {
			cfg.IncludeRawHex = b
		}
	}
	// v0.6.6: event_temp_threshold — 실내온도 변화 임계값 (단위 ℃, 기본 1.0).
	if v, ok := opts["event_temp_threshold"]; ok {
		f, err := toFloat64(v)
		if err != nil {
			return Hvacr01Config{}, fmt.Errorf("century_hvacr01: invalid event_temp_threshold: %w", err)
		}
		cfg.EventTempThreshold = f
	}
	// v0.6.0: report_mode — 상태보고 시점 정책. 2026-05-29 단일화 (keepalive_mode alias 제거).
	if v, ok := opts["report_mode"]; ok {
		if s, sok := v.(string); sok {
			switch s {
			case "relative", "absolute":
				cfg.ReportMode = s
			case "":
				// 빈 string 이면 default "relative" 유지
			default:
				return Hvacr01Config{}, fmt.Errorf("century_hvacr01: invalid report_mode %q (must be 'relative' or 'absolute')", s)
			}
		}
	}

	// Validation: device_state stream 이 enabled 여야 한다 (v0.5.1 — 유일한 emit stream).
	if !cfg.EmitDeviceState {
		return Hvacr01Config{}, ErrHvacr01NoOutputEnabled
	}

	return cfg, nil
}

// toFloat64 는 YAML/JSON 으로 들어온 수치 후보를 float64 로 변환한다 (v0.6.6).
// int / int64 / float64 / float32 / 숫자 문자열을 인식한다.
func toFloat64(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case string:
		var f float64
		if _, err := fmt.Sscanf(strings.TrimSpace(n), "%f", &f); err == nil {
			return f, nil
		}
		return 0, fmt.Errorf("not a valid number: %q", n)
	default:
		return 0, fmt.Errorf("expected number, got %T", v)
	}
}

// toInt 는 YAML/JSON 으로 들어온 정수 후보를 int 로 변환한다.
// int / float64 만 인식한다 (PHP/Python 등 다른 numeric 표현은 미지원).
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

// parseHexOrInt 는 hex 문자열 ("0x0030", "0X3B") 또는 정수 후보를 int 로 변환한다.
// AC-D3 의 hex 입력 허용 요구사항에 사용된다.
func parseHexOrInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	case string:
		s := strings.TrimSpace(n)
		if len(s) >= 2 && (s[:2] == "0x" || s[:2] == "0X") {
			var val int
			if _, err := fmt.Sscanf(s, "0x%x", &val); err == nil {
				return val, nil
			}
			if _, err := fmt.Sscanf(s, "0X%x", &val); err == nil {
				return val, nil
			}
			return 0, fmt.Errorf("not a valid hex literal: %q", n)
		}
		var val int
		if _, err := fmt.Sscanf(s, "%d", &val); err == nil {
			return val, nil
		}
		return 0, fmt.Errorf("not a valid integer: %q", n)
	default:
		return 0, fmt.Errorf("expected string or number, got %T", v)
	}
}

// parseDurationValue 는 time.Duration 또는 duration 문자열을 time.Duration 으로 파싱한다.
func parseDurationValue(v any) (time.Duration, error) {
	switch t := v.(type) {
	case time.Duration:
		return t, nil
	case string:
		return time.ParseDuration(t)
	default:
		return 0, fmt.Errorf("expected duration string, got %T", v)
	}
}
