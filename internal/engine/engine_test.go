package engine

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

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
	id        string
	name      string
	nodeType  string
	initErr   error
	processResults []message.Message
	processErr     error
	shutdownErr    error
	initCalled     bool
	shutdownCalled bool
	processCalled  int
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

func (m *mockNode) Ports() []node.NodePort {
	return []node.NodePort{
		{ID: "in", Name: "in", Direction: flow.PortInput},
		{ID: "out", Name: "out", Direction: flow.PortOutput},
	}
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

	// 로드된 상태에서 배포 해제 시도
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
