package lg

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// HVACR-01 설정 파싱 테스트
// ---------------------------------------------------------------------------

func TestParseHvacr01Config_Defaults(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"serial_port": "/dev/ttyUSB0",
	}

	cfg, err := parseHvacr01Config(opts)
	require.NoError(t, err)

	assert.Equal(t, "/dev/ttyUSB0", cfg.SerialPort)
	assert.Equal(t, 1200, cfg.BaudRate, "기본 보레이트는 1200")
	assert.Equal(t, 8, cfg.DataBits)
	assert.Equal(t, 1, cfg.StopBits)
	assert.Equal(t, "none", cfg.Parity)
	assert.True(t, cfg.VerifyRedundancy, "기본값은 이중기록 검증 활성화")
	assert.True(t, cfg.AutoDiscovery)
	assert.False(t, cfg.ControlEnabled, "제어는 항상 비활성화")
	assert.Equal(t, "serial", cfg.TransportType)
}

func TestParseHvacr01Config_CustomBaudRate(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"serial_port": "/dev/ttyUSB0",
		"baud_rate":   2400,
	}

	cfg, err := parseHvacr01Config(opts)
	require.NoError(t, err)
	assert.Equal(t, 2400, cfg.BaudRate)
}

func TestParseHvacr01Config_MissingSerialPort(t *testing.T) {
	t.Parallel()

	opts := map[string]any{}

	_, err := parseHvacr01Config(opts)
	assert.ErrorIs(t, err, ErrHvacr01SerialPortRequired)
}

func TestParseHvacr01Config_InvalidBaudRate(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"serial_port": "/dev/ttyUSB0",
		"baud_rate":   -1,
	}

	_, err := parseHvacr01Config(opts)
	assert.ErrorIs(t, err, ErrHvacr01InvalidBaudRate)
}

func TestParseHvacr01Config_InvalidTransportType(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"serial_port":    "/dev/ttyUSB0",
		"transport_type": "invalid",
	}

	_, err := parseHvacr01Config(opts)
	assert.ErrorIs(t, err, ErrHvacr01UnknownTransportType)
}

func TestParseHvacr01Config_TCPClientMissingPort(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"transport_type": "tcp-client",
		"tcp_host":       "192.168.1.100",
	}

	_, err := parseHvacr01Config(opts)
	assert.ErrorIs(t, err, ErrHvacr01TCPPortRequired)
}

func TestParseHvacr01Config_VerifyRedundancyFalse(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"serial_port":       "/dev/ttyUSB0",
		"verify_redundancy": false,
	}

	cfg, err := parseHvacr01Config(opts)
	require.NoError(t, err)
	assert.False(t, cfg.VerifyRedundancy)
}

// ---------------------------------------------------------------------------
// HVACR-01 에이전트 생성 테스트
// ---------------------------------------------------------------------------

func TestNewHvacr01Agent_MissingSerialPort(t *testing.T) {
	t.Parallel()

	config := agent.AgentConfig{
		ID:   "test-lg_hvacr01",
		Name: "test-lg_hvacr01",
		Type: "lg_hvacr01",
		Transport: agent.TransportConfig{
			Type:    "serial",
			Options: map[string]any{},
		},
	}

	_, err := NewHvacr01Agent(config)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// HVACR-01 Process 명령 테스트
// ---------------------------------------------------------------------------

func TestHvacr01Agent_ProcessGetStats(t *testing.T) {
	t.Parallel()

	config := agent.AgentConfig{
		ID:   "test-lg_hvacr01",
		Name: "test-lg_hvacr01",
		Type: "lg_hvacr01",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"serial_port": "/dev/null",
			},
		},
	}

	a, err := NewHvacr01Agent(config)
	require.NoError(t, err)

	hvacr01Agent := a.(*Hvacr01Agent)

	cmdBytes, _ := json.Marshal(map[string]any{
		"command": "get_stats",
	})

	resp, err := hvacr01Agent.Process(cmdBytes)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(resp, &result)
	require.NoError(t, err)

	assert.Contains(t, result, "odu_frames_captured")
	assert.Contains(t, result, "idu_frames_captured")
	assert.Contains(t, result, "frames_dropped")
	assert.Contains(t, result, "transport_connected")
}

func TestHvacr01Agent_ProcessUnsupportedCommand(t *testing.T) {
	t.Parallel()

	config := agent.AgentConfig{
		ID:   "test-lg_hvacr01",
		Name: "test-lg_hvacr01",
		Type: "lg_hvacr01",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"serial_port": "/dev/null",
			},
		},
	}

	a, err := NewHvacr01Agent(config)
	require.NoError(t, err)

	hvacr01Agent := a.(*Hvacr01Agent)

	cmdBytes, _ := json.Marshal(map[string]any{
		"command": "set_power",
	})

	_, err = hvacr01Agent.Process(cmdBytes)
	assert.Error(t, err, "미지원 명령은 에러를 반환해야 함")
}

// ---------------------------------------------------------------------------
// HVACR-01 에이전트 속성 테스트
// ---------------------------------------------------------------------------

func TestHvacr01Agent_Type(t *testing.T) {
	t.Parallel()

	config := agent.AgentConfig{
		ID:   "test-lg_hvacr01",
		Name: "test-lg_hvacr01",
		Type: "lg_hvacr01",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"serial_port": "/dev/null",
			},
		},
	}

	a, err := NewHvacr01Agent(config)
	require.NoError(t, err)
	assert.Equal(t, "lg_hvacr01", a.Type())
}

func TestHvacr01Agent_IDAndName(t *testing.T) {
	t.Parallel()

	config := agent.AgentConfig{
		ID:   "my-lg_hvacr01-id",
		Name: "my-lg_hvacr01-name",
		Type: "lg_hvacr01",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"serial_port": "/dev/null",
			},
		},
	}

	a, err := NewHvacr01Agent(config)
	require.NoError(t, err)
	assert.Equal(t, "my-lg_hvacr01-id", a.ID())
	assert.Equal(t, "my-lg_hvacr01-name", a.Name())
}

func TestHvacr01Agent_ListDevices_Default(t *testing.T) {
	t.Parallel()

	config := agent.AgentConfig{
		ID:   "test-lg_hvacr01",
		Name: "test-lg_hvacr01",
		Type: "lg_hvacr01",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"serial_port": "/dev/null",
			},
		},
	}

	a, err := NewHvacr01Agent(config)
	require.NoError(t, err)

	hvacr01Agent := a.(*Hvacr01Agent)
	devices := hvacr01Agent.ListDevices()

	// 기본적으로 ODU 디바이스 1개가 반환됨
	require.GreaterOrEqual(t, len(devices), 1)
	assert.Equal(t, "odu", devices[0].Address)
	assert.Equal(t, "HVACR.ODU", devices[0].Type)
}
