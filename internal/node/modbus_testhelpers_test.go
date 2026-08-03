package node

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// Modbus 노드 테스트 공통 모의 객체 / 헬퍼
// ---------------------------------------------------------------------------
// 이 파일은 삭제된 modbus/modbus-poller/modbus-writer 노드 테스트에서 공유되던
// 모의 객체와 헬퍼를 보존한 것으로, 새 command-set 노드 테스트가 재사용한다.

// mockModbusAgent 는 테스트용 agent.Agent 구현이다.
// Process() 호출 시 수신한 데이터를 기록하고 미리 설정된 응답을 반환한다.
type mockModbusAgent struct {
	processData []byte // 마지막 Process() 호출 시 전달된 데이터
	processResp []byte // Process() 호출 시 반환할 응답
	processErr  error  // Process() 호출 시 반환할 에러

	// lifecycle 호출 추적 (control 노드 테스트용)
	startCalls  int
	stopCalls   int
	pauseCalls  int
	resumeCalls int
}

func (m *mockModbusAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockModbusAgent) Start(_ context.Context) error       { m.startCalls++; return nil }
func (m *mockModbusAgent) Stop(_ context.Context) error        { m.stopCalls++; return nil }
func (m *mockModbusAgent) Pause(_ context.Context) error       { m.pauseCalls++; return nil }
func (m *mockModbusAgent) Resume(_ context.Context) error      { m.resumeCalls++; return nil }
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

// slowModbusAgent 는 Process() 호출 시 지연을 발생시키는 테스트용 Agent이다.
// timeout 경로 검증에 사용한다.
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

// ---------------------------------------------------------------------------
// 헬퍼 함수
// ---------------------------------------------------------------------------

// parseProcessCommand 는 Agent.Process()에 전달된 JSON 바이트를 파싱하여 map으로 반환한다.
func parseProcessCommand(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var cmd map[string]any
	err := json.Unmarshal(data, &cmd)
	require.NoError(t, err, "Process 명령 JSON 파싱 실패")
	return cmd
}

// mustJSON 은 테스트용 JSON 바이트를 생성하는 헬퍼이다.
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
