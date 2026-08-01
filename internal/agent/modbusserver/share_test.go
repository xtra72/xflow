package modbusserver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// 공유 레지스터 맵 (role main/sub) 테스트 (M3)
// ---------------------------------------------------------------------------

// serverConfig 는 지정한 ID/role/shared_from 으로 modbus-server AgentConfig 를 만든다.
func serverConfig(id, role, sharedFrom string) agent.AgentConfig {
	opts := map[string]any{
		"listen_address": "127.0.0.1",
		"listen_port":    0,
		"unit_id":        1,
		"register_map": map[string]any{
			"holding_registers": map[string]any{
				"start_address": 0,
				"count":         10,
			},
		},
	}
	if role != "" {
		opts["role"] = role
	}
	if sharedFrom != "" {
		opts["shared_from"] = sharedFrom
	}
	return agent.AgentConfig{
		ID:   id,
		Name: id,
		Type: "modbus-server",
		Transport: agent.TransportConfig{
			Type:    "modbus-server",
			Options: opts,
		},
	}
}

// serverConfigMultiDevice 는 devices 배열(각 unit_id 마다 holding_registers[0..10))로
// 구성된 main 서버 설정을 만든다.
func serverConfigMultiDevice(id string, unitIDs []byte) agent.AgentConfig {
	devices := make([]any, 0, len(unitIDs))
	for _, uid := range unitIDs {
		devices = append(devices, map[string]any{
			"unit_id": int(uid),
			"register_map": map[string]any{
				"holding_registers": map[string]any{
					"start_address": 0,
					"count":         10,
				},
			},
		})
	}
	return agent.AgentConfig{
		ID:   id,
		Name: id,
		Type: "modbus-server",
		Transport: agent.TransportConfig{
			Type: "modbus-server",
			Options: map[string]any{
				"listen_address": "127.0.0.1",
				"listen_port":    0,
				"devices":        devices,
			},
		},
	}
}

// serverConfigSubNoDevices 는 자체 register_map/devices 를 정의하지 않는 role=sub 설정이다.
func serverConfigSubNoDevices(id, sharedFrom string) agent.AgentConfig {
	return agent.AgentConfig{
		ID:   id,
		Name: id,
		Type: "modbus-server",
		Transport: agent.TransportConfig{
			Type: "modbus-server",
			Options: map[string]any{
				"listen_address": "127.0.0.1",
				"listen_port":    0,
				"role":           "sub",
				"shared_from":    sharedFrom,
			},
		},
	}
}

// TestSharedRegisterMap_SubAdoptsAllDevices 는 자체 디바이스가 없는 role=sub 서버가
// Start 시 주 서버의 모든 디바이스(멀티 unit_id)를 라이브 공유로 상속하고, 모든
// unit_id 에 대해 쓰기가 main↔sub 로 즉시 전파되는지 검증한다.
func TestSharedRegisterMap_SubAdoptsAllDevices(t *testing.T) {
	mgr := agent.NewManager()
	require.NoError(t, RegisterModbusServerTypes(mgr))

	units := []byte{1, 2, 3}
	mainAgent, err := mgr.Create(serverConfigMultiDevice("main-multi", units))
	require.NoError(t, err)
	mainSrv := mainAgent.(*ModbusServerAgent)

	subAgent, err := mgr.Create(serverConfigSubNoDevices("sub-adopt", "main-multi"))
	require.NoError(t, err)
	subSrv := subAgent.(*ModbusServerAgent)

	// Start 이전: 서브는 디바이스가 없다.
	require.Equal(t, 0, subSrv.deviceManager.DeviceCount())

	require.NoError(t, subSrv.Start(context.Background()))
	t.Cleanup(func() { _ = subSrv.Stop(context.Background()) })

	// 서브가 주 서버의 모든 디바이스를 상속했다.
	require.Equal(t, len(units), subSrv.deviceManager.DeviceCount())

	// 모든 unit_id 에 대해 동일 포인터 공유 + 양방향 쓰기 전파 검증.
	for i, uid := range units {
		mainDev := mainSrv.deviceManager.GetDevice(uid)
		subDev := subSrv.deviceManager.GetDevice(uid)
		require.NotNil(t, mainDev, "main unit %d", uid)
		require.NotNil(t, subDev, "sub unit %d", uid)
		require.Same(t, mainDev.RegisterMap, subDev.RegisterMap, "unit %d 동일 포인터", uid)

		// main → sub 전파
		wantMain := uint16(0x1000 + i)
		_, err := mainDev.RegisterMap.WriteHoldingRegisters(0, []uint16{wantMain})
		require.NoError(t, err)
		got, err := subDev.RegisterMap.ReadHoldingRegisters(0, 1)
		require.NoError(t, err)
		assert.Equal(t, []uint16{wantMain}, got, "unit %d main→sub", uid)

		// sub → main 전파
		wantSub := uint16(0x2000 + i)
		_, err = subDev.RegisterMap.WriteHoldingRegisters(1, []uint16{wantSub})
		require.NoError(t, err)
		got, err = mainDev.RegisterMap.ReadHoldingRegisters(1, 1)
		require.NoError(t, err)
		assert.Equal(t, []uint16{wantSub}, got, "unit %d sub→main", uid)
	}
}

// TestSharedRegisterMap_SubZeroDevices_MainNotFound 는 자체 디바이스 없는 서브가
// 존재하지 않는 main 을 가리킬 때 Start 가 ErrSharedMainNotFound 를 반환하는지 검증한다.
func TestSharedRegisterMap_SubZeroDevices_MainNotFound(t *testing.T) {
	mgr := agent.NewManager()
	require.NoError(t, RegisterModbusServerTypes(mgr))

	subAgent, err := mgr.Create(serverConfigSubNoDevices("sub-orphan2", "nope"))
	require.NoError(t, err)
	subSrv := subAgent.(*ModbusServerAgent)

	err = subSrv.Start(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSharedMainNotFound)
}

// TestSharedRegisterMap_MainNoDevices 는 상속 대상(여기서는 아직 Start 하지 않아
// 디바이스가 비어 있는 서브)이 디바이스를 갖지 않을 때 ErrSharedMainNoDevices 를
// 반환하는지 검증한다.
func TestSharedRegisterMap_MainNoDevices(t *testing.T) {
	mgr := agent.NewManager()
	require.NoError(t, RegisterModbusServerTypes(mgr))

	// subA: 자체 디바이스 없음, Start 하지 않음 → 디바이스 0개 상태 유지.
	mainAgent, err := mgr.Create(serverConfigMultiDevice("chain-main", []byte{1}))
	require.NoError(t, err)
	_ = mainAgent
	subA, err := mgr.Create(serverConfigSubNoDevices("chain-subA", "chain-main"))
	require.NoError(t, err)
	subASrv := subA.(*ModbusServerAgent)
	require.Equal(t, 0, subASrv.deviceManager.DeviceCount())

	// subB 는 아직 채워지지 않은 subA 를 상속 대상으로 지정 → 디바이스 없음 오류.
	subB, err := mgr.Create(serverConfigSubNoDevices("chain-subB", "chain-subA"))
	require.NoError(t, err)
	subBSrv := subB.(*ModbusServerAgent)

	err = subBSrv.Start(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSharedMainNoDevices)
}

// TestSharedRegisterMap_SubReferencesMain 는 role=sub 서버가 Start 시 주 서버의
// RegisterMap 포인터를 라이브 공유하는지 검증한다(동일 포인터 + 쓰기 즉시 반영).
func TestSharedRegisterMap_SubReferencesMain(t *testing.T) {
	mgr := agent.NewManager()
	require.NoError(t, RegisterModbusServerTypes(mgr))

	mainAgent, err := mgr.Create(serverConfig("main-srv", "main", ""))
	require.NoError(t, err)
	mainSrv := mainAgent.(*ModbusServerAgent)

	subAgent, err := mgr.Create(serverConfig("sub-srv", "sub", "main-srv"))
	require.NoError(t, err)
	subSrv := subAgent.(*ModbusServerAgent)

	// Start 이전: 서브는 자체 맵을 가지므로 포인터가 다르다.
	require.NotSame(t, mainSrv.SharedRegisterMap(), subSrv.SharedRegisterMap())

	// 서브 Start → 주 서버의 RegisterMap 으로 스왑된다.
	require.NoError(t, subSrv.Start(context.Background()))
	t.Cleanup(func() { _ = subSrv.Stop(context.Background()) })

	// Start 이후: 동일 포인터를 공유한다.
	require.Same(t, mainSrv.SharedRegisterMap(), subSrv.SharedRegisterMap())

	// 주 서버에 쓴 값이 서브에 즉시 반영된다(라이브 공유).
	_, err = mainSrv.SharedRegisterMap().WriteHoldingRegisters(3, []uint16{0x4242})
	require.NoError(t, err)

	vals, err := subSrv.SharedRegisterMap().ReadHoldingRegisters(3, 1)
	require.NoError(t, err)
	assert.Equal(t, []uint16{0x4242}, vals)
}

// TestSharedRegisterMap_MainNotFound 는 shared_from 이 없는 에이전트를 가리킬 때
// Start 가 ErrSharedMainNotFound 를 반환하는지 검증한다.
func TestSharedRegisterMap_MainNotFound(t *testing.T) {
	mgr := agent.NewManager()
	require.NoError(t, RegisterModbusServerTypes(mgr))

	subAgent, err := mgr.Create(serverConfig("sub-orphan", "sub", "does-not-exist"))
	require.NoError(t, err)
	subSrv := subAgent.(*ModbusServerAgent)

	err = subSrv.Start(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSharedMainNotFound)
}

// TestSharedRegisterMap_ConfigValidation 는 role=sub 인데 shared_from 이 없으면
// 설정 파싱이 거부하는지 검증한다.
func TestSharedRegisterMap_ConfigValidation(t *testing.T) {
	_, err := parseModbusServerConfig(map[string]any{
		"role": "sub",
		"register_map": map[string]any{
			"holding_registers": map[string]any{"start_address": 0, "count": 10},
		},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidSharedConfig)
}
