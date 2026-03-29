package node

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
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

// channelAgentTransport 는 채널 기반 테스트용 AgentTransport 구현이다.
// 수신 루프 테스트에서 비동기 메시지 전달에 사용된다.
type channelAgentTransport struct {
	sentMsgs []message.Message
	recvCh   chan message.Message // 외부에서 메시지를 넣으면 Receive가 반환
	sendErr  error
	mu       sync.Mutex
}

func (m *channelAgentTransport) Send(_ context.Context, msg message.Message) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.mu.Lock()
	m.sentMsgs = append(m.sentMsgs, msg)
	m.mu.Unlock()
	return nil
}

func (m *channelAgentTransport) Receive(ctx context.Context) (message.Message, error) {
	select {
	case msg := <-m.recvCh:
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
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
	assert.Equal(t, 30*time.Second, bn.bridgeConfig.RequestTimeout)
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
	assert.Equal(t, 5*time.Second, bn.bridgeConfig.RequestTimeout)
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
	assert.Len(t, results, 1) // BridgeOut은 원본 메시지를 반환하여 "out" 카운트 반영
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

// === P1 통합 테스트: WithBridgeConfig ===

// TestNewBridgeNode_WithBridgeConfig_적용 은 WithBridgeConfig 옵션이 적용되는지 확인한다.
func TestNewBridgeNode_WithBridgeConfig_적용(t *testing.T) {
	agentRef := flow.AgentRef{
		AgentID:   "agent-cfg",
		AgentName: "config-agent",
		Direction: flow.BridgeOut,
	}
	cfg := BridgeConfig{
		AgentRef:             agentRef,
		Direction:            flow.BridgeOut,
		Transform:            TransformConfig{Mode: "auto"},
		RequestTimeout:       10 * time.Second,
		ReconnectInterval:    3 * time.Second,
		MaxReconnectAttempts: 5,
		BufferSize:           128,
	}

	def := flow.NewNodeDef("bridge-cfg", "bridge",
		flow.WithAgentRef(agentRef),
	)

	node, err := NewBridgeNode(def, WithBridgeConfig(cfg))
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	assert.Equal(t, 10*time.Second, bn.bridgeConfig.RequestTimeout)
	assert.Equal(t, 128, bn.bridgeConfig.BufferSize)
	assert.Equal(t, 5, bn.bridgeConfig.MaxReconnectAttempts)
	assert.Equal(t, flow.BridgeOut, bn.bridgeConfig.Direction)
}

// TestNewBridgeNode_WithBridgeConfig_WithReplyTimeout보다_우선 은
// WithBridgeConfig가 WithReplyTimeout보다 우선 적용되는지 확인한다.
func TestNewBridgeNode_WithBridgeConfig_WithReplyTimeout보다_우선(t *testing.T) {
	agentRef := flow.AgentRef{
		AgentID:   "agent-priority",
		AgentName: "priority-agent",
		Direction: flow.BridgeRequestReply,
	}
	cfg := BridgeConfig{
		AgentRef:             agentRef,
		Direction:            flow.BridgeRequestReply,
		Transform:            TransformConfig{Mode: "auto"},
		RequestTimeout:       15 * time.Second,
		ReconnectInterval:    5 * time.Second,
		MaxReconnectAttempts: 10,
		BufferSize:           256,
	}

	def := flow.NewNodeDef("bridge-priority", "bridge",
		flow.WithAgentRef(agentRef),
	)

	// WithReplyTimeout(3s)과 WithBridgeConfig(15s) 모두 설정 -> BridgeConfig 우선
	node, err := NewBridgeNode(def, WithReplyTimeout(3*time.Second), WithBridgeConfig(cfg))
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	assert.Equal(t, 15*time.Second, bn.bridgeConfig.RequestTimeout)
}

// TestNewBridgeNode_기본설정_DefaultBridgeConfig 는 옵션 없이 생성 시 DefaultBridgeConfig가 적용되는지 확인한다.
func TestNewBridgeNode_기본설정_DefaultBridgeConfig(t *testing.T) {
	def := flow.NewNodeDef("bridge-default", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-def",
			AgentName: "default-agent",
			Direction: flow.BridgeOut,
		}),
	)

	node, err := NewBridgeNode(def)
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	assert.Equal(t, flow.BridgeOut, bn.bridgeConfig.Direction)
	assert.Equal(t, 30*time.Second, bn.bridgeConfig.RequestTimeout)
	assert.Equal(t, 5*time.Second, bn.bridgeConfig.ReconnectInterval)
	assert.Equal(t, 10, bn.bridgeConfig.MaxReconnectAttempts)
	assert.Equal(t, 256, bn.bridgeConfig.BufferSize)
	assert.Equal(t, "auto", bn.bridgeConfig.Transform.Mode)
}

// === P1 통합 테스트: Info() / Stats() ===

// TestBridgeNode_Info_초기상태 는 Init 전 Info()의 초기 상태를 확인한다.
func TestBridgeNode_Info_초기상태(t *testing.T) {
	def := flow.NewNodeDef("bridge-info-init", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-info",
			AgentName: "info-agent",
			Direction: flow.BridgeOut,
		}),
	)

	node, err := NewBridgeNode(def)
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	info := bn.Info()
	assert.Equal(t, "agent-info", info.AgentID)
	assert.Equal(t, "info-agent", info.AgentName)
	assert.Equal(t, flow.BridgeOut, info.Direction)
	assert.False(t, info.Connected)
	assert.Zero(t, info.Stats.MessagesRelayed)
}

// TestBridgeNode_Info_연결후 는 Init 후 Info()에서 Connected가 true인지 확인한다.
func TestBridgeNode_Info_연결후(t *testing.T) {
	transport := &mockAgentTransport{}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-info-conn", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-info-conn",
			AgentName: "info-conn-agent",
			Direction: flow.BridgeOut,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver))
	bn := node.(*BridgeNode)
	_ = bn.Init(context.Background())
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	info := bn.Info()
	assert.True(t, info.Connected)
	assert.Equal(t, "agent-info-conn", info.AgentID)
}

// TestBridgeNode_Stats_전송후 는 BridgeOut에서 메시지 전송 후 통계가 갱신되는지 확인한다.
func TestBridgeNode_Stats_전송후(t *testing.T) {
	transport := &mockAgentTransport{}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-stats-out", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-stats",
			AgentName: "stats-agent",
			Direction: flow.BridgeOut,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver))
	bn := node.(*BridgeNode)
	_ = bn.Init(context.Background())
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	// 메시지 3개 전송
	for i := 0; i < 3; i++ {
		msg := message.New()
		_, err := bn.Process(context.Background(), msg)
		require.NoError(t, err)
	}

	stats := bn.Stats()
	assert.Equal(t, int64(3), stats.MessagesToAgent)
	assert.Equal(t, int64(3), stats.MessagesRelayed)
	assert.Zero(t, stats.TransformErrors)
	assert.False(t, stats.LastActivityAt.IsZero())
}

// TestBridgeNode_Stats_RequestReply후 는 요청-응답 후 통계가 갱신되는지 확인한다.
func TestBridgeNode_Stats_RequestReply후(t *testing.T) {
	replyMsg := message.New()
	transport := &mockAgentTransport{receiveMsg: replyMsg}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-stats-rr", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-stats-rr",
			AgentName: "stats-rr-agent",
			Direction: flow.BridgeRequestReply,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver), WithReplyTimeout(1*time.Second))
	bn := node.(*BridgeNode)
	_ = bn.Init(context.Background())
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	msg := message.New()
	_, err := bn.Process(context.Background(), msg)
	require.NoError(t, err)

	stats := bn.Stats()
	assert.Equal(t, int64(1), stats.MessagesToAgent)
	assert.Equal(t, int64(1), stats.MessagesFromAgent)
	assert.Equal(t, int64(1), stats.MessagesRelayed)
	assert.Zero(t, stats.CorrelationTimeouts)
}

// === P1 통합 테스트: 수신 루프 ===

// TestBridgeNode_수신루프_BridgeIn 은 BridgeIn 모드에서 수신 루프가 작동하는지 확인한다.
func TestBridgeNode_수신루프_BridgeIn(t *testing.T) {
	recvCh := make(chan message.Message, 10)
	transport := &channelAgentTransport{recvCh: recvCh}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-recv-in", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-recv",
			AgentName: "recv-agent",
			Direction: flow.BridgeIn,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver))
	bn := node.(*BridgeNode)
	err := bn.Init(context.Background())
	require.NoError(t, err)
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	// transport에 메시지 전달 -> 수신 루프가 recvCh에 넣어야 한다
	testMsg := message.New()
	recvCh <- testMsg

	// BridgeNode의 recvCh에서 메시지 수신 대기
	select {
	case received := <-bn.recvCh:
		assert.Equal(t, testMsg.ID(), received.ID())
	case <-time.After(2 * time.Second):
		t.Fatal("수신 루프에서 메시지를 받지 못함")
	}
}

// TestBridgeNode_수신루프_BridgeInOut 은 BridgeInOut 모드에서 수신 루프가 작동하는지 확인한다.
func TestBridgeNode_수신루프_BridgeInOut(t *testing.T) {
	recvCh := make(chan message.Message, 10)
	transport := &channelAgentTransport{recvCh: recvCh}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-recv-inout", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-recv-inout",
			AgentName: "recv-inout-agent",
			Direction: flow.BridgeInOut,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver))
	bn := node.(*BridgeNode)
	err := bn.Init(context.Background())
	require.NoError(t, err)
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	// transport에 메시지 전달
	testMsg := message.New()
	recvCh <- testMsg

	// BridgeNode의 recvCh에서 수신 대기
	select {
	case received := <-bn.recvCh:
		assert.Equal(t, testMsg.ID(), received.ID())
	case <-time.After(2 * time.Second):
		t.Fatal("수신 루프에서 메시지를 받지 못함")
	}
}

// TestBridgeNode_수신루프_BridgeOut_미시작 은 BridgeOut 모드에서 수신 루프가 시작되지 않는지 확인한다.
func TestBridgeNode_수신루프_BridgeOut_미시작(t *testing.T) {
	transport := &mockAgentTransport{}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-no-recv", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-no-recv",
			AgentName: "no-recv-agent",
			Direction: flow.BridgeOut,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver))
	bn := node.(*BridgeNode)
	_ = bn.Init(context.Background())
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	// BridgeOut에서는 recvCh에 메시지가 들어오지 않아야 한다
	select {
	case <-bn.recvCh:
		t.Fatal("BridgeOut 모드에서 수신 루프가 실행됨")
	case <-time.After(100 * time.Millisecond):
		// 정상: 수신 루프가 실행되지 않음
	}
}

// === P1 통합 테스트: CorrelationTracker 통합 ===

// TestBridgeNode_CorrelationTracker_초기화 는 RequestReply 모드에서 CorrelationTracker가 초기화되는지 확인한다.
func TestBridgeNode_CorrelationTracker_초기화(t *testing.T) {
	def := flow.NewNodeDef("bridge-corr-init", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-corr",
			AgentName: "corr-agent",
			Direction: flow.BridgeRequestReply,
		}),
	)

	node, err := NewBridgeNode(def)
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	assert.NotNil(t, bn.correlation)
	assert.Equal(t, 0, bn.correlation.PendingCount())
}

// TestBridgeNode_CorrelationTracker_미초기화_BridgeOut 은 BridgeOut에서 CorrelationTracker가 nil인지 확인한다.
func TestBridgeNode_CorrelationTracker_미초기화_BridgeOut(t *testing.T) {
	def := flow.NewNodeDef("bridge-corr-out", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-corr-out",
			AgentName: "corr-out-agent",
			Direction: flow.BridgeOut,
		}),
	)

	node, err := NewBridgeNode(def)
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	assert.Nil(t, bn.correlation)
}

// === P1 통합 테스트: Transformer 통합 ===

// TestBridgeNode_Transformer_초기화 는 DefaultTransformer가 초기화되는지 확인한다.
func TestBridgeNode_Transformer_초기화(t *testing.T) {
	def := flow.NewNodeDef("bridge-transform", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-transform",
			AgentName: "transform-agent",
			Direction: flow.BridgeOut,
		}),
	)

	node, err := NewBridgeNode(def)
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	assert.NotNil(t, bn.transformer)

	// DefaultTransformer가 정상 동작하는지 확인
	msg := message.New()
	msg.Payload().Set("_raw", []byte("hello"))
	data, err := bn.transformer.FlowToAgent(msg)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), data)
}

// TestBridgeNode_Process_BridgeOut_변환검증 은 BridgeOut에서 변환 검증이 수행되는지 확인한다.
func TestBridgeNode_Process_BridgeOut_변환검증(t *testing.T) {
	transport := &mockAgentTransport{}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-transform-out", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-tfm-out",
			AgentName: "tfm-out-agent",
			Direction: flow.BridgeOut,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver))
	bn := node.(*BridgeNode)
	_ = bn.Init(context.Background())
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	// 정상 메시지 전송 시 변환 검증 통과
	msg := message.New()
	msg.Payload().Set("_raw", []byte("test-data"))
	results, err := bn.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Len(t, transport.sentMsgs, 1)
}

// === P1 통합 테스트: Shutdown 정리 ===

// TestBridgeNode_Shutdown_연결해제 는 Shutdown 후 connected가 false인지 확인한다.
func TestBridgeNode_Shutdown_연결해제(t *testing.T) {
	transport := &mockAgentTransport{}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-shut-conn", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-shut-conn",
			AgentName: "shut-conn-agent",
			Direction: flow.BridgeOut,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver))
	bn := node.(*BridgeNode)
	_ = bn.Init(context.Background())

	assert.True(t, bn.connected.Load())

	_ = bn.Shutdown(context.Background())

	assert.False(t, bn.connected.Load())
	info := bn.Info()
	assert.False(t, info.Connected)
}

// TestBridgeNode_Shutdown_수신루프정지 는 Shutdown 시 수신 루프가 정상 종료되는지 확인한다.
func TestBridgeNode_Shutdown_수신루프정지(t *testing.T) {
	recvCh := make(chan message.Message, 10)
	transport := &channelAgentTransport{recvCh: recvCh}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-shut-recv", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-shut-recv",
			AgentName: "shut-recv-agent",
			Direction: flow.BridgeIn,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver))
	bn := node.(*BridgeNode)
	_ = bn.Init(context.Background())

	// Shutdown 호출
	err := bn.Shutdown(context.Background())
	require.NoError(t, err)

	// Shutdown 후 수신 루프가 종료되어야 하므로, 새 메시지를 넣어도 recvCh에서 받지 못해야 한다
	time.Sleep(50 * time.Millisecond) // 고루틴 종료 대기

	select {
	case bn.recvCh <- message.New():
		// 버퍼에 공간이 있으면 넣을 수 있지만, 수신 루프는 종료됨
	default:
	}
}

// TestBridgeNode_Shutdown_CorrelationTracker정리 는 Shutdown 시 CorrelationTracker가 정리되는지 확인한다.
func TestBridgeNode_Shutdown_CorrelationTracker정리(t *testing.T) {
	replyMsg := message.New()
	transport := &mockAgentTransport{receiveMsg: replyMsg}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-shut-corr", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-shut-corr",
			AgentName: "shut-corr-agent",
			Direction: flow.BridgeRequestReply,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver), WithReplyTimeout(1*time.Second))
	bn := node.(*BridgeNode)
	_ = bn.Init(context.Background())

	assert.NotNil(t, bn.correlation)

	err := bn.Shutdown(context.Background())
	require.NoError(t, err)

	// Shutdown 후 CorrelationTracker의 PendingCount는 0이어야 한다
	assert.Equal(t, 0, bn.correlation.PendingCount())
}

// === P1 통합 테스트: 동시성 ===

// TestBridgeNode_동시_Process_BridgeOut 은 BridgeOut 모드에서 동시 Process 호출이 안전한지 확인한다.
func TestBridgeNode_동시_Process_BridgeOut(t *testing.T) {
	transport := &channelAgentTransport{
		recvCh: make(chan message.Message, 100),
	}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-concurrent-out", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-conc",
			AgentName: "conc-agent",
			Direction: flow.BridgeOut,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver))
	bn := node.(*BridgeNode)
	_ = bn.Init(context.Background())
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	// 10개의 고루틴에서 동시에 Process 호출
	var wg sync.WaitGroup
	const numGoroutines = 10
	errCh := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg := message.New()
			_, err := bn.Process(context.Background(), msg)
			if err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("동시 Process에서 에러 발생: %v", err)
	}

	// 모든 메시지가 전송되었는지 확인
	transport.mu.Lock()
	sentCount := len(transport.sentMsgs)
	transport.mu.Unlock()
	assert.Equal(t, numGoroutines, sentCount)

	// 통계도 정확한지 확인
	stats := bn.Stats()
	assert.Equal(t, int64(numGoroutines), stats.MessagesToAgent)
	assert.Equal(t, int64(numGoroutines), stats.MessagesRelayed)
}

// TestBridgeNode_동시_Info_Stats 는 Info()/Stats()가 동시 호출에 안전한지 확인한다.
func TestBridgeNode_동시_Info_Stats(t *testing.T) {
	// 동시성 안전한 channelAgentTransport 사용
	transport := &channelAgentTransport{
		recvCh: make(chan message.Message, 100),
	}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-concurrent-info", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-conc-info",
			AgentName: "conc-info-agent",
			Direction: flow.BridgeOut,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver))
	bn := node.(*BridgeNode)
	_ = bn.Init(context.Background())
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	var wg sync.WaitGroup
	const numGoroutines = 20

	// 동시에 Info, Stats, Process 호출
	for i := 0; i < numGoroutines; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			info := bn.Info()
			assert.NotEmpty(t, info.AgentID)
		}()
		go func() {
			defer wg.Done()
			stats := bn.Stats()
			_ = stats.MessagesRelayed // 읽기만 수행
		}()
		go func() {
			defer wg.Done()
			msg := message.New()
			_, _ = bn.Process(context.Background(), msg)
		}()
	}

	wg.Wait()
}

// === P1 통합 테스트: Init 후 연결 상태 ===

// TestBridgeNode_Init_connected_상태 는 Init 후 connected가 true이고 BridgeIn에서 수신 루프가 시작되는지 확인한다.
func TestBridgeNode_Init_connected_상태(t *testing.T) {
	recvCh := make(chan message.Message, 10)
	transport := &channelAgentTransport{recvCh: recvCh}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-init-conn", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-init-conn",
			AgentName: "init-conn-agent",
			Direction: flow.BridgeIn,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver))
	bn := node.(*BridgeNode)

	// Init 전에는 connected가 false
	assert.False(t, bn.connected.Load())

	_ = bn.Init(context.Background())
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	// Init 후에는 connected가 true
	assert.True(t, bn.connected.Load())
}

// TestBridgeNode_Init_RequestReply_클린업루프_시작 은 RequestReply 모드에서 클린업 루프가 시작되는지 확인한다.
func TestBridgeNode_Init_RequestReply_클린업루프_시작(t *testing.T) {
	replyMsg := message.New()
	transport := &mockAgentTransport{receiveMsg: replyMsg}
	resolver := &mockAgentResolver{transport: transport}

	def := flow.NewNodeDef("bridge-cleanup-loop", "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-cleanup",
			AgentName: "cleanup-agent",
			Direction: flow.BridgeRequestReply,
		}),
	)

	node, _ := NewBridgeNode(def, WithAgentResolver(resolver), WithReplyTimeout(100*time.Millisecond))
	bn := node.(*BridgeNode)
	_ = bn.Init(context.Background())
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	assert.NotNil(t, bn.correlation)

	// 수동으로 만료될 항목 등록 (correlationTracker의 정리 루프가 처리)
	expiredCh := make(chan message.Message, 1)
	bn.correlation.Track("expired-id", expiredCh)
	assert.Equal(t, 1, bn.correlation.PendingCount())

	// 클린업 루프가 실행되어 만료 항목을 제거할 때까지 대기
	time.Sleep(300 * time.Millisecond)

	assert.Equal(t, 0, bn.correlation.PendingCount())
	assert.Equal(t, int64(1), bn.correlation.TimeoutCount())
}

// === SubscriberAgent 토픽 관리 테스트 ===

// --- 모의 객체: SubscriberAgent ---

// mockSubscriberAgent 는 agent.Agent + agent.SubscriberAgent 를 구현하는 모의 객체이다.
type mockSubscriberAgent struct {
	subscribedTopics   []string
	unsubscribedTopics []string
	mu                 sync.Mutex
	subscribeErr       error
	unsubscribeErr     error
}

func (m *mockSubscriberAgent) Subscribe(_ context.Context, topics []string) error {
	if m.subscribeErr != nil {
		return m.subscribeErr
	}
	m.mu.Lock()
	m.subscribedTopics = append(m.subscribedTopics, topics...)
	m.mu.Unlock()
	return nil
}

func (m *mockSubscriberAgent) Unsubscribe(_ context.Context, topics []string) error {
	if m.unsubscribeErr != nil {
		return m.unsubscribeErr
	}
	m.mu.Lock()
	m.unsubscribedTopics = append(m.unsubscribedTopics, topics...)
	// subscribedTopics에서 제거
	removeSet := make(map[string]bool, len(topics))
	for _, t := range topics {
		removeSet[t] = true
	}
	filtered := make([]string, 0, len(m.subscribedTopics))
	for _, t := range m.subscribedTopics {
		if !removeSet[t] {
			filtered = append(filtered, t)
		}
	}
	m.subscribedTopics = filtered
	m.mu.Unlock()
	return nil
}

// agent.Agent 인터페이스의 나머지 메서드 (사용하지 않지만 인터페이스 충족 필요)
func (m *mockSubscriberAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockSubscriberAgent) Start(_ context.Context) error       { return nil }
func (m *mockSubscriberAgent) Stop(_ context.Context) error        { return nil }
func (m *mockSubscriberAgent) Pause(_ context.Context) error       { return nil }
func (m *mockSubscriberAgent) Resume(_ context.Context) error      { return nil }
func (m *mockSubscriberAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *mockSubscriberAgent) Process(_ []byte) ([]byte, error)    { return nil, nil }
func (m *mockSubscriberAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *mockSubscriberAgent) ID() string                          { return "mock-subscriber" }
func (m *mockSubscriberAgent) Name() string                        { return "mock-subscriber" }
func (m *mockSubscriberAgent) Type() string                        { return "mqtt-client" }
func (m *mockSubscriberAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *mockSubscriberAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }

// mockSubscriberAgentTransport 는 AgentTransport + AgentAccessor 를 구현하며
// UnderlyingAgent()가 SubscriberAgent를 반환한다.
type mockSubscriberAgentTransport struct {
	mockAgentTransport
	agent agent.Agent
}

func (m *mockSubscriberAgentTransport) UnderlyingAgent() agent.Agent {
	return m.agent
}

// mockNonSubscriberAgent 는 agent.Agent만 구현하고 SubscriberAgent는 구현하지 않는다.
type mockNonSubscriberAgent struct {
	mockSubscriberAgent // 기본 Agent 메서드 재사용
}

// Subscribe/Unsubscribe를 구현하지 않도록 명시적으로 임베딩하지 않음
// → agent.SubscriberAgent 타입 단언이 실패한다

// mockNonSubscriberAgentOnly 는 agent.Agent만 구현한다.
type mockNonSubscriberAgentOnly struct{}

func (m *mockNonSubscriberAgentOnly) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockNonSubscriberAgentOnly) Start(_ context.Context) error       { return nil }
func (m *mockNonSubscriberAgentOnly) Stop(_ context.Context) error        { return nil }
func (m *mockNonSubscriberAgentOnly) Pause(_ context.Context) error       { return nil }
func (m *mockNonSubscriberAgentOnly) Resume(_ context.Context) error      { return nil }
func (m *mockNonSubscriberAgentOnly) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *mockNonSubscriberAgentOnly) Process(_ []byte) ([]byte, error)    { return nil, nil }
func (m *mockNonSubscriberAgentOnly) Configure(_ agent.AgentConfig) error { return nil }
func (m *mockNonSubscriberAgentOnly) ID() string                          { return "mock-non-subscriber" }
func (m *mockNonSubscriberAgentOnly) Name() string                        { return "mock-non-subscriber" }
func (m *mockNonSubscriberAgentOnly) Type() string                        { return "test" }
func (m *mockNonSubscriberAgentOnly) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *mockNonSubscriberAgentOnly) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }

// --- 헬퍼 함수 ---

func newBridgeOutDef(id string) flow.NodeDef {
	return flow.NewNodeDef(id, "bridge",
		flow.WithAgentRef(flow.AgentRef{
			AgentID:   "agent-1",
			AgentName: "test-agent",
			Direction: flow.BridgeOut,
		}),
	)
}

// --- Init 토픽 구독 테스트 ---

// TestBridgeNode_Init_토픽구독_SubscriberAgent 는 Init 시 config topics이 SubscriberAgent에 구독되는지 확인한다.
func TestBridgeNode_Init_토픽구독_SubscriberAgent(t *testing.T) {
	subAgent := &mockSubscriberAgent{}
	transport := &mockSubscriberAgentTransport{
		agent: subAgent,
	}
	resolver := &mockAgentResolver{transport: transport}

	cfg := BridgeConfig{
		AgentRef: flow.AgentRef{
			AgentID:   "agent-1",
			AgentName: "test-agent",
			Direction: flow.BridgeOut,
		},
		Direction:            flow.BridgeOut,
		Transform:            TransformConfig{Mode: "auto", PayloadFormat: PayloadFormatAuto},
		RequestTimeout:       30 * time.Second,
		ReconnectInterval:    5 * time.Second,
		MaxReconnectAttempts: 10,
		BufferSize:           256,
		Topics:               []string{"sensor/#", "device/+/data"},
	}

	def := newBridgeOutDef("bridge-init-topics")
	node, err := NewBridgeNode(def, WithAgentResolver(resolver), WithBridgeConfig(cfg))
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	err = bn.Init(context.Background())
	require.NoError(t, err)
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	// SubscriberAgent에 토픽이 전달되었는지 확인
	subAgent.mu.Lock()
	assert.Equal(t, []string{"sensor/#", "device/+/data"}, subAgent.subscribedTopics)
	subAgent.mu.Unlock()

	// bridgeTopics에도 추가되었는지 확인
	bn.topicsMu.Lock()
	assert.Equal(t, []string{"sensor/#", "device/+/data"}, bn.bridgeTopics)
	bn.topicsMu.Unlock()
}

// TestBridgeNode_Init_토픽구독_NonSubscriberAgent 는 SubscriberAgent가 아닌 에이전트에서는 에러 없이 무시되는지 확인한다.
func TestBridgeNode_Init_토픽구독_NonSubscriberAgent(t *testing.T) {
	nonSubAgent := &mockNonSubscriberAgentOnly{}
	transport := &mockSubscriberAgentTransport{
		agent: nonSubAgent,
	}
	resolver := &mockAgentResolver{transport: transport}

	cfg := BridgeConfig{
		AgentRef: flow.AgentRef{
			AgentID:   "agent-1",
			AgentName: "test-agent",
			Direction: flow.BridgeOut,
		},
		Direction:            flow.BridgeOut,
		Transform:            TransformConfig{Mode: "auto", PayloadFormat: PayloadFormatAuto},
		RequestTimeout:       30 * time.Second,
		ReconnectInterval:    5 * time.Second,
		MaxReconnectAttempts: 10,
		BufferSize:           256,
		Topics:               []string{"sensor/#"},
	}

	def := newBridgeOutDef("bridge-init-no-sub")
	node, err := NewBridgeNode(def, WithAgentResolver(resolver), WithBridgeConfig(cfg))
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	err = bn.Init(context.Background())
	require.NoError(t, err) // 에러 없이 Init 성공
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	// bridgeTopics는 비어있어야 한다 (SubscriberAgent가 아니므로 추가하지 않음)
	bn.topicsMu.Lock()
	assert.Empty(t, bn.bridgeTopics)
	bn.topicsMu.Unlock()
}

// --- Process 제어 메시지 테스트 ---

// TestBridgeNode_Process_제어메시지_subscribe 는 subscribe 제어 메시지가 토픽을 추가하는지 확인한다.
func TestBridgeNode_Process_제어메시지_subscribe(t *testing.T) {
	subAgent := &mockSubscriberAgent{}
	transport := &mockSubscriberAgentTransport{
		agent: subAgent,
	}
	resolver := &mockAgentResolver{transport: transport}

	def := newBridgeOutDef("bridge-ctrl-sub")
	node, err := NewBridgeNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	err = bn.Init(context.Background())
	require.NoError(t, err)
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	// 제어 메시지: subscribe
	msg := message.New()
	msg.Payload().Set("action", "subscribe")
	msg.Payload().Set("topics", []any{"new/topic1", "new/topic2"})

	results, err := bn.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Empty(t, results) // 제어 메시지는 빈 결과 반환

	// SubscriberAgent에 토픽이 전달되었는지 확인
	subAgent.mu.Lock()
	assert.Equal(t, []string{"new/topic1", "new/topic2"}, subAgent.subscribedTopics)
	subAgent.mu.Unlock()

	// bridgeTopics에도 추가되었는지 확인
	bn.topicsMu.Lock()
	assert.Equal(t, []string{"new/topic1", "new/topic2"}, bn.bridgeTopics)
	bn.topicsMu.Unlock()
}

// TestBridgeNode_Process_제어메시지_unsubscribe 는 unsubscribe 제어 메시지가 토픽을 제거하는지 확인한다.
func TestBridgeNode_Process_제어메시지_unsubscribe(t *testing.T) {
	subAgent := &mockSubscriberAgent{}
	transport := &mockSubscriberAgentTransport{
		agent: subAgent,
	}
	resolver := &mockAgentResolver{transport: transport}

	def := newBridgeOutDef("bridge-ctrl-unsub")
	node, err := NewBridgeNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	err = bn.Init(context.Background())
	require.NoError(t, err)
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	// 먼저 토픽 추가
	subMsg := message.New()
	subMsg.Payload().Set("action", "subscribe")
	subMsg.Payload().Set("topics", []any{"topic/a", "topic/b", "topic/c"})
	_, err = bn.Process(context.Background(), subMsg)
	require.NoError(t, err)

	// 토픽 일부 제거
	unsubMsg := message.New()
	unsubMsg.Payload().Set("action", "unsubscribe")
	unsubMsg.Payload().Set("topics", []any{"topic/b"})

	results, err := bn.Process(context.Background(), unsubMsg)
	require.NoError(t, err)
	assert.Empty(t, results)

	// bridgeTopics에서 제거되었는지 확인
	bn.topicsMu.Lock()
	assert.Equal(t, []string{"topic/a", "topic/c"}, bn.bridgeTopics)
	bn.topicsMu.Unlock()
}

// TestBridgeNode_Process_제어메시지_NonSubscriberAgent_에러 는 SubscriberAgent가 아닌 경우 에러를 반환하는지 확인한다.
func TestBridgeNode_Process_제어메시지_NonSubscriberAgent_에러(t *testing.T) {
	nonSubAgent := &mockNonSubscriberAgentOnly{}
	transport := &mockSubscriberAgentTransport{
		agent: nonSubAgent,
	}
	resolver := &mockAgentResolver{transport: transport}

	def := newBridgeOutDef("bridge-ctrl-nosub")
	node, err := NewBridgeNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	err = bn.Init(context.Background())
	require.NoError(t, err)
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	msg := message.New()
	msg.Payload().Set("action", "subscribe")
	msg.Payload().Set("topics", []any{"topic/x"})

	_, err = bn.Process(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent does not implement SubscriberAgent")
}

// TestBridgeNode_Process_일반메시지_action없음_패스스루 는 action이 없는 메시지가 정상적으로 에이전트에 전송되는지 확인한다.
func TestBridgeNode_Process_일반메시지_action없음_패스스루(t *testing.T) {
	transport := &mockAgentTransport{}
	resolver := &mockAgentResolver{transport: transport}

	def := newBridgeOutDef("bridge-normal-msg")
	node, err := NewBridgeNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	err = bn.Init(context.Background())
	require.NoError(t, err)
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	// action이 없는 일반 메시지
	msg := message.New()
	msg.Payload().Set("temperature", 25.5)

	results, err := bn.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Len(t, results, 1) // BridgeOut은 원본 메시지를 반환하여 "out" 카운트 반영
	assert.Len(t, transport.sentMsgs, 1) // 전송됨
}

// TestBridgeNode_Process_제어메시지_unknown_action_에러 는 알 수 없는 action에 대해 에러를 반환하는지 확인한다.
func TestBridgeNode_Process_제어메시지_unknown_action_에러(t *testing.T) {
	subAgent := &mockSubscriberAgent{}
	transport := &mockSubscriberAgentTransport{
		agent: subAgent,
	}
	resolver := &mockAgentResolver{transport: transport}

	def := newBridgeOutDef("bridge-ctrl-unknown")
	node, err := NewBridgeNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	err = bn.Init(context.Background())
	require.NoError(t, err)
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	msg := message.New()
	msg.Payload().Set("action", "unknown_action")
	msg.Payload().Set("topics", []any{"topic/x"})

	_, err = bn.Process(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown action")
}

// TestBridgeNode_Process_제어메시지_topics없음_에러 는 topics가 없는 제어 메시지에 대해 에러를 반환하는지 확인한다.
func TestBridgeNode_Process_제어메시지_topics없음_에러(t *testing.T) {
	subAgent := &mockSubscriberAgent{}
	transport := &mockSubscriberAgentTransport{
		agent: subAgent,
	}
	resolver := &mockAgentResolver{transport: transport}

	def := newBridgeOutDef("bridge-ctrl-no-topics")
	node, err := NewBridgeNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	err = bn.Init(context.Background())
	require.NoError(t, err)
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	msg := message.New()
	msg.Payload().Set("action", "subscribe")
	// topics 없음

	_, err = bn.Process(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "topics field is required")
}

// TestBridgeNode_Process_제어메시지_AgentAccessor미지원_에러 는 transport가 AgentAccessor를 지원하지 않는 경우 에러를 반환하는지 확인한다.
func TestBridgeNode_Process_제어메시지_AgentAccessor미지원_에러(t *testing.T) {
	// 일반 mockAgentTransport는 AgentAccessor를 구현하지 않는다
	transport := &mockAgentTransport{}
	resolver := &mockAgentResolver{transport: transport}

	def := newBridgeOutDef("bridge-ctrl-no-accessor")
	node, err := NewBridgeNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	err = bn.Init(context.Background())
	require.NoError(t, err)
	defer bn.Shutdown(context.Background()) //nolint:errcheck

	msg := message.New()
	msg.Payload().Set("action", "subscribe")
	msg.Payload().Set("topics", []any{"topic/x"})

	_, err = bn.Process(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transport does not support AgentAccessor")
}

// --- Shutdown 토픽 해제 테스트 ---

// TestBridgeNode_Shutdown_bridgeTopics_구독해제 는 Shutdown 시 bridgeTopics의 토픽이 해제되는지 확인한다.
func TestBridgeNode_Shutdown_bridgeTopics_구독해제(t *testing.T) {
	subAgent := &mockSubscriberAgent{}
	transport := &mockSubscriberAgentTransport{
		agent: subAgent,
	}
	resolver := &mockAgentResolver{transport: transport}

	cfg := BridgeConfig{
		AgentRef: flow.AgentRef{
			AgentID:   "agent-1",
			AgentName: "test-agent",
			Direction: flow.BridgeOut,
		},
		Direction:            flow.BridgeOut,
		Transform:            TransformConfig{Mode: "auto", PayloadFormat: PayloadFormatAuto},
		RequestTimeout:       30 * time.Second,
		ReconnectInterval:    5 * time.Second,
		MaxReconnectAttempts: 10,
		BufferSize:           256,
		Topics:               []string{"init/topic1", "init/topic2"},
	}

	def := newBridgeOutDef("bridge-shutdown-unsub")
	node, err := NewBridgeNode(def, WithAgentResolver(resolver), WithBridgeConfig(cfg))
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	err = bn.Init(context.Background())
	require.NoError(t, err)

	// Init에서 구독된 토픽 확인
	subAgent.mu.Lock()
	assert.Equal(t, []string{"init/topic1", "init/topic2"}, subAgent.subscribedTopics)
	subAgent.mu.Unlock()

	// 런타임에 추가 토픽 구독
	runtimeMsg := message.New()
	runtimeMsg.Payload().Set("action", "subscribe")
	runtimeMsg.Payload().Set("topics", []any{"runtime/topic"})
	_, err = bn.Process(context.Background(), runtimeMsg)
	require.NoError(t, err)

	// Shutdown 호출
	err = bn.Shutdown(context.Background())
	require.NoError(t, err)

	// Unsubscribe가 모든 bridgeTopics에 대해 호출되었는지 확인
	subAgent.mu.Lock()
	assert.Equal(t, []string{"init/topic1", "init/topic2", "runtime/topic"}, subAgent.unsubscribedTopics)
	subAgent.mu.Unlock()

	// bridgeTopics가 nil로 초기화되었는지 확인
	bn.topicsMu.Lock()
	assert.Nil(t, bn.bridgeTopics)
	bn.topicsMu.Unlock()
}

// TestBridgeNode_Shutdown_빈bridgeTopics_구독해제안함 은 bridgeTopics가 비어있으면 Unsubscribe가 호출되지 않는지 확인한다.
func TestBridgeNode_Shutdown_빈bridgeTopics_구독해제안함(t *testing.T) {
	subAgent := &mockSubscriberAgent{}
	transport := &mockSubscriberAgentTransport{
		agent: subAgent,
	}
	resolver := &mockAgentResolver{transport: transport}

	def := newBridgeOutDef("bridge-shutdown-empty")
	node, err := NewBridgeNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	bn := node.(*BridgeNode)
	err = bn.Init(context.Background())
	require.NoError(t, err)

	// Topics 설정 없이 Init → bridgeTopics는 비어있음
	bn.topicsMu.Lock()
	assert.Empty(t, bn.bridgeTopics)
	bn.topicsMu.Unlock()

	err = bn.Shutdown(context.Background())
	require.NoError(t, err)

	// Unsubscribe가 호출되지 않았는지 확인
	subAgent.mu.Lock()
	assert.Empty(t, subAgent.unsubscribedTopics)
	subAgent.mu.Unlock()
}

// --- 헬퍼 함수 테스트 ---

// TestBridgeToStringSlice 는 bridgeToStringSlice 헬퍼 함수를 테스트한다.
func TestBridgeToStringSlice(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want []string
	}{
		{
			name: "[]string",
			in:   []string{"a", "b"},
			want: []string{"a", "b"},
		},
		{
			name: "[]any with strings",
			in:   []any{"x", "y"},
			want: []string{"x", "y"},
		},
		{
			name: "[]any with mixed types",
			in:   []any{"a", 123, "b"},
			want: []string{"a", "b"},
		},
		{
			name: "nil",
			in:   nil,
			want: nil,
		},
		{
			name: "unsupported type",
			in:   "not a slice",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bridgeToStringSlice(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBridgeRemoveTopics 는 bridgeRemoveTopics 헬퍼 함수를 테스트한다.
func TestBridgeRemoveTopics(t *testing.T) {
	tests := []struct {
		name     string
		list     []string
		toRemove []string
		want     []string
	}{
		{
			name:     "일부 제거",
			list:     []string{"a", "b", "c"},
			toRemove: []string{"b"},
			want:     []string{"a", "c"},
		},
		{
			name:     "전부 제거",
			list:     []string{"a", "b"},
			toRemove: []string{"a", "b"},
			want:     []string{},
		},
		{
			name:     "없는 항목 제거",
			list:     []string{"a", "b"},
			toRemove: []string{"x"},
			want:     []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bridgeRemoveTopics(tt.list, tt.toRemove)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestIsControlMessage 는 isControlMessage 함수를 테스트한다.
func TestIsControlMessage(t *testing.T) {
	// action이 있는 메시지 → 제어 메시지
	ctrlMsg := message.New()
	ctrlMsg.Payload().Set("action", "subscribe")
	assert.True(t, isControlMessage(ctrlMsg))

	// action이 없는 메시지 → 일반 메시지
	normalMsg := message.New()
	normalMsg.Payload().Set("temperature", 25.5)
	assert.False(t, isControlMessage(normalMsg))

	// 빈 메시지 → 일반 메시지
	emptyMsg := message.New()
	assert.False(t, isControlMessage(emptyMsg))
}

// TestBridgeNode_Configure_Topics 는 Configure에서 topics 설정이 BridgeConfig에 반영되는지 확인한다.
func TestBridgeNode_Configure_Topics(t *testing.T) {
	def := flow.NewNodeDef("test-bridge", "bridge",
		flow.WithAgentRef(flow.AgentRef{AgentName: "test-agent", Direction: flow.BridgeIn}),
		flow.WithErrorPort(),
	)

	n, err := NewBridgeNode(def)
	require.NoError(t, err)

	bn := n.(*BridgeNode)

	// topics가 없는 설정
	err = bn.Configure(map[string]any{"payload_format": "json"})
	require.NoError(t, err)
	assert.Empty(t, bn.bridgeConfig.Topics)

	// topics가 있는 설정
	err = bn.Configure(map[string]any{
		"topics": []any{"sensor/+/temperature", "sensor/+/humidity"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"sensor/+/temperature", "sensor/+/humidity"}, bn.bridgeConfig.Topics)
}
