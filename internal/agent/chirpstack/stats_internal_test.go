package chirpstack

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// newStatsTestAgent 는 통계 검증용 비활성화(연결 없음) ChirpStack 에이전트를 만든다.
//
// Enabled=false 이므로 Init 이 브로커에 연결하지 않는다 — 실제 MQTT 브로커 없이
// enqueue/ReceiveMessage 경계만 결정적으로 구동할 수 있다.
func newStatsTestAgent(t *testing.T, name string, bufferSize int) *ChirpStackAgent {
	t.Helper()
	resetNameRegistryForTest()

	cfg := newTestConfig("id-"+name, name)
	if bufferSize > 0 {
		cfg.Transport.Options = map[string]any{"buffer_size": bufferSize}
	}

	a, err := NewChirpStackAgent(cfg)
	if err != nil {
		t.Fatalf("NewChirpStackAgent: %v", err)
	}
	cs, ok := a.(*ChirpStackAgent)
	if !ok {
		t.Fatalf("타입 단언 실패: %T 는 *ChirpStackAgent 가 아니다", a)
	}
	return cs
}

// TestReceiveMessage_IncrementsInternalSent 는 노드가 recvCh 에서 메시지를 꺼낼 때마다
// "에이전트 → 노드" 내부 발신 카운터가 1 증가하는지 검증한다.
//
// 이 카운터가 없으면 UI 통계의 "내부 / 발신" 이 구조적으로 항상 0 이 되어, 노드로
// 실제 전달된 메시지 수를 볼 수 없다(본 결함의 근본 원인).
func TestReceiveMessage_IncrementsInternalSent(t *testing.T) {
	a := newStatsTestAgent(t, "cs-internal-sent", 16)

	const n = 5
	for i := 0; i < n; i++ {
		a.enqueue([]byte(`{"m":1}`), "app/1/device/x/event/up")
	}

	ctx := context.Background()
	for i := 0; i < n; i++ {
		if _, err := a.ReceiveMessage(ctx); err != nil {
			t.Fatalf("ReceiveMessage(%d): %v", i, err)
		}
	}

	s := a.Stats()
	if s.InternalMessagesSent != n {
		t.Errorf("InternalMessagesSent = %d, want %d", s.InternalMessagesSent, n)
	}
	// IncrInternalMessagesSent 는 총 발신 카운터도 함께 올린다.
	if s.MessagesSent != n {
		t.Errorf("MessagesSent = %d, want %d", s.MessagesSent, n)
	}
}

// TestReceiveMessage_StoppedPathDoesNotIncrement 는 중지(done close) 경로가 내부 발신
// 카운터를 올리지 않는지 검증한다 — 전달되지 않은 메시지를 전달된 것으로 계상하면
// 통계가 거짓이 된다.
func TestReceiveMessage_StoppedPathDoesNotIncrement(t *testing.T) {
	a := newStatsTestAgent(t, "cs-stopped", 4)

	// 버퍼를 비운 상태로 중지해야 select 가 done 분기로 결정적으로 진입한다.
	if err := a.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	if _, err := a.ReceiveMessage(context.Background()); err == nil {
		t.Fatal("중지된 에이전트의 ReceiveMessage 는 에러를 반환해야 한다")
	}

	s := a.Stats()
	if s.InternalMessagesSent != 0 {
		t.Errorf("InternalMessagesSent = %d, want 0 (중지 경로는 전달이 아니다)", s.InternalMessagesSent)
	}
	if s.MessagesSent != 0 {
		t.Errorf("MessagesSent = %d, want 0", s.MessagesSent)
	}
}

// TestReceiveMessage_ContextCancelledDoesNotIncrement 는 컨텍스트 취소 경로가 내부 발신
// 카운터를 올리지 않는지 검증한다.
func TestReceiveMessage_ContextCancelledDoesNotIncrement(t *testing.T) {
	a := newStatsTestAgent(t, "cs-ctx-cancel", 4)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := a.ReceiveMessage(ctx); err == nil {
		t.Fatal("취소된 컨텍스트의 ReceiveMessage 는 에러를 반환해야 한다")
	}

	s := a.Stats()
	if s.InternalMessagesSent != 0 {
		t.Errorf("InternalMessagesSent = %d, want 0 (취소 경로는 전달이 아니다)", s.InternalMessagesSent)
	}
	if s.MessagesSent != 0 {
		t.Errorf("MessagesSent = %d, want 0", s.MessagesSent)
	}
}

// TestExternalAndInternalAxesAreIndependent 는 외부(브로커↔에이전트) 축과
// 내부(에이전트↔노드) 축이 서로 오염되지 않는지 검증한다.
//
// 두 축이 섞이면 "브로커에서 몇 건 받았는지" 와 "노드로 몇 건 넘겼는지" 를 구분할 수
// 없어 숫자의 의미가 사라진다.
func TestExternalAndInternalAxesAreIndependent(t *testing.T) {
	a := newStatsTestAgent(t, "cs-axes", 16)

	const k = 3
	for i := 0; i < k; i++ {
		a.enqueue([]byte(`{"m":1}`), "app/1/device/x/event/up")
	}

	// enqueue 직후: 외부 수신만 올라가고 내부 발신은 아직 0 이다.
	s := a.Stats()
	if s.ExternalMessagesReceived != k {
		t.Errorf("enqueue 후 ExternalMessagesReceived = %d, want %d", s.ExternalMessagesReceived, k)
	}
	if s.InternalMessagesSent != 0 {
		t.Errorf("enqueue 후 InternalMessagesSent = %d, want 0 (아직 노드가 가져가지 않았다)", s.InternalMessagesSent)
	}

	ctx := context.Background()
	for i := 0; i < k; i++ {
		if _, err := a.ReceiveMessage(ctx); err != nil {
			t.Fatalf("ReceiveMessage(%d): %v", i, err)
		}
	}

	// 노드 전달 후: 외부 카운터는 그대로, 내부 발신만 올라간다.
	s = a.Stats()
	if s.ExternalMessagesReceived != k {
		t.Errorf("전달 후 ExternalMessagesReceived = %d, want %d (노드 전달은 외부 축을 바꾸지 않는다)", s.ExternalMessagesReceived, k)
	}
	if s.ExternalMessagesSent != 0 {
		t.Errorf("전달 후 ExternalMessagesSent = %d, want 0 (노드 전달은 브로커 발행이 아니다)", s.ExternalMessagesSent)
	}
	if s.InternalMessagesSent != k {
		t.Errorf("전달 후 InternalMessagesSent = %d, want %d", s.InternalMessagesSent, k)
	}
	if s.InternalMessagesReceived != 0 {
		t.Errorf("전달 후 InternalMessagesReceived = %d, want 0 (수신 경로는 노드→에이전트가 아니다)", s.InternalMessagesReceived)
	}
}

// TestChirpStackAgent_ImplementsBufferInfoProvider 는 선택 인터페이스 구현을 검증한다.
func TestChirpStackAgent_ImplementsBufferInfoProvider(t *testing.T) {
	a := newStatsTestAgent(t, "cs-bufiface", 8)

	var ag agent.Agent = a
	if _, ok := ag.(agent.BufferInfoProvider); !ok {
		t.Fatal("ChirpStackAgent 가 agent.BufferInfoProvider 를 만족하지 않는다")
	}
}

// TestBufferInfo_ReportsPendingAndCapacity 는 BufferInfo 가 recvCh 의 실제 적체량과
// 용량을 보고하는지 검증한다.
func TestBufferInfo_ReportsPendingAndCapacity(t *testing.T) {
	const capacity = 8
	a := newStatsTestAgent(t, "cs-bufinfo", capacity)

	pending, capV := a.BufferInfo()
	if pending != 0 {
		t.Errorf("초기 pending = %d, want 0", pending)
	}
	if capV != capacity {
		t.Errorf("capacity = %d, want %d", capV, capacity)
	}

	const k = 3
	for i := 0; i < k; i++ {
		a.enqueue([]byte(`{"m":1}`), "app/1/device/x/event/up")
	}

	pending, capV = a.BufferInfo()
	if pending != k {
		t.Errorf("적체 후 pending = %d, want %d", pending, k)
	}
	if capV != capacity {
		t.Errorf("적체 후 capacity = %d, want %d", capV, capacity)
	}

	// 드레인하면 pending 이 줄어든다.
	ctx := context.Background()
	for i := 0; i < k; i++ {
		if _, err := a.ReceiveMessage(ctx); err != nil {
			t.Fatalf("ReceiveMessage(%d): %v", i, err)
		}
	}
	if pending, _ = a.BufferInfo(); pending != 0 {
		t.Errorf("드레인 후 pending = %d, want 0", pending)
	}
}

// TestStats_IncludesBufferInfo 는 Stats 스냅샷이 버퍼 사용량을 담는지 검증한다.
// REST DTO(AgentStatsInfo.Buffer)가 이 필드를 그대로 읽으므로, 여기가 비면 API 응답도
// 항상 0 이 된다.
func TestStats_IncludesBufferInfo(t *testing.T) {
	const capacity = 8
	a := newStatsTestAgent(t, "cs-statsbuf", capacity)

	const k = 2
	for i := 0; i < k; i++ {
		a.enqueue([]byte(`{"m":1}`), "app/1/device/x/event/up")
	}

	s := a.Stats()
	if s.MsgBufferPending != k {
		t.Errorf("MsgBufferPending = %d, want %d", s.MsgBufferPending, k)
	}
	if s.MsgBufferCapacity != capacity {
		t.Errorf("MsgBufferCapacity = %d, want %d", s.MsgBufferCapacity, capacity)
	}
}

// TestReceiveMessage_ConcurrentEnqueueReceive 는 동시 enqueue/수신에서 카운터와
// BufferInfo 접근에 데이터 레이스가 없는지 검증한다 (-race 로 실행).
func TestReceiveMessage_ConcurrentEnqueueReceive(t *testing.T) {
	// enqueue 는 버퍼가 가득 차면 드롭하는 non-blocking 전송이므로, 용량을 총 건수보다
	// 크게 잡아 드롭이 없는 조건에서 카운터 일치를 검증한다(드롭 여부가 아니라 레이스
	// 여부가 이 테스트의 관심사다).
	const total = 200
	a := newStatsTestAgent(t, "cs-race", total+56)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < total; i++ {
			a.enqueue([]byte(`{"m":1}`), "app/1/device/x/event/up")
			// BufferInfo 를 생산 경로와 동시에 읽어 레이스 여부를 확인한다.
			_, _ = a.BufferInfo()
		}
	}()

	received := 0
	go func() {
		defer wg.Done()
		for received < total {
			if _, err := a.ReceiveMessage(ctx); err != nil {
				return
			}
			received++
		}
	}()

	wg.Wait()

	if received != total {
		t.Fatalf("수신 건수 = %d, want %d", received, total)
	}
	s := a.Stats()
	if s.ExternalMessagesErrored != 0 {
		t.Fatalf("드롭 발생 = %d, want 0 (용량이 총 건수보다 커야 한다)", s.ExternalMessagesErrored)
	}
	if s.InternalMessagesSent != int64(received) {
		t.Errorf("InternalMessagesSent = %d, want %d", s.InternalMessagesSent, received)
	}
}
