package modbus

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// 런타임 디바이스 로스터 영속화 — 저장 형상 왕복 검증
// ---------------------------------------------------------------------------
//
// 핵심 불변식: GetPersistableDeviceConfigs() 의 출력을 그대로 devices 옵션에 넣고
// 다시 파싱하면 동일한 디바이스 집합이 나와야 한다. 이게 깨지면 재시작 시 런타임
// 등록 디바이스가 사라지거나 설정 일부(그룹·타입·사용여부)가 유실된다.

// persistBaseConfig 는 디바이스 1개로 시작하는 modbus-client 에이전트 설정이다.
func persistBaseConfig() agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "modbus-persist",
		Name: "Persist Agent",
		Type: "modbus-client",
		Transport: agent.TransportConfig{
			Type: "modbus-tcp",
			Options: map[string]any{
				"read_mode":        "cached",
				"mode":             "interval",
				"poll_interval":    "10s",
				"request_timeout":  "1s",
				"msg_channel_size": 64,
				"devices": []any{
					map[string]any{
						"id": "seed", "host": "10.0.0.1", "port": 502, "unit_id": 1,
						"register_groups": []any{
							map[string]any{"name": "g", "function_code": 3, "start_address": 0, "quantity": 2},
						},
					},
				},
			},
		},
	}
}

// reparse 는 영속 형상을 devices 옵션으로 되돌려 파싱한다(재시작 재현).
func reparse(t *testing.T, devices []any) ModbusConfig {
	t.Helper()
	cfg, err := parseModbusConfig(map[string]any{
		"read_mode": "cached",
		"devices":   devices,
	})
	require.NoError(t, err, "영속 형상은 parseDeviceConfig 가 그대로 소비할 수 있어야 한다")
	return cfg
}

// TestGetPersistableDeviceConfigs_RoundTrip 는 런타임 add_device 로 등록한 디바이스가
// 영속 형상에 포함되고, 그 형상을 다시 파싱하면 설정이 무손실 복원됨을 검증한다.
func TestGetPersistableDeviceConfigs_RoundTrip(t *testing.T) {
	a, _ := newTestModbusAgent(t, persistBaseConfig(), &mockModbusTransport{connected: true})
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	// 런타임 등록: 선택 필드를 최대한 채워 왕복 손실을 드러낸다.
	addReq, err := json.Marshal(map[string]any{
		"command": "add_device",
		"params": map[string]any{
			"id": "gipam-1", "host": "192.168.0.10", "port": 502, "unit_id": 7,
			"max_block_registers": 56,
			"register_groups": []any{
				map[string]any{
					"name": "계측", "function_code": 4, "start_address": 4, "quantity": 2,
					"data_type": "float32", "poll_interval": "2s",
				},
				map[string]any{
					"name": "설정", "function_code": 3, "start_address": 1002, "quantity": 2,
					"data_type": "uint32", "poll_interval": "300s", "enabled": false,
				},
			},
		},
	})
	require.NoError(t, err)
	_, err = a.Process(addReq)
	require.NoError(t, err)

	// 영속 형상 → 재파싱(재시작 재현).
	restored := reparse(t, a.GetPersistableDeviceConfigs())
	require.Len(t, restored.Devices, 2, "seed + 런타임 등록 디바이스가 모두 남아야 한다")

	byID := make(map[string]DeviceConfig, len(restored.Devices))
	for _, d := range restored.Devices {
		byID[d.ID] = d
	}

	seed, ok := byID["seed"]
	require.True(t, ok, "설정에 있던 디바이스가 유실되면 안 된다")
	assert.Equal(t, byte(1), seed.UnitID)

	got, ok := byID["gipam-1"]
	require.True(t, ok, "런타임 등록 디바이스가 영속 형상에 있어야 한다")
	assert.Equal(t, "192.168.0.10", got.Host)
	assert.Equal(t, 502, got.Port)
	assert.Equal(t, byte(7), got.UnitID)
	require.NotNil(t, got.MaxBlockRegisters, "기종별 블록 상한이 보존되어야 한다")
	assert.Equal(t, uint16(56), *got.MaxBlockRegisters)

	require.Len(t, got.RegisterGroups, 2)
	assert.Equal(t, "계측", got.RegisterGroups[0].Name)
	assert.Equal(t, "float32", got.RegisterGroups[0].DataType)
	assert.Equal(t, 2*time.Second, got.RegisterGroups[0].PollInterval)
	assert.True(t, got.RegisterGroups[0].IsEnabled())

	assert.Equal(t, "설정", got.RegisterGroups[1].Name)
	assert.Equal(t, uint16(1002), got.RegisterGroups[1].StartAddress)
	assert.Equal(t, 300*time.Second, got.RegisterGroups[1].PollInterval)
	assert.False(t, got.RegisterGroups[1].IsEnabled(), "미사용 상태가 재시작 후에도 보존되어야 한다")
}

// TestGetPersistableDeviceConfigs_RemoveDropsDevice 는 remove_device 후 영속 형상에서
// 해당 디바이스가 빠짐을 검증한다(재시작 시 되살아나면 안 된다).
func TestGetPersistableDeviceConfigs_RemoveDropsDevice(t *testing.T) {
	a, _ := newTestModbusAgent(t, persistBaseConfig(), &mockModbusTransport{connected: true})
	require.NoError(t, a.Start(context.Background()))
	defer func() { _ = a.Stop(context.Background()) }()

	rmReq, err := json.Marshal(map[string]any{"command": "remove_device", "device_id": "seed"})
	require.NoError(t, err)
	_, err = a.Process(rmReq)
	require.NoError(t, err)

	assert.Empty(t, a.GetPersistableDeviceConfigs(), "삭제한 디바이스는 영속 형상에서 빠져야 한다")
}

// TestGetPersistableDeviceConfigs_TypeMapPreserved 는 type_map(주소별 타입 오버라이드)이
// 왕복에서 유실되지 않음을 검증한다.
func TestGetPersistableDeviceConfigs_TypeMapPreserved(t *testing.T) {
	cfg := persistBaseConfig()
	devs := cfg.Transport.Options["devices"].([]any)
	devs[0].(map[string]any)["register_groups"] = []any{
		map[string]any{
			"name": "mixed", "function_code": 3, "start_address": 0, "quantity": 4,
			"type_map": []any{
				map[string]any{"address": 0, "data_type": "float32", "byte_order": "CDAB"},
				map[string]any{"address": 2, "data_type": "uint16"},
			},
		},
	}

	a, _ := newTestModbusAgent(t, cfg, &mockModbusTransport{connected: true})
	restored := reparse(t, a.GetPersistableDeviceConfigs())

	require.Len(t, restored.Devices, 1)
	tm := restored.Devices[0].RegisterGroups[0].TypeMap
	require.Len(t, tm, 2, "type_map 항목이 보존되어야 한다")
	assert.Equal(t, uint16(0), tm[0].Address)
	assert.Equal(t, "float32", tm[0].DataType)
	assert.Equal(t, "CDAB", tm[0].ByteOrder, "byte_order 오버라이드가 보존되어야 한다")
	assert.Equal(t, "uint16", tm[1].DataType)
}

// TestGetPersistableDeviceConfigs_SurvivesJSONRoundTrip 는 저장소를 거치는 실제 경로를
// 재현한다. repo.Save 는 설정을 직렬화하므로 숫자가 float64 로 돌아온다 — 좁은 타입
// (byte/uint16)을 그대로 방출하면 여기서 조용히 0 이 되어 복원이 깨진다.
func TestGetPersistableDeviceConfigs_SurvivesJSONRoundTrip(t *testing.T) {
	a, _ := newTestModbusAgent(t, persistBaseConfig(), &mockModbusTransport{connected: true})

	raw, err := json.Marshal(a.GetPersistableDeviceConfigs())
	require.NoError(t, err)
	var devices []any
	require.NoError(t, json.Unmarshal(raw, &devices))

	restored := reparse(t, devices)
	require.Len(t, restored.Devices, 1)
	d := restored.Devices[0]
	assert.Equal(t, "seed", d.ID)
	assert.Equal(t, byte(1), d.UnitID)
	assert.Equal(t, 502, d.Port)
	require.Len(t, d.RegisterGroups, 1)
	assert.Equal(t, byte(3), d.RegisterGroups[0].FunctionCode, "function_code 가 0 이면 직렬화 왕복이 깨진 것")
	assert.Equal(t, uint16(2), d.RegisterGroups[0].Quantity)
}
