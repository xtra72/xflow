package serial

import (
	"encoding/hex"
	"fmt"
	"strconv"
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
	IdleTimeout    time.Duration // 스트림 모드 유휴 타임아웃 (framing=stream 시)
	GapTimeout     time.Duration // 프레임 간격 타임아웃 (모든 프레이밍 모드, 0이면 미사용)
	BufferSize     int           // 읽기 버퍼 크기(바이트)
	Framing        string        // 프레이밍 타입 (raw, newline, length_prefix, fixed_size, stream, frame)
	Delimiter      byte          // 구분자 (framing=newline 시)
	FixedSize      int           // 고정 크기 (framing=fixed_size 시)
	MaxMessageSize int           // 최대 메시지 크기 (0=무제한)
	LogDrops       bool          // 수신 버퍼 가득 참으로 메시지 드롭 시 WARN 로그 출력 여부 (기본값 false — 운영 환경 noise 억제)
	LogMessages    bool          // 송/수신(TX/RX) 메시지를 hex 로 INFO 로그 출력 여부 (기본값 false, opt-in 진단용)

	// frame 프레이밍 설정 (framing=frame 시)
	STX                  []byte // 프레임 시작 마커 (hex 문자열에서 파싱)
	ETX                  []byte // 프레임 종료 마커 (빈 슬라이스면 검증 생략)
	LengthOffset         int    // STX 부터 길이 필드까지 오프셋
	LengthSize           int    // 길이 필드 크기 (1 또는 2)
	LengthEndian         string // 길이 필드 엔디안 ("big" 또는 "little")
	LengthIncludesHeader bool   // true 면 길이 = 헤더+페이로드 (STX~LEN 끝까지 차감)
	LengthAdjustment     int    // 디코딩된 길이에 더할 보정값 (length_includes_header 후 적용)
	Checksum             string // 체크섬 유형 ("none", "sum8", "xor")
}

// ParseSerialConfig 는 Transport.Options 맵에서 SerialConfig 를 파싱한다.
func ParseSerialConfig(opts map[string]any) (SerialConfig, error) {
	cfg := SerialConfig{
		BaudRate:       DefaultBaudRate,
		DataBits:       DefaultDataBits,
		StopBits:       DefaultStopBits,
		Parity:         DefaultParity,
		ReadTimeout:    DefaultReadTimeout,
		IdleTimeout:    DefaultIdleTimeout,
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

	// idle_timeout (스트림 모드 유휴 타임아웃, framing=stream 시)
	if v, ok := opts["idle_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return SerialConfig{}, fmt.Errorf("serial: invalid idle_timeout: %w", err)
		}
		cfg.IdleTimeout = d
	}

	// gap_timeout (프레임 간격 타임아웃, 모든 프레이밍 모드에서 사용 가능)
	// 단위: ns, ms, s, m (Go time.ParseDuration 지원)
	if v, ok := opts["gap_timeout"]; ok {
		d, err := time.ParseDuration(v.(string))
		if err != nil {
			return SerialConfig{}, fmt.Errorf("serial: invalid gap_timeout: %w", err)
		}
		cfg.GapTimeout = d
	}

	// buffer_size
	if v, ok := opts["buffer_size"]; ok {
		cfg.BufferSize = toInt(v)
	}

	// framing
	if v, ok := opts["framing"]; ok {
		if s := v.(string); s != "" {
			cfg.Framing = s
		}
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

	// log_drops (기본값: false — 운영 환경 noise 억제, 디버깅 시 true)
	// 수신 속도가 소비 속도를 초과해 msgCh 가 가득 차면 매 드롭마다 WARN 로그가
	// 발생해 로그 폭주로 이어지므로 옵션으로 끌 수 있게 한다.
	if v, ok := opts["log_drops"]; ok {
		cfg.LogDrops = toBool(v)
	}

	// log_messages (기본값: false) — 송/수신(TX/RX) 메시지를 hex 로 INFO 로그 출력.
	// opt-in 진단용이며 운영 환경에서는 로그 폭주 우려로 비활성 권장.
	if v, ok := opts["log_messages"]; ok {
		cfg.LogMessages = toBool(v)
	}

	// frame 프레이밍 전용 설정
	if cfg.Framing == FramingFrame {
		if err := parseFrameConfig(opts, &cfg); err != nil {
			return SerialConfig{}, err
		}
	}

	return cfg, nil
}

// parseFrameConfig 는 frame 프레이밍 전용 설정을 파싱한다.
func parseFrameConfig(opts map[string]any, cfg *SerialConfig) error {
	// stx (필수)
	if v, ok := opts["stx"]; ok {
		b, err := hex.DecodeString(v.(string))
		if err != nil || len(b) == 0 {
			return fmt.Errorf("%w", ErrInvalidSTX)
		}
		cfg.STX = b
	}
	if len(cfg.STX) == 0 {
		return fmt.Errorf("%w", ErrInvalidSTX)
	}

	// etx (선택)
	if v, ok := opts["etx"]; ok {
		s := v.(string)
		if s != "" {
			b, err := hex.DecodeString(s)
			if err != nil {
				return fmt.Errorf("%w", ErrETXMismatch)
			}
			cfg.ETX = b
		}
	}

	// length_offset (기본: len(stx))
	cfg.LengthOffset = len(cfg.STX)
	if v, ok := opts["length_offset"]; ok {
		cfg.LengthOffset = toInt(v)
	}

	// length_size (기본: 1)
	cfg.LengthSize = 1
	if v, ok := opts["length_size"]; ok {
		cfg.LengthSize = toInt(v)
	}
	if cfg.LengthSize != 1 && cfg.LengthSize != 2 {
		return fmt.Errorf("%w: %d", ErrInvalidLengthSize, cfg.LengthSize)
	}

	// length_endian (기본: "big")
	cfg.LengthEndian = "big"
	if v, ok := opts["length_endian"]; ok {
		cfg.LengthEndian = v.(string)
	}
	if !validLengthEndians[cfg.LengthEndian] {
		return fmt.Errorf("%w: %q", ErrInvalidEndian, cfg.LengthEndian)
	}

	// length_includes_header (기본: false)
	if v, ok := opts["length_includes_header"]; ok {
		cfg.LengthIncludesHeader = toBool(v)
	}

	// length_adjustment (기본: 0)
	if v, ok := opts["length_adjustment"]; ok {
		cfg.LengthAdjustment = toInt(v)
	}

	// checksum (기본: "none")
	cfg.Checksum = "none"
	if v, ok := opts["checksum"]; ok {
		cfg.Checksum = v.(string)
	}
	if !validChecksumTypes[cfg.Checksum] {
		return fmt.Errorf("%w: %q", ErrInvalidChecksum, cfg.Checksum)
	}

	return nil
}

// toInt 는 int, float64, 또는 string 값을 int 로 변환한다.
// YAML/JSON 파싱에서 숫자가 float64 로, Web UI select 필드에서 string 으로 전달될 수 있으므로
// 세 타입 모두 처리한다.
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	default:
		return 0
	}
}

// toBool 은 bool 또는 string 값을 bool 로 변환한다.
func toBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return b == "true" || b == "1"
	default:
		return false
	}
}
