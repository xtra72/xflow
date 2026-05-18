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

	// DefaultReconnectInitial 은 tcp-client 의 exponential backoff 초기 슬립이다 (REQ-CENTURY-031).
	DefaultReconnectInitial = 5 * time.Second

	// DefaultMaxReconnectBackoff 는 tcp-client backoff 의 상한이다 (REQ-CENTURY-031).
	DefaultMaxReconnectBackoff = 5 * time.Minute
)

// CenturyConfig 는 Century HVAC 패시브 캡처 에이전트의 설정이다 (REQ-CENTURY-002, REQ-CENTURY-028).
//
// 모든 필드는 AgentConfig.Transport.Options 맵에서 parseCenturyConfig 로 채워지며,
// SPEC-CENTURY-001 §4 의 YAML 예시와 1:1 매핑된다.
//
// v0.2.0 (M6): TCP transport 지원 — TransportType 이 "serial" / "tcp-client" / "tcp-server"
// 중 하나를 가질 수 있으며, tcp-* 모드에서는 SerialPort 가 무시되고 TCPHost / TCPPort
// 등이 사용된다.
type CenturyConfig struct {
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

	// ReconnectInitial 는 tcp-client 의 exponential backoff 초기 슬립이다 (REQ-CENTURY-031).
	// 기본값: DefaultReconnectInitial (5s).
	ReconnectInitial time.Duration

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

	// Devices 는 설정 파일에서 사전 등록된 디바이스 목록이다.
	// AutoDiscovery 가 false 여도 여기에 등재된 디바이스는 시작 시 등록된다.
	Devices []agent.DeviceEntry
}

// parseCenturyConfig 는 AgentConfig.Transport.Options 맵에서 CenturyConfig 를 파싱한다.
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
func parseCenturyConfig(opts map[string]any) (CenturyConfig, error) {
	cfg := CenturyConfig{
		TransportType:       "serial",
		BaudRate:            9600,
		DataBits:            8,
		StopBits:            1,
		Parity:              "none",
		MasterAddress:       0x0030,
		SlaveAddress:        0x0001,
		SubDevID:            0x3B,
		RingBufferSize:      128,
		OfflineTimeout:      5 * time.Second,
		AutoDiscovery:       true,
		DedupeWrites:        true,
		TCPConnectTimeout:   DefaultTCPConnectTimeout,
		TCPReadTimeout:      DefaultTCPReadTimeout,
		ReconnectInitial:    DefaultReconnectInitial,
		MaxReconnectBackoff: DefaultMaxReconnectBackoff,
		// CycleIdleTimeout intentionally left zero — resolved at the end based on
		// transport_type (REQ-CENTURY-032) unless explicitly set by the user.
	}

	if v, ok := opts["transport_type"]; ok {
		s, sok := v.(string)
		if !sok {
			return CenturyConfig{}, fmt.Errorf("%w: transport_type must be a string", ErrUnknownTransportType)
		}
		cfg.TransportType = s
	}
	switch cfg.TransportType {
	case "serial", "tcp-client", "tcp-server":
		// valid transports (REQ-CENTURY-028)
	default:
		return CenturyConfig{}, fmt.Errorf("%w: got %q", ErrUnknownTransportType, cfg.TransportType)
	}

	if v, ok := opts["serial_port"]; ok {
		if s, sok := v.(string); sok {
			cfg.SerialPort = s
		}
	}
	// serial_port is only required for serial transport (REQ-CENTURY-028).
	if cfg.TransportType == "serial" && cfg.SerialPort == "" {
		return CenturyConfig{}, ErrSerialPortRequired
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
		return CenturyConfig{}, ErrCenturyTCPHostRequired
	}
	if cfg.TransportType == "tcp-client" || cfg.TransportType == "tcp-server" {
		if cfg.TCPPort < 1 || cfg.TCPPort > 65535 {
			return CenturyConfig{}, fmt.Errorf("%w: got %d", ErrCenturyTCPPortRequired, cfg.TCPPort)
		}
	}

	if v, ok := opts["tcp_connect_timeout"]; ok {
		d, err := parseDurationValue(v)
		if err != nil {
			return CenturyConfig{}, fmt.Errorf("century: invalid tcp_connect_timeout: %w", err)
		}
		if d <= 0 {
			return CenturyConfig{}, fmt.Errorf("century: tcp_connect_timeout must be > 0, got %s", d)
		}
		cfg.TCPConnectTimeout = d
	}
	if v, ok := opts["tcp_read_timeout"]; ok {
		d, err := parseDurationValue(v)
		if err != nil {
			return CenturyConfig{}, fmt.Errorf("century: invalid tcp_read_timeout: %w", err)
		}
		if d <= 0 {
			return CenturyConfig{}, fmt.Errorf("century: tcp_read_timeout must be > 0, got %s", d)
		}
		cfg.TCPReadTimeout = d
	}
	if v, ok := opts["reconnect_initial"]; ok {
		d, err := parseDurationValue(v)
		if err != nil {
			return CenturyConfig{}, fmt.Errorf("century: invalid reconnect_initial: %w", err)
		}
		if d <= 0 {
			return CenturyConfig{}, fmt.Errorf("century: reconnect_initial must be > 0, got %s", d)
		}
		cfg.ReconnectInitial = d
	}
	if v, ok := opts["max_reconnect_backoff"]; ok {
		d, err := parseDurationValue(v)
		if err != nil {
			return CenturyConfig{}, fmt.Errorf("century: invalid max_reconnect_backoff: %w", err)
		}
		if d <= 0 {
			return CenturyConfig{}, fmt.Errorf("century: max_reconnect_backoff must be > 0, got %s", d)
		}
		cfg.MaxReconnectBackoff = d
	}

	if v, ok := opts["baud_rate"]; ok {
		br := toInt(v)
		if br < 300 {
			return CenturyConfig{}, fmt.Errorf("%w: got %d", ErrInvalidBaudRate, br)
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
			return CenturyConfig{}, fmt.Errorf("%w: master_address: %v", ErrInvalidAddress, err)
		}
		cfg.MasterAddress = uint16(n)
	}
	if v, ok := opts["slave_address"]; ok {
		n, err := parseHexOrInt(v)
		if err != nil {
			return CenturyConfig{}, fmt.Errorf("%w: slave_address: %v", ErrInvalidAddress, err)
		}
		cfg.SlaveAddress = uint16(n)
	}
	if v, ok := opts["sub_dev_id"]; ok {
		n, err := parseHexOrInt(v)
		if err != nil {
			return CenturyConfig{}, fmt.Errorf("%w: sub_dev_id: %v", ErrInvalidAddress, err)
		}
		cfg.SubDevID = byte(n)
	}

	if v, ok := opts["ring_buffer_size"]; ok {
		n := toInt(v)
		if n < 16 {
			return CenturyConfig{}, fmt.Errorf("%w: got %d", ErrInvalidRingBufferSize, n)
		}
		cfg.RingBufferSize = n
	}

	if v, ok := opts["offline_timeout"]; ok {
		d, err := parseDurationValue(v)
		if err != nil {
			return CenturyConfig{}, fmt.Errorf("century: invalid offline_timeout: %w", err)
		}
		if d <= 0 {
			return CenturyConfig{}, fmt.Errorf("%w: got %s", ErrInvalidOfflineTimeout, d)
		}
		cfg.OfflineTimeout = d
	}

	// cycle_idle_timeout — explicit user value wins regardless of transport (REQ-CENTURY-032).
	// If unset, the default is resolved below based on TransportType.
	cycleIdleExplicit := false
	if v, ok := opts["cycle_idle_timeout"]; ok {
		d, err := parseDurationValue(v)
		if err != nil {
			return CenturyConfig{}, fmt.Errorf("century: invalid cycle_idle_timeout: %w", err)
		}
		if d <= 0 {
			return CenturyConfig{}, fmt.Errorf("%w: got %s", ErrInvalidCycleIdleTimeout, d)
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

	cfg.Devices = agent.ParseDevices(opts)

	return cfg, nil
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
