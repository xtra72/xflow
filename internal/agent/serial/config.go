package serial

import (
	"fmt"
	"time"
)

// SerialConfig 는 시리얼 에이전트 설정이다.
type SerialConfig struct {
	Port           string        // 시리얼 포트 경로 (예: /dev/ttyUSB0, COM3)
	BaudRate       int           // 전송 속도
	DataBits       int           // 데이터 비트 (5, 6, 7, 8)
	StopBits       int           // 스톱 비트 (1, 2)
	Parity         string        // 패리티 (none, even, odd, mark, space)
	ReadTimeout    time.Duration // 읽기 타임아웃
	BufferSize     int           // 읽기 버퍼 크기(바이트)
	Framing        string        // 프레이밍 타입 (raw, newline, length_prefix, fixed_size)
	Delimiter      byte          // 구분자 (framing=newline 시)
	FixedSize      int           // 고정 크기 (framing=fixed_size 시)
	MaxMessageSize int           // 최대 메시지 크기 (0=무제한)
}

// ParseSerialConfig 는 Transport.Options 맵에서 SerialConfig 를 파싱한다.
func ParseSerialConfig(opts map[string]any) (SerialConfig, error) {
	cfg := SerialConfig{
		BaudRate:       DefaultBaudRate,
		DataBits:       DefaultDataBits,
		StopBits:       DefaultStopBits,
		Parity:         DefaultParity,
		ReadTimeout:    DefaultReadTimeout,
		BufferSize:     DefaultBufferSize,
		Framing:        FramingRaw,
		MaxMessageSize: DefaultMaxMessageSize,
	}

	// port (필수)
	if v, ok := opts["port"]; ok {
		cfg.Port = v.(string)
	}
	if cfg.Port == "" {
		return SerialConfig{}, fmt.Errorf("%w", ErrPortRequired)
	}

	// baud_rate
	if v, ok := opts["baud_rate"]; ok {
		cfg.BaudRate = toInt(v)
	}
	if !validBaudRates[cfg.BaudRate] {
		return SerialConfig{}, fmt.Errorf("%w: %d", ErrInvalidBaudRate, cfg.BaudRate)
	}

	// data_bits
	if v, ok := opts["data_bits"]; ok {
		cfg.DataBits = toInt(v)
	}
	if !validDataBits[cfg.DataBits] {
		return SerialConfig{}, fmt.Errorf("%w: %d", ErrInvalidDataBits, cfg.DataBits)
	}

	// stop_bits
	if v, ok := opts["stop_bits"]; ok {
		cfg.StopBits = toInt(v)
	}
	if !validStopBits[cfg.StopBits] {
		return SerialConfig{}, fmt.Errorf("%w: %d", ErrInvalidStopBits, cfg.StopBits)
	}

	// parity
	if v, ok := opts["parity"]; ok {
		cfg.Parity = v.(string)
	}
	if !validParities[cfg.Parity] {
		return SerialConfig{}, fmt.Errorf("%w: %q", ErrInvalidParity, cfg.Parity)
	}

	// read_timeout
	if v, ok := opts["read_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return SerialConfig{}, fmt.Errorf("serial: invalid read_timeout: %w", err)
		}
		cfg.ReadTimeout = d
	}

	// buffer_size
	if v, ok := opts["buffer_size"]; ok {
		cfg.BufferSize = toInt(v)
	}

	// framing
	if v, ok := opts["framing"]; ok {
		cfg.Framing = v.(string)
	}
	if !validFramingTypes[cfg.Framing] {
		return SerialConfig{}, fmt.Errorf("%w: %q", ErrInvalidFraming, cfg.Framing)
	}

	// delimiter
	if v, ok := opts["delimiter"]; ok {
		cfg.Delimiter = byte(toInt(v))
	}

	// fixed_size
	if v, ok := opts["fixed_size"]; ok {
		cfg.FixedSize = toInt(v)
	}
	if cfg.Framing == FramingFixedSize && cfg.FixedSize <= 0 {
		return SerialConfig{}, fmt.Errorf("%w", ErrFixedSizeRequired)
	}

	// max_message_size
	if v, ok := opts["max_message_size"]; ok {
		cfg.MaxMessageSize = toInt(v)
	}

	return cfg, nil
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
