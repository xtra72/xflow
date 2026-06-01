package socket

import (
	"fmt"
	"strconv"
	"time"
)

// SocketConfig 는 모든 소켓 에이전트의 공통 설정이다.
type SocketConfig struct {
	Host       string // 바인드/연결 호스트 주소
	Port       int    // 포트 번호 (필수)
	BufferSize int    // 읽기 버퍼 크기 (바이트)
}

// TCPConfig 는 TCP 소켓의 공통 설정이다.
type TCPConfig struct {
	SocketConfig
	Framing        string // 프레이밍 타입: raw, newline, length_prefix, fixed_size
	Delimiter      byte   // 커스텀 구분자 (framing=newline 시 기본 '\n' 대체)
	FixedSize      int    // 고정 크기 (framing=fixed_size 시 필수)
	MaxMessageSize int    // 최대 메시지 크기 (0=무제한)
}

// TCPServerConfig 는 TCP 서버 에이전트 설정이다.
type TCPServerConfig struct {
	TCPConfig
	MaxConnections int  // 최대 동시 연결 수 (0=무제한)
	Broadcast      bool // true 시 송신 데이터를 모든 연결 클라이언트에 전송 (Target 무시)
}

// TCPClientConfig 는 TCP 클라이언트 에이전트 설정이다.
type TCPClientConfig struct {
	TCPConfig
	ReconnectInterval time.Duration // 재연결 간격
	MaxRetries        int           // 최대 재시도 횟수 (0=무한)
	ConnectTimeout    time.Duration // 연결 타임아웃
}

// UDPServerConfig 는 UDP 서버 에이전트 설정이다.
type UDPServerConfig struct {
	SocketConfig
}

// UDPClientConfig 는 UDP 클라이언트 에이전트 설정이다.
type UDPClientConfig struct {
	SocketConfig
}

// validFramingTypes 는 지원되는 프레이밍 타입 목록이다.
var validFramingTypes = map[string]bool{
	FramingRaw:          true,
	FramingNewline:      true,
	FramingLengthPrefix: true,
	FramingFixedSize:    true,
}

// ParseTCPServerConfig 는 TransportConfig.Options 맵에서 TCPServerConfig 를 파싱한다.
func ParseTCPServerConfig(opts map[string]any) (TCPServerConfig, error) {
	tcp, err := parseTCPConfig(opts, DefaultHost)
	if err != nil {
		return TCPServerConfig{}, err
	}
	cfg := TCPServerConfig{TCPConfig: tcp}
	if v, ok := opts["max_connections"]; ok {
		cfg.MaxConnections = toInt(v)
	}
	if v, ok := opts["broadcast"]; ok {
		cfg.Broadcast = toBool(v)
	}
	return cfg, nil
}

// ParseTCPClientConfig 는 TransportConfig.Options 맵에서 TCPClientConfig 를 파싱한다.
func ParseTCPClientConfig(opts map[string]any) (TCPClientConfig, error) {
	tcp, err := parseTCPConfig(opts, DefaultClientHost)
	if err != nil {
		return TCPClientConfig{}, err
	}
	cfg := TCPClientConfig{
		TCPConfig:         tcp,
		ReconnectInterval: DefaultReconnectInterval,
		ConnectTimeout:    DefaultConnectTimeout,
	}
	if v, ok := opts["reconnect_interval"]; ok {
		d, parseErr := time.ParseDuration(v.(string))
		if parseErr != nil {
			return TCPClientConfig{}, fmt.Errorf("%w: invalid reconnect_interval: %v", ErrInvalidConfig, parseErr)
		}
		cfg.ReconnectInterval = d
	}
	if v, ok := opts["max_retries"]; ok {
		cfg.MaxRetries = toInt(v)
	}
	if v, ok := opts["connect_timeout"]; ok {
		d, parseErr := time.ParseDuration(v.(string))
		if parseErr != nil {
			return TCPClientConfig{}, fmt.Errorf("%w: invalid connect_timeout: %v", ErrInvalidConfig, parseErr)
		}
		cfg.ConnectTimeout = d
	}
	return cfg, nil
}

// ParseUDPServerConfig 는 TransportConfig.Options 맵에서 UDPServerConfig 를 파싱한다.
func ParseUDPServerConfig(opts map[string]any) (UDPServerConfig, error) {
	sc, err := parseSocketConfig(opts, DefaultHost)
	if err != nil {
		return UDPServerConfig{}, err
	}
	return UDPServerConfig{SocketConfig: sc}, nil
}

// ParseUDPClientConfig 는 TransportConfig.Options 맵에서 UDPClientConfig 를 파싱한다.
func ParseUDPClientConfig(opts map[string]any) (UDPClientConfig, error) {
	sc, err := parseSocketConfig(opts, DefaultClientHost)
	if err != nil {
		return UDPClientConfig{}, err
	}
	return UDPClientConfig{SocketConfig: sc}, nil
}

// parseSocketConfig 는 공통 소켓 설정을 파싱한다.
func parseSocketConfig(opts map[string]any, defaultHost string) (SocketConfig, error) {
	cfg := SocketConfig{
		Host:       defaultHost,
		BufferSize: DefaultBufferSize,
	}
	if v, ok := opts["host"]; ok {
		cfg.Host = v.(string)
	}
	if v, ok := opts["port"]; ok {
		cfg.Port = toInt(v)
	}
	if cfg.Port <= 0 {
		return SocketConfig{}, fmt.Errorf("%w: port is required", ErrPortRequired)
	}
	if v, ok := opts["buffer_size"]; ok {
		cfg.BufferSize = toInt(v)
	}
	return cfg, nil
}

// parseTCPConfig 는 TCP 공통 설정을 파싱한다.
func parseTCPConfig(opts map[string]any, defaultHost string) (TCPConfig, error) {
	sc, err := parseSocketConfig(opts, defaultHost)
	if err != nil {
		return TCPConfig{}, err
	}
	cfg := TCPConfig{
		SocketConfig:   sc,
		Framing:        FramingRaw,
		MaxMessageSize: DefaultMaxMessageSize,
	}
	if v, ok := opts["framing"]; ok {
		cfg.Framing = v.(string)
	}
	if !validFramingTypes[cfg.Framing] {
		return TCPConfig{}, fmt.Errorf("%w: %q", ErrInvalidFraming, cfg.Framing)
	}
	if v, ok := opts["delimiter"]; ok {
		cfg.Delimiter = byte(toInt(v))
	}
	if v, ok := opts["fixed_size"]; ok {
		cfg.FixedSize = toInt(v)
	}
	if cfg.Framing == FramingFixedSize && cfg.FixedSize <= 0 {
		return TCPConfig{}, fmt.Errorf("%w", ErrFixedSizeRequired)
	}
	if v, ok := opts["max_message_size"]; ok {
		cfg.MaxMessageSize = toInt(v)
	}
	return cfg, nil
}

// toBool 는 bool 또는 문자열 "true"/"false" 값을 bool 로 변환한다.
// YAML/JSON 또는 Web UI 입력에서 bool 이 문자열로 전달될 수 있으므로 두 형태 모두 처리한다.
// 인식 불가한 값은 false 로 처리한다.
func toBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		parsed, err := strconv.ParseBool(b)
		if err != nil {
			return false
		}
		return parsed
	default:
		return false
	}
}

// toInt 는 int 또는 float64 값을 int 로 변환한다.
// YAML/JSON 파싱에서 숫자가 float64 로 전달될 수 있으므로 두 타입 모두 처리한다.
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}
