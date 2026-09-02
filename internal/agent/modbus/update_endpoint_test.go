package modbus

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// update_device 엔드포인트(host/port) 변경
// ---------------------------------------------------------------------------

// endpointAgentConfig 는 TCP 디바이스 1개를 가진 에이전트 설정이다.
func endpointAgentConfig(transport string) agent.AgentConfig {
	dev := map[string]any{
		"id": "d1", "host": "10.0.0.1", "port": 502, "unit_id": 1,
		"register_groups": []any{
			map[string]any{"name": "g", "function_code": 3, "start_address": 0, "quantity": 2},
		},
	}
	opts := map[string]any{
		"read_mode":        "cached",
		"mode":             "interval",
		"poll_interval":    "10s",
		"request_timeout":  "200ms",
		"msg_channel_size": 64,
		"devices":          []any{dev},
	}
	if transport == TransportRTU {
		opts["transport"] = TransportRTU
		opts["serial_port"] = "/dev/null"
		delete(dev, "host")
		delete(dev, "port")
	}
	return agent.AgentConfig{
		ID: "modbus-endpoint", Name: "Endpoint Agent", Type: "modbus-client",
		Transport: agent.TransportConfig{Type: "modbus-tcp", Options: opts},
	}
}

// deviceByID 는 현재 로스터에서 디바이스를 찾는다.
func deviceByID(a *ModbusAgent, id string) *ModbusDevice {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.findDeviceLocked(id)
}

// TestUpdateDevice_ChangesHostAndPort 는 host/port 변경이 실제로 반영되는지 검증한다.
func TestUpdateDevice_ChangesHostAndPort(t *testing.T) {
	a, _ := newTestModbusAgent(t, endpointAgentConfig(TransportTCP), &mockModbusTransport{connected: true})

	before := deviceByID(a, "d1")
	require.NotNil(t, before)
	oldTransport := before.transport

	raw, err := a.Process(updateDeviceJSON(t, "d1", map[string]any{"host": "192.168.0.50", "port": 5020}))
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(raw, &resp))
	assert.Equal(t, true, resp["endpoint_changed"])

	after := deviceByID(a, "d1")
	require.NotNil(t, after, "device id 는 유지되어야 한다")
	assert.Equal(t, "192.168.0.50", after.config.Host)
	assert.Equal(t, 5020, after.config.Port)
	assert.NotSame(t, oldTransport, after.transport, "엔드포인트가 바뀌면 트랜스포트를 다시 만든다")

	// 레지스터 그룹과 unit_id 는 승계되어야 한다.
	require.Len(t, after.config.RegisterGroups, 1)
	assert.Equal(t, "g", after.config.RegisterGroups[0].Name)
	assert.Equal(t, byte(1), after.UnitID())
}

// TestUpdateDevice_HostOnlyAndPortOnly 는 한쪽만 바꿔도 나머지가 보존되는지 검증한다.
func TestUpdateDevice_HostOnlyAndPortOnly(t *testing.T) {
	a, _ := newTestModbusAgent(t, endpointAgentConfig(TransportTCP), &mockModbusTransport{connected: true})

	_, err := a.Process(updateDeviceJSON(t, "d1", map[string]any{"host": "10.9.9.9"}))
	require.NoError(t, err)
	got := deviceByID(a, "d1")
	assert.Equal(t, "10.9.9.9", got.config.Host)
	assert.Equal(t, 502, got.config.Port, "port 를 안 보내면 기존 값이 유지되어야 한다")

	_, err = a.Process(updateDeviceJSON(t, "d1", map[string]any{"port": 1502}))
	require.NoError(t, err)
	got = deviceByID(a, "d1")
	assert.Equal(t, "10.9.9.9", got.config.Host, "host 를 안 보내면 기존 값이 유지되어야 한다")
	assert.Equal(t, 1502, got.config.Port)
}

// TestUpdateDevice_SameEndpointKeepsConnection 는 값이 같으면 연결을 건드리지 않음을 검증한다.
func TestUpdateDevice_SameEndpointKeepsConnection(t *testing.T) {
	a, _ := newTestModbusAgent(t, endpointAgentConfig(TransportTCP), &mockModbusTransport{connected: true})
	before := deviceByID(a, "d1").transport

	raw, err := a.Process(updateDeviceJSON(t, "d1", map[string]any{"host": "10.0.0.1", "port": 502}))
	require.NoError(t, err)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(raw, &resp))
	_, hasFlag := resp["endpoint_changed"]
	assert.False(t, hasFlag, "값이 같으면 교체하지 않는다")
	assert.Same(t, before, deviceByID(a, "d1").transport, "연결이 유지되어야 한다")
}

// TestUpdateDevice_EndpointWithGroups 는 엔드포인트와 register_groups 를 한 번에 바꿔도
// 둘 다 반영되는지 검증한다(부분 적용 없음).
func TestUpdateDevice_EndpointWithGroups(t *testing.T) {
	a, _ := newTestModbusAgent(t, endpointAgentConfig(TransportTCP), &mockModbusTransport{connected: true})
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	_, err := a.Process(updateDeviceJSON(t, "d1", map[string]any{"host": "172.16.0.5", "port": 5502,
		"register_groups": []any{
			map[string]any{"name": "new", "function_code": 4, "start_address": 100, "quantity": 4},
		},
	}))
	require.NoError(t, err)

	got := deviceByID(a, "d1")
	assert.Equal(t, "172.16.0.5", got.config.Host)
	assert.Equal(t, 5502, got.config.Port)
	require.Len(t, got.config.RegisterGroups, 1)
	assert.Equal(t, "new", got.config.RegisterGroups[0].Name)
	assert.Equal(t, byte(4), got.config.RegisterGroups[0].FunctionCode)
}

// TestUpdateDevice_EndpointPersisted 는 엔드포인트 변경이 영속 형상에 반영됨을 검증한다
// (재시작 후에도 새 주소로 접속해야 한다).
func TestUpdateDevice_EndpointPersisted(t *testing.T) {
	a, _ := newTestModbusAgent(t, endpointAgentConfig(TransportTCP), &mockModbusTransport{connected: true})

	_, err := a.Process(updateDeviceJSON(t, "d1", map[string]any{"host": "192.168.5.5", "port": 8502}))
	require.NoError(t, err)

	restored := reparse(t, a.GetPersistableDeviceConfigs())
	require.Len(t, restored.Devices, 1)
	assert.Equal(t, "192.168.5.5", restored.Devices[0].Host)
	assert.Equal(t, 8502, restored.Devices[0].Port)
}

// TestUpdateDevice_EndpointValidation 는 무효 입력을 거부하는지 검증한다(부분 적용 없음).
func TestUpdateDevice_EndpointValidation(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
	}{
		{name: "빈 host", params: map[string]any{"host": ""}},
		{name: "비문자열 host", params: map[string]any{"host": 123}},
		{name: "port 0", params: map[string]any{"port": 0}},
		{name: "port 초과", params: map[string]any{"port": 70000}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _ := newTestModbusAgent(t, endpointAgentConfig(TransportTCP), &mockModbusTransport{connected: true})
			_, err := a.Process(updateDeviceJSON(t, "d1", tt.params))
			require.Error(t, err)
			// 직전 상태 유지 — 아무것도 바뀌지 않아야 한다.
			got := deviceByID(a, "d1")
			assert.Equal(t, "10.0.0.1", got.config.Host)
			assert.Equal(t, 502, got.config.Port)
		})
	}
}

// TestUpdateDevice_EndpointRejectedForRTU 는 RTU 디바이스의 host/port 변경을 거부함을 검증한다.
func TestUpdateDevice_EndpointRejectedForRTU(t *testing.T) {
	a, _ := newTestModbusAgent(t, endpointAgentConfig(TransportRTU), &mockModbusTransport{connected: true})

	_, err := a.Process(updateDeviceJSON(t, "d1", map[string]any{"host": "10.0.0.9"}))
	require.Error(t, err, "RTU 는 host/port 개념이 없으므로 거부해야 한다")
	assert.Contains(t, err.Error(), "TCP")
}
