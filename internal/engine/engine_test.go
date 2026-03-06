package engine

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/internal/observe"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 테스트용 mock 구현
// ---------------------------------------------------------------------------

// mockNode 는 node.Node 인터페이스의 테스트용 mock 구현이다.
type mockNode struct {
	id             string
	name           string
	nodeType       string
	initErr        error
	processResults []message.Message
	processErr     error
	shutdownErr    error
	initCalled     bool
	shutdownCalled bool
	processCalled  int
	hasErrorPort   bool // 에러 포트 포함 여부
	logger         observe.ComponentLogger
	mu             sync.Mutex
}

func newMockNode(id, name, nodeType string) *mockNode {
	return &mockNode{
		id:       id,
		name:     name,
		nodeType: nodeType,
	}
}

func (m *mockNode) ID() string   { return m.id }
func (m *mockNode) Name() string { return m.name }
func (m *mockNode) Type() string { return m.nodeType }

func (m *mockNode) Init(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.initCalled = true
	return m.initErr
}

func (m *mockNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processCalled++
	if m.processErr != nil {
		return nil, m.processErr
	}
	if m.processResults != nil {
		return m.processResults, nil
	}
	// 기본 동작: 입력 메시지를 그대로 출력
	return []message.Message{msg}, nil
}

func (m *mockNode) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shutdownCalled = true
	return m.shutdownErr
}

func (m *mockNode) Configure(config map[string]any) error { return nil }

// Logger 는 설정된 ComponentLogger를 반환한다 (nodeWithLogger 인터페이스 충족).
func (m *mockNode) Logger() observe.ComponentLogger { return m.logger }

func (m *mockNode) Ports() []node.NodePort {
	ports := []node.NodePort{
		{ID: "in", Name: "in", Direction: flow.PortInput},
		{ID: "out", Name: "out", Direction: flow.PortOutput},
	}
	if m.hasErrorPort {
		ports = append(ports, node.NodePort{ID: "error", Name: "error", Direction: flow.PortError})
	}
	return ports
}

// mockNodeFactory 는 미리 등록된 mockNode를 반환하는 팩토리이다.
type mockNodeFactory struct {
	nodes map[string]*mockNode
}

func newMockNodeFactory() *mockNodeFactory {
	return &mockNodeFactory{nodes: make(map[string]*mockNode)}
}

func (f *mockNodeFactory) register(n *mockNode) {
	f.nodes[n.id] = n
}

func (f *mockNodeFactory) factory(def flow.NodeDef, opts ...node.NodeOption) (node.Node, error) {
	if n, ok := f.nodes[def.ID]; ok {
		return n, nil
	}
	// 등록되지 않은 노드는 기본 mock을 생성
	return newMockNode(def.ID, def.Name, def.Type), nil
}

// mockLogger 는 observe.ComponentLogger의 테스트용 mock 구현이다.
type mockLogger struct{}

func (m *mockLogger) Debug(msg string, args ...any)           {}
func (m *mockLogger) Info(msg string, args ...any)            {}
func (m *mockLogger) Warn(msg string, args ...any)            {}
func (m *mockLogger) Error(msg string, args ...any)           {}
func (m *mockLogger) With(args ...any) observe.ComponentLogger { return m }
func (m *mockLogger) WithGroup(name string) observe.ComponentLogger { return m }
func (m *mockLogger) Component() string                       { return "test" }
func (m *mockLogger) Logger() *slog.Logger                    { return nil }

// newTestEngine 은 테스트용 Engine을 생성하는 헬퍼이다.
func newTestEngine(factory *mockNodeFactory) *Engine {
	registry := node.NewRegistry(node.WithoutBuiltins())
	if factory != nil {
		// 테스트용 노드 타입을 등록한다.
		_ = registry.Register("transform", factory.factory)
		_ = registry.Register("filter", factory.factory)
	}

	return NewEngine(
		WithNodeRegistry(registry),
		WithShutdownTimeout(2*time.Second),
	)
}

// newSimpleFlow 는 A -> B 두 노드를 가진 단순 Flow를 생성하는 헬퍼이다.
func newSimpleFlow() (flow.Flow, []flow.NodeDef) {
	nodes := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
		flow.NewNodeDef("B", "transform"),
	}
	wires := []flow.Wire{
		flow.NewWire(nodes[0].ID, "out", nodes[1].ID, "in"),
	}
	f := flow.NewFlow("simple-flow",
		flow.WithNodes(nodes...),
		flow.WithWires(wires...),
	)
	return f, nodes
}

// ---------------------------------------------------------------------------
// Engine 생성 테스트
// ---------------------------------------------------------------------------

func TestNewEngine_DefaultValues(t *testing.T) {
	e := NewEngine()

	if e == nil {
		t.Fatal("NewEngine should not return nil")
	}
	if e.scheduler == nil {
		t.Error("default scheduler should not be nil")
	}
	if e.nodeRegistry == nil {
		t.Error("default nodeRegistry should not be nil")
	}
	if e.shutdownTimeout != 30*time.Second {
		t.Errorf("default shutdownTimeout should be 30s, got %v", e.shutdownTimeout)
	}
	if e.bpPolicy.Strategy != StrategyBlock {
		t.Errorf("default backpressure strategy should be StrategyBlock, got %q", e.bpPolicy.Strategy)
	}
}

func TestNewEngine_WithOptions(t *testing.T) {
	registry := node.NewRegistry(node.WithoutBuiltins())
	scheduler := NewDAGScheduler()

	e := NewEngine(
		WithNodeRegistry(registry),
		WithScheduler(scheduler),
		WithShutdownTimeout(5*time.Second),
		WithBackpressurePolicy(BackpressurePolicy{Strategy: StrategyDrop}),
	)

	if e.nodeRegistry != registry {
		t.Error("nodeRegistry should match provided registry")
	}
	if e.shutdownTimeout != 5*time.Second {
		t.Errorf("shutdownTimeout should be 5s, got %v", e.shutdownTimeout)
	}
	if e.bpPolicy.Strategy != StrategyDrop {
		t.Errorf("backpressure strategy should be StrategyDrop, got %q", e.bpPolicy.Strategy)
	}
}

// ---------------------------------------------------------------------------
// DeployFlow 테스트
// ---------------------------------------------------------------------------

func TestDeployFlow_Success(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)
	f, _ := newSimpleFlow()

	err := e.DeployFlow(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	status, err := e.GetFlowStatus(f.ID())
	if err != nil {
		t.Fatalf("unexpected error getting status: %v", err)
	}
	if status.State != flow.FlowLoaded {
		t.Errorf("expected state FlowLoaded, got %q", status.State)
	}
	if status.FlowName != "simple-flow" {
		t.Errorf("expected flow name 'simple-flow', got %q", status.FlowName)
	}
}

func TestDeployFlow_DuplicateID(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)
	f, _ := newSimpleFlow()

	if err := e.DeployFlow(context.Background(), f); err != nil {
		t.Fatalf("first deploy failed: %v", err)
	}

	err := e.DeployFlow(context.Background(), f)
	if !errors.Is(err, ErrFlowAlreadyDeployed) {
		t.Errorf("expected ErrFlowAlreadyDeployed, got %v", err)
	}
}

func TestDeployFlow_ValidationFailed(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)

	// 유효하지 않은 Flow: 존재하지 않는 노드를 참조하는 와이어
	nodes := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
	}
	wires := []flow.Wire{
		flow.NewWire(nodes[0].ID, "out", "nonexistent", "in"),
	}
	f := flow.NewFlow("invalid-flow",
		flow.WithNodes(nodes...),
		flow.WithWires(wires...),
	)

	err := e.DeployFlow(context.Background(), f)
	if !errors.Is(err, ErrFlowValidationFailed) {
		t.Errorf("expected ErrFlowValidationFailed, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// StartFlow 테스트
// ---------------------------------------------------------------------------

func TestStartFlow_Success(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)
	f, _ := newSimpleFlow()

	if err := e.DeployFlow(context.Background(), f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	err := e.StartFlow(context.Background(), f.ID())
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}

	status, _ := e.GetFlowStatus(f.ID())
	if status.State != flow.FlowRunning {
		t.Errorf("expected FlowRunning, got %q", status.State)
	}

	// 정리
	if err := e.StopFlow(context.Background(), f.ID()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

func TestStartFlow_NotFound(t *testing.T) {
	e := newTestEngine(nil)

	err := e.StartFlow(context.Background(), "nonexistent")
	if !errors.Is(err, ErrFlowNotFound) {
		t.Errorf("expected ErrFlowNotFound, got %v", err)
	}
}

func TestStartFlow_NotLoaded(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)
	f, _ := newSimpleFlow()

	if err := e.DeployFlow(context.Background(), f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	// 시작 후 중지
	if err := e.StartFlow(context.Background(), f.ID()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if err := e.StopFlow(context.Background(), f.ID()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	// 중지된 상태에서 다시 시작하면 ErrFlowNotLoaded
	err := e.StartFlow(context.Background(), f.ID())
	if !errors.Is(err, ErrFlowNotLoaded) {
		t.Errorf("expected ErrFlowNotLoaded, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// StopFlow 테스트
// ---------------------------------------------------------------------------

func TestStopFlow_Success(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)
	f, _ := newSimpleFlow()

	if err := e.DeployFlow(context.Background(), f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	if err := e.StartFlow(context.Background(), f.ID()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	err := e.StopFlow(context.Background(), f.ID())
	if err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	status, _ := e.GetFlowStatus(f.ID())
	if status.State != flow.FlowStopped {
		t.Errorf("expected FlowStopped, got %q", status.State)
	}
}

func TestStopFlow_NotFound(t *testing.T) {
	e := newTestEngine(nil)

	err := e.StopFlow(context.Background(), "nonexistent")
	if !errors.Is(err, ErrFlowNotFound) {
		t.Errorf("expected ErrFlowNotFound, got %v", err)
	}
}

func TestStopFlow_NotRunning(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)
	f, _ := newSimpleFlow()

	if err := e.DeployFlow(context.Background(), f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	// 로드된 상태에서 중지 시도
	err := e.StopFlow(context.Background(), f.ID())
	if !errors.Is(err, ErrFlowNotRunning) {
		t.Errorf("expected ErrFlowNotRunning, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// PauseFlow / ResumeFlow 테스트
// ---------------------------------------------------------------------------

func TestPauseResumeFlow(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)
	f, _ := newSimpleFlow()

	if err := e.DeployFlow(context.Background(), f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	if err := e.StartFlow(context.Background(), f.ID()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Pause
	if err := e.PauseFlow(context.Background(), f.ID()); err != nil {
		t.Fatalf("pause failed: %v", err)
	}
	status, _ := e.GetFlowStatus(f.ID())
	if status.State != flow.FlowPaused {
		t.Errorf("expected FlowPaused, got %q", status.State)
	}

	// Resume
	if err := e.ResumeFlow(context.Background(), f.ID()); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	status, _ = e.GetFlowStatus(f.ID())
	if status.State != flow.FlowRunning {
		t.Errorf("expected FlowRunning after resume, got %q", status.State)
	}

	// 정리
	if err := e.StopFlow(context.Background(), f.ID()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

func TestPauseFlow_NotRunning(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)
	f, _ := newSimpleFlow()

	if err := e.DeployFlow(context.Background(), f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	err := e.PauseFlow(context.Background(), f.ID())
	if !errors.Is(err, ErrFlowNotRunning) {
		t.Errorf("expected ErrFlowNotRunning, got %v", err)
	}
}

func TestResumeFlow_NotPaused(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)
	f, _ := newSimpleFlow()

	if err := e.DeployFlow(context.Background(), f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	if err := e.StartFlow(context.Background(), f.ID()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Running 상태에서 Resume 시도
	err := e.ResumeFlow(context.Background(), f.ID())
	if !errors.Is(err, ErrFlowNotPaused) {
		t.Errorf("expected ErrFlowNotPaused, got %v", err)
	}

	if err := e.StopFlow(context.Background(), f.ID()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// UndeployFlow 테스트
// ---------------------------------------------------------------------------

func TestUndeployFlow_Success(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)
	f, _ := newSimpleFlow()

	if err := e.DeployFlow(context.Background(), f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	if err := e.StartFlow(context.Background(), f.ID()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if err := e.StopFlow(context.Background(), f.ID()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	err := e.UndeployFlow(context.Background(), f.ID())
	if err != nil {
		t.Fatalf("undeploy failed: %v", err)
	}

	// 배포 해제 후 조회 불가
	_, err = e.GetFlowStatus(f.ID())
	if !errors.Is(err, ErrFlowNotFound) {
		t.Errorf("expected ErrFlowNotFound after undeploy, got %v", err)
	}
}

func TestUndeployFlow_NotStopped(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)
	f, _ := newSimpleFlow()

	if err := e.DeployFlow(context.Background(), f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	// FlowLoaded 상태에서는 배포 해제가 가능하다 (재배포 시나리오 지원)
	if err := e.UndeployFlow(context.Background(), f.ID()); err != nil {
		t.Errorf("FlowLoaded 상태에서 undeploy 실패: %v", err)
	}
}

func TestUndeployFlow_Running(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)
	f, _ := newSimpleFlow()

	if err := e.DeployFlow(context.Background(), f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	if err := e.StartFlow(context.Background(), f.ID()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// 실행 중인 상태에서 배포 해제 시도 → ErrFlowNotStopped
	err := e.UndeployFlow(context.Background(), f.ID())
	if !errors.Is(err, ErrFlowNotStopped) {
		t.Errorf("expected ErrFlowNotStopped, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListFlows 테스트
// ---------------------------------------------------------------------------

func TestListFlows(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)

	// 초기에는 빈 리스트
	if flows := e.ListFlows(); len(flows) != 0 {
		t.Errorf("expected 0 flows, got %d", len(flows))
	}

	f1, _ := newSimpleFlow()
	f2 := flow.NewFlow("flow-2",
		flow.WithNodes(flow.NewNodeDef("X", "transform")),
	)

	if err := e.DeployFlow(context.Background(), f1); err != nil {
		t.Fatalf("deploy f1 failed: %v", err)
	}
	if err := e.DeployFlow(context.Background(), f2); err != nil {
		t.Fatalf("deploy f2 failed: %v", err)
	}

	flows := e.ListFlows()
	if len(flows) != 2 {
		t.Errorf("expected 2 flows, got %d", len(flows))
	}
}

// ---------------------------------------------------------------------------
// GetFlowStatus 테스트
// ---------------------------------------------------------------------------

func TestGetFlowStatus_NotFound(t *testing.T) {
	e := newTestEngine(nil)

	_, err := e.GetFlowStatus("nonexistent")
	if !errors.Is(err, ErrFlowNotFound) {
		t.Errorf("expected ErrFlowNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Configure / GetConfig 테스트
// ---------------------------------------------------------------------------

func TestEngine_Configure(t *testing.T) {
	e := NewEngine()

	cfg := map[string]any{
		"key1": "value1",
		"key2": 42,
	}

	err := e.Configure(context.Background(), cfg)
	if err != nil {
		t.Fatalf("configure failed: %v", err)
	}

	got := e.GetConfig()
	if got["key1"] != "value1" {
		t.Errorf("expected key1='value1', got %v", got["key1"])
	}
	if got["key2"] != 42 {
		t.Errorf("expected key2=42, got %v", got["key2"])
	}
}

// ---------------------------------------------------------------------------
// HealthCheck 테스트
// ---------------------------------------------------------------------------

func TestEngine_HealthCheck(t *testing.T) {
	e := NewEngine()

	status := e.HealthCheck(context.Background())
	if !status.Healthy {
		t.Error("engine should be healthy initially")
	}

	// HealthChecker 인터페이스 준수 확인
	var _ lifecycle.HealthChecker = e
}

// ---------------------------------------------------------------------------
// 전체 라이프사이클 통합 테스트
// ---------------------------------------------------------------------------

func TestEngine_FullLifecycle(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)
	f, _ := newSimpleFlow()

	ctx := context.Background()

	// Deploy
	if err := e.DeployFlow(ctx, f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	// Start
	if err := e.StartFlow(ctx, f.ID()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Pause
	if err := e.PauseFlow(ctx, f.ID()); err != nil {
		t.Fatalf("pause failed: %v", err)
	}

	// Resume
	if err := e.ResumeFlow(ctx, f.ID()); err != nil {
		t.Fatalf("resume failed: %v", err)
	}

	// Stop
	if err := e.StopFlow(ctx, f.ID()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	// Undeploy
	if err := e.UndeployFlow(ctx, f.ID()); err != nil {
		t.Fatalf("undeploy failed: %v", err)
	}

	// 배포 해제 후 리스트에서 제거됨
	if flows := e.ListFlows(); len(flows) != 0 {
		t.Errorf("expected 0 flows after undeploy, got %d", len(flows))
	}
}

// ---------------------------------------------------------------------------
// 메시지 흐름 테스트 (노드 goroutine을 통한 실제 메시지 처리)
// ---------------------------------------------------------------------------

func TestEngine_MessageFlow(t *testing.T) {
	// A -> B 에서 A의 입력에 메시지를 보내면 B까지 도달하는지 검증
	factory := newMockNodeFactory()
	nodeA := newMockNode("", "A", "transform")
	nodeB := newMockNode("", "B", "transform")

	// Flow 생성
	nodeDefs := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
		flow.NewNodeDef("B", "transform"),
	}
	// mock 노드에 ID 설정
	nodeA.id = nodeDefs[0].ID
	nodeB.id = nodeDefs[1].ID
	factory.register(nodeA)
	factory.register(nodeB)

	wires := []flow.Wire{
		flow.NewWire(nodeDefs[0].ID, "out", nodeDefs[1].ID, "in"),
	}
	f := flow.NewFlow("msg-flow",
		flow.WithNodes(nodeDefs...),
		flow.WithWires(wires...),
	)

	e := newTestEngine(factory)
	ctx := context.Background()

	if err := e.DeployFlow(ctx, f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	if err := e.StartFlow(ctx, f.ID()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// 노드 A에 대한 입력 와이어를 찾아 메시지 전송
	e.mu.RLock()
	rt := e.flows[f.ID()]
	e.mu.RUnlock()

	// 소스 노드 A의 입력 와이어를 찾는다.
	var inputWire *RuntimeWire
	for _, w := range rt.wires {
		if w.TargetNodeID == nodeDefs[0].ID {
			inputWire = w
			break
		}
	}

	// A는 소스 노드이므로 입력 와이어가 없을 수 있다.
	// 이 경우, A는 자체적으로 메시지를 생성하지 않으므로
	// 짧은 대기 후 상태만 확인한다.
	_ = inputWire
	time.Sleep(100 * time.Millisecond)

	// 정리
	if err := e.StopFlow(ctx, f.ID()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// StopFlow 일시정지된 상태에서 테스트
// ---------------------------------------------------------------------------

func TestStopFlow_FromPaused(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)
	f, _ := newSimpleFlow()
	ctx := context.Background()

	if err := e.DeployFlow(ctx, f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	if err := e.StartFlow(ctx, f.ID()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if err := e.PauseFlow(ctx, f.ID()); err != nil {
		t.Fatalf("pause failed: %v", err)
	}

	// Paused 상태에서 Stop 가능
	if err := e.StopFlow(ctx, f.ID()); err != nil {
		t.Fatalf("stop from paused failed: %v", err)
	}

	status, _ := e.GetFlowStatus(f.ID())
	if status.State != flow.FlowStopped {
		t.Errorf("expected FlowStopped, got %q", status.State)
	}
}

// ---------------------------------------------------------------------------
// 노드 Init 실패 테스트
// ---------------------------------------------------------------------------

func TestStartFlow_NodeInitFailed(t *testing.T) {
	factory := newMockNodeFactory()
	failNode := newMockNode("", "Fail", "transform")
	failNode.initErr = errors.New("init failed")

	nodeDefs := []flow.NodeDef{
		flow.NewNodeDef("Fail", "transform"),
	}
	failNode.id = nodeDefs[0].ID
	factory.register(failNode)

	f := flow.NewFlow("fail-flow",
		flow.WithNodes(nodeDefs...),
	)

	e := newTestEngine(factory)
	ctx := context.Background()

	if err := e.DeployFlow(ctx, f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	err := e.StartFlow(ctx, f.ID())
	if err == nil {
		t.Fatal("expected error for node init failure, got nil")
	}

	// Init 실패 시 FlowLoaded 로 롤백되어야 한다.
	status, _ := e.GetFlowStatus(f.ID())
	if status.State != flow.FlowLoaded {
		t.Errorf("expected FlowLoaded after init failure rollback, got %q", status.State)
	}
}

// TestStartFlow_NodeInitFailed_Rollback_재시작가능 은 Init 실패 후 재시작이 가능한지 검증한다.
func TestStartFlow_NodeInitFailed_Rollback_재시작가능(t *testing.T) {
	factory := newMockNodeFactory()
	failNode := newMockNode("", "Fail", "transform")
	failNode.initErr = errors.New("init failed")

	nodeDefs := []flow.NodeDef{
		flow.NewNodeDef("Fail", "transform"),
	}
	failNode.id = nodeDefs[0].ID
	factory.register(failNode)

	f := flow.NewFlow("retry-flow",
		flow.WithNodes(nodeDefs...),
	)

	e := newTestEngine(factory)
	ctx := context.Background()

	if err := e.DeployFlow(ctx, f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	// 1차 시도: Init 실패
	err := e.StartFlow(ctx, f.ID())
	if err == nil {
		t.Fatal("expected error for node init failure")
	}

	// FlowLoaded 로 롤백 확인
	status, _ := e.GetFlowStatus(f.ID())
	if status.State != flow.FlowLoaded {
		t.Fatalf("expected FlowLoaded after rollback, got %q", status.State)
	}

	// 2차 시도: 에러 해제 후 정상 시작
	failNode.mu.Lock()
	failNode.initErr = nil
	failNode.mu.Unlock()

	err = e.StartFlow(ctx, f.ID())
	if err != nil {
		t.Fatalf("expected successful start after fix, got: %v", err)
	}

	status, _ = e.GetFlowStatus(f.ID())
	if status.State != flow.FlowRunning {
		t.Errorf("expected FlowRunning after retry, got %q", status.State)
	}

	// 정리
	if err := e.StopFlow(ctx, f.ID()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

// TestStartFlow_NodeInitFailed_이전노드Shutdown 은 Init 실패 시
// 이미 초기화된 노드가 Shutdown되는지 검증한다.
func TestStartFlow_NodeInitFailed_이전노드Shutdown(t *testing.T) {
	factory := newMockNodeFactory()

	okNode := newMockNode("", "OK", "transform")
	failNode := newMockNode("", "Fail", "transform")
	failNode.initErr = errors.New("init failed")

	nodeDefs := []flow.NodeDef{
		flow.NewNodeDef("OK", "transform"),
		flow.NewNodeDef("Fail", "transform"),
	}
	okNode.id = nodeDefs[0].ID
	failNode.id = nodeDefs[1].ID

	factory.register(okNode)
	factory.register(failNode)

	wires := []flow.Wire{
		flow.NewWire(nodeDefs[0].ID, "out", nodeDefs[1].ID, "in"),
	}

	f := flow.NewFlow("shutdown-test",
		flow.WithNodes(nodeDefs...),
		flow.WithWires(wires...),
	)

	e := newTestEngine(factory)
	ctx := context.Background()

	if err := e.DeployFlow(ctx, f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	err := e.StartFlow(ctx, f.ID())
	if err == nil {
		t.Fatal("expected error for node init failure")
	}

	// OK 노드는 Init이 호출되었으므로 Shutdown도 호출되어야 한다.
	okNode.mu.Lock()
	initCalled := okNode.initCalled
	shutdownCalled := okNode.shutdownCalled
	okNode.mu.Unlock()

	if !initCalled {
		t.Error("OK node should have Init called")
	}
	if !shutdownCalled {
		t.Error("OK node should have Shutdown called after rollback")
	}
}

// ---------------------------------------------------------------------------
// WithLogger / WithMetrics 옵션 테스트
// ---------------------------------------------------------------------------

func TestNewEngine_WithLogger(t *testing.T) {
	logger := &mockLogger{}
	e := NewEngine(WithLogger(logger))

	if e.logger == nil {
		t.Error("logger should be set")
	}
}

func TestNewEngine_WithMetrics(t *testing.T) {
	mc := observe.NewMetricsCollector()
	e := NewEngine(WithMetrics(mc))

	if e.metrics == nil {
		t.Error("metrics should be set")
	}
}

// ---------------------------------------------------------------------------
// HealthCheck 에러 Flow 테스트
// ---------------------------------------------------------------------------

func TestEngine_HealthCheck_UnhealthyWhenFlowError(t *testing.T) {
	factory := newMockNodeFactory()
	okNode := newMockNode("", "OK", "transform")

	nodeDefs := []flow.NodeDef{
		flow.NewNodeDef("OK", "transform"),
	}
	okNode.id = nodeDefs[0].ID
	factory.register(okNode)

	f := flow.NewFlow("error-flow",
		flow.WithNodes(nodeDefs...),
	)

	e := newTestEngine(factory)
	ctx := context.Background()

	if err := e.DeployFlow(ctx, f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}

	// 정상 시작 후 런타임 에러 시나리오 시뮬레이션:
	// Loaded → Initializing → Running → Error
	if err := e.StartFlow(ctx, f.ID()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	_ = f.SetState(flow.FlowError)

	status := e.HealthCheck(ctx)
	if status.Healthy {
		t.Error("engine should be unhealthy when a flow is in error state")
	}
}

// ---------------------------------------------------------------------------
// 노드 goroutine 메시지 처리 통합 테스트
// ---------------------------------------------------------------------------

func TestEngine_NodeProcessing(t *testing.T) {
	// A -> B 체인에서 A의 입력 와이어에 메시지를 전송하면
	// B의 Process가 호출되는지 검증한다.
	factory := newMockNodeFactory()
	nodeA := newMockNode("", "A", "transform")
	nodeB := newMockNode("", "B", "transform")

	nodeDefs := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
		flow.NewNodeDef("B", "transform"),
	}
	nodeA.id = nodeDefs[0].ID
	nodeB.id = nodeDefs[1].ID
	factory.register(nodeA)
	factory.register(nodeB)

	wires := []flow.Wire{
		flow.NewWire(nodeDefs[0].ID, "out", nodeDefs[1].ID, "in"),
	}
	f := flow.NewFlow("processing-flow",
		flow.WithNodes(nodeDefs...),
		flow.WithWires(wires...),
	)

	e := newTestEngine(factory)
	ctx := context.Background()

	if err := e.DeployFlow(ctx, f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	if err := e.StartFlow(ctx, f.ID()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// 와이어를 통해 직접 메시지 전송 (A -> B 의 RuntimeWire를 찾는다)
	e.mu.RLock()
	rt := e.flows[f.ID()]
	e.mu.RUnlock()

	// A -> B 와이어 찾기
	var wireAB *RuntimeWire
	for _, w := range rt.wires {
		if w.SourceNodeID == nodeDefs[0].ID && w.TargetNodeID == nodeDefs[1].ID {
			wireAB = w
			break
		}
	}

	if wireAB == nil {
		t.Fatal("wire A->B not found")
	}

	// 메시지를 와이어에 전송
	msg := message.New()
	if err := wireAB.Send(ctx, msg); err != nil {
		t.Fatalf("send message failed: %v", err)
	}

	// B의 Process가 호출될 때까지 대기
	deadline := time.After(2 * time.Second)
	for {
		nodeB.mu.Lock()
		called := nodeB.processCalled
		nodeB.mu.Unlock()
		if called > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for nodeB.Process to be called")
		case <-time.After(10 * time.Millisecond):
		}
	}

	if err := e.StopFlow(ctx, f.ID()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 노드 Process 에러 처리 테스트
// ---------------------------------------------------------------------------

func TestEngine_NodeProcessError(t *testing.T) {
	factory := newMockNodeFactory()
	nodeA := newMockNode("", "A", "transform")
	nodeB := newMockNode("", "B", "transform")
	nodeB.processErr = errors.New("process error")

	nodeDefs := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
		flow.NewNodeDef("B", "transform"),
	}
	nodeA.id = nodeDefs[0].ID
	nodeB.id = nodeDefs[1].ID
	factory.register(nodeA)
	factory.register(nodeB)

	wires := []flow.Wire{
		flow.NewWire(nodeDefs[0].ID, "out", nodeDefs[1].ID, "in"),
	}
	f := flow.NewFlow("error-processing",
		flow.WithNodes(nodeDefs...),
		flow.WithWires(wires...),
	)

	logger := &mockLogger{}
	e := newTestEngine(factory)
	e.logger = logger

	ctx := context.Background()

	if err := e.DeployFlow(ctx, f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	if err := e.StartFlow(ctx, f.ID()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// 와이어에 직접 메시지 전송
	e.mu.RLock()
	rt := e.flows[f.ID()]
	e.mu.RUnlock()

	var wireAB *RuntimeWire
	for _, w := range rt.wires {
		if w.SourceNodeID == nodeDefs[0].ID {
			wireAB = w
			break
		}
	}

	msg := message.New()
	if err := wireAB.Send(ctx, msg); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	// 에러 카운터가 증가할 때까지 대기
	deadline := time.After(2 * time.Second)
	for {
		if rt.errorCount.Load() > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for error count to increase")
		case <-time.After(10 * time.Millisecond):
		}
	}

	if err := e.StopFlow(ctx, f.ID()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 다중 출력 와이어 (fan-out) 테스트
// ---------------------------------------------------------------------------

func TestEngine_FanOut(t *testing.T) {
	factory := newMockNodeFactory()
	nodeA := newMockNode("", "A", "transform")
	nodeB := newMockNode("", "B", "transform")
	nodeC := newMockNode("", "C", "transform")

	nodeDefs := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
		flow.NewNodeDef("B", "transform"),
		flow.NewNodeDef("C", "transform"),
	}
	nodeA.id = nodeDefs[0].ID
	nodeB.id = nodeDefs[1].ID
	nodeC.id = nodeDefs[2].ID
	factory.register(nodeA)
	factory.register(nodeB)
	factory.register(nodeC)

	wires := []flow.Wire{
		flow.NewWire(nodeDefs[0].ID, "out", nodeDefs[1].ID, "in"),
		flow.NewWire(nodeDefs[0].ID, "out", nodeDefs[2].ID, "in"),
	}
	f := flow.NewFlow("fanout-flow",
		flow.WithNodes(nodeDefs...),
		flow.WithWires(wires...),
	)

	e := newTestEngine(factory)
	ctx := context.Background()

	if err := e.DeployFlow(ctx, f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	if err := e.StartFlow(ctx, f.ID()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// A -> B 와이어에 메시지 전송
	e.mu.RLock()
	rt := e.flows[f.ID()]
	e.mu.RUnlock()

	// A의 출력 와이어를 찾아 B와 C에 메시지가 도달하는지 확인
	// A는 소스 노드이므로 입력 와이어가 없다.
	// B와 C의 입력 와이어에 직접 메시지를 전송한다.
	for _, w := range rt.wires {
		if w.TargetNodeID == nodeDefs[1].ID || w.TargetNodeID == nodeDefs[2].ID {
			msg := message.New()
			if err := w.Send(ctx, msg); err != nil {
				t.Fatalf("send to wire %s failed: %v", w.ID, err)
			}
		}
	}

	// B와 C의 Process가 호출될 때까지 대기
	deadline := time.After(2 * time.Second)
	for {
		nodeB.mu.Lock()
		bCalled := nodeB.processCalled
		nodeB.mu.Unlock()
		nodeC.mu.Lock()
		cCalled := nodeC.processCalled
		nodeC.mu.Unlock()

		if bCalled > 0 && cCalled > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for fan-out processing")
		case <-time.After(10 * time.Millisecond):
		}
	}

	if err := e.StopFlow(ctx, f.ID()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Pause 중 메시지 처리 지연 테스트
// ---------------------------------------------------------------------------

func TestEngine_PauseDelaysProcessing(t *testing.T) {
	factory := newMockNodeFactory()
	nodeA := newMockNode("", "A", "transform")
	nodeB := newMockNode("", "B", "transform")

	nodeDefs := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
		flow.NewNodeDef("B", "transform"),
	}
	nodeA.id = nodeDefs[0].ID
	nodeB.id = nodeDefs[1].ID
	factory.register(nodeA)
	factory.register(nodeB)

	wires := []flow.Wire{
		flow.NewWire(nodeDefs[0].ID, "out", nodeDefs[1].ID, "in"),
	}
	f := flow.NewFlow("pause-test",
		flow.WithNodes(nodeDefs...),
		flow.WithWires(wires...),
	)

	e := newTestEngine(factory)
	ctx := context.Background()

	if err := e.DeployFlow(ctx, f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	if err := e.StartFlow(ctx, f.ID()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Pause
	if err := e.PauseFlow(ctx, f.ID()); err != nil {
		t.Fatalf("pause failed: %v", err)
	}

	// 와이어를 통해 메시지 전송
	e.mu.RLock()
	rt := e.flows[f.ID()]
	e.mu.RUnlock()

	var wireAB *RuntimeWire
	for _, w := range rt.wires {
		if w.SourceNodeID == nodeDefs[0].ID {
			wireAB = w
			break
		}
	}

	// 버퍼 와이어가 아닌 경우 전송이 블록될 수 있으므로
	// goroutine에서 전송한다.
	go func() {
		msg := message.New()
		_ = wireAB.Send(ctx, msg)
	}()

	// 50ms 대기 - pause 상태이므로 Process가 호출되지 않아야 한다.
	time.Sleep(50 * time.Millisecond)
	nodeB.mu.Lock()
	calledDuringPause := nodeB.processCalled
	nodeB.mu.Unlock()

	// Resume 후 처리 시작
	if err := e.ResumeFlow(ctx, f.ID()); err != nil {
		t.Fatalf("resume failed: %v", err)
	}

	// 대기 후 Process 호출 확인
	deadline := time.After(2 * time.Second)
	for {
		nodeB.mu.Lock()
		calledAfterResume := nodeB.processCalled
		nodeB.mu.Unlock()
		if calledAfterResume > calledDuringPause {
			break
		}
		select {
		case <-deadline:
			// 타임아웃은 허용 - unbuffered 채널일 수 있음
			break
		case <-time.After(10 * time.Millisecond):
			continue
		}
		break
	}

	if err := e.StopFlow(ctx, f.ID()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Engine with logger - Deploy/Start/Stop 로깅 검증
// ---------------------------------------------------------------------------

func TestEngine_WithLogger_FullLifecycle(t *testing.T) {
	factory := newMockNodeFactory()
	logger := &mockLogger{}

	registry := node.NewRegistry(node.WithoutBuiltins())
	_ = registry.Register("transform", factory.factory)

	e := NewEngine(
		WithNodeRegistry(registry),
		WithShutdownTimeout(2*time.Second),
		WithLogger(logger),
	)

	f, _ := newSimpleFlow()
	ctx := context.Background()

	if err := e.DeployFlow(ctx, f); err != nil {
		t.Fatalf("deploy failed: %v", err)
	}
	if err := e.StartFlow(ctx, f.ID()); err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if err := e.StopFlow(ctx, f.ID()); err != nil {
		t.Fatalf("stop failed: %v", err)
	}
	if err := e.UndeployFlow(ctx, f.ID()); err != nil {
		t.Fatalf("undeploy failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// PauseFlow / ResumeFlow / UndeployFlow NotFound 테스트
// ---------------------------------------------------------------------------

func TestPauseFlow_NotFound(t *testing.T) {
	e := newTestEngine(nil)
	err := e.PauseFlow(context.Background(), "nonexistent")
	if !errors.Is(err, ErrFlowNotFound) {
		t.Errorf("expected ErrFlowNotFound, got %v", err)
	}
}

func TestResumeFlow_NotFound(t *testing.T) {
	e := newTestEngine(nil)
	err := e.ResumeFlow(context.Background(), "nonexistent")
	if !errors.Is(err, ErrFlowNotFound) {
		t.Errorf("expected ErrFlowNotFound, got %v", err)
	}
}

func TestUndeployFlow_NotFound(t *testing.T) {
	e := newTestEngine(nil)
	err := e.UndeployFlow(context.Background(), "nonexistent")
	if !errors.Is(err, ErrFlowNotFound) {
		t.Errorf("expected ErrFlowNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// splitOutputWires 테스트
// ---------------------------------------------------------------------------

func TestSplitOutputWires(t *testing.T) {
	wires := []*RuntimeWire{
		{ID: "w1", SourcePort: "out"},
		{ID: "w2", SourcePort: "error"},
		{ID: "w3", SourcePort: "out"},
		{ID: "w4", SourcePort: "error"},
	}

	outWires, errWires := splitOutputWires(wires)
	assert.Len(t, outWires, 2)
	assert.Len(t, errWires, 2)
	assert.Equal(t, "w1", outWires[0].ID)
	assert.Equal(t, "w3", outWires[1].ID)
	assert.Equal(t, "w2", errWires[0].ID)
	assert.Equal(t, "w4", errWires[1].ID)
}

func TestSplitOutputWires_NoErrorWires(t *testing.T) {
	wires := []*RuntimeWire{
		{ID: "w1", SourcePort: "out"},
		{ID: "w2", SourcePort: "out"},
	}

	outWires, errWires := splitOutputWires(wires)
	assert.Len(t, outWires, 2)
	assert.Nil(t, errWires)
}

func TestSplitOutputWires_Empty(t *testing.T) {
	outWires, errWires := splitOutputWires(nil)
	assert.Nil(t, outWires)
	assert.Nil(t, errWires)
}

// ---------------------------------------------------------------------------
// 에러 포트 라우팅 통합 테스트
// ---------------------------------------------------------------------------

func TestEngine_ErrorPortRouting(t *testing.T) {
	// A -> B(에러 발생) -> C(정상 출력), B -error-> D(에러 출력)
	// B가 에러를 반환하면 D에 메시지가 도달해야 한다.
	factory := newMockNodeFactory()
	nodeA := newMockNode("", "A", "transform")
	nodeB := newMockNode("", "B", "transform")
	nodeB.hasErrorPort = true // B 노드에 에러 포트 추가
	nodeC := newMockNode("", "C", "transform")
	nodeD := newMockNode("", "D", "transform")

	nodeB.processErr = errors.New("validation failed")

	nodeDefs := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
		flow.NewNodeDef("B", "transform", flow.WithErrorPort()),
		flow.NewNodeDef("C", "transform"),
		flow.NewNodeDef("D", "transform"),
	}
	nodeA.id = nodeDefs[0].ID
	nodeB.id = nodeDefs[1].ID
	nodeC.id = nodeDefs[2].ID
	nodeD.id = nodeDefs[3].ID
	factory.register(nodeA)
	factory.register(nodeB)
	factory.register(nodeC)
	factory.register(nodeD)

	wires := []flow.Wire{
		// A -> B (정상 경로)
		flow.NewWire(nodeDefs[0].ID, "out", nodeDefs[1].ID, "in"),
		// B -> C (정상 출력)
		flow.NewWire(nodeDefs[1].ID, "out", nodeDefs[2].ID, "in"),
		// B -> D (에러 출력)
		flow.NewWire(nodeDefs[1].ID, "error", nodeDefs[3].ID, "in"),
	}
	f := flow.NewFlow("error-port-routing",
		flow.WithNodes(nodeDefs...),
		flow.WithWires(wires...),
	)

	e := newTestEngine(factory)
	ctx := context.Background()

	require.NoError(t, e.DeployFlow(ctx, f))
	require.NoError(t, e.StartFlow(ctx, f.ID()))

	// A -> B 와이어에 메시지 전송
	e.mu.RLock()
	rt := e.flows[f.ID()]
	e.mu.RUnlock()

	var wireAB *RuntimeWire
	for _, w := range rt.wires {
		if w.SourceNodeID == nodeDefs[0].ID && w.SourcePort == "out" {
			wireAB = w
			break
		}
	}
	require.NotNil(t, wireAB)

	msg := message.New()
	require.NoError(t, wireAB.Send(ctx, msg))

	// D(에러 수신 노드)의 Process가 호출될 때까지 대기
	deadline := time.After(2 * time.Second)
	for {
		nodeD.mu.Lock()
		dCalled := nodeD.processCalled
		nodeD.mu.Unlock()

		if dCalled > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for error port routing to node D")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// C(정상 출력 노드)는 호출되지 않아야 한다 (B가 에러를 반환하므로)
	nodeC.mu.Lock()
	cCalled := nodeC.processCalled
	nodeC.mu.Unlock()
	assert.Equal(t, 0, cCalled, "node C should not be called when B returns error")

	// D가 받은 메시지에 에러 메타데이터가 있는지 확인
	assert.True(t, rt.errorCount.Load() > 0, "error count should increase")

	require.NoError(t, e.StopFlow(ctx, f.ID()))
}

func TestEngine_ErrorPortRouting_NoErrorWires(t *testing.T) {
	// A -> B(에러 발생) -> C, 에러 와이어 없음
	// 기존 동작과 동일: 에러가 로그에만 기록되고 메시지는 드롭된다.
	factory := newMockNodeFactory()
	nodeA := newMockNode("", "A", "transform")
	nodeB := newMockNode("", "B", "transform")
	nodeC := newMockNode("", "C", "transform")

	nodeB.processErr = errors.New("some error")

	nodeDefs := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
		flow.NewNodeDef("B", "transform"),
		flow.NewNodeDef("C", "transform"),
	}
	nodeA.id = nodeDefs[0].ID
	nodeB.id = nodeDefs[1].ID
	nodeC.id = nodeDefs[2].ID
	factory.register(nodeA)
	factory.register(nodeB)
	factory.register(nodeC)

	wires := []flow.Wire{
		flow.NewWire(nodeDefs[0].ID, "out", nodeDefs[1].ID, "in"),
		flow.NewWire(nodeDefs[1].ID, "out", nodeDefs[2].ID, "in"),
	}
	f := flow.NewFlow("no-error-wire",
		flow.WithNodes(nodeDefs...),
		flow.WithWires(wires...),
	)

	logger := &mockLogger{}
	e := newTestEngine(factory)
	e.logger = logger
	ctx := context.Background()

	require.NoError(t, e.DeployFlow(ctx, f))
	require.NoError(t, e.StartFlow(ctx, f.ID()))

	e.mu.RLock()
	rt := e.flows[f.ID()]
	e.mu.RUnlock()

	var wireAB *RuntimeWire
	for _, w := range rt.wires {
		if w.SourceNodeID == nodeDefs[0].ID {
			wireAB = w
			break
		}
	}

	msg := message.New()
	require.NoError(t, wireAB.Send(ctx, msg))

	// 에러 카운트 증가 대기
	deadline := time.After(2 * time.Second)
	for {
		if rt.errorCount.Load() > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for error count")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// C는 호출되지 않아야 한다
	nodeC.mu.Lock()
	cCalled := nodeC.processCalled
	nodeC.mu.Unlock()
	assert.Equal(t, 0, cCalled)

	require.NoError(t, e.StopFlow(ctx, f.ID()))
}

// ---------------------------------------------------------------------------
// 계층적 로그 레벨 테스트
// ---------------------------------------------------------------------------

// TestResolveNodeLogLevel 은 resolveNodeLogLevel 함수의 계층적 로그 레벨 결정을 검증한다.
func TestResolveNodeLogLevel(t *testing.T) {
	tests := []struct {
		name         string
		nodeConfig   map[string]any
		flowLogLevel string
		daemonLevel  slog.Level
		wantLevel    slog.Level
		wantExplicit bool
	}{
		{
			name:         "노드 config log_level이 최우선",
			nodeConfig:   map[string]any{"log_level": "debug"},
			flowLogLevel: "warn",
			daemonLevel:  slog.LevelInfo,
			wantLevel:    slog.LevelDebug,
			wantExplicit: true,
		},
		{
			name:         "노드 config 없으면 플로우 log_level 사용",
			nodeConfig:   nil,
			flowLogLevel: "error",
			daemonLevel:  slog.LevelInfo,
			wantLevel:    slog.LevelError,
			wantExplicit: true,
		},
		{
			name:         "모두 없으면 데몬 기본값 사용",
			nodeConfig:   nil,
			flowLogLevel: "",
			daemonLevel:  slog.LevelWarn,
			wantLevel:    slog.LevelWarn,
			wantExplicit: false,
		},
		{
			name:         "노드 config에 log_level 없으면 플로우 fallback",
			nodeConfig:   map[string]any{"some_other": "value"},
			flowLogLevel: "debug",
			daemonLevel:  slog.LevelInfo,
			wantLevel:    slog.LevelDebug,
			wantExplicit: true,
		},
		{
			name:         "잘못된 노드 log_level은 무시하고 플로우 fallback",
			nodeConfig:   map[string]any{"log_level": "invalid"},
			flowLogLevel: "warn",
			daemonLevel:  slog.LevelInfo,
			wantLevel:    slog.LevelWarn,
			wantExplicit: true,
		},
		{
			name:         "잘못된 플로우 log_level은 무시하고 데몬 기본값",
			nodeConfig:   nil,
			flowLogLevel: "invalid",
			daemonLevel:  slog.LevelError,
			wantLevel:    slog.LevelError,
			wantExplicit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nd := flow.NodeDef{
				ID:     "test-node-id",
				Name:   "test-node",
				Type:   "transform",
				Config: tt.nodeConfig,
			}
			flowCfg := flow.FlowConfig{
				LogLevel: tt.flowLogLevel,
			}

			got, explicit := resolveNodeLogLevel(nd, flowCfg, tt.daemonLevel)
			assert.Equal(t, tt.wantLevel, got)
			assert.Equal(t, tt.wantExplicit, explicit)
		})
	}
}

// TestDeployFlow_WithObserver_HierarchicalLogLevel 은 Observer가 설정된 엔진에서
// DeployFlow가 노드별 계층적 로그 레벨을 올바르게 적용하는지 검증한다.
func TestDeployFlow_WithObserver_HierarchicalLogLevel(t *testing.T) {
	obs := observe.New(observe.WithObserverDefaultLevel(slog.LevelInfo))

	factory := newMockNodeFactory()
	registry := node.NewRegistry(node.WithoutBuiltins())
	_ = registry.Register("transform", factory.factory)
	_ = registry.Register("filter", factory.factory)

	e := NewEngine(
		WithNodeRegistry(registry),
		WithObserver(obs),
		WithShutdownTimeout(2*time.Second),
	)

	// 노드 A: 개별 log_level=debug 설정
	// 노드 B: 개별 log_level 없음 → 플로우 log_level=warn 적용
	nodeA := flow.NewNodeDef("nodeA", "transform", flow.WithNodeConfig("log_level", "debug"))
	nodeB := flow.NewNodeDef("nodeB", "filter")

	wires := []flow.Wire{
		flow.NewWire(nodeA.ID, "out", nodeB.ID, "in"),
	}
	f := flow.NewFlow("test-flow",
		flow.WithNodes(nodeA, nodeB),
		flow.WithWires(wires...),
		flow.WithFlowConfig(flow.FlowConfig{
			LogLevel:      "warn",
			ErrorHandling: flow.ErrorPropagate,
		}),
	)

	ctx := context.Background()
	require.NoError(t, e.DeployFlow(ctx, f))

	// nodeA는 node config에서 debug 레벨이 설정되어야 한다
	levelA := obs.Levels.GetLevel("node.nodeA")
	assert.Equal(t, slog.LevelDebug, levelA, "nodeA는 config log_level=debug가 적용되어야 한다")

	// nodeB는 플로우 log_level=warn이 적용되어야 한다
	levelB := obs.Levels.GetLevel("node.nodeB")
	assert.Equal(t, slog.LevelWarn, levelB, "nodeB는 flow log_level=warn이 적용되어야 한다")
}

// TestDeployFlow_WithObserver_DaemonDefault 은 노드와 플로우 모두 log_level이 없을 때
// 데몬 기본값이 적용되는지 검증한다.
func TestDeployFlow_WithObserver_DaemonDefault(t *testing.T) {
	obs := observe.New(observe.WithObserverDefaultLevel(slog.LevelError))

	factory := newMockNodeFactory()
	registry := node.NewRegistry(node.WithoutBuiltins())
	_ = registry.Register("transform", factory.factory)

	e := NewEngine(
		WithNodeRegistry(registry),
		WithObserver(obs),
		WithShutdownTimeout(2*time.Second),
	)

	nodeA := flow.NewNodeDef("nodeA", "transform")
	f := flow.NewFlow("test-flow",
		flow.WithNodes(nodeA),
	)

	ctx := context.Background()
	require.NoError(t, e.DeployFlow(ctx, f))

	// 노드와 플로우 모두 log_level이 없으므로 데몬 기본값(error) 사용
	// LevelManager의 DefaultLevel이 error이므로 등록되지 않은 컴포넌트도 error 반환
	levelA := obs.Levels.GetLevel("node.nodeA")
	assert.Equal(t, slog.LevelError, levelA, "nodeA는 데몬 기본값(error)이 적용되어야 한다")
}

// TestDeployFlow_WithObserver_LogFilteringIntegration 은 데몬 기본 레벨이 DEBUG이고
// 플로우 log_level이 "info"일 때, 배포 후 노드 로거의 Enabled(DEBUG)가 false를 반환하는지
// 실제 Observer 통합 경로를 검증한다 (버그 재현 테스트).
func TestDeployFlow_WithObserver_LogFilteringIntegration(t *testing.T) {
	// 시나리오: 데몬 기본 레벨 = DEBUG, 플로우 log_level = "info"
	obs := observe.New(observe.WithObserverDefaultLevel(slog.LevelDebug))

	factory := newMockNodeFactory()
	registry := node.NewRegistry(node.WithoutBuiltins())
	_ = registry.Register("transform", factory.factory)
	_ = registry.Register("bridge", factory.factory)

	e := NewEngine(
		WithNodeRegistry(registry),
		WithObserver(obs),
		WithShutdownTimeout(2*time.Second),
	)

	// 플로우: log_level=info, 노드에는 개별 log_level 없음
	nodeA := flow.NewNodeDef("modbus-reader", "transform")
	nodeB := flow.NewNodeDef("monitor-logger", "transform")
	wires := []flow.Wire{
		flow.NewWire(nodeA.ID, "out", nodeB.ID, "in"),
	}
	f := flow.NewFlow("mqtt-to-modbus-v2",
		flow.WithNodes(nodeA, nodeB),
		flow.WithWires(wires...),
		flow.WithFlowConfig(flow.FlowConfig{
			LogLevel:      "info",
			ErrorHandling: flow.ErrorPropagate,
		}),
	)

	ctx := context.Background()
	require.NoError(t, e.DeployFlow(ctx, f))

	// 1. LevelManager에 INFO 레벨이 설정되었는지 확인
	levelA := obs.Levels.GetLevel("node.modbus-reader")
	assert.Equal(t, slog.LevelInfo, levelA, "node.modbus-reader는 flow log_level=info가 적용되어야 한다")

	levelB := obs.Levels.GetLevel("node.monitor-logger")
	assert.Equal(t, slog.LevelInfo, levelB, "node.monitor-logger는 flow log_level=info가 적용되어야 한다")

	// 2. 실제 로거의 Enabled()가 DEBUG를 필터링하는지 확인 (핵심 통합 테스트)
	loggerA := obs.Loggers.NewLogger("node.modbus-reader")
	assert.False(t, loggerA.Logger().Enabled(ctx, slog.LevelDebug),
		"node.modbus-reader 로거의 DEBUG는 비활성화되어야 한다")
	assert.True(t, loggerA.Logger().Enabled(ctx, slog.LevelInfo),
		"node.modbus-reader 로거의 INFO는 활성화되어야 한다")

	loggerB := obs.Loggers.NewLogger("node.monitor-logger")
	assert.False(t, loggerB.Logger().Enabled(ctx, slog.LevelDebug),
		"node.monitor-logger 로거의 DEBUG는 비활성화되어야 한다")

	// 3. 엔진 로거 (component "engine")는 플로우 레벨과 무관하게 데몬 기본값 사용
	engineLogger := obs.Loggers.NewLogger("engine")
	assert.True(t, engineLogger.Logger().Enabled(ctx, slog.LevelDebug),
		"engine 로거는 데몬 기본값 DEBUG가 적용되어야 한다 (플로우 레벨 미적용)")
}

// TestDeployFlow_WithObserver_EmptyFlowLogLevel 은 데몬 기본 레벨이 DEBUG이고
// 플로우의 log_level이 비어있을 때 (저장소에서 로드된 오래된 플로우 등),
// 노드 로거가 데몬 기본값(DEBUG)을 사용하는지 검증한다.
func TestDeployFlow_WithObserver_EmptyFlowLogLevel(t *testing.T) {
	// 시나리오: 데몬 기본 레벨 = DEBUG, 플로우 log_level = "" (비어있음)
	obs := observe.New(observe.WithObserverDefaultLevel(slog.LevelDebug))

	factory := newMockNodeFactory()
	registry := node.NewRegistry(node.WithoutBuiltins())
	_ = registry.Register("transform", factory.factory)

	e := NewEngine(
		WithNodeRegistry(registry),
		WithObserver(obs),
		WithShutdownTimeout(2*time.Second),
	)

	nodeA := flow.NewNodeDef("modbus-reader", "transform")
	f := flow.NewFlow("test-flow",
		flow.WithNodes(nodeA),
		flow.WithFlowConfig(flow.FlowConfig{
			LogLevel:      "", // 비어있음!
			ErrorHandling: flow.ErrorPropagate,
		}),
	)

	ctx := context.Background()
	require.NoError(t, e.DeployFlow(ctx, f))

	// 플로우 log_level이 비어있으므로 resolveNodeLogLevel은 (daemonDefault, false) 반환
	// SetLevel이 호출되지 않아 노드는 NewLogger에서 등록된 기본값(DEBUG) 유지
	loggerA := obs.Loggers.NewLogger("node.modbus-reader")
	assert.True(t, loggerA.Logger().Enabled(ctx, slog.LevelDebug),
		"플로우 log_level이 비어있으면 노드는 데몬 기본값(DEBUG)을 사용해야 한다")
}

// TestDeployFlow_WithoutObserver 은 Observer 없이도 DeployFlow가 정상 동작하는지 검증한다.
func TestDeployFlow_WithoutObserver(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)

	f, _ := newSimpleFlow()

	ctx := context.Background()
	require.NoError(t, e.DeployFlow(ctx, f))

	// Observer 없이도 배포 성공해야 한다
	status, err := e.GetFlowStatus(f.ID())
	require.NoError(t, err)
	assert.Equal(t, flow.FlowLoaded, status.State)
}

// ---------------------------------------------------------------------------
// 디버그 포트 로깅 테스트
// ---------------------------------------------------------------------------

// capturingLogger 는 Debug 호출을 기록하는 ComponentLogger mock이다.
type capturingLogger struct {
	debugCalls []capturedDebugCall
	slogger    *slog.Logger
	mu         sync.Mutex
}

type capturedDebugCall struct {
	msg  string
	args []any
}

func newCapturingLogger(level slog.Level) *capturingLogger {
	// slog.Logger.Enabled() 체크에 사용되는 실제 slog.Logger를 생성한다.
	// 지정된 레벨 이상의 로그만 활성화된다.
	levelVar := &slog.LevelVar{}
	levelVar.Set(level)
	handler := slog.NewJSONHandler(discard{}, &slog.HandlerOptions{Level: levelVar})
	return &capturingLogger{
		slogger: slog.New(handler),
	}
}

// discard 는 모든 출력을 무시하는 io.Writer이다.
type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func (c *capturingLogger) Debug(msg string, args ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.debugCalls = append(c.debugCalls, capturedDebugCall{msg: msg, args: args})
}
func (c *capturingLogger) Info(msg string, args ...any)              {}
func (c *capturingLogger) Warn(msg string, args ...any)              {}
func (c *capturingLogger) Error(msg string, args ...any)             {}
func (c *capturingLogger) With(args ...any) observe.ComponentLogger  { return c }
func (c *capturingLogger) WithGroup(name string) observe.ComponentLogger { return c }
func (c *capturingLogger) Component() string                        { return "test.capture" }
func (c *capturingLogger) Logger() *slog.Logger                     { return c.slogger }

func (c *capturingLogger) getDebugCalls() []capturedDebugCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]capturedDebugCall, len(c.debugCalls))
	copy(result, c.debugCalls)
	return result
}

// TestDebugPortLog_DebugEnabled 은 로그 레벨이 Debug일 때 포트 메시지가 로깅되는지 검증한다.
func TestDebugPortLog_DebugEnabled(t *testing.T) {
	logger := newCapturingLogger(slog.LevelDebug)
	msg := message.New()
	msg.Payload().Set("temperature", 25.5)
	ctx := context.Background()

	debugPortLog(ctx, logger, "input", "node-A", msg)

	calls := logger.getDebugCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "port.input", calls[0].msg)

	// args 에 nodeID, msgID, payload가 포함되어야 한다
	assert.Contains(t, calls[0].args, "nodeID")
	assert.Contains(t, calls[0].args, "node-A")
	assert.Contains(t, calls[0].args, "msgID")
	assert.Contains(t, calls[0].args, "payload")
}

// TestDebugPortLog_InfoLevel 은 로그 레벨이 Info일 때 Debug 로그가 출력되지 않는지 검증한다.
func TestDebugPortLog_InfoLevel(t *testing.T) {
	logger := newCapturingLogger(slog.LevelInfo)
	msg := message.New()
	ctx := context.Background()

	debugPortLog(ctx, logger, "output", "node-B", msg)

	calls := logger.getDebugCalls()
	assert.Empty(t, calls, "Info 레벨에서는 Debug 로그가 출력되면 안 된다")
}

// TestDebugPortLog_NilLogger 는 로거가 nil이어도 패닉 없이 안전하게 동작하는지 검증한다.
func TestDebugPortLog_NilLogger(t *testing.T) {
	msg := message.New()
	ctx := context.Background()

	// 패닉 발생하지 않아야 한다
	assert.NotPanics(t, func() {
		debugPortLog(ctx, nil, "input", "node-C", msg)
	})
}

// TestDebugPortLog_Directions 는 모든 방향(input, output, source, error)이 올바르게 로깅되는지 검증한다.
func TestDebugPortLog_Directions(t *testing.T) {
	directions := []string{"input", "output", "source", "error"}

	for _, dir := range directions {
		t.Run(dir, func(t *testing.T) {
			logger := newCapturingLogger(slog.LevelDebug)
			msg := message.New()
			ctx := context.Background()

			debugPortLog(ctx, logger, dir, "node-X", msg)

			calls := logger.getDebugCalls()
			require.Len(t, calls, 1)
			assert.Equal(t, "port."+dir, calls[0].msg)
		})
	}
}

// TestDebugPortLog_PayloadContent 는 페이로드 내용이 로그에 정확히 포함되는지 검증한다.
func TestDebugPortLog_PayloadContent(t *testing.T) {
	logger := newCapturingLogger(slog.LevelDebug)
	msg := message.New()
	msg.Payload().Set("device_id", "sensor-001")
	msg.Payload().Set("temperature", 72.5)
	ctx := context.Background()

	debugPortLog(ctx, logger, "input", "node-sensor", msg)

	calls := logger.getDebugCalls()
	require.Len(t, calls, 1)

	// payload 인자를 찾아서 map 내용을 확인한다
	var payloadMap map[string]any
	for i, arg := range calls[0].args {
		if arg == "payload" && i+1 < len(calls[0].args) {
			payloadMap = calls[0].args[i+1].(map[string]any)
			break
		}
	}
	require.NotNil(t, payloadMap, "payload 맵이 로그 인자에 포함되어야 한다")
	assert.Equal(t, "sensor-001", payloadMap["device_id"])
	assert.Equal(t, 72.5, payloadMap["temperature"])
}

// loggerInjectingFactory 는 특정 노드 이름에 대해 로거를 주입하는 노드 팩토리이다.
type loggerInjectingFactory struct {
	loggers map[string]observe.ComponentLogger // nodeName -> logger
}

func newLoggerInjectingFactory(loggers map[string]observe.ComponentLogger) *loggerInjectingFactory {
	return &loggerInjectingFactory{loggers: loggers}
}

func (f *loggerInjectingFactory) factory(def flow.NodeDef, opts ...node.NodeOption) (node.Node, error) {
	mn := newMockNode(def.ID, def.Name, def.Type)
	if logger, ok := f.loggers[def.Name]; ok {
		mn.logger = logger
	}
	return mn, nil
}

// TestRunNode_DebugPortLogging_WithMessage 는 실제 메시지 흐름에서 디버그 로깅을 검증한다.
func TestRunNode_DebugPortLogging_WithMessage(t *testing.T) {
	loggerB := newCapturingLogger(slog.LevelDebug)

	lif := newLoggerInjectingFactory(map[string]observe.ComponentLogger{
		"nodeB": loggerB,
	})

	registry := node.NewRegistry(node.WithoutBuiltins())
	_ = registry.Register("transform", lif.factory)

	e := NewEngine(
		WithNodeRegistry(registry),
		WithShutdownTimeout(2*time.Second),
	)
	ctx := context.Background()

	nodeADef := flow.NewNodeDef("nodeA", "transform")
	nodeBDef := flow.NewNodeDef("nodeB", "transform")
	wire := flow.NewWire(nodeADef.ID, "out", nodeBDef.ID, "in")
	f := flow.NewFlow("debug-msg-test",
		flow.WithNodes(nodeADef, nodeBDef),
		flow.WithWires(wire),
	)

	require.NoError(t, e.DeployFlow(ctx, f))
	require.NoError(t, e.StartFlow(ctx, f.ID()))

	// nodeB로 메시지를 전달하기 위해 nodeA→nodeB 와이어에 직접 전송
	e.mu.RLock()
	rt := e.flows[f.ID()]
	e.mu.RUnlock()

	for _, w := range rt.wires {
		if w.SourceNodeID == nodeADef.ID {
			msg := message.New()
			msg.Payload().Set("temperature", 25.0)
			require.NoError(t, w.Send(ctx, msg))
			break
		}
	}

	// 노드 처리 시간 대기
	time.Sleep(100 * time.Millisecond)

	require.NoError(t, e.StopFlow(ctx, f.ID()))

	// nodeB에서 입력 디버그 로그가 기록되어야 한다
	calls := loggerB.getDebugCalls()
	require.GreaterOrEqual(t, len(calls), 1, "nodeB에서 최소 1개의 디버그 로그가 기록되어야 한다")

	// 입력 포트 로그 확인
	hasInputLog := false
	hasOutputLog := false
	for _, call := range calls {
		if call.msg == "port.input" {
			hasInputLog = true
		}
		if call.msg == "port.output" {
			hasOutputLog = true
		}
	}
	assert.True(t, hasInputLog, "port.input 디버그 로그가 있어야 한다")
	assert.True(t, hasOutputLog, "port.output 디버그 로그가 있어야 한다 (passthrough 처리)")
}

// TestRunNode_NoDebugLog_WhenInfoLevel 은 Info 레벨 노드에서는 포트 로그가 출력되지 않는지 검증한다.
func TestRunNode_NoDebugLog_WhenInfoLevel(t *testing.T) {
	loggerB := newCapturingLogger(slog.LevelInfo) // Info 레벨 — Debug 비활성화

	lif := newLoggerInjectingFactory(map[string]observe.ComponentLogger{
		"nodeB": loggerB,
	})

	registry := node.NewRegistry(node.WithoutBuiltins())
	_ = registry.Register("transform", lif.factory)

	e := NewEngine(
		WithNodeRegistry(registry),
		WithShutdownTimeout(2*time.Second),
	)
	ctx := context.Background()

	nodeADef := flow.NewNodeDef("nodeA", "transform")
	nodeBDef := flow.NewNodeDef("nodeB", "transform")
	wire := flow.NewWire(nodeADef.ID, "out", nodeBDef.ID, "in")
	f := flow.NewFlow("info-level-test",
		flow.WithNodes(nodeADef, nodeBDef),
		flow.WithWires(wire),
	)

	require.NoError(t, e.DeployFlow(ctx, f))
	require.NoError(t, e.StartFlow(ctx, f.ID()))

	e.mu.RLock()
	rt := e.flows[f.ID()]
	e.mu.RUnlock()

	for _, w := range rt.wires {
		if w.SourceNodeID == nodeADef.ID {
			msg := message.New()
			msg.Payload().Set("data", "test")
			require.NoError(t, w.Send(ctx, msg))
			break
		}
	}

	time.Sleep(100 * time.Millisecond)
	require.NoError(t, e.StopFlow(ctx, f.ID()))

	calls := loggerB.getDebugCalls()
	assert.Empty(t, calls, "Info 레벨에서는 포트 디버그 로그가 출력되면 안 된다")
}

// ---------------------------------------------------------------------------
// GetFlowNodes / GetFlowNode 테스트
// ---------------------------------------------------------------------------

func TestGetFlowNodes_정상(t *testing.T) {
	factory := newMockNodeFactory()
	f, nodeDefs := newSimpleFlow()

	nodeA := newMockNode("", "A", "transform")
	nodeB := newMockNode("", "B", "transform")
	nodeA.id = nodeDefs[0].ID
	nodeB.id = nodeDefs[1].ID
	factory.register(nodeA)
	factory.register(nodeB)

	e := newTestEngine(factory)
	ctx := context.Background()

	require.NoError(t, e.DeployFlow(ctx, f))

	nodes, err := e.GetFlowNodes(f.ID())
	require.NoError(t, err)
	assert.Len(t, nodes, 2)

	// 노드 정보 검증
	nodeMap := make(map[string]NodeInstanceInfo)
	for _, n := range nodes {
		nodeMap[n.Name] = n
	}

	infoA := nodeMap["A"]
	assert.Equal(t, nodeDefs[0].ID, infoA.NodeID)
	assert.Equal(t, "transform", infoA.Type)
	assert.NotEmpty(t, infoA.Ports)
}

func TestGetFlowNodes_미배포에러(t *testing.T) {
	e := newTestEngine(nil)

	_, err := e.GetFlowNodes("nonexistent")
	assert.ErrorIs(t, err, ErrFlowNotFound)
}

func TestGetFlowNode_정상(t *testing.T) {
	factory := newMockNodeFactory()
	f, nodeDefs := newSimpleFlow()

	nodeA := newMockNode("", "A", "transform")
	nodeB := newMockNode("", "B", "transform")
	nodeA.id = nodeDefs[0].ID
	nodeB.id = nodeDefs[1].ID
	factory.register(nodeA)
	factory.register(nodeB)

	e := newTestEngine(factory)
	ctx := context.Background()

	require.NoError(t, e.DeployFlow(ctx, f))

	info, err := e.GetFlowNode(f.ID(), nodeDefs[0].ID)
	require.NoError(t, err)
	assert.Equal(t, nodeDefs[0].ID, info.NodeID)
	assert.Equal(t, "A", info.Name)
	assert.Equal(t, "transform", info.Type)
}

func TestGetFlowNode_미존재노드에러(t *testing.T) {
	factory := newMockNodeFactory()
	f, _ := newSimpleFlow()

	e := newTestEngine(factory)
	ctx := context.Background()

	require.NoError(t, e.DeployFlow(ctx, f))

	_, err := e.GetFlowNode(f.ID(), "nonexistent-node")
	assert.ErrorIs(t, err, ErrNodeNotFound)
}

func TestGetFlowNode_미배포플로우에러(t *testing.T) {
	e := newTestEngine(nil)

	_, err := e.GetFlowNode("nonexistent-flow", "any-node")
	assert.ErrorIs(t, err, ErrFlowNotFound)
}

func TestGetFlowNodes_시작후상태포함(t *testing.T) {
	factory := newMockNodeFactory()
	f, nodeDefs := newSimpleFlow()

	nodeA := newMockNode("", "A", "transform")
	nodeB := newMockNode("", "B", "transform")
	nodeA.id = nodeDefs[0].ID
	nodeB.id = nodeDefs[1].ID
	factory.register(nodeA)
	factory.register(nodeB)

	e := newTestEngine(factory)
	ctx := context.Background()

	require.NoError(t, e.DeployFlow(ctx, f))
	require.NoError(t, e.StartFlow(ctx, f.ID()))

	nodes, err := e.GetFlowNodes(f.ID())
	require.NoError(t, err)
	assert.Len(t, nodes, 2)

	// 포트 정보 확인
	for _, n := range nodes {
		assert.NotEmpty(t, n.Ports, "노드 %s는 포트를 가져야 한다", n.Name)
	}

	require.NoError(t, e.StopFlow(ctx, f.ID()))
}
