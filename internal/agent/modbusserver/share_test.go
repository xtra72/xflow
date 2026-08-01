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
