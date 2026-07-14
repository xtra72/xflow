package serial

import (
	"errors"
	"testing"
	"time"
)

func TestParseSerialConfig(t *testing.T) {
	tests := []struct {
		name    string
		opts    map[string]any
		want    SerialConfig
		wantErr error
	}{
		{
			name: "포트만 지정하면 기본값 적용",
			opts: map[string]any{
				"port": "/dev/ttyUSB0",
			},
			want: SerialConfig{
				Port:           "/dev/ttyUSB0",
				BaudRate:       DefaultBaudRate,
				DataBits:       DefaultDataBits,
				StopBits:       DefaultStopBits,
				Parity:         DefaultParity,
				ReadTimeout:    DefaultReadTimeout,
				BufferSize:     DefaultBufferSize,
				Framing:        FramingRaw,
				MaxMessageSize: DefaultMaxMessageSize,
				IdleTimeout:    DefaultIdleTimeout,
			},
		},
		{
			name: "모든 필드 설정",
			opts: map[string]any{
				"port":             "/dev/ttyS0",
				"baud_rate":        float64(115200),
				"data_bits":        float64(7),
				"stop_bits":        float64(2),
				"parity":           "even",
				"read_timeout":     "500ms",
				"buffer_size":      float64(8192),
				"framing":          "newline",
				"delimiter":        float64('\t'),
				"max_message_size": float64(2048),
			},
			want: SerialConfig{
				Port:           "/dev/ttyS0",
				BaudRate:       115200,
				DataBits:       7,
				StopBits:       2,
				Parity:         "even",
				ReadTimeout:    500 * time.Millisecond,
				BufferSize:     8192,
				Framing:        FramingNewline,
				Delimiter:      '\t',
				MaxMessageSize: 2048,
				IdleTimeout:    DefaultIdleTimeout,
			},
		},
		{
			name:    "포트 미지정 오류",
			opts:    map[string]any{},
			wantErr: ErrPortRequired,
		},
		{
			name: "빈 포트 문자열 오류",
			opts: map[string]any{
				"port": "",
			},
			wantErr: ErrPortRequired,
		},
		{
			name: "빈 framing 문자열은 기본값(raw) 적용",
			opts: map[string]any{
				"port":    "/dev/ttyUSB0",
				"framing": "",
			},
			want: SerialConfig{
				Port:           "/dev/ttyUSB0",
				BaudRate:       DefaultBaudRate,
				DataBits:       DefaultDataBits,
				StopBits:       DefaultStopBits,
				Parity:         DefaultParity,
				ReadTimeout:    DefaultReadTimeout,
				BufferSize:     DefaultBufferSize,
				Framing:        FramingRaw,
				MaxMessageSize: DefaultMaxMessageSize,
				IdleTimeout:    DefaultIdleTimeout,
			},
		},
		{
			name: "잘못된 보드레이트 오류",
			opts: map[string]any{
				"port":      "/dev/ttyUSB0",
				"baud_rate": float64(12345),
			},
			wantErr: ErrInvalidBaudRate,
		},
		{
			name: "잘못된 데이터 비트 오류",
			opts: map[string]any{
				"port":      "/dev/ttyUSB0",
				"data_bits": float64(9),
			},
			wantErr: ErrInvalidDataBits,
		},
		{
			name: "잘못된 스톱 비트 오류",
			opts: map[string]any{
				"port":      "/dev/ttyUSB0",
				"stop_bits": float64(3),
			},
			wantErr: ErrInvalidStopBits,
		},
		{
			name: "잘못된 패리티 오류",
			opts: map[string]any{
				"port":   "/dev/ttyUSB0",
				"parity": "invalid",
			},
			wantErr: ErrInvalidParity,
		},
		{
			name: "잘못된 프레이밍 타입 오류",
			opts: map[string]any{
				"port":    "/dev/ttyUSB0",
				"framing": "unknown",
			},
			wantErr: ErrInvalidFraming,
		},
		{
			name: "fixed_size 프레이밍에서 크기 미지정 오류",
			opts: map[string]any{
				"port":    "/dev/ttyUSB0",
				"framing": "fixed_size",
			},
			wantErr: ErrFixedSizeRequired,
		},
		{
			name: "fixed_size 프레이밍 정상 설정",
			opts: map[string]any{
				"port":       "/dev/ttyUSB0",
				"framing":    "fixed_size",
				"fixed_size": float64(256),
			},
			want: SerialConfig{
				Port:           "/dev/ttyUSB0",
				BaudRate:       DefaultBaudRate,
				DataBits:       DefaultDataBits,
				StopBits:       DefaultStopBits,
				Parity:         DefaultParity,
				ReadTimeout:    DefaultReadTimeout,
				BufferSize:     DefaultBufferSize,
				Framing:        FramingFixedSize,
				FixedSize:      256,
				MaxMessageSize: DefaultMaxMessageSize,
				IdleTimeout:    DefaultIdleTimeout,
			},
		},
		{
			name: "read_timeout 파싱",
			opts: map[string]any{
				"port":         "/dev/ttyUSB0",
				"read_timeout": "2s",
			},
			want: SerialConfig{
				Port:           "/dev/ttyUSB0",
				BaudRate:       DefaultBaudRate,
				DataBits:       DefaultDataBits,
				StopBits:       DefaultStopBits,
				Parity:         DefaultParity,
				ReadTimeout:    2 * time.Second,
				BufferSize:     DefaultBufferSize,
				Framing:        FramingRaw,
				MaxMessageSize: DefaultMaxMessageSize,
				IdleTimeout:    DefaultIdleTimeout,
			},
		},
		{
			name: "잘못된 read_timeout 오류",
			opts: map[string]any{
				"port":         "/dev/ttyUSB0",
				"read_timeout": "not_a_duration",
			},
			wantErr: nil, // read_timeout 파싱 오류는 센티넬이 아닌 일반 오류
		},
		{
			name: "float64 숫자 변환 (JSON/YAML 호환)",
			opts: map[string]any{
				"port":        "/dev/ttyUSB0",
				"baud_rate":   float64(57600),
				"data_bits":   float64(8),
				"stop_bits":   float64(1),
				"buffer_size": float64(2048),
			},
			want: SerialConfig{
				Port:           "/dev/ttyUSB0",
				BaudRate:       57600,
				DataBits:       8,
				StopBits:       1,
				Parity:         DefaultParity,
				ReadTimeout:    DefaultReadTimeout,
				BufferSize:     2048,
				Framing:        FramingRaw,
				MaxMessageSize: DefaultMaxMessageSize,
				IdleTimeout:    DefaultIdleTimeout,
			},
		},
		{
			name: "정수 타입도 처리",
			opts: map[string]any{
				"port":      "/dev/ttyUSB0",
				"baud_rate": 9600,
				"data_bits": 8,
			},
			want: SerialConfig{
				Port:           "/dev/ttyUSB0",
				BaudRate:       9600,
				DataBits:       8,
				StopBits:       DefaultStopBits,
				Parity:         DefaultParity,
				ReadTimeout:    DefaultReadTimeout,
				BufferSize:     DefaultBufferSize,
				Framing:        FramingRaw,
				MaxMessageSize: DefaultMaxMessageSize,
				IdleTimeout:    DefaultIdleTimeout,
			},
		},
		{
			name: "length_prefix 프레이밍",
			opts: map[string]any{
				"port":    "/dev/ttyUSB0",
				"framing": "length_prefix",
			},
			want: SerialConfig{
				Port:           "/dev/ttyUSB0",
				BaudRate:       DefaultBaudRate,
				DataBits:       DefaultDataBits,
				StopBits:       DefaultStopBits,
				Parity:         DefaultParity,
				ReadTimeout:    DefaultReadTimeout,
				BufferSize:     DefaultBufferSize,
				Framing:        FramingLengthPrefix,
				MaxMessageSize: DefaultMaxMessageSize,
				IdleTimeout:    DefaultIdleTimeout,
			},
		},
		{
			name: "Windows COM 포트",
			opts: map[string]any{
				"port": "COM3",
			},
			want: SerialConfig{
				Port:           "COM3",
				BaudRate:       DefaultBaudRate,
				DataBits:       DefaultDataBits,
				StopBits:       DefaultStopBits,
				Parity:         DefaultParity,
				ReadTimeout:    DefaultReadTimeout,
				BufferSize:     DefaultBufferSize,
				Framing:        FramingRaw,
				MaxMessageSize: DefaultMaxMessageSize,
				IdleTimeout:    DefaultIdleTimeout,
			},
		},
		{
			name: "모든 패리티 값 - odd",
			opts: map[string]any{
				"port":   "/dev/ttyUSB0",
				"parity": "odd",
			},
			want: SerialConfig{
				Port:           "/dev/ttyUSB0",
				BaudRate:       DefaultBaudRate,
				DataBits:       DefaultDataBits,
				StopBits:       DefaultStopBits,
				Parity:         "odd",
				ReadTimeout:    DefaultReadTimeout,
				BufferSize:     DefaultBufferSize,
				Framing:        FramingRaw,
				MaxMessageSize: DefaultMaxMessageSize,
				IdleTimeout:    DefaultIdleTimeout,
			},
		},
		{
			name: "모든 패리티 값 - mark",
			opts: map[string]any{
				"port":   "/dev/ttyUSB0",
				"parity": "mark",
			},
			want: SerialConfig{
				Port:           "/dev/ttyUSB0",
				BaudRate:       DefaultBaudRate,
				DataBits:       DefaultDataBits,
				StopBits:       DefaultStopBits,
				Parity:         "mark",
				ReadTimeout:    DefaultReadTimeout,
				BufferSize:     DefaultBufferSize,
				Framing:        FramingRaw,
				MaxMessageSize: DefaultMaxMessageSize,
				IdleTimeout:    DefaultIdleTimeout,
			},
		},
		{
			name: "stream 프레이밍 기본 idle_timeout",
			opts: map[string]any{
				"port":    "/dev/ttyUSB0",
				"framing": "stream",
			},
			want: SerialConfig{
				Port:           "/dev/ttyUSB0",
				BaudRate:       DefaultBaudRate,
				DataBits:       DefaultDataBits,
				StopBits:       DefaultStopBits,
				Parity:         DefaultParity,
				ReadTimeout:    DefaultReadTimeout,
				BufferSize:     DefaultBufferSize,
				Framing:        FramingStream,
				MaxMessageSize: DefaultMaxMessageSize,
				IdleTimeout:    DefaultIdleTimeout,
			},
		},
		{
			name: "stream 프레이밍 커스텀 idle_timeout",
			opts: map[string]any{
				"port":         "/dev/ttyUSB0",
				"framing":      "stream",
				"idle_timeout": "5ms",
			},
			want: SerialConfig{
				Port:           "/dev/ttyUSB0",
				BaudRate:       DefaultBaudRate,
				DataBits:       DefaultDataBits,
				StopBits:       DefaultStopBits,
				Parity:         DefaultParity,
				ReadTimeout:    DefaultReadTimeout,
				BufferSize:     DefaultBufferSize,
				Framing:        FramingStream,
				MaxMessageSize: DefaultMaxMessageSize,
				IdleTimeout:    5 * time.Millisecond,
			},
		},
		{
			name: "모든 패리티 값 - space",
			opts: map[string]any{
				"port":   "/dev/ttyUSB0",
				"parity": "space",
			},
			want: SerialConfig{
				Port:           "/dev/ttyUSB0",
				BaudRate:       DefaultBaudRate,
				DataBits:       DefaultDataBits,
				StopBits:       DefaultStopBits,
				Parity:         "space",
				ReadTimeout:    DefaultReadTimeout,
				BufferSize:     DefaultBufferSize,
				Framing:        FramingRaw,
				MaxMessageSize: DefaultMaxMessageSize,
				IdleTimeout:    DefaultIdleTimeout,
			},
		},
		// --- frame 프레이밍 테스트 ---
		{
			name: "frame 프레이밍 기본 설정",
			opts: map[string]any{
				"port":    "/dev/ttyUSB0",
				"framing": "frame",
				"stx":     "02",
			},
			want: SerialConfig{
				Port:           "/dev/ttyUSB0",
				BaudRate:       DefaultBaudRate,
				DataBits:       DefaultDataBits,
				StopBits:       DefaultStopBits,
				Parity:         DefaultParity,
				ReadTimeout:    DefaultReadTimeout,
				BufferSize:     DefaultBufferSize,
				Framing:        FramingFrame,
				MaxMessageSize: DefaultMaxMessageSize,
				IdleTimeout:    DefaultIdleTimeout,
				STX:            []byte{0x02},
				LengthOffset:   1,
				LengthSize:     1,
				LengthEndian:   "big",
				Checksum:       "none",
			},
		},
		{
			name: "frame 프레이밍 모든 필드 설정",
			opts: map[string]any{
				"port":                   "/dev/ttyUSB0",
				"framing":                "frame",
				"stx":                    "AA55",
				"etx":                    "03",
				"length_offset":          float64(3),
				"length_size":            float64(2),
				"length_endian":          "little",
				"length_includes_header": true,
				"length_adjustment":      float64(-1),
				"checksum":               "sum8",
			},
			want: SerialConfig{
				Port:                 "/dev/ttyUSB0",
				BaudRate:             DefaultBaudRate,
				DataBits:             DefaultDataBits,
				StopBits:             DefaultStopBits,
				Parity:               DefaultParity,
				ReadTimeout:          DefaultReadTimeout,
				BufferSize:           DefaultBufferSize,
				Framing:              FramingFrame,
				MaxMessageSize:       DefaultMaxMessageSize,
				IdleTimeout:          DefaultIdleTimeout,
				STX:                  []byte{0xAA, 0x55},
				ETX:                  []byte{0x03},
				LengthOffset:         3,
				LengthSize:           2,
				LengthEndian:         "little",
				LengthIncludesHeader: true,
				LengthAdjustment:     -1,
				Checksum:             "sum8",
			},
		},
		{
			name: "frame 프레이밍 STX 미지정 오류",
			opts: map[string]any{
				"port":    "/dev/ttyUSB0",
				"framing": "frame",
			},
			wantErr: ErrInvalidSTX,
		},
		{
			name: "frame 프레이밍 잘못된 STX hex 오류",
			opts: map[string]any{
				"port":    "/dev/ttyUSB0",
				"framing": "frame",
				"stx":     "ZZ",
			},
			wantErr: ErrInvalidSTX,
		},
		{
			name: "frame 프레이밍 빈 STX 오류",
			opts: map[string]any{
				"port":    "/dev/ttyUSB0",
				"framing": "frame",
				"stx":     "",
			},
			wantErr: ErrInvalidSTX,
		},
		{
			name: "frame 프레이밍 잘못된 length_size 오류",
			opts: map[string]any{
				"port":        "/dev/ttyUSB0",
				"framing":     "frame",
				"stx":         "02",
				"length_size": float64(3),
			},
			wantErr: ErrInvalidLengthSize,
		},
		{
			name: "frame 프레이밍 잘못된 length_endian 오류",
			opts: map[string]any{
				"port":          "/dev/ttyUSB0",
				"framing":       "frame",
				"stx":           "02",
				"length_endian": "middle",
			},
			wantErr: ErrInvalidEndian,
		},
		{
			name: "frame 프레이밍 잘못된 checksum 오류",
			opts: map[string]any{
				"port":     "/dev/ttyUSB0",
				"framing":  "frame",
				"stx":      "02",
				"checksum": "crc16",
			},
			wantErr: ErrInvalidChecksum,
		},
		{
			name: "frame 프레이밍 xor 체크섬",
			opts: map[string]any{
				"port":     "/dev/ttyUSB0",
				"framing":  "frame",
				"stx":      "02",
				"checksum": "xor",
			},
			want: SerialConfig{
				Port:           "/dev/ttyUSB0",
				BaudRate:       DefaultBaudRate,
				DataBits:       DefaultDataBits,
				StopBits:       DefaultStopBits,
				Parity:         DefaultParity,
				ReadTimeout:    DefaultReadTimeout,
				BufferSize:     DefaultBufferSize,
				Framing:        FramingFrame,
				MaxMessageSize: DefaultMaxMessageSize,
				IdleTimeout:    DefaultIdleTimeout,
				STX:            []byte{0x02},
				LengthOffset:   1,
				LengthSize:     1,
				LengthEndian:   "big",
				Checksum:       "xor",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// read_timeout 파싱 오류 특수 처리
			if tt.name == "잘못된 read_timeout 오류" {
				_, err := ParseSerialConfig(tt.opts)
				if err == nil {
					t.Fatal("오류를 기대했으나 nil 반환")
				}
				return
			}

			got, err := ParseSerialConfig(tt.opts)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("오류를 기대했으나 nil 반환")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("기대한 오류: %v, 실제 오류: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("예상치 못한 오류: %v", err)
			}
			assertSerialConfig(t, tt.want, got)
		})
	}
}

// --- 헬퍼 함수 ---

func assertSerialConfig(t *testing.T, want, got SerialConfig) {
	t.Helper()
	if got.Port != want.Port {
		t.Errorf("Port: 기대값 %q, 실제값 %q", want.Port, got.Port)
	}
	if got.BaudRate != want.BaudRate {
		t.Errorf("BaudRate: 기대값 %d, 실제값 %d", want.BaudRate, got.BaudRate)
	}
	if got.DataBits != want.DataBits {
		t.Errorf("DataBits: 기대값 %d, 실제값 %d", want.DataBits, got.DataBits)
	}
	if got.StopBits != want.StopBits {
		t.Errorf("StopBits: 기대값 %d, 실제값 %d", want.StopBits, got.StopBits)
	}
	if got.Parity != want.Parity {
		t.Errorf("Parity: 기대값 %q, 실제값 %q", want.Parity, got.Parity)
	}
	if got.ReadTimeout != want.ReadTimeout {
		t.Errorf("ReadTimeout: 기대값 %v, 실제값 %v", want.ReadTimeout, got.ReadTimeout)
	}
	if got.BufferSize != want.BufferSize {
		t.Errorf("BufferSize: 기대값 %d, 실제값 %d", want.BufferSize, got.BufferSize)
	}
	if got.Framing != want.Framing {
		t.Errorf("Framing: 기대값 %q, 실제값 %q", want.Framing, got.Framing)
	}
	if got.Delimiter != want.Delimiter {
		t.Errorf("Delimiter: 기대값 %d, 실제값 %d", want.Delimiter, got.Delimiter)
	}
	if got.FixedSize != want.FixedSize {
		t.Errorf("FixedSize: 기대값 %d, 실제값 %d", want.FixedSize, got.FixedSize)
	}
	if got.MaxMessageSize != want.MaxMessageSize {
		t.Errorf("MaxMessageSize: 기대값 %d, 실제값 %d", want.MaxMessageSize, got.MaxMessageSize)
	}
	if got.IdleTimeout != want.IdleTimeout {
		t.Errorf("IdleTimeout: 기대값 %v, 실제값 %v", want.IdleTimeout, got.IdleTimeout)
	}
	if string(got.STX) != string(want.STX) {
		t.Errorf("STX: 기대값 %x, 실제값 %x", want.STX, got.STX)
	}
	if string(got.ETX) != string(want.ETX) {
		t.Errorf("ETX: 기대값 %x, 실제값 %x", want.ETX, got.ETX)
	}
	if got.LengthOffset != want.LengthOffset {
		t.Errorf("LengthOffset: 기대값 %d, 실제값 %d", want.LengthOffset, got.LengthOffset)
	}
	if got.LengthSize != want.LengthSize {
		t.Errorf("LengthSize: 기대값 %d, 실제값 %d", want.LengthSize, got.LengthSize)
	}
	if got.LengthEndian != want.LengthEndian {
		t.Errorf("LengthEndian: 기대값 %q, 실제값 %q", want.LengthEndian, got.LengthEndian)
	}
	if got.LengthIncludesHeader != want.LengthIncludesHeader {
		t.Errorf("LengthIncludesHeader: 기대값 %v, 실제값 %v", want.LengthIncludesHeader, got.LengthIncludesHeader)
	}
	if got.Checksum != want.Checksum {
		t.Errorf("Checksum: 기대값 %q, 실제값 %q", want.Checksum, got.Checksum)
	}
}

// TestParseSerialConfig_LogMessages 는 log_messages 옵션의 기본값(false)과 파싱을 검증한다.
func TestParseSerialConfig_LogMessages(t *testing.T) {
	def, err := ParseSerialConfig(map[string]any{"port": "/dev/ttyUSB0"})
	if err != nil {
		t.Fatalf("parse(default) error: %v", err)
	}
	if def.LogMessages {
		t.Errorf("LogMessages = true, want false (default)")
	}

	on, err := ParseSerialConfig(map[string]any{"port": "/dev/ttyUSB0", "log_messages": true})
	if err != nil {
		t.Fatalf("parse(log_messages=true) error: %v", err)
	}
	if !on.LogMessages {
		t.Errorf("LogMessages = false, want true")
	}
}
