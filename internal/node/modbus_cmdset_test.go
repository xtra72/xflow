package node

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// command-set 노드(modbus-write / modbus-read / modbus-control) 테스트
// ---------------------------------------------------------------------------
// 살베지된 mock(modbus_testhelpers_test.go)을 재사용한다. 실제 resolveModbusAgent는
// mock을 MODBUS 타입으로 감지하지 못하므로, Init 후 n.agent / n.agentType 을 직접 주입한다.

// setupWriteNode 는 Configure+Init 후 mock/agentType을 주입한 ModbusWriteNode를 만든다.
func setupWriteNode(t *testing.T, config map[string]any, agentType string, mock *mockModbusAgent) *ModbusWriteNode {
	t.Helper()
	node, err := NewModbusWriteNode(flow.NewNodeDef("test-mbw", "modbus-write"))
	require.NoError(t, err)
	n := node.(*ModbusWriteNode)
	require.NoError(t, n.Configure(config))
	require.NoError(t, n.Init(context.Background()))
	n.mu.Lock()
	n.agent = mock
	n.agentType = agentType
	n.mu.Unlock()
	return n
}

// setupReadNode 는 Configure+Init 후 mock/agentType을 주입한 ModbusReadNode를 만든다.
func setupReadNode(t *testing.T, config map[string]any, agentType string, mock *mockModbusAgent) *ModbusReadNode {
	t.Helper()
	node, err := NewModbusReadNode(flow.NewNodeDef("test-mbr", "modbus-read"))
	require.NoError(t, err)
	n := node.(*ModbusReadNode)
	require.NoError(t, n.Configure(config))
	require.NoError(t, n.Init(context.Background()))
	n.mu.Lock()
	n.agent = mock
	n.agentType = agentType
	n.mu.Unlock()
	return n
}

// setupControlNode 는 Configure+Init 후 mock/agentType을 주입한 ModbusControlNode를 만든다.
func setupControlNode(t *testing.T, config map[string]any, agentType string, mock *mockModbusAgent) *ModbusControlNode {
	t.Helper()
	node, err := NewModbusControlNode(flow.NewNodeDef("test-mbc", "modbus-control"))
	require.NoError(t, err)
	n := node.(*ModbusControlNode)
	require.NoError(t, n.Configure(config))
	require.NoError(t, n.Init(context.Background()))
	n.mu.Lock()
	n.agent = mock
	n.agentType = agentType
	n.mu.Unlock()
	return n
}

// resultEntries 는 결과 메시지 payload에서 results/values 배열을 map 슬라이스로 꺼낸다.
func resultEntries(t *testing.T, out message.Message, key string) []map[string]any {
	t.Helper()
	v, ok := out.Payload().Get(key)
	require.True(t, ok, "payload에 %q 키가 있어야 한다", key)
	arr, ok := v.([]map[string]any)
	require.True(t, ok, "%q는 []map[string]any 여야 한다", key)
	return arr
}

// ===========================================================================
// modbus-write
// ===========================================================================

// TestModbusWrite_Client_Single 은 Client 단일 코일/홀딩 쓰기를 검증한다.
func TestModbusWrite_Client_Single(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"status":"ok"}`)}
	cfg := map[string]any{
		"agent_ref": "mb-client",
		"command_set": []any{
			map[string]any{"area": "holding_registers", "address": float64(10), "value": float64(42)},
		},
	}
	n := setupWriteNode(t, cfg, agentTypeClient, mock)

	out, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	require.Len(t, out, 1)

	success, _ := out[0].Payload().Get("success")
	assert.Equal(t, true, success)
	assert.Equal(t, agentTypeClient, mustGet(t, out[0], "agent_type"))

	cmd := parseProcessCommand(t, mock.processData)
	assert.Equal(t, "write_register", cmd["command"])
	assert.Equal(t, "1", cmd["device_id"]) // unit_id 미지정 → 기본 device_id=1
}

// TestModbusWrite_Client_Single_Coil 은 Client 코일 쓰기가 write_coil을 사용함을 검증한다.
func TestModbusWrite_Client_Single_Coil(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"status":"ok"}`)}
	cfg := map[string]any{
		"agent_ref": "mb-client",
		"command_set": []any{
			map[string]any{"area": "coils", "address": float64(3), "value": true, "unit_id": float64(2)},
		},
	}
	n := setupWriteNode(t, cfg, agentTypeClient, mock)

	out, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	cmd := parseProcessCommand(t, mock.processData)
	assert.Equal(t, "write_coil", cmd["command"])
	assert.Equal(t, "2", cmd["device_id"])
	success, _ := out[0].Payload().Get("success")
	assert.Equal(t, true, success)
}

// TestModbusWrite_Server_Single 은 Server 단일 홀딩 레지스터 쓰기(set_register)를 검증한다.
func TestModbusWrite_Server_Single(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"ok":true}`)}
	cfg := map[string]any{
		"agent_ref": "mb-server",
		"command_set": []any{
			map[string]any{"area": "holding_registers", "address": float64(5), "value": float64(100)},
		},
	}
	n := setupWriteNode(t, cfg, agentTypeServer, mock)

	out, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	cmd := parseProcessCommand(t, mock.processData)
	assert.Equal(t, "set_register", cmd["command"])
	success, _ := out[0].Payload().Get("success")
	assert.Equal(t, true, success)
}

// TestModbusWrite_Server_Multi 는 Server 다중 코일 쓰기(set_coils)를 검증한다.
func TestModbusWrite_Server_Multi(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"ok":true}`)}
	cfg := map[string]any{
		"agent_ref": "mb-server",
		"command_set": []any{
			map[string]any{"area": "coils", "address": float64(0), "values": []any{true, false, true}},
		},
	}
	n := setupWriteNode(t, cfg, agentTypeServer, mock)

	_, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	cmd := parseProcessCommand(t, mock.processData)
	assert.Equal(t, "set_coils", cmd["command"])
	params := cmd["params"].(map[string]any)
	assert.Len(t, params["values"].([]any), 3)
}

// TestModbusWrite_Server_SharedUnitID0 은 unit_id:0(공유 컨테이너)이 params에 주입됨을 검증한다.
func TestModbusWrite_Server_SharedUnitID0(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"ok":true}`)}
	cfg := map[string]any{
		"agent_ref": "mb-server",
		"command_set": []any{
			map[string]any{"area": "holding_registers", "address": float64(1), "value": float64(7), "unit_id": float64(0)},
		},
	}
	n := setupWriteNode(t, cfg, agentTypeServer, mock)

	_, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	cmd := parseProcessCommand(t, mock.processData)
	params := cmd["params"].(map[string]any)
	assert.Equal(t, float64(0), params["unit_id"], "unit_id:0이 공유 컨테이너 대상으로 주입되어야 한다")
}

// TestModbusWrite_Server_DiscreteInput 은 Server discrete_inputs 쓰기가 set_input을 사용함을 검증한다.
func TestModbusWrite_Server_DiscreteInput(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"ok":true}`)}
	cfg := map[string]any{
		"agent_ref": "mb-server",
		"command_set": []any{
			map[string]any{"area": "discrete_inputs", "address": float64(2), "value": true},
		},
	}
	n := setupWriteNode(t, cfg, agentTypeServer, mock)

	_, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	cmd := parseProcessCommand(t, mock.processData)
	assert.Equal(t, "set_input", cmd["command"])
	params := cmd["params"].(map[string]any)
	assert.Equal(t, "discrete_inputs", params["area"])
}

// TestModbusWrite_Client_ReadOnlyArea_Fails 는 Client가 read-only 영역 쓰기 시 op 실패 사유를 남김을 검증한다.
func TestModbusWrite_Client_ReadOnlyArea_Fails(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"status":"ok"}`)}
	cfg := map[string]any{
		"agent_ref": "mb-client",
		"command_set": []any{
			map[string]any{"area": "input_registers", "address": float64(0), "value": float64(1)},
		},
	}
	n := setupWriteNode(t, cfg, agentTypeClient, mock)

	out, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	entries := resultEntries(t, out[0], "results")
	require.Len(t, entries, 1)
	assert.Equal(t, false, entries[0]["ok"])
	assert.Contains(t, entries[0]["reason"], "read-only")
	success, _ := out[0].Payload().Get("success")
	assert.Equal(t, false, success)
}

// TestModbusWrite_FailingOp_ProducesReason 은 실패 op가 reason을 남기고 모든 op를 실행함을 검증한다.
func TestModbusWrite_FailingOp_ProducesReason(t *testing.T) {
	// 에이전트가 illegal data address 오류를 반환(하드 오류 아님) → op별 reason 수집.
	mock := &mockModbusAgent{processErr: errors.New("illegal data address")}
	cfg := map[string]any{
		"agent_ref": "mb-client",
		"command_set": []any{
			map[string]any{"area": "holding_registers", "address": float64(1), "value": float64(1)},
			map[string]any{"area": "holding_registers", "address": float64(2), "value": float64(2)},
		},
	}
	n := setupWriteNode(t, cfg, agentTypeClient, mock)

	out, err := n.Process(context.Background(), message.New())
	require.NoError(t, err, "op별 오류는 하드 오류가 아니므로 error 포트로 라우팅하지 않는다")
	entries := resultEntries(t, out[0], "results")
	require.Len(t, entries, 2, "첫 실패에서 중단하지 않고 모든 op를 실행해야 한다")
	for _, e := range entries {
		assert.Equal(t, false, e["ok"])
		assert.Contains(t, e["reason"], "illegal data address")
	}
	success, _ := out[0].Payload().Get("success")
	assert.Equal(t, false, success)
}

// TestModbusWrite_ClientStatusError 는 Client status:"error" 응답을 실패로 판정함을 검증한다.
func TestModbusWrite_ClientStatusError(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"status":"error","error":"illegal address","exception_code":2}`)}
	cfg := map[string]any{
		"agent_ref": "mb-client",
		"command_set": []any{
			map[string]any{"area": "coils", "address": float64(0), "value": true},
		},
	}
	n := setupWriteNode(t, cfg, agentTypeClient, mock)

	out, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	entries := resultEntries(t, out[0], "results")
	assert.Equal(t, false, entries[0]["ok"])
	assert.Contains(t, entries[0]["reason"], "illegal address")
	assert.Contains(t, entries[0]["reason"], "exception_code")
}

// TestModbusWrite_PayloadCommandSetOverride 는 payload command_set이 config 기본값을 이김을 검증한다.
func TestModbusWrite_PayloadCommandSetOverride(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"ok":true}`)}
	cfg := map[string]any{
		"agent_ref": "mb-server",
		"command_set": []any{ // config 기본값: 홀딩 레지스터 쓰기
			map[string]any{"area": "holding_registers", "address": float64(0), "value": float64(1)},
		},
	}
	n := setupWriteNode(t, cfg, agentTypeServer, mock)

	msg := message.New()
	msg.Payload().Set("command_set", []any{ // payload 오버라이드: 코일 쓰기
		map[string]any{"area": "coils", "address": float64(9), "value": true},
	})
	_, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	cmd := parseProcessCommand(t, mock.processData)
	assert.Equal(t, "set_coil", cmd["command"], "payload command_set이 config를 오버라이드해야 한다")
}

// ===========================================================================
// modbus-read
// ===========================================================================

// TestModbusRead_Server_Raw 는 Server 기본(uint16) 읽기가 raw로 담김을 검증한다.
func TestModbusRead_Server_Raw(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"ok":true,"values":[10,20]}`)}
	cfg := map[string]any{
		"agent_ref": "mb-server",
		"command_set": []any{
			map[string]any{"area": "holding_registers", "address": float64(0), "count": float64(2)},
		},
	}
	n := setupReadNode(t, cfg, agentTypeServer, mock)

	out, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	cmd := parseProcessCommand(t, mock.processData)
	assert.Equal(t, "get_holding_registers", cmd["command"])

	entries := resultEntries(t, out[0], "values")
	require.Len(t, entries, 1)
	assert.Contains(t, entries[0], "raw")
	assert.NotContains(t, entries[0], "data_type")
	success, _ := out[0].Payload().Get("success")
	assert.Equal(t, true, success)
}

// TestModbusRead_Server_Typed 는 Server typed 읽기가 values+data_type로 담김을 검증한다.
func TestModbusRead_Server_Typed(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"ok":true,"values":[3.14]}`)}
	cfg := map[string]any{
		"agent_ref": "mb-server",
		"command_set": []any{
			map[string]any{"area": "holding_registers", "address": float64(0), "count": float64(2), "data_type": "float32"},
		},
	}
	n := setupReadNode(t, cfg, agentTypeServer, mock)

	out, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	cmd := parseProcessCommand(t, mock.processData)
	assert.Equal(t, "get_register_typed", cmd["command"])

	entries := resultEntries(t, out[0], "values")
	assert.Contains(t, entries[0], "values")
	assert.Equal(t, "float32", entries[0]["data_type"])
}

// TestModbusRead_Client_Raw 는 Client read_raw 읽기가 raw로 담김을 검증한다.
func TestModbusRead_Client_Raw(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"data":"AAEC"}`)}
	cfg := map[string]any{
		"agent_ref": "mb-client",
		"command_set": []any{
			map[string]any{"area": "holding_registers", "address": float64(0), "count": float64(2)},
		},
	}
	n := setupReadNode(t, cfg, agentTypeClient, mock)

	out, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	cmd := parseProcessCommand(t, mock.processData)
	assert.Equal(t, "read_raw", cmd["command"])

	entries := resultEntries(t, out[0], "values")
	assert.Equal(t, "AAEC", entries[0]["raw"])
}

// TestModbusRead_Server_UnitID0 은 읽기 op의 unit_id:0이 params에 주입됨을 검증한다.
func TestModbusRead_Server_UnitID0(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"ok":true,"values":[1]}`)}
	cfg := map[string]any{
		"agent_ref": "mb-server",
		"command_set": []any{
			map[string]any{"area": "holding_registers", "address": float64(0), "count": float64(1), "unit_id": float64(0)},
		},
	}
	n := setupReadNode(t, cfg, agentTypeServer, mock)

	_, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	cmd := parseProcessCommand(t, mock.processData)
	params := cmd["params"].(map[string]any)
	assert.Equal(t, float64(0), params["unit_id"])
}

// ===========================================================================
// modbus-control
// ===========================================================================

// TestModbusControl_Lifecycle 은 lifecycle 액션이 원본 agent 메서드를 직접 호출함을 검증한다.
func TestModbusControl_Lifecycle(t *testing.T) {
	mock := &mockModbusAgent{}
	cfg := map[string]any{
		"agent_ref": "mb-client",
		"command_set": []any{
			map[string]any{"action": "start"},
			map[string]any{"action": "pause"},
			map[string]any{"action": "resume"},
			map[string]any{"action": "stop"},
		},
	}
	n := setupControlNode(t, cfg, agentTypeClient, mock)

	out, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	assert.Equal(t, 1, mock.startCalls)
	assert.Equal(t, 1, mock.pauseCalls)
	assert.Equal(t, 1, mock.resumeCalls)
	assert.Equal(t, 1, mock.stopCalls)

	entries := resultEntries(t, out[0], "results")
	require.Len(t, entries, 4)
	assert.Equal(t, "start", entries[0]["action"])
	assert.Equal(t, true, entries[0]["ok"])
	success, _ := out[0].Payload().Get("success")
	assert.Equal(t, true, success)
}

// TestModbusControl_Reconnect 는 reconnect가 Stop→Start로 구현됨을 검증한다.
func TestModbusControl_Reconnect(t *testing.T) {
	mock := &mockModbusAgent{}
	cfg := map[string]any{
		"agent_ref":   "mb-client",
		"command_set": []any{map[string]any{"action": "reconnect"}},
	}
	n := setupControlNode(t, cfg, agentTypeClient, mock)

	_, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	assert.Equal(t, 1, mock.stopCalls)
	assert.Equal(t, 1, mock.startCalls)
}

// TestModbusControl_AddDevice_Server 는 서버 전용 add_device Process 명령 전달을 검증한다.
func TestModbusControl_AddDevice_Server(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"ok":true}`)}
	cfg := map[string]any{
		"agent_ref": "mb-server",
		"command_set": []any{
			map[string]any{"action": "add_device", "params": map[string]any{"unit_id": float64(5), "name": "dev5"}},
		},
	}
	n := setupControlNode(t, cfg, agentTypeServer, mock)

	out, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	cmd := parseProcessCommand(t, mock.processData)
	assert.Equal(t, "add_device", cmd["command"])
	assert.Equal(t, float64(5), cmd["unit_id"], "params가 최상위로 병합되어야 한다")

	entries := resultEntries(t, out[0], "results")
	assert.Equal(t, true, entries[0]["ok"])
}

// TestModbusControl_AddDevice_OnClient_Fails 는 client에 add_device 시 op 실패 사유를 남김을 검증한다.
func TestModbusControl_AddDevice_OnClient_Fails(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"ok":true}`)}
	cfg := map[string]any{
		"agent_ref": "mb-client",
		"command_set": []any{
			map[string]any{"action": "add_device", "params": map[string]any{"unit_id": float64(5)}},
		},
	}
	n := setupControlNode(t, cfg, agentTypeClient, mock)

	out, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	entries := resultEntries(t, out[0], "results")
	assert.Equal(t, false, entries[0]["ok"])
	assert.Contains(t, entries[0]["reason"], "server agent")
	assert.Nil(t, mock.processData, "타입 불일치이므로 agent.Process는 호출되지 않아야 한다")
}

// TestModbusControl_SetConfig_Client 는 client 전용 set_config Process 명령 전달을 검증한다.
func TestModbusControl_SetConfig_Client(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"status":"reconfigured"}`)}
	cfg := map[string]any{
		"agent_ref": "mb-client",
		"command_set": []any{
			map[string]any{"action": "set_config", "params": map[string]any{"device_id": "1", "params": map[string]any{"unit_id": float64(3)}}},
		},
	}
	n := setupControlNode(t, cfg, agentTypeClient, mock)

	out, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	cmd := parseProcessCommand(t, mock.processData)
	assert.Equal(t, "set_config", cmd["command"])
	assert.Equal(t, "1", cmd["device_id"])
	entries := resultEntries(t, out[0], "results")
	assert.Equal(t, true, entries[0]["ok"])
}

// TestModbusControl_GenericCommand 는 범용 command 탈출구가 {command,params}를 그대로 전달함을 검증한다.
func TestModbusControl_GenericCommand(t *testing.T) {
	mock := &mockModbusAgent{processResp: []byte(`{"ok":true,"state":"running"}`)}
	cfg := map[string]any{
		"agent_ref": "mb-server",
		"command_set": []any{
			map[string]any{"action": "command", "params": map[string]any{
				"command": "get_status",
				"params":  map[string]any{"verbose": true},
			}},
		},
	}
	n := setupControlNode(t, cfg, agentTypeServer, mock)

	out, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	cmd := parseProcessCommand(t, mock.processData)
	assert.Equal(t, "get_status", cmd["command"])
	params := cmd["params"].(map[string]any)
	assert.Equal(t, true, params["verbose"])
	entries := resultEntries(t, out[0], "results")
	assert.Equal(t, true, entries[0]["ok"])
}

// TestModbusControl_UnsupportedAction 은 미지원 액션이 op 실패 사유를 남김을 검증한다.
func TestModbusControl_UnsupportedAction(t *testing.T) {
	mock := &mockModbusAgent{}
	cfg := map[string]any{
		"agent_ref":   "mb-client",
		"command_set": []any{map[string]any{"action": "explode"}},
	}
	n := setupControlNode(t, cfg, agentTypeClient, mock)

	out, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	entries := resultEntries(t, out[0], "results")
	assert.Equal(t, false, entries[0]["ok"])
	assert.Contains(t, entries[0]["reason"], "unsupported action")
}

// ===========================================================================
// 공통: Configure / EmptyCommandSet
// ===========================================================================

// TestModbusCmdSet_MissingAgentRef 는 agent_ref 누락 시 Configure가 실패함을 검증한다.
func TestModbusCmdSet_MissingAgentRef(t *testing.T) {
	node, err := NewModbusWriteNode(flow.NewNodeDef("t", "modbus-write"))
	require.NoError(t, err)
	err = node.(*ModbusWriteNode).Configure(map[string]any{"command_set": []any{}})
	assert.ErrorIs(t, err, ErrModbusMissingAgentRef)
}

// TestModbusCmdSet_EmptyCommandSet 는 command_set이 비어있을 때 에러를 반환함을 검증한다.
func TestModbusCmdSet_EmptyCommandSet(t *testing.T) {
	mock := &mockModbusAgent{}
	n := setupWriteNode(t, map[string]any{"agent_ref": "x"}, agentTypeServer, mock)
	_, err := n.Process(context.Background(), message.New())
	assert.ErrorIs(t, err, ErrModbusEmptyCommandSet)
}

// mustGet 은 payload에서 문자열 값을 꺼내는 테스트 헬퍼이다.
func mustGet(t *testing.T, msg message.Message, key string) any {
	t.Helper()
	v, ok := msg.Payload().Get(key)
	require.True(t, ok, "payload에 %q 키가 있어야 한다", key)
	return v
}
