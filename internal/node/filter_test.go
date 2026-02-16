package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// --- FilterNode 인터페이스 준수 ---

var _ Node = (*FilterNode)(nil)

// --- NewFilterNode 테스트 ---

// TestNewFilterNode_정상생성 은 FilterNode가 올바르게 생성되는지 확인한다.
func TestNewFilterNode_정상생성(t *testing.T) {
	def := flow.NewNodeDef("filter-1", "filter")
	node, err := NewFilterNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "filter-1", node.Name())
	assert.Equal(t, "filter", node.Type())
}

// --- Init 테스트 ---

// TestFilterNode_Init_상태전이 는 Init 호출 시 Running 상태로 전이하는지 확인한다.
func TestFilterNode_Init_상태전이(t *testing.T) {
	def := flow.NewNodeDef("filter-init", "filter")
	node, _ := NewFilterNode(def)
	fn := node.(*FilterNode)

	err := fn.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, fn.CurrentState())
}

// --- Process 테스트 ---

// TestFilterNode_Process_조건nil_패스스루 는 조건이 nil이면 메시지를 통과시키는지 확인한다.
func TestFilterNode_Process_조건nil_패스스루(t *testing.T) {
	def := flow.NewNodeDef("filter-nil", "filter")
	node, _ := NewFilterNode(def)

	msg := message.New()
	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, msg.ID(), results[0].ID())
}

// TestFilterNode_Process_조건true_통과 는 조건이 true를 반환하면 메시지를 통과시키는지 확인한다.
func TestFilterNode_Process_조건true_통과(t *testing.T) {
	def := flow.NewNodeDef("filter-pass", "filter")
	node, _ := NewFilterNode(def)
	fn := node.(*FilterNode)

	fn.condition = func(msg message.Message) bool { return true }

	msg := message.New()
	results, err := fn.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

// TestFilterNode_Process_조건false_드롭 은 조건이 false를 반환하면 메시지를 드롭하는지 확인한다.
func TestFilterNode_Process_조건false_드롭(t *testing.T) {
	def := flow.NewNodeDef("filter-drop", "filter")
	node, _ := NewFilterNode(def)
	fn := node.(*FilterNode)

	fn.condition = func(msg message.Message) bool { return false }

	msg := message.New()
	results, err := fn.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestFilterNode_Process_페이로드기반조건 은 페이로드 값으로 필터링할 수 있는지 확인한다.
func TestFilterNode_Process_페이로드기반조건(t *testing.T) {
	def := flow.NewNodeDef("filter-payload", "filter")
	node, _ := NewFilterNode(def)
	fn := node.(*FilterNode)

	fn.condition = func(msg message.Message) bool {
		v, ok := msg.Payload().Get("level")
		if !ok {
			return false
		}
		level, ok := v.(string)
		return ok && level == "error"
	}

	tests := []struct {
		name     string
		payload  map[string]any
		expected int
	}{
		{"에러 레벨 통과", map[string]any{"level": "error"}, 1},
		{"정보 레벨 드롭", map[string]any{"level": "info"}, 0},
		{"레벨 없음 드롭", map[string]any{"other": "value"}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := message.New(message.WithPayload(message.NewPayload(tt.payload)))
			results, err := fn.Process(context.Background(), msg)
			require.NoError(t, err)
			assert.Len(t, results, tt.expected)
		})
	}
}

// --- Configure 테스트 ---

// TestFilterNode_Configure_조건설정 은 Configure로 조건을 설정할 수 있는지 확인한다.
func TestFilterNode_Configure_조건설정(t *testing.T) {
	def := flow.NewNodeDef("filter-cfg", "filter")
	node, _ := NewFilterNode(def)

	cond := FilterCondition(func(msg message.Message) bool { return false })
	err := node.Configure(map[string]any{"condition": cond})
	require.NoError(t, err)

	msg := message.New()
	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestFilterNode_Configure_nil에러 는 nil config 시 에러를 반환하는지 확인한다.
func TestFilterNode_Configure_nil에러(t *testing.T) {
	def := flow.NewNodeDef("filter-cfg-nil", "filter")
	node, _ := NewFilterNode(def)

	err := node.Configure(nil)
	assert.Error(t, err)
}

// --- Shutdown 테스트 ---

// TestFilterNode_Shutdown_상태전이 는 Shutdown 시 Stopping 상태로 전이하는지 확인한다.
func TestFilterNode_Shutdown_상태전이(t *testing.T) {
	def := flow.NewNodeDef("filter-shut", "filter")
	node, _ := NewFilterNode(def)
	fn := node.(*FilterNode)

	// Init으로 Running 상태가 되어야 Stopping으로 전이 가능
	_ = fn.Init(context.Background())
	err := fn.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, fn.CurrentState())
}

// --- 동시성 테스트 ---

// TestFilterNode_동시성안전_Process 는 Process가 동시성 안전한지 확인한다.
func TestFilterNode_동시성안전_Process(t *testing.T) {
	def := flow.NewNodeDef("filter-conc", "filter")
	node, _ := NewFilterNode(def)
	fn := node.(*FilterNode)

	fn.condition = func(msg message.Message) bool {
		return msg.Metadata().Has("pass")
	}

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			msg := message.New()
			_, _ = fn.Process(context.Background(), msg)
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
