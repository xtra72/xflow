package node

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/observe"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// mockLogger 는 테스트용 ComponentLogger 구현이다.
type mockLogger struct {
	mu       sync.Mutex
	messages []string
}

func (m *mockLogger) Debug(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, "debug:"+msg)
}

func (m *mockLogger) Info(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, "info:"+msg)
}

func (m *mockLogger) Warn(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, "warn:"+msg)
}

func (m *mockLogger) Error(msg string, args ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, "error:"+msg)
}

func (m *mockLogger) With(args ...any) observe.ComponentLogger { return m }

func (m *mockLogger) WithGroup(name string) observe.ComponentLogger { return m }

func (m *mockLogger) Component() string { return "mock" }

func (m *mockLogger) Logger() *slog.Logger { return nil }

// --- DebugNode 인터페이스 준수 ---

var _ Node = (*DebugNode)(nil)

// --- NewDebugNode 테스트 ---

// TestNewDebugNode_정상생성 은 DebugNode가 올바르게 생성되는지 확인한다.
func TestNewDebugNode_정상생성(t *testing.T) {
	def := flow.NewNodeDef("debug-1", "debug")
	node, err := NewDebugNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "debug-1", node.Name())
	assert.Equal(t, "debug", node.Type())
}

// --- Init 테스트 ---

// TestDebugNode_Init_상태전이 는 Init 호출 시 Running 상태로 전이하는지 확인한다.
func TestDebugNode_Init_상태전이(t *testing.T) {
	def := flow.NewNodeDef("debug-init", "debug")
	node, _ := NewDebugNode(def)
	dn := node.(*DebugNode)

	err := dn.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, dn.CurrentState())
}

// --- Process 테스트 ---

// TestDebugNode_Process_패스스루 는 메시지가 그대로 통과하는지 확인한다.
func TestDebugNode_Process_패스스루(t *testing.T) {
	def := flow.NewNodeDef("debug-pass", "debug")
	node, _ := NewDebugNode(def)

	msg := message.New(
		message.WithPayload(message.NewPayload(map[string]any{"key": "value"})),
		message.WithMetadata("meta1", "val1"),
	)
	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, msg.ID(), results[0].ID())
}

// TestDebugNode_Process_Debug레벨로깅 은 debug 레벨에서 로그가 기록되는지 확인한다.
func TestDebugNode_Process_Debug레벨로깅(t *testing.T) {
	ml := &mockLogger{}
	def := flow.NewNodeDef("debug-log-debug", "debug")
	node, _ := NewDebugNode(def, WithLogger(ml))
	dn := node.(*DebugNode)
	dn.logLevel = "debug"

	msg := message.New()
	results, err := dn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	ml.mu.Lock()
	defer ml.mu.Unlock()
	assert.NotEmpty(t, ml.messages)
	assert.Contains(t, ml.messages[0], "debug:")
}

// TestDebugNode_Process_Info레벨로깅 은 info 레벨에서 로그가 기록되는지 확인한다.
func TestDebugNode_Process_Info레벨로깅(t *testing.T) {
	ml := &mockLogger{}
	def := flow.NewNodeDef("debug-log-info", "debug")
	node, _ := NewDebugNode(def, WithLogger(ml))
	dn := node.(*DebugNode)
	dn.logLevel = "info"

	msg := message.New()
	_, err := dn.Process(context.Background(), msg)
	require.NoError(t, err)

	ml.mu.Lock()
	defer ml.mu.Unlock()
	assert.NotEmpty(t, ml.messages)
	assert.Contains(t, ml.messages[0], "info:")
}

// TestDebugNode_Process_Warn레벨로깅 은 warn 레벨에서 로그가 기록되는지 확인한다.
func TestDebugNode_Process_Warn레벨로깅(t *testing.T) {
	ml := &mockLogger{}
	def := flow.NewNodeDef("debug-log-warn", "debug")
	node, _ := NewDebugNode(def, WithLogger(ml))
	dn := node.(*DebugNode)
	dn.logLevel = "warn"

	msg := message.New()
	_, err := dn.Process(context.Background(), msg)
	require.NoError(t, err)

	ml.mu.Lock()
	defer ml.mu.Unlock()
	assert.NotEmpty(t, ml.messages)
	assert.Contains(t, ml.messages[0], "warn:")
}

// TestDebugNode_Process_로거nil_패스스루 는 로거가 nil이면 로그 없이 메시지를 통과시키는지 확인한다.
func TestDebugNode_Process_로거nil_패스스루(t *testing.T) {
	def := flow.NewNodeDef("debug-no-logger", "debug")
	node, _ := NewDebugNode(def) // 로거 없음
	dn := node.(*DebugNode)
	dn.logLevel = "debug"

	msg := message.New()
	results, err := dn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, msg.ID(), results[0].ID())
}

// --- Configure 테스트 ---

// TestDebugNode_Configure_레벨설정 은 Configure로 로그 레벨을 설정할 수 있는지 확인한다.
func TestDebugNode_Configure_레벨설정(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		expected string
	}{
		{"debug 레벨", "debug", "debug"},
		{"info 레벨", "info", "info"},
		{"warn 레벨", "warn", "warn"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := flow.NewNodeDef("debug-cfg-"+tt.level, "debug")
			node, _ := NewDebugNode(def)
			dn := node.(*DebugNode)

			err := dn.Configure(map[string]any{"level": tt.level})
			require.NoError(t, err)
			assert.Equal(t, tt.expected, dn.logLevel)
		})
	}
}

// TestDebugNode_Configure_기본레벨 은 level이 없으면 기본값 "debug"가 적용되는지 확인한다.
func TestDebugNode_Configure_기본레벨(t *testing.T) {
	def := flow.NewNodeDef("debug-cfg-default", "debug")
	node, _ := NewDebugNode(def)
	dn := node.(*DebugNode)

	// 기본값 확인
	assert.Equal(t, "debug", dn.logLevel)
}

// TestDebugNode_Configure_nil에러 는 nil config 시 에러를 반환하는지 확인한다.
func TestDebugNode_Configure_nil에러(t *testing.T) {
	def := flow.NewNodeDef("debug-cfg-nil", "debug")
	node, _ := NewDebugNode(def)

	err := node.Configure(nil)
	assert.Error(t, err)
}

// --- Shutdown 테스트 ---

// TestDebugNode_Shutdown_상태전이 는 Shutdown 시 Stopping 상태로 전이하는지 확인한다.
func TestDebugNode_Shutdown_상태전이(t *testing.T) {
	def := flow.NewNodeDef("debug-shut", "debug")
	node, _ := NewDebugNode(def)
	dn := node.(*DebugNode)

	_ = dn.Init(context.Background())
	err := dn.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, dn.CurrentState())
}

// --- 동시성 테스트 ---

// TestDebugNode_동시성안전_Process 는 Process가 동시성 안전한지 확인한다.
func TestDebugNode_동시성안전_Process(t *testing.T) {
	ml := &mockLogger{}
	def := flow.NewNodeDef("debug-conc", "debug")
	node, _ := NewDebugNode(def, WithLogger(ml))
	dn := node.(*DebugNode)
	dn.logLevel = "debug"

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg := message.New()
			_, _ = dn.Process(context.Background(), msg)
		}()
	}
	wg.Wait()
}
