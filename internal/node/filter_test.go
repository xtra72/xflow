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

// TestFilterNode_Process_조건false_rejectPort_기본 은 기본 모드에서 reject 시 _target_port=reject 메시지를 반환하는지 확인한다.
func TestFilterNode_Process_조건false_rejectPort_기본(t *testing.T) {
	def := flow.NewNodeDef("filter-drop", "filter")
	node, _ := NewFilterNode(def)
	fn := node.(*FilterNode)

	fn.condition = func(msg message.Message) bool { return false }

	msg := message.New()
	results, err := fn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	tp, ok := results[0].Metadata().Get("_target_port")
	assert.True(t, ok)
	assert.Equal(t, "reject", tp)
}

// TestFilterNode_Process_조건false_errorPort 은 error_port 모드에서 reject 시 ErrFilterRejected를 반환하는지 확인한다.
func TestFilterNode_Process_조건false_errorPort(t *testing.T) {
	def := flow.NewNodeDef("filter-drop-err", "filter")
	node, _ := NewFilterNode(def)
	fn := node.(*FilterNode)

	fn.condition = func(msg message.Message) bool { return false }
	fn.onReject = rejectToError

	msg := message.New()
	results, err := fn.Process(context.Background(), msg)
	assert.ErrorIs(t, err, ErrFilterRejected)
	assert.Nil(t, results)
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
		name      string
		payload   map[string]any
		wantPass  bool
	}{
		{"에러 레벨 통과", map[string]any{"level": "error"}, true},
		{"정보 레벨 거부", map[string]any{"level": "info"}, false},
		{"레벨 없음 거부", map[string]any{"other": "value"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := message.New(message.WithPayload(message.NewPayload(tt.payload)))
			results, err := fn.Process(context.Background(), msg)
			require.NoError(t, err)
			if tt.wantPass {
				assert.Len(t, results, 1)
			} else {
				require.Len(t, results, 1)
				tp, _ := results[0].Metadata().Get("_target_port")
				assert.Equal(t, "reject", tp)
			}
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
	require.Len(t, results, 1)
	tp, _ := results[0].Metadata().Get("_target_port")
	assert.Equal(t, "reject", tp)
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

// --- Configure 문자열 조건식 테스트 (SPEC-FILTER-001) ---

// TestFilterNode_Configure_문자열조건식_숫자비교 는 문자열 조건식으로 숫자 비교가 작동하는지 확인한다.
func TestFilterNode_Configure_문자열조건식_숫자비교(t *testing.T) {
	def := flow.NewNodeDef("filter-expr-num", "filter")
	node, _ := NewFilterNode(def)

	err := node.Configure(map[string]any{"condition": "$.payload.temperature >= 30"})
	require.NoError(t, err)

	tests := []struct {
		name     string
		payload  map[string]any
		wantPass bool
	}{
		{"온도 35 통과", map[string]any{"temperature": float64(35)}, true},
		{"온도 30 통과 (경계값)", map[string]any{"temperature": float64(30)}, true},
		{"온도 25 거부", map[string]any{"temperature": float64(25)}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := message.New(message.WithPayload(message.NewPayload(tt.payload)))
			results, err := node.Process(context.Background(), msg)
			require.NoError(t, err)
			if tt.wantPass {
				assert.Len(t, results, 1)
			} else {
				require.Len(t, results, 1)
				tp, _ := results[0].Metadata().Get("_target_port")
				assert.Equal(t, "reject", tp)
			}
		})
	}
}

// TestFilterNode_Configure_문자열조건식_문자열비교 는 문자열 조건식으로 문자열 비교가 작동하는지 확인한다.
func TestFilterNode_Configure_문자열조건식_문자열비교(t *testing.T) {
	def := flow.NewNodeDef("filter-expr-str", "filter")
	node, _ := NewFilterNode(def)

	err := node.Configure(map[string]any{"condition": "$.payload.status == 'active'"})
	require.NoError(t, err)

	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"status": "active"})))
	results, err := node.Process(context.Background(), msg1)
	require.NoError(t, err)
	assert.Len(t, results, 1)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"status": "inactive"})))
	results, err = node.Process(context.Background(), msg2)
	require.NoError(t, err)
	require.Len(t, results, 1)
	tp, _ := results[0].Metadata().Get("_target_port")
	assert.Equal(t, "reject", tp)
}

// TestFilterNode_Configure_문자열조건식_exists 는 exists 함수 조건식이 작동하는지 확인한다.
func TestFilterNode_Configure_문자열조건식_exists(t *testing.T) {
	def := flow.NewNodeDef("filter-expr-exists", "filter")
	node, _ := NewFilterNode(def)

	err := node.Configure(map[string]any{"condition": "exists($.payload.error)"})
	require.NoError(t, err)

	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{"error": "some error"})))
	results, err := node.Process(context.Background(), msg1)
	require.NoError(t, err)
	assert.Len(t, results, 1)

	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{"status": "ok"})))
	results, err = node.Process(context.Background(), msg2)
	require.NoError(t, err)
	require.Len(t, results, 1)
	tp, _ := results[0].Metadata().Get("_target_port")
	assert.Equal(t, "reject", tp)
}

// TestFilterNode_Configure_문자열조건식_복합 은 복합 조건식이 작동하는지 확인한다.
func TestFilterNode_Configure_문자열조건식_복합(t *testing.T) {
	def := flow.NewNodeDef("filter-expr-complex", "filter")
	node, _ := NewFilterNode(def)

	err := node.Configure(map[string]any{
		"condition": "!exists($.payload.error) && $.payload.value > 0",
	})
	require.NoError(t, err)

	tests := []struct {
		name     string
		payload  map[string]any
		wantPass bool
	}{
		{"에러 없고 양수", map[string]any{"value": float64(10)}, true},
		{"에러 있고 양수", map[string]any{"error": "err", "value": float64(10)}, false},
		{"에러 없고 음수", map[string]any{"value": float64(-5)}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := message.New(message.WithPayload(message.NewPayload(tt.payload)))
			results, err := node.Process(context.Background(), msg)
			require.NoError(t, err)
			if tt.wantPass {
				assert.Len(t, results, 1)
			} else {
				require.Len(t, results, 1)
				tp, _ := results[0].Metadata().Get("_target_port")
				assert.Equal(t, "reject", tp)
			}
		})
	}
}

// TestFilterNode_Configure_문자열조건식_빈문자열에러 는 빈 문자열 조건식 시 에러를 반환하는지 확인한다.
func TestFilterNode_Configure_문자열조건식_빈문자열에러(t *testing.T) {
	def := flow.NewNodeDef("filter-expr-empty", "filter")
	node, _ := NewFilterNode(def)

	err := node.Configure(map[string]any{"condition": ""})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidExpression)
}

// TestFilterNode_Configure_문자열조건식_파싱에러 는 잘못된 조건식 시 에러를 반환하는지 확인한다.
func TestFilterNode_Configure_문자열조건식_파싱에러(t *testing.T) {
	def := flow.NewNodeDef("filter-expr-invalid", "filter")
	node, _ := NewFilterNode(def)

	err := node.Configure(map[string]any{"condition": "$.a >="})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidExpression)
}

// TestFilterNode_Configure_Go함수우선 은 Go 함수가 문자열보다 우선하는지 확인한다.
func TestFilterNode_Configure_Go함수우선(t *testing.T) {
	def := flow.NewNodeDef("filter-fn-priority", "filter")
	node, _ := NewFilterNode(def)

	// Go 함수 설정 (항상 true)
	cond := FilterCondition(func(msg message.Message) bool { return true })
	err := node.Configure(map[string]any{"condition": cond})
	require.NoError(t, err)

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"temperature": float64(10)})))
	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Len(t, results, 1, "Go 함수가 우선이므로 항상 통과해야 한다")
}

// TestFilterNode_Configure_condition미설정_패스스루 는 condition 키 없을 때 패스스루를 확인한다.
func TestFilterNode_Configure_condition미설정_패스스루(t *testing.T) {
	def := flow.NewNodeDef("filter-no-cond", "filter")
	node, _ := NewFilterNode(def)

	err := node.Configure(map[string]any{"other": "value"})
	require.NoError(t, err)

	msg := message.New()
	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Len(t, results, 1, "condition 미설정이면 패스스루여야 한다")
}
