package lg

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// LGCNP 설정 파싱 테스트
// ---------------------------------------------------------------------------

func TestParseLGCNPConfig_Defaults(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"serial_port": "/dev/ttyUSB0",
	}

	cfg, err := parseLGCNPConfig(opts)
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

func TestParseLGCNPConfig_CustomBaudRate(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"serial_port": "/dev/ttyUSB0",
		"baud_rate":   2400,
	}

	cfg, err := parseLGCNPConfig(opts)
	require.NoError(t, err)
	assert.Equal(t, 2400, cfg.BaudRate)
}

func TestParseLGCNPConfig_MissingSerialPort(t *testing.T) {
	t.Parallel()

	opts := map[string]any{}

	_, err := parseLGCNPConfig(opts)
	assert.ErrorIs(t, err, ErrLGCNPSerialPortRequired)
}

func TestParseLGCNPConfig_InvalidBaudRate(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"serial_port": "/dev/ttyUSB0",
		"baud_rate":   -1,
	}

	_, err := parseLGCNPConfig(opts)
	assert.ErrorIs(t, err, ErrLGCNPInvalidBaudRate)
}

func TestParseLGCNPConfig_InvalidTransportType(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"serial_port":    "/dev/ttyUSB0",
		"transport_type": "invalid",
	}

	_, err := parseLGCNPConfig(opts)
	assert.ErrorIs(t, err, ErrLGCNPUnknownTransportType)
}

func TestParseLGCNPConfig_TCPClientMissingPort(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"transport_type": "tcp-client",
		"tcp_host":       "192.168.1.100",
	}

	_, err := parseLGCNPConfig(opts)
	assert.ErrorIs(t, err, ErrLGCNPTCPPortRequired)
}

func TestParseLGCNPConfig_VerifyRedundancyFalse(t *testing.T) {
	t.Parallel()

	opts := map[string]any{
		"serial_port":       "/dev/ttyUSB0",
		"verify_redundancy": false,
	}

	cfg, err := parseLGCNPConfig(opts)
	require.NoError(t, err)
	assert.False(t, cfg.VerifyRedundancy)
}

// ---------------------------------------------------------------------------
// LGCNP 에이전트 생성 테스트
// ---------------------------------------------------------------------------

func TestNewLGCNPAgent_MissingSerialPort(t *testing.T) {
	t.Parallel()

	config := agent.AgentConfig{
		ID:   "test-lgcnp",
		Name: "test-lgcnp",
		Type: "lgcnp",
		Transport: agent.TransportConfig{
			Type:    "serial",
			Options: map[string]any{},
		},
	}

	_, err := NewLGCNPAgent(config)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// LGCNP Process 명령 테스트
// ---------------------------------------------------------------------------

func TestLGCNPAgent_ProcessGetStats(t *testing.T) {
	t.Parallel()

	config := agent.AgentConfig{
		ID:   "test-lgcnp",
		Name: "test-lgcnp",
		Type: "lgcnp",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"serial_port": "/dev/null",
			},
		},
	}

	a, err := NewLGCNPAgent(config)
	require.NoError(t, err)

	lgcnpAgent := a.(*LGCNPAgent)

	cmdBytes, _ := json.Marshal(map[string]any{
		"command": "get_stats",
	})

	resp, err := lgcnpAgent.Process(cmdBytes)
	require.NoError(t, err)

	var result map[string]any
	err = json.Unmarshal(resp, &result)
	require.NoError(t, err)

	assert.Contains(t, result, "odu_frames_captured")
	assert.Contains(t, result, "idu_frames_captured")
	assert.Contains(t, result, "frames_dropped")
	assert.Contains(t, result, "transport_connected")
}

func TestLGCNPAgent_ProcessUnsupportedCommand(t *testing.T) {
	t.Parallel()

	config := agent.AgentConfig{
		ID:   "test-lgcnp",
		Name: "test-lgcnp",
		Type: "lgcnp",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"serial_port": "/dev/null",
			},
		},
	}

	a, err := NewLGCNPAgent(config)
	require.NoError(t, err)

	lgcnpAgent := a.(*LGCNPAgent)

	cmdBytes, _ := json.Marshal(map[string]any{
		"command": "set_power",
	})

	_, err = lgcnpAgent.Process(cmdBytes)
	assert.Error(t, err, "미지원 명령은 에러를 반환해야 함")
}

// ---------------------------------------------------------------------------
// LGCNP 에이전트 속성 테스트
// ---------------------------------------------------------------------------

func TestLGCNPAgent_Type(t *testing.T) {
	t.Parallel()

	config := agent.AgentConfig{
		ID:   "test-lgcnp",
		Name: "test-lgcnp",
		Type: "lgcnp",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"serial_port": "/dev/null",
			},
		},
	}

	a, err := NewLGCNPAgent(config)
	require.NoError(t, err)
	assert.Equal(t, "lgcnp", a.Type())
}

func TestLGCNPAgent_IDAndName(t *testing.T) {
	t.Parallel()

	config := agent.AgentConfig{
		ID:   "my-lgcnp-id",
		Name: "my-lgcnp-name",
		Type: "lgcnp",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"serial_port": "/dev/null",
			},
		},
	}

	a, err := NewLGCNPAgent(config)
	require.NoError(t, err)
	assert.Equal(t, "my-lgcnp-id", a.ID())
	assert.Equal(t, "my-lgcnp-name", a.Name())
}

func TestLGCNPAgent_ListDevices_Default(t *testing.T) {
	t.Parallel()

	config := agent.AgentConfig{
		ID:   "test-lgcnp",
		Name: "test-lgcnp",
		Type: "lgcnp",
		Transport: agent.TransportConfig{
			Type: "serial",
			Options: map[string]any{
				"serial_port": "/dev/null",
			},
		},
	}

	a, err := NewLGCNPAgent(config)
	require.NoError(t, err)

	lgcnpAgent := a.(*LGCNPAgent)
	devices := lgcnpAgent.ListDevices()

	// 기본적으로 ODU 디바이스 1개가 반환됨
	require.GreaterOrEqual(t, len(devices), 1)
	assert.Equal(t, "odu", devices[0].Address)
	assert.Equal(t, "HVACR.ODU", devices[0].Type)
}
