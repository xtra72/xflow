package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// gateProcNode 는 "미선언 출력 포트 emit 게이트" 검증용 가짜 처리 노드이다.
// declareOut 으로 "out" 출력 포트 선언 여부를 제어하며, Process 는 입력 메시지를
// 그대로 1개 반환하여 항상 "out" 으로 방출을 시도한다(mqtt-publisher/inventory 패턴 모사).
type gateProcNode struct {
	id         string
	name       string
	nodeType   string
	declareOut bool // true 면 Ports() 에 "out" 출력 포트를 포함한다.

	mu            sync.Mutex
	processCalled int
}

func newGateProcNode(id, name string, declareOut bool) *gateProcNode {
	return &gateProcNode{id: id, name: name, nodeType: "gateproc", declareOut: declareOut}
}

func (g *gateProcNode) ID() string                            { return g.id }
func (g *gateProcNode) Name() string                          { return g.name }
func (g *gateProcNode) Type() string                          { return g.nodeType }
func (g *gateProcNode) Init(ctx context.Context) error        { return nil }
func (g *gateProcNode) Shutdown(ctx context.Context) error    { return nil }
func (g *gateProcNode) Configure(config map[string]any) error { return nil }

func (g *gateProcNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	g.mu.Lock()
	g.processCalled++
	g.mu.Unlock()
	// 포트 선언과 무관하게 항상 "out" 으로 메시지 1개를 방출한다.
	return []message.Message{msg}, nil
}

func (g *gateProcNode) processedCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.processCalled
}

// Ports 는 입력 포트 "in" 을 항상 선언하고, declareOut 일 때만 출력 포트 "out" 을 선언한다.
func (g *gateProcNode) Ports() []node.NodePort {
	ports := []node.NodePort{
		{ID: "in", Name: "in", Direction: flow.PortInput},
	}
	if g.declareOut {
		ports = append(ports, node.NodePort{ID: "out", Name: "out", Direction: flow.PortOutput})
	}
	return ports
}

// deployGateFlow 는 source(mockNode) -> gateNode 로 연결된 플로우를 배포/시작하고,
// source 의 "out" 와이어와 플로우 ID 를 반환한다. gateNode 의 "out" 포트에는 와이어를
// 연결하지 않는다. handler 가 nil 이 아니면 엔진 로거로 주입해 경고 로그를 관측한다.
func deployGateFlow(t *testing.T, gateNode *gateProcNode, obs *fakeOutputObserver, handler *countingHandler) (*Engine, *RuntimeWire, string, string, func()) {
	t.Helper()

	factory := newMockNodeFactory()
	source := newMockNode("", "SRC", "transform")

	sourceDef := flow.NewNodeDef("SRC", "transform")
	gateDef := flow.NewNodeDef(gateNode.name, gateNode.nodeType)
	source.id = sourceDef.ID
	gateNode.id = gateDef.ID
	factory.register(source)

	// gateNode 팩토리: 등록된 gateNode 인스턴스를 반환한다.
	gateFactory := func(def flow.NodeDef, opts ...node.NodeOption) (node.Node, error) {
		if def.ID == gateDef.ID {
			return gateNode, nil
		}
		return factory.factory(def, opts...)
	}

	nodeDefs := []flow.NodeDef{sourceDef, gateDef}
	wires := []flow.Wire{
		flow.NewWire(sourceDef.ID, "out", gateDef.ID, "in"),
		// gateNode 의 "out" 포트에는 의도적으로 와이어를 연결하지 않는다.
	}
	f := flow.NewFlow("gate-flow",
		flow.WithNodes(nodeDefs...),
		flow.WithWires(wires...),
	)

	registry := node.NewRegistry(node.WithoutBuiltins())
	require.NoError(t, registry.Register("transform", factory.factory))
	require.NoError(t, registry.Register("gateproc", gateFactory))

	opts := []EngineOption{
		WithNodeRegistry(registry),
		WithShutdownTimeout(2 * time.Second),
	}
	if obs != nil {
		opts = append(opts, WithOutputObserver(obs))
	}
	if handler != nil {
		opts = append(opts, WithLogger(newTestLogger(handler)))
	}
	e := NewEngine(opts...)

	ctx := context.Background()
	require.NoError(t, e.DeployFlow(ctx, f))
	require.NoError(t, e.StartFlow(ctx, f.ID()))

	e.mu.RLock()
	rt := e.flows[f.ID()]
	e.mu.RUnlock()

	var srcWire *RuntimeWire
	for _, w := range rt.wires {
		if w.SourceNodeID == sourceDef.ID && w.SourcePort == "out" {
			srcWire = w
			break
		}
	}
	require.NotNil(t, srcWire)

	cleanup := func() { _ = e.StopFlow(ctx, f.ID()) }
	return e, srcWire, gateDef.ID, f.ID(), cleanup
}

// waitProcessed 는 gateNode 가 최소 1회 Process 될 때까지 대기한다.
func waitProcessed(t *testing.T, gateNode *gateProcNode) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for gateNode.processedCount() == 0 {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for gate node to process")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestUndeclaredOutputPortGate_DiscardsWhenUndeclaredAndUnwired 는 (A) 케이스이다.
// "out" 출력 포트를 선언하지 않은 노드가 "out" 으로 메시지를 방출하고 "out" 와이어가
// 없을 때, 출력 옵저버(tap)가 호출되지 않고 미연결 경고를 타지 않으며 "out" 포트
// 카운터도 증가하지 않음(존재하지 않음)을 검증한다.
//
// 변경 전(게이트 미적용)에는 notifyOutputObserver 가 무조건 호출되므로 이 테스트는
// 실패한다(옵저버가 호출됨). 게이트 적용 후 통과한다.
func TestUndeclaredOutputPortGate_DiscardsWhenUndeclaredAndUnwired(t *testing.T) {
	gateNode := newGateProcNode("", "UNDECLARED", false) // "out" 미선언
	obs := &fakeOutputObserver{}
	handler := &countingHandler{}
	e, srcWire, gateID, flowID, cleanup := deployGateFlow(t, gateNode, obs, handler)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, srcWire.Send(ctx, message.New()))

	waitProcessed(t, gateNode)
	// 처리 후 라우팅이 끝나도록 짧게 대기한다.
	time.Sleep(100 * time.Millisecond)

	// 1) 출력 옵저버는 gateNode 에 대해 호출되지 않아야 한다(tap 게이트).
	for _, c := range obs.snapshot() {
		assert.NotEqual(t, gateID, c.nodeID,
			"미선언 출력 포트는 tap 통지에서 게이트되어야 한다")
	}

	// 2) 미연결 출력 포트 경고가 발생하지 않아야 한다(경고 게이트).
	//    gateNode 의 미선언 "out" 은 게이트로 폐기되어 "연결된 와이어 없음" 경고를
	//    타지 않는다 → 해당 경고 0건이어야 한다.
	assert.Equal(t, 0, handler.warnCountContaining("출력 포트에 연결된 와이어 없음"),
		"미선언+미연결 출력 포트는 미연결 경고를 발생시키지 않아야 한다")

	// 3) gateNode 의 "out" 포트 카운터는 증가하지 않아야 한다(emit 카운트 게이트).
	//    포트가 선언되지 않았고 와이어도 없으면 카운터 자체가 생성되지 않거나,
	//    생성되더라도 emit(messages)이 0 이어야 한다.
	e.mu.RLock()
	rt := e.flows[flowID]
	e.mu.RUnlock()
	require.NotNil(t, rt)
	nc := rt.nodeCounters[gateID]
	require.NotNil(t, nc)
	if pc := nc.portCounters["out"]; pc != nil {
		assert.Equal(t, int64(0), pc.messages.Load(),
			"미선언+미연결 'out' 포트는 emit 카운트가 증가하지 않아야 한다")
	}
}

// TestDeclaredOutputPortGate_PreservesTapWhenWireMissing 는 (B) 대조군이다.
// "out" 출력 포트를 선언한 동일 노드가 와이어 없이 방출하면 기존대로 출력 옵저버가
// 호출되어 tap 동작이 보존됨을 검증한다(하위호환).
func TestDeclaredOutputPortGate_PreservesTapWhenWireMissing(t *testing.T) {
	gateNode := newGateProcNode("", "DECLARED", true) // "out" 선언
	obs := &fakeOutputObserver{}
	_, srcWire, gateID, _, cleanup := deployGateFlow(t, gateNode, obs, nil)
	defer cleanup()

	ctx := context.Background()
	require.NoError(t, srcWire.Send(ctx, message.New()))

	waitProcessed(t, gateNode)

	// gateNode 의 "out" tap 이 관측될 때까지 대기한다.
	deadline := time.After(2 * time.Second)
	var sawGate bool
	for !sawGate {
		for _, c := range obs.snapshot() {
			if c.nodeID == gateID && c.port == "out" {
				sawGate = true
				break
			}
		}
		if sawGate {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout: 선언된 출력 포트의 tap 통지가 관측되지 않음")
		case <-time.After(5 * time.Millisecond):
		}
	}
	assert.True(t, sawGate, "선언된 출력 포트는 와이어가 없어도 tap 이 보존되어야 한다")
}
