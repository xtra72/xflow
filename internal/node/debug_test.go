package node

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
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

// mockDebugSink 는 테스트용 DebugSink 구현이다.
type mockDebugSink struct {
	mu       sync.Mutex
	messages []string
	nodeIDs  []string
}

func (s *mockDebugSink) SendDebug(nodeID string, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodeIDs = append(s.nodeIDs, nodeID)
	s.messages = append(s.messages, message)
	return nil
}

// --- DebugNode 인터페이스 준수 ---

var _ Node = (*DebugNode)(nil)

// ============================================================================
// 기존 DebugNode 테스트 (특성화 테스트 - DDD PRESERVE)
// ============================================================================

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

// ============================================================================
// Output 기능 특성화 테스트 (M0: DDD PRESERVE - output.go에서 마이그레이션)
// ============================================================================

// TestDebugNode_OutputCompat_기본생성 은 output 타입으로 생성 시 기본값을 확인한다.
func TestDebugNode_OutputCompat_기본생성(t *testing.T) {
	def := flow.NodeDef{ID: "out1", Type: "output"}
	n, err := NewDebugNode(def)
	require.NoError(t, err)
	require.NotNil(t, n)

	dn := n.(*DebugNode)
	// output 노드의 기본 프리픽스는 노드 이름(Name)이다.
	// NodeDef 리터럴에서 Name이 빈 문자열이므로 프리픽스도 빈 문자열.
	assert.Equal(t, "", dn.prefix)
	assert.Nil(t, dn.tmpl)
}

// TestDebugNode_OutputCompat_Configure_기본값 은 빈 설정이 기본값을 유지하는지 확인한다.
func TestDebugNode_OutputCompat_Configure_기본값(t *testing.T) {
	def := flow.NodeDef{ID: "out1", Type: "output"}
	n, err := NewDebugNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{})
	assert.NoError(t, err)

	dn := n.(*DebugNode)
	assert.Nil(t, dn.tmpl)
}

// TestDebugNode_OutputCompat_Configure_WithTemplate 은 템플릿과 프리픽스 설정을 확인한다.
func TestDebugNode_OutputCompat_Configure_WithTemplate(t *testing.T) {
	def := flow.NodeDef{ID: "out1", Type: "output"}
	n, err := NewDebugNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"prefix":   "[test]",
		"template": "temp={{.temperature}}",
	})
	assert.NoError(t, err)

	dn := n.(*DebugNode)
	assert.Equal(t, "[test]", dn.prefix)
	assert.NotNil(t, dn.tmpl)
}

// TestDebugNode_OutputCompat_Configure_InvalidTemplate 은 잘못된 템플릿 시 에러를 반환하는지 확인한다.
func TestDebugNode_OutputCompat_Configure_InvalidTemplate(t *testing.T) {
	def := flow.NodeDef{ID: "out1", Type: "output"}
	n, err := NewDebugNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"template": "{{.invalid",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid template")
}

// TestDebugNode_OutputCompat_Process_NoTemplate 은 템플릿 없을 때 패스스루를 확인한다.
func TestDebugNode_OutputCompat_Process_NoTemplate(t *testing.T) {
	def := flow.NodeDef{ID: "out1", Type: "output"}
	n, err := NewDebugNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 25.5,
	})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, msg.ID(), results[0].ID())
}

// TestDebugNode_OutputCompat_Process_WithTemplate 은 템플릿 사용 시 패스스루를 확인한다.
func TestDebugNode_OutputCompat_Process_WithTemplate(t *testing.T) {
	def := flow.NodeDef{ID: "out1", Type: "output"}
	n, err := NewDebugNode(def)
	require.NoError(t, err)

	err = n.Configure(map[string]any{
		"template": "temp={{.temperature}}",
	})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.NoError(t, err)

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 25.5,
	})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, msg.ID(), results[0].ID())
}

// TestDebugNode_OutputCompat_Process_PassThrough 은 메시지가 변경 없이 통과하는지 확인한다.
func TestDebugNode_OutputCompat_Process_PassThrough(t *testing.T) {
	def := flow.NodeDef{ID: "out1", Type: "output"}
	n, err := NewDebugNode(def)
	require.NoError(t, err)
	_ = n.Configure(map[string]any{})
	_ = n.Init(context.Background())

	payload := map[string]any{"key": "value", "num": 42.0}
	msg := message.New(message.WithPayload(message.NewPayload(payload)))
	originalID := msg.ID()

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 원본 메시지가 변경되지 않았는지 확인
	assert.Equal(t, originalID, results[0].ID())

	keyVal, keyOk := results[0].Payload().Get("key")
	assert.True(t, keyOk)
	assert.Equal(t, "value", keyVal)

	numVal, numOk := results[0].Payload().Get("num")
	assert.True(t, numOk)
	assert.Equal(t, 42.0, numVal)
}

// TestDebugNode_OutputCompat_Shutdown 은 Shutdown이 정상 동작하는지 확인한다.
func TestDebugNode_OutputCompat_Shutdown(t *testing.T) {
	def := flow.NodeDef{ID: "out1", Type: "output"}
	n, err := NewDebugNode(def)
	require.NoError(t, err)
	_ = n.Init(context.Background())

	err = n.Shutdown(context.Background())
	assert.NoError(t, err)

	dn := n.(*DebugNode)
	assert.Equal(t, lifecycle.StateStopping, dn.BaseNode.CurrentState())
}

// ============================================================================
// M1: 템플릿 & 프리픽스 TDD 테스트 (RED-GREEN-REFACTOR)
// ============================================================================

// TestDebugNode_기본프리픽스_노드이름 은 기본 프리픽스가 노드 이름인지 확인한다.
func TestDebugNode_기본프리픽스_노드이름(t *testing.T) {
	def := flow.NewNodeDef("my-debug", "debug")
	node, err := NewDebugNode(def)
	require.NoError(t, err)
	dn := node.(*DebugNode)
	assert.Equal(t, "my-debug", dn.prefix)
}

// TestDebugNode_Configure_프리픽스설정 은 Configure로 프리픽스를 설정할 수 있는지 확인한다.
func TestDebugNode_Configure_프리픽스설정(t *testing.T) {
	def := flow.NewNodeDef("debug-prefix", "debug")
	node, _ := NewDebugNode(def)
	dn := node.(*DebugNode)

	err := dn.Configure(map[string]any{"prefix": "[custom]"})
	require.NoError(t, err)
	assert.Equal(t, "[custom]", dn.prefix)
}

// TestDebugNode_Configure_템플릿설정 은 Configure로 템플릿을 설정할 수 있는지 확인한다.
func TestDebugNode_Configure_템플릿설정(t *testing.T) {
	def := flow.NewNodeDef("debug-tmpl", "debug")
	node, _ := NewDebugNode(def)
	dn := node.(*DebugNode)

	err := dn.Configure(map[string]any{
		"template": "value={{.value}}",
	})
	require.NoError(t, err)
	assert.NotNil(t, dn.tmpl)
}

// TestDebugNode_Process_템플릿적용_로그출력 은 템플릿이 적용된 메시지가 로그에 기록되는지 확인한다.
func TestDebugNode_Process_템플릿적용_로그출력(t *testing.T) {
	ml := &mockLogger{}
	def := flow.NewNodeDef("debug-tmpl-log", "debug")
	node, _ := NewDebugNode(def, WithLogger(ml))
	dn := node.(*DebugNode)

	err := dn.Configure(map[string]any{
		"template": "temp={{.temperature}}",
		"prefix":   "[sensor]",
	})
	require.NoError(t, err)

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 25.5,
	})))

	results, err := dn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	ml.mu.Lock()
	defer ml.mu.Unlock()
	require.NotEmpty(t, ml.messages)
	// 기본 dest=logger, 기본 level=debug이므로 "debug:" 접두사
	assert.Contains(t, ml.messages[0], "debug:")
	assert.Contains(t, ml.messages[0], "[sensor]")
	assert.Contains(t, ml.messages[0], "temp=25.5")
}

// TestDebugNode_Process_템플릿실패_기본포맷폴백 은 템플릿 실행 실패 시 기본 포맷으로 폴백하는지 확인한다.
func TestDebugNode_Process_템플릿실패_기본포맷폴백(t *testing.T) {
	ml := &mockLogger{}
	def := flow.NewNodeDef("debug-tmpl-fail", "debug")
	node, _ := NewDebugNode(def, WithLogger(ml))
	dn := node.(*DebugNode)

	// 존재하지 않는 메서드 호출하는 템플릿으로 실행 오류 유발
	err := dn.Configure(map[string]any{
		"template": "{{.nonexistent | len}}",
	})
	require.NoError(t, err)

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"key": "value",
	})))

	results, err := dn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	ml.mu.Lock()
	defer ml.mu.Unlock()
	// 경고 로그 + 기본 포맷 폴백 로그가 기록되어야 함
	require.True(t, len(ml.messages) >= 2)
	assert.Contains(t, ml.messages[0], "warn:")
	assert.Contains(t, ml.messages[0], "template execution failed")
	// 폴백 출력에 기본 포맷이 포함되어야 함
	assert.Contains(t, ml.messages[1], "message id=")
}

// TestDebugNode_Process_프리픽스빈문자열 은 프리픽스가 빈 문자열이면 프리픽스 없이 출력하는지 확인한다.
func TestDebugNode_Process_프리픽스빈문자열(t *testing.T) {
	ml := &mockLogger{}
	def := flow.NodeDef{ID: "debug-no-prefix", Type: "debug"}
	node, _ := NewDebugNode(def, WithLogger(ml))
	dn := node.(*DebugNode)
	// NodeDef 리터럴이므로 Name=""이고 prefix=""

	msg := message.New()
	_, err := dn.Process(context.Background(), msg)
	require.NoError(t, err)

	ml.mu.Lock()
	defer ml.mu.Unlock()
	require.NotEmpty(t, ml.messages)
	// 프리픽스가 없으므로 "message id=" 로 바로 시작해야 함
	assert.Contains(t, ml.messages[0], "debug:message id=")
}

// ============================================================================
// M2: 출력 대상 (Output Destination) TDD 테스트
// ============================================================================

// TestDebugNode_Configure_출력대상설정 은 Configure로 출력 대상을 설정할 수 있는지 확인한다.
func TestDebugNode_Configure_출력대상설정(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{"slog", "slog", "slog"},
		{"logger", "logger", "logger"},
		{"terminal", "terminal", "terminal"},
		{"file", "file", "file"},
		{"editor", "editor", "editor"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := flow.NewNodeDef("debug-out-"+tt.output, "debug")
			node, _ := NewDebugNode(def)
			dn := node.(*DebugNode)

			err := dn.Configure(map[string]any{"output": tt.output})
			require.NoError(t, err)
			assert.Equal(t, tt.expected, dn.outputDest)
		})
	}
}

// TestDebugNode_Process_터미널출력 은 terminal 대상으로 stdout에 출력하는지 확인한다.
func TestDebugNode_Process_터미널출력(t *testing.T) {
	def := flow.NewNodeDef("debug-terminal", "debug")
	node, _ := NewDebugNode(def)
	dn := node.(*DebugNode)
	dn.outputDest = "terminal"

	// stdout 캡처
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"key": "val"})))
	results, err := dn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	assert.Contains(t, output, "message id=")
	assert.Contains(t, output, "debug-terminal") // 프리픽스로 노드 이름 포함
}

// TestDebugNode_Process_파일출력 은 file 대상으로 파일에 출력하는지 확인한다.
func TestDebugNode_Process_파일출력(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "debug-test-*.log")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	def := flow.NewNodeDef("debug-file", "debug")
	node, _ := NewDebugNode(def)
	dn := node.(*DebugNode)

	err = dn.Configure(map[string]any{
		"output": "file",
		"file":   tmpFile.Name(),
	})
	require.NoError(t, err)

	err = dn.Init(context.Background())
	require.NoError(t, err)

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"temp": 25.0})))
	results, processErr := dn.Process(context.Background(), msg)
	require.NoError(t, processErr)
	require.Len(t, results, 1)

	_ = dn.Shutdown(context.Background())

	content, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	assert.Contains(t, string(content), "message id=")
	assert.Contains(t, string(content), "debug-file") // 프리픽스
}

// TestDebugNode_Process_에디터출력_sink있음 은 editor 대상에서 sink가 있으면 SendDebug를 호출하는지 확인한다.
func TestDebugNode_Process_에디터출력_sink있음(t *testing.T) {
	sink := &mockDebugSink{}
	def := flow.NewNodeDef("debug-editor", "debug")
	node, _ := NewDebugNode(def)
	dn := node.(*DebugNode)
	dn.outputDest = "editor"
	dn.sink = sink

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"key": "val"})))
	results, err := dn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	sink.mu.Lock()
	defer sink.mu.Unlock()
	require.Len(t, sink.messages, 1)
	assert.Contains(t, sink.messages[0], "message id=")
	assert.Equal(t, dn.BaseNode.ID(), sink.nodeIDs[0])
}

// TestDebugNode_Process_에디터출력_sinkNil_로거폴백 은 editor 대상에서 sink가 nil이면 로거로 폴백하는지 확인한다.
func TestDebugNode_Process_에디터출력_sinkNil_로거폴백(t *testing.T) {
	ml := &mockLogger{}
	def := flow.NewNodeDef("debug-editor-fallback", "debug")
	node, _ := NewDebugNode(def, WithLogger(ml))
	dn := node.(*DebugNode)
	dn.outputDest = "editor"
	// sink는 nil

	msg := message.New()
	_, err := dn.Process(context.Background(), msg)
	require.NoError(t, err)

	ml.mu.Lock()
	defer ml.mu.Unlock()
	assert.NotEmpty(t, ml.messages)
	assert.Contains(t, ml.messages[0], "debug:")
}

// TestDebugNode_기본출력대상_slog 은 기본 출력 대상이 slog인지 확인한다.
func TestDebugNode_기본출력대상_slog(t *testing.T) {
	def := flow.NewNodeDef("debug-default-dest", "debug")
	node, _ := NewDebugNode(def)
	dn := node.(*DebugNode)
	assert.Equal(t, "slog", dn.outputDest)
}

// ============================================================================
// M3: 선택적 필드 (Selective Fields) TDD 테스트
// ============================================================================

// TestDebugNode_Configure_필드설정 은 Configure로 fields를 설정할 수 있는지 확인한다.
func TestDebugNode_Configure_필드설정(t *testing.T) {
	def := flow.NewNodeDef("debug-fields", "debug")
	node, _ := NewDebugNode(def)
	dn := node.(*DebugNode)

	err := dn.Configure(map[string]any{
		"fields": []any{"temperature", "humidity"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"temperature", "humidity"}, dn.fields)
}

// TestDebugNode_Process_필드필터링 은 fields 설정 시 지정된 필드만 출력하는지 확인한다.
func TestDebugNode_Process_필드필터링(t *testing.T) {
	ml := &mockLogger{}
	def := flow.NewNodeDef("debug-field-filter", "debug")
	node, _ := NewDebugNode(def, WithLogger(ml))
	dn := node.(*DebugNode)

	err := dn.Configure(map[string]any{
		"fields": []any{"temperature"},
	})
	require.NoError(t, err)

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 25.5,
		"humidity":    60.0,
		"pressure":    1013.0,
	})))

	results, err := dn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	ml.mu.Lock()
	defer ml.mu.Unlock()
	require.NotEmpty(t, ml.messages)
	// temperature만 포함되어야 함
	assert.Contains(t, ml.messages[0], "temperature")
	// humidity, pressure는 포함되지 않아야 함
	assert.NotContains(t, ml.messages[0], "humidity")
	assert.NotContains(t, ml.messages[0], "pressure")
}

// TestDebugNode_Process_필드필터링_템플릿조합 은 fields와 template을 함께 사용할 수 있는지 확인한다.
func TestDebugNode_Process_필드필터링_템플릿조합(t *testing.T) {
	ml := &mockLogger{}
	def := flow.NewNodeDef("debug-field-tmpl", "debug")
	node, _ := NewDebugNode(def, WithLogger(ml))
	dn := node.(*DebugNode)

	err := dn.Configure(map[string]any{
		"fields":   []any{"temperature"},
		"template": "temp={{.temperature}}",
		"prefix":   "[sensor]",
	})
	require.NoError(t, err)

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 25.5,
		"humidity":    60.0,
	})))

	_, err = dn.Process(context.Background(), msg)
	require.NoError(t, err)

	ml.mu.Lock()
	defer ml.mu.Unlock()
	require.NotEmpty(t, ml.messages)
	assert.Contains(t, ml.messages[0], "[sensor]")
	assert.Contains(t, ml.messages[0], "temp=25.5")
}

// TestDebugNode_Process_빈필드_전체출력 은 fields가 비어있으면 전체 페이로드를 출력하는지 확인한다.
func TestDebugNode_Process_빈필드_전체출력(t *testing.T) {
	ml := &mockLogger{}
	def := flow.NewNodeDef("debug-no-fields", "debug")
	node, _ := NewDebugNode(def, WithLogger(ml))
	dn := node.(*DebugNode)

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"a": 1,
		"b": 2,
	})))

	_, err := dn.Process(context.Background(), msg)
	require.NoError(t, err)

	ml.mu.Lock()
	defer ml.mu.Unlock()
	require.NotEmpty(t, ml.messages)
	// a, b 모두 포함되어야 함
	logOutput := ml.messages[0]
	assert.Contains(t, logOutput, "a")
	assert.Contains(t, logOutput, "b")
}

// TestDebugNode_Process_필드필터링_존재하지않는필드 은 존재하지 않는 필드를 지정하면 빈 페이로드가 되는지 확인한다.
func TestDebugNode_Process_필드필터링_존재하지않는필드(t *testing.T) {
	ml := &mockLogger{}
	def := flow.NewNodeDef("debug-missing-field", "debug")
	node, _ := NewDebugNode(def, WithLogger(ml))
	dn := node.(*DebugNode)

	err := dn.Configure(map[string]any{
		"fields": []any{"nonexistent"},
	})
	require.NoError(t, err)

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 25.5,
	})))

	results, err := dn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	ml.mu.Lock()
	defer ml.mu.Unlock()
	require.NotEmpty(t, ml.messages)
	// temperature가 출력에 포함되지 않아야 함
	assert.NotContains(t, ml.messages[0], "temperature")
	// payload가 빈 map으로 나와야 함
	assert.Contains(t, ml.messages[0], "payload=map[]")
}

// TestDebugNode_Process_패스스루_필드필터링시_원본보존 은 필드 필터링이 원본 메시지를 변경하지 않는지 확인한다.
func TestDebugNode_Process_패스스루_필드필터링시_원본보존(t *testing.T) {
	def := flow.NewNodeDef("debug-passthrough-fields", "debug")
	node, _ := NewDebugNode(def)
	dn := node.(*DebugNode)

	err := dn.Configure(map[string]any{
		"fields": []any{"a"},
	})
	require.NoError(t, err)

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"a": 1,
		"b": 2,
	})))
	originalID := msg.ID()

	results, err := dn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 원본 메시지가 그대로 반환되어야 함
	assert.Equal(t, originalID, results[0].ID())
	bVal, bOk := results[0].Payload().Get("b")
	assert.True(t, bOk, "원본 메시지의 'b' 필드가 보존되어야 함")
	assert.Equal(t, 2, bVal)
}

// ============================================================================
// M4: 하위 호환성 테스트
// ============================================================================

// TestDebugNode_레지스트리_output타입_DebugNode생성 은 "output" 타입이 DebugNode를 생성하는지 확인한다.
func TestDebugNode_레지스트리_output타입_DebugNode생성(t *testing.T) {
	r := NewRegistry()
	def := flow.NewNodeDef("test-output", "output")
	node, err := r.Create(def)
	require.NoError(t, err)

	// DebugNode로 타입 변환이 성공해야 함
	dn, ok := node.(*DebugNode)
	require.True(t, ok, "output 타입이 DebugNode를 반환해야 함")
	assert.NotNil(t, dn)
}

// TestDebugNode_레지스트리_output타입_config호환 은 output의 기존 config 키가 동작하는지 확인한다.
func TestDebugNode_레지스트리_output타입_config호환(t *testing.T) {
	r := NewRegistry()
	def := flow.NewNodeDef("test-output-cfg", "output")
	node, err := r.Create(def)
	require.NoError(t, err)

	err = node.Configure(map[string]any{
		"prefix":   "[my-output]",
		"template": "v={{.value}}",
		"file":     "/tmp/test.log",
	})
	require.NoError(t, err)

	dn := node.(*DebugNode)
	assert.Equal(t, "[my-output]", dn.prefix)
	assert.NotNil(t, dn.tmpl)
	assert.Equal(t, "/tmp/test.log", dn.filePath)
}

// ============================================================================
// 동시성 안전성 추가 테스트
// ============================================================================

// TestDebugNode_동시성안전_Configure와Process 는 Configure와 Process가 동시에 안전한지 확인한다.
func TestDebugNode_동시성안전_Configure와Process(t *testing.T) {
	ml := &mockLogger{}
	def := flow.NewNodeDef("debug-conc2", "debug")
	node, _ := NewDebugNode(def, WithLogger(ml))
	dn := node.(*DebugNode)

	var wg sync.WaitGroup
	// 동시에 Configure와 Process 실행
	for i := 0; i < 5; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			_ = dn.Configure(map[string]any{
				"level":  "info",
				"prefix": fmt.Sprintf("[prefix-%d]", i),
			})
		}(i)
		go func() {
			defer wg.Done()
			msg := message.New()
			_, _ = dn.Process(context.Background(), msg)
		}()
	}
	wg.Wait()
}
