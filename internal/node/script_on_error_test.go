package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// script_on_error_test.go 는 script 노드의 on_error 정책(error/ignore/drop)을 검증한다.
// 사용자 요구: "에러 메시지 띄우지 않고, 필드가 없으면 무시" — ignore/drop 모드에서
// Process 가 에러를 반환하지 않아 엔진이 ERROR 로그를 남기지 않도록 한다.

// newFailingScriptNode 는 실행 시 항상 실패하는 엔진으로 script 노드를 만든다.
// on_error 는 config 경로(Configure)로 주입한다.
func newFailingScriptNode(t *testing.T, onError string) *ScriptNode {
	t.Helper()
	engine := &mockScriptEngine{
		executeFn: func(_ context.Context, _ message.Message) (message.Message, error) {
			return nil, errors.New("attempt to index nil (field 'foo')")
		},
	}
	def := flow.NewNodeDef("script-on-error", "script")
	node, err := NewScriptNode(def, WithScriptEngine(engine))
	require.NoError(t, err)
	sn := node.(*ScriptNode)
	if onError != "" {
		require.NoError(t, sn.Configure(map[string]any{"on_error": onError}))
	}
	return sn
}

// TestScriptNode_OnError_Modes 는 실행 실패 시 on_error 정책별 결과를 표 기반으로 검증한다.
func TestScriptNode_OnError_Modes(t *testing.T) {
	inMsg := message.New(message.WithID("in-1"))

	tests := []struct {
		name        string
		onError     string
		wantErrIs   error // nil 이면 에러 없음 기대
		wantResults int   // 기대 출력 개수
		wantSameMsg bool  // 출력이 원본 입력과 동일해야 하는가
	}{
		{"unset defaults to error", "", ErrScriptExecutionFailed, 0, false},
		{"explicit error mode", "error", ErrScriptExecutionFailed, 0, false},
		{"ignore passes original through", "ignore", nil, 1, true},
		{"drop emits nothing", "drop", nil, 0, false},
		{"unknown falls back to error", "bogus", ErrScriptExecutionFailed, 0, false},
		{"case-insensitive IGNORE", "IGNORE", nil, 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sn := newFailingScriptNode(t, tt.onError)
			results, err := sn.Process(context.Background(), inMsg)

			if tt.wantErrIs != nil {
				assert.ErrorIs(t, err, tt.wantErrIs, "error 모드는 래핑 에러를 반환해야 한다")
			} else {
				assert.NoError(t, err, "ignore/drop 은 에러를 반환하지 않아야 한다(엔진 ERROR 로그 방지)")
			}

			assert.Len(t, results, tt.wantResults)
			if tt.wantSameMsg {
				require.Len(t, results, 1)
				assert.Same(t, inMsg, results[0], "ignore 는 원본 입력 메시지를 그대로 통과시켜야 한다")
			}
		})
	}
}

// TestScriptNode_OnError_Timeout_Modes 는 타임아웃 실패에도 on_error 정책이 적용됨을 검증한다.
func TestScriptNode_OnError_Timeout_Modes(t *testing.T) {
	newTimeoutNode := func(t *testing.T, onError string) *ScriptNode {
		t.Helper()
		engine := &mockScriptEngine{
			executeFn: func(ctx context.Context, _ message.Message) (message.Message, error) {
				<-ctx.Done() // 타임아웃까지 대기
				return nil, ctx.Err()
			},
		}
		def := flow.NewNodeDef("script-timeout-onerr", "script")
		node, err := NewScriptNode(def, WithScriptEngine(engine), WithScriptTimeout(30*time.Millisecond))
		require.NoError(t, err)
		sn := node.(*ScriptNode)
		if onError != "" {
			require.NoError(t, sn.Configure(map[string]any{"on_error": onError}))
		}
		return sn
	}

	inMsg := message.New(message.WithID("t-1"))

	t.Run("error mode returns ErrScriptTimeout", func(t *testing.T) {
		sn := newTimeoutNode(t, "error")
		_, err := sn.Process(context.Background(), inMsg)
		assert.ErrorIs(t, err, ErrScriptTimeout)
	})

	t.Run("ignore mode passes original through on timeout", func(t *testing.T) {
		sn := newTimeoutNode(t, "ignore")
		results, err := sn.Process(context.Background(), inMsg)
		assert.NoError(t, err)
		require.Len(t, results, 1)
		assert.Same(t, inMsg, results[0])
	})

	t.Run("drop mode emits nothing on timeout", func(t *testing.T) {
		sn := newTimeoutNode(t, "drop")
		results, err := sn.Process(context.Background(), inMsg)
		assert.NoError(t, err)
		assert.Empty(t, results)
	})
}

// TestScriptNode_OnError_SuccessUnaffected 는 성공적인 스크립트는 on_error 값과 무관하게
// 정상 출력을 내는지 검증한다.
func TestScriptNode_OnError_SuccessUnaffected(t *testing.T) {
	for _, mode := range []string{"error", "ignore", "drop"} {
		t.Run(mode, func(t *testing.T) {
			engine := &mockScriptEngine{
				executeFn: func(_ context.Context, msg message.Message) (message.Message, error) {
					return msg, nil // 성공
				},
			}
			def := flow.NewNodeDef("script-ok", "script")
			node, err := NewScriptNode(def, WithScriptEngine(engine))
			require.NoError(t, err)
			sn := node.(*ScriptNode)
			require.NoError(t, sn.Configure(map[string]any{"on_error": mode}))

			inMsg := message.New(message.WithID("ok-1"))
			results, perr := sn.Process(context.Background(), inMsg)
			require.NoError(t, perr)
			require.Len(t, results, 1)
			assert.Same(t, inMsg, results[0], "성공 시 on_error 와 무관하게 결과 통과")
		})
	}
}

// TestScriptNode_OnError_ParsedViaFactoryOption 은 on_error 가 옵션(base.config) 경로로도
// 파싱됨을 검증한다(WithScriptConfigOption 대신 직접 config 옵션 주입).
func TestScriptNode_OnError_ParsedViaConfigInFactory(t *testing.T) {
	engine := &mockScriptEngine{
		executeFn: func(_ context.Context, _ message.Message) (message.Message, error) {
			return nil, errors.New("boom")
		},
	}
	// on_error 를 옵션으로 주입(팩토리에서 base.config["on_error"] 로 읽힘).
	opt := func(b *BaseNode) {
		if b.config == nil {
			b.config = make(map[string]any)
		}
		b.config["on_error"] = "ignore"
	}
	def := flow.NewNodeDef("script-factory-onerr", "script")
	node, err := NewScriptNode(def, WithScriptEngine(engine), opt)
	require.NoError(t, err)
	sn := node.(*ScriptNode)

	inMsg := message.New(message.WithID("f-1"))
	results, perr := sn.Process(context.Background(), inMsg)
	assert.NoError(t, perr, "팩토리 config 경로 on_error=ignore 도 적용되어야 한다")
	require.Len(t, results, 1)
	assert.Same(t, inMsg, results[0])
}
