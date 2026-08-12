package node

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 테스트 더블
// ---------------------------------------------------------------------------

// fakeReceiverAgent 는 recvCh 로 주입한 레코드를 ReceiveMessage 로 흘려보내는 최소
// 에이전트이다. done 을 close 하면 정지된 에이전트(닫힌 채널)를 모사한다.
type fakeReceiverAgent struct {
	name    string
	recvCh  chan []byte
	done    chan struct{}
	failErr error // non-nil 이면 항상 즉시 실패 반환 (무음 실패 재현).
	calls   atomic.Int64
}

func newFakeReceiverAgent(name string) *fakeReceiverAgent {
	return &fakeReceiverAgent{
		name:   name,
		recvCh: make(chan []byte, 8),
		done:   make(chan struct{}),
	}
}

func (f *fakeReceiverAgent) Init(_ agent.AgentConfig) error      { return nil }
func (f *fakeReceiverAgent) Start(_ context.Context) error       { return nil }
func (f *fakeReceiverAgent) Stop(_ context.Context) error        { return nil }
func (f *fakeReceiverAgent) Pause(_ context.Context) error       { return nil }
func (f *fakeReceiverAgent) Resume(_ context.Context) error      { return nil }
func (f *fakeReceiverAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (f *fakeReceiverAgent) Configure(_ agent.AgentConfig) error { return nil }
func (f *fakeReceiverAgent) ID() string                          { return f.name }
func (f *fakeReceiverAgent) Name() string                        { return f.name }
func (f *fakeReceiverAgent) Type() string                        { return "chirpstack" }
func (f *fakeReceiverAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (f *fakeReceiverAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }
func (f *fakeReceiverAgent) Process(_ []byte) ([]byte, error)    { return nil, nil }

func (f *fakeReceiverAgent) ReceiveMessage(ctx context.Context) ([]byte, error) {
	f.calls.Add(1)
	if f.failErr != nil {
		return nil, f.failErr
	}
	select {
	case b := <-f.recvCh:
		return b, nil
	case <-f.done:
		return nil, errors.New("fake agent: stopped")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// swapAgentResolver 는 런타임에 대상 transport 를 교체할 수 있는 AgentResolver 이다
// (manager.Restart 가 에이전트 인스턴스를 통째로 교체하는 상황 모사).
type swapAgentResolver struct {
	mu        sync.Mutex
	transport AgentTransport
}

func (s *swapAgentResolver) ResolveAgent(_ context.Context, _ flow.AgentRef) (AgentTransport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transport, nil
}

func (s *swapAgentResolver) set(tr AgentTransport) {
	s.mu.Lock()
	s.transport = tr
	s.mu.Unlock()
}

// newTestChirpStackInNode 는 주어진 resolver 에 연결된 chirpstack-in 노드를 만들고
// Init 까지 수행한다.
func newTestChirpStackInNode(t *testing.T, res AgentResolver, opts ...NodeOption) *ChirpStackInNode {
	t.Helper()
	all := append([]NodeOption{WithAgentResolver(res)}, opts...)
	raw, err := NewChirpStackInNode(
		flow.NodeDef{ID: "cs-in-1", Name: "cs-in", Type: "chirpstack-in"},
		all...,
	)
	if err != nil {
		t.Fatalf("NewChirpStackInNode: %v", err)
	}
	n, ok := raw.(*ChirpStackInNode)
	if !ok {
		t.Fatalf("unexpected node type %T", raw)
	}
	if err := n.Configure(map[string]any{"agent_ref": "cs-agent"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := n.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return n
}

// csMeasurementRecord 는 에이전트가 fan-out 하는 per-measurement 레코드 JSON 이다.
func csMeasurementRecord(t *testing.T, measurement string) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"measurement": measurement,
		"value":       21.5,
		"unit_id":     "24e124141d180806",
		"time_ms":     time.Now().UnixMilli(),
	})
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	return b
}

// awaitMessage 는 sourceCh 에서 메시지 1건을 기다린다.
func awaitMessage(t *testing.T, ch <-chan message.Message, timeout time.Duration) message.Message {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(timeout):
		t.Fatalf("%v 안에 sourceCh 로 메시지가 도달하지 않았다", timeout)
		return nil
	}
}

// ---------------------------------------------------------------------------
// FIX-1: AgentReinitializer 구현 + 에이전트 인스턴스 교체 후 수신 재개
// ---------------------------------------------------------------------------

// TestChirpStackInNode_ImplementsAgentReinitializer 는 chirpstack-in 이
// AgentReinitializer 를 만족하는지 검증한다.
//
// Engine.ReinitNodesForAgent 는 이 인터페이스를 구현하지 않은 노드를 조용히
// 건너뛴다 — 미구현 시 에이전트 재시작 후 노드가 영구 무음이 되며 로그도 남지 않는다.
func TestChirpStackInNode_ImplementsAgentReinitializer(t *testing.T) {
	var n any = &ChirpStackInNode{}
	if _, ok := n.(AgentReinitializer); !ok {
		t.Fatal("ChirpStackInNode 가 AgentReinitializer 를 구현하지 않는다 — " +
			"엔진이 에이전트 재시작 시 이 노드를 건너뛴다")
	}
}

// TestChirpStackInNode_ReinitSwitchesToNewAgentInstance 는 에이전트가 새 인스턴스로
// 교체된 뒤 Reinit 이 새 인스턴스의 수신 채널을 소비하는지 검증한다.
//
// 함께 검증되는 것:
//   - stopCh 재생성: 재생성하지 않으면 새 receiveLoop 가 첫 select 에서 즉시 종료되어
//     B 의 레코드가 영원히 도달하지 않는다.
//   - agentName 재캡처: 교체된 인스턴스 이름이 device 그룹 승격에 반영된다.
func TestChirpStackInNode_ReinitSwitchesToNewAgentInstance(t *testing.T) {
	agentA := newFakeReceiverAgent("cs-agent-A")
	res := &swapAgentResolver{transport: &mockMQTTTransport{agent: agentA}}
	n := newTestChirpStackInNode(t, res)
	defer func() { _ = n.Shutdown(context.Background()) }()

	// 기준선: 구 인스턴스 A 의 레코드가 sourceCh 에 도달한다.
	agentA.recvCh <- csMeasurementRecord(t, "temperature")
	if msg := awaitMessage(t, n.SourceCh(), 2*time.Second); msg.Type() != "event" {
		t.Fatalf("type = %q, want event", msg.Type())
	}

	// 에이전트 재시작 모사: 구 인스턴스 정지(done close) + 새 인스턴스로 교체.
	// 이 시점부터 A 의 ReceiveMessage 는 즉시 에러를 반환하고, 노드는 무음이 된다.
	close(agentA.done)
	agentB := newFakeReceiverAgent("cs-agent-B")
	res.set(&mockMQTTTransport{agent: agentB})

	if err := n.Reinit(context.Background()); err != nil {
		t.Fatalf("Reinit: %v", err)
	}

	// 새 인스턴스 B 에 들어온 레코드가 sourceCh 에 도달해야 한다.
	agentB.recvCh <- csMeasurementRecord(t, "humidity")
	msg := awaitMessage(t, n.SourceCh(), 2*time.Second)
	if got, ok := msg.Metadata().Get("measurement"); !ok || got != "humidity" {
		t.Errorf("measurement = %q(ok=%v), want humidity", got, ok)
	}

	// 새 인스턴스가 실제로 소비되고 있어야 한다(구 인스턴스만 폴링하면 0 이다).
	if agentB.calls.Load() == 0 {
		t.Error("Reinit 후 새 에이전트 인스턴스에서 수신을 시도하지 않았다")
	}
}

// TestChirpStackInNode_ReinitStopsPreviousLoop 는 Reinit 이 구 세대 receiveLoop 를
// 확실히 종료시키는지(=구 인스턴스 폴링 중단) 검증한다. 종료되지 않으면 구 루프가
// 닫힌 채널을 상대로 무한 회전하며 CPU 를 태운다.
func TestChirpStackInNode_ReinitStopsPreviousLoop(t *testing.T) {
	agentA := newFakeReceiverAgent("cs-agent-A")
	res := &swapAgentResolver{transport: &mockMQTTTransport{agent: agentA}}
	n := newTestChirpStackInNode(t, res)
	defer func() { _ = n.Shutdown(context.Background()) }()

	close(agentA.done) // A 정지 — 이후 ReceiveMessage 는 즉시 실패.

	agentB := newFakeReceiverAgent("cs-agent-B")
	res.set(&mockMQTTTransport{agent: agentB})
	if err := n.Reinit(context.Background()); err != nil {
		t.Fatalf("Reinit: %v", err)
	}

	// 구 루프가 살아 있으면 A 의 호출 수가 계속 증가한다.
	time.Sleep(50 * time.Millisecond)
	before := agentA.calls.Load()
	time.Sleep(150 * time.Millisecond)
	after := agentA.calls.Load()
	if after != before {
		t.Errorf("구 receiveLoop 가 종료되지 않았다: A 호출 수 %d → %d", before, after)
	}
}

// TestChirpStackInNode_ReinitResolveFailure 는 재해석 실패 시 에러가 전파되는지
// 검증한다 (엔진은 이를 로그로 기록한다).
func TestChirpStackInNode_ReinitResolveFailure(t *testing.T) {
	agentA := newFakeReceiverAgent("cs-agent-A")
	res := &swapAgentResolver{transport: &mockMQTTTransport{agent: agentA}}
	n := newTestChirpStackInNode(t, res)
	defer func() { _ = n.Shutdown(context.Background()) }()

	// MessageReceiver 를 구현하지 않는 에이전트로 교체.
	res.set(&mockMQTTTransport{agent: &mockPlainAgent{}})
	if err := n.Reinit(context.Background()); err == nil {
		t.Error("MessageReceiver 미구현 에이전트로의 Reinit 은 에러여야 한다")
	}
}

// ---------------------------------------------------------------------------
// FIX-4: 수신 실패의 rate-limited 로깅
// ---------------------------------------------------------------------------

// TestReceiveFailureThrottle_IdleTimeoutsDoNotWarn 는 정상 유휴 경로인 5초
// context.DeadlineExceeded 가 실패로 계상되지도, 로그를 범람시키지도 않는지 검증한다.
func TestReceiveFailureThrottle_IdleTimeoutsDoNotWarn(t *testing.T) {
	var throttle receiveFailureThrottle
	now := time.Now()

	for i := 0; i < 100; i++ {
		// 유휴 타임아웃은 실제로 5초 간격으로 발생하므로 경고 간격을 넘어선다.
		now = now.Add(chirpStackReceiveTimeout)
		count, emit := throttle.note(context.DeadlineExceeded, now)
		if emit {
			t.Fatalf("유휴 타임아웃 %d 회차에서 경고가 방출되었다(로그 범람)", i+1)
		}
		if count != 0 {
			t.Fatalf("유휴 타임아웃은 실패로 계상되지 않아야 한다, got %d", count)
		}
	}
}

// TestReceiveFailureThrottle_RealErrorsRateLimited 는 실제 실패가 최초 1회 즉시 경고되고,
// 이후에는 간격당 1회로 제한되며, 연속 실패 횟수가 누적 보고되는지 검증한다.
func TestReceiveFailureThrottle_RealErrorsRateLimited(t *testing.T) {
	var throttle receiveFailureThrottle
	base := time.Now()
	err := errors.New("agent stopped")

	count, emit := throttle.note(err, base)
	if !emit || count != 1 {
		t.Fatalf("첫 실패 = (count=%d, emit=%v), want (1, true)", count, emit)
	}

	// 같은 간격 내의 후속 실패는 경고하지 않지만 카운터는 누적한다.
	emitted := 0
	for i := 0; i < 500; i++ {
		if _, e := throttle.note(err, base.Add(time.Millisecond)); e {
			emitted++
		}
	}
	if emitted != 0 {
		t.Errorf("간격 내 추가 경고 = %d건, want 0 (rate-limit 실패)", emitted)
	}

	// 간격 경과 후에는 다시 1회 경고하며 누적 연속 실패 횟수를 보고한다.
	count, emit = throttle.note(err, base.Add(chirpStackReceiveWarnInterval))
	if !emit {
		t.Fatal("간격 경과 후에는 다시 경고해야 한다")
	}
	if count != 502 {
		t.Errorf("연속 실패 횟수 = %d, want 502", count)
	}

	// 수신 성공 시 카운터가 초기화된다.
	throttle.reset()
	if count, _ := throttle.note(err, base.Add(2*chirpStackReceiveWarnInterval)); count != 1 {
		t.Errorf("reset 후 연속 실패 횟수 = %d, want 1", count)
	}
}

// TestChirpStackInNode_LogsPersistentReceiveFailure 는 수신이 계속 실패할 때 진단
// 로그가 남되(무음 실패 금지), 빠른 회전에도 로그가 범람하지 않는지 검증한다.
func TestChirpStackInNode_LogsPersistentReceiveFailure(t *testing.T) {
	lg := &mockLogger{}
	failing := newFakeReceiverAgent("cs-agent-fail")
	failing.failErr = errors.New("chirpstack: stopped")

	res := &swapAgentResolver{transport: &mockMQTTTransport{agent: failing}}
	n := newTestChirpStackInNode(t, res, WithLogger(lg))

	time.Sleep(100 * time.Millisecond)
	_ = n.Shutdown(context.Background())

	calls := failing.calls.Load()
	if calls < 10 {
		t.Fatalf("수신 시도 = %d회 — 실패 루프가 회전하지 않아 테스트 전제가 성립하지 않는다", calls)
	}

	lg.mu.Lock()
	var warns int
	for _, m := range lg.messages {
		if len(m) >= 5 && m[:5] == "warn:" {
			warns++
		}
	}
	lg.mu.Unlock()

	if warns == 0 {
		t.Errorf("수신이 %d회 연속 실패했는데 경고 로그가 한 줄도 없다(무음 실패)", calls)
	}
	if warns > 2 {
		t.Errorf("경고 로그 = %d건 — rate-limit 이 동작하지 않아 로그가 범람한다", warns)
	}
}
