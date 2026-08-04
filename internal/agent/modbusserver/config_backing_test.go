package modbusserver

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// M1 — 백킹 설정 스키마 + 검증 (REQ-MODBUS-010-01, AC-01/AC-02)
// ---------------------------------------------------------------------------

// minimalRegisterMap 는 백킹 테스트용 최소 register_map 설정을 반환한다.
func minimalRegisterMap() map[string]any {
	return map[string]any{
		"holding_registers": map[string]any{"address": 0, "count": 10},
	}
}

// AC-01: backing 키가 없는 device 는 Backing == nil (순수 slave, 하위 호환).
func TestParseBacking_AbsentBacking_YieldsNilPureSlave(t *testing.T) {
	raw := []any{
		map[string]any{
			"unit_id":      1,
			"register_map": minimalRegisterMap(),
		},
	}

	devices, err := parseDevicesConfig(raw)
	require.NoError(t, err)
	require.Len(t, devices, 1)
	assert.Nil(t, devices[0].Backing, "backing 키가 없으면 Backing 은 nil(순수 slave)이어야 한다")
}

// AC-02: 유효한 direct(tcp) 백킹은 파싱되어 필드가 채워진다.
func TestParseBacking_ValidDirectTCP(t *testing.T) {
	raw := []any{
		map[string]any{
			"unit_id":      1,
			"register_map": minimalRegisterMap(),
			"backing": map[string]any{
				"transport": "tcp",
				"host":      "192.168.0.10",
				"port":      1502,
				"unit_id":   3,
				"mode":      "direct",
				"timeout":   "750ms",
			},
		},
	}

	devices, err := parseDevicesConfig(raw)
	require.NoError(t, err)
	require.NotNil(t, devices[0].Backing)

	bc := devices[0].Backing
	assert.Equal(t, TransportTCP, bc.Transport)
	assert.Equal(t, "192.168.0.10", bc.Host)
	assert.Equal(t, 1502, bc.Port)
	assert.Equal(t, byte(3), bc.UnitID)
	assert.Equal(t, BackingModeDirect, bc.Mode)
	assert.Equal(t, 750*time.Millisecond, bc.Timeout)
}

// AC-02: 유효한 indirect(tcp) 백킹은 poll_interval/timeout 을 파싱한다.
func TestParseBacking_ValidIndirectTCP(t *testing.T) {
	raw := []any{
		map[string]any{
			"unit_id":      1,
			"register_map": minimalRegisterMap(),
			"backing": map[string]any{
				"host":          "10.0.0.5",
				"port":          502,
				"mode":          "indirect",
				"poll_interval": "1s",
				"timeout":       "5s",
			},
		},
	}

	devices, err := parseDevicesConfig(raw)
	require.NoError(t, err)
	bc := devices[0].Backing
	require.NotNil(t, bc)
	assert.Equal(t, TransportTCP, bc.Transport) // 기본값
	assert.Equal(t, byte(1), bc.UnitID)         // 기본값
	assert.Equal(t, BackingModeIndirect, bc.Mode)
	assert.Equal(t, time.Second, bc.PollInterval)
	assert.Equal(t, 5*time.Second, bc.Timeout)
}

// AC-02: 유효한 direct(rtu) 백킹은 시리얼 파라미터를 파싱한다.
func TestParseBacking_ValidDirectRTU(t *testing.T) {
	raw := []any{
		map[string]any{
			"unit_id":      1,
			"register_map": minimalRegisterMap(),
			"backing": map[string]any{
				"transport":   "rtu",
				"serial_port": "/dev/ttyUSB0",
				"baud_rate":   19200,
				"mode":        "direct",
			},
		},
	}

	devices, err := parseDevicesConfig(raw)
	require.NoError(t, err)
	bc := devices[0].Backing
	require.NotNil(t, bc)
	assert.Equal(t, TransportRTU, bc.Transport)
	assert.Equal(t, "/dev/ttyUSB0", bc.Serial.Port)
	assert.Equal(t, 19200, bc.Serial.BaudRate)
}

// AC-02: 부적합한 백킹 설정은 부분 적용 없이 오류로 거부된다.
func TestParseBacking_InvalidConfigs_Rejected(t *testing.T) {
	tests := []struct {
		name    string
		backing map[string]any
		wantErr error // errors.Is 로 검사할 sentinel (nil 이면 non-nil 만 확인)
	}{
		{
			name:    "mode 누락",
			backing: map[string]any{"host": "h", "port": 502},
			wantErr: ErrInvalidBackingConfig,
		},
		{
			name:    "mode 값 오류",
			backing: map[string]any{"host": "h", "port": 502, "mode": "hybrid"},
			wantErr: ErrInvalidBackingConfig,
		},
		{
			name:    "transport 값 오류",
			backing: map[string]any{"transport": "udp", "mode": "direct"},
			wantErr: ErrInvalidBackingConfig,
		},
		{
			name:    "tcp host 누락",
			backing: map[string]any{"transport": "tcp", "port": 502, "mode": "direct"},
			wantErr: ErrInvalidBackingConfig,
		},
		{
			name:    "tcp port 누락",
			backing: map[string]any{"transport": "tcp", "host": "h", "mode": "direct"},
			wantErr: ErrInvalidBackingConfig,
		},
		{
			name:    "tcp port 범위 초과",
			backing: map[string]any{"host": "h", "port": 70000, "mode": "direct"},
			wantErr: ErrInvalidBackingConfig,
		},
		{
			name:    "indirect poll_interval 누락",
			backing: map[string]any{"host": "h", "port": 502, "mode": "indirect", "timeout": "2s"},
			wantErr: ErrInvalidBackingConfig,
		},
		{
			name:    "indirect timeout 누락",
			backing: map[string]any{"host": "h", "port": 502, "mode": "indirect", "poll_interval": "1s"},
			wantErr: ErrInvalidBackingConfig,
		},
		{
			name:    "indirect poll_interval 비양수",
			backing: map[string]any{"host": "h", "port": 502, "mode": "indirect", "poll_interval": "0s", "timeout": "2s"},
			wantErr: ErrInvalidBackingConfig,
		},
		{
			name:    "unit_id 범위 초과",
			backing: map[string]any{"host": "h", "port": 502, "mode": "direct", "unit_id": 300},
			wantErr: ErrInvalidBackingConfig,
		},
		{
			name:    "duration 형식 오류",
			backing: map[string]any{"host": "h", "port": 502, "mode": "direct", "timeout": "abc"},
			wantErr: ErrInvalidBackingConfig,
		},
		{
			name:    "rtu serial_port 누락",
			backing: map[string]any{"transport": "rtu", "mode": "direct"},
			wantErr: ErrMissingSerialPort,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := []any{
				map[string]any{
					"unit_id":      1,
					"register_map": minimalRegisterMap(),
					"backing":      tt.backing,
				},
			}
			_, err := parseDevicesConfig(raw)
			require.Error(t, err, "부적합 백킹 설정은 오류여야 한다")
			if tt.wantErr != nil {
				assert.True(t, errors.Is(err, tt.wantErr), "err=%v 는 %v 를 감싸야 한다", err, tt.wantErr)
			}
		})
	}
}
