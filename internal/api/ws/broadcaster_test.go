package ws

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/engine"
)

// --- 테스트용 헬퍼 타입 ---

// mockMetricsSource 는 MetricsSource 인터페이스를 구현하는 테스트용 목 구조체이다.
type mockMetricsSource struct {
	mu    sync.Mutex
	flows []engine.FlowStatus
}

func (m *mockMetricsSource) ListFlows() []engine.FlowStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]engine.FlowStatus, len(m.flows))
	copy(cp, m.flows)
	return cp
}

func (m *mockMetricsSource) setFlows(flows []engine.FlowStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flows = flows
}

// mockStreamRouter 는 observe.StreamRouter 인터페이스를 구현하는 테스트용 목 구조체이다.
type mockStreamRouter struct {
	mu       sync.Mutex
	routes   map[string][]io.Writer
	defWrite io.Writer
}

func newMockStreamRouter() *mockStreamRouter {
	return &mockStreamRouter{
		routes: make(map[string][]io.Writer),
	}
}

func (m *mockStreamRouter) AddRoute(component string, writer io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routes[component] = append(m.routes[component], writer)
}

func (m *mockStreamRouter) RemoveRoute(component string, writer io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	writers := m.routes[component]
	for i, w := range writers {
		if w == writer {
			m.routes[component] = append(writers[:i], writers[i+1:]...)
			return
		}
	}
}

func (m *mockStreamRouter) Routes(component string) []io.Writer {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]io.Writer, len(m.routes[component]))
	copy(cp, m.routes[component])
	return cp
}

func (m *mockStreamRouter) SetDefaultWriter(writer io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defWrite = writer
}

func (m *mockStreamRouter) Handler() slog.Handler {
	return nil
}

func (m *mockStreamRouter) routeCount(component string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.routes[component])
}

// newTestHub 는 테스트용 Hub 를 생성하고 Run 고루틴을 시작한다.
// 반환된 cleanup 함수를 defer 로 호출하여 정리한다.
func newTestHub(t *testing.T) (*Hub, func()) {
	t.Helper()
	hub := NewHub(nil)
	go hub.Run()
	return hub, func() { hub.Stop() }
}

// --- TestNewMonitoringBroadcaster ---

func TestNewMonitoringBroadcaster(t *testing.T) {
	t.Parallel()

	hub, cleanup := newTestHub(t)
	defer cleanup()

	src := &mockMetricsSource{}

	t.Run("기본값", func(t *testing.T) {
		t.Parallel()
		b := NewMonitoringBroadcaster(hub, src, nil)
		if b == nil {
			t.Fatal("NewMonitoringBroadcaster 가 nil 을 반환함")
		}
		if b.interval != 1*time.Second {
			t.Errorf("기본 interval 이 1s 가 아님: got %v", b.interval)
		}
		if b.hub != hub {
			t.Error("hub 가 올바르게 설정되지 않음")
		}
		if b.engine != src {
			t.Error("engine 이 올바르게 설정되지 않음")
		}
		if b.streams != nil {
			t.Error("streams 가 nil 이어야 하지만 설정됨")
		}
	})

	t.Run("사용자_정의_옵션", func(t *testing.T) {
		t.Parallel()
		customInterval := 500 * time.Millisecond
		sr := newMockStreamRouter()

		b := NewMonitoringBroadcaster(hub, src, nil,
			WithBroadcastInterval(customInterval),
			WithStreamRouter(sr),
		)

		if b.interval != customInterval {
			t.Errorf("사용자 정의 interval 이 적용되지 않음: got %v, want %v", b.interval, customInterval)
		}
		if b.streams != sr {
			t.Error("streams 가 올바르게 설정되지 않음")
		}
	})

	t.Run("nil_logger_시_기본_로거_사용", func(t *testing.T) {
		t.Parallel()
		b := NewMonitoringBroadcaster(hub, src, nil)
		if b.logger == nil {
			t.Error("nil logger 전달 시 기본 로거가 설정되어야 함")
		}
	})
}

// --- TestWithBroadcastInterval ---

func TestWithBroadcastInterval(t *testing.T) {
	t.Parallel()

	hub, cleanup := newTestHub(t)
	defer cleanup()

	src := &mockMetricsSource{}

	tests := []struct {
		name     string
		input    time.Duration
		expected time.Duration
	}{
		{"양수_간격", 2 * time.Second, 2 * time.Second},
		{"밀리초_간격", 100 * time.Millisecond, 100 * time.Millisecond},
		{"0_이하_무시", 0, 1 * time.Second},
		{"음수_무시", -1 * time.Second, 1 * time.Second},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := NewMonitoringBroadcaster(hub, src, nil, WithBroadcastInterval(tc.input))
			if b.interval != tc.expected {
				t.Errorf("interval = %v, want %v", b.interval, tc.expected)
			}
		})
	}
}

// --- TestMonitoringBroadcaster_StartStop ---

func TestMonitoringBroadcaster_StartStop(t *testing.T) {
	t.Parallel()

	hub, cleanup := newTestHub(t)
	defer cleanup()

	src := &mockMetricsSource{
		flows: []engine.FlowStatus{
			{FlowID: "f1", MessageCount: 10, ErrorCount: 1},
		},
	}

	b := NewMonitoringBroadcaster(hub, src, nil,
		WithBroadcastInterval(10*time.Millisecond),
	)

	// Start 가 정상적으로 작동하는지 확인
	ctx := context.Background()
	b.Start(ctx)

	// 고루틴이 실행 중인지 확인하기 위해 짧은 시간 대기
	time.Sleep(50 * time.Millisecond)

	// Stop 이 블로킹 없이 정상 종료되는지 확인
	done := make(chan struct{})
	go func() {
		b.Stop()
		close(done)
	}()

	select {
	case <-done:
		// 정상 종료
	case <-time.After(2 * time.Second):
		t.Fatal("Stop 이 2초 내에 완료되지 않음 (교착 상태 가능성)")
	}
}

func TestMonitoringBroadcaster_StartStop_ParentContextCancel(t *testing.T) {
	t.Parallel()

	hub, cleanup := newTestHub(t)
	defer cleanup()

	src := &mockMetricsSource{}
	b := NewMonitoringBroadcaster(hub, src, nil,
		WithBroadcastInterval(10*time.Millisecond),
	)

	ctx, cancel := context.WithCancel(context.Background())
	b.Start(ctx)

	// 부모 컨텍스트 취소로 종료
	cancel()

	done := make(chan struct{})
	go func() {
		b.Stop()
		close(done)
	}()

	select {
	case <-done:
		// 정상 종료
	case <-time.After(2 * time.Second):
		t.Fatal("부모 컨텍스트 취소 후 Stop 이 완료되지 않음")
	}
}

// --- TestMonitoringBroadcaster_TickSkipsWhenNoClients ---

func TestMonitoringBroadcaster_TickSkipsWhenNoClients(t *testing.T) {
	t.Parallel()

	hub, cleanup := newTestHub(t)
	defer cleanup()

	// ListFlows 호출 여부를 추적하는 목
	callCount := 0
	src := &countingMetricsSource{
		flows:     []engine.FlowStatus{{FlowID: "f1", MessageCount: 100}},
		callCount: &callCount,
	}

	b := NewMonitoringBroadcaster(hub, src, nil,
		WithBroadcastInterval(10*time.Millisecond),
	)

	ctx := context.Background()
	b.Start(ctx)

	// 클라이언트 없이 여러 틱이 실행될 시간 대기
	time.Sleep(80 * time.Millisecond)

	b.Stop()

	// ClientCount 가 0 이므로 tick() 에서 ListFlows 를 호출하지 않아야 함
	// initCounters() 에서 한 번 호출되므로 최대 1 이어야 함
	if callCount > 1 {
		t.Errorf("클라이언트 없을 때 ListFlows 가 %d 번 호출됨 (initCounters 이후 추가 호출 없어야 함)", callCount)
	}
}

// countingMetricsSource 는 ListFlows 호출 횟수를 추적하는 목 구조체이다.
type countingMetricsSource struct {
	mu        sync.Mutex
	flows     []engine.FlowStatus
	callCount *int
}

func (c *countingMetricsSource) ListFlows() []engine.FlowStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	*c.callCount++
	cp := make([]engine.FlowStatus, len(c.flows))
	copy(cp, c.flows)
	return cp
}

// --- TestMonitoringBroadcaster_TickBroadcastsMetrics ---

func TestMonitoringBroadcaster_TickBroadcastsMetrics(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil)
	go hub.Run()
	defer hub.Stop()

	src := &mockMetricsSource{
		flows: []engine.FlowStatus{
			{FlowID: "f1", MessageCount: 50, ErrorCount: 5},
		},
	}

	b := NewMonitoringBroadcaster(hub, src, nil,
		WithBroadcastInterval(15*time.Millisecond),
	)

	// 테스트용 가짜 클라이언트를 등록하여 ClientCount > 0 으로 만든다
	sendCh := make(chan []byte, 256)
	fakeClient := &Client{
		hub:    hub,
		send:   sendCh,
		logger: slog.Default(),
	}
	hub.Register(fakeClient)

	// 등록이 처리될 시간 대기
	time.Sleep(20 * time.Millisecond)

	if hub.ClientCount() == 0 {
		t.Fatal("테스트 클라이언트 등록 실패")
	}

	ctx := context.Background()
	b.Start(ctx)

	// 최소 한 번의 틱이 실행될 시간 대기
	var received []byte
	select {
	case received = <-sendCh:
		// 메시지 수신 성공
	case <-time.After(2 * time.Second):
		t.Fatal("2초 내에 메트릭 메시지를 수신하지 못함")
	}

	b.Stop()

	// 수신된 메시지 검증
	var msg Message
	if err := json.Unmarshal(received, &msg); err != nil {
		t.Fatalf("메시지 JSON 파싱 실패: %v", err)
	}

	if msg.Type != TypeFlowMetrics {
		t.Errorf("메시지 타입 = %q, want %q", msg.Type, TypeFlowMetrics)
	}

	var payload map[string]float64
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatalf("페이로드 JSON 파싱 실패: %v", err)
	}

	// cpu, memory, throughput, error_rate 키가 존재하는지 확인
	requiredKeys := []string{"cpu", "memory", "throughput", "error_rate"}
	for _, key := range requiredKeys {
		if _, ok := payload[key]; !ok {
			t.Errorf("페이로드에 %q 키가 없음", key)
		}
	}
}

// --- TestMonitoringBroadcaster_ThroughputCalculation ---

func TestMonitoringBroadcaster_ThroughputCalculation(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil)
	go hub.Run()
	defer hub.Stop()

	// 초기: MessageCount=100, ErrorCount=10
	src := &mockMetricsSource{
		flows: []engine.FlowStatus{
			{FlowID: "f1", MessageCount: 100, ErrorCount: 10},
		},
	}

	b := NewMonitoringBroadcaster(hub, src, nil,
		WithBroadcastInterval(20*time.Millisecond),
	)

	// 가짜 클라이언트 등록
	sendCh := make(chan []byte, 256)
	fakeClient := &Client{
		hub:    hub,
		send:   sendCh,
		logger: slog.Default(),
	}
	hub.Register(fakeClient)
	time.Sleep(20 * time.Millisecond)

	ctx := context.Background()
	b.Start(ctx)

	// 첫 번째 틱 메시지 수신 (initCounters 에서 설정한 기준 대비 델타)
	select {
	case <-sendCh:
		// 첫 번째 메시지 버림 (0 델타)
	case <-time.After(2 * time.Second):
		t.Fatal("첫 번째 메트릭 수신 실패")
	}

	// 메시지 카운트를 증가시킨다 (델타: 50 메시지, 5 에러)
	src.setFlows([]engine.FlowStatus{
		{FlowID: "f1", MessageCount: 150, ErrorCount: 15},
	})

	// 두 번째 틱 메시지 수신
	var received []byte
	select {
	case received = <-sendCh:
		// 성공
	case <-time.After(2 * time.Second):
		t.Fatal("두 번째 메트릭 수신 실패")
	}

	b.Stop()

	var msg Message
	if err := json.Unmarshal(received, &msg); err != nil {
		t.Fatalf("메시지 파싱 실패: %v", err)
	}

	var payload map[string]float64
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatalf("페이로드 파싱 실패: %v", err)
	}

	// throughput 은 deltaMsg / elapsed 이므로 양수여야 함
	if payload["throughput"] <= 0 {
		t.Errorf("throughput 이 양수여야 함: got %f", payload["throughput"])
	}
}

// --- TestMonitoringBroadcaster_ErrorRateCalculation ---

func TestMonitoringBroadcaster_ErrorRateCalculation(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil)
	go hub.Run()
	defer hub.Stop()

	t.Run("0으로_나누기_방지_deltaMsg_0", func(t *testing.T) {
		// MessageCount 가 변하지 않으면 (deltaMsg=0) errorRate 는 0 이어야 함
		src := &mockMetricsSource{
			flows: []engine.FlowStatus{
				{FlowID: "f1", MessageCount: 100, ErrorCount: 10},
			},
		}

		b := NewMonitoringBroadcaster(hub, src, nil,
			WithBroadcastInterval(15*time.Millisecond),
		)

		sendCh := make(chan []byte, 256)
		fakeClient := &Client{
			hub:    hub,
			send:   sendCh,
			logger: slog.Default(),
		}
		hub.Register(fakeClient)
		time.Sleep(20 * time.Millisecond)

		ctx := context.Background()
		b.Start(ctx)

		// 틱 메시지 수신 (deltaMsg=0 이므로 errorRate=0)
		var received []byte
		select {
		case received = <-sendCh:
		case <-time.After(2 * time.Second):
			t.Fatal("메트릭 수신 실패")
		}

		b.Stop()

		var msg Message
		if err := json.Unmarshal(received, &msg); err != nil {
			t.Fatalf("메시지 파싱 실패: %v", err)
		}

		var payload map[string]float64
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			t.Fatalf("페이로드 파싱 실패: %v", err)
		}

		// deltaMsg 가 0 이므로 errorRate 는 0
		if payload["error_rate"] != 0 {
			t.Errorf("deltaMsg=0 일 때 error_rate = %f, want 0", payload["error_rate"])
		}
	})
}

// --- TestMonitoringBroadcaster_WithStreamRouter ---

func TestMonitoringBroadcaster_WithStreamRouter(t *testing.T) {
	t.Parallel()

	hub, cleanup := newTestHub(t)
	defer cleanup()

	src := &mockMetricsSource{}
	sr := newMockStreamRouter()

	b := NewMonitoringBroadcaster(hub, src, nil,
		WithBroadcastInterval(10*time.Millisecond),
		WithStreamRouter(sr),
	)

	// Start 전에는 defaultWriter 가 설정되지 않아야 함
	sr.mu.Lock()
	preDW := sr.defWrite
	sr.mu.Unlock()
	if preDW != nil {
		t.Error("Start 전에 defaultWriter 가 설정되어 있음")
	}

	ctx := context.Background()
	b.Start(ctx)

	// Start 후 defaultWriter 가 io.MultiWriter 로 교체되어야 함
	sr.mu.Lock()
	postDW := sr.defWrite
	sr.mu.Unlock()
	if postDW == nil {
		t.Error("Start 후 defaultWriter 가 nil")
	}

	if b.logWriter == nil {
		t.Error("Start 후 logWriter 가 nil")
	}

	b.Stop()

	// Stop 후 defaultWriter 가 os.Stdout 으로 복원되어야 함
	sr.mu.Lock()
	stopDW := sr.defWrite
	sr.mu.Unlock()
	if stopDW != os.Stdout {
		t.Error("Stop 후 defaultWriter 가 os.Stdout 이 아님")
	}

	if b.logWriter != nil {
		t.Error("Stop 후 logWriter 가 nil 이 아님")
	}
}

func TestMonitoringBroadcaster_WithStreamRouter_Nil(t *testing.T) {
	t.Parallel()

	hub, cleanup := newTestHub(t)
	defer cleanup()

	src := &mockMetricsSource{}

	// StreamRouter 없이 생성
	b := NewMonitoringBroadcaster(hub, src, nil,
		WithBroadcastInterval(10*time.Millisecond),
	)

	ctx := context.Background()
	b.Start(ctx)

	// logWriter 가 생성되지 않아야 함
	if b.logWriter != nil {
		t.Error("StreamRouter 없이 logWriter 가 생성됨")
	}

	b.Stop()
}

// --- TestMonitoringBroadcaster_InitCounters ---

func TestMonitoringBroadcaster_InitCounters(t *testing.T) {
	t.Parallel()

	hub, cleanup := newTestHub(t)
	defer cleanup()

	src := &mockMetricsSource{
		flows: []engine.FlowStatus{
			{FlowID: "f1", MessageCount: 100, ErrorCount: 10},
			{FlowID: "f2", MessageCount: 200, ErrorCount: 20},
		},
	}

	b := NewMonitoringBroadcaster(hub, src, nil)

	ctx := context.Background()
	b.Start(ctx)

	// initCounters 에서 합산된 값 확인
	if b.prevMsgCount != 300 {
		t.Errorf("prevMsgCount = %d, want 300", b.prevMsgCount)
	}
	if b.prevErrCount != 30 {
		t.Errorf("prevErrCount = %d, want 30", b.prevErrCount)
	}

	b.Stop()
}

// --- TestMonitoringBroadcaster_MultipleFlows ---

func TestMonitoringBroadcaster_MultipleFlows(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil)
	go hub.Run()
	defer hub.Stop()

	// 여러 플로우가 있는 경우 메트릭이 올바르게 집계되는지 확인
	src := &mockMetricsSource{
		flows: []engine.FlowStatus{
			{FlowID: "f1", MessageCount: 0, ErrorCount: 0},
			{FlowID: "f2", MessageCount: 0, ErrorCount: 0},
			{FlowID: "f3", MessageCount: 0, ErrorCount: 0},
		},
	}

	b := NewMonitoringBroadcaster(hub, src, nil,
		WithBroadcastInterval(15*time.Millisecond),
	)

	sendCh := make(chan []byte, 256)
	fakeClient := &Client{
		hub:    hub,
		send:   sendCh,
		logger: slog.Default(),
	}
	hub.Register(fakeClient)
	time.Sleep(20 * time.Millisecond)

	ctx := context.Background()
	b.Start(ctx)

	// 메시지 수신하여 크래시 없이 동작하는지 확인
	select {
	case received := <-sendCh:
		var msg Message
		if err := json.Unmarshal(received, &msg); err != nil {
			t.Fatalf("메시지 파싱 실패: %v", err)
		}
		if msg.Type != TypeFlowMetrics {
			t.Errorf("메시지 타입 = %q, want %q", msg.Type, TypeFlowMetrics)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("메트릭 수신 실패")
	}

	b.Stop()
}
