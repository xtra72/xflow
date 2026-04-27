package node

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/observe"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// mockMetricsCollector 는 테스트용 MetricsCollector 구현이다.
type mockMetricsCollector struct {
	mu       sync.Mutex
	counters map[string]*mockCounterMetric
}

func newMockMetricsCollector() *mockMetricsCollector {
	return &mockMetricsCollector{
		counters: make(map[string]*mockCounterMetric),
	}
}

func (m *mockMetricsCollector) Counter(name string, component string) observe.CounterMetric {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := name + ":" + component
	if c, ok := m.counters[key]; ok {
		return c
	}
	c := &mockCounterMetric{}
	m.counters[key] = c
	return c
}

func (m *mockMetricsCollector) Histogram(name string, component string) observe.HistogramMetric {
	return &mockHistogramMetric{}
}

func (m *mockMetricsCollector) Gauge(name string, component string) observe.GaugeMetric {
	return &mockGaugeMetric{}
}

func (m *mockMetricsCollector) Registry() *prometheus.Registry {
	return nil
}

func (m *mockMetricsCollector) getCounter(name, component string) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := name + ":" + component
	if c, ok := m.counters[key]; ok {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.value
	}
	return 0
}

// mockCounterMetric 는 테스트용 Counter 구현이다.
type mockCounterMetric struct {
	mu    sync.Mutex
	value float64
}

func (c *mockCounterMetric) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value++
}

func (c *mockCounterMetric) Add(v float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value += v
}

// mockHistogramMetric 는 테스트용 Histogram 구현이다.
type mockHistogramMetric struct{}

func (h *mockHistogramMetric) Observe(float64) {}

// mockGaugeMetric 는 테스트용 Gauge 구현이다.
type mockGaugeMetric struct{}

func (g *mockGaugeMetric) Set(float64) {}
func (g *mockGaugeMetric) Inc()        {}
func (g *mockGaugeMetric) Dec()        {}
func (g *mockGaugeMetric) Add(float64) {}

// dlMockLogger 는 DeadLetterNode 테스트용 로거이다.
type dlMockLogger struct {
	mu       sync.Mutex
	messages []string
}

func (m *dlMockLogger) Debug(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, "debug:"+msg)
}

func (m *dlMockLogger) Info(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, "info:"+msg)
}

func (m *dlMockLogger) Warn(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, "warn:"+msg)
}

func (m *dlMockLogger) Error(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, "error:"+msg)
}

func (m *dlMockLogger) With(args ...any) observe.ComponentLogger { return m }

func (m *dlMockLogger) WithGroup(name string) observe.ComponentLogger { return m }

func (m *dlMockLogger) Component() string { return "mock" }

func (m *dlMockLogger) Logger() *slog.Logger { return nil }

// --- DeadLetterNode 인터페이스 준수 ---

var _ Node = (*DeadLetterNode)(nil)

// --- NewDeadLetterNode 테스트 ---

// TestNewDeadLetterNode_정상생성 은 DeadLetterNode가 올바르게 생성되는지 확인한다.
func TestNewDeadLetterNode_정상생성(t *testing.T) {
	def := flow.NewNodeDef("dl-1", "deadletter")
	node, err := NewDeadLetterNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "dl-1", node.Name())
	assert.Equal(t, "deadletter", node.Type())
}

// --- Init 테스트 ---

// TestDeadLetterNode_Init_상태전이 는 Init 호출 시 Running 상태로 전이하는지 확인한다.
func TestDeadLetterNode_Init_상태전이(t *testing.T) {
	def := flow.NewNodeDef("dl-init", "deadletter")
	node, _ := NewDeadLetterNode(def)
	dn := node.(*DeadLetterNode)

	err := dn.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, dn.CurrentState())
}

// --- Process 테스트 ---

// TestDeadLetterNode_Process_패스스루_메타데이터보강 은 메시지를 통과시키면서 _processed_at을 추가하는지 확인한다.
func TestDeadLetterNode_Process_패스스루_메타데이터보강(t *testing.T) {
	def := flow.NewNodeDef("dl-pass", "deadletter")
	node, _ := NewDeadLetterNode(def)

	msg := message.New(
		message.WithMetadata("_deadletter_reason", "ttl_expired"),
	)

	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, msg.ID(), results[0].ID())

	// _processed_at 메타데이터 확인
	processedAt, ok := results[0].Metadata().Get("_processed_at")
	assert.True(t, ok)
	assert.NotEmpty(t, processedAt)
}

// TestDeadLetterNode_Process_다양한이유 는 테이블 드리븐 테스트로 다양한 deadletter reason을 검증한다.
func TestDeadLetterNode_Process_다양한이유(t *testing.T) {
	tests := []struct {
		name   string
		reason string
	}{
		{"TTL 만료", "ttl_expired"},
		{"전달 불가", "undeliverable"},
		{"최대 재시도 초과", "max_retries"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := flow.NewNodeDef("dl-reason-"+tt.reason, "deadletter")
			node, _ := NewDeadLetterNode(def)

			msg := message.New(
				message.WithMetadata("_deadletter_reason", tt.reason),
			)

			results, err := node.Process(context.Background(), msg)
			require.NoError(t, err)
			require.Len(t, results, 1)

			// _processed_at 확인
			_, ok := results[0].Metadata().Get("_processed_at")
			assert.True(t, ok)
		})
	}
}

// TestDeadLetterNode_Process_Log전략_로깅 은 log 전략에서 로거를 통해 기록하는지 확인한다.
func TestDeadLetterNode_Process_Log전략_로깅(t *testing.T) {
	ml := &dlMockLogger{}
	def := flow.NewNodeDef("dl-log", "deadletter")
	node, _ := NewDeadLetterNode(def, WithLogger(ml))
	dn := node.(*DeadLetterNode)
	dn.strategy = DeadLetterLog

	msg := message.New(
		message.WithMetadata("_deadletter_reason", "ttl_expired"),
	)

	_, err := dn.Process(context.Background(), msg)
	require.NoError(t, err)

	ml.mu.Lock()
	defer ml.mu.Unlock()
	assert.NotEmpty(t, ml.messages)
	assert.Contains(t, ml.messages[0], "info:")
}

// TestDeadLetterNode_Process_Store전략_메타데이터 는 store 전략에서 _deadletter_strategy 메타데이터를 추가하는지 확인한다.
func TestDeadLetterNode_Process_Store전략_메타데이터(t *testing.T) {
	def := flow.NewNodeDef("dl-store", "deadletter")
	node, _ := NewDeadLetterNode(def)
	dn := node.(*DeadLetterNode)
	dn.strategy = DeadLetterStore

	msg := message.New(
		message.WithMetadata("_deadletter_reason", "undeliverable"),
	)

	results, err := dn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	strategyMeta, ok := results[0].Metadata().Get("_deadletter_strategy")
	require.True(t, ok)
	assert.Equal(t, "store", strategyMeta)
}

// TestDeadLetterNode_Process_Forward전략_메타데이터 는 forward 전략에서 _deadletter_strategy 메타데이터를 추가하는지 확인한다.
func TestDeadLetterNode_Process_Forward전략_메타데이터(t *testing.T) {
	def := flow.NewNodeDef("dl-forward", "deadletter")
	node, _ := NewDeadLetterNode(def)
	dn := node.(*DeadLetterNode)
	dn.strategy = DeadLetterForward

	msg := message.New(
		message.WithMetadata("_deadletter_reason", "max_retries"),
	)

	results, err := dn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	strategyMeta, ok := results[0].Metadata().Get("_deadletter_strategy")
	require.True(t, ok)
	assert.Equal(t, "forward", strategyMeta)
}

// TestDeadLetterNode_Process_메트릭기록 은 메트릭 수집기가 있으면 카운터를 기록하는지 확인한다.
func TestDeadLetterNode_Process_메트릭기록(t *testing.T) {
	mc := newMockMetricsCollector()
	def := flow.NewNodeDef("dl-metrics", "deadletter")
	node, _ := NewDeadLetterNode(def, WithMetrics(mc))
	dn := node.(*DeadLetterNode)
	dn.strategy = DeadLetterLog

	ctx := context.Background()

	// ttl_expired 메시지
	msg1 := message.New(message.WithMetadata("_deadletter_reason", "ttl_expired"))
	_, _ = dn.Process(ctx, msg1)

	// undeliverable 메시지
	msg2 := message.New(message.WithMetadata("_deadletter_reason", "undeliverable"))
	_, _ = dn.Process(ctx, msg2)

	// ttl_expired 두 번째
	msg3 := message.New(message.WithMetadata("_deadletter_reason", "ttl_expired"))
	_, _ = dn.Process(ctx, msg3)

	// 카운터 확인
	assert.Equal(t, 2.0, mc.getCounter("deadletter_ttl_expired_total", "deadletter"))
	assert.Equal(t, 1.0, mc.getCounter("deadletter_undeliverable_total", "deadletter"))
}

// TestDeadLetterNode_Process_이유없는메시지 는 _deadletter_reason이 없는 메시지도 통과시키는지 확인한다.
func TestDeadLetterNode_Process_이유없는메시지(t *testing.T) {
	def := flow.NewNodeDef("dl-no-reason", "deadletter")
	node, _ := NewDeadLetterNode(def)

	msg := message.New()
	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
}

// --- Configure 테스트 ---

// TestDeadLetterNode_Configure_전략설정 은 Configure로 전략을 설정할 수 있는지 확인한다.
func TestDeadLetterNode_Configure_전략설정(t *testing.T) {
	tests := []struct {
		name     string
		strategy string
		expected DeadLetterStrategy
	}{
		{"log 전략", "log", DeadLetterLog},
		{"store 전략", "store", DeadLetterStore},
		{"forward 전략", "forward", DeadLetterForward},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := flow.NewNodeDef("dl-cfg-"+tt.strategy, "deadletter")
			node, _ := NewDeadLetterNode(def)
			dn := node.(*DeadLetterNode)

			err := dn.Configure(map[string]any{"strategy": tt.strategy})
			require.NoError(t, err)
			assert.Equal(t, tt.expected, dn.strategy)
		})
	}
}

// TestDeadLetterNode_Configure_기본전략 은 strategy가 없으면 기본값 "log"가 적용되는지 확인한다.
func TestDeadLetterNode_Configure_기본전략(t *testing.T) {
	def := flow.NewNodeDef("dl-cfg-default", "deadletter")
	node, _ := NewDeadLetterNode(def)
	dn := node.(*DeadLetterNode)

	// 기본값 확인
	assert.Equal(t, DeadLetterLog, dn.strategy)
}

// TestDeadLetterNode_Configure_nil에러 는 nil config 시 에러를 반환하는지 확인한다.
func TestDeadLetterNode_Configure_nil에러(t *testing.T) {
	def := flow.NewNodeDef("dl-cfg-nil", "deadletter")
	node, _ := NewDeadLetterNode(def)

	err := node.Configure(nil)
	assert.Error(t, err)
}

// --- Shutdown 테스트 ---

// TestDeadLetterNode_Shutdown_상태전이 는 Shutdown 시 Stopping 상태로 전이하는지 확인한다.
func TestDeadLetterNode_Shutdown_상태전이(t *testing.T) {
	def := flow.NewNodeDef("dl-shut", "deadletter")
	node, _ := NewDeadLetterNode(def)
	dn := node.(*DeadLetterNode)

	_ = dn.Init(context.Background())
	err := dn.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, dn.CurrentState())
}

// --- 동시성 테스트 ---

// TestDeadLetterNode_동시성안전_Process 는 Process가 동시성 안전한지 확인한다.
func TestDeadLetterNode_동시성안전_Process(t *testing.T) {
	def := flow.NewNodeDef("dl-conc", "deadletter")
	node, _ := NewDeadLetterNode(def)
	dn := node.(*DeadLetterNode)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg := message.New(
				message.WithMetadata("_deadletter_reason", "ttl_expired"),
			)
			_, _ = dn.Process(context.Background(), msg)
		}()
	}
	wg.Wait()
}
