package socket

import (
	"errors"
	"testing"
	"time"
)

// --- ParseTCPServerConfig 테스트 ---

func TestParseTCPServerConfig(t *testing.T) {
	tests := []struct {
		name    string
		opts    map[string]any
		want    TCPServerConfig
		wantErr error
	}{
		{
			name: "모든 필드 설정",
			opts: map[string]any{
				"host":             "192.168.1.1",
				"port":             float64(8080),
				"buffer_size":      float64(8192),
				"framing":          "newline",
				"delimiter":        float64('\t'),
				"max_message_size": float64(2048),
				"max_connections":  float64(100),
			},
			want: TCPServerConfig{
				TCPConfig: TCPConfig{
					SocketConfig: SocketConfig{
						Host:       "192.168.1.1",
						Port:       8080,
						BufferSize: 8192,
					},
					Framing:        FramingNewline,
					Delimiter:      '\t',
					MaxMessageSize: 2048,
				},
				MaxConnections: 100,
			},
		},
		{
			name: "기본값 적용",
			opts: map[string]any{
				"port": float64(9000),
			},
			want: TCPServerConfig{
				TCPConfig: TCPConfig{
					SocketConfig: SocketConfig{
						Host:       DefaultHost,
						Port:       9000,
						BufferSize: DefaultBufferSize,
					},
					Framing:        FramingRaw,
					MaxMessageSize: DefaultMaxMessageSize,
				},
				MaxConnections: 0, // 0 = 무제한
			},
		},
		{
			name:    "포트 미지정 오류",
			opts:    map[string]any{},
			wantErr: ErrPortRequired,
		},
		{
			name: "잘못된 프레이밍 타입 오류",
			opts: map[string]any{
				"port":    float64(9000),
				"framing": "invalid_type",
			},
			wantErr: ErrInvalidFraming,
		},
		{
			name: "fixed_size 프레이밍에서 크기 미지정 오류",
			opts: map[string]any{
				"port":    float64(9000),
				"framing": "fixed_size",
			},
			wantErr: ErrFixedSizeRequired,
		},
		{
			name: "fixed_size 프레이밍 정상 설정",
			opts: map[string]any{
				"port":       float64(9000),
				"framing":    "fixed_size",
				"fixed_size": float64(256),
			},
			want: TCPServerConfig{
				TCPConfig: TCPConfig{
					SocketConfig: SocketConfig{
						Host:       DefaultHost,
						Port:       9000,
						BufferSize: DefaultBufferSize,
					},
					Framing:        FramingFixedSize,
					FixedSize:      256,
					MaxMessageSize: DefaultMaxMessageSize,
				},
			},
		},
		{
			name: "정수 타입 포트도 처리",
			opts: map[string]any{
				"port": 9000,
			},
			want: TCPServerConfig{
				TCPConfig: TCPConfig{
					SocketConfig: SocketConfig{
						Host:       DefaultHost,
						Port:       9000,
						BufferSize: DefaultBufferSize,
					},
					Framing:        FramingRaw,
					MaxMessageSize: DefaultMaxMessageSize,
				},
			},
		},
		{
			name: "max_connections 0은 무제한으로 유효",
			opts: map[string]any{
				"port":            float64(9000),
				"max_connections": float64(0),
			},
			want: TCPServerConfig{
				TCPConfig: TCPConfig{
					SocketConfig: SocketConfig{
						Host:       DefaultHost,
						Port:       9000,
						BufferSize: DefaultBufferSize,
					},
					Framing:        FramingRaw,
					MaxMessageSize: DefaultMaxMessageSize,
				},
				MaxConnections: 0,
			},
		},
		{
			name: "length_prefix 프레이밍",
			opts: map[string]any{
				"port":    float64(9000),
				"framing": "length_prefix",
			},
			want: TCPServerConfig{
				TCPConfig: TCPConfig{
					SocketConfig: SocketConfig{
						Host:       DefaultHost,
						Port:       9000,
						BufferSize: DefaultBufferSize,
					},
					Framing:        FramingLengthPrefix,
					MaxMessageSize: DefaultMaxMessageSize,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTCPServerConfig(tt.opts)
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
			assertTCPServerConfig(t, tt.want, got)
		})
	}
}

// --- ParseTCPClientConfig 테스트 ---

func TestParseTCPClientConfig(t *testing.T) {
	tests := []struct {
		name    string
		opts    map[string]any
		want    TCPClientConfig
		wantErr error
	}{
		{
			name: "모든 필드 설정",
			opts: map[string]any{
				"host":               "10.0.0.1",
				"port":               float64(5000),
				"buffer_size":        float64(2048),
				"framing":            "length_prefix",
				"max_message_size":   float64(4096),
				"reconnect_interval": "3s",
				"max_retries":        float64(5),
				"connect_timeout":    "30s",
			},
			want: TCPClientConfig{
				TCPConfig: TCPConfig{
					SocketConfig: SocketConfig{
						Host:       "10.0.0.1",
						Port:       5000,
						BufferSize: 2048,
					},
					Framing:        FramingLengthPrefix,
					MaxMessageSize: 4096,
				},
				ReconnectInterval: 3 * time.Second,
				MaxRetries:        5,
				ConnectTimeout:    30 * time.Second,
			},
		},
		{
			name: "기본값 적용",
			opts: map[string]any{
				"port": float64(5000),
			},
			want: TCPClientConfig{
				TCPConfig: TCPConfig{
					SocketConfig: SocketConfig{
						Host:       DefaultClientHost,
						Port:       5000,
						BufferSize: DefaultBufferSize,
					},
					Framing:        FramingRaw,
					MaxMessageSize: DefaultMaxMessageSize,
				},
				ReconnectInterval: DefaultReconnectInterval,
				MaxRetries:        0, // 0 = 무한
				ConnectTimeout:    DefaultConnectTimeout,
			},
		},
		{
			name:    "포트 미지정 오류",
			opts:    map[string]any{},
			wantErr: ErrPortRequired,
		},
		{
			name: "max_retries 0은 무한으로 유효",
			opts: map[string]any{
				"port":        float64(5000),
				"max_retries": float64(0),
			},
			want: TCPClientConfig{
				TCPConfig: TCPConfig{
					SocketConfig: SocketConfig{
						Host:       DefaultClientHost,
						Port:       5000,
						BufferSize: DefaultBufferSize,
					},
					Framing:        FramingRaw,
					MaxMessageSize: DefaultMaxMessageSize,
				},
				ReconnectInterval: DefaultReconnectInterval,
				MaxRetries:        0,
				ConnectTimeout:    DefaultConnectTimeout,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTCPClientConfig(tt.opts)
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
			assertTCPClientConfig(t, tt.want, got)
		})
	}
}

// --- ParseUDPServerConfig 테스트 ---

func TestParseUDPServerConfig(t *testing.T) {
	tests := []struct {
		name    string
		opts    map[string]any
		want    UDPServerConfig
		wantErr error
	}{
		{
			name: "모든 필드 설정",
			opts: map[string]any{
				"host":        "0.0.0.0",
				"port":        float64(6000),
				"buffer_size": float64(8192),
			},
			want: UDPServerConfig{
				SocketConfig: SocketConfig{
					Host:       "0.0.0.0",
					Port:       6000,
					BufferSize: 8192,
				},
			},
		},
		{
			name: "기본값 적용",
			opts: map[string]any{
				"port": float64(6000),
			},
			want: UDPServerConfig{
				SocketConfig: SocketConfig{
					Host:       DefaultHost,
					Port:       6000,
					BufferSize: DefaultBufferSize,
				},
			},
		},
		{
			name:    "포트 미지정 오류",
			opts:    map[string]any{},
			wantErr: ErrPortRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseUDPServerConfig(tt.opts)
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
			if got != tt.want {
				t.Fatalf("기대값: %+v, 실제값: %+v", tt.want, got)
			}
		})
	}
}

// --- ParseUDPClientConfig 테스트 ---

func TestParseUDPClientConfig(t *testing.T) {
	tests := []struct {
		name    string
		opts    map[string]any
		want    UDPClientConfig
		wantErr error
	}{
		{
			name: "모든 필드 설정",
			opts: map[string]any{
				"host":        "10.0.0.1",
				"port":        float64(7000),
				"buffer_size": float64(2048),
			},
			want: UDPClientConfig{
				SocketConfig: SocketConfig{
					Host:       "10.0.0.1",
					Port:       7000,
					BufferSize: 2048,
				},
			},
		},
		{
			name: "기본값 적용",
			opts: map[string]any{
				"port": float64(7000),
			},
			want: UDPClientConfig{
				SocketConfig: SocketConfig{
					Host:       DefaultClientHost,
					Port:       7000,
					BufferSize: DefaultBufferSize,
				},
			},
		},
		{
			name:    "포트 미지정 오류",
			opts:    map[string]any{},
			wantErr: ErrPortRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseUDPClientConfig(tt.opts)
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
			if got != tt.want {
				t.Fatalf("기대값: %+v, 실제값: %+v", tt.want, got)
			}
		})
	}
}

// --- 헬퍼 함수 ---

func assertTCPServerConfig(t *testing.T, want, got TCPServerConfig) {
	t.Helper()
	assertTCPConfig(t, want.TCPConfig, got.TCPConfig)
	if got.MaxConnections != want.MaxConnections {
		t.Errorf("MaxConnections: 기대값 %d, 실제값 %d", want.MaxConnections, got.MaxConnections)
	}
}

func assertTCPClientConfig(t *testing.T, want, got TCPClientConfig) {
	t.Helper()
	assertTCPConfig(t, want.TCPConfig, got.TCPConfig)
	if got.ReconnectInterval != want.ReconnectInterval {
		t.Errorf("ReconnectInterval: 기대값 %v, 실제값 %v", want.ReconnectInterval, got.ReconnectInterval)
	}
	if got.MaxRetries != want.MaxRetries {
		t.Errorf("MaxRetries: 기대값 %d, 실제값 %d", want.MaxRetries, got.MaxRetries)
	}
	if got.ConnectTimeout != want.ConnectTimeout {
		t.Errorf("ConnectTimeout: 기대값 %v, 실제값 %v", want.ConnectTimeout, got.ConnectTimeout)
	}
}

func assertTCPConfig(t *testing.T, want, got TCPConfig) {
	t.Helper()
	if got.Host != want.Host {
		t.Errorf("Host: 기대값 %q, 실제값 %q", want.Host, got.Host)
	}
	if got.Port != want.Port {
		t.Errorf("Port: 기대값 %d, 실제값 %d", want.Port, got.Port)
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
