package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
)

// mockReinitNode 는 node.Node + node.AgentReinitializer 를 구현하는 테스트 전용 노드이다.
// Engine.ReinitNodesForAgent 가 올바른 노드만 Reinit 호출하는지 검증하는 데 사용된다.
type mockReinitNode struct {
	*mockNode
	agentRef    flow.AgentRef
	reinitCalls atomic.Int32
	reinitErr   error
	mu          sync.Mutex
}

func newMockReinitNode(id, name string, ref flow.AgentRef) *mockReinitNode {
	return &mockReinitNode{
		mockNode: newMockNode(id, name, "transform"),
		agentRef: ref,
	}
}

// AgentRef 는 AgentReinitializer 인터페이스 구현이다.
func (m *mockReinitNode) AgentRef() flow.AgentRef {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.agentRef
}

// Reinit 은 AgentReinitializer 인터페이스 구현이다.
// 호출 횟수를 기록하고 사전 설정된 에러를 반환한다.
func (m *mockReinitNode) Reinit(_ context.Context) error {
	m.reinitCalls.Add(1)
	return m.reinitErr
}

// 인터페이스 컴파일 체크
var _ node.AgentReinitializer = (*mockReinitNode)(nil)

// reinitNodeFactory 는 mockReinitNode 들을 이름으로 매핑하여 반환하는 팩토리이다.
// flow.NewNodeDef 는 ID 를 UUID 로 자동 생성하므로, 테스트에서는 Name 으로 조회한다.
type reinitNodeFactory struct {
	nodes map[string]*mockReinitNode
}

func newReinitNodeFactory() *reinitNodeFactory {
	return &reinitNodeFactory{nodes: make(map[string]*mockReinitNode)}
}

func (f *reinitNodeFactory) register(n *mockReinitNode) {
	f.nodes[n.Name()] = n
}

func (f *reinitNodeFactory) factory(def flow.NodeDef, _ ...node.NodeOption) (node.Node, error) {
	if n, ok := f.nodes[def.Name]; ok {
		// def.ID 가 UUID 로 변경되었으므로 등록된 모의 노드의 id 를 동기화한다.
		n.mockNode.id = def.ID
		return n, nil
	}
	// 등록되지 않은 노드는 일반 mockNode 로 폴백 (AgentReinitializer 미구현)
	return newMockNode(def.ID, def.Name, def.Type), nil
}

// newReinitTestEngine 는 reinitNodeFactory 기반의 테스트용 Engine 을 생성한다.
func newReinitTestEngine(factory *reinitNodeFactory) *Engine {
	registry := node.NewRegistry(node.WithoutBuiltins())
	_ = registry.Register("agent-backed", factory.factory)
	_ = registry.Register("transform", factory.factory)

	return NewEngine(
		WithNodeRegistry(registry),
	)
}

// TestEngine_ReinitNodesForAgent_TouchesAllReinitializers 는 ReinitNodesForAgent 가
// AgentReinitializer 인터페이스를 구현한 모든 노드를 호출하는지 확인한다.
// 수정 전에는 bridgeReinitializer (BridgeNode 전용) 만 호출되었기에 비-Bridge 노드는
// Reinit 호출이 누락되었다. 수정 후 인터페이스 이름을 AgentReinitializer 로 일반화하고
// 모든 에이전트 백엔드 노드가 이를 구현하므로 본 테스트가 통과해야 한다.
func TestEngine_ReinitNodesForAgent_TouchesAllReinitializers(t *testing.T) {
	factory := newReinitNodeFactory()
	agentRef := flow.AgentRef{AgentID: "agent-A", AgentName: "agent-A"}

	nodeA := newMockReinitNode("node-A", "node-A", agentRef)
	nodeB := newMockReinitNode("node-B", "node-B", agentRef)
	factory.register(nodeA)
	factory.register(nodeB)

	e := newReinitTestEngine(factory)

	nodes := []flow.NodeDef{
		flow.NewNodeDef("node-A", "agent-backed"),
		flow.NewNodeDef("node-B", "agent-backed"),
	}
	wires := []flow.Wire{
		flow.NewWire(nodes[0].ID, "out", nodes[1].ID, "in"),
	}
	f := flow.NewFlow("reinit-flow-all",
		flow.WithNodes(nodes...),
		flow.WithWires(wires...),
	)

	require.NoError(t, e.DeployFlow(context.Background(), f))
	require.NoError(t, e.StartFlow(context.Background(), f.ID()))
	defer func() { _ = e.StopFlow(context.Background(), f.ID()) }()

	e.ReinitNodesForAgent(agentRef.AgentID, agentRef.AgentName)

	assert.EqualValues(t, 1, nodeA.reinitCalls.Load(),
		"node-A 는 agent-A 를 참조하므로 Reinit 이 한 번 호출되어야 한다")
	assert.EqualValues(t, 1, nodeB.reinitCalls.Load(),
		"node-B 도 agent-A 를 참조하므로 Reinit 이 한 번 호출되어야 한다")
}

// TestEngine_ReinitNodesForAgent_SkipsNonMatching 는 ReinitNodesForAgent 가
// 지정된 에이전트를 참조하지 않는 노드는 Reinit 하지 않는지 확인한다.
func TestEngine_ReinitNodesForAgent_SkipsNonMatching(t *testing.T) {
	factory := newReinitNodeFactory()
	refA := flow.AgentRef{AgentID: "agent-A", AgentName: "agent-A"}
	refB := flow.AgentRef{AgentID: "agent-B", AgentName: "agent-B"}

	nodeA := newMockReinitNode("node-A", "node-A", refA)
	nodeB := newMockReinitNode("node-B", "node-B", refB)
	factory.register(nodeA)
	factory.register(nodeB)

	e := newReinitTestEngine(factory)

	nodes := []flow.NodeDef{
		flow.NewNodeDef("node-A", "agent-backed"),
		flow.NewNodeDef("node-B", "agent-backed"),
	}
	wires := []flow.Wire{
		flow.NewWire(nodes[0].ID, "out", nodes[1].ID, "in"),
	}
	f := flow.NewFlow("reinit-flow-skip",
		flow.WithNodes(nodes...),
		flow.WithWires(wires...),
	)

	require.NoError(t, e.DeployFlow(context.Background(), f))
	require.NoError(t, e.StartFlow(context.Background(), f.ID()))
	defer func() { _ = e.StopFlow(context.Background(), f.ID()) }()

	// agent-A 만 reinit
	e.ReinitNodesForAgent(refA.AgentID, refA.AgentName)

	assert.EqualValues(t, 1, nodeA.reinitCalls.Load(), "agent-A 를 참조하는 node-A 만 Reinit 되어야 한다")
	assert.EqualValues(t, 0, nodeB.reinitCalls.Load(), "agent-B 를 참조하는 node-B 는 Reinit 되지 않아야 한다")
}

// TestEngine_ReinitNodesForAgent_SkipsStoppedFlows 는 실행 중이 아닌 (rt.cancel == nil)
// 플로우의 노드는 Reinit 되지 않는지 확인한다 (기존 동작 보존).
func TestEngine_ReinitNodesForAgent_SkipsStoppedFlows(t *testing.T) {
	factory := newReinitNodeFactory()
	ref := flow.AgentRef{AgentID: "agent-A", AgentName: "agent-A"}

	n := newMockReinitNode("node-A", "node-A", ref)
	factory.register(n)

	e := newReinitTestEngine(factory)

	nodes := []flow.NodeDef{
		flow.NewNodeDef("node-A", "agent-backed"),
	}
	f := flow.NewFlow("reinit-flow-stopped",
		flow.WithNodes(nodes...),
	)

	// Deploy 만 하고 Start 하지 않은 상태 → rt.cancel == nil
	require.NoError(t, e.DeployFlow(context.Background(), f))

	e.ReinitNodesForAgent(ref.AgentID, ref.AgentName)

	assert.EqualValues(t, 0, n.reinitCalls.Load(),
		"실행되지 않은 플로우의 노드는 Reinit 되지 않아야 한다")
}
