package century

import (
	"fmt"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// CenturyConfig 는 Century HVAC 패시브 캡처 에이전트의 설정이다 (REQ-CENTURY-002).
//
// 모든 필드는 AgentConfig.Transport.Options 맵에서 parseCenturyConfig 로 채워지며,
// SPEC-CENTURY-001 §4 의 YAML 예시와 1:1 매핑된다.
type CenturyConfig struct {
	// TransportType 는 트랜스포트 종류이다. v0.1.0 은 "serial" 만 지원한다.
	TransportType string

	// SerialPort 는 시리얼 트랜스포트의 디바이스 경로이다 (예: /dev/ttyUSB0).
	// transport_type=serial 인 경우 필수.
	SerialPort string

	// BaudRate / DataBits / StopBits / Parity 는 시리얼 라인 파라미터이다.
	BaudRate int
	DataBits int
	StopBits int
	Parity   string

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
// 단, serial_port 는 transport_type=serial 인 경우 필수이다 (AC-D2).
//
// hex 문자열 ("0x0030") 과 정수 (48) 모두 주소 필드에 허용된다 (AC-D3).
func parseCenturyConfig(opts map[string]any) (CenturyConfig, error) {
	cfg := CenturyConfig{
		TransportType:    "serial",
		BaudRate:         9600,
		DataBits:         8,
		StopBits:         1,
		Parity:           "none",
		MasterAddress:    0x0030,
		SlaveAddress:     0x0001,
		SubDevID:         0x3B,
		RingBufferSize:   128,
		OfflineTimeout:   5 * time.Second,
		AutoDiscovery:    true,
		DedupeWrites:     true,
		CycleIdleTimeout: 100 * time.Millisecond,
	}

	if v, ok := opts["transport_type"]; ok {
		s, sok := v.(string)
		if !sok {
			return CenturyConfig{}, fmt.Errorf("%w: transport_type must be a string", ErrUnknownTransportType)
		}
		cfg.TransportType = s
	}
	if cfg.TransportType != "serial" {
		return CenturyConfig{}, fmt.Errorf("%w: got %q", ErrUnknownTransportType, cfg.TransportType)
	}

	if v, ok := opts["serial_port"]; ok {
		if s, sok := v.(string); sok {
			cfg.SerialPort = s
		}
	}
	if cfg.TransportType == "serial" && cfg.SerialPort == "" {
		return CenturyConfig{}, ErrSerialPortRequired
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

	if v, ok := opts["cycle_idle_timeout"]; ok {
		d, err := parseDurationValue(v)
		if err != nil {
			return CenturyConfig{}, fmt.Errorf("century: invalid cycle_idle_timeout: %w", err)
		}
		if d <= 0 {
			return CenturyConfig{}, fmt.Errorf("%w: got %s", ErrInvalidCycleIdleTimeout, d)
		}
		cfg.CycleIdleTimeout = d
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
