package modbus

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// M2 — F2 디바이스별 트랜스포트 override 파싱·검증 (REQ-MODBUS-008-02, AC-03/AC-04)
// M3 — F3 세션 공유 플래그 파싱 (REQ-MODBUS-008-03, AC-05)
// ---------------------------------------------------------------------------

// makeTCPDevice 는 테스트용 TCP 디바이스 맵을 만든다.
func makeTCPDevice(id, host string) map[string]any {
	return map[string]any{
		"id":      id,
		"host":    host,
		"port":    502,
		"unit_id": 1,
		"register_groups": []any{
			map[string]any{"name": "g", "function_code": 3, "start_address": 0, "quantity": 4},
		},
	}
}

// TestParseDevice_NoOverride_InheritsAgentDefault 는 per-device transport 미지정 시
// dc.Transport 가 빈 값(상속)으로 남아 에이전트 기본을 상속함을 검증한다(AC-03).
func TestParseDevice_NoOverride_InheritsAgentDefault(t *testing.T) {
	opts := map[string]any{
		"transport": "tcp",
		"devices":   []any{makeTCPDevice("d1", "10.0.0.1")},
	}
	cfg, err := parseModbusConfig(opts)
	require.NoError(t, err)
	require.Len(t, cfg.Devices, 1)
	assert.Equal(t, "", cfg.Devices[0].Transport, "override 미지정 시 Transport 는 빈 값(상속)이어야 한다")
	assert.Equal(t, TransportTCP, effectiveTransportKind(cfg.Devices[0], cfg))
}

// TestParseDevice_RTUOverride_WithSerial 는 per-device rtu override + 시리얼 파라미터가
// 올바로 파싱됨을 검증한다(AC-04). 에이전트 기본은 tcp 이다.
func TestParseDevice_RTUOverride_WithSerial(t *testing.T) {
	d2 := map[string]any{
		"id":          "d2",
		"unit_id":     2,
		"transport":   "rtu",
		"serial_port": "/dev/ttyUSB0",
		"baud_rate":   19200,
		"register_groups": []any{
			map[string]any{"name": "g", "function_code": 4, "start_address": 0, "quantity": 2},
		},
	}
	opts := map[string]any{
		"transport": "tcp",
		"devices":   []any{makeTCPDevice("d1", "10.0.0.1"), d2},
	}
	cfg, err := parseModbusConfig(opts)
	require.NoError(t, err)
	require.Len(t, cfg.Devices, 2)

	// D1 은 상속(tcp), D2 는 per-device rtu override.
	assert.Equal(t, "", cfg.Devices[0].Transport)
	assert.Equal(t, TransportRTU, cfg.Devices[1].Transport)
	assert.Equal(t, "/dev/ttyUSB0", cfg.Devices[1].Serial.Port)
	assert.Equal(t, 19200, cfg.Devices[1].Serial.BaudRate)
}

// TestParseDevice_RTUOverride_MissingSerialRejected 는 per-device rtu override 에 시리얼
// 파라미터(serial_port)가 누락되면 설정 오류로 거부됨을 검증한다(AC-04).
func TestParseDevice_RTUOverride_MissingSerialRejected(t *testing.T) {
	d2 := map[string]any{
		"id":        "d2",
		"unit_id":   2,
		"transport": "rtu", // serial_port 없음 → 거부
		"register_groups": []any{
			map[string]any{"name": "g", "function_code": 4, "start_address": 0, "quantity": 2},
		},
	}
	opts := map[string]any{
		"transport": "tcp",
		"devices":   []any{d2},
	}
	_, err := parseModbusConfig(opts)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrMissingSerialPort), "rtu override 는 serial_port 필수여야 한다")
}

// TestParseDevice_TCPOverride_MissingHostRejected 는 유효 트랜스포트가 tcp 인데 host 가
// 누락되면 설정 오류로 거부됨을 검증한다(AC-04). 에이전트 기본은 rtu 이고 D 는 tcp 로 override.
func TestParseDevice_TCPOverride_MissingHostRejected(t *testing.T) {
	d := map[string]any{
		"id":        "d",
		"unit_id":   1,
		"transport": "tcp", // host 없음 → 거부
		"register_groups": []any{
			map[string]any{"name": "g", "function_code": 3, "start_address": 0, "quantity": 4},
		},
	}
	opts := map[string]any{
		"transport":   "rtu",
		"serial_port": "/dev/ttyUSB0",
		"devices":     []any{d},
	}
	_, err := parseModbusConfig(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "host is required")
}

// TestParseDevice_InvalidTransportRejected 는 per-device transport 가 tcp/rtu 가 아니면
// ErrInvalidTransport 로 거부됨을 검증한다.
func TestParseDevice_InvalidTransportRejected(t *testing.T) {
	d := map[string]any{
		"id":        "d",
		"host":      "10.0.0.1",
		"transport": "udp", // 무효
		"register_groups": []any{
			map[string]any{"name": "g", "function_code": 3, "start_address": 0, "quantity": 4},
		},
	}
	opts := map[string]any{
		"transport": "tcp",
		"devices":   []any{d},
	}
	_, err := parseModbusConfig(opts)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidTransport))
}

// TestParseDevice_RTUInherit_NoSerialRequired 는 에이전트 기본이 rtu 이고 디바이스가 override
// 하지 않으면 per-device 시리얼 없이도 파싱됨을 검증한다(상속 rtu 는 에이전트 Serial 사용).
func TestParseDevice_RTUInherit_NoSerialRequired(t *testing.T) {
	d := map[string]any{
		"id":      "d",
		"unit_id": 1,
		"register_groups": []any{
			map[string]any{"name": "g", "function_code": 3, "start_address": 0, "quantity": 4},
		},
	}
	opts := map[string]any{
		"transport":   "rtu",
		"serial_port": "/dev/ttyUSB0",
		"devices":     []any{d},
	}
	cfg, err := parseModbusConfig(opts)
	require.NoError(t, err)
	require.Len(t, cfg.Devices, 1)
	assert.Equal(t, "", cfg.Devices[0].Transport, "상속 디바이스의 Transport 는 빈 값이어야 한다")
	assert.Equal(t, "", cfg.Devices[0].Serial.Port, "상속 rtu 디바이스는 per-device Serial 을 채우지 않는다")
	assert.Equal(t, TransportRTU, effectiveTransportKind(cfg.Devices[0], cfg))
}

// TestParseConfig_ShareSession_AgentLevel 은 에이전트 레벨 share_session 파싱을 검증한다(F3).
func TestParseConfig_ShareSession_AgentLevel(t *testing.T) {
	// 부재 시 기본 false (하위 호환).
	optsNo := map[string]any{"devices": []any{makeTCPDevice("d1", "10.0.0.1")}}
	cfgNo, err := parseModbusConfig(optsNo)
	require.NoError(t, err)
	assert.False(t, cfgNo.ShareSession, "share_session 부재 시 기본 false 여야 한다")

	// true 로 설정.
	optsYes := map[string]any{"share_session": true, "devices": []any{makeTCPDevice("d1", "10.0.0.1")}}
	cfgYes, err := parseModbusConfig(optsYes)
	require.NoError(t, err)
	assert.True(t, cfgYes.ShareSession)
}

// TestParseConfig_ShareSession_PerDeviceOverride 는 per-device share_session 오버라이드가
// 포인터로 파싱되어 에이전트 기본을 오버라이드함을 검증한다(F3).
func TestParseConfig_ShareSession_PerDeviceOverride(t *testing.T) {
	d1 := makeTCPDevice("d1", "10.0.0.1")
	d1["share_session"] = true
	d2 := makeTCPDevice("d2", "10.0.0.2") // 오버라이드 없음 → 상속
	opts := map[string]any{
		"share_session": false, // 에이전트 기본 false
		"devices":       []any{d1, d2},
	}
	cfg, err := parseModbusConfig(opts)
	require.NoError(t, err)
	require.Len(t, cfg.Devices, 2)

	require.NotNil(t, cfg.Devices[0].ShareSession)
	assert.True(t, *cfg.Devices[0].ShareSession)
	assert.True(t, effectiveShareSession(cfg.Devices[0], cfg), "override true 는 에이전트 false 를 이겨야 한다")

	assert.Nil(t, cfg.Devices[1].ShareSession, "오버라이드 미지정 디바이스는 nil(상속)이어야 한다")
	assert.False(t, effectiveShareSession(cfg.Devices[1], cfg), "상속 디바이스는 에이전트 기본(false)을 따른다")
}
