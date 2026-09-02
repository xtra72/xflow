package modbusserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// zero-device 서버 구성 테스트 (device 는 생성 후 config 업데이트로 추가)
// ---------------------------------------------------------------------------

// zeroDeviceMainConfig 는 devices/register_map 없는 서버 설정이다(빈 게이트웨이).
func zeroDeviceMainConfig(id string) agent.AgentConfig {
	return agent.AgentConfig{
		ID:   id,
		Name: id,
		Type: "modbus-gateway",
		Transport: agent.TransportConfig{
			Type: "modbus-gateway",
			Options: map[string]any{
				"listen_address": "127.0.0.1",
				"listen_port":    0,
				// devices/register_map 없음 → zero-device 서버
			},
		},
	}
}

// TestZeroDeviceMain_Parse 는 zero-device main 설정이 오류 없이 파싱되는지 검증한다.
func TestZeroDeviceMain_Parse(t *testing.T) {
	cfg, err := parseModbusServerConfig(map[string]any{
		"listen_address": "127.0.0.1",
		"listen_port":    0,
	})
	require.NoError(t, err, "devices/register_map 없는 서버는 오류 아님")
	assert.Empty(t, cfg.Devices, "zero-device 서버는 빈 Devices")
}

// TestZeroDeviceMain_ConstructStartProcess 는 zero-device main 이 빈 DeviceManager 로
// 구성되고 Start/Process(list_devices,get_status)/Stop 이 panic 없이 동작함을 검증한다.
func TestZeroDeviceMain_ConstructStartProcess(t *testing.T) {
	a, err := NewModbusServerAgent(zeroDeviceMainConfig("zero-main"))
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)

	// 빈 서빙 집합 + 컨테이너 없음 + primary RegisterMap nil.
	assert.Equal(t, 0, msa.deviceManager.DeviceCount())
	assert.Nil(t, msa.deviceManager.SharedContainer())
	assert.Nil(t, msa.SharedRegisterMap())

	// Start: 빈 서빙 집합으로 TCP 리스너 기동(panic 없음).
	require.NoError(t, msa.Start(context.Background()))
	t.Cleanup(func() { _ = msa.Stop(context.Background()) })

	// list_devices: 빈 목록.
	out, err := msa.Process([]byte(`{"command":"list_devices"}`))
	require.NoError(t, err)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(out, &listResp))
	assert.Equal(t, float64(0), listResp["device_count"])

	// get_status: panic 없이 응답.
	_, err = msa.Process([]byte(`{"command":"get_status"}`))
	require.NoError(t, err)
}

// TestZeroDeviceMain_AddDeviceThenServe 는 zero-device main 에 런타임 add_device 로
// 디바이스를 추가한 뒤 그 unit_id 로 읽기/쓰기가 동작함을 검증한다(생성 후 추가 경로).
func TestZeroDeviceMain_AddDeviceThenServe(t *testing.T) {
	a, err := NewModbusServerAgent(zeroDeviceMainConfig("zero-main-add"))
	require.NoError(t, err)
	msa := a.(*ModbusServerAgent)
	require.NoError(t, msa.Start(context.Background()))
	t.Cleanup(func() { _ = msa.Stop(context.Background()) })

	addCmd := map[string]any{
		"command": "add_device",
		"params": map[string]any{
			"unit_id": 1,
			"name":    "dev1",
			"register_map": map[string]any{
				"holding_registers": map[string]any{"address": 0, "count": 10},
			},
		},
	}
	addBytes, _ := json.Marshal(addCmd)
	_, err = msa.Process(addBytes)
	require.NoError(t, err, "빈 DeviceManager 에 add_device 성공")
	assert.Equal(t, 1, msa.deviceManager.DeviceCount())

	// 추가된 unit_id 로 set_register → get_holding_registers 왕복.
	setBytes, _ := json.Marshal(map[string]any{
		"command": "set_register",
		"params":  map[string]any{"unit_id": 1, "address": 3, "value": 0x1234},
	})
	_, err = msa.Process(setBytes)
	require.NoError(t, err)

	getBytes, _ := json.Marshal(map[string]any{
		"command": "get_holding_registers",
		"params":  map[string]any{"unit_id": 1, "address": 3, "quantity": 1},
	})
	out, err := msa.Process(getBytes)
	require.NoError(t, err)
	var getResp map[string]any
	require.NoError(t, json.Unmarshal(out, &getResp))
	vals, ok := getResp["values"].([]any)
	require.True(t, ok)
	require.Len(t, vals, 1)
	assert.Equal(t, float64(0x1234), vals[0])
}
