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

// --- CatchNode 인터페이스 준수 ---

var _ Node = (*CatchNode)(nil)

// --- NewCatchNode 테스트 ---

// TestNewCatchNode_정상생성 은 CatchNode가 올바르게 생성되는지 확인한다.
func TestNewCatchNode_정상생성(t *testing.T) {
	def := flow.NewNodeDef("catch-1", "catch")
	node, err := NewCatchNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "catch-1", node.Name())
	assert.Equal(t, "catch", node.Type())
}

// --- Init 테스트 ---

// TestCatchNode_Init_상태전이 는 Init 호출 시 Running 상태로 전이하는지 확인한다.
func TestCatchNode_Init_상태전이(t *testing.T) {
	def := flow.NewNodeDef("catch-init", "catch")
	node, _ := NewCatchNode(def)
	cn := node.(*CatchNode)

	err := cn.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, cn.CurrentState())
}

// --- Process 테스트 ---

// TestCatchNode_Process_catchTypes없음_모든에러처리 는 catchTypes가 없으면 모든 메시지를 처리하는지 확인한다.
func TestCatchNode_Process_catchTypes없음_모든에러처리(t *testing.T) {
	def := flow.NewNodeDef("catch-all", "catch")
	node, _ := NewCatchNode(def)

	msg := message.New(
		message.WithMetadata("_error_type", "timeout"),
		message.WithMetadata("_error_message", "요청 시간 초과"),
		message.WithMetadata("_source_node_id", "node-123"),
	)

	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

// TestCatchNode_Process_catchTypes매칭_처리 는 에러 타입이 catchTypes에 있으면 처리하는지 확인한다.
func TestCatchNode_Process_catchTypes매칭_처리(t *testing.T) {
	def := flow.NewNodeDef("catch-match", "catch")
	node, _ := NewCatchNode(def)
	cn := node.(*CatchNode)

	cn.catchTypes = []string{"timeout", "validation"}

	msg := message.New(
		message.WithMetadata("_error_type", "timeout"),
		message.WithMetadata("_error_message", "요청 시간 초과"),
	)

	results, err := cn.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

// TestCatchNode_Process_catchTypes비매칭_통과 는 에러 타입이 catchTypes에 없으면 통과시키는지 확인한다.
func TestCatchNode_Process_catchTypes비매칭_통과(t *testing.T) {
	def := flow.NewNodeDef("catch-nomatch", "catch")
	node, _ := NewCatchNode(def)
	cn := node.(*CatchNode)

	cn.catchTypes = []string{"timeout", "validation"}

	msg := message.New(
		message.WithMetadata("_error_type", "auth_failure"),
		message.WithMetadata("_error_message", "인증 실패"),
	)

	results, err := cn.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	// 통과된 메시지는 원본과 동일해야 한다
	assert.Equal(t, msg.ID(), results[0].ID())
}

// TestCatchNode_Process_에러타입없는메시지_패스스루 는 에러 타입이 없는 메시지를 통과시키는지 확인한다.
func TestCatchNode_Process_에러타입없는메시지_패스스루(t *testing.T) {
	def := flow.NewNodeDef("catch-no-error", "catch")
	node, _ := NewCatchNode(def)

	msg := message.New()
	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

// TestCatchNode_Process_다양한에러타입 은 테이블 드리븐 테스트로 다양한 에러 타입을 확인한다.
func TestCatchNode_Process_다양한에러타입(t *testing.T) {
	def := flow.NewNodeDef("catch-types", "catch")
	node, _ := NewCatchNode(def)
	cn := node.(*CatchNode)

	cn.catchTypes = []string{"timeout", "validation", "not_found"}

	tests := []struct {
		name       string
		errorType  string
		shouldPass bool // true면 catchTypes에 없어서 통과
	}{
		{"타임아웃 에러 처리", "timeout", false},
		{"유효성 에러 처리", "validation", false},
		{"Not Found 에러 처리", "not_found", false},
		{"인증 에러 통과", "auth_failure", true},
		{"내부 에러 통과", "internal_error", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := message.New(
				message.WithMetadata("_error_type", tt.errorType),
			)
			results, err := cn.Process(context.Background(), msg)
			require.NoError(t, err)
			assert.Len(t, results, 1)

			if tt.shouldPass {
				// 통과된 메시지는 원본 그대로
				assert.Equal(t, msg.ID(), results[0].ID())
			}
		})
	}
}

// --- Configure 테스트 ---

// TestCatchNode_Configure_catchTypes설정 은 Configure로 catchTypes를 설정할 수 있는지 확인한다.
func TestCatchNode_Configure_catchTypes설정(t *testing.T) {
	def := flow.NewNodeDef("catch-cfg", "catch")
	node, _ := NewCatchNode(def)

	err := node.Configure(map[string]any{
		"catch_types": []string{"timeout", "validation"},
	})
	require.NoError(t, err)

	cn := node.(*CatchNode)
	assert.Equal(t, []string{"timeout", "validation"}, cn.catchTypes)
}

// TestCatchNode_Configure_nil에러 는 nil config 시 에러를 반환하는지 확인한다.
func TestCatchNode_Configure_nil에러(t *testing.T) {
	def := flow.NewNodeDef("catch-cfg-nil", "catch")
	node, _ := NewCatchNode(def)

	err := node.Configure(nil)
	assert.Error(t, err)
}

// --- Shutdown 테스트 ---

// TestCatchNode_Shutdown_상태전이 는 Shutdown 시 Stopping 상태로 전이하는지 확인한다.
func TestCatchNode_Shutdown_상태전이(t *testing.T) {
	def := flow.NewNodeDef("catch-shut", "catch")
	node, _ := NewCatchNode(def)
	cn := node.(*CatchNode)

	_ = cn.Init(context.Background())
	err := cn.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, cn.CurrentState())
}

// --- 동시성 테스트 ---

// TestCatchNode_동시성안전_Process 는 Process가 동시성 안전한지 확인한다.
func TestCatchNode_동시성안전_Process(t *testing.T) {
	def := flow.NewNodeDef("catch-conc", "catch")
	node, _ := NewCatchNode(def)
	cn := node.(*CatchNode)

	cn.catchTypes = []string{"timeout"}

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			msg := message.New(
				message.WithMetadata("_error_type", "timeout"),
			)
			_, _ = cn.Process(context.Background(), msg)
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
