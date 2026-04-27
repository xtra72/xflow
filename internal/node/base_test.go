package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/observe"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// --- Node 인터페이스 테스트 ---

// TestNode_인터페이스준수 는 BaseNode이 기본 접근자를 제공하는지 확인한다.
// (Node 인터페이스의 Process는 구체 노드가 구현하므로 여기서는 BaseNode 자체를 테스트)
func TestNode_인터페이스준수(t *testing.T) {
	def := flow.NewNodeDef("test-node", "filter")
	base := NewBaseNode(def)

	assert.NotEmpty(t, base.ID())
	assert.Equal(t, "test-node", base.Name())
	assert.Equal(t, "filter", base.Type())
}

// --- BaseNode 생성 테스트 ---

// TestNewBaseNode_기본생성 은 기본 NodeDef로 BaseNode이 올바르게 생성되는지 확인한다.
func TestNewBaseNode_기본생성(t *testing.T) {
	def := flow.NewNodeDef("my-node", "transform")
	base := NewBaseNode(def)

	assert.Equal(t, def.ID, base.ID())
	assert.Equal(t, "my-node", base.Name())
	assert.Equal(t, "transform", base.Type())
	// 초기 상태는 Created
	assert.Equal(t, lifecycle.StateCreated, base.CurrentState())
}

// TestNewBaseNode_포트초기화 는 NodeDef의 포트가 BaseNode에 올바르게 초기화되는지 확인한다.
func TestNewBaseNode_포트초기화(t *testing.T) {
	def := flow.NewNodeDef("port-node", "filter",
		flow.WithInputPorts(
			flow.Port{ID: "p1", Name: "in", Direction: flow.PortInput},
			flow.Port{ID: "p2", Name: "data", Direction: flow.PortInput},
		),
		flow.WithOutputPorts(
			flow.Port{ID: "p3", Name: "out", Direction: flow.PortOutput},
		),
		flow.WithErrorPort(),
	)

	base := NewBaseNode(def)
	ports := base.Ports()

	// 입력 2개 + 출력 1개 + 에러 1개 = 4개
	assert.Len(t, ports, 4)
}

// TestNewBaseNode_에러포트기본생성 는 Errors가 비어있으면 기본 _error 포트가 생성되는지 확인한다.
func TestNewBaseNode_에러포트기본생성(t *testing.T) {
	def := flow.NewNodeDef("default-err", "filter")
	// Errors가 비어있으므로 기본 "_error" 포트가 생성되어야 한다
	base := NewBaseNode(def)

	errPort := base.GetErrorPort()
	require.NotNil(t, errPort)
	assert.Equal(t, "_error", errPort.Name)
	assert.Equal(t, flow.PortError, errPort.Direction)
}

// TestNewBaseNode_커스텀에러포트 는 Errors가 설정되면 해당 포트를 사용하는지 확인한다.
func TestNewBaseNode_커스텀에러포트(t *testing.T) {
	def := flow.NewNodeDef("custom-err", "filter", flow.WithErrorPort())

	base := NewBaseNode(def)

	errPort := base.GetErrorPort()
	require.NotNil(t, errPort)
	assert.Equal(t, "error", errPort.Name)
	assert.Equal(t, flow.PortError, errPort.Direction)
}

// TestNewBaseNode_옵션적용 은 WithLogger, WithMetrics 옵션이 적용되는지 확인한다.
func TestNewBaseNode_옵션적용(t *testing.T) {
	def := flow.NewNodeDef("opt-node", "transform")
	obs := observe.New()
	logger := obs.Loggers.NewLogger("test-node")
	metrics := obs.Metrics

	base := NewBaseNode(def,
		WithLogger(logger),
		WithMetrics(metrics),
	)

	assert.NotNil(t, base.Logger())
	assert.NotNil(t, base.MetricsCollector())
}

// TestNewBaseNode_옵션없이_nil반환 은 로거/메트릭 없이 생성 시 nil을 반환하는지 확인한다.
func TestNewBaseNode_옵션없이_nil반환(t *testing.T) {
	def := flow.NewNodeDef("no-opts", "filter")
	base := NewBaseNode(def)

	assert.Nil(t, base.Logger())
	assert.Nil(t, base.MetricsCollector())
}

// --- Configure / GetConfig 테스트 ---

// TestBaseNode_Configure_정상 은 Configure가 정상적으로 설정을 저장하는지 확인한다.
func TestBaseNode_Configure_정상(t *testing.T) {
	def := flow.NewNodeDef("cfg-node", "filter")
	base := NewBaseNode(def)

	config := map[string]any{
		"key1": "value1",
		"key2": 42,
	}
	err := base.Configure(config)
	require.NoError(t, err)

	got := base.GetConfig()
	assert.Equal(t, "value1", got["key1"])
	assert.Equal(t, 42, got["key2"])
}

// TestBaseNode_Configure_nil에러 는 nil config 전달 시 에러를 반환하는지 확인한다.
func TestBaseNode_Configure_nil에러(t *testing.T) {
	def := flow.NewNodeDef("cfg-nil", "filter")
	base := NewBaseNode(def)

	err := base.Configure(nil)
	assert.Error(t, err)
}

// TestBaseNode_GetConfig_복사본반환 은 GetConfig가 원본이 아닌 복사본을 반환하는지 확인한다.
func TestBaseNode_GetConfig_복사본반환(t *testing.T) {
	def := flow.NewNodeDef("cfg-copy", "filter")
	base := NewBaseNode(def)

	_ = base.Configure(map[string]any{"key": "value"})
	got := base.GetConfig()
	got["key"] = "modified"

	// 원본은 변경되지 않아야 한다
	original := base.GetConfig()
	assert.Equal(t, "value", original["key"])
}

// --- 포트 조회 테스트 ---

// TestBaseNode_GetPort_존재하는포트 는 이름으로 포트를 조회할 수 있는지 확인한다.
func TestBaseNode_GetPort_존재하는포트(t *testing.T) {
	def := flow.NewNodeDef("port-get", "filter")
	base := NewBaseNode(def)

	port, ok := base.GetPort("in")
	assert.True(t, ok)
	assert.Equal(t, "in", port.Name)
	assert.Equal(t, flow.PortInput, port.Direction)
}

// TestBaseNode_GetPort_존재하지않는포트 는 없는 포트 조회 시 false를 반환하는지 확인한다.
func TestBaseNode_GetPort_존재하지않는포트(t *testing.T) {
	def := flow.NewNodeDef("port-miss", "filter")
	base := NewBaseNode(def)

	_, ok := base.GetPort("nonexistent")
	assert.False(t, ok)
}

// TestBaseNode_GetOutputPort_정상 은 출력 포트를 이름으로 조회하는지 확인한다.
func TestBaseNode_GetOutputPort_정상(t *testing.T) {
	def := flow.NewNodeDef("out-port", "filter")
	base := NewBaseNode(def)

	port, ok := base.GetOutputPort("out")
	assert.True(t, ok)
	assert.Equal(t, "out", port.Name)
	assert.Equal(t, flow.PortOutput, port.Direction)
}

// TestBaseNode_GetOutputPort_없는포트 는 없는 출력 포트 조회 시 false를 반환하는지 확인한다.
func TestBaseNode_GetOutputPort_없는포트(t *testing.T) {
	def := flow.NewNodeDef("out-miss", "filter")
	base := NewBaseNode(def)

	// "in"은 입력 포트이므로 출력 포트로 조회하면 false여야 한다
	_, ok := base.GetOutputPort("in")
	assert.False(t, ok)
}

// --- Ports() 테스트 ---

// TestBaseNode_Ports_전체목록 은 모든 포트(입력+출력+에러)를 반환하는지 확인한다.
func TestBaseNode_Ports_전체목록(t *testing.T) {
	def := flow.NewNodeDef("all-ports", "filter", flow.WithErrorPort())
	base := NewBaseNode(def)

	ports := base.Ports()
	// 기본 입력 1개 + 기본 출력 1개 + 에러 1개 = 3개
	assert.Len(t, ports, 3)

	// 방향별 개수 확인
	var inputs, outputs, errs int
	for _, p := range ports {
		switch p.Direction {
		case flow.PortInput:
			inputs++
		case flow.PortOutput:
			outputs++
		case flow.PortError:
			errs++
		}
	}
	assert.Equal(t, 1, inputs)
	assert.Equal(t, 1, outputs)
	assert.Equal(t, 1, errs)
}

// --- Lifecycle 임베딩 테스트 ---

// TestBaseNode_Lifecycle_임베딩 은 BaseLifecycle이 올바르게 임베딩되는지 확인한다.
func TestBaseNode_Lifecycle_임베딩(t *testing.T) {
	def := flow.NewNodeDef("lc-node", "filter")
	base := NewBaseNode(def)

	// 초기 상태 확인
	assert.Equal(t, lifecycle.StateCreated, base.CurrentState())

	// 상태 전이 테스트
	err := base.TransitionTo(lifecycle.StateInitializing)
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateInitializing, base.CurrentState())

	err = base.TransitionTo(lifecycle.StateRunning)
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, base.CurrentState())
}

// --- 동시성 테스트 ---

// TestBaseNode_동시성안전_Configure 는 Configure가 동시성 안전한지 확인한다.
func TestBaseNode_동시성안전_Configure(t *testing.T) {
	def := flow.NewNodeDef("conc-node", "filter")
	base := NewBaseNode(def)

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			_ = base.Configure(map[string]any{"key": n})
			_ = base.GetConfig()
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestBaseNode_동시성안전_Ports 는 Ports/GetPort가 동시성 안전한지 확인한다.
func TestBaseNode_동시성안전_Ports(t *testing.T) {
	def := flow.NewNodeDef("conc-ports", "filter")
	base := NewBaseNode(def)

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_ = base.Ports()
			_, _ = base.GetPort("in")
			_, _ = base.GetOutputPort("out")
			_ = base.GetErrorPort()
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

// --- NodePort 구조체 테스트 ---

// TestNodePort_Connected기본값 은 NodePort의 Connected 기본값이 false인지 확인한다.
func TestNodePort_Connected기본값(t *testing.T) {
	port := NodePort{
		ID:        "p1",
		Name:      "test",
		Direction: flow.PortInput,
	}
	assert.False(t, port.Connected)
}

// --- Node 인터페이스 컴파일 타임 검증 ---

// 컴파일 타임에 Node 인터페이스가 Process 메서드를 포함하는지 확인하기 위한
// 더미 구현 (실제 노드 타입이 아닌 테스트용)
type testNode struct {
	*BaseNode
}

func (t *testNode) Init(ctx context.Context) error {
	return t.BaseNode.TransitionTo(lifecycle.StateInitializing)
}

func (t *testNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	return []message.Message{msg}, nil
}

func (t *testNode) Shutdown(_ context.Context) error {
	return t.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Node 인터페이스 준수를 컴파일 타임에 검증
var _ Node = (*testNode)(nil)
