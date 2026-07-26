package node

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 회귀 테스트: TCPOutNode.Process 가 context 를 존중한다
//
// 근본 결함: TCPOutNode.Process 가 받은 context 를 무시하고 agent.Process 를 타임아웃
// 없이 동기 호출했다. socket 트랜스포트가 stale 클라이언트로 blocking 되면 Process 가
// 무한 대기하여 runNode 가 wg 를 놓지 못하고 StopFlow 30초 상한을 소진한다.
//
// 수정: agent.Process 를 goroutine 으로 감싸고 ctx.Done() 을 select 한다. StopFlow 가
// nodeCtx 를 cancel(또는 deadline 초과)하면 Process 가 상한 시간 내에 즉시 반환한다.
// (samsung callAgentProcess 와 동일 패턴).
// ---------------------------------------------------------------------------

// blockingTCPAgent 는 Process 가 release 채널이 닫힐 때까지 블록되는 테스트용 Agent 이다.
// stale 소켓으로 conn.Write 가 무한 블록되는 상황을 재현한다.
type blockingTCPAgent struct {
	release      chan struct{}
	processBegan chan struct{}
	once         sync.Once
}

func newBlockingTCPAgent() *blockingTCPAgent {
	return &blockingTCPAgent{
		release:      make(chan struct{}),
		processBegan: make(chan struct{}, 1),
	}
}

func (m *blockingTCPAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *blockingTCPAgent) Start(_ context.Context) error       { return nil }
func (m *blockingTCPAgent) Stop(_ context.Context) error        { return nil }
func (m *blockingTCPAgent) Pause(_ context.Context) error       { return nil }
func (m *blockingTCPAgent) Resume(_ context.Context) error      { return nil }
func (m *blockingTCPAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *blockingTCPAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *blockingTCPAgent) ID() string                          { return "blocking-tcp" }
func (m *blockingTCPAgent) Name() string                        { return "blocking-tcp" }
func (m *blockingTCPAgent) Type() string                        { return "tcp-server" }
func (m *blockingTCPAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *blockingTCPAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }

// Process 는 release 가 닫히거나 5초가 지날 때까지 블록한다 (blocking conn.Write 재현).
func (m *blockingTCPAgent) Process(_ []byte) ([]byte, error) {
	select {
	case m.processBegan <- struct{}{}:
	default:
	}
	select {
	case <-m.release:
	case <-time.After(5 * time.Second):
	}
	return nil, nil
}

func (m *blockingTCPAgent) releaseNow() {
	m.once.Do(func() { close(m.release) })
}

// newRawMsg 는 raw 페이로드를 가진 메시지를 생성한다.
func newRawMsg() message.Message {
	msg := message.New()
	msg.Payload().Set("raw", []byte{0x01, 0x02, 0x03})
	return msg
}

// TestTCPOutNode_Process_ReturnsOnCtxCancel 은 ctx 취소 시 blocking 한 agent.Process
// 로부터 Process 가 상한 시간 내에 반환하는지 검증한다 (StopFlow cancel 경로).
func TestTCPOutNode_Process_ReturnsOnCtxCancel(t *testing.T) {
	ag := newBlockingTCPAgent()
	defer ag.releaseNow()
	n := newTestTCPOutNode(ag)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := n.Process(ctx, newRawMsg())
		done <- err
	}()

	// Process 가 agent.Process 에 진입할 때까지 대기한 뒤 cancel.
	select {
	case <-ag.processBegan:
	case <-time.After(2 * time.Second):
		t.Fatal("agent.Process 가 호출되지 않음")
	}
	cancel()

	select {
	case err := <-done:
		require.Error(t, err, "ctx 취소 시 Process 는 에러를 반환해야 한다")
		require.True(t, errors.Is(err, context.Canceled),
			"에러는 context.Canceled 를 감싸야 한다: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("ctx 취소 후에도 Process 가 반환하지 않음 — ctx 무시 (버그 미수정)")
	}
}

// TestTCPOutNode_Process_ReturnsOnDeadline 은 ctx deadline 초과 시 Process 가 상한
// 시간 내에 반환하는지 검증한다.
func TestTCPOutNode_Process_ReturnsOnDeadline(t *testing.T) {
	ag := newBlockingTCPAgent()
	defer ag.releaseNow()
	n := newTestTCPOutNode(ag)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := n.Process(ctx, newRawMsg())
		done <- err
	}()

	select {
	case err := <-done:
		require.Error(t, err)
		require.True(t, errors.Is(err, context.DeadlineExceeded),
			"에러는 context.DeadlineExceeded 를 감싸야 한다: %v", err)
		require.Less(t, time.Since(start), 2*time.Second,
			"Process 는 deadline 근처에서 반환해야 한다")
	case <-time.After(2 * time.Second):
		t.Fatal("deadline 초과 후에도 Process 가 반환하지 않음")
	}
}

// TestTCPOutNode_Process_NormalPath_NotBlocked 은 정상(non-blocking) agent 에서
// Process 가 정상 완료되고 결과를 반환하는지(정상 경로 회귀 없음) 검증한다.
func TestTCPOutNode_Process_NormalPath_NotBlocked(t *testing.T) {
	mockAgent := &mockTCPServerAgent{}
	n := newTestTCPOutNode(mockAgent)

	results, err := n.Process(context.Background(), newRawMsg())
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.True(t, mockAgent.processCalled, "정상 경로에서 agent.Process 가 호출되어야 한다")
}
