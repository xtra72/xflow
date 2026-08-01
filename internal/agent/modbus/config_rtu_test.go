package modbus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// 트랜스포트 선택 + RTU 시리얼 설정 파싱 테스트 (DDD/TDD, M4)
//
//   - AC-02: tcp vs rtu 라우팅이 올바른 트랜스포트 구현을 선택한다.
//   - AC-03: transport 생략 시 tcp 로 해석되어 기존 동작이 보존된다.
// ---------------------------------------------------------------------------

// TestParseModbusConfig_TransportDefaultsTCP 는 AC-03 을 검증한다:
// transport 키가 없으면 tcp 로 해석된다(하위 호환).
func TestParseModbusConfig_TransportDefaultsTCP(t *testing.T) {
	cfg, err := parseModbusConfig(minimalValidOpts())
	require.NoError(t, err)
	assert.Equal(t, TransportTCP, cfg.Transport, "transport 생략 시 기본값은 tcp 여야 한다")
	// 시리얼 설정은 파싱되지 않으므로 zero value 여야 한다.
	assert.Equal(t, SerialConfig{}, cfg.Serial)
}

// TestParseModbusConfig_TransportExplicitTCP 는 transport: tcp 명시를 검증한다.
func TestParseModbusConfig_TransportExplicitTCP(t *testing.T) {
	opts := minimalValidOpts()
	opts["transport"] = "tcp"
	cfg, err := parseModbusConfig(opts)
	require.NoError(t, err)
	assert.Equal(t, TransportTCP, cfg.Transport)
}

// TestParseModbusConfig_TransportRTU 는 transport: rtu 와 시리얼 파라미터 파싱을 검증한다(A-10).
func TestParseModbusConfig_TransportRTU(t *testing.T) {
	opts := map[string]any{
		"transport":   "rtu",
		"serial_port": "/dev/ttyUSB0",
		"baud_rate":   19200,
		"data_bits":   7,
		"stop_bits":   2,
		"parity":      "even",
		"devices": []any{
			map[string]any{
				"host":    "unused-for-rtu",
				"unit_id": 5,
				"register_groups": []any{
					map[string]any{"function_code": 3, "quantity": 4},
				},
			},
		},
	}
	cfg, err := parseModbusConfig(opts)
	require.NoError(t, err)

	assert.Equal(t, TransportRTU, cfg.Transport)
	assert.Equal(t, SerialConfig{
		Port:     "/dev/ttyUSB0",
		BaudRate: 19200,
		DataBits: 7,
		StopBits: 2,
		Parity:   "even",
	}, cfg.Serial)
}

// TestParseModbusConfig_RTUSerialDefaults 는 시리얼 파라미터 기본값(9600/8/1/none)을 검증한다.
func TestParseModbusConfig_RTUSerialDefaults(t *testing.T) {
	opts := map[string]any{
		"transport":   "rtu",
		"serial_port": "/dev/ttyS0",
		"devices": []any{
			map[string]any{
				"host":            "x",
				"register_groups": []any{map[string]any{"function_code": 3, "quantity": 2}},
			},
		},
	}
	cfg, err := parseModbusConfig(opts)
	require.NoError(t, err)
	assert.Equal(t, SerialConfig{Port: "/dev/ttyS0", BaudRate: 9600, DataBits: 8, StopBits: 1, Parity: "none"}, cfg.Serial)
}

// TestParseModbusConfig_TransportInvalid 는 알 수 없는 transport 를 오류로 거부하는지 검증한다.
func TestParseModbusConfig_TransportInvalid(t *testing.T) {
	opts := minimalValidOpts()
	opts["transport"] = "udp"
	_, err := parseModbusConfig(opts)
	require.ErrorIs(t, err, ErrInvalidTransport)
}

// TestParseModbusConfig_RTUMissingPort 는 rtu 인데 serial_port 가 없으면 오류인지 검증한다.
func TestParseModbusConfig_RTUMissingPort(t *testing.T) {
	opts := minimalValidOpts()
	opts["transport"] = "rtu"
	_, err := parseModbusConfig(opts)
	require.ErrorIs(t, err, ErrMissingSerialPort)
}

// TestParseModbusConfig_RTUInvalidSerialParams 는 시리얼 파라미터 검증을 테이블로 확인한다.
func TestParseModbusConfig_RTUInvalidSerialParams(t *testing.T) {
	base := func() map[string]any {
		return map[string]any{
			"transport":   "rtu",
			"serial_port": "/dev/ttyUSB0",
			"devices": []any{
				map[string]any{
					"host":            "x",
					"register_groups": []any{map[string]any{"function_code": 3, "quantity": 2}},
				},
			},
		}
	}
	tests := []struct {
		name  string
		key   string
		value any
	}{
		{name: "baud_zero", key: "baud_rate", value: 0},
		{name: "baud_negative", key: "baud_rate", value: -1},
		{name: "data_bits_low", key: "data_bits", value: 4},
		{name: "data_bits_high", key: "data_bits", value: 9},
		{name: "stop_bits_invalid", key: "stop_bits", value: 3},
		{name: "parity_invalid", key: "parity", value: "space"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := base()
			opts[tt.key] = tt.value
			_, err := parseModbusConfig(opts)
			require.ErrorIs(t, err, ErrInvalidSerialParam)
		})
	}
}

// TestParseDeviceConfig_RTUHostOptional 는 RTU 트랜스포트에서 host 없이
// unit_id 만으로 디바이스가 파싱되는지 검증한다(공유 시리얼 버스, A-4).
// host 는 빈 값으로 유지되고 unit_id 가 디바이스 키로 설정되어야 한다.
func TestParseDeviceConfig_RTUHostOptional(t *testing.T) {
	opts := map[string]any{
		"transport":   "rtu",
		"serial_port": "/dev/ttyUSB0",
		"devices": []any{
			map[string]any{
				"unit_id":         5,
				"register_groups": []any{map[string]any{"function_code": 3, "quantity": 4}},
			},
		},
	}
	cfg, err := parseModbusConfig(opts)
	require.NoError(t, err, "RTU 디바이스는 host 없이 unit_id 만으로 파싱되어야 한다")
	require.Len(t, cfg.Devices, 1)
	assert.Empty(t, cfg.Devices[0].Host, "RTU 디바이스의 host 는 빈 값으로 유지되어야 한다")
	assert.Equal(t, byte(5), cfg.Devices[0].UnitID, "unit_id 가 디바이스 키로 설정되어야 한다")
}

// TestParseDeviceConfig_TCPHostStillRequired 는 회귀 가드이다:
// TCP 트랜스포트(생략·명시 모두)에서 host 가 없으면 여전히 "host is required" 오류.
func TestParseDeviceConfig_TCPHostStillRequired(t *testing.T) {
	tests := []struct {
		name      string
		transport any // nil 이면 transport 키 생략(기본 tcp)
	}{
		{name: "transport_omitted", transport: nil},
		{name: "transport_explicit_tcp", transport: "tcp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := map[string]any{
				"devices": []any{
					map[string]any{
						"unit_id":         3,
						"register_groups": []any{map[string]any{"function_code": 3, "quantity": 4}},
					},
				},
			}
			if tt.transport != nil {
				opts["transport"] = tt.transport
			}
			_, err := parseModbusConfig(opts)
			require.Error(t, err, "TCP 디바이스는 host 가 없으면 오류여야 한다")
			assert.Contains(t, err.Error(), "host is required")
		})
	}
}

// ---------------------------------------------------------------------------
// 트랜스포트 라우팅 테스트 (AC-02)
// ---------------------------------------------------------------------------

// rtuAgentConfig 는 transport: rtu 인 테스트용 AgentConfig 를 반환한다.
func rtuAgentConfig() agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "modbus-rtu-1",
		Name: "Test Modbus RTU Agent",
		Type: "modbus-client", // type id 는 modbus-client 로 개명됨(pure rename)
		Transport: agent.TransportConfig{
			Type: "modbus-tcp",
			Options: map[string]any{
				"transport":        "rtu",
				"serial_port":      "/dev/ttyUSB0",
				"baud_rate":        9600,
				"read_mode":        "cached",
				"poll_interval":    "100ms",
				"request_timeout":  "1s",
				"msg_channel_size": 64,
				"devices": []any{
					map[string]any{
						"id":              "plc-1",
						"host":            "unused",
						"unit_id":         1,
						"register_groups": []any{map[string]any{"function_code": 3, "quantity": 10}},
					},
				},
			},
		},
	}
}

// TestTransportRouting_TCP 는 AC-02 를 검증한다: transport 생략(tcp)은 ModbusTCPTransport 로 라우팅.
func TestTransportRouting_TCP(t *testing.T) {
	a, err := NewModbusAgent(minimalAgentConfig())
	require.NoError(t, err)
	ma, ok := a.(*ModbusAgent)
	require.True(t, ok)
	require.Len(t, ma.devices, 1)

	_, isTCP := ma.devices[0].transport.(*ModbusTCPTransport)
	assert.True(t, isTCP, "tcp 설정은 ModbusTCPTransport 로 라우팅되어야 한다")
}

// TestTransportRouting_RTU 는 AC-02 를 검증한다: transport: rtu 는 ModbusRTUTransport 로 라우팅.
func TestTransportRouting_RTU(t *testing.T) {
	a, err := NewModbusAgent(rtuAgentConfig())
	require.NoError(t, err)
	ma, ok := a.(*ModbusAgent)
	require.True(t, ok)
	require.Len(t, ma.devices, 1)

	_, isRTU := ma.devices[0].transport.(*ModbusRTUTransport)
	assert.True(t, isRTU, "rtu 설정은 ModbusRTUTransport 로 라우팅되어야 한다")
}

// TestTransportRouting_RTUSharedBus 는 다중 디바이스가 하나의 시리얼 트랜스포트를
// 공유하는지 검증한다(반이중 멀티드롭, A-4).
func TestTransportRouting_RTUSharedBus(t *testing.T) {
	cfg := rtuAgentConfig()
	cfg.Transport.Options["devices"] = []any{
		map[string]any{
			"id": "plc-1", "host": "x", "unit_id": 1,
			"register_groups": []any{map[string]any{"function_code": 3, "quantity": 4}},
		},
		map[string]any{
			"id": "plc-2", "host": "x", "unit_id": 2,
			"register_groups": []any{map[string]any{"function_code": 3, "quantity": 4}},
		},
	}
	a, err := NewModbusAgent(cfg)
	require.NoError(t, err)
	ma := a.(*ModbusAgent)
	require.Len(t, ma.devices, 2)

	// 두 디바이스는 동일한 RTU 트랜스포트 인스턴스를 공유해야 한다.
	assert.Same(t, ma.devices[0].transport, ma.devices[1].transport,
		"RTU 멀티드롭은 단일 시리얼 버스를 공유해야 한다")
}

// TestTransportRouting_TypeIDPreserved 는 rtu 라우팅에서도 단일 등록 type id 가
// 유지되는지 검증한다(REQ-05): "modbus-client" 로 등록된 팩토리가 rtu 설정을 받아
// ModbusRTUTransport 로 라우팅하는 에이전트를 생성한다.
func TestTransportRouting_TypeIDPreserved(t *testing.T) {
	mgr := agent.NewManager()
	require.NoError(t, RegisterModbusTypes(mgr))

	a, err := mgr.Create(rtuAgentConfig())
	require.NoError(t, err, "type id modbus-client 로 rtu 에이전트를 생성할 수 있어야 한다")

	ma, ok := a.(*ModbusAgent)
	require.True(t, ok)
	_, isRTU := ma.devices[0].transport.(*ModbusRTUTransport)
	assert.True(t, isRTU, "modbus-client type id 를 통해 생성된 rtu 설정은 RTU 로 라우팅되어야 한다")
}
