package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// --- 모의 객체 ---

// mockAgentResolver 는 테스트용 AgentResolver 구현이다.
type mockAgentResolver struct {
	transport AgentTransport
	err       error
}

func (m *mockAgentResolver) ResolveAgent(_ context.Context, _ flow.AgentRef) (AgentTransport, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.transport, nil
}

// mockAgentTransport 는 테스트용 AgentTransport 구현이다.
type mockAgentTransport struct {
	sentMsgs    []message.Message
	receiveMsg  message.Message
	receiveErr  error
	sendErr     error
}

func (m *mockAgentTransport) Send(_ context.Context, msg message.Message) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sentMsgs = append(m.sentMsgs, msg)
	return nil
}

func (m *mockAgentTransport) Receive(ctx context.Context) (message.Message, error) {
	if m.receiveErr != nil {
		return nil, m.receiveErr
	}
	if m.receiveMsg != nil {
		return m.receiveMsg, nil
	}
	// 타임아웃까지 대기
	<-ctx.Done()
	return nil, ctx.Err()
}

// --- BridgeNode 인터페이스 준수 ---

var _ Node = (*BridgeNode)(nil)

// --- NewBridgeNode 테스트 ---

// TestNewBridgeNode_정상생성 은 BridgeNode가 올바르게 생성되는지 확인한다.
func TestNewBridgeNode_정상생성(t *testing.T) {
	def := flow.NewNodeDef("bridge-1", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-1",
			AgentName: "test-agent",
			Direction: flow.BridgeOut,
		}),
	)

	node, err := NewBridgeNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "bridge-1", node.Name())
}

// TestNewBridgeNode_AgentRef없음_에러 는 AgentRef가 nil이면 에러를 반환하는지 확인한다.
func TestNewBridgeNode_AgentRef없음_에러(t *testing.T) {
	def := flow.NewNodeDef("bridge-no-agent", "bridge")
	// AgentRef가 nil

	_, err := NewBridgeNode(def)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

// TestNewBridgeNode_기본타임아웃 은 기본 replyTimeout이 30초인지 확인한다.
func TestNewBridgeNode_기본타임아웃(t *testing.T) {
	def := flow.NewNodeDef("bridge-timeout", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-1",
			AgentName: "test-agent",
			Direction: flow.BridgeRequestReply,
		}),
	)

	node, err := NewBridgeNode(def)
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	assert.Equal(t, 30*time.Second, bn.replyTimeout)
}

// TestNewBridgeNode_커스텀타임아웃 은 WithReplyTimeout이 적용되는지 확인한다.
func TestNewBridgeNode_커스텀타임아웃(t *testing.T) {
	def := flow.NewNodeDef("bridge-custom-timeout", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-1",
			AgentName: "test-agent",
			Direction: flow.BridgeRequestReply,
		}),
	)

	node, err := NewBridgeNode(def, WithReplyTimeout(5*time.Second))
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	assert.Equal(t, 5*time.Second, bn.replyTimeout)
}

// --- Init 테스트 ---

// TestBridgeNode_Init_resolver없음_에러 는 resolver가 nil이면 에러를 반환하는지 확인한다.
func TestBridgeNode_Init_resolver없음_에러(t *testing.T) {
	def := flow.NewNodeDef("bridge-no-resolver", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-1",
			AgentName: "test-agent",
			Direction: flow.BridgeOut,
		}),
	)

	node, _ := NewBridgeNode(def)
	bn := node.(*BridgeNode)

	err := bn.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentNotFound)
}

// TestBridgeNode_Init_resolver정상 은 resolver가 정상이면 Running 상태로 전이하는지 확인한다.
func TestBridgeNode_Init_resolver정상(t *testing.T) {
	transport := &mockAgentTransport{}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-ok-init", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-1",
			AgentName: "test-agent",
			Direction: flow.BridgeOut,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver))
	bn := node.(*BridgeNode)

	err := bn.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, bn.CurrentState())
}

// TestBridgeNode_Init_resolver에러 는 resolver가 에러를 반환하면 에러를 전파하는지 확인한다.
func TestBridgeNode_Init_resolver에러(t *testing.T) {
	resolveErr := errors.New("에이전트 해석 실패")
	resolver := &mockAgentResolver{err: resolveErr}

	def := flow.NewNodeDef("bridge-resolve-err", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-1",
			AgentName: "test-agent",
			Direction: flow.BridgeOut,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver))
	bn := node.(*BridgeNode)

	err := bn.Init(context.Background())
	require.Error(t, err)
}

// --- Process 테스트 ---

// TestBridgeNode_Process_BridgeOut_전송 은 BridgeOut 모드에서 메시지를 에이전트에 전송하는지 확인한다.
func TestBridgeNode_Process_BridgeOut_전송(t *testing.T) {
	transport := &mockAgentTransport{}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-out", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-1",
			AgentName: "test-agent",
			Direction: flow.BridgeOut,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver))
	bn := node.(*BridgeNode)
	_ = bn.Init(context.Background())

	msg := message.New()
	results, err := bn.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Empty(t, results) // BridgeOut은 빈 결과 반환
	assert.Len(t, transport.sentMsgs, 1) // 전송됨
}

// TestBridgeNode_Process_BridgeOut_전송에러 는 전송 에러 시 에러를 반환하는지 확인한다.
func TestBridgeNode_Process_BridgeOut_전송에러(t *testing.T) {
	sendErr := errors.New("전송 실패")
	transport := &mockAgentTransport{sendErr: sendErr}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-out-err", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-1",
			AgentName: "test-agent",
			Direction: flow.BridgeOut,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver))
	bn := node.(*BridgeNode)
	_ = bn.Init(context.Background())

	msg := message.New()
	_, err := bn.Process(context.Background(), msg)
	assert.Error(t, err)
	assert.Equal(t, sendErr, err)
}

// TestBridgeNode_Process_BridgeIn_패스스루 는 BridgeIn 모드에서 메시지를 통과시키는지 확인한다.
func TestBridgeNode_Process_BridgeIn_패스스루(t *testing.T) {
	transport := &mockAgentTransport{}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-in", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-1",
			AgentName: "test-agent",
			Direction: flow.BridgeIn,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver))
	bn := node.(*BridgeNode)
	_ = bn.Init(context.Background())

	msg := message.New()
	results, err := bn.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, msg.ID(), results[0].ID())
}

// TestBridgeNode_Process_BridgeInOut_패스스루 는 BridgeInOut 모드에서 메시지를 통과시키는지 확인한다.
func TestBridgeNode_Process_BridgeInOut_패스스루(t *testing.T) {
	transport := &mockAgentTransport{}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-inout", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-1",
			AgentName: "test-agent",
			Direction: flow.BridgeInOut,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver))
	bn := node.(*BridgeNode)
	_ = bn.Init(context.Background())

	msg := message.New()
	results, err := bn.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

// TestBridgeNode_Process_RequestReply_정상 은 요청-응답 패턴이 정상 동작하는지 확인한다.
func TestBridgeNode_Process_RequestReply_정상(t *testing.T) {
	replyMsg := message.New()
	transport := &mockAgentTransport{receiveMsg: replyMsg}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-rr", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-1",
			AgentName: "test-agent",
			Direction: flow.BridgeRequestReply,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver), WithReplyTimeout(1*time.Second))
	bn := node.(*BridgeNode)
	_ = bn.Init(context.Background())

	msg := message.New()
	results, err := bn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, replyMsg.ID(), results[0].ID())
}

// TestBridgeNode_Process_RequestReply_타임아웃 은 응답 타임아웃 시 에러를 반환하는지 확인한다.
func TestBridgeNode_Process_RequestReply_타임아웃(t *testing.T) {
	transport := &mockAgentTransport{receiveErr: context.DeadlineExceeded}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-rr-timeout", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-1",
			AgentName: "test-agent",
			Direction: flow.BridgeRequestReply,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver), WithReplyTimeout(50*time.Millisecond))
	bn := node.(*BridgeNode)
	_ = bn.Init(context.Background())

	msg := message.New()
	_, err := bn.Process(context.Background(), msg)
	assert.ErrorIs(t, err, ErrRequestTimeout)
}

// --- Shutdown 테스트 ---

// TestBridgeNode_Shutdown_상태전이 는 Shutdown 시 Stopping 상태로 전이하는지 확인한다.
func TestBridgeNode_Shutdown_상태전이(t *testing.T) {
	transport := &mockAgentTransport{}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-shut", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-1",
			AgentName: "test-agent",
			Direction: flow.BridgeOut,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver))
	bn := node.(*BridgeNode)
	_ = bn.Init(context.Background())

	err := bn.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, bn.CurrentState())
}
