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
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 회귀 테스트: SerialOutNode.Process 가 context 를 존중한다
//
// 근본 결함: SerialOutNode.Process 가 받은 context 를 무시하고 agent.Process 를
// 타임아웃 없이 동기 호출했다. SerialAgent.Process → framer.Write(port, data) 로
// 이어지는데, go.bug.st/serial 에는 SetWriteDeadline 이 없고 흐름제어 옵션도 없어
// 컨버터/장비가 write 를 빼가지 못하면 시리얼 write 가 영구 블록된다. 그러면
// Process 가 반환하지 못해 상류(serial-to-ethernet 브리지의 tcp-in 등)가 stall 된다.
//
// 수정: agent.Process 를 goroutine 으로 감싸고 ctx.Done() 을 select 한다(callAgentProcess).
// StopFlow 가 nodeCtx 를 cancel(또는 deadline 초과)하면 Process 가 상한 시간 내 반환한다.
// (tcp_io.go / samsung callAgentProcess 와 동일 패턴).
// ---------------------------------------------------------------------------

// blockingSerialAgent 는 Process 가 release 채널이 닫힐 때까지 블록되는 테스트용 Agent 이다.
// 시리얼 write 가 무한 블록되는 상황(deadline 부재)을 재현한다.
type blockingSerialAgent struct {
	release      chan struct{}
	processBegan chan struct{}
	once         sync.Once
}

func newBlockingSerialAgent() *blockingSerialAgent {
	return &blockingSerialAgent{
		release:      make(chan struct{}),
		processBegan: make(chan struct{}, 1),
	}
}

func (m *blockingSerialAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *blockingSerialAgent) Start(_ context.Context) error       { return nil }
func (m *blockingSerialAgent) Stop(_ context.Context) error        { return nil }
func (m *blockingSerialAgent) Pause(_ context.Context) error       { return nil }
func (m *blockingSerialAgent) Resume(_ context.Context) error      { return nil }
func (m *blockingSerialAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *blockingSerialAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *blockingSerialAgent) ID() string                          { return "blocking-serial" }
func (m *blockingSerialAgent) Name() string                        { return "blocking-serial" }
func (m *blockingSerialAgent) Type() string                        { return "serial" }
func (m *blockingSerialAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *blockingSerialAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }

// Process 는 release 가 닫히거나 5초가 지날 때까지 블록한다 (blocking 시리얼 write 재현).
func (m *blockingSerialAgent) Process(_ []byte) ([]byte, error) {
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

func (m *blockingSerialAgent) releaseNow() {
	m.once.Do(func() { close(m.release) })
}

// newSerialRawMsg 는 raw 페이로드를 가진 메시지를 생성한다.
func newSerialRawMsg() message.Message {
	msg := message.New()
	msg.Payload().Set("raw", []byte{0x01, 0x02, 0x03})
	return msg
}

// TestSerialOutNode_Process_ReturnsOnCtxCancel 은 ctx 취소 시 blocking 한 agent.Process
// 로부터 Process 가 상한 시간 내에 반환하는지 검증한다 (StopFlow cancel 경로).
func TestSerialOutNode_Process_ReturnsOnCtxCancel(t *testing.T) {
	ag := newBlockingSerialAgent()
	defer ag.releaseNow()
	n := newTestSerialOutNode(ag)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := n.Process(ctx, newSerialRawMsg())
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

// TestSerialOutNode_Process_ReturnsOnDeadline 은 ctx deadline 초과 시 Process 가 상한
// 시간 내에 반환하는지 검증한다.
func TestSerialOutNode_Process_ReturnsOnDeadline(t *testing.T) {
	ag := newBlockingSerialAgent()
	defer ag.releaseNow()
	n := newTestSerialOutNode(ag)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := n.Process(ctx, newSerialRawMsg())
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

// TestSerialOutNode_Process_WriteTimeout_NoIncomingDeadline 은 핵심 보강 테스트다.
// incoming ctx = context.Background()(deadline 없음, 정상 운행 시뮬레이션)이고 agent.Process
// 가 블록될 때, SerialOutNode.Process 가 노드 레벨 write_timeout 내에 context.DeadlineExceeded
// 를 반환하는지 검증한다.
//
// 수정 전(ctx 만 존중, 자체 타임아웃 없음): incoming ctx 가 취소되지 않으면 시리얼 write 가
// 무한 블록 → Process 미반환 → 상류 stall. 정상 운행 경로가 안 고쳐진다.
// 수정 후(samsung 패턴, per-call write_timeout): incoming ctx 에 deadline 이 없어도 노드가
// write_timeout 내에 DeadlineExceeded 로 반환한다.
func TestSerialOutNode_Process_WriteTimeout_NoIncomingDeadline(t *testing.T) {
	ag := newBlockingSerialAgent()
	defer ag.releaseNow()
	n := newTestSerialOutNode(ag)
	// 짧은 write_timeout 설정 (deadline 없는 background ctx 로 진행).
	require.NoError(t, n.Configure(map[string]any{
		"agent_ref":     "serial-agent-1",
		"write_timeout": "50ms",
	}))

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		// incoming ctx 에 deadline/취소 없음 — 정상 운행 중 시뮬레이션.
		_, err := n.Process(context.Background(), newSerialRawMsg())
		done <- err
	}()

	select {
	case err := <-done:
		require.Error(t, err, "write_timeout 초과 시 Process 는 에러를 반환해야 한다")
		require.True(t, errors.Is(err, context.DeadlineExceeded),
			"에러는 context.DeadlineExceeded 를 감싸야 한다: %v", err)
		require.GreaterOrEqual(t, time.Since(start), 40*time.Millisecond,
			"write_timeout(50ms) 이전에 반환하면 안 된다")
		require.Less(t, time.Since(start), 2*time.Second,
			"incoming ctx 에 deadline 이 없어도 write_timeout 내에 반환해야 한다")
	case <-time.After(2 * time.Second):
		t.Fatal("background ctx + blocking agent 에서 Process 가 반환하지 않음 — 자체 타임아웃 없음(버그 미보강)")
	}
}

// TestSerialOutNode_Configure_WriteTimeout 은 write_timeout 설정이 파싱·배선되는지와
// 잘못된 값이 Configure 단계에서 거부되는지 검증한다.
func TestSerialOutNode_Configure_WriteTimeout(t *testing.T) {
	// 유효한 값 파싱.
	n := newTestSerialOutNode(&mockSerialAgent{})
	require.NoError(t, n.Configure(map[string]any{
		"agent_ref":     "serial-agent-1",
		"write_timeout": "250ms",
	}))
	require.Equal(t, 250*time.Millisecond, n.writeTimeout)

	// 미설정 시 기본값 5s.
	n2 := newTestSerialOutNode(&mockSerialAgent{})
	require.NoError(t, n2.Configure(map[string]any{
		"agent_ref": "serial-agent-1",
	}))
	require.Equal(t, serialDefaultWriteTimeout, n2.writeTimeout)

	// 잘못된 형식 거부.
	n3 := newTestSerialOutNode(&mockSerialAgent{})
	require.Error(t, n3.Configure(map[string]any{
		"agent_ref":     "serial-agent-1",
		"write_timeout": "bogus",
	}))

	// 0/음수 거부.
	n4 := newTestSerialOutNode(&mockSerialAgent{})
	require.Error(t, n4.Configure(map[string]any{
		"agent_ref":     "serial-agent-1",
		"write_timeout": "0s",
	}))
}

// TestSerialOutNode_Process_NormalPath_NotBlocked 은 정상(non-blocking) agent 에서
// Process 가 정상 완료되고 데이터가 그대로 전달·pass-through 되는지(정상 경로 회귀 없음)
// 검증한다.
func TestSerialOutNode_Process_NormalPath_NotBlocked(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	n := newTestSerialOutNode(mockAgent)

	results, err := n.Process(context.Background(), newSerialRawMsg())
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.True(t, mockAgent.processCalled, "정상 경로에서 agent.Process 가 호출되어야 한다")
	assert.Equal(t, []byte{0x01, 0x02, 0x03}, mockAgent.processData,
		"정상 경로에서 raw 바이트가 변경 없이 agent 로 전달되어야 한다")

	// pass-through: 입력 메시지가 출력으로 전달되고 node_id 메타데이터가 부여된다.
	nodeID, ok := results[0].Metadata().Get("node_id")
	assert.True(t, ok)
	assert.NotEmpty(t, nodeID)
}
