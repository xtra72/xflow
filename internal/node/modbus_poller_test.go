package node

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent/modbusserver"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 헬퍼 함수
// ---------------------------------------------------------------------------

func newModbusPollerNodeDef(name string) flow.NodeDef {
	return flow.NewNodeDef(name, "modbus-poller")
}

// mustJSON 은 테스트용 JSON 바이트를 생성하는 헬퍼이다.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// ---------------------------------------------------------------------------
// 1. TestNewModbusPollerNode_Basic - 팩토리 함수 기본 생성
// ---------------------------------------------------------------------------

func TestNewModbusPollerNode_Basic(t *testing.T) {
	def := newModbusPollerNodeDef("test-poller")
	node, err := NewModbusPollerNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)

	// SourceNode 인터페이스 확인
	sn, ok := node.(SourceNode)
	assert.True(t, ok)
	assert.NotNil(t, sn.SourceCh())
}

// ---------------------------------------------------------------------------
// 2. TestModbusPollerNode_Configure - 설정 테스트
// ---------------------------------------------------------------------------

// TestModbusPollerNode_Configure_Valid 는 유효한 설정이 올바르게 파싱되는지 확인한다.
func TestModbusPollerNode_Configure_Valid(t *testing.T) {
	def := newModbusPollerNodeDef("test-poller")
	node, _ := NewModbusPollerNode(def)
	n := node.(*ModbusPollerNode)

	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"device_id":     float64(2),
		"poll_interval": "2s",
		"register_map": []any{
			map[string]any{"name": "temperature", "address": float64(0), "count": float64(2), "data_type": "float32"},
			map[string]any{"name": "humidity", "address": float64(2), "count": float64(2), "data_type": "float32"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "modbus-server-1", n.pollerConfig.AgentRef)
	assert.Equal(t, uint8(2), n.pollerConfig.DeviceID)
	assert.Equal(t, 2*time.Second, n.pollInterval)
	assert.Len(t, n.pollerConfig.RegisterMap, 2)
}

// TestModbusPollerNode_Configure_Defaults 는 기본값이 올바르게 적용되는지 확인한다.
func TestModbusPollerNode_Configure_Defaults(t *testing.T) {
	def := newModbusPollerNodeDef("test-poller")
	node, _ := NewModbusPollerNode(def)
	n := node.(*ModbusPollerNode)

	err := n.Configure(map[string]any{
		"agent_ref": "modbus-server-1",
		"register_map": []any{
			map[string]any{"name": "temp", "address": float64(0)},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, uint8(1), n.pollerConfig.DeviceID)
	assert.Equal(t, 5*time.Second, n.pollInterval)

	// register_map 항목 기본값 확인
	entry := n.pollerConfig.RegisterMap[0]
	assert.Equal(t, "holding_registers", entry.RegisterArea)
	assert.Equal(t, uint16(1), entry.Count)
	assert.Equal(t, "uint16", entry.DataType)
	assert.Equal(t, "big_endian", entry.ByteOrder)
	assert.Equal(t, uint8(1), entry.DeviceID) // config의 device_id 상속
}

// TestModbusPollerNode_Configure_MissingAgentRef 는 agent_ref 누락 시 에러를 반환하는지 확인한다.
func TestModbusPollerNode_Configure_MissingAgentRef(t *testing.T) {
	def := newModbusPollerNodeDef("test-poller")
	node, _ := NewModbusPollerNode(def)
	n := node.(*ModbusPollerNode)

	err := n.Configure(map[string]any{
		"register_map": []any{
			map[string]any{"name": "temp", "address": float64(0)},
		},
	})
	assert.ErrorIs(t, err, ErrModbusPollerMissingAgentRef)
}

// TestModbusPollerNode_Configure_MissingRegisterMap 는 register_map 누락 시 에러를 반환하는지 확인한다.
func TestModbusPollerNode_Configure_MissingRegisterMap(t *testing.T) {
	def := newModbusPollerNodeDef("test-poller")
	node, _ := NewModbusPollerNode(def)
	n := node.(*ModbusPollerNode)

	err := n.Configure(map[string]any{
		"agent_ref": "modbus-server-1",
	})
	assert.ErrorIs(t, err, ErrModbusPollerMissingRegisterMap)
}

// TestModbusPollerNode_Configure_InvalidPollInterval 는 poll_interval이 100ms 미만일 때 에러를 반환하는지 확인한다.
func TestModbusPollerNode_Configure_InvalidPollInterval(t *testing.T) {
	def := newModbusPollerNodeDef("test-poller")
	node, _ := NewModbusPollerNode(def)
	n := node.(*ModbusPollerNode)

	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"poll_interval": "50ms",
		"register_map": []any{
			map[string]any{"name": "temp", "address": float64(0)},
		},
	})
	assert.ErrorIs(t, err, ErrModbusPollerInvalidPollInterval)
}

// TestModbusPollerNode_Configure_WithRegisterMap 는 register_map의 register_area, device_id가 올바르게 파싱되는지 확인한다.
func TestModbusPollerNode_Configure_WithRegisterMap(t *testing.T) {
	def := newModbusPollerNodeDef("test-poller")
	node, _ := NewModbusPollerNode(def)
	n := node.(*ModbusPollerNode)

	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"device_id":     float64(1),
		"poll_interval": "1s",
		"register_map": []any{
			map[string]any{
				"name": "temperature", "register_area": "input_registers",
				"address": float64(0), "count": float64(2), "data_type": "float32",
				"device_id": float64(3),
			},
			map[string]any{
				"name": "coil_status", "register_area": "coils",
				"address": float64(0), "count": float64(8),
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, n.pollerConfig.RegisterMap, 2)

	// 항목별 register_area, device_id 확인
	assert.Equal(t, "input_registers", n.pollerConfig.RegisterMap[0].RegisterArea)
	assert.Equal(t, uint8(3), n.pollerConfig.RegisterMap[0].DeviceID) // 개별 지정
	assert.Equal(t, "coils", n.pollerConfig.RegisterMap[1].RegisterArea)
	assert.Equal(t, uint8(1), n.pollerConfig.RegisterMap[1].DeviceID) // config 기본값 상속
}

// ---------------------------------------------------------------------------
// 3. TestModbusPollerNode_Init - 초기화 테스트
// ---------------------------------------------------------------------------

// TestModbusPollerNode_Init_Success 는 Server Agent 감지 및 초기화 성공을 확인한다.
func TestModbusPollerNode_Init_Success(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusPollerNodeDef("test-poller")
	node, _ := NewModbusPollerNode(def, WithAgentResolver(resolver))
	n := node.(*ModbusPollerNode)

	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"poll_interval": "1s",
		"register_map": []any{
			map[string]any{"name": "temp", "address": float64(0), "count": float64(2), "data_type": "float32"},
		},
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "server", n.agentType)

	// Cleanup
	n.Shutdown(context.Background())
}

// TestModbusPollerNode_Init_NoResolver 는 AgentResolver가 없을 때 에러를 반환하는지 확인한다.
func TestModbusPollerNode_Init_NoResolver(t *testing.T) {
	def := newModbusPollerNodeDef("test-poller")
	node, _ := NewModbusPollerNode(def)
	n := node.(*ModbusPollerNode)

	err := n.Configure(map[string]any{
		"agent_ref": "modbus-server-1",
		"register_map": []any{
			map[string]any{"name": "temp", "address": float64(0)},
		},
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	assert.ErrorIs(t, err, ErrModbusNoResolver)
}

// ---------------------------------------------------------------------------
// 4. TestModbusPollerNode_PollLoop - 폴링 루프 테스트
// ---------------------------------------------------------------------------

// TestModbusPollerNode_PollLoop_RegisterMap 는 register_map 기반 폴링을 테스트한다.
func TestModbusPollerNode_PollLoop_RegisterMap(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusPollerNodeDef("test-poller")
	node, _ := NewModbusPollerNode(def, WithAgentResolver(resolver))
	n := node.(*ModbusPollerNode)

	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"poll_interval": "200ms",
		"register_map": []any{
			map[string]any{"name": "temperature", "address": float64(0), "count": float64(2), "data_type": "float32"},
			map[string]any{"name": "humidity", "address": float64(2), "count": float64(2), "data_type": "float32"},
		},
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	// Mock responds with value for each read (mutex 보호로 pollLoop goroutine과의 data race 방지)
	mockAgent := &mockModbusAgent{
		processResp: mustJSON(t, map[string]any{"ok": true, "value": 42.0}),
	}
	n.mu.Lock()
	n.agent = mockAgent
	n.mu.Unlock()

	select {
	case msg := <-n.SourceCh():
		assert.NotNil(t, msg)
		temp, ok := msg.Payload().Get("temperature")
		assert.True(t, ok)
		assert.Equal(t, 42.0, temp)
		humi, ok := msg.Payload().Get("humidity")
		assert.True(t, ok)
		assert.Equal(t, 42.0, humi)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for poll message")
	}

	n.Shutdown(context.Background())
}

// TestModbusPollerNode_PollLoop_SetsMessageTypeEvent 는 modbus 폴 루프가
// emit 한 메시지가 metadata.message_type="event" 와 modbus_source="poll" 을
// 모두 가지는지 확인한다. 통일 분류 표준: 2026-05-14 SPEC.
func TestModbusPollerNode_PollLoop_SetsMessageTypeEvent(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusPollerNodeDef("test-poller-mt")
	node, _ := NewModbusPollerNode(def, WithAgentResolver(resolver))
	n := node.(*ModbusPollerNode)

	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"poll_interval": "200ms",
		"register_map": []any{
			map[string]any{"name": "temperature", "address": float64(0), "count": float64(2), "data_type": "float32"},
		},
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	mockAgent := &mockModbusAgent{
		processResp: mustJSON(t, map[string]any{"ok": true, "value": 42.0}),
	}
	n.mu.Lock()
	n.agent = mockAgent
	n.mu.Unlock()

	select {
	case msg := <-n.SourceCh():
		mt, ok := msg.Metadata().Get("message_type")
		require.True(t, ok, "message_type 메타데이터 누락 — agent 노드 통일 표준 위반")
		assert.Equal(t, "event", mt, "modbus poll 은 event 분류여야 한다")

		source, ok := msg.Metadata().Get("modbus_source")
		require.True(t, ok)
		assert.Equal(t, "poll", source)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for poll message")
	}

	n.Shutdown(context.Background())
}

// ---------------------------------------------------------------------------
// 5. TestModbusPollerNode_Process - 동적 설정 변경 테스트
// ---------------------------------------------------------------------------

// TestModbusPollerNode_Process_OverrideDeviceID 는 메시지로 device_id를 오버라이드할 수 있는지 확인한다.
func TestModbusPollerNode_Process_OverrideDeviceID(t *testing.T) {
	def := newModbusPollerNodeDef("test-poller")
	node, _ := NewModbusPollerNode(def)
	n := node.(*ModbusPollerNode)

	err := n.Configure(map[string]any{
		"agent_ref": "modbus-server-1",
		"register_map": []any{
			map[string]any{"name": "temp", "address": float64(0)},
		},
	})
	require.NoError(t, err)

	msg := message.New()
	msg.Payload().Set("device_id", float64(5))

	result, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, result, 1)

	updated, ok := result[0].Payload().Get("config_updated")
	assert.True(t, ok)
	assert.Equal(t, true, updated)

	n.mu.RLock()
	assert.Equal(t, uint8(5), n.pollerConfig.DeviceID)
	n.mu.RUnlock()
}

// TestModbusPollerNode_Process_OverridePollInterval 은 poll_interval 오버라이드 시 reconfigCh 신호를 확인한다.
func TestModbusPollerNode_Process_OverridePollInterval(t *testing.T) {
	def := newModbusPollerNodeDef("test-poller")
	node, _ := NewModbusPollerNode(def)
	n := node.(*ModbusPollerNode)

	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"poll_interval": "5s",
		"register_map": []any{
			map[string]any{"name": "temp", "address": float64(0)},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, n.pollInterval)

	// poll_interval 변경
	msg := message.New()
	msg.Payload().Set("poll_interval", "2s")

	result, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, result, 1)

	// pollInterval 반영 확인
	n.mu.RLock()
	assert.Equal(t, 2*time.Second, n.pollInterval)
	assert.Equal(t, "2s", n.pollerConfig.PollInterval)
	n.mu.RUnlock()

	// reconfigCh에 신호가 전달되었는지 확인
	select {
	case <-n.reconfigCh:
		// ok
	default:
		t.Fatal("reconfigCh에 신호가 전달되지 않았다")
	}
}

// TestModbusPollerNode_Process_OverridePollInterval_TooShort 은 최소값 미만 poll_interval은 무시되는지 확인한다.
func TestModbusPollerNode_Process_OverridePollInterval_TooShort(t *testing.T) {
	def := newModbusPollerNodeDef("test-poller")
	node, _ := NewModbusPollerNode(def)
	n := node.(*ModbusPollerNode)

	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"poll_interval": "5s",
		"register_map": []any{
			map[string]any{"name": "temp", "address": float64(0)},
		},
	})
	require.NoError(t, err)

	// 최소값(100ms) 미만은 무시됨
	msg := message.New()
	msg.Payload().Set("poll_interval", "50ms")

	result, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, result, 1)

	// 기존 값 유지
	n.mu.RLock()
	assert.Equal(t, 5*time.Second, n.pollInterval)
	n.mu.RUnlock()

	// config_updated는 false (다른 변경 없으므로)
	updated, ok := result[0].Payload().Get("config_updated")
	assert.True(t, ok)
	assert.Equal(t, false, updated)
}

// TestModbusPollerNode_Process_OverrideRegisterMap 은 register_map 동적 변경을 확인한다.
func TestModbusPollerNode_Process_OverrideRegisterMap(t *testing.T) {
	def := newModbusPollerNodeDef("test-poller")
	node, _ := NewModbusPollerNode(def)
	n := node.(*ModbusPollerNode)

	err := n.Configure(map[string]any{
		"agent_ref": "modbus-server-1",
		"register_map": []any{
			map[string]any{"name": "temp", "address": float64(0)},
		},
	})
	require.NoError(t, err)
	assert.Len(t, n.pollerConfig.RegisterMap, 1)

	// register_map 동적 변경
	msg := message.New()
	msg.Payload().Set("register_map", []any{
		map[string]any{"name": "temp", "address": float64(0), "count": float64(2), "data_type": "float32"},
		map[string]any{"name": "humi", "address": float64(2), "count": float64(2), "data_type": "float32"},
	})

	result, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, result, 1)

	n.mu.RLock()
	assert.Len(t, n.pollerConfig.RegisterMap, 2)
	assert.Equal(t, "temp", n.pollerConfig.RegisterMap[0].Name)
	assert.Equal(t, "humi", n.pollerConfig.RegisterMap[1].Name)
	n.mu.RUnlock()
}

// TestModbusPollerNode_Process_NilPayload 는 nil payload 시 passthrough을 확인한다.
func TestModbusPollerNode_Process_NilPayload(t *testing.T) {
	def := newModbusPollerNodeDef("test-poller")
	node, _ := NewModbusPollerNode(def)
	n := node.(*ModbusPollerNode)

	err := n.Configure(map[string]any{
		"agent_ref": "modbus-server-1",
		"register_map": []any{
			map[string]any{"name": "temp", "address": float64(0)},
		},
	})
	require.NoError(t, err)

	msg := message.New()
	result, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, result, 1)
}

// ---------------------------------------------------------------------------
// 6. TestModbusPollerNode_Shutdown - 종료 테스트
// ---------------------------------------------------------------------------

// TestModbusPollerNode_Shutdown 은 이중 종료 시에도 패닉이 발생하지 않는지 확인한다.
func TestModbusPollerNode_Shutdown(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusPollerNodeDef("test-poller")
	node, _ := NewModbusPollerNode(def, WithAgentResolver(resolver))
	n := node.(*ModbusPollerNode)

	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"poll_interval": "1s",
		"register_map": []any{
			map[string]any{"name": "temp", "address": float64(0)},
		},
	})
	require.NoError(t, err)
	err = n.Init(context.Background())
	require.NoError(t, err)

	// Double shutdown should not panic
	assert.NotPanics(t, func() {
		n.Shutdown(context.Background())
		n.Shutdown(context.Background())
	})
}
