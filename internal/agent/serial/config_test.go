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
}
