package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// --- 모의 객체 ---

// mockScriptEngine 은 테스트용 ScriptEngine 구현이다.
type mockScriptEngine struct {
	compileErr error
	executeErr error
	executeFn  func(ctx context.Context, msg message.Message) (message.Message, error)
	compiled   bool
	closed     bool
	source     string
}

func (m *mockScriptEngine) Compile(source string) error {
	m.source = source
	if m.compileErr != nil {
		return m.compileErr
	}
	m.compiled = true
	return nil
}

func (m *mockScriptEngine) Execute(ctx context.Context, msg message.Message) (message.Message, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, msg)
	}
	if m.executeErr != nil {
		return nil, m.executeErr
	}
	return msg, nil
}

func (m *mockScriptEngine) Close() error {
	m.closed = true
	return nil
}

// --- ScriptNode 인터페이스 준수 ---

var _ Node = (*ScriptNode)(nil)

// --- NewScriptNode 테스트 ---

// TestNewScriptNode_정상생성 은 ScriptNode가 올바르게 생성되는지 확인한다.
func TestNewScriptNode_정상생성(t *testing.T) {
	def := flow.NewNodeDef("script-1", "script")
	node, err := NewScriptNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "script-1", node.Name())
	assert.Equal(t, "script", node.Type())
}

// TestNewScriptNode_기본타임아웃 은 기본 scriptTimeout이 5초인지 확인한다.
func TestNewScriptNode_기본타임아웃(t *testing.T) {
	def := flow.NewNodeDef("script-timeout", "script")
	node, _ := NewScriptNode(def)
	sn := node.(*ScriptNode)
	assert.Equal(t, 5*time.Second, sn.scriptTimeout)
}

// TestNewScriptNode_커스텀타임아웃 은 WithScriptTimeout이 적용되는지 확인한다.
func TestNewScriptNode_커스텀타임아웃(t *testing.T) {
	def := flow.NewNodeDef("script-custom-timeout", "script")
	node, _ := NewScriptNode(def, WithScriptTimeout(2*time.Second))
	sn := node.(*ScriptNode)
	assert.Equal(t, 2*time.Second, sn.scriptTimeout)
}

// TestNewScriptNode_엔진주입 은 WithScriptEngine이 적용되는지 확인한다.
func TestNewScriptNode_엔진주입(t *testing.T) {
	engine := &mockScriptEngine{}
	def := flow.NewNodeDef("script-engine", "script")
	node, _ := NewScriptNode(def, WithScriptEngine(engine))
	sn := node.(*ScriptNode)
	assert.NotNil(t, sn.engine)
}

// --- Init 테스트 ---

// TestScriptNode_Init_엔진없음_정상 은 엔진 없이도 Init이 성공하는지 확인한다.
func TestScriptNode_Init_엔진없음_정상(t *testing.T) {
	def := flow.NewNodeDef("script-init-no-engine", "script")
	node, _ := NewScriptNode(def)
	sn := node.(*ScriptNode)

	err := sn.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, sn.CurrentState())
}

// TestScriptNode_Init_컴파일성공 은 엔진이 있고 소스가 있으면 컴파일하는지 확인한다.
func TestScriptNode_Init_컴파일성공(t *testing.T) {
	engine := &mockScriptEngine{}
	def := flow.NewNodeDef("script-init-compile", "script",
		flow.WithNodeConfig("script", "print('hello')"),
	)
	node, _ := NewScriptNode(def, WithScriptEngine(engine))
	sn := node.(*ScriptNode)
	sn.scriptSource = "print('hello')"

	err := sn.Init(context.Background())
	require.NoError(t, err)
	assert.True(t, engine.compiled)
	assert.Equal(t, lifecycle.StateRunning, sn.CurrentState())
}

// TestScriptNode_Init_컴파일실패 는 컴파일 실패 시 에러를 반환하는지 확인한다.
func TestScriptNode_Init_컴파일실패(t *testing.T) {
	engine := &mockScriptEngine{compileErr: errors.New("문법 오류")}
	def := flow.NewNodeDef("script-init-fail", "script")
	node, _ := NewScriptNode(def, WithScriptEngine(engine))
	sn := node.(*ScriptNode)
	sn.scriptSource = "invalid syntax"

	err := sn.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrScriptCompileFailed)
}

// --- Process 테스트 ---

// TestScriptNode_Process_엔진nil_패스스루 는 엔진이 nil이면 메시지를 통과시키는지 확인한다.
func TestScriptNode_Process_엔진nil_패스스루(t *testing.T) {
	def := flow.NewNodeDef("script-nil-engine", "script")
	node, _ := NewScriptNode(def)

	msg := message.New()
	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, msg.ID(), results[0].ID())
}

// TestScriptNode_Process_실행성공 은 스크립트 실행이 성공하면 결과를 반환하는지 확인한다.
func TestScriptNode_Process_실행성공(t *testing.T) {
	resultMsg := message.New()
	engine := &mockScriptEngine{
		executeFn: func(_ context.Context, _ message.Message) (message.Message, error) {
			return resultMsg, nil
		},
	}

	def := flow.NewNodeDef("script-exec-ok", "script")
	node, _ := NewScriptNode(def, WithScriptEngine(engine))

	msg := message.New()
	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, resultMsg.ID(), results[0].ID())
}

// TestScriptNode_Process_실행에러 는 스크립트 실행 에러 시 ErrScriptExecutionFailed를 반환하는지 확인한다.
func TestScriptNode_Process_실행에러(t *testing.T) {
	engine := &mockScriptEngine{
		executeFn: func(_ context.Context, _ message.Message) (message.Message, error) {
			return nil, errors.New("런타임 에러")
		},
	}

	def := flow.NewNodeDef("script-exec-err", "script")
	node, _ := NewScriptNode(def, WithScriptEngine(engine))

	msg := message.New()
	_, err := node.Process(context.Background(), msg)
	assert.ErrorIs(t, err, ErrScriptExecutionFailed)
}

// TestScriptNode_Process_타임아웃 은 스크립트 실행 타임아웃 시 ErrScriptTimeout을 반환하는지 확인한다.
func TestScriptNode_Process_타임아웃(t *testing.T) {
	engine := &mockScriptEngine{
		executeFn: func(ctx context.Context, _ message.Message) (message.Message, error) {
			// 타임아웃까지 대기
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	def := flow.NewNodeDef("script-timeout", "script")
	node, _ := NewScriptNode(def, WithScriptEngine(engine), WithScriptTimeout(50*time.Millisecond))

	msg := message.New()
	_, err := node.Process(context.Background(), msg)
	assert.ErrorIs(t, err, ErrScriptTimeout)
}

// --- Shutdown 테스트 ---

// TestScriptNode_Shutdown_엔진Close호출 은 Shutdown 시 엔진의 Close가 호출되는지 확인한다.
func TestScriptNode_Shutdown_엔진Close호출(t *testing.T) {
	engine := &mockScriptEngine{}
	def := flow.NewNodeDef("script-shut", "script")
	node, _ := NewScriptNode(def, WithScriptEngine(engine))
	sn := node.(*ScriptNode)

	_ = sn.Init(context.Background())
	err := sn.Shutdown(context.Background())
	require.NoError(t, err)
	assert.True(t, engine.closed)
	assert.Equal(t, lifecycle.StateStopping, sn.CurrentState())
}

// TestScriptNode_Shutdown_엔진nil_정상 은 엔진이 nil이어도 Shutdown이 정상 동작하는지 확인한다.
func TestScriptNode_Shutdown_엔진nil_정상(t *testing.T) {
	def := flow.NewNodeDef("script-shut-nil", "script")
	node, _ := NewScriptNode(def)
	sn := node.(*ScriptNode)

	_ = sn.Init(context.Background())
	err := sn.Shutdown(context.Background())
	require.NoError(t, err)
}

// --- Configure 테스트 ---

// TestScriptNode_Configure_스크립트소스변경 은 Configure로 스크립트를 변경할 수 있는지 확인한다.
func TestScriptNode_Configure_스크립트소스변경(t *testing.T) {
	engine := &mockScriptEngine{}
	def := flow.NewNodeDef("script-cfg", "script")
	node, _ := NewScriptNode(def, WithScriptEngine(engine))

	err := node.Configure(map[string]any{"script": "new_script()"})
	require.NoError(t, err)

	sn := node.(*ScriptNode)
	assert.Equal(t, "new_script()", sn.scriptSource)
	assert.True(t, engine.compiled)
	assert.Equal(t, "new_script()", engine.source)
}

// TestScriptNode_Configure_재컴파일실패 는 재컴파일 실패 시 에러를 반환하는지 확인한다.
func TestScriptNode_Configure_재컴파일실패(t *testing.T) {
	engine := &mockScriptEngine{compileErr: errors.New("컴파일 에러")}
	def := flow.NewNodeDef("script-cfg-fail", "script")
	node, _ := NewScriptNode(def, WithScriptEngine(engine))

	err := node.Configure(map[string]any{"script": "bad_syntax"})
	assert.ErrorIs(t, err, ErrScriptCompileFailed)
}

// --- 동시성 테스트 ---

// TestScriptNode_동시성안전_Process 는 Process가 동시성 안전한지 확인한다.
func TestScriptNode_동시성안전_Process(t *testing.T) {
	engine := &mockScriptEngine{
		executeFn: func(_ context.Context, msg message.Message) (message.Message, error) {
			return msg, nil
		},
	}

	def := flow.NewNodeDef("script-conc", "script")
	node, _ := NewScriptNode(def, WithScriptEngine(engine))

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			msg := message.New()
			_, _ = node.Process(context.Background(), msg)
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
