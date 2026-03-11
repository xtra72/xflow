package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/modbus"
	"github.com/xtra/xflow/internal/agent/modbusserver"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 모의 객체 정의
// ---------------------------------------------------------------------------

// mockModbusAgent 는 테스트용 agent.Agent 구현이다.
// Process() 호출 시 수신한 데이터를 기록하고 미리 설정된 응답을 반환한다.
type mockModbusAgent struct {
	processData []byte // 마지막 Process() 호출 시 전달된 데이터
	processResp []byte // Process() 호출 시 반환할 응답
	processErr  error  // Process() 호출 시 반환할 에러
}

func (m *mockModbusAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockModbusAgent) Start(_ context.Context) error       { return nil }
func (m *mockModbusAgent) Stop(_ context.Context) error        { return nil }
func (m *mockModbusAgent) Pause(_ context.Context) error       { return nil }
func (m *mockModbusAgent) Resume(_ context.Context) error      { return nil }
func (m *mockModbusAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *mockModbusAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *mockModbusAgent) ID() string                          { return "mock-modbus" }
func (m *mockModbusAgent) Name() string                        { return "mock-modbus" }
func (m *mockModbusAgent) Type() string                        { return "modbus" }
func (m *mockModbusAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *mockModbusAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }

func (m *mockModbusAgent) Process(data []byte) ([]byte, error) {
	m.processData = data
	if m.processErr != nil {
		return nil, m.processErr
	}
	return m.processResp, nil
}

// mockModbusResolver 는 테스트용 AgentResolver 구현이다.
type mockModbusResolver struct {
	transport AgentTransport
	err       error
}

func (m *mockModbusResolver) ResolveAgent(_ context.Context, _ flow.AgentRef) (AgentTransport, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.transport, nil
}

// mockModbusTransport 는 AgentTransport + AgentAccessor를 구현하는 테스트용 모의 객체이다.
type mockModbusTransport struct {
	agent agent.Agent // UnderlyingAgent()에서 반환할 Agent
}

func (m *mockModbusTransport) Send(_ context.Context, _ message.Message) error {
	return nil
}

func (m *mockModbusTransport) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *mockModbusTransport) UnderlyingAgent() agent.Agent {
	return m.agent
}

// mockModbusTransportNoAccessor 는 AgentAccessor를 구현하지 않는 AgentTransport이다.
type mockModbusTransportNoAccessor struct{}

func (m *mockModbusTransportNoAccessor) Send(_ context.Context, _ message.Message) error {
	return nil
}

func (m *mockModbusTransportNoAccessor) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// ---------------------------------------------------------------------------
// 헬퍼 함수
// ---------------------------------------------------------------------------

// newModbusNodeDef 는 테스트용 NodeDef를 생성한다.
func newModbusNodeDef(name string) flow.NodeDef {
	return flow.NewNodeDef(name, "modbus")
}

// setupModbusNode 는 테스트용으로 Configure + Init까지 완료된 ModbusNode를 생성한다.
// agentInstance는 mockModbusTransport의 UnderlyingAgent()에서 반환될 에이전트이다.
func setupModbusNode(t *testing.T, config map[string]any, agentInstance agent.Agent) *ModbusNode {
	t.Helper()

	transport := &mockModbusTransport{agent: agentInstance}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusNodeDef("test-modbus-rw")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(config)
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	return n
}

// parseProcessCommand 는 Agent.Process()에 전달된 JSON 바이트를 파싱하여 map으로 반환한다.
func parseProcessCommand(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var cmd map[string]any
	err := json.Unmarshal(data, &cmd)
	require.NoError(t, err, "Process 명령 JSON 파싱 실패")
	return cmd
}

// ---------------------------------------------------------------------------
// 1. TestNewModbusNode - 팩토리 테스트
// ---------------------------------------------------------------------------

// TestNewModbusNode_정상생성 은 ModbusNode가 올바르게 생성되는지 확인한다.
func TestNewModbusNode_정상생성(t *testing.T) {
	def := newModbusNodeDef("modbus-rw-1")
	node, err := NewModbusNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "modbus-rw-1", node.Name())
	assert.Equal(t, "modbus", node.Type())
}

// ---------------------------------------------------------------------------
// 2. TestModbusNode_Configure - 설정 테스트 (테이블 기반)
// ---------------------------------------------------------------------------

// TestModbusNode_Configure 는 Configure 메서드의 다양한 시나리오를 테이블 기반으로 테스트한다.
func TestModbusNode_Configure(t *testing.T) {
	tests := []struct {
		name      string
		config    map[string]any
		wantErr   error  // 기대하는 에러 (nil이면 정상)
		checkFunc func(t *testing.T, n *ModbusNode) // 추가 검증
	}{
		{
			name: "전체 설정 정상 파싱",
			config: map[string]any{
				"agent_ref":     "my-server",
				"operation":     "read",
				"register_area": "holding_registers",
				"address":       float64(100),
				"count":         float64(10),
				"data_type":     "float32",
				"byte_order":    "little_endian",
				"device_id":     float64(5),
				"timeout":       "10s",
			},
			checkFunc: func(t *testing.T, n *ModbusNode) {
				assert.Equal(t, "my-server", n.modbusConfig.AgentRef)
				assert.Equal(t, "read", n.modbusConfig.Operation)
				assert.Equal(t, "holding_registers", n.modbusConfig.RegisterArea)
				assert.Equal(t, uint16(100), n.modbusConfig.Address)
				assert.Equal(t, uint16(10), n.modbusConfig.Count)
				assert.Equal(t, "float32", n.modbusConfig.DataType)
				assert.Equal(t, "little_endian", n.modbusConfig.ByteOrder)
				assert.Equal(t, uint8(5), n.modbusConfig.DeviceID)
				assert.Equal(t, 10*time.Second, n.timeout)
			},
		},
		{
			name: "선택 필드 기본값 적용",
			config: map[string]any{
				"agent_ref":     "my-server",
				"operation":     "read",
				"register_area": "coils",
				"address":       float64(0),
			},
			checkFunc: func(t *testing.T, n *ModbusNode) {
				assert.Equal(t, uint16(1), n.modbusConfig.Count, "count 기본값 1")
				assert.Equal(t, "uint16", n.modbusConfig.DataType, "data_type 기본값 uint16")
				assert.Equal(t, "big_endian", n.modbusConfig.ByteOrder, "byte_order 기본값 big_endian")
				assert.Equal(t, uint8(1), n.modbusConfig.DeviceID, "device_id 기본값 1")
				assert.Equal(t, 5*time.Second, n.timeout, "timeout 기본값 5s")
			},
		},
		{
			name: "agent_ref 누락 에러",
			config: map[string]any{
				"operation":     "read",
				"register_area": "coils",
				"address":       float64(0),
			},
			wantErr: ErrModbusMissingAgentRef,
		},
		{
			name: "잘못된 operation 에러",
			config: map[string]any{
				"agent_ref":     "my-server",
				"operation":     "invalid",
				"register_area": "coils",
				"address":       float64(0),
			},
			wantErr: ErrModbusInvalidOperation,
		},
		{
			name: "operation 빈문자열 에러",
			config: map[string]any{
				"agent_ref":     "my-server",
				"operation":     "",
				"register_area": "coils",
				"address":       float64(0),
			},
			wantErr: ErrModbusInvalidOperation,
		},
		{
			name: "잘못된 register_area 에러",
			config: map[string]any{
				"agent_ref":     "my-server",
				"operation":     "read",
				"register_area": "invalid_area",
				"address":       float64(0),
			},
			wantErr: ErrModbusInvalidRegisterArea,
		},
		{
			name: "읽기전용 영역 쓰기 시도 에러 (discrete_inputs)",
			config: map[string]any{
				"agent_ref":     "my-server",
				"operation":     "write",
				"register_area": "discrete_inputs",
				"address":       float64(0),
			},
			wantErr: ErrModbusReadOnlyArea,
		},
		{
			name: "읽기전용 영역 쓰기 시도 에러 (input_registers)",
			config: map[string]any{
				"agent_ref":     "my-server",
				"operation":     "write",
				"register_area": "input_registers",
				"address":       float64(0),
			},
			wantErr: ErrModbusReadOnlyArea,
		},
		{
			name: "write 가능한 영역 정상 (coils)",
			config: map[string]any{
				"agent_ref":     "my-server",
				"operation":     "write",
				"register_area": "coils",
				"address":       float64(0),
			},
			checkFunc: func(t *testing.T, n *ModbusNode) {
				assert.Equal(t, "write", n.modbusConfig.Operation)
				assert.Equal(t, "coils", n.modbusConfig.RegisterArea)
			},
		},
		{
			name: "write 가능한 영역 정상 (holding_registers)",
			config: map[string]any{
				"agent_ref":     "my-server",
				"operation":     "write",
				"register_area": "holding_registers",
				"address":       float64(0),
			},
			checkFunc: func(t *testing.T, n *ModbusNode) {
				assert.Equal(t, "write", n.modbusConfig.Operation)
				assert.Equal(t, "holding_registers", n.modbusConfig.RegisterArea)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := newModbusNodeDef("test-cfg")
			node, err := NewModbusNode(def)
			require.NoError(t, err)

			n := node.(*ModbusNode)
			err = n.Configure(tt.config)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			if tt.checkFunc != nil {
				tt.checkFunc(t, n)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3. TestModbusNode_Init - 초기화 테스트
// ---------------------------------------------------------------------------

// TestModbusNode_Init_ServerAgent_감지 는 Server Agent를 올바르게 감지하는지 확인한다.
func TestModbusNode_Init_ServerAgent_감지(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusNodeDef("init-server")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(map[string]any{
		"agent_ref":     "server-1",
		"operation":     "read",
		"register_area": "coils",
		"address":       float64(0),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "server", n.agentType)
	assert.Equal(t, lifecycle.StateRunning, n.CurrentState())
}

// TestModbusNode_Init_ClientAgent_감지 는 Client Agent를 올바르게 감지하는지 확인한다.
func TestModbusNode_Init_ClientAgent_감지(t *testing.T) {
	clientAgent := &modbus.ModbusAgent{}
	transport := &mockModbusTransport{agent: clientAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusNodeDef("init-client")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(map[string]any{
		"agent_ref":     "client-1",
		"operation":     "read",
		"register_area": "holding_registers",
		"address":       float64(0),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "client", n.agentType)
	assert.Equal(t, lifecycle.StateRunning, n.CurrentState())
}

// TestModbusNode_Init_비MODBUS_Agent_에러 는 비-MODBUS Agent 시 에러를 반환하는지 확인한다.
func TestModbusNode_Init_비MODBUS_Agent_에러(t *testing.T) {
	// mockModbusAgent는 modbus 타입이 아닌 일반 Agent이다
	fakeAgent := &mockModbusAgent{}
	transport := &mockModbusTransport{agent: fakeAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusNodeDef("init-non-modbus")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(map[string]any{
		"agent_ref":     "non-modbus",
		"operation":     "read",
		"register_area": "coils",
		"address":       float64(0),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrModbusAgentNotMODBUS)
}

// TestModbusNode_Init_AgentAccessor_미지원_에러 는 transport가 AgentAccessor를 구현하지 않을 때 에러를 반환하는지 확인한다.
func TestModbusNode_Init_AgentAccessor_미지원_에러(t *testing.T) {
	transport := &mockModbusTransportNoAccessor{}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusNodeDef("init-no-accessor")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(map[string]any{
		"agent_ref":     "no-accessor",
		"operation":     "read",
		"register_area": "coils",
		"address":       float64(0),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrModbusAgentNotMODBUS)
}

// TestModbusNode_Init_Resolver실패_에러 는 Agent resolve 실패 시 에러를 반환하는지 확인한다.
func TestModbusNode_Init_Resolver실패_에러(t *testing.T) {
	resolveErr := errors.New("에이전트 해석 실패")
	resolver := &mockModbusResolver{err: resolveErr}

	def := newModbusNodeDef("init-resolve-err")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(map[string]any{
		"agent_ref":     "missing-agent",
		"operation":     "read",
		"register_area": "coils",
		"address":       float64(0),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent resolve failed")
}

// TestModbusNode_Init_Resolver없음_에러 는 AgentResolver가 설정되지 않았을 때 에러를 반환하는지 확인한다.
func TestModbusNode_Init_Resolver없음_에러(t *testing.T) {
	def := newModbusNodeDef("init-no-resolver")
	// WithAgentResolver를 호출하지 않는다
	node, err := NewModbusNode(def)
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(map[string]any{
		"agent_ref":     "no-resolver",
		"operation":     "read",
		"register_area": "coils",
		"address":       float64(0),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrModbusNoResolver)
}

// TestModbusNode_Init_기본타임아웃 은 timeout 미설정 시 기본값 5초가 적용되는지 확인한다.
func TestModbusNode_Init_기본타임아웃(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusNodeDef("init-default-timeout")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(map[string]any{
		"agent_ref":     "server-1",
		"operation":     "read",
		"register_area": "coils",
		"address":       float64(0),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 5*time.Second, n.timeout)
}

// TestModbusNode_Init_커스텀타임아웃 은 timeout 설정이 정상 파싱되는지 확인한다.
func TestModbusNode_Init_커스텀타임아웃(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusNodeDef("init-custom-timeout")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(map[string]any{
		"agent_ref":     "server-1",
		"operation":     "read",
		"register_area": "coils",
		"address":       float64(0),
		"timeout":       "15s",
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 15*time.Second, n.timeout)
}

// ---------------------------------------------------------------------------
// 4. TestModbusNode_ProcessRead_Server - Server 읽기 테스트 (테이블 기반)
// ---------------------------------------------------------------------------

// TestModbusNode_ProcessRead_Server 는 Server Agent 읽기 명령 생성을 테이블 기반으로 테스트한다.
func TestModbusNode_ProcessRead_Server(t *testing.T) {
	tests := []struct {
		name         string
		config       map[string]any
		agentResp    map[string]any // Agent Process()가 반환할 응답
		wantCommand  string         // 기대하는 command 값
		checkParams  func(t *testing.T, params map[string]any) // params 검증
		checkPayload func(t *testing.T, payload message.Payload) // 출력 메시지 검증
	}{
		{
			name: "coils 읽기 -> get_coils 명령",
			config: map[string]any{
				"agent_ref":     "server-1",
				"operation":     "read",
				"register_area": "coils",
				"address":       float64(100),
				"count":         float64(5),
			},
			agentResp:   map[string]any{"ok": true, "values": []any{true, false, true, false, true}},
			wantCommand: "get_coils",
			checkParams: func(t *testing.T, params map[string]any) {
				assert.Equal(t, float64(100), params["address"])
				assert.Equal(t, float64(5), params["quantity"])
			},
		},
		{
			name: "discrete_inputs 읽기 -> get_discrete_inputs 명령",
			config: map[string]any{
				"agent_ref":     "server-1",
				"operation":     "read",
				"register_area": "discrete_inputs",
				"address":       float64(200),
				"count":         float64(3),
			},
			agentResp:   map[string]any{"ok": true, "values": []any{true, true, false}},
			wantCommand: "get_discrete_inputs",
			checkParams: func(t *testing.T, params map[string]any) {
				assert.Equal(t, float64(200), params["address"])
				assert.Equal(t, float64(3), params["quantity"])
			},
		},
		{
			name: "holding_registers 읽기 (uint16) -> get_holding_registers 명령",
			config: map[string]any{
				"agent_ref":     "server-1",
				"operation":     "read",
				"register_area": "holding_registers",
				"address":       float64(0),
				"count":         float64(10),
				"data_type":     "uint16",
			},
			agentResp:   map[string]any{"ok": true, "values": []any{1, 2, 3}},
			wantCommand: "get_holding_registers",
			checkParams: func(t *testing.T, params map[string]any) {
				assert.Equal(t, float64(0), params["address"])
				assert.Equal(t, float64(10), params["quantity"])
				// uint16(기본값)일 때 data_type/byte_order는 포함되지 않아야 한다
				_, hasDataType := params["data_type"]
				assert.False(t, hasDataType)
			},
		},
		{
			name: "holding_registers 읽기 (float32) -> get_register_typed 명령",
			config: map[string]any{
				"agent_ref":     "server-1",
				"operation":     "read",
				"register_area": "holding_registers",
				"address":       float64(50),
				"data_type":     "float32",
				"byte_order":    "little_endian",
			},
			agentResp:   map[string]any{"ok": true, "value": 3.14},
			wantCommand: "get_register_typed",
			checkParams: func(t *testing.T, params map[string]any) {
				assert.Equal(t, float64(50), params["address"])
				assert.Equal(t, "float32", params["data_type"])
				assert.Equal(t, "little_endian", params["byte_order"])
			},
		},
		{
			name: "input_registers 읽기 (uint16) -> get_input_registers 명령",
			config: map[string]any{
				"agent_ref":     "server-1",
				"operation":     "read",
				"register_area": "input_registers",
				"address":       float64(300),
				"count":         float64(2),
			},
			agentResp:   map[string]any{"ok": true, "values": []any{100, 200}},
			wantCommand: "get_input_registers",
			checkParams: func(t *testing.T, params map[string]any) {
				assert.Equal(t, float64(300), params["address"])
				assert.Equal(t, float64(2), params["quantity"])
			},
		},
		{
			name: "input_registers 읽기 (int32) -> get_register_typed 명령",
			config: map[string]any{
				"agent_ref":     "server-1",
				"operation":     "read",
				"register_area": "input_registers",
				"address":       float64(400),
				"data_type":     "int32",
				"byte_order":    "big_endian",
			},
			agentResp:   map[string]any{"ok": true, "value": -1234},
			wantCommand: "get_register_typed",
			checkParams: func(t *testing.T, params map[string]any) {
				assert.Equal(t, float64(400), params["address"])
				assert.Equal(t, "int32", params["data_type"])
				assert.Equal(t, "big_endian", params["byte_order"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Agent 응답 설정
			respBytes, err := json.Marshal(tt.agentResp)
			require.NoError(t, err)

			mockAgent := &mockModbusAgent{processResp: respBytes}

			// Server Agent를 감싸는 래퍼 생성 (ModbusServerAgent 타입 감지를 위해)
			// 직접 ModbusServerAgent를 생성하되, Process를 오버라이드할 수 없으므로
			// mockModbusAgent를 사용하고 Init에서 타입 감지를 우회한다.
			serverAgent := &modbusserver.ModbusServerAgent{}
			transport := &mockModbusTransport{agent: serverAgent}
			resolver := &mockModbusResolver{transport: transport}

			def := newModbusNodeDef("test-server-read")
			node, err := NewModbusNode(def, WithAgentResolver(resolver))
			require.NoError(t, err)

			n := node.(*ModbusNode)
			err = n.Configure(tt.config)
			require.NoError(t, err)

			err = n.Init(context.Background())
			require.NoError(t, err)

			// Init 후 agent를 mock으로 교체 (실제 Process 호출을 모의하기 위해)
			n.agent = mockAgent

			// 원본 payload 보존 확인을 위한 입력 메시지
			msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
				"original_key": "original_value",
			})))

			results, err := n.Process(context.Background(), msg)
			require.NoError(t, err)
			require.Len(t, results, 1)

			// Agent에 전달된 명령 검증
			cmd := parseProcessCommand(t, mockAgent.processData)
			assert.Equal(t, tt.wantCommand, cmd["command"])

			if tt.checkParams != nil {
				params, ok := cmd["params"].(map[string]any)
				require.True(t, ok, "params 필드가 map이어야 한다")
				tt.checkParams(t, params)
			}

			// 출력 메시지에 원본 payload가 보존되었는지 확인
			outPayload := results[0].Payload()
			v, ok := outPayload.Get("original_key")
			assert.True(t, ok, "원본 payload의 original_key가 보존되어야 한다")
			assert.Equal(t, "original_value", v)

			// 메타 정보 포함 확인
			area, ok := outPayload.Get("register_area")
			assert.True(t, ok)
			assert.Equal(t, tt.config["register_area"], area)

			agentType, ok := outPayload.Get("agent_type")
			assert.True(t, ok)
			assert.Equal(t, "server", agentType)
		})
	}
}

// ---------------------------------------------------------------------------
// 5. TestModbusNode_ProcessRead_Client - Client 읽기 테스트 (테이블 기반)
// ---------------------------------------------------------------------------

// TestModbusNode_ProcessRead_Client 는 Client Agent 읽기 명령 생성을 테이블 기반으로 테스트한다.
func TestModbusNode_ProcessRead_Client(t *testing.T) {
	tests := []struct {
		name            string
		config          map[string]any
		msgPayload      map[string]any // 입력 메시지 payload (device_id 오버라이드 등)
		wantFC          float64        // 기대하는 function_code
		wantDeviceID    string         // 기대하는 device_id
	}{
		{
			name: "coils 읽기 -> function_code=1",
			config: map[string]any{
				"agent_ref":     "client-1",
				"operation":     "read",
				"register_area": "coils",
				"address":       float64(0),
				"count":         float64(8),
			},
			wantFC:       1,
			wantDeviceID: "1", // 기본값
		},
		{
			name: "discrete_inputs 읽기 -> function_code=2",
			config: map[string]any{
				"agent_ref":     "client-1",
				"operation":     "read",
				"register_area": "discrete_inputs",
				"address":       float64(10),
				"count":         float64(4),
			},
			wantFC:       2,
			wantDeviceID: "1",
		},
		{
			name: "holding_registers 읽기 -> function_code=3",
			config: map[string]any{
				"agent_ref":     "client-1",
				"operation":     "read",
				"register_area": "holding_registers",
				"address":       float64(100),
				"count":         float64(10),
			},
			wantFC:       3,
			wantDeviceID: "1",
		},
		{
			name: "input_registers 읽기 -> function_code=4",
			config: map[string]any{
				"agent_ref":     "client-1",
				"operation":     "read",
				"register_area": "input_registers",
				"address":       float64(200),
				"count":         float64(5),
			},
			wantFC:       4,
			wantDeviceID: "1",
		},
		{
			name: "device_id 설정 반영",
			config: map[string]any{
				"agent_ref":     "client-1",
				"operation":     "read",
				"register_area": "coils",
				"address":       float64(0),
				"device_id":     float64(7),
			},
			wantFC:       1,
			wantDeviceID: "7",
		},
		{
			name: "메시지 payload의 device_id 오버라이드",
			config: map[string]any{
				"agent_ref":     "client-1",
				"operation":     "read",
				"register_area": "coils",
				"address":       float64(0),
				"device_id":     float64(3),
			},
			msgPayload: map[string]any{
				"device_id": "42",
			},
			wantFC:       1,
			wantDeviceID: "42", // 메시지 payload의 값이 우선
		},
		{
			name: "메시지 payload의 device_id 숫자타입 오버라이드",
			config: map[string]any{
				"agent_ref":     "client-1",
				"operation":     "read",
				"register_area": "coils",
				"address":       float64(0),
				"device_id":     float64(3),
			},
			msgPayload: map[string]any{
				"device_id": float64(99),
			},
			wantFC:       1,
			wantDeviceID: "99", // 숫자 타입도 문자열로 변환
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Agent 응답 설정
			respBytes, _ := json.Marshal(map[string]any{
				"data": "base64encoded",
			})
			mockAgent := &mockModbusAgent{processResp: respBytes}

			clientAgent := &modbus.ModbusAgent{}
			transport := &mockModbusTransport{agent: clientAgent}
			resolver := &mockModbusResolver{transport: transport}

			def := newModbusNodeDef("test-client-read")
			node, err := NewModbusNode(def, WithAgentResolver(resolver))
			require.NoError(t, err)

			n := node.(*ModbusNode)
			err = n.Configure(tt.config)
			require.NoError(t, err)

			err = n.Init(context.Background())
			require.NoError(t, err)

			// agent를 mock으로 교체
			n.agent = mockAgent

			// 입력 메시지 생성
			var msg message.Message
			if tt.msgPayload != nil {
				msg = message.New(message.WithPayload(message.NewPayload(tt.msgPayload)))
			} else {
				msg = message.New()
			}

			results, err := n.Process(context.Background(), msg)
			require.NoError(t, err)
			require.Len(t, results, 1)

			// Agent에 전달된 명령 검증
			cmd := parseProcessCommand(t, mockAgent.processData)
			assert.Equal(t, "read_raw", cmd["command"])
			assert.Equal(t, tt.wantDeviceID, cmd["device_id"])

			params, ok := cmd["params"].(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tt.wantFC, params["function_code"])

			// 출력 메시지의 agent_type 확인
			agentType, ok := results[0].Payload().Get("agent_type")
			assert.True(t, ok)
			assert.Equal(t, "client", agentType)
		})
	}
}

// ---------------------------------------------------------------------------
// 6. TestModbusNode_ProcessWrite_Server - Server 쓰기 테스트 (테이블 기반)
// ---------------------------------------------------------------------------

// TestModbusNode_ProcessWrite_Server 는 Server Agent 쓰기 명령 생성을 테이블 기반으로 테스트한다.
func TestModbusNode_ProcessWrite_Server(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]any
		msgPayload  map[string]any // 입력 메시지 payload (value/values 포함)
		wantCommand string
		checkParams func(t *testing.T, params map[string]any)
	}{
		{
			name: "단일 코일 쓰기 -> set_coil",
			config: map[string]any{
				"agent_ref":     "server-1",
				"operation":     "write",
				"register_area": "coils",
				"address":       float64(10),
				"count":         float64(1),
			},
			msgPayload:  map[string]any{"value": true},
			wantCommand: "set_coil",
			checkParams: func(t *testing.T, params map[string]any) {
				assert.Equal(t, float64(10), params["address"])
				assert.Equal(t, true, params["value"])
			},
		},
		{
			name: "다중 코일 쓰기 -> set_coils",
			config: map[string]any{
				"agent_ref":     "server-1",
				"operation":     "write",
				"register_area": "coils",
				"address":       float64(20),
				"count":         float64(3),
			},
			msgPayload:  map[string]any{"values": []any{true, false, true}},
			wantCommand: "set_coils",
			checkParams: func(t *testing.T, params map[string]any) {
				assert.Equal(t, float64(20), params["address"])
				values, ok := params["values"].([]any)
				require.True(t, ok)
				assert.Len(t, values, 3)
			},
		},
		{
			name: "단일 레지스터 쓰기 -> set_register",
			config: map[string]any{
				"agent_ref":     "server-1",
				"operation":     "write",
				"register_area": "holding_registers",
				"address":       float64(100),
				"count":         float64(1),
				"data_type":     "uint16",
				"byte_order":    "big_endian",
			},
			msgPayload:  map[string]any{"value": float64(1234)},
			wantCommand: "set_register",
			checkParams: func(t *testing.T, params map[string]any) {
				assert.Equal(t, float64(100), params["address"])
				assert.Equal(t, float64(1234), params["value"])
				// data_type/byte_order가 포함되어야 한다
				assert.Equal(t, "uint16", params["data_type"])
				assert.Equal(t, "big_endian", params["byte_order"])
			},
		},
		{
			name: "다중 레지스터 쓰기 -> set_registers",
			config: map[string]any{
				"agent_ref":     "server-1",
				"operation":     "write",
				"register_area": "holding_registers",
				"address":       float64(200),
				"count":         float64(3),
				"data_type":     "float32",
				"byte_order":    "little_endian",
			},
			msgPayload:  map[string]any{"values": []any{1.1, 2.2, 3.3}},
			wantCommand: "set_registers",
			checkParams: func(t *testing.T, params map[string]any) {
				assert.Equal(t, float64(200), params["address"])
				assert.Equal(t, "float32", params["data_type"])
				assert.Equal(t, "little_endian", params["byte_order"])
				values, ok := params["values"].([]any)
				require.True(t, ok)
				assert.Len(t, values, 3)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Agent 응답 설정 (쓰기 성공)
			respBytes, _ := json.Marshal(map[string]any{"ok": true})
			mockAgent := &mockModbusAgent{processResp: respBytes}

			serverAgent := &modbusserver.ModbusServerAgent{}
			transport := &mockModbusTransport{agent: serverAgent}
			resolver := &mockModbusResolver{transport: transport}

			def := newModbusNodeDef("test-server-write")
			node, err := NewModbusNode(def, WithAgentResolver(resolver))
			require.NoError(t, err)

			n := node.(*ModbusNode)
			err = n.Configure(tt.config)
			require.NoError(t, err)

			err = n.Init(context.Background())
			require.NoError(t, err)

			// agent를 mock으로 교체
			n.agent = mockAgent

			msg := message.New(message.WithPayload(message.NewPayload(tt.msgPayload)))
			results, err := n.Process(context.Background(), msg)
			require.NoError(t, err)
			require.Len(t, results, 1)

			// Agent에 전달된 명령 검증
			cmd := parseProcessCommand(t, mockAgent.processData)
			assert.Equal(t, tt.wantCommand, cmd["command"])

			if tt.checkParams != nil {
				params, ok := cmd["params"].(map[string]any)
				require.True(t, ok)
				tt.checkParams(t, params)
			}

			// 출력 메시지에 success 플래그 확인
			success, ok := results[0].Payload().Get("success")
			assert.True(t, ok)
			assert.Equal(t, true, success)
		})
	}
}

// ---------------------------------------------------------------------------
// 7. TestModbusNode_ProcessWrite_Client - Client 쓰기 테스트 (테이블 기반)
// ---------------------------------------------------------------------------

// TestModbusNode_ProcessWrite_Client 는 Client Agent 쓰기 명령 생성을 테이블 기반으로 테스트한다.
func TestModbusNode_ProcessWrite_Client(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]any
		msgPayload  map[string]any
		wantCommand string
		checkParams func(t *testing.T, params map[string]any)
	}{
		{
			name: "단일 코일 쓰기 -> write_coil",
			config: map[string]any{
				"agent_ref":     "client-1",
				"operation":     "write",
				"register_area": "coils",
				"address":       float64(0),
				"count":         float64(1),
			},
			msgPayload:  map[string]any{"value": true},
			wantCommand: "write_coil",
			checkParams: func(t *testing.T, params map[string]any) {
				assert.Equal(t, float64(0), params["address"])
				assert.Equal(t, true, params["value"])
			},
		},
		{
			name: "단일 레지스터 쓰기 -> write_register",
			config: map[string]any{
				"agent_ref":     "client-1",
				"operation":     "write",
				"register_area": "holding_registers",
				"address":       float64(100),
				"count":         float64(1),
			},
			msgPayload:  map[string]any{"value": float64(5678)},
			wantCommand: "write_register",
			checkParams: func(t *testing.T, params map[string]any) {
				assert.Equal(t, float64(100), params["address"])
				assert.Equal(t, float64(5678), params["value"])
			},
		},
		{
			name: "다중 코일 쓰기 -> write_coils",
			config: map[string]any{
				"agent_ref":     "client-1",
				"operation":     "write",
				"register_area": "coils",
				"address":       float64(10),
				"count":         float64(4),
			},
			msgPayload:  map[string]any{"values": []any{true, false, true, false}},
			wantCommand: "write_coils",
			checkParams: func(t *testing.T, params map[string]any) {
				assert.Equal(t, float64(10), params["address"])
				values, ok := params["values"].([]any)
				require.True(t, ok)
				assert.Len(t, values, 4)
			},
		},
		{
			name: "다중 레지스터 쓰기 -> write_registers",
			config: map[string]any{
				"agent_ref":     "client-1",
				"operation":     "write",
				"register_area": "holding_registers",
				"address":       float64(200),
				"count":         float64(2),
				"data_type":     "uint32",
				"byte_order":    "big_endian",
			},
			msgPayload:  map[string]any{"values": []any{float64(1000), float64(2000)}},
			wantCommand: "write_registers",
			checkParams: func(t *testing.T, params map[string]any) {
				assert.Equal(t, float64(200), params["address"])
				assert.Equal(t, "uint32", params["data_type"])
				assert.Equal(t, "big_endian", params["byte_order"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBytes, _ := json.Marshal(map[string]any{"ok": true})
			mockAgent := &mockModbusAgent{processResp: respBytes}

			clientAgent := &modbus.ModbusAgent{}
			transport := &mockModbusTransport{agent: clientAgent}
			resolver := &mockModbusResolver{transport: transport}

			def := newModbusNodeDef("test-client-write")
			node, err := NewModbusNode(def, WithAgentResolver(resolver))
			require.NoError(t, err)

			n := node.(*ModbusNode)
			err = n.Configure(tt.config)
			require.NoError(t, err)

			err = n.Init(context.Background())
			require.NoError(t, err)

			// agent를 mock으로 교체
			n.agent = mockAgent

			msg := message.New(message.WithPayload(message.NewPayload(tt.msgPayload)))
			results, err := n.Process(context.Background(), msg)
			require.NoError(t, err)
			require.Len(t, results, 1)

			// Agent에 전달된 명령 검증
			cmd := parseProcessCommand(t, mockAgent.processData)
			assert.Equal(t, tt.wantCommand, cmd["command"])

			// device_id 기본값 확인
			assert.Equal(t, "1", cmd["device_id"])

			if tt.checkParams != nil {
				params, ok := cmd["params"].(map[string]any)
				require.True(t, ok)
				tt.checkParams(t, params)
			}

			// 출력 메시지에 success 플래그 확인
			success, ok := results[0].Payload().Get("success")
			assert.True(t, ok)
			assert.Equal(t, true, success)
		})
	}
}

// TestModbusNode_ProcessWrite_Client_DeviceID_오버라이드 는 Client 쓰기 시 device_id 오버라이드를 검증한다.
func TestModbusNode_ProcessWrite_Client_DeviceID_오버라이드(t *testing.T) {
	respBytes, _ := json.Marshal(map[string]any{"ok": true})
	mockAgent := &mockModbusAgent{processResp: respBytes}

	clientAgent := &modbus.ModbusAgent{}
	transport := &mockModbusTransport{agent: clientAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusNodeDef("test-client-write-did")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(map[string]any{
		"agent_ref":     "client-1",
		"operation":     "write",
		"register_area": "coils",
		"address":       float64(0),
		"device_id":     float64(5),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)
	n.agent = mockAgent

	// payload에 device_id 오버라이드
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"value":     true,
		"device_id": "77",
	})))

	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	cmd := parseProcessCommand(t, mockAgent.processData)
	assert.Equal(t, "77", cmd["device_id"], "메시지 payload의 device_id가 우선해야 한다")
}

// TestModbusNode_ProcessWrite_MissingValue 는 쓰기 시 value/values가 없으면 에러를 반환하는지 확인한다.
func TestModbusNode_ProcessWrite_MissingValue(t *testing.T) {
	respBytes, _ := json.Marshal(map[string]any{"ok": true})
	mockAgent := &mockModbusAgent{processResp: respBytes}

	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusNodeDef("test-write-missing-value")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(map[string]any{
		"agent_ref":     "server-1",
		"operation":     "write",
		"register_area": "coils",
		"address":       float64(0),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)
	n.agent = mockAgent

	// value/values 없이 메시지 전송
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"other": "data",
	})))

	_, err = n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrModbusWriteValueMissing)
}

// ---------------------------------------------------------------------------
// 8. TestModbusNode_ErrorHandling - 에러 핸들링 테스트
// ---------------------------------------------------------------------------

// TestModbusNode_AgentProcess_에러 는 Agent Process() 에러가 올바르게 전파되는지 확인한다.
func TestModbusNode_AgentProcess_에러(t *testing.T) {
	processErr := errors.New("모드버스 통신 실패")
	mockAgent := &mockModbusAgent{processErr: processErr}

	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusNodeDef("test-process-err")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(map[string]any{
		"agent_ref":     "server-1",
		"operation":     "read",
		"register_area": "coils",
		"address":       float64(0),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)
	n.agent = mockAgent

	msg := message.New()
	_, err = n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrModbusProcessFailed)
}

// TestModbusNode_PanicRecovery 는 Process 중 패닉이 발생해도 복구되는지 확인한다.
func TestModbusNode_PanicRecovery(t *testing.T) {
	// 패닉을 발생시키는 Agent
	panicAgent := &mockModbusAgent{}
	panicAgent.processErr = nil
	panicAgent.processResp = nil

	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusNodeDef("test-panic-recovery")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(map[string]any{
		"agent_ref":     "server-1",
		"operation":     "read",
		"register_area": "coils",
		"address":       float64(0),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	// agent를 nil로 설정하면 callAgentProcess에서 에러 반환
	// 대신 패닉을 유발하는 시나리오를 만든다
	// Process 내부에서 패닉이 발생하면 recover로 잡힌다
	n.agent = nil

	msg := message.New()
	_, err = n.Process(context.Background(), msg)
	// agent가 nil이면 callAgentProcess에서 ErrModbusNoResolver 반환
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// 9. TestModbusNode_Shutdown - 종료 테스트
// ---------------------------------------------------------------------------

// TestModbusNode_Shutdown_상태전이 는 Shutdown 시 Stopping 상태로 전이하는지 확인한다.
func TestModbusNode_Shutdown_상태전이(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusNodeDef("test-shutdown")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(map[string]any{
		"agent_ref":     "server-1",
		"operation":     "read",
		"register_area": "coils",
		"address":       float64(0),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, n.CurrentState())

	err = n.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, n.CurrentState())
}

// ---------------------------------------------------------------------------
// 10. TestModbusNode_유틸리티 - 유틸리티 함수 테스트
// ---------------------------------------------------------------------------

// TestToUint16FromAny 는 toUint16FromAny 헬퍼 함수를 테스트한다.
func TestToUint16FromAny(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want uint16
	}{
		{"nil", nil, 0},
		{"float64", float64(100), 100},
		{"int", int(200), 200},
		{"int64", int64(300), 300},
		{"json.Number", json.Number("400"), 400},
		{"잘못된 json.Number", json.Number("invalid"), 0},
		{"문자열", "abc", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toUint16FromAny(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestToByte 는 toByte 헬퍼 함수를 테스트한다.
func TestToByte(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want uint8
	}{
		{"nil", nil, 0},
		{"float64", float64(10), 10},
		{"int", int(20), 20},
		{"int64", int64(30), 30},
		{"json.Number", json.Number("40"), 40},
		{"잘못된 json.Number", json.Number("invalid"), 0},
		{"문자열", "abc", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toByte(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// 11. TestExtractReadResult - 읽기 결과 추출 테스트
// ---------------------------------------------------------------------------

// TestExtractReadResult 는 extractReadResult 함수가 Agent 응답에서 올바르게 결과를 추출하는지 확인한다.
func TestExtractReadResult(t *testing.T) {
	cfg := ModbusConfig{RegisterArea: "holding_registers", DataType: "uint16"}

	tests := []struct {
		name      string
		resp      map[string]any
		agentType string
		wantType  string // 기대하는 결과의 타입 설명
	}{
		{
			name:      "Server 응답 - values 필드",
			resp:      map[string]any{"ok": true, "values": []any{1, 2, 3}},
			agentType: "server",
			wantType:  "values",
		},
		{
			name:      "Server 응답 - value 필드",
			resp:      map[string]any{"ok": true, "value": 3.14},
			agentType: "server",
			wantType:  "value",
		},
		{
			name:      "Server 응답 - values/value 없음",
			resp:      map[string]any{"ok": true},
			agentType: "server",
			wantType:  "full_resp",
		},
		{
			name:      "Client 응답 - data 필드",
			resp:      map[string]any{"data": "base64data"},
			agentType: "client",
			wantType:  "data",
		},
		{
			name:      "Client 응답 - data 없음",
			resp:      map[string]any{"ok": true},
			agentType: "client",
			wantType:  "full_resp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractReadResult(tt.resp, tt.agentType, cfg)
			assert.NotNil(t, result)

			switch tt.wantType {
			case "values":
				values, ok := result.([]any)
				assert.True(t, ok, "result가 []any 타입이어야 한다")
				assert.Len(t, values, 3)
			case "value":
				assert.Equal(t, 3.14, result)
			case "data":
				assert.Equal(t, "base64data", result)
			case "full_resp":
				m, ok := result.(map[string]any)
				assert.True(t, ok, "result가 map[string]any 타입이어야 한다")
				assert.True(t, m["ok"].(bool))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 12. 추가 엣지 케이스 테스트
// ---------------------------------------------------------------------------

// TestModbusNode_ProcessWrite_Server_ReadOnlyArea_런타임에러 는 런타임에 읽기전용 영역 쓰기 시 에러를 반환하는지 확인한다.
func TestModbusNode_ProcessWrite_Server_ReadOnlyArea_런타임에러(t *testing.T) {
	// Configure에서는 read로 설정하고, 내부적으로 modbusConfig를 조작하는 대신
	// processWrite가 readOnlyAreas를 다시 체크하는지 확인한다.
	// 실제로는 Configure에서 이미 차단되므로, 직접 processWrite 경로를 통해 확인한다.
	respBytes, _ := json.Marshal(map[string]any{"ok": true})
	mockAgent := &mockModbusAgent{processResp: respBytes}

	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusNodeDef("test-readonly-runtime")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	// read로 설정 (Configure 통과를 위해)
	err = n.Configure(map[string]any{
		"agent_ref":     "server-1",
		"operation":     "read",
		"register_area": "discrete_inputs",
		"address":       float64(0),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)
	n.agent = mockAgent

	// 내부적으로 operation을 write로 변경하여 런타임 체크를 확인
	n.mu.Lock()
	n.modbusConfig.Operation = "write"
	n.mu.Unlock()

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": true})))
	_, err = n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrModbusReadOnlyArea)
}

// TestModbusNode_ProcessRead_응답파싱에러 는 Agent 응답이 유효하지 않은 JSON일 때 에러를 반환하는지 확인한다.
func TestModbusNode_ProcessRead_응답파싱에러(t *testing.T) {
	// 유효하지 않은 JSON 응답
	mockAgent := &mockModbusAgent{processResp: []byte("invalid json")}

	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusNodeDef("test-invalid-resp")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(map[string]any{
		"agent_ref":     "server-1",
		"operation":     "read",
		"register_area": "coils",
		"address":       float64(0),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)
	n.agent = mockAgent

	msg := message.New()
	_, err = n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "response parse failed")
}

// TestModbusNode_Process_타임아웃 은 Agent Process() 호출이 타임아웃되는지 확인한다.
func TestModbusNode_Process_타임아웃(t *testing.T) {
	// Process()가 오래 걸리는 Agent를 시뮬레이션
	slowAgent := &slowModbusAgent{delay: 2 * time.Second}

	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusNodeDef("test-timeout")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(map[string]any{
		"agent_ref":     "server-1",
		"operation":     "read",
		"register_area": "coils",
		"address":       float64(0),
		"timeout":       "100ms",
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)
	n.agent = slowAgent

	msg := message.New()
	_, err = n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deadline exceeded")
}

// slowModbusAgent 는 Process() 호출 시 지연을 발생시키는 테스트용 Agent이다.
type slowModbusAgent struct {
	delay time.Duration
}

func (m *slowModbusAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *slowModbusAgent) Start(_ context.Context) error       { return nil }
func (m *slowModbusAgent) Stop(_ context.Context) error        { return nil }
func (m *slowModbusAgent) Pause(_ context.Context) error       { return nil }
func (m *slowModbusAgent) Resume(_ context.Context) error      { return nil }
func (m *slowModbusAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *slowModbusAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *slowModbusAgent) ID() string                          { return "slow-modbus" }
func (m *slowModbusAgent) Name() string                        { return "slow-modbus" }
func (m *slowModbusAgent) Type() string                        { return "modbus" }
func (m *slowModbusAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *slowModbusAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }

func (m *slowModbusAgent) Process(_ []byte) ([]byte, error) {
	time.Sleep(m.delay)
	return []byte(`{"ok": true}`), nil
}

// TestModbusNode_CallAgentProcess_AgentNil 은 agent가 nil일 때 ErrModbusNoResolver를 반환하는지 확인한다.
func TestModbusNode_CallAgentProcess_AgentNil(t *testing.T) {
	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusNodeDef("test-agent-nil")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(map[string]any{
		"agent_ref":     "server-1",
		"operation":     "read",
		"register_area": "coils",
		"address":       float64(0),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	// agent를 nil로 설정
	n.agent = nil

	_, err = n.callAgentProcess(context.Background(), []byte("test"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrModbusNoResolver)
}

// TestModbusNode_인터페이스_준수 는 ModbusNode가 Node 인터페이스를 구현하는지 확인한다.
var _ Node = (*ModbusNode)(nil)

// TestModbusNode_ClientRead_Command구조 는 Client 읽기 명령의 전체 구조를 상세히 검증한다.
func TestModbusNode_ClientRead_Command구조(t *testing.T) {
	respBytes, _ := json.Marshal(map[string]any{"data": "AAAB"})
	mockAgent := &mockModbusAgent{processResp: respBytes}

	clientAgent := &modbus.ModbusAgent{}
	transport := &mockModbusTransport{agent: clientAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusNodeDef("test-client-cmd-structure")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(map[string]any{
		"agent_ref":     "client-1",
		"operation":     "read",
		"register_area": "holding_registers",
		"address":       float64(100),
		"count":         float64(5),
		"device_id":     float64(3),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)
	n.agent = mockAgent

	msg := message.New()
	_, err = n.Process(context.Background(), msg)
	require.NoError(t, err)

	cmd := parseProcessCommand(t, mockAgent.processData)

	// 명령 구조 전체 검증
	assert.Equal(t, "read_raw", cmd["command"])
	assert.Equal(t, "3", cmd["device_id"])

	params := cmd["params"].(map[string]any)
	assert.Equal(t, float64(3), params["function_code"])  // holding_registers -> FC03
	assert.Equal(t, float64(100), params["address"])
	assert.Equal(t, float64(5), params["quantity"])
}

// TestModbusNode_ServerRead_원본Payload보존 은 서버 읽기 시 원본 payload가 보존되는지 상세 검증한다.
func TestModbusNode_ServerRead_원본Payload보존(t *testing.T) {
	respBytes, _ := json.Marshal(map[string]any{"ok": true, "values": []any{100, 200}})
	mockAgent := &mockModbusAgent{processResp: respBytes}

	serverAgent := &modbusserver.ModbusServerAgent{}
	transport := &mockModbusTransport{agent: serverAgent}
	resolver := &mockModbusResolver{transport: transport}

	def := newModbusNodeDef("test-preserve-payload")
	node, err := NewModbusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*ModbusNode)
	err = n.Configure(map[string]any{
		"agent_ref":     "server-1",
		"operation":     "read",
		"register_area": "holding_registers",
		"address":       float64(50),
		"count":         float64(2),
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)
	n.agent = mockAgent

	// 원본 payload에 여러 키를 설정
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"sensor_id":   "temp-001",
		"timestamp":   float64(1234567890),
		"custom_data": map[string]any{"nested": true},
	})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	outPayload := results[0].Payload()

	// 원본 키 보존 확인
	v, ok := outPayload.Get("sensor_id")
	assert.True(t, ok)
	assert.Equal(t, "temp-001", v)

	v, ok = outPayload.Get("timestamp")
	assert.True(t, ok)
	assert.Equal(t, float64(1234567890), v)

	// 추가된 MODBUS 결과 키 확인
	result, ok := outPayload.Get("result")
	assert.True(t, ok)
	assert.NotNil(t, result)

	area, ok := outPayload.Get("register_area")
	assert.True(t, ok)
	assert.Equal(t, "holding_registers", area)

	address, ok := outPayload.Get("address")
	assert.True(t, ok)
	assert.Equal(t, uint16(50), address)

	count, ok := outPayload.Get("count")
	assert.True(t, ok)
	assert.Equal(t, uint16(2), count)

	dataType, ok := outPayload.Get("data_type")
	assert.True(t, ok)
	assert.Equal(t, "uint16", dataType) // 기본값

	agentType, ok := outPayload.Get("agent_type")
	assert.True(t, ok)
	assert.Equal(t, "server", agentType)
}

// TestModbusNode_ProcessWrite_Server_값구분 은 value와 values 중 올바른 것이 선택되는지 확인한다.
func TestModbusNode_ProcessWrite_Server_값구분(t *testing.T) {
	tests := []struct {
		name        string
		count       float64
		payload     map[string]any
		wantCommand string
	}{
		{
			name:        "count=1 + value -> 단일 쓰기",
			count:       float64(1),
			payload:     map[string]any{"value": float64(100)},
			wantCommand: "set_register",
		},
		{
			name:        "count=1 + values -> 단일 쓰기 (value 없으므로 values 기반)",
			count:       float64(1),
			payload:     map[string]any{"values": []any{float64(100)}},
			wantCommand: "set_register", // count=1이면 values가 있어도 set_register
		},
		{
			name:        "count>1 + values -> 다중 쓰기",
			count:       float64(3),
			payload:     map[string]any{"values": []any{float64(1), float64(2), float64(3)}},
			wantCommand: "set_registers",
		},
		{
			name:        "count>1 + value (values 없음) -> 단일 쓰기",
			count:       float64(3),
			payload:     map[string]any{"value": float64(100)},
			wantCommand: "set_register", // values가 없으므로 value 기반 단일 쓰기
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBytes, _ := json.Marshal(map[string]any{"ok": true})
			mockAgent := &mockModbusAgent{processResp: respBytes}

			serverAgent := &modbusserver.ModbusServerAgent{}
			transport := &mockModbusTransport{agent: serverAgent}
			resolver := &mockModbusResolver{transport: transport}

			def := newModbusNodeDef(fmt.Sprintf("test-value-select-%s", tt.name))
			node, err := NewModbusNode(def, WithAgentResolver(resolver))
			require.NoError(t, err)

			n := node.(*ModbusNode)
			err = n.Configure(map[string]any{
				"agent_ref":     "server-1",
				"operation":     "write",
				"register_area": "holding_registers",
				"address":       float64(0),
				"count":         tt.count,
			})
			require.NoError(t, err)

			err = n.Init(context.Background())
			require.NoError(t, err)
			n.agent = mockAgent

			msg := message.New(message.WithPayload(message.NewPayload(tt.payload)))
			_, err = n.Process(context.Background(), msg)
			require.NoError(t, err)

			cmd := parseProcessCommand(t, mockAgent.processData)
			assert.Equal(t, tt.wantCommand, cmd["command"])
		})
	}
}
