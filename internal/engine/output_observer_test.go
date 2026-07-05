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

// fakeOutputObserver 는 OutputObserver 호출을 기록하는 테스트용 구현이다.
type fakeOutputObserver struct {
	mu    sync.Mutex
	calls []observerCall
}

type observerCall struct {
	flowID string
	nodeID string
	port   string
	msgID  string
}

func (f *fakeOutputObserver) OnNodeOutput(flowID, nodeID, port string, msg message.Message) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, observerCall{
		flowID: flowID,
		nodeID: nodeID,
		port:   port,
		msgID:  msg.ID(),
	})
}

func (f *fakeOutputObserver) snapshot() []observerCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]observerCall, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *fakeOutputObserver) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// TestEngine_OutputObserver_CalledOnEmit 는 OutputObserver가 주입되면
// 노드가 메시지를 출력할 때마다 OnNodeOutput이 정확한 flowID/nodeID/port로
// 호출되는지 검증한다.
func TestEngine_OutputObserver_CalledOnEmit(t *testing.T) {
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
	f := flow.NewFlow("observer-flow",
		flow.WithNodes(nodeDefs...),
		flow.WithWires(wires...),
	)

	obs := &fakeOutputObserver{}
	registry := node.NewRegistry(node.WithoutBuiltins())
	_ = registry.Register("transform", factory.factory)
	e := NewEngine(
		WithNodeRegistry(registry),
		WithShutdownTimeout(2*time.Second),
		WithOutputObserver(obs),
	)
	ctx := context.Background()

	require.NoError(t, e.DeployFlow(ctx, f))
	require.NoError(t, e.StartFlow(ctx, f.ID()))

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

	// B가 메시지를 처리하여 "out" 포트로 출력할 때까지 대기.
	deadline := time.After(2 * time.Second)
	for {
		nodeB.mu.Lock()
		bCalled := nodeB.processCalled
		nodeB.mu.Unlock()
		if bCalled > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for node B to process")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// 옵저버 호출이 기록될 때까지 짧게 대기.
	deadline = time.After(2 * time.Second)
	for obs.count() == 0 {
		select {
		case <-deadline:
			t.Fatal("timeout waiting for output observer call")
		case <-time.After(10 * time.Millisecond):
		}
	}

	require.NoError(t, e.StopFlow(ctx, f.ID()))

	calls := obs.snapshot()
	require.NotEmpty(t, calls, "observer should be called at least once")
	for _, c := range calls {
		assert.Equal(t, f.ID(), c.flowID, "flowID should match deployed flow")
		assert.Equal(t, "out", c.port, "default output port should be 'out'")
		assert.Contains(t, []string{nodeDefs[0].ID, nodeDefs[1].ID}, c.nodeID,
			"nodeID should be one of the flow nodes")
	}

	// B 노드(default passthrough)의 "out" 출력이 관측되어야 한다.
	var sawB bool
	for _, c := range calls {
		if c.nodeID == nodeDefs[1].ID {
			sawB = true
		}
	}
	assert.True(t, sawB, "node B output should be observed")
}

// TestEngine_OutputObserver_NilSafe 는 옵저버가 주입되지 않은 경우
// 플로우 실행이 정상 동작하며 panic이 없는지 검증한다 (zero-overhead 경로).
func TestEngine_OutputObserver_NilSafe(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)
	f, nodeDefs := newSimpleFlow()
	ctx := context.Background()

	require.NoError(t, e.DeployFlow(ctx, f))
	require.NoError(t, e.StartFlow(ctx, f.ID()))

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

	require.NotPanics(t, func() {
		_ = wireAB.Send(ctx, message.New())
		time.Sleep(50 * time.Millisecond)
	})

	require.NoError(t, e.StopFlow(ctx, f.ID()))
}

// TestEngine_SetOutputObserver 는 SetOutputObserver로 사후 주입이 가능한지 검증한다.
func TestEngine_SetOutputObserver(t *testing.T) {
	e := NewEngine()
	assert.Nil(t, e.OutputObserver(), "observer should be nil by default")

	obs := &fakeOutputObserver{}
	e.SetOutputObserver(obs)
	assert.NotNil(t, e.OutputObserver(), "observer should be set")
}
