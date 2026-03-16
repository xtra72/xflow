package node

import (
	"context"
	"fmt"
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

func newModbusWriterNodeDef(name string) flow.NodeDef {
	return flow.NewNodeDef(name, "modbus-writer")
}

// ---------------------------------------------------------------------------
// 1. TestNewModbusWriterNode_Basic - 팩토리 함수 기본 생성
// ---------------------------------------------------------------------------

// TestNewModbusWriterNode_Basic 는 ModbusWriterNode가 올바르게 생성되는지 확인한다.
// Node 인터페이스를 구현하지만 SourceNode 인터페이스는 구현하지 않아야 한다.
func TestNewModbusWriterNode_Basic(t *testing.T) {
	def := newModbusWriterNodeDef("test-writer")
	node, err := NewModbusWriterNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "test-writer", node.Name())
	assert.Equal(t, "modbus-writer", node.Type())

	// SourceNode 인터페이스를 구현하지 않아야 한다 (ProcessNode이므로)
	_, ok := node.(SourceNode)
	assert.False(t, ok, "ModbusWriterNode는 SourceNode가 아니어야 한다")
}

// ---------------------------------------------------------------------------
// 2. TestModbusWriterNode_Configure - 설정 테스트
// ---------------------------------------------------------------------------

// TestModbusWriterNode_Configure_Valid 는 유효한 설정이 올바르게 파싱되는지 확인한다.
func TestModbusWriterNode_Configure_Valid(t *testing.T) {
	def := newModbusWriterNodeDef("test-writer")
	node, _ := NewModbusWriterNode(def)
	n := node.(*ModbusWriterNode)

	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"register_area": "holding_registers",
		"address":       float64(100),
		"count":         float64(10),
		"data_type":     "float32",
		"byte_order":    "little_endian",
		"device_id":     float64(5),
		"timeout":       "10s",
	})
	require.NoError(t, err)
	assert.Equal(t, "modbus-server-1", n.writerConfig.AgentRef)
	assert.Equal(t, "holding_registers", n.writerConfig.RegisterArea)
	assert.Equal(t, uint16(100), n.writerConfig.Address)
	assert.Equal(t, uint16(10), n.writerConfig.Count)
	assert.Equal(t, "float32", n.writerConfig.DataType)
	assert.Equal(t, "little_endian", n.writerConfig.ByteOrder)
	assert.Equal(t, uint8(5), n.writerConfig.DeviceID)
	assert.Equal(t, 10*time.Second, n.timeout)
}

// TestModbusWriterNode_Configure_Defaults 는 기본값이 올바르게 적용되는지 확인한다.
func TestModbusWriterNode_Configure_Defaults(t *testing.T) {
	def := newModbusWriterNodeDef("test-writer")
	node, _ := NewModbusWriterNode(def)
	n := node.(*ModbusWriterNode)

	err := n.Configure(map[string]any{
		"agent_ref": "modbus-server-1",
	})
	require.NoError(t, err)
	assert.Equal(t, "holding_registers", n.writerConfig.RegisterArea)
	assert.Equal(t, uint16(1), n.writerConfig.Count)
	assert.Equal(t, "uint16", n.writerConfig.DataType)
	assert.Equal(t, "big_endian", n.writerConfig.ByteOrder)
	assert.Equal(t, uint8(1), n.writerConfig.DeviceID)
	assert.Equal(t, 5*time.Second, n.timeout)
}

// ---------------------------------------------------------------------------
// 3. TestModbusWriterNode_Configure_Errors - 설정 에러 테스트 (테이블 기반)
// ---------------------------------------------------------------------------

// TestModbusWriterNode_Configure_Errors 는 설정 에러를 테이블 기반으로 테스트한다.
func TestModbusWriterNode_Configure_Errors(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]any
		wantErr error
	}{
		{
			name:    "agent_ref 누락",
			config:  map[string]any{},
			wantErr: ErrModbusWriterMissingAgentRef,
		},
		{
			name: "agent_ref 빈문자열",
			config: map[string]any{
				"agent_ref": "",
			},
			wantErr: ErrModbusWriterMissingAgentRef,
		},
		{
			name: "읽기전용 영역 - discrete_inputs",
			config: map[string]any{
				"agent_ref":     "server-1",
				"register_area": "discrete_inputs",
			},
			wantErr: ErrModbusWriterInvalidRegisterArea,
		},
		{
			name: "읽기전용 영역 - input_registers",
			config: map[string]any{
				"agent_ref":     "server-1",
				"register_area": "input_registers",
			},
			wantErr: ErrModbusWriterInvalidRegisterArea,
		},
		{
			name: "잘못된 register_area",
			config: map[string]any{
				"agent_ref":     "server-1",
				"register_area": "invalid_area",
			},
			wantErr: ErrModbusWriterInvalidRegisterArea,
		},
		{
			name: "존재하지 않는 register_area",
			config: map[string]any{
				"agent_ref":     "server-1",
				"register_area": "some_nonexistent_area",
			},
			wantErr: ErrModbusWriterInvalidRegisterArea,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := newModbusWriterNodeDef("test-cfg-err")
			node, err := NewModbusWriterNode(def)
			require.NoError(t, err)

			n := node.(*ModbusWriterNode)
			err = n.Configure(tt.config)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestModbusWriterNode_Configure_WritableAreas 는 쓰기 가능 영역이 정상 통과하는지 확인한다.
func TestModbusWriterNode_Configure_WritableAreas(t *testing.T) {
	tests := []struct {
		name string
		area string
	}{
		{"coils 영역", "coils"},
		{"holding_registers 영역", "holding_registers"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := newModbusWriterNodeDef("test-writable")
			node, _ := NewModbusWriterNode(def)
			n := node.(*ModbusWriterNode)

			err := n.Configure(map[string]any{
				"agent_ref":     "server-1",
				"register_area": tt.area,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.area, n.writerConfig.RegisterArea)
		})
	}
}

// ---------------------------------------------------------------------------
// 4. TestModbusWriterNode_Init - 초기화 테스트
// ---------------------------------------------------------------------------

// TestModbusWriterNode_Init_Success 는 Server Agent 감지 및 초기화 성공을 확인한다.
func TestModbusWriterNode_Init_Success(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusWriterNodeDef("test-writer")
	node, _ := NewModbusWriterNode(def, WithAgentResolver(resolver))
	n := node.(*ModbusWriterNode)

	err := n.Configure(map[string]any{
		"agent_ref": "modbus-server-1",
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "server", n.agentType)

	// Cleanup
	n.Shutdown(context.Background())
}

// TestModbusWriterNode_Init_NoResolver 는 AgentResolver가 없을 때 에러를 반환하는지 확인한다.
func TestModbusWriterNode_Init_NoResolver(t *testing.T) {
	def := newModbusWriterNodeDef("test-writer")
	node, _ := NewModbusWriterNode(def)
	n := node.(*ModbusWriterNode)

	err := n.Configure(map[string]any{
		"agent_ref": "modbus-server-1",
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	assert.ErrorIs(t, err, ErrModbusNoResolver)
}

// ---------------------------------------------------------------------------
// 5. TestModbusWriterNode_Process_SingleWrite - 단일 값 쓰기 테스트
// ---------------------------------------------------------------------------

// TestModbusWriterNode_Process_SingleRegisterWrite 는 Server Agent에 단일 레지스터 쓰기를 테스트한다.
func TestModbusWriterNode_Process_SingleRegisterWrite(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusWriterNodeDef("test-writer")
	node, _ := NewModbusWriterNode(def, WithAgentResolver(resolver))
	n := node.(*ModbusWriterNode)

	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"register_area": "holding_registers",
		"address":       float64(100),
		"data_type":     "float32",
		"byte_order":    "big_endian",
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	// Init 후 agent를 mock으로 교체 (mutex 보호)
	respBytes := mustJSON(t, map[string]any{"ok": true})
	mockAgent := &mockModbusAgent{processResp: respBytes}
	n.mu.Lock()
	n.agent = mockAgent
	n.mu.Unlock()

	// value=42.5 쓰기
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"value": 42.5,
	})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// Agent에 전달된 명령 검증
	cmd := parseProcessCommand(t, mockAgent.processData)
	assert.Equal(t, "set_register", cmd["command"])

	params, ok := cmd["params"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(100), params["address"])
	assert.Equal(t, 42.5, params["value"])
	assert.Equal(t, "float32", params["data_type"])
	assert.Equal(t, "big_endian", params["byte_order"])

	// 출력 메시지 검증
	outPayload := results[0].Payload()
	success, ok := outPayload.Get("success")
	assert.True(t, ok)
	assert.Equal(t, true, success)

	area, ok := outPayload.Get("register_area")
	assert.True(t, ok)
	assert.Equal(t, "holding_registers", area)

	agentType, ok := outPayload.Get("agent_type")
	assert.True(t, ok)
	assert.Equal(t, "server", agentType)
}

// ---------------------------------------------------------------------------
// 6. TestModbusWriterNode_Process_CoilWrite - 코일 쓰기 테스트
// ---------------------------------------------------------------------------

// TestModbusWriterNode_Process_CoilWrite 는 Server Agent에 코일 쓰기를 테스트한다.
func TestModbusWriterNode_Process_CoilWrite(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusWriterNodeDef("test-coil-writer")
	node, _ := NewModbusWriterNode(def, WithAgentResolver(resolver))
	n := node.(*ModbusWriterNode)

	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"register_area": "coils",
		"address":       float64(10),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	respBytes := mustJSON(t, map[string]any{"ok": true})
	mockAgent := &mockModbusAgent{processResp: respBytes}
	n.mu.Lock()
	n.agent = mockAgent
	n.mu.Unlock()

	// coil 값 true 쓰기
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"value": true,
	})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// Agent에 전달된 명령 검증
	cmd := parseProcessCommand(t, mockAgent.processData)
	assert.Equal(t, "set_coil", cmd["command"])

	params, ok := cmd["params"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(10), params["address"])
	assert.Equal(t, true, params["value"])

	// coils 영역이므로 data_type/byte_order가 없어야 한다
	_, hasDataType := params["data_type"]
	assert.False(t, hasDataType, "coils 영역에는 data_type이 없어야 한다")
}

// ---------------------------------------------------------------------------
// 7. TestModbusWriterNode_Process_MessageOverride - 메시지 오버라이드 테스트
// ---------------------------------------------------------------------------

// TestModbusWriterNode_Process_MessageOverride 는 payload에서 address/data_type 등을 오버라이드하는지 확인한다.
func TestModbusWriterNode_Process_MessageOverride(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusWriterNodeDef("test-override-writer")
	node, _ := NewModbusWriterNode(def, WithAgentResolver(resolver))
	n := node.(*ModbusWriterNode)

	// 기본 설정: address=0, data_type=uint16
	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"register_area": "holding_registers",
		"address":       float64(0),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	respBytes := mustJSON(t, map[string]any{"ok": true})
	mockAgent := &mockModbusAgent{processResp: respBytes}
	n.mu.Lock()
	n.agent = mockAgent
	n.mu.Unlock()

	// payload에서 address, data_type를 오버라이드
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"value":     float64(999),
		"address":   float64(200),
		"data_type": "float32",
	})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// Agent에 전달된 명령에서 오버라이드된 address 확인
	cmd := parseProcessCommand(t, mockAgent.processData)
	params, ok := cmd["params"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(200), params["address"], "메시지 payload의 address가 우선해야 한다")
	assert.Equal(t, "float32", params["data_type"], "메시지 payload의 data_type이 우선해야 한다")

	// 출력 메시지에 오버라이드된 값이 반영되었는지 확인
	outPayload := results[0].Payload()
	addr, ok := outPayload.Get("address")
	assert.True(t, ok)
	assert.Equal(t, uint16(200), addr)

	dt, ok := outPayload.Get("data_type")
	assert.True(t, ok)
	assert.Equal(t, "float32", dt)
}

// ---------------------------------------------------------------------------
// 8. TestModbusWriterNode_Process_NoValue - value/values 누락 테스트
// ---------------------------------------------------------------------------

// TestModbusWriterNode_Process_NoValue 는 payload에 value와 values가 모두 없을 때 조용히 스킵하는지 확인한다.
func TestModbusWriterNode_Process_NoValue(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusWriterNodeDef("test-no-value")
	node, _ := NewModbusWriterNode(def, WithAgentResolver(resolver))
	n := node.(*ModbusWriterNode)

	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"register_area": "holding_registers",
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	respBytes := mustJSON(t, map[string]any{"ok": true})
	mockAgent := &mockModbusAgent{processResp: respBytes}
	n.mu.Lock()
	n.agent = mockAgent
	n.mu.Unlock()

	// value/values 없이 메시지 전송 → 조용히 스킵 (strip_nulls 호환)
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"other": "data",
	})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Nil(t, results)
}

// ---------------------------------------------------------------------------
// 9. TestModbusWriterNode_Process_ReadOnlyOverride - 읽기전용 영역 오버라이드 차단 테스트
// ---------------------------------------------------------------------------

// TestModbusWriterNode_Process_ReadOnlyOverride 는 메시지 오버라이드로 읽기전용 영역이 지정되었을 때 에러를 반환하는지 확인한다.
func TestModbusWriterNode_Process_ReadOnlyOverride(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusWriterNodeDef("test-readonly-override")
	node, _ := NewModbusWriterNode(def, WithAgentResolver(resolver))
	n := node.(*ModbusWriterNode)

	// 쓰기 가능 영역으로 Configure (통과해야 함)
	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"register_area": "holding_registers",
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	respBytes := mustJSON(t, map[string]any{"ok": true})
	mockAgent := &mockModbusAgent{processResp: respBytes}
	n.mu.Lock()
	n.agent = mockAgent
	n.mu.Unlock()

	// 메시지에서 register_area를 읽기전용(input_registers)으로 오버라이드
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"value":         float64(42),
		"register_area": "input_registers",
	})))

	_, err = n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrModbusReadOnlyArea)
}

// ---------------------------------------------------------------------------
// 10. TestModbusWriterNode_Process_AgentProcessError - Agent 에러 전파 테스트
// ---------------------------------------------------------------------------

// TestModbusWriterNode_Process_AgentProcessError 는 Agent Process() 에러가 올바르게 전파되는지 확인한다.
func TestModbusWriterNode_Process_AgentProcessError(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusWriterNodeDef("test-agent-err")
	node, _ := NewModbusWriterNode(def, WithAgentResolver(resolver))
	n := node.(*ModbusWriterNode)

	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"register_area": "holding_registers",
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	// 에러를 반환하는 mock agent
	mockAgent := &mockModbusAgent{
		processErr: fmt.Errorf("모드버스 통신 실패"),
	}
	n.mu.Lock()
	n.agent = mockAgent
	n.mu.Unlock()

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"value": float64(42),
	})))

	_, err = n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrModbusProcessFailed)
}

// ---------------------------------------------------------------------------
// 11. TestModbusWriterNode_Process_MultipleValues - 다중 값 쓰기 테스트
// ---------------------------------------------------------------------------

// TestModbusWriterNode_Process_MultipleValues 는 count>1 + values로 다중 쓰기를 테스트한다.
func TestModbusWriterNode_Process_MultipleValues(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusWriterNodeDef("test-multi-write")
	node, _ := NewModbusWriterNode(def, WithAgentResolver(resolver))
	n := node.(*ModbusWriterNode)

	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"register_area": "holding_registers",
		"address":       float64(200),
		"count":         float64(3),
		"data_type":     "float32",
		"byte_order":    "big_endian",
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	respBytes := mustJSON(t, map[string]any{"ok": true})
	mockAgent := &mockModbusAgent{processResp: respBytes}
	n.mu.Lock()
	n.agent = mockAgent
	n.mu.Unlock()

	// 다중 값 쓰기
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"values": []any{1.1, 2.2, 3.3},
	})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	cmd := parseProcessCommand(t, mockAgent.processData)
	assert.Equal(t, "set_registers", cmd["command"])

	params, ok := cmd["params"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(200), params["address"])
	assert.Equal(t, "float32", params["data_type"])
	assert.Equal(t, "big_endian", params["byte_order"])

	values, ok := params["values"].([]any)
	require.True(t, ok)
	assert.Len(t, values, 3)
}

// ---------------------------------------------------------------------------
// 12. TestModbusWriterNode_Process_MultipleCoils - 다중 코일 쓰기 테스트
// ---------------------------------------------------------------------------

// TestModbusWriterNode_Process_MultipleCoils 는 count>1 + values로 다중 코일 쓰기를 테스트한다.
func TestModbusWriterNode_Process_MultipleCoils(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusWriterNodeDef("test-multi-coil")
	node, _ := NewModbusWriterNode(def, WithAgentResolver(resolver))
	n := node.(*ModbusWriterNode)

	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"register_area": "coils",
		"address":       float64(20),
		"count":         float64(4),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	respBytes := mustJSON(t, map[string]any{"ok": true})
	mockAgent := &mockModbusAgent{processResp: respBytes}
	n.mu.Lock()
	n.agent = mockAgent
	n.mu.Unlock()

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"values": []any{true, false, true, false},
	})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	cmd := parseProcessCommand(t, mockAgent.processData)
	assert.Equal(t, "set_coils", cmd["command"])

	params, ok := cmd["params"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(20), params["address"])

	values, ok := params["values"].([]any)
	require.True(t, ok)
	assert.Len(t, values, 4)
}

// ---------------------------------------------------------------------------
// 13. TestModbusWriterNode_Process_OutputPayload - 출력 메시지 상세 검증
// ---------------------------------------------------------------------------

// TestModbusWriterNode_Process_OutputPayload 는 쓰기 성공 시 출력 메시지의 필드를 상세 검증한다.
func TestModbusWriterNode_Process_OutputPayload(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusWriterNodeDef("test-output")
	node, _ := NewModbusWriterNode(def, WithAgentResolver(resolver))
	n := node.(*ModbusWriterNode)

	err := n.Configure(map[string]any{
		"agent_ref":     "modbus-server-1",
		"register_area": "holding_registers",
		"address":       float64(50),
		"count":         float64(2),
		"data_type":     "float32",
		"byte_order":    "little_endian",
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	respBytes := mustJSON(t, map[string]any{"ok": true})
	mockAgent := &mockModbusAgent{processResp: respBytes}
	n.mu.Lock()
	n.agent = mockAgent
	n.mu.Unlock()

	// 원본 payload에 커스텀 키를 포함
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"value":     float64(25.5),
		"sensor_id": "temp-001",
	})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	outPayload := results[0].Payload()

	// success 플래그 확인
	success, ok := outPayload.Get("success")
	assert.True(t, ok)
	assert.Equal(t, true, success)

	// 메타 정보 확인
	area, ok := outPayload.Get("register_area")
	assert.True(t, ok)
	assert.Equal(t, "holding_registers", area)

	addr, ok := outPayload.Get("address")
	assert.True(t, ok)
	assert.Equal(t, uint16(50), addr)

	// count는 data_type에서 유추 가능하므로 출력하지 않음
	_, hasCnt := outPayload.Get("count")
	assert.False(t, hasCnt)

	dt, ok := outPayload.Get("data_type")
	assert.True(t, ok)
	assert.Equal(t, "float32", dt)

	bo, ok := outPayload.Get("byte_order")
	assert.True(t, ok)
	assert.Equal(t, "little_endian", bo)

	agentType, ok := outPayload.Get("agent_type")
	assert.True(t, ok)
	assert.Equal(t, "server", agentType)

	// 원본 payload 키 보존 확인
	sensorID, ok := outPayload.Get("sensor_id")
	assert.True(t, ok)
	assert.Equal(t, "temp-001", sensorID)
}

// ---------------------------------------------------------------------------
// 14. TestModbusWriterNode_Shutdown - 종료 테스트
// ---------------------------------------------------------------------------

// TestModbusWriterNode_Shutdown 은 이중 종료 시에도 패닉이 발생하지 않는지 확인한다.
func TestModbusWriterNode_Shutdown(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusWriterNodeDef("test-shutdown")
	node, _ := NewModbusWriterNode(def, WithAgentResolver(resolver))
	n := node.(*ModbusWriterNode)

	err := n.Configure(map[string]any{
		"agent_ref": "modbus-server-1",
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
